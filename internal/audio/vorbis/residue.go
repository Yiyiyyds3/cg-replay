package vorbis

// residueDecode 解码一个 submap 的 residue，结果累加进 v（每个声道长度 n/2）。
// doNotDecode[j] 为 true 表示该声道本帧不参与。
func (s *setup) residueDecode(br *bitReader, r residue, v [][]float32, doNotDecode []bool, n int) {
	if r.kind == 2 {
		s.residueDecodeInterleaved(br, r, v, doNotDecode, n)
		return
	}

	actualSize := n / 2
	limitBegin := minInt(r.begin, actualSize)
	limitEnd := minInt(r.end, actualSize)
	nToRead := limitEnd - limitBegin
	if nToRead <= 0 {
		return
	}
	partitionsToRead := nToRead / r.partitionSize
	if partitionsToRead <= 0 {
		return
	}

	classbook := s.codebooks[r.classbook]
	classwords := classbook.dimensions
	classifications := make([][]int, len(v))
	for i := range classifications {
		classifications[i] = make([]int, partitionsToRead)
	}

	for pass := 0; pass < 8; pass++ {
		partitionCount := 0
		for partitionCount < partitionsToRead {
			if pass == 0 {
				for j := range v {
					if doNotDecode[j] {
						continue
					}
					temp := classbook.decodeScalar(br)
					for i := classwords - 1; i >= 0; i-- {
						idx := i + partitionCount
						if idx < partitionsToRead {
							classifications[j][idx] = temp % r.classifications
						}
						temp /= r.classifications
					}
				}
			}
			for i := 0; i < classwords && partitionCount < partitionsToRead; i++ {
				for j := range v {
					if doNotDecode[j] {
						continue
					}
					vqClass := classifications[j][partitionCount]
					if vqClass < 0 || vqClass >= len(r.books) {
						continue
					}
					bookIdx := r.books[vqClass][pass]
					if bookIdx < 0 || bookIdx >= len(s.codebooks) {
						continue
					}
					offset := limitBegin + partitionCount*r.partitionSize
					if r.kind == 0 {
						decodePartition0(br, s.codebooks[bookIdx], v[j], offset, r.partitionSize)
					} else {
						decodePartition1(br, s.codebooks[bookIdx], v[j], offset, r.partitionSize)
					}
				}
				partitionCount++
			}
		}
	}
}

// residueDecodeInterleaved 处理 format 2：所有声道共用一条交织向量（§8.6.3）。
func (s *setup) residueDecodeInterleaved(br *bitReader, r residue, v [][]float32, doNotDecode []bool, n int) {
	ch := len(v)
	if ch == 0 {
		return
	}
	// 若所有声道都 do-not-decode，则整个 residue 不解码。
	any := false
	for _, d := range doNotDecode {
		if !d {
			any = true
			break
		}
	}
	if !any {
		return
	}

	actualSize := (n / 2) * ch
	limitBegin := minInt(r.begin, actualSize)
	limitEnd := minInt(r.end, actualSize)
	nToRead := limitEnd - limitBegin
	if nToRead <= 0 {
		return
	}
	partitionsToRead := nToRead / r.partitionSize
	if partitionsToRead <= 0 {
		return
	}

	classbook := s.codebooks[r.classbook]
	classwords := classbook.dimensions
	classifications := make([]int, partitionsToRead)

	// 交织缓冲区，解码完再反交织回各声道。
	inter := make([]float32, n/2*ch)

	for pass := 0; pass < 8; pass++ {
		partitionCount := 0
		for partitionCount < partitionsToRead {
			if pass == 0 {
				temp := classbook.decodeScalar(br)
				for i := classwords - 1; i >= 0; i-- {
					idx := i + partitionCount
					if idx < partitionsToRead {
						classifications[idx] = temp % r.classifications
					}
					temp /= r.classifications
				}
			}
			for i := 0; i < classwords && partitionCount < partitionsToRead; i++ {
				vqClass := classifications[partitionCount]
				if vqClass < 0 || vqClass >= len(r.books) {
					partitionCount++
					continue
				}
				bookIdx := r.books[vqClass][pass]
				if bookIdx < 0 || bookIdx >= len(s.codebooks) {
					partitionCount++
					continue
				}
				offset := limitBegin + partitionCount*r.partitionSize
				decodePartition1(br, s.codebooks[bookIdx], inter, offset, r.partitionSize)
				partitionCount++
			}
		}
	}

	// 反交织：inter[i*ch+j] → v[j][i]
	for i := 0; i < n/2; i++ {
		for j := 0; j < ch; j++ {
			idx := i*ch + j
			if idx < len(inter) && i < len(v[j]) {
				v[j][i] += inter[idx]
			}
		}
	}
}

// decodePartition0 按 residue format 0 解码一个 partition（§8.6.4）。
func decodePartition0(br *bitReader, book *codebook, v []float32, offset, partitionSize int) {
	dim := book.dimensions
	if dim <= 0 {
		return
	}
	step := partitionSize / dim
	vec := make([]float32, dim)
	for i := 0; i < step; i++ {
		book.decodeVQ(br, vec)
		for j := 0; j < dim; j++ {
			idx := offset + i + j*step
			if idx >= 0 && idx < len(v) {
				v[idx] += vec[j]
			}
		}
	}
}

// decodePartition1 按 residue format 1 解码一个 partition（§8.6.5）。
func decodePartition1(br *bitReader, book *codebook, v []float32, offset, partitionSize int) {
	dim := book.dimensions
	if dim <= 0 {
		return
	}
	vec := make([]float32, dim)
	i := 0
	for i < partitionSize {
		book.decodeVQ(br, vec)
		for j := 0; j < dim && i < partitionSize; j++ {
			idx := offset + i
			if idx >= 0 && idx < len(v) {
				v[idx] += vec[j]
			}
			i++
		}
	}
}
