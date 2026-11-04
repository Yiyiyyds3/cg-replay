package scn

// show.go —— PSB scene → ir.Event.Show（顺序台本）。
//
// 9nine 的 texts[] 每条都带一份「当时场景状态」（bg / ev / layer / character /
// msgwin / se / bgm）。因此不需要解释 lines 里的 VM 字节码：
//   - 逐条 text 比较图片引用，变了就 backlay + crossfade；
//   - 台词、语音、BGM 从 text 自身字段取；
//   - character 是 E-mote，按硬边界跳过，只置 HasEmote。
//
// 这不是「通用引擎模拟器」：只覆盖 9nine 剧情回放需要的最小指令集，
// 未识别的 class 不进台本也不假装演出来。

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"ucgr/internal/adapter"
	"ucgr/internal/ir"
)

// 9nine 固定图层编号（z 序由 player.LayerZ 解析）：
// base(0) < layer0(1) < layer1(2) < …；character 层跳过。
const (
	layerBG = "base"
	layerEV = "layer1"
)

type showBuilder struct {
	root string
	opt  Options
	ix   *AssetIndex

	ops   []ir.ShowOp
	clock int64

	shown    map[string]string // 图层 → 当前图片引用（未解析）
	assets   map[string]string // 原始引用 → 相对路径
	texts    []ir.TextLine
	voices   []ir.Voice
	hasEmote bool
	lineNo   int
}

func newShowBuilder(root string, opt Options) *showBuilder {
	opt = opt.Normalize()
	return &showBuilder{
		root:   root,
		opt:    opt,
		ix:     assetIndex(root, opt),
		shown:  map[string]string{},
		assets: map[string]string{},
	}
}

func (b *showBuilder) add(op ir.ShowOp) {
	op.TMs = b.clock
	b.ops = append(b.ops, op)
}

func (b *showBuilder) addWait(ms int64, attr map[string]string) {
	if ms < 0 {
		ms = 0
	}
	b.add(ir.ShowOp{Kind: ir.ShowWait, DurMs: ms, Attr: attr})
	b.clock += ms
}

// addMessageBox 设置 9nine 的消息窗口几何（1280×720 下贴在底部）。
func (b *showBuilder) addMessageBox() {
	top := b.opt.StageH - 210
	if top < 0 {
		top = 0
	}
	b.add(ir.ShowOp{Kind: ir.ShowLayerOpt, Layer: "message0", Page: "fore",
		Attr: map[string]string{"visible": "true"}})
	b.add(ir.ShowOp{Kind: ir.ShowLayerAttr, Layer: "message0", Page: "fore",
		Attr: map[string]string{
			"left": strconv.Itoa(0), "top": strconv.Itoa(top),
			"width": strconv.Itoa(b.opt.StageW), "height": "210",
			"opacity": "160",
			"margint": "30", "marginl": "64", "marginr": "64", "marginb": "16",
		}})
}

