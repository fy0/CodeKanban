package tables

import (
	"time"

	"code-kanban/utils/model_base"
)

// WebSessionItemTable caches normalized timeline items for a web session.
type WebSessionItemTable struct {
	model_base.StringPKBaseModel

	WebSessionID   string  `gorm:"type:text;not null;index:idx_web_session_item_order,priority:1;index:idx_web_session_item_source,priority:1;index" json:"webSessionId"`
	WebTurnID      *string `gorm:"type:text;index" json:"webTurnId"`
	SourceThreadID *string `gorm:"type:text;index:idx_web_session_item_source,priority:2;index" json:"sourceThreadId"`
	SourceTurnID   *string `gorm:"type:text;index" json:"sourceTurnId"`
	SourceItemID   *string `gorm:"type:text;index:idx_web_session_item_source,priority:3;index" json:"sourceItemId"`
	OrderIndex     int64   `gorm:"type:integer;not null;index:idx_web_session_item_order,priority:2" json:"orderIndex"`

	ItemKind string `gorm:"type:text;not null;index" json:"itemKind"`
	ItemType string `gorm:"type:text;not null;index" json:"itemType"`
	Role     string `gorm:"type:text" json:"role"`
	Status   string `gorm:"type:text" json:"status"`
	Level    string `gorm:"type:text" json:"level"`
	Text     string `gorm:"type:text" json:"text"`
	Done     bool   `gorm:"type:boolean;not null;default:false" json:"done"`

	Timestamp  *time.Time `gorm:"type:datetime;index" json:"timestamp"`
	ObservedAt *time.Time `gorm:"type:datetime;index" json:"observedAt"`

	AttachmentsJSON string `gorm:"type:text" json:"attachmentsJson"`
	ToolJSON        string `gorm:"type:text" json:"toolJson"`
	DetailJSON      string `gorm:"type:text" json:"detailJson"`
	PayloadJSON     string `gorm:"type:text" json:"payloadJson"`
}

func (WebSessionItemTable) TableName() string {
	return "web_session_items"
}
