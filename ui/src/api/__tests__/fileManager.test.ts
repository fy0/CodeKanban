import { beforeEach, describe, expect, it, vi } from 'vitest';

const { getMethodMock, getSendMock, getAbortMock } = vi.hoisted(() => {
  const getSendMock = vi.fn();
  const getAbortMock = vi.fn();
  return {
    getMethodMock: vi.fn(() => ({
      send: getSendMock,
      abort: getAbortMock,
    })),
    getSendMock,
    getAbortMock,
  };
});

vi.mock('@/api', () => ({
  ApiError: class ApiError extends Error {},
  urlBase: '',
}));

vi.mock('@/api/http', () => ({
  http: {
    Get: getMethodMock,
    Post: vi.fn(),
    Patch: vi.fn(),
    Delete: vi.fn(),
  },
}));

import { fileManagerApi } from '@/api/fileManager';

describe('fileManagerApi.search', () => {
  beforeEach(() => {
    getMethodMock.mockClear();
    getSendMock.mockReset();
    getAbortMock.mockReset();
    getSendMock.mockResolvedValue({
      item: {
        scope: {
          id: 'scope-1',
          kind: 'project',
          label: 'Project',
          rootPath: '/tmp/project',
        },
        currentPath: 'docs',
        entries: [],
        truncated: false,
      },
    });
  });

  it('passes recursive search params to the backend', async () => {
    await fileManagerApi.search('project-1', 'scope-1', 'docs/api', 'foo*bar', true);

    expect(getMethodMock).toHaveBeenCalledWith(
      '/projects/project-1/files/search?scopeId=scope-1&path=docs%2Fapi&query=foo*bar&regex=true'
    );
  });
});

describe('fileManagerApi.listChanges', () => {
  beforeEach(() => {
    getMethodMock.mockClear();
    getSendMock.mockReset();
    getAbortMock.mockReset();
    getSendMock.mockResolvedValue({
      item: {
        scope: {
          id: 'scope-1',
          kind: 'project',
          label: 'Project',
          rootPath: '/tmp/project',
        },
        entries: [],
        changeToken: 'token-1',
        truncated: false,
        statsComplete: true,
        statsTimedOut: false,
        untrackedIncluded: false,
      },
    });
  });

  it('passes backend guardrail params instead of relying on local filtering', async () => {
    const result = await fileManagerApi.listChanges('project-1', 'scope-1', {
      includeUntracked: false,
      withStats: true,
      timeoutMs: 5000,
      maxEntries: 1000,
    });

    expect(getMethodMock).toHaveBeenCalledWith(
      '/projects/project-1/files/changes?scopeId=scope-1&includeUntracked=false&withStats=true&timeoutMs=5000&maxEntries=1000'
    );
    expect(result.changeToken).toBe('token-1');
  });

  it('aborts the in-flight request when the caller aborts the signal', async () => {
    let rejectRequest: ((reason?: unknown) => void) | null = null;
    getMethodMock.mockImplementationOnce(() => ({
      send: vi.fn(
        () =>
          new Promise((_, reject) => {
            rejectRequest = reject;
          })
      ),
      abort: vi.fn(() => {
        rejectRequest?.(new DOMException('git changes load aborted', 'AbortError'));
      }),
    }));
    const controller = new AbortController();

    const request = fileManagerApi.listChanges('project-1', 'scope-1', {
      signal: controller.signal,
    });
    controller.abort();

    await expect(request).rejects.toMatchObject({
      name: 'AbortError',
    });
  });
});

describe('fileManagerApi.changesSummary', () => {
  beforeEach(() => {
    getMethodMock.mockClear();
    getSendMock.mockReset();
    getAbortMock.mockReset();
    getSendMock.mockResolvedValue({
      item: {
        scope: {
          id: 'scope-1',
          kind: 'project',
          label: 'Project',
          rootPath: '/tmp/project',
        },
        count: 1,
        changeToken: 'token-1',
        additions: null,
        deletions: null,
        statsComplete: false,
        statsTimedOut: false,
      },
    });
  });

  it('loads a status-only summary token', async () => {
    const result = await fileManagerApi.changesSummary('project-1', 'scope-1', {
      includeUntracked: false,
      withStats: false,
    });

    expect(getMethodMock).toHaveBeenCalledWith(
      '/projects/project-1/files/changes-summary?scopeId=scope-1'
    );
    expect(result.changeToken).toBe('token-1');
  });

  it('aborts an in-flight summary request', async () => {
    let rejectRequest: ((reason?: unknown) => void) | null = null;
    getMethodMock.mockImplementationOnce(() => ({
      send: vi.fn(
        () =>
          new Promise((_, reject) => {
            rejectRequest = reject;
          })
      ),
      abort: vi.fn(() => {
        rejectRequest?.(new DOMException('git changes summary load aborted', 'AbortError'));
      }),
    }));
    const controller = new AbortController();

    const request = fileManagerApi.changesSummary('project-1', 'scope-1', {
      signal: controller.signal,
    });
    controller.abort();

    await expect(request).rejects.toMatchObject({
      name: 'AbortError',
    });
  });
});
