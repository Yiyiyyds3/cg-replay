//go:build windows

package audio

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"ucgr/internal/ir"
)

// writeWAV 把 16bit PCM 写成标准 WAV 文件（供解码测试用）。
func writeWAV(t *testing.T, path string, p *PCM) {
	t.Helper()
	var buf bytes.Buffer
	blockAlign := p.Channels * p.BitsPerSample / 8
	byteRate := p.SampleRate * blockAlign
	dataLen := len(p.Data)

	buf.WriteString("RIFF")
	binary.Write(&buf, binary.LittleEndian, uint32(36+dataLen))
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	binary.Write(&buf, binary.LittleEndian, uint32(16))
	binary.Write(&buf, binary.LittleEndian, uint16(1)) // PCM
	binary.Write(&buf, binary.LittleEndian, uint16(p.Channels))
	binary.Write(&buf, binary.LittleEndian, uint32(p.SampleRate))
	binary.Write(&buf, binary.LittleEndian, uint32(byteRate))
	binary.Write(&buf, binary.LittleEndian, uint16(blockAlign))
	binary.Write(&buf, binary.LittleEndian, uint16(p.BitsPerSample))
	buf.WriteString("data")
	binary.Write(&buf, binary.LittleEndian, uint32(dataLen))
	buf.Write(p.Data)

	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("写 WAV 失败: %v", err)
	}
}

// TestSystemKAGSemantics 覆盖 play / fadeIn / fadeOut / stop / SE 槽位 / 静音 / StopAll，
// 走的是真实链路：resolve 找文件 → MF 解码 → ConvertTo → 进入混音器。
func TestSystemKAGSemantics(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "raw", "data", "bgm")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// 1 秒的立体声常量信号（足够长，测试期间不会自然结束）。
	samples := make([]int16, devSampleRate*devChannels)
	for i := range samples {
		samples[i] = 1000
	}
	writeWAV(t, filepath.Join(dir, "ansin.wav"), mkPCM(devSampleRate, devChannels, samples...))

	sys, err := NewSystem(root)
	if err != nil {
		t.Skipf("无音频设备，跳过：%v", err)
	}
	defer sys.Close()

	lock := func() *Voice { sys.mu.Lock(); defer sys.mu.Unlock(); return sys.bgm }
	seOf := func(slot int) *Voice { sys.mu.Lock(); defer sys.mu.Unlock(); return sys.se[slot] }

	// play：硬替换，BGM 就位
	sys.Handle(ir.ShowOp{Kind: ir.ShowBGMPlay, Ref: "ansin"})
	if lock() == nil {
		t.Fatalf("playbgm 后应有 BGM 在播")
	}
	// 再 play 另一份（同文件）应替换而非叠加
	sys.Handle(ir.ShowOp{Kind: ir.ShowBGMPlay, Ref: "ansin"})
	if n := sys.mix.Voices(); n != 1 {
		t.Errorf("play 应替换旧曲，混音器里应只有 1 条 BGM，实得 %d", n)
	}
	// fadeIn：从 0 淡入（time=1000ms；DurMs 是解析后的毫秒值）
	sys.Handle(ir.ShowOp{Kind: ir.ShowBGMFadeIn, Ref: "ansin", DurMs: 1000, Attr: map[string]string{"loop": "true", "time": "1000"}})
	v := lock()
	if v == nil {
		t.Fatalf("fadeinbgm 后应有 BGM 在播")
	}
	sys.mix.mu.Lock()
	c, tgt := v.cur, v.target
	sys.mix.mu.Unlock()
	if tgt != 1 || c >= 1 {
		t.Errorf("fadeinbgm 应从 0 淡入到 1，实得 cur=%v target=%v", c, tgt)
	}
	// fadeOut：淡出后停止
	sys.Handle(ir.ShowOp{Kind: ir.ShowBGMFadeOut, Ref: "", DurMs: 1000, Attr: map[string]string{"time": "1000"}})
	if lock() != nil {
		t.Errorf("fadeoutbgm 后系统不应再持有 BGM")
	}
	// stopbgm
	sys.Handle(ir.ShowOp{Kind: ir.ShowBGMPlay, Ref: "ansin"})
	sys.Handle(ir.ShowOp{Kind: ir.ShowBGMStop})
	if lock() != nil {
		t.Errorf("stopbgm 后不应有 BGM")
	}
	// SE 按 buf 槽位并存
	sys.Handle(ir.ShowOp{Kind: ir.ShowSEPlay, Ref: "ansin", Attr: map[string]string{"buf": "2"}})
	sys.Handle(ir.ShowOp{Kind: ir.ShowSEPlay, Ref: "ansin", Attr: map[string]string{"buf": "3"}})
	if seOf(2) == nil || seOf(3) == nil {
		t.Errorf("SE slot2/slot3 都应各自播放")
	}
	sys.Handle(ir.ShowOp{Kind: ir.ShowSEStop, Attr: map[string]string{"buf": "2"}})
	if seOf(2) != nil {
		t.Errorf("stopse buf=2 后 slot2 应清空")
	}
	if seOf(3) == nil {
		t.Errorf("slot3 不应受影响")
	}
	// 静音 / 取消
	sys.SetMute(true)
	if !sys.Muted() {
		t.Errorf("SetMute(true) 后应静音")
	}
	sys.SetMute(false)
	if sys.Muted() {
		t.Errorf("SetMute(false) 后应取消静音")
	}
	// StopAll 清场
	sys.StopAll()
	if lock() != nil || len(sys.se) != 0 {
		t.Errorf("StopAll 后应无 BGM 且无 SE")
	}
}
