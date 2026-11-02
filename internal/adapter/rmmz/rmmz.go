// rmmz —— RPG Maker MV/MZ 事件数据（data/*.json）到统一演出的翻译器。
//
// RPG Maker 的「剧本」不是文本脚本，而是编辑器序列化出来的事件指令：
// 地图事件 / 公共事件 / 敌群事件里每个 page 都是一串 {code, indent, parameters}。
// CG 演出常见写法就是「显示图片(231) + 台词(401)」循环，所以这里按
// 「一段连续的显示图片」切演出段落，把 231/232/235/401 翻成 IR 台本。
//
// 素材侧只读不写：图片按 img/pictures/<name>.<ext> 找，
// MV 的 .rpgmvp / MZ 的 .png_ 由 decode.DecodeImage 在解码时还原。
package rmmz

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"ucgr/internal/decode"
	"ucgr/internal/ir"
)

// EngineID 引擎 id（约定文件 / 事件库都用它分派）。
const EngineID = "rmmz"

// 默认舞台尺寸：RPG Maker MV/MZ 的 boxWidth/boxHeight。
const (
	CanvasW = 816
	CanvasH = 624
)

// Command 一条事件指令。
type Command struct {
	Code   int   `json:"code"`
	Indent int   `json:"indent"`
	Params []any `json:"parameters"`
}

func (c Command) num(i int) float64 {
	if i < 0 || i >= len(c.Params) {
		return 0
	}
	switch v := c.Params[i].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	}
	return 0
}

func (c Command) str(i int) string {
	if i < 0 || i >= len(c.Params) {
		return ""
	}
	if s, ok := c.Params[i].(string); ok {
		return s
	}
	return ""
}

// System 只取用得到的字段。
type System struct {
	GameTitle     string `json:"gameTitle"`
	EncryptionKey string `json:"encryptionKey"`
}

// readJSON 读 JSON 并去掉 UTF-8 BOM。
//
// 不少汉化/移植版给地图文件加了 BOM，标准库 json 会直接报
// "invalid character 'ï'"，这里统一在入口处理掉。
func readJSON(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return bytes.TrimPrefix(raw, []byte("\xEF\xBB\xBF")), nil
}

// i18nEntry I18NTexts.json 的一条（只取用得上的语言）。
type i18nEntry struct {
	ID    int    `json:"id"`
	JA    string `json:"ja_JP"`
	ZH    string `json:"zh_CN"`
	ZH_TW string `json:"zh_TW"`
}

// LoadI18N 读游戏自带的翻译表，返回 id → 文本（中文优先）。
//
// 这类游戏把台词写成 `\I18N[13226]` 占位，真正文本在 data/I18NTexts.json；
// 不查这张表，回放里就只剩一串编号。表缺失时返回空 map，调用方退化处理。
func LoadI18N(root, dataDir string) map[int]string {
	out := map[int]string{}
	raw, err := readJSON(filepath.Join(root, filepath.FromSlash(dataDir), "I18NTexts.json"))
	if err != nil {
		return out
	}
	var entries []i18nEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return out
	}
	for _, e := range entries {
		switch {
		case strings.TrimSpace(e.ZH) != "":
			out[e.ID] = e.ZH
		case strings.TrimSpace(e.JA) != "":
			out[e.ID] = e.JA
		case strings.TrimSpace(e.ZH_TW) != "":
			out[e.ID] = e.ZH_TW
		}
	}
	return out
}

