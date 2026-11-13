// cover.go —— 卡片封面：沿事件时间轴采样、选帧、缓存与回退。
//
// 只负责封面派生物；播放/合成仍走 player.Player + player.Compose，不复制
// replaySession。封面缓存文件名带算法版本，旧版黑图自然失效。
package main

import (
	"encoding/json"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"ucgr/internal/adapter/rmmz"
	"ucgr/internal/adapter/unitycg"
	"ucgr/internal/cover"
	"ucgr/internal/ir"
	"ucgr/internal/library"
	"ucgr/internal/player"
	"ucgr/internal/registry"
	"ucgr/internal/replay"
)

const coverW, coverH = 440, 248

// coverMeta 与 PNG 同目录的缓存元数据。缺 meta / 版本不符一律视为失效，
// 这样升级算法后不会继续显示旧缓存里的入场黑屏。
type coverMeta struct {
	Version   int     `json:"version"`
	Algorithm string  `json:"algorithm"`
	Valid     bool    `json:"valid"`
	Fallback  bool    `json:"fallback"`
	Score     float64 `json:"score"`
	AtMs      int64   `json:"at_ms"`
	Samples   int     `json:"samples"`
	Source    string  `json:"source,omitempty"`
	Reason    string  `json:"reason,omitempty"`
	CreatedAt string  `json:"created_at"`
}

func coverCachePath(dir, kind, id string) string {
	return filepath.Join(dir, fmt.Sprintf("%s_%s_v%d.png",
		safeName(kind), safeName(id), cover.Version))
}

func coverMetaPath(pngPath string) string {
	return strings.TrimSuffix(pngPath, filepath.Ext(pngPath)) + ".json"
}

func readCoverMeta(pngPath string) (coverMeta, error) {
	var m coverMeta
	data, err := os.ReadFile(coverMetaPath(pngPath))
	if err != nil {
		return m, err
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return m, err
	}
	return m, nil
}

func writeCoverMeta(pngPath string, m coverMeta) error {
	tmp := coverMetaPath(pngPath) + fmt.Sprintf(".tmp-%d", time.Now().UnixNano())
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	_ = os.Remove(coverMetaPath(pngPath))
	return os.Rename(tmp, coverMetaPath(pngPath))
}

func coverCacheUsable(pngPath string) bool {
	st, err := os.Stat(pngPath)
	if err != nil || st.IsDir() || st.Size() == 0 {
		return false
	}
	m, err := readCoverMeta(pngPath)
	if err != nil {
		return false
	}
	return m.Version == cover.Version && m.Algorithm == cover.Algorithm
}

