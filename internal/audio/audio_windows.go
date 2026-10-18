//go:build windows

package audio

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"ucgr/internal/ir"
)

// kagFadeDefaultMs 是 KAG BGM.fadeIn / SE.fadeIn 未给 time 时的默认淡入时长。
const kagFadeDefaultMs = 5000

// voiceSlot 是语音（vo）在 SE 槽位表里使用的独立槽位，避免与普通 SE 抢占。
const voiceSlot = -1000

// System 忠实实现 KAG 的 BGM/SE 播放语义：一个 waveOut 设备 + 一个软件混音器。
// BGM 单条（play 时硬替换、fadeIn 从 0 淡入、fadeOut 淡出后停止）；
// SE 按 buf 槽位并存，可叠加；语音用独立槽位。
type System struct {
	mix    *Mixer
	dev    *Device
	root   string
	mu     sync.Mutex
	bgm    *Voice
	se     map[int]*Voice
	cache  map[string]*PCM
	mute   bool
	closed bool
}

// NewSystem 打开音频设备；root 为解包根（用于把 storage 名解析成文件）。
func NewSystem(root string) (*System, error) {
	mix := &Mixer{}
	dev, err := OpenDevice(mix)
	if err != nil {
		return nil, err
	}
	return &System{mix: mix, dev: dev, root: root, se: map[int]*Voice{}, cache: map[string]*PCM{}}, nil
}

// Close 关闭音频系统（幂等）。
func (s *System) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	if s.dev != nil {
		s.dev.Close()
	}
}

// SetMute 全局静音（M 键）。
func (s *System) SetMute(m bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mute = m
	if s.dev != nil {
		if m {
			s.dev.SetVolume(0)
		} else {
			s.dev.SetVolume(0xFFFF | 0xFFFF<<16)
		}
	}
}

// Muted 返回当前是否静音。
func (s *System) Muted() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mute
}

// StopAll 停止所有 BGM 与 SE（保留音频设备），用于停止回放时清场。
func (s *System) StopAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.bgmStopLocked()
	for slot := range s.se {
		s.seStopLocked(slot)
	}
}

// Handle 是播放器的音频回调：按台本音频 op 执行 KAG 语义。
func (s *System) Handle(op ir.ShowOp) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	switch op.Kind {
	case ir.ShowBGMPlay:
		s.bgmPlay(op.Ref, attrBool(op.Attr, "loop", true), 0)
	case ir.ShowBGMFadeIn:
		s.bgmPlay(op.Ref, attrBool(op.Attr, "loop", true), fadeMs(op, kagFadeDefaultMs))
	case ir.ShowBGMFadeOut:
		s.bgmFadeOut(fadeMs(op, 0))
	case ir.ShowBGMStop:
		s.bgmStop()
	case ir.ShowSEPlay:
		s.sePlay(seBuf(op.Attr), op.Ref, attrBool(op.Attr, "loop", false), 0)
	case ir.ShowSEFadeIn:
		s.sePlay(seBuf(op.Attr), op.Ref, attrBool(op.Attr, "loop", false), fadeMs(op, kagFadeDefaultMs))
	case ir.ShowSEFadeOut:
		s.seFadeOut(seBuf(op.Attr), fadeMs(op, kagFadeDefaultMs))
	case ir.ShowSEStop:
		s.seStop(seBuf(op.Attr))
	case ir.ShowVoicePlay:
		s.sePlay(voiceSlot, op.Ref, false, 0)
	}
}

// bgmPlay 对应 KAG BGM.play（fadeMs<=0，硬替换）与 BGM.fadeIn（fadeMs>0，从 0 淡入）。
func (s *System) bgmPlay(ref string, loop bool, fadeMs int64) {
	s.bgmStopLocked()
	pcm := s.load(ref, "bgm")
	if pcm == nil {
		return
	}
	v := newVoice(pcm, loop)
	if fadeMs > 0 {
		v.cur = 0
		v.fadeTo(1, fadeMs)
	}
	s.mix.Add(v)
	s.bgm = v
}

