//go:build !windows

package textcodec

// decodeSJIS 非 Windows 无 CP932 内置支持，原样返回。
func decodeSJIS(b []byte) string { return string(b) }
