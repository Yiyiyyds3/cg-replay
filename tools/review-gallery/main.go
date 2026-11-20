// Command review-gallery 把「游戏库验收 pass 清单」渲染成一份自包含的人工抽检画廊。
//
// 设计约束（与 reports/ 侧验收口径对齐）：
//   - 只读：不写回仓库任何受版本控制的文件；不生成缩略图、不拷贝原图；
//   - 自包含：单文件 HTML，CSS/JS 内联，不引用任何 CDN 或外网资源；
//   - 零解码：像素尺寸只读文件头（PNG IHDR / JPEG SOFn / WebP VP8*）取，
//     最多读 64KB，不对大图做全量解码；
//   - 清单可覆盖：默认读 reports/人工抽检/pass清单.json，缺失时从
//     reports/audit/** 自动发现，并落一份临时清单到 tools/review-gallery/.cache/。
//
// v1.1 起页面升级成「能带着线索快速过图」的画布：
//   - 顶部「目录导航」：53 款锚点列表（编号 + 游戏名 + CG 数 + 预筛 severity），固定顶部可折叠；
//   - 每款标题条带「来源徽章」：engine 为 unity/spike 且 cg_index_dir 非空 → 导出产物（一级证据），
//     否则 → rmmz 源目录预览（无导出物）；
//   - 每款内嵌「预筛线索条」：读 reports/人工抽检/预筛指标.json 与 预筛指标_rmmz.json（按 name 合并），
//     字段缺失全部容错，severity>=2 的款标题条加 ⚠；
//   - 顶部「筛选按钮」：全部 / 只看导出产物 / 只看 rmmz 源目录 / 只看大款 / 只看预筛⚠，纯前端不刷新。
//
// 用法见 tools/review-gallery/README.md。
package main

