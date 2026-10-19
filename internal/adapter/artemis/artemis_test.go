package artemis

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ucgr/internal/adapter"
	"ucgr/internal/ir"
)

// testAST 是一份覆盖主要语法的 fixture：两种 block 键写法、位置参数、
// 命名参数、text 载荷（name/rt2/vo）、转场、选择支与未知命令。
const testAST = `astver = 2.0
astname = "ast"
ast = {
	block_00000 = {
		{"savetitle", text="銀色、遥か"},
		{"bg", time=2000, file="black", path=":bg/"},
		{"bg", file="ev_cut_001_00", id=1, lv=5, path=":cg/", sync=0},
		{"ex", time=2000, func="wait"},
		{"text"},
		{"extrans", time=1500, rule="fade"},
		{"fg", file="chara_a", ch=1, path=":fg/tami/z1/"},
		{"cgdel", id=1, lv=5, time=500, rule="wipe"},
		{"select", label="1,2", text="選ぶ,選ばない"},
		{"unknowncmd", file="mystery_asset"},
		text = {
			vo = {
				{"vo", file="fem_a_0001", ch="tami"},
			},
			ja = {
				{ name={"少女","しょうじょ"}, "「こんにちは」", {"rt2"}, "その２行目です。", {"rt2"}, },
			},
		},
		linkback = "block_00000",
		linknext = "block_00002",
		line = 887,
	},
	["0001_00001"] = {
		{"evset", file="ev_cut_002_00", set="ev_cut_002", path=":ev/"},
		{"ex", time=1000, func="wait"},
		text = {
			ja = {
				{ "二つ目のブロック", {"rt2"}, },
			},
		},
	},
	label = {
		top = { block="block_00000", label=1 },
	},
}
`

func parseTestAST(t *testing.T, src string) *Ast {
	t.Helper()
	a := &Adapter{}
	obj, err := a.ParseScript([]byte(src), &adapter.ScriptCtx{EntryPath: "test.ast"})
	if err != nil {
		t.Fatalf("ParseScript 失败: %v", err)
	}
	ast, ok := obj.(*Ast)
	if !ok {
		t.Fatalf("ParseScript 返回类型 = %T，想要 *Ast", obj)
	}
	return ast
}

func testResolver(ref string) (*ir.AssetRef, bool) {
	return &ir.AssetRef{Hash: "h-" + ref, OrigPath: ref, Role: "其它"}, true
}

func TestParseScriptStructure(t *testing.T) {
	ast := parseTestAST(t, testAST)
	if len(ast.Blocks) != 2 {
		t.Fatalf("block 数 = %d，想要 2", len(ast.Blocks))
	}
	b0 := ast.Blocks[0]
	if b0.Name != "block_00000" {
		t.Errorf("block0 名 = %q", b0.Name)
	}
	if b0.Title != "銀色、遥か" {
		t.Errorf("block0 标题 = %q，想要 savetitle 的 text=", b0.Title)
	}
	if b0.LinkBack != "block_00000" || b0.LinkNext != "block_00002" || b0.Line != 887 {
		t.Errorf("block0 串接字段 = %q/%q/%d", b0.LinkBack, b0.LinkNext, b0.Line)
	}
	if ast.Blocks[1].Name != "0001_00001" {
		t.Errorf("block1 名 = %q（应支持 [\"…\"] 键写法）", ast.Blocks[1].Name)
	}
	if got := ast.Labels["top"]; got != 0 {
		t.Errorf("label top = %d，想要 0", got)
	}

	// 命令：操作数/属性分开
	var bg *Command
	for _, it := range b0.Seq {
		if it.Kind == "cmd" && it.Cmd != nil && it.Cmd.Tag == "bg" {
			bg = it.Cmd
			break
		}
	}
	if bg == nil {
		t.Fatal("没找到 bg 命令")
	}
	if bg.Attrs["file"] != "black" || bg.Attrs["time"] != "2000" {
		t.Errorf("bg 属性 = %v", bg.Attrs)
	}

	// 文本行：说话人 + rt2 换行 + 语音配对
	lines := ast.TextLines()
	if len(lines) != 2 {
		t.Fatalf("文本行数 = %d，想要 2", len(lines))
	}
	if lines[0].Speaker != "少女" {
		t.Errorf("第一行说话人 = %q，想要 少女", lines[0].Speaker)
	}
	want := "「こんにちは」\nその２行目です。"
	if lines[0].Text != want {
		t.Errorf("第一行文本 = %q，想要 %q", lines[0].Text, want)
	}
	if lines[0].Voice != "fem_a_0001" {
		t.Errorf("第一行语音 = %q", lines[0].Voice)
	}
	if lines[1].Speaker != "" || lines[1].Voice != "" {
		t.Errorf("第二行不该有说话人/语音：%q %q", lines[1].Speaker, lines[1].Voice)
	}
}

