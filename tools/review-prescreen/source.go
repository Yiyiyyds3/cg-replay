// 源目录模式（-from-manifest）：给「没有导出目录、图就在游戏源目录里」的款用。
//
// rmmz 的 37 款 pass 就是这种：图散在 www/img、img 子树里（pictures/、pictures/busts/ …），
// 没有统一的导出产物。这个模式按清单逐款进 source_real_path，递归找 png/jpg/webp，
// 沿用导出目录模式的全部指标，并额外统计「图片族分布」与加密图数量。
//
// 用法：
//
//	go run ./tools/review-prescreen -from-manifest reports/人工抽检/pass清单.json -engine rmmz
//	go run ./tools/review-prescreen -from-manifest reports/人工抽检/pass清单.json -engine rmmz -out reports/人工抽检
//	go run ./tools/review-prescreen -from-manifest ... -engine rmmz -only "气息（上等）"
//
// 输出与 预筛指标.json 同构，另加 engine / families / scan_roots / scanned / encrypted 等字段。
package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Family 是一个「图片族」的统计：img 子树下前两段路径 + 张数 + 是否引擎素材。
type Family struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
	Kind  string `json:"kind"` // cg=参与打分；asset=引擎素材，默认不参与打分
}

// ManifestEntry 只取 pass清单.json 里用得上的字段。
type ManifestEntry struct {
	ID             int    `json:"id"`
	Name           string `json:"name"`
	Engine         string `json:"engine"`
	Status         string `json:"status"`
	CGCount        int    `json:"cg_count"`
	SourceDir      string `json:"source_dir"`
	SourceRealPath string `json:"source_real_path"`
	SourceExists   bool   `json:"source_dir_exists"`
}

// srcScan 是一款游戏扫完 img 子树的中间结果。
type srcScan struct {
	Refs      []*ImgRef // 参与打分的图（引擎素材已剔除）
	All       int       // 扫到的 png/jpg/webp 总数
	Encrypted int       // .rpgmvp / .png_ 加密图数量
	AssetRefs int       // 被当成引擎素材剔除的张数
	Families  []Family
	Roots     []string // 实际扫的 img 根（相对游戏目录）
	Note      string
}

// encImageExts 是 RPG Maker 加密图的扩展名：读得到文件名、读不到像素。
var encImageExts = map[string]bool{".rpgmvp": true, ".png_": true}

// assetFamilies 是引擎自带素材目录（RPG Maker 默认 img 结构 + 常见插件目录）。
// 这些不是 CG，混进指标会把「重复/离群」冲成噪声，默认剔除（-keep-assets 可关）。
var assetFamilies = map[string]bool{
	"tilesets": true, "tileset": true, "characters": true, "faces": true,
	"enemies": true, "sv_enemies": true, "sv_actors": true,
	"battlebacks1": true, "battlebacks2": true, "parallaxes": true,
	"animations": true, "titles1": true, "titles2": true, "system": true,
	"transitions": true, "fogs": true, "fog": true, "effects": true,
	"windowskins": true, "sprites": true, "heros": true, "movies": true,
	"icon": true, "icons": true, "eventindicators": true, "particles": true,
	"layers": true, "ui": true, "hud": true, "font": true, "fonts": true,
	"spines": true, "spine": true, "battlers": true, "summons": true,
}

// assetTokens 出现在族名任意一段里就按素材算（按 ASCII 词边界匹配，避免 rui/yui 这类人名误伤）。
// 例子：pictures/ホーム画面UI達、pictures/露出シーンUI関係 → 全是 UI 图，不该进 CG 指标。
var assetTokens = []string{"ui", "icon", "hud", "menu", "logo", "font", "button", "window", "cursor", "frame"}

func isWordByte(b byte) bool { return b >= 'a' && b <= 'z' || b >= '0' && b <= '9' }

