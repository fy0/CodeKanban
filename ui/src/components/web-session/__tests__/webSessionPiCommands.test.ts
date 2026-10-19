import { describe, expect, it } from 'vitest';

import { parseWebSessionPiSlashCommand } from '@/components/web-session/webSessionPiCommands';

describe('Pi slash command parsing', () => {
  it('preserves custom compact instructions', () => {
    expect(parseWebSessionPiSlashCommand(' /compact  Keep the API decisions and risks ')).toEqual({
      kind: 'compact',
      customInstructions: 'Keep the API decisions and risks',
    });
  });

  it('accepts nested provider/model identifiers', () => {
    expect(
      parseWebSessionPiSlashCommand('/model commandcode/deepseek/deepseek-v4.1-flash')
    ).toEqual({
      kind: 'model',
      model: 'commandcode/deepseek/deepseek-v4.1-flash',
    });
  });

  it('maps Pi off to the session none effort', () => {
    expect(parseWebSessionPiSlashCommand('/thinking off')).toEqual({
      kind: 'thinking',
      level: 'none',
    });
  });

  it('maps the native tree command to the Web Session tree drawer', () => {
    expect(parseWebSessionPiSlashCommand('/tree')).toEqual({ kind: 'tree' });
    expect(parseWebSessionPiSlashCommand('/tree extra')).toEqual({
      kind: 'error',
      command: 'tree',
      reason: 'unexpected_argument',
    });
  });

  it('rejects missing and unsupported arguments', () => {
    expect(parseWebSessionPiSlashCommand('/model')).toEqual({
      kind: 'error',
      command: 'model',
      reason: 'missing_argument',
    });
    expect(parseWebSessionPiSlashCommand('/model deepseek-v4')).toEqual({
      kind: 'error',
      command: 'model',
      reason: 'invalid_model',
    });
    expect(parseWebSessionPiSlashCommand('/thinking ultra')).toEqual({
      kind: 'error',
      command: 'thinking',
      reason: 'invalid_level',
    });
    expect(parseWebSessionPiSlashCommand('/name')).toEqual({
      kind: 'error',
      command: 'name',
      reason: 'missing_argument',
    });
  });

  it('leaves extension and skill commands on the normal prompt path', () => {
    expect(parseWebSessionPiSlashCommand('/my-extension arg')).toBeNull();
    expect(parseWebSessionPiSlashCommand('please compact this')).toBeNull();
  });
});