import (
	"bufio"
	_ "embed"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed assets/gallery.css
var cssText string

//go:embed assets/gallery.js
var jsText string

const (
	appVersion = "1.1.0"
	// rmmzDefaultMax：自动发现时给 rmmz 款设定的单款抽样上限。
	// rmmz 的 img/pictures 单款可达 3000+ 张（本库 16 款明文合计 15933 张），
	// 全量内联会让 HTML 体积失控，故按等步长抽样，覆盖整个目录。
	rmmzDefaultMax = 200
)

// gameEnt 是清单里的一款游戏。
type gameEnt struct {
	ID           string
	Name         string
	Group        string
	Batch        string
	Engine       string
	CGDir        string
	ImageRoot    string
	ImageSubdirs []string
	MaxImages    int
	Note         string
	// CGIndex 是清单里「真实 cg_index_dir」原文（rmmz 款通常为空）。
	// 它与 CGDir 分开保存：CGDir 会回退到 source_dir，但「有没有导出物」只能看 CGIndex。
	CGIndex string
	// SourceDir 是回放/扫描实际读的源目录（source_real_path 优先），仅用于展示。
	SourceDir string
	// CGCount 是清单给的 CG 数（unity 为索引数、rmmz 为 events 数）；0 表示清单未给。
	CGCount int
}

// listData 是清单文件的顶层结构。
type listData struct {
	Source      string
	GeneratedAt string
	Games       []gameEnt
}

// imgFile 是单张图片。
type imgFile struct {
	Rel   string
	Abs   string
	Bytes int64
	W, H  int
}

// gameScan 是单款游戏的扫描结果。
type gameScan struct {
	E         gameEnt
	Index     int
	DirAbs    string
	Roots     []string
	Found     int
	Shown     int
	Bytes     int64
	ShownByte int64
	Encrypted int
	Status    string
	Problem   string
	Images    []imgFile
}

func main() {
	listPath := flag.String("list", filepath.Join("reports", "人工抽检", "pass清单.json"), "pass 清单 JSON 路径（缺失则自动发现）")
	outPath := flag.String("out", filepath.Join("out", "review", "index.html"), "输出 HTML 路径")
	reportsDir := flag.String("reports", "reports", "验收报告目录（自动发现的数据源）")
	unityRoot := flag.String("unitycg", "D:\\UCGR_Work\\unitycg", "Unity 导出 cg-index 根目录")
	autoList := flag.String("auto-list", filepath.Join("tools", "review-gallery", ".cache", "pass清单.auto.json"), "自动发现时临时清单的落盘路径")
	maxPer := flag.Int("max-per-game", 0, "覆盖单款渲染上限（0 表示按清单 max_images 走）")
	title := flag.String("title", "UCGR 人工抽检画廊", "页面标题")
	prescreenUnity := flag.String("prescreen", filepath.Join("reports", "人工抽检", "预筛指标.json"), "预筛指标 JSON（unity 侧），缺失则跳过")
	prescreenRmmz := flag.String("prescreen-rmmz", filepath.Join("reports", "人工抽检", "预筛指标_rmmz.json"), "预筛指标 JSON（rmmz 侧，可选），与上面按 name 合并")
	quiet := flag.Bool("quiet", false, "只输出最终统计行")
	flag.Parse()

	start := time.Now()
	var ld *listData
	var err error

	if fileExists(*listPath) {
		ld, err = readList(*listPath)
		if err != nil {
			fail("读清单失败 %s: %v", *listPath, err)
		}
		if !*quiet {
			fmt.Printf("[list] 使用清单 %s（%d 款）\n", *listPath, len(ld.Games))
		}
	} else {
		if !*quiet {
			fmt.Printf("[list] %s 不存在，转入自动发现\n", *listPath)
		}
		ld, err = discover(*reportsDir, *unityRoot)
		if err != nil {
			fail("自动发现失败: %v", err)
		}
		ld.GeneratedAt = time.Now().Format(time.RFC3339)
		if err := writeList(*autoList, ld); err != nil {
			fail("写临时清单失败: %v", err)
		}
		if !*quiet {
			fmt.Printf("[list] 自动发现 %d 款 → 临时清单 %s\n", len(ld.Games), *autoList)
		}
	}
	if len(ld.Games) == 0 {
		fail("清单为空，拒绝生成空画廊")
	}

	scans := make([]*gameScan, 0, len(ld.Games))
	for i := range ld.Games {
		e := ld.Games[i]
		limit := e.MaxImages
		if *maxPer > 0 {
			limit = *maxPer
		}
		s := scanGame(e, limit)
		s.Index = i + 1
		scans = append(scans, s)
		if !*quiet && (i+1)%10 == 0 {
			fmt.Printf("[scan] %d/%d ...\n", i+1, len(ld.Games))
		}
	}

	ps := loadPrescreen([]string{*prescreenUnity, *prescreenRmmz})
	views, vst := buildViews(scans, ps)
	if !*quiet {
		fmt.Printf("[prescreen] 加载 %s · 覆盖 %d/%d 款 · 跳过 %s\n",
			orDefault(strings.Join(ps.Sources, " + "), "（无）"), vst.covered, len(views),
			orDefault(strings.Join(ps.Skipped, " / "), "（无）"))
	}

	if err := render(*outPath, *title, ld, views, vst, ps); err != nil {
		fail("渲染失败: %v", err)
	}

	var (
		totalShown, totalFound, encGames, missingGames, truncated int
		totalBytes, shownBytes                                    int64
	)
	for _, s := range scans {
		totalShown += s.Shown
		totalFound += s.Found
		totalBytes += s.Bytes
		shownBytes += s.ShownByte
		if s.Found > s.Shown {
			truncated++
		}
		switch s.Status {
		case "encrypted":
			encGames++
		case "missing_dir":
			missingGames++
		}
	}

	outAbs, _ := filepath.Abs(*outPath)
	var size int64
	if fi, statErr := os.Stat(*outPath); statErr == nil {
		size = fi.Size()
	}
	fmt.Printf("games=%d images_shown=%d images_found=%d bytes_found=%d bytes_shown=%d truncated_games=%d encrypted_games=%d missing_games=%d html_bytes=%d elapsed=%s\n",
		len(scans), totalShown, totalFound, totalBytes, shownBytes, truncated, encGames, missingGames, size,
		time.Since(start).Round(time.Millisecond))
	fmt.Printf("html=%s\n", outAbs)
	if size <= 50*1024 {
		fmt.Fprintf(os.Stderr, "警告：HTML 体积 %d 字节 <= 50KB，请检查清单与图片发现\n", size)
	}
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "review-gallery: "+format+"\n", args...)
	os.Exit(1)
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func isDir(p string) bool {
	if p == "" {
		return false
	}
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func orDefault(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func asList(v any) []any {
	l, _ := v.([]any)
	return l
}

func asStr(m map[string]any, k string) string {
	if m == nil {
		return ""
	}
	s, _ := m[k].(string)
	return s
}

func asInt(m map[string]any, k string) int {
	if m == nil {
		return 0
	}
	switch v := m[k].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
	}
	return 0
}

func asStrs(m map[string]any, k string) []string {
	l := asList(m[k])
	out := make([]string, 0, len(l))
	for _, it := range l {
		if s, ok := it.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}

// ---------------------------------------------------------------- 预筛线索

const (
	// bigCGCount：「只看大款」的阈值（清单 cg_count > 200）。
	bigCGCount = 200
	// prescreenWarnSev：severity 达到它在标题条挂 ⚠。
	prescreenWarnSev = 2
	// 来源徽章文案（README 里的规则与这里一一对应）。
	badgeExport = "导出产物（一级证据）"
	badgeSource = "rmmz 源目录预览（无导出物）"
)

// prescreen 是「预筛指标」两份 JSON 合并后的索引（按 name 合并，后者覆盖同名键）。
type prescreen struct {
	Sources []string                  // 已加载的文件名（按加载顺序）
	Skipped []string                  // 缺失或解析失败的文件（含原因）
	Names   []string                  // 首次出现的顺序，渲染/匹配用
	Games   map[string]map[string]any // name → 预筛记录（原样保留，字段全容错）
	byDir   map[string]string         // 目录 basename（小写）→ name
}

// loadPrescreen 读若干预筛 JSON；缺文件、坏 JSON 一律降级为「跳过」，不中断生成。
func loadPrescreen(paths []string) *prescreen {
	ps := &prescreen{Games: map[string]map[string]any{}, byDir: map[string]string{}}
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if !fileExists(path) {
			ps.Skipped = append(ps.Skipped, filepath.Base(path)+"（不存在）")
			continue
		}
		b, err := os.ReadFile(path)
		if err != nil {
			ps.Skipped = append(ps.Skipped, filepath.Base(path)+"（读取失败："+err.Error()+"）")
			continue
		}
		games, err := parsePrescreen(b)
		if err != nil {
			ps.Skipped = append(ps.Skipped, filepath.Base(path)+"（解析失败："+err.Error()+"）")
			continue
		}
		src := filepath.Base(path)
		ps.Sources = append(ps.Sources, src)
		for _, g := range games {
			name := strings.TrimSpace(asStr(g, "name"))
			if name == "" {
				continue
			}
			g["__src"] = src
			if prev, ok := ps.Games[name]; ok {
				// 同款出现在两份文件里：按字段合并，后来者覆盖同名字段。
				for k, v := range g {
					prev[k] = v
				}
			} else {
				ps.Games[name] = g
				ps.Names = append(ps.Names, name)
			}
			if key := prescreenDirKey(asStr(g, "dir")); key != "" {
				ps.byDir[key] = name
			}
		}
	}
	return ps
}

// parsePrescreen 兼容三种形态：{games:[...]}、裸数组、{name:{...}} 映射。
func parsePrescreen(b []byte) ([]map[string]any, error) {
	var root any
	if err := json.Unmarshal(b, &root); err != nil {
		return nil, err
	}
	collect := func(list []any) []map[string]any {
		out := make([]map[string]any, 0, len(list))
		for _, it := range list {
			if m := asMap(it); m != nil {
				out = append(out, m)
			}
		}
		return out
	}
	switch v := root.(type) {
	case []any:
		return collect(v), nil
	case map[string]any:
		if list := asList(v["games"]); len(list) > 0 {
			return collect(list), nil
		}
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make([]map[string]any, 0, len(keys))
		for _, k := range keys {
			m := asMap(v[k])
			if m == nil {
				continue
			}
			if strings.TrimSpace(asStr(m, "name")) == "" {
				m["name"] = k
			}
			out = append(out, m)
		}
		if len(out) > 0 {
			return out, nil
		}
	}
	return nil, fmt.Errorf("预筛 JSON 必须是数组、含 games 数组的对象，或 name→对象 的映射")
}

// prescreenDirKey 把预筛记录里的 dir 归一成目录名小写键：
// D:\UCGR_Work\unitycg\IdealEvesRoom\cg → idealevesroom。
func prescreenDirKey(dir string) string {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return ""
	}
	clean := filepath.Clean(dir)
	base := filepath.Base(clean)
	if strings.EqualFold(base, "cg") {
		if up := filepath.Base(filepath.Dir(clean)); up != "" && up != "." && up != string(os.PathSeparator) {
			return strings.ToLower(up)
		}
	}
	return strings.ToLower(base)
}

// match 给一款游戏找预筛记录，四级回退：同名 → 忽略大小写同名 → 目录名 → 最长子串。
func (ps *prescreen) match(e gameEnt) (map[string]any, bool) {
	if ps == nil || len(ps.Games) == 0 {
		return nil, false
	}
	name := strings.TrimSpace(e.Name)
	if name != "" {
		if m, ok := ps.Games[name]; ok {
			return m, true
		}
		lower := strings.ToLower(name)
		for _, n := range ps.Names {
			if strings.ToLower(n) == lower {
				return ps.Games[n], true
			}
		}
	}
	for _, d := range []string{e.CGIndex, e.CGDir} {
		if key := prescreenDirKey(d); key != "" {
			if n, ok := ps.byDir[key]; ok {
				return ps.Games[n], true
			}
		}
	}
	lower := strings.ToLower(name)
	best, bestLen := "", 0
	for _, n := range ps.Names {
		k := strings.ToLower(n)
		if len([]rune(k)) < 4 || !strings.Contains(lower, k) {
			continue
		}
		if len([]rune(k)) > bestLen {
			best, bestLen = n, len([]rune(k))
		}
	}
	if best != "" {
		return ps.Games[best], true
	}
	return nil, false
}

// asFloatOK 取浮点字段，兼容 JSON 数字与数字字符串；缺失返回 false。
func asFloatOK(m map[string]any, k string) (float64, bool) {
	if m == nil {
		return 0, false
	}
	switch v := m[k].(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	case string:
		if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			return f, true
		}
	}
	return 0, false
}

