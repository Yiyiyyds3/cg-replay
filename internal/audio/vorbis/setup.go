package vorbis

import "fmt"

// identification 是 Vorbis 的识别头（§4.2.2）。
type identification struct {
	Channels   int
	SampleRate int
	Blocksize0 int
	Blocksize1 int
}

// floor0 配置（§6）。
type floor0 struct {
	order           int
	rate            int
	barkMapSize     int
	amplitudeBits   int
	amplitudeOffset int
	bookList        []int
}

// floor1 配置（§7.2.2）。
type floor1 struct {
	partitions       int
	partitionClass   []int
	classDimensions  []int
	classSubclasses  []int
	classMasterbooks []int
	subclassBooks    [][]int
	multiplier       int
	rangebits        int
	xList            []int
}

// floor 是一个 floor 定义，type 为 0 或 1。
type floor struct {
	kind int
	f0   *floor0
	f1   *floor1
}

// residue 配置（§8.2）。三种 type 的配置格式相同。
type residue struct {
	kind            int
	begin           int
	end             int
	partitionSize   int
	classifications int
	classbook       int
	cascade         []int
	books           [][]int // [classification][pass]，-1 表示该 pass 无码本
}

// mapping 配置（§4.2.4）。
type mapping struct {
	submaps       int
	couplingSteps int
	magnitude     []int
	angle         []int
	mux           []int
	submapFloor   []int
	submapResidue []int
}

// mode 配置。
type mode struct {
	blockflag     bool
	windowtype    int
	transformtype int
	mapping       int
}

// setup 是完整的编解码配置。
type setup struct {
	id        identification
	codebooks []*codebook
	floors    []floor
	residues  []residue
	mappings  []mapping
	modes     []mode
}

// parseIdentification 解析识别头（不含前导 type 字节与 "vorbis" 串）。
func parseIdentification(br *bitReader) (identification, error) {
	var id identification
	version := br.read(32)
	if version != 0 {
		return id, fmt.Errorf("vorbis: 不支持的版本 %d", version)
	}
	id.Channels = int(br.read(8))
	id.SampleRate = int(br.read(32))
	if id.Channels <= 0 || id.SampleRate <= 0 {
		return id, fmt.Errorf("vorbis: 非法声道数/采样率 %d/%d", id.Channels, id.SampleRate)
	}
	// 三个码率提示值，只做提示。
	br.read(32)
	br.read(32)
	br.read(32)
	bs0 := int(br.read(4))
	bs1 := int(br.read(4))
	if bs0 < 6 || bs1 < 6 || bs0 > 13 || bs1 > 13 || bs0 > bs1 {
		return id, fmt.Errorf("vorbis: 非法块长指数 %d/%d", bs0, bs1)
	}
	id.Blocksize0 = 1 << uint(bs0)
	id.Blocksize1 = 1 << uint(bs1)
	if br.read(1) != 1 {
		return id, fmt.Errorf("vorbis: 识别头 framing bit 未置位")
	}
	return id, nil
}

