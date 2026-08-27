export type TerminalConnectionPolicy = 'active-only' | 'active-plus-mirror';

export const DEFAULT_TERMINAL_CONNECTION_POLICY: TerminalConnectionPolicy = 'active-only';
export const DEFAULT_INACTIVE_TERMINAL_SNAPSHOT_INTERVAL_MS = 2000;

export function sanitizeTerminalConnectionPolicy(value: unknown): TerminalConnectionPolicy {
  return value === 'active-plus-mirror' ? 'active-plus-mirror' : 'active-only';
}
