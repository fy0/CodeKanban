package tables

import "code-kanban/utils/model_base"

// TaskCommentTable stores comment threads associated with a task.
type TaskCommentTable struct {
	model_base.StringPKBaseModel

	TaskID  string `gorm:"type:text;not null;index" json:"taskId"`
	Content string `gorm:"type:text;not null" json:"content"`

	Task *TaskTable `gorm:"foreignKey:TaskID;constraint:OnDelete:CASCADE" json:"task,omitempty"`
}

// TableName maps the gorm model to the task_comments table.
func (TaskCommentTable) TableName() string {
	return "task_comments"
}
