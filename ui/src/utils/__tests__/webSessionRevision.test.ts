import { describe, expect, it } from 'vitest';

import {
  compareWebSessionAttentionRevisions,
  compareWebSessionRevisions,
  normalizeWebSessionAttentionRevision,
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

  it('normalizes and compares non-negative attention revisions', () => {
    expect(normalizeWebSessionAttentionRevision('000')).toBe('0');
    expect(normalizeWebSessionAttentionRevision('00042')).toBe('42');
    expect(normalizeWebSessionAttentionRevision('9223372036854775807')).toBe('9223372036854775807');
    expect(normalizeWebSessionAttentionRevision('-1')).toBe('');
    expect(compareWebSessionAttentionRevisions('0', '0')).toBe(0);
    expect(compareWebSessionAttentionRevisions('9223372036854775807', '42')).toBe(1);
    expect(compareWebSessionAttentionRevisions('42', '9223372036854775807')).toBe(-1);
    expect(compareWebSessionAttentionRevisions('', '0')).toBeNull();
  });
});
