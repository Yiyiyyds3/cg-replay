// review-prescreen：CG 导出目录的「机器预筛」。
//
// 目的：人工逐张看几千张图不现实，先用机器把最容易漏的坏图揪出来
// （极小图 / 尺寸离群 / 空白帧 / 重复帧 / 命名断裂 / 体积异常），人工只看可疑处。
//
// 用法：
//
//	go run ./tools/review-prescreen -root "D:\UCGR_Work\unitycg"
//	go run ./tools/review-prescreen -root ... -only BloodOfYidhra,EroTavern
//
// 输出：reports/人工抽检/预筛指标.json、reports/人工抽检/预筛可疑.md
package main

import (
	"bufio"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Params 是本次运行的参数，会原样写进 JSON 便于复现。
type Params struct {
	Root       string  `json:"root,omitempty"`
	MaxSample  int     `json:"max_sample"`
	MaxPixels  int64   `json:"max_pixels"`
	Conc       int     `json:"conc"`
	MinSide    int     `json:"min_side"`
	VarThresh  float64 `json:"var_thresh"`
	SmallKB    float64 `json:"small_kb"`
	OutlierMul float64 `json:"outlier_mul"`
	Seed       int64   `json:"seed"`

	// 源目录模式专用（导出目录模式留空，omitempty 保证 预筛指标.json 形状不变）
	Manifest   string `json:"manifest,omitempty"`
	Engine     string `json:"engine,omitempty"`
	KeepAssets bool   `json:"keep_assets,omitempty"`
	SkipSparse bool   `json:"skip_sparse,omitempty"`
}

// ImgRef 是一张图的元数据。W/H 来自文件头，不解码。
type ImgRef struct {
	Path   string
	Rel    string
	Dir    string
	Name   string
	Ext    string
	Size   int64
	W, H   int
	DimsOK bool
	Broken bool

	// 扩展名与实际内容不符（如 .png 实为 JPEG）
	Misnamed bool

	// 源目录模式：图片族（相对 img 根的前两段）与「是否引擎素材」
	Fam      string
	FamAsset bool
}

// Game 是一款游戏的指标，前 11 个字段是验收要求的固定形状。
type Game struct {
	Name      string  `json:"name"`
	Dir       string  `json:"dir"`
	Engine    string  `json:"engine,omitempty"`
	Images    int     `json:"images"`
	Small     int     `json:"small"`
	Outliers  int     `json:"outliers"`
	Blank     int     `json:"blank"`
	DupGroups int     `json:"dup_groups"`
	DupRate   float64 `json:"dup_rate"`
	Sparse    int     `json:"sparse"`
	AvgKB     float64 `json:"avg_kb"`
	Severity  int     `json:"severity"`

	SeverityReasons []string   `json:"severity_reasons"`
	Verdict         string     `json:"verdict"`
	ModeWH          string     `json:"mode_wh"`
	ModeCount       int        `json:"mode_count"`
	Sampled         int        `json:"sampled"`
	Decoded         int        `json:"decoded"`
	SkippedBig      int        `json:"skipped_big"`
	SkippedWebP     int        `json:"skipped_webp"`
	DecodeErr       int        `json:"decode_err"`
	Broken          int        `json:"broken"`
	TinyFiles       int        `json:"tiny_files"`
	SmallSamples    []string   `json:"small_samples"`
	OutlierSamples  []string   `json:"outlier_samples"`
	OutlierSizes    []string   `json:"outlier_sizes"`
	BlankSamples    []string   `json:"blank_samples"`
	TinySamples     []string   `json:"tiny_samples"`
	DupSampleGroups [][]string `json:"dup_sample_groups"`
	ExactDupGroups  int        `json:"exact_dup_groups"`
	ExactDupRate    float64    `json:"exact_dup_rate"`
	SeqGaps         int        `json:"seq_gaps"`
	SeqBreakDirs    int        `json:"seq_break_dirs"`
	NameConflicts   int        `json:"name_conflicts"`
	SpineFiles      int        `json:"spine_files"`
	ScanMS          int64      `json:"scan_ms"`

	// 源目录模式专用（导出目录模式全部 omitempty 消失，JSON 形状保持不变）
	ManifestDir     string   `json:"manifest_dir,omitempty"`
	ManifestCG      int      `json:"manifest_cg,omitempty"`
	ScanRoots       []string `json:"scan_roots,omitempty"`
	Scanned         int      `json:"scanned,omitempty"`
	AssetImages     int      `json:"asset_images,omitempty"`
	Encrypted       int      `json:"encrypted,omitempty"`
	Families        []Family `json:"families,omitempty"`
	Note            string   `json:"note,omitempty"`
	Misnamed        int      `json:"misnamed,omitempty"`
	MisnamedSamples []string `json:"misnamed_samples,omitempty"`
}

// Summary 是全局分布。
type Summary struct {
	Games          int   `json:"games"`
	ImagesTotal    int   `json:"images_total"`
	SeverityCounts []int `json:"severity_counts"`

	// 源目录模式专用
	Missing        []string `json:"missing,omitempty"`
	MissingCount   int      `json:"missing_count,omitempty"`
	ScannedTotal   int      `json:"scanned_total,omitempty"`
	EncryptedTotal int      `json:"encrypted_total,omitempty"`
	PlainGames     int      `json:"plain_games,omitempty"`
	NoPlainGames   int      `json:"no_plain_games,omitempty"`
}

type Report struct {
	Root        string  `json:"root"`
	Manifest    string  `json:"manifest,omitempty"`
	Engine      string  `json:"engine,omitempty"`
	GeneratedAt string  `json:"generated_at"`
	Params      Params  `json:"params"`
	Summary     Summary `json:"summary"`
	Games       []*Game `json:"games"`
}

var imageExts = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".webp": true}
var spineExts = map[string]bool{".json": true, ".atlas": true, ".skel": true, ".base64": true}

