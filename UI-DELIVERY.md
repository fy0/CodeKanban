# CodeKanban UI · Revision 03

## 本轮重点

Studio · 精工保留中性实色与蓝色强调，并恢复原导航、首页布局、工作区标签、无外框输入区及项目直线选中标记。墨稿已移除，由 **Tide · 潮汐** 的雾青海蓝、轻渐变、无衬线字体和圆润内容卡片替换，支持亮暗两套配色及旧设置迁移。Forge 保留工业风格；Classic 保留原配色机制。

会话列表的紧凑是业务要求，不是需要被主题“修正”的缺陷。真实列表仍采用原 **34px 行高 / 38px 虚拟槽位**，标题单行，辅助时间不另占一行。主题的舒适密度、较大字号和圆角不会扩大列表行。完整标题可通过悬停和可访问名称读取；选中状态有 aria-current；千位会话计数不再挤在固定宽度徽标里。原型改为 32 条单行标题，方便比较大量会话时的阅读节奏。

工作区保持左项目、中主工作区、右会话。没有为了主题重排业务分区，也没有修改原有侧栏宽度、折叠状态和会话生命周期。

## 直接查看

远端独立原型：`ui-prototypes/index.html`。真实应用：设置页顶部 **Theme Studio** 或原主题菜单，选择 **Studio · 精工**。已有个人覆盖项仍然保留；需要查看新的默认外观时，使用“重置当前自定义项”，不要清空全部设置。

本轮源码、原型、检查脚本和报告均在连接的 Windows 项目中。`ui/dist` 是本轮实际构建产物。没有自动提交、推送 Git 或重启已有业务服务；没有改动真实项目数据库、会话、终端或账号配置。

## 验证入口

现在不再依赖旧 Codex 工具宿主。验证由本地 Node、项目现有依赖、已安装的 Chromium 和临时 Go 后端执行，不安装新包，不需要远程编码代理。

```sh
# 项目根目录：完整类型检查、生产构建、前端全套测试
node tools/validate-appearance.mjs

# 从真实主题重新生成静态入口，然后检查离线页面
node tools/build-ui-prototypes.mjs
node tools/verify-ui-static.mjs

# 构建专用临时测试后端（Windows）；不会启动或替换已有服务
go build -o output/ui-audit-v3/codekanban-check.exe .
node tools/verify-ui-runtime.mjs
```

浏览器检查使用系统已有 Chrome/Edge；可通过 `CHROMIUM_PATH` 指定已有浏览器，不会安装浏览器或连接个人浏览器配置。运行时检查使用独立临时 HOME、SQLite、Git 仓库、端口和浏览器配置，创建 1,000 条空闲会话元数据。不会向模型发送消息。终端检查只在临时仓库执行一条无副作用的 echo 命令；登录检查只给临时后端设置测试密码。每次结束关闭测试进程。

## 结果与边界

`docs/ui/VALIDATION.md` 是本轮验收摘要；`validation-runtime-results.json` 和三个日志记录完整构建／测试退出码；`docs/ui/validation-v3/runtime.json`、`static.json` 是浏览器检查明细，截图在同目录。

真实流式模型执行、外部模型服务故障、第三方提供方审批未消耗账号额度进行测试。不能把空闲会话、终端和主题回归称为这些外部服务的完整验收。完整主题 API、JSON 继承与插件说明仍见 `docs/ui/THEMES.md`。
