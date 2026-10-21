// Package unpack 把 galgame 归档解成「只够重演」的最小集合。
//
// 目标（用户提出的方向）：拖一个 galgame 进来 → 自动识别引擎 → 解出
// **只够重演 CG 事件**的最小集合 → 写成现有加载器能直接吃的目录 → 登记入库。
//
// 阶段 1 覆盖 KiriKiri（XP3）：
//   - 未加密的 XP3：完整支持（多段拼接 + zlib 解压 + 依赖闭包 + 落盘）。
//   - 加密条目（XP3 adlr / BSE 加密图）：**不解密**，如实报告跳过（要每作密钥）。
//   - 已解包目录：无需解包，直接登记。
//
// 落盘布局刻意与现有加载器一致（raw/data/scenario + raw/data/image），
// 所以解包产物能直接被 library.Scan + replay.GameLoader 消费，不需要改加载侧。
package unpack

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Scope 最小集合的取材范围。
type Scope string

const (
	// ScopeCG 只要重演 CG 事件所需（默认）：画廊脚本 + 它们引用的图。
	ScopeCG Scope = "cg"
	// ScopeAll 全部事件所需：所有场景/段落脚本引用到的素材。
	ScopeAll Scope = "all"
)

// Plan 解包预览（拖进来先看「会解出什么」，不落盘）。
type Plan struct {
	EngineID      string   `json:"engine_id"`
	Root          string   `json:"root"`
	Kind          string   `json:"kind"`           // loose=已是解包目录 / xp3=XP3 归档
	Archives      []string `json:"archives"`       // 发现的归档
	ScenarioFiles int      `json:"scenario_files"` // 剧本文件数
	Labels        int      `json:"labels"`         // 事件（标签）数
	Refs          int      `json:"refs"`           // 引用到的素材数
	Encrypted     int      `json:"encrypted"`      // 加密条目数（不解，要密钥）
	Notes         []string `json:"notes"`
	Unsupported   bool     `json:"unsupported"` // 引擎暂不支持自动解包
	Reason        string   `json:"reason"`      // 不支持的原因（对用户明说）
}

// Result 解包结果。
type Result struct {
	Dest      string   `json:"dest"`
	EngineID  string   `json:"engine_id"`
	Events    int      `json:"events"`
	Files     int      `json:"files"`
	Bytes     int64    `json:"bytes"`
	Missing   []string `json:"missing"`   // 引用了但归档里找不到的素材
	Encrypted []string `json:"encrypted"` // 加密条目（跳过）
	Warnings  []string `json:"warnings"`
}

// Preview 预览一个游戏目录/归档，不落盘（解包前先给用户看「会解出什么」）。
func Preview(root string) (*Plan, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, fmt.Errorf("没有选择路径")
	}
	st, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("读不到 %s：%w", root, err)
	}
	if !st.IsDir() {
		// 单个归档文件。
		if strings.EqualFold(filepath.Ext(root), ".xp3") {
			return kirikiriPlan([]string{root})
		}
		return &Plan{
			Root:        root,
			Kind:        "file",
			Unsupported: true,
			Reason:      fmt.Sprintf("暂不支持直接解 %s 这样的单文件（阶段 1 只做 XP3 与已解包目录）", filepath.Ext(root)),
		}, nil
	}
	// 已是解包目录？
	if st, err := os.Stat(filepath.Join(root, "raw", "data", "scenario")); err == nil && st.IsDir() {
		return &Plan{
			Root:     root,
			Kind:     "loose",
			EngineID: "kirikiri",
			Notes:    []string{"这已经是解包目录，直接登记即可（无需解包）"},
		}, nil
	}
	archives, err := findXP3(root)
	if err != nil {
		return nil, err
	}
	if len(archives) == 0 {
		return &Plan{
			Root:        root,
			Kind:        "unknown",
			Unsupported: true,
			Reason:      "这个目录里没找到 .xp3 归档，也没找到已解包结构（raw/data/scenario）",
		}, nil
	}
	return kirikiriPlan(archives)
}

// Extract 解包到 dest（dest 会自动创建），写出最小集合 + 约定文件。
func Extract(root, dest string, scope Scope) (*Result, error) {
	root = strings.TrimSpace(root)
	dest = strings.TrimSpace(dest)
	if root == "" || dest == "" {
		return nil, fmt.Errorf("解包需要源路径与目标目录")
	}
	if scope == "" {
		scope = ScopeCG
	}
	st, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("读不到 %s：%w", root, err)
	}
	if st.IsDir() {
		if s, err := os.Stat(filepath.Join(root, "raw", "data", "scenario")); err == nil && s.IsDir() {
			return &Result{
				Dest:     root,
				EngineID: "kirikiri",
				Warnings: []string{"源目录已是解包结构，未做复制（直接登记即可）"},
			}, nil
		}
		archives, err := findXP3(root)
		if err != nil {
			return nil, err
		}
		if len(archives) == 0 {
			return nil, fmt.Errorf("没找到 .xp3 归档")
		}
		return kirikiriExtract(archives, dest, scope)
	}
	if strings.EqualFold(filepath.Ext(root), ".xp3") {
		return kirikiriExtract([]string{root}, dest, scope)
	}
	return nil, fmt.Errorf("暂不支持解 %s（阶段 1 只做 XP3 与已解包目录）", filepath.Ext(root))
}

// findXP3 在目录里找 XP3 归档（顶层 + 两层子目录，避免扫全盘）。
func findXP3(root string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // 单个目录读不了就跳过
		}
		if d.IsDir() {
			rel, _ := filepath.Rel(root, path)
			if rel != "." && strings.Count(filepath.ToSlash(rel), "/") >= 2 {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".xp3") {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("扫描归档失败：%w", err)
	}
	sort.Strings(out)
	return out, nil
}
