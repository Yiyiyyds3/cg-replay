// Package scn 适配 M2 / KiriKiri Evolution 的 PSB 剧本（.scn）。
//
// 9nine 系列没有可读的 KAG .ks 剧情脚本：剧情被编译成 PSB 容器里的
// scenes → lines / texts。本适配器只读 PSB，把它翻译成项目统一的
// ir.Event.Show 顺序台本：
//   - 背景（class=stage）、事件 CG（class=event）、额外图层（class=layer）
//   - 台词（texts[].text）、说话人、语音（texts[].voice）
//   - BGM（class=bgm）
//   - 转场（跨文本时图片变化 → backlay + crossfade）
//
// 硬边界（docs/07 FR-19）：9nine 的人物是 E-mote 动态立绘（class=character），
// 需要官方运行时，**不复刻**；这里只置 HasEmote 并跳过 character 层，
// 不把立绘拆成静态图假装能播。
package scn

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"ucgr/internal/adapter"
	"ucgr/internal/ir"
	"ucgr/internal/psb"
)

func init() {
	adapter.Register("scn", &Engine{})
}

// Engine 是 scn 引擎适配器（实现 adapter.EngineAdapter）。
type Engine struct{}

// Script 一份 .scn 剧本。
type Script struct {
	Path   string
	Name   string // a01.txt（去掉 .scn）
	Scenes []Scene
	// Dropped 值树里被跳过的坏节点数（解包产物局部损坏时 > 0）。
	Dropped int
	file    *psb.File
}

// Scene 剧本里的一个 scene（原作的事件单位，label 形如 *a01_1_a）。
type Scene struct {
	Index     int
	Label     string
	Title     string
	FirstLine int64
	Texts     []Text
	Nexts     []string
}

// Text 一条台词及其对应的场景状态。
type Text struct {
	Speaker  string
	Body     string
	Voices   []Voice
	TimeHint int64
	Commands []Command
}

// Voice 一条语音引用。
type Voice struct {
	Name string
	Type int64
	Pan  int64
}

// Command 场景状态里的一条指令（PSB 的 [class, target, params] 三元组）。
type Command struct {
	Class    string // stage/event/layer/character/msgwin/se/bgm
	Name     string // bg / ev / layer0 / 翔 / ...
	File     string // 图片 storage 名（不带扩展名）
	Showmode int64
	Actions  map[string]any // zpos / xpos / ypos / zoomx / ...
	Replay   map[string]any // bgm 的 filename/state/volume
}

// ParseScript 解析一份 .scn。entryPath 只用于命名与错误信息。
func ParseScript(data []byte, entryPath string) (*Script, error) {
	f, err := psb.Parse(data)
	if err != nil {
		return nil, err
	}
	root, ok := f.Root.(psb.Object)
	if !ok {
		return nil, fmt.Errorf("%s: PSB 根不是字典", filepath.Base(entryPath))
	}
	scenes := psb.ArrayOf(root["scenes"])
	if len(scenes) == 0 {
		return nil, fmt.Errorf("%s: 没有 scenes（不是剧本 PSB？）", filepath.Base(entryPath))
	}
	sc := &Script{
		Path:    entryPath,
		Name:    strings.TrimSuffix(filepath.Base(entryPath), filepath.Ext(entryPath)),
		Dropped: f.Dropped,
		file:    f,
	}
	for i, raw := range scenes {
		obj := psb.ObjectOf(raw)
		if obj == nil {
			continue
		}
		scene := Scene{
			Index:     i,
			Label:     strings.TrimPrefix(psb.StringOf(obj["label"]), "*"),
			Title:     psb.StringOf(obj["title"]),
			FirstLine: psb.IntOf(obj["firstLine"]),
		}
		for _, nx := range psb.ArrayOf(obj["nexts"]) {
			if s := psb.StringOf(nx); s != "" {
				scene.Nexts = append(scene.Nexts, s)
			}
		}
		for _, tr := range psb.ArrayOf(obj["texts"]) {
			if t, ok := parseText(tr); ok {
				scene.Texts = append(scene.Texts, t)
			}
		}
		sc.Scenes = append(sc.Scenes, scene)
	}
	if !sc.sane() {
		if psb.IsObfuscated(data) {
			return nil, fmt.Errorf("%s: 0x20 混淆的 PSB 结构不可信（名字表/根对象错位），疑似解包产物损坏",
				filepath.Base(entryPath))
		}
		return nil, fmt.Errorf("%s: 没有可信的场景（scene 标签非 ASCII 或台词缺失），不像 9nine 剧本 PSB",
			filepath.Base(entryPath))
	}
	return sc, nil
}

