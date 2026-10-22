package vorbis

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDecodeCorpus 解码目录下所有 .ogg，验证真实语料不 panic、帧数非 0。
// 需要设置 UCGR_OGG_DIR 指向语料根；默认跳过。
func TestDecodeCorpus(t *testing.T) {
	dir := os.Getenv("UCGR_OGG_DIR")
	if dir == "" {
		t.Skip("未设置 UCGR_OGG_DIR")
	}
	var files []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.EqualFold(filepath.Ext(path), ".ogg") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("遍历语料失败：%v", err)
	}
	if len(files) == 0 {
		t.Fatalf("目录下没有 .ogg：%s", dir)
	}
	channels := map[int]int{}
	var failed, short int
	for _, p := range files {
		fi, err := os.Stat(p)
		if err != nil {
			failed++
			t.Errorf("%s: %v", filepath.Base(p), err)
			continue
		}
		// 本作大量 .ogg 是只保留三个头页的占位文件（100~700 字节），
		// 没有音频页，真实播放器也无法解出声音；不作为解码失败统计。
		if fi.Size() < 4096 {
			short++
			continue
		}
		s, err := DecodeFile(p)
		if err != nil {
			failed++
			t.Errorf("%s: %v", filepath.Base(p), err)
			continue
		}
		if s.Frames() == 0 {
			failed++
			t.Errorf("%s: 解码为 0 帧", filepath.Base(p))
			continue
		}
		channels[s.Channels]++
	}
	t.Logf("语料 %d 个 .ogg（占位/头页 %d 个已跳过），失败 %d；声道分布 %v",
		len(files), short, failed, channels)
}

// TestDecodeRogoAgainstReference 解码 rogo.ogg 并与 PyAV 的 s16 参考逐样本对比。
// 需要设置 UCGR_OGG 与 UCGR_REF_DIR；默认跳过。
func TestDecodeRogoAgainstReference(t *testing.T) {
	ogg := os.Getenv("UCGR_OGG")
	if ogg == "" {
		t.Skip("未设置 UCGR_OGG")
	}
	refDir := os.Getenv("UCGR_REF_DIR")
	if refDir == "" {
		t.Skip("未设置 UCGR_REF_DIR（PyAV 生成的参考 PCM 目录）")
	}
	refPath := filepath.Join(refDir, "rogo.s16")
	ref, err := os.ReadFile(refPath)
	if err != nil {
		t.Skipf("读不到参考文件：%v", err)
	}

	s, err := DecodeFile(ogg)
	if err != nil {
		t.Fatalf("解码失败：%v", err)
	}

	refFrames := len(ref) / 2
	t.Logf("解码：%d Hz · %d 声道 · %d 帧 · %d ms", s.SampleRate, s.Channels, s.Frames(), s.DurationMs())
	t.Logf("参考：%d 帧", refFrames)

	if diff := s.Frames() - refFrames; diff < -8 || diff > 8 {
		t.Fatalf("帧数不一致：解码 %d vs 参考 %d（差 %d）", s.Frames(), refFrames, diff)
	}

	n := s.Frames()
	if refFrames < n {
		n = refFrames
	}

	// 逐样本比较，并给出 RMS 曲线。
	var maxErr float64
	var sumSq float64
	const blockMs = 50
	block := s.SampleRate * blockMs / 1000
	blocks := n / block
	if blocks > 12 {
		blocks = 12
	}

	for i := 0; i < n; i++ {
		got := float64(s.Samples[i*s.Channels])
		want := float64(int16(binary.LittleEndian.Uint16(ref[i*2:]))) / 32768.0
		d := math.Abs(got - want)
		if d > maxErr {
			maxErr = d
		}
		sumSq += d * d
	}
	rms := math.Sqrt(sumSq / float64(n))
	t.Logf("逐样本：最大误差 %.6f，RMS 误差 %.6f", maxErr, rms)

	for b := 0; b < blocks; b++ {
		var sg, sw float64
		for i := b * block; i < (b+1)*block; i++ {
			g := float64(s.Samples[i*s.Channels])
			w := float64(int16(binary.LittleEndian.Uint16(ref[i*2:]))) / 32768.0
			sg += g * g
			sw += w * w
		}
		t.Logf("  %3dms  RMS 解码 %.6f  参考 %.6f",
			b*blockMs, math.Sqrt(sg/float64(block)), math.Sqrt(sw/float64(block)))
	}

	if rms > 0.01 {
		t.Errorf("RMS 误差过大：%.6f", rms)
	}
}
