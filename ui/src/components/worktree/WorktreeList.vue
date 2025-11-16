<template>
  <div class="worktree-list">
    <div class="list-header">
      <n-space justify="space-between" align="center">
        <h3>Worktrees</h3>
        <n-space align="center">
          <n-button
            text
            size="small"
            :disabled="!projectStore.currentProject"
            @click="goToBranchManagement"
          >
            <template #icon>
              <n-icon><GitBranchOutline /></n-icon>
            </template>
            分支
          </n-button>
          <n-button
            text
            @click="handleRefreshAll"
            :loading="refreshAllWorktreesReq.loading.value"
            :disabled="!projectStore.currentProject || !gitFeaturesAvailable"
          >
            <template #icon>
              <n-icon><RefreshOutline /></n-icon>
            </template>
          </n-button>
          <n-button
            type="primary"
            size="small"
            @click="showCreateDialog = true"
            :disabled="!projectStore.currentProject || !gitFeaturesAvailable"
          >
            <template #icon>
              <n-icon><AddOutline /></n-icon>
            </template>
            新建
          </n-button>
        </n-space>
      </n-space>
    </div>

    <n-alert
      v-if="showGitWarning"
      type="warning"
      class="git-warning"
      title="Worktree 功能不可用"
      :show-icon="false"
    >
      当前项目目录不是 Git 仓库，无法创建或刷新 Worktree；仍可使用任务等其他功能。
    </n-alert>

    <n-scrollbar style="max-height: calc(100vh - 80px)">
      <div v-if="projectStore.worktrees.length" class="worktree-items">
        <WorktreeCard
          v-for="worktree in projectStore.worktrees"
          :key="worktree.id"
          :worktree="worktree"
          :selected="projectStore.selectedWorktreeId === worktree.id"
          :can-sync="canSyncWorktree(worktree)"
          :can-merge="canMergeWorktree(worktree)"
          :can-commit="canCommitWorktree(worktree)"
          :is-deleting="deletingWorktreeId === worktree.id"
          :default-editor="defaultEditorPreference"
          :editor-options="worktreeEditorOptions"
          @select="handleSelectWorktree"
          @refresh="handleRefresh"
          @delete="confirmDelete"
          @open-explorer="handleOpenExplorer"
          @open-terminal="handleOpenTerminal"
          @open-editor="handleOpenEditor"
          @sync-default="openSyncDialog"
          @merge-to-default="openMergeDialog"
          @commit-worktree="openCommitDialog"
        />
      </div>
      <n-empty v-else description="暂无 Worktree" class="worktree-empty" />
    </n-scrollbar>

    <WorktreeCreateDialog
      v-if="projectStore.currentProject"
      v-model:show="showCreateDialog"
      @success="handleWorktreeCreated"
    />

    <n-modal
      v-model:show="branchOperation.visible"
      preset="dialog"
      :title="branchOperationTitle"
      :mask-closable="false"
    >
      <n-space vertical size="large">
        <n-text>{{ branchOperationDescription }}</n-text>
        <div>
          <n-select
            v-if="branchOperationOptions.length"
            v-model:value="branchOperation.targetBranch"
            :options="branchOperationOptions"
            filterable
            placeholder="请选择分支"
          />
          <n-alert v-else type="warning" show-icon>
            暂无可用分支，请先在分支管理中创建或创建对应 Worktree。
          </n-alert>
        </div>
        <div v-if="branchOperation.type === 'merge'">
          <n-radio-group v-model:value="branchOperation.strategy" size="small">
            <n-radio value="merge">Merge</n-radio>
            <n-radio value="squash">Squash</n-radio>
          </n-radio-group>
        </div>
        <div v-if="showSquashCommitOptions">
          <n-space vertical size="small">
            <n-space align="center">
              <n-checkbox v-model:checked="branchOperation.commitImmediately">自动提交</n-checkbox>
              <n-text depth="3">勾选后会在 squash 结束后立刻创建 commit</n-text>
            </n-space>
            <n-input
              v-if="shouldCommitAfterSquash"
              v-model:value="branchOperation.commitMessage"
              type="textarea"
              :autosize="{ minRows: 2, maxRows: 4 }"
              placeholder="feat: 描述本次 Squash 的改动"
            />
          </n-space>
        </div>
      </n-space>
      <template #action>
        <n-button @click="closeBranchOperation" :disabled="branchOperationLoading">取消</n-button>
        <n-button
          type="primary"
          :loading="branchOperationLoading"
          :disabled="!branchOperationOptions.length"
          @click="confirmBranchOperation"
        >
          执行
        </n-button>
      </template>
    </n-modal>

    <n-modal
      v-model:show="commitDialog.visible"
      preset="dialog"
      title="提交更改"
      :mask-closable="false"
    >
      <n-space vertical size="large">
        <n-text>
          在 Worktree「{{ commitDialog.worktree?.branchName ?? '' }}」中提交所有更改。
        </n-text>
        <n-input
          v-model:value="commitDialog.message"
          type="textarea"
          placeholder="请输入提交信息"
          :autosize="{ minRows: 3, maxRows: 6 }"
        />
      </n-space>
      <template #action>
        <n-button @click="closeCommitDialog" :disabled="commitWorktreeReq.loading.value">取消</n-button>
        <n-button
          type="primary"
          :loading="commitWorktreeReq.loading.value"
          :disabled="!commitDialog.message.trim()"
          @click="submitCommit"
        >
          提交
        </n-button>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue';
