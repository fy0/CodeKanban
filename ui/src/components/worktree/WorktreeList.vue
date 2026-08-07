<template>
  <div class="worktree-list">
    <div class="list-header">
      <n-space justify="space-between" align="center">
        <h3>{{ t('worktree.title') }}</h3>
        <n-space align="center">
          <n-button
            text
            size="small"
            :disabled="!projectStore.currentProject || !canReadBranches"
            @click="goToBranchManagement"
          >
            <template #icon>
              <n-icon><GitBranchOutline /></n-icon>
            </template>
            {{ t('worktree.branches') }}
          </n-button>
          <n-button
            text
            @click="handleRefreshAll"
            :loading="refreshAllWorktreesReq.loading.value"
            :disabled="!projectStore.currentProject || !canReadStatus"
          >
            <template #icon>
              <n-icon><RefreshOutline /></n-icon>
            </template>
          </n-button>
          <n-button
            type="primary"
            size="small"
            @click="showCreateDialog = true"
            :disabled="!projectStore.currentProject || !canWriteWorktrees"
          >
            <template #icon>
              <n-icon><AddOutline /></n-icon>
            </template>
            {{ t('worktree.new') }}
          </n-button>
        </n-space>
      </n-space>
    </div>

    <n-alert v-if="showGitWarning" type="warning" class="git-warning" :show-icon="false">
      <n-space align="center" size="small">
        <span>{{ t('worktree.notGitRepoShort') }}</span>
        <n-popover trigger="hover" placement="bottom-start">
          <template #trigger>
            <n-button text circle size="tiny" class="git-warning__details-btn">
              <n-icon :size="16"><InformationCircleOutline /></n-icon>
            </n-button>
          </template>
          <span>{{ t('worktree.notGitRepoDetails') }}</span>
        </n-popover>
      </n-space>
    </n-alert>

    <n-scrollbar class="worktree-scrollbar">
      <div v-if="projectStore.worktrees.length" class="worktree-items">
        <WorktreeCard
          v-for="worktree in projectStore.worktrees"
          :key="worktree.id"
          :worktree="worktree"
          :selected="projectStore.selectedWorktreeId === worktree.id"
          :can-refresh="canRefreshWorktree(worktree)"
          :can-merge="canMergeWorktree(worktree)"
          :can-commit="canCommitWorktree(worktree)"
          :refresh-disabled-reason="getRefreshDisabledReason(worktree)"
          :merge-disabled-reason="getMergeDisabledReason(worktree)"
          :commit-disabled-reason="getCommitDisabledReason(worktree)"
          :is-deleting="deletingWorktreeId === worktree.id"
          :default-editor="defaultEditorPreference"
          :editor-options="worktreeEditorOptions"
          @select="handleSelectWorktree"
          @refresh="handleRefresh"
          @delete="confirmDelete"
          @open-explorer="handleOpenExplorer"
          @open-terminal="handleOpenTerminal"
          @open-editor="handleOpenEditor"
          @merge-to-default="openMergeDialog"
          @commit-worktree="openCommitDialog"
        />
      </div>
      <n-empty v-else :description="t('worktree.noWorktrees')" class="worktree-empty" />
    </n-scrollbar>

    <WorktreeCreateDialog
      v-if="projectStore.currentProject"
      v-model:show="showCreateDialog"
      @success="handleWorktreeCreated"
    />

    <n-modal
      v-model:show="branchOperation.visible"
      preset="dialog"
      :title="t('worktree.mergeTo')"
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
            :placeholder="t('worktree.selectBranch')"
          />
          <n-alert v-else type="warning" show-icon>
            {{ t('worktree.noTargetWorktree') }}
          </n-alert>
        </div>
      </n-space>
      <template #action>
        <n-button @click="closeBranchOperation" :disabled="branchOperationLoading">{{
          t('common.cancel')
        }}</n-button>
        <n-button
          type="primary"
          :loading="branchOperationLoading"
          :disabled="!branchOperationOptions.length || !canExecuteBranchOperation"
          @click="confirmBranchOperation"
        >
          {{ t('branch.executeMerge') }}
        </n-button>
      </template>
    </n-modal>

    <n-modal
      v-model:show="commitDialog.visible"
      preset="dialog"
      :title="t('worktree.commitChanges')"
      :mask-closable="false"
    >
      <n-space vertical size="large">
        <n-text>
          {{ t('worktree.commitDescription', { branch: commitDialog.worktree?.branchName ?? '' }) }}
        </n-text>
        <n-input
          v-model:value="commitDialog.message"
          type="textarea"
          :placeholder="t('worktree.commitMessagePlaceholder')"
          :autosize="{ minRows: 3, maxRows: 6 }"
        />
      </n-space>
      <template #action>
        <n-button @click="closeCommitDialog" :disabled="commitWorktreeReq.loading.value">{{
          t('common.cancel')
        }}</n-button>
        <n-button
          type="primary"
          :loading="commitWorktreeReq.loading.value"
          :disabled="!commitDialog.message.trim()"
          @click="submitCommit"
        >
          {{ t('common.submit') }}
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
import {
  AddOutline,
  GitBranchOutline,
  InformationCircleOutline,
  RefreshOutline,
} from '@vicons/ionicons5';
import WorktreeCard from './WorktreeCard.vue';
import WorktreeCreateDialog from './WorktreeCreateDialog.vue';
import { useProjectStore } from '@/stores/project';
import { useSettingsStore } from '@/stores/settings';
import { useLocale } from '@/composables/useLocale';
import Apis from '@/api';
import { useReq } from '@/api/composable';
import { extractItem } from '@/api/response';
import type { MergeResult, Worktree } from '@/types/models';
import type { EditorPreference } from '@/stores/settings';
import { DEFAULT_EDITOR, EDITOR_OPTIONS, EDITOR_LABEL_MAP } from '@/constants/editor';
import { gitOperationAvailable } from '@/utils/projectGitCapability';

