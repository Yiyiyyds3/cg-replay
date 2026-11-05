//go:build !windows

package alice

import "errors"

// decodeGBK 非 Windows 退化：原样字节当 Latin-1（目标平台是 Windows）。
func decodeGBK(b []byte) string { return string(b) }

// cp932Encode / gbkEncode 非 Windows 无系统转码，报错（测试会 skip）。
func cp932Encode(s string) ([]byte, error) { return nil, errors.New("mb encode unavailable") }
func gbkEncode(s string) ([]byte, error)  { return nil, errors.New("mb encode unavailable") }
