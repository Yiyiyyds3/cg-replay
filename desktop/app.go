// 绑定层：把 core 能力暴露给 React 前端（方案 B M2：事件网格卡墙）。
//
// 职责边界（docs/06 / 文稿 §红线）：本壳不做任何合成/解码/索引逻辑，
// 只做「桥 + 薄缓存」；封面是唯一派生物，只写 exe 同目录 covers/（DR-30）。
// conf 与 CLI（cmd/cgreplay）共用同目录的 cgreplay.conf：两边互通。
package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"ucgr/internal/library"
	"ucgr/internal/replay"
)

// exeDir 返回 exe 所在目录（conf/covers/library.json 都在这里）。
func exeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

// conf 记忆上次的选择，与 cmd/cgreplay 同格式同文件。
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

// loadConf 读 exe 同目录 conf；不存在返回零值 + missing 标记（不静默）。
func (a *App) loadConf() (conf, bool) {
	var c conf
	data, err := os.ReadFile(filepath.Join(exeDir(), confName))
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
	path := filepath.Join(exeDir(), confName)
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

// SetRoot 保存根目录并立即建库，返回库（卡片网格直接用）。
func (a *App) SetRoot(root string) (*library.Library, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, fmt.Errorf("根目录为空")
	}
	if _, err := os.Stat(library.ScenarioDir(root)); err != nil {
		return nil, fmt.Errorf("目录里没有 raw\\data\\scenario：%w", err)
	}
	c, _ := a.loadConf()
	c.Root = root
	c.Library = filepath.Join(exeDir(), "library.json")
	if err := a.saveConf(c); err != nil {
		return nil, err
	}
	lib, err := library.Scan(root)
	if err != nil {
		return nil, err
	}
	if err := lib.Save(c.Library); err != nil {
		return nil, err
	}
	return lib, nil
}

// GetLibrary 读库：有清单读清单，没有就扫描建库并落盘。
// 任何失败原样上抛给前端横幅（不静默降级）。
func (a *App) GetLibrary() (*library.Library, error) {
	c, ok := a.loadConf()
	if !ok {
		c.Library = filepath.Join(exeDir(), "library.json")
	}
	// ① 优先读清单；root 已变则失效重扫。
	if c.Library != "" {
		if lib, err := library.Load(c.Library); err == nil && (c.Root == "" || lib.Root == c.Root) {
			if c.Root != "" && lib.Root != "" {
				return lib, nil
			}
		}
	}
	// ② 无清单 → 扫根目录建库。
	if c.Root == "" {
		c.Root = exeDir()
		if _, err := os.Stat(library.ScenarioDir(c.Root)); err != nil {
			return nil, fmt.Errorf("未设置游戏根目录（conf 缺失），请先选择解包目录")
		}
	}
	c.Root = strings.TrimSpace(c.Root)
	lib, err := library.Scan(c.Root)
	if err != nil {
		return nil, err
	}
	if c.Library == "" {
		c.Library = filepath.Join(exeDir(), "library.json")
	}
	if err := a.saveConf(c); err != nil {
		return nil, err
	}
	if err := lib.Save(c.Library); err != nil {
		return nil, err
	}
	return lib, nil
}

// Rescan 强制重扫（网格刷新按钮/设置页用）。
func (a *App) Rescan() (*library.Library, error) {
	c, ok := a.loadConf()
	if !ok || c.Root == "" {
		return nil, fmt.Errorf("未设置游戏根目录，请先选择解包目录")
	}
	lib, err := library.Scan(c.Root)
	if err != nil {
		return nil, err
	}
	if c.Library == "" {
		c.Library = filepath.Join(exeDir(), "library.json")
	}
	if err := lib.Save(c.Library); err != nil {
		return nil, err
	}
	return lib, nil
}

// GetCover CG 卡封面（懒加载 + 磁盘缓存 covers/<id>.png，返回 data URL 交给 <img>）。
// 只用 Pages[0] 原图裁 55:31 比例，不做整段合成（网格打开要快）。
// 场景/段落暂无封面（前端用占位渐变），M3 回放页复用本函数做抽帧封面。
func (a *App) GetCover(kind, id string) (string, error) {
	c, ok := a.loadConf()
	if !ok || c.Root == "" {
		return "", fmt.Errorf("未设置游戏根目录")
	}
	dir := filepath.Join(exeDir(), "covers")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, safeName(kind)+"_"+safeName(id)+".png")
	if _, err := os.Stat(path); err != nil {
		lib, err := a.loadOrScanLib(c)
		if err != nil {
			return "", err
		}
		load := replay.GameLoader(c.Root)
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

// loadOrScanLib 读库清单，读不到（或 root 已变）就地扫描兜底。
func (a *App) loadOrScanLib(c conf) (*library.Library, error) {
	if c.Library != "" {
		if lib, err := library.Load(c.Library); err == nil && (c.Root == "" || lib.Root == c.Root) {
			return lib, nil
		}
	}
	if c.Root == "" {
		return nil, fmt.Errorf("未设置游戏根目录")
	}
	return library.Scan(c.Root)
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
			newOut := src.At(b.Min.X + ox + x*cw/targetW, b.Min.Y + sy)
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
