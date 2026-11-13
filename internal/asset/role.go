// Package asset 统一「素材角色」与归档内相对路径的口径。
//
// 解包落盘、归档同名条目选择、回放加载三方共用同一套分类，避免出现
// 「脚本引用 ev001，磁盘上却把同名背景图塞进 CG 槽位」这类跨层错位。
package asset

import (
	"path/filepath"
	"regexp"
	"strings"
)

// Role 素材角色（同时也是落盘子目录名）。
type Role string

const (
	RoleCG    Role = "cg"    // 事件 CG / 画廊图
	RoleBG    Role = "bg"    // 背景、黑幕、白幕
	RoleStand Role = "stand" // 立绘 / 人物差分
	RoleMovie Role = "movie" // OP/ED 与事件影片
	RoleUI    Role = "ui"    // 系统界面、按钮、标题、图标
	RoleMisc  Role = "misc"  // 无法可靠分类的其它素材
)

// Roles 落盘时会用到的全部角色目录（顺序固定，便于测试与诊断）。
var Roles = []Role{RoleCG, RoleBG, RoleStand, RoleMovie, RoleUI, RoleMisc}

var (
	videoRe = regexp.MustCompile(`(?i)\.(mpg|mpeg|wmv|mp4|avi|webm|mov|mkv)$`)
	// 名字前缀必须落在分隔符上：bg01 / bg_scene 算 bg，bgm 不算。
	uiPrefixes    = []string{"sys", "ui", "btn", "button", "menu", "title", "logo", "icon", "config", "cursor"}
	moviePrefixes = []string{"movie", "opening", "ending", "op", "ed"}
	cgPrefixes    = []string{"cg", "ev", "event", "eventcg"}
	bgPrefixes    = []string{"bg", "back", "background", "black", "white", "背景", "黑幕", "白幕"}
	standPrefixes = []string{"fg", "stand", "chara", "tachie", "tachi", "sp", "立绘", "立ち絵"}
)

var mediaRoots = map[string]bool{
	"image": true, "images": true, "image00": true, "image01": true, "image02": true,
	"cgimage": true, "fgimage": true, "bgimage": true, "evimage": true, "sysimage": true,
	"movie": true, "movies": true, "video": true, "videos": true,
	"system": true, "sys": true, "ui": true, "graphics": true, "graphic": true,
}

// RoleDir 返回角色目录名。
func (r Role) Dir() string { return string(r) }

// ValidRole 报告 s 是否是已知角色目录名。
func ValidRole(s string) bool {
	for _, r := range Roles {
		if strings.EqualFold(s, string(r)) {
			return true
		}
	}
	return false
}

// RoleOf 按归档路径与文件名判断素材角色。
//
// 优先看目录（evimage/fgimage/bgimage/system/movie…），目录看不出时再看
// 文件名前缀。未知一律 misc，宁可让加载器回退查找，也不把 UI/背景塞进 CG。
func RoleOf(p string) Role {
	s := strings.ToLower(filepath.ToSlash(strings.TrimSpace(p)))
	if s == "" {
		return RoleMisc
	}
	if videoRe.MatchString(s) {
		return RoleMovie
	}
	segs := strings.Split(s, "/")
	// 目录命中（最后一段是文件名，不参与目录判断）。
	for _, seg := range segs[:max(0, len(segs)-1)] {
		switch seg {
		case "ev", "evimage", "evimages", "event", "events", "eventcg", "cg", "cgs":
			return RoleCG
		case "fg", "fgimage", "fgimages", "chara", "chars", "tachie", "tachi", "stand", "stands", "立绘", "立ち絵":
			return RoleStand
		case "bgimage", "bgimages", "background", "backgrounds", "bg", "backs", "背景":
			return RoleBG
		case "sys", "sysimage", "system", "ui", "menu", "menus", "button", "buttons", "icon", "icons", "logo", "title":
			return RoleUI
		case "movie", "movies", "video", "videos", "anime":
			return RoleMovie
		}
	}
	base := strings.TrimSuffix(segs[len(segs)-1], filepath.Ext(segs[len(segs)-1]))
	base = strings.TrimSpace(base)
	switch {
	case hasPrefix(base, uiPrefixes...):
		return RoleUI
	case hasPrefix(base, moviePrefixes...):
		return RoleMovie
	case hasPrefix(base, cgPrefixes...):
		return RoleCG
	case hasPrefix(base, bgPrefixes...):
		return RoleBG
	case hasPrefix(base, standPrefixes...):
		return RoleStand
	}
	return RoleMisc
}

