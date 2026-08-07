package websession

import (
	"context"
	"fmt"
	"strings"
	"time"

	"code-kanban/model/tables"
	"code-kanban/utils"
)

func claudeResultUsage(raw map[string]any) (input, cached, output int64) {
	usage := decodeRawObject(raw["usage"])
	input = int64(numberValue(usage["input_tokens"]))
	output = int64(numberValue(usage["output_tokens"]))
	cacheRead := int64(numberValue(usage["cache_read_input_tokens"]))
	cacheCreation := int64(numberValue(usage["cache_creation_input_tokens"]))
	// Claude's input_tokens excludes cache reads/creation, while the shared
	// session model treats input_tokens as the full context input and keeps the
	// cached portion in a separate bucket. Normalize before persisting so the
	// context estimate does not collapse to only the uncached prefix.
	cached = cacheRead + cacheCreation
	input += cached
	return input, cached, output
}

func claudeResultContextWindow(raw map[string]any, session tables.WebSessionTable, resolvedModel string) int64 {
	modelUsage := decodeRawObject(raw["modelUsage"])
	if len(modelUsage) == 0 {
		modelUsage = decodeRawObject(raw["model_usage"])
	}
	if len(modelUsage) == 0 {
		return 0
	}
	windows := make(map[string]int64, len(modelUsage))
	for modelName, rawUsage := range modelUsage {
		usage := decodeRawObject(rawUsage)
		window := int64(numberValue(firstNonNilValue(usage["contextWindow"], usage["context_window"])))
		if window <= 0 {
			continue
		}
		windows[strings.ToLower(strings.TrimSpace(modelName))] = window
	}
	for _, preferred := range []string{resolvedModel, session.Model} {
		preferred = strings.ToLower(strings.TrimSpace(preferred))
		if preferred == "" {
			continue
		}
		if window := windows[preferred]; window > 0 {
			return window
		}
		matchedWindow := int64(0)
		matchCount := 0
		for modelKey, window := range windows {
			if strings.Contains(modelKey, preferred) || strings.Contains(preferred, modelKey) {
				matchedWindow = window
				matchCount++
			}
		}
		if matchCount == 1 {
			return matchedWindow
		}
	}
	if len(windows) == 1 {
		for _, window := range windows {
			return window
		}
	}
	return 0
}

func firstNonNilValue(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func claudeCompactionStatus(raw map[string]any) (started, completed, succeeded bool, message string) {
	subtype := strings.ToLower(strings.TrimSpace(stringValue(raw["subtype"])))
	status := strings.ToLower(strings.TrimSpace(stringValue(raw["status"])))
	result := strings.ToLower(strings.TrimSpace(stringValue(raw["compact_result"])))
	message = strings.TrimSpace(firstNonEmpty(
		stringValue(raw["compact_error"]),
		stringValue(raw["error"]),
		stringValue(raw["message"]),
	))
	hasCompactError := strings.TrimSpace(stringValue(raw["compact_error"])) != ""
	explicitCompaction := result != "" || hasCompactError || strings.Contains(status, "compact") || strings.Contains(subtype, "compact")
	if !explicitCompaction {
		return false, false, false, message
	}
	started = status == "compacting" || status == "compaction_started" ||
		subtype == "compacting" || subtype == "compaction_started"
	completed = result != "" || hasCompactError || status == "compacted" || status == "compaction_completed" ||
		subtype == "compacted" || subtype == "compaction_completed" ||
		strings.Contains(status, "compaction_failed") || strings.Contains(status, "compaction_error") ||
		strings.Contains(subtype, "compaction_failed") || strings.Contains(subtype, "compaction_error")
	if !started && !completed {
		return false, false, false, message
	}
	succeeded = result == "success" || result == "succeeded" || result == "completed" ||
		status == "compacted" || status == "compaction_completed" ||
		subtype == "compacted" || subtype == "compaction_completed"
	if strings.Contains(result, "fail") || strings.Contains(result, "error") ||
		strings.Contains(status, "fail") || strings.Contains(status, "error") {
		succeeded = false
	}
	return started, completed, succeeded, message
}

func claudeCompactionToolMeta(message string) map[string]any {
	return map[string]any{
		"title":    "ContextCompaction",
		"kind":     "context_compaction",
		"subtitle": strings.TrimSpace(message),
	}
}

func (m *Manager) handleClaudeCompactionStatus(session tables.WebSessionTable, run *activeRun, raw map[string]any) {
	started, completed, succeeded, message := claudeCompactionStatus(raw)
	if !started && !completed {
		return
	}
	now := time.Now()
	if started {
		toolID := utils.NewID()
		if !run.beginClaudeCompaction(toolID, now) {
			return
		}
		meta := claudeCompactionToolMeta("Claude is compacting the conversation context.")
		_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
			ID:        utils.NewID(),
			Seq:       0,
			Type:      "tool_st",
			RunID:     run.runID,
			ParentID:  run.assistantMessageIDSnapshot(),
			Timestamp: now,
			Payload: map[string]any{
				"tid":  toolID,
				"name": "ContextCompaction",
				"kind": "context_compaction",
				"meta": meta,
			},
		})
	}
	if !completed {
		return
	}
	toolID, _ := run.claudeCompactionState()
	if toolID == "" {
		toolID = utils.NewID()
		run.beginClaudeCompaction(toolID, now)
	}
	if strings.TrimSpace(message) == "" {
		if succeeded {
			message = "Context compacted"
		} else {
			message = "Claude context compaction failed"
		}
	}
	meta := claudeCompactionToolMeta(message)
	_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
		ID:        utils.NewID(),
		Seq:       0,
		Type:      "tool_end",
		RunID:     run.runID,
		ParentID:  run.assistantMessageIDSnapshot(),
		Timestamp: now,
		Payload: map[string]any{
			"tid":  toolID,
			"name": "ContextCompaction",
			"kind": "context_compaction",
			"out":  message,
			"ok":   succeeded,
			"meta": meta,
		},
	})
	run.clearClaudeCompaction()
	if succeeded {
		if record, err := m.GetSession(context.Background(), session.ID); err == nil {
			_ = m.updateRuntimeState(context.Background(), session.ID, contextEstimateBaselineResetUpdate(record, now))
		}
	}
}

