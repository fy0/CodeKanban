import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { useFileManagerStore } from '@/stores/fileManager';
import type {
  FileManagerEntry,
  FileManagerListResult,
  FileManagerScope,
  FileManagerUploadSession,
} from '@/types/fileManager';

const {
  listScopesMock,
  listMock,
  createUploadSessionMock,
  getUploadSessionMock,
  uploadChunkMock,
  completeUploadMock,
  cancelUploadMock,
} = vi.hoisted(() => ({
  window: vi.stubGlobal('window', {
    location: {
      origin: 'http://localhost:5173',
    },
  }),
  listScopesMock: vi.fn(),
  listMock: vi.fn(),
  createUploadSessionMock: vi.fn(),
  getUploadSessionMock: vi.fn(),
  uploadChunkMock: vi.fn(),
  completeUploadMock: vi.fn(),
  cancelUploadMock: vi.fn(),
}));

vi.mock('@/api/fileManager', () => ({
  fileManagerApi: {
    listScopes: listScopesMock,
    list: listMock,
    preview: vi.fn(),
    buildContentUrl: vi.fn(),
    createDirectory: vi.fn(),
    rename: vi.fn(),
    copy: vi.fn(),
    move: vi.fn(),
    remove: vi.fn(),
    createArchive: vi.fn(),
    createUploadSession: createUploadSessionMock,
    getUploadSession: getUploadSessionMock,
    uploadChunk: uploadChunkMock,
    completeUpload: completeUploadMock,
    cancelUpload: cancelUploadMock,
    downloadToBlob: vi.fn(),
    saveBlob: vi.fn(),
    startBrowserDownload: vi.fn(),
  },
}));

function makeScope(): FileManagerScope {
  return {
    id: 'scope-1',
    kind: 'project',
    label: 'Project',
    rootPath: '/tmp/project',
  };
}

function makeFileEntry(name: string, size: number): FileManagerEntry {
  return {
    name,
    path: `docs/${name}`,
    kind: 'file',
    size,
    modifiedAt: '2026-04-11T10:00:00.000Z',
    previewKind: 'binary',
    hidden: false,
  };
}

function makeListResult(
  scope: FileManagerScope,
  currentPath: string,
  entries: FileManagerEntry[]
): FileManagerListResult {
  return {
    scope,
    currentPath,
    breadcrumbs: [
      {
        name: '',
        path: '',
      },
    ],
    entries,
  };
}

function makeSession(size: number, offset = 0): FileManagerUploadSession {
  return {
    uploadId: 'upload-1',
    projectId: 'project-1',
    scopeId: 'scope-1',
    directoryPath: 'docs',
    targetPath: 'docs/docx2pdf',
    fileName: 'docx2pdf',
    size,
    offset,
    chunkSize: size,
    createdAt: '2026-04-11T10:00:00.000Z',
    updatedAt: '2026-04-11T10:00:00.000Z',
    expiresAt: '2026-04-11T11:00:00.000Z',
  };
}

function makeUploadFile() {
  return new File(['test'], 'docx2pdf', {
    type: 'application/octet-stream',
    lastModified: Date.now(),
  });
}

async function flushPromises(iterations = 8) {
  for (let index = 0; index < iterations; index += 1) {
    await Promise.resolve();
  }
}

