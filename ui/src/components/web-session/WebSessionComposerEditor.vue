<template>
  <div class="web-session-composer-editor" :style="editorStyleVars">
    <EditorContent
      v-if="editorRef"
      :editor="editorRef"
      class="web-session-composer-editor__content"
    />

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
import { Extension, type JSONContent } from '@tiptap/core';
import Document from '@tiptap/extension-document';
import HardBreak from '@tiptap/extension-hard-break';
import Paragraph from '@tiptap/extension-paragraph';
import Text from '@tiptap/extension-text';
import { Placeholder } from '@tiptap/extensions/placeholder';
import { UndoRedo } from '@tiptap/extensions/undo-redo';
import type { Node as ProseMirrorNode, Slice } from '@tiptap/pm/model';
import { Plugin, PluginKey } from '@tiptap/pm/state';
import { Decoration, DecorationSet, type EditorView } from '@tiptap/pm/view';
import { Editor, EditorContent } from '@tiptap/vue-3';
import {
  computed,
  nextTick,
  onBeforeUnmount,
  onMounted,
  reactive,
  ref,
  shallowRef,
  watch,
} from 'vue';

import type { CodexSkillSummary } from '@/types/models';
import {
  buildWebSessionComposerCompletions,
  buildWebSessionComposerHighlights,
  composerJSONToText,
  composerOffsetToPosition,
  composerPositionToOffset,
  composerTextToJSON,
  resolveWebSessionComposerKeyAction,
  type WebSessionComposerCompletionOption,
  type WebSessionComposerEditorExposed,
  type WebSessionComposerSelection,
} from '@/components/web-session/webSessionComposerEditor';

const highlightPluginKey = new PluginKey<DecorationSet>('webSessionComposerHighlights');
const ComposerDocument = Document.extend({
  content: 'paragraph',
});
const ComposerParagraph = Paragraph.extend({
  marks: '',
});

const props = withDefaults(
  defineProps<{
    modelValue: string;
    placeholder?: string;
    minRows?: number;
    maxRows?: number;
    compact?: boolean;
    skills?: CodexSkillSummary[];
    goalEnabled?: boolean;
  }>(),
  {
    placeholder: '',
    minRows: 3,
    maxRows: 10,
    compact: false,
    skills: () => [],
    goalEnabled: true,
  }
);

const emit = defineEmits<{
  (event: 'update:modelValue', value: string): void;
  (event: 'submit'): void;
  (event: 'focus'): void;
  (event: 'blur'): void;
}>();

const editorRef = shallowRef<Editor | null>(null);
const isComposing = ref(false);
let applyingExternalValue = false;
let pendingExternalValue: string | null = null;
let lastLocallyEmittedValue: string | null = null;
let dismissedCompletionSignature = '';
const completionState = reactive({
  open: false,
  from: 0,
  to: 0,
  selectedIndex: 0,
  options: [] as WebSessionComposerCompletionOption[],
});

const editorStyleVars = computed(() => ({
  '--composer-editor-min-rows': String(Math.max(1, props.minRows)),
  '--composer-editor-max-rows': String(Math.max(props.minRows, props.maxRows)),
  '--composer-editor-padding-top': props.compact ? '8px' : '10px',
  '--composer-editor-padding-bottom': props.compact ? '8px' : '12px',
  '--composer-editor-extra-height': props.compact ? '24px' : '28px',
}));

function getEditorText(editor: { getJSON: () => JSONContent } | null = editorRef.value) {
  return editor ? composerJSONToText(editor.getJSON()) : String(props.modelValue ?? '');
}

function buildHighlightDecorations(doc: ProseMirrorNode) {
  const text = composerJSONToText(doc.toJSON() as JSONContent);
  const decorations = buildWebSessionComposerHighlights(text, props.skills, props.goalEnabled).map(
    range => {
      const className =
        range.kind === 'goal'
          ? 'composer-goal-command'
          : range.kind === 'skill'
            ? 'composer-skill-token'
            : 'composer-skill-token composer-skill-token--unknown';
      return Decoration.inline(
        composerOffsetToPosition(range.from, text.length),
        composerOffsetToPosition(range.to, text.length),
        { class: className }
      );
    }
  );
  return DecorationSet.create(doc, decorations);
}

