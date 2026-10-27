package unpack

import (
	"os"
	"path/filepath"
	"testing"

	"ucgr/internal/adapter/catsystem2"
	"ucgr/internal/ir"
	"ucgr/internal/replay"
	"ucgr/internal/video"
)

// TestRealCS2VideoOP 真机验证：OP 事件能被识别成影片，并能抽到画面帧。
//
//	UCGR_CS2_UNPACKED="<含 scene/ 与 视频/ 的目录>" go test ./internal/unpack -run TestRealCS2Video -v
func TestRealCS2VideoOP(t *testing.T) {
	root := os.Getenv("UCGR_CS2_UNPACKED")
	if root == "" {
		t.Skip("未设置 UCGR_CS2_UNPACKED")
	}
	if !video.Available() {
		t.Skip("未装 ffmpeg")
	}
	data, err := os.ReadFile(filepath.Join(root, "scene", "00_movie_OP.cst"))
	if err != nil {
		t.Skip("样本脚本不存在")
	}
	ev, err := catsystem2.BuildShowScript(data, "00_movie_OP.cst")
	if err != nil {
		t.Fatalf("BuildShowScript: %v", err)
	}
	var ref string
	for _, op := range ev.Show {
		if op.Kind == ir.ShowVideoPlay {
			ref = op.Ref
			break
		}
	}
	if ref == "" {
		t.Fatalf("OP 事件里没有 video_play 指令：%+v", ev.Show[:minInt(5, len(ev.Show))])
	}
	path, err := replay.AssetPath(root, ref)
	if err != nil {
		t.Fatalf("视频 %q 解析不到文件：%v", ref, err)
	}
	t.Logf("影片指令 ref=%q → %s", ref, filepath.Base(path))

	img, err := video.Frame(path, 2000)
	if err != nil {
		t.Fatalf("取帧失败：%v", err)
	}
	b := img.Bounds()
	if b.Dx() < 100 || b.Dy() < 100 {
		t.Fatalf("帧尺寸异常：%v", b)
	}
	// 非黑像素比例：确认真的抽到了画面
	var nonBlack, total int
	for y := b.Min.Y; y < b.Max.Y; y += 8 {
		for x := b.Min.X; x < b.Max.X; x += 8 {
			r, g, bl, _ := img.At(x, y).RGBA()
			total++
			if r > 0x1000 || g > 0x1000 || bl > 0x1000 {
				nonBlack++
			}
		}
	}
	ratio := float64(nonBlack) / float64(total)
	t.Logf("2s 处帧：%dx%d，非黑像素 %.1f%%", b.Dx(), b.Dy(), ratio*100)
	if ratio < 0.05 {
		t.Fatalf("抽出的帧几乎全黑（%.1f%%），可能没解码成功", ratio*100)
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