// parseSetup 解析 setup 头（不含前导 type 字节与 "vorbis" 串）。
func parseSetup(br *bitReader, channels int) (*setup, error) {
	s := &setup{}

	// ① codebooks
	codebookCount := int(br.read(8)) + 1
	s.codebooks = make([]*codebook, 0, codebookCount)
	for i := 0; i < codebookCount; i++ {
		cb, err := readCodebook(br)
		if err != nil {
			return nil, fmt.Errorf("码本 #%d：%w", i, err)
		}
		s.codebooks = append(s.codebooks, cb)
	}

	// ② time domain transforms（占位，必须为 0）
	timeCount := int(br.read(6)) + 1
	for i := 0; i < timeCount; i++ {
		if br.read(16) != 0 {
			return nil, fmt.Errorf("vorbis: time domain transform #%d 非 0", i)
		}
	}

	// ③ floors
	floorCount := int(br.read(6)) + 1
	s.floors = make([]floor, 0, floorCount)
	for i := 0; i < floorCount; i++ {
		kind := int(br.read(16))
		switch kind {
		case 0:
			f, err := parseFloor0(br)
			if err != nil {
				return nil, fmt.Errorf("floor #%d：%w", i, err)
			}
			s.floors = append(s.floors, floor{kind: 0, f0: f})
		case 1:
			f, err := parseFloor1(br)
			if err != nil {
				return nil, fmt.Errorf("floor #%d：%w", i, err)
			}
			s.floors = append(s.floors, floor{kind: 1, f1: f})
		default:
			return nil, fmt.Errorf("vorbis: 未知 floor 类型 %d", kind)
		}
	}

	// ④ residues
	residueCount := int(br.read(6)) + 1
	s.residues = make([]residue, 0, residueCount)
	for i := 0; i < residueCount; i++ {
		r, err := parseResidue(br)
		if err != nil {
			return nil, fmt.Errorf("residue #%d：%w", i, err)
		}
		s.residues = append(s.residues, r)
	}

	// ⑤ mappings
	mappingCount := int(br.read(6)) + 1
	s.mappings = make([]mapping, 0, mappingCount)
	for i := 0; i < mappingCount; i++ {
		m, err := parseMapping(br, channels, len(s.floors), len(s.residues))
		if err != nil {
			return nil, fmt.Errorf("mapping #%d：%w", i, err)
		}
		s.mappings = append(s.mappings, m)
	}

	// ⑥ modes
	modeCount := int(br.read(6)) + 1
	s.modes = make([]mode, 0, modeCount)
	for i := 0; i < modeCount; i++ {
		var m mode
		m.blockflag = br.read(1) != 0
		m.windowtype = int(br.read(16))
		m.transformtype = int(br.read(16))
		m.mapping = int(br.read(8))
		if m.windowtype != 0 {
			return nil, fmt.Errorf("vorbis: mode #%d window type 非 0", i)
		}
		if m.transformtype != 0 {
			return nil, fmt.Errorf("vorbis: mode #%d transform type 非 0", i)
		}
		if m.mapping >= len(s.mappings) {
			return nil, fmt.Errorf("vorbis: mode #%d 的 mapping 越界", i)
		}
		s.modes = append(s.modes, m)
	}

	// ⑦ framing bit
	if br.read(1) != 1 {
		return nil, fmt.Errorf("vorbis: setup 头 framing bit 未置位")
	}
	return s, nil
}

// parseFloor0 解析 floor 类型 0（§6.2）。
func parseFloor0(br *bitReader) (*floor0, error) {
	f := &floor0{}
	f.order = int(br.read(8))
	f.rate = int(br.read(16))
	f.barkMapSize = int(br.read(16))
	f.amplitudeBits = int(br.read(6))
	f.amplitudeOffset = int(br.read(8))
	n := int(br.read(4)) + 1
	f.bookList = make([]int, n)
	for i := 0; i < n; i++ {
		f.bookList[i] = int(br.read(8))
	}
	return f, nil
}

// parseFloor1 解析 floor 类型 1（§7.2.2）。
func parseFloor1(br *bitReader) (*floor1, error) {
	f := &floor1{}
	f.partitions = int(br.read(5))
	f.partitionClass = make([]int, f.partitions)
	maxClass := -1
	for i := 0; i < f.partitions; i++ {
		c := int(br.read(4))
		f.partitionClass[i] = c
		if c > maxClass {
			maxClass = c
		}
	}
	f.classDimensions = make([]int, maxClass+1)
	f.classSubclasses = make([]int, maxClass+1)
	f.classMasterbooks = make([]int, maxClass+1)
	f.subclassBooks = make([][]int, maxClass+1)
	for i := 0; i <= maxClass; i++ {
		f.classDimensions[i] = int(br.read(3)) + 1
		sub := int(br.read(2))
		f.classSubclasses[i] = sub
		if sub > 0 {
			f.classMasterbooks[i] = int(br.read(8))
		} else {
			f.classMasterbooks[i] = -1
		}
		n := 1 << uint(sub)
		f.subclassBooks[i] = make([]int, n)
		for j := 0; j < n; j++ {
			f.subclassBooks[i][j] = int(br.read(8)) - 1
		}
	}

	f.multiplier = int(br.read(2)) + 1
	f.rangebits = int(br.read(4))

	// X 列表：0 与 2^rangebits 固定，然后按各 partition 的 class 维度展开。
	f.xList = []int{0, 1 << uint(f.rangebits)}
	for i := 0; i < f.partitions; i++ {
		c := f.partitionClass[i]
		for j := 0; j < f.classDimensions[c]; j++ {
			f.xList = append(f.xList, int(br.read(f.rangebits)))
		}
	}
	if len(f.xList) > 65 {
		return nil, fmt.Errorf("vorbis: floor1 X 列表过长（%d）", len(f.xList))
	}
	return f, nil
}

