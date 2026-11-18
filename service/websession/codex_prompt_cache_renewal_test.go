package websession

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"code-kanban/model"
	"code-kanban/model/tables"
	"code-kanban/utils"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestPromptCacheRootAndProtocolErrors(t *testing.T) {
	for _, test := range []struct {
		thread string
		root   bool
	}{
		{`{"ephemeral":false,"source":"vscode","threadSource":"user"}`, true},
		{`{"ephemeral":true}`, false},
		{`{"ephemeral":false,"forkedFromId":"source"}`, false},
		{`{"ephemeral":false,"parentThreadId":"parent"}`, false},
		{`{"ephemeral":false,"threadSource":"side_conversation"}`, false},
		{`{"ephemeral":false,"source":{"subagent":{}}}`, false},
		{`{}`, false},
	} {
		if got := isCodexSampleRoot(json.RawMessage(test.thread)); got != test.root {
			t.Errorf("root(%s) = %t, want %t", test.thread, got, test.root)
		}
	}
	if !codexExperimentalAPIDisabled(json.RawMessage(`{"capabilities":{"experimentalApi":false}}`)) ||
		codexExperimentalAPIDisabled(json.RawMessage(`{"userAgent":"codex"}`)) {
		t.Fatal("initialize response confused missing capability metadata with an explicit rejection")
	}
	client := &codexAppServerClient{closed: make(chan struct{})}
	for _, test := range []struct {
		err  error
		want string
	}{
		{&codexAppServerErr{Code: -32601}, "method_not_found"},
		{&codexAppServerErr{Code: -32600, Message: "unknown variant `thread/sample`"}, "method_not_found"},
		{&codexAppServerErr{Code: -32600, Message: "experimentalApi not enabled"}, "experimental_api_disabled"},
		{&codexAppServerErr{Code: -32602}, "protocol_incompatible"},
		{&codexAppServerErr{Code: -32600, Message: "sample source must be idle"}, "parent_busy"},
		{&codexAppServerErr{Code: -32600, Message: "sample source must be a persistent root thread"}, "parent_not_root"},
		{&codexAppServerErr{Code: -32600, Message: "sample source turn must be the latest finished turn"}, "expected_turn_changed"},
		{&codexAppServerErr{Code: -32600, Message: "thread/sample requires a loaded source"}, "parent_missing"},
		{context.DeadlineExceeded, "sample_timeout"},
		{context.Canceled, "sample_cancelled"},
		{errors.New("other error"), "rpc_error"},
	} {
		if got := codexSampleError(client, test.err); got != test.want {
			t.Errorf("classifying %v = %q, want %q", test.err, got, test.want)
		}
	}
}

func TestPromptCacheModeCycleAndFailureBackoff(t *testing.T) {
	renewal := &promptCacheRenewal{
		wake:   make(chan struct{}, 1),
		config: utils.WebSessionPromptCacheRenewalConfig{IntervalSeconds: 60, FailureBackoffSeconds: 90},
	}
	for count := 1; count <= 3; count++ {
		renewal.updateMode("cycle")
		if renewal.remaining != count || renewal.infinite {
			t.Fatalf("click %d = %d, infinite %t", count, renewal.remaining, renewal.infinite)
		}
	}
	renewal.updateMode("cycle")
	if !renewal.infinite || renewal.remaining != 0 {
		t.Fatal("fourth click must start infinite mode")
	}
	renewal.updateMode("cycle")
	if renewal.infinite || renewal.remaining != 0 {
		t.Fatal("clicking infinite mode must disable it")
	}
	now := time.Now()
	for count := 1; count <= 3; count++ {
		renewal.updateMode("cycle")
	}
	for count := 2; count >= 0; count-- {
		renewal.finishAttempt(renewal.generation, "", now)
		if renewal.remaining != count || renewal.nextAt.Sub(now) != time.Minute {
			t.Fatalf("unexpected finite completion state: remaining %d", renewal.remaining)
		}
	}
	renewal.infinite = true
	for range promptCacheFailureLimit {
		renewal.finishAttempt(renewal.generation, "rpc_error", now)
	}
	if renewal.infinite || renewal.nextAt.Sub(now) != 90*time.Second {
		t.Fatal("infinite failures must back off and stop at the failure limit")
	}
}