// Claude's stream-json permission bridge is deliberately kept separate from
// the transcript parser. Control requests are live RPC messages: the process
// must stay open while the browser answers, whereas transcript history can be
// rebuilt later from the native JSONL file.
type claudeControlRequest struct {
	RequestID           string
	ToolName            string
	DisplayName         string
	ToolUseID           string
	Input               any
	RequiresInteraction bool
}

func decodeClaudeControlRequest(raw map[string]any) (claudeControlRequest, bool) {
	if strings.TrimSpace(stringValue(raw["type"])) != "control_request" {
		return claudeControlRequest{}, false
	}
	request := decodeRawObject(raw["request"])
	if strings.TrimSpace(stringValue(request["subtype"])) != "can_use_tool" {
		return claudeControlRequest{}, false
	}
	requestID := strings.TrimSpace(stringValue(raw["request_id"]))
	toolName := normalizeClaudeControlToolName(stringValue(request["tool_name"]))
	toolUseID := strings.TrimSpace(firstNonEmpty(
		stringValue(request["tool_use_id"]),
		stringValue(request["toolUseId"]),
	))
	input := firstNonNilValue(request["input"], request["tool_input"], request["toolInput"])
	if requestID == "" || toolName == "" {
		return claudeControlRequest{}, false
	}
	return claudeControlRequest{
		RequestID:           requestID,
		ToolName:            toolName,
		DisplayName:         firstNonEmpty(stringValue(request["display_name"]), toolName),
		ToolUseID:           toolUseID,
		Input:               input,
		RequiresInteraction: request["requires_user_interaction"] == true,
	}, true
}

func normalizeClaudeControlToolName(toolName string) string {
	trimmed := strings.TrimSpace(toolName)
	switch strings.ToLower(trimmed) {
	case "askuserquestion", "ask_user_question":
		return "AskUserQuestion"
	case "exitplanmode", "exit_plan_mode":
		return "ExitPlanMode"
	default:
		return trimmed
	}
}

func claudeControlResult(behavior string, input any, message string) map[string]any {
	if behavior == "allow" {
		updatedInput := decodeRawObject(input)
		if updatedInput == nil {
			updatedInput = map[string]any{}
		}
		return map[string]any{
			"behavior":     "allow",
			"updatedInput": updatedInput,
		}
	}
	return map[string]any{
		"behavior": "deny",
		"message":  firstNonEmpty(strings.TrimSpace(message), "The user declined this tool use."),
	}
}

func claudeControlResponse(requestID string, result map[string]any) map[string]any {
	return map[string]any{
		"type": "control_response",
		"response": map[string]any{
			"subtype":    "success",
			"request_id": strings.TrimSpace(requestID),
			"response":   result,
		},
	}
}

func claudeControlCancelID(raw map[string]any) string {
	if strings.TrimSpace(stringValue(raw["type"])) != "control_cancel_request" {
		return ""
	}
	return strings.TrimSpace(stringValue(raw["request_id"]))
}

