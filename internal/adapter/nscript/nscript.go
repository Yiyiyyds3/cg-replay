// Package nscript —— NScripter 引擎适配器（五方法实现）。
//
// NScripter 的剧本是整作一份：nscript.dat（每字节 XOR 0x84 的 CP932 明文）
// 或 0.txt / 00.txt。图片散装在 bmp/ 等目录，或打进 arc.nsa / arcN.nsa
// （大端索引：名字\0 + u8 压缩类型 + u32 偏移/长度/原始长；
// 0=无压缩、1=SPB、2=LZSS、3=NBZ）。
//
// 本机语料：寒蝉鸣泣之时·解题篇、海猫鸣泣之时 出题篇/散（07th Expansion，
// 萌化版）。
package nscript

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

const engineID = "nscript"

// dirProbeMaxDepth 目录探测深度。
const dirProbeMaxDepth = 4

// reverseExts 其它引擎的反向证据：有它们就不认领。
var reverseExts = []string{".xp3", ".pfs", ".hvl", ".cst", ".ast", ".int"}

// Adapter 是 NScripter 引擎适配器。
type Adapter struct{}

func init() {
	adapter.Register(engineID, &Adapter{})
}

// Identify 报告路径是否属于 NScripter。
//   - 文件：nscript.dat 且 XOR 0x84 后首行是 `;` 注释；0.txt / 00.txt 明文
//     剧本（首行 `;value` 开头才算，避免误认领普通同名文件）；
//   - 目录：深度 4 内存在上述剧本，且没有其它引擎的反向证据。
func (a *Adapter) Identify(archiveOrDir string) (string, error) {
	st, err := os.Stat(archiveOrDir)
	if err != nil {
		return "", err
	}
	if !st.IsDir() {
		if isNScripterScriptFile(archiveOrDir) {
			return engineID, nil
		}
		return "", nil
	}
	for _, ext := range reverseExts {
		if hasFileWithExt(archiveOrDir, ext, dirProbeMaxDepth) {
			return "", nil
		}
	}
	if findScriptFile(archiveOrDir, dirProbeMaxDepth) != "" {
		return engineID, nil
	}
	return "", nil
}

// ParseArchive 列出解包目录或 NSA/SAR 归档里的文件。
func (a *Adapter) ParseArchive(path string) ([]adapter.ArchiveEntry, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !st.IsDir() {
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".nsa" || ext == ".sar" {
			return parseNsaList(path)
		}
		return nil, fmt.Errorf("nscript: ParseArchive 只支持解包目录或 .nsa/.sar 归档，得到 %s", path)
	}
	var out []adapter.ArchiveEntry
	err = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
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

// ParseScript 解析脚本数据。
//
// data 可以是原始密文（nscript.dat，XOR 0x84）或已解码的 UTF-8 文本。
// 嗅探规则：XOR 采样里出现 NScripter 明文关键字（;value / *define /
// numalias…）才算密文——有的剧本（寒蝉解）解出来以空行开头，
// 首字节判断会误判成「已解码」而二次 XOR。
func (a *Adapter) ParseScript(data []byte, ctx *adapter.ScriptCtx) (adapter.ScriptAst, error) {
	entry := "nscript.dat"
	if ctx != nil && ctx.EntryPath != "" {
		entry = ctx.EntryPath
	}
	text := string(scriptPlainText(data))
	return ParseScript(text, entry)
}

// scriptPlainText 把密文或明文统一成 UTF-8 明文。
func scriptPlainText(data []byte) []byte {
	if len(data) == 0 {
		return data
	}
	// 先试 XOR：采样出现明文关键字 → 密文。
	sampleLen := len(data)
	if sampleLen > 8192 {
		sampleLen = 8192
	}
	sample := make([]byte, sampleLen)
	for i := 0; i < sampleLen; i++ {
		sample[i] = data[i] ^ scriptKey
	}
	for _, kw := range []string{";value", "*define", "numalias", "stralias"} {
		if bytes.Contains(sample, []byte(kw)) {
			out := make([]byte, len(data))
			for i := range data {
				out[i] = data[i] ^ scriptKey
			}
			return []byte(decodeMB(out, 932))
		}
	}
	// 原样含关键字 → 已是明文。
	for _, kw := range []string{"*define", "numalias", "stralias", ";value"} {
		if bytes.Contains(data[:sampleLen], []byte(kw)) {
			return data
		}
	}
	// 都不含：按密文兜底（老脚本 define 块可能没有这些关键字）。
	return []byte(decodeMB(data, 932))
}

// Transitions 报本引擎的转场实现映射。
//
// v1 只认 L0 回退：NScripter 的 print 效果号（bg/ld 第三参）→ impl 的映射
// 未确认到具体值，宁可空表也不编造。
func (a *Adapter) Transitions() *adapter.TransitionRegistry {
	return adapter.NewTransitionRegistry()
}

// ---- 目录探测小工具 ----

// isNScripterScriptFile 判断单个文件是不是 NScripter 剧本。
func isNScripterScriptFile(path string) bool {
	base := slashBase(path)
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return false
	}
	if strings.EqualFold(base, "nscript.dat") {
		return LooksLikeNScripterDat(data)
	}
	lower := strings.ToLower(base)
	if lower == "0.txt" || lower == "00.txt" {
		// 明文剧本：首行应是 ; 注释。
		return data[0] == ';'
	}
	return false
}

// findScriptFile 返回深度内第一个 NScripter 剧本路径（"" = 没找到）。
func findScriptFile(root string, maxDepth int) string {
	found := ""
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || found != "" {
			return nil
		}
		if d.IsDir() {
			if depthOf(root, p) >= maxDepth {
				return fs.SkipDir
			}
			return nil
		}
		if isNScripterScriptFile(p) {
			found = p
			return errStopWalk
		}
		return nil
	})
	return found
}

// hasFileWithExt 在 maxDepth 层内是否存在指定扩展名的文件。
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

// parseNsaList 解析 NSA/SAR 索引并列条目。
func parseNsaList(path string) ([]adapter.ArchiveEntry, error) {
	a, err := ParseNsa(path)
	if err != nil {
		return nil, err
	}
	out := make([]adapter.ArchiveEntry, 0, len(a.Entries))
	for _, k := range a.order {
		e := a.Entries[k]
		comp := adapter.CompNone
		if e.Comp == compSPB || e.Comp == compLZSS || e.Comp == compNBZ {
			comp = adapter.CompLzss
		}
		out = append(out, adapter.ArchiveEntry{
			Path:             strings.ReplaceAll(e.Name, "\\", "/"),
			Size:             int64(e.Length),
			Offset:           int64(e.Offset),
			Compression:      comp,
			UncompressedSize: int64(e.OrigLen),
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

var errStopWalk = fmt.Errorf("stop")
