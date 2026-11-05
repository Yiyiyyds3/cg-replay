// npa.go —— Nitro+ NPA 归档容器解析。
//
// 头 41 字节：'NPA\x01' + 3 填充 + i32 key1(7) + i32 key2(11) + u8 compressed(15)
// + u8 encrypted(16) + i32 total(17) + i32 folder(21) + i32 files(25) + 8 保留
// + u32 dirSize(37)。目录区自 41 起，条目步长 4+nameLen+17。
//
// 条目：i32 nameLen + 加密名 + u8 type(1=目录/2=文件) + i32 folderId + u32 off
// + u32 size + u32 unpacked。名字解密 raw[x] += FC*x - arcKey 各字节 - index 各字节。
// 数据真实偏移 = dirSize + off + 41。
//
// 实测自鬼哭街重制版（2019，SDActi 引擎）：nss.npa/nss-1.npa（zlib 脚本）、
// cg.npa（2842 张原样 JPG/PNG，仅前 0x1000+名长 加密）、sound.npa（OGG）。
package nitro

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"ucgr/internal/textcodec"
)

// Entry NPA 条目。
type Entry struct {
	Name      string // 解密后显示名（CP932）
	PlainName []byte // 解密后明文名原始字节（key 派生 plain 变体用）
	RawName   []byte // 加密名原始字节（key 派生 raw 变体用）
	Offset    int64  // 数据真实偏移（已含 dirSize+41）
	Size      int64  // 存储大小
	Unpacked  int64  // 原始大小
	IsDir     bool
}

// Archive 一个 NPA 归档。
type Archive struct {
	Path       string
	Compressed bool
	Encrypted  bool
	arcKey     int64
	Entries    []Entry
	byName     map[string]*Entry // 小写归一化名（\ / 归一）
}

// LooksLikeNpa 头部结构合法性（不读文件系统）。
func LooksLikeNpa(head []byte, fileSize int64) bool {
	if len(head) < 41 || fileSize < 41 {
		return false
	}
	if !bytes.HasPrefix(head, []byte{'N', 'P', 'A', 0x01}) {
		return false
	}
	dirSize := int64(binary.LittleEndian.Uint32(head[37:41]))
	if dirSize <= 0 || dirSize > fileSize {
		return false
	}
	total := int64(int32(binary.LittleEndian.Uint32(head[17:21])))
	if total <= 0 || total > 1000000 {
		return false
	}
	// 目录区必须装得下 total 个最小条目（4+0+17=21 字节/条）
	if dirSize < total*21 {
		return false
	}
	return true
}

// LooksLikeNpaFile 单个文件是否为合法 NPA。
func LooksLikeNpaFile(path string) bool {
	if !strings.EqualFold(filepath.Ext(path), ".npa") {
		return false
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return false
	}
	head := make([]byte, 41)
	if _, err := io.ReadFull(f, head); err != nil {
		return false
	}
	return LooksLikeNpa(head, st.Size())
}

// sar int32 算术右移（Go 的 int64 右移即算术移位，先压回 32 位）。
func sar(v int64, n uint) int64 {
	return i32Wrap(v) >> n
}

// decName 解密条目名。index 是**含目录条目**的循环序号。
func decName(raw []byte, index int64, arcKey int64) []byte {
	out := make([]byte, len(raw))
	for x, b := range raw {
		key := int64(0xFC) * int64(x)
		key -= sar(arcKey, 24)
		key -= sar(arcKey, 16)
		key -= sar(arcKey, 8)
		key -= arcKey
		key -= sar(index, 24)
		key -= sar(index, 16)
		key -= sar(index, 8)
		key -= index
		out[x] = byte((int64(b) + key) & 0xff)
	}
	return out
}

// normKey 条目名归一化键（小写 + \ / 统一 + 剥 cg/ 前缀的两种形态都登记）。
func normKey(name string) string {
	s := strings.ToLower(name)
	s = strings.ReplaceAll(s, "\\", "/")
	return s
}

