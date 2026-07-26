package tables

import (
	"time"

	"code-kanban/utils/model_base"
)

// WebSessionSubAgentTable stores the latest known state for one Codex child thread.
type WebSessionSubAgentTable struct {
	model_base.StringPKBaseModel

	WebSessionID     string     `gorm:"type:text;not null;uniqueIndex:idx_web_session_sub_agent_thread,priority:1;index" json:"webSessionId"`
	ThreadID         string     `gorm:"type:text;not null;uniqueIndex:idx_web_session_sub_agent_thread,priority:2;index" json:"threadId"`
	ParentThreadID   *string    `gorm:"type:text;index" json:"parentThreadId"`
	AgentPath        string     `gorm:"type:text" json:"agentPath"`
	Nickname         string     `gorm:"type:text" json:"nickname"`
	Role             string     `gorm:"type:text" json:"role"`
	Status           string     `gorm:"type:text;not null;index" json:"status"`
	Summary          string     `gorm:"type:text" json:"summary"`
	CurrentTurnID    *string    `gorm:"type:text;index" json:"currentTurnId"`
	LatestItemID     *string    `gorm:"type:text" json:"latestItemId"`
	LatestOrderIndex int64      `gorm:"type:integer;not null;default:0" json:"latestOrderIndex"`
	LastEventSeq     int64      `gorm:"type:integer;not null;default:0" json:"lastEventSeq"`
	StartedAt        *time.Time `gorm:"type:datetime" json:"startedAt"`
	LastActivityAt   *time.Time `gorm:"type:datetime;index" json:"lastActivityAt"`
	EndedAt          *time.Time `gorm:"type:datetime" json:"endedAt"`
}

func (WebSessionSubAgentTable) TableName() string {
	return "web_session_sub_agents"
}
