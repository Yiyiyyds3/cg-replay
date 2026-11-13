// 回放页会话层（方案 B M3）：事件卡 → IR 台本 → player.Player → JPEG 帧流。
//
// 语义对齐 cmd/cgreplay：IR 构建入口（BuildShowSegment / BuildShowChapterFile）、
// 播放策略（DefaultOptions + FontSize=48）、loader（Cached + GameLoader）、
// 自动推进节奏（停留 > autoWaitMs 注入一次 Input.Advance）。
//
// 两个壳层差异（不碰内核，diff=0 红线不受影响——逐帧回归口径走 CLI 路径）：
//   - 帧给前端是 JPEG dataURL（舞台底为黑、恒不透明，alpha 对本用途无意义）；
//   - 缺素材不中止整帧：loader 记账后回 1×1 透明占位，画面继续叠，
//     缺清单常驻红横幅交代（FR-12 不静默；CLI 语义仍是遇缺中止整帧）。
package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"ucgr/internal/adapter"
	"ucgr/internal/adapter/artemis"
	"ucgr/internal/adapter/bgi"
	"ucgr/internal/adapter/catsystem2"
	"ucgr/internal/adapter/kirikiri"
	"ucgr/internal/adapter/rmmz"
	"ucgr/internal/adapter/unitycg"
	"ucgr/internal/audio"
	"ucgr/internal/catalog"
	"ucgr/internal/ir"
	"ucgr/internal/library"
	"ucgr/internal/player"
	"ucgr/internal/replay"
	"ucgr/internal/video"
)

const (
	// defaultStageW/H 台本未声明画布尺寸时的默认舞台（KiriKiri 本作 4:3）。
	// 编译剧本（scn/9nine）会在 IR 里带 1280×720，按事件走。
	defaultStageW = 1600
	defaultStageH = 1200
	autoWaitMs    = 2500 // 与 cmd/cgreplay 一致
	jpegQuality   = 88
	encMinGap     = 50 * time.Millisecond // 播放中两次编码的最小间隔（≈20fps 封顶）
	videoFPS      = 20                    // 影片流的目标帧率（与渲染上限一致）

	durSnapshots = 240 // 时长估计取样点数
)

// replaySession 当前回放会话（同一时刻最多一个）。
type replaySession struct {
	mu    sync.Mutex
	root  string
	kind  string // cg / scene / chapter
	id    string
	title string
	// gameID 会话所属游戏（SessionMeta 回传前端做收藏/跨游戏定位）。
	gameID string

	// mode 当前模式（story / gallery，非 CG 为空）。
	// galleryEv/storyEv 同时留在内存里：前端「剧情 / 鉴赏」切换只换播放器，
	// 不重新解析剧本；dialogue 是给前端的台词数据（含「无台词」的如实说明）。
	mode      string
	galleryEv *ir.Event
	storyEv   *ir.Event
	dialogue  *DialogueDTO

	// voiceRef/voiceAt 最近一次语音 op（frameLocked 透传给前端做状态指示）。
	voiceRef string
	voiceAt  int64

	// sw/sh 本会话的渲染画布（= 事件声明的原生舞台；素材按原生坐标合成，
	// 画布尺寸必须匹配，否则素材会挤在左上角）。
	sw, sh int

	ev   *ir.Event
	opts player.Options
	p    *player.Player

	load  player.Loader            // 缺素材记账版 loader
	vs    map[string]*video.Stream // 每条影片一条流（惰性打开，按路径缓存）
	audio *audio.System

	// spine 非空：这是一部 Spine 骨骼动画，Go 侧不出 JPEG，画面由前端 canvas 渲染。
	spine *SpineDTO

	playing  bool
	auto     bool
	muted    bool
	input    player.Input
	lastTick time.Time
	waited   bool // 正停在等待点（自动推进计时用）
	waitedAt time.Time

	missing map[string]string // ref → 最近一次加载错误

	lastSig   string // 上次编码的画面签名；不变就直接复用
	lastJPEG  string
	lastDrawn []string  // 上次编码的实际绘制清单（含影片/缺素材诊断）
	lastEnc   time.Time // 上次编码时刻（播放中节流，防 16ms 轮询饿死锁）

	// snap 最近一张完整帧快照。s.mu 太忙时 TryLock 不住就直接回它，
	// 避免轮询请求线性堆积把 mutex 排成分钟级队列（2026-10-21 实证：
	// waiter 累到 400+、ToggleAuto 排队 60s+）。
	snapMu   sync.RWMutex
	snapLast FrameDTO
	snapOK   bool

	durMs int64
}

