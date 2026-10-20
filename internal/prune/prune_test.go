package prune

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildPoolWithExtra(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "原始归档解包")
	extra := filepath.Join(base, "音频")
	if err := os.MkdirAll(filepath.Join(root, "image00"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(extra, "语音_AA"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "image00", "bg01.hg3"), []byte("img"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(extra, "语音_AA", "v001.ogg"), []byte("voice"), 0o644); err != nil {
		t.Fatal(err)
	}

	p, err := BuildPoolWithExtra(root, extra)
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Match("v001"); len(got) != 1 || filepath.ToSlash(got[0]) != "../音频/语音_aa/v001.ogg" {
		t.Fatalf("外部音频根未入池: %#v", got)
	}
	if got := p.Match("bg01"); len(got) != 1 || filepath.ToSlash(got[0]) != "image00/bg01.hg3" {
		t.Fatalf("主素材根匹配错: %#v", got)
	}
}
