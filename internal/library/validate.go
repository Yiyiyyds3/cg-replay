// validate.go —— 事件库有效性过滤：只留「真能播出画面」的事件。
//
// 结论先行：CG 事件至少要有一页能命中素材文件，否则整条从库里删掉；场景事件
// 只留「特殊 CG / 事件 CG」（主人不要常规地图事件），RPG Maker 的 map: 前缀
// 场景未命中特殊特征就整条剔除；章节跟着被删的场景一起删。
//
// 判据只看文件索引，不解码像素：真正的「整段纯黑」由 vision.CheckFrames 在
// 回放期判定（见 internal/vision/frame.go），这里管的是另一类空壳——连图都不
// 存在的 CG 槽位（画廊里有格子、素材没解出来），它们留在库里只会让人点开一片空。
package library

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"
)

// ValidateSchema 过滤报告的能力版本。报告字段增删时递增，读报告的一方据此
// 判断是老格式还是新格式（与库清单的 SchemaVersion 同样的口径）。
const ValidateSchema = 1

// DroppedMax 明细上限：报告要能直接贴进日志，几百条以上没人看，按条数截断
// 并在 Notes 里交代「还剩多少条没列」。
const DroppedMax = 200

var (
	// imageExts 算「有效画面」的扩展名。.tlg / .ctex 是引擎自有封装图
	// （TLG5、CatSystem2 的 CTEX），解出来就是图，同样算有效画面。
	// .rpgmvp / .png_ / .rpgmvm 是 RPG Maker MV/MZ 的加密图名——44/54 款游戏
	// 的 root 直接指 E 盘源目录（未解包），图片只有加密形态：加密文件存在
	// 就等于资源在，同样算有效画面（真正「整段全黑」的判定在回放期）。
	imageExts = map[string]bool{
		".png": true, ".jpg": true, ".jpeg": true, ".webp": true,
		".bmp": true, ".tlg": true, ".ctex": true, ".avif": true,
		".rpgmvp": true, ".png_": true, ".rpgmvm": true,
	}
	// audioExts 音频扩展名：只用来统计 AudioCount（音频不参与事件有效性判定）。
	// .ogg_ / .rpgmvo / .rpgmvm 是 RPG Maker 加密后的语音名，算音频；影片
	// （.webm/.mp4）既不是图也不是音频，两边都不计。
	audioExts = map[string]bool{
		".ogg": true, ".wav": true, ".mp3": true, ".m4a": true,
		".ogg_": true, ".rpgmvo": true, ".rpgmvm": true,
	}
)

// specialSceneRe 特殊 CG / 事件 CG 的特征式。
//
// 只认这几类：带 ev/event 编号段（EV001、ev_12、/event/3）、H 场景（hscene、
// h001）、回想（recall）、附赠（omake）、额外（extra）、鉴赏（gallery、cgrai）、
// 事件 CG（event_cg）、EGZ 后缀。常规地图事件一律不算。
var specialSceneRe = regexp.MustCompile(`(?i)(^|[/\_#])ev(ent)?([_/\#0-9]|$)|hscene|(^|_)h[0-9]|recall|omake|extra|gallery|cgrai|event_cg|egz`)

// ValidateReport 一次过滤的结果（落成 <out>.validate.json 给流水线读）。
type ValidateReport struct {
	Schema      int    `json:"schema"`
	Game        string `json:"game,omitempty"`
	EngineID    string `json:"engine_id,omitempty"`
	Root        string `json:"root"`
	GeneratedAt string `json:"generated_at"`
	// CGKept / CGDroppedEmpty 过滤后留下的 CG 数与「一页都命中不了」被删的 CG 数。
	CGKept         int `json:"cg_kept"`
	CGDroppedEmpty int `json:"cg_dropped_empty"`
	// SceneKept / SceneDropped 场景事件的去向（只对特殊 CG 口径生效）。
	SceneKept    int `json:"scene_kept"`
	SceneDropped int `json:"scene_dropped"`
	// AudioCount walk 时顺路数出来的音频文件数（音频不参与判定，只做盘点）。
	AudioCount int `json:"audio_count"`
	// TextScripts / TextLines 本次留 0：台词要解脚本，归 textdump 命令统计，
	// 过滤这一步顺手做只会把一次 walk 变成一次全量解析。
	TextScripts int `json:"text_scripts"`
	TextLines   int `json:"text_lines"`
	// Dropped 被删事件的「id: 原因」，最多 DroppedMax 条。
	Dropped []string `json:"dropped,omitempty"`
	// Notes 过滤口径的补充说明（读报告的人据此判断结论的适用范围）。
	Notes []string `json:"notes,omitempty"`
}