// SessionMeta 回放会话元信息（OpenEvent 返回）。
type SessionMeta struct {
	Kind       string `json:"kind"`
	ID         string `json:"id"`
	Title      string `json:"title"`
	GameID     string `json:"game_id"` // 收藏/跨游戏定位用
	StageW     int    `json:"stage_w"`
	StageH     int    `json:"stage_h"`
	OpCount    int    `json:"op_count"`
	TextCount  int    `json:"text_count"`
	DurationMs int64  `json:"duration_ms"`
	// Spine 非空：前端用 WebGL canvas 渲染骨骼动画（Go 侧不编码 JPEG）。
	Spine *SpineDTO `json:"spine,omitempty"`
	// Mode 当前回放模式：story（剧情：主线演出，带台词/语音）/ gallery（鉴赏：纯图 CG）。
	// 非 CG 事件为空串。Modes 列出可切换的模式，>=2 时前端显示「剧情 / 鉴赏」。
	Mode  string   `json:"mode,omitempty"`
	Modes []string `json:"modes,omitempty"`
	// Dialogue CG 的台词数据（可用性 + 条数 + 无数据原因）。非 CG 事件为 nil。
	Dialogue *DialogueDTO `json:"dialogue,omitempty"`
}

// VoiceDTO 帧级语音状态。
//
// player 只把 voice_play 派给音频层，音频层不回报播放进度（voice.go 也不含时长查询），
// 所以这里给「最近一次触发的语音 + 触发时的演出时刻」，前端只当作「这句有语音」的
// 指示，不谎报播放进度与剩余时长。
type VoiceDTO struct {
	Ref     string `json:"ref"`
	SinceMs int64  `json:"since_ms"`
	Muted   bool   `json:"muted"`
}

// FrameDTO 一帧的前端所需全集。
type FrameDTO struct {
	JPEG    string     `json:"jpeg"`
	Now     int64      `json:"now"`
	DurMs   int64      `json:"dur_ms"`
	Playing bool       `json:"playing"`
	Done    bool       `json:"done"`
	Waiting bool       `json:"waiting"`
	Auto    bool       `json:"auto"`
	Muted   bool       `json:"muted"`
	QuakeX  int        `json:"quake_x"`
	QuakeY  int        `json:"quake_y"`
	Missing []string   `json:"missing"`
	Drawn   []string   `json:"drawn"` // 实际画上的层与失败诊断（含影片取帧错误）
	Layers  []LayerDTO `json:"layers"`
	Texts   []BoxDTO   `json:"texts"`
	Cached  bool       `json:"cached"`
	// Voice 最近触发的语音（nil = 本会话还没有语音）。
	Voice *VoiceDTO `json:"voice,omitempty"`
}

// LayerDTO 调试抽屉 D 的图层一览。
type LayerDTO struct {
	Layer   string  `json:"layer"`
	Ref     string  `json:"ref"`
	Visible bool    `json:"visible"`
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	Opacity float64 `json:"opacity"`
	Z       int     `json:"z"`
	Missing bool    `json:"missing"`
}

// rgbaDTO FrameColor 的 JSON 友好形态。
type rgbaDTO struct {
	R, G, B uint8
}

// BoxDTO 前端文本覆盖层（几何由后端给，前端照抄定位；框图缺席时可用底色兜底）。
type BoxDTO struct {
	Layer        string  `json:"layer"`
	X            float64 `json:"x"`
	Y            float64 `json:"y"`
	W            float64 `json:"w"`
	H            float64 `json:"h"`
	HasGeom      bool    `json:"has_geom"`
	Visible      bool    `json:"visible"`
	FrameColor   rgbaDTO `json:"frame_color"`
	FrameOpacity uint8   `json:"frame_opacity"`
	FontSize     float64 `json:"font_size"`
	LineSpacing  float64 `json:"line_spacing"`
	// 内边距与框图名：前端按原生舞台坐标排版台词（文本框底色/框图已由
	// Go 侧 RenderFrame 画进 JPEG，前端只负责写字，不能重复画底）。
	MarginL float64   `json:"margin_l"`
	MarginT float64   `json:"margin_t"`
	MarginR float64   `json:"margin_r"`
	MarginB float64   `json:"margin_b"`
	Frame   string    `json:"frame,omitempty"`
	Lines   []LineDTO `json:"lines"`
}

type LineDTO struct {
	Speaker string `json:"speaker"`
	Text    string `json:"text"`
}

// stageLoader 缺素材记账：错误收进 s.missing，回 1×1 透明让合成继续。
// FR-12 的壳层口径：缺素材横幅常驻不静默，但不至于黑掉一整帧。
func (s *replaySession) stageLoader(base player.Loader) player.Loader {
	return func(ref string) (image.Image, error) {
		img, err := base(ref)
		if err == nil {
			return img, nil
		}
		if _, seen := s.missing[ref]; !seen {
			s.missing[ref] = err.Error()
		}
		return image.NewRGBA(image.Rect(0, 0, 1, 1)), nil
	}
}

