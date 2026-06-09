package websession

import "time"

type Agent string

const (
	AgentClaude Agent = "claude"
	AgentCodex  Agent = "codex"
)

type ClaudeRuntime string

const (
	ClaudeRuntimeNative ClaudeRuntime = "claude"
	ClaudeRuntimeCCR    ClaudeRuntime = "ccr"
)

type SessionBackend string

const (
	SessionBackendLegacyExec     SessionBackend = "legacy_exec"
	SessionBackendCodexAppServer SessionBackend = "codex_app_server"
)

type WorkflowMode string

const (
	WorkflowModeDefault WorkflowMode = "default"
	WorkflowModePlan    WorkflowMode = "plan"
)

type PermissionLevel string

const (
	PermissionLevelDefault  PermissionLevel = "default"
	PermissionLevelElevated PermissionLevel = "elevated"
	PermissionLevelYolo     PermissionLevel = "yolo"
)

type AutoRetryScope string

const (
	AutoRetryScopeNetworkOnly         AutoRetryScope = "network_only"
	AutoRetryScopeNetworkAndRateLimit AutoRetryScope = "network_and_rate_limit"
	AutoRetryScopeAllFailures         AutoRetryScope = "all_failures"
)

type AutoRetryPreset string

const (
	AutoRetryPresetGentleStop     AutoRetryPreset = "gentle_stop"
	AutoRetryPresetAggressiveStop AutoRetryPreset = "aggressive_stop"
	AutoRetryPresetSustain60s     AutoRetryPreset = "sustain_60s"
)

type Status string

const (
	StatusIdle            Status = "idle"
	StatusRunning         Status = "running"
	StatusWaitingApproval Status = "waiting_approval"
	StatusDone            Status = "done"
	StatusError           Status = "err"
	StatusAborting        Status = "aborting"
)

type AssistantState string

const (
	AssistantStateNone                AssistantState = ""
	AssistantStateWorking             AssistantState = "working"
	AssistantStateWaitingApproval     AssistantState = "waiting_approval"
	AssistantStateWaitingInput        AssistantState = "waiting_input"
	AssistantStateWaitingPlanApproval AssistantState = "waiting_plan_approval"
)

type ReasoningEffort string

const (
	ReasoningEffortDefault ReasoningEffort = "default"
	ReasoningEffortNone    ReasoningEffort = "none"
	ReasoningEffortLow     ReasoningEffort = "low"
	ReasoningEffortMedium  ReasoningEffort = "medium"
	ReasoningEffortHigh    ReasoningEffort = "high"
	ReasoningEffortXHigh   ReasoningEffort = "xhigh"
)

type Usage struct {
	InputTokens       int64   `json:"inputTokens"`
	CachedInputTokens int64   `json:"cachedInputTokens"`
	OutputTokens      int64   `json:"outputTokens"`
	Cost              float64 `json:"cost"`
}

type ContextEstimate struct {
	InputTokens       int64 `json:"inputTokens"`
	CachedInputTokens int64 `json:"cachedInputTokens"`
	OutputTokens      int64 `json:"outputTokens"`
	UsedTokens        int64 `json:"usedTokens"`
}

type ContextEstimateMode string

const (
	ContextEstimateModeCumulativeTotal  ContextEstimateMode = "cumulative_total"
	ContextEstimateModeSinceCompaction  ContextEstimateMode = "since_compaction"
	ContextEstimateModeLatestTurnDelta  ContextEstimateMode = "latest_turn_delta"
	ContextEstimateModeLatestTokenCount ContextEstimateMode = "latest_token_count"
)

type ContextWindowSource string

const (
	ContextWindowSourceConfig       ContextWindowSource = "config"
	ContextWindowSourceDefault      ContextWindowSource = "default"
	ContextWindowSourceSessionUsage ContextWindowSource = "session_usage"
	ContextWindowSourceUnavailable  ContextWindowSource = "unavailable"
)

type GoalStatus string

const (
	GoalStatusActive       GoalStatus = "active"
	GoalStatusPaused       GoalStatus = "paused"
	GoalStatusBlocked      GoalStatus = "blocked"
	GoalStatusUsageLimited GoalStatus = "usageLimited"
	GoalStatusBudgetLimit  GoalStatus = "budgetLimited"
	GoalStatusComplete     GoalStatus = "complete"
)

