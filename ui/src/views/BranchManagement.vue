<template>
  <div class="branch-page">
    <n-page-header>
      <template #title>
        <n-space align="center" size="small">
          <n-button quaternary size="small" @click="goBackToWorkspace" :disabled="!currentProjectId">
            <template #icon>
              <n-icon><ChevronBackOutline /></n-icon>
            </template>
            返回项目
          </n-button>
          <span>{{ pageHeading }}</span>
        </n-space>
      </template>
      <template #extra>
        <n-space>
          <n-button quaternary :disabled="!currentProjectId" :loading="projectStore.loading" @click="reloadBranches(true)">
            <template #icon>
              <n-icon><RefreshOutline /></n-icon>
            </template>
            刷新
          </n-button>
          <n-button type="primary" :disabled="!currentProjectId" @click="openCreateModal()">
            <template #icon>
              <n-icon><AddOutline /></n-icon>
            </template>
            新建分支
          </n-button>
        </n-space>
      </template>
    </n-page-header>

    <n-space class="branch-toolbar" align="center">
      <n-input
        ref="searchInputRef"
        v-model:value="searchInput"
        round
        clearable
        size="large"
        placeholder="搜索分支 (Ctrl+F)"
      >
        <template #prefix>
          <n-icon><SearchOutline /></n-icon>
        </template>
      </n-input>
      <n-statistic label="本地分支">{{ branchList.local.length }}</n-statistic>
      <n-statistic label="远程分支">{{ branchList.remote.length }}</n-statistic>
      <n-statistic label="已绑定 Worktree">{{ worktreeBoundCount }}</n-statistic>
    </n-space>

    <n-alert v-if="branchError" type="error" class="branch-alert" closable @close="branchError = null">
      {{ branchError }}
    </n-alert>

    <n-spin :show="branchLoading">
      <n-grid cols="24" x-gap="16" y-gap="16">
        <n-gi :span="24" :lg="12">
          <n-card title="本地分支">
            <template #header-extra>
              <n-text depth="3">共 {{ filteredLocalBranches.length }} 个</n-text>
            </template>
            <template v-if="filteredLocalBranches.length === 0">
              <n-empty description="暂无本地分支" />
            </template>
            <template v-else>
              <n-virtual-list
                v-if="useVirtualLocal"
                class="branch-list branch-list--virtual"
                :items="filteredLocalBranches"
                :item-size="86"
              >
                <template #default="{ item }">
                  <BranchListItem
                    :branch="item"
                    mode="local"
                    :default-branch="defaultBranch"
                    @create-worktree="handleCreateWorktree"
                    @open-worktree="handleOpenWorktree"
                    @delete="handleDeleteBranch"
                  />
                </template>
              </n-virtual-list>
              <div v-else class="branch-list">
                <BranchListItem
                  v-for="branch in filteredLocalBranches"
                  :key="branch.name"
                  :branch="branch"
                  mode="local"
                  :default-branch="defaultBranch"
                  @create-worktree="handleCreateWorktree"
                  @open-worktree="handleOpenWorktree"
                  @delete="handleDeleteBranch"
                />
              </div>
            </template>
          </n-card>
        </n-gi>

        <n-gi :span="24" :lg="12">
          <n-card title="远程分支">
            <template #header-extra>
              <n-text depth="3">共 {{ filteredRemoteBranches.length }} 个</n-text>
            </template>
            <template v-if="filteredRemoteBranches.length === 0">
              <n-empty description="暂无远程分支" />
            </template>
            <template v-else>
              <n-virtual-list
                v-if="useVirtualRemote"
                class="branch-list branch-list--virtual"
                :items="filteredRemoteBranches"
                :item-size="86"
              >
                <template #default="{ item }">
                  <BranchListItem
                    :branch="item"
                    mode="remote"
                    @checkout="handleCheckoutRemote"
                  />
                </template>
              </n-virtual-list>
              <div v-else class="branch-list">
                <BranchListItem
                  v-for="branch in filteredRemoteBranches"
                  :key="branch.name"
                  :branch="branch"
                  mode="remote"
                  @checkout="handleCheckoutRemote"
                />
              </div>
            </template>
          </n-card>
        </n-gi>

        <n-gi :span="24">
          <div ref="mergeSectionRef">
            <n-card title="合并与冲突检测">
              <template #header-extra>
                <n-button secondary size="small" :disabled="!canMerge" @click="scrollToMerge">
                  定位到表单
                </n-button>
              </template>
              <n-form ref="mergeFormRef" :model="mergeForm" :rules="mergeFormRules" label-placement="left">
                <n-grid cols="1 640:2" x-gap="16">
                  <n-gi>
                    <n-form-item label="目标 Worktree" path="worktreeId">
                      <n-select
                        v-model:value="mergeForm.worktreeId"
                        :options="worktreeOptions"
                        placeholder="请选择 Worktree"
                        :disabled="worktreeOptions.length === 0"
                      />
                    </n-form-item>
                  </n-gi>
                  <n-gi>
                    <n-form-item label="目标分支" path="targetBranch">
                      <n-select
                        v-model:value="mergeForm.targetBranch"
                        :options="localBranchOptions"
                        filterable
                        placeholder="请选择目标分支"
                      />
                    </n-form-item>
                  </n-gi>
                  <n-gi>
                    <n-form-item label="源分支" path="sourceBranch">
                      <n-select
                        v-model:value="mergeForm.sourceBranch"
                        :options="localBranchOptions"
                        filterable
                        placeholder="请选择要合并的分支"
                      />
                    </n-form-item>
                  </n-gi>
                </n-grid>

                <n-form-item label="策略">
                  <n-radio-group v-model:value="mergeForm.strategy">
                    <n-radio value="merge">Merge</n-radio>
                    <n-radio value="rebase">Rebase</n-radio>
                    <n-radio value="squash">Squash</n-radio>
                  </n-radio-group>
                </n-form-item>
                <template v-if="showSquashCommitOptions">
                  <n-form-item label="提交控制">
                    <n-space align="center">
                      <n-checkbox v-model:checked="mergeForm.commitImmediately">同时提交</n-checkbox>
                      <n-text depth="3">勾选后会在 squash 结束后立刻创建 commit</n-text>
                    </n-space>
                  </n-form-item>
                  <n-form-item v-if="shouldCommitAfterSquash" label="提交信息" path="commitMessage">
                    <n-input
                      v-model:value="mergeForm.commitMessage"
                      type="textarea"
                      :autosize="{ minRows: 2, maxRows: 4 }"
                      placeholder="feat: 描述本次 Squash 的改动"
                    />
                  </n-form-item>
                </template>

                <n-space>
                  <n-button
                    type="primary"
                    :loading="mergeBranchReq.loading.value"
                    :disabled="!canExecuteMerge"
                    @click="submitMerge"
                  >
                    执行合并
                  </n-button>
                  <n-button @click="refreshMergeStatus" :disabled="!mergeForm.worktreeId">
                    刷新 Worktree 状态
                  </n-button>
                </n-space>

                <n-alert
                  v-if="mergeResult"
                  :type="mergeResult.success ? 'success' : 'warning'"
                  show-icon
                  class="merge-result"
                >
                  {{ mergeResult.message }}
                  <template v-if="mergeResult.conflicts?.length">
                    <div class="conflict-list">
                      <div v-for="file in mergeResult.conflicts" :key="file">{{ file }}</div>
                    </div>
                  </template>
                </n-alert>
              </n-form>
            </n-card>
          </div>
        </n-gi>
      </n-grid>
    </n-spin>

    <n-modal v-model:show="showCreateModal" preset="dialog" title="新建分支" :mask-closable="false">
      <n-form ref="createFormRef" :model="createForm" :rules="createFormRules" label-placement="top">
        <n-form-item label="分支名称" path="name">
          <n-input v-model:value="createForm.name" placeholder="feature/awesome" />
        </n-form-item>
        <n-form-item label="基础分支" path="base">
          <n-select
            v-model:value="createForm.base"
            :options="baseBranchOptions"
            filterable
            placeholder="默认为项目默认分支"
          />
        </n-form-item>
        <n-form-item>
          <n-checkbox v-model:checked="createForm.createWorktree">同时创建 Worktree</n-checkbox>
        </n-form-item>
      </n-form>
      <template #action>
        <n-space justify="end">
          <n-button @click="closeCreateModal">取消</n-button>
          <n-button type="primary" :loading="createBranchReq.loading.value" @click="submitCreateBranch">
            创建
          </n-button>
        </n-space>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useDialog, useMessage, type FormInst, type FormRules, type InputInst } from 'naive-ui';
