package decode

import (
	"bytes"
	"image"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// HG3 参考比对需要本机样本（不入库）。提供参考图的两种方式，二选一：
//
//	UCGR_HG3_SRC  = 放 .hg3 的目录
//	UCGR_HG3_REF  = 用 GARbro 现转的 PNG 目录（GarHg3.exe <src> <ref>，平铺命名
//	                <stem>_0000.png、<stem>_0001.png…）
//	UCGR_HG3_PIC  = 解包时已转好的图库目录（如《甜蜜女友2+》的 图片\：同名 PNG 分散在
//	                185 个子目录里，按文件名索引即可，不必再跑 GarHg3）
//	UCGR_HG3_LIMIT = 只比对前 N 个 .hg3（0 或未设 = 全部）
//
// 未设 UCGR_HG3_SRC（或两种参考图都没给）时跳过，与 Ogg 测试同策略。
func TestHG3AgainstReference(t *testing.T) {
	src := os.Getenv("UCGR_HG3_SRC")
	refDir := os.Getenv("UCGR_HG3_REF")
	picDir := os.Getenv("UCGR_HG3_PIC")
	if src == "" || (refDir == "" && picDir == "") {
		t.Skip("未设置 UCGR_HG3_SRC +（UCGR_HG3_REF 或 UCGR_HG3_PIC），跳过 HG3 参考比对")
	}
	refs := map[string]string{}
	if picDir != "" {
		count := map[string]int{}
		err := filepath.WalkDir(picDir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(strings.ToLower(d.Name()), ".png") {
				return nil //nolint:nilerr // 个别子目录读不到就算了，靠下面的「缺参考图」统计兜底
			}
			refs[d.Name()] = p
			count[d.Name()]++
			return nil
		})
		if err != nil {
			t.Fatalf("遍历图库 %s 失败: %v", picDir, err)
		}
		for n, c := range count {
			if c > 1 {
				delete(refs, n) // 重名参考图不猜
			}
		}
		t.Logf("图库索引：%d 个 PNG（重名被剔除后 %d 个）", len(count), len(refs))
	}
	files, _ := filepath.Glob(filepath.Join(src, "*.hg3"))
	if len(files) == 0 {
		t.Fatalf("目录 %s 下没有 .hg3", src)
	}
	sort.Strings(files)
	limit, _ := strconv.Atoi(os.Getenv("UCGR_HG3_LIMIT"))
	if limit > 0 && limit < len(files) {
		files = files[:limit]
	}
	const reportMax = 20
	var images, mismatched, missing, filesDone int
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("读 %s 失败: %v", f, err)
		}
		stem := strings.TrimSuffix(filepath.Base(f), filepath.Ext(f))
		ims, err := DecodeHG3All(data)
		if err != nil {
			t.Errorf("%s: 解码失败: %v", stem, err)
			continue
		}
		filesDone++
		for i, im := range ims {
			name := stem + "_" + pad4(i) + ".png"
			refPath := ""
			if picDir != "" {
				refPath = refs[name]
			} else {
				refPath = filepath.Join(refDir, name)
			}
			if refPath == "" {
				missing++
				if missing <= reportMax {
					t.Errorf("%s: 图库里没有对应参考图", name)
				}
				continue
			}
			want, err := readRefPNG(refPath)
			if err != nil {
				t.Errorf("%s: 读参考图失败: %v", name, err)
				continue
			}
			images++
			if d, detail := diffBGRA(im, want); d > 0 {
				mismatched++
				if mismatched <= reportMax {
					t.Errorf("%s: %d×%d 与参考图不一致（最大通道差 %d）%s", name, im.Width, im.Height, d, detail)
				}
			}
			if dump := os.Getenv("UCGR_HG3_DUMP"); dump != "" {
				if err := dumpPNG(filepath.Join(dump, name), im); err != nil {
					t.Fatalf("导出 %s 失败: %v", name, err)
				}
			}
		}
	}
	if images == 0 {
		t.Fatalf("没有比对到任何图（参考目录 %s / 图库 %s 为空？）", refDir, picDir)
	}
	t.Logf("HG3 参考比对：%d 个文件、%d 张图，逐字节一致 %d 张，有差异 %d 张，缺参考图 %d 张",
		filesDone, images, images-mismatched, mismatched, missing)
}

func pad4(i int) string {
	s := strconv.Itoa(i)
	for len(s) < 4 {
		s = "0" + s
	}
	return s
}

