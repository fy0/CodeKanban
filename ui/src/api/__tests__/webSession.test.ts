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

describe('webSessionApi.runtimeConfig', () => {
  it('uses the server refresh endpoint for an explicit forced refresh', async () => {
    getMethodMock.mockClear();
    getSendMock.mockReset();
    getSendMock.mockResolvedValueOnce({
      item: {
        agents: {},
        contextWindowTokens: 0,
        compactLimitTokens: 0,
        source: 'unavailable',
        models: [],
        piModels: [],
      },
    });

    await webSessionApi.runtimeConfig({ force: true });

    expect(getMethodMock).toHaveBeenCalledWith('/web-sessions/runtime-config?refresh=true', {
      cacheFor: 0,
    });
    expect(getSendMock).toHaveBeenCalledWith(true);
  });
});

describe('webSessionApi.querySessions', () => {
  it('requests one globally paginated list for the project scope', async () => {
    postMethodMock.mockClear();
    postSendMock.mockReset();
    postSendMock.mockResolvedValueOnce({
      item: {
        items: [{ id: 'session-1', projectId: 'project-2' }],
        total: 41,
        hasMore: true,
        nextOffset: 40,
      },
    });

    const result = await webSessionApi.querySessions({
      projectIds: ['project-1', 'project-2'],
      archived: false,
      offset: 0,
      limit: 100,
    });

    expect(postMethodMock).toHaveBeenCalledTimes(1);
    expect(postMethodMock).toHaveBeenCalledWith('/web-sessions/query', {
      projectIds: ['project-1', 'project-2'],
      archived: false,
      offset: 0,
      limit: 100,
    });
    expect(result).toMatchObject({ total: 41, hasMore: true, nextOffset: 40 });
  });
});

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
        revision: '7',
        session: { id: 'session-branch' },
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
    expect(result.revision).toBe('7');
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

describe('webSessionApi.history', () => {
  beforeEach(() => {
    getMethodMock.mockReset();
  });

  it('requests the first chronological page with an after cursor at zero', async () => {
    const sendMock = vi.fn().mockResolvedValue({
      item: {
        items: [{ id: 'history-1', orderIndex: 1 }],
        hasMore: false,
        hasLater: true,
        afterCursor: '1',
        total: 3,
      },
    });
    getMethodMock.mockReturnValue({ send: sendMock });

    const result = await webSessionApi.history('project-1', 'session-1', {
      afterCursor: '0',
      limit: 80,
    });

    expect(getMethodMock).toHaveBeenCalledWith(
      '/projects/project-1/web-sessions/session-1/history?afterCursor=0&limit=80'
    );
    expect(sendMock).toHaveBeenCalledWith(true);
    expect(result).toMatchObject({ hasLater: true, afterCursor: '1', total: 3 });
  });
});

describe('webSessionApi.catchUp', () => {
  beforeEach(() => {
    getMethodMock.mockReset();
  });

  it('requests a stable event-cursor window', async () => {
    const sendMock = vi.fn().mockResolvedValue({
      item: {
        revision: '9',
        historyEpoch: '2',
        nextEventCursor: '8:12',
        targetEventCursor: '9:9223372036854775807',
        hasMore: true,
        resetRequired: false,
        session: { id: 'session-1' },
        items: [],
        total: 4,
        pendingEpoch: 'process-1',
        pendingVersion: 1,
        pendingInputs: [],
        scheduledInputs: [],
        subAgents: [],
      },
    });
    getMethodMock.mockReturnValue({ send: sendMock, abort: vi.fn() });

    const result = await webSessionApi.catchUp('project-1', 'session-1', {
      afterEventCursor: '7:9223372036854775807',
      targetEventCursor: '9:9223372036854775807',
      historyEpoch: '2',
      limit: 80,
    });

    expect(getMethodMock).toHaveBeenCalledWith(
      '/projects/project-1/web-sessions/session-1/catch-up?afterEventCursor=7%3A9223372036854775807&historyEpoch=2&limit=80&targetEventCursor=9%3A9223372036854775807',
      { shareRequest: false }
    );
    expect(sendMock).toHaveBeenCalledWith(true);
    expect(result).toMatchObject({ hasMore: true, nextEventCursor: '8:12' });
  });
});

describe('webSessionApi.commandGroupDetail', () => {
  beforeEach(() => {
    getMethodMock.mockReset();
  });

  it('loads a typed command group detail from the session endpoint', async () => {
    const sendMock = vi.fn().mockResolvedValue({
      item: {
        groupId: 'group/1',
        kind: 'command_execution',
        title: 'CommandExecution',
        summary: 'git status',
        count: 2,
        firstSeq: 10,
        lastSeq: 20,
        status: 'done',
        items: [],
      },
    });
    getMethodMock.mockReturnValue({ send: sendMock });

    const result = await webSessionApi.commandGroupDetail('project-1', 'session-1', 'group/1');

    expect(getMethodMock).toHaveBeenCalledWith(
      '/projects/project-1/web-sessions/session-1/command-groups/group%2F1',
      { cacheFor: 0 }
    );
    expect(sendMock).toHaveBeenCalledWith(true);
    expect(result).toMatchObject({ groupId: 'group/1', count: 2 });
  });
});

describe('webSessionApi work timing', () => {
  beforeEach(() => {
    getMethodMock.mockReset();
    postMethodMock.mockClear();
    postSendMock.mockReset();
  });

  it('requests a single session calculation from the encoded endpoint', async () => {
    postSendMock.mockResolvedValueOnce({
      item: {
        status: 'calculated',
        session: { id: 'session/1' },
        items: [],
      },
    });

    const result = await webSessionApi.calculateWorkTiming('project 1', 'session/1');

    expect(postMethodMock).toHaveBeenCalledWith(
      '/projects/project%201/web-sessions/session%2F1/work-timing/calculate'
    );
    expect(result.status).toBe('calculated');
  });

  it('loads indexed status and sends the requested batch size', async () => {
    const getSendMock = vi.fn().mockResolvedValue({
      item: {
        remainingSessionCount: 12,
        busySessionCount: 1,
        completeSessionCount: 3,
        partialSessionCount: 0,
        unavailableSessionCount: 0,
        failedSessionCount: 0,
      },
    });
    getMethodMock.mockReturnValueOnce({ send: getSendMock });

    const status = await webSessionApi.workTimingBackfillStatus();
    expect(getMethodMock).toHaveBeenCalledWith('/system/web-session-work-timing-backfill/status', {
      cacheFor: 0,
    });
    expect(status.remainingSessionCount).toBe(12);

    postSendMock.mockResolvedValueOnce({
      item: {
        ...status,
        attemptedSessionCount: 25,
        calculatedSessionCount: 20,
        partialResultCount: 1,
        unavailableResultCount: 4,
        failedResultCount: 0,
      },
    });
    await webSessionApi.runWorkTimingBackfill(25);
    expect(postMethodMock).toHaveBeenCalledWith('/system/web-session-work-timing-backfill/run', {
      limit: 25,
    });
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
