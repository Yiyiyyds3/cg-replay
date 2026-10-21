// replayloop：脱离 Wails 桥复现回放会话的压力形态，定位 mutex 长持。
// 用法：go run ./tools/replayloop <root|.conf> <script> <pos> <secs>
//   pos>=0 用 BuildShowChapterFile(sp,pos)；-1 用 BuildShowSegment(sp,"")。
// 节奏：60/main 毫秒一次「Advance(墙钟上限200ms)+RenderFrame(1600×1200)+JPEG编码」，
// 每 2500ms 在 waiting 时注入一次点击推进（与壳层 auto 一致）。
// 输出：每步耗时、>500ms 步打点、结束时统计 max/p99/持锁最长。
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"ucgr/internal/adapter/kirikiri"
	"ucgr/internal/audio"
	"ucgr/internal/ir"
	"ucgr/internal/player"
	"ucgr/internal/replay"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("usage: replayloop <root|.conf> <script> <pos> [secs]")
		os.Exit(2)
	}
	root := os.Args[1]
	if strings.HasSuffix(root, ".conf") {
		r := readConfRoot(root)
		if r == "" {
			os.Exit(2)
		}
		root = r
	}
	script, pos := os.Args[2], atoi(os.Args[3])
	secs := 60
	if len(os.Args) > 4 {
		secs = atoi(os.Args[4])
	}

	sp := filepath.Join(root, filepath.FromSlash(script))
	var ev, err = Build(sp, pos)
	if err != nil {
		fmt.Println("ERR build:", err)
		os.Exit(1)
	}
	fmt.Printf("Show=%d ops, Lines/Texts=%d Layers=%d\n", len(ev.Show), len(ev.Texts), len(ev.Layers))
	opt := player.DefaultOptions()
	opt.FontSize = 48
	load := player.Cached(player.Loader(replay.GameLoader(root)))
	p := player.New(ev, opt)
	audioSys, err := audio.NewSystem(root)
	if err != nil {
		fmt.Printf("audio err: %v\n", err)
	} else {
		p.Audio = audioSys.Handle
	}
	defer audioSys.Close()
	p.Advance(0, player.Input{})

	var mu sync.Mutex
	last := time.Now()
	waited, waitedAt := false, time.Now()
	maxMs, encMs, compMs := time.Duration(0), time.Duration(0), time.Duration(0)
	dur := time.Duration(secs) * time.Second
	deadline := time.Now().Add(dur)
	n := 0
	for time.Now().Before(deadline) {
		t0 := time.Now()
		dt := t0.Sub(last)
		if dt > 200*time.Millisecond {
			dt = 200 * time.Millisecond
		}
		last = t0
		auto := false
		if waited && t0.Sub(waitedAt) > 2500*time.Millisecond {
			auto = true
			waitedAt = t0
		}
		mu.Lock() // ===== 锁内：推进 + 合成 + 编码（与 replay.go 同形态） =====
		in := player.Input{}
		if auto {
			in.Advance = true
		}
		if dt > 0 || auto {
			p.Advance(dt.Milliseconds(), in)
		}
		f := p.Frame()
		if waited != f.Waiting {
			waited, waitedAt = f.Waiting, t0
		}
		adv := time.Since(t0)
		t1 := time.Now()
		rgba, _, err := player.RenderFrame(p, load, 1600, 1200)
		comp := time.Since(t1)
		t2 := time.Now()
		_ = rgba
		_ = err
		// JPEG 编码也计入（replay.go 的 renderJPEG）
		_ = encodeJ(rgba, 88)
		enc := time.Since(t2)
		total := time.Since(t0)
		mu.Unlock()
		if total > maxMs {
			maxMs = total
		}
		if adv+comp > encMs {
			encMs = adv + comp
		}
		if comp > compMs {
			compMs = comp
		}
		if n%50 == 0 || total > 500*time.Millisecond {
			fmt.Printf("#%04d now=%d adv=%s comp=%s enc=%s total=%s layers=%d waiting=%v done=%v\n",
				n, f.Now, adv.Round(3), comp.Round(3), enc.Round(3), total.Round(3), len(f.Layers), f.Waiting, f.Done)
		}
		n++
		time.Sleep(60 * time.Millisecond)
	}
	fmt.Printf("iters=%d max=%s (adv+comp p?=%s comp=%s)\n", n, maxMs.Round(3), encMs.Round(3), compMs.Round(3))
}

func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

func readConfRoot(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Println("ERR conf:", err)
		return ""
	}
	var c struct {
		Root string `json:"root"`
	}
	if err := parseConf(data, &c); err != nil {
		fmt.Println("ERR conf:", err)
		return ""
	}
	return strings.TrimSpace(c.Root)
}

func parseConf(data []byte, v any) error {
	// tiny wrapper: avoid importing encoding/json twice —
	return json.Unmarshal(data, v)
}

func Build(sp string, pos int) (*ir.Event, error) {
	if pos >= 0 {
		return kirikiri.BuildShowChapterFile(sp, pos)
	}
	return kirikiri.BuildShowSegment(sp, "")
}

func encodeJ(img image.Image, q int) error {
	var buf bytes.Buffer
	return jpeg.Encode(&buf, img, &jpeg.Options{Quality: q})
}
