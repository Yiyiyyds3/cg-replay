package replay

import (
	"fmt"
	"image"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// assetDirs 常见的素材子目录（相对解包根）。
var assetDirs = []string{
	"",
	// CatSystem2：解包树把图片按归档分成 image00/image01/cndate04，
	// 剧本里的裸名（如 bg03_smr_b）需要在这些目录里按 stem+ext 找。
	"image00", "image01", "cndate04",
	"raw/data/fgimage", "raw/data/image", "raw/data/bgimage", "raw/data/system", "raw/data/rule",
	"fgimage", "image", "bgimage", "system", "rule",
}

// assetExts 候选扩展名（storage 名常常不带扩展名；.hg3 是 CatSystem2 侧，见 decode/hg3.go）。
var assetExts = []string{"", ".png", ".jpg", ".jpeg", ".webp", ".tlg", ".hg3"}

// GameLoader 在解包目录下按「storage 名」找图并解码。
//
// 演出台本里记的是脚本引用（storage 名，可能没有扩展名），所以这里先按原路径找，
// 再按文件名在常见素材子目录里试扩展名；都找不到时退回**整棵树的文件名索引**
// （Artemis / CatSystem2 / BGI 的整理版解包把图片按归档分到多层子目录，
// 靠固定目录名猜不全）。索引只建一次并缓存。
func GameLoader(root string) Loader {
	cache := map[string]string{}
	nsaCache := map[string]image.Image{} // NSA 解出的图（内存，无文件路径）
	pacCache := map[string]image.Image{} // NeXAS PAC 解出的图（Baldr Sky）
	var index map[string]string
	var nsas *nsaPool    // NScripter 归档兜底（海猫出题的图全在 arc*.nsa）
	var pacs *pacPool    // NeXAS 归档兜底（Baldr Sky 的图全在 Bmp/Stand/System.pac）
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
		if p, ok := cache[rel]; ok {
			if p == "" {
				return nil, fmt.Errorf("素材未找到: %s", rel)
			}
			return decodeFile(p)
		}
		if img, ok := nsaCache[rel]; ok {
			return img, nil
		}
		if img, ok := pacCache[rel]; ok {
			return img, nil
		}
		slashRel := filepath.FromSlash(rel)
		base := filepath.Base(slashRel)
		stem := strings.TrimSuffix(base, filepath.Ext(base))
		cands := []string{filepath.Join(root, slashRel)}
		for _, d := range assetDirs {
			for _, ext := range assetExts {
				if d == "" && ext == "" {
					continue
				}
				cands = append(cands, filepath.Join(root, filepath.FromSlash(d), stem+ext))
			}
		}
		for _, c := range cands {
			if st, err := os.Stat(c); err == nil && !st.IsDir() {
				cache[rel] = c
				return decodeFile(c)
			}
		}
		// 固定目录都不中：用文件名索引兜底。
		if index == nil {
			buildIndex()
		}
		key := strings.ToLower(stem)
		if p, ok := index[key]; ok {
			cache[rel] = p
			return decodeFile(p)
		}
		if base := diffSuffixRe.ReplaceAllString(key, ""); base != "" {
			if p, ok := index[base]; ok {
				cache[rel] = p
				return decodeFile(p)
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
		cache[rel] = ""
		return nil, fmt.Errorf("素材未找到: %s", rel)
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
