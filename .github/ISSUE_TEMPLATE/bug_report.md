---
name: Bug report
about: 报告可复现的缺陷或回归
title: "[Bug] "
labels: bug
assignees: ""
---

## 问题描述

<!-- 一句话说明出了什么问题。 -->

## 复现步骤

1.
2.
3.

## 期望结果

## 实际结果

## 环境

- 版本 / commit：
- 操作系统：
- Go / Node / npm 版本：
- 是否使用 `wails build` 产物：
- 游戏 / 样本名称（如可提供）：

## 日志与截图

<!-- 粘贴完整错误、退出码、相关日志或截图；请勿只写「打不开」。 -->

## 验证命令

```powershell
go build ./...
go vet ./...
go test ./...

cd desktop
go build ./...
go vet ./...
go test ./...

cd frontend
npm ci
npm run build
```

## 附加信息

<!-- 是否为回归、最近一次正常版本、可能相关的文件或 commit。 -->