func main() {
	var (
		root       = flag.String("root", "D:\\UCGR_Work\\unitycg", "CG 导出根目录：其下每个子目录是一款游戏，游戏内取 cg/ 子目录（没有则取自身）")
		jsonOut    = flag.String("json", filepath.Join("reports", "人工抽检", "预筛指标.json"), "指标 JSON 输出路径")
		mdOut      = flag.String("md", filepath.Join("reports", "人工抽检", "预筛可疑.md"), "可疑清单 Markdown 输出路径")
		only       = flag.String("only", "", "只跑指定游戏（逗号分隔）")
		maxSample  = flag.Int("max-sample", 300, "单目录最大抽样张数（含首尾各 5 张）")
		maxPixels  = flag.Int64("max-pixels", 9000000, "解码像素上限：超过只读文件头拿尺寸，不解码（900 万 ≈ 4096x2200）")
		conc       = flag.Int("conc", 8, "并发数，上限 8")
		minSide    = flag.Int("min-side", 64, "极小图阈值：任一边 < 该像素数")
		varTh      = flag.Float64("var", 25, "疑似纯色/空帧：抽样灰度方差 < 该值")
		smallKB    = flag.Float64("small-kb", 5, "体积异常：单图 < 该 KB 数")
		outlierMul = flag.Float64("outlier-mul", 1.5, "尺寸离群：偏离 (w,h) 众数超过该倍数")
		seed       = flag.Int64("seed", 20261116, "抽样随机种子")
		quiet      = flag.Bool("quiet", false, "不打印进度")

		manifestPath = flag.String("from-manifest", "", "源目录模式：pass清单.json 路径（给出即走源目录模式，按 source_real_path 扫 img 子树）")
		engineFlag   = flag.String("engine", "", "源目录模式：只跑该引擎的条目（如 rmmz）")
		outDir       = flag.String("out", "", "源目录模式：输出目录（默认 reports/人工抽检），文件名按引擎自动取 预筛指标_<engine>.json / 预筛可疑_<engine>.md")
		keepAssets   = flag.Bool("keep-assets", false, "源目录模式：引擎素材（tilesets/characters/enemies…）也参与打分，默认剔除")
		sparsePen    = flag.Bool("sparse-penalty", false, "源目录模式：散图（一张图独占目录/命名不成系列）也扣分，默认只统计不扣分（rmmz 的 pictures 多为平铺人写标题）")
	)
	flag.Parse()

	c := *conc
	if c < 1 {
		c = 1
	}
	if c > 8 {
		c = 8
	}
	p := Params{
		Root: *root, MaxSample: *maxSample, MaxPixels: *maxPixels, Conc: c,
		MinSide: *minSide, VarThresh: *varTh, SmallKB: *smallKB, OutlierMul: *outlierMul, Seed: *seed,
		Manifest: *manifestPath, Engine: *engineFlag, KeepAssets: *keepAssets,
		SkipSparse: *manifestPath != "" && !*sparsePen,
	}

	// 源目录模式：按清单逐款扫 source_real_path，输出 *_<engine>.json / *_<engine>.md
	if *manifestPath != "" {
		p.Root = "" // 源目录模式的「根」是清单里每款的 source_real_path，不是 -root
		setFlags := map[string]bool{}
		flag.Visit(func(fl *flag.Flag) { setFlags[fl.Name] = true })
		if err := runSource(p, sourceOptions{
			Manifest: *manifestPath, Engine: *engineFlag, Only: *only,
			OutDir: *outDir, JSONOut: *jsonOut, MDOut: *mdOut,
			JSONSet: setFlags["json"], MDSet: setFlags["md"], Quiet: *quiet,
		}); err != nil {
			fmt.Fprintln(os.Stderr, "ERR:", err)
			os.Exit(1)
		}
		return
	}

	rep, err := scan(p, *only, *quiet)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERR:", err)
		os.Exit(1)
	}

	for _, path := range []string{*jsonOut, *mdOut} {
		if d := filepath.Dir(path); d != "" && d != "." {
			if err := os.MkdirAll(d, 0o755); err != nil {
				fmt.Fprintln(os.Stderr, "ERR: 建目录失败", d, err)
				os.Exit(1)
			}
		}
	}
	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERR: marshal", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*jsonOut, append(data, '\n'), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "ERR: 写 JSON", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*mdOut, []byte(renderMarkdown(rep)), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "ERR: 写 Markdown", err)
		os.Exit(1)
	}

	printTable(rep)
	fmt.Printf("\n写出 %s\n写出 %s\n", *jsonOut, *mdOut)
	if rep.Summary.ImagesTotal == 0 {
		os.Exit(2)
	}
}

// ---------------------------------------------------------------------------
// 扫描

