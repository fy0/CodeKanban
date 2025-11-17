package tables

import (
	"time"

	"code-kanban/utils/model_base"
)

// ProjectTable defines the persisted metadata for a tracked git repository.
type ProjectTable struct {
	model_base.StringPKBaseModel

	Name             string     `gorm:"type:text;not null;index" json:"name"`
	Path             string     `gorm:"type:text;not null;uniqueIndex" json:"path"`
	Description      string     `gorm:"type:text" json:"description"`
	DefaultBranch    string     `gorm:"type:text" json:"defaultBranch"`
	WorktreeBasePath string     `gorm:"type:text" json:"worktreeBasePath"`
	RemoteURL        string     `gorm:"type:text" json:"remoteUrl"`
	LastSyncAt       *time.Time `gorm:"type:datetime" json:"lastSyncAt"`
	HidePath         bool       `gorm:"type:boolean;not null;default:false" json:"hidePath"`
}

// TableName maps the gorm model to the projects table.
func (ProjectTable) TableName() string {
	return "projects"
}
