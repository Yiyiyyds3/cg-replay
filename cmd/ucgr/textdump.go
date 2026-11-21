// textdump.go —— 剧本文本导出：把一款游戏的可读台词逐脚本导成 txt。
//
// 目的：文本验收。CG/事件过滤器（validate.go）管「画面在不在」，这里管
// 「台词能不能拿出来」——每脚本一个 txt（UTF-8 无 BOM），另落一份
// _textdump.json 汇总（脚本数/行数/字数），reconcile 流水线直接读。
//
// 口径从简（做减法）：KiriKiri 提取「标签之外的可读文本」，RPG Maker MV/MZ
// 提取对话（401，续行拼接）与选项（102），其它引擎明确报不支持而不是
// 静默给空结果。导出布局：kirikiri 保留相对目录结构，rmmz 平铺 data_ 前缀
// （MapXXX.json 和 CommonEvents.json 都在同一个 data 目录里，保留目录只会
// 多一层没有信息量的 www/data/）。
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"ucgr/internal/adapter"
	"ucgr/internal/adapter/kirikiri"
	"ucgr/internal/adapter/rmmz"
	"ucgr/internal/library"
)

// TextDumpSchema 汇总清单能力版本，字段增删时递增。
const TextDumpSchema = 1

// textScriptStat 单个脚本的导出统计。
type textScriptStat struct {
	Path  string `json:"path"`
	Lines int    `json:"lines"`
	Chars int    `json:"chars"`
}

// textDumpReport 一次导出的汇总（写 <out>/_textdump.json）。
type textDumpReport struct {
	Schema       int              `json:"schema"`
	Game         string           `json:"game,omitempty"`
	EngineID     string           `json:"engine_id,omitempty"`
	Root         string           `json:"root"`
	GeneratedAt  string           `json:"generated_at"`
	Scripts      []textScriptStat `json:"scripts"`
	TotalScripts int              `json:"total_scripts"`
	TotalLines   int              `json:"total_lines"`
	TotalChars   int              `json:"total_chars"`
	// Notes 导出口径与跳过原因（比如引擎暂不支持），读报告的人不用猜。
	Notes []string `json:"notes,omitempty"`
}

// kagTagRe KAG 标签（[...]），文本提取时整段删掉。
var kagTagRe = regexp.MustCompile(`\[[^\]]*\]`)

// ksPlainLines 从 .ks 文本里抽可读行。
//
// 三类行不算台词：; 注释、@ 宏（wait/bg/jump…）、* 标签（跳转锚点）；
// 行内夹着的 [标签] 整段剥掉，剩下的就是显示文本。KAG 的显示文本就是
// 标签之间的裸文本，够验收用；宏/嵌入 JS（[eval]）天然被过滤掉。
func ksPlainLines(src string) []string {
	var lines []string
	for _, raw := range strings.Split(src, "\n") {
		line := strings.TrimRight(raw, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, ";") ||
			strings.HasPrefix(trimmed, "@") || strings.HasPrefix(trimmed, "*") {
			continue
		}
		text := strings.TrimSpace(kagTagRe.ReplaceAllString(line, ""))
		if text != "" {
			lines = append(lines, text)
		}
	}
	return lines
}

// rmmzCmdJSON 一条事件指令（只关心 code 与参数）。类型名避开测试里的
// rmmzCmd 造数函数——那边是测试专用的，这边是解析用的，各用各的名字。
type rmmzCmdJSON struct {
	Code       int               `json:"code"`
	Parameters []json.RawMessage `json:"parameters"`
}

// textOfRaw 把 JSON 字符串参数解成 Go string（失败给空）。
func textOfRaw(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return s
}

