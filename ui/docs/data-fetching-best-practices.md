# 数据拉取最佳实践

本文档总结了项目中使用 `useReq`、`useInit`、`useReqComputed` 进行数据拉取的最佳实践和常见模式。

## 🚨 核心原则（必读）

### 原则 1：必须使用 Apis 对象，禁止使用 Store

所有 API 请求必须直接使用 `Apis` 对象，**禁止**通过 Store 包装调用。

```typescript
import Apis from '@/api';

// ✅ 正确：直接使用 Apis
const { send } = useReq((id: string) => Apis.adminProjectPhaseNode.update({ data: { id } }));

// ❌ 错误：不要使用 Store
import { useAdminStore } from '@/stores/admin';
const admin = useAdminStore();
const result = await admin.updateProjectPhaseNode(data); // 禁止！
```

**原因：**

- Store 应只用于存储应用状态，不应包含 API 请求方法
- 直接使用 Apis 可以获得更好的类型推导和自动补全
- 减少了一层不必要的抽象，代码更清晰易维护
- 统一的 API 调用规范，避免混乱

### 原则 2：第一个参数必须是纯粹的 API 请求

`useReq` 和 `useReqComputed` 的第一个参数**只能返回 Apis 请求**，不能包含任何业务逻辑。

```typescript
// ✅ 正确：第一个参数只返回 API 请求
const { send: updatePhase, loading } = useReq((data: PhaseData) =>
  Apis.adminProjectPhaseNode.update({ data })
);

// 业务逻辑放在 handler 中
const handleSave = async () => {
  if (!formData.value.name) {
    message.error('名称不能为空');
    return;
  }

  try {
    await updatePhase(formData.value);
    message.success('保存成功');
    emit('refresh');
  } catch (error) {
    message.error('保存失败');
  }
};

// ❌ 错误：在 useReq 内部写业务逻辑
const { send: updatePhase } = useReq(async () => {
  if (!formData.value.name) {
    // ❌ 验证逻辑不应该在这里
    throw new Error('名称不能为空');
  }
  const result = await Apis.adminProjectPhaseNode.update({ data: formData.value });
  message.success('保存成功'); // ❌ 成功提示不应该在这里
  emit('refresh'); // ❌ 副作用不应该在这里
  return result;
});
```

**原因：**

- 保持请求定义的纯粹性，便于复用和测试
- 业务逻辑集中在 handler 中，代码更清晰
- 便于错误处理和状态管理
- 符合单一职责原则

### 原则 3：请求定义中不得直接使用 props、ref 等响应式变量

`useReq` 和 `useReqComputed` 的第一个参数中**不能直接访问 props、ref、computed** 等响应式变量，所有动态值都必须通过参数在调用 `send` 时传入。

```typescript
// ✅ 正确：通过参数传入动态值
const { send: updatePhase, loading } = useReq((id: string, data: PhaseData) =>
  Apis.adminProjectPhaseNode.update({
    pathParams: { id },
    data,
  })
);

const handleSave = async () => {
  // 在 send 时传入参数
  await updatePhase(props.currentPhase.id, formData.value);
};

// ❌ 错误：直接在请求定义中访问 props/ref
const { send: updatePhase, loading } = useReq(() =>
  Apis.adminProjectPhaseNode.update({
    pathParams: { id: props.currentPhase.id }, // ❌ 不要直接访问 props
    data: formData.value, // ❌ 不要直接访问 ref
  })
);

// ❌ 错误：直接在请求定义中访问 computed
const currentId = computed(() => props.currentPhase?.id);
const { send: updatePhase } = useReq(() =>
  Apis.adminProjectPhaseNode.update({
    pathParams: { id: currentId.value }, // ❌ 不要直接访问 computed
  })
);
```

**原因：**

- 保证请求定义的可复用性，同一个请求可以用不同参数多次调用
- 避免闭包陷阱，确保每次调用使用的是最新值
- 便于测试和调试，参数来源清晰
- 符合函数式编程的纯函数原则

**正确的完整示例：**

