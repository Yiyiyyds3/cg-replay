// Package library 扫出「原作自己的事件表」——本作（重返蓝鲸岛）把 CG 鉴赏写成
// cgrai.ks…cgrai6.ks（6 页 × 16 格，cg001…cg096）+ cgrai00.ks（每个 *cgNN 一段演出），
// 主线则是编号剧本 01kuukou.ks…76utyu.ks。
//
// 这里只做「按原作的分割方式切事件」：每个 CG 一个独立事件，每个主线剧本一个场景事件。
package library

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"ucgr/internal/adapter/kirikiri"
)

// CGEvent 一个 CG 事件的回放单位（= cgrai00.ks 里的一个 *cgNN 段）。
type CGEvent struct {
	ID     string   `json:"id"`     // 画廊槽位名，如 cg017
	Index  int      `json:"index"`  // 17
	Title  string   `json:"title"`  // 展示名（优先首张图名）
	Thumb  string   `json:"thumb"`  // 画廊里的缩略图 storage（cg017）
	Script string   `json:"script"` // 相对 root 的脚本路径
	Label  string   `json:"label"`  // 标签名（cg17）
	Pages  []string `json:"pages"`  // 依次显示的图（storage 名）
}

// SceneEvent 一个主线场景事件（= 一个编号剧本）。
type SceneEvent struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Script string `json:"script"` // 相对 root 的脚本路径
}

// ChapterEvent 一个「演出段落」= 原作自己的章节标记 `*labelNN|显示名`。
//
// 场景脚本动辄一整块自动播放一小时（71kaigan01.ks），按章节切开才能逐段回放。
// 章节标记会重号（分支路线同号异名），故用 LabelPos（在脚本标签表里的下标）寻址。
type ChapterEvent struct {
	ID       string `json:"id"`        // 唯一 id：<场景id>#<labelPos>
	Index    int    `json:"index"`     // 章节号（label94 → 94）
	Title    string `json:"title"`     // `|` 之后的显示名
	Script   string `json:"script"`    // 相对 root 的脚本路径
	Scene    string `json:"scene"`     // 所属场景 id
	LabelPos int    `json:"label_pos"` // 章节标记在脚本标签表里的下标
	Name     string `json:"name"`      // 标签名（label94）
}

// Library 事件库清单。
type Library struct {
	Root        string         `json:"root"`
	Game        string         `json:"game,omitempty"`
	GeneratedAt string         `json:"generated_at"`
	CGs         []CGEvent      `json:"cgs"`
	Scenes      []SceneEvent   `json:"scenes"`
	Chapters    []ChapterEvent `json:"chapters,omitempty"`
}

var (
	thumbRe   = regexp.MustCompile(`^cg\d+$`)
	labelRe   = regexp.MustCompile(`^cg\d+$`)
	sceneRe   = regexp.MustCompile(`^\d{2}[A-Za-z_0-9]*\.ks$`)
	galleryRe = regexp.MustCompile(`^cgrai[0-9]*\.ks$`)
)

// ScenarioDir 解包目录下的剧本目录。
func ScenarioDir(root string) string { return filepath.Join(root, "raw", "data", "scenario") }

