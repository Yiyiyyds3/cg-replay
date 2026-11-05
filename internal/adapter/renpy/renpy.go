// renpy —— Ren'Py 游戏的「事件库直连」适配器。
//
// 职责（与 rmmz 同构）：
//   - Scan：扫 game 目录的明文 .rpy，把每个含图片演出的 label 登记为一个事件；
//   - BuildShow：把一个 label 的语句序列翻译成 IR 演出台本（图层 + 台词 + 转场）。
//
// 图像解析三条路（按优先级）：
//  1. `image 名 = "路径"` 显式定义（相对 game 目录）；
//  2. game 目录下的明文图像文件（Ren'Py 自动按「目录名+文件名」定义图像）；
//  3. game 目录下 .rpa 归档里的同名资源——播放时**按需解包**到
//     game/.ucgr-cache/（不改写游戏文件，缓存以归档内原路径存放）。
//
// 只解析明文 .rpy；剧本全在 .rpyc/.rpa 里时 Scan 报错说明（不做反编译）。
package renpy

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"ucgr/internal/decode"
	"ucgr/internal/ir"
)

// EngineID 引擎 id（约定文件 / 事件库都用它分派）。
const EngineID = "renpy"

// 缓存目录名（game 目录下，解包 .rpa 资源用）。
const cacheDirName = ".ucgr-cache"

// imageExts Ren'Py 自动图像的扩展名。
var imageExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".gif": true,
}

// Ref 一个可回放的演出（= 一个 label）。
type Ref struct {
	Key    string // label 名（重复时加文件名前缀）
	File   string // 相对 gameDir 的斜杠路径
	Title  string // label 名（后续可换首句台词）
	Images int    // 可解析的图片语句数
	Texts  int    // 台词行数
}

// ScanResult 一次扫描的结果与诊断。
type ScanResult struct {
	GameDir  string // 相对 root 的斜杠路径
	Refs     []Ref
	Warnings []string
	Scripts  int // 解析的 .rpy 数
	Images   int // 明文图像文件数
	Archives int // .rpa 归档数（含剧本锁死的情况）
}

// FindGameDir 找游戏的 game 目录（相对 root 的斜杠路径）。
//
// 覆盖三种摆放：root/game（标准）、root 的 1~3 层子目录里的 game（整合包
// 套壳，目录名大小写不限）、root 本身就是 game 目录（用户直接选了 game）。
func FindGameDir(root string) (string, bool) {
	// ① root 本身：有 .rpy/.rpa 或 images 目录就算。
	if isGameDir(root) {
		return ".", true
	}
	// ② 逐层向下找名为 game 的目录（BFS，最多 3 层）。
	type node struct {
		rel   string
		depth int
	}
	queue := []node{{"", 0}}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		if n.depth > 3 {
			continue
		}
		ents, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(n.rel)))
		if err != nil {
			continue
		}
		for _, e := range ents {
			if !e.IsDir() {
				continue
			}
			rel := e.Name()
			if n.rel != "" {
				rel = n.rel + "/" + e.Name()
			}
			if strings.EqualFold(e.Name(), "game") {
				full := filepath.Join(root, filepath.FromSlash(rel))
				if isGameDir(full) {
					return rel, true
				}
			}
			queue = append(queue, node{rel, n.depth + 1})
		}
	}
	return "", false
}

// isGameDir 目录像不像 game 目录：有 .rpy/.rpyc/.rpa 或 images 子目录。
func isGameDir(dir string) bool {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	hasImages := false
	for _, e := range ents {
		name := e.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if e.IsDir() {
			if strings.EqualFold(name, "images") {
				hasImages = true
			}
			continue
		}
		if ext == ".rpy" || ext == ".rpyc" || ext == ".rpa" {
			return true
		}
	}
	return hasImages
}

