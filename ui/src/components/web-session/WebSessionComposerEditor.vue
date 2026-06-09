<template>
  <div class="web-session-composer-editor" :style="editorStyleVars">
    <div ref="editorHostRef" class="web-session-composer-editor__host"></div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue';
import type { CodexSkillSummary } from '@/types/models';
import {
  CODEX_SKILL_TOKEN_PATTERN,
  filterCodexSkills,
} from '@/components/web-session/webSessionCodexSkills';
import type {
  WebSessionComposerEditorExposed,
  WebSessionComposerSelection,
} from '@/components/web-session/webSessionComposerEditor';

type CodeMirrorBundle = Awaited<ReturnType<typeof loadCodeMirrorBundle>>;
type CodeMirrorView = InstanceType<CodeMirrorBundle['EditorView']>;
type CodeMirrorCompartment = InstanceType<CodeMirrorBundle['Compartment']>;

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

const editorHostRef = ref<HTMLDivElement | null>(null);
const editorRef = shallowRef<CodeMirrorView | null>(null);
const codeMirrorRef = shallowRef<CodeMirrorBundle | null>(null);
const placeholderCompartment = shallowRef<CodeMirrorCompartment | null>(null);
const skillCompartment = shallowRef<CodeMirrorCompartment | null>(null);
let applyingExternalValue = false;

const editorStyleVars = computed(() => ({
  '--composer-editor-min-rows': String(Math.max(1, props.minRows)),
  '--composer-editor-max-rows': String(Math.max(props.minRows, props.maxRows)),
  '--composer-editor-padding-top': props.compact ? '8px' : '10px',
  '--composer-editor-padding-bottom': props.compact ? '8px' : '12px',
  '--composer-editor-extra-height': props.compact ? '24px' : '28px',
}));

async function loadCodeMirrorBundle() {
  const [
    { EditorState, Compartment },
    { defaultKeymap, history, historyKeymap },
    autocomplete,
    view,
  ] = await Promise.all([
    import('@codemirror/state'),
    import('@codemirror/commands'),
    import('@codemirror/autocomplete'),
    import('@codemirror/view'),
  ]);

  return {
    EditorState,
    Compartment,
    defaultKeymap,
    history,
    historyKeymap,
    autocompletion: autocomplete.autocompletion,
    completionKeymap: autocomplete.completionKeymap,
    EditorView: view.EditorView,
    keymap: view.keymap,
    placeholder: view.placeholder,
    ViewPlugin: view.ViewPlugin,
    Decoration: view.Decoration,
    MatchDecorator: view.MatchDecorator,
  };
}

function buildSkillCompletionExtension(codeMirror: CodeMirrorBundle) {
  const source = (context: any) => {
    const slashMatch = context.matchBefore(/\/[a-z-]*$/i);
    if (slashMatch && slashMatch.from === 0) {
      const query = slashMatch.text.slice(1).toLowerCase();
      const commands = [
        {
          label: 'goal',
          displayLabel: '/goal',
          detail: 'Persistent goal',
          description: 'Set or update the persistent Codex thread goal.',
          apply: 'goal ',
        },
      ].filter(command => command.label.startsWith(query));

      if (commands.length > 0) {
        return {
          from: slashMatch.from + 1,
          options: commands.map(command => ({
            label: command.label,
            displayLabel: command.displayLabel,
            detail: command.detail,
            type: 'keyword',
            apply: command.apply,
            info: () => {
              const wrapper = document.createElement('div');
              wrapper.className = 'cm-skill-completion-info';

              const title = document.createElement('div');
              title.className = 'cm-skill-completion-info-title';
              title.textContent = command.displayLabel;
              wrapper.appendChild(title);

              const description = document.createElement('div');
              description.className = 'cm-skill-completion-info-description';
              description.textContent = command.description;
              wrapper.appendChild(description);

              return wrapper;
            },
          })),
        };
      }
    }

    const match = context.matchBefore(/\$[a-z0-9._-]*$/i);
    if (!match) {
      return null;
    }

    const query = match.text.slice(1);
    const matches = filterCodexSkills(props.skills, query).slice(0, 12);
    if (matches.length === 0) {
      return null;
    }

    return {
      from: match.from + 1,
      options: matches.map(skill => ({
        label: skill.name,
        displayLabel: `$${skill.name}`,
        type: 'keyword',
        info:
          skill.description || skill.displayName
            ? () => {
                const wrapper = document.createElement('div');
                wrapper.className = 'cm-skill-completion-info';

                if (skill.displayName) {
                  const title = document.createElement('div');
                  title.className = 'cm-skill-completion-info-title';
                  title.textContent = skill.displayName;
                  wrapper.appendChild(title);
                }

                if (skill.description) {
                  const description = document.createElement('div');
                  description.className = 'cm-skill-completion-info-description';
                  description.textContent = skill.description;
                  wrapper.appendChild(description);
                }

                return wrapper;
              }
            : undefined,
      })),
    };
  };

  const skillNames = new Set(
    props.skills.map(skill => skill.name.trim().toLowerCase()).filter(Boolean)
  );
  const decorator = new codeMirror.MatchDecorator({
    regexp: CODEX_SKILL_TOKEN_PATTERN,
    decoration: match =>
      codeMirror.Decoration.mark({
        class: skillNames.has(match[0].slice(1).toLowerCase())
          ? 'cm-skill-token'
          : 'cm-skill-token cm-skill-token--unknown',
      }),
  });

  const skillDecorations = codeMirror.ViewPlugin.fromClass(
    class {
      decorations;

      constructor(view: CodeMirrorView) {
        this.decorations = decorator.createDeco(view);
      }

      update(update: any) {
        this.decorations = decorator.updateDeco(update, this.decorations);
      }
    },
    {
      decorations: value => value.decorations,
    }
  );

  return [
    codeMirror.autocompletion({
      activateOnTyping: true,
      icons: false,
      tooltipClass: () => 'cm-skill-autocomplete',
      optionClass: () => 'cm-skill-completion-option',
      addToOptions: [
        {
          position: 75,
          render: completion => {
            const skill = props.skills.find(item => item.name === completion.label);
            if (!skill) {
              return null;
            }

            const details: string[] = [];
            if (skill.displayName && skill.displayName.trim() !== skill.name.trim()) {
              details.push(skill.displayName.trim());
            }
            if (skill.source) {
              details.push(skill.source);
            }
            if (details.length === 0) {
              return null;
            }

            const subtitle = document.createElement('div');
            subtitle.className = 'cm-skill-completion-subtitle';
            subtitle.textContent = details.join(' · ');
            return subtitle;
          },
        },
      ],
      override: [source],
    }),
    skillDecorations,
  ];
}

