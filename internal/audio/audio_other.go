//go:build !windows

package audio

import (
	"errors"

	"ucgr/internal/ir"
)

// DecodeFile 在非 Windows 平台不可用。
func DecodeFile(string) (*PCM, error) {
	return nil, errors.New("audio: 仅支持 Windows（Media Foundation / waveOut）")
}

// System 在非 Windows 平台是空实现，保持与 Windows 侧相同的 API。
type System struct{}

// NewSystem 返回空系统。
func NewSystem(string) (*System, error) { return &System{}, nil }

// Close 无操作。
func (s *System) Close() {}

// Handle 无操作。
func (s *System) Handle(ir.ShowOp) {}

// SetMute 无操作。
func (s *System) SetMute(bool) {}

// Muted 恒为 false。
func (s *System) Muted() bool { return false }

// StopAll 无操作。
func (s *System) StopAll() {}
