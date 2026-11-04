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

	// —— 以下为编译型剧本引擎（scn / PSB）专用，其它引擎留空即可 ——

	// CGList CG 鉴赏表（相对 root 的 .csv，如 "raw/data/main/cglist.csv"）。
	// 空 = 该游戏没有 CG 鉴赏表（不产出 CG 事件）。
	CGList string `json:"cg_list"`
	// StageW/StageH 原作舞台尺寸（像素）。0 = 用引擎默认（scn 为 1280×720）。
	// 回放画布按它渲染——素材按原生坐标合成，画布尺寸不对会错位。
	StageW int `json:"stage_w"`
	StageH int `json:"stage_h"`
	// ImageDirs/AudioDirs 素材目录（相对 root，可多个；留空用引擎默认目录）。
	ImageDirs []string `json:"image_dirs"`
	AudioDirs []string `json:"audio_dirs"`
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

// DefaultRMMZ RPG Maker MV/MZ 的默认约定。
//
// MV 的 data 目录在 www/data，MZ 在 data；实际使用前由 rmmz.FindDataDir
// 探测，这里给的 www/data 只是让 ScanWith 有个非空默认值。
func DefaultRMMZ() Convention {
	return Convention{
		EngineID:    "rmmz",
		ScenarioDir: "www/data",
		ScriptExt:   ".json",
	}
}

// DefaultSCN 编译剧本（PSB / M2 KiriKiri Evolution）的默认约定。
//
// 9nine 系列用 .scn（PSB 容器）：剧本在 raw/data/scn，CG 鉴赏表在
// raw/data/main/cglist.csv，舞台 1280×720。整理版布局（原始归档解包/
// _分类）用游戏根目录的 ucgr-convention.json 覆盖这三个路径即可。
func DefaultSCN() Convention {
	return Convention{
		EngineID:    "scn",
		ScenarioDir: filepath.Join("raw", "data", "scn"),
		ScriptExt:   ".scn",
		CGList:      filepath.Join("raw", "data", "main", "cglist.csv"),
		StageW:      1280,
		StageH:      720,
	}
}

// DefaultArtemis Artemis 的默认约定。
//
// .ast 剧本在解包树里的层级不固定（实测：`文本/<卷>.pfs/script`、
// `原始归档解包/<卷>.pfs/script`），所以 ScenarioDir 留空 = 由扫描器
// 自动探测；用户也可用 ucgr-convention.json 显式指定。
func DefaultArtemis() Convention {
	return Convention{EngineID: "artemis"}
}

// DefaultBGI BGI 的默认约定。
//
// .bs 剧本在 `文本_脚本/dataXXXXX/` 下分多个归档目录，同样交给自动探测。
func DefaultBGI() Convention {
	return Convention{EngineID: "bgi"}
}

// DefaultCatSystem2 CatSystem2 的默认约定。
//
// .cst 剧本在 `原始归档解包/scene/` 或 `_分类/文本/scene/` 下，
// 层级随解包工具而异，交给自动探测。
func DefaultCatSystem2() Convention {
	return Convention{EngineID: "catsystem2"}
}

// DefaultNScripter NScripter 的默认约定。
//
// 剧本是整作一份 nscript.dat（或 0.txt），在游戏根目录，交给自动探测；
// 没有画廊/CG 表——事件由适配器按「换背景」切分。
func DefaultNScripter() Convention {
	return Convention{EngineID: "nscript"}
}

// autoScenarioEngines 剧本目录可由扫描器自动探测的引擎。
//
// 这些引擎的剧本在解包树里的层级不固定（多卷 .pfs / 多个 dataXXXXX
// 目录）或整作一份在根目录（nscript），约定里的 scenario_dir 留空是
// **正常形态**，不算配置缺失。
var autoScenarioEngines = map[string]bool{
	"artemis":    true,
	"bgi":        true,
	"catsystem2": true,
	"nscript":    true,
}

// ConventionByEngine 按引擎 id 取默认约定。未知引擎返回空约定 + false
// （调用方应回退到默认约定或报错，不要瞎猜）。
func ConventionByEngine(engineID string) (Convention, bool) {
	switch strings.ToLower(strings.TrimSpace(engineID)) {
	case "kirikiri", "":
		return DefaultKiriKiri(), true
	case "rmmz":
		return DefaultRMMZ(), true
	case "scn":
		return DefaultSCN(), true
	case "artemis":
		return DefaultArtemis(), true
	case "bgi":
		return DefaultBGI(), true
	case "catsystem2":
		return DefaultCatSystem2(), true
	case "nscript":
		return DefaultNScripter(), true
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
		// 自动探测引擎（多卷解包）允许留空；其余引擎留空说明配置没写全。
		if !autoScenarioEngines[strings.ToLower(strings.TrimSpace(c.EngineID))] {
			return fmt.Errorf("约定缺少 scenario_dir")
		}
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
	for name, p := range map[string]string{"cg_list": c.CGList} {
		if p != "" && filepath.IsAbs(p) {
			return fmt.Errorf("%s 必须是相对游戏根目录的路径", name)
		}
	}
	for _, p := range append(append([]string{}, c.ImageDirs...), c.AudioDirs...) {
		if filepath.IsAbs(p) {
			return fmt.Errorf("素材目录 %s 必须是相对游戏根目录的路径", p)
		}
	}
	if c.StageW < 0 || c.StageH < 0 {
		return fmt.Errorf("stage_w/stage_h 不能为负")
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
	if c.CGList != "" {
		out.CGList = c.CGList
	}
	if c.StageW > 0 {
		out.StageW = c.StageW
	}
	if c.StageH > 0 {
		out.StageH = c.StageH
	}
	if len(c.ImageDirs) > 0 {
		out.ImageDirs = append([]string(nil), c.ImageDirs...)
	}
	if len(c.AudioDirs) > 0 {
		out.AudioDirs = append([]string(nil), c.AudioDirs...)
	}
	return out
}
