// Command ucgr 是统一 CG 演出回放库的核心 CLI。
//
// 对应上游 FINAL 1017 §10「核心 CLI = Go 独立可执行文件」：
// 摄入/扫描/哈希/裁剪全用 Go 实现，编译为单 exe，
// 可通过 Wails IPC 或命令行直接调用。
//
// 子命令：
//
//	ucgr scan  <目录>             S0 体积盘点（按扩展名分类统计）
//	ucgr probe <xp3>              解析 XP3 索引，输出条目概况
//	ucgr refs  <目录> <脚本...>    提取脚本资源引用并匹配解包池
//	ucgr prune <目录> <脚本...>    引用裁剪：保留集复制到目标目录
//	ucgr engines                 列出已注册引擎适配器
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"ucgr/internal/adapter"
	"ucgr/internal/adapter/kirikiri"
	"ucgr/internal/hash"
	"ucgr/internal/ir"
	"ucgr/internal/prune"
	"ucgr/internal/scan"

	// 空白导入触发各适配器 init() 注册
	_ "ucgr/internal/adapter/kirikiri"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]
	args := os.Args[2:]
	var err error
	switch cmd {
	case "scan":
		err = cmdScan(args)
	case "probe":
		err = cmdProbe(args)
	case "refs":
		err = cmdRefs(args)
	case "prune":
		err = cmdPrune(args)
	case "ir":
		err = cmdIR(args)
	case "replay":
		err = cmdReplay(args)
	case "engines":
		cmdEngines()
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `ucgr — 统一 CG 演出回放库核心 CLI

用法:
  ucgr scan  <目录>             S0 体积盘点
  ucgr probe <xp3 文件>         解析 XP3 索引概况
  ucgr refs  <解包目录> <脚本…>  提取引用并匹配资源池
  ucgr prune <解包目录> <脚本…> [--out 目录]  裁剪保留集
  ucgr ir    <解包目录> <脚本> [--out file.json]  构建 IR（匹配+哈希+校验）
  ucgr replay <ir.json>          文本回放（按时间轴打印演出）
  ucgr engines                 列出已注册引擎
`)
}

func cmdScan(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("用法: ucgr scan <目录>")
	}
	res, err := scan.ScanDir(args[0], 0)
	if err != nil {
		return err
	}
	fmt.Print(scan.FormatReport(res))
	return nil
}

func cmdProbe(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("用法: ucgr probe <xp3>")
	}
	var a adapter.EngineAdapter = &kirikiri.Adapter{}
	ents, err := a.ParseArchive(args[0])
	if err != nil {
		return err
	}
	var total, comp, enc int64
	for _, e := range ents {
		total += e.UncompressedSize
		if e.Compression == adapter.CompZlib {
			comp++
		}
		if e.Encrypted {
			enc++
		}
	}
	fmt.Printf("归档: %s\n", args[0])
	fmt.Printf("条目: %d    原始体积: %s    压缩段: %d    加密条目: %d\n",
		len(ents), prune.HumanBytes(total), comp, enc)
	// 扩展名分布
	dist := map[string]int{}
	for _, e := range ents {
		ext := e.Path[strings.LastIndex(e.Path, "."):]
		if i := strings.LastIndex(e.Path, "/"); i >= 0 {
			ext = e.Path[strings.LastIndex(e.Path, "."):]
		}
		dist[strings.ToLower(ext)]++
	}
	type kv struct {
		k string
		v int
	}
	var top []kv
	for k, v := range dist {
		top = append(top, kv{k, v})
	}
	sort.Slice(top, func(i, j int) bool { return top[i].v > top[j].v })
	s := make([]string, 0, 8)
	for i, e := range top {
		if i >= 8 {
			break
		}
		s = append(s, fmt.Sprintf("%s=%d", e.k, e.v))
	}
	fmt.Println("扩展名分布:", strings.Join(s, " "))
	return nil
}

