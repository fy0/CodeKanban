package ai_assistant2

import (
	"path"
	"strings"

	"code-kanban/utils/ai_assistant2/types"
)

// DetectionRule defines how to detect a specific AI assistant from command
type DetectionRule struct {
	Type                  types.AssistantType
	Patterns              []string // Command line patterns to match (case-insensitive)
	ExecutableNames       []string // Executable names to match exactly (case-insensitive)
	PatternsInExecutables bool     // Restrict patterns to executable/script tokens
	Description           string
}

var defaultRules = []DetectionRule{
	{
		Type: types.AssistantTypeClaudeCode,
		Patterns: []string{
			"@anthropic-ai/claude-code",
			"claude-code/cli.js",
			"claude-code/bin/",
			"ccr code",
			"ccr.cmd code",
			"ccr.ps1 code",
			"claude-code-router/dist/cli.js code",
			"@musistudio/claude-code-router/dist/cli.js code",
		},
		ExecutableNames: []string{
			"claude",
			"claude-code",
		},
		Description: "Detects Anthropic Claude Code CLI",
	},
	{
		Type: types.AssistantTypeCodex,
		Patterns: []string{
			"@openai/codex",
			"codex/bin/codex.js",
			"codex.js",
		},
		ExecutableNames: []string{
			"codex",
		},
		Description: "Detects OpenAI Codex CLI",
	},
	{
		Type: types.AssistantTypePi,
		Patterns: []string{
			"@earendil-works/pi-coding-agent",
			"@mariozechner/pi-coding-agent",
			"pi-coding-agent/dist/cli.js",
		},
		ExecutableNames: []string{
			"pi",
			"pi.exe",
			"pi.cmd",
			"pi.ps1",
		},
		PatternsInExecutables: true,
		Description:           "Detects Pi coding agent CLI",
	},
	{
		Type: types.AssistantTypeQwenCode,
		Patterns: []string{
			"@qwen-code/qwen-code",
			"qwen-code/cli.js",
			"qwen-code/bin/",
		},
		Description: "Detects Qwen Code CLI",
	},
	{
		Type: types.AssistantTypeGemini,
		Patterns: []string{
			"@google/gemini-cli",
			"gemini-cli/dist/index.js",
			"gemini-cli/bin/",
		},
		Description: "Detects Google Gemini CLI",
	},
}

// Match checks if the command matches this rule
func (r *DetectionRule) Match(command string) bool {
	if command == "" {
		return false
	}

	normalizedCmd := normalizeCommand(command)

	candidates := candidateExecutables(normalizedCmd)
	for _, pattern := range r.Patterns {
		normalizedPattern := strings.ToLower(pattern)
		if !r.PatternsInExecutables && strings.Contains(normalizedCmd, normalizedPattern) {
			return true
		}
		if r.PatternsInExecutables {
			for _, candidate := range candidates {
				if strings.Contains(candidate, normalizedPattern) {
					return true
				}
			}
		}
	}

	if len(r.ExecutableNames) == 0 {
		return false
	}
	for _, candidate := range candidates {
		if r.matchesExecutable(candidate) {
			return true
		}
	}

	return false
}

func normalizeCommand(command string) string {
	normalized := strings.ToLower(strings.TrimSpace(command))
	return strings.ReplaceAll(normalized, "\\", "/")
}

func splitCommandTokens(command string) []string {
	command = strings.TrimSpace(command)
	if command == "" {
		return nil
	}

	tokens := make([]string, 0, 8)
	var current strings.Builder
	var quote rune
	flush := func() {
		if current.Len() == 0 {
			return
		}
		tokens = append(tokens, current.String())
		current.Reset()
	}
	chars := []rune(command)
	for index, char := range chars {
		if quote != 0 {
			if char == quote {
				// cmd.exe wraps /c payloads as ""executable" args". The
				// second opening quote is a wrapper delimiter, not an empty arg.
				if current.Len() == 0 && index+1 < len(chars) && !isCommandSpace(chars[index+1]) {
					continue
				}
				quote = 0
			} else {
				current.WriteRune(char)
			}
			continue
		}
		switch char {
		case '"', '\'':
			quote = char
		case ' ', '\t', '\r', '\n':
			flush()
		default:
			current.WriteRune(char)
		}
	}
	flush()
	return tokens
}