```typescript
const props = defineProps<{
  currentPhase?: AuditProjectPhaseTreeNode;
}>();

const formData = ref({ name: '', code: '' });

// ✅ 请求定义：通过参数接收所有需要的值
const { send: updatePhase, loading: saveLoading } = useReq((id: string, updateData: any) =>
  Apis.adminProjectPhaseNode.update({
    pathParams: { id },
    data: updateData,
  })
);

const { send: deletePhase, loading: deleteLoading } = useReq((ids: string[]) =>
  Apis.adminProjectPhaseNode.delete({
    data: { ids },
  })
);

// ✅ handler 中传入参数
const handleSave = async () => {
  if (!props.currentPhase?.id) {
    message.error('阶段ID不存在');
    return;
  }

  try {
    // 在调用时传入 props 和 ref 的值
    await updatePhase(props.currentPhase.id, formData.value);
    message.success('保存成功');
  } catch (error) {
    message.error('保存失败');
  }
};

const handleDelete = async () => {
  if (!props.currentPhase?.id) {
    message.error('阶段ID不存在');
    return;
  }

  try {
    // 在调用时传入 props 的值
    await deletePhase([props.currentPhase.id]);
    message.success('删除成功');
  } catch (error) {
    message.error('删除失败');
  }
};
```

### 原则 4：合理使用 loading 状态

使用 `useReq` 返回的 `loading` 状态，不要手动管理 loading。

```typescript
// ✅ 正确：使用 useReq 的 loading
const { send: updatePhase, loading: saveLoading } = useReq(
  (data: any) => Apis.adminProjectPhaseNode.update({ data })
)

const handleSave = async () => {
  await updatePhase(formData.value)
}

// 模板中使用
<n-button :loading="saveLoading" @click="handleSave">保存</n-button>

// ❌ 错误：手动管理 loading
const saveLoading = ref(false)

const handleSave = async () => {
  saveLoading.value = true  // ❌ 不需要手动设置
  try {
    await Apis.adminProjectPhaseNode.update({ data: formData.value })
  } finally {
    saveLoading.value = false  // ❌ 不需要手动设置
  }
}
```

---

## 核心 Composables 概览

### useReq

用于需要手动触发的请求，适合需要动态参数或条件触发的场景。

### useReqComputed

用于需要响应式数据和自动缓存的请求，适合频繁调用且结果稳定的场景。

### useInit

用于组件初始化时的数据加载，支持依赖追踪和自动重新执行。

---

## 1. useReqComputed - 响应式数据请求

### 基础用法

适用于需要立即获取数据、支持响应式更新和缓存的场景。

```typescript
const { loading, send, data } = useReqComputed(Apis.platform.orgListAllUnit());

// 在初始化时调用
useInit(() => {
  send();
});

// 使用响应式数据
const options = computed(() => {
  return data.value?.items || [];
});
```

### 带参数的请求

当请求需要动态参数时，将 API 调用包装为函数：

```typescript
const { loading, send } = useReqComputed((templateId: string) =>
  Apis.auditProjectPhaseNodes.tree({
    params: { templateId },
  })
);

// 调用时传入参数
await send(templateId);
```

### 数据处理 - onDataRefresh

使用 `.onDataRefresh()` 链式调用处理返回的数据：

```typescript
const userList = ref<PlatformUser[]>([]);

const { loading, send } = useReqComputed(Apis.platform.userListV2).onDataRefresh(data => {
  userList.value = data.value?.items || [];
});

// 带参数调用
await send({
  params: {
    rootOrgCode: orgCode,
    keyword: searchKeyword,
  },
});
```

**为什么使用 onDataRefresh？**

- 数据转换：将 API 响应转换为组件所需格式
- 过滤处理：应用业务逻辑过滤数据
- 副作用处理：在数据更新时执行其他操作

### 配置选项

#### 缓存控制

```typescript
const { send } = useReqComputed(
  (templateId: string) =>
    Apis.auditProjectPhaseNodes.tree({
      params: { templateId },
    }),
  {
    cacheFor: -1, // -1 = 无限缓存，适合不频繁变化的数据
  }
);
```

**缓存策略建议：**

