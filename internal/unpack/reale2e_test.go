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
	"image"
	"os"
	"path/filepath"
	"testing"

	"ucgr/internal/adapter"
	"ucgr/internal/adapter/catsystem2"
	"ucgr/internal/library"
	"ucgr/internal/player"
	"ucgr/internal/replay"
	"ucgr/internal/video"
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

// TestRealCS2UnpackedPlayback 在**已解包**的 CS2 目录上验证「真能播」。
//
// 不需要重新解包：直接用别人解好的树（原生布局，root 取含 scene/ 与 image00/ 的那层）。
// 流程与 GUI 打开事件一致：事件库 → BuildShowScript → player.Advance → RenderFrame。
//
//	UCGR_CS2_UNPACKED="<含 scene/、image00/ 的目录>" go test ./internal/unpack -run TestRealCS2Unpacked -v
func TestRealCS2UnpackedPlayback(t *testing.T) {
	root := os.Getenv("UCGR_CS2_UNPACKED")
	if root == "" {
		t.Skip("未设置 UCGR_CS2_UNPACKED，跳过真机播放验证")
	}
	conv := adapter.Convention{EngineID: "catsystem2", ScenarioDir: "scene", ScriptExt: ".cst"}
	lib, err := library.ScanWith(root, conv)
	if err != nil {
		t.Fatalf("ScanWith: %v", err)
	}
	t.Logf("事件库：%d 个事件", len(lib.Scenes))

	load := player.Cached(player.Loader(replay.GameLoader(root)))
	// 与桌面端同一条路径：流式解码（一条 ffmpeg 进程连续出帧）
	streams := map[string]*video.Stream{}
	defer func() {
		for _, st := range streams {
			_ = st.Close()
		}
	}()
	videoProv := func(ref string, tMs int64) (image.Image, error) {
		pth, err := replay.AssetPath(root, ref)
		if err != nil {
			return nil, err
		}
		st := streams[pth]
		if st == nil {
			st, err = video.OpenStream(pth, 1280, 720, 20)
			if err != nil {
				return nil, err
			}
			streams[pth] = st
		}
		return st.Frame(tMs)
	}
	var rendered, blank, tried int
	for i := 0; i < len(lib.Scenes) && tried < 30; i++ {
		sc := lib.Scenes[i]
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(sc.Script)))
		if err != nil {
			continue
		}
		ev, err := catsystem2.BuildShowScript(data, filepath.Base(sc.Script))
		if err != nil || len(ev.Show) == 0 {
			continue
		}
		// **不能**用 SkipWaits：它会把脚本一口气推到末尾，采到的永远是终态
		//（真机上就因此把「结尾黑幕」误判成整段黑屏）。改为正常推进 + 把
		// 文字阅读时间压到最低，按时间步进采样，取全程最大可见度。
		opt := player.DefaultOptions()
		opt.CharMs, opt.BaseMs, opt.MinWaitMs, opt.MaxWaitMs = 1, 1, 1, 1
		// 影片时长（台本里不写）——与桌面端一致，用 ffprobe 查
		opt.VideoDur = func(ref string) int64 {
			pth, err := replay.AssetPath(root, ref)
			if err != nil {
				return 0
			}
			return video.Duration(pth)
		}
		p := player.New(ev, opt)
		best := 0.0
		for step := 0; step < 12000; step++ {
			p.Advance(16, player.Input{})
			if step%100 == 0 || p.Done() {
				if img, _, err := player.RenderFrameWithVideo(p, load, videoProv, 1280, 720); err == nil {
					if r := nonBlackRatio(img); r > best {
						best = r
					}
				}
			}
			if p.Done() {
				break
			}
		}
		_, missing, err := player.RenderFrameWithVideo(p, load, videoProv, 1280, 720)
		if err != nil {
			t.Logf("%s: 渲染失败 %v", sc.ID, err)
			continue
		}
		ratio := best
		if ratio < 0.02 {
			for _, ls := range p.Frame().Layers {
				t.Logf("    层 %s ref=%s resolved=%q vis=%v x=%.0f y=%.0f op=%.2f",
					ls.Layer, ls.Ref, ls.Resolved, ls.Visible, ls.X, ls.Y, ls.Opacity)
			}
		}
		sample := missing
		if len(sample) > 3 {
			sample = sample[:3]
		}
		t.Logf("%s: Show %d 条，非黑像素 %.1f%%，缺素材 %d %v", sc.ID, len(ev.Show), ratio*100, len(missing), sample)
		tried++
		if ratio > 0.02 {
			rendered++
		} else {
			blank++
		}
	}
	t.Logf("统计：试了 %d 个事件，能画出画面 %d，全黑 %d", tried, rendered, blank)
	if rendered == 0 {
		t.Fatal("没有任何事件能画出画面（非黑像素 < 2%）")
	}
}

// nonBlackRatio 统计非黑像素比例（每 2 像素采一个）。
func nonBlackRatio(img *image.RGBA) float64 {
	var nonBlack, total int
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y += 2 {
		for x := b.Min.X; x < b.Max.X; x += 2 {
			r, g, bl, _ := img.At(x, y).RGBA()
			total++
			if r > 0x1000 || g > 0x1000 || bl > 0x1000 {
				nonBlack++
			}
		}
	}
	if total == 0 {
		return 0
	}
	return float64(nonBlack) / float64(total)
}
