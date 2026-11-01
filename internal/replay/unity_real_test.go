package replay

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"ucgr/internal/adapter"
	"ucgr/internal/adapter/unitycg"
	"ucgr/internal/library"
	"ucgr/internal/player"
)

// TestUnityRealPlay 真机验证：导出的 CG 目录能不能被当成「游戏库」扫出来、并真的渲染出画面。
//
// 需要环境变量：
//
//	UCGR_UNITY_ROOT  导出器产出的目录（含 cg-index.json / cg/）
//	UCGR_UNITY_SHOT  可选，把每部 CG 的第一帧存成 PNG，便于人眼确认
func TestUnityRealPlay(t *testing.T) {
	root := os.Getenv("UCGR_UNITY_ROOT")
	if root == "" {
		t.Skip("需要 UCGR_UNITY_ROOT")
	}
	lib, err := library.ScanWith(root, adapter.Convention{EngineID: "unity"})
	if err != nil {
		t.Fatalf("扫描失败: %v", err)
	}
	t.Logf("事件库：%d 个可播事件（engine=%s）", len(lib.Scenes), lib.EngineID)

	load := player.Cached(player.Loader(GameLoader(root)))
	for _, s := range lib.Scenes {
		cg, err := library.BuildUnityEvent(root, s.ID)
		if err != nil {
			t.Fatalf("%s: %v", s.ID, err)
		}
		ev, err := unitycg.BuildShow(cg)
		if err != nil {
			t.Fatalf("%s: %v", s.ID, err)
		}
		p := player.New(ev, player.DefaultOptions())
		p.Advance(0, player.Input{})
		img, drawn, err := player.Compose(p.Frame(), load, 0, 0)
		if err != nil {
			t.Fatalf("%s 渲染失败: %v", s.ID, err)
		}
		nonBlank := countNonBlank(img)
		t.Logf("%-14s kind=%-6s 台本 %2d 条 · %dx%d · 非空像素 %d · 图层 %v",
			s.ID, cg.Kind, len(ev.Show), img.Bounds().Dx(), img.Bounds().Dy(), nonBlank, drawn)
		if nonBlank == 0 {
			t.Errorf("%s 渲染出来是空白", s.ID)
		}
		if dir := os.Getenv("UCGR_UNITY_SHOT"); dir != "" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatalf("建截图目录失败: %v", err)
			}
			out := filepath.Join(dir, s.ID+".png")
			f, err := os.Create(out)
			if err != nil {
				t.Fatalf("写截图失败: %v", err)
			}
			if err := png.Encode(f, img); err != nil {
				t.Fatalf("编码截图失败: %v", err)
			}
			f.Close()
		}
	}
}

func countNonBlank(img *image.RGBA) int {
	b := img.Bounds()
	n := 0
	for y := b.Min.Y; y < b.Max.Y; y += 4 {
		for x := b.Min.X; x < b.Max.X; x += 4 {
			r, g, bl, a := img.At(x, y).RGBA()
			if a > 0 && (r > 0 || g > 0 || bl > 0) {
				n++
			}
		}
	}
	return n
}
