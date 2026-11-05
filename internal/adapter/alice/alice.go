// alice.go —— AliceSoft System 4 引擎适配器（五方法实现）。
//
// 兰斯10（2018，中文版实测）：游戏根目录 = Rance10.ain（AI2 zlib 包裹的
// 区段流脚本，汉化文本 GBK）+ Rance10CG*.afa（图像，AJP/QNT/DCF）
// + Rance10Sound.afa（OGG）+ Rance10Flat*.afa（活动部件数据）。
// 演出提取走「场景函数」（函数名含 CJK）→ 见 events.go。
package alice

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"ucgr/internal/adapter"
	"ucgr/internal/ir"
)

const engineID = "alice"

// Adapter 是 System 4 引擎适配器。
type Adapter struct{}

func init() {
	adapter.Register(engineID, &Adapter{})
}

// reverseExts 其它引擎的反向证据：有它们就不认领目录。
var reverseExts = []string{".xp3", ".pfs", ".hvl", ".cst", ".ast", ".int",
	".nsa", ".sar", ".pac", ".npa", ".alf"}

// Identify 报告路径是否属于 System 4：
//   - 文件：.ain 且 AI2 头合法，或 .afa 且 AFAH 头合法；
//   - 目录：含合法 .ain / .afa，且没有其它引擎的反向证据。
func (a *Adapter) Identify(archiveOrDir string) (string, error) {
	st, err := os.Stat(archiveOrDir)
	if err != nil {
		return "", err
	}
	if !st.IsDir() {
		ext := strings.ToLower(filepath.Ext(archiveOrDir))
		switch ext {
		case ".ain":
			if LooksLikeAINFile(archiveOrDir) {
				return engineID, nil
			}
		case ".afa":
			if LooksLikeAFAFile(archiveOrDir) {
				return engineID, nil
			}
		}
		return "", nil
	}
	for _, ext := range reverseExts {
		if hasFileWithExt(archiveOrDir, ext, 3) {
			return "", nil
		}
	}
	if findSuffix(archiveOrDir, ".ain", 3) != "" {
		return engineID, nil
	}
	return "", nil
}

// ParseArchive 列出目录下全部 .afa 的条目（路径前缀 = afa 基名）或单个 afa。
func (a *Adapter) ParseArchive(path string) ([]adapter.ArchiveEntry, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	var out []adapter.ArchiveEntry
	add := func(afaPath string) {
		af, err := ParseAFA(afaPath)
		if err != nil {
			return // 单个 afa 坏了不拖垮整体
		}
		base := strings.TrimSuffix(filepath.Base(afaPath), filepath.Ext(afaPath))
		for _, e := range af.Entries {
			out = append(out, adapter.ArchiveEntry{
				Path:        base + "/" + e.Name,
				Size:        int64(e.Size),
				// AFA 的 off 相对数据区（dataStart）；ArchiveEntry.Offset
				// 的契约是「归档文件内绝对偏移」（ReadArchiveEntry 直接
				// Seek 它），必须加上 dataStart。
				Offset:      af.DataStart + e.Off,
				Compression: adapter.CompNone,
				Encrypted:   false,
			})
		}
	}
	if !st.IsDir() {
		add(path)
		if len(out) == 0 {
			return nil, fmt.Errorf("alice: 无法解析归档 %s", path)
		}
		return out, nil
	}
	_ = filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(p), ".afa") {
			add(p)
		}
		return nil
	})
	if len(out) == 0 {
		return nil, fmt.Errorf("alice: 目录里没有可解析的 .afa 归档: %s", path)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// ScriptAst 产出（BuildIR 消费）。
type ScriptAst struct {
	AIN     *AIN
	FuncIdx int // 场景函数下标（-1 = 全部场景函数）
}

// ParseScript 解析 .ain 数据。
func (a *Adapter) ParseScript(data []byte, ctx *adapter.ScriptCtx) (adapter.ScriptAst, error) {
	ain, err := ParseAIN(data)
	if err != nil {
		return nil, err
	}
	idx := -1
	if ctx != nil && ctx.EntryPath != "" {
		// EntryPath 约定为「函数名」（场景函数命名即事件名）
		if i := ain.FindFunc(ctx.EntryPath); i >= 0 {
			idx = i
		}
	}
	return &ScriptAst{AIN: ain, FuncIdx: idx}, nil
}

// BuildIR 构造 IR：FuncIdx >= 0 → 单函数事件；-1 → 全部场景函数。
func (a *Adapter) BuildIR(astObj adapter.ScriptAst, ctx *adapter.BuildCtx) (*ir.IR, error) {
	sa, ok := astObj.(*ScriptAst)
	if !ok {
		return nil, fmt.Errorf("alice: BuildIR 需要 *ScriptAst，实际 %T", astObj)
	}
	out := &ir.IR{
		IRVersion: "1.0",
		EngineID:  engineID,
		Fidelity:  "generic",
		Meta: ir.Meta{
			ScriptEncoding: "GBK",
			GeneratedBy:    "ucgr/alice.BuildIR v1",
		},
	}
	if ctx != nil {
		out.GameID = ctx.GameID
	}
	if sa.FuncIdx >= 0 {
		ev, err := sa.AIN.SceneEventOf(sa.FuncIdx)
		if err != nil {
			return nil, err
		}
		out.Events = append(out.Events, *ev)
	} else {
		evs, skipped := sa.AIN.BuildSceneEvents()
		out.Events = evs
		if len(out.Events) == 0 {
			return nil, fmt.Errorf("alice: ain 里没有可回放的场景函数（跳过 %d 个坏函数）", skipped)
		}
	}
	return out, nil
}

// Transitions 转场注册表（v1 空：System 4 的转场由引擎状态机接管）。
func (a *Adapter) Transitions() *adapter.TransitionRegistry {
	return adapter.NewTransitionRegistry()
}

// ---- 工具 ----

// findSuffix 深度内找第一个指定后缀文件。
func findSuffix(root, ext string, maxDepth int) string {
	found := ""
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || found != "" {
			return nil
		}
		if d.IsDir() {
			if rel, e := filepath.Rel(root, p); e == nil && rel != "." &&
				strings.Count(rel, string(filepath.Separator)) >= maxDepth {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.EqualFold(filepath.Ext(p), ext) {
			found = p
		}
		return nil
	})
	return found
}

// hasFileWithExt 深度内是否存在指定后缀文件。
func hasFileWithExt(root, ext string, maxDepth int) bool {
	return findSuffix(root, ext, maxDepth) != ""
}
