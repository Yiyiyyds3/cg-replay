// engine_detect.go —— 解包目录的引擎判定。
//
// 为什么不能只靠 adapter.IdentifyPath：9nine 的解包树里同时有 .scn（PSB 剧本）
// 和 raw/data/scenario 下少量 .ks（引擎宏/选择菜单），KiriKiri 与 scn 两个
// 适配器都会「认领」，竞聘结果是歧义。这里按「编译剧本优先」的确定性顺序
// 探测，识别不出来再退回通用竞聘。
package library

import (
	"fmt"
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
// 顺序：PSB .scn（scn）→ 含 .ks 的剧本目录（kirikiri）→ 通用竞聘（bgi/artemis…）。
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
	if id, err := adapter.IdentifyPath(root); err == nil {
		return id
	}
	return ""
}

// GameRootOf 由剧本路径向上找游戏根：
// 直接含 raw/、原始归档解包/、_分类/ 的那层，或 data/raw 目录的上一层。
//
// 9nine 的三种解包布局都能落到对的地方：
//
//	<root>/raw/data/scn/a01.txt.scn
//	<root>/原始归档解包/data_脚本/scn/e01a.txt.scn
//	<root>/_分类/脚本/data_scn__e01a.txt.scn
func GameRootOf(script string) string {
	dir := filepath.Dir(script)
	for i := 0; i < 6; i++ {
		for _, m := range []string{"raw", "原始归档解包", "_分类"} {
			if st, err := os.Stat(filepath.Join(dir, m)); err == nil && st.IsDir() {
				return dir
			}
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
	return dir
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
		engineID = DetectEngine(root)
	}
	if engineID == "" {
		if exists {
			return "", adapter.Convention{}, fmt.Errorf(
				"识别不出引擎：请在 %s 里写明 engine_id（支持：%s）",
				adapter.ConventionFileName, strings.Join(adapter.IDs(), " / "))
		}
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
