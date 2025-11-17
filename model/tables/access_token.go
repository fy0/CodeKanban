package tables

import (
	"time"

	"code-kanban/utils/model_base"
)

// UserAccessTokenTable stores refreshable access tokens bound to a user.
type UserAccessTokenTable struct {
	model_base.StringPKBaseModel
	UserID    string    `gorm:"type:text;not null;index:idx_access_tokens_user_id" json:"userId"`
	ExpiredAt time.Time `gorm:"not null" json:"expiredAt"`
}

func (*UserAccessTokenTable) TableName() string {
	return "user_access_tokens"
}
