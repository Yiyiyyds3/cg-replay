// media —— 「视频/图片为主」游戏的事件库直连适配器。
//
// 适用形态：游戏没有可解析的剧本（或剧本不值得解析），CG 以编号图片、
// 影片目录的形式直接摆在资源目录里。实测：
//   - cocos2d-js（逃离深渊的少女）：Resources/img/001.png… + Movies/002.mp4
//   - Electron/web（Touch Simulation Game）：dist/assets/{images,video} +
//     dist/data/scenarios/*.csv（CSV 事件分组暂不解析，按目录归组）
//
// 事件单位：一张图 = 一个 still 事件；一个影片 = 一个 video 事件。
// 尺寸门槛（默认图 64KiB / 影片 1MiB）把 UI 图标、按钮图挡在外面；
// 引擎/系统目录（node_modules、saves、cache…）整棵跳过。
package media

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"ucgr/internal/decode"
	"ucgr/internal/ir"
)

// EngineID 引擎 id（约定文件 / 事件库都用它分派）。
const EngineID = "media"

// 默认尺寸门槛。
const (
	MinImageBytes = 64 << 10 // 64 KiB：CG 量级（UI 图标远小于此）
	MinVideoBytes = 1 << 20  // 1 MiB
)

var imageExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".webp": true, ".bmp": true, ".gif": true,
}

// skipDirs 引擎/系统目录（按目录名匹配，大小写不敏感），整棵跳过。
var skipDirs = map[string]bool{
	"node_modules": true, "electron": true, "locales": true, "saves": true,
	"save": true, "cache": true, "tl": true, ".git": true, ".ucgr-cache": true,
	"logs": true, "crashpad": true, "system": true,
}

// Ref 一个可回放的媒体事件。
type Ref struct {
	ID    string // 事件 id：路径茎（斜杠转下划线）
	Path  string // 相对 root 的斜杠路径
	Kind  string // still / video
	Title string // 展示名：分组目录 · 文件名
	Bytes int64
}

// ScanResult 一次扫描的结果与诊断。
type ScanResult struct {
	Root     string
	Refs     []Ref
	Warnings []string
	Files    int // 走过的媒体文件总数（含被门槛挡掉的）
}

