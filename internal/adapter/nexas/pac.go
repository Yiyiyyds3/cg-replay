// pac.go —— NeXAS PAC 归档解析（Baldr Sky 实测口径）。
//
// 布局：头 12 字节（"PAC"+标记 + i32 条目数 + i32 打包版本）→ 数据区
// （连续压缩流）→ 尾部索引（末 4 字节 = 索引字节数，取反 + Huffman 解码）。
// 条目 76 字节：name[64] + offset + 原始长 + 打包长。
//
// 官方打包器的索引位流末尾不补齐位（Script.pac 实测差 3 字节），最后条目
// 字段可能残缺：zlib 流是自描述的，把残缺条目的打包长放宽到「到索引区的
// 上界」，解压自然停在流尾——精度无损。
package nexas

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strings"
)

const pacIndexEntrySize = 0x4C // 64 名字 + 3×4 字段

// PackVersion PAC 打包版本（头第 3 个 i32）。
const (
	PackNone = 0 // 直存
	PackLzss = 3 // NeXAS LZSS
	PackZlib = 4 // zlib
	PackZstd = 7 // zstd（未实现，识别时报错不猜）
)

// Entry PAC 归档条目。
type Entry struct {
	Name     string // 引擎内码字节直转的显示名（GBK/CP932 见 decode.go）
	RawName  []byte // 原始字节（跨归档匹配用，不做编码转换）
	Offset   int64
	Size     int64 // 打包长（encoded）
	Unpacked int64 // 原始长（decoded）
	PackVer  int32
	IsPacked bool // 打包长 != 原始长（且版本非 0）
}

// Archive 一个 .pac 文件。
type Archive struct {
	Path    string
	Count   int
	Version int32
	Entries []Entry
	byName  map[string]int // RawName 字节串 → 下标（字节级匹配最稳）
}

// LooksLikePac 快速判断文件头是不是 NeXAS PAC。
func LooksLikePac(head []byte, fileSize int64) bool {
	if len(head) < 12 || fileSize < 16 {
		return false
	}
	if string(head[:3]) != "PAC" {
		return false
	}
	count := int32(binary.LittleEndian.Uint32(head[4:8]))
	ver := int32(binary.LittleEndian.Uint32(head[8:12]))
	if count <= 0 || count > 1_000_000 {
		return false
	}
	switch ver {
	case PackNone, PackLzss, PackZlib, PackZstd:
		return true
	}
	return false
}

