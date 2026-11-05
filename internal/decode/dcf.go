// dcf.go —— AliceSoft DCF 差分 CG 解码。
//
// DCF 是 System 4 的「差分 CG」格式：一张 CG 的某个差分只存**与底图不同的
// 16×16 块**，其余块直接继承底图。兰斯10 的 CG.afa 里 2083/3460 条是 DCF
// （差分链常为 差分４→差分２→原图，底图自身也可能是 DCF，需递归合成）。
//
// 结构（据 alice-tools libsys4 dcf.c，已用真实语料核对）：
//
//	dcf 段：magic "dcf " + u32 header_size（不含 magic+size 的 8 字节）
//	       + u32 version（==1）+ u32 width + u32 height + u32 bpp（==32）
//	       + u32 name_len + name[name_len]
//	       name 每字节循环左移 rot=(name_len%7)+1 解密，是**底图条目名**
//	       （实测带 .png 之类假后缀，实际内容是 QNT，须去后缀按词干查）
//	dfdl 段：magic "dfdl" + u32 section_size + u32 uncompressed_size
//	       + zlib 流（section_size-4 字节）→ chunk map
//	       chunk map 前 u32 == uncompressed_size-4（即块数）作校验；
//	       之后每字节对应一个 16×16 块，**0 = 与底图不同，需从差分图贴**
//	dcgd 段：magic "dcgd" + u32 section_size + 完整 QNT 差分图（整图尺寸）
//
// 合成：把底图拷出来，凡 chunk map 为 0 的块从差分图对应位置整块覆盖；
// 宽/高不足 16 的边缘余量整条覆盖（libsys4 dcf_apply_diff 同款）。
// 底图悬空时降级直解差分 QNT（libsys4 同款：qnt_extract(cg_data)）。
//
// 底图查找：DecodeDCF 需要调用方注入 lookup（给词干、回原始字节），
// 因为底图在 AFA 归档里、且链式差分要求 lookup 内部能再递归 DCF。
package decode

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// DecodeDCF 解出 DCF 差分图。lookup 接收底图词干（去扩展名前的原始名字
// 由调用方决定怎么归一），返回底图原始字节；底图若还是 DCF，lookup 内部
// 应再走 DecodeDCF 形成递归。
func DecodeDCF(data []byte, lookup func(stem string) ([]byte, error)) (*Image, error) {
	if len(data) < 28 || !bytes.HasPrefix(data, []byte("dcf ")) {
		return nil, fmt.Errorf("非 DCF 文件（魔数 %q）", string(data[:min(4, len(data))]))
	}
	hdrSize := int(int32(binary.LittleEndian.Uint32(data[4:])))
	if hdrSize < 20 || 8+hdrSize > len(data) {
		return nil, fmt.Errorf("DCF header_size=%d 非法（文件 %d 字节）", hdrSize, len(data))
	}
	ver := int32(binary.LittleEndian.Uint32(data[8:]))
	if ver != 1 {
		return nil, fmt.Errorf("DCF version=%d 不受支持（应 1）", ver)
	}
	w := int(int32(binary.LittleEndian.Uint32(data[12:])))
	h := int(int32(binary.LittleEndian.Uint32(data[16:])))
	bpp := int32(binary.LittleEndian.Uint32(data[20:]))
	nameLen := int(int32(binary.LittleEndian.Uint32(data[24:])))
	if w <= 0 || h <= 0 || w > 1<<16 || h > 1<<16 {
		return nil, fmt.Errorf("DCF 尺寸非法：%dx%d", w, h)
	}
	if bpp != 32 {
		return nil, fmt.Errorf("DCF bpp=%d 不受支持（应 32）", bpp)
	}
	if nameLen <= 0 || nameLen > 2000 || 28+nameLen != 8+hdrSize {
		return nil, fmt.Errorf("DCF 底图名长度 %d 非法（header_size=%d）", nameLen, hdrSize)
	}
	name := make([]byte, nameLen)
	copy(name, data[28:28+nameLen])
	rot := byte(nameLen%7) + 1
	for i := range name {
		name[i] = name[i]<<rot | name[i]>>(8-rot)
	}

	p := 8 + hdrSize
	cmap, body, err := dcfSections(data, p)
	if err != nil {
		return nil, err
	}
	// chunk map 校验：首 u32 == 块数
	chunks := int(int32(binary.LittleEndian.Uint32(cmap)))
	if chunks != len(cmap)-4 {
		return nil, fmt.Errorf("DCF chunk map 校验失败：%d != %d", chunks, len(cmap)-4)
	}
	if !bytes.HasPrefix(body, []byte("QNT\x00")) {
		return nil, fmt.Errorf("DCF 差分数据不是 QNT（魔数 %q）", string(body[:min(4, len(body))]))
	}
	diff, err := decodeQNT(body)
	if err != nil {
		return nil, fmt.Errorf("DCF 差分 QNT 解码失败：%w", err)
	}
	if diff.Width != w || diff.Height != h {
		return nil, fmt.Errorf("DCF 头尺寸 %dx%d 与差分 QNT 尺寸 %dx%d 不符",
			w, h, diff.Width, diff.Height)
	}

	if lookup == nil {
		return nil, fmt.Errorf("DCF 需要底图查找回调（底图 %q）", string(name))
	}
	baseData, err := lookup(string(name))
	if err != nil {
		// 底图悬空在真实语料中存在（兰斯10 CG.afa 有未被引用的差分），
		// 按 libsys4 同款策略降级：直解差分 QNT 返回（可能有底图区域空洞），
		// 不让整个事件出图失败。
		return diff, nil
	}
	// 底图可能是链式 DCF（实测 差分４→差分２→原图），递归处理
	var base *Image
	if bytes.HasPrefix(baseData, []byte("dcf ")) {
		base, err = DecodeDCF(baseData, lookup)
	} else {
		base, err = Decode(baseData)
	}
	if err != nil || base.Width != w || base.Height != h {
		// 底图坏/尺寸不符：同样降级，不拖垮整个事件
		return diff, nil
	}

	blitDiff(base.Pixels, diff.Pixels, w, h, cmap[4:])
	return base, nil
}

