package tables

import (
	"code-kanban/utils/model_base"
)

// UserTable stores credential and profile data for a platform user.
type UserTable struct {
	model_base.StringPKBaseModel

	Nickname string `gorm:"type:text" json:"nickname"`
	Avatar   string `gorm:"type:text" json:"avatar"`
	Brief    string `gorm:"type:text" json:"brief"`

	Username string `gorm:"type:text;uniqueIndex:idx_users_username;not null" json:"username"`
	Password string `gorm:"type:text;not null" json:"-"`
	// Salt is stored separately so password rehashing can alter algorithms without
	// having to inspect legacy hash format.
	Salt string `gorm:"type:text;not null" json:"-"`

	Disabled bool `gorm:"not null;default:false" json:"disabled"`
}

func (*UserTable) TableName() string {
	return "users"
}
