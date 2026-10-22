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
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"ucgr/internal/adapter"
)

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
}

// openArchives 打开归档集合并索引全部条目（同名路径的多段按出现顺序归并）。
func openArchives(archives []string, parse func(string) ([]adapter.ArchiveEntry, error)) (*archiveSet, error) {
	s := &archiveSet{
		archives: archives,
		files:    map[string]*os.File{},
		entries:  map[string]archiveEntry{},
		owner:    map[string]string{},
	}
	for _, arch := range archives {
		segs, err := parse(arch)
		if err != nil {
			s.close()
			return nil, fmt.Errorf("解析归档 %s 失败：%w", filepath.Base(arch), err)
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
		for _, p := range order {
			if _, exists := s.entries[p]; exists {
				continue // 同名条目以先出现的归档为准
			}
			s.entries[p] = archiveEntry{path: p, segs: grouped[p], encrypted: enc[p]}
			s.owner[p] = arch
			if enc[p] {
				s.encrypted = append(s.encrypted, p)
			}
		}
	}
	sort.Strings(s.encrypted)
	return s, nil
}

// close 关掉所有归档句柄。
func (s *archiveSet) close() {
	for _, f := range s.files {
		_ = f.Close()
	}
	s.files = map[string]*os.File{}
}

// read 读一个条目的完整内容（多段顺序拼接，每段各自解压）。
func (s *archiveSet) read(e archiveEntry) ([]byte, error) {
	arch, ok := s.owner[e.path]
	if !ok {
		return nil, fmt.Errorf("条目 %s 不知道属于哪个归档", e.path)
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
