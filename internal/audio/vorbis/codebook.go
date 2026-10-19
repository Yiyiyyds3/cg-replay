package vorbis

import (
	"fmt"
	"math"
)

// codebook 是规范 §3 的码本：一张 Huffman 码表，可选带一张 VQ 乘数表。
type codebook struct {
	dimensions int
	entries    int

	// lengths[i] 是 entry i 的码字长度；0 表示该 entry 未使用。
	lengths []int

	// Huffman 解码树（按位走）。
	nodes []huffNode

	// VQ 查表（lookupType 1/2）；lookupType 0 时为空。
	lookupType    int
	minimum       float32
	delta         float32
	valueBits     int
	sequenceP     bool
	multiplicands []uint32
	lookupValues  int

	// singleBit 表示这是「只有一个 entry」的码本：读值时固定返回该 entry，
	// 但仍要吃掉 1 bit（errata 20150226）。
	singleBit bool
}

// huffNode 是 Huffman 解码树的一个节点。叶子用 entry>=0 标记。
type huffNode struct {
	child [2]int32 // 子节点下标；-1 表示不存在
	entry int32    // >=0 表示叶子，值为 entry 下标
	leaf  bool
}

// decodeScalar 从码本读一个 entry 下标（§3.2.1）。
func (cb *codebook) decodeScalar(br *bitReader) int {
	return cb.decode(br)
}

// decode 走 Huffman 树读一个码字，返回 entry 下标。
func (cb *codebook) decode(br *bitReader) int {
	if cb.singleBit {
		br.read(1)
		return int(cb.nodes[0].entry)
	}
	if len(cb.nodes) == 0 {
		return 0
	}
	n := 0
	for {
		if cb.nodes[n].leaf {
			return int(cb.nodes[n].entry)
		}
		bit := br.read(1)
		next := cb.nodes[n].child[bit]
		if next < 0 {
			// 码流与码本不符：返回 0 并标记 overrun，避免死循环。
			br.overrun = true
			return 0
		}
		n = int(next)
	}
}

// decodeVQ 从码本读一个 VQ 向量，写入 out（长度必须 >= dimensions）。
func (cb *codebook) decodeVQ(br *bitReader, out []float32) {
	idx := cb.decode(br)
	cb.lookup(idx, out)
}

// lookup 把 entry 下标展开为 VQ 向量（§3.3）。
func (cb *codebook) lookup(index int, out []float32) {
	dim := cb.dimensions
	if cb.lookupType == 0 || len(cb.multiplicands) == 0 {
		// 无查表：维度 1 时值为 0；否则全 0（规范未定义，保守处理）。
		for i := 0; i < dim && i < len(out); i++ {
			out[i] = 0
		}
		return
	}

	last := float32(0)
	if cb.lookupType == 1 {
		divisor := 1
		for i := 0; i < dim; i++ {
			mo := (index / divisor) % cb.lookupValues
			v := float32(cb.multiplicands[mo])*cb.delta + cb.minimum + last
			out[i] = v
			if cb.sequenceP {
				last = v
			}
			divisor *= cb.lookupValues
		}
		return
	}

	// lookupType 2
	off := index * dim
	for i := 0; i < dim; i++ {
		mo := off + i
		if mo < 0 || mo >= len(cb.multiplicands) {
			out[i] = 0
			continue
		}
		v := float32(cb.multiplicands[mo])*cb.delta + cb.minimum + last
		out[i] = v
		if cb.sequenceP {
			last = v
		}
	}
}

// readCodebook 从 setup 头解出一个码本（§3.2.1）。
func readCodebook(br *bitReader) (*codebook, error) {
	sync := br.read(24)
	if sync != 0x564342 {
		return nil, fmt.Errorf("码本同步字错误：0x%06X", sync)
	}

	cb := &codebook{}
	cb.dimensions = int(br.read(16))
	cb.entries = int(br.read(24))
	if cb.entries <= 0 || cb.entries > 1<<24 {
		return nil, fmt.Errorf("码本 entry 数非法：%d", cb.entries)
	}
	if cb.dimensions <= 0 {
		return nil, fmt.Errorf("码本维度非法：%d", cb.dimensions)
	}

	cb.lengths = make([]int, cb.entries)
	ordered := br.read(1) != 0
	if ordered {
		currentEntry := 0
		currentLength := int(br.read(5)) + 1
		for currentEntry < cb.entries {
			number := int(br.read(ilog(uint32(cb.entries - currentEntry))))
			// number 允许为 0：表示该长度没有 entry，直接进入下一长度。
			if currentEntry+number > cb.entries {
				return nil, fmt.Errorf("码本有序长度表越界")
			}
			for i := 0; i < number; i++ {
				cb.lengths[currentEntry+i] = currentLength
			}
			currentEntry += number
			currentLength++
			if currentLength > 32 {
				return nil, fmt.Errorf("码本码字长度超过 32")
			}
		}
	} else {
		sparse := br.read(1) != 0
		for i := 0; i < cb.entries; i++ {
			if sparse {
				if br.read(1) != 0 {
					cb.lengths[i] = int(br.read(5)) + 1
				}
			} else {
				cb.lengths[i] = int(br.read(5)) + 1
			}
		}
	}

	cb.lookupType = int(br.read(4))
	switch cb.lookupType {
	case 0:
		// 无 VQ 表。
	case 1, 2:
		cb.minimum = float32Unpack(br.read(32))
		cb.delta = float32Unpack(br.read(32))
		cb.valueBits = int(br.read(4)) + 1
		cb.sequenceP = br.read(1) != 0
		if cb.lookupType == 1 {
			cb.lookupValues = lookup1Values(cb.entries, cb.dimensions)
		} else {
			cb.lookupValues = cb.entries * cb.dimensions
		}
		if cb.lookupValues <= 0 {
			return nil, fmt.Errorf("码本 VQ 表长度非法：%d", cb.lookupValues)
		}
		cb.multiplicands = make([]uint32, cb.lookupValues)
		for i := 0; i < cb.lookupValues; i++ {
			cb.multiplicands[i] = br.read(cb.valueBits)
		}
	default:
		return nil, fmt.Errorf("码本 lookup type 非法：%d", cb.lookupType)
	}

	if err := cb.buildHuffman(); err != nil {
		return nil, err
	}
	return cb, nil
}