// buildArtemisShow 构建 Artemis 的 block 演出（事件 Label = block 序号）。
func buildArtemisShow(scriptPath, block string, stageW, stageH int) (*ir.Event, error) {
	idx, err := strconv.Atoi(strings.TrimSpace(block))
	if err != nil {
		return nil, fmt.Errorf("Artemis 事件缺 block 序号（%q）", block)
	}
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		return nil, err
	}
	return artemis.BuildShowBlockStage(data, filepath.Base(scriptPath), idx, stageW, stageH)
}

// buildArtemisScript 构建 Artemis 整份剧本的演出（所有 block 按块序拼接）。
func buildArtemisScript(scriptPath string, stageW, stageH int) (*ir.Event, error) {
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		return nil, err
	}
	return artemis.BuildShowScriptStage(data, filepath.Base(scriptPath), stageW, stageH)
}

// buildBGIShow 构建 BGI 的第 n 个事件演出（Label = 事件序号）。
func buildBGIShow(scriptPath, ordinal string, stageW, stageH int) (*ir.Event, error) {
	n, err := strconv.Atoi(strings.TrimSpace(ordinal))
	if err != nil {
		return nil, fmt.Errorf("BGI 事件缺序号（%q）", ordinal)
	}
	return bgi.BuildShowEventFileStage(scriptPath, n, stageW, stageH)
}

// buildCS2Show 构建 CatSystem2 的单个 .cst 演出（一个剧本 = 一个事件）。
//
// 走适配器的 ParseScript + BuildIR——与 CLI 的 astRefs/prune 同一条解析
// 路径，避免壳层另写一份命令语义。stageW/H 为 0 时用适配器默认画布。
func buildCS2Show(scriptPath string, stageW, stageH int) (*ir.Event, error) {
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		return nil, err
	}
	a := &catsystem2.Adapter{}
	ast, err := a.ParseScript(data, &adapter.ScriptCtx{EntryPath: scriptPath})
	if err != nil {
		return nil, err
	}
	cfg := map[string]string{"script_name": scriptPath}
	if stageW > 0 && stageH > 0 {
		cfg["canvas_w"] = strconv.Itoa(stageW)
		cfg["canvas_h"] = strconv.Itoa(stageH)
	}
	out, err := a.BuildIR(ast, &adapter.BuildCtx{GameID: "catsystem2", Config: cfg})
	if err != nil {
		return nil, err
	}
	if len(out.Events) == 0 {
		return nil, fmt.Errorf("CatSystem2 剧本 %s 没有产出事件", filepath.Base(scriptPath))
	}
	return &out.Events[0], nil
}

// confAuto 读 conf 里的 auto（与旧 CLI 同字段）。
func (a *App) confAuto() bool {
	c, ok := a.loadConf()
	return ok && c.Auto
}

