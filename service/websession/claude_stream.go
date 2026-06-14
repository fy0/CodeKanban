package websession

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"code-kanban/model"
	"code-kanban/model/tables"
	"code-kanban/utils"
	"code-kanban/utils/ai_assistant2/log_watcher"

	"gorm.io/gorm"
)

const sourceKindClaudeStreamJSON = "claude_stream_json"

type claudeSessionParseResult struct {
	SessionID       string
	LastPrompt      string
	PermissionMode  string
	StartedAt       *time.Time
	UpdatedAt       *time.Time
	LatestMessageAt *time.Time
	TurnCount       int
	Items           []HistoryItem
}

func stripClaudePlanPreamble(text string) string {
	trimmed := strings.TrimSpace(text)
	prefix := planPromptPreamble + "\n\nUser request:\n"
	if strings.HasPrefix(trimmed, prefix) {
		return strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
	}
	return trimmed
}

func claudePlanHistoryItem(sourceItemID string, text string, planFilePath string, ts *time.Time, payload map[string]any) HistoryItem {
	planID := firstNonEmpty(strings.TrimSpace(sourceItemID), utils.NewID())
	meta := map[string]any{
		"title": "Plan",
		"kind":  "plan",
	}
	if strings.TrimSpace(planFilePath) != "" {
		meta["path"] = strings.TrimSpace(planFilePath)
		meta["subtitle"] = strings.TrimSpace(planFilePath)
	}
	return HistoryItem{
		ID:           planID,
		SourceItemID: nilIfEmptyHistory(planID),
		Kind:         "tool",
		ItemType:     "plan",
		Timestamp:    ts,
		ObservedAt:   ts,
		Payload:      payload,
		Tool: &HistoryTool{
			ID:     planID,
			Name:   "Plan",
			Kind:   "plan",
			Output: text,
			Status: "done",
			Meta:   meta,
		},
	}
}

func defaultSourceKind(agent Agent) string {
	if normalizeAgent(agent) == AgentClaude {
		return sourceKindClaudeStreamJSON
	}
	return string(defaultSessionBackend(agent))
}