// LoadLocale 读 DKTools_Localization 的翻译表（locales/<locale>/*.json）。
//
// 这类游戏把台词写成 `{MAP0010001000101}`、`{CName002}` 占位键，
// 真正的文本按语言放在 www/locales/zh-CN 下。中文目录不存在时返回空 map，
// 避免把英文/日文当中文混进来。
func LoadLocale(root, dataDir string) map[string]string {
	out := map[string]string{}
	parent := filepath.ToSlash(filepath.Dir(dataDir))
	if parent == "." {
		parent = ""
	}
	var bases []string
	if parent != "" {
		bases = append(bases, parent+"/locales")
	}
	bases = append(bases, "locales")
	for _, base := range bases {
		dir := filepath.Join(root, filepath.FromSlash(base))
		for _, name := range []string{"zh-CN", "zh_CN", "zh", "zh-TW", "zh_TW"} {
			loc := filepath.Join(dir, name)
			ents, err := os.ReadDir(loc)
			if err != nil {
				continue
			}
			for _, e := range ents {
				if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".json") {
					continue
				}
				raw, rerr := readJSON(filepath.Join(loc, e.Name()))
				if rerr != nil {
					continue
				}
				var m map[string]any
				if json.Unmarshal(raw, &m) != nil {
					continue
				}
				for k, v := range m {
					if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
						out[k] = s
					}
				}
			}
			if len(out) > 0 {
				return out
			}
		}
	}
	return out
}

// Ref 一个可回放的演出段落。
type Ref struct {
	Key      string // map:Map001.json:12:0#0 / common:5#0 / troop:3:0#0
	File     string // data 目录下的文件名
	Title    string
	Preview  string // 首句台词（卡片副标题）
	Pictures int
	Texts    int
}

// ScanResult 一次扫描的结果与诊断。
type ScanResult struct {
	DataDir  string
	ImageDir string // 主图片目录（相对 root），空 = 没找到
	System   System
	Refs     []Ref
	Warnings []string

	sizes   map[string]picInfo
	cleaner textCleaner
}

// picInfo 图片尺寸与「像不像 CG」的缓存。
type picInfo struct {
	w, h  int
	large bool
}

// FindDataDir 在游戏根目录里找 data 目录（相对 root 的斜杠路径）。
//
// 覆盖三种常见摆放：
//   - MV：www/data
//   - MZ：data
//   - 汉化/外挂整合包：内嵌/data（图片可能在 外挂/Game/img）
func FindDataDir(root string) (string, bool) {
	cands := []string{"www/data", "data", "内嵌/data"}
	for _, c := range cands {
		if hasSystemFile(filepath.Join(root, filepath.FromSlash(c))) {
			return c, true
		}
	}
	// 再退一层：<子目录>/www/data、<子目录>/data（用户选到了整合包外层时）。
	ents, err := os.ReadDir(root)
	if err != nil {
		return "", false
	}
	for _, e := range ents {
		// 这里必须 os.Stat 跟随联接：ReadDir 返回的 DirEntry.IsDir() 对
		// junction/符号联接是 false（Lstat 语义），而游戏库条目本身就是联接，
		// 整合包外层再套一层联接时会整层被跳过。
		sub := filepath.Join(root, e.Name())
		if st, serr := os.Stat(sub); serr != nil || !st.IsDir() {
			continue
		}
		for _, c := range []string{"www/data", "data"} {
			rel := e.Name() + "/" + c
			if hasSystemFile(filepath.Join(root, filepath.FromSlash(rel))) {
				return filepath.ToSlash(rel), true
			}
		}
	}
	return "", false
}

func hasSystemFile(dir string) bool {
	st, err := os.Stat(filepath.Join(dir, "System.json"))
	return err == nil && !st.IsDir()
}

// ImageDirs 列出可能的 img 目录（相对 root），按可信度排序。
func ImageDirs(root, dataDir string) []string {
	parent := filepath.ToSlash(filepath.Dir(dataDir))
	if parent == "." {
		parent = ""
	}
	var raw []string
	if parent != "" {
		raw = append(raw, parent+"/img")
	}
	raw = append(raw,
		"img", "www/img",
		"内嵌/img", "外挂/Game/img", "外挂/img",
		"../外挂/Game/img", "../img",
	)
	seen := map[string]bool{}
	var out []string
	for _, d := range raw {
		if d == "" || seen[d] {
			continue
		}
		seen[d] = true
		if st, err := os.Stat(filepath.Join(root, filepath.FromSlash(d))); err == nil && st.IsDir() {
			out = append(out, d)
		}
	}
	return out
}

// MainImageDir 返回第一个含 pictures 子目录的 img 目录。
func MainImageDir(root string, dirs []string) string {
	for _, d := range dirs {
		if st, err := os.Stat(filepath.Join(root, filepath.FromSlash(d), "pictures")); err == nil && st.IsDir() {
			return d
		}
	}
	return ""
}

