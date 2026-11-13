package main

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ucgr/internal/cover"
	"ucgr/internal/ir"
	"ucgr/internal/player"
)

func TestCoverCacheVersionInvalidatesLegacy(t *testing.T) {
	dir := t.TempDir()
	png := coverCachePath(dir, "cg", "cg001")
	legacy := filepath.Join(dir, "cg_cg001.png")
	if png == legacy {
		t.Fatal("新版缓存名不能与旧版相同，否则旧黑图会继续复用")
	}
	if err := os.WriteFile(legacy, []byte("\x89PNG\r\n\x1a\nold"), 0o644); err != nil {
		t.Fatal(err)
	}
	if coverCacheUsable(png) {
		t.Fatal("只有旧文件时缓存必须失效")
	}
	if err := os.WriteFile(png, []byte("\x89PNG\r\n\x1a\nnew"), 0o644); err != nil {
		t.Fatal(err)
	}
	if coverCacheUsable(png) {
		t.Fatal("缺 meta 的新文件也应失效（防止半写文件被当成品）")
	}
	if err := writeCoverMeta(png, coverMeta{
		Version:   cover.Version,
		Algorithm: cover.Algorithm,
		Valid:     true,
		CreatedAt: time.Now().Format(time.RFC3339),
	}); err != nil {
		t.Fatal(err)
	}
	if !coverCacheUsable(png) {
		t.Fatal("当前版本 + meta 完整的缓存应命中")
	}
	if err := writeCoverMeta(png, coverMeta{
		Version:   cover.Version - 1,
		Algorithm: cover.Algorithm,
	}); err != nil {
		t.Fatal(err)
	}
	if coverCacheUsable(png) {
		t.Fatal("旧算法版本 meta 必须让缓存失效")
	}
}

func TestLayerShowRefsLargestFirst(t *testing.T) {
	ev := &ir.Event{Show: []ir.ShowOp{
		{Kind: ir.ShowLayerShow, Ref: "ui-frame", Attr: map[string]string{"width": "4000", "height": "3000"}},
		{Kind: ir.ShowLayerShow, Ref: "bg", Attr: map[string]string{"width": "1280", "height": "720"}},
		{Kind: ir.ShowLayerShow, Ref: "bg", Attr: map[string]string{"width": "1280", "height": "720"}},
		{Kind: ir.ShowLayerShow, Ref: "sprite", Attr: map[string]string{"width": "200", "height": "500", "scale": "2"}},
	}}
	refs := layerShowRefs(ev)
	if len(refs) != 3 {
		t.Fatalf("应去重为 3 个引用，实际 %v", refs)
	}
	if refs[0] != "ui-frame" || refs[1] != "bg" || refs[2] != "sprite" {
		t.Fatalf("候选应按显示面积排序：%v", refs)
	}
}

func TestPickDirectImageUsesNonBlackCandidate(t *testing.T) {
	black := image.NewRGBA(image.Rect(0, 0, 64, 36))
	colored := image.NewRGBA(image.Rect(0, 0, 64, 36))
	for y := 0; y < 36; y++ {
		for x := 0; x < 64; x++ {
			colored.SetRGBA(x, y, color.RGBA{230, 180, 40, 255})
		}
	}
	load := func(ref string) (image.Image, error) {
		if ref == "color" {
			return colored, nil
		}
		return black, nil
	}
	pick, ok := pickDirectImage([]string{"black", "color"}, player.Loader(load))
	if !ok || pick.ref != "color" || !pick.metrics.Valid() {
		t.Fatalf("应跳过全黑候选选彩色图：ok=%v pick=%+v", ok, pick)
	}
}
