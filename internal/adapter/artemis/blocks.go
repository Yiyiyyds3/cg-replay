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
	return BuildShowBlockStage(data, name, blockIdx, 1920, 1080)
}

// BuildShowBlockStage 同 BuildShowBlock，但按给定舞台尺寸摆对话框。
func BuildShowBlockStage(data []byte, name string, blockIdx, stageW, stageH int) (*ir.Event, error) {
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
	ev.Show = withMessageBox(BuildShowFromEvent(ev), stageW, stageH)
	return ev, nil
}

// ScriptTitle 整份剧本的展示名：首个有标题的 block 的标题，没有就用文件名。
func ScriptTitle(ast *Ast) string {
	if ast == nil {
		return ""
	}
	for _, b := range ast.Blocks {
		if t := strings.TrimSpace(b.Title); t != "" {
			return t
		}
	}
	return ""
}

// BuildShowScript 把整份 .ast 的所有 block 按块序拼成一个可回放事件。
//
// 为什么不用「一 block 一事件」：Artemis 的 block 粒度很细（实测一个
// 65 KiB 剧本有 179 个 block），按 block 出事件会让樱之刻这类作品产出
// 十万级条目，列表没法用。整份剧本作为事件单位，与 scn / BGI 的场景
// 事件粒度一致；块内顺序按文件顺序（不追 jump/call 的分支）。
//
// 舞台尺寸取 Artemis 语料的常见值 1920×1080；壳层知道更准确的尺寸时
// 用 BuildShowScriptStage。
func BuildShowScript(data []byte, name string) (*ir.Event, error) {
	return BuildShowScriptStage(data, name, 1920, 1080)
}

// BuildShowScriptStage 同 BuildShowScript，但按给定舞台尺寸摆对话框。
func BuildShowScriptStage(data []byte, name string, stageW, stageH int) (*ir.Event, error) {
	ast, err := ParseAst(data, name)
	if err != nil {
		return nil, err
	}
	if len(ast.Blocks) == 0 {
		return nil, fmt.Errorf("artemis: %s 里没有 block", name)
	}
	base := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	if base == "" || base == "." {
		base = "script"
	}
	resolve := func(ref string) (*ir.AssetRef, bool) {
		return &ir.AssetRef{OrigPath: ref, Role: "其它"}, true
	}
	out := &ir.Event{EventID: base, Kind: ir.KindScene, Ord: 1, Title: ScriptTitle(ast)}
	seenAsset := map[string]bool{}
	seenChar := map[string]bool{}
	var clock int64
	for i, blk := range ast.Blocks {
		one := buildBlockEvent(base, blk, i+1, resolve)
		// 块内多行是「同一屏」的连续文本，块与块之间才换屏：
		// 不插 text_clear 的话整份剧本的台词会全堆在最后一屏。
		for _, op := range withBlockBreaks(BuildShowFromEvent(one)) {
			op.TMs += clock
			out.Show = append(out.Show, op)
		}
		remap := map[string]string{}
		for _, t := range one.Texts {
			t.TMs += clock
			nid := fmt.Sprintf("L%04d", len(out.Texts)+1)
			remap[t.LineID] = nid
			t.LineID = nid
			out.Texts = append(out.Texts, t)
		}
		for _, v := range one.Voices {
			if nid, ok := remap[v.LineID]; ok {
				v.LineID = nid
			}
			out.Voices = append(out.Voices, v)
		}
		for _, a := range one.Assets {
			if seenAsset[a.OrigPath] {
				continue
			}
			seenAsset[a.OrigPath] = true
			out.Assets = append(out.Assets, a)
		}
		for _, c := range one.Characters {
			if !seenChar[c] {
				seenChar[c] = true
				out.Characters = append(out.Characters, c)
			}
		}
		out.Unrecognized = append(out.Unrecognized, one.Unrecognized...)
		// 块之间至少推进半秒，避免「没有任何 wait 的块」全部压在同一时刻。
		clock += one.EndMs
		if one.EndMs <= 0 {
			clock += 500
		}
	}
	out.EndMs = clock
	out.Show = withMessageBox(out.Show, stageW, stageH)
	return out, nil
}

// withBlockBreaks 在一个 block 的台本前后补「换屏」指令：
// 首个台词前 text_clear（清掉上一块），末个台词后 text_wait（等玩家点击）。
func withBlockBreaks(ops []ir.ShowOp) []ir.ShowOp {
	first, last := -1, -1
	for i, op := range ops {
		if op.Kind == ir.ShowTextRun {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	if first < 0 {
		return ops
	}
	out := make([]ir.ShowOp, 0, len(ops)+2)
	for i, op := range ops {
		if i == first {
			out = append(out, ir.ShowOp{TMs: op.TMs, Kind: ir.ShowTextClear, Layer: msgLayer})
		}
		out = append(out, op)
		if i == last {
			out = append(out, ir.ShowOp{TMs: op.TMs, Kind: ir.ShowTextWait})
		}
	}
	return out
}

// withMessageBox 在台本最前面补「底部对话框」的层指令。
func withMessageBox(ops []ir.ShowOp, stageW, stageH int) []ir.ShowOp {
	if len(ops) == 0 {
		return ops
	}
	return append(ir.MessageBoxOps(msgLayer, stageW, stageH), ops...)
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

// BlockMovieFlags 与 BlockTitles 同序：每个 block 是否含影片指令。
//
// Artemis 的影片是 `movie` 命令（buildIR 里映射成 ir.LayerMovie）。这里只做
// 「从 Artemis 语法里认出影片」这一件事，判定「算不算影片事件」交给 ir.HasVideo。
func BlockMovieFlags(ast *Ast) []bool {
	if ast == nil {
		return nil
	}
	out := make([]bool, 0, len(ast.Blocks))
	for _, b := range ast.Blocks {
		has := false
		for _, it := range b.Seq {
			if it.Cmd == nil {
				continue
			}
			if strings.EqualFold(it.Cmd.Tag, "movie") {
				has = true
				break
			}
		}
		out = append(out, has)
	}
	return out
}
