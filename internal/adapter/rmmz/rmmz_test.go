package rmmz

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ucgr/internal/ir"
)

func TestCleanTextPlaceholders(t *testing.T) {
	c := textCleaner{
		i18n:   map[int]string{13226: "在这座恶魔之塔中"},
		locale: map[string]string{"CName002": "御影"},
	}
	cases := []struct{ in, want string }{
		{`\I18N[13226]`, "在这座恶魔之塔中"},
		{`\I18N[99999]`, "【I18N 99999 缺失】"},
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
	// 翻译表缺失时不能把整行清空，否则扫描阶段会丢掉整个演出段。
	if got := CleanText(`\I18N[123]`); got == "" {
		t.Error("缺翻译表时 \\I18N 占位被清空，演出段会消失")
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

// TestScanDedupesIdenticalPages 同一事件的不同 page 指令完全相同时只登记一次。
func TestScanDedupesIdenticalPages(t *testing.T) {
	root := t.TempDir()
	picDir := filepath.Join(root, "img", "pictures", "CG")
	if err := os.MkdirAll(picDir, 0o755); err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 300, 300))
	for y := 0; y < 300; y++ {
		for x := 0; x < 300; x++ {
			img.SetRGBA(x, y, color.RGBA{200, 100, 50, 255})
		}
	}
	f, err := os.Create(filepath.Join(picDir, "a.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	page := `{"list":[` +
		`{"code":231,"indent":0,"parameters":[1,"CG/a",2,0,0,0,100,100,255]},` +
		`{"code":401,"indent":0,"parameters":["台词"]}]}`
	dataDir := filepath.Join(root, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mf := `{"displayName":"测试地图","events":[{"id":1,"name":"事件","pages":[` + page + `,` + page + `]}]}`
	if err := os.WriteFile(filepath.Join(dataDir, "Map001.json"), []byte(mf), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Scan(root, "data")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Refs) != 1 {
		t.Fatalf("重复 page 登记出 %d 条事件，want 1", len(res.Refs))
	}
	found := false
	for _, w := range res.Warnings {
		if strings.Contains(w, "已跳过重复页") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("没有重复页告警：%#v", res.Warnings)
	}
}

// TestBuildShowMovePictureFades 231 opacity=0 后用 232 移动图片淡入：
// 231 不能被强改成 255，232 要输出带 from/to/duration 的图层属性动画。
func TestBuildShowMovePictureFades(t *testing.T) {
	root := t.TempDir()
	picDir := filepath.Join(root, "img", "pictures", "CG")
	if err := os.MkdirAll(picDir, 0o755); err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 300, 300))
	for y := 0; y < 300; y++ {
		for x := 0; x < 300; x++ {
			img.SetRGBA(x, y, color.RGBA{200, 100, 50, 255})
		}
	}
	f, err := os.Create(filepath.Join(picDir, "a.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	list := `[` +
		`{"code":231,"indent":0,"parameters":[1,"CG/a",2,0,0,0,100,100,0]},` +
		`{"code":232,"indent":0,"parameters":[1,2,0,0,100,100,255,0,30,1]},` +
		`{"code":401,"indent":0,"parameters":["台词"]}]`
	dataDir := filepath.Join(root, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mf := `{"displayName":"测试地图","events":[{"id":1,"name":"事件","pages":[{"list":` + list + `}]}]}`
	if err := os.WriteFile(filepath.Join(dataDir, "Map001.json"), []byte(mf), 0o644); err != nil {
		t.Fatal(err)
	}
	ev, err := BuildShow(root, "data", "map:Map001.json:1:0#0")
	if err != nil {
		t.Fatal(err)
	}
	var show, move *ir.ShowOp
	for i := range ev.Show {
		switch ev.Show[i].Kind {
		case ir.ShowLayerShow:
			show = &ev.Show[i]
		case ir.ShowLayerAttr:
			move = &ev.Show[i]
		}
	}
	if show == nil || show.Attr["opacity"] != "0" {
		t.Fatalf("231 的 opacity=0 被改掉：%#v", show)
	}
	if move == nil {
		t.Fatal("232 没有生成图层属性动画")
	}
	if move.DurMs != 500 || move.Attr["from"] != "0" || move.Attr["to"] != "255" {
		t.Fatalf("232 淡入参数 = dur=%d from=%q to=%q，want 500/0/255",
			move.DurMs, move.Attr["from"], move.Attr["to"])
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
