package nitro

import (
	"encoding/binary"
	"strings"
	"testing"
)

// ---- 密钥表与 key 派生 ----

func TestGenKeyTableIsPermutation(t *testing.T) {
	seen := map[byte]bool{}
	for _, v := range keyTable {
		if seen[v] {
			t.Fatalf("置换表有重复值 %d", v)
		}
		seen[v] = true
	}
}

func TestEntryKeyMatchesGroundTruth(t *testing.T) {
	// 实测（鬼哭街重制版 cg.npa）：明文名 bg\b_sv往来01.jpg（CP932 16 字节，
	// 求和 1582）+ usz=847950 + cg.npa arcKey → key=40；权威密钥表下 cg.npa 2842/2842 条目全部通过魔数校验。
	plain := []byte{0x62, 0x67, 0x5c, 0x62, 0x5f, 0x73, 0x76, 0x89, 0x9d, 0x97, 0x88, 0x30, 0x31, 0x2e, 0x6a, 0x70, 0x67}
	arcKey := i32Wrap(0x1af58 * 0xacc1)
	k := entryKey(plain, 847950, arcKey)
	if k != 40 {
		t.Fatalf("entryKey 实测不匹配: got %d want 40", k)
	}
}

// ---- 名字解密 ----

func TestDecNameKnown(t *testing.T) {
	// nss.npa 第一条目（index 0）加密名 eaeff2f732fc40494d → 0101h.nss
	raw := []byte{0xea, 0xef, 0xf2, 0xf7, 0x32, 0xfc, 0x40, 0x49, 0x4d}
	arcKey := i32Wrap(0x1af58 * 0x34b00)
	got := string(decName(raw, 0, arcKey))
	if got != "0101h.nss" {
		t.Fatalf("decName = %q, want 0101h.nss", got)
	}
}

// ---- LooksLikeNpa ----

func TestLooksLikeNpa(t *testing.T) {
	head := make([]byte, 41)
	copy(head, "NPA\x01\x00\x00\x00")
	binary.LittleEndian.PutUint32(head[7:], 0x1af58)
	binary.LittleEndian.PutUint32(head[11:], 0x34b00)
	head[15] = 1 // compressed
	head[16] = 1 // encrypted
	binary.LittleEndian.PutUint32(head[17:], 81)
	binary.LittleEndian.PutUint32(head[21:], 0)
	binary.LittleEndian.PutUint32(head[25:], 81)
	binary.LittleEndian.PutUint32(head[37:], 2617)
	if !LooksLikeNpa(head, 628491) {
		t.Fatal("合法头被拒绝")
	}
	if LooksLikeNpa(head, 100) {
		t.Fatal("dirSize 超文件大小应拒绝")
	}
	bad := append([]byte(nil), head...)
	bad[0] = 'X'
	if LooksLikeNpa(bad, 628491) {
		t.Fatal("错误 magic 应拒绝")
	}
}

// ---- NSS 解析（合成样本）----

func makeNSS(speaker, body string) []byte {
	return []byte("scene 0103.nss {\r\n" +
		"\tOnBG(10,\"B_テスト背景\");\r\n" +
		"\tCreateTextureEX(\"絵１\", 200, Center, InBottom, \"cg/bg/B_テスト背景.jpg\");\r\n" +
		"\tCreateTextureEX(\"立１\", 202, InLeft, 0, \"cg/st/l/stテスト_l.png\");\r\n" +
		"\tSoundPlay(\"@m07\",0,450,true);\r\n" +
		"<PRE @box0>\r\n" +
		"[text0010]\r\n" +
		"//【" + speaker + "】\r\n" +
		"<voice name=\"" + speaker + "\" class=\"" + speaker + "\" src=\"voice/01/0300010b01\">\r\n" +
		"　<RUBY text=\"テスト\">試験</RUBY>の正文。<k>つづき。\r\n" +
		"</PRE>\r\n" +
		"\tSetText();\r\n" +
		"\tTypeBegin(0);\r\n" +
		"}\r\n")
}

