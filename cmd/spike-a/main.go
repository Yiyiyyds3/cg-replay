// Command spike-a 档实操验证（临时工具，不进正式链路）：
//
//	-mode unity  <导出目录>          → ScanUnity 数事件
//	-mode unpack <游戏根> <目标目录>  → unpack.ExtractWith 解包 + 报告产物
//	-mode scan   <解包目录>          → kirikiri ScanWith 数事件
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"ucgr/internal/adapter"
	_ "ucgr/internal/adapter/kirikiri"
	"ucgr/internal/library"
	"ucgr/internal/unpack"
)

func main() {
	mode := flag.String("mode", "", "unity | unpack | scan")
	flag.Parse()
	os.Exit(run(*mode, flag.Arg(0), flag.Arg(1)))
}

func run(mode, a1, a2 string) int {
	switch mode {
	case "unity":
		lib, err := library.ScanUnity(a1)
		if err != nil {
			fmt.Println("FAIL:", err)
			return 1
		}
		out(map[string]int{"scenes": len(lib.Scenes), "events": len(lib.Scenes), "cgs": len(lib.CGs)})
		return 0
	case "unpack":
		res, err := unpack.ExtractWith(a1, a2, unpack.Options{Scope: unpack.ScopeCG, SkipVideo: true})
		if err != nil {
			fmt.Println("FAIL:", err)
			return 1
		}
		b, _ := json.Marshal(res)
		fmt.Println(string(b))
		return 0
	case "scan":
		var lib *library.Library
		var err error
		// 先试 rmmz（www/data 有 System.json 就走 rmmz），否则按 KiriKiri。
		if st, e := os.Stat(filepath.Join(a1, "www", "data", "System.json")); e == nil && !st.IsDir() {
			lib, err = library.ScanRMMZ(a1, adapter.Convention{})
		} else {
			conv := adapter.DefaultKiriKiri()
			lib, err = library.ScanWith(a1, conv)
		}
		if err != nil {
			fmt.Println("FAIL:", err)
			return 1
		}
		out(map[string]int{
			"cgs":    len(lib.CGs),
			"scenes": len(lib.Scenes),
			"events": len(lib.CGs) + len(lib.Scenes) + len(lib.Chapters),
			"warn":   len(lib.Warnings),
		})
		return 0
	}
	fmt.Println("未知 -mode", mode)
	return 2
}

func out(m map[string]int) {
	b, _ := json.Marshal(m)
	fmt.Println(string(b))
}
