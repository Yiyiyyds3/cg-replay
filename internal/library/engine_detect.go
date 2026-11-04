// engine_detect.go —— 解包目录的引擎判定。
//
// 为什么不能只靠 adapter.IdentifyPath：9nine 的解包树里同时有 .scn（PSB 剧本）
// 和 raw/data/scenario 下少量 .ks（引擎宏/选择菜单），KiriKiri 与 scn 两个
// 适配器都会「认领」，竞聘结果是歧义。这里按「编译剧本优先」的确定性顺序
// 探测，识别不出来再退回通用竞聘。
package library

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"ucgr/internal/adapter"
	_ "ucgr/internal/adapter/scn" // 注册 scn 适配器（Identify 探测 .scn 需要）
)

// kirikiriScenarioDirs KiriKiri 解包树的脚本目录候选（相对 root，按常见度）。
var kirikiriScenarioDirs = []string{
	"raw/data/scenario",
	"data/scenario",
	"scenario",
}

// DetectEngine 判定一个解包目录属于哪个引擎。
//
// 顺序：PSB .scn（scn）→ 含 .ks 的剧本目录（kirikiri）→ NScripter
// （nscript.dat / 0.txt）→ 通用竞聘（bgi/artemis…）。
// 都不匹配返回 ""（调用方应报错，不要瞎猜）。
func DetectEngine(root string) string {
	if a, ok := adapter.Get("scn"); ok {
		if id, err := a.Identify(root); err == nil && id != "" {
			return id
		}
	}
	for _, rel := range kirikiriScenarioDirs {
		if hasAnyFile(filepath.Join(root, filepath.FromSlash(rel)), ".ks") {
			return "kirikiri"
		}
	}
	if _, err := os.Stat(filepath.Join(root, "nscript.dat")); err == nil {
		return "nscript"
	}
	for _, name := range []string{"0.txt", "00.txt"} {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			return "nscript"
		}
	}
	// NeXAS：Script.pac（或任何含 .bin 条目的 .pac）。
	if findScriptPac(root) != "" {
		return "nexas"
	}
	if id, err := adapter.IdentifyPath(root); err == nil {
		return id
	}
	return ""
}

