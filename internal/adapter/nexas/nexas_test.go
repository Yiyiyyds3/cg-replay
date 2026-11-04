// nexas_test.go —— 合成用例：Huffman 往返、PAC 往返、脚本解析与分类。
package nexas

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// encodeNeXAS 模拟官方打包器：树位流 + 数据位流，每满 8 位取反成字节。
// （对齐 NexasPackEdit 的 HuffmanEncode.Packbyte 行为。）
type bitWriter struct {
	bits  uint32
	count int
	out   []byte
}

func (w *bitWriter) put(v uint32, n int) {
	for i := n - 1; i >= 0; i-- {
		w.bits = w.bits<<1 | (v>>i)&1
		w.count++
		if w.count == 8 {
			w.out = append(w.out, uint8(^w.bits))
			w.bits = 0
			w.count = 0
		}
	}
}

// encodeIndex 用固定码表把索引编码成 NeXAS Huffman 流。
//
// 字节集限制在 {0x00,0x01,0x02,0x03}，满二叉树（2 内部 + 4 叶子），
// 码长 2 位；树序列化与数据编码同一码表。
func encodeIndex(t *testing.T, index []byte) []byte {
	t.Helper()
	w := &bitWriter{}
	w.put(1, 1) // root
	w.put(1, 1) // root.lhs = 内部
	w.put(0, 1) //   lhs.leaf 0x00
	w.put(0x00, 8)
	w.put(0, 1) //   rhs.leaf 0x01
	w.put(0x01, 8)
	w.put(1, 1) // root.rhs = 内部
	w.put(0, 1) //   lhs.leaf 0x02
	w.put(0x02, 8)
	w.put(0, 1) //   rhs.leaf 0x03
	w.put(0x03, 8)
	codes := map[byte]uint32{0x00: 0b00, 0x01: 0b01, 0x02: 0b10, 0x03: 0b11}
	for _, b := range index {
		if _, ok := codes[b]; !ok {
			t.Fatalf("测试数据含码表外字节 %02x", b)
		}
		w.put(codes[b], 2)
	}
	return w.out
}

// TestHuffmanRoundTrip 往返：编码 → 解码相等。
func TestHuffmanRoundTrip(t *testing.T) {
	// 构造 209 个条目的合成索引（字节集限制在码表内）。
	var index []byte
	for i := 0; i < 209; i++ {
		var ent [76]byte
		// 名字用 0x01 填（表内字节），字段值拆成 0x00/0x01/0x02/0x03 位。
		for j := range ent {
			ent[j] = byte(i%2) + 1 // 0x01 / 0x02
		}
		// offset 等字段编码成低位字节（信息虽退化，往返一致性才是目标）。
		binary.LittleEndian.PutUint32(ent[64:], uint32(12+i*100))
		for j := 64; j < 76; j++ {
			ent[j] &= 3
		}
		index = append(index, ent[:]...)
	}
	packed := encodeIndex(t, index)
	out, err := neXASHuffmanDecode(packed, len(index))
	if err != nil {
		t.Fatalf("解码失败: %v", err)
	}
	// 官方打包器的位流尾部不补齐位（Script.pac 实测差 3 字节）——
	// 测试编码器同样丢尾位，所以断言「解出的前缀完全一致」。
	if len(out) < len(index)-8 {
		t.Fatalf("解出过少: %d / %d", len(out), len(index))
	}
	if !bytes.Equal(out, index[:len(out)]) {
		t.Fatalf("前缀不一致")
	}
}

// TestHuffmanBadInput 非 Huffman 数据应报错（树溢出/耗尽），不 panic。
func TestHuffmanBadInput(t *testing.T) {
	// 全 0x00 取反后是全 1 位流 = 无限递归内部节点 → 256 槽溢出。
	garbage := bytes.Repeat([]byte{0x00}, 512)
	if _, err := neXASHuffmanDecode(garbage, 1000); err == nil {
		t.Fatal("全 1 位流应报错")
	}
	// 短流耗尽。
	if _, err := neXASHuffmanDecode([]byte{0xFF}, 1000); err == nil {
		t.Fatal("耗尽流应报错（除非部分输出）")
	}
}

