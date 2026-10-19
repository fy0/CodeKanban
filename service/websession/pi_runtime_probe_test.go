package websession

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPiProbeHelperProcess(t *testing.T) {
	if os.Getenv("CODEKANBAN_PI_PROBE_HELPER") != "1" {
		return
	}
	if marker := os.Getenv("CODEKANBAN_PI_PROBE_MARKER"); marker != "" {
		file, _ := os.OpenFile(marker, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if file != nil {
			_, _ = file.WriteString("start\n")
			_ = file.Close()
		}
	}
	for _, arg := range os.Args {
		if arg == "--version" {
			if delay, err := time.ParseDuration(os.Getenv("CODEKANBAN_PI_PROBE_VERSION_DELAY")); err == nil {
				time.Sleep(delay)
			}
			fmt.Fprintln(os.Stdout, os.Getenv("CODEKANBAN_PI_PROBE_VERSION"))
			os.Exit(0)
		}
	}
	if argsMarker := os.Getenv("CODEKANBAN_PI_PROBE_ARGS_MARKER"); argsMarker != "" {
		file, _ := os.OpenFile(argsMarker, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if file != nil {
			_, _ = file.WriteString(strings.Join(os.Args, "\n") + "\n---\n")
			_ = file.Close()
		}
	}

	skip := os.Getenv("CODEKANBAN_PI_PROBE_SKIP")
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var request struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		}
		if json.Unmarshal(scanner.Bytes(), &request) != nil || request.ID == skip {
			continue
		}
		data := any(map[string]any{})
		if request.Type == "get_available_models" {
			data = map[string]any{"models": []map[string]any{{
				"provider": "anthropic", "id": "claude-sonnet-4", "name": "Claude Sonnet 4",
				"reasoning": true, "input": []string{"text", "image"}, "contextWindow": 200000,
				"thinking": map[string]any{"efforts": []string{"off", "low", "high"}},
			}}}
		}
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
			"id":      request.ID,
			"type":    "response",
			"command": request.Type,
			"success": true,
			"data":    data,
		})
	}
	os.Exit(0)
}

func piProbeTestCommand() string {
	return `"` + os.Args[0] + `" -test.run=TestPiProbeHelperProcess --`
}

func TestDetectPiVersionAllowsPortableLauncherStartup(t *testing.T) {
	if piVersionProbeTimeout != 5*time.Second {
		t.Fatalf("Pi version probe timeout = %v, want 5s", piVersionProbeTimeout)
	}
	t.Setenv("CODEKANBAN_PI_PROBE_HELPER", "1")
	t.Setenv("CODEKANBAN_PI_PROBE_VERSION", "0.84.1")
	t.Setenv("CODEKANBAN_PI_PROBE_VERSION_DELAY", "100ms")

	version := detectPiVersion(piProbeTestCommand())
	if version == nil || *version != "0.84.1" {
		t.Fatalf("Pi version = %#v, want 0.84.1", version)
	}
}

func TestGetWebSessionRuntimeConfigProbesPiRPCAndCachesSuccess(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "probe-starts.txt")
	t.Setenv("CODEKANBAN_PI_PROBE_HELPER", "1")
	t.Setenv("CODEKANBAN_PI_PROBE_VERSION", "0.84.1")
	t.Setenv("CODEKANBAN_PI_PROBE_MARKER", marker)

	manager := &Manager{cfg: Config{
		PiPath:  piProbeTestCommand(),
		DataDir: t.TempDir(),
	}}
	config := manager.GetWebSessionRuntimeConfig()
	if !config.HasPi || !config.PiRPCCompatible {
		t.Fatalf("unexpected Pi probe result: %#v", config)
	}
	if config.PiVersion == nil || *config.PiVersion != "0.84.1" {
		t.Fatalf("Pi version = %#v, want 0.84.1", config.PiVersion)
	}
	if config.PiDiagnostics != "" || config.PiMinVersion != piMinVersion {
		t.Fatalf("unexpected Pi diagnostics: code=%q minimum=%q", config.PiDiagnostics, config.PiMinVersion)
	}
	if !config.SupportsPiWebSession || !config.Agents[AgentPi].SupportsWebSession {
		t.Fatal("compatible Pi RPC should enable Pi Web Sessions")
	}
	piCapability := config.Agents[AgentPi]
	if !piCapability.Installed || !piCapability.SupportsImages || !piCapability.SupportsCompaction ||
		!piCapability.SupportsSteer || !piCapability.SupportsFollowUp || !piCapability.SupportsTree {
		t.Fatal("compatible Pi RPC should expose messaging and native tree controls")
	}
	if len(config.PiModels) != 1 || config.PiModels[0].Provider != "anthropic" || !config.PiModels[0].Reasoning {
		t.Fatalf("Pi model catalog = %#v", config.PiModels)
	}
	if got := strings.Join(config.PiModels[0].ReasoningEfforts, ","); got != "none,low,high" {
		t.Fatalf("Pi model reasoning efforts = %q", got)
	}

	_ = manager.GetWebSessionRuntimeConfig()
	raw, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("read probe marker: %v", err)
	}
	if starts := strings.Count(string(raw), "start\n"); starts != 3 {
		t.Fatalf("helper starts = %d, want 3 (version + RPC + models) after cached second read", starts)
	}
}

