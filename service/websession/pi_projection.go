package websession

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"code-kanban/model/tables"
	"code-kanban/utils"
)

const piToolProgressInterval = 100 * time.Millisecond

// piToolHistoryKind is the history kind stamped on every Pi tool call.
//
// Pi reports heterogeneous tools (bash, file edits, MCP and extension tools)
// so they are classified as dynamic tool calls: that makes them eligible for the
// shared compact-tool folding, keeps the tool name as the row label, and leaves
// the active-call timeout policy alone because dynamic_tool_call still resolves
// to the generic tool policy in activeCallTimeoutKindFromTool.
const piToolHistoryKind = "dynamic_tool_call"

type piRPCMessage struct {
	Usage        *piRPCTokenUsage           `json:"usage"`
	Role         string                     `json:"role"`
	Timestamp    int64                      `json:"timestamp"`
	StopReason   string                     `json:"stopReason"`
	ErrorMessage string                     `json:"errorMessage"`
	Content      []piRPCContentMessageBlock `json:"content"`
}

// Pi uses a string content value for system/user messages and an array of
// typed blocks for assistant messages. Keep the wire parser tolerant of both
// forms (and of a single block object from older/custom providers) so a
// non-assistant lifecycle event cannot abort an otherwise valid run.
type piRPCContentMessageBlock = struct {
	Type      string         `json:"type"`
	Text      string         `json:"text"`
	Thinking  string         `json:"thinking"`
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

func decodePiRPCMessageContent(data []byte) ([]piRPCContentMessageBlock, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, nil
	}
	if trimmed[0] == '"' {
		var text string
		if err := json.Unmarshal(trimmed, &text); err != nil {
			return nil, err
		}
		return []piRPCContentMessageBlock{{Type: "text", Text: text}}, nil
	}
	if trimmed[0] == '{' {
		var block piRPCContentMessageBlock
		if err := json.Unmarshal(trimmed, &block); err != nil {
			return nil, err
		}
		return []piRPCContentMessageBlock{block}, nil
	}
	var list []piRPCContentMessageBlock
	if err := json.Unmarshal(trimmed, &list); err != nil {
		return nil, err
	}
	return list, nil
}

