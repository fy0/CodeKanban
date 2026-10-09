# 离线 UI 方案 · Revision 03

Studio 当前为中性实色的 **精工 / Precision**。会话示例增加到 32 条，改为单行紧凑列表；不使用第二行元信息或宽大卡片。真实应用保持原 34px 行高 / 38px 虚拟槽，不随主题密度或字号扩大。

从 `index.html` 开始。所有 CSS、JavaScript、图标和演示数据均在本目录，页面不依赖 CDN，也没有后端请求。每种设计有以下入口：

| 文件 | 内容 |
| --- | --- |
| `projects.html` | 项目列表、搜索、状态筛选和项目操作 |
| `workspace.html` | 会话列表、消息、计划、工具调用、输入框与详情 |
| `terminal.html` | 演示终端、输入与输出；不运行真实命令 |
| `files.html` | 文件树、代码浏览与预览 |
| `changes.html` | 变更列表、差异和演示暂存 |
| `branches.html` | 分支、工作树与操作入口 |
| `settings.html` | 外观及其余九个设置分区 |
| `guide.html` | 使用指南与快捷操作 |
| `components.html` | 28 种弹窗、抽屉和状态入口 |
| `login.html` | 登录表单样式；不提交凭据 |

主题目录分别为 `classic/`、`studio/`、`forge/`、`tide/`。`shared/app.js` 是共用的演示组件和交互；`shared/app.css` 是基础组件；`shared/workspace.css` 固定左项目／中工作区／右会话；`shared/designs.css` 与真实应用共享材质和品牌样式。`shared/tokens.js` 从真实应用的 `ui/src/themes/builtin.json` 生成。

在完整源码目录重新生成入口：

```sh
node tools/build-ui-prototypes.mjs
```

演示支持搜索、筛选、表单校验、弹窗开关、键盘 Escape、主题明暗、强调色、圆角、会话输入和终端帮助命令。演示中的保存仅作用于原型的本地状态，不改变实际 CodeKanban 配置。

浏览器禁止 `file://` localStorage 时，原型仍可使用，只是不保存偏好。企业策略完全禁止本地脚本时，需要在允许的本地静态服务器上浏览。原型没有尝试绕过浏览器安全策略。

静态覆盖代表当前全部路由及主要业务表面，不是把所有后端数据和每一项高级配置复制进假服务器。真实应用继续使用原有完整业务组件。


总览页 `index.html` 与 `shared/gallery.js` 独立维护；生成脚本更新 40 个视图、4 个别名、tokens 和共用 designs.css。Studio · 精工、Forge · 铸造、Tide · 潮汐的工作区分区相同，不同的是材质与排版。
