<template>
  <div class="branch-page">
    <n-page-header>
      <template #title>
        <n-space align="center" size="small">
          <n-button
            quaternary
            size="small"
            @click="goBackToWorkspace"
            :disabled="!currentProjectId"
          >
            <template #icon>
              <n-icon><ChevronBackOutline /></n-icon>
            </template>
            {{ t('branch.backToProject') }}
          </n-button>
          <span>{{ pageHeading }}</span>
        </n-space>
      </template>
      <template #extra>
        <n-space align="center">
          <LanguageSwitcher />
          <n-button
            quaternary
            :disabled="!currentProjectId || !canReadBranches"
            :loading="projectStore.loading"
            @click="reloadBranches(true)"
          >
            <template #icon>
              <n-icon><RefreshOutline /></n-icon>
            </template>
            {{ t('branch.refresh') }}
          </n-button>
          <n-button
            type="primary"
            :disabled="!currentProjectId || !canWriteBranches"
            @click="openCreateModal()"
          >
            <template #icon>
              <n-icon><AddOutline /></n-icon>
            </template>
            {{ t('branch.newBranch') }}
          </n-button>
        </n-space>
      </template>
    </n-page-header>

    <n-space v-if="gitFeaturesAvailable || !currentProjectId" class="branch-toolbar" align="center">
      <n-input
        ref="searchInputRef"
        v-model:value="searchInput"
        round
        clearable
        size="large"
        :placeholder="t('branch.searchBranch')"
      >
        <template #prefix>
          <n-icon><SearchOutline /></n-icon>
        </template>
      </n-input>
      <n-statistic :label="t('branch.localBranches')">{{ branchList.local.length }}</n-statistic>
      <n-statistic :label="t('branch.remoteBranches')">{{ branchList.remote.length }}</n-statistic>
      <n-statistic :label="t('branch.worktreeBound')">{{ worktreeBoundCount }}</n-statistic>
    </n-space>

    <n-alert
      v-if="branchError"
      type="error"
      class="branch-alert"
      closable
      @close="branchError = null"
    >
      {{ branchError }}
    </n-alert>

    <n-alert v-if="showGitWarning" type="warning" class="branch-alert" :show-icon="false">
      <n-space align="center" size="small">
        <span>{{ t('branch.notGitRepoShort') }}</span>
        <n-popover trigger="hover" placement="bottom-start">
          <template #trigger>
            <n-button text circle size="tiny">
              <n-icon><InformationCircleOutline /></n-icon>
            </n-button>
          </template>
          <span>{{ t('branch.notGitRepoDetails') }}</span>
        </n-popover>
      </n-space>
    </n-alert>

    <n-spin :show="branchLoading">
      <n-grid v-if="gitFeaturesAvailable || !currentProjectId" cols="24" x-gap="16" y-gap="16">
        <n-gi :span="24" :lg="12">
          <n-card :title="t('branch.localBranches')">
            <template #header-extra>
              <n-text depth="3">{{
                t('branch.totalCount', { count: filteredLocalBranches.length })
              }}</n-text>
            </template>
            <template v-if="filteredLocalBranches.length === 0">
              <n-empty :description="t('branch.noLocalBranches')" />
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
          <n-card :title="t('branch.remoteBranches')">
            <template #header-extra>
              <n-text depth="3">{{
                t('branch.totalCount', { count: filteredRemoteBranches.length })
              }}</n-text>
            </template>
            <template v-if="filteredRemoteBranches.length === 0">
              <n-empty :description="t('branch.noRemoteBranches')" />
            </template>
            <template v-else>
              <n-virtual-list
                v-if="useVirtualRemote"
                class="branch-list branch-list--virtual"
                :items="filteredRemoteBranches"
                :item-size="86"
              >
                <template #default="{ item }">
                  <BranchListItem :branch="item" mode="remote" @checkout="handleCheckoutRemote" />
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
          <div>
            <n-card :title="t('branch.mergeAndConflict')">
              <n-form
                ref="mergeFormRef"
                :model="mergeForm"
                :rules="mergeFormRules"
                label-placement="left"
              >
                <n-grid cols="1 640:2" x-gap="16">
                  <n-gi>
                    <n-form-item :label="t('branch.targetWorktree')" path="worktreeId">
                      <n-select
                        v-model:value="mergeForm.worktreeId"
                        :options="worktreeOptions"
                        :placeholder="t('branch.selectWorktree')"
                        :disabled="worktreeOptions.length === 0"
                      />
                    </n-form-item>
                  </n-gi>
                  <n-gi>
                    <n-form-item :label="t('branch.targetBranch')" path="targetBranch">
                      <n-select
                        v-model:value="mergeForm.targetBranch"
                        :options="localBranchOptions"
                        filterable
                        :placeholder="t('branch.selectTargetBranch')"
                      />
                    </n-form-item>
                  </n-gi>
                  <n-gi>
                    <n-form-item :label="t('branch.sourceBranch')" path="sourceBranch">
                      <n-select
                        v-model:value="mergeForm.sourceBranch"
                        :options="localBranchOptions"
                        filterable
                        :placeholder="t('branch.selectSourceBranch')"
                      />
                    </n-form-item>
                  </n-gi>
                </n-grid>

                <n-form-item :label="t('branch.strategy')">
                  <n-radio-group v-model:value="mergeForm.strategy">
                    <n-radio value="merge">{{ t('branch.merge') }}</n-radio>
                    <n-radio value="rebase" :disabled="!canUseMergeStrategy('rebase')">
                      {{ t('branch.rebase') }}
                    </n-radio>
                    <n-radio value="squash" :disabled="!canUseMergeStrategy('squash')">
                      {{ t('branch.squash') }}
                    </n-radio>
                  </n-radio-group>
                </n-form-item>
                <template v-if="showSquashCommitOptions">
                  <n-form-item :label="t('branch.commitControl')">
                    <n-space align="center">
                      <n-checkbox v-model:checked="mergeForm.commitImmediately">{{
                        t('branch.commitImmediately')
                      }}</n-checkbox>
                      <n-text depth="3">{{ t('branch.commitImmediatelyHint') }}</n-text>
                    </n-space>
                  </n-form-item>
                  <n-form-item
                    v-if="shouldCommitAfterSquash"
                    :label="t('branch.commitMessage')"
                    path="commitMessage"
                  >
                    <n-input
                      v-model:value="mergeForm.commitMessage"
                      type="textarea"
                      :autosize="{ minRows: 2, maxRows: 4 }"
                      :placeholder="t('branch.commitMessagePlaceholder')"
                    />
                  </n-form-item>
                </template>

                <n-alert
                  v-if="
                    gitOperationEngine(
                      projectStore.gitCapabilities,
                      mergeOperation,
                      mergeForm.worktreeId
                    ) === 'builtin'
                  "
                  type="info"
                  :show-icon="false"
                  class="merge-result"
                >
                  {{ t('branch.fastForwardOnly') }}
                </n-alert>

                <n-space>
                  <n-button
                    type="primary"
                    :loading="mergeBranchReq.loading.value"
                    :disabled="!canExecuteMerge"
                    @click="submitMerge"
                  >
                    {{ t('branch.executeMerge') }}
                  </n-button>
                  <n-button
                    @click="refreshMergeStatus"
                    :disabled="!mergeForm.worktreeId || !canReadStatus"
                  >
                    {{ t('branch.refreshWorktreeStatus') }}
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

    <n-modal
      v-model:show="showCreateModal"
      preset="dialog"
      :title="t('branch.createBranchDialog')"
      :mask-closable="false"
    >
      <n-form
        ref="createFormRef"
        :model="createForm"
        :rules="createFormRules"
        label-placement="top"
      >
        <n-form-item :label="t('branch.branchNameField')" path="name">
          <n-input
            v-model:value="createForm.name"
            :placeholder="t('branch.branchNameFieldPlaceholder')"
          />
        </n-form-item>
        <n-form-item :label="t('branch.baseBranchField')" path="base">
          <n-select
            v-model:value="createForm.base"
            :options="baseBranchOptions"
            filterable
            :placeholder="t('branch.baseBranchDefault')"
          />
        </n-form-item>
        <n-form-item>
          <n-checkbox v-model:checked="createForm.createWorktree">{{
            t('branch.createWorktreeWithBranch')
          }}</n-checkbox>
        </n-form-item>
      </n-form>
      <template #action>
        <n-space justify="end">
          <n-button @click="closeCreateModal">{{ t('common.cancel') }}</n-button>
          <n-button
            type="primary"
            :loading="createBranchReq.loading.value"
            @click="submitCreateBranch"
          >
            {{ t('common.create') }}
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
import {
  AddOutline,
  ChevronBackOutline,
  InformationCircleOutline,
  RefreshOutline,
  SearchOutline,
} from '@vicons/ionicons5';
import type { BranchInfo, BranchListResult, MergeResult } from '@/types/models';
import { useProjectStore } from '@/stores/project';
import { useLocale } from '@/composables/useLocale';
import LanguageSwitcher from '@/components/common/LanguageSwitcher.vue';
import { debounce } from '@/utils/debounce';
import Apis from '@/api';
import { useReq } from '@/api/composable';
import { extractItem } from '@/api/response';
import BranchListItem from '@/components/branch/BranchListItem.vue';
import { useHotkeys } from '@/composables/useHotkeys';
import { gitOperationAvailable, gitOperationEngine } from '@/utils/projectGitCapability';
import { getErrorMessage } from '@/utils/errorHandler';