func saveCoverPNGAtomic(src image.Image, path string) error {
	tmp := path + fmt.Sprintf(".tmp-%d", time.Now().UnixNano())
	if err := savePNG(src, tmp); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	_ = os.Remove(path)
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

var coverLocks sync.Map // path → *sync.Mutex

func lockCover(path string) func() {
	v, _ := coverLocks.LoadOrStore(path, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// buildCover 生成封面原图与元数据。时间轴采样失败时回退到页面图/最大
// layer_show 素材；回退也失败才返回错误（前端显示“无封面”，不阻断卡片墙）。
func (a *App) buildCover(g *registry.Game, lib *library.Library, kind, id string) (image.Image, coverMeta, error) {
	load := player.Cached(player.Loader(replay.GameLoader(g.Root)))
	ev, sw, sh, evErr := buildCoverEvent(g, lib, kind, id)
	refs := coverFallbackRefs(g, lib, kind, id, ev)
	// 采样超时后渲染协程可能还在读 load；兜底解码改用独立 loader，避免
	// 两个协程同时写 player.Cached 的 map。
	fallbackLoad := player.Cached(player.Loader(replay.GameLoader(g.Root)))

	if evErr == nil && ev != nil && len(ev.Show) > 0 {
		cfg := cover.DefaultConfig()
		cfg.StageW, cfg.StageH = sw, sh
		r := cover.RenderEvent(ev, load, cfg)
		meta := coverMeta{
			Version:   cover.Version,
			Algorithm: cover.Algorithm,
			Valid:     r.Valid,
			Fallback:  r.Fallback,
			Score:     r.Metrics.Score,
			AtMs:      r.AtMs,
			Samples:   r.Sampled,
			Source:    "timeline",
			Reason:    r.Reason,
			CreatedAt: time.Now().Format(time.RFC3339),
		}
		if r.Valid && r.Image != nil {
			return r.Image, meta, nil
		}
		// 时间轴全是黑帧：回退到静态首图/最大 layer_show，依然要打角标。
		if pick, ok := pickDirectImage(refs, fallbackLoad); ok {
			meta.Fallback = true
			meta.Source = "fallback:" + pick.ref
			meta.Score = pick.metrics.Score
			meta.AtMs = 0
			if meta.Reason == "" {
				meta.Reason = "all_sampled_frames_black"
			}
			return pick.img, meta, nil
		}
		return nil, meta, fmt.Errorf("封面采样无有效帧且无可解码回退图（%s）", r.Reason)
	}

	// 台本构建失败也必须尽量出图：直接解码候选静态图，成功但仍黑时才标记。
	if pick, ok := pickDirectImage(refs, fallbackLoad); ok {
		reason := ""
		if evErr != nil {
			reason = "event_build_failed: " + evErr.Error()
		} else {
			reason = "empty_show"
		}
		m := coverMeta{
			Version:   cover.Version,
			Algorithm: cover.Algorithm,
			Valid:     pick.metrics.Valid(),
			Fallback:  !pick.metrics.Valid(),
			Score:     pick.metrics.Score,
			Samples:   0,
			Source:    "direct:" + pick.ref,
			Reason:    reason,
			CreatedAt: time.Now().Format(time.RFC3339),
		}
		return pick.img, m, nil
	}
	if evErr != nil {
		return nil, coverMeta{}, evErr
	}
	return nil, coverMeta{}, fmt.Errorf("事件 %s 没有可用的封面素材", id)
}

type directPick struct {
	img     image.Image
	metrics cover.Metrics
	ref     string
}

func pickDirectImage(refs []string, load player.Loader) (directPick, bool) {
	var best directPick
	found := false
	for _, ref := range refs {
		if strings.TrimSpace(ref) == "" {
			continue
		}
		img, err := load(ref)
		if err != nil {
			continue
		}
		m := cover.Analyze(img)
		if !found || m.Score > best.metrics.Score {
			best = directPick{img: img, metrics: m, ref: ref}
			found = true
		}
	}
	return best, found
}

// coverFallbackRefs 构造静态兜底候选：
//   - cg：所有 Pages，再退 Thumb（Pages[0] 可能也是黑幕）；
//   - scene/chapter：台本里按显示面积从大到小的 layer_show 素材；
//   - Unity：索引里的帧序列。
func coverFallbackRefs(g *registry.Game, lib *library.Library, kind, id string, ev *ir.Event) []string {
	var out []string
	seen := map[string]bool{}
	add := func(ref string) {
		ref = strings.TrimSpace(ref)
		if ref == "" || seen[ref] {
			return
		}
		seen[ref] = true
		out = append(out, ref)
	}
	switch kind {
	case "cg":
		if cg := lib.FindCG(id); cg != nil {
			for _, p := range cg.Pages {
				add(p)
			}
			add(cg.Thumb)
		}
	case "scene":
		if strings.EqualFold(strings.TrimSpace(lib.EngineID), "unity") {
			if cg, err := library.BuildUnityEvent(g.Root, id); err == nil {
				frames := cg.Frames
				if len(frames) == 0 {
					frames = cg.Files.PNG
				}
				for _, f := range frames {
					add(unitycg.Ref(cg.ID, f))
				}
			}
		}
	}
	if ev != nil {
		for _, ref := range layerShowRefs(ev) {
			add(ref)
		}
	}
	return out
}

type layerRef struct {
	ref  string
	area float64
	ord  int
}

func layerShowRefs(ev *ir.Event) []string {
	var cands []layerRef
	for i, op := range ev.Show {
		if op.Kind != ir.ShowLayerShow || strings.TrimSpace(op.Ref) == "" {
			continue
		}
		w := attrFloat(op.Attr, "width", "w")
		h := attrFloat(op.Attr, "height", "h")
		if w <= 0 || h <= 0 {
			// 未声明几何时仍保留，排在有面积信息之后。
			w, h = 1, 1
		}
		area := w * h
		if s := attrFloat(op.Attr, "scale"); s > 0 && s != 1 {
			area *= s * s
		}
		cands = append(cands, layerRef{ref: op.Ref, area: area, ord: i})
	}
	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].area == cands[j].area {
			return cands[i].ord < cands[j].ord
		}
		return cands[i].area > cands[j].area
	})
	seen := map[string]bool{}
	out := make([]string, 0, len(cands))
	for _, c := range cands {
		if seen[c.ref] {
			continue
		}
		seen[c.ref] = true
		out = append(out, c.ref)
	}
	if len(out) == 0 {
		for _, a := range ev.Assets {
			if strings.TrimSpace(a.OrigPath) != "" {
				out = append(out, a.OrigPath)
			}
		}
	}
	return out
}

