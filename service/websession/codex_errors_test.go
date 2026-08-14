package websession

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

type codexDiscardWriteCloser struct {
	io.Writer
}

func (codexDiscardWriteCloser) Close() error {
	return nil
}

func TestCodexAppServerRequestTimeoutIdentifiesMethodAndCleansPending(t *testing.T) {
	client := &codexAppServerClient{
		stdin:   codexDiscardWriteCloser{Writer: io.Discard},
		pending: make(map[string]chan codexAppServerIncoming),
		closed:  make(chan struct{}),
	}
	client.recordMCPStartupStatus(codexAppServerIncoming{
		Method: "mcpServer/startupStatus/updated",
		Params: json.RawMessage(`{"name":"playwright","status":"starting"}`),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := client.request(ctx, "thread/resume", map[string]any{"threadId": "thread_test"})

	var timeoutErr *codexAppServerRequestTimeoutError
	if !errors.As(err, &timeoutErr) {
		t.Fatalf("request error = %v, want codex app-server timeout", err)
	}
	if timeoutErr.Method != "thread/resume" {
		t.Fatalf("timeout method = %q, want thread/resume", timeoutErr.Method)
	}
	if !strings.Contains(timeoutErr.Error(), "MCP status: playwright=starting") {
		t.Fatalf("timeout error = %q, want MCP startup status", timeoutErr.Error())
	}
	client.pendingMu.Lock()
	pendingCount := len(client.pending)
	client.pendingMu.Unlock()
	if pendingCount != 0 {
		t.Fatalf("pending request count = %d, want 0 after timeout", pendingCount)
	}
}

func TestRequestCodexThreadAfterActiveWriter(t *testing.T) {
	t.Run("retries active writer", func(t *testing.T) {
		attempts := 0
		response, err := requestCodexThreadAfterActiveWriter(context.Background(), func() (codexAppServerIncoming, error) {
			attempts++
			if attempts < 3 {
				return codexAppServerIncoming{}, errors.New("thread thread_test already has an active writer")
			}
			return codexAppServerIncoming{Result: json.RawMessage(`{"thread":{"id":"thread_test"}}`)}, nil
		})
		if err != nil {
			t.Fatalf("requestCodexThreadAfterActiveWriter returned error: %v", err)
		}
		if attempts != 3 || parseCodexThreadID(response.Result) != "thread_test" {
			t.Fatalf("unexpected retry result: attempts=%d response=%#v", attempts, response)
		}
	})

	t.Run("does not retry unrelated error", func(t *testing.T) {
		attempts := 0
		wantErr := errors.New("thread resume failed")
		_, err := requestCodexThreadAfterActiveWriter(context.Background(), func() (codexAppServerIncoming, error) {
			attempts++
			return codexAppServerIncoming{}, wantErr
		})
		if !errors.Is(err, wantErr) || attempts != 1 {
			t.Fatalf("unexpected unrelated error result: attempts=%d err=%v", attempts, err)
		}
	})

	t.Run("stops when context is canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		attempts := 0
		_, err := requestCodexThreadAfterActiveWriter(ctx, func() (codexAppServerIncoming, error) {
			attempts++
			cancel()
			return codexAppServerIncoming{}, errors.New("thread thread_test already has an active writer")
		})
		if !errors.Is(err, context.Canceled) || attempts != 1 {
			t.Fatalf("unexpected cancellation result: attempts=%d err=%v", attempts, err)
		}
	})
}

func TestCodexErrorInfoSupportsRolloutAndAppServerShapes(t *testing.T) {
	tests := []struct {
		name   string
		record map[string]any
	}{
		{name: "rollout snake case", record: map[string]any{"codex_error_info": "cyber_policy"}},
		{name: "app server camel case", record: map[string]any{"codexErrorInfo": "cyber_policy"}},
		{
			name: "nested app server error",
			record: map[string]any{
				"error": map[string]any{"codexErrorInfo": "cyberPolicy"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := codexErrorInfo(test.record); got != codexCyberPolicyErrorCode {
				t.Fatalf("codexErrorInfo returned %q", got)
			}
			if !isCodexCyberPolicyError(test.record) {
				t.Fatal("expected cyber policy error")
			}
		})
	}
}

func TestCodexCyberPolicyErrorFallsBackToOfficialMessage(t *testing.T) {
	tests := []struct {
		name   string
		record map[string]any
		want   bool
	}{
		{
			name: "rollout content message",
			record: map[string]any{
				"message": "This content was flagged for possible cybersecurity risk. If this seems wrong, try rephrasing your request.",
			},
			want: true,
		},
		{
			name: "nested app server request message",
			record: map[string]any{
				"error": map[string]any{
					"message": "THIS REQUEST HAS BEEN FLAGGED FOR POSSIBLE CYBERSECURITY RISK.",
				},
			},
			want: true,
		},
		{
			name:   "unrelated security failure",
			record: map[string]any{"message": "request failed during a security review"},
			want:   false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isCodexCyberPolicyError(test.record); got != test.want {
				t.Fatalf("isCodexCyberPolicyError returned %v, want %v", got, test.want)
			}
		})
	}
}

func TestCodexTurnParsersInferCyberPolicyFromOfficialMessage(t *testing.T) {
	message := "This content was flagged for possible cybersecurity risk. If this seems wrong, try rephrasing your request."
	errorMessage, errorCode := parseCodexTurnError(json.RawMessage(`{
		"error": {
			"message": "This content was flagged for possible cybersecurity risk. If this seems wrong, try rephrasing your request."
		}
	}`))
	if errorMessage != message || errorCode != codexCyberPolicyErrorCode {
		t.Fatalf("unexpected error notification classification: message=%q code=%q", errorMessage, errorCode)
	}

	status, completionMessage, completionCode := parseCodexTurnCompletion(json.RawMessage(`{
		"turn": {
			"status": "failed",
			"error": {
				"message": "This content was flagged for possible cybersecurity risk. If this seems wrong, try rephrasing your request."
			}
		}
	}`))
	if status != "failed" || completionMessage != message || completionCode != codexCyberPolicyErrorCode {
		t.Fatalf(
			"unexpected turn completion classification: status=%q message=%q code=%q",
			status,
			completionMessage,
			completionCode,
		)
	}
}

func TestShouldAutoRetryFailureNeverRetriesCyberPolicy(t *testing.T) {
	for _, scope := range []AutoRetryScope{
		AutoRetryScopeNetworkOnly,
		AutoRetryScopeNetworkAndRateLimit,
		AutoRetryScopeAllFailures,
	} {
		if shouldAutoRetryFailure(scope, codexCyberPolicyErrorCode, "temporarily unavailable") {
			t.Fatalf("expected cyber policy failure not to retry for scope %q", scope)
		}
		if shouldAutoRetryFailure(scope, codexRuntimeErrorCode, codexCyberPolicyFallbackText) {
			t.Fatalf("expected message-only cyber policy failure not to retry for scope %q", scope)
		}
	}
}

func TestCodexModelCapacityErrorClassification(t *testing.T) {
	tests := []struct {
		name    string
		code    string
		message string
		want    bool
	}{
		{name: "structured code", code: codexModelCapacityErrorCode, want: true},
		{
			name:    "app server message",
			message: "Selected model is at capacity. Please try a different model.",
			want:    true,
		},
		{
			name:    "case insensitive message",
			message: "SELECTED MODEL IS AT CAPACITY. PLEASE TRY A DIFFERENT MODEL.",
			want:    true,
		},
		{name: "other model error", message: "Selected model is unavailable.", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isCodexModelCapacityError(test.code, test.message); got != test.want {
				t.Fatalf("isCodexModelCapacityError returned %v, want %v", got, test.want)
			}
		})
	}
}

