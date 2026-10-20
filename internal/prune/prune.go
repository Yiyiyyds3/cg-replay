// Package prune 引用裁剪器：把脚本引用匹配到解包树真实文件。
//
// 对应 docs/05 阶段 C。匹配规则（P0 实测驱动，同 gar_tool/ks_refs.py）：
//  1. 精确 relpath 命中
//  2. 补全常见扩展名（tlg/png/ogg/hg3…）
//  3. basename 唯一命中
//  4. 同名多版本（基础卷/汉化卷）全部命中
//
// 未命中引用走保守候选（dynamic）：剥 ev/e/large_ 前缀取核心 token，
// 文件名含该 token 的全部保留——宁多勿缺（安全方向）。
package prune

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
)

// RES_EXTS 常见资源扩展名（KiriKiri + CatSystem2 + Artemis）。
var RES_EXTS = []string{
	".tlg", ".png", ".jpg", ".jpeg", ".bmp", ".ogg", ".wav", ".mp3",
	".mpg", ".mpeg", ".wmv", ".ogv", ".asf", ".avi", ".mp4", ".csv",
	".txt", ".tjs", ".asd", ".ks", ".ma", ".amv", ".sli", ".func",
	".scn", ".sinfo", ".pimg", ".fdr", ".psb", ".dat",
	".hg3", ".hg2", ".fes", ".kcs", ".kx2", ".mot", ".ptcl", ".lpb",
	".psz", ".zt", ".anm",
}

// Pool 解包资源池（relpath → 绝对路径 + 大小）。
type Pool struct {
	Root    string
	ByRel   map[string]PoolFile // NFKC 归一 relpath → 文件
	ByName  map[string][]string // basename → relpath 列表
	ByGroup map[string][]string // 多图层组：<name> → [<name>_<序号>...]（CS2 展开）
}

// PoolFile 池内一个文件。
type PoolFile struct {
	Rel  string
	Abs  string
	Size int64
}

// groupRe 多图层文件名：<name>_<序号>。
var groupRe = regexp.MustCompile(`^(.*)_\d+$`)

// BuildPool 遍历 root 建池（排除 .ks/.cst 剧本与常见副本目录）。
// 剧本按 DR-25 全留，不进资产池（与 Python 原型 ks_refs.build_pool /
// cst_refs.build_pool_cs2 同口径）；否则 prune 会把剧本本身当“未命中”裁掉。
func BuildPool(root string) (*Pool, error) {
	return BuildPoolWithExtra(root)
}

// BuildPoolWithExtra 在 root 之外再挂若干素材根（例如 CS2 的 ../音频）。
// 外部根的 rel 会写成 ../音频/...，回放时按同一个 root 仍能解析。
func BuildPoolWithExtra(root string, extras ...string) (*Pool, error) {
	p := &Pool{
		Root:    root,
		ByRel:   map[string]PoolFile{},
		ByName:  map[string][]string{},
		ByGroup: map[string][]string{},
	}
	excludeDir := map[string]bool{"解包输出": true, "_分类": true}
	addRoot := func(base string) error {
		return filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if excludeDir[d.Name()] {
					return filepath.SkipDir
				}
				return nil
			}
			if !d.Type().IsRegular() {
				return nil
			}
			switch strings.ToLower(filepath.Ext(path)) {
			case ".ks", ".cst":
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			rel := normRel(path, root)
			if _, exists := p.ByRel[rel]; exists {
				return nil
			}
			p.ByRel[rel] = PoolFile{Rel: rel, Abs: path, Size: info.Size()}
			base := filepath.Base(rel)
			p.ByName[base] = append(p.ByName[base], rel)
			// 多图层组索引：Tama01f_0001 → 组 "tama01f"
			noext := strings.TrimSuffix(base, filepath.Ext(base))
			if m := groupRe.FindStringSubmatch(noext); m != nil {
				p.ByGroup[m[1]] = append(p.ByGroup[m[1]], rel)
			} else {
				p.ByGroup[noext] = append(p.ByGroup[noext], rel)
			}
			return nil
		})
	}
	if err := addRoot(root); err != nil {
		return nil, err
	}
	for _, extra := range extras {
		if extra == "" {
			continue
		}
		if _, err := os.Stat(extra); err != nil {
			return nil, err
		}
		if err := addRoot(extra); err != nil {
			return nil, err
		}
	}
	return p, nil
}

