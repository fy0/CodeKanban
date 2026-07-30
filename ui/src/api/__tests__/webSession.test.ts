import { beforeEach, describe, expect, it, vi } from 'vitest';

const { postMethodMock, postSendMock, postAbortMock } = vi.hoisted(() => {
  const postSendMock = vi.fn();
  const postAbortMock = vi.fn();
  return {
    postMethodMock: vi.fn(() => ({
      send: postSendMock,
      abort: postAbortMock,
    })),
    postSendMock,
    postAbortMock,
  };
});

vi.mock('@/api', () => ({
  urlBase: '',
}));

vi.mock('@/api/http', () => ({
  http: {
    Get: vi.fn(),
    Post: postMethodMock,
    Patch: vi.fn(),
    Delete: vi.fn(),
  },
}));

import { webSessionApi } from '@/api/webSession';

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