func TestCodexModelCapacityErrorSupportsAppServerFailureShapes(t *testing.T) {
	errorMessage, errorCode := parseCodexTurnError(json.RawMessage(`{
		"error": {
			"message": "Selected model is at capacity. Please try a different model."
		}
	}`))
	if !isCodexModelCapacityError(errorCode, errorMessage) {
		t.Fatalf("expected error notification to classify as model capacity, code=%q message=%q", errorCode, errorMessage)
	}

	status, completionMessage, completionCode := parseCodexTurnCompletion(json.RawMessage(`{
		"turn": {
			"status": "failed",
			"error": {
				"message": "Selected model is at capacity. Please try a different model."
			}
		}
	}`))
	if status != "failed" || !isCodexModelCapacityError(completionCode, completionMessage) {
		t.Fatalf(
			"expected failed turn completion to classify as model capacity, status=%q code=%q message=%q",
			status,
			completionCode,
			completionMessage,
		)
	}
}

func TestShouldAutoRetryFailureRetriesModelCapacityForEveryScope(t *testing.T) {
	for _, scope := range []AutoRetryScope{
		AutoRetryScopeNetworkOnly,
		AutoRetryScopeNetworkAndRateLimit,
		AutoRetryScopeAllFailures,
	} {
		if !shouldAutoRetryFailure(
			scope,
			codexModelCapacityErrorCode,
			"Selected model is at capacity. Please try a different model.",
		) {
			t.Fatalf("expected model capacity failure to retry for scope %q", scope)
		}
	}
}