func claudeSessionFilePath(cwd, sessionID string) (string, error) {
	normalizedCwd := strings.TrimSpace(cwd)
	normalizedSessionID := strings.TrimSpace(sessionID)
	if normalizedCwd == "" {
		return "", fmt.Errorf("cwd is required")
	}
	if normalizedSessionID == "" {
		return "", fmt.Errorf("session id is required")
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(
		homeDir,
		log_watcher.ClaudeCodeDirName,
		log_watcher.ClaudeCodeProjectsSubDir,
		log_watcher.EncodePathForClaude(normalizedCwd),
		normalizedSessionID+".jsonl",
	), nil
}

func claudeUserMessagePayload(text string, attachments []Attachment, workflowMode WorkflowMode) ([]byte, error) {
	preparedText := strings.TrimSpace(text)
	contentBlocks := make([]map[string]any, 0, len(attachments)+1)
	if preparedText != "" {
		contentBlocks = append(contentBlocks, map[string]any{
			"type": "text",
			"text": preparedText,
		})
	}
	for _, attachment := range attachments {
		data, err := os.ReadFile(attachment.Path)
		if err != nil {
			return nil, err
		}
		contentBlocks = append(contentBlocks, map[string]any{
			"type": "image",
			"source": map[string]any{
				"type":       "base64",
				"media_type": attachment.Mime,
				"data":       base64.StdEncoding.EncodeToString(data),
			},
		})
	}

	payload := map[string]any{
		"type": "user",
		"message": map[string]any{
			"role": "user",
		},
	}
	message := payload["message"].(map[string]any)
	if len(contentBlocks) == 0 {
		message["content"] = preparedText
	} else if len(contentBlocks) == 1 && len(attachments) == 0 && contentBlocks[0]["type"] == "text" {
		message["content"] = preparedText
	} else {
		message["content"] = contentBlocks
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}

func (m *Manager) buildClaudeResumeCommand(ctx context.Context, session tables.WebSessionTable) (*exec.Cmd, error) {
	args := []string{
		"-p",
		"--output-format", "stream-json",
		"--input-format", "stream-json",
		"--replay-user-messages",
		"--verbose",
	}
	claudeRuntime := effectiveClaudeRuntime(session)
	if claudeRuntime == ClaudeRuntimeCCR {
		if err := m.ensureCCRClaudeHookSettings(); err != nil {
			return nil, err
		}
	} else {
		settingsPath, err := m.ensureClaudeHookServer()
		if err != nil {
			return nil, err
		}
		args = append(args, "--settings", settingsPath)
	}
	workflowMode := effectiveWorkflowMode(session)
	permissionLevel := effectivePermissionLevel(session)
	switch normalizeWorkflowMode(workflowMode) {
	case WorkflowModePlan:
		args = append(args, "--permission-mode", "plan")
	default:
		switch permissionLevel {
		case PermissionLevelYolo:
			args = append(args, "--dangerously-skip-permissions")
		case PermissionLevelElevated:
			args = append(args, "--permission-mode", "acceptEdits")
		default:
			return nil, fmt.Errorf("claude web sessions do not support permission level %q", permissionLevel)
		}
	}
	if session.NativeSessionID == nil || strings.TrimSpace(*session.NativeSessionID) == "" {
		return nil, fmt.Errorf("claude deferred resume requires a native session id")
	}
	args = append(args, "--resume", strings.TrimSpace(*session.NativeSessionID))
	if strings.TrimSpace(session.Model) != "" {
		args = append(args, "--model", strings.TrimSpace(session.Model))
	}
	if effort := claudeReasoningEffortArg(ReasoningEffort(session.ReasoningEffort)); effort != "" {
		args = append(args, "--effort", effort)
	}
	cmd := m.buildClaudeCommand(ctx, claudeRuntime, args)
	cmd.Dir = session.Cwd
	cmd.Env = m.claudeCommandEnv(claudeRuntime)
	return cmd, nil
}

func claudeReasoningEffortArg(effort ReasoningEffort) string {
	switch normalizeReasoningEffort(effort) {
	case ReasoningEffortLow:
		return "low"
	case ReasoningEffortMedium:
		return "medium"
	case ReasoningEffortHigh:
		return "high"
	case ReasoningEffortXHigh:
		return "max"
	default:
		return ""
	}
}

func claudeToolKind(toolName string) string {
	switch strings.TrimSpace(toolName) {
	case "Bash":
		return "command_execution"
	case "Edit", "Write", "MultiEdit", "NotebookEdit":
		return "file_change"
	case "WebSearch":
		return "web_search"
	default:
		return "dynamic_tool_call"
	}
}

func claudeToolDisplayName(toolName string, kind string) string {
	switch kind {
	case "command_execution":
		return "CommandExecution"
	case "file_change":
		return "FileChange"
	case "web_search":
		return "WebSearch"
	default:
		return firstNonEmpty(strings.TrimSpace(toolName), "DynamicToolCall")
	}
}

func claudeToolInput(toolName string, input any) any {
	record := decodeRawObject(input)
	switch strings.TrimSpace(toolName) {
	case "Bash":
		return map[string]any{
			"command":     stringValue(record["command"]),
			"description": stringValue(record["description"]),
		}
	case "Edit", "Write", "MultiEdit", "NotebookEdit":
		return map[string]any{
			"file_path": firstNonEmpty(
				stringValue(record["file_path"]),
				stringValue(record["path"]),
			),
			"old_string": stringValue(record["old_string"]),
			"new_string": stringValue(record["new_string"]),
		}
	case "WebSearch":
		return map[string]any{
			"query": stringValue(record["query"]),
		}
	default:
		return input
	}
}

func claudeToolMeta(toolName string, kind string, input any) map[string]any {
	record := decodeRawObject(input)
	subtitle := ""
	switch kind {
	case "command_execution":
		subtitle = firstNonEmpty(stringValue(record["command"]), stringValue(record["description"]))
	case "file_change":
		subtitle = firstNonEmpty(stringValue(record["file_path"]), stringValue(record["path"]))
	case "web_search":
		subtitle = stringValue(record["query"])
	default:
		subtitle = firstNonEmpty(stringValue(record["description"]), stringValue(record["query"]))
	}
	return map[string]any{
		"title":    claudeToolDisplayName(toolName, kind),
		"kind":     kind,
		"subtitle": subtitle,
	}
}

func claudeToolResultContentText(content any) string {
	switch typed := content.(type) {
	case string:
		return typed
	case []any:
		parts := make([]string, 0, len(typed))
		for _, raw := range typed {
			record := decodeRawObject(raw)
			switch strings.TrimSpace(stringValue(record["type"])) {
			case "text":
				if text := strings.TrimSpace(stringValue(record["text"])); text != "" {
					parts = append(parts, text)
				}
			default:
				if rendered := strings.TrimSpace(firstNonEmpty(stringValue(record["text"]), mustJSONText(record))); rendered != "" {
					parts = append(parts, rendered)
				}
			}
		}
		return strings.TrimSpace(strings.Join(parts, "\n"))
	case map[string]any:
		record := typed
		if cleaned := strings.TrimSpace(stringValue(record["cleanedText"])); cleaned != "" {
			return cleaned
		}
		if text := strings.TrimSpace(stringValue(record["text"])); text != "" {
			return text
		}
		if raw := strings.TrimSpace(stringValue(record["rawContent"])); raw != "" {
			return raw
		}
		return strings.TrimSpace(mustJSONText(record))
	default:
		return strings.TrimSpace(mustJSONText(content))
	}
}

func claudeToolUseResultSummary(raw any) string {
	switch typed := raw.(type) {
	case string:
		return strings.TrimSpace(typed)
	case map[string]any:
		return strings.TrimSpace(firstNonEmpty(
			stringValue(typed["stdout"]),
			stringValue(typed["stderr"]),
			mustJSONText(typed),
		))
	default:
		return strings.TrimSpace(mustJSONText(raw))
	}
}

func decodeClaudeAnswerValues(value any) []string {
	switch typed := value.(type) {
	case string:
		if trimmed := strings.TrimSpace(typed); trimmed != "" {
			return []string{trimmed}
		}
	case []string:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				values = append(values, trimmed)
			}
		}
		return values
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			if trimmed := strings.TrimSpace(stringValue(item)); trimmed != "" {
				values = append(values, trimmed)
			}
		}
		return values
	case map[string]any:
		for _, key := range []string{"answers", "values", "labels"} {
			if values := decodeClaudeAnswerValues(typed[key]); len(values) > 0 {
				return values
			}
		}
		for _, key := range []string{"answer", "value", "label"} {
			if value := strings.TrimSpace(stringValue(typed[key])); value != "" {
				return []string{value}
			}
		}
	}
	return nil
}

