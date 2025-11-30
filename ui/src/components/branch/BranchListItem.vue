<template>
  <div class="branch-card">
    <div class="branch-card__header">
      <n-space align="center" size="small">
        <n-icon size="18">
          <GitBranchOutline />
        </n-icon>
        <n-ellipsis class="branch-name">
          {{ branch.name }}
        </n-ellipsis>
        <n-tag v-if="mode === 'remote'" size="small" :bordered="false">{{ t('branch.remote') }}</n-tag>
        <n-tag v-if="isDefault" size="small" type="info" :bordered="false">{{ t('branch.default') }}</n-tag>
        <n-tag v-if="branch.isCurrent" size="small" type="success" :bordered="false">{{ t('branch.current') }}</n-tag>
        <n-tag v-if="branch.hasWorktree" size="small" type="warning" :bordered="false">Worktree</n-tag>
      </n-space>
      <n-dropdown
        v-if="actionOptions.length"
        trigger="click"
        :options="actionOptions"
        @select="handleSelect"
      >
        <n-button text size="small">
          <n-icon size="18">
            <EllipsisHorizontalOutline />
          </n-icon>
        </n-button>
      </n-dropdown>
    </div>
    <div class="branch-card__meta">
      <n-popover
        v-if="hasCommitDetails"
        trigger="hover"
        placement="bottom-start"
        :delay="1000"
        :show-arrow="false"
      >
        <template #trigger>
          <n-ellipsis class="branch-card__commit" :line-clamp="1">
            <span class="branch-card__commit-label">{{ t('branch.latestCommit') }}: </span>
            <span class="branch-card__commit-text">{{ commitInlineText }}</span>
          </n-ellipsis>
        </template>
        <div class="branch-card__commit-popover">
          <span class="branch-card__commit-hash">{{ popoverHash || t('worktree.noCommit') }}</span>
          <span v-if="popoverMessage" class="branch-card__commit-message">{{ popoverMessage }}</span>
        </div>
      </n-popover>
      <n-text v-else depth="3">{{ t('branch.latestCommit') }}: {{ t('worktree.noCommitInfo') }}</n-text>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { DropdownOption } from 'naive-ui';
import { EllipsisHorizontalOutline, GitBranchOutline } from '@vicons/ionicons5';
import type { BranchInfo } from '@/types/models';
import { useLocale } from '@/composables/useLocale';

const { t } = useLocale();

const props = defineProps<{
  branch: BranchInfo;
  mode: 'local' | 'remote';
  defaultBranch?: string;
}>();

const emit = defineEmits<{
  (event: 'create-worktree', branch: BranchInfo): void;
  (event: 'open-worktree', branch: BranchInfo): void;
  (event: 'delete', branch: BranchInfo): void;
  (event: 'checkout', branch: BranchInfo): void;
}>();

const isDefault = computed(() => {
  return props.defaultBranch ? props.branch.name === props.defaultBranch : false;
});

const commitInlineText = computed(() => {
  const hash = (props.branch.headCommit || '').trim();
  const message = (props.branch.headCommitMessage || '').trim();
  if (hash && message) {
    return `${hash} ${message}`;
  }
  return hash || message;
});

const popoverHash = computed(() => (props.branch.headCommit || '').trim());
const popoverMessage = computed(() => (props.branch.headCommitMessage || '').trim());
const hasCommitDetails = computed(() => Boolean(commitInlineText.value));

const actionOptions = computed<DropdownOption[]>(() => {
  if (props.mode === 'local') {
    const options: DropdownOption[] = [];
    if (props.branch.hasWorktree) {
      options.push({ label: t('branch.openWorktree'), key: 'open-worktree' });
    } else {
      options.push({ label: t('branch.createWorktree'), key: 'create-worktree' });
    }
    options.push({
      label: t('branch.deleteBranch'),
      key: 'delete',
    });
    return options;
  }
  return [{ label: t('branch.createLocalBranch'), key: 'checkout' }];
});

function handleSelect(key: string | number) {
  switch (key) {
    case 'create-worktree':
      emit('create-worktree', props.branch);
      break;
    case 'open-worktree':
      emit('open-worktree', props.branch);
      break;
    case 'delete':
      emit('delete', props.branch);
      break;
    case 'checkout':
      emit('checkout', props.branch);
      break;
    default:
      break;
  }
}
</script>

<style scoped>
.branch-card {
  border: 1px solid var(--n-border-color);
  border-radius: 8px;
  padding: 12px;
  background-color: var(--app-surface-color);
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.branch-card__header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
}

.branch-card__meta {
  font-size: 12px;
  color: var(--n-text-color-2);
}

.branch-card__commit {
  max-width: 100%;
  font-size: 12px;
  color: var(--n-text-color-2);
  display: block;
  white-space: nowrap;
}

.branch-card__commit-text {
  white-space: nowrap;
}

.branch-card__commit-popover {
  display: flex;
  flex-direction: column;
  gap: 4px;
  max-width: 360px;
}

.branch-card__commit-hash {
  font-family: SFMono-Regular, Consolas, 'Liberation Mono', Menlo, monospace;
  font-weight: 600;
}

.branch-card__commit-message {
  white-space: pre-wrap;
}

.branch-name {
  max-width: 220px;
}
</style>
