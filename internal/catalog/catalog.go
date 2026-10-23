// Package catalog 是中心库的 SQLite 目录（MVP 第一版）。
//
// 它只存「可检索的元数据」：游戏、事件、台词、收藏、最近播放；
// 回放仍然读 library JSON + 原始资产，不经过本包。这样索引失败不会
// 改变回放语义，后续跨游戏卡片墙/全文检索只需要查这一张库。
//
// 表结构对齐 docs/02 的 games/events/favorites 子集；台词全文用
// FTS5 trigram（支持中文子串），短于 3 个字符时退回 LIKE。
package catalog

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	_ "modernc.org/sqlite"

	"ucgr/internal/library"
	"ucgr/internal/registry"
)

// Version 当前 catalog schema 版本。
const Version = 1

// FileName 目录库文件名（与 games.json 同目录）。
const FileName = "catalog.db"

const schema = `
CREATE TABLE IF NOT EXISTS meta (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS games (
  game_id         TEXT PRIMARY KEY,
  title           TEXT NOT NULL,
  root            TEXT NOT NULL,
  library_path    TEXT,
  engine          TEXT,
  fidelity        TEXT NOT NULL DEFAULT 'generic',
  mode            TEXT NOT NULL DEFAULT 'takeover',
  cg_count        INTEGER NOT NULL DEFAULT 0,
  scene_count     INTEGER NOT NULL DEFAULT 0,
  chapter_count   INTEGER NOT NULL DEFAULT 0,
  event_count     INTEGER NOT NULL DEFAULT 0,
  added_at        TEXT NOT NULL,
  last_played_at  TEXT,
  indexed_at      TEXT NOT NULL,
  created_at      TEXT NOT NULL,
  updated_at      TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS events (
  game_id          TEXT NOT NULL REFERENCES games(game_id) ON DELETE CASCADE,
  kind             TEXT NOT NULL,
  event_id         TEXT NOT NULL,
  title            TEXT,
  ord              INTEGER NOT NULL,
  script           TEXT,
  thumb            TEXT,
  pages            INTEGER NOT NULL DEFAULT 0,
  scene            TEXT,
  label_pos        INTEGER NOT NULL DEFAULT 0,
  cont_script      TEXT,
  cont_label_pos   INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (game_id, kind, event_id)
);
CREATE INDEX IF NOT EXISTS idx_events_game_kind ON events(game_id, kind, ord);

CREATE TABLE IF NOT EXISTS text_lines (
  line_id   INTEGER PRIMARY KEY AUTOINCREMENT,
  game_id   TEXT NOT NULL REFERENCES games(game_id) ON DELETE CASCADE,
  kind      TEXT NOT NULL,
  event_id  TEXT NOT NULL,
  t_ms      INTEGER,
  speaker   TEXT,
  text      TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_text_lines_event ON text_lines(game_id, kind, event_id);

CREATE VIRTUAL TABLE IF NOT EXISTS text_lines_fts USING fts5(
  text,
  speaker,
  content = 'text_lines',
  content_rowid = 'line_id',
  tokenize = 'trigram'
);

CREATE TRIGGER IF NOT EXISTS text_lines_ai AFTER INSERT ON text_lines BEGIN
  INSERT INTO text_lines_fts(rowid, text, speaker)
  VALUES (new.line_id, new.text, COALESCE(new.speaker, ''));
END;
CREATE TRIGGER IF NOT EXISTS text_lines_ad AFTER DELETE ON text_lines BEGIN
  INSERT INTO text_lines_fts(text_lines_fts, rowid, text, speaker)
  VALUES ('delete', old.line_id, old.text, COALESCE(old.speaker, ''));
END;
CREATE TRIGGER IF NOT EXISTS text_lines_au AFTER UPDATE ON text_lines BEGIN
  INSERT INTO text_lines_fts(text_lines_fts, rowid, text, speaker)
  VALUES ('delete', old.line_id, old.text, COALESCE(old.speaker, ''));
  INSERT INTO text_lines_fts(rowid, text, speaker)
  VALUES (new.line_id, new.text, COALESCE(new.speaker, ''));
END;

CREATE TABLE IF NOT EXISTS favorites (
  fav_id     INTEGER PRIMARY KEY AUTOINCREMENT,
  item_type  TEXT NOT NULL CHECK (item_type IN ('game','event','character')),
  game_id    TEXT NOT NULL,
  item_id    TEXT NOT NULL,
  rating     INTEGER CHECK (rating BETWEEN 1 AND 5),
  note       TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE (item_type, game_id, item_id)
);

CREATE TABLE IF NOT EXISTS recent (
  game_id   TEXT NOT NULL,
  event_id  TEXT NOT NULL,
  kind      TEXT NOT NULL,
  played_at TEXT NOT NULL,
  play_count INTEGER NOT NULL DEFAULT 1,
  PRIMARY KEY (game_id, event_id)
);
CREATE INDEX IF NOT EXISTS idx_recent_played ON recent(played_at DESC);
`

