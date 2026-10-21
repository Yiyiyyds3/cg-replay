// 绑定层：把 core 能力暴露给 React 前端（方案 B M2：事件网格卡墙）。
//
// 职责边界（docs/06 / 文稿 §红线）：本壳不做任何合成/解码/索引逻辑，
// 只做「桥 + 薄缓存」；封面是唯一派生物，只写 exe 同目录 covers/（DR-30）。
// conf 与 CLI（cmd/cgreplay）共用同目录的 cgreplay.conf：两边互通。
package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"ucgr/internal/adapter"
	"ucgr/internal/catalog"
	"ucgr/internal/library"
	"ucgr/internal/registry"
	"ucgr/internal/replay"
)

// gameConvention 取一个游戏的扫描约定：
//
//	① 引擎默认约定（KiriKiri = 本作实测的 raw\data\scenario + cgrai*.ks + cgNN）；
//	② 游戏根目录下的 `ucgr-convention.json` 覆盖（同引擎不同游戏的目录/命名差异
//	   **只改配置不改代码** —— 这就是「引擎适配一劳永逸」落地的关键）。
//
// 约定文件写坏了要报错（用户明确写了配置却写坏，静默忽略会让人以为配置生效了）。
func gameConvention(root, engineID string) (adapter.Convention, error) {
	conv, ok := adapter.ConventionByEngine(engineID)
	if !ok {
		// 未知引擎先按 KiriKiri 试（约定文件可覆盖）；真扫不出事件时上层会报错。
		conv = adapter.DefaultKiriKiri()
	}
	file, exists, err := adapter.LoadConventionFile(root)
	if err != nil {
		return conv, err
	}
	if exists {
		conv = file.Merge(conv)
	}
	if err := conv.Validate(); err != nil {
		return conv, err
	}
	return conv, nil
}

// exeDir 返回 exe 所在目录（只读用途：如日志排查时告诉用户 exe 在哪）。
func exeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

// dataDir 返回**可写数据目录**（conf/games.json/libraries/covers/exports/
// catalog.db/errors.log 都落这里）：优先 %LOCALAPPDATA%\cgreplay-ui，回退
// %APPDATA%\cgreplay-ui，再回退 exe 同目录（拿不到用户目录的老行为）。
// 开发期（build/bin 直接跑）与安装后（Program Files）走同一位置，
// 行为一致；且安装到 Program Files 后非管理员也能正常写。
// 副作用：确保目录存在（建不出来则回退 exeDir）。
func dataDir() string {
	var base string
	switch {
	case os.Getenv("LOCALAPPDATA") != "":
		base = os.Getenv("LOCALAPPDATA")
	case os.Getenv("APPDATA") != "":
		base = os.Getenv("APPDATA")
	default:
		return exeDir()
	}
	dir := filepath.Join(base, "cgreplay-ui")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return exeDir()
	}
	return dir
}

// legacyDataNames 老版本（v0.2 及以前）落在 exe 同目录的数据文件。
var legacyDataFiles = []string{
	confName,
	registry.FileName,
	catalog.FileName,
	catalog.FileName + "-wal",
	catalog.FileName + "-shm",
	"errors.log",
}

// legacyDataDirs 老版本落在 exe 同目录的数据目录。
var legacyDataDirs = []string{"libraries", "covers", "exports"}

// migrateLegacyData 一次性把 from（老 exe 目录）里的既有数据搬到 to（dataDir）。
// 规则：目标已存在则不覆盖；**只搬不删**（老目录留着当备份——删用户数据
// 是不可逆操作，不符合本项目的静默原则）。from == to 或 from 没数据时no-op。
func migrateLegacyData(from, to string) {
	if from == to {
		return
	}
	if err := os.MkdirAll(to, 0o755); err != nil {
		return // 建不了目标：保持老路径行为，后续读写用 exeDir 回退
	}
	for _, name := range legacyDataFiles {
		copyIfAbsent(filepath.Join(from, name), filepath.Join(to, name))
	}
	for _, name := range legacyDataDirs {
		copyDirIfAbsent(filepath.Join(from, name), filepath.Join(to, name))
	}
}

// copyIfAbsent 目标不存在才复制（存在即跳过，不覆盖用户数据）。
func copyIfAbsent(src, dst string) {
	if src == dst {
		return
	}
	if _, err := os.Stat(dst); err == nil {
		return
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return // 源不存在（首次运行）或读不了：跳过
	}
	_ = os.WriteFile(dst, data, 0o644)
}

// copyDirIfAbsent 目标目录不存在才整目录复制（存在即跳过）。
func copyDirIfAbsent(src, dst string) {
	if src == dst {
		return
	}
	if _, err := os.Stat(dst); err == nil {
		return
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return // 源不存在：跳过
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return
	}
	for _, e := range entries {
		s, d := filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())
		if e.IsDir() {
			copyDirIfAbsent(s, d)
			continue
		}
		copyIfAbsent(s, d)
	}
}

