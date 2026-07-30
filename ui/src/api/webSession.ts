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

export type ArchivedQueryResult = {
  items: WebSessionSummary[];
  total: number;
  hasMore: boolean;
  nextOffset: number;
};

export type SessionSearchChunkResult = {
  items: WebSessionSummary[];
  nextCursor?: string;
  done: boolean;
  scanned: number;
  total: number;
};

export type WebSessionHistoryCleanupParams = {
  scope: 'all' | 'projects';
  projectIds: string[];
  olderThanDays: number;
  retainPerProject: number;
};

export type WebSessionHistoryCleanupStorageStats = {
  pageSizeBytes: number;
  pageCount: number;
  freePageCount: number;
  reusableBytes: number;
};

export type WebSessionHistoryCleanupStats = {
  scopedProjectCount: number;
  scopedSessionCount: number;
  historySessionCount: number;
  skippedBusySessionCount: number;
  nonSyncableSessionCount: number;
  itemRowCount: number;
  turnRowCount: number;
  obsoleteItemRowCount: number;
  obsoleteTurnRowCount: number;
  storage: WebSessionHistoryCleanupStorageStats;
};

export type WebSessionHistoryCleanupResult = WebSessionHistoryCleanupStats & {
  clearedSessionIds: string[];
  historyFileFailureCount: number;
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
  readyAt?: string | number | null;
  paused?: boolean;
  createdAt?: string | number | null;
};

export type WebSessionScheduledInputRecord = {
  id?: string;
  action?: 'message' | 'execute_plan' | string;
  targetId?: string;
  mode?: 'send' | 'interrupt' | 'redirect' | 'queue' | string;
  status?: 'scheduled' | 'failed' | 'expired' | 'dispatched' | 'canceled' | string;
  lastError?: string;
  text?: string;
  attachmentIds?: string[];
  scheduleKind?: 'at_time' | 'when_idle' | string;
  scheduledFor?: string | number | null;
  idleSince?: string | number | null;
  blockingReasons?: string[];
  conditionError?: string;
  createdAt?: string | number | null;
  updatedAt?: string | number | null;
  sentAt?: string | number | null;
  canceledAt?: string | number | null;
};

export type WebSessionPendingApprovalRecord = {
  itemId?: string;
  kind?: 'command_approval' | 'file_change_approval' | 'permissions_approval' | string;
  prompt?: string;
  command?: string;
  requestedAt?: string | number | null;
  actionable?: boolean;
};

export type WebSessionSubAgentRecord = {
  threadId?: string;
  parentThreadId?: string | null;
  path?: string;
  nickname?: string;
  role?: string;
  status?:
    | 'pending_init'
    | 'running'
    | 'interrupted'
    | 'completed'
    | 'errored'
    | 'shutdown'
    | 'not_found'
    | string;
  summary?: string;
  currentTurnId?: string | null;
  latestItemId?: string | null;
  latestOrderIndex?: number;
  startedAt?: string | number | null;
  lastActivityAt?: string | number | null;
  endedAt?: string | number | null;
};

export type WebSessionSnapshot = {
  revision?: string;
  unchanged?: boolean;
  session?: WebSessionSummary;
  history?: WebSessionHistoryWindow;
  pendingInputs?: WebSessionPendingInputRecord[];
  scheduledInputs?: WebSessionScheduledInputRecord[];
  pendingApproval?: WebSessionPendingApprovalRecord | null;
  subAgents?: WebSessionSubAgentRecord[];
};

export type WebSessionImportResult = Omit<
  WebSessionSnapshot,
  'session' | 'history' | 'unchanged'
