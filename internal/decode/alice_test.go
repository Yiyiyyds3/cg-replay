package decode

// alice_test.go —— AliceSoft AJP/QNT/DCF 解码测试。
//
// 合成样本：QNT 用测试内 mini 编码器做 round-trip（覆盖新旧双布局与奇数
// 尺寸）；AJP 用标准库 JPEG + zlib 掩码 + 16 字节 XOR 组装。
// 真实语料：兰斯10（Rance10CG.afa），语料缺失自动 skip；DCF 走链式差分
// （差分→差分→原图）验证递归合成。

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---- QNT mini 编码器（decodeQNT 的逆）----

func qntEncode(t *testing.T, w, h int, bgra []byte, newLayout bool) []byte {
	t.Helper()
	pw, ph := (w+1)&^1, (h+1)&^1

	// BGRA → BGR 行优先
	pic := make([]byte, w*h*3)
	for p := 0; p < w*h; p++ {
		pic[p*3+0] = bgra[p*4+0]
		pic[p*3+1] = bgra[p*4+1]
		pic[p*3+2] = bgra[p*4+2]
	}
	// 逆预测：解码全部基于「已还原像素」，所以编码公式的输入也全部取自
	// 原图（src），与遍历顺序无关：
	//   首行   comp[x]   = recovered[x-1] - recovered[x]
	//   首列   comp[y]   = recovered[上]   - recovered[y]
	//   其余   comp[y,x] = ((recovered[上]+recovered[左])>>1) - recovered[y,x]
	comp := make([]byte, len(pic))
	src := pic
	copy(comp[:3], src[:3]) // 首像素预测直通（BGR 三通道）
	for y := 1; y < h; y++ {
		for x := 1; x < w; x++ {
			for c := 0; c < 3; c++ {
				up := int(src[((y-1)*w+x)*3+c])
				left := int(src[(y*w+x-1)*3+c])
				comp[(y*w+x)*3+c] = byte((up+left)>>1 - int(src[(y*w+x)*3+c]))
			}
		}
		for c := 0; c < 3; c++ {
			comp[(y*w)*3+c] = src[((y-1)*w)*3+c] - src[(y*w)*3+c]
		}
	}
	for x := 1; x < w; x++ {
		for c := 0; c < 3; c++ {
			comp[x*3+c] = src[(x-1)*3+c] - src[x*3+c]
		}
	}
	// 逆 2×2 重排：BGR comp（预测后）→ R,G,B 平面序 raw
	raw := make([]byte, pw*ph*3)
	j := 0
	for i := 2; i >= 0; i-- {
		for y := 0; y < ph; y += 2 {
			for x := 0; x < pw; x += 2 {
				get := func(yy, xx int) byte {
					if yy >= h || xx >= w {
						return 0 // padding 补 0
					}
					return comp[(yy*w+xx)*3+i]
				}
				raw[j] = get(y, x)
				raw[j+1] = get(y+1, x)
				raw[j+2] = get(y, x+1)
				raw[j+3] = get(y+1, x+1)
				j += 4
			}
		}
	}
	var pcomp bytes.Buffer
	zw := zlib.NewWriter(&pcomp)
	zw.Write(raw)
	zw.Close()

	// alpha 逆预测（解码公式的逆；行优先 + 奇宽补位）
	aw := make([]byte, w*h)
	for p := 0; p < w*h; p++ {
		aw[p] = bgra[p*4+3]
	}
	acomp := make([]byte, pw*h)
	acomp[0] = aw[0]
	for x := 1; x < w; x++ { // 首行：解码 alpha[x]=alpha[x-1]-raw → raw=alpha[x-1]-alpha[x]
		acomp[x] = aw[x-1] - aw[x]
	}
	for y := 1; y < h; y++ {
		acomp[y*pw] = aw[(y-1)*w] - aw[y*w]
		for x := 1; x < w; x++ { // 解码 alpha=(left+up)>>1-raw → raw=((left+up)>>1)-alpha
			left, up := int(aw[y*w+x-1]), int(aw[(y-1)*w+x])
			acomp[y*pw+x] = byte((left+up)>>1 - int(aw[y*w+x]))
		}
	}
	var acomp2 bytes.Buffer
	zw2 := zlib.NewWriter(&acomp2)
	zw2.Write(acomp)
	zw2.Close()

	hdr := make([]byte, 48)
	binary.LittleEndian.PutUint32(hdr[0:], 0x00544e51) // "QNT\0"
	if newLayout {
		// 新布局：rsv0 非 0、hdr_size @8、字段自 @12 起
		hdr = make([]byte, 52)
		binary.LittleEndian.PutUint32(hdr[4:], 2)
		binary.LittleEndian.PutUint32(hdr[8:], 52)
		binary.LittleEndian.PutUint32(hdr[20:], uint32(w))
		binary.LittleEndian.PutUint32(hdr[24:], uint32(h))
		binary.LittleEndian.PutUint32(hdr[28:], 24)
		binary.LittleEndian.PutUint32(hdr[36:], uint32(pcomp.Len()))
		binary.LittleEndian.PutUint32(hdr[40:], uint32(acomp2.Len()))
	} else {
		// 旧布局：rsv0 == 0、hdr 固定 48、字段自 @8 起
		binary.LittleEndian.PutUint32(hdr[4:], 0)
		binary.LittleEndian.PutUint32(hdr[16:], uint32(w))
		binary.LittleEndian.PutUint32(hdr[20:], uint32(h))
		binary.LittleEndian.PutUint32(hdr[24:], 24)
		binary.LittleEndian.PutUint32(hdr[32:], uint32(pcomp.Len()))
		binary.LittleEndian.PutUint32(hdr[36:], uint32(acomp2.Len()))
	}
	out := append(hdr, pcomp.Bytes()...)
	return append(out, acomp2.Bytes()...)
}

