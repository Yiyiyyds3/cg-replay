package main

// cgstory_test.go —— CG「剧情模式」关联与回退的单测。
//
// 用合成剧本复刻本作的真实结构：
//   - cgrai00.ks 的 *cgNN 段只有图片（画廊）；
//   - 主线 71kaigan01.ks 的 *labelNN|标题 章节里有同一张图 + 台词 + 语音。
// 期望：能关联到章节 -> 走剧情模式；关联不到 -> 回退纯图并如实标注「无台词数据」。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ucgr/internal/adapter"
	"ucgr/internal/adapter/kirikiri"
	"ucgr/internal/library"
)

// writeKSScript 在解包树里写一个剧本文件（自动建目录）。
func writeKSScript(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("建目录: %v", err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("写剧本 %s: %v", rel, err)
	}
}

// galleryScript 画廊索引：每个 *cgNN 段一张图 + 转场 + 点击等待（无台词无语音）。
const galleryScript = `*cg17
[image layer=base storage="kuukou01" page=back visible=false]
[trans time=200]
[wt]
[l]

*cg18
[image layer=base storage="mina02" page=back visible=false]
[trans time=200]
[wt]
[l]
`

// storyScript 主线：机场章节，同一张图 + 台词 + 语音。
const storyScript = `*label12|机场的 CG
[image layer=base storage="kuukou01" page=back visible=false]
[trans time=200]
[wt]
[台詞 name="竜"]
　机场的天空蓝得刺眼。[l]
[vo file="v017"]
　「走吧。」[l]

*label13|之后
[cm]
　另一段。[l]
`

// cgFixture 造一份最小解包树 + 库清单。
func cgFixture(t *testing.T) (root string, lib *library.Library, cg *library.CGEvent) {
	t.Helper()
	root = t.TempDir()
	writeKSScript(t, root, "raw/data/scenario/cgrai00.ks", galleryScript)
	writeKSScript(t, root, "raw/data/scenario/71kaigan01.ks", storyScript)
	lib = &library.Library{
		GeneratedAt: "test-1",
		Scenes: []library.SceneEvent{
			{ID: "71kaigan01", Title: "机场", Script: "raw/data/scenario/71kaigan01.ks"},
		},
	}
	cg = &library.CGEvent{
		ID:     "cg017",
		Index:  17,
		Title:  "cg017 · kuukou01",
		Thumb:  "cg017",
		Script: "raw/data/scenario/cgrai00.ks",
		Label:  "cg17",
		Pages:  []string{"kuukou01"},
	}
	return root, lib, cg
}

// TestResolveCGStoryFindsChapter：CG 页图在主线的章节里 -> 关联到该章节。
func TestResolveCGStoryFindsChapter(t *testing.T) {
	root, lib, cg := cgFixture(t)
	link, err := resolveCGStory(root, lib, cg)
	if err != nil {
		t.Fatalf("resolveCGStory: %v", err)
	}
	if link == nil {
		t.Fatal("没关联到剧情章节（期望命中 71kaigan01 的 *label12|机场的 CG）")
	}
	if link.Chapter != "机场的 CG" {
		t.Errorf("Chapter = %q，期望 机场的 CG", link.Chapter)
	}
	if link.Script != "raw/data/scenario/71kaigan01.ks" {
		t.Errorf("Script = %q", link.Script)
	}
	if link.Page != "kuukou01" {
		t.Errorf("Page = %q，期望 kuukou01", link.Page)
	}
}

// TestBuildCGStoryEventKeepsTextAndVoice：剧情台本必须带台词与语音 op。
func TestBuildCGStoryEventKeepsTextAndVoice(t *testing.T) {
	root, lib, cg := cgFixture(t)
	link, err := resolveCGStory(root, lib, cg)
	if err != nil || link == nil {
		t.Fatalf("resolveCGStory: link=%v err=%v", link, err)
	}
	ev, err := buildCGStoryEvent(root, link)
	if err != nil {
		t.Fatalf("buildCGStoryEvent: %v", err)
	}
	if n := countShowOps(ev, "text"); n == 0 {
		t.Fatalf("剧情台本没有台词（show=%d 条）", len(ev.Show))
	}
	if n := countShowOps(ev, "voice_play"); n != 1 {
		t.Errorf("语音条数 = %d，期望 1", n)
	}
	d := dialogForStory(link, ev)
	if !d.Available || d.TextLines == 0 || d.VoiceLines != 1 {
		t.Errorf("Dialogue = %+v，期望 available/text>0/voice=1", d)
	}
}

