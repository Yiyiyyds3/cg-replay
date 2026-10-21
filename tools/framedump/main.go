// framedump：把一个事件的 n 个采样点 Compose 后做像素 SHA-256 清单。
// 用途：内核渲染优化前后 diff=0 回归（逐位一致验证）。
// 用法：go run ./tools/framedump <root|conf> <script> <pos> <label|-> <n>
//   pos>=0   → BuildShowChapterFile(sp, pos)   // 演出章节
//   pos ==-1 → BuildShowSegment(sp, "")        // 主线场景
//   pos ==-2 → BuildShowSegment(sp, label)     // CG
//   第一参数以 .conf 结尾时从配置文件读 root（避免控制台编码问题）。
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"ucgr/internal/adapter/kirikiri"
	"ucgr/internal/ir"
	"ucgr/internal/player"
	"ucgr/internal/replay"
)

func main() {
	root := os.Args[1]
	if strings.HasSuffix(root, ".conf") {
		b, err := os.ReadFile(root)
		if err != nil {
			fmt.Println("ERR conf:", err)
			os.Exit(1)
		}
		var c struct {
			Root string `json:"root"`
		}
		if err := json.Unmarshal(b, &c); err != nil {
			fmt.Println("ERR conf:", err)
			os.Exit(1)
		}
		root = c.Root
	}
	script, posS, label, nS := os.Args[2], os.Args[3], os.Args[4], os.Args[5]
	pos, _ := strconv.Atoi(posS)
	n, _ := strconv.Atoi(nS)
	sp := filepath.Join(root, filepath.FromSlash(script))

	var ev *ir.Event
	var err error
	switch {
	case pos >= 0:
		ev, err = kirikiri.BuildShowChapterFile(sp, pos)
	case pos == -1:
		ev, err = kirikiri.BuildShowSegment(sp, "")
	default:
		ev, err = kirikiri.BuildShowSegment(sp, label)
	}
	if err != nil {
		fmt.Println("ERR build:", err)
		os.Exit(1)
	}

	load := player.Cached(player.Loader(replay.GameLoader(root)))
	// 采样点：Show 台本的 TMs 去重升序（ kirikiri 系事件 Timeline/Texts 为空，
	// SampleTimes 只会给 {0}），取均匀子样至多 n 个，必含末值。
	var ts []int64
	seen := map[int64]bool{}
	for _, op := range ev.Show {
		if !seen[op.TMs] {
			seen[op.TMs] = true
			ts = append(ts, op.TMs)
		}
	}
	sort.Slice(ts, func(i, j int) bool { return ts[i] < ts[j] })
	if len(ts) > n {
		stride := int64(len(ts)) / int64(n)
		sub := make([]int64, 0, n)
		for i := int64(0); i < int64(n); i++ {
			sub = append(sub, ts[i*stride])
		}
		if sub[len(sub)-1] != ts[len(ts)-1] {
			sub[len(sub)-1] = ts[len(ts)-1] // 末值必含
		}
		ts = sub
	}
	// 走 player 路径（确定性步进推进到目标 t 再 RenderFrame）：
	// Show 系事件的层状态源是播放器，不是 StatesAt（Timeline 为空）。
	opt := player.DefaultOptions()
	opt.FontSize = 48
	p := player.New(ev, opt)
	p.Advance(0, player.Input{})
	const step = int64(25)
	for i, t := range ts {
		for p.Now() < t && !p.Done() {
			d := t - p.Now()
			if d > step {
				d = step
			}
			p.Advance(d, player.Input{})
		}
		sts := p.Frame()
		img, drawn, err := player.RenderFrame(p, load, 0, 0)
		if err != nil {
			fmt.Printf("%d\t%d\tERR\t%v\n", i, t, err)
			continue
		}
		sum := sha256.Sum256(img.Pix)
		fmt.Printf("%d\t%d\t%s\t%dx%d\tlayers=%d\tboxes=%d\twaiting=%v\n", i, t,
			hex.EncodeToString(sum[:]),
			img.Bounds().Dx(), img.Bounds().Dy(), len(drawn), len(sts.Texts), sts.Waiting)
	}
}
