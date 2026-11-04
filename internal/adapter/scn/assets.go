package scn

// assets.go —— 9nine 的素材索引与解析。
//
// PSB 里只存 storage 名（bg001a、ev101b、kr0021、bgm05），不带目录和扩展名。
// 不同解包方式的目录结构又不一样：
//   - 原样解包：raw/data/bgimage、raw/evimage、raw/voice/kr、raw/data/bgm
//   - 整理版：  原始归档解包/data_图片/bgimage、原始归档解包/evimage_图片、
//     原始归档解包/voice_音频/ep3、原始归档解包/data_音频/bgm
//
// 所以这里按「配置目录 + 默认目录」建一份 stem → 相对路径索引，一次遍历，
// 之后 O(1) 解析。索引按 root+目录缓存，只有首次事件构建会走盘。

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"ucgr/internal/ir"
)

// Options 一次事件构建的可配置项。
type Options struct {
	ImageDirs []string
	AudioDirs []string
	StageW    int
	StageH    int
}

// DefaultOptions 9nine 的舞台是 1280×720。
func DefaultOptions() Options {
	return Options{StageW: 1280, StageH: 720}
}

// Normalize 补默认舞台尺寸，去空目录。
func (o Options) Normalize() Options {
	if o.StageW <= 0 {
		o.StageW = 1280
	}
	if o.StageH <= 0 {
		o.StageH = 720
	}
	o.ImageDirs = cleanDirs(o.ImageDirs)
	o.AudioDirs = cleanDirs(o.AudioDirs)
	return o
}

func cleanDirs(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, d := range in {
		d = strings.TrimSpace(filepath.ToSlash(d))
		d = strings.Trim(d, "/")
		if d == "" || d == "." || seen[d] {
			continue
		}
		seen[d] = true
		out = append(out, d)
	}
	return out
}

// 默认目录只列实际存在的两类布局；不存在会自动跳过。
var defaultImageDirs = []string{
	"raw/data/bgimage", "raw/data/image", "raw/evimage", "raw/data/thum",
	"原始归档解包/data_图片/bgimage", "原始归档解包/data_图片/image",
	"原始归档解包/data_图片/thum", "原始归档解包/evimage_图片",
	// 扁平整理版（新章/春色/雪色）：所有图片铺在同一层，
	// 文件名带原始目录前缀（data_bgimage__bg001a.png）。
	"_分类/图片",
}

var defaultAudioDirs = []string{
	"raw/data/bgm", "raw/data/sound", "raw/voice",
	"原始归档解包/data_音频/bgm", "原始归档解包/data_音频/sound",
	"原始归档解包/voice_音频",
	"_分类/音频",
}

var mediaExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".webp": true, ".bmp": true,
	".tlg": true, ".hg3": true, ".hg2": true,
	".ogg": true, ".wav": true, ".mp3": true, ".m4a": true, ".opus": true,
	".mpg": true, ".mpeg": true, ".amv": true, ".wmv": true, ".mp4": true,
}

// AssetIndex stem（小写、去扩展名）→ 相对 root 的路径。
type AssetIndex struct {
	root   string
	images map[string]string
	audio  map[string]string
}

var indexCache sync.Map // key: root + "\x00" + dirs

func assetIndex(root string, opts Options) *AssetIndex {
	opts = opts.Normalize()
	key := root + "\x00" + strings.Join(opts.ImageDirs, ",") + "\x00" + strings.Join(opts.AudioDirs, ",")
	if v, ok := indexCache.Load(key); ok {
		return v.(*AssetIndex)
	}
	ix := &AssetIndex{root: root, images: map[string]string{}, audio: map[string]string{}}
	imgDirs := append(append([]string{}, defaultImageDirs...), opts.ImageDirs...)
	audioDirs := append(append([]string{}, defaultAudioDirs...), opts.AudioDirs...)
	for _, d := range imgDirs {
		ix.scan(filepath.Join(root, filepath.FromSlash(d)), ix.images)
	}
	for _, d := range audioDirs {
		ix.scan(filepath.Join(root, filepath.FromSlash(d)), ix.audio)
	}
	indexCache.Store(key, ix)
	return ix
}

func (ix *AssetIndex) scan(dir string, dst map[string]string) {
	st, err := os.Stat(dir)
	if err != nil || !st.IsDir() {
		return
	}
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if !mediaExts[ext] {
			return nil
		}
		rel, err := filepath.Rel(ix.root, path)
		if err != nil {
			return nil
		}
		stem := strings.ToLower(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
		put(dst, stem, filepath.ToSlash(rel))
		// 扁平整理版把目录压进文件名前缀（data_bgimage__bg001a、
		// evimage_ev106a），补一份去掉前缀的别名，PSB 里的 storage 名才命中。
		for _, alias := range stemAliases(stem) {
			put(dst, alias, filepath.ToSlash(rel))
		}
		return nil
	})
}

// put 先到先得：同名素材按目录优先级（默认目录在前）保留第一份。
func put(dst map[string]string, key, rel string) {
	if key == "" {
		return
	}
	if _, exists := dst[key]; !exists {
		dst[key] = rel
	}
}