func hasAssetToken(fam string) bool {
	for _, seg := range strings.Split(strings.ToLower(fam), "/") {
		for _, tok := range assetTokens {
			for i := 0; i+len(tok) <= len(seg); i++ {
				if seg[i:i+len(tok)] != tok {
					continue
				}
				if i > 0 && isWordByte(seg[i-1]) {
					continue
				}
				if j := i + len(tok); j < len(seg) && isWordByte(seg[j]) {
					continue
				}
				return true
			}
		}
	}
	return false
}

var assetPrefixes = []string{"special__", "menu__", "map__", "battle__", "system__", "ui__", "hud__"}

func isAssetFamily(fam string) bool {
	first := strings.ToLower(strings.SplitN(fam, "/", 2)[0])
	if assetFamilies[first] {
		return true
	}
	for _, p := range assetPrefixes {
		if strings.HasPrefix(first, p) {
			return true
		}
	}
	return hasAssetToken(fam)
}

func loadManifest(path, engine, only string) ([]ManifestEntry, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读清单失败 %s: %w", path, err)
	}
	var all []ManifestEntry
	if err := json.Unmarshal(raw, &all); err != nil {
		return nil, fmt.Errorf("解析清单失败 %s: %w", path, err)
	}
	eng := strings.ToLower(strings.TrimSpace(engine))
	names := map[string]bool{}
	for _, s := range strings.Split(only, ",") {
		if s = strings.TrimSpace(s); s != "" {
			names[strings.ToLower(s)] = true
		}
	}
	var out []ManifestEntry
	for _, e := range all {
		if eng != "" && strings.ToLower(e.Engine) != eng {
			continue
		}
		if len(names) > 0 && !names[strings.ToLower(e.Name)] {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

// findScanRoots 在游戏目录下递归找名为 img 的目录（不分大小写），父目录命中后不再往下钻。
func findScanRoots(gameDir string) []string {
	var all []string
	filepath.WalkDir(gameDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		if strings.EqualFold(d.Name(), "img") {
			all = append(all, p)
			return filepath.SkipDir
		}
		return nil
	})
	// 只留最外层：嵌在别的 img 里的 img 不重复算
	var kept []string
	for _, p := range all {
		nested := false
		for _, k := range kept {
			if strings.HasPrefix(strings.ToLower(p), strings.ToLower(k)+string(os.PathSeparator)) {
				nested = true
				break
			}
		}
		if !nested {
			kept = append(kept, p)
		}
	}
	return kept
}

// famKey 取相对 img 根的前两段作为图片族：pictures/busts/Liza/x.png → pictures/busts。
func famKey(relDir string) string {
	relDir = strings.TrimPrefix(filepath.ToSlash(relDir), "./")
	if relDir == "" || relDir == "." {
		return "(根目录)"
	}
	parts := strings.Split(relDir, "/")
	if len(parts) > 2 {
		parts = parts[:2]
	}
	return strings.Join(parts, "/")
}

// scanSource 扫一款游戏的 img 子树：统计族分布、加密图，挑出参与打分的 refs。
func scanSource(gameDir string, keepAssets bool) (*srcScan, error) {
	st, err := os.Stat(gameDir)
	if err != nil {
		return nil, err
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("不是目录 %s", gameDir)
	}
	s := &srcScan{}
	roots := findScanRoots(gameDir)
	if len(roots) == 0 {
		roots = []string{gameDir}
		s.Note = "未找到 img 子树，回退扫游戏根目录"
	}
	multi := len(roots) > 1
	if multi {
		s.Note = appendNote(s.Note, fmt.Sprintf("扫到 %d 个 img 根，两套目录各存一份图，重复率会被抬高", len(roots)))
	}

	type agg struct {
		n     int
		asset bool
	}
	fams := map[string]*agg{}
	var refs []*ImgRef

	for _, root := range roots {
		label := ""
		if root == gameDir {
			s.Roots = append(s.Roots, ".")
		} else if r, rerr := filepath.Rel(gameDir, root); rerr == nil {
			label = filepath.ToSlash(r)
			s.Roots = append(s.Roots, label)
		}
		filepath.WalkDir(root, func(p string, d fs.DirEntry, werr error) error {
			if werr != nil || d.IsDir() {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(d.Name()))
			if encImageExts[ext] {
				s.Encrypted++
				return nil
			}
			if !imageExts[ext] {
				return nil
			}
			info, ierr := d.Info()
			if ierr != nil {
				return nil
			}
			rel, rerr := filepath.Rel(gameDir, p)
			if rerr != nil {
				rel = d.Name()
			}
			rel = filepath.ToSlash(rel)
			relRoot, _ := filepath.Rel(root, p)
			fam := famKey(filepath.Dir(relRoot))
			asset := isAssetFamily(fam)
			if multi && label != "" {
				fam = label + "/" + fam
			}
			a := fams[fam]
			if a == nil {
				a = &agg{asset: asset}
				fams[fam] = a
			}
			a.n++
			s.All++
			refs = append(refs, &ImgRef{
				Path: p, Rel: rel, Dir: filepath.ToSlash(filepath.Dir(rel)),
				Name: d.Name(), Ext: ext, Size: info.Size(),
				Fam: fam, FamAsset: asset,
			})
			return nil
		})
	}

	for name, a := range fams {
		kind := "cg"
		if a.asset {
			kind = "asset"
		}
		s.Families = append(s.Families, Family{Name: name, Count: a.n, Kind: kind})
	}
	sort.Slice(s.Families, func(i, j int) bool {
		if s.Families[i].Count != s.Families[j].Count {
			return s.Families[i].Count > s.Families[j].Count
		}
		return s.Families[i].Name < s.Families[j].Name
	})

	for _, r := range refs {
		if !keepAssets && r.FamAsset {
			s.AssetRefs++
			continue
		}
		s.Refs = append(s.Refs, r)
	}
	sort.Slice(s.Refs, func(i, j int) bool { return s.Refs[i].Rel < s.Refs[j].Rel })

	// 兜底：一张 CG 族都没识别出来、但确实有明文图、也没加密 → 全量参与，别把款判成空
	if len(s.Refs) == 0 && s.All > 0 && s.Encrypted == 0 {
		for _, r := range refs {
			s.Refs = append(s.Refs, r)
		}
		s.AssetRefs = 0
		s.Note = appendNote(s.Note, "未识别出 CG 族，回退全量扫描")
	}
	return s, nil
}

func appendNote(a, b string) string {
	if a == "" {
		return b
	}
	return a + "；" + b
}

// applySourceSignals 在通用指标之上补源目录特有的判据，并重算 severity / verdict。
func applySourceSignals(g *Game, p Params) {
	var total int
	var reasons []string
	if g.Images > 0 {
		total, reasons = scorePoints(g, p)
	}
	extra := 0
	add := func(n int, why string) {
		extra += n
		reasons = append(reasons, fmt.Sprintf("%s (+%d)", why, n))
	}

	switch {
	case g.Images == 0 && g.Encrypted > 0:
		add(5, fmt.Sprintf("CG 族里没有明文图，只剩加密图 %d 个（.rpgmvp/.png_；另有引擎素材 %d 张已剔除），机器读不到像素", g.Encrypted, g.AssetImages))
	case g.Images == 0:
		add(5, "源目录里没有 png/jpg/webp，机器没东西可算")
	case g.Encrypted > 0:
		switch {
		case g.Encrypted > g.Images*3:
			add(2, fmt.Sprintf("加密图 %d 个未计入（明文只 %d 张）", g.Encrypted, g.Images))
		case g.Encrypted*3 > g.Images:
			add(1, fmt.Sprintf("另有加密图 %d 个未计入（明文 %d 张）", g.Encrypted, g.Images))
		}
	}
	if g.ManifestCG >= 10 && g.Encrypted == 0 && g.Images*10 < g.ManifestCG*3 {
		add(2, fmt.Sprintf("清单 cg_count=%d，源目录明文图只有 %d 张", g.ManifestCG, g.Images))
	}
	if g.AssetImages > 0 && g.Images > 0 {
		reasons = append(reasons, fmt.Sprintf("另有引擎素材 %d 张（tilesets/characters 等，已剔除，见 families）", g.AssetImages))
	}

	total += extra
	if len(reasons) == 0 {
		reasons = []string{"无明显异常"}
	}
	g.Severity = sevBand(total)
	g.SeverityReasons = reasons
	g.Verdict = verdictFor(g.Severity)
}

type sourceOptions struct {
	Manifest string
	Engine   string
	Only     string
	OutDir   string
	JSONOut  string
	MDOut    string
	JSONSet  bool
	MDSet    bool
	Quiet    bool
}

func runSource(p Params, o sourceOptions) error {
	entries, err := loadManifest(o.Manifest, o.Engine, o.Only)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return fmt.Errorf("清单里没有匹配的款（engine=%q only=%q）", o.Engine, o.Only)
	}

	suffix := o.Engine
	if suffix == "" {
		suffix = "all"
	}
	outDir := o.OutDir
	if outDir == "" {
		outDir = filepath.Join("reports", "人工抽检")
	}
	jsonOut, mdOut := o.JSONOut, o.MDOut
	if !o.JSONSet || jsonOut == "" {
		jsonOut = filepath.Join(outDir, "预筛指标_"+suffix+".json")
	}
	if !o.MDSet || mdOut == "" {
		mdOut = filepath.Join(outDir, "预筛可疑_"+suffix+".md")
	}

	rep := &Report{
		Root:        o.Manifest,
		Manifest:    o.Manifest,
		Engine:      suffix,
		GeneratedAt: time.Now().Format(time.RFC3339),
		Params:      p,
		Summary:     Summary{SeverityCounts: make([]int, 4)},
	}

	for _, e := range entries {
		if _, serr := os.Stat(e.SourceRealPath); serr != nil {
			rep.Summary.Missing = append(rep.Summary.Missing, e.Name)
			fmt.Fprintf(os.Stderr, "缺目录 %s → %s\n", e.Name, e.SourceRealPath)
			continue
		}
		start := time.Now()
		s, serr := scanSource(e.SourceRealPath, p.KeepAssets)
		if serr != nil {
			rep.Summary.Missing = append(rep.Summary.Missing, e.Name)
			fmt.Fprintf(os.Stderr, "读不了 %s → %v\n", e.Name, serr)
			continue
		}
		g := analyze(p, e.Name, e.SourceRealPath, s.Refs, 0)
		g.Engine = e.Engine
		g.ManifestDir = e.SourceDir
		g.ManifestCG = e.CGCount
		g.ScanRoots = s.Roots
		g.Scanned = s.All
		g.AssetImages = s.AssetRefs
		g.Encrypted = s.Encrypted
		g.Families = s.Families
		g.Note = s.Note
		applySourceSignals(g, p)
		g.ScanMS = time.Since(start).Milliseconds()

		rep.Games = append(rep.Games, g)
		rep.Summary.ImagesTotal += g.Images
		rep.Summary.ScannedTotal += g.Scanned
		rep.Summary.EncryptedTotal += g.Encrypted
		rep.Summary.SeverityCounts[g.Severity]++
		if g.Images > 0 {
			rep.Summary.PlainGames++
		} else {
			rep.Summary.NoPlainGames++
		}
		if !o.Quiet {
			fmt.Fprintf(os.Stderr, "%-30s 图 %-5d 极小 %-4d 离群 %-4d 空白 %-3d 重复组 %-3d 冗余 %5.1f%% 均 %6.1fKB 加密 %-5d sev=%d  (%dms)\n",
				cut(e.Name, 30), g.Images, g.Small, g.Outliers, g.Blank, g.DupGroups, g.DupRate*100, g.AvgKB, g.Encrypted, g.Severity, g.ScanMS)
		}
	}
	rep.Summary.Games = len(rep.Games)
	rep.Summary.MissingCount = len(rep.Summary.Missing)

	sort.Slice(rep.Games, func(i, j int) bool {
		if rep.Games[i].Severity != rep.Games[j].Severity {
			return rep.Games[i].Severity > rep.Games[j].Severity
		}
		return rep.Games[i].Name < rep.Games[j].Name
	})

	for _, path := range []string{jsonOut, mdOut} {
		if d := filepath.Dir(path); d != "" && d != "." {
			if err := os.MkdirAll(d, 0o755); err != nil {
				return fmt.Errorf("建目录失败 %s: %w", d, err)
			}
		}
	}
	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	if err := os.WriteFile(jsonOut, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("写 JSON %s: %w", jsonOut, err)
	}
	if err := os.WriteFile(mdOut, []byte(renderSourceMarkdown(rep)), 0o644); err != nil {
		return fmt.Errorf("写 Markdown %s: %w", mdOut, err)
	}

	printSourceTable(rep)
	fmt.Printf("\n写出 %s\n写出 %s\n", jsonOut, mdOut)
	if rep.Summary.ImagesTotal == 0 {
		fmt.Fprintln(os.Stderr, "注意：本次没有任何明文图可机检")
	}
	return nil
}

