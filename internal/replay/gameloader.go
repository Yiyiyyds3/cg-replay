package replay

import (
	"fmt"
	"image"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"ucgr/internal/asset"
	"ucgr/internal/ir"
)

// assetDirs 常见的素材子目录（相对解包根）。
//
// 角色子目录排在平铺的 raw/data/image 前面：新版解包按 cg/bg/stand/movie/ui
// 分类落盘，同时又保留了旧版平铺目录；优先角色目录才能选中同名里正确的那份。
var assetDirs = []string{
	"",
	"raw/data/image/cg", "raw/data/image/bg", "raw/data/image/stand",
	"raw/data/image/movie", "raw/data/image/ui", "raw/data/image/misc",
	"raw/data/fgimage", "raw/data/image", "raw/data/bgimage", "raw/data/system", "raw/data/rule",
	"fgimage", "image", "bgimage", "system", "rule",
	"cg", "bg", "stand", "movie", "ui", "misc",
	// CatSystem2：解包树把图片按归档分成 image00/image01/cndate04，
	// 剧本里的裸名（如 bg03_smr_b）需要在这些目录里按 stem+ext 找。
	"image00", "image01", "cndate04",
	// 影片（OP/ED）：CS2 解包树把动画放在「视频/动画_movie」等目录。
	"视频/动画_movie", "视频/动画_汉化更新", "video", "anime/movie", "raw/data/movie",
}

// imageExts 静态图候选扩展名（storage 名常常不带扩展名；.hg3 见 decode/hg3.go）。
var imageExts = []string{"", ".png", ".jpg", ".jpeg", ".webp", ".tlg", ".hg3"}

// videoExts 影片扩展名（只给 AssetPath 找视频用，静态图 loader 不能碰）。
var videoExts = []string{".mpg", ".mpeg", ".wmv", ".mp4", ".avi", ".webm"}

// assetExts 全部候选（静态图优先，找不到才当视频试）。
var assetExts = append(append([]string{}, imageExts...), videoExts...)

// assetRoots 素材根的候选：自身 + 上一层（容纳「视频/」与「原始归档解包/」平级的布局）。
func assetRoots(root string) []string {
	root = strings.TrimRight(root, `\/`)
	out := []string{root}
	if parent := filepath.Dir(root); parent != "" && parent != root {
		out = append(out, parent)
	}
	return out
}

// AssetPath 把演出台本里的「storage 名」解析成解包目录下的真实文件路径。
//
// 与 GameLoader 共用同一套候选规则：先按引用里的原相对路径找，再按角色
// 子目录（cg/bg/stand/movie/ui/misc）回退，最后才按文件名兜底。旧版平铺
// 解包目录没有角色层，仍会在 raw/data/image 等老目录里命中。
func AssetPath(root, rel string) (string, error) {
	if p, err := findAssetFile(root, rel, true); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("素材未找到: %s", strings.TrimSpace(rel))
}

