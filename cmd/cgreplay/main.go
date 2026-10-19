// Command cgreplay 是「双击即用」的 CG 演出回放器（Windows GUI）。
//
// 一个窗口里列全原作自己的事件单位——CG 鉴赏的 96 格（cgrai00.ks 的 *cgNN）
// 与主线剧本（01kuukou.ks…76utyu.ks），点一下就地回放，不用命令行、不用手输路径。
//
// 用法：
//
//	cgreplay.exe                      # 读 exe 同目录的 cgreplay.conf，没有就提示选目录
//	cgreplay.exe --root <解包目录>      # 直接扫这个解包目录
//	cgreplay.exe --lib  <库.json>      # 直接读已有事件库
//	cgreplay.exe --selftest            # 不开窗口自检：扫库 + 构建第一个 CG + 出一帧 PNG
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"ucgr/internal/adapter/kirikiri"
	"ucgr/internal/audio"
	"ucgr/internal/ir"
	"ucgr/internal/library"
	"ucgr/internal/platform/win32"
	"ucgr/internal/player"
	"ucgr/internal/replay"
)

// 控件 ID（1001/1002 被 win32 层占用：列表 / 状态栏）。
const (
	idChoose = 1003
	idRescan = 1004
	idPlay   = 1005
	idStop   = 1006
	idPrev   = 1007
	idNext   = 1008
	idAuto   = 1009
	idShot   = 1010
	idSearch = 1011
	idHint   = 1012
)

// 编辑框通知码（WM_COMMAND 的高位字）。
const (
	enChange        = 0x0300 // EN_CHANGE
	idEnterFallback = 1      // 单行编辑框回车：有些系统按 IDOK(1) 上报
)

// 窗口与画布尺寸；列表行高与行内缩略图边长。
const (
	clientW, clientH = 1520, 960
	gameW, gameH     = 1600, 1200
	autoWaitMs       = 2500
	listRowH         = 44
	listThumb        = 40
)

// conf 记忆上次的选择（存 exe 同目录），双击启动就能直接进。
type conf struct {
	Root    string `json:"root"`
	Library string `json:"library"`
	Auto    bool   `json:"auto"`
}

// entry 列表行 → 事件（CG / 场景 / 演出段落 三选一）。
type entry struct {
	cg      *library.CGEvent
	scene   *library.SceneEvent
	chapter *library.ChapterEvent
}

func (e entry) title() string {
	switch {
	case e.cg != nil:
		if n := len(e.cg.Pages); n > 1 {
			return fmt.Sprintf("%s（%d 页）", e.cg.Title, n)
		}
		return e.cg.Title
	case e.chapter != nil:
		return fmt.Sprintf("  #%d %s", e.chapter.Index, e.chapter.Title)
	case e.scene != nil:
		return "[场景] " + e.scene.Title
	}
	return "?"
}

type app struct {
	g        *win32.App
	conf     conf
	confPath string
	lib      *library.Library
	all      []entry // 全部事件（未过滤）
	entries  []entry // 当前列表显示（已过滤）

	p        *player.Player
	load     player.Loader
	loadRoot string
	td       *win32.TextDrawer
	listTD   *win32.TextDrawer
	thumbs   map[string]*win32.Canvas
	filter   string
	auto     bool
	lastAd   time.Time
	title    string
	done     bool
	shotN    int

	audio     *audio.System
	audioRoot string
}