// Catalog 一个打开的中心库。
type Catalog struct {
	db   *sql.DB
	path string
}

// Open 打开（不存在则创建）catalog.db，并执行幂等迁移。
func Open(path string) (*Catalog, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("catalog 路径为空")
	}
	if path != ":memory:" && !strings.HasPrefix(path, "file:") {
		if dir := filepath.Dir(path); dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, fmt.Errorf("创建 catalog 目录失败：%w", err)
			}
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("打开 catalog 失败：%w", err)
	}
	// modernc 的 SQLite 是单写者；桌面端只有一个连接，避免 SQLITE_BUSY。
	db.SetMaxOpenConns(1)
	for _, pragma := range []string{
		"PRAGMA busy_timeout = 5000",
		"PRAGMA foreign_keys = ON",
	} {
		if _, err := db.Exec(pragma); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("catalog %s 失败：%w", pragma, err)
		}
	}
	if path != ":memory:" {
		if _, err := db.Exec("PRAGMA journal_mode = WAL"); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("catalog 开启 WAL 失败：%w", err)
		}
		// 启动时把上次会话残留的 WAL 合并回主库（TRUNCATE）。
		// 强杀/崩溃退出时 SQLite 不会 checkpoint，WAL 会随每次启动累积
		// （2026-10-23 体检实测：生产 exe 每次启动 +约 74 KB，主库 139 KB
		// 一直没并进去）。启动期收敛一次即可，与「优雅退出时 Close」互补。
		// best-effort：合并失败不影响可用性（WAL 本身是持久的），故忽略错误。
		// 注意 wal_checkpoint 会返回一行结果，用 QueryRow 取掉，避免 Exec 报错。
		var busy, logFrames, done int
		_ = db.QueryRow("PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &logFrames, &done)
	}
	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Catalog{db: db, path: path}, nil
}

func migrate(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("catalog 迁移开始失败：%w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(schema); err != nil {
		return fmt.Errorf("catalog 建表失败：%w", err)
	}

	var raw string
	err = tx.QueryRow(`SELECT value FROM meta WHERE key = 'schema_version'`).Scan(&raw)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if _, err := tx.Exec(`INSERT INTO meta(key, value) VALUES ('schema_version', ?)`, strconv.Itoa(Version)); err != nil {
			return fmt.Errorf("catalog 写 schema_version 失败：%w", err)
		}
		if _, err := tx.Exec(`INSERT INTO meta(key, value) VALUES ('created_at', ?)`, now()); err != nil {
			return fmt.Errorf("catalog 写 created_at 失败：%w", err)
		}
	case err != nil:
		return fmt.Errorf("catalog 读 schema_version 失败：%w", err)
	default:
		v, err := strconv.Atoi(raw)
		if err != nil {
			return fmt.Errorf("catalog schema_version=%q 无法解析", raw)
		}
		if v > Version {
			return fmt.Errorf("catalog 版本 %d 高于本程序支持的 %d", v, Version)
		}
	}
	return tx.Commit()
}

// Close 关闭目录库。
func (c *Catalog) Close() error {
	if c == nil || c.db == nil {
		return nil
	}
	return c.db.Close()
}

// Path 返回打开时的路径（测试/诊断用）。
func (c *Catalog) Path() string { return c.path }

// Stats 目录规模。
type Stats struct {
	Games     int `json:"games"`
	Events    int `json:"events"`
	TextLines int `json:"text_lines"`
}