// pushLines 把指令序列转成台词行：401 连续条拼接成一句（MV 把长台词拆成
// 多条 401），102 的每个选项一行，其余指令是动作/等待，攒着的续行就地断句。
func pushLines(list []*rmmzCmdJSON, lines *[]string) {
	var pending []string
	flush := func() {
		if len(pending) > 0 {
			s := strings.Join(pending, "")
			if strings.TrimSpace(s) != "" {
				*lines = append(*lines, strings.TrimSpace(s))
			}
			pending = nil
		}
	}
	for _, cmd := range list {
		if cmd == nil {
			continue
		}
		switch cmd.Code {
		case 401:
			if len(cmd.Parameters) > 0 {
				pending = append(pending, textOfRaw(cmd.Parameters[0]))
			}
		case 102:
			flush()
			if len(cmd.Parameters) > 0 {
				var opts []string
				if err := json.Unmarshal(cmd.Parameters[0], &opts); err == nil {
					for _, o := range opts {
						if strings.TrimSpace(o) != "" {
							*lines = append(*lines, strings.TrimSpace(o))
						}
					}
				}
			}
		default:
			flush()
		}
	}
	flush()
}

// extractRmmzText 从 data/*.json 抽台词。
//
// MapXXX.json 是对象 {events:[{pages:[{list:[…]}]}]}，CommonEvents.json 是
// 数组 [{list:[…]}]——两种顶层都吃，先试对象再试数组。
func extractRmmzText(content []byte) ([]string, error) {
	var lines []string
	var obj struct {
		Events []*struct {
			Pages []*struct {
				List []*rmmzCmdJSON `json:"list"`
			} `json:"pages"`
		} `json:"events"`
	}
	if err := json.Unmarshal(content, &obj); err == nil && len(obj.Events) > 0 {
		for _, ev := range obj.Events {
			if ev == nil {
				continue
			}
			for _, page := range ev.Pages {
				if page != nil {
					pushLines(page.List, &lines)
				}
			}
		}
		return lines, nil
	}
	var arr []*struct {
		List []*rmmzCmdJSON `json:"list"`
	}
	if err := json.Unmarshal(content, &arr); err == nil {
		for _, ev := range arr {
			if ev != nil {
				pushLines(ev.List, &lines)
			}
		}
		return lines, nil
	}
	return nil, fmt.Errorf("不是 RPG Maker data JSON（对象与数组两种顶层都解析失败）")
}

// txtNameFor 导出 txt 名。
//
// kirikiri 保留相对目录结构（脚本多、按目录浏览有意义）；rmmz 平铺
// data_ 前缀（www/data 一层没有信息量）。路径里的非法分隔符统一换 __。
func txtNameFor(engine, rel string) string {
	// 不做 ToLower：导出名要跟原文件名一致（data_Map001.txt 不是 data_map001.txt），
	// 验收脚本按原大小写对表。
	n := filepath.ToSlash(rel)
	n = strings.TrimSuffix(n, filepath.Ext(n))
	if strings.EqualFold(engine, "rmmz") {
		n = strings.TrimPrefix(n, "www/")
		n = strings.ReplaceAll(n, "/", "_")
		return n + ".txt"
	}
	return n + ".txt"
}

