package websession

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
)

const (
	piMinVersion             = "0.84.1"
	piProbeSuccessCacheTTL   = 5 * time.Minute
	piProbeFailureCacheTTL   = 5 * time.Second
	piProbeTimeout           = 5 * time.Second
	piProbeMaxFrameBytes     = 1024 * 1024
	piDiagnosticNotInstalled = "not_installed"
	piDiagnosticVersion      = "version_unknown"
	piDiagnosticTooOld       = "version_too_old"
	piDiagnosticStart        = "rpc_start_failed"
	piDiagnosticProtocol     = "rpc_protocol_incompatible"
	piDiagnosticTimeout      = "rpc_timeout"
)

var piMinimumVersion = semver.MustParse(piMinVersion)

type piRuntimeProbeResult struct {
	installed  bool
	version    *string
	compatible bool
	diagnostic string
	models     []PiModelInfo
}

type piRuntimeProbeCache struct {
	result    piRuntimeProbeResult
	expiresAt time.Time
	loaded    bool
}

type piProbeResponse struct {
	Type    string `json:"type"`
	ID      string `json:"id"`
	Command string `json:"command"`
	Success bool   `json:"success"`
}

func (m *Manager) applyPiRuntimeCapabilities(config WebSessionRuntimeConfig) WebSessionRuntimeConfig {
	config.PiMinVersion = piMinVersion
	if m == nil {
		config.Agents = runtimeAgentCapabilities(config)
		return config
	}

	probe := m.getPiRuntimeProbe()
	config.HasPi = probe.installed
	config.PiVersion = probe.version
	config.PiRPCCompatible = probe.compatible
	config.PiDiagnostics = probe.diagnostic
	config.PiModels = append([]PiModelInfo(nil), probe.models...)
	config.SupportsPiWebSession = probe.compatible
	config.Agents = runtimeAgentCapabilities(config)
	return config
}

func (m *Manager) getPiRuntimeProbe() piRuntimeProbeResult {
	m.piProbeMu.Lock()
	defer m.piProbeMu.Unlock()

	now := time.Now()
	if m.piProbe.loaded && now.Before(m.piProbe.expiresAt) {
		return m.piProbe.result
	}

	result := probePiRuntime(m.cfg.PiPath, m.cfg.DataDir)
	ttl := piProbeFailureCacheTTL
	if result.compatible {
		ttl = piProbeSuccessCacheTTL
	}
	m.piProbe = piRuntimeProbeCache{
		result:    result,
		expiresAt: now.Add(ttl),
		loaded:    true,
	}
	return result
}

func probePiRuntime(command, workingDir string) piRuntimeProbeResult {
	result := piRuntimeProbeResult{}
	if !hasExecutable(command) {
		result.diagnostic = piDiagnosticNotInstalled
		return result
	}
	result.installed = true

	version := detectPiVersion(command)
	if version == nil {
		result.diagnostic = piDiagnosticVersion
		return result
	}
	result.version = version
	parsedVersion, err := semver.NewVersion(*version)
	if err != nil {
		result.diagnostic = piDiagnosticVersion
		return result
	}
	if parsedVersion.LessThan(piMinimumVersion) {
		result.diagnostic = piDiagnosticTooOld
		return result
	}

	ctx, cancel := context.WithTimeout(context.Background(), piProbeTimeout)
	defer cancel()
	if err := runPiRPCProbe(ctx, command, workingDir); err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded), errors.Is(ctx.Err(), context.DeadlineExceeded):
			result.diagnostic = piDiagnosticTimeout
		case errors.Is(err, errPiProbeStart):
			result.diagnostic = piDiagnosticStart
		default:
			result.diagnostic = piDiagnosticProtocol
		}
		return result
	}

	result.compatible = true
	modelCtx, modelCancel := context.WithTimeout(context.Background(), piProbeTimeout)
	result.models, _ = loadPiModelCatalog(modelCtx, command, workingDir)
	modelCancel()
	return result
}