var extTry = []string{".png", ".jpg", ".jpeg", ".webp", ".rpgmvp", ".png_"}

// LocatePicture 按图片名找实际文件（相对 root），找不到返回 false。
//
// 除了常规扩展名，还要兼容 DLsite 审查版的「马赛克变体」：BuildModeChange
// 插件（dlsite/fanza 模式）会在原名后加 "JP" 再取图，例如
// CG/ChelseaH7/ChelseaH7_01 → CG/ChelseaH7/ChelseaH7_01JP.png；
// 这类包里往往只保留 JP 文件，不回退就会整段 CG 找不到图。
func LocatePicture(root string, imageDirs []string, name string) (string, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", false
	}
	name = strings.TrimSuffix(name, filepath.Ext(name))
	for _, dir := range imageDirs {
		for _, suffix := range []string{"", "JP"} {
			for _, ext := range extTry {
				rel := filepath.ToSlash(filepath.Join(dir, "pictures", name+suffix+ext))
				if st, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err == nil && !st.IsDir() {
					return rel, true
				}
			}
		}
	}
	return "", false
}

// LoadSystem 读 System.json（缺文件时返回零值，不报错）。
func LoadSystem(root, dataDir string) System {
	var s System
	data, err := readJSON(filepath.Join(root, filepath.FromSlash(dataDir), "System.json"))
	if err != nil {
		return s
	}
	_ = json.Unmarshal(data, &s)
	return s
}

type mapFile struct {
	DisplayName string `json:"displayName"`
	Events      []struct {
		ID    int    `json:"id"`
		Name  string `json:"name"`
		Pages []struct {
			List []Command `json:"list"`
		} `json:"pages"`
	} `json:"events"`
}

type commonEvent struct {
	ID   int       `json:"id"`
	Name string    `json:"name"`
	List []Command `json:"list"`
}

type troop struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Pages []struct {
		List []Command `json:"list"`
	} `json:"pages"`
}

// Scan 扫一个游戏的 data 目录，列出含图片演出的段落。
func Scan(root, dataDir string) (*ScanResult, error) {
	dir := filepath.Join(root, filepath.FromSlash(dataDir))
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("读不到 data 目录 %s：%w", dir, err)
	}
	imgDirs := ImageDirs(root, dataDir)
	res := &ScanResult{
		DataDir:  dataDir,
		ImageDir: MainImageDir(root, imgDirs),
		System:   LoadSystem(root, dataDir),
		cleaner:  textCleaner{i18n: LoadI18N(root, dataDir), locale: LoadLocale(root, dataDir)},
	}

	var mapFiles []string
	for _, e := range ents {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, "Map") || !strings.EqualFold(filepath.Ext(name), ".json") {
			continue
		}
		if strings.EqualFold(name, "MapInfos.json") {
			continue
		}
		mapFiles = append(mapFiles, name)
	}
	sort.Strings(mapFiles)

	for _, name := range mapFiles {
		var mf mapFile
		raw, rerr := readJSON(filepath.Join(dir, name))
		if rerr != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("%s 读取失败：%v", name, rerr))
			continue
		}
		if err := json.Unmarshal(raw, &mf); err != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("%s 解析失败：%v", name, err))
			continue
		}
		for _, ev := range mf.Events {
			// 同一事件的多个 page 常常只是「初始 / 已读」条件不同、指令完全一样。
			// 逐页登记会在事件库里出现重复 CG，这里按指令内容去重并留下告警。
			pageSeen := map[string]int{}
			for pi, page := range ev.Pages {
				if sig := pageSignature(page.List); sig != "" {
					if first, ok := pageSeen[sig]; ok {
						res.Warnings = append(res.Warnings, fmt.Sprintf(
							"%s 事件 %d 的 page%d 与 page%d 指令相同，已跳过重复页", name, ev.ID, pi+1, first+1))
						continue
					}
					pageSeen[sig] = pi
				}
				key := fmt.Sprintf("map:%s:%d:%d", name, ev.ID, pi)
				res.addSegments(key, name, MapGroup(name, mf.DisplayName), ev.Name, page.List, root, imgDirs)
			}
		}
	}

	// 公共事件：不少 H 演出挂在这里（地图事件只负责调用）。
	if raw, rerr := readJSON(filepath.Join(dir, "CommonEvents.json")); rerr == nil {
		var ces []commonEvent
		if err := json.Unmarshal(raw, &ces); err != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("CommonEvents.json 解析失败：%v", err))
		} else {
			for _, ce := range ces {
				if len(ce.List) == 0 {
					continue
				}
				key := fmt.Sprintf("common:%d", ce.ID)
				res.addSegments(key, "CommonEvents.json", "公共事件", ce.Name, ce.List, root, imgDirs)
			}
		}
	}

	// 敌群事件：战斗 H 的演出在这里。
	if raw, rerr := readJSON(filepath.Join(dir, "Troops.json")); rerr == nil {
		var troops []troop
		if err := json.Unmarshal(raw, &troops); err != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("Troops.json 解析失败：%v", err))
		} else {
			for _, tr := range troops {
				for pi, page := range tr.Pages {
					if len(page.List) == 0 {
						continue
					}
					key := fmt.Sprintf("troop:%d:%d", tr.ID, pi)
					res.addSegments(key, "Troops.json", "战斗事件", tr.Name, page.List, root, imgDirs)
				}
			}
		}
	}
	return res, nil
}