// cmdTextDump 入口：ucgr textdump <解包目录> --out <导出目录> [--engine <id>]
func cmdTextDump(args []string) error {
	var root, out, engineFlag string
	for i := 0; i < len(args); i++ {
		switch a := args[i]; a {
		case "--out":
			if i+1 >= len(args) {
				return fmt.Errorf("--out 缺少取值")
			}
			i++
			out = args[i]
		case "--engine":
			if i+1 >= len(args) {
				return fmt.Errorf("--engine 缺少取值")
			}
			i++
			engineFlag = args[i]
		default:
			if root == "" {
				root = a
			} else {
				return fmt.Errorf("多余参数: %s", a)
			}
		}
	}
	if root == "" || out == "" {
		return fmt.Errorf("用法: ucgr textdump <解包目录> --out <导出目录> [--engine <id>]")
	}

	// --engine <id>（全局参数）指定引擎时按其默认约定扫；否则自动探测。
	engineID := strings.TrimSpace(engineFlag)
	var conv adapter.Convention
	var err error
	if engineID != "" {
		var ok bool
		conv, ok = adapter.ConventionByEngine(engineID)
		if !ok {
			conv = adapter.Convention{EngineID: engineID}
		}
	} else {
		var probe string
		probe, conv, err = library.DetectConvention(root)
		if err != nil {
			// 空目录 / 还没解包 / 认不出引擎，对 textdump 都不是错误：
			// 这是验收工具，如实说没东西可导就够，退出码给 0。
			fmt.Printf("未发现剧本：%s\n", err)
			return nil
		}
		engineID = probe
	}

	type scriptFile struct{ rel, abs string }
	var scripts []scriptFile
	notes := []string{}

	switch strings.ToLower(strings.TrimSpace(engineID)) {
	case "kirikiri":
		// 剧本目录候选：约定写的最优先，其余按常见解包布局兜底。只认一个路径会让
		// 「明明有 .ks」的目录导出空文本（实测：整理版把 scenario 放在根下的布局）。
		var cands []string
		if d := strings.TrimSpace(conv.ScenarioDir); d != "" {
			cands = append(cands, d)
		}
		cands = append(cands,
			library.ScenarioDir(root),
			filepath.Join("raw", "data", "scenario"),
			filepath.Join("data", "scenario"),
			filepath.Join("data", "script"),
			"scenario",
		)
		seen := map[string]bool{}
		var dirs []string
		for _, d := range cands {
			key := strings.ToLower(filepath.Clean(d))
			if seen[key] {
				continue
			}
			seen[key] = true
			if st, serr := os.Stat(filepath.Clean(d)); serr == nil && st.IsDir() {
				dirs = append(dirs, d)
			}
		}
		dirs = append(dirs, root) // 最后的兜底：整棵树里找 .ks
		for _, dir := range dirs {
			found := 0
			_ = filepath.Walk(dir, func(p string, info os.FileInfo, werr error) error {
				if werr != nil || info == nil || info.IsDir() {
					return nil
				}
				if !strings.EqualFold(filepath.Ext(p), ".ks") {
					return nil
				}
				rel, rerr := filepath.Rel(root, p)
				if rerr != nil {
					return nil
				}
				scripts = append(scripts, scriptFile{rel: rel, abs: p})
				found++
				return nil
			})
			if found > 0 {
				break
			}
			scripts = nil // 这层没脚本就换下一个候选，别留下半截结果
		}
		if len(scripts) == 0 {
			notes = append(notes, "剧本目录里没有 .ks 文件，无文本可导出")
		}
	case "rmmz":
		dataDir, ok := rmmz.FindDataDir(root)
		if !ok {
			notes = append(notes, "未找到 www/data 目录，无文本可导出")
		} else {
			// FindDataDir 返回的是相对 root 的路径（www/data），拼回绝对路径再读盘，
			// 否则 os.ReadDir 会相对进程 CWD 找，换个目录跑就一个剧本都找不到。
			absData := filepath.Join(root, filepath.FromSlash(dataDir))
			entries, _ := os.ReadDir(absData)
			names := make([]string, 0, len(entries))
			for _, e := range entries {
				n := e.Name()
				if e.IsDir() || !strings.EqualFold(filepath.Ext(n), ".json") {
					continue
				}
				lower := strings.ToLower(n)
				if strings.HasPrefix(lower, "map") || strings.HasPrefix(lower, "commonevents") {
					names = append(names, n)
				}
			}
			sort.Strings(names)
			for _, n := range names {
				rel, rerr := filepath.Rel(root, filepath.Join(absData, n))
				if rerr != nil {
					continue
				}
				scripts = append(scripts, scriptFile{rel: rel, abs: filepath.Join(absData, n)})
			}
			if len(scripts) == 0 {
				notes = append(notes, "www/data 里没有 Map*/CommonEvents 剧本，无文本可导出")
			}
		}
	case "artemis":
		// Artemis：.ast（Lua 表剧本）与 .s（明文）都收。剧本目录不固定——
		// 约定写了就只用它，否则在 root 下按后缀找（解包布局各家不同）。
		var dirs []string
		if d := strings.TrimSpace(conv.ScenarioDir); d != "" {
			dirs = append(dirs, filepath.Join(root, filepath.FromSlash(d)))
		}
		dirs = append(dirs, root)
		for _, dir := range dirs {
			found := 0
			_ = filepath.Walk(dir, func(p string, info os.FileInfo, werr error) error {
				if werr != nil || info == nil || info.IsDir() {
					return nil
				}
				ext := strings.ToLower(filepath.Ext(p))
				if ext != ".ast" && ext != ".s" {
					return nil
				}
				rel, rerr := filepath.Rel(root, p)
				if rerr != nil {
					return nil
				}
				scripts = append(scripts, scriptFile{rel: rel, abs: p})
				found++
				return nil
			})
			if found > 0 {
				break
			}
			scripts = nil // 这层没脚本就换下一个候选，别留下半截结果
		}
		if len(scripts) == 0 {
			notes = append(notes, "没有找到 .ast/.s 剧本，无文本可导出")
		}
	default:
		notes = append(notes, fmt.Sprintf("引擎 %q 的文本导出暂不支持（只做了 kirikiri/rmmz/artemis），留空不影响其它步骤", engineID))
	}

	if len(scripts) == 0 {
		// 空目录：友好报「未发现剧本」且不算失败，也不落索引文件。
		fmt.Printf("文本导出 %s（引擎 %s）\n  未发现剧本（没有可导出的脚本）\n", root, engineID)
		for _, n := range notes {
			fmt.Printf("  ! %s\n", n)
		}
		return nil
	}

	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	rep := &textDumpReport{
		Schema:      TextDumpSchema,
		Game:        filepath.Base(strings.TrimRight(root, ` \ .`)),
		EngineID:    engineID,
		Root:        root,
		GeneratedAt: time.Now().Format(time.RFC3339),
		Notes:       notes,
	}
	for _, sf := range scripts {
		var lines []string
		content, rerr := os.ReadFile(sf.abs)
		if rerr == nil {
			switch strings.ToLower(strings.TrimSpace(engineID)) {
			case "kirikiri":
				lines = kirikiriLines(sf.abs, content)
			case "rmmz":
				lines, _ = extractRmmzText(content)
			case "artemis":
				lines = extractArtemisText(string(content))
			}
		}
		name := txtNameFor(engineID, sf.rel)
		rep.Scripts = append(rep.Scripts, textScriptStat{
			Path:  filepath.ToSlash(name),
			Lines: len(lines),
			Chars: len([]rune(strings.Join(lines, ""))),
		})
		if len(lines) == 0 {
			continue // 全空脚本不落盘，省得导出一堆 0 字节文件
		}
		txtPath := filepath.Join(out, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(txtPath), 0o755); err != nil {
			return err
		}
		if werr := os.WriteFile(txtPath, []byte(strings.Join(lines, "\n")), 0o644); werr != nil {
			return werr
		}
	}
	for _, s := range rep.Scripts {
		rep.TotalScripts++
		rep.TotalLines += s.Lines
		rep.TotalChars += s.Chars
	}
	meta, merr := json.MarshalIndent(rep, "", "  ")
	if merr != nil {
		return merr
	}
	if werr := os.WriteFile(filepath.Join(out, "_textdump.json"), meta, 0o644); werr != nil {
		return werr
	}
	fmt.Printf("文本导出 %s（引擎 %s）\n  脚本 %d 个 · %d 行 · %d 字\n",
		root, engineID, rep.TotalScripts, rep.TotalLines, rep.TotalChars)
	for _, n := range rep.Notes {
		fmt.Printf("  ! %s\n", n)
	}
	return nil
}

// extractArtemisText 从 Artemis 明文剧本抽台词：以 > 开头的是命令行，跳过；
// ; 与 // 是注释，同样跳过；其余非空行就是可读文本。
func extractArtemisText(content string) []string {
	var lines []string
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(strings.TrimRight(raw, "\r"))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, ">") || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "//") {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

// kirikiriLines 导出一份 .ks 的显示文本。
//
// 优先走适配器的 AST：里面的 DecodeScript 会处理 SJIS / UTF-16LE / 单字节 XOR 变体，
// 并把 [iscript] 代码块、; 注释、[...] 标签剥掉。直接把原始字节当字符串用，
// Shift_JIS 剧本会整篇变成替换字符（实测某作 8MB 导出全是乱码）。
// AST 取不到文本时才退回解码后的轻量行解析，至少把能读的行捞出来。
func kirikiriLines(path string, content []byte) []string {
	if ast, err := kirikiri.ParseScriptFile(path); err == nil && len(ast.Texts) > 0 {
		return ast.Texts
	}
	text, _ := kirikiri.DecodeScript(content)
	if strings.TrimSpace(text) == "" {
		return ksPlainLines(string(content))
	}
	return ksPlainLines(text)
}
