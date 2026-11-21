// textdump_test.go —— textdump 子命令的端到端小样。
//
// 两个迷你样本各造一份：kirikiri 的 .ks（标签/宏/注释/标签夹在文本行里）与
// rmmz 的 Map001.json（101 + 401 续行 + 102 选项）。断言导出文件的行数与
// _textdump.json 的关键字段——验收脚本读的就是这些字段，字段一改这里先红。
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// tdNL 换行符（避免在测试源码里写转义序列，读起来跟文件内容一致）。
var tdNL = string(rune(10))

// tdCmd 造一条 rmmz 事件指令（code + 位置参数）。
func tdCmd(code int, params ...any) map[string]any {
	return map[string]any{"code": code, "indent": 0, "parameters": params}
}

// tdWrite 写测试文件（父目录不存在时先建）。
func tdWrite(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("建目录 %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("写 %s: %v", path, err)
	}
}

// tdWriteJSON 把任意结构序列化成 JSON 落盘（测试数据不用手写 JSON 字面量）。
func tdWriteJSON(t *testing.T, path string, v any) {
	t.Helper()
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("序列化 %s: %v", path, err)
	}
	tdWrite(t, path, b)
}

// tdIndex 读导出目录里的 _textdump.json 并解析成通用 map。
func tdIndex(t *testing.T, out string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(out, "_textdump.json"))
	if err != nil {
		t.Fatalf("读 _textdump.json: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("解析 _textdump.json: %v", err)
	}
	return m
}

// tdNum 取数字字段。
func tdNum(t *testing.T, m map[string]any, key string) int {
	t.Helper()
	v, ok := m[key].(float64)
	if !ok {
		t.Fatalf("字段 %s 不是数字：%v", key, m[key])
	}
	return int(v)
}

// tdStr 取字符串字段。
func tdStr(t *testing.T, m map[string]any, key string) string {
	t.Helper()
	v, ok := m[key].(string)
	if !ok {
		t.Fatalf("字段 %s 不是字符串：%v", key, m[key])
	}
	return v
}

// tdTextLines 读导出文本并按行拆（丢掉空行）。
func tdTextLines(t *testing.T, path string) []string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读 %s: %v", path, err)
	}
	var out []string
	for _, l := range strings.Split(string(raw), tdNL) {
		if s := strings.TrimSpace(l); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// TestCmdTextDumpKirikiri 迷你 .ks：只导出显示字符串行，注释/标签/@宏 都不进文本。
func TestCmdTextDumpKirikiri(t *testing.T) {
	root := t.TempDir()
	scen := filepath.Join(root, "raw", "data", "scenario")
	ks := `*start
@bg storage="bg01"
; 这一行是注释，不该进导出
First line
[wait time=300]
Second line
`
	tdWrite(t, filepath.Join(scen, "a01.ks"), []byte(ks))

	out := filepath.Join(t.TempDir(), "dump")
	if err := cmdTextDump([]string{root, "--out", out}); err != nil {
		t.Fatalf("cmdTextDump: %v", err)
	}

	txtPath := filepath.Join(out, "raw", "data", "scenario", "a01.txt")
	lines := tdTextLines(t, txtPath)
	if len(lines) != 2 {
		t.Fatalf("台词行数 = %d（want 2）：%q", len(lines), lines)
	}
	if lines[0] != "First line" || lines[1] != "Second line" {
		t.Fatalf("台词内容 = %q（want [First line, Second line]）", lines)
	}
	if raw, rerr := os.ReadFile(txtPath); rerr == nil && len(raw) > 0 && raw[0] == 0xEF {
		t.Fatal("导出文件带了 UTF-8 BOM")
	}

	idx := tdIndex(t, out)
	if got := tdNum(t, idx, "schema"); got != 1 {
		t.Fatalf("schema = %d（want 1）", got)
	}
	if got := tdStr(t, idx, "engine_id"); got != "kirikiri" {
		t.Fatalf("engine_id = %q（want kirikiri）", got)
	}
	if got := tdStr(t, idx, "game"); got != filepath.Base(root) {
		t.Fatalf("game = %q（want %q）", got, filepath.Base(root))
	}
	if _, err := time.Parse(time.RFC3339, tdStr(t, idx, "generated_at")); err != nil {
		t.Fatalf("generated_at 不是 RFC3339：%v", err)
	}
	if got := tdNum(t, idx, "total_scripts"); got != 1 {
		t.Fatalf("total_scripts = %d（want 1）", got)
	}
	if got := tdNum(t, idx, "total_lines"); got != 2 {
		t.Fatalf("total_lines = %d（want 2）", got)
	}
	// chars = len("First line") + len("Second line") = 10 + 11
	if got := tdNum(t, idx, "total_chars"); got != 21 {
		t.Fatalf("total_chars = %d（want 21）", got)
	}
	scripts, ok := idx["scripts"].([]any)
	if !ok || len(scripts) != 1 {
		t.Fatalf("scripts = %v（want 1 条）", idx["scripts"])
	}
	first, _ := scripts[0].(map[string]any)
	if got := tdStr(t, first, "path"); got != "raw/data/scenario/a01.txt" {
		t.Fatalf("scripts[0].path = %q", got)
	}
	if got := tdNum(t, first, "lines"); got != 2 {
		t.Fatalf("scripts[0].lines = %d（want 2）", got)
	}
	if got := tdNum(t, first, "chars"); got != 21 {
		t.Fatalf("scripts[0].chars = %d（want 21）", got)
	}
}

// TestCmdTextDumpRMMZ 迷你 rmmz：401 续行拼成一句、102 每个选项一行、
// 公共事件单独导一份。
func TestCmdTextDumpRMMZ(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "www", "data")
	tdWriteJSON(t, filepath.Join(dataDir, "System.json"), map[string]any{"gameTitle": "迷你"})
	tdWriteJSON(t, filepath.Join(dataDir, "Map001.json"), map[string]any{
		"displayName": "测试地图",
		"events": []any{
			map[string]any{
				"id":   1,
				"name": "CG 事件",
				"pages": []any{
					map[string]any{"list": []any{
						tdCmd(101, "", 0, 0, 2, "艾莉丝"),
						tdCmd(401, "第一句"),
						tdCmd(401, "接在后面"),
						tdCmd(102, []any{"选项一", "选项二"}),
					}},
				},
			},
		},
	})
	tdWriteJSON(t, filepath.Join(dataDir, "CommonEvents.json"), []any{
		map[string]any{"id": 1, "name": "公共事件", "list": []any{
			tdCmd(401, "公共台词"),
		}},
	})

	out := filepath.Join(t.TempDir(), "dump")
	if err := cmdTextDump([]string{root, "--out", out}); err != nil {
		t.Fatalf("cmdTextDump: %v", err)
	}

	mapLines := tdTextLines(t, filepath.Join(out, "data_Map001.txt"))
	want := []string{"第一句接在后面", "选项一", "选项二"}
	if len(mapLines) != len(want) {
		t.Fatalf("Map001 行数 = %d（want %d）：%q", len(mapLines), len(want), mapLines)
	}
	for i := range want {
		if mapLines[i] != want[i] {
			t.Fatalf("Map001 第 %d 行 = %q（want %q）", i+1, mapLines[i], want[i])
		}
	}
	ceLines := tdTextLines(t, filepath.Join(out, "data_CommonEvents.txt"))
	if len(ceLines) != 1 || ceLines[0] != "公共台词" {
		t.Fatalf("CommonEvents 台词 = %q（want [公共台词]）", ceLines)
	}

	idx := tdIndex(t, out)
	if got := tdStr(t, idx, "engine_id"); got != "rmmz" {
		t.Fatalf("engine_id = %q（want rmmz）", got)
	}
	if got := tdNum(t, idx, "total_scripts"); got != 2 {
		t.Fatalf("total_scripts = %d（want 2）", got)
	}
	if got := tdNum(t, idx, "total_lines"); got != 4 {
		t.Fatalf("total_lines = %d（want 4）", got)
	}
	scripts, _ := idx["scripts"].([]any)
	if len(scripts) != 2 {
		t.Fatalf("scripts 条数 = %d（want 2）", len(scripts))
	}
	var paths []string
	for _, s := range scripts {
		m, _ := s.(map[string]any)
		paths = append(paths, tdStr(t, m, "path"))
	}
	if paths[0] != "data_CommonEvents.txt" || paths[1] != "data_Map001.txt" {
		t.Fatalf("scripts 路径 = %v", paths)
	}
}

