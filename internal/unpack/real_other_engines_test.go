package unpack

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"ucgr/internal/adapter/artemis"
	"ucgr/internal/adapter/bgi"
)

// TestRealArtemis 真机：某一批 .ast 全部能解析，且能构建出演出台本。
//
//	UCGR_AST_DIR="<含 .ast 的目录>" go test ./internal/unpack -run TestRealArtemis -v
func TestRealArtemis(t *testing.T) {
	dir := os.Getenv("UCGR_AST_DIR")
	if dir == "" {
		t.Skip("未设置 UCGR_AST_DIR")
	}
	files := globScripts(t, dir, ".ast")
	if len(files) == 0 {
		t.Skip("目录里没有 .ast")
	}
	var okN, failN, showN, ops int
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			failN++
			continue
		}
		ev, err := artemis.BuildShowBlock(data, filepath.Base(f), 1)
		if err != nil {
			if failN == 0 {
				t.Logf("首个失败（%s）：%v", filepath.Base(f), err)
			}
			failN++
			continue
		}
		okN++
		if len(ev.Show) > 0 {
			showN++
			ops += len(ev.Show)
		}
	}
	t.Logf("Artemis：%d 个 .ast，解析成功 %d，失败 %d；有台本的 %d 个，Show 合计 %d 条（均值 %.0f）",
		len(files), okN, failN, showN, ops, float64(ops)/float64(maxOne(showN)))
	if okN == 0 {
		t.Fatal("没有一个 .ast 能解析")
	}
}

// TestRealBGI 真机：某一批 .bs 能被 builder 切出事件并产出台本。
//
//	UCGR_BS_DIR="<含 .bs 的目录>" go test ./internal/unpack -run TestRealBGI -v
func TestRealBGI(t *testing.T) {
	dir := os.Getenv("UCGR_BS_DIR")
	if dir == "" {
		t.Skip("未设置 UCGR_BS_DIR")
	}
	files := globScripts(t, dir, ".bs")
	if len(files) == 0 {
		t.Skip("目录里没有 .bs")
	}
	var okN, failN, showN, ops int
	for _, f := range files {
		ev, err := bgi.BuildShowEventFile(f, 1)
		if err != nil {
			if failN == 0 {
				t.Logf("首个失败（%s）：%v", filepath.Base(f), err)
			}
			failN++
			continue
		}
		okN++
		if len(ev.Show) > 0 {
			showN++
			ops += len(ev.Show)
		}
	}
	t.Logf("BGI：%d 个 .bs，构建成功 %d，失败 %d；有台本的 %d 个，Show 合计 %d 条（均值 %.0f）",
		len(files), okN, failN, showN, ops, float64(ops)/float64(maxOne(showN)))
	if okN == 0 {
		t.Fatal("没有一个 .bs 能构建出演出")
	}
}

func globScripts(t *testing.T, dir, ext string) []string {
	t.Helper()
	var out []string
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(p), ext) {
			out = append(out, p)
		}
		return nil
	})
	sort.Strings(out)
	if len(out) > 40 { // 抽样即可，跑得快
		out = out[:40]
	}
	return out
}

func maxOne(n int) int {
	if n <= 0 {
		return 1
	}
	return n
}
