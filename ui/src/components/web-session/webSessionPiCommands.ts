import type { WebSessionReasoningEffort } from '@/types/models';

const PI_THINKING_LEVELS = new Set<WebSessionReasoningEffort>([
  'none',
  'minimal',
  'low',
  'medium',
  'high',
  'xhigh',
  'max',
]);

export type WebSessionPiSlashCommand =
  | {
      kind: 'compact';
      customInstructions: string;
    }
  | {
      kind: 'model';
      model: string;
    }
  | {
      kind: 'thinking';
      level: WebSessionReasoningEffort;
    }
  | {
      kind: 'name';
      title: string;
    }
  | {
      kind: 'tree';
    };

export type WebSessionPiSlashCommandError = {
  kind: 'error';
  command: 'model' | 'thinking' | 'name' | 'tree';
  reason: 'missing_argument' | 'invalid_level' | 'invalid_model' | 'unexpected_argument';
};

export type ParsedWebSessionPiSlashCommand =
  | WebSessionPiSlashCommand
  | WebSessionPiSlashCommandError;

/**
 * Parse the Pi commands that have a direct Web Session equivalent.
 *
 * Unknown commands intentionally return null so Pi extensions and skills can
 * continue through the normal prompt path.
 */
export function parseWebSessionPiSlashCommand(
  input: string
): ParsedWebSessionPiSlashCommand | null {
  const trimmed = input.trim();
  const match = /^\/([a-z][a-z0-9-]*)(?:\s+([\s\S]*))?$/i.exec(trimmed);
  if (!match) {
    return null;
  }

  const command = match[1]!.toLowerCase();
  const argument = (match[2] ?? '').trim();
  switch (command) {
    case 'compact':
      return { kind: 'compact', customInstructions: argument };
    case 'model':
      if (!argument) {
        return { kind: 'error', command, reason: 'missing_argument' };
      }
      return argument.includes('/') && !argument.startsWith('/') && !argument.endsWith('/')
        ? { kind: 'model', model: argument }
        : { kind: 'error', command, reason: 'invalid_model' };
    case 'thinking': {
      if (!argument) {
        return { kind: 'error', command, reason: 'missing_argument' };
      }
      const level = argument.toLowerCase() === 'off' ? 'none' : argument.toLowerCase();
      return PI_THINKING_LEVELS.has(level as WebSessionReasoningEffort)
        ? { kind: 'thinking', level: level as WebSessionReasoningEffort }
        : { kind: 'error', command, reason: 'invalid_level' };
    }
    case 'name':
      return argument
        ? { kind: 'name', title: argument }
        : { kind: 'error', command, reason: 'missing_argument' };
    case 'tree':
      return argument
        ? { kind: 'error', command, reason: 'unexpected_argument' }
        : { kind: 'tree' };
    default:
      return null;
  }
}