// TestParseBinChinese 合成 GBK 脚本：说话人段 + 台词段 + 语音号。
func TestParseBinChinese(t *testing.T) {
	// GBK 字面量（手动编码，避免测试依赖编码 API）：
	// 甲   = d0 d7
	// 乙   = d2 d2
	// 台词正文用「你好」= c4 e3 ba c3
	jia := []byte{0xD0, 0xD7}
	hello := []byte{0xC4, 0xE3, 0xBA, 0xC3}
	voice := []byte("@v70000000")
	quote := []byte{0xA1, 0xB0} // 「 在 GBK = A1B0（全角引号）
	close := []byte{0xA1, 0xB1}
	// 头：n=1 → 文本区从 16 字节起。
	head := make([]byte, 16)
	binary.LittleEndian.PutUint64(head, 1)
	var data []byte
	data = append(data, head...)
	data = append(data, jia...)               // 说话人段
	data = append(data, 0x00, 0x00)
	data = append(data, voice...)             // @v 语音
	data = append(data, quote...)
	data = append(data, hello...)
	data = append(data, close...)             // 「你好」
	data = append(data, 0x00, 0x00, 0xFF, 0x10) // 尾部控制区
	s, err := ParseBin(data, "00：test.bin")
	if err != nil {
		t.Fatalf("ParseBin: %v", err)
	}
	if len(s.Segs) != 2 {
		t.Fatalf("段数 = %d, 期望 2", len(s.Segs))
	}
	if s.Segs[0].Kind != SegSpeaker {
		t.Fatalf("段0 类型 = %v, 期望 SegSpeaker", s.Segs[0].Kind)
	}
	if s.Segs[1].Kind != SegText {
		t.Fatalf("段1 类型 = %v, 期望 SegText", s.Segs[1].Kind)
	}
	if s.Segs[1].Voice != "70000000" {
		t.Fatalf("语音号 = %q, 期望 70000000", s.Segs[1].Voice)
	}
	if s.Segs[1].Text == "" {
		t.Fatal("台词文本为空")
	}
}

// TestParseBinBadHeader 头指针越界应报错。
func TestParseBinBadHeader(t *testing.T) {
	head := make([]byte, 8)
	binary.LittleEndian.PutUint64(head, 1<<30)
	if _, err := ParseBin(head, "bad.bin"); err == nil {
		t.Fatal("越界头应报错")
	}
}

// TestClassify 分支/资源/指令分类。
func TestClassify(t *testing.T) {
	cases := []struct {
		text string
		kind SegKind
	}{
		{"03：千夏編01.bin", SegBranch},
		{"GakuenChapterTitleBG01.png", SegAsset},
		{"System24.fnt", SegAsset},
		{"#Walk01_B", SegLabel},
		{"RE_Hit02", SegCmd},
		{"@v10000030「中尉！！」", SegText},
	}
	for _, c := range cases {
		seg := classify(c.text)
		if seg.Kind != c.kind {
			t.Errorf("classify(%q) = %v, 期望 %v", c.text, seg.Kind, c.kind)
		}
	}
}

// TestCleanText 注音剥离：@r須藤@すどう@@r雅@まさ@ → 須藤雅。
func TestCleanText(t *testing.T) {
	in := "隣を歩いていた@r須藤@すどう@@r雅@まさ@が、話しかけてきた。"
	got := cleanText(in)
	want := "隣を歩いていた須藤雅が、話しかけてきた。"
	if got != want {
		t.Fatalf("cleanText = %q, 期望 %q", got, want)
	}
	// @n → 空格；@k/@d 删除。
	if got := cleanText("第一行@n第二行@k"); got != "第一行 第二行" {
		t.Fatalf("cleanText(@n) = %q", got)
	}
}

// TestLzss 字面量 + 窗口引用混合。
func TestLzss(t *testing.T) {
	// 1) 纯字面量：标志 0xFF（8 个 1）+ 8 字节。
	stream := append([]byte{0xFF}, []byte("ABCDEFGH")...)
	if out := lzssDecode(stream, 8); !bytes.Equal(out, []byte("ABCDEFGH")) {
		t.Fatalf("字面量 = %q", out)
	}
	// 2) 字面量 + 引用：先 8 个字面量进窗口（窗口起点 4096-18=4078），
	//    再从 pos=4078 复制 4 字节 → 输出 ABCDEFGHABCD。
	//    引用字节：posLo=0xEE, lenByte=0xF1（pos 高 4 位 0xF + 长度 4-3=1）。
	stream = append([]byte{0xFF}, []byte("ABCDEFGH")...)
	stream = append(stream, 0x00, 0xEE, 0xF1)
	if out := lzssDecode(stream, 12); !bytes.Equal(out, []byte("ABCDEFGHABCD")) {
		t.Fatalf("引用 = %q", out)
	}
}
