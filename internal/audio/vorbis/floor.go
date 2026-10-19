package vorbis

// floorDecode 解码一帧的 floor，返回长度为 n/2 的 floor 曲线。
// ok=false 表示该声道本帧未使用（no_residue）。
func (s *setup) floorDecode(br *bitReader, f floor, n int) ([]float32, bool) {
	switch f.kind {
	case 1:
		return s.floor1Decode(br, f.f1, n)
	default:
		// floor 0 本作素材未使用，暂不支持。
		return nil, false
	}
}

// floor1Decode 解码 floor 类型 1 并计算曲线（§7.2.3 / §7.2.4）。
func (s *setup) floor1Decode(br *bitReader, f *floor1, n int) ([]float32, bool) {
	if br.read(1) == 0 {
		// 该声道本帧未使用。
		return nil, false
	}

	rangeTab := [4]int{256, 128, 86, 64}
	rangeVal := rangeTab[f.multiplier-1]
	bits := ilog(uint32(rangeVal - 1))

	values := len(f.xList)
	y := make([]int, values)
	y[0] = int(br.read(bits))
	y[1] = int(br.read(bits))

	offset := 2
	for i := 0; i < f.partitions; i++ {
		c := f.partitionClass[i]
		cdim := f.classDimensions[c]
		cbits := f.classSubclasses[c]
		csub := (1 << uint(cbits)) - 1
		cval := 0
		if cbits > 0 {
			cval = s.codebooks[f.classMasterbooks[c]].decodeScalar(br)
		}
		for j := 0; j < cdim; j++ {
			book := f.subclassBooks[c][cval&csub]
			cval >>= uint(cbits)
			if book >= 0 && book < len(s.codebooks) {
				y[offset+j] = s.codebooks[book].decodeScalar(br)
			} else {
				y[offset+j] = 0
			}
		}
		offset += cdim
	}

	if br.overrun {
		return nil, false
	}

	// 第一步：由相邻值预测，把 Y 展开为 finalY。
	finalY := make([]int, values)
	step2 := make([]bool, values)
	finalY[0] = y[0]
	finalY[1] = y[1]
	step2[0] = true
	step2[1] = true

	for i := 2; i < values; i++ {
		low := lowNeighbor(f.xList, i)
		high := highNeighbor(f.xList, i)
		predicted := renderPoint(f.xList[low], finalY[low], f.xList[high], finalY[high], f.xList[i])

		val := y[i]
		highroom := rangeVal - predicted
		lowroom := predicted
		room := 2 * minInt(highroom, lowroom)

		if val != 0 {
			step2[low] = true
			step2[high] = true
			step2[i] = true
			if val >= room {
				if highroom > lowroom {
					finalY[i] = val - lowroom + predicted
				} else {
					finalY[i] = predicted - val + highroom - 1
				}
			} else if val&1 != 0 {
				finalY[i] = predicted - ((val + 1) / 2)
			} else {
				finalY[i] = predicted + (val / 2)
			}
		} else {
			step2[i] = false
			finalY[i] = predicted
		}
	}
	for i := range finalY {
		if finalY[i] < 0 {
			finalY[i] = 0
		}
		if finalY[i] >= rangeVal {
			finalY[i] = rangeVal - 1
		}
	}

	// 第二步：按 X 升序渲染线段。
	order := make([]int, values)
	for i := range order {
		order[i] = i
	}
	// 按 X 值做插入排序（values <= 65）。
	for i := 1; i < len(order); i++ {
		for j := i; j > 0 && f.xList[order[j]] < f.xList[order[j-1]]; j-- {
			order[j], order[j-1] = order[j-1], order[j]
		}
	}

	half := n / 2
	curve := make([]int, half)

	hx := 0
	lx := 0
	ly := finalY[order[0]] * f.multiplier
	hy := 0
	for i := 1; i < values; i++ {
		idx := order[i]
		if step2[idx] {
			hy = finalY[idx] * f.multiplier
			hx = f.xList[idx]
			renderLine(lx, ly, hx, hy, curve)
			lx = hx
			ly = hy
		}
	}
	if hx < half {
		renderLine(hx, hy, half, hy, curve)
	} else if hx > half {
		// 超出部分截断。
		for i := half; i < hx && i < len(curve); i++ {
			_ = i
		}
	}

	out := make([]float32, half)
	for i := 0; i < half; i++ {
		v := curve[i]
		if v < 0 {
			v = 0
		}
		if v > 255 {
			v = 255
		}
		out[i] = floor1InverseDB[v]
	}
	return out, true
}

// lowNeighbor 返回 n<x 中满足 v[n]<v[x] 且 v[n] 最大的下标（§9.2.4）。
func lowNeighbor(v []int, x int) int {
	n := -1
	for i := 0; i < x; i++ {
		if v[i] < v[x] {
			if n < 0 || v[i] > v[n] {
				n = i
			}
		}
	}
	if n < 0 {
		return 0
	}
	return n
}

// highNeighbor 返回 n<x 中满足 v[n]>v[x] 且 v[n] 最小的下标（§9.2.5）。
func highNeighbor(v []int, x int) int {
	n := -1
	for i := 0; i < x; i++ {
		if v[i] > v[x] {
			if n < 0 || v[i] < v[n] {
				n = i
			}
		}
	}
	if n < 0 {
		return 1
	}
	return n
}

// renderPoint 在两点之间做线性插值（§9.2.6），返回 X 处的整数 Y。
func renderPoint(x0, y0, x1, y1, x int) int {
	dy := y1 - y0
	adx := x1 - x0
	if adx == 0 {
		return y0
	}
	ady := dy
	if ady < 0 {
		ady = -ady
	}
	err := ady * (x - x0)
	off := err / adx
	if dy < 0 {
		return y0 - off
	}
	return y0 + off
}

// renderLine 在 [x0,x1) 上按 Bresenham 风格整数插值填充 v（§9.2.7）。
func renderLine(x0, y0, x1, y1 int, v []int) {
	dy := y1 - y0
	adx := x1 - x0
	if adx == 0 {
		return
	}
	ady := dy
	if ady < 0 {
		ady = -ady
	}
	base := dy / adx
	x := x0
	y := y0
	err := 0

	sy := base + 1
	if dy < 0 {
		sy = base - 1
	}
	absBase := base
	if absBase < 0 {
		absBase = -absBase
	}
	ady -= absBase * adx

	if x >= 0 && x < len(v) {
		v[x] = y
	}
	for x = x0 + 1; x < x1; x++ {
		err += ady
		if err >= adx {
			err -= adx
			y += sy
		} else {
			y += base
		}
		if x >= 0 && x < len(v) {
			v[x] = y
		}
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
