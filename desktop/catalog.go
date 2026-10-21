// 桌面壳的中心库接线：SQLite catalog 只做可检索元数据，
// 回放仍走 library JSON；索引失败写 errors.log，不阻断回放。
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"ucgr/internal/catalog"
	"ucgr/internal/library"
	"ucgr/internal/registry"
)

// CatalogStatsDTO 前端索引状态角标。
type CatalogStatsDTO struct {
	Games     int    `json:"games"`
	Events    int    `json:"events"`
	TextLines int    `json:"text_lines"`
	Error     string `json:"error,omitempty"`
}

// withCatalog 串行打开/使用 catalog（modernc SQLite 单连接）。
func (a *App) withCatalog(fn func(*catalog.Catalog) error) error {
	a.catalogMu.Lock()
	defer a.catalogMu.Unlock()
	if a.catalogDB == nil {
		c, err := catalog.Open(filepath.Join(exeDir(), catalog.FileName))
		if err != nil {
			return err
		}
		a.catalogDB = c
	}
	return fn(a.catalogDB)
}

// syncCatalog 把当前游戏的库清单覆盖进 catalog；失败只记日志，不改变回放路径。
func (a *App) syncCatalog(g *registry.Game, lib *library.Library) {
	if g == nil || lib == nil {
		return
	}
	if err := a.withCatalog(func(c *catalog.Catalog) error {
		return c.SyncGame(*g, lib)
	}); err != nil {
		a.logAuxError("catalog sync", err)
	}
}

// CatalogStats 返回索引规模；error 字段供前端常驻显示，避免静默失败。
func (a *App) CatalogStats() CatalogStatsDTO {
	var out CatalogStatsDTO
	err := a.withCatalog(func(c *catalog.Catalog) error {
		s, err := c.Stats()
		if err != nil {
			return err
		}
		out.Games, out.Events, out.TextLines = s.Games, s.Events, s.TextLines
		return nil
	})
	if err != nil {
		out.Error = err.Error()
	}
	return out
}

// shutdown 关闭 SQLite（Wails OnShutdown）。
func (a *App) shutdown(ctx context.Context) {
	_ = ctx
	a.catalogMu.Lock()
	defer a.catalogMu.Unlock()
	if a.catalogDB != nil {
		_ = a.catalogDB.Close()
		a.catalogDB = nil
	}
}

func (a *App) logAuxError(area string, err error) {
	if err == nil {
		return
	}
	f, ferr := os.OpenFile(filepath.Join(exeDir(), "errors.log"),
		os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if ferr != nil {
		return
	}
	defer f.Close()
	_, _ = fmt.Fprintf(f, "%s [%s] %v\n", time.Now().Format("2006-01-02 15:04:05"), area, err)
}
