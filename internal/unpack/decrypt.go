// decrypt.go —— XP3 的解密方案（阶段 3：密钥通道）。
//
// 背景（查证结论）：XP3 索引里的 **adlr 块是条目名的 Adler32 校验和**，
// **不等于加密**——很多游戏带 adlr 但数据是明文（我们此前把 adlr 当加密
// 标记，会误跳过大量条目，本轮修正）。
//
// 真加密时，各家游戏用的多是「用 hash 派生单字节 XOR 密钥」的家族
// （GARbro 的 CryptAlgorithms 一族）：
//
//	hash      key = (byte)hash
//	natsupochi key = (byte)(hash >> 3)
//	poringsoft key = (byte)~(hash + 1)
//	sourire    key = (byte)(hash ^ 0xCD)
//	haikuo     key = (byte)(hash ^ (hash >> 8))
//	xor        key = 用户给的固定字节
//
// 解密时机与 arc_unpacker 一致：**先解压，再对整份数据 XOR**。
// 更复杂的方案（CxEncryption / HxCrypt / 柚子社系）需要每作密钥或
// 字节码模拟，本阶段不做——遇到时如实报告，不硬凑。
package unpack

import (
	"fmt"

	"ucgr/internal/adapter"
)

// Scheme 解密方案。
type Scheme string

const (
	// SchemeNone 不解密（默认）：adlr 只是校验和，内容按明文处理。
	SchemeNone Scheme = ""
	// SchemeHash key = (byte)hash（GARbro HashCrypt，最常见）。
	SchemeHash Scheme = "hash"
	// SchemeNatsupochi key = (byte)(hash >> 3)。
	SchemeNatsupochi Scheme = "natsupochi"
	// SchemePoringSoft key = (byte)~(hash + 1)。
	SchemePoringSoft Scheme = "poringsoft"
	// SchemeSourire key = (byte)(hash ^ 0xCD)。
	SchemeSourire Scheme = "sourire"
	// SchemeHaikuo key = (byte)(hash ^ (hash >> 8))。
	SchemeHaikuo Scheme = "haikuo"
	// SchemeXor 固定单字节密钥（用户填）。
	SchemeXor Scheme = "xor"
)

// SchemeFromString 解析方案名（空 = 不解密）。
func SchemeFromString(s string) (Scheme, error) {
	switch Scheme(normalizeScheme(s)) {
	case SchemeNone:
		return SchemeNone, nil
	case SchemeHash:
		return SchemeHash, nil
	case SchemeNatsupochi:
		return SchemeNatsupochi, nil
	case SchemePoringSoft:
		return SchemePoringSoft, nil
	case SchemeSourire:
		return SchemeSourire, nil
	case SchemeHaikuo:
		return SchemeHaikuo, nil
	case SchemeXor:
		return SchemeXor, nil
	}
	return SchemeNone, fmt.Errorf("未知的解密方案 %q（可用：hash / natsupochi / poringsoft / sourire / haikuo / xor）", s)
}

// normalizeScheme 小写化并去空格。
func normalizeScheme(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r == ' ' || r == '\t' {
			continue
		}
		if r >= 'A' && r <= 'Z' {
			r = r - 'A' + 'a'
		}
		out = append(out, r)
	}
	return string(out)
}

// SchemeKey 按方案从条目派生 XOR 密钥。
//
// hash 家族需要条目带 adlr（HasHash）；xor 方案用用户给的固定字节。
func SchemeKey(s Scheme, e adapter.ArchiveEntry, fixed byte) (byte, error) {
	switch s {
	case SchemeNone:
		return 0, nil
	case SchemeXor:
		return fixed, nil
	case SchemeHash, SchemeNatsupochi, SchemePoringSoft, SchemeSourire, SchemeHaikuo:
		if !e.HasHash {
			return 0, fmt.Errorf("条目 %s 没有 adlr 校验和，无法按 %s 方案派生密钥", e.Path, s)
		}
		h := e.Hash
		switch s {
		case SchemeHash:
			return byte(h), nil
		case SchemeNatsupochi:
			return byte(h >> 3), nil
		case SchemePoringSoft:
			return byte(^(h + 1)), nil
		case SchemeSourire:
			return byte(h ^ 0xCD), nil
		case SchemeHaikuo:
			return byte(h ^ (h >> 8)), nil
		}
	}
	return 0, fmt.Errorf("未知方案 %q", s)
}

// ApplyDecrypt 对已解压的数据做 XOR 解密（原地，返回同一片）。
//
// 密钥为 0 时**不改数据**——很多条目本来就是明文（XOR 0 等于恒等）。
func ApplyDecrypt(s Scheme, e adapter.ArchiveEntry, data []byte, fixed byte) ([]byte, error) {
	if s == SchemeNone || len(data) == 0 {
		return data, nil
	}
	// 没有 adlr 的条目是明文（游戏只加密部分条目）——不报错，直接放行。
	if s != SchemeXor && !e.HasHash {
		return data, nil
	}
	key, err := SchemeKey(s, e, fixed)
	if err != nil {
		return nil, err
	}
	if key == 0 {
		return data, nil
	}
	for i := range data {
		data[i] ^= key
	}
	return data, nil
}
