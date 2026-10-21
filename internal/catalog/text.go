// text.go —— 中心库的台词抽取与最近播放。
//
// 台词抽取只读 ir.Event 的 Show 台本（text 指令），与 internal/player
// 的显示语义一致：说话人为空 = 沿用上一个说话人（名字牌不随旁白清掉）。
// 不碰回放路径：抽取失败只影响检索，不影响 OpenEvent/播放。
package catalog

import (
	"fmt"
	"strings"
	"time"

	"ucgr/internal/ir"
)

// TextLinesFromEvent 从一份演出台本抽台词行（供 ReplaceTextLines 写入）。
// 返回的行不带 GameID/Kind/EventID，由调用方按事件定位补齐。
func TextLinesFromEvent(ev *ir.Event) []TextLine {
	if ev == nil {
		return nil
	}
	out := make([]TextLine, 0, 64)
	speaker := "" // 沿用语义：空 = 沿用上一个说话人（同播放器名字牌）
	for _, op := range ev.Show {
		if op.Kind != ir.ShowTextRun || op.Text == nil {
			continue
		}
		text := strings.TrimSpace(op.Text.Text)
		if text == "" {
			continue
		}
		if s := strings.TrimSpace(op.Text.Speaker); s != "" {
			speaker = s
		}
		out = append(out, TextLine{
			Kind:    "", // 由调用方填
			EventID: "",
			TMs:     op.TMs,
			Speaker: speaker,
			Text:    text,
		})
	}
	return out
}

// RecentItem 一条最近播放记录（带游戏/事件标题，前端直接可渲染）。
type RecentItem struct {
	GameID    string `json:"game_id"`
	GameTitle string `json:"game_title"`
	Kind      string `json:"kind"`
	EventID   string `json:"event_id"`
	Title     string `json:"title"`
	PlayedAt  string `json:"played_at"`
	PlayCount int    `json:"play_count"`
}

// NoteRecent 记录一次播放：同一事件重复播放只更新时刻并递增 play_count。
func (c *Catalog) NoteRecent(gameID, kind, eventID string) error {
	gameID = strings.TrimSpace(gameID)
	kind = strings.TrimSpace(kind)
	eventID = strings.TrimSpace(eventID)
	if gameID == "" || kind == "" || eventID == "" {
		return fmt.Errorf("写最近播放失败：game_id/kind/event_id 不能为空")
	}
	now := now()
	_, err := c.db.Exec(`
		INSERT INTO recent(game_id, event_id, kind, played_at, play_count)
		VALUES (?, ?, ?, ?, 1)
		ON CONFLICT(game_id, event_id) DO UPDATE SET
			kind = excluded.kind,
			played_at = excluded.played_at,
			play_count = recent.play_count + 1`,
		gameID, eventID, kind, now)
	if err != nil {
		return fmt.Errorf("写最近播放失败：%w", err)
	}
	// 只保留最近 100 条，避免无限增长（旧行删除不影响播放）。
	if _, err := c.db.Exec(`
		DELETE FROM recent WHERE rowid NOT IN (
			SELECT rowid FROM recent ORDER BY played_at DESC LIMIT 100)`); err != nil {
		return fmt.Errorf("修剪最近播放失败：%w", err)
	}
	return nil
}

// Recent 最近播放列表（新的在前）；事件行缺失时标题回落为事件 id。
func (c *Catalog) Recent(limit int) ([]RecentItem, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	rows, err := c.db.Query(`
		SELECT r.game_id, COALESCE(g.title, r.game_id), r.kind, r.event_id,
		       COALESCE(e.title, r.event_id), r.played_at, r.play_count
		FROM recent r
		LEFT JOIN games g ON g.game_id = r.game_id
		LEFT JOIN events e ON e.game_id = r.game_id AND e.kind = r.kind AND e.event_id = r.event_id
		ORDER BY r.played_at DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("读最近播放失败：%w", err)
	}
	defer rows.Close()
	out := make([]RecentItem, 0, limit)
	for rows.Next() {
		var it RecentItem
		if err := rows.Scan(&it.GameID, &it.GameTitle, &it.Kind, &it.EventID,
			&it.Title, &it.PlayedAt, &it.PlayCount); err != nil {
			return nil, fmt.Errorf("读最近播放行失败：%w", err)
		}
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历最近播放失败：%w", err)
	}
	return out, nil
}

// PruneRecent 清理 played_at 早于 days 天的最近播放，返回删除条数（0 = 没得删）。
//
// 72h 冷归档窗口：days 传 0 用默认 3 天；负数或 >3650 直接报错（不静默改语义）。
// played_at 由 now() 统一写成 UTC RFC3339 文本，字典序即时间序，可直接比较。
// 只删 recent 表——不碰 favorites/text_lines/events（用户数据与索引都不受影响）。
func (c *Catalog) PruneRecent(days int) (int, error) {
	if days == 0 {
		days = 3
	}
	if days < 0 || days > 3650 {
		return 0, fmt.Errorf("清理窗口 %d 天非法（0 = 默认 3 天，范围 1~3650）", days)
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -days).Format(time.RFC3339)
	res, err := c.db.Exec(`DELETE FROM recent WHERE played_at < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("清理最近播放失败：%w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("清理最近播放计数失败：%w", err)
	}
	return int(n), nil
}

// TextCounts 每个游戏已索引的台词条数（没有台词行的游戏不出现在 map 里）。
func (c *Catalog) TextCounts() (map[string]int, error) {
	rows, err := c.db.Query(`SELECT game_id, COUNT(*) FROM text_lines GROUP BY game_id`)
	if err != nil {
		return nil, fmt.Errorf("读台词分布失败：%w", err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var (
			id    string
			count int
		)
		if err := rows.Scan(&id, &count); err != nil {
			return nil, fmt.Errorf("读台词分布行失败：%w", err)
		}
		out[id] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历台词分布失败：%w", err)
	}
	return out, nil
}