// TestCmdTextDumpEmptyDir 空目录：友好报「未发现剧本」且不算失败。
func TestCmdTextDumpEmptyDir(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(t.TempDir(), "dump")
	if err := cmdTextDump([]string{root, "--out", out}); err != nil {
		t.Fatalf("空目录应当友好退出（err=nil），实际：%v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "_textdump.json")); !os.IsNotExist(err) {
		t.Fatalf("空目录不该产出索引，实际：%v", err)
	}
}

// TestKsPlainLines 轻量解析：注释/标签行/@宏 跳过，行内标签剥掉只留文本。
func TestKsPlainLines(t *testing.T) {
	src := strings.Join([]string{
		"*label",
		"@wait 100",
		"; 注释",
		"[bg storage=x]台词一",
		"文本二",
		"",
	}, tdNL)
	got := ksPlainLines(src)
	want := []string{"台词一", "文本二"}
	if len(got) != len(want) {
		t.Fatalf("行数 = %d（want %d）：%q", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("第 %d 行 = %q（want %q）", i+1, got[i], want[i])
		}
	}
}

// TestCmdTextDumpArtemis 迷你 Artemis 明文剧本：> 命令行跳过，其余非空行是台词。
func TestCmdTextDumpArtemis(t *testing.T) {
	root := t.TempDir()
	scn := filepath.Join(root, "scenario")
	src := strings.Join([]string{
		">bg slot=bg01",
		"; 注释行",
		"第一行台词",
		">se 1",
		"第二行台词",
	}, tdNL)
	tdWrite(t, filepath.Join(scn, "a01.s"), []byte(src))

	out := filepath.Join(t.TempDir(), "dump")
	// .s 不是 DetectEngine 认得的剧本签名，显式指定引擎（CLI 同样支持 --engine）。
	if err := cmdTextDump([]string{root, "--out", out, "--engine", "artemis"}); err != nil {
		t.Fatalf("cmdTextDump: %v", err)
	}
	lines := tdTextLines(t, filepath.Join(out, "scenario", "a01.txt"))
	if len(lines) != 2 || lines[0] != "第一行台词" || lines[1] != "第二行台词" {
		t.Fatalf("Artemis 台词 = %q", lines)
	}
	idx := tdIndex(t, out)
	if got := tdStr(t, idx, "engine_id"); got != "artemis" {
		t.Fatalf("engine_id = %q（want artemis）", got)
	}
	if got := tdNum(t, idx, "total_lines"); got != 2 {
		t.Fatalf("total_lines = %d（want 2）", got)
	}
}