type SyncState string

const (
	SyncStateFresh   SyncState = "fresh"
	SyncStateStale   SyncState = "stale"
	SyncStateMissing SyncState = "missing"
	SyncStateSyncing SyncState = "syncing"
	SyncStateError   SyncState = "error"
)

type SyncMode string

const (
	SyncModeFast SyncMode = "fast"
	SyncModeDeep SyncMode = "deep"
)

type PendingInputMode string

const (
	PendingInputModeRedirect PendingInputMode = "redirect"
	PendingInputModeQueue    PendingInputMode = "queue"
)

type ScheduledInputMode string

const (
	ScheduledInputModeSend      ScheduledInputMode = "send"
	ScheduledInputModeInterrupt ScheduledInputMode = "interrupt"
	ScheduledInputModeRedirect  ScheduledInputMode = "redirect"
	ScheduledInputModeQueue     ScheduledInputMode = "queue"
)

type ScheduledInputStatus string

const (
	ScheduledInputStatusScheduled  ScheduledInputStatus = "scheduled"
	ScheduledInputStatusDispatched ScheduledInputStatus = "dispatched"
	ScheduledInputStatusCanceled   ScheduledInputStatus = "canceled"
	ScheduledInputStatusFailed     ScheduledInputStatus = "failed"
)

type SessionSummary struct {
	ID                       string              `json:"id"`
	ProjectID                string              `json:"projectId"`
	WorktreeID               *string             `json:"worktreeId,omitempty"`
	OrderIndex               float64             `json:"orderIndex"`
	Agent                    Agent               `json:"agent"`
	ClaudeRuntime            ClaudeRuntime       `json:"claudeRuntime"`
	Title                    string              `json:"title"`
	Model                    string              `json:"model"`
	ReasoningEffort          ReasoningEffort     `json:"reasoningEffort"`
	WorkflowMode             WorkflowMode        `json:"workflowMode"`
	PermissionLevel          PermissionLevel     `json:"permissionLevel"`
	ActiveCallTimeoutEnabled bool                `json:"activeCallTimeoutEnabled"`
	AutoRetryEnabled         bool                `json:"autoRetryEnabled"`
	AutoRetryScope           AutoRetryScope      `json:"autoRetryScope"`
	AutoRetryPreset          AutoRetryPreset     `json:"autoRetryPreset"`
	Cwd                      string              `json:"cwd"`
	NativeSessionID          *string             `json:"nativeSessionId,omitempty"`
	Status                   Status              `json:"status"`
	AssistantState           AssistantState      `json:"assistantState,omitempty"`
	HasUnread                bool                `json:"hasUnread"`
	ArchivedAt               *time.Time          `json:"archivedAt,omitempty"`
	ActivityAt               time.Time           `json:"activityAt"`
	StatusUpdatedAt          *time.Time          `json:"statusUpdatedAt,omitempty"`
	LastMessageAt            *time.Time          `json:"lastMessageAt,omitempty"`
	AssistantStateUpdatedAt  *time.Time          `json:"assistantStateUpdatedAt,omitempty"`
	SourceKind               string              `json:"sourceKind"`
	SyncState                SyncState           `json:"syncState"`
	LastSyncMode             SyncMode            `json:"lastSyncMode,omitempty"`
	SourceCreatedAt          *time.Time          `json:"sourceCreatedAt,omitempty"`
	SourceUpdatedAt          *time.Time          `json:"sourceUpdatedAt,omitempty"`
	LastSyncedAt             *time.Time          `json:"lastSyncedAt,omitempty"`
	ThreadPath               *string             `json:"threadPath,omitempty"`
	ThreadPreview            *string             `json:"threadPreview,omitempty"`
	TurnCount                int                 `json:"turnCount"`
	ItemCount                int                 `json:"itemCount"`
	SyncError                *string             `json:"syncError,omitempty"`
	CreatedAt                time.Time           `json:"createdAt"`
	UpdatedAt                time.Time           `json:"updatedAt"`
	Usage                    Usage               `json:"usage"`
	LatestTurnUsage          ContextEstimate     `json:"latestTurnUsage"`
	ContextEstimate          ContextEstimate     `json:"contextEstimate"`
	ContextEstimateMode      ContextEstimateMode `json:"contextEstimateMode"`
	LastContextCompactionAt  *time.Time          `json:"lastContextCompactionAt,omitempty"`
	ContextWindowTokens      *int64              `json:"contextWindowTokens,omitempty"`
	ContextWindowSource      ContextWindowSource `json:"contextWindowSource"`
	Goal                     *SessionGoal        `json:"goal,omitempty"`
}

