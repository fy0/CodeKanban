package tables

import "time"

// WebSessionTokenSpeedTable keeps one measurement per model generation request.
// Late usage reports update the same row instead of counting the request twice.
type WebSessionTokenSpeedTable struct {
	WebSessionID string `gorm:"type:text;primaryKey;index:idx_web_session_token_speed_recent,priority:1"`
	RunID        string `gorm:"type:text;primaryKey"`
	SampleID     string `gorm:"type:text;primaryKey"`
	Agent        string `gorm:"type:text"`
	Model        string `gorm:"type:text"`
	StartedAt    time.Time
	EndedAt      time.Time `gorm:"index:idx_web_session_token_speed_recent,priority:2"`
	OutputTokens float64
	DurationMs   int64
	Estimated    bool
	Version      uint64
}

func (WebSessionTokenSpeedTable) TableName() string {
	return "web_session_token_speeds"
}
