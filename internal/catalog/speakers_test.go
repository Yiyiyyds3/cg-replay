package catalog

import (
	"testing"
)

func TestSpeakers(t *testing.T) {
	c, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer c.Close()
	if err := c.SyncGame(testGame(), testLibrary()); err != nil {
		t.Fatalf("SyncGame: %v", err)
	}

	// 无台词时返回空（不是报错）——前端据此显示「台词未抽取」。
	got, err := c.Speakers("", 0)
	if err != nil {
		t.Fatalf("Speakers empty: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("空库说话人 = %+v, want 空", got)
	}

	lines := []TextLine{
		{Kind: "cg", EventID: "cg001", Speaker: "少女", Text: "一"},
		{Kind: "cg", EventID: "cg001", Speaker: "少女", Text: "二"},
		{Kind: "cg", EventID: "cg002", Speaker: "少年", Text: "三"},
		{Kind: "cg", EventID: "cg002", Speaker: "", Text: "旁白不计数"}, // 空 speaker 排除
		{Kind: "scene", EventID: "01kuukou", Speaker: "少女", Text: "四"},
	}
	if err := c.ReplaceTextLines(testGame().ID, "", "", lines); err != nil {
		t.Fatalf("ReplaceTextLines: %v", err)
	}

	all, err := c.Speakers("", 0)
	if err != nil {
		t.Fatalf("Speakers: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("len(speakers)=%d, want 2（少女/少年，空 speaker 排除）(%+v)", len(all), all)
	}
	// 按条数降序：少女 3 条 / 2 事件；少年 1 条 / 1 事件。
	if all[0].Name != "少女" || all[0].Lines != 3 || all[0].Events != 2 {
		t.Fatalf("top speaker = %+v, want 少女 3 条 2 事件", all[0])
	}
	if all[1].Name != "少年" || all[1].Lines != 1 || all[1].Events != 1 {
		t.Fatalf("second speaker = %+v, want 少年 1 条 1 事件", all[1])
	}
	if all[0].GameID != testGame().ID || all[0].GameTitle != testGame().Title {
		t.Fatalf("speaker 游戏字段 = %+v", all[0])
	}

	// 游戏过滤：不存在的游戏返回空。
	none, err := c.Speakers("gnotexist00", 0)
	if err != nil || len(none) != 0 {
		t.Fatalf("不存在的游戏应返回空：%+v err=%v", none, err)
	}

	// limit 生效。
	one, err := c.Speakers("", 1)
	if err != nil || len(one) != 1 || one[0].Name != "少女" {
		t.Fatalf("limit=1 = %+v err=%v", one, err)
	}

	// 第二款游戏：跨游戏聚合时两款都在，且各自条数独立。
	g2 := testGame()
	g2.ID, g2.Title = "gtest000002", "第二作"
	if err := c.SyncGame(g2, testLibrary()); err != nil {
		t.Fatalf("SyncGame g2: %v", err)
	}
	if err := c.ReplaceTextLines(g2.ID, "cg", "cg001", []TextLine{
		{Kind: "cg", EventID: "cg001", Speaker: "少女", Text: "他作的她"},
	}); err != nil {
		t.Fatalf("ReplaceTextLines g2: %v", err)
	}
	both, _ := c.Speakers("", 0)
	if len(both) != 3 {
		t.Fatalf("跨游戏 len=%d, want 3（少女×2 款 + 少年）(%+v)", len(both), both)
	}
	// 条数相同时按名称排序稳定：两款「少女」都是 1/3……本例 g2 少女 1 条排最后。
	names := []string{both[0].Name, both[1].Name, both[2].Name}
	if names[0] != "少女" || names[1] != "少女" || names[2] != "少年" {
		t.Fatalf("跨游戏排序 = %+v", names)
	}
	// 单游戏过滤仍只返回该游戏。
	g1Only, _ := c.Speakers(testGame().ID, 0)
	if len(g1Only) != 2 {
		t.Fatalf("单游戏过滤 len=%d, want 2", len(g1Only))
	}
}

func TestSpeakersRealCorpus(t *testing.T) {
	root := getenvBlueRoot()
	if root == "" {
		t.Skip("UCGR_BLUE_ROOT 未设置")
	}
	c, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer c.Close()
	// 复用 text_corpus_test 的抽取结果：直接跑一遍索引（幂等）。
	indexBlueWhale(t, c)

	all, err := c.Speakers("", 0)
	if err != nil {
		t.Fatalf("Speakers: %v", err)
	}
	if len(all) == 0 {
		t.Fatal("蓝鲸岛应有说话人（text_lines 已入库）")
	}
	// 条数降序 + 每条都有游戏标题。
	for i := 1; i < len(all); i++ {
		if all[i-1].Lines < all[i].Lines {
			t.Fatalf("未按条数降序：%+v", all[i-1])
		}
		if all[i].GameTitle == "" {
			t.Fatalf("说话人缺少游戏标题：%+v", all[i])
		}
	}
	t.Logf("说话人 %d 位；top1 = %s（%d 条 / %d 事件）",
		len(all), all[0].Name, all[0].Lines, all[0].Events)
	if all[0].Lines < 100 {
		t.Fatalf("top 说话人条数 %d 偏少（蓝鲸岛 47k 台词，主角应在千条级）", all[0].Lines)
	}
}
