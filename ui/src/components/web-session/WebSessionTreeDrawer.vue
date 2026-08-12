<template>
  <n-drawer
    :show="show"
    :placement="isMobile ? 'bottom' : 'right'"
    :width="isMobile ? undefined : 440"
    :height="isMobile ? 'min(88dvh, 720px)' : undefined"
    @update:show="emit('update:show', $event)"
    @after-enter="void loadTree()"
  >
    <n-drawer-content
      :title="t('webSession.treeTitle')"
      closable
      :native-scrollbar="false"
      :body-content-style="treeBodyContentStyle"
    >
      <div class="web-session-tree">
        <div class="web-session-tree__toolbar">
          <span class="web-session-tree__status">
            {{ tree ? t('webSession.treeNodeCount', { count: tree.nodes.length }) : '' }}
          </span>
          <n-tooltip trigger="hover">
            <template #trigger>
              <n-button
                quaternary
                circle
                size="small"
                :loading="loading"
                :disabled="Boolean(operation)"
                :aria-label="t('common.refresh')"
                @click="void loadTree()"
              >
                <template #icon
                  ><n-icon><RefreshOutline /></n-icon
                ></template>
              </n-button>
            </template>
            {{ t('common.refresh') }}
          </n-tooltip>
        </div>

        <div v-if="error" class="web-session-tree__error" role="alert">
          <span>{{ error }}</span>
          <n-button text type="primary" size="small" @click="void loadTree()">
            {{ t('common.retry') }}
          </n-button>
        </div>

        <div v-if="loading && !tree" class="web-session-tree__loading">
          <n-spin size="small" />
        </div>
        <n-empty
          v-else-if="tree && tree.nodes.length === 0"
          class="web-session-tree__empty"
          :description="t('webSession.treeEmpty')"
        />
        <n-virtual-list
          v-else-if="tree"
          class="web-session-tree__list"
          :items="rows"
          :item-size="52"
          key-field="id"
        >
          <template #default="{ item }">
            <button
              type="button"
              class="web-session-tree__node"
              :class="{
                'is-active': item.node.active,
                'is-selected': selectedId === item.id,
                'is-leaf': tree?.leafId === item.id,
              }"
              :style="{ '--tree-depth': Math.min(item.depth, 12) }"
              :title="item.node.preview || nodeTypeLabel(item.node)"
              :aria-current="tree?.leafId === item.id ? 'step' : undefined"
              @click="selectedId = item.id"
            >
              <span class="web-session-tree__rail" aria-hidden="true"></span>
              <span class="web-session-tree__node-icon" aria-hidden="true">
                <n-icon size="15"><component :is="nodeIcon(item.node)" /></n-icon>
              </span>
              <span class="web-session-tree__node-copy">
                <span class="web-session-tree__node-meta">
                  <span>{{ nodeTypeLabel(item.node) }}</span>
                  <span v-if="item.node.label">{{ item.node.label }}</span>
                  <span v-if="tree?.leafId === item.id" class="web-session-tree__leaf-label">
                    {{ t('webSession.treeCurrentLeaf') }}
                  </span>
                </span>
                <span class="web-session-tree__node-preview">
                  {{ item.node.preview || t('webSession.treeNoPreview') }}
                </span>
              </span>
            </button>
          </template>
        </n-virtual-list>

        <div class="web-session-tree__actions">
          <div class="web-session-tree__selection">
            <span>{{
              selectedNode ? nodeTypeLabel(selectedNode) : t('webSession.treeSelectNode')
            }}</span>
            <span v-if="selectedNode?.preview">{{ selectedNode.preview }}</span>
          </div>
          <n-checkbox v-model:checked="summarize" :disabled="!canOperate">
            {{ t('webSession.treeSummarize') }}
          </n-checkbox>
          <div class="web-session-tree__commands">
            <n-button
              secondary
              size="small"
              :disabled="!canNavigate"
              :loading="operation === 'navigate'"
              @click="confirmNavigate"
            >
              <template #icon
                ><n-icon><ReturnUpBackOutline /></n-icon
              ></template>
              {{ navigateLabel }}
            </n-button>
            <n-button
              secondary
              size="small"
              :disabled="!canFork"
              :loading="operation === 'fork'"
              @click="void forkSelected()"
            >
              <template #icon
                ><n-icon><GitBranchOutline /></n-icon
              ></template>
              {{ t('webSession.treeFork') }}
            </n-button>
            <n-button
              secondary
              size="small"
              :disabled="!canClone"
              :loading="operation === 'clone'"
              @click="void cloneCurrent()"
            >
              <template #icon
                ><n-icon><CopyOutline /></n-icon
              ></template>
              {{ t('webSession.treeClone') }}
            </n-button>
          </div>
          <p v-if="!canMutate" class="web-session-tree__hint">
            {{ t('webSession.treeIdleRequired') }}
          </p>
        </div>
      </div>
    </n-drawer-content>
  </n-drawer>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useDialog, useMessage } from 'naive-ui';