// collectScripts 展开脚本参数：文件直接用；目录则递归找 .ks
// （排除 解包输出/_分类 副本树）。
func collectScripts(args []string) ([]string, error) {
	var out []string
	for _, a := range args {
		info, err := os.Stat(a)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			out = append(out, a)
			continue
		}
		err = filepath.WalkDir(a, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if d.Name() == "解包输出" || d.Name() == "_分类" {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.EqualFold(filepath.Ext(p), ".ks") {
				out = append(out, p)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// readScriptMaybeFixed 读脚本；解码失败时回退到 <游戏目录>_脚本修复/ 的修复副本
// （ks_fix9.py 的产出：前人解密不完整的脚本已修好放在那里）。
// 查找顺序：精确 relpath → 去掉首段 → 全树 basename glob（与 Python 版一致）。
func readScriptMaybeFixed(script, gameDir string) []byte {
	data, err := os.ReadFile(script)
	if err != nil {
		return nil
	}
	fixRoot := gameDir + "_脚本修复"
	if _, err := os.Stat(fixRoot); err != nil {
		return data
	}
	rel, err := filepath.Rel(gameDir, script)
	if err != nil {
		return data
	}
	cands := []string{filepath.Join(fixRoot, rel)}
	parts := strings.Split(rel, string(os.PathSeparator))
	if len(parts) > 1 {
		cands = append(cands, filepath.Join(append([]string{fixRoot}, parts[1:]...)...))
	}
	if g, _ := filepath.Glob(filepath.Join(fixRoot, "**", filepath.Base(script))); len(g) > 0 {
		cands = append(cands, g...)
	}
	for _, c := range cands {
		if fixed, err := os.ReadFile(c); err == nil {
			return fixed
		}
	}
	return data
}

func cmdRefs(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("用法: ucgr refs <解包目录> <脚本|目录…>")
	}
	root := args[0]
	pool, err := prune.BuildPool(root)
	if err != nil {
		return err
	}
	scripts, err := collectScripts(args[1:])
	if err != nil {
		return err
	}
	var a adapter.EngineAdapter = &kirikiri.Adapter{}
	var allRefs []string
	failed := 0
	dynVoice := false
	for _, sp := range scripts {
		data := readScriptMaybeFixed(sp, root)
		ast, err := a.ParseScript(data, &adapter.ScriptCtx{EntryPath: sp})
		if err != nil {
			failed++
			continue
		}
		if ast.(*kirikiri.Ast).DynamicVoice {
			dynVoice = true
		}
		allRefs = append(allRefs, ast.(*kirikiri.Ast).ExtractRefs()...)
	}
	fmt.Printf("脚本: %d 个（解码失败 %d 个）\n", len(scripts), failed)
	if failed > 0 {
		fmt.Printf("⚠️ %d 个脚本解不动 → 按安全原则整游戏保守保留，下列数字仅为下界\n", failed)
	}
	res := pool.Prune(allRefs)
	if dynVoice {
		if n := res.GuardCategory(pool, "voice"); n > 0 {
			fmt.Printf("🔒 动态语音守卫：+%d 个语音文件（脚本用 voiceBase 动态拼名，静态不可解）\n", n)
		}
	}
	fmt.Printf("池: %d 个文件 / %s\n", len(pool.ByRel), prune.HumanBytes(res.PoolBytes))
	fmt.Printf("命中: %d 个文件 / %s\n", len(res.Matched), prune.HumanBytes(keepOf(res.Matched, pool)))
	fmt.Printf("保守保留: %d 个文件 / %s\n", len(res.Dynamic), prune.HumanBytes(keepOf(res.Dynamic, pool)))
	fmt.Printf("未命中引用: %d 条\n", len(res.Unmatched))
	if len(res.Unmatched) > 0 {
		show := res.Unmatched
		if len(show) > 12 {
			show = show[:12]
		}
		fmt.Println("  样例:", strings.Join(show, ", "))
	}
	fmt.Printf("保留率: %.1f%%\n", pct(res.KeepBytes, res.PoolBytes))
	return nil
}

func cmdPrune(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("用法: ucgr prune <解包目录> <脚本|目录…> [--out 目录]")
	}
	outDir := ""
	rest := args
	for i := 0; i < len(rest)-1; i++ {
		if rest[i] == "--out" {
			outDir = rest[i+1]
			rest = append(rest[:i], rest[i+2:]...)
			break
		}
	}
	if outDir == "" {
		return fmt.Errorf("必须指定 --out <目录>")
	}
	root := rest[0]
	pool, err := prune.BuildPool(root)
	if err != nil {
		return err
	}
	scripts, err := collectScripts(rest[1:])
	if err != nil {
		return err
	}
	var a adapter.EngineAdapter = &kirikiri.Adapter{}
	var allRefs []string
	dynVoice := false
	failed := 0
	for _, sp := range scripts {
		data := readScriptMaybeFixed(sp, root)
		ast, err := a.ParseScript(data, &adapter.ScriptCtx{EntryPath: sp})
		if err != nil {
			failed++
			continue
		}
		if ast.(*kirikiri.Ast).DynamicVoice {
			dynVoice = true
		}
		allRefs = append(allRefs, ast.(*kirikiri.Ast).ExtractRefs()...)
	}
	res := pool.Prune(allRefs)
	if dynVoice {
		if n := res.GuardCategory(pool, "voice"); n > 0 {
			fmt.Printf("🔒 动态语音守卫：+%d 个语音文件\n", n)
		}
	}
	rels := res.KeepRels()
	if failed > 0 {
		// 安全原则：有脚本解不动 → 整游戏保留（不做部分裁剪）
		rels = make([]string, 0, len(pool.ByRel))
		for rel := range pool.ByRel {
			rels = append(rels, rel)
		}
		sort.Strings(rels)
		fmt.Printf("🔒 %d 个脚本解码失败 → 按安全原则保留【整个池】%d 个文件\n",
			failed, len(rels))
	}
	n, failed, err := pool.CopyTo(outDir, rels)
	if err != nil {
		return err
	}
	fmt.Printf("保留 %d 个文件 / %s → %s（跳过 %d 个）\n", len(rels), prune.HumanBytes(n), outDir, failed)
	fmt.Printf("池 %s，剔除 %s（%.1f%%）\n",
		prune.HumanBytes(res.PoolBytes),
		prune.HumanBytes(res.PoolBytes-res.KeepBytes),
		100-pct(res.KeepBytes, res.PoolBytes))
	return nil
}

// cmdIR 构建 IR：解析脚本 → 匹配资源池 + 哈希 → BuildIR → 校验 → 写 JSON。
// 用法：ucgr ir <解包目录> <脚本> [--out file.json]
func cmdIR(args []string) error {
	outPath := ""
	rest := args
	for i := 0; i < len(rest)-1; i++ {
		if rest[i] == "--out" {
			outPath = rest[i+1]
			rest = append(rest[:i], rest[i+2:]...)
			break
		}
	}
	if len(rest) < 2 {
		return fmt.Errorf("用法: ucgr ir <解包目录> <脚本> [--out file.json]")
	}
	root, script := rest[0], rest[1]
	pool, err := prune.BuildPool(root)
	if err != nil {
		return err
	}
	data := readScriptMaybeFixed(script, root)
	var a adapter.EngineAdapter = &kirikiri.Adapter{}
	ast, err := a.ParseScript(data, &adapter.ScriptCtx{EntryPath: script})
	if err != nil {
		return err
	}
	hashCache := map[string]string{}
	resolver := func(ref string) (*ir.AssetRef, bool) {
		rel := ""
		if hits := pool.Match(ref); len(hits) > 0 {
			rel = hits[0]
		} else if cands := pool.DynamicCandidates(ref); len(cands) > 0 {
			rel = cands[0] // 保守：取第一个候选（已在 ④ 保留集内）
		} else {
			return nil, false
		}
		h, ok := hashCache[rel]
		if !ok {
			if h, err = hash.FileSHA256(pool.ByRel[rel].Abs); err != nil {
				return nil, false
			}
			hashCache[rel] = h
		}
		return &ir.AssetRef{Hash: h, OrigPath: rel, Role: "其它"}, true
	}
	var unrecognized []ir.Unrecognized
	out, err := a.BuildIR(ast, &adapter.BuildCtx{
		GameID:             filepath.Base(root),
		Config:             map[string]string{"script_name": script},
		RecordUnrecognized: func(u ir.Unrecognized) { unrecognized = append(unrecognized, u) },
		Resolver:           resolver,
	})
	if err != nil {
		return err
	}
	// 校验：V1 需要包内哈希集合 = 全部已解析资产
	pkgHashes := map[string]bool{}
	for _, ev := range out.Events {
		for _, as := range ev.Assets {
			pkgHashes[as.Hash] = true
		}
	}
	errs := ir.Validate(out)
	errs = append(errs, ir.CheckAssetRefs(out, pkgHashes)...)
	if len(errs) > 0 {
		fmt.Printf("⚠️ IR 校验未过（%d 条）：\n", len(errs))
		for i, e := range errs {
			if i >= 8 {
				fmt.Printf("   …另 %d 条\n", len(errs)-8)
				break
			}
			fmt.Println("   -", e)
		}
	} else {
		fmt.Println("✅ IR 校验通过（V1~V8）")
	}
	// 摘要：逐事件清单
	fmt.Printf("共 %d 个事件（CG show→hide 切分）：\n", len(out.Events))
	for i, ev := range out.Events {
		if i >= 12 {
			fmt.Printf("  …另 %d 个事件\n", len(out.Events)-12)
			break
		}
		nv := 0
		for _, t := range ev.Texts {
			if t.Voice != nil {
				nv++
			}
		}
		fmt.Printf("  #%-2d %-16s %-10s 图层%d 台词%d(语音%d) 转场%d 资产%d 未识别%d\n",
			ev.Ord, ev.EventID, ev.Kind, len(ev.Layers), len(ev.Texts), nv,
			len(ev.Transitions), len(ev.Assets), len(ev.Unrecognized))
	}
	if outPath != "" {
		b, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(outPath, b, 0o644); err != nil {
			return err
		}
		fmt.Printf("IR 已写入 %s（%d 字节）\n", outPath, len(b))
	}
	return nil
}

// cmdReplay 文本回放：按 IR 时间轴顺序打印演出过程（§7.5.5 方式 A）。
// 用法：ucgr replay <ir.json>
func cmdReplay(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("用法: ucgr replay <ir.json>")
	}
	b, err := os.ReadFile(args[0])
	if err != nil {
		return err
	}
	var doc ir.IR
	if err := json.Unmarshal(b, &doc); err != nil {
		return fmt.Errorf("IR 解析失败: %w", err)
	}
	if errs := ir.Validate(&doc); len(errs) > 0 {
		fmt.Printf("⚠️ IR 校验未过（%d 条），仍尝试回放\n", len(errs))
	}
	fmt.Printf("游戏 %s · 引擎 %s · 保真度 %s · 事件 %d 个\n",
		doc.GameID, doc.EngineID, doc.Fidelity, len(doc.Events))
	for _, ev := range doc.Events {
		fmt.Printf("\n=== 事件 %s（%s）图层 %d 台词 %d ===\n",
			ev.EventID, ev.Kind, len(ev.Layers), len(ev.Texts))
		// 图层清单
		for _, l := range ev.Layers {
			name := "-"
			if l.Asset != nil {
				name = l.Asset.OrigPath
			}
			fmt.Printf("  [图层 %-6s %-10s z=%d %s]\n", l.LayerID, l.Kind, l.Z, name)
		}
		// 时间轴 + 台词合并打印
		textByID := map[string]ir.TextLine{}
		for _, t := range ev.Texts {
			textByID[t.LineID] = t
		}
		fmt.Println("  --- 时间轴 ---")
		for _, op := range ev.Timeline {
			switch op.Op {
			case "text_show":
				t := textByID[op.Target]
				spk := t.Speaker
				if spk == "" {
					spk = "?"
				}
				voice := ""
				if t.Voice != nil {
					voice = " ♪" + t.Voice.OrigPath
				}
				fmt.Printf("  %6dms 【%s】%s%s\n", op.TMs, spk, truncate(t.Text, 50), voice)
			case "image_show":
				ref := ""
				if p, ok := op.Params.(map[string]interface{}); ok {
					if r, ok := p["ref"].(string); ok {
						ref = r
					}
				}
				fmt.Printf("  %6dms 🖼 图层 %s ← %s\n", op.TMs, op.Target, ref)
			case "voice_play":
				fmt.Printf("  %6dms 🔊 %s\n", op.TMs, op.Target)
			default:
				fmt.Printf("  %6dms · %s %s\n", op.TMs, op.Op, op.Target)
			}
		}
		for _, tr := range ev.Transitions {
			fb := ""
			if tr.Fallback {
				fb = "（L0 近似）"
			}
			fmt.Printf("  [转场 %s → %s %dms%s]\n", tr.TransitionID, tr.Impl, tr.DurationMs, fb)
		}
		for _, br := range ev.Branches {
			fmt.Printf("  [分支 %s → %s（默认目标 %s）]\n", br.BranchID, br.Type, br.DefaultTarget)
		}
	}
	return nil
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func cmdEngines() {
	for _, id := range adapter.IDs() {
		fmt.Println(id)
	}
}

func keepOf(m map[string][]string, p *prune.Pool) int64 {
	var t int64
	for k := range m {
		t += p.ByRel[k].Size
	}
	return t
}

func pct(a, b int64) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b) * 100
}
