package rmmz

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCleanTextPlaceholders(t *testing.T) {
	c := textCleaner{
		i18n:   map[int]string{13226: "在这座恶魔之塔中"},
		locale: map[string]string{"CName002": "御影"},
	}
	cases := []struct{ in, want string }{
		{`\I18N[13226]`, "在这座恶魔之塔中"},
		{`{CName002}好`, "御影好"},
		{`\N<莉扎西亚>莉扎西亚：住手！`, "莉扎西亚：住手！"},
		{`\C[3]红色\C[0]`, "红色"},
		{`\n换行`, "换行"},
		{`\博尔克绕到身后`, "博尔克绕到身后"},
		{`莉扎西亚🟥照你说的做了`, "莉扎西亚照你说的做了"},
		{`\{字面括号\}`, "字面括号"},
	}
	for _, tc := range cases {
		if got := c.clean(tc.in); got != tc.want {
			t.Errorf("clean(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	if got := CleanText(`\N[1]你好`); got != "你好" {
		t.Errorf("CleanText = %q, want 你好", got)
	}
}

func TestWrapText(t *testing.T) {
	got := WrapText("一二三四五六七八九十", 4)
	want := []string{"一二三四", "五六七八", "九十"}
	if len(got) != len(want) {
		t.Fatalf("WrapText = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("WrapText[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	if got := WrapText("第一行\n第二行", 28); len(got) != 2 || got[0] != "第一行" || got[1] != "第二行" {
		t.Fatalf("换行未切开: %#v", got)
	}
}

func TestSegments(t *testing.T) {
	cmds := []Command{
		{Code: 231, Params: []any{1.0, "a"}},
		{Code: 401, Params: []any{"一"}},
		{Code: 235},
		{Code: 401, Params: []any{"画外"}},
		{Code: 231, Params: []any{2.0, "b"}},
		{Code: 401, Params: []any{"二"}},
	}
	segs := Segments(cmds)
	if len(segs) != 2 {
		t.Fatalf("段落数 = %d, want 2", len(segs))
	}
	if len(segs[0]) != 3 || len(segs[1]) != 2 {
		t.Fatalf("段落长度 = %d,%d，want 3,2", len(segs[0]), len(segs[1]))
	}
}

// TestLocatePictureJPFallback DLsite 审查版只保留马赛克变体（<名>JP.png）时，
// 仍要能定位到实际文件；原名存在时优先原名。
func TestLocatePictureJPFallback(t *testing.T) {
	root := t.TempDir()
	picDir := filepath.Join(root, "img", "pictures", "CG")
	if err := os.MkdirAll(picDir, 0o755); err != nil {
		t.Fatal(err)
	}
	jp := filepath.Join(picDir, "ChelseaH7_01JP.png")
	if err := os.WriteFile(jp, []byte("jp"), 0o644); err != nil {
		t.Fatal(err)
	}
	rel, ok := LocatePicture(root, []string{"img"}, "CG/ChelseaH7_01")
	if !ok || rel != "img/pictures/CG/ChelseaH7_01JP.png" {
		t.Fatalf("JP 变体定位 = %q,%v，want img/pictures/CG/ChelseaH7_01JP.png,true", rel, ok)
	}

	plain := filepath.Join(picDir, "ChelseaH7_01.png")
	if err := os.WriteFile(plain, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	rel, ok = LocatePicture(root, []string{"img"}, "CG/ChelseaH7_01")
	if !ok || rel != "img/pictures/CG/ChelseaH7_01.png" {
		t.Fatalf("原名优先 = %q,%v，want img/pictures/CG/ChelseaH7_01.png,true", rel, ok)
	}
}