// diffBGRA 比较解码结果与参考 PNG；返回最大通道差与首个差异点的描述。
func diffBGRA(im *Image, want *image.NRGBA) (int, string) {
	w, h := want.Rect.Dx(), want.Rect.Dy()
	if im.Width != w || im.Height != h {
		return 255, "尺寸不同"
	}
	ps := 4
	if im.Colors == 3 {
		ps = 3
	}
	if len(im.Pixels) < w*h*ps || len(want.Pix) < w*h*4 {
		return 255, "像素缓冲不足（得到 " + strconv.Itoa(len(im.Pixels)) + "，需要 " + strconv.Itoa(w*h*ps) + "）"
	}
	maxDiff := 0
	first := ""
	for i := 0; i < w*h; i++ {
		var b, g, r, a byte
		if ps == 3 {
			b, g, r, a = im.Pixels[i*3], im.Pixels[i*3+1], im.Pixels[i*3+2], 255
		} else {
			b, g, r, a = im.Pixels[i*4], im.Pixels[i*4+1], im.Pixels[i*4+2], im.Pixels[i*4+3]
		}
		// 参考 PNG 由 Go 解成 NRGBA，字节序是 R、G、B、A，与解码结果的 B、G、R、A 相反。
		rb, rg, rr, ra := want.Pix[i*4+2], want.Pix[i*4+1], want.Pix[i*4+0], want.Pix[i*4+3]
		d := maxInt(maxInt(abs(int(rb)-int(b)), abs(int(rg)-int(g))),
			maxInt(abs(int(rr)-int(r)), abs(int(ra)-int(a))))
		if d > maxDiff {
			maxDiff = d
		}
		if d > 0 && first == "" {
			first = "（首个差异 第" + strconv.Itoa(i) + "像素 参考 B" +
				strconv.Itoa(int(rb)) + " G" + strconv.Itoa(int(rg)) + " R" + strconv.Itoa(int(rr)) +
				" A" + strconv.Itoa(int(ra)) + " ／ 得到 B" + strconv.Itoa(int(b)) + " G" +
				strconv.Itoa(int(g)) + " R" + strconv.Itoa(int(r)) + " A" + strconv.Itoa(int(a)) + "）"
		}
	}
	return maxDiff, first
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func readRefPNG(path string) (*image.NRGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, err
	}
	if n, ok := img.(*image.NRGBA); ok {
		return n, nil
	}
	b := img.Bounds()
	n := image.NewNRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			n.Set(x, y, img.At(x, y))
		}
	}
	return n, nil
}

// dumpPNG 把解码结果（BGRA / BGR）导出成 PNG，便于人工目视比对。
func dumpPNG(path string, im *Image) error {
	n := image.NewNRGBA(image.Rect(0, 0, im.Width, im.Height))
	ps := 4
	if im.Colors == 3 {
		ps = 3
	}
	for i := 0; i < im.Width*im.Height && (i+1)*ps <= len(im.Pixels); i++ {
		n.Pix[i*4+0] = im.Pixels[i*ps+2]
		n.Pix[i*4+1] = im.Pixels[i*ps+1]
		n.Pix[i*4+2] = im.Pixels[i*ps+0]
		n.Pix[i*4+3] = 255
		if ps == 4 {
			n.Pix[i*4+3] = im.Pixels[i*ps+3]
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, n)
}

// decode.Decode 必须把 HG3 分派到 HG3 解码器（而不是落到「非 TLG/JPEG/HG3」）。
func TestDecodeDispatchesHG3(t *testing.T) {
	data := append([]byte("HG-3\x0c\x00\x00\x00"), bytes.Repeat([]byte{0}, 0x60)...)
	_, err := Decode(data)
	if err == nil {
		t.Fatal("假 HG3 本应报错却成功")
	}
	if strings.Contains(err.Error(), "非 TLG/JPEG/HG3") {
		t.Fatalf("HG3 没有被分派到 HG3 解码器（得到 %v）", err)
	}
}

// 非 HG3 输入必须给出明确错误，不能静默返回。
func TestHG3RejectsForeignData(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"空", nil},
		{"TLG 头", append(append([]byte{}, tlgMagic5...), make([]byte, 32)...)},
		{"只有魔数", []byte("HG-3\x0c\x00\x00\x00")},
		{"假头", append([]byte("HG-3\x0c\x00\x00\x00"), bytes.Repeat([]byte{0}, 0x60)...)},
	} {
		if _, err := DecodeHG3(tc.data, 0); err == nil {
			t.Errorf("%s：本应报错却成功", tc.name)
		}
	}
}
