<template>
  <div class="web-session-composer-editor" :style="editorStyleVars">
    <textarea
      ref="textareaRef"
      class="web-session-composer-editor__textarea"
      :value="modelValue"
      :placeholder="placeholder"
      rows="1"
      spellcheck="false"
      autocorrect="off"
      autocapitalize="off"
      autocomplete="off"
      @blur="handleBlur"
      @click="refreshCompletion"
      @compositionend="handleCompositionEnd"
      @compositionstart="handleCompositionStart"
      @focus="handleFocus"
      @input="handleInput"
      @keydown="handleKeydown"
      @keyup="refreshCompletion"
      @select="refreshCompletion"
    ></textarea>

    <div v-if="completionState.open" class="web-session-composer-editor__completion">
      <button
        v-for="(option, index) in completionState.options"
        :key="option.key"
        type="button"
        class="web-session-composer-editor__completion-option"
        :class="{ 'is-selected': index === completionState.selectedIndex }"
        @mousedown.prevent="applyCompletion(option)"
      >
        <span class="web-session-composer-editor__completion-label">{{ option.label }}</span>
        <span v-if="option.detail" class="web-session-composer-editor__completion-detail">
          {{ option.detail }}
        </span>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, reactive, ref, watch } from 'vue';
import type { CodexSkillSummary } from '@/types/models';
import { filterCodexSkills } from '@/components/web-session/webSessionCodexSkills';
import type {
  WebSessionComposerEditorExposed,
  WebSessionComposerSelection,
} from '@/components/web-session/webSessionComposerEditor';

interface CompletionOption {
  key: string;
  label: string;
  detail: string;
  apply: string;
}

const props = withDefaults(
  defineProps<{
    modelValue: string;
    placeholder?: string;
    minRows?: number;
    maxRows?: number;
    compact?: boolean;
    skills?: CodexSkillSummary[];
  }>(),
  {
    placeholder: '',
    minRows: 3,
    maxRows: 10,
    compact: false,
    skills: () => [],
  }
);

const emit = defineEmits<{
  (event: 'update:modelValue', value: string): void;
  (event: 'submit'): void;
  (event: 'focus'): void;
  (event: 'blur'): void;
}>();

const textareaRef = ref<HTMLTextAreaElement | null>(null);
const isComposing = ref(false);
let suppressNextCompletionRefresh = false;
const completionState = reactive({
  open: false,
  from: 0,
  to: 0,
  selectedIndex: 0,
  options: [] as CompletionOption[],
});

const editorStyleVars = computed(() => ({
  '--composer-editor-min-rows': String(Math.max(1, props.minRows)),
  '--composer-editor-max-rows': String(Math.max(props.minRows, props.maxRows)),
  '--composer-editor-padding-top': props.compact ? '8px' : '10px',
  '--composer-editor-padding-bottom': props.compact ? '8px' : '12px',
  '--composer-editor-extra-height': props.compact ? '24px' : '28px',
}));

function closeCompletion() {
  completionState.open = false;
  completionState.options = [];
  completionState.selectedIndex = 0;
}

function syncTextareaHeight() {
  const textarea = textareaRef.value;
  if (!textarea) {
    return;
  }

  textarea.style.height = 'auto';
  const styles = window.getComputedStyle(textarea);
  const minHeight = Number.parseFloat(styles.minHeight || '0') || 0;
  const maxHeight = Number.parseFloat(styles.maxHeight || '0') || Number.POSITIVE_INFINITY;
  const nextHeight = Math.max(minHeight, Math.min(textarea.scrollHeight, maxHeight));
  textarea.style.height = `${nextHeight}px`;
  textarea.style.overflowY = textarea.scrollHeight > maxHeight ? 'auto' : 'hidden';
}

function getTextareaSelection(): WebSessionComposerSelection {
  const textarea = textareaRef.value;
  if (!textarea) {
    const length = String(props.modelValue || '').length;
    return {
      start: length,
      end: length,
    };
  }

  return {
    start: textarea.selectionStart,
    end: textarea.selectionEnd,
  };
}

