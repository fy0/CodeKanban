import { describe, expect, it } from 'vitest';

import {
  compareWebSessionRevisions,
  normalizeWebSessionRevision,
} from '@/utils/webSessionRevision';

describe('webSessionRevision', () => {
  it('normalizes positive decimal revisions without losing precision', () => {
    expect(normalizeWebSessionRevision('00042')).toBe('42');
    expect(normalizeWebSessionRevision('9223372036854775807')).toBe('9223372036854775807');
    expect(normalizeWebSessionRevision('0')).toBe('');
    expect(normalizeWebSessionRevision('not-a-revision')).toBe('');
  });

  it('compares revisions as bigint values', () => {
    expect(compareWebSessionRevisions('10', '9')).toBe(1);
    expect(compareWebSessionRevisions('9', '10')).toBe(-1);
    expect(compareWebSessionRevisions('10', '10')).toBe(0);
    expect(compareWebSessionRevisions('', '10')).toBeNull();
  });
});
