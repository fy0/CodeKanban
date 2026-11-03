package websession

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"code-kanban/model/tables"
)

// Opt-in protocol check: no prompts or model inference, only a disposable
// native session used to verify persistence and the installed CLI's schema.
func TestDevinInstalledCLIModelSwitch(t *testing.T) {
	if os.Getenv("CODEKANBAN_TEST_DEVIN_ACP") != "1" {
		t.Skip("set CODEKANBAN_TEST_DEVIN_ACP=1 to check the installed Devin CLI")
	}
	path, err := exec.LookPath("devin")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	manager := &Manager{cfg: Config{DevinPath: path}}
	cwd := t.TempDir()
	var client *devinACPClient
	nativeID := ""
	closeClient := func() {
		_ = client.stdin.Close()
		select {
		case <-client.closed:
		case <-time.After(3 * time.Second):
		}
		client.close()
	}
	defer func() {
		if client != nil {
			if nativeID != "" {
				_, _ = client.request(ctx, "session/delete", map[string]any{"sessionId": nativeID})
			}
			closeClient()
		}
	}()
	start := func(model string) {
		t.Helper()
		client, err = startDevinACP(ctx, path, cwd, model)
		if err != nil {
			t.Fatal(err)
		}
		// Repeated loads and config changes emit session/update notifications.
		// This protocol-only check has no UI projection to consume them.
		go func(c *devinACPClient) {
			for range c.events {
			}
		}(client)
		if _, err := client.request(ctx, "initialize", manager.devinACPInitializeParams()); err != nil {
			t.Fatal(err)
		}
	}
	start("claude-opus-5-5-medium")
	raw, err := client.request(ctx, "session/new", map[string]any{
		"cwd": cwd, "mcpServers": []any{}, "_meta": map[string]any{"cognition.ai/persistOnCreate": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	var created struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(raw, &created); err != nil || created.SessionID == "" {
		t.Fatalf("invalid session/new response: %s", raw)
	}
	nativeID = created.SessionID
	closeClient()
	start("swe-2-high")
	load := func() json.RawMessage {
		t.Helper()
		raw, err := client.request(ctx, "session/load", map[string]any{"sessionId": nativeID, "cwd": cwd, "mcpServers": []any{}})
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	raw = load()
	var before devinACPModelSettings
	if err := json.Unmarshal(raw, &before); err != nil {
		t.Fatal(err)
	}
	if option := before.option("model"); option == nil || option.CurrentValue != "claude-opus-5-5-medium" {
		t.Fatal("expected session/load to restore the saved Opus model despite --model swe-2-high")
	}
	for _, test := range []struct{ model, value, effort, speed string }{
		{"swe-2-max", "swe-2-high", "max", "standard"},
		{"claude-opus-5-5-high-fast", "claude-opus-5-5-medium", "high", "fast"},
		{"gpt-6-sol-high-priority", "gpt-6-sol-medium", "high", "fast"},
		{"fusion-claude-opus-5-5-high-fast-sidekick-swe-2-medium", "fusion-claude-opus-5-5-medium-sidekick-swe-2-medium", "high", "fast"},
		{"swe-2-max-lightning", "swe-2-high", "max", "fast"},
		{"swe-2-max", "swe-2-high", "max", "standard"},
	} {
		session := tables.WebSessionTable{Model: test.model, ReasoningEffort: test.effort}
		if err := applyDevinSessionModel(ctx, client, session, nativeID, load()); err != nil {
			t.Fatalf("switch to %s: %v", test.model, err)
		}
		var after devinACPModelSettings
		if err := json.Unmarshal(load(), &after); err != nil {
			t.Fatal(err)
		}
		for category, want := range map[string]string{"model": test.value, "thought_level": test.effort, "speed": test.speed} {
			if option := after.option(category); option == nil || option.CurrentValue != want {
				t.Fatalf("%s: %s was not applied: want %q", test.model, category, want)
			}
		}
		t.Logf("Applied %s: model=%s, thinking=%s, speed=%s", test.model, test.value, test.effort, test.speed)
	}
}

func TestDevinACPModelValuePreservesVariants(t *testing.T) {
	var settings devinACPModelSettings
	if err := json.Unmarshal([]byte(`{"configOptions":[{"id":"model","options":[{"value":"swe-2-high"},{"value":"gpt-6-sol-medium"},{"value":"gpt-6-sol-medium-fast"},{"value":"fusion-claude-opus-5-5-medium-sidekick-swe-2-high"}]}]}`), &settings); err != nil {
		t.Fatal(err)
	}
	for model, want := range map[string]string{
		"swe-2-max":               "swe-2-high",
		"gpt-6-sol-high":          "gpt-6-sol-medium",
		"gpt-6-sol-high-fast":     "gpt-6-sol-medium-fast",
		"gpt-6-sol-high-priority": "gpt-6-sol-medium-fast",
		"gpt-6-sol-high-1m":       "gpt-6-sol-high-1m",
		"swe-2-max-lightning":     "swe-2-high",
		"fusion-claude-opus-5-5-high-fast-sidekick-swe-2-high": "fusion-claude-opus-5-5-medium-sidekick-swe-2-high",
		"fusion-claude-opus-5-5-medium-sidekick-swe-2-high":    "fusion-claude-opus-5-5-medium-sidekick-swe-2-high",
		"fusion-claude-opus-5-5-medium-sidekick-swe-2-medium":  "fusion-claude-opus-5-5-medium-sidekick-swe-2-medium",
	} {
		if got := devinACPModelValue(settings.option("model"), model); got != want {
			t.Errorf("model %q resolved to %q, want %q", model, got, want)
		}
	}
}

// Run the full ACP transport against a subprocess so these tests cover the
// ordering of attach, model/effort changes, and prompt (including failures).
func TestDevinRunAppliesModelBeforePrompt(t *testing.T) {
	for _, test := range []struct {
		mode      string
		attach    []string
		changes   []string
		wantError string
		fresh     bool
		model     string
	}{
		{mode: "new", fresh: true, attach: []string{"session/new"}, changes: []string{"model=swe-2-high", "thought_level=max"}},
		{mode: "load", attach: []string{"session/load"}, changes: []string{"model=swe-2-high", "thought_level=max"}},
		{mode: "resume", attach: []string{"session/resume"}, changes: []string{"model=swe-2-high", "thought_level=max"}},
		{mode: "resume-fallback", attach: []string{"session/resume", "session/load"}, changes: []string{"model=swe-2-high", "thought_level=max"}},
		{mode: "already-applied", attach: []string{"session/load"}},
		{mode: "inherited-speed", attach: []string{"session/load"}, changes: []string{"speed=standard"}},
		{mode: "lightning", model: "swe-2-max-lightning", attach: []string{"session/load"}, changes: []string{"model=swe-2-high", "thought_level=max", "speed=fast"}},
		{mode: "legacy", attach: []string{"session/load"}, changes: []string{"modelId=swe-2-max"}},
		{mode: "reject-model", attach: []string{"session/load"}, changes: []string{"model=swe-2-high"}, wantError: "switch rejected"},
		{mode: "ignore-model", attach: []string{"session/load"}, changes: []string{"model=swe-2-high"}, wantError: "did not apply model"},
		{mode: "reject-thinking", attach: []string{"session/load"}, changes: []string{"model=swe-2-high", "thought_level=max"}, wantError: "switch rejected"},
		{mode: "ignore-thinking", attach: []string{"session/load"}, changes: []string{"model=swe-2-high", "thought_level=max"}, wantError: "did not apply thought_level"},
	} {
		t.Run(test.mode, func(t *testing.T) {
			manager, session := newDevinSubAgentTestManager(t)
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			logPath := filepath.Join(t.TempDir(), "requests.jsonl")
			t.Setenv("CODEKANBAN_DEVIN_MODEL_TEST_MODE", test.mode)
			t.Setenv("CODEKANBAN_DEVIN_MODEL_TEST_LOG", logPath)
			manager.cfg.DevinPath = fmt.Sprintf(`"%s" -test.run=^TestDevinModelACPProcess$ -- devin-model-helper`, executable)
			session.Model = "swe-2-max"
			if test.model != "" {
				session.Model = test.model
			}
			session.ReasoningEffort = "medium" // The previous Opus setting must not win.
			if !test.fresh {
				nativeID := "native-old-opus"
				session.NativeSessionID = &nativeID
			}
			run := &activeRun{runID: "model-switch", sessionID: session.ID, agent: AgentDevin, backend: SessionBackendDevinACP}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			manager.runDevinACPSession(ctx, run, *session, "continue", nil)
			if test.wantError == "" && run.lastError != "" {
				t.Fatalf("run failed: %s", run.lastError)
			}
			if test.wantError != "" && (!strings.Contains(run.lastError, "model switch failed") || !strings.Contains(run.lastError, test.wantError)) {
				t.Fatalf("run error = %q, want model switch failure containing %q", run.lastError, test.wantError)
			}
			data, err := os.ReadFile(logPath)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
				var request devinACPMessage
				if err := json.Unmarshal([]byte(line), &request); err != nil {
					t.Fatal(err)
				}
				var params map[string]string
				if request.Method == "session/set_config_option" || request.Method == "session/set_model" {
					if err := json.Unmarshal(request.Params, &params); err != nil {
						t.Fatal(err)
					}
					if request.Method == "session/set_model" {
						got = append(got, "modelId="+params["modelId"])
					} else {
						got = append(got, params["configId"]+"="+params["value"])
					}
				} else {
					got = append(got, request.Method)
				}
			}
			want := append([]string{"initialize"}, test.attach...)
			want = append(want, test.changes...)
			if test.wantError == "" {
				want = append(want, "session/prompt")
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("ACP requests = %v, want %v", got, want)
			}
		})
	}
}

func TestDevinModelACPProcess(t *testing.T) {
	if len(os.Args) < 3 || !strings.Contains(strings.Join(os.Args, " "), "-- devin-model-helper") {
		return
	}
	for _, arg := range os.Args {
		if arg == "--version" {
			fmt.Println("devin 3000.11.234")
			os.Exit(0)
		}
	}
	mode := os.Getenv("CODEKANBAN_DEVIN_MODEL_TEST_MODE")
	logFile, err := os.Create(os.Getenv("CODEKANBAN_DEVIN_MODEL_TEST_LOG"))
	if err != nil {
		os.Exit(1)
	}
	defer logFile.Close()
	model, thinking, speed := "claude-opus-5-5-medium", "medium", "standard"
	if mode == "already-applied" || mode == "inherited-speed" {
		model, thinking = "swe-2-high", "max"
	}
	if mode == "inherited-speed" {
		speed = "fast"
	}
	settings := func() map[string]any {
		if mode == "legacy" {
			return map[string]any{"models": map[string]any{"currentModelId": model}}
		}
		return map[string]any{"configOptions": []any{
			map[string]any{"id": "model", "category": "model", "currentValue": model, "options": []any{
				map[string]any{"value": "claude-opus-5-5-medium"}, map[string]any{"value": "swe-2-high"},
			}},
			map[string]any{"id": "thought_level", "category": "thought_level", "currentValue": thinking},
			map[string]any{"id": "speed", "category": "model_config", "currentValue": speed},
		}}
	}
	decoder, encoder := json.NewDecoder(os.Stdin), json.NewEncoder(os.Stdout)
	for {
		var request devinACPMessage
		if err := decoder.Decode(&request); err != nil {
			os.Exit(0)
		}
		if err := json.NewEncoder(logFile).Encode(request); err != nil {
			os.Exit(1)
		}
		result := map[string]any{}
		var rpcError *devinACPError
		switch request.Method {
		case "initialize":
			capabilities := map[string]any{"loadSession": true}
			if strings.HasPrefix(mode, "resume") {
				capabilities["sessionCapabilities"] = map[string]any{"resume": map[string]any{}}
			}
			result["agentCapabilities"] = capabilities
		case "session/new", "session/load", "session/resume":
			result = settings()
			result["sessionId"] = "native-old-opus"
			if mode == "resume-fallback" && request.Method == "session/resume" {
				rpcError = &devinACPError{Code: -32601, Message: "Method not found"}
			}
		case "session/set_config_option":
			var params map[string]string
			_ = json.Unmarshal(request.Params, &params)
			isModel := params["configId"] == "model"
			isSpeed := params["configId"] == "speed"
			if (mode == "reject-model" && isModel) || (mode == "reject-thinking" && !isModel) {
				rpcError = &devinACPError{Code: -32602, Message: "switch rejected"}
			} else if isModel && mode != "ignore-model" {
				if params["value"] != "swe-2-high" {
					rpcError = &devinACPError{Code: -32602, Message: "unsupported model variant"}
				}
				model, thinking = params["value"], "high"
			} else if isSpeed {
				speed = params["value"]
			} else if !isModel && mode != "ignore-thinking" {
				thinking = params["value"]
			}
			result = settings()
		case "session/set_model":
			var params map[string]string
			_ = json.Unmarshal(request.Params, &params)
			model = params["modelId"]
		case "session/prompt":
			wantSpeed := "standard"
			if mode == "lightning" {
				wantSpeed = "fast"
			}
			if (mode == "legacy" && model != "swe-2-max") || (mode != "legacy" && (model != "swe-2-high" || thinking != "max")) {
				rpcError = &devinACPError{Code: -32603, Message: "prompt used the previous model or thinking level"}
			}
			if speed != wantSpeed {
				rpcError = &devinACPError{Code: -32603, Message: "prompt used the previous speed"}
			}
			result["stopReason"] = "end_turn"
		}
		response := map[string]any{"jsonrpc": "2.0", "id": request.ID}
		if rpcError != nil {
			response["error"] = rpcError
		} else {
			response["result"] = result
		}
		if err := encoder.Encode(response); err != nil {
			os.Exit(1)
		}
	}
}
