<template>
  <n-card
    class="worktree-card"
    :class="{ 'is-main': worktree.isMain, 'is-selected': selected }"
    size="small"
    @click="handleSelect"
  >
    <template #header>
      <n-space justify="space-between" align="center">
        <n-space align="center" size="small">
          <n-ellipsis style="max-width: 160px">
            {{ worktree.branchName }}
          </n-ellipsis>
          <n-tag v-if="worktree.isMain" size="small" round type="info">默认</n-tag>
        </n-space>
        <n-space align="center" :size="8">
          <n-tooltip trigger="hover" placement="bottom">
            <template #trigger>
              <n-button text size="small" @click.stop="emit('refresh', worktree.id)">
                <n-icon><RefreshOutline /></n-icon>
              </n-button>
            </template>
            <div>
              <div>刷新状态</div>
              <div style="font-size: 12px; opacity: 0.7;">
                {{ formatRefreshTime(worktree.statusUpdatedAt) }}
              </div>
            </div>
          </n-tooltip>
          <n-tooltip trigger="hover" placement="bottom">
            <template #trigger>
              <n-button text size="small" @click.stop="emit('open-terminal', worktree)">
                <n-icon><Terminal /></n-icon>
              </n-button>
            </template>
            打开终端
          </n-tooltip>
          <n-dropdown :options="actions" @select="handleAction">
            <n-button text size="small" @click.stop>
              <n-icon><EllipsisHorizontalOutline /></n-icon>
            </n-button>
          </n-dropdown>
        </n-space>
      </n-space>
    </template>

    <n-space vertical size="small">
      <GitStatusBadge :worktree="worktree" />

      <n-text depth="3" class="meta-text">
        {{ worktree.headCommit || '无提交信息' }}
      </n-text>

      <n-text depth="3" class="meta-text">
        {{ formatCommitTime(worktree.headCommitDate) }}
      </n-text>
    </n-space>

    <div class="worktree-card__actions" @click.stop>
      <n-button size="tiny" tertiary :disabled="!canSync" @click="emit('sync-default', worktree)">
        Rebase
      </n-button>
      <n-button
        size="tiny"
        tertiary
        :disabled="!canMerge"
        @click="emit('merge-to-default', { worktree, strategy: 'squash' })"
      >
        合并至
      </n-button>
      <n-button
        size="tiny"
        tertiary
        :disabled="!canCommit"
        @click="emit('commit-worktree', worktree)"
      >
        Commit
      </n-button>
    </div>
  </n-card>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import dayjs from 'dayjs';
import relativeTime from 'dayjs/plugin/relativeTime';
import type { DropdownOption } from 'naive-ui';
import { EllipsisHorizontalOutline, RefreshOutline, Terminal } from '@vicons/ionicons5';
import GitStatusBadge from '@/components/common/GitStatusBadge.vue';
import type { Worktree } from '@/types/models';

dayjs.extend(relativeTime);
dayjs.locale('zh-cn');

const props = defineProps<{
  worktree: Worktree;
  selected?: boolean;
  canSync?: boolean;
  canMerge?: boolean;
  canCommit?: boolean;
  isDeleting?: boolean;
}>();

const emit = defineEmits<{
  refresh: [id: string];
  delete: [worktree: Worktree];
  'open-explorer': [path: string];
  'open-terminal': [worktree: Worktree];
  select: [id: string];
  'sync-default': [worktree: Worktree];
  'merge-to-default': [payload: { worktree: Worktree; strategy: 'merge' | 'squash' }];
  'commit-worktree': [worktree: Worktree];
}>();

const actions = computed<DropdownOption[]>(() => {
  const baseActions: DropdownOption[] = [
    { label: '打开文件管理器', key: 'explorer' },
    { label: '打开终端', key: 'terminal' },
  ];

  if (props.canSync) {
    baseActions.push({
      label: 'Rebase',
      key: 'sync-rebase',
    });
  }

  if (props.canMerge) {
    baseActions.push({
      label: '合并至',
      key: 'merge-group',
      children: [
        { label: 'Merge', key: 'merge-merge' },
        { label: 'Squash', key: 'merge-squash' },
      ],
    });
  }

  if (props.canCommit) {
    baseActions.push({
      label: 'Commit',
      key: 'commit',
    });
  }

  baseActions.push({
    label: props.isDeleting ? '删除中...' : '删除',
    key: 'delete',
    disabled: props.worktree.isMain || props.isDeleting,
  });
  return baseActions;
});

function handleAction(key: string | number) {
  switch (key) {
    case 'explorer':
      emit('open-explorer', props.worktree.path);
      break;
    case 'terminal':
      emit('open-terminal', props.worktree);
      break;
    case 'sync-rebase':
      emit('sync-default', props.worktree);
      break;
    case 'merge-merge':
      emit('merge-to-default', { worktree: props.worktree, strategy: 'merge' });
      break;
    case 'merge-squash':
      emit('merge-to-default', { worktree: props.worktree, strategy: 'squash' });
      break;
    case 'commit':
      emit('commit-worktree', props.worktree);
      break;
    case 'delete':
      emit('delete', props.worktree);
      break;
    default:
      break;
  }
}

function formatCommitTime(time: string | null) {
  if (!time) {
    return '无提交';
  }
  return '提交于 ' + dayjs(time).fromNow();
}

function formatRefreshTime(time: string | null) {
  if (!time) {
    return '未刷新';
  }
  return '上次刷新：' + dayjs(time).fromNow();
}

function handleSelect() {
  emit('select', props.worktree.id);
}
</script>

<style scoped>
.worktree-card {
  margin-bottom: 8px;
  cursor: pointer;
  transition: box-shadow 0.2s ease, transform 0.2s ease;
}

.worktree-card:hover {
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.1);
  transform: translateY(-1px);
}

.worktree-card.is-selected {
  border-color: var(--n-color-primary);
  box-shadow: 0 0 0 1px var(--n-color-primary);
}

.meta-text {
  font-size: 12px;
}

.worktree-card__actions {
  display: flex;
  gap: 8px;
  margin-top: 8px;
}
</style>