function buildSlashCommandHighlightExtension(codeMirror: CodeMirrorBundle) {
  return codeMirror.ViewPlugin.fromClass(
    class {
      decorations;

      constructor(view: CodeMirrorView) {
        this.decorations = this.buildDecorations(view);
      }

      update(update: any) {
        if (update.docChanged || update.viewportChanged) {
          this.decorations = this.buildDecorations(update.view);
        }
      }

      buildDecorations(view: CodeMirrorView) {
        const builder: Array<ReturnType<typeof codeMirror.Decoration.mark>["range"]> = [];
        const commandMark = codeMirror.Decoration.mark({
          class: 'cm-slash-command cm-slash-command--goal',
        });
        const argMark = codeMirror.Decoration.mark({
          class: 'cm-slash-command-arg cm-slash-command-arg--goal',
        });

        for (const { from, to } of view.visibleRanges) {
          const text = view.state.doc.sliceString(from, to);
          const regex = /^\/goal\b/gm;
          let match: RegExpExecArray | null;
          while ((match = regex.exec(text))) {
            const raw = match[0] || '';
            const start = from + match.index;
            builder.push(commandMark.range(start, start + raw.length));
          }
        }

        return codeMirror.Decoration.set(builder, true);
      }
    },
    {
      decorations: value => value.decorations,
    }
  );
}

function buildPlaceholderExtension(codeMirror: CodeMirrorBundle) {
  const placeholder = String(props.placeholder || '').trim();
  return placeholder ? codeMirror.placeholder(placeholder) : [];
}