// Scan 扫 game 目录，列出可回放的 label 事件。
func Scan(root, gameDir string) (*ScanResult, error) {
	dir := root
	if gameDir != "" && gameDir != "." {
		dir = filepath.Join(root, filepath.FromSlash(gameDir))
	}
	res := &ScanResult{GameDir: filepath.ToSlash(gameDir)}

	// ① 收明文脚本（跳过翻译目录与缓存目录）。
	var scripts []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // 单个文件读不了不挡全库
		}
		if d.IsDir() {
			name := d.Name()
			if strings.EqualFold(name, "tl") || strings.EqualFold(name, cacheDirName) ||
				strings.EqualFold(name, "saves") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.EqualFold(filepath.Ext(d.Name()), ".rpy") {
			scripts = append(scripts, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("遍历 game 目录失败：%w", err)
	}
	sort.Strings(scripts)
	res.Scripts = len(scripts)
	if len(scripts) == 0 {
		hint := ""
		if n := countArchives(dir); n > 0 {
			res.Archives = n
			hint = fmt.Sprintf("（发现 %d 个 .rpa 归档，剧本锁在里面；需要先解包出明文 .rpy）", n)
		}
		return nil, fmt.Errorf("game 目录里没有明文 .rpy 剧本%s", hint)
	}

	ix := newIndex(dir, root, gameDir)

	// ② 每个 label 一个事件；可解析图片数为 0 的跳过。
	type labelRef struct {
		ref   Ref
		order int
	}
	seen := map[string]bool{}
	var refs []labelRef
	for _, sp := range scripts {
		sc, err := ParseScriptFile(sp)
		if err != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("%s 解析失败：%v",
				filepath.Base(sp), err))
			continue
		}
		for k, v := range sc.ImageDefs {
			if _, ok := ix.defs[k]; !ok {
				ix.defs[k] = v
			}
		}
		relFile := ix.relOf(sp)
		for _, label := range sc.Labels {
			key := label
			if seen[key] {
				key = strings.TrimSuffix(filepath.Base(sp), filepath.Ext(sp)) + "_" + label
			}
			seen[key] = true
			imgs, texts := 0, 0
			for _, st := range sc.Body[label] {
				switch st.Kind {
				case "scene", "show":
					if _, _, ok := ix.resolve(st.Name); ok {
						imgs++
					}
				case "say":
					if strings.TrimSpace(st.Text) != "" {
						texts++
					}
				}
			}
			if imgs == 0 {
				continue // 纯菜单/初始化 label
			}
			refs = append(refs, labelRef{ref: Ref{
				Key: key, File: relFile, Title: label,
				Images: imgs, Texts: texts,
			}, order: len(refs)})
		}
		for _, w := range sc.Warnings {
			res.Warnings = append(res.Warnings, filepath.Base(sp)+" "+w)
		}
	}
	res.Images = len(ix.files)
	res.Archives = len(ix.packs)
	res.Warnings = append(res.Warnings, ix.warn...)
	for _, r := range refs {
		res.Refs = append(res.Refs, r.ref)
	}
	if len(res.Refs) == 0 {
		return nil, fmt.Errorf("game 目录里没有含图片演出的 label（解析 %d 个脚本，明文图像 %d 个，警告 %d 条）",
			res.Scripts, res.Images, len(res.Warnings))
	}
	return res, nil
}

func countArchives(dir string) int {
	n := 0
	ents, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	for _, e := range ents {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".rpa") {
			n++
		}
	}
	return n
}

// imageIndex 图像名 → 文件的三级解析器。
type imageIndex struct {
	defs    map[string]string // image 语句定义：小写空格名 → 归档相对路径
	files   map[string]string // 明文图像：小写空格名 → 归档相对路径
	packs   map[string]*RPA   // 归档内路径（小写）→ 归档
	archive map[string]string // 小写空格名 → 归档内路径
	// filesSuffix / archiveSuffix 去掉首级目录名后的兜底键（千千阙歌：
	// 文件在 images/backgrounds/office reception day.jpg，脚本却写
	// `show office reception day`——真实引擎按后缀模糊匹配）。键可能
	// 撞名，值按字典序取第一个。
	filesSuffix   map[string][]string
	archiveSuffix map[string][]string
	warn          []string // 归档索引解析失败等诊断
	gameDir       string   // 绝对路径
	root          string   // 游戏根（算 Ref 用）
	rel           string   // gameDir 相对 root 的斜杠路径
}

