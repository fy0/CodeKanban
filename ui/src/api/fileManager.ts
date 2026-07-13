import { ApiError, urlBase } from '@/api';
import { extractItem, extractItems } from '@/api/response';
import { http } from '@/api/http';
import type {
  FileManagerArchiveJob,
  FileManagerBulkResult,
  FileManagerChangesResult,
  FileManagerChangesSummaryResult,
  FileManagerDiffResult,
  FileManagerEntry,
  FileManagerListResult,
  FileManagerPreviewResult,
  FileManagerSearchResult,
  FileManagerScope,
  FileManagerUploadSession,
} from '@/types/fileManager';

type ItemResponse<T> = {
  item?: T;
};

type ItemsResponse<T> = {
  items?: T[];
};

export interface TransferProgressPayload {
  loaded: number;
  total?: number;
  progress: number | null;
}

function resolveUrl(path: string) {
  return urlBase ? new URL(path, urlBase).toString() : path;
}

function readStoredToken() {
  return window.localStorage.getItem('token');
}

function normalizeJsonError(payload: unknown, fallback: string) {
  if (typeof payload === 'object' && payload !== null && 'detail' in payload) {
    return String((payload as { detail?: string }).detail || fallback);
  }
  if (typeof payload === 'string' && payload.trim()) {
    return payload;
  }
  return fallback;
}

