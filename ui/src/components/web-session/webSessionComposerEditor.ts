import type { JSONContent } from '@tiptap/core';

import type { CodexSkillSummary } from '@/types/models';
import { filterCodexSkills } from '@/components/web-session/webSessionCodexSkills';

export interface WebSessionComposerSelection {
  start: number;
  end: number;
}

export interface WebSessionComposerEditorExposed {
  focus: () => void;
  getSelectionRange: () => WebSessionComposerSelection;
  setSelectionRange: (start: number, end?: number) => void;
}

export interface WebSessionComposerCompletionOption {
  key: string;
  label: string;
  detail: string;
  apply: string;
}

export interface WebSessionComposerCompletionResult {
  from: number;
  to: number;
  options: WebSessionComposerCompletionOption[];
}

export type WebSessionComposerHighlightKind = 'skill' | 'unknown-skill' | 'goal' | 'compact';

export interface WebSessionComposerHighlightRange {
  from: number;
  to: number;
  kind: WebSessionComposerHighlightKind;
}

export type WebSessionComposerKeyAction =
  | 'none'
  | 'completion-next'
  | 'completion-previous'
  | 'completion-close'
  | 'completion-apply'
  | 'submit'
  | 'hard-break';

export interface WebSessionComposerKeyInput {
  key: string;
  altKey?: boolean;
  ctrlKey?: boolean;
  metaKey?: boolean;
  shiftKey?: boolean;
  isComposing?: boolean;
  keyCode?: number;
  completionOpen?: boolean;
}

export type WebSessionComposerCompositionAction =
  | { type: 'none' }
  | { type: 'apply-external'; value: string }
  | { type: 'emit-local'; value: string };

export function resolveWebSessionComposerCompositionEnd(input: {
  startValue: string;
  localValue: string;
  modelValue: string;
  pendingExternalValue: string | null;
}): WebSessionComposerCompositionAction {
  if (input.localValue !== input.startValue) {
    return input.localValue === input.modelValue
      ? { type: 'none' }
      : { type: 'emit-local', value: input.localValue };
  }

  if (input.pendingExternalValue != null && input.modelValue !== input.localValue) {
    return { type: 'apply-external', value: input.modelValue };
  }

  return input.localValue === input.modelValue
    ? { type: 'none' }
    : { type: 'emit-local', value: input.localValue };
}

export function composerTextToJSON(value: string): JSONContent {
  const text = String(value ?? '');
  const content: JSONContent[] = [];
  let segmentStart = 0;

  for (let index = 0; index < text.length; index += 1) {
    if (text[index] !== '\n') {
      continue;
    }

    if (index > segmentStart) {
      content.push({ type: 'text', text: text.slice(segmentStart, index) });
    }
    content.push({ type: 'hardBreak' });
    segmentStart = index + 1;
  }

  if (segmentStart < text.length) {
    content.push({ type: 'text', text: text.slice(segmentStart) });
  }

  return {
    type: 'doc',
    content: [
      {
        type: 'paragraph',
        ...(content.length > 0 ? { content } : {}),
      },
    ],
  };
}

export function composerJSONToText(value: JSONContent): string {
  const paragraph = value.type === 'doc' ? value.content?.[0] : value;
  if (!paragraph || paragraph.type !== 'paragraph') {
    return '';
  }

  return (paragraph.content ?? [])
    .map(node => {
      if (node.type === 'text') {
        return String(node.text ?? '');
      }
      return node.type === 'hardBreak' ? '\n' : '';
    })
    .join('');
}

export function composerOffsetToPosition(offset: number, textLength: number) {
  const safeLength = Math.max(0, textLength);
  return Math.max(0, Math.min(offset, safeLength)) + 1;
}

export function composerPositionToOffset(position: number, textLength: number) {
  const safeLength = Math.max(0, textLength);
  return Math.max(0, Math.min(position - 1, safeLength));
}

export function buildWebSessionComposerHighlights(
  text: string,
  skills: readonly Pick<CodexSkillSummary, 'name'>[],
  goalEnabled = true
) {
  const knownSkills = new Set(skills.map(skill => skill.name.trim().toLowerCase()).filter(Boolean));
  const ranges: WebSessionComposerHighlightRange[] = [];

  for (const match of String(text ?? '').matchAll(/\$[a-z0-9][a-z0-9._-]*/gi)) {
    const token = match[0];
    const from = match.index;
    ranges.push({
      from,
      to: from + token.length,
      kind: knownSkills.has(token.slice(1).toLowerCase()) ? 'skill' : 'unknown-skill',
    });
  }

  if (goalEnabled) {
    for (const match of String(text ?? '').matchAll(/^\/goal\b/gm)) {
      const from = match.index;
      ranges.push({
        from,
        to: from + match[0].length,
        kind: 'goal',
      });
    }
  }

  for (const match of String(text ?? '').matchAll(/^\/compact(?:\s|$)/gm)) {
    const from = match.index;
    ranges.push({
      from,
      to: from + '/compact'.length,
      kind: 'compact',
    });
  }

  return ranges.sort((left, right) => left.from - right.from || left.to - right.to);
}

export function buildWebSessionComposerCompletions(
  text: string,
  cursor: number,
  skills: CodexSkillSummary[],
  goalEnabled = true
): WebSessionComposerCompletionResult {
  const normalizedText = String(text ?? '');
  const safeCursor = Math.max(0, Math.min(cursor, normalizedText.length));
  const beforeCursor = normalizedText.slice(0, safeCursor);
  const slashMatch = beforeCursor.match(/^\/[a-z-]*$/i);
  if (slashMatch) {
    const query = slashMatch[0].slice(1).toLowerCase();
    const slashOptions: WebSessionComposerCompletionOption[] = [
      ...(goalEnabled
        ? [{ key: 'slash:goal', label: '/goal', detail: 'Persistent goal', apply: '/goal ' }]
        : []),
      {
        key: 'slash:compact',
        label: '/compact',
        detail: 'Summarize conversation',
        apply: '/compact ',
      },
    ];
    const options = slashOptions.filter(option => option.label.slice(1).startsWith(query));
    return {
      from: 0,
      to: safeCursor,
      options,
    };
  }

  const skillMatch = beforeCursor.match(/\$[a-z0-9._-]*$/i);
  if (skillMatch) {
    const query = skillMatch[0].slice(1);
    const options = filterCodexSkills(skills, query)
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
      from: safeCursor - skillMatch[0].length,
      to: safeCursor,
      options,
    };
  }

  return {
    from: safeCursor,
    to: safeCursor,
    options: [],
  };
}

export function resolveWebSessionComposerKeyAction(
  input: WebSessionComposerKeyInput
): WebSessionComposerKeyAction {
  if (input.isComposing || input.keyCode === 229) {
    return 'none';
  }

  if (input.completionOpen) {
    if (input.key === 'ArrowDown') {
      return 'completion-next';
    }
    if (input.key === 'ArrowUp') {
      return 'completion-previous';
    }
    if (input.key === 'Escape') {
      return 'completion-close';
    }
  }

  if (input.key !== 'Enter') {
    return 'none';
  }

  const hasModifier = input.altKey || input.ctrlKey || input.metaKey || input.shiftKey;
  if (hasModifier) {
    return 'hard-break';
  }

  return input.completionOpen ? 'completion-apply' : 'submit';
}
