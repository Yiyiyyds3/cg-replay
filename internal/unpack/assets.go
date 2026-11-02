// assets.go —— 引擎无关的归档条目集。
//
// 四个引擎的归档容器不同（XP3 / PFS / ARC / KIF），但**条目模型一致**：
// 每个条目有路径 + 一个或多个「段」（偏移/长度/压缩方式）。解包层只做编排，
// 容器解析与段读取复用各引擎适配器已有的能力：
//
//   - 条目列举：引擎适配器的 ParseArchive
//   - 段读取：adapter.ReadArchiveEntry（seek + 读 + zlib）
//   - 加密标记：条目自带 Encrypted/KeyHint（XP3 的 adlr、ARC 的 BSE 加密图）
package unpack

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"ucgr/internal/adapter"
)

// archiveParseError 带上「是哪个归档解析失败」，供上层按真实文件嗅探格式。
// 只返回 error 文本的话，调用方只能拿 archives[0] 去猜，多归档时经常猜错。
type archiveParseError struct {
	Path string
	Err  error
}

func (e *archiveParseError) Error() string {
	return fmt.Sprintf("解析归档 %s 失败：%v", filepath.Base(e.Path), e.Err)
}

func (e *archiveParseError) Unwrap() error { return e.Err }

// archiveEntry 归档里的一个文件（多段已归并）。
type archiveEntry struct {
	path      string
	segs      []adapter.ArchiveEntry
	encrypted bool
}

// base 取路径的文件名。
func (e archiveEntry) base() string { return filepath.Base(filepath.FromSlash(e.path)) }

// stem 取去掉扩展名的文件名（storage 名常常不带扩展名）。
func (e archiveEntry) stem() string {
	b := e.base()
	return strings.TrimSuffix(b, filepath.Ext(b))
}

// archiveSet 一批归档的条目索引（持有文件句柄，避免每读一条就重扫归档）。
type archiveSet struct {
	archives  []string
	files     map[string]*os.File
	entries   map[string]archiveEntry
	owner     map[string]string // 条目路径 → 所属归档
	encrypted []string
	// adlr 专指 XP3 的 adlr 校验和块（KeyHint=adlr）；CS2 的 KIF 是 Blowfish
	// 加密、读取时自动解密，不该被算成「带 adlr」。
	adlr     []string
	skipped  []string // 解析失败的归档（含原因）——不阻断其余归档
	warnings []string // 非致命问题（如解密读取器打不开）

	// 引擎自带的条目读取器（CS2 的 KIF 需要解密）；缺省走通用 seek+zlib。
	readers map[string]func([]adapter.ArchiveEntry) ([]byte, error)
	closers []func() error
}

// openArchives 打开归档集合并索引全部条目（同名路径的多段按出现顺序归并）。
//
// 单个坏归档不再拖垮整批：解析失败的归档记 skipped 后跳过，其余归档照常
// 建索引；只有「一个都打不开」时才返回错误，并带上第一个失败归档的路径。
func openArchives(archives []string, parse func(string) ([]adapter.ArchiveEntry, error), openReader func(string) (func([]adapter.ArchiveEntry) ([]byte, error), func() error, error)) (*archiveSet, error) {
	s := &archiveSet{
		archives: archives,
		files:    map[string]*os.File{},
		entries:  map[string]archiveEntry{},
		owner:    map[string]string{},
		readers:  map[string]func([]adapter.ArchiveEntry) ([]byte, error){},
	}
	var firstErr *archiveParseError
	for _, arch := range archives {
		segs, err := parse(arch)
		if err != nil {
			// 一个坏归档不该拖垮整场解包：实测有的游戏目录里混着扩展名骗人的
			// 文件（如 .int 其实是 EVB）。记下来，继续处理其余归档。
			if firstErr == nil {
				firstErr = &archiveParseError{Path: arch, Err: err}
			}
			msg := fmt.Sprintf("%s：%v", filepath.Base(arch), err)
			if hint := sniffHint(arch); hint != "" {
				msg += "（" + hint + "）"
			}
			s.skipped = append(s.skipped, msg)
			continue
		}
		order := []string{}
		grouped := map[string][]adapter.ArchiveEntry{}
		enc := map[string]bool{}
		for _, e := range segs {
			if _, ok := grouped[e.Path]; !ok {
				order = append(order, e.Path)
			}
			grouped[e.Path] = append(grouped[e.Path], e)
			if e.Encrypted {
				enc[e.Path] = true
			}
		}
		if openReader != nil {
			if read, closeFn, rerr := openReader(arch); rerr == nil {
				s.readers[arch] = read
				if closeFn != nil {
					s.closers = append(s.closers, closeFn)
				}
			} else {
				// 不能静默回落：通用读取只做 zlib，KIF 的 Blowfish 数据会
				// 原样读成密文，最后表现为「脚本解析失败」而看不到根因。
				s.warnings = append(s.warnings,
					fmt.Sprintf("%s 的解密读取器打不开：%v", filepath.Base(arch), rerr))
			}
		}
		for _, p := range order {
			if _, exists := s.entries[p]; exists {
				// 同名条目以后出现的归档为准：补丁包（update.xp3 等）通常
				// 排在 base 之后，先到先得会拿到旧脚本。
				s.encrypted = removeString(s.encrypted, p)
				s.adlr = removeString(s.adlr, p)
			}
			s.entries[p] = archiveEntry{path: p, segs: grouped[p], encrypted: enc[p]}
			s.owner[p] = arch
			if enc[p] {
				s.encrypted = append(s.encrypted, p)
				if strings.EqualFold(grouped[p][0].KeyHint, "adlr") {
					s.adlr = append(s.adlr, p)
				}
			}
		}
	}
	if len(s.entries) == 0 {
		s.close()
		if firstErr != nil {
			return nil, firstErr
		}
		if len(s.skipped) > 0 {
			return nil, fmt.Errorf("所有归档都无法解析：%s", strings.Join(s.skipped, "；"))
		}
	}
	sort.Strings(s.encrypted)
	return s, nil
}

