package tables

import (
	"time"

	"code-kanban/utils/model_base"
)

type WebSessionScheduledInputTable struct {
	model_base.StringPKBaseModel

	WebSessionID      string     `gorm:"type:text;not null;index" json:"webSessionId"`
	Action            string     `gorm:"type:text;not null;default:message;index" json:"action"`
	TargetID          string     `gorm:"type:text;index" json:"targetId"`
	PayloadJSON       string     `gorm:"column:payload_json;type:text;not null;default:'{}'" json:"payloadJson"`
	Mode              string     `gorm:"type:text;not null;default:send;index" json:"mode"`
	Text              string     `gorm:"type:text" json:"text"`
	AttachmentIDsJSON string     `gorm:"column:attachment_ids_json;type:text;not null;default:'[]'" json:"attachmentIdsJson"`
	ScheduledFor      time.Time  `gorm:"type:datetime;not null;index" json:"scheduledFor"`
	Status            string     `gorm:"type:text;not null;default:scheduled;index" json:"status"`
	LastError         string     `gorm:"type:text;not null;default:''" json:"lastError"`
	SentAt            *time.Time `gorm:"type:datetime" json:"sentAt"`
	CanceledAt        *time.Time `gorm:"type:datetime" json:"canceledAt"`
}

func (WebSessionScheduledInputTable) TableName() string {
	return "web_session_scheduled_inputs"
}
