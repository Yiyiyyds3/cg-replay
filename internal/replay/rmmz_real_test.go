package replay

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"ucgr/internal/adapter"
	"ucgr/internal/adapter/rmmz"
	"ucgr/internal/library"
	"ucgr/internal/player"
)

// TestRMMZRealPlay 真机验证：RPG Maker MV/MZ 游戏能扫出演出、并渲染出画面。
//
// 需要环境变量：
//
//	UCGR_RMMZ_ROOT  游戏根目录（含 www/data 或 data）
//	UCGR_RMMZ_SHOT  可选，把第一个事件的画面存成 PNG
func TestRMMZRealPlay(t *testing.T) {
	root := os.Getenv("UCGR_RMMZ_ROOT")
	if root == "" {
		t.Skip("需要 UCGR_RMMZ_ROOT")
	}
	dataDir, ok := rmmz.FindDataDir(root)
	if !ok {
		t.Fatalf("没找到 data 目录")
	}
	lib, err := library.ScanWith(root, adapter.Convention{EngineID: "rmmz", ScenarioDir: dataDir})
	if err != nil {
		t.Fatalf("扫描失败: %v", err)
	}
	if len(lib.Scenes) == 0 {
		t.Fatal("没有扫到可回放事件")
	}
	t.Logf("事件库：%d 个可播事件（data=%s）", len(lib.Scenes), dataDir)

	load := player.Cached(player.Loader(GameLoader(root)))
	for _, s := range lib.Scenes[:1] {
		ev, err := rmmz.BuildShow(root, dataDir, s.Label)
		if err != nil {
			t.Fatalf("%s: %v", s.ID, err)
		}
		p := player.New(ev, player.DefaultOptions())
		p.Advance(0, player.Input{})
		p.Advance(3000, player.Input{})
		img, drawn, err := player.RenderFrame(p, load, 1600, 1200)
		if err != nil {
			t.Fatalf("%s 渲染失败: %v", s.ID, err)
		}
		t.Logf("%s 画上 %d 层", s.ID, len(drawn))
		if len(drawn) == 0 {
			t.Fatalf("%s 没有画出任何图层", s.ID)
		}
		if out := os.Getenv("UCGR_RMMZ_SHOT"); out != "" {
			if err := saveFramePNG(filepath.Join(out, "rmmz.png"), img); err != nil {
				t.Fatalf("存图失败: %v", err)
			}
		}
	}
}

func saveFramePNG(path string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