// pageSignature 用于识别内容完全相同的事件页。
func pageSignature(cmds []Command) string {
	b, err := json.Marshal(cmds)
	if err != nil {
		return ""
	}
	return string(b)
}

func (r *ScanResult) addSegments(key, file, group, name string, cmds []Command, root string, imgDirs []string) {
	segs := Segments(cmds)
	for si, seg := range segs {
		pics, texts := 0, 0
		preview := ""
		for _, c := range seg {
			switch c.Code {
			case 231:
				if rel, ok := LocatePicture(root, imgDirs, c.str(1)); ok && r.pictureLarge(root, rel) {
					pics++
				}
			case 401:
				text := r.cleaner.clean(c.str(0))
				if t := Flatten(text); t != "" {
					// 首行常常只是人名；太短就继续往后找一句完整的当标题。
					if len([]rune(preview)) < 6 && len([]rune(t)) > len([]rune(preview)) {
						preview = t
					}
					texts++
				}
			}
		}
		if pics == 0 || texts == 0 {
			continue // 只有 UI 图片或纯文字，不是 CG 演出
		}
		if n := len([]rune(preview)); n > 24 {
			preview = string([]rune(preview)[:24]) + "…"
		}
		title := Flatten(r.cleaner.clean(group))
		if n := Flatten(r.cleaner.clean(name)); n != "" && n != title {
			title += " · " + n
		}
		if preview != "" {
			title += " · " + preview
		}
		r.Refs = append(r.Refs, Ref{
			Key:      fmt.Sprintf("%s#%d", key, si),
			File:     file,
			Title:    title,
			Preview:  preview,
			Pictures: pics,
			Texts:    texts,
		})
	}
}

// pictureLarge 判断图片是不是「CG/立绘」量级。
//
// RPG Maker 的图片素材统一是 816×624 画布，行走图也只是画布里的一小块，
// 所以不能只看尺寸，要看**不透明像素的占比**：CG/立绘会铺满大半张，
// 行走图/图标只有几个百分点。抽样检查（每 3 像素取一个）足够快。
func (r *ScanResult) pictureLarge(root, rel string) bool {
	if r.sizes == nil {
		r.sizes = map[string]picInfo{}
	}
	if d, ok := r.sizes[rel]; ok {
		return d.large
	}
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		r.sizes[rel] = picInfo{}
		return false
	}
	img, err := decode.DecodeImage(data)
	if err != nil {
		r.sizes[rel] = picInfo{}
		return false
	}
	b := img.Bounds()
	opaque, total := 0, 0
	for y := b.Min.Y; y < b.Max.Y; y += 3 {
		for x := b.Min.X; x < b.Max.X; x += 3 {
			total++
			_, _, _, a := img.At(x, y).RGBA()
			if a > 0x2000 {
				opaque++
			}
		}
	}
	ok := total > 0 && b.Dx()*b.Dy() >= 60000 && float64(opaque)/float64(total) >= 0.10
	r.sizes[rel] = picInfo{w: b.Dx(), h: b.Dy(), large: ok}
	return ok
}

