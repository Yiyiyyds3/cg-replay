// reale2e_test.go —— 真机端到端（受环境变量控制，CI 默认跳过）。
//
// 跑的是 GUI 同款代码路径：Preview（UnpackPreview 用）→ 事件库（AddGame 用）→
// 构建演出（OpenEvent 用）。解包落盘默认**不做**（CS2 的依赖闭包约等于全量素材，
// 磁盘开销大），需要时设 UCGR_CS2_EXTRACT=1。
//
// 用法：
//
//	UCGR_CS2_DIR="<CS2 游戏原始目录>" go test ./internal/unpack -run TestRealCS2 -v
package unpack

import (
	"os"
	"testing"

	"ucgr/internal/adapter"
	"ucgr/internal/adapter/catsystem2"
	"ucgr/internal/library"
)

func TestRealCS2EndToEnd(t *testing.T) {
	root := os.Getenv("UCGR_CS2_DIR")
	if root == "" {
		t.Skip("未设置 UCGR_CS2_DIR，跳过真机语料")
	}
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		t.Fatalf("UCGR_CS2_DIR 不可用：%v", root)
	}

	// ① 预览（与 GUI「拖进来先看会解出什么」同一条路径）
	plan, err := Preview(root)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	t.Logf("预览：引擎=%s 归档=%d 剧本=%d 事件=%d 引用=%d 带 adlr=%d",
		plan.EngineID, len(plan.Archives), plan.ScenarioFiles, plan.Labels, plan.Refs, plan.Encrypted)
	for _, n := range plan.Notes {
		t.Logf("  note：%s", n)
	}
	if plan.EngineID != "catsystem2" {
		t.Fatalf("引擎应为 catsystem2，实际 %q", plan.EngineID)
	}
	if plan.ScenarioFiles == 0 {
		t.Fatalf("没认出剧本：%+v", plan)
	}

	// ② 直接从归档里取第一个能解析的 .cst，构建演出（OpenEvent 的那一步）
	deps, _ := engineFor("catsystem2")
	assets, err := openArchives(plan.Archives, deps.ParseArchive, deps.OpenReader)
	if err != nil {
		t.Fatalf("openArchives: %v", err)
	}
	defer assets.close()
	for _, sk := range assets.skipped {
		t.Logf("  跳过归档：%s", sk)
	}

	var built int
	for path, e := range assets.entries {
		if !deps.IsScript(path) {
			continue
		}
		data, err := assets.read(e)
		if err != nil {
			continue
		}
		ev, err := catsystem2.BuildShowScript(data, e.base())
		if err != nil {
			continue
		}
		kinds := map[string]int{}
		for _, op := range ev.Show {
			kinds[op.Kind]++
		}
		t.Logf("脚本 %s：Show %d 条 / 文本 %d 行 / 种类 %v", path, len(ev.Show), len(ev.Texts), kinds)
		if len(ev.Show) > 0 {
			built++
		}
		if built >= 3 {
			break
		}
	}
	if built == 0 {
		t.Fatal("没有脚本能构建出演出（Show 全空）")
	}

	// ③ 事件库（默认只验证「能从解包目录扫出来」的路径——这里用合成方式跳过落盘：
	//    直接检查 ScanCS2 在空目录上的报错文案，真实落盘走 UCGR_CS2_EXTRACT=1）
	if os.Getenv("UCGR_CS2_EXTRACT") == "1" {
		dest := t.TempDir()
		res, err := Extract(root, dest, ScopeCG)
		if err != nil {
			t.Fatalf("Extract: %v", err)
		}
		t.Logf("解包：%d 个文件 %.1f MB；事件 %d；跳过归档 %d",
			res.Files, float64(res.Bytes)/1048576, res.Events, len(res.Warnings))
		conv := adapter.Convention{EngineID: "catsystem2", ScenarioDir: "scene", ScriptExt: ".cst"}
		lib, err := library.ScanWith(dest, conv)
		if err != nil {
			t.Fatalf("ScanWith(cs2): %v", err)
		}
		t.Logf("事件库：%d 个事件", len(lib.Scenes))
		if len(lib.Scenes) == 0 {
			t.Fatal("事件库为空")
		}
	}
}