// drop 记账一条被删事件（超过上限只计数，由调用方在 Notes 里交代省略条数）。
func (r *ValidateReport) drop(id, reason string) {
	if id == "" {
		id = "(无 id)"
	}
	if len(r.Dropped) >= DroppedMax {
		return
	}
	r.Dropped = append(r.Dropped, id+": "+reason)
}

// assetIndex 一次 walk 建出来的素材索引。
//
// 三套键都存小写：rel 是按 root 的相对路径，base 是带扩展名的 basename，
// stem 是剥掉扩展名的 basename。三套是为了兜住各家 storage 写法不一致
// （详见 hitPage）。
type assetIndex struct {
	rel     map[string]bool
	relStem map[string]bool // rel 剥掉扩展名（img/ev001.rpgmvp → img/ev001）
	base    map[string]bool
	stem    map[string]bool
}

// indexAssets 递归 walk 一次 root，收集图片索引并顺路数音频文件。
//
// 只记「文件的相对路径与名字」，不打开文件：判定用的是「文件在不在」，
// 打开几万张图只为看它存在与否是纯浪费。
func indexAssets(root string) (*assetIndex, int) {
	idx := &assetIndex{
		rel:     make(map[string]bool),
		relStem: make(map[string]bool),
		base:    make(map[string]bool),
		stem:    make(map[string]bool),
	}
	audio := 0
	_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			// 单个文件/子目录读不到（权限、链接断链）不影响整体过滤：跳过即可，
			// 少记一张图最多是多删一个事件，不会让整个命令失败。
			return nil
		}
		if info.IsDir() {
			return nil
		}
		name := info.Name()
		ext := strings.ToLower(filepath.Ext(name))
		switch {
		case imageExts[ext]:
			rel, rerr := filepath.Rel(root, p)
			if rerr != nil {
				return nil
			}
			relKey := strings.ToLower(filepath.ToSlash(rel))
			idx.rel[relKey] = true
			if stem := strings.TrimSuffix(relKey, ext); stem != "" {
				idx.relStem[stem] = true
			}
			base := strings.ToLower(name)
			idx.base[base] = true
			if stem := strings.TrimSuffix(base, ext); stem != "" {
				idx.stem[stem] = true
			}
		case audioExts[ext]:
			audio++
		}
		return nil
	})
	return idx, audio
}

// hitPage 一页图是否命中素材索引：相对路径 → basename → 剥扩展名。
//
// 三层是因为各家 storage 写法不统一：KiriKiri 常给「bg/cg001」这种不带扩展名
// 的相对名，RPG Maker 给「pictures/EV001」，媒体扫描给完整文件名。先按相对
// 路径精确命中，再退到 basename（同名多路径时任意一条命中就算命中），最后把
// 页名剥掉扩展名去比索引的 stem（「cg001.png」对「cg001」）。
func (idx *assetIndex) hitPage(page string) bool {
	p := strings.ToLower(filepath.ToSlash(strings.TrimSpace(page)))
	p = strings.TrimPrefix(p, "./")
	if p == "" {
		return false
	}
	if idx.rel[p] {
		return true
	}
	if stem := strings.TrimSuffix(p, filepath.Ext(p)); stem != "" && idx.relStem[stem] {
		return true
	}
	base := p
	if i := strings.LastIndexByte(base, '/'); i >= 0 {
		base = base[i+1:]
	}
	if idx.base[base] {
		return true
	}
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	return stem != "" && idx.stem[stem]
}

// hitAnyPage 该事件是否至少有一页命中（Pages 为空 = 无画面，直接算不命中）。
func (idx *assetIndex) hitAnyPage(pages []string) bool {
	for _, p := range pages {
		if idx.hitPage(p) {
			return true
		}
	}
	return false
}

// sceneSignature 把场景的 ID / Label / Title / Script 拼成一段特征串。
//
// 拼接后再把所有非字母数字的字符（空格、·、冒号、横线…）折成 "/"：特征正则
// 的边界类是 [/_#]，这样「芙拉村 · EV015」里的 EV015 才落在 token 开头被认出来。
// 只把四个字段原样拼起来的话，标题里的空格会把边界吃掉，带 EV 编号的事件就漏了。
// 空字段不留空段，免得 // 造出假边界。
func sceneSignature(s SceneEvent) string {
	parts := make([]string, 0, 4)
	for _, p := range []string{s.ID, s.Label, s.Title, s.Script} {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	var b strings.Builder
	for _, r := range strings.Join(parts, "/") {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('/')
	}
	return b.String()
}

// isMapScene rmmz 的常规地图事件 id 形如 map:Map001.json:12:0#0。
func isMapScene(id string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(id)), "map:")
}