function parseJSONPayload(xhr: XMLHttpRequest) {
  if (xhr.response && typeof xhr.response === 'object') {
    return xhr.response;
  }
  if (!xhr.responseText) {
    return undefined;
  }
  try {
    return JSON.parse(xhr.responseText);
  } catch {
    return xhr.responseText;
  }
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

export const fileManagerApi = {
  async listScopes(
    projectId: string,
    options?: {
      signal?: AbortSignal;
    }
  ): Promise<FileManagerScope[]> {
    const method = http.Get<ItemsResponse<FileManagerScope>>(`/projects/${projectId}/files/scopes`);
    const abortHandler = () => {
      method.abort();
    };

    if (options?.signal?.aborted) {
      throw createAbortError('git changes scope load aborted');
    }

    options?.signal?.addEventListener('abort', abortHandler, { once: true });
    let payload: ItemsResponse<FileManagerScope> = {};
    try {
      payload = (await method.send(true)) ?? {};
    } finally {
      options?.signal?.removeEventListener('abort', abortHandler);
    }
    return extractItems<FileManagerScope>(payload);
  },

  async list(projectId: string, scopeId: string, path = ''): Promise<FileManagerListResult> {
    const params = new URLSearchParams();
    params.set('scopeId', scopeId);
    params.set('path', path);
    const payload =
      (await http
        .Get<
          ItemResponse<FileManagerListResult>
        >(`/projects/${projectId}/files/list?${params.toString()}`)
        .send(true)) ?? {};
    const item = extractItem<FileManagerListResult>(payload);
    if (!item) {
      throw new Error('failed to load files');
    }
    return item;
  },

  async search(
    projectId: string,
    scopeId: string,
    path: string,
    query: string,
    useRegex: boolean
  ): Promise<FileManagerSearchResult> {
    const params = new URLSearchParams();
    params.set('scopeId', scopeId);
    params.set('path', path);
    params.set('query', query);
    params.set('regex', String(useRegex));
    const payload =
      (await http
        .Get<
          ItemResponse<FileManagerSearchResult>
        >(`/projects/${projectId}/files/search?${params.toString()}`)
        .send(true)) ?? {};
    const item = extractItem<FileManagerSearchResult>(payload);
    if (!item) {
      throw new Error('failed to search files');
    }
    return item;
  },

  async listChanges(
    projectId: string,
    scopeId: string,
    options?: {
      includeUntracked?: boolean;
      withStats?: boolean;
      timeoutMs?: number;
      maxEntries?: number;
      signal?: AbortSignal;
    }
  ): Promise<FileManagerChangesResult> {
    const params = new URLSearchParams();
    params.set('scopeId', scopeId);
    params.set('includeUntracked', String(options?.includeUntracked ?? true));
    params.set('withStats', String(options?.withStats ?? true));
    if (typeof options?.timeoutMs === 'number' && Number.isFinite(options.timeoutMs)) {
      params.set('timeoutMs', String(Math.max(0, Math.trunc(options.timeoutMs))));
    }
    if (typeof options?.maxEntries === 'number' && Number.isFinite(options.maxEntries)) {
      params.set('maxEntries', String(Math.max(1, Math.trunc(options.maxEntries))));
    }

    const method = http.Get<ItemResponse<FileManagerChangesResult>>(
      `/projects/${projectId}/files/changes?${params.toString()}`
    );
    const abortHandler = () => {
      method.abort();
    };

    if (options?.signal?.aborted) {
      throw createAbortError('git changes load aborted');
    }

    options?.signal?.addEventListener('abort', abortHandler, { once: true });
    let payload: ItemResponse<FileManagerChangesResult> = {};
    try {
      payload = (await method.send(true)) ?? {};
    } finally {
      options?.signal?.removeEventListener('abort', abortHandler);
    }
    const item = extractItem<FileManagerChangesResult>(payload);
    if (!item) {
      throw new Error('failed to load git changes');
    }
    return {
      ...item,
      changeToken: item.changeToken ?? '',
      entries: (item.entries ?? []).map(entry => ({
        ...entry,
        additions: Math.max(0, Math.trunc(entry.additions ?? 0)),
        deletions: Math.max(0, Math.trunc(entry.deletions ?? 0)),
        statsAvailable: entry.statsAvailable === true,
      })),
      truncated: item.truncated === true,
      statsComplete: item.statsComplete === true,
      statsTimedOut: item.statsTimedOut === true,
      untrackedIncluded: item.untrackedIncluded !== false,
      warningReason: item.warningReason ?? '',
    };
  },

  async changesSummary(
    projectId: string,
    scopeId: string,
    options?: {
      includeUntracked?: boolean;
      withStats?: boolean;
      timeoutMs?: number;
      signal?: AbortSignal;
    }
  ): Promise<FileManagerChangesSummaryResult> {
    const params = new URLSearchParams();
    params.set('scopeId', scopeId);
    if (options?.includeUntracked) {
      params.set('includeUntracked', 'true');
    }
    if (options?.withStats) {
      params.set('withStats', 'true');
    }
    if (typeof options?.timeoutMs === 'number' && Number.isFinite(options.timeoutMs)) {
      params.set('timeoutMs', String(Math.max(0, Math.trunc(options.timeoutMs))));
    }
    const method = http.Get<ItemResponse<FileManagerChangesSummaryResult>>(
      `/projects/${projectId}/files/changes-summary?${params.toString()}`
    );
    const abortHandler = () => {
      method.abort();
    };

    if (options?.signal?.aborted) {
      throw createAbortError('git changes summary load aborted');
    }

    options?.signal?.addEventListener('abort', abortHandler, { once: true });
    let payload: ItemResponse<FileManagerChangesSummaryResult> = {};
    try {
      payload = (await method.send(true)) ?? {};
    } finally {
      options?.signal?.removeEventListener('abort', abortHandler);
    }
    const item = extractItem<FileManagerChangesSummaryResult>(payload);
    if (!item) {
      throw new Error('failed to load git changes summary');
    }
    return {
      ...item,
      changeToken: item.changeToken ?? '',
      additions: item.additions ?? null,
      deletions: item.deletions ?? null,
      statsComplete: item.statsComplete === true,
      statsTimedOut: item.statsTimedOut === true,
    };
  },

  async preview(
    projectId: string,
    scopeId: string,
    path: string
  ): Promise<FileManagerPreviewResult> {
    const params = new URLSearchParams();
    params.set('scopeId', scopeId);
    params.set('path', path);
    const payload =
      (await http
        .Get<
          ItemResponse<FileManagerPreviewResult>
        >(`/projects/${projectId}/files/preview?${params.toString()}`)
        .send(true)) ?? {};
    const item = extractItem<FileManagerPreviewResult>(payload);
    if (!item) {
      throw new Error('failed to load file preview');
    }
    return item;
  },

  async diff(projectId: string, scopeId: string, path: string): Promise<FileManagerDiffResult> {
    const params = new URLSearchParams();
    params.set('scopeId', scopeId);
    params.set('path', path);
    const payload =
      (await http
        .Get<
          ItemResponse<FileManagerDiffResult>
        >(`/projects/${projectId}/files/diff?${params.toString()}`)
        .send(true)) ?? {};
    const item = extractItem<FileManagerDiffResult>(payload);
    if (!item) {
      throw new Error('failed to load file diff');
    }
    return item;
  },

  buildContentUrl(
    projectId: string,
    scopeId: string,
    path: string,
    disposition: 'inline' | 'attachment'
  ) {
    const params = new URLSearchParams();
    params.set('scopeId', scopeId);
    params.set('path', path);
    params.set('disposition', disposition);
    return resolveUrl(`/api/v1/projects/${projectId}/files/content?${params.toString()}`);
  },

  async createDirectory(projectId: string, scopeId: string, parentPath: string, name: string) {
    const payload =
      (await http
        .Post<ItemResponse<FileManagerEntry>>(`/projects/${projectId}/files/directories`, {
          scopeId,
          parentPath,
          name,
        })
        .send()) ?? {};
    const item = extractItem<FileManagerEntry>(payload);
    if (!item) {
      throw new Error('failed to create directory');
    }
    return item;
  },

  async rename(projectId: string, scopeId: string, path: string, newName: string) {
    const payload =
      (await http
        .Post<ItemResponse<FileManagerEntry>>(`/projects/${projectId}/files/rename`, {
          scopeId,
          path,
          newName,
        })
        .send()) ?? {};
    const item = extractItem<FileManagerEntry>(payload);
    if (!item) {
      throw new Error('failed to rename file');
    }
    return item;
  },

  async copy(projectId: string, scopeId: string, sourcePaths: string[], destinationPath: string) {
    const payload =
      (await http
        .Post<ItemResponse<FileManagerBulkResult>>(`/projects/${projectId}/files/copy`, {
          scopeId,
          sourcePaths,
          destinationPath,
        })
        .send()) ?? {};
    const item = extractItem<FileManagerBulkResult>(payload);
    if (!item) {
      throw new Error('failed to copy files');
    }
    return item;
  },

  async move(projectId: string, scopeId: string, sourcePaths: string[], destinationPath: string) {
    const payload =
      (await http
        .Post<ItemResponse<FileManagerBulkResult>>(`/projects/${projectId}/files/move`, {
          scopeId,
          sourcePaths,
          destinationPath,
        })
        .send()) ?? {};
    const item = extractItem<FileManagerBulkResult>(payload);
    if (!item) {
      throw new Error('failed to move files');
    }
    return item;
  },

  async remove(projectId: string, scopeId: string, paths: string[]) {
    const payload =
      (await http
        .Post<ItemResponse<FileManagerBulkResult>>(`/projects/${projectId}/files/delete`, {
          scopeId,
          paths,
        })
        .send()) ?? {};
    const item = extractItem<FileManagerBulkResult>(payload);
    if (!item) {
      throw new Error('failed to delete files');
    }
    return item;
  },

  async createArchive(projectId: string, scopeId: string, paths: string[], fileName = '') {
    const payload =
      (await http
        .Post<ItemResponse<FileManagerArchiveJob>>(`/projects/${projectId}/files/archives`, {
          scopeId,
          paths,
          fileName,
        })
        .send()) ?? {};
    const item = extractItem<FileManagerArchiveJob>(payload);
    if (!item) {
      throw new Error('failed to prepare archive');
    }
    return item;
  },

  async createUploadSession(
    projectId: string,
    scopeId: string,
    directoryPath: string,
    fileName: string,
    size: number
  ) {
    const payload =
      (await http
        .Post<ItemResponse<FileManagerUploadSession>>(
          `/projects/${projectId}/files/upload-sessions`,
          {
            scopeId,
            directoryPath,
            fileName,
            size,
          }
        )
        .send()) ?? {};
    const item = extractItem<FileManagerUploadSession>(payload);
    if (!item) {
      throw new Error('failed to start upload session');
    }
    return item;
  },

  async getUploadSession(projectId: string, uploadId: string) {
    const payload =
      (await http
        .Get<
          ItemResponse<FileManagerUploadSession>
        >(`/projects/${projectId}/files/upload-sessions/${uploadId}`)
        .send(true)) ?? {};
    const item = extractItem<FileManagerUploadSession>(payload);
    if (!item) {
      throw new Error('upload session not found');
    }
    return item;
  },

  uploadChunk(
    projectId: string,
    uploadId: string,
    offset: number,
    chunk: Blob,
    options?: {
      total?: number;
      onProgress?: (payload: TransferProgressPayload) => void;
      onXhr?: (xhr: XMLHttpRequest) => void;
    }
  ) {
    return new Promise<FileManagerUploadSession>((resolve, reject) => {
      const xhr = new XMLHttpRequest();
      const targetUrl = resolveUrl(
        `/api/v1/projects/${projectId}/files/upload-sessions/${uploadId}`
      );

      options?.onXhr?.(xhr);
      xhr.open('PATCH', targetUrl, true);
      xhr.withCredentials = true;
      xhr.responseType = 'json';
      xhr.setRequestHeader('Upload-Offset', String(offset));
      xhr.setRequestHeader('Content-Type', 'application/octet-stream');

      const token = readStoredToken();
      if (token) {
        xhr.setRequestHeader('Authorization', token);
      }

      xhr.upload.onprogress = event => {
        if (!options?.onProgress) {
          return;
        }
        const loaded = offset + event.loaded;
        const total = options.total ?? (event.lengthComputable ? offset + event.total : undefined);
        const progress =
          typeof total === 'number' && total > 0
            ? Math.max(0, Math.min(100, Math.round((loaded / total) * 100)))
            : null;
        options.onProgress({
          loaded,
          total,
          progress,
        });
      };

      xhr.onerror = () => {
        reject(new Error('network error while uploading chunk'));
      };

      xhr.onabort = () => {
        reject(createAbortError('upload aborted'));
      };

      xhr.onload = () => {
        const payload = parseJSONPayload(xhr);
        if (xhr.status < 200 || xhr.status >= 300) {
          reject(new ApiError(xhr.status, xhr.statusText || 'Upload failed', payload));
          return;
        }
        const item = extractItem<FileManagerUploadSession>(
          payload as ItemResponse<FileManagerUploadSession>
        );
        if (!item?.uploadId) {
          reject(new Error('upload chunk succeeded but response is invalid'));
          return;
        }
        resolve(item);
      };

      xhr.send(chunk);
    });
  },

  async completeUpload(projectId: string, uploadId: string) {
    const payload =
      (await http
        .Post<
          ItemResponse<FileManagerEntry>
        >(`/projects/${projectId}/files/upload-sessions/${uploadId}/complete`, {})
        .send()) ?? {};
    const item = extractItem<FileManagerEntry>(payload);
    if (!item) {
      throw new Error('failed to finalize upload');
    }
    return item;
  },

  async cancelUpload(projectId: string, uploadId: string) {
    await http.Delete(`/projects/${projectId}/files/upload-sessions/${uploadId}`).send();
  },

  async downloadToBlob(
    sourceUrl: string,
    options?: {
      total?: number;
      signal?: AbortSignal;
      onProgress?: (payload: TransferProgressPayload) => void;
    }
  ) {
    const response = await fetch(resolveUrl(sourceUrl), {
      credentials: 'include',
      headers: readStoredToken() ? { Authorization: readStoredToken() as string } : undefined,
      signal: options?.signal,
    });
    if (!response.ok) {
      let detail = '';
      try {
        detail = normalizeJsonError(await response.json(), '');
      } catch {
        try {
          detail = await response.text();
        } catch {
          detail = '';
        }
      }
      throw new Error(detail || `download failed with status ${response.status}`);
    }

    const totalHeader = response.headers.get('content-length');
    const total =
      options?.total ??
      (totalHeader && Number.isFinite(Number(totalHeader)) ? Number(totalHeader) : undefined);

    if (!response.body) {
      const blob = await response.blob();
      options?.onProgress?.({
        loaded: blob.size,
        total,
        progress:
          typeof total === 'number' && total > 0
            ? Math.max(0, Math.min(100, Math.round((blob.size / total) * 100)))
            : null,
      });
      return blob;
    }

    const reader = response.body.getReader();
    const chunks: Uint8Array[] = [];
    let loaded = 0;

    while (true) {
      const { done, value } = await reader.read();
      if (done) {
        break;
      }
      if (value) {
        chunks.push(value);
        loaded += value.byteLength;
        options?.onProgress?.({
          loaded,
          total,
          progress:
            typeof total === 'number' && total > 0
              ? Math.max(0, Math.min(100, Math.round((loaded / total) * 100)))
              : null,
        });
      }
    }

    return new Blob(chunks);
  },

  saveBlob(blob: Blob, fileName: string) {
    const objectUrl = URL.createObjectURL(blob);
    const anchor = document.createElement('a');
    anchor.href = objectUrl;
    anchor.download = fileName;
    anchor.style.display = 'none';
    document.body.append(anchor);
    anchor.click();
    anchor.remove();
    window.setTimeout(() => {
      URL.revokeObjectURL(objectUrl);
    }, 1000);
  },

  startBrowserDownload(sourceUrl: string) {
    if (typeof document === 'undefined') {
      throw new Error('browser download is unavailable in the current environment');
    }

    const iframe = document.createElement('iframe');
    iframe.style.display = 'none';
    iframe.src = resolveUrl(sourceUrl);
    iframe.setAttribute('aria-hidden', 'true');
    document.body.append(iframe);

    window.setTimeout(() => {
      iframe.remove();
    }, 60_000);
  },
};
