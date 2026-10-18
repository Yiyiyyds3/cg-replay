//go:build windows

package win32

import (
	"os"
	"testing"
)

func TestLooksLikeSFNTAndDeobfuscate(t *testing.T) {
	sfnt := []byte("OTTO\x00\x10\x01\x00\x00\x04\x00\x00")
	if !looksLikeSFNT(sfnt) {
		t.Fatal("OTTO 应被识别为 sfnt")
	}
	ob := make([]byte, len(sfnt))
	for i, c := range sfnt {
		ob[i] = c ^ 0x14
	}
	if looksLikeSFNT(ob) {
		t.Fatal("混淆后不应被识别为 sfnt")
	}
	got, changed := deobfuscateFont(ob)
	if !changed || !looksLikeSFNT(got) {
		t.Fatalf("去混淆失败 changed=%v", changed)
	}
	if string(got[:4]) != "OTTO" {
		t.Fatalf("还原后魔数=%q", got[:4])
	}
	// 未混淆的应原样返回
	if _, ch := deobfuscateFont(sfnt); ch {
		t.Fatal("未混淆的字体不应被改动")
	}
}

// TestRegisterFontFileReal 需要 UCGR_OTF 指向真实的（混淆的）字体文件。
func TestRegisterFontFileReal(t *testing.T) {
	ff := os.Getenv("UCGR_OTF")
	if ff == "" {
		t.Skip("UCGR_OTF 未设置")
	}
	face, ok := RegisterFontFile(ff)
	if !ok {
		t.Fatal("RegisterFontFile 失败")
	}
	if face == "" {
		t.Fatal("未解析出家族名")
	}
	t.Logf("face=%q", face)
}