import {
  ChatbubbleOutline,
  CodeSlashOutline,
  CopyOutline,
  DocumentTextOutline,
  GitBranchOutline,
  RefreshOutline,
  ReturnUpBackOutline,
  SparklesOutline,
} from '@vicons/ionicons5';
import { useLocale } from '@/composables/useLocale';
import { useResponsive } from '@/composables/useResponsive';
import {
  webSessionApi,
  type WebSessionPiTree,
  type WebSessionPiTreeNode,
  type WebSessionPiTreeMutationResult,
} from '@/api/webSession';
import {
  canForkWebSessionPiTreeNode,
  projectWebSessionPiTreeRows,
} from '@/components/web-session/webSessionTree';

const props = defineProps<{
  show: boolean;
  projectId: string;
  sessionId: string;
  canMutate: boolean;
}>();

const treeBodyContentStyle = {
  height: '100%',
  minHeight: 0,
  overflow: 'hidden',
  padding: 0,
};

const emit = defineEmits<{
  (event: 'update:show', value: boolean): void;
  (event: 'navigated', value: { projectId: string; sessionId: string; editorText: string }): void;
  (
    event: 'created',
    value: {
      projectId: string;
      sessionId: string;
      result: WebSessionPiTreeMutationResult;
      activate: boolean;
    }
  ): void;
}>();

const { t } = useLocale();
const { isMobile } = useResponsive();
const dialog = useDialog();
const message = useMessage();
const tree = ref<WebSessionPiTree | null>(null);
const loading = ref(false);
const error = ref('');
const selectedId = ref('');
const summarize = ref(false);
const operation = ref<'navigate' | 'fork' | 'clone' | ''>('');
let requestEpoch = 0;
let operationToken = 0;

const byId = computed(() => new Map((tree.value?.nodes ?? []).map(node => [node.id, node])));
const rows = computed(() => projectWebSessionPiTreeRows(tree.value?.nodes ?? []));
const selectedNode = computed(() => byId.value.get(selectedId.value));
const canOperate = computed(() => props.canMutate && !loading.value && !operation.value);
const canNavigate = computed(
  () => canOperate.value && Boolean(selectedNode.value) && selectedId.value !== tree.value?.leafId
);
const canFork = computed(() => canOperate.value && canForkWebSessionPiTreeNode(selectedNode.value));
const canClone = computed(() => canOperate.value && Boolean(tree.value?.revision));
const navigateLabel = computed(() =>
  selectedNode.value?.role === 'user'
    ? t('webSession.treeRewriteFromHere')
    : t('webSession.treeContinueFromHere')
);

watch(
  () => [props.show, props.projectId, props.sessionId] as const,
  ([show]) => {
    requestEpoch += 1;
    operationToken += 1;
    tree.value = null;
    selectedId.value = '';
    summarize.value = false;
    operation.value = '';
    error.value = '';
    if (show) void loadTree();
  }
);

async function loadTree() {
  if (!props.show || !props.projectId || !props.sessionId || loading.value) return;
  const epoch = ++requestEpoch;
  loading.value = true;
  error.value = '';
  try {
    const next = await webSessionApi.tree(props.projectId, props.sessionId);
    if (epoch !== requestEpoch || !props.show) return;
    tree.value = next;
    const currentSelection = selectedId.value;
    selectedId.value = next.nodes.some(node => node.id === currentSelection)
      ? currentSelection
      : (next.leafId ??
        next.nodes.find(node => node.active)?.id ??
        next.nodes[next.nodes.length - 1]?.id ??
        '');
  } catch (cause) {
    if (epoch === requestEpoch && props.show) {
      error.value = cause instanceof Error ? cause.message : t('webSession.treeLoadFailed');
    }
  } finally {
    if (epoch === requestEpoch) loading.value = false;
  }
}