// hasPrefix 判断 name 是否以 prefixes 之一开头，且前缀后是分隔符/数字/结尾。
// 这样 bg01、bg_scene 命中 bg，bgm、edge 不会误命中。
func hasPrefix(name string, prefixes ...string) bool {
	for _, p := range prefixes {
		if !strings.HasPrefix(name, p) {
			continue
		}
		if len(name) == len(p) {
			return true
		}
		c := name[len(p)]
		if c == '_' || c == '-' || c == '.' || c == ' ' || (c >= '0' && c <= '9') {
			return true
		}
	}
	return false
}

// Relative 把归档条目路径整理成「保留原子目录、可安全落盘」的相对路径。
//
// 会剥掉归档根（raw/data、data）与最外层素材根（image、fgimage、movie…），
// 但保留其后的目录层级，因此 image/ev/a.png 与 image/bg/a.png 落盘后仍是
// 两个不同文件。空路径回退到文件名，绝不返回绝对路径或包含 .. 的路径。
func Relative(p string) string {
	s := filepath.ToSlash(strings.TrimSpace(p))
	s = strings.TrimPrefix(s, filepath.VolumeName(s))
	raw := strings.Split(s, "/")
	parts := make([]string, 0, len(raw))
	for _, part := range raw {
		part = strings.TrimSpace(part)
		if part == "" || part == "." || part == ".." || strings.HasSuffix(part, ":") {
			continue
		}
		parts = append(parts, safeSegment(part))
	}
	// 归档根前缀。
	for len(parts) > 1 {
		switch strings.ToLower(parts[0]) {
		case "raw", "data":
			parts = parts[1:]
		default:
			goto media
		}
	}
media:
	// 只剥一层素材根：保留 image/ev 里的 ev，同时避免 image/image 这种重复。
	if len(parts) > 1 && mediaRoots[strings.ToLower(parts[0])] {
		parts = parts[1:]
	}
	if len(parts) == 0 {
		return "unnamed"
	}
	return strings.Join(parts, "/")
}

// RelativeForRole 与 Relative 类似，并在首段已是同类角色目录时去掉它。
func RelativeForRole(p string, role Role) string {
	rel := Relative(p)
	parts := strings.Split(rel, "/")
	if len(parts) > 1 && strings.EqualFold(parts[0], role.Dir()) {
		rel = strings.Join(parts[1:], "/")
	}
	if rel == "" {
		return "unnamed"
	}
	return rel
}

// DestPath 计算解包产物中素材的落盘路径：
//
//	<dest>/<imageDir>/<role>/<保留的原相对子路径>
func DestPath(dest, imageDir, src string) string {
	role := RoleOf(src)
	rel := RelativeForRole(src, role)
	return filepath.Join(dest, filepath.FromSlash(imageDir), role.Dir(), filepath.FromSlash(rel))
}

// safeSegment 去掉 Windows 不允许的字符（归档名可能来自日文/中文环境）。
func safeSegment(s string) string {
	const bad = `<>:"|?*`
	s = strings.Map(func(r rune) rune {
		if strings.ContainsRune(bad, r) {
			return '_'
		}
		return r
	}, s)
	s = strings.TrimRight(s, ". ")
	if s == "" {
		return "unnamed"
	}
	return s
}
