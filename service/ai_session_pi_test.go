package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"code-kanban/model"
	"code-kanban/model/tables"
)

func writeServicePiSession(t *testing.T, root, dirName, id, cwd string, started time.Time, branched bool) string {
	t.Helper()
	dir := filepath.Join(root, dirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create fixture dir: %v", err)
	}
	lines := []string{
		fmt.Sprintf(`{"type":"session","version":3,"id":%q,"timestamp":%q,"cwd":%q}`, id, started.UTC().Format(time.RFC3339Nano), cwd),
		fmt.Sprintf(`{"type":"model_change","id":"model001","parentId":null,"timestamp":%q,"provider":"openai","modelId":"gpt-5"}`, started.Add(time.Second).UTC().Format(time.RFC3339Nano)),
		fmt.Sprintf(`{"type":"message","id":"user0001","parentId":"model001","timestamp":%q,"message":{"role":"user","content":"root request","timestamp":%d}}`, started.Add(2*time.Second).UTC().Format(time.RFC3339Nano), started.Add(2*time.Second).UnixMilli()),
		fmt.Sprintf(`{"type":"message","id":"assist01","parentId":"user0001","timestamp":%q,"message":{"role":"assistant","content":[{"type":"text","text":"root answer"}],"provider":"openai","model":"gpt-5","timestamp":%d}}`, started.Add(3*time.Second).UTC().Format(time.RFC3339Nano), started.Add(3*time.Second).UnixMilli()),
	}
	if branched {
		lines = append(lines,
			fmt.Sprintf(`{"type":"message","id":"olduser1","parentId":"assist01","timestamp":%q,"message":{"role":"user","content":"abandoned request","timestamp":%d}}`, started.Add(4*time.Second).UTC().Format(time.RFC3339Nano), started.Add(4*time.Second).UnixMilli()),
			fmt.Sprintf(`{"type":"message","id":"oldasst1","parentId":"olduser1","timestamp":%q,"message":{"role":"assistant","content":[{"type":"text","text":"abandoned answer"}],"provider":"other","model":"old","timestamp":%d}}`, started.Add(5*time.Second).UTC().Format(time.RFC3339Nano), started.Add(5*time.Second).UnixMilli()),
			fmt.Sprintf(`{"type":"message","id":"activeu1","parentId":"assist01","timestamp":%q,"message":{"role":"user","content":"active request","timestamp":%d}}`, started.Add(6*time.Second).UTC().Format(time.RFC3339Nano), started.Add(6*time.Second).UnixMilli()),
			fmt.Sprintf(`{"type":"message","id":"activea1","parentId":"activeu1","timestamp":%q,"message":{"role":"assistant","content":[{"type":"text","text":"active answer"}],"provider":"openai","model":"gpt-5","timestamp":%d}}`, started.Add(7*time.Second).UTC().Format(time.RFC3339Nano), started.Add(7*time.Second).UnixMilli()),
			fmt.Sprintf(`{"type":"session_info","id":"info0001","parentId":"activea1","timestamp":%q,"name":"Named Pi Session"}`, started.Add(8*time.Second).UTC().Format(time.RFC3339Nano)),
			fmt.Sprintf(`{"type":"custom","id":"marker01","parentId":"info0001","timestamp":%q,"customType":"codekanban.active-leaf.v1"}`, started.Add(9*time.Second).UTC().Format(time.RFC3339Nano)),
		)
	}
	filePath := filepath.Join(dir, started.UTC().Format("20060102T150405")+"_"+id+".jsonl")
	if err := os.WriteFile(filePath, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return filePath
}

func resetPiHistoryTestCaches() {
	dirCacheMu.Lock()
	dirCache = make(map[string]*dirCacheEntry)
	dirCacheMu.Unlock()
	scanStatesMu.Lock()
	scanStates = make(map[string]*scanState)
	scanStatesMu.Unlock()
}

func TestParsePiSessionUsesActiveBranch(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	started := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	filePath := writeServicePiSession(t, root, "encoded", "pi-session", project, started, true)

	svc := NewAISessionService()
	data, err := svc.parsePiSessionFile(filePath)
	if err != nil {
		t.Fatalf("parsePiSessionFile: %v", err)
	}
	if data.SessionID != "pi-session" || data.Cwd != filepath.Clean(project) {
		t.Fatalf("unexpected identity: id=%q cwd=%q", data.SessionID, data.Cwd)
	}
	if data.Title != "Named Pi Session" || data.Model != "openai/gpt-5" {
		t.Fatalf("unexpected metadata: title=%q model=%q", data.Title, data.Model)
	}
	if data.MessageCount != 2 || data.AssistantMessageCount != 2 {
		t.Fatalf("unexpected active counts: user=%d assistant=%d", data.MessageCount, data.AssistantMessageCount)
	}

	messages, err := svc.parsePiConversation(filePath)
	if err != nil {
		t.Fatalf("parsePiConversation: %v", err)
	}
	if len(messages) != 4 {
		t.Fatalf("active conversation length = %d, want 4", len(messages))
	}
	joined := ""
	for _, message := range messages {
		joined += message.Content + "\n"
	}
	if strings.Contains(joined, "abandoned") || !strings.Contains(joined, "active request") {
		t.Fatalf("conversation did not follow active branch: %q", joined)
	}
}

func TestGetPiSessionsKeepsInProgressScanStatePastCacheTTL(t *testing.T) {
	resetPiHistoryTestCaches()
	root := t.TempDir()
	project := filepath.Join(root, "project")
	t.Setenv("PI_CODING_AGENT_SESSION_DIR", root)
	stateKey := piScanStateKey(project, root)
	expected := &AISessionSummary{SessionID: "already-discovered", Type: string(tables.AISessionTypePi)}
	scanStatesMu.Lock()
	scanStates[stateKey] = &scanState{
		phase:            ScanPhaseExtended,
		sessions:         []*AISessionSummary{expected},
		cursor:           filepath.Join(root, "cursor.jsonl"),
		hasMore:          true,
		backgroundActive: true,
	}
	scanStatesMu.Unlock()
	dirCacheMu.Lock()
	dirCache[piSessionCacheKey(project, root)] = &dirCacheEntry{
		sessions: []*AISessionSummary{},
		cachedAt: time.Now().Add(-2 * dirCacheTTL),
	}
	dirCacheMu.Unlock()

	sessions, phase, err := NewAISessionService().getPiSessionsPhased(context.Background(), project)
	if err != nil {
		t.Fatalf("getPiSessionsPhased returned error: %v", err)
	}
	if phase != ScanPhaseExtended || len(sessions) != 1 || sessions[0].SessionID != expected.SessionID {
		t.Fatalf("in-progress snapshot = %#v, phase=%q", sessions, phase)
	}
	state := scanStates[stateKey]
	state.mu.RLock()
	defer state.mu.RUnlock()
	if state.cursor == "" || !state.hasMore || len(state.sessions) != 1 {
		t.Fatalf("in-progress state was reset: %#v", state)
	}
}

func TestPiSessionDiscoveryUsesBoundedCursorBatches(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatalf("create project: %v", err)
	}
	started := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	for index := 0; index < piSessionDiscoveryBatchSize+44; index++ {
		id := fmt.Sprintf("session-%03d", index)
		writeServicePiSession(t, root, "sessions", id, project, started.Add(time.Duration(index)*time.Second), false)
	}

	first, cursor, hasMore, err := listPiSessionCandidateBatch(
		context.Background(),
		root,
		canonicalAIProjectPath(project),
		"",
	)
	if err != nil {
		t.Fatalf("first discovery batch: %v", err)
	}
	if len(first) != piSessionDiscoveryBatchSize || !hasMore || cursor == "" {
		t.Fatalf("first batch count=%d cursor=%q hasMore=%v", len(first), cursor, hasMore)
	}
	second, secondCursor, secondHasMore, err := listPiSessionCandidateBatch(
		context.Background(),
		root,
		canonicalAIProjectPath(project),
		cursor,
	)
	if err != nil {
		t.Fatalf("second discovery batch: %v", err)
	}
	if len(second) != 44 || secondHasMore || secondCursor != "" {
		t.Fatalf("second batch count=%d cursor=%q hasMore=%v", len(second), secondCursor, secondHasMore)
	}
}

