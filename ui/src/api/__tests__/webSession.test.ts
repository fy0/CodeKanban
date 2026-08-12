import { beforeEach, describe, expect, it, vi } from 'vitest';

const { getMethodMock, getSendMock, postMethodMock, postSendMock, postAbortMock, fetchMock } =
  vi.hoisted(() => {
    const getSendMock = vi.fn();
    const postSendMock = vi.fn();
    const postAbortMock = vi.fn();
    return {
      getMethodMock: vi.fn(() => ({ send: getSendMock })),
      getSendMock,
      postMethodMock: vi.fn(() => ({
        send: postSendMock,
        abort: postAbortMock,
      })),
      postSendMock,
      postAbortMock,
      fetchMock: vi.fn(),
    };
  });

vi.mock('@/api', () => ({
  urlBase: '',
  ApiError: class ApiError extends Error {
    status: number;
    statusText: string;
    data: unknown;

    constructor(status: number, statusText: string, data: unknown) {
      super(statusText);
      this.status = status;
      this.statusText = statusText;
      this.data = data;
    }
  },
}));

vi.mock('@/api/http', () => ({
  http: {
    Get: getMethodMock,
    Post: postMethodMock,
    Patch: vi.fn(),
    Delete: vi.fn(),
  },
}));

vi.stubGlobal('fetch', fetchMock);

import { buildWebSessionLocalFileContentUrl, webSessionApi } from '@/api/webSession';

describe('webSessionApi.search', () => {
  beforeEach(() => {
    postMethodMock.mockClear();
    postSendMock.mockReset();
    postAbortMock.mockReset();
    postSendMock.mockResolvedValue({
      item: {
        items: [{ id: 'session-1' }],
        nextCursor: 'cursor-2',
        done: false,
        scanned: 50,
        total: 120,
      },
    });
  });

  it('sends a bounded progressive search chunk request', async () => {
    const result = await webSessionApi.search({
      projectIds: ['project-1'],
      query: '  needle  ',
      includeArchived: true,
      cursor: 'cursor-1',
      scanLimit: 50,
    });

    expect(postMethodMock).toHaveBeenCalledWith('/web-sessions/search', {
      projectIds: ['project-1'],
      query: 'needle',
      includeArchived: true,
      includeBody: true,
      cursor: 'cursor-1',
      scanLimit: 50,
    });
    expect(result).toMatchObject({
      nextCursor: 'cursor-2',
      scanned: 50,
      total: 120,
    });
  });

  it('aborts the active chunk request with the caller signal', async () => {
    let rejectRequest: ((reason?: unknown) => void) | null = null;
    postMethodMock.mockImplementationOnce(() => ({
      send: vi.fn(
        () =>
          new Promise((_, reject) => {
            rejectRequest = reject;
          })
      ),
      abort: vi.fn(() => {
        rejectRequest?.(new DOMException('session search aborted', 'AbortError'));
      }),
    }));
    const controller = new AbortController();

    const request = webSessionApi.search(
      {
        projectIds: ['project-1'],
        query: 'needle',
      },
      { signal: controller.signal }
    );
    controller.abort();

    await expect(request).rejects.toMatchObject({ name: 'AbortError' });
  });

  it('creates an edited-message branch through the source history item', async () => {
    postSendMock.mockResolvedValueOnce({
      item: {
        session: { id: 'session-branch' },
        history: { items: [], hasMore: false, total: 0 },
      },
    });

    const result = await webSessionApi.editUserMessage(
      'project-1',
      'session-source',
      'history-item-2',
      'revised prompt'
    );

    expect(postMethodMock).toHaveBeenCalledWith(
      '/projects/project-1/web-sessions/session-source/messages/history-item-2/edit',
      { text: 'revised prompt' }
    );
    expect(result.session?.id).toBe('session-branch');
  });

  it('omits unspecified session defaults so the server can resolve global settings', async () => {
    postSendMock.mockResolvedValueOnce({
      item: { id: 'session-created' },
    });

    await webSessionApi.create('project-1', { agent: 'codex' });

    const requestBody = postMethodMock.mock.calls.at(-1)?.[1] as Record<string, unknown>;
    expect(requestBody).not.toHaveProperty('model');
    expect(requestBody).not.toHaveProperty('reasoningEffort');
    expect(requestBody).not.toHaveProperty('permissionLevel');
  });
});