import { useTitle } from '@vueuse/core';
import { AddOutline, ChevronBackOutline, RefreshOutline, SearchOutline } from '@vicons/ionicons5';
import type { BranchInfo, BranchListResult, MergeResult } from '@/types/models';
import { useProjectStore } from '@/stores/project';
import { debounce } from '@/utils/debounce';
import Apis from '@/api';
import { useReq } from '@/api/composable';
import { extractItem } from '@/api/response';
import BranchListItem from '@/components/branch/BranchListItem.vue';
import { useHotkeys } from '@/composables/useHotkeys';

const route = useRoute();
const router = useRouter();
const projectStore = useProjectStore();
const dialog = useDialog();
const message = useMessage();

const currentProjectId = computed(() => (typeof route.params.id === 'string' ? route.params.id : ''));
const pageHeading = computed(() =>
  projectStore.currentProject ? `${projectStore.currentProject.name} · 分支管理` : '分支管理',
);
useTitle(
  computed(() =>
    projectStore.currentProject ? `${projectStore.currentProject.name} - 分支管理` : '分支管理',
  ),
);

const branchListReq = useReq((projectId: string) =>
  Apis.branch.list({
    pathParams: { projectId },
  }),
  { cacheFor: 60000 },
);

const createBranchReq = useReq(
  (projectId: string, payload: { name: string; base?: string; createWorktree?: boolean }) =>
    Apis.branch.create({
      pathParams: { projectId },
      data: {
        name: payload.name,
        base: payload.base ?? '',
        createWorktree: payload.createWorktree ?? false,
      },
    }),
);