// MapGroup 地图显示名；没起名时用「地图<编号>」（Map149.json → 地图149）。
func MapGroup(file, displayName string) string {
	if s := strings.TrimSpace(displayName); s != "" {
		return s
	}
	base := strings.TrimSuffix(file, filepath.Ext(file))
	return "地图" + strings.TrimPrefix(base, "Map")
}

// Segments 把 page 的指令切成演出段落：
// 一段 = 从第一次「显示图片」起，到「全部消除图片」或列表结束。
func Segments(cmds []Command) [][]Command {
	var out [][]Command
	var cur []Command
	visible := map[int]bool{}
	active := false
	for _, c := range cmds {
		if c.Code == 231 {
			id := int(c.num(0))
			if !active {
				active = true
				cur = nil
			}
			visible[id] = true
		}
		if active {
			cur = append(cur, c)
		}
		if c.Code == 235 {
			id := int(c.num(0))
			if id <= 0 {
				visible = map[int]bool{}
			} else {
				delete(visible, id)
			}
		}
		if active && len(visible) == 0 {
			out = append(out, cur)
			cur, active = nil, false
		}
	}
	if active && len(cur) > 0 {
		out = append(out, cur)
	}
	return out
}

// FindSegment 按 Ref.Key 找对应的指令段。
func FindSegment(root, dataDir, key string) ([]Command, error) {
	base, idxStr, ok := strings.Cut(key, "#")
	if !ok {
		return nil, fmt.Errorf("事件键格式不对：%s", key)
	}
	segIdx, err := strconv.Atoi(idxStr)
	if err != nil {
		return nil, fmt.Errorf("事件键段落号非法：%s", key)
	}
	dir := filepath.Join(root, filepath.FromSlash(dataDir))
	parts := strings.Split(base, ":")
	if len(parts) < 2 {
		return nil, fmt.Errorf("事件键格式不对：%s", key)
	}
	var list []Command
	switch parts[0] {
	case "map":
		if len(parts) < 4 {
			return nil, fmt.Errorf("地图事件键不完整：%s", key)
		}
		raw, err := readJSON(filepath.Join(dir, parts[1]))
		if err != nil {
			return nil, err
		}
		var mf mapFile
		if err := json.Unmarshal(raw, &mf); err != nil {
			return nil, err
		}
		evID, _ := strconv.Atoi(parts[2])
		page, _ := strconv.Atoi(parts[3])
		for _, ev := range mf.Events {
			if ev.ID != evID || page < 0 || page >= len(ev.Pages) {
				continue
			}
			list = ev.Pages[page].List
			break
		}
	case "common":
		raw, err := readJSON(filepath.Join(dir, "CommonEvents.json"))
		if err != nil {
			return nil, err
		}
		var ces []commonEvent
		if err := json.Unmarshal(raw, &ces); err != nil {
			return nil, err
		}
		id, _ := strconv.Atoi(parts[1])
		for _, ce := range ces {
			if ce.ID == id {
				list = ce.List
				break
			}
		}
	case "troop":
		if len(parts) < 3 {
			return nil, fmt.Errorf("敌群事件键不完整：%s", key)
		}
		raw, err := readJSON(filepath.Join(dir, "Troops.json"))
		if err != nil {
			return nil, err
		}
		var troops []troop
		if err := json.Unmarshal(raw, &troops); err != nil {
			return nil, err
		}
		id, _ := strconv.Atoi(parts[1])
		page, _ := strconv.Atoi(parts[2])
		for _, tr := range troops {
			if tr.ID == id && page >= 0 && page < len(tr.Pages) {
				list = tr.Pages[page].List
				break
			}
		}
	default:
		return nil, fmt.Errorf("未知事件类型：%s", key)
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("找不到事件 %s", key)
	}
	segs := Segments(list)
	if segIdx < 0 || segIdx >= len(segs) {
		return nil, fmt.Errorf("事件 %s 的段落 %d 不存在（共 %d 段）", key, segIdx, len(segs))
	}
	return segs[segIdx], nil
}

