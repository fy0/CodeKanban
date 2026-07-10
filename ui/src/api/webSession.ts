import type {
  WebSessionAttachment,
  WebSessionCodexRuntimeConfig,
  WebSessionReasoningEffort,
  WebSessionSummary,
} from '@/types/models';
import { urlBase } from '@/api';
import { extractItem } from './response';
import { http } from './http';

type ItemResponse<T> = {
  item?: T;
};

function createAbortError() {
  if (typeof DOMException !== 'undefined') {
    return new DOMException('The operation was aborted.', 'AbortError');
  }
  const error = new Error('The operation was aborted.');
  error.name = 'AbortError';
  return error;
}

export type WebSessionAttachmentUploadProgress = {
  loaded: number;
  total?: number;
  percent: number | null;
};

type ArchivedQueryResult = {
  items: WebSessionSummary[];
  total: number;
  hasMore: boolean;
  nextOffset: number;
};

type CountsResponse = {
  counts?: Record<string, number>;
};

export type WebSessionHistoryWindow = {
  items: unknown[];
  hasMore: boolean;
  beforeCursor?: string;
  total: number;
};

export type WebSessionPendingInputRecord = {
  id?: string;
  mode?: 'redirect' | 'queue' | string;
  text?: string;
  attachmentIds?: string[];
  createdAt?: string | number | null;
};

export type WebSessionScheduledInputRecord = {
  id?: string;
  mode?: 'send' | 'interrupt' | 'redirect' | 'queue' | string;
  status?: 'scheduled' | 'failed' | 'dispatched' | 'canceled' | string;
  text?: string;
  attachmentIds?: string[];
  scheduledFor?: string | number | null;
  createdAt?: string | number | null;
  updatedAt?: string | number | null;
  sentAt?: string | number | null;
  canceledAt?: string | number | null;
};

export type WebSessionSnapshot = {
  session: WebSessionSummary;
  history: WebSessionHistoryWindow;
  pendingInputs?: WebSessionPendingInputRecord[];
  scheduledInputs?: WebSessionScheduledInputRecord[];
};

export type WebSessionImportResult = WebSessionSnapshot & {
  created: boolean;
  reused: boolean;
  synced: boolean;
};