func isCommandSpace(char rune) bool {
	switch char {
	case ' ', '\t', '\r', '\n':
		return true
	default:
		return false
	}
}

func candidateExecutables(command string) []string {
	tokens := splitCommandTokens(command)
	if len(tokens) == 0 {
		return nil
	}

	candidates := make([]string, 0, 4)
	appendCandidate := func(token string) {
		token = strings.TrimSpace(token)
		if token == "" {
			return
		}
		candidates = append(candidates, token)
	}

	appendCandidate(tokens[0])

	if isNodeRuntime(tokens[0]) && len(tokens) > 1 {
		appendCandidate(tokens[1])
	}

	if isShellExecutable(tokens[0]) {
		scriptTokens := extractShellCommandTokens(tokens)
		if len(scriptTokens) > 0 {
			appendCandidate(scriptTokens[0])
			if isNodeRuntime(scriptTokens[0]) && len(scriptTokens) > 1 {
				appendCandidate(scriptTokens[1])
			}
		}
	}

	return candidates
}

func extractShellCommandTokens(tokens []string) []string {
	if len(tokens) < 2 {
		return nil
	}

	for idx := 1; idx < len(tokens); idx++ {
		token := strings.ToLower(tokens[idx])
		if token == "-c" || token == "-lc" || token == "-cl" || token == "/c" || token == "-command" {
			if idx+1 >= len(tokens) {
				return nil
			}
			scriptTokens := append([]string(nil), tokens[idx+1:]...)
			if len(scriptTokens) == 1 {
				scriptTokens = splitCommandTokens(scriptTokens[0])
			}
			if len(scriptTokens) > 0 && scriptTokens[0] == "&" {
				scriptTokens = scriptTokens[1:]
			}
			return scriptTokens
		}
	}

	return nil
}

func isNodeRuntime(token string) bool {
	base := path.Base(token)
	switch base {
	case "node", "node.exe":
		return true
	default:
		return false
	}
}

func isShellExecutable(token string) bool {
	base := path.Base(token)
	switch base {
	case "bash", "bash.exe", "sh", "sh.exe", "zsh", "zsh.exe", "fish", "fish.exe",
		"cmd", "cmd.exe", "powershell", "powershell.exe", "pwsh", "pwsh.exe":
		return true
	default:
		return false
	}
}

func (r *DetectionRule) matchesExecutable(token string) bool {
	base := path.Base(token)
	for _, executable := range r.ExecutableNames {
		executable = strings.ToLower(strings.TrimSpace(executable))
		if executable == "" {
			continue
		}
		if token == executable || base == executable {
			return true
		}
	}
	return false
}

// AssistantDetector detects AI assistant type from command
type AssistantDetector struct {
	rules []DetectionRule
}

// NewAssistantDetector creates a new AI assistant detector
func NewAssistantDetector() *AssistantDetector {
	return &AssistantDetector{
		rules: defaultRules,
	}
}

// DetectFromCommand analyzes a command string and returns the AI assistant type
func (d *AssistantDetector) DetectFromCommand(command string) *types.AssistantInfo {
	if command == "" {
		return nil
	}

	for _, rule := range d.rules {
		if rule.Match(command) {
			return &types.AssistantInfo{
				Type:        rule.Type,
				Name:        string(rule.Type),
				DisplayName: rule.Type.DisplayName(),
				Command:     command,
				Detected:    true,
			}
		}
	}

	return nil
}

var defaultDetector = NewAssistantDetector()

// DetectFromCommand uses the default detector to analyze a command
func DetectFromCommand(command string) *types.AssistantInfo {
	return defaultDetector.DetectFromCommand(command)
}
