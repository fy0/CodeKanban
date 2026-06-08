import EventEmitter from 'eventemitter3';
import { defineStore } from 'pinia';
import { computed, reactive, ref } from 'vue';
import {
  webSessionApi,
  type WebSessionAttachmentUploadProgress,
  type WebSessionImportResult,
} from '@/api/webSession';
import type {
  WebSessionAttachment,
  WebSessionContextWindowSource,
  WebSessionSummary,
} from '@/types/models';
import {
  buildWebSessionSnapshotVersion,
  compareWebSessionSnapshotVersion,
  selectLatestWebSessionSnapshotVersion,
  shouldApplyIncomingWebSessionSnapshot,
  type WebSessionSnapshotVersion,
  type WebSessionSnapshotVersionInput,
} from '@/stores/webSessionSnapshotVersion';
import { normalizeWebSessionSyncState } from '@/utils/webSessionSyncState';
import { buildUploadImageFileName } from '@/utils/webSessionImages';
import { resolveWsUrl } from '@/utils/ws';

type WireFrameKind = 'ack' | 'snap' | 'evt' | 'err' | 'hb';
type WireHeartbeatOp = 'ping' | 'pong' | 'focus';
type WebSessionSocketKind = 'event' | 'command';
type SessionStatus = WebSessionSummary['status'];
type SessionAssistantState =
  | 'working'
  | 'waiting_approval'
  | 'waiting_input'
  | 'waiting_plan_approval';

type WireSession = {
  id: string;
  pid: string;
  wid?: string | null;
  oi?: number;
  ag: 'claude' | 'codex';
  cr?: 'claude' | 'ccr';
  md: string;
  re?: 'default' | 'none' | 'low' | 'medium' | 'high' | 'xhigh';
  wm: 'default' | 'plan';
  pl: 'default' | 'elevated' | 'yolo';
  acte?: boolean;
  ae?: boolean;
  ars?: 'network_only' | 'network_and_rate_limit' | 'all_failures';
  arp?: 'gentle_stop' | 'aggressive_stop' | 'sustain_60s';
  ttl: string;
  cwd: string;
  nsid?: string | null;
  st: SessionStatus;
  ast?: SessionAssistantState | null;
  unr: boolean;
  aa?: number | null;
  act?: number | null;
  sta?: number | null;
  ca?: number | null;
  lu: number;
  lma?: number | null;
  asu?: number | null;
  sk: string;
  ss: 'fresh' | 'stale' | 'missing' | 'syncing' | 'error';
  lsm?: 'fast' | 'deep';
  sca?: number | null;
  sua?: number | null;
  lsa?: number | null;
  tp?: string | null;
  tpv?: string | null;
  tc?: number;
  ic?: number;
  se?: string | null;
  usa?: {
    in?: number;
    cin?: number;
    out?: number;
  };
  cea?: {
    in?: number;
    cin?: number;
    out?: number;
    usd?: number;
  };
  ltu?: {
    in?: number;
    cin?: number;
    out?: number;
    usd?: number;
  };
  cem?: 'cumulative_total' | 'since_compaction' | 'latest_turn_delta' | 'latest_token_count';
  lcca?: number | null;
  cost?: number;
  cwt?: number | null;
  cws?: WebSessionContextWindowSource;
};

type WirePendingInput = {
  id?: string;
  m?: 'redirect' | 'queue' | string;
  txt?: string;
  atts?: string[];
  ca?: number | null;
};

type WireScheduledInput = {
  id?: string;
  m?: 'send' | 'interrupt' | 'redirect' | 'queue' | string;
  st?: 'scheduled' | 'failed' | 'dispatched' | 'canceled' | string;
  txt?: string;
  atts?: string[];
  sf?: number | null;
  ca?: number | null;
  ua?: number | null;
  sa?: number | null;
  xa?: number | null;
};

type WireHistoryItem = {
  id: string;
  stid?: string | null;
  siid?: string | null;
  oi: number;
  kd: 'user' | 'assistant' | 'system' | 'tool';
  tp: string;
  txt?: string;
  ts2?: number | null;
  obs?: number | null;
  atts?: Array<{
    id: string;
    name: string;
    mime?: string;
    sz?: number;
    path?: string;
  }>;
  tl?: {
    id: string;
    name: string;
    kind?: string;
    in?: unknown;
    out?: string;
    st: 'running' | 'done' | 'error' | string;
    meta?: Record<string, unknown>;
    cg?: {
      id: string;
      count: number;
      firstSeq?: number;
      lastSeq?: number;
      latestToolId?: string;
      compacted?: boolean;
    };
  } | null;
  lvl?: 'info' | 'warn' | 'error' | string;
  dn?: boolean;
  dt?: {
    type: 'approval_request' | 'approval_response' | 'user_input_request' | 'user_input_response';
    prompt?: string;
    questions?: WebSessionUserInputQuestion[];
    answers?: WebSessionHistoryAnswerEntry[];
    action?: 'approve' | 'reject' | string;
  } | null;
  pl?: Record<string, unknown>;
};

type WireFrame = {
  v: number;
  k: WireFrameKind;
  rid?: string;
  sid?: string;
  ts: number;
  op?: string;
  p?: unknown;
  ok?: number;
  s?: WireSession;
  h?: {
    its: WireHistoryItem[];
    hm: boolean;
    bc?: string;
    tot: number;
  };
  i?: WireHistoryItem;
  pi?: WirePendingInput[];
  si?: WireScheduledInput[];
  code?: string;
  msg?: string;
  retry?: boolean;
};

export interface WebSessionToolBlock {
  id: string;
  name: string;
  kind?: string;
  input?: unknown;
  output?: string;
  status: 'running' | 'done' | 'error';
  startedAt?: number;
  meta?: Record<string, unknown>;
  commandGroup?: {
    id: string;
    count: number;
    firstSeq?: number;
    lastSeq?: number;
    latestToolId?: string;
    compacted?: boolean;
  };
}

export interface WebSessionLiveSubAgent {
  id: string;
  title: string;
  summary: string;
  startedAt?: number;
}

export interface WebSessionHistoryAnswerEntry {
  id: string;
  label: string;
  values: string[];
  masked?: boolean;
}

export interface WebSessionHistoryDetail {
  type: 'approval_request' | 'approval_response' | 'user_input_request' | 'user_input_response';
  prompt?: string;
  questions?: WebSessionUserInputQuestion[];
  answers?: WebSessionHistoryAnswerEntry[];
  action?: 'approve' | 'reject' | string;
}

export interface WebSessionBlock {
  key: string;
  id: string;
  sourceTurnId?: string | null;
  sourceItemId?: string | null;
  orderIndex: number;
  kind: 'user' | 'assistant' | 'system' | 'tool';
  itemType: string;
  text: string;
  timestamp: number;
  observedAt?: number | null;
  attachments: Array<{
    id: string;
    name: string;
    mime?: string;
    size?: number;
    path?: string;
  }>;
  tool?: WebSessionToolBlock;
  level?: 'info' | 'warn' | 'error';
  done?: boolean;
  detail?: WebSessionHistoryDetail;
  payload?: Record<string, unknown>;
}

export interface WebSessionApprovalState {
  id: string;
  prompt: string;
  requestedAt: number;
  stale: boolean;
  recoveryReason?: string;
  recoveryMessage?: string;
}

export interface WebSessionUserInputOption {
  label: string;
  description: string;
}

export interface WebSessionUserInputQuestion {
  id: string;
  header: string;
  question: string;
  multiSelect: boolean;
  isOther: boolean;
  isSecret: boolean;
  options: WebSessionUserInputOption[];
}

export interface WebSessionUserInputState {
  id: string;
  itemId: string;
  prompt: string;
  questions: WebSessionUserInputQuestion[];
  requestedAt: number;
  stale: boolean;
  recoveryReason?: string;
  recoveryMessage?: string;
}

export interface WebSessionLiveState {
  phase:
    | 'idle'
    | 'starting'
    | 'thinking'
    | 'retrying'
    | 'tool'
    | 'waiting_approval'
    | 'waiting_plan_approval'
    | 'waiting_input'
    | 'done'
    | 'error';
  running: boolean;
  updatedAt: number;
  startedAt?: number;
  tool?: {
    id: string;
    name: string;
    kind?: string;
    summary?: string;
    count?: number;
    groupId?: string;
    startedAt?: number;
  };
  activeSubAgents?: WebSessionLiveSubAgent[];
  activeSubAgentCount?: number;
  approval?: WebSessionApprovalState | null;
  userInput?: WebSessionUserInputState | null;
  errorMessage?: string;
  retry?: {
    code: string;
    message: string;
    attempt?: number;
    maxAttempts?: number;
  };
}

export interface WebSessionPendingInput {
  id: string;
  mode: 'redirect' | 'queue';
  text: string;
  attachmentIds: string[];
  createdAt: number;
}

export interface WebSessionScheduledInput {
  id: string;
  mode: 'send' | 'interrupt' | 'queue';
  status: 'scheduled' | 'failed';
  text: string;
  attachmentIds: string[];
  scheduledFor: number;
  createdAt: number;
  updatedAt: number;
  sentAt: number | null;
  canceledAt: number | null;
}

type RuntimeMutationStateSnapshot = {
  blockCount: number;
  historyTotal: number;
  pendingInputCount: number;
  livePhase: WebSessionLiveState['phase'];
  liveRunning: boolean;
  liveUpdatedAt: number;
  approvalId: string;
  userInputId: string;
};

type RuntimeMutationHydrationOptions = {
  label: string;
  timeoutMs?: number;
  passiveWaitMs?: number;
  snapshotPollMs?: number;
  predicate: () => boolean;
};

export interface WebSessionDraftState {
  text: string;
  attachments: WebSessionAttachment[];
  updatedAt: number;
}

export interface WebSessionPendingUserInputDraftState {
  selections: Record<string, string[]>;
  drafts: Record<string, string>;
  updatedAt: number;
}

export interface WebSessionDraftAttachmentUploadState {
  id: string;
  fileName: string;
  currentFileIndex: number;
  totalFiles: number;
  loaded: number;
  total?: number;
  percent: number | null;
}

export interface WebSessionDraftAttachmentUploadError {
  fileName: string;
  message: string;
}

export interface WebSessionDraftAttachmentUploadBatchResult {
  attachments: WebSessionAttachment[];
  errors: WebSessionDraftAttachmentUploadError[];
}

type WebSessionAssistantDescriptor = {
  type: 'claude-code' | 'codex';
  name: 'Claude Code' | 'Codex';
  displayName: 'Claude Code' | 'Codex';
};

export interface WebSessionAIEvent {
  sessionId: string;
  sessionTitle: string;
  projectId: string;
  assistant: WebSessionAssistantDescriptor;
}

export interface WebSessionApprovalEvent extends WebSessionAIEvent {
  approval: WebSessionApprovalState;
}

type HistoryMeta = {
  hasMore: boolean;
  beforeCursor: string;
  total: number;
  loading: boolean;
};

type ArchivedListMeta = {
  scopeKey: string;
  total: number;
  offset: number;
  hasMore: boolean;
  loading: boolean;
};

type ArchivedListScopeState = {
  projectIds: string[];
  sessionIds: string[];
  meta: ArchivedListMeta;
};

type SyncSessionOptions = {
  rememberActive?: boolean;
};

type LoadSessionSnapshotOptions = {
  rememberActive?: boolean;
  signal?: AbortSignal;
  preserveArchivedPosition?: boolean;
};

type PendingAutoRetryOverride = {
  enabled: boolean;
  scope: WebSessionSummary['autoRetryScope'];
  preset: WebSessionSummary['autoRetryPreset'];
  appliedAt: number;
  expiresAt: number;
};

type PendingActiveCallTimeoutOverride = {
  enabled: boolean;
  appliedAt: number;
  expiresAt: number;
};

const ACTIVE_SESSION_STORAGE_KEY = 'kanban-web-active-session';
const SESSION_DRAFT_STORAGE_KEY = 'kanban-web-session-drafts';
const COMMAND_WS_PATH = '/api/v1/web-sessions/ws';
const EVENTS_WS_PATH = '/api/v1/web-sessions/events';
const WEB_SESSION_HEARTBEAT_INTERVAL_MS = 15000;
const WEB_SESSION_SOCKET_IDLE_TIMEOUT_MS = WEB_SESSION_HEARTBEAT_INTERVAL_MS * 2 + 5000;
const WEB_SESSION_SOCKET_WATCHDOG_INTERVAL_MS = 5000;
const WEB_SESSION_EVENT_RECONNECT_BASE_DELAY_MS = 1200;
const WEB_SESSION_EVENT_RECONNECT_MAX_DELAY_MS = 15000;
const WEB_SESSION_AUTO_RETRY_OPTIMISTIC_TTL_MS = 5000;
const WEB_SESSION_RUNTIME_MUTATION_PASSIVE_WAIT_MS = 120;
const WEB_SESSION_RUNTIME_MUTATION_PASSIVE_POLL_MS = 16;
const WEB_SESSION_RUNTIME_MUTATION_SNAPSHOT_POLL_MS = 180;
const WEB_SESSION_RUNTIME_MUTATION_TIMEOUT_MS = 2500;
const WEB_SESSION_RUNTIME_ABORT_TIMEOUT_MS = 5000;
const WEB_SESSION_MAX_RETAINED_BLOCKS = 400;
const WEB_SESSION_MIN_RETAINED_BLOCKS = 160;
const PROCESS_RESTART_REASON = 'process_restart';
const DEFAULT_RECOVERY_MESSAGE =
  'The previous run was interrupted because the app restarted. Send a new message to continue.';

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === 'object' && !Array.isArray(value));
}

function normalizeStoredAttachment(value: unknown): WebSessionAttachment | null {
  if (!isRecord(value)) {
    return null;
  }
  const id = typeof value.id === 'string' ? value.id.trim() : '';
  const name = typeof value.name === 'string' ? value.name.trim() : '';
  if (!id || !name) {
    return null;
  }
  return {
    id,
    name,
    mime: typeof value.mime === 'string' ? value.mime : '',
    size: typeof value.size === 'number' && Number.isFinite(value.size) ? value.size : 0,
    path: typeof value.path === 'string' ? value.path : '',
    createdAt: typeof value.createdAt === 'string' ? value.createdAt : '',
  };
}

function normalizeStoredDrafts(
  value: unknown
): Record<string, Record<string, WebSessionDraftState>> {
  if (!isRecord(value)) {
    return {};
  }
  const result: Record<string, Record<string, WebSessionDraftState>> = {};
  Object.entries(value).forEach(([projectId, projectValue]) => {
    if (!projectId.trim() || !isRecord(projectValue)) {
      return;
    }
    const projectDrafts: Record<string, WebSessionDraftState> = {};
    Object.entries(projectValue).forEach(([sessionId, draftValue]) => {
      if (!sessionId.trim() || !isRecord(draftValue)) {
        return;
      }
      const text = typeof draftValue.text === 'string' ? draftValue.text : '';
      const attachments = Array.isArray(draftValue.attachments)
        ? draftValue.attachments
            .map(item => normalizeStoredAttachment(item))
            .filter((item): item is WebSessionAttachment => Boolean(item))
        : [];
      if (!text.trim() && attachments.length === 0) {
        return;
      }
      projectDrafts[sessionId] = {
        text,
        attachments,
        updatedAt:
          typeof draftValue.updatedAt === 'number' && Number.isFinite(draftValue.updatedAt)
            ? draftValue.updatedAt
            : Date.now(),
      };
    });
    if (Object.keys(projectDrafts).length > 0) {
      result[projectId] = projectDrafts;
    }
  });
  return result;
}

function loadStoredActiveSessions() {
  try {
    const raw = localStorage.getItem(ACTIVE_SESSION_STORAGE_KEY);
    if (!raw) {
      return {};
    }
    const parsed = JSON.parse(raw) as Record<string, string>;
    return parsed && typeof parsed === 'object' ? parsed : {};
  } catch {
    return {};
  }
}

function persistActiveSessions(value: Record<string, string>) {
  try {
    const persisted = Object.fromEntries(
      Object.entries(value).filter(([, sessionId]) => typeof sessionId === 'string' && sessionId)
    );
    localStorage.setItem(ACTIVE_SESSION_STORAGE_KEY, JSON.stringify(persisted));
  } catch (error) {
    console.warn('[Web Session] Failed to persist active sessions', error);
  }
}

function loadStoredSessionDrafts() {
  try {
    const raw = localStorage.getItem(SESSION_DRAFT_STORAGE_KEY);
    if (!raw) {
      return {};
    }
    return normalizeStoredDrafts(JSON.parse(raw));
  } catch {
    return {};
  }
}

function persistSessionDrafts(value: Record<string, Record<string, WebSessionDraftState>>) {
  try {
    const persisted = normalizeStoredDrafts(value);
    if (Object.keys(persisted).length === 0) {
      localStorage.removeItem(SESSION_DRAFT_STORAGE_KEY);
      return;
    }
    localStorage.setItem(SESSION_DRAFT_STORAGE_KEY, JSON.stringify(persisted));
  } catch (error) {
    console.warn('[Web Session] Failed to persist session drafts', error);
  }
}

function compareSessions(left: WebSessionSummary, right: WebSessionSummary) {
  if (left.orderIndex !== right.orderIndex) {
    return left.orderIndex - right.orderIndex;
  }
  if (left.updatedAt !== right.updatedAt) {
    return right.updatedAt.localeCompare(left.updatedAt);
  }
  return left.id.localeCompare(right.id);
}

function sortSessions(sessions: WebSessionSummary[]) {
  return [...sessions].sort(compareSessions);
}

function normalizeAssistantStateValue(value: unknown): SessionAssistantState | '' {
  switch (String(value ?? '').trim()) {
    case 'working':
    case 'waiting_approval':
    case 'waiting_input':
    case 'waiting_plan_approval':
      return String(value).trim() as SessionAssistantState;
    default:
      return '';
  }
}

function getSessionAssistantStateValue(
  session?: WebSessionSummary | null
): SessionAssistantState | '' {
  if (!session) {
    return '';
  }
  return normalizeAssistantStateValue(session.assistantState);
}