func TestModelCapacityRetryUsesFixedInitialDelayThenPreset(t *testing.T) {
	message := "Selected model is at capacity. Please try a different model."
	if delay, ok := autoRetryDelayForFailure(AutoRetryPresetAggressiveStop, 1, "", message); !ok || delay != 3*time.Second {
		t.Fatalf("expected initial model capacity delay 3s, got delay=%s ok=%v", delay, ok)
	}
	if delay, ok := autoRetryDelayForFailure(
		AutoRetryPresetAggressiveStop,
		2,
		codexModelCapacityErrorCode,
		message,
	); !ok || delay != 5*time.Second {
		t.Fatalf("expected second aggressive delay 5s, got delay=%s ok=%v", delay, ok)
	}
	if delay, ok := autoRetryDelayForFailure(AutoRetryPresetAggressiveStop, 1, "runtime_error", "other failure"); !ok || delay != 2*time.Second {
		t.Fatalf("expected non-capacity aggressive delay 2s, got delay=%s ok=%v", delay, ok)
	}
}

func TestAutoRetryMaxAttemptsOverridesPresetStop(t *testing.T) {
	if delay, ok := autoRetryDelayForFailureWithMax(
		AutoRetryPresetGentleStop,
		3,
		2,
		"",
		"network timeout",
	); ok || delay != 0 {
		t.Fatalf("expected gentle retry to stop at configured max, got delay=%s ok=%v", delay, ok)
	}

	if delay, ok := autoRetryDelayForFailureWithMax(
		AutoRetryPresetGentleStop,
		5,
		6,
		"",
		"network timeout",
	); !ok || delay != 60*time.Second {
		t.Fatalf("expected custom gentle retry to reuse final delay, got delay=%s ok=%v", delay, ok)
	}

	if delay, ok := autoRetryDelayForFailureWithMax(
		AutoRetryPresetSustain60s,
		3,
		2,
		"",
		"network timeout",
	); ok || delay != 0 {
		t.Fatalf("expected sustain retry to honor configured max, got delay=%s ok=%v", delay, ok)
	}

	if delay, ok := autoRetryDelayForFailureWithMax(
		AutoRetryPresetGentleStop,
		5,
		0,
		"",
		"network timeout",
	); ok || delay != 0 {
		t.Fatalf("expected zero max to preserve gentle preset stop, got delay=%s ok=%v", delay, ok)
	}
}
