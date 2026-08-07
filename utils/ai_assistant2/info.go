package ai_assistant2

import "code-kanban/utils/ai_assistant2/types"

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