func normRel(path, root string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		rel = path
	}
	return normalize(rel)
}

func normalize(s string) string {
	s = strings.ReplaceAll(s, "\\", "/")
	// 全角/半角归一；保护 ／（合法文件名字符，转成 / 会伪造路径分隔符）
	s = strings.ReplaceAll(s, "／", "\x00")
	s = strings.ToLower(nfkc(s))
	return strings.ReplaceAll(s, "\x00", "／")
}

func nfkc(s string) string {
	// 简化归一：全角字母数字 → 半角。完整 NFKC 表过大，此处够用。
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '！':
			b.WriteRune('!')
		case '＂':
			b.WriteRune('"')
		case '＃':
			b.WriteRune('#')
		case '％':
			b.WriteRune('%')
		case '＆':
			b.WriteRune('&')
		case '＇':
			b.WriteRune('\'')
		case '（':
			b.WriteRune('(')
		case '）':
			b.WriteRune(')')
		case '＋':
			b.WriteRune('+')
		case '，':
			b.WriteRune(',')
		case '－':
			b.WriteRune('-')
		case '．':
			b.WriteRune('.')
		case '／':
			b.WriteRune('/')
		case '：':
			b.WriteRune(':')
		case '；':
			b.WriteRune(';')
		case '＜':
			b.WriteRune('<')
		case '＝':
			b.WriteRune('=')
		case '＞':
			b.WriteRune('>')
		case '？':
			b.WriteRune('?')
		case '＠':
			b.WriteRune('@')
		case '［':
			b.WriteRune('[')
		case '］':
			b.WriteRune(']')
		case '｛':
			b.WriteRune('{')
		case '｝':
			b.WriteRune('}')
		case '～':
			b.WriteRune('~')
		default:
			// 全角英数字
			if r >= 0xFF21 && r <= 0xFF3A {
				b.WriteRune(r - 0xFF21 + 'A')
			} else if r >= 0xFF41 && r <= 0xFF5A {
				b.WriteRune(r - 0xFF41 + 'a')
			} else if r >= 0xFF10 && r <= 0xFF19 {
				b.WriteRune(r - 0xFF10 + '0')
			} else {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

// Match 单引用匹配：返回命中的 relpath 列表（可能多版本）。
func (p *Pool) Match(ref string) []string {
	r := normalize(strings.TrimPrefix(ref, "./"))
	if r == "" {
		return nil
	}
	if _, ok := p.ByRel[r]; ok {
		return []string{r}
	}
	if filepath.Ext(r) == "" {
		// 1) relpath + 扩展名
		for _, ext := range RES_EXTS {
			if _, ok := p.ByRel[r+ext]; ok {
				return []string{r + ext}
			}
		}
		// 2) basename + 扩展名（引用是裸名，池键是完整 relpath）
		for _, ext := range RES_EXTS {
			if hits := p.ByName[r+ext]; len(hits) > 0 {
				return hits
			}
		}
		if hits := p.ByName[r]; len(hits) > 0 {
			return hits
		}
	} else {
		if hits := p.ByName[r]; len(hits) > 0 {
			return hits
		}
	}
	// 3) 多图层组（CS2：Tama01f → Tama01f_0001..N，整组缺一不可渲染）
	if hits := p.ByGroup[r]; len(hits) > 0 {
		return sortedSet(hits)
	}
	// 4) 前缀唯一
	var cand []string
	for k := range p.ByRel {
		if strings.HasPrefix(filepath.Base(k), r+".") {
			cand = append(cand, k)
		}
	}
	if len(cand) == 1 {
		return cand
	}
	return nil
}

// sortedSet 排序去重。
func sortedSet(ss []string) []string {
	if len(ss) == 0 {
		return nil
	}
	sort.Strings(ss)
	out := ss[:1]
	for _, s := range ss[1:] {
		if s != out[len(out)-1] {
			out = append(out, s)
		}
	}
	return out
}

// DynamicCandidates 未命中引用的保守候选（防误裁）。
func (p *Pool) DynamicCandidates(ref string) []string {
	r := normalize(ref)
	tokens := map[string]bool{r: true}
	for _, pre := range []string{"ev", "e", "large_", "bg_", "cg_"} {
		if strings.HasPrefix(r, pre) && len(r) > len(pre)+2 {
			tokens[r[len(pre):]] = true
		}
	}
	if m := regexp.MustCompile(`([0-9]+[a-z][0-9a-z_]*)$`).FindStringSubmatch(r); m != nil {
		tokens[m[1]] = true
	}
	var hits []string
	for tok := range tokens {
		if len(tok) < 3 || !hasLetter(tok) {
			continue
		}
		for name, rels := range p.ByName {
			base := strings.TrimSuffix(name, filepath.Ext(name))
			if strings.Contains(base, tok) {
				hits = append(hits, rels...)
			}
		}
	}
	sort.Strings(hits)
	return dedup(hits)
}

func hasLetter(s string) bool {
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			return true
		}
	}
	return false
}