// GameRootOf 由剧本路径向上找游戏根：
// 直接含 raw/、原始归档解包/、_分类/ 的那层，含「图片 + 文本」整理布局的那层，
// 含 scene/*.cst（CatSystem2 整理版）的那层，或 data/raw 目录的上一层。
//
// 9nine 的三种解包布局都能落到对的地方：
//
//	<root>/raw/data/scn/a01.txt.scn
//	<root>/原始归档解包/data_脚本/scn/e01a.txt.scn
//	<root>/_分类/脚本/data_scn__e01a.txt.scn
//
// Artemis / CatSystem2 的整理版解包（`文本/<卷>.pfs/script`、
// `<root>/scene/*.cst`）没有上面这些标记，靠「图片 + 文本」两个同级目录或
// scene 目录认。
//
// 找不到任何标记时返回**剧本所在目录**，而不是一路爬到盘根——爬到盘根会让
// 舞台推断去统计整个盘的图片众数，得出一个看似有值、实际全错的尺寸。
func GameRootOf(script string) string {
	start := filepath.Dir(script)
	dir := start
	for i := 0; i < 8; i++ {
		for _, m := range []string{"raw", "原始归档解包", "_分类"} {
			if st, err := os.Stat(filepath.Join(dir, m)); err == nil && st.IsDir() {
				return dir
			}
		}
		if isDir(filepath.Join(dir, "图片")) && isDir(filepath.Join(dir, "文本")) {
			return dir
		}
		if isDir(filepath.Join(dir, "图片")) && isDir(filepath.Join(dir, "文本_脚本")) {
			return dir
		}
		// CatSystem2 整理版：<root>/scene/*.cst。
		if d := filepath.Join(dir, "scene"); isDir(d) && hasAnyFile(d, ".cst") {
			return dir
		}
		// NScripter：剧本整作一份在根目录（nscript.dat / 0.txt）。
		for _, name := range []string{"nscript.dat", "0.txt", "00.txt"} {
			if st, err := os.Stat(filepath.Join(dir, name)); err == nil && !st.IsDir() {
				return dir
			}
		}
		// NeXAS：剧本打进 Script.pac。
		if findScriptPac(dir) != "" {
			return dir
		}
		base := strings.ToLower(filepath.Base(dir))
		if base == "data" || base == "raw" {
			return filepath.Dir(dir)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return start
}

// hasAnyFile 报告 dir 下（不递归）是否有指定后缀的文件。
func hasAnyFile(dir, ext string) bool {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range ents {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ext) {
			return true
		}
	}
	return false
}

// DetectConvention 判定引擎并给出该游戏的约定（约定文件的 engine_id 优先，
// 其次目录探测；都没有时按 KiriKiri 处理，与历史行为一致）。
//
// 特例：目录里一个剧本都没有、但有原作的打包签名（.int / .pfs / .xp3…）时
// **不猜引擎**，直接报「只有打包文件、还没解包」——把「没解包」错报成
// 「不是 KiriKiri 引擎」会让人去改引擎代码，方向完全错了。
//
// 前端（desktop）与 CLI 都走这里，避免两处各写一份识别顺序。
func DetectConvention(root string) (string, adapter.Convention, error) {
	file, exists, err := adapter.LoadConventionFile(root)
	if err != nil {
		return "", adapter.Convention{}, err
	}
	engineID := ""
	if exists {
		engineID = strings.ToLower(strings.TrimSpace(file.EngineID))
	}
	if engineID == "" {
		if exists {
			return "", adapter.Convention{}, fmt.Errorf(
				"识别不出引擎：请在 %s 里写明 engine_id（支持：%s）",
				adapter.ConventionFileName, strings.Join(adapter.IDs(), " / "))
		}
		engineID = DetectEngine(root)
	}
	// 整棵树一个剧本都没有、却有原作打包签名（.int / .pfs / .xp3…）时，
	// 不猜引擎，直接说「还没解包」——把「没解包」错报成「不是 KiriKiri 引擎」
	// 会让人去改引擎代码，方向完全错了。用户写了约定文件就尊重用户的判断。
	if !exists && !anyScriptFile(root) {
		if packed, evidence := PackedEngineOf(root); packed != "" {
			return "", adapter.Convention{}, fmt.Errorf(
				"这层只有打包文件（%s，属 %s 引擎），还没解包：先解包成可读剧本再扫（解包产物在别的目录时，直接对那个目录扫）",
				evidence, packed)
		}
		// 既没有剧本也没有认得出的打包签名：多半是别的引擎
		// （RealLive / Siglus…）或没解包，别硬按 KiriKiri 去找。
		return "", adapter.Convention{}, fmt.Errorf(
			"没有找到可回放的剧本（.ks/.scn/.ast/.bs/.cst/nscript.dat/Script.pac），也没认出打包签名：可能是其它引擎，或还没解包")
	}
	if engineID == "" {
		engineID = "kirikiri"
	}
	conv, ok := adapter.ConventionByEngine(engineID)
	if !ok {
		return engineID, adapter.Convention{}, fmt.Errorf(
			"未知引擎 %q（支持：%s）", engineID, strings.Join(adapter.IDs(), " / "))
	}
	if exists {
		conv = file.Merge(conv)
	}
	conv.EngineID = engineID
	if err := conv.Validate(); err != nil {
		return engineID, conv, err
	}
	return engineID, conv, nil
}

// packedSignatures 原作打包文件的识别顺序（更专属的放前面：
// BGI 的 .hvl、CatSystem2 的 boot.dfn 都比通用 .int 更可靠）。
var packedSignatures = []struct {
	engine string
	match  func(name string) bool
}{
	{"bgi", func(n string) bool {
		return n == "bgi.hvl" || n == "bgi.sdb" || strings.HasSuffix(n, ".sdb") && strings.HasPrefix(n, "bgi")
	}},
	{"catsystem2", func(n string) bool {
		// .int 是 CatSystem2 的资源包（image00.int 等），认得比较稳；
		// cs2conf*/boot.dfn 是启动件，作为同引擎的更强证据。
		return n == "cs2.exe" || n == "boot.dfn" ||
			strings.HasPrefix(n, "cs2conf") || strings.HasSuffix(n, ".int")
	}},
	{"artemis", func(n string) bool {
		return strings.HasSuffix(n, ".pfs") || strings.Contains(n, ".pfs.")
	}},
	{"kirikiri", func(n string) bool {
		return strings.HasSuffix(n, ".xp3")
	}},
	{"nexas", func(n string) bool {
		// Script.pac / Filechk.exe 是 NeXAS（戏画）游戏目录的标志件。
		return strings.EqualFold(n, "script.pac") || strings.EqualFold(n, "filechk.exe")
	}},
}

// replayableScriptExts 本仓库能直接回放的剧本后缀。
var replayableScriptExts = map[string]bool{
	".ks": true, ".scn": true, ".ast": true, ".bs": true, ".cst": true,
}

// anyScriptFile 报告 root 下是否至少有一个可回放的剧本文件（最多探 6 层）。
//
// 用来区分「没解包」与「解包了但布局不常见」：没有剧本就没有回放素材，
// 这时报错该指向解包，而不是指向引擎识别。
func anyScriptFile(root string) bool {
	found := false
	root = filepath.Clean(root)
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || found {
			return nil
		}
		rel, e := filepath.Rel(root, p)
		if e != nil {
			return nil
		}
		if d.IsDir() {
			if rel != "." && strings.Count(rel, string(filepath.Separator)) >= discoverMaxDepth {
				return fs.SkipDir
			}
			return nil
		}
		if replayableScriptExts[strings.ToLower(filepath.Ext(p))] {
			found = true
			return fs.SkipAll
		}
		// NScripter 的剧本是整作一份（无需解包即可回放），同样算数。
		switch strings.ToLower(d.Name()) {
		case "nscript.dat", "0.txt", "00.txt":
			found = true
			return fs.SkipAll
		}
		// NeXAS 的剧本也在打包里（Script.pac，无需解包即可回放）。
		if strings.EqualFold(d.Name(), "script.pac") {
			found = true
			return fs.SkipAll
		}
		return nil
	})
	return found
}

