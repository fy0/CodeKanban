import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import { describe, expect, it, vi } from 'vitest';

import {
  canNavigateWebSessionUserMessage,
  findAdjacentWebSessionUserMessageKey,
  resolveWebSessionUserMessageTarget,
  type WebSessionUserMessageNavigationBlock,
} from '@/components/web-session/webSessionUserMessageNavigation';

const webSessionPanelPath = fileURLToPath(new URL('../WebSessionPanel.vue', import.meta.url));
const webSessionPanelSource = readFileSync(webSessionPanelPath, 'utf8');

const blocks: WebSessionUserMessageNavigationBlock[] = [
  { key: 'user-1', kind: 'user' },
  { key: 'assistant-1', kind: 'assistant' },
  { key: 'tool-1', kind: 'tool' },
  { key: 'user-2', kind: 'user' },
  { key: 'system-1', kind: 'system' },
  { key: 'user-3', kind: 'user' },
];

describe('webSessionUserMessageNavigation', () => {
  it('finds adjacent user messages relative to the clicked message', () => {
    expect(findAdjacentWebSessionUserMessageKey(blocks, 'user-2', 'previous')).toBe('user-1');
    expect(findAdjacentWebSessionUserMessageKey(blocks, 'user-2', 'next')).toBe('user-3');
  });

  it('ignores non-user blocks and handles loaded boundaries', () => {
    expect(findAdjacentWebSessionUserMessageKey(blocks, 'assistant-1', 'previous')).toBeNull();
    expect(findAdjacentWebSessionUserMessageKey(blocks, 'user-1', 'previous')).toBeNull();
    expect(findAdjacentWebSessionUserMessageKey(blocks, 'user-3', 'next')).toBeNull();

    expect(
      canNavigateWebSessionUserMessage({
        blocks,
        currentKey: 'user-1',
        direction: 'previous',
        canLoadEarlier: true,
      })
    ).toBe(true);
    expect(
      canNavigateWebSessionUserMessage({
        blocks,
        currentKey: 'user-3',
        direction: 'next',
        canLoadEarlier: true,
      })
    ).toBe(false);
  });

  it('loads multiple earlier pages until it finds a previous user message', async () => {
    let currentBlocks: WebSessionUserMessageNavigationBlock[] = [
      { key: 'user-current', kind: 'user' },
      { key: 'assistant-current', kind: 'assistant' },
    ];
    const pages: WebSessionUserMessageNavigationBlock[][] = [
      [
        { key: 'system-old', kind: 'system' },
        { key: 'tool-old', kind: 'tool' },
      ],
      [
        { key: 'user-previous', kind: 'user' },
        { key: 'assistant-previous', kind: 'assistant' },
      ],
    ];
    let loadState = 0;
    const loadEarlier = vi.fn(async () => {
      currentBlocks = [...(pages.shift() ?? []), ...currentBlocks];
      loadState += 1;
    });

    await expect(
      resolveWebSessionUserMessageTarget({
        currentKey: 'user-current',
        direction: 'previous',
        getBlocks: () => currentBlocks,
        canLoadEarlier: () => pages.length > 0,
        getLoadStateKey: () => String(loadState),
        loadEarlier,
      })
    ).resolves.toBe('user-previous');
    expect(loadEarlier).toHaveBeenCalledTimes(2);
  });

  it('stops when an earlier-history request makes no progress', async () => {
    const loadEarlier = vi.fn(async () => undefined);

    await expect(
      resolveWebSessionUserMessageTarget({
        currentKey: 'user-1',
        direction: 'previous',
        getBlocks: () => blocks,
        canLoadEarlier: () => true,
        getLoadStateKey: () => 'unchanged',
        loadEarlier,
      })
    ).resolves.toBeNull();
    expect(loadEarlier).toHaveBeenCalledOnce();
  });

  it('never loads earlier history while navigating forward', async () => {
    const loadEarlier = vi.fn(async () => undefined);

    await expect(
      resolveWebSessionUserMessageTarget({
        currentKey: 'user-3',
        direction: 'next',
        getBlocks: () => blocks,
        canLoadEarlier: () => true,
        getLoadStateKey: () => 'unchanged',
        loadEarlier,
      })
    ).resolves.toBeNull();
    expect(loadEarlier).not.toHaveBeenCalled();
  });

  it('renders always-visible accessible controls before the user role and timestamp', () => {
    const navigationIndex = webSessionPanelSource.indexOf('class="user-message-navigation"');
    const roleIndex = webSessionPanelSource.indexOf('class="item-role"');
    const timeIndex = webSessionPanelSource.indexOf('class="item-time"');

    expect(navigationIndex).toBeGreaterThan(-1);
    expect(navigationIndex).toBeLessThan(roleIndex);
    expect(roleIndex).toBeLessThan(timeIndex);
    expect(webSessionPanelSource).toContain('<ChevronUpOutline />');
    expect(webSessionPanelSource).toContain('<ChevronDownOutline />');
    expect(webSessionPanelSource).toContain(':aria-label="t(\'terminal.prevUserMessage\')"');
    expect(webSessionPanelSource).toContain(':aria-label="t(\'terminal.nextUserMessage\')"');
  });

  it('keeps the controls compact and makes unavailable directions visibly quieter', () => {
    expect(webSessionPanelSource).toMatch(
      /\.user-message-navigation\s*\{[^}]*gap:\s*0;[^}]*height:\s*20px;/s
    );
    expect(webSessionPanelSource).toMatch(
      /\.user-message-navigation-button\s*\{[^}]*width:\s*20px !important;[^}]*height:\s*20px !important;/s
    );
    expect(webSessionPanelSource).toMatch(
      /\.user-message-navigation-button:disabled:not\(\.n-button--loading\)\s*\{[^}]*opacity:\s*0\.18 !important;/s
    );
  });
});