function createEditorTheme(codeMirror: CodeMirrorBundle) {
  return codeMirror.EditorView.theme({
    '&': {
      fontSize: '14px',
      lineHeight: '1.68',
      backgroundColor: 'transparent',
      color: 'var(--n-text-color)',
      minHeight:
        'calc(var(--composer-editor-font-size, 14px) * 1.68 * var(--composer-editor-min-rows) + var(--composer-editor-extra-height, 28px))',
    },
    '&.cm-focused': {
      outline: 'none',
    },
    '.cm-scroller': {
      overflow: 'auto',
      minHeight: 'inherit',
      maxHeight:
        'calc(var(--composer-editor-font-size, 14px) * 1.68 * var(--composer-editor-max-rows) + var(--composer-editor-extra-height, 28px))',
      fontFamily: 'inherit',
    },
    '.cm-content': {
      padding:
        'var(--composer-editor-padding-top, 10px) 0 var(--composer-editor-padding-bottom, 12px)',
      fontFamily: 'inherit',
      minHeight: 'inherit',
      boxSizing: 'border-box',
    },
    '.cm-line': {
      padding: '0',
      minHeight: '1.68em',
    },
    '.cm-tooltip-autocomplete': {
      border: '1px solid color-mix(in srgb, var(--n-border-color) 82%, transparent)',
      borderRadius: '8px',
      backgroundColor: 'var(--app-surface-color, #fff)',
      boxShadow: '0 14px 28px rgba(15, 23, 42, 0.12)',
      overflow: 'hidden',
      fontSize: '13px',
      fontFamily:
        '\"Segoe UI\", \"PingFang SC\", \"Hiragino Sans GB\", \"Microsoft YaHei\", sans-serif',
      padding: '6px',
    },
    '.cm-tooltip-autocomplete ul': {
      fontFamily: 'inherit',
      maxHeight: 'min(40vh, 320px)',
      padding: '4px',
    },
    '.cm-tooltip-autocomplete ul li': {
      minHeight: 'unset',
      padding: '11px 14px',
      borderRadius: '5px',
      display: 'flex',
      flexWrap: 'wrap',
      alignItems: 'flex-start',
      gap: '6px 8px',
      cursor: 'pointer',
      lineHeight: '1.35',
      transition: 'background-color 0.16s ease, color 0.16s ease, box-shadow 0.16s ease',
    },
    '.cm-tooltip-autocomplete ul li:hover': {
      backgroundColor: 'color-mix(in srgb, var(--n-primary-color) 8%, transparent)',
      color: 'color-mix(in srgb, var(--n-primary-color) 88%, #0f172a)',
      boxShadow: 'inset 0 0 0 1px color-mix(in srgb, var(--n-primary-color) 16%, transparent)',
    },
    '.cm-tooltip-autocomplete ul li:hover .cm-skill-completion-subtitle': {
      color: 'color-mix(in srgb, var(--n-primary-color) 78%, #334155)',
    },
    '.cm-tooltip-autocomplete ul li:active': {
      backgroundColor: 'color-mix(in srgb, var(--n-primary-color) 14%, transparent)',
    },
    '.cm-completionLabel': {
      flex: '1 1 auto',
      minWidth: '0',
      fontSize: '13px',
      fontWeight: '500',
      lineHeight: '1.35',
    },
    '.cm-tooltip-autocomplete ul li[aria-selected]': {
      backgroundColor: 'color-mix(in srgb, var(--n-primary-color) 12%, transparent)',
      color: 'var(--n-primary-color)',
    },
    '.cm-tooltip-autocomplete ul li[aria-selected] .cm-skill-completion-subtitle': {
      color: 'color-mix(in srgb, var(--n-primary-color) 84%, #1f2937)',
    },
  });
}

function createEditor() {
  const codeMirror = codeMirrorRef.value;
  const host = editorHostRef.value;
  if (!codeMirror || !host || editorRef.value) {
    return;
  }

  const placeholderScope = new codeMirror.Compartment();
  const skillScope = new codeMirror.Compartment();
  placeholderCompartment.value = placeholderScope;
  skillCompartment.value = skillScope;

  const submitKeymap = [
    {
      key: 'Enter',
      run: () => {
        emit('submit');
        return true;
      },
    },
  ];

  const editor = new codeMirror.EditorView({
    state: codeMirror.EditorState.create({
      doc: props.modelValue,
      extensions: [
        codeMirror.EditorView.lineWrapping,
        createEditorTheme(codeMirror),
        codeMirror.history(),
        codeMirror.keymap.of([
          ...codeMirror.completionKeymap,
          ...submitKeymap,
          ...codeMirror.historyKeymap,
          ...codeMirror.defaultKeymap,
        ]),
        codeMirror.EditorView.updateListener.of(update => {
          if (update.docChanged) {
            const nextValue = update.state.doc.toString();
            if (!applyingExternalValue && nextValue !== props.modelValue) {
              emit('update:modelValue', nextValue);
            }
          }
        }),
        codeMirror.EditorView.domEventHandlers({
          focus: () => {
            emit('focus');
            return false;
          },
          blur: () => {
            emit('blur');
            return false;
          },
        }),
        placeholderScope.of(buildPlaceholderExtension(codeMirror)),
        skillScope.of(buildSkillCompletionExtension(codeMirror)),
        buildSlashCommandHighlightExtension(codeMirror),
      ],
    }),
    parent: host,
  });

  editorRef.value = editor;
}

function focus() {
  editorRef.value?.focus();
}

function getSelectionRange(): WebSessionComposerSelection {
  const editor = editorRef.value;
  if (!editor) {
    return {
      start: String(props.modelValue || '').length,
      end: String(props.modelValue || '').length,
    };
  }

  const selection = editor.state.selection.main;
  return {
    start: selection.from,
    end: selection.to,
  };
}

