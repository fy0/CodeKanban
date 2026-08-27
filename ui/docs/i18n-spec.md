# 国际化（i18n）规范

本文档说明项目中国际化功能的架构、使用方法和最佳实践。

## 概述

项目使用 [vue-i18n](https://vue-i18n.intlify.dev/) 作为国际化解决方案，采用 Composition API 模式，支持自动语言检测和本地存储语言偏好。

### 支持的语言

- **zh-CN**（简体中文）- 默认语言
- **en-US**（美式英语）

## 核心架构

### 文件结构

```
ui/src/
├── i18n/
│   ├── index.ts              # i18n 实例配置
│   └── locales/
│       ├── zh-CN.ts          # 简体中文翻译
│       └── en-US.ts          # 英文翻译
└── composables/
    └── useLocale.ts          # 语言切换 composable
```

### i18n 配置 (`ui/src/i18n/index.ts`)

```typescript
import { createI18n } from 'vue-i18n';
import zhCN from './locales/zh-CN';
import enUS from './locales/en-US';

export type MessageSchema = typeof zhCN;

const i18n = createI18n<[MessageSchema], 'zh-CN' | 'en-US'>({
  legacy: false, // 使用 Composition API
  locale: initialLocale, // 初始语言
  fallbackLocale: 'zh-CN', // 回退语言
  messages: {
    'zh-CN': zhCN,
    'en-US': enUS,
  },
});
```

**核心特性：**

1. **自动语言检测**
   - 检测浏览器语言（`navigator.languages`）
   - 优先匹配中文（zh-_）和英文（en-_）
   - 检测到的语言自动保存到 `localStorage`

2. **语言持久化**
   - 语言偏好保存在 `localStorage` 的 `app-locale` 键
   - 页面刷新后保持用户选择的语言

3. **类型安全**
   - 使用 `MessageSchema` 确保翻译键的类型安全
   - 所有语言文件必须与 `zh-CN` 的结构保持一致

### useLocale Composable (`ui/src/composables/useLocale.ts`)

提供统一的语言切换接口：

```typescript
export function useLocale() {
  const { locale, t } = useI18n();

  const setLocale = (newLocale: LocaleType) => {
    locale.value = newLocale;
    localStorage.setItem('app-locale', newLocale);
    dayjs.locale(newLocale === 'zh-CN' ? 'zh-cn' : 'en');
  };

  const toggleLocale = () => {
    const newLocale: LocaleType = locale.value === 'zh-CN' ? 'en-US' : 'zh-CN';
    setLocale(newLocale);
  };

  return { locale, setLocale, toggleLocale, t };
}
```

**功能：**

- `locale`: 当前语言（响应式）
- `setLocale`: 设置语言
- `toggleLocale`: 在中英文之间切换
- `t`: 翻译函数（从 `useI18n` 导出）
- **自动同步 dayjs 语言**

---

## 使用方法

### 1. 在组件中使用翻译

#### 方式 1：使用 `useI18n`（推荐）

```typescript
<script setup lang="ts">
import { useI18n } from 'vue-i18n';

const { t } = useI18n();
</script>

<template>
  <h1>{{ t('project.title') }}</h1>
  <button>{{ t('common.confirm') }}</button>
</template>
```

#### 方式 2：使用 `useLocale`

```typescript
<script setup lang="ts">
import { useLocale } from '@/composables/useLocale';

const { t, locale, setLocale, toggleLocale } = useLocale();
</script>

<template>
  <h1>{{ t('project.title') }}</h1>
  <button @click="toggleLocale">
    {{ locale === 'zh-CN' ? 'Switch to English' : '切换到中文' }}
  </button>
</template>
```

### 2. 带参数的翻译

翻译文本支持插值：

**定义翻译：**

```typescript
// zh-CN.ts
export default {
  branch: {
    totalCount: '共 {count} 个',
    defaultBranchLabel: '{branch} (默认)',
  },
};
```

**使用：**

```typescript
<template>
  <p>{{ t('branch.totalCount', { count: 10 }) }}</p>
  <p>{{ t('branch.defaultBranchLabel', { branch: 'main' }) }}</p>
</template>
```

**渲染结果：**

- `共 10 个`
- `main (默认)`

### 3. 在 JS/TS 代码中使用

```typescript
import { useI18n } from 'vue-i18n';

const { t } = useI18n();

const showMessage = () => {
  window.$message.success(t('message.saveSuccess'));
  window.$message.error(t('message.saveFailed'));
};

const confirmDelete = () => {
  window.$dialog.warning({
    title: t('project.deleteProject'),
    content: t('project.deleteConfirm'),
    positiveText: t('common.confirm'),
    negativeText: t('common.cancel'),
  });
};
```

---

## 翻译文件结构

### 模块化组织

翻译按功能模块组织，便于维护和查找：

```typescript
export default {
  common: {
    // 通用词汇
    confirm: '确认',
    cancel: '取消',
    save: '保存',
    // ...
  },
  nav: {
    // 导航
    settings: '总设置',
    guide: '使用指引',
    // ...
  },
  project: {
    // 项目相关
    title: '项目列表',
    createProject: '创建项目',
    // ...
  },
  branch: {
    // 分支相关
    title: '分支管理',
    createBranch: '创建分支',
    // ...
  },
  task: {
    // 任务相关
    title: '任务',
    addTask: '添加任务',
    status: {
      // 嵌套结构
      todo: '待办',
      inProgress: '进行中',
      // ...
    },
    priority: {
      low: '低',
      medium: '中',
      // ...
    },
  },
  worktree: {
    /* ... */
  },
  terminal: {
    /* ... */
  },
  notepad: {
    /* ... */
  },
  settings: {
    /* ... */
  },
  message: {
    // 提示消息
    saveSuccess: '保存成功',
    saveFailed: '保存失败',
    // ...
  },
  validation: {
    // 验证消息
    projectNameRequired: '请输入项目名称',
    // ...
  },
};
```

### 命名规范

1. **使用驼峰命名法**

   ```typescript
   createProject: '创建项目'; // ✅ 正确
   create_project: '创建项目'; // ❌ 错误
   ```

2. **语义化命名**

   ```typescript
   deleteConfirm: '确定要删除吗？'; // ✅ 清晰
   msg1: '确定要删除吗？'; // ❌ 不清晰
   ```

3. **相关项分组**
   ```typescript
   task: {
     status: {
       todo: '待办',
       inProgress: '进行中',
       done: '已完成',
     },
   }
   ```

---

## 添加新翻译

### 步骤 1：添加到中文翻译文件

在 `ui/src/i18n/locales/zh-CN.ts` 中添加新的翻译键：

```typescript
export default {
  // ... 现有翻译
  myModule: {
    newFeature: '新功能',
    description: '这是一个新功能的描述',
  },
};
```

### 步骤 2：添加到英文翻译文件

在 `ui/src/i18n/locales/en-US.ts` 中添加对应的英文翻译：

```typescript
export default {
  // ... existing translations
  myModule: {
    newFeature: 'New Feature',
    description: 'This is a description of the new feature',
  },
};
```

### 步骤 3：在组件中使用

```typescript
<script setup lang="ts">
import { useI18n } from 'vue-i18n';
const { t } = useI18n();
</script>

<template>
  <h2>{{ t('myModule.newFeature') }}</h2>
  <p>{{ t('myModule.description') }}</p>
</template>
```

### 重要提醒

⚠️ **所有语言文件必须保持相同的键结构**

TypeScript 会基于 `zh-CN` 的结构进行类型检查。如果 `en-US` 缺少某些键，会出现类型错误。

---

## 添加新语言

### 步骤 1：创建语言文件

创建新的语言文件 `ui/src/i18n/locales/ja-JP.ts`（以日语为例）：

```typescript
export default {
  common: {
    confirm: '確認',
    cancel: 'キャンセル',
    save: '保存',
    // ... 翻译所有键
  },
  // ... 复制 zh-CN 的完整结构
};
```

### 步骤 2：更新 i18n 配置

修改 `ui/src/i18n/index.ts`：

```typescript
import jaJP from './locales/ja-JP';

// 更新类型定义
const i18n = createI18n<[MessageSchema], 'zh-CN' | 'en-US' | 'ja-JP'>({
  legacy: false,
  locale: initialLocale,
  fallbackLocale: 'zh-CN',
  messages: {
    'zh-CN': zhCN,
    'en-US': enUS,
    'ja-JP': jaJP, // 添加新语言
  },
});

// 更新语言检测逻辑
function detectBrowserLocale(): 'zh-CN' | 'en-US' | 'ja-JP' {
  const browserLanguages = navigator.languages || [navigator.language];

  for (const lang of browserLanguages) {
    const lowercaseLang = lang.toLowerCase();

    if (lowercaseLang.startsWith('zh')) return 'zh-CN';
    if (lowercaseLang.startsWith('en')) return 'en-US';
    if (lowercaseLang.startsWith('ja')) return 'ja-JP'; // 添加日语检测
  }

  return 'zh-CN';
}
```

### 步骤 3：更新 useLocale

修改 `ui/src/composables/useLocale.ts`：

```typescript
export type LocaleType = 'zh-CN' | 'en-US' | 'ja-JP';

export function useLocale() {
  const { locale, t } = useI18n();

  const setLocale = (newLocale: LocaleType) => {
    locale.value = newLocale;
    localStorage.setItem('app-locale', newLocale);

    // 更新 dayjs 语言映射
    const dayjsLocaleMap = {
      'zh-CN': 'zh-cn',
      'en-US': 'en',
      'ja-JP': 'ja',
    };
    dayjs.locale(dayjsLocaleMap[newLocale]);
  };

  // ... 其他代码
}
```

### 步骤 4：添加语言切换 UI

在设置页面或导航栏中添加语言选择器：

```typescript
<template>
  <n-select
    v-model:value="currentLocale"
    :options="localeOptions"
    @update:value="handleLocaleChange"
  />
</template>

<script setup lang="ts">
import { useLocale, type LocaleType } from '@/composables/useLocale';

const { locale, setLocale } = useLocale();
const currentLocale = computed({
  get: () => locale.value,
  set: (val) => setLocale(val as LocaleType),
});

const localeOptions = [
  { label: '简体中文', value: 'zh-CN' },
  { label: 'English', value: 'en-US' },
  { label: '日本語', value: 'ja-JP' },
];
</script>
```

---

## 最佳实践

### 1. 始终使用翻译键，避免硬编码

```typescript
// ✅ 正确
<button>{{ t('common.save') }}</button>

// ❌ 错误
<button>保存</button>
<button>Save</button>
```

### 2. 保持翻译简洁

```typescript
// ✅ 正确
deleteConfirm: '确定要删除吗？';

// ❌ 过于冗长
deleteConfirm: '您确定要删除这个项目吗？删除后将无法恢复，请谨慎操作。';
```

### 3. 避免在翻译中包含 HTML

```typescript
// ❌ 错误
description: '点击<strong>这里</strong>查看详情'

// ✅ 正确 - 使用组件
<template>
  <p>
    {{ t('common.clickHere') }}
    <strong>{{ t('common.here') }}</strong>
    {{ t('common.viewDetails') }}
  </p>
</template>
```

### 4. 复数和数量处理

对于需要根据数量变化的文本，使用插值：

```typescript
// zh-CN.ts
branch: {
  totalCount: '共 {count} 个',
}

// 使用
{{ t('branch.totalCount', { count: branches.length }) }}
```

### 5. 日期和时间

使用 dayjs 处理日期时间，确保 locale 同步：

```typescript
import dayjs from 'dayjs';
import { useLocale } from '@/composables/useLocale';

const { locale } = useLocale();

// dayjs 会自动使用正确的语言
const formattedDate = dayjs(date).format('YYYY-MM-DD HH:mm:ss');
```

### 6. 翻译文件保持同步

每次添加新翻译时，确保所有语言文件都添加了对应的键：

```typescript
// ✅ 正确 - 所有语言都有
// zh-CN.ts
myModule: {
  newKey: '新功能';
}

// en-US.ts
myModule: {
  newKey: 'New Feature';
}

// ❌ 错误 - en-US 缺少翻译
// zh-CN.ts
myModule: {
  newKey: '新功能';
}

// en-US.ts
myModule: {
  /* newKey 缺失 */
}
```

### 7. 使用 TypeScript 类型检查

利用 `MessageSchema` 确保类型安全：

```typescript
// 这会产生类型错误，因为键不存在
{
  {
    t('nonexistent.key');
  }
} // ❌ TypeScript 错误

// 正确的键会有自动补全
{
  {
    t('common.confirm');
  }
} // ✅ 有自动补全提示
```

---

## 常见问题

### Q1: 翻译未生效？

**检查：**

1. 翻译键是否正确
2. 是否在所有语言文件中都添加了该键
3. 组件是否正确使用 `useI18n` 或 `useLocale`

### Q2: 如何在非组件代码中使用翻译？

```typescript
import i18n from '@/i18n';

const t = i18n.global.t;
const message = t('message.saveSuccess');
```

### Q3: dayjs 语言未同步？

确保在 `main.ts` 中导入了 dayjs 的语言包：

```typescript
import 'dayjs/locale/zh-cn';
import 'dayjs/locale/en';
```

并在 `useLocale` 的 `setLocale` 方法中正确设置：

```typescript
dayjs.locale(newLocale === 'zh-CN' ? 'zh-cn' : 'en');
```

### Q4: 如何处理长文本？

对于较长的文本，考虑：

1. **拆分为多个键**

   ```typescript
   guide: {
     step1: '第一步：创建项目',
     step2: '第二步：配置分支',
     step3: '第三步：开始开发',
   }
   ```

2. **使用 Markdown 组件**（如果项目有 Markdown 渲染器）

   ```typescript
   guideContent: `
     ## 快速开始
     1. 创建项目
     2. 配置分支
     3. 开始开发
   `;
   ```

### Q5: 如何测试不同语言？

在浏览器控制台中临时切换语言：

```javascript
localStorage.setItem('app-locale', 'en-US');
location.reload();
```

或者在设置页面添加语言切换器。

---

## 扩展阅读

- [vue-i18n 官方文档](https://vue-i18n.intlify.dev/)
- [Composition API 模式](https://vue-i18n.intlify.dev/guide/advanced/composition.html)
- [dayjs 国际化](https://day.js.org/docs/en/i18n/i18n)

---

## 参考文件

- `ui/src/i18n/index.ts` - i18n 配置
- `ui/src/i18n/locales/zh-CN.ts` - 中文翻译
- `ui/src/i18n/locales/en-US.ts` - 英文翻译
- `ui/src/composables/useLocale.ts` - 语言切换 composable
