package ai_assistant2

import (
	"time"

	"code-kanban/utils/ai_assistant2/types"
)

// AIAssistantInfo is exported from types package for convenience
type AIAssistantInfo = types.AIAssistantInfo

// ToAIAssistantInfo converts AssistantInfo to AIAssistantInfo for API responses
func ToAIAssistantInfo(info *types.AssistantInfo) *AIAssistantInfo {
	if info == nil {
		return nil
	}

	return &AIAssistantInfo{
		Type:        string(info.Type),
		Name:        info.Name,
		DisplayName: info.DisplayName,
		Detected:    info.Detected,
		Command:     info.Command,
	}
}

// SetState updates the state of AIAssistantInfo
func SetState(info *AIAssistantInfo, state types.State, timestamp time.Time) {
	if info == nil {
		return
	}

	info.State = string(state)
	info.StateUpdatedAt = timestamp
}
