package websession

import (
	"encoding/json"
	"testing"
	"time"
)

func TestPendingWireMarksPiNativeQueueReadOnly(t *testing.T) {
	createdAt := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	encoded, err := json.Marshal(newPendingFrame("session_1", "epoch_1", 1, []PendingInput{
		{
			ID:           "pi-native-1",
			Mode:         PendingInputModeQueue,
			Text:         "Accepted by Pi",
			NativeQueued: true,
			CreatedAt:    createdAt,
		},
	}))
	if err != nil {
		t.Fatalf("marshal pending frame: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("decode pending frame: %v", err)
	}
	items, ok := payload["pi"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected one pending item, got %s", encoded)
	}
	item, _ := items[0].(map[string]any)
	if item["nq"] != true || item["id"] != "pi-native-1" || item["m"] != string(PendingInputModeQueue) {
		t.Fatalf("expected compact native queue marker, got %#v", item)
	}
}

func TestPendingWireIncludesCodexSteerRetryState(t *testing.T) {
	createdAt := time.Date(2026, 8, 26, 6, 0, 0, 0, time.UTC)
	encoded, err := json.Marshal(newPendingFrame("session_1", "epoch_1", 1, []PendingInput{
		{
			ID:            "codex-steer-1",
			Mode:          PendingInputModeRedirect,
			Text:          "Continue with the corrected scope",
			Status:        PendingInputStatusRetrying,
			AttemptCount:  3,
			LastError:     "active turn is temporarily unavailable",
			LastErrorCode: "activeTurnNotSteerable",
			CreatedAt:     createdAt,
		},
	}))
	if err != nil {
		t.Fatalf("marshal pending frame: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("decode pending frame: %v", err)
	}
	items, ok := payload["pi"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected one pending item, got %s", encoded)
	}
	item, _ := items[0].(map[string]any)
	if item["st"] != string(PendingInputStatusRetrying) || item["ac"] != float64(3) ||
		item["err"] != "active turn is temporarily unavailable" ||
		item["ec"] != "activeTurnNotSteerable" {
		t.Fatalf("expected compact retry metadata, got %#v", item)
	}
}

func TestPendingWireIncludesClockAndExplicitEmptySnapshot(t *testing.T) {
	encoded, err := json.Marshal(newPendingFrame("session_1", "epoch_1", 0, nil))
	if err != nil {
		t.Fatalf("marshal pending frame: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("decode pending frame: %v", err)
	}
	items, ok := payload["pi"].([]any)
	if !ok || len(items) != 0 || payload["pe"] != "epoch_1" || payload["pv"] != float64(0) {
		t.Fatalf("expected pending clock and explicit empty snapshot, got %s", encoded)
	}
	if _, ok := payload["rev"]; ok {
		t.Fatalf("pending frames must not carry a durable revision, got %s", encoded)
	}
}
