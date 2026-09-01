import { describe, expect, it } from 'vitest';

import { formatWebSessionTokenCount } from '@/components/web-session/webSessionContextDisplay';

describe('webSessionContextDisplay', () => {
  it('uses raw values below one thousand and compact K/M/B values above it', () => {
    expect(formatWebSessionTokenCount(999)).toBe('999');
    expect(formatWebSessionTokenCount(1000)).toBe('1K');
    expect(formatWebSessionTokenCount(125_500)).toBe('125.5K');
    expect(formatWebSessionTokenCount(2_000_000)).toBe('2M');
    expect(formatWebSessionTokenCount(1_250_000_000)).toBe('1.25B');
    expect(formatWebSessionTokenCount(3_000_000_000)).toBe('3B');
    expect(formatWebSessionTokenCount(3_100_000_000)).toBe('3.1B');
    expect(formatWebSessionTokenCount(3_026_292_009)).toBe('3.03B');
    expect(formatWebSessionTokenCount(3_456_000_000)).toBe('3.46B');
  });

  it('returns an independently requested exact value', () => {
    expect(formatWebSessionTokenCount(125_500, true)).toBe(new Intl.NumberFormat().format(125_500));
    expect(formatWebSessionTokenCount(125_500)).toBe('125.5K');
  });
});