// OpenEvent 打开事件进入回放（先关旧会话；IR 构建与 CLI play() 同口径）。
func (a *App) OpenEvent(kind, id string) (SessionMeta, error) {
	a.sessMu.Lock()
	defer a.sessMu.Unlock()
	a.closeSessionLocked()

	c, _ := a.loadConf()
	reg, g, err := a.activeGame(c)
	if err != nil {
		return SessionMeta{}, err
	}
	root := g.Root

	lib, err := a.loadOrScanLib(g)
	if err != nil {
		return SessionMeta{}, err
	}
	conv, err := gameConvention(g.Root, g.Engine)
	if err != nil {
		return SessionMeta{}, err
	}

	var script, label, title string
	chapterPos := -1
	var chEvent *library.ChapterEvent
	switch kind {
	case "cg":
		e := lib.FindCG(id)
		if e == nil {
			return SessionMeta{}, fmt.Errorf("找不到 CG 事件 %s", id)
		}
		script, label, title = e.Script, e.Label, e.Title
	case "chapter":
		e := lib.FindChapter(id)
		if e == nil {
			return SessionMeta{}, fmt.Errorf("找不到演出段落 %s", id)
		}
		script, chapterPos, title = e.Script, e.LabelPos, e.Title
		chEvent = e
	case "scene":
		e := lib.FindScene(id)
		if e == nil {
			return SessionMeta{}, fmt.Errorf("找不到场景 %s", id)
		}
		script, title = e.Script, e.Title
	default:
		return SessionMeta{}, fmt.Errorf("未知事件类型 %s", kind)
	}
	if script == "" {
		return SessionMeta{}, fmt.Errorf("%s 没有脚本信息", title)
	}
	sp := filepath.Join(root, filepath.FromSlash(script))
	// 引擎号以磁盘约定文件为准：旧登记里的 g.Engine 可能为空（历史数据），
	// 直接按空串分派会全部落到 KiriKiri 解析器。
	engineID := g.Engine
	if conv, cerr := gameConvention(root, g.Engine); cerr == nil && conv.EngineID != "" {
		engineID = conv.EngineID
	}
	// 画布尺寸：约定写了就用约定（9nine = 1280×720）；没写就用扫描时
	// 从素材推断的原作尺寸（CatSystem2/Artemis/BGI 同引擎不同作品分辨率
	// 不同）；都没有才退回默认 4:3 舞台。素材按原生坐标合成，必须一致。
	sw, sh := defaultStageW, defaultStageH
	if conv.StageW > 0 && conv.StageH > 0 {
		sw, sh = conv.StageW, conv.StageH
	} else if lib.StageW > 0 && lib.StageH > 0 {
		sw, sh = lib.StageW, lib.StageH
	}
	var ev *ir.Event
	var unityCG *unitycg.CG
	// 按**库扫出来的引擎**分派：registry 里的 Engine 可能为空，
	// 约定文件（ucgr-convention.json）才是权威。
	engine := strings.ToLower(strings.TrimSpace(lib.EngineID))
	if engine == "" {
		engine = strings.ToLower(strings.TrimSpace(g.Engine))
	}
	switch {
	case engine == "unity":
		// Unity 的 CG 没有剧本：台本由 cg-index.json + 帧序列拼出来。
		cg, cerr := library.BuildUnityEvent(root, id)
		if cerr != nil {
			return SessionMeta{}, cerr
		}
		unityCG = &cg
		ev, err = unitycg.BuildShow(cg)
	case engine == "rmmz":
		// RPG Maker：Script 是 data 目录下的 JSON，dataDir 从它反推。
		ev, err = rmmz.BuildShow(root, filepath.ToSlash(filepath.Dir(script)), id)
	case chapterPos >= 0 && chEvent != nil:
		// 走 library 入口：章跨文件时自动补演下一场景的章前段（KiriKiri 专有）。
		ev, err = library.BuildChapterEvent(root, chEvent)
	case strings.EqualFold(engineID, "artemis"):
		// Artemis：事件单位是剧本里的 block，Label 记 block 序号。
		ev, err = buildArtemisShow(sp, label, sw, sh)
	case strings.EqualFold(engineID, "bgi"):
		// BGI：事件由适配器按换背景/换事件切出，Label 记事件序号；
		// builder 已经产好 Show 台本，直接用。
		ev, err = buildBGIShow(sp, label, sw, sh)
	case strings.EqualFold(engineID, "catsystem2"):
		// CS2：一份 .cst 一个事件（无 Label），构建时把 Timeline 转成 Show。
		ev, err = buildCS2Show(sp, sw, sh)
	default:
		ev, err = kirikiri.BuildShowSegment(sp, label)
	}
	if err != nil {
		return SessionMeta{}, fmt.Errorf("构建台本失败：%w", err)
	}
	// CG：先按画廊台本构建，再尝试换成「剧情模式」（主线章节，带台词/语音）。
	// 换不动就原样留在鉴赏模式，Dialogue 里写明「该 CG 无台词数据」。
	var galleryEv, storyEv *ir.Event
	var dialogue *DialogueDTO
	if kind == "cg" {
		galleryEv = ev
		if e := lib.FindCG(id); e != nil {
			ev, dialogue = applyCGStory(ev, root, lib, e)
			if ev != galleryEv {
				storyEv = ev
			}
		}
	}
	if len(ev.Show) == 0 {
		return SessionMeta{}, fmt.Errorf("%s 没有可回放的演出（台本 0 条）", title)
	}

	// Spine 的素材必须在开会话前读齐：缺 skeleton/atlas/贴图时明确报错，
	// 不能开一个只有黑屏的会话。
	var spine *SpineDTO
	if unityCG != nil && unityCG.Kind == "spine" {
		spine, err = loadSpineAssets(root, *unityCG)
		if err != nil {
			return SessionMeta{}, fmt.Errorf("加载 Spine 素材失败：%w", err)
		}
	}

	s := &replaySession{
		root:      root,
		kind:      kind,
		id:        id,
		title:     title,
		gameID:    g.ID,
		sw:        sw,
		sh:        sh,
		ev:        ev,
		galleryEv: galleryEv,
		storyEv:   storyEv,
		dialogue:  dialogue,
		opts:      player.DefaultOptions(),
		auto:      a.confAuto(),
		playing:   true,
		missing:   map[string]string{},
		spine:     spine,
	}
	if kind == "cg" {
		if storyEv != nil {
			s.mode = "story"
		} else {
			s.mode = "gallery" // 回退纯图也是明确的鉴赏模式（前端不显示空模式）
		}
	}
	s.opts.FontSize = 48
	// 影片时长：台本里通常不写，靠 ffprobe 查；失败原因交给播放器写进诊断。
	s.opts.VideoDur = func(ref string) (int64, error) {
		p, err := replay.AssetPath(root, ref)
		if err != nil {
			return 0, err
		}
		return video.Duration(p)
	}
	s.opts.StageW, s.opts.StageH = sw, sh // 消息层默认几何按舞台底部排
	s.load = s.stageLoader(player.Cached(player.Loader(replay.GameLoader(root))))
	s.p = player.New(ev, s.opts)
	if sys, aerr := audio.NewSystem(root); aerr == nil {
		s.audio = sys
		s.p.Audio = s.audioHandle(sys.Handle)
	}
	s.p.Advance(0, player.Input{}) // 先把 t=0 的台本指令执行掉（同 play()）
	s.lastTick = time.Now()

	// 时长口径与 CLI 一致：Show 台本的脚本时钟最大值（TMs 只被
	// [wait]/[wt]/[trans]/fade 推动，作进度条参考刻度；阅读节奏由
	// 播放器自动推进策略补齐，不在脚本数据里）。
	for _, op := range ev.Show {
		if op.TMs > s.durMs {
			s.durMs = op.TMs
		}
	}
	// Unity/Spine 的台本只有一条 wait（TMs=0），真实长度在 EndMs。
	if ev.EndMs > s.durMs {
		s.durMs = ev.EndMs
	}
	a.sess = s

	// 最近播放时间：派生元数据，写失败不阻断回放。
	reg.Touch(g.ID)
	_ = reg.Save(a.registryPath())
	// 中心库 recent：同样只记元数据；失败只进 errors.log（FR-12 不静默但不阻断）。
	if rerr := a.withCatalog(func(c *catalog.Catalog) error {
		return c.NoteRecent(g.ID, kind, id)
	}); rerr != nil {
		a.logAuxError("catalog recent", rerr)
	}

	return s.metaLocked(), nil
}

