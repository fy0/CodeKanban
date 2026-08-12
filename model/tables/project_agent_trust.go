package tables

import (
	"time"

	"code-kanban/utils/model_base"
)

// ProjectAgentTrustTable records explicit approval for an agent to load
// project-local resources from a specific project path.
type ProjectAgentTrustTable struct {
	model_base.StringPKBaseModel

	ProjectID   string     `gorm:"type:text;not null;uniqueIndex:idx_project_agent_trust,priority:1" json:"projectId"`
	Agent       string     `gorm:"type:text;not null;uniqueIndex:idx_project_agent_trust,priority:2" json:"agent"`
	TrustedPath string     `gorm:"type:text;not null" json:"trustedPath"`
	TrustedAt   time.Time  `gorm:"type:datetime;not null" json:"trustedAt"`
	RevokedAt   *time.Time `gorm:"type:datetime;index" json:"revokedAt"`
}

func (ProjectAgentTrustTable) TableName() string {
	return "project_agent_trusts"
}