// asIntOK 取整数字段（允许 3.0 或 "3"）；缺失返回 false。
func asIntOK(m map[string]any, k string) (int, bool) {
	f, ok := asFloatOK(m, k)
	if !ok {
		return 0, false
	}
	return int(f), true
}

// asStrOK 取字符串字段（允许数字转字符串）；空串算缺失。
func asStrOK(m map[string]any, k string) (string, bool) {
	if m == nil {
		return "", false
	}
	switch v := m[k].(type) {
	case string:
		if s := strings.TrimSpace(v); s != "" {
			return v, true
		}
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), true
	}
	return "", false
}

// ---------------------------------------------------------------- 渲染视图

// scanView 是一款 section 的渲染视图：扫描结果 + 来源徽章 + 预筛线索。
type scanView struct {
	S       *gameScan
	Export  bool // true=导出产物；false=源目录预览
	CGCount int  // CG 数（清单 cg_count 优先，缺失回退扫描发现数）
	Sev     int  // 预筛 severity；-1 表示预筛未收录本款
	Warn    bool // Sev >= prescreenWarnSev
	M       map[string]any
}

// viewStats 是顶部筛选按钮的计数。
type viewStats struct{ total, export, source, big, sev2, covered int }

// buildViews 把扫描结果与预筛记录合成渲染视图，并算出筛选按钮的计数。
func buildViews(scans []*gameScan, ps *prescreen) ([]*scanView, viewStats) {
	views := make([]*scanView, 0, len(scans))
	var st viewStats
	for _, s := range scans {
		v := &scanView{
			S:       s,
			Export:  isUnityLike(s.E.Engine) && strings.TrimSpace(s.E.CGIndex) != "",
			CGCount: s.E.CGCount,
			Sev:     -1,
		}
		if v.CGCount <= 0 {
			v.CGCount = s.Found
		}
		if m, ok := ps.match(s.E); ok {
			v.M = m
			st.covered++
			if n, ok := asIntOK(m, "severity"); ok {
				v.Sev = n
			}
		}
		v.Warn = v.Sev >= prescreenWarnSev
		st.total++
		if v.Export {
			st.export++
		} else {
			st.source++
		}
		if v.CGCount > bigCGCount {
			st.big++
		}
		if v.Warn {
			st.sev2++
		}
		views = append(views, v)
	}
	return views, st
}

// sevClass 把 severity 映射成 CSS 后缀：无记录 none，其余 s<数字>。
func sevClass(sev int) string {
	if sev < 0 {
		return "none"
	}
	if sev > 5 {
		sev = 5
	}
	return "s" + strconv.Itoa(sev)
}

// ---------------------------------------------------------------- 清单读写

// readList 读清单，兼容两种形态：
//
//	{ "games": [ ... ] } —— 本工具 -auto-list 产出的形态
//	[ { ... }, { ... } ] —— reports/人工抽检/pass清单.json 的形态（裸数组）
//
// 单款目录字段按别名取值：cg_dir ← cg_dir / cg_index_dir / source_real_path / source_dir。
func readList(p string) (*listData, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var root any
	if err := json.Unmarshal(b, &root); err != nil {
		return nil, err
	}
	ld := &listData{}
	var items []any
	switch v := root.(type) {
	case []any:
		items = v
		ld.Source = "清单（裸数组）：" + filepath.ToSlash(p)
	case map[string]any:
		ld.Source = asStr(v, "source")
		ld.GeneratedAt = asStr(v, "generated_at")
		items = asList(v["games"])
	default:
		return nil, fmt.Errorf("清单必须是 JSON 数组，或含 games 数组的对象")
	}
	if ld.Source == "" {
		ld.Source = "清单：" + filepath.ToSlash(p)
	}
	used := map[string]bool{}
	for _, it := range items {
		m := asMap(it)
		if m == nil {
			continue
		}
		e := normalizeEntry(m)
		if e.CGDir == "" {
			continue
		}
		if e.ID == "" || used[e.ID] {
			e.ID = fmt.Sprintf("g%03d", len(ld.Games)+1)
		}
		used[e.ID] = true
		ld.Games = append(ld.Games, e)
	}
	if len(ld.Games) == 0 {
		return nil, fmt.Errorf("清单里没有可用条目（至少需要 name 与 cg_dir / cg_index_dir / source_dir 之一）")
	}
	return ld, nil
}

// normalizeEntry 把一条清单记录归一成 gameEnt，兼容字段别名。
func normalizeEntry(m map[string]any) gameEnt {
	e := gameEnt{
		ID:           asID(m),
		Name:         asStr(m, "name"),
		Group:        asStr(m, "group"),
		Batch:        asStr(m, "batch"),
		Engine:       strings.ToLower(strings.TrimSpace(asStr(m, "engine"))),
		CGDir:        firstNonEmpty(asStr(m, "cg_dir"), asStr(m, "cg_index_dir"), asStr(m, "source_real_path"), asStr(m, "source_dir")),
		ImageRoot:    asStr(m, "image_root"),
		ImageSubdirs: asStrs(m, "image_subdirs"),
		MaxImages:    asInt(m, "max_images"),
		Note:         firstNonEmpty(asStr(m, "note"), asStr(m, "cg_index_note")),
		CGIndex:      strings.TrimSpace(asStr(m, "cg_index_dir")),
		SourceDir:    firstNonEmpty(asStr(m, "source_real_path"), asStr(m, "source_dir")),
		CGCount:      asInt(m, "cg_count"),
	}
	if e.Name == "" && e.CGDir != "" {
		e.Name = filepath.Base(e.CGDir)
	}
	if e.ImageRoot == "" && isDir(e.CGDir) && !isUnityLike(e.Engine) {
		e.ImageRoot = findPicturesDir(e.CGDir, 5)
	}
	if e.MaxImages == 0 && strings.EqualFold(e.Engine, "rmmz") {
		e.MaxImages = rmmzDefaultMax
	}
	return e
}