import { storeToRefs } from 'pinia';
import { useRouter } from 'vue-router';
import { useDialog, useMessage } from 'naive-ui';
import { AddOutline, GitBranchOutline, RefreshOutline } from '@vicons/ionicons5';
import WorktreeCard from './WorktreeCard.vue';
import WorktreeCreateDialog from './WorktreeCreateDialog.vue';
import { useProjectStore } from '@/stores/project';
import { useSettingsStore } from '@/stores/settings';
import Apis from '@/api';
import { useReq } from '@/api/composable';
import { extractItem } from '@/api/response';
import type { BranchListResult, MergeResult, Worktree } from '@/types/models';
import type { EditorPreference } from '@/stores/settings';
import { DEFAULT_EDITOR, EDITOR_OPTIONS, EDITOR_LABEL_MAP } from '@/constants/editor';

const emit = defineEmits<{
  'open-terminal': [payload: Worktree];
}>();

const projectStore = useProjectStore();
const router = useRouter();
const message = useMessage();
const dialog = useDialog();
const settingsStore = useSettingsStore();
const { editorSettings } = storeToRefs(settingsStore);

const defaultEditorPreference = computed<EditorPreference>(
  () => editorSettings.value.defaultEditor ?? DEFAULT_EDITOR,
);
const customEditorCommand = computed(() => editorSettings.value.customCommand?.trim() ?? '');
const worktreeEditorOptions = computed(() =>
  EDITOR_OPTIONS.map(option => ({
    ...option,
    disabled: option.value === 'custom' && !customEditorCommand.value,
  })),
);

const showCreateDialog = ref(false);

// 刷新所有 worktree 状态
const refreshAllWorktreesReq = useReq(
  (projectId: string) => Apis.worktree.refreshAllByProject({
    pathParams: { projectId }
  })
);

// 刷新单个 worktree 状态
const refreshWorktreeStatusReq = useReq(
  (worktreeId: string) => Apis.worktree.refreshStatus({
    pathParams: { id: worktreeId }
  })
);

const defaultBranch = computed(() => projectStore.currentProject?.defaultBranch ?? '');
const mainWorktree = computed(() => projectStore.worktrees.find(worktree => worktree.isMain) ?? null);
const hasMainWorktree = computed(() => Boolean(mainWorktree.value));
const gitFeaturesAvailable = computed(() => {
  if (!projectStore.currentProject) {
    return false;
  }
  if (projectStore.loading) {
    return true;
  }
  return hasMainWorktree.value;
});
const showGitWarning = computed(
  () => Boolean(projectStore.currentProject) && !projectStore.loading && !hasMainWorktree.value
);

// 判断worktree是否有未提交的更改
function hasUncommittedChanges(worktree: Worktree): boolean {
  return (
    (worktree.statusModified ?? 0) > 0 ||
    (worktree.statusStaged ?? 0) > 0 ||
    (worktree.statusUntracked ?? 0) > 0
  );
}

