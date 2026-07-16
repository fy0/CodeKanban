import { describe, expect, it } from 'vitest';

import {
  formatWebSessionDateTime,
  formatWebSessionSidebarTime,
  formatWebSessionTimestamp,
} from '@/components/web-session/webSessionTimeFormat';

describe('webSessionTimeFormat', () => {
  it('renders same-day timestamps with time only', () => {
    const now = new Date(2026, 3, 12, 18, 30, 0);
    const timestamp = new Date(2026, 3, 12, 16, 45, 20).getTime();
    const expected = new Intl.DateTimeFormat('zh-CN', {
      timeStyle: 'medium',
    }).format(new Date(timestamp));

    expect(formatWebSessionTimestamp(timestamp, 'zh-CN', now)).toBe(expected);
  });

  it('renders older timestamps with a compact localized date and time', () => {
    const now = new Date(2026, 3, 12, 18, 30, 0);
    const timestamp = new Date(2026, 3, 10, 16, 45, 20).getTime();
    const expected = new Intl.DateTimeFormat('zh-CN', {
      dateStyle: 'short',
      timeStyle: 'medium',
    }).format(new Date(timestamp));

    expect(formatWebSessionTimestamp(timestamp, 'zh-CN', now)).toBe(expected);
  });

  it('uses local calendar day boundaries instead of a rolling 24-hour window', () => {
    const now = new Date(2026, 3, 13, 0, 15, 0);
    const timestamp = new Date(2026, 3, 12, 23, 50, 20).getTime();
    const expected = new Intl.DateTimeFormat('zh-CN', {
      dateStyle: 'short',
      timeStyle: 'medium',
    }).format(new Date(timestamp));

    expect(formatWebSessionTimestamp(timestamp, 'zh-CN', now)).toBe(expected);
  });

  it('returns an empty string for invalid timestamps', () => {
    expect(formatWebSessionTimestamp(Number.NaN, 'zh-CN')).toBe('');
    expect(formatWebSessionTimestamp(0, 'zh-CN')).toBe('');
    expect(formatWebSessionDateTime(-1, 'zh-CN')).toBe('');
  });

  it('formats full datetime titles using the active locale', () => {
    const timestamp = new Date(2026, 3, 10, 16, 45, 20).getTime();
    const zhExpected = new Intl.DateTimeFormat('zh-CN', {
      dateStyle: 'medium',
      timeStyle: 'medium',
    }).format(new Date(timestamp));
    const enExpected = new Intl.DateTimeFormat('en-US', {
      dateStyle: 'medium',
      timeStyle: 'medium',
    }).format(new Date(timestamp));

    expect(formatWebSessionDateTime(timestamp, 'zh-CN')).toBe(zhExpected);
    expect(formatWebSessionDateTime(timestamp, 'en-US')).toBe(enExpected);
  });

  it('formats compact sidebar timestamps by local calendar date', () => {
    const now = new Date(2026, 6, 16, 18, 30, 0);

    expect(formatWebSessionSidebarTime(new Date(2026, 6, 16, 9, 8).getTime(), now)).toBe('09:08');
    expect(formatWebSessionSidebarTime(new Date(2026, 6, 15, 23, 50).getTime(), now)).toBe('23:50');
    expect(formatWebSessionSidebarTime(new Date(2026, 6, 15, 23, 50).getTime(), now, false)).toBe(
      '7/15'
    );
    expect(formatWebSessionSidebarTime(new Date(2026, 6, 10, 23, 50).getTime(), now)).toBe('7/10');
    expect(formatWebSessionSidebarTime(new Date(2025, 11, 31, 23, 50).getTime(), now)).toBe(
      '25/12/31'
    );
    expect(formatWebSessionSidebarTime(Number.NaN, now)).toBe('');
  });
});
