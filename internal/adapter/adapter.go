// Package adapter 定义引擎适配器契约与注册表。
//
// 对应 docs/04_引擎适配器接口规范.md（五方法 + 转场注册表 + 回退规则）。
// 新增引擎只需实现 EngineAdapter 并 Register，核心代码零改动。
package adapter

import (
	"fmt"
	"os"
	"sort"

	"ucgr/internal/ir"
)

// Compression 段压缩方式。
type Compression string

const (
	CompNone  Compression = "none"
	CompZlib  Compression = "zlib"
	CompLzss  Compression = "lzss"
)

// ArchiveEntry 归档内一个条目（不解压内容）。
type ArchiveEntry struct {
	Path             string      `json:"path"`               // 归档内相对路径（统一 '/' 分隔）
	Size             int64       `json:"size"`               // 原始大小
	Offset           int64       `json:"offset"`             // 段数据偏移
	Compression      Compression `json:"compression"`        // 压缩方式
	UncompressedSize int64       `json:"uncompressed_size"`  // 解压后大小（未压缩时同 Size）
	Encrypted        bool        `json:"encrypted"`          // 是否加密
	KeyHint          string      `json:"key_hint,omitempty"` // 加密算法线索（如 XP3 的 adlr）
}

// ScriptCtx 脚本解析上下文。
type ScriptCtx struct {
	GameID    string            `json:"game_id"`
	EntryPath string            `json:"entry_path"`
	Config    map[string]string `json:"config,omitempty"`
}

// BuildCtx IR 构建上下文。
type BuildCtx struct {
	GameID    string            `json:"game_id"`
	Config    map[string]string `json:"config,omitempty"`
	Log       func(format string, args ...interface{})
	RecordUnrecognized func(u ir.Unrecognized)
}

// ScriptAst 引擎专有 AST。对核心不透明：只有产出它的适配器能解释。
type ScriptAst interface{}

// TransitionImpl 转场实现描述。
type TransitionImpl struct {
	ImplKey      string      `json:"impl_key"`
	DurationMs   int64       `json:"duration_ms"`
	ParamsSchema interface{} `json:"params_schema,omitempty"`
}

// TransitionRegistry 引擎原生转场 id → 实现。
type TransitionRegistry struct {
	Impls map[string]TransitionImpl `json:"impls"`
}

// NewTransitionRegistry 构造空注册表。
func NewTransitionRegistry() *TransitionRegistry {
	return &TransitionRegistry{Impls: map[string]TransitionImpl{}}
}

// Register 登记一个转场实现。
func (r *TransitionRegistry) Register(nativeID string, impl TransitionImpl) {
	r.Impls[nativeID] = impl
}

// EngineAdapter 引擎适配器契约（五方法）。
//
// 对应 docs/04 §1。实现方注意：
//   - Identify 不确定必须返回 ""，禁止瞎猜（错判会导致整游戏裁剪错误）
//   - ParseScript 单脚本失败不应中断整个游戏（由调用方跳过并记录）
//   - BuildIR 产物必须通过 ir.Validate 与 ir.CheckAssetRefs
type EngineAdapter interface {
	// Identify 引擎识别：输入归档文件或解包目录，返回 engine_id 或 ""。
	Identify(archiveOrDir string) (string, error)
	// ParseArchive 归档解析：列出条目（不解压内容）。
	ParseArchive(path string) ([]ArchiveEntry, error)
	// ParseScript 脚本解析：产出引擎专有 AST。
	ParseScript(data []byte, ctx *ScriptCtx) (ScriptAst, error)
	// BuildIR 构建 IR。
	BuildIR(ast ScriptAst, ctx *BuildCtx) (*ir.IR, error)
	// Transitions 转场注册表。
	Transitions() *TransitionRegistry
}

// registry 引擎适配器注册表。
var registry = map[string]EngineAdapter{}

// Register 注册一个引擎适配器（重复注册同名 id 会 panic，防止静默覆盖）。
func Register(engineID string, a EngineAdapter) {
	if engineID == "" {
		panic("adapter: engine_id 不能为空")
	}
	if _, dup := registry[engineID]; dup {
		panic("adapter: engine_id 重复注册: " + engineID)
	}
	registry[engineID] = a
}

// Get 按 id 取适配器。
func Get(engineID string) (EngineAdapter, bool) {
	a, ok := registry[engineID]
	return a, ok
}

// IDs 返回全部已注册引擎 id（排序）。
func IDs() []string {
	ids := make([]string, 0, len(registry))
	for id := range registry {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// IdentifyPath 对单个路径竞聘全部已注册适配器。
// 平票按注册顺序（IDs 排序）取第一个；全部无意见识失败。
func IdentifyPath(path string) (string, error) {
	type hit struct {
		id       string
		priority int
	}
	var hits []hit
	for _, id := range IDs() {
		a := registry[id]
		got, err := a.Identify(path)
		if err != nil {
			continue // 单个适配器识别失败不拖垮整体
		}
		if got != "" {
			hits = append(hits, hit{id: got})
		}
	}
	if len(hits) == 0 {
		return "", fmt.Errorf("未能识别引擎: %s", path)
	}
	if len(hits) > 1 {
		ids := make([]string, 0, len(hits))
		for _, h := range hits {
			ids = append(ids, h.id)
		}
		return "", fmt.Errorf("引擎识别歧义: %s -> %v", path, ids)
	}
	return hits[0].id, nil
}

// ReadArchiveEntry 按条目读取并解压完整内容（段拼接 + zlib）。
func ReadArchiveEntry(f *os.File, e ArchiveEntry) ([]byte, error) {
	if _, err := f.Seek(e.Offset, 0); err != nil {
		return nil, err
	}
	raw := make([]byte, e.Size)
	if _, err := f.Read(raw); err != nil {
		return nil, fmt.Errorf("读取段失败 %s: %w", e.Path, err)
	}
	if e.Compression == CompZlib {
		// 延迟到调用方解压：某些引擎压缩段外还叠加密层
		return raw, nil
	}
	return raw, nil
}