type SessionGoal struct {
	ThreadID        string     `json:"threadId"`
	Objective       string     `json:"objective"`
	Status          GoalStatus `json:"status"`
	TokenBudget     *int64     `json:"tokenBudget,omitempty"`
	TokensUsed      int64      `json:"tokensUsed"`
	TimeUsedSeconds int64      `json:"timeUsedSeconds"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

type ArchivedQueryResult struct {
	Items      []SessionSummary `json:"items"`
	Total      int              `json:"total"`
	HasMore    bool             `json:"hasMore"`
	NextOffset int              `json:"nextOffset"`
}

type Attachment struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Mime      string    `json:"mime"`
	Size      int64     `json:"size"`
	Path      string    `json:"path"`
	CreatedAt time.Time `json:"createdAt"`
}

type HistoryToolCommandGroup struct {
	ID           string `json:"id"`
	Count        int    `json:"count"`
	FirstSeq     int64  `json:"firstSeq,omitempty"`
	LastSeq      int64  `json:"lastSeq,omitempty"`
	LatestToolID string `json:"latestToolId,omitempty"`
	Compacted    bool   `json:"compacted,omitempty"`
}

type HistoryTool struct {
	ID           string                   `json:"id"`
	Name         string                   `json:"name"`
	Kind         string                   `json:"kind,omitempty"`
	Input        any                      `json:"input,omitempty"`
	Output       string                   `json:"output,omitempty"`
	Status       string                   `json:"status"`
	Meta         map[string]any           `json:"meta,omitempty"`
	CommandGroup *HistoryToolCommandGroup `json:"commandGroup,omitempty"`
}

type HistoryAnswerEntry struct {
	ID     string   `json:"id"`
	Label  string   `json:"label"`
	Values []string `json:"values"`
	Masked bool     `json:"masked,omitempty"`
}

type HistoryDetail struct {
	Type      string                `json:"type"`
	Prompt    string                `json:"prompt,omitempty"`
	Questions []toolRequestQuestion `json:"questions,omitempty"`
	Answers   []HistoryAnswerEntry  `json:"answers,omitempty"`
	Action    string                `json:"action,omitempty"`
}

type HistoryAttachment struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Mime string `json:"mime,omitempty"`
	Size int64  `json:"size,omitempty"`
	Path string `json:"path,omitempty"`
}

type HistoryItem struct {
	ID           string              `json:"id"`
	SourceTurnID *string             `json:"sourceTurnId,omitempty"`
	SourceItemID *string             `json:"sourceItemId,omitempty"`
	OrderIndex   int64               `json:"orderIndex"`
	Kind         string              `json:"kind"`
	ItemType     string              `json:"itemType"`
	Text         string              `json:"text"`
	Timestamp    *time.Time          `json:"timestamp,omitempty"`
	ObservedAt   *time.Time          `json:"observedAt,omitempty"`
	Attachments  []HistoryAttachment `json:"attachments,omitempty"`
	Tool         *HistoryTool        `json:"tool,omitempty"`
	Level        string              `json:"level,omitempty"`
	Done         bool                `json:"done,omitempty"`
	Detail       *HistoryDetail      `json:"detail,omitempty"`
	Payload      map[string]any      `json:"payload,omitempty"`
}

type HistoryWindow struct {
	Events       []Event       `json:"events,omitempty"`
	Items        []HistoryItem `json:"items"`
	HasMore      bool          `json:"hasMore"`
	BeforeCursor string        `json:"beforeCursor,omitempty"`
	Total        int           `json:"total"`
}

type PendingInput struct {
	ID            string           `json:"id"`
	Mode          PendingInputMode `json:"mode"`
	Text          string           `json:"text"`
	AttachmentIDs []string         `json:"attachmentIds"`
	CreatedAt     time.Time        `json:"createdAt"`
}

type ScheduledInput struct {
	ID            string               `json:"id"`
	Mode          ScheduledInputMode   `json:"mode"`
	Text          string               `json:"text"`
	AttachmentIDs []string             `json:"attachmentIds"`
	ScheduledFor  time.Time            `json:"scheduledFor"`
	Status        ScheduledInputStatus `json:"status"`
	CreatedAt     time.Time            `json:"createdAt"`
	UpdatedAt     time.Time            `json:"updatedAt"`
	SentAt        *time.Time           `json:"sentAt,omitempty"`
	CanceledAt    *time.Time           `json:"canceledAt,omitempty"`
}

type PendingUserInput struct {
	ItemID      string                `json:"itemId"`
	Prompt      string                `json:"prompt,omitempty"`
	Questions   []toolRequestQuestion `json:"questions,omitempty"`
	RequestedAt *time.Time            `json:"requestedAt,omitempty"`
}

type SessionSnapshot struct {
	Session          SessionSummary    `json:"session"`
	History          HistoryWindow     `json:"history"`
	PendingInputs    []PendingInput    `json:"pendingInputs"`
	ScheduledInputs  []ScheduledInput  `json:"scheduledInputs"`
	PendingUserInput *PendingUserInput `json:"pendingUserInput,omitempty"`
}

type ImportResult struct {
	Session         SessionSummary   `json:"session"`
	History         HistoryWindow    `json:"history"`
	PendingInputs   []PendingInput   `json:"pendingInputs"`
	ScheduledInputs []ScheduledInput `json:"scheduledInputs"`
	Created         bool             `json:"created"`
	Reused          bool             `json:"reused"`
	Synced          bool             `json:"synced"`
}

type ImportSourceSummary struct {
	AISessionID           string          `json:"aiSessionId"`
	SessionID             string          `json:"sessionId"`
	Model                 string          `json:"model,omitempty"`
	Title                 string          `json:"title,omitempty"`
	SessionStartedAt      time.Time       `json:"sessionStartedAt"`
	LastMessageAt         *time.Time      `json:"lastMessageAt,omitempty"`
	MessageCount          int             `json:"messageCount"`
	AssistantMessageCount int             `json:"assistantMessageCount"`
	FilePath              string          `json:"filePath"`
	Duplicate             bool            `json:"duplicate"`
	ExistingSession       *SessionSummary `json:"existingSession,omitempty"`
}

type ImportSourceList struct {
	Items     []ImportSourceSummary `json:"items"`
	ScanPhase string                `json:"scanPhase,omitempty"`
}

type Event struct {
	ID        string         `json:"id"`
	Seq       int64          `json:"seq"`
	Type      string         `json:"type"`
	RunID     string         `json:"runId,omitempty"`
	ParentID  string         `json:"parentId,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
	Payload   map[string]any `json:"payload,omitempty"`
}