// conf 记忆上次的选择，与 cmd/cgreplay 同格式（文件位置不同：
// GUI 在 dataDir()，CLI 在 exe 同目录——GUI 安装后 exe 目录不可写）。
type conf struct {
	Root    string `json:"root"`
	Library string `json:"library"`
	Auto    bool   `json:"auto"`
}

var confName = "cgreplay.conf"

// Conf 对外暴露的配置 DTO。
type Conf struct {
	Root        string `json:"root"`
	Library     string `json:"library"`
	Auto        bool   `json:"auto"`
	ConfMissing bool   `json:"conf_missing"`
}

// GameDTO 前端游戏切换器条目（与 internal/registry.Game 对齐）。
type GameDTO struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Root         string `json:"root"`
	Library      string `json:"library,omitempty"`
	Engine       string `json:"engine,omitempty"`
	AddedAt      string `json:"added_at"`
	LastPlayedAt string `json:"last_played_at,omitempty"`
}

// GamesDTO 游戏库清单。
type GamesDTO struct {
	Active string    `json:"active"`
	Games  []GameDTO `json:"games"`
}

// loadConf 读 dataDir conf；不存在返回零值 + missing 标记（不静默）。
func (a *App) loadConf() (conf, bool) {
	var c conf
	data, err := os.ReadFile(filepath.Join(dataDir(), confName))
	if err != nil {
		return c, false
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return c, false
	}
	return c, true
}

// saveConf 原子落盘。
func (a *App) saveConf(c conf) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(dataDir(), confName)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// GetConf 返回当前 conf（前端启动时调；missing 时提示选目录，FR-12 不静默）。
func (a *App) GetConf() (Conf, error) {
	c, ok := a.loadConf()
	return Conf{Root: c.Root, Library: c.Library, Auto: c.Auto, ConfMissing: !ok}, nil
}

// PickDirectory 弹系统目录选择框，返回绝对路径（用户取消返回 ""）。
func (a *App) PickDirectory() (string, error) {
	return wruntime.OpenDirectoryDialog(a.ctx, wruntime.OpenDialogOptions{
		Title: "选择游戏解包目录（含 raw\\data\\scenario 的那一层）",
	})
}

// registryPath 多游戏登记表路径（与 cgreplay.conf 同目录）。
func (a *App) registryPath() string { return registry.Path(dataDir()) }