func claudeControlToolCommand(input any) string {
	record := decodeRawObject(input)
	return firstNonEmpty(
		stringValue(record["command"]),
		stringValue(record["cmd"]),
		stringValue(record["file_path"]),
		stringValue(record["path"]),
		stringValue(record["query"]),
	)
}

func claudeControlPrompt(request claudeControlRequest, kind string) string {
	if request.ToolName == "AskUserQuestion" {
		return summarizeToolQuestions(decodeToolQuestions(decodeRawObject(request.Input)["questions"]))
	}
	if request.ToolName == "ExitPlanMode" {
		return "Approve Claude's plan and continue?"
	}
	if request.DisplayName != "" && request.DisplayName != request.ToolName {
		return fmt.Sprintf("Claude is waiting for approval to use %s.", request.DisplayName)
	}
	if kind != "" {
		return fmt.Sprintf("Claude is waiting for approval to use %s.", kind)
	}
	return "Claude is waiting for approval before continuing."
}

func (m *Manager) handleClaudeControlRequest(
	session tables.WebSessionTable,
	run *activeRun,
	request claudeControlRequest,
) {
	if run == nil || strings.TrimSpace(request.RequestID) == "" {
		return
	}
	if pending, ok := run.pendingServerRequest(); ok &&
		strings.TrimSpace(pending.ControlRequestID) == strings.TrimSpace(request.RequestID) {
		return
	}

	now := time.Now()
	toolName := normalizeClaudeControlToolName(request.ToolName)
	itemID := firstNonEmpty(
		strings.TrimSpace(request.ToolUseID),
		run.currentToolMessageSnapshot(),
		strings.TrimSpace(request.RequestID),
	)
	if toolName == "AskUserQuestion" {
		questions := decodeToolQuestions(decodeRawObject(request.Input)["questions"])
		pending := &pendingServerRequest{
			ControlRequestID: request.RequestID,
			Kind:             pendingServerRequestUserInput,
			ItemID:           itemID,
			Prompt:           firstNonEmpty(summarizeToolQuestions(questions), request.DisplayName),
			Questions:        questions,
			Input:            request.Input,
			RequestedAt:      &now,
		}
		run.setPendingServerRequest(pending)
		m.pauseActiveCallTimeout(run)
		_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
			ID:        utils.NewID(),
			Seq:       0,
			Type:      "user_input_req",
			RunID:     run.runID,
			ParentID:  run.assistantMessageIDSnapshot(),
			Timestamp: now,
			Payload: map[string]any{
				"iid": itemID,
				"txt": pending.Prompt,
				"qs":  questions,
			},
		})
		_ = m.updateRuntimeState(context.Background(), session.ID,
			applyAssistantStateUpdates(map[string]any{"updated_at": now}, AssistantStateWaitingInput, now))
		m.broadcastSessionSummary(context.Background(), session.ID)
		return
	}

	kind := claudeToolKind(toolName)
	pendingKind := pendingServerRequestToolApproval
	switch kind {
	case "command_execution":
		pendingKind = pendingServerRequestCommandApproval
	case "file_change":
		pendingKind = pendingServerRequestFileChangeApproval
	}
	if toolName == "ExitPlanMode" {
		pendingKind = pendingServerRequestPlanApproval
		run.markCompletedPlanTool()
	}
	pending := &pendingServerRequest{
		ControlRequestID: request.RequestID,
		Kind:             pendingKind,
		ItemID:           itemID,
		Prompt:           claudeControlPrompt(request, kind),
		Command:          claudeControlToolCommand(request.Input),
		Input:            request.Input,
		RequestedAt:      &now,
	}
	run.setPendingServerRequest(pending)
	m.pauseActiveCallTimeout(run)
	_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
		ID:        utils.NewID(),
		Seq:       0,
		Type:      "approval_req",
		RunID:     run.runID,
		ParentID:  run.assistantMessageIDSnapshot(),
		Timestamp: now,
		Payload: map[string]any{
			"iid":     itemID,
			"kind":    firstNonEmpty(string(pendingKind), kind),
			"prompt":  pending.Prompt,
			"command": pending.Command,
			"name":    claudeToolDisplayName(toolName, kind),
			"in":      claudeToolInput(toolName, request.Input),
		},
	})
	state := AssistantStateWaitingApproval
	if pendingKind == pendingServerRequestPlanApproval {
		state = AssistantStateWaitingPlanApproval
	}
	_ = m.updateRuntimeState(context.Background(), session.ID,
		applyAssistantStateUpdates(map[string]any{"updated_at": now}, state, now))
	m.broadcastSessionSummary(context.Background(), session.ID)
}