func TestPromptCacheRuntimeRenewal(t *testing.T) {
	for _, mode := range []string{"success", "unsupported", "stale", "hold"} {
		t.Run(mode, func(t *testing.T) {
			defer initTestDB(t)()
			project := seedProject(t)
			core, logs := observer.New(zap.InfoLevel)
			codexPath := writeFakeCodexAppServerCLI(t, "prompt_cache_"+mode)
			manager, err := NewManager(Config{
				DataDir: t.TempDir(), CodexPath: codexPath,
				PromptCacheRenewal: utils.WebSessionPromptCacheRenewalConfig{Enabled: true, ShowUnsupported: true},
			}, zap.New(core))
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			session, err := manager.CreateSession(ctx, CreateParams{ProjectID: project.ID, Agent: AgentCodex})
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				manager.StopAllPromptCacheRenewals()
				waitForSessionToSettle(t, manager, session.ID)
				if err := manager.waitForCodexRunDrain(ctx, session.ID); err != nil {
					t.Error(err)
				}
			}()
			if err := manager.SendMessage(ctx, session.ID, "user message", nil); err != nil {
				t.Fatal(err)
			}
			waitForSessionToSettle(t, manager, session.ID)
			waitForSessionStatus(t, manager, session.ID, StatusDone)
			parent, err := manager.GetSession(ctx, session.ID)
			if err != nil {
				t.Fatal(err)
			}
			samplePath := codexPath + ".state.samples"
			wantAvailability := "ready"
			if mode == "unsupported" {
				wantAvailability = "unsupported"
			}
			awaitPromptCacheCondition(t, func() bool {
				return manager.PromptCacheRenewal(ctx, parent, "").Availability == wantAvailability
			})
			if mode != "unsupported" {
				manager.mu.RLock()
				renewal := manager.promptCacheRenewals[session.ID]
				manager.mu.RUnlock()
				manager.mu.Lock()
				manager.runs[session.ID] = &activeRun{}
				manager.mu.Unlock()
				if got := manager.promptCacheParentError(ctx, renewal); got != "parent_busy" {
					t.Errorf("active parent classification = %q", got)
				}
				manager.mu.Lock()
				delete(manager.runs, session.ID)
				manager.mu.Unlock()
				if state := manager.PromptCacheRenewal(ctx, parent, "cycle"); state.Remaining != 1 {
					t.Fatalf("enqueue state = %#v", state)
				}
				renewal.mu.Lock()
				renewal.nextAt = time.Now()
				renewal.mu.Unlock()
				select {
				case renewal.wake <- struct{}{}:
				default:
				}
				awaitPromptCacheCondition(t, func() bool { return len(promptCacheSampleFrames(t, samplePath)) == 2 })
				frames := promptCacheSampleFrames(t, samplePath)
				params := decodeRawObject(frames[1].Params)
				if len(params) != 3 || params["threadId"] != "thread_test" || params["expectedTurnId"] != "turn_test" || params["prompt"] != codexSamplePrompt {
					t.Fatalf("wrong sample params: %s", frames[1].Params)
				}
				if mode == "hold" {
					manager.PromptCacheRenewal(ctx, parent, "cycle")
					if len(promptCacheSampleFrames(t, samplePath)) != 2 {
						t.Fatal("started a concurrent sample")
					}
					if state := manager.PromptCacheRenewal(ctx, parent, "off"); state.Infinite || state.Remaining != 0 {
						t.Fatalf("cancelling did not clear queue: %#v", state)
					}
					waitForFile(t, codexPath+".state.sample-cancels")
					cancelParams, err := os.ReadFile(codexPath + ".state.sample-cancels")
					if err != nil || decodeRawObject(json.RawMessage(bytes.TrimSpace(cancelParams)))["requestId"] != appServerIDKey(frames[1].ID) {
						t.Fatalf("sample cancellation didn't target the in-flight RPC: %v", err)
					}
				}
				awaitPromptCacheCondition(t, func() bool { return logs.FilterMessage("Codex prompt cache sample").Len() == 1 })
				fields := logs.FilterMessage("Codex prompt cache sample").All()[0].ContextMap()
				wantError := map[string]string{"success": "", "stale": "expected_turn_changed", "hold": "sample_cancelled"}[mode]
				if fields["error_category"] != wantError || fields["success"] != (mode == "success") || fields["parent_thread_id"] != "thread_test" || fields["expected_turn_id"] != "turn_test" {
					t.Fatalf("incorrect sample log context: %#v", fields)
				}
				if duration, ok := fields["rpc_duration"].(time.Duration); !ok || duration <= 0 {
					t.Fatalf("missing sample RPC duration: %#v", fields)
				}
				if mode == "success" && (fields["cached_input_tokens"] != int64(3072) || fields["input_tokens"] != int64(4096) || fields["output_tokens"] != int64(1)) {
					t.Fatalf("incorrect sample usage: %#v", fields)
				}
				var items []tables.WebSessionItemTable
				if err := model.GetDB().Where("web_session_id = ?", session.ID).Find(&items).Error; err != nil {
					t.Fatal(err)
				}
				itemsJSON, _ := json.Marshal(items)
				if bytes.Contains(itemsJSON, []byte("private sample output")) || bytes.Contains(itemsJSON, []byte(codexSamplePrompt)) {
					t.Fatal("sample leaked into the transcript")
				}
			}
			if err := manager.SendMessage(ctx, session.ID, "next user message", nil); err != nil {
				t.Fatalf("sample affected the next send: %v", err)
			}
			waitForSessionToSettle(t, manager, session.ID)
			waitForSessionStatus(t, manager, session.ID, StatusDone)
			if mode == "unsupported" && len(promptCacheSampleFrames(t, samplePath)) != 1 {
				t.Fatal("unsupported server was probed again after another real turn")
			}
			if _, err := os.Stat(codexPath + ".state.unexpected-fallback"); !os.IsNotExist(err) {
				t.Fatalf("unexpected fallback: %v", err)
			}
		})
	}
}

func awaitPromptCacheCondition(t *testing.T, condition func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for prompt cache renewal")
}

func promptCacheSampleFrames(t *testing.T, path string) []codexAppServerIncoming {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var frames []codexAppServerIncoming
	for _, line := range strings.Split(strings.TrimSpace(string(content)), "\n") {
		var frame codexAppServerIncoming
		if json.Unmarshal([]byte(line), &frame) == nil {
			frames = append(frames, frame)
		}
	}
	return frames
}