const emit = defineEmits<{
  'open-terminal': [payload: Worktree];
}>();

const projectStore = useProjectStore();
const router = useRouter();
const message = useMessage();
const dialog = useDialog();
const settingsStore = useSettingsStore();
const { editorSettings } = storeToRefs(settingsStore);
const { t } = useLocale();

const defaultEditorPreference = computed<EditorPreference>(
  () => editorSettings.value.defaultEditor ?? DEFAULT_EDITOR
);
const customEditorCommand = computed(() => editorSettings.value.customCommand?.trim() ?? '');
const worktreeEditorOptions = computed(() =>
  EDITOR_OPTIONS.map(option => ({
    ...option,
    disabled: option.value === 'custom' && !customEditorCommand.value,
  }))
);

const showCreateDialog = ref(false);

// 刷新所有 worktree 状态
const refreshAllWorktreesReq = useReq((projectId: string) =>
  Apis.worktree.refreshAllByProject({
    pathParams: { projectId },
  })
);

// 刷新单个 worktree 状态
const refreshWorktreeStatusReq = useReq((worktreeId: string) =>
  Apis.worktree.refreshStatus({
    pathParams: { id: worktreeId },
  })
);

const defaultBranch = computed(() => projectStore.currentProject?.defaultBranch ?? '');
const mainWorktree = computed(
  () => projectStore.worktrees.find(worktree => worktree.isMain) ?? null
);
const gitFeaturesAvailable = computed(() => Boolean(projectStore.gitCapabilities?.repository));
const canReadBranches = computed(() =>
  gitOperationAvailable(projectStore.gitCapabilities, 'branchesRead')
);
const canReadStatus = computed(() => gitOperationAvailable(projectStore.gitCapabilities, 'status'));
const canWriteWorktrees = computed(() =>
  gitOperationAvailable(projectStore.gitCapabilities, 'worktreesWrite')
);
const showGitWarning = computed(
  () => Boolean(projectStore.currentProject) && !projectStore.loading && !gitFeaturesAvailable.value
);

// 判断worktree是否有未提交的更改
function hasTrackedChanges(worktree: Worktree): boolean {
  return (worktree.statusModified ?? 0) > 0 || (worktree.statusStaged ?? 0) > 0;
}

