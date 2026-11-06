package websession

import (
	"context"
	"reflect"
	"testing"

	"code-kanban/model"
	"code-kanban/model/tables"

	"go.uber.org/zap"
)

func TestManagerSearchSessionsChunkOnlyMatchesConversationMessages(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}

	const needle = "conversation-only-needle"
	tests := []struct {
		name     string
		kind     string
		itemType string
		text     string
		title    string
		preview  string
		want     []SessionSearchMatchSource
		wantAll  []SessionSearchMatchSource
	}{
		{name: "user message", kind: "user", itemType: "user_message", text: needle, want: []SessionSearchMatchSource{SessionSearchMatchBody}},
		{name: "assistant reply", kind: "assistant", itemType: "agent_message", text: needle, want: []SessionSearchMatchSource{SessionSearchMatchBody}},
		{name: "native assistant reply", kind: "assistant", itemType: "agentMessage", text: needle, want: []SessionSearchMatchSource{SessionSearchMatchBody}},
		{name: "tool call text", kind: "tool", itemType: "command_execution", text: needle, wantAll: []SessionSearchMatchSource{SessionSearchMatchBody}},
		{name: "reasoning text", kind: "tool", itemType: "reasoning", text: needle, wantAll: []SessionSearchMatchSource{SessionSearchMatchBody}},
		{name: "system message", kind: "system", itemType: "notification", text: needle, wantAll: []SessionSearchMatchSource{SessionSearchMatchBody}},
		{name: "unknown message type", kind: "other", text: needle, wantAll: []SessionSearchMatchSource{SessionSearchMatchBody}},
		{name: "tool parameters and output", kind: "tool", itemType: "command_execution"},
		{name: "reasoning output", kind: "tool", itemType: "reasoning"},
		{name: "assistant metadata", kind: "assistant", itemType: "agent_message", text: "ordinary reply"},
		{name: "title still matches", kind: "tool", text: needle, title: needle, want: []SessionSearchMatchSource{SessionSearchMatchTitle}, wantAll: []SessionSearchMatchSource{SessionSearchMatchTitle, SessionSearchMatchBody}},
		{name: "prompt preview still matches", kind: "tool", preview: needle, want: []SessionSearchMatchSource{SessionSearchMatchBody}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			project := seedProject(t)
			title := tt.title
			if title == "" {
				title = tt.name
			}
			session := seedWebSession(t, project.ID, title, 1000)
			if tt.preview != "" {
				if err := model.GetDB().Model(session).Update("thread_preview", tt.preview).Error; err != nil {
					t.Fatal(err)
				}
			}
			row := tables.WebSessionItemTable{
				WebSessionID: session.ID,
				OrderIndex:   1,
				ItemKind:     tt.kind,
				ItemType:     tt.itemType,
				Text:         tt.text,
				ToolJSON:     `{"input":"` + needle + `","output":"` + needle + `"}`,
				PayloadJSON:  `{"text":"` + needle + `"}`,
			}
			row.Init()
			if err := model.GetDB().Create(&row).Error; err != nil {
				t.Fatal(err)
			}
			checkSearch := func(messagesOnly bool, want []SessionSearchMatchSource) {
				t.Helper()
				result, err := manager.SearchSessionsChunk(context.Background(), []string{project.ID}, needle, false, true, messagesOnly, "", 50)
				if err != nil {
					t.Fatal(err)
				}
				if !result.Done || result.Scanned != 1 || result.Total != 1 {
					t.Fatalf("unexpected search progress: %+v", result)
				}
				if len(want) == 0 {
					if len(result.Items) != 0 {
						t.Fatalf("messagesOnly=%v: unexpected match: %+v", messagesOnly, result.Items)
					}
					return
				}
				if len(result.Items) != 1 || result.Items[0].ID != session.ID || !reflect.DeepEqual(result.Items[0].SearchMatchSources, want) {
					t.Fatalf("messagesOnly=%v: unexpected search matches: %+v, want sources %v", messagesOnly, result.Items, want)
				}
			}
			checkSearch(true, tt.want)
			wantAll := tt.wantAll
			if wantAll == nil {
				wantAll = tt.want
			}
			checkSearch(false, wantAll)
		})
	}
}
