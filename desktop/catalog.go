// 桌面壳的中心库接线：SQLite catalog 只做可检索元数据，
// 回放仍走 library JSON；索引失败写 errors.log，不阻断回放。
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ucgr/internal/adapter/kirikiri"
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

// ---------- 跨游戏查询（第三阶段：全部游戏视图 / 台词检索） ----------

// CatalogEventDTO 跨游戏事件卡片（与 internal/catalog.Event 对齐）。
type CatalogEventDTO struct {
	GameID    string `json:"game_id"`
	GameTitle string `json:"game_title"`
	GameRoot  string `json:"game_root"`
	Kind      string `json:"kind"`
	ID        string `json:"id"`
	Title     string `json:"title"`
	Ord       int    `json:"ord"`
	Pages     int    `json:"pages"`
	HasCover  bool   `json:"has_cover"`
}

// CatalogEventsDTO 跨游戏事件查询结果；warnings 记录同步失败的游戏
// （部分成功也返回，不因一个游戏失败整体报错）。
type CatalogEventsDTO struct {
	Events   []CatalogEventDTO `json:"events"`
	Warnings []string          `json:"warnings"`
}

// CatalogTextHitDTO 一条台词命中（与 internal/catalog.TextHit 对齐）。
type CatalogTextHitDTO struct {
	LineID    int64  `json:"line_id"`
	GameID    string `json:"game_id"`
	GameTitle string `json:"game_title"`
	Kind      string `json:"kind"`
	EventID   string `json:"event_id"`
	TMs       int64  `json:"t_ms"`
	Speaker   string `json:"speaker"`
	Text      string `json:"text"`
}

// CatalogTextDTO 台词检索结果；indexed=false 时前端必须提示
// 「台词索引尚未抽取」，不能把 0 条结果伪装成「没搜到」。
type CatalogTextDTO struct {
	Hits     []CatalogTextHitDTO `json:"hits"`
	Warnings []string            `json:"warnings"`
	Indexed  bool                `json:"indexed"`
}

// CatalogIndexResultDTO 台词索引结果（部分失败进 warnings）。
type CatalogIndexResultDTO struct {
	GameID   string   `json:"game_id"`
	Events   int      `json:"events"`
	Lines    int      `json:"lines"`
	Warnings []string `json:"warnings"`
}

// CatalogRecentDTO 最近播放条目（与 internal/catalog.RecentItem 对齐）。
type CatalogRecentDTO struct {
	GameID    string `json:"game_id"`
	GameTitle string `json:"game_title"`
	Kind      string `json:"kind"`
	EventID   string `json:"event_id"`
	Title     string `json:"title"`
	PlayedAt  string `json:"played_at"`
	PlayCount int    `json:"play_count"`
}

// ---------- 收藏（favorites 表；本轮 UI 只做 event 级） ----------