func main() {
	rootFlag := flag.String("root", "", "解包目录（含 raw\\data\\scenario 的那一层）")
	libFlag := flag.String("lib", "", "事件库 JSON（不给则按解包目录现扫）")
	autoFlag := flag.Bool("auto", true, "自动推进点击等待")
	selftest := flag.Bool("selftest", false, "不开窗口自检：扫库 → 构建 CG 事件 → 出一帧 PNG 后退出")
	selfID := flag.String("selftest-event", "", "自检用的事件 id（默认第一个 CG）")
	flag.Parse()

	a := &app{conf: conf{Auto: *autoFlag}, auto: *autoFlag}
	a.confPath = filepath.Join(exeDir(), "cgreplay.conf")
	a.loadConf()
	if *rootFlag != "" {
		a.conf.Root = *rootFlag
	}
	if *libFlag != "" {
		a.conf.Library = *libFlag
	}

	if *selftest {
		os.Exit(a.selftest(*selfID))
	}

	g, err := win32.NewApp("CG 演出回放器 — 重返蓝鲸岛", clientW, clientH, gameW, gameH, win32.GUIHooks{
		OnCommand:     a.onCommand,
		OnTick:        a.onTick,
		OnKey:         a.onKey,
		OnDrawItem:    a.drawItem,
		OnSearchEnter: a.play,
		OnSearchNav:   a.searchNav,
	})
	if err != nil {
		fatal(err)
	}
	defer g.Free()
	a.g = g
	if err := a.buildUI(); err != nil {
		fatal(err)
	}
	a.reload(false)
	a.ensureAudio(a.conf.Root)
	if err := g.Run(); err != nil {
		fatal(err)
	}
	if a.audio != nil {
		a.audio.Close()
	}
}

// ---------------------------------------------------------------
// 界面
// ---------------------------------------------------------------

func (a *app) buildUI() error {
	g := a.g
	if err := g.AddEdit(idSearch, "", 8, 8, 300, 24); err != nil {
		return err
	}
	if err := g.AddStatic(idHint, "输入名字过滤（回车播放，↑/↓ 选择）", 316, 12, 240, 18); err != nil {
		return err
	}
	if err := g.AddOwnerDrawList(win32.IDList, 8, 40, 430, 848, listRowH); err != nil {
		return err
	}
	g.SubclassEdit(idSearch)
	a.listTD = win32.NewTextDrawer(win32.ListTextStyle("SimSun", 15))
	if err := g.AddPlayArea(446, 8, 1066, 880); err != nil {
		return err
	}
	btns := []struct {
		id   int
		text string
		w    int
	}{
		{idChoose, "选择游戏目录…", 130},
		{idRescan, "重新扫描", 90},
		{idPlay, "播放 / 重放", 100},
		{idStop, "停止", 70},
		{idPrev, "上一个", 70},
		{idNext, "下一个", 70},
		{idAuto, a.autoText(), 130},
		{idShot, "截图", 60},
	}
	x := 8
	for _, b := range btns {
		if err := g.AddButton(b.id, b.text, x, 896, b.w, 30); err != nil {
			return err
		}
		x += b.w + 6
	}
	return g.AddStatic(win32.IDStatus, "就绪", 8, 932, clientW-16, 22)
}

func (a *app) autoText() string {
	if a.auto {
		return "自动推进：开"
	}
	return "自动推进：关"
}

// ---------------------------------------------------------------
// 事件库
// ---------------------------------------------------------------

func (a *app) reload(force bool) {
	if a.conf.Library != "" && !force {
		if lib, err := library.Load(a.conf.Library); err == nil {
			a.lib = lib
			if lib.Root != "" {
				a.conf.Root = lib.Root
			}
			a.ensureLoader(a.conf.Root)
			a.fillList()
			return
		}
	}
	root := strings.TrimSpace(a.conf.Root)
	if root == "" {
		a.g.SetStatus("请点「选择游戏目录…」选解包目录（含 raw\\data\\scenario 的那一层）")
		return
	}
	lib, err := library.Scan(root)
	if err != nil {
		a.g.SetStatus("扫描失败：" + err.Error())
		return
	}
	a.lib = lib
	if lp := filepath.Join(exeDir(), "cgreplay-lib.json"); lib.Save(lp) == nil {
		a.conf.Library = lp
	}
	a.saveConf()
	a.ensureLoader(root)
	a.fillList()
}