// Stats 统计当前目录库。
func (c *Catalog) Stats() (Stats, error) {
	var s Stats
	err := c.db.QueryRow(`
		SELECT (SELECT COUNT(*) FROM games),
		       (SELECT COUNT(*) FROM events),
		       (SELECT COUNT(*) FROM text_lines)`).
		Scan(&s.Games, &s.Events, &s.TextLines)
	if err != nil {
		return Stats{}, fmt.Errorf("读 catalog 统计失败：%w", err)
	}
	return s, nil
}

// Event 跨游戏事件行（卡片墙/搜索结果共用）。
type Event struct {
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

// SyncGame 用一份 library 清单覆盖某游戏的事件索引（幂等）。
func (c *Catalog) SyncGame(g registry.Game, lib *library.Library) error {
	if strings.TrimSpace(g.ID) == "" {
		return errors.New("同步 catalog 失败：游戏 id 为空")
	}
	if lib == nil {
		return fmt.Errorf("同步 catalog 失败：游戏「%s」库清单为空", g.Title)
	}
	rows := eventsFor(g, lib)
	now := now()
	added := strings.TrimSpace(g.AddedAt)
	if added == "" {
		added = now
	}
	title := strings.TrimSpace(g.Title)
	if title == "" {
		title = filepath.Base(strings.TrimRight(g.Root, `\/`))
	}
	if title == "" {
		title = g.ID
	}

	tx, err := c.db.Begin()
	if err != nil {
		return fmt.Errorf("同步 catalog 开始失败：%w", err)
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.Exec(`
		INSERT INTO games(
			game_id, title, root, library_path, engine, cg_count, scene_count,
			chapter_count, event_count, added_at, last_played_at, indexed_at,
			created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(game_id) DO UPDATE SET
			title = excluded.title,
			root = excluded.root,
			library_path = excluded.library_path,
			engine = excluded.engine,
			cg_count = excluded.cg_count,
			scene_count = excluded.scene_count,
			chapter_count = excluded.chapter_count,
			event_count = excluded.event_count,
			last_played_at = excluded.last_played_at,
			indexed_at = excluded.indexed_at,
			updated_at = excluded.updated_at`,
		g.ID, title, g.Root, g.Library, g.Engine,
		len(lib.CGs), len(lib.Scenes), len(lib.Chapters), len(rows),
		added, g.LastPlayedAt, now, added, now)
	if err != nil {
		return fmt.Errorf("同步 catalog 游戏「%s」失败：%w", title, err)
	}
	if _, err := tx.Exec(`DELETE FROM events WHERE game_id = ?`, g.ID); err != nil {
		return fmt.Errorf("清理 catalog 事件失败：%w", err)
	}
	stmt, err := tx.Prepare(`
		INSERT INTO events(
			game_id, kind, event_id, title, ord, script, thumb, pages,
			scene, label_pos, cont_script, cont_label_pos)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("准备 catalog 事件写入失败：%w", err)
	}
	defer stmt.Close()
	for _, e := range rows {
		if _, err := stmt.Exec(
			e.gameID, e.kind, e.id, e.title, e.ord, e.script, e.thumb, e.pages,
			e.scene, e.labelPos, e.contScript, e.contLabelPos); err != nil {
			return fmt.Errorf("写 catalog 事件 %s/%s 失败：%w", e.kind, e.id, err)
		}
	}
	// 事件被重扫删除时，对应台词（含 FTS 触发器）一并清掉，避免搜到幽灵行。
	if _, err := tx.Exec(`
		DELETE FROM text_lines
		WHERE game_id = ?
		  AND NOT EXISTS (
			SELECT 1 FROM events e
			WHERE e.game_id = text_lines.game_id
			  AND e.kind = text_lines.kind
			  AND e.event_id = text_lines.event_id
		  )`, g.ID); err != nil {
		return fmt.Errorf("清理 catalog 幽灵台词失败：%w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("同步 catalog 提交失败：%w", err)
	}
	return nil
}

type eventRow struct {
	gameID, kind, id, title string
	ord                     int
	script, thumb           string
	pages                   int
	scene                   string
	labelPos                int
	contScript              string
	contLabelPos            int
}

func eventsFor(g registry.Game, lib *library.Library) []eventRow {
	out := make([]eventRow, 0, len(lib.CGs)+len(lib.Scenes)+len(lib.Chapters))
	ord := 0
	for _, c := range lib.CGs {
		title := strings.TrimSpace(c.Title)
		if title == "" {
			title = c.ID
		}
		out = append(out, eventRow{
			gameID: g.ID, kind: "cg", id: c.ID, title: title, ord: ord,
			script: c.Script, thumb: c.Thumb, pages: len(c.Pages),
		})
		ord++
	}
	for _, s := range lib.Scenes {
		title := strings.TrimSpace(s.Title)
		if title == "" {
			title = s.ID
		}
		out = append(out, eventRow{
			gameID: g.ID, kind: "scene", id: s.ID, title: title, ord: ord,
			script: s.Script,
		})
		ord++
	}
	for _, ch := range lib.Chapters {
		title := strings.TrimSpace(ch.Title)
		if title == "" {
			title = ch.Name
		}
		if title == "" {
			title = ch.ID
		}
		out = append(out, eventRow{
			gameID: g.ID, kind: "chapter", id: ch.ID, title: title, ord: ord,
			script: ch.Script, scene: ch.Scene, labelPos: ch.LabelPos,
			contScript: ch.ContScript, contLabelPos: ch.ContLabelPos,
		})
		ord++
	}
	return out
}

// Events 按游戏标题 + 游戏内顺序返回事件；limit<=0 用默认值。
func (c *Catalog) Events(limit int) ([]Event, error) {
	return c.queryEvents("", nil, limit)
}

// SearchEvents 按事件标题、事件 id、游戏标题做子串检索。
func (c *Catalog) SearchEvents(q string, limit int) ([]Event, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return c.Events(limit)
	}
	pat := likePattern(q)
	return c.queryEvents(
		`WHERE e.title LIKE ? ESCAPE '\' OR e.event_id LIKE ? ESCAPE '\' OR g.title LIKE ? ESCAPE '\'`,
		[]any{pat, pat, pat}, limit)
}

func (c *Catalog) queryEvents(where string, args []any, limit int) ([]Event, error) {
	if limit <= 0 {
		limit = 500
	}
	if limit > 5000 {
		limit = 5000
	}
	sqlText := `
		SELECT e.game_id, g.title, g.root, e.kind, e.event_id,
		       COALESCE(e.title, ''), e.ord, e.pages
		FROM events e
		JOIN games g ON g.game_id = e.game_id
		` + where + `
		ORDER BY g.title COLLATE NOCASE, e.ord
		LIMIT ?`
	args = append(args, limit)
	rows, err := c.db.Query(sqlText, args...)
	if err != nil {
		return nil, fmt.Errorf("查 catalog 事件失败：%w", err)
	}
	defer rows.Close()
	out := make([]Event, 0, 64)
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.GameID, &e.GameTitle, &e.GameRoot, &e.Kind,
			&e.ID, &e.Title, &e.Ord, &e.Pages); err != nil {
			return nil, fmt.Errorf("读 catalog 事件失败：%w", err)
		}
		e.HasCover = e.Kind == "cg" && e.Pages > 0
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 catalog 事件失败：%w", err)
	}
	return out, nil
}