// metaLocked 组装会话元信息（会话未共享时或持有 s.mu 时调用）。
func (s *replaySession) metaLocked() SessionMeta {
	meta := SessionMeta{
		Kind:       s.kind,
		ID:         s.id,
		Title:      s.title,
		GameID:     s.gameID,
		StageW:     s.sw,
		StageH:     s.sh,
		OpCount:    len(s.ev.Show),
		TextCount:  countShowOps(s.ev, ir.ShowTextRun),
		DurationMs: s.durMs,
		Mode:       s.mode,
		Dialogue:   s.dialogue,
		Spine:      s.spine,
	}
	if s.kind == "cg" {
		if s.storyEv != nil {
			meta.Modes = []string{"story", "gallery"}
		} else {
			meta.Modes = []string{"gallery"}
		}
	}
	return meta
}

// audioHandle 在系统音频回调外面包一层，记下「最近触发的语音」。
// 只做记录，不改播放语义（BGM/SE/语音仍然原样交给底层 handle）。
func (s *replaySession) audioHandle(handle func(ir.ShowOp)) func(ir.ShowOp) {
	return func(op ir.ShowOp) {
		if op.Kind == ir.ShowVoicePlay {
			s.voiceRef = op.Ref
			if s.p != nil {
				s.voiceAt = s.p.Now()
			}
		}
		if handle != nil {
			handle(op)
		}
	}
}

// resetTimelineLocked 按当前 s.ev 重建播放器（剧情/鉴赏切换用）。
//
// 保留音频挂接（BGM 不断），清掉画面签名/缺素材记账/帧快照，
// 时长按新台本重算——两个模式的时长刻度本来就不一样。
func (s *replaySession) resetTimelineLocked() {
	p := player.New(s.ev, s.opts)
	if s.p != nil {
		p.Audio = s.p.Audio
	} else if s.audio != nil {
		p.Audio = s.audioHandle(s.audio.Handle)
	}
	p.Advance(0, player.Input{})
	s.p = p
	s.closeVideo()
	s.input = player.Input{}
	s.missing = map[string]string{}
	s.lastSig, s.lastJPEG, s.lastDrawn = "", "", nil
	s.lastEnc = time.Time{}
	s.lastTick = time.Now()
	s.waited, s.waitedAt = false, time.Time{}
	s.voiceRef, s.voiceAt = "", 0
	s.durMs = 0
	for _, op := range s.ev.Show {
		if op.TMs > s.durMs {
			s.durMs = op.TMs
		}
	}
	if s.ev.EndMs > s.durMs {
		s.durMs = s.ev.EndMs
	}
	s.snapMu.Lock()
	s.snapOK = false
	s.snapLast = FrameDTO{}
	s.snapMu.Unlock()
}

// closeSessionLocked 关掉当前会话（调方必须持有 a.sessMu）。
func (a *App) closeSessionLocked() {
	if a.sess == nil {
		return
	}
	s := a.sess
	a.sess = nil
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.audio != nil {
		s.audio.Close()
	}
	s.closeVideo()
	s.p = nil
}

