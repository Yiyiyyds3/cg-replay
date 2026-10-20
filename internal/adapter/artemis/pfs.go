package artemis

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

// PFS 是 Artemis 引擎的资源归档（GARbro ArcPFS.cs）：
//
//	pf<ver> | uint32 index_size | int32 count | index...
//
// v6/v8 的 index 从文件偏移 7 开始；每个条目为
// int32 name_len | name | 4 字节未知 | uint32 offset | uint32 size。
// v6 明文；v8 的数据用 SHA1(index) 循环 XOR。v2 的 count 在 0xB，
// 条目里多一段 0x10 字节的尾部信息，且不加密。
const (
	pfsMaxCount = 1 << 20
	pfsMaxIndex = 256 << 20
)

// parsePFS 列出 .pfs 归档条目（只读索引，不读取数据段）。
func parsePFS(path string) ([]adapter.ArchiveEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	var hdr [15]byte
	if _, err := io.ReadFull(f, hdr[:11]); err != nil {
		return nil, fmt.Errorf("artemis: pfs 头读取失败: %w", err)
	}
	if string(hdr[:2]) != "pf" {
		return nil, fmt.Errorf("artemis: 不是 PFS 归档（签名 %q）", hdr[:2])
	}
	version := int(hdr[2] - '0')

	switch version {
	case 6, 8:
		return parsePFSIndex(f, st.Size(), hdr[:], version, false)
	case 2:
		if _, err := io.ReadFull(f, hdr[11:15]); err != nil {
			return nil, fmt.Errorf("artemis: pfs v2 头读取失败: %w", err)
		}
		return parsePFSIndex(f, st.Size(), hdr[:], version, true)
	default:
		return nil, fmt.Errorf("artemis: 不支持的 PFS 版本 %d（当前支持 v2/v6/v8）", version)
	}
}

func parsePFSIndex(f *os.File, fileSize int64, hdr []byte, version int, v2 bool) ([]adapter.ArchiveEntry, error) {
	indexSize := binary.LittleEndian.Uint32(hdr[3:7])
	countAt := 7
	if v2 {
		countAt = 0xB
	}
	count := int(binary.LittleEndian.Uint32(hdr[countAt : countAt+4]))
	if count < 0 || count > pfsMaxCount {
		return nil, fmt.Errorf("artemis: PFS 条目数异常: %d", count)
	}
	if indexSize > pfsMaxIndex || int64(indexSize) > fileSize-7 {
		return nil, fmt.Errorf("artemis: PFS 索引大小异常: %d（文件 %d 字节）", indexSize, fileSize)
	}
	index := make([]byte, indexSize)
	if len(index) > 0 {
		if _, err := f.ReadAt(index, 7); err != nil {
			return nil, fmt.Errorf("artemis: PFS 索引读取失败: %w", err)
		}
	}

	indexOffset := 4
	if v2 {
		indexOffset = 8
	}
	out := make([]adapter.ArchiveEntry, 0, count)
	for i := 0; i < count; i++ {
		if indexOffset+4 > len(index) {
			return nil, fmt.Errorf("artemis: PFS 第 %d 个条目越过索引末尾", i)
		}
		nameLen := int(int32(binary.LittleEndian.Uint32(index[indexOffset:])))
		if nameLen < 0 || nameLen > len(index)-indexOffset-4 {
			return nil, fmt.Errorf("artemis: PFS 第 %d 个条目名长度异常: %d", i, nameLen)
		}
		nameBytes := index[indexOffset+4 : indexOffset+4+nameLen]
		if v2 {
			indexOffset += nameLen + 0x10
		} else {
			indexOffset += nameLen + 8
		}
		if indexOffset+8 > len(index) {
			return nil, fmt.Errorf("artemis: PFS 第 %d 个条目缺少 offset/size", i)
		}
		offset := int64(binary.LittleEndian.Uint32(index[indexOffset:]))
		size := int64(binary.LittleEndian.Uint32(index[indexOffset+4:]))
		indexOffset += 8
		if offset < 0 || size < 0 || offset > fileSize || size > fileSize-offset {
			return nil, fmt.Errorf("artemis: PFS 第 %d 个条目越界: offset=%d size=%d file=%d", i, offset, size, fileSize)
		}

		name := decodePFSName(nameBytes)
		out = append(out, adapter.ArchiveEntry{
			Path:             name,
			Size:             size,
			Offset:           offset,
			Compression:      adapter.CompNone,
			UncompressedSize: size,
			Encrypted:        version == 8,
			KeyHint:          pfsKeyHint(version),
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func pfsKeyHint(version int) string {
	if version == 8 {
		return "SHA1(index) XOR"
	}
	return ""
}

// decodePFSName 先按 CP932 解，失败时退回原字节（UTF-8/ASCII 兜底）。
func decodePFSName(b []byte) string {
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
