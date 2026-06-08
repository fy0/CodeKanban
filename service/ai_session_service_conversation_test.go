package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"code-kanban/model"
	"code-kanban/model/tables"

	"gorm.io/gorm"
)

func TestParseCodexConversationPreservesImageOnlyMessages(t *testing.T) {
	filePath := writeConversationTempFile(t, `{"timestamp":"2026-01-01T00:00:00Z","type":"event_msg","payload":{"type":"user_message","message":"","images":["C:/temp/example.png"]}}`)

	svc := NewAISessionService()
	messages, err := svc.parseCodexConversation(filePath)
	if err != nil {
		t.Fatalf("parseCodexConversation returned error: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	if messages[0].Role != "user" {
		t.Fatalf("expected user role, got %q", messages[0].Role)
	}
	if len(messages[0].Images) != 1 {
		t.Fatalf("expected 1 image attachment, got %d", len(messages[0].Images))
	}
	if messages[0].Images[0].Label != "example.png" {
		t.Fatalf("expected image label example.png, got %q", messages[0].Images[0].Label)
	}
}

func TestParseCodexConversationUsesLocalImagesAndPlaceholders(t *testing.T) {
	imagePath := filepath.Join(t.TempDir(), "codex-clipboard-demo.png")
	if err := os.WriteFile(imagePath, []byte("png-data"), 0o644); err != nil {
		t.Fatalf("failed to write temp image: %v", err)
	}
	filePath := writeConversationTempFile(t, `{"timestamp":"2026-01-01T00:00:00Z","type":"event_msg","payload":{"type":"user_message","message":"[Image #1]","images":[],"local_images":["`+filepath.ToSlash(imagePath)+`"],"text_elements":[{"placeholder":"[Image #1]"}]}}`)

	svc := NewAISessionService()
	messages, err := svc.parseCodexConversation(filePath)
	if err != nil {
		t.Fatalf("parseCodexConversation returned error: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	if len(messages[0].Images) != 1 {
		t.Fatalf("expected 1 image attachment, got %d", len(messages[0].Images))
	}
	if messages[0].Images[0].Label != "[Image #1]" {
		t.Fatalf("expected placeholder label [Image #1], got %q", messages[0].Images[0].Label)
	}
	if !messages[0].Images[0].Previewable {
		t.Fatal("expected local image attachment to be previewable")
	}
}

func TestParseClaudeConversationExtractsImageAttachments(t *testing.T) {
	filePath := writeConversationTempFile(t, `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"Look at this"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"aGVsbG8="}}]},"timestamp":"2026-01-01T00:00:00Z","isMeta":false}`)

	svc := NewAISessionService()
	messages, err := svc.parseClaudeCodeConversation(filePath)
	if err != nil {
		t.Fatalf("parseClaudeCodeConversation returned error: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	if messages[0].Content != "Look at this" {
		t.Fatalf("expected text content to be preserved, got %q", messages[0].Content)
	}
	if len(messages[0].Images) != 1 {
		t.Fatalf("expected 1 image attachment, got %d", len(messages[0].Images))
	}
	if !messages[0].Images[0].Previewable {
		t.Fatal("expected embedded image to be previewable")
	}
	if messages[0].Images[0].MimeType != "image/png" {
		t.Fatalf("expected image/png mime type, got %q", messages[0].Images[0].MimeType)
	}
}

func TestDecorateConversationMessagesAssignsPreviewURLs(t *testing.T) {
	messages := []*ConversationMessage{
		{
			Role:    "user",
			Content: "hello",
			Images: []ConversationImageAttachment{{
				Label:       "demo.png",
				Previewable: true,
				MimeType:    "image/png",
				Source:      `C:\temp\demo.png`,
			}},
		},
	}

	decorateConversationMessages(messages, "session-123")
	attachment := messages[0].Images[0]
	if attachment.ID != "m0-i0" {
		t.Fatalf("expected attachment id m0-i0, got %q", attachment.ID)
	}
	if attachment.PreviewURL != "/api/v1/ai-sessions/by-session-id/session-123/conversation/images/m0-i0" {
		t.Fatalf("unexpected preview URL: %q", attachment.PreviewURL)
	}
}

func TestResolveConversationImagePreviewSupportsDataURIAndFile(t *testing.T) {
	t.Run("data-uri", func(t *testing.T) {
		preview, err := resolveConversationImagePreview(ConversationImageAttachment{
			Previewable: true,
			Source:      "data:image/png;base64,aGVsbG8=",
		})
		if err != nil {
			t.Fatalf("resolveConversationImagePreview returned error: %v", err)
		}
		if preview.MimeType != "image/png" {
			t.Fatalf("expected image/png mime type, got %q", preview.MimeType)
		}
		if string(preview.Data) != "hello" {
			t.Fatalf("expected decoded data hello, got %q", string(preview.Data))
		}
	})

	t.Run("local-file", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "sample.png")
		if err := os.WriteFile(filePath, []byte("png-data"), 0o644); err != nil {
			t.Fatalf("failed to write temp image: %v", err)
		}

		preview, err := resolveConversationImagePreview(ConversationImageAttachment{
			Previewable: true,
			Source:      filePath,
		})
		if err != nil {
			t.Fatalf("resolveConversationImagePreview returned error: %v", err)
		}
		if preview.FilePath != filePath {
			t.Fatalf("expected preview file path %q, got %q", filePath, preview.FilePath)
		}
		if preview.MimeType != "image/png" {
			t.Fatalf("expected image/png mime type, got %q", preview.MimeType)
		}
	})
}

func TestBuildConversationImageAttachmentMarksRemoteURLsAsUnpreviewable(t *testing.T) {
	attachment, ok := buildConversationImageAttachment("https://example.com/demo.png", "", 0, false)
	if !ok {
		t.Fatal("expected remote image attachment to be created")
	}
	if attachment.Previewable {
		t.Fatal("expected remote image attachment to be unpreviewable")
	}
	if attachment.Label != "demo.png" {
		t.Fatalf("expected demo.png label, got %q", attachment.Label)
	}
}

func TestGetSessionConversationBySessionIDResolvesMissingCodexCache(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	projectPath := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(projectPath, 0o755); err != nil {
		t.Fatalf("failed to create project dir: %v", err)
	}

	sessionID := "019d792d-5fe5-74b2-bc43-8770241cbea4"
	filePath := writeCodexRolloutFile(t, homeDir, sessionID, projectPath)

	svc := NewAISessionService()
	conversation, err := svc.GetSessionConversationBySessionID(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetSessionConversationBySessionID returned error: %v", err)
	}
	if conversation.SessionID != sessionID {
		t.Fatalf("expected session id %q, got %q", sessionID, conversation.SessionID)
	}
	if conversation.Title != "hello from preview" {
		t.Fatalf("expected title to be restored from rollout, got %q", conversation.Title)
	}
	if len(conversation.Messages) != 2 {
		t.Fatalf("expected 2 conversation messages, got %d", len(conversation.Messages))
	}
	if conversation.Messages[0].Role != "user" || conversation.Messages[0].Content != "hello from preview" {
		t.Fatalf("unexpected first message: %#v", conversation.Messages[0])
	}
	if conversation.Messages[1].Role != "assistant" || conversation.Messages[1].Content != "preview works" {
		t.Fatalf("unexpected second message: %#v", conversation.Messages[1])
	}

	var cached tables.AISessionTable
	if err := model.GetDB().
		Where("session_id = ? AND type = ?", sessionID, tables.AISessionTypeCodex).
		First(&cached).Error; err != nil {
		t.Fatalf("expected codex session to be cached after preview, got error: %v", err)
	}
	if cached.FilePath != filePath {
		t.Fatalf("expected cached file path %q, got %q", filePath, cached.FilePath)
	}
	if cached.ProjectPath != projectPath {
		t.Fatalf("expected cached project path %q, got %q", projectPath, cached.ProjectPath)
	}
}

func TestGetSessionConversationBySessionIDUsesExistingCodexCache(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	projectPath := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(projectPath, 0o755); err != nil {
		t.Fatalf("failed to create project dir: %v", err)
	}

	sessionID := "019d792d-5fe5-74b2-bc43-8770241cbea5"
	filePath := writeCodexRolloutFile(t, homeDir, sessionID, projectPath)
	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("failed to stat rollout file: %v", err)
	}

	startedAt := time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)
	lastMessageAt := startedAt.Add(3 * time.Second)
	cached := &tables.AISessionTable{
		SessionID:             sessionID,
		Type:                  tables.AISessionTypeCodex,
		ProjectPath:           projectPath,
		FilePath:              filePath,
		Model:                 "gpt-5.4",
		Title:                 "hello from preview",
		SessionStartedAt:      startedAt,
		LastMessageAt:         &lastMessageAt,
		MessageCount:          0,
		AssistantMessageCount: 0,
		FileModTime:           info.ModTime(),
		FileSize:              info.Size(),
	}
	cached.Init()
	if err := model.GetDB().Create(cached).Error; err != nil {
		t.Fatalf("failed to seed cached codex session: %v", err)
	}

	svc := NewAISessionService()
	conversation, err := svc.GetSessionConversationBySessionID(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetSessionConversationBySessionID returned error: %v", err)
	}
	if len(conversation.Messages) != 2 {
		t.Fatalf("expected 2 conversation messages, got %d", len(conversation.Messages))
	}

	var refreshed tables.AISessionTable
	if err := model.GetDB().First(&refreshed, "id = ?", cached.ID).Error; err != nil {
		t.Fatalf("failed to reload cached codex session: %v", err)
	}
	if refreshed.MessageCount != 1 {
		t.Fatalf("expected message count to refresh to 1, got %d", refreshed.MessageCount)
	}
	if refreshed.AssistantMessageCount != 1 {
		t.Fatalf("expected assistant message count to refresh to 1, got %d", refreshed.AssistantMessageCount)
	}
}

