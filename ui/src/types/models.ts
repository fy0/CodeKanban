export interface Project {
  id: string;
  name: string;
  path: string;
  description: string | null;
  defaultBranch: string | null;
  worktreeBasePath: string | null;
  remoteUrl: string | null;
  hidePath: boolean;
  priority: number | null;
  lastSyncAt?: string | null;
  lastAccessedAt: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface Worktree {
  id: string;
  projectId: string;
  branchName: string;
  path: string;
  isMain: boolean;
  isBare?: boolean;
  headCommit: string | null;
  headCommitMessage?: string | null;
  headCommitDate: string | null;
  statusAhead: number | null;
  statusBehind: number | null;
  statusModified: number | null;
  statusStaged: number | null;
  statusUntracked: number | null;
  statusConflicts?: number | null;
  statusUpdatedAt: string | null;
  createdAt: string;
  updatedAt: string;
}

export type GitCapabilityMode = 'read_write' | 'read_only' | 'unavailable';

export interface GitOperationCapabilities {
  branchesRead: boolean;
  branchesWrite: boolean;
  status: boolean;
  diff: boolean;
  worktreesRead: boolean;
  worktreesWrite: boolean;
  commit: boolean;
  fastForwardMerge: boolean;
  merge: boolean;
  rebase: boolean;
  squash: boolean;
}

export type GitEngine = 'builtin' | 'system' | 'unavailable';

export type GitOperationEngines = {
  [K in keyof GitOperationCapabilities]: GitEngine;
};

export interface GitCapabilityReason {
  code: string;
  detail?: string;
}

export interface GitWorktreeCapabilityResult {
  id: string;
  operations: GitOperationCapabilities;
  engines: GitOperationEngines;
  reasons: GitCapabilityReason[];
}

export interface GitCapabilityResult {
  repository: boolean;
  mode: GitCapabilityMode;
  operations: GitOperationCapabilities;
  engines: GitOperationEngines;
  reasons: GitCapabilityReason[];
  worktrees: GitWorktreeCapabilityResult[];
}

export interface TerminalSession {
  id: string;
  projectId: string;
  worktreeId: string;
  workingDir: string;
  title: string;
  orderIndex?: number;
  createdAt: string;
  lastActive: string;
  status: 'starting' | 'running' | 'closed' | 'error';
  wsPath: string;
  wsUrl: string;
  rows: number;
  cols: number;
  // Process information
  processPid?: number;
  processStatus?: 'idle' | 'busy' | 'unknown';
  processHasChildren?: boolean;
  runningCommand?: string;
  metadataCapturedAt?: string;
  traffic?: {
    upstreamBytes: number;
    downstreamBytes: number;
    totalBytes: number;
    upstreamRecentBytes: number;
    downstreamRecentBytes: number;
    totalRecentBytes: number;
    upstreamAvgBytesPerSec: number;
    downstreamAvgBytesPerSec: number;
    totalAvgBytesPerSec: number;
    upstreamRecentAvgBytesPerSec: number;
    downstreamRecentAvgBytesPerSec: number;
    totalRecentAvgBytesPerSec: number;
  };
  // AI Assistant information
  aiAssistant?: {
    type: string;
    name: string;
    displayName: string;
    detected: boolean;
    command?: string;
  };
}

export interface ProjectAgentTrustStatus {
  projectId: string;
  agent: 'pi';
  projectPath: string;
  trustedPath?: string | null;
  trusted: boolean;
  trustedAt?: string | null;
  revokedAt?: string | null;
}

export interface AISessionMessage {
  timestamp: string;
  message: string;
}

export interface AISessionMessages {
  sessionId?: string;
  model?: string;
  cliVersion?: string;
  filePath?: string;
  messageCount: number;
  messages: AISessionMessage[];
}

// AI Session 摘要信息（用于列表显示）
export type AISessionType = 'claude_code' | 'codex' | 'pi';

export interface AISessionSummary {
  id: string;
  sessionId: string;
  type: AISessionType;
  model: string;
  title: string;
  sessionStartedAt: string;
  lastMessageAt?: string | null;
  messageCount: number;
  assistantMessageCount: number;
  filePath: string;
}

// 扫描阶段类型
export type ScanPhase = 'recent' | 'extended' | 'complete';

// 项目的 AI Sessions
export interface ProjectAISessions {
  hasClaudeCode: boolean;
  hasCodex: boolean;
  hasPi: boolean;
  claudeSessions: AISessionSummary[];
  codexSessions: AISessionSummary[];
  piSessions: AISessionSummary[];
  claudeScanPhase?: ScanPhase; // 扫描阶段：recent=24小时内, extended=1-15天, complete=完成
  codexScanPhase?: ScanPhase;
  piScanPhase?: ScanPhase;
  piBeforeCursor?: string;
}

// AI Session 对话内容
export interface ConversationMessage {
  role: 'user' | 'assistant';
  content: string;
  timestamp: string;
  kind?: string;
  toolUseId?: string;
  hasMore?: boolean;
  full?: string;
  images?: Array<{
    id: string;
    label: string;
    previewable: boolean;
    previewUrl?: string;
    mimeType?: string;
  }>;
}

export interface ConversationResponse {
  sessionId: string;
  title: string;
  messages: ConversationMessage[];
}

export interface ConversationWindowResponse extends ConversationResponse {
  total: number;
  totalUserMessages: number;
  userMessagesBeforeWindow: number;
  windowStart: number;
  windowEnd: number;
  hasMoreBefore: boolean;
  beforeCursor?: string;
}

export interface BranchInfo {
  name: string;
  isCurrent: boolean;
  isRemote: boolean;
  headCommit: string;
  headCommitMessage?: string | null;
  hasWorktree?: boolean;
}

export interface BranchListResult {
  local: BranchInfo[];
  remote: BranchInfo[];
}

export interface MergeResult {
  success: boolean;
  conflicts: string[];
  message: string;
}

export interface NotePad {
  id: string;
  projectId?: string | null;
  name: string;
  content: string;
  orderIndex: number;
  createdAt: string;
  updatedAt: string;
}

export interface WebSessionActiveCallTimeoutKindsConfig {
  useDefault: boolean;
  mcp: boolean;
  command: boolean;
  tool: boolean;
}

export interface WebSessionActiveCallTimeoutConfig {
  enabledMode: 'default' | 'on' | 'off';
  timeoutMode: 'default' | 'custom';
  customTimeoutSeconds: number;
  promptTemplate: string;
  callKinds: WebSessionActiveCallTimeoutKindsConfig;
}

export interface WebSessionAutoRetryDefaultsConfig {
  scope: 'network_only' | 'network_and_rate_limit' | 'all_failures';
  preset: 'gentle_stop' | 'aggressive_stop' | 'sustain_60s';
  maxAttempts: number;
  dispatchPendingOnFailure: boolean;
}

export interface DeveloperConfig {
  enableTerminalScrollback: boolean;
  enableTerminalStateSnapshot: boolean;
  webSessionCodexClientName: string;
  webSessionCodexClientTitle: string;
  webSessionCodexClientVersion: string;
  webSessionCodexDefaultModel: string;
  webSessionCodexContextWindow?: number;
  webSessionCodexDefaultReasoningEffort: WebSessionCodexDefaultReasoningEffort;
  webSessionCodexDefaultPermissionLevel: WebSessionCodexDefaultPermissionLevel;
  webSessionCodexDefaultSyncMode: 'default' | 'fast' | 'deep';
  webSessionClaudeDefaultModel: string;
  webSessionClaudeDefaultReasoningEffort: WebSessionAgentDefaultReasoningEffort;
  webSessionClaudeDefaultRuntime: WebSessionClaudeDefaultRuntime;
  webSessionPiDefaultModel: string;
  webSessionPiDefaultReasoningEffort: WebSessionAgentDefaultReasoningEffort;
  webSessionDevinDefaultModel: string;
  webSessionDevinDefaultReasoningEffort: WebSessionAgentDefaultReasoningEffort;
  webSessionDevinDefaultPermissionLevel: WebSessionAgentDefaultPermissionLevel;
  webSessionAutoRetryDefaults: WebSessionAutoRetryDefaultsConfig;
  webSessionActiveCallTimeout: WebSessionActiveCallTimeoutConfig;
}

export interface WorktreeConfig {
  globalBaseDir: string;
  globalDirNamePattern: string;
}

export interface ShellOption {
  id: string;
  name: string;
  command: string;
  available: boolean;
  description: string;
  warning?: string; // Optional warning key for i18n translation
}

export interface AvailableShellsResponse {
  platform: 'windows' | 'darwin' | 'linux';
  currentShell: string;
  defaultShell: string;
  options: ShellOption[];
  customAllowed: boolean;
}

export interface WebSessionUsage {
  inputTokens: number;
  cachedInputTokens: number;
  outputTokens: number;
  cost: number;
  // Devin-reported session quota totals (usage_update totalAcuCost/totalCreditCost)
  acuCost?: number;
  creditCost?: number;
}

export interface WebSessionContextEstimate {
  inputTokens: number;
  cachedInputTokens: number;
  outputTokens: number;
  usedTokens: number;
}

// Account-level Devin plan quota read from the Devin CLI's local user_status
// cache. Percent fields are 0-100 remaining values; reset fields are unix
// seconds.
export interface WebSessionDevinQuota {
  planName: string;
  dailyRemainingPercent: number;
  weeklyRemainingPercent: number;
  dailyResetAtUnix: number;
  weeklyResetAtUnix: number;
  fetchedAtUnix: number;
}

export type WebSessionContextEstimateMode =
  | 'cumulative_total'
  | 'since_compaction'
  | 'latest_turn_delta'
  | 'latest_token_count';

export type WebSessionContextWindowSource =
  | 'config'
  | 'default'
  | 'model_catalog'
  | 'session_usage'
  | 'unavailable';

export type WebSessionWorkTimingBackfillState =
  | 'pending'
  | 'complete'
  | 'partial'
  | 'unavailable'
  | 'failed';

export type WebSessionWorkTimingOutcome =
  | 'completed'
  | 'canceled'
  | 'failed'
  | 'timeout'
  | 'interrupted';

export interface WebSessionTokenSpeed {
  epoch: string;
  version: number;
  runId: string;
  sampleId: string;
  tokensPerSecond: number | null;
  estimated: boolean;
  generating: boolean;
  updatedAt: string;
}

export interface WebSessionWorkTimingCurrentRun {
  id: string;
  startedAt: string;
  pausedAt?: string | null;
  pausedDurationMs: number;
}

export interface WebSessionWorkTiming {
  completedDurationMs: number;
  currentRun?: WebSessionWorkTimingCurrentRun | null;
  backfillState: WebSessionWorkTimingBackfillState;
  backfillVersion: number;
}

export type WebSessionReasoningEffort =
  | 'default'
  | 'none'
  | 'minimal'
  | 'low'
  | 'medium'
  | 'high'
  | 'xhigh'
  | 'max'
  | 'ultra';

export type WebSessionCodexDefaultReasoningEffort = WebSessionReasoningEffort | 'model_default';

export type WebSessionAgentDefaultReasoningEffort = WebSessionCodexDefaultReasoningEffort;

export type WebSessionCodexDefaultPermissionLevel = 'default' | 'standard' | 'elevated' | 'yolo';

export type WebSessionAgentDefaultPermissionLevel = WebSessionCodexDefaultPermissionLevel;

export type WebSessionClaudeDefaultRuntime = 'default' | 'claude' | 'ccr';

export interface WebSessionCodexModelInfo {
  model: string;
  displayName: string;
  hidden?: boolean;
  defaultReasoningEffort: WebSessionReasoningEffort;
  supportedReasoningEfforts: WebSessionReasoningEffort[];
}

export type WebSessionGoalStatus =
  | 'active'
  | 'paused'
  | 'blocked'
  | 'usageLimited'
  | 'budgetLimited'
  | 'complete';

export interface WebSessionGoal {
  threadId: string;
  objective: string;
  status: WebSessionGoalStatus;
  tokenBudget?: number | null;
  tokensUsed: number;
  timeUsedSeconds: number;
  createdAt: string;
  updatedAt: string;
}

export type WebSessionAgent = 'claude' | 'codex' | 'pi' | 'devin';

export interface WebSessionAgentPermissionModeCapability {
  id: 'unrestricted' | 'approval' | 'sandbox' | string;
  available: boolean;
}

export interface WebSessionAgentCapability {
  installed: boolean;
  version?: string | null;
  supportsWebSession: boolean;
  supportsTree: boolean;
  supportsImages: boolean;
  supportsCompaction: boolean;
  supportsSteer: boolean;
  supportsFork?: boolean;
  supportsFollowUp: boolean;
  supportsGoal: boolean;
  supportsSubAgentRegistry: boolean;
  permissionModes: WebSessionAgentPermissionModeCapability[];
}

export interface WebSessionPiModelInfo {
  provider: string;
  id: string;
  name: string;
  reasoning: boolean;
  reasoningEfforts?: WebSessionReasoningEffort[];
  input: string[];
  contextWindow: number;
  maxTokens: number;
}

export interface WebSessionDevinModelInfo {
  model: string;
  displayName: string;
  family?: string;
  defaultReasoningEffort?: WebSessionReasoningEffort;
  supportedReasoningEfforts?: WebSessionReasoningEffort[];
  maxContextTokens?: number;
  maxOutputTokens?: number;
  costTier?: string;
  costSummary?: string;
  description?: string;
  recommended?: boolean;
  isNew?: boolean;
  isBeta?: boolean;
  isPromo?: boolean;
  supportsImages?: boolean;
}

export interface WebSessionCCRModelInfo {
  model: string;
  provider: string;
  displayName?: string;
}

export interface WebSessionRuntimeConfig {
  agents?: Partial<Record<WebSessionAgent, WebSessionAgentCapability>>;
  capabilitiesRefreshing?: boolean;
  model?: string;
  contextWindowTokens: number;
  compactLimitTokens: number;
  source: WebSessionContextWindowSource;
  models: WebSessionCodexModelInfo[];
  devinModels?: WebSessionDevinModelInfo[];
  piModels?: WebSessionPiModelInfo[];
  ccrModels?: WebSessionCCRModelInfo[];
  hasCodex: boolean;
  hasClaudeCode: boolean;
  hasDevin?: boolean;
  codexVersion?: string | null;
  devinVersion?: string | null;
  hasPi?: boolean;
  piVersion?: string | null;
  supportsPiWebSession?: boolean;
  piRpcCompatible?: boolean;
  piMinVersion?: string;
  piDiagnostics?: string;
  supportsWebSession: boolean;
  supportsDevinWebSession?: boolean;
  supportsDevinSessionFork?: boolean;
  supportsDevinSubAgents?: boolean;
  webSessionMinCodexVersion: string;
  supportsMultiAgentV2?: boolean;
  multiAgentV2MinCodexVersion?: string;
  supportsGoalMode: boolean;
  goalModeMinCodexVersion: string;
}

export type CodexSkillSource = 'user' | 'system' | 'bundled';

export interface CodexSkillSummary {
  name: string;
  displayName: string;
  description: string;
  defaultPrompt: string;
  source: CodexSkillSource;
}

export interface WebSessionSummary {
  id: string;
  revision?: string;
  attentionRevision?: string;
  historyEpoch?: string;
  eventCursor?: string;
  projectId: string;
  worktreeId?: string | null;
  orderIndex: number;
  agent: WebSessionAgent;
  claudeRuntime?: 'claude' | 'ccr';
  backend?: 'legacy_exec' | 'codex_app_server' | 'pi_rpc' | 'devin_acp';
  title: string;
  model: string;
  reasoningEffort: WebSessionReasoningEffort;
  codexFastMode?: boolean;
  workflowMode: 'default' | 'plan';
  permissionLevel: 'default' | 'elevated' | 'yolo';
  activeCallTimeoutEnabled?: boolean;
  contextWindowSetting?: number;
  appliedContextWindowSetting?: number | null;
  codexModelMetadataFallback?: boolean;
  autoRetryEnabled: boolean;
  autoRetryPolicyMode: 'default' | 'custom';
  autoRetryScope: 'network_only' | 'network_and_rate_limit' | 'all_failures';
  autoRetryPreset: 'gentle_stop' | 'aggressive_stop' | 'sustain_60s';
  autoRetryMaxAttempts?: number;
  autoRetryDispatchPendingOnFailure: boolean;
  cwd: string;
  nativeSessionId?: string | null;
  nativeLeafId?: string | null;
  sourceRevision?: string | null;
  cyberPolicyFlagged?: boolean;
  hasScheduledInput?: boolean;
  hasScheduledPlanExecution?: boolean;
  status: 'idle' | 'running' | 'waiting_approval' | 'done' | 'err' | 'aborting';
  assistantState?:
    | 'working'
    | 'waiting_approval'
    | 'waiting_input'
    | 'waiting_plan_approval'
    | null;
  hasUnread: boolean;
  archivedAt?: string | null;
  activityAt: string;
  statusUpdatedAt?: string | null;
  lastMessageAt?: string | null;
  assistantStateUpdatedAt?: string | null;
  sourceKind: string;
  syncState: 'fresh' | 'stale' | 'missing' | 'syncing' | 'error';
  lastSyncMode?: 'fast' | 'deep' | null;
  sourceCreatedAt?: string | null;
  sourceUpdatedAt?: string | null;
  lastSyncedAt?: string | null;
  threadPath?: string | null;
  threadPreview?: string | null;
  turnCount: number;
  itemCount: number;
  syncError?: string | null;
  createdAt: string;
  updatedAt: string;
  usage: WebSessionUsage;
  devinQuota?: WebSessionDevinQuota | null;
  latestTurnUsage?: WebSessionContextEstimate | null;
  contextEstimate: WebSessionContextEstimate;
  contextEstimateMode: WebSessionContextEstimateMode;
  lastContextCompactionAt?: string | null;
  contextWindowTokens?: number | null;
  contextWindowSource: WebSessionContextWindowSource;
  workTiming?: WebSessionWorkTiming;
  tokenSpeed?: WebSessionTokenSpeed | null;
  goal?: WebSessionGoal | null;
  searchMatchSources?: Array<'title' | 'body'>;
}

export interface WebSessionAttachment {
  id: string;
  name: string;
  mime: string;
  size: number;
  path: string;
  createdAt: string;
}