// Scan 扫一个解包目录，产出事件库。
func Scan(root string) (*Library, error) {
	dir := ScenarioDir(root)
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("读不到剧本目录 %s：%w", dir, err)
	}
	lib := &Library{
		Root:        root,
		Game:        filepath.Base(strings.TrimRight(root, `\/`)),
		GeneratedAt: time.Now().Format(time.RFC3339),
	}

	// ① 画廊页（cgrai.ks、cgrai2..6.ks）给出槽位顺序 + 缩略图 + 跳转标签。
	type slot struct{ thumb, label string }
	var slots []slot
	var galleryFiles []string
	for _, e := range ents {
		name := e.Name()
		if e.IsDir() || !galleryRe.MatchString(name) || strings.EqualFold(name, "cgrai00.ks") {
			continue
		}
		galleryFiles = append(galleryFiles, name)
	}
	sort.Strings(galleryFiles)
	for _, name := range galleryFiles {
		ast, err := kirikiri.ParseScriptFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		for _, sec := range kirikiri.Sections(ast) {
			for _, it := range kirikiri.Segment(ast, sec.Name) {
				if it.Kind != "cmd" || it.Cmd == nil || !strings.EqualFold(it.Cmd.Tag, "sysbutton") {
					continue
				}
				thumb := strings.TrimSpace(it.Cmd.Attrs["graphic"])
				target := strings.TrimSpace(it.Cmd.Attrs["target"])
				if !thumbRe.MatchString(thumb) {
					continue
				}
				label := strings.TrimPrefix(target, "*")
				if !labelRe.MatchString(label) {
					continue
				}
				slots = append(slots, slot{thumb: thumb, label: label})
			}
		}
	}

	// ② cgrai00.ks 里每个 *cgNN 段的图片序列。
	cgScript := filepath.Join(dir, "cgrai00.ks")
	rel := func(p string) string {
		if r, err := filepath.Rel(root, p); err == nil {
			return filepath.ToSlash(r)
		}
		return filepath.ToSlash(p)
	}
	pages := map[string][]string{}
	if ast, err := kirikiri.ParseScriptFile(cgScript); err == nil {
		for _, sec := range kirikiri.Sections(ast) {
			if !labelRe.MatchString(sec.Name) {
				continue
			}
			if imgs := kirikiri.SegmentImages(ast, sec.Name); len(imgs) > 0 {
				pages[sec.Name] = imgs
			}
		}
	}

	for _, s := range slots {
		ev := CGEvent{
			ID:     s.thumb,
			Index:  kirikiri.LabelIndex(s.thumb),
			Thumb:  s.thumb,
			Label:  s.label,
			Script: rel(cgScript),
			Pages:  pages[s.label],
		}
		ev.Title = s.thumb
		if len(ev.Pages) > 0 {
			ev.Title = fmt.Sprintf("%s · %s", s.thumb, ev.Pages[0])
		}
		lib.CGs = append(lib.CGs, ev)
	}
	sort.SliceStable(lib.CGs, func(i, j int) bool { return lib.CGs[i].Index < lib.CGs[j].Index })

	// ③ 主线剧本（编号 .ks）= 场景事件。
	var names []string
	for _, e := range ents {
		if e.IsDir() || !sceneRe.MatchString(e.Name()) {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		id := strings.TrimSuffix(name, filepath.Ext(name))
		lib.Scenes = append(lib.Scenes, SceneEvent{
			ID:     id,
			Title:  id,
			Script: rel(filepath.Join(dir, name)),
		})
	}

	// ④ 演出段落：场景脚本里原作自己的章节标记（*labelNN|显示名）。
	// 一个长脚本常有几十个标签但只有十来个章节标记，按章节切才是「演出段落」。
	for _, s := range lib.Scenes {
		ast, err := kirikiri.ParseScriptFile(filepath.Join(root, filepath.FromSlash(s.Script)))
		if err != nil {
			continue
		}
		for _, ch := range kirikiri.Chapters(ast) {
			lib.Chapters = append(lib.Chapters, ChapterEvent{
				ID:       fmt.Sprintf("%s#%d", s.ID, ch.LabelPos),
				Index:    ch.Index,
				Title:    ch.Title,
				Script:   s.Script,
				Scene:    s.ID,
				LabelPos: ch.LabelPos,
				Name:     ch.Name,
			})
		}
	}
	return lib, nil
}

// Save 写库清单（JSON）。
func (l *Library) Save(path string) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// Load 读库清单。
func Load(path string) (*Library, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var lib Library
	if err := json.Unmarshal(data, &lib); err != nil {
		return nil, fmt.Errorf("解析库清单 %s 失败：%w", path, err)
	}
	return &lib, nil
}

// FindCG 按 id（cg017 / cg17 / 17）找 CG 事件。
func (l *Library) FindCG(id string) *CGEvent {
	key := strings.ToLower(strings.TrimSpace(id))
	digits := tailDigits(key)
	for i := range l.CGs {
		c := &l.CGs[i]
		if strings.ToLower(c.ID) == key || strings.ToLower(c.Label) == key {
			return c
		}
		if digits != "" && strconv.Itoa(c.Index) == digits {
			return c
		}
	}
	return nil
}

// FindScene 按 id（72toire / 71kaigan01）找场景事件。
func (l *Library) FindScene(id string) *SceneEvent {
	key := strings.ToLower(strings.TrimSpace(id))
	for i := range l.Scenes {
		if strings.ToLower(l.Scenes[i].ID) == key {
			return &l.Scenes[i]
		}
	}
	return nil
}

// FindChapter 按 id（71kaigan01#27 / 或章节标题）找演出段落。
func (l *Library) FindChapter(id string) *ChapterEvent {
	key := strings.ToLower(strings.TrimSpace(id))
	for i := range l.Chapters {
		if strings.ToLower(l.Chapters[i].ID) == key {
			return &l.Chapters[i]
		}
	}
	// 退一步：按章节标题精确匹配（用户可能直接输「水畔」）。
	for i := range l.Chapters {
		if strings.ToLower(l.Chapters[i].Title) == key {
			return &l.Chapters[i]
		}
	}
	return nil
}

func tailDigits(s string) string {
	i := len(s)
	for i > 0 && s[i-1] >= '0' && s[i-1] <= '9' {
		i--
	}
	return s[i:]
}
