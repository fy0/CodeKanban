import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import { describe, expect, it, vi } from 'vitest';

import {
  canNavigateWebSessionUserMessage,
  findAdjacentWebSessionUserMessageKey,
  findViewportAdjacentWebSessionUserMessageKey,
  resolveWebSessionTimelineStartConfirmation,
  resolveWebSessionUserMessageTarget,
  type WebSessionUserMessageNavigationBlock,
} from '@/components/web-session/webSessionUserMessageNavigation';

const webSessionPanelPath = fileURLToPath(new URL('../WebSessionPanel.vue', import.meta.url));
const webSessionPanelSource = readFileSync(webSessionPanelPath, 'utf8');
const webSessionTimelineStylePath = fileURLToPath(
  new URL('../styles/webSessionPanelTimeline.css', import.meta.url)
);
const webSessionTimelineStyleSource = readFileSync(webSessionTimelineStylePath, 'utf8');
const webSessionMessageEditDialogPath = fileURLToPath(
  new URL('../WebSessionMessageEditDialog.vue', import.meta.url)
);
const webSessionMessageEditDialogSource = readFileSync(webSessionMessageEditDialogPath, 'utf8');

const blocks: WebSessionUserMessageNavigationBlock[] = [
  { key: 'user-1', kind: 'user' },
  { key: 'assistant-1', kind: 'assistant' },
  { key: 'tool-1', kind: 'tool' },
  { key: 'user-2', kind: 'user' },
  { key: 'system-1', kind: 'system' },
  { key: 'user-3', kind: 'user' },
];