// failedArchivePath 从 openArchives 的错误里取出真正失败的那个归档。
// 不是 archiveParseError（例如所有归档都为空）时返回空串，调用方再回退。
func failedArchivePath(err error) string {
	var ae *archiveParseError
	if errors.As(err, &ae) {
		return ae.Path
	}
	return ""
}

// removeString 删掉切片里所有等于 v 的元素（保持顺序）。
func removeString(in []string, v string) []string {
	out := in[:0]
	for _, s := range in {
		if s != v {
			out = append(out, s)
		}
	}
	return out
}

// close 关掉所有归档句柄。
func (s *archiveSet) close() {
	for _, f := range s.files {
		_ = f.Close()
	}
	s.files = map[string]*os.File{}
	for _, c := range s.closers {
		_ = c()
	}
	s.closers = nil
}

// read 读一个条目的完整内容（多段顺序拼接，每段各自解压）。
func (s *archiveSet) read(e archiveEntry) ([]byte, error) {
	arch, ok := s.owner[e.path]
	if !ok {
		return nil, fmt.Errorf("条目 %s 不知道属于哪个归档", e.path)
	}
	if read, ok := s.readers[arch]; ok {
		return read(e.segs)
	}
	f, ok := s.files[arch]
	if !ok {
		var err error
		f, err = os.Open(arch)
		if err != nil {
			return nil, err
		}
		s.files[arch] = f
	}
	var out []byte
	for _, seg := range e.segs {
		b, err := adapter.ReadArchiveEntry(f, seg)
		if err != nil {
			return nil, err
		}
		out = append(out, b...)
	}
	return out, nil
}

// nameIndex 按文件名与「无扩展名」各建一份索引——演出脚本里的 storage 名
// 常常不带扩展名，两份索引都要有才找得到。
func (s *archiveSet) nameIndex() (map[string]archiveEntry, map[string]archiveEntry) {
	byBase := map[string]archiveEntry{}
	byStem := map[string]archiveEntry{}
	for _, e := range s.entries {
		lb := strings.ToLower(e.base())
		if _, ok := byBase[lb]; !ok {
			byBase[lb] = e
		}
		ls := strings.ToLower(e.stem())
		if _, ok := byStem[ls]; !ok {
			byStem[ls] = e
		}
	}
	return byBase, byStem
}

// lookup 按引用名找条目：先按完整文件名，再按去扩展名的名字。
func (s *archiveSet) lookup(ref string) (archiveEntry, bool) {
	byBase, byStem := s.nameIndex()
	ref = strings.TrimSpace(ref)
	lb := strings.ToLower(filepath.Base(filepath.FromSlash(ref)))
	if e, ok := byBase[lb]; ok {
		return e, true
	}
	ls := strings.TrimSuffix(lb, filepath.Ext(lb))
	if e, ok := byStem[ls]; ok {
		return e, true
	}
	return archiveEntry{}, false
}
