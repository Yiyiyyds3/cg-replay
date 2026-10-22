// blocks.go —— Artemis 剧本的「块级」入口（事件库与回放构建都要用）。
//
// Artemis 的一个 `.ast` 里含多个 block（剧情段），事件单位就是 block
// （与 `BuildIR` 的粒度一致：每个 block 出一个 ir.Event）。
// 事件库层用它切出事件清单，回放层用它按事件 id 重建出演示。
package artemis

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"ucgr/internal/adapter"
	"ucgr/internal/ir"
)

// ParseAst 解析一份 .ast 剧本（明文 Lua 表）。
func ParseAst(data []byte, name string) (*Ast, error) {
	a := &Adapter{}
	obj, err := a.ParseScript(data, &adapter.ScriptCtx{EntryPath: name})
	if err != nil {
		return nil, err
	}
	ast, ok := obj.(*Ast)
	if !ok {
		return nil, fmt.Errorf("artemis: 脚本 %s 的解析结果类型异常", name)
	}
	return ast, nil
}

// ParseAstFile 读文件并解析。
func ParseAstFile(path string) (*Ast, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseAst(data, filepath.Base(path))
}

// BuildShowBlock 构建某个 block 的演出事件（GUI 逐块回放用）。
//
// blockIdx 从 1 开始（与 BuildIR 里的事件序号一致）。解析器认不出的指令
// 照旧记进 ir.Event 的未识别清单（不静默丢弃）。
func BuildShowBlock(data []byte, name string, blockIdx int) (*ir.Event, error) {
	ast, err := ParseAst(data, name)
	if err != nil {
		return nil, err
	}
	if blockIdx < 1 || blockIdx > len(ast.Blocks) {
		return nil, fmt.Errorf("artemis: %s 里没有第 %d 个 block（共 %d 个）", name, blockIdx, len(ast.Blocks))
	}
	base := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	if base == "" || base == "." {
		base = "script"
	}
	resolve := func(ref string) (*ir.AssetRef, bool) {
		return &ir.AssetRef{OrigPath: ref, Role: "其它"}, true
	}
	ev := buildBlockEvent(base, ast.Blocks[blockIdx-1], blockIdx, resolve)
	// 播放器吃的是顺序台本（ir.Event.Show）——从通用 Timeline 转一份。
	ev.Show = BuildShowFromEvent(ev)
	return ev, nil
}

// BlockTitles 列出每个 block 的序号与标题（事件库切分用；无标题的用序号兜底）。
func BlockTitles(ast *Ast) []struct {
	Index int
	Title string
} {
	if ast == nil {
		return nil
	}
	out := make([]struct {
		Index int
		Title string
	}, 0, len(ast.Blocks))
	for i, b := range ast.Blocks {
		title := strings.TrimSpace(b.Title)
		if title == "" {
			title = b.Name
		}
		out = append(out, struct {
			Index int
			Title string
		}{Index: i + 1, Title: title})
	}
	return out
}