// buildScene 把一条 scene 的 texts 追加进 builder。
func (b *showBuilder) buildScene(sc Scene, evID string) error {
	if len(sc.Texts) == 0 {
		return fmt.Errorf("scene %q 没有台词", sc.Label)
	}
	b.addMessageBox()
	var bgm string
	for _, t := range sc.Texts {
		changes, nextBGM, emote := b.collectState(t.Commands, bgm)
		b.hasEmote = b.hasEmote || emote
		if nextBGM != bgm {
			bgm = nextBGM
			if bgm == "" {
				b.add(ir.ShowOp{Kind: ir.ShowBGMStop})
			} else {
				b.add(ir.ShowOp{Kind: ir.ShowBGMPlay, Ref: b.resolveAudio(bgm)})
			}
		}
		b.applyImageChanges(changes)

		body := cleanBody(t.Body)
		if strings.TrimSpace(body) == "" {
			continue
		}
		// 一句一屏：先清上一句，再写当前句，最后等点击。
		b.add(ir.ShowOp{Kind: ir.ShowTextClear, Layer: "message0"})
		b.add(ir.ShowOp{Kind: ir.ShowTextRun, Layer: "message0",
			Text: &ir.ShowText{Speaker: t.Speaker, Text: body}})
		b.lineNo++
		line := ir.TextLine{
			LineID:  fmt.Sprintf("%s#%d", evID, b.lineNo),
			TMs:     b.clock,
			Speaker: t.Speaker,
			Text:    body,
		}
		if len(t.Voices) > 0 {
			rel := b.resolveAudio(t.Voices[0].Name)
			line.Voice = &ir.AssetRef{OrigPath: rel, Role: "语音"}
			b.voices = append(b.voices, ir.Voice{LineID: line.LineID,
				Asset: ir.AssetRef{OrigPath: rel, Role: "语音"}})
			b.add(ir.ShowOp{Kind: ir.ShowVoicePlay, Ref: rel})
		}
		b.texts = append(b.texts, line)
		b.add(ir.ShowOp{Kind: ir.ShowTextWait})
		b.clock += 900
	}
	return nil
}

// inlineCmdRe 台词里的内联控制命令，例如 %p-1;、%fＭＳ ゴシック;、%p;、%fuser;。
// 这些是原作给引擎的排版指令，回放器不解释，直接剥掉。
var inlineCmdRe = regexp.MustCompile(`%[A-Za-z][^;%]{0,40};`)

// rubyRe 注音标记：[しろみつがわ,3] 紧跟正文。播放器不排注音，去掉方括号保留正文。
// 只在「方括号里有逗号 + 数字」时才认，避免误吃正常的引号内容。
var rubyRe = regexp.MustCompile(`\[[^\[\],]{0,40},[0-9]+\]`)

// cleanBody 把 PSB 台词整理成可直接绘制的文本：
// 还原换行转义、去掉内联命令与注音标记。
func cleanBody(s string) string {
	s = strings.ReplaceAll(s, `\n`, "\n")
	s = strings.ReplaceAll(s, `\r`, "")
	s = inlineCmdRe.ReplaceAllString(s, "")
	s = rubyRe.ReplaceAllString(s, "")
	return s
}

type imageChange struct {
	layer string
	ref   string // 空 = 隐藏
}

// collectState 从一条 text 的状态块里抽出要改的图片层、BGM 与 E-mote 标记。
func (b *showBuilder) collectState(cmds []Command, currentBGM string) ([]imageChange, string, bool) {
	var changes []imageChange
	bgm := currentBGM
	emote := false
	for _, c := range cmds {
		switch strings.ToLower(c.Class) {
		case "stage", "background":
			if c.File != "" && b.shown[layerBG] != c.File {
				changes = append(changes, imageChange{layer: layerBG, ref: c.File})
			}
		case "event", "cg":
			if c.File != "" && b.shown[layerEV] != c.File {
				changes = append(changes, imageChange{layer: layerEV, ref: c.File})
			}
		case "layer":
			name := strings.ToLower(c.Name)
			if name == "" {
				continue
			}
			if !strings.HasPrefix(name, "layer") {
				name = "layer" + name
			}
			// 与 ev 同图时跳过：9nine 常用 layer0 对同一张 CG 做缩放演出，
			// 本播放器暂不支持缩放，重复铺一层只会盖住背景。
			if c.File != "" && c.File == b.shown[layerEV] {
				continue
			}
			if c.File != "" && b.shown[name] != c.File {
				changes = append(changes, imageChange{layer: name, ref: c.File})
			}
		case "character", "chara", "stand":
			emote = true
		case "bgm":
			file := ""
			state := int64(1)
			if v, ok := c.Replay["filename"]; ok {
				file = psbString(v)
			}
			if file == "" {
				file = c.Name
			}
			if v, ok := c.Replay["state"]; ok {
				state = psbInt(v)
			}
			if state == 0 {
				file = ""
			}
			bgm = file
		}
	}
	return changes, bgm, emote
}

