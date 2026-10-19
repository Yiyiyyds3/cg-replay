package vorbis

import (
	"os"
	"path/filepath"
	"testing"
)

// testOGG 返回用于对拍的测试文件；未设置 UCGR_OGG 时跳过。
func testOGG(t *testing.T) string {
	t.Helper()
	p := os.Getenv("UCGR_OGG")
	if p == "" {
		t.Skip("未设置 UCGR_OGG，跳过真实文件测试")
	}
	if _, err := os.Stat(p); err != nil {
		t.Skipf("测试文件不存在：%v", err)
	}
	return p
}

// TestParseHeaders 验证三个头能被解析出来。
func TestParseHeaders(t *testing.T) {
	path := testOGG(t)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	pages, err := parsePages(data)
	if err != nil {
		t.Fatalf("parsePages: %v", err)
	}
	serial, ok := selectStream(pages)
	if !ok {
		t.Fatal("找不到 Vorbis 流")
	}
	pkts := packets(pages, serial)
	if len(pkts) < 3 {
		t.Fatalf("包数不足：%d", len(pkts))
	}

	// 识别头：跳过 1 字节 type + "vorbis"。
	if pkts[0].Data[0] != 1 {
		t.Fatalf("首包不是识别头，type=%d", pkts[0].Data[0])
	}
	id, err := parseIdentification(newBitReader(pkts[0].Data[7:]))
	if err != nil {
		t.Fatalf("parseIdentification: %v", err)
	}
	t.Logf("识别头：%d 声道 · %d Hz · 块长 %d/%d",
		id.Channels, id.SampleRate, id.Blocksize0, id.Blocksize1)

	if pkts[1].Data[0] != 3 {
		t.Fatalf("第二包不是注释头，type=%d", pkts[1].Data[0])
	}

	if pkts[2].Data[0] != 5 {
		t.Fatalf("第三包不是 setup 头，type=%d", pkts[2].Data[0])
	}
	s, err := parseSetup(newBitReader(pkts[2].Data[7:]), id.Channels)
	if err != nil {
		t.Fatalf("parseSetup: %v", err)
	}
	t.Logf("setup：码本 %d · floor %d · residue %d · mapping %d · mode %d",
		len(s.codebooks), len(s.floors), len(s.residues), len(s.mappings), len(s.modes))
	for i, f := range s.floors {
		if f.kind == 1 {
			t.Logf("  floor#%d type1：partitions=%d multiplier=%d rangebits=%d X=%d 个",
				i, f.f1.partitions, f.f1.multiplier, f.f1.rangebits, len(f.f1.xList))
		} else {
			t.Logf("  floor#%d type0：order=%d books=%d", i, f.f0.order, len(f.f0.bookList))
		}
	}
	for i, r := range s.residues {
		t.Logf("  residue#%d type%d：begin=%d end=%d partition=%d classes=%d classbook=%d",
			i, r.kind, r.begin, r.end, r.partitionSize, r.classifications, r.classbook)
	}
	for i, m := range s.modes {
		t.Logf("  mode#%d blockflag=%v mapping=%d", i, m.blockflag, m.mapping)
	}
	t.Logf("音频包数：%d", len(pkts)-3)
	_ = filepath.Base(path)
}
