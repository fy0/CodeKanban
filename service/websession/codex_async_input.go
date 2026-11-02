package websession

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"code-kanban/utils"
)

const codexAsyncInputDetailType = "async_user_input_request"

// Async questions are delivered as agent messages, not server requests. Their
// replies enter the thread as ordinary user messages (steering an active turn).
func codexAsyncInputDetail(item map[string]any) *HistoryDetail {
	if !strings.EqualFold(stringValue(item["delivery"]), "async") {
		return nil
	}
	var questions []struct {
		Title   string   `json:"title"`
		Options []string `json:"options"`
	}
	raw, err := json.Marshal(item["questions"])
	if err != nil || json.Unmarshal(raw, &questions) != nil || len(questions) == 0 {
		return nil
	}
	detail := &HistoryDetail{Type: codexAsyncInputDetailType}
	for index, question := range questions {
		if strings.TrimSpace(question.Title) == "" {
			return nil
		}
		next := toolRequestQuestion{ID: strconv.Itoa(index), Question: question.Title, IsOther: true}
		for _, option := range question.Options {
			if strings.TrimSpace(option) != "" {
				next.Options = append(next.Options, toolRequestOption{Label: option})
			}
		}
		detail.Questions = append(detail.Questions, next)
	}
	return detail
}

func codexAsyncInputHistoryItem(item map[string]any, ts time.Time) HistoryItem {
	detail := codexAsyncInputDetail(item)
	text := stringValue(item["text"])
	if text == "" && detail != nil {
		var parts []string
		for _, question := range detail.Questions {
			lines := []string{question.Question}
			for _, option := range question.Options {
				lines = append(lines, "- "+option.Label)
			}
			parts = append(parts, strings.Join(lines, "\n"))
		}
		text = strings.Join(parts, "\n\n")
	}
	id := firstNonEmpty(stringValue(item["id"]), utils.NewID())
	return HistoryItem{
		ID: id, SourceItemID: ptr("assistant:" + id), Kind: "assistant", ItemType: "agent_message",
		Text: text, Timestamp: ptr(ts), ObservedAt: ptr(ts), Done: true, Detail: detail, Payload: cloneMap(item),
	}
}

func formatCodexAsyncInputAnswers(questions []toolRequestQuestion, answers map[string][]string) (string, map[string][]string, error) {
	parts := []string{"Answers to your questions:"}
	normalized := make(map[string][]string, len(questions))
	for _, question := range questions {
		values := answers[question.ID]
		if len(values) != 1 || strings.TrimSpace(values[0]) == "" {
			return "", nil, fmt.Errorf("an answer is required for each question")
		}
		value := strings.TrimSpace(values[0])
		normalized[question.ID] = []string{value}
		parts = append(parts, question.Question+"\n"+value)
	}
	return strings.Join(parts, "\n\n"), normalized, nil
}

func (m *Manager) handleCodexAsyncInputCommand(ctx context.Context, client *client, frame wireCommandFrame) error {
	var payload struct {
		ItemID  string              `json:"iid"`
		Answers map[string][]string `json:"ans"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", "invalid async input payload", false))
	}
	if err := m.respondToCodexAsyncInput(ctx, frame.SessionID, payload.ItemID, payload.Answers); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "invalid_state", err.Error(), false))
	}
	return m.sendMutationAck(ctx, client, frame, nil)
}

func (m *Manager) respondToCodexAsyncInput(ctx context.Context, sessionID, itemID string, answers map[string][]string) error {
	// Keep duplicate submissions serialized without holding the dispatch lock:
	// the regular send/steer path takes that lock itself.
	lock := &m.asyncUserInputLocks[sessionRevisionLockIndex(sessionID)]
	lock.Lock()
	defer lock.Unlock()
	record, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if record.ArchivedAt != nil || !isCodexSteerSession(record) {
		return fmt.Errorf("async questions require an active Codex session")
	}
	if err := m.ensureSessionMessagingAuthorized(ctx, record); err != nil {
		return err
	}
	item, err := m.findHistoryItemByID(ctx, sessionID, strings.TrimSpace(itemID))
	if err != nil {
		return err
	}
	if item.Detail == nil || item.Detail.Type != codexAsyncInputDetailType || len(item.Detail.Questions) == 0 {
		return fmt.Errorf("async question not found")
	}
	if item.SourceThreadID != nil && record.NativeSessionID != nil && *item.SourceThreadID != *record.NativeSessionID {
		return fmt.Errorf("cannot answer a sub-agent question from the parent thread")
	}
	if item.Detail.Action == "submitted" {
		return nil
	}
	text, normalized, err := formatCodexAsyncInputAnswers(item.Detail.Questions, answers)
	if err != nil {
		return err
	}
	if _, err := m.sendMessageWithModeResult(ctx, sessionID, text, nil, PendingInputModeRedirect, "async-answer:"+item.ID); err != nil {
		return err
	}
	threadID := ""
	if item.SourceThreadID != nil {
		threadID = *item.SourceThreadID
	}
	_, err = m.appendAndBroadcast(ctx, sessionID, record, Event{
		ID: utils.NewID(), Type: "async_user_input_res", ThreadID: threadID, Timestamp: time.Now(),
		Payload: map[string]any{"iid": item.ID, "ans": normalized},
	})
	return err
}