// TestCGStoryFallbackWhenUnreferenced：图没在主线出现 -> 关联不到（回退纯图）。
func TestCGStoryFallbackWhenUnreferenced(t *testing.T) {
	root, lib, cg := cgFixture(t)
	cg.Pages = []string{"nowhere99"}
	link, err := resolveCGStory(root, lib, cg)
	if err != nil {
		t.Fatalf("resolveCGStory: %v", err)
	}
	if link != nil {
		t.Fatalf("不该关联到章节：%+v", link)
	}
}

// TestApplyCGStoryFallbackNote：没有剧情时 applyCGStory 不改台本、给出原因。
func TestApplyCGStoryFallbackNote(t *testing.T) {
	root, lib, cg := cgFixture(t)
	cg.Pages = []string{"nowhere99"}
	gallery, err := kirikiri.BuildShowSegment(filepath.Join(root, filepath.FromSlash(cg.Script)), cg.Label)
	if err != nil {
		t.Fatalf("构建画廊台本: %v", err)
	}
	if countShowOps(gallery, "text") != 0 {
		t.Fatalf("画廊台本不该有台词（show=%d）", len(gallery.Show))
	}
	ev, d := applyCGStory(gallery, root, lib, cg)
	if ev != gallery {
		t.Errorf("没有剧情时不该替换台本")
	}
	if d == nil || d.Available {
		t.Fatalf("Dialogue = %+v，期望 available=false", d)
	}
	if !strings.Contains(d.Note, "无台词数据") {
		t.Errorf("Note = %q，期望包含「无台词数据」", d.Note)
	}
}

// TestResolveCGStorySkipsEarlierSilentReuse：同一剧本里先出现一次无台词纯图，
// 后面的正式章节仍要能被关联到（不能只看第一处就走）。
func TestResolveCGStorySkipsEarlierSilentReuse(t *testing.T) {
	root, lib, cg := cgFixture(t)
	script := `*flash
[image layer=base storage="kuukou01"]
[l]

*label12|机场的 CG
[image layer=base storage="kuukou01"]
[台詞 name="竜"]
　正式演出。[l]
`
	writeKSScript(t, root, "raw/data/scenario/71kaigan01.ks", script)

	link, err := resolveCGStory(root, lib, cg)
	if err != nil {
		t.Fatalf("resolveCGStory: %v", err)
	}
	if link == nil {
		t.Fatal("漏掉了章内第二处引用")
	}
	if link.Chapter != "机场的 CG" {
		t.Errorf("Chapter = %q，期望命中后面的正式章节", link.Chapter)
	}
	if link.Candidates != 2 {
		t.Errorf("Candidates = %d，期望 2（两次图片引用）", link.Candidates)
	}
}

// TestCGRefPrefixMatch：主线用差分图名（hebi01a）也要能挂到画廊页（hebi01）。
func TestCGRefPrefixMatch(t *testing.T) {
	want := map[string]bool{"hebi01": true}
	if !cgRefPrefixMatch(want, "hebi01a") {
		t.Error("hebi01a 应能前缀匹配 hebi01")
	}
	if cgRefPrefixMatch(want, "hebi02") {
		t.Error("hebi02 不该匹配 hebi01")
	}
	if cgRefPrefixMatch(map[string]bool{"cg1": true}, "cg12345") {
		t.Error("过短的页名不该参与前缀匹配")
	}
}

// TestBuildCGEventPrefersStory：完整入口 buildCGEvent 应返回剧情台本 + 鉴赏台本两份。
func TestBuildCGEventPrefersStory(t *testing.T) {
	root, lib, cg := cgFixture(t)
	primary, gallery, d, err := buildCGEvent(root, "kirikiri", adapter.DefaultKiriKiri(), lib, cg)
	if err != nil {
		t.Fatalf("buildCGEvent: %v", err)
	}
	if gallery == nil || primary == nil {
		t.Fatal("两个台本都要有（剧情 + 鉴赏）")
	}
	if primary == gallery {
		t.Fatal("有剧情数据时主台本应换成剧情章节")
	}
	if d == nil || !d.Available {
		t.Fatalf("Dialogue = %+v，期望可用", d)
	}
	if countShowOps(primary, "text") == 0 {
		t.Error("剧情台本没有台词")
	}
}