func newIndex(dir, root, rel string) *imageIndex {
	ix := &imageIndex{
		defs:          map[string]string{},
		files:         map[string]string{},
		packs:         map[string]*RPA{},
		archive:       map[string]string{},
		filesSuffix:   map[string][]string{},
		archiveSuffix: map[string][]string{},
		gameDir:       dir,
		root:          root,
		rel:           filepath.ToSlash(rel),
	}
	// ① 明文图像（game 目录递归，跳过 tl/saves/cache）。
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			switch strings.EqualFold(d.Name(), "tl") || strings.EqualFold(d.Name(), "saves") ||
				strings.EqualFold(d.Name(), cacheDirName) {
			case true:
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		if !imageExts[ext] {
			return nil
		}
		relPath, err := filepath.Rel(dir, path)
		if err != nil {
			return nil
		}
		name := imageKeyFromPath(filepath.ToSlash(relPath))
		if _, dup := ix.files[name]; !dup {
			ix.files[name] = filepath.ToSlash(relPath)
		}
		addSuffixKey(ix.filesSuffix, name)
		return nil
	})
	// ② .rpa 归档索引（读索引不读数据体，GB 级归档也只有几 MB 索引）。
	ents, err := os.ReadDir(dir)
	if err == nil {
		for _, e := range ents {
			if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".rpa") {
				continue
			}
			a, err := OpenRPA(filepath.Join(dir, e.Name()))
			if err != nil {
				ix.warn = append(ix.warn, fmt.Sprintf("归档 %s 索引解析失败：%v", e.Name(), err))
				continue
			}
			for name := range a.Files {
				lower := strings.ToLower(name)
				ix.packs[lower] = a
				key := imageKeyFromPath(lower)
				if _, dup := ix.archive[key]; !dup {
					ix.archive[key] = name
				}
				addSuffixKey(ix.archiveSuffix, key)
			}
		}
	}
	return ix
}

// imageKeyFromPath "images/002.png" → "002"、"CG/NunCG/x.jpg" → "cg nuncg x"。
//
// Ren'Py 自动图像名 = 路径去掉扩展名、分隔符换空格；images/ 目录本身不算
// 进名字（game/images/002.png 的图像名就是 002）。
func imageKeyFromPath(rel string) string {
	rel = strings.TrimSuffix(rel, filepath.Ext(rel))
	parts := strings.Split(rel, "/")
	if len(parts) > 1 && strings.EqualFold(parts[0], "images") {
		parts = parts[1:]
	}
	return strings.ToLower(strings.Join(parts, " "))
}

// addSuffixKey 把「去掉首级目录名」的兜底键挂进表（backgrounds office day → office day）。
func addSuffixKey(m map[string][]string, key string) {
	if i := strings.IndexByte(key, ' '); i >= 0 {
		suf := key[i+1:]
		m[suf] = append(m[suf], key)
	}
}

// suffixPick 取后缀键的候选（字典序第一个，保证确定性）；没有返回 false。
func suffixPick(m map[string][]string, key string) (string, bool) {
	cands, ok := m[key]
	if !ok || len(cands) == 0 {
		return "", false
	}
	best := cands[0]
	for _, c := range cands[1:] {
		if c < best {
			best = c
		}
	}
	return best, true
}

func (ix *imageIndex) relOf(abs string) string {
	r, err := filepath.Rel(ix.gameDir, abs)
	if err != nil {
		return filepath.ToSlash(abs)
	}
	return filepath.ToSlash(r)
}

// resolve 图像名 → (归档相对路径, 是否在 .rpa 里, 是否找得到)。
func (ix *imageIndex) resolve(name string) (string, bool, bool) {
	key := strings.ToLower(strings.Join(strings.Fields(name), " "))
	if key == "" {
		return "", false, false
	}
	if v, ok := ix.defs[key]; ok {
		// 显式定义指向明文文件？
		if _, err := os.Stat(filepath.Join(ix.gameDir, filepath.FromSlash(v))); err == nil {
			return v, false, true
		}
		// 定义指向归档内路径？（部分打包器在归档里统一挂 images/ 前缀）
		if _, ok := ix.packs[strings.ToLower(v)]; ok {
			return v, true, true
		}
		if _, ok := ix.packs["images/"+strings.ToLower(v)]; ok {
			return "images/" + v, true, true
		}
	}
	if v, ok := ix.files[key]; ok {
		return v, false, true
	}
	if v, ok := ix.archive[key]; ok {
		return v, true, true
	}
	// 后缀兜底：脚本省略了子目录名（真实引擎的模糊匹配行为）。
	if k, ok := suffixPick(ix.filesSuffix, key); ok {
		return ix.files[k], false, true
	}
	if k, ok := suffixPick(ix.archiveSuffix, key); ok {
		return ix.archive[k], true, true
	}
	// 兜底：明文文件直接按名字找（scene 039 → game/images/039.jpg 由
	// 自动图像覆盖；这里兜的是带扩展名写在语句里的情况）。
	if strings.Contains(name, ".") {
		rel := filepath.ToSlash(name)
		if _, err := os.Stat(filepath.Join(ix.gameDir, filepath.FromSlash(rel))); err == nil {
			return rel, false, true
		}
	}
	return "", false, false
}