function confirmNavigate() {
  if (!canNavigate.value || !selectedNode.value) return;
  dialog.warning({
    title: navigateLabel.value,
    content: t('webSession.treeNavigateConfirm'),
    positiveText: navigateLabel.value,
    negativeText: t('common.cancel'),
    onPositiveClick: navigateSelected,
  });
}

function operationIsCurrent(token: number, projectId: string, sessionId: string) {
  return (
    token === operationToken &&
    props.show &&
    props.projectId === projectId &&
    props.sessionId === sessionId
  );
}

async function navigateSelected() {
  if (!canNavigate.value || !tree.value || !selectedNode.value) return false;
  const projectId = props.projectId;
  const sessionId = props.sessionId;
  const selected = selectedNode.value;
  const revision = tree.value.revision;
  const shouldSummarize = summarize.value;
  const token = ++operationToken;
  operation.value = 'navigate';
  try {
    const result = await webSessionApi.navigateTree(projectId, sessionId, {
      targetId: selected.id,
      revision,
      summarize: shouldSummarize,
    });
    emit('navigated', {
      projectId,
      sessionId,
      editorText: result.editorText ?? '',
    });
    if (operationIsCurrent(token, projectId, sessionId)) {
      tree.value = result.tree;
      selectedId.value = result.tree.leafId ?? selected.id;
      message.success(t('webSession.treeNavigateSuccess'));
    }
    return true;
  } catch (cause) {
    if (operationIsCurrent(token, projectId, sessionId)) {
      message.error(cause instanceof Error ? cause.message : t('webSession.treeNavigateFailed'));
      void loadTree();
    }
    return false;
  } finally {
    if (token === operationToken) operation.value = '';
  }
}

async function forkSelected() {
  if (!canFork.value || !tree.value || !selectedNode.value) return;
  const projectId = props.projectId;
  const sessionId = props.sessionId;
  const targetId = selectedNode.value.id;
  const revision = tree.value.revision;
  const token = ++operationToken;
  operation.value = 'fork';
  try {
    const result = await webSessionApi.forkTree(projectId, sessionId, {
      targetId,
      revision,
    });
    const activate = operationIsCurrent(token, projectId, sessionId);
    emit('created', { projectId, sessionId, result, activate });
    if (activate) {
      emit('update:show', false);
      message.success(t('webSession.treeForkSuccess'));
    }
  } catch (cause) {
    if (operationIsCurrent(token, projectId, sessionId)) {
      message.error(cause instanceof Error ? cause.message : t('webSession.treeForkFailed'));
      void loadTree();
    }
  } finally {
    if (token === operationToken) operation.value = '';
  }
}

async function cloneCurrent() {
  if (!canClone.value || !tree.value) return;
  const projectId = props.projectId;
  const sessionId = props.sessionId;
  const revision = tree.value.revision;
  const token = ++operationToken;
  operation.value = 'clone';
  try {
    const result = await webSessionApi.cloneTree(projectId, sessionId, { revision });
    const activate = operationIsCurrent(token, projectId, sessionId);
    emit('created', { projectId, sessionId, result, activate });
    if (activate) {
      emit('update:show', false);
      message.success(t('webSession.treeCloneSuccess'));
    }
  } catch (cause) {
    if (operationIsCurrent(token, projectId, sessionId)) {
      message.error(cause instanceof Error ? cause.message : t('webSession.treeCloneFailed'));
      void loadTree();
    }
  } finally {
    if (token === operationToken) operation.value = '';
  }
}

function nodeTypeLabel(node: WebSessionPiTreeNode) {
  if (node.type === 'message') {
    return node.role === 'user'
      ? t('webSession.treeUserMessage')
      : t('webSession.treeAssistantMessage');
  }
  if (node.type === 'custom_message') return t('webSession.treeCustomMessage');
  if (node.type === 'compaction') return t('webSession.treeCompaction');
  if (node.type === 'branch_summary') return t('webSession.treeBranchSummary');
  return node.type.split('_').join(' ');
}

function nodeIcon(node: WebSessionPiTreeNode) {
  if (node.type === 'message') return node.role === 'user' ? ChatbubbleOutline : SparklesOutline;
  if (node.type === 'compaction' || node.type === 'branch_summary') return DocumentTextOutline;
  return CodeSlashOutline;
}
</script>