// isUnityLike 判断是否是「导出目录」型引擎（cg 索引库形态）。
func isUnityLike(engine string) bool {
	return strings.EqualFold(engine, "unity") || strings.EqualFold(engine, "spike")
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// asID 取条目 id（字符串或数字），产出稳定 id。
func asID(m map[string]any) string {
	switch v := m["id"].(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		return fmt.Sprintf("g%03d", int(v))
	}
	return ""
}

func writeList(p string, ld *listData) error {
	games := make([]any, 0, len(ld.Games))
	for _, e := range ld.Games {
		m := map[string]any{
			"id":     e.ID,
			"name":   e.Name,
			"group":  e.Group,
			"batch":  e.Batch,
			"engine": e.Engine,
			"cg_dir": e.CGDir,
		}
		if e.ImageRoot != "" {
			m["image_root"] = e.ImageRoot
		}
		if len(e.ImageSubdirs) > 0 {
			m["image_subdirs"] = e.ImageSubdirs
		}
		if e.MaxImages > 0 {
			m["max_images"] = e.MaxImages
		}
		if e.Note != "" {
			m["note"] = e.Note
		}
		if e.CGIndex != "" {
			m["cg_index_dir"] = e.CGIndex
		}
		if e.SourceDir != "" {
			m["source_dir"] = e.SourceDir
		}
		if e.CGCount > 0 {
			m["cg_count"] = e.CGCount
		}
		games = append(games, m)
	}
	root := map[string]any{
		"schema":       1,
		"generated_at": ld.GeneratedAt,
		"source":       ld.Source,
		"games":        games,
	}
	b, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if dir := filepath.Dir(p); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(p, b, 0o644)
}

// ---------------------------------------------------------------- 自动发现

// discover 从 reports/audit/** 重建 pass 清单。
//
//	rmmz ：batch-1..7.json 与 batch-rmmz-recheck.json 里 verdict=pass 的款，
//	       同名款后者覆盖前者（复扫口径）；图片目录取 games 的 img/pictures。
//	unity：unity-batch1..4/summary.json 里 verdict=pass 的款，
//	       按 short 名解析 D:\UCGR_Work\unitycg\<dir>。
//
// 该库当前应得 53 款（rmmz 37 + unity 16）。
func discover(reportsDir, unityRoot string) (*listData, error) {
	auditDir := filepath.Join(reportsDir, "audit")
	if !isDir(auditDir) {
		return nil, fmt.Errorf("审计目录不存在: %s", auditDir)
	}
	ld := &listData{
		Source: "auto-discover: reports/audit/batch-1..7.json + batch-rmmz-recheck.json + unity-batch1..4/summary.json",
	}
	seen := map[string]int{}

	for _, batch := range []string{"batch-1", "batch-2", "batch-3", "batch-4", "batch-5", "batch-6", "batch-7", "batch-rmmz-recheck"} {
		p := filepath.Join(auditDir, batch+".json")
		if !fileExists(p) {
			continue
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		var recs []map[string]any
		if err := json.Unmarshal(b, &recs); err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		for _, r := range recs {
			if !strings.EqualFold(asStr(r, "verdict"), "pass") {
				continue
			}
			gameRoot := strings.TrimSpace(asStr(r, "real_path"))
			if gameRoot == "" || !isDir(gameRoot) {
				gameRoot = strings.TrimSpace(asStr(r, "path"))
			}
			e := gameEnt{
				Name:      asStr(r, "game"),
				Group:     asStr(r, "group"),
				Batch:     batch,
				Engine:    orDefault(asStr(r, "engine_resolved"), "rmmz"),
				CGDir:     gameRoot,
				MaxImages: rmmzDefaultMax,
			}
			if pic := findPicturesDir(gameRoot, 5); pic != "" {
				e.ImageRoot = pic
			}
			if idx, ok := seen[e.Name]; ok {
				ld.Games[idx] = e
				continue
			}
			seen[e.Name] = len(ld.Games)
			ld.Games = append(ld.Games, e)
		}
	}

	summaries, _ := filepath.Glob(filepath.Join(auditDir, "unity-batch*", "summary.json"))
	sort.Strings(summaries)
	for _, sp := range summaries {
		b, err := os.ReadFile(sp)
		if err != nil {
			return nil, err
		}
		var sum map[string]any
		if err := json.Unmarshal(b, &sum); err != nil {
			return nil, fmt.Errorf("%s: %w", sp, err)
		}
		batch := orDefault(asStr(sum, "batch"), filepath.Base(filepath.Dir(sp)))
		for _, it := range asList(sum["games"]) {
			g := asMap(it)
			if !strings.EqualFold(asStr(g, "verdict"), "pass") {
				continue
			}
			e := gameEnt{
				Name:   asStr(g, "game"),
				Group:  asStr(g, "group"),
				Batch:  batch,
				Engine: "unity",
				CGDir:  resolveUnityDir(unityRoot, asStr(g, "short"), asStr(g, "game")),
			}
			if idx, ok := seen[e.Name]; ok {
				ld.Games[idx] = e
				continue
			}
			seen[e.Name] = len(ld.Games)
			ld.Games = append(ld.Games, e)
		}
	}

	for i := range ld.Games {
		ld.Games[i].ID = fmt.Sprintf("g%03d", i+1)
	}
	if len(ld.Games) == 0 {
		return nil, fmt.Errorf("reports/audit 下没有发现任何 pass 款")
	}
	return ld, nil
}

// resolveUnityDir 把 summary 的 short 名解析成 <unityRoot>\<dir>。
// 个别款 short 与目录名不一致（OneSixNine → 1692614），故两级回退：
// 先试同名目录；否则在根目录里找「目录名出现在游戏名里」的最长匹配。
func resolveUnityDir(root, short, game string) string {
	if short != "" {
		p := filepath.Join(root, short)
		if isDir(p) {
			return p
		}
	}
	ents, err := os.ReadDir(root)
	if err != nil {
		if short != "" {
			return filepath.Join(root, short)
		}
		return ""
	}
	lg := strings.ToLower(game)
	best, bestLen := "", 0
	for _, de := range ents {
		if !de.IsDir() {
			continue
		}
		n := strings.ToLower(de.Name())
		if len(n) < 4 {
			continue
		}
		if strings.Contains(lg, n) && len(n) > bestLen {
			best, bestLen = de.Name(), len(n)
		}
	}
	if best != "" {
		return filepath.Join(root, best)
	}
	if short != "" {
		return filepath.Join(root, short)
	}
	return ""
}

// findPicturesDir 在 root 下（限深）找 rmmz 的 CG 目录：优先 <img>/pictures，
// 退而求其次 <img>/cg 与 <img>/cgset。找不到返回空串（交给扫描阶段兜底探测）。
func findPicturesDir(root string, maxDepth int) string {
	if !isDir(root) {
		return ""
	}
	var pics, others []string
	baseDepth := strings.Count(filepath.Clean(root), string(os.PathSeparator))
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		if strings.Count(filepath.Clean(p), string(os.PathSeparator))-baseDepth > maxDepth {
			return filepath.SkipDir
		}
		name := strings.ToLower(d.Name())
		parent := strings.ToLower(filepath.Base(filepath.Dir(p)))
		if parent != "img" {
			return nil
		}
		switch name {
		case "pictures":
			pics = append(pics, p)
		case "cg", "cgset":
			others = append(others, p)
		}
		return nil
	})
	pick := func(list []string) string {
		if len(list) == 0 {
			return ""
		}
		sort.Slice(list, func(i, j int) bool {
			di := strings.Count(list[i], string(os.PathSeparator))
			dj := strings.Count(list[j], string(os.PathSeparator))
			if di != dj {
				return di < dj
			}
			return list[i] < list[j]
		})
		return list[0]
	}
	if p := pick(pics); p != "" {
		return p
	}
	return pick(others)
}

