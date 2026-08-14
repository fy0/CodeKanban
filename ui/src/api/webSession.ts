import type {
  WebSessionAgent,
  WebSessionAttachment,
  WebSessionReasoningEffort,
  WebSessionRuntimeConfig,
  WebSessionSummary,
  WebSessionWorkTimingOutcome,
} from '@/types/models';
import { ApiError, urlBase } from '@/api';
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

export type SessionConversationSearchMatch = {
  id: string;
  sourceThreadId?: string;
  sourceTurnId?: string;
  sourceItemId?: string;
  orderIndex: number;
  kind: 'user' | 'assistant' | 'tool' | 'system' | string;
  toolId?: string;
  commandGroupId?: string;
};

export type SessionConversationSearchResult = {
  items: SessionConversationSearchMatch[];
  nextCursor?: string;
  done: boolean;
  total: number;
};

export type WebSessionHistoryCleanupParams = {
  scope: 'all' | 'projects';
  projectIds: string[];
  olderThanDays: number;
  retainPerProject: number;
  archivedOnly?: boolean;
  archivedOlderThanDays?: number;
};

export type WebSessionHistoryCleanupStorageStats = {
  databaseBytes: number;
  walBytes: number;
  freeDiskBytes: number;
  pageSizeBytes: number;
  pageCount: number;
  freePageCount: number;
  reusableBytes: number;
  historyBytes: number;
  historyFileBytes: number;
  itemBytes: number;
  turnBytes: number;
  subAgentBytes: number;
  itemRowCount: number;
  turnRowCount: number;
  subAgentRowCount: number;
  archivedSessionCount: number;
  archivedCacheBytes: number;
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
  estimatedBytes: number;
  historyFileBytes: number;
  storage: WebSessionHistoryCleanupStorageStats;
};

export type WebSessionHistoryCleanupResult = WebSessionHistoryCleanupStats & {
  clearedSessionIds: string[];
  historyFileFailureCount: number;
};

export type WebSessionHistoryArchiveParams = {
  scope: 'all' | 'projects';
  projectIds: string[];
  olderThanDays: number;
};

export type WebSessionHistoryArchiveStats = {
  scopedProjectCount: number;
  scopedSessionCount: number;
  candidateSessionCount: number;
  skippedBusySessionCount: number;
};

export type WebSessionHistoryArchiveResult = WebSessionHistoryArchiveStats & {
  archivedSessionIds: string[];
};

export type WebSessionWorkTimingItemPatch = {
  itemId: string;
  runId: string;
  runDurationMs: number;
  runOutcome: WebSessionWorkTimingOutcome;
};

export type WebSessionWorkTimingCalculationStatus =
  | 'calculated'
  | 'already_current'
  | 'busy'
  | 'partial'
  | 'unavailable'
  | 'failed';

export type WebSessionWorkTimingCalculationResult = {
  status: WebSessionWorkTimingCalculationStatus;
  session: WebSessionSummary;
  items: WebSessionWorkTimingItemPatch[];
  error?: string;
};

export type WebSessionWorkTimingBackfillStatus = {
  remainingSessionCount: number;
  busySessionCount: number;
  completeSessionCount: number;
  partialSessionCount: number;
  unavailableSessionCount: number;
  failedSessionCount: number;
};

export type WebSessionWorkTimingBackfillResult = WebSessionWorkTimingBackfillStatus & {
  attemptedSessionCount: number;
  calculatedSessionCount: number;
  partialResultCount: number;
  unavailableResultCount: number;
  failedResultCount: number;
};

type CountsResponse = {
  counts?: Record<string, number>;
};

function webSessionLocalFileBasePath(projectId: string, sessionId: string) {
  return `/api/v1/projects/${encodeURIComponent(projectId)}/web-sessions/${encodeURIComponent(sessionId)}/local-files`;
}

export function buildWebSessionLocalFileContentUrl(
  projectId: string,
  sessionId: string,
  path: string
) {
  const params = new URLSearchParams();
  params.set('path', path);
  const requestPath = `${webSessionLocalFileBasePath(projectId, sessionId)}/content?${params.toString()}`;
  return urlBase ? new URL(requestPath, urlBase).toString() : requestPath;
}