// sane 报告解析结果是否可信：至少一个 scene 的标签是 ASCII 且带有效台词。
//
// 0x20 混淆变体里有一部分 .scn 的名字表对不齐（Trie 越界），解析器会「读出来
// 但全是乱码」。这里做结构自检，宁可报错让事件库跳过，也不把乱码当剧情。
func (s *Script) sane() bool {
	for _, sc := range s.Scenes {
		if !validSceneLabel(sc.Label) {
			continue
		}
		for _, t := range sc.Texts {
			if strings.TrimSpace(t.Body) != "" {
				return true
			}
		}
	}
	return false
}

// labelRe 9nine 的 scene 标签：start / a01_1_c / e04d / dummyselect1。
var labelRe = regexp.MustCompile(`^[A-Za-z0-9_.\-]+$`)

func validSceneLabel(s string) bool { return labelRe.MatchString(s) }

func parseText(raw any) (Text, bool) {
	arr := psb.ArrayOf(raw)
	if len(arr) < 3 {
		return Text{}, false
	}
	body := psb.StringOf(arr[2])
	// 台词必须是合法 UTF-8：坏掉的解包产物会读出控制字符/乱码，
	// 与其把乱码端上回放页，不如当作「这条台词没解析出来」。
	if body != "" && !utf8.ValidString(body) {
		return Text{}, false
	}
	t := Text{
		Speaker:  psb.StringOf(arr[0]),
		Body:     body,
		TimeHint: psb.IntOf(arr[4]),
	}
	if len(arr) > 3 {
		for _, v := range psb.ArrayOf(arr[3]) {
			vo := psb.ObjectOf(v)
			if vo == nil {
				continue
			}
			name := psb.StringOf(vo["voice"])
			if name == "" {
				continue
			}
			t.Voices = append(t.Voices, Voice{
				Name: name,
				Type: psb.IntOf(vo["type"]),
				Pan:  psb.IntOf(vo["pan"]),
			})
		}
	}
	if len(arr) > 5 {
		state := psb.ObjectOf(arr[5])
		if state != nil {
			for _, c := range psb.ArrayOf(state["data"]) {
				if cmd, ok := parseCommand(c); ok {
					t.Commands = append(t.Commands, cmd)
				}
			}
		}
	}
	return t, true
}

func parseCommand(raw any) (Command, bool) {
	arr := psb.ArrayOf(raw)
	if len(arr) < 2 {
		return Command{}, false
	}
	params := psb.ObjectOf(arr[2])
	cmd := Command{
		Class: strings.TrimSpace(psb.StringOf(arr[0])),
		Name:  strings.TrimSpace(psb.StringOf(arr[1])),
	}
	if params == nil {
		return cmd, cmd.Class != "" || cmd.Name != ""
	}
	if s := psb.StringOf(params["class"]); s != "" {
		cmd.Class = s
	}
	if s := psb.StringOf(params["name"]); s != "" {
		cmd.Name = s
	}
	cmd.Showmode = psb.IntOf(params["showmode"])
	if redraw := psb.ObjectOf(params["redraw"]); redraw != nil {
		if img := psb.ObjectOf(redraw["imageFile"]); img != nil {
			cmd.File = psb.StringOf(img["file"])
		}
	}
	if acts := psb.ArrayOf(params["action"]); len(acts) > 0 {
		cmd.Actions = map[string]any{}
		for _, a := range acts {
			pair := psb.ArrayOf(a)
			if len(pair) < 2 {
				continue
			}
			key := psb.StringOf(pair[0])
			if key == "" {
				continue
			}
			cmd.Actions[key] = pair[1]
		}
	}
	if replay := psb.ObjectOf(params["replay"]); replay != nil {
		cmd.Replay = map[string]any{}
		for k, v := range replay {
			cmd.Replay[k] = v
		}
	}
	return cmd, true
}

// SceneInfos 返回剧本里可回放的 scene（有台词且标签可信的才进事件库；
// 纯选择/纯 CG 的空 scene、解包损坏产生的乱码 scene 都不产出空事件）。
func (s *Script) SceneInfos() []Scene {
	out := make([]Scene, 0, len(s.Scenes))
	for _, sc := range s.Scenes {
		if len(sc.Texts) == 0 || !validSceneLabel(sc.Label) {
			continue
		}
		out = append(out, sc)
	}
	return out
}

