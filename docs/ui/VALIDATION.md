# 验证记录 · Revision 03

日期：2026-10-04。执行环境：连接项目的 Windows、Node v24.4.0、已有 Vue/Vite/Vitest 依赖、系统 Chromium。本轮已经实际运行完整前端检查和真实后端浏览器回归；上一轮工具宿主阻塞不再是当前状态。

## 实际结果

| 检查 | 结果 | 证据 |
| --- | --- | --- |
| 完整 vue-tsc 类型检查 | 通过，exit 0 | `validation-runtime-results.json`、`validation-type-check.log` |
| Vite 生产构建 | 通过，exit 0 | `validation-build-only.log`；产物为 `ui/dist` |
| Vitest 全套 | 136 个测试文件、1,080 项全部通过 | `validation-test.log` |
| 主题与会话组件定向 ESLint | 通过，exit 0 | 对 `src/themes`、`components/appearance`、appearance store/runtime、会话行与新增测试运行，零 warning |
| 真实生产 UI + 临时 Go 后端 | 26 组全部通过，0 未捕获页面异常 | `validation-v3/runtime.json` |
| 独立静态页面 | 240 次渲染通过 | `validation-v3/static.json` |
| 静态弹窗／状态 | 112 次通过 | 同上：28 种 × 四套 |
| 静态分区／紧凑行检查 | 64 次通过 | 同上，是 240 次渲染中的子集，不额外计为端到端用例 |
| 修改区空白检查 | `git diff --check` 通过 | 运行时、原型、工具和文档相关路径 |

首次完整复核发现原有外观测试中四处仍写死上一版本的颜色／圆角，导致断言失败。已改为使用当前主题定义验证继承、重置和兼容行为，没有删除这些测试。另新增五项会话行测试，覆盖完整标题、原 38px 虚拟槽、原生按钮选择、菜单独立性及选中语义。

## 真实浏览器回归的范围

使用本次 Vite 生产产物和实际 Go 服务，不使用静态原型代替业务页面。测试创建独立临时 HOME、配置、SQLite、Git 仓库、端口和浏览器配置；在真实数据库中创建 1,000 条空闲会话元数据。默认不是连接用户已有服务，也不继承用户的模型凭据。

26 组检查包括：

- 全量会话加载；四套主题 × 明暗模式的左右分区、侧栏宽度、DOM 身份和草稿连续性；大字号／舒适密度下仍保持 34px 行高、38px 虚拟槽及单行标题；千位计数完整显示。
- 滚动到 1,000 条会话的底部且 DOM 挂载行数保持有界；输入标题查询并用真实 Enter 按键选中目标会话。
- Theme Studio 的真实界面、拒绝无效继承、双模式导入预览、明确应用、刷新后保留、自适应系统明暗和真实跨标签页 storage 同步。
- 实际 xterm 和终端会话在四套主题切换后保留；连接状态仍为 ready；通过键盘输入 echo 并从实际 WebSocket 收到经 shell 展开的结果。
- 文件、Git 差异、项目总览、设置、指南、分支页面；390px 移动工作区；启用临时后端的密码保护后，八种主题／明暗登录外观以及真实 Cookie 鉴权与受保护路由跳转。

终端检查仅在临时仓库执行无副作用的 echo。鉴权修改仅限临时后端。测试完成关闭浏览器、终端和测试服务，不改变已有业务服务、真实数据库、密码或模型配置。

## 静态检查的范围

通过真实 `file://` 导航加载本地 HTML/CSS/JS，不注入替代页面。四套 × 十个视图 × 1440/1024/390px × 明暗，共 240 次；检查整页横向溢出及脚本异常。桌面工作区同时检查左项目、中主区域、右会话的边界与单行 34px 原型会话行。另打开全部 112 个弹窗／状态入口并通过 Escape 关闭。

当前截图在 `validation-v3/`。`*-workspace.png` 为真实生产应用与临时后端，`*-prototype.png` 为静态演示。旧 `screenshots/` 和 V2 JSON 是历史资料，不能代替本轮结果。

## 复跑

项目根目录，使用已有依赖：

```sh
node tools/validate-appearance.mjs
node tools/build-ui-prototypes.mjs
node tools/verify-ui-static.mjs
go build -o output/ui-audit-v3/codekanban-check.exe .
node tools/verify-ui-runtime.mjs
```

浏览器驱动是无新增依赖的 Node/CDP 小工具，使用现有 Chrome/Edge 和全新临时用户目录；也支持通过 `CHROMIUM_PATH` 指定已安装浏览器。工具不安装依赖、不发送模型消息。构建报告保存关键源码 SHA-256，浏览器报告逐项记录结果与测量值。

## 未扩大宣称的范围

没有向外部模型服务发送请求，因此不宣称付费模型的真实流式推理、第三方审批及提供方断线恢复已完成端到端验收。上传、所有高级配置和全部键盘／输入法组合也没有穷举。它们的已有前端测试参与全套运行，但不等同于外部服务实测。

Vite 仍有大 chunk 提示；本轮未为消除提示进行大规模拆包。该提示不是构建失败。已有服务没有被自动部署或重启；嵌入旧前端的已发布 Go 二进制需要正常重新打包／部署才能使用新产物。

Git 工作区本来已有其他修改。本轮只操作主题、紧凑会话行、原型、测试和文档相关文件，没有自动提交或推送。完整 workspace review 的快照接口返回 source identity unavailable，已用文件快照、定向 Git diff 和 diff --check 复核；没有为了清理检查结果删除不明来源的未跟踪文件。
