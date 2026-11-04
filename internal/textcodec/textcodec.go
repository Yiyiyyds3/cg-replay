// Package textcodec 提供无第三方依赖的 CP932（Shift_JIS）解码。
//
// 目标平台是 Windows，走 MultiByteToWideChar；其他平台退化为原样返回
// （只为可移植性，9nine 的适配只在 Windows 上实际使用）。
package textcodec

// SJIS 把 CP932 字节解码成 UTF-8 字符串；无法解码时原样返回。
func SJIS(data []byte) string {
	return decodeSJIS(data)
}
