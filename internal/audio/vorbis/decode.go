package vorbis

import (
	"fmt"
	"os"
)

// Stream 是解码后的音频。
type Stream struct {
	SampleRate int
	Channels   int
	Samples    []float32 // 交错
}

// Frames 返回每声道帧数。
func (s *Stream) Frames() int {
	if s.Channels <= 0 {
		return 0
	}
	return len(s.Samples) / s.Channels
}

// DurationMs 返回时长（毫秒）。
func (s *Stream) DurationMs() int64 {
	if s.SampleRate <= 0 {
		return 0
	}
	return int64(s.Frames()) * 1000 / int64(s.SampleRate)
}

// DecodeFile 解码一个 Ogg Vorbis 文件。
func DecodeFile(path string) (*Stream, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Decode(data)
}

// Decode 解码 Ogg Vorbis 字节流。
func Decode(data []byte) (*Stream, error) {
	pages, err := parsePages(data)
	if err != nil {
		return nil, err
	}
	serial, ok := selectStream(pages)
	if !ok {
		return nil, fmt.Errorf("vorbis：未找到识别头")
	}
	pkts := packets(pages, serial)
	if len(pkts) < 3 {
		return nil, fmt.Errorf("vorbis：包数不足（%d）", len(pkts))
	}

	br := newBitReader(pkts[0].Data)
	if br.read(8) != 1 {
		return nil, fmt.Errorf("vorbis：识别头类型错误")
	}
	if err := expectVorbis(br); err != nil {
		return nil, err
	}
	id, err := parseIdentification(br)
	if err != nil {
		return nil, err
	}

	// 第二包：注释头，跳过内容。
	br = newBitReader(pkts[1].Data)
	if br.read(8) != 3 {
		return nil, fmt.Errorf("vorbis：注释头类型错误")
	}

	// 第三包：setup 头。
	br = newBitReader(pkts[2].Data)
	if br.read(8) != 5 {
		return nil, fmt.Errorf("vorbis：setup 头类型错误")
	}
	if err := expectVorbis(br); err != nil {
		return nil, err
	}
	st, err := parseSetup(br, id.Channels)
	if err != nil {
		return nil, err
	}

	d := &decoder{id: id, s: st}
	out := make([][]float32, id.Channels)
	for _, p := range pkts[3:] {
		out = d.decodePacket(p.Data, out)
	}

	n := 0
	if len(out) > 0 {
		n = len(out[0])
	}
	samples := make([]float32, n*id.Channels)
	for i := 0; i < n; i++ {
		for c := 0; c < id.Channels; c++ {
			if c < len(out) && i < len(out[c]) {
				samples[i*id.Channels+c] = out[c][i]
			}
		}
	}

	// 用最后一个 granule 位置裁剪尾部（Ogg 用它记录精确总样本数）。
	if last := lastGranule(pkts); last > 0 && int(last) < n {
		samples = samples[:int(last)*id.Channels]
	}

	return &Stream{SampleRate: id.SampleRate, Channels: id.Channels, Samples: samples}, nil
}

func expectVorbis(br *bitReader) error {
	var b [6]byte
	for i := 0; i < 6; i++ {
		b[i] = byte(br.read(8))
	}
	if string(b[:]) != "vorbis" {
		return fmt.Errorf("vorbis：缺少 vorbis 标记")
	}
	return nil
}

func lastGranule(pkts []oggPacket) int64 {
	for i := len(pkts) - 1; i >= 0; i-- {
		if pkts[i].Granule > 0 {
			return pkts[i].Granule
		}
	}
	return 0
}

type decoder struct {
	id identification
	s  *setup

	prevCh [][]float64
	prevN  int
}

