package library

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ucgr/internal/adapter"
)

// scnRoot 真实 9nine 解包根（UCGR_9NINE_ROOT），未设置则跳过。
func scnRoot(t *testing.T) string {
	t.Helper()
	root := strings.TrimSpace(os.Getenv("UCGR_9NINE_ROOT"))
	if root == "" {
		t.Skip("未设置 UCGR_9NINE_ROOT，跳过真实语料测试")
	}
	return root
}

func TestScanSCNReal(t *testing.T) {
	root := scnRoot(t)
	conv := adapter.DefaultSCN()
	lib, err := ScanWith(root, conv)
	if err != nil {
		t.Fatalf("扫描失败：%v", err)
	}
	if len(lib.Scenes) == 0 {
		t.Fatal("场景事件为 0")
	}
	if len(lib.CGs) == 0 {
		t.Fatal("CG 事件为 0")
	}

	// 场景 id 必须是 <剧本名>#<序号>，且脚本文件真实存在。
	for _, s := range lib.Scenes {
		i := strings.LastIndex(s.ID, "#")
		if i <= 0 || i == len(s.ID)-1 {
			t.Fatalf("场景 id 形态不对：%q", s.ID)
		}
		p := filepath.Join(root, filepath.FromSlash(s.Script))
		if st, err := os.Stat(p); err != nil || st.IsDir() {
			t.Fatalf("场景 %s 的脚本不存在：%s", s.ID, s.Script)
		}
	}
	// CG 事件的 Pages 应已解析成相对路径（封面直接加载）。
	resolved := 0
	for _, c := range lib.CGs {
		for _, p := range c.Pages {
			if strings.Contains(p, "/") || strings.Contains(p, `\`) {
				resolved++
				break
			}
		}
	}
	if resolved == 0 {
		t.Error("CG 页序列一个都没解析成路径（素材目录约定可能不对）")
	}
	t.Logf("%s：scenes=%d cgs=%d；首场景=%s(%s)", lib.Game, len(lib.Scenes), len(lib.CGs),
		lib.Scenes[0].ID, lib.Scenes[0].Title)
}

func TestScanSCNFindAndBuild(t *testing.T) {
	root := scnRoot(t)
	conv := adapter.DefaultSCN()
	lib, err := ScanWith(root, conv)
	if err != nil {
		t.Fatal(err)
	}
	// FindScene 按 id 命中。
	if s := lib.FindScene(lib.Scenes[0].ID); s == nil {
		t.Fatalf("FindScene(%q) 未命中", lib.Scenes[0].ID)
	}
	// FindCG 支持缩略图名与纯数字两种输入。
	c := lib.CGs[0]
	if got := lib.FindCG(c.ID); got == nil {
		t.Fatalf("FindCG(%q) 未命中", c.ID)
	}
	if d := tailDigits(c.ID); d != "" {
		if got := lib.FindCG(d); got == nil {
			t.Errorf("FindCG(%q) 未命中 %s", d, c.ID)
		}
	}
}