describe('fileManager upload retries', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    listScopesMock.mockReset();
    listMock.mockReset();
    createUploadSessionMock.mockReset();
    getUploadSessionMock.mockReset();
    uploadChunkMock.mockReset();
    completeUploadMock.mockReset();
    cancelUploadMock.mockReset();
  });

  it('stores and conditionally consumes file open requests', () => {
    const store = useFileManagerStore();
    const firstRequest = store.requestFileOpen('project-1', {
      scopeId: 'scope-1',
      path: 'docs/README.md',
    });

    expect(store.getOpenRequest('project-1')).toEqual(firstRequest);

    const secondRequest = store.requestFileOpen('project-1', {
      scopeId: 'scope-2',
      path: 'src/main.ts',
    });
    store.consumeFileOpenRequest('project-1', firstRequest.id);
    expect(store.getOpenRequest('project-1')).toEqual(secondRequest);

    store.consumeFileOpenRequest('project-1', secondRequest.id);
    expect(store.getOpenRequest('project-1')).toBeNull();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('retries a failed upload until it succeeds', async () => {
    vi.useFakeTimers();
    const store = useFileManagerStore();
    const file = makeUploadFile();
    const sessionStart = makeSession(file.size, 0);
    const sessionDone = makeSession(file.size, file.size);

    createUploadSessionMock.mockResolvedValueOnce(sessionStart);
    getUploadSessionMock.mockResolvedValue(sessionStart);
    uploadChunkMock
      .mockRejectedValueOnce(new Error('network error while uploading chunk'))
      .mockRejectedValueOnce(new Error('network error while uploading chunk'))
      .mockResolvedValueOnce(sessionDone);
    completeUploadMock.mockResolvedValue(makeFileEntry(file.name, file.size));

    await store.enqueueUploads('project-1', 'scope-1', 'docs', [file]);
    await flushPromises();
    await vi.runAllTimersAsync();
    await flushPromises();

    const [task] = store.getTransferTasks('project-1');
    expect(task?.status).toBe('completed');
    expect(task?.progress).toBe(100);
    expect(task?.retryAttempt).toBeUndefined();
    expect(uploadChunkMock).toHaveBeenCalledTimes(3);
    expect(getUploadSessionMock).toHaveBeenCalledTimes(2);
    expect(completeUploadMock).toHaveBeenCalledTimes(1);
  });

  it('stops after three automatic retries fail', async () => {
    vi.useFakeTimers();
    const store = useFileManagerStore();
    const file = makeUploadFile();
    const sessionStart = makeSession(file.size, 0);

    createUploadSessionMock.mockResolvedValueOnce(sessionStart);
    getUploadSessionMock.mockResolvedValue(sessionStart);
    uploadChunkMock.mockRejectedValue(new Error('network error while uploading chunk'));

    await store.enqueueUploads('project-1', 'scope-1', 'docs', [file]);
    await flushPromises();
    await vi.runAllTimersAsync();
    await flushPromises();

    const [task] = store.getTransferTasks('project-1');
    expect(task?.status).toBe('failed');
    expect(task?.error).toBe('network error while uploading chunk');
    expect(task?.retryAttempt).toBeUndefined();
    expect(uploadChunkMock).toHaveBeenCalledTimes(4);
    expect(getUploadSessionMock).toHaveBeenCalledTimes(3);
  });

  it('keeps the task completed even when the post-upload refresh fails', async () => {
    const store = useFileManagerStore();
    const file = makeUploadFile();
    const scope = makeScope();
    const sessionStart = makeSession(file.size, 0);
    const sessionDone = makeSession(file.size, file.size);

    listScopesMock.mockResolvedValueOnce([scope]);
    listMock
      .mockResolvedValueOnce(makeListResult(scope, 'docs', []))
      .mockRejectedValueOnce(new Error('refresh failed'));
    createUploadSessionMock.mockResolvedValueOnce(sessionStart);
    uploadChunkMock.mockResolvedValueOnce(sessionDone);
    completeUploadMock.mockResolvedValueOnce(makeFileEntry(file.name, file.size));

    await store.loadDirectory('project-1', {
      scopeId: scope.id,
      path: 'docs',
    });
    await store.enqueueUploads('project-1', scope.id, 'docs', [file]);
    await flushPromises();

    const [task] = store.getTransferTasks('project-1');
    expect(task?.status).toBe('completed');
    expect(task?.progress).toBe(100);
    expect(store.getError('project-1')).toBe('refresh failed');
    expect(listMock).toHaveBeenCalledTimes(2);
  });

  it('treats a near-complete upload as completed when the file already exists after retry recovery', async () => {
    vi.useFakeTimers();
    const store = useFileManagerStore();
    const file = makeUploadFile();
    const scope = makeScope();
    const sessionStart = makeSession(file.size, 0);
    const sessionDone = makeSession(file.size, file.size);

    createUploadSessionMock.mockResolvedValueOnce(sessionStart);
    uploadChunkMock.mockResolvedValueOnce(sessionDone);
    completeUploadMock.mockRejectedValueOnce(new Error('network error while finalizing upload'));
    getUploadSessionMock.mockRejectedValueOnce(new Error('upload session not found'));
    listMock
      .mockResolvedValueOnce(makeListResult(scope, 'docs', []))
      .mockResolvedValueOnce(makeListResult(scope, 'docs', [makeFileEntry(file.name, file.size)]));

    await store.enqueueUploads('project-1', 'scope-1', 'docs', [file]);
    await flushPromises();
    await vi.runAllTimersAsync();
    await flushPromises();

    const [task] = store.getTransferTasks('project-1');
    expect(task?.status).toBe('completed');
    expect(task?.progress).toBe(100);
    expect(completeUploadMock).toHaveBeenCalledTimes(1);
    expect(getUploadSessionMock).toHaveBeenCalledTimes(1);
    expect(listMock).toHaveBeenCalledTimes(2);
  });
});