func TestGetWebSessionRuntimeConfigReportsSafePiDiagnostics(t *testing.T) {
	tests := []struct {
		name       string
		version    string
		skip       string
		path       string
		diagnostic string
		installed  bool
	}{
		{name: "not installed", path: filepath.Join(t.TempDir(), "missing-pi"), diagnostic: piDiagnosticNotInstalled},
		{name: "unknown version", version: "development", diagnostic: piDiagnosticVersion, installed: true},
		{name: "old version", version: "0.84.0", diagnostic: piDiagnosticTooOld, installed: true},
		{name: "missing RPC command", version: "0.84.1", skip: "tree", diagnostic: piDiagnosticProtocol, installed: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CODEKANBAN_PI_PROBE_HELPER", "1")
			t.Setenv("CODEKANBAN_PI_PROBE_VERSION", tt.version)
			t.Setenv("CODEKANBAN_PI_PROBE_SKIP", tt.skip)
			command := tt.path
			if command == "" {
				command = piProbeTestCommand()
			}
			manager := &Manager{cfg: Config{PiPath: command, DataDir: t.TempDir()}}
			config := manager.GetWebSessionRuntimeConfig()
			if config.HasPi != tt.installed || config.PiRPCCompatible {
				t.Fatalf("unexpected Pi state: hasPi=%v compatible=%v", config.HasPi, config.PiRPCCompatible)
			}
			if config.PiDiagnostics != tt.diagnostic {
				t.Fatalf("Pi diagnostics = %q, want %q", config.PiDiagnostics, tt.diagnostic)
			}
		})
	}
}

func TestBuildPiCommandSupportsConfiguredArguments(t *testing.T) {
	cmd, err := buildPiCommand(context.Background(), `"C:\Program Files\node.exe" "C:\pi\dist\cli.js"`, "--version")
	if err != nil {
		t.Fatalf("buildPiCommand returned error: %v", err)
	}
	if cmd.Path == "" || len(cmd.Args) != 3 || cmd.Args[1] != `C:\pi\dist\cli.js` || cmd.Args[2] != "--version" {
		t.Fatalf("unexpected command: path=%q args=%#v", cmd.Path, cmd.Args)
	}
}

func TestLoadPiModelCatalogKeepsExtensionsEnabled(t *testing.T) {
	argsMarker := filepath.Join(t.TempDir(), "probe-args.txt")
	t.Setenv("CODEKANBAN_PI_PROBE_HELPER", "1")
	t.Setenv("CODEKANBAN_PI_PROBE_ARGS_MARKER", argsMarker)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	models, err := loadPiModelCatalog(ctx, piProbeTestCommand(), "")
	if err != nil {
		t.Fatalf("loadPiModelCatalog returned error: %v", err)
	}
	if len(models) != 1 || models[0].Provider != "anthropic" {
		t.Fatalf("Pi model catalog = %#v", models)
	}
	raw, err := os.ReadFile(argsMarker)
	if err != nil {
		t.Fatalf("read probe args marker: %v", err)
	}
	if strings.Contains(string(raw), "--no-extensions") {
		t.Fatal("model catalog probe must keep extensions enabled so plugin providers are listed")
	}
}

func TestNormalizePiModelReasoningEfforts(t *testing.T) {
	max := "max"
	high := "high"
	if got := normalizePiModelReasoningEfforts(true, []string{"off", "low", "high", "max"}, nil); strings.Join(got, ",") != "none,low,high,max" {
		t.Fatalf("explicit Pi reasoning efforts = %#v", got)
	}
	if got := normalizePiModelReasoningEfforts(true, nil, map[string]*string{
		"minimal": nil,
		"low":     &high,
		"medium":  nil,
		"high":    &high,
		"xhigh":   nil,
		"max":     &max,
	}); strings.Join(got, ",") != "low,high,max" {
		t.Fatalf("mapped Pi reasoning efforts = %#v", got)
	}
	if got := normalizePiModelReasoningEfforts(false, []string{"low"}, nil); got != nil {
		t.Fatalf("non-reasoning Pi model efforts = %#v", got)
	}
}
