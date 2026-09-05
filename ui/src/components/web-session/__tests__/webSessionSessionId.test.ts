import { describe, expect, it } from 'vitest';

import { resolveCopyableAgentSessionId } from '../webSessionSessionId';

describe('Web Session agent session ID', () => {
  it.each(['codex', 'claude', 'pi'] as const)(
    'allows copying a normalized %s native session ID',
    agent => {
      expect(
        resolveCopyableAgentSessionId(
          {
            agent,
            nativeSessionId: '  native-session-id  ',
          },
          false
        )
      ).toBe('native-session-id');
    }
  );

  it('rejects draft sessions and missing native session IDs', () => {
    expect(
      resolveCopyableAgentSessionId({ agent: 'codex', nativeSessionId: 'native-session-id' }, true)
    ).toBe('');
    expect(resolveCopyableAgentSessionId({ agent: 'codex', nativeSessionId: null }, false)).toBe(
      ''
    );
    expect(resolveCopyableAgentSessionId({ agent: 'codex', nativeSessionId: '   ' }, false)).toBe(
      ''
    );
  });
});