const route = useRoute();
const router = useRouter();
const projectStore = useProjectStore();
const dialog = useDialog();
const message = useMessage();
const { t } = useLocale();

const currentProjectId = computed(() =>
  typeof route.params.id === 'string' ? route.params.id : ''
);
const pageHeading = computed(() =>
  projectStore.currentProject
    ? `${projectStore.currentProject.name} · ${t('branch.title')}`
    : t('branch.title')
);
const gitFeaturesAvailable = computed(() => Boolean(projectStore.gitCapabilities?.repository));
const canReadBranches = computed(() =>
  gitOperationAvailable(projectStore.gitCapabilities, 'branchesRead')
);
const canWriteBranches = computed(() =>
  gitOperationAvailable(projectStore.gitCapabilities, 'branchesWrite')
);
const canReadStatus = computed(() => gitOperationAvailable(projectStore.gitCapabilities, 'status'));
const showGitWarning = computed(
  () => Boolean(projectStore.currentProject) && !projectStore.loading && !gitFeaturesAvailable.value
);

const branchListReq = useReq(
  (projectId: string, force = false) =>
    Apis.branch.list({
      pathParams: { projectId },
      ...(force ? { params: { force: true } } : {}),
    } as Parameters<typeof Apis.branch.list>[0]),
  { cacheFor: 60000 }
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
    })
);