func randBGRA(w, h int, seed int64) []byte {
	r := rand.New(rand.NewSource(seed))
	b := make([]byte, w*h*4)
	for i := range b {
		b[i] = byte(r.Intn(256))
	}
	for p := 0; p < w*h; p++ { // alpha 固定不透明，避免编码器歧义
		b[p*4+3] = 0xFF
	}
	return b
}

func TestQNTRoundTrip(t *testing.T) {
	cases := []struct {
		w, h      int
		newLayout bool
	}{
		{4, 2, false},   // 偶×偶 旧布局（兰斯10 实测形态）
		{4, 2, true},    // 新布局
		{5, 3, false},   // 奇×奇：尾行/尾列边界
		{1, 1, false},   // 极小
		{33, 17, false}, // 跨多块
		{2, 9, true},    // 奇高新布局
	}
	for _, c := range cases {
		src := randBGRA(c.w, c.h, int64(c.w*100+c.h))
		enc := qntEncode(t, c.w, c.h, src, c.newLayout)
		img, err := decodeQNT(enc)
		if err != nil {
			t.Fatalf("QNT %dx%d(new=%v) 解码失败：%v", c.w, c.h, c.newLayout, err)
		}
		if img.Width != c.w || img.Height != c.h {
			t.Fatalf("QNT %dx%d(new=%v) 尺寸 %dx%d", c.w, c.h, c.newLayout, img.Width, img.Height)
		}
		if !bytes.Equal(src, img.Pixels) {
			n := 0
			for i := range src {
				if src[i] != img.Pixels[i] {
					n++
				}
			}
			t.Fatalf("QNT %dx%d(new=%v) round-trip 有 %d 字节不符", c.w, c.h, c.newLayout, n)
		}
	}
}

// 合成 AJP：标准库 JPEG + zlib 掩码 + 16 字节 XOR。
func makeAJP(t *testing.T, w, h int, withMask bool) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{byte(x * 7), byte(y * 5), byte(x * y), 255})
		}
	}
	var jb bytes.Buffer
	if err := jpeg.Encode(&jb, img, &jpeg.Options{Quality: 100}); err != nil {
		t.Fatal(err)
	}
	jd := jb.Bytes()

	var mask []byte
	if withMask {
		m := make([]byte, w*h)
		for i := range m {
			m[i] = byte(i % 256)
		}
		var zb bytes.Buffer
		zw := zlib.NewWriter(&zb)
		zw.Write(m)
		zw.Close()
		mask = zb.Bytes()
	}
	hdr := make([]byte, 36)
	copy(hdr, "AJP\x00")
	binary.LittleEndian.PutUint32(hdr[12:], uint32(w))
	binary.LittleEndian.PutUint32(hdr[16:], uint32(h))
	binary.LittleEndian.PutUint32(hdr[20:], 36)
	binary.LittleEndian.PutUint32(hdr[24:], uint32(len(jd)))
	moff, msize := 0, 0
	if withMask {
		moff = 36 + len(jd)
		msize = len(mask)
	}
	binary.LittleEndian.PutUint32(hdr[28:], uint32(moff))
	binary.LittleEndian.PutUint32(hdr[32:], uint32(msize))

	jblock := make([]byte, len(jd))
	copy(jblock, jd)
	xorAJPTail16(jblock)
	out := append(hdr, jblock...)
	if withMask {
		mblock := make([]byte, len(mask))
		copy(mblock, mask)
		xorAJPTail16(mblock)
		out = append(out, mblock...)
	}
	return out
}

