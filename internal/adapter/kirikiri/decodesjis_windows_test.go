package kirikiri

import "testing"

// TestDecodeSJIS 只在 Windows 编译运行：验证 CP932 经 MultiByteToWideChar 解码。
func TestDecodeSJIS(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want string
	}{
		{"nil", nil, ""},
		{"ascii", []byte("abc"), "abc"},
		{"hiragana", []byte{0x82, 0xA0}, "あ"},
		{"kanji", []byte{0x93, 0xFA, 0x96, 0x7B}, "日本"},
	}
	for _, c := range cases {
		if got := decodeSJIS(c.in); got != c.want {
			t.Errorf("%s: decodeSJIS(% x) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}