// Validate 原地过滤事件库，返回过滤报告；root 空则用库自己记的根。
//
// 改的是调用者手里的 lib 本身（CGs / Scenes / Chapters 三个切片被替换），
// 所以调用方在 Save 之前调用它，落盘的库就只剩有效事件。
func (lib *Library) Validate(root string) *ValidateReport {
	if strings.TrimSpace(root) == "" {
		root = lib.Root
	}
	rep := &ValidateReport{
		Schema:      ValidateSchema,
		Game:        lib.Game,
		EngineID:    lib.EngineID,
		Root:        root,
		GeneratedAt: time.Now().Format(time.RFC3339),
	}

	// ① 一次 walk 建索引：图片列表 + 音频计数，后面所有判定都查这份索引。
	idx, audio := indexAssets(root)
	rep.AudioCount = audio

	// ② 安全阀：索引为空说明 root 下根本看不到任何素材文件——多半是指错了
	//    目录或整包未解出。这时一条都不删（宁多勿错杀），把原因写进 Notes 让
	//    上游按索引规模自己发现目录问题；否则「整库被误判为空」比留几个空壳
	//    事件严重得多。
	if len(idx.rel) == 0 {
		rep.Notes = append(rep.Notes,
			"素材索引为空（root 下没有任何图片文件可见），本次未执行过滤——请检查 root 是否指错目录")
		rep.CGKept = len(lib.CGs)
		rep.SceneKept = len(lib.Scenes)
		return rep
	}

	// ③ CG：Pages 非空且至少一页命中才留；全空/全落空整条删掉。
	keptCG := make([]CGEvent, 0, len(lib.CGs))
	for _, ev := range lib.CGs {
		if idx.hitAnyPage(ev.Pages) {
			keptCG = append(keptCG, ev)
			continue
		}
		rep.CGDroppedEmpty++
		if len(ev.Pages) == 0 {
			rep.drop(ev.ID, "没有画面页（Pages 为空）")
		} else {
			rep.drop(ev.ID, fmt.Sprintf("画面页全未命中素材（%d 页，首张 %q）",
				len(ev.Pages), ev.Pages[0]))
		}
	}
	lib.CGs = keptCG
	rep.CGKept = len(lib.CGs)

	// ③ 场景：只留特殊 CG / 事件 CG。rmmz 的 map: 前缀场景是常规地图事件
	//    （走路、捡东西、NPC 对话），主人不要，未命中特殊特征就整条剔除；
	//    其它引擎的场景本就是剧本文件，不在这条口径里，一律保留。
	rmmzMap := strings.EqualFold(strings.TrimSpace(lib.EngineID), "rmmz")
	keptScenes := make([]SceneEvent, 0, len(lib.Scenes))
	droppedScene := make(map[string]bool, len(lib.Scenes))
	for _, s := range lib.Scenes {
		if specialSceneRe.MatchString(sceneSignature(s)) {
			keptScenes = append(keptScenes, s)
			continue
		}
		if rmmzMap && isMapScene(s.ID) {
			droppedScene[s.ID] = true
			rep.SceneDropped++
			rep.drop(s.ID, "常规地图事件（未命中特殊 CG 特征）")
			continue
		}
		keptScenes = append(keptScenes, s)
	}
	lib.Scenes = keptScenes
	rep.SceneKept = len(lib.Scenes)

	// ④ 章节：所属场景被删的章节一并删（章节 id = <场景id>#<labelPos>，
	//    留着会指向一个不存在的场景，前端点进去就是空）。
	chapterDropped := 0
	if len(droppedScene) > 0 && len(lib.Chapters) > 0 {
		keptChapters := make([]ChapterEvent, 0, len(lib.Chapters))
		for _, ch := range lib.Chapters {
			if droppedScene[ch.Scene] {
				chapterDropped++
				rep.drop(ch.ID, "所属场景 "+ch.Scene+" 已被剔除")
				continue
			}
			keptChapters = append(keptChapters, ch)
		}
		lib.Chapters = keptChapters
	}

	// ⑤ Notes：交代口径与截断，读报告的人不用猜结论的适用范围。
	rep.Notes = append(rep.Notes,
		fmt.Sprintf("素材索引：图片 %d 条路径、音频 %d 个（只记文件存在性，不解码像素）",
			len(idx.rel), audio))
	if !rmmzMap {
		rep.Notes = append(rep.Notes,
			fmt.Sprintf("引擎 %q 不是 rmmz：场景按特殊特征筛选只作用于 map: 前缀的常规地图事件，剧本场景一律保留",
				lib.EngineID))
	}
	if total := rep.CGDroppedEmpty + rep.SceneDropped + chapterDropped; total > len(rep.Dropped) {
		rep.Notes = append(rep.Notes,
			fmt.Sprintf("剔除明细仅保留前 %d 条，另 %d 条未列出", len(rep.Dropped), total-len(rep.Dropped)))
	}
	return rep
}
