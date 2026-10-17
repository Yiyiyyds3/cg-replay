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
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"ucgr/internal/adapter"
	"ucgr/internal/adapter/kirikiri"
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
	for _, sp := range scripts {
		data := readScriptMaybeFixed(sp, root)
		ast, err := a.ParseScript(data, &adapter.ScriptCtx{EntryPath: sp})
		if err != nil {
			failed++
			continue
		}
		allRefs = append(allRefs, ast.(*kirikiri.Ast).ExtractRefs()...)
	}
	fmt.Printf("脚本: %d 个（解码失败 %d 个）\n", len(scripts), failed)
	res := pool.Prune(allRefs)
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
	for _, sp := range scripts {
		data := readScriptMaybeFixed(sp, root)
		ast, err := a.ParseScript(data, &adapter.ScriptCtx{EntryPath: sp})
		if err != nil {
			continue
		}
		allRefs = append(allRefs, ast.(*kirikiri.Ast).ExtractRefs()...)
	}
	res := pool.Prune(allRefs)
	rels := res.KeepRels()
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
