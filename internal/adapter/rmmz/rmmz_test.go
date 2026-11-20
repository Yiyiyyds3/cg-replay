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

func TestScriptCGName(t *testing.T) {
	cases := []struct {
		code int
		str  string
		want string
	}{
		{355, `this.cgLoad('pic001', 0);`, "pic001"},
		{655, `this.cgLoad("pic018", 2);`, "pic018"},
		{355, `this.picsOut(30);`, ""},
		{231, `this.cgLoad('pic001', 0);`, ""}, // 231 不走脚本 CG
		{401, `this.cgLoad('pic001', 0);`, ""},
	}
	for _, tc := range cases {
		c := Command{Code: tc.code, Params: []any{tc.str}}
		if got := scriptCGName(c); got != tc.want {
			t.Errorf("scriptCGName(code=%d) = %q, want %q", tc.code, got, tc.want)
		}
	}
}

func TestLocateCGSet(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "www", "img", "pictures", "pic001"), 0o755)
	os.WriteFile(filepath.Join(root, "www", "img", "pictures", "pic001", "1_1.rpgmvp"), []byte("x"), 0o644)

	rel, ok := locateCGSet(root, []string{"www/img"}, "pic001")
	if !ok || rel != "www/img/pictures/pic001/1_1.rpgmvp" {
		t.Errorf("locateCGSet = %q,%v", rel, ok)
	}
	if _, ok := locateCGSet(root, []string{"www/img"}, "pic999"); ok {
		t.Error("不存在的组不应命中")
	}
}

func TestSegmentsScriptCG(t *testing.T) {
	// Wolfzq 风格：脚本 cgLoad 开段，picsChange2 中段，picsOut 清空结束。
	cmds := []Command{
		{Code: 101, Params: []any{"", 0}},
		{Code: 401, Params: []any{"这是对话"}},
		{Code: 355, Params: []any{`this.cgLoad('pic001', 0);`}},
		{Code: 655, Params: []any{`this.picsChange2();`}},
		{Code: 401, Params: []any{"差分切换后的对话"}},
		{Code: 355, Params: []any{`this.picsOut(30);`}},
		{Code: 401, Params: []any{"收尾"}},
	}
	segs := Segments(cmds)
	if len(segs) != 1 {
		t.Fatalf("Segments = %d 段, want 1", len(segs))
	}
	hasCG := false
	for _, c := range segs[0] {
		if scriptCGName(c) == "pic001" {
			hasCG = true
		}
	}
	if !hasCG {
		t.Error("段落里应包含 cgLoad('pic001') 指令")
	}
}

// TestScanScriptDrivenDiagnostic 图片由插件函数（code 355）驱动、完全没有
// Show Picture（code 231）的游戏，要给出可行动的诊断而不是笼统的「无图片演出」。
func TestScanScriptDrivenDiagnostic(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mf := `{"displayName":"测试地图","events":[{"id":1,"name":"事件","pages":[{"list":[` +
		`{"code":355,"indent":0,"parameters":["this.picsChange2();"]},` +
		`{"code":355,"indent":0,"parameters":["this.picsDel();"]},` +
		`{"code":401,"indent":0,"parameters":["台词"]}` +
		`]}]}]}`
	if err := os.WriteFile(filepath.Join(dataDir, "Map001.json"), []byte(mf), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Scan(root, "data")
	if err == nil {
		t.Fatal("插件驱动型游戏应返回明确诊断，不能静默当成空库")
	}
	for _, want := range []string{"插件/脚本驱动", "picsChange2"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("诊断信息缺少 %q：%v", want, err)
		}
	}
}

