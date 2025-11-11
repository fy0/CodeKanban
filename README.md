# AICode kanban

此项目的主要功能是管理多个项目的大量终端，用于AI时代编程时，让你比较容易的处理复数的 claude code / codex / gemini code 等等

另提供简易的 git worktree 管理功能，worktree是一种轻量级分支，这样你就可以同时让ai写两个功能，如果某一个不满意回滚就可以了。

目前无用的页面:
- 任务管理功能: 初始的设想是一任务对应一shell，实际发现并无必要。可能考虑删掉这玩意。
- 分支管理页面: 没用上
- 记事本: 本身想着随手存一些prompt，但是好像也没啥用

未来可能的一些功能:
- 完成提醒功能
- 统一的设置中心
- 使用编辑器打开目录功能

总之目前基本上满足日常需求，我自己会就这样用一段时间，过段时间不满意了统一修改。

除了这段话之外，此项目的AI含量极高。


## 能力概览
- **配置中心**：`utils/app_config.go` 读取 / 写入 `config.yaml`，并提供日志级别、OpenAPI、Huma 文档路径等开关。
- **日志系统**：`utils/logger.go` 使用 zap，支持控制台与文件双输出。
- **数据层**：
  - `model/table` 存放 GORM 表声明，可按需扩展具体业务模型。
  - `model` 目录下封装了 GORM 初始化、迁移、关闭等生命周期方法。
  - `model/sqlc_gen` 保留了 schema 生成脚本，若后续需要接入 sqlc，可按需启用。
- **API 框架**：`api/api.go` 预置 Fiber + Huma 集成，自动挂载 OpenAPI JSON 与自定义 docsPath。
- **分支管理**：`model/branch.go` 提供分支增删查、合并与 Worktree 联动，输出缓存、结构化日志与 Prometheus 指标，前端 `BranchManagement.vue` 提供可视化操作。
- **工具集**：保留 ID 生成、SQL schema 生成等常用组件。

## 使用步骤
1. 初始化依赖：`go mod tidy`
2. 根据环境修改 `config.yaml`（数据库 DSN、日志输出等）。
3. 启动服务：`go run .`
   - 如需强制迁移，可追加 `-m` 或 `--migrate`。
4. 若未来需要生成 sqlc 代码，可在补齐 SQL 文件后执行 `go generate -run="sqlc"`（可选）。

## 目录说明
- `api/`：HTTP & Huma 相关实现。
- `model/`：数据层（GORM 表、初始化、可选的 sqlc schema 工具）。
- `utils/`：配置、日志、ID、sqlc 生成工具等通用模块。
- `static/`、`docs/`、`data/`：静态资源、自定义文档、运行期数据占位。

## Windows 服务脚本
- 安装：`go run . -i`
- 卸载：`go run . --uninstall`

复制项目后，可按需替换示例模型或移除不需要的模块，使其贴合自身业务。

## 分支管理

### 后端能力

- `GET /api/v1/projects/{projectId}/branches`：返回本地 / 远程分支列表，自动标记本地 Worktree 绑定。
- `POST /api/v1/projects/{projectId}/branches`：校验分支命名并创建分支，可选同步创建 Worktree。
- `DELETE /api/v1/projects/{projectId}/branches/{branchName}`：支持 `force` 查询参数，删除前会阻止默认分支和当前检出的分支。
- `POST /api/v1/worktrees/{id}/merge`：在指定 Worktree 内执行 merge/rebase/squash，返回冲突文件清单；落盘 Prometheus 指标与结构化日志。

### 前端入口

- 路由 `/#/project/:id/branches` 提供分支管理页面，可从 Worktree 侧栏的「分支」按钮进入。
- 页面特性：
  - 顶部提供快捷键提示（Ctrl+N 创建、Ctrl+R 刷新、Ctrl+F 聚焦搜索）。
  - 左右分栏展示本地 / 远程分支，列表支持超 200 条的虚拟滚动。
  - 本地分支可直接创建 / 打开 Worktree 或发起删除；远程分支可一键创建本地分支。
  - 底部合并区支持选择 Worktree + 源分支 + 策略并展示冲突文件，合并结束自动刷新 Worktree 状态。
- 所有 API 调用遵循 `ui/docs/data-fetching-best-practices.md` 规范，通过 `useReq` + `Apis.branch.*` 实现。
