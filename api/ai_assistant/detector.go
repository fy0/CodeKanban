package ai_assistant

import (
	"sync"
)

// Detector is responsible for detecting AI assistants from process information.
type Detector struct {
	rules []DetectionRule
	mu    sync.RWMutex
}

// NewDetector creates a new AI assistant detector with default rules.
func NewDetector() *Detector {
	return &Detector{
		rules: GetDefaultRules(),
	}
}

// NewDetectorWithRules creates a detector with custom rules.
func NewDetectorWithRules(rules []DetectionRule) *Detector {
	return &Detector{
		rules: rules,
	}
}

// Detect analyzes a command string and returns detected AI assistant info.
// Returns nil if no AI assistant is detected.
func (d *Detector) Detect(command string) *AIAssistantInfo {
	if command == "" {
		return nil
	}

	d.mu.RLock()
	defer d.mu.RUnlock()

	for _, rule := range d.rules {
		if rule.Match(command) {
			return &AIAssistantInfo{
				Type:        rule.Type,
				Name:        string(rule.Type),
				DisplayName: rule.Type.DisplayName(),
				Detected:    true,
				Command:     command,
			}
		}
	}

	return nil
}

// DetectMultiple checks multiple commands and returns all detected assistants.
func (d *Detector) DetectMultiple(commands []string) []*AIAssistantInfo {
	var results []*AIAssistantInfo

	for _, cmd := range commands {
		if info := d.Detect(cmd); info != nil {
			results = append(results, info)
		}
	}

	return results
}

// IsAIAssistant checks if the given command is running an AI assistant.
func (d *Detector) IsAIAssistant(command string) bool {
	return d.Detect(command) != nil
}

// GetAssistantType returns the type of AI assistant, or empty string if none detected.
func (d *Detector) GetAssistantType(command string) AIAssistantType {
	if info := d.Detect(command); info != nil {
		return info.Type
	}
	return AIAssistantUnknown
}

// AddRule adds a custom detection rule.
func (d *Detector) AddRule(rule DetectionRule) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.rules = append(d.rules, rule)
}

// GetRules returns a copy of current detection rules.
func (d *Detector) GetRules() []DetectionRule {
	d.mu.RLock()
	defer d.mu.RUnlock()

	rules := make([]DetectionRule, len(d.rules))
	copy(rules, d.rules)
	return rules
}

// Default detector instance for package-level functions.
var defaultDetector = NewDetector()

// Detect uses the default detector to analyze a command.
func Detect(command string) *AIAssistantInfo {
	return defaultDetector.Detect(command)
}

// IsAIAssistant uses the default detector to check if command is an AI assistant.
func IsAIAssistant(command string) bool {
	return defaultDetector.IsAIAssistant(command)
}

// GetAssistantType uses the default detector to get the assistant type.
func GetAssistantType(command string) AIAssistantType {
	return defaultDetector.GetAssistantType(command)
}
