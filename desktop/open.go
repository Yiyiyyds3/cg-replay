// 阶段 4 手动退路：在资源管理器中打开解包产物/源目录。
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// OpenPath 用系统文件管理器打开一个目录（也接受文件路径，打开其父目录）。
// 只做本地路径展示，不读取内容；路径不存在时明确报错。
func (a *App) OpenPath(path string) error {
	p := strings.TrimSpace(path)
	if p == "" {
		return fmt.Errorf("路径为空")
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return fmt.Errorf("路径无效：%w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return fmt.Errorf("路径不存在：%w", err)
	}
	if !info.IsDir() {
		abs = filepath.Dir(abs)
	}
	cmd := exec.Command("explorer.exe", abs)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("打开目录失败：%w", err)
	}
	_ = cmd.Process.Release()
	return nil
}
