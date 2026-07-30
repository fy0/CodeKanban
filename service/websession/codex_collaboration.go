package websession

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

const codexCollaborationNamespace = "collaboration"

type codexCollaborationCall struct {
	ThreadID  string
	TurnID    string
	CallID    string
	Name      string
	Input     map[string]any
	ParentID  string
	StartedAt time.Time
	Ignored   bool
}

type codexCollaborationCallKey struct {
	ThreadID string
	TurnID   string
	CallID   string
}

type codexCollaborationCallState struct {
	Call      codexCollaborationCall
	Covered   bool
	Resolving bool
}

type codexCollaborationTracker struct {
	mu       sync.Mutex
	pending  map[codexCollaborationCallKey]codexCollaborationCallState
	covered  map[codexCollaborationCallKey]struct{}
	resolved map[codexCollaborationCallKey]struct{}
}

func (t *codexCollaborationTracker) record(call codexCollaborationCall) bool {
	key := call.key()
	if key.CallID == "" || key.ThreadID == "" || key.TurnID == "" {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ensureMaps()
	if _, ok := t.resolved[key]; ok {
		return true
	}
	if _, ok := t.pending[key]; ok {
		return true
	}
	_, covered := t.covered[key]
	t.pending[key] = codexCollaborationCallState{Call: call, Covered: covered}
	return true
}

func (t *codexCollaborationTracker) markCovered(threadID string, turnID string, callID string) {
	key := newCodexCollaborationCallKey(threadID, turnID, callID)
	if key.CallID == "" || key.ThreadID == "" || key.TurnID == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ensureMaps()
	t.covered[key] = struct{}{}
	if state, ok := t.pending[key]; ok {
		state.Covered = true
		t.pending[key] = state
	}
}

func (t *codexCollaborationTracker) markCoveredByCall(threadID string, callID string) {
	threadID = strings.TrimSpace(threadID)
	callID = strings.TrimSpace(callID)
	if threadID == "" || callID == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ensureMaps()
	key := t.findKeyLocked(threadID, callID)
	if key.TurnID == "" {
		return
	}
	t.covered[key] = struct{}{}
	state := t.pending[key]
	state.Covered = true
	t.pending[key] = state
}

func (t *codexCollaborationTracker) resolve(
	threadID string,
	turnID string,
	callID string,
	output any,
) (codexCollaborationCall, string, bool, bool) {
	key := newCodexCollaborationCallKey(threadID, turnID, callID)
	if key.CallID == "" || key.ThreadID == "" {
		return codexCollaborationCall{}, "", false, false
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	t.ensureMaps()
	if key.TurnID == "" {
		key = t.findKeyLocked(key.ThreadID, key.CallID)
	}
	if key.TurnID == "" {
		return codexCollaborationCall{}, "", false, false
	}
	if _, ok := t.resolved[key]; ok {
		return codexCollaborationCall{}, "", false, true
	}
	state, ok := t.pending[key]
	if !ok {
		return codexCollaborationCall{}, "", false, false
	}
	if state.Resolving {
		return codexCollaborationCall{}, "", false, true
	}

	text := codexCollaborationOutputText(output)
	if state.Call.Ignored || state.Covered || codexCollaborationOutputSucceeded(state.Call.Name, text) {
		delete(t.pending, key)
		delete(t.covered, key)
		t.resolved[key] = struct{}{}
		return state.Call, text, false, true
	}
	state.Resolving = true
	t.pending[key] = state
	return state.Call, text, true, true
}

func (t *codexCollaborationTracker) finishFailure(call codexCollaborationCall, persisted bool) {
	key := call.key()
	if key.CallID == "" || key.ThreadID == "" || key.TurnID == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ensureMaps()
	state, ok := t.pending[key]
	if !ok {
		return
	}
	if persisted {
		delete(t.pending, key)
		delete(t.covered, key)
		t.resolved[key] = struct{}{}
		return
	}
	state.Resolving = false
	t.pending[key] = state
}

func (t *codexCollaborationTracker) clear() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pending = nil
	t.covered = nil
	t.resolved = nil
}

func (t *codexCollaborationTracker) ensureMaps() {
	if t.pending == nil {
		t.pending = make(map[codexCollaborationCallKey]codexCollaborationCallState)
	}
	if t.covered == nil {
		t.covered = make(map[codexCollaborationCallKey]struct{})
	}
	if t.resolved == nil {
		t.resolved = make(map[codexCollaborationCallKey]struct{})
	}
}

func (t *codexCollaborationTracker) findKeyLocked(threadID string, callID string) codexCollaborationCallKey {
	for key := range t.pending {
		if key.ThreadID == threadID && key.CallID == callID {
			return key
		}
	}
	return codexCollaborationCallKey{}
}

func (c codexCollaborationCall) key() codexCollaborationCallKey {
	return newCodexCollaborationCallKey(c.ThreadID, c.TurnID, c.CallID)
}

func newCodexCollaborationCallKey(threadID string, turnID string, callID string) codexCollaborationCallKey {
	return codexCollaborationCallKey{
		ThreadID: strings.TrimSpace(threadID),
		TurnID:   strings.TrimSpace(turnID),
		CallID:   strings.TrimSpace(callID),
	}
}

func parseCodexCollaborationCall(
	threadID string,
	turnID string,
	item map[string]any,
	startedAt time.Time,
) (codexCollaborationCall, bool) {
	if strings.TrimSpace(stringValue(item["type"])) != "function_call" {
		return codexCollaborationCall{}, false
	}

	name := strings.TrimSpace(stringValue(item["name"]))
	namespace := strings.TrimSpace(stringValue(item["namespace"]))
	if namespace != codexCollaborationNamespace &&
		(namespace != "" || !isCodexV2CollaborationToolName(name)) {
		return codexCollaborationCall{}, false
	}
	call := codexCollaborationCall{
		ThreadID:  strings.TrimSpace(threadID),
		TurnID:    strings.TrimSpace(turnID),
		CallID:    strings.TrimSpace(stringValue(item["call_id"])),
		Name:      name,
		StartedAt: startedAt,
	}
	if call.TurnID == "" {
		call.TurnID = codexResponseItemTurnID(item)
	}

	arguments := decodeDeepSyncArguments(stringValue(item["arguments"]))
	switch name {
	case "spawn_agent":
		call.Input = sanitizeCodexCollaborationInput(name, arguments)
	case "send_message", "followup_task", "interrupt_agent", "wait_agent":
		call.Input = sanitizeCodexCollaborationInput(name, arguments)
	case "list_agents":
		call.Ignored = true
		call.Input = map[string]any{"tool": "listAgents"}
	default:
		return codexCollaborationCall{}, false
	}
	return call, call.ThreadID != "" && call.TurnID != "" && call.CallID != ""
}

func isCodexV2CollaborationToolName(name string) bool {
	switch strings.TrimSpace(name) {
	case "spawn_agent", "send_message", "followup_task", "interrupt_agent", "wait_agent", "list_agents":
		return true
	default:
		return false
	}
}

func sanitizeCodexCollaborationInput(name string, raw any) map[string]any {
	record := decodeRawObject(raw)
	result := map[string]any{"tool": codexCollaborationOperation(name)}
	copyValue := func(key string) {
		if value, ok := record[key]; ok {
			switch value.(type) {
			case string, float64, int, int64, bool:
				result[key] = value
			}
		}
	}
	switch name {
	case "spawn_agent":
		for _, key := range []string{"task_name", "fork_turns", "agent_type", "model", "reasoning_effort"} {
			copyValue(key)
		}
	case "send_message", "followup_task", "interrupt_agent":
		copyValue("target")
	case "wait_agent":
		copyValue("timeout_ms")
	}
	return result
}

func codexCollaborationOperation(name string) string {
	switch strings.TrimSpace(name) {
	case "spawn_agent":
		return "spawnAgent"
	case "send_message":
		return "sendMessage"
	case "followup_task":
		return "followupTask"
	case "interrupt_agent":
		return "interruptAgent"
	case "wait_agent":
		return "wait"
	case "list_agents":
		return "listAgents"
	default:
		return strings.TrimSpace(name)
	}
}

func codexCollaborationOutputText(raw any) string {
	if text, ok := raw.(string); ok {
		return strings.TrimSpace(text)
	}
	parts := make([]string, 0)
	for _, item := range decodeRawSlice(raw) {
		record := decodeRawObject(item)
		if strings.TrimSpace(stringValue(record["type"])) != "input_text" {
			continue
		}
		if text := strings.TrimSpace(stringValue(record["text"])); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func decodeRawSlice(raw any) []any {
	switch typed := raw.(type) {
	case []any:
		return typed
	case nil:
		return nil
	default:
		return nil
	}
}

func codexCollaborationOutputSucceeded(name string, output string) bool {
	switch strings.TrimSpace(name) {
	case "send_message", "followup_task":
		return strings.TrimSpace(output) == ""
	case "spawn_agent":
		var payload struct {
			TaskName string `json:"task_name"`
		}
		return json.Unmarshal([]byte(output), &payload) == nil && strings.TrimSpace(payload.TaskName) != ""
	case "interrupt_agent":
		var payload map[string]json.RawMessage
		if json.Unmarshal([]byte(output), &payload) != nil {
			return false
		}
		_, ok := payload["previous_status"]
		return ok
	case "wait_agent":
		var payload struct {
			Message  string `json:"message"`
			TimedOut *bool  `json:"timed_out"`
		}
		return json.Unmarshal([]byte(output), &payload) == nil &&
			strings.TrimSpace(payload.Message) != "" && payload.TimedOut != nil
	default:
		return false
	}
}

func codexCollaborationMeta(call codexCollaborationCall) map[string]any {
	subtitle := firstNonEmpty(
		stringValue(call.Input["task_name"]),
		stringValue(call.Input["target"]),
	)
	if subtitle == "" {
		if timeout, ok := call.Input["timeout_ms"]; ok {
			subtitle = fmt.Sprintf("%v ms", timeout)
		}
	}
	return map[string]any{
		"title":    "Sub Agent",
		"kind":     "sub_agent_tool_call",
		"subtitle": subtitle,
	}
}