const deleteBranchReq = useReq((projectId: string, branchName: string, force: boolean) =>
  Apis.branch.delete({
    pathParams: { projectId, branchName },
    params: { force },
  })
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
    }
  ) =>
    Apis.branch.merge({
      pathParams: { id: worktreeId },
      data: payload,
    })
);

const refreshWorktreeStatusReq = useReq((worktreeId: string) =>
  Apis.worktree.refreshStatus({
    pathParams: { id: worktreeId },
  })
);

const searchInputRef = ref<InputInst | null>(null);
const searchInput = ref('');
const searchTerm = ref('');
const branchError = ref<string | null>(null);
const mergeResult = ref<MergeResult | null>(null);

const createFormRef = ref<FormInst | null>(null);
const createForm = reactive({
  name: '',
  base: '',
  createWorktree: false,
});
const createFormRules: FormRules = {
  name: [{ required: true, message: t('validation.branchNameRequired') }],
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
const mergeOperation = computed(() => {
  if (mergeForm.strategy === 'rebase') return 'rebase' as const;
  if (mergeForm.strategy === 'squash') return 'squash' as const;
  return 'merge' as const;
});
const showSquashCommitOptions = computed(() => mergeForm.strategy === 'squash');
const shouldCommitAfterSquash = computed(
  () => showSquashCommitOptions.value && mergeForm.commitImmediately
);
const mergeFormRules: FormRules = {
  worktreeId: [{ required: true, message: t('branch.selectWorktree') }],
  targetBranch: [{ required: true, message: t('branch.selectTargetBranch') }],
  sourceBranch: [{ required: true, message: t('branch.selectSourceBranch') }],
  commitMessage: [
    {
      trigger: ['input', 'blur'],
      validator: () => {
        if (shouldCommitAfterSquash.value && !mergeForm.commitMessage.trim()) {
          return new Error(t('branch.commitMessageRequired'));
        }
        return true;
      },
    },
  ],
};

function canUseMergeStrategy(strategy: 'merge' | 'rebase' | 'squash') {
  return gitOperationAvailable(projectStore.gitCapabilities, strategy, mergeForm.worktreeId);
}

watch(
  [() => mergeForm.worktreeId, () => projectStore.gitCapabilities],
  () => {
    if (!canUseMergeStrategy(mergeForm.strategy)) {
      mergeForm.strategy = 'merge';
    }
  },
  { deep: true }
);

watch(
  () => mergeForm.strategy,
  strategy => {
    if (strategy !== 'squash') {
      mergeForm.commitImmediately = true;
      mergeForm.commitMessage = '';
    }
  }
);

watch(
  () => route.params.id,
  id => {
    if (typeof id === 'string' && id) {
      initializeProject(id);
    }
  },
  { immediate: true }
);

watch(
  () => mergeForm.worktreeId,
  worktreeId => syncTargetBranchFromSelection(worktreeId, true)
);

watch(
  () => projectStore.worktrees.map(worktree => `${worktree.id}:${worktree.branchName}`).join(','),
  () => {
    if (mergeForm.worktreeId) {
      syncTargetBranchFromSelection(mergeForm.worktreeId, false);
    }
  }
);

async function initializeProject(id: string) {
  try {
    await projectStore.fetchProject(id);
    createForm.base = projectStore.currentProject?.defaultBranch ?? '';
    if (!canReadBranches.value) {
      branchError.value = null;
      branchListReq.data.value = undefined as never;
      return;
    }
    await reloadBranches(true);
  } catch (error) {
    branchError.value = getErrorMessage(error, t('branch.loadProjectFailed'));
  }
}

async function reloadBranches(force = false) {
  if (!currentProjectId.value || !canReadBranches.value) {
    branchError.value = null;
    branchListReq.data.value = undefined as never;
    return;
  }
  branchError.value = null;
  try {
    if (force) {
      await branchListReq.forceReload(currentProjectId.value, true);
    } else {
      await branchListReq.send(currentProjectId.value);
    }
  } catch (error) {
    branchError.value = getErrorMessage(error, t('branch.fetchBranchesFailed'));
  }
}

const branchList = computed<BranchListResult>(() => {
  const payload = extractItem(branchListReq.data.value) as BranchListResult | undefined;
  return payload ?? { local: [], remote: [] };
});

const branchLoading = computed(() => branchListReq.loading.value || projectStore.loading);

const defaultBranch = computed(() => projectStore.currentProject?.defaultBranch ?? '');
const worktreeBoundCount = computed(
  () => branchList.value.local.filter(branch => branch.hasWorktree).length
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
  }))
);

