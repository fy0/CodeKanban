# Theme Studio 与开发者扩展 · Revision 04

## 本轮调整

**Tide · 潮汐** 替换墨稿：雾青画布、海蓝强调色、柔和光泽与圆润内容卡片；暗色使用深海蓝。移除墨稿的纸张底纹、衬线标题、双线和纸页叠层。已有 `paper` 选择、个人覆盖、继承关系和旧备份会迁移到 `tide`；个人明确设置的颜色和布局仍保留。

所有主题沿用原有页面导航。Studio · 精工保留原首页布局、工作区标签、无外框消息输入区和左侧项目直线选中标记；潮汐也保留无外框输入区和直线标记。

会话列表是高密度导航，不是内容卡片。四套主题均保留原 **34px 单行 / 38px 虚拟槽位**，标题不增加第二行；可见标题使用清晰的无衬线字体，完整标题通过原生 tooltip 与按钮可访问名称提供。`density` 不改写这个会话列表尺寸契约，左右分栏宽度继续由原工作区拖动状态管理。原型改为 32 条单行示例，真实浏览器使用 1000 条临时会话做回归。

## 用户入口

真实应用的设置页顶部和现有主题切换菜单可打开 Theme Studio。默认 Classic 使用原有偏好，不会在第一次启动时覆盖旧调色板。选择 Studio、Forge 或 Tide 后，外观设置立即作用于真实业务组件。

四套界面共享一套业务实现。选择界面与选择明暗是两个概念：切换界面采用该界面的默认模式；之后可选择亮色、暗色或跟随系统。每套界面的颜色／布局覆盖独立保存。每套新主题的亮色与暗色又分别保存颜色覆盖。

## 可定制项目

| 布局项 | 可选值／范围 | 应用范围 |
| --- | --- | --- |
| `navigation` | `original`, `sidebar`, `rail`, `top` | 为旧导入文件保留，页面统一沿用原有导航 |
| `density` | `comfortable`, `compact` | 普通控件高度和信息密度；会话列表保持原紧凑尺寸 |
| `font` | `system`, `humanist`, `mono` | 固定安全字体栈；不下载字体 |
| `fontSize` | 12–20 | 界面字号，像素 |
| `lineHeight` | 1.3–2 | 阅读行高 |
| `radius` | 0–24 | 圆角，像素 |
| `spacing` | 8–32 | 主要布局间距，像素 |
| `sidebarWidth` | 180–300 | 为旧导入文件保留；工作区宽度由原有拖动状态管理 |
| `contentWidth` | 640–1600 | 项目／设置／指南及会话阅读最大宽度 |
| `panelStyle` | `flat`, `outlined`, `elevated` | 面板表现 |
| `material` | `solid`, `glass`, `paper` | 面板基础材质 |
| `texture` | `none`, `grid`, `ruled` | 工作区画布底纹 |
| `headingFont` | `sans`, `serif`, `mono` | 标题字体栈，不下载字体 |
| `borderWidth` | 1–2 | 可定制面板描边，像素 |
| `motion` | `full`, `reduced` | 动画；操作系统减少动态效果设置始终优先 |

颜色分为三组，共 19 个 token：

```text
canvas surface raised sunken text muted subtle border accent
success warning error info plan working addition deletion
terminalBg terminalFg
```

每个颜色只接受完整六位十六进制 `#RRGGBB`。`text` 和 `muted` 相对 `surface` 的对比度会在编辑面板提示；这不是整页无障碍认证。各个业务状态还有文字和图标，不只依靠颜色。

Classic 的颜色仍由原“主题设置”中的预设／调色控件管理；新工作室可修改其布局。Classic 的主题导出采用规范化基础定义，不冒充导出旧预设的全部历史派生色。要完整保留旧调色板，应使用原有完整设置备份。

## 固定的工作区结构

项目工作区没有主题专属外层侧栏或顶栏。`AppearanceShell` 始终保留同一个 slot，不给 RouterView 增加主题相关 key。原左侧项目面板、中间 WorkspaceTabView、Web 会话面板内右侧会话列表的位置不变；原工作树面板及各业务面板的可见性逻辑不变。

原有分栏宽度和折叠状态由原业务状态维护，主题不重置。静态稿的终端／文件／变更页也展示同样的右侧会话列；真实应用这些非 Web 会话面板仍使用各自原有的侧栏逻辑，并未重写业务布局。

三套品牌样式定义在 `appearance-designs.css`，由生成脚本复制到静态目录。材质和标题参数是新增可选字段，缺失时回退 `solid / none / sans / 1`，不提升 Schema 版本。品牌专属装饰（例如纸页双线、工业信号条）属于设计命名空间，不会被某个基础 token 完全抹除。

## JSON 主题

可直接导入 `examples/themes/moss-studio.json`，最小示例：

```json
{
  "schemaVersion": 1,
  "id": "my-studio",
  "name": "我的工作台",
  "nameEn": "My Studio",
  "extends": "studio",
  "defaultMode": "system",
  "colors": {
    "light": { "accent": "#28684B" },
    "dark": { "accent": "#A3D8B8" }
  },
  "layout": {
    "radius": 10,
    "density": "compact",
    "navigation": "sidebar"
  }
}
```

合并顺序：根主题 → 子主题定义 → 用户逐主题覆盖。颜色按明暗分开合并；布局不随明暗变化。循环、缺失父主题、超过 12 层的继承链和缺失最终颜色会被拒绝。没有 `extends` 的独立主题必须提供完整的两套颜色。

