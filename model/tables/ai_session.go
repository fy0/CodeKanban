package tables

import (
	"time"

	"code-kanban/utils/model_base"
)

// AISessionType defines the type of AI assistant
type AISessionType string

const (
	AISessionTypeClaudeCode AISessionType = "claude_code"
	AISessionTypeCodex      AISessionType = "codex"
	AISessionTypePi         AISessionType = "pi"
)

// AISessionTable stores cached AI assistant session metadata.
// This table caches session information scanned from log files to avoid
// repeated filesystem scans.
type AISessionTable struct {
	model_base.StringPKBaseModel

	// SessionID is the unique identifier from the AI assistant
	SessionID string `gorm:"type:text;not null;uniqueIndex:idx_session_type" json:"sessionId"`

	// Type identifies which AI assistant (claude_code, codex, pi)
	Type AISessionType `gorm:"type:text;not null;uniqueIndex:idx_session_type" json:"type"`

	// ProjectPath is the working directory associated with this session
	ProjectPath string `gorm:"type:text;not null;index" json:"projectPath"`

	// FilePath is the full path to the session log file
	FilePath string `gorm:"type:text;not null" json:"filePath"`

	// Model is the AI model used (e.g., claude-opus-4-5-20251101, gpt-4)
	Model string `gorm:"type:text" json:"model"`

	// Title is a summary/title extracted from the first user message
	Title string `gorm:"type:text" json:"title"`

	// SessionStartedAt is when the session was created
	SessionStartedAt time.Time `gorm:"type:datetime;not null" json:"sessionStartedAt"`

	// LastMessageAt is the timestamp of the last user message
	LastMessageAt *time.Time `gorm:"type:datetime" json:"lastMessageAt"`

	// MessageCount is the number of user messages in the session
	MessageCount int `gorm:"type:integer;default:0" json:"messageCount"`

	// AssistantMessageCount is the number of assistant messages in the session
	AssistantMessageCount int `gorm:"type:integer;default:0" json:"assistantMessageCount"`

	// FileModTime is the last modification time of the log file (for cache invalidation)
	FileModTime time.Time `gorm:"type:datetime;not null" json:"fileModTime"`

	// FileSize is the size of the log file in bytes (for cache invalidation)
	FileSize int64 `gorm:"type:integer;not null" json:"fileSize"`
}

// TableName maps the gorm model to the ai_sessions table.
func (AISessionTable) TableName() string {
	return "ai_sessions"
}