func decodeClaudeAskUserAnswers(raw string, questions []toolRequestQuestion) []HistoryAnswerEntry {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var decoded any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return nil
	}
	record := decodeRawObject(decoded)
	if nested := decodeRawObject(record["answers"]); len(nested) > 0 {
		record = nested
	}
	answerMap := make(map[string]any, len(record))
	for key, value := range record {
		if values := decodeClaudeAnswerValues(value); len(values) > 0 {
			answerMap[key] = values
		}
	}
	if len(answerMap) == 0 {
		return nil
	}
	return historyAnswerEntries(answerMap, questions)
}

func (m *Manager) registerClaudeImageAttachment(block map[string]any) (HistoryAttachment, bool) {
	source := decodeRawObject(block["source"])
	if path := firstNonEmpty(
		stringValue(source["path"]),
		stringValue(block["path"]),
		stringValue(block["file_path"]),
	); strings.TrimSpace(path) != "" {
		attachment, err := m.registerExternalAttachment(path)
		return attachment, err == nil
	}

	if strings.TrimSpace(stringValue(source["type"])) != "base64" {
		return HistoryAttachment{}, false
	}
	encodedData := strings.TrimSpace(stringValue(source["data"]))
	if encodedData == "" {
		return HistoryAttachment{}, false
	}
	data, err := base64.StdEncoding.DecodeString(encodedData)
	if err != nil {
		return HistoryAttachment{}, false
	}
	mimeType := firstNonEmpty(
		strings.TrimSpace(stringValue(source["media_type"])),
		strings.TrimSpace(stringValue(source["mediaType"])),
		"application/octet-stream",
	)
	ext := filepath.Ext(firstNonEmpty(stringValue(block["file_name"]), stringValue(block["fileName"])))
	if ext == "" {
		if extensions, extErr := mime.ExtensionsByType(mimeType); extErr == nil && len(extensions) > 0 {
			ext = extensions[0]
		}
	}
	sum := sha1.Sum(append([]byte(mimeType), data...))
	attachmentID := fmt.Sprintf("claude_%x", sum[:8])
	targetPath := m.store.attachmentPath(attachmentID, ext)
	if _, statErr := os.Stat(targetPath); statErr != nil {
		if writeErr := os.WriteFile(targetPath, data, 0o644); writeErr != nil {
			return HistoryAttachment{}, false
		}
		meta := attachmentMeta{
			ID:        attachmentID,
			Name:      firstNonEmpty(stringValue(block["file_name"]), filepath.Base(targetPath), attachmentID),
			Mime:      mimeType,
			Size:      int64(len(data)),
			Path:      targetPath,
			CreatedAt: time.Now(),
		}
		if metaBytes, marshalErr := json.Marshal(meta); marshalErr == nil {
			_ = os.WriteFile(m.store.attachmentPath(attachmentID, ".json"), metaBytes, 0o644)
		}
	}
	return HistoryAttachment{
		ID:   attachmentID,
		Name: firstNonEmpty(stringValue(block["file_name"]), filepath.Base(targetPath), attachmentID),
		Mime: mimeType,
		Size: int64(len(data)),
		Path: targetPath,
	}, true
}

