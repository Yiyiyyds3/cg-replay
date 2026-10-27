package replay

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
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
	// 影片（OP/ED）：CS2 解包树把动画放在「视频/动画_movie」等目录。
	"视频/动画_movie", "视频/动画_汉化更新", "video", "movie", "anime/movie", "raw/data/movie",
}

// assetExts 候选扩展名（storage 名常常不带扩展名；.hg3 是 CatSystem2 侧，见 decode/hg3.go）。
var assetExts = []string{"", ".png", ".jpg", ".jpeg", ".webp", ".tlg", ".hg3",
	// 影片扩展名放最后：静态图优先，找不到才当视频试。
	".mpg", ".mpeg", ".wmv", ".mp4", ".avi", ".webm"}

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
// 再按文件名在常见素材子目录里试扩展名。带缓存，重复引用只读一次磁盘。
func GameLoader(root string) Loader {
	cache := map[string]string{}
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
		slashRel := filepath.FromSlash(rel)
		base := filepath.Base(slashRel)
		stem := strings.TrimSuffix(base, filepath.Ext(base))
		cands := []string{filepath.Join(root, slashRel)}
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
				cache[rel] = c
				return decodeFile(c)
			}
		}
		cache[rel] = ""
		return nil, fmt.Errorf("素材未找到: %s", rel)
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