func (m *Manager) respondClaudeControl(
	session tables.WebSessionTable,
	run *activeRun,
	pending *pendingServerRequest,
	behavior string,
	input any,
	message string,
	answers map[string][]string,
) error {
	if run == nil || pending == nil || strings.TrimSpace(pending.ControlRequestID) == "" {
		return fmt.Errorf("Claude control request is unavailable")
	}
	if err := run.writeJSONInput(claudeControlResponse(
		pending.ControlRequestID,
		claudeControlResult(behavior, input, message),
	)); err != nil {
		return err
	}
	run.clearPendingServerRequest()
	if pending.Kind == pendingServerRequestPlanApproval {
		run.clearCompletedPlanTool()
	}
	m.resumeActiveCallTimeout(run)
	now := time.Now()
	payload := map[string]any{
		"iid":    pending.ItemID,
		"prompt": pending.Prompt,
		"act":    "approve",
	}
	if behavior != "allow" {
		payload["act"] = "reject"
		payload["err"] = firstNonEmpty(strings.TrimSpace(message), "Approval rejected")
	}
	if len(answers) > 0 {
		payload["ans"] = answers
	}
	eventType := "approval_res"
	if pending.Kind == pendingServerRequestUserInput {
		eventType = "user_input_res"
	}
	if _, err := m.appendAndBroadcast(context.Background(), session.ID, session, Event{
		ID:        utils.NewID(),
		Seq:       0,
		Type:      eventType,
		RunID:     run.runID,
		ParentID:  run.assistantMessageIDSnapshot(),
		Timestamp: now,
		Payload:   payload,
	}); err != nil {
		return err
	}
	_ = m.updateRuntimeState(context.Background(), session.ID,
		applyAssistantStateUpdates(map[string]any{"updated_at": now}, AssistantStateWorking, now))
	m.broadcastSessionSummary(context.Background(), session.ID)
	return nil
}

// handleClaudeControlCancel clears the matching browser prompt and records a
// terminal response so history reconstruction cannot revive a stale card.
func (m *Manager) handleClaudeControlCancel(session tables.WebSessionTable, run *activeRun, requestID string) {
	if run == nil {
		return
	}
	pending, ok := run.pendingServerRequest()
	if !ok || strings.TrimSpace(pending.ControlRequestID) != strings.TrimSpace(requestID) ||
		!run.clearPendingControlRequest(requestID) {
		return
	}
	if pending.Kind == pendingServerRequestPlanApproval {
		run.clearCompletedPlanTool()
	}
	m.resumeActiveCallTimeout(run)
	now := time.Now()
	eventType := "approval_res"
	payload := map[string]any{
		"iid":    pending.ItemID,
		"prompt": pending.Prompt,
		"act":    "cancel",
		"err":    "Claude canceled this request.",
	}
	if pending.Kind == pendingServerRequestUserInput {
		eventType = "user_input_res"
	}
	_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
		ID:        utils.NewID(),
		Seq:       0,
		Type:      eventType,
		RunID:     run.runID,
		ParentID:  run.assistantMessageIDSnapshot(),
		Timestamp: now,
		Payload:   payload,
	})
	_ = m.updateRuntimeState(context.Background(), session.ID,
		applyAssistantStateUpdates(map[string]any{"updated_at": now}, AssistantStateWorking, now))
	m.broadcastSessionSummary(context.Background(), session.ID)
}

func (m *Manager) claudeControlResponseInput(pending *pendingServerRequest, answers map[string][]string) map[string]any {
	input := decodeRawObject(pending.Input)
	if input == nil {
		input = map[string]any{}
	}
	answerMap := map[string]any{}
	for index, question := range pending.Questions {
		values := answers[strings.TrimSpace(question.ID)]
		if len(values) == 0 {
			values = answers[fmt.Sprintf("%d", index)]
		}
		if len(values) == 0 {
			continue
		}
		key := strings.TrimSpace(firstNonEmpty(question.Question, question.Header, question.ID))
		if key == "" {
			continue
		}
		normalized := make([]string, 0, len(values))
		for _, value := range values {
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				normalized = append(normalized, trimmed)
			}
		}
		if len(normalized) > 0 {
			answerMap[key] = strings.Join(normalized, ",")
		}
	}
	input["answers"] = answerMap
	return input
}

func hasClaudeAnswerValues(answers map[string][]string) bool {
	for _, values := range answers {
		for _, value := range values {
			if strings.TrimSpace(value) != "" {
				return true
			}
		}
	}
	return false
}

func (m *Manager) claudeControlAvailable(run *activeRun) bool {
	if run == nil {
		return false
	}
	pending, ok := run.pendingServerRequest()
	return ok && strings.TrimSpace(pending.ControlRequestID) != ""
}