// ParseNpa 解析 NPA 归档（目录区一次性读入）。
func ParseNpa(path string) (*Archive, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	head := make([]byte, 41)
	if _, err := io.ReadFull(f, head); err != nil {
		return nil, err
	}
	if !LooksLikeNpa(head, st.Size()) {
		return nil, fmt.Errorf("nitro: 不是合法的 NPA 归档: %s", path)
	}
	key1 := int64(int32(binary.LittleEndian.Uint32(head[7:11])))
	key2 := int64(int32(binary.LittleEndian.Uint32(head[11:15])))
	total := int64(int32(binary.LittleEndian.Uint32(head[17:21])))
	dirSize := int64(binary.LittleEndian.Uint32(head[37:41]))
	dir := make([]byte, dirSize)
	if _, err := io.ReadFull(f, dir); err != nil {
		return nil, fmt.Errorf("nitro: 目录区读取失败: %w", err)
	}
	a := &Archive{
		Path:       path,
		Compressed: head[15] != 0,
		Encrypted:  head[16] != 0,
		arcKey:     i32Wrap(key1 * key2),
		byName:     map[string]*Entry{},
	}
	pos := 0
	for i := int64(0); i < total && pos+21 <= len(dir); i++ {
		nameLen := int(int32(binary.LittleEndian.Uint32(dir[pos : pos+4])))
		if nameLen < 0 || pos+4+nameLen+17 > len(dir) {
			return nil, fmt.Errorf("nitro: 目录条目 %d 名长非法", i)
		}
		raw := dir[pos+4 : pos+4+nameLen]
		isDir := dir[pos+4+nameLen] == 1
		e := Entry{RawName: append([]byte(nil), raw...), IsDir: isDir}
		if !isDir {
			off := int64(binary.LittleEndian.Uint32(dir[pos+4+nameLen+5:]))
			sz := int64(binary.LittleEndian.Uint32(dir[pos+4+nameLen+9:]))
			usz := int64(binary.LittleEndian.Uint32(dir[pos+4+nameLen+13:]))
			e.Offset = dirSize + off + 41
			e.Size = sz
			e.Unpacked = usz
			plain := decName(raw, i, a.arcKey)
			e.PlainName = plain
			e.Name = textcodec.SJIS(plain)
			a.Entries = append(a.Entries, e)
			k := normKey(e.Name)
			if _, dup := a.byName[k]; !dup {
				a.byName[k] = &a.Entries[len(a.Entries)-1]
			}
		}
		pos += 4 + nameLen + 17
	}
	if len(a.Entries) == 0 {
		return nil, fmt.Errorf("nitro: 归档里没有文件条目: %s", path)
	}
	return a, nil
}

// Lookup 按名字找条目（大小写不敏感，\ / 归一，"cg/" 前缀可选）。
func (a *Archive) Lookup(name string) *Entry {
	k := normKey(name)
	if e, ok := a.byName[k]; ok {
		return e
	}
	// 去 / 加包目录前缀重试（如 "cg/bg/x.jpg" → "bg/x.jpg"）
	if i := strings.Index(k, "/"); i >= 0 {
		if e, ok := a.byName[k[i+1:]]; ok {
			return e
		}
	}
	return nil
}

// ReadEntry 读取并解密/解压条目内容。
//
// 内容 key 派生先试**解密后明文名**（鬼哭街重制版 cg.npa），失败退回
// **加密名**（老批次 nss.npa 实测两种都有）。判定依据：魔数（jpg/png/ogg）
// 或 zlib 头，兜底解压校验。
func (a *Archive) ReadEntry(e Entry) ([]byte, error) {
	if e.IsDir {
		return nil, fmt.Errorf("nitro: 条目是目录: %s", e.Name)
	}
	f, err := os.Open(a.Path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data := make([]byte, e.Size)
	if _, err := io.ReadFull(io.NewSectionReader(f, e.Offset, e.Size), data); err != nil {
		return nil, err
	}
	if !a.Encrypted {
		return a.maybeInflate(data, e)
	}
	elen := 0x1000 + len(e.RawName)
	if elen > len(data) {
		elen = len(data)
	}
	// 变体 1：明文名
	if out, ok := a.tryKey(data, elen, entryKey(e.PlainName, e.Unpacked, a.arcKey), e); ok {
		return out, nil
	}
	// 变体 2：加密名
	if out, ok := a.tryKey(data, elen, entryKey(e.RawName, e.Unpacked, a.arcKey), e); ok {
		return out, nil
	}
	return nil, fmt.Errorf("nitro: 条目内容解密失败（两种 key 变体均未通过校验）: %s", e.Name)
}

// tryKey 用给定 key 解密前 elen 字节并校验。
func (a *Archive) tryKey(data []byte, elen int, key byte, e Entry) ([]byte, bool) {
	buf := make([]byte, len(data))
	copy(buf, data)
	for i := 0; i < elen; i++ {
		buf[i] = keyTable[buf[i]] - key - byte(i)
	}
	if !plausibleHead(buf) {
		return nil, false
	}
	out, err := a.maybeInflate(buf, e)
	if err != nil {
		return nil, false
	}
	return out, true
}

// plausibleHead 内容头魔数校验（jpg/png/ogg/zlib）。
func plausibleHead(b []byte) bool {
	if len(b) < 4 {
		return false
	}
	switch {
	case b[0] == 0xFF && b[1] == 0xD8: // jpg
		return true
	case b[0] == 0x89 && b[1] == 'P': // png
		return true
	case b[0] == 'O' && b[1] == 'g' && b[2] == 'g': // ogg
		return true
	case b[0] == 0x78 && (b[1] == 0x01 || b[1] == 0x5E || b[1] == 0x9C || b[1] == 0xDA): // zlib
		return true
	}
	return false
}

// maybeInflate 压缩归档时 zlib 解压（尾部未解密区也一并喂给流式解压器）。
func (a *Archive) maybeInflate(data []byte, e Entry) ([]byte, error) {
	if !a.Compressed || e.Size == e.Unpacked {
		return data, nil
	}
	zr, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("nitro: zlib 头非法: %w", err)
	}
	defer zr.Close()
	out, err := io.ReadAll(zr)
	if err != nil && len(out) == 0 {
		return nil, fmt.Errorf("nitro: zlib 解压失败: %w", err)
	}
	return out, nil
}
