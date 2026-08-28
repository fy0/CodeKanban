package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	processutil "code-kanban/utils/process"
)

type Engine string

const (
	EngineBuiltin     Engine = "builtin"
	EngineSystem      Engine = "system"
	EngineUnavailable Engine = "unavailable"

	systemGitVersionProbeTimeout    = 3 * time.Second
	systemGitRepositoryProbeTimeout = 3 * time.Second
	systemGitCommandTimeout         = 30 * time.Second
	systemGitCancelTimeout          = 500 * time.Millisecond
	systemGitWaitDelay              = time.Second
)

type EnginePreference string

const (
	EnginePreferenceAuto    EnginePreference = "auto"
	EnginePreferenceBuiltin EnginePreference = "builtin"
	EnginePreferenceSystem  EnginePreference = "system"
)

type EngineSettings struct {
	Read       EnginePreference `json:"read"`
	Write      EnginePreference `json:"write"`
	Executable string           `json:"executable,omitempty"`
}

type SystemGitInfo struct {
	Available  bool   `json:"available"`
	Executable string `json:"executable,omitempty"`
	Version    string `json:"version,omitempty"`
	Error      string `json:"error,omitempty"`
}

var engineRuntime = struct {
	sync.RWMutex
	settings EngineSettings
	probeKey string
	probeAt  time.Time
	probe    SystemGitInfo
}{
	settings: EngineSettings{
		Read:  EnginePreferenceAuto,
		Write: EnginePreferenceAuto,
	},
}

var (
	testEnvOverride   []string
	testEnvOverrideMu sync.RWMutex
)

func NormalizeEnginePreference(value EnginePreference) EnginePreference {
	switch EnginePreference(strings.ToLower(strings.TrimSpace(string(value)))) {
	case EnginePreferenceBuiltin:
		return EnginePreferenceBuiltin
	case EnginePreferenceSystem:
		return EnginePreferenceSystem
	default:
		return EnginePreferenceAuto
	}
}

func ConfigureEngines(settings EngineSettings) {
	normalized := EngineSettings{
		Read:       NormalizeEnginePreference(settings.Read),
		Write:      NormalizeEnginePreference(settings.Write),
		Executable: strings.TrimSpace(settings.Executable),
	}
	engineRuntime.Lock()
	changed := engineRuntime.settings != normalized
	engineRuntime.settings = normalized
	if changed {
		engineRuntime.probeKey = ""
		engineRuntime.probeAt = time.Time{}
		engineRuntime.probe = SystemGitInfo{}
	}
	engineRuntime.Unlock()
	if changed {
		clearCapabilityCache()
	}
}

func CurrentEngineSettings() EngineSettings {
	engineRuntime.RLock()
	defer engineRuntime.RUnlock()
	return engineRuntime.settings
}

func ProbeSystemGit(ctx context.Context, refresh bool) SystemGitInfo {
	if ctx == nil {
		ctx = context.Background()
	}
	settings := CurrentEngineSettings()
	key := settings.Executable + "\x00" + os.Getenv("PATH")

	engineRuntime.RLock()
	if !refresh && engineRuntime.probeKey == key && time.Since(engineRuntime.probeAt) < time.Minute {
		result := engineRuntime.probe
		engineRuntime.RUnlock()
		return result
	}
	engineRuntime.RUnlock()

	result := probeSystemGit(ctx, settings.Executable)
	engineRuntime.Lock()
	engineRuntime.probeKey = key
	engineRuntime.probeAt = time.Now()
	engineRuntime.probe = result
	engineRuntime.Unlock()
	return result
}