// buildHuffman 按 Vorbis 规范的 canonical 顺序为每个已用 entry 分配码字。
//
// 注意：码字不是「按长度分组后依次分配」，而是按 **entry 出现顺序** 用
// libvorbis sharedbook.c 的 marker 算法分配；两种分配在长度交错时结果不同，
// 解码端必须与编码端完全一致，否则读出的 Huffman entry 会整体错位。
func (cb *codebook) buildHuffman() error {
	anyUsed := false
	single := -1
	nUsed := 0
	for i, l := range cb.lengths {
		if l > 0 {
			if l > 32 {
				return fmt.Errorf("码本码字长度 %d 超过 32", l)
			}
			anyUsed = true
			nUsed++
			single = i
		}
	}
	if !anyUsed {
		// 空码本：给一个根叶子，读任何位都返回 0。
		cb.nodes = []huffNode{{leaf: true, entry: 0}}
		return nil
	}

	// 单 entry 码本特例（errata 20150226）：唯一码字长度必须为 1，且要吃掉 1 bit。
	if nUsed == 1 {
		cb.nodes = []huffNode{{leaf: true, entry: int32(single)}}
		cb.singleBit = true
		return nil
	}

	// 建树：根为 0 号节点。
	cb.nodes = []huffNode{{child: [2]int32{-1, -1}}}
	var marker [33]uint32

	for entry, l := range cb.lengths {
		if l == 0 {
			continue
		}
		cw := marker[l]
		if l < 32 && cw>>uint(l) != 0 {
			return fmt.Errorf("码本 overspecified（entry %d 长度 %d 处码字溢出）", entry, l)
		}

		// 从 MSb 到 LSb 逐位插入。
		n := 0
		for bitPos := l - 1; bitPos >= 0; bitPos-- {
			bit := int((cw >> uint(bitPos)) & 1)
			if cb.nodes[n].leaf {
				return fmt.Errorf("码本码字冲突（前缀重复）")
			}
			child := cb.nodes[n].child[bit]
			if child < 0 {
				cb.nodes = append(cb.nodes, huffNode{child: [2]int32{-1, -1}})
				child = int32(len(cb.nodes) - 1)
				cb.nodes[n].child[bit] = child
			}
			n = int(child)
		}
		if !cb.nodes[n].leaf && (cb.nodes[n].child[0] >= 0 || cb.nodes[n].child[1] >= 0) {
			return fmt.Errorf("码本码字是其它码字的前缀")
		}
		cb.nodes[n].leaf = true
		cb.nodes[n].entry = int32(entry)

		// marker 递推（逐字对应 libvorbis sharedbook.c 的 _make_words）。
		for j := l; j > 0; j-- {
			if marker[j]&1 != 0 {
				if j == 1 {
					marker[1]++
				} else {
					marker[j] = marker[j-1] << 1
				}
				break
			}
			marker[j]++
		}
		// 把悬在刚占用节点下面的更长 marker 挪到新节点上。
		cur := cw
		for j := l + 1; j < 33; j++ {
			if marker[j]>>1 == cur {
				cur = marker[j]
				marker[j] = marker[j-1] << 1
			} else {
				break
			}
		}
	}
	return nil
}

// float32Unpack 按 §9.2.2 解包一个 32bit 浮点描述。
func float32Unpack(x uint32) float32 {
	mantissa := int32(x & 0x1fffff)
	sign := x & 0x80000000
	exponent := int32((x & 0x7fe00000) >> 21)
	if sign != 0 {
		mantissa = -mantissa
	}
	return float32(math.Ldexp(float64(mantissa), int(exponent)-788))
}