export const webSessionApi = {
  async runtimeConfig(): Promise<WebSessionCodexRuntimeConfig> {
    const config = extractItem<WebSessionCodexRuntimeConfig>(
      await http
        .Get<ItemResponse<WebSessionCodexRuntimeConfig>>('/web-sessions/runtime-config', {
          cacheFor: 0,
        })
        .send(true)
    );
    if (!config) {
      throw new Error('failed to load AI session runtime config');
    }
    return config;
  },

  async list(projectId: string): Promise<WebSessionSummary[]> {
    const body =
      (await http
        .Get<{ items?: WebSessionSummary[] }>(`/projects/${projectId}/web-sessions`)
        .send(true)) ?? {};
    return body.items ?? [];
  },

  async counts(): Promise<Record<string, number>> {
    const body = (await http.Get<CountsResponse>('/web-sessions/counts').send(true)) ?? {};
    return body.counts ?? {};
  },

  async create(
    projectId: string,
    data: {
      worktreeId?: string;
      agent: 'claude' | 'codex';
      claudeRuntime?: 'claude' | 'ccr';
      model?: string;
      reasoningEffort?: WebSessionReasoningEffort;
      workflowMode?: 'default' | 'plan';
      permissionLevel?: 'default' | 'elevated' | 'yolo';
      activeCallTimeoutEnabled?: boolean;
      autoRetryEnabled?: boolean;
      autoRetryScope?: 'network_only' | 'network_and_rate_limit' | 'all_failures';
      autoRetryPreset?: 'gentle_stop' | 'aggressive_stop' | 'sustain_60s';
      permissionMode?: string;
      title?: string;
    }
  ): Promise<WebSessionSummary> {
    const body =
      (await http
        .Post<ItemResponse<WebSessionSummary>>(`/projects/${projectId}/web-sessions`, {
          worktreeId: data.worktreeId ?? '',
          agent: data.agent,
          claudeRuntime: data.claudeRuntime ?? 'claude',
          model: data.model ?? '',
          reasoningEffort: data.reasoningEffort ?? 'default',
          workflowMode: data.workflowMode ?? 'default',
          permissionLevel: data.permissionLevel ?? 'elevated',
          activeCallTimeoutEnabled: data.activeCallTimeoutEnabled,
          autoRetryEnabled: data.autoRetryEnabled === true,
          autoRetryScope: data.autoRetryScope ?? 'network_only',
          autoRetryPreset: data.autoRetryPreset ?? 'gentle_stop',
          permissionMode: data.permissionMode ?? '',
          title: data.title ?? '',
        })
        .send()) ?? {};
    if (!body.item) {
      throw new Error('failed to create AI session');
    }
    return body.item;
  },

  async importSession(
    projectId: string,
    data: {
      aiSessionId?: string;
      sessionId?: string;
      mode?: 'fast' | 'deep';
    }
  ): Promise<WebSessionImportResult> {
    const body =
      (await http
        .Post<ItemResponse<WebSessionImportResult>>(`/projects/${projectId}/web-sessions/import`, {
          aiSessionId: data.aiSessionId ?? '',
          sessionId: data.sessionId ?? '',
          ...(data.mode ? { mode: data.mode } : {}),
        })
        .send()) ?? {};
    if (!body.item) {
      throw new Error('failed to import AI session');
    }
    return body.item;
  },

  async archive(projectId: string, sessionId: string): Promise<WebSessionSummary> {
    const body =
      (await http
        .Post<
          ItemResponse<WebSessionSummary>
        >(`/projects/${projectId}/web-sessions/${sessionId}/archive`)
        .send()) ?? {};
    if (!body.item) {
      throw new Error('failed to archive AI session');
    }
    return body.item;
  },

  async unarchive(projectId: string, sessionId: string): Promise<WebSessionSummary> {
    const body =
      (await http
        .Post<
          ItemResponse<WebSessionSummary>
        >(`/projects/${projectId}/web-sessions/${sessionId}/unarchive`)
        .send()) ?? {};
    if (!body.item) {
      throw new Error('failed to unarchive AI session');
    }
    return body.item;
  },

  async snapshot(
    projectId: string,
    sessionId: string,
    options?: {
      limit?: number;
      signal?: AbortSignal;
    }
  ): Promise<WebSessionSnapshot> {
    const limit =
      typeof options?.limit === 'number' && Number.isFinite(options.limit) ? options.limit : 80;
    const method = http.Get<ItemResponse<WebSessionSnapshot>>(
      `/projects/${projectId}/web-sessions/${sessionId}/snapshot?limit=${limit}`
    );
    const abortHandler = () => {
      method.abort();
    };

    if (options?.signal?.aborted) {
      throw createAbortError();
    }

    options?.signal?.addEventListener('abort', abortHandler, { once: true });
    let body: ItemResponse<WebSessionSnapshot> = {};
    try {
      body = (await method.send(true)) ?? {};
    } finally {
      options?.signal?.removeEventListener('abort', abortHandler);
    }
    if (!body.item) {
      throw new Error('failed to load AI session snapshot');
    }
    return body.item;
  },

  async history(
    projectId: string,
    sessionId: string,
    options?: {
      beforeCursor?: string;
      limit?: number;
    }
  ): Promise<WebSessionHistoryWindow> {
    const params = new URLSearchParams();
    if (options?.beforeCursor) {
      params.set('beforeCursor', options.beforeCursor);
    }
    if (typeof options?.limit === 'number' && Number.isFinite(options.limit)) {
      params.set('limit', String(Math.max(1, Math.trunc(options.limit))));
    }
    const suffix = params.toString();
    const body =
      (await http
        .Get<
          ItemResponse<WebSessionHistoryWindow>
        >(`/projects/${projectId}/web-sessions/${sessionId}/history${suffix ? `?${suffix}` : ''}`)
        .send(true)) ?? {};
    if (!body.item) {
      throw new Error('failed to load AI session history');
    }
    return body.item;
  },

  async sync(
    projectId: string,
    sessionId: string,
    mode?: 'fast' | 'deep',
    clearExisting = false
  ): Promise<WebSessionSnapshot> {
    const body =
      (await http
        .Post<ItemResponse<WebSessionSnapshot>>(
          `/projects/${projectId}/web-sessions/${sessionId}/sync`,
          {
            ...(mode ? { mode } : {}),
            clearExisting,
          }
        )
        .send()) ?? {};
    if (!body.item) {
      throw new Error('failed to sync AI session');
    }
    return body.item;
  },

  async delete(projectId: string, sessionId: string): Promise<void> {
    await http.Delete(`/projects/${projectId}/web-sessions/${sessionId}`).send();
  },

  async queryArchived(data: {
    projectIds: string[];
    offset?: number;
    limit?: number;
  }): Promise<ArchivedQueryResult> {
    const body =
      (await http
        .Post<ItemResponse<ArchivedQueryResult>>('/web-sessions/archived/query', {
          projectIds: data.projectIds,
          offset: data.offset ?? 0,
          limit: data.limit ?? 20,
        })
        .send()) ?? {};
    if (!body.item) {
      throw new Error('failed to query archived AI sessions');
    }
    return body.item;
  },

  async uploadAttachment(
    projectId: string,
    file: File,
    options?: {
      onProgress?: (progress: WebSessionAttachmentUploadProgress) => void;
    }
  ): Promise<WebSessionAttachment> {
    const formData = new FormData();
    formData.append('file', file);

    return new Promise<WebSessionAttachment>((resolve, reject) => {
      const xhr = new XMLHttpRequest();
      const uploadUrl = urlBase
        ? new URL(`/api/v1/projects/${projectId}/web-sessions/attachments`, urlBase).toString()
        : `/api/v1/projects/${projectId}/web-sessions/attachments`;

      xhr.open('POST', uploadUrl, true);
      xhr.withCredentials = true;
      xhr.responseType = 'json';

      xhr.upload.onprogress = event => {
        if (!options?.onProgress) {
          return;
        }

        const percent =
          event.lengthComputable && event.total > 0
            ? Math.max(0, Math.min(100, Math.round((event.loaded / event.total) * 100)))
            : null;

        options.onProgress({
          loaded: event.loaded,
          total: event.lengthComputable ? event.total : undefined,
          percent,
        });
      };

      xhr.onerror = () => {
        reject(new Error('network error while uploading attachment'));
      };

      xhr.onload = () => {
        let payload: unknown = xhr.response;

        if (!payload && xhr.responseText) {
          try {
            payload = JSON.parse(xhr.responseText);
          } catch {
            payload = xhr.responseText;
          }
        }

        if (xhr.status < 200 || xhr.status >= 300) {
          const detail =
            typeof payload === 'object' && payload !== null && 'detail' in payload
              ? String((payload as { detail?: string }).detail || '')
              : '';
          reject(new Error(detail || `upload failed with status ${xhr.status}`));
          return;
        }

        const item = extractItem<WebSessionAttachment>(
          payload as ItemResponse<WebSessionAttachment>
        );
        if (!item?.id) {
          reject(new Error('upload succeeded but no attachment was returned'));
          return;
        }

        resolve(item);
      };

      xhr.send(formData);
    });
  },
};
