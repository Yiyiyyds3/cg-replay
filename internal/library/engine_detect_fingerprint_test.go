package library

import (
	"os"
	"path/filepath"
	"testing"
)

// buildFingerprintDir 建一个临时目录并写入给定文件（/ 分隔）与空目录。
func buildFingerprintDir(t *testing.T, files, dirs []string) string {
	t.Helper()
	root := t.TempDir()
	for _, name := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range dirs {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(name)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// TestDetectEngineUnsupportedFingerprints 每个新指纹一个正例：只认不处理，
// DetectEngine 要报出引擎名而不是「未知」。
func TestDetectEngineUnsupportedFingerprints(t *testing.T) {
	cases := []struct {
		want  string
		files []string
		dirs  []string
	}{
		{"alicesoft", []string{"DataPack.afa", "Syscom.ain", "BGI.ald"}, nil},
		{"yuris", []string{"data.ypf", "script.ybn"}, nil},
		{"siglus", []string{"scene.pck", "graphic.pak"}, nil},
		{"siglus", []string{"Graphic.pak", "SiglusEngine.exe"}, nil},
		{"qlie", []string{"data.pack", "filechk.exe"}, nil},
		{"majiro", []string{"script.mjo"}, nil},
		{"reallive", []string{"bg001.g00", "voice.nwa"}, nil},
		{"advhd", []string{"scenario.ws2"}, nil},
		{"cmvs", []string{"data.ps2", "bg.pb3", "se.cpz"}, nil},
		{"livemaker", []string{"Data/lsb/scene01.lsb"}, nil},
		{"wolf", []string{"Data.wolf"}, nil},
		{"wolf", []string{"patch01.wolf"}, nil},
		{"rmxp", []string{"Game.ini", "Data/Actors.rxdata"}, []string{"Data"}},
		{"rmxp", []string{"Game.rgss3a"}, nil},
		{"eushully", []string{"DATA000.alf"}, nil},
		{"unreal", []string{"Game/Content/Paks/Game.pak"}, []string{"Game/Content"}},
		{"unreal", []string{"Game.utoc", "Game.ucas", "Game.pak"}, nil},
	}
	for _, c := range cases {
		root := buildFingerprintDir(t, c.files, c.dirs)
		if got := DetectEngine(root); got != c.want {
			t.Errorf("%v → DetectEngine = %q, want %q", c.files, got, c.want)
		}
	}
}

// TestDetectEngineFingerprintsDontStealSupported 现有已支持引擎的目录
// 不能被新指纹抢走：kirikiri(.xp3 剧本树)、rmmz(www/data)、unity(导出库)
// 三个反例必须仍然判回原引擎。
func TestDetectEngineFingerprintsDontStealSupported(t *testing.T) {
	// kirikiri：data/scenario/*.ks。
	kk := buildFingerprintDir(t,
		[]string{"data/scenario/a01.ks", "data.xp3"}, nil)
	if got := DetectEngine(kk); got != "kirikiri" {
		t.Errorf("kirikiri 目录被认成 %q, want kirikiri", got)
	}
	// rmmz：www/data/System.json。
	rmmz := buildFingerprintDir(t,
		[]string{"www/data/System.json"}, []string{"www/data"})
	if got := DetectEngine(rmmz); got != "rmmz" {
		t.Errorf("rmmz 目录被认成 %q, want rmmz", got)
	}
	// unity：导出 CG 库（cg-index.json + cg/）。
	uni := buildFingerprintDir(t,
		[]string{"cg-index.json", "cg/cg001/img.png"}, []string{"cg/cg001"})
	if got := DetectEngine(uni); got != "unity" {
		t.Errorf("unity 导出库被认成 %q, want unity", got)
	}
	// bgi：裸 .arc 目录在改动前就认不出（adapter 要解包物，PackedEngineOf
	// 才认 BGI.hvl）。这里验证的是新指纹不把 .arc 抢成 majiro 等新引擎，
	// 仍保持既有行为 ""，打包签名仍归 bgi。
	bgi := buildFingerprintDir(t,
		[]string{"BGI.hvl", "BGI.sdb", "data.arc"}, nil)
	if got := DetectEngine(bgi); got != "" {
		t.Errorf("bgi 裸目录被认成 %q, want \"\"（既有行为，新指纹不得抢 .arc）", got)
	}
	if got, _ := PackedEngineOf(bgi); got != "bgi" {
		t.Errorf("bgi 裸目录 PackedEngineOf = %q, want bgi", got)
	}
}

// TestDetectEngineFingerprintEmpty 空目录与陌生文件不触发任何新指纹。
func TestDetectEngineFingerprintEmpty(t *testing.T) {
	if got := DetectEngine(t.TempDir()); got != "" {
		t.Errorf("空目录 DetectEngine = %q, want \"\"", got)
	}
	root := buildFingerprintDir(t, []string{"readme.txt", "说明.txt"}, nil)
	if got := DetectEngine(root); got != "" {
		t.Errorf("陌生目录 DetectEngine = %q, want \"\"", got)
	}
}
