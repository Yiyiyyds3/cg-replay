// convention.go —— 「游戏约定」配置。
//
// 为什么要它：**引擎适配器是一劳永逸的**（同一引擎的所有游戏共用一套脚本
// 语法与解析器，文本编码变体也在适配器内部处理）。但同一引擎的不同游戏，
// **解包目录结构与事件命名约定**可能不同——本作是
// `raw/data/scenario` + `cgrai*.ks` 画廊 + `cgNN` 标签 + `01kuukou.ks` 场景；
// 另一款 KiriKiri 游戏可能是 `data/script` + `gallery.ks` + `scene_12.ks`。
//
// 把这些**约定**抽成配置后，换一款游戏就是「加一份配置」，而不是改代码。
// 约定可以按引擎给默认值，也可以用游戏根目录下的 `ucgr-convention.json`
// 覆盖（同引擎的具体游戏优先用自己的）。
package adapter

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Convention 一个游戏的解包目录与事件命名约定。
//
// 字段均为**相对游戏根目录**的路径或正则（RE2 语法）。留空 = 不启用该项
// （例如没有画廊文件的游戏，CGLabelRe 为空即不产出 CG 事件）。
type Convention struct {
	// EngineID 引擎 id（kirikiri / artemis / bgi / catsystem2…）。
	EngineID string `json:"engine_id"`
	// ScenarioDir 剧本目录（相对 root）。
	ScenarioDir string `json:"scenario_dir"`
	// ScriptExt 脚本文件后缀（含点，如 ".ks"）。
	ScriptExt string `json:"script_ext"`
	// GalleryRe 画廊脚本文件正则（匹配整个文件名，如 `^cgrai[0-9]*\.ks$`）。
	// 空 = 该游戏没有画廊（不产出 CG 事件）。
	GalleryRe string `json:"gallery_re"`
	// GalleryIndex 画廊索引脚本名（相对剧本目录，如 "cgrai00.ks"），
	// 里面是各 *cgNN 段的图片序列。
	GalleryIndex string `json:"gallery_index"`
	// CGLabelRe CG 事件标签正则（如 `^cg\d+$`）——画廊里 sysbutton 的
	// graphic/target 都要符合它才算一个 CG 事件。
	CGLabelRe string `json:"cg_label_re"`
	// SceneRe 场景脚本文件正则（如 `^\d{2}[A-Za-z_0-9]*\.ks$`）。
	// 空 = 场景由 fallbackStoryScenes 兜底（按章节标记找）。
	SceneRe string `json:"scene_re"`
	// MovieDirs 影片脚本目录（相对 root）。KiriKiri 的 OP/ED 放在
	// raw/data/sysscn/ 之类的系统目录里，不在剧本目录，所以收不到。
	// 这些目录里**只收含影片指令的脚本**（别的都是系统 UI 脚本，收进来是噪声）。
	MovieDirs []string `json:"movie_dirs,omitempty"`
}

// ConventionFileName 游戏根目录下的约定覆盖文件名。
const ConventionFileName = "ucgr-convention.json"

// DefaultKiriKiri KiriKiri 引擎的默认约定（取自本作实测的目录/命名）。
//
// 换一款 KiriKiri 游戏若目录或命名不同，用游戏根目录下的
// `ucgr-convention.json` 覆盖即可，不必改代码。
func DefaultKiriKiri() Convention {
	return Convention{
		EngineID:     "kirikiri",
		ScenarioDir:  filepath.Join("raw", "data", "scenario"),
		ScriptExt:    ".ks",
		GalleryRe:    `^cgrai[0-9]*\.ks$`,
		GalleryIndex: "cgrai00.ks",
		CGLabelRe:    `^cg\d+$`,
		SceneRe:      `^\d{2}[A-Za-z_0-9]*\.ks$`,
		MovieDirs:    []string{filepath.Join("raw", "data", "sysscn")},
	}
}

// ConventionByEngine 按引擎 id 取默认约定。未知引擎返回空约定 + false
// （调用方应回退到默认约定或报错，不要瞎猜）。
func ConventionByEngine(engineID string) (Convention, bool) {
	switch strings.ToLower(strings.TrimSpace(engineID)) {
	case "kirikiri", "":
		return DefaultKiriKiri(), true
	default:
		return Convention{}, false
	}
}

// LoadConventionFile 读游戏根目录下的约定覆盖文件；不存在返回 (零值, false)。
// 解析失败返回错误（不静默忽略——用户明确写了配置却写坏了要说出来）。
func LoadConventionFile(root string) (Convention, bool, error) {
	p := filepath.Join(root, ConventionFileName)
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return Convention{}, false, nil
		}
		return Convention{}, false, fmt.Errorf("读约定文件失败：%w", err)
	}
	var c Convention
	if err := json.Unmarshal(data, &c); err != nil {
		return Convention{}, true, fmt.Errorf("解析约定文件 %s 失败：%w", p, err)
	}
	return c, true, nil
}

// SaveConventionFile 写约定覆盖文件（给新游戏生成模板用）。
func SaveConventionFile(root string, c Convention) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化约定失败：%w", err)
	}
	data = append(data, '\n')
	p := filepath.Join(root, ConventionFileName)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		return fmt.Errorf("写约定文件失败：%w", err)
	}
	return nil
}

// Validate 校验约定自洽（正则能编译、路径非绝对、目录非空）。
func (c Convention) Validate() error {
	if c.ScenarioDir == "" {
		return fmt.Errorf("约定缺少 scenario_dir")
	}
	if filepath.IsAbs(c.ScenarioDir) {
		return fmt.Errorf("scenario_dir 必须是相对游戏根目录的路径")
	}
	for name, expr := range map[string]string{
		"gallery_re":  c.GalleryRe,
		"cg_label_re": c.CGLabelRe,
		"scene_re":    c.SceneRe,
	} {
		if expr == "" {
			continue
		}
		if _, err := regexp.Compile(expr); err != nil {
			return fmt.Errorf("%s 正则非法：%w", name, err)
		}
	}
	return nil
}

// Merge 用覆盖文件的字段补齐默认值（覆盖文件里留空的字段沿用默认）。
func (c Convention) Merge(base Convention) Convention {
	out := base
	if c.EngineID != "" {
		out.EngineID = c.EngineID
	}
	if c.ScenarioDir != "" {
		out.ScenarioDir = c.ScenarioDir
	}
	if c.ScriptExt != "" {
		out.ScriptExt = c.ScriptExt
	}
	if c.GalleryRe != "" {
		out.GalleryRe = c.GalleryRe
	}
	if c.GalleryIndex != "" {
		out.GalleryIndex = c.GalleryIndex
	}
	if c.CGLabelRe != "" {
		out.CGLabelRe = c.CGLabelRe
	}
	if c.SceneRe != "" {
		out.SceneRe = c.SceneRe
	}
	if len(c.MovieDirs) > 0 {
		out.MovieDirs = c.MovieDirs
	}
	return out
}
