package kirikiri

import (
	"strings"
	"testing"

	"ucgr/internal/adapter"
	"ucgr/internal/ir"
)

// mkCmd 造一条命令（Attrs 为 nil 时给空表，避免调用方到处判空）。
func mkCmd(tag string, attrs map[string]string) *Command {
	if attrs == nil {
		attrs = map[string]string{}
	}
	return &Command{Tag: tag, Attrs: attrs, Raw: tag}
}

// testResolver 按引用前缀给出「像真实解包树」的路径，好让 kindForLayer 的路径判定生效。
func testResolver(ref string) (*ir.AssetRef, bool) {
	path := "raw/data/image/" + ref + ".jpg"
	switch {
	case strings.HasPrefix(ref, "chr"):
		path = "raw/data/fgimage/" + ref + ".png"
	case strings.HasPrefix(ref, "bg"):
		path = "raw/data/bgimage/" + ref + ".png"
	case strings.HasPrefix(ref, "ev"):
		path = "raw/data/evimage/" + ref + ".png"
	}
	return &ir.AssetRef{Hash: "h-" + ref, OrigPath: path, Role: roleOf(path)}, true
}

func buildTestIR(t *testing.T, items []SeqItem) *ir.IR {
	t.Helper()
	ast := &Ast{Encoding: "utf-8"}
	for _, it := range items {
		if it.Kind == "cmd" && it.Cmd != nil {
			ast.Commands = append(ast.Commands, *it.Cmd)
		}
	}
	ast.Seq = items
	out, err := (&Adapter{}).BuildIR(ast, &adapter.BuildCtx{
		Config:   map[string]string{"script_name": "t.ks"},
		Resolver: testResolver,
	})
	if err != nil {
		t.Fatalf("BuildIR: %v", err)
	}
	return out
}

// TestParseScriptSkipsIScript：[iscript]…[endscript] 里的 JS 不能进台词（S3 spike 缺陷 1）。
func TestParseScriptSkipsIScript(t *testing.T) {
	src := "【甲】こんにちは\r\n" +
		"[iscript]\r\n" +
		"var x = 1;\r\n" +
		"kag.fore.layers[0].visible = false;\r\n" +
		"[endscript]\r\n" +
		"【乙】さようなら\r\n"
	raw, err := (&Adapter{}).ParseScript([]byte(src), &adapter.ScriptCtx{})
	if err != nil {
		t.Fatalf("ParseScript: %v", err)
	}
	ast, ok := raw.(*Ast)
	if !ok {
		t.Fatalf("ParseScript 返回 %T，期望 *Ast", raw)
	}
	if len(ast.Texts) != 2 {
		t.Fatalf("台词数 = %d，期望 2（iscript 块必须整块丢弃）：%q", len(ast.Texts), ast.Texts)
	}
	for _, tx := range ast.Texts {
		if strings.Contains(tx, "var x") || strings.Contains(tx, "kag.") || strings.Contains(tx, "visible") {
			t.Fatalf("JS 代码混进台词: %q", tx)
		}
	}
}

// TestParseScriptStripsSoundMark：♪path 内联音效标记不该混进台词（缺陷 2）。
func TestParseScriptStripsSoundMark(t *testing.T) {
	// 明文探测要过 kagScore >= 10 的门槛，故补几条标签行（不影响台词断言）
	src := "[wait time=100]\r\n" +
		"[trans time=200]\r\n" +
		"今日はいい天気ですね。♪raw/data/sound/click.ogg\r\n" +
		"[r]\r\n" +
		"[cm]\r\n"
	raw, err := (&Adapter{}).ParseScript([]byte(src), &adapter.ScriptCtx{})
	if err != nil {
		t.Fatalf("ParseScript: %v", err)
	}
	ast := raw.(*Ast)
	if len(ast.Texts) != 1 {
		t.Fatalf("台词数 = %d，期望 1", len(ast.Texts))
	}
	if strings.Contains(ast.Texts[0], "♪") || strings.Contains(ast.Texts[0], ".ogg") {
		t.Fatalf("♪ 资源标记未剥离: %q", ast.Texts[0])
	}
	if !strings.Contains(ast.Texts[0], "今日はいい天気ですね") {
		t.Fatalf("正文被误伤: %q", ast.Texts[0])
	}
}