// extract 把 .rpa 里的资源解包到缓存（game/.ucgr-cache/<归档内路径>），返回缓存路径。
func (ix *imageIndex) extract(entryPath string) (string, error) {
	a, ok := ix.packs[strings.ToLower(entryPath)]
	if !ok {
		return "", fmt.Errorf("归档里没有 %s", entryPath)
	}
	data, err := a.ReadFile(entryPath)
	if err != nil {
		return "", err
	}
	dst := filepath.Join(ix.gameDir, cacheDirName, filepath.FromSlash(entryPath))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return "", err
	}
	return dst, nil
}

// refFromRoot 归档相对路径 → 相对 root 的斜杠路径（明文 = gameDir 前缀）。
func (ix *imageIndex) refFromRoot(rel string) string {
	if ix.rel == "" || ix.rel == "." {
		return rel
	}
	return ix.rel + "/" + rel
}

// ---- BuildShow ----

// 转场近似时长（毫秒）。
var transitionMs = map[string]int64{
	"dissolve": 500, "fade": 1000, "flash": 300, "hpunch": 300, "vpunch": 300,
	"pixellate": 500, "move": 500, "zoomin": 500,
}

// BuildShow 把一个 label 的语句翻译成 IR 演出台本。
func BuildShow(root, gameDir, key string) (*ir.Event, error) {
	dir := root
	if gameDir != "" && gameDir != "." {
		dir = filepath.Join(root, filepath.FromSlash(gameDir))
	}
	ix := newIndex(dir, root, gameDir)

	// 找 label 所在脚本与其语句序列。
	var stmts []Stmt
	var fileRel string
	var scripts []string
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if strings.EqualFold(name, "tl") || strings.EqualFold(name, cacheDirName) ||
				strings.EqualFold(name, "saves") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.EqualFold(filepath.Ext(d.Name()), ".rpy") {
			scripts = append(scripts, path)
		}
		return nil
	})
	sort.Strings(scripts)
	for _, sp := range scripts {
		sc, err := ParseScriptFile(sp)
		if err != nil {
			continue
		}
		for k, v := range sc.ImageDefs {
			ix.defs[k] = v
		}
		if body, ok := sc.Body[key]; ok {
			stmts = body
			fileRel = ix.relOf(sp)
			break
		}
	}
	if stmts == nil {
		return nil, fmt.Errorf("game 目录里找不到 label %s", key)
	}

	ev := &ir.Event{
		EventID: key,
		Kind:    ir.KindScene,
		Title:   key,
		Assets:  []ir.AssetRef{},
	}
	var clock int64
	// shown 维护「当前显示中的 tag → 图层名」，scene 清空它们（Ren'Py 语义）。
	type shownLayer struct{ ref string }
	shown := map[string]shownLayer{} // tag → 层
	layerOf := func(tag string) string {
		return "layer_" + strings.ReplaceAll(strings.ToLower(tag), " ", "_")
	}
	assetSeen := map[string]bool{}
	addAsset := func(ref string) {
		if !assetSeen[ref] {
			assetSeen[ref] = true
			ev.Assets = append(ev.Assets, ir.AssetRef{
				Hash:     "", // 播放器按 OrigPath 直读
				OrigPath: ref,
				Role:     "image",
			})
		}
	}
	// showTag 处理 scene/show：解析、解包、产 layer_show。
	showTag := func(tag, name string, replace bool) {
		rel, inRPA, ok := ix.resolve(name)
		if !ok {
			return
		}
		ref := ix.refFromRoot(rel)
		if inRPA {
			dst, err := ix.extract(rel)
			if err != nil {
				return
			}
			r, err := filepath.Rel(root, dst)
			if err != nil {
				return
			}
			ref = filepath.ToSlash(r)
		}
		layer := layerOf(tag)
		if replace {
			for t, s := range shown {
				ev.Show = append(ev.Show, ir.ShowOp{TMs: clock, Kind: ir.ShowLayerHide, Layer: layerOf(t)})
				delete(shown, t)
				_ = s
			}
		}
		w, h := imageDims(filepath.Join(root, filepath.FromSlash(ref)))
		left, top, scale := fitStage(w, h)
		ev.Show = append(ev.Show, ir.ShowOp{
			TMs: clock, Kind: ir.ShowLayerShow, Layer: layer, Ref: ref,
			Attr: map[string]string{
				"left":  fmt.Sprintf("%.1f", left),
				"top":   fmt.Sprintf("%.1f", top),
				"scale": fmt.Sprintf("%.6f", scale),
			},
		})
		shown[tag] = shownLayer{ref: ref}
		addAsset(ref)
	}
	sayText := func(speaker, text string) {
		if strings.TrimSpace(text) == "" {
			return
		}
		for _, line := range wrapText(text, 30) {
			ev.Show = append(ev.Show, ir.ShowOp{
				TMs: clock, Kind: ir.ShowTextRun, Layer: "message0",
				Text: &ir.ShowText{Speaker: speaker, Text: line},
			})
			clock += int64(len([]rune(line))) * 60
		}
		ev.Show = append(ev.Show, ir.ShowOp{TMs: clock, Kind: ir.ShowTextWait})
		clock += 1200
	}

	for _, st := range stmts {
		switch st.Kind {
		case "scene":
			showTag(st.Name, st.Name, true)
		case "show":
			tag := firstToken(st.Name)
			showTag(tag, st.Name, false)
		case "hide":
			tag := strings.ToLower(strings.Join(strings.Fields(st.Name), " "))
			if _, ok := shown[tag]; ok {
				ev.Show = append(ev.Show, ir.ShowOp{
					TMs: clock, Kind: ir.ShowLayerHide, Layer: layerOf(tag)})
				delete(shown, tag)
			}
		case "say":
			sayText(st.Name, st.Text)
		case "with":
			dur, ok := transitionMs[strings.ToLower(st.Name)]
			if !ok {
				dur = 500
			}
			ev.Show = append(ev.Show, ir.ShowOp{TMs: clock, Kind: ir.ShowWait, DurMs: dur})
			clock += dur
		case "pause":
			ev.Show = append(ev.Show, ir.ShowOp{TMs: clock, Kind: ir.ShowWait, DurMs: 800})
			clock += 800
		}
	}
	if len(ev.Show) == 0 {
		return nil, fmt.Errorf("label %s 没有可回放的演出（图片可能全在 .rpyc/.rpa 里）", key)
	}
	_ = fileRel
	ev.Show = append(ev.Show, ir.ShowOp{TMs: clock, Kind: ir.ShowWait, DurMs: 1500})
	ev.EndMs = clock + 1500
	return ev, nil
}

