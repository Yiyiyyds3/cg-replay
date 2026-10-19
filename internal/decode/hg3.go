// HG3（CatSystem2 引擎图片，.hg3）解码。
//
// 依据 GARbro `ArcFormats/CatSystem/ImageHG3.cs` 与 `ArcHG3.cs`，并用
// 《アマカノ2+（甜蜜女友2+）》4645 个实测样本（image00 / image01 / cndate04）
// 与 GARbro 转出的参考 PNG 逐一核对。
//
// 文件结构（base = "stdinfo" 字符串所在位置；单图文件恒为 0x14）：
//
//	base+0x00  "stdinfo\0"（8 字节）
//	base+0x08  headerSize（小端 u32，相对 base；图像段从 base+headerSize 开始）
//	base+0x10  width        base+0x14  height      base+0x18  bpp（24 或 32）
//	base+0x1C  offsetX      base+0x20  offsetY
//	base+0x30  canvasWidth  base+0x34  canvasHeight
//
// 一个 .hg3 可含多段（多张分层图，例如换了差分部件）：段起始于 0x0C，
// 每段形如 u32 sectionSize + "stdinfo"，段内布局同上；下段起点 = 本段起点 + sectionSize
// （sectionSize 为 0 表示直到文件尾）。
//
// 图像段（base+headerSize 起）：8 字节类型标签，实测样本全是 "img0000\0"：
//
//	+0x18  packedDataSize   +0x1C dataSize
//	+0x20  packedCtlSize    +0x24 ctlSize
//	+0x28  数据段（zlib，解出 dataSize 字节）
//	其后 控制段（zlib，解出 ctlSize 字节；LSB 优先位流）
//
// 控制位流语义：首个 bit = copy 标志；随后 GetBitCount 得「输出总长」；
// 之后循环 GetBitCount 得 run 长度 —— copy 为真时从数据段顺序搬运 run 字节，
// 为假时该段填 0，copy 取反。搬运完的缓冲再做两级差分还原（先首行横向、再逐行纵向），
// 最后行序上下翻转（img0000 是自下而上存的）。
package decode

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
)

var hg3Magic = []byte("HG-3")

const hg3MaxDim = 1 << 16

// Hg3Section 描述一个图像段（一个 .hg3 里的第 i 张图）的头部信息。
type Hg3Section struct {
	Index      int
	Base       int // "stdinfo" 在文件中的偏移
	HeaderSize int // 相对 Base 的图像段偏移
	Width      int
	Height     int
	BPP        int
	OffsetX    int
	OffsetY    int
	CanvasW    int
	CanvasH    int
}

