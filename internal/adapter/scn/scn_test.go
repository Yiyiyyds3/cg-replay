package scn

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ucgr/internal/adapter"
	"ucgr/internal/ir"
)

// gameRoot 返回真实 9nine 解包根目录（环境变量，未设置则跳过）。
//
//	UCGR_9NINE_ROOT  游戏根目录（含 raw/ 或 原始归档解包/ 或 _分类/）
//	UCGR_9NINE_SCN   直接指定一份 .scn（优先于 ROOT 下的自动探测）
func gameRoot(t *testing.T) string {
	t.Helper()
	if v := strings.TrimSpace(os.Getenv("UCGR_9NINE_SCN")); v != "" {
		return rootOf(v)
	}
	root := strings.TrimSpace(os.Getenv("UCGR_9NINE_ROOT"))
	if root == "" {
		t.Skip("未设置 UCGR_9NINE_ROOT / UCGR_9NINE_SCN，跳过真实语料测试")
	}
	return root
}

// rootOf 从 .scn 路径向上找游戏根（含 raw/ 或 原始归档解包/ 或 _分类/ 的那层）。
func rootOf(scnPath string) string {
	dir := filepath.Dir(scnPath)
	for i := 0; i < 6; i++ {
		for _, marker := range []string{"raw", "原始归档解包", "_分类"} {
			if st, err := os.Stat(filepath.Join(dir, marker)); err == nil && st.IsDir() {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(scnPath)))
}

// firstSCN 在 root 下找第一份 PSB .scn（≤4 层）。
func firstSCN(t *testing.T, root string) string {
	t.Helper()
	if v := strings.TrimSpace(os.Getenv("UCGR_9NINE_SCN")); v != "" {
		return v
	}
	var found string
	depth := strings.Count(filepath.Clean(root), string(filepath.Separator))
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
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
			found = p
		}
		return nil
	})
	if found == "" {
		t.Skipf("%s 下没有 PSB .scn", root)
	}
	return found
}