func TestExtractRefsStripsAlias(t *testing.T) {
	ast := parseTestAST(t, testAST)
	refs := SortedRefs(ast)
	want := []string{"black", "chara_a", "ev_cut_001_00", "ev_cut_002_00", "fem_a_0001", "mystery_asset"}
	if len(refs) != len(want) {
		t.Fatalf("引用数 = %d（%v），想要 %d（%v）", len(refs), refs, len(want), want)
	}
	for i := range want {
		if refs[i] != want[i] {
			t.Errorf("引用[%d] = %q，想要 %q", i, refs[i], want[i])
		}
	}
	// 带别名的值必须剥掉前缀（P2 报告口径：贪心剥掉 ^:[A-Za-z0-9_/]*/ 整段）
	if got := cleanRef(":fg/tami/z1/chara_a"); got != "chara_a" {
		t.Errorf("cleanRef 剥别名 = %q", got)
	}
	if got := cleanRef(":ani/kiss"); got != "kiss" {
		t.Errorf("cleanRef 剥别名（:ani/）= %q", got)
	}
	if got := cleanRef("chara_a"); got != "chara_a" {
		t.Errorf("cleanRef 不动无别名的值 = %q", got)
	}
}

func TestBuildIRGolden(t *testing.T) {
	ast := parseTestAST(t, testAST)
	a := &Adapter{}
	out, err := a.BuildIR(ast, &adapter.BuildCtx{
		GameID:   "testgame",
		Config:   map[string]string{"script_name": "test.ast"},
		Resolver: testResolver,
	})
	if err != nil {
		t.Fatalf("BuildIR 失败: %v", err)
	}
	if out.EngineID != "artemis" || out.Fidelity != ir.FidelityGeneric {
		t.Errorf("引擎/保真度 = %q/%q", out.EngineID, out.Fidelity)
	}
	if len(out.Events) != 2 {
		t.Fatalf("事件数 = %d，想要 2", len(out.Events))
	}
	e0, e1 := out.Events[0], out.Events[1]
	if e0.EventID != "test#block_00000" || e1.EventID != "test#0001_00001" {
		t.Errorf("事件 ID = %q / %q", e0.EventID, e1.EventID)
	}
	if e0.Kind != ir.KindScene || e1.Kind != ir.KindStillCG {
		t.Errorf("事件 kind = %q / %q（含 evset 者应为 still_cg）", e0.Kind, e1.Kind)
	}
	if e0.Title != "銀色、遥か" {
		t.Errorf("事件标题 = %q", e0.Title)
	}
	if e0.EndMs <= 0 {
		t.Errorf("事件时长 = %d，想要 > 0", e0.EndMs)
	}

	// 文本 + 语音绑定（V5）
	if len(e0.Texts) != 1 || e0.Texts[0].Voice == nil {
		t.Fatalf("事件 0 文本/语音 = %d/%v", len(e0.Texts), e0.Texts[0].Voice)
	}
	if e0.Texts[0].Voice.OrigPath != "fem_a_0001" {
		t.Errorf("语音引用 = %q", e0.Texts[0].Voice.OrigPath)
	}
	if len(e0.Voices) != 1 || e0.Voices[0].LineID != e0.Texts[0].LineID {
		t.Errorf("Voices 与 Texts 未对齐: %+v", e0.Voices)
	}

	// 图层种类
	byID := map[string]ir.Layer{}
	for _, l := range e0.Layers {
		byID[l.LayerID] = l
	}
	if l, ok := byID["bg"]; !ok || l.Kind != ir.LayerBackground {
		t.Errorf("bg 层 = %+v", byID["bg"])
	}
	if l, ok := byID["bg1"]; !ok || l.Kind != ir.LayerCG {
		t.Errorf("bg1 层 = %+v", byID["bg1"])
	}
	if l, ok := byID["fg1"]; !ok || l.Kind != ir.LayerStand {
		t.Errorf("fg1 层 = %+v", byID["fg1"])
	}

	// 时间轴单调（V3）
	last := int64(-1)
	for _, op := range e0.Timeline {
		if op.TMs < last {
			t.Fatalf("时间轴回退: %d < %d", op.TMs, last)
		}
		last = op.TMs
	}
	// 转场必须 impl 合法或带 fallback（V7）
	for _, tr := range e0.Transitions {
		if !ir.IsL0Impl(tr.Impl) && tr.Fallback != true {
			t.Errorf("转场 %+v 既非 L0 也没标 fallback", tr)
		}
	}
	// 选择支
	if len(e0.Branches) != 1 || len(e0.Branches[0].Options) != 2 {
		t.Errorf("选择支 = %+v", e0.Branches)
	}
	// 未知命令必须保守保留
	if len(e0.Unrecognized) != 1 || !strings.Contains(e0.Unrecognized[0].Raw, "unknowncmd") {
		t.Errorf("unrecognized = %+v", e0.Unrecognized)
	}
	// 整份 IR 过校验
	if errs := ir.Validate(out); len(errs) != 0 {
		t.Fatalf("IR 校验失败 %d 条: %v", len(errs), errs)
	}
}