// Hg3Sections 列出 .hg3 内全部图像段。单图文件返回 1 段。
func Hg3Sections(data []byte) ([]Hg3Section, error) {
	if !bytes.HasPrefix(data, hg3Magic) || len(data) < 0x60 {
		return nil, fmt.Errorf("不是 HG3 文件（魔数 %q）", head(data, 4))
	}
	if binary.LittleEndian.Uint32(data[4:]) != 0x0c {
		return nil, fmt.Errorf("HG3 版本号异常（%d）", binary.LittleEndian.Uint32(data[4:]))
	}
	var out []Hg3Section
	off := 0x0c
	for off+0x14 < len(data) && bytes.HasPrefix(data[off+8:], []byte("stdinfo")) {
		secSize := int(binary.LittleEndian.Uint32(data[off:]))
		if secSize == 0 {
			secSize = len(data) - off
		}
		if secSize < 0x14 || off+secSize > len(data) {
			break
		}
		s, ok := parseHg3Section(data, off+8, len(out))
		if ok {
			out = append(out, s)
		}
		off += secSize
	}
	if len(out) == 0 {
		// 退回单图路径：GARbro 的 Hg3Format 直接认 0x14 处的头。
		if s, ok := parseHg3Section(data, 0x14, 0); ok {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("HG3 未找到图像段")
	}
	return out, nil
}

// parseHg3Section 解析 base 处的一段头；base+headerSize 必须紧接 "img" 标签。
func parseHg3Section(data []byte, base, index int) (Hg3Section, bool) {
	if base < 0 || base+0x38 > len(data) || !bytes.HasPrefix(data[base:], []byte("stdinfo")) {
		return Hg3Section{}, false
	}
	hs := int(binary.LittleEndian.Uint32(data[base+8:]))
	if hs <= 0 || base+hs+8 > len(data) || !bytes.HasPrefix(data[base+hs:], []byte("img")) {
		return Hg3Section{}, false
	}
	s := Hg3Section{
		Index:      index,
		Base:       base,
		HeaderSize: hs,
		Width:      int(binary.LittleEndian.Uint32(data[base+0x10:])),
		Height:     int(binary.LittleEndian.Uint32(data[base+0x14:])),
		BPP:        int(int32(binary.LittleEndian.Uint32(data[base+0x18:]))),
		OffsetX:    int(int32(binary.LittleEndian.Uint32(data[base+0x1c:]))),
		OffsetY:    int(int32(binary.LittleEndian.Uint32(data[base+0x20:]))),
		CanvasW:    int(binary.LittleEndian.Uint32(data[base+0x30:])),
		CanvasH:    int(binary.LittleEndian.Uint32(data[base+0x34:])),
	}
	if s.Width <= 0 || s.Height <= 0 || s.Width > hg3MaxDim || s.Height > hg3MaxDim {
		return Hg3Section{}, false
	}
	return s, true
}

// DecodeHG3 解出 .hg3 内指定段（index 从 0 起）。
func DecodeHG3(data []byte, index int) (*Image, error) {
	secs, err := Hg3Sections(data)
	if err != nil {
		return nil, err
	}
	if index < 0 || index >= len(secs) {
		return nil, fmt.Errorf("HG3 段序号 %d 越界（共 %d 段）", index, len(secs))
	}
	return decodeHg3Section(data, secs[index])
}

// DecodeHG3All 解出 .hg3 内全部分层图（顺序同段序）。
func DecodeHG3All(data []byte) ([]*Image, error) {
	secs, err := Hg3Sections(data)
	if err != nil {
		return nil, err
	}
	out := make([]*Image, 0, len(secs))
	for _, s := range secs {
		im, err := decodeHg3Section(data, s)
		if err != nil {
			return nil, fmt.Errorf("第 %d 段：%w", s.Index, err)
		}
		out = append(out, im)
	}
	return out, nil
}

func decodeHg3Section(data []byte, s Hg3Section) (*Image, error) {
	if s.BPP != 24 && s.BPP != 32 {
		return nil, fmt.Errorf("HG3 仅支持 24/32 位色深，实为 %d", s.BPP)
	}
	p := s.Base + s.HeaderSize
	if p+8 > len(data) {
		return nil, fmt.Errorf("HG3 图像段越界")
	}
	tag := data[p : p+8]
	switch {
	case bytes.Equal(tag, []byte("img0000\x00")):
		return decodeHg3Img0000(data, s, p)
	case bytes.Equal(tag, []byte("img_jpg\x00")):
		return nil, fmt.Errorf("HG3 img_jpg（JPEG + 独立 alpha 段）暂未实现：本机 4645 个样本中 0 例")
	case bytes.Equal(tag, []byte("img_wbp\x00")):
		return nil, fmt.Errorf("HG3 img_wbp（WebP）暂未实现")
	default:
		return nil, fmt.Errorf("HG3 未知图像类型 %q", string(tag))
	}
}

func decodeHg3Img0000(data []byte, s Hg3Section, p int) (*Image, error) {
	if p+0x28 > len(data) {
		return nil, fmt.Errorf("HG3 图像段头越界")
	}
	packedData := int(binary.LittleEndian.Uint32(data[p+0x18:]))
	dataSize := int(binary.LittleEndian.Uint32(data[p+0x1c:]))
	packedCtl := int(binary.LittleEndian.Uint32(data[p+0x20:]))
	ctlSize := int(binary.LittleEndian.Uint32(data[p+0x24:]))
	dataOff := p + 0x28
	if packedData < 0 || packedCtl < 0 || dataOff+packedData+packedCtl > len(data) {
		return nil, fmt.Errorf("HG3 数据段越界（data %d + ctl %d 字节）", packedData, packedCtl)
	}
	raw, err := hg3Inflate(data[dataOff:dataOff+packedData], dataSize)
	if err != nil {
		return nil, fmt.Errorf("HG3 数据段解压失败: %w", err)
	}
	ctl, err := hg3Inflate(data[dataOff+packedData:dataOff+packedData+packedCtl], ctlSize)
	if err != nil {
		return nil, fmt.Errorf("HG3 控制段解压失败: %w", err)
	}

	br := &lsbBits{b: ctl}
	copyFlag := br.bit() != 0
	outSize, err := hg3BitCount(br)
	if err != nil {
		return nil, fmt.Errorf("HG3 控制流读取输出长度失败: %w", err)
	}
	if outSize <= 0 || outSize > 1<<30 {
		return nil, fmt.Errorf("HG3 控制流输出长度异常（%d）", outSize)
	}
	plane := make([]byte, outSize)
	src, dst := 0, 0
	for dst < outSize {
		n, err := hg3BitCount(br)
		if err != nil {
			return nil, fmt.Errorf("HG3 控制流读取 run 长度失败（已解 %d/%d）: %w", dst, outSize, err)
		}
		if copyFlag {
			if src+n > len(raw) || dst+n > outSize {
				return nil, fmt.Errorf("HG3 控制流搬运越界（src %d+%d > %d，dst %d+%d > %d）",
					src, n, len(raw), dst, n, outSize)
			}
			copy(plane[dst:], raw[src:src+n])
			src += n
		}
		dst += n
		copyFlag = !copyFlag
	}

	pixels, err := hg3ApplyDelta(plane, s)
	if err != nil {
		return nil, err
	}
	// img0000 的像素是自下而上存的，翻回自上而下。
	stride := s.Width * (s.BPP / 8)
	flipped := make([]byte, len(pixels))
	for y := 0; y < s.Height; y++ {
		copy(flipped[y*stride:], pixels[(s.Height-1-y)*stride:(s.Height-y)*stride])
	}
	colors := 4
	if s.BPP == 24 {
		colors = 3
	}
	return &Image{Width: s.Width, Height: s.Height, Colors: colors, Pixels: flipped}, nil
}

// hg3Inflate 解一段 zlib，要求恰好得到 want 字节。
func hg3Inflate(src []byte, want int) ([]byte, error) {
	if want < 0 {
		return nil, fmt.Errorf("解压长度非法（%d）", want)
	}
	zr, err := zlib.NewReader(bytes.NewReader(src))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	out := make([]byte, want)
	if _, err := io.ReadFull(zr, out); err != nil {
		return nil, err
	}
	return out, nil
}

// hg3ApplyDelta 还原 img0000 的像素：四平面 2 位组交织 → 单字节反符号 → 两级差分。
//
// 交织：每个像素 4 字节，第 k 个平面只贡献该字节的 2 位（表 table[k]）；
// 差分：先对第 0 行做横向累加，再自第 1 行起逐行纵向累加（与 GARbro 逐句对应）。
func hg3ApplyDelta(plane []byte, s Hg3Section) ([]byte, error) {
	var table [4][0x100]uint32
	for i := 0; i < 0x100; i++ {
		val := uint32(i) & 0xC0
		val <<= 6
		val |= uint32(i) & 0x30
		val <<= 6
		val |= uint32(i) & 0x0C
		val <<= 6
		val |= uint32(i) & 0x03
		table[0][i] = val << 6
		table[1][i] = val << 4
		table[2][i] = val << 2
		table[3][i] = val
	}
	if len(plane)%4 != 0 {
		return nil, fmt.Errorf("HG3 平面数据长度 %d 不是 4 的倍数", len(plane))
	}
	planeSize := len(plane) / 4
	out := make([]byte, len(plane))
	dst := 0
	p0, p1, p2, p3 := 0, planeSize, 2*planeSize, 3*planeSize
	for dst+4 <= len(out) {
		val := table[0][plane[p0]] | table[1][plane[p1]] | table[2][plane[p2]] | table[3][plane[p3]]
		p0++
		p1++
		p2++
		p3++
		out[dst] = hg3ConvertValue(byte(val))
		out[dst+1] = hg3ConvertValue(byte(val >> 8))
		out[dst+2] = hg3ConvertValue(byte(val >> 16))
		out[dst+3] = hg3ConvertValue(byte(val >> 24))
		dst += 4
	}
	pixelSize := s.BPP / 8
	stride := s.Width * pixelSize
	if stride*s.Height > len(out) {
		return nil, fmt.Errorf("HG3 尺寸与像素数据不符（需要 %d 字节，实有 %d）", stride*s.Height, len(out))
	}
	for x := pixelSize; x < stride; x++ { // 首行横向累加
		out[x] += out[x-pixelSize]
	}
	prev := 0
	for y := 1; y < s.Height; y++ { // 逐行纵向累加
		line := prev + stride
		for x := 0; x < stride; x++ {
			out[line+x] += out[prev+x]
		}
		prev = line
	}
	return out, nil
}

// hg3ConvertValue 单字节反符号：最低位为 1 时，高 7 位取反。
func hg3ConvertValue(v byte) byte {
	carry := v&1 != 0
	v >>= 1
	if carry {
		return v ^ 0xFF
	}
	return v
}

// lsbBits 是 LSB 优先的位读取器（对应 GARbro BitStream.cs 的 LsbBitStream）。
type lsbBits struct {
	b   []byte
	pos int
	cur byte
	n   uint
}

func (r *lsbBits) bit() int {
	if r.n == 0 {
		if r.pos >= len(r.b) {
			return 0 // 位流读尽：GARbro 返回 -1，这里按 0 处理并靠上层长度校验兜底
		}
		r.cur = r.b[r.pos]
		r.pos++
		r.n = 8
	}
	v := int(r.cur & 1)
	r.cur >>= 1
	r.n--
	return v
}

// hg3BitCount 读一个「一元长度 + 若干位」编码的计数（GARbro HgReader.GetBitCount）。
func hg3BitCount(r *lsbBits) (int, error) {
	n := 0
	for r.bit() == 0 {
		n++
		if n >= 0x20 {
			return 0, fmt.Errorf("位计数溢出")
		}
	}
	value := 1
	for ; n > 0; n-- {
		value = (value << 1) | r.bit()
	}
	return value, nil
}

func head(b []byte, n int) []byte {
	if len(b) < n {
		return b
	}
	return b[:n]
}