// Scan 扫一个游戏目录，列出图片/影片事件。
func Scan(root string) (*ScanResult, error) {
	res := &ScanResult{Root: root}
	type byPath []Ref
	var stills, videos byPath
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if skipDirs[strings.ToLower(d.Name())] {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		media := imageExts[ext] || ir.IsVideoRef(d.Name())
		if !media {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		res.Files++
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		group := filepath.ToSlash(filepath.Dir(rel))
		group = group[strings.Index(group, "/")+1:] // 去掉第一段（Resources/dist 之类的壳）
		if i := strings.LastIndex(group, "/"); i >= 0 {
			group = group[i+1:] // 只留最后一级目录做分组名
		}
		ref := Ref{
			ID:    strings.ReplaceAll(strings.TrimSuffix(rel, filepath.Ext(rel)), "/", "_"),
			Path:  rel,
			Title: group + " · " + strings.TrimSuffix(d.Name(), filepath.Ext(d.Name())),
		}
		switch {
		case ir.IsVideoRef(d.Name()):
			if info.Size() < MinVideoBytes {
				return nil
			}
			ref.Kind = "video"
			ref.Bytes = info.Size()
			videos = append(videos, ref)
		case info.Size() >= MinImageBytes:
			ref.Kind = "still"
			ref.Bytes = info.Size()
			stills = append(stills, ref)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("遍历目录失败：%w", err)
	}
	sort.Slice(stills, func(i, j int) bool { return stills[i].Path < stills[j].Path })
	sort.Slice(videos, func(i, j int) bool { return videos[i].Path < videos[j].Path })
	res.Refs = append(append(res.Refs, stills...), videos...)
	if len(res.Refs) == 0 {
		hint := ""
		if res.Files > 0 {
			hint = fmt.Sprintf("（走过 %d 个媒体文件，全被尺寸门槛挡掉；素材锁在 asar/pck 归档里时也扫不到）", res.Files)
		}
		return nil, fmt.Errorf("没有扫到可回放的图片/影片%s", hint)
	}
	return res, nil
}

// FindRef 按 id 找事件。
func FindRef(res *ScanResult, id string) (Ref, bool) {
	for _, r := range res.Refs {
		if r.ID == id {
			return r, true
		}
	}
	return Ref{}, false
}

// BuildShow 把一个媒体事件翻成 IR 台本。
func BuildShow(root string, id string) (*ir.Event, error) {
	res, err := Scan(root)
	if err != nil {
		return nil, err
	}
	ref, ok := FindRef(res, id)
	if !ok {
		return nil, fmt.Errorf("媒体库里没有事件 %s", id)
	}
	ev := &ir.Event{
		EventID: ref.ID,
		Kind:    ir.KindScene,
		Title:   ref.Title,
		Assets:  []ir.AssetRef{{OrigPath: ref.Path, Role: ref.Kind}},
	}
	switch ref.Kind {
	case "video":
		dur := mp4DurationMs(filepath.Join(root, filepath.FromSlash(ref.Path)))
		if dur <= 0 {
			dur = 15000 // 解不出时长（非 mp4/坏文件）时的保底
		}
		ev.Show = append(ev.Show,
			ir.ShowOp{TMs: 0, Kind: ir.ShowVideoPlay, Layer: "layer0", Ref: ref.Path},
			ir.ShowOp{TMs: dur, Kind: ir.ShowWait, DurMs: 500},
		)
		ev.EndMs = dur + 500
	default:
		step := int64(4000) // 单图停留 4 秒（CG 鉴赏节奏）
		ev.Show = append(ev.Show, layerShowOp(root, ref.Path, 0), ir.ShowOp{TMs: 0, Kind: ir.ShowWait, DurMs: step})
		ev.EndMs = step
	}
	return ev, nil
}

// layerShowOp 产一条居中适配 1600×1200 舞台的 layer_show（尺寸读不出就按 1920×1080 估）。
func layerShowOp(root, rel string, tms int64) ir.ShowOp {
	w, h := 0, 0
	if data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel))); err == nil {
		if img, derr := decode.DecodeImage(data); derr == nil {
			b := img.Bounds()
			w, h = b.Dx(), b.Dy()
		}
	}
	if w <= 0 || h <= 0 {
		w, h = 1920, 1080
	}
	fx := 1600.0 / float64(w)
	fy := 1200.0 / float64(h)
	s := fx
	if fy < s {
		s = fy
	}
	left := (1600 - float64(w)*s) / 2
	top := (1200 - float64(h)*s) / 2
	return ir.ShowOp{
		TMs: tms, Kind: ir.ShowLayerShow, Layer: "layer0", Ref: rel,
		Attr: map[string]string{
			"left":  fmt.Sprintf("%.1f", left),
			"top":   fmt.Sprintf("%.1f", top),
			"scale": fmt.Sprintf("%.6f", s),
		},
	}
}

// mp4DurationMs 从 mp4 的 moov/mvhd 里读时长（毫秒）。读不到返回 0。
func mp4DurationMs(path string) int64 {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	// mvhd：version0 = ver/flags@4(4B) + creation@8(4B) + mod@12(4B) +
	//        timescale@16(4B) + duration@20(4B)；
	//        version1 = creation@8(8B) + mod@16(8B) + timescale@24(4B) +
	//        duration@28(8B)。
	i := 0
	for {
		j := indexFrom(data, []byte("mvhd"), i)
		if j < 0 || j+32 > len(data) {
			return 0
		}
		v := data[j+4]
		if v == 1 {
			if j+36 > len(data) {
				return 0
			}
			timescale := binary.BigEndian.Uint32(data[j+24 : j+28])
			duration := binary.BigEndian.Uint64(data[j+28 : j+36])
			if timescale == 0 {
				return 0
			}
			return int64(duration) * 1000 / int64(timescale)
		}
		timescale := binary.BigEndian.Uint32(data[j+16 : j+20])
		duration := binary.BigEndian.Uint32(data[j+20 : j+24])
		if timescale == 0 {
			return 0
		}
		return int64(duration) * 1000 / int64(timescale)
	}
}

func indexFrom(data []byte, pat []byte, from int) int {
	if from < 0 {
		from = 0
	}
	limit := len(data) - len(pat)
	for i := from; i <= limit; i++ {
		if data[i] == pat[0] && string(data[i:i+len(pat)]) == string(pat) {
			return i
		}
	}
	return -1
}