// GetFrame 拉当前帧。锁空闲才推进+编码；锁忙（前帧还在编码）直接回
// 最近快照并标 Cached，绝不在 mutex 上排队（防轮询请求无上限积压）。
func (a *App) GetFrame() (FrameDTO, error) {
	if a.sess == nil {
		return FrameDTO{}, fmt.Errorf("没有活动会话")
	}
	s := a.sess
	if !s.mu.TryLock() {
		s.snapMu.RLock()
		defer s.snapMu.RUnlock()
		if !s.snapOK {
			return FrameDTO{}, fmt.Errorf("会话忙，首帧尚未就绪")
		}
		d := s.snapLast
		d.Cached = true
		return d, nil
	}
	defer s.mu.Unlock()
	if s.p == nil {
		return FrameDTO{}, fmt.Errorf("会话已关闭")
	}
	return s.frameLocked(), nil
}

// frameLocked 推进 + 编码（调方持有 s.mu）。
func (s *replaySession) frameLocked() FrameDTO {
	// ① 推进：墙钟差 + 手动输入 + 自动推进注入。
	now := time.Now()
	dt := now.Sub(s.lastTick).Milliseconds()
	if dt > 200 { // 后台/卡顿回来不猛跳
		dt = 200
	}
	s.lastTick = now
	autoIn := false
	if s.auto && s.waited && now.Sub(s.waitedAt) > autoWaitMs*time.Millisecond {
		autoIn = true
		s.waitedAt = now
	}
	if dt > 0 || autoIn || s.input.Advance {
		in := s.input
		s.input = player.Input{} // 手动输入是边沿触发，只吃一次
		if autoIn {
			in.Advance = true
		}
		if s.playing || in.Advance {
			s.p.Advance(dt, in)
		}
	}

	f := s.p.Frame()
	if s.waited != f.Waiting {
		s.waited = f.Waiting
		s.waitedAt = now
	}

	// ② 画面指纹：变化才重编码 JPEG（读字/静止时零开销）。
	// 编码在锁内做（RenderFrame 与 Advance 必须串行），但播放中按
	// encMinGap 节流；否则 16ms 轮询×1600×1200 重编码会长期持锁，
	// 其他绑定调用全部排队成假死（ stacks.log 实证）。暂停态遇到
	// 签名变化（seek/点击）立即编码，不受节流。
	sig := frameSig(f, len(s.missing))
	dto := FrameDTO{
		Now:     f.Now,
		DurMs:   s.durMs,
		Playing: s.playing,
		Done:    f.Done,
		Waiting: f.Waiting,
		Auto:    s.auto,
		Muted:   s.muted,
		QuakeX:  f.QuakeX,
		QuakeY:  f.QuakeY,
		Missing: missingSorted(s.missing),
		Layers:  layerDTOs(f, s.missing),
		Texts:   boxDTOs(f),
	}
	dto.Cached = sig == s.lastSig
	if s.voiceRef != "" {
		dto.Voice = &VoiceDTO{Ref: s.voiceRef, SinceMs: s.voiceAt, Muted: s.muted}
	}
	// Spine 会话没有 Go 侧画面：不编码 JPEG，前端 canvas 按 Now 自己出帧。
	// 仍然更新快照，s.mu 忙时 GetFrame 能拿到最新的时间与播放状态。
	if s.spine != nil {
		dto.JPEG = ""
		dto.Drawn = nil
		s.lastSig = sig
		s.lastJPEG = ""
		s.lastDrawn = nil
		s.snapMu.Lock()
		s.snapLast = dto
		s.snapOK = true
		s.snapMu.Unlock()
		return dto
	}
	if dto.Cached && s.lastJPEG != "" {
		dto.JPEG = s.lastJPEG
		dto.Drawn = append([]string(nil), s.lastDrawn...)
		return dto
	}
	if s.playing && time.Since(s.lastEnc) < encMinGap {
		// 播放中节流窗口未到：回上一张（前端 60ms 后自然拿到新的）。
		dto.JPEG = s.lastJPEG
		dto.Cached = true
		dto.Drawn = append([]string(nil), s.lastDrawn...)
		return dto
	}
	s.lastSig = sig
	s.lastEnc = time.Now()
	j, drawn, err := renderJPEG(s.p, s.load, s.videoProvider(), s.sw, s.sh)
	if err != nil {
		dto.JPEG = s.lastJPEG // 理论上到不了（stageLoader 已吞素材错）
		return dto
	}
	dto.JPEG = j
	dto.Drawn = drawn
	s.lastJPEG = j
	s.lastDrawn = drawn
	// 发快照（TryLock 失败的调用方从这里拿最近帧）。
	s.snapMu.Lock()
	s.snapLast = dto
	s.snapOK = true
	s.snapMu.Unlock()
	return dto
}

// frameSig 便宜的画面指纹（不比像素）。
func frameSig(f player.Frame, missingCount int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "t%d|q%d,%d|d%v|w%v|l%d|m%d",
		f.Now, f.QuakeX, f.QuakeY, f.Done, f.Waiting, len(f.Layers), missingCount)
	for _, t := range f.Texts {
		if !t.Visible {
			continue
		}
		fmt.Fprintf(&b, "|%s", t.Layer)
		if n := len(t.Lines); n > 0 {
			fmt.Fprintf(&b, "/%s:%s", t.Lines[n-1].Speaker, t.Lines[n-1].Text)
		}
	}
	return b.String()
}