// TextLine 一句台词（未来由 IR/文本抽取写入）。
type TextLine struct {
	GameID  string
	Kind    string
	EventID string
	TMs     int64
	Speaker string
	Text    string
}

// ReplaceTextLines 覆盖台词。
//   - kind 与 eventID 都非空：覆盖该事件；
//   - 都为空：覆盖整个游戏，逐行读取 Kind/EventID；
//   - 只填一个：报错（事件必须由 kind+eventID 唯一定位）。
func (c *Catalog) ReplaceTextLines(gameID, kind, eventID string, lines []TextLine) error {
	gameID = strings.TrimSpace(gameID)
	kind = strings.TrimSpace(kind)
	eventID = strings.TrimSpace(eventID)
	if gameID == "" {
		return errors.New("写 catalog 台词失败：游戏 id 为空")
	}
	if (kind == "") != (eventID == "") {
		return errors.New("写 catalog 台词失败：kind 与 eventID 必须同时为空或同时非空")
	}
	tx, err := c.db.Begin()
	if err != nil {
		return fmt.Errorf("写 catalog 台词开始失败：%w", err)
	}
	defer func() { _ = tx.Rollback() }()
	switch {
	case eventID != "":
		if _, err := tx.Exec(`DELETE FROM text_lines WHERE game_id = ? AND kind = ? AND event_id = ?`, gameID, kind, eventID); err != nil {
			return fmt.Errorf("清理 catalog 台词失败：%w", err)
		}
	default:
		if _, err := tx.Exec(`DELETE FROM text_lines WHERE game_id = ?`, gameID); err != nil {
			return fmt.Errorf("清理 catalog 台词失败：%w", err)
		}
	}
	stmt, err := tx.Prepare(`INSERT INTO text_lines(game_id, kind, event_id, t_ms, speaker, text) VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("准备 catalog 台词写入失败：%w", err)
	}
	defer stmt.Close()
	for _, l := range lines {
		if strings.TrimSpace(l.Text) == "" {
			continue
		}
		lineKind, lineEvent := kind, eventID
		if lineEvent == "" {
			lineKind = strings.TrimSpace(l.Kind)
			lineEvent = strings.TrimSpace(l.EventID)
		}
		if lineKind == "" || lineEvent == "" {
			return fmt.Errorf("写 catalog 台词失败：台词缺少 kind/event_id（%q）", l.Text)
		}
		if _, err := stmt.Exec(gameID, lineKind, lineEvent, l.TMs, l.Speaker, l.Text); err != nil {
			return fmt.Errorf("写 catalog 台词失败：%w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("写 catalog 台词提交失败：%w", err)
	}
	return nil
}

// TextHit 一条台词命中。
type TextHit struct {
	LineID    int64  `json:"line_id"`
	GameID    string `json:"game_id"`
	GameTitle string `json:"game_title"`
	Kind      string `json:"kind"`
	EventID   string `json:"event_id"`
	TMs       int64  `json:"t_ms"`
	Speaker   string `json:"speaker"`
	Text      string `json:"text"`
}

// SearchText 搜台词。>=3 字符走 FTS5 trigram（中文子串可用），
// 更短的查询退回 LIKE，避免 trigram 对 1~2 字符无结果。
func (c *Catalog) SearchText(q string, limit int) ([]TextHit, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return []TextHit{}, nil
	}
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	base := `
		SELECT t.line_id, t.game_id, g.title, t.kind, t.event_id,
		       COALESCE(t.t_ms, 0), COALESCE(t.speaker, ''), t.text
		FROM text_lines t
		JOIN games g ON g.game_id = t.game_id
		JOIN events e ON e.game_id = t.game_id AND e.kind = t.kind AND e.event_id = t.event_id
		`
	var (
		rows *sql.Rows
		err  error
	)
	if utf8.RuneCountInString(q) >= 3 {
		rows, err = c.db.Query(base+`
			JOIN text_lines_fts f ON f.rowid = t.line_id
			WHERE text_lines_fts MATCH ?
			ORDER BY rank
			LIMIT ?`, ftsPhrase(q), limit)
	} else {
		pat := likePattern(q)
		rows, err = c.db.Query(base+`
			WHERE t.text LIKE ? ESCAPE '\' OR COALESCE(t.speaker, '') LIKE ? ESCAPE '\'
			ORDER BY t.game_id, t.kind, t.event_id, t.t_ms
			LIMIT ?`, pat, pat, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("搜 catalog 台词失败：%w", err)
	}
	defer rows.Close()
	out := make([]TextHit, 0, 32)
	for rows.Next() {
		var h TextHit
		if err := rows.Scan(&h.LineID, &h.GameID, &h.GameTitle, &h.Kind, &h.EventID,
			&h.TMs, &h.Speaker, &h.Text); err != nil {
			return nil, fmt.Errorf("读 catalog 台词失败：%w", err)
		}
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 catalog 台词失败：%w", err)
	}
	return out, nil
}

func now() string { return time.Now().UTC().Format(time.RFC3339) }

func likePattern(q string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q)
	return "%" + r + "%"
}

func ftsPhrase(q string) string {
	return `"` + strings.ReplaceAll(q, `"`, `""`) + `"`
}