const deleteBranchReq = useReq(
  (projectId: string, branchName: string, force: boolean) =>
    Apis.branch.delete({
      pathParams: { projectId, branchName },
      params: { force },
    }),
);

const mergeBranchReq = useReq(
  (
    worktreeId: string,
    payload: {
      targetBranch: string;
      sourceBranch: string;
      strategy: 'merge' | 'rebase' | 'squash';
      commit: boolean;
      commitMessage: string;
    },
  ) =>
    Apis.branch.merge({
      pathParams: { id: worktreeId },
      data: payload,
    }),
);

const refreshWorktreeStatusReq = useReq(
  (worktreeId: string) => Apis.worktree.refreshStatus({
    pathParams: { id: worktreeId }
  })
);

const searchInputRef = ref<InputInst | null>(null);
const searchInput = ref('');
const searchTerm = ref('');
const branchError = ref<string | null>(null);
const mergeResult = ref<MergeResult | null>(null);
const mergeSectionRef = ref<HTMLElement | null>(null);

const createFormRef = ref<FormInst | null>(null);
const createForm = reactive({
  name: '',
  base: '',
  createWorktree: false,
});
const createFormRules: FormRules = {
  name: [{ required: true, message: '请输入分支名称' }],
};

const mergeFormRef = ref<FormInst | null>(null);
const mergeForm = reactive({
  worktreeId: '',
  targetBranch: '',
  sourceBranch: '',
  strategy: 'merge' as 'merge' | 'rebase' | 'squash',
  commitImmediately: true,
  commitMessage: '',
});
const showSquashCommitOptions = computed(() => mergeForm.strategy === 'squash');
const shouldCommitAfterSquash = computed(() => showSquashCommitOptions.value && mergeForm.commitImmediately);
const mergeFormRules: FormRules = {
  worktreeId: [{ required: true, message: '请选择 Worktree' }],
  targetBranch: [{ required: true, message: '请选择目标分支' }],
  sourceBranch: [{ required: true, message: '请选择源分支' }],
  commitMessage: [
    {
      trigger: ['input', 'blur'],
      validator: () => {
        if (shouldCommitAfterSquash.value && !mergeForm.commitMessage.trim()) {
          return new Error('请输入提交信息');
        }
        return true;
      },
    },
  ],
};