// loadRegistry 读游戏登记表。首次运行（games.json 不存在）把旧版 conf
// 里的单根目录迁移成一条登记项；迁移只做一次，失败原样上抛。
func (a *App) loadRegistry(c conf) (*registry.Registry, error) {
	path := a.registryPath()
	reg, err := registry.Load(path)
	if err == nil {
		if reg.ActiveGame() == nil && len(reg.Games) > 0 {
			reg.Active = reg.Games[0].ID
		}
		return reg, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	reg = registry.New()
	if strings.TrimSpace(c.Root) != "" {
		if clean, cerr := registry.CleanRoot(c.Root); cerr == nil {
			if _, serr := os.Stat(library.ScenarioDir(clean)); serr == nil {
				libPath := strings.TrimSpace(c.Library)
				if libPath == "" {
					libPath = registry.LibraryPath(dataDir(), registry.IDFor(clean))
				}
				g, _ := reg.Ensure(clean, libPath)
				reg.Active = g.ID
			}
		}
	}
	if err := reg.Save(path); err != nil {
		return nil, err
	}
	return reg, nil
}

// activeGame 当前游戏；没有登记项时给出可操作的错误（不静默）。
func (a *App) activeGame(c conf) (*registry.Registry, *registry.Game, error) {
	reg, err := a.loadRegistry(c)
	if err != nil {
		return nil, nil, err
	}
	g := reg.ActiveGame()
	if g == nil {
		return nil, nil, fmt.Errorf("还没有游戏库，请先添加游戏解包目录")
	}
	return reg, g, nil
}

// syncActiveConf 把当前游戏写回 cgreplay.conf（CLI 版继续读同一份配置）。
func (a *App) syncActiveConf(g *registry.Game) error {
	c, _ := a.loadConf()
	if c.Root == g.Root && c.Library == g.Library {
		return nil
	}
	c.Root = g.Root
	c.Library = g.Library
	return a.saveConf(c)
}

// ListGames 游戏库清单（卡墙启动时调，前端据此渲染切换器）。
func (a *App) ListGames() (GamesDTO, error) {
	c, _ := a.loadConf()
	reg, err := a.loadRegistry(c)
	if err != nil {
		return GamesDTO{}, err
	}
	out := GamesDTO{Active: reg.Active, Games: make([]GameDTO, 0, len(reg.Games))}
	for _, g := range reg.Games {
		out.Games = append(out.Games, GameDTO{
			ID: g.ID, Title: g.Title, Root: g.Root, Library: g.Library,
			Engine: g.Engine, AddedAt: g.AddedAt, LastPlayedAt: g.LastPlayedAt,
		})
	}
	return out, nil
}

// SetRoot 兼容旧前端：等价于添加/切换游戏。
func (a *App) SetRoot(root string) (*library.Library, error) { return a.AddGame(root) }

// AddGame 登记并扫描一款游戏，设为当前游戏，返回事件库。
// 同一根目录重复添加只切换并重扫，不会产生重复登记。
func (a *App) AddGame(root string) (*library.Library, error) {
	clean, err := registry.CleanRoot(root)
	if err != nil {
		return nil, err
	}
	conv, cerr := gameConvention(clean, "")
	if cerr != nil {
		return nil, cerr
	}
	if _, err := os.Stat(filepath.Join(clean, filepath.FromSlash(conv.ScenarioDir))); err != nil {
		return nil, fmt.Errorf("目录里没有 %s：%w", conv.ScenarioDir, err)
	}
	lib, err := library.ScanWith(clean, conv)
	if err != nil {
		return nil, err
	}
	if len(lib.CGs)+len(lib.Scenes)+len(lib.Chapters) == 0 {
		return nil, fmt.Errorf("没有识别到可回放事件（当前仅支持 KiriKiri 布局：raw\\data\\scenario）")
	}
	c, _ := a.loadConf()
	reg, err := a.loadRegistry(c)
	if err != nil {
		return nil, err
	}
	id := registry.IDFor(clean)
	libPath := registry.LibraryPath(dataDir(), id)
	if old := reg.Find(id); old != nil && strings.TrimSpace(old.Library) != "" {
		libPath = old.Library
	}
	g, _ := reg.Ensure(clean, libPath)
	reg.Active = g.ID
	if err := lib.Save(g.Library); err != nil {
		return nil, err
	}
	if err := reg.Save(a.registryPath()); err != nil {
		return nil, err
	}
	if err := a.syncActiveConf(g); err != nil {
		return nil, err
	}
	a.syncCatalog(g, lib)
	return lib, nil
}

// SetActiveGame 切换当前游戏（读该游戏自己的库清单；失效则重扫）。
func (a *App) SetActiveGame(id string) (*library.Library, error) {
	c, _ := a.loadConf()
	reg, err := a.loadRegistry(c)
	if err != nil {
		return nil, err
	}
	g := reg.Find(strings.TrimSpace(id))
	if g == nil {
		return nil, fmt.Errorf("游戏 %s 未登记", id)
	}
	if _, err := os.Stat(library.ScenarioDir(g.Root)); err != nil {
		return nil, fmt.Errorf("游戏「%s」根目录不可用：%w", g.Title, err)
	}
	if g.Library == "" {
		g.Library = registry.LibraryPath(dataDir(), g.ID)
	}
	lib, err := a.loadOrScanLib(g)
	if err != nil {
		return nil, err
	}
	reg.Active = g.ID
	reg.Touch(g.ID)
	if err := reg.Save(a.registryPath()); err != nil {
		return nil, err
	}
	if err := a.syncActiveConf(g); err != nil {
		return nil, err
	}
	a.syncCatalog(g, lib)
	return lib, nil
}

// GetLibrary 读当前游戏的库：有清单读清单，没有就扫描建库并落盘。
// 任何失败原样上抛给前端横幅（不静默降级）。
func (a *App) GetLibrary() (*library.Library, error) {
	c, _ := a.loadConf()
	reg, g, err := a.activeGame(c)
	if err != nil {
		return nil, err
	}
	if g.Library == "" {
		g.Library = registry.LibraryPath(dataDir(), g.ID)
		if err := reg.Save(a.registryPath()); err != nil {
			return nil, err
		}
	}
	lib, err := a.loadOrScanLib(g)
	if err != nil {
		return nil, err
	}
	if err := a.syncActiveConf(g); err != nil {
		return nil, err
	}
	a.syncCatalog(g, lib)
	return lib, nil
}

// Rescan 强制重扫当前游戏（网格刷新按钮/设置页用）。
func (a *App) Rescan() (*library.Library, error) {
	c, _ := a.loadConf()
	_, g, err := a.activeGame(c)
	if err != nil {
		return nil, err
	}
	conv, cerr := gameConvention(g.Root, g.Engine)
	if cerr != nil {
		return nil, cerr
	}
	lib, err := library.ScanWith(g.Root, conv)
	if err != nil {
		return nil, err
	}
	if g.Library == "" {
		g.Library = registry.LibraryPath(dataDir(), g.ID)
	}
	if err := lib.Save(g.Library); err != nil {
		return nil, err
	}
	if err := a.syncActiveConf(g); err != nil {
		return nil, err
	}
	a.syncCatalog(g, lib)
	return lib, nil
}

// GetCover CG 卡封面（懒加载 + 磁盘缓存 covers/<game>/<id>.png，返回 data URL 交给 <img>）。
// 只用 Pages[0] 原图裁 55:31 比例，不做整段合成（网格打开要快）。
// 场景/段落暂无封面（前端用占位渐变），M3 回放页复用本函数做抽帧封面。
// 跨游戏视图（全部游戏卡片墙）用 GetCoverForGame 指定游戏。
func (a *App) GetCover(kind, id string) (string, error) {
	c, _ := a.loadConf()
	_, g, err := a.activeGame(c)
	if err != nil {
		return "", err
	}
	return a.coverForGame(g, kind, id)
}

// GetCoverForGame 与 GetCover 同逻辑，但按 gameID 路由（跨游戏卡片墙用）。
func (a *App) GetCoverForGame(gameID, kind, id string) (string, error) {
	c, _ := a.loadConf()
	reg, err := a.loadRegistry(c)
	if err != nil {
		return "", err
	}
	g := reg.Find(strings.TrimSpace(gameID))
	if g == nil {
		return "", fmt.Errorf("游戏 %s 未登记", gameID)
	}
	return a.coverForGame(g, kind, id)
}

// coverForGame 封面主逻辑：缓存目录按游戏分（不同游戏 CG 槽位会重名，如 cg017）。
func (a *App) coverForGame(g *registry.Game, kind, id string) (string, error) {
	// 封面缓存按游戏分目录：不同游戏的 CG 槽位会重名（cg017）。
	dir := filepath.Join(dataDir(), "covers", safeName(g.ID))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, safeName(kind)+"_"+safeName(id)+".png")
	if _, err := os.Stat(path); err != nil {
		lib, err := a.loadOrScanLib(g)
		if err != nil {
			return "", err
		}
		load := replay.GameLoader(g.Root)
		var rel string
		switch kind {
		case "cg":
			cg := lib.FindCG(id)
			switch {
			case cg == nil:
				return "", fmt.Errorf("找不到 CG 事件 %s", id)
			case len(cg.Pages) > 0:
				rel = cg.Pages[0]
			case cg.Thumb != "":
				rel = cg.Thumb
			default:
				return "", fmt.Errorf("CG 事件 %s 没有页面图", id)
			}
		default:
			return "", fmt.Errorf("暂不支持 %s 封面", kind)
		}
		src, err := load(rel)
		if err != nil {
			return "", fmt.Errorf("封面图 %s 加载失败：%w", rel, err)
		}
		if err := saveRatioCrop(src, path, 440, 248); err != nil {
			return "", err
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(data), nil
}

// loadOrScanLib 读该游戏的库清单，读不到（或 root 已变）就地扫描并回写。
func (a *App) loadOrScanLib(g *registry.Game) (*library.Library, error) {
	if g.Library != "" {
		if lib, err := library.Load(g.Library); err == nil && lib.Root == g.Root {
			return lib, nil
		}
	}
	if strings.TrimSpace(g.Root) == "" {
		return nil, fmt.Errorf("游戏「%s」没有根目录", g.Title)
	}
	conv, cerr := gameConvention(g.Root, g.Engine)
	if cerr != nil {
		return nil, cerr
	}
	lib, err := library.ScanWith(g.Root, conv)
	if err != nil {
		return nil, err
	}
	if g.Library != "" {
		if err := lib.Save(g.Library); err != nil {
			return nil, err
		}
	}
	return lib, nil
}

// safeName 去掉路径分隔符，防封面缓存名穿目录。
func safeName(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			return '_'
		}
		return r
	}, s)
}

// saveRatioCrop 中心裁到 targetW:targetH 比例再缩放，PNG 落盘。
func saveRatioCrop(src image.Image, path string, targetW, targetH int) error {
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	tr := float64(targetW) / float64(targetH)
	sr := float64(sw) / float64(sh)
	cw, chh := sw, sh
	if sr > tr { // 太宽 → 裁两边
		cw = int(tr*float64(sh) + 0.5)
	} else { // 太高 → 裁上下
		chh = int(float64(sw)/tr + 0.5)
	}
	ox, oy := (sw-cw)/2, (sh-chh)/2

	out := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
	for y := 0; y < targetH; y++ {
		sy := oy + y*chh/targetH
		for x := 0; x < targetW; x++ {
			newOut := src.At(b.Min.X+ox+x*cw/targetW, b.Min.Y+sy)
			out.Set(x, y, newOut)
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, out)
}
