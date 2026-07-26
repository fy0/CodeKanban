package websession

import (
	"encoding/json"
	"testing"
	"time"
)

func TestSubAgentWireSnapshotCarriesThreadAttributionAndRegistry(t *testing.T) {
	startedAt := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	parentThreadID := "thread_root"
	sourceThreadID := "thread_child"
	snapshot := SessionSnapshot{
		History: HistoryWindow{
			Items: []HistoryItem{
				{
					ID:             "child_command",
					SourceThreadID: &sourceThreadID,
					OrderIndex:     1,
					Kind:           "tool",
					ItemType:       "command_execution",
				},
			},
			Total: 1,
		},
		SubAgents: []WebSessionSubAgent{
			{
				ThreadID:       sourceThreadID,
				ParentThreadID: &parentThreadID,
				Nickname:       "Atlas",
				Role:           "worker",
				Status:         WebSessionSubAgentRunning,
				StartedAt:      &startedAt,
			},
		},
	}

	encoded, err := json.Marshal(newSnapshotFrame("session_1", snapshot))
	if err != nil {
		t.Fatalf("marshal snapshot frame: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("decode snapshot frame: %v", err)
	}
	agents, ok := payload["ags"].([]any)
	if !ok || len(agents) != 1 {
		t.Fatalf("expected one compact sub-agent entry, got %s", encoded)
	}
	agent, _ := agents[0].(map[string]any)
	if agent["tid"] != sourceThreadID || agent["ptid"] != parentThreadID ||
		agent["nn"] != "Atlas" || agent["rl"] != "worker" || agent["st"] != "running" {
		t.Fatalf("unexpected compact sub-agent entry: %#v", agent)
	}
	history, _ := payload["h"].(map[string]any)
	items, _ := history["its"].([]any)
	item, _ := items[0].(map[string]any)
	if item["sthid"] != sourceThreadID {
		t.Fatalf("expected compact source thread id, got %#v", item)
	}

	emptyEncoded, err := json.Marshal(newSnapshotFrame("session_empty", SessionSnapshot{}))
	if err != nil {
		t.Fatalf("marshal empty snapshot frame: %v", err)
	}
	var emptyPayload map[string]any
	if err := json.Unmarshal(emptyEncoded, &emptyPayload); err != nil {
		t.Fatalf("decode empty snapshot frame: %v", err)
	}
	emptyAgents, ok := emptyPayload["ags"].([]any)
	if !ok || len(emptyAgents) != 0 {
		t.Fatalf("new snapshots must advertise an authoritative empty registry, got %s", emptyEncoded)
	}
}

func TestSubAgentWireIncrementalUsesDedicatedOperation(t *testing.T) {
	agent := WebSessionSubAgent{
		ThreadID: "thread_child",
		Status:   WebSessionSubAgentCompleted,
		Summary:  "Review complete",
	}
	encoded, err := json.Marshal(newSubAgentFrame("session_1", agent, nil))
	if err != nil {
		t.Fatalf("marshal sub-agent frame: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("decode sub-agent frame: %v", err)
	}
	if payload["k"] != "evt" || payload["op"] != "sub_agent" {
		t.Fatalf("unexpected incremental frame envelope: %s", encoded)
	}
	compactAgent, _ := payload["ag"].(map[string]any)
	if compactAgent["tid"] != "thread_child" || compactAgent["st"] != "completed" ||
		compactAgent["sm"] != "Review complete" {
		t.Fatalf("unexpected incremental agent payload: %#v", compactAgent)
	}
}
