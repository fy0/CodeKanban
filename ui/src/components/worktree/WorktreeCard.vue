<template>
  <n-card
    class="worktree-card"
    :class="{ 'is-main': worktree.isMain, 'is-selected': selected }"
    size="small"
    @click="handleSelect"
  >
    <template #header>
      <div class="worktree-card__header">
        <div class="worktree-card__branch">
          <n-ellipsis class="worktree-card__branch-name">
            {{ worktree.branchName }}
          </n-ellipsis>
          <n-tag v-if="worktree.isMain" size="small" round type="info">{{
            t('worktree.default')
          }}</n-tag>
        </div>
        <div class="worktree-card__actions-header">
          <n-button-group size="tiny">
            <n-tooltip trigger="hover" placement="bottom">
              <template #trigger>
                <n-button
                  text
                  size="tiny"
                  @click.stop="handleEditorButtonClick"
                  class="action-button"
                >
                  <n-icon :size="14"><CodeSlashOutline /></n-icon>
                </n-button>
              </template>
              {{ t('worktree.openWith', { editor: defaultEditorLabel }) }}
            </n-tooltip>
            <n-dropdown :options="editorDropdownOptions" @select="handleEditorSelect">
              <n-button text size="tiny" @click.stop class="action-button">
                <n-icon :size="14"><ChevronDownOutline /></n-icon>
              </n-button>
            </n-dropdown>
          </n-button-group>
          <n-popover trigger="hover" placement="bottom" :disabled="!refreshDisabledReason">
            <template #trigger>
              <n-tooltip
                trigger="hover"
                placement="bottom"
                :disabled="Boolean(refreshDisabledReason)"
              >
                <template #trigger>
                  <n-button
                    text
                    size="tiny"
                    :disabled="!canRefresh"
                    @click.stop="emit('refresh', worktree.id)"
                    class="action-button"
                  >
                    <n-icon :size="14"><RefreshOutline /></n-icon>
                  </n-button>
                </template>
                <div>
                  <div>{{ t('worktree.refreshStatus') }}</div>
                  <div style="font-size: 12px; opacity: 0.7">
                    {{ formatRefreshTime(worktree.statusUpdatedAt) }}
                  </div>
                </div>
              </n-tooltip>
            </template>
            <span>{{ refreshDisabledReason }}</span>
          </n-popover>
          <n-tooltip trigger="hover" placement="bottom">
            <template #trigger>
              <n-button
                text
                size="tiny"
                @click.stop="emit('open-terminal', worktree)"
                class="action-button"
              >
                <n-icon :size="14"><Terminal /></n-icon>
              </n-button>
            </template>
            {{ t('worktree.openTerminal') }}
          </n-tooltip>
          <n-dropdown :options="actions" @select="handleAction">
            <n-button text size="tiny" @click.stop class="action-button">
              <n-icon :size="14"><EllipsisHorizontalOutline /></n-icon>
            </n-button>
          </n-dropdown>
        </div>
      </div>
    </template>

    <n-space vertical size="small">
      <GitStatusBadge :worktree="worktree" />

      <template v-if="hasCommitDetails">
        <n-popover trigger="hover" placement="bottom-start" :delay="1000" :show-arrow="false">
          <template #trigger>
            <n-text depth="3" class="meta-text worktree-card__commit" tag="span">
              <n-ellipsis :line-clamp="1" class="worktree-card__commit-text">
                {{ commitInlineText }}
              </n-ellipsis>
            </n-text>
          </template>
          <div class="worktree-card__commit-popover">
            <span class="worktree-card__commit-hash">{{
              popoverCommitHash || t('worktree.noCommit')
            }}</span>
            <span v-if="popoverCommitMessage" class="worktree-card__commit-message">{{
              popoverCommitMessage
            }}</span>
          </div>
        </n-popover>
      </template>
      <n-text v-else depth="3" class="meta-text">
        {{ t('worktree.noCommitInfo') }}
      </n-text>

      <n-text depth="3" class="meta-text">
        {{ formatCommitTime(worktree.headCommitDate) }}
      </n-text>
    </n-space>

    <div class="worktree-card__actions" @click.stop>
      <n-popover trigger="hover" placement="bottom" :disabled="!mergeDisabledReason">
        <template #trigger>
          <n-button
            size="tiny"
            tertiary
            :disabled="!canMerge"
            @click="emit('merge-to-default', { worktree })"
          >
            {{ t('worktree.mergeTo') }}
          </n-button>
        </template>
        <span>{{ mergeDisabledReason }}</span>
      </n-popover>
      <n-popover trigger="hover" placement="bottom" :disabled="!commitDisabledReason">
        <template #trigger>
          <n-button
            size="tiny"
            tertiary
            :disabled="!canCommit"
            @click="emit('commit-worktree', worktree)"
          >
            Commit
          </n-button>
        </template>
        <span>{{ commitDisabledReason }}</span>
      </n-popover>
    </div>
  </n-card>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import dayjs from 'dayjs';