> & {
  session: WebSessionSummary;
  history: WebSessionHistoryWindow;
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
      autoRetryDispatchPendingOnFailure?: boolean;
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
          ...(data.model !== undefined ? { model: data.model } : {}),
          ...(data.reasoningEffort !== undefined ? { reasoningEffort: data.reasoningEffort } : {}),
          workflowMode: data.workflowMode ?? 'default',
          ...(data.permissionLevel !== undefined ? { permissionLevel: data.permissionLevel } : {}),
          activeCallTimeoutEnabled: data.activeCallTimeoutEnabled,
          autoRetryEnabled: data.autoRetryEnabled === true,
          autoRetryScope: data.autoRetryScope ?? 'network_only',
          autoRetryPreset: data.autoRetryPreset ?? 'gentle_stop',
          autoRetryDispatchPendingOnFailure: data.autoRetryDispatchPendingOnFailure === true,
          permissionMode: data.permissionMode ?? '',
          title: data.title ?? '',
        })
        .send()) ?? {};
    if (!body.item) {
      throw new Error('failed to create AI session');
    }
    return body.item;
  },

  async editUserMessage(
    projectId: string,
    sessionId: string,
    itemId: string,
    text: string
  ): Promise<WebSessionSnapshot> {
    const body =
      (await http
        .Post<
          ItemResponse<WebSessionSnapshot>
        >(`/projects/${projectId}/web-sessions/${sessionId}/messages/${itemId}/edit`, { text })
        .send()) ?? {};
    if (!body.item?.session || !body.item.history) {
      throw new Error('failed to create edited message branch');
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
      knownRevision?: string;
    }
  ): Promise<WebSessionSnapshot> {
    const limit =
      typeof options?.limit === 'number' && Number.isFinite(options.limit) ? options.limit : 80;
    const query = new URLSearchParams({ limit: String(limit) });
    if (options?.knownRevision) {
      query.set('knownRevision', options.knownRevision);
    }
    const method = http.Get<ItemResponse<WebSessionSnapshot>>(
      `/projects/${projectId}/web-sessions/${sessionId}/snapshot?${query.toString()}`
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

  async previewHistoryCleanup(
    data: WebSessionHistoryCleanupParams
  ): Promise<WebSessionHistoryCleanupStats> {
    const body =
      (await http
        .Post<
          ItemResponse<WebSessionHistoryCleanupStats>
        >('/system/web-session-history-cleanup/preview', data)
        .send()) ?? {};
    if (!body.item) {
      throw new Error('failed to preview web session history cleanup');
    }
    return body.item;
  },

  async runHistoryCleanup(
    data: WebSessionHistoryCleanupParams
  ): Promise<WebSessionHistoryCleanupResult> {
    const body =
      (await http
        .Post<
          ItemResponse<WebSessionHistoryCleanupResult>
        >('/system/web-session-history-cleanup/run', data)
        .send()) ?? {};
    if (!body.item) {
      throw new Error('failed to run web session history cleanup');
    }
    return body.item;
  },

  async queryArchived(data: {
    projectIds: string[];
    query?: string;
    offset?: number;
    limit?: number;
  }): Promise<ArchivedQueryResult> {
    const body =
      (await http
        .Post<ItemResponse<ArchivedQueryResult>>('/web-sessions/archived/query', {
          projectIds: data.projectIds,
          query: data.query?.trim() || undefined,
          offset: data.offset ?? 0,
          limit: data.limit ?? 20,
        })
        .send()) ?? {};
    if (!body.item) {
      throw new Error('failed to query archived AI sessions');
    }
    return body.item;
  },

  async search(
    data: {
      projectIds: string[];
      query: string;
      includeArchived?: boolean;
      includeBody?: boolean;
      cursor?: string;
      scanLimit?: number;
    },
    options?: { signal?: AbortSignal }
  ): Promise<SessionSearchChunkResult> {
    const method = http.Post<ItemResponse<SessionSearchChunkResult>>('/web-sessions/search', {
      projectIds: data.projectIds,
      query: data.query.trim(),
      includeArchived: data.includeArchived === true,
      includeBody: data.includeBody !== false,
      cursor: data.cursor || undefined,
      scanLimit: data.scanLimit ?? 50,
    });
    const abortHandler = () => {
      method.abort();
    };

    if (options?.signal?.aborted) {
      throw createAbortError();
    }
    options?.signal?.addEventListener('abort', abortHandler, { once: true });
    let body: ItemResponse<SessionSearchChunkResult> = {};
    try {
      body = (await method.send()) ?? {};
    } finally {
      options?.signal?.removeEventListener('abort', abortHandler);
    }
    if (!body.item) {
      throw new Error('failed to search AI sessions');
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

  async importRemoteAttachment(projectId: string, url: string): Promise<WebSessionAttachment> {
    const body =
      (await http
        .Post<
          ItemResponse<WebSessionAttachment>
        >(`/projects/${projectId}/web-sessions/attachments/import-url`, { url })
        .send()) ?? {};
    if (!body.item?.id) {
      throw new Error('remote image download succeeded but no attachment was returned');
    }
    return body.item;
  },

  async importClipboardAttachment(
    projectId: string,
    source: string
  ): Promise<WebSessionAttachment> {
    const body =
      (await http
        .Post<
          ItemResponse<WebSessionAttachment>
        >(`/projects/${projectId}/web-sessions/attachments/import-clipboard`, { source })
        .send()) ?? {};
    if (!body.item?.id) {
      throw new Error('clipboard image import succeeded but no attachment was returned');
    }
    return body.item;
  },
};
