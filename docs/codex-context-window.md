# Codex 会话上下文窗口

会话设置和用量面板的窗口上限旁均可选择 `默认`、`512K`、`768K`、`1M`。档位分别对应 `0`、`512000`、`768000`、`1000000` tokens，默认表示不覆盖 Codex 自身配置。

全局“会话默认值与重试策略”中的窗口档位只用于新建 Codex 会话，创建后独立保存。已有会话和恢复的草稿不随全局默认改变；清空上下文及从已有会话派生时保留会话设置。

运行中修改只影响下次运行，不中断当前任务。用量面板仍依据运行时实际报告的窗口计算；设置菜单区分当前设置、实际窗口（或上次运行窗口）和待生效状态。实际窗口可能因 Codex 预留空间而小于设置值，二者不相等不代表设置失败。

本功能仅覆盖 `model_context_window`，不写入 `model_auto_compact_token_limit`，不修改用户的 `.codex/config.toml`，也不能突破服务端模型上限。

## CLI 模型元数据限制

运行请求成功并不代表请求窗口已完整生效。面板分别展示保存值、运行请求值和 CLI 报告窗口，不能把后者解释为服务端模型的最大能力。

本机 Codex 0.147.0 的离线复现显示：当 `gpt-6-astra` 元数据缺失时，Codex 回退到 `context_window = max_context_window = 272000`，并将更大的 `model_context_window` 截断到该上限；扣除 5% 预留后报告 `258400`。无论在线程配置还是进程启动参数中传入 `768000` 都无法绕过这一限制。需由 Codex 获得正确模型目录，不能靠前端放大数字解决，也不应伪造模型最大能力。

模型元数据回退警告会保存在会话状态和历史中；切换模型或开始下一次运行时重新检测。既有日志未保存此警告的会话，需要下一次运行收到 Codex 警告后才能显示标记。

### 版本差异与离线验证

2026-09-05 核对上游 `rust-v0.153.4`：`codex-rs/models-manager/models.json` 已包含 `gpt-6-astra`，并将它及 GPT-5.6 系列的 `max_context_window` 设为 `872000`；默认窗口仍为 `272000`。`model_info.rs` 仍按 `min(请求窗口, 元数据最大窗口)` 应用覆盖，并非任意大小的硬覆盖。

使用官方 0.153.4 Windows 二进制、独立临时 `CODEX_HOME` 和不可连接的本地 provider 验证，直接读取临时会话日志中的 `task_started.model_context_window`，未调用在线模型：

| 模型 | 请求设置 | CLI 报告窗口 |
| --- | --- | --- |
| gpt-6-astra | 默认 | 258400 |
| gpt-6-astra | 512000 | 486400 |
| gpt-6-astra | 768000 | 729600 |
| gpt-6-astra | 1000000 | 828400 |
| gpt-5.6-sol | 768000 | 729600 |

因此本机 0.147.0 下 GPT-6 的 768K 截断可通过升级到 0.153.4 并重新启动承载会话的 Codex 进程解决；仅更新磁盘上的程序不会改变已运行进程。1M 设置在这些模型上仍被截断到 872000，再扣除 5% 预留。上述验证只确认客户端窗口计算，不证明 provider 的服务端容量或计费规则，也不代表所有模型都采用同一上限。

## 接口

- 全局配置：`webSessionCodexContextWindow`，默认 `0`。
- 创建会话：可选 `contextWindowSetting`，省略取当时的全局默认，显式 `0` 使用 Codex 默认。
- 会话响应：`contextWindowSetting` 为保存值；`appliedContextWindowSetting` 保持兼容命名，表示最近启动线程的请求值（不是完整生效确认），尚未启动时为 `null`。CLI 报告窗口仍由 `contextWindowTokens` 和 `contextWindowSource` 表达；`codexModelMetadataFallback` / WebSocket `cmmf` 表示当前或最近运行收到模型元数据回退警告。
- WebSocket 创建和 `set_cws` 命令使用 `cwset` 字段；会话快照用 `cwset` 和 `acwset` 传送设置值与运行值，保留原有 `cws` 表示窗口来源。
- 非 Codex 会话不可设置非默认窗口，非法档位被拒绝。新增数据库字段通过已有自动迁移流程创建，已有记录保持默认。