watch(
  () => route.params.id,
  id => {
    if (typeof id === 'string' && id) {
      initializeProject(id);
    }
  },
  { immediate: true },
);

watch(
  () => mergeForm.worktreeId,
  worktreeId => syncTargetBranchFromSelection(worktreeId, true),
);

watch(
  () => projectStore.worktrees.map(worktree => `${worktree.id}:${worktree.branchName}`).join(','),
  () => {
    if (mergeForm.worktreeId) {
      syncTargetBranchFromSelection(mergeForm.worktreeId, false);
    }
  },
);

watch(
  () => mergeForm.strategy,
  strategy => {
    if (strategy !== 'squash') {
      mergeForm.commitMessage = '';
      mergeForm.commitImmediately = true;
    }
  },
);

async function initializeProject(id: string) {
  try {
    await projectStore.fetchProject(id);
    createForm.base = projectStore.currentProject?.defaultBranch ?? '';
    await reloadBranches(true);
  } catch (error: any) {
    branchError.value = error?.message ?? '加载项目失败';
  }
}

async function reloadBranches(force = false) {
  if (!currentProjectId.value) {
    return;
  }
  branchError.value = null;
  try {
    if (force) {
      await branchListReq.forceReload(currentProjectId.value);
    } else {
      await branchListReq.send(currentProjectId.value);
    }
  } catch (error: any) {
    branchError.value = error?.message ?? '获取分支失败';
  }
}

const branchList = computed<BranchListResult>(() => {
  const payload = extractItem(branchListReq.data.value) as BranchListResult | undefined;
  return payload ?? { local: [], remote: [] };
});

const branchLoading = computed(() => branchListReq.loading.value || projectStore.loading);

const defaultBranch = computed(() => projectStore.currentProject?.defaultBranch ?? '');
const worktreeBoundCount = computed(
  () => branchList.value.local.filter(branch => branch.hasWorktree).length,
);

const searchApply = debounce((value: string) => {
  searchTerm.value = value.trim().toLowerCase();
}, 200);

watch(searchInput, value => searchApply(value));

const filteredLocalBranches = computed(() => filterBranches(branchList.value.local));
const filteredRemoteBranches = computed(() => filterBranches(branchList.value.remote));

const useVirtualLocal = computed(() => filteredLocalBranches.value.length > 200);
const useVirtualRemote = computed(() => filteredRemoteBranches.value.length > 200);

const worktreeOptions = computed(() =>
  projectStore.worktrees.map(worktree => ({
    label: `${worktree.branchName} · ${worktree.path}`,
    value: worktree.id,
  })),
);

const localBranchOptions = computed(() =>
  branchList.value.local.map(branch => ({
    label: branch.name,
    value: branch.name,
  })),
);

const baseBranchOptions = computed(() => [
  ...(defaultBranch.value
    ? [{ label: `${defaultBranch.value} (默认)`, value: defaultBranch.value }]
    : []),
  ...branchList.value.local
    .filter(branch => branch.name !== defaultBranch.value)
    .map(branch => ({ label: branch.name, value: branch.name })),
]);

const showCreateModal = ref(false);

function openCreateModal(name = '', base = '') {
  createForm.name = name;
  createForm.base = base || defaultBranch.value;
  createForm.createWorktree = false;
  showCreateModal.value = true;
}

function closeCreateModal() {
  showCreateModal.value = false;
}