func scan(p Params, only string, quiet bool) (*Report, error) {
	st, err := os.Stat(p.Root)
	if err != nil {
		return nil, fmt.Errorf("根目录不可读 %s: %w", p.Root, err)
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("根目录不是目录 %s", p.Root)
	}
	names := map[string]bool{}
	for _, s := range strings.Split(only, ",") {
		if s = strings.TrimSpace(s); s != "" {
			names[strings.ToLower(s)] = true
		}
	}

	entries, err := os.ReadDir(p.Root)
	if err != nil {
		return nil, err
	}
	rep := &Report{
		Root:        p.Root,
		GeneratedAt: time.Now().Format(time.RFC3339),
		Params:      p,
		Summary:     Summary{SeverityCounts: make([]int, 4)},
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if len(names) > 0 && !names[strings.ToLower(name)] {
			continue
		}
		top := filepath.Join(p.Root, name)
		dir := top
		if sub := filepath.Join(top, "cg"); isDir(sub) {
			dir = sub
		}
		refs, spine := collect(dir)
		if len(refs) == 0 {
			if !quiet {
				fmt.Fprintf(os.Stderr, "跳过 %-22s 无图片\n", name)
			}
			continue
		}
		start := time.Now()
		g := analyze(p, name, dir, refs, spine)
		g.ScanMS = time.Since(start).Milliseconds()
		rep.Games = append(rep.Games, g)
		rep.Summary.ImagesTotal += g.Images
		rep.Summary.SeverityCounts[g.Severity]++
		if !quiet {
			fmt.Fprintf(os.Stderr, "%-22s 图 %-5d 极小 %-4d 离群 %-4d 空白 %-3d 重复组 %-3d 冗余 %5.1f%% 散图 %-4d 均 %6.1fKB  sev=%d  (%dms)\n",
				name, g.Images, g.Small, g.Outliers, g.Blank, g.DupGroups, g.DupRate*100, g.Sparse, g.AvgKB, g.Severity, g.ScanMS)
		}
	}
	rep.Summary.Games = len(rep.Games)

	sort.Slice(rep.Games, func(i, j int) bool {
		if rep.Games[i].Severity != rep.Games[j].Severity {
			return rep.Games[i].Severity > rep.Games[j].Severity
		}
		return rep.Games[i].Name < rep.Games[j].Name
	})
	return rep, nil
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// collect 递归收集图片，返回图片列表与 spine 附属文件数。
func collect(root string) ([]*ImgRef, int) {
	var refs []*ImgRef
	spine := 0
	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		if spineExts[ext] {
			spine++
			return nil
		}
		if !imageExts[ext] {
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil {
			return nil
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			rel = d.Name()
		}
		rel = filepath.ToSlash(rel)
		dirRel := filepath.ToSlash(filepath.Dir(rel))
		refs = append(refs, &ImgRef{
			Path: path, Rel: rel, Dir: dirRel, Name: d.Name(), Ext: ext, Size: info.Size(),
		})
		return nil
	})
	sort.Slice(refs, func(i, j int) bool { return refs[i].Rel < refs[j].Rel })
	return refs, spine
}

// ---------------------------------------------------------------------------
// 单款游戏

func analyze(p Params, name, dir string, refs []*ImgRef, spine int) *Game {
	g := &Game{Name: name, Dir: dir, Images: len(refs), SpineFiles: spine}

	// 1) 读文件头拿尺寸（并行，限并发）
	probeAll(refs, p.Conc)

	var totalBytes int64
	broken := 0
	for _, r := range refs {
		totalBytes += r.Size
		if r.Broken {
			broken++
		}
	}
	g.Broken = broken
	if g.Images > 0 {
		g.AvgKB = round1(float64(totalBytes) / float64(g.Images) / 1024)
	}

	// 2) 极小图
	for _, r := range refs {
		if r.DimsOK && (r.W < p.MinSide || r.H < p.MinSide) {
			g.Small++
			pushSample(&g.SmallSamples, r.Rel, 10)
		}
	}

	// 3) 尺寸离群
	g.ModeWH, g.ModeCount = modeSize(refs)
	mw, mh := parseWH(g.ModeWH)
	if mw > 0 && mh > 0 {
		hist := map[string]int{}
		for _, r := range refs {
			if !r.DimsOK {
				continue
			}
			if maxRatio(r.W, r.H, mw, mh) > p.OutlierMul {
				g.Outliers++
				hist[fmt.Sprintf("%dx%d", r.W, r.H)]++
				pushSample(&g.OutlierSamples, fmt.Sprintf("%s (%dx%d)", r.Rel, r.W, r.H), 10)
			}
		}
		g.OutlierSizes = topSizes(hist, 5)
	}

	// 4) 体积异常
	for _, r := range refs {
		if float64(r.Size)/1024 < p.SmallKB {
			g.TinyFiles++
			pushSample(&g.TinySamples, fmt.Sprintf("%s (%.1fKB)", r.Rel, float64(r.Size)/1024), 10)
		}
	}

	// 4.5) 扩展名与内容不符（.png 实为 JPEG 等）：图本身没问题，但导入脚本会踩坑
	for _, r := range refs {
		if r.Misnamed {
			g.Misnamed++
			pushSample(&g.MisnamedSamples, r.Rel, 8)
		}
	}

	// 5) 命名/序列
	g.SeqGaps, g.SeqBreakDirs, g.NameConflicts, g.Sparse = analyzeNames(refs)

	// 6) 抽样解码：方差 + dHash
	sample := pickSample(refs, p.MaxSample, p.Seed)
	g.Sampled = len(sample)
	res := decodeAll(sample, p)

	type key struct {
		w, h int
		hash uint64
	}
	buckets := map[key][]string{}
	exact := map[string][]string{}
	var decoded int
	for i, r := range sample {
		switch res[i].status {
		case "ok":
			decoded++
			if res[i].variance < p.VarThresh {
				g.Blank++
				pushSample(&g.BlankSamples, fmt.Sprintf("%s (var=%.1f)", r.Rel, res[i].variance), 10)
			}
			k := key{r.W, r.H, res[i].hash}
			buckets[k] = append(buckets[k], r.Rel)
			if res[i].sha != "" {
				exact[res[i].sha] = append(exact[res[i].sha], r.Rel)
			}
		case "big":
			g.SkippedBig++
		case "webp":
			g.SkippedWebP++
		default:
			g.DecodeErr++
			pushSample(&g.BlankSamples, r.Rel+" (解码失败)", 10)
		}
	}
	g.Decoded = decoded

	redundant := 0
	var groups [][]string
	for _, files := range buckets {
		if len(files) < 2 {
			continue
		}
		sort.Strings(files)
		groups = append(groups, files)
		redundant += len(files) - 1
	}
	sort.Slice(groups, func(i, j int) bool {
		if len(groups[i]) != len(groups[j]) {
			return len(groups[i]) > len(groups[j])
		}
		return groups[i][0] < groups[j][0]
	})
	g.DupGroups = len(groups)
	if decoded > 0 {
		g.DupRate = round4(float64(redundant) / float64(decoded))
	}

	// 字节级完全相同的分组（SHA-1），用来说明「同图重复」到底有多硬
	exactRedundant := 0
	for _, files := range exact {
		if len(files) < 2 {
			continue
		}
		g.ExactDupGroups++
		exactRedundant += len(files) - 1
	}
	if decoded > 0 {
		g.ExactDupRate = round4(float64(exactRedundant) / float64(decoded))
	}
	for i, grp := range groups {
		if i >= 8 {
			break
		}
		line := grp
		if len(line) > 6 {
			line = append(append([]string{}, line[:6]...), fmt.Sprintf("…等 %d 张", len(grp)))
		}
		g.DupSampleGroups = append(g.DupSampleGroups, line)
	}

	g.Severity, g.SeverityReasons = score(g, p)
	g.Verdict = verdictFor(g.Severity)
	return g
}

