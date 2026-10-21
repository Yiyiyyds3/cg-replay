// Package registry 管理「多游戏根目录登记」——库首页的游戏维度。
//
// 中心库最终会落到 SQLite（docs/02 / docs/06），MVP 先用一份 games.json
// 原子落盘，字段与未来的 games 表一一对应，避免二次迁移：
//
//	{ version, active, games: [{id,title,root,library,engine,added_at,last_played_at}] }
//
// id 由根目录路径的哈希决定（不是标题）：同一游戏换标题、重扫都不变；
// 不同副本即使同名也不会互相覆盖。library 指向该游戏自己的库清单。
package registry

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Version 当前登记表 schema 版本。
const Version = 1

// FileName 登记表文件名（与 cgreplay.conf 同目录）。
const FileName = "games.json"

// Game 一款游戏的登记项。
type Game struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Root         string `json:"root"`
	Library      string `json:"library,omitempty"`
	Engine       string `json:"engine,omitempty"`
	AddedAt      string `json:"added_at"`
	LastPlayedAt string `json:"last_played_at,omitempty"`
}

// Registry 游戏登记表。
type Registry struct {
	Version int    `json:"version"`
	Active  string `json:"active,omitempty"`
	Games   []Game `json:"games"`
}

// New 空登记表。
func New() *Registry { return &Registry{Version: Version} }

// Path 登记表在 dir 下的路径。
func Path(dir string) string { return filepath.Join(dir, FileName) }

// LibraryPath 某游戏的库清单路径（与 games.json 同级的 libraries/ 目录）。
func LibraryPath(dir, id string) string {
	return filepath.Join(dir, "libraries", id+".json")
}

// CleanRoot 规范化根目录：去空白、绝对路径、Clean。不做大小写转换
// （Windows 上大小写不敏感，但显示/回放仍用用户给的写法）。
func CleanRoot(root string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", errors.New("根目录为空")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

// IDFor 由规范化根目录生成稳定 id（g + sha1 前 10 位）。
// 路径比较用大小写不敏感（Windows 语义），因此同一目录换大小写仍是同一 id。
func IDFor(root string) string {
	clean, err := CleanRoot(root)
	if err != nil {
		clean = strings.TrimSpace(root)
	}
	key := strings.ToLower(filepath.ToSlash(clean))
	sum := sha1.Sum([]byte(key))
	return "g" + hex.EncodeToString(sum[:])[:10]
}

// Load 读登记表；文件不存在返回零值 + os.ErrNotExist（调用方据此做迁移）。
func Load(path string) (*Registry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var r Registry
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("解析游戏登记表 %s 失败：%w", path, err)
	}
	if r.Version == 0 {
		r.Version = Version
	}
	if r.Version > Version {
		return nil, fmt.Errorf("游戏登记表版本 %d 高于本程序支持的 %d", r.Version, Version)
	}
	return &r, nil
}

// Save 原子落盘（先写 .tmp 再 rename）。
func (r *Registry) Save(path string) error {
	if r.Version == 0 {
		r.Version = Version
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Find 按 id 找游戏。
func (r *Registry) Find(id string) *Game {
	for i := range r.Games {
		if r.Games[i].ID == id {
			return &r.Games[i]
		}
	}
	return nil
}

// ActiveGame 当前游戏；无有效 active 时返回 nil。
func (r *Registry) ActiveGame() *Game { return r.Find(r.Active) }

// Ensure 登记一个根目录（幂等）。返回登记项与是否新增。
// libraryPath 为空时保留已有路径；新增时按调用方给的值写入。
func (r *Registry) Ensure(root, libraryPath string) (*Game, bool) {
	clean, err := CleanRoot(root)
	if err != nil {
		clean = strings.TrimSpace(root)
	}
	for i := range r.Games {
		if sameRoot(r.Games[i].Root, clean) {
			if libraryPath != "" {
				r.Games[i].Library = libraryPath
			}
			return &r.Games[i], false
		}
	}
	g := Game{
		ID:      IDFor(clean),
		Title:   filepath.Base(clean),
		Root:    clean,
		Library: libraryPath,
		AddedAt: time.Now().Format(time.RFC3339),
	}
	r.Games = append(r.Games, g)
	return &r.Games[len(r.Games)-1], true
}

// SetActive 切换当前游戏；id 不存在返回错误。
func (r *Registry) SetActive(id string) error {
	if r.Find(id) == nil {
		return fmt.Errorf("游戏 %s 未登记", id)
	}
	r.Active = id
	return nil
}

// Remove 删除登记项；删的是当前游戏时自动切到第一项（没有则清空）。
func (r *Registry) Remove(id string) bool {
	for i := range r.Games {
		if r.Games[i].ID != id {
			continue
		}
		r.Games = append(r.Games[:i], r.Games[i+1:]...)
		if r.Active == id {
			r.Active = ""
			if len(r.Games) > 0 {
				r.Active = r.Games[0].ID
			}
		}
		return true
	}
	return false
}

// Touch 记录最近播放时间（失败由调用方决定是否忽略）。
func (r *Registry) Touch(id string) bool {
	g := r.Find(id)
	if g == nil {
		return false
	}
	g.LastPlayedAt = time.Now().Format(time.RFC3339)
	return true
}

func sameRoot(a, b string) bool {
	if strings.EqualFold(filepath.Clean(a), filepath.Clean(b)) {
		return true
	}
	// 双保险：不同分隔符写法（/ vs \）在 Windows 上等价。
	na := strings.EqualFold(filepath.ToSlash(filepath.Clean(a)), filepath.ToSlash(filepath.Clean(b)))
	return na
}