// stemAliases 由扁平整理版文件名推出去前缀的 storage 名。
//
//	"data_bgimage__bg001a" → ["bg001a"]
//	"evimage_ev106a"       → ["ev106a"]
//	"fgimage_春風__春風Ａ2_0_6550" → ["春風Ａ2_0_6550"]（立绘，不参与回放）
func stemAliases(stem string) []string {
	var out []string
	add := func(s string) {
		if s != "" && s != stem {
			out = append(out, s)
		}
	}
	// 优先按 "__"（目录分隔被压成双下划线）取最后一段。
	if i := strings.LastIndex(stem, "__"); i >= 0 {
		add(stem[i+2:])
	}
	// 单下划线布局：<类别目录>_<文件名>，只认已知类别前缀，避免误切。
	for _, pre := range []string{"data_", "evimage_", "fgimage_", "video_", "voice_"} {
		if !strings.HasPrefix(stem, pre) {
			continue
		}
		rest := stem[len(pre):]
		if i := strings.Index(rest, "__"); i >= 0 {
			rest = rest[i+2:]
		}
		add(rest)
		break
	}
	return out
}

// Image 解析图片 storage 名，返回相对 root 的路径；解析不到时原样返回。
func (ix *AssetIndex) Image(ref string) string { return ix.resolve(ref, ix.images) }

// Audio 解析语音/BGM storage 名。
func (ix *AssetIndex) Audio(ref string) string { return ix.resolve(ref, ix.audio) }

// ResolveImage 用默认参数把 storage 名解析成相对 root 的路径。
// 事件库扫描（封面/页序列）用它把 PSB 里的裸名落成可直接加载的路径。
func ResolveImage(root, ref string) string {
	return assetIndex(root, DefaultOptions()).Image(ref)
}

// ResolveImageComposite 解析 CG 表里的合成引用 `差分?底图`。
//
// 新章的 sd CG 行写作 `sd_z01a?sd_z01_base1`：差分叠在底图上。本播放器
// 不做实时合成，优先取差分（CG 内容本体）；差分不在时退到底图。
func ResolveImageComposite(root, ref string) (string, string) {
	a, b, ok := strings.Cut(ref, "?")
	if !ok {
		return ResolveImage(root, ref), ""
	}
	if pa := ResolveImage(root, a); pa != "" && pa != a {
		return pa, ""
	}
	if pb := ResolveImage(root, b); pb != "" && pb != b {
		return pb, ""
	}
	return ResolveImage(root, a), ""
}

// ResolveAudio 同上，针对语音/BGM。
func ResolveAudio(root, ref string) string {
	return assetIndex(root, DefaultOptions()).Audio(ref)
}

func (ix *AssetIndex) resolve(ref string, m map[string]string) string {
	ref = strings.TrimSpace(filepath.ToSlash(ref))
	if ref == "" {
		return ""
	}
	// 已经带目录且文件存在：直接用。
	if strings.Contains(ref, "/") {
		if st, err := os.Stat(filepath.Join(ix.root, filepath.FromSlash(ref))); err == nil && !st.IsDir() {
			return ref
		}
	}
	stem := strings.ToLower(strings.TrimSuffix(filepath.Base(ref), filepath.Ext(ref)))
	if p, ok := m[stem]; ok {
		return p
	}
	// storage 名可能带扩展名（bg001a.png）；按 stem 再试一次。
	if p, ok := m[strings.ToLower(ref)]; ok {
		return p
	}
	return ref
}

// roleOf 资源角色分类（与 KiriKiri 适配器同口径，供 IR/裁剪报告使用）。
func roleOf(ref string) string {
	r := strings.ToLower(filepath.ToSlash(ref))
	switch {
	case strings.Contains(r, "/evimage/") || strings.Contains(r, "evimage_"), strings.HasPrefix(r, "ev"):
		return "CG事件图"
	case strings.Contains(r, "/bgimage/") || strings.Contains(r, "bgimage_"), strings.HasPrefix(r, "bg"):
		return "背景"
	case strings.Contains(r, "/voice/") || strings.Contains(r, "voice_"), strings.HasPrefix(r, "vo"):
		return "语音"
	case strings.Contains(r, "/bgm/") || strings.Contains(r, "data_音频/bgm"), strings.HasPrefix(r, "bgm"):
		return "BGM"
	case strings.Contains(r, "/sound/") || strings.Contains(r, "data_音频/sound"):
		return "音效"
	}
	return "其它"
}

// collectAssets 去重收集资源并生成 IR AssetRef（OrigPath 为相对 root 路径，
// 解析不到时保留原 storage 名，回放器仍可按 basename 兜底）。
func collectAssets(refs map[string]string, role func(string) string) []ir.AssetRef {
	if len(refs) == 0 {
		return nil
	}
	keys := make([]string, 0, len(refs))
	for k := range refs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]ir.AssetRef, 0, len(keys))
	for _, k := range keys {
		out = append(out, ir.AssetRef{OrigPath: refs[k], Role: role(k)})
	}
	return out
}
