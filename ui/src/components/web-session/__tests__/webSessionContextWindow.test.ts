import { describe, expect, it } from 'vitest';
import { contextWindowOptions, contextWindowPending } from '../webSessionContextWindow';

describe('context window settings', () => {
  it('uses decimal token presets', () => {
    expect(contextWindowOptions('Default').map(option => option.value)).toEqual([
      0, 512000, 768000, 1000000,
    ]);
  });

  it('compares run settings rather than the effective token window', () => {
    expect(contextWindowPending(512000, 512000)).toBe(false);
    expect(contextWindowPending(768000, 512000)).toBe(true);
    expect(contextWindowPending(0, 512000)).toBe(true);
    expect(contextWindowPending(0, null)).toBe(false);
    expect(contextWindowPending(1000000, null)).toBe(true);
  });
});
