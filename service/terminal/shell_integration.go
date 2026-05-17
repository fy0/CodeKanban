package terminal

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	shellIntegrationFamilyBash = "bash"
	shellIntegrationFamilyZsh  = "zsh"
)

var shellIntegrationSequencePrefix = []byte("\x1b]633;CodeKanban;")

type shellIntegrationEventType string

const (
	shellIntegrationEventCwd         shellIntegrationEventType = "cwd"
	shellIntegrationEventCommand     shellIntegrationEventType = "cmd"
	shellIntegrationEventPromptReady shellIntegrationEventType = "prompt"
)

type shellIntegrationEvent struct {
	Type       shellIntegrationEventType
	Value      string
	OccurredAt time.Time
}

type shellIntegrationConfig struct {
	Family    string
	Supported bool
	Cleanup   func()
}

type shellIntegrationState struct {
	mu                   sync.Mutex
	family               string
	supported            bool
	cleanup              func()
	partial              []byte
	pendingCommand       string
	replayEligible       bool
	commandStartedAt     time.Time
	shellState           string
	startupReplayCommand string
	startupReplaySent    bool
}

func prepareShellIntegration(command []string) ([]string, []string, shellIntegrationConfig, error) {
	cloned := append([]string{}, command...)
	if runtime.GOOS == "windows" || len(cloned) == 0 {
		return cloned, nil, shellIntegrationConfig{}, nil
	}

	family := detectShellIntegrationFamily(cloned[0])
	cfg := shellIntegrationConfig{Family: family}
	if family == "" {
		return cloned, nil, cfg, nil
	}

	// Keep v1 support intentionally narrow: the default interactive shell path,
	// or a custom bash/zsh binary with no extra arguments. Other variants still
	// restore tabs and cwd, but skip command replay hooks.
	if len(cloned) != 1 {
		return cloned, nil, cfg, nil
	}

	switch family {
	case shellIntegrationFamilyBash:
		return prepareBashShellIntegration(cloned[0])
	case shellIntegrationFamilyZsh:
		return prepareZshShellIntegration(cloned[0])
	default:
		return cloned, nil, cfg, nil
	}
}

func detectShellIntegrationFamily(command string) string {
	base := strings.ToLower(filepath.Base(strings.TrimSpace(command)))
	switch {
	case strings.HasPrefix(base, "bash"):
		return shellIntegrationFamilyBash
	case strings.HasPrefix(base, "zsh"):
		return shellIntegrationFamilyZsh
	default:
		return ""
	}
}

func prepareBashShellIntegration(shellPath string) ([]string, []string, shellIntegrationConfig, error) {
	dir, err := os.MkdirTemp("", "codekanban-bash-*")
	if err != nil {
		return []string{shellPath}, nil, shellIntegrationConfig{Family: shellIntegrationFamilyBash}, err
	}

	rcPath := filepath.Join(dir, "bashrc")
	if err := os.WriteFile(rcPath, []byte(buildBashShellIntegrationScript()), 0o600); err != nil {
		_ = os.RemoveAll(dir)
		return []string{shellPath}, nil, shellIntegrationConfig{Family: shellIntegrationFamilyBash}, err
	}

	return []string{shellPath, "--rcfile", rcPath, "-i"}, nil, shellIntegrationConfig{
		Family:    shellIntegrationFamilyBash,
		Supported: true,
		Cleanup: func() {
			_ = os.RemoveAll(dir)
		},
	}, nil
}

func prepareZshShellIntegration(shellPath string) ([]string, []string, shellIntegrationConfig, error) {
	dir, err := os.MkdirTemp("", "codekanban-zsh-*")
	if err != nil {
		return []string{shellPath}, nil, shellIntegrationConfig{Family: shellIntegrationFamilyZsh}, err
	}

	if err := os.WriteFile(filepath.Join(dir, ".zshenv"), []byte(buildZshEnvIntegrationScript()), 0o600); err != nil {
		_ = os.RemoveAll(dir)
		return []string{shellPath}, nil, shellIntegrationConfig{Family: shellIntegrationFamilyZsh}, err
	}
	if err := os.WriteFile(filepath.Join(dir, ".zshrc"), []byte(buildZshRcIntegrationScript()), 0o600); err != nil {
		_ = os.RemoveAll(dir)
		return []string{shellPath}, nil, shellIntegrationConfig{Family: shellIntegrationFamilyZsh}, err
	}

	userZdotdir := strings.TrimSpace(os.Getenv("ZDOTDIR"))
	if userZdotdir == "" {
		userZdotdir = strings.TrimSpace(os.Getenv("HOME"))
	}

	return []string{shellPath, "-i"}, []string{
			"ZDOTDIR=" + dir,
			"CODEKANBAN_INTEGRATION_ZDOTDIR=" + dir,
			"CODEKANBAN_USER_ZDOTDIR=" + userZdotdir,
		}, shellIntegrationConfig{
			Family:    shellIntegrationFamilyZsh,
			Supported: true,
			Cleanup: func() {
				_ = os.RemoveAll(dir)
			},
		}, nil
}

