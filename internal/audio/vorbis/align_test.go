package vorbis

import (
	"os"
	"testing"
)

// TestHeaderBitAlignment 检查三个头包是否被精确读完（framing bit 之后应无剩余位）。
func TestHeaderBitAlignment(t *testing.T) {
	path := os.Getenv("UCGR_OGG")
	if path == "" {
		t.Skip("未设置 UCGR_OGG")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读文件失败：%v", err)
	}
	pages, err := parsePages(data)
	if err != nil {
		t.Fatalf("parsePages: %v", err)
	}
	serial, ok := selectStream(pages)
	if !ok {
		t.Fatal("未找到 vorbis 流")
	}
	pkts := packets(pages, serial)
	t.Logf("总包数 %d", len(pkts))
	for i := 0; i < 3 && i < len(pkts); i++ {
		t.Logf("包#%d 长度 %d 字节 = %d 位", i, len(pkts[i].Data), len(pkts[i].Data)*8)
	}

	// ① identification
	br := newBitReader(pkts[0].Data)
	br.read(8) // packet type
	for i := 0; i < 6; i++ {
		br.read(8) // "vorbis"
	}
	id, err := parseIdentification(br)
	if err != nil {
		t.Fatalf("identification: %v", err)
	}
	t.Logf("identification：%d 声道 %d Hz 块长 %d/%d，剩余位 %d",
		id.Channels, id.SampleRate, id.Blocksize0, id.Blocksize1, br.bitsLeft())

	// ② comment（跳过）
	br2 := newBitReader(pkts[1].Data)
	br2.read(8) // packet type
	for i := 0; i < 6; i++ {
		br2.read(8) // "vorbis"
	}
	vlen := int(br2.read(32))
	br2.skip(vlen * 8)
	cmt := int(br2.read(32))
	for i := 0; i < cmt; i++ {
		l := int(br2.read(32))
		br2.skip(l * 8)
	}
	br2.read(1) // framing
	t.Logf("comment：长度 %d，剩余位 %d（应为 0）", vlen, br2.bitsLeft())

	// ③ setup —— 关键
	br3 := newBitReader(pkts[2].Data)
	br3.read(8) // packet type
	for i := 0; i < 6; i++ {
		br3.read(8)
	}
	t.Logf("setup 头标志后位置 %d 位", br3.pos)
	s, err := parseSetup(br3, id.Channels)
	if err != nil {
		t.Fatalf("parseSetup: %v", err)
	}
	t.Logf("setup：码本 %d floor %d residue %d mapping %d mode %d",
		len(s.codebooks), len(s.floors), len(s.residues), len(s.mappings), len(s.modes))
	t.Logf("setup 消耗 %d / %d 位，剩余 %d 位（应为 0 或极少填充）",
		br3.pos, len(pkts[2].Data)*8, br3.bitsLeft())
	if br3.overrun {
		t.Errorf("setup 解析发生 overrun")
	}
	if br3.bitsLeft() != 0 {
		t.Errorf("setup 头未精确对齐：剩余 %d 位", br3.bitsLeft())
	}
}
