// favorites.go —— 中心库的收藏（favorites 表）。
//
// 三级收藏（item_type）：game / event / character。本轮 UI 只做 event 级，
// 读写层一次到位（UNIQUE(item_type, game_id, item_id) 已由 schema 建好）。
// rating 0 = 未评分 → 存 NULL（schema CHECK 只允许 1~5 或 NULL）。
package catalog

import (
	"errors"
	"fmt"
	"strings"
)

// 收藏类型（与 schema 的 CHECK 一致）。
const (
	FavGame = "game"
	FavEven = "event"
	FavChar = "character"
)

// ValidFavType 报告收藏类型是否合法。
func ValidFavType(t string) bool {
	switch t {
	case FavGame, FavEven, FavChar:
		return true
	}
	return false
}

// Favorite 一条收藏（Title/GameTitle 为展示用，join 失败回落 item_id）。
type Favorite struct {
	ItemType  string `json:"item_type"`
	GameID    string `json:"game_id"`
	ItemID    string `json:"item_id"`
	Rating    int    `json:"rating"` // 0 = 未评分
	Note      string `json:"note"`
	Title     string `json:"title"`
	GameTitle string `json:"game_title"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func normalizeFav(itemType, gameID, itemID string, rating int, note string) (string, string, string, int, string, error) {
	itemType = strings.TrimSpace(itemType)
	gameID = strings.TrimSpace(gameID)
	itemID = strings.TrimSpace(itemID)
	if !ValidFavType(itemType) {
		return "", "", "", 0, "", fmt.Errorf("收藏类型 %q 非法（可用：game/event/character）", itemType)
	}
	if gameID == "" {
		return "", "", "", 0, "", errors.New("game_id 不能为空")
	}
	if itemID == "" {
		return "", "", "", 0, "", errors.New("item_id 不能为空")
	}
	if rating != 0 && (rating < 1 || rating > 5) {
		return "", "", "", 0, "", fmt.Errorf("评分 %d 非法（0 = 未评分，或 1~5）", rating)
	}
	return itemType, gameID, itemID, rating, strings.TrimSpace(note), nil
}

// AddFavorite 新增或更新收藏（同 (item_type, game_id, item_id) 已存在则
// 覆盖 rating/note，不动 created_at）。
func (c *Catalog) AddFavorite(itemType, gameID, itemID string, rating int, note string) error {
	itemType, gameID, itemID, rating, note, err := normalizeFav(itemType, gameID, itemID, rating, note)
	if err != nil {
		return err
	}
	now := now()
	var ratingArg any
	if rating > 0 {
		ratingArg = rating
	} // 0 → NULL（CHECK 只允许 1~5 或 NULL）
	if _, err := c.db.Exec(`
		INSERT INTO favorites(item_type, game_id, item_id, rating, note, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(item_type, game_id, item_id) DO UPDATE SET
			rating = excluded.rating,
			note = excluded.note,
			updated_at = excluded.updated_at`,
		itemType, gameID, itemID, ratingArg, note, now, now); err != nil {
		return fmt.Errorf("写收藏失败：%w", err)
	}
	return nil
}

// RemoveFavorite 删除收藏；不存在时幂等返回 nil（UI 重复点「取消」不报错）。
func (c *Catalog) RemoveFavorite(itemType, gameID, itemID string) error {
	itemType = strings.TrimSpace(itemType)
	gameID = strings.TrimSpace(gameID)
	itemID = strings.TrimSpace(itemID)
	if !ValidFavType(itemType) || gameID == "" || itemID == "" {
		return fmt.Errorf("删除收藏失败：参数不完整（%s/%s/%s）", itemType, gameID, itemID)
	}
	if _, err := c.db.Exec(
		`DELETE FROM favorites WHERE item_type = ? AND game_id = ? AND item_id = ?`,
		itemType, gameID, itemID); err != nil {
		return fmt.Errorf("删除收藏失败：%w", err)
	}
	return nil
}

// ListFavorites 收藏列表（新的在前）。itemType 为空 = 全部类型；
// 标题取 events（event）/ games（game）的 join，缺失回落 item_id。
func (c *Catalog) ListFavorites(itemType string, limit int) ([]Favorite, error) {
	itemType = strings.TrimSpace(itemType)
	if itemType != "" && !ValidFavType(itemType) {
		return nil, fmt.Errorf("收藏类型 %q 非法（可用：game/event/character）", itemType)
	}
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	where := ""
	args := make([]any, 0, 2)
	if itemType != "" {
		where = `WHERE f.item_type = ?`
		args = append(args, itemType)
	}
	args = append(args, limit)
	rows, err := c.db.Query(`
		SELECT f.item_type, f.game_id, f.item_id,
		       COALESCE(f.rating, 0), COALESCE(f.note, ''),
		       COALESCE(e.title, CASE WHEN f.item_type = 'game' THEN g.title END, f.item_id),
		       COALESCE(g.title, f.game_id),
		       f.created_at, f.updated_at
		FROM favorites f
		LEFT JOIN events e ON e.game_id = f.game_id AND e.event_id = f.item_id
		                   AND f.item_type = 'event'
		LEFT JOIN games g ON g.game_id = f.game_id
		`+where+`
		ORDER BY f.updated_at DESC
		LIMIT ?`, args...)
	if err != nil {
		return nil, fmt.Errorf("读收藏失败：%w", err)
	}
	defer rows.Close()
	out := make([]Favorite, 0, 16)
	seen := map[string]bool{} // events 主键是 (game_id, kind, event_id)，同 id 跨 kind 时防扇出重复行
	for rows.Next() {
		var f Favorite
		if err := rows.Scan(&f.ItemType, &f.GameID, &f.ItemID, &f.Rating, &f.Note,
			&f.Title, &f.GameTitle, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, fmt.Errorf("读收藏行失败：%w", err)
		}
		key := FavoriteKey(f.ItemType, f.GameID, f.ItemID)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历收藏失败：%w", err)
	}
	return out, nil
}

// FavoriteKey 收藏集合的 map 键（前端星标状态同理）。
func FavoriteKey(itemType, gameID, itemID string) string {
	return itemType + "|" + gameID + "|" + itemID
}