// 判断worktree是否可以进行sync/rebase操作
function canSyncWorktree(worktree: Worktree): boolean {
  // 需要有默认分支，worktree是干净的，且当前分支不是默认分支
  // 注意：isMain 是指主worktree（项目根目录），不是main分支，所以要用 branchName 判断
  return (
    Boolean(defaultBranch.value) &&
    !hasUncommittedChanges(worktree) &&
    worktree.branchName !== defaultBranch.value
  );
}

// 判断worktree是否可以进行merge操作
function canMergeWorktree(worktree: Worktree): boolean {
  // 需要有主worktree，worktree是干净的，且当前分支不是默认分支
  // 注意：isMain 是指主worktree（项目根目录），不是main分支，所以要用 branchName 判断
  return (
    Boolean(mainWorktree.value) &&
    !hasUncommittedChanges(worktree) &&
    worktree.branchName !== defaultBranch.value
  );
}

// 判断worktree是否可以进行commit操作
function canCommitWorktree(worktree: Worktree): boolean {
  // git功能可用，且有待提交的内容
  return gitFeaturesAvailable.value && hasUncommittedChanges(worktree);
}

type BranchOperationType = 'rebase' | 'merge';

const branchOperation = reactive({
  visible: false,
  type: null as BranchOperationType | null,
  worktree: null as Worktree | null,
  targetBranch: '',
  strategy: 'merge' as 'merge' | 'squash' | 'rebase',
  commitImmediately: true,
  commitMessage: '',
});
const branchOperationLoading = ref(false);
const showSquashCommitOptions = computed(() => branchOperation.strategy === 'squash');
const shouldCommitAfterSquash = computed(
  () => showSquashCommitOptions.value && branchOperation.commitImmediately,
);
const commitDialog = reactive({
  visible: false,
  worktree: null as Worktree | null,
  message: '',
});
const deletingWorktreeId = ref<string | null>(null);
const commitWorktreeReq = useReq(
  (worktreeId: string, commitMessage: string) =>
    Apis.worktree.commit({
      pathParams: { id: worktreeId },
      data: { message: commitMessage },
    }),
);

const branchMergeReq = useReq(
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

const branchListReq = useReq(
  (projectId: string, force = false) =>
    Apis.branch.list({
      pathParams: { projectId },
      ...(force ? { params: { force: true } } : {}),
    } as any),
);

const branchList = computed<BranchListResult>(() => {
  const payload = extractItem(branchListReq.data.value) as BranchListResult | undefined;
  return payload ?? { local: [], remote: [] };
});

const localBranchOptions = computed(() =>
  branchList.value.local.map(branch => ({
    label: branch.name,
    value: branch.name,
  })),
);

const mergeTargetOptions = computed(() => {
  const existing = new Set(projectStore.worktrees.map(worktree => worktree.branchName));
  return localBranchOptions.value.filter(option => existing.has(option.value));
});

watch(
  () => projectStore.currentProject?.id,
  id => {
    if (id) {
      branchListReq.send(id);
    } else {
      branchListReq.data.value = undefined as any;
    }
  },
  { immediate: true },
);

watch(gitFeaturesAvailable, enabled => {
  if (!enabled) {
    showCreateDialog.value = false;
  }
});

const hasWorktreeForBranch = (branchName: string) =>
  projectStore.worktrees.some(worktree => worktree.branchName === branchName);

const resolveSyncBranchDefault = () => {
  if (defaultBranch.value) {
    return defaultBranch.value;
  }
  return localBranchOptions.value[0]?.value ?? '';
};

const resolveMergeTargetDefault = () => {
  if (defaultBranch.value && hasWorktreeForBranch(defaultBranch.value)) {
    return defaultBranch.value;
  }
  return mergeTargetOptions.value[0]?.value ?? '';
};

async function handleRefreshAll() {
  if (!projectStore.currentProject) {
    return;
  }
  if (!gitFeaturesAvailable.value) {
    message.warning('当前项目不是 Git 仓库，无法刷新 Worktree 状态');
    return;
  }
  try {
    // 先从 git 仓库同步 worktree 到数据库（会识别外部创建的新 worktree）
    await projectStore.syncWorktrees(projectStore.currentProject.id);
    // 然后刷新所有 worktree 的 git 状态
    await refreshAllWorktreesReq.send(projectStore.currentProject.id);
    // 最后重新获取列表以确保 UI 显示最新的状态
    await projectStore.fetchWorktrees(projectStore.currentProject.id);
    await branchListReq.forceReload(projectStore.currentProject.id, true);
    message.success('已刷新所有 Worktree 状态');
  } catch (error: any) {
    message.error(error?.message ?? '刷新失败', { duration: 0, closable: true, keepAliveOnHover: true });
  }
}

async function handleRefresh(id: string) {
  try {
    const result = await refreshWorktreeStatusReq.send(id);
    const updated = extractItem(result);
    if (updated) {
      projectStore.updateWorktreeInList(id, updated);
    }
    message.success('状态已刷新');
  } catch (error: any) {
    message.error(error?.message ?? '刷新失败', { duration: 0, closable: true, keepAliveOnHover: true });
  }
}


function confirmDelete(worktree: Worktree) {
  dialog.warning({
    title: '确认删除',
    content: `确认要删除 worktree "${worktree.branchName}" 吗？`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      await performDeleteWorktree(worktree);
    },
  });
}