func attrFloat(attrs map[string]string, keys ...string) float64 {
	for _, k := range keys {
		if v, ok := attrs[k]; ok {
			if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
				return f
			}
		}
	}
	return 0
}

// buildCoverEvent 复用壳层 eventbuild.go 的引擎分派入口构建台本，返回原生舞台尺寸。
// 不调用 OpenEvent（那会改最近播放/创建会话），只借同一批 builder。
func buildCoverEvent(g *registry.Game, lib *library.Library, kind, id string) (*ir.Event, int, int, error) {
	conv, _ := gameConvention(g.Root, g.Engine)
	sw, sh := defaultStageW, defaultStageH
	if conv.StageW > 0 && conv.StageH > 0 {
		sw, sh = conv.StageW, conv.StageH
	} else if lib.StageW > 0 && lib.StageH > 0 {
		sw, sh = lib.StageW, lib.StageH
	}

	var script string
	switch kind {
	case "cg":
		cg := lib.FindCG(id)
		if cg == nil {
			return nil, sw, sh, fmt.Errorf("找不到 CG 事件 %s", id)
		}
		script = cg.Script
	case "scene":
		scene := lib.FindScene(id)
		if scene == nil {
			return nil, sw, sh, fmt.Errorf("找不到场景 %s", id)
		}
		script = scene.Script
	case "chapter":
		chapter := lib.FindChapter(id)
		if chapter == nil {
			return nil, sw, sh, fmt.Errorf("找不到演出段落 %s", id)
		}
		script = chapter.Script
	default:
		return nil, sw, sh, fmt.Errorf("暂不支持 %s 封面", kind)
	}

	engine := strings.ToLower(strings.TrimSpace(lib.EngineID))
	if engine == "" {
		engine = strings.ToLower(strings.TrimSpace(g.Engine))
	}
	if engine == "" {
		engine = strings.ToLower(strings.TrimSpace(conv.EngineID))
	}
	var ev *ir.Event
	var err error
	switch {
	case engine == "unity":
		var ucg unitycg.CG
		ucg, err = library.BuildUnityEvent(g.Root, id)
		if err == nil {
			ev, err = unitycg.BuildShow(ucg)
		}
	case engine == "rmmz":
		ev, err = rmmz.BuildShow(g.Root, filepath.ToSlash(filepath.Dir(script)), id)
	default:
		// scene/chapter/cg 统一走壳层既有构建入口，避免封面另写一份引擎分派。
		ev, _, err = buildEvent(g.Root, engine, conv, lib, kind, id)
	}
	if err != nil {
		return nil, sw, sh, err
	}
	if ev == nil {
		return nil, sw, sh, fmt.Errorf("事件 %s 没有构建出演出台本", id)
	}
	return ev, sw, sh, nil
}
