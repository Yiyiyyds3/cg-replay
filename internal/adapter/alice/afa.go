// afa.go —— AliceSoft AFA 归档（AlicArch v2，兰斯10 实测破解）。
//
// 头 44 字节 = "AFAH" + u32(0x1c) + "AlicArch" + u32 version + u32 unk
// + u32 dataStart + "INFO" + u32 tablePacked(+16) + u32 tableUnpacked
// + u32 nrFiles；表（zlib，@44 起）解压后是连续条目：
//
//	u32 nameLen + u32 strLen + name[strLen]（GBK，尾含 \0）
//	u32 unk0 + u32 unk1 + u32 off + u32 size
//
// 数据区 @dataStart："DATA" + u32 dataSize + 条目流；条目数据 @
// dataStart+off（首条 off=8 即 DATA 头大小，off 链单调衔接）。
// 表解析以「消费字节数 = 表长」验证（Rance10CG.afa 3460 条全通过）。
package alice

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strings"
)

// AFAEntry 归档条目。
type AFAEntry struct {
	Name string // 去掉尾部 \0 的条目名（GBK → UTF-8）
	Off  int64  // 相对 dataStart 的偏移
	Size int32
}

// AFA 一个 .afa 归档。
type AFA struct {
	Path      string
	Version   uint32
	DataStart int64
	fileSize  int64
	Entries   []AFAEntry
	byName    map[string]int // 规范化名 → 下标
}

// LooksLikeAFA 报告文件头是不是 AFAH/AlicArch。
func LooksLikeAFA(head []byte) bool {
	return len(head) >= 44 && bytes.Equal(head[:4], []byte("AFAH")) &&
		bytes.Equal(head[8:16], []byte("AlicArch"))
}

// LooksLikeAFAFile 报告路径指向 AFA 归档。
func LooksLikeAFAFile(path string) bool {
	head, err := readHead(path, 44)
	if err != nil {
		return false
	}
	return LooksLikeAFA(head)
}

// ParseAFA 解析 .afa 归档。
func ParseAFA(path string) (*AFA, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if !LooksLikeAFA(data) {
		return nil, fmt.Errorf("alice: %s 不是 AFA 归档", path)
	}
	a := &AFA{Path: path}
	a.Version = binary.LittleEndian.Uint32(data[16:])
	a.DataStart = int64(binary.LittleEndian.Uint32(data[24:]))
	tablePacked := int(binary.LittleEndian.Uint32(data[32:])) - 16
	tableUnc := int(binary.LittleEndian.Uint32(data[36:]))
	nr := int(binary.LittleEndian.Uint32(data[40:]))
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	a.fileSize = st.Size()

	if string(data[a.DataStart:a.DataStart+4]) != "DATA" {
		return nil, fmt.Errorf("alice: %s 数据区缺 DATA 标记", path)
	}
	tbl, err := inflate(data[44 : 44+tablePacked], tableUnc)
	if err != nil {
		return nil, fmt.Errorf("alice: %s 条目表解压失败：%w", path, err)
	}
	r := &reader{buf: tbl}
	a.Entries = make([]AFAEntry, 0, nr)
	for i := 0; i < nr; i++ {
		if r.i+8 > len(tbl) {
			return nil, fmt.Errorf("alice: %s 条目表截断（%d/%d）", path, i, nr)
		}
		r.i32() // nameLen（= strLen-1，展示用，实际读 strLen 字节）
		strLen := int(r.u32())
		if strLen <= 0 || r.i+strLen > len(tbl) {
			return nil, fmt.Errorf("alice: %s 条目 %d 名字越界", path, i)
		}
		name := decodeGBK(tbl[r.i : r.i+strLen])
		name = strings.TrimRight(name, "\x00")
		r.i += strLen
		r.i32() // unk0
		r.i32() // unk1
		off := r.u32()
		size := r.i32()
		a.Entries = append(a.Entries, AFAEntry{Name: name, Off: int64(off), Size: size})
	}
	a.buildIndex()
	return a, nil
}

func (a *AFA) buildIndex() {
	a.byName = make(map[string]int, len(a.Entries))
	for i := range a.Entries {
		key := normKey(a.Entries[i].Name)
		if _, dup := a.byName[key]; !dup {
			a.byName[key] = i
		}
	}
}

// normKey 规范化查找键（小写 + 全半角 ／ 归一）。
func normKey(name string) string {
	s := strings.ToLower(name)
	s = strings.ReplaceAll(s, "／", "/")
	s = strings.ReplaceAll(s, "\\", "/")
	return s
}

// Lookup 精确名（含扩展名）查条目。
func (a *AFA) Lookup(name string) *AFAEntry {
	if a.byName == nil {
		return nil
	}
	if i, ok := a.byName[normKey(name)]; ok {
		return &a.Entries[i]
	}
	return nil
}

// LookupStem 按名字后缀（去扩展名）匹配条目——AFA 条目带「立／」「事／」
// 等目录前缀，脚本里引用的是裸名（「萨娜琪亚／基本」）。取**最长**匹配
// （名字更具体者优先）。
func (a *AFA) LookupStem(stem string) *AFAEntry {
	if a.byName == nil {
		return nil
	}
	key := normKey(stem)
	best := -1
	bestLen := -1
	for i := range a.Entries {
		nk := normKey(a.Entries[i].Name)
		nk = strings.TrimSuffix(nk, ".ajp")
		nk = strings.TrimSuffix(nk, ".qnt")
		nk = strings.TrimSuffix(nk, ".dcf")
		if !strings.HasSuffix(nk, "/"+key) && nk != key {
			continue
		}
		if len(nk) > bestLen {
			best = i
			bestLen = len(nk)
		}
	}
	if best < 0 {
		return nil
	}
	return &a.Entries[best]
}

// ReadEntry 读条目数据。
func (a *AFA) ReadEntry(e AFAEntry) ([]byte, error) {
	f, err := os.Open(a.Path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	start := a.DataStart + e.Off
	if e.Size < 0 || start+int64(e.Size) > a.fileSize {
		return nil, fmt.Errorf("alice: 条目 %s 越界（off=%d size=%d）", e.Name, e.Off, e.Size)
	}
	buf := make([]byte, e.Size)
	if _, err := f.ReadAt(buf, start); err != nil && err != io.EOF {
		return nil, err
	}
	return buf, nil
}

// inflate zlib 解压（期望 unc 字节）。
func inflate(data []byte, unc int) ([]byte, error) {
	zr, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	out := make([]byte, 0, unc)
	buf := make([]byte, 64*1024)
	for len(out) < unc {
		n, err := zr.Read(buf)
		out = append(out, buf[:n]...)
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
	}
	return out, nil
}

// readHead 读文件头部。
func readHead(path string, n int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf := make([]byte, n)
	m, err := f.Read(buf)
	if err != nil && m == 0 {
		return nil, err
	}
	return buf[:m], nil
}
