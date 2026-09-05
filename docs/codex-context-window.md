# Codex 会话上下文窗口

会话设置和用量面板的窗口上限旁均可选择 `默认`、`512K`、`768K`、`1M`。档位分别对应 `0`、`512000`、`768000`、`1000000` tokens，默认表示不覆盖 Codex 自身配置。

全局“会话默认值与重试策略”中的窗口档位只用于新建 Codex 会话，创建后独立保存。已有会话和恢复的草稿不随全局默认改变；清空上下文及从已有会话派生时保留会话设置。

运行中修改只影响下次运行，不中断当前任务。用量面板仍依据运行时实际报告的窗口计算；设置菜单区分当前设置、实际窗口（或上次运行窗口）和待生效状态。实际窗口可能因 Codex 预留空间而小于设置值，二者不相等不代表设置失败。

本功能仅覆盖 `model_context_window`，不写入 `model_auto_compact_token_limit`，不修改用户的 `.codex/config.toml`，也不能突破服务端模型上限。

## 接口

- 全局配置：`webSessionCodexContextWindow`，默认 `0`。
- 创建会话：可选 `contextWindowSetting`，省略取当时的全局默认，显式 `0` 使用 Codex 默认。
- 会话响应：`contextWindowSetting` 为保存值；`appliedContextWindowSetting` 为最近启动线程采用的值，尚未启动时为 `null`。实际窗口仍由 `contextWindowTokens` 和 `contextWindowSource` 表达。
- WebSocket 创建和 `set_cws` 命令使用 `cwset` 字段；会话快照用 `cwset` 和 `acwset` 传送设置值与运行值，保留原有 `cws` 表示窗口来源。
- 非 Codex 会话不可设置非默认窗口，非法档位被拒绝。新增数据库字段通过已有自动迁移流程创建，已有记录保持默认。
