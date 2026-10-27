// Package video 用本机 ffmpeg 提取视频帧，供回放 OP/ED 等影片事件使用。
//
// 为什么走 ffmpeg 而不是纯 Go：语料里是 .mpg（773 个）与 .wmv（50 个），
// MPEG-1/2 与 WMV 的纯 Go 解码器要么没有、要么不成熟；而本机已装 ffmpeg
// （导出 MP4 功能本来就用它），复用它最省事、也最稳。
//
// 单帧提取用 `-ss` 快速 seek（关键帧为单位），对回放足够；帧结果按
// (路径, 100ms 桶) 做小缓存 —— 桌面端 20fps 渲染时同一帧会被反复请求。
package video

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
)

// Exts 视频扩展名（用于判定「这个 ref 是影片」）。
var Exts = map[string]bool{
	".mpg": true, ".mpeg": true, ".wmv": true, ".mp4": true,
	".avi": true, ".webm": true, ".mov": true, ".mkv": true,
}

// IsVideo 判断路径/引用是否为影片（按扩展名）。
func IsVideo(p string) bool {
	return Exts[filepath.Ext(p)]
}

// Locate 找 ffmpeg（PATH → WinGet 链接 → C:/ffmpeg）。找不到返回空串。
func Locate() string {
	if p, err := exec.LookPath("ffmpeg"); err == nil {
		return p
	}
	var cands []string
	if la := os.Getenv("LOCALAPPDATA"); la != "" {
		cands = append(cands, filepath.Join(la, "Microsoft", "WinGet", "Links", "ffmpeg.exe"))
	}
	cands = append(cands, `C:/ffmpeg/bin/ffmpeg.exe`)
	for _, c := range cands {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	return ""
}

// Available 报告能否提帧。
func Available() bool { return Locate() != "" }

var (
	mu    sync.Mutex
	cache = map[string]*image.RGBA{} // "路径@t-桶" → 帧
	order []string                   // 简单 FIFO 淘汰
)

const cacheMax = 48

// Frame 取 path 在 tMs 时刻的一帧（带缓存）。
func Frame(path string, tMs int64) (image.Image, error) {
	if tMs < 0 {
		tMs = 0
	}
	key := fmt.Sprintf("%s@%d", path, tMs/100)
	mu.Lock()
	if img, ok := cache[key]; ok {
		mu.Unlock()
		return img, nil
	}
	mu.Unlock()

	exe := Locate()
	if exe == "" {
		return nil, fmt.Errorf("未检测到 ffmpeg，无法解码视频（把 ffmpeg 加入 PATH 或装到 C://ffmpeg//bin）")
	}
	cmd := exec.Command(exe,
		"-v", "error",
		"-ss", fmt.Sprintf("%.3f", float64(tMs)/1000),
		"-i", path,
		"-frames:v", "1",
		"-f", "image2pipe", "-vcodec", "png", "-",
	)
	hideConsole(cmd)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg 取帧失败：%v（%s）", err, trim(stderr.String()))
	}
	img, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		return nil, fmt.Errorf("视频帧解码失败：%w", err)
	}
	rgba := toRGBA(img)

	mu.Lock()
	cache[key] = rgba
	order = append(order, key)
	if len(order) > cacheMax {
		delete(cache, order[0])
		order = order[1:]
	}
	mu.Unlock()
	return rgba, nil
}

// ClearCache 清空帧缓存（换游戏/换事件时调用）。
func ClearCache() {
	mu.Lock()
	cache = map[string]*image.RGBA{}
	order = nil
	mu.Unlock()
}

func trim(s string) string {
	s = string(bytes.TrimSpace([]byte(s)))
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}

func toRGBA(img image.Image) *image.RGBA {
	if r, ok := img.(*image.RGBA); ok {
		return r
	}
	b := img.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			dst.Set(x-b.Min.X, y-b.Min.Y, img.At(x, y))
		}
	}
	return dst
}

// Duration 用 ffprobe 取时长（毫秒）；失败返回 0（调用方按「未知」处理）。
// 结果按路径缓存 —— 影片时长不会变。
var durMu sync.Mutex
var durCache = map[string]int64{}

func Duration(path string) int64 {
	durMu.Lock()
	if d, ok := durCache[path]; ok {
		durMu.Unlock()
		return d
	}
	durMu.Unlock()

	exe := Locate()
	if exe == "" {
		return 0
	}
	probe := filepath.Join(filepath.Dir(exe), "ffprobe.exe")
	if st, err := os.Stat(probe); err != nil || st.IsDir() {
		if p, err := exec.LookPath("ffprobe"); err == nil {
			probe = p
		} else {
			return 0
		}
	}
	cmd := exec.Command(probe, "-v", "error",
		"-show_entries", "format=duration", "-of", "csv=p=0", path)
	hideConsole(cmd)
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	secs, err := strconv.ParseFloat(string(bytes.TrimSpace(out)), 64)
	if err != nil || secs <= 0 {
		return 0
	}
	ms := int64(secs * 1000)
	durMu.Lock()
	durCache[path] = ms
	durMu.Unlock()
	return ms
}