// decodePacket 解码一个音频包，返回追加后的各声道样本。
func (d *decoder) decodePacket(data []byte, out [][]float32) [][]float32 {
	br := newBitReader(data)
	if br.read(1) != 0 {
		return out // 非音频包，忽略
	}
	modeBits := ilog(uint32(len(d.s.modes) - 1))
	modeNum := int(br.read(modeBits))
	if modeNum < 0 || modeNum >= len(d.s.modes) {
		return out
	}
	mode := d.s.modes[modeNum]

	n := d.id.Blocksize0
	long := mode.blockflag
	if long {
		n = d.id.Blocksize1
	}

	prevFlag, nextFlag := true, true
	if long {
		prevFlag = br.read(1) != 0
		nextFlag = br.read(1) != 0
	}

	w := window(n, d.id.Blocksize0, long, prevFlag, nextFlag)

	mapping := d.s.mappings[mode.mapping]
	ch := d.id.Channels
	half := n / 2

	// floor 逐声道解码
	noResidue := make([]bool, ch)
	curves := make([][]float32, ch)
	for i := 0; i < ch; i++ {
		sm := 0
		if i < len(mapping.mux) {
			sm = mapping.mux[i]
		}
		fi := 0
		if sm >= 0 && sm < len(mapping.submapFloor) {
			fi = mapping.submapFloor[sm]
		}
		if fi >= 0 && fi < len(d.s.floors) {
			c, ok := d.s.floorDecode(br, d.s.floors[fi], n)
			curves[i] = c
			noResidue[i] = !ok
		} else {
			noResidue[i] = true
		}
	}

	if br.overrun {
		// 包截断：本帧按静音处理。
		curves = make([][]float32, ch)
		for i := range curves {
			curves[i] = make([]float32, half)
		}
		noResidue = make([]bool, ch)
		for i := range noResidue {
			noResidue[i] = true
		}
	}

	// 非零向量传播：耦合两端只要有一端有 residue，就都保留。
	for k := 0; k < len(mapping.magnitude) && k < len(mapping.angle); k++ {
		m, a := mapping.magnitude[k], mapping.angle[k]
		if m < 0 || a < 0 || m >= ch || a >= ch {
			continue
		}
		if !noResidue[m] || !noResidue[a] {
			noResidue[m] = false
			noResidue[a] = false
		}
	}

	// residue 逐 submap
	spectra := make([][]float32, ch)
	for i := range spectra {
		spectra[i] = make([]float32, half)
	}
	for sm := 0; sm < len(mapping.submapResidue); sm++ {
		var chans []int
		for i := 0; i < ch; i++ {
			m := 0
			if i < len(mapping.mux) {
				m = mapping.mux[i]
			}
			if m == sm {
				chans = append(chans, i)
			}
		}
		if len(chans) == 0 {
			continue
		}
		ri := mapping.submapResidue[sm]
		if ri < 0 || ri >= len(d.s.residues) {
			continue
		}
		v := make([][]float32, len(chans))
		dnd := make([]bool, len(chans))
		for k, ci := range chans {
			v[k] = spectra[ci]
			dnd[k] = noResidue[ci]
		}
		d.s.residueDecode(br, d.s.residues[ri], v, dnd, n)
	}

	// 反耦合（逆序）
	for k := len(mapping.magnitude) - 1; k >= 0; k-- {
		if k >= len(mapping.angle) {
			continue
		}
		mi, ai := mapping.magnitude[k], mapping.angle[k]
		if mi < 0 || ai < 0 || mi >= ch || ai >= ch {
			continue
		}
		mv := spectra[mi]
		av := spectra[ai]
		for i := 0; i < half; i++ {
			M, A := mv[i], av[i]
			var newM, newA float32
			switch {
			case M > 0 && A > 0:
				newM, newA = M, M-A
			case M > 0:
				newA, newM = M, M+A
			case A > 0:
				newM, newA = M, M+A
			default:
				newA, newM = M, M-A
			}
			mv[i], av[i] = newM, newA
		}
	}

	// 点积 + IMDCT + 加窗
	cur := make([][]float64, ch)
	for i := 0; i < ch; i++ {
		spec := make([]float64, half)
		curve := curves[i]
		for j := 0; j < half; j++ {
			var cv float64
			if j < len(curve) {
				cv = float64(curve[j])
			}
			spec[j] = cv * float64(spectra[i][j])
		}
		time := imdct(spec, n)
		for j := 0; j < n; j++ {
			time[j] *= w[j]
		}
		cur[i] = time
	}

	// 首帧只用于 priming，不输出。
	if d.prevCh == nil {
		d.prevCh = cur
		d.prevN = n
		return out
	}

	// 返回「上一窗中心 → 当前窗中心」这一段，长度 = 上一块长/4 + 当前块长/4。
	// 当前窗从它的起点（下标 0）参与；上一窗从 3*prevN/4 - n/4 参与，
	// 因为两窗中心相距 prevN/4 + n/4，而上一窗终点在 T_{i-1}+prevN/2。
	length := d.prevN/4 + n/4
	curStart := n/4 - d.prevN/4
	for c := 0; c < ch; c++ {
		if c >= len(out) {
			out = append(out, nil)
		}
		buf := make([]float32, length)
		for j := 0; j < length; j++ {
			var v float64
			p := d.prevN/2 + j
			if c < len(d.prevCh) && p >= 0 && p < len(d.prevCh[c]) {
				v += d.prevCh[c][p]
			}
			ci := curStart + j
			if c < len(cur) && ci >= 0 && ci < len(cur[c]) {
				v += cur[c][ci]
			}
			buf[j] = float32(v)
		}
		out[c] = append(out[c], buf...)
	}

	d.prevCh = cur
	d.prevN = n
	return out
}