// ensureLoader 按素材根准备/重建素材加载器（列表缩略图与回放共用）。
func (a *app) ensureLoader(root string) {
	if root == "" || (a.load != nil && a.loadRoot == root) {
		return
	}
	for _, cv := range a.thumbs {
		if cv != nil {
			cv.Free()
		}
	}
	a.thumbs = map[string]*win32.Canvas{}
	a.load = player.Cached(player.Loader(replay.GameLoader(root)))
	a.loadRoot = root
}

func (a *app) fillList() {
	a.all = a.all[:0]
	for i := range a.lib.CGs {
		a.all = append(a.all, entry{cg: &a.lib.CGs[i]})
	}
	for i := range a.lib.Scenes {
		s := &a.lib.Scenes[i]
		a.all = append(a.all, entry{scene: s})
		// 场景下面挂它的演出段落（原作自己的章节标记），缩进以示从属。
		for j := range a.lib.Chapters {
			if a.lib.Chapters[j].Scene == s.ID {
				a.all = append(a.all, entry{chapter: &a.lib.Chapters[j]})
			}
		}
	}
	a.applyFilter()
}

// applyFilter 按搜索框内容重建列表（空查询 = 全部）。
func (a *app) applyFilter() {
	g := a.g
	g.ListClear()
	a.entries = a.entries[:0]
	q := strings.ToLower(strings.TrimSpace(a.filter))
	for _, e := range a.all {
		if q != "" && !entryMatches(e, q) {
			continue
		}
		a.entries = append(a.entries, e)
		g.ListAdd(e.title())
	}
	if len(a.entries) > 0 {
		g.ListSetSel(0)
	}
	a.g.SetStatus(fmt.Sprintf("事件库：CG %d 个 · 场景 %d 个 · 演出段落 %d 个 · 列表 %d 项%s · 素材根 %s",
		len(a.lib.CGs), len(a.lib.Scenes), len(a.lib.Chapters), len(a.entries),
		filterNote(q), a.conf.Root))
}

func filterNote(q string) string {
	if q == "" {
		return ""
	}
	return fmt.Sprintf("（过滤「%s」）", q)
}