function hasPendingChanges(worktree: Worktree): boolean {
  return hasTrackedChanges(worktree) || (worktree.statusUntracked ?? 0) > 0;
}

function canRefreshWorktree(worktree: Worktree): boolean {
  return gitOperationAvailable(projectStore.gitCapabilities, 'status', worktree.id);
}

// 判断worktree是否可以进行merge操作
function canMergeWorktree(worktree: Worktree): boolean {
  const target = mainWorktree.value;
  return (
    Boolean(target) &&
    gitOperationAvailable(projectStore.gitCapabilities, 'status', worktree.id) &&
    gitOperationAvailable(projectStore.gitCapabilities, 'merge', target?.id) &&
    !hasPendingChanges(worktree) &&
    worktree.branchName !== defaultBranch.value
  );
}

// 判断worktree是否可以进行commit操作
function canCommitWorktree(worktree: Worktree): boolean {
  return (
    gitOperationAvailable(projectStore.gitCapabilities, 'commit', worktree.id) &&
    hasPendingChanges(worktree)
  );
}

function getRefreshDisabledReason(worktree: Worktree): string {
  if (canRefreshWorktree(worktree)) {
    return '';
  }
  return t('worktree.refreshDisabledGit');
}

function getMergeDisabledReason(worktree: Worktree): string {
  if (canMergeWorktree(worktree)) {
    return '';
  }
  if (!gitOperationAvailable(projectStore.gitCapabilities, 'merge', mainWorktree.value?.id)) {
    return t('worktree.mergeDisabledGit');
  }
  if (!mainWorktree.value) {
    return t('worktree.mergeDisabledNoMainWorktree');
  }
  if (worktree.branchName === defaultBranch.value) {
    return t('worktree.mergeDisabledOnDefault');
  }
  if (hasPendingChanges(worktree)) {
    return t('worktree.mergeDisabledDirty');
  }
  return t('worktree.actionDisabledGeneric');
}

function getCommitDisabledReason(worktree: Worktree): string {
  if (canCommitWorktree(worktree)) {
    return '';
  }
  if (!gitOperationAvailable(projectStore.gitCapabilities, 'commit', worktree.id)) {
    return t('worktree.commitDisabledGit');
  }
  if (!hasPendingChanges(worktree)) {
    return t('worktree.commitDisabledClean');
  }
  return t('worktree.actionDisabledGeneric');
}

const branchOperation = reactive({
  visible: false,
  worktree: null as Worktree | null,
  targetBranch: '',
});
const branchOperationLoading = ref(false);
const commitDialog = reactive({
  visible: false,
  worktree: null as Worktree | null,
  message: '',
});
const deletingWorktreeId = ref<string | null>(null);
const commitWorktreeReq = useReq((worktreeId: string, commitMessage: string) =>
  Apis.worktree.commit({
    pathParams: { id: worktreeId },
    data: { message: commitMessage },
  })
);

const branchMergeReq = useReq(
  (
    worktreeId: string,
    payload: {
      targetBranch: string;
      sourceBranch: string;
      strategy: 'merge';
      commit: boolean;
      commitMessage: string;
    }
  ) =>
    Apis.branch.merge({
      pathParams: { id: worktreeId },
      data: payload,
    })
);

const mergeTargetOptions = computed(() =>
  projectStore.worktrees
    .filter(worktree => Boolean(worktree.branchName))
    .map(worktree => ({
      label: worktree.branchName,
      value: worktree.branchName,
    }))
);

const canExecuteBranchOperation = computed(() => {
  const source = branchOperation.worktree;
  const target = projectStore.worktrees.find(
    worktree => worktree.branchName === branchOperation.targetBranch
  );
  return Boolean(
    source &&
      target &&
      source.id !== target.id &&
      !hasPendingChanges(source) &&
      !hasPendingChanges(target) &&
      gitOperationAvailable(projectStore.gitCapabilities, 'merge', target.id)
  );
});

watch(gitFeaturesAvailable, enabled => {
  if (!enabled) {
    showCreateDialog.value = false;
  }
});

