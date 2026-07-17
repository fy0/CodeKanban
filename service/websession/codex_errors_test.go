package websession

import "testing"

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

func TestShouldAutoRetryFailureNeverRetriesCyberPolicy(t *testing.T) {
	for _, scope := range []AutoRetryScope{
		AutoRetryScopeNetworkOnly,
		AutoRetryScopeNetworkAndRateLimit,
		AutoRetryScopeAllFailures,
	} {
		if shouldAutoRetryFailure(scope, codexCyberPolicyErrorCode, "temporarily unavailable") {
			t.Fatalf("expected cyber policy failure not to retry for scope %q", scope)
		}
	}
}
