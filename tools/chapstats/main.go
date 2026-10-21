// chapstats：在真实解包目录上核验「章节跨文件首段归属」。
//
// 输出：章节总数、带接续的章节、接续明细、以及若有章前段却没挂上的孤例。
// 用法：go run ./tools/chapstats <root|.conf>
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"ucgr/internal/library"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("用法: go run ./tools/chapstats <root|.conf>")
		return
	}
	root := os.Args[1]
	if strings.HasSuffix(strings.ToLower(root), ".conf") {
		data, err := os.ReadFile(root)
		if err != nil {
			fmt.Println("ERR:", err)
			return
		}
		var c struct{ Root string }
		if err := json.Unmarshal(data, &c); err != nil {
			fmt.Println("ERR:", err)
			return
		}
		root = c.Root
	}
	lib, err := library.Scan(root)
	if err != nil {
		fmt.Println("ERR:", err)
		return
	}
	// go run ./tools/chapstats <root|.conf> --save <out.json>
	if len(os.Args) >= 4 && os.Args[2] == "--save" {
		if err := lib.Save(os.Args[3]); err != nil {
			fmt.Println("ERR: 保存失败", err)
			return
		}
		fmt.Println("已写出", os.Args[3])
	}
	fmt.Printf("场景 %d / 章节 %d\n", len(lib.Scenes), len(lib.Chapters))
	cont := 0
	for _, ch := range lib.Chapters {
		if ch.ContScript != "" {
			cont++
			fmt.Printf("接续：%-22s (%s#%d) ← %s\n", ch.ID, ch.Scene, ch.LabelPos, ch.ContScript)
		}
	}
	fmt.Printf("带接续的章节 %d 个\n", cont)

	// 抽样验证：合并事件 = 本章 + 章前段，TMs 仍单调。
	for i := range lib.Chapters {
		ch := &lib.Chapters[i]
		if ch.ContScript == "" {
			continue
		}
		base, err := library.BuildChapterEvent(root, &library.ChapterEvent{
			ID: ch.ID, Index: ch.Index, Title: ch.Title, Script: ch.Script,
			Scene: ch.Scene, LabelPos: ch.LabelPos, Name: ch.Name,
		})
		if err != nil {
			fmt.Println("基准构建失败", ch.ID, err)
			continue
		}
		merged, err := library.BuildChapterEvent(root, ch)
		if err != nil {
			fmt.Println("合并构建失败", ch.ID, err)
			continue
		}
		mono := true
		for k := 1; k < len(merged.Show); k++ {
			if merged.Show[k].TMs < merged.Show[k-1].TMs {
				mono = false
				break
			}
		}
		fmt.Printf("样本 %-18s base=%-4d merged=%-4d (+%d) TMs单调=%v\n",
			ch.ID, len(base.Show), len(merged.Show), len(merged.Show)-len(base.Show), mono)
	}
}
