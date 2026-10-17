// Package kif 解析 CatSystem2 的 .int/.kif 归档（内部标签 "KIF"，kiri2/CS2 的私有归档格式）。
//
// 格式（源自 GARbro ArcFormats/CatSystem/ArcINT.cs，已用本机 8 款 CS2 游戏实测验证）：
//
//	0x00  "KIF\0"
//	0x04  u32le 条目数 count（含元数据槽）
//	0x08  "__key__.dat\0" → 加密布局；否则为明文布局（名字与数据都不加密，
//	      名字字段长度按 {0x20, 0x40} 两种候选试，对应 GARbro 的 NameSizes）
//	0x4c  u32le seed：MersenneTwister(seed) 的首个输出（小端 4 字节）即 Blowfish 密钥
//	0x48  起每条：0x40 字节混淆名字 + u32le(offset) + u32le(size)，步长 0x48；槽 0 不是文件
//
// 解密：offset/size 作为 8 字节一块用 Blowfish 直接解；条目数据用同一个 Blowfish 按
// ECB 解 8 字节块（尾部不足 8 字节的部分原样保留）。
//
// 名字：每条的名字密钥为 MT(SRand(main_key+i)).Rand()，但 DecipherName 只用到该密钥
// 四字节之和的低 8 位 k0，故**不持有 main_key 时每条名字也仅有 256 种可能**。本包用
// 「条目内容魔数 → 扩展名」作约束枚举 k0，再按大小写归一化取最大组定名：
// 字母完全可靠，**大小写不保证**（缺 main_key 时无法判定），故下游匹配须大小写无关。
// 持有口令时用 SetPassphrase/SetMainKey 走精确路径（名字与原始归档逐字节一致）。
package kif

import (
	"encoding/binary"
	"fmt"
	"os"
	"sort"
	"strings"
)

const (
	magic        = "KIF\x00"
	indexStart   = 8
	entryStride  = 0x48
	nameField    = 0x40
	seedOffset   = 0x4c
	headSniffLen = 32
)

const keyDatMarker = "__key__.dat\x00"

// plainNameSizes 明文 KIF 布局的名字字段长度候选（GARbro NameSizes）。
var plainNameSizes = []int{0x20, 0x40}

// 名字的可信度分档（**大小写不在其中**：缺 main_key 时大小写恒有双解，见包注释）。
const (
	NameExactKind     = "exact"     // 归一化后只有一个候选名字：字母完全可靠
	NameDominantKind  = "dominant"  // 多个候选分组但最大组占支配地位（孪生组 ~10 个 vs 杂音 1 个）
	NameAmbiguousKind = "ambiguous" // 分组势均力敌，采信最大组但有风险
	NameUnknownKind   = "unknown"   // 无法定名
)

// Entry 归档内一个条目。
type Entry struct {
	Name     string // 解出的名字（可能缺大小写精度，见 NameKind）
	Offset   int64  // 数据在归档中的偏移
	Size     int64  // 数据字节数
	NameKind string // Name*Kind 之一
}

// Archive 一个已打开的 KIF 归档。用完须 Close。
type Archive struct {
	Path    string
	Count   int // 头部记录的条目数（含元数据槽，故真实文件数 = Count-1）
	Seed    uint32
	Entries []Entry
	Skipped int // 越界条目数（数据范围超文件尾，已跳过）

	file  *os.File
	bf    *blowfish
	plain bool     // 明文布局：名字与数据都不加密
	idx   []int    // Entries[i] 的名字槽下标 i+1
	raw   [][]byte // Entries[i] 的原始（混淆）名字字节
}

// Open 打开 KIF 归档并解出全部条目名。
func Open(path string) (*Archive, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	a := &Archive{Path: path, file: f}
	if err := a.readIndex(); err != nil {
		f.Close()
		return nil, fmt.Errorf("KIF %s: %w", path, err)
	}
	return a, nil
}

// Close 关闭底层文件。
func (a *Archive) Close() error {
	if a.file != nil {
		return a.file.Close()
	}
	return nil
}