function buildCompletionOptions(text: string, cursor: number) {
  const beforeCursor = text.slice(0, cursor);
  const slashMatch = beforeCursor.match(/^\/[a-z-]*$/i);
  if (slashMatch) {
    const query = slashMatch[0].slice(1).toLowerCase();
    const options: CompletionOption[] = [
      { key: 'slash:goal', label: '/goal', detail: 'Persistent goal', apply: '/goal ' },
    ].filter(option => option.label.slice(1).startsWith(query));
    return {
      from: 0,
      to: cursor,
      options,
    };
  }

  const skillMatch = beforeCursor.match(/\$[a-z0-9._-]*$/i);
  if (skillMatch) {
    const query = skillMatch[0].slice(1);
    const options = filterCodexSkills(props.skills, query)
      .slice(0, 12)
      .map(skill => {
        const details = [
          skill.displayName && skill.displayName !== skill.name ? skill.displayName : '',
          skill.source,
        ]
          .map(value => String(value || '').trim())
          .filter(Boolean);
        return {
          key: `skill:${skill.name}`,
          label: `$${skill.name}`,
          detail: details.join(' · '),
          apply: `$${skill.name}`,
        };
      });
    return {
      from: cursor - skillMatch[0].length,
      to: cursor,
      options,
    };
  }

  return {
    from: cursor,
    to: cursor,
    options: [],
  };
}

function refreshCompletion() {
  if (suppressNextCompletionRefresh) {
    suppressNextCompletionRefresh = false;
    closeCompletion();
    return;
  }

  if (isComposing.value) {
    closeCompletion();
    return;
  }

  const textarea = textareaRef.value;
  if (!textarea || textarea.selectionStart !== textarea.selectionEnd) {
    closeCompletion();
    return;
  }

  const result = buildCompletionOptions(props.modelValue, textarea.selectionStart);
  if (result.options.length === 0) {
    closeCompletion();
    return;
  }

  completionState.open = true;
  completionState.from = result.from;
  completionState.to = result.to;
  completionState.options = result.options;
  completionState.selectedIndex = Math.min(
    completionState.selectedIndex,
    result.options.length - 1
  );
}

function applyCompletion(option: CompletionOption) {
  const text = String(props.modelValue ?? '');
  const nextValue = `${text.slice(0, completionState.from)}${option.apply}${text.slice(completionState.to)}`;
  const cursor = completionState.from + option.apply.length;
  suppressNextCompletionRefresh = true;
  emit('update:modelValue', nextValue);
  closeCompletion();
  nextTick(() => {
    setSelectionRange(cursor, cursor);
    syncTextareaHeight();
  });
}

function handleInput(event: Event) {
  const target = event.target as HTMLTextAreaElement | null;
  emit('update:modelValue', target?.value ?? '');
  nextTick(() => {
    syncTextareaHeight();
    refreshCompletion();
  });
}

function handleFocus() {
  emit('focus');
  refreshCompletion();
}

function handleBlur() {
  emit('blur');
  closeCompletion();
}

function handleCompositionStart() {
  isComposing.value = true;
  closeCompletion();
}

function handleCompositionEnd() {
  isComposing.value = false;
  nextTick(refreshCompletion);
}

function isPlainEnter(event: KeyboardEvent) {
  return (
    event.key === 'Enter' && !event.altKey && !event.ctrlKey && !event.metaKey && !event.shiftKey
  );
}

function handleKeydown(event: KeyboardEvent) {
  if (isComposing.value || event.isComposing || event.keyCode === 229) {
    return;
  }

  if (completionState.open) {
    if (event.key === 'ArrowDown') {
      event.preventDefault();
      completionState.selectedIndex =
        (completionState.selectedIndex + 1) % completionState.options.length;
      return;
    }
    if (event.key === 'ArrowUp') {
      event.preventDefault();
      completionState.selectedIndex =
        (completionState.selectedIndex - 1 + completionState.options.length) %
        completionState.options.length;
      return;
    }
    if (event.key === 'Escape') {
      event.preventDefault();
      closeCompletion();
      return;
    }
    if (isPlainEnter(event)) {
      event.preventDefault();
      applyCompletion(completionState.options[completionState.selectedIndex]);
      return;
    }
  }

  if (isPlainEnter(event)) {
    event.preventDefault();
    emit('submit');
  }
}