func (m *Manager) claudeHistoryAttachments(blocks []any) []HistoryAttachment {
	attachments := make([]HistoryAttachment, 0)
	for _, raw := range blocks {
		block := decodeRawObject(raw)
		switch strings.TrimSpace(stringValue(block["type"])) {
		case "image", "input_image":
			if attachment, ok := m.registerClaudeImageAttachment(block); ok {
				attachments = append(attachments, attachment)
			}
		}
	}
	if len(attachments) == 0 {
		return nil
	}
	return attachments
}

func claudeTextBlocks(blocks []any) string {
	parts := make([]string, 0, len(blocks))
	for _, raw := range blocks {
		block := decodeRawObject(raw)
		if strings.TrimSpace(stringValue(block["type"])) != "text" {
			continue
		}
		if text := strings.TrimSpace(stringValue(block["text"])); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func (m *Manager) parseClaudeStreamHistory(filePath string, workflowMode WorkflowMode) (claudeSessionParseResult, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return claudeSessionParseResult{}, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return claudeSessionParseResult{}, err
	}

	result := claudeSessionParseResult{
		UpdatedAt: ptr(info.ModTime()),
	}
	pendingTools := make(map[string]int)
	pendingAskQuestions := make(map[string][]toolRequestQuestion)
	turnIDs := make(map[string]struct{})
	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 2*1024*1024)

	appendItem := func(item HistoryItem) int {
		item.OrderIndex = int64(len(result.Items) + 1)
		if strings.TrimSpace(item.ID) == "" {
			item.ID = fmt.Sprintf("claude_%d", item.OrderIndex)
		}
		result.Items = append(result.Items, item)
		return len(result.Items) - 1
	}

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		entryType := strings.TrimSpace(stringValue(entry["type"]))
		entryTimestamp := parseHistoryTimestamp(entry["timestamp"])
		if entryTimestamp != nil {
			if result.StartedAt == nil || entryTimestamp.Before(*result.StartedAt) {
				value := *entryTimestamp
				result.StartedAt = &value
			}
			if result.LatestMessageAt == nil || entryTimestamp.After(*result.LatestMessageAt) {
				value := *entryTimestamp
				result.LatestMessageAt = &value
			}
		}
		if result.SessionID == "" {
			result.SessionID = stringValue(entry["sessionId"])
		}

		switch entryType {
		case "last-prompt":
			result.LastPrompt = firstNonEmpty(stringValue(entry["lastPrompt"]), result.LastPrompt)
		case "queue-operation":
			if strings.TrimSpace(stringValue(entry["operation"])) == "enqueue" {
				result.LastPrompt = firstNonEmpty(stringValue(entry["content"]), result.LastPrompt)
			}
		case "permission-mode":
			result.PermissionMode = firstNonEmpty(stringValue(entry["permissionMode"]), result.PermissionMode)
		case "assistant":
			message := decodeRawObject(entry["message"])
			assistantID := firstNonEmpty(stringValue(entry["uuid"]), stringValue(message["id"]), utils.NewID())
			content := make([]any, 0)
			for _, block := range decodeRawArray(message["content"]) {
				content = append(content, block)
			}
			text := claudeTextBlocks(content)
			if text != "" {
				appendItem(HistoryItem{
					ID:           assistantID,
					SourceItemID: nilIfEmptyHistory(assistantID),
					Kind:         "assistant",
					ItemType:     "agent_message",
					Text:         text,
					Timestamp:    entryTimestamp,
					ObservedAt:   entryTimestamp,
					Done:         true,
					Payload:      cloneMap(entry),
				})
			}
			for _, rawBlock := range content {
				block := decodeRawObject(rawBlock)
				if strings.TrimSpace(stringValue(block["type"])) != "tool_use" {
					continue
				}
				toolUseID := firstNonEmpty(stringValue(block["id"]), utils.NewID())
				toolName := strings.TrimSpace(stringValue(block["name"]))
				toolInput := block["input"]
				if toolName == "AskUserQuestion" {
					questions := decodeToolQuestions(decodeRawObject(toolInput)["questions"])
					appendItem(HistoryItem{
						ID:           toolUseID,
						SourceItemID: nilIfEmptyHistory(toolUseID),
						Kind:         "system",
						ItemType:     "user_input_request",
						Text:         summarizeToolQuestions(questions),
						Timestamp:    entryTimestamp,
						ObservedAt:   entryTimestamp,
						Level:        "warn",
						Detail: &HistoryDetail{
							Type:      "user_input_request",
							Prompt:    summarizeToolQuestions(questions),
							Questions: questions,
						},
						Payload: cloneMap(entry),
					})
					pendingAskQuestions[toolUseID] = questions
					continue
				}
				if toolName == "ExitPlanMode" {
					input := decodeRawObject(toolInput)
					appendItem(claudePlanHistoryItem(
						toolUseID,
						strings.TrimSpace(stringValue(input["plan"])),
						strings.TrimSpace(stringValue(input["planFilePath"])),
						entryTimestamp,
						cloneMap(entry),
					))
					continue
				}
				kind := claudeToolKind(toolName)
				index := appendItem(HistoryItem{
					ID:           toolUseID,
					SourceItemID: nilIfEmptyHistory(toolUseID),
					Kind:         "tool",
					ItemType:     kind,
					Timestamp:    entryTimestamp,
					ObservedAt:   entryTimestamp,
					Payload:      cloneMap(entry),
					Tool: &HistoryTool{
						ID:     toolUseID,
						Name:   claudeToolDisplayName(toolName, kind),
						Kind:   kind,
						Input:  claudeToolInput(toolName, toolInput),
						Status: "running",
						Meta:   claudeToolMeta(toolName, kind, claudeToolInput(toolName, toolInput)),
					},
				})
				pendingTools[toolUseID] = index
			}
		case "user":
			if entry["isMeta"] == true {
				continue
			}
			message := decodeRawObject(entry["message"])
			if strings.TrimSpace(stringValue(message["role"])) != "user" {
				continue
			}
			switch content := message["content"].(type) {
			case string:
				text := strings.TrimSpace(content)
				if normalizeWorkflowMode(workflowMode) == WorkflowModePlan {
					text = stripClaudePlanPreamble(text)
				}
				if text == "" {
					continue
				}
				userMessageID := firstNonEmpty(stringValue(entry["uuid"]), utils.NewID())
				appendItem(HistoryItem{
					ID:           userMessageID,
					SourceItemID: nilIfEmptyHistory(userMessageID),
					Kind:         "user",
					ItemType:     "user_message",
					Text:         text,
					Timestamp:    entryTimestamp,
					ObservedAt:   entryTimestamp,
					Payload:      cloneMap(entry),
				})
				if promptID := strings.TrimSpace(stringValue(entry["promptId"])); promptID != "" {
					turnIDs[promptID] = struct{}{}
				}
			case []any:
				if text := claudeTextBlocks(content); text != "" || len(m.claudeHistoryAttachments(content)) > 0 {
					userMessageID := firstNonEmpty(stringValue(entry["uuid"]), utils.NewID())
					appendItem(HistoryItem{
						ID:           userMessageID,
						SourceItemID: nilIfEmptyHistory(userMessageID),
						Kind:         "user",
						ItemType:     "user_message",
						Text:         text,
						Timestamp:    entryTimestamp,
						ObservedAt:   entryTimestamp,
						Attachments:  m.claudeHistoryAttachments(content),
						Payload:      cloneMap(entry),
					})
					if promptID := strings.TrimSpace(stringValue(entry["promptId"])); promptID != "" {
						turnIDs[promptID] = struct{}{}
					}
				}
				for _, rawBlock := range content {
					block := decodeRawObject(rawBlock)
					if strings.TrimSpace(stringValue(block["type"])) != "tool_result" {
						continue
					}
					toolUseID := strings.TrimSpace(stringValue(block["tool_use_id"]))
					isError := block["is_error"] == true
					contentText := strings.TrimSpace(claudeToolResultContentText(block["content"]))
					if contentText == "" {
						contentText = claudeToolUseResultSummary(entry["toolUseResult"])
					}
					if questions, ok := pendingAskQuestions[toolUseID]; ok {
						delete(pendingAskQuestions, toolUseID)
						item := HistoryItem{
							ID:         firstNonEmpty(stringValue(entry["uuid"]), utils.NewID()),
							Kind:       "system",
							ItemType:   "user_input_response",
							Text:       firstNonEmpty(contentText, "Submitted requested input"),
							Timestamp:  entryTimestamp,
							ObservedAt: entryTimestamp,
							Level:      "info",
							Payload:    cloneMap(entry),
						}
						if isError {
							item.Level = "warn"
							item.Text = firstNonEmpty(contentText, "User input request failed")
						} else if answers := decodeClaudeAskUserAnswers(contentText, questions); len(answers) > 0 {
							item.Detail = &HistoryDetail{
								Type:    "user_input_response",
								Answers: answers,
							}
						}
						appendItem(item)
						continue
					}

					if index, ok := pendingTools[toolUseID]; ok && index >= 0 && index < len(result.Items) {
						item := result.Items[index]
						item.ObservedAt = entryTimestamp
						item.Done = true
						item.Payload = cloneMap(entry)
						if item.Tool == nil {
							item.Tool = &HistoryTool{
								ID:     toolUseID,
								Name:   "DynamicToolCall",
								Kind:   "dynamic_tool_call",
								Status: "done",
							}
						}
						item.Tool.Output = truncateToolOutput(item.Tool.Kind, contentText)
						if isError {
							item.Tool.Status = "error"
						} else {
							item.Tool.Status = "done"
						}
						result.Items[index] = item
						delete(pendingTools, toolUseID)
					}
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return claudeSessionParseResult{}, err
	}
	result.TurnCount = len(turnIDs)
	return result, nil
}

func (m *Manager) syncClaudeSessionFromSource(
	ctx context.Context,
	session tables.WebSessionTable,
	force bool,
	clearExisting bool,
) (SessionSnapshot, error) {
	filePath := strings.TrimSpace("")
	if session.ThreadPath != nil {
		filePath = strings.TrimSpace(*session.ThreadPath)
	}
	if filePath == "" && session.NativeSessionID != nil {
		resolved, err := claudeSessionFilePath(session.Cwd, strings.TrimSpace(*session.NativeSessionID))
		if err != nil {
			return SessionSnapshot{}, err
		}
		filePath = resolved
	}
	if filePath == "" {
		return SessionSnapshot{}, fmt.Errorf("session has no claude session file")
	}
	if _, err := os.Stat(filePath); err != nil {
		return SessionSnapshot{}, err
	}

	parsed, err := m.parseClaudeStreamHistory(filePath, effectiveWorkflowMode(session))
	if err != nil {
		return SessionSnapshot{}, err
	}

	itemRows := make([]tables.WebSessionItemTable, 0, len(parsed.Items))
	for _, item := range parsed.Items {
		row := tables.WebSessionItemTable{}
		row.Init()
		applyHistoryItemToRow(&row, session.ID, item)
		itemRows = append(itemRows, row)
	}

	updates := map[string]any{
		"source_kind":       sourceKindClaudeStreamJSON,
		"source_created_at": parsed.StartedAt,
		"source_updated_at": parsed.UpdatedAt,
		"last_synced_at":    time.Now(),
		"sync_state":        SyncStateFresh,
		"sync_error":        nil,
		"last_sync_mode":    string(SyncModeFast),
		"thread_path":       nilIfEmpty(filePath),
		"thread_preview":    nilIfEmpty(parsed.LastPrompt),
		"turn_count":        parsed.TurnCount,
		"item_count":        len(itemRows),
		"last_event_seq":    0,
		"updated_at":        time.Now(),
	}
	if parsed.SessionID != "" {
		updates["native_session_id"] = parsed.SessionID
	}
	if force {
		if parsed.LatestMessageAt != nil {
			updates["activity_at"] = *parsed.LatestMessageAt
			updates["last_message_at"] = *parsed.LatestMessageAt
		} else if parsed.UpdatedAt != nil {
			updates["activity_at"] = *parsed.UpdatedAt
		}
	}
	if err := m.store.deleteSessionFiles(session.ID); err != nil {
		return SessionSnapshot{}, err
	}
	if err := m.replaceSessionHistoryCache(ctx, session, nil, itemRows, updates); err != nil {
		return SessionSnapshot{}, err
	}
	refreshed, err := m.GetSession(ctx, session.ID)
	if err != nil {
		return SessionSnapshot{}, err
	}
	return m.loadSnapshotLocal(ctx, refreshed, DefaultHistoryWindow, false)
}

func (m *Manager) findClaudePendingUserInputRequest(
	ctx context.Context,
	sessionID string,
	itemID string,
) (*pendingServerRequest, error) {
	db := model.GetDB()
	if db == nil {
		return nil, model.ErrDBNotInitialized
	}
	var row tables.WebSessionItemTable
	if err := db.WithContext(ctx).
		Where("web_session_id = ? AND source_item_id = ? AND item_type = ?", sessionID, strings.TrimSpace(itemID), "user_input_request").
		First(&row).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("no pending user input request")
		}
		return nil, err
	}
	item := mapHistoryItemRowWithSession(row, sessionID)
	if item.Detail == nil || len(item.Detail.Questions) == 0 {
		return nil, fmt.Errorf("pending user input request is missing structured questions")
	}
	return &pendingServerRequest{
		Kind:      pendingServerRequestUserInput,
		ItemID:    strings.TrimSpace(itemID),
		Prompt:    firstNonEmpty(item.Detail.Prompt, item.Text),
		Questions: item.Detail.Questions,
	}, nil
}

