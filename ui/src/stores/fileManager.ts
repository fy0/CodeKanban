import { defineStore } from 'pinia';
import { ref } from 'vue';
import { fileManagerApi } from '@/api/fileManager';
import type {
  FileManagerEntry,
  FileManagerListResult,
  FileManagerScope,
  FileTransferTask,
} from '@/types/fileManager';

type UploadRuntime = {
  kind: 'upload';
  file: File;
  directoryPath: string;
  uploadId?: string;
  xhr?: XMLHttpRequest | null;
  retryController?: AbortController | null;
  pauseRequested: boolean;
  cancelRequested: boolean;
  lastSampleAt: number;
  lastSampleLoaded: number;
};

type DownloadRuntime = {
  kind: 'download';
  url: string;
  controller?: AbortController | null;
  pauseRequested: boolean;
  cancelRequested: boolean;
  lastSampleAt: number;
  lastSampleLoaded: number;
};

type TransferRuntime = UploadRuntime | DownloadRuntime;

export interface FileManagerOpenRequest {
  id: number;
  scopeId: string;
  path: string;
}

const UPLOAD_CONCURRENCY = 2;
const DOWNLOAD_CONCURRENCY = 2;
const UPLOAD_RETRY_LIMIT = 3;
const UPLOAD_RETRY_DELAY_MS = 1000;

function makeScopePathKey(projectId: string, scopeId: string) {
  return `${projectId}:${scopeId}`;
}

function createTaskID() {
  return `transfer-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`;
}

function isAbortError(error: unknown) {
  return error instanceof Error && error.name === 'AbortError';
}

function createAbortError(message: string) {
  try {
    return new DOMException(message, 'AbortError');
  } catch {
    const error = new Error(message);
    error.name = 'AbortError';
    return error;
  }
}

function waitForDelay(ms: number, signal?: AbortSignal) {
  return new Promise<void>((resolve, reject) => {
    const timer = globalThis.setTimeout(() => {
      signal?.removeEventListener('abort', handleAbort);
      resolve();
    }, ms);

    const handleAbort = () => {
      globalThis.clearTimeout(timer);
      signal?.removeEventListener('abort', handleAbort);
      reject(createAbortError('upload retry aborted'));
    };

    if (signal) {
      if (signal.aborted) {
        handleAbort();
        return;
      }
      signal.addEventListener('abort', handleAbort, { once: true });
    }
  });
}

function isUploadSessionMissingError(error: unknown) {
  return error instanceof Error && error.message === 'upload session not found';
}