// assetCandidates 按优先级生成候选路径（不访问磁盘）。
//
// 顺序：
//  1. root 下按引用原路径；
//  2. 根/上一层 + 各素材目录下，按引用原相对路径（新版角色目录优先）；
//  3. 根/上一层 + 各素材目录下，按文件名（旧版平铺与裸名引用）。
func assetCandidates(root, rel string, exts []string) []string {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return nil
	}
	cleaned := filepath.FromSlash(asset.Relative(rel))
	original := filepath.FromSlash(strings.TrimLeft(strings.ReplaceAll(rel, `\`, `/`), "/"))
	baseOnly := filepath.Base(cleaned)
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		p = filepath.Clean(p)
		if p == "" || p == "." || seen[p] {
			return
		}
		seen[p] = true
		out = append(out, p)
	}
	addRef := func(dir, ref string) {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			return
		}
		stem := strings.TrimSuffix(ref, filepath.Ext(ref))
		add(filepath.Join(dir, ref))
		for _, ext := range exts {
			if ext == "" {
				continue
			}
			add(filepath.Join(dir, stem+ext))
		}
	}
	// 1) 原路径（旧库/手工目录优先）。
	addRef(root, original)
	// 2) 各素材目录下保留相对子路径，然后才是裸文件名。
	// 引用自身带角色提示时，把对应角色目录排到最前（chara_a 先查 stand/，
	// 不能因为 cg/ 字典序靠前就把同名 CG 塞给立绘引用）。
	for _, base := range assetRoots(root) {
		for _, d := range orderedAssetDirs(rel) {
			dir := base
			if d != "" {
				dir = filepath.Join(base, filepath.FromSlash(d))
			}
			addRef(dir, cleaned)
			if baseOnly != cleaned {
				addRef(dir, baseOnly)
			}
		}
	}
	return out
}

// orderedAssetDirs 按引用角色重排素材目录：同角色目录优先，其余保持原序。
func orderedAssetDirs(ref string) []string {
	role := asset.RoleOf(ref)
	if role == asset.RoleMisc {
		return assetDirs
	}
	preferred := make([]string, 0, 2)
	rest := make([]string, 0, len(assetDirs))
	for _, d := range assetDirs {
		slash := strings.ToLower(filepath.ToSlash(d))
		if slash == role.Dir() || strings.HasSuffix(slash, "/"+role.Dir()) {
			preferred = append(preferred, d)
		} else {
			rest = append(rest, d)
		}
	}
	return append(preferred, rest...)
}

// findAssetFile 查找素材文件：候选路径 → 整棵树索引兜底。
// includeVideo 为 false 时跳过影片（GameLoader 只解码静态图）。
func findAssetFile(root, rel string, includeVideo bool) (string, error) {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return "", fmt.Errorf("空素材名")
	}
	exts := imageExts
	if includeVideo {
		exts = assetExts
	}
	for _, c := range assetCandidates(root, rel, exts) {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c, nil
		}
	}
	files := collectAssetFiles(root, includeVideo)
	if best := bestAssetFile(files, rel); best != "" {
		return best, nil
	}
	return "", fmt.Errorf("素材未找到: %s", rel)
}

// assetFile 整棵树索引里的一项。
type assetFile struct {
	path string
	rel  string // 相对 root，'/' 分隔，小写
	stem string
	role asset.Role
}

// collectAssetFiles 为文件系统兜底建立一次性索引。
//
// 角色目录优先，但这里保留全部同名文件；真正选哪一个由 bestAssetFile 按
// 引用名打分，不能再用「先遍历到谁就是谁」。
func collectAssetFiles(root string, includeVideo bool) []assetFile {
	var out []assetFile
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			switch strings.ToLower(d.Name()) {
			case "voice", "音频", "语音", "sound", "bgm", "se", "字体", "font", "_font_cache", ".git":
				return fs.SkipDir
			}
			if rel, e := filepath.Rel(root, p); e == nil && rel != "." &&
				strings.Count(rel, string(filepath.Separator)) >= 8 {
				return fs.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(p))
		switch ext {
		case ".png", ".jpg", ".jpeg", ".webp", ".tlg", ".hg3", ".bmp":
		default:
			if !includeVideo || !videoExtsSet[ext] {
				return nil
			}
		}
		rel, e := filepath.Rel(root, p)
		if e != nil {
			return nil
		}
		rel = strings.ToLower(filepath.ToSlash(rel))
		out = append(out, assetFile{
			path: p,
			rel:  rel,
			stem: strings.ToLower(strings.TrimSuffix(filepath.Base(p), ext)),
			role: asset.RoleOf(rel),
		})
		return nil
	})
	return out
}

var videoExtsSet = func() map[string]bool {
	m := map[string]bool{}
	for _, e := range videoExts {
		m[e] = true
	}
	return m
}()

// bestAssetFile 按「引用相对路径 > 角色一致 > 文件名 > 浅层」选择同名候选。
func bestAssetFile(files []assetFile, ref string) string {
	refRel := strings.ToLower(filepath.ToSlash(strings.TrimSpace(ref)))
	refRel = strings.TrimPrefix(refRel, "/")
	refNoExt := strings.TrimSuffix(refRel, strings.ToLower(filepath.Ext(refRel)))
	refStem := strings.ToLower(strings.TrimSuffix(filepath.Base(refRel), filepath.Ext(refRel)))
	role := asset.RoleOf(ref)
	best, bestScore := "", -1<<30
	for _, f := range files {
		score := 0
		matched := false
		relNoExt := strings.TrimSuffix(f.rel, strings.ToLower(filepath.Ext(f.rel)))
		if refNoExt != "" && (relNoExt == refNoExt || strings.HasSuffix(relNoExt, "/"+refNoExt)) {
			score += 160 + len(refNoExt)*2
			matched = true
		}
		if f.stem == refStem || diffSuffixRe.ReplaceAllString(f.stem, "") == refStem {
			score += 80
			matched = true
		}
		if !matched {
			continue
		}
		if role != asset.RoleMisc && f.role == role {
			score += 60
		}
		// 引用名自带目录时，路径后缀命中比裸文件名可靠得多。
		if strings.Contains(refNoExt, "/") && strings.HasSuffix(relNoExt, "/"+refNoExt) {
			score += 40
		}
		score -= strings.Count(f.rel, "/")
		if score > bestScore {
			best, bestScore = f.path, score
		}
	}
	if bestScore < 0 {
		return ""
	}
	return best
}

// GameLoader 在解包目录下按「storage 名」找图并解码。
//
// 演出台本里记的是脚本引用（storage 名，可能没有扩展名），所以这里先按原路径找，
// 再按文件名在常见素材子目录里试扩展名。缓存的是**解码结果**，不是路径：
// 重复引用只读一次磁盘。影片不走这里（否则会把几百 MB 的 .mpg 整个读进内存）。
//
// 固定目录猜不全时（Artemis / CatSystem2 / BGI 的整理版解包把图片按归档分到多层
// 子目录），退回**整棵树的文件名索引**兜底；再找不到就试 NScripter 归档
// （arc*.nsa / *.sar，海猫出题的图全在里面）。索引只建一次并缓存。
func GameLoader(root string) Loader {
	type entry struct {
		img image.Image
		err error
	}
	cache := map[string]entry{}
	nsaCache := map[string]image.Image{} // NSA 解出的图（内存，无文件路径）
	pacCache := map[string]image.Image{} // NeXAS PAC 解出的图（Baldr Sky）
	npaCache := map[string]image.Image{} // NPA 解出的图（鬼哭街重制版）
	var index []assetFile
	var nsas *nsaPool // NScripter 归档兜底（海猫出题的图全在 arc*.nsa）
	var pacs *pacPool // NeXAS 归档兜底（Baldr Sky 的图全在 Bmp/Stand/System.pac）
	var npas *npaPool // Nitro+ 归档兜底（鬼哭街重制版的图全在 cg.npa）
	buildIndex := func() {
		index = collectAssetFiles(root, false)
	}
	return func(rel string) (image.Image, error) {
		rel = strings.TrimSpace(rel)
		if rel == "" {
			return nil, fmt.Errorf("空素材名")
		}
		if ir.IsVideoRef(rel) {
			return nil, fmt.Errorf("影片不能当静态图层加载: %s", rel)
		}
		if e, ok := cache[rel]; ok {
			return e.img, e.err
		}
		if img, ok := nsaCache[rel]; ok {
			return img, nil
		}
		put := func(img image.Image, err error) (image.Image, error) {
			cache[rel] = entry{img: img, err: err}
			return img, err
		}
		if img, ok := pacCache[rel]; ok {
			return img, nil
		}
		if img, ok := npaCache[rel]; ok {
			return img, nil
		}
		for _, c := range assetCandidates(root, rel, imageExts) {
			if st, err := os.Stat(c); err == nil && !st.IsDir() {
				return put(decodeFile(c))
			}
		}
		// 固定目录都不中：用整棵树索引兜底（同名按引用路径/角色评分）。
		if index == nil {
			buildIndex()
		}
		if p := bestAssetFile(index, rel); p != "" {
			return put(decodeFile(p))
		}
		// 文件系统里没有：试 NScripter 归档（arc*.nsa / *.sar）。
		if nsas == nil {
			nsas = newNsaPool(root)
		}
		if !nsas.empty() {
			if data, ok := nsas.image(rel); ok {
				if img, err := DecodeImage(data); err == nil {
					nsaCache[rel] = img
					return img, nil
				}
			}
		}
		// 再试 NeXAS 归档（*.pac）。
		if pacs == nil {
			pacs = newPacPool(root)
		}
		if !pacs.empty() {
			if data, ok := pacs.image(rel); ok {
				if img, err := DecodeImage(data); err == nil {
					pacCache[rel] = img
					return img, nil
				}
			}
		}
		// 再试 Nitro+ 归档（*.npa）。
		if npas == nil {
			npas = newNpaPool(root)
		}
		if !npas.empty() {
			if data, ok := npas.image(rel); ok {
				if img, err := DecodeImage(data); err == nil {
					npaCache[rel] = img
					return img, nil
				}
			}
		}
		return put(nil, fmt.Errorf("素材未找到: %s", rel))
	}
}

// diffSuffixRe CatSystem2 HG3 展开名的序号后缀（bg01_0000 → bg01）。
var diffSuffixRe = regexp.MustCompile(`_\d{2,4}$`)

// decodeFile 读文件并按需解码（PNG / TLG5 / 明文或掩码 JPEG）。
func decodeFile(path string) (image.Image, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return DecodeImage(data)
}
