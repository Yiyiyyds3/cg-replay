package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ucgr/internal/adapter/unitycg"
)

func TestLoadSpineAssets(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "cg", "HCG02")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{
		"HCG_02.json":  `{"skeleton":{"spine":"4.2"}}`,
		"HCG_02.atlas": "HCG_02.png\nsize: 1024,1024\npma:true\n",
		"HCG_02.png":   "fake-png",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cg := unitycg.CG{ID: "HCG02", Kind: "spine"}
	cg.Files.Text = []string{"HCG_02.json", "HCG_02.atlas"}
	cg.Files.PNG = []string{"HCG_02.png"}
	cg.Spine = &unitycg.SpineInfo{
		Skins: []string{"default", "clothes/nude"},
		Animations: []unitycg.AnimRef{
			{Name: "idle", Loop: true, Dur: 1.5},
		},
	}

	sp, err := loadSpineAssets(root, cg)
	if err != nil {
		t.Fatalf("loadSpineAssets: %v", err)
	}
	if !strings.Contains(sp.Skeleton, `"spine"`) || !strings.Contains(sp.Atlas, "size:") {
		t.Fatalf("skeleton/atlas 内容不对：%+v", sp)
	}
	if len(sp.Textures) != 1 || !strings.HasPrefix(sp.Textures[0].DataURL, "data:image/png;base64,") {
		t.Fatalf("贴图应转成 dataURL：%+v", sp.Textures)
	}
	if sp.Animation != "idle" || !sp.Loop {
		t.Fatalf("默认动画不对：%+v", sp)
	}
	// 皮肤要下发（换装面板靠它）；atlas 的 pma 要读出来（否则边缘发黑）
	if len(sp.Skins) != 2 || sp.Skins[1] != "clothes/nude" {
		t.Fatalf("皮肤没下发：%+v", sp.Skins)
	}
	if !sp.PMA {
		t.Fatal("atlas 写了 pma:true，DTO 应为 true")
	}
}

// TestAtlasUsesPMA atlas 的 pma 标记要按原文读，不能猜。
func TestAtlasUsesPMA(t *testing.T) {
	if !atlasUsesPMA("a.png\nsize: 10,10\npma:true\n") {
		t.Fatal("pma:true 应识别为预乘")
	}
	if atlasUsesPMA("a.png\nsize: 10,10\n") {
		t.Fatal("没写 pma 时应为 false")
	}
	if !atlasUsesPMA("a.png\nsize: 10,10\nPMA: true\n") {
		t.Fatal("大小写/空格变体也应识别")
	}
}

func TestLoadSpineAssetsRejectsTraversal(t *testing.T) {
	cg := unitycg.CG{ID: "HCG02", Kind: "spine"}
	cg.Files.Text = []string{"../evil.json", "HCG_02.atlas"}
	if _, err := loadSpineAssets(t.TempDir(), cg); err == nil {
		t.Fatal("带 ../ 的文件名应被拒绝")
	}
}