// Identify 识别 .scn 文件或含 .scn 的解包目录。
func (e *Engine) Identify(path string) (string, error) {
	if isSCNFile(path) {
		return "scn", nil
	}
	st, err := os.Stat(path)
	if err != nil || !st.IsDir() {
		return "", nil
	}
	// 目录：最多向下 4 层找 .scn（9nine 的 raw/data/scn、
	// 整理版 原始归档解包/data_脚本/scn 都在这个深度内）。
	depth := strings.Count(filepath.Clean(path), string(filepath.Separator))
	found := ""
	filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err != nil || found != "" {
			return nil
		}
		if d.IsDir() {
			if strings.Count(filepath.Clean(p), string(filepath.Separator))-depth > 4 {
				return filepath.SkipDir
			}
			return nil
		}
		if isSCNFile(p) {
			found = "scn"
		}
		return nil
	})
	return found, nil
}

func isSCNFile(path string) bool {
	if !strings.EqualFold(filepath.Ext(path), ".scn") {
		return false
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	var b [4]byte
	if _, err := io.ReadFull(f, b[:]); err != nil {
		return false
	}
	// 明文 "PSB\0" 与 0x20 混淆变体 "psb " 都算（新章/春色/雪色是后者）。
	return psb.IsPSB(b[:])
}

// ParseArchive scn 不是归档格式。
func (e *Engine) ParseArchive(string) ([]adapter.ArchiveEntry, error) {
	return nil, fmt.Errorf("scn 是明文 PSB 剧本，不是归档；请直接传入 .scn 文件")
}

// ParseScript 实现 adapter.EngineAdapter。
func (e *Engine) ParseScript(data []byte, ctx *adapter.ScriptCtx) (adapter.ScriptAst, error) {
	entry := ""
	if ctx != nil {
		entry = ctx.EntryPath
	}
	return ParseScript(data, entry)
}

// BuildIR 把整份剧本的所有 scene 串成一个 IR（CLI 分析用）。
func (e *Engine) BuildIR(ast adapter.ScriptAst, ctx *adapter.BuildCtx) (*ir.IR, error) {
	sc, ok := ast.(*Script)
	if !ok || sc == nil {
		return nil, fmt.Errorf("scn: AST 类型错误")
	}
	opts := OptionsFromConfig(ctx)
	out := &ir.IR{
		IRVersion: "1.0",
		EngineID:  "scn",
		Fidelity:  ir.FidelityGeneric,
		Meta: ir.Meta{
			ScriptEncoding: "psb-utf8",
			GeneratedBy:    "ucgr/scn",
		},
		CanvasW: opts.StageW,
		CanvasH: opts.StageH,
	}
	for i, scene := range sc.SceneInfos() {
		ev, err := BuildShowScene(rootFromConfig(ctx), sc.Path, scene.Index, opts)
		if err != nil {
			if ctx != nil && ctx.RecordUnrecognized != nil {
				ctx.RecordUnrecognized(ir.Unrecognized{
					NodePath: fmt.Sprintf("scenes[%d]", scene.Index),
					Raw:      scene.Label,
					Reason:   err.Error(),
				})
			}
			continue
		}
		ev.Ord = i + 1
		out.Events = append(out.Events, *ev)
		if ev.HasEmote {
			out.HasEmote = true
		}
	}
	return out, nil
}

// Transitions 转场注册表：PSB 跨文本的图片变化按 crossfade 处理。
func (e *Engine) Transitions() *adapter.TransitionRegistry {
	r := adapter.NewTransitionRegistry()
	r.Register("crossfade", adapter.TransitionImpl{ImplKey: "crossfade", DurationMs: 600})
	r.Register("fade", adapter.TransitionImpl{ImplKey: "fade", DurationMs: 600})
	return r
}

// Scenes 读一个 .scn 的 scene 列表（事件库扫描用）。
func Scenes(path string) ([]Scene, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	sc, err := ParseScript(data, path)
	if err != nil {
		return nil, err
	}
	return sc.SceneInfos(), nil
}

// SceneCount 读一个 .scn 的可回放 scene 数（扫描摘要用）。
func SceneCount(path string) int {
	scenes, err := Scenes(path)
	if err != nil {
		return 0
	}
	return len(scenes)
}

// SortedSCNFiles 返回目录下所有 PSB .scn 文件（按文件名排序）。
func SortedSCNFiles(dir string) ([]string, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		p := filepath.Join(dir, e.Name())
		if isSCNFile(p) {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out, nil
}
