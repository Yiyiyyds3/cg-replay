package wolf

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ucgr/internal/ir"
)

// synthCommonEvents 构造最小 CommonEvent.dat：2 个事件（1 剧情 + 1 系统）。
func synthCommonEvents(t *testing.T) []byte {
	t.Helper()
	var b []byte
	put := func(v []byte) { b = append(b, v...) }
	put32 := func(v uint32) {
		var x [4]byte
		binary.LittleEndian.PutUint32(x[:], v)
		put(x[:])
	}
	putStr := func(s string) {
		put32(uint32(len(s) + 1))
		put([]byte(s))
		put([]byte{0})
	}
	put([]byte{0}) // indicator
	put(magicCommonEvent[:])
	put([]byte{0x90}) // version（非 v3.5）
	put32(2)          // 事件数

	// CE1：剧情事件（1 条 Message + 1 条 Picture）
	put([]byte{0x8E})
	put32(1)               // intId
	put32(0)               // unknown1
	put(make([]byte, 7))   // unknown2
	putStr("善第1章")       // name
	put32(2)               // 命令数
	// Message(101)：0 个整数参数 + 1 个字符串参数
	put([]byte{1})
	put32(101)
	put([]byte{0}) // indent
	put([]byte{1}) // strCount
	putStr("\\cdb[0:1:0]\n「せーので、行こう。」")
	put([]byte{0}) // terminator
	// Picture(150)：args[0]=0x00（类型 0=file）、args[1]=槽位 3
	put([]byte{3})
	put32(150)
	put32(0)
	put32(3)
	put([]byte{0}) // indent
	put([]byte{1})
	putStr("CG/ev_test_1.png")
	put([]byte{0})
	// 收尾段
	putStr("") // unknown11
	putStr("事件说明") // description
	put([]byte{0x8F})
	put32(0) // strings
	put32(0) // bytes
	put32(0) // string 串
	put32(0) // int 串
	put(make([]byte, 0x1D))
	for i := 0; i < 100; i++ {
		putStr("")
	}
	put([]byte{0x91})
	putStr("")
	put([]byte{0x91}) // 无 0x92 尾段

	// CE2：系统事件（无 Message，应被过滤）
	put([]byte{0x8E})
	put32(2)
	put32(0)
	put(make([]byte, 7))
	putStr("○システム")
	put32(0)
	putStr("")
	putStr("システム")
	put([]byte{0x8F})
	put32(0)
	put32(0)
	put32(0)
	put32(0)
	put(make([]byte, 0x1D))
	for i := 0; i < 100; i++ {
		putStr("")
	}
	put([]byte{0x91})
	putStr("")
	put([]byte{0x91})

	put([]byte{0x89}) // terminator
	return b
}

func TestSynthCommonEvents(t *testing.T) {
	ce, err := parseCommonEvents(synthCommonEvents(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(ce.Events) != 2 {
		t.Fatalf("事件数 = %d", len(ce.Events))
	}
	if ce.Events[0].Name != "善第1章" || len(ce.Events[0].Commands) != 2 {
		t.Fatalf("CE1 解析不符：%+v", ce.Events[0])
	}
	evs, skipped, err := ce.SceneEvents()
	if err != nil {
		t.Fatal(err)
	}
	if skipped != 1 || len(evs) != 1 {
		t.Fatalf("场景过滤不符：evs=%d skipped=%d", len(evs), skipped)
	}
	ev := evs[0]
	if ev.EventID != "CE001" || ev.Title != "善第1章" {
		t.Fatalf("EventID/Title = %s/%s", ev.EventID, ev.Title)
	}
	if len(ev.Texts) != 1 || !strings.Contains(ev.Texts[0].Text, "せーので") {
		t.Fatalf("台词不符：%+v", ev.Texts)
	}
	// Picture → 图层指令
	var hasPic bool
	for _, op := range ev.Show {
		if op.Kind == ir.ShowLayerShow && op.Ref == "CG/ev_test_1.png" {
			hasPic = true
		}
	}
	if !hasPic {
		t.Fatal("Picture 未映射为图层指令")
	}
	// 首条 Message 前应有 MessageBoxOps（layer_opt + layer_attr）
	if len(ev.Show) < 4 || ev.Show[0].Kind != ir.ShowLayerOpt {
		t.Fatal("对话框几何缺失")
	}
}

func TestSynthCorrupt(t *testing.T) {
	base := synthCommonEvents(t)
	cases := map[string][]byte{
		"huge-count": func() []byte {
			b := append([]byte{}, base[:11]...) // indicator+magic+version
			var x [4]byte
			binary.LittleEndian.PutUint32(x[:], 0xFFFFFFF0)
			return append(b, x[:]...)
		}(),
		"truncated": base[:len(base)/2],
		"bad-magic": func() []byte {
			b := append([]byte{}, base...)
			b[1] = 0x99
			return b
		}(),
	}
	for name, data := range cases {
		ce, err := parseCommonEvents(data)
		if err == nil {
			t.Logf("%s: 恰好完整（%d 事件），不 panic 即可", name, len(ce.Events))
			continue
		}
		if ce != nil {
			t.Errorf("%s: 出错时不应返回半成品", name)
		}
	}
}

// ---- 真实语料 ----

const thanatosDir = `E:\Personal_entertainment\game\zfsgalgame\塔纳托斯-Thanatos`

func thanatosDataDir(t *testing.T) string {
	t.Helper()
	if d := os.Getenv("UCGR_THANATOS_DIR"); d != "" {
		return d
	}
	if LooksLikeWolfDir(thanatosDir) {
		return thanatosDir
	}
	t.Skip("塔纳托斯语料不可用")
	return ""
}

func TestRealWolfParse(t *testing.T) {
	dir := thanatosDataDir(t)
	ce, err := ParseCommonEventFile(filepath.Join(dir, "data", "BasicData", "CommonEvent.dat"))
	if err != nil {
		t.Fatal(err)
	}
	if len(ce.Events) < 400 {
		t.Fatalf("事件数 = %d（期望 500+）", len(ce.Events))
	}
	evs, skipped, err := ce.SceneEvents()
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) < 100 {
		t.Fatalf("剧情事件 = %d（期望 100+），跳过 %d", len(evs), skipped)
	}
	// 抽一个章节 CE：善第1章
	var found *ir.Event
	for i := range evs {
		if evs[i].Title == "善第1章" {
			found = &evs[i]
			break
		}
	}
	if found == nil {
		t.Fatal("没找到「善第1章」")
	}
	if len(found.Texts) < 100 {
		t.Fatalf("善第1章台词 = %d（期望 100+）", len(found.Texts))
	}
	t.Logf("剧情事件 %d 个（跳过系统 CE %d）；善第1章 %d 条台词、%d 条 Show 指令",
		len(evs), skipped, len(found.Texts), len(found.Show))
}

func TestRealWolfIdentify(t *testing.T) {
	dir := thanatosDataDir(t)
	id, err := (&Adapter{}).Identify(dir)
	if err != nil {
		t.Fatal(err)
	}
	if id != "wolf" {
		t.Fatalf("Identify = %q", id)
	}
}
