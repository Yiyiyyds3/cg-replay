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
		"HCG_02.atlas": "HCG_02.png\nsize: 1024,1024\n",
		"HCG_02.png":   "fake-png",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cg := unitycg.CG{ID: "HCG02", Kind: "spine"}
	cg.Files.Text = []string{"HCG_02.json", "HCG_02.atlas"}
	cg.Files.PNG = []string{"HCG_02.png"}
	cg.Spine = &unitycg.SpineInfo{Animations: []unitycg.AnimRef{
		{Name: "idle", Loop: true, Dur: 1.5},
	}}

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
}

func TestLoadSpineAssetsRejectsTraversal(t *testing.T) {
	cg := unitycg.CG{ID: "HCG02", Kind: "spine"}
	cg.Files.Text = []string{"../evil.json", "HCG_02.atlas"}
	if _, err := loadSpineAssets(t.TempDir(), cg); err == nil {
		t.Fatal("带 ../ 的文件名应被拒绝")
	}
}