// topSizes 把「尺寸 -> 张数」按张数降序压成 "1204x540 x376" 这样的几行。
func topSizes(hist map[string]int, n int) []string {
	type kv struct {
		k string
		v int
	}
	list := make([]kv, 0, len(hist))
	for k, v := range hist {
		list = append(list, kv{k, v})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].v != list[j].v {
			return list[i].v > list[j].v
		}
		return list[i].k < list[j].k
	})
	out := make([]string, 0, n)
	for i, e := range list {
		if i >= n {
			break
		}
		out = append(out, fmt.Sprintf("%s x%d", e.k, e.v))
	}
	return out
}

func pushSample(list *[]string, s string, limit int) {
	if len(*list) < limit {
		*list = append(*list, s)
	}
}

func modeSize(refs []*ImgRef) (string, int) {
	counts := map[string]int{}
	for _, r := range refs {
		if !r.DimsOK {
			continue
		}
		counts[fmt.Sprintf("%dx%d", r.W, r.H)]++
	}
	best, bestN, bestArea := "", 0, 0
	for k, n := range counts {
		w, h := parseWH(k)
		if n > bestN || (n == bestN && w*h > bestArea) {
			best, bestN, bestArea = k, n, w*h
		}
	}
	return best, bestN
}

func parseWH(s string) (int, int) {
	var w, h int
	if _, err := fmt.Sscanf(s, "%dx%d", &w, &h); err != nil {
		return 0, 0
	}
	return w, h
}

func maxRatio(w, h, mw, mh int) float64 {
	r := 0.0
	for _, v := range []float64{
		float64(w) / float64(mw), float64(mw) / float64(w),
		float64(h) / float64(mh), float64(mh) / float64(h),
	} {
		if v > r {
			r = v
		}
	}
	return r
}

// analyzeNames 返回：序列断裂缺口数、有断裂的目录数、跨目录重名冲突数、散图数。
func analyzeNames(refs []*ImgRef) (gaps, breakDirs, conflicts, sparse int) {
	byDir := map[string][]*ImgRef{}
	for _, r := range refs {
		byDir[r.Dir] = append(byDir[r.Dir], r)
	}
	dirs := make([]string, 0, len(byDir))
	for d := range byDir {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)

	for _, d := range dirs {
		list := byDir[d]
		prefixCount := map[string]int{}
		prefixes := map[string][]int{}
		prefixOf := map[*ImgRef]string{}
		for _, r := range list {
			stem := strings.TrimSuffix(r.Name, filepath.Ext(r.Name))
			pre, digits := splitTrailingDigits(stem)
			prefixCount[pre]++
			prefixOf[r] = pre
			if digits != "" {
				n := 0
				for _, ch := range digits {
					n = n*10 + int(ch-'0')
				}
				prefixes[pre] = append(prefixes[pre], n)
			}
		}
		majority, bestN := "", -1
		for pre, n := range prefixCount {
			if n > bestN || (n == bestN && pre < majority) {
				majority, bestN = pre, n
			}
		}
		// 散图：独占目录且文件名与目录名对不上；或目录内多图但前缀与主前缀不同
		for _, r := range list {
			if len(list) == 1 {
				stem := strings.ToLower(strings.TrimSuffix(r.Name, filepath.Ext(r.Name)))
				dirName := strings.ToLower(filepath.Base(d))
				if d == "." {
					dirName = ""
				}
				if stem != dirName {
					sparse++
				}
				continue
			}
			if prefixOf[r] != majority {
				sparse++
			}
		}
		for _, nums := range prefixes {
			if len(nums) < 2 {
				continue
			}
			min, max := nums[0], nums[0]
			seen := map[int]bool{}
			for _, n := range nums {
				seen[n] = true
				if n < min {
					min = n
				}
				if n > max {
					max = n
				}
			}
			g := (max - min + 1) - len(seen)
			if g > 0 {
				gaps += g
				breakDirs++
			}
		}
	}

	byName := map[string]int{}
	for _, r := range refs {
		byName[strings.ToLower(r.Name)]++
	}
	for _, n := range byName {
		if n > 1 {
			conflicts += n - 1
		}
	}
	return
}

func splitTrailingDigits(stem string) (string, string) {
	i := len(stem)
	for i > 0 && stem[i-1] >= '0' && stem[i-1] <= '9' {
		i--
	}
	return stem[:i], stem[i:]
}

// pickSample：首 5 + 尾 5 + 随机补足。
func pickSample(refs []*ImgRef, maxN int, seed int64) []*ImgRef {
	if maxN <= 0 || len(refs) <= maxN {
		return refs
	}
	out := make([]*ImgRef, 0, maxN)
	seen := map[int]bool{}
	add := func(i int) bool {
		if seen[i] || len(out) >= maxN {
			return false
		}
		seen[i] = true
		out = append(out, refs[i])
		return true
	}
	for i := 0; i < 5 && i < len(refs); i++ {
		add(i)
	}
	for i := len(refs) - 1; i >= len(refs)-5 && i >= 0; i-- {
		add(i)
	}
	rnd := rand.New(rand.NewSource(seed))
	tries := 0
	for len(out) < maxN && tries < maxN*200 {
		add(rnd.Intn(len(refs)))
		tries++
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Rel < out[j].Rel })
	return out
}

// ---------------------------------------------------------------------------
// 文件头

func probeAll(refs []*ImgRef, conc int) {
	jobs := make(chan *ImgRef)
	var wg sync.WaitGroup
	for i := 0; i < conc; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for r := range jobs {
				r.W, r.H, r.DimsOK, r.Broken, r.Misnamed = probeOne(r.Path, r.Ext, r.Size)
			}
		}()
	}
	for _, r := range refs {
		jobs <- r
	}
	close(jobs)
	wg.Wait()
}

