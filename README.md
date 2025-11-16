# 代码看板 Code kanban

AI时代的辅助编程工具，帮助你提速10倍。

## 核心功能

- **多项目多终端管理**：轻松在 3-4 个代码仓库、二十几个终端之间切换，每个终端运行不同的 AI 编程任务
- **Git Worktree 管理**：轻量级分支管理，同时让 AI 开发多个功能，不满意随时回滚
- **任务看板系统**：可视化管理开发任务，支持任务状态跟踪和分支关联
- **Web 终端集成**：使用VSC同款技术栈的 Web 终端，支持标签管理、拖动排序、折叠展开等 (快捷键`)
- **笔记功能**：支持多标签笔记，自动保存，标签可重命名和排序 (快捷键1)
- **编辑器集成**：快捷打开 VSCode、Cursor、Zed 等编辑器
- **使用你喜欢的工具**: Claude Code, Codex, Gemini, Qwen Code, Droid, ... 啥都行


## 开发指南

### 环境要求
- **Node.js**: v20.19.0+ 或 v22.12.0+
- **Go**: 1.24.6+
- **包管理器**: pnpm（推荐）

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


### 未来可能的一些功能
- 移动端支持
- 代码清理: 如前端的src/api，应当全走自动生成
- 完成提醒功能，例如AI干完之后播放个声音，告诉你已经弄好了。
- 空闲终端列表 / 待交互终端列表。
