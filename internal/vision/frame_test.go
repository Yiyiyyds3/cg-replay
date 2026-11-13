package vision

import (
	"errors"
	"image"
	"image/color"
	"testing"
)

func solid(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

func TestIsBlackFrame(t *testing.T) {
	black := solid(64, 36, color.RGBA{A: 255})
	if !IsBlackFrame(black) {
		t.Fatal("纯黑帧应被判为黑")
	}
	visible := solid(64, 36, color.RGBA{R: 230, G: 220, B: 210, A: 255})
	if IsBlackFrame(visible) {
		t.Fatal("有画面的帧不应被判为黑")
	}
}

// 首帧纯黑、后续有画面：整段不能被判成黑屏，更不能触发会话拒绝。
func TestEvaluateBlackFirstThenVisible(t *testing.T) {
	frames := []image.Image{
		solid(64, 36, color.RGBA{A: 255}),
		solid(64, 36, color.RGBA{R: 30, G: 120, B: 220, A: 255}),
		solid(64, 36, color.RGBA{R: 240, G: 200, B: 120, A: 255}),
	}
	r, err := CheckFrames(frames)
	if err != nil {
		t.Fatalf("首帧黑、后续有画面不应报错：%v", err)
	}
	if !r.HasPicture || r.AllBlack || r.Inconclusive {
		t.Fatalf("判定 = %+v，want HasPicture=true / AllBlack=false / Inconclusive=false", r)
	}
	if r.PictureFrames != 2 {
		t.Fatalf("有画面的帧数 = %d，want 2", r.PictureFrames)
	}
}

// 纯黑判定按通道阈值收口：8 算黑，9 已经算非黑；不能把 8/9 边界
// 因浮点或压缩噪声随机翻转。
func TestBlackChannelBoundary(t *testing.T) {
	at8 := solid(32, 18, color.RGBA{R: 8, G: 8, B: 8, A: 255})
	at9 := solid(32, 18, color.RGBA{R: 9, G: 9, B: 9, A: 255})
	if !IsBlackFrame(at8) {
		t.Fatalf("通道值 8 应算黑：%+v", Analyze(at8))
	}
	if IsBlackFrame(at9) {
		t.Fatalf("通道值 9 应算有效画面：%+v", Analyze(at9))
	}
}

// 两帧黑仍然不够做「整段全黑」结论，只能 Inconclusive。
func TestEvaluateTwoBlackFramesIsInconclusive(t *testing.T) {
	r, err := CheckFrames([]image.Image{
		solid(32, 18, color.RGBA{A: 255}),
		solid(32, 18, color.RGBA{A: 255}),
	})
	if err != nil {
		t.Fatalf("两帧不足三帧，不应报全黑错误：%v", err)
	}
	if r.AllBlack || !r.Inconclusive || r.HasPicture {
		t.Fatalf("两帧黑应 Inconclusive：%+v", r)
	}
}

// 单帧纯黑只表示采样不足，不能当成整段黑屏。
func TestEvaluateSingleBlackIsInconclusive(t *testing.T) {
	r, err := CheckFrames([]image.Image{solid(32, 18, color.RGBA{A: 255})})
	if err != nil {
		t.Fatalf("单帧纯黑不应报错：%v", err)
	}
	if r.AllBlack || !r.Inconclusive {
		t.Fatalf("单帧纯黑应 Inconclusive 而非 AllBlack：%+v", r)
	}
}

// 全黑事件在够帧数后要如实报错，但函数本身不 panic。
func TestCheckFramesAllBlackReportsError(t *testing.T) {
	frames := []image.Image{
		solid(32, 18, color.RGBA{A: 255}),
		solid(32, 18, color.RGBA{A: 255}),
		solid(32, 18, color.RGBA{A: 255}),
	}
	r, err := CheckFrames(frames)
	if !errors.Is(err, ErrAllBlack) {
		t.Fatalf("全黑事件应返回 ErrAllBlack，实际 %v（%+v）", err, r)
	}
	if !r.AllBlack || r.HasPicture {
		t.Fatalf("全黑判定不对：%+v", r)
	}
}

// 低亮度但有明显画面变化（暗景/文字）不应被误判为纯黑。
func TestEvaluateDarkSceneWithDetail(t *testing.T) {
	img := solid(80, 45, color.RGBA{R: 4, G: 4, B: 5, A: 255})
	for y := 20; y < 24; y++ {
		for x := 20; x < 58; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 120, G: 140, B: 160, A: 255})
		}
	}
	st := Analyze(img)
	if !st.HasPicture || st.ColorVariance < minColorVariance {
		t.Fatalf("暗景中的文字应算有效画面：%+v", st)
	}
}