func loadPiModelCatalog(ctx context.Context, command, workingDir string) ([]PiModelInfo, error) {
	cmd, err := buildPiCommand(
		ctx,
		command,
		"--mode", "rpc",
		"--no-session",
		"--no-approve",
		"--no-extensions",
		"--no-skills",
		"--no-prompt-templates",
		"--no-themes",
	)
	if err != nil {
		return nil, err
	}
	if dir := strings.TrimSpace(workingDir); dir != "" {
		if info, statErr := os.Stat(dir); statErr == nil && info.IsDir() {
			cmd.Dir = dir
		}
	}
	client, err := startPiRPCClient(cmd)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	var response struct {
		Models []PiModelInfo `json:"models"`
	}
	if err := client.Request(ctx, "get_available_models", nil, &response); err != nil {
		return nil, err
	}
	models := response.Models[:0]
	seen := make(map[string]struct{}, len(response.Models))
	for _, model := range response.Models {
		model.Provider = strings.TrimSpace(model.Provider)
		model.ID = strings.TrimSpace(model.ID)
		model.Name = strings.TrimSpace(model.Name)
		if model.Provider == "" || model.ID == "" {
			continue
		}
		key := strings.ToLower(model.Provider + "/" + model.ID)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		model.Input = append([]string(nil), model.Input...)
		models = append(models, model)
	}
	sort.SliceStable(models, func(i, j int) bool {
		left := strings.ToLower(models[i].Provider + "/" + models[i].Name + "/" + models[i].ID)
		right := strings.ToLower(models[j].Provider + "/" + models[j].Name + "/" + models[j].ID)
		return left < right
	})
	return append([]PiModelInfo(nil), models...), nil
}

func detectPiVersion(command string) *string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd, err := buildPiCommand(ctx, command, "--version")
	if err != nil {
		return nil
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil
	}
	match := codexVersionPattern.FindString(string(output))
	if strings.TrimSpace(match) == "" {
		return nil
	}
	version := strings.TrimSpace(match)
	return &version
}

var errPiProbeStart = errors.New("pi probe process failed to start")

func runPiRPCProbe(ctx context.Context, command, workingDir string) error {
	cmd, err := buildPiCommand(
		ctx,
		command,
		"--mode", "rpc",
		"--no-session",
		"--no-approve",
		"--no-extensions",
		"--no-skills",
		"--no-prompt-templates",
		"--no-themes",
	)
	if err != nil {
		return fmt.Errorf("%w: %v", errPiProbeStart, err)
	}
	if dir := strings.TrimSpace(workingDir); dir != "" {
		if info, statErr := os.Stat(dir); statErr == nil && info.IsDir() {
			cmd.Dir = dir
		}
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("%w: stdin", errPiProbeStart)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("%w: stdout", errPiProbeStart)
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%w: start", errPiProbeStart)
	}

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()
	defer stopPiProbeProcess(cmd, stdin, waitCh)

	expected := map[string]string{
		"state":   "get_state",
		"entries": "get_entries",
		"tree":    "get_tree",
		"stats":   "get_session_stats",
	}
	encoder := json.NewEncoder(stdin)
	for id, commandType := range expected {
		if err := encoder.Encode(map[string]string{"id": id, "type": commandType}); err != nil {
			return err
		}
	}
	if err := stdin.Close(); err != nil {
		return err
	}

	received := make(map[string]struct{}, len(expected))
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), piProbeMaxFrameBytes)
	for scanner.Scan() {
		line := scanner.Bytes()
		var response piProbeResponse
		if err := json.Unmarshal(line, &response); err != nil {
			return err
		}
		if response.Type != "response" {
			continue
		}
		expectedCommand, ok := expected[response.ID]
		if !ok {
			continue
		}
		if !response.Success || response.Command != expectedCommand {
			return fmt.Errorf("unexpected response for %s", response.ID)
		}
		received[response.ID] = struct{}{}
		if len(received) == len(expected) {
			return nil
		}
	}
	if err := scanner.Err(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return io.ErrUnexpectedEOF
}

func stopPiProbeProcess(cmd *exec.Cmd, stdin io.Closer, waitCh <-chan error) {
	if stdin != nil {
		_ = stdin.Close()
	}
	select {
	case <-waitCh:
		return
	case <-time.After(300 * time.Millisecond):
	}
	killCmdTree(cmd)
	select {
	case <-waitCh:
	case <-time.After(time.Second):
	}
}

func buildPiCommand(ctx context.Context, command string, args ...string) (*exec.Cmd, error) {
	parts := splitCommandParts(command)
	if len(parts) == 0 {
		return nil, errors.New("pi command is empty")
	}
	commandArgs := append(append([]string{}, parts[1:]...), args...)
	executable := parts[0]
	if resolved, err := exec.LookPath(executable); err == nil {
		executable = resolved
	}
	if runtime.GOOS == "windows" {
		ext := strings.ToLower(filepath.Ext(executable))
		if ext == ".cmd" || ext == ".bat" {
			comspec := strings.TrimSpace(os.Getenv("ComSpec"))
			if comspec == "" {
				comspec = "cmd.exe"
			}
			return buildWindowsBatchCommand(ctx, comspec, executable, commandArgs), nil
		}
	}
	return exec.CommandContext(ctx, executable, commandArgs...), nil
}
