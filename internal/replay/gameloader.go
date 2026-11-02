package replay

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
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
func GameLoader(root string) Loader {
	type entry struct {
		img image.Image
		err error
	}
	cache := map[string]entry{}
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
		put := func(img image.Image, err error) (image.Image, error) {
			cache[rel] = entry{img: img, err: err}
			return img, err
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
		return put(nil, fmt.Errorf("素材未找到: %s", rel))
	}
}

// decodeFile 读文件并按需解码（PNG / TLG5 / 明文或掩码 JPEG）。
func decodeFile(path string) (image.Image, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return DecodeImage(data)
}
