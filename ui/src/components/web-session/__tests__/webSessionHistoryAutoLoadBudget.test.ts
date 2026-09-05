import { describe, expect, it } from 'vitest';

import {
  createWebSessionHistoryAutoLoadBudget,
  WEB_SESSION_HISTORY_AUTO_LOAD_LIMIT,
} from '@/components/web-session/webSessionHistoryAutoLoadBudget';

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
});