// FavoriteDTO 一条收藏（与 internal/catalog.Favorite 对齐）。
type FavoriteDTO struct {
	ItemType  string `json:"item_type"` // game / event / character
	GameID    string `json:"game_id"`
	ItemID    string `json:"item_id"`
	Rating    int    `json:"rating"` // 0 = 未评分
	Note      string `json:"note"`
	Title     string `json:"title"`      // 展示用（events/games join，缺失回落 item_id）
	GameTitle string `json:"game_title"` // 展示用
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// AddFavorite 新增/更新收藏（同键 upsert rating/note）。
func (a *App) AddFavorite(itemType, gameID, itemID string, rating int, note string) error {
	return a.withCatalog(func(c *catalog.Catalog) error {
		return c.AddFavorite(itemType, gameID, itemID, rating, note)
	})
}

// RemoveFavorite 取消收藏（不存在时幂等成功）。
func (a *App) RemoveFavorite(itemType, gameID, itemID string) error {
	return a.withCatalog(func(c *catalog.Catalog) error {
		return c.RemoveFavorite(itemType, gameID, itemID)
	})
}

// ListFavorites 收藏列表（itemType 为空 = 全部；新的在前）。
func (a *App) ListFavorites(itemType string) ([]FavoriteDTO, error) {
	var out []FavoriteDTO
	err := a.withCatalog(func(c *catalog.Catalog) error {
		items, lerr := c.ListFavorites(itemType, 0)
		if lerr != nil {
			return lerr
		}
		out = make([]FavoriteDTO, 0, len(items))
		for _, f := range items {
			out = append(out, FavoriteDTO{
				ItemType: f.ItemType, GameID: f.GameID, ItemID: f.ItemID,
				Rating: f.Rating, Note: f.Note, Title: f.Title,
				GameTitle: f.GameTitle, CreatedAt: f.CreatedAt, UpdatedAt: f.UpdatedAt,
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ---------- 说话人聚合（character 级收藏的前置） ----------

// SpeakerDTO 说话人聚合行（与 internal/catalog.Speaker 对齐）。
type SpeakerDTO struct {
	GameID    string `json:"game_id"`
	GameTitle string `json:"game_title"`
	Name      string `json:"name"`
	Lines     int    `json:"lines"`
	Events    int    `json:"events"`
}

// CatalogSpeakers 说话人聚合（gameID 为空 = 全部已索引游戏；按条数降序）。
// 数据源是 text_lines：台词未抽取的游戏不会出现说话人。
func (a *App) CatalogSpeakers(gameID string) ([]SpeakerDTO, error) {
	var out []SpeakerDTO
	err := a.withCatalog(func(c *catalog.Catalog) error {
		items, serr := c.Speakers(gameID, 0)
		if serr != nil {
			return serr
		}
		out = make([]SpeakerDTO, 0, len(items))
		for _, s := range items {
			out = append(out, SpeakerDTO{
				GameID: s.GameID, GameTitle: s.GameTitle, Name: s.Name,
				Lines: s.Lines, Events: s.Events,
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ensureCatalogSync 查询前把登记表里的每款游戏同步进 catalog
// （loadOrScanLib + SyncGame）。失败的游戏记 warnings 后跳过，
// 返回已成功部分——不启动时全量扫盘，只在查询时按需同步。
func (a *App) ensureCatalogSync() []string {
	var warnings []string
	c, _ := a.loadConf()
	reg, err := a.loadRegistry(c)
	if err != nil {
		return []string{fmt.Sprintf("读游戏登记表失败：%v", err)}
	}
	for i := range reg.Games {
		g := &reg.Games[i]
		lib, lerr := a.loadOrScanLib(g)
		if lerr != nil {
			warnings = append(warnings, fmt.Sprintf("游戏「%s」库读取失败：%v", g.Title, lerr))
			continue
		}
		if serr := a.withCatalog(func(cat *catalog.Catalog) error {
			return cat.SyncGame(*g, lib)
		}); serr != nil {
			warnings = append(warnings, fmt.Sprintf("游戏「%s」索引同步失败：%v", g.Title, serr))
		}
	}
	return warnings
}

// CatalogEvents 跨游戏事件查询（query 为空 = 全部；按游戏标题+序）。
func (a *App) CatalogEvents(query string, limit int) (CatalogEventsDTO, error) {
	warnings := a.ensureCatalogSync()
	var out CatalogEventsDTO
	out.Warnings = warnings
	err := a.withCatalog(func(c *catalog.Catalog) error {
		events, qerr := c.SearchEvents(query, limit)
		if qerr != nil {
			return qerr
		}
		out.Events = make([]CatalogEventDTO, 0, len(events))
		for _, e := range events {
			out.Events = append(out.Events, CatalogEventDTO{
				GameID: e.GameID, GameTitle: e.GameTitle, GameRoot: e.GameRoot,
				Kind: e.Kind, ID: e.ID, Title: e.Title, Ord: e.Ord,
				Pages: e.Pages, HasCover: e.HasCover,
			})
		}
		return nil
	})
	if err != nil {
		return CatalogEventsDTO{}, err
	}
	return out, nil
}

// CatalogSearchText 台词全文检索（FTS5 trigram；<3 字符走 LIKE）。
func (a *App) CatalogSearchText(query string, limit int) (CatalogTextDTO, error) {
	warnings := a.ensureCatalogSync()
	var out CatalogTextDTO
	out.Warnings = warnings
	err := a.withCatalog(func(c *catalog.Catalog) error {
		s, serr := c.Stats()
		if serr != nil {
			return serr
		}
		out.Indexed = s.TextLines > 0
		hits, qerr := c.SearchText(query, limit)
		if qerr != nil {
			return qerr
		}
		out.Hits = make([]CatalogTextHitDTO, 0, len(hits))
		for _, h := range hits {
			out.Hits = append(out.Hits, CatalogTextHitDTO{
				LineID: h.LineID, GameID: h.GameID, GameTitle: h.GameTitle,
				Kind: h.Kind, EventID: h.EventID, TMs: h.TMs,
				Speaker: h.Speaker, Text: h.Text,
			})
		}
		return nil
	})
	if err != nil {
		return CatalogTextDTO{}, err
	}
	return out, nil
}

// CatalogTextCounts 每款游戏已索引的台词条数（0 台词游戏不在 map 里）。
func (a *App) CatalogTextCounts() (map[string]int, error) {
	var out map[string]int
	err := a.withCatalog(func(c *catalog.Catalog) error {
		counts, cerr := c.TextCounts()
		if cerr != nil {
			return cerr
		}
		out = counts
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// CatalogRecent 最近播放列表（新的在前）。
func (a *App) CatalogRecent(limit int) ([]CatalogRecentDTO, error) {
	var out []CatalogRecentDTO
	err := a.withCatalog(func(c *catalog.Catalog) error {
		items, rerr := c.Recent(limit)
		if rerr != nil {
			return rerr
		}
		out = make([]CatalogRecentDTO, 0, len(items))
		for _, it := range items {
			out = append(out, CatalogRecentDTO{
				GameID: it.GameID, GameTitle: it.GameTitle, Kind: it.Kind,
				EventID: it.EventID, Title: it.Title,
				PlayedAt: it.PlayedAt, PlayCount: it.PlayCount,
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// CatalogIndexText 抽取一款游戏全部事件的台词写入 catalog（幂等覆盖）。
// IR 构建与 OpenEvent 同口径（BuildShowSegment / BuildChapterEvent），
// 单事件失败只记 warning，不阻断其余事件。抽完不自动全量重跑：
// 由前端在「全部游戏」视图对 0 台词游戏按需调用。
func (a *App) CatalogIndexText(gameID string) (CatalogIndexResultDTO, error) {
	gameID = strings.TrimSpace(gameID)
	if gameID == "" {
		return CatalogIndexResultDTO{}, fmt.Errorf("游戏 id 不能为空")
	}
	c, _ := a.loadConf()
	reg, err := a.loadRegistry(c)
	if err != nil {
		return CatalogIndexResultDTO{}, err
	}
	g := reg.Find(gameID)
	if g == nil {
		return CatalogIndexResultDTO{}, fmt.Errorf("游戏 %s 未登记", gameID)
	}
	lib, err := a.loadOrScanLib(g)
	if err != nil {
		return CatalogIndexResultDTO{}, err
	}
	out := CatalogIndexResultDTO{GameID: g.ID}
	writeLines := func(kind, id string, lines []catalog.TextLine) {
		for i := range lines {
			lines[i].Kind = kind
			lines[i].EventID = id
		}
		if werr := a.withCatalog(func(cat *catalog.Catalog) error {
			return cat.ReplaceTextLines(g.ID, kind, id, lines)
		}); werr != nil {
			out.Warnings = append(out.Warnings, fmt.Sprintf("写入台词 %s/%s 失败：%v", kind, id, werr))
			return
		}
		out.Events++
		out.Lines += len(lines)
	}

	for _, cg := range lib.CGs {
		// 画廊空槽（本作 cg091~cg096：pages 为空、label 无对应段）：
		// 原作即无内容，OpenEvent 会明确报错；索引侧静默跳过，不当故障。
		if len(cg.Pages) == 0 {
			continue
		}
		ev, berr := kirikiri.BuildShowSegment(
			filepath.Join(g.Root, filepath.FromSlash(cg.Script)), cg.Label)
		if berr != nil {
			out.Warnings = append(out.Warnings, fmt.Sprintf("CG %s 台本构建失败：%v", cg.ID, berr))
			continue
		}
		writeLines("cg", cg.ID, catalog.TextLinesFromEvent(ev))
	}
	for _, s := range lib.Scenes {
		ev, berr := kirikiri.BuildShowSegment(
			filepath.Join(g.Root, filepath.FromSlash(s.Script)), "")
		if berr != nil {
			out.Warnings = append(out.Warnings, fmt.Sprintf("场景 %s 台本构建失败：%v", s.ID, berr))
			continue
		}
		writeLines("scene", s.ID, catalog.TextLinesFromEvent(ev))
	}
	for _, ch := range lib.Chapters {
		ev, berr := library.BuildChapterEvent(g.Root, &ch)
		if berr != nil {
			out.Warnings = append(out.Warnings, fmt.Sprintf("段落 %s 台本构建失败：%v", ch.ID, berr))
			continue
		}
		writeLines("chapter", ch.ID, catalog.TextLinesFromEvent(ev))
	}
	return out, nil
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