<style scoped>
.web-session-tree {
  display: grid;
  grid-template-rows: auto auto minmax(0, 1fr) auto;
  height: 100%;
  min-height: 360px;
  background: var(--app-surface-color, #fff);
}

.web-session-tree__toolbar {
  grid-row: 1;
  display: flex;
  min-height: 42px;
  align-items: center;
  justify-content: space-between;
  padding: 6px 12px;
  border-bottom: 1px solid var(--app-border-color, rgba(127, 127, 127, 0.2));
}

.web-session-tree__status,
.web-session-tree__hint {
  color: var(--app-text-color-3, #777);
  font-size: 12px;
}

.web-session-tree__error {
  grid-row: 2;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 8px 12px;
  color: var(--error-color, #d03050);
  background: color-mix(in srgb, var(--error-color, #d03050) 8%, transparent);
  font-size: 12px;
}

.web-session-tree__loading,
.web-session-tree__empty {
  grid-row: 3;
  display: grid;
  min-height: 220px;
  place-items: center;
}

.web-session-tree__list {
  grid-row: 3;
  min-height: 0;
}

.web-session-tree__node {
  position: relative;
  display: grid;
  width: 100%;
  height: 52px;
  grid-template-columns: 22px minmax(0, 1fr);
  align-items: center;
  gap: 8px;
  padding: 5px 12px 5px calc(12px + var(--tree-depth) * 14px);
  overflow: hidden;
  border: 0;
  border-bottom: 1px solid color-mix(in srgb, var(--app-border-color, #999) 55%, transparent);
  background: transparent;
  color: inherit;
  text-align: left;
  cursor: pointer;
}

.web-session-tree__node:hover,
.web-session-tree__node.is-selected {
  background: color-mix(in srgb, var(--primary-color, #18a058) 9%, transparent);
}

.web-session-tree__node.is-selected {
  box-shadow: inset 3px 0 0 var(--primary-color, #18a058);
}

.web-session-tree__rail {
  position: absolute;
  top: 0;
  bottom: 0;
  left: calc(21px + var(--tree-depth) * 14px);
  width: 1px;
  background: var(--app-border-color, rgba(127, 127, 127, 0.25));
}

.web-session-tree__node.is-active .web-session-tree__rail {
  width: 2px;
  background: var(--primary-color, #18a058);
}

.web-session-tree__node-icon {
  z-index: 1;
  display: grid;
  width: 22px;
  height: 22px;
  place-items: center;
  border-radius: 50%;
  background: var(--app-surface-color, #fff);
  color: var(--app-text-color-3, #777);
}

.web-session-tree__node.is-active .web-session-tree__node-icon {
  color: var(--primary-color, #18a058);
}

.web-session-tree__node-copy {
  display: grid;
  min-width: 0;
  gap: 2px;
}

.web-session-tree__node-meta {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 7px;
  color: var(--app-text-color-3, #777);
  font-size: 11px;
  text-transform: capitalize;
}

.web-session-tree__leaf-label {
  color: var(--primary-color, #18a058);
  font-weight: 600;
}

.web-session-tree__node-preview,
.web-session-tree__selection span:last-child {
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
}

.web-session-tree__node-preview {
  font-size: 13px;
}

.web-session-tree__actions {
  grid-row: 4;
  display: grid;
  gap: 10px;
  min-height: 150px;
  padding: 12px;
  border-top: 1px solid var(--app-border-color, rgba(127, 127, 127, 0.2));
  background: var(--app-surface-color, #fff);
}

.web-session-tree__selection {
  display: grid;
  min-width: 0;
  gap: 2px;
}

.web-session-tree__selection span:first-child {
  font-size: 12px;
  font-weight: 600;
}

.web-session-tree__selection span:last-child {
  color: var(--app-text-color-3, #777);
  font-size: 12px;
}

.web-session-tree__commands {
  display: grid;
  grid-template-columns: minmax(0, 1.4fr) minmax(0, 1fr) minmax(0, 1fr);
  gap: 8px;
}

.web-session-tree__commands :deep(.n-button__content) {
  min-width: 0;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
}

.web-session-tree__hint {
  min-height: 18px;
  margin: 0;
}

@media (max-width: 640px) {
  .web-session-tree {
    min-height: 0;
  }

  .web-session-tree__commands {
    grid-template-columns: 1fr 1fr;
  }

  .web-session-tree__commands > :first-child {
    grid-column: 1 / -1;
  }
}
</style>