func missingSorted(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func layerDTOs(f player.Frame, missing map[string]string) []LayerDTO {
	out := make([]LayerDTO, 0, len(f.Layers))
	for _, l := range f.Layers {
		_, miss1 := missing[l.Ref]
		_, miss2 := missing[l.Resolved]
		out = append(out, LayerDTO{
			Layer: l.Layer, Ref: l.Ref, Visible: l.Visible,
			X: l.X, Y: l.Y, Opacity: l.Opacity, Z: l.Z,
			Missing: miss1 || miss2,
		})
	}
	return out
}

func boxDTOs(f player.Frame) []BoxDTO {
	out := make([]BoxDTO, 0, len(f.Texts))
	for _, t := range f.Texts {
		if !t.Visible {
			continue
		}
		b := BoxDTO{
			Layer:        t.Layer,
			X:            t.X,
			Y:            t.Y,
			W:            t.W,
			H:            t.H,
			HasGeom:      t.HasGeom,
			Visible:      t.Visible,
			FrameColor:   rgbaDTO{t.FrameColor[0], t.FrameColor[1], t.FrameColor[2]},
			FrameOpacity: t.FrameOpacity,
			FontSize:     t.FontSize,
			LineSpacing:  t.LineSpacing,
			MarginL:      t.MarginL,
			MarginT:      t.MarginT,
			MarginR:      t.MarginR,
			MarginB:      t.MarginB,
			Frame:        t.Frame,
		}
		for _, ln := range t.Lines {
			b.Lines = append(b.Lines, LineDTO{Speaker: ln.Speaker, Text: ln.Text})
		}
		out = append(out, b)
	}
	return out
}

// videoProvider 影片取帧：引用 → 解包目录里的真实文件 → ffmpeg 流式解码。
//
// 回放时间单调向前，所以开一条进程连续出帧（按时间丢帧），比每帧 seek 省得多；
// 换片或倒退时 Stream 自己会重开。没有 ffmpeg 时返回错误，渲染层记进缺素材、
// 不中断播放。
func (s *replaySession) videoProvider() player.VideoProvider {
	root := s.root
	return func(ref string, tMs int64) (image.Image, error) {
		path, err := replay.AssetPath(root, ref)
		if err != nil {
			return nil, err
		}
		if s.vs == nil {
			s.vs = map[string]*video.Stream{}
		}
		st, ok := s.vs[path]
		if !ok {
			var err error
			st, err = video.OpenStream(path, s.sw, s.sh, videoFPS)
			if err != nil {
				return nil, err
			}
			s.vs[path] = st
		}
		return st.Frame(tMs)
	}
}

// closeVideo 关掉全部影片流（换事件/关会话时调用）。
func (s *replaySession) closeVideo() {
	for path, st := range s.vs {
		_ = st.Close()
		delete(s.vs, path)
	}
}

// renderJPEG RenderFrame → JPEG dataURL（quake 偏移已由 Compose/定位处理？——
// RenderFrame 只管图层与框底，前端拿 QuakeX/Y 对 <img> 做位移）。
// drawn 原样返回：影片取帧失败等诊断不能在这里被丢掉。
func renderJPEG(p *player.Player, load player.Loader, video player.VideoProvider, w, h int) (string, []string, error) {
	rgba, drawn, err := player.RenderFrameWithVideo(p, load, video, w, h)
	if err != nil {
		return "", drawn, err
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, rgba, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return "", drawn, err
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), drawn, nil
}

// Play 恢复自动推进。
func (a *App) Play() error {
	if a.sess == nil {
		return fmt.Errorf("没有活动会话")
	}
	s := a.sess
	s.mu.Lock()
	defer s.mu.Unlock()
	s.playing = true
	s.lastTick = time.Now()
	return nil
}

// Pause 暂停（不终止会话）。
func (a *App) Pause() error {
	if a.sess == nil {
		return fmt.Errorf("没有活动会话")
	}
	s := a.sess
	s.mu.Lock()
	defer s.mu.Unlock()
	s.playing = false
	return nil
}

// TogglePlay 播放/暂停切换，返回切换后的状态。
func (a *App) TogglePlay() (bool, error) {
	if a.sess == nil {
		return false, fmt.Errorf("没有活动会话")
	}
	s := a.sess
	s.mu.Lock()
	defer s.mu.Unlock()
	s.playing = !s.playing
	s.lastTick = time.Now()
	return s.playing, nil
}

// InputClick 注入一次点击推进（对齐 CLI「空格/回车」）。
//
// 暂停态也生效：点击推进是「过掉当前点击点」，与自动播放开关无关
// （frameLocked 里 `playing || in.Advance` 才会调 p.Advance）。
// 只对 text 点击点有效；停在 [wait]/影片/转场这类时间阻塞点时点击不越过
// （与原作一致，硬跳时间轴请用 SeekTo）。
func (a *App) InputClick() error {
	if a.sess == nil {
		return fmt.Errorf("没有活动会话")
	}
	s := a.sess
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.p == nil {
		return fmt.Errorf("会话已关闭")
	}
	s.input.Advance = true
	// 手动推进要立刻结算：把等待计时起点更新到现在，避免自动推进
	// 在点击后马上又补一次（两连跳）。
	s.waitedAt = time.Now()
	return nil
}

// SetReplayMode 切 CG 的回放模式：story（剧情演出，带台词/语音）/ gallery（纯图鉴赏）。
//
// 两个台本都在开会话时构建好，切换只重建播放器（保留音频挂接与播放开关），
// 不做第二次剧本解析。返回切换后的 SessionMeta（时长/op 数/台词数据都会变）。
func (a *App) SetReplayMode(mode string) (SessionMeta, error) {
	if a.sess == nil {
		return SessionMeta{}, fmt.Errorf("没有活动会话")
	}
	s := a.sess
	s.mu.Lock()
	defer s.mu.Unlock()
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "story":
		if s.storyEv == nil {
			return SessionMeta{}, fmt.Errorf("该事件没有剧情演出数据（只有纯图 CG）")
		}
		s.ev, s.mode = s.storyEv, "story"
	case "gallery":
		if s.galleryEv == nil {
			return SessionMeta{}, fmt.Errorf("该事件不是 CG 鉴赏条目，没有鉴赏模式")
		}
		s.ev, s.mode = s.galleryEv, "gallery"
	default:
		return SessionMeta{}, fmt.Errorf("未知回放模式 %q（可选 story / gallery）", mode)
	}
	s.resetTimelineLocked()
	return s.metaLocked(), nil
}

