package decode

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

// plainJPEG 造一张确定的小 JPEG（4×3，逐像素渐变），供掩码测试反复使用。
func plainJPEG(t *testing.T) []byte {
	t.Helper()
	src := image.NewRGBA(image.Rect(0, 0, 4, 3))
	for y := 0; y < 3; y++ {
		for x := 0; x < 4; x++ {
			src.Set(x, y, color.RGBA{
				R: uint8(20 * x), G: uint8(40 * y), B: uint8(10 * (x + y)), A: 255,
			})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, src, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatalf("jpeg.Encode: %v", err)
	}
	return buf.Bytes()
}

// xorMasked 返回整文件逐字节 XOR k 的副本。
func xorMasked(b []byte, k byte) []byte {
	out := make([]byte, len(b))
	for i, v := range b {
		out[i] = v ^ k
	}
	return out
}

func TestUnmaskJPEGPlain(t *testing.T) {
	plain := plainJPEG(t)
	got, k, ok := UnmaskJPEG(plain)
	if !ok {
		t.Fatalf("UnmaskJPEG(明文 JPEG) = ok false，期望 true")
	}
	if k != 0 {
		t.Errorf("明文掩码 = 0x%02X，期望 0", k)
	}
	if !bytes.Equal(got, plain) {
		t.Errorf("明文数据不应被改写")
	}
}

func TestUnmaskJPEGMasked(t *testing.T) {
	plain := plainJPEG(t)
	masked := xorMasked(plain, 0xA4)
	got, k, ok := UnmaskJPEG(masked)
	if !ok {
		t.Fatalf("UnmaskJPEG(0xA4 掩码) = ok false，期望 true")
	}
	if k != 0xA4 {
		t.Errorf("掩码 = 0x%02X，期望 0xA4", k)
	}
	if !bytes.Equal(got, plain) {
		t.Errorf("还原结果与原图不一致")
	}
}

// TestUnmaskJPEGRejectsOthers 非「整文件 XOR 的 JPEG」一律判否，且不改写数据。
func TestUnmaskJPEGRejectsOthers(t *testing.T) {
	cases := []struct {
		name string
		data []byte
	}{
		{"空", nil},
		{"过短", []byte{0x5B, 0x7C}},
		{"PNG", []byte("\x89PNG\r\n\x1a\nabcdefgh")},
		{"头对尾不对", []byte{0xFF, 0xD8, 0xFF, 0x00, 0x00, 0x00}},
		{"像掩码但尾不对", bytes.Repeat([]byte{0x5B, 0x7C, 0x5B, 0x44}, 8)},
	}
	for _, c := range cases {
		got, k, ok := UnmaskJPEG(c.data)
		if ok {
			t.Errorf("%s: ok=true（k=0x%02X），期望 false", c.name, k)
		}
		if !bytes.Equal(got, c.data) {
			t.Errorf("%s: 判定失败时不应改写数据", c.name)
		}
	}
}

// TestDecodeJPEGPlainAndMasked 明文与 0xA4 掩码两条路都通，且解码结果逐字节相同。
func TestDecodeJPEGPlainAndMasked(t *testing.T) {
	plain := plainJPEG(t)
	base, err := Decode(plain)
	if err != nil {
		t.Fatalf("Decode(明文 JPEG): %v", err)
	}
	if base.Width != 4 || base.Height != 3 {
		t.Errorf("尺寸 = %dx%d，期望 4x3", base.Width, base.Height)
	}
	if base.Colors != 4 {
		t.Errorf("Colors = %d，期望 4", base.Colors)
	}
	if base.Mask != 0 {
		t.Errorf("明文 Mask = 0x%02X，期望 0", base.Mask)
	}
	if len(base.Pixels) != 4*3*4 {
		t.Fatalf("像素字节数 = %d，期望 %d", len(base.Pixels), 4*3*4)
	}

	masked, err := Decode(xorMasked(plain, 0xA4))
	if err != nil {
		t.Fatalf("Decode(0xA4 掩码 JPEG): %v", err)
	}
	if masked.Mask != 0xA4 {
		t.Errorf("Mask = 0x%02X，期望 0xA4", masked.Mask)
	}
	if masked.Width != base.Width || masked.Height != base.Height {
		t.Errorf("掩码图尺寸 = %dx%d，期望 %dx%d",
			masked.Width, masked.Height, base.Width, base.Height)
	}
	if !bytes.Equal(base.Pixels, masked.Pixels) {
		t.Errorf("掩码图与明文图的解码结果不一致")
	}
}

// TestDecodeJPEGPixelOrder 抽查 BGRA 次序（不透明像素第 4 字节为 alpha）。
func TestDecodeJPEGPixelOrder(t *testing.T) {
	img, err := Decode(plainJPEG(t))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if img.Pixels[3] != 0xFF {
		t.Errorf("首像素 alpha = 0x%02X，期望 0xFF（B、G、R、A 的 A）", img.Pixels[3])
	}
}
