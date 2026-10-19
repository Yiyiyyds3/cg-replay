package vorbis

import "testing"

// TestHuffmanMarkerOrder 锁定 Vorbis 的码字分配顺序：按 entry 顺序用 marker
// 递推，而不是「先按长度分组再顺序分配」。长度交错时两者结果不同。
func TestHuffmanMarkerOrder(t *testing.T) {
	cb := &codebook{lengths: []int{2, 1, 2}}
	if err := cb.buildHuffman(); err != nil {
		t.Fatalf("buildHuffman: %v", err)
	}
	cases := []struct {
		name  string
		byte0 byte
		want  int
	}{
		{"entry0 code 00", 0b00, 0},
		{"entry1 code 1", 0b1, 1},
		{"entry2 code 01", 0b10, 2}, // 位流先读低位的 0，再读高位的 1
	}
	for _, c := range cases {
		br := newBitReader([]byte{c.byte0})
		if got := cb.decodeScalar(br); got != c.want {
			t.Errorf("%s: 解出 entry %d，期望 %d", c.name, got, c.want)
		}
	}
}

func TestLookup1Values(t *testing.T) {
	cases := []struct{ entries, dim, want int }{
		{1, 1, 1},
		{4, 2, 2},
		{8, 3, 2},
		{27, 3, 3},
		{28, 3, 3},
		{64, 2, 8},
	}
	for _, c := range cases {
		if got := lookup1Values(c.entries, c.dim); got != c.want {
			t.Errorf("lookup1Values(%d,%d)=%d，期望 %d", c.entries, c.dim, got, c.want)
		}
	}
}