- `cacheFor: -1`：用于字典、配置等静态数据
- `cacheFor: 300000`：用于相对稳定的数据（5分钟）
- 不设置：用于需要实时更新的数据

---

## 2. useReq - 手动触发请求

### 重要提醒

⚠️ **useReq 第一个参数必须是纯粹的 API 请求，不能包含业务逻辑！**

所有验证、提示、副作用等业务逻辑都应该在外层 handler 函数中处理。

### 基础用法

适用于需要完全手动控制、可能需要多次不同参数调用的场景。

```typescript
const { send: fetchProjectDetail, loading } = useReq((projectId: string) =>
  Apis.auditProject.get({
    pathParams: { id: projectId },
  })
);

// 手动调用
const result = await fetchProjectDetail(projectId.value);
```

### 正确示例：分离请求和业务逻辑

```typescript
// ✅ 正确：请求定义只包含 API 调用
const { send: updatePhase, loading: saveLoading } = useReq((data: PhaseData) =>
  Apis.adminProjectPhaseNode.update({ data })
);

const { send: deletePhase, loading: deleteLoading } = useReq((ids: string[]) =>
  Apis.adminProjectPhaseNode.delete({ data: { ids } })
);

// 业务逻辑在 handler 中处理
const handleSave = async () => {
  // 验证
  if (!formData.value.name?.trim()) {
    message.error('名称不能为空');
    return;
  }

  // 调用请求
  try {
    await updatePhase(formData.value);
    message.success('保存成功');
    emit('refresh');
  } catch (error) {
    console.error('保存失败:', error);
    message.error('保存失败');
  }
};

const handleDelete = () => {
  dialog.warning({
    title: '删除确认',
    content: '确定要删除吗？',
    onPositiveClick: async () => {
      if (!currentId.value) {
        message.error('ID不存在');
        return;
      }

      try {
        await deletePhase([currentId.value]);
        message.success('删除成功');
        emit('refresh');
      } catch (error) {
        console.error('删除失败:', error);
        message.error('删除失败');
      }
    },
  });
};
```

### 错误处理配置

```typescript
const { send, loading } = useReq(
  (projectId: string) =>
    Apis.auditProject.get({
      pathParams: { id: projectId },
    }),
  {
    skipShowError: true, // 跳过自动错误提示，手动处理错误
  }
);

try {
  const result = await send(projectId);
  // 处理成功结果
} catch (error) {
  // 自定义错误处理
}
```

### useReq vs useReqComputed

| 特性        | useReq                 | useReqComputed           |
| ----------- | ---------------------- | ------------------------ |
| 响应式 data | ❌                     | ✅                       |
| 自动缓存    | ❌                     | ✅                       |
| 返回值      | Promise 结果           | 响应式 data              |
| 适用场景    | 一次性请求、需要返回值 | 多次调用、需要响应式数据 |

---

## 3. useInit - 组件初始化

### 基础用法

在组件挂载时自动执行初始化逻辑：

```typescript
useInit(() => {
  fetchUnits();
  fetchOrgTree();
});
```

### 支持异步操作

```typescript
useInit(async () => {
  const resp = await fetchProjectDetail(projectId.value);
  const projectData = resp?.item;

  if (projectData?.type) {
    await getSidebarData(projectData.type);
  }
});
```

### 依赖追踪

当依赖变化时自动重新执行：

```typescript
const projectId = computed(() => route.params.id as string);

useInit(async () => {
  const resp = await fetchProjectDetail(projectId.value);
  // 处理数据...
}, [projectId]); // projectId 变化时重新执行
```

**依赖数组规则：**

- 传入响应式引用数组
- 当任一依赖变化时，重新执行初始化函数
- 类似 React 的 useEffect 依赖数组

---

## 4. 组合使用模式

### 模式 1：级联选择器

适用于多级联动的数据加载场景（如：单位 → 部门 → 用户）。

