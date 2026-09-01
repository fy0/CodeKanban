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
	"go.uber.org/zap"
)

const (
	piMinVersion             = "0.84.1"
	piProbeSuccessCacheTTL   = 5 * time.Minute
	piProbeFailureCacheTTL   = time.Minute
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

type piRuntimeProbeTimings struct {
	version time.Duration
	rpc     time.Duration
	models  time.Duration
}

type piRuntimeProbeCache = runtimeCapabilityCache[piRuntimeProbeResult]

type piProbeResponse struct {
	Type    string `json:"type"`
	ID      string `json:"id"`
	Command string `json:"command"`
	Success bool   `json:"success"`
}

func (m *Manager) applyPiRuntimeCapabilitiesWithRefresh(
	config WebSessionRuntimeConfig,
	force bool,
) WebSessionRuntimeConfig {
	config.PiMinVersion = piMinVersion
	if m == nil {
		config.Agents = runtimeAgentCapabilities(config)
		return config
	}

	probe := m.getPiRuntimeProbeWithRefresh(force)
	return mergePiRuntimeCapabilities(config, probe)
}

func (m *Manager) applyPiRuntimeCapabilitiesBackground(
	config WebSessionRuntimeConfig,
) (WebSessionRuntimeConfig, bool) {
	config.PiMinVersion = piMinVersion
	if m == nil {
		config.Agents = runtimeAgentCapabilities(config)
		return config, false
	}
	probe, refreshing := m.piProbe.getBackground(
		runtimeCapabilityCachePolicy{
			successTTL:     piProbeSuccessCacheTTL,
			failureBackoff: piProbeFailureCacheTTL,
		},
		clonePiRuntimeProbeResult,
		m.probePiRuntimeCapabilities,
	)
	return mergePiRuntimeCapabilities(config, probe), refreshing
}

func mergePiRuntimeCapabilities(
	config WebSessionRuntimeConfig,
	probe piRuntimeProbeResult,
) WebSessionRuntimeConfig {
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
	return m.getPiRuntimeProbeWithRefresh(false)
}

func (m *Manager) getPiRuntimeProbeWithRefresh(force bool) piRuntimeProbeResult {
	return m.piProbe.get(
		force,
		runtimeCapabilityCachePolicy{
			successTTL:     piProbeSuccessCacheTTL,
			failureBackoff: piProbeFailureCacheTTL,
		},
		clonePiRuntimeProbeResult,
		m.probePiRuntimeCapabilities,
	)
}

func (m *Manager) probePiRuntimeCapabilities() (result piRuntimeProbeResult, probeErr error) {
	startedAt := time.Now()
	timings := piRuntimeProbeTimings{}
	defer func() {
		m.logRuntimeCapabilityProbe(
			"pi_runtime",
			startedAt,
			probeErr,
			zap.Bool("installed", result.installed),
			zap.Bool("compatible", result.compatible),
			zap.String("diagnostic", result.diagnostic),
			zap.Int("modelCount", len(result.models)),
			zap.Duration("versionDuration", timings.version),
			zap.Duration("rpcDuration", timings.rpc),
			zap.Duration("modelDuration", timings.models),
		)
	}()
	if m.runtimeCapabilityProbes.pi != nil {
		return m.runtimeCapabilityProbes.pi()
	}
	result, timings = probePiRuntimeWithTimings(m.cfg.PiPath, m.cfg.DataDir)
	switch result.diagnostic {
	case "", piDiagnosticNotInstalled, piDiagnosticTooOld:
		return result, nil
	default:
		return result, errors.New("Pi runtime capability probe failed")
	}
}

func clonePiRuntimeProbeResult(result piRuntimeProbeResult) piRuntimeProbeResult {
	cloned := result
	if result.version != nil {
		version := *result.version
		cloned.version = &version
	}
	cloned.models = make([]PiModelInfo, len(result.models))
	for index, model := range result.models {
		cloned.models[index] = model
		cloned.models[index].Input = append([]string(nil), model.Input...)
	}
	return cloned
}

func probePiRuntime(command, workingDir string) piRuntimeProbeResult {
	result, _ := probePiRuntimeWithTimings(command, workingDir)
	return result
}

func probePiRuntimeWithTimings(command, workingDir string) (piRuntimeProbeResult, piRuntimeProbeTimings) {
	result := piRuntimeProbeResult{}
	timings := piRuntimeProbeTimings{}
	if !hasExecutable(command) {
		result.diagnostic = piDiagnosticNotInstalled
		return result, timings
	}
	result.installed = true

	versionStartedAt := time.Now()
	version := detectPiVersion(command)
	timings.version = time.Since(versionStartedAt)
	if version == nil {
		result.diagnostic = piDiagnosticVersion
		return result, timings
	}
	result.version = version
	parsedVersion, err := semver.NewVersion(*version)
	if err != nil {
		result.diagnostic = piDiagnosticVersion
		return result, timings
	}
	if parsedVersion.LessThan(piMinimumVersion) {
		result.diagnostic = piDiagnosticTooOld
		return result, timings
	}

	ctx, cancel := context.WithTimeout(context.Background(), piProbeTimeout)
	defer cancel()
	rpcStartedAt := time.Now()
	if err := runPiRPCProbe(ctx, command, workingDir); err != nil {
		timings.rpc = time.Since(rpcStartedAt)
		switch {
		case errors.Is(err, context.DeadlineExceeded), errors.Is(ctx.Err(), context.DeadlineExceeded):
			result.diagnostic = piDiagnosticTimeout
		case errors.Is(err, errPiProbeStart):
			result.diagnostic = piDiagnosticStart
		default:
			result.diagnostic = piDiagnosticProtocol
		}
		return result, timings
	}
	timings.rpc = time.Since(rpcStartedAt)

	result.compatible = true
	modelCtx, modelCancel := context.WithTimeout(context.Background(), piProbeTimeout)
	modelStartedAt := time.Now()
	result.models, _ = loadPiModelCatalog(modelCtx, command, workingDir)
	timings.models = time.Since(modelStartedAt)
	modelCancel()
	return result, timings
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