// entryMatches 判断事件是否命中查询串（大小写不敏感；CG 还认 17 / cg17 等写法）。
func entryMatches(e entry, q string) bool {
	var fields []string
	switch {
	case e.cg != nil:
		fields = append(fields, e.cg.ID, e.cg.Label, e.cg.Title)
		fields = append(fields, e.cg.Pages...)
		if n, err := strconv.Atoi(strings.TrimPrefix(strings.TrimPrefix(q, "cg"), "0")); err == nil && n == e.cg.Index {
			return true
		}
	case e.chapter != nil:
		fields = append(fields, e.chapter.Title, e.chapter.Name, e.chapter.ID, e.chapter.Scene)
	case e.scene != nil:
		fields = append(fields, e.scene.ID, e.scene.Title)
	}
	for _, f := range fields {
		if f != "" && strings.Contains(strings.ToLower(f), q) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------
// 回放
// ---------------------------------------------------------------

func (a *app) play() {
	i := a.g.ListSel()
	if i < 0 || i >= len(a.entries) {
		a.g.SetStatus("先在上面的列表里选一个事件（双击即播）")
		return
	}
	root := strings.TrimSpace(a.conf.Root)
	if root == "" {
		a.g.SetStatus("还没选游戏目录")
		return
	}
	e := a.entries[i]
	var script, label string
	chapterPos := -1
	switch {
	case e.cg != nil:
		script, label = e.cg.Script, e.cg.Label
	case e.chapter != nil:
		script, chapterPos = e.chapter.Script, e.chapter.LabelPos
	case e.scene != nil:
		script = e.scene.Script
	}
	if script == "" {
		a.g.SetStatus("这个事件没有脚本信息")
		return
	}
	sp := filepath.Join(root, filepath.FromSlash(script))
	var ev *ir.Event
	var err error
	if chapterPos >= 0 {
		ev, err = kirikiri.BuildShowChapterFile(sp, chapterPos)
	} else {
		ev, err = kirikiri.BuildShowSegment(sp, label)
	}
	if err != nil {
		a.g.SetStatus("构建台本失败：" + err.Error())
		return
	}
	if len(ev.Show) == 0 {
		a.g.SetStatus(e.title() + " 没有可回放的演出（台本 0 条）")
		return
	}
	opt := player.DefaultOptions()
	opt.FontSize = 48
	a.ensureDrawer(root, opt)
	a.p = player.New(ev, opt)
	a.ensureAudio(root)
	if a.audio != nil {
		a.p.Audio = a.audio.Handle
	}
	a.ensureLoader(root)
	a.p.Advance(0, player.Input{}) // 先把 t=0 的台本指令执行掉，首帧才不会空
	a.lastAd = time.Now()
	a.title = e.title()
	a.done = false
	a.g.Focus()
	a.present()
	wait := "等点击推进"
	if a.auto {
		wait = "自动推进"
	}
	a.g.SetStatus(fmt.Sprintf("%s · 台本 %d 条 · %s · 空格/回车/点画面推进 · M 静音", a.title, len(ev.Show), wait))
}

func (a *app) stop() {
	if a.p == nil {
		return
	}
	a.p = nil
	a.stopAudio()
	a.g.Focus()
	a.g.SetStatus("已停止：" + a.title)
}

func (a *app) selectDelta(d int) {
	n := a.g.ListCount()
	if n == 0 {
		return
	}
	i := a.g.ListSel()
	if i < 0 {
		i = 0
	} else {
		i += d
	}
	if i < 0 {
		i = 0
	}
	if i >= n {
		i = n - 1
	}
	a.g.ListSetSel(i)
	a.play()
}

// searchNav 搜索框里上/下移动列表选择（不立即播放，回车才播）。
func (a *app) searchNav(down bool) {
	n := a.g.ListCount()
	if n == 0 {
		return
	}
	i := a.g.ListSel()
	if i < 0 {
		i = 0
	} else if down {
		i++
	} else {
		i--
	}
	if i < 0 {
		i = 0
	}
	if i >= n {
		i = n - 1
	}
	a.g.ListSetSel(i)
	a.g.ListTop(i)
}

// drawItem 自绘列表行：选中底色 + CG 缩略图 + 标题。
func (a *app) drawItem(itemID int, hdc uintptr, sel bool, x, y, w, h int) {
	if itemID < 0 || itemID >= len(a.entries) {
		return
	}
	e := a.entries[itemID]
	bg := [3]uint8{255, 255, 255}
	if sel {
		bg = [3]uint8{205, 225, 255}
	}
	win32.FillRect(hdc, x, y, w, h, bg, 255)
	tx := x + 6
	if cv := a.thumbFor(e); cv != nil {
		win32.Stretch(hdc, cv, x+4, y+(h-cv.H)/2, cv.W, cv.H)
		tx = x + 4 + cv.W + 8
	}
	if a.listTD != nil {
		st := a.listTD.TextStyle()
		a.listTD.Draw(hdc, e.title(), tx, y, x+w-tx-6, h, *st)
	}
}

// thumbFor 取事件的方形缩略图（CG 用画廊图最左一格；场景/段落无图）。
//
// 画廊按钮图是 840×210 的横排（同一张 CG 的几个按钮状态），取最左一格方形即可。
func (a *app) thumbFor(e entry) *win32.Canvas {
	if e.cg == nil || a.load == nil || a.thumbs == nil {
		return nil
	}
	key := strings.TrimSpace(e.cg.Thumb)
	if key == "" {
		return nil
	}
	if cv, ok := a.thumbs[key]; ok {
		return cv // 成功的是画布，失败的记 nil，避免反复读盘
	}
	a.thumbs[key] = nil
	img, err := a.load(key)
	if err != nil {
		return nil
	}
	b := img.Bounds()
	side := b.Dy()
	if b.Dx() < side {
		side = b.Dx()
	}
	if side <= 0 {
		return nil
	}
	sq := image.NewRGBA(image.Rect(0, 0, side, side))
	draw.Draw(sq, sq.Bounds(), img, image.Pt(b.Min.X, b.Min.Y), draw.Src)
	cv, err := win32.NewCanvas(listThumb, listThumb)
	if err != nil {
		return nil
	}
	cv.BlitRGBA(fitSquare(sq, listThumb))
	a.thumbs[key] = cv
	return cv
}

// fitSquare 最近邻缩放到 size×size（缩略图很小，够用且快）。
func fitSquare(src *image.RGBA, size int) *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, size, size))
	sb := src.Bounds()
	if sb.Dx() <= 0 || sb.Dy() <= 0 {
		return out
	}
	for y := 0; y < size; y++ {
		sy := sb.Min.Y + y*sb.Dy()/size
		for x := 0; x < size; x++ {
			sx := sb.Min.X + x*sb.Dx()/size
			si := src.PixOffset(sx, sy)
			di := out.PixOffset(x, y)
			copy(out.Pix[di:di+4], src.Pix[si:si+4])
		}
	}
	return out
}