func probeSystemGit(ctx context.Context, configured string) SystemGitInfo {
	candidate := strings.TrimSpace(configured)
	if candidate == "" {
		candidate = "git"
	}
	resolved, err := exec.LookPath(candidate)
	if err != nil {
		return SystemGitInfo{Error: fmt.Sprintf("system Git was not found: %v", err)}
	}
	if absolute, absErr := filepath.Abs(resolved); absErr == nil {
		resolved = absolute
	}

	probeCtx, cancel := withSystemGitTimeout(ctx, systemGitVersionProbeTimeout)
	defer cancel()
	cmd := exec.CommandContext(probeCtx, resolved, "--version")
	cmd.Env = buildGitCommandEnv()
	output, err := cmd.Output()
	if err != nil {
		if errors.Is(probeCtx.Err(), context.DeadlineExceeded) {
			return SystemGitInfo{Executable: resolved, Error: "system Git version probe timed out"}
		}
		return SystemGitInfo{Executable: resolved, Error: fmt.Sprintf("system Git cannot be executed: %v", err)}
	}
	return SystemGitInfo{
		Available:  true,
		Executable: resolved,
		Version:    strings.TrimSpace(string(output)),
	}
}

func buildGitCommandEnv() []string {
	env := os.Environ()
	env = append(env,
		"GIT_TERMINAL_PROMPT=0",
		"GIT_MERGE_AUTOEDIT=no",
		"GIT_ASKPASS=",
		"SSH_ASKPASS=",
	)
	testEnvOverrideMu.RLock()
	if len(testEnvOverride) > 0 {
		env = append(env, testEnvOverride...)
	}
	testEnvOverrideMu.RUnlock()
	return env
}

// SetTestEnvOverride allows tests to inject additional environment variables
// into system Git commands. Call with nil to clear the override.
func SetTestEnvOverride(env []string) {
	testEnvOverrideMu.Lock()
	defer testEnvOverrideMu.Unlock()
	testEnvOverride = append([]string(nil), env...)
}

func newSystemGitCommandContext(ctx context.Context, dir string, args ...string) (*exec.Cmd, error) {
	info := ProbeSystemGit(ctx, false)
	if !info.Available {
		detail := info.Error
		if detail == "" {
			detail = "system Git is unavailable"
		}
		return nil, &OperationError{Code: ErrorCodeSystemGitUnavailable, Detail: detail}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := exec.CommandContext(ctx, info.Executable, args...)
	cmd.Cancel = func() error {
		return cancelSystemGitCommand(cmd)
	}
	cmd.WaitDelay = systemGitWaitDelay
	cmd.Env = buildGitCommandEnv()
	if strings.TrimSpace(dir) != "" {
		cmd.Dir = dir
	}
	return cmd, nil
}

func cancelSystemGitCommand(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return os.ErrProcessDone
	}
	treeResult := make(chan error, 1)
	go func() {
		treeResult <- processutil.KillProcessTree(int32(cmd.Process.Pid))
	}()
	timer := time.NewTimer(systemGitCancelTimeout)
	defer timer.Stop()
	select {
	case err := <-treeResult:
		if err == nil {
			return nil
		}
	case <-timer.C:
	}
	if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	return nil
}

func runSystemGit(ctx context.Context, dir string, operation GitOperation, args ...string) ([]byte, error) {
	commandCtx, cancel := withSystemGitTimeout(ctx, systemGitCommandTimeout)
	defer cancel()
	cmd, err := newSystemGitCommandContext(commandCtx, dir, args...)
	if err != nil {
		return nil, err
	}
	output, err := cmd.CombinedOutput()
	if err == nil {
		return output, nil
	}
	detail := strings.TrimSpace(string(output))
	if detail == "" {
		detail = err.Error()
	}
	return output, mapSystemGitCommandError(commandCtx, operation, detail, err)
}

// runSystemGitOutput keeps diagnostic output separate from stdout so callers
// can safely parse porcelain and other machine-readable Git output.
func runSystemGitOutput(ctx context.Context, dir string, operation GitOperation, args ...string) ([]byte, error) {
	return runSystemGitOutputWithExitOne(ctx, dir, operation, false, args...)
}