func TestAJPSynth(t *testing.T) {
	for _, withMask := range []bool{false, true} {
		src := makeAJP(t, 64, 48, withMask)
		img, err := decodeAJP(src)
		if err != nil {
			t.Fatalf("AJP(withMask=%v) 解码失败：%v", withMask, err)
		}
		if img.Width != 64 || img.Height != 48 {
			t.Fatalf("AJP 尺寸 %dx%d", img.Width, img.Height)
		}
		if withMask && img.Pixels[3] != 0 {
			t.Fatalf("AJP mask alpha 应为 0（首像素 mask=0），得 %d", img.Pixels[3])
		}
		if !withMask && img.Pixels[3] != 0xFF {
			t.Fatalf("AJP 无 mask 应不透明，得 %d", img.Pixels[3])
		}
	}
	// 坏块：JPEG SOI 不符应报错
	bad := makeAJP(t, 8, 8, false)
	bad[36] ^= 0xFF
	if _, err := decodeAJP(bad); err == nil {
		t.Fatal("损坏的 AJP 应报错")
	}
}

// ---- 真实语料 ----

func rance10Dir(t *testing.T) string {
	t.Helper()
	if d := os.Getenv("UCGR_RANCE10_DIR"); d != "" {
		return d
	}
	const def = `E:\Personal_entertainment\game\zfsgalgame\Rance10\兰斯10`
	if _, err := os.Stat(def + "\\Rance10.ain"); err == nil {
		return def
	}
	t.Skip("兰斯10 语料不可用")
	return ""
}

// miniAFA 只为测试读 AFA 条目（正式解析在 internal/adapter/alice）。
func miniAFA(t *testing.T, path string) map[string][]byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读 %s 失败：%v", path, err)
	}
	if !bytes.HasPrefix(data, []byte("AFAH")) {
		t.Fatalf("%s 非 AFA", path)
	}
	dataStart := int(binary.LittleEndian.Uint32(data[24:]))
	tablePacked := int(binary.LittleEndian.Uint32(data[32:]))
	nr := int(binary.LittleEndian.Uint32(data[40:]))
	zr, err := zlib.NewReader(bytes.NewReader(data[44 : 44+tablePacked]))
	if err != nil {
		t.Fatalf("AFA 表 zlib 打开失败：%v", err)
	}
	tbl, err := io.ReadAll(zr)
	zr.Close()
	if err != nil {
		t.Fatalf("AFA 表解压失败：%v", err)
	}
	byName := map[string][]byte{}
	pos := 0
	for i := 0; i < nr && pos+8+16 <= len(tbl); i++ {
		strLen := int(int32(binary.LittleEndian.Uint32(tbl[pos+4:])))
		if strLen <= 0 || pos+8+strLen+16 > len(tbl) {
			t.Fatalf("AFA 条目 %d 非法（strLen=%d）", i, strLen)
		}
		name := string(bytes.TrimRight(tbl[pos+8:pos+8+strLen], "\x00"))
		off := int(int32(binary.LittleEndian.Uint32(tbl[pos+8+strLen+8:])))
		size := int(int32(binary.LittleEndian.Uint32(tbl[pos+8+strLen+12:])))
		if off < 0 || size < 0 || dataStart+off+size > len(data) {
			t.Fatalf("AFA 条目 %s 越界", name)
		}
		byName[name] = data[dataStart+off : dataStart+off+size]
		pos += 8 + strLen + 16
	}
	if len(byName) == 0 {
		t.Fatal("AFA 表解析为空")
	}
	return byName
}

// rance10Lookup 匹配语义（对齐 libsys4 archive_basename）：
// 键与底图名都先去掉扩展名，再「精确匹配，或键以 "/"+查询 结尾」（裸名场景，
// 键多一层 立／ 等目录前缀）。GBK 字节原样比较（两侧同源，无需归一；
// 大小写/全角归一会破坏 GBK 双字节，正式实现的归一在 adapter/alice 的
// UTF-8 域完成）。
func rance10Lookup(byName map[string][]byte) func(stem string) ([]byte, error) {
	stemless := map[string][]byte{} // 键去扩展名 → 数据
	for k, v := range byName {
		if i := strings.LastIndex(k, "."); i > 0 {
			stemless[k[:i]] = v
		}
	}
	return func(stem string) ([]byte, error) {
		key := stem
		if i := strings.LastIndex(stem, "."); i > 0 {
			key = stem[:i] // 底图名常带假后缀（.png 等，实际内容是 QNT）
		}
		if v, ok := stemless[key]; ok {
			return v, nil
		}
		// 裸名退化：键去扩展名后以 "/"+key 结尾，取最长者
		pfx := "/" + key
		var bestK string
		var best []byte
		for k, v := range stemless {
			if strings.HasSuffix(k, pfx) && len(k) > len(bestK) {
				bestK = k
				best = v
			}
		}
		if best != nil {
			return best, nil
		}
		return nil, fmt.Errorf("AFA 中找不到 %q", stem)
	}
}

