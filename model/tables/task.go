package tables

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"go-template/utils/model_base"
)

// StringArray provides json serialization helpers for simple string lists.
type StringArray []string

// Value implements driver.Valuer so GORM can persist the string slice as JSON.
func (a StringArray) Value() (driver.Value, error) {
	if len(a) == 0 {
		return "[]", nil
	}
	bytes, err := json.Marshal([]string(a))
	if err != nil {
		return nil, err
	}
	return string(bytes), nil
}

// Scan implements sql.Scanner to read a JSON encoded string slice from the DB.
func (a *StringArray) Scan(value any) error {
	if value == nil {
		*a = []string{}
		return nil
	}

	var data []byte
	switch v := value.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return fmt.Errorf("unsupported type %T for StringArray", value)
	}

	if len(data) == 0 {
		*a = []string{}
		return nil
	}

	return json.Unmarshal(data, a)
}

// TaskTable stores kanban/task metadata with optional worktree relations.
type TaskTable struct {
	model_base.StringPKBaseModel

	ProjectID   string      `gorm:"type:text;not null;index" json:"projectId"`
	WorktreeID  *string     `gorm:"type:text;index" json:"worktreeId"`
	BranchName  string      `gorm:"type:text" json:"branchName"` // 存储关联的分支名称，即使worktree被删除也能显示
	Title       string      `gorm:"type:text;not null" json:"title"`
	Description string      `gorm:"type:text" json:"description"`
	Status      string      `gorm:"type:text;not null;index" json:"status"` // todo/in_progress/done/archived
	Priority    int         `gorm:"type:integer;default:0;index" json:"priority"`
	OrderIndex  float64     `gorm:"type:real;not null;index" json:"orderIndex"`
	Tags        StringArray `gorm:"type:text" json:"tags"`
	DueDate     *time.Time  `gorm:"type:datetime" json:"dueDate"`
	CompletedAt *time.Time  `gorm:"type:datetime" json:"completedAt"`

	Project  *ProjectTable  `gorm:"foreignKey:ProjectID;constraint:OnDelete:CASCADE" json:"project,omitempty"`
	Worktree *WorktreeTable `gorm:"foreignKey:WorktreeID;constraint:OnDelete:SET NULL" json:"worktree,omitempty"`
}

// TableName maps the gorm model to the tasks table.
func (TaskTable) TableName() string {
	return "tasks"
}