// imageDims 读图像尺寸（读不了返回 0,0）。
func imageDims(path string) (int, int) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, 0
	}
	img, err := decode.DecodeImage(data)
	if err != nil {
		return 0, 0
	}
	b := img.Bounds()
	return b.Dx(), b.Dy()
}

// fitStage 把 w×h 的图等比放进 1600×1200 舞台并居中。
// 尺寸未知时按 1920×1080 估（Ren'Py 的绝对主流分辨率）。
func fitStage(w, h int) (left, top, scale float64) {
	if w <= 0 || h <= 0 {
		w, h = 1920, 1080
	}
	fx := 1600.0 / float64(w)
	fy := 1200.0 / float64(h)
	s := fx
	if fy < s {
		s = fy
	}
	return (1600 - float64(w)*s) / 2, (1200 - float64(h)*s) / 2, s
}

// wrapText 按消息窗宽度折行（30 个全角字）。
func wrapText(s string, max int) []string {
	var out []string
	for _, seg := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		rs := []rune(strings.TrimRight(seg, " \t"))
		for len(rs) > max {
			out = append(out, string(rs[:max]))
			rs = rs[max:]
		}
		if len(rs) > 0 {
			out = append(out, string(rs))
		}
	}
	if len(out) == 0 {
		out = []string{s}
	}
	return out
}
