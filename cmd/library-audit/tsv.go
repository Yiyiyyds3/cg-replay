package main

import (
	"fmt"
	"os"
	"strings"
)

// TSVRow 任务清单的一行。
//
// 清单格式（见 reports/游戏库验收_任务清单.tsv，5 列，无表头）：
//
//	分组 \t 游戏名 \t 相对路径 \t engine_guess \t 难度
//
// 注意两条来自真实语料的事实：
//   - 游戏名里可能有单引号（`Goddesses' Whim`）、括号、中文、日文，
//     一律**不做任何引号/转义解析**，按制表符切完就原样保留；
//   - 路径用 '/' 分隔且带中文目录名，落地时用 filepath.FromSlash 转换。
type TSVRow struct {
	Group       string // 评价分组（大雷/很牢/牢/拉/一搬/一般/未评级/中等/中上/不赖/夯/创意/上等）
	Game        string // 游戏名（= 相对路径的最后一段）
	RelPath     string // 相对 -root 的路径（'/' 分隔）
	EngineGuess string // 清单里的引擎推断（与 DetectEngine 结果并列记录，不一致不算错）
	Difficulty  string // easy / normal / hard（清单口径，仅作参考）
	LineNo      int    // TSV 行号（1 起，便于回查）
}

// readTSV 读并解析任务清单。
func readTSV(path string) ([]TSVRow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	rows, err := parseTSV(string(data))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return rows, nil
}

// parseTSV 解析清单正文。空行、注释行（# 开头）、表头（首列「分组」）跳过。
func parseTSV(text string) ([]TSVRow, error) {
	text = strings.TrimPrefix(text, "\ufeff") // 编辑器可能写 BOM
	lines := strings.Split(text, "\n")
	rows := make([]TSVRow, 0, len(lines))
	for i, raw := range lines {
		line := strings.TrimRight(raw, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		cols := strings.Split(line, "\t")
		if len(cols) < 3 {
			return nil, fmt.Errorf("第 %d 行只有 %d 列（至少要 分组/游戏/相对路径 3 列）：%q", i+1, len(cols), line)
		}
		group := strings.TrimSpace(cols[0])
		if group == "分组" { // 表头
			continue
		}
		row := TSVRow{
			Group:   group,
			Game:    strings.TrimSpace(cols[1]),
			RelPath: strings.TrimSpace(cols[2]),
			LineNo:  i + 1,
		}
		if len(cols) > 3 {
			row.EngineGuess = strings.TrimSpace(cols[3])
		}
		if len(cols) > 4 {
			row.Difficulty = strings.TrimSpace(cols[4])
		}
		if row.RelPath == "" {
			return nil, fmt.Errorf("第 %d 行相对路径为空：%q", i+1, line)
		}
		if row.Game == "" {
			row.Game = lastSegment(row.RelPath)
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// splitGroups 把 -group 参数拆成分组集合（逗号分隔，去空白）。
func splitGroups(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	out := []string{}
	seen := map[string]bool{}
	for _, part := range strings.Split(s, ",") {
		g := strings.TrimSpace(part)
		if g == "" || seen[g] {
			continue
		}
		seen[g] = true
		out = append(out, g)
	}
	return out
}

// filterRows 按分组过滤；groups 为空表示全跑。
func filterRows(rows []TSVRow, groups []string) []TSVRow {
	if len(groups) == 0 {
		return rows
	}
	want := map[string]bool{}
	for _, g := range groups {
		want[g] = true
	}
	out := make([]TSVRow, 0, len(rows))
	for _, r := range rows {
		if want[r.Group] {
			out = append(out, r)
		}
	}
	return out
}

// lastSegment 取相对路径最后一段（游戏名兜底）。
func lastSegment(rel string) string {
	rel = strings.TrimRight(strings.ReplaceAll(rel, "\\", "/"), "/")
	if i := strings.LastIndex(rel, "/"); i >= 0 {
		return rel[i+1:]
	}
	return rel
}