func buildBashShellIntegrationScript() string {
	return `
if [ -f "$HOME/.bashrc" ]; then
  . "$HOME/.bashrc"
fi

__codekanban_b64() {
  command -v base64 >/dev/null 2>&1 || return 0
  printf '%s' "$1" | base64 | tr -d '\r\n'
}

__codekanban_emit() {
  local kind="$1"
  local payload=""
  if [ "$#" -ge 2 ]; then
    payload="$(__codekanban_b64 "$2")"
  fi
  if [ -z "$payload" ] && [ "$kind" != "prompt" ]; then
    return 0
  fi
  printf '\033]633;CodeKanban;%s;%s\a' "$kind" "$payload"
}

__codekanban_preexec() {
  local current="$BASH_COMMAND"
  case "$current" in
    __codekanban_* ) return 0 ;;
  esac
  if [ -n "${COMP_LINE-}" ] || [ "${__codekanban_in_command:-0}" = "1" ]; then
    return 0
  fi
  __codekanban_in_command=1
  __codekanban_emit "cmd" "$current"
}

__codekanban_precmd() {
  local status=$?
  if [ "${__codekanban_in_command:-0}" = "1" ]; then
    __codekanban_in_command=0
    __codekanban_emit "prompt" ""
  fi
  __codekanban_emit "cwd" "$PWD"
  return $status
}

trap '__codekanban_preexec' DEBUG
if [ -n "${PROMPT_COMMAND-}" ]; then
  PROMPT_COMMAND="__codekanban_precmd;${PROMPT_COMMAND}"
else
  PROMPT_COMMAND="__codekanban_precmd"
fi
`
}

func buildZshEnvIntegrationScript() string {
	return `
if [ -n "${CODEKANBAN_USER_ZDOTDIR-}" ] && [ -f "${CODEKANBAN_USER_ZDOTDIR}/.zshenv" ]; then
  export ZDOTDIR="${CODEKANBAN_USER_ZDOTDIR}"
  source "${CODEKANBAN_USER_ZDOTDIR}/.zshenv"
fi
export ZDOTDIR="${CODEKANBAN_INTEGRATION_ZDOTDIR}"
`
}

func buildZshRcIntegrationScript() string {
	return `
if [ -n "${CODEKANBAN_USER_ZDOTDIR-}" ] && [ -f "${CODEKANBAN_USER_ZDOTDIR}/.zshrc" ]; then
  export ZDOTDIR="${CODEKANBAN_USER_ZDOTDIR}"
  source "${CODEKANBAN_USER_ZDOTDIR}/.zshrc"
fi
export ZDOTDIR="${CODEKANBAN_INTEGRATION_ZDOTDIR}"

autoload -Uz add-zsh-hook

__codekanban_b64() {
  command -v base64 >/dev/null 2>&1 || return 0
  printf '%s' "$1" | base64 | tr -d '\r\n'
}

__codekanban_emit() {
  local kind="$1"
  local payload=""
  if [ "$#" -ge 2 ]; then
    payload="$(__codekanban_b64 "$2")"
  fi
  if [ -z "$payload" ] && [ "$kind" != "prompt" ]; then
    return 0
  fi
  printf '\033]633;CodeKanban;%s;%s\a' "$kind" "$payload"
}

__codekanban_preexec() {
  __codekanban_emit "cmd" "$1"
}

__codekanban_precmd() {
  __codekanban_emit "prompt" ""
  __codekanban_emit "cwd" "$PWD"
}

add-zsh-hook preexec __codekanban_preexec
add-zsh-hook precmd __codekanban_precmd
`
}

