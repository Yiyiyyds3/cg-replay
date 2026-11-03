// spine.go —— Spine 骨骼动画的素材装载（桌面壳专属）。
//
// Go 侧没有 Spine 渲染器：这里只把导出器产出的 skeleton JSON / atlas /
// 贴图读出来，连同动画元信息塞进 SessionMeta（只传一次），画面由前端
// Spine WebGL 组件按 FrameDTO.Now 同步渲染。
package main

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"ucgr/internal/adapter/unitycg"
)

const (
	maxSpineText  = 32 << 20 // skeleton JSON / atlas 单文件上限
	maxSpinePNG   = 16 << 20 // 单张贴图上限
	maxSpineTotal = 32 << 20 // 整个 Spine 会话（未 base64 前）上限
)

// SpineDTO 前端渲染 Spine 所需的全部素材（由 OpenEvent 一次性下发）。
type SpineDTO struct {
	Skeleton   string              `json:"skeleton"` // skeleton JSON 原文
	Atlas      string              `json:"atlas"`    // atlas 原文
	Textures   []SpineTextureDTO   `json:"textures"`
	Animations []SpineAnimationDTO `json:"animations"`
	Animation  string              `json:"animation"` // 默认动画名（空=前端取第一段）
	Loop       bool                `json:"loop"`
	Warnings   []string            `json:"warnings,omitempty"`
}

// SpineTextureDTO 一张 atlas 贴图（dataURL 避免 Wails 静态资源读不到游戏根目录）。
type SpineTextureDTO struct {
	Name    string `json:"name"`
	DataURL string `json:"data_url"`
}

// SpineAnimationDTO 动画元信息（时长毫秒，方便前端/进度条对齐）。
type SpineAnimationDTO struct {
	Name  string `json:"name"`
	Loop  bool   `json:"loop"`
	DurMs int64  `json:"dur_ms"`
}

// loadSpineAssets 读出一部 Spine CG 的渲染素材。
func loadSpineAssets(root string, cg unitycg.CG) (*SpineDTO, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("解析游戏目录失败：%w", err)
	}
	var jsonName, atlasName string
	for _, f := range cg.Files.Text {
		switch strings.ToLower(filepath.Ext(f)) {
		case ".json":
			if jsonName == "" {
				jsonName = f
			}
		case ".atlas":
			if atlasName == "" {
				atlasName = f
			}
		}
	}
	if jsonName == "" {
		return nil, fmt.Errorf("CG %s 缺少 skeleton JSON（files.text=%v）", cg.ID, cg.Files.Text)
	}
	if atlasName == "" {
		return nil, fmt.Errorf("CG %s 缺少 atlas 文件（files.text=%v）", cg.ID, cg.Files.Text)
	}

	raw, err := readSpineFile(absRoot, cg.ID, jsonName, maxSpineText)
	if err != nil {
		return nil, fmt.Errorf("读 skeleton 失败：%w", err)
	}
	atlasRaw, err := readSpineFile(absRoot, cg.ID, atlasName, maxSpineText)
	if err != nil {
		return nil, fmt.Errorf("读 atlas 失败：%w", err)
	}

	out := &SpineDTO{
		Skeleton: string(raw),
		Atlas:    string(atlasRaw),
	}
	if cg.Spine == nil {
		out.Warnings = append(out.Warnings, "索引缺少 Spine 统计，默认按第一段动画播放")
	} else if cg.Spine.Error != "" {
		out.Warnings = append(out.Warnings, "Spine 统计失败："+cg.Spine.Error)
	}
	for _, a := range cg.SpineAnimations() {
		out.Animations = append(out.Animations, SpineAnimationDTO{
			Name:  a.Name,
			Loop:  a.Loop,
			DurMs: int64(a.Dur * 1000),
		})
	}
	if a, ok := cg.PickSpineAnimation(); ok {
		out.Animation = a.Name
		out.Loop = a.Loop
	}

	var total int64
	for _, name := range cg.Files.PNG {
		data, rerr := readSpineFile(absRoot, cg.ID, name, maxSpinePNG)
		if rerr != nil {
			out.Warnings = append(out.Warnings, fmt.Sprintf("贴图 %s 读取失败：%v", name, rerr))
			continue
		}
		total += int64(len(data))
		if total > maxSpineTotal {
			return nil, fmt.Errorf("CG %s 的 Spine 贴图总大小超过 %d MB", cg.ID, maxSpineTotal>>20)
		}
		out.Textures = append(out.Textures, SpineTextureDTO{
			Name:    name,
			DataURL: "data:image/png;base64," + base64.StdEncoding.EncodeToString(data),
		})
	}
	if len(out.Textures) == 0 {
		return nil, fmt.Errorf("CG %s 没有可用的 Spine 贴图（files.png=%v）", cg.ID, cg.Files.PNG)
	}
	return out, nil
}

// readSpineFile 只允许 cg/<id>/<单个文件名>，并限制文件大小。
func readSpineFile(root, cgID, name string, maxSize int64) ([]byte, error) {
	if cgID == "" || cgID == "." || cgID == ".." || filepath.Base(cgID) != cgID ||
		strings.ContainsAny(cgID, `/\:`+"\x00") {
		return nil, fmt.Errorf("CG id 非法：%q", cgID)
	}
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name ||
		strings.ContainsAny(name, `/\:`+"\x00") {
		return nil, fmt.Errorf("文件名非法：%q", name)
	}
	p := filepath.Join(root, "cg", cgID, name)
	rel, err := filepath.Rel(root, p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("路径越出游戏目录：%s", name)
	}
	st, err := os.Stat(p)
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("不是普通文件：%s", name)
	}
	if st.Size() > maxSize {
		return nil, fmt.Errorf("%s 大小 %d MB，超过上限 %d MB", name, st.Size()>>20, maxSize>>20)
	}
	return os.ReadFile(p)
}
