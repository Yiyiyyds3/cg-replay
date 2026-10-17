package decode

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
)

// buildTLG5 拼一个「全原样通道（mark=1）」的 TLG5 文件，
// 用来把组合/累加算法钉死在测试里（不依赖磁盘上的真实游戏资产）。
// planes[c] 是该通道的块内增量字节，长度须为 blockHeight*width。
func buildTLG5(width, height, blockHeight, colors int, planes [][]byte) []byte {
	var b bytes.Buffer
	b.WriteString("TLG5.0\x00raw\x1a")
	b.WriteByte(byte(colors))
	var tmp [4]byte
	put := func(v int) {
		binary.LittleEndian.PutUint32(tmp[:], uint32(v))
		b.Write(tmp[:])
	}
	put(width)
	put(height)
	put(blockHeight)
	blockCount := (height-1)/blockHeight + 1
	for i := 0; i < blockCount; i++ {
		put(0) // 每块尺寸索引：解码时会跳过
	}
	for y := 0; y < height; y += blockHeight {
		for c := 0; c < colors; c++ {
			b.WriteByte(1) // mark != 0 → 原样字节
			put(len(planes[c]))
			b.Write(planes[c])
		}
	}
	return b.Bytes()
}

func TestTLG5Colors4(t *testing.T) {
	// 2x2 四通道；期望值由算法手算（首行横向累加 + 色差 g，次行纵向累加）。
	data := buildTLG5(2, 2, 2, 4, [][]byte{
		{1, 2, 3, 4},     // B 增量
		{10, 20, 30, 40}, // G 增量
		{5, 6, 7, 8},     // R 增量
		{0, 0, 0, 0},     // A 增量
	})
	img, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if img.Width != 2 || img.Height != 2 || img.Colors != 4 {
		t.Fatalf("尺寸/通道不符: %dx%d colors=%d", img.Width, img.Height, img.Colors)
	}
	want := []byte{
		11, 10, 15, 0, // 像素0 B,G,R,A
		33, 30, 41, 0, // 像素1
		44, 40, 52, 0, // 像素2（第二行首像素）
		110, 100, 126, 0, // 像素3
	}
	if !bytes.Equal(img.Pixels, want) {
		t.Fatalf("像素不符\n got=%v\nwant=%v", img.Pixels, want)
	}
}

func TestTLG5Colors3(t *testing.T) {
	// 2x1 三通道：A 固定 0xff。
	data := buildTLG5(2, 1, 1, 3, [][]byte{
		{1, 2},
		{10, 20},
		{5, 6},
	})
	img, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	want := []byte{11, 10, 15, 0xff, 33, 30, 41, 0xff}
	if !bytes.Equal(img.Pixels, want) {
		t.Fatalf("像素不符\n got=%v\nwant=%v", img.Pixels, want)
	}
}

func TestDecodeRejects(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want string
	}{
		{"过短", []byte("TLG"), "文件过短"},
		{"非 TLG", []byte("PNG\x89abcdefgh"), "非 TLG"},
		{"TLG6 未实现", append([]byte("TLG6.0\x00raw\x1a"), make([]byte, 16)...), "TLG6"},
		{"TLG0 未实现", append([]byte("TLG0.0\x00sds\x1a"), make([]byte, 16)...), "TLG0"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Decode(c.data)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err=%v，期望包含 %q", err, c.want)
			}
		})
	}
}

func TestTLG5TruncatedChunk(t *testing.T) {
	data := buildTLG5(2, 1, 1, 3, [][]byte{{1, 2}, {10, 20}, {5, 6}})
	// 砍掉尾部，制造通道数据越界（报错须点明块/通道，便于定位）。
	_, err := Decode(data[:len(data)-3])
	if err == nil || !strings.Contains(err.Error(), "截断") || !strings.Contains(err.Error(), "通道") {
		t.Fatalf("err=%v，期望包含「截断」且点明「通道」", err)
	}
}