// parseResidue 解析 residue 配置（§8.2）。
func parseResidue(br *bitReader) (residue, error) {
	var r residue
	r.kind = int(br.read(16))
	if r.kind > 2 {
		return r, fmt.Errorf("vorbis: 未知 residue 类型 %d", r.kind)
	}
	r.begin = int(br.read(24))
	r.end = int(br.read(24))
	r.partitionSize = int(br.read(24)) + 1
	r.classifications = int(br.read(6)) + 1
	r.classbook = int(br.read(8))
	if r.classbook < 0 {
		return r, fmt.Errorf("vorbis: residue classbook 非法")
	}
	r.cascade = make([]int, r.classifications)
	for i := 0; i < r.classifications; i++ {
		high := 0
		low := int(br.read(3))
		if br.read(1) != 0 {
			high = int(br.read(5))
		}
		r.cascade[i] = high*8 + low
	}
	r.books = make([][]int, r.classifications)
	for i := 0; i < r.classifications; i++ {
		r.books[i] = make([]int, 8)
		for j := 0; j < 8; j++ {
			if r.cascade[i]&(1<<uint(j)) != 0 {
				r.books[i][j] = int(br.read(8))
			} else {
				r.books[i][j] = -1
			}
		}
	}
	return r, nil
}

// parseMapping 解析 mapping（§4.2.4 第 5 项）。
func parseMapping(br *bitReader, channels, floors, residues int) (mapping, error) {
	var m mapping
	kind := int(br.read(16))
	if kind != 0 {
		return m, fmt.Errorf("vorbis: 未知 mapping 类型 %d", kind)
	}
	if br.read(1) != 0 {
		m.submaps = int(br.read(4)) + 1
	} else {
		m.submaps = 1
	}
	if br.read(1) != 0 {
		m.couplingSteps = int(br.read(8)) + 1
		m.magnitude = make([]int, m.couplingSteps)
		m.angle = make([]int, m.couplingSteps)
		bits := ilog(uint32(channels - 1))
		for i := 0; i < m.couplingSteps; i++ {
			m.magnitude[i] = int(br.read(bits))
			m.angle[i] = int(br.read(bits))
			if m.magnitude[i] == m.angle[i] || m.magnitude[i] >= channels || m.angle[i] >= channels {
				return m, fmt.Errorf("vorbis: 非法声道耦合步")
			}
		}
	}
	if br.read(2) != 0 {
		return m, fmt.Errorf("vorbis: mapping reserved 位非 0")
	}
	if m.submaps > 1 {
		m.mux = make([]int, channels)
		for i := 0; i < channels; i++ {
			m.mux[i] = int(br.read(4))
			if m.mux[i] >= m.submaps {
				return m, fmt.Errorf("vorbis: mapping mux 越界")
			}
		}
	} else {
		m.mux = make([]int, channels)
	}
	m.submapFloor = make([]int, m.submaps)
	m.submapResidue = make([]int, m.submaps)
	for i := 0; i < m.submaps; i++ {
		br.read(8) // 未使用
		m.submapFloor[i] = int(br.read(8))
		m.submapResidue[i] = int(br.read(8))
		if m.submapFloor[i] >= floors {
			return m, fmt.Errorf("vorbis: submap floor 越界")
		}
		if m.submapResidue[i] >= residues {
			return m, fmt.Errorf("vorbis: submap residue 越界")
		}
	}
	return m, nil
}