// present 合成当前帧并贴到画布（文字用 GDI 另画在图层之上）。
func (a *app) present() {
	if a.p == nil || a.g == nil {
		return
	}
	c := a.g.Canvas()
	rgba, _, err := player.RenderFrame(a.p, a.load, gameW, gameH)
	if err != nil {
		a.g.SetStatus("合成失败：" + err.Error())
		return
	}
	c.BlitRGBA(rgba)
	if a.td != nil {
		a.td.DrawBoxes(c.DC(), a.p.Frame().Texts, 1)
	}
}

func (a *app) ensureDrawer(root string, opt player.Options) {
	if a.td != nil {
		return
	}
	// 本作自带思源宋体 SC（OTF/CFF，且被 XOR 混淆）：还原并私有注册到 GDI。
	face := "SimSun"
	ff := filepath.Join(root, "raw", "data", "SourceHanSerifSC-Regular.otf")
	if n, ok := win32.RegisterFontFile(ff); ok {
		face = n
	}
	a.td = win32.NewTextDrawer(win32.DefaultTextStyle(face, opt.FontSize))
}

// ensureAudio 按当前素材根准备/重建音频系统（KAG 语义的 BGM/SE）。
func (a *app) ensureAudio(root string) {
	if a.audio != nil && a.audioRoot == root {
		return
	}
	if a.audio != nil {
		a.audio.Close()
		a.audio = nil
	}
	a.audioRoot = ""
	if root == "" {
		return
	}
	if sys, err := audio.NewSystem(root); err == nil {
		a.audio = sys
		a.audioRoot = root
	}
}

// stopAudio 停止所有 BGM/SE（保留音频设备）。
func (a *app) stopAudio() {
	if a.audio != nil {
		a.audio.StopAll()
	}
}

func (a *app) saveShot() {
	if a.g == nil {
		return
	}
	c := a.g.Canvas()
	c.ForceOpaque()
	dir := filepath.Join(exeDir(), "shots")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		a.g.SetStatus("截图失败：" + err.Error())
		return
	}
	a.shotN++
	p := filepath.Join(dir, fmt.Sprintf("shot_%03d.png", a.shotN))
	fh, err := os.Create(p)
	if err != nil {
		a.g.SetStatus("截图失败：" + err.Error())
		return
	}
	defer fh.Close()
	if err := png.Encode(fh, c.Image()); err != nil {
		a.g.SetStatus("截图失败：" + err.Error())
		return
	}
	a.g.SetStatus("已截图：" + p)
}

