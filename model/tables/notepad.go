package tables

import (
	"go-template/utils/model_base"
)

// NotePadTable stores notepad tabs with their content.
type NotePadTable struct {
	model_base.StringPKBaseModel

	ProjectID  *string `gorm:"type:text;index" json:"projectId"` // null for global notes
	Name       string  `gorm:"type:text;not null" json:"name"`
	Content    string  `gorm:"type:text" json:"content"`
	OrderIndex float64 `gorm:"type:real;not null;index" json:"orderIndex"`
}

// TableName maps the gorm model to the notepads table.
func (NotePadTable) TableName() string {
	return "notepads"
}