```typescript
// 1. 定义响应式状态
const unitCode = ref<string>('');
const deptCode = ref<string>('');
const userList = ref<PlatformUser[]>([]);

// 2. 定义请求
const {
  loading: loadingUnits,
  send: fetchUnits,
  data: unitsData,
} = useReqComputed(Apis.platform.orgListAllUnit());

const { loading: loadingUsers, send: fetchUsers } = useReqComputed(
  Apis.platform.userListV2
).onDataRefresh(data => {
  userList.value = data.value?.items || [];
});

// 3. 初始化父级数据
useInit(() => {
  fetchUnits();
});

// 4. 监听级联触发
const currentOrgCode = computed(() => deptCode.value || unitCode.value);

watch(currentOrgCode, newVal => {
  if (newVal) {
    fetchUsers({
      params: { rootOrgCode: newVal },
    });
  }
});

// 5. 重置子级状态
watch(unitCode, (newVal, oldVal) => {
  if (newVal !== oldVal) {
    deptCode.value = '';
    userList.value = [];
  }
});
```

**关键点：**

- 父级变化时重置子级数据
- 使用 computed 合并多个条件
- 条件判断避免无效请求

### 模式 2：搜索防抖

适用于关键词搜索场景：

```typescript
const keyword = ref('');
const userList = ref<PlatformUser[]>([]);

const { loading, send: fetchUserList } = useReqComputed(Apis.platform.userList).onDataRefresh(
  data => {
    let users = data.value?.items || [];
    // 应用过滤器
    users = users.filter(u => !u.disabled);
    if (props.userFilter) {
      users = users.filter(props.userFilter);
    }
    userList.value = users;
  }
);

// 监听搜索关键词
watch(keyword, newKeyword => {
  if (newKeyword) {
    fetchUserList({
      params: { keyword: newKeyword },
    });
  }
});
```

**提示：** 如需防抖，可配合 `es-toolkit` 的 `debounce` 使用：

```typescript
import { debounce } from 'es-toolkit/compat';

const debouncedFetch = debounce((keyword: string) => {
  fetchUserList({ params: { keyword } });
}, 300);

watch(keyword, newKeyword => {
  if (newKeyword) {
    debouncedFetch(newKeyword);
  }
});
```

### 模式 3：依赖序列请求

适用于后续请求依赖前一个请求结果的场景：

```typescript
const projectId = computed(() => route.params.id as string);
const localPhaseTree = ref<AuditProjectPhaseTreeNode[]>([]);

const { send: fetchProjectDetail } = useReq((projectId: string) =>
  Apis.auditProject.get({
    pathParams: { id: projectId },
  })
);

const { send: fetchPhaseTree } = useReqComputed(
  (templateId: string) =>
    Apis.auditProjectPhaseNodes.tree({
      params: { templateId },
    }),
  { cacheFor: -1 }
);

useInit(async () => {
  // 1. 先获取项目详情
  const resp = await fetchProjectDetail(projectId.value);
  const projectData = resp?.item;

  // 2. 使用项目类型获取阶段树
  if (projectData?.type) {
    const treeResp = await fetchPhaseTree(projectData.type);
    if (treeResp?.items) {
      localPhaseTree.value = treeResp.items.filter(item => item !== null);
    }
  }
}, [projectId]);
```

**关键点：**

- 使用 async/await 保证执行顺序
- 在 useInit 中串行调用
- 使用依赖数组实现响应式重新加载

---

## 5. Loading 状态管理

### 单个请求

```typescript
const { loading, send } = useReqComputed(Apis.platform.userList)

// 模板中使用
<n-select :loading="loading" />
```

### 合并多个 Loading

```typescript
const { loading: projectLoading } = useReq(...)
const { loading: phaseLoading } = useReqComputed(...)

const isLoading = computed(() => projectLoading.value || phaseLoading.value)

// 模板中使用
<div v-if="isLoading">加载中...</div>
```

---

## 6. 常见问题与解决方案

### Q1: 数据未及时更新？

**原因：** 使用了 `useReq` 但期望响应式数据

**解决：** 改用 `useReqComputed` + `.onDataRefresh()`

