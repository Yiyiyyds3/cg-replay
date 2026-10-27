// stream.go —— 流式解码：一条 ffmpeg 进程顺序吐 RGBA 帧。
//
// 单帧 seek（video.Frame）每次都要起一个 ffmpeg 进程，桌面端 20fps 播放时
// 开销偏大。回放的时间是单调向前的，所以更适合开一条进程连续出帧、
// 按时间跳过中间帧；只有时间倒退（seek/重播）才重开进程。
package video

import (
	"fmt"
	"image"
	"io"
	"os/exec"
)

// Stream 一条顺序解码的视频流。
type Stream struct {
	cmd  *exec.Cmd
	out  io.ReadCloser
	path string
	w, h int
	fps  int
	cur  int // 已读到的帧号（0 = 尚未读）
	buf  []byte
	last *image.RGBA // 最后一帧（读到片尾时冻住它，不报错）
}

// OpenStream 按目标尺寸与帧率开一条流（缩放在 ffmpeg 侧做）。
func OpenStream(path string, w, h, fps int) (*Stream, error) {
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("目标尺寸非法：%dx%d", w, h)
	}
	if fps <= 0 {
		fps = 20
	}
	s := &Stream{path: path, w: w, h: h, fps: fps, buf: make([]byte, w*h*4)}
	if err := s.start(0); err != nil {
		return nil, err
	}
	return s, nil
}

// start 从 fromFrame 起重开一条流（-ss 精确到秒，帧内再丢弃）。
func (s *Stream) start(fromFrame int) error {
	exe := Locate()
	if exe == "" {
		return fmt.Errorf("未检测到 ffmpeg，无法解码视频")
	}
	secs := float64(fromFrame) / float64(s.fps)
	cmd := exec.Command(exe,
		"-v", "error",
		"-ss", fmt.Sprintf("%.3f", secs),
		"-i", s.path,
		"-vf", fmt.Sprintf("scale=%d:%d", s.w, s.h),
		"-r", fmt.Sprint(s.fps),
		"-f", "rawvideo", "-pix_fmt", "rgba", "-",
	)
	hideConsole(cmd)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return err
	}
	s.cmd, s.out, s.cur = cmd, out, fromFrame
	return nil
}

// Frame 取 tMs 处的一帧（顺序读；时间倒退则重开）。
func (s *Stream) Frame(tMs int64) (image.Image, error) {
	if s == nil {
		return nil, fmt.Errorf("视频流未初始化")
	}
	if tMs < 0 {
		tMs = 0
	}
	target := int(tMs) * s.fps / 1000
	if target < s.cur-1 { // 倒退超过一帧：重开
		s.Close()
		if err := s.start(target); err != nil {
			return nil, err
		}
	}
	for s.cur <= target {
		if _, err := io.ReadFull(s.out, s.buf); err != nil {
			// 读到片尾：冻在最后一帧（播放器的正常行为），别报错打断演出。
			if s.last != nil {
				return s.last, nil
			}
			return nil, fmt.Errorf("视频读帧失败（t=%dms）：%w", tMs, err)
		}
		s.cur++
		s.last = pixelsToRGBA(s.buf, s.w, s.h)
	}
	if s.last != nil {
		return s.last, nil
	}
	return nil, fmt.Errorf("视频还没有可用帧")
}

// Close 结束进程。
func (s *Stream) Close() error {
	if s == nil || s.cmd == nil {
		return nil
	}
	if s.out != nil {
		_ = s.out.Close()
	}
	if s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
	err := s.cmd.Wait()
	s.cmd, s.out = nil, nil
	return err
}

// pixelsToRGBA 把 ffmpeg 的 rgba 裸数据包成 image.RGBA。
func pixelsToRGBA(buf []byte, w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	copy(img.Pix, buf)
	return img
}
