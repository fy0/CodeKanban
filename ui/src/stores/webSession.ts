import EventEmitter from 'eventemitter3';
import { defineStore } from 'pinia';
import { computed, reactive, ref } from 'vue';
import {
  webSessionApi,
  type WebSessionAttachmentUploadProgress,
  type WebSessionCommandExecutionGroupDetail,
  type WebSessionImportResult,
  type WebSessionHydrationTarget,
  type WebSessionReconcileResult,
  type WebSessionReconcileTarget,
  type WebSessionSnapshot,
  type WebSessionPendingApprovalRecord,
  type WebSessionSubAgentRecord,
} from '@/api/webSession';
import type {
  WebSessionAgent,
  WebSessionAttachment,
  WebSessionContextWindowSource,
  WebSessionGoal,
  WebSessionReasoningEffort,
  WebSessionSummary,
  WebSessionWorkTimingOutcome,
} from '@/types/models';
import { WebSessionSync } from '@/stores/webSessionSync';
import {
  buildWebSessionCommandFrame,
  buildWebSessionHeartbeatFrame,
  parseWebSessionWireFrame,
  WebSessionCommandError,
  type WebSessionWireFrame,
  type WebSessionWireHeartbeatOperation,
} from '@/stores/webSessionWireProtocol';
import { normalizeWebSessionSyncState } from '@/utils/webSessionSyncState';
import {
  compareWebSessionRevisions,
  normalizeWebSessionRevision,
} from '@/utils/webSessionRevision';
import { buildUploadImageFileName } from '@/utils/webSessionImages';
import { resolveWsUrl } from '@/utils/ws';
import { selectMostRecentWebSession } from '@/utils/webSessionRecency';

type WebSessionSocketKind = 'event' | 'command';
type SessionStatus = WebSessionSummary['status'];
type SessionAssistantState =
  | 'working'
  | 'waiting_approval'
  | 'waiting_input'
  | 'waiting_plan_approval';

type WireSession = {
  id: string;
  rev?: string;
  pid: string;
  wid?: string | null;
  oi?: number;
  ag: WebSessionAgent;
  cr?: 'claude' | 'ccr';
  md: string;
  re?: WebSessionReasoningEffort;
  wm: 'default' | 'plan';
  pl: 'default' | 'elevated' | 'yolo';
  acte?: boolean;
  ae?: boolean;
  arpm?: 'default' | 'custom';
  ars?: 'network_only' | 'network_and_rate_limit' | 'all_failures';
  arp?: 'gentle_stop' | 'aggressive_stop' | 'sustain_60s';
  aram?: number;
  ardpf?: boolean;
  ttl: string;
  cwd: string;
  nsid?: string | null;
  cpf?: boolean;
  spe?: boolean;
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
  goal?: {
    tid?: string;
    obj?: string;
    st?: string;
    tb?: number | null;
    tu?: number;
    tsu?: number;
    ca?: number | null;
    ua?: number | null;
  } | null;
  wt?: {
    dur?: number;
    cur?: {
      id?: string;
      sa?: number | null;
      pa?: number | null;
      pd?: number;
    } | null;
    bs?: 'pending' | 'complete' | 'partial' | 'unavailable' | 'failed' | string;
    bv?: number;
  };
};

type WirePendingInput = {
  id?: string;
  m?: 'redirect' | 'queue' | string;
  txt?: string;
  atts?: string[];
  ra?: number | null;
  ps?: boolean;
  nq?: boolean;
  ca?: number | null;
};

type WireScheduledInput = {
  id?: string;
  a?: 'message' | 'execute_plan' | string;
  tid?: string;
  m?: 'send' | 'interrupt' | 'redirect' | 'queue' | string;
  epm?: boolean;
  st?: 'scheduled' | 'failed' | 'expired' | 'dispatched' | 'canceled' | string;
  err?: string;
  txt?: string;
  atts?: string[];
  sk?: 'at_time' | 'when_idle' | string;
  sf?: number | null;
  is?: number | null;
  br?: string[];
  ce?: string;
  ca?: number | null;
  ua?: number | null;
  sa?: number | null;
  xa?: number | null;
};

type WireHistoryItem = {
  id: string;
  sthid?: string | null;
  stid?: string | null;
  siid?: string | null;
  rid?: string | null;
  dur?: number | null;
  out?: WebSessionWorkTimingOutcome | string;
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
    approvalKind?: string;
    command?: string;
    questions?: WebSessionUserInputQuestion[];
    answers?: WebSessionHistoryAnswerEntry[];
    action?: 'approve' | 'reject' | string;
  } | null;
  pl?: Record<string, unknown>;
};

type WireSubAgent = {
  tid?: string;
  ptid?: string | null;
  p?: string;
  nn?: string;
  rl?: string;
  st?: string;
  sm?: string;
  ctid?: string | null;
  liid?: string | null;
  loi?: number;
  sa?: number | null;
  la?: number | null;
  ea?: number | null;
};

type WireFrame = WebSessionWireFrame<
  WireSession,
  WireHistoryItem,
  WireSubAgent,
  WirePendingInput,
  WireScheduledInput
>;

function parseWireFrame(raw: unknown): WireFrame {
  return parseWebSessionWireFrame<
    WireSession,
    WireHistoryItem,
    WireSubAgent,
    WirePendingInput,
    WireScheduledInput
  >(raw);
}

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

export type WebSessionSubAgentStatus =
  | 'pending_init'
  | 'running'
  | 'idle'
  | 'interrupted'
  | 'completed'
  | 'errored'
  | 'shutdown'
  | 'not_found';

export interface WebSessionSubAgent extends WebSessionLiveSubAgent {
  parentThreadId?: string | null;
  path: string;
  nickname: string;
  role: string;
  status: WebSessionSubAgentStatus;
  currentTurnId?: string | null;
  latestItemId?: string | null;
  latestOrderIndex: number;
  lastActivityAt?: number;
  endedAt?: number;
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
  approvalKind?: string;
  command?: string;
  questions?: WebSessionUserInputQuestion[];
  answers?: WebSessionHistoryAnswerEntry[];
  action?: 'approve' | 'reject' | string;
}

export type WebSessionMessageDeliveryState = 'sending' | 'failed' | 'accepted';

export interface WebSessionBlock {
  key: string;
  id: string;
  sourceThreadId?: string | null;
  sourceTurnId?: string | null;
  sourceItemId?: string | null;
  runId?: string | null;
  runDurationMs?: number | null;
  runOutcome?: WebSessionWorkTimingOutcome | null;
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
  deliveryState?: WebSessionMessageDeliveryState;
}

export interface WebSessionSendMessageOptions {
  outgoingMessageId?: string;
  attachments?: WebSessionBlock['attachments'];
}

export class WebSessionMessageDeliveryError extends Error {
  readonly outgoingMessageId: string;
  readonly originalError: unknown;

  constructor(outgoingMessageId: string, originalError: unknown) {
    super(
      originalError instanceof Error
        ? originalError.message
        : String(originalError || 'message delivery failed')
    );
    this.name = 'WebSessionMessageDeliveryError';
    this.outgoingMessageId = outgoingMessageId;
    this.originalError = originalError;
  }
}

export function isWebSessionMessageDeliveryError(
  error: unknown
): error is WebSessionMessageDeliveryError {
  return error instanceof WebSessionMessageDeliveryError;
}

type WebSessionOutgoingMessage = WebSessionBlock & {
  deliveryState: WebSessionMessageDeliveryState;
  baseOrderIndex: number;
};

export interface WebSessionHistoryPage {
  items: WebSessionBlock[];
  hasMore: boolean;
  beforeCursor: string;
  hasLater: boolean;
  afterCursor: string;
  total: number;
}

export interface WebSessionApprovalState {
  id: string;
  itemId: string;
  kind: string;
  prompt: string;
  command: string;
  requestedAt: number;
  stale: boolean;
  actionable: boolean;
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

function normalizeGoal(goal: WireSession['goal']): WebSessionGoal | null {
  if (!goal || typeof goal !== 'object') {
    return null;
  }
  const objective = typeof goal.obj === 'string' ? goal.obj.trim() : '';
  const threadId = typeof goal.tid === 'string' ? goal.tid.trim() : '';
  const status = typeof goal.st === 'string' ? (goal.st.trim() as WebSessionGoal['status']) : '';
  if (!objective || !threadId || !status) {
    return null;
  }
  return {
    threadId,
    objective,
    status,
    tokenBudget: typeof goal.tb === 'number' && Number.isFinite(goal.tb) ? Number(goal.tb) : null,
    tokensUsed: Number(goal.tu ?? 0),
    timeUsedSeconds: Number(goal.tsu ?? 0),
    createdAt:
      typeof goal.ca === 'number' && Number.isFinite(goal.ca)
        ? new Date(goal.ca).toISOString()
        : new Date().toISOString(),
    updatedAt:
      typeof goal.ua === 'number' && Number.isFinite(goal.ua)
        ? new Date(goal.ua).toISOString()
        : new Date().toISOString(),
  };
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
    remoteUrl?: string;
    attempt?: number;
    maxAttempts?: number;
  };
}

type RuntimeProjection = {
  liveState: WebSessionLiveState;
  pendingApproval: WebSessionApprovalState | null;
  pendingUserInput: WebSessionUserInputState | null;
};

type RuntimeRetryState = NonNullable<WebSessionLiveState['retry']> & {
  updatedAt: number;
};

type RuntimeAccumulator = {
  pendingApproval: WebSessionApprovalState | null;
  pendingUserInput: WebSessionUserInputState | null;
  activeTool: WebSessionLiveState['tool'];
  activeSubAgents: Map<string, WebSessionLiveSubAgent>;
  knownSubAgents: Map<string, WebSessionLiveSubAgent>;
  authoritativeSubAgents: boolean;
  rootThreadId: string;
  sawAssistantOutput: boolean;
  assistantDone: boolean;
  firstAssistantOutputAt?: number;
  errorMessage: string;
  updatedAt: number;
  runStartedAt?: number;
  activeRunId?: string;
  runActive: boolean;
  retryState?: RuntimeRetryState;
  latestProgressTimestamp?: number;
};

type RuntimeProjectionCacheEntry = {
  blocks: WebSessionBlock[];
  session: WebSessionSummary;
  projection: RuntimeProjection;
  accumulator: RuntimeAccumulator;
  beforeLastAccumulator: RuntimeAccumulator;
};

const webSessionRuntimePerformanceCounters = {
  fullDerivations: 0,
  incrementalDerivations: 0,
  scannedBlocks: 0,
  eventSorts: 0,
};

export const webSessionRuntimePerformance = {
  reset() {
    webSessionRuntimePerformanceCounters.fullDerivations = 0;
    webSessionRuntimePerformanceCounters.incrementalDerivations = 0;
    webSessionRuntimePerformanceCounters.scannedBlocks = 0;
    webSessionRuntimePerformanceCounters.eventSorts = 0;
  },
  snapshot() {
    return { ...webSessionRuntimePerformanceCounters };
  },
};

export interface WebSessionPendingInput {
  id: string;
  mode: 'redirect' | 'queue';
  text: string;
  attachmentIds: string[];
  readyAt: number | null;
  paused: boolean;
  nativeQueued?: boolean;
  createdAt: number;
}

export const WEB_SESSION_NATIVE_STEER_UNDO_WINDOW_MS = 5_000;

type WebSessionPendingInputMode = WebSessionPendingInput['mode'];

export interface WebSessionScheduledInput {
  id: string;
  action: 'message' | 'execute_plan';
  targetId: string;
  mode: 'send' | 'interrupt' | 'queue';
  exitPlanMode: boolean;
  status: 'scheduled' | 'failed' | 'expired';
  lastError: string;
  text: string;
  attachmentIds: string[];
  scheduleKind: 'at_time' | 'when_idle';
  scheduledFor: number | null;
  idleSince: number | null;
  blockingReasons: Array<'git_dirty' | 'git_unavailable' | 'non_plan_session_active'>;
  conditionError: string;
  createdAt: number;
  updatedAt: number;
  sentAt: number | null;
  canceledAt: number | null;
}

export interface WebSessionPlanExecutionTarget {
  planItemId: string;
  pendingItemId?: string;
  questionId?: string;
  executeOptionLabel?: string;
}

export type WebSessionSchedule =
  | { scheduleKind: 'at_time'; scheduledFor: number }
  | { scheduleKind: 'when_idle'; scheduledFor?: null };

export type WebSessionPlanSchedule = WebSessionSchedule;

type RuntimeMutationStateSnapshot = {
  blockCount: number;
  historyTotal: number;
  pendingInputCount: number;
  pendingInputVersion: number;
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
  forceSnapshot?: boolean;
  predicate: () => boolean;
};

export interface WebSessionDraftState {
  text: string;
  attachments: WebSessionAttachment[];
  updatedAt: number;
}

export function mergeWebSessionDraftForRestore(
  current: WebSessionDraftState,
  submitted: WebSessionDraftState,
  restoredAt = Date.now()
): WebSessionDraftState {
  const submittedText = String(submitted.text ?? '');
  const currentText = String(current.text ?? '');
  const text =
    currentText.trim().length === 0
      ? submittedText
      : submittedText.trim().length === 0 || currentText === submittedText
        ? currentText
        : `${submittedText}\n\n${currentText}`;
  const attachmentIds = new Set<string>();
  const attachments = [...submitted.attachments, ...current.attachments].filter(attachment => {
    if (attachmentIds.has(attachment.id)) {
      return false;
    }
    attachmentIds.add(attachment.id);
    return true;
  });

  return {
    text,
    attachments,
    updatedAt: restoredAt,
  };
}

export interface WebSessionPendingUserInputDraftState {
  selections: Record<string, string[]>;
  drafts: Record<string, string>;
  updatedAt: number;
}

export interface WebSessionPendingInputEditDraft {
  text: string;
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

type CreateSessionOptions = {
  rememberActive?: boolean;
};

type LoadSessionSnapshotOptions = {
  rememberActive?: boolean;
  signal?: AbortSignal;
  preserveArchivedPosition?: boolean;
  conditional?: boolean;
  limit?: number;
  skipTrailing?: boolean;
};

type PendingAutoRetryOverride = {
  enabled: boolean;
  policyMode: WebSessionSummary['autoRetryPolicyMode'];
  scope: WebSessionSummary['autoRetryScope'];
  preset: WebSessionSummary['autoRetryPreset'];
  maxAttempts: number;
  appliedAt: number;
  ackedAt?: number;
};

type PendingAutoRetryDispatchOverride = {
  enabled: boolean;
  appliedAt: number;
  ackedAt?: number;
};

type PendingActiveCallTimeoutOverride = {
  enabled: boolean;
  appliedAt: number;
  expiresAt: number;
};

const ACTIVE_SESSION_STORAGE_KEY = 'kanban-web-active-session';
const SESSION_DRAFT_STORAGE_KEY = 'kanban-web-session-drafts';
const PENDING_INPUT_EDIT_DRAFT_STORAGE_KEY = 'kanban-web-session-pending-input-edits';
const COMMAND_WS_PATH = '/api/v1/web-sessions/ws';
const EVENTS_WS_PATH = '/api/v1/web-sessions/events';
const WEB_SESSION_HEARTBEAT_INTERVAL_MS = 15000;
const WEB_SESSION_SOCKET_IDLE_TIMEOUT_MS = WEB_SESSION_HEARTBEAT_INTERVAL_MS * 2 + 5000;
const WEB_SESSION_SOCKET_WATCHDOG_INTERVAL_MS = 5000;
const WEB_SESSION_EVENT_RECONNECT_BASE_DELAY_MS = 1200;
const WEB_SESSION_EVENT_RECONNECT_MAX_DELAY_MS = 15000;
const WEB_SESSION_RECONCILE_RECENT_WINDOW_MS = 6 * 60 * 60 * 1000;
const WEB_SESSION_RECONCILE_RECENT_LIMIT = 48;
const WEB_SESSION_RECONCILE_MAX_TARGETS = 256;
const WEB_SESSION_RECONCILE_MIN_INTERVAL_MS = 1000;
const WEB_SESSION_AUTO_RETRY_OPTIMISTIC_TTL_MS = 5000;
const WEB_SESSION_RUNTIME_MUTATION_PASSIVE_WAIT_MS = 150;
const WEB_SESSION_RUNTIME_MUTATION_PASSIVE_POLL_MS = 16;
const WEB_SESSION_RUNTIME_MUTATION_TIMEOUT_MS = 2500;
const WEB_SESSION_RUNTIME_ABORT_TIMEOUT_MS = 5000;
const WEB_SESSION_MAX_RETAINED_BLOCKS = 400;
const WEB_SESSION_MIN_RETAINED_BLOCKS = 160;
const PROCESS_RESTART_REASON = 'process_restart';
const DEFAULT_RECOVERY_MESSAGE =
  'The previous run was interrupted because the app restarted. Send a new message to continue.';

function normalizeAutoRetryMaxAttempts(value: unknown) {
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) {
    return 0;
  }
  return Math.min(Math.max(Math.round(parsed), 0), 100);
}

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

