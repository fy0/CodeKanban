package tables

import (
	"time"

	"code-kanban/utils/model_base"
)

// WebSessionTable stores metadata for browser-based Claude/Codex sessions.
// The actual conversation cache is normalized into SQLite tables.
type WebSessionTable struct {
	model_base.StringPKBaseModel

	ProjectID  string  `gorm:"type:text;not null;index" json:"projectId"`
	WorktreeID *string `gorm:"type:text;index" json:"worktreeId"`
	OrderIndex float64 `gorm:"type:real;not null;default:0;index" json:"orderIndex"`

	Agent                             string `gorm:"type:text;not null;index" json:"agent"`
	ClaudeRuntime                     string `gorm:"type:text;not null;default:claude" json:"claudeRuntime"`
	Backend                           string `gorm:"type:text;not null;default:legacy_exec" json:"-"`
	Title                             string `gorm:"type:text;not null" json:"title"`
	TitleAuto                         bool   `gorm:"type:boolean;not null;default:false" json:"-"`
	Model                             string `gorm:"type:text" json:"model"`
	ReasoningEffort                   string `gorm:"type:text" json:"reasoningEffort"`
	WorkflowMode                      string `gorm:"type:text;not null;default:default" json:"workflowMode"`
	SessionStartSource                string `gorm:"type:text;not null;default:startup" json:"-"`
	PermissionLevel                   string `gorm:"type:text;not null;default:elevated" json:"permissionLevel"`
	ActiveCallTimeoutEnabled          *bool  `gorm:"type:boolean" json:"activeCallTimeoutEnabled,omitempty"`
	ContextWindowSetting              int64  `gorm:"not null;default:0" json:"contextWindowSetting"`
	AppliedContextWindowSetting       *int64 `json:"appliedContextWindowSetting"`
	CodexModelMetadataFallback        bool   `gorm:"not null;default:false" json:"codexModelMetadataFallback"`
	AutoRetryEnabled                  bool   `gorm:"type:boolean;not null;default:false" json:"autoRetryEnabled"`
	AutoRetryPolicyMode               string `gorm:"type:text;not null;default:default" json:"autoRetryPolicyMode"`
	AutoRetryScope                    string `gorm:"type:text;not null;default:network_only" json:"autoRetryScope"`
	AutoRetryPreset                   string `gorm:"type:text;not null;default:gentle_stop" json:"autoRetryPreset"`
	AutoRetryMaxAttempts              int    `gorm:"type:integer;not null;default:0" json:"autoRetryMaxAttempts"`
	AutoRetryDispatchPendingOnFailure bool   `gorm:"type:boolean;not null;default:false" json:"autoRetryDispatchPendingOnFailure"`
	LegacyPermissionMode              string `gorm:"column:permission_mode;type:text" json:"-"`
	Cwd                               string `gorm:"type:text;not null" json:"cwd"`

	NativeSessionID         *string    `gorm:"type:text" json:"nativeSessionId"`
	NativeLeafID            *string    `gorm:"type:text" json:"nativeLeafId"`
	SourceRevision          *string    `gorm:"type:text" json:"sourceRevision"`
	CyberPolicyFlagged      bool       `gorm:"type:boolean;not null;default:false" json:"cyberPolicyFlagged"`
	Status                  string     `gorm:"type:text;not null;index" json:"status"`
	AssistantState          string     `gorm:"type:text;index" json:"assistantState"`
	HasUnread               bool       `gorm:"type:boolean;not null;default:false" json:"hasUnread"`
	AttentionRevision       int64      `gorm:"type:integer;not null;default:0" json:"attentionRevision"`
	HistoryEpoch            int64      `gorm:"type:integer;not null;default:1" json:"historyEpoch"`
	ArchivedAt              *time.Time `gorm:"type:datetime;index" json:"archivedAt"`
	ActivityAt              time.Time  `gorm:"type:datetime;index" json:"activityAt"`
	StatusUpdatedAt         *time.Time `gorm:"type:datetime;index" json:"statusUpdatedAt"`
	AssistantStateUpdatedAt *time.Time `gorm:"type:datetime" json:"assistantStateUpdatedAt"`
	SourceKind              string     `gorm:"type:text;not null;default:codex_app_server" json:"sourceKind"`
	SyncState               string     `gorm:"type:text;not null;default:missing;index" json:"syncState"`
	LastSyncMode            string     `gorm:"type:text" json:"lastSyncMode"`
	SourceCreatedAt         *time.Time `gorm:"type:datetime" json:"sourceCreatedAt"`
	SourceUpdatedAt         *time.Time `gorm:"type:datetime;index" json:"sourceUpdatedAt"`
	LastSyncedAt            *time.Time `gorm:"type:datetime" json:"lastSyncedAt"`
	ThreadPath              *string    `gorm:"type:text" json:"threadPath"`
	ThreadPreview           *string    `gorm:"type:text" json:"threadPreview"`
	TurnCount               int        `gorm:"type:integer;not null;default:0" json:"turnCount"`
	ItemCount               int        `gorm:"type:integer;not null;default:0" json:"itemCount"`

	LastMessageAt    *time.Time `gorm:"type:datetime" json:"lastMessageAt"`
	LastEventSeq     int64      `gorm:"type:integer;not null;default:0" json:"lastEventSeq"`
	SnapshotRevision int64      `gorm:"type:integer;not null;default:1" json:"snapshotRevision"`

	WorkDurationMs                 int64      `gorm:"type:integer;not null;default:0" json:"-"`
	WorkCurrentRunID               *string    `gorm:"type:text;index" json:"-"`
	WorkCurrentRunStartedAt        *time.Time `gorm:"type:datetime" json:"-"`
	WorkCurrentRunPausedAt         *time.Time `gorm:"type:datetime" json:"-"`
	WorkCurrentRunPausedDurationMs int64      `gorm:"type:integer;not null;default:0" json:"-"`
	WorkCurrentRunPauseDepth       int        `gorm:"type:integer;not null;default:0" json:"-"`
	WorkRetryWaitStartedAt         *time.Time `gorm:"type:datetime" json:"-"`
	WorkRetrySourceRunID           *string    `gorm:"type:text" json:"-"`
	WorkTimingBackfillState        string     `gorm:"type:text;not null;default:pending;index:idx_web_session_work_backfill,priority:1" json:"-"`
	WorkTimingBackfillVersion      int        `gorm:"type:integer;not null;default:0;index:idx_web_session_work_backfill,priority:2" json:"-"`

	GoalObjective       *string    `gorm:"type:text" json:"goalObjective"`
	GoalStatus          *string    `gorm:"type:text" json:"goalStatus"`
	GoalTokenBudget     *int64     `gorm:"type:integer" json:"goalTokenBudget"`
	GoalTokensUsed      int64      `gorm:"type:integer;not null;default:0" json:"goalTokensUsed"`
	GoalTimeUsedSeconds int64      `gorm:"type:integer;not null;default:0" json:"goalTimeUsedSeconds"`
	GoalCreatedAt       *time.Time `gorm:"type:datetime" json:"goalCreatedAt"`
	GoalUpdatedAt       *time.Time `gorm:"type:datetime" json:"goalUpdatedAt"`

	TotalInputTokens                  int64      `gorm:"type:integer;not null;default:0" json:"totalInputTokens"`
	TotalCachedInputTokens            int64      `gorm:"type:integer;not null;default:0" json:"totalCachedInputTokens"`
	TotalOutputTokens                 int64      `gorm:"type:integer;not null;default:0" json:"totalOutputTokens"`
	TotalCost                         float64    `gorm:"type:real;not null;default:0" json:"totalCost"`
	LastCompletedInputTokens          int64      `gorm:"type:integer;not null;default:0" json:"-"`
	LastCompletedCachedInputTokens    int64      `gorm:"type:integer;not null;default:0" json:"-"`
	LastCompletedOutputTokens         int64      `gorm:"type:integer;not null;default:0" json:"-"`
	LatestTurnInputTokens             int64      `gorm:"type:integer;not null;default:0" json:"-"`
	LatestTurnCachedInputTokens       int64      `gorm:"type:integer;not null;default:0" json:"-"`
	LatestTurnOutputTokens            int64      `gorm:"type:integer;not null;default:0" json:"-"`
	LatestTurnUsageUpdatedAt          *time.Time `gorm:"type:datetime" json:"-"`
	LatestTokenCountInputTokens       int64      `gorm:"type:integer;not null;default:0" json:"-"`
	LatestTokenCountCachedInputTokens int64      `gorm:"type:integer;not null;default:0" json:"-"`
	LatestTokenCountOutputTokens      int64      `gorm:"type:integer;not null;default:0" json:"-"`
	LatestTokenCountTotalTokens       int64      `gorm:"type:integer;not null;default:0" json:"-"`
	LatestTokenCountUpdatedAt         *time.Time `gorm:"type:datetime" json:"-"`
	SessionContextWindowTokens        int64      `gorm:"type:integer;not null;default:0" json:"-"`
	SessionContextWindowObservedAt    *time.Time `gorm:"type:datetime" json:"-"`
	ContextBaselineInputTokens        int64      `gorm:"type:integer;not null;default:0" json:"-"`
	ContextBaselineCachedInputTokens  int64      `gorm:"type:integer;not null;default:0" json:"-"`
	ContextBaselineOutputTokens       int64      `gorm:"type:integer;not null;default:0" json:"-"`
	LastContextCompactionAt           *time.Time `gorm:"type:datetime" json:"-"`
	AutoRetryAttempt                  int        `gorm:"type:integer;not null;default:0" json:"-"`
	AutoRetryNextAt                   *time.Time `gorm:"type:datetime" json:"-"`
	AutoRetryLastErrorCode            *string    `gorm:"type:text" json:"-"`

	LastError *string `gorm:"type:text" json:"lastError"`
	SyncError *string `gorm:"type:text" json:"syncError"`
}

func (WebSessionTable) TableName() string {
	return "web_sessions"
}