function getAssistantStateUpdatedAt(session?: WebSessionSummary | null) {
  if (!session) {
    return undefined;
  }
  if (session.assistantStateUpdatedAt) {
    const parsed = Date.parse(session.assistantStateUpdatedAt);
    if (Number.isFinite(parsed)) {
      return parsed;
    }
  }
  return undefined;
}

function isWorkingPhase(phase: WebSessionLiveState['phase']) {
  return phase === 'starting' || phase === 'thinking' || phase === 'retrying' || phase === 'tool';
}

function isProcessRestartPayload(payload?: Record<string, unknown>) {
  return String(payload?.reason ?? '') === PROCESS_RESTART_REASON;
}

function getRecoveryMessage(payload?: Record<string, unknown>) {
  const message = typeof payload?.msg === 'string' ? payload.msg.trim() : '';
  return message || DEFAULT_RECOVERY_MESSAGE;
}

function normalizeHistorySourceItemId(
  record: Record<string, unknown>,
  payload?: Record<string, unknown>
) {
  if (typeof record.siid === 'string' && record.siid.trim()) {
    return record.siid;
  }
  if (typeof record.sourceItemId === 'string' && record.sourceItemId.trim()) {
    return record.sourceItemId;
  }
  if (typeof payload?.iid === 'string' && payload.iid.trim()) {
    return payload.iid;
  }
  return null;
}

function asRecord(value: unknown): Record<string, unknown> | undefined {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    return undefined;
  }
  return value as Record<string, unknown>;
}

function parseJsonRecord(value: unknown): Record<string, unknown> | undefined {
  if (isRecord(value)) {
    return value;
  }
  if (typeof value !== 'string') {
    return undefined;
  }
  const trimmed = value.trim();
  if (!trimmed.startsWith('{') || !trimmed.endsWith('}')) {
    return undefined;
  }
  try {
    return asRecord(JSON.parse(trimmed));
  } catch {
    return undefined;
  }
}

function parseHistoryTimeValue(value: unknown): number | null {
  if (typeof value === 'number' && Number.isFinite(value)) {
    return value;
  }
  if (typeof value === 'string') {
    const parsed = Date.parse(value);
    return Number.isFinite(parsed) ? parsed : null;
  }
  return null;
}

function parseToolCommandGroup(value: unknown) {
  const record = asRecord(value);
  if (!record) {
    return undefined;
  }
  const id = String(record.id ?? '').trim();
  if (!id) {
    return undefined;
  }
  return {
    id,
    count: Math.max(1, Number(record.count ?? 1) || 1),
    firstSeq:
      typeof record.firstSeq === 'number' && Number.isFinite(record.firstSeq)
        ? record.firstSeq
        : undefined,
    lastSeq:
      typeof record.lastSeq === 'number' && Number.isFinite(record.lastSeq)
        ? record.lastSeq
        : undefined,
    latestToolId: String(record.latestToolId ?? '').trim() || undefined,
    compacted: record.compacted === true,
  };
}

function normalizeToolKindValue(value: unknown) {
  const normalized = String(value ?? '').trim();
  if (normalized === 'commandExecution') {
    return 'command_execution';
  }
  if (
    normalized === 'collabAgentToolCall' ||
    normalized === 'collab_agent_tool_call' ||
    normalized === 'subAgentToolCall' ||
    normalized === 'sub_agent_tool_call'
  ) {
    return 'sub_agent_tool_call';
  }
  if (normalized === 'mcpToolCall') {
    return 'mcp_tool_call';
  }
  if (normalized === 'fileChange') {
    return 'file_change';
  }
  if (normalized === 'webSearch') {
    return 'web_search';
  }
  return normalized;
}

function extractToolSummary(payload: Record<string, unknown>) {
  const kind = normalizeToolKindValue(payload.kind ?? asRecord(payload.meta)?.kind);
  const input = asRecord(payload.in);
  const meta = asRecord(payload.meta);
  const subtitle = String(meta?.subtitle ?? '').trim();

  if (kind === 'command_execution') {
    const command = String(input?.command ?? '').trim();
    return command || subtitle;
  }

  if (kind === 'file_change') {
    const path =
      String(input?.path ?? input?.file_path ?? input?.new_path ?? input?.old_path ?? '').trim() ||
      subtitle;
    if (path) {
      return path;
    }
    const changes = Array.isArray(input?.changes) ? input.changes.length : 0;
    return changes > 0 ? `${changes} change${changes > 1 ? 's' : ''}` : '';
  }

  if (kind === 'mcp_tool_call') {
    const toolName = String(input?.tool_name ?? input?.name ?? '').trim();
    const args = asRecord(input?.arguments);
    const target =
      String(
        args?.url ??
          args?.query ??
          args?.path ??
          args?.file ??
          args?.name ??
          args?.id ??
          input?.server ??
          input?.path ??
          ''
      ).trim() || subtitle;
    if (toolName && target && toolName !== target) {
      return `${toolName} · ${target}`;
    }
    return toolName || target;
  }

  if (kind === 'sub_agent_tool_call') {
    const args = asRecord(input?.arguments);
    const summary =
      String(
        input?.task ??
          input?.prompt ??
          input?.description ??
          input?.instruction ??
          input?.instructions ??
          input?.objective ??
          input?.title ??
          args?.task ??
          args?.prompt ??
          args?.description ??
          args?.instruction ??
          args?.objective ??
          args?.title ??
          ''
      ).trim() || subtitle;
    return summary;
  }

  if (kind === 'web_search') {
    const query = String(input?.query ?? '').trim();
    if (query) {
      return query;
    }
    const action = asRecord(input?.action);
    const queries = Array.isArray(action?.queries)
      ? action?.queries
          .map(value => String(value ?? '').trim())
          .filter((value): value is string => Boolean(value))
      : [];
    return queries[0] ?? subtitle;
  }

  return subtitle;
}

function isSubAgentToolKind(kind?: string) {
  return normalizeToolKindValue(kind) === 'sub_agent_tool_call';
}

function normalizeActiveTool(block: WebSessionBlock) {
  if (!block.tool) {
    return undefined;
  }
  const rawKind = block.tool.kind || String(block.tool.meta?.kind ?? '');
  return {
    id: block.tool.id,
    name: block.tool.name,
    kind: normalizeToolKindValue(rawKind),
    summary: extractToolSummary({
      kind: rawKind,
      in: asRecord(block.tool.input) ?? block.tool.input,
      meta: block.tool.meta,
      out: block.tool.output,
    } as Record<string, unknown>),
    count: block.tool.commandGroup?.count,
    groupId: block.tool.commandGroup?.id,
    startedAt: block.tool.startedAt ?? block.timestamp,
  };
}

function normalizeSubAgentTitle(tool: WebSessionToolBlock) {
  const meta = asRecord(tool.meta);
  return (
    String(meta?.title ?? '').trim() ||
    String(meta?.name ?? '').trim() ||
    String(tool.name ?? '').trim() ||
    'Sub Agent'
  );
}

function isGenericSubAgentTitle(value: string) {
  const normalized = String(value ?? '').trim();
  return normalized === '' || normalized === 'Sub Agent';
}

function deriveSubAgentTitle(summary: string, fallback: string) {
  const normalizedFallback = String(fallback ?? '').trim() || 'Sub Agent';
  if (!isGenericSubAgentTitle(normalizedFallback)) {
    return normalizedFallback;
  }
  const firstLine = String(summary ?? '')
    .split('\n')
    .map(line => line.trim())
    .find(Boolean);
  if (!firstLine) {
    return normalizedFallback;
  }
  const prefix = firstLine.split(/[：:]/)[0]?.trim() ?? '';
  if (prefix && prefix.length <= 24) {
    return prefix;
  }
  const compact = firstLine.slice(0, 24).trim();
  return compact || normalizedFallback;
}

function normalizeSubAgentReceiverIds(
  input?: Record<string, unknown>,
  output?: Record<string, unknown>
) {
  const fromValue = (value: unknown) =>
    Array.isArray(value)
      ? value.map(item => String(item ?? '').trim()).filter((item): item is string => Boolean(item))
      : [];

  const direct = fromValue(input?.receiverThreadIds);
  if (direct.length > 0) {
    return direct;
  }
  const outputIds = fromValue(output?.receiverThreadIds);
  if (outputIds.length > 0) {
    return outputIds;
  }

  const states = asRecord(input?.agentsStates) ?? asRecord(output?.agentsStates);
  if (!states) {
    return [];
  }
  return Object.keys(states)
    .map(key => key.trim())
    .filter(Boolean);
}

function normalizeSubAgentStateMap(
  input?: Record<string, unknown>,
  output?: Record<string, unknown>
) {
  const source = asRecord(input?.agentsStates) ?? asRecord(output?.agentsStates);
  const stateMap = new Map<
    string,
    {
      status: string;
      message: string;
    }
  >();
  if (!source) {
    return stateMap;
  }
  Object.entries(source).forEach(([id, value]) => {
    const normalizedID = String(id ?? '').trim();
    if (!normalizedID) {
      return;
    }
    const record = asRecord(value);
    stateMap.set(normalizedID, {
      status: String(record?.status ?? '').trim(),
      message: String(record?.message ?? '').trim(),
    });
  });
  return stateMap;
}

function resolveSubAgentSummary(
  input?: Record<string, unknown>,
  meta?: Record<string, unknown>,
  output?: Record<string, unknown>
) {
  return extractToolSummary({
    kind: 'sub_agent_tool_call',
    in: {
      ...(output ?? {}),
      ...(input ?? {}),
    },
    meta,
    out: typeof output === 'string' ? output : undefined,
  } as Record<string, unknown>);
}

function normalizeSubAgentOperation(
  input?: Record<string, unknown>,
  output?: Record<string, unknown>
) {
  return String(input?.tool ?? output?.tool ?? '').trim();
}

function rememberKnownSubAgents(
  block: WebSessionBlock,
  registry: Map<string, WebSessionLiveSubAgent>
) {
  if (!block.tool) {
    return;
  }
  const rawKind = block.tool.kind || String(block.tool.meta?.kind ?? '');
  if (!isSubAgentToolKind(rawKind)) {
    return;
  }
  const input = asRecord(block.tool.input);
  const output = parseJsonRecord(block.tool.output);
  const ids = normalizeSubAgentReceiverIds(input, output);
  if (ids.length === 0) {
    return;
  }
  const meta = asRecord(block.tool.meta);
  const stateMap = normalizeSubAgentStateMap(input, output);
  const summary = resolveSubAgentSummary(input, meta, output);
  const title = deriveSubAgentTitle(summary, normalizeSubAgentTitle(block.tool));
  const startedAt = block.tool.startedAt ?? block.timestamp;

  ids.forEach(id => {
    const existing = registry.get(id);
    const state = stateMap.get(id);
    const nextSummary = state?.message || summary || existing?.summary || '';
    registry.set(id, {
      id,
      title: existing?.title || title,
      summary: nextSummary,
      startedAt: existing?.startedAt ?? startedAt,
    });
  });
}

function normalizeLiveSubAgents(
  block: WebSessionBlock,
  registry: Map<string, WebSessionLiveSubAgent>
): WebSessionLiveSubAgent[] {
  const rawKind = block.tool?.kind || String(block.tool?.meta?.kind ?? '');
  if (!block.tool || !isSubAgentToolKind(rawKind)) {
    return [];
  }
  const input = asRecord(block.tool.input);
  const output = parseJsonRecord(block.tool.output);
  const meta = asRecord(block.tool.meta);
  const ids = normalizeSubAgentReceiverIds(input, output);
  const stateMap = normalizeSubAgentStateMap(input, output);
  const summary = resolveSubAgentSummary(input, meta, output);
  const title = deriveSubAgentTitle(summary, normalizeSubAgentTitle(block.tool));
  const startedAt = block.tool.startedAt ?? block.timestamp;

  if (ids.length === 0) {
    return [
      {
        id: block.tool.id,
        title,
        summary,
        startedAt,
      },
    ];
  }

  return ids.map(id => {
    const known = registry.get(id);
    const state = stateMap.get(id);
    return {
      id,
      title: known?.title || title,
      summary: known?.summary || state?.message || summary,
      startedAt: known?.startedAt ?? startedAt,
    };
  });
}

function activeSubAgentIDsForBlock(
  block: WebSessionBlock,
  registry: Map<string, WebSessionLiveSubAgent>
) {
  return normalizeLiveSubAgents(block, registry)
    .map(agent => String(agent.id ?? '').trim())
    .filter(Boolean);
}

function applyAssistantNamedSubAgents(
  text: string,
  registry: Map<string, WebSessionLiveSubAgent>,
  active?: Map<string, WebSessionLiveSubAgent>
) {
  const normalizedText = String(text ?? '').trim();
  if (!normalizedText.includes('已启动')) {
    return;
  }
  const match = normalizedText.match(/已启动\s+(.+?)[。.!?]/);
  if (!match) {
    return;
  }
  const names = match[1]
    .split(/[、，,]/)
    .map(name => name.trim())
    .filter(Boolean);
  if (names.length < 2) {
    return;
  }
  const candidates = [...registry.values()]
    .sort((left, right) => {
      const leftTime = left.startedAt ?? 0;
      const rightTime = right.startedAt ?? 0;
      if (leftTime !== rightTime) {
        return leftTime - rightTime;
      }
      return left.id.localeCompare(right.id);
    })
    .filter(
      item =>
        isGenericSubAgentTitle(item.title) ||
        item.title.startsWith('测试任务') ||
        item.title.startsWith('Task ') ||
        item.title === item.summary
    );
  if (candidates.length < names.length) {
    return;
  }
  names.forEach((name, index) => {
    const candidate = candidates[index];
    if (!candidate) {
      return;
    }
    const next = {
      ...candidate,
      title: name,
    };
    registry.set(candidate.id, next);
    if (active?.has(candidate.id)) {
      active.set(candidate.id, next);
    }
  });
}

function syncActiveSubAgentLifecycle(
  block: WebSessionBlock,
  registry: Map<string, WebSessionLiveSubAgent>,
  active: Map<string, WebSessionLiveSubAgent>
) {
  if (!block.tool) {
    return;
  }
  const rawKind = block.tool.kind || String(block.tool.meta?.kind ?? '');
  if (!isSubAgentToolKind(rawKind)) {
    return;
  }
  const input = asRecord(block.tool.input);
  const output = parseJsonRecord(block.tool.output);
  const operation = normalizeSubAgentOperation(input, output);
  const agents = normalizeLiveSubAgents(block, registry);

  if (operation === 'spawnAgent') {
    agents.forEach(agent => {
      active.set(agent.id, registry.get(agent.id) ?? agent);
    });
    return;
  }

  if (block.tool.status === 'running') {
    agents.forEach(agent => {
      active.set(agent.id, registry.get(agent.id) ?? agent);
    });
    return;
  }

  const ids = activeSubAgentIDsForBlock(block, registry);
  ids.forEach(id => {
    active.delete(id);
  });
}

function uniqueActiveSubAgents(items: WebSessionLiveSubAgent[]) {
  const byId = new Map<string, WebSessionLiveSubAgent>();
  for (const item of items) {
    const id = String(item.id ?? '').trim();
    if (!id) {
      continue;
    }
    byId.set(id, item);
  }
  return [...byId.values()].sort((left, right) => {
    const leftTime = left.startedAt ?? 0;
    const rightTime = right.startedAt ?? 0;
    if (leftTime !== rightTime) {
      return leftTime - rightTime;
    }
    return left.id.localeCompare(right.id);
  });
}

function withActiveSubAgents<T extends WebSessionLiveState>(
  state: T,
  activeSubAgents: WebSessionLiveSubAgent[]
): T {
  if (!state.running) {
    return state;
  }
  const uniqueItems = uniqueActiveSubAgents(activeSubAgents);
  if (uniqueItems.length === 0) {
    return state;
  }
  return {
    ...state,
    activeSubAgents: uniqueItems,
    activeSubAgentCount: uniqueItems.length,
  };
}

function getTransportRetryPayload(payload?: Record<string, unknown>) {
  if (!payload || String(payload.code ?? '').trim() !== 'transport_retrying') {
    return null;
  }
  const attempt =
    typeof payload.attempt === 'number' && Number.isFinite(payload.attempt) && payload.attempt > 0
      ? Math.trunc(payload.attempt)
      : undefined;
  const maxAttempts =
    typeof payload.maxAttempts === 'number' &&
    Number.isFinite(payload.maxAttempts) &&
    payload.maxAttempts > 0
      ? Math.trunc(payload.maxAttempts)
      : undefined;
  return {
    code: 'transport_retrying',
    message: String(payload.txt ?? '').trim(),
    attempt,
    maxAttempts,
  };
}

function isRetryClearingProgressBlock(
  block: WebSessionBlock,
  retryPayload: ReturnType<typeof getTransportRetryPayload>
) {
  if (retryPayload) {
    return false;
  }
  if (block.kind === 'assistant' || block.kind === 'user' || block.kind === 'tool') {
    return true;
  }
  if (block.detail?.type) {
    return true;
  }
  switch (block.itemType) {
    case 'note':
    case 'approval_req':
    case 'approval_res':
    case 'user_input_request':
    case 'user_input_response':
    case 'run_done':
    case 'run_abort':
    case 'run_fail':
      return true;
    default:
      return false;
  }
}

function parseUserInputQuestions(value: unknown): WebSessionUserInputQuestion[] {
  if (!Array.isArray(value)) {
    return [];
  }
  return value
    .map(item => {
      const record = asRecord(item);
      if (!record) {
        return null;
      }
      return {
        id: String(record.id ?? record.question ?? record.header ?? ''),
        header: String(record.header ?? ''),
        question: String(record.question ?? ''),
        multiSelect: record.multiSelect === true,
        isOther: record.isOther === true,
        isSecret: record.isSecret === true,
        options: Array.isArray(record.options)
          ? record.options
              .map(option => {
                const optionRecord = asRecord(option);
                if (!optionRecord) {
                  return null;
                }
                return {
                  label: String(optionRecord.label ?? ''),
                  description: String(optionRecord.description ?? ''),
                };
              })
              .filter((option): option is WebSessionUserInputOption => Boolean(option))
          : [],
      };
    })
    .filter((question): question is WebSessionUserInputQuestion => Boolean(question));
}

