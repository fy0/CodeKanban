import { describe, expect, it } from 'vitest';

import { resolveCopyableAbsoluteHref, resolveNavigableHref } from '@/utils/messageLinkNavigation';

const BASE_URL = 'http://127.0.0.1:6022/home/dev/CodeKanban/ui/';

describe('messageLinkNavigation', () => {
  it('allows absolute https links', () => {
    expect(resolveNavigableHref('https://example.com/docs', BASE_URL)).toBe(
      'https://example.com/docs'
    );
  });

  it('resolves same-origin relative paths with fragments', () => {
    expect(
      resolveNavigableHref('/home/dev/CodeKanban/ui/src/stores/webSession.ts#L1539', BASE_URL)
    ).toBe('http://127.0.0.1:6022/home/dev/CodeKanban/ui/src/stores/webSession.ts#L1539');
  });

  it('rejects empty and hash-only links', () => {
    expect(resolveNavigableHref('', BASE_URL)).toBeNull();
    expect(resolveNavigableHref('   ', BASE_URL)).toBeNull();
    expect(resolveNavigableHref('#section-1', BASE_URL)).toBeNull();
  });

  it('rejects unsupported protocols', () => {
    expect(resolveNavigableHref('javascript:alert(1)', BASE_URL)).toBeNull();
    expect(resolveNavigableHref('data:text/plain,hello', BASE_URL)).toBeNull();
    expect(resolveNavigableHref('mailto:test@example.com', BASE_URL)).toBeNull();
  });

  it('only allows absolute http and https links for copy buttons', () => {
    expect(resolveCopyableAbsoluteHref('http://10.128.128.111:6032/')).toBe(
      'http://10.128.128.111:6032/'
    );
    expect(resolveCopyableAbsoluteHref('https://example.com/docs')).toBe(
      'https://example.com/docs'
    );
    expect(
      resolveCopyableAbsoluteHref('/home/dev/CodeKanban/ui/src/stores/webSession.ts')
    ).toBeNull();
    expect(resolveCopyableAbsoluteHref('mailto:test@example.com')).toBeNull();
    expect(resolveCopyableAbsoluteHref('')).toBeNull();
  });
});