const localBranchOptions = computed(() =>
  branchList.value.local.map(branch => ({
    label: branch.name,
    value: branch.name,
  }))
);

const baseBranchOptions = computed(() => [
  ...(defaultBranch.value
    ? [
        {
          label: t('branch.defaultBranchLabel', { branch: defaultBranch.value }),
          value: defaultBranch.value,
        },
      ]
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
  if (!currentProjectId.value || !canWriteBranches.value) {
    message.warning(t('branch.notGitRepoShort'));
    return;
  }
  try {
    await createFormRef.value?.validate();
    await createBranchReq.send(currentProjectId.value, {
      name: createForm.name,
      base: createForm.base,
      createWorktree: createForm.createWorktree,
    });
    message.success(t('branch.branchCreated'));
    showCreateModal.value = false;
    await reloadBranches(true);
    if (createForm.createWorktree) {
      await projectStore.fetchWorktrees(currentProjectId.value);
    }
  } catch (error) {
    const errorMessage = getErrorMessage(error);
    if (errorMessage) {
      message.error(errorMessage);
    }
  }
}

function handleCheckoutRemote(branch: BranchInfo) {
  const simplified = branch.name.includes('/')
    ? branch.name.split('/').slice(1).join('/')
    : branch.name;
  openCreateModal(simplified, branch.name);
}

async function handleDeleteBranch(branch: BranchInfo) {
  if (!currentProjectId.value || !canWriteBranches.value) {
    message.warning(t('branch.notGitRepoShort'));
    return;
  }
  const requiresForce = Boolean(branch.hasWorktree);
  dialog.warning({
    title: requiresForce ? t('branch.forceDeleteBranch') : t('branch.deleteBranch'),
    content: `${t('branch.confirmDeleteBranch', { name: branch.name })}${
      requiresForce ? ` ${t('branch.confirmDeleteBranchWithWorktree')}` : ''
    }`,
    negativeText: t('common.cancel'),
    positiveText: requiresForce ? t('branch.forceDelete') : t('common.delete'),
    onPositiveClick: async () => {
      try {
        await deleteBranchReq.send(currentProjectId.value, branch.name, requiresForce);
        message.success(t('branch.branchDeleted'));
        await reloadBranches(true);
        await projectStore.fetchWorktrees(currentProjectId.value);
      } catch (error) {
        message.error(getErrorMessage(error, t('branch.deleteFailed')));
      }
    },
  });
}

async function handleCreateWorktree(branch: BranchInfo) {
  if (
    !currentProjectId.value ||
    !gitOperationAvailable(projectStore.gitCapabilities, 'worktreesWrite')
  ) {
    message.warning(t('branch.notGitRepoShort'));
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
    message.success(t('branch.worktreeCreated'));
  } catch (error) {
    message.error(getErrorMessage(error, t('branch.createWorktreeFailed')));
  }
}

function handleOpenWorktree(branch: BranchInfo) {
  if (!projectStore.currentProject) {
    return;
  }
  const target = projectStore.worktrees.find(worktree => worktree.branchName === branch.name);
  if (!target) {
    message.warning(t('branch.noWorktreeCreated'));
    return;
  }
  projectStore.setSelectedWorktree(target.id);
  router.push({ name: 'project', params: { id: projectStore.currentProject.id } });
}

async function submitMerge() {
  if (
    !gitOperationAvailable(projectStore.gitCapabilities, mergeOperation.value, mergeForm.worktreeId)
  ) {
    message.warning(t('branch.notGitRepoShort'));
    return;
  }
  const worktreeId = mergeForm.worktreeId;
  const targetBranch = mergeForm.targetBranch.trim();
  const sourceBranch = mergeForm.sourceBranch;
  if (!worktreeId || !sourceBranch) {
    message.warning(t('branch.selectWorktreeAndSource'));
    return;
  }
  if (!targetBranch) {
    message.warning(t('branch.cannotDetermineTarget'));
    return;
  }
  const commitAfter = shouldCommitAfterSquash.value;
  const commitMessage = commitAfter ? mergeForm.commitMessage.trim() : '';
  if (commitAfter && !commitMessage) {
    message.warning(t('branch.commitMessageRequired'));
    return;
  }
  try {
    const response = await mergeBranchReq.send(worktreeId, {
      targetBranch,
      sourceBranch,
      strategy: mergeForm.strategy,
      commit: commitAfter,
      commitMessage,
    });
    const payload = extractItem(response) as MergeResult | undefined;
    if (payload) {
      mergeResult.value = payload;
      if (payload.success) {
        message.success(payload.message || t('branch.mergeCompleted'));
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
          await Promise.all(
            Array.from(refreshIds).map(async id => {
              const result = await refreshWorktreeStatusReq.send(id);
              const updated = extractItem(result);
              if (updated) {
                projectStore.updateWorktreeInList(id, updated);
              }
            })
          );
        }
      } else {
        message.warning(payload.message || t('branch.hasConflicts'));
      }
    }
  } catch (error) {
    message.error(getErrorMessage(error, t('branch.mergeFailed')));
  }
}

async function refreshMergeStatus() {
  if (!canReadStatus.value) {
    message.warning(t('branch.notGitRepoShort'));
    return;
  }
  if (!mergeForm.worktreeId) {
    return;
  }
  try {
    const result = await refreshWorktreeStatusReq.send(mergeForm.worktreeId);
    const updated = extractItem(result);
    if (updated) {
      projectStore.updateWorktreeInList(mergeForm.worktreeId, updated);
    }
    message.success(t('branch.worktreeStatusRefreshed'));
  } catch (error) {
    message.error(getErrorMessage(error, t('branch.refreshFailed')));
  }
}

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
  return (
    !hasUncommittedChanges &&
    gitOperationAvailable(
      projectStore.gitCapabilities,
      mergeOperation.value,
      mergeForm.worktreeId
    ) &&
    (!shouldCommitAfterSquash.value || Boolean(mergeForm.commitMessage.trim()))
  );
});

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
