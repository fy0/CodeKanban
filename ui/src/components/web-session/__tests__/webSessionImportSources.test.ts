import { describe, expect, it } from 'vitest';

import {
  countImportableWebSessionSources,
  normalizeWebSessionImportSources,
  type WebSessionImportSourceWire,
} from '../webSessionImportSources';

function source(overrides: Partial<WebSessionImportSourceWire> = {}): WebSessionImportSourceWire {
  return {
    aiSessionId: 'ai-1',
    sessionId: 'native-1',
    model: 'openai/gpt-5',
    title: 'Session',
    sessionStartedAt: '2026-08-01T00:00:00Z',
    lastMessageAt: '2026-08-01T00:01:00Z',
    messageCount: 1,
    assistantMessageCount: 1,
    filePath: '/tmp/session.jsonl',
    duplicate: false,
    ...overrides,
  };
}

describe('Web Session import sources', () => {
  it('honors the service importability contract for Pi sources', () => {
    const [legacyPi] = normalizeWebSessionImportSources([
      source({ agent: 'pi', importable: false }),
    ]);
    const [rpcPi] = normalizeWebSessionImportSources([source({ agent: 'pi', importable: true })]);
    expect(legacyPi).toMatchObject({ agent: 'pi', importable: false });
    expect(rpcPi).toMatchObject({ agent: 'pi', importable: true });
    expect(countImportableWebSessionSources([legacyPi, rpcPi])).toBe(1);
  });

  it('keeps old Codex-only services importable', () => {
    const [codex] = normalizeWebSessionImportSources([source()]);
    expect(codex).toMatchObject({ agent: 'codex', importable: true });
    expect(countImportableWebSessionSources([codex])).toBe(1);
  });

  it('does not count duplicates as importable actions', () => {
    const [codex] = normalizeWebSessionImportSources([source({ duplicate: true })]);
    expect(countImportableWebSessionSources([codex])).toBe(0);
  });
});