```typescript
// ❌ 错误：data 不是响应式的
const { send, data } = useReq(Apis.platform.userList);
const list = ref(data); // data 不会自动更新

// ✅ 正确：使用 useReqComputed
const list = ref<User[]>([]);
const { send } = useReqComputed(Apis.platform.userList).onDataRefresh(data => {
  list.value = data.value?.items || [];
});
```

### Q2: 重复请求如何避免？

**方案 1：** 使用缓存

```typescript
const { send } = useReqComputed(
  Apis.platform.orgListAllUnit(),
  { cacheFor: -1 } // 启用缓存
);
```

**方案 2：** 条件判断

```typescript
watch(keyword, newKeyword => {
  if (!newKeyword) return; // 空关键词不请求
  if (newKeyword.length < 2) return; // 少于2个字符不请求
  fetchUserList({ params: { keyword: newKeyword } });
});
```

### Q3: 如何处理请求失败？

**方案 1：** 跳过自动错误提示，自定义处理

```typescript
const { send } = useReq(Apis.xxx.get, { skipShowError: true });

try {
  const result = await send();
} catch (error) {
  // 自定义错误处理
  console.error('请求失败：', error);
  window.$message.error('数据加载失败，请重试');
}
```

**方案 2：** 使用全局错误处理（默认行为）

```typescript
const { send } = useReq(Apis.xxx.get);
// 错误会自动显示消息提示
```

### Q4: 组件卸载时如何取消请求？

Alova 会自动处理组件卸载时的请求取消，无需手动处理。

---

## 7. 最佳实践清单

### 🚨 核心原则（必须遵守）

- ✅ **必须使用 Apis**：禁止通过 Store 包装调用 API
- ✅ **第一个参数纯粹**：useReq/useReqComputed 第一个参数只返回 API 请求，不包含业务逻辑
- ✅ **不得直接使用响应式变量**：请求定义中不能直接访问 props、ref、computed，通过参数传入
- ✅ **业务逻辑外置**：验证、提示、副作用等都在 handler 中处理
- ✅ **使用 loading 状态**：使用 useReq 返回的 loading，不手动管理

### 选择合适的 Composable

- ✅ **useReqComputed**：需要响应式数据、频繁调用、需要缓存
- ✅ **useReq**：一次性请求、需要明确的返回值、复杂错误处理
- ✅ **useInit**：组件初始化、依赖追踪、自动重新加载

### 代码组织

- ✅ 将请求定义放在组件顶部，状态定义之后
- ✅ 使用 `useInit` 统一管理初始化逻辑
- ✅ 使用 `watch` 处理响应式触发
- ✅ 使用 `computed` 合并多个状态或 loading

### 性能优化

- ✅ 为不常变化的数据启用缓存（`cacheFor: -1`）
- ✅ 使用条件判断避免无效请求
- ✅ 搜索场景使用防抖（`debounce`）
- ✅ 大列表使用虚拟滚动组件

### 错误处理

- ✅ 默认使用全局错误提示
- ✅ 特殊场景使用 `skipShowError: true` 自定义处理
- ✅ 关键请求使用 `try/catch` 包裹

### 类型安全

- ✅ 使用 TypeScript 定义请求参数和返回类型
- ✅ 为 `ref` 声明明确的类型
- ✅ 使用 API 自动生成的类型定义

### API 调用规范

- ✅ **必须使用 Apis 对象**：`import Apis from '@/api'`
- ❌ **禁止使用 Store**：不要通过 `useAdminStore()` 等调用 API
- ✅ **参数格式统一**：`pathParams`、`params`、`data` 按规范使用
- ✅ **请求定义纯粹**：useReq/useReqComputed 第一个参数只返回 Apis 调用

---

## 8. 参考示例

### 完整示例 1：级联选择器

参考文件：`src/components/users/OrgUserSelectV2.vue`

### 完整示例 2：搜索选择器

参考文件：`src/components/users/UserSelect.vue`

### 完整示例 3：依赖序列加载

参考文件：`src/views/audit/ProjectLeftMenu.vue`

---

## 9. 扩展阅读

- [Alova 官方文档](https://alova.js.org/)
- [项目 Alova 使用笔记](../alova-usage-notes.md)
- [API Composable 源码](../../src/api/composable.ts)