type DeleteWorktreeOptions = {
  force?: boolean;
  deleteBranch?: boolean;
};

async function performDeleteWorktree(
  worktree: Worktree,
  options: DeleteWorktreeOptions = {},
): Promise<void> {
  const force = options.force ?? false;
  const deleteBranch = options.deleteBranch ?? true;
  deletingWorktreeId.value = worktree.id;
  try {
    await projectStore.deleteWorktree(worktree.id, force, deleteBranch);
    message.success('Worktree 已删除');
  } catch (error: any) {
    const errorMessage = extractErrorMessage(error);
    if (!force && shouldOfferForceDeletion(errorMessage)) {
      deletingWorktreeId.value = null;
      dialog.warning({
        title: '强制删除 Worktree？',
        content: '检测到该 worktree 存在未提交或未跟踪的文件，强制删除将丢弃所有更改，是否继续？',
        positiveText: '强制删除',
        negativeText: '取消',
        onPositiveClick: async () => {
          await performDeleteWorktree(worktree, { ...options, force: true, deleteBranch });
        },
      });
      return;
    }
    message.error(errorMessage || '删除失败', { duration: 0, closable: true, keepAliveOnHover: true });
  } finally {
    deletingWorktreeId.value = null;
  }
}

function extractErrorMessage(error: any): string {
  if (!error) {
    return '';
  }
  if (typeof error === 'string') {
    return error;
  }
  if (typeof error.message === 'string') {
    return error.message;
  }
  return '';
}

function shouldOfferForceDeletion(message: string): boolean {
  if (!message) {
    return false;
  }
  const normalized = message.toLowerCase();
  return (
    message.includes('目录不为空') ||
    normalized.includes('--force') ||
    normalized.includes('not empty') ||
    normalized.includes('untracked') ||
    normalized.includes('modified')
  );
}

async function handleOpenExplorer(path: string) {
  try {
    await projectStore.openInExplorer(path);
  } catch (error: any) {
    message.error(error?.message ?? '打开文件管理器失败', { duration: 0, closable: true, keepAliveOnHover: true });
  }
}

function handleOpenTerminal(worktree: Worktree) {
  emit('open-terminal', worktree);
}

async function handleOpenEditor(payload: { worktree: Worktree; editor: EditorPreference }) {
  const { worktree, editor } = payload;
  if (!worktree) {
    return;
  }
  if (editor === 'custom' && !customEditorCommand.value) {
    message.warning('请先在设置中配置自定义命令');
    return;
  }
  try {
    await projectStore.openInEditor(
      worktree.path,
      editor,
      editor === 'custom' ? customEditorCommand.value : undefined,
    );
    const label = EDITOR_LABEL_MAP[editor] ?? '编辑器';
    message.success(`已在 ${label} 中打开`);
  } catch (error: any) {
    message.error(error?.message ?? '打开编辑器失败', { duration: 0, closable: true, keepAliveOnHover: true });
  }
}

function handleWorktreeCreated(worktree?: Worktree) {
  // createWorktree 已经在 store 中完成了刷新，这里只需显示成功消息
  if (worktree) {
    message.success(`Worktree ${worktree.branchName} 创建成功`);
  }
}

function handleSelectWorktree(worktreeId: string) {
  if (projectStore.selectedWorktreeId === worktreeId) {
    projectStore.setSelectedWorktree(null);
  } else {
    projectStore.setSelectedWorktree(worktreeId);
  }
}

