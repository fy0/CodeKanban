package websession

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCodexRolloutTailerAcceptsLargeSessionMetadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	metadata, err := json.Marshal(map[string]any{
		"type": "session_meta",
		"payload": map[string]any{
			"id":                "thread-large-meta",
			"base_instructions": strings.Repeat("x", 300*1024),
		},
	})
	if err != nil {
		t.Fatalf("marshal session metadata: %v", err)
	}
	if err := os.WriteFile(path, append(metadata, '\n'), 0o600); err != nil {
		t.Fatalf("write rollout: %v", err)
	}
	if _, err := newCodexRolloutTailer(path, "thread-large-meta"); err != nil {
		t.Fatalf("large valid session metadata was rejected: %v", err)
	}
}

func TestCodexRolloutTailerStartsAtPaginatedSubagentBoundary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	content := strings.Join([]string{
		`{"timestamp":"2026-07-30T01:00:00Z","ordinal":0,"type":"session_meta","payload":{"id":"thread-child","subagent_history_start_ordinal":3}}`,
		`{"timestamp":"2026-07-30T01:00:01Z","ordinal":1,"type":"response_item","payload":{"type":"function_call","call_id":"inherited-call"}}`,
		`{"timestamp":"2026-07-30T01:00:02Z","ordinal":2,"type":"response_item","payload":{"type":"function_call_output","call_id":"inherited-call"}}`,
		`{"timestamp":"2026-07-30T01:00:03Z","ordinal":3,"type":"response_item","payload":{"type":"function_call","call_id":"local-call"}}`,
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write rollout: %v", err)
	}
	tailer, err := newCodexRolloutTailerAtOffset(path, "thread-child", true)
	if err != nil {
		t.Fatalf("new child tailer: %v", err)
	}
	var callIDs []string
	if err := tailer.drain(func(entry codexRolloutEntry) error {
		if callID := stringValue(entry.Payload["call_id"]); callID != "" {
			callIDs = append(callIDs, callID)
		}
		return nil
	}); err != nil {
		t.Fatalf("drain child rollout: %v", err)
	}
	if len(callIDs) != 1 || callIDs[0] != "local-call" {
		t.Fatalf("expected only local child history, got %#v", callIDs)
	}
}

func TestCodexRolloutTailerReadsOnlyCompleteAppendedLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	initial := strings.Join([]string{
		`{"type":"session_meta","payload":{"id":"thread-1"}}`,
		`{"type":"response_item","payload":{"type":"message"}}`,
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
		t.Fatalf("write rollout: %v", err)
	}

	tailer, err := newCodexRolloutTailer(path, "thread-1")
	if err != nil {
		t.Fatalf("newCodexRolloutTailer: %v", err)
	}
	partial := `{"type":"response_item","payload":{"type":"function_call","call_id":"call-1"}}`
	appendRolloutTestData(t, path, partial)

	var entries []codexRolloutEntry
	if err := tailer.drain(func(entry codexRolloutEntry) error {
		entries = append(entries, entry)
		return nil
	}); err != nil {
		t.Fatalf("drain partial line: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected partial line to remain pending, got %d entries", len(entries))
	}

	appendRolloutTestData(t, path, "\r\n")
	if err := tailer.drain(func(entry codexRolloutEntry) error {
		entries = append(entries, entry)
		return nil
	}); err != nil {
		t.Fatalf("drain completed line: %v", err)
	}
	if len(entries) != 1 || stringValue(entries[0].Payload["call_id"]) != "call-1" {
		t.Fatalf("unexpected entries: %#v", entries)
	}
	if err := tailer.drain(func(entry codexRolloutEntry) error {
		entries = append(entries, entry)
		return nil
	}); err != nil {
		t.Fatalf("drain without changes: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected appended line exactly once, got %d entries", len(entries))
	}
}

func TestCodexRolloutTailerSkipsCompleteMalformedLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	if err := os.WriteFile(path, []byte(`{"type":"session_meta","payload":{"id":"thread-1"}}`+"\n"), 0o600); err != nil {
		t.Fatalf("write rollout: %v", err)
	}
	tailer, err := newCodexRolloutTailer(path, "thread-1")
	if err != nil {
		t.Fatalf("newCodexRolloutTailer: %v", err)
	}
	appendRolloutTestData(t, path, "not-json\n"+`{"type":"event_msg","payload":{"type":"turn_complete"}}`+"\n")

	var entries []codexRolloutEntry
	if err := tailer.drain(func(entry codexRolloutEntry) error {
		entries = append(entries, entry)
		return nil
	}); err != nil {
		t.Fatalf("drain: %v", err)
	}
	if len(entries) != 1 || stringValue(entries[0].Payload["type"]) != "turn_complete" {
		t.Fatalf("unexpected entries: %#v", entries)
	}
}