func (message *piRPCMessage) UnmarshalJSON(data []byte) error {
	var wire struct {
		Usage        *piRPCTokenUsage `json:"usage"`
		Role         string           `json:"role"`
		Timestamp    int64            `json:"timestamp"`
		StopReason   string           `json:"stopReason"`
		ErrorMessage string           `json:"errorMessage"`
		Content      json.RawMessage  `json:"content"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	content, err := decodePiRPCMessageContent(wire.Content)
	if err != nil {
		return fmt.Errorf("decode Pi message content: %w", err)
	}
	message.Usage = wire.Usage
	message.Role = wire.Role
	message.Timestamp = wire.Timestamp
	message.StopReason = wire.StopReason
	message.ErrorMessage = wire.ErrorMessage
	message.Content = content
	return nil
}

type piAssistantMessageEvent struct {
	Type         string `json:"type"`
	ContentIndex int    `json:"contentIndex"`
	Delta        string `json:"delta"`
	Content      string `json:"content"`
	ToolCall     struct {
		Type      string         `json:"type"`
		ID        string         `json:"id"`
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	} `json:"toolCall"`
}

type piRPCMessageEvent struct {
	Type    string           `json:"type"`
	Usage   *piRPCTokenUsage `json:"usage"`
	Message piRPCMessage     `json:"message"`
}

type piRPCToolExecutionEvent struct {
	Type          string         `json:"type"`
	ToolCallID    string         `json:"toolCallId"`
	ToolName      string         `json:"toolName"`
	Args          map[string]any `json:"args"`
	PartialResult any            `json:"partialResult"`
	Result        any            `json:"result"`
	IsError       bool           `json:"isError"`
}

func (m *Manager) handlePiRuntimeEvent(dispatch *piRuntimeRun, event piRPCEvent) error {
	if dispatch == nil || dispatch.run == nil {
		return nil
	}
	switch event.Type {
	case "agent_start":
		now := time.Now()
		dispatch.mu.Lock()
		dispatch.hasLatestUsage = false
		dispatch.mu.Unlock()
		return m.updateRuntimeState(context.Background(), dispatch.session.ID, applyAssistantStateUpdates(map[string]any{
			"status": string(StatusRunning), "updated_at": now,
			"latest_token_count_input_tokens":        0,
			"latest_token_count_cached_input_tokens": 0,
			"latest_token_count_output_tokens":       0,
			"latest_token_count_total_tokens":        0,
			"latest_token_count_updated_at":          nil,
			"latest_turn_input_tokens":               0,
			"latest_turn_cached_input_tokens":        0,
			"latest_turn_output_tokens":              0,
			"latest_turn_usage_updated_at":           nil,
		}, AssistantStateWorking, now))
	case "message_start":
		var payload piRPCMessageEvent
		if err := json.Unmarshal(event.Raw, &payload); err != nil {
			return fmt.Errorf("decode Pi message_start: %w", err)
		}
		if strings.EqualFold(payload.Message.Role, "assistant") {
			return m.startPiAssistantMessage(dispatch)
		}
	case "message_update":
		var payload struct {
			Usage                 *piRPCTokenUsage        `json:"usage"`
			AssistantMessageEvent piAssistantMessageEvent `json:"assistantMessageEvent"`
		}
		if err := json.Unmarshal(event.Raw, &payload); err != nil {
			return fmt.Errorf("decode Pi message_update: %w", err)
		}
		if strings.TrimSpace(payload.AssistantMessageEvent.Type) != "" {
			if err := m.handlePiAssistantMessageEvent(dispatch, payload.AssistantMessageEvent); err != nil {
				return err
			}
		}
		if payload.Usage != nil {
			if err := m.handlePiUsageUpdate(dispatch, *payload.Usage); err != nil {
				return err
			}
		}
		return nil
	case "message_end":
		var payload piRPCMessageEvent
		if err := json.Unmarshal(event.Raw, &payload); err != nil {
			return fmt.Errorf("decode Pi message_end: %w", err)
		}
		if strings.EqualFold(payload.Message.Role, "assistant") {
			if payload.Message.Usage == nil {
				payload.Message.Usage = payload.Usage
			}
			return m.finishPiAssistantMessage(dispatch, payload.Message)
		}
	case "tool_execution_start", "tool_execution_update", "tool_execution_end":
		return m.handlePiToolExecution(dispatch, event)
	case "compaction_start", "compaction_end":
		return m.handlePiCompactionEvent(dispatch, event)
	case "queue_update":
		return m.handlePiQueueUpdate(dispatch, event)
	case "auto_retry_start", "auto_retry_end", "summarization_retry_scheduled", "summarization_retry_attempt_start", "summarization_retry_finished":
		return m.handlePiRetryEvent(dispatch, event)
	case "extension_ui_request":
		return m.handlePiExtensionUIRequest(dispatch, event.Raw)
	case "extension_error":
		var payload struct {
			Error string `json:"error"`
			Path  string `json:"extensionPath"`
		}
		if err := json.Unmarshal(event.Raw, &payload); err != nil {
			return fmt.Errorf("decode Pi extension_error: %w", err)
		}
		text := "Pi extension failed"
		if strings.TrimSpace(payload.Path) != "" {
			text += ": " + strings.TrimSpace(payload.Path)
		}
		m.appendRunNote(dispatch.session.ID, dispatch.session, dispatch.run, "warning", text, map[string]any{"code": "pi_extension_error"})
	case "session_info_changed":
		return m.handlePiSessionInfoChanged(dispatch, event.Raw)
	case "thinking_level_changed":
		return m.handlePiThinkingLevelChanged(dispatch, event.Raw)
	case "agent_settled":
		return m.finishPiSettledProjection(dispatch)
	}
	return nil
}

func (m *Manager) handlePiUsageUpdate(dispatch *piRuntimeRun, usage piRPCTokenUsage) error {
	if dispatch == nil || dispatch.run == nil {
		return nil
	}
	input := maxInt64(0, usage.Input)
	output := maxInt64(0, usage.Output)
	cached := usage.cachedInputTokens()
	total := usage.totalTokens()
	if input == 0 && output == 0 && cached == 0 && total == 0 {
		return nil
	}
	cost := usage.costTotal()
	dispatch.mu.Lock()
	if dispatch.hasLatestUsage && piRPCUsageEqual(dispatch.latestUsage, usage) {
		dispatch.mu.Unlock()
		return nil
	}
	dispatch.latestUsage = usage
	dispatch.hasLatestUsage = true
	dispatch.mu.Unlock()

	now := time.Now()
	if err := m.updateRuntimeState(context.Background(), dispatch.session.ID, map[string]any{
		"latest_token_count_input_tokens":        input,
		"latest_token_count_cached_input_tokens": cached,
		"latest_token_count_output_tokens":       output,
		"latest_token_count_total_tokens":        total,
		"latest_token_count_updated_at":          now,
		"latest_turn_input_tokens":               input,
		"latest_turn_cached_input_tokens":        cached,
		"latest_turn_output_tokens":              output,
		"latest_turn_usage_updated_at":           now,
		"updated_at":                             now,
	}); err != nil {
		return err
	}
	payload := map[string]any{"in": input, "cin": cached, "out": output, "tot": total}
	if cost != 0 {
		payload["cost"] = cost
	}
	if _, err := m.appendAndBroadcast(context.Background(), dispatch.session.ID, dispatch.session, Event{
		ID: utils.NewID(), Type: "usage", RunID: dispatch.run.runID, Timestamp: now, Payload: payload,
	}); err != nil {
		return err
	}
	m.broadcastSessionSummary(context.Background(), dispatch.session.ID)
	return nil
}

func piRPCUsageEqual(left, right piRPCTokenUsage) bool {
	return maxInt64(0, left.Input) == maxInt64(0, right.Input) &&
		maxInt64(0, left.Output) == maxInt64(0, right.Output) &&
		left.cachedInputTokens() == right.cachedInputTokens() &&
		left.totalTokens() == right.totalTokens() &&
		left.costTotal() == right.costTotal()
}

func (m *Manager) handlePiSessionInfoChanged(dispatch *piRuntimeRun, raw json.RawMessage) error {
	var payload struct {
		Name *string `json:"name"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return fmt.Errorf("decode Pi session_info_changed: %w", err)
	}
	name := strings.TrimSpace(pointerString(payload.Name))
	updates := map[string]any{"updated_at": time.Now()}
	if name != "" {
		updates["title"] = name
		updates["title_auto"] = false
	}
	if err := m.updateRuntimeState(context.Background(), dispatch.session.ID, updates); err != nil {
		return err
	}
	m.broadcastSessionSummary(context.Background(), dispatch.session.ID)
	return nil
}

