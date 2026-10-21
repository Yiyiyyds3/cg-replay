// speakers.go —— 中心库的说话人聚合（character 级收藏的前置）。
//
// 数据来源就是 text_lines（台词抽取的产物）：按 (game_id, speaker) 分组
// 计数。空 speaker（旁白/未署名）不计角色——抽取时 speaker 为空会沿用
// 上一个说话人，真正没署名的行极少，宁缺勿假。
package catalog

import (
	"fmt"
	"strings"
)

// Speaker 一个说话人（角色维度）的聚合行。
type Speaker struct {
	GameID    string `json:"game_id"`
	GameTitle string `json:"game_title"`
	Name      string `json:"name"`
	Lines     int    `json:"lines"`  // 台词条数
	Events    int    `json:"events"` // 出现的事件数
}

// Speakers 聚合说话人，按条数降序。
//   - gameID 为空 = 全部已索引游戏（前端「角色」区跨游戏用）；
//   - gameID 非空 = 只看该游戏；
//   - limit<=0 用默认值（500）。
func (c *Catalog) Speakers(gameID string, limit int) ([]Speaker, error) {
	gameID = strings.TrimSpace(gameID)
	if limit <= 0 {
		limit = 500
	}
	if limit > 2000 {
		limit = 2000
	}
	where := `WHERE t.speaker <> ''`
	args := make([]any, 0, 2)
	if gameID != "" {
		where += ` AND t.game_id = ?`
		args = append(args, gameID)
	}
	args = append(args, limit)
	rows, err := c.db.Query(`
		SELECT t.game_id, COALESCE(g.title, t.game_id), t.speaker,
		       COUNT(*), COUNT(DISTINCT t.event_id)
		FROM text_lines t
		LEFT JOIN games g ON g.game_id = t.game_id
		`+where+`
		GROUP BY t.game_id, t.speaker
		ORDER BY COUNT(*) DESC, t.speaker COLLATE NOCASE
		LIMIT ?`, args...)
	if err != nil {
		return nil, fmt.Errorf("聚合说话人失败：%w", err)
	}
	defer rows.Close()
	out := make([]Speaker, 0, 32)
	for rows.Next() {
		var s Speaker
		if err := rows.Scan(&s.GameID, &s.GameTitle, &s.Name, &s.Lines, &s.Events); err != nil {
			return nil, fmt.Errorf("读说话人行失败：%w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历说话人失败：%w", err)
	}
	return out, nil
}
