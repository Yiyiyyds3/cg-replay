package unpack

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// TestFindArchivesDepth 校验 findArchives 的下钻层数：根 + 两级子目录，第三级不要。
func TestFindArchivesDepth(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{
		"a.xp3",
		"one/b.xp3",
		"one/two/c.xp3",
		"one/two/three/d.xp3", // 超过 maxArchiveDepth，必须不出现
		"one/readme.txt",
	} {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := findArchives(root, "kirikiri")
	if err != nil {
		t.Fatalf("findArchives: %v", err)
	}
	var rels []string
	for _, p := range got {
		r, _ := filepath.Rel(root, p)
		rels = append(rels, filepath.ToSlash(r))
	}
	sort.Strings(rels)
	want := []string{"a.xp3", "one/b.xp3", "one/two/c.xp3"}
	if strings.Join(rels, ",") != strings.Join(want, ",") {
		t.Fatalf("归档集不对：got %v，want %v", rels, want)
	}
}

// TestFindArchivesJunction 校验联接子目录能被下钻。
//
// 实测背景：Windows 目录联接在 os.ReadDir 的条目里 IsDir() 报 false（Type 也不带
// ModeSymlink），只靠 d.IsDir() 会把整片联接目录漏掉；findArchives 必须 Stat 兜底。
func TestFindArchivesJunction(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("目录联接是 Windows 行为，其他平台跳过")
	}
	base := t.TempDir()
	target := filepath.Join(base, "target")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "j.xp3"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(base, "root")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	cmd := exec.Command("cmd", "/c", "mklink", "/J", link, target)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("建联接失败（可能是权限/文件系统限制）：%v %s", err, out)
	}
	got, err := findArchives(root, "kirikiri")
	if err != nil {
		t.Fatalf("findArchives: %v", err)
	}
	found := false
	for _, p := range got {
		if strings.EqualFold(filepath.Base(p), "j.xp3") {
			found = true
		}
	}
	if !found {
		t.Fatalf("联接子目录里的归档没被找到：got %v", got)
	}
}
