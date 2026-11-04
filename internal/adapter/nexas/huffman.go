// Package nexas —— 戏画 NeXAS 引擎适配器（Baldr Sky 系列）。
//
// PAC 归档：头 12 字节（"PAC"+标记字节 + i32 条目数 + i32 打包版本），
// 数据区从 12 起是连续压缩流（版本 4=zlib / 3=LZSS / 0=直存 / 7=zstd）；
// 索引在文件尾（末 4 字节 = 索引字节数），逐字节取反后是 NeXAS 变体的
// Huffman 流（位 1=内部节点先左后右、位 0=叶子跟 8 位符号值）。
// 条目 76 字节：name[64] + offset + 原始长 + 打包长。
//
// 本机语料：Baldr Sky（Steam 官方中文版，2019）与 BALDRSKY DIVEX（汉化版）。
// 文本与条目名都是 GBK（汉化组连索引名字符一起转了码），日文原版则是 CP932
// ——解码按脚本投票嗅探。
package nexas

import "errors"

// neXASHuffmanDecode 解 NeXAS 变体 Huffman（输入应先逐字节取反）。
//
// 位流 MSB-first：1=内部节点（token 递增，先左子树后右子树递归），
// 0=叶子（跟 8 位符号值）。树最多 256 槽（255 个内部节点）；叶子用
// 「值+1024」表示，避免内部/叶子节点混淆。
func neXASHuffmanDecode(packed []byte, outLen int) ([]byte, error) {
	inv := make([]byte, len(packed))
	for i, v := range packed {
		inv[i] = ^v
	}
	bs := &bitStream{data: inv}
	type node struct{ l, r uint16 }
	tree := make([]node, 256)
	free := 0
	var build func() (uint16, error)
	build = func() (uint16, error) {
		b, ok := bs.nextBit()
		if !ok {
			return 0, errors.New("huffman: 位流耗尽")
		}
		if b == 1 {
			if free >= 256 {
				return 0, errors.New("huffman: 内部节点超 256（不是 NeXAS 索引）")
			}
			r := uint16(free)
			free++
			l, err := build()
			if err != nil {
				return 0, err
			}
			rr, err := build()
			if err != nil {
				return 0, err
			}
			tree[r] = node{l, rr}
			return r, nil
		}
		v, ok := bs.nextByte()
		if !ok {
			return 0, errors.New("huffman: 叶子值耗尽")
		}
		return uint16(v) + 1024, nil
	}
	root, err := build()
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, outLen)
	for len(out) < outLen {
		t := root
		for t < 1024 {
			b, ok := bs.nextBit()
			if !ok {
				// 位流末尾差几个位（官方打包器的尾部不补齐位）：
				// 返回已解出的部分，调用方对残缺条目自行兜底。
				return out, nil
			}
			n := tree[t]
			if b == 1 {
				t = n.r
			} else {
				t = n.l
			}
		}
		out = append(out, byte(t-1024))
	}
	return out, nil
}

// bitStream MSB-first 位流（与 NeXAS 引擎的 BitStream 一致）。
type bitStream struct {
	data    []byte
	bytePos int
	bitPos  int
}

func (b *bitStream) nextBit() (byte, bool) {
	if b.bytePos >= len(b.data) {
		return 0, false
	}
	r := (b.data[b.bytePos] >> (7 - b.bitPos)) & 1
	b.bitPos++
	if b.bitPos == 8 {
		b.bitPos = 0
		b.bytePos++
	}
	return r, true
}

func (b *bitStream) nextByte() (byte, bool) {
	if b.bitPos > 0 && b.bytePos >= len(b.data)-1 {
		return 0, false
	}
	if b.bitPos == 0 {
		v := b.data[b.bytePos]
		b.bytePos++
		return v, true
	}
	p1 := b.data[b.bytePos] << b.bitPos
	b.bytePos++
	if b.bytePos >= len(b.data) {
		return 0, false
	}
	p2 := b.data[b.bytePos] >> (8 - b.bitPos)
	return p1 | p2, true
}

// lzssDecode NeXAS 变体 LZSS（奥村式改）：窗口 4096、最长匹配 18、
// 阈值 2、标志字节 MSB-first（1=字面量，0=引用：低 8 位位置 + 高 4 位长度）。
func lzssDecode(src []byte, outLen int) []byte {
	const (
		windowSize      = 4096
		maxMatch        = 18
		threshold       = 2
	)
	out := make([]byte, 0, outLen)
	var window [windowSize]byte
	win := windowSize - maxMatch
	en := 0
	for en < len(src) {
		flags := src[en]
		en++
		for bit := 7; bit >= 0 && en < len(src); bit-- {
			if (flags>>bit)&1 != 0 {
				out = append(out, src[en])
				window[win] = src[en]
				win = (win + 1) & (windowSize - 1)
				en++
				if len(out) >= outLen {
					return out[:outLen]
				}
			} else {
				if en+1 >= len(src) {
					return out
				}
				pos := int(src[en])
				length := int(src[en+1])
				en += 2
				pos |= (length >> 4) << 8
				length = length&0x0f + threshold + 1
				for i := 0; i < length && len(out) < outLen; i++ {
					v := window[(pos+i)&(windowSize-1)]
					out = append(out, v)
					window[win] = v
					win = (win + 1) & (windowSize - 1)
				}
				if len(out) >= outLen {
					return out[:outLen]
				}
			}
		}
	}
	return out
}