type CommandExecutionGroupItem struct {
	ToolID      string    `json:"toolId"`
	Kind        string    `json:"kind"`
	Title       string    `json:"title"`
	Summary     string    `json:"summary"`
	Command     string    `json:"command"`
	Input       any       `json:"input,omitempty"`
	Output      string    `json:"output,omitempty"`
	Status      string    `json:"status"`
	Timestamp   time.Time `json:"timestamp"`
	StartedAt   time.Time `json:"startedAt,omitempty"`
	CompletedAt time.Time `json:"completedAt,omitempty"`
}

type CommandExecutionGroupDetail struct {
	GroupID    string                      `json:"groupId"`
	Kind       string                      `json:"kind"`
	Title      string                      `json:"title"`
	Summary    string                      `json:"summary"`
	Count      int                         `json:"count"`
	FirstSeq   int64                       `json:"firstSeq"`
	LastSeq    int64                       `json:"lastSeq"`
	Status     string                      `json:"status"`
	LatestTool string                      `json:"latestToolId,omitempty"`
	Items      []CommandExecutionGroupItem `json:"items"`
}

type CreateParams struct {
	ProjectID                string
	WorktreeID               string
	Agent                    Agent
	ClaudeRuntime            ClaudeRuntime
	Backend                  SessionBackend
	Model                    string
	ReasoningEffort          ReasoningEffort
	WorkflowMode             WorkflowMode
	PermissionLevel          PermissionLevel
	ActiveCallTimeoutEnabled *bool
	AutoRetryEnabled         bool
	AutoRetryScope           AutoRetryScope
	AutoRetryPreset          AutoRetryPreset
	Title                    string
}