func cut(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func printSourceTable(rep *Report) {
	fmt.Printf("清单 %s（engine=%s）\n", rep.Manifest, rep.Engine)
	fmt.Printf("%-30s %6s %5s %5s %5s %6s %7s %8s %6s %4s\n",
		"游戏", "图片", "极小", "离群", "空白", "重复组", "冗余率", "均KB", "加密", "sev")
	for _, g := range rep.Games {
		fmt.Printf("%-30s %6d %5d %5d %5d %6d %6.1f%% %8.1f %6d %4d\n",
			cut(g.Name, 30), g.Images, g.Small, g.Outliers, g.Blank, g.DupGroups, g.DupRate*100, g.AvgKB, g.Encrypted, g.Severity)
	}
	fmt.Printf("\nseverity 分布：0=%d 1=%d 2=%d 3=%d\n",
		rep.Summary.SeverityCounts[0], rep.Summary.SeverityCounts[1],
		rep.Summary.SeverityCounts[2], rep.Summary.SeverityCounts[3])
	fmt.Printf("款数 %d（缺目录 %d），扫到图 %d 张，参与打分 %d 张，加密图 %d 个\n",
		rep.Summary.Games, rep.Summary.MissingCount, rep.Summary.ScannedTotal, rep.Summary.ImagesTotal, rep.Summary.EncryptedTotal)
}

func renderSourceMarkdown(rep *Report) string {
	var sb strings.Builder
	sb.WriteString("# 机器预筛 · rmmz 源目录可疑清单\n\n")
	sb.WriteString("由 tools/review-prescreen -from-manifest 生成，所有数字来自实跑。\n\n")
	fmt.Fprintf(&sb, "- 来源清单：%s（engine=%s，命中 %d 款）\n", rep.Manifest, rep.Engine, rep.Summary.Games+rep.Summary.MissingCount)
	fmt.Fprintf(&sb, "- 生成时间：%s\n", rep.GeneratedAt)
	fmt.Fprintf(&sb, "- 缺目录：%d 款", rep.Summary.MissingCount)
	if len(rep.Summary.Missing) > 0 {
		sb.WriteString("（" + strings.Join(rep.Summary.Missing, "、") + "）")
	}
	sb.WriteString("\n")
	fmt.Fprintf(&sb, "- 图：扫到 png/jpg/webp %d 张，参与打分 %d 张，加密图（.rpgmvp/.png_）%d 个\n\n",
		rep.Summary.ScannedTotal, rep.Summary.ImagesTotal, rep.Summary.EncryptedTotal)

	fmt.Fprintf(&sb, "- 抽样：每款最多 %d 张（首 5 + 尾 5 + 随机补足），解码像素上限 %d（超过只读头），并发 %d\n",
		rep.Params.MaxSample, rep.Params.MaxPixels, rep.Params.Conc)
	fmt.Fprintf(&sb, "- 阈值：极小图 任一边<%dpx；离群 偏离众数>%g倍；空白 抽样灰度方差<%g；重复 同尺寸+8x8 dHash；体积 <%gKB\n",
		rep.Params.MinSide, rep.Params.OutlierMul, rep.Params.VarThresh, rep.Params.SmallKB)
	if rep.Params.SkipSparse {
		sb.WriteString("- 散图判据：源目录模式只统计不扣分（-sparse-penalty 可打开）——rmmz 的 pictures/ 常是平铺的人写标题，前缀系列判据不适用\n")
	}
	if rep.Params.KeepAssets {
		sb.WriteString("- 族过滤：已关闭（-keep-assets，引擎素材也参与打分）\n")
	} else {
		sb.WriteString("- 族过滤：tilesets/characters/enemies/sv_*/battlebacks/parallaxes/animations/system/titles* 等引擎素材默认剔除，不参与打分（族分布里仍列全）\n")
	}

	fmt.Fprintf(&sb, "\nseverity 分布：0=%d，1=%d，2=%d，3=%d\n\n",
		rep.Summary.SeverityCounts[0], rep.Summary.SeverityCounts[1],
		rep.Summary.SeverityCounts[2], rep.Summary.SeverityCounts[3])

	sb.WriteString("## 机检覆盖\n\n")
	fmt.Fprintf(&sb, "- 有明文图、机器能算：**%d 款**（参与打分 %d 张）\n", rep.Summary.PlainGames, rep.Summary.ImagesTotal)
	fmt.Fprintf(&sb, "- CG 族全加密、机器读不到像素：**%d 款**（加密图 %d 个）\n", rep.Summary.NoPlainGames, rep.Summary.EncryptedTotal)
	sb.WriteString("- sev 3 有两类原因：真的查出问题（空白/重复/离群/命名断裂…），或「图加密了机器查不了」。后者只能人工开游戏看，已单列在下文「加密款」。\n\n")

	sb.WriteString("### 可机检款（按 severity 降序，这才是能横向比的）\n\n")
	sb.WriteString("| sev | 游戏 | 图片 | 极小 | 离群 | 空白 | 近似重复组 | 冗余率 | 均KB | 结论 |\n")
	sb.WriteString("| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |\n")
	for _, g := range rep.Games {
		if g.Images == 0 {
			continue
		}
		fmt.Fprintf(&sb, "| %d | %s | %d | %d | %d | %d | %d | %.1f%% | %.1f | %s |\n",
			g.Severity, g.Name, g.Images, g.Small, g.Outliers, g.Blank, g.DupGroups,
			g.DupRate*100, g.AvgKB, g.Verdict)
	}

	sb.WriteString("\n## 总览（全部 37 款，按 severity 降序）\n\n")
	sb.WriteString("| sev | 游戏 | 图片 | 族数 | 极小 | 离群 | 空白 | 近似重复组 | 冗余率 | 均KB | 加密图 | 结论 |\n")
	sb.WriteString("| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |\n")
	for _, g := range rep.Games {
		fmt.Fprintf(&sb, "| %d | %s | %d | %d | %d | %d | %d | %d | %.1f%% | %.1f | %d | %s |\n",
			g.Severity, g.Name, g.Images, len(g.Families), g.Small, g.Outliers, g.Blank,
			g.DupGroups, g.DupRate*100, g.AvgKB, g.Encrypted, g.Verdict)
	}

	sb.WriteString("\n## 每款一句话结论\n")
	for _, g := range rep.Games {
		why := strings.Join(g.SeverityReasons, "；")
		if len([]rune(why)) > 160 {
			why = cut(why, 160)
		}
		fmt.Fprintf(&sb, "- **%s** — sev %d — %s（%s）\n", g.Name, g.Severity, g.Verdict, why)
	}

	sb.WriteString("\n## 图片族分布（每款前 6 族）\n")
	for _, g := range rep.Games {
		var parts []string
		for i, f := range g.Families {
			if i >= 6 {
				parts = append(parts, fmt.Sprintf("…等 %d 族", len(g.Families)))
				break
			}
			mark := ""
			if f.Kind == "asset" {
				mark = "(素材)"
			}
			parts = append(parts, fmt.Sprintf("%s %d%s", f.Name, f.Count, mark))
		}
		if len(parts) == 0 {
			parts = []string{"(无)"}
		}
		fmt.Fprintf(&sb, "- **%s**：%s\n", g.Name, strings.Join(parts, "、"))
	}

	sb.WriteString("\n## 加密款（机器读不到像素）\n\n")
	sb.WriteString("| 游戏 | 加密图 | 明文图 | 扫描根 |\n| --- | ---: | ---: | --- |\n")
	anyEnc := false
	for _, g := range rep.Games {
		if g.Encrypted == 0 {
			continue
		}
		anyEnc = true
		fmt.Fprintf(&sb, "| %s | %d | %d | %s |\n", g.Name, g.Encrypted, g.Images, strings.Join(g.ScanRoots, "、"))
	}
	if !anyEnc {
		sb.WriteString("\n本次没有加密图。\n")
	}

	sb.WriteString("\n## severity=3 具体文件\n")
	any := false
	for _, g := range rep.Games {
		if g.Severity != 3 {
			continue
		}
		any = true
		fmt.Fprintf(&sb, "\n### %s（图 %d 张，扫到 %d 张，加密 %d 个）\n", g.Name, g.Images, g.Scanned, g.Encrypted)
		fmt.Fprintf(&sb, "- 目录：%s\n", g.Dir)
		if g.Note != "" {
			fmt.Fprintf(&sb, "- 备注：%s\n", g.Note)
		}
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
		if len(g.OutlierSamples) > 0 {
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
		sb.WriteString("\n本次无 severity=3 的款。\n")
	}

	sb.WriteString("\n## 指标口径\n")
	sb.WriteString("- images：参与打分的 png/jpg/webp 张数（引擎素材族默认剔除；加密图 .rpgmvp/.png_ 不算图，单独计 encrypted）。\n")
	sb.WriteString("- families：相对 img 根的前两段路径（如 pictures、pictures/busts、pictures/CG）。kind=asset 表示判成引擎素材：RPG Maker 标准目录（tilesets/characters/enemies/sv_*/battlebacks/parallaxes/animations/system/titles*…）+ 族名里带 ui/icon/menu/hud/logo/font/button/window/cursor/frame 这类词的目录。\n")
	sb.WriteString("- small / outliers / blank / dup_groups / dup_rate / exact_dup_* / sparse / avg_kb：与导出目录模式完全同一套口径。\n")
	sb.WriteString("- encrypted：源目录里的 .rpgmvp/.png_ 数量；这些图读不到像素，指标只能覆盖明文部分。\n")
	sb.WriteString("- misnamed：扩展名与实际格式不符（如 .png 实为 JPEG）；图能正常看，但按扩展名解析的脚本会失败。\n")
	sb.WriteString("- severity：通用判据 + 源目录补充判据（全加密 +3、多数加密 +2、部分加密 +1、明文图远少于清单 cg_count +2）加权后分档。\n")
	sb.WriteString("- verdict：3=建议全量肉眼 / 1-2=抽检 30 张即可 / 0=无需看。\n")
	return sb.String()
}
