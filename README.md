# Go Template

该模板集成了 GORM 数据层、zap 日志、koanf 配置以及 Fiber + Huma 的 API 框架，可直接拷贝用于新项目。

## 能力概览
- **配置中心**：`utils/app_config.go` 读取 / 写入 `config.yaml`，并提供日志级别、OpenAPI、Huma 文档路径等开关。
- **日志系统**：`utils/logger.go` 使用 zap，支持控制台与文件双输出。
- **数据层**：
  - `model/table` 存放 GORM 表声明（默认只有 `ExampleUserTable` 示范）。
  - `model` 目录下封装了 GORM 初始化、迁移、关闭等生命周期方法。
  - `model/sqlc_gen` 保留了 schema 生成脚本，若后续需要接入 sqlc，可按需启用。
- **API 框架**：`api/api.go` 预置 Fiber + Huma 集成，自动挂载 OpenAPI JSON 与自定义 docsPath。
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