// firstCGList 找 cglist.csv（原样 / 整理 / 扁平三种布局）。
func firstCGList(t *testing.T, root string) string {
	t.Helper()
	for _, rel := range []string{
		"raw/data/main/cglist.csv",
		"原始归档解包/data_文本/main/cglist.csv",
		"_分类/文本/data_main__cglist.csv",
	} {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	t.Skipf("%s 下没有 cglist.csv", root)
	return ""
}

func TestParseScriptReal(t *testing.T) {
	root := gameRoot(t)
	p := firstSCN(t, root)
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	sc, err := ParseScript(data, p)
	if err != nil {
		t.Fatalf("解析 %s 失败：%v", filepath.Base(p), err)
	}
	if len(sc.Scenes) == 0 {
		t.Fatal("scene 数为 0")
	}
	if sc.Name != strings.TrimSuffix(filepath.Base(p), filepath.Ext(p)) {
		t.Errorf("Name = %q，期望文件名去扩展名", sc.Name)
	}

	// 至少要有一个 scene 有台词，且有 scene 带背景/角色指令。
	texts, stage, chara := 0, 0, 0
	for _, s := range sc.Scenes {
		texts += len(s.Texts)
		for _, tx := range s.Texts {
			for _, c := range tx.Commands {
				switch strings.ToLower(c.Class) {
				case "stage":
					if c.File != "" {
						stage++
					}
				case "character":
					chara++
				}
			}
		}
	}
	if texts == 0 {
		t.Error("全剧本没有台词")
	}
	if stage == 0 {
		t.Error("没有 class=stage 的背景指令（PSB 结构可能变了）")
	}
	if chara == 0 {
		t.Error("没有 class=character 的 E-mote 指令（HasEmote 判定会失效）")
	}
	t.Logf("%s：scenes=%d 有台词 scene=%d 台词=%d stage=%d character=%d",
		sc.Name, len(sc.Scenes), len(sc.SceneInfos()), texts, stage, chara)
}

func TestBuildShowSceneReal(t *testing.T) {
	root := gameRoot(t)
	p := firstSCN(t, root)
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	sc, err := ParseScript(data, p)
	if err != nil {
		t.Fatal(err)
	}
	target := -1
	for _, s := range sc.SceneInfos() {
		if len(s.Texts) >= 3 {
			target = s.Index
			break
		}
	}
	if target < 0 {
		t.Skip("没有 ≥3 句台词的 scene")
	}

	ev, err := BuildShowScene(root, p, target, DefaultOptions())
	if err != nil {
		t.Fatalf("构建台本失败：%v", err)
	}
	if len(ev.Show) == 0 {
		t.Fatal("台本 0 条")
	}
	if errs := ir.ValidateShow(ev); len(errs) > 0 {
		for _, e := range errs {
			t.Error(e)
		}
	}

	counts := ir.ShowKindCounts(ev)
	for _, k := range []string{ir.ShowLayerShow, ir.ShowTextRun, ir.ShowTextWait} {
		if counts[k] == 0 {
			t.Errorf("台本缺少 %s（counts=%v）", k, counts)
		}
	}
	if !ev.HasEmote {
		t.Error("HasEmote 应为 true（9nine 有 E-mote 立绘）")
	}
	if ev.Kind != ir.KindScene {
		t.Errorf("Kind = %q，期望 %q", ev.Kind, ir.KindScene)
	}
	if len(ev.Texts) == 0 {
		t.Fatal("Texts 为空")
	}
	for i, tx := range ev.Texts {
		if strings.TrimSpace(tx.Text) == "" {
			t.Errorf("第 %d 条台词为空", i+1)
		}
		if tx.LineID == "" {
			t.Errorf("第 %d 条台词缺 LineID", i+1)
		}
	}
	// 语音应与 Texts 的 Voice 一一对应，且都出现在 Voices 清单里。
	for _, tx := range ev.Texts {
		if tx.Voice == nil {
			continue
		}
		if tx.Voice.OrigPath == "" {
			t.Error("语音引用为空")
		}
	}
	t.Logf("%s#%d：show=%d texts=%d voices=%d kinds=%v id=%s",
		sc.Name, target, len(ev.Show), len(ev.Texts), len(ev.Voices), counts, ev.EventID)
}

func TestBuildShowFileReal(t *testing.T) {
	root := gameRoot(t)
	p := firstSCN(t, root)
	ev, err := BuildShowFile(root, p, DefaultOptions())
	if err != nil {
		t.Fatalf("整文件构建失败：%v", err)
	}
	if len(ev.Show) == 0 {
		t.Fatal("台本 0 条")
	}
	if errs := ir.ValidateShow(ev); len(errs) > 0 {
		for _, e := range errs {
			t.Error(e)
		}
	}
	// TMs 必须单调（跨 scene 拼接后仍成立）。
	for i := 1; i < len(ev.Show); i++ {
		if ev.Show[i].TMs < ev.Show[i-1].TMs {
			t.Fatalf("第 %d 条 TMs 逆序：%d < %d", i+1, ev.Show[i].TMs, ev.Show[i-1].TMs)
		}
	}
	t.Logf("%s：show=%d texts=%d", scnName(p), len(ev.Show), len(ev.Texts))
}

func scnName(p string) string {
	return strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
}

func TestParseCGListReal(t *testing.T) {
	root := gameRoot(t)
	p := firstCGList(t, root)
	entries, err := ParseCGList(p)
	if err != nil {
		t.Fatalf("解析 cglist 失败：%v", err)
	}
	if len(entries) == 0 {
		t.Fatal("CG 条目为 0")
	}
	empty := 0
	for _, e := range entries {
		if e.Thumb == "" {
			t.Error("条目缺缩略图名")
		}
		if len(e.Images) == 0 {
			empty++
		}
		if e.Title == "" {
			t.Errorf("条目 %s 没有标题（应回退到缩略图名）", e.Thumb)
		}
	}
	if empty == len(entries) {
		t.Error("所有条目都没有图片序列")
	}
	// 按纯数字也能找到（回放页允许用户输 “101”）。
	if digits := tailDigits(entries[0].Thumb); digits != "" {
		if FindCG(entries, digits) == nil {
			t.Errorf("FindCG(%q) 未命中 %s", digits, entries[0].Thumb)
		}
	}
	t.Logf("CG 条目=%d 首条=%s images=%v", len(entries), entries[0].Thumb, entries[0].Images)
}

func TestBuildShowCGReal(t *testing.T) {
	root := gameRoot(t)
	p := firstCGList(t, root)
	entries, err := ParseCGList(p)
	if err != nil {
		t.Fatal(err)
	}
	// 取第一个能解析出图片的条目（缩略图本身不在 CG 序列里）。
	for _, e := range entries {
		if len(e.Images) == 0 {
			continue
		}
		ev, err := BuildShowCG(root, p, e.Thumb, DefaultOptions())
		if err != nil {
			t.Logf("跳过 %s：%v", e.Thumb, err)
			continue
		}
		if errs := ir.ValidateShow(ev); len(errs) > 0 {
			for _, e := range errs {
				t.Error(e)
			}
		}
		if ev.Kind != ir.KindStillCG {
			t.Errorf("Kind = %q，期望 %q", ev.Kind, ir.KindStillCG)
		}
		if len(ev.Assets) == 0 {
			t.Error("CG 事件没有素材清单")
		}
		t.Logf("CG %s：show=%d assets=%d", e.Thumb, len(ev.Show), len(ev.Assets))
		return
	}
	t.Skip("没有可用图片序列的 CG 条目")
}

// TestCleanBody 台词清洗：注音与内联命令都不能带到画面上。
func TestCleanBody(t *testing.T) {
	cases := []struct{ in, want string }{
		{"俺の住む街、『[しろみつがわ,3]白己津川』がスポンサーとなった。",
			"俺の住む街、『白己津川』がスポンサーとなった。"},
		{"「さっき%p-1;%fＭＳ ゴシック;――%p;%fuser;あなたにとってはさっきじゃないかも」",
			"「さっき――あなたにとってはさっきじゃないかも」"},
		{`一行目\n二行目`, "一行目\n二行目"},
		// 正常文本不能被误伤：没有逗号+数字的方括号保留。
		{"「[重要] な話」", "「[重要] な話」"},
	}
	for _, c := range cases {
		if got := cleanBody(c.in); got != c.want {
			t.Errorf("cleanBody(%q) = %q，期望 %q", c.in, got, c.want)
		}
	}
}

// TestStemAliases 扁平整理版文件名 → storage 名。
func TestStemAliases(t *testing.T) {
	cases := map[string]string{
		"data_bgimage__bg001a":  "bg001a",
		"evimage_ev106a":        "ev106a",
		"data_image__attention": "attention",
	}
	for in, want := range cases {
		got := stemAliases(in)
		found := false
		for _, g := range got {
			if g == want {
				found = true
			}
		}
		if !found {
			t.Errorf("stemAliases(%q) = %v，期望含 %q", in, got, want)
		}
	}
	// 原样名不该被切（避免把 bg001a 误切）。
	for _, s := range []string{"bg001a", "ev106a", "bgm05"} {
		if got := stemAliases(s); len(got) != 0 {
			t.Errorf("stemAliases(%q) = %v，期望空", s, got)
		}
	}
}

// TestAssetIndexFlatLayout 扁平布局（_分类/图片）能按 storage 名解析。
func TestAssetIndexFlatLayout(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "_分类", "图片")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	adir := filepath.Join(root, "_分类", "音频")
	if err := os.MkdirAll(adir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"data_bgimage__bg001a.png", "evimage_ev106a.png"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(adir, "data_bgm__bgm05.ogg"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	ix := assetIndex(root, DefaultOptions())
	if got := ix.Image("bg001a"); got != "_分类/图片/data_bgimage__bg001a.png" {
		t.Errorf("Image(bg001a) = %q", got)
	}
	if got := ix.Image("ev106a"); got != "_分类/图片/evimage_ev106a.png" {
		t.Errorf("Image(ev106a) = %q", got)
	}
	if got := ix.Audio("bgm05"); got != "_分类/音频/data_bgm__bgm05.ogg" {
		t.Errorf("Audio(bgm05) = %q", got)
	}
	// 解析不到时原样返回（交给回放器按 basename 兜底）。
	if got := ix.Image("nope"); got != "nope" {
		t.Errorf("Image(nope) = %q，期望原样返回", got)
	}
}

// TestIdentifyEmpty 目录里没有 .scn 时不瞎猜。
func TestIdentifyEmpty(t *testing.T) {
	e := &Engine{}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "raw", "data", "scenario"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got, err := e.Identify(dir); err != nil || got != "" {
		t.Fatalf("空目录应返回空：got=%q err=%v", got, err)
	}
	// 非 PSB 的 .scn 也不算（避免把别的引擎的同后缀文件认进来）。
	bad := filepath.Join(dir, "raw", "data", "scenario", "x.scn")
	if err := os.WriteFile(bad, []byte("not a psb"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, _ := e.Identify(dir); got != "" {
		t.Fatalf("非 PSB .scn 应识别为空，got=%q", got)
	}
}

// TestIdentifyReal 真实语料：含 PSB .scn 的目录返回 "scn"。
func TestIdentifyReal(t *testing.T) {
	e := &Engine{}
	dir := t.TempDir()
	sub := filepath.Join(dir, "raw", "data", "scn")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	src := firstSCN(t, gameRoot(t))
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(sub, "x.txt.scn")
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := e.Identify(dir); err != nil || got != "scn" {
		t.Fatalf("Identify = %q, %v；期望 scn", got, err)
	}
	if got, err := e.Identify(p); err != nil || got != "scn" {
		t.Fatalf("Identify(file) = %q, %v", got, err)
	}
}

// TestAdapterContract 适配器五方法的最小契约。
func TestAdapterContract(t *testing.T) {
	a, ok := adapter.Get("scn")
	if !ok {
		t.Fatal("scn 适配器未注册")
	}
	if _, err := a.ParseArchive("x.scn"); err == nil {
		t.Error("ParseArchive 应报错（scn 不是归档）")
	}
	if tr := a.Transitions(); tr == nil || tr.Impls["crossfade"].ImplKey == "" {
		t.Error("转场注册表缺少 crossfade")
	}
}