func (a *Archive) readIndex() error {
	st, err := a.file.Stat()
	if err != nil {
		return err
	}
	size := st.Size()

	var head [80]byte
	n, err := a.file.ReadAt(head[:], 0)
	if err != nil && n < 8 {
		return fmt.Errorf("读取头部失败: %w", err)
	}
	if string(head[:4]) != magic {
		return fmt.Errorf("魔数不符（%q）", head[:4])
	}
	count := int(binary.LittleEndian.Uint32(head[4:8]))
	a.Count = count
	if count <= 0 {
		return nil
	}
	if string(head[8:8+len(keyDatMarker)]) != keyDatMarker {
		return a.readPlainIndex(count, size)
	}
	if count <= 1 {
		return nil // 加密布局里 slot 0 只是元数据槽，故无真实条目
	}
	if int64(count)*entryStride+indexStart > size {
		return fmt.Errorf("条目数 %d 与文件大小 %d 不符（索引越界）", count, size)
	}
	a.Seed = binary.LittleEndian.Uint32(head[seedOffset:])

	var m mt19937
	m.seed(a.Seed)
	a.bf = newBlowfish(le4(m.next()))

	raw := make([]byte, entryStride)
	for i := 1; i < count; i++ {
		off := int64(indexStart) + int64(i)*entryStride
		if _, err := a.file.ReadAt(raw, off); err != nil {
			return fmt.Errorf("读取第 %d 条索引失败: %w", i, err)
		}
		namebuf := make([]byte, nameField)
		copy(namebuf, raw[:nameField])
		eo := int64(binary.LittleEndian.Uint32(raw[nameField:])) + int64(i)
		es := int64(binary.LittleEndian.Uint32(raw[nameField+4:]))
		xl, xr := uint32(eo), uint32(es)
		a.bf.decipherBlock(&xl, &xr)
		eo, es = int64(xl), int64(xr)

		if es < 0 || eo < 0 || eo+es > size {
			a.Skipped++
			continue
		}
		a.idx = append(a.idx, i)
		a.raw = append(a.raw, namebuf)
		a.Entries = append(a.Entries, Entry{Offset: eo, Size: es})
	}

	for i := range a.Entries {
		e := &a.Entries[i]
		exts := a.sniffExts(e)
		e.Name, e.NameKind = resolveName(a.raw[i], exts)
		if e.Name == "" {
			e.Name = fmt.Sprintf("unnamed_%05d.bin", a.idx[i])
			e.NameKind = NameUnknownKind
		}
	}
	return nil
}

