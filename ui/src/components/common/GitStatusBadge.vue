<template>
  <n-space size="small">
    <n-tooltip
      v-if="(worktree.statusAhead ?? 0) > 0 || (worktree.statusBehind ?? 0) > 0"
      trigger="hover"
      placement="bottom"
    >
      <template #trigger>
        <n-tag size="small" type="info" :bordered="false">
          ↑{{ worktree.statusAhead ?? 0 }} ↓{{ worktree.statusBehind ?? 0 }}
        </n-tag>
      </template>
      领先/落后远程分支的提交数
    </n-tooltip>

    <n-tooltip v-if="(worktree.statusModified ?? 0) > 0" trigger="hover" placement="bottom">
      <template #trigger>
        <n-tag size="small" type="warning" :bordered="false">
          ~{{ worktree.statusModified ?? 0 }}
        </n-tag>
      </template>
      已修改但未暂存的文件数
    </n-tooltip>

    <n-tooltip v-if="(worktree.statusStaged ?? 0) > 0" trigger="hover" placement="bottom">
      <template #trigger>
        <n-tag size="small" type="success" :bordered="false">
          +{{ worktree.statusStaged ?? 0 }}
        </n-tag>
      </template>
      已暂存待提交的文件数
    </n-tooltip>

    <n-tooltip v-if="(worktree.statusUntracked ?? 0) > 0" trigger="hover" placement="bottom">
      <template #trigger>
        <n-tag size="small" :bordered="false">
          ?{{ worktree.statusUntracked ?? 0 }}
        </n-tag>
      </template>
      未跟踪的新文件数
    </n-tooltip>
  </n-space>
</template>

<script setup lang="ts">
import type { Worktree } from '@/types/models';

defineProps<{
  worktree: Worktree;
}>();
</script>