// applyImageChanges 第一次出现直接 fore 显示；之后走 backlay + crossfade。
func (b *showBuilder) applyImageChanges(changes []imageChange) {
	if len(changes) == 0 {
		return
	}
	var trans []imageChange
	for _, ch := range changes {
		if _, exists := b.shown[ch.layer]; !exists {
			// 首次出现：直接上 fore，不必为一张新图做淡入。
			b.shown[ch.layer] = ch.ref
			if ch.ref != "" {
				rel := b.resolveImage(ch.ref)
				b.add(ir.ShowOp{Kind: ir.ShowLayerShow, Layer: ch.layer, Page: "fore",
					Ref: rel, Attr: map[string]string{"left": "0", "top": "0"}})
			}
			continue
		}
		trans = append(trans, ch)
	}
	if len(trans) == 0 {
		return
	}
	b.add(ir.ShowOp{Kind: ir.ShowScreenCopy})
	for _, ch := range trans {
		b.shown[ch.layer] = ch.ref
		if ch.ref == "" {
			b.add(ir.ShowOp{Kind: ir.ShowLayerHide, Layer: ch.layer, Page: "back"})
			continue
		}
		rel := b.resolveImage(ch.ref)
		b.add(ir.ShowOp{Kind: ir.ShowLayerShow, Layer: ch.layer, Page: "back",
			Ref: rel, Attr: map[string]string{"left": "0", "top": "0"}})
	}
	b.add(ir.ShowOp{Kind: ir.ShowTransition, DurMs: 600})
	// [wt] 只等转场，不额外增加阅读等待。
	b.add(ir.ShowOp{Kind: ir.ShowWait, DurMs: 0, Attr: map[string]string{"wt": "1"}})
	b.clock += 600
}

func (b *showBuilder) resolveImage(ref string) string {
	if ref == "" {
		return ""
	}
	rel := b.ix.Image(ref)
	b.assets[ref] = rel
	return rel
}

func (b *showBuilder) resolveAudio(ref string) string {
	if ref == "" {
		return ""
	}
	rel := b.ix.Audio(ref)
	b.assets[ref] = rel
	return rel
}

func (b *showBuilder) event(evID, title string) *ir.Event {
	if title == "" {
		title = evID
	}
	show := make([]ir.ShowOp, len(b.ops))
	copy(show, b.ops)
	return &ir.Event{
		EventID:  evID,
		Kind:     ir.KindScene,
		Title:    title,
		HasEmote: b.hasEmote,
		Show:     show,
		Texts:    append([]ir.TextLine(nil), b.texts...),
		Voices:   append([]ir.Voice(nil), b.voices...),
		Assets:   collectAssets(b.assets, roleOf),
	}
}

// BuildShowScene 构建一个 .scn 里第 sceneIndex 个 scene 的演出。
func BuildShowScene(root, scriptPath string, sceneIndex int, opt Options) (*ir.Event, error) {
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		return nil, err
	}
	sc, err := ParseScript(data, scriptPath)
	if err != nil {
		return nil, err
	}
	var target *Scene
	for i := range sc.Scenes {
		if sc.Scenes[i].Index == sceneIndex {
			target = &sc.Scenes[i]
			break
		}
	}
	if target == nil {
		return nil, fmt.Errorf("%s 没有第 %d 个 scene", filepath.Base(scriptPath), sceneIndex)
	}
	b := newShowBuilder(root, opt)
	evID := fmt.Sprintf("%s#%d", sc.Name, target.Index)
	if err := b.buildScene(*target, evID); err != nil {
		return nil, err
	}
	title := target.Title
	if title == "" {
		title = target.Label
	}
	return b.event(evID, title), nil
}

