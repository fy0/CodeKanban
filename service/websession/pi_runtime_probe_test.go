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
			fmt.Fprintln(os.Stdout, os.Getenv("CODEKANBAN_PI_PROBE_VERSION"))
			os.Exit(0)
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
