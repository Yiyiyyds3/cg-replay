package bgi

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"ucgr/internal/adapter"
)

// BGI .arc 有两种已确认布局（GARbro ArcBGI.cs）：
//
//	PackFile    | count | 0x20 字节条目 ...   index 从 0x10 开始
//	BURIKO ARC20| count | 0x80 字节条目 ...   index 从 0x10 开始
//
// 条目的 offset 都相对于索引之后的数据区起点；名字是 NUL 截断的 CP932。
// 这里只列目录，不解 DSC/BSE 数据段。
const (
	bgiArcMagic    = "PackFile    "
	burikoArcMagic = "BURIKO ARC20"
	bgiArcMaxCount = 0xFFFFF
)

func arcHasMagic(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	var hdr [12]byte
	if _, err := io.ReadFull(f, hdr[:]); err != nil {
		return false
	}
	s := string(hdr[:])
	return s == bgiArcMagic || s == burikoArcMagic
}

// parseARC 列出 BGI .arc 条目（只读索引，不解压）。
func parseARC(path string) ([]adapter.ArchiveEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	var hdr [16]byte
	if _, err := io.ReadFull(f, hdr[:]); err != nil {
		return nil, fmt.Errorf("bgi: .arc 头读取失败: %w", err)
	}

	switch string(hdr[:12]) {
	case bgiArcMagic:
		return parseBGIArcV1(f, st.Size(), hdr[:])
	case burikoArcMagic:
		return parseBGIArcV2(f, st.Size(), hdr[:])
	default:
		return nil, fmt.Errorf("bgi: 不是 BGI .arc（签名 %q）", hdr[:12])
	}
}

func parseBGIArcV1(f *os.File, fileSize int64, hdr []byte) ([]adapter.ArchiveEntry, error) {
	count := int64(binary.LittleEndian.Uint32(hdr[12:16]))
	if count > bgiArcMaxCount {
		return nil, fmt.Errorf("bgi: .arc 条目数异常: %d", count)
	}
	indexSize := count * 0x20
	base := int64(0x10) + indexSize
	if base > fileSize {
		return nil, fmt.Errorf("bgi: .arc 索引越界: count=%d file=%d", count, fileSize)
	}
	index := make([]byte, indexSize)
	if len(index) > 0 {
		if _, err := f.ReadAt(index, 0x10); err != nil {
			return nil, fmt.Errorf("bgi: .arc 索引读取失败: %w", err)
		}
	}
	out := make([]adapter.ArchiveEntry, 0, count)
	for i := int64(0); i < count; i++ {
		rec := index[i*0x20 : (i+1)*0x20]
		name := decodeARCName(rec[:0x10])
		rel := int64(binary.LittleEndian.Uint32(rec[0x10:]))
		size := int64(binary.LittleEndian.Uint32(rec[0x14:]))
		offset := base + rel
		if offset < base || size < 0 || offset > fileSize || size > fileSize-offset {
			return nil, fmt.Errorf("bgi: .arc 第 %d 个条目越界: offset=%d size=%d file=%d", i, offset, size, fileSize)
		}
		out = append(out, adapter.ArchiveEntry{
			Path:             name,
			Size:             size,
			Offset:           offset,
			Compression:      adapter.CompNone,
			UncompressedSize: size,
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func parseBGIArcV2(f *os.File, fileSize int64, hdr []byte) ([]adapter.ArchiveEntry, error) {
	count := int64(int32(binary.LittleEndian.Uint32(hdr[12:16])))
	if count < 0 || count > bgiArcMaxCount {
		return nil, fmt.Errorf("bgi: BURIKO arc 条目数异常: %d", count)
	}
	indexSize := count * 0x80
	base := int64(0x10) + indexSize
	if base > fileSize {
		return nil, fmt.Errorf("bgi: BURIKO arc 索引越界: count=%d file=%d", count, fileSize)
	}
	index := make([]byte, indexSize)
	if len(index) > 0 {
		if _, err := f.ReadAt(index, 0x10); err != nil {
			return nil, fmt.Errorf("bgi: BURIKO arc 索引读取失败: %w", err)
		}
	}
	out := make([]adapter.ArchiveEntry, 0, count)
	for i := int64(0); i < count; i++ {
		rec := index[i*0x80 : (i+1)*0x80]
		name := decodeARCName(rec[:0x60])
		rel := int64(binary.LittleEndian.Uint32(rec[0x60:]))
		size := int64(binary.LittleEndian.Uint32(rec[0x64:]))
		offset := base + rel
		if offset < base || size < 0 || offset > fileSize || size > fileSize-offset {
			return nil, fmt.Errorf("bgi: BURIKO arc 第 %d 个条目越界: offset=%d size=%d file=%d", i, offset, size, fileSize)
		}
		out = append(out, adapter.ArchiveEntry{
			Path:             name,
			Size:             size,
			Offset:           offset,
			Compression:      adapter.CompNone,
			UncompressedSize: size,
			Encrypted:        bseEncrypted(f, offset, size),
			KeyHint:          bseKeyHint(f, offset, size),
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// bseEncrypted 判断条目是否为加密的 BSE 1.00/1.01 图像。
func bseEncrypted(f *os.File, offset, size int64) bool {
	if size < 0x50 {
		return false
	}
	var hdr [0x10]byte
	if _, err := f.ReadAt(hdr[:], offset); err != nil {
		return false
	}
	if string(hdr[:6]) != "BSE 1." {
		return false
	}
	v := binary.LittleEndian.Uint16(hdr[8:10])
	return v == 0x100 || v == 0x101
}

func bseKeyHint(f *os.File, offset, size int64) string {
	if !bseEncrypted(f, offset, size) {
		return ""
	}
	return "BSE 1.00/1.01 头加密"
}

// decodeARCName 按 NUL 截断后解 CP932；失败时退回原字节。
func decodeARCName(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	if len(b) == 0 {
		return ""
	}
	ascii := true
	for _, c := range b {
		if c >= 0x80 {
			ascii = false
			break
		}
	}
	if ascii {
		return strings.ReplaceAll(string(b), `\`, "/")
	}
	s, err := mbToWide(b, 932)
	if err != nil {
		return strings.ReplaceAll(string(b), `\`, "/")
	}
	return strings.ReplaceAll(s, `\`, "/")
}
