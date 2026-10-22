// 解包导入（阶段 1：KiriKiri/XP3）。
//
// 流程：用户给一个游戏目录或 .xp3 → 预览「会解出什么」→ 确认 → 解出
// **只够重演 CG 事件**的最小集合到每用户数据目录 → 登记进游戏库 → 直接可播。
//
// 落盘布局与现有加载器一致（raw/data/scenario + raw/data/image + 约定文件），
// 所以解包产物不需要加载侧任何改动。
package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"ucgr/internal/library"
	"ucgr/internal/registry"
	"ucgr/internal/unpack"
)

// UnpackPlanDTO 解包预览（前端展示用）。
type UnpackPlanDTO struct {
	EngineID      string   `json:"engine_id"`
	Kind          string   `json:"kind"` // loose / xp3
	Archives      []string `json:"archives"`
	ScenarioFiles int      `json:"scenario_files"`
	Labels        int      `json:"labels"` // 预计的 CG 事件数
	Refs          int      `json:"refs"`   // 预计引用到的素材数
	Encrypted     int      `json:"encrypted"`
	Notes         []string `json:"notes"`
	Unsupported   bool     `json:"unsupported"`
	Reason        string   `json:"reason"`
}

// UnpackImportDTO 导入结果（解包 + 登记）。
type UnpackImportDTO struct {
	Dest      string   `json:"dest"`
	EngineID  string   `json:"engine_id"`
	Events    int      `json:"events"`
	Files     int      `json:"files"`
	Bytes     int64    `json:"bytes"`
	Missing   []string `json:"missing"`
	Encrypted []string `json:"encrypted"`
	Warnings  []string `json:"warnings"`
	GameID    string   `json:"game_id"`
	// Connected=false 表示素材已解出，但事件库还未接入（目前只支持 KiriKiri）。
	// 这不是解包失败：产物保留在 Dest，前端要给打开目录/手动退路，不能只弹一句错。
	Connected bool   `json:"connected"`
	Reason    string `json:"reason,omitempty"`
	Hint      string `json:"hint,omitempty"`
	// Library 登记后的事件库（形状与 library.Library 一致，前端直接当 LibraryDTO 用）。
	Library *library.Library `json:"library,omitempty"`
}

// UnpackPreview 预览一个游戏目录/归档的解包计划（不落盘）。
func (a *App) UnpackPreview(path string) (*UnpackPlanDTO, error) {
	plan, err := unpack.Preview(strings.TrimSpace(path))
	if err != nil {
		return nil, err
	}
	return &UnpackPlanDTO{
		EngineID:      plan.EngineID,
		Kind:          plan.Kind,
		Archives:      plan.Archives,
		ScenarioFiles: plan.ScenarioFiles,
		Labels:        plan.Labels,
		Refs:          plan.Refs,
		Encrypted:     plan.Encrypted,
		Notes:         plan.Notes,
		Unsupported:   plan.Unsupported,
		Reason:        plan.Reason,
	}, nil
}

// UnpackImportDecrypt 带解密方案地解包（方案名见 unpack.SchemeFromString；
// 固定密钥 key 供 "xor" 方案用；其余方案由条目自身的 adlr 派生密钥）。
func (a *App) UnpackImportDecrypt(path, scheme string, key int) (*UnpackImportDTO, error) {
	sc, err := unpack.SchemeFromString(scheme)
	if err != nil {
		return nil, err
	}
	return a.unpackImport(path, unpack.Options{
		Scope:   unpack.ScopeCG,
		Decrypt: sc,
		Key:     byte(key),
	})
}

// UnpackImport 解包并登记：解出最小集合 → 写进每用户数据目录 → 登记为游戏。
//
// 目标目录固定为 `%LOCALAPPDATA%\cgreplay-ui\games\<源路径 id>`，同一份源
// 重复导入落在同一处（幂等）。登记时以**解包产物目录**为游戏根目录，
// 播放器直接就能读。
func (a *App) UnpackImport(path string) (*UnpackImportDTO, error) {
	return a.unpackImport(path, unpack.Options{Scope: unpack.ScopeCG})
}

// unpackImport 真正的解包 + 登记流程（两种入口共用）。
func (a *App) unpackImport(path string, opts unpack.Options) (*UnpackImportDTO, error) {
	src := strings.TrimSpace(path)
	if src == "" {
		return nil, fmt.Errorf("没有选择要导入的路径")
	}
	plan, err := unpack.Preview(src)
	if err != nil {
		return nil, err
	}
	if plan.Unsupported {
		return nil, fmt.Errorf("这个路径没法自动解包：%s", plan.Reason)
	}
	if plan.Kind == "loose" {
		return nil, fmt.Errorf("这已经是解包目录了，直接用「添加游戏」登记就行（不必再解一遍）")
	}

	// 目标目录：按源路径 id 固定，重复导入幂等。
	id := registry.IDFor(src)
	dest := filepath.Join(dataDir(), "games", id)
	res, err := unpack.ExtractWith(src, dest, opts)
	if err != nil {
		return nil, err
	}
	if res.Files == 0 {
		return nil, fmt.Errorf("解包后没有任何文件（%s）", src)
	}

	// 登记进游戏库（以解包产物为根目录）。
	// 注意：事件库切分（library.Scan）目前是 KiriKiri 形状的——别的引擎
	// 解出来了也接不进事件库，这种要**明说**，不能报一个含糊的错。
	lib, err := a.AddGame(dest)
	if err != nil {
		if res.EngineID != "kirikiri" {
			// 解包本身成功，只是事件库接不上；用结果 DTO 返回而不是错误字符串，
			// 前端才能保留产物路径、给「打开目录」和手动退路。
			return &UnpackImportDTO{
				Dest:      res.Dest,
				EngineID:  res.EngineID,
				Events:    res.Events,
				Files:     res.Files,
				Bytes:     res.Bytes,
				Missing:   res.Missing,
				Encrypted: res.Encrypted,
				Warnings:  res.Warnings,
				Connected: false,
				Reason:    err.Error(),
				Hint: fmt.Sprintf(
					"事件库目前只支持 KiriKiri。解出的最小集合已保存在 %s；"+
						"可以打开目录检查，或等引擎泛化后重新导入。", dest),
			}, nil
		}
		return nil, fmt.Errorf("解包成功但登记失败：%w", err)
	}
	return &UnpackImportDTO{
		Dest:      res.Dest,
		EngineID:  res.EngineID,
		Events:    res.Events,
		Files:     res.Files,
		Bytes:     res.Bytes,
		Missing:   res.Missing,
		Encrypted: res.Encrypted,
		Warnings:  res.Warnings,
		GameID:    id,
		Connected: true,
		Library:   lib,
	}, nil
}
