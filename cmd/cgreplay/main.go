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
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ucgr/internal/adapter/kirikiri"
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
)

// 窗口与画布尺寸。
const (
	clientW, clientH = 1520, 960
	gameW, gameH     = 1600, 1200
	autoWaitMs       = 2500
)

// conf 记忆上次的选择（存 exe 同目录），双击启动就能直接进。
type conf struct {
	Root    string `json:"root"`
	Library string `json:"library"`
	Auto    bool   `json:"auto"`
}

// entry 列表行 → 事件（CG 或场景二选一）。
type entry struct {
	cg    *library.CGEvent
	scene *library.SceneEvent
}

func (e entry) title() string {
	switch {
	case e.cg != nil:
		if n := len(e.cg.Pages); n > 1 {
			return fmt.Sprintf("%s（%d 页）", e.cg.Title, n)
		}
		return e.cg.Title
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
	entries  []entry

	p      *player.Player
	load   player.Loader
	td     *win32.TextDrawer
	auto   bool
	lastAd time.Time
	title  string
	done   bool
	shotN  int
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
		OnCommand: a.onCommand,
		OnTick:    a.onTick,
		OnKey:     a.onKey,
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
	if err := g.Run(); err != nil {
		fatal(err)
	}
}

// ---------------------------------------------------------------
// 界面
// ---------------------------------------------------------------

func (a *app) buildUI() error {
	g := a.g
	if err := g.AddList(win32.IDList, 8, 8, 430, 880); err != nil {
		return err
	}
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
	a.fillList()
}

func (a *app) fillList() {
	g := a.g
	g.ListClear()
	a.entries = a.entries[:0]
	for i := range a.lib.CGs {
		e := entry{cg: &a.lib.CGs[i]}
		a.entries = append(a.entries, e)
		g.ListAdd(e.title())
	}
	for i := range a.lib.Scenes {
		e := entry{scene: &a.lib.Scenes[i]}
		a.entries = append(a.entries, e)
		g.ListAdd(e.title())
	}
	if len(a.entries) > 0 {
		g.ListSetSel(0)
	}
	g.SetStatus(fmt.Sprintf("事件库：CG %d 个 · 场景 %d 个 · 素材根 %s",
		len(a.lib.CGs), len(a.lib.Scenes), a.conf.Root))
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
	switch {
	case e.cg != nil:
		script, label = e.cg.Script, e.cg.Label
	case e.scene != nil:
		script = e.scene.Script
	}
	if script == "" {
		a.g.SetStatus("这个事件没有脚本信息")
		return
	}
	sp := filepath.Join(root, filepath.FromSlash(script))
	ev, err := kirikiri.BuildShowSegment(sp, label)
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
	a.load = player.Cached(player.Loader(replay.GameLoader(root)))
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
	a.g.SetStatus(fmt.Sprintf("%s · 台本 %d 条 · %s · 空格/回车/点画面手动推进", a.title, len(ev.Show), wait))
}

func (a *app) stop() {
	if a.p == nil {
		return
	}
	a.p = nil
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
	face := "SourceHanSerifSC-Regular"
	ff := filepath.Join(root, "raw", "data", "SourceHanSerifSC-Regular.otf")
	if st, err := os.Stat(ff); err == nil && !st.IsDir() {
		if !win32.RegisterFont(ff) {
			face = "SimSun" // GDI 认不了 OTF/CFF 时退回系统字体
		}
	} else {
		face = "SimSun"
	}
	a.td = win32.NewTextDrawer(win32.DefaultTextStyle(face, opt.FontSize))
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
	fmt.Printf("事件库：CG %d · 场景 %d\n", len(lib.CGs), len(lib.Scenes))
	var cg *library.CGEvent
	if id != "" {
		cg = lib.FindCG(id)
	} else if len(lib.CGs) > 0 {
		cg = &lib.CGs[0]
	}
	if cg == nil {
		fmt.Println("自检失败：找不到 CG 事件")
		return 2
	}
	sp := filepath.Join(root, filepath.FromSlash(cg.Script))
	ev, err := kirikiri.BuildShowSegment(sp, cg.Label)
	if err != nil {
		fmt.Println("自检失败：", err)
		return 2
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
	fmt.Printf("自检 OK：%s · 台本 %d 条 · 页 %v → %s\n", cg.Title, len(ev.Show), cg.Pages, out)
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
