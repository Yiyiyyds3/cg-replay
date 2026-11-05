package replay

import (
	"fmt"
	"image"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"ucgr/internal/ir"
)

// assetDirs 常见的素材子目录（相对解包根）。
var assetDirs = []string{
	"",
	// CatSystem2：解包树把图片按归档分成 image00/image01/cndate04，
	// 剧本里的裸名（如 bg03_smr_b）需要在这些目录里按 stem+ext 找。
	"image00", "image01", "cndate04",
	"raw/data/fgimage", "raw/data/image", "raw/data/bgimage", "raw/data/system", "raw/data/rule",
	"fgimage", "image", "bgimage", "system", "rule",
	// 影片（OP/ED）：CS2 解包树把动画放在「视频/动画_movie」等目录。
	"视频/动画_movie", "视频/动画_汉化更新", "video", "movie", "anime/movie", "raw/data/movie",
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
// 与 GameLoader 共用同一套候选规则：先按原路径找，再在常见素材子目录里试扩展名。
func AssetPath(root, rel string) (string, error) {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return "", fmt.Errorf("空素材名")
	}
	slashRel := filepath.FromSlash(rel)
	base := filepath.Base(slashRel)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	cands := []string{filepath.Join(root, slashRel)}
	// 也要往上一层找：第三方解包树把「视频/」放在 原始归档解包/ 的同级，
	// 用户通常把 root 指到 原始归档解包/（那里才有 scene/）。
	for _, base := range assetRoots(root) {
		for _, d := range assetDirs {
			for _, ext := range assetExts {
				if d == "" && ext == "" {
					continue
				}
				cands = append(cands, filepath.Join(base, filepath.FromSlash(d), stem+ext))
			}
		}
	}
	for _, c := range cands {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c, nil
		}
	}
	return "", fmt.Errorf("素材未找到: %s", rel)
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
	var index map[string]string
	var nsas *nsaPool    // NScripter 归档兜底（海猫出题的图全在 arc*.nsa）
	var pacs *pacPool    // NeXAS 归档兜底（Baldr Sky 的图全在 Bmp/Stand/System.pac）
	var npas *npaPool    // Nitro+ 归档兜底（鬼哭街重制版的图全在 cg.npa）
	buildIndex := func() {
		index = map[string]string{}
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				switch strings.ToLower(d.Name()) {
				case "voice", "音频", "语音", "sound", "bgm", "se", "video", "视频",
					"字体", "font", "_font_cache", ".git":
					return fs.SkipDir
				}
				if rel, e := filepath.Rel(root, p); e == nil && rel != "." &&
					strings.Count(rel, string(filepath.Separator)) >= 8 {
					return fs.SkipDir
				}
				return nil
			}
			switch strings.ToLower(filepath.Ext(p)) {
			case ".png", ".jpg", ".jpeg", ".webp", ".tlg", ".hg3", ".bmp":
			default:
				return nil
			}
			stem := strings.ToLower(strings.TrimSuffix(filepath.Base(p), filepath.Ext(p)))
			if _, ok := index[stem]; !ok {
				index[stem] = p
			}
			// CatSystem2 的 HG3 展开名带 _<序号> 后缀（bg01_0000.png），
			// 剧本里引用的是裸名（bg01）——同时登记去后缀的别名。
			if base := diffSuffixRe.ReplaceAllString(stem, ""); base != "" && base != stem {
				if _, ok := index[base]; !ok {
					index[base] = p
				}
			}
			return nil
		})
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
		slashRel := filepath.FromSlash(rel)
		base := filepath.Base(slashRel)
		stem := strings.TrimSuffix(base, filepath.Ext(base))
		cands := []string{filepath.Join(root, slashRel)}
		for _, base := range assetRoots(root) {
			for _, d := range assetDirs {
				for _, ext := range imageExts {
					if d == "" && ext == "" {
						continue
					}
					cands = append(cands, filepath.Join(base, filepath.FromSlash(d), stem+ext))
				}
			}
		}
		for _, c := range cands {
			if st, err := os.Stat(c); err == nil && !st.IsDir() {
				return put(decodeFile(c))
			}
		}
		// 固定目录都不中：用文件名索引兜底。
		if index == nil {
			buildIndex()
		}
		key := strings.ToLower(stem)
		if p, ok := index[key]; ok {
			return put(decodeFile(p))
		}
		if base := diffSuffixRe.ReplaceAllString(key, ""); base != "" {
			if p, ok := index[base]; ok {
				return put(decodeFile(p))
			}
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