export type WebSessionHistoryWindow = {
  items: unknown[];
  hasMore: boolean;
  beforeCursor?: string;
  hasLater?: boolean;
  afterCursor?: string;
  total: number;
};

export type WebSessionPiTreeNode = {
  id: string;
  parentId?: string;
  type: string;
  timestamp?: string;
  role?: string;
  label?: string;
  preview?: string;
  active: boolean;
  children: string[];
};

export type WebSessionPiTree = {
  sessionId: string;
  leafId?: string;
  revision: string;
  nodes: WebSessionPiTreeNode[];
};

export type WebSessionPiTreeNavigateResult = {
  tree: WebSessionPiTree;
  editorText?: string;
};

export type WebSessionPiTreeMutationResult = {
  session: WebSessionSummary;
  tree: WebSessionPiTree;
  editorText?: string;
};

export type WebSessionCommandExecutionGroupItem = {
  toolId: string;
  kind: string;
  title: string;
  summary: string;
  command: string;
  input?: unknown;
  output?: string;
  status: 'running' | 'done' | 'error';
  timestamp: string;
  startedAt?: string;
  completedAt?: string;
};

export type WebSessionCommandExecutionGroupDetail = {
  groupId: string;
  kind: string;
  title: string;
  summary: string;
  count: number;
  firstSeq: number;
  lastSeq: number;
  status: 'running' | 'done' | 'error';
  latestToolId?: string;
  items: WebSessionCommandExecutionGroupItem[];
};

export type WebSessionPendingInputRecord = {
  id?: string;
  mode?: 'redirect' | 'queue' | string;
  text?: string;
  attachmentIds?: string[];
  readyAt?: string | number | null;
  paused?: boolean;
  nativeQueued?: boolean;
  createdAt?: string | number | null;
};

