package tables

import (
	"time"

	"go-template/utils/model_base"
)

// WorktreeTable tracks git worktree metadata alongside cached status information.
type WorktreeTable struct {
	model_base.StringPKBaseModel

	ProjectID  string `gorm:"type:text;not null;index" json:"projectId"`
	BranchName string `gorm:"type:text;not null;index" json:"branchName"`
	Path       string `gorm:"type:text;not null;uniqueIndex:idx_worktrees_path,where:deleted_at IS NULL" json:"path"`
	IsMain        bool       `gorm:"type:boolean;default:false" json:"isMain"`
	IsBare        bool       `gorm:"type:boolean;default:false" json:"isBare"`
	HeadCommit    string     `gorm:"type:text" json:"headCommit"`
	HeadCommitDate *time.Time `gorm:"type:datetime" json:"headCommitDate"`

	StatusAhead     int        `gorm:"type:integer;default:0" json:"statusAhead"`
	StatusBehind    int        `gorm:"type:integer;default:0" json:"statusBehind"`
	StatusModified  int        `gorm:"type:integer;default:0" json:"statusModified"`
	StatusStaged    int        `gorm:"type:integer;default:0" json:"statusStaged"`
	StatusUntracked int        `gorm:"type:integer;default:0" json:"statusUntracked"`
	StatusConflicts int        `gorm:"type:integer;default:0" json:"statusConflicts"`
	StatusUpdatedAt *time.Time `gorm:"type:datetime" json:"statusUpdatedAt"`

	Project *ProjectTable `gorm:"foreignKey:ProjectID;constraint:OnDelete:CASCADE" json:"project,omitempty"`
}

// TableName maps the gorm model to the worktrees table.
func (WorktreeTable) TableName() string {
	return "worktrees"
}
