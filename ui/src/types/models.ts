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
  headCommit: string | null;
  headCommitMessage?: string | null;
  headCommitDate: string | null;
  statusAhead: number | null;
  statusBehind: number | null;
  statusModified: number | null;
  statusStaged: number | null;
  statusUntracked: number | null;
  statusUpdatedAt: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface Task {
  id: string;
  projectId: string;
  worktreeId?: string | null;
  branchName: string; // 关联的分支名称，即使worktree被删除也能显示
  title: string;
  description: string;
  status: 'todo' | 'in_progress' | 'done' | 'archived';
  priority: number;
  orderIndex: number;
  tags: string[];
  dueDate?: string | null;
  completedAt?: string | null;
  createdAt: string;
  updatedAt: string;
  worktree?: Worktree;
}

export interface TaskComment {
  id: string;
  taskId: string;
  content: string;
  createdAt: string;
  updatedAt: string;
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
    state?: string;
    stateUpdatedAt?: string;
    interrupted?: boolean;
    stats?: {
      thinkingDuration: number;
      executingDuration: number;
      waitingApprovalDuration: number;
      waitingInputDuration: number;
      currentStateDuration: number;
    };
  };
  taskId?: string;
  aiSessionId?: string;
  aiAssistantRecentInput?: string;
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
export interface AISessionSummary {
  id: string;
  sessionId: string;
  type: 'claude_code' | 'codex';
  model: string;
  title: string;
  sessionStartedAt: string;
  lastMessageAt?: string | null;
  messageCount: number;
  filePath: string;
}

// 扫描阶段类型
export type ScanPhase = 'recent' | 'extended' | 'complete';

// 项目的 AI Sessions
export interface ProjectAISessions {
  hasClaudeCode: boolean;
  hasCodex: boolean;
  claudeSessions: AISessionSummary[];
  codexSessions: AISessionSummary[];
  claudeScanPhase?: ScanPhase; // 扫描阶段：recent=24小时内, extended=1-15天, complete=完成
  codexScanPhase?: ScanPhase;
}

// 任务关联的 AI Session（包含详情）
export interface TaskAISessionWithDetails {
  id: string;
  taskId: string;
  sessionId: string;
  aiSessionDbId: string;
  type: 'claude_code' | 'codex';
  model: string;
  title: string;
  sessionStartedAt: string;
  lastMessageAt?: string | null;
  messageCount: number;
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

export interface AIAssistantStatusConfig {
  claudeCode: boolean;
  codex: boolean;
  qwenCode: boolean;
  gemini: boolean;
  cursor: boolean;
  copilot: boolean;
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

export interface DeveloperConfig {
  enableTerminalScrollback: boolean;
  renameSessionTitleEachCommand: boolean;
  enableTerminalStateSnapshot: boolean;
  webSessionCodexDefaultSyncMode: 'fast' | 'deep';
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
}

export interface WebSessionContextEstimate {
  inputTokens: number;
  cachedInputTokens: number;
  outputTokens: number;
  usedTokens: number;
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

export interface WebSessionCodexModelInfo {
  model: string;
  displayName: string;
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

export interface WebSessionCodexRuntimeConfig {
  model?: string;
  contextWindowTokens: number;
  compactLimitTokens: number;
  source: WebSessionContextWindowSource;
  models: WebSessionCodexModelInfo[];
  hasCodex: boolean;
  hasClaudeCode: boolean;
  codexVersion?: string | null;
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
  projectId: string;
  worktreeId?: string | null;
  orderIndex: number;
  agent: 'claude' | 'codex';
  claudeRuntime?: 'claude' | 'ccr';
  title: string;
  model: string;
  reasoningEffort: WebSessionReasoningEffort;
  workflowMode: 'default' | 'plan';
  permissionLevel: 'default' | 'elevated' | 'yolo';
  activeCallTimeoutEnabled?: boolean;
  autoRetryEnabled: boolean;
  autoRetryScope: 'network_only' | 'network_and_rate_limit' | 'all_failures';
  autoRetryPreset: 'gentle_stop' | 'aggressive_stop' | 'sustain_60s';
  cwd: string;
  nativeSessionId?: string | null;
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
  latestTurnUsage?: WebSessionContextEstimate | null;
  contextEstimate: WebSessionContextEstimate;
  contextEstimateMode: WebSessionContextEstimateMode;
  lastContextCompactionAt?: string | null;
  contextWindowTokens?: number | null;
  contextWindowSource: WebSessionContextWindowSource;
  goal?: WebSessionGoal | null;
}

export interface WebSessionAttachment {
  id: string;
  name: string;
  mime: string;
  size: number;
  path: string;
  createdAt: string;
}