// BuildShowFile 把整份 .scn 的所有 scene 串成一个事件（场景级入口）。
func BuildShowFile(root, scriptPath string, opt Options) (*ir.Event, error) {
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		return nil, err
	}
	sc, err := ParseScript(data, scriptPath)
	if err != nil {
		return nil, err
	}
	b := newShowBuilder(root, opt)
	for _, scene := range sc.SceneInfos() {
		evID := fmt.Sprintf("%s#%d", sc.Name, scene.Index)
		if err := b.buildScene(scene, evID); err != nil {
			continue
		}
	}
	if len(b.ops) == 0 {
		return nil, fmt.Errorf("%s 没有可回放的 scene", filepath.Base(scriptPath))
	}
	return b.event(sc.Name, sc.Name), nil
}

// BuildShowCG 构建一个 CG 鉴赏事件（图片序列 + 淡入淡出）。
func BuildShowCG(root, cgListPath, id string, opt Options) (*ir.Event, error) {
	entries, err := ParseCGList(cgListPath)
	if err != nil {
		return nil, err
	}
	e := FindCG(entries, id)
	if e == nil {
		return nil, fmt.Errorf("CG 列表 %s 里没有 %q", filepath.Base(cgListPath), id)
	}
	opt = opt.Normalize()
	ix := assetIndex(root, opt)
	b := newShowBuilder(root, opt)
	var pages []string
	for _, p := range e.Images {
		rel := ix.Image(p)
		if rel == "" {
			continue
		}
		b.assets[p] = rel
		pages = append(pages, rel)
	}
	if len(pages) == 0 {
		return nil, fmt.Errorf("CG %s 没有可用图片", e.Thumb)
	}
	b.add(ir.ShowOp{Kind: ir.ShowLayerShow, Layer: layerBG, Page: "fore",
		Ref: pages[0], Attr: map[string]string{"left": "0", "top": "0"}})
	b.addWait(1200, nil)
	for _, rel := range pages[1:] {
		b.add(ir.ShowOp{Kind: ir.ShowScreenCopy})
		b.add(ir.ShowOp{Kind: ir.ShowLayerShow, Layer: layerBG, Page: "back",
			Ref: rel, Attr: map[string]string{"left": "0", "top": "0"}})
		b.add(ir.ShowOp{Kind: ir.ShowTransition, DurMs: 600})
		b.add(ir.ShowOp{Kind: ir.ShowWait, DurMs: 0, Attr: map[string]string{"wt": "1"}})
		b.addWait(1200, nil)
	}
	ev := b.event(e.Thumb, e.Title)
	ev.Kind = ir.KindStillCG
	return ev, nil
}

// OptionsFromConfig 从 adapter.BuildCtx 的 config 读构建参数
// （key：image_dirs / audio_dirs / stage_w / stage_h，目录用逗号分隔）。
func OptionsFromConfig(ctx *adapter.BuildCtx) Options {
	opt := DefaultOptions()
	if ctx == nil || ctx.Config == nil {
		return opt
	}
	if v := ctx.Config["image_dirs"]; v != "" {
		opt.ImageDirs = strings.Split(v, ",")
	}
	if v := ctx.Config["audio_dirs"]; v != "" {
		opt.AudioDirs = strings.Split(v, ",")
	}
	if v, err := strconv.Atoi(ctx.Config["stage_w"]); err == nil && v > 0 {
		opt.StageW = v
	}
	if v, err := strconv.Atoi(ctx.Config["stage_h"]); err == nil && v > 0 {
		opt.StageH = v
	}
	return opt.Normalize()
}

func rootFromConfig(ctx *adapter.BuildCtx) string {
	if ctx == nil || ctx.Config == nil {
		return ""
	}
	return ctx.Config["root"]
}

// psbString/psbInt 是给 map[string]any 用的小转换，避免把 psb 包类型漏到上层。
func psbString(v any) string {
	s, _ := v.(string)
	return s
}

func psbInt(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case float64:
		return int64(x)
	}
	return 0
}
