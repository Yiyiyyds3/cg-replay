package vorbis

import (
	"bytes"
	"crypto/md5"
)

// rpgmvSig RPG Maker 加密音频的文件签名。
var rpgmvSig = []byte("RPGMV")

// emptyKey md5("") 的 16 字节——未设 encryptionKey 的游戏用这个。
var emptyKey = func() []byte {
	s := md5.Sum([]byte(""))
	return s[:]
}()

// restoreRPGMV 迭代还原 MV 加密音频：**每一页的头 16 字节**都被密钥异或
// （实测妹神官 03_The_Calm.rpgmvo：页 0 头还原后正常走页，页 1 头仍是密文）。
// 算法：输出流从「签名后 16 字节异或密钥」起步；沿输出流按页步进，每当
// 下一页头不是 OggS，就从加密文件取 16 字节异或密钥补进输出流，直到收尾。
// 尾部剩余原样补齐（最后一段 body 明文）。
func restoreRPGMV(data, key []byte) []byte {
	enc := data[16:]                     // 签名后：页 0 头的密文 16B + …
	out := make([]byte, 0, len(data)+64)
	out = append(out, xor16(enc[:16], key)...) // 页 0 头前 16B（OggS+ver+type+granule）
	encOff := 16
	walk := 0
	for {
		if walk+27 > len(out) {
			break
		}
		if out[walk+4] != 0 { // Ogg 版本异常：结构崩了，停止迭代
			break
		}
		nseg := int(out[walk+26])
		if walk+27+nseg > len(out) {
			break
		}
		bodyLen := 0
		for i := 0; i < nseg; i++ {
			bodyLen += int(out[walk+27+i])
		}
		next := walk + 27 + nseg + bodyLen
		if next+27 > len(out) {
			// 下一页头不在输出流里：若加密文件里还有页头密文就补，否则收尾
			if encOff+16 <= len(enc) {
				out = append(out, xor16(enc[encOff:encOff+16], key)...)
				encOff += 16
			}
			walk = next
			continue
		}
		if out[next+4] == 0 && out[next] == 'O' && out[next+1] == 'g' && out[next+2] == 'g' && out[next+3] == 'S' {
			walk = next // 下一页头已在输出流里（明文页头）
			continue
		}
		if encOff+16 > len(enc) {
			break
		}
		// 下一页头是密文：还原 16B 补进输出流
		out = append(out, xor16(enc[encOff:encOff+16], key)...)
		encOff += 16
		walk = next
	}
	if encOff < len(enc) {
		out = append(out, enc[encOff:]...)
	}
	return out
}

// xor16 16 字节异或。
func xor16(a, key []byte) []byte {
	out := make([]byte, 16)
	for i := range out {
		out[i] = a[i] ^ key[i%len(key)]
	}
	return out
}

// rpgmvVariants 生成加密音频的候选还原流，逐个喂解码器试解，第一个成功的生效：
//   keyed : MV 迭代逐页还原（key = System.json encryptionKey 的 16 字节 hex 值）；
//   mz    : MZ 直替型——签名后就是原文件 16B 起，14 字节重建头接 data[16:]；
//   empty : 密钥 = md5("")（未设 encryptionKey 的游戏，迭代同样适用）。
// 非加密数据返回空切片（调用方走原逻辑）。
func rpgmvVariants(data, key []byte) [][]byte {
	if len(data) <= 32 || !bytes.HasPrefix(data, rpgmvSig) {
		return nil
	}
	var out [][]byte
	if len(key) == 16 {
		out = append(out, restoreRPGMV(data, key))
	}
	out = append(out, restoreRPGMV(data, emptyKey))

	// MZ 直替型：签名后就是原文件 16B 起（页 0 头前段被覆盖），14 字节
	// 重建头（OggS+ver+type+granule 全零）接 data[16:]，页序列原样续上。
	mz := make([]byte, 0, len(data)-16+14)
	mz = append(mz, []byte("OggS\x00\x02\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00")...)
	mz = append(mz, data[16:]...)
	out = append(out, mz)
	return out
}
