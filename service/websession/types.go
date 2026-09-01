package websession

import "time"

type Agent string

const (
	AgentClaude Agent = "claude"
	AgentCodex  Agent = "codex"
	AgentPi     Agent = "pi"
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
	SessionBackendPiRPC          SessionBackend = "pi_rpc"
)

type CodexAppServerState string

const (
	CodexAppServerInactive    CodexAppServerState = "inactive"
	CodexAppServerStarting    CodexAppServerState = "starting"
	CodexAppServerActive      CodexAppServerState = "active"
	CodexAppServerDraining    CodexAppServerState = "draining"
	CodexAppServerTerminating CodexAppServerState = "terminating"
)

type CodexAppServerRuntime struct {
	State          CodexAppServerState `json:"state"`
	RunID          string              `json:"runId,omitempty"`
	ProcessRootPID int                 `json:"processRootPid,omitempty"`
	CanTerminate   bool                `json:"canTerminate"`
}

type WorkflowMode string

const (
	WorkflowModeDefault WorkflowMode = "default"
	WorkflowModePlan    WorkflowMode = "plan"
)

type SessionStartSource string

const (
	SessionStartSourceStartup SessionStartSource = "startup"
	SessionStartSourceClear   SessionStartSource = "clear"
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

type AutoRetryPolicyMode string

const (
	AutoRetryPolicyModeDefault AutoRetryPolicyMode = "default"
	AutoRetryPolicyModeCustom  AutoRetryPolicyMode = "custom"
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
	ReasoningEffortMinimal ReasoningEffort = "minimal"
	ReasoningEffortLow     ReasoningEffort = "low"
	ReasoningEffortMedium  ReasoningEffort = "medium"
	ReasoningEffortHigh    ReasoningEffort = "high"
	ReasoningEffortXHigh   ReasoningEffort = "xhigh"
	ReasoningEffortMax     ReasoningEffort = "max"
	ReasoningEffortUltra   ReasoningEffort = "ultra"
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
	ContextWindowSourceModelCatalog ContextWindowSource = "model_catalog"
	ContextWindowSourceSessionUsage ContextWindowSource = "session_usage"
	ContextWindowSourceUnavailable  ContextWindowSource = "unavailable"
)

type WorkTimingBackfillState string

const (
	WorkTimingBackfillPending     WorkTimingBackfillState = "pending"
	WorkTimingBackfillComplete    WorkTimingBackfillState = "complete"
	WorkTimingBackfillPartial     WorkTimingBackfillState = "partial"
	WorkTimingBackfillUnavailable WorkTimingBackfillState = "unavailable"
	WorkTimingBackfillFailed      WorkTimingBackfillState = "failed"
)

type WorkTimingOutcome string

const (
	WorkTimingOutcomeCompleted   WorkTimingOutcome = "completed"
	WorkTimingOutcomeCanceled    WorkTimingOutcome = "canceled"
	WorkTimingOutcomeFailed      WorkTimingOutcome = "failed"
	WorkTimingOutcomeTimeout     WorkTimingOutcome = "timeout"
	WorkTimingOutcomeInterrupted WorkTimingOutcome = "interrupted"
)

type WorkTimingCurrentRun struct {
	ID               string     `json:"id"`
	StartedAt        time.Time  `json:"startedAt"`
	PausedAt         *time.Time `json:"pausedAt,omitempty"`
	PausedDurationMs int64      `json:"pausedDurationMs"`
}

type WorkTiming struct {
	CompletedDurationMs int64                   `json:"completedDurationMs"`
	CurrentRun          *WorkTimingCurrentRun   `json:"currentRun,omitempty"`
	BackfillState       WorkTimingBackfillState `json:"backfillState"`
	BackfillVersion     int                     `json:"backfillVersion"`
}

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

type SessionSearchMatchSource string

const (
	SessionSearchMatchTitle SessionSearchMatchSource = "title"
	SessionSearchMatchBody  SessionSearchMatchSource = "body"
)

type PendingInputMode string

const (
	PendingInputModeRedirect PendingInputMode = "redirect"
	PendingInputModeQueue    PendingInputMode = "queue"
)

type PendingInputStatus string

const (
	PendingInputStatusRetrying   PendingInputStatus = "retrying"
	PendingInputStatusPersisting PendingInputStatus = "persisting"
	PendingInputStatusFailed     PendingInputStatus = "failed"
)

type ScheduledInputMode string

const (
	ScheduledInputModeSend      ScheduledInputMode = "send"
	ScheduledInputModeInterrupt ScheduledInputMode = "interrupt"
	ScheduledInputModeRedirect  ScheduledInputMode = "redirect"
	ScheduledInputModeQueue     ScheduledInputMode = "queue"
)

type ScheduledInputAction string

const (
	ScheduledInputActionMessage     ScheduledInputAction = "message"
	ScheduledInputActionExecutePlan ScheduledInputAction = "execute_plan"
)

type ScheduledInputStatus string

const (
	ScheduledInputStatusScheduled  ScheduledInputStatus = "scheduled"
	ScheduledInputStatusDispatched ScheduledInputStatus = "dispatched"
	ScheduledInputStatusCanceled   ScheduledInputStatus = "canceled"
	ScheduledInputStatusFailed     ScheduledInputStatus = "failed"
	ScheduledInputStatusExpired    ScheduledInputStatus = "expired"
)

type ScheduledInputScheduleKind string

const (
	ScheduledInputScheduleAtTime   ScheduledInputScheduleKind = "at_time"
	ScheduledInputScheduleWhenIdle ScheduledInputScheduleKind = "when_idle"
)

type ScheduledInputDependencyStatus string

const (
	ScheduledInputDependencyNone      ScheduledInputDependencyStatus = "none"
	ScheduledInputDependencyWaiting   ScheduledInputDependencyStatus = "waiting"
	ScheduledInputDependencySatisfied ScheduledInputDependencyStatus = "satisfied"
	ScheduledInputDependencyFailed    ScheduledInputDependencyStatus = "failed"
	ScheduledInputDependencyCanceled  ScheduledInputDependencyStatus = "canceled"
	ScheduledInputDependencyExpired   ScheduledInputDependencyStatus = "expired"
	ScheduledInputDependencyMissing   ScheduledInputDependencyStatus = "missing"
)

type ScheduledInputBlockingReason string

const (
	ScheduledInputBlockedGitDirty             ScheduledInputBlockingReason = "git_dirty"
	ScheduledInputBlockedGitUnavailable       ScheduledInputBlockingReason = "git_unavailable"
	ScheduledInputBlockedNonPlanSessionActive ScheduledInputBlockingReason = "non_plan_session_active"
)

type SessionSummary struct {
	ID                                string                     `json:"id"`
	Revision                          string                     `json:"revision"`
	ProjectID                         string                     `json:"projectId"`
	WorktreeID                        *string                    `json:"worktreeId,omitempty"`
	OrderIndex                        float64                    `json:"orderIndex"`
	Agent                             Agent                      `json:"agent"`
	ClaudeRuntime                     ClaudeRuntime              `json:"claudeRuntime"`
	Backend                           SessionBackend             `json:"backend"`
	Title                             string                     `json:"title"`
	Model                             string                     `json:"model"`
	ReasoningEffort                   ReasoningEffort            `json:"reasoningEffort"`
	WorkflowMode                      WorkflowMode               `json:"workflowMode"`
	PermissionLevel                   PermissionLevel            `json:"permissionLevel"`
	ActiveCallTimeoutEnabled          bool                       `json:"activeCallTimeoutEnabled"`
	AutoRetryEnabled                  bool                       `json:"autoRetryEnabled"`
	AutoRetryPolicyMode               AutoRetryPolicyMode        `json:"autoRetryPolicyMode"`
	AutoRetryScope                    AutoRetryScope             `json:"autoRetryScope"`
	AutoRetryPreset                   AutoRetryPreset            `json:"autoRetryPreset"`
	AutoRetryMaxAttempts              int                        `json:"autoRetryMaxAttempts"`
	AutoRetryDispatchPendingOnFailure bool                       `json:"autoRetryDispatchPendingOnFailure"`
	Cwd                               string                     `json:"cwd"`
	NativeSessionID                   *string                    `json:"nativeSessionId,omitempty"`
	NativeLeafID                      *string                    `json:"nativeLeafId,omitempty"`
	SourceRevision                    *string                    `json:"sourceRevision,omitempty"`
	CyberPolicyFlagged                bool                       `json:"cyberPolicyFlagged"`
	HasScheduledPlanExecution         bool                       `json:"hasScheduledPlanExecution,omitempty"`
	Status                            Status                     `json:"status"`
	AssistantState                    AssistantState             `json:"assistantState,omitempty"`
	HasUnread                         bool                       `json:"hasUnread"`
	AttentionRevision                 string                     `json:"attentionRevision"`
	HistoryEpoch                      string                     `json:"historyEpoch"`
	EventCursor                       string                     `json:"eventCursor"`
	ArchivedAt                        *time.Time                 `json:"archivedAt,omitempty"`
	ActivityAt                        time.Time                  `json:"activityAt"`
	StatusUpdatedAt                   *time.Time                 `json:"statusUpdatedAt,omitempty"`
	LastMessageAt                     *time.Time                 `json:"lastMessageAt,omitempty"`
	AssistantStateUpdatedAt           *time.Time                 `json:"assistantStateUpdatedAt,omitempty"`
	SourceKind                        string                     `json:"sourceKind"`
	SyncState                         SyncState                  `json:"syncState"`
	LastSyncMode                      SyncMode                   `json:"lastSyncMode,omitempty"`
	SourceCreatedAt                   *time.Time                 `json:"sourceCreatedAt,omitempty"`
	SourceUpdatedAt                   *time.Time                 `json:"sourceUpdatedAt,omitempty"`
	LastSyncedAt                      *time.Time                 `json:"lastSyncedAt,omitempty"`
	ThreadPath                        *string                    `json:"threadPath,omitempty"`
	ThreadPreview                     *string                    `json:"threadPreview,omitempty"`
	TurnCount                         int                        `json:"turnCount"`
	ItemCount                         int                        `json:"itemCount"`
	SyncError                         *string                    `json:"syncError,omitempty"`
	CreatedAt                         time.Time                  `json:"createdAt"`
	UpdatedAt                         time.Time                  `json:"updatedAt"`
	Usage                             Usage                      `json:"usage"`
	LatestTurnUsage                   ContextEstimate            `json:"latestTurnUsage"`
	ContextEstimate                   ContextEstimate            `json:"contextEstimate"`
	ContextEstimateMode               ContextEstimateMode        `json:"contextEstimateMode"`
	LastContextCompactionAt           *time.Time                 `json:"lastContextCompactionAt,omitempty"`
	ContextWindowTokens               *int64                     `json:"contextWindowTokens,omitempty"`
	ContextWindowSource               ContextWindowSource        `json:"contextWindowSource"`
	Goal                              *SessionGoal               `json:"goal,omitempty"`
	SearchMatchSources                []SessionSearchMatchSource `json:"searchMatchSources,omitempty"`
	WorkTiming                        WorkTiming                 `json:"workTiming"`
}

type SessionReconcileTarget struct {
	ID       string `json:"id"`
	Revision string `json:"revision,omitempty"`
}

type SessionReconcileResult struct {
	Items      []SessionSummary `json:"items"`
	MissingIDs []string         `json:"missingIds"`
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

type SessionSearchChunkResult struct {
	Items      []SessionSummary `json:"items"`
	NextCursor string           `json:"nextCursor,omitempty"`
	Done       bool             `json:"done"`
	Scanned    int              `json:"scanned"`
	Total      int              `json:"total"`
}

type SessionConversationSearchMatch struct {
	ID             string  `json:"id"`
	SourceThreadID *string `json:"sourceThreadId,omitempty"`
	SourceTurnID   *string `json:"sourceTurnId,omitempty"`
	SourceItemID   *string `json:"sourceItemId,omitempty"`
	OrderIndex     int64   `json:"orderIndex"`
	Kind           string  `json:"kind"`
	ToolID         string  `json:"toolId,omitempty"`
	CommandGroupID string  `json:"commandGroupId,omitempty"`
}

type SessionConversationSearchResult struct {
	Items      []SessionConversationSearchMatch `json:"items"`
	NextCursor string                           `json:"nextCursor,omitempty"`
	Done       bool                             `json:"done"`
	Total      int                              `json:"total"`
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
	Type         string                `json:"type"`
	Prompt       string                `json:"prompt,omitempty"`
	ApprovalKind string                `json:"approvalKind,omitempty"`
	Command      string                `json:"command,omitempty"`
	Questions    []toolRequestQuestion `json:"questions,omitempty"`
	Answers      []HistoryAnswerEntry  `json:"answers,omitempty"`
	Action       string                `json:"action,omitempty"`
}

type HistoryAttachment struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Mime string `json:"mime,omitempty"`
	Size int64  `json:"size,omitempty"`
	Path string `json:"path,omitempty"`
}

type HistoryItem struct {
	ID             string              `json:"id"`
	SourceThreadID *string             `json:"sourceThreadId,omitempty"`
	SourceTurnID   *string             `json:"sourceTurnId,omitempty"`
	SourceItemID   *string             `json:"sourceItemId,omitempty"`
	RunID          *string             `json:"runId,omitempty"`
	RunDurationMs  *int64              `json:"runDurationMs,omitempty"`
	RunOutcome     WorkTimingOutcome   `json:"runOutcome,omitempty"`
	OrderIndex     int64               `json:"orderIndex"`
	Kind           string              `json:"kind"`
	ItemType       string              `json:"itemType"`
	Text           string              `json:"text"`
	Timestamp      *time.Time          `json:"timestamp,omitempty"`
	ObservedAt     *time.Time          `json:"observedAt,omitempty"`
	Attachments    []HistoryAttachment `json:"attachments,omitempty"`
	Tool           *HistoryTool        `json:"tool,omitempty"`
	Level          string              `json:"level,omitempty"`
	Done           bool                `json:"done,omitempty"`
	Detail         *HistoryDetail      `json:"detail,omitempty"`
	Payload        map[string]any      `json:"payload,omitempty"`
	LastEventSeq   int64               `json:"lastEventSeq,omitempty"`
}

type HistoryWindow struct {
	Items        []HistoryItem `json:"items"`
	HasMore      bool          `json:"hasMore"`
	BeforeCursor string        `json:"beforeCursor,omitempty"`
	HasLater     bool          `json:"hasLater,omitempty"`
	AfterCursor  string        `json:"afterCursor,omitempty"`
	Total        int           `json:"total"`
}

type PendingInput struct {
	ID            string             `json:"id"`
	Mode          PendingInputMode   `json:"mode"`
	Text          string             `json:"text"`
	AttachmentIDs []string           `json:"attachmentIds"`
	ReadyAt       *time.Time         `json:"readyAt,omitempty"`
	Paused        bool               `json:"paused,omitempty"`
	NativeQueued  bool               `json:"nativeQueued,omitempty"`
	Status        PendingInputStatus `json:"status,omitempty"`
	AttemptCount  int                `json:"attemptCount,omitempty"`
	LastError     string             `json:"lastError,omitempty"`
	LastErrorCode string             `json:"lastErrorCode,omitempty"`
	CreatedAt     time.Time          `json:"createdAt"`

	codexMessageID    string
	codexSteerReceipt *codexSteerReceipt
}

type ScheduledInput struct {
	ID               string                         `json:"id"`
	DependsOnID      string                         `json:"dependsOnId,omitempty"`
	DependencyStatus ScheduledInputDependencyStatus `json:"dependencyStatus"`
	Action           ScheduledInputAction           `json:"action"`
	TargetID         string                         `json:"targetId,omitempty"`
	Mode             ScheduledInputMode             `json:"mode"`
	ExitPlanMode     bool                           `json:"exitPlanMode,omitempty"`
	Text             string                         `json:"text"`
	AttachmentIDs    []string                       `json:"attachmentIds"`
	ScheduleKind     ScheduledInputScheduleKind     `json:"scheduleKind"`
	ScheduledFor     *time.Time                     `json:"scheduledFor"`
	IdleSince        *time.Time                     `json:"idleSince,omitempty"`
	BlockingReasons  []ScheduledInputBlockingReason `json:"blockingReasons"`
	ConditionError   string                         `json:"conditionError,omitempty"`
	Status           ScheduledInputStatus           `json:"status"`
	LastError        string                         `json:"lastError,omitempty"`
	CreatedAt        time.Time                      `json:"createdAt"`
	UpdatedAt        time.Time                      `json:"updatedAt"`
	SentAt           *time.Time                     `json:"sentAt,omitempty"`
	CanceledAt       *time.Time                     `json:"canceledAt,omitempty"`
}

type PendingUserInput struct {
	ItemID      string                `json:"itemId"`
	Prompt      string                `json:"prompt,omitempty"`
	Questions   []toolRequestQuestion `json:"questions,omitempty"`
	RequestedAt *time.Time            `json:"requestedAt,omitempty"`
}

type PendingApproval struct {
	ItemID      string     `json:"itemId"`
	Kind        string     `json:"kind"`
	Prompt      string     `json:"prompt"`
	Command     string     `json:"command,omitempty"`
	RequestedAt *time.Time `json:"requestedAt,omitempty"`
	Actionable  bool       `json:"actionable"`
}

type SessionSnapshot struct {
	Revision         string                `json:"revision"`
	HistoryEpoch     string                `json:"historyEpoch"`
	EventCursor      string                `json:"eventCursor"`
	PendingEpoch     string                `json:"pendingEpoch"`
	PendingVersion   uint64                `json:"pendingVersion"`
	Session          SessionSummary        `json:"session"`
	History          HistoryWindow         `json:"history"`
	PendingInputs    []PendingInput        `json:"pendingInputs"`
	ScheduledInputs  []ScheduledInput      `json:"scheduledInputs"`
	PendingApproval  *PendingApproval      `json:"pendingApproval,omitempty"`
	PendingUserInput *PendingUserInput     `json:"pendingUserInput,omitempty"`
	SubAgents        []WebSessionSubAgent  `json:"subAgents"`
	CodexAppServer   CodexAppServerRuntime `json:"codexAppServer"`
}

type SessionSnapshotResponse struct {
	Revision         string                `json:"revision"`
	HistoryEpoch     string                `json:"historyEpoch"`
	EventCursor      string                `json:"eventCursor"`
	PendingEpoch     string                `json:"pendingEpoch"`
	PendingVersion   uint64                `json:"pendingVersion"`
	Unchanged        bool                  `json:"unchanged"`
	Session          *SessionSummary       `json:"session,omitempty"`
	History          *HistoryWindow        `json:"history,omitempty"`
	PendingInputs    []PendingInput        `json:"pendingInputs"`
	ScheduledInputs  []ScheduledInput      `json:"scheduledInputs,omitempty"`
	PendingApproval  *PendingApproval      `json:"pendingApproval,omitempty"`
	PendingUserInput *PendingUserInput     `json:"pendingUserInput,omitempty"`
	SubAgents        []WebSessionSubAgent  `json:"subAgents"`
	CodexAppServer   CodexAppServerRuntime `json:"codexAppServer"`
}

// SessionHydrationTarget identifies state that the client hydrates through the
// conditional snapshot endpoint. Mutation endpoints must not return history.
type SessionHydrationTarget struct {
	Revision string         `json:"revision"`
	Session  SessionSummary `json:"session"`
}

func NewSessionHydrationTarget(session SessionSummary) SessionHydrationTarget {
	return SessionHydrationTarget{
		Revision: session.Revision,
		Session:  session,
	}
}

func NewSessionSnapshotResponse(snapshot SessionSnapshot) SessionSnapshotResponse {
	return SessionSnapshotResponse{
		Revision:         snapshot.Revision,
		HistoryEpoch:     snapshot.HistoryEpoch,
		EventCursor:      snapshot.EventCursor,
		PendingEpoch:     snapshot.PendingEpoch,
		PendingVersion:   snapshot.PendingVersion,
		Session:          &snapshot.Session,
		History:          &snapshot.History,
		PendingInputs:    snapshot.PendingInputs,
		ScheduledInputs:  snapshot.ScheduledInputs,
		PendingApproval:  snapshot.PendingApproval,
		PendingUserInput: snapshot.PendingUserInput,
		SubAgents:        snapshot.SubAgents,
		CodexAppServer:   snapshot.CodexAppServer,
	}
}

type WebSessionSubAgentStatus string

const (
	WebSessionSubAgentPendingInit WebSessionSubAgentStatus = "pending_init"
	WebSessionSubAgentRunning     WebSessionSubAgentStatus = "running"
	WebSessionSubAgentIdle        WebSessionSubAgentStatus = "idle"
	WebSessionSubAgentInterrupted WebSessionSubAgentStatus = "interrupted"
	WebSessionSubAgentCompleted   WebSessionSubAgentStatus = "completed"
	WebSessionSubAgentErrored     WebSessionSubAgentStatus = "errored"
	WebSessionSubAgentShutdown    WebSessionSubAgentStatus = "shutdown"
	WebSessionSubAgentNotFound    WebSessionSubAgentStatus = "not_found"
)

type WebSessionSubAgent struct {
	ThreadID         string                   `json:"threadId"`
	ParentThreadID   *string                  `json:"parentThreadId,omitempty"`
	Path             string                   `json:"path,omitempty"`
	Nickname         string                   `json:"nickname,omitempty"`
	Role             string                   `json:"role,omitempty"`
	Status           WebSessionSubAgentStatus `json:"status"`
	Summary          string                   `json:"summary,omitempty"`
	CurrentTurnID    *string                  `json:"currentTurnId,omitempty"`
	LatestItemID     *string                  `json:"latestItemId,omitempty"`
	LatestOrderIndex int64                    `json:"latestOrderIndex,omitempty"`
	StartedAt        *time.Time               `json:"startedAt,omitempty"`
	LastActivityAt   *time.Time               `json:"lastActivityAt,omitempty"`
	EndedAt          *time.Time               `json:"endedAt,omitempty"`
}

type ImportResult struct {
	Session         SessionSummary       `json:"session"`
	History         HistoryWindow        `json:"history"`
	PendingInputs   []PendingInput       `json:"pendingInputs"`
	ScheduledInputs []ScheduledInput     `json:"scheduledInputs"`
	SubAgents       []WebSessionSubAgent `json:"subAgents"`
	Created         bool                 `json:"created"`
	Reused          bool                 `json:"reused"`
	Synced          bool                 `json:"synced"`
}

type ImportHydrationTarget struct {
	Revision string         `json:"revision"`
	Session  SessionSummary `json:"session"`
	Created  bool           `json:"created"`
	Reused   bool           `json:"reused"`
	Synced   bool           `json:"synced"`
}

func NewImportHydrationTarget(result ImportResult) ImportHydrationTarget {
	return ImportHydrationTarget{
		Revision: result.Session.Revision,
		Session:  result.Session,
		Created:  result.Created,
		Reused:   result.Reused,
		Synced:   result.Synced,
	}
}

type ImportSourceSummary struct {
	Agent                 Agent           `json:"agent"`
	Importable            bool            `json:"importable"`
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
	Items        []ImportSourceSummary `json:"items"`
	ScanPhase    string                `json:"scanPhase,omitempty"`
	BeforeCursor string                `json:"beforeCursor,omitempty"`
}

type Event struct {
	ID        string         `json:"id"`
	Seq       int64          `json:"seq"`
	Type      string         `json:"type"`
	RunID     string         `json:"runId,omitempty"`
	ParentID  string         `json:"parentId,omitempty"`
	ThreadID  string         `json:"threadId,omitempty"`
	TurnID    string         `json:"turnId,omitempty"`
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
	ProjectID                         string
	WorktreeID                        string
	Agent                             Agent
	ClaudeRuntime                     ClaudeRuntime
	Backend                           SessionBackend
	Model                             string
	ReasoningEffort                   ReasoningEffort
	WorkflowMode                      WorkflowMode
	PermissionLevel                   PermissionLevel
	ActiveCallTimeoutEnabled          *bool
	AutoRetryEnabled                  bool
	AutoRetryPolicyMode               *AutoRetryPolicyMode
	AutoRetryScope                    *AutoRetryScope
	AutoRetryPreset                   *AutoRetryPreset
	AutoRetryMaxAttempts              *int
	AutoRetryDispatchPendingOnFailure *bool
	Title                             string
}
