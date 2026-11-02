package unpack

import (
	"os"
	"path/filepath"
	"testing"
)

// TestExtractRealKiriKiri 用真机 XP3 跑一遍最小集合解包（UCGR_KF_ROOT 指向游戏根）。
func TestExtractRealKiriKiri(t *testing.T) {
	root := os.Getenv("UCGR_KF_ROOT")
	if root == "" {
		t.Skip("未设置 UCGR_KF_ROOT，跳过真机语料")
	}
	p, err := Preview(root)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	t.Logf("plan: engine=%s kind=%s archives=%d scripts=%d labels=%d refs=%d enc=%d notes=%v reason=%s",
		p.EngineID, p.Kind, len(p.Archives), p.ScenarioFiles, p.Labels, p.Refs, p.Encrypted, p.Notes, p.Reason)
	dest := os.Getenv("UCGR_KF_OUT")
	if dest == "" {
		dest = filepath.Join(t.TempDir(), "out")
	}
	res, err := ExtractWith(root, dest, Options{Scope: ScopeCG})
	if err != nil {
		t.Fatalf("ExtractWith: %v", err)
	}
	t.Logf("result: dest=%s events=%d files=%d bytes=%d missing=%d encrypted=%d",
		res.Dest, res.Events, res.Files, res.Bytes, len(res.Missing), len(res.Encrypted))
	t.Logf("missing refs: %v", res.Missing)
	t.Logf("warnings: %v", res.Warnings)
	filepath.Walk(dest, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			rel, _ := filepath.Rel(dest, path)
			t.Logf("  %s (%d)", rel, info.Size())
		}
		return nil
	})
}