func TestCodexRolloutTailerRetriesEntryWhenHandlerFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	if err := os.WriteFile(path, []byte(`{"type":"session_meta","payload":{"id":"thread-1"}}`+"\n"), 0o600); err != nil {
		t.Fatalf("write rollout: %v", err)
	}
	tailer, err := newCodexRolloutTailer(path, "thread-1")
	if err != nil {
		t.Fatalf("newCodexRolloutTailer: %v", err)
	}
	appendRolloutTestData(t, path, `{"type":"response_item","payload":{"type":"function_call","call_id":"call-1"}}`+"\n")

	attempts := 0
	handle := func(entry codexRolloutEntry) error {
		attempts++
		if attempts == 1 {
			return errors.New("persist failed")
		}
		return nil
	}
	if err := tailer.drain(handle); err == nil {
		t.Fatal("expected first handler failure")
	}
	if err := tailer.drain(handle); err != nil {
		t.Fatalf("retry drain: %v", err)
	}
	if err := tailer.drain(handle); err != nil {
		t.Fatalf("final empty drain: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("expected failed entry to retry exactly once, got %d attempts", attempts)
	}
}

func TestCodexRolloutMonitorRetriesHandlerFailureDuringPolling(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	if err := os.WriteFile(path, []byte(`{"type":"session_meta","payload":{"id":"thread-1"}}`+"\n"), 0o600); err != nil {
		t.Fatalf("write rollout: %v", err)
	}
	tailer, err := newCodexRolloutTailer(path, "thread-1")
	if err != nil {
		t.Fatalf("newCodexRolloutTailer: %v", err)
	}
	appendRolloutTestData(t, path, `{"type":"response_item","payload":{"type":"function_call","call_id":"call-1"}}`+"\n")

	attempts := make(chan struct{}, 3)
	succeeded := make(chan struct{})
	monitor, err := newCodexRolloutMonitor(
		context.Background(),
		time.Time{},
		map[string]*codexRolloutTailer{"thread-1": tailer},
		func(string, codexRolloutEntry) error {
			attempts <- struct{}{}
			if len(attempts) == 1 {
				return errors.New("persist failed")
			}
			select {
			case <-succeeded:
			default:
				close(succeeded)
			}
			return nil
		},
		nil,
	)
	if err != nil {
		t.Fatalf("new rollout monitor: %v", err)
	}
	defer monitor.stopAndDrain()

	select {
	case <-succeeded:
	case <-time.After(3 * time.Second):
		t.Fatalf("monitor did not retry handler failure; attempts=%d", len(attempts))
	}
	if got := len(attempts); got != 2 {
		t.Fatalf("expected exactly two handler attempts, got %d", got)
	}
}

func TestCodexRolloutTailerRejectsWrongSessionAndTruncation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	if err := os.WriteFile(path, []byte(`{"type":"session_meta","payload":{"id":"thread-1"}}`+"\n"), 0o600); err != nil {
		t.Fatalf("write rollout: %v", err)
	}
	if _, err := newCodexRolloutTailer(path, "thread-2"); err == nil {
		t.Fatal("expected wrong-session error")
	}

	tailer, err := newCodexRolloutTailer(path, "thread-1")
	if err != nil {
		t.Fatalf("newCodexRolloutTailer: %v", err)
	}
	if err := os.Truncate(path, 0); err != nil {
		t.Fatalf("truncate rollout: %v", err)
	}
	if err := tailer.drain(func(codexRolloutEntry) error { return nil }); err == nil {
		t.Fatal("expected truncation error")
	}
}