func TestParseNSSJapaneseCP(t *testing.T) {
	enc, err := cp932Encode(string(makeNSS("テスト男", "ダミー")))
	if err != nil {
		t.Skipf("CP932 编码不可用: %v", err)
	}
	data := enc
	s, err := ParseNSS(data, "0103.nss")
	if err != nil {
		t.Fatal(err)
	}
	if s.TextCP != 932 {
		t.Fatalf("TextCP = %d, want 932", s.TextCP)
	}
	var texts, imgs, voices, bgms int
	for _, seg := range s.Segs {
		switch seg.Kind {
		case SegText:
			texts++
			if seg.Speaker != "テスト男" {
				t.Fatalf("说话人 = %q", seg.Speaker)
			}
			if !strings.Contains(seg.Text, "試験") || strings.Contains(seg.Text, "RUBY") {
				t.Fatalf("正文清洗异常: %q", seg.Text)
			}
		case SegImage:
			imgs++
		case SegVoice:
			voices++
			if seg.Ref != "voice/01/0300010b01" {
				t.Fatalf("voice ref = %q", seg.Ref)
			}
		case SegBGM:
			bgms++
			if seg.Ref != "bgm/m07.ogg" {
				t.Fatalf("bgm ref = %q", seg.Ref)
			}
		}
	}
	if texts != 1 || imgs != 3 || voices != 1 || bgms != 1 {
		t.Fatalf("段统计: text=%d img=%d voice=%d bgm=%d", texts, imgs, voices, bgms)
	}
}

func TestParseNSSGBKChinese(t *testing.T) {
	// GBK 编码的中文正文（汉化版 nss-1.npa 形态）：正文无假名 → 判 936。
	src := "scene 0103.nss {\r\n" +
		"<PRE @box0>\r\n" +
		"[text0010]\r\n" +
		"　这是一段中文正文。\r\n" +
		"</PRE>\r\n" +
		"\tSetText();\r\n" +
		"}\r\n"
	enc, err := gbkEncode(src)
	if err != nil {
		t.Skipf("GBK 编码不可用: %v", err)
	}
	s, err := ParseNSS(enc, "0103.nss")
	if err != nil {
		t.Fatal(err)
	}
	if s.TextCP != 936 {
		t.Fatalf("TextCP = %d, want 936", s.TextCP)
	}
	found := false
	for _, seg := range s.Segs {
		if seg.Kind == SegText && strings.Contains(seg.Text, "这是一段中文正文") {
			found = true
		}
	}
	if !found {
		t.Fatal("GBK 正文未解析出")
	}
}

// ---- BuildIR 基本形状 ----

func TestBuildIRBasic(t *testing.T) {
	enc, err := cp932Encode(string(makeNSS("テスト男", "ダミー")))
	if err != nil {
		t.Skipf("CP932 编码不可用: %v", err)
	}
	s, err := ParseNSS(enc, "0103.nss")
	if err != nil {
		t.Fatal(err)
	}
	ad := &Adapter{}
	irObj, err := ad.BuildIR(s, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(irObj.Events) != 1 {
		t.Fatalf("事件数 = %d, want 1", len(irObj.Events))
	}
	ev := irObj.Events[0]
	if ev.EventID != "0103#001" {
		t.Fatalf("EventID = %q", ev.EventID)
	}
	if ev.Kind != "scene" {
		t.Fatalf("Kind = %q", ev.Kind)
	}
	if len(ev.Texts) != 1 || ev.Texts[0].Speaker != "テスト男" {
		t.Fatalf("Texts 异常: %+v", ev.Texts)
	}
	if len(ev.Layers) == 0 || ev.Layers[0].Kind != "background" {
		t.Fatalf("背景层缺失: %+v", ev.Layers)
	}
	hasClear := false
	for _, op := range ev.Show {
		if op.Kind == "text_clear" {
			hasClear = true
		}
	}
	if !hasClear {
		t.Fatal("Show 里没有 text_clear")
	}
	if len(ev.Characters) != 1 || ev.Characters[0] != "テスト男" {
		t.Fatalf("Characters = %v", ev.Characters)
	}
}
