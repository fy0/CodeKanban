# 参考调研与设计取舍 · Revision 03

## 本轮反馈落实

用户明确不喜欢 Studio 的玻璃与紫色光晕，并说明会话紧凑度服务于大量标题的快速扫描。因此 Studio 改为中性实色、细线、连续工作台和克制蓝色强调；所有主题都保留原紧凑行。用户反馈优先于用装饰制造视觉差异。下文保留 Revision 02 的调研来源和历史方案；当前 Studio 以 `builtin.json` 与实际截图为准。

## Revision 02 调研记录

调研日期：2026-10-04。引用产品官方文档／仓库，参考交互组织，不复制品牌资产、截图、字体或代码。

## 用户反馈优先

上一版用导航位置区分风格，静态会话页还把会话列表置于左侧。此次取消这种区分方式：左侧项目、中间主工作区、右侧会话是所有新主题的固定工作习惯。设计差异来自材质、字形、控件、边框、明暗层次和状态表现。

| 方案 | 新的设计语言 | 不变的部分 |
| --- | --- | --- |
| Studio / Prism | 紫灰光晕、玻璃面板、圆润悬浮层、胶囊标签 | 左项目／中工作区／右会话 |
| Forge / Industrial | 石墨、琥珀信号、硬边界、网格与等宽排版 | 同上 |
| Tide / 潮汐 | 雾青与海蓝、轻渐变、无衬线排版、圆润内容卡片与柔和阴影 | 同上 |
| Classic | 保留原业务界面与旧偏好 | 兼容基线 |

## ZCode 参考

官方 ZCode Agent 文档把上下文补充、文件引用、命令、模型／执行模式与分支组织在同一任务工作台。输入区域有 `@` 文件、`#` 会话、`/` 命令，以及技能入口。

本轮在静态输入框中提供可点击的文件／会话／命令示例快捷项；保留模型、权限模式和分支的可见位置；工具调用可展开，变更从同一任务进入。点击快捷项只插入演示引用文本，并不等于实现 ZCode 的上下文解析、Memory、Goal Mode 或其代理后端。真实应用继续使用 CodeKanban 原有上下文与执行功能，本轮没有替换其协议。

来源：
- ZCode Agent：https://zcode.z.ai/cn/docs/agents
- ZCode 文档：https://zcode.z.ai/cn/docs

## 主题与扩展参考

DeepSeek Harness 的插件化组织为“主题数据／布局／UI 贡献”分层提供参考；OpenCode 的 JSON 与明暗配色用于参考可移植主题。Claude Code Desktop 与 Vibe Kanban 用于参考会话、项目和变更的邻近工作流。这是 CodeKanban 自己的接口，不兼容这些产品的主题或插件格式。

来源：
- DeepSeek Harness：https://github.com/deepseek-ai/deepseek-harness
- OpenCode Themes：https://opencode.ai/docs/themes/
- Claude Code Desktop：https://code.claude.com/docs/en/desktop
- Vibe Kanban：https://www.vibekanban.com/docs

## 实现边界

普通 JSON 只有受校验的颜色与布局数据，不执行 CSS、脚本或远程资源。完整 UI 扩展只允许可信编译期插件。

真实界面复用原业务组件。静态稿用于比较视觉和演示交互，并非像素级承诺或完整后端功能仿真。尤其终端、文件与变更的真实业务侧栏保持原逻辑；本轮不为了对齐假数据页面而重排业务组件。