export type WebSessionScheduledInputRecord = {
  id?: string;
  action?: 'message' | 'execute_plan' | string;
  targetId?: string;
  mode?: 'send' | 'interrupt' | 'redirect' | 'queue' | string;
  exitPlanMode?: boolean;
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
  async runtimeConfig(): Promise<WebSessionRuntimeConfig> {
    const config = extractItem<WebSessionRuntimeConfig>(
      await http
        .Get<ItemResponse<WebSessionRuntimeConfig>>('/web-sessions/runtime-config', {
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
      agent: WebSessionAgent;
      claudeRuntime?: 'claude' | 'ccr';
      model?: string;
      reasoningEffort?: WebSessionReasoningEffort;
      workflowMode?: 'default' | 'plan';
      permissionLevel?: 'default' | 'elevated' | 'yolo';
      activeCallTimeoutEnabled?: boolean;
      autoRetryEnabled?: boolean;
      autoRetryPolicyMode?: 'default' | 'custom';
      autoRetryScope?: 'network_only' | 'network_and_rate_limit' | 'all_failures';
      autoRetryPreset?: 'gentle_stop' | 'aggressive_stop' | 'sustain_60s';
      autoRetryMaxAttempts?: number;
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
          ...(data.autoRetryPolicyMode !== undefined
            ? { autoRetryPolicyMode: data.autoRetryPolicyMode }
            : {}),
          ...(data.autoRetryScope !== undefined ? { autoRetryScope: data.autoRetryScope } : {}),
          ...(data.autoRetryPreset !== undefined ? { autoRetryPreset: data.autoRetryPreset } : {}),
          ...(data.autoRetryMaxAttempts !== undefined
            ? { autoRetryMaxAttempts: data.autoRetryMaxAttempts }
            : {}),
          ...(data.autoRetryDispatchPendingOnFailure !== undefined
            ? {
                autoRetryDispatchPendingOnFailure: data.autoRetryDispatchPendingOnFailure === true,
              }
            : {}),
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
      agent?: 'codex' | 'pi';
      aiSessionId?: string;
      sessionId?: string;
      mode?: 'fast' | 'deep';
    }
  ): Promise<WebSessionImportResult> {
    const body =
      (await http
        .Post<ItemResponse<WebSessionImportResult>>(`/projects/${projectId}/web-sessions/import`, {
          agent: data.agent ?? 'codex',
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

  async tree(projectId: string, sessionId: string): Promise<WebSessionPiTree> {
    const body =
      (await http
        .Get<
          ItemResponse<WebSessionPiTree>
        >(`/projects/${encodeURIComponent(projectId)}/web-sessions/${encodeURIComponent(sessionId)}/tree`)
        .send(true)) ?? {};
    if (!body.item) {
      throw new Error('failed to load Pi session tree');
    }
    return body.item;
  },

  async navigateTree(
    projectId: string,
    sessionId: string,
    data: { targetId: string; revision: string; summarize?: boolean }
  ): Promise<WebSessionPiTreeNavigateResult> {
    const body =
      (await http
        .Post<
          ItemResponse<WebSessionPiTreeNavigateResult>
        >(`/projects/${encodeURIComponent(projectId)}/web-sessions/${encodeURIComponent(sessionId)}/tree/navigate`, data)
        .send()) ?? {};
    if (!body.item?.tree) {
      throw new Error('failed to navigate Pi session tree');
    }
    return body.item;
  },

  async forkTree(
    projectId: string,
    sessionId: string,
    data: { targetId: string; revision: string }
  ): Promise<WebSessionPiTreeMutationResult> {
    const body =
      (await http
        .Post<
          ItemResponse<WebSessionPiTreeMutationResult>
        >(`/projects/${encodeURIComponent(projectId)}/web-sessions/${encodeURIComponent(sessionId)}/tree/fork`, data)
        .send()) ?? {};
    if (!body.item?.session?.id) {
      throw new Error('failed to fork Pi session tree');
    }
    return body.item;
  },

  async cloneTree(
    projectId: string,
    sessionId: string,
    data: { revision: string }
  ): Promise<WebSessionPiTreeMutationResult> {
    const body =
      (await http
        .Post<
          ItemResponse<WebSessionPiTreeMutationResult>
        >(`/projects/${encodeURIComponent(projectId)}/web-sessions/${encodeURIComponent(sessionId)}/tree/clone`, data)
        .send()) ?? {};
    if (!body.item?.session?.id) {
      throw new Error('failed to clone Pi session tree');
    }
    return body.item;
  },

  async history(
    projectId: string,
    sessionId: string,
    options?: {
      beforeCursor?: string;
      afterCursor?: string;
      limit?: number;
    }
  ): Promise<WebSessionHistoryWindow> {
    const params = new URLSearchParams();
    if (options?.beforeCursor) {
      params.set('beforeCursor', options.beforeCursor);
    }
    if (options?.afterCursor) {
      params.set('afterCursor', options.afterCursor);
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

  async calculateWorkTiming(
    projectId: string,
    sessionId: string
  ): Promise<WebSessionWorkTimingCalculationResult> {
    const body =
      (await http
        .Post<
          ItemResponse<WebSessionWorkTimingCalculationResult>
        >(`/projects/${encodeURIComponent(projectId)}/web-sessions/${encodeURIComponent(sessionId)}/work-timing/calculate`)
        .send()) ?? {};
    if (!body.item) {
      throw new Error('failed to calculate web session work timing');
    }
    return body.item;
  },

  async commandGroupDetail(
    projectId: string,
    sessionId: string,
    groupId: string
  ): Promise<WebSessionCommandExecutionGroupDetail> {
    const body =
      (await http
        .Get<
          ItemResponse<WebSessionCommandExecutionGroupDetail>
        >(`/projects/${encodeURIComponent(projectId)}/web-sessions/${encodeURIComponent(sessionId)}/command-groups/${encodeURIComponent(groupId)}`, { cacheFor: 0 })
        .send(true)) ?? {};
    if (!body.item) {
      throw new Error('failed to load tool group detail');
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

  async historyStorageOverview(): Promise<WebSessionHistoryCleanupStorageStats> {
    const body =
      (await http
        .Get<
          ItemResponse<WebSessionHistoryCleanupStorageStats>
        >('/system/web-session-storage-overview', { cacheFor: 0 })
        .send(true)) ?? {};
    if (!body.item) {
      throw new Error('failed to load web session storage overview');
    }
    return body.item;
  },

  async previewHistoryArchive(
    data: WebSessionHistoryArchiveParams
  ): Promise<WebSessionHistoryArchiveStats> {
    const body =
      (await http
        .Post<
          ItemResponse<WebSessionHistoryArchiveStats>
        >('/system/web-session-history-archive/preview', data)
        .send()) ?? {};
    if (!body.item) {
      throw new Error('failed to preview web session history archive');
    }
    return body.item;
  },

  async runHistoryArchive(
    data: WebSessionHistoryArchiveParams
  ): Promise<WebSessionHistoryArchiveResult> {
    const body =
      (await http
        .Post<
          ItemResponse<WebSessionHistoryArchiveResult>
        >('/system/web-session-history-archive/run', data)
        .send()) ?? {};
    if (!body.item) {
      throw new Error('failed to archive web sessions');
    }
    return body.item;
  },

  async workTimingBackfillStatus(): Promise<WebSessionWorkTimingBackfillStatus> {
    const body =
      (await http
        .Get<
          ItemResponse<WebSessionWorkTimingBackfillStatus>
        >('/system/web-session-work-timing-backfill/status', { cacheFor: 0 })
        .send(true)) ?? {};
    if (!body.item) {
      throw new Error('failed to load web session work timing backfill status');
    }
    return body.item;
  },

  async runWorkTimingBackfill(limit = 50): Promise<WebSessionWorkTimingBackfillResult> {
    const body =
      (await http
        .Post<
          ItemResponse<WebSessionWorkTimingBackfillResult>
        >('/system/web-session-work-timing-backfill/run', { limit })
        .send()) ?? {};
    if (!body.item) {
      throw new Error('failed to run web session work timing backfill');
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

  async searchConversation(
    projectId: string,
    sessionId: string,
    data: {
      query: string;
      includeUser: boolean;
      includeAssistant: boolean;
      includeTools: boolean;
      includeSystem: boolean;
      sourceThreadId?: string;
      cursor?: string;
      limit?: number;
    },
    options?: { signal?: AbortSignal }
  ): Promise<SessionConversationSearchResult> {
    const method = http.Post<ItemResponse<SessionConversationSearchResult>>(
      `/projects/${encodeURIComponent(projectId)}/web-sessions/${encodeURIComponent(sessionId)}/search`,
      {
        query: data.query.trim(),
        includeUser: data.includeUser === true,
        includeAssistant: data.includeAssistant === true,
        includeTools: data.includeTools === true,
        includeSystem: data.includeSystem === true,
        sourceThreadId: data.sourceThreadId || undefined,
        cursor: data.cursor || undefined,
        limit: data.limit ?? 100,
      }
    );
    const abortHandler = () => {
      method.abort();
    };

    if (options?.signal?.aborted) {
      throw createAbortError();
    }
    options?.signal?.addEventListener('abort', abortHandler, { once: true });
    let body: ItemResponse<SessionConversationSearchResult> = {};
    try {
      body = (await method.send()) ?? {};
    } finally {
      options?.signal?.removeEventListener('abort', abortHandler);
    }
    if (!body.item) {
      throw new Error('failed to search AI session conversation');
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

  async probeLocalFile(projectId: string, sessionId: string, path: string): Promise<void> {
    const response = await fetch(buildWebSessionLocalFileContentUrl(projectId, sessionId, path), {
      method: 'HEAD',
      credentials: 'include',
    });
    if (!response.ok) {
      throw new ApiError(
        response.status,
        response.statusText || 'Local file is unavailable',
        undefined
      );
    }
  },

  async openLocalFileLocation(projectId: string, sessionId: string, path: string): Promise<void> {
    await http
      .Post(
        `/projects/${encodeURIComponent(projectId)}/web-sessions/${encodeURIComponent(sessionId)}/local-files/open-location`,
        { path }
      )
      .send();
  },

  startLocalFileDownload(projectId: string, sessionId: string, path: string) {
    if (typeof document === 'undefined') {
      throw new Error('browser download is unavailable in the current environment');
    }

    const iframe = document.createElement('iframe');
    iframe.style.display = 'none';
    iframe.src = buildWebSessionLocalFileContentUrl(projectId, sessionId, path);
    iframe.setAttribute('aria-hidden', 'true');
    document.body.append(iframe);

    window.setTimeout(() => {
      iframe.remove();
    }, 60_000);
  },
};
