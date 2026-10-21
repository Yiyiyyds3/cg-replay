package main

import (
	"os"
	"path/filepath"
	"testing"

	"ucgr/internal/catalog"
	"ucgr/internal/registry"
)

func TestDataDir(t *testing.T) {
	// 有 LOCALAPPDATA → 其下 cgreplay-ui（且被创建）。
	tmp := t.TempDir()
	t.Setenv("LOCALAPPDATA", tmp)
	t.Setenv("APPDATA", filepath.Join(tmp, "roaming"))
	got := dataDir()
	want := filepath.Join(tmp, "cgreplay-ui")
	if got != want {
		t.Fatalf("dataDir() = %q, want %q", got, want)
	}
	if st, err := os.Stat(got); err != nil || !st.IsDir() {
		t.Fatalf("dataDir 目录未创建：%v", err)
	}

	// 没有 LOCALAPPDATA → 回退 APPDATA。
	t.Setenv("LOCALAPPDATA", "")
	got = dataDir()
	want = filepath.Join(tmp, "roaming", "cgreplay-ui")
	if got != want {
		t.Fatalf("回退 APPDATA 失败：%q, want %q", got, want)
	}

	// 两个都没有 → 回退 exeDir（老行为，不丢数据）。
	t.Setenv("APPDATA", "")
	if got := dataDir(); got != exeDir() {
		t.Fatalf("双无时应回退 exeDir，got %q", got)
	}
}

func TestMigrateLegacyData(t *testing.T) {
	from := filepath.Join(t.TempDir(), "bin")       // 老 exe 目录
	to := filepath.Join(t.TempDir(), "cgreplay-ui") // 新 dataDir

	// 老目录造点数据：单文件 + 嵌套目录 + 与迁移无关的文件（不该被搬）。
	write := func(p, content string) {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(from, confName), `{"root":"E:\\game"}`)
	write(filepath.Join(from, registry.FileName), `{"active":"g1"}`)
	write(filepath.Join(from, catalog.FileName), "SQLite format 3")
	write(filepath.Join(from, catalog.FileName+"-wal"), "waldata")
	write(filepath.Join(from, "libraries", "g1.json"), "{}")
	write(filepath.Join(from, "libraries", "sub", "x.bin"), "deep")
	write(filepath.Join(from, "covers", "g1", "cg_cg001.png"), "png")
	write(filepath.Join(from, "exports", "cg_cg001", "a.gif"), "gif")
	write(filepath.Join(from, "无关文件.txt"), "keep me")   // 不在名单里，不搬
	write(filepath.Join(from, "cgreplay-ui.exe"), "MZ") // exe 自己，不搬

	migrateLegacyData(from, to)

	// 名单内文件全部到位。
	for _, rel := range []string{
		confName,
		registry.FileName,
		catalog.FileName,
		catalog.FileName + "-wal",
		filepath.Join("libraries", "g1.json"),
		filepath.Join("libraries", "sub", "x.bin"),
		filepath.Join("covers", "g1", "cg_cg001.png"),
		filepath.Join("exports", "cg_cg001", "a.gif"),
	} {
		if _, err := os.Stat(filepath.Join(to, rel)); err != nil {
			t.Fatalf("应迁移 %s：%v", rel, err)
		}
	}
	// 名单外文件不搬（含 exe 自己）。
	for _, rel := range []string{"无关文件.txt", "cgreplay-ui.exe"} {
		if _, err := os.Stat(filepath.Join(to, rel)); err == nil {
			t.Fatalf("不该迁移 %s", rel)
		}
	}
	// 老目录不删（只搬不删，留着当备份）。
	if _, err := os.Stat(filepath.Join(from, confName)); err != nil {
		t.Fatal("老目录数据被删了——违反只搬不删")
	}
	// 内容一致。
	b, err := os.ReadFile(filepath.Join(to, confName))
	if err != nil || string(b) != `{"root":"E:\\game"}` {
		t.Fatalf("迁移内容不符：%q %v", b, err)
	}

	// 目标已存在 → 不覆盖（幂等，重复启动安全）。
	if err := os.WriteFile(filepath.Join(to, confName), []byte(`{"root":"D:\\other"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	migrateLegacyData(from, to)
	b, _ = os.ReadFile(filepath.Join(to, confName))
	if string(b) != `{"root":"D:\\other"}` {
		t.Fatalf("重复迁移覆盖了用户数据：%q", b)
	}

	// from == to → no-op（不自己复制自己）。
	migrateLegacyData(to, to)
	if _, err := os.Stat(filepath.Join(to, confName)); err != nil {
		t.Fatal("from==to 时数据没了")
	}
}