func TestProjectAISessionsIndexesRecentAndOlderPiHistory(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()
	resetPiHistoryTestCaches()

	home := t.TempDir()
	root := filepath.Join(home, "pi-sessions")
	project := filepath.Join(home, "project")
	otherProject := filepath.Join(home, "other")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatalf("create project: %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("PI_CODING_AGENT_SESSION_DIR", root)

	now := time.Now().UTC()
	writeServicePiSession(t, root, "recent-dir", "recent-session", project, now.Add(-time.Hour), true)
	oldPath := writeServicePiSession(t, root, "old-dir", "old-session", project, now.Add(-30*24*time.Hour), false)
	oldTime := now.Add(-30 * 24 * time.Hour)
	if err := os.Chtimes(oldPath, oldTime, oldTime); err != nil {
		t.Fatalf("age old fixture: %v", err)
	}
	writeServicePiSession(t, root, "misleading-project-dir", "other-session", otherProject, now, false)
	if err := os.WriteFile(filepath.Join(root, "broken.jsonl"), []byte("not-json\n"), 0o644); err != nil {
		t.Fatalf("write broken fixture: %v", err)
	}

	svc := NewAISessionService()
	var result *ProjectAISessions
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		var err error
		result, err = svc.GetProjectAISessions(context.Background(), project)
		if err != nil {
			t.Fatalf("GetProjectAISessions: %v", err)
		}
		if result.PiScanPhase == ScanPhaseComplete && len(result.PiSessions) == 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if result == nil || !result.HasPi || len(result.PiSessions) != 2 {
		t.Fatalf("unexpected Pi history result: %#v", result)
	}
	ids := map[string]bool{}
	for _, session := range result.PiSessions {
		ids[session.SessionID] = true
		if session.Type != string(tables.AISessionTypePi) {
			t.Fatalf("session type = %q, want pi", session.Type)
		}
	}
	if !ids["recent-session"] || !ids["old-session"] || ids["other-session"] {
		t.Fatalf("unexpected indexed session IDs: %#v", ids)
	}

	var count int64
	if err := model.GetDB().Model(&tables.AISessionTable{}).
		Where("type = ?", tables.AISessionTypePi).Count(&count).Error; err != nil || count != 2 {
		t.Fatalf("Pi cache count = %d, err=%v", count, err)
	}
	conversation, err := svc.GetSessionConversationBySessionID(context.Background(), "recent-session")
	if err != nil || len(conversation.Messages) != 4 {
		t.Fatalf("Pi conversation by session ID = %#v, %v", conversation, err)
	}
}