async function submitCreateBranch() {
  if (!currentProjectId.value) {
    return;
  }
  try {
    await createFormRef.value?.validate();
    await createBranchReq.send(currentProjectId.value, {
      name: createForm.name,
      base: createForm.base,
      createWorktree: createForm.createWorktree,
    });
    message.success('分支创建成功');
    showCreateModal.value = false;
    await reloadBranches(true);
    if (createForm.createWorktree) {
      await projectStore.fetchWorktrees(currentProjectId.value);
    }
  } catch (error: any) {
    if (error?.message) {
      message.error(error.message);
    }
  }
}

function handleCheckoutRemote(branch: BranchInfo) {
  const simplified = branch.name.includes('/') ? branch.name.split('/').slice(1).join('/') : branch.name;
  openCreateModal(simplified, branch.name);
}

async function handleDeleteBranch(branch: BranchInfo) {
  if (!currentProjectId.value) {
    return;
  }
  const requiresForce = Boolean(branch.hasWorktree);
  dialog.warning({
    title: requiresForce ? '强制删除分支' : '删除分支',
    content: `确定要删除分支 "${branch.name}" 吗？${
      requiresForce ? ' 关联的 Worktree 会一并删除。' : ''
    }`,
    negativeText: '取消',
    positiveText: requiresForce ? '强制删除' : '删除',
    onPositiveClick: async () => {
      try {
        await deleteBranchReq.send(currentProjectId.value, branch.name, requiresForce);
        message.success('分支已删除');
        await reloadBranches(true);
        await projectStore.fetchWorktrees(currentProjectId.value);
      } catch (error: any) {
        message.error(error?.message ?? '删除失败');
      }
    },
  });
}

async function handleCreateWorktree(branch: BranchInfo) {
  if (!currentProjectId.value) {
    return;
  }
  try {
    await projectStore.createWorktree(currentProjectId.value, {
      branchName: branch.name,
      baseBranch: branch.name,
      createBranch: false,
    });
    await projectStore.fetchWorktrees(currentProjectId.value);
    await reloadBranches(true);
    message.success('Worktree 创建成功');
  } catch (error: any) {
    message.error(error?.message ?? '创建 Worktree 失败');
  }
}

function handleOpenWorktree(branch: BranchInfo) {
  if (!projectStore.currentProject) {
    return;
  }
  const target = projectStore.worktrees.find(worktree => worktree.branchName === branch.name);
  if (!target) {
    message.warning('尚未创建 Worktree');
    return;
  }
  projectStore.setSelectedWorktree(target.id);
  router.push({ name: 'project', params: { id: projectStore.currentProject.id } });
}

async function submitMerge() {
  const worktreeId = mergeForm.worktreeId;
  const targetBranch = mergeForm.targetBranch.trim();
  const sourceBranch = mergeForm.sourceBranch;
  if (!worktreeId || !sourceBranch) {
    message.warning('请选择 Worktree 和源分支');
    return;
  }
  if (!targetBranch) {
    message.warning('无法确定目标分支，请重新选择 Worktree');
    return;
  }
  const commitAfter = shouldCommitAfterSquash.value;
  const commitMessage = commitAfter ? mergeForm.commitMessage.trim() : '';
  if (commitAfter && !commitMessage) {
    message.warning('请输入提交信息');
    return;
  }
  try {
    const response = await mergeBranchReq.send(worktreeId, {
      targetBranch,
      sourceBranch,
      strategy: mergeForm.strategy,
      commit: commitAfter,
      commitMessage: commitMessage || '',
    });
    const payload = extractItem(response) as MergeResult | undefined;
    if (payload) {
      mergeResult.value = payload;
      if (payload.success) {
        message.success(payload.message || '合并完成');
        if (commitAfter) {
          mergeForm.commitMessage = '';
        }
        const refreshIds = new Set<string>();
        projectStore.worktrees.forEach(worktree => {
          if (worktree.branchName === targetBranch || worktree.branchName === sourceBranch) {
            refreshIds.add(worktree.id);
          }
        });
        if (!refreshIds.has(worktreeId)) {
          refreshIds.add(worktreeId);
        }
        if (refreshIds.size > 0) {
          await Promise.all(Array.from(refreshIds).map(async (id) => {
            const result = await refreshWorktreeStatusReq.send(id);
            const updated = extractItem(result);
            if (updated) {
              projectStore.updateWorktreeInList(id, updated);
            }
          }));
        }
      } else {
        message.warning(payload.message || '存在冲突');
      }
    }
  } catch (error: any) {
    message.error(error?.message ?? '合并失败');
  }
}