func runSystemGitOutputAllowExitOne(ctx context.Context, dir string, operation GitOperation, args ...string) ([]byte, error) {
	return runSystemGitOutputWithExitOne(ctx, dir, operation, true, args...)
}

func runSystemGitOutputWithExitOne(
	ctx context.Context,
	dir string,
	operation GitOperation,
	allowExitOne bool,
	args ...string,
) ([]byte, error) {
	commandCtx, cancel := withSystemGitTimeout(ctx, systemGitCommandTimeout)
	defer cancel()
	cmd, err := newSystemGitCommandContext(commandCtx, dir, args...)
	if err != nil {
		return nil, err
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err == nil {
		return stdout.Bytes(), nil
	}
	var exitErr *exec.ExitError
	if allowExitOne && errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return stdout.Bytes(), nil
	}
	detail := strings.TrimSpace(stderr.String())
	if detail == "" {
		detail = strings.TrimSpace(stdout.String())
	}
	if detail == "" {
		detail = err.Error()
	}
	return stdout.Bytes(), mapSystemGitCommandError(commandCtx, operation, detail, err)
}

func withSystemGitTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(ctx, timeout)
}

func mapSystemGitCommandError(ctx context.Context, operation GitOperation, detail string, err error) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return &OperationError{
			Code:      ErrorCodeSystemGitFailed,
			Operation: operation,
			Detail:    "system Git command timed out",
			Err:       context.DeadlineExceeded,
		}
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return &OperationError{
			Code:      ErrorCodeSystemGitFailed,
			Operation: operation,
			Detail:    "system Git command canceled",
			Err:       context.Canceled,
		}
	}
	return mapSystemGitError(operation, detail, err)
}

func mapSystemGitError(operation GitOperation, detail string, err error) error {
	normalized := strings.ToLower(detail)
	code := ErrorCodeSystemGitFailed
	switch {
	case strings.Contains(normalized, "not a git repository"):
		code = ErrorCodeNotRepository
	case strings.Contains(normalized, "index.lock") || strings.Contains(normalized, "another git process") || strings.Contains(normalized, "unable to create") && strings.Contains(normalized, ".lock"):
		code = ErrorCodeRepositoryLocked
	case strings.Contains(normalized, "non-fast-forward") || strings.Contains(normalized, "not possible to fast-forward"):
		code = ErrorCodeNonFastForward
	case strings.Contains(normalized, "conflict"):
		code = ErrorCodeMergeConflict
	case strings.Contains(normalized, "contains modified or untracked files") || strings.Contains(normalized, "worktree has uncommitted changes"):
		code = ErrorCodeWorktreeDirty
	case strings.Contains(normalized, "unknown revision") || strings.Contains(normalized, "not a valid object name") || strings.Contains(normalized, "invalid reference"):
		code = ErrorCodeInvalidReference
	}
	return &OperationError{Code: code, Operation: operation, Detail: detail, Err: err}
}

func systemGitRepositoryAvailable(ctx context.Context, path string) bool {
	probeCtx, cancel := withSystemGitTimeout(ctx, systemGitRepositoryProbeTimeout)
	defer cancel()
	if strings.TrimSpace(path) == "" || !ProbeSystemGit(probeCtx, false).Available {
		return false
	}
	cmd, err := newSystemGitCommandContext(probeCtx, "", "-C", path, "rev-parse", "--git-dir")
	if err != nil {
		return false
	}
	return cmd.Run() == nil
}

func preferenceForOperation(operation GitOperation) EnginePreference {
	settings := CurrentEngineSettings()
	switch operation {
	case OperationBranchesRead, OperationStatus, OperationDiff, OperationWorktreesRead:
		return settings.Read
	default:
		return settings.Write
	}
}

func autoPrefersSystem(GitOperation) bool {
	// Auto keeps the built-in engine as a fallback, while using system Git
	// whenever it can service the requested repository operation.
	return true
}