// ---------------------------------------------------------------- 扫描

var (
	plainExt = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".webp": true}
	cryptExt = map[string]bool{".png_": true, ".jpg_": true, ".rpgmvp": true, ".rpgmvo": true, ".rpgmva": true}
)

// resolveRoots 决定这款游戏从哪些目录收集图片。
func resolveRoots(e gameEnt) []string {
	if e.ImageRoot != "" {
		return []string{e.ImageRoot}
	}
	if len(e.ImageSubdirs) > 0 {
		out := make([]string, 0, len(e.ImageSubdirs))
		for _, sub := range e.ImageSubdirs {
			out = append(out, filepath.Join(e.CGDir, filepath.FromSlash(sub)))
		}
		return out
	}
	// 导出型目录：<cg-index 目录>cg（unity / spike 与任何带 cg 子目录的索引库）
	if cg := filepath.Join(e.CGDir, "cg"); isDir(cg) {
		return []string{cg}
	}
	if isUnityLike(e.Engine) {
		return []string{e.CGDir}
	}
	for _, c := range []string{
		filepath.Join(e.CGDir, "img", "pictures"),
		filepath.Join(e.CGDir, "www", "img", "pictures"),
		filepath.Join(e.CGDir, "img", "cg"),
		filepath.Join(e.CGDir, "img"),
		filepath.Join(e.CGDir, "www", "img"),
	} {
		if isDir(c) {
			return []string{c}
		}
	}
	return []string{e.CGDir}
}

func scanGame(e gameEnt, limit int) *gameScan {
	s := &gameScan{E: e, DirAbs: e.CGDir, Status: "ok"}
	if !isDir(e.CGDir) {
		s.Status = "missing_dir"
		s.Problem = "目录不存在：" + e.CGDir
		return s
	}
	roots := resolveRoots(e)
	s.Roots = roots

	var all []imgFile
	for _, root := range roots {
		if !isDir(root) {
			continue
		}
		_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(p))
			if cryptExt[ext] {
				s.Encrypted++
				return nil
			}
			if !plainExt[ext] {
				return nil
			}
			info, ierr := d.Info()
			if ierr != nil || info.Size() == 0 {
				return nil
			}
			rel, rerr := filepath.Rel(e.CGDir, p)
			if rerr != nil || strings.HasPrefix(rel, "..") {
				rel = filepath.Base(p)
			}
			w, h := probeSize(p)
			all = append(all, imgFile{
				Rel:   filepath.ToSlash(rel),
				Abs:   p,
				Bytes: info.Size(),
				W:     w,
				H:     h,
			})
			return nil
		})
	}

	sort.Slice(all, func(i, j int) bool { return all[i].Rel < all[j].Rel })
	s.Found = len(all)
	for _, f := range all {
		s.Bytes += f.Bytes
	}
	if s.Found == 0 {
		if s.Encrypted > 0 {
			s.Status = "encrypted"
			s.Problem = fmt.Sprintf("目录内只有 RPG Maker 加密图（.png_/.rpgmvp 等 %d 个），浏览器无法直接显示", s.Encrypted)
		} else {
			s.Status = "no_images"
			s.Problem = "目录内没有 png/jpg/webp 图片"
		}
		return s
	}

	shown := all
	if limit > 0 && len(all) > limit {
		shown = sampleEvenly(all, limit)
	}
	s.Images = shown
	s.Shown = len(shown)
	for _, f := range shown {
		s.ShownByte += f.Bytes
	}
	return s
}

// sampleEvenly 按等步长抽样，保证覆盖整个目录而不是只取前 N 张。
func sampleEvenly(in []imgFile, n int) []imgFile {
	out := make([]imgFile, 0, n)
	step := float64(len(in)) / float64(n)
	for i := 0; i < n; i++ {
		idx := int(float64(i) * step)
		if idx >= len(in) {
			idx = len(in) - 1
		}
		out = append(out, in[idx])
	}
	return out
}

// ---------------------------------------------------------------- 文件头取尺寸

// probeSize 只读文件头 64KB 解析像素尺寸，不做全量解码。
func probeSize(path string) (int, int) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	buf := make([]byte, 64*1024)
	n, _ := io.ReadFull(f, buf)
	b := buf[:n]
	if len(b) < 16 {
		return 0, 0
	}
	switch {
	case len(b) >= 24 && b[0] == 0x89 && b[1] == 'P' && b[2] == 'N' && b[3] == 'G':
		return int(binary.BigEndian.Uint32(b[16:20])), int(binary.BigEndian.Uint32(b[20:24]))
	case b[0] == 0xFF && b[1] == 0xD8:
		return jpegSize(b)
	case len(b) >= 30 && string(b[0:4]) == "RIFF" && string(b[8:12]) == "WEBP":
		return webpSize(b)
	}
	return 0, 0
}

func jpegSize(b []byte) (int, int) {
	i := 2
	for i+9 < len(b) {
		if b[i] != 0xFF {
			i++
			continue
		}
		m := b[i+1]
		if m == 0xFF {
			i++
			continue
		}
		if m == 0xD8 || m == 0x01 || (m >= 0xD0 && m <= 0xD7) {
			i += 2
			continue
		}
		if m == 0xD9 || m == 0xDA {
			return 0, 0
		}
		seg := int(binary.BigEndian.Uint16(b[i+2 : i+4]))
		if seg < 2 {
			return 0, 0
		}
		if m >= 0xC0 && m <= 0xCF && m != 0xC4 && m != 0xC8 && m != 0xCC {
			h := int(binary.BigEndian.Uint16(b[i+5 : i+7]))
			w := int(binary.BigEndian.Uint16(b[i+7 : i+9]))
			return w, h
		}
		i += 2 + seg
	}
	return 0, 0
}

func webpSize(b []byte) (int, int) {
	switch string(b[12:16]) {
	case "VP8 ":
		if b[23] != 0x9D || b[24] != 0x01 || b[25] != 0x2A {
			return 0, 0
		}
		w := int(binary.LittleEndian.Uint16(b[26:28])) & 0x3FFF
		h := int(binary.LittleEndian.Uint16(b[28:30])) & 0x3FFF
		return w, h
	case "VP8L":
		if b[20] != 0x2F {
			return 0, 0
		}
		v := binary.LittleEndian.Uint32(b[21:25])
		return int(v&0x3FFF) + 1, int((v>>14)&0x3FFF) + 1
	case "VP8X":
		w := int(b[24]) | int(b[25])<<8 | int(b[26])<<16
		h := int(b[27]) | int(b[28])<<8 | int(b[29])<<16
		return w + 1, h + 1
	}
	return 0, 0
}

