package log_watcher

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"code-kanban/utils/ai_assistant2/types"
)

func writePiSessionFixture(
	t *testing.T,
	root string,
	dirName string,
	sessionID string,
	cwd string,
	startedAt time.Time,
	message string,
) string {
	t.Helper()
	dir := filepath.Join(root, dirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create Pi session dir: %v", err)
	}
	filePath := filepath.Join(dir, startedAt.UTC().Format("20060102T150405")+"_"+sessionID+".jsonl")
	content := fmt.Sprintf(
		`{"type":"session","version":3,"id":%q,"timestamp":%q,"cwd":%q}`+"\n"+
			`{"type":"model_change","id":"model001","parentId":null,"timestamp":%q,"provider":"openai","modelId":"gpt-5"}`+"\n"+
			`{"type":"message","id":"message1","parentId":"model001","timestamp":%q,"message":{"role":"user","content":[{"type":"text","text":%q}],"timestamp":%d}}`+"\n",
		sessionID,
		startedAt.UTC().Format(time.RFC3339Nano),
		cwd,
		startedAt.UTC().Format(time.RFC3339Nano),
		startedAt.Add(time.Second).UTC().Format(time.RFC3339Nano),
		message,
		startedAt.Add(time.Second).UnixMilli(),
	)
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatalf("write Pi session: %v", err)
	}
	if err := os.Chtimes(filePath, startedAt, startedAt); err != nil {
		t.Fatalf("set Pi session times: %v", err)
	}
	return filePath
}

func TestResolvePiSessionDirHonorsOverrides(t *testing.T) {
	custom := filepath.Join(t.TempDir(), "custom-sessions")
	t.Setenv("PI_CODING_AGENT_SESSION_DIR", custom)
	t.Setenv("PI_CODING_AGENT_DIR", filepath.Join(t.TempDir(), "ignored-agent"))
	got, err := ResolvePiSessionDir()
	if err != nil {
		t.Fatalf("ResolvePiSessionDir returned error: %v", err)
	}
	if got != filepath.Clean(custom) {
		t.Fatalf("session dir = %q, want %q", got, filepath.Clean(custom))
	}

	t.Setenv("PI_CODING_AGENT_SESSION_DIR", "")
	agentDir := filepath.Join(t.TempDir(), "agent")
	t.Setenv("PI_CODING_AGENT_DIR", agentDir)
	got, err = ResolvePiSessionDir()
	if err != nil {
		t.Fatalf("ResolvePiSessionDir with agent dir returned error: %v", err)
	}
	if got != filepath.Join(agentDir, "sessions") {
		t.Fatalf("session dir = %q, want agent sessions dir", got)
	}
}

func TestPiFileSearcherMatchesHeaderCwdAndStartTime(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project-a")
	otherProject := filepath.Join(root, "project-b")
	start := time.Now().UTC().Truncate(time.Second)

	expected := writePiSessionFixture(t, root, "encoded-a", "session-a", project, start.Add(time.Second), "first")
	writePiSessionFixture(t, root, "misleading-encoded-a", "session-b", otherProject, start.Add(2*time.Second), "other")
	writePiSessionFixture(t, root, "encoded-a", "session-old", project, start.Add(-time.Hour), "old")
	if err := os.WriteFile(filepath.Join(root, "encoded-a", "malformed.jsonl"), []byte("not-json\n"), 0o644); err != nil {
		t.Fatalf("write malformed fixture: %v", err)
	}

	searcher := NewPiFileSearcherWithSessionDir(root, project)
	found, err := searcher.FindSessionFile(context.Background(), start)
	if err != nil {
		t.Fatalf("FindSessionFile returned error: %v", err)
	}
	if found != expected {
		t.Fatalf("found %q, want %q", found, expected)
	}

	byID, err := searcher.FindBySessionID(context.Background(), "session-a")
	if err != nil || byID != expected {
		t.Fatalf("FindBySessionID = %q, %v; want %q", byID, err, expected)
	}
}

func TestPiFileSearcherFindsResumedSessionByModificationTime(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	processStartedAt := time.Now().UTC().Truncate(time.Second)
	resumed := writePiSessionFixture(
		t,
		root,
		"encoded",
		"resumed-session",
		project,
		processStartedAt.Add(-30*24*time.Hour),
		"old request",
	)
	modifiedAt := processStartedAt.Add(2 * time.Second)
	if err := os.Chtimes(resumed, modifiedAt, modifiedAt); err != nil {
		t.Fatalf("mark resumed fixture as modified: %v", err)
	}
	writePiSessionFixture(
		t,
		root,
		"encoded",
		"inactive-session",
		project,
		processStartedAt.Add(-24*time.Hour),
		"inactive",
	)

	searcher := NewPiFileSearcherWithSessionDir(root, project)
	found, err := searcher.FindSessionFile(context.Background(), processStartedAt)
	if err != nil {
		t.Fatalf("FindSessionFile returned error: %v", err)
	}
	if found != resumed {
		t.Fatalf("found %q, want resumed session %q", found, resumed)
	}
}

func TestParsePiLineCapturesSessionModelAndUserMessage(t *testing.T) {
	watcher := NewLogWatcher(WatcherConfig{})
	watcher.parseLineFn = ParsePiLineWrapper

	for _, line := range []string{
		`{"type":"session","version":3,"id":"session-1","timestamp":"2026-08-11T10:00:00Z","cwd":"D:\\repo"}`,
		`{"type":"model_change","id":"model001","parentId":null,"timestamp":"2026-08-11T10:00:01Z","provider":"openai","modelId":"gpt-5"}`,
	} {
		if message, err := watcher.parseLine(line); err != nil || message != nil {
			t.Fatalf("parse metadata = %#v, %v", message, err)
		}
	}
	message, err := watcher.parseLine(`{"type":"message","id":"message1","parentId":"model001","timestamp":"2026-08-11T10:00:02Z","message":{"role":"user","content":[{"type":"text","text":"hello"}]}}`)
	if err != nil || message == nil || message.Message != "hello" {
		t.Fatalf("parse user message = %#v, %v", message, err)
	}
	info := watcher.Info()
	if info.SessionID != "session-1" || info.SessionMeta == nil || info.SessionMeta.Model != "openai/gpt-5" {
		t.Fatalf("unexpected watcher info: %#v", info)
	}
}

func TestFactoryCreatesPiWatcher(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PI_CODING_AGENT_SESSION_DIR", root)
	watcher, err := CreateWatcherForAssistantWithWorkingDir(
		types.AssistantTypePi,
		time.Now(),
		t.TempDir(),
		nil,
		nil,
	)
	if err != nil || watcher == nil || watcher.parseLineFn == nil {
		t.Fatalf("Pi watcher = %#v, %v", watcher, err)
	}
}
