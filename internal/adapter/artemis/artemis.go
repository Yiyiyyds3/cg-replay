// Package artemis 实现 Artemis（あかべぇそふとつぅ 系）引擎适配器。
//
// 对应 docs/04_引擎适配器接口规范.md 的五方法 + 转场注册表 + 回退规则；
// 新增引擎只需实现 EngineAdapter 并 Register，核心代码零改动。
//
// 本机语料（3 款，见 reports/P2_原型报告_Artemis引用提取.md）：
//
//	樱之刻解包       文本\sakuranotoki.pfs\script\*.ast   535 个
//	甜蜜女友3解包   原始归档解包\Amakano3.pfs\script\*.ast  1300 个
//	たねつみの歌解包  ...\*.ast                              83 个
//
// .ast 是**明文 Lua 表**（无需解密），结构见 lua.go/ast.go 的注释。
// 识别纪律（docs/04 §4）：签名优先；`.sli` 是 E-mote 相关格式、KiriKiri 与
// Artemis 都可能出现，**禁止**单凭 `.sli` 判引擎；不确定必须返回 ""。
package artemis

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"ucgr/internal/adapter"
)

func init() {
	adapter.Register("artemis", &Adapter{})
}

// dirProbeMaxDepth 目录识别时向下探查的最大层数
// （语料实测：.ast 最深在 <根>\原始归档解包\<卷>.pfs\script\x.ast = 3 层）。
const dirProbeMaxDepth = 5

// scriptExt 是 Artemis 剧本扩展名。
const scriptExt = ".ast"

// archiveExt 是 Artemis 归档（pfs = 多卷容器）扩展名。
const archiveExt = ".pfs"

// Adapter 是 Artemis 引擎适配器。
type Adapter struct{}

// Identify 判定路径是否属于 Artemis。
//
// 规则（保守，宁缺勿滥）：
//   - 文件：扩展名 .ast（剧本）或 .pfs（归档）；
//   - 目录：向下 dirProbeMaxDepth 层内存在 .ast；
//   - 反向证据：目录内根层存在 .xp3（KiriKiri）或 .cst（CatSystem2）时不认领。
func (a *Adapter) Identify(archiveOrDir string) (string, error) {
	fi, err := os.Stat(archiveOrDir)
	if err != nil {
		return "", nil // 路径不可读：不认领，交给别的适配器（IdentifyPath 会报“未能识别引擎”）
	}
	if !fi.IsDir() {
		switch strings.ToLower(filepath.Ext(archiveOrDir)) {
		case scriptExt, archiveExt:
			return "artemis", nil
		}
		return "", nil
	}
	// 反向证据：明确的他家引擎特征
	if hasFileWithExt(archiveOrDir, ".xp3", 1) || hasFileWithExt(archiveOrDir, ".cst", 1) {
		return "", nil
	}
	if hasFileWithExt(archiveOrDir, scriptExt, dirProbeMaxDepth) {
		return "artemis", nil
	}
	return "", nil
}

// hasFileWithExt 在 root 下最多 maxDepth 层内查找指定扩展名（大小写无关）。
func hasFileWithExt(root, ext string, maxDepth int) bool {
	root = filepath.Clean(root)
	found := false
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // 读不到就跳过，不拖垮识别
		}
		if d.IsDir() {
			rel, rerr := filepath.Rel(root, path)
			if rerr != nil {
				return nil
			}
			depth := 0
			if rel != "." {
				depth = len(strings.Split(rel, string(filepath.Separator)))
			}
			if depth > maxDepth {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ext) {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

// ParseArchive 列出归档条目（只列条目，不解压）。
//
// 支持**已解包目录**（WalkDir；.ast 剧本不列入，剧本走 ParseScript）
// 与 .pfs 容器（v2/v6/v8；v8 标记 SHA1(index) XOR 加密）。
func (a *Adapter) ParseArchive(path string) ([]adapter.ArchiveEntry, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !fi.IsDir() {
		if strings.EqualFold(filepath.Ext(path), archiveExt) {
			return parsePFS(path)
		}
		return nil, fmt.Errorf("artemis: 不是目录也不是 .pfs 归档: %s", path)
	}
	var out []adapter.ArchiveEntry
	root := filepath.Clean(path)
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(p), scriptExt) {
			return nil // 剧本单独走 ParseScript
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil {
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil {
			return nil
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

// ParseScript 解析单个 .ast 剧本。
func (a *Adapter) ParseScript(data []byte, ctx *adapter.ScriptCtx) (adapter.ScriptAst, error) {
	name := ""
	if ctx != nil {
		name = ctx.EntryPath
	}
	chunk, err := parseLuaChunk(data)
	if err != nil {
		return nil, fmt.Errorf("artemis: %s: %w", filepath.Base(name), err)
	}
	ast, err := buildAst(chunk, filepath.Base(name))
	if err != nil {
		return nil, fmt.Errorf("artemis: %s: %w", filepath.Base(name), err)
	}
	return ast, nil
}

// Transitions 返回 Artemis 原生转场 → L0 通用键的映射。
//
// L0 通用键（21 项的那张「通用表」）当前引擎实现口径为
// fade / slide_left / slide_right / zoom / wipe（见 ir.IsL0Impl）。
// 语料实测原生转场集中在 extrans/fgf/cgdel 的 rule= 上（crossfade/blind/…），
// 这里只登记已确认的映射；其余原生 id 在 BuildIR 里按
// 「impl=fade + fallback=true」回退（保证不崩能看）。
func (a *Adapter) Transitions() *adapter.TransitionRegistry {
	r := adapter.NewTransitionRegistry()
	r.Register("crossfade", adapter.TransitionImpl{ImplKey: "fade", DurationMs: 800})
	r.Register("fade", adapter.TransitionImpl{ImplKey: "fade", DurationMs: 800})
	r.Register("blind", adapter.TransitionImpl{ImplKey: "wipe", DurationMs: 800})
	r.Register("wipe", adapter.TransitionImpl{ImplKey: "wipe", DurationMs: 800})
	return r
}
