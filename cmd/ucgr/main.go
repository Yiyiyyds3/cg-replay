// Command ucgr 是统一 CG 演出回放库的核心 CLI。
//
// 对应上游 FINAL 1017 §10「核心 CLI = Go 独立可执行文件」：
// 摄入/扫描/哈希/裁剪全用 Go 实现，编译为单 exe，
// 可通过 Wails IPC 或命令行直接调用。
//
// 子命令：
//
//	ucgr scan  <目录>             S0 体积盘点（按扩展名分类统计）
//	ucgr probe <归档文件>          解析归档索引，输出条目概况
//	ucgr refs  <目录> <脚本...>    提取脚本资源引用并匹配解包池
//	ucgr prune <目录> <脚本...>    引用裁剪：保留集复制到目标目录
//	ucgr decode <图片>           按需解码资产（TLG5/JPEG → PNG）
//	ucgr unpack <归档>           解包归档（CatSystem2 KIF：.int/.kif）
//	ucgr engines                 列出已注册引擎适配器
package main

import (
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"ucgr/internal/adapter"
	"ucgr/internal/adapter/kirikiri"
	"ucgr/internal/audio"
	"ucgr/internal/decode"
	"ucgr/internal/hash"
	"ucgr/internal/ir"
	"ucgr/internal/kif"
	"ucgr/internal/library"
	"ucgr/internal/platform/win32"
	"ucgr/internal/player"
	"ucgr/internal/prune"
	"ucgr/internal/replay"
	"ucgr/internal/scan"

	// 空白导入触发各适配器 init() 注册（新增引擎在此加一行）
	_ "ucgr/internal/adapter/artemis"
	_ "ucgr/internal/adapter/bgi"
	_ "ucgr/internal/adapter/catsystem2"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]
	args := stripEngineFlag(os.Args[2:])
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
	case "play":
		err = cmdPlay(args)
	case "events":
		err = cmdEvents(args)
	case "decode":
		err = cmdDecode(args)
	case "unpack":
		err = cmdUnpack(args)
	case "engines":
		cmdEngines()
	case "audio":
		err = cmdAudio(args)
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
  ucgr probe <归档文件>          解析归档索引概况
  ucgr refs  <解包目录> <脚本…>  提取引用并匹配资源池
  ucgr prune <解包目录> <脚本…> [--out 目录]  裁剪保留集
  ucgr ir    <解包目录> <脚本> [--out file.json]  构建 IR（匹配+哈希+校验）
  ucgr replay <ir.json> [--assets 素材根] [--render 目录] [--gif out.gif]
                [--at ms,ms,…] [--frames N] [--size WxH] [--event N]
                             回放 IR：默认打印时间轴；给 --assets+--render/--gif
                             则只从 IR + 素材合成画面（S3「可播放演出」，掩码 JPEG 自动还原）
  ucgr play  <ir.json> --assets <素材根> [--event N] [--speed X] [--char-ms N] [--no-wait]
                [--shot 目录 [--at ms,ms,…]] [--size WxH] [--font 名字] [--font-file otf]
                             按原作演出播放事件：图层双缓冲 + 转场 + 抖动 + 对话框文本
                             带 --at 时离线出 PNG（不弹窗），否则开窗口实时播放
                             （空格/回车 推进，P 暂停，Tab 加速，S 跳等待，F12 截图，Esc 退出）
  ucgr play  --script <脚本.ks> [--label cg01] [--assets 素材根] [其余同上]
                             直接按原作脚本的某个标签段（原作自己的事件单位）播放，
                             不经过 ir.json —— CG 鉴赏的 *cgNN、场景的整脚本都行
  ucgr events <解包目录> [--out 库.json]      扫出「原作自己的事件表」：
                             每个 CG 一个独立事件（cgrai 画廊槽位 + cgrai00.ks 的 *cgNN），
                             每个编号剧本一个场景事件；--out 落盘给 GUI 用
  ucgr decode <图片文件> [--out out.png] [--raw out.bin]  按需解码（TLG5/JPEG→PNG，自动还原 XOR 掩码）
  ucgr unpack <归档> [--out 目录] [--key 口令] [--limit N]  解包归档（CatSystem2 KIF）
  ucgr engines                 列出已注册引擎
  ucgr audio <音频文件> [--out out.pcm] [--play-ms N]   解码音频并打印格式（Windows 走 Media Foundation，
                               OGG/Vorbis 依赖系统解码器）；--out 落盘原始 PCM（交错、小端）

全局参数:
  --engine <id>                跳过自动识别，直接指定引擎（id 见 ucgr engines）
