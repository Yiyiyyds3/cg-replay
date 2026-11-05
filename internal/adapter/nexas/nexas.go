// nexas.go —— 戏画 NeXAS 引擎适配器（五方法实现）。
//
// Baldr Sky 系列的游戏目录是一组 .pac 归档：Script.pac（.bin 编译脚本）、
// Bmp.pac / Stand.pac / System.pac（PNG/BMP 资源）、Voice.pac（语音）等。
// 适配器的归档层把整组 .pac 当成一个虚拟归档（路径前缀 = pac 文件名）。
package nexas

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"ucgr/internal/adapter"
)

const engineID = "nexas"

// dirProbeMaxDepth 目录探测深度。
const dirProbeMaxDepth = 3

// reverseExts 其它引擎的反向证据：有它们就不认领目录。
var reverseExts = []string{".xp3", ".pfs", ".hvl", ".cst", ".ast", ".int", ".nsa", ".sar"}

// Adapter 是 NeXAS 引擎适配器。
type Adapter struct{}

func init() {
	adapter.Register(engineID, &Adapter{})
}

// Identify 报告路径是否属于 NeXAS。
//   - 文件：.pac 且头部结构合法；
//   - 目录：含合法的 .pac 归档（如 Script.pac），且没有其它引擎的反向证据。
func (a *Adapter) Identify(archiveOrDir string) (string, error) {
	st, err := os.Stat(archiveOrDir)
	if err != nil {
		return "", err
	}
	if !st.IsDir() {
		if LooksLikePacFile(archiveOrDir) {
			return engineID, nil
		}
		return "", nil
	}
	for _, ext := range reverseExts {
		if hasFileWithExt(archiveOrDir, ext, dirProbeMaxDepth) {
			return "", nil
		}
	}
	if findPacFile(archiveOrDir) != "" {
		return engineID, nil
	}
	return "", nil
}

// ParseArchive 列出目录下全部 .pac 的条目（路径前缀 = pac 基名），或单个 pac。
func (a *Adapter) ParseArchive(path string) ([]adapter.ArchiveEntry, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !st.IsDir() {
		return parsePacList(path)
	}
	var out []adapter.ArchiveEntry
	_ = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if !LooksLikePacFile(p) {
			return nil
		}
		ents, err := parsePacList(p)
		if err != nil {
			return nil // 单个 pac 坏了不拖垮整体
		}
		out = append(out, ents...)
		return nil
	})
	if len(out) == 0 {
		return nil, fmt.Errorf("nexas: 目录里没有可解析的 .pac 归档: %s", path)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// ParseScript 解析脚本数据。
//
// data 是解压后的 .bin 内容。优先取 ctx.EntryPath 里的条目名（含 pac 前缀）。
func (a *Adapter) ParseScript(data []byte, ctx *adapter.ScriptCtx) (adapter.ScriptAst, error) {
	entry := "script.bin"
	if ctx != nil && ctx.EntryPath != "" {
		entry = ctx.EntryPath
	}
	name := entry
	if i := strings.LastIndexByte(entry, '/'); i >= 0 {
		name = entry[i+1:]
	}
	return ParseBin(data, name)
}

// Transitions 报本引擎的转场实现映射。
//
// v1 只认 L0 回退：NeXAS 的转场由引擎状态机接管（.spm 粒子/标题资产），
// 段落流里没有可解析的效果号，宁可空表也不编造。
func (a *Adapter) Transitions() *adapter.TransitionRegistry {
	return adapter.NewTransitionRegistry()
}

// ---- 工具 ----

// LooksLikePacFile 单个文件是不是 NeXAS PAC（看头 12 字节 + 尾部索引大小）。
func LooksLikePacFile(path string) bool {
	if !strings.EqualFold(filepath.Ext(path), ".pac") {
		return false
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return false
	}
	head := make([]byte, 12)
	if _, err := f.ReadAt(head, 0); err != nil {
		return false
	}
	return LooksLikePac(head, st.Size())
}

// findPacFile 返回目录里第一个合法 .pac（深度内）。
func findPacFile(root string) string {
	found := ""
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || found != "" {
			return nil
		}
		if d.IsDir() {
			if depthOf(root, p) >= dirProbeMaxDepth {
				return fs.SkipDir
			}
			return nil
		}
		if LooksLikePacFile(p) {
			found = p
			return errStopWalk
		}
		return nil
	})
	return found
}

// hasFileWithExt 深度内是否存在指定后缀文件。
func hasFileWithExt(root, ext string, maxDepth int) bool {
	found := false
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || found {
			return nil
		}
		if d.IsDir() {
			if depthOf(root, p) >= maxDepth {
				return fs.SkipDir
			}
			return nil
		}
		if strings.EqualFold(filepath.Ext(p), ext) {
			found = true
			return errStopWalk
		}
		return nil
	})
	return found
}

// parsePacList 解析单个 .pac 的条目列表。
func parsePacList(path string) ([]adapter.ArchiveEntry, error) {
	a, err := ParsePac(path)
	if err != nil {
		return nil, err
	}
	base := slashBase(path)
	comp := adapter.CompZlib
	if a.Version == PackLzss {
		comp = adapter.CompLzss
	} else if a.Version == PackNone {
		comp = adapter.CompNone
	}
	out := make([]adapter.ArchiveEntry, 0, len(a.Entries))
	for _, e := range a.Entries {
		out = append(out, adapter.ArchiveEntry{
			Path:             base + "/" + e.Name,
			Size:             e.Size,
			Offset:           e.Offset,
			Compression:      comp,
			UncompressedSize: e.Unpacked,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func depthOf(root, p string) int {
	rel, err := filepath.Rel(root, p)
	if err != nil || rel == "." {
		return 0
	}
	return len(strings.Split(filepath.ToSlash(rel), "/"))
}

func slashBase(p string) string {
	return strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
}

var errStopWalk = fmt.Errorf("stop")