describe('webSessionApi.searchConversation', () => {
  beforeEach(() => {
    postMethodMock.mockClear();
    postSendMock.mockReset();
    postAbortMock.mockReset();
    postSendMock.mockResolvedValue({
      item: {
        items: [
          {
            id: 'item-1',
            orderIndex: 3,
            kind: 'tool',
            toolId: 'tool-1',
          },
        ],
        done: true,
        total: 1,
      },
    });
  });

  it('sends the current-session category filters and decodes matches', async () => {
    const result = await webSessionApi.searchConversation('project/1', 'session 2', {
      query: '  needle  ',
      includeUser: true,
      includeAssistant: true,
      includeTools: false,
      includeSystem: false,
      sourceThreadId: 'thread-1',
      cursor: 'cursor-1',
      limit: 100,
    });

    expect(postMethodMock).toHaveBeenCalledWith(
      '/projects/project%2F1/web-sessions/session%202/search',
      {
        query: 'needle',
        includeUser: true,
        includeAssistant: true,
        includeTools: false,
        includeSystem: false,
        sourceThreadId: 'thread-1',
        cursor: 'cursor-1',
        limit: 100,
      }
    );
    expect(result.items[0]).toMatchObject({ id: 'item-1', kind: 'tool' });
  });

  it('aborts an in-flight current-session search with the caller signal', async () => {
    let rejectRequest: ((reason?: unknown) => void) | null = null;
    const abortMock = vi.fn(() => {
      rejectRequest?.(new DOMException('conversation search aborted', 'AbortError'));
    });
    postMethodMock.mockImplementationOnce(() => ({
      send: vi.fn(
        () =>
          new Promise((_, reject) => {
            rejectRequest = reject;
          })
      ),
      abort: abortMock,
    }));
    const controller = new AbortController();

    const request = webSessionApi.searchConversation(
      'project-1',
      'session-1',
      {
        query: 'needle',
        includeUser: true,
        includeAssistant: true,
        includeTools: false,
        includeSystem: false,
      },
      { signal: controller.signal }
    );
    controller.abort();

    await expect(request).rejects.toMatchObject({ name: 'AbortError' });
    expect(abortMock).toHaveBeenCalledOnce();
  });
});

describe('webSessionApi Pi tree', () => {
  const tree = {
    sessionId: 'session-1',
    leafId: 'leaf-1',
    revision: 'rev-1',
    nodes: [
      {
        id: 'leaf-1',
        type: 'message',
        role: 'user',
        active: true,
        children: [],
      },
    ],
  };

  beforeEach(() => {
    getMethodMock.mockClear();
    getSendMock.mockReset();
    postMethodMock.mockClear();
    postSendMock.mockReset();
  });

  it('loads the encoded project session tree', async () => {
    getSendMock.mockResolvedValueOnce({ item: tree });

    await expect(webSessionApi.tree('project/1', 'session 2')).resolves.toEqual(tree);
    expect(getMethodMock).toHaveBeenCalledWith(
      '/projects/project%2F1/web-sessions/session%202/tree'
    );
  });

  it('navigates with a revision and optional summary request', async () => {
    postSendMock.mockResolvedValueOnce({ item: { tree, editorText: 'rewrite this' } });

    await expect(
      webSessionApi.navigateTree('project-1', 'session-1', {
        targetId: 'leaf-1',
        revision: 'rev-1',
        summarize: true,
      })
    ).resolves.toMatchObject({ tree, editorText: 'rewrite this' });
    expect(postMethodMock).toHaveBeenCalledWith(
      '/projects/project-1/web-sessions/session-1/tree/navigate',
      { targetId: 'leaf-1', revision: 'rev-1', summarize: true }
    );
  });

  it('forks and clones into a returned target session', async () => {
    postSendMock
      .mockResolvedValueOnce({
        item: {
          session: { id: 'forked' },
          tree: { ...tree, sessionId: 'forked' },
          editorText: 'u',
        },
      })
      .mockResolvedValueOnce({
        item: { session: { id: 'cloned' }, tree: { ...tree, sessionId: 'cloned' } },
      });

    await expect(
      webSessionApi.forkTree('project-1', 'session-1', {
        targetId: 'leaf-1',
        revision: 'rev-1',
      })
    ).resolves.toMatchObject({ session: { id: 'forked' }, editorText: 'u' });
    expect(postMethodMock).toHaveBeenNthCalledWith(
      1,
      '/projects/project-1/web-sessions/session-1/tree/fork',
      { targetId: 'leaf-1', revision: 'rev-1' }
    );

    await expect(
      webSessionApi.cloneTree('project-1', 'session-1', { revision: 'rev-1' })
    ).resolves.toMatchObject({ session: { id: 'cloned' } });
    expect(postMethodMock).toHaveBeenNthCalledWith(
      2,
      '/projects/project-1/web-sessions/session-1/tree/clone',
      { revision: 'rev-1' }
    );
  });
});

describe('webSessionApi local files', () => {
  beforeEach(() => {
    postMethodMock.mockClear();
    postSendMock.mockReset();
    fetchMock.mockReset();
  });

  it('builds an encoded local-file download URL', () => {
    expect(
      buildWebSessionLocalFileContentUrl('project/1', 'session 2', 'C:\\Temp\\report results.csv')
    ).toBe(
      '/api/v1/projects/project%2F1/web-sessions/session%202/local-files/content?path=C%3A%5CTemp%5Creport+results.csv'
    );
  });

  it('preflights a local-file download with credentials', async () => {
    fetchMock.mockResolvedValueOnce(new Response(null, { status: 200 }));

    await webSessionApi.probeLocalFile('project-1', 'session-1', 'C:\\Temp\\report.csv');

    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/projects/project-1/web-sessions/session-1/local-files/content?path=C%3A%5CTemp%5Creport.csv',
      {
        method: 'HEAD',
        credentials: 'include',
      }
    );
  });

  it('opens a validated local-file location through the session endpoint', async () => {
    postSendMock.mockResolvedValueOnce({ message: 'file location opened' });

    await webSessionApi.openLocalFileLocation('project-1', 'session-1', 'C:\\Temp\\report.csv');

    expect(postMethodMock).toHaveBeenCalledWith(
      '/projects/project-1/web-sessions/session-1/local-files/open-location',
      { path: 'C:\\Temp\\report.csv' }
    );
  });
});