function summarizeUserInputPrompt(payload: Record<string, unknown>) {
  const explicit = String(payload.txt ?? '').trim();
  if (explicit) {
    return explicit;
  }
  const questions = parseUserInputQuestions(payload.qs);
  const lines = questions
    .map(question => question.question.trim() || question.header.trim())
    .filter(Boolean);
  return lines.length > 0 ? lines.join('\n') : 'Additional input is required.';
}

function summarizeUserInputAnswer(payload: Record<string, unknown>) {
  const answers = asRecord(payload.ans);
  if (!answers) {
    return 'Submitted requested input';
  }
  const parts = Object.values(answers)
    .flatMap(value => (Array.isArray(value) ? value : []))
    .map(value => String(value).trim())
    .filter(Boolean);
  if (parts.length === 0) {
    return 'Submitted requested input';
  }
  return parts.join(', ');
}

function buildUserInputAnswerEntries(
  payload: Record<string, unknown>,
  questions: WebSessionUserInputQuestion[]
): WebSessionHistoryAnswerEntry[] {
  const answers = asRecord(payload.ans);
  if (!answers) {
    return [];
  }

  const questionMap = new Map(questions.map(question => [question.id, question]));
  const result: WebSessionHistoryAnswerEntry[] = [];
  Object.entries(answers).forEach(([questionId, value]) => {
    const question = questionMap.get(questionId);
    const values = (Array.isArray(value) ? value : [])
      .map(item => String(item).trim())
      .filter(Boolean);
    if (values.length === 0) {
      return;
    }
    result.push({
      id: questionId,
      label:
        question?.header?.trim() || question?.question?.trim() || questionId || 'Submitted answer',
      values,
      masked: question?.isSecret === true,
    });
  });
  return result;
}

function normalizeProjectScope(projectIds: string[]) {
  const ids = Array.from(
    new Set(projectIds.map(projectId => String(projectId || '').trim()).filter(Boolean))
  ).sort((left, right) => left.localeCompare(right));
  return {
    ids,
    key: ids.join('::'),
  };
}

function defaultArchivedListMeta(scopeKey = ''): ArchivedListMeta {
  return {
    scopeKey,
    total: 0,
    offset: 0,
    hasMore: false,
    loading: false,
  };
}