import relativeTime from 'dayjs/plugin/relativeTime';
import type { DropdownOption } from 'naive-ui';
import {
  ChevronDownOutline,
  CodeSlashOutline,
  EllipsisHorizontalOutline,
  RefreshOutline,
  Terminal,
} from '@vicons/ionicons5';
import GitStatusBadge from '@/components/common/GitStatusBadge.vue';
import type { Worktree } from '@/types/models';
import type { EditorPreference } from '@/stores/settings';
import {
  DEFAULT_EDITOR,
  EDITOR_OPTIONS,
  EDITOR_LABEL_MAP,
  isEditorPreference,
} from '@/constants/editor';
import { useLocale } from '@/composables/useLocale';

const { t, locale } = useLocale();

dayjs.extend(relativeTime);

type EditorOption = {
  label: string;
  value: EditorPreference;
  disabled?: boolean;
};

const props = withDefaults(
  defineProps<{
    worktree: Worktree;
    selected?: boolean;
    canRefresh?: boolean;
    canMerge?: boolean;
    canCommit?: boolean;
    refreshDisabledReason?: string;
    mergeDisabledReason?: string;
    commitDisabledReason?: string;
    isDeleting?: boolean;
    defaultEditor?: EditorPreference;
    editorOptions?: EditorOption[];
  }>(),
  {
    defaultEditor: DEFAULT_EDITOR,
    editorOptions: () => EDITOR_OPTIONS.map(option => ({ ...option })),
    canRefresh: true,
    refreshDisabledReason: '',
    mergeDisabledReason: '',
    commitDisabledReason: '',
  }
);

const emit = defineEmits<{
  refresh: [id: string];
  delete: [worktree: Worktree];
  'open-explorer': [path: string];
  'open-terminal': [worktree: Worktree];
  'open-editor': [payload: { worktree: Worktree; editor: EditorPreference }];
  select: [id: string];
  'merge-to-default': [payload: { worktree: Worktree }];
  'commit-worktree': [worktree: Worktree];
}>();

const actions = computed<DropdownOption[]>(() => {
  const baseActions: DropdownOption[] = [
    { label: t('worktree.openInExplorer2'), key: 'explorer' },
    { label: t('worktree.openTerminal'), key: 'terminal' },
  ];

  if (props.canMerge) {
    baseActions.push({
      label: t('worktree.mergeTo'),
      key: 'merge-group',
    });
  }

  if (props.canCommit) {
    baseActions.push({
      label: 'Commit',
      key: 'commit',
    });
  }

  baseActions.push({
    label: props.isDeleting ? t('worktree.deleting') : t('common.delete'),
    key: 'delete',
    disabled: props.worktree.isMain || props.isDeleting,
  });
  return baseActions;
});

const resolvedDefaultEditor = computed<EditorPreference>(() =>
  props.defaultEditor && isEditorPreference(props.defaultEditor)
    ? props.defaultEditor
    : DEFAULT_EDITOR
);

const resolvedEditorOptions = computed<EditorOption[]>(() =>
  (props.editorOptions && props.editorOptions.length ? props.editorOptions : EDITOR_OPTIONS).map(
    option => ({ ...option })
  )
);

const editorDropdownOptions = computed<DropdownOption[]>(() =>
  resolvedEditorOptions.value.map(option => ({
    label: option.label,
    key: option.value,
    disabled: option.disabled,
  }))
);

const defaultEditorLabel = computed(
  () => EDITOR_LABEL_MAP[resolvedDefaultEditor.value] ?? '编辑器'
);

