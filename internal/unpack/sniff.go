// sniff.go —— 归档解析失败时，说清「它到底是个什么东西」。
//
// 实测踩到两次：扩展名写着 .xp3，但磁数其实是 ZIP / EVB（E-mote 编译产物）。
// 解析器拒绝是对的，但只报「不是 XP3 归档」对用户没有帮助 —— 他不知道该丢什么
// 进来、还是该换个引擎。这里按文件头给一句人话。
package unpack

import (
	"bytes"
	"fmt"
	"io"
	"os"
)

// sniffHint 按文件头判断真实格式，返回给用户看的提示（空串 = 无话可说）。
func sniffHint(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	head := make([]byte, 16)
	n, _ := io.ReadFull(f, head)
	if n < 4 {
		return "文件太小，读不出头部"
	}
	head = head[:n]
	switch {
	case bytes.HasPrefix(head, []byte("PK\x03\x04")):
		return "它其实是 ZIP 封包（PK 魔数），只是扩展名写成了归档后缀"
	case bytes.HasPrefix(head, []byte("EVB")):
		return "它是 EVB（E-mote 的编译产物），不是归档容器"
	case bytes.HasPrefix(head, []byte("XP3\r\n \n\x1a")):
		return "头部是 XP3 但索引解析失败——可能是索引被加密（每作不同），需手动解包"
	case bytes.HasPrefix(head, []byte("pf")):
		return "它看起来是 Artemis PFS"
	case bytes.HasPrefix(head, []byte("BurikoCompiledScript")):
		return "它是 BGI 编译脚本（.bs），不是归档"
	}
	return fmt.Sprintf("文件头不是归档魔数（开头 % x）", head[:min(6, len(head))])
}

// min 兼容（Go 1.21 起内置 min 也可用，这里显式写一份避免版本差异）。
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
