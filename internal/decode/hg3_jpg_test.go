package decode

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"testing"
)

// buildHG3JPG 造一个最小的单段 img_jpg HG3。
//   - jpg 是完整 JPEG 字节；
//   - alpha 为 nil 时不写 img_al 段，否则按 zlib 压缩写入；
//   - imgmode 为真时写一个只靠“存在”生效的 imgmode 段。
//
// 段偏移以 stdinfo 的 Base（0x14）为原点，与 GARbro 的输入流定位一致。
func buildHG3JPG(t *testing.T, w, h int, jpg, alpha []byte, imgmode bool) []byte {
	t.Helper()
	const base = 0x14
	const headerSize = 0x38
	out := make([]byte, base+headerSize)
	copy(out, "HG-3")
	binary.LittleEndian.PutUint32(out[4:], 0x0c)
	copy(out[base:], "stdinfo")
	binary.LittleEndian.PutUint32(out[base+8:], headerSize)
	binary.LittleEndian.PutUint32(out[base+0x10:], uint32(w))
	binary.LittleEndian.PutUint32(out[base+0x14:], uint32(h))
	binary.LittleEndian.PutUint32(out[base+0x18:], 24)

	add := func(name string, payload []byte) {
		rec := make([]byte, 12)
		copy(rec, name)
		binary.LittleEndian.PutUint32(rec[8:], uint32(len(rec)+len(payload)))
		out = append(out, rec...)
		out = append(out, payload...)
	}

	jpgPayload := make([]byte, 4)
	binary.LittleEndian.PutUint32(jpgPayload, uint32(len(jpg)))
	jpgPayload = append(jpgPayload, jpg...)
	add("img_jpg", jpgPayload)

	if alpha != nil {
		packed := zlibBytes(t, alpha)
		payload := make([]byte, 12) // 4 未知字节 + packedSize + alphaSize
		binary.LittleEndian.PutUint32(payload[4:], uint32(len(packed)))
		binary.LittleEndian.PutUint32(payload[8:], uint32(len(alpha)))
		payload = append(payload, packed...)
		add("img_al", payload)
	}
	if imgmode {
		add("imgmode", make([]byte, 4))
	}
	out = append(out, make([]byte, 12)...) // section_size == 0 结束目录
	return out
}

func zlibBytes(t *testing.T, b []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	if _, err := zw.Write(b); err != nil {
		t.Fatalf("zlib.Write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zlib.Close: %v", err)
	}
	return buf.Bytes()
}

func TestDecodeHG3ImgJPG(t *testing.T) {
	jpg := plainJPEG(t)
	base, err := decodeJPEG(jpg, 0)
	if err != nil {
		t.Fatalf("decodeJPEG: %v", err)
	}
	alpha := []byte{0, 17, 34, 51, 68, 85, 102, 119, 136, 153, 170, 187}

	for _, tc := range []struct {
		name    string
		alpha   []byte
		imgmode bool
	}{
		{"无 alpha 段", nil, false},
		{"有 alpha 段", alpha, false},
		{"alpha + imgmode", alpha, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := buildHG3JPG(t, base.Width, base.Height, jpg, tc.alpha, tc.imgmode)
			im, err := DecodeHG3(data, 0)
			if err != nil {
				t.Fatalf("DecodeHG3: %v", err)
			}
			if im.Width != base.Width || im.Height != base.Height || im.Colors != 4 {
				t.Fatalf("结果 = %dx%d/%d 色，期望 %dx%d/4 色",
					im.Width, im.Height, im.Colors, base.Width, base.Height)
			}
			want := append([]byte(nil), base.Pixels...)
			if tc.imgmode {
				for i := 0; i < base.Width*base.Height; i++ {
					want[i*4], want[i*4+2] = want[i*4+2], want[i*4]
				}
			}
			for i := 0; i < base.Width*base.Height; i++ {
				a := byte(0xFF)
				if tc.alpha != nil {
					a = tc.alpha[i]
				}
				want[i*4+3] = a
			}
			if !bytes.Equal(im.Pixels, want) {
				t.Errorf("BGRA 像素与期望不一致")
			}
		})
	}
}

func TestDecodeHG3ImgJPGRejectsBadInput(t *testing.T) {
	jpg := plainJPEG(t) // 4×3
	t.Run("头尺寸与 JPEG 不符", func(t *testing.T) {
		data := buildHG3JPG(t, 5, 3, jpg, nil, false)
		if _, err := DecodeHG3(data, 0); err == nil {
			t.Fatal("尺寸不符本应报错")
		}
	})
	t.Run("alpha 长度不足", func(t *testing.T) {
		data := buildHG3JPG(t, 4, 3, jpg, []byte{1, 2, 3}, false)
		if _, err := DecodeHG3(data, 0); err == nil {
			t.Fatal("alpha 长度不足本应报错")
		}
	})
	t.Run("没有 img_jpg 段", func(t *testing.T) {
		data := buildHG3JPG(t, 4, 3, jpg, nil, false)
		copy(data[0x4c:], "img_bad") // 把首段名字改坏，目录里仍有 img_jpg 但类型不同
		// 直接改首段名字会同时让 DecodeHG3 走到未知类型，这里只验证 img_jpg 路径本身。
		if _, err := decodeHg3ImgJPG(data, Hg3Section{Base: 0x14, HeaderSize: 0x38, Width: 4, Height: 3}); err == nil {
			t.Fatal("缺少 img_jpg 子段本应报错")
		}
	})
}