export const useFileManagerStore = defineStore('fileManager', () => {
  const scopesByProject = ref<Record<string, FileManagerScope[]>>({});
  const currentListByProject = ref<Record<string, FileManagerListResult | null>>({});
  const activeScopeIdByProject = ref<Record<string, string>>({});
  const currentPathByScope = ref<Record<string, string>>({});
  const loadingByProject = ref<Record<string, boolean>>({});
  const errorByProject = ref<Record<string, string>>({});
  const openRequestByProject = ref<Record<string, FileManagerOpenRequest | undefined>>({});
  const transferTasks = ref<FileTransferTask[]>([]);

  const runtimes = new Map<string, TransferRuntime>();
  let openRequestSequence = 0;
  let uploadPumpActive = false;
  let downloadPumpActive = false;

  function setProjectLoading(projectId: string, value: boolean) {
    loadingByProject.value = {
      ...loadingByProject.value,
      [projectId]: value,
    };
  }

  function setProjectError(projectId: string, value: string) {
    errorByProject.value = {
      ...errorByProject.value,
      [projectId]: value,
    };
  }

  function setScopes(projectId: string, scopes: FileManagerScope[]) {
    scopesByProject.value = {
      ...scopesByProject.value,
      [projectId]: scopes,
    };
  }

  function setCurrentList(projectId: string, list: FileManagerListResult | null) {
    currentListByProject.value = {
      ...currentListByProject.value,
      [projectId]: list,
    };
  }

  function setActiveScope(projectId: string, scopeId: string, path: string) {
    activeScopeIdByProject.value = {
      ...activeScopeIdByProject.value,
      [projectId]: scopeId,
    };
    currentPathByScope.value = {
      ...currentPathByScope.value,
      [makeScopePathKey(projectId, scopeId)]: path,
    };
  }

  function getScopes(projectId: string) {
    return scopesByProject.value[projectId] ?? [];
  }

  function getList(projectId: string) {
    return currentListByProject.value[projectId] ?? null;
  }

  function getActiveScope(projectId: string) {
    const scopeId = activeScopeIdByProject.value[projectId] ?? '';
    return getScopes(projectId).find(item => item.id === scopeId) ?? null;
  }

  function getLoading(projectId: string) {
    return Boolean(loadingByProject.value[projectId]);
  }

  function getError(projectId: string) {
    return errorByProject.value[projectId] ?? '';
  }

  function getOpenRequest(projectId: string) {
    return openRequestByProject.value[projectId] ?? null;
  }

  function requestFileOpen(projectId: string, target: { scopeId: string; path: string }) {
    openRequestSequence += 1;
    const request: FileManagerOpenRequest = {
      id: openRequestSequence,
      scopeId: target.scopeId,
      path: target.path,
    };
    openRequestByProject.value = {
      ...openRequestByProject.value,
      [projectId]: request,
    };
    return request;
  }

  function consumeFileOpenRequest(projectId: string, requestId: number) {
    if (getOpenRequest(projectId)?.id !== requestId) {
      return;
    }
    const nextRequests = { ...openRequestByProject.value };
    delete nextRequests[projectId];
    openRequestByProject.value = nextRequests;
  }

  function getTransferTasks(projectId?: string) {
    if (!projectId) {
      return transferTasks.value;
    }
    return transferTasks.value.filter(task => task.projectId === projectId);
  }

  function getTaskByID(taskId: string) {
    return transferTasks.value.find(task => task.id === taskId) ?? null;
  }

  function updateTask(taskId: string, patch: Partial<FileTransferTask>) {
    const index = transferTasks.value.findIndex(task => task.id === taskId);
    if (index === -1) {
      return;
    }
    transferTasks.value.splice(index, 1, {
      ...transferTasks.value[index],
      ...patch,
      updatedAt: Date.now(),
    });
  }

  function removeTask(taskId: string) {
    transferTasks.value = transferTasks.value.filter(task => task.id !== taskId);
    runtimes.delete(taskId);
  }

  function clearFinishedTasks(projectId?: string) {
    const removable = new Set(
      transferTasks.value
        .filter(
          task =>
            (!projectId || task.projectId === projectId) &&
            ['completed', 'failed', 'canceled'].includes(task.status)
        )
        .map(task => task.id)
    );
    if (!removable.size) {
      return;
    }
    transferTasks.value = transferTasks.value.filter(task => !removable.has(task.id));
    removable.forEach(taskId => {
      runtimes.delete(taskId);
    });
  }

  function chooseScope(projectId: string, preferredWorktreeId?: string, requestedScopeId?: string) {
    const scopes = getScopes(projectId);
    if (scopes.length === 0) {
      return null;
    }
    if (requestedScopeId) {
      const explicit = scopes.find(scope => scope.id === requestedScopeId);
      if (explicit) {
        return explicit;
      }
    }
    const active = getActiveScope(projectId);
    if (active) {
      return active;
    }
    if (preferredWorktreeId) {
      const preferred = scopes.find(scope => scope.worktreeId === preferredWorktreeId);
      if (preferred) {
        return preferred;
      }
    }
    return scopes[0];
  }

  async function ensureScopes(projectId: string) {
    if (getScopes(projectId).length > 0) {
      return getScopes(projectId);
    }
    const scopes = await fileManagerApi.listScopes(projectId);
    setScopes(projectId, scopes);
    return scopes;
  }

  async function loadDirectory(
    projectId: string,
    options?: {
      scopeId?: string;
      path?: string;
      preferredWorktreeId?: string | null;
    }
  ) {
    setProjectLoading(projectId, true);
    setProjectError(projectId, '');
    try {
      await ensureScopes(projectId);
      const scope = chooseScope(
        projectId,
        options?.preferredWorktreeId ?? undefined,
        options?.scopeId
      );
      if (!scope) {
        setCurrentList(projectId, null);
        return null;
      }
      const scopeKey = makeScopePathKey(projectId, scope.id);
      const nextPath = options?.path ?? currentPathByScope.value[scopeKey] ?? '';
      const result = await fileManagerApi.list(projectId, scope.id, nextPath);
      setCurrentList(projectId, result);
      setActiveScope(projectId, result.scope.id, result.currentPath);
      return result;
    } catch (error) {
      const detail = error instanceof Error ? error.message : 'Failed to load files';
      setProjectError(projectId, detail);
      throw error;
    } finally {
      setProjectLoading(projectId, false);
    }
  }

  async function refreshProject(projectId: string) {
    const activeScope = getActiveScope(projectId);
    const currentList = getList(projectId);
    return loadDirectory(projectId, {
      scopeId: activeScope?.id,
      path: currentList?.currentPath ?? '',
    });
  }

  async function createDirectory(
    projectId: string,
    scopeId: string,
    parentPath: string,
    name: string
  ) {
    await fileManagerApi.createDirectory(projectId, scopeId, parentPath, name);
    await refreshProject(projectId);
  }

  async function renameEntry(projectId: string, scopeId: string, path: string, newName: string) {
    await fileManagerApi.rename(projectId, scopeId, path, newName);
    await refreshProject(projectId);
  }

  async function copyEntries(
    projectId: string,
    scopeId: string,
    sourcePaths: string[],
    destinationPath: string
  ) {
    const result = await fileManagerApi.copy(projectId, scopeId, sourcePaths, destinationPath);
    await refreshProject(projectId);
    return result;
  }

  async function moveEntries(
    projectId: string,
    scopeId: string,
    sourcePaths: string[],
    destinationPath: string
  ) {
    const result = await fileManagerApi.move(projectId, scopeId, sourcePaths, destinationPath);
    await refreshProject(projectId);
    return result;
  }

  async function deleteEntries(projectId: string, scopeId: string, paths: string[]) {
    const result = await fileManagerApi.remove(projectId, scopeId, paths);
    await refreshProject(projectId);
    return result;
  }

  function appendTask(task: FileTransferTask, runtime: TransferRuntime) {
    transferTasks.value = [task, ...transferTasks.value];
    runtimes.set(task.id, runtime);
  }

  function updateTransferMetrics(taskId: string, loaded: number, total?: number) {
    const runtime = runtimes.get(taskId);
    if (!runtime) {
      return;
    }
    const now = Date.now();
    const deltaLoaded = Math.max(0, loaded - runtime.lastSampleLoaded);
    const deltaTime = Math.max(1, now - runtime.lastSampleAt);
    const speed = deltaLoaded > 0 ? (deltaLoaded / deltaTime) * 1000 : 0;
    runtime.lastSampleAt = now;
    runtime.lastSampleLoaded = loaded;
    updateTask(taskId, {
      loaded,
      total,
      progress:
        typeof total === 'number' && total > 0
          ? Math.max(0, Math.min(100, Math.round((loaded / total) * 100)))
          : null,
      speed,
    });
  }

  function isUploadLikelyComplete(taskId: string, runtime: UploadRuntime) {
    const task = getTaskByID(taskId);
    const loaded = Math.max(task?.loaded ?? 0, runtime.lastSampleLoaded, 0);
    return runtime.file.size > 0 && loaded >= runtime.file.size;
  }

  async function didUploadedFileReachTarget(
    task: FileTransferTask,
    runtime: UploadRuntime
  ): Promise<boolean> {
    try {
      const list = await fileManagerApi.list(task.projectId, task.scopeId, runtime.directoryPath);
      return list.entries.some(
        entry =>
          entry.kind === 'file' &&
          entry.name === runtime.file.name &&
          entry.size === runtime.file.size
      );
    } catch {
      return false;
    }
  }

  async function shouldTreatUploadAsCompleted(
    taskId: string,
    task: FileTransferTask,
    runtime: UploadRuntime,
    error: unknown
  ) {
    if (isAbortError(error) || !isUploadLikelyComplete(taskId, runtime)) {
      return false;
    }
    return didUploadedFileReachTarget(task, runtime);
  }

  async function createOrResumeUploadSession(
    taskId: string,
    task: FileTransferTask,
    runtime: UploadRuntime
  ) {
    if (runtime.uploadId) {
      try {
        return await fileManagerApi.getUploadSession(task.projectId, runtime.uploadId);
      } catch (error) {
        if (!isUploadSessionMissingError(error) || isUploadLikelyComplete(taskId, runtime)) {
          throw error;
        }
        runtime.uploadId = undefined;
      }
    }

    const session = await fileManagerApi.createUploadSession(
      task.projectId,
      task.scopeId,
      runtime.directoryPath,
      runtime.file.name,
      runtime.file.size
    );
    runtime.uploadId = session.uploadId;
    return session;
  }

  async function runUploadAttempt(taskId: string, task: FileTransferTask, runtime: UploadRuntime) {
    let session = await createOrResumeUploadSession(taskId, task, runtime);
    runtime.uploadId = session.uploadId;
    runtime.lastSampleAt = Date.now();
    runtime.lastSampleLoaded = session.offset;
    updateTransferMetrics(taskId, session.offset, session.size);

    while (session.offset < session.size) {
      if (runtime.pauseRequested || runtime.cancelRequested) {
        throw createAbortError('upload aborted');
      }
      const nextEnd = Math.min(session.offset + session.chunkSize, runtime.file.size);
      const chunk = runtime.file.slice(session.offset, nextEnd);
      try {
        session = await fileManagerApi.uploadChunk(
          task.projectId,
          session.uploadId,
          session.offset,
          chunk,
          {
            total: session.size,
            onProgress: payload => {
              updateTransferMetrics(taskId, payload.loaded, payload.total);
            },
            onXhr: xhr => {
              runtime.xhr = xhr;
            },
          }
        );
      } finally {
        runtime.xhr = null;
      }
      updateTransferMetrics(taskId, session.offset, session.size);
    }

    await fileManagerApi.completeUpload(task.projectId, session.uploadId);
  }

  async function finalizeUploadSuccess(
    taskId: string,
    task: FileTransferTask,
    runtime: UploadRuntime
  ) {
    runtime.xhr = null;
    runtime.retryController = null;
    runtime.uploadId = undefined;
    updateTask(taskId, {
      status: 'completed',
      loaded: task.total ?? runtime.file.size,
      progress: 100,
      speed: 0,
      error: '',
      retryAttempt: undefined,
      retryMaxAttempts: undefined,
    });

    const currentList = getList(task.projectId);
    if (
      currentList &&
      currentList.scope.id === task.scopeId &&
      currentList.currentPath === runtime.directoryPath
    ) {
      try {
        await refreshProject(task.projectId);
      } catch {}
    }
  }

  async function enqueueUploads(
    projectId: string,
    scopeId: string,
    directoryPath: string,
    files: File[]
  ) {
    for (const file of files) {
      const taskId = createTaskID();
      appendTask(
        {
          id: taskId,
          projectId,
          scopeId,
          directoryPath,
          direction: 'upload',
          name: file.name,
          status: 'queued',
          loaded: 0,
          total: file.size,
          progress: 0,
          speed: 0,
          createdAt: Date.now(),
          updatedAt: Date.now(),
        },
        {
          kind: 'upload',
          file,
          directoryPath,
          retryController: null,
          pauseRequested: false,
          cancelRequested: false,
          lastSampleAt: Date.now(),
          lastSampleLoaded: 0,
        }
      );
    }
    void pumpUploads();
  }

  async function enqueueDownloads(
    projectId: string,
    scopeId: string,
    directoryPath: string,
    entries: FileManagerEntry[],
    options?: {
      forceArchive?: boolean;
    }
  ) {
    const selected = entries.filter(Boolean);
    if (selected.length === 0) {
      return;
    }

    const requiresArchive =
      options?.forceArchive === true ||
      selected.length > 1 ||
      selected.some(item => item.kind === 'directory');

    if (requiresArchive) {
      const archive = await fileManagerApi.createArchive(
        projectId,
        scopeId,
        selected.map(item => item.path)
      );
      const taskId = createTaskID();
      appendTask(
        {
          id: taskId,
          projectId,
          scopeId,
          directoryPath,
          direction: 'download',
          name: archive.fileName,
          status: 'queued',
          loaded: 0,
          total: archive.size,
          progress: 0,
          speed: 0,
          createdAt: Date.now(),
          updatedAt: Date.now(),
        },
        {
          kind: 'download',
          url: archive.downloadUrl,
          pauseRequested: false,
          cancelRequested: false,
          lastSampleAt: Date.now(),
          lastSampleLoaded: 0,
        }
      );
      void pumpDownloads();
      return;
    }

    const entry = selected[0];
    if (!entry) {
      return;
    }
    const taskId = createTaskID();
    appendTask(
      {
        id: taskId,
        projectId,
        scopeId,
        directoryPath,
        direction: 'download',
        name: entry.name,
        status: 'queued',
        loaded: 0,
        total: entry.size,
        progress: entry.size > 0 ? 0 : null,
        speed: 0,
        createdAt: Date.now(),
        updatedAt: Date.now(),
      },
      {
        kind: 'download',
        url: fileManagerApi.buildContentUrl(projectId, scopeId, entry.path, 'attachment'),
        pauseRequested: false,
        cancelRequested: false,
        lastSampleAt: Date.now(),
        lastSampleLoaded: 0,
      }
    );
    void pumpDownloads();
  }

  async function runUploadTask(taskId: string) {
    const runtime = runtimes.get(taskId);
    const task = getTaskByID(taskId);
    if (!runtime || runtime.kind !== 'upload' || !task) {
      return;
    }

    updateTask(taskId, {
      status: 'running',
      error: '',
      retryAttempt: undefined,
      retryMaxAttempts: undefined,
    });

    try {
      let retryAttempt = 0;

      while (true) {
        try {
          await runUploadAttempt(taskId, task, runtime);
          await finalizeUploadSuccess(taskId, task, runtime);
          return;
        } catch (error) {
          runtime.xhr = null;
          runtime.retryController = null;

          if (isAbortError(error) && runtime.pauseRequested) {
            updateTask(taskId, {
              status: 'paused',
              speed: 0,
              retryAttempt: undefined,
              retryMaxAttempts: undefined,
            });
            return;
          }
          if (isAbortError(error) && runtime.cancelRequested) {
            try {
              if (runtime.uploadId) {
                await fileManagerApi.cancelUpload(task.projectId, runtime.uploadId);
              }
            } catch {}
            runtime.uploadId = undefined;
            updateTask(taskId, {
              status: 'canceled',
              speed: 0,
              retryAttempt: undefined,
              retryMaxAttempts: undefined,
            });
            return;
          }
          if (await shouldTreatUploadAsCompleted(taskId, task, runtime, error)) {
            await finalizeUploadSuccess(taskId, task, runtime);
            return;
          }
          if (retryAttempt >= UPLOAD_RETRY_LIMIT) {
            updateTask(taskId, {
              status: 'failed',
              error: error instanceof Error ? error.message : 'Upload failed',
              speed: 0,
              retryAttempt: undefined,
              retryMaxAttempts: undefined,
            });
            return;
          }

          retryAttempt += 1;
          updateTask(taskId, {
            status: 'running',
            error: '',
            speed: 0,
            retryAttempt,
            retryMaxAttempts: UPLOAD_RETRY_LIMIT,
          });
          runtime.lastSampleAt = Date.now();
          runtime.lastSampleLoaded = getTaskByID(taskId)?.loaded ?? runtime.lastSampleLoaded;
          runtime.retryController = new AbortController();
          try {
            await waitForDelay(UPLOAD_RETRY_DELAY_MS, runtime.retryController.signal);
          } finally {
            runtime.retryController = null;
          }
        }
      }
    } finally {
      runtime.retryController = null;
      void pumpUploads();
    }
  }

  async function runDownloadTask(taskId: string) {
    const runtime = runtimes.get(taskId);
    const task = transferTasks.value.find(item => item.id === taskId);
    if (!runtime || runtime.kind !== 'download' || !task) {
      return;
    }

    updateTask(taskId, {
      status: 'running',
      error: '',
    });

    try {
      fileManagerApi.startBrowserDownload(runtime.url);
      updateTask(taskId, {
        status: 'completed',
        loaded: task.total ?? task.loaded,
        total: task.total,
        progress: task.total ? 100 : null,
        speed: 0,
      });
    } catch (error) {
      if (isAbortError(error) && runtime.cancelRequested) {
        updateTask(taskId, {
          status: 'canceled',
          speed: 0,
        });
      } else {
        updateTask(taskId, {
          status: 'failed',
          error: error instanceof Error ? error.message : 'Download failed',
          speed: 0,
        });
      }
    } finally {
      runtime.controller = null;
      void pumpDownloads();
    }
  }

  async function pumpUploads() {
    if (uploadPumpActive) {
      return;
    }
    uploadPumpActive = true;
    try {
      while (
        transferTasks.value.filter(task => task.direction === 'upload' && task.status === 'running')
          .length < UPLOAD_CONCURRENCY
      ) {
        const nextTask = transferTasks.value.find(
          task => task.direction === 'upload' && task.status === 'queued'
        );
        if (!nextTask) {
          break;
        }
        void runUploadTask(nextTask.id);
      }
    } finally {
      uploadPumpActive = false;
    }
  }

  async function pumpDownloads() {
    if (downloadPumpActive) {
      return;
    }
    downloadPumpActive = true;
    try {
      while (
        transferTasks.value.filter(
          task => task.direction === 'download' && task.status === 'running'
        ).length < DOWNLOAD_CONCURRENCY
      ) {
        const nextTask = transferTasks.value.find(
          task => task.direction === 'download' && task.status === 'queued'
        );
        if (!nextTask) {
          break;
        }
        void runDownloadTask(nextTask.id);
      }
    } finally {
      downloadPumpActive = false;
    }
  }

  function pauseTask(taskId: string) {
    const runtime = runtimes.get(taskId);
    const task = transferTasks.value.find(item => item.id === taskId);
    if (!runtime || runtime.kind !== 'upload' || !task) {
      return;
    }
    if (task.status === 'queued') {
      updateTask(taskId, {
        status: 'paused',
        speed: 0,
        retryAttempt: undefined,
        retryMaxAttempts: undefined,
      });
      return;
    }
    if (task.status !== 'running') {
      return;
    }
    runtime.pauseRequested = true;
    runtime.cancelRequested = false;
    runtime.retryController?.abort();
    runtime.xhr?.abort();
  }

  function resumeTask(taskId: string) {
    const runtime = runtimes.get(taskId);
    const task = transferTasks.value.find(item => item.id === taskId);
    if (!runtime || !task) {
      return;
    }
    if (task.status !== 'paused') {
      return;
    }
    runtime.pauseRequested = false;
    runtime.cancelRequested = false;
    updateTask(taskId, {
      status: 'queued',
      speed: 0,
      error: '',
      retryAttempt: undefined,
      retryMaxAttempts: undefined,
    });
    if (runtime.kind === 'upload') {
      void pumpUploads();
    }
  }

  function retryTask(taskId: string) {
    const runtime = runtimes.get(taskId);
    const task = transferTasks.value.find(item => item.id === taskId);
    if (!runtime || !task || (task.status !== 'failed' && task.status !== 'canceled')) {
      return;
    }
    if (runtime.kind === 'upload' && task.status === 'canceled') {
      runtime.uploadId = undefined;
      runtime.lastSampleLoaded = 0;
      updateTask(taskId, {
        loaded: 0,
        progress: 0,
      });
    }
    runtime.pauseRequested = false;
    runtime.cancelRequested = false;
    updateTask(taskId, {
      status: 'queued',
      error: '',
      speed: 0,
      retryAttempt: undefined,
      retryMaxAttempts: undefined,
    });
    if (runtime.kind === 'upload') {
      void pumpUploads();
    } else {
      void pumpDownloads();
    }
  }

  function cancelTask(taskId: string) {
    const runtime = runtimes.get(taskId);
    const task = transferTasks.value.find(item => item.id === taskId);
    if (!runtime || !task) {
      return;
    }
    if (task.status === 'completed') {
      return;
    }
    runtime.cancelRequested = true;
    runtime.pauseRequested = false;

    if (runtime.kind === 'upload') {
      if (task.status === 'queued' || task.status === 'paused' || task.status === 'failed') {
        void (async () => {
          try {
            if (runtime.uploadId) {
              await fileManagerApi.cancelUpload(task.projectId, runtime.uploadId);
            }
          } catch {}
          runtime.uploadId = undefined;
          updateTask(taskId, {
            status: 'canceled',
            speed: 0,
            retryAttempt: undefined,
            retryMaxAttempts: undefined,
          });
        })();
        return;
      }
      runtime.retryController?.abort();
      runtime.xhr?.abort();
      return;
    }

    if (task.status === 'queued' || task.status === 'failed') {
      updateTask(taskId, {
        status: 'canceled',
        speed: 0,
        retryAttempt: undefined,
        retryMaxAttempts: undefined,
      });
      return;
    }
    runtime.controller?.abort();
  }

  return {
    transferTasks,
    getScopes,
    getList,
    getActiveScope,
    getLoading,
    getError,
    getOpenRequest,
    getTransferTasks,
    ensureScopes,
    loadDirectory,
    refreshProject,
    createDirectory,
    renameEntry,
    copyEntries,
    moveEntries,
    deleteEntries,
    enqueueUploads,
    enqueueDownloads,
    pauseTask,
    resumeTask,
    retryTask,
    cancelTask,
    removeTask,
    clearFinishedTasks,
    requestFileOpen,
    consumeFileOpenRequest,
  };
});