func (m *Manager) handlePiThinkingLevelChanged(dispatch *piRuntimeRun, raw json.RawMessage) error {
	var payload struct {
		Level string `json:"level"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return fmt.Errorf("decode Pi thinking_level_changed: %w", err)
	}
	effort := piThinkingLevelToReasoning(payload.Level)
	if effort == ReasoningEffortDefault {
		return nil
	}
	if err := m.updateRuntimeState(context.Background(), dispatch.session.ID, map[string]any{
		"reasoning_effort": string(effort), "updated_at": time.Now(),
	}); err != nil {
		return err
	}
	m.broadcastSessionSummary(context.Background(), dispatch.session.ID)
	return nil
}

func (m *Manager) startPiAssistantMessage(dispatch *piRuntimeRun) error {
	dispatch.mu.Lock()
	if dispatch.assistantMessageOpen {
		dispatch.mu.Unlock()
		return nil
	}
	messageID := utils.NewID()
	dispatch.assistantMessageID = messageID
	dispatch.assistantMessageOpen = true
	dispatch.contents = make(map[int]*piRuntimeContentState)
	dispatch.hasLatestUsage = false
	dispatch.mu.Unlock()
	m.updateTokenSpeed(dispatch.session.ID, dispatch.run.runID, func(s *tokenSpeedState, now time.Time) {
		s.begin(messageID, now, true)
	}, true)

	dispatch.run.mu.Lock()
	dispatch.run.assistantMessageID = messageID
	dispatch.run.mu.Unlock()
	_, err := m.appendAndBroadcast(context.Background(), dispatch.session.ID, dispatch.session, Event{
		ID: utils.NewID(), Type: "msg_a_st", RunID: dispatch.run.runID,
		ParentID: messageID, Timestamp: time.Now(), Payload: map[string]any{"mid": messageID},
	})
	return err
}

func (m *Manager) ensurePiAssistantMessage(dispatch *piRuntimeRun) error {
	dispatch.mu.Lock()
	open := dispatch.assistantMessageOpen
	dispatch.mu.Unlock()
	if open {
		return nil
	}
	if err := m.startPiAssistantMessage(dispatch); err != nil {
		return err
	}
	// Reconstructing a missing message_start cannot establish a full timing boundary.
	m.updateTokenSpeed(dispatch.session.ID, dispatch.run.runID, func(s *tokenSpeedState, _ time.Time) {
		if cur := s.current(); cur != nil {
			cur.completeBoundary = false
		}
	}, false)
	return nil
}

func (m *Manager) handlePiAssistantMessageEvent(dispatch *piRuntimeRun, update piAssistantMessageEvent) error {
	if err := m.ensurePiAssistantMessage(dispatch); err != nil {
		return err
	}
	now := time.Now()
	dispatch.mu.Lock()
	messageID := dispatch.assistantMessageID
	state := dispatch.contents[update.ContentIndex]
	if state == nil {
		state = &piRuntimeContentState{}
		dispatch.contents[update.ContentIndex] = state
	}
	emitThinking := false
	switch update.Type {
	case "text_start", "text_delta", "text_end":
		state.kind = "text"
		if update.Type == "text_delta" {
			state.text += update.Delta
		} else if update.Type == "text_end" {
			state.text = update.Content
		}
	case "thinking_start", "thinking_delta", "thinking_end":
		state.kind = "thinking"
		if update.Type == "thinking_delta" {
			state.text += update.Delta
		} else if update.Type == "thinking_end" {
			state.text = update.Content
		}
		emitThinking = update.Type != "thinking_delta" || state.lastEmit.IsZero() || now.Sub(state.lastEmit) >= piToolProgressInterval
		if emitThinking {
			state.lastEmit = now
		}
	case "toolcall_start", "toolcall_delta", "toolcall_end":
		state.kind = "toolCall"
		if update.Type == "toolcall_delta" {
			state.text += update.Delta
		} else if update.Type == "toolcall_end" {
			state.toolID = strings.TrimSpace(update.ToolCall.ID)
			if state.toolID != "" {
				tool := dispatch.tools[state.toolID]
				if tool == nil {
					tool = &piRuntimeToolState{id: state.toolID}
					dispatch.tools[state.toolID] = tool
				}
				tool.name = strings.TrimSpace(update.ToolCall.Name)
				tool.args = update.ToolCall.Arguments
				tool.parentID = messageID
			}
		}
	}
	content := state.text
	dispatch.mu.Unlock()
	if update.Type == "text_delta" || update.Type == "thinking_delta" || update.Type == "toolcall_delta" {
		m.updateTokenSpeed(dispatch.session.ID, dispatch.run.runID, func(s *tokenSpeedState, now time.Time) {
			s.delta(messageID, fmt.Sprintf("%d", update.ContentIndex), update.Delta, now)
		}, false)
	}

	switch update.Type {
	case "text_delta":
		if update.Delta == "" {
			return nil
		}
		dispatch.run.markAssistantDeltaSeen(messageID)
		return m.enqueueTextDelta(context.Background(), dispatch.session.ID, dispatch.session, Event{
			ID: utils.NewID(), Type: "txt_d", RunID: dispatch.run.runID, ParentID: messageID,
			Timestamp: now, Payload: map[string]any{"mid": messageID, "txt": update.Delta},
		})
	case "thinking_start", "thinking_delta":
		if !emitThinking {
			return nil
		}
		return m.emitPiThinking(dispatch, messageID, update.ContentIndex, content, false)
	case "thinking_end":
		return m.emitPiThinking(dispatch, messageID, update.ContentIndex, content, true)
	}
	return nil
}

func (m *Manager) emitPiThinking(dispatch *piRuntimeRun, messageID string, index int, text string, done bool) error {
	toolID := fmt.Sprintf("pi-thinking:%s:%s:%d", dispatch.run.runID, messageID, index)
	eventType := "tool_st"
	if done {
		eventType = "tool_end"
	}
	_, err := m.appendAndBroadcast(context.Background(), dispatch.session.ID, dispatch.session, Event{
		ID: utils.NewID(), Type: eventType, RunID: dispatch.run.runID, ParentID: messageID,
		Timestamp: time.Now(), Payload: map[string]any{
			"tid": toolID, "name": "Reasoning", "kind": "reasoning",
			"out": truncateToolOutput("reasoning", text), "ok": true,
		},
	})
	return err
}

func (m *Manager) finishPiAssistantMessage(dispatch *piRuntimeRun, message piRPCMessage) error {
	if err := m.ensurePiAssistantMessage(dispatch); err != nil {
		return err
	}
	if message.Usage != nil {
		if err := m.handlePiUsageUpdate(dispatch, *message.Usage); err != nil {
			return err
		}
	}
	dispatch.mu.Lock()
	messageID := dispatch.assistantMessageID
	dispatch.assistantMessageOpen = false
	dispatch.lastAttemptError = strings.TrimSpace(message.ErrorMessage)
	if dispatch.lastAttemptError == "" && (strings.EqualFold(message.StopReason, "error") || strings.EqualFold(message.StopReason, "aborted")) {
		dispatch.lastAttemptError = "Pi assistant attempt failed"
	}
	dispatch.mu.Unlock()

	m.updateTokenSpeed(dispatch.session.ID, dispatch.run.runID, func(s *tokenSpeedState, now time.Time) {
		var output *int64
		if message.Usage != nil {
			output = &message.Usage.Output
		}
		s.finish(messageID, output, now, message.ErrorMessage == "" && message.StopReason != "error" && message.StopReason != "aborted")
	}, true)
	for index, block := range message.Content {
		if block.Type != "thinking" {
			continue
		}
		if err := m.emitPiThinking(dispatch, messageID, index, block.Thinking, true); err != nil {
			return err
		}
	}
	text := piMessageText(message)
	_, err := m.appendAndBroadcast(context.Background(), dispatch.session.ID, dispatch.session, Event{
		ID: utils.NewID(), Type: "txt_end", RunID: dispatch.run.runID, ParentID: messageID,
		Timestamp: time.Now(), Payload: map[string]any{"mid": messageID, "txt": text},
	})
	if err == nil && dispatch.lastAttemptError == "" {
		dispatch.run.mu.Lock()
		dispatch.run.completedReply = true
		dispatch.run.mu.Unlock()
	}
	return err
}

func (m *Manager) handlePiToolExecution(dispatch *piRuntimeRun, event piRPCEvent) error {
	if event.Type == "tool_execution_start" {
		m.updateTokenSpeed(dispatch.session.ID, dispatch.run.runID, func(s *tokenSpeedState, now time.Time) { s.finish("", nil, now, true) }, true)
	}
	var payload piRPCToolExecutionEvent
	if err := json.Unmarshal(event.Raw, &payload); err != nil {
		return fmt.Errorf("decode Pi %s: %w", event.Type, err)
	}
	toolID := strings.TrimSpace(payload.ToolCallID)
	if toolID == "" {
		return errors.New("Pi tool event is missing toolCallId")
	}
	now := time.Now()
	dispatch.mu.Lock()
	tool := dispatch.tools[toolID]
	if tool == nil {
		tool = &piRuntimeToolState{id: toolID}
		dispatch.tools[toolID] = tool
	}
	if strings.TrimSpace(payload.ToolName) != "" {
		tool.name = strings.TrimSpace(payload.ToolName)
	}
	if payload.Args != nil {
		tool.args = payload.Args
	}
	if tool.parentID == "" {
		tool.parentID = dispatch.assistantMessageID
	}
	outputValue := payload.PartialResult
	if event.Type == "tool_execution_end" {
		outputValue = payload.Result
	}
	if outputValue != nil {
		tool.output = truncateToolOutput(tool.name, piToolResultText(outputValue))
	}
	if event.Type == "tool_execution_update" && !tool.lastEmit.IsZero() && now.Sub(tool.lastEmit) < piToolProgressInterval {
		dispatch.mu.Unlock()
		return nil
	}
	tool.lastEmit = now
	if event.Type == "tool_execution_end" {
		tool.completed = true
	}
	snapshot := *tool
	dispatch.mu.Unlock()

	eventType := "tool_st"
	if event.Type == "tool_execution_end" {
		eventType = "tool_end"
	}
	_, err := m.appendAndBroadcast(context.Background(), dispatch.session.ID, dispatch.session, Event{
		ID: utils.NewID(), Type: eventType, RunID: dispatch.run.runID, ParentID: snapshot.parentID,
		Timestamp: now, Payload: map[string]any{
			"tid": snapshot.id, "name": firstNonEmpty(snapshot.name, "Tool"), "kind": piToolHistoryKind,
			"in": snapshot.args, "out": snapshot.output, "ok": !payload.IsError,
		},
	})
	return err
}

func (m *Manager) handlePiCompactionEvent(dispatch *piRuntimeRun, event piRPCEvent) error {
	var payload map[string]any
	if err := json.Unmarshal(event.Raw, &payload); err != nil {
		return fmt.Errorf("decode Pi %s: %w", event.Type, err)
	}
	dispatch.mu.Lock()
	if dispatch.compactionToolID == "" {
		dispatch.compactionToolID = "pi-compaction:" + utils.NewID()
	}
	toolID := dispatch.compactionToolID
	parentID := dispatch.assistantMessageID
	if event.Type == "compaction_end" {
		dispatch.compactionToolID = ""
	}
	dispatch.mu.Unlock()

	reason := strings.TrimSpace(stringValue(payload["reason"]))
	output := "Pi is compacting the conversation context."
	eventType := "tool_st"
	ok := true
	if event.Type == "compaction_end" {
		eventType = "tool_end"
		ok = !boolValue(payload["aborted"]) && strings.TrimSpace(stringValue(payload["errorMessage"])) == ""
		output = firstNonEmpty(strings.TrimSpace(stringValue(decodeRawObject(payload["result"])["summary"])), "Context compacted")
		if ok {
			dispatch.mu.Lock()
			dispatch.compactionCompleted = time.Now()
			dispatch.mu.Unlock()
		} else {
			output = firstNonEmpty(strings.TrimSpace(stringValue(payload["errorMessage"])), "Pi context compaction did not complete")
		}
	}
	_, err := m.appendAndBroadcast(context.Background(), dispatch.session.ID, dispatch.session, Event{
		ID: utils.NewID(), Type: eventType, RunID: dispatch.run.runID, ParentID: parentID,
		Timestamp: time.Now(), Payload: map[string]any{
			"tid": toolID, "name": "ContextCompaction", "kind": "context_compaction",
			"in": map[string]any{"reason": reason}, "out": output, "ok": ok,
		},
	})
	return err
}

func (m *Manager) handlePiQueueUpdate(dispatch *piRuntimeRun, event piRPCEvent) error {
	var payload struct {
		Steering []string `json:"steering"`
		FollowUp []string `json:"followUp"`
	}
	if err := json.Unmarshal(event.Raw, &payload); err != nil {
		return fmt.Errorf("decode Pi queue_update: %w", err)
	}
	m.replacePiNativeQueuedInputs(dispatch.session.ID, payload.Steering, payload.FollowUp)
	return nil
}

func (m *Manager) handlePiRetryEvent(dispatch *piRuntimeRun, event piRPCEvent) error {
	if event.Type == "auto_retry_start" || event.Type == "summarization_retry_scheduled" {
		m.updateTokenSpeed(dispatch.session.ID, dispatch.run.runID, func(s *tokenSpeedState, now time.Time) { s.finish("", nil, now, false) }, true)
	}
	var payload map[string]any
	if err := json.Unmarshal(event.Raw, &payload); err != nil {
		return fmt.Errorf("decode Pi %s: %w", event.Type, err)
	}
	text := "Pi retry state changed"
	level := "info"
	switch event.Type {
	case "auto_retry_start":
		text = fmt.Sprintf("Pi is retrying the model request (%d/%d)", int(numberValue(payload["attempt"])), int(numberValue(payload["maxAttempts"])))
		level = "warning"
	case "auto_retry_end":
		if boolValue(payload["success"]) {
			text = "Pi model request retry succeeded"
			dispatch.mu.Lock()
			dispatch.lastAttemptError = ""
			dispatch.mu.Unlock()
		} else {
			text = firstNonEmpty(strings.TrimSpace(stringValue(payload["finalError"])), "Pi model request retry failed")
			level = "error"
			dispatch.mu.Lock()
			dispatch.lastAttemptError = text
			dispatch.mu.Unlock()
		}
	case "summarization_retry_scheduled":
		text = fmt.Sprintf("Pi scheduled a summarization retry (%d/%d)", int(numberValue(payload["attempt"])), int(numberValue(payload["maxAttempts"])))
		level = "warning"
	case "summarization_retry_attempt_start":
		text = "Pi is retrying conversation summarization"
	case "summarization_retry_finished":
		text = "Pi summarization retry finished"
	}
	extra := cloneMap(payload)
	delete(extra, "type")
	extra["code"] = "pi_" + event.Type
	m.appendRunNote(dispatch.session.ID, dispatch.session, dispatch.run, level, text, extra)
	return nil
}

func (m *Manager) handlePiExtensionUIRequest(dispatch *piRuntimeRun, raw json.RawMessage) error {
	var request struct {
		ID          string   `json:"id"`
		Method      string   `json:"method"`
		Title       string   `json:"title"`
		Message     string   `json:"message"`
		Options     []string `json:"options"`
		Placeholder string   `json:"placeholder"`
		Prefill     string   `json:"prefill"`
		Timeout     int64    `json:"timeout"`
		NotifyType  string   `json:"notifyType"`
		StatusText  string   `json:"statusText"`
		WidgetLines []string `json:"widgetLines"`
		Text        string   `json:"text"`
	}
	if err := json.Unmarshal(raw, &request); err != nil {
		return fmt.Errorf("decode Pi extension_ui_request: %w", err)
	}
	request.Method = strings.TrimSpace(request.Method)
	if request.Method == "setStatus" {
		return nil
	}
	request.ID = strings.TrimSpace(request.ID)
	if request.ID == "" || request.Method == "" {
		return errors.New("Pi extension UI request is missing id or method")
	}
	if request.Method != "select" && request.Method != "confirm" && request.Method != "input" && request.Method != "editor" {
		text := firstNonEmpty(strings.TrimSpace(request.Message), strings.TrimSpace(request.StatusText), strings.Join(request.WidgetLines, "\n"), strings.TrimSpace(request.Text), strings.TrimSpace(request.Title))
		if text != "" {
			level := strings.ToLower(strings.TrimSpace(request.NotifyType))
			// Extension notify probes (rtk rewrite traces and similar debug
			// chatter) only matter when the extension flags them as warnings
			// or errors; the rest never leave the backend.
			if level != "warning" && level != "error" {
				return nil
			}
			m.appendRunNote(dispatch.session.ID, dispatch.session, dispatch.run, level, truncateToolOutput("tool", text), map[string]any{"code": "pi_extension_ui_" + request.Method})
		}
		return nil
	}

	now := time.Now()
	var expiresAt *time.Time
	if request.Timeout > 0 {
		value := now.Add(time.Duration(request.Timeout) * time.Millisecond)
		expiresAt = &value
	}
	itemID := "pi-ui:" + request.ID
	pending := &pendingServerRequest{
		ItemID: itemID, Prompt: firstNonEmpty(strings.TrimSpace(request.Title), strings.TrimSpace(request.Message), "Pi extension input"),
		RequestedAt: &now, PiRuntime: dispatch.runtime, PiRequestID: request.ID, PiMethod: request.Method,
	}
	eventType := "user_input_req"
	payload := map[string]any{"iid": itemID, "txt": pending.Prompt}
	if request.Method == "confirm" {
		pending.Kind = pendingServerRequestToolApproval
		pending.Command = strings.TrimSpace(request.Message)
		eventType = "approval_req"
		payload["kind"] = string(pending.Kind)
		payload["prompt"] = pending.Prompt
		payload["command"] = pending.Command
	} else {
		pending.Kind = pendingServerRequestUserInput
		question := toolRequestQuestion{ID: "value", Header: pending.Prompt, Question: pending.Prompt, IsOther: request.Method != "select"}
		for _, option := range request.Options {
			question.Options = append(question.Options, toolRequestOption{Label: option})
		}
		pending.Questions = []toolRequestQuestion{question}
		pending.Input = map[string]any{"placeholder": request.Placeholder, "prefill": request.Prefill}
		payload["qs"] = pending.Questions
	}
	if _, exists := dispatch.run.pendingServerRequest(); exists {
		return errors.New("Pi extension UI request arrived while another request is pending")
	}
	if !dispatch.run.setPendingServerRequest(pending) {
		return errors.New("Pi extension UI request could not be registered")
	}
	dispatch.mu.Lock()
	dispatch.dialog = &piRuntimeDialog{id: request.ID}
	parentID := dispatch.assistantMessageID
	dispatch.mu.Unlock()

	assistantState := AssistantStateWaitingInput
	if pending.isApproval() {
		assistantState = AssistantStateWaitingApproval
	}
	if err := m.updateRuntimeState(context.Background(), dispatch.session.ID, applyAssistantStateUpdates(map[string]any{"updated_at": now}, assistantState, now)); err != nil {
		dispatch.run.clearPendingServerRequest()
		return err
	}
	if _, err := m.appendAndBroadcast(context.Background(), dispatch.session.ID, dispatch.session, Event{
		ID: utils.NewID(), Type: eventType, RunID: dispatch.run.runID, ParentID: parentID, Timestamp: now, Payload: payload,
	}); err != nil {
		dispatch.run.clearPendingServerRequest()
		return err
	}
	m.broadcastSessionSummary(context.Background(), dispatch.session.ID)
	if expiresAt != nil {
		delay := time.Until(*expiresAt)
		time.AfterFunc(delay, func() { m.expirePiExtensionDialog(dispatch, request.ID) })
	}
	return nil
}

func (m *Manager) expirePiExtensionDialog(dispatch *piRuntimeRun, requestID string) {
	if dispatch == nil || dispatch.run == nil {
		return
	}
	pending, ok := dispatch.run.takePendingPiRequestForResponse(requestID, true)
	if !ok || pending.PiRuntime != dispatch.runtime {
		return
	}
	eventType := "user_input_res"
	eventPayload := map[string]any{
		"iid": pending.ItemID,
		"err": "Pi extension input timed out",
	}
	if pending.isApproval() {
		eventType = "approval_res"
		eventPayload = map[string]any{
			"iid":    pending.ItemID,
			"act":    "cancel",
			"prompt": pending.Prompt,
		}
	}
	_ = m.respondPiExtensionRequest(dispatch.session, dispatch.run, pending, map[string]any{"cancelled": true}, eventType, eventPayload)
}

func firstPiUserInputAnswer(answers map[string][]string) string {
	for _, key := range []string{"value", "0"} {
		for _, value := range answers[key] {
			if normalized := strings.TrimSpace(value); normalized != "" {
				return normalized
			}
		}
	}
	for _, values := range answers {
		for _, value := range values {
			if normalized := strings.TrimSpace(value); normalized != "" {
				return normalized
			}
		}
	}
	return ""
}

func piExtensionCancellationEvent(request *pendingServerRequest, reason string) (string, map[string]any) {
	if request != nil && request.isApproval() {
		return "approval_res", map[string]any{
			"iid":    request.ItemID,
			"act":    "cancel",
			"prompt": request.Prompt,
		}
	}
	return "user_input_res", map[string]any{
		"iid": request.ItemID,
		"err": firstNonEmpty(strings.TrimSpace(reason), "Pi extension input ended before a response"),
	}
}

func (m *Manager) appendPiExtensionCompletion(
	session tables.WebSessionTable,
	run *activeRun,
	eventType string,
	eventPayload map[string]any,
) error {
	if run == nil {
		return nil
	}
	if eventPayload == nil {
		eventPayload = map[string]any{}
	}
	_, err := m.appendAndBroadcast(context.Background(), session.ID, session, Event{
		ID: utils.NewID(), Type: eventType, RunID: run.runID, ParentID: run.assistantMessageIDSnapshot(),
		Timestamp: time.Now(), Payload: eventPayload,
	})
	return err
}

func (m *Manager) closePendingPiDialog(session tables.WebSessionTable, run *activeRun, reason string) error {
	if run == nil {
		return nil
	}
	waitCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	inFlight, waitErr := run.waitForPiResponseHistory(waitCtx)
	cancel()
	if waitErr != nil && inFlight == nil {
		return waitErr
	}
	if inFlight != nil {
		eventType, eventPayload := piExtensionCancellationEvent(inFlight, reason)
		return m.appendPiExtensionCompletion(session, run, eventType, eventPayload)
	}
	pending, ok := run.pendingServerRequest()
	if !ok || strings.TrimSpace(pending.PiRequestID) == "" {
		return nil
	}
	request, taken := run.takePendingPiRequest(pending.PiRequestID)
	if !taken {
		return nil
	}
	eventType, eventPayload := piExtensionCancellationEvent(request, reason)
	return m.appendPiExtensionCompletion(session, run, eventType, eventPayload)
}

func (m *Manager) respondPiExtensionRequest(
	session tables.WebSessionTable,
	run *activeRun,
	request *pendingServerRequest,
	response map[string]any,
	eventType string,
	eventPayload map[string]any,
) error {
	if run == nil || request == nil || request.PiRuntime == nil || strings.TrimSpace(request.PiRequestID) == "" {
		return errors.New("Pi extension response channel is unavailable")
	}
	historyFinished := false
	defer func() {
		if !historyFinished {
			run.finishPiResponseHistory(request.PiResponseGeneration, false, errors.New("Pi extension response ended before history was persisted"))
		}
	}()
	runtime := request.PiRuntime
	m.piRuntimeMu.Lock()
	registered := m.piRuntimes[session.ID]
	m.piRuntimeMu.Unlock()
	if registered != runtime {
		return errors.New("Pi extension response runtime is no longer active")
	}
	runtime.mu.Lock()
	dispatch := runtime.active
	valid := !runtime.stopped && dispatch != nil && dispatch.run == run
	runtime.mu.Unlock()
	if !valid {
		return errors.New("Pi extension response run is no longer active")
	}
	now := time.Now()
	if eventType == "approval_res" && request.Command != "" {
		if eventPayload == nil {
			eventPayload = map[string]any{}
		}
		if strings.TrimSpace(stringValue(eventPayload["command"])) == "" {
			eventPayload["command"] = request.Command
		}
	}
	if err := m.updateRuntimeState(context.Background(), session.ID, applyAssistantStateUpdates(map[string]any{"updated_at": now}, AssistantStateWorking, now)); err != nil {
		runtime.stop(errors.New("Pi extension response state update failed"))
		return err
	}
	dispatch.mu.Lock()
	if dispatch.dialog != nil && dispatch.dialog.id == request.PiRequestID {
		dispatch.dialog = nil
	}
	dispatch.mu.Unlock()
	if err := m.appendPiExtensionCompletion(session, run, eventType, eventPayload); err != nil {
		run.finishPiResponseHistory(request.PiResponseGeneration, false, err)
		historyFinished = true
		runtime.stop(errors.New("Pi extension response history update failed"))
		return err
	}
	m.broadcastSessionSummary(context.Background(), session.ID)

	response["type"] = "extension_ui_response"
	response["id"] = request.PiRequestID
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	err := runtime.client.Send(ctx, response)
	cancel()
	if err != nil {
		run.finishPiResponseHistory(request.PiResponseGeneration, true, err)
		historyFinished = true
		runtime.stop(errors.New("Pi extension response failed"))
		return err
	}
	run.finishPiResponseHistory(request.PiResponseGeneration, true, nil)
	historyFinished = true
	barrierCtx, barrierCancel := context.WithTimeout(context.Background(), 10*time.Second)
	err = runtime.client.Barrier(barrierCtx, run.runID, request.PiResponseGeneration)
	barrierCancel()
	if err != nil {
		runtime.stop(errors.New("Pi extension response barrier failed"))
		return err
	}
	return nil
}

func (m *Manager) finishPiSettledProjection(dispatch *piRuntimeRun) error {
	m.clearPiNativeQueuedInputs(dispatch.session.ID)
	dispatch.mu.Lock()
	messageOpen := dispatch.assistantMessageOpen
	messageID := dispatch.assistantMessageID
	lastError := dispatch.lastAttemptError
	compactionID := dispatch.compactionToolID
	tools := make([]piRuntimeToolState, 0, len(dispatch.tools))
	for _, tool := range dispatch.tools {
		if tool != nil && !tool.completed {
			tools = append(tools, *tool)
		}
	}
	dispatch.assistantMessageOpen = false
	dispatch.compactionToolID = ""
	dispatch.dialog = nil
	dispatch.mu.Unlock()

	if messageOpen {
		if _, err := m.appendAndBroadcast(context.Background(), dispatch.session.ID, dispatch.session, Event{
			ID: utils.NewID(), Type: "txt_end", RunID: dispatch.run.runID, ParentID: messageID,
			Timestamp: time.Now(), Payload: map[string]any{"mid": messageID},
		}); err != nil {
			return err
		}
	}
	for _, tool := range tools {
		if _, err := m.appendAndBroadcast(context.Background(), dispatch.session.ID, dispatch.session, Event{
			ID: utils.NewID(), Type: "tool_end", RunID: dispatch.run.runID, ParentID: tool.parentID,
			Timestamp: time.Now(), Payload: map[string]any{
				"tid": tool.id, "name": firstNonEmpty(tool.name, "Tool"), "kind": piToolHistoryKind,
				"in": tool.args, "out": tool.output, "ok": false,
			},
		}); err != nil {
			return err
		}
	}
	if compactionID != "" {
		_, _ = m.appendAndBroadcast(context.Background(), dispatch.session.ID, dispatch.session, Event{
			ID: utils.NewID(), Type: "tool_end", RunID: dispatch.run.runID, ParentID: messageID,
			Timestamp: time.Now(), Payload: map[string]any{
				"tid": compactionID, "name": "ContextCompaction", "kind": "context_compaction",
				"out": "Pi context compaction ended without a completion event", "ok": false,
			},
		})
	}
	if err := m.closePendingPiDialog(dispatch.session, dispatch.run, "Pi extension input ended before the run settled"); err != nil {
		return err
	}
	if strings.TrimSpace(lastError) != "" {
		return fmt.Errorf("Pi assistant run failed: %s", truncateString(strings.TrimSpace(lastError), 2000))
	}
	return nil
}

func piMessageText(message piRPCMessage) string {
	var builder strings.Builder
	for _, block := range message.Content {
		if block.Type == "text" {
			builder.WriteString(block.Text)
		}
	}
	return builder.String()
}

func piToolResultText(value any) string {
	record := decodeRawObject(value)
	content, ok := record["content"].([]any)
	if !ok {
		if text := strings.TrimSpace(stringValue(record["text"])); text != "" {
			return truncateToolOutput("tool", text)
		}
		encoded, _ := json.Marshal(value)
		return truncateToolOutput("tool", string(encoded))
	}
	parts := make([]string, 0, len(content))
	for _, item := range content {
		block := decodeRawObject(item)
		if strings.EqualFold(stringValue(block["type"]), "text") {
			parts = append(parts, stringValue(block["text"]))
		}
	}
	return truncateToolOutput("tool", strings.Join(parts, "\n"))
}