func TestCodexRolloutTailerRejectsReplacedSession(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	initial := strings.Join([]string{
		`{"type":"session_meta","payload":{"id":"thread-1"}}`,
		`{"type":"response_item","payload":{"type":"message"}}`,
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
		t.Fatalf("write rollout: %v", err)
	}
	tailer, err := newCodexRolloutTailer(path, "thread-1")
	if err != nil {
		t.Fatalf("newCodexRolloutTailer: %v", err)
	}

	replacement := strings.Join([]string{
		`{"type":"session_meta","payload":{"id":"thread-2"}}`,
		`{"type":"response_item","payload":{"type":"function_call","call_id":"other-thread-call"}}`,
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(replacement), 0o600); err != nil {
		t.Fatalf("replace rollout: %v", err)
	}
	if err := tailer.drain(func(codexRolloutEntry) error { return nil }); err == nil ||
		!strings.Contains(err.Error(), "belongs to thread thread-2") {
		t.Fatalf("expected replaced-session error, got %v", err)
	}
}

func TestCodexRolloutMonitorBaselinesExistingAndReplaysNewDescendants(t *testing.T) {
	rootPath := filepath.Join(t.TempDir(), "root.jsonl")
	existingPath := filepath.Join(t.TempDir(), "existing.jsonl")
	newPath := filepath.Join(t.TempDir(), "new.jsonl")
	cutoff := time.Date(2026, 7, 30, 1, 0, 0, 0, time.UTC)
	writeRollout := func(path string, threadID string, lines ...string) {
		t.Helper()
		content := append([]string{
			`{"timestamp":"2026-07-30T00:00:00Z","type":"session_meta","payload":{"id":"` + threadID + `"}}`,
		}, lines...)
		if err := os.WriteFile(path, []byte(strings.Join(append(content, ""), "\n")), 0o600); err != nil {
			t.Fatalf("write rollout %s: %v", threadID, err)
		}
	}
	writeRollout(rootPath, "thread-root")
	writeRollout(existingPath, "thread-existing",
		`{"timestamp":"2026-07-30T00:30:00Z","type":"response_item","payload":{"type":"function_call","call_id":"old-existing"}}`,
	)
	writeRollout(newPath, "thread-new",
		`{"timestamp":"2026-07-30T00:30:00Z","type":"response_item","payload":{"type":"function_call","call_id":"old-new"}}`,
		`{"timestamp":"2026-07-30T01:00:01Z","type":"response_item","payload":{"type":"function_call","call_id":"current-new"}}`,
	)

	rootTailer, err := newCodexRolloutTailer(rootPath, "thread-root")
	if err != nil {
		t.Fatalf("root tailer: %v", err)
	}
	existingTailer, err := newCodexRolloutTailer(existingPath, "thread-existing")
	if err != nil {
		t.Fatalf("existing tailer: %v", err)
	}
	appendRolloutTestData(t, existingPath,
		`{"timestamp":"2026-07-30T01:00:02Z","type":"response_item","payload":{"type":"function_call","call_id":"current-existing"}}`+"\n",
	)

	type observedEntry struct {
		threadID string
		callID   string
	}
	observed := make([]observedEntry, 0)
	monitor, err := newCodexRolloutMonitor(
		context.Background(),
		cutoff,
		map[string]*codexRolloutTailer{
			"thread-root":     rootTailer,
			"thread-existing": existingTailer,
		},
		func(threadID string, entry codexRolloutEntry) error {
			if callID := strings.TrimSpace(stringValue(entry.Payload["call_id"])); callID != "" {
				observed = append(observed, observedEntry{threadID: threadID, callID: callID})
			}
			return nil
		},
		nil,
	)
	if err != nil {
		t.Fatalf("new monitor: %v", err)
	}
	if err := monitor.addThreadFromBeginning("thread-new", newPath); err != nil {
		t.Fatalf("add new descendant: %v", err)
	}
	monitor.stopAndDrain()

	got := make(map[string]string, len(observed))
	for _, entry := range observed {
		got[entry.threadID] = entry.callID
	}
	if got["thread-existing"] != "current-existing" || got["thread-new"] != "current-new" {
		t.Fatalf("unexpected monitored entries: %#v", observed)
	}
	for _, entry := range observed {
		if strings.HasPrefix(entry.callID, "old-") {
			t.Fatalf("historical descendant entry was replayed: %#v", observed)
		}
	}
}

func appendRolloutTestData(t *testing.T, path string, content string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open rollout for append: %v", err)
	}
	defer file.Close()
	if _, err := file.WriteString(content); err != nil {
		t.Fatalf("append rollout: %v", err)
	}
}