// ---------------------------------------------------------------- 渲染辅助

// fileURL 把 Windows 绝对路径转成可被 img src 用的 file:// URL（按字节百分号编码）。
func fileURL(abs string) string {
	p := strings.ReplaceAll(abs, "\\", "/")
	var sb strings.Builder
	sb.Grow(len(p)*2 + 8)
	sb.WriteString("file:///")
	for i := 0; i < len(p); i++ {
		c := p[i]
		if c < 0x80 && (c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' ||
			c == '-' || c == '.' || c == '_' || c == '~' || c == '/' || c == ':') {
			sb.WriteByte(c)
			continue
		}
		fmt.Fprintf(&sb, "%%%02X", c)
	}
	return sb.String()
}

func humanBytes(n int64) string {
	if n <= 0 {
		return "0 B"
	}
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	units := []string{"KB", "MB", "GB", "TB"}
	f := float64(n) / 1024
	i := 0
	for f >= 1024 && i < len(units)-1 {
		f /= 1024
		i++
	}
	return fmt.Sprintf("%.1f %s", f, units[i])
}

func comma(n int) string {
	s := strconv.Itoa(n)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	var out []byte
	for i := 0; i < len(s); i++ {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, s[i])
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}

func esc(s string) string { return html.EscapeString(s) }

func dims(im imgFile) string {
	if im.W > 0 && im.H > 0 {
		return fmt.Sprintf("%d×%d · %s", im.W, im.H, humanBytes(im.Bytes))
	}
	return "尺寸未知 · " + humanBytes(im.Bytes)
}

// ---------------------------------------------------------------- 渲染