func TestBuildIRUnknownKeepsAssets(t *testing.T) {
	ast := parseTestAST(t, testAST)
	a := &Adapter{}
	out, err := a.BuildIR(ast, &adapter.BuildCtx{GameID: "g", Resolver: testResolver})
	if err != nil {
		t.Fatalf("BuildIR 失败: %v", err)
	}
	e := out.Events[0]
	if len(e.Unrecognized) != 1 {
		t.Fatalf("unrecognized 数 = %d", len(e.Unrecognized))
	}
	hashes := map[string]bool{}
	for _, ar := range e.Assets {
		hashes[ar.Hash] = true
	}
	for _, h := range e.Unrecognized[0].KeptAssets {
		if !hashes[h] {
			t.Errorf("kept_assets %q 不在 assets 里（V6 会失败）", h)
		}
	}
	if len(e.Unrecognized[0].KeptAssets) == 0 {
		t.Error("未知命令的引用应被保守保留")
	}
}

func TestIdentify(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "01_01.ast"), []byte("astver = 2.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := (&Adapter{}).Identify(dir)
	if err != nil || got != "artemis" {
		t.Fatalf("Identify(.ast 目录) = %q, %v", got, err)
	}
	// .xp3（KiriKiri）不该被认领
	dir2 := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir2, "data.xp3"), []byte("XP3"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir2, "01_01.ast"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := (&Adapter{}).Identify(dir2); err != nil || got != "" {
		t.Fatalf("Identify(.xp3 目录) = %q, %v（应放弃认领）", got, err)
	}
	// 单文件
	f := filepath.Join(dir, "01_01.ast")
	if got, err := (&Adapter{}).Identify(f); err != nil || got != "artemis" {
		t.Fatalf("Identify(.ast 文件) = %q, %v", got, err)
	}
	// .pfs 归档：不静默失败
	if _, err := (&Adapter{}).ParseArchive(filepath.Join(dir, "x.pfs")); err == nil {
		t.Error("ParseArchive(.pfs) 应报明确错误")
	} else if !strings.Contains(err.Error(), ".pfs") {
		t.Errorf("ParseArchive 错误应提到 .pfs: %v", err)
	}
}

// TestRealCorpus 用实在的解包树跑一遍全部 .ast（UCGR_ARTEMIS_DIR 指向游戏根）。
func TestRealCorpus(t *testing.T) {
	root := os.Getenv("UCGR_ARTEMIS_DIR")
	if root == "" {
		t.Skip("未设置 UCGR_ARTEMIS_DIR，跳过真机语料")
	}
	a := &Adapter{}
	var files, blocks, texts, failed int
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if !strings.EqualFold(filepath.Ext(path), scriptExt) {
			return nil
		}
		files++
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			failed++
			return nil
		}
		obj, perr := a.ParseScript(data, &adapter.ScriptCtx{EntryPath: path})
		if perr != nil {
			failed++
			t.Logf("解析失败 %s: %v", path, perr)
			return nil
		}
		ast := obj.(*Ast)
		blocks += len(ast.Blocks)
		texts += len(ast.TextLines())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("语料：%d 个 .ast / %d 个 block / %d 条文本，失败 %d", files, blocks, texts, failed)
	if files == 0 {
		t.Fatal("没有找到 .ast")
	}
	if failed != 0 {
		t.Fatalf("%d 个脚本解析失败", failed)
	}
}
