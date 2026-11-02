package unpack

import (
	"strings"
	"testing"
)

// 宏家族脚本（Mai@KF 一系）→ 标准 KAG：只保留能演的部分，& 变量引用跳过。
func TestBuildWKSCompatTranslate(t *testing.T) {
	src := strings.Join([]string{
		"; 注释",
		"*ev_001",
		"[macImageDraw file=EH_A09_01 time=300]",
		"[macWait time=800]",
		"[macTrans time=400]",
		"[macPlayBgm file=bgm01.ogg]",
		"[macImageDraw file=&bg_main]",
		"[TransName]",
		"English line",
		"[TransHitret]",
		`[Talk name="A" text="hello"]`,
		"[hitret]",
		"[macPlaySe file=se01.ogg]",
	}, "\n")
	resolve := func(ref string) bool {
		switch strings.ToLower(ref) {
		case "eh_a09_01", "bgm01.ogg", "se01.ogg":
			return true
		}
		return false
	}
	out, warns := buildWKSCompat([]byte(src), "EV_001.wks", resolve)
	if out == nil {
		t.Fatalf("应生成兼容台本，warns=%v", warns)
	}
	got := string(out)
	for _, want := range []string{
		"*label01|EV_001",
		"[image storage=EH_A09_01]",
		"[wait time=800]",
		"[trans time=400]",
		"[playbgm storage=bgm01.ogg]",
		`[Talk name="A" text="hello"]`,
		"[l]\n[er]",
		"[playse storage=se01.ogg]",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("输出缺少 %q，实际：\n%s", want, got)
		}
	}
	for _, bad := range []string{"&bg_main", "TransName", "English line", "macImageDraw"} {
		if strings.Contains(got, bad) {
			t.Errorf("输出不应包含 %q，实际：\n%s", bad, got)
		}
	}
	// 下一行就是 macWait，不该再补 1200ms。
	if strings.Contains(got, "[wait time=1200]") {
		t.Errorf("已有 macWait 时不应补默认等待：\n%s", got)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "CG 图 1 张") || !strings.Contains(warns[0], "跳过 1 条") {
		t.Errorf("告警不符合预期：%v", warns)
	}
}

// 非宏家族脚本不生成兼容台本（原样交给现有构建器）。
func TestBuildWKSCompatNonMacro(t *testing.T) {
	out, warns := buildWKSCompat([]byte("*label01\n文本\n"), "plain.wks", func(string) bool { return true })
	if out != nil || warns != nil {
		t.Fatalf("非宏家族脚本应原样跳过，得到 out=%q warns=%v", out, warns)
	}
}

// 全是 & 变量引用时，不生成兼容台本，并如实告警（FR-12 不静默）。
func TestBuildWKSCompatNoResolvableImage(t *testing.T) {
	src := "[macImageDraw file=&bg_a]\n[macImageDraw file=&bg_b]\n"
	out, warns := buildWKSCompat([]byte(src), "VAR.wks", func(string) bool { return false })
	if out != nil {
		t.Fatalf("没有静态图不应生成台本，得到 %q", out)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "没有可静态解析的 CG 图") {
		t.Fatalf("缺少「无静态图」告警：%v", warns)
	}
}

// 同名脚本跨归档去重：append 补丁层优先，且告警写明忽略了谁。
func TestPickScriptsDedupe(t *testing.T) {
	s := &archiveSet{
		entries: map[string]archiveEntry{
			"00/AAA.wks": {path: "00/AAA.wks"},
			"AAA.wks":    {path: "AAA.wks"},
			"00/BBB.wks": {path: "00/BBB.wks"},
			"00/CCC.txt": {path: "00/CCC.txt"},
		},
		owner: map[string]string{
			"00/AAA.wks": `E:\g\scenario.xp3`,
			"AAA.wks":    `E:\g\append.xp3`,
			"00/BBB.wks": `E:\g\scenario.xp3`,
			"00/CCC.txt": `E:\g\scenario.xp3`,
		},
	}
	isScript := func(p string) bool { return strings.HasSuffix(strings.ToLower(p), ".wks") }
	got, warns := pickScripts(s, isScript)
	if len(got) != 2 {
		t.Fatalf("期望 2 个脚本（AAA 去重 + BBB），得到 %d：%+v", len(got), got)
	}
	if got[0].path != "AAA.wks" {
		t.Errorf("AAA 应取 append 副本，得到 %s", got[0].path)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "append.xp3:AAA.wks") {
		t.Errorf("去重告警不符合预期：%v", warns)
	}
}