func (m *Manager) findClaudePendingApprovalRequest(
	ctx context.Context,
	sessionID string,
) (*pendingServerRequest, error) {
	db := model.GetDB()
	if db == nil {
		return nil, model.ErrDBNotInitialized
	}
	var row tables.WebSessionItemTable
	if err := db.WithContext(ctx).
		Where("web_session_id = ? AND item_type = ?", sessionID, "approval_request").
		Order("order_index DESC").
		First(&row).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("no pending approval")
		}
		return nil, err
	}
	item := mapHistoryItemRowWithSession(row, sessionID)
	payload := cloneMap(item.Payload)
	itemID := strings.TrimSpace(stringValue(payload["iid"]))
	if itemID == "" {
		return nil, fmt.Errorf("approval request is missing tool id")
	}
	return &pendingServerRequest{
		Kind:   pendingServerRequestPlanApproval,
		ItemID: itemID,
		Prompt: firstNonEmpty(strings.TrimSpace(stringValue(payload["prompt"])), item.Text),
	}, nil
}

func (m *Manager) startClaudeDeferredResume(
	ctx context.Context,
	record tables.WebSessionTable,
	pending *pendingServerRequest,
) error {
	if m.hasActiveRun(record.ID) {
		return fmt.Errorf("session is already running")
	}
	runID := utils.NewID()
	now := time.Now()
	if _, err := m.appendAndBroadcast(ctx, record.ID, record, Event{
		ID:        utils.NewID(),
		Seq:       0,
		Type:      "run_st",
		RunID:     runID,
		Timestamp: now,
		Payload: map[string]any{
			"ag": string(normalizeAgent(Agent(record.Agent))),
			"md": record.Model,
			"re": record.ReasoningEffort,
			"wm": effectiveWorkflowMode(record),
			"pl": effectivePermissionLevel(record),
		},
	}); err != nil {
		return err
	}

	if err := model.GetDB().WithContext(ctx).Model(&tables.WebSessionTable{}).
		Where("id = ?", record.ID).
		Updates(applyAssistantStateUpdates(map[string]any{
			"status":                     string(StatusRunning),
			"has_unread":                 false,
			"last_error":                 nil,
			"auto_retry_last_error_code": nil,
			"updated_at":                 now,
		}, AssistantStateWorking, now)).Error; err != nil {
		return err
	}
	m.broadcastSessionSummary(ctx, record.ID)

	runCtx, cancel := context.WithCancel(context.Background())
	run := &activeRun{
		sessionID:         record.ID,
		agent:             AgentClaude,
		backend:           effectiveSessionBackend(record),
		runID:             runID,
		cancel:            cancel,
		done:              make(chan struct{}),
		claudeResumeOnly:  true,
		deferredUserInput: pending != nil && pending.Kind == pendingServerRequestUserInput,
	}
	run.setPendingServerRequest(pending)

	m.mu.Lock()
	m.runs[record.ID] = run
	m.mu.Unlock()

	go m.runSession(runCtx, run, record, "", nil)
	return nil
}
