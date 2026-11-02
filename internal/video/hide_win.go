//go:build windows

package video

import (
	"os/exec"
	"syscall"
)

// hideConsole 避免每次提帧都闪一个黑窗。
func hideConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}