// ParsePac 解析 .pac 文件（索引 + 条目表，不读数据）。
func ParsePac(path string) (*Archive, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	head := make([]byte, 12)
	if _, err := io.ReadFull(f, head); err != nil {
		return nil, err
	}
	if string(head[:3]) != "PAC" {
		return nil, fmt.Errorf("%s: 不是 PAC 归档", path)
	}
	count := int(binary.LittleEndian.Uint32(head[4:8]))
	ver := int32(binary.LittleEndian.Uint32(head[8:12]))
	if count <= 0 || count > 1_000_000 {
		return nil, fmt.Errorf("%s: 条目数可疑（%d）", path, count)
	}
	// 尾部 4 字节 = 索引字节数。
	if _, err := f.Seek(-4, io.SeekEnd); err != nil {
		return nil, err
	}
	var sizeBuf [4]byte
	if _, err := io.ReadFull(f, sizeBuf[:]); err != nil {
		return nil, err
	}
	idxSize := int(binary.LittleEndian.Uint32(sizeBuf[:]))
	if idxSize <= 0 || idxSize > count*pacIndexEntrySize*2 || int64(idxSize)+4 > st.Size() {
		return nil, fmt.Errorf("%s: 索引大小可疑（%d）", path, idxSize)
	}
	packed := make([]byte, idxSize)
	if _, err := f.ReadAt(packed, st.Size()-4-int64(idxSize)); err != nil {
		return nil, err
	}
	index, err := neXASHuffmanDecode(packed, count*pacIndexEntrySize)
	if err != nil {
		return nil, fmt.Errorf("%s: 索引解码失败: %w", path, err)
	}
	indexStart := st.Size() - 4 - int64(idxSize) // 索引区起点（数据区终点）

	// 条目名内码投票（GBK=汉化版 / CP932=日文原版），先收集原始字节。
	rawNames := make([][]byte, 0, count)
	for i := 0; i < count; i++ {
		o := i * pacIndexEntrySize
		if len(index) < o+64 {
			break
		}
		raw := index[o : o+64]
		if nul := bytes.IndexByte(raw, 0); nul >= 0 {
			raw = raw[:nul]
		}
		rawNames = append(rawNames, append([]byte(nil), raw...))
	}
	nameCP := namesVoteCP(rawNames)

	a := &Archive{
		Path:    path,
		Count:   count,
		Version: ver,
		byName:  map[string]int{},
	}
	// 残缺的尾部条目：数据区是连续 zlib 流，offset = 上一条目末端，
	// 打包长放宽到索引区起点（zlib 自描述，解压停在流尾）。
	prevEnd := int64(12)
	for i := 0; i < count; i++ {
		o := i * pacIndexEntrySize
		raw := index[o : o+64]
		if nul := bytes.IndexByte(raw, 0); nul >= 0 {
			raw = raw[:nul]
		}
		var offset, decoded, encoded int64
		complete := len(index) >= o+pacIndexEntrySize
		if complete {
			offset = int64(binary.LittleEndian.Uint32(index[o+64 : o+68]))
			decoded = int64(binary.LittleEndian.Uint32(index[o+68 : o+72]))
			encoded = int64(binary.LittleEndian.Uint32(index[o+72 : o+76]))
		} else {
			offset = prevEnd
			encoded = indexStart - offset
			if encoded <= 0 {
				break
			}
			decoded = 0 // 读条目时实测
		}
		e := Entry{
			Name:     decodeNameCP(raw, nameCP),
			RawName:  append([]byte(nil), raw...),
			Offset:   offset,
			Size:     encoded,
			Unpacked: decoded,
			PackVer:  ver,
			IsPacked: ver != PackNone && encoded != decoded,
		}
		a.Entries = append(a.Entries, e)
		a.byName[string(e.RawName)] = len(a.Entries) - 1
		prevEnd = offset + encoded
	}
	return a, nil
}

// Lookup 按名字查条目（显示名 + 原始字节 + 省扩展名三级匹配）。
func (a *Archive) Lookup(name string) (Entry, bool) {
	for i := range a.Entries {
		if strings.EqualFold(a.Entries[i].Name, name) {
			return a.Entries[i], true
		}
	}
	if idx, ok := a.byName[name]; ok {
		return a.Entries[idx], true
	}
	// 省扩展名（.png 最常见）。
	base := strings.TrimSuffix(name, ".png")
	for i := range a.Entries {
		if strings.EqualFold(strings.TrimSuffix(a.Entries[i].Name, ".png"), base) {
			return a.Entries[i], true
		}
	}
	return Entry{}, false
}

// ReadEntry 读出并解压条目内容。
func (a *Archive) ReadEntry(e Entry) ([]byte, error) {
	f, err := os.Open(a.Path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	raw := make([]byte, e.Size)
	if _, err := f.ReadAt(raw, e.Offset); err != nil && len(raw) == 0 {
		return nil, err
	}
	if !e.IsPacked {
		if e.Unpacked > 0 && int64(len(raw)) > e.Unpacked {
			raw = raw[:e.Unpacked]
		}
		return raw, nil
	}
	switch e.PackVer {
	case PackZlib:
		zr, err := zlib.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		defer zr.Close()
		out, err := io.ReadAll(zr)
		if err != nil && len(out) == 0 {
			return nil, err
		}
		return out, nil
	case PackLzss:
		return lzssDecode(raw, int(e.Unpacked)), nil
	case PackNone:
		return raw, nil
	default:
		return nil, fmt.Errorf("%s: 不支持的打包版本 %d", a.Path, e.PackVer)
	}
}
