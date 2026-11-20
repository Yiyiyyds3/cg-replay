// Command unpack_all 一次性全量解包工具（ScopeAll + SkipVideo + AllImages），
// 用于 suspect 款补图（图片按归档全量解，不只看脚本引用）。
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"ucgr/internal/unpack"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("usage: unpack_all <xp3游戏目录> <目标目录>")
		os.Exit(2)
	}
	// AllImages：图片不按引用闭包解，而是「归档里所有图」全解——补图/建 CG 库
	// 时脚本里那一堆运行时拼名字的差分、立ち絵表情、サムネ全靠这一条。
	res, err := unpack.ExtractWith(os.Args[1], os.Args[2], unpack.Options{
		Scope:     unpack.ScopeAll,
		SkipVideo: true,
		AllImages: true,
	})
	if err != nil {
		fmt.Println("FAIL:", err)
		os.Exit(1)
	}
	b, _ := json.Marshal(res)
	fmt.Println(string(b))
}
