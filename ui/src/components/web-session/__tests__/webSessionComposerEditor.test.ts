import { describe, expect, it } from 'vitest';

import type { CodexSkillSummary } from '@/types/models';
import {
  buildWebSessionComposerCompletions,
  buildWebSessionComposerHighlights,
  composerJSONToText,
  composerOffsetToPosition,
  composerPositionToOffset,
  composerTextToJSON,
  resolveWebSessionComposerKeyAction,
} from '@/components/web-session/webSessionComposerEditor';

function skill(name: string, displayName = name): CodexSkillSummary {
  return {
    name,
    displayName,
    description: `${displayName} description`,
    defaultPrompt: '',
    source: 'user',
  };
}

describe('web session composer plain-text document', () => {
  it.each([
    '',
    'plain text',
    '  leading and trailing  ',
    'first\nsecond',
    'first\n\nthird\n',
    '\n\n',
    '中文输入\nemoji 😀 and e\u0301',
    '<tag attr="value">& literal markup',
  ])('round-trips %j without HTML parsing', text => {
    expect(composerJSONToText(composerTextToJSON(text))).toBe(text);
  });

  it('represents every newline as a hard break in one paragraph', () => {
    expect(composerTextToJSON('a\n\nb')).toEqual({
      type: 'doc',
      content: [
        {
          type: 'paragraph',
          content: [
            { type: 'text', text: 'a' },
            { type: 'hardBreak' },
            { type: 'hardBreak' },
            { type: 'text', text: 'b' },
          ],
        },
      ],
    });
  });

  it('maps UTF-16 text offsets to ProseMirror positions and back', () => {
    const text = 'A😀\n中';
    for (let offset = 0; offset <= text.length; offset += 1) {
      const position = composerOffsetToPosition(offset, text.length);
      expect(composerPositionToOffset(position, text.length)).toBe(offset);
    }
    expect(composerOffsetToPosition(-10, text.length)).toBe(1);
    expect(composerOffsetToPosition(100, text.length)).toBe(text.length + 1);
    expect(composerPositionToOffset(-10, text.length)).toBe(0);
    expect(composerPositionToOffset(100, text.length)).toBe(text.length);
  });
});

describe('web session composer decorations and completions', () => {
  const skills = [skill('openai-docs', 'OpenAI Docs')];

  it('finds known and unknown skills plus line-start goal commands', () => {
    const text = '/goal ship it\nUse $OPENAI-DOCS and $missing\n/goal again';
    const highlights = buildWebSessionComposerHighlights(text, skills).map(range => ({
      token: text.slice(range.from, range.to),
      kind: range.kind,
    }));

    expect(highlights).toEqual([
      { token: '/goal', kind: 'goal' },
      { token: '$OPENAI-DOCS', kind: 'skill' },
      { token: '$missing', kind: 'unknown-skill' },
      { token: '/goal', kind: 'goal' },
    ]);
  });

  it('keeps goal completion at the document start', () => {
    expect(buildWebSessionComposerCompletions('/go', 3, skills)).toMatchObject({
      from: 0,
      to: 3,
      options: [{ label: '/goal', apply: '/goal ' }],
    });
    expect(buildWebSessionComposerCompletions('before /go', 10, skills).options).toEqual([]);
  });

  it('filters skill completions at the current cursor', () => {
    expect(buildWebSessionComposerCompletions('Use $open', 9, skills)).toMatchObject({
      from: 4,
      to: 9,
      options: [
        {
          label: '$openai-docs',
          detail: 'OpenAI Docs · user',
          apply: '$openai-docs',
        },
      ],
    });
  });
});

describe('web session composer keyboard policy', () => {
  it('submits plain Enter and inserts hard breaks for modified Enter', () => {
    expect(resolveWebSessionComposerKeyAction({ key: 'Enter' })).toBe('submit');
    expect(resolveWebSessionComposerKeyAction({ key: 'Enter', shiftKey: true })).toBe('hard-break');
    expect(resolveWebSessionComposerKeyAction({ key: 'Enter', ctrlKey: true })).toBe('hard-break');
    expect(resolveWebSessionComposerKeyAction({ key: 'Enter', metaKey: true })).toBe('hard-break');
    expect(resolveWebSessionComposerKeyAction({ key: 'Enter', altKey: true })).toBe('hard-break');
  });

  it('gives an open completion menu priority over submission', () => {
    expect(resolveWebSessionComposerKeyAction({ key: 'ArrowDown', completionOpen: true })).toBe(
      'completion-next'
    );
    expect(resolveWebSessionComposerKeyAction({ key: 'ArrowUp', completionOpen: true })).toBe(
      'completion-previous'
    );
    expect(resolveWebSessionComposerKeyAction({ key: 'Escape', completionOpen: true })).toBe(
      'completion-close'
    );
    expect(resolveWebSessionComposerKeyAction({ key: 'Enter', completionOpen: true })).toBe(
      'completion-apply'
    );
  });

  it('does not intercept IME confirmation keys', () => {
    expect(resolveWebSessionComposerKeyAction({ key: 'Enter', isComposing: true })).toBe('none');
    expect(resolveWebSessionComposerKeyAction({ key: 'Enter', keyCode: 229 })).toBe('none');
  });
});
