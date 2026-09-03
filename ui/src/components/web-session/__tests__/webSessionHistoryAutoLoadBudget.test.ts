import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

import {
  createWebSessionHistoryAutoLoadBudget,
  WEB_SESSION_HISTORY_AUTO_LOAD_LIMIT,
} from '@/components/web-session/webSessionHistoryAutoLoadBudget';

const webSessionPanelPath = fileURLToPath(new URL('../WebSessionPanel.vue', import.meta.url));
const webSessionPanelSource = readFileSync(webSessionPanelPath, 'utf8');

function sourceBetween(start: string, end: string) {
  const startIndex = webSessionPanelSource.indexOf(start);
  const endIndex = webSessionPanelSource.indexOf(end, startIndex + start.length);
  expect(startIndex).toBeGreaterThanOrEqual(0);
  expect(endIndex).toBeGreaterThan(startIndex);
  return webSessionPanelSource.slice(startIndex, endIndex);
}

describe('webSessionHistoryAutoLoadBudget', () => {
  it('allows three automatic history requests and rejects the fourth', () => {
    const budget = createWebSessionHistoryAutoLoadBudget();

    expect(WEB_SESSION_HISTORY_AUTO_LOAD_LIMIT).toBe(3);
    expect(budget.tryConsume()).toBe(true);
    expect(budget.tryConsume()).toBe(true);
    expect(budget.tryConsume()).toBe(true);
    expect(budget.remaining()).toBe(0);
    expect(budget.tryConsume()).toBe(false);
  });

  it('resets the allowance for a new activation', () => {
    const budget = createWebSessionHistoryAutoLoadBudget();

    for (let index = 0; index < WEB_SESSION_HISTORY_AUTO_LOAD_LIMIT; index += 1) {
      expect(budget.tryConsume()).toBe(true);
    }
    budget.reset();

    expect(budget.remaining()).toBe(WEB_SESSION_HISTORY_AUTO_LOAD_LIMIT);
    expect(budget.tryConsume()).toBe(true);
  });

  it('shares attempts between position restoration and viewport filling', () => {
    const budget = createWebSessionHistoryAutoLoadBudget();
    const restorePosition = () => budget.tryConsume();
    const fillViewport = () => budget.tryConsume();

    expect(restorePosition()).toBe(true);
    expect(restorePosition()).toBe(true);
    expect(fillViewport()).toBe(true);
    expect(restorePosition()).toBe(false);
    expect(fillViewport()).toBe(false);
  });

  it('does not refund an attempt when its request fails', async () => {
    const budget = createWebSessionHistoryAutoLoadBudget();

    expect(budget.tryConsume()).toBe(true);
    await expect(Promise.reject(new Error('request failed'))).rejects.toThrow('request failed');

    expect(budget.remaining()).toBe(WEB_SESSION_HISTORY_AUTO_LOAD_LIMIT - 1);
  });

  it('guards both automatic loaders while leaving explicit history loads unbounded', () => {
    const restoreSource = sourceBetween(
      'async function restorePendingTimelinePosition()',
      'function beginTimelinePositionRestore('
    );
    const fillSource = sourceBetween(
      'function ensureTimelineHistoryFilled()',
      'function recalcTabTitleWidth('
    );
    const scrollSource = sourceBetween(
      'function handleTimelineScroll(event: Event)',
      'function ensureTimelineHistoryFilled()'
    );
    const searchSource = sourceBetween(
      'async function loadEarlierTimelineSearchHistory(',
      'function setTimelineBlockRef('
    );

    expect(restoreSource).toContain('timelineHistoryAutoLoadBudget.tryConsume()');
    expect(restoreSource).toContain('findClosestWebSessionTimelineAnchor(');
    expect(restoreSource).toContain('captureTimelinePosition(projectId, sessionId, true)');
    expect(fillSource).toContain('timelineHistoryAutoLoadBudget.tryConsume()');
    expect(scrollSource).not.toContain('timelineHistoryAutoLoadBudget.tryConsume()');
    expect(searchSource).not.toContain('timelineHistoryAutoLoadBudget.tryConsume()');
    expect(webSessionPanelSource.match(/timelineHistoryAutoLoadBudget\.reset\(\)/g)).toHaveLength(
      2
    );
  });
});
