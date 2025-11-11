# Huma 接口命名约定

本文汇总了 `api/` 目录下现有 Huma 接口的 `Tags` 与 `OperationID` 命名方式，便于后续新增接口时保持一致风格。

## Tags 命名规律

- 每个接口仅配置一个 Tag，统一写成 `kebab-case 英文前缀 + 中文描述` 的形式，例如 `attachments-附件`、`approval-flow-node-审批流程节点`。
- 英文前缀通常复用路由分组路径的层级，`/approval/flow-node` → `approval-flow-node`，`/admin/app/project-template` → `admin-app-project-template`。
- 中文部分用一句话概括模块职能，强调接口归属（如“管理端权限”“审计项目”），便于文档检索。
- 同一分组的所有接口复用相同 Tag，保持前缀一致，有助于前端或文档侧按模块聚合。

## OperationID 命名规律

- 默认使用全小写 `kebab-case`，词元用 `-` 连接，少数历史接口（例如 `GetScopeTypeOptions`）为保兼容暂未调整，新接口不再新增此类写法。
- 前缀部分与 Tag 的英文前缀保持一致，体现接口所在模块，如 `attachments-*`、`approval-flow-instance-*`、`platform-user-*`。
- 结尾动词或短语描述具体动作：`create`/`update`/`delete`/`get`/`list`/`page`/`download` 等；场景差异通过附加短语表达（`-by-id`、`-by-project`、`-status-transition`、`-started-by-me`）。
- 统计、计数类接口统一用 `-count`、`-statistics`；多态或状态流转相关接口使用 `-status-*`、`-toggle-*` 等明确含义的后缀。
- 面向平台/管理端等角色的接口在前缀直接加角色限定（如 `admin-`、`platform-`），避免歧义。

## 约定与建议

- 新增接口时先确认分组 Tag，若已有同类模块直接沿用；若需新建模块，按“路径层级 → 英文前缀”规则命名，并补充清晰的中文释义。
- OperationID 应保持与路由命名、服务方法语义一致，方便前端、测试脚手架以及文档映射。
- 历史 CamelCase OperationID 如需调整，请提前同步调用方并安排联动修改，避免破坏 SDK 或前端依赖。
