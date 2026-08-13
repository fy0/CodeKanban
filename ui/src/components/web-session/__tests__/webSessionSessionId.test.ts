import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

import { resolveCopyableAgentSessionId } from '../webSessionSessionId';

const webSessionPanelPath = fileURLToPath(new URL('../WebSessionPanel.vue', import.meta.url));
const webSessionPanelSource = readFileSync(webSessionPanelPath, 'utf8');

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

  it('wires the copy action into the shared desktop and mobile session menu', () => {
    expect(webSessionPanelSource).toContain("key: 'copy-session-id'");
    expect(webSessionPanelSource).toContain("label: t('terminal.copyAISessionId')");
    expect(webSessionPanelSource).toContain('icon: renderDropdownIcon(CopyOutline)');
    expect(webSessionPanelSource).toContain('buildSessionActionOptions(contextMenuSession.value)');
    expect(webSessionPanelSource).toContain('buildSessionActionOptions(currentSession.value)');
  });

  it('copies the native ID with localized success and failure feedback', () => {
    const copyActionSource = webSessionPanelSource.match(
      /if \(action === 'copy-session-id'\)[\s\S]*?(?=\s+if \(action === 'rename'\))/
    )?.[0];

    expect(copyActionSource).toContain(
      'resolveCopyableAgentSessionId(session, isDraftSession(session))'
    );
    expect(copyActionSource).toContain('await copyText(sessionId');
    expect(copyActionSource).toContain("failureMessage: t('terminal.copyFailed')");
    expect(copyActionSource).toContain("successMessage: t('terminal.aiSessionIdCopied')");
  });
});
