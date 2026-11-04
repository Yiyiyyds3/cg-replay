// stage.go —— 从解包素材推断原作舞台尺寸。
//
// 为什么需要：桌面壳按「约定里的舞台尺寸」合成画面，素材是原生坐标——
// 尺寸不对，背景会裁切或留黑边。编译剧本（scn）能在约定里写死 1280×720，
// 但 CatSystem2 / Artemis / BGI 的同一引擎不同作品分辨率不同
// （实测 1280×720、1024×576、1920×1080 都有），只能从素材本身推断。
//
// 判据：**背景图（bg*）里出现最多的那个尺寸**。
// 用背景图而不是「所有大图」的理由：立绘/差分/E-mote 部件在图库里数量
// 更多且尺寸零碎（实测有 774×781 的部件把推断带偏到非 16:9）。
package library

import (
	"image"
	_ "image/jpeg" // 注册 JPEG 解码器（DecodeConfig 只需要头）
	_ "image/png"  // 注册 PNG 解码器
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	// stageSampleMax 每轮最多采样多少张图（大库有几万张，全读太慢）。
	stageSampleMax = 600
	// stageMinW/stageMinH 视为「舞台尺寸级」大图的尺寸下限。
	stageMinW = 640
	stageMinH = 360
	// stageProbeDepth 采样时向下探查的最大层数。
	stageProbeDepth = 6
	// stageMinVotes 至少几张图同尺寸才认（防止用一两张特写图推断）。
	stageMinVotes = 3
)

// DetectStageSize 从解包素材推断舞台尺寸（导出给 CLI 等没有走事件库扫描的
// 调用方用），推断不出返回 (0, 0)。
func DetectStageSize(root string) (int, int) {
	return detectStageSize(root)
}

// detectStageSize 从 root 下的图片推断舞台尺寸，推断不出返回 (0, 0)。
func detectStageSize(root string) (int, int) {
	if w, h := modalSize(root, isBGPath); w > 0 {
		return w, h
	}
	// 没有背景图命名的素材（解包工具改了名）：退回「所有大图里的众数」。
	return modalSize(root, nil)
}

// modalSize 统计符合条件的图片尺寸众数；filter 为 nil 时接受所有大图。
func modalSize(root string, filter func(string) bool) (int, int) {
	type size struct{ w, h int }
	votes := map[size]int{}
	sampled := 0
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if sampled >= stageSampleMax {
			return fs.SkipAll
		}
		if d.IsDir() {
			if extraAssetDirs[strings.ToLower(d.Name())] {
				return fs.SkipDir
			}
			if rel, e := filepath.Rel(root, p); e == nil && rel != "." &&
				strings.Count(rel, string(filepath.Separator)) >= stageProbeDepth {
				return fs.SkipDir
			}
			return nil
		}
		switch strings.ToLower(filepath.Ext(p)) {
		case ".png", ".jpg", ".jpeg":
		default:
			return nil
		}
		if filter != nil && !filter(p) {
			return nil
		}
		f, err := os.Open(p)
		if err != nil {
			return nil
		}
		cfg, _, err := image.DecodeConfig(f)
		_ = f.Close()
		if err != nil {
			return nil
		}
		sampled++
		if cfg.Width >= stageMinW && cfg.Height >= stageMinH {
			votes[size{cfg.Width, cfg.Height}]++
		}
		return nil
	})
	best, bestN := size{}, 0
	for s, n := range votes {
		// 票数相同时取更大的（同一作品既有 720p 也有 1080p 素材时不要选小的）。
		if n > bestN || (n == bestN && s.w*s.h > best.w*best.h) {
			best, bestN = s, n
		}
	}
	if bestN < stageMinVotes {
		return 0, 0
	}
	return best.w, best.h
}

// isExtraAssetPath 判断路径是否属于「随游戏附带的周边素材」而不是舞台素材。
//
// 为什么要排掉：真实案例「灰色的乐园FHD」只解出了剧本，图片还锁在
// image00.int 里，整棵树里唯一的图片是《壁紙集》的 1920×1170 壁纸——
// 按它推断舞台会得到非 16:9 的画布，回放时背景整体错位。宁可报「未推断」，
// 让调用方退回引擎默认值，也不要给一个看起来有值、实际是错的尺寸。
var extraAssetDirs = map[string]bool{
	"壁紙": true, "壁纸": true, "壁紙集": true, "wallpaper": true, "wallpapers": true,
	"特典": true, "特典ドラマ": true, "ost": true, "soundtrack": true,
	"サウンドトラック": true, "预览": true, "sample": true,
}

func isExtraAssetPath(p string) bool {
	for _, part := range strings.Split(strings.ToLower(filepath.ToSlash(p)), "/") {
		if extraAssetDirs[part] {
			return true
		}
	}
	return false
}

// isBGPath 判断路径是否像背景图：文件名以 bg 开头，或位于 bg 目录下。
func isBGPath(p string) bool {
	if strings.HasPrefix(strings.ToLower(filepath.Base(p)), "bg") {
		return true
	}
	slash := strings.ToLower(filepath.ToSlash(p))
	return strings.Contains(slash, "/bg/") || strings.Contains(slash, "/background/")
}
