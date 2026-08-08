import { describe, expect, it } from 'vitest';

import {
  DEFAULT_WEB_SESSION_ACTIVITY_DISPLAY_MODE,
  isWebSessionActivityDisplayToolKind,
  normalizeWebSessionActivityToolKind,
  sanitizeWebSessionActivityDisplayMode,
  resolveWebSessionActivityDisplayMode,
  shouldUseWebSessionActivityDisplayMode,
} from '@/constants/webSessionActivityDisplayMode';

describe('webSessionActivityDisplayMode', () => {
  it('sanitizes supported display modes', () => {
    expect(sanitizeWebSessionActivityDisplayMode('default')).toBe('default');
    expect(sanitizeWebSessionActivityDisplayMode('text')).toBe('text');
    expect(sanitizeWebSessionActivityDisplayMode('card')).toBe('card');
    expect(sanitizeWebSessionActivityDisplayMode('unknown')).toBe(
      DEFAULT_WEB_SESSION_ACTIVITY_DISPLAY_MODE
    );
    expect(sanitizeWebSessionActivityDisplayMode(null)).toBe(
      DEFAULT_WEB_SESSION_ACTIVITY_DISPLAY_MODE
    );
  });

  it('maps the default mode to text rendering', () => {
    expect(resolveWebSessionActivityDisplayMode('default')).toBe('text');
    expect(resolveWebSessionActivityDisplayMode('text')).toBe('text');
    expect(resolveWebSessionActivityDisplayMode('card')).toBe('card');
  });

  it('detects modes that replace the old tool card rendering', () => {
    expect(shouldUseWebSessionActivityDisplayMode('default')).toBe(true);
    expect(shouldUseWebSessionActivityDisplayMode('text')).toBe(true);
    expect(shouldUseWebSessionActivityDisplayMode('card')).toBe(true);
  });

  it('normalizes legacy tool kind aliases', () => {
    expect(normalizeWebSessionActivityToolKind('commandExecution')).toBe('command_execution');
    expect(normalizeWebSessionActivityToolKind('fileChange')).toBe('file_change');
    expect(normalizeWebSessionActivityToolKind('mcpToolCall')).toBe('mcp_tool_call');
    expect(normalizeWebSessionActivityToolKind('webSearch')).toBe('web_search');
  });

  it('limits activity display rows to command-like tools and reasoning', () => {
    expect(isWebSessionActivityDisplayToolKind('command_execution')).toBe(true);
    expect(isWebSessionActivityDisplayToolKind('file_change')).toBe(true);
    expect(isWebSessionActivityDisplayToolKind('mcp_tool_call')).toBe(true);
    expect(isWebSessionActivityDisplayToolKind('web_search')).toBe(true);
    expect(isWebSessionActivityDisplayToolKind('dynamic_tool_call')).toBe(true);
    expect(isWebSessionActivityDisplayToolKind('reasoning')).toBe(true);
    expect(isWebSessionActivityDisplayToolKind('context_compaction')).toBe(false);
    expect(isWebSessionActivityDisplayToolKind('plan')).toBe(false);
    expect(isWebSessionActivityDisplayToolKind('image_view')).toBe(false);
  });
});
