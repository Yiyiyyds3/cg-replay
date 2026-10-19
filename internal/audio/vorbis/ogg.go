// Package vorbis 是一个零第三方依赖的 Ogg Vorbis 解码器。
//
// 只实现解码所需的部分：Ogg 容器（页/分段/包重组）、Vorbis I 的
// 三个头（identification/comment/setup）、codebook（Huffman + VQ）、
// floor1（floor0 保留未实现）、residue 0/1/2、声道耦合、IMDCT 与重叠相加。
//
// 设计取舍：音频文件通常只有几 MB，因此整文件读入内存后按页解析，
// 不做流式状态机；这样代码简单且便于逐阶段对拍验证。
package vorbis

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// oggCapture Ogg 页的捕获串。
const oggCapture = "OggS"

// ErrNotOgg 输入不是 Ogg 流。
var ErrNotOgg = errors.New("vorbis: 不是 Ogg 流")

// oggPage 一页 Ogg。
type oggPage struct {
	Type     byte
	Granule  int64
	Serial   uint32
	Seq      uint32
	Segments []byte
	Body     []byte
}

// oggPacket 一个重组后的包，连同它结束所在页的 granule position。
type oggPacket struct {
	Data    []byte
	Granule int64
}

// parsePages 把整个 Ogg 流切成页。
func parsePages(data []byte) ([]oggPage, error) {
	var pages []oggPage
	for off := 0; off+27 <= len(data); {
		if string(data[off:off+4]) != oggCapture {
			return nil, ErrNotOgg
		}
		if data[off+4] != 0 {
			return nil, fmt.Errorf("vorbis: 不支持的 Ogg 版本 %d", data[off+4])
		}
		p := oggPage{
			Type:    data[off+5],
			Granule: int64(binary.LittleEndian.Uint64(data[off+6:])),
			Serial:  binary.LittleEndian.Uint32(data[off+14:]),
			Seq:     binary.LittleEndian.Uint32(data[off+18:]),
		}
		nseg := int(data[off+26])
		if off+27+nseg > len(data) {
			return nil, fmt.Errorf("vorbis: Ogg 页分段表越界")
		}
		p.Segments = data[off+27 : off+27+nseg]
		bodyLen := 0
		for _, s := range p.Segments {
			bodyLen += int(s)
		}
		start := off + 27 + nseg
		if start+bodyLen > len(data) {
			return nil, fmt.Errorf("vorbis: Ogg 页数据越界")
		}
		p.Body = data[start : start+bodyLen]
		pages = append(pages, p)
		off = start + bodyLen
	}
	if len(pages) == 0 {
		return nil, ErrNotOgg
	}
	return pages, nil
}

// packets 按 serial 把页重组成包。一个包可以跨页，由分段表中的 255 续段表示。
func packets(pages []oggPage, serial uint32) []oggPacket {
	var out []oggPacket
	var cur []byte
	started := false
	for _, p := range pages {
		if p.Serial != serial {
			continue
		}
		if p.Type&0x01 == 0 {
			// 非续页：丢弃未完成的残包（正常流不会出现）。
			cur = nil
			started = false
		} else if !started {
			cur = nil
		}
		body := p.Body
		for _, seg := range p.Segments {
			n := int(seg)
			if n > len(body) {
				n = len(body)
			}
			cur = append(cur, body[:n]...)
			body = body[n:]
			if seg < 255 {
				// 包在此结束。
				pkt := make([]byte, len(cur))
				copy(pkt, cur)
				out = append(out, oggPacket{Data: pkt, Granule: p.Granule})
				cur = nil
				started = false
			} else {
				started = true
			}
		}
	}
	return out
}

// selectStream 找到携带 Vorbis identification 头的 logical stream。
func selectStream(pages []oggPage) (uint32, bool) {
	seen := map[uint32]bool{}
	for _, p := range pages {
		if p.Type&0x02 == 0 || seen[p.Serial] {
			continue
		}
		seen[p.Serial] = true
		if len(p.Body) >= 7 && p.Body[0] == 1 && string(p.Body[1:7]) == "vorbis" {
			return p.Serial, true
		}
	}
	return 0, false
}
