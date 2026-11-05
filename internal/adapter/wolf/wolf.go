// wolf.go —— Wolf RPG Editor 引擎适配器（五方法）。
//
// 识别依据（塔纳托斯实测）：游戏根目录含 Game.exe + data/BasicData/CommonEvent.dat，
// data/ 目录结构与 Wolf RPG Editor 标准布局一致（BasicData/CG/MapData/.mps…）。
// 素材全散装（PNG/OGG/MP3），无需归档解密。
package wolf

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"ucgr/internal/adapter"
	"ucgr/internal/ir"
)

const engineID = "wolf"

// Adapter 是 Wolf RPG Editor 引擎适配器。
type Adapter struct{}

func init() {
	adapter.Register(engineID, &Adapter{})
}

// reverseExts 其它引擎的反向证据：有它们就不认领目录。
var reverseExts = []string{".xp3", ".pfs", ".hvl", ".cst", ".ast", ".int",
	".nsa", ".sar", ".pac", ".npa", ".afa", ".ain"}

// commonEventRel CommonEvent.dat 相对游戏根的路径。
const commonEventRel = "data/BasicData/CommonEvent.dat"

// LooksLikeWolfDir 报告目录是否为 Wolf RPG 游戏（Game.exe + CommonEvent.dat）。
func LooksLikeWolfDir(dir string) bool {
	return isFile(filepath.Join(dir, "Game.exe")) && isFile(filepath.Join(dir, filepath.FromSlash(commonEventRel)))
}

// ParseCommonEventFile 读入并解析 CommonEvent.dat（library / CLI 共用）。
func ParseCommonEventFile(path string) (*commonEvents, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	ce, err := parseCommonEvents(data)
	if err != nil {
		return nil, fmt.Errorf("解析 %s 失败：%w", path, err)
	}
	return ce, nil
}

// Identify 报告路径是否属于 Wolf RPG 引擎。
func (a *Adapter) Identify(archiveOrDir string) (string, error) {
	st, err := os.Stat(archiveOrDir)
	if err != nil {
		return "", err
	}
	if !st.IsDir() {
		return "", nil // Wolf 的 .dat 单文件不足以判定（扩展名太泛）
	}
	for _, ext := range reverseExts {
		if hasFileWithExt(archiveOrDir, ext, 3) {
			return "", nil
		}
	}
	if LooksLikeWolfDir(archiveOrDir) {
		return engineID, nil
	}
	return "", nil
}

// ParseArchive Wolf 无归档容器（素材全散装），返回空条目表。
func (a *Adapter) ParseArchive(path string) ([]adapter.ArchiveEntry, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !st.IsDir() {
		return []adapter.ArchiveEntry{}, nil
	}
	var out []adapter.ArchiveEntry
	_ = filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(p), ".dat") || strings.EqualFold(filepath.Ext(p), ".mps") {
			base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
			rel, e := filepath.Rel(path, p)
			if e != nil {
				return nil
			}
			info, e2 := d.Info()
			if e2 != nil {
				return nil
			}
			out = append(out, adapter.ArchiveEntry{
				Path:        base + "/" + filepath.ToSlash(rel),
				Size:        info.Size(),
				Offset:      0,
				Compression: adapter.CompNone,
			})
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// wolfAst ParseScript 产物：解析好的 CommonEvent 集合。
type wolfAst struct {
	CE *commonEvents
}

// ParseScript 解析 CommonEvent.dat 数据。
func (a *Adapter) ParseScript(data []byte, ctx *adapter.ScriptCtx) (adapter.ScriptAst, error) {
	ce, err := parseCommonEvents(data)
	if err != nil {
		return nil, err
	}
	return &wolfAst{CE: ce}, nil
}

// BuildIR 构建 IR：全部剧情 CE → 事件列表。
func (a *Adapter) BuildIR(astObj adapter.ScriptAst, ctx *adapter.BuildCtx) (*ir.IR, error) {
	sa, ok := astObj.(*wolfAst)
	if !ok {
		return nil, fmt.Errorf("wolf: BuildIR 需要 *wolfAst，实际 %T", astObj)
	}
	out := &ir.IR{
		IRVersion: ir.IRVersion,
		EngineID:  engineID,
		Fidelity:  ir.FidelityGeneric,
		Meta: ir.Meta{
			ScriptEncoding: "UTF-8",
			GeneratedBy:    "ucgr/wolf.BuildIR v1",
		},
	}
	if ctx != nil {
		out.GameID = ctx.GameID
	}
	evs, skipped, err := sa.CE.SceneEvents()
	if err != nil {
		return nil, err
	}
	out.Events = evs
	if len(out.Events) == 0 {
		return nil, fmt.Errorf("wolf: CommonEvent 里没有可回放的剧情事件（跳过 %d 个系统 CE）", skipped)
	}
	return out, nil
}

// Transitions 转场注册表（v1 空：Wolf 的转场由 Prepare/ExecuteTransition
// 指令驱动，v1 未接入）。
func (a *Adapter) Transitions() *adapter.TransitionRegistry {
	return adapter.NewTransitionRegistry()
}

// ---- 工具 ----

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func hasFileWithExt(root, ext string, maxDepth int) bool {
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
	return found != ""
}