export const useWebSessionStore = defineStore('web-session', () => {
  const sessionsByProject = ref<Record<string, WebSessionSummary[]>>({});
  const archivedSessionsById = ref<Record<string, WebSessionSummary>>({});
  const archivedScopeStates = ref<Record<string, ArchivedListScopeState>>({});
  const eventsBySession = ref<Record<string, WebSessionBlock[]>>({});
  const historyBySession = ref<Record<string, HistoryMeta>>({});
  const draftStateByProject =
    ref<Record<string, Record<string, WebSessionDraftState>>>(loadStoredSessionDrafts());
  const userInputDraftStateByKey = ref<Record<string, WebSessionPendingUserInputDraftState>>({});
  const draftAttachmentUploadsByProject = ref<
    Record<string, Record<string, WebSessionDraftAttachmentUploadState>>
  >({});
  const pendingInputsBySession = ref<Record<string, WebSessionPendingInput[]>>({});
  const scheduledInputsBySession = ref<Record<string, WebSessionScheduledInput[]>>({});
  const activeSessionIdByProject = ref<Record<string, string>>(loadStoredActiveSessions());
  const loadedProjects = ref<Record<string, boolean>>({});
  const cachedCounts = reactive(new Map<string, number>());
  const emitter = new EventEmitter();

  const connectionState = ref<'idle' | 'connecting' | 'open' | 'closed'>('idle');
  const eventLastSeenAt = ref(0);
  const eventLastDisconnectReason = ref<string | null>(null);
  const eventRecoveryVersion = ref(0);
  const lastError = ref<string | null>(null);

  let eventSocket: WebSocket | null = null;
  let eventConnectPromise: Promise<void> | null = null;
  let eventReconnectTimer: number | null = null;
  let eventWatchdogTimer: number | null = null;
  let eventReconnectAttempt = 0;
  let eventHasConnectedOnce = false;
  let eventFocusedSessionId = '';
  let commandSocket: WebSocket | null = null;
  let commandConnectPromise: Promise<void> | null = null;
  let commandWatchdogTimer: number | null = null;
  let commandLastSeenAt = 0;
  const pending = new Map<
    string,
    {
      resolve: (value: WireFrame) => void;
      reject: (reason?: unknown) => void;
    }
  >();
  const draftAttachmentUploadQueues = new Map<string, Promise<unknown>>();
  const pendingAutoRetryOverrides = reactive(new Map<string, PendingAutoRetryOverride>());
  const pendingActiveCallTimeoutOverrides = reactive(
    new Map<string, PendingActiveCallTimeoutOverride>()
  );
  const appliedSnapshotVersionBySession = new Map<string, WebSessionSnapshotVersion>();
  const completedTransitionVersionBySession = new Map<string, number>();
  let draftAttachmentUploadSeed = 0;

  const allSessionIds = computed(() => {
    const ids = new Set<string>();
    Object.values(sessionsByProject.value).forEach(items => {
      items.forEach(item => ids.add(item.id));
    });
    Object.values(archivedScopeStates.value).forEach(scopeState => {
      scopeState.sessionIds.forEach(sessionId => ids.add(sessionId));
    });
    return ids;
  });

  function getSessions(projectId: string) {
    return sessionsByProject.value[projectId] ?? [];
  }

  function syncSessionCount(projectId: string) {
    if (!projectId) {
      return;
    }
    cachedCounts.set(projectId, getSessions(projectId).length);
  }

  function getSessionCount(projectId: string) {
    return cachedCounts.get(projectId) ?? 0;
  }

  function getArchivedScopeState(scope: { ids: string[]; key: string }) {
    if (!scope.key) {
      return null;
    }
    return archivedScopeStates.value[scope.key] ?? null;
  }

  function getArchivedSessions(projectIds: string[]) {
    const scope = normalizeProjectScope(projectIds);
    const scopeState = getArchivedScopeState(scope);
    if (!scopeState) {
      return [];
    }
    return scopeState.sessionIds
      .map(sessionId => archivedSessionsById.value[sessionId])
      .filter((session): session is WebSessionSummary => Boolean(session));
  }

  function getArchivedMeta(projectIds: string[]): ArchivedListMeta {
    const scope = normalizeProjectScope(projectIds);
    const scopeState = getArchivedScopeState(scope);
    if (!scopeState) {
      return defaultArchivedListMeta();
    }
    return scopeState.meta;
  }

  function hasArchivedScope(projectIds: string[]) {
    const scope = normalizeProjectScope(projectIds);
    return Boolean(scope.key && getArchivedScopeState(scope));
  }

  function getActiveSessionId(projectId: string) {
    return activeSessionIdByProject.value[projectId] ?? '';
  }

  function hasStoredActiveSession(projectId: string) {
    return Object.prototype.hasOwnProperty.call(activeSessionIdByProject.value, projectId);
  }

  function getActiveSession(projectId: string) {
    const activeId = getActiveSessionId(projectId);
    return getSessions(projectId).find(item => item.id === activeId) ?? null;
  }

  function findSessionById(sessionId: string) {
    for (const sessions of Object.values(sessionsByProject.value)) {
      const matched = sessions.find(item => item.id === sessionId);
      if (matched) {
        return matched;
      }
    }
    const archived = archivedSessionsById.value[sessionId];
    if (archived) {
      return archived;
    }
    return null;
  }

  function getLatestEventSeq(sessionId: string) {
    const events = eventsBySession.value[sessionId] ?? [];
    return events.length > 0 ? (events[events.length - 1]?.orderIndex ?? 0) : 0;
  }

  function getDraft(projectId: string, sessionId: string): WebSessionDraftState {
    const normalizedProjectId = String(projectId || '').trim();
    const normalizedSessionId = String(sessionId || '').trim();
    if (!normalizedProjectId || !normalizedSessionId) {
      return {
        text: '',
        attachments: [],
        updatedAt: 0,
      };
    }
    return (
      draftStateByProject.value[normalizedProjectId]?.[normalizedSessionId] ?? {
        text: '',
        attachments: [],
        updatedAt: 0,
      }
    );
  }

  function clonePendingUserInputDraftState(
    state: WebSessionPendingUserInputDraftState
  ): WebSessionPendingUserInputDraftState {
    const selections: Record<string, string[]> = {};
    Object.entries(state.selections ?? {}).forEach(([questionId, values]) => {
      selections[questionId] = Array.isArray(values)
        ? values.map(value => String(value ?? ''))
        : [];
    });

    const drafts: Record<string, string> = {};
    Object.entries(state.drafts ?? {}).forEach(([questionId, value]) => {
      drafts[questionId] = String(value ?? '');
    });

    return {
      selections,
      drafts,
      updatedAt:
        typeof state.updatedAt === 'number' && Number.isFinite(state.updatedAt)
          ? state.updatedAt
          : Date.now(),
    };
  }

  function getPendingUserInputDraft(key: string): WebSessionPendingUserInputDraftState | null {
    const normalizedKey = String(key || '').trim();
    if (!normalizedKey) {
      return null;
    }
    const state = userInputDraftStateByKey.value[normalizedKey];
    return state ? clonePendingUserInputDraftState(state) : null;
  }

  function setPendingUserInputDraft(
    key: string,
    state: Pick<WebSessionPendingUserInputDraftState, 'selections' | 'drafts'>
  ) {
    const normalizedKey = String(key || '').trim();
    if (!normalizedKey) {
      return;
    }
    userInputDraftStateByKey.value = {
      ...userInputDraftStateByKey.value,
      [normalizedKey]: clonePendingUserInputDraftState({
        selections: state.selections,
        drafts: state.drafts,
        updatedAt: Date.now(),
      }),
    };
  }

  function clearPendingUserInputDraft(key: string) {
    const normalizedKey = String(key || '').trim();
    if (!normalizedKey || !userInputDraftStateByKey.value[normalizedKey]) {
      return;
    }
    const nextState = { ...userInputDraftStateByKey.value };
    delete nextState[normalizedKey];
    userInputDraftStateByKey.value = nextState;
  }

  function getDraftAttachments(projectId: string, sessionId: string) {
    return getDraft(projectId, sessionId).attachments;
  }

  function getDraftAttachmentUpload(projectId: string, sessionId: string) {
    const normalizedProjectId = String(projectId || '').trim();
    const normalizedSessionId = String(sessionId || '').trim();
    if (!normalizedProjectId || !normalizedSessionId) {
      return null;
    }
    return (
      draftAttachmentUploadsByProject.value[normalizedProjectId]?.[normalizedSessionId] ?? null
    );
  }

  function getPendingInputs(sessionId: string) {
    return pendingInputsBySession.value[sessionId] ?? [];
  }

  function getScheduledInputs(sessionId: string) {
    return scheduledInputsBySession.value[sessionId] ?? [];
  }

  function getHistoryMeta(sessionId: string): HistoryMeta {
    return (
      historyBySession.value[sessionId] ?? {
        hasMore: false,
        beforeCursor: '',
        total: 0,
        loading: false,
      }
    );
  }

  function setHistoryLoading(sessionId: string, loading: boolean) {
    historyBySession.value = {
      ...historyBySession.value,
      [sessionId]: {
        ...getHistoryMeta(sessionId),
        loading,
      },
    };
  }

  function currentSnapshotVersionInput(sessionId: string): WebSessionSnapshotVersionInput | null {
    const session = findSessionById(sessionId);
    if (!session) {
      return null;
    }
    return {
      session,
      historyTotal: getHistoryMeta(sessionId).total,
    };
  }

  function rememberAppliedSnapshotVersion(
    sessionId: string,
    snapshot: WebSessionSnapshotVersionInput
  ) {
    const nextVersion = buildWebSessionSnapshotVersion(snapshot);
    const currentVersion = appliedSnapshotVersionBySession.get(sessionId) ?? null;
    if (!currentVersion || compareWebSessionSnapshotVersion(nextVersion, currentVersion) >= 0) {
      appliedSnapshotVersionBySession.set(sessionId, nextVersion);
    }
  }

  function setPendingAutoRetryOverride(
    sessionId: string,
    config: {
      enabled: boolean;
      scope: WebSessionSummary['autoRetryScope'];
      preset: WebSessionSummary['autoRetryPreset'];
    },
    appliedAt = Date.now()
  ) {
    pendingAutoRetryOverrides.set(sessionId, {
      enabled: config.enabled === true,
      scope: config.scope,
      preset: config.preset,
      appliedAt,
      expiresAt: appliedAt + WEB_SESSION_AUTO_RETRY_OPTIMISTIC_TTL_MS,
    });
  }

  function clearPendingAutoRetryOverride(sessionId: string) {
    pendingAutoRetryOverrides.delete(sessionId);
  }

  function applyPendingAutoRetryOverride(summary: WebSessionSummary): WebSessionSummary {
    const pendingOverride = pendingAutoRetryOverrides.get(summary.id);
    if (!pendingOverride) {
      return summary;
    }
    if (Date.now() > pendingOverride.expiresAt) {
      pendingAutoRetryOverrides.delete(summary.id);
      return summary;
    }
    const matchesPendingConfig =
      summary.autoRetryEnabled === pendingOverride.enabled &&
      summary.autoRetryScope === pendingOverride.scope &&
      summary.autoRetryPreset === pendingOverride.preset;
    if (matchesPendingConfig) {
      pendingAutoRetryOverrides.delete(summary.id);
      return summary;
    }
    const updatedAt = Date.parse(summary.updatedAt || '');
    const mergedUpdatedAt = Number.isFinite(updatedAt)
      ? Math.max(updatedAt, pendingOverride.appliedAt)
      : pendingOverride.appliedAt;
    return {
      ...summary,
      autoRetryEnabled: pendingOverride.enabled,
      autoRetryScope: pendingOverride.scope,
      autoRetryPreset: pendingOverride.preset,
      updatedAt: new Date(mergedUpdatedAt).toISOString(),
    };
  }

  function setPendingActiveCallTimeoutOverride(
    sessionId: string,
    enabled: boolean,
    appliedAt = Date.now()
  ) {
    pendingActiveCallTimeoutOverrides.set(sessionId, {
      enabled: enabled === true,
      appliedAt,
      expiresAt: appliedAt + WEB_SESSION_AUTO_RETRY_OPTIMISTIC_TTL_MS,
    });
  }

  function clearPendingActiveCallTimeoutOverride(sessionId: string) {
    pendingActiveCallTimeoutOverrides.delete(sessionId);
  }

  function applyPendingActiveCallTimeoutOverride(summary: WebSessionSummary): WebSessionSummary {
    const pendingOverride = pendingActiveCallTimeoutOverrides.get(summary.id);
    if (!pendingOverride) {
      return summary;
    }
    if (Date.now() > pendingOverride.expiresAt) {
      pendingActiveCallTimeoutOverrides.delete(summary.id);
      return summary;
    }
    if (summary.activeCallTimeoutEnabled === pendingOverride.enabled) {
      pendingActiveCallTimeoutOverrides.delete(summary.id);
      return summary;
    }
    const updatedAt = Date.parse(summary.updatedAt || '');
    const mergedUpdatedAt = Number.isFinite(updatedAt)
      ? Math.max(updatedAt, pendingOverride.appliedAt)
      : pendingOverride.appliedAt;
    return {
      ...summary,
      activeCallTimeoutEnabled: pendingOverride.enabled,
      updatedAt: new Date(mergedUpdatedAt).toISOString(),
    };
  }

  function applySessionSnapshot(
    sessionId: string,
    summary: WebSessionSummary,
    items: WebSessionBlock[],
    pendingInputs: WebSessionPendingInput[],
    scheduledInputs: WebSessionScheduledInput[],
    history: {
      hasMore: boolean;
      beforeCursor?: string;
      total: number;
    },
    options?: {
      preserveArchivedPosition?: boolean;
    }
  ) {
    upsertSession(summary, options);
    resetSessionEvents(sessionId, items);
    setPendingInputs(sessionId, pendingInputs);
    setScheduledInputs(sessionId, scheduledInputs);
    historyBySession.value = {
      ...historyBySession.value,
      [sessionId]: {
        hasMore: Boolean(history.hasMore),
        beforeCursor: String(history.beforeCursor ?? ''),
        total: Number(history.total ?? 0),
        loading: false,
      },
    };
    rememberAppliedSnapshotVersion(sessionId, {
      session: summary,
      historyTotal: history.total,
    });
    if (summary.status === 'done') {
      completedTransitionVersionBySession.set(
        sessionId,
        Math.max(
          Date.parse(summary.updatedAt || '') || 0,
          Date.parse(summary.lastMessageAt || '') || 0
        )
      );
    }
  }

  function rememberActiveSession(projectId: string, sessionId: string) {
    activeSessionIdByProject.value = {
      ...activeSessionIdByProject.value,
      [projectId]: sessionId,
    };
    persistActiveSessions(activeSessionIdByProject.value);
  }

  function setActiveSession(projectId: string, sessionId: string) {
    if (!projectId) {
      return;
    }
    if (!sessionId) {
      activeSessionIdByProject.value = {
        ...activeSessionIdByProject.value,
        [projectId]: '',
      };
      return;
    }
    rememberActiveSession(projectId, sessionId);
  }

  function commitProjectDrafts(projectId: string, drafts: Record<string, WebSessionDraftState>) {
    const normalizedProjectId = String(projectId || '').trim();
    if (!normalizedProjectId) {
      return;
    }
    const nextDraftState = { ...draftStateByProject.value };
    if (Object.keys(drafts).length > 0) {
      nextDraftState[normalizedProjectId] = drafts;
    } else {
      delete nextDraftState[normalizedProjectId];
    }
    draftStateByProject.value = nextDraftState;
    persistSessionDrafts(nextDraftState);
  }

  function commitDraftAttachmentUploads(
    projectId: string,
    uploads: Record<string, WebSessionDraftAttachmentUploadState>
  ) {
    const normalizedProjectId = String(projectId || '').trim();
    if (!normalizedProjectId) {
      return;
    }
    const nextUploads = { ...draftAttachmentUploadsByProject.value };
    if (Object.keys(uploads).length > 0) {
      nextUploads[normalizedProjectId] = uploads;
    } else {
      delete nextUploads[normalizedProjectId];
    }
    draftAttachmentUploadsByProject.value = nextUploads;
  }

  function setDraftAttachmentUploadState(
    projectId: string,
    sessionId: string,
    upload: WebSessionDraftAttachmentUploadState | null
  ) {
    const normalizedProjectId = String(projectId || '').trim();
    const normalizedSessionId = String(sessionId || '').trim();
    if (!normalizedProjectId || !normalizedSessionId) {
      return;
    }
    const projectUploads = draftAttachmentUploadsByProject.value[normalizedProjectId] ?? {};
    const nextProjectUploads = { ...projectUploads };
    if (upload) {
      nextProjectUploads[normalizedSessionId] = upload;
    } else {
      delete nextProjectUploads[normalizedSessionId];
    }
    commitDraftAttachmentUploads(normalizedProjectId, nextProjectUploads);
  }

  function createDraftAttachmentUploadID() {
    draftAttachmentUploadSeed += 1;
    return `upload-${Date.now()}-${draftAttachmentUploadSeed}`;
  }

  function draftAttachmentUploadQueueKey(projectId: string, sessionId: string) {
    return `${projectId}:${sessionId}`;
  }

  function normalizeDraftAttachmentFileName(file: File, index: number) {
    return buildUploadImageFileName(file.name, index, file.type);
  }

  function normalizeDraftAttachmentFile(file: File, index: number) {
    const fileName = normalizeDraftAttachmentFileName(file, index);
    if (fileName === file.name) {
      return {
        file,
        fileName,
      };
    }
    return {
      file: new File([file], fileName, {
        type: file.type,
        lastModified: file.lastModified,
      }),
      fileName,
    };
  }

  async function uploadAttachments(
    projectId: string,
    sessionId: string,
    files: File[]
  ): Promise<WebSessionDraftAttachmentUploadBatchResult> {
    const normalizedProjectId = String(projectId || '').trim();
    const normalizedSessionId = String(sessionId || '').trim();
    const imageFiles = Array.from(files).filter(file => file.type.startsWith('image/'));
    if (!normalizedProjectId || !normalizedSessionId || imageFiles.length === 0) {
      return {
        attachments: [],
        errors: [],
      };
    }

    const queueKey = draftAttachmentUploadQueueKey(normalizedProjectId, normalizedSessionId);
    const previousTask = draftAttachmentUploadQueues.get(queueKey) ?? Promise.resolve();
    const task = previousTask
      .catch(() => undefined)
      .then(async () => {
        const attachments: WebSessionAttachment[] = [];
        const errors: WebSessionDraftAttachmentUploadError[] = [];
        const batchID = createDraftAttachmentUploadID();
        const existingAttachmentCount = getDraft(normalizedProjectId, normalizedSessionId)
          .attachments.length;

        for (const [index, file] of imageFiles.entries()) {
          const nextAttachmentIndex = existingAttachmentCount + attachments.length + 1;
          const normalizedFile = normalizeDraftAttachmentFile(file, nextAttachmentIndex);
          const fileName = normalizedFile.fileName;
          const applyProgress = (progress: WebSessionAttachmentUploadProgress) => {
            setDraftAttachmentUploadState(normalizedProjectId, normalizedSessionId, {
              id: batchID,
              fileName,
              currentFileIndex: index + 1,
              totalFiles: imageFiles.length,
              loaded: progress.loaded,
              total: progress.total,
              percent: progress.percent ?? 0,
            });
          };

          applyProgress({
            loaded: 0,
            total: file.size > 0 ? file.size : undefined,
            percent: 0,
          });

          try {
            const attachment = await webSessionApi.uploadAttachment(
              normalizedProjectId,
              normalizedFile.file,
              {
                onProgress: applyProgress,
              }
            );
            attachments.push(attachment);
            updateDraft(normalizedProjectId, normalizedSessionId, draft => ({
              ...draft,
              attachments: [...draft.attachments, attachment],
              updatedAt: Date.now(),
            }));
          } catch (error) {
            errors.push({
              fileName,
              message: error instanceof Error ? error.message : 'failed to upload attachment',
            });
          }
        }

        setDraftAttachmentUploadState(normalizedProjectId, normalizedSessionId, null);
        return {
          attachments,
          errors,
        };
      });

    draftAttachmentUploadQueues.set(queueKey, task);

    try {
      return await task;
    } finally {
      if (draftAttachmentUploadQueues.get(queueKey) === task) {
        draftAttachmentUploadQueues.delete(queueKey);
      }
    }
  }

  function updateDraft(
    projectId: string,
    sessionId: string,
    updater: (draft: WebSessionDraftState) => WebSessionDraftState | null
  ) {
    const normalizedProjectId = String(projectId || '').trim();
    const normalizedSessionId = String(sessionId || '').trim();
    if (!normalizedProjectId || !normalizedSessionId) {
      return;
    }
    const projectDrafts = draftStateByProject.value[normalizedProjectId] ?? {};
    const currentDraft = projectDrafts[normalizedSessionId] ?? {
      text: '',
      attachments: [],
      updatedAt: 0,
    };
    const nextDraft = updater({
      text: currentDraft.text,
      attachments: [...currentDraft.attachments],
      updatedAt: currentDraft.updatedAt,
    });
    const nextProjectDrafts = { ...projectDrafts };
    if (!nextDraft || (!nextDraft.text.trim() && nextDraft.attachments.length === 0)) {
      delete nextProjectDrafts[normalizedSessionId];
    } else {
      nextProjectDrafts[normalizedSessionId] = {
        text: nextDraft.text,
        attachments: [...nextDraft.attachments],
        updatedAt: nextDraft.updatedAt || Date.now(),
      };
    }
    commitProjectDrafts(normalizedProjectId, nextProjectDrafts);
  }

  function setDraftText(projectId: string, sessionId: string, text: string) {
    updateDraft(projectId, sessionId, draft => ({
      ...draft,
      text,
      updatedAt: Date.now(),
    }));
  }

  function clearDraft(projectId: string, sessionId: string) {
    updateDraft(projectId, sessionId, () => null);
  }

  function moveDraft(projectId: string, fromSessionId: string, toSessionId: string) {
    const normalizedProjectId = String(projectId || '').trim();
    const normalizedFromSessionId = String(fromSessionId || '').trim();
    const normalizedToSessionId = String(toSessionId || '').trim();
    if (!normalizedProjectId || !normalizedFromSessionId || !normalizedToSessionId) {
      return;
    }
    if (normalizedFromSessionId === normalizedToSessionId) {
      return;
    }
    const projectDrafts = draftStateByProject.value[normalizedProjectId] ?? {};
    const sourceDraft = projectDrafts[normalizedFromSessionId];
    if (!sourceDraft) {
      return;
    }
    const targetDraft = projectDrafts[normalizedToSessionId] ?? {
      text: '',
      attachments: [],
      updatedAt: 0,
    };
    const mergedAttachments = [
      ...sourceDraft.attachments,
      ...targetDraft.attachments.filter(
        attachment =>
          !sourceDraft.attachments.some(sourceAttachment => sourceAttachment.id === attachment.id)
      ),
    ];
    const nextProjectDrafts = { ...projectDrafts };
    delete nextProjectDrafts[normalizedFromSessionId];
    nextProjectDrafts[normalizedToSessionId] = {
      text: sourceDraft.text.trim() ? sourceDraft.text : targetDraft.text,
      attachments: mergedAttachments,
      updatedAt: Date.now(),
    };
    commitProjectDrafts(normalizedProjectId, nextProjectDrafts);
  }

  function normalizeSession(session: WireSession): WebSessionSummary {
    const archivedAt =
      typeof session.aa === 'number' && Number.isFinite(session.aa)
        ? new Date(session.aa).toISOString()
        : null;
    const activityAt =
      typeof session.act === 'number' && Number.isFinite(session.act)
        ? new Date(session.act).toISOString()
        : new Date(session.lu).toISOString();
    const statusUpdatedAt =
      typeof session.sta === 'number' && Number.isFinite(session.sta)
        ? new Date(session.sta).toISOString()
        : null;
    const createdAt =
      typeof session.ca === 'number' && Number.isFinite(session.ca)
        ? new Date(session.ca).toISOString()
        : new Date(session.lu).toISOString();
    return {
      id: session.id,
      projectId: session.pid,
      worktreeId: session.wid ?? null,
      orderIndex: Number(session.oi ?? 0),
      agent: session.ag,
      claudeRuntime: session.cr === 'ccr' ? 'ccr' : 'claude',
      title: session.ttl,
      model: session.md,
      reasoningEffort: session.re ?? 'default',
      workflowMode: session.wm ?? 'default',
      permissionLevel: session.pl ?? 'elevated',
      activeCallTimeoutEnabled: session.acte === true,
      autoRetryEnabled: session.ae === true,
      autoRetryScope:
        session.ars === 'network_and_rate_limit' || session.ars === 'all_failures'
          ? session.ars
          : 'network_only',
      autoRetryPreset:
        session.arp === 'aggressive_stop' || session.arp === 'sustain_60s'
          ? session.arp
          : 'gentle_stop',
      cwd: session.cwd,
      nativeSessionId: session.nsid ?? null,
      status: session.st,
      assistantState: normalizeAssistantStateValue(session.ast) || null,
      hasUnread: session.unr,
      archivedAt,
      activityAt,
      statusUpdatedAt,
      lastMessageAt: session.lma ? new Date(session.lma).toISOString() : null,
      assistantStateUpdatedAt: session.asu ? new Date(session.asu).toISOString() : null,
      sourceKind: session.sk ?? 'codex_app_server',
      syncState: normalizeWebSessionSyncState(session.ss),
      lastSyncMode: session.lsm === 'deep' || session.lsm === 'fast' ? session.lsm : null,
      sourceCreatedAt: session.sca ? new Date(session.sca).toISOString() : null,
      sourceUpdatedAt: session.sua ? new Date(session.sua).toISOString() : null,
      lastSyncedAt: session.lsa ? new Date(session.lsa).toISOString() : null,
      threadPath: session.tp ?? null,
      threadPreview: session.tpv ?? null,
      turnCount: Number(session.tc ?? 0),
      itemCount: Number(session.ic ?? 0),
      syncError: session.se ?? null,
      createdAt,
      updatedAt: new Date(session.lu).toISOString(),
      usage: {
        inputTokens: session.usa?.in ?? 0,
        cachedInputTokens: session.usa?.cin ?? 0,
        outputTokens: session.usa?.out ?? 0,
        cost: session.cost ?? 0,
      },
      latestTurnUsage: session.ltu
        ? {
            inputTokens: session.ltu.in ?? 0,
            cachedInputTokens: session.ltu.cin ?? 0,
            outputTokens: session.ltu.out ?? 0,
            usedTokens:
              session.ltu.usd ??
              Math.max(0, Number(session.ltu.in ?? 0) + Number(session.ltu.out ?? 0)),
          }
        : null,
      contextEstimate: {
        inputTokens: session.cea?.in ?? session.usa?.in ?? 0,
        cachedInputTokens: session.cea?.cin ?? session.usa?.cin ?? 0,
        outputTokens: session.cea?.out ?? session.usa?.out ?? 0,
        usedTokens:
          session.cea?.usd ??
          Math.max(
            0,
            Number(session.cea?.in ?? session.usa?.in ?? 0) +
              Number(session.cea?.out ?? session.usa?.out ?? 0)
          ),
      },
      contextEstimateMode:
        session.cem === 'latest_token_count'
          ? 'latest_token_count'
          : session.cem === 'latest_turn_delta'
            ? 'latest_turn_delta'
            : session.cem === 'since_compaction'
              ? 'since_compaction'
              : 'cumulative_total',
      lastContextCompactionAt: session.lcca ? new Date(session.lcca).toISOString() : null,
      contextWindowTokens:
        typeof session.cwt === 'number' && Number.isFinite(session.cwt) ? session.cwt : null,
      contextWindowSource:
        session.cws === 'config' ||
        session.cws === 'default' ||
        session.cws === 'session_usage' ||
        session.cws === 'unavailable'
          ? session.cws
          : 'unavailable',
    };
  }

  function normalizePendingInput(item: {
    id?: string;
    mode?: 'redirect' | 'queue' | string;
    text?: string;
    attachmentIds?: string[];
    createdAt?: string | number | null;
  }): WebSessionPendingInput | null {
    const id = typeof item.id === 'string' ? item.id.trim() : '';
    if (!id) {
      return null;
    }
    const mode = item.mode === 'redirect' ? 'redirect' : item.mode === 'queue' ? 'queue' : '';
    if (!mode) {
      return null;
    }
    const createdAt =
      typeof item.createdAt === 'number'
        ? item.createdAt
        : Date.parse(typeof item.createdAt === 'string' ? item.createdAt : '');
    return {
      id,
      mode,
      text: typeof item.text === 'string' ? item.text : '',
      attachmentIds: Array.isArray(item.attachmentIds)
        ? item.attachmentIds.filter((value): value is string => typeof value === 'string')
        : [],
      createdAt: Number.isFinite(createdAt) ? createdAt : Date.now(),
    };
  }

  function normalizeScheduledInput(item: {
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
  }): WebSessionScheduledInput | null {
    const id = typeof item.id === 'string' ? item.id.trim() : '';
    if (!id) {
      return null;
    }
    const mode =
      item.mode === 'send'
        ? 'send'
        : item.mode === 'interrupt' || item.mode === 'redirect'
          ? 'interrupt'
          : item.mode === 'queue'
            ? 'queue'
            : '';
    const status =
      item.status === 'failed' ? 'failed' : item.status === 'scheduled' ? 'scheduled' : '';
    if (!mode || !status) {
      return null;
    }
    const scheduledFor =
      typeof item.scheduledFor === 'number'
        ? item.scheduledFor
        : Date.parse(typeof item.scheduledFor === 'string' ? item.scheduledFor : '');
    if (!Number.isFinite(scheduledFor)) {
      return null;
    }
    const createdAt =
      typeof item.createdAt === 'number'
        ? item.createdAt
        : Date.parse(typeof item.createdAt === 'string' ? item.createdAt : '');
    const updatedAt =
      typeof item.updatedAt === 'number'
        ? item.updatedAt
        : Date.parse(typeof item.updatedAt === 'string' ? item.updatedAt : '');
    const sentAt =
      typeof item.sentAt === 'number'
        ? item.sentAt
        : Date.parse(typeof item.sentAt === 'string' ? item.sentAt : '');
    const canceledAt =
      typeof item.canceledAt === 'number'
        ? item.canceledAt
        : Date.parse(typeof item.canceledAt === 'string' ? item.canceledAt : '');
    return {
      id,
      mode,
      status,
      text: typeof item.text === 'string' ? item.text : '',
      attachmentIds: Array.isArray(item.attachmentIds)
        ? item.attachmentIds.filter((value): value is string => typeof value === 'string')
        : [],
      scheduledFor,
      createdAt: Number.isFinite(createdAt) ? createdAt : Date.now(),
      updatedAt: Number.isFinite(updatedAt)
        ? updatedAt
        : Number.isFinite(createdAt)
          ? createdAt
          : Date.now(),
      sentAt: Number.isFinite(sentAt) ? sentAt : null,
      canceledAt: Number.isFinite(canceledAt) ? canceledAt : null,
    };
  }

  function insertPendingInput(
    items: WebSessionPendingInput[],
    item: WebSessionPendingInput
  ): WebSessionPendingInput[] {
    if (item.mode !== 'redirect') {
      return [...items, item];
    }
    const insertAt = items.findIndex(existing => existing.mode !== 'redirect');
    if (insertAt < 0) {
      return [...items, item];
    }
    return [...items.slice(0, insertAt), item, ...items.slice(insertAt)];
  }

  function sortScheduledInputs(items: WebSessionScheduledInput[]) {
    return [...items].sort((left, right) => {
      if (left.status !== right.status) {
        return left.status === 'scheduled' ? -1 : 1;
      }
      if (left.scheduledFor !== right.scheduledFor) {
        return left.scheduledFor - right.scheduledFor;
      }
      return left.createdAt - right.createdAt;
    });
  }

  function normalizeHistoryItem(item: WireHistoryItem | Record<string, unknown>): WebSessionBlock {
    const record = asRecord(item) ?? {};
    const rawTimestamp = parseHistoryTimeValue(record.ts2 ?? record.timestamp);
    const rawObservedAt = parseHistoryTimeValue(record.obs ?? record.observedAt);
    const rawAttachments = Array.isArray(record.atts)
      ? record.atts
      : Array.isArray(record.attachments)
        ? record.attachments
        : [];
    const rawTool = asRecord(record.tl ?? record.tool);
    const rawDetail = asRecord(record.dt ?? record.detail);
    const rawPayload = asRecord(record.pl ?? record.payload);
    const kind = String(record.kd ?? record.kind ?? '').trim();
    const itemType = String(record.tp ?? record.itemType ?? '').trim();
    const detailType = String(rawDetail?.type ?? '').trim();
    const inferredDetailType =
      detailType ||
      (itemType === 'user_input_response'
        ? 'user_input_response'
        : itemType === 'user_input_request'
          ? 'user_input_request'
          : '');
    const detailQuestions = Array.isArray(rawDetail?.questions)
      ? (rawDetail.questions as WebSessionUserInputQuestion[])
      : undefined;
    const rawDetailAnswers = Array.isArray(rawDetail?.answers)
      ? (rawDetail.answers as WebSessionHistoryAnswerEntry[])
      : undefined;
    const detailAnswers =
      rawDetailAnswers && rawDetailAnswers.length > 0
        ? rawDetailAnswers
        : inferredDetailType === 'user_input_response'
          ? buildUserInputAnswerEntries(rawPayload ?? {}, detailQuestions ?? [])
          : rawDetailAnswers;

    return {
      key: `${String(record.id ?? '')}:${Number(record.oi ?? record.orderIndex ?? 0)}`,
      id: String(record.id ?? ''),
      sourceTurnId:
        typeof record.stid === 'string'
          ? record.stid
          : typeof record.sourceTurnId === 'string'
            ? record.sourceTurnId
            : null,
      sourceItemId: normalizeHistorySourceItemId(record, rawPayload),
      orderIndex: Number(record.oi ?? record.orderIndex ?? 0),
      kind:
        kind === 'user' || kind === 'assistant' || kind === 'system' || kind === 'tool'
          ? kind
          : 'system',
      itemType,
      text:
        typeof record.txt === 'string'
          ? record.txt
          : typeof record.text === 'string'
            ? record.text
            : '',
      timestamp: rawTimestamp ?? rawObservedAt ?? 0,
      observedAt: rawObservedAt ?? rawTimestamp ?? null,
      attachments: rawAttachments
        .map(attachment => asRecord(attachment))
        .filter((attachment): attachment is Record<string, unknown> => Boolean(attachment))
        .map(attachment => ({
          id: String(attachment.id ?? ''),
          name: String(attachment.name ?? ''),
          mime: typeof attachment.mime === 'string' ? attachment.mime : undefined,
          size:
            typeof attachment.sz === 'number'
              ? attachment.sz
              : typeof attachment.size === 'number'
                ? attachment.size
                : undefined,
          path: typeof attachment.path === 'string' ? attachment.path : undefined,
        }))
        .filter(attachment => Boolean(attachment.id || attachment.name)),
      tool: rawTool
        ? {
            id: String(rawTool.id ?? ''),
            name: String(rawTool.name ?? ''),
            kind: typeof rawTool.kind === 'string' ? rawTool.kind : undefined,
            input: rawTool.in ?? rawTool.input,
            output:
              typeof rawTool.out === 'string'
                ? rawTool.out
                : typeof rawTool.output === 'string'
                  ? rawTool.output
                  : undefined,
            status:
              rawTool.st === 'error' || rawTool.st === 'running' || rawTool.st === 'done'
                ? rawTool.st
                : rawTool.status === 'error' ||
                    rawTool.status === 'running' ||
                    rawTool.status === 'done'
                  ? rawTool.status
                  : rawTool.st === 'completed' || rawTool.status === 'completed'
                    ? 'done'
                    : 'running',
            startedAt:
              rawTimestamp != null
                ? rawTimestamp
                : rawObservedAt != null
                  ? rawObservedAt
                  : undefined,
            meta: asRecord(rawTool.meta),
            commandGroup: parseToolCommandGroup(rawTool.cg ?? rawTool.commandGroup),
          }
        : undefined,
      level:
        record.lvl === 'warn' || record.lvl === 'error' || record.lvl === 'info'
          ? record.lvl
          : record.level === 'warn' || record.level === 'error' || record.level === 'info'
            ? record.level
            : undefined,
      done: record.dn === true || record.done === true,
      detail:
        rawDetail || inferredDetailType
          ? {
              type:
                inferredDetailType === 'approval_request' ||
                inferredDetailType === 'approval_response' ||
                inferredDetailType === 'user_input_request' ||
                inferredDetailType === 'user_input_response'
                  ? inferredDetailType
                  : 'approval_request',
              prompt: typeof rawDetail?.prompt === 'string' ? rawDetail.prompt : undefined,
              questions: detailQuestions,
              answers: detailAnswers,
              action: typeof rawDetail?.action === 'string' ? rawDetail.action : undefined,
            }
          : undefined,
      payload: rawPayload,
    };
  }

  function compareArchivedSessions(left: WebSessionSummary, right: WebSessionSummary) {
    const leftActivity = Date.parse(left.activityAt || left.updatedAt || left.createdAt);
    const rightActivity = Date.parse(right.activityAt || right.updatedAt || right.createdAt);
    if (
      Number.isFinite(leftActivity) &&
      Number.isFinite(rightActivity) &&
      leftActivity !== rightActivity
    ) {
      return rightActivity - leftActivity;
    }
    return right.id.localeCompare(left.id);
  }

  function areStringArraysEqual(left: string[], right: string[]) {
    return left.length === right.length && left.every((value, index) => value === right[index]);
  }

  function reconcileArchivedListMeta(meta: ArchivedListMeta, total: number, offset: number) {
    const nextTotal = Math.max(0, total);
    const nextOffset = Math.max(0, Math.min(offset, nextTotal));
    return {
      ...meta,
      total: nextTotal,
      offset: nextOffset,
      hasMore: nextOffset < nextTotal,
    };
  }

  function getMatchingArchivedScopeKeys(projectId: string) {
    if (!projectId) {
      return [];
    }
    return Object.entries(archivedScopeStates.value)
      .filter(([, scopeState]) => scopeState.projectIds.includes(projectId))
      .map(([scopeKey]) => scopeKey);
  }

  function sortArchivedScopeContainingSession(sessionId: string) {
    const nextScopes = { ...archivedScopeStates.value };
    let changed = false;

    Object.entries(archivedScopeStates.value).forEach(([scopeKey, scopeState]) => {
      if (!scopeState.sessionIds.includes(sessionId)) {
        return;
      }
      const nextSessionIds = sortArchivedSessionIds(scopeState.sessionIds);
      if (areStringArraysEqual(nextSessionIds, scopeState.sessionIds)) {
        return;
      }
      nextScopes[scopeKey] = {
        ...scopeState,
        sessionIds: nextSessionIds,
      };
      changed = true;
    });

    if (changed) {
      archivedScopeStates.value = nextScopes;
    }
  }

  function addArchivedSessionToMatchingScopes(summary: WebSessionSummary) {
    const matchingScopeKeys = getMatchingArchivedScopeKeys(summary.projectId);
    if (matchingScopeKeys.length === 0) {
      return;
    }

    const nextScopes = { ...archivedScopeStates.value };
    let changed = false;

    matchingScopeKeys.forEach(scopeKey => {
      const scopeState = archivedScopeStates.value[scopeKey];
      if (!scopeState) {
        return;
      }

      const alreadyIncluded = scopeState.sessionIds.includes(summary.id);
      const nextSessionIds = sortArchivedSessionIds(
        alreadyIncluded ? scopeState.sessionIds : [...scopeState.sessionIds, summary.id]
      );
      const nextMeta = alreadyIncluded
        ? reconcileArchivedListMeta(scopeState.meta, scopeState.meta.total, scopeState.meta.offset)
        : reconcileArchivedListMeta(
            scopeState.meta,
            scopeState.meta.total + 1,
            scopeState.meta.offset + 1
          );

      if (
        alreadyIncluded &&
        areStringArraysEqual(nextSessionIds, scopeState.sessionIds) &&
        nextMeta.total === scopeState.meta.total &&
        nextMeta.offset === scopeState.meta.offset &&
        nextMeta.hasMore === scopeState.meta.hasMore
      ) {
        return;
      }

      nextScopes[scopeKey] = {
        ...scopeState,
        sessionIds: nextSessionIds,
        meta: nextMeta,
      };
      changed = true;
    });

    if (changed) {
      archivedScopeStates.value = nextScopes;
    }
  }

  function sortArchivedSessionIds(ids: string[]) {
    return [...ids].sort((leftId, rightId) => {
      const left = archivedSessionsById.value[leftId];
      const right = archivedSessionsById.value[rightId];
      if (!left && !right) {
        return leftId.localeCompare(rightId);
      }
      if (!left) {
        return 1;
      }
      if (!right) {
        return -1;
      }
      return compareArchivedSessions(left, right);
    });
  }

  function upsertArchivedSession(
    summary: WebSessionSummary,
    options?: {
      includeInMatchingScopes?: boolean;
      preserveScopeOrder?: boolean;
    }
  ) {
    const previous = archivedSessionsById.value[summary.id];
    archivedSessionsById.value = {
      ...archivedSessionsById.value,
      [summary.id]: {
        ...previous,
        ...summary,
      },
    };

    if (options?.includeInMatchingScopes) {
      addArchivedSessionToMatchingScopes(summary);
      return;
    }

    if (!previous) {
      return;
    }

    if (options?.preserveScopeOrder) {
      return;
    }

    sortArchivedScopeContainingSession(summary.id);
  }

  function removeArchivedSessionRecord(sessionId: string, options?: { clearSummary?: boolean }) {
    const archived = archivedSessionsById.value[sessionId];
    const projectId = archived?.projectId ?? '';
    const nextScopes = { ...archivedScopeStates.value };
    let changed = false;

    Object.entries(archivedScopeStates.value).forEach(([scopeKey, scopeState]) => {
      const containsSession = scopeState.sessionIds.includes(sessionId);
      const matchesProject = Boolean(projectId) && scopeState.projectIds.includes(projectId);
      if (!containsSession && !matchesProject) {
        return;
      }

      const nextSessionIds = containsSession
        ? scopeState.sessionIds.filter(id => id !== sessionId)
        : scopeState.sessionIds;
      const nextMeta = matchesProject
        ? reconcileArchivedListMeta(
            scopeState.meta,
            scopeState.meta.total - 1,
            scopeState.meta.offset - (containsSession ? 1 : 0)
          )
        : scopeState.meta;

      if (
        areStringArraysEqual(nextSessionIds, scopeState.sessionIds) &&
        nextMeta.total === scopeState.meta.total &&
        nextMeta.offset === scopeState.meta.offset &&
        nextMeta.hasMore === scopeState.meta.hasMore
      ) {
        return;
      }

      nextScopes[scopeKey] = {
        ...scopeState,
        sessionIds: nextSessionIds,
        meta: nextMeta,
      };
      changed = true;
    });

    if (changed) {
      archivedScopeStates.value = nextScopes;
    }

    if (options?.clearSummary !== false) {
      const next = { ...archivedSessionsById.value };
      delete next[sessionId];
      archivedSessionsById.value = next;
    }
  }

  function removeCurrentSessionRecord(projectId: string, sessionId: string) {
    const current = sessionsByProject.value[projectId] ?? [];
    const removed = current.find(item => item.id === sessionId) ?? null;
    const next = current.filter(item => item.id !== sessionId);
    sessionsByProject.value = {
      ...sessionsByProject.value,
      [projectId]: next,
    };
    syncSessionCount(projectId);
    const currentActive = activeSessionIdByProject.value[projectId];
    if (currentActive === sessionId) {
      activeSessionIdByProject.value = {
        ...activeSessionIdByProject.value,
        [projectId]: '',
      };
      persistActiveSessions(activeSessionIdByProject.value);
    }
    return removed;
  }

  function clearSessionRuntimeState(sessionId: string, projectId?: string) {
    const nextEvents = { ...eventsBySession.value };
    delete nextEvents[sessionId];
    eventsBySession.value = nextEvents;
    const nextHistory = { ...historyBySession.value };
    delete nextHistory[sessionId];
    historyBySession.value = nextHistory;
    appliedSnapshotVersionBySession.delete(sessionId);
    pendingAutoRetryOverrides.delete(sessionId);
    pendingActiveCallTimeoutOverrides.delete(sessionId);
    const nextPendingInputs = { ...pendingInputsBySession.value };
    delete nextPendingInputs[sessionId];
    pendingInputsBySession.value = nextPendingInputs;
    const nextScheduledInputs = { ...scheduledInputsBySession.value };
    delete nextScheduledInputs[sessionId];
    scheduledInputsBySession.value = nextScheduledInputs;
    completedTransitionVersionBySession.delete(sessionId);
    if (projectId) {
      clearDraft(projectId, sessionId);
    }
  }

  function upsertCurrentSession(summary: WebSessionSummary) {
    const nextSummary = applyPendingActiveCallTimeoutOverride(
      applyPendingAutoRetryOverride(summary)
    );
    const current = sessionsByProject.value[nextSummary.projectId] ?? [];
    const next = [...current];
    const index = next.findIndex(item => item.id === nextSummary.id);
    if (index >= 0) {
      next.splice(index, 1, {
        ...next[index],
        ...nextSummary,
      });
    } else {
      next.unshift(nextSummary);
    }
    sessionsByProject.value = {
      ...sessionsByProject.value,
      [nextSummary.projectId]: sortSessions(next),
    };
    syncSessionCount(nextSummary.projectId);
  }

  function upsertSession(
    summary: WebSessionSummary,
    options?: { preserveArchivedPosition?: boolean }
  ) {
    if (summary.archivedAt) {
      const wasCurrentSession = Boolean(
        (sessionsByProject.value[summary.projectId] ?? []).some(item => item.id === summary.id)
      );
      removeCurrentSessionRecord(summary.projectId, summary.id);
      upsertArchivedSession(summary, {
        includeInMatchingScopes: wasCurrentSession,
        preserveScopeOrder: options?.preserveArchivedPosition === true,
      });
      return;
    }
    removeArchivedSessionRecord(summary.id);
    upsertCurrentSession(summary);
  }

  function removeSession(projectId: string, sessionId: string) {
    const removed =
      removeCurrentSessionRecord(projectId, sessionId) ??
      archivedSessionsById.value[sessionId] ??
      null;
    removeArchivedSessionRecord(sessionId);
    clearSessionRuntimeState(sessionId, projectId);
    if (removed) {
      emitter.emit('ai:closed', {
        sessionId: removed.id,
        sessionTitle: removed.title,
        projectId: removed.projectId,
        assistant: getAssistantDescriptor(removed),
      } satisfies WebSessionAIEvent);
    }
  }

  function setPendingInputs(sessionId: string, items: WebSessionPendingInput[]) {
    const nextPendingInputs = { ...pendingInputsBySession.value };
    if (items.length === 0) {
      delete nextPendingInputs[sessionId];
    } else {
      nextPendingInputs[sessionId] = items;
    }
    pendingInputsBySession.value = nextPendingInputs;
  }

  function setScheduledInputs(sessionId: string, items: WebSessionScheduledInput[]) {
    const nextScheduledInputs = { ...scheduledInputsBySession.value };
    if (items.length === 0) {
      delete nextScheduledInputs[sessionId];
    } else {
      nextScheduledInputs[sessionId] = sortScheduledInputs(items);
    }
    scheduledInputsBySession.value = nextScheduledInputs;
  }

  function mergeEvents(sessionId: string, incoming: WebSessionBlock[]) {
    const merged = [...(eventsBySession.value[sessionId] ?? [])];
    const indexById = new Map(merged.map((item, index) => [item.id, index]));
    incoming.forEach(item => {
      if (!item || !item.id) {
        return;
      }
      const existingIndex = indexById.get(item.id);
      if (existingIndex == null) {
        merged.push(item);
        indexById.set(item.id, merged.length - 1);
        return;
      }
      merged.splice(existingIndex, 1, {
        ...merged[existingIndex],
        ...item,
      });
    });
    merged.sort((left, right) => left.orderIndex - right.orderIndex);
    eventsBySession.value = {
      ...eventsBySession.value,
      [sessionId]: merged,
    };
  }

  function trimInactiveSessionEvents(activeSessionId = '') {
    const nextEvents = { ...eventsBySession.value };
    let changed = false;
    Object.entries(eventsBySession.value).forEach(([sessionId, items]) => {
      if (sessionId === activeSessionId || items.length <= WEB_SESSION_MAX_RETAINED_BLOCKS) {
        return;
      }
      nextEvents[sessionId] = items.slice(-WEB_SESSION_MIN_RETAINED_BLOCKS);
      const nextMeta = getHistoryMeta(sessionId);
      historyBySession.value = {
        ...historyBySession.value,
        [sessionId]: {
          ...nextMeta,
          hasMore: true,
          beforeCursor:
            nextEvents[sessionId].length > 0
              ? String(nextEvents[sessionId][0]?.orderIndex ?? nextMeta.beforeCursor)
              : nextMeta.beforeCursor,
        },
      };
      changed = true;
    });
    if (changed) {
      eventsBySession.value = nextEvents;
    }
  }

  function resetSessionEvents(sessionId: string, events: WebSessionBlock[]) {
    eventsBySession.value = {
      ...eventsBySession.value,
      [sessionId]: [...events].sort((left, right) => left.orderIndex - right.orderIndex),
    };
  }

  function buildBlocks(sessionId: string): WebSessionBlock[] {
    return eventsBySession.value[sessionId] ?? [];
  }

  const getBlocks = (sessionId: string) => buildBlocks(sessionId);

  function getPendingApproval(sessionId: string): WebSessionApprovalState | null {
    const blocks = buildBlocks(sessionId);
    let pending: WebSessionApprovalState | null = null;
    for (const block of blocks) {
      if (block.detail?.type === 'approval_request') {
        pending = {
          id: block.id,
          prompt: block.detail.prompt ?? block.text,
          requestedAt: block.timestamp,
          stale: false,
        };
        continue;
      }
      if (block.detail?.type === 'approval_response' || block.kind === 'user') {
        pending = null;
        continue;
      }
      if (
        block.itemType === 'run_abort' &&
        pending &&
        isProcessRestartPayload(block.payload ?? undefined)
      ) {
        pending = {
          ...pending,
          stale: true,
          recoveryReason: String(block.payload?.reason ?? ''),
          recoveryMessage: getRecoveryMessage(block.payload ?? undefined),
        };
        continue;
      }
      if (block.itemType === 'run_abort' || block.itemType === 'run_fail') {
        pending = null;
      }
    }
    return pending;
  }

  function getPendingUserInput(sessionId: string): WebSessionUserInputState | null {
    const blocks = buildBlocks(sessionId);
    let pending: WebSessionUserInputState | null = null;
    for (const block of blocks) {
      if (block.detail?.type === 'user_input_request') {
        pending = {
          id: block.id,
          itemId: block.sourceItemId || block.id,
          prompt: block.detail.prompt ?? block.text,
          questions: block.detail.questions ?? [],
          requestedAt: block.timestamp,
          stale: false,
        };
        continue;
      }
      if (block.detail?.type === 'user_input_response' || block.kind === 'user') {
        pending = null;
        continue;
      }
      if (
        block.itemType === 'run_abort' &&
        pending &&
        isProcessRestartPayload(block.payload ?? undefined)
      ) {
        pending = {
          ...pending,
          stale: true,
          recoveryReason: String(block.payload?.reason ?? ''),
          recoveryMessage: getRecoveryMessage(block.payload ?? undefined),
        };
        continue;
      }
      if (block.itemType === 'run_abort' || block.itemType === 'run_fail') {
        pending = null;
      }
    }
    return pending;
  }

  function getLiveState(sessionId: string): WebSessionLiveState {
    const session = findSessionById(sessionId);
    const approval = getPendingApproval(sessionId);
    const userInput = getPendingUserInput(sessionId);
    const assistantState = getSessionAssistantStateValue(session);
    let activeTool:
      | {
          id: string;
          name: string;
          kind?: string;
          summary?: string;
          count?: number;
          groupId?: string;
          startedAt?: number;
        }
      | undefined;
    let activeSubAgents = new Map<string, WebSessionLiveSubAgent>();
    let knownSubAgents = new Map<string, WebSessionLiveSubAgent>();
    let sawAssistantOutput = false;
    let assistantDone = false;
    let firstAssistantOutputAt: number | undefined;
    let errorMessage = '';
    let updatedAt = session ? Date.parse(session.updatedAt) || Date.now() : Date.now();
    const assistantStateUpdatedAt = getAssistantStateUpdatedAt(session);
    let runStartedAt: number | undefined;
    let retryState:
      | {
          code: string;
          message: string;
          attempt?: number;
          maxAttempts?: number;
          updatedAt: number;
        }
      | undefined;

    for (const block of buildBlocks(sessionId)) {
      updatedAt = block.observedAt || block.timestamp || updatedAt;
      if (block.kind === 'assistant') {
        sawAssistantOutput = true;
        assistantDone = block.done === true;
        retryState = undefined;
        if (!firstAssistantOutputAt && block.timestamp > 0) {
          firstAssistantOutputAt = block.timestamp;
        }
        applyAssistantNamedSubAgents(block.text, knownSubAgents, activeSubAgents);
      }
      if (block.kind === 'user' && block.timestamp > 0) {
        runStartedAt = block.timestamp;
        sawAssistantOutput = false;
        assistantDone = false;
        firstAssistantOutputAt = undefined;
        activeTool = undefined;
        activeSubAgents = new Map();
        knownSubAgents = new Map();
        errorMessage = '';
        retryState = undefined;
      }
      const retryPayload = getTransportRetryPayload(block.payload);
      if (block.itemType === 'note' && retryPayload) {
        retryState = {
          ...retryPayload,
          updatedAt: block.observedAt || block.timestamp || updatedAt,
        };
      } else if (retryState && isRetryClearingProgressBlock(block, retryPayload)) {
        retryState = undefined;
      }
      if (block.kind === 'tool' && block.tool) {
        const normalizedToolKind = normalizeToolKindValue(
          block.tool.kind || String(block.tool.meta?.kind ?? '')
        );
        if (normalizedToolKind === 'reasoning') {
          continue;
        }
        if (normalizedToolKind === 'sub_agent_tool_call') {
          rememberKnownSubAgents(block, knownSubAgents);
          syncActiveSubAgentLifecycle(block, knownSubAgents, activeSubAgents);
          retryState = undefined;
          continue;
        }
        if (block.tool.status === 'running') {
          activeTool = normalizeActiveTool(block);
          retryState = undefined;
        } else if (activeTool?.id === block.tool.id) {
          activeTool = undefined;
          retryState = undefined;
        }
      }
      if (block.itemType === 'run_fail') {
        errorMessage = block.text || 'Run failed';
        activeTool = undefined;
        activeSubAgents = new Map();
        retryState = undefined;
      }
      if (block.itemType === 'run_abort') {
        activeTool = undefined;
        activeSubAgents = new Map();
      }
    }

    if (assistantState === 'waiting_approval') {
      return withActiveSubAgents(
        {
          phase: 'waiting_approval',
          running: session?.status === 'running',
          updatedAt: approval?.requestedAt ?? assistantStateUpdatedAt ?? updatedAt,
          startedAt: approval?.requestedAt ?? assistantStateUpdatedAt ?? runStartedAt,
          approval,
          tool: activeTool,
        },
        [...activeSubAgents.values()]
      );
    }

    if (assistantState === 'waiting_plan_approval') {
      return withActiveSubAgents(
        {
          phase: 'waiting_plan_approval',
          running: false,
          updatedAt: assistantStateUpdatedAt || updatedAt,
          startedAt: assistantStateUpdatedAt ?? runStartedAt,
        },
        [...activeSubAgents.values()]
      );
    }

    if (assistantState === 'waiting_input') {
      return withActiveSubAgents(
        {
          phase: 'waiting_input',
          running: session?.status === 'running',
          updatedAt: userInput?.requestedAt ?? assistantStateUpdatedAt ?? updatedAt,
          startedAt: userInput?.requestedAt ?? assistantStateUpdatedAt ?? runStartedAt,
          tool: activeTool,
          userInput,
        },
        [...activeSubAgents.values()]
      );
    }

    if (session?.status === 'running') {
      const hasNewerWorkingSummary =
        retryState != null &&
        assistantState === 'working' &&
        assistantStateUpdatedAt != null &&
        assistantStateUpdatedAt > retryState.updatedAt;
      if (retryState && !hasNewerWorkingSummary) {
        return withActiveSubAgents(
          {
            phase: 'retrying',
            running: true,
            updatedAt: retryState.updatedAt,
            startedAt: runStartedAt,
            retry: {
              code: retryState.code,
              message: retryState.message,
              attempt: retryState.attempt,
              maxAttempts: retryState.maxAttempts,
            },
          },
          [...activeSubAgents.values()]
        );
      }
      if (activeTool) {
        return withActiveSubAgents(
          {
            phase: 'tool',
            running: true,
            updatedAt,
            startedAt: activeTool.startedAt ?? assistantStateUpdatedAt ?? runStartedAt,
            tool: activeTool,
          },
          [...activeSubAgents.values()]
        );
      }
      if (sawAssistantOutput && !assistantDone) {
        return withActiveSubAgents(
          {
            phase: 'thinking',
            running: true,
            updatedAt,
            startedAt: firstAssistantOutputAt ?? assistantStateUpdatedAt ?? runStartedAt,
          },
          [...activeSubAgents.values()]
        );
      }
      return withActiveSubAgents(
        {
          phase: 'starting',
          running: true,
          updatedAt,
          startedAt: assistantStateUpdatedAt ?? runStartedAt,
        },
        [...activeSubAgents.values()]
      );
    }

    if (session?.status === 'done') {
      return {
        phase: 'done',
        running: false,
        updatedAt,
        startedAt: runStartedAt,
      };
    }

    if (session?.status === 'err') {
      return {
        phase: 'error',
        running: false,
        updatedAt,
        startedAt: runStartedAt,
        errorMessage,
      };
    }

    return {
      phase: 'idle',
      running: false,
      updatedAt,
    };
  }

  function snapshotRuntimeMutationState(sessionId: string): RuntimeMutationStateSnapshot {
    const liveState = getLiveState(sessionId);
    return {
      blockCount: buildBlocks(sessionId).length,
      historyTotal: getHistoryMeta(sessionId).total,
      pendingInputCount: getPendingInputs(sessionId).length,
      livePhase: liveState.phase,
      liveRunning: liveState.running,
      liveUpdatedAt: liveState.updatedAt,
      approvalId: getPendingApproval(sessionId)?.id ?? '',
      userInputId: getPendingUserInput(sessionId)?.itemId ?? '',
    };
  }

  function delayRuntimeMutation(ms: number) {
    const timeoutMs = Math.max(0, Math.trunc(ms));
    return new Promise<void>(resolve => {
      globalThis.setTimeout(resolve, timeoutMs);
    });
  }

  async function waitForRuntimeMutationPredicate(
    predicate: () => boolean,
    timeoutMs: number,
    pollMs: number
  ) {
    const deadline = Date.now() + Math.max(0, timeoutMs);
    while (Date.now() < deadline) {
      if (predicate()) {
        return true;
      }
      await delayRuntimeMutation(Math.min(Math.max(1, pollMs), Math.max(1, deadline - Date.now())));
    }
    return predicate();
  }

  async function hydrateRuntimeMutation(
    sessionId: string,
    options: RuntimeMutationHydrationOptions
  ) {
    if (options.predicate()) {
      return true;
    }

    const passiveWaitMs = Math.max(
      0,
      options.passiveWaitMs ?? WEB_SESSION_RUNTIME_MUTATION_PASSIVE_WAIT_MS
    );
    if (passiveWaitMs > 0) {
      const settledPassively = await waitForRuntimeMutationPredicate(
        options.predicate,
        passiveWaitMs,
        WEB_SESSION_RUNTIME_MUTATION_PASSIVE_POLL_MS
      );
      if (settledPassively) {
        return true;
      }
    }

    const session = findSessionById(sessionId);
    if (!session?.projectId || session.archivedAt) {
      return options.predicate();
    }

    const deadline =
      Date.now() + Math.max(0, options.timeoutMs ?? WEB_SESSION_RUNTIME_MUTATION_TIMEOUT_MS);
    const snapshotPollMs = Math.max(
      1,
      options.snapshotPollMs ?? WEB_SESSION_RUNTIME_MUTATION_SNAPSHOT_POLL_MS
    );
    let lastSnapshotError: unknown = null;

    while (Date.now() < deadline) {
      if (options.predicate()) {
        return true;
      }

      try {
        await loadSessionSnapshot(session.projectId, sessionId, {
          rememberActive: false,
        });
        lastSnapshotError = null;
      } catch (error) {
        lastSnapshotError = error;
      }

      if (options.predicate()) {
        return true;
      }

      const remainingMs = deadline - Date.now();
      if (remainingMs <= 0) {
        break;
      }
      await delayRuntimeMutation(Math.min(snapshotPollMs, remainingMs));
    }

    if (lastSnapshotError) {
      console.warn('[Web Session] Runtime mutation hydration did not settle cleanly', {
        sessionId,
        label: options.label,
        error: lastSnapshotError,
      });
    }
    return options.predicate();
  }

  function updateSessionStatus(
    sessionId: string,
    updater: (current: WebSessionSummary) => WebSessionSummary
  ) {
    const entries = Object.entries(sessionsByProject.value);
    let changed = false;
    const nextSessions: Record<string, WebSessionSummary[]> = {};
    entries.forEach(([projectId, sessions]) => {
      const nextProjectSessions = sessions.map(item => {
        if (item.id !== sessionId) {
          return item;
        }
        changed = true;
        return updater(item);
      });
      nextSessions[projectId] = sortSessions(nextProjectSessions);
    });
    if (changed) {
      sessionsByProject.value = nextSessions;
      return;
    }

    const archived = archivedSessionsById.value[sessionId];
    if (archived) {
      archivedSessionsById.value = {
        ...archivedSessionsById.value,
        [sessionId]: updater(archived),
      };
      sortArchivedScopeContainingSession(sessionId);
    }
  }

  function getAssistantDescriptor(session: WebSessionSummary): WebSessionAssistantDescriptor {
    return session.agent === 'claude'
      ? {
          type: 'claude-code',
          name: 'Claude Code',
          displayName: 'Claude Code',
        }
      : {
          type: 'codex',
          name: 'Codex',
          displayName: 'Codex',
        };
  }

  function emitStateTransition(
    sessionId: string,
    previousState: WebSessionLiveState,
    previousApproval: WebSessionApprovalState | null
  ) {
    const session = findSessionById(sessionId);
    if (!session) {
      return;
    }

    const nextState = getLiveState(sessionId);
    const nextApproval = getPendingApproval(sessionId);
    const hasPendingInputs = getPendingInputs(sessionId).length > 0;
    const approvalForNotification =
      nextApproval ??
      (nextState.phase === 'waiting_approval' || nextState.phase === 'waiting_plan_approval'
        ? {
            id: `status:${sessionId}:${nextState.updatedAt}`,
            prompt: '',
            requestedAt: nextState.updatedAt,
            stale: false,
          }
        : null);
    const baseEvent: WebSessionAIEvent = {
      sessionId,
      sessionTitle: session.title,
      projectId: session.projectId,
      assistant: getAssistantDescriptor(session),
    };

    if (isWorkingPhase(nextState.phase) && !isWorkingPhase(previousState.phase)) {
      emitter.emit('ai:working', baseEvent);
    }

    if (
      approvalForNotification &&
      (!previousApproval ||
        previousApproval.id !== approvalForNotification.id ||
        previousApproval.requestedAt !== approvalForNotification.requestedAt ||
        (previousState.phase !== 'waiting_approval' &&
          previousState.phase !== 'waiting_plan_approval'))
    ) {
      emitter.emit('ai:approval-needed', {
        ...baseEvent,
        approval: approvalForNotification,
      } satisfies WebSessionApprovalEvent);
    }

    if (nextState.phase === 'done' && previousState.phase !== 'done' && !hasPendingInputs) {
      const completionVersion = Math.max(
        nextState.updatedAt,
        Date.parse(session.updatedAt || '') || 0,
        Date.parse(session.lastMessageAt || '') || 0
      );
      const lastCompletionVersion = completedTransitionVersionBySession.get(sessionId) ?? -1;
      if (completionVersion > lastCompletionVersion) {
        completedTransitionVersionBySession.set(sessionId, completionVersion);
        emitter.emit('ai:completed', baseEvent);
      }
    }

    if (
      (nextState.phase === 'idle' || nextState.phase === 'error') &&
      nextState.phase !== previousState.phase
    ) {
      emitter.emit('ai:closed', baseEvent);
    }
  }

  function applyFrame(frame: WireFrame) {
    if (frame.k === 'hb') {
      return;
    }

    if (frame.k === 'err') {
      lastError.value = frame.msg ?? 'Unknown websocket error';
      if (frame.rid && pending.has(frame.rid)) {
        pending.get(frame.rid)?.reject(new Error(frame.msg ?? frame.code ?? 'unknown error'));
        pending.delete(frame.rid);
      }
      return;
    }

    if (frame.k === 'ack') {
      if (frame.rid && pending.has(frame.rid)) {
        pending.get(frame.rid)?.resolve(frame);
        pending.delete(frame.rid);
      }
      return;
    }

    if (frame.k === 'snap' && frame.sid && frame.s) {
      const summary = normalizeSession(frame.s);
      const historyTotal = Number(frame.h?.tot ?? frame.h?.its?.length ?? 0);
      const snapshotInput = currentSnapshotVersionInput(frame.sid);
      const appliedVersion = appliedSnapshotVersionBySession.get(frame.sid) ?? null;
      const currentVersion = selectLatestWebSessionSnapshotVersion(
        appliedVersion,
        snapshotInput ? buildWebSessionSnapshotVersion(snapshotInput) : null
      );
      const incomingSnapshot = {
        session: summary,
        historyTotal,
      };
      if (
        !shouldApplyIncomingWebSessionSnapshot({
          appliedVersion,
          currentSnapshot: snapshotInput,
          incomingSnapshot,
        })
      ) {
        if (import.meta.env.DEV) {
          console.debug('[Web Session] Dropped stale snapshot frame', {
            sessionId: frame.sid,
            currentVersion,
            incomingVersion: buildWebSessionSnapshotVersion(incomingSnapshot),
          });
        }
        setHistoryLoading(frame.sid, false);
        return;
      }
      applySessionSnapshot(
        frame.sid,
        summary,
        Array.isArray(frame.h?.its) ? frame.h.its.map(item => normalizeHistoryItem(item)) : [],
        Array.isArray(frame.pi)
          ? frame.pi
              .map(item =>
                normalizePendingInput({
                  id: item.id,
                  mode: item.m,
                  text: item.txt,
                  attachmentIds: item.atts,
                  createdAt: item.ca,
                })
              )
              .filter((item): item is WebSessionPendingInput => item != null)
          : [],
        Array.isArray(frame.si)
          ? frame.si
              .map(item =>
                normalizeScheduledInput({
                  id: item.id,
                  mode: item.m,
                  status: item.st,
                  text: item.txt,
                  attachmentIds: item.atts,
                  scheduledFor: item.sf,
                  createdAt: item.ca,
                  updatedAt: item.ua,
                  sentAt: item.sa,
                  canceledAt: item.xa,
                })
              )
              .filter((item): item is WebSessionScheduledInput => item != null)
          : [],
        {
          hasMore: frame.h?.hm ?? false,
          beforeCursor: frame.h?.bc ?? '',
          total: historyTotal,
        }
      );
      return;
    }

    if (frame.k === 'evt' && frame.sid) {
      const previousState = getLiveState(frame.sid);
      const previousApproval = getPendingApproval(frame.sid);
      if (frame.s) {
        upsertSession(normalizeSession(frame.s));
      }
      if (frame.op === 'pending') {
        setPendingInputs(
          frame.sid,
          Array.isArray(frame.pi)
            ? frame.pi
                .map(item =>
                  normalizePendingInput({
                    id: item.id,
                    mode: item.m,
                    text: item.txt,
                    attachmentIds: item.atts,
                    createdAt: item.ca,
                  })
                )
                .filter((item): item is WebSessionPendingInput => item != null)
            : []
        );
        emitStateTransition(frame.sid, previousState, previousApproval);
        return;
      }

      if (frame.op === 'scheduled') {
        setScheduledInputs(
          frame.sid,
          Array.isArray(frame.si)
            ? frame.si
                .map(item =>
                  normalizeScheduledInput({
                    id: item.id,
                    mode: item.m,
                    status: item.st,
                    text: item.txt,
                    attachmentIds: item.atts,
                    scheduledFor: item.sf,
                    createdAt: item.ca,
                    updatedAt: item.ua,
                    sentAt: item.sa,
                    canceledAt: item.xa,
                  })
                )
                .filter((item): item is WebSessionScheduledInput => item != null)
            : []
        );
        emitStateTransition(frame.sid, previousState, previousApproval);
        return;
      }

      if (frame.op === 'hist_page' && frame.h) {
        const historicalItems = Array.isArray(frame.h.its)
          ? frame.h.its.map(item => normalizeHistoryItem(item))
          : [];
        mergeEvents(frame.sid, historicalItems);
        historyBySession.value = {
          ...historyBySession.value,
          [frame.sid]: {
            ...getHistoryMeta(frame.sid),
            hasMore: Boolean(frame.h.hm),
            beforeCursor: String(frame.h.bc ?? ''),
            loading: false,
          },
        };
        return;
      }

      if (frame.op === 'hist_item' && frame.i) {
        const item = normalizeHistoryItem(frame.i);
        mergeEvents(frame.sid, [item]);
      }

      emitStateTransition(frame.sid, previousState, previousApproval);
    }
  }

  function rejectPendingCommands(reason: Error) {
    pending.forEach(entry => {
      entry.reject(reason);
    });
    pending.clear();
  }

  function buildHeartbeatPayload(op: WireHeartbeatOp, sessionId = '') {
    return JSON.stringify({
      v: 1,
      k: 'hb',
      ts: Date.now(),
      op,
      sid: sessionId || undefined,
    });
  }

  function setSocketLastSeen(kind: WebSessionSocketKind, timestamp = Date.now()) {
    if (kind === 'event') {
      eventLastSeenAt.value = timestamp;
      return;
    }
    commandLastSeenAt = timestamp;
  }

  function getSocketLastSeen(kind: WebSessionSocketKind) {
    return kind === 'event' ? eventLastSeenAt.value : commandLastSeenAt;
  }

  function clearSocketWatchdog(kind: WebSessionSocketKind) {
    const timer = kind === 'event' ? eventWatchdogTimer : commandWatchdogTimer;
    if (timer != null) {
      window.clearInterval(timer);
    }
    if (kind === 'event') {
      eventWatchdogTimer = null;
      return;
    }
    commandWatchdogTimer = null;
  }

  function closeSocketForHeartbeatTimeout(kind: WebSessionSocketKind, socket: WebSocket) {
    if (kind === 'event') {
      eventLastDisconnectReason.value = 'heartbeat_timeout';
      lastError.value = 'web session event websocket heartbeat timed out';
      connectionState.value = 'closed';
    } else {
      rejectPendingCommands(new Error('websocket command channel heartbeat timed out'));
    }
    clearSocketWatchdog(kind);
    try {
      socket.close();
    } catch (error) {
      console.error('[Web Session] Failed to close websocket after heartbeat timeout', error);
    }
  }

  function startSocketWatchdog(kind: WebSessionSocketKind, socket: WebSocket) {
    clearSocketWatchdog(kind);
    setSocketLastSeen(kind);
    const timer = window.setInterval(() => {
      const activeSocket = kind === 'event' ? eventSocket : commandSocket;
      if (activeSocket !== socket || socket.readyState !== WebSocket.OPEN) {
        return;
      }
      const lastSeen = getSocketLastSeen(kind);
      if (lastSeen <= 0) {
        setSocketLastSeen(kind);
        return;
      }
      if (Date.now() - lastSeen > WEB_SESSION_SOCKET_IDLE_TIMEOUT_MS) {
        closeSocketForHeartbeatTimeout(kind, socket);
      }
    }, WEB_SESSION_SOCKET_WATCHDOG_INTERVAL_MS);
    if (kind === 'event') {
      eventWatchdogTimer = timer;
      return;
    }
    commandWatchdogTimer = timer;
  }

  function sendSocketHeartbeat(socket: WebSocket | null, op: WireHeartbeatOp, sessionId = '') {
    if (!socket || socket.readyState !== WebSocket.OPEN) {
      return;
    }
    socket.send(buildHeartbeatPayload(op, sessionId));
  }

  function sendEventSessionFocus(sessionId = '') {
    sendSocketHeartbeat(eventSocket, 'focus', sessionId);
  }

  function setEventSessionFocus(sessionId: string) {
    const normalizedSessionId = String(sessionId || '').trim();
    if (eventFocusedSessionId === normalizedSessionId) {
      return;
    }
    eventFocusedSessionId = normalizedSessionId;
    sendEventSessionFocus(normalizedSessionId);
  }

  function handleSocketHeartbeat(kind: WebSessionSocketKind, socket: WebSocket, frame: WireFrame) {
    if (frame.k !== 'hb') {
      return false;
    }
    setSocketLastSeen(kind);
    if (frame.op === 'ping') {
      try {
        sendSocketHeartbeat(socket, 'pong');
      } catch (error) {
        console.error('[Web Session] Failed to reply to websocket heartbeat', error);
      }
    }
    return true;
  }

  function clearEventReconnectTimer() {
    if (eventReconnectTimer != null) {
      window.clearTimeout(eventReconnectTimer);
      eventReconnectTimer = null;
    }
  }

  function scheduleEventReconnect() {
    if (allSessionIds.value.size === 0) {
      clearEventReconnectTimer();
      return;
    }
    clearEventReconnectTimer();
    const delayMs = Math.min(
      WEB_SESSION_EVENT_RECONNECT_BASE_DELAY_MS * 2 ** eventReconnectAttempt,
      WEB_SESSION_EVENT_RECONNECT_MAX_DELAY_MS
    );
    eventReconnectAttempt += 1;
    eventReconnectTimer = window.setTimeout(() => {
      eventReconnectTimer = null;
      if (allSessionIds.value.size === 0) {
        return;
      }
      void openEventStream().catch(() => undefined);
    }, delayMs);
  }

  function openEventStream(): Promise<void> {
    if (eventSocket && eventSocket.readyState === WebSocket.OPEN) {
      connectionState.value = 'open';
      return Promise.resolve();
    }
    if (eventConnectPromise) {
      return eventConnectPromise;
    }
    clearEventReconnectTimer();
    connectionState.value = 'connecting';
    eventConnectPromise = new Promise((resolve, reject) => {
      let settled = false;
      let ws: WebSocket;
      try {
        ws = new WebSocket(resolveWsUrl(EVENTS_WS_PATH));
      } catch (error) {
        scheduleEventReconnect();
        reject(error);
        return;
      }
      ws.onopen = () => {
        settled = true;
        eventSocket = ws;
        connectionState.value = 'open';
        eventLastDisconnectReason.value = null;
        eventReconnectAttempt = 0;
        startSocketWatchdog('event', ws);
        eventConnectPromise = null;
        if (eventFocusedSessionId) {
          sendEventSessionFocus(eventFocusedSessionId);
        }
        if (eventHasConnectedOnce) {
          eventRecoveryVersion.value += 1;
          emitter.emit('web-session:event-stream-recovered', {
            recoveredAt: new Date().toISOString(),
          });
        }
        eventHasConnectedOnce = true;
        resolve();
      };
      ws.onmessage = event => {
        try {
          const frame = JSON.parse(event.data) as WireFrame;
          setSocketLastSeen('event');
          if (handleSocketHeartbeat('event', ws, frame)) {
            return;
          }
          applyFrame(frame);
        } catch (error) {
          console.error('[Web Session] Failed to parse event websocket frame', error);
        }
      };
      ws.onerror = event => {
        console.error('[Web Session] event websocket error', event);
      };
      ws.onclose = () => {
        eventSocket = null;
        connectionState.value = 'closed';
        if (!eventLastDisconnectReason.value) {
          eventLastDisconnectReason.value = 'socket_closed';
        }
        clearSocketWatchdog('event');
        eventConnectPromise = null;
        scheduleEventReconnect();
        if (!settled) {
          reject(new Error('websocket event stream closed before opening'));
          return;
        }
      };
    });
    return eventConnectPromise.catch(error => {
      eventConnectPromise = null;
      connectionState.value = 'closed';
      throw error;
    });
  }

  function openCommandSocket(): Promise<void> {
    if (commandSocket && commandSocket.readyState === WebSocket.OPEN) {
      return Promise.resolve();
    }
    if (commandConnectPromise) {
      return commandConnectPromise;
    }
    commandConnectPromise = new Promise((resolve, reject) => {
      let settled = false;
      const ws = new WebSocket(resolveWsUrl(COMMAND_WS_PATH));
      ws.onopen = () => {
        settled = true;
        commandSocket = ws;
        startSocketWatchdog('command', ws);
        commandConnectPromise = null;
        resolve();
      };
      ws.onmessage = event => {
        try {
          const frame = JSON.parse(event.data) as WireFrame;
          setSocketLastSeen('command');
          if (handleSocketHeartbeat('command', ws, frame)) {
            return;
          }
          applyFrame(frame);
        } catch (error) {
          console.error('[Web Session] Failed to parse command websocket frame', error);
        }
      };
      ws.onerror = event => {
        console.error('[Web Session] command websocket error', event);
      };
      ws.onclose = () => {
        commandSocket = null;
        clearSocketWatchdog('command');
        commandConnectPromise = null;
        if (!settled) {
          reject(new Error('websocket command channel closed before opening'));
          return;
        }
        rejectPendingCommands(new Error('websocket command channel closed'));
      };
    });
    return commandConnectPromise.catch(error => {
      commandConnectPromise = null;
      throw error;
    });
  }

  async function sendCommand(op: string, sessionId: string, payload: Record<string, unknown> = {}) {
    await openCommandSocket();
    if (!commandSocket || commandSocket.readyState !== WebSocket.OPEN) {
      throw new Error('websocket command channel is not connected');
    }
    const requestId = `ws_${Date.now()}_${Math.random().toString(36).slice(2, 8)}`;
    const frame = {
      v: 1,
      k: 'cmd',
      rid: requestId,
      sid: sessionId || undefined,
      op,
      p: payload,
    };
    const promise = new Promise<WireFrame>((resolve, reject) => {
      pending.set(requestId, { resolve, reject });
    });
    commandSocket.send(JSON.stringify(frame));
    return promise;
  }

  async function runRuntimeMutationCommand(
    sessionId: string,
    op: string,
    payload: Record<string, unknown>,
    hydration: RuntimeMutationHydrationOptions
  ) {
    await sendCommand(op, sessionId, payload);
    await hydrateRuntimeMutation(sessionId, hydration);
  }

  async function loadSessions(projectId: string, force = false) {
    if (!projectId) {
      return [];
    }
    if (!force && loadedProjects.value[projectId]) {
      return sessionsByProject.value[projectId] ?? [];
    }
    const sessions = await webSessionApi.list(projectId);
    sessionsByProject.value = {
      ...sessionsByProject.value,
      [projectId]: sortSessions(sessions),
    };
    syncSessionCount(projectId);
    loadedProjects.value = {
      ...loadedProjects.value,
      [projectId]: true,
    };
    if (!hasStoredActiveSession(projectId) && sessions[0]?.id) {
      rememberActiveSession(projectId, sessions[0].id);
    }
    return sessions;
  }

  async function loadSessionCounts() {
    try {
      const counts = await webSessionApi.counts();
      cachedCounts.clear();
      Object.entries(counts).forEach(([projectId, count]) => {
        cachedCounts.set(projectId, Math.max(0, Number(count) || 0));
      });
      return counts;
    } catch (error) {
      console.error('Failed to load web session counts', error);
      return {};
    }
  }

  function invalidateArchivedSessions() {
    archivedScopeStates.value = {};
  }

  async function loadArchivedSessions(
    projectIds: string[],
    options?: {
      reset?: boolean;
      limit?: number;
    }
  ) {
    const scope = normalizeProjectScope(projectIds);
    if (!scope.key) {
      invalidateArchivedSessions();
      return [];
    }

    const limit = Math.max(1, options?.limit ?? 20);
    const previousScopeState = getArchivedScopeState(scope);
    const previousMeta = previousScopeState?.meta ?? defaultArchivedListMeta(scope.key);
    const reset = options?.reset === true || !previousScopeState;
    const offset = reset ? 0 : previousMeta.offset;

    archivedScopeStates.value = {
      ...archivedScopeStates.value,
      [scope.key]: {
        projectIds: [...scope.ids],
        sessionIds: [...(previousScopeState?.sessionIds ?? [])],
        meta: {
          scopeKey: scope.key,
          total: reset ? 0 : previousMeta.total,
          offset,
          hasMore: reset ? false : previousMeta.hasMore,
          loading: true,
        },
      },
    };

    try {
      const result = await webSessionApi.queryArchived({
        projectIds: scope.ids,
        offset,
        limit,
      });
      result.items.forEach(item => {
        upsertArchivedSession(item);
      });
      const nextSessionIds = sortArchivedSessionIds(
        reset
          ? result.items.map(item => item.id)
          : Array.from(
              new Set([
                ...(previousScopeState?.sessionIds ?? []),
                ...result.items.map(item => item.id),
              ])
            )
      );
      archivedScopeStates.value = {
        ...archivedScopeStates.value,
        [scope.key]: {
          projectIds: [...scope.ids],
          sessionIds: nextSessionIds,
          meta: {
            scopeKey: scope.key,
            total: result.total,
            offset: result.nextOffset,
            hasMore: result.hasMore,
            loading: false,
          },
        },
      };
      return getArchivedSessions(scope.ids);
    } catch (error) {
      archivedScopeStates.value = {
        ...archivedScopeStates.value,
        [scope.key]: {
          projectIds: [...scope.ids],
          sessionIds: [...(previousScopeState?.sessionIds ?? [])],
          meta: {
            scopeKey: scope.key,
            total: reset ? 0 : previousMeta.total,
            offset,
            hasMore: reset ? false : previousMeta.hasMore,
            loading: false,
          },
        },
      };
      throw error;
    }
  }

  async function loadSessionSnapshot(
    projectId: string,
    sessionId: string,
    options?: LoadSessionSnapshotOptions
  ) {
    if (!projectId || !sessionId) {
      return null;
    }
    setHistoryLoading(sessionId, true);
    try {
      const snapshot = options?.signal
        ? await webSessionApi.snapshot(projectId, sessionId, {
            signal: options.signal,
          })
        : await webSessionApi.snapshot(projectId, sessionId);
      if (snapshot?.session) {
        applySessionSnapshot(
          sessionId,
          snapshot.session,
          Array.isArray(snapshot.history?.items)
            ? snapshot.history.items.map(item => normalizeHistoryItem(item as WireHistoryItem))
            : [],
          Array.isArray(snapshot.pendingInputs)
            ? snapshot.pendingInputs
                .map(item => normalizePendingInput(item))
                .filter((item): item is WebSessionPendingInput => item != null)
            : [],
          Array.isArray(snapshot.scheduledInputs)
            ? snapshot.scheduledInputs
                .map(item => normalizeScheduledInput(item))
                .filter((item): item is WebSessionScheduledInput => item != null)
            : [],
          {
            hasMore: Boolean(snapshot.history?.hasMore),
            beforeCursor: String(snapshot.history?.beforeCursor ?? ''),
            total: Number(snapshot.history?.total ?? 0),
          },
          {
            preserveArchivedPosition: options?.preserveArchivedPosition === true,
          }
        );
      } else {
        setHistoryLoading(sessionId, false);
      }
      if (options?.rememberActive !== false) {
        rememberActiveSession(projectId, sessionId);
      }
      return snapshot;
    } catch (error) {
      setHistoryLoading(sessionId, false);
      throw error;
    }
  }

  async function renameSession(projectId: string, sessionId: string, title: string) {
    await sendCommand('rename', sessionId, { ttl: title });
    rememberActiveSession(projectId, sessionId);
  }

  async function archiveSession(projectId: string, sessionId: string) {
    const summary = await webSessionApi.archive(projectId, sessionId);
    removeCurrentSessionRecord(projectId, sessionId);
    setPendingInputs(sessionId, []);
    setScheduledInputs(sessionId, []);
    upsertArchivedSession(summary, { includeInMatchingScopes: true });
    return summary;
  }

  async function unarchiveSession(projectId: string, sessionId: string) {
    const summary = await webSessionApi.unarchive(projectId, sessionId);
    removeArchivedSessionRecord(sessionId);
    upsertCurrentSession(summary);
    return summary;
  }

  async function importSession(
    projectId: string,
    sessionId: string,
    mode?: 'fast' | 'deep'
  ): Promise<WebSessionImportResult> {
    const result = await webSessionApi.importSession(projectId, {
      sessionId,
      mode,
    });
    if (result?.session) {
      applySessionSnapshot(
        result.session.id,
        result.session,
        Array.isArray(result.history?.items)
          ? result.history.items.map(item => normalizeHistoryItem(item as WireHistoryItem))
          : [],
        Array.isArray(result.pendingInputs)
          ? result.pendingInputs
              .map(item => normalizePendingInput(item))
              .filter((item): item is WebSessionPendingInput => item != null)
          : [],
        Array.isArray(result.scheduledInputs)
          ? result.scheduledInputs
              .map(item => normalizeScheduledInput(item))
              .filter((item): item is WebSessionScheduledInput => item != null)
          : [],
        {
          hasMore: Boolean(result.history?.hasMore),
          beforeCursor: String(result.history?.beforeCursor ?? ''),
          total: Number(result.history?.total ?? 0),
        }
      );
      rememberActiveSession(projectId, result.session.id);
    }
    return result;
  }

  async function syncSession(
    projectId: string,
    sessionId: string,
    mode?: 'fast' | 'deep',
    clearExisting = false,
    options?: SyncSessionOptions
  ) {
    const session = findSessionById(sessionId);
    const rememberActive = options?.rememberActive ?? !session?.archivedAt;
    updateSessionStatus(sessionId, current => ({
      ...current,
      syncState: 'syncing',
      syncError: null,
      updatedAt: new Date().toISOString(),
    }));
    setHistoryLoading(sessionId, true);
    try {
      const snapshot = await webSessionApi.sync(projectId, sessionId, mode, clearExisting);
      if (snapshot?.session) {
        applySessionSnapshot(
          sessionId,
          snapshot.session,
          Array.isArray(snapshot?.history?.items)
            ? snapshot.history.items.map(item => normalizeHistoryItem(item as WireHistoryItem))
            : [],
          Array.isArray(snapshot.pendingInputs)
            ? snapshot.pendingInputs
                .map(item => normalizePendingInput(item))
                .filter((item): item is WebSessionPendingInput => item != null)
            : [],
          Array.isArray(snapshot.scheduledInputs)
            ? snapshot.scheduledInputs
                .map(item => normalizeScheduledInput(item))
                .filter((item): item is WebSessionScheduledInput => item != null)
            : [],
          {
            hasMore: Boolean(snapshot?.history?.hasMore),
            beforeCursor: String(snapshot?.history?.beforeCursor ?? ''),
            total: Number(snapshot?.history?.total ?? 0),
          }
        );
      }
      if (rememberActive) {
        rememberActiveSession(projectId, sessionId);
      }
      return snapshot;
    } catch (error) {
      setHistoryLoading(sessionId, false);
      updateSessionStatus(sessionId, current => ({
        ...current,
        syncState: 'error',
        syncError: error instanceof Error ? error.message : String(error),
        updatedAt: new Date().toISOString(),
      }));
      throw error;
    }
  }

  async function deleteSession(projectId: string, sessionId: string) {
    await webSessionApi.delete(projectId, sessionId);
    removeSession(projectId, sessionId);
  }

  async function sendMessage(
    sessionId: string,
    text: string,
    attachmentIds: string[],
    mode?: 'redirect' | 'queue'
  ) {
    const session = findSessionById(sessionId);
    if (session?.archivedAt) {
      throw new Error('session is archived');
    }
    const beforeState = snapshotRuntimeMutationState(sessionId);
    let optimisticPendingId = '';
    if (session?.status === 'running' && mode) {
      optimisticPendingId = `pending_${Date.now()}_${Math.random().toString(36).slice(2, 8)}`;
      setPendingInputs(
        sessionId,
        insertPendingInput(getPendingInputs(sessionId), {
          id: optimisticPendingId,
          mode,
          text,
          attachmentIds: [...attachmentIds],
          createdAt: Date.now(),
        })
      );
    }

    try {
      await runRuntimeMutationCommand(
        sessionId,
        'send',
        {
          txt: text,
          atts: attachmentIds,
          ...(mode ? { mode, pid: optimisticPendingId } : {}),
        },
        {
          label: 'send',
          predicate: () => {
            if (
              optimisticPendingId &&
              getPendingInputs(sessionId).some(item => item.id === optimisticPendingId)
            ) {
              return true;
            }
            const liveState = getLiveState(sessionId);
            if (buildBlocks(sessionId).length > beforeState.blockCount) {
              return true;
            }
            if (getHistoryMeta(sessionId).total > beforeState.historyTotal) {
              return true;
            }
            if (getPendingInputs(sessionId).length > beforeState.pendingInputCount) {
              return true;
            }
            if (!beforeState.liveRunning && liveState.running) {
              return true;
            }
            if (
              liveState.phase !== beforeState.livePhase &&
              liveState.updatedAt > beforeState.liveUpdatedAt
            ) {
              return true;
            }
            return liveState.updatedAt > beforeState.liveUpdatedAt && liveState.phase !== 'idle';
          },
        }
      );
    } catch (error) {
      if (optimisticPendingId) {
        setPendingInputs(
          sessionId,
          getPendingInputs(sessionId).filter(item => item.id !== optimisticPendingId)
        );
      }
      throw error;
    }
  }

  async function removePendingInput(sessionId: string, pendingId: string) {
    await runRuntimeMutationCommand(
      sessionId,
      'pending_del',
      { id: pendingId },
      {
        label: 'pending_del',
        predicate: () => !getPendingInputs(sessionId).some(item => item.id === pendingId),
      }
    );
  }

  async function scheduleMessage(
    sessionId: string,
    text: string,
    attachmentIds: string[],
    scheduledFor: number,
    mode: 'send' | 'interrupt' | 'queue' = 'send'
  ) {
    const session = findSessionById(sessionId);
    if (session?.archivedAt) {
      throw new Error('session is archived');
    }
    const frame = await sendCommand('schedule_send', sessionId, {
      txt: text,
      atts: attachmentIds,
      mode,
      at: scheduledFor,
    });
    const payload = asRecord(frame.p);
    const created = normalizeScheduledInput({
      id: typeof payload?.id === 'string' ? payload.id : '',
      mode: typeof payload?.m === 'string' ? payload.m : '',
      status: typeof payload?.st === 'string' ? payload.st : '',
      text: typeof payload?.txt === 'string' ? payload.txt : text,
      attachmentIds: Array.isArray(payload?.atts)
        ? payload.atts.filter((value): value is string => typeof value === 'string')
        : attachmentIds,
      scheduledFor: typeof payload?.sf === 'number' ? payload.sf : scheduledFor,
      createdAt:
        typeof payload?.ca === 'number' || typeof payload?.ca === 'string' ? payload.ca : null,
      updatedAt:
        typeof payload?.ua === 'number' || typeof payload?.ua === 'string' ? payload.ua : null,
      sentAt:
        typeof payload?.sa === 'number' || typeof payload?.sa === 'string' ? payload.sa : null,
      canceledAt:
        typeof payload?.xa === 'number' || typeof payload?.xa === 'string' ? payload.xa : null,
    });
    if (created) {
      setScheduledInputs(
        sessionId,
        sortScheduledInputs([
          ...getScheduledInputs(sessionId).filter(item => item.id !== created.id),
          created,
        ])
      );
    }
    return created;
  }

  async function removeScheduledInput(sessionId: string, inputId: string) {
    await sendCommand('scheduled_del', sessionId, { id: inputId });
    setScheduledInputs(
      sessionId,
      getScheduledInputs(sessionId).filter(item => item.id !== inputId)
    );
  }

  async function abortSession(sessionId: string) {
    await runRuntimeMutationCommand(
      sessionId,
      'abort',
      {},
      {
        label: 'abort',
        timeoutMs: WEB_SESSION_RUNTIME_ABORT_TIMEOUT_MS,
        predicate: () => !getLiveState(sessionId).running,
      }
    );
  }

  async function approveSession(sessionId: string) {
    const beforeState = snapshotRuntimeMutationState(sessionId);
    await runRuntimeMutationCommand(
      sessionId,
      'approve',
      {},
      {
        label: 'approve',
        predicate: () => {
          const liveState = getLiveState(sessionId);
          if (beforeState.approvalId && !getPendingApproval(sessionId)) {
            return true;
          }
          if (
            liveState.phase !== beforeState.livePhase &&
            liveState.updatedAt > beforeState.liveUpdatedAt
          ) {
            return true;
          }
          return liveState.running && liveState.phase !== 'waiting_approval';
        },
      }
    );
  }

  async function rejectSession(sessionId: string) {
    const beforeState = snapshotRuntimeMutationState(sessionId);
    await runRuntimeMutationCommand(
      sessionId,
      'reject',
      {},
      {
        label: 'reject',
        predicate: () => {
          const liveState = getLiveState(sessionId);
          if (beforeState.approvalId && !getPendingApproval(sessionId)) {
            return true;
          }
          if (
            liveState.phase !== beforeState.livePhase &&
            liveState.updatedAt > beforeState.liveUpdatedAt
          ) {
            return true;
          }
          return !liveState.running && liveState.phase !== 'waiting_approval';
        },
      }
    );
  }

  async function answerUserInput(
    sessionId: string,
    itemId: string,
    answers: Record<string, string[]>
  ) {
    const beforeState = snapshotRuntimeMutationState(sessionId);
    await runRuntimeMutationCommand(
      sessionId,
      'user_input',
      { iid: itemId, ans: answers },
      {
        label: 'user_input',
        predicate: () => {
          const liveState = getLiveState(sessionId);
          const pendingUserInput = getPendingUserInput(sessionId);
          if (
            beforeState.userInputId &&
            (!pendingUserInput || pendingUserInput.itemId !== beforeState.userInputId)
          ) {
            return true;
          }
          if (
            liveState.phase !== beforeState.livePhase &&
            liveState.updatedAt > beforeState.liveUpdatedAt
          ) {
            return true;
          }
          return liveState.running && liveState.phase !== 'waiting_input';
        },
      }
    );
  }

  async function loadMoreHistory(sessionId: string, limit = 80) {
    const meta = getHistoryMeta(sessionId);
    if (meta.loading || !meta.hasMore || !meta.beforeCursor) {
      return;
    }
    const session = findSessionById(sessionId);
    if (!session) {
      return;
    }
    historyBySession.value = {
      ...historyBySession.value,
      [sessionId]: {
        ...meta,
        loading: true,
      },
    };
    try {
      const history = await webSessionApi.history(session.projectId, sessionId, {
        beforeCursor: meta.beforeCursor,
        limit,
      });
      const historicalItems = Array.isArray(history.items)
        ? history.items.map(item => normalizeHistoryItem(item as WireHistoryItem))
        : [];
      mergeEvents(sessionId, historicalItems);
      historyBySession.value = {
        ...historyBySession.value,
        [sessionId]: {
          ...getHistoryMeta(sessionId),
          hasMore: Boolean(history.hasMore),
          beforeCursor: String(history.beforeCursor ?? ''),
          total: Number(history.total ?? getHistoryMeta(sessionId).total),
          loading: false,
        },
      };
    } catch (error) {
      historyBySession.value = {
        ...historyBySession.value,
        [sessionId]: {
          ...meta,
          loading: false,
        },
      };
      throw error;
    }
  }

  async function updateModel(sessionId: string, model: string) {
    await sendCommand('set_md', sessionId, { md: model });
  }

  async function updateClaudeRuntime(sessionId: string, claudeRuntime: 'claude' | 'ccr') {
    await sendCommand('set_cr', sessionId, { cr: claudeRuntime });
  }

  async function updateReasoningEffort(
    sessionId: string,
    reasoningEffort: 'default' | 'none' | 'low' | 'medium' | 'high' | 'xhigh'
  ) {
    await sendCommand('set_re', sessionId, { re: reasoningEffort });
  }

  async function updateWorkflowMode(sessionId: string, workflowMode: 'default' | 'plan') {
    const session = findSessionById(sessionId);
    const previousWorkflowMode = session?.workflowMode;
    const shouldOptimisticallyUpdate = Boolean(session) && previousWorkflowMode !== workflowMode;

    if (shouldOptimisticallyUpdate) {
      updateSessionStatus(sessionId, current => ({
        ...current,
        workflowMode,
      }));
    }

    try {
      await sendCommand('set_wm', sessionId, { wm: workflowMode });
    } catch (error) {
      if (shouldOptimisticallyUpdate && previousWorkflowMode) {
        updateSessionStatus(sessionId, current => ({
          ...current,
          workflowMode: previousWorkflowMode,
        }));
      }
      throw error;
    }
  }

  async function updatePermissionLevel(
    sessionId: string,
    permissionLevel: 'default' | 'elevated' | 'yolo'
  ) {
    await sendCommand('set_pl', sessionId, { pl: permissionLevel });
  }

  async function updateAgent(sessionId: string, agent: 'claude' | 'codex') {
    await sendCommand('set_ag', sessionId, { ag: agent });
  }

  async function updateActiveCallTimeout(sessionId: string, enabled: boolean) {
    const session = findSessionById(sessionId);
    const optimisticUpdatedAt = new Date().toISOString();
    const previous =
      session && !session.archivedAt
        ? {
            enabled: session.activeCallTimeoutEnabled === true,
          }
        : null;
    if (previous) {
      setPendingActiveCallTimeoutOverride(
        sessionId,
        enabled === true,
        Date.parse(optimisticUpdatedAt)
      );
      updateSessionStatus(sessionId, current => ({
        ...current,
        activeCallTimeoutEnabled: enabled === true,
        updatedAt: optimisticUpdatedAt,
      }));
    }
    try {
      await sendCommand('set_act', sessionId, {
        acte: enabled === true,
      });
    } catch (error) {
      clearPendingActiveCallTimeoutOverride(sessionId);
      if (previous) {
        updateSessionStatus(sessionId, current => ({
          ...current,
          activeCallTimeoutEnabled: previous.enabled,
          updatedAt: optimisticUpdatedAt,
        }));
      }
      throw error;
    }
  }

  async function updateAutoRetry(
    sessionId: string,
    config: {
      enabled: boolean;
      scope: 'network_only' | 'network_and_rate_limit' | 'all_failures';
      preset: 'gentle_stop' | 'aggressive_stop' | 'sustain_60s';
    }
  ) {
    const session = findSessionById(sessionId);
    const optimisticUpdatedAt = new Date().toISOString();
    const previous =
      session && !session.archivedAt
        ? {
            enabled: session.autoRetryEnabled,
            scope: session.autoRetryScope,
            preset: session.autoRetryPreset,
          }
        : null;
    if (previous) {
      setPendingAutoRetryOverride(sessionId, config, Date.parse(optimisticUpdatedAt));
      updateSessionStatus(sessionId, current => ({
        ...current,
        autoRetryEnabled: config.enabled === true,
        autoRetryScope: config.scope,
        autoRetryPreset: config.preset,
        updatedAt: optimisticUpdatedAt,
      }));
    }
    try {
      await sendCommand('set_ar', sessionId, {
        ae: config.enabled === true,
        ars: config.scope,
        arp: config.preset,
      });
    } catch (error) {
      clearPendingAutoRetryOverride(sessionId);
      if (previous) {
        updateSessionStatus(sessionId, current => ({
          ...current,
          autoRetryEnabled: previous.enabled,
          autoRetryScope: previous.scope,
          autoRetryPreset: previous.preset,
          updatedAt: optimisticUpdatedAt,
        }));
      }
      throw error;
    }
  }

  async function moveSession(
    projectId: string,
    sessionId: string,
    previousSessionId = '',
    nextSessionId = ''
  ) {
    const current = getSessions(projectId);
    if (
      !projectId ||
      !sessionId ||
      (previousSessionId && previousSessionId === sessionId) ||
      (nextSessionId && nextSessionId === sessionId)
    ) {
      return;
    }

    const original = [...current];
    const reordered = current.filter(session => session.id !== sessionId);
    const moving = current.find(session => session.id === sessionId);
    if (!moving) {
      return;
    }

    let insertIndex = reordered.length;
    if (previousSessionId) {
      const previousIndex = reordered.findIndex(session => session.id === previousSessionId);
      insertIndex = previousIndex >= 0 ? previousIndex + 1 : reordered.length;
    } else if (nextSessionId) {
      const nextIndex = reordered.findIndex(session => session.id === nextSessionId);
      insertIndex = nextIndex >= 0 ? nextIndex : 0;
    }

    reordered.splice(insertIndex, 0, moving);
    const reorderedWithOrder = reordered.map((session, index) => ({
      ...session,
      orderIndex: (index + 1) * 1000,
    }));
    sessionsByProject.value = {
      ...sessionsByProject.value,
      [projectId]: reorderedWithOrder,
    };

    try {
      await sendCommand('move', moving.id, {
        prv: previousSessionId,
        nxt: nextSessionId,
      });
    } catch (error) {
      sessionsByProject.value = {
        ...sessionsByProject.value,
        [projectId]: sortSessions(original),
      };
      await loadSessions(projectId, true);
      throw error;
    }
  }

  async function uploadAttachment(projectId: string, sessionId: string, file: File) {
    const result = await uploadAttachments(projectId, sessionId, [file]);
    if (result.errors.length > 0) {
      throw new Error(result.errors[0]?.message || 'failed to upload attachment');
    }
    const attachment = result.attachments[0];
    if (!attachment) {
      throw new Error('failed to upload attachment');
    }
    return attachment;
  }

  function removeDraftAttachment(projectId: string, sessionId: string, attachmentId: string) {
    updateDraft(projectId, sessionId, draft => ({
      ...draft,
      attachments: draft.attachments.filter(item => item.id !== attachmentId),
      updatedAt: Date.now(),
    }));
  }

  async function createSessionViaHttp(
    projectId: string,
    payload: {
      worktreeId?: string;
      agent: 'claude' | 'codex';
      claudeRuntime?: 'claude' | 'ccr';
      model?: string;
      reasoningEffort?: 'default' | 'none' | 'low' | 'medium' | 'high' | 'xhigh';
      workflowMode?: 'default' | 'plan';
      permissionLevel?: 'default' | 'elevated' | 'yolo';
      activeCallTimeoutEnabled?: boolean;
      autoRetryEnabled?: boolean;
      autoRetryScope?: 'network_only' | 'network_and_rate_limit' | 'all_failures';
      autoRetryPreset?: 'gentle_stop' | 'aggressive_stop' | 'sustain_60s';
      title?: string;
    }
  ) {
    const session = await webSessionApi.create(projectId, payload);
    upsertSession(session);
    rememberActiveSession(projectId, session.id);
    emitter.emit('web-session:created', {
      projectId,
      sessionId: session.id,
    });
    return session;
  }

  return {
    connectionState,
    eventLastSeenAt,
    eventLastDisconnectReason,
    eventRecoveryVersion,
    lastError,
    getDraft,
    getSessions,
    getSessionCount,
    getArchivedSessions,
    getArchivedMeta,
    hasArchivedScope,
    getActiveSessionId,
    hasStoredActiveSession,
    getActiveSession,
    getDraftAttachments,
    getDraftAttachmentUpload,
    getPendingUserInputDraft,
    setPendingUserInputDraft,
    clearPendingUserInputDraft,
    setDraftText,
    getPendingInputs,
    getScheduledInputs,
    getHistoryMeta,
    getBlocks,
    getLatestEventSeq,
    loadSessions,
    loadSessionCounts,
    loadArchivedSessions,
    invalidateArchivedSessions,
    setActiveSession,
    loadSessionSnapshot,
    createSession: createSessionViaHttp,
    importSession,
    renameSession,
    archiveSession,
    unarchiveSession,
    syncSession,
    deleteSession,
    sendMessage,
    scheduleMessage,
    abortSession,
    approveSession,
    rejectSession,
    answerUserInput,
    loadMoreHistory,
    trimInactiveSessionEvents,
    updateModel,
    updateClaudeRuntime,
    updateReasoningEffort,
    updateWorkflowMode,
    updatePermissionLevel,
    updateAgent,
    updateActiveCallTimeout,
    updateAutoRetry,
    moveSession,
    getPendingApproval,
    getPendingUserInput,
    getLiveState,
    uploadAttachments,
    uploadAttachment,
    removeDraftAttachment,
    removePendingInput,
    removeScheduledInput,
    clearDraft,
    moveDraft,
    openEventStream,
    setEventSessionFocus,
    sessionCounts: cachedCounts,
    emitter,
  };
});