async function refreshMergeStatus() {
  if (!mergeForm.worktreeId) {
    return;
  }
  try {
    const result = await refreshWorktreeStatusReq.send(mergeForm.worktreeId);
    const updated = extractItem(result);
    if (updated) {
      projectStore.updateWorktreeInList(mergeForm.worktreeId, updated);
    }
    message.success('Worktree 状态已刷新');
  } catch (error: any) {
    message.error(error?.message ?? '刷新失败');
  }
}

const canMerge = computed(() => projectStore.worktrees.length > 0 && branchList.value.local.length > 1);

const canExecuteMerge = computed(() => {
  // 必须选择了worktree、目标分支和源分支
  if (!mergeForm.worktreeId || !mergeForm.targetBranch || !mergeForm.sourceBranch) {
    return false;
  }
  // 源分支和目标分支不能相同
  if (mergeForm.targetBranch === mergeForm.sourceBranch) {
    return false;
  }
  // 检查选中的worktree状态
  const selectedWorktree = projectStore.worktrees.find(w => w.id === mergeForm.worktreeId);
  if (!selectedWorktree) {
    return false;
  }
  // worktree必须是干净的（没有未提交的更改）
  const hasUncommittedChanges =
    (selectedWorktree.statusModified ?? 0) > 0 ||
    (selectedWorktree.statusStaged ?? 0) > 0 ||
    (selectedWorktree.statusUntracked ?? 0) > 0;
  return !hasUncommittedChanges;
});

function scrollToMerge() {
  mergeSectionRef.value?.scrollIntoView({ behavior: 'smooth', block: 'start' });
}

function goBackToWorkspace() {
  if (!currentProjectId.value) {
    router.push({ name: 'projects' });
  } else {
    router.push({ name: 'project', params: { id: currentProjectId.value } });
  }
}

function filterBranches(list: BranchInfo[]) {
  if (!searchTerm.value) {
    return list;
  }
  return list.filter(branch => {
    const needle = searchTerm.value;
    return (
      branch.name.toLowerCase().includes(needle) ||
      (branch.headCommit || '').toLowerCase().includes(needle)
    );
  });
}

useHotkeys([
  {
    key: 'n',
    ctrl: true,
    handler: () => openCreateModal(),
  },
  {
    key: 'r',
    ctrl: true,
    handler: () => reloadBranches(true),
  },
  {
    key: 'f',
    ctrl: true,
    handler: () => searchInputRef.value?.focus(),
  },
]);

function syncTargetBranchFromSelection(worktreeId: string, force = false) {
  if (!worktreeId) {
    mergeForm.targetBranch = force ? '' : mergeForm.targetBranch;
    return;
  }
  const target = projectStore.worktrees.find(worktree => worktree.id === worktreeId);
  if (!target) {
    if (force) {
      mergeForm.targetBranch = '';
    }
    return;
  }
  if (force || !mergeForm.targetBranch) {
    mergeForm.targetBranch = target.branchName;
  }
}
</script>

<style scoped>
.branch-page {
  padding: 24px;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.branch-toolbar {
  gap: 24px;
  flex-wrap: wrap;
}

.branch-toolbar :deep(.n-input) {
  min-width: 260px;
  max-width: 360px;
}

.branch-summary {
  width: 100%;
}

.branch-alert {
  margin-bottom: 8px;
}

.branch-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
  max-height: 60vh;
  overflow-y: auto;
}

.branch-list--virtual {
  max-height: 60vh;
}

.merge-result {
  margin-top: 16px;
}

.conflict-list {
  margin-top: 8px;
  font-size: 12px;
  line-height: 1.6;
}
</style>