// readPlainIndex 解析明文布局（@8 不是 __key__.dat）：名字明文 CP932、数据不加密。
// 名字字段长度按 plainNameSizes 逐个试，任一条校验失败就换下一个候选。
func (a *Archive) readPlainIndex(count int, size int64) error {
	var lastErr error
	for _, nameLen := range plainNameSizes {
		stride := int64(nameLen) + 8
		need := int64(indexStart) + int64(count)*stride
		if need > size {
			lastErr = fmt.Errorf("明文布局（名字字段 %d）：索引需要 %d 字节 > 文件大小 %d", nameLen, need, size)
			continue
		}
		buf := make([]byte, need)
		if _, err := a.file.ReadAt(buf, 0); err != nil {
			lastErr = err
			continue
		}
		ents := make([]Entry, 0, count)
		off := int64(indexStart)
		for i := 0; i < count; i++ {
			name := decodeName(trimNul(buf[off : off+int64(nameLen)]))
			if name == "" {
				lastErr = fmt.Errorf("明文布局（名字字段 %d）：第 %d 条名字为空", nameLen, i)
				break
			}
			eo := int64(binary.LittleEndian.Uint32(buf[off+int64(nameLen):]))
			es := int64(binary.LittleEndian.Uint32(buf[off+int64(nameLen)+4:]))
			if eo <= off+int64(nameLen) || eo+es > size {
				lastErr = fmt.Errorf("明文布局（名字字段 %d）：第 %d 条 %q 的 offset/size 非法", nameLen, i, name)
				break
			}
			ents = append(ents, Entry{Name: name, Offset: eo, Size: es, NameKind: NameExactKind})
			off += stride
		}
		if len(ents) == count {
			a.plain = true
			a.Entries = ents
			return nil
		}
		if lastErr == nil {
			lastErr = fmt.Errorf("明文布局（名字字段 %d）解析不完整", nameLen)
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("明文布局不成立（entry_count=%d）", count)
	}
	return lastErr
}

// trimNul 截到第一个 0 字节（不含）。
func trimNul(b []byte) []byte {
	for i, c := range b {
		if c == 0 {
			return b[:i]
		}
	}
	return b
}

// sniffExts 读条目数据头部，按魔数给出可能的扩展名（无匹配时返回 nil）。
func (a *Archive) sniffExts(e *Entry) []string {
	n := e.Size
	if n > headSniffLen {
		n = headSniffLen
	}
	if n < 8 {
		return nil
	}
	buf := make([]byte, n)
	if _, err := a.file.ReadAt(buf, e.Offset); err != nil {
		return nil
	}
	if !a.plain {
		a.bf.decipherBytes(buf)
	}
	return magicExts(buf)
}

// Read 读取并解密一个条目的内容（尾部不足 8 字节的部分原样返回）。
func (a *Archive) Read(e Entry) ([]byte, error) {
	if e.Size <= 0 {
		return nil, nil
	}
	buf := make([]byte, e.Size)
	if _, err := a.file.ReadAt(buf, e.Offset); err != nil {
		return nil, err
	}
	if !a.plain {
		a.bf.decipherBytes(buf)
	}
	return buf, nil
}

// SetMainKey 用已知的 main_key 精确解出全部名字（与原始归档逐字节一致，含大小写）。
func (a *Archive) SetMainKey(mainKey uint32) {
	var m mt19937
	for i := range a.Entries {
		m.seed(mainKey + uint32(a.idx[i]))
		k := m.next()
		name := decipherNameKey(a.raw[i], k)
		if name != "" {
			a.Entries[i].Name = name
			a.Entries[i].NameKind = NameExactKind
		}
	}
}

// SetPassphrase 用口令精确解名字（内部即 EncodePassPhrase → SetMainKey）。
// 口令按 CP932 编码参与 CRC；此处仅接受 ASCII 口令（本机 7 款游戏实测无一可用，
// 其 exe 只带数字资源 id，没有 GARbro 期望的 DATA/V_CODE2 资源）。
func (a *Archive) SetPassphrase(pass string) {
	a.SetMainKey(PassphraseToKey(pass))
}

// KindStats 统计各可信度档的条目数。
func (a *Archive) KindStats() map[string]int {
	out := map[string]int{}
	for _, e := range a.Entries {
		out[e.NameKind]++
	}
	return out
}

// decode 名字字节 → 字符串（ASCII 快路径，其余走 CP932）。
func decipherNameKey(namebuf []byte, key uint32) string {
	k := byte((key >> 24) + (key >> 16) + (key >> 8) + key)
	return decipherNameK(namebuf, k)
}

var nameAlphabet = []byte("zyxwvutsrqponmlkjihgfedcbaZYXWVUTSRQPONMLKJIHGFEDCBA")

// decipherNameK 按「四字节之和的低 8 位 = k」解一条名字（直到 NUL 为止）。
// 注意 k 逐字节递增且**不按 256 回绕**（与 GARbro 的 int k 一致：k0 较大时名字后半段
// 会用到 256 以上的 k，按 byte 回绕会与规范产生分歧）。
func decipherNameK(namebuf []byte, k byte) string {
	out := make([]byte, 0, nameField)
	ki := int(k)
	for i := 0; i < len(namebuf); i++ {
		c := namebuf[i]
		if c == 0 {
			break
		}
		if j := indexByte(nameAlphabet, c); j >= 0 {
			j -= ki % 0x34
			if j < 0 {
				j += 0x34
			}
			c = nameAlphabet[0x33-j]
		}
		out = append(out, c)
		ki++
	}
	return decodeName(out)
}

func indexByte(b []byte, c byte) int {
	for i, v := range b {
		if v == c {
			return i
		}
	}
	return -1
}

type cand struct {
	k0   int
	name string
}

// resolveName 枚举 256 个 k0 定名。先用内容魔数给的 exts 约束，失败再放宽到已知扩展名表。
func resolveName(namebuf []byte, exts []string) (string, string) {
	if name, kind, ok := resolvePass(namebuf, exts); ok {
		return name, kind
	}
	if name, kind, ok := resolvePass(namebuf, nil); ok {
		return name, kind
	}
	return "", NameUnknownKind
}

func resolvePass(namebuf []byte, exts []string) (string, string, bool) {
	groups := map[string][]cand{}
	var order []string
	for k0 := 0; k0 < 256; k0++ {
		n := decipherNameK(namebuf, byte(k0))
		if n == "" || len(n) > 63 {
			continue
		}
		stem, ext := splitExt(n)
		if !validToken(stem) || !validExt(ext) {
			continue
		}
		ext = "." + strings.ToLower(ext)
		if exts != nil {
			if !containsStr(exts, ext) {
				continue
			}
		} else if !knownExts[ext] {
			continue
		}
		lower := strings.ToLower(n)
		if _, ok := groups[lower]; !ok {
			order = append(order, lower)
		}
		groups[lower] = append(groups[lower], cand{k0, n})
	}
	if len(order) == 0 {
		return "", "", false
	}
	if len(order) == 1 {
		g := groups[order[0]]
		sort.Slice(g, func(i, j int) bool { return g[i].k0 < g[j].k0 })
		kind := NameDominantKind
		if exts != nil {
			kind = NameExactKind
		}
		return g[0].name, kind, true
	}
	// 多个大小写归组：最大组若明显更大（孪生组 5~10 个，真异名一般 1 个）→ 只差大小写。
	bestKey, bestLen, second := "", 0, 0
	for _, k := range order {
		n := len(groups[k])
		if n > bestLen {
			second, bestLen, bestKey = bestLen, n, k
		} else if n > second {
			second = n
		}
	}
	g := groups[bestKey]
	sort.Slice(g, func(i, j int) bool { return g[i].k0 < g[j].k0 })
	kind := NameAmbiguousKind
	if bestLen >= 3 && bestLen > second {
		kind = NameDominantKind
	}
	return g[0].name, kind, true
}

func splitExt(n string) (string, string) {
	i := strings.LastIndexByte(n, '.')
	if i <= 0 {
		return n, ""
	}
	return n[:i], n[i+1:]
}

func validToken(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-' {
			continue
		}
		return false
	}
	return true
}