const ComposerHighlights = Extension.create({
  name: 'webSessionComposerHighlights',
  addProseMirrorPlugins() {
    return [
      new Plugin({
        key: highlightPluginKey,
        state: {
          init: (_, state) => buildHighlightDecorations(state.doc),
          apply: (transaction, current) =>
            transaction.docChanged || transaction.getMeta(highlightPluginKey)
              ? buildHighlightDecorations(transaction.doc)
              : current,
        },
        props: {
          decorations: state => highlightPluginKey.getState(state) ?? null,
        },
      }),
    ];
  },
});

function closeCompletion() {
  completionState.open = false;
  completionState.options = [];
  completionState.selectedIndex = 0;
}

function getSelectionRange(editor = editorRef.value): WebSessionComposerSelection {
  if (!editor) {
    const length = String(props.modelValue ?? '').length;
    return { start: length, end: length };
  }

  const textLength = getEditorText(editor).length;
  return {
    start: composerPositionToOffset(editor.state.selection.from, textLength),
    end: composerPositionToOffset(editor.state.selection.to, textLength),
  };
}

function getCompletionSignature(text: string, selection: WebSessionComposerSelection) {
  return `${selection.start}:${selection.end}:${text}`;
}

function refreshCompletion() {
  const editor = editorRef.value;
  if (!editor || isComposing.value || editor.view.composing) {
    closeCompletion();
    return;
  }

  const selection = getSelectionRange(editor);
  if (selection.start !== selection.end) {
    closeCompletion();
    return;
  }

  const text = getEditorText(editor);
  const signature = getCompletionSignature(text, selection);
  if (signature === dismissedCompletionSignature) {
    closeCompletion();
    return;
  }
  dismissedCompletionSignature = '';

  const result = buildWebSessionComposerCompletions(
    text,
    selection.start,
    props.skills,
    props.goalEnabled
  );
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

function inlineContentFromText(text: string) {
  return composerTextToJSON(text).content?.[0]?.content ?? [];
}

function insertPlainText(text: string, from: number, to: number) {
  const editor = editorRef.value;
  if (!editor) {
    return false;
  }

  const currentText = getEditorText(editor);
  const safeFrom = Math.max(0, Math.min(from, currentText.length));
  const safeTo = Math.max(safeFrom, Math.min(to, currentText.length));
  const normalizedText = String(text ?? '').replace(/\r\n?/g, '\n');
  const cursor = safeFrom + normalizedText.length;
  return editor
    .chain()
    .focus()
    .insertContentAt(
      {
        from: composerOffsetToPosition(safeFrom, currentText.length),
        to: composerOffsetToPosition(safeTo, currentText.length),
      },
      inlineContentFromText(normalizedText)
    )
    .setTextSelection(
      composerOffsetToPosition(
        cursor,
        currentText.length - (safeTo - safeFrom) + normalizedText.length
      )
    )
    .run();
}

function applyCompletion(option: WebSessionComposerCompletionOption) {
  const editor = editorRef.value;
  if (!editor) {
    return;
  }

  const from = completionState.from;
  const to = completionState.to;
  closeCompletion();
  insertPlainText(option.apply, from, to);
  const text = getEditorText(editor);
  const selection = getSelectionRange(editor);
  dismissedCompletionSignature = getCompletionSignature(text, selection);
}

function dismissCompletion() {
  const editor = editorRef.value;
  if (editor) {
    dismissedCompletionSignature = getCompletionSignature(
      getEditorText(editor),
      getSelectionRange(editor)
    );
  }
  closeCompletion();
}

function handleEditorKeydown(view: EditorView, event: KeyboardEvent) {
  const action = resolveWebSessionComposerKeyAction({
    key: event.key,
    altKey: event.altKey,
    ctrlKey: event.ctrlKey,
    metaKey: event.metaKey,
    shiftKey: event.shiftKey,
    isComposing: isComposing.value || view.composing || event.isComposing,
    keyCode: event.keyCode,
    completionOpen: completionState.open,
  });

  if (action === 'none') {
    return false;
  }

  event.preventDefault();
  if (action === 'completion-next') {
    completionState.selectedIndex =
      (completionState.selectedIndex + 1) % completionState.options.length;
  } else if (action === 'completion-previous') {
    completionState.selectedIndex =
      (completionState.selectedIndex - 1 + completionState.options.length) %
      completionState.options.length;
  } else if (action === 'completion-close') {
    dismissCompletion();
  } else if (action === 'completion-apply') {
    applyCompletion(completionState.options[completionState.selectedIndex]);
  } else if (action === 'hard-break') {
    editorRef.value?.commands.setHardBreak();
  } else if (action === 'submit') {
    emit('submit');
  }
  return true;
}

function handleEditorPaste(event: ClipboardEvent) {
  if (event.defaultPrevented) {
    return true;
  }

  const clipboardData = event.clipboardData;
  if (!clipboardData) {
    return false;
  }

  event.preventDefault();
  const selection = getSelectionRange();
  insertPlainText(clipboardData.getData('text/plain'), selection.start, selection.end);
  return true;
}

function handleEditorDrop(event: DragEvent) {
  const hasImage = Array.from(event.dataTransfer?.files ?? []).some(file =>
    file.type.toLowerCase().startsWith('image/')
  );
  if (!hasImage) {
    return false;
  }

  event.preventDefault();
  return true;
}

function serializeClipboardText(slice: Slice) {
  return slice.content.textBetween(0, slice.content.size, '\n', (node: ProseMirrorNode) =>
    node.type.name === 'hardBreak' ? '\n' : ''
  );
}

function syncExternalValue(value: string) {
  const editor = editorRef.value;
  if (!editor) {
    return;
  }

  const nextValue = String(value ?? '');
  if (nextValue === getEditorText(editor)) {
    return;
  }

  applyingExternalValue = true;
  editor.commands.setContent(composerTextToJSON(nextValue), {
    emitUpdate: false,
    errorOnInvalidContent: true,
  });
  applyingExternalValue = false;
  dismissedCompletionSignature = '';
  nextTick(refreshCompletion);
}

function focus() {
  editorRef.value?.commands.focus();
}

function setSelectionRange(start: number, end = start) {
  const editor = editorRef.value;
  if (!editor) {
    return;
  }

  const textLength = getEditorText(editor).length;
  const from = composerOffsetToPosition(start, textLength);
  const to = composerOffsetToPosition(end, textLength);
  editor.chain().setTextSelection({ from, to }).focus().run();
  refreshCompletion();
}

watch(
  () => props.modelValue,
  nextValue => {
    const editor = editorRef.value;
    const normalizedValue = String(nextValue ?? '');
    if (normalizedValue === lastLocallyEmittedValue) {
      lastLocallyEmittedValue = null;
      return;
    }
    if (!editor || normalizedValue === getEditorText(editor)) {
      return;
    }
    if (isComposing.value || editor.view.composing) {
      pendingExternalValue = normalizedValue;
      return;
    }
    syncExternalValue(normalizedValue);
  }
);

watch(
  () => props.placeholder,
  nextValue => {
    const editor = editorRef.value;
    if (!editor) {
      return;
    }
    editor.view.dom.setAttribute('aria-label', nextValue || 'Message');
    editor.view.dispatch(editor.state.tr.setMeta('webSessionComposerPlaceholder', true));
  }
);

watch(
  () => props.skills,
  () => {
    const editor = editorRef.value;
    if (!editor) {
      return;
    }
    editor.view.dispatch(editor.state.tr.setMeta(highlightPluginKey, true));
    refreshCompletion();
  },
  { deep: true }
);

onMounted(() => {
  const editor = new Editor({
    content: composerTextToJSON(props.modelValue),
    extensions: [
      ComposerDocument,
      ComposerParagraph,
      Text,
      HardBreak,
      UndoRedo,
      Placeholder.configure({
        placeholder: () => props.placeholder,
        emptyEditorClass: 'is-editor-empty',
        emptyNodeClass: 'is-empty',
      }),
      ComposerHighlights,
    ],
    editorProps: {
      attributes: {
        class: 'web-session-composer-editor__surface',
        role: 'textbox',
        'aria-label': props.placeholder || 'Message',
        'aria-multiline': 'true',
        spellcheck: 'false',
        autocorrect: 'off',
        autocapitalize: 'off',
        autocomplete: 'off',
      },
      handleKeyDown: handleEditorKeydown,
      handlePaste: (_, event) => handleEditorPaste(event),
      handleDrop: (_, event) => handleEditorDrop(event),
      clipboardTextSerializer: serializeClipboardText,
      handleDOMEvents: {
        compositionstart: () => {
          isComposing.value = true;
          closeCompletion();
          return false;
        },
        compositionend: () => {
          isComposing.value = false;
          nextTick(() => {
            if (pendingExternalValue != null) {
              const pending = pendingExternalValue;
              pendingExternalValue = null;
              syncExternalValue(pending);
            } else {
              refreshCompletion();
            }
          });
          return false;
        },
      },
    },
    onUpdate: ({ editor: currentEditor }) => {
      const nextValue = getEditorText(currentEditor);
      if (!applyingExternalValue && nextValue !== props.modelValue) {
        lastLocallyEmittedValue = nextValue;
        emit('update:modelValue', nextValue);
      }
      if (!isComposing.value && !currentEditor.view.composing) {
        nextTick(refreshCompletion);
      }
    },
    onSelectionUpdate: () => {
      if (!isComposing.value) {
        refreshCompletion();
      }
    },
    onFocus: () => {
      emit('focus');
      refreshCompletion();
    },
    onBlur: () => {
      emit('blur');
      closeCompletion();
    },
  });
  editorRef.value = editor;
});

onBeforeUnmount(() => {
  editorRef.value?.destroy();
  editorRef.value = null;
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

.web-session-composer-editor__content {
  width: 100%;
  min-width: 0;
}

.web-session-composer-editor :deep(.web-session-composer-editor__surface) {
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
  overflow-x: hidden;
  overflow-y: auto;
  padding: var(--composer-editor-padding-top, 10px) 0 var(--composer-editor-padding-bottom, 12px);
  border: 0;
  outline: none;
  box-sizing: border-box;
  background: transparent;
  color: var(--n-text-color);
  font: inherit;
  font-size: 14px;
  line-height: 1.68;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.web-session-composer-editor :deep(.web-session-composer-editor__surface p) {
  min-height: 1.68em;
  margin: 0;
}

.web-session-composer-editor
  :deep(.web-session-composer-editor__surface p.is-editor-empty::before) {
  height: 0;
  float: left;
  color: var(--n-text-color-3, #999);
  content: attr(data-placeholder);
  pointer-events: none;
  transition: opacity 0.12s ease;
}

.web-session-composer-editor
  :deep(.web-session-composer-editor__surface.ProseMirror-focused p.is-editor-empty::before) {
  opacity: 0;
}

.web-session-composer-editor :deep(.composer-skill-token) {
  border-radius: 8px;
  background: color-mix(in srgb, var(--n-primary-color) 10%, transparent);
  color: color-mix(in srgb, var(--n-primary-color) 86%, #0f172a);
  padding: 0 1px;
  box-decoration-break: clone;
}

.web-session-composer-editor :deep(.composer-skill-token--unknown) {
  background: color-mix(in srgb, var(--n-border-color) 70%, transparent);
  color: var(--n-text-color-2);
}

.web-session-composer-editor :deep(.composer-goal-command) {
  border-radius: 7px;
  background: linear-gradient(
    135deg,
    color-mix(in srgb, #f59e0b 28%, transparent) 0%,
    color-mix(in srgb, #f97316 22%, transparent) 100%
  );
  color: color-mix(in srgb, #9a3412 82%, #111827);
  font-weight: 700;
  padding: 0 2px;
  box-decoration-break: clone;
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