// probeOne 只读文件头，不解码。
// misnamed 表示「扩展名与实际内容不符」：源目录里 .png 实为 JPEG 这种很常见，
// 图本身是好的，不该算损坏，单独记一笔。
func probeOne(path, ext string, size int64) (w, h int, dimsOK, broken, misnamed bool) {
	if size <= 0 {
		return 0, 0, false, true, false
	}
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, false, true, false
	}
	defer f.Close()

	switch ext {
	case ".png":
		var hdr [33]byte
		if _, err := io.ReadFull(f, hdr[:]); err != nil {
			return 0, 0, false, true, false
		}
		if hdr[0] == 0x89 && string(hdr[1:4]) == "PNG" && string(hdr[12:16]) == "IHDR" {
			w = int(binary.BigEndian.Uint32(hdr[16:20]))
			h = int(binary.BigEndian.Uint32(hdr[20:24]))
			return w, h, w > 0 && h > 0, false, false
		}
		if hdr[0] == 0xFF && hdr[1] == 0xD8 {
			w, h, ok := readJPEGDims(f)
			return w, h, ok, false, true
		}
		return 0, 0, false, true, false

	case ".jpg", ".jpeg":
		if w, h, ok := readJPEGDims(f); ok {
			return w, h, true, false, false
		}
		if w, h, ok := readPNGDims(f); ok {
			return w, h, true, false, true
		}
		return 0, 0, false, true, false

	case ".webp":
		var hdr [40]byte
		if _, err := io.ReadFull(f, hdr[:]); err != nil {
			return 0, 0, false, true, false
		}
		if string(hdr[0:4]) != "RIFF" || string(hdr[8:12]) != "WEBP" {
			return 0, 0, false, true, false
		}
		w, h, ok := webpDims(hdr[:])
		return w, h, ok, false, false
	}
	return 0, 0, false, true, false
}

// readPNGDims 从文件头取 PNG 尺寸（给「扩展名写 .jpg、内容是 PNG」的情况用）。
func readPNGDims(f *os.File) (int, int, bool) {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return 0, 0, false
	}
	var hdr [33]byte
	if _, err := io.ReadFull(f, hdr[:]); err != nil {
		return 0, 0, false
	}
	if hdr[0] != 0x89 || string(hdr[1:4]) != "PNG" || string(hdr[12:16]) != "IHDR" {
		return 0, 0, false
	}
	w := int(binary.BigEndian.Uint32(hdr[16:20]))
	h := int(binary.BigEndian.Uint32(hdr[20:24]))
	return w, h, w > 0 && h > 0
}

func jpegSignature(f *os.File) bool {
	var b [2]byte
	if _, err := f.ReadAt(b[:], 0); err != nil {
		return false
	}
	return b[0] == 0xFF && b[1] == 0xD8
}

func readJPEGDims(f *os.File) (w, h int, ok bool) {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return 0, 0, false
	}
	var b [2]byte
	if _, err := io.ReadFull(f, b[:]); err != nil || b[0] != 0xFF || b[1] != 0xD8 {
		return 0, 0, false
	}
	for {
		if _, err := io.ReadFull(f, b[:]); err != nil {
			return 0, 0, false
		}
		if b[0] != 0xFF {
			continue // 重新对齐到标记
		}
		m := b[1]
		if m == 0xFF {
			if _, err := f.Seek(-1, io.SeekCurrent); err != nil {
				return 0, 0, false
			}
			continue
		}
		if m == 0xD8 || m == 0x01 || (m >= 0xD0 && m <= 0xD7) {
			continue // 独立标记，无长度
		}
		if m == 0xD9 || m == 0xDA {
			return 0, 0, false
		}
		var lb [2]byte
		if _, err := io.ReadFull(f, lb[:]); err != nil {
			return 0, 0, false
		}
		segLen := int(binary.BigEndian.Uint16(lb[:]))
		if segLen < 2 {
			return 0, 0, false
		}
		isSOF := (m >= 0xC0 && m <= 0xC3) || (m >= 0xC5 && m <= 0xC7) ||
			(m >= 0xC9 && m <= 0xCB) || (m >= 0xCD && m <= 0xCF)
		if isSOF {
			var sof [5]byte
			if _, err := io.ReadFull(f, sof[:]); err != nil {
				return 0, 0, false
			}
			h = int(binary.BigEndian.Uint16(sof[1:3]))
			w = int(binary.BigEndian.Uint16(sof[3:5]))
			return w, h, w > 0 && h > 0
		}
		if _, err := f.Seek(int64(segLen-2), io.SeekCurrent); err != nil {
			return 0, 0, false
		}
	}
}

func webpDims(hdr []byte) (int, int, bool) {
	if len(hdr) < 40 {
		return 0, 0, false
	}
	chunk := string(hdr[12:16])
	switch chunk {
	case "VP8X":
		w := int(uint32(hdr[24])|uint32(hdr[25])<<8|uint32(hdr[26])<<16) + 1
		h := int(uint32(hdr[27])|uint32(hdr[28])<<8|uint32(hdr[29])<<16) + 1
		return w, h, true
	case "VP8 ":
		if hdr[23] != 0x9D || hdr[24] != 0x01 || hdr[25] != 0x2A {
			return 0, 0, false
		}
		w := int(binary.LittleEndian.Uint16(hdr[26:28]) & 0x3FFF)
		h := int(binary.LittleEndian.Uint16(hdr[28:30]) & 0x3FFF)
		return w, h, w > 0 && h > 0
	case "VP8L":
		if hdr[20] != 0x2F {
			return 0, 0, false
		}
		bits := uint32(hdr[21]) | uint32(hdr[22])<<8 | uint32(hdr[23])<<16 | uint32(hdr[24])<<24
		w := int(bits&0x3FFF) + 1
		h := int((bits>>14)&0x3FFF) + 1
		return w, h, true
	}
	return 0, 0, false
}

// ---------------------------------------------------------------------------
// 解码抽样：方差 + 8x8 dHash

