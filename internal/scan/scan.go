// Package scan 提供大规模目录体积盘点（S0）能力。
//
// 对应上游 FINAL 1017 §7.5.2：按扩展名分类统计体积，回答
// 「CG 图片 vs 非 CG 图片比例 / 动态立绘占比 / 语音总量 / OP-ED 视频占比」。
// 208 万文件量级：filepath.WalkDir 产路径、goroutine worker pool 并发 stat。
package scan

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// Category 资源分类（上游 §7.5.2 的七类 + 归档/其它）。
type Category string

const (
	CatImage   Category = "图片"             // tlg/png/jpg/hg3/sgcg/bip 等
	CatAudio   Category = "音频"             // ogg/wav/mp3
	CatScript  Category = "脚本"             // ks/tjs/ast/cst/lua/bs/hcb/scn
	CatAnim    Category = "动画/动态立绘/口型" // anm/psb/psz/lpb/amv/sli
	CatVideo   Category = "视频"             // mpg/avi/webm/wmv/ogv/mp4
	CatFont    Category = "字体"             // ttf/ttc/otf
	CatArchive Category = "归档"             // xp3/pfs/int/arc/nsa/alf/afs/pac/npa
	CatOther   Category = "其它/系统"
)

// extCategory 扩展名 → 分类。覆盖本项目已见的全部引擎格式。
var extCategory = map[string]Category{
	// 图片
	".tlg": CatImage, ".png": CatImage, ".jpg": CatImage, ".jpeg": CatImage,
	".bmp": CatImage, ".hg3": CatImage, ".hg2": CatImage, ".sgcg": CatImage,
	".bip": CatImage, ".t2p": CatImage, ".pgd": CatImage, ".qnt": CatImage,
	".hzc": CatImage, ".pactex": CatImage, ".dif": CatImage, ".pfl": CatImage,
	// 音频
	".ogg": CatAudio, ".wav": CatAudio, ".mp3": CatAudio, ".adx": CatAudio,
	// 脚本
	".ks": CatScript, ".tjs": CatScript, ".asd": CatScript, ".ast": CatScript,
	".cst": CatScript, ".lua": CatScript, ".bs": CatScript, ".hcb": CatScript,
	".scn": CatScript, ".sli": CatScript, ".func": CatScript, ".sinfo": CatScript,
	".ma": CatScript, ".nss": CatScript, ".ksc": CatScript,
	// 动画/动态立绘/口型
	".anm": CatAnim, ".psb": CatAnim, ".psz": CatAnim, ".lpb": CatAnim,
	".amv": CatAnim, ".kcs": CatAnim, ".kx2": CatAnim, ".fes": CatAnim,
	".mot": CatAnim, ".ptcl": CatAnim, ".zt": CatAnim,
	// 视频
	".mpg": CatVideo, ".mpeg": CatVideo, ".avi": CatVideo, ".webm": CatVideo,
	".wmv": CatVideo, ".ogv": CatVideo, ".mp4": CatVideo, ".asf": CatVideo,
	// 字体
	".ttf": CatFont, ".ttc": CatFont, ".otf": CatFont,
	// 归档
	".xp3": CatArchive, ".pfs": CatArchive, ".int": CatArchive, ".kif": CatArchive,
	".arc": CatArchive, ".nsa": CatArchive, ".alf": CatArchive, ".afs": CatArchive,
	".pac": CatArchive, ".npa": CatArchive, ".afa": CatArchive, ".lpk": CatArchive,
	".pck": CatArchive, ".paz": CatArchive, ".dat": CatArchive, ".bin": CatArchive,
}

// Classify 按扩展名分类；无扩展名或未收录 → 其它/系统。
func Classify(path string) Category {
	ext := filepath.Ext(path)
	if ext == "" {
		return CatOther
	}
	if c, ok := extCategory[ext]; ok {
		return c
	}
	return CatOther
}

// Stat 单个分类的统计。
type Stat struct {
	Files int64
	Bytes int64
}

// Result 一次盘点的结果。
type Result struct {
	Root       string             `json:"root"`
	Total      Stat               `json:"total"`
	Categories map[Category]Stat  `json:"categories"`
	Errors     int64              `json:"errors"` // stat 失败计数
}

// NewResult 构造空结果。
func NewResult(root string) *Result {
	return &Result{
		Root:       root,
		Categories: map[Category]Stat{},
	}
}

// Add 累加一个文件。
func (r *Result) Add(cat Category, size int64) {
	r.Total.Files++
	r.Total.Bytes += size
	s := r.Categories[cat]
	s.Files++
	s.Bytes += size
	r.Categories[cat] = s
}

// SortedCategories 按体积降序返回分类，便于报告。
func (r *Result) SortedCategories() []Category {
	cats := make([]Category, 0, len(r.Categories))
	for c := range r.Categories {
		cats = append(cats, c)
	}
	sort.Slice(cats, func(i, j int) bool {
		return r.Categories[cats[i]].Bytes > r.Categories[cats[j]].Bytes
	})
	return cats
}

// ScanDir 并发遍历 root 统计体积。workers ≤ 0 时用 GOMAXPROCS。
//
// 流程：WalkDir 在主 goroutine 产文件路径 → channel → worker pool 并发
// stat 累加（结果通过 channel 汇回主 goroutine，避免锁）。
func ScanDir(root string, workers int) (*Result, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		// 单文件：直接统计
		res := NewResult(root)
		res.Add(Classify(root), info.Size())
		return res, nil
	}
	if workers <= 0 {
		workers = 8
	}

	type fileStat struct {
		cat  Category
		size int64
		ok   bool
	}
	paths := make(chan string, 1024)
	stats := make(chan fileStat, 1024)

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range paths {
				fi, err := os.Stat(p)
				if err != nil {
					stats <- fileStat{ok: false}
					continue
				}
				stats <- fileStat{cat: Classify(p), size: fi.Size(), ok: true}
			}
		}()
	}
	go func() {
		wg.Wait()
		close(stats)
	}()

	res := NewResult(root)
	walkDone := make(chan error, 1)
	go func() {
		walkDone <- filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				res.Errors++
				return nil // 跳过不可访问项，不中断盘点
			}
			if d.Type().IsRegular() {
				paths <- p
			}
			return nil
		})
		close(paths)
	}()

	for s := range stats {
		if s.ok {
			res.Add(s.cat, s.size)
		} else {
			res.Errors++
		}
	}
	if err := <-walkDone; err != nil {
		return nil, err
	}
	return res, nil
}

// FormatReport 生成文本报告（体积降序 + 占比）。
func FormatReport(r *Result) string {
	var b []byte
	b = append(b, fmt.Sprintf("盘点目录: %s\n", r.Root)...)
	b = append(b, fmt.Sprintf("总文件数: %d    总体积: %s    错误: %d\n",
		r.Total.Files, HumanBytes(r.Total.Bytes), r.Errors)...)
	b = append(b, "分类统计（按体积降序）:\n"...)
	for _, c := range r.SortedCategories() {
		s := r.Categories[c]
		pct := 0.0
		if r.Total.Bytes > 0 {
			pct = float64(s.Bytes) / float64(r.Total.Bytes) * 100
		}
		b = append(b, fmt.Sprintf("  %-22s %10s %8d 个 %6.1f%%\n",
			c, HumanBytes(s.Bytes), s.Files, pct)...)
	}
	return string(b)
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