// ToggleAuto 切自动推进（与 CLI M 键同语义，写回 conf.Auto）。
func (a *App) ToggleAuto() (bool, error) {
	if a.sess == nil {
		return false, fmt.Errorf("没有活动会话")
	}
	s := a.sess
	s.mu.Lock()
	s.auto = !s.auto
	auto := s.auto
	s.mu.Unlock()

	c, _ := a.loadConf()
	c.Auto = auto
	if err := a.saveConf(c); err != nil {
		return auto, err
	}
	return auto, nil
}

// SetMuted 静音（只是不再把音频 op 派给平台层，会话继续跑）。
func (a *App) SetMuted(m bool) error {
	if a.sess == nil {
		return fmt.Errorf("没有活动会话")
	}
	s := a.sess
	s.mu.Lock()
	defer s.mu.Unlock()
	s.muted = m
	if s.audio != nil {
		if m {
			s.p.Audio = nil
		} else {
			s.p.Audio = s.audio.Handle
		}
	}
	return nil
}

// ExitReplay 退出回放页（关会话、停 BGM/SE）。
func (a *App) ExitReplay() error {
	a.sessMu.Lock()
	defer a.sessMu.Unlock()
	a.closeSessionLocked()
	return nil
}

// SeekTo 跳到目标毫秒：重建播放器 + 快进（纯状态推进，不渲染，倍速批量走）。
func (a *App) SeekTo(ms int64) error {
	if a.sess == nil {
		return fmt.Errorf("没有活动会话")
	}
	s := a.sess
	if ms < 0 {
		ms = 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	opt := s.opts
	p := player.New(s.ev, opt)
	p.Audio = s.p.Audio // 音频挂接保持
	p.Advance(0, player.Input{})
	for t := p.Now(); t < ms && !p.Done(); t = p.Now() {
		step := int64(200)
		if ms-t < step {
			step = ms - t
		}
		p.Advance(step, player.Input{})
	}
	s.p = p
	s.missing = map[string]string{} // 重建后按新时间点重新记账
	s.lastSig = ""
	s.lastJPEG = ""
	s.lastDrawn = nil
	s.lastEnc = time.Time{} // 立即允许一次编码
	s.lastTick = time.Now()
	return nil
}

// Screenshot 把当前帧 PNG 落盘到 exe\exports\，返回路径（DR-30 只写派生物）。
func (a *App) Screenshot() (string, error) {
	if a.sess == nil {
		return "", fmt.Errorf("没有活动会话")
	}
	s := a.sess
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.spine != nil {
		return "", fmt.Errorf("Spine 骨骼动画暂不支持服务端截图（画面由前端 WebGL 渲染）")
	}

	rgba, _, err := player.RenderFrame(s.p, s.load, s.sw, s.sh)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(dataDir(), "exports", safeName(s.kind)+"_"+safeName(s.id))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := fmt.Sprintf("%s.png", time.Now().Format("20060102-150405.000"))
	if err := savePNG(rgba, filepath.Join(dir, name)); err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

// savePNG PNG 落盘。
func savePNG(src image.Image, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, src)
}