const hasWorktreeForBranch = (branchName: string) =>
  projectStore.worktrees.some(worktree => worktree.branchName === branchName);

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
  if (!canReadStatus.value) {
    message.warning(t('worktree.notGitRepoCannotRefresh'));
    return;
  }
  try {
    // 先从 git 仓库同步 worktree 到数据库（会识别外部创建的新 worktree）
    await projectStore.syncWorktrees(projectStore.currentProject.id);
    // 然后刷新所有 worktree 的 git 状态
    await refreshAllWorktreesReq.send(projectStore.currentProject.id);
    // 最后重新获取列表以确保 UI 显示最新的状态
    await projectStore.fetchWorktrees(projectStore.currentProject.id);
    message.success(t('worktree.allWorktreesRefreshed'));
  } catch (error: any) {
    message.error(error?.message ?? t('branch.refreshFailed'), {
      duration: 0,
      closable: true,
      keepAliveOnHover: true,
    });
  }
}

async function handleRefresh(id: string) {
  try {
    const result = await refreshWorktreeStatusReq.send(id);
    const updated = extractItem(result);
    if (updated) {
      projectStore.updateWorktreeInList(id, updated);
    }
    message.success(t('worktree.statusRefreshed'));
  } catch (error: any) {
    message.error(error?.message ?? t('branch.refreshFailed'), {
      duration: 0,
      closable: true,
      keepAliveOnHover: true,
    });
  }
}

function confirmDelete(worktree: Worktree) {
  dialog.warning({
    title: t('worktree.confirmDeleteTitle'),
    content: t('worktree.confirmDeleteContent', { name: worktree.branchName }),
    positiveText: t('common.delete'),
    negativeText: t('common.cancel'),
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
  options: DeleteWorktreeOptions = {}
): Promise<void> {
  const force = options.force ?? false;
  const deleteBranch = options.deleteBranch ?? true;
  deletingWorktreeId.value = worktree.id;
  try {
    await projectStore.deleteWorktree(worktree.id, force, deleteBranch);
    message.success(t('worktree.worktreeDeleted'));
  } catch (error: any) {
    const errorMessage = extractErrorMessage(error);
    if (!force && shouldOfferForceDeletion(errorMessage)) {
      deletingWorktreeId.value = null;
      dialog.warning({
        title: t('worktree.forceDeleteTitle'),
        content: t('worktree.forceDeleteContent'),
        positiveText: t('worktree.forceDelete'),
        negativeText: t('common.cancel'),
        onPositiveClick: async () => {
          await performDeleteWorktree(worktree, { ...options, force: true, deleteBranch });
        },
      });
      return;
    }
    message.error(errorMessage || t('worktree.deleteFailed'), {
      duration: 0,
      closable: true,
      keepAliveOnHover: true,
    });
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
    message.includes(t('worktree.directoryNotEmpty')) ||
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
    message.error(error?.message ?? t('worktree.openExplorerFailed'), {
      duration: 0,
      closable: true,
      keepAliveOnHover: true,
    });
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
    message.warning(t('worktree.configureCustomCommandFirst'));
    return;
  }
  try {
    await projectStore.openInEditor(
      worktree.path,
      editor,
      editor === 'custom' ? customEditorCommand.value : undefined
    );
    const label = EDITOR_LABEL_MAP[editor] ?? t('worktree.editor');
    message.success(t('worktree.openedInEditor', { editor: label }));
  } catch (error: any) {
    message.error(error?.message ?? t('worktree.openEditorFailed'), {
      duration: 0,
      closable: true,
      keepAliveOnHover: true,
    });
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
    message.warning(t('project.selectProjectFirst'));
    return;
  }
  if (!canReadBranches.value) {
    message.warning(t('worktree.notGitRepoShort'));
    return;
  }
  router.push({ name: 'project-branches', params: { id: projectStore.currentProject.id } });
}

function openMergeDialog(payload: { worktree: Worktree }) {
  const initial = resolveMergeTargetDefault();
  if (!initial) {
    message.error(t('worktree.noTargetWorktree'), {
      duration: 0,
      closable: true,
      keepAliveOnHover: true,
    });
    return;
  }
  branchOperation.visible = true;
  branchOperation.worktree = payload.worktree;
  branchOperation.targetBranch = initial;
}

function closeBranchOperation() {
  branchOperation.visible = false;
  branchOperation.worktree = null;
  branchOperation.targetBranch = '';
}

const branchOperationDescription = computed(() => {
  if (!branchOperation.worktree) {
    return '';
  }
  const target = branchOperation.targetBranch || defaultBranch.value || '';
  return t('worktree.mergeDescription', {
    strategy: 'merge',
    source: branchOperation.worktree.branchName,
    target,
  });
});

const branchOperationOptions = mergeTargetOptions;

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
  { deep: true }
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
    message.warning(t('worktree.commitMessagePlaceholder'));
    return;
  }
  try {
    await commitWorktreeReq.send(commitDialog.worktree.id, trimmed);
    const result = await refreshWorktreeStatusReq.send(commitDialog.worktree.id);
    const updated = extractItem(result);
    if (updated) {
      projectStore.updateWorktreeInList(commitDialog.worktree.id, updated);
    }
    message.success(t('worktree.commitSuccess'));
    closeCommitDialog();
  } catch (error: any) {
    message.error(error?.message ?? t('worktree.commitFailed'), {
      duration: 0,
      closable: true,
      keepAliveOnHover: true,
    });
  }
}