describe('webSessionUserMessageNavigation', () => {
  it('requires two clicks on the same session to jump to the timeline start', () => {
    const first = resolveWebSessionTimelineStartConfirmation({
      sessionId: 'session-1',
      currentState: null,
      now: 1000,
      ttlMs: 5000,
    });
    expect(first).toEqual({
      shouldProceed: false,
      nextState: { sessionId: 'session-1', expiresAt: 6000 },
    });

    expect(
      resolveWebSessionTimelineStartConfirmation({
        sessionId: 'session-1',
        currentState: first.nextState,
        now: 1500,
        ttlMs: 5000,
      })
    ).toEqual({ shouldProceed: true, nextState: null });

    expect(
      resolveWebSessionTimelineStartConfirmation({
        sessionId: 'session-2',
        currentState: first.nextState,
        now: 1500,
        ttlMs: 5000,
      }).shouldProceed
    ).toBe(false);
  });

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
        canLoadLater: true,
      })
    ).toBe(true);
  });

  it('finds viewport-relative user messages without treating an aligned item as adjacent', () => {
    const candidates = [
      { key: 'user-1', top: -500 },
      { key: 'user-2', top: 0 },
      { key: 'user-3', top: 420 },
    ];

    expect(findViewportAdjacentWebSessionUserMessageKey(candidates, 0, 'previous')).toBe('user-1');
    expect(findViewportAdjacentWebSessionUserMessageKey(candidates, 0, 'next')).toBe('user-3');
    expect(findViewportAdjacentWebSessionUserMessageKey(candidates, 240, 'previous')).toBe(
      'user-2'
    );
    expect(findViewportAdjacentWebSessionUserMessageKey(candidates, 240, 'next')).toBe('user-3');
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

  it('loads later chronological pages when navigating forward from the start window', async () => {
    let currentBlocks: WebSessionUserMessageNavigationBlock[] = [
      { key: 'user-current', kind: 'user' },
      { key: 'assistant-current', kind: 'assistant' },
    ];
    let canLoadLater = true;
    let loadState = 0;
    const loadLater = vi.fn(async () => {
      currentBlocks = [...currentBlocks, { key: 'user-next', kind: 'user' }];
      canLoadLater = false;
      loadState += 1;
    });

    await expect(
      resolveWebSessionUserMessageTarget({
        currentKey: 'user-current',
        direction: 'next',
        getBlocks: () => currentBlocks,
        canLoadEarlier: () => false,
        canLoadLater: () => canLoadLater,
        getLoadStateKey: () => String(loadState),
        loadEarlier: vi.fn(async () => undefined),
        loadLater,
      })
    ).resolves.toBe('user-next');
    expect(loadLater).toHaveBeenCalledOnce();
  });

  it('renders always-visible accessible controls before the user role and timestamp', () => {
    const navigationIndex = webSessionPanelSource.indexOf('class="user-message-navigation"');
    const editIndex = webSessionPanelSource.indexOf('<CreateOutline />', navigationIndex);
    const previousIndex = webSessionPanelSource.indexOf('<ChevronUpOutline />', navigationIndex);
    const roleIndex = webSessionPanelSource.indexOf('class="item-role"');
    const timeIndex = webSessionPanelSource.indexOf('class="item-time"');

    expect(navigationIndex).toBeGreaterThan(-1);
    expect(editIndex).toBeGreaterThan(navigationIndex);
    expect(editIndex).toBeLessThan(previousIndex);
    expect(navigationIndex).toBeLessThan(roleIndex);
    expect(roleIndex).toBeLessThan(timeIndex);
    expect(webSessionPanelSource).toContain('<ChevronUpOutline />');
    expect(webSessionPanelSource).toContain('<ChevronDownOutline />');
    expect(webSessionPanelSource).toContain(':aria-label="t(\'terminal.prevUserMessage\')"');
    expect(webSessionPanelSource).toContain(':aria-label="t(\'terminal.nextUserMessage\')"');
    expect(webSessionPanelSource).toContain(
      'class="user-message-navigation-button user-message-edit-button"'
    );
    expect(webSessionMessageEditDialogSource).toContain(
      "t('webSession.editUserMessageWorkspaceWarning')"
    );
  });

  it('keeps the controls compact and makes unavailable directions visibly quieter', () => {
    expect(webSessionTimelineStyleSource).toMatch(
      /\.user-message-navigation\s*\{[^}]*gap:\s*0;[^}]*height:\s*20px;/s
    );
    expect(webSessionTimelineStyleSource).toMatch(
      /\.user-message-navigation-button\s*\{[^}]*width:\s*20px !important;[^}]*height:\s*20px !important;/s
    );
    expect(webSessionTimelineStyleSource).toMatch(
      /\.user-message-navigation-button:disabled:not\(\.n-button--loading\)\s*\{[^}]*opacity:\s*0\.18 !important;/s
    );
  });

  it('renders floating edge controls and jumps to both edges without smooth scrolling', () => {
    expect(webSessionPanelSource).toMatch(
      /v-if="!timelineSearchOpen"\s+class="timeline-navigation-reveal-zone"/
    );
    expect(webSessionPanelSource).toContain('v-if="timelineNavigationControlsExpanded"');
    expect(webSessionPanelSource).toContain('name="timeline-navigation-reveal"');
    expect(webSessionPanelSource).toContain('class="timeline-navigation-activation-zone"');
    expect(webSessionTimelineStyleSource).toMatch(
      /\.timeline-navigation-reveal-zone\s*\{[^}]*flex:\s*0 0 118px;[^}]*width:\s*118px;[^}]*min-width:\s*118px;/s
    );
    expect(webSessionPanelSource).toMatch(
      /v-if="!timelineNavigationControlsExpanded"\s+type="button"\s+class="timeline-navigation-activation-zone"/
    );
    expect(webSessionTimelineStyleSource).toMatch(
      /\.timeline-navigation-activation-zone\s*\{[^}]*width:\s*100%;[^}]*height:\s*28px;/s
    );
    expect(webSessionPanelSource).toContain('@mouseenter="handleTimelineNavigationPointerEnter"');
    expect(webSessionPanelSource).toContain('@focusout="handleTimelineNavigationFocusOut"');
    expect(webSessionPanelSource).toContain('@click="handleTimelineNavigationActivation"');
    expect(webSessionPanelSource).toContain('WEB_SESSION_TIMELINE_NAVIGATION_VISIBLE_MS = 5000');
    expect(webSessionPanelSource).toContain("t('webSession.timelineJumpToStart')");
    expect(webSessionPanelSource).toContain("t('webSession.timelineJumpToEnd')");
    expect(webSessionPanelSource).toContain('@click="void handleTimelineStartClick()"');
    expect(webSessionPanelSource).toContain(':show="timelineStartConfirmationArmed"');

    const startIndex = webSessionPanelSource.indexOf('async function jumpToTimelineStart()');
    const endIndex = webSessionPanelSource.indexOf('async function jumpToTimelineEnd()');
    const loadSearchHistoryIndex = webSessionPanelSource.indexOf(
      'async function loadEarlierTimelineSearchHistory('
    );
    const startFunction = webSessionPanelSource.slice(startIndex, endIndex);
    const endFunction = webSessionPanelSource.slice(endIndex, loadSearchHistoryIndex);

    expect(startFunction).toContain("afterCursor: '0'");
    expect(startFunction).toContain('container.scrollTop = 0');
    expect(startFunction).not.toContain("behavior: 'smooth'");
    expect(endFunction).toContain('loadSessionSnapshot');
    expect(endFunction).toContain('syncScrollToBottom()');
    expect(endFunction).not.toContain("behavior: 'smooth'");
    expect(webSessionPanelSource).toContain('!getCurrentTimelineEdgeWindow()');
    expect(webSessionTimelineStyleSource).toMatch(
      /\.timeline-navigation-button\s*\{[^}]*width:\s*28px !important;[^}]*height:\s*28px !important;/s
    );
    expect(webSessionTimelineStyleSource).toMatch(
      /\.timeline-navigation-reveal-enter-from,[\s\S]*?max-width:\s*0;[\s\S]*?opacity:\s*0;[\s\S]*?transform:\s*translateX\(8px\);/
    );
  });
});
