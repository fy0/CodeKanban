package terminal

import (
	"testing"
	"time"

	"code-kanban/utils/ai_assistant2"
)

func TestRecordManager_AddAndGetCompletions(t *testing.T) {
	rm := NewRecordManager()

	record := &CompletionRecord{
		ID:            "rec1",
		SessionID:     "sess1",
		ProjectID:     "proj1",
		Title:         "Test Session",
		LastUserInput: "help me fix the bug",
		State:         "completed",
		CompletedAt:   time.Now(),
	}

	rm.AddCompletion(record)

	completions := rm.GetCompletions()
	if len(completions) != 1 {
		t.Fatalf("expected 1 completion, got %d", len(completions))
	}
	if completions[0].LastUserInput != "help me fix the bug" {
		t.Fatalf("expected LastUserInput 'help me fix the bug', got %q", completions[0].LastUserInput)
	}
}

func TestRecordManager_UpdateCompletionBySession(t *testing.T) {
	rm := NewRecordManager()

	record := &CompletionRecord{
		ID:            "rec1",
		SessionID:     "sess1",
		ProjectID:     "proj1",
		Title:         "Test Session",
		LastUserInput: "initial input",
		State:         "completed",
		CompletedAt:   time.Now(),
	}

	rm.AddCompletion(record)

	// 更新状态和用户输入
	updated := rm.UpdateCompletionBySession("sess1", "working", "new user input")
	if !updated {
		t.Fatal("expected UpdateCompletionBySession to return true")
	}

	completions := rm.GetCompletions()
	if len(completions) != 1 {
		t.Fatalf("expected 1 completion, got %d", len(completions))
	}
	if completions[0].State != "working" {
		t.Fatalf("expected state 'working', got %q", completions[0].State)
	}
	if completions[0].LastUserInput != "new user input" {
		t.Fatalf("expected LastUserInput 'new user input', got %q", completions[0].LastUserInput)
	}
}

func TestRecordManager_UpdateCompletionBySession_EmptyInput(t *testing.T) {
	rm := NewRecordManager()

	record := &CompletionRecord{
		ID:            "rec1",
		SessionID:     "sess1",
		ProjectID:     "proj1",
		Title:         "Test Session",
		LastUserInput: "original input",
		State:         "completed",
		CompletedAt:   time.Now(),
	}

	rm.AddCompletion(record)

	// 空输入不应该覆盖原有的 LastUserInput
	updated := rm.UpdateCompletionBySession("sess1", "working", "")
	if !updated {
		t.Fatal("expected UpdateCompletionBySession to return true")
	}

	completions := rm.GetCompletions()
	if completions[0].LastUserInput != "original input" {
		t.Fatalf("expected LastUserInput to remain 'original input', got %q", completions[0].LastUserInput)
	}
}

func TestRecordManager_UpdateCompletionBySession_NotFound(t *testing.T) {
	rm := NewRecordManager()

	updated := rm.UpdateCompletionBySession("nonexistent", "working", "input")
	if updated {
		t.Fatal("expected UpdateCompletionBySession to return false for nonexistent session")
	}
}

func TestRecordManager_DismissCompletion(t *testing.T) {
	rm := NewRecordManager()

	record := &CompletionRecord{
		ID:            "rec1",
		SessionID:     "sess1",
		ProjectID:     "proj1",
		Title:         "Test Session",
		LastUserInput: "test input",
		State:         "completed",
		CompletedAt:   time.Now(),
	}

	rm.AddCompletion(record)

	// Dismiss 后不应该出现在 GetCompletions 结果中
	rm.DismissCompletion("rec1")

	completions := rm.GetCompletions()
	if len(completions) != 0 {
		t.Fatalf("expected 0 completions after dismiss, got %d", len(completions))
	}
}

func TestRecordManager_ClearSessionRecords(t *testing.T) {
	rm := NewRecordManager()

	rm.AddCompletion(&CompletionRecord{
		ID:        "rec1",
		SessionID: "sess1",
		ProjectID: "proj1",
	})
	rm.AddCompletion(&CompletionRecord{
		ID:        "rec2",
		SessionID: "sess1",
		ProjectID: "proj1",
	})
	rm.AddCompletion(&CompletionRecord{
		ID:        "rec3",
		SessionID: "sess2",
		ProjectID: "proj1",
	})

	rm.ClearSessionRecords("sess1")

	completions := rm.GetCompletions()
	if len(completions) != 1 {
		t.Fatalf("expected 1 completion after clearing sess1, got %d", len(completions))
	}
	if completions[0].SessionID != "sess2" {
		t.Fatalf("expected remaining completion to be from sess2")
	}
}

func TestRecordManager_ApprovalRecords(t *testing.T) {
	rm := NewRecordManager()

	record := &ApprovalRecord{
		ID:          "apr1",
		SessionID:   "sess1",
		ProjectID:   "proj1",
		Title:       "Test Session",
		RequestedAt: time.Now(),
	}

	rm.AddApproval(record)

	approvals := rm.GetApprovals()
	if len(approvals) != 1 {
		t.Fatalf("expected 1 approval, got %d", len(approvals))
	}

	rm.DismissApproval("apr1")
	approvals = rm.GetApprovals()
	if len(approvals) != 0 {
		t.Fatalf("expected 0 approvals after dismiss, got %d", len(approvals))
	}
}

func TestRecordManager_ClearCompletionsBySession(t *testing.T) {
	rm := NewRecordManager()

	rm.AddCompletion(&CompletionRecord{
		ID:            "rec1",
		SessionID:     "sess1",
		LastUserInput: "input1",
	})

	// 清除后再添加新记录
	rm.ClearCompletionsBySession("sess1")
	rm.AddCompletion(&CompletionRecord{
		ID:            "rec2",
		SessionID:     "sess1",
		LastUserInput: "input2",
	})

	completions := rm.GetCompletions()
	if len(completions) != 1 {
		t.Fatalf("expected 1 completion, got %d", len(completions))
	}
	if completions[0].ID != "rec2" {
		t.Fatalf("expected new record rec2, got %s", completions[0].ID)
	}
	if completions[0].LastUserInput != "input2" {
		t.Fatalf("expected LastUserInput 'input2', got %q", completions[0].LastUserInput)
	}
}

func TestCompletionRecord_WithAssistantInfo(t *testing.T) {
	rm := NewRecordManager()

	assistant := &ai_assistant2.AIAssistantInfo{
		Name:        "claude",
		DisplayName: "Claude",
		Type:        "claude-code",
		State:       "working",
	}

	record := &CompletionRecord{
		ID:            "rec1",
		SessionID:     "sess1",
		ProjectID:     "proj1",
		Title:         "Test Session",
		Assistant:     assistant,
		LastUserInput: "write a function",
		State:         "working",
		CompletedAt:   time.Now(),
	}

	rm.AddCompletion(record)

	completions := rm.GetCompletions()
	if len(completions) != 1 {
		t.Fatalf("expected 1 completion, got %d", len(completions))
	}
	if completions[0].Assistant == nil {
		t.Fatal("expected Assistant to be set")
	}
	if completions[0].Assistant.DisplayName != "Claude" {
		t.Fatalf("expected Assistant.DisplayName 'Claude', got %q", completions[0].Assistant.DisplayName)
	}
}