func validExt(s string) bool {
	if s == "" || len(s) > 5 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' {
			continue
		}
		return false
	}
	return true
}

func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// magicExt 内容魔数 → 扩展名（实测语料所得，覆盖本机 8 款 CS2 游戏）。
var magicExt = []struct {
	magic string
	exts  []string
}{
	{"OggS", []string{".ogg"}},
	{"LPD\x00", []string{".lpb"}}, // CS2 语音双份存储：.lpb（可循环）+ .ogg
	{"RIFF", []string{".wav", ".avi"}},
	{"TLG5.0", []string{".tlg"}},
	{"TLG6.0", []string{".tlg"}},
	{"TLG0.0", []string{".tlg"}},
	{"\x89PNG", []string{".png"}},
	{"\xff\xd8\xff", []string{".jpg", ".jpeg"}},
	{"PSB\x00", []string{".psb"}},
	{"PSZ\x00", []string{".psz"}},
	{"HG-3", []string{".hg3"}},
	{"HG-2", []string{".hg2"}},
	{"PK\x03\x04", []string{".zip"}},
	{"KIF\x00", []string{".int", ".kif"}},
	{"CatS", []string{".cst"}}, // CatScene 剧本
	{"Compressed", []string{".cst"}},
	{"BM", []string{".bmp"}},
	{"GIF8", []string{".gif"}},
	{"ID3", []string{".mp3"}},
	{"fLaC", []string{".flac"}},
	{"MThd", []string{".mid"}},
}

func magicExts(head []byte) []string {
	for _, m := range magicExt {
		if strings.HasPrefix(string(head), m.magic) {
			return m.exts
		}
	}
	return nil
}

// knownExts 无魔数可用时的放宽白名单。
var knownExts = map[string]bool{
	".ogg": true, ".lpb": true, ".cst": true, ".psb": true, ".psz": true,
	".tlg": true, ".png": true, ".jpg": true, ".jpeg": true, ".wav": true,
	".avi": true, ".mpg": true, ".mpeg": true, ".mp4": true, ".wmv": true,
	".hg3": true, ".hg2": true, ".bmp": true, ".gif": true, ".mp3": true,
	".flac": true, ".mid": true, ".txt": true, ".csv": true, ".dat": true,
	".int": true, ".kif": true, ".zip": true, ".bin": true, ".ttf": true,
	".otf": true, ".ico": true, ".exe": true, ".dll": true, ".scn": true,
}

// PassphraseToKey 复刻 GARbro 的 KeyData.EncodePassPhrase（CRC32 变体）。
// 口令需 CP932 编码；此处按字节直取，ASCII 口令等价。
func PassphraseToKey(pass string) uint32 {
	key := uint32(0xffffffff)
	for _, c := range []byte(pass) {
		key = ^crc32Table[byte(key>>24)^c] ^ (key << 8)
	}
	return key
}

var crc32Table [256]uint32

func init() {
	for n := 0; n < 256; n++ {
		c := uint32(n) << 24
		for k := 0; k < 8; k++ {
			if c&0x80000000 != 0 {
				c = 0x04c11db7 ^ (c << 1)
			} else {
				c <<= 1
			}
		}
		crc32Table[n] = c
	}
}

func le4(v uint32) []byte {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], v)
	return b[:]
}

