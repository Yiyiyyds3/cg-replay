package vorbis

// bitReader 按 Vorbis 约定读位：字节内从低位到高位（LSB-first）。
type bitReader struct {
	data    []byte
	pos     int // 当前位位置
	overrun bool
}

func newBitReader(data []byte) *bitReader { return &bitReader{data: data} }

// read 读 n 位（0<=n<=32），返回无符号值。
func (b *bitReader) read(n int) uint32 {
	if n <= 0 {
		return 0
	}
	if n > 32 {
		n = 32
	}
	if b.pos+n > len(b.data)*8 {
		b.overrun = true
		b.pos = len(b.data) * 8
		return 0
	}
	idx := b.pos >> 3
	off := uint(b.pos & 7)
	var v uint64
	// 最多需要 ceil((off+n)/8) <= 5 字节。
	for i := 0; i < 5; i++ {
		if idx+i < len(b.data) {
			v |= uint64(b.data[idx+i]) << uint(8*i)
		}
	}
	v >>= off
	b.pos += n
	if n == 32 {
		return uint32(v)
	}
	return uint32(v & (uint64(1)<<uint(n) - 1))
}

// readSigned 读 n 位并按二补码解释为有符号数。
func (b *bitReader) readSigned(n int) int32 {
	v := b.read(n)
	if n == 0 {
		return 0
	}
	shift := 32 - uint(n)
	return int32(v<<shift) >> shift
}

// bitsLeft 剩余位数。
func (b *bitReader) bitsLeft() int {
	left := len(b.data)*8 - b.pos
	if left < 0 {
		return 0
	}
	return left
}

// skip 跳过 n 位。
func (b *bitReader) skip(n int) { b.pos += n }

// ilog 返回表示 x 所需的位数（x>0 时等价于 floor(log2(x))+1）。
func ilog(x uint32) int {
	n := 0
	for x > 0 {
		n++
		x >>= 1
	}
	return n
}

// lookup1Values 返回满足 r^dim <= entries 的最大 r。
func lookup1Values(entries, dim int) int {
	if dim <= 0 {
		return 0
	}
	r := 1
	for {
		p := 1
		ok := true
		for i := 0; i < dim; i++ {
			p *= r + 1
			if p > entries {
				ok = false
				break
			}
		}
		if !ok {
			break
		}
		r++
	}
	return r
}