`)
}

// gameRootOf 从脚本路径反推解包根（<root>\raw\data\scenario\x.ks → <root>）。
func gameRootOf(script string) string {
	p := filepath.Clean(script)
	sep := string(filepath.Separator)
	if i := strings.Index(strings.ToLower(p), strings.ToLower(sep+"raw"+sep)); i > 0 {
		return p[:i]
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(p))))
}

// cmdAudio 解码一个音频文件并打印格式；--out 时落盘原始 PCM（交错、小端）。
func cmdAudio(args []string) error {
	var file, out string
	var playMs int64
	for i := 0; i < len(args); i++ {
		switch a := args[i]; a {
		case "--out":
			if i+1 >= len(args) {
				return fmt.Errorf("--out 缺少取值")
			}
			i++
			out = args[i]
		case "--play-ms":
			if i+1 >= len(args) {
				return fmt.Errorf("--play-ms 缺少取值")
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil {
				return fmt.Errorf("--play-ms 取值无效: %v", err)
			}
			playMs = int64(n)
		case "-h", "--help":
			fmt.Println("用法: ucgr audio <音频文件> [--out out.pcm] [--play-ms N]")
			return nil
		default:
			if file == "" {
				file = a
			} else {
				return fmt.Errorf("多余的参数: %s", a)
			}
		}
	}
	if file == "" {
		return fmt.Errorf("缺少音频文件（用法: ucgr audio <音频文件> [--out out.pcm] [--play-ms N]）")
	}
	fi, err := os.Stat(file)
	if err != nil {
		return err
	}
	pcm, err := audio.DecodeFile(file)
	if err != nil {
		return err
	}
	fmt.Printf("音频 %s\n", file)
	fmt.Printf("  文件 %d 字节\n", fi.Size())
	fmt.Printf("  解码 %s\n", pcm)
	if out != "" {
		if err := os.WriteFile(out, pcm.Data, 0o644); err != nil {
			return err
		}
		fmt.Printf("  已写入 %s\n", out)
	}
	if playMs > 0 {
		fmt.Printf("  播放 %d ms（waveOut 混音）…\n", playMs)
		if err := audio.PlayOnce(pcm, playMs); err != nil {
			return err
		}
	}
	return nil
}

// cmdEvents 扫出「原作自己的事件表」：每个 CG 一个独立事件 + 每个编号剧本一个场景事件。
func cmdEvents(args []string) error {
	var root, out string
	for i := 0; i < len(args); i++ {
		switch a := args[i]; a {
		case "--out":
			if i+1 >= len(args) {
				return fmt.Errorf("--out 缺少取值")
			}
			i++
			out = args[i]
		default:
			if root == "" {
				root = a
			} else {
				return fmt.Errorf("多余参数: %s", a)
			}
		}
	}
	if root == "" {
		return fmt.Errorf("用法: ucgr events <解包目录> [--out 库.json]")
	}
	lib, err := library.Scan(root)
	if err != nil {
		return err
	}
	fmt.Printf("事件库 %s（%s）\n  CG 事件 %d 个 · 场景事件 %d 个 · 演出段落 %d 个\n",
		lib.Game, lib.Root, len(lib.CGs), len(lib.Scenes), len(lib.Chapters))
	const showMax = 24
	for i, c := range lib.CGs {
		if i >= showMax {
			fmt.Printf("  …（CG 其余 %d 个略）\n", len(lib.CGs)-i)
			break
		}
		imgs := strings.Join(c.Pages, " → ")
		if imgs == "" {
			imgs = "（未解析到图）"
		}
		fmt.Printf("  %-7s #%-3d %-4s %s\n", c.ID, c.Index, c.Label, imgs)
	}
	const sceneMax = 12
	for i, s := range lib.Scenes {
		if i >= sceneMax {
			fmt.Printf("  …（场景其余 %d 个略）\n", len(lib.Scenes)-i)
			break
		}
		fmt.Printf("  场景 %-14s %s\n", s.ID, s.Script)
	}
	const chapMax = 16
	for i, c := range lib.Chapters {
		if i >= chapMax {
			fmt.Printf("  …（演出段落其余 %d 个略）\n", len(lib.Chapters)-i)
			break
		}
		fmt.Printf("  段落 #%-3d %-10s %s\n", c.Index, c.Scene, c.Title)
	}
	if out != "" {
		if err := lib.Save(out); err != nil {
			return err
		}
		fmt.Printf("已写入 %s\n", out)
	}
	return nil
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
		return fmt.Errorf("用法: ucgr probe <归档文件>")
	}
	a, engineID, err := adapterFor(args[0])
	if err != nil {
		return err
	}
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
	fmt.Printf("归档: %s（引擎 %s）\n", args[0], engineID)
	fmt.Printf("条目: %d    原始体积: %s    压缩段: %d    加密条目: %d\n",
		len(ents), prune.HumanBytes(total), comp, enc)
	// 扩展名分布
	dist := map[string]int{}
	for _, e := range ents {
		name := e.Path
		if i := strings.LastIndexByte(name, '/'); i >= 0 {
			name = name[i+1:]
		}
		ext := "(无扩展名)"
		if i := strings.LastIndexByte(name, '.'); i >= 0 {
			ext = name[i:]
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

// collectScripts 展开脚本参数：文件直接用；目录则递归找 .ks / .cst
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
			ext := strings.ToLower(filepath.Ext(p))
			if ext == ".ks" || ext == ".cst" {
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
func readScriptMaybeFixed(script, gameDir string) ([]byte, error) {
	data, err := os.ReadFile(script)
	if err != nil {
		// 找不到就明确报错，别把空 data 交给解析器（会以越界 panic 收场）。
		return nil, fmt.Errorf("读不到脚本 %s：%w", script, err)
	}
	fixRoot := gameDir + "_脚本修复"
	if _, err := os.Stat(fixRoot); err != nil {
		return data, nil
	}
	rel, err := filepath.Rel(gameDir, script)
	if err != nil {
		return data, nil
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
			return fixed, nil
		}
	}
	return data, nil
}

func cmdRefs(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("用法: ucgr refs <解包目录> <脚本|目录…>")
	}
	root := args[0]
	a, engineID, err := adapterFor(root)
	if err != nil {
		return err
	}
	pool, err := poolFor(root, engineID)
	if err != nil {
		return err
	}
	scripts, err := collectScripts(args[1:])
	if err != nil {
		return err
	}
	var allRefs []string
	failed := 0
	dynVoice := false
	for _, sp := range scripts {
		data, rerr := readScriptMaybeFixed(sp, root)
		if rerr != nil {
			fmt.Fprintf(os.Stderr, "⚠️ %v\n", rerr)
			failed++
			continue
		}
		ast, err := a.ParseScript(data, &adapter.ScriptCtx{EntryPath: sp})
		if err != nil {
			failed++
			continue
		}
		refs, ok := astRefs(ast)
		if !ok {
			failed++ // AST 不支持引用提取 → 按安全原则整游戏保留
			continue
		}
		if kv, ok := ast.(*kirikiri.Ast); ok && kv.DynamicVoice {
			dynVoice = true
		}
		allRefs = append(allRefs, refs...)
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
	a, engineID, err := adapterFor(root)
	if err != nil {
		return err
	}
	pool, err := poolFor(root, engineID)
	if err != nil {
		return err
	}
	scripts, err := collectScripts(rest[1:])
	if err != nil {
		return err
	}
	var allRefs []string
	dynVoice := false
	failed := 0
	for _, sp := range scripts {
		data, rerr := readScriptMaybeFixed(sp, root)
		if rerr != nil {
			fmt.Fprintf(os.Stderr, "⚠️ %v\n", rerr)
			failed++
			continue
		}
		ast, err := a.ParseScript(data, &adapter.ScriptCtx{EntryPath: sp})
		if err != nil {
			failed++
			continue
		}
		refs, ok := astRefs(ast)
		if !ok {
			failed++ // AST 不支持引用提取 → 按安全原则整游戏保留
			continue
		}
		if kv, ok := ast.(*kirikiri.Ast); ok && kv.DynamicVoice {
			dynVoice = true
		}
		allRefs = append(allRefs, refs...)
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
	a, engineID, err := adapterFor(root)
	if err != nil {
		return err
	}
	pool, err := poolFor(root, engineID)
	if err != nil {
		return err
	}
	data, rerr := readScriptMaybeFixed(script, root)
	if rerr != nil {
		return rerr
	}
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
		emote := ""
		if ev.HasEmote {
			emote = " E-mote(不可复刻)"
		}
		fmt.Printf("  #%-2d %-16s %-10s 图层%d 台词%d(语音%d) 转场%d 资产%d 未识别%d%s\n",
			ev.Ord, ev.EventID, ev.Kind, len(ev.Layers), len(ev.Texts), nv,
			len(ev.Transitions), len(ev.Assets), len(ev.Unrecognized), emote)
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

// cmdReplay 回放。默认按 IR 时间轴打印演出过程（§7.5.5 方式 A）；
// 给了 --render / --gif 就把 IR 合成成真正的画面（§0.5 S3「可播放演出」）——
// 素材解码全走 internal/decode，掩码 JPEG 不需要外部工具。
//
// 用法：ucgr replay <ir.json> [--text]
//
//	[--assets 素材根] [--render 输出目录] [--gif out.gif]
//	[--at 0,2100,12000] [--frames N] [--size 1600x1200] [--event N]
func cmdReplay(args []string) error {
	var (
		irPath                             string
		assets, renderDir, gifPath, atSpec string
		sizeSpec                           string
		evSel, frames, delayCs             int
		forceText                          bool
		err                                error
	)
	frames = 12
	delayCs = 40
	for i := 0; i < len(args); i++ {
		take := func() (string, error) {
			if i+1 >= len(args) {
				return "", fmt.Errorf("%s 缺少取值", args[i])
			}
			i++
			return args[i], nil
		}
		switch a := args[i]; a {
		case "--assets":
			assets, err = take()
		case "--render":
			renderDir, err = take()
		case "--gif":
			gifPath, err = take()
		case "--at":
			atSpec, err = take()
		case "--size":
			sizeSpec, err = take()
		case "--frames":
			var v string
			if v, err = take(); err == nil {
				frames, err = strconv.Atoi(v)
			}
		case "--delay":
			var v string
			if v, err = take(); err == nil {
				delayCs, err = strconv.Atoi(v)
			}
		case "--event":
			var v string
			if v, err = take(); err == nil {
				evSel, err = strconv.Atoi(v)
			}
		case "--text":
			forceText = true
		default:
			if irPath == "" {
				irPath = a
			} else {
				err = fmt.Errorf("多余参数: %s", a)
			}
		}
		if err != nil {
			return err
		}
	}
	if irPath == "" {
		return fmt.Errorf("用法: ucgr replay <ir.json> [--assets 素材根] [--render 目录] [--gif out.gif] [--at ms,ms,…]")
	}
	b, err := os.ReadFile(irPath)
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

	// 合成模式：只从 IR + 素材出画面。
	if renderDir != "" || gifPath != "" {
		if assets == "" {
			return fmt.Errorf("合成画面需要 --assets <素材根>（可用 S2 裁剪产物目录）")
		}
		if len(doc.Events) == 0 {
			return fmt.Errorf("IR 里没有事件")
		}
		idx := evSel - 1
		if evSel == 0 {
			idx = 0
		}
		if idx < 0 || idx >= len(doc.Events) {
			return fmt.Errorf("--event %d 超出范围（共 %d 个事件）", evSel, len(doc.Events))
		}
		ev := &doc.Events[idx]
		load := replay.DirLoader(assets)
		var times []int64
		if atSpec != "" {
			if times, err = parseTimes(atSpec); err != nil {
				return err
			}
		} else {
			times = replay.SampleTimes(ev, frames)
		}
		w, h := parseSize(sizeSpec)
		if w <= 0 || h <= 0 {
			if doc.CanvasW > 0 && doc.CanvasH > 0 {
				w, h = doc.CanvasW, doc.CanvasH
			}
		}
		fmt.Printf("\n=== 事件 %s（%s）合成：%d 帧，素材根 %s ===\n", ev.EventID, ev.Kind, len(times), assets)
		fmt.Println("  图层（按 z 升序，z 小者在下）：")
		layers := append([]ir.Layer(nil), ev.Layers...)
		sort.SliceStable(layers, func(i, j int) bool { return layers[i].Z < layers[j].Z })
		for _, l := range layers {
			name := "-"
			if l.Asset != nil {
				name = l.Asset.OrigPath
			}
			fmt.Printf("    z=%-3d %-9s %-10s %s\n", l.Z, l.LayerID, l.Kind, name)
		}
		canvas := make([]*image.RGBA, 0, len(times))
		var missing []string
		for _, t := range times {
			img, drawn, err := replay.Compose(replay.StatesAt(ev, t), load, w, h)
			if err != nil {
				missing = append(missing, err.Error())
				continue
			}
			fmt.Printf("  t=%6dms  可见 %d 层  %s\n", t, len(drawn), strings.Join(drawn, " + "))
			canvas = append(canvas, img)
			if renderDir != "" {
				path := filepath.Join(renderDir, fmt.Sprintf("frame_t%06d.png", t))
				if err := os.MkdirAll(renderDir, 0o755); err != nil {
					return err
				}
				f, err := os.Create(path)
				if err != nil {
					return err
				}
				err = png.Encode(f, img)
				f.Close()
				if err != nil {
					return err
				}
			}
		}
		if len(missing) > 0 {
			fmt.Printf("⚠️ %d 帧因素材缺失跳过：%s\n", len(missing), strings.Join(missing, "; "))
		}
		if renderDir != "" {
			fmt.Printf("PNG 帧已写入 %s（%d 张）\n", renderDir, len(canvas))
		}
		if gifPath != "" {
			if err := replay.WriteGIF(canvas, gifPath, delayCs); err != nil {
				return err
			}
			fmt.Printf("动图已写入 %s（%d 帧，%d cs/帧）\n", gifPath, len(canvas), delayCs)
		}
		if forceText {
			replayDump(&doc)
		}
		return nil
	}

	replayDump(&doc)
	return nil
}

// cmdPlay 按原作演出播放一个事件。
//
// 与 replay 的「按时间戳采样合成」不同，play 是顺序解释器：
// 图层双缓冲（fore/back）+ [trans] 过渡进度 + [quake] 抖动 +
// 对话框文本（[台詞]/[l]/[r]/[cm]），台词之间的点击点按阅读时长自动推进。
//
// 用法：
//
//	ucgr play <ir.json> --assets <素材根> [--event N] [--speed X] [--char-ms N]
//	          [--no-wait] [--shot 目录 [--at ms,ms,…]] [--size WxH]
//	          [--font 字体名] [--font-file otf]
func cmdPlay(args []string) error {
	var (
		irPath, assets, shotDir, atSpec, sizeSpec, fontName, fontFile string
		scriptPath, label                                             string
		evSel, charMs                                                 int
		chapterPos                                                    int
		hasChapter                                                    bool
		speed                                                         float64
		noWait                                                        bool
		err                                                           error
	)
	speed = 1
	for i := 0; i < len(args); i++ {
		take := func() (string, error) {
			if i+1 >= len(args) {
				return "", fmt.Errorf("%s 缺少取值", args[i])
			}
			i++
			return args[i], nil
		}
		switch a := args[i]; a {
		case "--assets":
			assets, err = take()
		case "--event":
			var v string
			if v, err = take(); err == nil {
				evSel, err = strconv.Atoi(v)
			}
		case "--speed":
			var v string
			if v, err = take(); err == nil {
				speed, err = strconv.ParseFloat(v, 64)
			}
		case "--char-ms":
			var v string
			if v, err = take(); err == nil {
				charMs, err = strconv.Atoi(v)
			}
		case "--no-wait":
			noWait = true
		case "--script":
			scriptPath, err = take()
		case "--label":
			label, err = take()
		case "--chapter":
			var v string
			if v, err = take(); err == nil {
				chapterPos, err = strconv.Atoi(v)
				if err == nil {
					hasChapter = true
				}
			}
		case "--shot":
			shotDir, err = take()
		case "--at":
			atSpec, err = take()
		case "--size":
			sizeSpec, err = take()
		case "--font":
			fontName, err = take()
		case "--font-file":
			fontFile, err = take()
		default:
			if irPath == "" {
				irPath = a
			} else {
				err = fmt.Errorf("多余参数: %s", a)
			}
		}
		if err != nil {
			return err
		}
	}
	if irPath == "" && scriptPath == "" {
		return fmt.Errorf("用法: ucgr play <ir.json> --assets <素材根> [--event N] [--shot 目录 --at ms,…]\n" +
			"      或: ucgr play --script <脚本.ks> [--label cg01] [--assets 素材根]\n" +
			"      或: ucgr play --script <脚本.ks> --chapter <标签下标> [--assets 素材根]")
	}
	if assets == "" && scriptPath != "" {
		assets = gameRootOf(scriptPath)
	}
	if assets == "" {
		return fmt.Errorf("play 需要 --assets <素材根>（解包目录或 S2 裁剪产物）")
	}

	var (
		ev      *ir.Event
		gameID  string
		evLabel string
	)
	if scriptPath != "" {
		// 按原作自己的分割单位播放：章节（*labelNN|显示名）、标签段（*cgNN）或整脚本。
		var (
			e   *ir.Event
			err error
		)
		if hasChapter {
			e, err = kirikiri.BuildShowChapterFile(scriptPath, chapterPos)
		} else {
			e, err = kirikiri.BuildShowSegment(scriptPath, label)
		}
		if err != nil {
			return err
		}
		if len(e.Show) == 0 {
			return fmt.Errorf("脚本 %s 的标签段 %q 里没有可演的台本", filepath.Base(scriptPath), label)
		}
		ev = e
		gameID = filepath.Base(gameRootOf(scriptPath))
		switch {
		case hasChapter:
			evLabel = fmt.Sprintf("章节 %s（%s）", e.Title, e.EventID)
		case strings.TrimSpace(label) != "":
			evLabel = "标签 " + strings.TrimPrefix(label, "*")
		default:
			evLabel = "整脚本 " + filepath.Base(scriptPath)
		}
	} else {
		b, rerr := os.ReadFile(irPath)
		if rerr != nil {
			return rerr
		}
		var doc ir.IR
		if uerr := json.Unmarshal(b, &doc); uerr != nil {
			return fmt.Errorf("IR 解析失败: %w", uerr)
		}
		if len(doc.Events) == 0 {
			return fmt.Errorf("IR 里没有事件")
		}
		idx := evSel - 1
		if evSel == 0 {
			idx = 0
		}
		if idx < 0 || idx >= len(doc.Events) {
			return fmt.Errorf("--event %d 超出范围（共 %d 个事件）", evSel, len(doc.Events))
		}
		ev = &doc.Events[idx]
		if len(ev.Show) == 0 {
			return fmt.Errorf("事件 %s 没有演出台本（show），无法播放", ev.EventID)
		}
		gameID = doc.GameID
		evLabel = fmt.Sprintf("#%d %s（%s）", idx+1, ev.EventID, ev.Kind)
	}
	w, h := parseSize(sizeSpec)
	if w <= 0 || h <= 0 {
		w, h = 1600, 1200 // Config.tjs：scWidth/scHeight
	}

	load := player.Cached(playLoader(assets))
	opt := player.DefaultOptions()
	if speed > 0 {
		opt.Speed = speed
	}
	if charMs > 0 {
		opt.CharMs = float64(charMs)
	}
	opt.SkipWaits = noWait

	// 字体：本作自带思源宋体 SC（OTF/CFF，且被单字节 XOR 混淆）。
	// RegisterFontFile 会还原混淆并私有注册到 GDI，返回可用的家族名。
	face := fontName
	ff := fontFile
	if ff == "" {
		ff = filepath.Join(assets, "raw", "data", "SourceHanSerifSC-Regular.otf")
	}
	if n, ok := win32.RegisterFontFile(ff); ok {
		if face == "" {
			face = n
		}
		fmt.Printf("字体：私有注册 %s（家族 %s，使用 %s）\n", filepath.Base(ff), n, face)
	} else {
		if face == "" {
			face = "SimSun"
		}
		fmt.Printf("字体：%s 加载失败，使用系统字体 %s\n", filepath.Base(ff), face)
	}
	td := win32.NewTextDrawer(win32.DefaultTextStyle(face, opt.FontSize))
	defer td.Free()

	fmt.Printf("游戏 %s · 事件 %s · 台本 %d 条 · 画布 %dx%d · 字号 %.0fpx · 速度 ×%.2f\n",
		gameID, evLabel, len(ev.Show), w, h, opt.FontSize, opt.Speed)

	p := player.New(ev, opt)

	// 离线出图模式：按 16ms 步进到目标时刻，出「纯图层」与「含文字」两张 PNG。
	if atSpec != "" {
		times, terr := parseTimes(atSpec)
		if terr != nil {
			return terr
		}
		if shotDir == "" {
			shotDir = "."
		}
		if err := os.MkdirAll(shotDir, 0o755); err != nil {
			return err
		}
		var last int64
		// 先跑一次零步进：把 t=0 的台本指令（背景/对话框几何/首行台词）执行掉，
		// 否则 t=0 的首帧会是空画布。
		p.Advance(0, player.Input{})
		for _, t := range times {
			if t < last {
				t = last
			}
			for p.Now() < t {
				if p.Done() && p.Frame().TransProgress == 0 {
					break // 台本已演完且末段转场已走完，画面定格
				}
				p.Advance(16, player.Input{})
			}
			last = t
			f := p.Frame()
			rgba, drawn, cerr := renderFrame(p, load, w, h)
			if cerr != nil {
				fmt.Printf("⚠️ t=%dms 合成失败：%v\n", t, cerr)
				continue
			}
			plain := filepath.Join(shotDir, fmt.Sprintf("play_t%06d.png", t))
			if err := writePNG(plain, rgba); err != nil {
				return err
			}
			withText := filepath.Join(shotDir, fmt.Sprintf("play_t%06d_text.png", t))
			if err := writeTextPNG(withText, td, f, rgba, w, h); err != nil {
				return err
			}
			fmt.Printf("  t=%6dms  层 %d（%s）· 文本块 %d · 等待中 %v → %s\n",
				t, len(drawn), strings.Join(drawn, " + "), len(f.Texts), f.Waiting, filepath.Base(withText))
			for _, box := range f.Texts {
				fmt.Printf("      [层 %s] 可见 %v 几何(%.0f,%.0f,%.0f,%.0f) hasGeom=%v frame=%q 底色(%d,%d,%d)@%d 边距(%.0f,%.0f,%.0f,%.0f) 行 %d\n",
					box.Layer, box.Visible, box.X, box.Y, box.W, box.H, box.HasGeom, box.Frame,
					box.FrameColor[0], box.FrameColor[1], box.FrameColor[2], box.FrameOpacity,
					box.MarginL, box.MarginT, box.MarginR, box.MarginB, len(box.Lines))
			}
		}
		fmt.Printf("PNG 已写入 %s\n", shotDir)
		fmt.Printf("播放器时刻 %dms · 台本读完 %v\n", p.Now(), p.Done())
		return nil
	}

	// 音频：按 KAG 语义播放 BGM/SE（Media Foundation 解码 + winmm waveOut 混音）。
	if sys, aerr := audio.NewSystem(assets); aerr == nil {
		defer sys.Close()
		p.Audio = sys.Handle
	} else {
		fmt.Printf("（音频未启用：%v）\n", aerr)
	}

	// 窗口实时播放。
	paused, spd := false, speed
	if spd <= 0 {
		spd = 1
	}
	opt.Speed = 1 // 倍速在下面按 dt 计算，避免与 Advance 内部乘 Speed 重复
	shotN := 0
	title := fmt.Sprintf("ucgr play — %s %s", gameID, ev.EventID)
	if ev.HasEmote {
		title += " [E-mote 不可复刻]"
	}
	return win32.Run(win32.Options{
		Title:  title,
		Width:  w,
		Height: h,
	}, func(c *win32.Canvas, dtMs int64, in win32.Input) {
		if in.TogglePause {
			paused = !paused
		}
		if in.ToggleSpeed {
			if spd >= 4 {
				spd = 1
			} else {
				spd *= 2
			}
		}
		if in.ToggleSkip {
			noWait = !noWait
			p.SetSkip(noWait)
		}
		if !paused {
			if in.Advance {
				p.Advance(0, player.Input{Advance: true})
			}
			p.Advance(int64(float64(dtMs)*spd), player.Input{})
		}
		f := p.Frame()
		rgba, _, cerr := renderFrame(p, load, w, h)
		if cerr != nil {
			rgba = image.NewRGBA(image.Rect(0, 0, w, h))
		}
		c.BlitRGBA(rgba)
		td.DrawBoxes(c.DC(), f.Texts, 1)
		if in.RequestShot {
			if shotDir == "" {
				shotDir = "."
			}
			_ = os.MkdirAll(shotDir, 0o755)
			path := filepath.Join(shotDir, fmt.Sprintf("shot_%04d_t%06d.png", shotN, f.Now))
			c.ForceOpaque()
			if err := writePNG(path, c.Image()); err == nil {
				fmt.Printf("截图 → %s\n", path)
			}
			shotN++
		}
	})
}

// renderFrame 把玩家当前帧合成成 RGBA（图层 + 消息层框图），文字另由 GDI 画。
// 实现已提到 internal/player.RenderFrame，CLI 与 GUI 共用同一份。
func renderFrame(p *player.Player, load player.Loader, w, h int) (*image.RGBA, []string, error) {
	return player.RenderFrame(p, load, w, h)
}

// writePNG 存一张 RGBA 到 PNG。
func writePNG(path string, img image.Image) error {
	fh, err := os.Create(path)
	if err != nil {
		return err
	}
	defer fh.Close()
	return png.Encode(fh, img)
}

// writeTextPNG 先把 RGBA 贴进 GDI 画布、再画文字，最后存 PNG。
func writeTextPNG(path string, td *win32.TextDrawer, f player.Frame, rgba *image.RGBA, w, h int) error {
	c, err := win32.NewCanvas(w, h)
	if err != nil {
		return err
	}
	defer c.Free()
	c.BlitRGBA(rgba)
	td.DrawBoxes(c.DC(), f.Texts, 1)
	c.ForceOpaque() // GDI 不维护 alpha，存图前补成不透明
	return writePNG(path, c.Image())
}

// playLoader 在素材根下解析素材名（实现见 internal/replay.GameLoader，GUI 共用同一份）。
func playLoader(root string) player.Loader {
	return player.Loader(replay.GameLoader(root))
}

// parseTimes 解析 --at 的毫秒列表（逗号或空格分隔）。
func parseTimes(spec string) ([]int64, error) {
	fields := strings.FieldsFunc(spec, func(r rune) bool { return r == ',' || r == ' ' || r == ';' })
	if len(fields) == 0 {
		return nil, fmt.Errorf("--at 为空")
	}
	out := make([]int64, 0, len(fields))
	for _, f := range fields {
		v, err := strconv.ParseInt(f, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("--at 里的 %q 不是毫秒数", f)
		}
		out = append(out, v)
	}
	return out, nil
}

// parseSize 解析 --size WxH；空或非法返回 0,0（交给 Compose 按背景自动定）。
func parseSize(spec string) (int, int) {
	if spec == "" {
		return 0, 0
	}
	parts := strings.FieldsFunc(strings.ToLower(spec), func(r rune) bool { return r == 'x' || r == '*' || r == ',' })
	if len(parts) != 2 {
		return 0, 0
	}
	w, err1 := strconv.Atoi(parts[0])
	h, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || w <= 0 || h <= 0 {
		return 0, 0
	}
	return w, h
}

// replayDump 文本回放：图层清单 + 时间轴 + 转场/分支。
func replayDump(doc *ir.IR) {
	for _, ev := range doc.Events {
		emote := ""
		if ev.HasEmote {
			emote = "  ⚠ E-mote 不可复刻"
		}
		fmt.Printf("\n=== 事件 %s（%s）图层 %d 台词 %d%s ===\n",
			ev.EventID, ev.Kind, len(ev.Layers), len(ev.Texts), emote)
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
}

// cmdDecode 按需解码资产（当前支持 KiriKiri TLG5）为 PNG，
// 让回放在没有原播放器时也能「看」静态合成图（P1-3 解码层的最小集）。
// 用法：ucgr unpack <归档> [--out 目录] [--key 口令] [--limit N]
//
// 目前支持 CatSystem2 的 KIF 归档（.int/.kif）。不给 --out 时只列条目；给了就解包写盘。
// 索引与数据解密都不需要游戏主程序口令，只有**名字的大小写**需要口令才能精确还原。
func cmdUnpack(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("用法: ucgr unpack <归档> [--out 目录] [--key 口令] [--limit N]")
	}
	in := args[0]
	outDir, key := "", ""
	limit := 0
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--out":
			if i+1 >= len(args) {
				return fmt.Errorf("--out 缺少参数")
			}
			outDir = args[i+1]
			i++
		case "--key":
			if i+1 >= len(args) {
				return fmt.Errorf("--key 缺少参数")
			}
			key = args[i+1]
			i++
		case "--limit":
			if i+1 >= len(args) {
				return fmt.Errorf("--limit 缺少参数")
			}
			if _, err := fmt.Sscanf(args[i+1], "%d", &limit); err != nil {
				return fmt.Errorf("--limit 需要整数: %v", err)
			}
			i++
		default:
			return fmt.Errorf("未知参数: %s", args[i])
		}
	}

	arc, err := kif.Open(in)
	if err != nil {
		return err
	}
	defer arc.Close()
	if key != "" {
		arc.SetPassphrase(key)
		fmt.Printf("已按口令 %q 走精确解名\n", key)
	}
	fmt.Printf("归档: %s（KIF，条目 %d，越界跳过 %d）\n", in, len(arc.Entries), arc.Skipped)
	fmt.Printf("名字可信度: %s\n", kindSummary(arc.KindStats()))

	if outDir == "" {
		for i, e := range arc.Entries {
			if limit > 0 && i >= limit {
				fmt.Printf("  …（其余 %d 条省略，--limit 0 取消限制）\n", len(arc.Entries)-limit)
				break
			}
			fmt.Printf("  %-44s %10d  %s\n", e.Name, e.Size, e.NameKind)
		}
		return nil
	}

	var total int64
	for _, e := range arc.Entries {
		data, err := arc.Read(e)
		if err != nil {
			return fmt.Errorf("读取 %s 失败: %w", e.Name, err)
		}
		dest := filepath.Join(outDir, filepath.FromSlash(e.Name))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dest, data, 0o644); err != nil {
			return err
		}
		total += int64(len(data))
	}
	fmt.Printf("已解包 %d 个文件 / %s → %s\n", len(arc.Entries), fmtBytes(total), outDir)
	return nil
}

// kindSummary 把名字可信度统计压成一行（固定顺序，便于跨实现比对）。
func kindSummary(stats map[string]int) string {
	order := []string{kif.NameExactKind, kif.NameDominantKind, kif.NameAmbiguousKind, kif.NameUnknownKind}
	parts := make([]string, 0, len(order))
	for _, k := range order {
		if v := stats[k]; v > 0 {
			parts = append(parts, fmt.Sprintf("%s=%d", k, v))
		}
	}
	return strings.Join(parts, " ")
}

// fmtBytes 人类可读体积。
func fmtBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	units := []string{"KiB", "MiB", "GiB", "TiB"}
	v := float64(n)
	i := -1
	for v >= unit && i < len(units)-1 {
		v /= unit
		i++
	}
	return fmt.Sprintf("%.2f %s", v, units[i])
}

// 用法：ucgr decode <图片文件> [--out out.png] [--raw out.bin]
// --raw 写出裸 BGRA 字节，供与参考实现逐字节比对。
// JPEG 若是「整文件单字节 XOR 掩码」变种（KiriKiri 系常见），自动还原后再解，见 internal/decode/jpeg.go。
func cmdDecode(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("用法: ucgr decode <图片文件> [--out out.png] [--raw out.bin]")
	}
	in := args[0]
	var out, raw string
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--out":
			if i+1 >= len(args) {
				return fmt.Errorf("--out 缺少参数")
			}
			out, i = args[i+1], i+1
		case "--raw":
			if i+1 >= len(args) {
				return fmt.Errorf("--raw 缺少参数")
			}
			raw, i = args[i+1], i+1
		default:
			return fmt.Errorf("未知参数 %q", args[i])
		}
	}
	data, err := os.ReadFile(in)
	if err != nil {
		return err
	}
	img, err := decode.Decode(data)
	if err != nil {
		return fmt.Errorf("%s: %w", in, err)
	}
	if img.Mask != 0 {
		fmt.Printf("检测到整文件 XOR 掩码 0x%02X，已还原后再解码\n", img.Mask)
	}
	fmt.Printf("解码 %s → %dx%d（%d 通道，字节序 BGRA）\n", in, img.Width, img.Height, img.Colors)
	if raw != "" {
		if err := os.WriteFile(raw, img.Pixels, 0o644); err != nil {
			return err
		}
		fmt.Printf("已写出裸像素: %s（%d 字节）\n", raw, len(img.Pixels))
	}
	if out == "" {
		out = strings.TrimSuffix(in, filepath.Ext(in)) + ".png"
	}
	nrgba := imageToNRGBA(img)
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	if err := png.Encode(f, nrgba); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	fmt.Printf("已写出 PNG: %s\n", out)
	return nil
}

// imageToNRGBA 把解码结果（B、G、R[、A] 字节序）转成 PNG 用的非预乘 NRGBA。
// 24 位图（Colors==3）每像素 3 字节，alpha 一律补 255；不补的话 24 位图会被
// 按 4 字节错位读出（越界、颜色全乱），所以这里必须分流。
func imageToNRGBA(img *decode.Image) *image.NRGBA {
	ps := 4
	if img.Colors == 3 {
		ps = 3
	}
	n := image.NewNRGBA(image.Rect(0, 0, img.Width, img.Height))
	for i := 0; i < img.Width*img.Height && (i+1)*ps <= len(img.Pixels); i++ {
		n.Pix[i*4+0] = img.Pixels[i*ps+2] // B → R
		n.Pix[i*4+1] = img.Pixels[i*ps+1] // G
		n.Pix[i*4+2] = img.Pixels[i*ps+0] // R → B
		n.Pix[i*4+3] = 0xFF
		if ps == 4 {
			n.Pix[i*4+3] = img.Pixels[i*ps+3]
		}
	}
	return n
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

// engineOverride 全局 --engine <id>：跳过自动识别，直接指定引擎。
var engineOverride string

// stripEngineFlag 从参数里摘出全局 --engine <id>
// （识别不准或存在歧义时可显式指定，不用改代码）。
func stripEngineFlag(args []string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if args[i] == "--engine" && i+1 < len(args) {
			engineOverride = args[i+1]
			i++
			continue
		}
		out = append(out, args[i])
	}
	return out
}

// adapterFor 取引擎适配器：有 --engine 时按其指定，否则按路径自动识别
// （走注册表，新增引擎零改动）。
func adapterFor(path string) (adapter.EngineAdapter, string, error) {
	if engineOverride != "" {
		a, ok := adapter.Get(engineOverride)
		if !ok {
			return nil, "", fmt.Errorf("未知引擎 %q（已注册：%s）", engineOverride, strings.Join(adapter.IDs(), ", "))
		}
		return a, engineOverride, nil
	}
	id, err := adapter.IdentifyPath(path)
	if err != nil {
		return nil, "", fmt.Errorf("%w（可用 --engine <id> 显式指定；已注册：%s）", err, strings.Join(adapter.IDs(), ", "))
	}
	a, ok := adapter.Get(id)
	if !ok {
		return nil, "", fmt.Errorf("引擎 %s 已识别但未注册", id)
	}
	return a, id, nil
}

// astRefs 从引擎专有 AST 提取资源引用。
// 各适配器的 Ast 都实现 ExtractRefs() []string；未实现返回 ok=false，
// 调用方须按安全原则整游戏保留。
// poolFor 建素材池；CatSystem2 会把游戏根/解包根旁边的音频目录一起挂进来。
// 语音（pcm）通常在 原始归档解包/../音频 下，不挂进来就会全部变成未命中。
func poolFor(root, engineID string) (*prune.Pool, error) {
	var extras []string
	if engineID == "catsystem2" {
		seen := map[string]bool{}
		for _, name := range []string{"音频", "语音", "voice", "sound", "audio"} {
			for _, base := range []string{root, filepath.Dir(root)} {
				p := filepath.Join(base, name)
				if p == root || seen[p] {
					continue
				}
				if st, err := os.Stat(p); err == nil && st.IsDir() {
					seen[p] = true
					extras = append(extras, p)
				}
			}
		}
	}
	return prune.BuildPoolWithExtra(root, extras...)
}

func astRefs(ast adapter.ScriptAst) ([]string, bool) {
	if r, ok := ast.(interface{ ExtractRefs() []string }); ok {
		return r.ExtractRefs(), true
	}
	return nil, false
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