// dcfSections 依次解析 dfdl / dcgd 段，返回（解压的 chunk map, 差分 QNT 数据）。
func dcfSections(data []byte, p int) ([]byte, []byte, error) {
	// dfdl
	if p+12 > len(data) || !bytes.HasPrefix(data[p:], []byte("dfdl")) {
		return nil, nil, fmt.Errorf("DCF 缺 dfdl 段（偏移 %d）", p)
	}
	dfdlSize := int(int32(binary.LittleEndian.Uint32(data[p+4:])))
	if dfdlSize < 4 || p+8+dfdlSize > len(data) {
		return nil, nil, fmt.Errorf("DCF dfdl 段尺寸 %d 非法", dfdlSize)
	}
	uncSize := int(int32(binary.LittleEndian.Uint32(data[p+8:])))
	if uncSize <= 4 || uncSize > 40000 {
		return nil, nil, fmt.Errorf("DCF chunk map 解压尺寸 %d 非法", uncSize)
	}
	cmap, err := inflateExact(data[p+12:p+8+dfdlSize], uncSize)
	if err != nil {
		return nil, nil, fmt.Errorf("DCF chunk map 解压失败：%w", err)
	}
	// dcgd
	q := p + 8 + dfdlSize
	if q+8 > len(data) || !bytes.HasPrefix(data[q:], []byte("dcgd")) {
		return nil, nil, fmt.Errorf("DCF 缺 dcgd 段（偏移 %d）", q)
	}
	dcgdSize := int(int32(binary.LittleEndian.Uint32(data[q+4:])))
	if dcgdSize <= 0 || q+8+dcgdSize > len(data) {
		return nil, nil, fmt.Errorf("DCF dcgd 段尺寸 %d 非法", dcgdSize)
	}
	return cmap, data[q+8 : q+8+dcgdSize], nil
}

// blitDiff 按 chunk map（0=差异块）把差分图块覆盖到底图上。
func blitDiff(base, diff []byte, w, h int, cmap []byte) {
	cw, ch := w/16, h/16
	stride := w * 4
	blit := func(x, y, bw, bh int) {
		for row := 0; row < bh; row++ {
			off := stride*(y+row) + x*4
			copy(base[off:off+bw*4], diff[off:off+bw*4])
		}
	}
	for i, c := range cmap {
		if c != 0 || i >= cw*ch {
			continue
		}
		blit((i%cw)*16, (i/cw)*16, 16, 16)
	}
	if rw := w % 16; rw != 0 { // 边缘余量整条覆盖
		blit(cw*16, 0, rw, h)
	}
	if rh := h % 16; rh != 0 {
		blit(0, ch*16, w, rh)
	}
}
