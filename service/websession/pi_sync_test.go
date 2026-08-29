package websession

import (
	"testing"
	"time"

	"code-kanban/model/tables"
)

func TestPiHistoryRowsEqualIgnoresSourceIdentityAdoption(t *testing.T) {
	timestamp := time.UnixMilli(1_000)
	left := tables.WebSessionItemTable{
		OrderIndex:   1,
		LastEventSeq: 7,
		ItemKind:     "assistant",
		ItemType:     "agent_message",
		Text:         "same timeline content",
		Done:         true,
		Timestamp:    &timestamp,
		ObservedAt:   &timestamp,
	}
	right := left
	right.WebTurnID = nilIfEmptyHistory("turn-row")
	right.SourceThreadID = nilIfEmptyHistory("native-session")
	right.SourceTurnID = nilIfEmptyHistory("native-turn")
	right.SourceItemID = nilIfEmptyHistory("native-item")
	right.Role = "assistant"
	right.Status = "completed"

	if !piHistoryRowsEqual(left, right) {
		t.Fatal("source identity adoption should not invalidate an unchanged timeline")
	}
	right.Text = "changed timeline content"
	if piHistoryRowsEqual(left, right) {
		t.Fatal("timeline content changes must advance the history epoch")
	}
}