func (s *Session) cleanupShellIntegration() {
	s.shellIntegration.mu.Lock()
	cleanup := s.shellIntegration.cleanup
	s.shellIntegration.cleanup = nil
	s.shellIntegration.partial = nil
	s.shellIntegration.mu.Unlock()

	if cleanup != nil {
		cleanup()
	}
}

func (s *Session) stripShellIntegrationSequences(chunk []byte) []byte {
	s.shellIntegration.mu.Lock()
	if len(s.shellIntegration.partial) > 0 {
		chunk = append(append([]byte{}, s.shellIntegration.partial...), chunk...)
		s.shellIntegration.partial = nil
	}
	s.shellIntegration.mu.Unlock()

	if len(chunk) == 0 {
		return nil
	}

	output := make([]byte, 0, len(chunk))
	cursor := 0
	for cursor < len(chunk) {
		offset := bytes.Index(chunk[cursor:], shellIntegrationSequencePrefix)
		if offset == -1 {
			output = append(output, chunk[cursor:]...)
			break
		}
		start := cursor + offset
		output = append(output, chunk[cursor:start]...)

		payloadStart := start + len(shellIntegrationSequencePrefix)
		terminatorOffset := bytes.IndexByte(chunk[payloadStart:], '\a')
		if terminatorOffset == -1 {
			s.shellIntegration.mu.Lock()
			s.shellIntegration.partial = append([]byte{}, chunk[start:]...)
			s.shellIntegration.mu.Unlock()
			break
		}

		payloadEnd := payloadStart + terminatorOffset
		s.handleShellIntegrationSequence(chunk[payloadStart:payloadEnd])
		cursor = payloadEnd + 1
	}

	return output
}

func (s *Session) handleShellIntegrationSequence(payload []byte) {
	if len(payload) == 0 {
		return
	}
	parts := strings.SplitN(string(payload), ";", 2)
	if len(parts) != 2 {
		return
	}

	kind := shellIntegrationEventType(strings.TrimSpace(parts[0]))
	encoded := strings.TrimSpace(parts[1])
	value := ""
	if encoded != "" {
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return
		}
		value = string(decoded)
	}

	event := shellIntegrationEvent{
		Type:       kind,
		Value:      value,
		OccurredAt: time.Now(),
	}

	var replayCommand string
	switch kind {
	case shellIntegrationEventCwd:
		s.setWorkingDir(value)
	case shellIntegrationEventCommand:
		replayEligible := isReplayEligibleCommand(value)
		s.shellIntegration.mu.Lock()
		s.shellIntegration.pendingCommand = strings.TrimSpace(value)
		s.shellIntegration.replayEligible = replayEligible
		s.shellIntegration.commandStartedAt = event.OccurredAt
		s.shellIntegration.shellState = terminalRestoreShellStateRunning
		s.shellIntegration.mu.Unlock()
	case shellIntegrationEventPromptReady:
		s.shellIntegration.mu.Lock()
		s.shellIntegration.pendingCommand = ""
		s.shellIntegration.replayEligible = false
		s.shellIntegration.commandStartedAt = time.Time{}
		s.shellIntegration.shellState = terminalRestoreShellStateIdle
		if !s.shellIntegration.startupReplaySent && strings.TrimSpace(s.shellIntegration.startupReplayCommand) != "" {
			replayCommand = s.shellIntegration.startupReplayCommand
			s.shellIntegration.startupReplaySent = true
		}
		s.shellIntegration.mu.Unlock()
	default:
		return
	}

	if s.shellEventCallback != nil {
		s.shellEventCallback(event)
	}

	if replayCommand != "" {
		go s.replayStartupCommand(replayCommand)
	}
}

func (s *Session) replayStartupCommand(command string) {
	command = strings.TrimSpace(command)
	if command == "" {
		return
	}
	time.Sleep(20 * time.Millisecond)
	_, _ = s.Write([]byte(command + "\n"))
}

func isReplayEligibleCommand(command string) bool {
	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		return false
	}
	if strings.Contains(trimmed, "\n") || strings.Contains(trimmed, "\r") {
		return false
	}

	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return false
	}

	switch fields[0] {
	case "cd", "pushd", "popd", "clear", "reset", "exit", "logout", "fg", "bg", "jobs", "history":
		return false
	default:
		return true
	}
}