func TestBuildConversationWindowFromMessages(t *testing.T) {
	svc := NewAISessionService()
	messages := []*ConversationMessage{
		{Role: "user", Content: "u1", Timestamp: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		{Role: "assistant", Content: "a1", Timestamp: time.Date(2026, 1, 1, 0, 0, 1, 0, time.UTC)},
		{Role: "user", Content: "u2", Timestamp: time.Date(2026, 1, 1, 0, 0, 2, 0, time.UTC)},
		{Role: "assistant", Content: "a2", Timestamp: time.Date(2026, 1, 1, 0, 0, 3, 0, time.UTC)},
		{Role: "user", Content: "u3", Timestamp: time.Date(2026, 1, 1, 0, 0, 4, 0, time.UTC)},
	}

	window := svc.buildConversationWindowFromMessages("session-1", "", messages, conversationWindowOptions{
		beforeCursor: -1,
		limit:        2,
	})
	if window.Total != 5 {
		t.Fatalf("expected total 5, got %d", window.Total)
	}
	if window.WindowStart != 3 || window.WindowEnd != 5 {
		t.Fatalf("unexpected window bounds: %d-%d", window.WindowStart, window.WindowEnd)
	}
	if !window.HasMoreBefore || window.BeforeCursor != "3" {
		t.Fatalf("unexpected cursor state: %#v", window)
	}
	if window.TotalUserMessages != 3 || window.UserMessagesBeforeWindow != 2 {
		t.Fatalf("unexpected user message counts: %#v", window)
	}
	if len(window.Messages) != 2 || window.Messages[0].Content != "a2" || window.Messages[1].Content != "u3" {
		t.Fatalf("unexpected messages: %#v", window.Messages)
	}

	earlier := svc.buildConversationWindowFromMessages("session-1", "", messages, conversationWindowOptions{
		beforeCursor: 3,
		limit:        2,
	})
	if earlier.WindowStart != 1 || earlier.WindowEnd != 3 {
		t.Fatalf("unexpected earlier bounds: %d-%d", earlier.WindowStart, earlier.WindowEnd)
	}
	if earlier.UserMessagesBeforeWindow != 1 {
		t.Fatalf("expected one user message before earlier window, got %d", earlier.UserMessagesBeforeWindow)
	}
}

func TestNormalizeConversationWindowOptions(t *testing.T) {
	options, err := normalizeConversationWindowOptions(" 3 ", 200)
	if err != nil {
		t.Fatalf("normalizeConversationWindowOptions returned error: %v", err)
	}
	if options.beforeCursor != 3 {
		t.Fatalf("expected beforeCursor 3, got %d", options.beforeCursor)
	}
	if options.limit != 120 {
		t.Fatalf("expected clamped limit 120, got %d", options.limit)
	}

	if _, err := normalizeConversationWindowOptions("-1", 10); err == nil {
		t.Fatal("expected invalid negative cursor to fail")
	}
	if _, err := normalizeConversationWindowOptions("abc", 10); err == nil {
		t.Fatal("expected invalid cursor string to fail")
	}
}

func TestGetSessionConversationBySessionIDReturnsNotFoundWithoutRollout(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	t.Setenv("HOME", t.TempDir())

	svc := NewAISessionService()
	_, err := svc.GetSessionConversationBySessionID(
		context.Background(),
		"019d792d-5fe5-74b2-bc43-8770241cbea6",
	)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected record not found error, got %v", err)
	}
}