const commitHash = computed(() => (props.worktree.headCommit || '').trim());
const commitMessage = computed(() => (props.worktree.headCommitMessage || '').trim());
const hasCommitDetails = computed(() => Boolean(commitHash.value || commitMessage.value));
const commitInlineText = computed(() => {
  if (!hasCommitDetails.value) {
    return '';
  }
  if (commitHash.value && commitMessage.value) {
    return `${commitHash.value} ${commitMessage.value}`;
  }
  return commitHash.value || commitMessage.value;
});
const popoverCommitHash = computed(() => commitHash.value);
const popoverCommitMessage = computed(() => commitMessage.value);

function handleAction(key: string | number) {
  switch (key) {
    case 'explorer':
      emit('open-explorer', props.worktree.path);
      break;
    case 'terminal':
      emit('open-terminal', props.worktree);
      break;
    case 'merge-group':
      emit('merge-to-default', { worktree: props.worktree });
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

function handleEditorButtonClick() {
  emit('open-editor', { worktree: props.worktree, editor: resolvedDefaultEditor.value });
}

function handleEditorSelect(key: string | number) {
  if (typeof key !== 'string' || !isEditorPreference(key)) {
    return;
  }
  emit('open-editor', { worktree: props.worktree, editor: key });
}

function formatCommitTime(time: string | null) {
  if (!time) {
    return t('worktree.noCommit');
  }
  const dayjsLocale = locale.value === 'zh-CN' ? 'zh-cn' : 'en';
  return t('worktree.committedAt') + ' ' + dayjs(time).locale(dayjsLocale).fromNow();
}

function formatRefreshTime(time: string | null) {
  if (!time) {
    return t('worktree.notRefreshed');
  }
  const dayjsLocale = locale.value === 'zh-CN' ? 'zh-cn' : 'en';
  return t('worktree.lastRefreshed') + dayjs(time).locale(dayjsLocale).fromNow();
}

function handleSelect() {
  emit('select', props.worktree.id);
}
</script>

<style scoped>
.worktree-card {
  margin-bottom: 8px;
  cursor: pointer;
  transition:
    box-shadow 0.2s ease,
    transform 0.2s ease;
}

.worktree-card:hover {
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.1);
  transform: translateY(-1px);
}

.worktree-card.is-selected {
  border-color: var(--n-color-primary);
  box-shadow: 0 0 0 1px var(--n-color-primary);
}

.worktree-card__header {
  display: flex;
  flex-direction: column;
  align-items: stretch;
  gap: 4px 8px;
}

.worktree-card__branch {
  display: flex;
  align-items: center;
  width: 100%;
  min-width: 0;
  gap: 8px;
}

.worktree-card__branch-name {
  flex: 1 1 auto;
  min-width: 0;
  max-width: 160px;
}

.meta-text {
  font-size: 12px;
}

.worktree-card__commit {
  max-width: 100%;
  font-size: 12px;
  color: var(--n-text-color-2);
  display: inline-flex;
  gap: 4px;
  align-items: center;
}

.worktree-card__commit-text {
  flex: 1 1 auto;
  min-width: 0;
  display: block;
  color: var(--n-text-color-2);
}

.worktree-card__commit-popover {
  display: flex;
  flex-direction: column;
  gap: 4px;
  max-width: 360px;
}

.worktree-card__commit-hash {
  font-family: SFMono-Regular, Consolas, 'Liberation Mono', Menlo, monospace;
  font-weight: 600;
}

.worktree-card__commit-message {
  white-space: pre-wrap;
}

.worktree-card__actions {
  display: flex;
  gap: 8px;
  margin-top: 8px;
}

.worktree-card__actions-header {
  display: flex;
  align-items: center;
  justify-content: flex-start;
  flex: 0 0 auto;
  width: 100%;
}

.worktree-card__actions-header :deep(.action-button) {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  height: 28px;
  min-width: unset !important;
  width: auto !important;
  padding: 0 3px !important;
}

.worktree-card__actions-header :deep(.n-button-group .action-button) {
  padding: 0 !important;
}

.worktree-card__actions-header :deep(.n-button-group .action-button + .action-button) {
  margin-left: -14px !important;
}

.worktree-card__actions-header :deep(.action-button .n-icon) {
  display: flex;
  align-items: center;
  justify-content: center;
}
</style>
