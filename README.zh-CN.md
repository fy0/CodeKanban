<div align="center">

# 代码看板 Code kanban

[中文](README.zh-CN.md) | [English](README.md)

只需一个页面，管理你所有的终端和和AI编程工具(claude code/codex)。

[核心特性](#核心特性) • [快速体验](#快速体验) • [开发指南](#开发指南) • [关于](#关于)

![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go)
![Vue](https://img.shields.io/badge/Vue-3.5-4FC08D?logo=vue.js)
![TypeScript](https://img.shields.io/badge/TypeScript-5.8-3178C6?logo=typescript)
![License](https://img.shields.io/badge/license-Apache--2.0-green)

</div>

## 快速体验

立即试用：

```bash
npx codekanban
# 或
npm install -g codekanban@latest
```

## 核心特性

- **🚀 开箱即用**  
  单文件部署，本地数据库，双击即可运行。

- **📊 多项目多终端管理**  
  一个页面统一管理所有项目和终端（快捷键 `` ` ``）。

- **🔍 AI 编程工具状态检测**  
  自动识别 Claude Code / Codex 等工具的运行状态（空闲/工作中/需审批），任务完成时自动提醒。

- **📜 历史记录**  
  记录你与 AI 的对话，一键回溯修改历程。

- **🌿 Worktree 管理**  
  采用混合 Git 引擎：go-git 在进程内快速读取状态、Diff 和增删行，提交及仓库写操作则
  优先使用系统 Git，以保留 hook、签名、filter 和用户配置。可在设置中分别选择读取与写入引擎。

- **📝 笔记功能**  
  支持多标签页笔记，随手记录灵感与问题（快捷键 `1`）。

## 开发指南

### 环境要求
- **Node.js**: v20.19.0+ 或 v22.12.0+
- **Go**: 1.25+
- **包管理器**: pnpm 9.15.9

### 安装依赖

**前端依赖**：
```bash
cd ui
pnpm install
```

**后端依赖**：
```bash
go mod tidy
```

### 开发运行

**前端开发服务器**：
```bash
cd ui
pnpm dev
```
访问地址：`http://localhost:5173`

**后端开发服务器**：
```bash
go run . # 注意，初次运行后会生成config.yaml，端口3007，由于跟正式版本冲突，无法同时运行，建议改为3005。以下当作已经修改
```
- 服务端口：`http://localhost:3005`
- OpenAPI 文档：`http://localhost:3005/docs`
- 健康检查：`http://localhost:3005/api/v1/health`

**可选参数**：
- `-m` 或 `--migrate`：强制执行数据库迁移
- `-i` 或 `--install`：安装为系统服务
- `--uninstall`：卸载系统服务

### 生产构建

**完整构建**（推荐）：
```bash
python build.py
```
此脚本会自动完成以下步骤：
1. 构建前端（`pnpm build`）
2. 将前端产物复制到 `static/` 目录
3. 构建 Go 可执行文件（带优化）

**手动构建**：
```bash
# 构建前端
cd ui && pnpm build

# 构建后端
go build -ldflags="-s -w" -trimpath -o CodeKanban
```

**构建产物**：
- 前端：`ui/dist/` → `static/` (移动到此目录后，构建后端会自动存入可执行文件，实现单文件启动)
- 后端：`CodeKanban.exe`（Windows）或 `CodeKanban`（Linux/macOS）

### 访问应用

**开发环境**：
- 前端开发服务器：`http://localhost:5173`
- 后端 API：`http://localhost:3005`

**生产环境**：
运行构建后的可执行文件，访问 `http://localhost:3007`

## 关于

我们处在一个日新月异也异常撕裂的时代，我们的作品也是如此。

这个工具切实的提升了我的效率，但也许效率的提升也会减少工作需求，而AI的发展会消灭这个行业。

不管怎么说，希望大家用的开心。

如果有帮到你，可以点点star或者给我一点赞助。


### 未来可能的一些功能
- 移动端支持
- 代码清理: 如前端的src/api，应当全走自动生成
- 完成提醒功能，例如AI干完之后播放个声音，告诉你已经弄好了。
- 空闲终端列表 / 待交互终端列表。


## 可安装的 Codex 技能包

本仓库提供一个围绕统一公共 CLI `codekanban-cli` 构建的 Codex 技能包。需要在其他机器上使用 CodeKanban 技能工作流时，无需复制整个仓库即可安装。

JavaScript 包结构：

- `packages/node-sdk`：供 JavaScript 集成使用的纯 SDK
- `packages/codekanban-cli`：唯一对外发布的 CLI，也是打包后 Codex 技能的维护位置
- `packages/codekanban-cli/skills/codekanban-cli`：仓库提供的 Codex 技能源码

关键默认值：

- 默认服务地址：`http://127.0.0.1:3007`
- 首次配置认证：`printf '%s' '<PASSWORD>' | codekanban-cli auth save-token --password-stdin`
- 指定其他服务地址：`codekanban-cli --base-url http://192.168.1.50:3007 session list --path /repo`
- 认证文件位置：Windows 为 `%APPDATA%\codekanban-cli\session.json`；macOS/Linux 为 `$XDG_CONFIG_HOME/codekanban-cli/session.json` 或 `~/.config/codekanban-cli/session.json`

离线安装包会安装一个 CLI 包并复制一个 Codex 技能。安装 CLI 和技能后，请重启 Codex，使新技能被正确发现。