function focus() {
  textareaRef.value?.focus();
}

function getSelectionRange(): WebSessionComposerSelection {
  return getTextareaSelection();
}

function setSelectionRange(start: number, end = start) {
  const textarea = textareaRef.value;
  if (!textarea) {
    return;
  }

  const length = textarea.value.length;
  const anchor = Math.max(0, Math.min(start, length));
  const head = Math.max(0, Math.min(end, length));
  textarea.setSelectionRange(anchor, head);
  textarea.focus();
  refreshCompletion();
}

watch(
  () => props.modelValue,
  () => {
    nextTick(() => {
      syncTextareaHeight();
      refreshCompletion();
    });
  }
);

watch(
  () => [props.minRows, props.maxRows, props.compact],
  () => {
    nextTick(syncTextareaHeight);
  }
);

watch(
  () => props.skills,
  () => {
    refreshCompletion();
  },
  { deep: true }
);

onMounted(() => {
  syncTextareaHeight();
});

defineExpose<WebSessionComposerEditorExposed>({
  focus,
  getSelectionRange,
  setSelectionRange,
});
</script>

<style scoped>
.web-session-composer-editor {
  position: relative;
  width: 100%;
  min-width: 0;
}

.web-session-composer-editor__textarea {
  display: block;
  width: 100%;
  min-width: 0;
  min-height: calc(
    var(--composer-editor-font-size, 14px) * 1.68 * var(--composer-editor-min-rows) +
      var(--composer-editor-extra-height, 28px)
  );
  max-height: calc(
    var(--composer-editor-font-size, 14px) * 1.68 * var(--composer-editor-max-rows) +
      var(--composer-editor-extra-height, 28px)
  );
  padding: var(--composer-editor-padding-top, 10px) 0 var(--composer-editor-padding-bottom, 12px);
  border: 0;
  outline: none;
  resize: none;
  box-sizing: border-box;
  background: transparent;
  color: var(--n-text-color);
  font: inherit;
  font-size: 14px;
  line-height: 1.68;
}

.web-session-composer-editor__textarea::placeholder {
  color: var(--n-text-color-3, #999);
}

.web-session-composer-editor__textarea:focus::placeholder {
  opacity: 0;
}

.web-session-composer-editor__completion {
  position: absolute;
  left: 0;
  bottom: calc(100% + 6px);
  z-index: 20;
  width: min(360px, 88vw);
  max-height: min(40vh, 320px);
  overflow: auto;
  padding: 6px;
  border: 1px solid color-mix(in srgb, var(--n-border-color) 82%, transparent);
  border-radius: 8px;
  background: var(--app-surface-color, #fff);
  box-shadow: 0 14px 28px rgba(15, 23, 42, 0.12);
}

.web-session-composer-editor__completion-option {
  width: 100%;
  min-height: 40px;
  padding: 9px 12px;
  border: 0;
  border-radius: 5px;
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  gap: 4px 8px;
  background: transparent;
  color: var(--n-text-color);
  font: inherit;
  font-size: 13px;
  line-height: 1.35;
  text-align: left;
  cursor: pointer;
}

.web-session-composer-editor__completion-option:hover,
.web-session-composer-editor__completion-option.is-selected {
  background: color-mix(in srgb, var(--n-primary-color) 12%, transparent);
  color: var(--n-primary-color);
}

.web-session-composer-editor__completion-label {
  flex: 1 1 auto;
  min-width: 0;
  font-weight: 500;
}

.web-session-composer-editor__completion-detail {
  flex: 1 0 100%;
  color: var(--n-text-color-3);
  font-size: 11px;
  line-height: 1.45;
  word-break: break-word;
}
</style>
