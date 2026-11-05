//go:build !windows

package nexas

// mbToWide 非 Windows 退化：原样字节当 Latin-1（目标平台是 Windows）。
func mbToWide(b []byte, cp uint32) (string, error) {
	return string(b), nil
}

// mbStrict 非 Windows 退化：恒成功。
func mbStrict(b []byte, cp uint32) bool { return true }
