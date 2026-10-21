// text_corpus_test.go —— 真机语料 smoke（env-gated）：
// 复现 desktop CatalogIndexText 的抽取管线（Scan → BuildShowSegment/
// BuildChapterEvent → TextLinesFromEvent → ReplaceTextLines），
// 并用一条真实台词回验 FTS 检索。设置 UCGR_BLUE_ROOT 指向
// 「重返蓝鲸岛解包」根目录后运行；未设置则 skip。
package catalog

import (
	"os"
	"path/filepath"
	"testing"

	"ucgr/internal/adapter/kirikiri"
	"ucgr/internal/ir"
	"ucgr/internal/library"
	"ucgr/internal/registry"
)

func TestTextLinesFromRealCorpus(t *testing.T) {
	root := os.Getenv("UCGR_BLUE_ROOT")
	if root == "" {
		t.Skip("UCGR_BLUE_ROOT 未设置")
	}
	lib, err := library.Scan(root)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	g := registry.Game{ID: "gcorpus0001", Title: "重返蓝鲸岛", Root: root, Engine: "kirikiri"}

	c, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer c.Close()
	if err := c.SyncGame(g, lib); err != nil {
		t.Fatalf("SyncGame: %v", err)
	}

	write := func(kind, id, script, label string, ch *library.ChapterEvent) {
		var (
			ev  *ir.Event
			err error
		)
		sp := filepath.Join(root, filepath.FromSlash(script))
		if ch != nil {
			ev, err = library.BuildChapterEvent(root, ch)
		} else {
			ev, err = kirikiri.BuildShowSegment(sp, label)
		}
		if err != nil {
			t.Fatalf("%s %s 台本构建失败：%v", kind, id, err)
		}
		lines := TextLinesFromEvent(ev)
		for i := range lines {
			lines[i].Kind = kind
			lines[i].EventID = id
		}
		if err := c.ReplaceTextLines(g.ID, kind, id, lines); err != nil {
			t.Fatalf("%s %s 写台词失败：%v", kind, id, err)
		}
	}
	for _, cg := range lib.CGs {
		if len(cg.Pages) == 0 {
			continue // 画廊空槽（cg091~cg096），与 desktop CatalogIndexText 同口径
		}
		write("cg", cg.ID, cg.Script, cg.Label, nil)
	}
	for _, s := range lib.Scenes {
		write("scene", s.ID, s.Script, "", nil)
	}
	for i := range lib.Chapters {
		ch := lib.Chapters[i]
		write("chapter", ch.ID, ch.Script, "", &ch)
	}

	s, err := c.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if s.TextLines < 10000 {
		t.Fatalf("台词条数 %d 异常偏少（蓝鲸岛 96 CG + 79 场景 + 115 段落应在万级）", s.TextLines)
	}
	t.Logf("索引台词 %d 条 / 事件 %d 个", s.TextLines, s.Events)

	// 回验：取第一条台词的前 3 个字符做 FTS 查询，必须能搜回它自己。
	rows, err := c.db.Query(`SELECT text FROM text_lines LIMIT 1`)
	if err != nil {
		t.Fatalf("读样本台词: %v", err)
	}
	var sample string
	if rows.Next() {
		_ = rows.Scan(&sample)
	}
	_ = rows.Close()
	if sample == "" {
		t.Fatal("没有抽到任何台词")
	}
	q := []rune(sample)
	need := 3
	if len(q) < need {
		need = len(q)
	}
	hits, err := c.SearchText(string(q[:need]), 0)
	if err != nil {
		t.Fatalf("SearchText(%q): %v", string(q[:need]), err)
	}
	if len(hits) == 0 {
		t.Fatalf("样本台词 %q 的前 %d 字符搜不回来", sample, need)
	}
}