function setSelectionRange(start: number, end = start) {
  const editor = editorRef.value;
  if (!editor) {
    return;
  }

  const length = editor.state.doc.length;
  const anchor = Math.max(0, Math.min(start, length));
  const head = Math.max(0, Math.min(end, length));
  editor.dispatch({
    selection: {
      anchor,
      head,
    },
  });
  editor.focus();
}

watch(
  () => props.modelValue,
  nextValue => {
    const editor = editorRef.value;
    if (!editor) {
      return;
    }

    const currentValue = editor.state.doc.toString();
    if (nextValue === currentValue) {
      return;
    }

    applyingExternalValue = true;
    editor.dispatch({
      changes: {
        from: 0,
        to: currentValue.length,
        insert: nextValue,
      },
    });
    applyingExternalValue = false;
  }
);

watch(
  () => props.placeholder,
  () => {
    const editor = editorRef.value;
    const codeMirror = codeMirrorRef.value;
    const scope = placeholderCompartment.value;
    if (!editor || !codeMirror || !scope) {
      return;
    }

    editor.dispatch({
      effects: scope.reconfigure(buildPlaceholderExtension(codeMirror)),
    });
  }
);

watch(
  () => props.skills,
  () => {
    const editor = editorRef.value;
    const codeMirror = codeMirrorRef.value;
    const scope = skillCompartment.value;
    if (!editor || !codeMirror || !scope) {
      return;
    }

    editor.dispatch({
      effects: scope.reconfigure(buildSkillCompletionExtension(codeMirror)),
    });
  },
  { deep: true }
);

onMounted(async () => {
  codeMirrorRef.value = await loadCodeMirrorBundle();
  createEditor();
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
  width: 100%;
  min-width: 0;
  min-height: calc(
    var(--composer-editor-font-size, 14px) * 1.68 * var(--composer-editor-min-rows) +
      var(--composer-editor-extra-height, 28px)
  );
}

.web-session-composer-editor__host {
  width: 100%;
  min-height: inherit;
}

.web-session-composer-editor :deep(.cm-editor) {
  width: 100%;
  min-width: 0;
  min-height: inherit;
  background: transparent;
  position: relative;
}

.web-session-composer-editor :deep(.cm-placeholder) {
  color: var(--n-text-color-3, #999);
  transition: opacity 0.12s ease;
}

.web-session-composer-editor :deep(.cm-focused .cm-placeholder) {
  opacity: 0;
}

.web-session-composer-editor :deep(.cm-skill-completion-subtitle) {
  flex: 1 0 100%;
  font-size: 11px;
  line-height: 1.45;
  color: var(--n-text-color-3);
  white-space: normal;
  word-break: break-word;
}

.web-session-composer-editor :deep(.cm-completionMatchedText) {
  text-decoration: none;
  color: inherit;
  background: color-mix(in srgb, var(--n-primary-color) 14%, transparent);
  border-radius: 4px;
  padding: 0 1px;
}

.web-session-composer-editor :deep(.cm-completionInfo) {
  border: 1px solid color-mix(in srgb, var(--n-border-color) 82%, transparent);
  border-radius: 8px;
  background: var(--app-surface-color, #fff);
  box-shadow: 0 14px 28px rgba(15, 23, 42, 0.12);
  padding: 10px 12px;
  max-width: min(320px, 72vw);
}

.web-session-composer-editor :deep(.cm-skill-completion-info) {
  display: grid;
  gap: 6px;
}

.web-session-composer-editor :deep(.cm-skill-completion-info-title) {
  font-size: 12px;
  font-weight: 700;
  line-height: 1.4;
  color: var(--n-text-color);
}

.web-session-composer-editor :deep(.cm-skill-completion-info-description) {
  font-size: 12px;
  line-height: 1.5;
  color: var(--n-text-color-2);
}

.web-session-composer-editor :deep(.cm-skill-token) {
  border-radius: 8px;
  background: color-mix(in srgb, var(--n-primary-color) 10%, transparent);
  color: color-mix(in srgb, var(--n-primary-color) 86%, #0f172a);
  padding: 0 1px;
}

.web-session-composer-editor :deep(.cm-skill-token--unknown) {
  background: color-mix(in srgb, var(--n-border-color) 70%, transparent);
  color: var(--n-text-color-2);
}

.web-session-composer-editor :deep(.cm-slash-command) {
  border-radius: 7px;
  background: color-mix(in srgb, #0f172a 10%, transparent);
  color: #0f172a;
  font-weight: 700;
  padding: 0 2px;
}

.web-session-composer-editor :deep(.cm-slash-command--goal) {
  background: linear-gradient(
    135deg,
    color-mix(in srgb, #f59e0b 28%, transparent) 0%,
    color-mix(in srgb, #f97316 22%, transparent) 100%
  );
  color: color-mix(in srgb, #9a3412 82%, #111827);
}
</style>