function normalizeStoredPendingInputEditDrafts(
  value: unknown
): Record<string, Record<string, Record<string, WebSessionPendingInputEditDraft>>> {
  if (!isRecord(value)) {
    return {};
  }
  const result: Record<
    string,
    Record<string, Record<string, WebSessionPendingInputEditDraft>>
  > = {};
  Object.entries(value).forEach(([projectId, projectValue]) => {
    if (!projectId.trim() || !isRecord(projectValue)) {
      return;
    }
    const projectDrafts: Record<string, Record<string, WebSessionPendingInputEditDraft>> = {};
    Object.entries(projectValue).forEach(([sessionId, sessionValue]) => {
      if (!sessionId.trim() || !isRecord(sessionValue)) {
        return;
      }
      const sessionDrafts: Record<string, WebSessionPendingInputEditDraft> = {};
      Object.entries(sessionValue).forEach(([pendingId, draftValue]) => {
        if (!pendingId.trim() || !isRecord(draftValue) || typeof draftValue.text !== 'string') {
          return;
        }
        sessionDrafts[pendingId] = {
          text: draftValue.text,
          updatedAt:
            typeof draftValue.updatedAt === 'number' && Number.isFinite(draftValue.updatedAt)
              ? draftValue.updatedAt
              : Date.now(),
        };
      });
      if (Object.keys(sessionDrafts).length > 0) {
        projectDrafts[sessionId] = sessionDrafts;
      }
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

function loadStoredPendingInputEditDrafts() {
  try {
    const raw = localStorage.getItem(PENDING_INPUT_EDIT_DRAFT_STORAGE_KEY);
    if (!raw) {
      return {};
    }
    return normalizeStoredPendingInputEditDrafts(JSON.parse(raw));
  } catch {
    return {};
  }
}

function persistPendingInputEditDrafts(
  value: Record<string, Record<string, Record<string, WebSessionPendingInputEditDraft>>>
) {
  try {
    const persisted = normalizeStoredPendingInputEditDrafts(value);
    if (Object.keys(persisted).length === 0) {
      localStorage.removeItem(PENDING_INPUT_EDIT_DRAFT_STORAGE_KEY);
      return;
    }
    localStorage.setItem(PENDING_INPUT_EDIT_DRAFT_STORAGE_KEY, JSON.stringify(persisted));
  } catch (error) {
    console.warn('[Web Session] Failed to persist pending input edit drafts', error);
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

function normalizeSubAgentStatus(value: unknown): WebSessionSubAgentStatus {
  const normalized = String(value ?? '')
    .trim()
    .toLowerCase()
    .replace(/[-_]/g, '');
  switch (normalized) {
    case 'running':
    case 'active':
      return 'running';
    case 'idle':
    case 'notloaded':
      return 'idle';
    case 'interrupted':
    case 'aborted':
      return 'interrupted';
    case 'completed':
    case 'done':
      return 'completed';
    case 'errored':
    case 'error':
    case 'failed':
    case 'systemerror':
      return 'errored';
    case 'shutdown':
    case 'closed':
      return 'shutdown';
    case 'notfound':
      return 'not_found';
    default:
      return 'pending_init';
  }
}

function subAgentDisplayTitle(input: {
  nickname?: string;
  role?: string;
  path?: string;
  id: string;
}) {
  if (input.nickname && input.role) {
    return `${input.nickname} [${input.role}]`;
  }
  if (input.nickname) {
    return input.nickname;
  }
  const pathParts = String(input.path ?? '')
    .split('/')
    .map(part => part.trim())
    .filter(Boolean);
  const pathName = pathParts[pathParts.length - 1];
  if (pathName) {
    return pathName;
  }
  if (input.role) {
    return `[${input.role}]`;
  }
  return input.id.slice(0, 12) || 'Sub Agent';
}

function normalizeSubAgent(
  value: WebSessionSubAgentRecord | WireSubAgent | null | undefined
): WebSessionSubAgent | null {
  const record = asRecord(value);
  if (!record) {
    return null;
  }
  const id = String(record.tid ?? record.threadId ?? '').trim();
  if (!id) {
    return null;
  }
  const nickname = String(record.nn ?? record.nickname ?? '').trim();
  const role = String(record.rl ?? record.role ?? '').trim();
  const path = String(record.p ?? record.path ?? '').trim();
  const startedAt = parseHistoryTimeValue(record.sa ?? record.startedAt) ?? undefined;
  const currentTurnId = String(record.ctid ?? record.currentTurnId ?? '').trim() || null;
  const normalizedStatus = normalizeSubAgentStatus(record.st ?? record.status);
  return {
    id,
    parentThreadId: String(record.ptid ?? record.parentThreadId ?? '').trim() || null,
    path,
    nickname,
    role,
    status: normalizedStatus === 'running' && !currentTurnId ? 'idle' : normalizedStatus,
    title: subAgentDisplayTitle({ id, nickname, role, path }),
    summary: String(record.sm ?? record.summary ?? '').trim(),
    currentTurnId,
    latestItemId: String(record.liid ?? record.latestItemId ?? '').trim() || null,
    latestOrderIndex: Number(record.loi ?? record.latestOrderIndex ?? 0) || 0,
    startedAt,
    lastActivityAt: parseHistoryTimeValue(record.la ?? record.lastActivityAt) ?? undefined,
    endedAt: parseHistoryTimeValue(record.ea ?? record.endedAt) ?? undefined,
  };
}

function isActiveSubAgent(agent: WebSessionSubAgent) {
  return (
    agent.status === 'pending_init' || (agent.status === 'running' && Boolean(agent.currentTurnId))
  );
}

function normalizePendingApproval(
  value: WebSessionPendingApprovalRecord | null | undefined
): WebSessionApprovalState | null {
  const record = asRecord(value);
  if (!record) {
    return null;
  }
  const itemId = String(record.iid ?? record.itemId ?? '').trim();
  if (!itemId) {
    return null;
  }
  return {
    id: itemId,
    itemId,
    kind: String(record.kind ?? '').trim(),
    prompt: String(record.txt ?? record.prompt ?? '').trim(),
    command: String(record.cmd ?? record.command ?? '').trim(),
    requestedAt: parseHistoryTimeValue(record.ra ?? record.requestedAt) ?? Date.now(),
    stale: false,
    actionable: record.act === true || record.actionable === true,
  };
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

  const outputIds = fromValue(output?.receiverThreadIds);
  if (outputIds.length > 0) {
    return outputIds;
  }
  const direct = fromValue(input?.receiverThreadIds);
  if (direct.length > 0) {
    return direct;
  }

  const outputStates = asRecord(output?.agentsStates);
  const inputStates = asRecord(input?.agentsStates);
  const states = outputStates && Object.keys(outputStates).length > 0 ? outputStates : inputStates;
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
  const outputStates = asRecord(output?.agentsStates);
  const inputStates = asRecord(input?.agentsStates);
  const source = outputStates && Object.keys(outputStates).length > 0 ? outputStates : inputStates;
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

  const stateMap = normalizeSubAgentStateMap(input, output);
  if (stateMap.size === 0) {
    return;
  }
  stateMap.forEach((state, id) => {
    if (!state.status) {
      return;
    }
    const normalizedStatus = normalizeSubAgentStatus(state.status);
    if (normalizedStatus === 'pending_init' || normalizedStatus === 'running') {
      const known = registry.get(id);
      const fallback = agents.find(agent => agent.id === id);
      if (known || fallback) {
        active.set(id, known ?? fallback!);
      }
      return;
    }
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
  const remoteUrl =
    typeof payload.remoteUrl === 'string' && payload.remoteUrl.trim()
      ? payload.remoteUrl.trim()
      : undefined;
  return {
    code: 'transport_retrying',
    message: String(payload.txt ?? '').trim(),
    remoteUrl,
    attempt,
    maxAttempts,
  };
}

function getBlockFreshnessTimestamp(block: WebSessionBlock) {
  const freshness = block.observedAt ?? block.timestamp;
  return typeof freshness === 'number' && Number.isFinite(freshness) && freshness > 0
    ? freshness
    : null;
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

function getRetryClearingProgressTimestamp(
  block: WebSessionBlock,
  retryPayload: ReturnType<typeof getTransportRetryPayload>
) {
  if (!isRetryClearingProgressBlock(block, retryPayload)) {
    return null;
  }
  return getBlockFreshnessTimestamp(block);
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
  const outgoingMessagesBySession = ref<Record<string, WebSessionOutgoingMessage[]>>({});
  const historyBySession = ref<Record<string, HistoryMeta>>({});
  const draftStateByProject =
    ref<Record<string, Record<string, WebSessionDraftState>>>(loadStoredSessionDrafts());
  const pendingInputEditDraftStateByProject = ref<
    Record<string, Record<string, Record<string, WebSessionPendingInputEditDraft>>>
  >(loadStoredPendingInputEditDrafts());
  const userInputDraftStateByKey = ref<Record<string, WebSessionPendingUserInputDraftState>>({});
  const draftAttachmentUploadsByProject = ref<
    Record<string, Record<string, WebSessionDraftAttachmentUploadState>>
  >({});
  const pendingInputsBySession = ref<Record<string, WebSessionPendingInput[]>>({});
  const pendingInputVersionBySession = new Map<string, number>();
  const scheduledInputsBySession = ref<Record<string, WebSessionScheduledInput[]>>({});
  const snapshotApprovalsBySession = ref<Record<string, WebSessionApprovalState>>({});
  const subAgentsBySession = ref<Record<string, WebSessionSubAgent[]>>({});
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
      operation: string;
      sessionId: string;
    }
  >();
  const draftAttachmentUploadQueues = new Map<string, Promise<unknown>>();
  const pendingAutoRetryOverrides = reactive(new Map<string, PendingAutoRetryOverride>());
  const pendingAutoRetryDispatchOverrides = reactive(
    new Map<string, PendingAutoRetryDispatchOverride>()
  );
  const pendingActiveCallTimeoutOverrides = reactive(
    new Map<string, PendingActiveCallTimeoutOverride>()
  );
  const sessionSync = new WebSessionSync({
    hydrate: async request => {
      const session = findSessionById(request.sessionId);
      if (!session?.projectId || session.archivedAt) {
        return;
      }
      await loadSessionSnapshot(session.projectId, request.sessionId, {
        rememberActive: false,
        conditional: true,
      });
    },
    onError: (request, error) => {
      console.warn('[Web Session] Failed to hydrate requested session revision', {
        sessionId: request.sessionId,
        revision: request.revision,
        reason: request.reason,
        error,
      });
    },
  });
  const inFlightSessionLists = new Map<string, Promise<WebSessionSummary[]>>();
  let inFlightSessionReconcile: Promise<WebSessionReconcileResult> | null = null;
  let lastSessionReconcileCompletedAt = 0;
  const inFlightWorkTimingCalculations = new Map<
    string,
    ReturnType<typeof webSessionApi.calculateWorkTiming>
  >();
  const inFlightSnapshots = new Map<
    string,
    {
      promise: Promise<WebSessionSnapshot>;
      controller: AbortController;
      consumers: Set<symbol>;
    }
  >();
  const commandGroupDetailsByKey = new Map<string, WebSessionCommandExecutionGroupDetail>();
  const inFlightCommandGroupDetailsByKey = new Map<
    string,
    Promise<WebSessionCommandExecutionGroupDetail>
  >();
  const completedTransitionVersionBySession = new Map<string, number>();
  const currentSessionProjectById = new Map<string, string>();
  const runtimeProjectionCacheBySession = new Map<string, RuntimeProjectionCacheEntry>();
  const eventIndexBySession = new Map<string, Map<string, number>>();
  const emptySessionBlocks: WebSessionBlock[] = [];
  let draftAttachmentUploadSeed = 0;
  let outgoingMessageSeed = 0;

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

  function replaceProjectSessions(projectId: string, sessions: WebSessionSummary[]) {
    const previous = sessionsByProject.value[projectId] ?? [];
    previous.forEach(session => {
      if (currentSessionProjectById.get(session.id) === projectId) {
        currentSessionProjectById.delete(session.id);
      }
    });
    sessions.forEach(session => {
      currentSessionProjectById.set(session.id, projectId);
    });
    sessionsByProject.value = {
      ...sessionsByProject.value,
      [projectId]: sessions,
    };
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
    const indexedProjectId = currentSessionProjectById.get(sessionId);
    if (indexedProjectId) {
      const indexedSession = getSessions(indexedProjectId).find(item => item.id === sessionId);
      if (indexedSession) {
        return indexedSession;
      }
      currentSessionProjectById.delete(sessionId);
    }
    for (const [projectId, sessions] of Object.entries(sessionsByProject.value)) {
      const matched = sessions.find(item => item.id === sessionId);
      if (matched) {
        currentSessionProjectById.set(sessionId, projectId);
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

  function commandGroupDetailCacheKey(sessionId: string, groupId: string) {
    return `${sessionId}\u0000${groupId}`;
  }

  async function loadCommandGroupDetail(
    sessionId: string,
    groupId: string
  ): Promise<WebSessionCommandExecutionGroupDetail> {
    const normalizedSessionId = String(sessionId ?? '').trim();
    const normalizedGroupId = String(groupId ?? '').trim();
    if (!normalizedSessionId || !normalizedGroupId) {
      throw new Error('session and tool group are required');
    }

    const key = commandGroupDetailCacheKey(normalizedSessionId, normalizedGroupId);
    const cached = commandGroupDetailsByKey.get(key);
    if (cached) {
      return cached;
    }

    const inFlight = inFlightCommandGroupDetailsByKey.get(key);
    if (inFlight) {
      return inFlight;
    }

    const session = findSessionById(normalizedSessionId);
    if (!session) {
      throw new Error('session not found');
    }

    const request = webSessionApi
      .commandGroupDetail(session.projectId, normalizedSessionId, normalizedGroupId)
      .then(detail => {
        if (detail.status !== 'running') {
          commandGroupDetailsByKey.set(key, detail);
        }
        return detail;
      });
    const trackedRequest = request.finally(() => {
      if (inFlightCommandGroupDetailsByKey.get(key) === trackedRequest) {
        inFlightCommandGroupDetailsByKey.delete(key);
      }
    });
    inFlightCommandGroupDetailsByKey.set(key, trackedRequest);
    return trackedRequest;
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

  function isSessionSnapshotCurrent(sessionId: string, serverRevision?: string | null) {
    return sessionSync.isSnapshotCurrent(sessionId, serverRevision);
  }

  function setPendingAutoRetryOverride(
    sessionId: string,
    config: {
      enabled: boolean;
      policyMode: WebSessionSummary['autoRetryPolicyMode'];
      scope: WebSessionSummary['autoRetryScope'];
      preset: WebSessionSummary['autoRetryPreset'];
      maxAttempts?: number;
    },
    appliedAt = Date.now()
  ) {
    pendingAutoRetryOverrides.set(sessionId, {
      enabled: config.enabled === true,
      policyMode: config.policyMode,
      scope: config.scope,
      preset: config.preset,
      maxAttempts: normalizeAutoRetryMaxAttempts(config.maxAttempts),
      appliedAt,
    });
  }

  function acknowledgePendingAutoRetryOverride(sessionId: string, ackedAt = Date.now()) {
    const pendingOverride = pendingAutoRetryOverrides.get(sessionId);
    if (!pendingOverride) {
      return;
    }
    pendingAutoRetryOverrides.set(sessionId, {
      ...pendingOverride,
      ackedAt,
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
    const matchesPendingConfig =
      summary.autoRetryEnabled === pendingOverride.enabled &&
      summary.autoRetryPolicyMode === pendingOverride.policyMode &&
      summary.autoRetryScope === pendingOverride.scope &&
      summary.autoRetryPreset === pendingOverride.preset &&
      normalizeAutoRetryMaxAttempts(summary.autoRetryMaxAttempts) === pendingOverride.maxAttempts;
    const updatedAt = Date.parse(summary.updatedAt || '');
    const hasAuthoritativeUpdate =
      Number.isFinite(updatedAt) &&
      updatedAt >= (pendingOverride.ackedAt ?? pendingOverride.appliedAt);
    if (matchesPendingConfig && hasAuthoritativeUpdate) {
      pendingAutoRetryOverrides.delete(summary.id);
      return summary;
    }
    const mergedUpdatedAt = Number.isFinite(updatedAt)
      ? Math.max(updatedAt, pendingOverride.appliedAt)
      : pendingOverride.appliedAt;
    return {
      ...summary,
      autoRetryEnabled: pendingOverride.enabled,
      autoRetryPolicyMode: pendingOverride.policyMode,
      autoRetryScope: pendingOverride.scope,
      autoRetryPreset: pendingOverride.preset,
      autoRetryMaxAttempts: pendingOverride.maxAttempts,
      updatedAt: new Date(mergedUpdatedAt).toISOString(),
    };
  }

  function setPendingAutoRetryDispatchOverride(
    sessionId: string,
    enabled: boolean,
    appliedAt = Date.now()
  ) {
    pendingAutoRetryDispatchOverrides.set(sessionId, {
      enabled: enabled === true,
      appliedAt,
    });
  }

  function acknowledgePendingAutoRetryDispatchOverride(sessionId: string, ackedAt = Date.now()) {
    const pendingOverride = pendingAutoRetryDispatchOverrides.get(sessionId);
    if (!pendingOverride) {
      return;
    }
    pendingAutoRetryDispatchOverrides.set(sessionId, {
      ...pendingOverride,
      ackedAt,
    });
  }

  function clearPendingAutoRetryDispatchOverride(sessionId: string) {
    pendingAutoRetryDispatchOverrides.delete(sessionId);
  }

  function applyPendingAutoRetryDispatchOverride(summary: WebSessionSummary): WebSessionSummary {
    const pendingOverride = pendingAutoRetryDispatchOverrides.get(summary.id);
    if (!pendingOverride) {
      return summary;
    }
    const matchesPendingConfig =
      summary.autoRetryDispatchPendingOnFailure === pendingOverride.enabled;
    const updatedAt = Date.parse(summary.updatedAt || '');
    const hasAuthoritativeUpdate =
      Number.isFinite(updatedAt) &&
      updatedAt >= (pendingOverride.ackedAt ?? pendingOverride.appliedAt);
    if (matchesPendingConfig && hasAuthoritativeUpdate) {
      pendingAutoRetryDispatchOverrides.delete(summary.id);
      return summary;
    }
    const mergedUpdatedAt = Number.isFinite(updatedAt)
      ? Math.max(updatedAt, pendingOverride.appliedAt)
      : pendingOverride.appliedAt;
    return {
      ...summary,
      autoRetryDispatchPendingOnFailure: pendingOverride.enabled,
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
    pendingApproval: WebSessionApprovalState | null,
    pendingInputs: WebSessionPendingInput[],
    scheduledInputs: WebSessionScheduledInput[],
    history: {
      hasMore: boolean;
      beforeCursor?: string;
      total: number;
    },
    options?: {
      preserveArchivedPosition?: boolean;
      subAgents?: WebSessionSubAgent[];
    }
  ) {
    upsertSession(summary, options);
    setSnapshotApproval(sessionId, pendingApproval);
    resetSessionEvents(sessionId, items);
    setPendingInputs(sessionId, pendingInputs);
    setScheduledInputs(sessionId, scheduledInputs);
    if (options && Object.prototype.hasOwnProperty.call(options, 'subAgents')) {
      setSubAgents(sessionId, options.subAgents ?? []);
    }
    historyBySession.value = {
      ...historyBySession.value,
      [sessionId]: {
        hasMore: Boolean(history.hasMore),
        beforeCursor: String(history.beforeCursor ?? ''),
        total: Number(history.total ?? 0),
        loading: false,
      },
    };
    sessionSync.markHydrated(sessionId, summary.revision);
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

  function commitPendingInputEditDrafts(
    projectId: string,
    sessionId: string,
    drafts: Record<string, WebSessionPendingInputEditDraft>
  ) {
    const normalizedProjectId = String(projectId || '').trim();
    const normalizedSessionId = String(sessionId || '').trim();
    if (!normalizedProjectId || !normalizedSessionId) {
      return;
    }

    const nextDraftState = { ...pendingInputEditDraftStateByProject.value };
    const nextProjectDrafts = { ...(nextDraftState[normalizedProjectId] ?? {}) };
    if (Object.keys(drafts).length > 0) {
      nextProjectDrafts[normalizedSessionId] = drafts;
      nextDraftState[normalizedProjectId] = nextProjectDrafts;
    } else {
      delete nextProjectDrafts[normalizedSessionId];
      if (Object.keys(nextProjectDrafts).length > 0) {
        nextDraftState[normalizedProjectId] = nextProjectDrafts;
      } else {
        delete nextDraftState[normalizedProjectId];
      }
    }
    pendingInputEditDraftStateByProject.value = nextDraftState;
    persistPendingInputEditDrafts(nextDraftState);
  }

  function getPendingInputEditDraft(
    projectId: string,
    sessionId: string,
    pendingId: string
  ): WebSessionPendingInputEditDraft | null {
    const normalizedProjectId = String(projectId || '').trim();
    const normalizedSessionId = String(sessionId || '').trim();
    const normalizedPendingId = String(pendingId || '').trim();
    if (!normalizedProjectId || !normalizedSessionId || !normalizedPendingId) {
      return null;
    }
    const draft =
      pendingInputEditDraftStateByProject.value[normalizedProjectId]?.[normalizedSessionId]?.[
        normalizedPendingId
      ];
    return draft ? { ...draft } : null;
  }

  function getPendingInputEditDrafts(
    projectId: string,
    sessionId: string
  ): Record<string, WebSessionPendingInputEditDraft> {
    const normalizedProjectId = String(projectId || '').trim();
    const normalizedSessionId = String(sessionId || '').trim();
    if (!normalizedProjectId || !normalizedSessionId) {
      return {};
    }
    const drafts =
      pendingInputEditDraftStateByProject.value[normalizedProjectId]?.[normalizedSessionId] ?? {};
    return Object.fromEntries(Object.entries(drafts).map(([id, draft]) => [id, { ...draft }]));
  }

  function setPendingInputEditDraft(
    projectId: string,
    sessionId: string,
    pendingId: string,
    text: string
  ) {
    const normalizedProjectId = String(projectId || '').trim();
    const normalizedSessionId = String(sessionId || '').trim();
    const normalizedPendingId = String(pendingId || '').trim();
    if (!normalizedProjectId || !normalizedSessionId || !normalizedPendingId) {
      return;
    }
    const currentDrafts = getPendingInputEditDrafts(normalizedProjectId, normalizedSessionId);
    currentDrafts[normalizedPendingId] = {
      text: String(text ?? ''),
      updatedAt: Date.now(),
    };
    commitPendingInputEditDrafts(normalizedProjectId, normalizedSessionId, currentDrafts);
  }

  function clearPendingInputEditDraft(projectId: string, sessionId: string, pendingId: string) {
    const normalizedPendingId = String(pendingId || '').trim();
    if (!normalizedPendingId) {
      return;
    }
    const normalizedProjectId = String(projectId || '').trim();
    const normalizedSessionId = String(sessionId || '').trim();
    const currentDrafts = getPendingInputEditDrafts(normalizedProjectId, normalizedSessionId);
    if (!Object.prototype.hasOwnProperty.call(currentDrafts, normalizedPendingId)) {
      return;
    }
    delete currentDrafts[normalizedPendingId];
    commitPendingInputEditDrafts(normalizedProjectId, normalizedSessionId, currentDrafts);
  }

  function clearPendingInputEditDrafts(projectId: string, sessionId: string) {
    const normalizedProjectId = String(projectId || '').trim();
    const normalizedSessionId = String(sessionId || '').trim();
    if (!normalizedProjectId || !normalizedSessionId) {
      return;
    }
    if (!pendingInputEditDraftStateByProject.value[normalizedProjectId]?.[normalizedSessionId]) {
      return;
    }
    commitPendingInputEditDrafts(normalizedProjectId, normalizedSessionId, {});
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

  function restoreDraft(
    projectId: string,
    sessionId: string,
    submittedDraft: WebSessionDraftState
  ) {
    updateDraft(projectId, sessionId, currentDraft =>
      mergeWebSessionDraftForRestore(currentDraft, submittedDraft)
    );
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
      revision: normalizeWebSessionRevision(session.rev),
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
      autoRetryPolicyMode: session.arpm === 'custom' ? 'custom' : 'default',
      autoRetryScope:
        session.ars === 'network_and_rate_limit' || session.ars === 'all_failures'
          ? session.ars
          : 'network_only',
      autoRetryPreset:
        session.arp === 'aggressive_stop' || session.arp === 'sustain_60s'
          ? session.arp
          : 'gentle_stop',
      autoRetryMaxAttempts: normalizeAutoRetryMaxAttempts(session.aram),
      autoRetryDispatchPendingOnFailure: session.ardpf === true,
      cwd: session.cwd,
      nativeSessionId: session.nsid ?? null,
      cyberPolicyFlagged: session.cpf === true,
      hasScheduledPlanExecution: session.spe === true,
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
      workTiming: {
        completedDurationMs: Math.max(0, Number(session.wt?.dur ?? 0) || 0),
        currentRun:
          typeof session.wt?.cur?.sa === 'number' && Number.isFinite(session.wt.cur.sa)
            ? {
                id: String(session.wt.cur.id ?? ''),
                startedAt: new Date(session.wt.cur.sa).toISOString(),
                pausedAt:
                  typeof session.wt.cur.pa === 'number' && Number.isFinite(session.wt.cur.pa)
                    ? new Date(session.wt.cur.pa).toISOString()
                    : null,
                pausedDurationMs: Math.max(0, Number(session.wt.cur.pd ?? 0) || 0),
              }
            : null,
        backfillState:
          session.wt?.bs === 'complete' ||
          session.wt?.bs === 'partial' ||
          session.wt?.bs === 'unavailable' ||
          session.wt?.bs === 'failed'
            ? session.wt.bs
            : 'pending',
        backfillVersion: Math.max(0, Math.trunc(Number(session.wt?.bv ?? 0) || 0)),
      },
      goal: normalizeGoal(session.goal),
    };
  }

  function normalizePendingInput(item: {
    id?: string;
    mode?: 'redirect' | 'queue' | string;
    text?: string;
    attachmentIds?: string[];
    readyAt?: string | number | null;
    paused?: boolean;
    nativeQueued?: boolean;
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
    const parsedReadyAt =
      typeof item.readyAt === 'number'
        ? item.readyAt
        : Date.parse(typeof item.readyAt === 'string' ? item.readyAt : '');
    return {
      id,
      mode,
      text: typeof item.text === 'string' ? item.text : '',
      attachmentIds: Array.isArray(item.attachmentIds)
        ? item.attachmentIds.filter((value): value is string => typeof value === 'string')
        : [],
      readyAt: Number.isFinite(parsedReadyAt) ? parsedReadyAt : null,
      paused: item.paused === true,
      ...(item.nativeQueued === true ? { nativeQueued: true } : {}),
      createdAt: Number.isFinite(createdAt) ? createdAt : Date.now(),
    };
  }

  function normalizeScheduledInput(item: {
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
  }): WebSessionScheduledInput | null {
    const id = typeof item.id === 'string' ? item.id.trim() : '';
    if (!id) {
      return null;
    }
    const action = item.action === 'execute_plan' ? 'execute_plan' : 'message';
    const mode =
      item.mode === 'send'
        ? 'send'
        : item.mode === 'interrupt' || item.mode === 'redirect'
          ? 'interrupt'
          : item.mode === 'queue'
            ? 'queue'
            : '';
    const status =
      item.status === 'failed'
        ? 'failed'
        : item.status === 'expired'
          ? 'expired'
          : item.status === 'scheduled'
            ? 'scheduled'
            : '';
    if (!mode || !status) {
      return null;
    }
    const scheduleKind = item.scheduleKind === 'when_idle' ? 'when_idle' : 'at_time';
    const parsedScheduledFor =
      typeof item.scheduledFor === 'number'
        ? item.scheduledFor
        : Date.parse(typeof item.scheduledFor === 'string' ? item.scheduledFor : '');
    if (scheduleKind === 'at_time' && !Number.isFinite(parsedScheduledFor)) {
      return null;
    }
    const scheduledFor = Number.isFinite(parsedScheduledFor) ? parsedScheduledFor : null;
    const parsedIdleSince =
      typeof item.idleSince === 'number'
        ? item.idleSince
        : Date.parse(typeof item.idleSince === 'string' ? item.idleSince : '');
    const validBlockingReasons = new Set([
      'git_dirty',
      'git_unavailable',
      'non_plan_session_active',
    ]);
    const blockingReasons = Array.isArray(item.blockingReasons)
      ? item.blockingReasons.filter(
          (value): value is WebSessionScheduledInput['blockingReasons'][number] =>
            validBlockingReasons.has(value)
        )
      : [];
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
      action,
      targetId: typeof item.targetId === 'string' ? item.targetId.trim() : '',
      mode,
      exitPlanMode: item.exitPlanMode === true,
      status,
      lastError: typeof item.lastError === 'string' ? item.lastError.trim() : '',
      text: typeof item.text === 'string' ? item.text : '',
      attachmentIds: Array.isArray(item.attachmentIds)
        ? item.attachmentIds.filter((value): value is string => typeof value === 'string')
        : [],
      scheduleKind,
      scheduledFor,
      idleSince: Number.isFinite(parsedIdleSince) ? parsedIdleSince : null,
      blockingReasons,
      conditionError: typeof item.conditionError === 'string' ? item.conditionError.trim() : '',
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
      if (left.scheduleKind !== right.scheduleKind) {
        return left.scheduleKind === 'when_idle' ? -1 : 1;
      }
      if (
        left.scheduleKind === 'at_time' &&
        right.scheduleKind === 'at_time' &&
        left.scheduledFor !== right.scheduledFor
      ) {
        return (
          (left.scheduledFor ?? Number.MAX_SAFE_INTEGER) -
          (right.scheduledFor ?? Number.MAX_SAFE_INTEGER)
        );
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
      sourceThreadId:
        typeof record.sthid === 'string'
          ? record.sthid
          : typeof record.sourceThreadId === 'string'
            ? record.sourceThreadId
            : null,
      sourceTurnId:
        typeof record.stid === 'string'
          ? record.stid
          : typeof record.sourceTurnId === 'string'
            ? record.sourceTurnId
            : null,
      sourceItemId: normalizeHistorySourceItemId(record, rawPayload),
      runId:
        typeof record.rid === 'string'
          ? record.rid
          : typeof record.runId === 'string'
            ? record.runId
            : null,
      runDurationMs:
        typeof (record.dur ?? record.runDurationMs) === 'number' &&
        Number.isFinite(record.dur ?? record.runDurationMs)
          ? Math.max(0, Number(record.dur ?? record.runDurationMs))
          : null,
      runOutcome:
        record.out === 'completed' ||
        record.out === 'canceled' ||
        record.out === 'failed' ||
        record.out === 'timeout' ||
        record.out === 'interrupted'
          ? record.out
          : record.runOutcome === 'completed' ||
              record.runOutcome === 'canceled' ||
              record.runOutcome === 'failed' ||
              record.runOutcome === 'timeout' ||
              record.runOutcome === 'interrupted'
            ? record.runOutcome
            : null,
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
              approvalKind:
                typeof rawDetail?.approvalKind === 'string'
                  ? rawDetail.approvalKind
                  : typeof rawPayload?.kind === 'string'
                    ? rawPayload.kind
                    : undefined,
              command:
                typeof rawDetail?.command === 'string'
                  ? rawDetail.command
                  : typeof rawPayload?.command === 'string'
                    ? rawPayload.command
                    : undefined,
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
    replaceProjectSessions(projectId, next);
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
    const nextOutgoingMessages = { ...outgoingMessagesBySession.value };
    delete nextOutgoingMessages[sessionId];
    outgoingMessagesBySession.value = nextOutgoingMessages;
    const nextHistory = { ...historyBySession.value };
    delete nextHistory[sessionId];
    historyBySession.value = nextHistory;
    sessionSync.clear(sessionId);
    pendingInputVersionBySession.delete(sessionId);
    pendingAutoRetryOverrides.delete(sessionId);
    pendingAutoRetryDispatchOverrides.delete(sessionId);
    pendingActiveCallTimeoutOverrides.delete(sessionId);
    const nextSnapshotApprovals = { ...snapshotApprovalsBySession.value };
    delete nextSnapshotApprovals[sessionId];
    snapshotApprovalsBySession.value = nextSnapshotApprovals;
    const nextSubAgents = { ...subAgentsBySession.value };
    delete nextSubAgents[sessionId];
    subAgentsBySession.value = nextSubAgents;
    runtimeProjectionCacheBySession.delete(sessionId);
    eventIndexBySession.delete(sessionId);
    const nextPendingInputs = { ...pendingInputsBySession.value };
    delete nextPendingInputs[sessionId];
    pendingInputsBySession.value = nextPendingInputs;
    const nextScheduledInputs = { ...scheduledInputsBySession.value };
    delete nextScheduledInputs[sessionId];
    scheduledInputsBySession.value = nextScheduledInputs;
    completedTransitionVersionBySession.delete(sessionId);
    if (projectId) {
      clearPendingInputEditDrafts(projectId, sessionId);
      clearDraft(projectId, sessionId);
    }
  }

  function upsertCurrentSession(summary: WebSessionSummary) {
    const nextSummary = applyPendingActiveCallTimeoutOverride(
      applyPendingAutoRetryDispatchOverride(applyPendingAutoRetryOverride(summary))
    );
    const previousProjectId = currentSessionProjectById.get(nextSummary.id);
    if (previousProjectId && previousProjectId !== nextSummary.projectId) {
      removeCurrentSessionRecord(previousProjectId, nextSummary.id);
    }
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
    replaceProjectSessions(nextSummary.projectId, sortSessions(next));
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
      if (wasCurrentSession) {
        invalidateRuntimeProjection(summary.id);
      }
      removeCurrentSessionRecord(summary.projectId, summary.id);
      upsertArchivedSession(summary, {
        includeInMatchingScopes: wasCurrentSession,
        preserveScopeOrder: options?.preserveArchivedPosition === true,
      });
      return;
    }
    if (archivedSessionsById.value[summary.id]) {
      invalidateRuntimeProjection(summary.id);
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

  function invalidateCleanedHistories(sessionIds: string[]) {
    const ids = Array.from(new Set(sessionIds.map(id => String(id || '').trim()).filter(Boolean)));
    ids.forEach(sessionId => {
      clearSessionRuntimeState(sessionId);
      updateSessionStatus(sessionId, current => ({
        ...current,
        revision: undefined,
        hasUnread: false,
        syncState: 'missing',
        syncError: null,
        lastSyncMode: null,
        lastSyncedAt: null,
        turnCount: 0,
        itemCount: 0,
      }));
    });
  }

  function setPendingInputs(
    sessionId: string,
    items: WebSessionPendingInput[],
    options?: { authoritative?: boolean }
  ) {
    const nextPendingInputs = { ...pendingInputsBySession.value };
    if (items.length === 0) {
      delete nextPendingInputs[sessionId];
    } else {
      nextPendingInputs[sessionId] = items;
    }
    pendingInputsBySession.value = nextPendingInputs;
    if (options?.authoritative !== false) {
      pendingInputVersionBySession.set(
        sessionId,
        (pendingInputVersionBySession.get(sessionId) ?? 0) + 1
      );
    }
  }

  function setScheduledInputs(sessionId: string, items: WebSessionScheduledInput[]) {
    const nextScheduledInputs = { ...scheduledInputsBySession.value };
    if (items.length === 0) {
      delete nextScheduledInputs[sessionId];
    } else {
      nextScheduledInputs[sessionId] = sortScheduledInputs(items);
    }
    scheduledInputsBySession.value = nextScheduledInputs;
    const hasScheduledPlanExecution = items.some(
      item => item.action === 'execute_plan' && item.status === 'scheduled'
    );
    updateSessionStatus(
      sessionId,
      current => ({
        ...current,
        hasScheduledPlanExecution,
      }),
      { preserveOrder: true }
    );
  }

  function setSnapshotApproval(
    sessionId: string,
    approval: WebSessionApprovalState | null,
    invalidate = true
  ) {
    const next = { ...snapshotApprovalsBySession.value };
    if (approval) {
      next[sessionId] = approval;
    } else {
      delete next[sessionId];
    }
    snapshotApprovalsBySession.value = next;
    if (invalidate) {
      invalidateRuntimeProjection(sessionId);
    }
  }

  function approvalStateFromHistoryBlock(block: WebSessionBlock): WebSessionApprovalState | null {
    if (block.detail?.type !== 'approval_request') {
      return null;
    }
    const itemId = block.sourceItemId || block.id;
    return {
      id: itemId,
      itemId,
      kind: block.detail.approvalKind ?? '',
      prompt: block.detail.prompt ?? block.text,
      command: block.detail.command ?? '',
      requestedAt: block.timestamp,
      stale: false,
      actionable: true,
    };
  }

  function syncSnapshotApprovalFromRealtimeBlock(sessionId: string, block: WebSessionBlock) {
    const approval = approvalStateFromHistoryBlock(block);
    if (approval) {
      setSnapshotApproval(sessionId, approval, false);
      return;
    }
    if (
      block.detail?.type === 'approval_response' ||
      block.kind === 'user' ||
      block.itemType === 'run_abort' ||
      block.itemType === 'run_fail'
    ) {
      setSnapshotApproval(sessionId, null, false);
    }
  }

  function invalidateRuntimeProjection(sessionId: string) {
    runtimeProjectionCacheBySession.delete(sessionId);
  }

  function getSubAgents(sessionId: string) {
    const items = subAgentsBySession.value[sessionId] ?? [];
    const rootThreadId = String(findSessionById(sessionId)?.nativeSessionId ?? '').trim();
    return rootThreadId ? items.filter(item => item.id !== rootThreadId) : items;
  }

  function hasAuthoritativeSubAgents(sessionId: string) {
    return Object.prototype.hasOwnProperty.call(subAgentsBySession.value, sessionId);
  }

  function setSubAgents(sessionId: string, items: WebSessionSubAgent[]) {
    const rootThreadId = String(findSessionById(sessionId)?.nativeSessionId ?? '').trim();
    const normalized = items
      .filter(item => !rootThreadId || item.id !== rootThreadId)
      .sort((left, right) => {
        const leftTime = left.startedAt ?? 0;
        const rightTime = right.startedAt ?? 0;
        if (leftTime !== rightTime) {
          return leftTime - rightTime;
        }
        return left.id.localeCompare(right.id);
      });
    subAgentsBySession.value = {
      ...subAgentsBySession.value,
      [sessionId]: normalized,
    };
    invalidateRuntimeProjection(sessionId);
  }

  function upsertSubAgent(sessionId: string, agent: WebSessionSubAgent) {
    const current = getSubAgents(sessionId);
    const index = current.findIndex(item => item.id === agent.id);
    const next = [...current];
    if (index >= 0) {
      next[index] = agent;
    } else {
      next.push(agent);
    }
    setSubAgents(sessionId, next);
  }

  function getOutgoingMessages(sessionId: string) {
    return outgoingMessagesBySession.value[sessionId] ?? [];
  }

  function setOutgoingMessages(sessionId: string, messages: WebSessionOutgoingMessage[]) {
    const next = { ...outgoingMessagesBySession.value };
    if (messages.length === 0) {
      delete next[sessionId];
    } else {
      next[sessionId] = messages;
    }
    outgoingMessagesBySession.value = next;
  }

  function buildOutgoingMessageAttachments(
    attachmentIds: string[],
    attachments: WebSessionBlock['attachments'] = []
  ): WebSessionBlock['attachments'] {
    const attachmentById = new Map(
      attachments
        .filter(attachment => Boolean(attachment?.id))
        .map(attachment => [attachment.id, attachment] as const)
    );
    return attachmentIds
      .map(attachmentId => String(attachmentId || '').trim())
      .filter(Boolean)
      .map(attachmentId => {
        const attachment = attachmentById.get(attachmentId);
        return {
          id: attachmentId,
          name: attachment?.name || attachmentId,
          ...(attachment?.mime ? { mime: attachment.mime } : {}),
          ...(typeof attachment?.size === 'number' ? { size: attachment.size } : {}),
          ...(attachment?.path ? { path: attachment.path } : {}),
        };
      });
  }

  function stageOutgoingMessage(
    sessionId: string,
    text: string,
    attachmentIds: string[],
    options?: WebSessionSendMessageOptions
  ) {
    const current = getOutgoingMessages(sessionId);
    const normalizedText = text.trim();
    const requestedId = String(options?.outgoingMessageId || '').trim();
    const existingIndex = requestedId
      ? current.findIndex(message => message.id === requestedId)
      : -1;
    const attachments = buildOutgoingMessageAttachments(attachmentIds, options?.attachments);

    if (existingIndex >= 0) {
      const existing = current[existingIndex]!;
      const next = [...current];
      next.splice(existingIndex, 1, {
        ...existing,
        text: normalizedText,
        attachments,
        deliveryState: 'sending',
      });
      setOutgoingMessages(sessionId, next);
      return existing.id;
    }

    outgoingMessageSeed += 1;
    const timestamp = Date.now();
    const id =
      requestedId ||
      `outgoing_${timestamp}_${outgoingMessageSeed}_${Math.random().toString(36).slice(2, 8)}`;
    const events = buildBlocks(sessionId);
    const session = findSessionById(sessionId);
    const baseOrderIndex = Math.max(
      0,
      session?.itemCount ?? 0,
      getHistoryMeta(sessionId).total,
      events.reduce((latest, block) => Math.max(latest, block.orderIndex), 0)
    );
    setOutgoingMessages(sessionId, [
      ...current,
      {
        key: `outgoing:${id}`,
        id,
        runId: null,
        runDurationMs: null,
        runOutcome: null,
        orderIndex: baseOrderIndex,
        baseOrderIndex,
        kind: 'user',
        itemType: 'user_message',
        text: normalizedText,
        timestamp,
        observedAt: timestamp,
        attachments,
        deliveryState: 'sending',
      },
    ]);
    return id;
  }

  function setOutgoingMessageDeliveryState(
    sessionId: string,
    outgoingMessageId: string,
    deliveryState: WebSessionMessageDeliveryState
  ) {
    const current = getOutgoingMessages(sessionId);
    const index = current.findIndex(message => message.id === outgoingMessageId);
    if (index < 0) {
      return false;
    }
    if (current[index]?.deliveryState === deliveryState) {
      return true;
    }
    const next = [...current];
    next.splice(index, 1, {
      ...current[index]!,
      deliveryState,
    });
    setOutgoingMessages(sessionId, next);
    return true;
  }

  function discardOutgoingMessage(sessionId: string, outgoingMessageId: string) {
    const current = getOutgoingMessages(sessionId);
    const next = current.filter(message => message.id !== outgoingMessageId);
    if (next.length === current.length) {
      return;
    }
    setOutgoingMessages(sessionId, next);
  }

  function blockAttachmentIds(block: Pick<WebSessionBlock, 'attachments'>) {
    return block.attachments
      .map(attachment => String(attachment.id || '').trim())
      .filter(Boolean)
      .sort((left, right) => left.localeCompare(right));
  }

  function matchesOutgoingMessage(
    outgoing: WebSessionOutgoingMessage,
    authoritative: WebSessionBlock
  ) {
    if (
      authoritative.kind !== 'user' ||
      authoritative.itemType !== 'user_message' ||
      authoritative.orderIndex <= outgoing.baseOrderIndex ||
      authoritative.text.trim() !== outgoing.text
    ) {
      return false;
    }
    const outgoingAttachmentIds = blockAttachmentIds(outgoing);
    const authoritativeAttachmentIds = blockAttachmentIds(authoritative);
    return (
      outgoingAttachmentIds.length === authoritativeAttachmentIds.length &&
      outgoingAttachmentIds.every(
        (attachmentId, index) => attachmentId === authoritativeAttachmentIds[index]
      )
    );
  }

  function reconcileOutgoingMessages(sessionId: string, events: WebSessionBlock[]) {
    const current = getOutgoingMessages(sessionId);
    if (current.length === 0 || events.length === 0) {
      return;
    }
    const authoritativeUserMessages = events.filter(block => block.kind === 'user');
    const claimedIndexes = new Set<number>();
    const remaining = current.filter(outgoing => {
      const matchIndex = authoritativeUserMessages.findIndex(
        (block, index) => !claimedIndexes.has(index) && matchesOutgoingMessage(outgoing, block)
      );
      if (matchIndex < 0) {
        return true;
      }
      claimedIndexes.add(matchIndex);
      return false;
    });
    if (remaining.length !== current.length) {
      setOutgoingMessages(sessionId, remaining);
    }
  }

  function sortEventBlocks(items: WebSessionBlock[]) {
    webSessionRuntimePerformanceCounters.eventSorts += 1;
    return [...items].sort((left, right) => left.orderIndex - right.orderIndex);
  }

  function rebuildEventIndex(sessionId: string, items: WebSessionBlock[]) {
    eventIndexBySession.set(sessionId, new Map(items.map((item, index) => [item.id, index])));
  }

  function getEventIndex(sessionId: string, items: WebSessionBlock[]) {
    const existing = eventIndexBySession.get(sessionId);
    if (existing) {
      return existing;
    }
    rebuildEventIndex(sessionId, items);
    return eventIndexBySession.get(sessionId)!;
  }

  function replaceSessionEvents(sessionId: string, events: WebSessionBlock[]) {
    eventsBySession.value = {
      ...eventsBySession.value,
      [sessionId]: events,
    };
    reconcileOutgoingMessages(sessionId, events);
    return eventsBySession.value[sessionId] ?? [];
  }

  function mergeHistoricalEvents(sessionId: string, incoming: WebSessionBlock[]) {
    if (incoming.length === 0) {
      return;
    }
    const current = eventsBySession.value[sessionId] ?? emptySessionBlocks;
    const indexById = getEventIndex(sessionId, current);
    const merged = [...current];
    incoming.forEach(item => {
      if (!item?.id) {
        return;
      }
      const existingIndex = indexById.get(item.id);
      if (existingIndex == null) {
        indexById.set(item.id, merged.length);
        merged.push(item);
        return;
      }
      merged[existingIndex] = {
        ...merged[existingIndex],
        ...item,
      };
    });
    const storedEvents = replaceSessionEvents(sessionId, sortEventBlocks(merged));
    rebuildEventIndex(sessionId, storedEvents);
    invalidateRuntimeProjection(sessionId);
  }

  function mergeRealtimeEvent(sessionId: string, item: WebSessionBlock) {
    if (!item?.id) {
      return;
    }
    const current = eventsBySession.value[sessionId] ?? emptySessionBlocks;
    const indexById = getEventIndex(sessionId, current);
    const existingIndex = indexById.get(item.id);
    const previousCache = runtimeProjectionCacheBySession.get(sessionId);
    syncSnapshotApprovalFromRealtimeBlock(sessionId, item);
    let nextEvents: WebSessionBlock[];
    let incrementalSeed: RuntimeAccumulator | null = null;
    let incrementalStartIndex = -1;

    if (existingIndex != null) {
      nextEvents = [...current];
      nextEvents[existingIndex] = {
        ...current[existingIndex],
        ...item,
      };
      if (existingIndex === current.length - 1) {
        incrementalSeed = previousCache?.beforeLastAccumulator ?? null;
        incrementalStartIndex = existingIndex;
      }
    } else {
      const lastOrderIndex = current[current.length - 1]?.orderIndex ?? Number.NEGATIVE_INFINITY;
      if (item.orderIndex >= lastOrderIndex) {
        nextEvents = [...current, item];
        indexById.set(item.id, nextEvents.length - 1);
        incrementalSeed = previousCache?.accumulator ?? null;
        incrementalStartIndex = nextEvents.length - 1;
      } else {
        nextEvents = sortEventBlocks([...current, item]);
      }
    }

    const storedEvents = replaceSessionEvents(sessionId, nextEvents);
    if (existingIndex == null && incrementalStartIndex < 0) {
      rebuildEventIndex(sessionId, storedEvents);
    }

    const session = findSessionById(sessionId);
    if (
      session &&
      incrementalSeed &&
      previousCache?.blocks === current &&
      previousCache.session === session
    ) {
      cacheIncrementalRuntimeProjection(
        sessionId,
        session,
        storedEvents,
        incrementalSeed,
        incrementalStartIndex
      );
      return;
    }
    invalidateRuntimeProjection(sessionId);
  }

  function trimInactiveSessionEvents(activeSessionId = '') {
    const nextEvents = { ...eventsBySession.value };
    let changed = false;
    Object.entries(eventsBySession.value).forEach(([sessionId, items]) => {
      if (sessionId === activeSessionId || items.length <= WEB_SESSION_MAX_RETAINED_BLOCKS) {
        return;
      }
      nextEvents[sessionId] = items.slice(-WEB_SESSION_MIN_RETAINED_BLOCKS);
      rebuildEventIndex(sessionId, nextEvents[sessionId]);
      invalidateRuntimeProjection(sessionId);
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
    const storedEvents = replaceSessionEvents(sessionId, sortEventBlocks(events));
    rebuildEventIndex(sessionId, storedEvents);
    invalidateRuntimeProjection(sessionId);
  }

  function buildBlocks(sessionId: string): WebSessionBlock[] {
    return eventsBySession.value[sessionId] ?? emptySessionBlocks;
  }

  const getBlocks = (sessionId: string) => buildBlocks(sessionId);

  function getTimelineBlocks(sessionId: string): WebSessionBlock[] {
    const events = buildBlocks(sessionId);
    const outgoing = getOutgoingMessages(sessionId);
    if (outgoing.length === 0) {
      return events;
    }

    const pending = [...outgoing].sort(
      (left, right) =>
        left.baseOrderIndex - right.baseOrderIndex ||
        left.timestamp - right.timestamp ||
        left.id.localeCompare(right.id)
    );
    const timeline: WebSessionBlock[] = [];
    let pendingIndex = 0;
    for (const event of events) {
      while (
        pendingIndex < pending.length &&
        pending[pendingIndex]!.baseOrderIndex < event.orderIndex
      ) {
        timeline.push(pending[pendingIndex]!);
        pendingIndex += 1;
      }
      timeline.push(event);
    }
    timeline.push(...pending.slice(pendingIndex));
    return timeline;
  }

  function createRuntimeAccumulator(session: WebSessionSummary | null): RuntimeAccumulator {
    const snapshotApproval =
      session?.assistantState === 'waiting_approval'
        ? (snapshotApprovalsBySession.value[session.id] ?? null)
        : null;
    const authoritativeSubAgents = Boolean(session && hasAuthoritativeSubAgents(session.id));
    const registry = session ? getSubAgents(session.id) : [];
    const knownSubAgents = new Map<string, WebSessionLiveSubAgent>();
    const activeSubAgents = new Map<string, WebSessionLiveSubAgent>();
    registry.forEach(agent => {
      knownSubAgents.set(agent.id, agent);
      if (isActiveSubAgent(agent)) {
        activeSubAgents.set(agent.id, agent);
      }
    });
    return {
      pendingApproval: snapshotApproval,
      pendingUserInput: null,
      activeTool: undefined,
      activeSubAgents,
      knownSubAgents,
      authoritativeSubAgents,
      rootThreadId: session?.nativeSessionId?.trim() ?? '',
      sawAssistantOutput: false,
      assistantDone: false,
      errorMessage: '',
      updatedAt: session ? Date.parse(session.updatedAt) || Date.now() : Date.now(),
      runActive: false,
    };
  }

  function cloneRuntimeAccumulator(accumulator: RuntimeAccumulator): RuntimeAccumulator {
    return {
      ...accumulator,
      activeSubAgents: new Map(accumulator.activeSubAgents),
      knownSubAgents: new Map(accumulator.knownSubAgents),
    };
  }

  function applyRuntimeBlock(accumulator: RuntimeAccumulator, block: WebSessionBlock) {
    const approval = approvalStateFromHistoryBlock(block);
    if (approval) {
      accumulator.pendingApproval = approval;
    } else if (block.detail?.type === 'approval_response' || block.kind === 'user') {
      accumulator.pendingApproval = null;
    } else if (
      block.itemType === 'run_abort' &&
      accumulator.pendingApproval &&
      isProcessRestartPayload(block.payload ?? undefined)
    ) {
      accumulator.pendingApproval = {
        ...accumulator.pendingApproval,
        stale: true,
        recoveryReason: String(block.payload?.reason ?? ''),
        recoveryMessage: getRecoveryMessage(block.payload ?? undefined),
      };
    } else if (block.itemType === 'run_abort' || block.itemType === 'run_fail') {
      accumulator.pendingApproval = null;
    }

    if (block.detail?.type === 'user_input_request') {
      accumulator.pendingUserInput = {
        id: block.id,
        itemId: block.sourceItemId || block.id,
        prompt: block.detail.prompt ?? block.text,
        questions: block.detail.questions ?? [],
        requestedAt: block.timestamp,
        stale: false,
      };
    } else if (block.detail?.type === 'user_input_response' || block.kind === 'user') {
      accumulator.pendingUserInput = null;
    } else if (
      block.itemType === 'run_abort' &&
      accumulator.pendingUserInput &&
      isProcessRestartPayload(block.payload ?? undefined)
    ) {
      accumulator.pendingUserInput = {
        ...accumulator.pendingUserInput,
        stale: true,
        recoveryReason: String(block.payload?.reason ?? ''),
        recoveryMessage: getRecoveryMessage(block.payload ?? undefined),
      };
    } else if (block.itemType === 'run_abort' || block.itemType === 'run_fail') {
      accumulator.pendingUserInput = null;
    }

    const sourceThreadId = String(block.sourceThreadId ?? '').trim();
    const isChildThreadBlock = Boolean(
      sourceThreadId &&
        (accumulator.knownSubAgents.has(sourceThreadId) ||
          (accumulator.rootThreadId && sourceThreadId !== accumulator.rootThreadId))
    );
    if (isChildThreadBlock) {
      return;
    }

    accumulator.updatedAt = block.observedAt || block.timestamp || accumulator.updatedAt;
    if (block.kind === 'assistant') {
      accumulator.sawAssistantOutput = true;
      accumulator.assistantDone = block.done === true;
      accumulator.retryState = undefined;
      if (!accumulator.firstAssistantOutputAt && block.timestamp > 0) {
        accumulator.firstAssistantOutputAt = block.timestamp;
      }
      applyAssistantNamedSubAgents(
        block.text,
        accumulator.knownSubAgents,
        accumulator.activeSubAgents
      );
    }
    const blockRunId = String(block.runId ?? '').trim();
    if (block.itemType === 'run_st' && block.timestamp > 0) {
      accumulator.runStartedAt = block.timestamp;
      accumulator.activeRunId = blockRunId || undefined;
      accumulator.runActive = true;
      accumulator.sawAssistantOutput = false;
      accumulator.assistantDone = false;
      accumulator.firstAssistantOutputAt = undefined;
      accumulator.activeTool = undefined;
      if (!accumulator.authoritativeSubAgents) {
        accumulator.activeSubAgents = new Map();
        accumulator.knownSubAgents = new Map();
      }
      accumulator.errorMessage = '';
    } else if (
      block.kind === 'user' &&
      block.timestamp > 0 &&
      (!accumulator.runActive || Boolean(blockRunId && blockRunId !== accumulator.activeRunId))
    ) {
      accumulator.runStartedAt = block.timestamp;
      accumulator.activeRunId = blockRunId || undefined;
      accumulator.runActive = true;
      accumulator.sawAssistantOutput = false;
      accumulator.assistantDone = false;
      accumulator.firstAssistantOutputAt = undefined;
      accumulator.activeTool = undefined;
      if (!accumulator.authoritativeSubAgents) {
        accumulator.activeSubAgents = new Map();
        accumulator.knownSubAgents = new Map();
      }
      accumulator.errorMessage = '';
    }
    const retryPayload = getTransportRetryPayload(block.payload);
    const progressTimestamp = getRetryClearingProgressTimestamp(block, retryPayload);
    if (progressTimestamp != null) {
      accumulator.latestProgressTimestamp = Math.max(
        accumulator.latestProgressTimestamp ?? 0,
        progressTimestamp
      );
    }
    if (block.itemType === 'note' && retryPayload) {
      accumulator.retryState = {
        ...retryPayload,
        updatedAt: block.observedAt || block.timestamp || accumulator.updatedAt,
      };
    }
    if (block.kind === 'tool' && block.tool) {
      const normalizedToolKind = normalizeToolKindValue(
        block.tool.kind || String(block.tool.meta?.kind ?? '')
      );
      if (normalizedToolKind === 'reasoning') {
        return;
      }
      if (normalizedToolKind === 'sub_agent_tool_call') {
        if (!accumulator.authoritativeSubAgents) {
          rememberKnownSubAgents(block, accumulator.knownSubAgents);
          syncActiveSubAgentLifecycle(
            block,
            accumulator.knownSubAgents,
            accumulator.activeSubAgents
          );
        }
        accumulator.retryState = undefined;
        return;
      }
      if (block.tool.status === 'running') {
        accumulator.activeTool = normalizeActiveTool(block);
        accumulator.retryState = undefined;
      } else if (accumulator.activeTool?.id === block.tool.id) {
        accumulator.activeTool = undefined;
        accumulator.retryState = undefined;
      }
    }
    if (block.itemType === 'run_fail') {
      accumulator.errorMessage = block.text || 'Run failed';
      accumulator.activeTool = undefined;
      if (!accumulator.authoritativeSubAgents) {
        accumulator.activeSubAgents = new Map();
      }
      accumulator.retryState = undefined;
    }
    if (block.itemType === 'run_abort') {
      accumulator.activeTool = undefined;
      if (!accumulator.authoritativeSubAgents) {
        accumulator.activeSubAgents = new Map();
      }
    }
    if (
      block.itemType === 'run_done' ||
      block.itemType === 'run_abort' ||
      block.itemType === 'run_fail'
    ) {
      accumulator.runActive = false;
      accumulator.activeRunId = undefined;
    }
  }

  function applySnapshotApprovalToAccumulator(
    session: WebSessionSummary | null,
    accumulator: RuntimeAccumulator
  ) {
    if (session?.assistantState !== 'waiting_approval') {
      return;
    }
    const approval = snapshotApprovalsBySession.value[session.id];
    if (approval) {
      accumulator.pendingApproval = approval;
    }
  }

  function finalizeRuntimeProjection(
    session: WebSessionSummary | null,
    accumulator: RuntimeAccumulator
  ): RuntimeProjection {
    const approval =
      session?.assistantState === 'waiting_approval'
        ? (snapshotApprovalsBySession.value[session.id] ?? accumulator.pendingApproval)
        : accumulator.pendingApproval;
    const userInput = accumulator.pendingUserInput;
    const assistantState = getSessionAssistantStateValue(session);
    const assistantStateUpdatedAt = getAssistantStateUpdatedAt(session);
    let liveState: WebSessionLiveState;
    if (assistantState === 'waiting_approval') {
      liveState = withActiveSubAgents(
        {
          phase: 'waiting_approval',
          running: session?.status === 'running',
          updatedAt: approval?.requestedAt ?? assistantStateUpdatedAt ?? accumulator.updatedAt,
          startedAt: approval?.requestedAt ?? assistantStateUpdatedAt ?? accumulator.runStartedAt,
          approval,
          tool: accumulator.activeTool,
        },
        [...accumulator.activeSubAgents.values()]
      );
    } else if (assistantState === 'waiting_plan_approval') {
      liveState = withActiveSubAgents(
        {
          phase: 'waiting_plan_approval',
          running: false,
          updatedAt: assistantStateUpdatedAt || accumulator.updatedAt,
          startedAt: assistantStateUpdatedAt ?? accumulator.runStartedAt,
        },
        [...accumulator.activeSubAgents.values()]
      );
    } else if (assistantState === 'waiting_input') {
      liveState = withActiveSubAgents(
        {
          phase: 'waiting_input',
          running: session?.status === 'running',
          updatedAt: userInput?.requestedAt ?? assistantStateUpdatedAt ?? accumulator.updatedAt,
          startedAt: userInput?.requestedAt ?? assistantStateUpdatedAt ?? accumulator.runStartedAt,
          tool: accumulator.activeTool,
          userInput,
        },
        [...accumulator.activeSubAgents.values()]
      );
    } else if (session?.status === 'running') {
      const hasRecoveredFromRetry =
        accumulator.retryState != null &&
        accumulator.latestProgressTimestamp != null &&
        accumulator.latestProgressTimestamp > accumulator.retryState.updatedAt;
      const hasNewerWorkingSummary =
        accumulator.retryState != null &&
        assistantState === 'working' &&
        assistantStateUpdatedAt != null &&
        assistantStateUpdatedAt > accumulator.retryState.updatedAt;
      if (accumulator.retryState && !hasRecoveredFromRetry && !hasNewerWorkingSummary) {
        liveState = withActiveSubAgents(
          {
            phase: 'retrying',
            running: true,
            updatedAt: accumulator.retryState.updatedAt,
            startedAt: accumulator.runStartedAt,
            retry: {
              code: accumulator.retryState.code,
              message: accumulator.retryState.message,
              remoteUrl: accumulator.retryState.remoteUrl,
              attempt: accumulator.retryState.attempt,
              maxAttempts: accumulator.retryState.maxAttempts,
            },
          },
          [...accumulator.activeSubAgents.values()]
        );
      } else if (accumulator.activeTool) {
        liveState = withActiveSubAgents(
          {
            phase: 'tool',
            running: true,
            updatedAt: accumulator.updatedAt,
            startedAt:
              accumulator.activeTool.startedAt ??
              assistantStateUpdatedAt ??
              accumulator.runStartedAt,
            tool: accumulator.activeTool,
          },
          [...accumulator.activeSubAgents.values()]
        );
      } else if (accumulator.sawAssistantOutput && !accumulator.assistantDone) {
        liveState = withActiveSubAgents(
          {
            phase: 'thinking',
            running: true,
            updatedAt: accumulator.updatedAt,
            startedAt:
              accumulator.firstAssistantOutputAt ??
              assistantStateUpdatedAt ??
              accumulator.runStartedAt,
          },
          [...accumulator.activeSubAgents.values()]
        );
      } else {
        liveState = withActiveSubAgents(
          {
            phase: 'starting',
            running: true,
            updatedAt: accumulator.updatedAt,
            startedAt: assistantStateUpdatedAt ?? accumulator.runStartedAt,
          },
          [...accumulator.activeSubAgents.values()]
        );
      }
    } else if (session?.status === 'done') {
      liveState = withActiveSubAgents(
        {
          phase: 'done',
          running: false,
          updatedAt: accumulator.updatedAt,
          startedAt: accumulator.runStartedAt,
        },
        [...accumulator.activeSubAgents.values()]
      );
    } else if (session?.status === 'err') {
      liveState = withActiveSubAgents(
        {
          phase: 'error',
          running: false,
          updatedAt: accumulator.updatedAt,
          startedAt: accumulator.runStartedAt,
          errorMessage: accumulator.errorMessage,
        },
        [...accumulator.activeSubAgents.values()]
      );
    } else {
      liveState = withActiveSubAgents(
        {
          phase: 'idle',
          running: false,
          updatedAt: accumulator.updatedAt,
        },
        [...accumulator.activeSubAgents.values()]
      );
    }

    return {
      liveState,
      pendingApproval: approval,
      pendingUserInput: userInput,
    };
  }

  function deriveRuntimeProjection(
    session: WebSessionSummary | null,
    blocks: WebSessionBlock[],
    capture?: {
      accumulator?: RuntimeAccumulator;
      beforeLastAccumulator?: RuntimeAccumulator;
    }
  ): RuntimeProjection {
    webSessionRuntimePerformanceCounters.fullDerivations += 1;
    webSessionRuntimePerformanceCounters.scannedBlocks += blocks.length;
    const accumulator = createRuntimeAccumulator(session);
    let beforeLastAccumulator = cloneRuntimeAccumulator(accumulator);
    blocks.forEach((block, index) => {
      if (index === blocks.length - 1) {
        beforeLastAccumulator = cloneRuntimeAccumulator(accumulator);
      }
      applyRuntimeBlock(accumulator, block);
    });
    applySnapshotApprovalToAccumulator(session, accumulator);
    if (capture) {
      capture.accumulator = accumulator;
      capture.beforeLastAccumulator = beforeLastAccumulator;
    }
    return finalizeRuntimeProjection(session, accumulator);
  }

  function cacheIncrementalRuntimeProjection(
    sessionId: string,
    session: WebSessionSummary,
    blocks: WebSessionBlock[],
    seed: RuntimeAccumulator,
    startIndex: number
  ) {
    webSessionRuntimePerformanceCounters.incrementalDerivations += 1;
    webSessionRuntimePerformanceCounters.scannedBlocks += blocks.length - startIndex;
    const accumulator = cloneRuntimeAccumulator(seed);
    let beforeLastAccumulator = cloneRuntimeAccumulator(accumulator);
    for (let index = startIndex; index < blocks.length; index += 1) {
      if (index === blocks.length - 1) {
        beforeLastAccumulator = cloneRuntimeAccumulator(accumulator);
      }
      applyRuntimeBlock(accumulator, blocks[index]!);
    }
    applySnapshotApprovalToAccumulator(session, accumulator);
    runtimeProjectionCacheBySession.set(sessionId, {
      blocks,
      session,
      projection: finalizeRuntimeProjection(session, accumulator),
      accumulator,
      beforeLastAccumulator,
    });
  }

  function getRuntimeProjection(sessionId: string): RuntimeProjection {
    const session = findSessionById(sessionId);
    const blocks = buildBlocks(sessionId);
    if (session) {
      const cached = runtimeProjectionCacheBySession.get(sessionId);
      if (cached?.blocks === blocks && cached.session === session) {
        return cached.projection;
      }
      const capture: {
        accumulator?: RuntimeAccumulator;
        beforeLastAccumulator?: RuntimeAccumulator;
      } = {};
      const projection = deriveRuntimeProjection(session, blocks, capture);
      runtimeProjectionCacheBySession.set(sessionId, {
        blocks,
        session,
        projection,
        accumulator: capture.accumulator!,
        beforeLastAccumulator: capture.beforeLastAccumulator!,
      });
      return projection;
    }
    return deriveRuntimeProjection(null, blocks);
  }

  function getPendingApproval(sessionId: string): WebSessionApprovalState | null {
    return getRuntimeProjection(sessionId).pendingApproval;
  }

  function getPendingUserInput(sessionId: string): WebSessionUserInputState | null {
    return getRuntimeProjection(sessionId).pendingUserInput;
  }

  function getLiveState(sessionId: string): WebSessionLiveState {
    return getRuntimeProjection(sessionId).liveState;
  }

  function snapshotRuntimeMutationState(sessionId: string): RuntimeMutationStateSnapshot {
    const projection = getRuntimeProjection(sessionId);
    const liveState = projection.liveState;
    return {
      blockCount: buildBlocks(sessionId).length,
      historyTotal: getHistoryMeta(sessionId).total,
      pendingInputCount: getPendingInputs(sessionId).length,
      pendingInputVersion: pendingInputVersionBySession.get(sessionId) ?? 0,
      livePhase: liveState.phase,
      liveRunning: liveState.running,
      liveUpdatedAt: liveState.updatedAt,
      approvalId: projection.pendingApproval?.id ?? '',
      userInputId: projection.pendingUserInput?.itemId ?? '',
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
    options: RuntimeMutationHydrationOptions,
    acknowledgedRevision: string
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

    let snapshotError: unknown = null;
    if (options.forceSnapshot || !sessionSync.isSnapshotCurrent(sessionId, acknowledgedRevision)) {
      try {
        await loadSessionSnapshot(session.projectId, sessionId, {
          rememberActive: false,
          conditional: !options.forceSnapshot,
        });
      } catch (error) {
        snapshotError = error;
      }
    }
    if (options.predicate()) {
      return true;
    }

    const timeoutMs = Math.max(0, options.timeoutMs ?? WEB_SESSION_RUNTIME_MUTATION_TIMEOUT_MS);
    const settledFromEvents = await waitForRuntimeMutationPredicate(
      options.predicate,
      timeoutMs,
      WEB_SESSION_RUNTIME_MUTATION_PASSIVE_POLL_MS
    );
    if (settledFromEvents) {
      return true;
    }

    if (snapshotError) {
      console.warn('[Web Session] Runtime mutation hydration did not settle cleanly', {
        sessionId,
        label: options.label,
        error: snapshotError,
      });
    }
    return options.predicate();
  }

  function updateSessionStatus(
    sessionId: string,
    updater: (current: WebSessionSummary) => WebSessionSummary,
    options?: { preserveOrder?: boolean }
  ) {
    const indexedProjectId = currentSessionProjectById.get(sessionId);
    const projectId =
      indexedProjectId ||
      Object.entries(sessionsByProject.value).find(([, sessions]) =>
        sessions.some(item => item.id === sessionId)
      )?.[0];
    if (projectId) {
      const sessions = getSessions(projectId);
      const index = sessions.findIndex(item => item.id === sessionId);
      if (index >= 0) {
        const nextSessions = [...sessions];
        nextSessions.splice(index, 1, updater(sessions[index]!));
        replaceProjectSessions(
          projectId,
          options?.preserveOrder ? nextSessions : sortSessions(nextSessions)
        );
        return;
      }
      currentSessionProjectById.delete(sessionId);
    }

    const archived = archivedSessionsById.value[sessionId];
    if (archived) {
      archivedSessionsById.value = {
        ...archivedSessionsById.value,
        [sessionId]: updater(archived),
      };
      if (!options?.preserveOrder) {
        sortArchivedScopeContainingSession(sessionId);
      }
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

  function getApprovalForNotification(
    sessionId: string,
    projection: RuntimeProjection
  ): WebSessionApprovalState | null {
    if (projection.pendingApproval) {
      return projection.pendingApproval;
    }

    const state = projection.liveState;
    if (state.phase !== 'waiting_plan_approval') {
      return null;
    }

    return {
      id: `status:${sessionId}:${state.phase}:${state.updatedAt}`,
      itemId: '',
      kind: 'plan_approval',
      prompt: '',
      command: '',
      requestedAt: state.updatedAt,
      stale: false,
      actionable: false,
    };
  }

  function emitStateTransition(
    sessionId: string,
    previousProjection: RuntimeProjection,
    nextProjection: RuntimeProjection
  ) {
    const session = findSessionById(sessionId);
    if (!session) {
      return;
    }

    const previousState = previousProjection.liveState;
    const nextState = nextProjection.liveState;
    const hasPendingInputs = getPendingInputs(sessionId).length > 0;
    const previousApprovalForNotification = getApprovalForNotification(
      sessionId,
      previousProjection
    );
    const approvalForNotification = getApprovalForNotification(sessionId, nextProjection);
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
      (!previousApprovalForNotification ||
        previousApprovalForNotification.id !== approvalForNotification.id ||
        previousApprovalForNotification.requestedAt !== approvalForNotification.requestedAt)
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
      const request = frame.rid ? pending.get(frame.rid) : undefined;
      const isMissingPendingInput = Boolean(
        request &&
          frame.code === 'not_found' &&
          frame.msg === 'pending input not found' &&
          ['pending_del', 'pending_update', 'pending_reorder'].includes(request.operation)
      );
      if (!isMissingPendingInput) {
        lastError.value = frame.msg ?? 'Unknown websocket error';
      }
      if (frame.rid && request) {
        request.reject(
          new WebSessionCommandError({
            code: frame.code ?? '',
            message: frame.msg ?? frame.code ?? 'unknown error',
            operation: request.operation,
            sessionId: request.sessionId,
          })
        );
        pending.delete(frame.rid);
      }
      return;
    }

    if (frame.k === 'ack') {
      if (frame.sid) {
        sessionSync.observe(frame.sid, frame.rev);
      }
      if (frame.op === 'set_ar' && frame.sid) {
        acknowledgePendingAutoRetryOverride(frame.sid, Date.now());
      }
      if (frame.op === 'set_ardpf' && frame.sid) {
        acknowledgePendingAutoRetryDispatchOverride(frame.sid, Date.now());
      }
      if (frame.rid && pending.has(frame.rid)) {
        pending.get(frame.rid)?.resolve(frame);
        pending.delete(frame.rid);
      }
      return;
    }

    if (frame.k === 'evt' && frame.sid) {
      if (frame.op === 'resync_required') {
        const payload = asRecord(frame.p);
        sessionSync.requestHydration(frame.sid, frame.rev, String(payload?.reason ?? '').trim());
        return;
      }
      sessionSync.observe(frame.sid, frame.rev);
      if (!sessionSync.shouldApply(frame.sid, frame.rev)) {
        return;
      }
      const shouldEmitTransition = frame.op !== 'hist_page';
      const previousProjection = shouldEmitTransition ? getRuntimeProjection(frame.sid) : null;
      if (frame.s) {
        upsertSession({
          ...normalizeSession(frame.s),
          revision: normalizeWebSessionRevision(frame.rev ?? frame.s.rev),
        });
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
                    readyAt: item.ra,
                    paused: item.ps,
                    nativeQueued: item.nq,
                    createdAt: item.ca,
                  })
                )
                .filter((item): item is WebSessionPendingInput => item != null)
            : []
        );
        emitStateTransition(frame.sid, previousProjection!, getRuntimeProjection(frame.sid));
        sessionSync.markApplied(frame.sid, frame.rev);
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
                    action: item.a,
                    targetId: item.tid,
                    mode: item.m,
                    exitPlanMode: item.epm,
                    status: item.st,
                    lastError: item.err,
                    text: item.txt,
                    attachmentIds: item.atts,
                    scheduleKind: item.sk,
                    scheduledFor: item.sf,
                    idleSince: item.is,
                    blockingReasons: item.br,
                    conditionError: item.ce,
                    createdAt: item.ca,
                    updatedAt: item.ua,
                    sentAt: item.sa,
                    canceledAt: item.xa,
                  })
                )
                .filter((item): item is WebSessionScheduledInput => item != null)
            : []
        );
        emitStateTransition(frame.sid, previousProjection!, getRuntimeProjection(frame.sid));
        sessionSync.markApplied(frame.sid, frame.rev);
        return;
      }

      if (frame.op === 'sub_agent' && frame.ag) {
        const agent = normalizeSubAgent(frame.ag);
        if (agent) {
          upsertSubAgent(frame.sid, agent);
        }
        emitStateTransition(frame.sid, previousProjection!, getRuntimeProjection(frame.sid));
        sessionSync.markApplied(frame.sid, frame.rev);
        return;
      }

      if (frame.op === 'hist_page' && frame.h) {
        const historicalItems = Array.isArray(frame.h.its)
          ? frame.h.its.map(item => normalizeHistoryItem(item))
          : [];
        mergeHistoricalEvents(frame.sid, historicalItems);
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
        mergeRealtimeEvent(frame.sid, item);
      }

      emitStateTransition(frame.sid, previousProjection!, getRuntimeProjection(frame.sid));
      sessionSync.markApplied(frame.sid, frame.rev);
    }
  }

  function rejectPendingCommands(reason: Error) {
    pending.forEach(entry => {
      entry.reject(reason);
    });
    pending.clear();
  }

  function buildHeartbeatPayload(operation: WebSessionWireHeartbeatOperation, sessionId = '') {
    return JSON.stringify(buildWebSessionHeartbeatFrame(operation, sessionId));
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

  function sendSocketHeartbeat(
    socket: WebSocket | null,
    operation: WebSessionWireHeartbeatOperation,
    sessionId = ''
  ) {
    if (!socket || socket.readyState !== WebSocket.OPEN) {
      return;
    }
    socket.send(buildHeartbeatPayload(operation, sessionId));
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
          void reconcileRecentSessions().catch(error => {
            console.warn('[Web Session] Failed to reconcile after event stream recovery', error);
          });
        }
        eventHasConnectedOnce = true;
        resolve();
      };
      ws.onmessage = event => {
        try {
          const frame = parseWireFrame(event.data);
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
          const frame = parseWireFrame(event.data);
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
    const frame = buildWebSessionCommandFrame(requestId, op, sessionId, payload);
    const promise = new Promise<WireFrame>((resolve, reject) => {
      pending.set(requestId, { resolve, reject, operation: op, sessionId });
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
    let acknowledgement: WireFrame;
    try {
      acknowledgement = await sendCommand(op, sessionId, payload);
    } catch (error) {
      if (await reconcileMissingPendingInput(error)) {
        return;
      }
      throw error;
    }
    const revision = normalizeWebSessionRevision(acknowledgement.rev);
    if (!revision) {
      throw new Error(`websocket command ${op} returned no snapshot revision`);
    }
    await hydrateRuntimeMutation(sessionId, hydration, revision);
  }

  async function reconcileMissingPendingInput(error: unknown) {
    if (
      !(error instanceof WebSessionCommandError) ||
      error.code !== 'not_found' ||
      error.message !== 'pending input not found' ||
      !['pending_del', 'pending_update', 'pending_reorder'].includes(error.operation)
    ) {
      return false;
    }
    const session = findSessionById(error.sessionId);
    if (!session?.projectId || session.archivedAt) {
      return false;
    }
    await loadSessionSnapshot(session.projectId, error.sessionId, {
      rememberActive: false,
    });
    return true;
  }

  async function loadSessions(projectId: string, force = false) {
    if (!projectId) {
      return [];
    }
    if (!force && loadedProjects.value[projectId]) {
      return sessionsByProject.value[projectId] ?? [];
    }
    const existingRequest = inFlightSessionLists.get(projectId);
    if (existingRequest) {
      return existingRequest;
    }
    const request = webSessionApi.list(projectId).then(items => {
      const currentById = new Map(
        (sessionsByProject.value[projectId] ?? []).map(session => [session.id, session])
      );
      const sessions = items.map(item => {
        sessionSync.observe(item.id, item.revision);
        const session = applyPendingActiveCallTimeoutOverride(
          applyPendingAutoRetryDispatchOverride(
            applyPendingAutoRetryOverride({
              ...item,
              hasScheduledPlanExecution: item.hasScheduledPlanExecution === true,
            })
          )
        );
        const current = currentById.get(session.id);
        return current && compareWebSessionRevisions(current.revision, session.revision) === 1
          ? current
          : session;
      });
      replaceProjectSessions(projectId, sortSessions(sessions));
      syncSessionCount(projectId);
      loadedProjects.value = {
        ...loadedProjects.value,
        [projectId]: true,
      };
      if (!hasStoredActiveSession(projectId)) {
        const mostRecentSession = selectMostRecentWebSession(sessions);
        if (mostRecentSession) {
          rememberActiveSession(projectId, mostRecentSession.id);
        }
      }
      return sessions;
    });
    inFlightSessionLists.set(projectId, request);
    void request.then(
      () => {
        if (inFlightSessionLists.get(projectId) === request) {
          inFlightSessionLists.delete(projectId);
        }
      },
      () => {
        if (inFlightSessionLists.get(projectId) === request) {
          inFlightSessionLists.delete(projectId);
        }
      }
    );
    return request;
  }

  function sessionReconcileTimestamp(session: WebSessionSummary) {
    const timestamps = [
      session.statusUpdatedAt,
      session.assistantStateUpdatedAt,
      session.activityAt,
      session.updatedAt,
    ]
      .map(value => Date.parse(value || ''))
      .filter(Number.isFinite);
    return timestamps.length > 0 ? Math.max(...timestamps) : 0;
  }

  function isSessionReconcilePriority(session: WebSessionSummary) {
    return (
      session.status === 'running' ||
      session.status === 'waiting_approval' ||
      session.status === 'aborting' ||
      Boolean(session.assistantState)
    );
  }

  function buildSessionReconcileTargets(now = Date.now()): WebSessionReconcileTarget[] {
    const sessionsByID = new Map<string, WebSessionSummary>();
    Object.values(sessionsByProject.value).forEach(sessions => {
      sessions.forEach(session => sessionsByID.set(session.id, session));
    });
    Object.values(archivedSessionsById.value).forEach(session => {
      if (!sessionsByID.has(session.id)) {
        sessionsByID.set(session.id, session);
      }
    });

    const sessions = [...sessionsByID.values()];
    const priorityIDs = new Set<string>();
    const priority = sessions
      .filter(
        session => session.id === eventFocusedSessionId || isSessionReconcilePriority(session)
      )
      .sort((left, right) => {
        if (left.id === eventFocusedSessionId) return -1;
        if (right.id === eventFocusedSessionId) return 1;
        return sessionReconcileTimestamp(right) - sessionReconcileTimestamp(left);
      });
    priority.forEach(session => priorityIDs.add(session.id));
    const cutoff = now - WEB_SESSION_RECONCILE_RECENT_WINDOW_MS;
    const recent = sessions
      .filter(
        session =>
          !session.archivedAt &&
          sessionReconcileTimestamp(session) >= cutoff &&
          !priorityIDs.has(session.id)
      )
      .sort((left, right) => sessionReconcileTimestamp(right) - sessionReconcileTimestamp(left))
      .slice(0, WEB_SESSION_RECONCILE_RECENT_LIMIT);

    return [...priority, ...recent].slice(0, WEB_SESSION_RECONCILE_MAX_TARGETS).map(session => ({
      id: session.id,
      ...(session.revision ? { revision: session.revision } : {}),
    }));
  }

  async function reconcileRecentSessions() {
    if (inFlightSessionReconcile) {
      return inFlightSessionReconcile;
    }
    const now = Date.now();
    if (now - lastSessionReconcileCompletedAt < WEB_SESSION_RECONCILE_MIN_INTERVAL_MS) {
      return { items: [], missingIds: [] } satisfies WebSessionReconcileResult;
    }
    const targets = buildSessionReconcileTargets(now);
    if (targets.length === 0) {
      return { items: [], missingIds: [] } satisfies WebSessionReconcileResult;
    }

    const request = webSessionApi.reconcile(targets).then(result => {
      result.items.forEach(summary => {
        const current = findSessionById(summary.id);
        if (current && compareWebSessionRevisions(current.revision, summary.revision) === 1) {
          return;
        }
        sessionSync.observe(summary.id, summary.revision);
        upsertSession({
          ...summary,
          hasScheduledPlanExecution: summary.hasScheduledPlanExecution === true,
        });
        sessionSync.markApplied(summary.id, summary.revision);
      });
      result.missingIds.forEach(sessionId => {
        const current = findSessionById(sessionId);
        if (current) {
          removeSession(current.projectId, sessionId);
        }
      });
      lastSessionReconcileCompletedAt = Date.now();
      return result;
    });
    inFlightSessionReconcile = request;
    const clearRequest = () => {
      if (inFlightSessionReconcile === request) {
        inFlightSessionReconcile = null;
      }
    };
    void request.then(clearRequest, clearRequest);
    return request;
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
  ): Promise<WebSessionSnapshot | null> {
    if (!projectId || !sessionId) {
      return null;
    }
    setHistoryLoading(sessionId, true);
    try {
      const limit = Math.max(1, Math.trunc(options?.limit ?? 80));
      const key = `${projectId}:${sessionId}:${limit}`;
      let flight = inFlightSnapshots.get(key);
      if (!flight) {
        const controller = new AbortController();
        const knownRevision = options?.conditional ? sessionSync.getHydrated(sessionId) : '';
        const promise = webSessionApi.snapshot(projectId, sessionId, {
          limit,
          signal: controller.signal,
          ...(knownRevision ? { knownRevision } : {}),
        });
        flight = {
          promise,
          controller,
          consumers: new Set<symbol>(),
        };
        inFlightSnapshots.set(key, flight);
        const activeFlight = flight;
        void promise.then(
          () => {
            if (inFlightSnapshots.get(key) === activeFlight) {
              inFlightSnapshots.delete(key);
            }
          },
          () => {
            if (inFlightSnapshots.get(key) === activeFlight) {
              inFlightSnapshots.delete(key);
            }
          }
        );
      }

      const consumer = Symbol(sessionId);
      flight.consumers.add(consumer);
      let abortHandler: (() => void) | null = null;
      const snapshot = await new Promise<WebSessionSnapshot>((resolve, reject) => {
        const release = () => {
          options?.signal?.removeEventListener('abort', abortHandler!);
          flight?.consumers.delete(consumer);
          if (flight && flight.consumers.size === 0 && inFlightSnapshots.get(key) === flight) {
            flight.controller.abort();
          }
        };
        abortHandler = () => {
          release();
          const error = new Error('The operation was aborted.');
          error.name = 'AbortError';
          reject(error);
        };
        if (options?.signal?.aborted) {
          abortHandler();
          return;
        }
        options?.signal?.addEventListener('abort', abortHandler, { once: true });
        flight!.promise.then(
          value => {
            release();
            resolve(value);
          },
          error => {
            release();
            reject(error);
          }
        );
      });
      const responseRevision = normalizeWebSessionRevision(
        snapshot.revision ?? snapshot.session?.revision
      );
      let result = snapshot;
      let accepted = true;
      sessionSync.observe(sessionId, responseRevision);
      if (snapshot.unchanged) {
        sessionSync.markHydrated(sessionId, responseRevision);
        setHistoryLoading(sessionId, false);
      } else if (snapshot.session && snapshot.history) {
        const summary = {
          ...snapshot.session,
          revision: responseRevision || snapshot.session.revision,
        };
        if (!sessionSync.shouldApply(sessionId, responseRevision)) {
          accepted = false;
          setHistoryLoading(sessionId, false);
          result = {
            revision: sessionSync.getApplied(sessionId),
            unchanged: true,
          };
        } else {
          applySessionSnapshot(
            sessionId,
            summary,
            Array.isArray(snapshot.history?.items)
              ? snapshot.history.items.map(item => normalizeHistoryItem(item as WireHistoryItem))
              : [],
            normalizePendingApproval(snapshot.pendingApproval),
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
              ...(Array.isArray(snapshot.subAgents)
                ? {
                    subAgents: snapshot.subAgents
                      .map(item => normalizeSubAgent(item))
                      .filter((item): item is WebSessionSubAgent => item != null),
                  }
                : {}),
            }
          );
        }
      } else {
        setHistoryLoading(sessionId, false);
      }
      if (accepted && options?.rememberActive !== false) {
        rememberActiveSession(projectId, sessionId);
      }
      const observedRevision = sessionSync.getObserved(sessionId);
      if (
        options?.conditional &&
        !options.skipTrailing &&
        compareWebSessionRevisions(responseRevision, observedRevision) === -1
      ) {
        return loadSessionSnapshot(projectId, sessionId, {
          ...options,
          skipTrailing: true,
        });
      }
      return result;
    } catch (error) {
      setHistoryLoading(sessionId, false);
      throw error;
    }
  }

  async function hydrateSessionTarget(
    projectId: string,
    target: WebSessionHydrationTarget,
    options?: { preserveArchivedPosition?: boolean }
  ) {
    const revision = normalizeWebSessionRevision(target.revision ?? target.session.revision);
    const summary = {
      ...target.session,
      revision: revision || target.session.revision,
    };
    sessionSync.observe(summary.id, summary.revision);
    upsertSession(summary, options);
    await loadSessionSnapshot(projectId, summary.id, {
      rememberActive: false,
      conditional: true,
      preserveArchivedPosition: options?.preserveArchivedPosition === true,
    });
    return summary;
  }

  async function renameSession(projectId: string, sessionId: string, title: string) {
    await sendCommand('rename', sessionId, { ttl: title });
    rememberActiveSession(projectId, sessionId);
  }

  async function archiveSession(projectId: string, sessionId: string) {
    const summary = await webSessionApi.archive(projectId, sessionId);
    invalidateRuntimeProjection(sessionId);
    removeCurrentSessionRecord(projectId, sessionId);
    setPendingInputs(sessionId, []);
    setScheduledInputs(sessionId, []);
    upsertArchivedSession(summary, { includeInMatchingScopes: true });
    return summary;
  }

  async function unarchiveSession(projectId: string, sessionId: string) {
    const summary = await webSessionApi.unarchive(projectId, sessionId);
    invalidateRuntimeProjection(sessionId);
    removeArchivedSessionRecord(sessionId);
    upsertCurrentSession(summary);
    return summary;
  }

  async function importSession(
    projectId: string,
    sessionId: string,
    mode?: 'fast' | 'deep',
    agent: 'codex' | 'pi' = 'codex'
  ): Promise<WebSessionImportResult> {
    const result = await webSessionApi.importSession(projectId, {
      agent,
      sessionId,
      mode,
    });
    if (result?.session) {
      await hydrateSessionTarget(projectId, result);
      rememberActiveSession(projectId, result.session.id);
    }
    return result;
  }

  async function editUserMessage(
    projectId: string,
    sessionId: string,
    itemId: string,
    text: string
  ) {
    const target = await webSessionApi.editUserMessage(projectId, sessionId, itemId, text);
    const branchId = target.session.id;
    await hydrateSessionTarget(projectId, target);
    rememberActiveSession(projectId, branchId);
    emitter.emit('web-session:created', {
      projectId,
      sessionId: branchId,
    });
    return target;
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
      const target = await webSessionApi.sync(projectId, sessionId, mode, clearExisting);
      await hydrateSessionTarget(projectId, target, {
        preserveArchivedPosition: Boolean(session?.archivedAt),
      });
      if (rememberActive) {
        rememberActiveSession(projectId, sessionId);
      }
      return target;
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
    mode?: 'redirect' | 'queue',
    options?: WebSessionSendMessageOptions
  ) {
    const session = findSessionById(sessionId);
    if (session?.archivedAt) {
      throw new Error('session is archived');
    }
    const outgoingMessageId = mode
      ? ''
      : stageOutgoingMessage(sessionId, text, attachmentIds, options);
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
          readyAt:
            mode === 'redirect' && (session.agent === 'codex' || session.agent === 'pi')
              ? Date.now() + WEB_SESSION_NATIVE_STEER_UNDO_WINDOW_MS
              : null,
          paused: false,
          createdAt: Date.now(),
        }),
        { authoritative: false }
      );
    }

    const payload = {
      txt: text,
      atts: attachmentIds,
      ...(mode ? { mode, pid: optimisticPendingId } : {}),
    };
    const hydration: RuntimeMutationHydrationOptions = {
      label: 'send',
      forceSnapshot: Boolean(optimisticPendingId),
      predicate: () => {
        if (optimisticPendingId) {
          return (
            (pendingInputVersionBySession.get(sessionId) ?? 0) > beforeState.pendingInputVersion
          );
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
    };

    if (outgoingMessageId) {
      let acknowledgement: WireFrame;
      try {
        acknowledgement = await sendCommand('send', sessionId, payload);
      } catch (error) {
        if (setOutgoingMessageDeliveryState(sessionId, outgoingMessageId, 'failed')) {
          throw new WebSessionMessageDeliveryError(outgoingMessageId, error);
        }
        return;
      }

      setOutgoingMessageDeliveryState(sessionId, outgoingMessageId, 'accepted');
      const revision = normalizeWebSessionRevision(acknowledgement.rev);
      if (!revision) {
        console.warn('[Web Session] Accepted send returned no snapshot revision', {
          sessionId,
        });
        return;
      }
      try {
        await hydrateRuntimeMutation(sessionId, hydration, revision);
      } catch (error) {
        console.warn('[Web Session] Accepted send hydration failed', {
          sessionId,
          error,
        });
      }
      return;
    }

    try {
      await runRuntimeMutationCommand(sessionId, 'send', payload, hydration);
    } catch (error) {
      if (optimisticPendingId) {
        setPendingInputs(
          sessionId,
          getPendingInputs(sessionId).filter(item => item.id !== optimisticPendingId),
          { authoritative: false }
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

  async function updatePendingInput(sessionId: string, pendingId: string, text: string) {
    const normalizedText = text.trim();
    await runRuntimeMutationCommand(
      sessionId,
      'pending_update',
      { id: pendingId, txt: normalizedText, paused: false },
      {
        label: 'pending_update',
        predicate: () =>
          getPendingInputs(sessionId).some(
            item => item.id === pendingId && item.text === normalizedText && !item.paused
          ),
      }
    );
  }

  async function pausePendingInput(sessionId: string, pendingId: string) {
    await runRuntimeMutationCommand(
      sessionId,
      'pending_update',
      { id: pendingId, paused: true },
      {
        label: 'pending_pause',
        predicate: () =>
          getPendingInputs(sessionId).some(item => item.id === pendingId && item.paused),
      }
    );
  }

  async function resumePendingInput(sessionId: string, pendingId: string) {
    await runRuntimeMutationCommand(
      sessionId,
      'pending_update',
      { id: pendingId, paused: false },
      {
        label: 'pending_resume',
        predicate: () =>
          getPendingInputs(sessionId).some(item => item.id === pendingId && !item.paused),
      }
    );
  }

  async function reorderPendingInput(
    sessionId: string,
    pendingId: string,
    mode: WebSessionPendingInputMode,
    index: number
  ) {
    const normalizedIndex = Math.max(0, Math.trunc(index));
    await runRuntimeMutationCommand(
      sessionId,
      'pending_reorder',
      {
        id: pendingId,
        mode,
        idx: normalizedIndex,
      },
      {
        label: 'pending_reorder',
        predicate: () => {
          const partition = getPendingInputs(sessionId).filter(item => item.mode === mode);
          return partition[normalizedIndex]?.id === pendingId;
        },
      }
    );
  }

  async function clearPendingInputs(sessionId: string) {
    await runRuntimeMutationCommand(
      sessionId,
      'pending_clear',
      {},
      {
        label: 'pending_clear',
        predicate: () => getPendingInputs(sessionId).length === 0,
      }
    );
  }

  async function scheduleMessage(
    sessionId: string,
    text: string,
    attachmentIds: string[],
    scheduledForOrSchedule: number | WebSessionSchedule,
    mode: 'send' | 'interrupt' | 'queue' = 'send',
    options: { exitPlanMode?: boolean } = {}
  ) {
    const session = findSessionById(sessionId);
    if (session?.archivedAt) {
      throw new Error('session is archived');
    }
    const schedule: WebSessionSchedule =
      typeof scheduledForOrSchedule === 'number'
        ? { scheduleKind: 'at_time', scheduledFor: scheduledForOrSchedule }
        : scheduledForOrSchedule;
    const frame = await sendCommand('schedule_send', sessionId, {
      txt: text,
      atts: attachmentIds,
      mode,
      epm: options.exitPlanMode === true,
      sk: schedule.scheduleKind,
      ...(schedule.scheduleKind === 'at_time' ? { at: schedule.scheduledFor } : {}),
    });
    const payload = asRecord(frame.p);
    const created = normalizeScheduledInput({
      id: typeof payload?.id === 'string' ? payload.id : '',
      action: typeof payload?.a === 'string' ? payload.a : 'message',
      targetId: typeof payload?.tid === 'string' ? payload.tid : '',
      mode: typeof payload?.m === 'string' ? payload.m : '',
      exitPlanMode: typeof payload?.epm === 'boolean' ? payload.epm : options.exitPlanMode === true,
      status: typeof payload?.st === 'string' ? payload.st : '',
      lastError: typeof payload?.err === 'string' ? payload.err : '',
      text: typeof payload?.txt === 'string' ? payload.txt : text,
      attachmentIds: Array.isArray(payload?.atts)
        ? payload.atts.filter((value): value is string => typeof value === 'string')
        : attachmentIds,
      scheduleKind: typeof payload?.sk === 'string' ? payload.sk : schedule.scheduleKind,
      scheduledFor:
        typeof payload?.sf === 'number'
          ? payload.sf
          : schedule.scheduleKind === 'at_time'
            ? schedule.scheduledFor
            : null,
      idleSince:
        typeof payload?.is === 'number' || typeof payload?.is === 'string' ? payload.is : null,
      blockingReasons: Array.isArray(payload?.br)
        ? payload.br.filter((value): value is string => typeof value === 'string')
        : [],
      conditionError: typeof payload?.ce === 'string' ? payload.ce : '',
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

  async function schedulePlanExecution(
    sessionId: string,
    scheduledForOrSchedule: number | WebSessionSchedule,
    target: WebSessionPlanExecutionTarget
  ) {
    const session = findSessionById(sessionId);
    if (session?.archivedAt) {
      throw new Error('session is archived');
    }
    const schedule: WebSessionSchedule =
      typeof scheduledForOrSchedule === 'number'
        ? { scheduleKind: 'at_time', scheduledFor: scheduledForOrSchedule }
        : scheduledForOrSchedule;
    const frame = await sendCommand('schedule_plan', sessionId, {
      pid: target.planItemId,
      iid: target.pendingItemId ?? '',
      qid: target.questionId ?? '',
      opt: target.executeOptionLabel ?? '',
      sk: schedule.scheduleKind,
      ...(schedule.scheduleKind === 'at_time' ? { at: schedule.scheduledFor } : {}),
    });
    const payload = asRecord(frame.p);
    const created = normalizeScheduledInput({
      id: typeof payload?.id === 'string' ? payload.id : '',
      action: typeof payload?.a === 'string' ? payload.a : 'execute_plan',
      targetId: typeof payload?.tid === 'string' ? payload.tid : target.planItemId,
      mode: typeof payload?.m === 'string' ? payload.m : 'send',
      status: typeof payload?.st === 'string' ? payload.st : '',
      lastError: typeof payload?.err === 'string' ? payload.err : '',
      text: typeof payload?.txt === 'string' ? payload.txt : 'Implement the plan.',
      attachmentIds: Array.isArray(payload?.atts)
        ? payload.atts.filter((value): value is string => typeof value === 'string')
        : [],
      scheduleKind: typeof payload?.sk === 'string' ? payload.sk : schedule.scheduleKind,
      scheduledFor:
        typeof payload?.sf === 'number'
          ? payload.sf
          : schedule.scheduleKind === 'at_time'
            ? schedule.scheduledFor
            : null,
      idleSince:
        typeof payload?.is === 'number' || typeof payload?.is === 'string' ? payload.is : null,
      blockingReasons: Array.isArray(payload?.br)
        ? payload.br.filter((value): value is string => typeof value === 'string')
        : [],
      conditionError: typeof payload?.ce === 'string' ? payload.ce : '',
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

  async function updateScheduledInput(
    sessionId: string,
    inputId: string,
    update: {
      scheduleKind?: 'at_time' | 'when_idle';
      scheduledFor?: number | null;
      text?: string;
      mode?: 'send' | 'interrupt' | 'queue';
      exitPlanMode?: boolean;
    }
  ) {
    const current = getScheduledInputs(sessionId).find(item => item.id === inputId);
    if (!current) {
      throw new Error('scheduled input not found');
    }
    const frame = await sendCommand('scheduled_update', sessionId, {
      id: inputId,
      ...(update.scheduleKind ? { sk: update.scheduleKind } : {}),
      ...(typeof update.scheduledFor === 'number' ? { at: update.scheduledFor } : {}),
      ...(typeof update.text === 'string' ? { txt: update.text } : {}),
      ...(update.mode ? { mode: update.mode } : {}),
      ...(typeof update.exitPlanMode === 'boolean' ? { epm: update.exitPlanMode } : {}),
    });
    const payload = asRecord(frame.p);
    const updated = normalizeScheduledInput({
      id: typeof payload?.id === 'string' ? payload.id : current.id,
      action: typeof payload?.a === 'string' ? payload.a : current.action,
      targetId: typeof payload?.tid === 'string' ? payload.tid : current.targetId,
      mode: typeof payload?.m === 'string' ? payload.m : (update.mode ?? current.mode),
      exitPlanMode:
        typeof payload?.epm === 'boolean'
          ? payload.epm
          : (update.exitPlanMode ?? current.exitPlanMode),
      status: typeof payload?.st === 'string' ? payload.st : 'scheduled',
      lastError: typeof payload?.err === 'string' ? payload.err : '',
      text: typeof payload?.txt === 'string' ? payload.txt : (update.text ?? current.text),
      attachmentIds: Array.isArray(payload?.atts)
        ? payload.atts.filter((value): value is string => typeof value === 'string')
        : current.attachmentIds,
      scheduleKind:
        typeof payload?.sk === 'string'
          ? payload.sk
          : (update.scheduleKind ?? current.scheduleKind),
      scheduledFor:
        typeof payload?.sf === 'number'
          ? payload.sf
          : update.scheduleKind === 'when_idle'
            ? null
            : (update.scheduledFor ?? current.scheduledFor),
      idleSince:
        typeof payload?.is === 'number' || typeof payload?.is === 'string' ? payload.is : null,
      blockingReasons: Array.isArray(payload?.br)
        ? payload.br.filter((value): value is string => typeof value === 'string')
        : [],
      conditionError: typeof payload?.ce === 'string' ? payload.ce : '',
      createdAt:
        typeof payload?.ca === 'number' || typeof payload?.ca === 'string'
          ? payload.ca
          : current.createdAt,
      updatedAt:
        typeof payload?.ua === 'number' || typeof payload?.ua === 'string'
          ? payload.ua
          : Date.now(),
      sentAt:
        typeof payload?.sa === 'number' || typeof payload?.sa === 'string' ? payload.sa : null,
      canceledAt:
        typeof payload?.xa === 'number' || typeof payload?.xa === 'string' ? payload.xa : null,
    });
    if (!updated) {
      throw new Error('invalid scheduled update response');
    }
    setScheduledInputs(
      sessionId,
      sortScheduledInputs([
        ...getScheduledInputs(sessionId).filter(item => item.id !== updated.id),
        updated,
      ])
    );
    return updated;
  }

  async function dispatchScheduledInputNow(sessionId: string, inputId: string) {
    await sendCommand('scheduled_now', sessionId, { id: inputId });
    setScheduledInputs(
      sessionId,
      getScheduledInputs(sessionId).filter(item => item.id !== inputId)
    );
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

  async function compactSession(sessionId: string) {
    await runRuntimeMutationCommand(
      sessionId,
      'compact',
      {},
      {
        label: 'compact',
        predicate: () => getLiveState(sessionId).running,
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
      mergeHistoricalEvents(sessionId, historicalItems);
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

  async function fetchHistoryWindow(
    sessionId: string,
    options?: {
      beforeCursor?: string;
      afterCursor?: string;
      limit?: number;
    }
  ): Promise<WebSessionHistoryPage> {
    const session = findSessionById(sessionId);
    if (!session) {
      throw new Error('session not found');
    }
    const history = await webSessionApi.history(session.projectId, sessionId, options);
    return {
      items: Array.isArray(history.items)
        ? history.items.map(item => normalizeHistoryItem(item as WireHistoryItem))
        : [],
      hasMore: Boolean(history.hasMore),
      beforeCursor: String(history.beforeCursor ?? ''),
      hasLater: Boolean(history.hasLater),
      afterCursor: String(history.afterCursor ?? ''),
      total: Number(history.total ?? 0),
    };
  }

  async function updateModel(sessionId: string, model: string) {
    await sendCommand('set_md', sessionId, { md: model });
  }

  async function updateClaudeRuntime(sessionId: string, claudeRuntime: 'claude' | 'ccr') {
    await sendCommand('set_cr', sessionId, { cr: claudeRuntime });
  }

  async function updateReasoningEffort(
    sessionId: string,
    reasoningEffort: WebSessionReasoningEffort
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

  async function refreshGoal(sessionId: string) {
    const acknowledgement = await sendCommand('goal_get', sessionId, {});
    const revision = normalizeWebSessionRevision(acknowledgement.rev);
    if (!revision) {
      throw new Error('websocket command goal_get returned no snapshot revision');
    }
    const payload = asRecord(acknowledgement.p);
    if (!payload || !Object.prototype.hasOwnProperty.call(payload, 'goal')) {
      throw new Error('websocket command goal_get returned no goal payload');
    }
    updateSessionStatus(sessionId, current => ({
      ...current,
      revision,
      goal: normalizeGoal(payload.goal as WireSession['goal']),
    }));
    sessionSync.markApplied(sessionId, revision);
  }

  async function setGoal(
    sessionId: string,
    objective: string,
    status: WebSessionGoal['status'] = 'active'
  ) {
    await runRuntimeMutationCommand(
      sessionId,
      'goal_set',
      { obj: objective, st: status },
      {
        label: 'goal_set',
        predicate: () => findSessionById(sessionId)?.goal?.objective?.trim() === objective.trim(),
      }
    );
  }

  async function bootstrapGoal(
    sessionId: string,
    objective: string,
    status: WebSessionGoal['status'] = 'active'
  ) {
    await runRuntimeMutationCommand(
      sessionId,
      'goal_bootstrap',
      { obj: objective, st: status },
      {
        label: 'goal_bootstrap',
        predicate: () => {
          const session = findSessionById(sessionId);
          if (!session) {
            return false;
          }
          return (
            session.goal?.objective?.trim() === objective.trim() &&
            Boolean(session.nativeSessionId) &&
            getLiveState(sessionId).running
          );
        },
      }
    );
  }

  async function pauseGoal(sessionId: string) {
    await runRuntimeMutationCommand(
      sessionId,
      'goal_pause',
      {},
      {
        label: 'goal_pause',
        predicate: () => findSessionById(sessionId)?.goal?.status === 'paused',
      }
    );
  }

  async function resumeGoal(sessionId: string) {
    await runRuntimeMutationCommand(
      sessionId,
      'goal_resume',
      {},
      {
        label: 'goal_resume',
        predicate: () => findSessionById(sessionId)?.goal?.status === 'active',
      }
    );
  }

  async function clearGoal(sessionId: string) {
    await runRuntimeMutationCommand(
      sessionId,
      'goal_clear',
      {},
      {
        label: 'goal_clear',
        predicate: () => !findSessionById(sessionId)?.goal,
      }
    );
  }

  async function updatePermissionLevel(
    sessionId: string,
    permissionLevel: 'default' | 'elevated' | 'yolo'
  ) {
    await sendCommand('set_pl', sessionId, { pl: permissionLevel });
  }

  async function updateAgent(sessionId: string, agent: WebSessionAgent) {
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
      policyMode?: 'default' | 'custom';
      scope?: 'network_only' | 'network_and_rate_limit' | 'all_failures';
      preset?: 'gentle_stop' | 'aggressive_stop' | 'sustain_60s';
      maxAttempts?: number;
    }
  ) {
    const session = findSessionById(sessionId);
    const hasPolicyOverride =
      config.scope !== undefined || config.preset !== undefined || config.maxAttempts !== undefined;
    const policyMode =
      config.policyMode ??
      (hasPolicyOverride
        ? 'custom'
        : session?.autoRetryPolicyMode === 'custom'
          ? 'custom'
          : 'default');
    const scope = config.scope ?? session?.autoRetryScope ?? 'network_only';
    const preset = config.preset ?? session?.autoRetryPreset ?? 'gentle_stop';
    const maxAttempts = normalizeAutoRetryMaxAttempts(
      config.maxAttempts ?? session?.autoRetryMaxAttempts
    );
    const optimisticUpdatedAt = new Date().toISOString();
    const previous =
      session && !session.archivedAt
        ? {
            enabled: session.autoRetryEnabled,
            policyMode: session.autoRetryPolicyMode,
            scope: session.autoRetryScope,
            preset: session.autoRetryPreset,
            maxAttempts: normalizeAutoRetryMaxAttempts(session.autoRetryMaxAttempts),
          }
        : null;
    if (previous) {
      setPendingAutoRetryOverride(
        sessionId,
        { enabled: config.enabled, policyMode, scope, preset, maxAttempts },
        Date.parse(optimisticUpdatedAt)
      );
      updateSessionStatus(sessionId, current => ({
        ...current,
        autoRetryEnabled: config.enabled === true,
        autoRetryPolicyMode: policyMode,
        autoRetryScope: scope,
        autoRetryPreset: preset,
        autoRetryMaxAttempts: maxAttempts,
        updatedAt: optimisticUpdatedAt,
      }));
    }
    try {
      await sendCommand('set_ar', sessionId, {
        ae: config.enabled === true,
        arpm: policyMode,
        ...(policyMode === 'custom' ? { ars: scope, arp: preset, aram: maxAttempts } : {}),
      });
    } catch (error) {
      clearPendingAutoRetryOverride(sessionId);
      if (previous) {
        updateSessionStatus(sessionId, current => ({
          ...current,
          autoRetryEnabled: previous.enabled,
          autoRetryPolicyMode: previous.policyMode,
          autoRetryScope: previous.scope,
          autoRetryPreset: previous.preset,
          autoRetryMaxAttempts: previous.maxAttempts,
          updatedAt: optimisticUpdatedAt,
        }));
      }
      throw error;
    }
  }

  async function updateAutoRetryDispatchPendingOnFailure(sessionId: string, enabled: boolean) {
    const session = findSessionById(sessionId);
    const optimisticUpdatedAt = new Date().toISOString();
    const previous =
      session && !session.archivedAt ? session.autoRetryDispatchPendingOnFailure === true : null;
    if (previous !== null) {
      setPendingAutoRetryDispatchOverride(sessionId, enabled, Date.parse(optimisticUpdatedAt));
      updateSessionStatus(sessionId, current => ({
        ...current,
        autoRetryDispatchPendingOnFailure: enabled === true,
        updatedAt: optimisticUpdatedAt,
      }));
    }
    try {
      await sendCommand('set_ardpf', sessionId, {
        ardpf: enabled === true,
      });
    } catch (error) {
      clearPendingAutoRetryDispatchOverride(sessionId);
      if (previous !== null) {
        updateSessionStatus(sessionId, current => ({
          ...current,
          autoRetryDispatchPendingOnFailure: previous,
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
    replaceProjectSessions(projectId, reorderedWithOrder);

    try {
      await sendCommand('move', moving.id, {
        prv: previousSessionId,
        nxt: nextSessionId,
      });
    } catch (error) {
      replaceProjectSessions(projectId, sortSessions(original));
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

  async function importRemoteAttachment(projectId: string, sessionId: string, url: string) {
    const normalizedProjectId = String(projectId || '').trim();
    const normalizedSessionId = String(sessionId || '').trim();
    if (!normalizedProjectId || !normalizedSessionId || !String(url || '').trim()) {
      throw new Error('remote image URL is required');
    }

    const attachment = await webSessionApi.importRemoteAttachment(normalizedProjectId, url);
    updateDraft(normalizedProjectId, normalizedSessionId, draft => ({
      ...draft,
      attachments: [...draft.attachments, attachment],
      updatedAt: Date.now(),
    }));
    return attachment;
  }

  async function importClipboardAttachment(projectId: string, sessionId: string, source: string) {
    const normalizedProjectId = String(projectId || '').trim();
    const normalizedSessionId = String(sessionId || '').trim();
    if (!normalizedProjectId || !normalizedSessionId || !String(source || '').trim()) {
      throw new Error('clipboard image source is required');
    }

    const attachment = await webSessionApi.importClipboardAttachment(normalizedProjectId, source);
    updateDraft(normalizedProjectId, normalizedSessionId, draft => ({
      ...draft,
      attachments: [...draft.attachments, attachment],
      updatedAt: Date.now(),
    }));
    return attachment;
  }

  function removeDraftAttachment(projectId: string, sessionId: string, attachmentId: string) {
    updateDraft(projectId, sessionId, draft => ({
      ...draft,
      attachments: draft.attachments.filter(item => item.id !== attachmentId),
      updatedAt: Date.now(),
    }));
  }

  function applyWorkTimingItemPatches(
    sessionId: string,
    items: Array<{
      itemId: string;
      runId: string;
      runDurationMs: number;
      runOutcome: WebSessionWorkTimingOutcome;
    }>
  ) {
    const current = eventsBySession.value[sessionId];
    if (!current?.length || items.length === 0) {
      return;
    }
    const patches = new Map(items.map(item => [item.itemId, item]));
    let changed = false;
    const next = current.map(block => {
      const patch = patches.get(block.id);
      if (!patch) {
        return block;
      }
      changed = true;
      return {
        ...block,
        runId: patch.runId,
        runDurationMs: Math.max(0, Number(patch.runDurationMs) || 0),
        runOutcome: patch.runOutcome,
      };
    });
    if (changed) {
      eventsBySession.value = {
        ...eventsBySession.value,
        [sessionId]: next,
      };
    }
  }

  function calculateSessionWorkTiming(projectId: string, sessionId: string) {
    const normalizedProjectId = String(projectId || '').trim();
    const normalizedSessionId = String(sessionId || '').trim();
    if (!normalizedProjectId || !normalizedSessionId) {
      return Promise.reject(new Error('project and session are required'));
    }
    const requestKey = `${normalizedProjectId}:${normalizedSessionId}`;
    const existing = inFlightWorkTimingCalculations.get(requestKey);
    if (existing) {
      return existing;
    }
    const request = webSessionApi
      .calculateWorkTiming(normalizedProjectId, normalizedSessionId)
      .then(result => {
        upsertSession(result.session);
        applyWorkTimingItemPatches(normalizedSessionId, result.items);
        return result;
      })
      .finally(() => {
        if (inFlightWorkTimingCalculations.get(requestKey) === request) {
          inFlightWorkTimingCalculations.delete(requestKey);
        }
      });
    inFlightWorkTimingCalculations.set(requestKey, request);
    return request;
  }

  async function createSessionViaHttp(
    projectId: string,
    payload: {
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
      title?: string;
    },
    options?: CreateSessionOptions
  ) {
    const session = await webSessionApi.create(projectId, payload);
    upsertSession(session);
    if (options?.rememberActive !== false) {
      rememberActiveSession(projectId, session.id);
    }
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
    getPendingInputEditDraft,
    getPendingInputEditDrafts,
    getPendingUserInputDraft,
    setPendingInputEditDraft,
    setPendingUserInputDraft,
    clearPendingInputEditDraft,
    clearPendingInputEditDrafts,
    clearPendingUserInputDraft,
    setDraftText,
    getPendingInputs,
    getScheduledInputs,
    getHistoryMeta,
    fetchHistoryWindow,
    loadCommandGroupDetail,
    getSubAgents,
    isSessionSnapshotCurrent,
    getBlocks,
    getTimelineBlocks,
    getLatestEventSeq,
    loadSessions,
    reconcileRecentSessions,
    loadSessionCounts,
    loadArchivedSessions,
    invalidateArchivedSessions,
    setActiveSession,
    loadSessionSnapshot,
    createSession: createSessionViaHttp,
    editUserMessage,
    importSession,
    renameSession,
    archiveSession,
    unarchiveSession,
    syncSession,
    deleteSession,
    invalidateCleanedHistories,
    stageOutgoingMessage,
    discardOutgoingMessage,
    sendMessage,
    scheduleMessage,
    schedulePlanExecution,
    updateScheduledInput,
    dispatchScheduledInputNow,
    abortSession,
    compactSession,
    approveSession,
    rejectSession,
    answerUserInput,
    loadMoreHistory,
    trimInactiveSessionEvents,
    updateModel,
    updateClaudeRuntime,
    updateReasoningEffort,
    updateWorkflowMode,
    refreshGoal,
    setGoal,
    bootstrapGoal,
    pauseGoal,
    resumeGoal,
    clearGoal,
    updatePermissionLevel,
    updateAgent,
    updateActiveCallTimeout,
    updateAutoRetry,
    updateAutoRetryDispatchPendingOnFailure,
    moveSession,
    getPendingApproval,
    getPendingUserInput,
    getLiveState,
    uploadAttachments,
    uploadAttachment,
    importRemoteAttachment,
    importClipboardAttachment,
    removeDraftAttachment,
    calculateSessionWorkTiming,
    removePendingInput,
    updatePendingInput,
    pausePendingInput,
    resumePendingInput,
    reorderPendingInput,
    clearPendingInputs,
    removeScheduledInput,
    clearDraft,
    restoreDraft,
    moveDraft,
    openEventStream,
    setEventSessionFocus,
    sessionCounts: cachedCounts,
    emitter,
  };
});
