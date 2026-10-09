# 编译期扩展示例

复制本目录到 `ui/src/appearance-plugins/forest-team/`，在 `ui/src/main.ts` 的 import 区加入：

```ts
import './appearance-plugins/forest-team';
```

必须在创建 appearance store 之前注册。然后重新构建前端。Theme Studio 中会出现 Forest Team Studio，桌面外层工具栏会出现 Engineering 标记。它与内置主题使用相同的校验器、持久化和切换机制。

此例不是动态安装器。它是受信任的源代码扩展，默认没有启用，也不接受 JSON 文件注册脚本。自有 CSS 使用 `data-interface='forest-team'` 命名空间。