// ---------------------------------------------------------------
// 回调
// ---------------------------------------------------------------

func (a *app) onCommand(id, notify int) {
	switch {
	case id == win32.IDList && notify == win32.NotifyDblClk:
		a.play()
	case id == idChoose:
		dir, err := a.g.BrowseFolder("选解包目录（含 raw\\data\\scenario 的那一层）")
		if err != nil {
			a.g.SetStatus("目录对话框失败：" + err.Error())
			return
		}
		if dir == "" {
			return
		}
		a.conf.Root = dir
		a.conf.Library = ""
		a.saveConf()
		a.reload(true)
	case id == idRescan:
		a.conf.Library = ""
		a.reload(true)
	case id == idPlay:
		a.play()
	case id == idStop:
		a.stop()
	case id == idPrev:
		a.selectDelta(-1)
	case id == idNext:
		a.selectDelta(1)
	case id == idAuto:
		a.auto = !a.auto
		a.conf.Auto = a.auto
		a.saveConf()
		a.g.SetStatus(a.autoText() + "（关时靠空格/回车/点画面推进）")
	case id == idShot:
		a.saveShot()
	case id == idSearch && notify == enChange:
		a.filter = a.g.EditText(idSearch)
		a.applyFilter()
	case id == idEnterFallback && a.p == nil: // 单行编辑框回车在有些系统上按 IDOK 上报
		a.play()
	}
}

func (a *app) onTick(dtMs int64, in win32.Input) {
	if a.p == nil {
		return
	}
	if in.Advance {
		a.p.Advance(0, player.Input{Advance: true})
		a.lastAd = time.Now()
	} else {
		a.p.Advance(dtMs, player.Input{})
	}
	if a.auto {
		if f := a.p.Frame(); f.Waiting && time.Since(a.lastAd) > autoWaitMs*time.Millisecond {
			a.p.Advance(0, player.Input{Advance: true})
			a.lastAd = time.Now()
		}
	}
	if in.RequestShot {
		a.saveShot()
	}
	a.present()
	if !a.done && a.p.Done() {
		a.done = true
		a.g.SetStatus(a.title + " · 播放结束（点「下一个」或按 N 继续）")
	}
}

