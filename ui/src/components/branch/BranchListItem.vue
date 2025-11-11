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
        <n-tag v-if="mode === 'remote'" size="small" :bordered="false">远程</n-tag>
        <n-tag v-if="isDefault" size="small" type="info" :bordered="false">默认</n-tag>
        <n-tag v-if="branch.isCurrent" size="small" type="success" :bordered="false">当前</n-tag>
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
      <n-text depth="3">最新提交: {{ branch.headCommit || '—' }}</n-text>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { DropdownOption } from 'naive-ui';
import { EllipsisHorizontalOutline, GitBranchOutline } from '@vicons/ionicons5';
import type { BranchInfo } from '@/types/models';

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

const actionOptions = computed<DropdownOption[]>(() => {
  if (props.mode === 'local') {
    const options: DropdownOption[] = [];
    if (props.branch.hasWorktree) {
      options.push({ label: '打开 Worktree', key: 'open-worktree' });
    } else {
      options.push({ label: '创建 Worktree', key: 'create-worktree' });
    }
    options.push({
      label: '删除分支',
      key: 'delete',
    });
    return options;
  }
  return [{ label: '创建本地分支', key: 'checkout' }];
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
  border: 1px solid var(--color-border);
  border-radius: 8px;
  padding: 12px;
  background-color: var(--color-background);
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
  color: var(--vt-c-text-light-2);
}

.branch-name {
  max-width: 220px;
}
</style>