type decResult struct {
	status   string // ok / big / webp / err
	variance float64
	hash     uint64
	sha      string // 文件字节的 SHA-1，用来区分「完全同一文件」与「近似重复」
	err      error
}

func decodeAll(sample []*ImgRef, p Params) []decResult {
	out := make([]decResult, len(sample))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for i := 0; i < p.Conc; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				out[idx] = decodeOne(sample[idx], p)
			}
		}()
	}
	for i := range sample {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	return out
}

func decodeOne(r *ImgRef, p Params) decResult {
	if r.Ext == ".webp" {
		return decResult{status: "webp"}
	}
	if r.DimsOK && int64(r.W)*int64(r.H) > p.MaxPixels {
		return decResult{status: "big"}
	}
	sha, herr := hashFile(r.Path)
	if herr != nil {
		return decResult{status: "err", err: herr}
	}
	f, err := os.Open(r.Path)
	if err != nil {
		return decResult{status: "err", err: err}
	}
	defer f.Close()
	img, _, err := image.Decode(bufio.NewReaderSize(f, 1<<16))
	if err != nil {
		return decResult{status: "err", err: err}
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w < 2 || h < 2 {
		return decResult{status: "ok", variance: 0, hash: 0, sha: sha}
	}
	if !r.DimsOK {
		r.W, r.H, r.DimsOK = w, h, true
	}
	return decResult{status: "ok", variance: grayVariance(img, b, 32), hash: dhash(img, b), sha: sha}
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha1.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// grayAt 取灰度：先把 alpha 按白底合成，避免「全透明被当成全黑」
// 把 alpha 图的差异抹平（spine 图集大量透明边）。
func grayAt(img image.Image, b image.Rectangle, x, y int) float64 {
	c := color.NRGBAModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
	a := float64(c.A) / 255
	return (0.299*float64(c.R)+0.587*float64(c.G)+0.114*float64(c.B))*a + 255*(1-a)
}

// grayVariance 在 32x32 网格上抽样算像素方差（不逐像素遍历整图）。
func grayVariance(img image.Image, b image.Rectangle, grid int) float64 {
	w, h := b.Dx(), b.Dy()
	vals := make([]float64, 0, grid*grid)
	sum := 0.0
	for j := 0; j < grid; j++ {
		y := j * (h - 1) / (grid - 1)
		for i := 0; i < grid; i++ {
			x := i * (w - 1) / (grid - 1)
			v := grayAt(img, b, x, y)
			vals = append(vals, v)
			sum += v
		}
	}
	mean := sum / float64(len(vals))
	acc := 0.0
	for _, v := range vals {
		d := v - mean
		acc += d * d
	}
	return acc / float64(len(vals))
}

// dhash：9x8 灰度网格，相邻列比较得 64 bit。
func dhash(img image.Image, b image.Rectangle) uint64 {
	var g [8][9]float64
	w, h := b.Dx(), b.Dy()
	for j := 0; j < 8; j++ {
		y := j * (h - 1) / 7
		for i := 0; i < 9; i++ {
			x := i * (w - 1) / 8
			g[j][i] = grayAt(img, b, x, y)
		}
	}
	var hash uint64
	for j := 0; j < 8; j++ {
		for i := 0; i < 8; i++ {
			hash <<= 1
			if g[j][i] > g[j][i+1] {
				hash |= 1
			}
		}
	}
	return hash
}

// ---------------------------------------------------------------------------
// 评分

func score(g *Game, p Params) (int, []string) {
	total, reasons := scorePoints(g, p)
	return sevBand(total), reasons
}

// sevBand 把加权总分映射到 0-3 档（0 无异常 / 1 轻微 / 2 需看 / 3 建议全量肉眼）。
func sevBand(total int) int {
	switch {
	case total >= 5:
		return 3
	case total >= 3:
		return 2
	case total >= 1:
		return 1
	}
	return 0
}

// verdictFor 把 severity 翻成一句人话结论。
func verdictFor(sev int) string {
	switch sev {
	case 3:
		return "建议全量肉眼"
	case 1, 2:
		return "抽检 30 张即可"
	}
	return "无需看"
}

// scorePoints 是各判据加权求和本身，返回总分与逐条理由。
func scorePoints(g *Game, p Params) (int, []string) {
	total := 0
	var reasons []string
	add := func(n int, why string) {
		total += n
		reasons = append(reasons, fmt.Sprintf("%s (+%d)", why, n))
	}
	pct := func(n, d int) float64 {
		if d == 0 {
			return 0
		}
		return float64(n) / float64(d)
	}

	if g.Broken > 0 {
		add(3, fmt.Sprintf("文件头损坏 %d 张", g.Broken))
	}
	if g.Misnamed > 0 {
		add(1, fmt.Sprintf("扩展名与实际格式不符 %d/%d 张", g.Misnamed, g.Images))
	}
	if g.Decoded > 0 {
		br := pct(g.Blank, g.Decoded)
		switch {
		case br > 0.20:
			add(3, fmt.Sprintf("空白帧 %d/%d (%.0f%%)", g.Blank, g.Decoded, br*100))
		case br > 0.05:
			add(2, fmt.Sprintf("空白帧 %d/%d (%.0f%%)", g.Blank, g.Decoded, br*100))
		case g.Blank > 0:
			add(1, fmt.Sprintf("空白帧 %d/%d (%.1f%%)", g.Blank, g.Decoded, br*100))
		}
	}
	if g.DecodeErr > 0 {
		add(2, fmt.Sprintf("抽样解码失败 %d 张", g.DecodeErr))
	}
	if r := pct(g.Small, g.Images); r > 0 {
		if r > 0.30 {
			add(2, fmt.Sprintf("极小图 %d/%d (%.0f%%)", g.Small, g.Images, r*100))
		} else {
			add(1, fmt.Sprintf("极小图 %d/%d (%.1f%%)", g.Small, g.Images, r*100))
		}
	}
	// 众数占主导（>=50% 图片）时「离群」才有判别力；否则尺寸本来就离散
	if r := pct(g.Outliers, g.Images); r > 0 {
		dom := pct(g.ModeCount, g.Images)
		switch {
		case dom < 0.50:
			add(1, fmt.Sprintf("尺寸离散（众数 %s 仅占 %.0f%%，离群 %d/%d）", g.ModeWH, dom*100, g.Outliers, g.Images))
		case r > 0.20:
			add(2, fmt.Sprintf("尺寸离群 %d/%d (%.0f%%)", g.Outliers, g.Images, r*100))
		default:
			add(1, fmt.Sprintf("尺寸离群 %d/%d (%.1f%%)", g.Outliers, g.Images, r*100))
		}
	}
	switch {
	case g.DupRate > 0.60:
		add(2, fmt.Sprintf("重复冗余率 %.1f%%（近似重复 %d 组）", g.DupRate*100, g.DupGroups))
	case g.DupRate > 0.35:
		add(1, fmt.Sprintf("重复冗余率 %.1f%%（近似重复 %d 组）", g.DupRate*100, g.DupGroups))
	case g.DupGroups > 0 && g.DupRate > 0.10:
		add(1, fmt.Sprintf("重复帧 %d 组，冗余率 %.1f%%", g.DupGroups, g.DupRate*100))
	}
	if r := pct(g.TinyFiles, g.Images); r > 0 {
		if r > 0.20 {
			add(2, fmt.Sprintf("<%gKB 小文件 %d/%d (%.0f%%)", p.SmallKB, g.TinyFiles, g.Images, r*100))
		} else if r > 0.02 {
			add(1, fmt.Sprintf("<%gKB 小文件 %d/%d (%.1f%%)", p.SmallKB, g.TinyFiles, g.Images, r*100))
		}
	}
	if g.SeqGaps > 0 {
		n := 1
		if g.SeqGaps > 10 {
			n = 2
		}
		add(n, fmt.Sprintf("序列断裂 %d 处 / %d 个目录", g.SeqGaps, g.SeqBreakDirs))
	}
	if !p.SkipSparse {
		if r := pct(g.Sparse, g.Images); r > 0.80 {
			add(1, fmt.Sprintf("散图 %d/%d (%.0f%%)", g.Sparse, g.Images, r*100))
		}
	}
	if g.AvgKB > 0 && g.AvgKB < 30 {
		add(1, fmt.Sprintf("均单图仅 %.1fKB", g.AvgKB))
	}

	if len(reasons) == 0 {
		reasons = []string{"无明显异常"}
	}
	return total, reasons
}

// ---------------------------------------------------------------------------
// 输出

func printTable(rep *Report) {
	fmt.Printf("根目录 %s\n", rep.Root)
	fmt.Printf("%-22s %6s %5s %5s %5s %6s %7s %6s %8s %4s\n",
		"游戏", "图片", "极小", "离群", "空白", "重复组", "冗余率", "散图", "均KB", "sev")
	for _, g := range rep.Games {
		fmt.Printf("%-22s %6d %5d %5d %5d %6d %6.1f%% %6d %8.1f %4d\n",
			g.Name, g.Images, g.Small, g.Outliers, g.Blank, g.DupGroups, g.DupRate*100, g.Sparse, g.AvgKB, g.Severity)
	}
	fmt.Printf("\nseverity 分布：0=%d 1=%d 2=%d 3=%d\n",
		rep.Summary.SeverityCounts[0], rep.Summary.SeverityCounts[1],
		rep.Summary.SeverityCounts[2], rep.Summary.SeverityCounts[3])
}

func renderMarkdown(rep *Report) string {
	var sb strings.Builder
	sb.WriteString("# 机器预筛 · 可疑清单\n\n")
	sb.WriteString("由 `tools/review-prescreen` 生成，所有数字来自实跑。\n\n")
	fmt.Fprintf(&sb, "- 扫描根目录：%s\n", rep.Root)
	fmt.Fprintf(&sb, "- 生成时间：%s\n", rep.GeneratedAt)
	fmt.Fprintf(&sb, "- 游戏数：%d，图片总数：%d\n", rep.Summary.Games, rep.Summary.ImagesTotal)
	fmt.Fprintf(&sb, "- 抽样：单目录最多 %d 张（首 5 + 尾 5 + 随机补足），解码像素上限 %d（超过只读头），并发 %d\n",
		rep.Params.MaxSample, rep.Params.MaxPixels, rep.Params.Conc)
	fmt.Fprintf(&sb, "- 阈值：极小图 任一边<%dpx；离群 偏离众数>%g倍；空白 抽样灰度方差<%g；重复 同尺寸+8x8 dHash；体积 <%gKB\n\n",
		rep.Params.MinSide, rep.Params.OutlierMul, rep.Params.VarThresh, rep.Params.SmallKB)

	fmt.Fprintf(&sb, "severity 分布：0=%d，1=%d，2=%d，3=%d\n\n",
		rep.Summary.SeverityCounts[0], rep.Summary.SeverityCounts[1],
		rep.Summary.SeverityCounts[2], rep.Summary.SeverityCounts[3])

	sb.WriteString("## 总览（按 severity 降序）\n\n")
	sb.WriteString("| sev | 游戏 | 图片 | 极小 | 离群 | 空白 | 近似重复组 | 冗余率 | 完全重复组 | 散图 | 均KB | 结论 |\n")
	sb.WriteString("| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |\n")
	for _, g := range rep.Games {
		fmt.Fprintf(&sb, "| %d | %s | %d | %d | %d | %d | %d | %.1f%% | %d | %d | %.1f | %s |\n",
			g.Severity, g.Name, g.Images, g.Small, g.Outliers, g.Blank, g.DupGroups,
			g.DupRate*100, g.ExactDupGroups, g.Sparse, g.AvgKB, g.Verdict)
	}

	sb.WriteString("\n## 每款一条结论\n")
	for _, g := range rep.Games {
		fmt.Fprintf(&sb, "\n### %s — severity %d — %s\n", g.Name, g.Severity, g.Verdict)
		fmt.Fprintf(&sb, "- 目录：%s\n", g.Dir)
		fmt.Fprintf(&sb, "- 图像 %d 张（spine 附属 %d 个），众数尺寸 %s（%d 张），均单图 %.1fKB\n",
			g.Images, g.SpineFiles, g.ModeWH, g.ModeCount, g.AvgKB)
		fmt.Fprintf(&sb, "- 抽样 %d 张，成功解码 %d（跳过大图 %d、webp %d、解码失败 %d）\n",
			g.Sampled, g.Decoded, g.SkippedBig, g.SkippedWebP, g.DecodeErr)
		fmt.Fprintf(&sb, "- 判据：%s\n", strings.Join(g.SeverityReasons, "；"))
		fmt.Fprintf(&sb, "- 重复：近似重复（8x8 dHash 同桶）%d 组、冗余率 %.1f%%；其中字节完全相同（SHA-1）%d 组、冗余率 %.1f%%\n",
			g.DupGroups, g.DupRate*100, g.ExactDupGroups, g.ExactDupRate*100)
		switch g.Verdict {
		case "建议全量肉眼":
			sb.WriteString("- 结论：**建议全量肉眼**（可疑文件名见下方 severity=3 段）\n")
		case "无需看":
			sb.WriteString("- 结论：**无需看**，机器未发现可疑点\n")
		default:
			sb.WriteString("- 结论：**抽检 30 张即可**，优先看下面列出的样本\n")
		}
		if len(g.BlankSamples) > 0 {
			sb.WriteString("- 空白/解码异常样本：" + strings.Join(quoteAll(g.BlankSamples), "、") + "\n")
		}
		if len(g.SmallSamples) > 0 {
			sb.WriteString("- 极小图样本：" + strings.Join(quoteAll(g.SmallSamples), "、") + "\n")
		}
		if len(g.OutlierSizes) > 0 {
			fmt.Fprintf(&sb, "- 离群尺寸分布（共 %d 张）：%s\n", g.Outliers, strings.Join(g.OutlierSizes, "、"))
		}
		if len(g.OutlierSamples) > 0 {
			sb.WriteString("- 离群样本：" + strings.Join(quoteAll(g.OutlierSamples), "、") + "\n")
		}
	}

	sb.WriteString("\n## severity=3 具体文件\n")
	any := false
	for _, g := range rep.Games {
		if g.Severity != 3 {
			continue
		}
		any = true
		fmt.Fprintf(&sb, "\n### %s（共 %d 张）\n", g.Name, g.Images)
		if len(g.BlankSamples) > 0 {
			fmt.Fprintf(&sb, "- 空白/解码异常（共 %d 张，列举前 %d）：\n", g.Blank, len(g.BlankSamples))
			for _, s := range g.BlankSamples {
				sb.WriteString("  - " + s + "\n")
			}
		}
		if len(g.DupSampleGroups) > 0 {
			fmt.Fprintf(&sb, "- 近似重复组（共 %d 组，列举前 %d 组）：\n", g.DupGroups, len(g.DupSampleGroups))
			for _, grp := range g.DupSampleGroups {
				sb.WriteString("  - " + strings.Join(quoteAll(grp), " = ") + "\n")
			}
		}
		if len(g.OutlierSizes) > 0 {
			fmt.Fprintf(&sb, "- 尺寸离群（共 %d 张），尺寸分布：%s\n", g.Outliers, strings.Join(g.OutlierSizes, "、"))
			for _, s := range g.OutlierSamples {
				sb.WriteString("  - " + s + "\n")
			}
		}
		if len(g.SmallSamples) > 0 {
			fmt.Fprintf(&sb, "- 极小图（共 %d 张，列举前 %d）：\n", g.Small, len(g.SmallSamples))
			for _, s := range g.SmallSamples {
				sb.WriteString("  - " + s + "\n")
			}
		}
		if len(g.TinySamples) > 0 {
			fmt.Fprintf(&sb, "- 体积异常（共 %d 张，列举前 %d）：\n", g.TinyFiles, len(g.TinySamples))
			for _, s := range g.TinySamples {
				sb.WriteString("  - " + s + "\n")
			}
		}
	}
	if !any {
		sb.WriteString("\n本次无 severity=3 的游戏。\n")
	}

	sb.WriteString("\n## 指标口径\n")
	sb.WriteString("- images：目录内 png/jpg/jpeg/webp 总数（spine 的 .json/.atlas/.skel 不计入）。\n")
	sb.WriteString("- small：文件头读出任一边 < 64px 的张数。\n")
	sb.WriteString("- outliers：以 (w,h) 精确众数为基准，宽或高任一方偏离 > 1.5 倍（放大或缩小都算）。\n")
	sb.WriteString("- blank：抽样解码后 32x32 网格灰度方差 < 25 的张数（纯色/近纯色/空帧）。\n")
	sb.WriteString("- dup_groups / dup_rate：抽样解码张数内，同 (w,h) 且 8x8 灰度 dHash（alpha 按白底合成后取灰）完全相同的分桶中，桶内 >=2 张记一组；dup_rate = 冗余张数(每桶 n-1 累加) / 解码张数。这是**近似重复**：同一模板改几个字也会撞桶。\n")
	sb.WriteString("- exact_dup_groups / exact_dup_rate：同样口径，但用文件字节 SHA-1 判等，只有真正同一份文件才算。两个数一起看：近似多、完全少 = 模板复用；两个都高 = 纯冗余。\n")
	sb.WriteString("- sparse：所在目录只有 1 张图，或该图前缀不是本目录多数前缀的张数。\n")
	sb.WriteString("- avg_kb：目录总字节 / 图片数 / 1024。\n")
	sb.WriteString("- severity：各判据加权求和后分档（0 无异常 / 1 轻微 / 2 需看 / 3 建议全量肉眼），权重见 README。\n")
	return sb.String()
}

func quoteAll(list []string) []string {
	out := make([]string, 0, len(list))
	for _, s := range list {
		out = append(out, "`"+s+"`")
	}
	return out
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }
func round4(v float64) float64 { return math.Round(v*10000) / 10000 }