func dedup(ss []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// Result 一次裁剪的结果。
type Result struct {
	Matched   map[string][]string `json:"matched"`   // relpath → 触发它的引用
	Dynamic   map[string][]string `json:"dynamic"`   // relpath → 触发它的未命中引用
	Unmatched []string            `json:"unmatched"` // 未命中且无保守候选的引用
	Emote     map[string]int64    `json:"emote"`     // E-mote 剥出（不复刻，可删）
	PoolBytes int64               `json:"pool_bytes"`
	KeepBytes int64               `json:"keep_bytes"`
}

// EmoteBytes E-mote 剥出总量。
func (r *Result) EmoteBytes() int64 {
	var t int64
	for _, s := range r.Emote {
		t += s
	}
	return t
}

// Prune 对引用列表执行匹配 + 保守保留。
// DR-16：BGM 全部保留 —— 路径含 "bgm" 的池内文件一律计入保留集。
func (p *Pool) Prune(refs []string) *Result {
	res := &Result{
		Matched: map[string][]string{},
		Dynamic: map[string][]string{},
		Emote:   map[string]int64{},
	}
	seen := map[string]bool{}
	for _, ref := range refs {
		if seen[ref] {
			continue
		}
		seen[ref] = true
		if hits := p.Match(ref); len(hits) > 0 {
			for _, h := range hits {
				res.Matched[h] = append(res.Matched[h], ref)
			}
		} else if cands := p.DynamicCandidates(ref); len(cands) > 0 {
			for _, c := range cands {
				res.Dynamic[c] = append(res.Dynamic[c], ref)
			}
		} else {
			res.Unmatched = append(res.Unmatched, ref)
		}
	}
	// DR-16：BGM 全留
	for rel := range p.ByRel {
		if strings.Contains(rel, "bgm") {
			if _, ok := res.Matched[rel]; !ok {
				if _, ok := res.Dynamic[rel]; !ok {
					res.Matched[rel] = append(res.Matched[rel], "<bgm-all>")
				}
			}
		}
	}
	// E-mote 硬边界（FINAL §4.4/§11）：.psb/.psz/.lpb 不复刻 →
	// 从 ④ 剥出单独计为可删量（不参与保留率分母的扣除，仅报告）。
	for rel := range p.ByRel {
		if !isEmote(rel) {
			continue
		}
		if _, ok := res.Matched[rel]; ok {
			delete(res.Matched, rel)
			res.Emote[rel] = p.ByRel[rel].Size
			res.KeepBytes -= p.ByRel[rel].Size
		} else if _, ok := res.Dynamic[rel]; ok {
			delete(res.Dynamic, rel)
			res.Emote[rel] = p.ByRel[rel].Size
			res.KeepBytes -= p.ByRel[rel].Size
		}
	}
	for _, f := range p.ByRel {
		res.PoolBytes += f.Size
	}
	for rel := range res.Matched {
		res.KeepBytes += p.ByRel[rel].Size
	}
	for rel := range res.Dynamic {
		res.KeepBytes += p.ByRel[rel].Size
	}
	sort.Strings(res.Unmatched)
	return res
}

// isEmote E-mote 数据（动态立绘/口型）——按硬边界不复刻。
func isEmote(rel string) bool {
	l := strings.ToLower(rel)
	for _, e := range []string{".psb", ".psz", ".lpb"} {
		if strings.HasSuffix(l, e) {
			return true
		}
	}
	return false
}

// GuardCategory 动态引用守卫：把路径含 key 的全部池内文件加入保留集。
//
// 用途：脚本用动态方式引用某类资源时（如夏空语音文件名由 voiceBase 运行时
// 拼接，静态解析不到任何语音文件），按"宁多勿缺"整类保留，防止误裁。
// 返回新增的文件数。
func (r *Result) GuardCategory(p *Pool, key string) int {
	n := 0
	for rel := range p.ByRel {
		if !strings.Contains(rel, key) {
			continue
		}
		if _, ok := r.Matched[rel]; ok {
			continue
		}
		if _, ok := r.Dynamic[rel]; ok {
			continue
		}
		r.Matched[rel] = []string{"<guard:" + key + ">"}
		r.KeepBytes += p.ByRel[rel].Size
		n++
	}
	return n
}

// KeepRels 返回应保留的全部 relpath（命中 + 保守保留，去重排序）。
func (r *Result) KeepRels() []string {
	set := map[string]bool{}
	for k := range r.Matched {
		set[k] = true
	}
	for k := range r.Dynamic {
		set[k] = true
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// CopyTo 把保留集合复制到目标目录（保持相对路径）。
// 单文件失败跳过并继续（如源文件名含 Win32 非法字符），返回总数与失败数。
func (p *Pool) CopyTo(dst string, rels []string) (int64, int, error) {
	var total int64
	failed := 0
	for _, rel := range rels {
		f, ok := p.ByRel[rel]
		if !ok {
			continue
		}
		target := filepath.Join(dst, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			failed++
			continue
		}
		n, err := copyFile(f.Abs, target)
		if err != nil {
			failed++
			continue
		}
		total += n
	}
	return total, failed, nil
}

// winPath 为 Windows 扩展长度路径（\\?\ 前缀），
// 避免源文件名含 ? 等 Win32 非法字符时打不开（NTFS 本身允许这些字符）。
func winPath(p string) string {
	if runtime.GOOS != "windows" {
		return p
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	if strings.HasPrefix(abs, `\\?\`) {
		return abs
	}
	return `\\?\` + abs
}

func copyFile(src, dst string) (int64, error) {
	in, err := os.Open(winPath(src))
	if err != nil {
		return 0, err
	}
	defer in.Close()
	out, err := os.Create(winPath(dst))
	if err != nil {
		return 0, err
	}
	defer out.Close()
	return ioCopy(out, in)
}

func ioCopy(dst *os.File, src *os.File) (int64, error) {
	buf := make([]byte, 1<<20)
	var total int64
	for {
		n, err := src.Read(buf)
		if n > 0 {
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return total, werr
			}
			total += int64(n)
		}
		if err != nil {
			if err.Error() == "EOF" {
				return total, nil
			}
			return total, err
		}
	}
}

// HumanBytes 人类可读体积。
func HumanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