function goToBranchManagement() {
  if (!projectStore.currentProject) {
    message.warning('请先选择项目');
    return;
  }
  router.push({ name: 'project-branches', params: { id: projectStore.currentProject.id } });
}

function openSyncDialog(worktree: Worktree) {
  const initial = resolveSyncBranchDefault();
  if (!initial) {
    message.error('暂无可用的上游分支，请先在分支管理中创建', { duration: 0, closable: true, keepAliveOnHover: true });
    return;
  }
  branchOperation.visible = true;
  branchOperation.type = 'rebase';
  branchOperation.worktree = worktree;
  branchOperation.targetBranch = initial;
  branchOperation.strategy = 'rebase';
}

function openMergeDialog(payload: { worktree: Worktree; strategy?: 'merge' | 'squash' }) {
  const initial = resolveMergeTargetDefault();
  if (!initial) {
    message.error('没有可用的目标 Worktree，请先创建对应分支的 Worktree', { duration: 0, closable: true, keepAliveOnHover: true });
    return;
  }
  branchOperation.visible = true;
  branchOperation.type = 'merge';
  branchOperation.worktree = payload.worktree;
  branchOperation.targetBranch = initial;
  branchOperation.strategy = payload.strategy ?? 'squash';
}

function closeBranchOperation() {
  branchOperation.visible = false;
  branchOperation.type = null;
  branchOperation.worktree = null;
  branchOperation.targetBranch = '';
  branchOperation.strategy = 'merge';
  branchOperation.commitImmediately = true;
  branchOperation.commitMessage = '';
}

const branchOperationTitle = computed(() => {
  if (branchOperation.type === 'rebase') {
    return 'Rebase';
  }
  if (branchOperation.type === 'merge') {
    return '合并至';
  }
  return '分支操作';
});

const branchOperationDescription = computed(() => {
  if (!branchOperation.worktree || !branchOperation.type) {
    return '';
  }
  if (branchOperation.type === 'rebase') {
    return `将在 Worktree「${branchOperation.worktree.branchName}」中执行 rebase，来源分支：${
      branchOperation.targetBranch || defaultBranch.value || ''
    }`;
  }
  const strategyLabel = branchOperation.strategy === 'squash' ? 'squash' : 'merge';
  const target = branchOperation.targetBranch || defaultBranch.value || '';
  return `将以 ${strategyLabel} 方式把「${branchOperation.worktree.branchName}」合并到分支 ${target}。`;
});

const branchOperationOptions = computed(() => {
  if (branchOperation.type === 'rebase') {
    return localBranchOptions.value;
  }
  if (branchOperation.type === 'merge') {
    return mergeTargetOptions.value;
  }
  return [];
});

watch(
  () => branchOperationOptions.value,
  options => {
    if (!branchOperation.visible) {
      return;
    }
    if (!options.length) {
      branchOperation.targetBranch = '';
      return;
    }
    if (!options.some(option => option.value === branchOperation.targetBranch)) {
      branchOperation.targetBranch = options[0].value;
    }
  },
  { deep: true },
);

watch(
  () => branchOperation.type,
  type => {
    if (!type) {
      branchOperation.targetBranch = '';
      branchOperation.strategy = 'merge';
      return;
    }
    if (type === 'rebase') {
      branchOperation.strategy = 'rebase';
      branchOperation.targetBranch = resolveSyncBranchDefault();
    } else if (type === 'merge') {
      if (branchOperation.strategy === 'rebase') {
        branchOperation.strategy = 'merge';
      }
      branchOperation.targetBranch = resolveMergeTargetDefault();
    }
  },
);

watch(
  () => branchOperation.strategy,
  strategy => {
    if (strategy !== 'squash') {
      branchOperation.commitMessage = '';
      branchOperation.commitImmediately = true;
    }
  },
);

function openCommitDialog(worktree: Worktree) {
  commitDialog.visible = true;
  commitDialog.worktree = worktree;
  commitDialog.message = '';
}

function closeCommitDialog() {
  commitDialog.visible = false;
  commitDialog.worktree = null;
  commitDialog.message = '';
}