// bgmFadeOut 对应 KAG BGM.fadeOut：淡出后停止（ms<=0 立即停止）。
func (s *System) bgmFadeOut(ms int64) {
	v := s.bgm
	if v == nil {
		return
	}
	s.bgm = nil
	if ms <= 0 {
		v.stop()
		s.mix.Remove(v)
		return
	}
	v.fadeOutAndStop(ms) // 结束后由 Mixer 自动移除
}

func (s *System) bgmStop() { s.bgmStopLocked() }

func (s *System) bgmStopLocked() {
	if s.bgm != nil {
		s.bgm.stop()
		s.mix.Remove(s.bgm)
		s.bgm = nil
	}
}

// sePlay 对应 KAG SE[buf].play（fadeMs<=0）与 SE[buf].fadeIn（fadeMs>0 从 0 淡入）。
func (s *System) sePlay(slot int, ref string, loop bool, fadeMs int64) {
	s.seStopLocked(slot)
	pcm := s.load(ref, "se")
	if pcm == nil {
		return
	}
	v := newVoice(pcm, loop)
	if fadeMs > 0 {
		v.cur = 0
		v.fadeTo(1, fadeMs)
	}
	s.mix.Add(v)
	s.se[slot] = v
}

func (s *System) seFadeOut(slot int, ms int64) {
	v := s.se[slot]
	if v == nil {
		return
	}
	delete(s.se, slot)
	if ms <= 0 {
		v.stop()
		s.mix.Remove(v)
		return
	}
	v.fadeOutAndStop(ms)
}

func (s *System) seStop(slot int) { s.seStopLocked(slot) }

func (s *System) seStopLocked(slot int) {
	if v := s.se[slot]; v != nil {
		v.stop()
		s.mix.Remove(v)
		delete(s.se, slot)
	}
}

// load 解析并解码素材（按 role 决定搜索目录），结果缓存在内存。
func (s *System) load(ref, role string) *PCM {
	if ref == "" {
		return nil
	}
	key := role + "\x00" + ref
	if p, ok := s.cache[key]; ok {
		return p
	}
	path := s.resolve(ref, role)
	if path == "" {
		return nil
	}
	p, err := DecodeFile(path)
	if err != nil {
		return nil
	}
	p = ConvertTo(p, devSampleRate, devChannels)
	s.cache[key] = p
	return p
}

// resolve 把 storage 名解析成文件路径。KAG 依次尝试 wav→ogg→tcw；本实现额外支持 mp3。
func (s *System) resolve(ref, role string) string {
	var dirs []string
	if role == "se" {
		dirs = []string{"raw/data/sound", "sound", "raw/data/voice", "voice", "raw/data/bgm", "bgm"}
	} else {
		dirs = []string{"raw/data/bgm", "bgm", "raw/data/sound", "sound"}
	}
	exts := []string{"", ".wav", ".ogg", ".tcw", ".mp3"}
	if filepath.Ext(ref) != "" {
		exts = []string{""}
	}
	for _, d := range dirs {
		for _, e := range exts {
			if p := filepath.Join(s.root, filepath.FromSlash(d), ref+e); isFile(p) {
				return p
			}
		}
	}
	if p := filepath.Join(s.root, filepath.FromSlash(ref)); isFile(p) {
		return p
	}
	return ""
}

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// attrBool 读布尔属性，缺省用 def（KAG: true/1 为真）。
func attrBool(m map[string]string, key string, def bool) bool {
	if m == nil {
		return def
	}
	v, ok := m[key]
	if !ok {
		return def
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "1", "yes":
		return true
	case "false", "0", "no":
		return false
	}
	return def
}

// seBuf 读 SE 的 buf 槽位，缺省 0。
func seBuf(m map[string]string) int {
	if m == nil {
		return 0
	}
	if v, ok := m["buf"]; ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
	}
	return 0
}

// fadeMs 返回该 op 的渐变时长：脚本显式给了 time 用解析值，否则用 def。
func fadeMs(op ir.ShowOp, def int64) int64 {
	if op.Attr != nil {
		if _, ok := op.Attr["time"]; ok {
			return op.DurMs
		}
	}
	return def
}
