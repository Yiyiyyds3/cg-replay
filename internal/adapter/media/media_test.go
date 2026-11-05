package media

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ucgr/internal/ir"
)

func bigPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	// 随机噪声：PNG 压不动，确保超过 MinImageBytes 门槛。
	rnd := rand.New(rand.NewSource(1))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{byte(rnd.Intn(256)), byte(rnd.Intn(256)), byte(rnd.Intn(256)), 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func bigJunk(n int) []byte {
	return bytes.Repeat([]byte("0123456789"), n/10+1)[:n]
}

// writeMediaTree 一个最小「视频/图片为主」游戏目录：
// Resources/img（编号 CG）+ Resources/Movies（影片）+ 被门槛挡掉的小图。
func writeMediaTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write := func(rel string, data []byte) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("Resources/img/001.png", bigPNG(t, 200, 150))
	write("Resources/img/002.png", bigPNG(t, 200, 150))
	write("Resources/Movies/002.mp4", bigJunk(2<<20))
	write("dist/assets/images/icon.png", []byte("tiny")) // 太小，应被挡掉
	return root
}

func TestScan(t *testing.T) {
	root := writeMediaTree(t)
	res, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Refs) != 3 {
		t.Fatalf("应有 3 个事件（2 图 1 影片），实为 %d：%+v", len(res.Refs), res.Refs)
	}
	if res.Refs[0].Kind != "still" || res.Refs[0].ID != "Resources_img_001" {
		t.Fatalf("首个事件错：%+v", res.Refs[0])
	}
	if res.Refs[2].Kind != "video" {
		t.Fatalf("影片事件错：%+v", res.Refs[2])
	}
	// 影片排在图片后（按类型分组、组内按路径）。
	for _, r := range res.Refs[:2] {
		if r.Kind != "still" {
			t.Fatalf("still 应排在 video 前：%+v", r)
		}
	}
}

func TestBuildShowStill(t *testing.T) {
	root := writeMediaTree(t)
	ev, err := BuildShow(root, "Resources_img_001")
	if err != nil {
		t.Fatal(err)
	}
	if ev.Kind != ir.KindScene || len(ev.Show) < 2 {
		t.Fatalf("台本不完整：%+v", ev.Show)
	}
	if ev.Show[0].Kind != ir.ShowLayerShow || ev.Show[0].Ref != "Resources/img/001.png" {
		t.Fatalf("layer_show 错：%+v", ev.Show[0])
	}
	if ev.Show[0].Attr == nil || ev.Show[0].Attr["scale"] == "" {
		t.Fatal("应有几何属性（适配舞台）")
	}
	if ev.EndMs <= 0 {
		t.Fatal("EndMs 应大于 0")
	}
}

func TestBuildShowVideo(t *testing.T) {
	root := writeMediaTree(t)
	ev, err := BuildShow(root, "Resources_Movies_002")
	if err != nil {
		t.Fatal(err)
	}
	if ev.Show[0].Kind != ir.ShowVideoPlay {
		t.Fatalf("影片事件首条应为 video_play：%+v", ev.Show[0])
	}
	// 假 mp4 解不出时长 → 走保底。
	if ev.EndMs <= 0 {
		t.Fatal("EndMs 应有保底值")
	}
}

func TestBuildShowUnknownID(t *testing.T) {
	if _, err := BuildShow(writeMediaTree(t), "nope"); err == nil {
		t.Fatal("未知 id 应报错")
	}
}

func TestSkipEngineDirs(t *testing.T) {
	root := writeMediaTree(t)
	write := func(rel string, data []byte) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("node_modules/pkg/big.png", bigPNG(t, 200, 150))
	write("saves/big.png", bigPNG(t, 200, 150))
	res, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range res.Refs {
		if strings.Contains(r.Path, "node_modules") || strings.Contains(r.Path, "saves") {
			t.Fatalf("引擎/系统目录应被跳过：%s", r.Path)
		}
	}
}
