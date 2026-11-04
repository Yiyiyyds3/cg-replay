// decode.go —— CP932 解码封装（与 bgi/artemis/catsystem2 同款）。
package nscript

// decodeMB 多字节编码 → UTF-8（Windows 走 MultiByteToWideChar，
// 其它平台退化为原样字节）。目标平台是 Windows。
func decodeMB(b []byte, cp int) string {
	if len(b) == 0 {
		return ""
	}
	if s, err := mbToWide(b, uint32(cp)); err == nil {
		return s
	}
	return string(b)
}