`id`、`extends` 和 `design` 是 2–48 位、以小写字母开头的安全名称，后续允许数字和短横线。`nameEn` 可省略，回退到 `name`。`description` / `descriptionEn` 可选。未知字段会被拒绝。

`design` 是 CSS 命名空间，不是脚本路径。内置命名空间是 `classic`、`studio`、`forge`、`tide`；旧文件中的 `design: paper` 和 `extends: paper` 自动升级为 `tide`。可信插件可使用自己的名称并随构建提供对应 CSS。未提供额外 CSS 的命名空间仍使用通用布局与 token，不会尝试远程加载资源。继承已有视觉时可省略 `design`。

`ui/src/themes/theme.schema.json` 可作为编辑器 JSON Schema 关联。为避免在主题中隐式引入远程资源，不在导入文件内添加 `$schema` 字段；在编辑器配置中关联本地 schema。运行时校验器还会验证完整继承图和最终颜色，因此不能仅凭 Schema 校验决定导入。

## 导入、备份和安全边界

主题文件最多 64 KiB，完整外观备份最多 2 MiB，最多保存 24 个用户主题。导入先校验，展示亮／暗色预览，然后明确确认应用。已有同名用户主题需要勾选替换；内置和编译期插件的 ID 不能被用户文件覆盖。

导出主题会展开继承与当前覆盖，形成无需父主题文件的可移植定义。导出完整外观则保存选择、模式、用户主题和全部覆盖。删除主题前检查依赖，存在子主题时拒绝删除；重置只清除当前主题的覆盖。

用户 JSON 不支持 CSS 文本、JavaScript、字体 URL、资源 URL、组件代码或远程请求。所有颜色和数值在应用前校验。项目名、主题名等使用 Vue 文本渲染，不以 HTML 执行。

偏好保存在 `codekanban.appearance.v1`。旧 `general_settings` 不被迁移或删除。跨标签页使用 storage 事件同步，收到无效数据时保留当前状态；浏览器存储失败时仍应用内存状态并显示警告。关闭或刷新页面后未持久化的状态可能丢失。

一般设置备份也包含外观。解析备份时在任何服务端导入之前校验主题；取消选择客户端设置时，同时排除其外观数据。旧备份没有外观字段时，保留当前外观。

外观偏好属于当前浏览器，不自动跨设备同步。显式的终端配色继续独立于 UI 主题；只有“跟随界面主题”的终端使用新的终端 token。生成的 ANSI 前景和光标相对终端背景做独立对比度修正；界面语义色不被改变。用户指定的终端正文／背景仍按其选择保留，因而不宣称任意自定义终端一定无障碍。

## 可信编译期插件

公共入口：`@/themes`。

```ts
import { defineComponent, h } from 'vue';
import { registerAppearancePlugin } from '@/themes';

const TeamBadge = defineComponent({
  setup: () => () => h('span', { class: 'team-badge' }, 'Engineering'),
});

registerAppearancePlugin({
  id: 'team-appearance',
  themes: [{
    schemaVersion: 1,
    id: 'team-studio',
    name: '团队工作台',
    nameEn: 'Team Studio',
    extends: 'studio',
    layout: { density: 'compact' },
  }],
  contributions: [{
    id: 'team-badge',
    slot: 'shell.toolbar',
    order: 20,
    component: TeamBadge,
  }],
});
```

在应用挂载、任何 appearance store 被创建之前注册，例如把注册代码放进 `ui/src/appearance-plugins.ts`，在 `main.ts` 顶部导入。初次创建 store 后注册表封闭，避免运行中的主题继承关系被不受控地替换。

可用插槽是 `shell.brand`、`shell.toolbar`、`shell.footer`、`settings.footer`。贡献按照 `order` 和 `id` 稳定排序。品牌插槽有默认内容；组件渲染错误被局部边界隔离，不把整个路由树卸载。主题和贡献先全部校验，通过后才原子注册。

编译期插件是受信任应用代码，拥有与其他源码相同的能力，不是沙箱。安装前必须审查。用户导入的 JSON 不能注册 UI 贡献，不能运行本例。这里借鉴了插件化的分层思想，但不兼容 DeepSeek Harness、OpenCode 或其他产品的插件二进制／配置协议。

完整插件和独立 CSS 命名空间示例在 `examples/appearance-plugin/`，默认不安装。

## 实现分层

`core.ts` 是不依赖 Vue/DOM 的校验、继承和颜色运算；`registry.ts` 管理内置和可信插件；`stores/appearance.ts` 负责偏好、导入与跨窗口同步；`adapter.ts` 映射原有 ThemeSettings 和终端配色；`useAppearanceRuntime.ts` 负责根节点数据属性与布局变量；`AppearanceShell.vue`、`ThemeStudio.vue` 负责导航与编辑。

工作区不渲染外层导航时，`shell.*` 插槽也不会出现；`settings.footer` 仍可通过 Theme Studio 使用。

`App.vue` 仍负责生成原来的语义 CSS 变量和 Naive UI overrides。新主题不是四套路由，也不依赖覆盖全部业务组件的全局 `!important`。只有减少动态效果使用必要的强制规则。

需要增加新的语义 token 时，同时更新 `COLOR_KEYS`、完整内置调色板、adapter、Schema 和测试。只增加品牌方案时，使用现有 token、主题继承和 CSS 命名空间即可。