// PackedEngineOf 在 root 下（最多两层）找原作的打包文件，返回引擎与证据文件名。
//
// 用途只有一个：把「这层还没解包」和「这层不是该引擎」区分开。返回 "" 表示
// 没找到能说明问题的打包文件（真的不认识，别再往下猜）。
func PackedEngineOf(root string) (string, string) {
	type hit struct {
		depth int
		name  string
	}
	hits := map[string]hit{}
	root = filepath.Clean(root)
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, e := filepath.Rel(root, p)
		if e != nil {
			return nil
		}
		depth := strings.Count(rel, string(filepath.Separator))
		if d.IsDir() {
			if rel != "." && depth >= 2 {
				return fs.SkipDir
			}
			return nil
		}
		name := strings.ToLower(d.Name())
		for _, sig := range packedSignatures {
			if !sig.match(name) {
				continue
			}
			if cur, ok := hits[sig.engine]; !ok || depth < cur.depth {
				hits[sig.engine] = hit{depth: depth, name: filepath.ToSlash(rel)}
			}
		}
		return nil
	})
	best, bestDepth := "", 99
	for _, sig := range packedSignatures {
		if h, ok := hits[sig.engine]; ok && h.depth < bestDepth {
			best, bestDepth = sig.engine, h.depth
		}
	}
	if best == "" {
		return "", ""
	}
	return best, hits[best].name
}