// mt19937 是 GARbro 的 Mersenne Twister 变体（SRand 用 69069 LCG 初始化，非标准 init_genrand）。
type mt19937 struct {
	mt  [624]uint32
	mti int
}

var mag01 = [2]uint32{0, 0x9908b0df}

func (m *mt19937) seed(seed uint32) {
	for i := 0; i < len(m.mt); i++ {
		upper := seed & 0xffff0000
		seed = 69069*seed + 1
		m.mt[i] = upper | ((seed & 0xffff0000) >> 16)
		seed = 69069*seed + 1
	}
	m.mti = len(m.mt) // 首次 Rand() 触发重新生成
}

func (m *mt19937) next() uint32 {
	if m.mti >= len(m.mt) {
		kk := 0
		for ; kk < 624-397; kk++ {
			y := (m.mt[kk] & 0x80000000) | (m.mt[kk+1] & 0x7fffffff)
			m.mt[kk] = m.mt[kk+397] ^ (y >> 1) ^ mag01[y&1]
		}
		for ; kk < 623; kk++ {
			y := (m.mt[kk] & 0x80000000) | (m.mt[kk+1] & 0x7fffffff)
			m.mt[kk] = m.mt[kk+397-624] ^ (y >> 1) ^ mag01[y&1]
		}
		y := (m.mt[623] & 0x80000000) | (m.mt[0] & 0x7fffffff)
		m.mt[623] = m.mt[396] ^ (y >> 1) ^ mag01[y&1]
		m.mti = 0
	}
	y := m.mt[m.mti]
	m.mti++
	y ^= y >> 11
	y ^= (y << 7) & 0x9d2c5680
	y ^= (y << 15) & 0xefc60000
	y ^= y >> 18
	return y
}

// blowfish 是 GARbro 的 Blowfish（密钥长度可变，18 个 P + 4×256 S 盒见 blowfish_tables.go）。
type blowfish struct {
	p [18]uint32
	s [4][256]uint32
}

func newBlowfish(key []byte) *blowfish {
	b := &blowfish{p: bfP, s: bfS}
	j := 0
	for i := 0; i < 18; i++ {
		var data uint32
		for k := 0; k < 4; k++ {
			data = (data << 8) | uint32(key[j])
			j++
			if j >= len(key) {
				j = 0
			}
		}
		b.p[i] ^= data
	}
	var dl, dr uint32
	for i := 0; i < 18; i += 2 {
		b.encipherBlock(&dl, &dr)
		b.p[i], b.p[i+1] = dl, dr
	}
	for i := 0; i < 4; i++ {
		for j := 0; j < 256; j += 2 {
			b.encipherBlock(&dl, &dr)
			b.s[i][j], b.s[i][j+1] = dl, dr
		}
	}
	return b
}

func (b *blowfish) f(x uint32) uint32 {
	d := x & 0xff
	c := (x >> 8) & 0xff
	bb := (x >> 16) & 0xff
	a := (x >> 24) & 0xff
	y := b.s[0][a] + b.s[1][bb]
	y ^= b.s[2][c]
	y += b.s[3][d]
	return y
}

func (b *blowfish) encipherBlock(xl, xr *uint32) {
	Xl, Xr := *xl, *xr
	for i := 0; i < 16; i++ {
		Xl ^= b.p[i]
		Xr = b.f(Xl) ^ Xr
		Xl, Xr = Xr, Xl
	}
	Xl, Xr = Xr, Xl
	Xr ^= b.p[16]
	Xl ^= b.p[17]
	*xl, *xr = Xl, Xr
}

func (b *blowfish) decipherBlock(xl, xr *uint32) {
	Xl, Xr := *xl, *xr
	for i := 17; i > 1; i-- {
		Xl ^= b.p[i]
		Xr = b.f(Xl) ^ Xr
		Xl, Xr = Xr, Xl
	}
	Xl, Xr = Xr, Xl
	Xr ^= b.p[1]
	Xl ^= b.p[0]
	*xl, *xr = Xl, Xr
}

// decipherBytes 按 GARbro Decipher(byte[]) 的**小端**组字逐 8 字节解密；尾部余数原样保留。
func (b *blowfish) decipherBytes(data []byte) {
	n := len(data) / 8 * 8
	for i := 0; i < n; i += 8 {
		xl := binary.LittleEndian.Uint32(data[i:])
		xr := binary.LittleEndian.Uint32(data[i+4:])
		b.decipherBlock(&xl, &xr)
		binary.LittleEndian.PutUint32(data[i:], xl)
		binary.LittleEndian.PutUint32(data[i+4:], xr)
	}
}