// TestScanShowPictureUnaffected 有 Show Picture 的游戏不受新诊断影响，
// 且 ScanResult 会回填统计字段。
func TestScanShowPictureUnaffected(t *testing.T) {
	root := t.TempDir()
	picDir := filepath.Join(root, "img", "pictures")
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
	dataDir := filepath.Join(root, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mf := `{"displayName":"测试地图","events":[{"id":1,"name":"事件","pages":[{"list":[` +
		`{"code":231,"indent":0,"parameters":[1,"a",0,0,0,100,100,255,0]},` +
		`{"code":401,"indent":0,"parameters":["台词"]},` +
		`{"code":235,"indent":0,"parameters":[0]}` +
		`]}]}]}`
	if err := os.WriteFile(filepath.Join(dataDir, "Map001.json"), []byte(mf), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Scan(root, "data")
	if err != nil {
		t.Fatalf("正常游戏不应报错: %v", err)
	}
	if res.ShowPictureCommands != 1 {
		t.Fatalf("ShowPictureCommands = %d, want 1", res.ShowPictureCommands)
	}
	if len(res.Refs) != 1 {
		t.Fatalf("Refs = %d, want 1", len(res.Refs))
	}
}

// TestScriptPictureNames 脚本实参里的图片名提取：认出图片函数里的字符串字面量，
// 放过变量、表达式拼接、本地化占位与普通台词。
func TestScriptPictureNames(t *testing.T) {
	cases := []struct {
		code int
		str  string
		want []string
	}{
		{355, `$gameScreen.showPicture(10, "CG07_01", 0, 0, 0, 100, 100, 255, 0);`, []string{"CG07_01"}},
		{355, `$gameScreen.showPicture(20, 'CG07_02', 0, 0, 0, 100, 100, 0, 0);`, []string{"CG07_02"}},
		{655, `  $gameScreen.showPicture(5, '差分_1', 1, 400, 300, 100, 100, 255, 0);`, []string{"差分_1"}},
		{355, `this.picsChange2('立绘 02');`, []string{"立绘 02"}},
		{355, `this.loadPicture('CG/03');`, []string{"CG/03"}},
		// 下面这些都不是「静态可读的图片名」。
		{355, `$gameScreen.showPicture(picId, picName, 0, 0, 0, 100, 100, 255, 0);`, nil},
		{355, `  $gameScreen.showPicture(FRONT, 'stands/' + raw, 1, x, y, sx, 100, 0, 0);`, nil},
		{355, `$utils.playHint('#{hintClothes}');`, nil},
		{355, `$gameTemp._sysMsgQ.push(['体力が' + gained + '上昇しました']);`, nil},
		{401, `$gameScreen.showPicture(1, '不算', 0);`, nil}, // 不是脚本指令
	}
	for _, tc := range cases {
		got := ScriptPictureNames(Command{Code: tc.code, Params: []any{tc.str}})
		if strings.Join(got, "\x00") != strings.Join(tc.want, "\x00") {
			t.Errorf("ScriptPictureNames(code=%d, %q) = %v, want %v", tc.code, tc.str, got, tc.want)
		}
	}
}

// TestScriptPictureShowsKeepArgs 展开出来的 231 要沿用 showPicture 的位置参数
// （编号、原点、坐标、缩放、透明度），否则回放里图片会摆错位置。
func TestScriptPictureShowsKeepArgs(t *testing.T) {
	c := Command{Code: 355, Params: []any{`$gameScreen.showPicture(10, "CG07_01", 1, 400, 300, 50, 50, 128, 0);`}}
	shows := scriptPictureShows(c)
	if len(shows) != 1 {
		t.Fatalf("scriptPictureShows = %d 条, want 1", len(shows))
	}
	s := shows[0]
	if s.Code != 231 || s.str(1) != "CG07_01" {
		t.Fatalf("展开结果 code=%d name=%q", s.Code, s.str(1))
	}
	if int(s.num(0)) != 10 || int(s.num(2)) != 1 || s.num(4) != 400 || s.num(5) != 300 ||
		s.num(6) != 50 || s.num(8) != 128 {
		t.Fatalf("参数没带过来：%v", s.Params)
	}
}

// TestExpandScriptPicturesIDs 调用里不带编号时，展开出来的 231 要拿到唯一编号，
// 免得同一段里多张图互相覆盖。
func TestExpandScriptPicturesIDs(t *testing.T) {
	cmds := []Command{
		{Code: 355, Params: []any{`this.picsChange2('a_01');`}},
		{Code: 355, Params: []any{`this.picsChange2('a_02');`}},
	}
	out := expandScriptPictures(cmds)
	if len(out) != 2 {
		t.Fatalf("expandScriptPictures = %d 条, want 2", len(out))
	}
	if int(out[0].num(0)) == int(out[1].num(0)) || int(out[0].num(0)) < 900 {
		t.Fatalf("编号应唯一且从 900 起：%v / %v", out[0].Params[0], out[1].Params[0])
	}
}

// TestScanScriptPicturesBecomeEvents 图片名写在脚本实参里的游戏要能扫出演出段，
// 而不是被判成「插件驱动」直接出局；BuildShow 也要能生成同一段落的表现。
func TestScanScriptPicturesBecomeEvents(t *testing.T) {
	root := t.TempDir()
	picDir := filepath.Join(root, "img", "pictures")
	if err := os.MkdirAll(picDir, 0o755); err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 300, 300))
	for y := 0; y < 300; y++ {
		for x := 0; x < 300; x++ {
			img.SetRGBA(x, y, color.RGBA{10, 120, 200, 255})
		}
	}
	f, err := os.Create(filepath.Join(picDir, "cg01.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()
	dataDir := filepath.Join(root, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mf := `{"displayName":"脚本图地图","events":[{"id":1,"name":"事件","pages":[{"list":[` +
		`{"code":355,"indent":0,"parameters":["$gameScreen.showPicture(10, \"cg01\", 0, 0, 0, 100, 100, 255, 0);"]},` +
		`{"code":401,"indent":0,"parameters":["台词"]},` +
		`{"code":355,"indent":0,"parameters":["$gameScreen.erasePicture(10);"]}` +
		`]}]}]}`
	if err := os.WriteFile(filepath.Join(dataDir, "Map001.json"), []byte(mf), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Scan(root, "data")
	if err != nil {
		t.Fatalf("脚本图片名可读，不该报插件级诊断: %v", err)
	}
	if res.ScriptPictures != 1 {
		t.Fatalf("ScriptPictures = %d, want 1", res.ScriptPictures)
	}
	if len(res.Refs) != 1 {
		t.Fatalf("Refs = %d, want 1", len(res.Refs))
	}
	if res.Refs[0].Pictures != 1 {
		t.Fatalf("段落图片数 = %d, want 1", res.Refs[0].Pictures)
	}
	ev, err := BuildShow(root, "data", res.Refs[0].Key)
	if err != nil {
		t.Fatalf("BuildShow: %v", err)
	}
	shown := false
	for _, op := range ev.Show {
		if op.Kind == ir.ShowLayerShow && op.Ref == "img/pictures/cg01.png" {
			shown = true
			if op.Layer != "layer10" {
				t.Errorf("层名 = %q, want layer10（沿用 showPicture 的第一个实参）", op.Layer)
			}
		}
	}
	if !shown {
		t.Errorf("BuildShow 没生成 cg01 的图层显示：%+v", ev.Show)
	}
}

