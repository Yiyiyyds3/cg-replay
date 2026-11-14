// Command library-audit 游戏库「只读批量审计」。
//
// 逐款做四件事：
//  1. os.Stat + 顶层 os.ReadDir（记 top_entries）；
//  2. library.DetectEngine 嗅探引擎（与 TSV 的 engine_guess 并列记录，不一致不算错）；
//  3. 按引擎构造 adapter.Convention，调 library.ScanWith 生成事件库（panic 一律 recover 进 errors）；
//  4. 事件为 0 或扫描报错时，再调 unpack.Preview（只读预览，不落盘）记 can_unpack。
//
// 红线：全程只读。不写、不改、不删、不移动、不解包落盘到游戏目录；
// 所有产物只写 -out 指定的目录（默认 reports/audit）。
//
// 判定口径见 docs/14 ④⑤。verdict 四态：
//
//	pass    事件数 > 0 且扫描无 error；
//	partial 扫描报错/零事件，但只读预览判定「可解包」（素材在归档里，值得下一步解包）；
//	fail    有明确错误且不可解包（或路径不存在）；
//	skip    引擎在 main 上没有 ScanXxx 分派（Ren'Py 分支态 / Unreal / Godot / Cocos /
//	        GameMaker / WolfRPG / Electron / 未知 / 私有容器），或单款超时。
//
// 单款超时不看结果：goroutine + select(-timeout)，超时即 skip/reason=timeout，
// 写明「不得死磕」（docs/14 ⑥）。
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// 默认值：仓库根模块下的相对路径 + 游戏库真实根目录。
const (
	defaultTSV  = "reports/游戏库验收_任务清单.tsv"
	defaultRoot = `E:\Personal_entertainment\game\ero\egame\_游戏库\按评价`
	defaultOut  = "reports/audit"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run 可测的入口：返回进程退出码（正常跑完一律 0——审计结论不靠退出码表达，
// docs/14 ⑤-7「审计全程 exit 0，每款有明确 verdict」）。
func run(argv []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("library-audit", flag.ContinueOnError)
	fs.SetOutput(stderr)
	tsvPath := fs.String("tsv", defaultTSV, "任务清单 TSV 路径")
	root := fs.String("root", defaultRoot, "游戏库根目录（按评价）")
	groupFlag := fs.String("group", "", "只跑指定分组（逗号分隔；不给=全跑）")
	outDir := fs.String("out", defaultOut, "产物目录")
	timeout := fs.Duration("timeout", 15*time.Minute, "单款审计时长上限")
	batch := fs.String("batch", "batch-1", "本批 verdict 数组文件名（不带 .json）")
	if err := fs.Parse(argv); err != nil {
		return 2
	}

	rows, err := readTSV(*tsvPath)
	if err != nil {
		fmt.Fprintf(stderr, "读任务清单失败：%v\n", err)
		return 1
	}
	groups := splitGroups(*groupFlag)
	selected := filterRows(rows, groups)
	if len(selected) == 0 {
		fmt.Fprintf(stderr, "没有选中任何条目（TSV 共 %d 行，分组过滤=%v）\n", len(rows), groups)
		return 1
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fmt.Fprintf(stderr, "建产物目录 %s 失败：%v\n", *outDir, err)
		return 1
	}

	a := defaultAuditor(*timeout)
	started := time.Now()
	verdicts := make([]Verdict, 0, len(selected))
	seq := map[string]int{}
	for _, row := range selected {
		abs := filepath.Join(*root, filepath.FromSlash(row.RelPath))
		v := auditWithTimeout(a, row, abs)
		verdicts = append(verdicts, v)

		// stdout 每款一行：verdict | group | game | events | engine_detected | reason
		fmt.Fprintf(stdout, "%s | %s | %s | %d | %s | %s\n",
			v.Verdict, v.Group, v.Game, v.Events, orDefault(v.EngineDetected, "-"), v.Reason)

		seq[v.Group]++
		name := fmt.Sprintf("%s_%02d.json", sanitizeName(v.Group), seq[v.Group])
		if werr := writeJSON(filepath.Join(*outDir, name), v); werr != nil {
			fmt.Fprintf(stderr, "写 %s 失败：%v\n", name, werr)
		}
	}

	batchFile := filepath.Join(*outDir, *batch+".json")
	if werr := writeJSON(batchFile, verdicts); werr != nil {
		fmt.Fprintf(stderr, "写 %s 失败：%v\n", batchFile, werr)
	}
	sum := summarize(verdicts, *root, *tsvPath, groups, time.Since(started))
	sum.BatchFile = filepath.ToSlash(batchFile)
	if werr := writeJSON(filepath.Join(*outDir, "summary.json"), sum); werr != nil {
		fmt.Fprintf(stderr, "写 summary.json 失败：%v\n", werr)
	}

	printSummary(stdout, sum)
	return 0
}

// Bucket 分组/引擎维度的计数。
type Bucket struct {
	Total   int `json:"total"`
	Pass    int `json:"pass"`
	Partial int `json:"partial"`
	Fail    int `json:"fail"`
	Skip    int `json:"skip"`
}

func (b *Bucket) add(v Verdict) {
	b.Total++
	switch v.Verdict {
	case "pass":
		b.Pass++
	case "partial":
		b.Partial++
	case "fail":
		b.Fail++
	case "skip":
		b.Skip++
	}
}

// Summary 本批汇总（-out/summary.json）。
type Summary struct {
	GeneratedAt     string             `json:"generated_at"`
	Root            string             `json:"root"`
	TSV             string             `json:"tsv"`
	GroupsRequested []string           `json:"groups_requested,omitempty"`
	Total           int                `json:"total"`
	Pass            int                `json:"pass"`
	Partial         int                `json:"partial"`
	Fail            int                `json:"fail"`
	Skip            int                `json:"skip"`
	ByGroup         map[string]*Bucket `json:"by_group"`
	ByEngine        map[string]*Bucket `json:"by_engine"`
	DurationSec     float64            `json:"duration_sec"`
	BatchFile       string             `json:"batch_file,omitempty"`
}

// summarize 统计总数 / 各 verdict / 按分组与按引擎的计数。
func summarize(verdicts []Verdict, root, tsv string, groups []string, took time.Duration) Summary {
	s := Summary{
		GeneratedAt:     time.Now().Format(time.RFC3339),
		Root:            root,
		TSV:             tsv,
		GroupsRequested: groups,
		Total:           len(verdicts),
		ByGroup:         map[string]*Bucket{},
		ByEngine:        map[string]*Bucket{},
		DurationSec:     took.Seconds(),
	}
	for _, v := range verdicts {
		switch v.Verdict {
		case "pass":
			s.Pass++
		case "partial":
			s.Partial++
		case "fail":
			s.Fail++
		case "skip":
			s.Skip++
		}
		g := s.ByGroup[v.Group]
		if g == nil {
			g = &Bucket{}
			s.ByGroup[v.Group] = g
		}
		g.add(v)
		k := engineKey(v)
		e := s.ByEngine[k]
		if e == nil {
			e = &Bucket{}
			s.ByEngine[k] = e
		}
		e.add(v)
	}
	return s
}

// engineKey 引擎维度计数用的键：DetectEngine 优先，没有就退回兜底映射出来的引擎。
func engineKey(v Verdict) string {
	if v.EngineDetected != "" {
		return v.EngineDetected
	}
	if v.EngineResolved != "" {
		return v.EngineResolved + "(guess)"
	}
	return "unknown"
}

func printSummary(w io.Writer, s Summary) {
	fmt.Fprintf(w, "[汇总] 总数=%d pass=%d partial=%d fail=%d skip=%d 用时=%.1fs\n",
		s.Total, s.Pass, s.Partial, s.Fail, s.Skip, s.DurationSec)
	for _, k := range sortedKeys(s.ByGroup) {
		b := s.ByGroup[k]
		fmt.Fprintf(w, "  分组 %s: 总数=%d pass=%d partial=%d fail=%d skip=%d\n",
			k, b.Total, b.Pass, b.Partial, b.Fail, b.Skip)
	}
	for _, k := range sortedKeys(s.ByEngine) {
		b := s.ByEngine[k]
		fmt.Fprintf(w, "  引擎 %s: 总数=%d pass=%d partial=%d fail=%d skip=%d\n",
			k, b.Total, b.Pass, b.Partial, b.Fail, b.Skip)
	}
}

func sortedKeys(m map[string]*Bucket) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// writeJSON 写一份缩进 JSON（自动建父目录）。
func writeJSON(path string, v interface{}) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// sanitizeName 把分组名变成安全的文件名片段（游戏库里分组名本身很干净，
// 这里只防意外：路径分隔符、Windows 保留字符、空白）。
func sanitizeName(s string) string {
	s = strings.TrimSpace(s)
	repl := func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|', '\t', '\n', '\r':
			return '_'
		}
		return r
	}
	out := strings.Map(repl, s)
	if out == "" {
		return "未分组"
	}
	return out
}

func orDefault(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}