func render(outPath, title string, ld *listData, views []*scanView, st viewStats, ps *prescreen) error {
	if dir := filepath.Dir(outPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriterSize(f, 1<<20)
	defer w.Flush()

	var (
		totalShown, totalFound, truncated, encGames, missingGames int
		totalBytes                                                int64
	)
	for _, v := range views {
		s := v.S
		totalShown += s.Shown
		totalFound += s.Found
		totalBytes += s.Bytes
		if s.Found > s.Shown {
			truncated++
		}
		switch s.Status {
		case "encrypted":
			encGames++
		case "missing_dir":
			missingGames++
		}
	}
	genAt := time.Now().Format("2006-01-02 15:04:05")
	p := func(format string, a ...any) { fmt.Fprintf(w, format, a...) }

	p("<!doctype html>\n<html lang=\"zh-CN\">\n<head>\n<meta charset=\"utf-8\">\n")
	p("<meta name=\"viewport\" content=\"width=device-width,initial-scale=1\">\n")
	p("<title>%s</title>\n", esc(title))
	p("<style>\n%s\n</style>\n</head>\n<body>\n", cssText)

	// ---------- 顶部汇总条 ----------
	p("<header class=\"topbar\">\n")
	p("  <div class=\"tbrow\">\n")
	p("    <h1>%s<span class=\"sub\">pass %d 款 · 生成于 %s · v%s</span></h1>\n", esc(title), len(views), esc(genAt), appVersion)
	p("    <div class=\"acts\">\n")
	p("      <button id=\"btn-csv\" type=\"button\">导出 CSV</button>\n")
	p("      <button id=\"btn-filter\" type=\"button\">只看未判定</button>\n")
	p("      <button id=\"btn-clear\" type=\"button\" class=\"danger\">清空本机判定</button>\n")
	p("    </div>\n")
	p("  </div>\n")
	p("  <div class=\"stats\">\n")
	p("    <div class=\"stat\"><b>%d</b><span>游戏数</span></div>\n", len(views))
	p("    <div class=\"stat\"><b>%s</b><span>总图数（目录内发现 %s）</span></div>\n", comma(totalShown), comma(totalFound))
	p("    <div class=\"stat\"><b>%s</b><span>总体积</span></div>\n", esc(humanBytes(totalBytes)))
	p("    <div class=\"stat\"><b>%d/%d</b><span>预筛线索覆盖</span></div>\n", st.covered, len(views))
	p("    <div class=\"stat\"><b id=\"st-done\">0/%d</b><span>已判定</span></div>\n", len(views))
	p("    <div class=\"stat\"><b id=\"st-cur\">#1</b><span>当前游戏</span></div>\n")
	p("  </div>\n")
	p("  <div class=\"prog\"><i id=\"progbar\"></i></div>\n")
	p("  <div class=\"hint\"><kbd>&larr;</kbd><kbd>&rarr;</kbd> 切游戏 · <kbd>J</kbd><kbd>K</kbd> 滚动 · <kbd>1</kbd> 好 <kbd>2</kbd> 小问题 <kbd>3</kbd> 严重 <kbd>4</kbd> 待查 · 判定自动存 localStorage（键 <code>ucgrReviewGallery.v1</code>）</div>\n")
	p("  <div class=\"hint warn\" id=\"storage-warn\" hidden>本页面读不到 localStorage（file:// 限制）：判定只存在内存里，请用「导出 CSV」保存结果。</div>\n")
	// ---------- 筛选条（纯前端：只改可见性，不刷新、不写判定） ----------
	p("  <div class=\"fbar\" id=\"fbar\" role=\"group\" aria-label=\"筛选\">\n")
	p("    <span class=\"flabel\">筛选</span>\n")
	p("    <button type=\"button\" class=\"fbtn on\" data-f=\"all\" title=\"不筛选，显示全部款\">全部 <b>%d</b></button>\n", st.total)
	p("    <button type=\"button\" class=\"fbtn\" data-f=\"export\" title=\"engine=unity/spike 且 cg_index_dir 非空\">只看导出产物 <b>%d</b></button>\n", st.export)
	p("    <button type=\"button\" class=\"fbtn\" data-f=\"source\" title=\"engine=rmmz 或 cg_index_dir 为空\">只看 rmmz 源目录 <b>%d</b></button>\n", st.source)
	p("    <button type=\"button\" class=\"fbtn\" data-f=\"big\" title=\"清单 cg_count > 200（缺失时回退扫描发现数）\">只看大款 cg_count&gt;%d <b>%d</b></button>\n", bigCGCount, st.big)
	p("    <button type=\"button\" class=\"fbtn\" data-f=\"sev2\" title=\"预筛 severity >= 2\">只看预筛⚠ severity≥%d <b>%d</b></button>\n", prescreenWarnSev, st.sev2)
	p("    <span class=\"fcount\">当前可见 <b id=\"st-visible\">%d</b>/%d 款</span>\n", len(views), len(views))
	p("  </div>\n")

	// ---------- 目录导航（贴在顶部、可折叠） ----------
	p("  <nav class=\"toc\" id=\"toc\">\n")
	p("    <div class=\"tocbar\">\n")
	p("      <button type=\"button\" class=\"toctoggle\" id=\"toc-toggle\" aria-expanded=\"true\" aria-controls=\"toc-panel\">目录导航 · %d 款<i>▾</i></button>\n", len(views))
	p("      <span class=\"tochint\">编号 + 游戏名 + CG数 + 预筛 severity；点一下跳过去，滚动后自动收起</span>\n")
	p("    </div>\n")
	p("    <div class=\"tocpanel\" id=\"toc-panel\">\n")
	for _, v := range views {
		tocCls := ""
		if v.Export {
			tocCls = " exp"
		}
		p("      <a class=\"tocitem%s\" href=\"#s%d\" data-target=\"s%d\"><i class=\"tno\">%02d</i><span class=\"tname\">%s</span><span class=\"tcg\">%s 图</span><span class=\"tsev %s\">%s</span></a>\n",
			tocCls, v.S.Index, v.S.Index, v.S.Index, esc(v.S.E.Name), comma(v.CGCount), sevClass(v.Sev), esc(sevLabel(v.Sev, v.Warn)))
	}
	p("    </div>\n")
	p("  </nav>\n")
	p("</header>\n")

	// ---------- 跳转导航 ----------
	p("<nav class=\"jump\">\n<span class=\"jumplabel\">跳转</span>\n")
	for _, v := range views {
		p("<button type=\"button\" class=\"jchip\" data-idx=\"%d\" title=\"%s\">%02d</button>\n", v.S.Index-1, esc(v.S.E.Name), v.S.Index)
	}
	p("</nav>\n")

	p("<main>\n")
	for _, v := range views {
		renderGame(w, v)
	}
	p("</main>\n")

	p("<footer class=\"foot\">\n")
	p("  <p>清单来源：%s</p>\n", esc(orDefault(ld.Source, "-")))
	p("  <p>图片计数：页面渲染 %s 张 / 目录内发现 %s 张（%d 款按单款上限抽样）；加密或目录缺失 %d 款。</p>\n",
		comma(totalShown), comma(totalFound), truncated, encGames+missingGames)
	p("  <p>本页不生成缩略图、不拷贝原图：所有 img 直挂原始绝对路径（file://），能否显示取决于本机该路径是否可读。</p>\n")
	p("  <p>预筛线索：%s；覆盖 %d/%d 款（字段缺失显示 n/a，不猜值、不参与打分）；severity≥%d 的款标题条挂 ⚠。</p>\n",
		esc(prescreenDesc(ps)), st.covered, len(views), prescreenWarnSev)
	p("  <p>来源徽章：「%s」%d 款（engine=unity/spike 且 cg_index_dir 非空）；「%s」%d 款（engine=rmmz 或 cg_index_dir 为空，回放直接读源目录）。</p>\n",
		esc(badgeExport), st.export, esc(badgeSource), st.source)
	p("  <p>顶部筛选按钮只改本页可见性（不刷新、不动判定）；「已判定」计数与 CSV 导出始终按全部 %d 款。</p>\n", len(views))
	p("</footer>\n")
	p("<script>\n%s\n</script>\n</body>\n</html>\n", jsText)
	return w.Flush()
}

func renderGame(w io.Writer, v *scanView) {
	p := func(format string, a ...any) { fmt.Fprintf(w, format, a...) }
	s := v.S
	e := s.E

	statusClass, statusText := "ok", "正常"
	switch s.Status {
	case "encrypted":
		statusClass, statusText = "bad", "加密图不可渲染"
	case "missing_dir":
		statusClass, statusText = "bad", "目录缺失"
	case "no_images":
		statusClass, statusText = "warn", "无明文图片"
	}

	srcKind := "source"
	if v.Export {
		srcKind = "export"
	}
	sevAttr := ""
	if v.Sev >= 0 {
		sevAttr = strconv.Itoa(v.Sev)
	}

	p("<section class=\"game\" id=\"s%d\" data-gid=\"%s\" data-name=\"%s\" data-group=\"%s\" data-batch=\"%s\" data-engine=\"%s\" data-cgdir=\"%s\" data-status=\"%s\" data-found=\"%d\" data-shown=\"%d\" data-bytes=\"%d\" data-enc=\"%d\" data-src=\"%s\" data-cg=\"%d\" data-sev=\"%s\">\n",
		s.Index, esc(e.ID), esc(e.Name), esc(e.Group), esc(e.Batch), esc(e.Engine), esc(e.CGDir), esc(s.Status),
		s.Found, s.Shown, s.Bytes, s.Encrypted, srcKind, v.CGCount, sevAttr)

	p("  <div class=\"ghead\">\n    <div class=\"gtitle\">\n")
	p("      <span class=\"gidx\">%02d</span>\n", s.Index)
	p("      <h2>%s</h2>\n", esc(e.Name))
	if v.Warn {
		p("      <span class=\"chip sevwarn\" title=\"%s\">⚠ 预筛 severity %d</span>\n", esc(v.reasonTip()), v.Sev)
	}
	if v.Export {
		p("      <span class=\"srcbadge exp\" title=\"%s\">%s</span>\n",
			esc("engine="+orDefault(e.Engine, "?")+" · cg_index_dir="+e.CGIndex+"（真实导出目录，一级证据）"), badgeExport)
	} else {
		why := "engine=" + orDefault(e.Engine, "?") + " · cg_index_dir 为空"
		if !isUnityLike(e.Engine) {
			why = "engine=" + orDefault(e.Engine, "?") + "（rmmz 无独立导出 CG 索引库，cg_index_dir 为空）"
		}
		p("      <span class=\"srcbadge src\" title=\"%s\">%s</span>\n",
			esc(why+"；回放直接读源目录 "+orDefault(e.SourceDir, e.CGDir)), badgeSource)
	}
	if strings.TrimSpace(e.Group) != "" {
		p("      <span class=\"chip\">组 %s</span>\n", esc(e.Group))
	}
	if strings.TrimSpace(e.Batch) != "" {
		p("      <span class=\"chip\">批次 %s</span>\n", esc(e.Batch))
	}
	if strings.TrimSpace(e.Engine) != "" {
		p("      <span class=\"chip\">%s</span>\n", esc(e.Engine))
	}
	p("      <span class=\"chip %s\">%s 图 · %s</span>\n", statusClass, comma(s.Found), esc(humanBytes(s.Bytes)))
	if v.CGCount > 0 && v.CGCount != s.Found {
		p("      <span class=\"chip\" title=\"清单 cg_count；「只看大款」按它判定\">清单 CG %s</span>\n", comma(v.CGCount))
	}
	if s.Found > s.Shown {
		p("      <span class=\"chip warn\">显示 %s/%s</span>\n", comma(s.Shown), comma(s.Found))
	}
	if s.Encrypted > 0 {
		p("      <span class=\"chip bad\">加密图 %s</span>\n", comma(s.Encrypted))
	}
	p("      <span class=\"chip\">%s</span>\n", esc(statusText))
	p("      <span class=\"vchip\" data-v=\"\">未判定</span>\n")
	p("    </div>\n    <div class=\"gjudge\">\n")
	p("      <label>结论<select class=\"vsel\"><option value=\"\">未判定</option><option value=\"good\">好</option><option value=\"minor\">小问题</option><option value=\"severe\">严重</option><option value=\"todo\">待查</option></select></label>\n")
	p("      <input class=\"vnote\" type=\"text\" placeholder=\"备注（可空，自动保存）\">\n")
	p("    </div>\n  </div>\n")

	renderClues(w, v)

	plabel := "cg-index 目录"
	if !v.Export {
		plabel = "源目录（回放读取）"
	}
	p("  <div class=\"pathrow\"><span class=\"plabel\">%s</span><code class=\"path\">%s</code><button type=\"button\" class=\"copy\">复制路径</button></div>\n", plabel, esc(e.CGDir))
	if v.Export && strings.TrimSpace(e.SourceDir) != "" {
		p("  <div class=\"pathrow sub\"><span class=\"plabel\">游戏源目录</span><code class=\"path\">%s</code></div>\n", esc(e.SourceDir))
	}
	if len(s.Roots) > 0 && (len(s.Roots) > 1 || s.Roots[0] != e.CGDir) {
		p("  <div class=\"pathrow sub\"><span class=\"plabel\">扫描根</span><code class=\"path\">%s</code></div>\n", esc(strings.Join(s.Roots, "  |  ")))
	}
	if e.Note != "" {
		p("  <div class=\"pathrow sub\"><span class=\"plabel\">清单备注</span><span class=\"note\">%s</span></div>\n", esc(e.Note))
	}

	if s.Shown == 0 {
		p("  <div class=\"empty\">\n")
		p("    <b>本款没有可渲染的图片</b>\n")
		p("    <p>status=<code>%s</code> · %s</p>\n", esc(s.Status), esc(s.Problem))
		if s.Encrypted > 0 {
			p("    <p>目录内加密图 %s 个；要肉眼验收需先用项目内的解密/导出链路产出明文 png。</p>\n", comma(s.Encrypted))
		}
		p("    <p>本款仍计入清单与统计（结论/备注照常可填），用于标记「图片缺失」。</p>\n")
		p("  </div>\n")
	} else {
		p("  <div class=\"grid\">\n")
		for _, im := range s.Images {
			p("    <figure class=\"card\" title=\"%s\">\n", esc(im.Rel))
			p("      <div class=\"thumb\"><img loading=\"lazy\" decoding=\"async\" src=\"%s\" alt=\"%s\"></div>\n", esc(fileURL(im.Abs)), esc(im.Rel))
			p("      <figcaption><span class=\"fn\">%s</span><span class=\"dim\">%s</span></figcaption>\n", esc(filepath.Base(im.Rel)), esc(dims(im)))
			p("    </figure>\n")
		}
		p("  </div>\n")
	}
	p("</section>\n")
}

// reasonTip 把预筛判据拼成 title 提示文案（判据缺失时退化成一行 severity）。
func (v *scanView) reasonTip() string {
	rs := asStrs(v.M, "severity_reasons")
	if len(rs) == 0 {
		return "预筛 severity " + strconv.Itoa(v.Sev)
	}
	return "预筛判据：" + strings.Join(rs, " / ")
}

// percent 把 dup_rate 归一成百分数：0.0588 → 5.9%；>1 视作已经是百分数。
func percent(f float64) string {
	if f <= 1 {
		return fmt.Sprintf("%.1f%%", f*100)
	}
	return fmt.Sprintf("%.1f%%", f)
}

// sevLabel 是目录导航里的 severity 短标签。
func sevLabel(sev int, warn bool) string {
	if sev < 0 {
		return "sev—"
	}
	if warn {
		return "⚠sev" + strconv.Itoa(sev)
	}
	return "sev" + strconv.Itoa(sev)
}

// prescreenDesc 描述预筛数据来源，供页脚展示。
func prescreenDesc(ps *prescreen) string {
	if ps == nil || (len(ps.Sources) == 0 && len(ps.Skipped) == 0) {
		return "未配置"
	}
	parts := make([]string, 0, 2)
	if len(ps.Sources) > 0 {
		parts = append(parts, strings.Join(ps.Sources, " + ")+"（已加载）")
	}
	if len(ps.Skipped) > 0 {
		parts = append(parts, strings.Join(ps.Skipped, " / "))
	}
	return strings.Join(parts, "；")
}

// renderClues 渲染「预筛线索条」：severity / verdict / small / outliers / blank / dup_rate / avg_kb / tiny_files。
// 任一字段缺失都渲染成 n/a（连字段本身都没有时也不报错），不猜值、不省略。
func renderClues(w io.Writer, v *scanView) {
	p := func(format string, a ...any) { fmt.Fprintf(w, format, a...) }
	m := v.M
	if m == nil {
		p("  <div class=\"clues none\">\n")
		p("    <span class=\"clabel\">预筛线索</span>\n")
		p("    <span class=\"cnone\">预筛指标未收录本款（预筛指标.json / 预筛指标_rmmz.json 无同名条目）</span>\n")
		p("  </div>\n")
		return
	}
	if tip := v.reasonTip(); tip != "" {
		p("  <div class=\"clues\" data-sev=\"%d\" title=\"%s\">\n", v.Sev, esc(tip))
	} else {
		p("  <div class=\"clues\" data-sev=\"%d\">\n", v.Sev)
	}
	p("    <span class=\"clabel\">预筛线索</span>\n")
	if v.Sev >= 0 {
		p("    <span class=\"cchip sev %s\">severity %d</span>\n", sevClass(v.Sev), v.Sev)
	} else {
		p("    <span class=\"cchip miss\">severity n/a</span>\n")
	}
	if verdict, ok := asStrOK(m, "verdict"); ok {
		p("    <span class=\"cchip verdict\">verdict %s</span>\n", esc(verdict))
	} else {
		p("    <span class=\"cchip miss\">verdict n/a</span>\n")
	}
	chipInt := func(key, label string) {
		if n, ok := asIntOK(m, key); ok {
			p("    <span class=\"cchip\">%s %d</span>\n", label, n)
			return
		}
		p("    <span class=\"cchip miss\">%s n/a</span>\n", label)
	}
	chipInt("small", "small")
	chipInt("outliers", "outliers")
	chipInt("blank", "blank")
	if f, ok := asFloatOK(m, "dup_rate"); ok {
		p("    <span class=\"cchip\">dup_rate %s</span>\n", esc(percent(f)))
	} else {
		p("    <span class=\"cchip miss\">dup_rate n/a</span>\n")
	}
	if f, ok := asFloatOK(m, "avg_kb"); ok {
		p("    <span class=\"cchip\">avg_kb %.1f</span>\n", f)
	} else {
		p("    <span class=\"cchip miss\">avg_kb n/a</span>\n")
	}
	chipInt("tiny_files", "tiny_files")
	if src, ok := asStrOK(m, "__src"); ok {
		p("    <span class=\"csrc\">源 %s</span>\n", esc(src))
	} else {
		p("    <span class=\"csrc\">源 预筛指标</span>\n")
	}
	p("  </div>\n")
}
