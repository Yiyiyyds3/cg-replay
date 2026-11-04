// nitro.go —— Nitro+ / SDActi 引擎适配器（五方法实现）。
//
// 鬼哭街重制版（2019）的游戏目录是一组 .npa 归档：nss.npa / nss-1.npa（NSS
// 明文脚本，nss-1 是汉化覆盖包）、cg.npa（JPG/PNG 直存，仅前 0x1000+名长
// 加密）、sound.npa（OGG）、dx/（视频，不在包内）。适配器的归档层把整组
// .npa 当成一个虚拟归档（路径前缀 = npa 文件名）。
package nitro

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"ucgr/internal/adapter"
)

const engineID = "nitro"

// dirProbeMaxDepth 目录探测深度。
const dirProbeMaxDepth = 3

// reverseExts 其它引擎的反向证据：有它们就不认领目录。
var reverseExts = []string{".xp3", ".pfs", ".hvl", ".cst", ".ast", ".int",
	".nsa", ".sar", ".pac", ".alf", ".afa", ".arc", ".lpk"}

// Adapter 是 Nitro+ 引擎适配器。
type Adapter struct{}

func init() {
	adapter.Register(engineID, &Adapter{})
}

// Identify 报告路径是否属于 Nitro+。
//   - 文件：.npa 且头部结构合法；
//   - 目录：含合法的 .npa 归档，且没有其它引擎的反向证据。
func (a *Adapter) Identify(archiveOrDir string) (string, error) {
	st, err := os.Stat(archiveOrDir)
	if err != nil {
		return "", err
	}
	if !st.IsDir() {
		if LooksLikeNpaFile(archiveOrDir) {
			return engineID, nil
		}
		return "", nil
	}
	for _, ext := range reverseExts {
		if hasFileWithExt(archiveOrDir, ext, dirProbeMaxDepth) {
			return "", nil
		}
	}
	if findNpaFile(archiveOrDir) != "" {
		return engineID, nil
	}
	return "", nil
}

// ParseArchive 列出目录下全部 .npa 的条目（路径前缀 = npa 基名），或单个 npa。
func (a *Adapter) ParseArchive(path string) ([]adapter.ArchiveEntry, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !st.IsDir() {
		return parseNpaList(path)
	}
	var out []adapter.ArchiveEntry
	_ = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if !LooksLikeNpaFile(p) {
			return nil
		}
		ents, err := parseNpaList(p)
		if err != nil {
			return nil // 单个 npa 坏了不拖垮整体
		}
		out = append(out, ents...)
		return nil
	})
	if len(out) == 0 {
		return nil, fmt.Errorf("nitro: 目录里没有可解析的 .npa 归档: %s", path)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// ParseScript 解析脚本数据（解密解压后的 .nss 明文）。
func (a *Adapter) ParseScript(data []byte, ctx *adapter.ScriptCtx) (adapter.ScriptAst, error) {
	entry := "script.nss"
	if ctx != nil && ctx.EntryPath != "" {
		entry = ctx.EntryPath
	}
	name := entry
	if i := strings.LastIndexByte(name, '/'); i >= 0 {
		name = name[i+1:]
	}
	name = strings.ReplaceAll(name, "\\", "/")
	if i := strings.LastIndexByte(name, '/'); i >= 0 {
		name = name[i+1:]
	}
	return ParseNSS(data, name)
}

// Transitions 报本引擎的转场实现映射。
//
// v1 只认 L0 回退：NSS 的 FadeBG/Zoom 等由引擎命令接管，段流里没有可解析
// 的效果号，宁可空表也不编造。
func (a *Adapter) Transitions() *adapter.TransitionRegistry {
	return adapter.NewTransitionRegistry()
}

// ---- 工具 ----

// parseNpaList 单个 npa 的条目列表。
func parseNpaList(path string) ([]adapter.ArchiveEntry, error) {
	a, err := ParseNpa(path)
	if err != nil {
		return nil, err
	}
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	comp := adapter.CompNone
	if a.Compressed {
		comp = adapter.CompZlib
	}
	out := make([]adapter.ArchiveEntry, 0, len(a.Entries))
	for _, e := range a.Entries {
		if e.IsDir {
			continue
		}
		out = append(out, adapter.ArchiveEntry{
			Path:             base + "/" + e.Name,
			Size:             e.Size,
			Offset:           e.Offset,
			Compression:      comp,
			UncompressedSize: e.Unpacked,
			Encrypted:        a.Encrypted,
		})
	}
	return out, nil
}

// LooksLikeNpaFile 单个文件是不是 NPA（见 npa.go）。

// findNpaFile 返回目录里第一个合法 .npa（深度内）。
func findNpaFile(root string) string {
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
		if LooksLikeNpaFile(p) {
			found = p
		}
		return nil
	})
	return found
}

// hasFileWithExt 深度内是否存在指定扩展名文件。
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
		}
		return nil
	})
	return found
}

// depthOf p 相对 root 的目录深度。
func depthOf(root, p string) int {
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return 0
	}
	n := 0
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == ".." || part == "." {
			continue
		}
		n++
	}
	return n - 1 // 相对路径最后一个分量是文件名/目录名本身
}
