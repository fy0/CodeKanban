package tables

import (
	"time"

	"code-kanban/utils/model_base"
)

// TerminalRestoreSessionTable stores enough terminal metadata to rebuild a
// project-local terminal workspace after the app process restarts.
type TerminalRestoreSessionTable struct {
	model_base.StringPKBaseModel

	ProjectID         string     `gorm:"type:text;not null;index" json:"projectId"`
	WorktreeID        string     `gorm:"type:text;not null;index" json:"worktreeId"`
	Title             string     `gorm:"type:text;not null" json:"title"`
	TaskID            *string    `gorm:"type:text;index" json:"taskId"`
	OrderIndex        float64    `gorm:"type:real;not null;default:0;index" json:"orderIndex"`
	InitialWorkingDir string     `gorm:"type:text;not null" json:"initialWorkingDir"`
	LastCwd           string     `gorm:"type:text;not null" json:"lastCwd"`
	ShellFamily       string     `gorm:"type:text;not null;default:''" json:"shellFamily"`
	ShellSupported    bool       `gorm:"type:boolean;not null;default:false" json:"shellSupported"`
	ShellState        string     `gorm:"type:text;not null;default:idle" json:"shellState"`
	PendingCommand    *string    `gorm:"type:text" json:"pendingCommand"`
	ReplayEligible    bool       `gorm:"type:boolean;not null;default:false" json:"replayEligible"`
	CommandStartedAt  *time.Time `gorm:"type:datetime" json:"commandStartedAt"`
}

func (TerminalRestoreSessionTable) TableName() string {
	return "terminal_restore_sessions"
}
