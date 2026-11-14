package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ucgr/internal/adapter"
	"ucgr/internal/library"
	"ucgr/internal/unpack"
)

// —— TSV 解析 ——

func TestParseTSVHandlesTabsChineseAndApostrophe(t *testing.T) {
	text := "\ufeff" +
		"大雷\t女主角（大雷）\t大雷/女主角（大雷）\tNW.js / RPG Maker(MV/MZ)\teasy\r\n" +
		"\n" + // 空行
		"# 注释行\n" +
		"未评级\tTempPC\t未评级/TempPC\tRen'Py\thard\r\n" +
		"牢\tGoddesses' Whim\t牢/Goddesses' Whim/子目录\tUnity\teasy\n"

	rows, err := parseTSV(text)
	if err != nil {
		t.Fatalf("parseTSV 失败：%v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("行数 = %d，想要 3（空行/注释行要跳过）：%+v", len(rows), rows)
	}
	if rows[0].Group != "大雷" || rows[0].Game != "女主角（大雷）" {
		t.Errorf("第 1 行中文/字段错：%+v", rows[0])
	}
	if rows[0].RelPath != "大雷/女主角（大雷）" || rows[0].EngineGuess != "NW.js / RPG Maker(MV/MZ)" || rows[0].Difficulty != "easy" {
		t.Errorf("第 1 行其余字段错：%+v", rows[0])
	}
	// 单引号必须原样保留（不许被当成 SQL/引号转义吃掉）
	if rows[1].Game != "TempPC" || rows[1].EngineGuess != "Ren'Py" {
		t.Errorf("单引号游戏名被改坏：%+v", rows[1])
	}
	if rows[2].Game != "Goddesses' Whim" {
		t.Errorf("含单引号的游戏名解析错：%q", rows[2].Game)
	}
	if rows[2].RelPath != "牢/Goddesses' Whim/子目录" {
		t.Errorf("相对路径解析错：%q", rows[2].RelPath)
	}
	if rows[0].LineNo != 1 || rows[2].LineNo != 5 {
		t.Errorf("行号错：%d / %d", rows[0].LineNo, rows[2].LineNo)
	}
}

func TestParseTSVRejectsShortRows(t *testing.T) {
	if _, err := parseTSV("只有一列\n"); err == nil {
		t.Fatal("列数不足应当报错")
	}
}

func TestFilterRowsAndSplitGroups(t *testing.T) {
	rows, err := parseTSV("大雷\tA\t大雷/A\tGodot\thard\n很牢\tB\t很牢/B\tGodot\thard\n牢\tC\t牢/C\tUnity\teasy\n")
	if err != nil {
		t.Fatalf("parseTSV 失败：%v", err)
	}
	got := filterRows(rows, splitGroups(" 大雷 , 牢 ,, 大雷 "))
	if len(got) != 2 || got[0].Game != "A" || got[1].Game != "C" {
		t.Fatalf("分组过滤结果错：%+v", got)
	}
	if all := filterRows(rows, nil); len(all) != 3 {
		t.Fatalf("不给分组应当全跑，实际 %d", len(all))
	}
}

func TestNormalizeEngineGuess(t *testing.T) {
	cases := map[string]string{
		"Unity":                    "unity",
		"NW.js / RPG Maker(MV/MZ)": "rmmz",
		"已适配(ucgr)":                "rmmz",
		"NScripter/ONScripter":     "nscript",
		"Ren'Py":                   "renpy",
		"Unreal Engine":            "unreal",
		"Godot":                    "godot",
		"未知":                       "",
		"未知(仅 game/ 目录)":           "",
		"":                         "",
	}
	for in, want := range cases {
		if got := normalizeEngineGuess(in); got != want {
			t.Errorf("normalizeEngineGuess(%q) = %q，想要 %q", in, got, want)
		}
	}
}

// —— verdict 判定 ——

// newTestAuditor 返回一个全可注入的 auditor（detect/scan/preview/probe 全部打桩）。
func newTestAuditor() *auditor {
	return &auditor{
		timeout: 2 * time.Second,
		detect:  func(string) string { return "" },
		scanWith: func(string, adapter.Convention) (*library.Library, error) {
			t := errors.New("默认桩：不该被调用")
			return nil, t
		},
		preview: func(string) (*unpack.Plan, error) { return &unpack.Plan{Unsupported: true, Reason: "桩"}, nil },
		probe:   func(string, string, *library.Library) (int, int, int, []string, []string) { return 0, 0, 0, nil, nil },
	}
}

func tempGameDir(t *testing.T, name string) string {
	t.Helper()
	dir := t.TempDir()
	game := filepath.Join(dir, name)
	if err := os.MkdirAll(game, 0o755); err != nil {
		t.Fatalf("建临时游戏目录失败：%v", err)
	}
	return game
}

func rowFor(guess string) TSVRow {
	return TSVRow{Group: "牢", Game: "样例", RelPath: "牢/样例", EngineGuess: guess, LineNo: 1}
}

func TestAuditNoEntryEngineSkips(t *testing.T) {
	a := newTestAuditor()
	a.detect = func(string) string { return "godot" } // Godot 在 main 无 ScanXxx 分派
	dir := tempGameDir(t, "SOTDQ")

	v := a.audit(rowFor("Godot"), dir)
	if v.Verdict != "skip" {
		t.Fatalf("verdict = %q，想要 skip", v.Verdict)
	}
	if v.Reason != "engine_unsupported_in_main" {
		t.Errorf("reason = %q，想要 engine_unsupported_in_main", v.Reason)
	}
	if !containsSubstr(v.Errors, "main 无该引擎 ScanXxx 分派") {
		t.Errorf("errors 里必须写明「main 无该引擎 ScanXxx 分派」，实际：%v", v.Errors)
	}
	if v.EngineDetected != "godot" || v.EngineGuess != "Godot" {
		t.Errorf("engine_detected/engine_guess 要并列记录：%q / %q", v.EngineDetected, v.EngineGuess)
	}
}

func TestAuditUnknownEngineSkipsWithGuessFallback(t *testing.T) {
	a := newTestAuditor()
	a.detect = func(string) string { return "" } // DetectEngine 认不出
	dir := tempGameDir(t, "未知容器")
	v := a.audit(rowFor("未知"), dir)
	if v.Verdict != "skip" || v.Reason != "engine_unsupported_in_main" {
		t.Fatalf("未知引擎应当 skip，实际 %q/%q", v.Verdict, v.Reason)
	}
	if v.EngineResolved != "" {
		t.Errorf("认不出时 engine_resolved 应为空，实际 %q", v.EngineResolved)
	}
}

func TestAuditGuessFallbackDrivesScan(t *testing.T) {
	a := newTestAuditor()
	a.detect = func(string) string { return "" } // 只能靠 TSV 的 engine_guess 兜底
	called := ""
	a.scanWith = func(_ string, conv adapter.Convention) (*library.Library, error) {
		called = conv.EngineID
		return &library.Library{EngineID: "unity", Scenes: []library.SceneEvent{{ID: "a"}}}, nil
	}
	dir := tempGameDir(t, "unity游戏")
	v := a.audit(rowFor("Unity"), dir)
	if called != "unity" {
		t.Fatalf("兜底约定 EngineID = %q，想要 unity", called)
	}
	if v.Verdict != "pass" || v.Events != 1 {
		t.Fatalf("events>0 应判 pass，实际 %q events=%d", v.Verdict, v.Events)
	}
}

func TestAuditPassWhenEventsPositive(t *testing.T) {
	a := newTestAuditor()
	a.detect = func(string) string { return "kirikiri" }
	a.scanWith = func(string, adapter.Convention) (*library.Library, error) {
		return &library.Library{
			EngineID: "kirikiri",
			CGs:      []library.CGEvent{{ID: "cg001"}},
			Scenes:   []library.SceneEvent{{ID: "01a"}, {ID: "02b"}},
			Chapters: []library.ChapterEvent{{ID: "01a#0"}},
		}, nil
	}
	a.probe = func(engine, root string, lib *library.Library) (int, int, int, []string, []string) {
		return 42, 7, 1, []string{"探测 1 个章节"}, nil
	}
	dir := tempGameDir(t, "kirikiri游戏")
	v := a.audit(rowFor("KiriKiri"), dir)
	if v.Verdict != "pass" || v.Reason != "events>0" {
		t.Fatalf("verdict/reason 错：%q/%q（errors=%v）", v.Verdict, v.Reason, v.Errors)
	}
	if v.CG != 1 || v.Scenes != 2 || v.Chapters != 1 || v.Events != 3 {
		t.Errorf("计数错：cg=%d scenes=%d chapters=%d events=%d", v.CG, v.Scenes, v.Chapters, v.Events)
	}
	if v.TextLines != 42 || v.Voices != 7 || v.IRProbed != 1 {
		t.Errorf("台词/语音/取样数错：%d/%d/%d", v.TextLines, v.Voices, v.IRProbed)
	}
}

func TestAuditScanErrorKeepsRawError(t *testing.T) {
	a := newTestAuditor()
	a.detect = func(string) string { return "kirikiri" }
	raw := "读不到剧本目录 D:\\x\\raw\\data\\scenario（open：系统找不到指定的路径）；自动探测也没找到 .ks 剧本"
	a.scanWith = func(string, adapter.Convention) (*library.Library, error) {
		return nil, errors.New(raw)
	}
	a.preview = func(string) (*unpack.Plan, error) {
		return &unpack.Plan{Unsupported: true, Reason: "这层只有打包文件（game.xp3，属 kirikiri 引擎），还没解包"}, nil
	}
	dir := tempGameDir(t, "坏目录")
	v := a.audit(rowFor("KiriKiri"), dir)
	if v.Verdict != "fail" || v.Reason != "scan_error" {
		t.Fatalf("verdict/reason 错：%q/%q", v.Verdict, v.Reason)
	}
	if !containsSubstr(v.Errors, raw) {
		t.Fatalf("原始报错必须完整进 errors，实际：%v", v.Errors)
	}
	if v.CanUnpack {
		t.Errorf("Unsupported 的预览不能算可解包")
	}
	if !containsSubstr(v.UnpackNotes, "这层只有打包文件") {
		t.Errorf("Plan.Reason 要记进 unpack_notes：%v", v.UnpackNotes)
	}
}

func TestAuditScanErrorButUnpackableIsPartial(t *testing.T) {
	a := newTestAuditor()
	a.detect = func(string) string { return "kirikiri" }
	a.scanWith = func(string, adapter.Convention) (*library.Library, error) {
		return nil, errors.New("归档还没解包：没有 .ks")
	}
	a.preview = func(string) (*unpack.Plan, error) {
		return &unpack.Plan{EngineID: "kirikiri", Kind: "archive", Archives: []string{"data.xp3"}, Labels: 12}, nil
	}
	dir := tempGameDir(t, "待解包")
	v := a.audit(rowFor("KiriKiri"), dir)
	if v.Verdict != "partial" || v.Reason != "scan_error_can_unpack" {
		t.Fatalf("可解包应判 partial，实际 %q/%q（errors=%v）", v.Verdict, v.Reason, v.Errors)
	}
	if !v.CanUnpack {
		t.Error("can_unpack 应为 true")
	}
	if !containsSubstr(v.Errors, "归档还没解包") {
		t.Errorf("原始报错丢了：%v", v.Errors)
	}
}

func TestAuditZeroEventsPartialOrFail(t *testing.T) {
	a := newTestAuditor()
	a.detect = func(string) string { return "kirikiri" }
	a.scanWith = func(string, adapter.Convention) (*library.Library, error) {
		return &library.Library{EngineID: "kirikiri"}, nil
	}
	dir := tempGameDir(t, "零事件")

	// 不可解包 → fail/no_events
	v := a.audit(rowFor("KiriKiri"), dir)
	if v.Verdict != "fail" || v.Reason != "no_events" {
		t.Fatalf("零事件且不可解包应 fail/no_events，实际 %q/%q", v.Verdict, v.Reason)
	}

	// 可解包 → partial
	a.preview = func(string) (*unpack.Plan, error) {
		return &unpack.Plan{EngineID: "kirikiri", Kind: "archive", Archives: []string{"a.xp3"}}, nil
	}
	v = a.audit(rowFor("KiriKiri"), dir)
	if v.Verdict != "partial" || v.Reason != "no_events_can_unpack" {
		t.Fatalf("零事件但可解包应 partial，实际 %q/%q", v.Verdict, v.Reason)
	}
}

func TestAuditPanicIsRecoveredIntoErrors(t *testing.T) {
	a := newTestAuditor()
	a.detect = func(string) string { return "kirikiri" }
	a.scanWith = func(string, adapter.Convention) (*library.Library, error) {
		panic("slice bounds out of range [3:2]")
	}
	dir := tempGameDir(t, "会崩的目录")
	v := a.audit(rowFor("KiriKiri"), dir)
	if v.Verdict != "fail" || v.Reason != "scan_panic" {
		t.Fatalf("panic 应判 fail/scan_panic，实际 %q/%q", v.Verdict, v.Reason)
	}
	if !containsSubstr(v.Errors, "slice bounds out of range [3:2]") {
		t.Fatalf("panic 原文必须进 errors，实际：%v", v.Errors)
	}
}

func TestAuditPathMissing(t *testing.T) {
	a := newTestAuditor()
	v := a.audit(rowFor("Unity"), filepath.Join(t.TempDir(), "不存在的目录"))
	if v.Verdict != "fail" || v.Reason != "path_missing" {
		t.Fatalf("路径不存在应 fail/path_missing，实际 %q/%q", v.Verdict, v.Reason)
	}
	if len(v.Errors) == 0 {
		t.Error("path_missing 也要有原始 os.Stat 报错")
	}
}

func TestAuditTopEntriesAndCategoryDirs(t *testing.T) {
	a := newTestAuditor()
	a.detect = func(string) string { return "godot" }
	dir := tempGameDir(t, "顶层")
	for _, name := range []string{"a.exe", "b.pck", "readme.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, d := range []string{"cg", "bg", "stand", "movie", "ui"} {
		if err := os.MkdirAll(filepath.Join(dir, "data", d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	v := a.audit(rowFor("Godot"), dir)
	if v.TopEntries != 4 { // 3 个文件 + data 目录
		t.Errorf("top_entries = %d，想要 4", v.TopEntries)
	}
	if !containsSubstr(v.CategoryDirs, "ui") || len(v.CategoryDirs) != 5 {
		t.Errorf("category_dirs 应识别出 5 类，实际 %v", v.CategoryDirs)
	}
}

// —— 超时分支 ——

func TestAuditTimeout(t *testing.T) {
	a := newTestAuditor()
	a.timeout = 20 * time.Millisecond
	a.detect = func(string) string { return "kirikiri" }
	a.scanWith = func(string, adapter.Convention) (*library.Library, error) {
		time.Sleep(300 * time.Millisecond) // 模拟扫描卡死
		return &library.Library{}, nil
	}
	dir := tempGameDir(t, "慢目录")

	started := time.Now()
	v := auditWithTimeout(a, rowFor("KiriKiri"), dir)
	if took := time.Since(started); took > 250*time.Millisecond {
		t.Errorf("超时没生效：等了 %s", took)
	}
	if v.Verdict != "skip" || v.Reason != "timeout" {
		t.Fatalf("超时应 skip/timeout，实际 %q/%q", v.Verdict, v.Reason)
	}
	if !containsSubstr(v.Errors, "超") {
		t.Errorf("超时原因要写进 errors：%v", v.Errors)
	}
}

// —— summary 计数 ——

func TestSummarizeCounts(t *testing.T) {
	verdicts := []Verdict{
		{Group: "大雷", EngineDetected: "rmmz", Verdict: "pass"},
		{Group: "很牢", EngineDetected: "godot", Verdict: "skip"},
		{Group: "牢", EngineDetected: "unity", Verdict: "fail"},
		{Group: "牢", EngineDetected: "", EngineResolved: "unreal", Verdict: "skip"},
		{Group: "拉", EngineDetected: "", EngineResolved: "", Verdict: "skip"},
	}
	s := summarize(verdicts, "root", "tsv", []string{"大雷", "很牢", "牢", "拉"}, 1500*time.Millisecond)
	if s.Total != 5 || s.Pass != 1 || s.Partial != 0 || s.Fail != 1 || s.Skip != 3 {
		t.Fatalf("总数/各态计数错：%+v", s)
	}
	if s.ByGroup["牢"].Total != 2 || s.ByGroup["牢"].Fail != 1 || s.ByGroup["牢"].Skip != 1 {
		t.Errorf("分组计数错：%+v", s.ByGroup["牢"])
	}
	if s.ByEngine["godot"].Skip != 1 {
		t.Errorf("引擎计数错：%+v", s.ByEngine)
	}
	if s.ByEngine["unreal(guess)"].Skip != 1 {
		t.Errorf("兜底引擎键应带 (guess) 后缀：%+v", s.ByEngine)
	}
	if s.ByEngine["unknown"].Skip != 1 {
		t.Errorf("两边都认不出应记 unknown：%+v", s.ByEngine)
	}
	if s.DurationSec < 1.4 || s.DurationSec > 1.6 {
		t.Errorf("用时换算错：%v", s.DurationSec)
	}
}

// —— 封面候选名判定（防误判成「有封面」） ——

func TestCoverNameMatches(t *testing.T) {
	yes := []string{"cover", "cover_01", "package", "bg_cover", "hyoushi", "表紙", "jacket_2"}
	for _, n := range yes {
		if !coverNameMatches(n) {
			t.Errorf("%q 应当被认成封面候选", n)
		}
	}
	no := []string{"recovery1", "recovery", "discover", "background", "battle_bg", "cg001"}
	for _, n := range no {
		if coverNameMatches(n) {
			t.Errorf("%q 不该被认成封面候选（re-cover-y / discover 这类是误判源）", n)
		}
	}
}

// —— 小工具 ——

func containsSubstr(list []string, sub string) bool {
	for _, s := range list {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