// BuildShow 把一个演出段落翻成 IR 台本。
//
// 坐标换算：RPG Maker 的 816×624 画布等比放进播放器的 1600×1200 舞台，
// 居中留黑边；图片层名用 layer<图片编号>，编号大的压在上面（与原作一致）。
func BuildShow(root, dataDir, key string) (*ir.Event, error) {
	cmds, err := FindSegment(root, dataDir, key)
	if err != nil {
		return nil, err
	}
	imgDirs := ImageDirs(root, dataDir)
	cleaner := textCleaner{i18n: LoadI18N(root, dataDir), locale: LoadLocale(root, dataDir)}
	ev := &ir.Event{EventID: key, Kind: ir.KindScene, Title: key}

	dims := map[string][2]int{}
	picDim := func(rel string) (int, int) {
		if d, ok := dims[rel]; ok {
			return d[0], d[1]
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			dims[rel] = [2]int{}
			return 0, 0
		}
		img, err := decode.DecodeImage(data)
		if err != nil {
			dims[rel] = [2]int{}
			return 0, 0
		}
		b := img.Bounds()
		dims[rel] = [2]int{b.Dx(), b.Dy()}
		return b.Dx(), b.Dy()
	}

	fx := float64(1600) / CanvasW
	fy := float64(1200) / CanvasH
	fit := fx
	if fy < fit {
		fit = fy
	}
	offX := (1600 - CanvasW*fit) / 2
	offY := (1200 - CanvasH*fit) / 2

	var clock int64
	visible := map[int]bool{}
	picRef := map[int]string{}
	picOpacity := map[int]int{}
	msgOpen := false
	speaker := ""
	finishMsg := func() {
		if !msgOpen {
			return
		}
		ev.Show = append(ev.Show, ir.ShowOp{TMs: clock, Kind: ir.ShowTextWait})
		clock += 1200
		msgOpen = false
	}

	// 对话框几何：底部横条（文字由前端按 BoxDTO 画）。
	ev.Show = append(ev.Show, ir.ShowOp{
		TMs: 0, Kind: ir.ShowLayerAttr, Layer: "message0",
		Attr: map[string]string{
			"left": strconv.Itoa(80), "top": strconv.Itoa(880),
			"width": strconv.Itoa(1440), "height": strconv.Itoa(250),
			"marginl": "24", "margint": "20", "marginr": "24", "marginb": "20",
		},
	})

	for _, c := range cmds {
		switch c.Code {
		case 101:
			finishMsg()
			speaker = Flatten(cleaner.clean(c.str(4)))
			ev.Show = append(ev.Show, ir.ShowOp{TMs: clock, Kind: ir.ShowTextClear, Layer: "message0"})
			msgOpen = true
		case 401:
			text := cleaner.clean(c.str(0))
			if strings.TrimSpace(text) == "" {
				continue
			}
			if !msgOpen {
				ev.Show = append(ev.Show, ir.ShowOp{TMs: clock, Kind: ir.ShowTextClear, Layer: "message0"})
				msgOpen = true
			}
			for _, line := range WrapText(text, 28) {
				ev.Show = append(ev.Show, ir.ShowOp{
					TMs: clock, Kind: ir.ShowTextRun, Layer: "message0",
					Text: &ir.ShowText{Speaker: speaker, Text: line},
				})
				clock += int64(len([]rune(line))) * 60
			}
		case 102:
			// 选项：把选项文本也当成一段消息显示（回放沿默认分支，不做交互）。
			var opts []string
			for i := 0; ; i++ {
				s := Flatten(cleaner.clean(c.str(i)))
				if s == "" {
					break
				}
				opts = append(opts, s)
			}
			if len(opts) > 0 {
				finishMsg()
				ev.Show = append(ev.Show,
					ir.ShowOp{TMs: clock, Kind: ir.ShowTextClear, Layer: "message0"},
					ir.ShowOp{TMs: clock, Kind: ir.ShowTextRun, Layer: "message0",
						Text: &ir.ShowText{Text: "【选择】" + strings.Join(opts, " / ")}},
					ir.ShowOp{TMs: clock, Kind: ir.ShowTextWait},
				)
				clock += 1500
			}
		case 231:
			id := int(c.num(0))
			name := c.str(1)
			rel, ok := LocatePicture(root, imgDirs, name)
			if !ok {
				continue
			}
			sx := c.num(6)
			sy := c.num(7)
			if sx <= 0 {
				sx = 100
			}
			if sy <= 0 {
				sy = 100
			}
			scale := fit * ((sx + sy) / 2) / 100
			// 0 是合法的目标透明度：常见写法是 231 先放 opacity=0 的图，
			// 再用 232（移动图片）在 duration 内淡入。只有参数缺失才用 255。
			opacity := 255.0
			if len(c.Params) > 8 {
				opacity = c.num(8)
			}
			// 直接指定（params[3]=0）时 x/y 是像素坐标；否则坐标来自游戏变量，
			// 静态回放读不到变量值，退回左上角，别把变量号当坐标画到画外。
			px, py := 0.0, 0.0
			if c.num(3) == 0 {
				px, py = c.num(4), c.num(5)
			}
			left := offX + px*fit
			top := offY + py*fit
			if c.num(2) == 1 { // origin=1：x/y 是图片中心（立绘、过场贴图常见）
				if dw, dh := picDim(rel); dw > 0 {
					left -= float64(dw) * scale / 2
					top -= float64(dh) * scale / 2
				}
			}
			ev.Show = append(ev.Show, ir.ShowOp{
				TMs: clock, Kind: ir.ShowLayerShow,
				Layer: fmt.Sprintf("layer%d", id),
				Ref:   rel,
				Attr: map[string]string{
					"left":    fmt.Sprintf("%.1f", left),
					"top":     fmt.Sprintf("%.1f", top),
					"scale":   fmt.Sprintf("%.6f", scale),
					"opacity": fmt.Sprintf("%d", int(opacity)),
				},
			})
			visible[id] = true
			picRef[id] = rel
			picOpacity[id] = int(opacity)
		case 232:
			// 移动图片：params = [id, origin, x, y, scaleX, scaleY, opacity, blend, duration, wait]
			id := int(c.num(0))
			rel, ok := picRef[id]
			if !ok {
				continue
			}
			sx := c.num(4)
			sy := c.num(5)
			if sx <= 0 {
				sx = 100
			}
			if sy <= 0 {
				sy = 100
			}
			scale := fit * ((sx + sy) / 2) / 100
			opacity := float64(picOpacity[id])
			if len(c.Params) > 6 {
				opacity = c.num(6)
			}
			px, py := 0.0, 0.0
			if c.num(1) == 0 {
				px, py = c.num(2), c.num(3)
			}
			left := offX + px*fit
			top := offY + py*fit
			if c.num(1) == 1 {
				if dw, dh := picDim(rel); dw > 0 {
					left -= float64(dw) * scale / 2
					top -= float64(dh) * scale / 2
				}
			}
			durMs := int64(c.num(8) * 1000 / 60)
			if durMs < 0 {
				durMs = 0
			}
			attrs := map[string]string{
				"left":    fmt.Sprintf("%.1f", left),
				"top":     fmt.Sprintf("%.1f", top),
				"scale":   fmt.Sprintf("%.6f", scale),
				"opacity": fmt.Sprintf("%d", int(opacity)),
			}
			if durMs > 0 && int(opacity) != picOpacity[id] {
				attrs["from"] = strconv.Itoa(picOpacity[id])
				attrs["to"] = strconv.Itoa(int(opacity))
			}
			ev.Show = append(ev.Show, ir.ShowOp{
				TMs: clock, Kind: ir.ShowLayerAttr, Layer: fmt.Sprintf("layer%d", id),
				DurMs: durMs, Attr: attrs,
			})
			picOpacity[id] = int(opacity)
			clock += durMs
		case 235:
			// 消除图片：params = [id]；id<=0 视为清空全部（部分插件脚本会这么写）。
			id := int(c.num(0))
			if id <= 0 {
				for lid := range visible {
					ev.Show = append(ev.Show, ir.ShowOp{
						TMs: clock, Kind: ir.ShowLayerHide, Layer: fmt.Sprintf("layer%d", lid),
					})
					delete(picRef, lid)
					delete(picOpacity, lid)
				}
				visible = map[int]bool{}
				break
			}
			ev.Show = append(ev.Show, ir.ShowOp{
				TMs: clock, Kind: ir.ShowLayerHide, Layer: fmt.Sprintf("layer%d", id),
			})
			delete(visible, id)
			delete(picRef, id)
			delete(picOpacity, id)
		case 221, 222:
			// 画面淡出/淡入（参数为空，原引擎默认约 24 帧）。回放没有全屏遮罩，
			// 保留一个短等待，避免前后 CG 挤在同一时刻。
			ev.Show = append(ev.Show, ir.ShowOp{TMs: clock, Kind: ir.ShowWait, DurMs: 400})
			clock += 400
		}
	}
	finishMsg()
	if len(ev.Show) == 0 {
		return nil, fmt.Errorf("事件 %s 没有可回放的指令", key)
	}
	ev.Show = append(ev.Show, ir.ShowOp{TMs: clock, Kind: ir.ShowWait, DurMs: 1500})
	ev.EndMs = clock + 1500
	return ev, nil
}

