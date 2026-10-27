package video

import (
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestFrameReal 用 ffmpeg 现造一段测试视频，验证能提帧且不同时刻画面不同。
func TestFrameReal(t *testing.T) {
	if !Available() {
		t.Skip("未装 ffmpeg")
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "t.mpg")
	cmd := exec.Command(Locate(), "-v", "error", "-f", "lavfi",
		"-i", "testsrc=duration=3:size=320x240:rate=25", "-c:v", "mpeg1video", p)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("造样本失败：%v %s", err, out)
	}
	if st, err := os.Stat(p); err != nil || st.Size() == 0 {
		t.Fatalf("样本为空：%v", err)
	}
	a, err := Frame(p, 300)
	if err != nil {
		t.Fatalf("取帧 300ms：%v", err)
	}
	b, err := Frame(p, 2000)
	if err != nil {
		t.Fatalf("取帧 2000ms：%v", err)
	}
	if a.Bounds() != b.Bounds() || a.Bounds().Dx() != 320 {
		t.Fatalf("帧尺寸异常：%v / %v", a.Bounds(), b.Bounds())
	}
	// testsrc 是彩条滚动，两个时刻应不同
	same := true
	for i := 0; i < 100 && same; i++ {
		x, y := i*3%320, i*7%240
		if a.At(x, y) != b.At(x, y) {
			same = false
		}
	}
	if same {
		t.Fatal("不同时刻的帧完全相同（提帧可能没生效）")
	}
}

// TestStreamFrames 流式解码：单调前进能取到不同帧，倒退会重开。
func TestStreamFrames(t *testing.T) {
	if !Available() {
		t.Skip("未装 ffmpeg")
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "s.mpg")
	cmd := exec.Command(Locate(), "-v", "error", "-f", "lavfi",
		"-i", "testsrc=duration=3:size=320x240:rate=25", "-c:v", "mpeg1video", p)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("造样本失败：%v %s", err, out)
	}
	st, err := OpenStream(p, 160, 120, 20)
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	defer st.Close()

	a, err := st.Frame(300)
	if err != nil {
		t.Fatalf("Frame(300): %v", err)
	}
	if a.Bounds().Dx() != 160 || a.Bounds().Dy() != 120 {
		t.Fatalf("尺寸应为 160x120，实际 %v", a.Bounds())
	}
	b, err := st.Frame(2000)
	if err != nil {
		t.Fatalf("Frame(2000): %v", err)
	}
	if samePixels(a, b) {
		t.Fatal("两个时刻的帧相同（顺序读没生效）")
	}
	// 倒退：应重开并仍能出帧
	c, err := st.Frame(200)
	if err != nil {
		t.Fatalf("倒退取帧失败：%v", err)
	}
	if c.Bounds().Dx() != 160 {
		t.Fatalf("倒退后尺寸异常：%v", c.Bounds())
	}
}

func samePixels(a, b image.Image) bool {
	if a.Bounds() != b.Bounds() {
		return false
	}
	bd := a.Bounds()
	for y := bd.Min.Y; y < bd.Max.Y; y += 4 {
		for x := bd.Min.X; x < bd.Max.X; x += 4 {
			r1, g1, b1, _ := a.At(x, y).RGBA()
			r2, g2, b2, _ := b.At(x, y).RGBA()
			if r1 != r2 || g1 != g2 || b1 != b2 {
				return false
			}
		}
	}
	return true
}
