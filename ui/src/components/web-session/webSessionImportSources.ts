import type { WebSessionSummary } from '@/types/models';

export type WebSessionImportSourceAgent = 'codex' | 'pi';

export type WebSessionImportSourceSummary = {
  agent: WebSessionImportSourceAgent;
  importable: boolean;
  aiSessionId: string;
  sessionId: string;
  model: string;
  title: string | null;
  sessionStartedAt: string;
  lastMessageAt: string | null;
  messageCount: number;
  assistantMessageCount: number;
  filePath: string;
  duplicate: boolean;
  existingSession?: WebSessionSummary | null;
};

export type WebSessionImportSourceWire = Omit<
  WebSessionImportSourceSummary,
  'agent' | 'importable'
> & {
  agent?: string;
  importable?: boolean;
};

export function normalizeWebSessionImportSources(
  value: readonly WebSessionImportSourceWire[] | null | undefined
): WebSessionImportSourceSummary[] {
  if (!Array.isArray(value)) return [];
  return value.map(source => {
    const agent: WebSessionImportSourceAgent = source.agent === 'pi' ? 'pi' : 'codex';
    return {
      ...source,
      agent,
      importable: source.importable ?? agent === 'codex',
    };
  });
}

export function countImportableWebSessionSources(
  sources: readonly WebSessionImportSourceSummary[]
): number {
  return sources.filter(source => source.importable && !source.duplicate).length;
}