async function submitCommit() {
  if (!commitDialog.worktree) {
    return;
  }
  const trimmed = commitDialog.message.trim();
  if (!trimmed) {
    message.warning('请输入提交信息');
    return;
  }
  try {
    await commitWorktreeReq.send(commitDialog.worktree.id, trimmed);
    const result = await refreshWorktreeStatusReq.send(commitDialog.worktree.id);
    const updated = extractItem(result);
    if (updated) {
      projectStore.updateWorktreeInList(commitDialog.worktree.id, updated);
    }
    message.success('提交成功');
    closeCommitDialog();
  } catch (error: any) {
    message.error(error?.message ?? '提交失败', { duration: 0, closable: true, keepAliveOnHover: true });
  }
}

async function confirmBranchOperation() {
  if (!branchOperation.worktree || !branchOperation.type || !branchOperation.targetBranch) {
    message.warning('请选择目标分支');
    return;
  }
  if (shouldCommitAfterSquash.value && !branchOperation.commitMessage.trim()) {
    message.warning('请输入提交信息');
    return;
  }
  branchOperationLoading.value = true;
  try {
    if (branchOperation.type === 'rebase') {
      await performMerge(branchOperation.worktree.id, {
        targetBranch: branchOperation.worktree.branchName,
        sourceBranch: branchOperation.targetBranch,
        strategy: 'rebase',
      });
      const result = await refreshWorktreeStatusReq.send(branchOperation.worktree.id);
      const updated = extractItem(result);
      if (updated) {
        projectStore.updateWorktreeInList(branchOperation.worktree.id, updated);
      }
    } else {
      const targetWorktree = projectStore.worktrees.find(
        worktree => worktree.branchName === branchOperation.targetBranch,
      );
      if (!targetWorktree) {
        throw new Error('未找到目标分支对应的 Worktree');
      }
      const strategy = branchOperation.strategy === 'squash' ? 'squash' : 'merge';
      const shouldCommit = strategy === 'squash' && branchOperation.commitImmediately;
      const commitMsg = shouldCommit ? branchOperation.commitMessage.trim() : '';
      await performMerge(targetWorktree.id, {
        targetBranch: targetWorktree.branchName,
        sourceBranch: branchOperation.worktree.branchName,
        strategy,
        commit: shouldCommit,
        commitMessage: commitMsg,
      });
      const refreshIds = Array.from(
        new Set([targetWorktree.id, branchOperation.worktree.id]),
      );
      await Promise.all(refreshIds.map(async (id) => {
        const result = await refreshWorktreeStatusReq.send(id);
        const updated = extractItem(result);
        if (updated) {
          projectStore.updateWorktreeInList(id, updated);
        }
      }));
      if (shouldCommit) {
        branchOperation.commitMessage = '';
      }
    }
    closeBranchOperation();
  } catch (error: any) {
    message.error(error?.message ?? '操作失败', { duration: 0, closable: true, keepAliveOnHover: true });
  } finally {
    branchOperationLoading.value = false;
  }
}

async function performMerge(
  worktreeId: string,
  payload: {
    targetBranch: string;
    sourceBranch: string;
    strategy: 'merge' | 'rebase' | 'squash';
    commit?: boolean;
    commitMessage?: string;
  },
) {
  const response = await branchMergeReq.send(worktreeId, {
    targetBranch: payload.targetBranch,
    sourceBranch: payload.sourceBranch,
    strategy: payload.strategy,
    commit: payload.commit ?? false,
    commitMessage: payload.commitMessage ?? '',
  });
  const result = extractItem(response) as MergeResult | undefined;
  if (!result) {
    message.success('操作成功');
    return;
  }
  if (result.success) {
    message.success(result.message || '操作成功');
  } else {
    const conflicts = result.conflicts?.length ? `冲突文件：${result.conflicts.join(', ')}` : '';
    message.warning(result.message || '存在冲突，请手动处理', { duration: 0, closable: true, keepAliveOnHover: true });
    if (conflicts) {
      message.info(conflicts, { duration: 0, closable: true, keepAliveOnHover: true });
    }
  }
}
</script>

<style scoped>
.worktree-list {
  height: 100%;
  display: flex;
  flex-direction: column;
}

.list-header {
  padding: 16px;
  border-bottom: 1px solid var(--n-border-color);
}

.worktree-items {
  padding: 8px;
}

h3 {
  margin: 0;
}

.git-warning {
  margin: 8px 16px;
}

.worktree-empty {
  margin: 16px;
}
</style>
