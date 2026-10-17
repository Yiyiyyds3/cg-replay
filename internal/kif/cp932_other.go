//go:build !windows

package kif

// decodeName 在非 Windows 平台退化为按字节直取（本仓库仅面向 Windows 语料；
// 非 ASCII 名字会显示为乱码但不影响归档解析与解包）。
func decodeName(b []byte) string {
	return string(b)
}
