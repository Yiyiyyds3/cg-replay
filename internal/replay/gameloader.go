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
	"raw/data/fgimage", "raw/data/image", "raw/data/bgimage", "raw/data/system", "raw/data/rule",
	"fgimage", "image", "bgimage", "system", "rule",
}

// assetExts 候选扩展名（storage 名常常不带扩展名）。
var assetExts = []string{"", ".png", ".jpg", ".jpeg", ".webp", ".tlg"}

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
