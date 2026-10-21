// irprobe：检查一个章节事件的 IR 结构（Show/Timeline/Texts 的 TMs 分布）。
// 用法：go run ./tools/irprobe <cgreplay.conf 路径> <scriptRel> <labelPos>
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"ucgr/internal/adapter/kirikiri"
)

func main() {
	confPath, script, pos := os.Args[1], os.Args[2], os.Args[3]
	var p int
	fmt.Sscan(pos, &p)
	b, err := os.ReadFile(confPath)
	if err != nil {
		fmt.Println("ERR 读 conf:", err)
		return
	}
	var conf struct {
		Root string `json:"root"`
	}
	if err := json.Unmarshal(b, &conf); err != nil {
		fmt.Println("ERR 解析 conf:", err)
		return
	}
	sp := filepath.Join(conf.Root, filepath.FromSlash(script))
	ev, err := kirikiri.BuildShowChapterFile(sp, p)
	if err != nil {
		fmt.Println("ERR:", err)
		return
	}
	fmt.Printf("Show=%d Timeline=%d Texts=%d Voices=%d Layers=%d\n",
		len(ev.Show), len(ev.Timeline), len(ev.Texts), len(ev.Voices), len(ev.Layers))
	maxShow, maxTl, maxTx := int64(-1), int64(-1), int64(-1)
	seen := map[string]int{}
	for _, op := range ev.Show {
		if op.TMs > maxShow {
			maxShow = op.TMs
		}
		seen[op.Kind]++
	}
	for _, op := range ev.Timeline {
		if op.TMs > maxTl {
			maxTl = op.TMs
		}
	}
	for _, t := range ev.Texts {
		if t.TMs > maxTx {
			maxTx = t.TMs
		}
	}
	fmt.Printf("maxShowTM=%d maxTimelineTMs=%d maxTextsTMs=%d\n", maxShow, maxTl, maxTx)
	fmt.Println("ShowOp 类型分布:", seen)
	// 打印前 8 个 Show op 的 TMs/Kind/要点
	for i, op := range ev.Show {
		if i >= 8 {
			break
		}
		fmt.Printf("  op[%d] TMs=%d Kind=%s Layer=%s Page=%s Ref=%s\n", i, op.TMs, op.Kind, op.Layer, op.Page, op.Ref)
		if op.Text != nil {
			fmt.Printf("        text: speaker=%q text=%q\n", op.Text.Speaker, op.Text.Text)
		}
	}
}
