package websession

import (
	"encoding/json"
	"testing"
	"time"
)

func TestSubAgentWireIncrementalsCarryThreadAttributionAndRegistryEntry(t *testing.T) {
	startedAt := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	parentThreadID := "thread_root"
	sourceThreadID := "thread_child"
	historyFrame := newHistoryItemFrame("session_1", HistoryItem{
		ID:             "child_command",
		SourceThreadID: &sourceThreadID,
		OrderIndex:     1,
		Kind:           "tool",
		ItemType:       "command_execution",
	}, nil)
	agentFrame := newSubAgentFrame("session_1", WebSessionSubAgent{
		ThreadID:       sourceThreadID,
		ParentThreadID: &parentThreadID,
		Nickname:       "Atlas",
		Role:           "worker",
		Status:         WebSessionSubAgentRunning,
		StartedAt:      &startedAt,
	}, nil)

	encoded, err := json.Marshal(historyFrame)
	if err != nil {
		t.Fatalf("marshal history frame: %v", err)
	}
	var historyPayload map[string]any
	if err := json.Unmarshal(encoded, &historyPayload); err != nil {
		t.Fatalf("decode history frame: %v", err)
	}
	item, _ := historyPayload["i"].(map[string]any)
	if item["sthid"] != sourceThreadID {
		t.Fatalf("expected compact source thread id, got %#v", item)
	}

	encoded, err = json.Marshal(agentFrame)
	if err != nil {
		t.Fatalf("marshal sub-agent frame: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("decode sub-agent frame: %v", err)
	}
	agent, _ := payload["ag"].(map[string]any)
	if agent["tid"] != sourceThreadID || agent["ptid"] != parentThreadID ||
		agent["nn"] != "Atlas" || agent["rl"] != "worker" || agent["st"] != "running" {
		t.Fatalf("unexpected compact sub-agent entry: %#v", agent)
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
