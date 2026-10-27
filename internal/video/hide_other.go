//go:build !windows

package video

import "os/exec"

// hideConsole 非 Windows 无需处理。
func hideConsole(cmd *exec.Cmd) {}
