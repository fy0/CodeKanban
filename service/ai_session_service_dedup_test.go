package service

import (
	"testing"
	"time"
)

func TestDedupeAISessionSummariesBySessionID(t *testing.T) {
	now := time.Now()
	earlier := now.Add(-1 * time.Hour)

	older := &AISessionSummary{
		ID:               "1",
		SessionID:        "sess-1",
		SessionStartedAt: earlier,
		LastMessageAt:    &earlier,
		MessageCount:     1,
	}
	newer := &AISessionSummary{
		ID:               "1",
		SessionID:        "sess-1",
		SessionStartedAt: earlier,
		LastMessageAt:    &now,
		MessageCount:     2,
	}
	other := &AISessionSummary{
		ID:               "2",
		SessionID:        "sess-2",
		SessionStartedAt: earlier,
		MessageCount:     3,
	}

	out := dedupeAISessionSummariesBySessionID([]*AISessionSummary{older, newer, other})
	if len(out) != 2 {
		t.Fatalf("expected 2 sessions after dedupe, got %d", len(out))
	}

	var gotSess1 *AISessionSummary
	for _, session := range out {
		if session.SessionID == "sess-1" {
			gotSess1 = session
			break
		}
	}
	if gotSess1 == nil {
		t.Fatalf("expected sess-1 to exist after dedupe")
	}
	if gotSess1.MessageCount != 2 {
		t.Fatalf("expected sess-1 messageCount=2, got %d", gotSess1.MessageCount)
	}
	if gotSess1.LastMessageAt == nil || !gotSess1.LastMessageAt.Equal(now) {
		t.Fatalf("expected sess-1 lastMessageAt=%v, got %v", now, gotSess1.LastMessageAt)
	}
}
