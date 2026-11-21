package audio

import (
	"os"
	"path/filepath"
	"testing"
)

// TestResolveRPGMV 加密语音扩展名必须在探测表里（V3 音频验收的关键一环）。
func TestResolveRPGMV(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "www", "audio", "bgm")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "calm.rpgmvo"), []byte("RPGMV"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &System{root: root}
	if got := s.resolve("calm", "bgm"); got == "" {
		t.Fatal("resolve 找不到 www/audio/bgm/calm.rpgmvo（加密语音探测缺失）")
	}
}