// TestRealAliceFormats 用兰斯10 真实 CG.afa 验证 QNT / AJP / DCF 全链路。
func TestRealAliceFormats(t *testing.T) {
	dir := rance10Dir(t)
	byName := map[string][]byte{}
	entries, err := filepath.Glob(dir + "\\Rance10*.afa")
	if err != nil || len(entries) == 0 {
		t.Fatalf("glob Rance10*.afa 失败：%v", err)
	}
	for _, p := range entries { // 合并全部归档（底图可跨 afa，贴近 afapool 多池形态）
		for k, v := range miniAFA(t, p) {
			byName[k] = v
		}
	}
	lookup := rance10Lookup(byName)

	// QNT：整图直解（键是 GBK 字节串，不能按 UTF-8 中文匹配，按扩展名选取）
	var qntName string
	for name, v := range byName {
		if bytes.HasPrefix(v, []byte("QNT\x00")) && strings.HasSuffix(name, ".qnt") {
			qntName = name
			break
		}
	}
	if qntName == "" {
		t.Fatal("没找到 QNT 样本")
	}
	img, err := Decode(byName[qntName])
	if err != nil {
		t.Fatalf("QNT %s 解码失败：%v", qntName, err)
	}
	if img.Width <= 0 || img.Height <= 0 {
		t.Fatalf("QNT 尺寸非法 %dx%d", img.Width, img.Height)
	}
	if !hasVariance(img.Pixels) {
		t.Fatal("QNT 像素无变化")
	}

	// AJP：带 zlib 掩码，解码后 alpha 应有 0（系统图大量透明区）
	var ajpName string
	for name, v := range byName {
		if bytes.HasPrefix(v, []byte("AJP\x00")) {
			ajpName = name
			break
		}
	}
	if ajpName == "" {
		t.Fatal("没找到 AJP 样本")
	}
	img, err = Decode(byName[ajpName])
	if err != nil {
		t.Fatalf("AJP %s 解码失败：%v", ajpName, err)
	}
	zeros := 0
	for p := 0; p < img.Width*img.Height; p++ {
		if img.Pixels[p*4+3] == 0 {
			zeros++
		}
	}
	if zeros == 0 {
		t.Fatalf("AJP %s alpha 无透明像素（掩码语义存疑）", ajpName)
	}

	// DCF：遍历取若干样本——底图存在的验证合成生效（结果 != 裸差分），
	// 底图悬空的验证降级直解不报错（真实语料两种都有）。
	var dcfNames []string
	for name, v := range byName {
		if bytes.HasPrefix(v, []byte("dcf ")) && strings.HasSuffix(name, ".dcf") {
			dcfNames = append(dcfNames, name)
		}
	}
	if len(dcfNames) == 0 {
		t.Fatal("没找到 DCF 样本")
	}
	composed, degraded := 0, 0
	const maxProbe = 120 // 每张 DCF 是 1080p 级解码，探测到两种路径都有即可
	for i, dcfName := range dcfNames {
		if i >= maxProbe {
			break
		}
		dcf, err := DecodeDCF(byName[dcfName], lookup)
		if err != nil {
			t.Fatalf("DCF %s 解码失败：%v", string(dcfName), err)
		}
		if dcf.Width <= 0 || dcf.Height <= 0 {
			t.Fatalf("DCF %s 尺寸非法 %dx%d", string(dcfName), dcf.Width, dcf.Height)
		}
		diffImg, err := Decode(extractDCGD(t, byName[dcfName]))
		if err != nil {
			t.Fatalf("DCF %s 差分 QNT 裸解失败：%v", string(dcfName), err)
		}
		if bytes.Equal(dcf.Pixels, diffImg.Pixels) {
			degraded++
		} else {
			composed++
		}
		if composed > 0 && degraded > 0 {
			break
		}
	}
	if composed == 0 {
		t.Fatal("所有 DCF 都走了降级路径，合成逻辑未被真实语料覆盖")
	}
}

// extractDCGD 取 DCF 内嵌的差分 QNT 字节。
func extractDCGD(t *testing.T, dcf []byte) []byte {
	t.Helper()
	hdrSize := int(int32(binary.LittleEndian.Uint32(dcf[4:])))
	p := 8 + hdrSize
	if string(dcf[p:p+4]) != "dfdl" {
		t.Fatal("DCF 缺 dfdl")
	}
	dfdlSize := int(int32(binary.LittleEndian.Uint32(dcf[p+4:])))
	q := p + 8 + dfdlSize
	if string(dcf[q:q+4]) != "dcgd" {
		t.Fatal("DCF 缺 dcgd")
	}
	dcgdSize := int(int32(binary.LittleEndian.Uint32(dcf[q+4:])))
	return dcf[q+8 : q+8+dcgdSize]
}

func hasVariance(px []byte) bool {
	first := px[0]
	for _, b := range px {
		if b != first {
			return true
		}
	}
	return false
}

