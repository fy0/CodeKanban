package tables

import (
	"time"

	"code-kanban/utils/model_base"
)

// WebSessionRunTimingTable is the durable, idempotent ledger for measured runs.
type WebSessionRunTimingTable struct {
	model_base.StringPKBaseModel

	WebSessionID string `gorm:"type:text;not null;uniqueIndex:idx_web_session_run_timing,priority:1;index" json:"webSessionId"`
	RunID        string `gorm:"type:text;not null;uniqueIndex:idx_web_session_run_timing,priority:2" json:"runId"`

	StartedAt        time.Time  `gorm:"type:datetime;not null" json:"startedAt"`
	EndedAt          *time.Time `gorm:"type:datetime" json:"endedAt"`
	PausedDurationMs int64      `gorm:"type:integer;not null;default:0" json:"pausedDurationMs"`
	DurationMs       int64      `gorm:"type:integer;not null;default:0" json:"durationMs"`
	Outcome          string     `gorm:"type:text;index" json:"outcome"`
	TerminalEventSeq int64      `gorm:"type:integer;not null;default:0" json:"terminalEventSeq"`
	Backfilled       bool       `gorm:"type:boolean;not null;default:false" json:"backfilled"`

	AnchorItemID         *string `gorm:"type:text;index" json:"anchorItemId"`
	AnchorSourceThreadID *string `gorm:"type:text" json:"anchorSourceThreadId"`
	AnchorSourceTurnID   *string `gorm:"type:text" json:"anchorSourceTurnId"`
	AnchorSourceItemID   *string `gorm:"type:text" json:"anchorSourceItemId"`
}

func (WebSessionRunTimingTable) TableName() string {
	return "web_session_run_timings"
}
