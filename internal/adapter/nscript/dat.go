// dat.go —— NScripter 剧本数据读取。
//
// nscript.dat = 每字节 XOR 0x84 的 Shift-JIS(CP932) 明文脚本（nsdec 算法，
// 不是真加密）。明文剧本（0.txt / 00.txt）原样读取。统一返回 UTF-8 文本。
package nscript

import (
	"bytes"
	"os"
	"strings"
)

// scriptKey nscript.dat 的固定 XOR 键。
const scriptKey = 0x84

// ReadScriptData 读一个剧本文件并解码为 UTF-8 文本。
//
// nscript.dat 先整文件 XOR 0x84 再按 CP932 解码；其余按明文处理
// （0.txt 等也是 CP932）。
func ReadScriptData(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if strings.EqualFold(slashBase(path), "nscript.dat") {
		for i := range raw {
			raw[i] ^= scriptKey
		}
	}
	return []byte(decodeMB(raw, 932)), nil
}

// LooksLikeNScripterDat 判断一段字节是不是 NScripter 剧本：
// XOR 0x84 采样里出现明文关键字（;value / *define / numalias…）即密文；
// 本身已含关键字即明文剧本。不认首字节——有的剧本解出来以空行开头。
func LooksLikeNScripterDat(raw []byte) bool {
	if len(raw) < 8 {
		return false
	}
	n := len(raw)
	if n > 512 {
		n = 512
	}
	dec := make([]byte, n)
	for i := 0; i < n; i++ {
		dec[i] = raw[i] ^ scriptKey
	}
	for _, kw := range []string{";value", "*define", "numalias", "stralias"} {
		if bytes.Contains(dec, []byte(kw)) {
			return true
		}
	}
	for _, kw := range []string{";value", "*define", "numalias", "stralias"} {
		if bytes.Contains(raw[:n], []byte(kw)) {
			return true
		}
	}
	return false
}

func slashBase(p string) string {
	i := strings.LastIndexAny(p, `/\`)
	if i >= 0 {
		return p[i+1:]
	}
	return p
}
