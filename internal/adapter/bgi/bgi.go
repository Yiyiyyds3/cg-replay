// adapter.go —— BGI / Ethornell 适配器（五方法实现）。
//
// 本机语料是三款 BGI 游戏的解包目录（樱之诗 / 秽翼的尤斯蒂娅 / 美好的每一天），
// 脚本在 <根>/文本_脚本/**/*.bs；.arc 支持 PackFile 与 BURIKO ARC20 两种索引，
// 只列目录、不解 DSC/BSE 数据段。
package bgi

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"ucgr/internal/adapter"
)

// Adapter 是 BGI 引擎适配器。
type Adapter struct{}

func init() {
	adapter.Register(engineID, &Adapter{})
}

// Identify 报告路径是否属于 BGI。
//   - 文件：.bs 且头是 BurikoCompiledScriptVer1.00；.arc 且头是
//     "PackFile    " / "BURIKO ARC20"（不看扩展名之外的证据，不猜）；
//   - 目录：深度 5 内存在带头魔数的 .bs，且没有其它引擎的反向证据（.xp3/.cst/.ast）。
//
// 不确定一律返回 ""（契约纪律：识别歧义由调用方报错，比误认领好）。
func (a *Adapter) Identify(archiveOrDir string) (string, error) {
	st, err := os.Stat(archiveOrDir)
	if err != nil {
		return "", err
	}
	if !st.IsDir() {
		switch {
		case strings.EqualFold(filepath.Ext(archiveOrDir), scriptExt) && fileHasMagic(archiveOrDir):
			return engineID, nil
		case strings.EqualFold(filepath.Ext(archiveOrDir), archiveExt) && arcHasMagic(archiveOrDir):
			return engineID, nil
		}
		return "", nil
	}
	for _, ext := range []string{".xp3", ".cst", ".ast", ".pfs"} {
		if hasFileWithExt(archiveOrDir, ext, dirProbeMaxDepth) {
			return "", nil
		}
	}
	if findScriptWithMagic(archiveOrDir, dirProbeMaxDepth) != "" {
		return engineID, nil
	}
	return "", nil
}

// ParseArchive 列出解包目录或 BGI .arc 归档里的文件。
func (a *Adapter) ParseArchive(path string) ([]adapter.ArchiveEntry, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !st.IsDir() {
		if strings.EqualFold(filepath.Ext(path), archiveExt) {
			return parseARC(path)
		}
		return nil, fmt.Errorf("bgi: ParseArchive 只支持解包目录或 .arc 归档，得到 %s", path)
	}
	var out []adapter.ArchiveEntry
	err = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // 单个文件读不到不中断整体（契约纪律）
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		rel, rerr := filepath.Rel(path, p)
		if rerr != nil {
			rel = p
		}
		out = append(out, adapter.ArchiveEntry{
			Path:        filepath.ToSlash(rel),
			Size:        info.Size(),
			Compression: adapter.CompNone,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// ParseScript 解析 .bs 脚本。
func (a *Adapter) ParseScript(data []byte, ctx *adapter.ScriptCtx) (adapter.ScriptAst, error) {
	entry := ""
	if ctx != nil {
		entry = ctx.EntryPath
	}
	return ParseScriptData(data, entry)
}

// Transitions 报本引擎的转场实现映射。
// v1 只认 L0 回退：BGI 的淡入淡出指令（0x0110 一类的 ui 命令）尚未确认到具体
// opcode → impl 的映射，宁可空表，也不编造（编造会让 V7 校验"过"而实际是假的）。
func (a *Adapter) Transitions() *adapter.TransitionRegistry {
	return adapter.NewTransitionRegistry()
}

// ---- 目录探测小工具 ----

// fileHasMagic 读文件头判断是不是 V1 编译脚本。
func fileHasMagic(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	buf := make([]byte, len(scriptMagic))
	n, _ := f.Read(buf)
	return n >= len(scriptMagic) && bytes.Equal(buf, []byte(scriptMagic))
}

// hasFileWithExt 在 maxDepth 层内是否存在指定扩展名的文件。
func hasFileWithExt(root, ext string, maxDepth int) bool {
	found := ""
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if depthOf(root, p) >= maxDepth {
				return fs.SkipDir
			}
			return nil
		}
		if strings.EqualFold(filepath.Ext(p), ext) {
			found = p
			return errStopWalk
		}
		return nil
	})
	return found != ""
}

// findScriptWithMagic 返回深度内第一个头带魔数的 .bs 路径（"" = 没找到）。
func findScriptWithMagic(root string, maxDepth int) string {
	found := ""
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if depthOf(root, p) >= maxDepth {
				return fs.SkipDir
			}
			return nil
		}
		if strings.EqualFold(filepath.Ext(p), scriptExt) && fileHasMagic(p) {
			found = p
			return errStopWalk
		}
		return nil
	})
	return found
}

func depthOf(root, p string) int {
	rel, err := filepath.Rel(root, p)
	if err != nil || rel == "." {
		return 0
	}
	return len(strings.Split(filepath.ToSlash(rel), "/"))
}

// errStopWalk 提前结束遍历。
var errStopWalk = fmt.Errorf("stop")