func (a *app) onKey(key int) bool {
	switch key {
	case 0x1B: // Esc：播放中先停回列表，不直接退出窗口
		if a.p != nil {
			a.stop()
			return true
		}
	case 'N':
		a.selectDelta(1)
		return true
	case 'B':
		a.selectDelta(-1)
		return true
	case 'M': // 静音开关
		if a.audio != nil {
			a.audio.SetMute(!a.audio.Muted())
		}
		return true
	case 0x0D, 0x20: // 回车 / 空格：在播放就当作手动推进，没在播放就播当前选中项
		if a.p != nil {
			a.lastAd = time.Now()
		} else {
			a.play()
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------
// 配置与自检
// ---------------------------------------------------------------

func (a *app) loadConf() {
	data, err := os.ReadFile(a.confPath)
	if err != nil {
		return
	}
	var c conf
	if json.Unmarshal(data, &c) != nil {
		return
	}
	a.conf = c
	a.auto = c.Auto
}

func (a *app) saveConf() {
	data, err := json.MarshalIndent(a.conf, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(a.confPath, append(data, '\n'), 0o644)
}

// selftest 不开窗口跑一遍主链路：扫库 → 构建 CG 事件 → 推 400ms → 出一帧 PNG。
func (a *app) selftest(id string) int {
	root := strings.TrimSpace(a.conf.Root)
	if root == "" {
		fmt.Println("自检失败：没有解包目录（用 --root 指定）")
		return 2
	}
	lib, err := library.Scan(root)
	if err != nil {
		fmt.Println("自检失败：", err)
		return 2
	}
	fmt.Printf("事件库：CG %d · 场景 %d · 演出段落 %d\n", len(lib.CGs), len(lib.Scenes), len(lib.Chapters))

	// ① 按名过滤自检：cg17 应命中 cg017 这一条。
	hit, hitID := 0, ""
	for i := range lib.CGs {
		if entryMatches(entry{cg: &lib.CGs[i]}, "cg17") {
			hit++
			hitID = lib.CGs[i].ID
		}
	}
	fmt.Printf("按名过滤「cg17」命中 %d 个（%s）\n", hit, hitID)

	// ② 缩略图自检：解码第一个 CG 的画廊图，裁剪最左一格方形缩略图并落盘。
	if len(lib.CGs) > 0 {
		load := player.Cached(player.Loader(replay.GameLoader(root)))
		if img, lerr := load(lib.CGs[0].Thumb); lerr == nil {
			b := img.Bounds()
			side := b.Dy()
			if b.Dx() < side {
				side = b.Dx()
			}
			if side > 0 {
				sq := image.NewRGBA(image.Rect(0, 0, side, side))
				draw.Draw(sq, sq.Bounds(), img, image.Pt(b.Min.X, b.Min.Y), draw.Src)
				tp := filepath.Join(exeDir(), "selftest-thumb.png")
				if fh, cerr := os.Create(tp); cerr == nil {
					png.Encode(fh, fitSquare(sq, 210))
					fh.Close()
				}
				fmt.Printf("缩略图 %s：%d×%d → %s\n", lib.CGs[0].Thumb, b.Dx(), b.Dy(), tp)
			}
		}
	}

	// 自检对象可以是 CG、演出段落，或（id 为空时）第一个 CG。
	var (
		ev    *ir.Event
		desc  string
		extra string
	)
	if ch := lib.FindChapter(id); ch != nil {
		sp := filepath.Join(root, filepath.FromSlash(ch.Script))
		e, berr := kirikiri.BuildShowChapterFile(sp, ch.LabelPos)
		if berr != nil {
			fmt.Println("自检失败：", berr)
			return 2
		}
		ev, desc, extra = e, fmt.Sprintf("章节 #%d %s", ch.Index, ch.Title), ch.ID
	} else {
		var cg *library.CGEvent
		if id != "" {
			cg = lib.FindCG(id)
		} else if len(lib.CGs) > 0 {
			cg = &lib.CGs[0]
		}
		if cg == nil {
			fmt.Println("自检失败：找不到 CG 事件或演出段落")
			return 2
		}
		sp := filepath.Join(root, filepath.FromSlash(cg.Script))
		e, berr := kirikiri.BuildShowSegment(sp, cg.Label)
		if berr != nil {
			fmt.Println("自检失败：", berr)
			return 2
		}
		ev, desc, extra = e, cg.Title, fmt.Sprintf("页 %v", cg.Pages)
	}
	p := player.New(ev, player.DefaultOptions())
	load := player.Cached(player.Loader(replay.GameLoader(root)))
	// 与 CLI 出图同一节奏：先零步进执行 t=0 的台本指令，再按 16ms 步进推到 400ms。
	p.Advance(0, player.Input{})
	for p.Now() < 400 {
		p.Advance(16, player.Input{})
	}
	rgba, _, err := player.RenderFrame(p, load, gameW, gameH)
	if err != nil {
		fmt.Println("自检失败：", err)
		return 2
	}
	out := filepath.Join(exeDir(), "selftest.png")
	fh, err := os.Create(out)
	if err != nil {
		fmt.Println("自检失败：", err)
		return 2
	}
	defer fh.Close()
	if err := png.Encode(fh, rgba); err != nil {
		fmt.Println("自检失败：", err)
		return 2
	}
	fmt.Printf("自检 OK：%s · 台本 %d 条 · %s → %s\n", desc, len(ev.Show), extra, out)
	return 0
}

func exeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "cgreplay:", err)
	os.Exit(1)
}