async function confirmBranchOperation() {
  if (!branchOperation.worktree || !branchOperation.targetBranch) {
    message.warning(t('worktree.selectTargetBranch'));
    return;
  }
  if (!canExecuteBranchOperation.value) {
    message.warning(t('worktree.mergeDisabledDirty'));
    return;
  }
  branchOperationLoading.value = true;
  try {
    const targetWorktree = projectStore.worktrees.find(
      worktree => worktree.branchName === branchOperation.targetBranch
    );
    if (!targetWorktree) {
      throw new Error(t('worktree.targetWorktreeNotFound'));
    }
    await performMerge(targetWorktree.id, {
      targetBranch: targetWorktree.branchName,
      sourceBranch: branchOperation.worktree.branchName,
      strategy: 'merge',
      commit: false,
      commitMessage: '',
    });
    const refreshIds = Array.from(new Set([targetWorktree.id, branchOperation.worktree.id]));
    await Promise.all(
      refreshIds.map(async id => {
        const result = await refreshWorktreeStatusReq.send(id);
        const updated = extractItem(result);
        if (updated) {
          projectStore.updateWorktreeInList(id, updated);
        }
      })
    );
    closeBranchOperation();
  } catch (error: any) {
    message.error(error?.message ?? t('common.error'), {
      duration: 0,
      closable: true,
      keepAliveOnHover: true,
    });
  } finally {
    branchOperationLoading.value = false;
  }
}

async function performMerge(
  worktreeId: string,
  payload: {
    targetBranch: string;
    sourceBranch: string;
    strategy: 'merge';
    commit: boolean;
    commitMessage: string;
  }
) {
  const response = await branchMergeReq.send(worktreeId, {
    targetBranch: payload.targetBranch,
    sourceBranch: payload.sourceBranch,
    strategy: payload.strategy,
    commit: payload.commit,
    commitMessage: payload.commitMessage,
  });
  const result = extractItem(response) as MergeResult | undefined;
  if (!result) {
    message.success(t('common.success'));
    return;
  }
  if (result.success) {
    message.success(result.message || t('common.success'));
  } else {
    const conflicts = result.conflicts?.length
      ? t('worktree.conflictFiles', { files: result.conflicts.join(', ') })
      : '';
    message.warning(result.message || t('worktree.hasConflictsManual'), {
      duration: 0,
      closable: true,
      keepAliveOnHover: true,
    });
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
  min-height: 0;
}

.list-header {
  padding: 16px;
  border-bottom: 1px solid var(--n-border-color);
}

.worktree-scrollbar {
  flex: 1;
  min-height: 0;
}

.worktree-items {
  padding: 8px;
}

h3 {
  margin: 0;
}

.git-warning {
  margin: 8px;
}

.worktree-empty {
  margin: 16px;
}
</style>
