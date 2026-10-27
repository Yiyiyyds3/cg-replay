package video

import (
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