// TestBuildIRLayerSemantics：层 id/kind/z/初态资产/几何 + layopt 隐藏 + wt 走时（缺陷 3/4/6）。
func TestBuildIRLayerSemantics(t *testing.T) {
	items := []SeqItem{
		{Kind: "cmd", Cmd: mkCmd("image", map[string]string{"layer": "base", "storage": "bg01", "page": "fore"})},
		{Kind: "cmd", Cmd: mkCmd("image", map[string]string{
			"layer": "3", "storage": "chr01", "page": "back", "left": "100", "top": "220"})},
		{Kind: "text", Text: "こんにちは"},
		{Kind: "cmd", Cmd: mkCmd("trans", map[string]string{"time": "1000"})},
		{Kind: "cmd", Cmd: mkCmd("wt", nil)},
		{Kind: "cmd", Cmd: mkCmd("image", map[string]string{"layer": "3", "storage": "chr02", "left": "120"})},
		{Kind: "cmd", Cmd: mkCmd("layopt", map[string]string{"layer": "3", "visible": "false"})},
	}
	out := buildTestIR(t, items)
	if len(out.Events) != 1 {
		t.Fatalf("事件数 = %d，期望 1（无 CG 标签 → 整脚本单 scene）", len(out.Events))
	}
	ev := out.Events[0]
	byID := map[string]ir.Layer{}
	for _, L := range ev.Layers {
		byID[L.LayerID] = L
	}

	// 层 id 规范化：base → layerbase；数值层号 → layer3（与 [layopt layer=3] 对齐）
	base, ok := byID["layerbase"]
	if !ok {
		t.Fatalf("缺少 layerbase（现有层：%v）", keysOf(byID))
	}
	if base.Kind != ir.LayerBackground {
		t.Errorf("layerbase.Kind = %q，期望 background", base.Kind)
	}
	if base.Z != 0 {
		t.Errorf("layerbase.Z = %d，期望 0", base.Z)
	}
	if !base.Visible {
		t.Errorf("layerbase.Visible = false，期望 true（初态）")
	}

	ch, ok := byID["layer3"]
	if !ok {
		t.Fatalf("缺少 layer3（现有层：%v）", keysOf(byID))
	}
	if ch.Kind != ir.LayerStand {
		t.Errorf("layer3.Kind = %q，期望 stand（/fgimage/ 路径判定）", ch.Kind)
	}
	if ch.Z != 3 {
		t.Errorf("layer3.Z = %d，期望 3（取 layer 属性而非出现序）", ch.Z)
	}
	if ch.Transform == nil {
		t.Fatalf("layer3.Transform = nil，期望带 left/top")
	}
	if ch.Transform.X == nil || *ch.Transform.X != 100 {
		t.Errorf("layer3.Transform.X = %v，期望 100", ch.Transform.X)
	}
	if ch.Transform.Y == nil || *ch.Transform.Y != 220 {
		t.Errorf("layer3.Transform.Y = %v，期望 220", ch.Transform.Y)
	}
	if ch.Transform.Anchor != "top-left" {
		t.Errorf("layer3.Transform.Anchor = %q，期望 top-left", ch.Transform.Anchor)
	}
	if ch.Asset == nil || !strings.Contains(ch.Asset.OrigPath, "chr01") {
		t.Errorf("layer3.Asset = %+v，期望**初态** chr01（不能被后续 show 覆盖）", ch.Asset)
	}

	// 时间轴：2 次 image_show + 1 次 image_hide；hide 必须落在 layer3 上
	var shows, hides, shows3 int
	var showParams map[string]interface{}
	var lastT int64
	for _, op := range ev.Timeline {
		switch op.Op {
		case "image_show":
			shows++
			if op.Target == "layer3" {
				shows3++
				if showParams == nil {
					showParams, _ = op.Params.(map[string]interface{})
				}
			}
		case "image_hide":
			hides++
			if op.Target != "layer3" {
				t.Errorf("image_hide 目标 = %q，期望 layer3", op.Target)
			}
		}
		if op.TMs < lastT {
			t.Errorf("时间轴 t_ms 非降序：%d 之后出现 %d", lastT, op.TMs)
		}
		lastT = op.TMs
	}
	// base 显示 1 次 + layer3 显示 2 次（chr01/chr02）+ layer3 隐藏 1 次
	if shows != 3 || hides != 1 {
		t.Errorf("image_show=%d image_hide=%d，期望 3/1", shows, hides)
	}
	if shows3 != 2 {
		t.Errorf("layer3 的 image_show = %d，期望 2（chr01 与 chr02 各一次）", shows3)
	}
	if showParams == nil {
		t.Fatalf("layer3 的 image_show 没有 params")
	}
	if got, _ := showParams["left"].(string); got != "100" { // Params 里的左位置
		t.Errorf("image_show params.left = %v，期望 \"100\"", showParams["left"])
	}
	if _, ok := showParams["top"].(string); !ok {
		t.Errorf("image_show params 缺 top：%v", showParams)
	}
	// [trans time=1000] 先走一半（500），[wt] 补另一半 → 之后还有后续命令，总时钟必须 > 0
	if lastT == 0 {
		t.Errorf("时间轴全程 t_ms=0：转场/wt 未参与走时")
	}
}

func keysOf[T any](m map[string]T) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
