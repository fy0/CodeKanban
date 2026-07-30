import { describe, expect, it } from 'vitest';

import type { FileManagerScope } from '@/types/fileManager';
import {
  isPotentialMessageFileHref,
  resolveMessageLocalFileName,
  resolveMessageLocalFilePath,
  resolveMessageFileTarget,
} from '@/utils/messageFileNavigation';

const BASE_URL = 'http://localhost:3007/#/project/project-1?tab=web';

const windowsScopes: FileManagerScope[] = [
  {
    id: 'project:project-1',
    kind: 'project',
    label: 'vfbox',
    rootPath: 'D:\\codes\\2026\\vfbox',
  },
  {
    id: 'worktree:feature',
    kind: 'worktree',
    label: 'feature',
    rootPath: 'D:\\codes\\2026\\vfbox\\.worktrees\\feature',
    worktreeId: 'feature',
  },
];

describe('messageFileNavigation', () => {
  it('resolves Windows file URLs and relative files for local-file actions', () => {
    expect(
      resolveMessageLocalFilePath(
        'file:///C:/Users/test/AppData/Local/Temp/report%20results.csv',
        BASE_URL,
        { workingDirectory: 'D:\\codes\\2026\\vfbox' }
      )
    ).toBe('C:/Users/test/AppData/Local/Temp/report results.csv');

    expect(
      resolveMessageLocalFilePath('reports/result.csv#latest', BASE_URL, {
        workingDirectory: 'D:\\codes\\2026\\vfbox',
      })
    ).toBe('D:/codes/2026/vfbox/reports/result.csv');
  });

  it('normalizes line suffixes and derives local-file display names', () => {
    const path = resolveMessageLocalFilePath(
      'file:///D:/codes/2026/vfbox/src/main.ts:42:7#L42',
      BASE_URL
    );

    expect(path).toBe('D:/codes/2026/vfbox/src/main.ts');
    expect(resolveMessageLocalFileName(path ?? '')).toBe('main.ts');
  });

  it('does not classify external or unsafe protocols as local-file paths', () => {
    expect(
      resolveMessageLocalFilePath('https://example.com/report.csv', BASE_URL, {
        workingDirectory: 'D:\\codes\\2026\\vfbox',
      })
    ).toBeNull();
    expect(resolveMessageLocalFilePath('javascript:alert(1)', BASE_URL)).toBeNull();
    expect(resolveMessageLocalFilePath('file://server/share/report.csv', BASE_URL)).toBeNull();
  });

  it('resolves the same-origin Windows file URL produced by markdown links', () => {
    expect(
      resolveMessageFileTarget(
        'http://localhost:3007/D:/codes/2026/vfbox/docs/README.md',
        BASE_URL,
        windowsScopes
      )
    ).toEqual({
      scopeId: 'project:project-1',
      path: 'docs/README.md',
    });

    expect(
      resolveMessageFileTarget('/D:/codes/2026/vfbox/docs/README.md', BASE_URL, windowsScopes)
    ).toEqual({
      scopeId: 'project:project-1',
      path: 'docs/README.md',
    });
  });

  it('decodes file paths, compares Windows roots case-insensitively, and strips line suffixes', () => {
    expect(
      resolveMessageFileTarget(
        '/d:/CODES/2026/VFBOX/docs/My%20File.ts:42:7#L42',
        BASE_URL,
        windowsScopes
      )
    ).toEqual({
      scopeId: 'project:project-1',
      path: 'docs/My File.ts',
    });
  });

  it('prefers the most specific matching worktree scope', () => {
    expect(
      resolveMessageFileTarget(
        '/D:/codes/2026/vfbox/.worktrees/feature/src/main.ts',
        BASE_URL,
        windowsScopes
      )
    ).toEqual({
      scopeId: 'worktree:feature',
      path: 'src/main.ts',
    });
  });

  it('resolves relative links from the current session working directory', () => {
    expect(
      resolveMessageFileTarget('README.md', BASE_URL, windowsScopes, {
        workingDirectory: 'D:\\codes\\2026\\vfbox\\docs',
      })
    ).toEqual({
      scopeId: 'project:project-1',
      path: 'docs/README.md',
    });
  });

  it('uses the preferred scope for project-relative links without a working directory', () => {
    expect(
      resolveMessageFileTarget('src/main.ts', BASE_URL, windowsScopes, {
        preferredScopeId: 'worktree:feature',
      })
    ).toEqual({
      scopeId: 'worktree:feature',
      path: 'src/main.ts',
    });
  });

  it('supports POSIX project roots', () => {
    const scopes: FileManagerScope[] = [
      {
        id: 'project:linux',
        kind: 'project',
        label: 'linux',
        rootPath: '/home/dev/codekanban',
      },
    ];
    expect(
      resolveMessageFileTarget('/home/dev/codekanban/ui/src/main.ts', BASE_URL, scopes)
    ).toEqual({
      scopeId: 'project:linux',
      path: 'ui/src/main.ts',
    });
  });

  it('does not resolve external, sibling, or escaping paths as project files', () => {
    expect(
      resolveMessageFileTarget('https://example.com/docs/README.md', BASE_URL, windowsScopes)
    ).toBeNull();
    expect(
      resolveMessageFileTarget('/D:/codes/2026/vfbox-copy/docs/README.md', BASE_URL, windowsScopes)
    ).toBeNull();
    expect(
      resolveMessageFileTarget('../../secrets.txt', BASE_URL, windowsScopes, {
        workingDirectory: 'D:\\codes\\2026\\vfbox',
      })
    ).toBeNull();
  });

  it('only marks local-looking links as potential file links', () => {
    expect(isPotentialMessageFileHref('/D:/codes/2026/vfbox/docs/README.md', BASE_URL)).toBe(true);
    expect(isPotentialMessageFileHref('docs/README.md', BASE_URL)).toBe(true);
    expect(isPotentialMessageFileHref('https://example.com/docs', BASE_URL)).toBe(false);
    expect(isPotentialMessageFileHref('mailto:test@example.com', BASE_URL)).toBe(false);
    expect(isPotentialMessageFileHref('#introduction', BASE_URL)).toBe(false);
  });
});
