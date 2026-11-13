// assets.go —— 引擎无关的归档条目集。
//
// 四个引擎的归档容器不同（XP3 / PFS / ARC / KIF），但**条目模型一致**：
// 每个条目有路径 + 一个或多个「段」（偏移/长度/压缩方式）。解包层只做编排，
// 容器解析与段读取复用各引擎适配器已有的能力：
//
//   - 条目列举：引擎适配器的 ParseArchive
//   - 段读取：adapter.ReadArchiveEntry（seek + 读原始字节；**不解压**）
//   - 段解压：本包按段做 zlib——解压必须留在解包层，因为后续口径是
//     「先解压、再对整份数据解密」（见 decrypt.go），本层才掌握引擎差异
//   - 加密标记：条目自带 Encrypted/KeyHint（XP3 的 adlr、ARC 的 BSE 加密图）
package unpack

import (
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"ucgr/internal/adapter"
	"ucgr/internal/asset"
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

	// 名字索引只建一次：lookup 原来每次调用都重建，条目上万时是 O(n²)。
	// 同名条目保留全部候选：脚本可能只给裸名，必须按引用里的目录/角色提示
	// 选对，不能按 map 顺序随便拿一个（否则背景图会顶掉同名 CG）。
	byBase map[string][]archiveEntry // 文件名（小写，含扩展名）→ 条目
	byStem map[string][]archiveEntry // 去扩展名的文件名（小写）→ 条目
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
		// ReadArchiveEntry 对压缩段只读原始字节（留给调用方处理加密层），
		// 解压必须在这里做——多段各自 zlib，顺序拼接才是完整文件。
		if seg.Compression == adapter.CompZlib {
			zr, err := zlib.NewReader(bytes.NewReader(b))
			if err != nil {
				return nil, fmt.Errorf("段不是合法 zlib %s: %w", e.path, err)
			}
			plain, err := io.ReadAll(zr)
			_ = zr.Close()
			if err != nil {
				return nil, fmt.Errorf("段解压失败 %s: %w", e.path, err)
			}
			b = plain
		}
		out = append(out, b...)
	}
	return out, nil
}

// nameIndex 按文件名与「无扩展名」各建一份索引——演出脚本里的 storage 名
// 常常不带扩展名，两份索引都要有才找得到。
//
// 索引只建一次（lookup 会被引用集反复调用）。同名条目全部保留，按路径
// 排序后用 lookup 的角色/目录评分选最佳候选，保证结果确定且不把背景图
// 当成同名 CG。
func (s *archiveSet) nameIndex() (map[string][]archiveEntry, map[string][]archiveEntry) {
	if s.byBase != nil {
		return s.byBase, s.byStem
	}
	paths := make([]string, 0, len(s.entries))
	for p := range s.entries {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	s.byBase = map[string][]archiveEntry{}
	s.byStem = map[string][]archiveEntry{}
	for _, p := range paths {
		e := s.entries[p]
		lb := strings.ToLower(e.base())
		s.byBase[lb] = append(s.byBase[lb], e)
		ls := strings.ToLower(e.stem())
		s.byStem[ls] = append(s.byStem[ls], e)
	}
	return s.byBase, s.byStem
}

// lookup 按引用名找条目：先按完整文件名，再按去扩展名的名字；同名候选
// 按「引用里的目录提示 + 角色一致 + 路径后缀」评分，分数相同时路径序靠前。
func (s *archiveSet) lookup(ref string) (archiveEntry, bool) {
	byBase, byStem := s.nameIndex()
	ref = strings.TrimSpace(ref)
	lb := strings.ToLower(filepath.Base(filepath.FromSlash(ref)))
	ls := strings.TrimSuffix(lb, filepath.Ext(lb))
	var cands []archiveEntry
	cands = append(cands, byBase[lb]...)
	cands = append(cands, byStem[ls]...)
	if len(cands) == 0 {
		return archiveEntry{}, false
	}
	best := cands[0]
	bestScore := -1
	for _, e := range cands {
		score := assetLookupScore(ref, e.path)
		if score > bestScore {
			best, bestScore = e, score
		}
	}
	return best, true
}

// assetLookupScore 给同名候选打分：引用里的目录/角色提示越具体，分越高。
func assetLookupScore(ref, path string) int {
	refSlash := strings.ToLower(filepath.ToSlash(strings.TrimSpace(ref)))
	pathSlash := strings.ToLower(filepath.ToSlash(path))
	refStem := strings.TrimSuffix(strings.TrimPrefix(filepath.ToSlash(refSlash), "/"), filepath.Ext(refSlash))
	pathStem := strings.TrimSuffix(strings.TrimPrefix(filepath.ToSlash(pathSlash), "/"), filepath.Ext(pathSlash))
	score := 0
	if refStem != "" && (pathStem == refStem || strings.HasSuffix(pathStem, "/"+refStem)) {
		score += 100 + len(refStem)*2
	}
	if asset.RoleOf(ref) == asset.RoleOf(path) {
		score += 60
	}
	// 引用里带扩展名时，扩展名一致是强提示（foo.png 不该拿 foo.tlg）。
	refExt := strings.ToLower(filepath.Ext(refSlash))
	if refExt != "" && refExt == strings.ToLower(filepath.Ext(pathSlash)) {
		score += 10
	}
	// 同时命中多个候选时，浅层路径更可能是游戏主素材。
	score -= strings.Count(pathSlash, "/")
	return score
}