var (
	ctrlI18N    = regexp.MustCompile(`\\I18N\[(\d+)\]`)
	ctrlLocale  = regexp.MustCompile(`\{([A-Za-z][A-Za-z0-9_]*)\}`)
	ctrlCode    = regexp.MustCompile(`\\[A-Za-z][A-Za-z0-9_]*\[[^\]]*\]`)
	ctrlTag     = regexp.MustCompile(`\\[A-Za-z][A-Za-z0-9_]*<[^>]*>`)
	ctrlNewline = regexp.MustCompile(`\\[nN]`)
	ctrlStray   = regexp.MustCompile(`\\([^\x00-\x7F])`)
	ctrlEsc     = regexp.MustCompile(`\\[.\|!><^{}$\\]`)
)

// textCleaner 把台词里的占位符换成能读的文字。
type textCleaner struct {
	i18n   map[int]string
	locale map[string]string
}

func (t textCleaner) clean(s string) string {
	if len(t.locale) > 0 {
		s = ctrlLocale.ReplaceAllStringFunc(s, func(m string) string {
			if v, ok := t.locale[m[1:len(m)-1]]; ok {
				return v
			}
			return m // 不是翻译键就按原文保留
		})
	}
	// 即使翻译表为空也要处理占位符：直接删掉会让整行变空，
	// 上游按 texts==0 把整段演出当成「没有台词」丢掉。
	s = ctrlI18N.ReplaceAllStringFunc(s, func(m string) string {
		id, _ := strconv.Atoi(m[6 : len(m)-1])
		if v, ok := t.i18n[id]; ok && strings.TrimSpace(v) != "" {
			return v
		}
		return fmt.Sprintf("【I18N %d 缺失】", id)
	})
	s = ctrlTag.ReplaceAllString(s, "")
	s = ctrlCode.ReplaceAllString(s, "")
	s = ctrlNewline.ReplaceAllString(s, " ")
	s = ctrlStray.ReplaceAllString(s, "$1")
	s = ctrlEsc.ReplaceAllString(s, "")
	// 汉化文本里用来标"叙述行"的色块，不是台词内容。
	s = strings.ReplaceAll(s, "🟥", "")
	return strings.TrimSpace(s)
}

// CleanText 无翻译表时的清理（\C[3]、\N[1]、\{、\! 等控制码）。
func CleanText(s string) string {
	return textCleaner{}.clean(s)
}

// Flatten 把台词里的换行/连续空白压成单个空格（卡片标题用）。
func Flatten(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// WrapText 按消息窗宽度折行：RM 引擎会自动折行，播放器不会。
// max 取 28 个全角字（1440px 窗宽 / 48px 字号，留出左右边距）。
func WrapText(s string, max int) []string {
	if max <= 0 {
		max = 28
	}
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
