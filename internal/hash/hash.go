// Package hash 提供文件内容哈希（入库与回灌校验用）。
//
// v1 用标准库 crypto/sha256：上游 FINAL 1017 §9 允许 BLAKE3 / SHA-256 二选一。
// 后续可无缝换 github.com/minio/blake3（依赖就绪后），字段名不变。
package hash

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

// FileSHA256 计算文件内容的 SHA-256（十六进制小写）。
// 用 1MiB 缓冲流式读取，208 万文件量级下内存恒定。
func FileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	buf := make([]byte, 1<<20)
	if _, err := io.CopyBuffer(h, f, buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// BytesSHA256 计算字节内容的 SHA-256（十六进制小写）。
func BytesSHA256(b []byte) string {
	h := sha256.New()
	h.Write(b)
	return hex.EncodeToString(h.Sum(nil))
}

// Short 截断哈希用于日志显示。
func Short(h string) string {
	if len(h) > 12 {
		return h[:12] + "…"
	}
	return h
}

// VerifyFile 校验文件内容是否等于期望哈希（回灌/入库校验用）。
func VerifyFile(path, expect string) error {
	got, err := FileSHA256(path)
	if err != nil {
		return err
	}
	if got != expect {
		return fmt.Errorf("哈希不符: %s 期望 %s 实得 %s", path, Short(expect), Short(got))
	}
	return nil
}