func writeConversationTempFile(t *testing.T, content string) string {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "conversation-*.jsonl")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	if _, err := file.WriteString(content + "\n"); err != nil {
		_ = file.Close()
		t.Fatalf("failed to write temp file: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("failed to close temp file: %v", err)
	}
	return file.Name()
}

func writeCodexRolloutFile(t *testing.T, homeDir string, sessionID string, projectPath string) string {
	t.Helper()

	dir := filepath.Join(homeDir, ".codex", "sessions", "2026", "04", "15")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("failed to create rollout dir: %v", err)
	}

	filePath := filepath.Join(dir, fmt.Sprintf("rollout-2026-04-15T12-00-00-%s.jsonl", sessionID))
	lines := []string{
		fmt.Sprintf(`{"timestamp":"2026-04-15T12:00:00Z","type":"session_meta","payload":{"cwd":%q,"timestamp":"2026-04-15T12:00:00Z"}}`, projectPath),
		`{"timestamp":"2026-04-15T12:00:01Z","type":"turn_context","payload":{"model":"gpt-5.4"}}`,
		`{"timestamp":"2026-04-15T12:00:02Z","type":"event_msg","payload":{"type":"user_message","message":"hello from preview"}}`,
		`{"timestamp":"2026-04-15T12:00:03Z","type":"event_msg","payload":{"type":"agent_message","message":"preview works"}}`,
	}
	if err := os.WriteFile(filePath, []byte(lines[0]+"\n"+lines[1]+"\n"+lines[2]+"\n"+lines[3]+"\n"), 0o644); err != nil {
		t.Fatalf("failed to write rollout file: %v", err)
	}
	return filePath
}
