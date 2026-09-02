package websession

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"code-kanban/model"
	"code-kanban/model/tables"

	"go.uber.org/zap"
)

func TestPiRuntimeFakeProcess(t *testing.T) {
	if os.Getenv("CODEKANBAN_FAKE_PI_RUNTIME") != "1" {
		return
	}
	args := argsAfterDoubleDash(os.Args)
	if containsString(args, "--version") {
		fmt.Println("0.84.1")
		return
	}
	noSession := containsString(args, "--no-session")
	bridgePath := fakePiFlagValue(args, "--extension")
	sessionPath := fakePiFlagValue(args, "--session")
	if sessionPath == "" {
		sessionPath = os.Getenv("CODEKANBAN_FAKE_PI_SESSION")
	}
	sessionID := "fake-pi-session"
	if restoredID := readFakePiSessionHeaderID(sessionPath); restoredID != "" {
		sessionID = restoredID
	}
	cwd, _ := os.Getwd()
	if !noSession {
		appendFakePiLog(map[string]any{"startup": os.Getpid(), "session": true, "args": args})
	}

	modelProvider := "openai"
	modelID := "gpt-fake"
	thinkingLevel := "medium"
	entries, leafID := readFakePiSessionEntries(sessionPath)
	pr5Active := false
	pr5InputAnswered := false
	pr5Steered := false
	pr5FollowedUp := false
	pr5Finished := false
	entrySequence := len(entries)
	mutationSequence := 0
	appendNativeEntry := func(entry map[string]any) {
		parentID := leafID
		entrySequence++
		leafID = "leaf-" + strconv.Itoa(entrySequence)
		entry["id"] = leafID
		entry["parentId"] = nilIfEmpty(parentID)
		entry["timestamp"] = time.Now().UTC().Format(time.RFC3339Nano)
		entries = append(entries, entry)
		file, _ := os.OpenFile(sessionPath, os.O_APPEND|os.O_WRONLY, 0o600)
		if file != nil {
			encoded, _ := json.Marshal(entry)
			_, _ = fmt.Fprintln(file, string(encoded))
			_ = file.Close()
		}
	}
	appendEntry := func(role, text string) {
		appendNativeEntry(map[string]any{
			"type":    "message",
			"message": map[string]any{"role": role, "content": []any{map[string]any{"type": "text", "text": text}}},
		})
	}
	finishPR5 := func() {
		if pr5Finished || !pr5Active || !pr5InputAnswered || !pr5Steered || !pr5FollowedUp {
			return
		}
		pr5Finished = true
		writeFakePiEvent(map[string]any{"type": "auto_retry_start", "attempt": 1, "maxAttempts": 3, "delayMs": 1, "errorMessage": "retryable"})
		writeFakePiEvent(map[string]any{"type": "auto_retry_end", "success": true, "attempt": 1})
		writeFakePiEvent(map[string]any{"type": "compaction_start", "reason": "threshold"})
		writeFakePiEvent(map[string]any{"type": "compaction_end", "result": map[string]any{"summary": "compacted context"}, "aborted": false})
		writeFakePiEvent(map[string]any{"type": "tool_execution_update", "toolCallId": "parallel-b", "toolName": "Read", "partialResult": map[string]any{"content": []any{map[string]any{"type": "text", "text": "B partial"}}}})
		writeFakePiEvent(map[string]any{"type": "tool_execution_end", "toolCallId": "parallel-b", "toolName": "Read", "result": map[string]any{"content": []any{map[string]any{"type": "text", "text": "B done"}}}, "isError": false})
		writeFakePiEvent(map[string]any{"type": "tool_execution_end", "toolCallId": "parallel-a", "toolName": "Write", "result": map[string]any{"content": []any{map[string]any{"type": "text", "text": "A done"}}}, "isError": false})
		writeFakePiEvent(map[string]any{"type": "message_update", "assistantMessageEvent": map[string]any{"type": "text_delta", "contentIndex": 1, "delta": "stale delta"}})
		writeFakePiEvent(map[string]any{"type": "message_end", "message": map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "thinking", "thinking": "final reasoning"}, map[string]any{"type": "text", "text": "authoritative reply"}}}})
		writeFakePiEvent(map[string]any{"type": "queue_update", "steering": []string{}, "followUp": []string{}})
		appendEntry("assistant", "authoritative reply")
		writeFakePiEvent(map[string]any{"type": "agent_end", "messages": []any{}})
		writeFakePiEvent(map[string]any{"type": "agent_settled"})
	}
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var command map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &command); err != nil {
			return
		}
		appendFakePiLog(command)
		id := command["id"]
		kind, _ := command["type"].(string)
		if noSession {
			data := map[string]any{}
			if kind == "get_available_models" {
				data["models"] = []any{map[string]any{
					"provider": "openai", "id": "gpt-test", "name": "GPT Test",
					"reasoning": true, "input": []string{"text", "image"},
					"contextWindow": 32000, "maxTokens": 4096,
				}}
			}
			writeFakePiResponse(id, kind, data)
			continue
		}
		switch kind {
		case "get_state":
			finishPR5()
			stateSessionPath := sessionPath
			if mutationSequence > 0 && os.Getenv("CODEKANBAN_FAKE_PI_INVALID_MUTATION_STATE") == "1" {
				stateSessionPath = ""
			}
			writeFakePiResponse(id, kind, map[string]any{
				"sessionId": sessionID, "sessionFile": stateSessionPath,
				"model":         map[string]any{"provider": modelProvider, "id": modelID, "name": "Fake"},
				"thinkingLevel": thinkingLevel, "isStreaming": false,
			})
		case "get_commands":
			writeFakePiResponse(id, kind, map[string]any{"commands": []any{map[string]any{
				"name": piBridgeCommandName, "source": "extension",
				"sourceInfo": map[string]any{
					"path": bridgePath, "source": "cli", "scope": "temporary", "origin": "top-level",
				},
			}}})
		case "get_entries":
			var leaf any
			if leafID != "" {
				leaf = leafID
			}
			writeFakePiResponse(id, kind, map[string]any{"entries": entries, "leafId": leaf})
		case "get_tree":
			var leaf any
			if leafID != "" {
				leaf = leafID
			}
			writeFakePiResponse(id, kind, map[string]any{"tree": buildFakePiTree(entries), "leafId": leaf})
		case "get_session_stats":
			writeFakePiResponse(id, kind, map[string]any{
				"sessionId": sessionID, "sessionFile": sessionPath,
				"tokens":       map[string]any{"input": 10, "output": 5, "cacheRead": 2, "cacheWrite": 0, "total": 17},
				"cost":         0.01,
				"contextUsage": map[string]any{"tokens": 17, "contextWindow": 32000, "percent": 0.1},
			})
		case "fork", "clone":
			targetLeafID := leafID
			selectedText := ""
			if kind == "fork" {
				entryID, _ := command["entryId"].(string)
				target := findFakePiEntry(entries, entryID)
				if target == nil || fakePiEntryRole(target) != "user" {
					writeFakePiError(id, kind, "invalid fork target")
					continue
				}
				targetLeafID = fakePiParentID(target)
				selectedText = fakePiEntryText(target)
			}
			if kind == "clone" && targetLeafID == "" {
				writeFakePiError(id, kind, "cannot clone empty session")
				continue
			}
			mutationSequence++
			newPath, newID, newEntries, newLeaf, err := createFakePiBranchedSession(
				sessionPath, sessionID, cwd, entries, targetLeafID, mutationSequence,
			)
			if err != nil {
				writeFakePiError(id, kind, err.Error())
				continue
			}
			sessionPath, sessionID, entries, leafID = newPath, newID, newEntries, newLeaf
			entrySequence = len(entries)
			if kind == "fork" {
				writeFakePiResponse(id, kind, map[string]any{"text": selectedText, "cancelled": false})
			} else {
				writeFakePiResponse(id, kind, map[string]any{"cancelled": false})
			}
		case "set_model":
			modelProvider, _ = command["provider"].(string)
			modelID, _ = command["modelId"].(string)
			writeFakePiResponse(id, kind, map[string]any{"provider": modelProvider, "id": modelID})
		case "set_thinking_level":
			thinkingLevel, _ = command["level"].(string)
			writeFakePiResponse(id, kind, nil)
		case "prompt":
			if err := ensureFakePiSessionFile(sessionPath, sessionID, cwd); err != nil {
				writeFakePiError(id, kind, "failed to persist fake session")
				continue
			}
			if delay, err := time.ParseDuration(os.Getenv("CODEKANBAN_FAKE_PI_PROMPT_ACK_DELAY")); err == nil && delay > 0 {
				time.Sleep(delay)
			}
			writeFakePiResponse(id, kind, nil)
			message, _ := command["message"].(string)
			if payload, ok := parseFakePiBridgeCommand(message); ok {
				target := findFakePiEntry(entries, payload.TargetID)
				if target == nil {
					continue
				}
				targetType, _ := target["type"].(string)
				targetParent, _ := target["parentId"].(string)
				leafID = payload.TargetID
				if targetType == "custom_message" || fakePiEntryRole(target) == "user" {
					leafID = targetParent
				}
				if payload.Summarize {
					appendNativeEntry(map[string]any{"type": "branch_summary", "summary": "fake branch summary"})
				}
				appendNativeEntry(map[string]any{
					"type": "custom", "customType": piBridgeMarkerType,
					"data": map[string]any{"targetId": payload.TargetID, "summarize": payload.Summarize, "nonce": payload.Nonce},
				})
				continue
			}
			if message == "pr6-tree-seed" {
				appendEntry("user", message)
				branchParent := leafID
				appendEntry("assistant", "abandoned branch")
				leafID = branchParent
				appendEntry("assistant", "active branch")
				writeFakePiEvent(map[string]any{"type": "agent_start"})
				writeFakePiEvent(map[string]any{"type": "message_start", "message": map[string]any{"role": "assistant", "content": []any{}}})
				writeFakePiEvent(map[string]any{"type": "message_update", "assistantMessageEvent": map[string]any{"type": "text_delta", "contentIndex": 0, "delta": "active branch"}})
				writeFakePiEvent(map[string]any{"type": "message_end", "message": map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "text", "text": "active branch"}}}})
				writeFakePiEvent(map[string]any{"type": "agent_end", "messages": []any{}})
				writeFakePiEvent(map[string]any{"type": "agent_settled"})
				continue
			}
			if message == "hold" {
				continue
			}
			if message == "pr5-events" {
				pr5Active = true
				appendEntry("user", message)
				writeFakePiEvent(map[string]any{"type": "agent_start"})
				writeFakePiEvent(map[string]any{"type": "message_start", "message": map[string]any{"role": "assistant", "content": []any{}}})
				writeFakePiEvent(map[string]any{"type": "message_update", "assistantMessageEvent": map[string]any{"type": "thinking_start", "contentIndex": 0}})
				writeFakePiEvent(map[string]any{"type": "message_update", "assistantMessageEvent": map[string]any{"type": "thinking_delta", "contentIndex": 0, "delta": "live reasoning"}})
				writeFakePiEvent(map[string]any{"type": "message_update", "assistantMessageEvent": map[string]any{"type": "thinking_end", "contentIndex": 0, "content": "stale reasoning"}})
				writeFakePiEvent(map[string]any{"type": "tool_execution_start", "toolCallId": "parallel-a", "toolName": "Write", "args": map[string]any{"path": "a.txt"}})
				writeFakePiEvent(map[string]any{"type": "tool_execution_start", "toolCallId": "parallel-b", "toolName": "Read", "args": map[string]any{"path": "b.txt"}})
				writeFakePiEvent(map[string]any{"type": "extension_ui_request", "id": "confirm-1", "method": "confirm", "title": "Confirm action", "message": "Proceed?", "timeout": 5000})
				continue
			}
			if message == "timeout-confirm" {
				appendEntry("user", message)
				writeFakePiEvent(map[string]any{"type": "agent_start"})
				writeFakePiEvent(map[string]any{"type": "message_start", "message": map[string]any{"role": "assistant", "content": []any{}}})
				writeFakePiEvent(map[string]any{"type": "extension_ui_request", "id": "timeout-confirm-1", "method": "confirm", "title": "Timed confirmation", "message": "Respond before timeout", "timeout": 20})
				continue
			}
			if message == "settle-with-dialog" {
				appendEntry("user", message)
				writeFakePiEvent(map[string]any{"type": "agent_start"})
				writeFakePiEvent(map[string]any{"type": "message_start", "message": map[string]any{"role": "assistant", "content": []any{}}})
				writeFakePiEvent(map[string]any{"type": "extension_ui_request", "id": "settle-dialog-1", "method": "confirm", "title": "Unanswered confirmation", "message": "Pi settled early"})
				writeFakePiEvent(map[string]any{"type": "agent_settled"})
				continue
			}
			if message == "hold-dialog" {
				appendEntry("user", message)
				writeFakePiEvent(map[string]any{"type": "agent_start"})
				writeFakePiEvent(map[string]any{"type": "message_start", "message": map[string]any{"role": "assistant", "content": []any{}}})
				writeFakePiEvent(map[string]any{"type": "extension_ui_request", "id": "abort-dialog-1", "method": "confirm", "title": "Abort confirmation", "message": "Waiting for abort"})
				continue
			}
			appendEntry("assistant", "fake reply")
			writeFakePiEvent(map[string]any{"type": "agent_start"})
			writeFakePiEvent(map[string]any{"type": "message_start", "message": map[string]any{"role": "assistant", "content": []any{}}})
			writeFakePiEvent(map[string]any{"type": "message_update", "assistantMessageEvent": map[string]any{"type": "text_delta", "delta": "fake reply"}})
			writeFakePiEvent(map[string]any{"type": "message_end", "message": map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "text", "text": "fake reply"}}}})
			writeFakePiEvent(map[string]any{"type": "agent_end", "messages": []any{}})
			writeFakePiEvent(map[string]any{"type": "agent_settled"})
		case "compact":
			writeFakePiEvent(map[string]any{"type": "compaction_start", "reason": "manual"})
			appendNativeEntry(map[string]any{
				"type": "compaction", "summary": "manual compact summary", "tokensBefore": 17,
				"firstKeptEntryId": nil, "retainedTail": []any{},
			})
			result := map[string]any{"summary": "manual compact summary", "tokensBefore": 17}
			writeFakePiEvent(map[string]any{"type": "compaction_end", "result": result, "aborted": false})
			writeFakePiResponse(id, kind, result)
		case "steer", "follow_up":
			message, _ := command["message"].(string)
			appendEntry("user", message)
			if kind == "steer" {
				pr5Steered = true
			} else {
				pr5FollowedUp = true
			}
			steering := []string{}
			followUp := []string{}
			if pr5Steered {
				steering = append(steering, "redirect while active")
			}
			if pr5FollowedUp {
				followUp = append(followUp, "queue while active")
			}
			writeFakePiEvent(map[string]any{"type": "queue_update", "steering": steering, "followUp": followUp})
			writeFakePiResponse(id, kind, nil)
		case "extension_ui_response":
			requestID, _ := command["id"].(string)
			switch requestID {
			case "confirm-1":
				writeFakePiEvent(map[string]any{"type": "extension_ui_request", "id": "input-1", "method": "input", "title": "Provide value", "placeholder": "value", "timeout": 5000})
			case "input-1":
				pr5InputAnswered = true
				finishPR5()
			case "timeout-confirm-1":
				appendEntry("assistant", "continued after timeout")
				writeFakePiEvent(map[string]any{"type": "message_update", "assistantMessageEvent": map[string]any{"type": "text_delta", "contentIndex": 0, "delta": "continued after timeout"}})
				writeFakePiEvent(map[string]any{"type": "message_end", "message": map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "text", "text": "continued after timeout"}}}})
				writeFakePiEvent(map[string]any{"type": "agent_end", "messages": []any{}})
				writeFakePiEvent(map[string]any{"type": "agent_settled"})
			}
		case "abort":
			writeFakePiResponse(id, kind, nil)
			writeFakePiEvent(map[string]any{"type": "agent_settled"})
		default:
			writeFakePiError(id, kind, "unknown command")
		}
	}
}

func readFakePiSessionHeaderID(sessionPath string) string {
	file, err := os.Open(sessionPath)
	if err != nil {
		return ""
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		return ""
	}
	var header struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(scanner.Bytes(), &header) != nil {
		return ""
	}
	return strings.TrimSpace(header.ID)
}

func readFakePiSessionEntries(sessionPath string) ([]map[string]any, string) {
	file, err := os.Open(sessionPath)
	if err != nil {
		return nil, ""
	}
	defer file.Close()
	entries := make([]map[string]any, 0)
	leafID := ""
	scanner := bufio.NewScanner(file)
	lineIndex := 0
	for scanner.Scan() {
		lineIndex++
		if lineIndex == 1 {
			continue
		}
		entry := map[string]any{}
		if json.Unmarshal(scanner.Bytes(), &entry) != nil {
			continue
		}
		id, _ := entry["id"].(string)
		if strings.TrimSpace(id) == "" {
			continue
		}
		entries = append(entries, entry)
		leafID = id
	}
	return entries, leafID
}

func buildFakePiTree(entries []map[string]any) []any {
	nodes := make(map[string]map[string]any, len(entries))
	order := make([]string, 0, len(entries))
	for _, entry := range entries {
		id, _ := entry["id"].(string)
		if strings.TrimSpace(id) == "" {
			continue
		}
		nodes[id] = map[string]any{"entry": entry, "children": []any{}}
		order = append(order, id)
	}
	roots := make([]any, 0, 1)
	for _, id := range order {
		entry := nodes[id]["entry"].(map[string]any)
		parentID := stringValue(entry["parentId"])
		if pointer, ok := entry["parentId"].(*string); ok {
			parentID = pointerString(pointer)
		}
		parent := nodes[parentID]
		if parentID == "" || parent == nil || parentID == id {
			roots = append(roots, nodes[id])
			continue
		}
		children := parent["children"].([]any)
		parent["children"] = append(children, nodes[id])
	}
	return roots
}

func parseFakePiBridgeCommand(message string) (piBridgeMarkerData, bool) {
	prefix := "/" + piBridgeCommandName + " "
	if !strings.HasPrefix(message, prefix) {
		return piBridgeMarkerData{}, false
	}
	encoded := strings.TrimSpace(strings.TrimPrefix(message, prefix))
	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return piBridgeMarkerData{}, false
	}
	var payload piBridgeMarkerData
	if json.Unmarshal(decoded, &payload) != nil || strings.TrimSpace(payload.TargetID) == "" || strings.TrimSpace(payload.Nonce) == "" {
		return piBridgeMarkerData{}, false
	}
	return payload, true
}

func findFakePiEntry(entries []map[string]any, id string) map[string]any {
	for _, entry := range entries {
		if entry["id"] == id {
			return entry
		}
	}
	return nil
}

func fakePiEntryRole(entry map[string]any) string {
	message, _ := entry["message"].(map[string]any)
	role, _ := message["role"].(string)
	return role
}

func fakePiParentID(entry map[string]any) string {
	if entry == nil {
		return ""
	}
	if parent, ok := entry["parentId"].(*string); ok {
		return pointerString(parent)
	}
	return stringValue(entry["parentId"])
}

func fakePiEntryText(entry map[string]any) string {
	message, _ := entry["message"].(map[string]any)
	content := message["content"]
	if text, ok := content.(string); ok {
		return text
	}
	parts, _ := content.([]any)
	var builder strings.Builder
	for _, partValue := range parts {
		part, _ := partValue.(map[string]any)
		if part["type"] == "text" {
			builder.WriteString(stringValue(part["text"]))
		}
	}
	return builder.String()
}

func createFakePiBranchedSession(
	sourcePath string,
	sourceID string,
	cwd string,
	entries []map[string]any,
	targetLeafID string,
	sequence int,
) (string, string, []map[string]any, string, error) {
	byID := make(map[string]map[string]any, len(entries))
	for _, entry := range entries {
		id := stringValue(entry["id"])
		if id != "" {
			byID[id] = entry
		}
	}
	chain := make([]map[string]any, 0)
	seen := make(map[string]struct{})
	for id := strings.TrimSpace(targetLeafID); id != ""; {
		if _, duplicate := seen[id]; duplicate {
			return "", "", nil, "", errors.New("fake Pi branch contains a cycle")
		}
		seen[id] = struct{}{}
		entry := byID[id]
		if entry == nil {
			return "", "", nil, "", errors.New("fake Pi branch target is missing")
		}
		chain = append(chain, entry)
		id = fakePiParentID(entry)
	}
	for left, right := 0, len(chain)-1; left < right; left, right = left+1, right-1 {
		chain[left], chain[right] = chain[right], chain[left]
	}

	mutationID := make([]byte, 16)
	if _, err := rand.Read(mutationID); err != nil {
		return "", "", nil, "", err
	}
	newID := fmt.Sprintf("%s-branch-%d-%s", sourceID, sequence, hex.EncodeToString(mutationID))
	newPath := filepath.Join(filepath.Dir(sourcePath), newID+".jsonl")
	header := map[string]any{
		"type": "session", "version": 3, "id": newID,
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano), "cwd": cwd,
		"parentSession": sourcePath,
	}
	projected := make([]map[string]any, 0, len(chain))
	lines := make([][]byte, 0, len(chain)+1)
	encodedHeader, _ := json.Marshal(header)
	lines = append(lines, encodedHeader)
	parentID := ""
	for _, original := range chain {
		encoded, _ := json.Marshal(original)
		clone := map[string]any{}
		if err := json.Unmarshal(encoded, &clone); err != nil {
			return "", "", nil, "", err
		}
		clone["parentId"] = nilIfEmpty(parentID)
		parentID = stringValue(clone["id"])
		projected = append(projected, clone)
		encoded, _ = json.Marshal(clone)
		lines = append(lines, encoded)
	}
	var content strings.Builder
	for _, line := range lines {
		content.Write(line)
		content.WriteByte('\n')
	}
	if err := os.WriteFile(newPath, []byte(content.String()), 0o600); err != nil {
		return "", "", nil, "", err
	}
	return newPath, newID, projected, parentID, nil
}

func ensureFakePiSessionFile(sessionPath, sessionID, cwd string) error {
	if _, err := os.Stat(sessionPath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	header := fmt.Sprintf("{\"type\":\"session\",\"version\":3,\"id\":%q,\"timestamp\":%q,\"cwd\":%q}\n", sessionID, time.Now().UTC().Format(time.RFC3339Nano), cwd)
	return os.WriteFile(sessionPath, []byte(header), 0o600)
}

func TestPiRuntimeValidatesModelAndReasoningControls(t *testing.T) {
	if _, _, err := splitPiModel("anthropic/claude-sonnet-4"); err != nil {
		t.Fatalf("valid Pi model: %v", err)
	}
	if _, _, err := splitPiModel("claude-sonnet-4"); err == nil {
		t.Fatal("model without provider should fail")
	}
	if err := validatePiReasoningEffort(ReasoningEffortMax); err != nil {
		t.Fatalf("max reasoning should be supported: %v", err)
	}
	if err := validatePiReasoningEffort(ReasoningEffortUltra); err == nil {
		t.Fatal("ultra reasoning should be rejected before starting a prompt")
	}
}

func TestResolvePiRuntimeSessionRootUsesReportedStandardLayout(t *testing.T) {
	t.Setenv("PI_CODING_AGENT_SESSION_DIR", t.TempDir())
	actualRoot := filepath.Join(t.TempDir(), "agent", "sessions")
	sessionFile := filepath.Join(actualRoot, "--portable-project--", "session.jsonl")

	got, err := resolvePiRuntimeSessionRoot(sessionFile)
	if err != nil {
		t.Fatalf("resolvePiRuntimeSessionRoot returned error: %v", err)
	}
	if !samePiRuntimePath(got, actualRoot) {
		t.Fatalf("session root = %q, want %q", got, actualRoot)
	}

	invalidFile := filepath.Join(t.TempDir(), "project", "session.jsonl")
	if _, err := resolvePiRuntimeSessionRoot(invalidFile); err == nil || !strings.Contains(err.Error(), "outside the configured session root") {
		t.Fatalf("expected non-standard fallback path rejection, got %v", err)
	}
}

func TestValidatePiRuntimeStateRejectsSessionOutsideConfiguredRoot(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.jsonl")
	projectPath := t.TempDir()
	if err := os.WriteFile(outside, []byte(fmt.Sprintf("{\"type\":\"session\",\"id\":\"native-outside\",\"cwd\":%q}\n", filepath.ToSlash(projectPath))), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PI_CODING_AGENT_SESSION_DIR", root)
	err := validatePiRuntimeState(tables.WebSessionTable{Cwd: projectPath}, piRPCState{
		SessionID:   "native-outside",
		SessionFile: outside,
	})
	if err == nil || !strings.Contains(err.Error(), "outside the configured session root") {
		t.Fatalf("expected session-root rejection, got %v", err)
	}
}

func TestManagerImportPiSessionValidatesNativeIdentity(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()
	project := seedProject(t)
	sessionRoot := t.TempDir()
	t.Setenv("PI_CODING_AGENT_SESSION_DIR", sessionRoot)
	sessionID := "imported-pi-session"
	sessionPath := filepath.Join(sessionRoot, "imported.jsonl")
	header := fmt.Sprintf("{\"type\":\"session\",\"version\":3,\"id\":%q,\"timestamp\":%q,\"cwd\":%q}\n", sessionID, time.Now().UTC().Format(time.RFC3339Nano), project.Path)
	content := header +
		"{\"type\":\"message\",\"id\":\"u1\",\"parentId\":null,\"timestamp\":\"2026-05-01T01:00:00Z\",\"message\":{\"role\":\"user\",\"content\":\"active prompt\"}}\n" +
		"{\"type\":\"message\",\"id\":\"a1\",\"parentId\":\"u1\",\"timestamp\":\"2026-05-01T01:00:01Z\",\"message\":{\"role\":\"assistant\",\"content\":[{\"type\":\"text\",\"text\":\"active reply\"}]}}\n" +
		"{\"type\":\"message\",\"id\":\"abandoned\",\"parentId\":\"a1\",\"timestamp\":\"2026-05-01T01:00:02Z\",\"message\":{\"role\":\"assistant\",\"content\":[{\"type\":\"text\",\"text\":\"abandoned branch\"}]}}\n" +
		"{\"type\":\"message\",\"id\":\"leaf\",\"parentId\":\"a1\",\"timestamp\":\"2026-05-01T01:00:03Z\",\"message\":{\"role\":\"user\",\"content\":\"active leaf\"}}\n"
	if err := os.WriteFile(sessionPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write Pi session fixture: %v", err)
	}
	info, err := os.Stat(sessionPath)
	if err != nil {
		t.Fatalf("stat Pi session fixture: %v", err)
	}
	source := tables.AISessionTable{
		SessionID: sessionID, Type: tables.AISessionTypePi, ProjectPath: project.Path,
		FilePath: sessionPath, Title: "Imported Pi", Model: "openai/gpt-test",
		SessionStartedAt: info.ModTime(), FileModTime: info.ModTime(), FileSize: info.Size(),
	}
	source.Init()
	if err := model.GetDB().Create(&source).Error; err != nil {
		t.Fatalf("seed Pi history record: %v", err)
	}
	unavailable, err := NewManager(Config{
		DataDir: t.TempDir(), PiPath: filepath.Join(t.TempDir(), "missing-pi"),
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("create unavailable manager: %v", err)
	}
	if _, err := unavailable.ImportPiSessionBySessionID(context.Background(), project.ID, sessionID); err == nil || !strings.Contains(err.Error(), errPiWebSessionUnavailable) {
		t.Fatalf("expected unavailable import rejection, got %v", err)
	}
	var importedCount int64
	if err := model.GetDB().Model(&tables.WebSessionTable{}).
		Where("project_id = ? AND agent = ?", project.ID, string(AgentPi)).Count(&importedCount).Error; err != nil || importedCount != 0 {
		t.Fatalf("unavailable import created %d sessions, err=%v", importedCount, err)
	}

	t.Setenv("CODEKANBAN_FAKE_PI_RUNTIME", "1")
	t.Setenv("CODEKANBAN_FAKE_PI_SESSION", sessionPath)
	manager, err := NewManager(Config{
		DataDir: t.TempDir(), PiPath: fmt.Sprintf("%q -test.run=^TestPiRuntimeFakeProcess$ --", os.Args[0]),
		PiRuntimeIdleTTL: time.Minute,
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	defer manager.StopProjectPiRuntimes(project.ID)
	if _, err := manager.TrustProjectForPi(context.Background(), project.ID); err != nil {
		t.Fatalf("trust project for Pi: %v", err)
	}
	result, err := manager.ImportPiSessionBySessionID(context.Background(), project.ID, sessionID)
	if err != nil {
		t.Fatalf("ImportPiSessionBySessionID returned error: %v", err)
	}
	if !result.Created || result.Session.Agent != AgentPi {
		t.Fatalf("unexpected imported Pi session: %+v", result.Session)
	}
	record, err := manager.GetSession(context.Background(), result.Session.ID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if record.Backend != string(SessionBackendPiRPC) || pointerString(result.Session.NativeSessionID) != sessionID || pointerString(result.Session.ThreadPath) != sessionPath {
		t.Fatalf("unexpected imported native identity: summary=%+v record=%+v", result.Session, record)
	}
	if !result.Synced || pointerString(record.NativeLeafID) != "leaf" || !historyContainsText(result.History, "active prompt") || !historyContainsText(result.History, "active leaf") || historyContainsText(result.History, "abandoned branch") {
		t.Fatalf("unexpected imported Pi projection: synced=%v leaf=%q history=%#v", result.Synced, pointerString(record.NativeLeafID), result.History.Items)
	}
	reused, err := manager.ImportPiSessionBySessionID(context.Background(), project.ID, sessionID)
	if err != nil || !reused.Reused || reused.Session.ID != result.Session.ID {
		t.Fatalf("expected duplicate import to reuse %q, got %+v err=%v", result.Session.ID, reused, err)
	}

	outsidePath := filepath.Join(t.TempDir(), "outside.jsonl")
	outsideID := "outside-pi-session"
	outsideHeader := fmt.Sprintf("{\"type\":\"session\",\"version\":3,\"id\":%q,\"timestamp\":%q,\"cwd\":%q}\n", outsideID, time.Now().UTC().Format(time.RFC3339Nano), project.Path)
	if err := os.WriteFile(outsidePath, []byte(outsideHeader), 0o600); err != nil {
		t.Fatalf("write outside Pi fixture: %v", err)
	}
	outsideInfo, _ := os.Stat(outsidePath)
	outsideSource := tables.AISessionTable{
		SessionID: outsideID, Type: tables.AISessionTypePi, ProjectPath: project.Path,
		FilePath: outsidePath, SessionStartedAt: outsideInfo.ModTime(), FileModTime: outsideInfo.ModTime(), FileSize: outsideInfo.Size(),
	}
	outsideSource.Init()
	if err := model.GetDB().Create(&outsideSource).Error; err != nil {
		t.Fatalf("seed outside Pi history record: %v", err)
	}
	if _, err := manager.ImportPiSessionBySessionID(context.Background(), project.ID, outsideID); err == nil || !strings.Contains(err.Error(), "outside the configured session root") {
		t.Fatalf("expected outside-root import rejection, got %v", err)
	}
}

func TestManagerPiRPCAcceptsStandardSessionRootReportedByLauncher(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()
	project := seedProject(t)
	configuredRoot := t.TempDir()
	reportedRoot := filepath.Join(t.TempDir(), "agent", "sessions")
	sessionPath := filepath.Join(reportedRoot, "--portable-project--", "session.jsonl")
	if err := os.MkdirAll(filepath.Dir(sessionPath), 0o755); err != nil {
		t.Fatalf("create reported session directory: %v", err)
	}
	t.Setenv("CODEKANBAN_FAKE_PI_RUNTIME", "1")
	t.Setenv("CODEKANBAN_FAKE_PI_SESSION", sessionPath)
	t.Setenv("PI_CODING_AGENT_SESSION_DIR", configuredRoot)
	manager, err := NewManager(Config{
		DataDir: t.TempDir(), PiPath: fmt.Sprintf("%q -test.run=^TestPiRuntimeFakeProcess$ --", os.Args[0]),
		PiRuntimeIdleTTL: time.Minute,
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	defer manager.StopProjectPiRuntimes(project.ID)
	if _, err := manager.TrustProjectForPi(context.Background(), project.ID); err != nil {
		t.Fatalf("TrustProjectForPi returned error: %v", err)
	}
	created, err := manager.CreateSession(context.Background(), CreateParams{
		ProjectID: project.ID,
		Agent:     AgentPi,
		Model:     "openai/gpt-test",
	})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	if err := manager.SendMessage(context.Background(), created.ID, "portable launcher", nil); err != nil {
		t.Fatalf("SendMessage returned error: %v", err)
	}
	waitForSessionToSettle(t, manager, created.ID)
	record := mustGetSession(t, manager, created.ID)
	if record.Status != string(StatusDone) || !samePiRuntimePath(pointerString(record.ThreadPath), sessionPath) {
		t.Fatalf("portable session did not complete: status=%q error=%q path=%q", record.Status, pointerString(record.LastError), pointerString(record.ThreadPath))
	}
}

func TestManagerPiRPCSendReusesRuntimeAndPersistsIdentity(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()
	project := seedProject(t)
	fakeDir := t.TempDir()
	sessionPath := filepath.Join(fakeDir, "fake-session.jsonl")
	logPath := filepath.Join(fakeDir, "commands.jsonl")
	t.Setenv("CODEKANBAN_FAKE_PI_RUNTIME", "1")
	t.Setenv("CODEKANBAN_FAKE_PI_SESSION", sessionPath)
	t.Setenv("PI_CODING_AGENT_SESSION_DIR", fakeDir)
	t.Setenv("CODEKANBAN_FAKE_PI_LOG", logPath)
	manager, err := NewManager(Config{
		DataDir:          t.TempDir(),
		PiPath:           fmt.Sprintf("%q -test.run=^TestPiRuntimeFakeProcess$ --", os.Args[0]),
		PiRuntimeIdleTTL: time.Minute,
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	defer manager.StopProjectPiRuntimes(project.ID)
	if _, err := manager.TrustProjectForPi(context.Background(), project.ID); err != nil {
		t.Fatalf("TrustProjectForPi returned error: %v", err)
	}
	var codexProbeCalls atomic.Int32
	var piProbeCalls atomic.Int32
	manager.runtimeCapabilityProbes.codexBinary = func() (CodexRuntimeConfig, error) {
		codexProbeCalls.Add(1)
		return CodexRuntimeConfig{HasCodex: true}, nil
	}
	manager.runtimeCapabilityProbes.pi = func() (piRuntimeProbeResult, error) {
		piProbeCalls.Add(1)
		return piRuntimeProbeResult{installed: true, compatible: true}, nil
	}
	created, err := manager.CreateSession(context.Background(), CreateParams{
		ProjectID: project.ID, Agent: AgentPi, Model: "openai/gpt-test",
		ReasoningEffort: ReasoningEffortHigh,
	})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	image, err := manager.saveAttachmentBytes("pixel.png", "image/png", []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\x00"))
	if err != nil {
		t.Fatalf("save image attachment: %v", err)
	}
	if err := manager.SendMessage(context.Background(), created.ID, "first", []string{image.ID}); err != nil {
		t.Fatalf("SendMessage with image returned error: %v", err)
	}
	waitForSessionToSettle(t, manager, created.ID)
	if codexProbeCalls.Load() != 0 || piProbeCalls.Load() != 0 {
		t.Fatalf("ordinary Pi send invoked capability probes: codex=%d pi=%d", codexProbeCalls.Load(), piProbeCalls.Load())
	}
	if err := manager.SendMessage(context.Background(), created.ID, "second", nil); err != nil {
		t.Fatalf("second SendMessage returned error: %v", err)
	}
	waitForSessionToSettle(t, manager, created.ID)
	manager.StopSessionPiRuntime(created.ID)
	manager.cfg.PiRuntimeIdleTTL = 25 * time.Millisecond
	if err := manager.SendMessage(context.Background(), created.ID, "restored", nil); err != nil {
		t.Fatalf("restored SendMessage returned error: %v", err)
	}
	waitForSessionToSettle(t, manager, created.ID)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		manager.piRuntimeMu.Lock()
		_, active := manager.piRuntimes[created.ID]
		manager.piRuntimeMu.Unlock()
		if !active {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	manager.piRuntimeMu.Lock()
	_, idleRuntimeStillActive := manager.piRuntimes[created.ID]
	manager.piRuntimeMu.Unlock()
	if idleRuntimeStillActive {
		t.Fatal("idle Pi runtime was not evicted after its TTL")
	}

	record, err := manager.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if pointerString(record.NativeSessionID) != "fake-pi-session" || !samePiRuntimePath(pointerString(record.ThreadPath), sessionPath) {
		t.Fatalf("unexpected Pi identity: native=%q path=%q status=%q error=%q", pointerString(record.NativeSessionID), pointerString(record.ThreadPath), record.Status, pointerString(record.LastError))
	}
	if pointerString(record.NativeLeafID) == "" || pointerString(record.SourceRevision) == "" {
		t.Fatalf("missing Pi leaf/revision: leaf=%q revision=%q", pointerString(record.NativeLeafID), pointerString(record.SourceRevision))
	}
	if record.Model != "openai/gpt-test" || record.ReasoningEffort != string(ReasoningEffortHigh) {
		t.Fatalf("model/thinking = %q/%q", record.Model, record.ReasoningEffort)
	}
	window, err := manager.History(context.Background(), created.ID, DefaultHistoryWindow, nil)
	if err != nil {
		t.Fatalf("History returned error: %v", err)
	}
	if !historyContainsText(window, "fake reply") {
		t.Fatalf("history does not contain fake reply: %#v", window.Items)
	}
	commands := readFakePiLog(t, logPath)
	starts := 0
	prompts := 0
	imagePrompt := false
	restoredStart := false
	for _, command := range commands {
		if _, ok := command["startup"]; ok {
			starts++
			if args, ok := command["args"].([]any); ok {
				for _, arg := range args {
					if arg == "--session" {
						restoredStart = true
					}
				}
			}
		}
		if command["type"] == "prompt" {
			prompts++
			if images, ok := command["images"].([]any); ok && len(images) == 1 {
				image, _ := images[0].(map[string]any)
				imagePrompt = image["data"] == "iVBORw0KGgoAAAAA" && image["mimeType"] == "image/png"
			}
		}
	}
	if starts != 2 || prompts != 3 || !imagePrompt || !restoredStart {
		t.Fatalf("runtime starts=%d prompts=%d imagePrompt=%v restoredStart=%v, commands=%#v", starts, prompts, imagePrompt, restoredStart, commands)
	}
}

func TestManagerPiRPCTreeNavigatePersistsAcrossRuntimeRestore(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()
	project := seedProject(t)
	fakeDir := t.TempDir()
	sessionPath := filepath.Join(fakeDir, "tree-session.jsonl")
	logPath := filepath.Join(fakeDir, "tree-commands.jsonl")
	t.Setenv("CODEKANBAN_FAKE_PI_RUNTIME", "1")
	t.Setenv("CODEKANBAN_FAKE_PI_SESSION", sessionPath)
	t.Setenv("PI_CODING_AGENT_SESSION_DIR", fakeDir)
	t.Setenv("CODEKANBAN_FAKE_PI_LOG", logPath)
	manager, err := NewManager(Config{
		DataDir: t.TempDir(), PiPath: fmt.Sprintf("%q -test.run=^TestPiRuntimeFakeProcess$ --", os.Args[0]),
		PiRuntimeIdleTTL: time.Minute,
	}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer manager.StopProjectPiRuntimes(project.ID)
	if _, err := manager.TrustProjectForPi(context.Background(), project.ID); err != nil {
		t.Fatal(err)
	}
	created, err := manager.CreateSession(context.Background(), CreateParams{ProjectID: project.ID, Agent: AgentPi})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.SendMessage(context.Background(), created.ID, "pr6-tree-seed", nil); err != nil {
		t.Fatal(err)
	}
	waitForSessionToSettle(t, manager, created.ID)

	tree, err := manager.GetPiSessionTree(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetPiSessionTree: %v", err)
	}
	if tree.SessionID != "fake-pi-session" || tree.Revision == "" || len(tree.Nodes) != 3 {
		t.Fatalf("unexpected Pi tree snapshot: %#v", tree)
	}
	var userID, abandonedID, activeID string
	for _, node := range tree.Nodes {
		switch node.Preview {
		case "pr6-tree-seed":
			userID = node.ID
		case "abandoned branch":
			abandonedID = node.ID
		case "active branch":
			activeID = node.ID
		}
	}
	if userID == "" || abandonedID == "" || activeID == "" || pointerString(tree.LeafID) != activeID {
		t.Fatalf("tree nodes/leaf mismatch: user=%q abandoned=%q active=%q tree=%#v", userID, abandonedID, activeID, tree)
	}
	originalRevision := tree.Revision
	manager.replacePiNativeQueuedInputs(created.ID, []string{"native queued"}, nil)
	if _, err := manager.GetPiSessionTree(context.Background(), created.ID); err != nil {
		t.Fatalf("read tree with native queued input: %v", err)
	}
	if _, err := manager.NavigatePiSessionTree(context.Background(), created.ID, PiTreeNavigateInput{
		TargetID: abandonedID, Revision: originalRevision,
	}); err == nil || !strings.Contains(err.Error(), "messages are pending") {
		t.Fatalf("expected native queue to block navigation, got %v", err)
	}
	if _, err := manager.ForkPiSessionTree(context.Background(), created.ID, PiTreeForkInput{
		TargetID: userID, Revision: originalRevision,
	}); err == nil || !strings.Contains(err.Error(), "messages are pending") {
		t.Fatalf("expected native queue to block fork, got %v", err)
	}
	if _, err := manager.ClonePiSessionTree(context.Background(), created.ID, PiTreeCloneInput{
		Revision: originalRevision,
	}); err == nil || !strings.Contains(err.Error(), "messages are pending") {
		t.Fatalf("expected native queue to block clone, got %v", err)
	}
	manager.clearPiNativeQueuedInputs(created.ID)

	result, err := manager.NavigatePiSessionTree(context.Background(), created.ID, PiTreeNavigateInput{
		TargetID: abandonedID, Revision: originalRevision,
	})
	if err != nil {
		t.Fatalf("NavigatePiSessionTree: %v", err)
	}
	if pointerString(result.Tree.LeafID) != abandonedID || result.Tree.Revision == originalRevision {
		t.Fatalf("navigation did not advance tree identity: %#v", result)
	}
	window, err := manager.History(context.Background(), created.ID, DefaultHistoryWindow, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !historyContainsText(window, "abandoned branch") || historyContainsText(window, "active branch") {
		t.Fatalf("navigation did not replace the active branch timeline: %#v", window.Items)
	}
	for _, item := range window.Items {
		if strings.Contains(item.Text, piBridgeMarkerType) {
			t.Fatalf("bridge marker leaked into history: %#v", item)
		}
	}
	if _, err := manager.NavigatePiSessionTree(context.Background(), created.ID, PiTreeNavigateInput{
		TargetID: activeID, Revision: originalRevision,
	}); !errors.Is(err, ErrPiTreeRevisionConflict) {
		t.Fatalf("expected stale revision conflict, got %v", err)
	}

	manager.StopSessionPiRuntime(created.ID)
	restored, err := manager.GetPiSessionTree(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetPiSessionTree after restore: %v", err)
	}
	if pointerString(restored.LeafID) != abandonedID || restored.Revision != result.Tree.Revision {
		t.Fatalf("restored tree lost durable logical leaf: before=%#v after=%#v", result.Tree, restored)
	}
	rootResult, err := manager.NavigatePiSessionTree(context.Background(), created.ID, PiTreeNavigateInput{
		TargetID: userID, Revision: restored.Revision,
	})
	if err != nil {
		t.Fatalf("navigate to root user: %v", err)
	}
	if rootResult.EditorText != "pr6-tree-seed" || rootResult.Tree.LeafID != nil {
		t.Fatalf("root-user navigation did not return editor text/reset leaf: %#v", rootResult)
	}
	window, err = manager.History(context.Background(), created.ID, DefaultHistoryWindow, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(window.Items) != 0 {
		t.Fatalf("root-user navigation retained an active timeline: %#v", window.Items)
	}

	commands := readFakePiLog(t, logPath)
	starts, bridgePrompts := 0, 0
	for _, command := range commands {
		if _, ok := command["startup"]; ok {
			starts++
		}
		if command["type"] == "prompt" && strings.HasPrefix(stringValue(command["message"]), "/"+piBridgeCommandName+" ") {
			bridgePrompts++
		}
	}
	if starts != 2 || bridgePrompts != 2 {
		t.Fatalf("unexpected navigate runtime lifecycle: starts=%d bridgePrompts=%d commands=%#v", starts, bridgePrompts, commands)
	}
}

func TestManagerPiRPCTreeWireCommands(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()
	project := seedProject(t)
	fakeDir := t.TempDir()
	sourcePath := filepath.Join(fakeDir, "wire-tree-source.jsonl")
	t.Setenv("CODEKANBAN_FAKE_PI_RUNTIME", "1")
	t.Setenv("CODEKANBAN_FAKE_PI_SESSION", sourcePath)
	t.Setenv("PI_CODING_AGENT_SESSION_DIR", fakeDir)
	manager, err := NewManager(Config{
		DataDir: t.TempDir(), PiPath: fmt.Sprintf("%q -test.run=^TestPiRuntimeFakeProcess$ --", os.Args[0]),
		PiRuntimeIdleTTL: time.Minute,
	}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer manager.StopProjectPiRuntimes(project.ID)
	if _, err := manager.TrustProjectForPi(context.Background(), project.ID); err != nil {
		t.Fatal(err)
	}
	created, err := manager.CreateSession(context.Background(), CreateParams{
		ProjectID: project.ID, Agent: AgentPi, Model: "openai/gpt-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.SendMessage(context.Background(), created.ID, "pr6-tree-seed", nil); err != nil {
		t.Fatal(err)
	}
	waitForSessionToSettle(t, manager, created.ID)

	conn := &captureWSConn{}
	client := manager.RegisterCommandClient(conn)
	defer manager.UnregisterClient(client)
	send := func(requestID, operation, payload string) wireFrame {
		t.Helper()
		conn.frames = nil
		command := fmt.Sprintf(`{"v":1,"k":"cmd","rid":%q,"sid":%q,"op":%q`, requestID, created.ID, operation)
		if payload != "" {
			command += `,"p":` + payload
		}
		command += `}`
		if err := manager.HandleCommand(context.Background(), client, []byte(command)); err != nil {
			t.Fatalf("HandleCommand %s: %v", operation, err)
		}
		if len(conn.frames) != 1 {
			t.Fatalf("%s returned %d frames: %#v", operation, len(conn.frames), conn.frames)
		}
		return conn.frames[0]
	}
	decodePayload := func(frame wireFrame, target any) {
		t.Helper()
		data, err := json.Marshal(frame.Payload)
		if err != nil {
			t.Fatalf("marshal %s payload: %v", frame.Operation, err)
		}
		if err := json.Unmarshal(data, target); err != nil {
			t.Fatalf("decode %s payload: %v; payload=%s", frame.Operation, err, data)
		}
	}

	getFrame := send("tree-get", "tree_get", "")
	if getFrame.Kind != "ack" || getFrame.Operation != "tree_get" || getFrame.SessionID != created.ID {
		t.Fatalf("unexpected tree_get frame: %#v", getFrame)
	}
	var tree PiTreeSnapshot
	decodePayload(getFrame, &tree)
	var userID, abandonedID string
	for _, node := range tree.Nodes {
		switch node.Preview {
		case "pr6-tree-seed":
			userID = node.ID
		case "abandoned branch":
			abandonedID = node.ID
		}
	}
	if tree.Revision == "" || userID == "" || abandonedID == "" {
		t.Fatalf("tree_get omitted required tree state: %#v", tree)
	}

	staleFrame := send("tree-nav-stale", "tree_nav", fmt.Sprintf(`{"tid":%q,"rev":"stale","sum":false}`, abandonedID))
	if staleFrame.Kind != "err" || staleFrame.Code != "conflict" {
		t.Fatalf("stale tree_nav did not return conflict: %#v", staleFrame)
	}
	navFrame := send("tree-nav", "tree_nav", fmt.Sprintf(`{"tid":%q,"rev":%q,"sum":false}`, abandonedID, tree.Revision))
	if navFrame.Kind != "ack" || navFrame.Operation != "tree_nav" {
		t.Fatalf("unexpected tree_nav frame: %#v", navFrame)
	}
	var navigation PiTreeNavigateResult
	decodePayload(navFrame, &navigation)
	if pointerString(navigation.Tree.LeafID) != abandonedID || navigation.Tree.Revision == tree.Revision {
		t.Fatalf("tree_nav did not return the switched tree: %#v", navigation)
	}
	sourceBefore := mustGetSession(t, manager, created.ID)

	type createWireResult struct {
		Session    wireSess       `json:"s"`
		Tree       PiTreeSnapshot `json:"tree"`
		EditorText string         `json:"editorText"`
	}
	forkFrame := send("tree-fork", "tree_fork", fmt.Sprintf(`{"tid":%q,"rev":%q}`, userID, navigation.Tree.Revision))
	if forkFrame.Kind != "ack" || forkFrame.Operation != "tree_fork" {
		t.Fatalf("unexpected tree_fork frame: %#v", forkFrame)
	}
	var forkResult createWireResult
	decodePayload(forkFrame, &forkResult)
	if forkResult.Session.ID == "" || forkResult.Session.ID == created.ID || forkResult.EditorText != "pr6-tree-seed" {
		t.Fatalf("unexpected tree_fork payload: %#v", forkResult)
	}

	cloneFrame := send("tree-clone", "tree_clone", fmt.Sprintf(`{"rev":%q}`, navigation.Tree.Revision))
	if cloneFrame.Kind != "ack" || cloneFrame.Operation != "tree_clone" {
		t.Fatalf("unexpected tree_clone frame: kind=%q op=%q code=%q message=%q", cloneFrame.Kind, cloneFrame.Operation, cloneFrame.Code, cloneFrame.Message)
	}
	var cloneResult createWireResult
	decodePayload(cloneFrame, &cloneResult)
	if cloneResult.Session.ID == "" || cloneResult.Session.ID == created.ID || cloneResult.Session.ID == forkResult.Session.ID || cloneResult.EditorText != "" {
		t.Fatalf("unexpected tree_clone payload: %#v", cloneResult)
	}

	sourceAfter := mustGetSession(t, manager, created.ID)
	if pointerString(sourceAfter.NativeSessionID) != pointerString(sourceBefore.NativeSessionID) ||
		!samePiRuntimePath(pointerString(sourceAfter.ThreadPath), pointerString(sourceBefore.ThreadPath)) ||
		pointerString(sourceAfter.NativeLeafID) != pointerString(sourceBefore.NativeLeafID) ||
		pointerString(sourceAfter.SourceRevision) != pointerString(sourceBefore.SourceRevision) {
		t.Fatalf("wire tree mutations changed source identity: before=%#v after=%#v", sourceBefore, sourceAfter)
	}
}

func TestManagerPiRPCTreeMutationFailureKeepsSourceAndCreatesNoTarget(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()
	project := seedProject(t)
	fakeDir := t.TempDir()
	sourcePath := filepath.Join(fakeDir, "failed-mutation-source.jsonl")
	logPath := filepath.Join(fakeDir, "failed-mutation-commands.jsonl")
	t.Setenv("CODEKANBAN_FAKE_PI_RUNTIME", "1")
	t.Setenv("CODEKANBAN_FAKE_PI_SESSION", sourcePath)
	t.Setenv("CODEKANBAN_FAKE_PI_INVALID_MUTATION_STATE", "1")
	t.Setenv("PI_CODING_AGENT_SESSION_DIR", fakeDir)
	t.Setenv("CODEKANBAN_FAKE_PI_LOG", logPath)
	manager, err := NewManager(Config{
		DataDir: t.TempDir(), PiPath: fmt.Sprintf("%q -test.run=^TestPiRuntimeFakeProcess$ --", os.Args[0]),
		PiRuntimeIdleTTL: time.Minute,
	}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer manager.StopProjectPiRuntimes(project.ID)
	if _, err := manager.TrustProjectForPi(context.Background(), project.ID); err != nil {
		t.Fatal(err)
	}
	created, err := manager.CreateSession(context.Background(), CreateParams{
		ProjectID: project.ID, Agent: AgentPi, Model: "openai/gpt-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.SendMessage(context.Background(), created.ID, "pr6-tree-seed", nil); err != nil {
		t.Fatal(err)
	}
	waitForSessionToSettle(t, manager, created.ID)
	tree, err := manager.GetPiSessionTree(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	sourceBefore := mustGetSession(t, manager, created.ID)
	historyBefore, err := manager.History(context.Background(), created.ID, DefaultHistoryWindow, nil)
	if err != nil {
		t.Fatal(err)
	}

	_, err = manager.ClonePiSessionTree(context.Background(), created.ID, PiTreeCloneInput{Revision: tree.Revision})
	if err == nil || !strings.Contains(err.Error(), "incomplete session identity") {
		t.Fatalf("expected post-mutation identity failure, got %v", err)
	}
	sourceAfter := mustGetSession(t, manager, created.ID)
	if pointerString(sourceAfter.NativeSessionID) != pointerString(sourceBefore.NativeSessionID) ||
		!samePiRuntimePath(pointerString(sourceAfter.ThreadPath), pointerString(sourceBefore.ThreadPath)) ||
		pointerString(sourceAfter.SourceRevision) != pointerString(sourceBefore.SourceRevision) {
		t.Fatalf("failed mutation changed source identity: before=%#v after=%#v", sourceBefore, sourceAfter)
	}
	var sessionCount, turnCount, itemCount int64
	db := model.GetDB()
	if err := db.Model(&tables.WebSessionTable{}).Where("project_id = ? AND agent = ?", project.ID, string(AgentPi)).Count(&sessionCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&tables.WebSessionTurnTable{}).Where("web_session_id <> ?", created.ID).Count(&turnCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&tables.WebSessionItemTable{}).Where("web_session_id <> ?", created.ID).Count(&itemCount).Error; err != nil {
		t.Fatal(err)
	}
	if sessionCount != 1 || turnCount != 0 || itemCount != 0 {
		t.Fatalf("failed mutation left target rows: sessions=%d turns=%d items=%d", sessionCount, turnCount, itemCount)
	}
	historyAfter, err := manager.History(context.Background(), created.ID, DefaultHistoryWindow, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(historyAfter.Items) != len(historyBefore.Items) || !historyContainsText(historyAfter, "active branch") {
		t.Fatalf("failed mutation changed source history: before=%#v after=%#v", historyBefore.Items, historyAfter.Items)
	}
	manager.piRuntimeMu.Lock()
	_, runtimePresent := manager.piRuntimes[created.ID]
	manager.piRuntimeMu.Unlock()
	if runtimePresent {
		t.Fatal("failed mutation retained the switched Pi runtime")
	}

	t.Setenv("CODEKANBAN_FAKE_PI_INVALID_MUTATION_STATE", "0")
	if _, err := manager.GetPiSessionTree(context.Background(), created.ID); err != nil {
		t.Fatalf("restore source after failed mutation: %v", err)
	}
	starts := 0
	for _, command := range readFakePiLog(t, logPath) {
		if _, startup := command["startup"]; startup {
			starts++
		}
	}
	if starts != 2 {
		t.Fatalf("expected source runtime restore after failed mutation, starts=%d", starts)
	}
}

func TestManagerPiRPCProjectsPR5EventsAndNativePendingInputs(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()
	project := seedProject(t)
	fakeDir := t.TempDir()
	sessionPath := filepath.Join(fakeDir, "pr5-session.jsonl")
	logPath := filepath.Join(fakeDir, "pr5-commands.jsonl")
	t.Setenv("CODEKANBAN_FAKE_PI_RUNTIME", "1")
	t.Setenv("CODEKANBAN_FAKE_PI_SESSION", sessionPath)
	t.Setenv("PI_CODING_AGENT_SESSION_DIR", fakeDir)
	t.Setenv("CODEKANBAN_FAKE_PI_LOG", logPath)
	manager, err := NewManager(Config{
		DataDir: t.TempDir(), PiPath: fmt.Sprintf("%q -test.run=^TestPiRuntimeFakeProcess$ --", os.Args[0]),
		PiRuntimeIdleTTL: time.Minute,
	}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer manager.StopProjectPiRuntimes(project.ID)
	if _, err := manager.TrustProjectForPi(context.Background(), project.ID); err != nil {
		t.Fatal(err)
	}
	if !manager.GetWebSessionRuntimeConfig().SupportsPiWebSession {
		t.Fatal("fake Pi runtime should support Web Sessions")
	}
	created, err := manager.CreateSession(context.Background(), CreateParams{ProjectID: project.ID, Agent: AgentPi})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.SendMessage(context.Background(), created.ID, "pr5-events", nil); err != nil {
		t.Fatal(err)
	}
	approval := waitForPendingServerRequest(t, manager, created.ID, pendingServerRequestToolApproval)
	if approval == nil || approval.PiRequestID != "confirm-1" {
		t.Fatalf("unexpected Pi approval: %#v", approval)
	}
	snapshot, err := manager.loadSnapshotLocal(context.Background(), mustGetSession(t, manager, created.ID), DefaultHistoryWindow)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.PendingApproval == nil || !snapshot.PendingApproval.Actionable {
		t.Fatalf("expected actionable Pi approval, got %#v", snapshot.PendingApproval)
	}
	if err := manager.sendMessageWithMode(context.Background(), created.ID, "redirect while active", nil, PendingInputModeRedirect, "pi-steer"); err != nil {
		t.Fatal(err)
	}
	if err := manager.sendMessageWithMode(context.Background(), created.ID, "queue while active", nil, PendingInputModeQueue, "pi-follow-up"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	for _, command := range readFakePiLog(t, logPath) {
		if command["type"] == "steer" || command["type"] == "follow_up" {
			t.Fatalf("queued input crossed pending Pi dialog: %#v", command)
		}
	}
	if err := manager.respondToApproval(created.ID, "approve"); err != nil {
		t.Fatalf("respondToApproval: %v", err)
	}
	input := waitForPendingServerRequest(t, manager, created.ID, pendingServerRequestUserInput)
	if input == nil || input.PiRequestID != "input-1" {
		t.Fatalf("unexpected Pi input request: %#v", input)
	}
	if err := manager.respondToUserInput(created.ID, input.ItemID, map[string][]string{"value": {"typed value"}}); err != nil {
		t.Fatalf("respondToUserInput: %v", err)
	}
	waitForPiNativeQueue(t, manager, created.ID, 2)
	manager.piRuntimeMu.Lock()
	managerRuntime := manager.piRuntimes[created.ID]
	manager.piRuntimeMu.Unlock()
	if managerRuntime == nil {
		t.Fatal("expected active Pi runtime")
	}
	var state piRPCState
	if err := managerRuntime.client.Request(context.Background(), "get_state", nil, &state); err != nil {
		t.Fatalf("release fake Pi queued continuation: %v", err)
	}
	waitForSessionToSettle(t, manager, created.ID)
	if queued := manager.pendingInputsDisplaySnapshot(created.ID); len(queued) != 0 {
		t.Fatalf("native Pi queue was not cleared after settle: %#v", queued)
	}

	events, err := manager.store.readEvents(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	texts := userMessageTexts(events)
	if !containsString(texts, "redirect while active") || !containsString(texts, "queue while active") {
		t.Fatalf("queued Pi messages were not projected: %#v", texts)
	}
	var authoritative, stale bool
	toolEnds := map[string]string{}
	runDoneIndex := -1
	lastProjectionIndex := -1
	for index, event := range events {
		switch event.Type {
		case "txt_end":
			text := stringValue(event.Payload["txt"])
			authoritative = authoritative || text == "authoritative reply"
			stale = stale || text == "stale delta"
			lastProjectionIndex = index
		case "tool_end":
			toolEnds[stringValue(event.Payload["tid"])] = stringValue(event.Payload["out"])
			lastProjectionIndex = index
		case "note":
			code := stringValue(event.Payload["code"])
			if strings.HasPrefix(code, "pi_auto_retry") || strings.HasPrefix(code, "pi_compaction") {
				lastProjectionIndex = index
			}
		case "run_done":
			runDoneIndex = index
		}
	}
	if !authoritative || stale {
		t.Fatalf("Pi message_end did not authoritatively calibrate text: authoritative=%v staleEnd=%v", authoritative, stale)
	}
	if toolEnds["parallel-a"] != "A done" || toolEnds["parallel-b"] != "B done" {
		t.Fatalf("parallel Pi tools overwrote each other: %#v", toolEnds)
	}
	if runDoneIndex <= lastProjectionIndex {
		record := mustGetSession(t, manager, created.ID)
		var failures []map[string]any
		for _, event := range events {
			if event.Type == "run_fail" {
				failures = append(failures, event.Payload)
			}
		}
		t.Fatalf("run_done arrived before projection settled: runDone=%d lastProjection=%d status=%q lastError=%q failures=%#v", runDoneIndex, lastProjectionIndex, record.Status, pointerString(record.LastError), failures)
	}
	window, err := manager.History(context.Background(), created.ID, DefaultHistoryWindow, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !historyContainsText(window, "authoritative reply") || historyContainsText(window, "stale delta") {
		t.Fatalf("unexpected authoritative Pi history: %#v", window.Items)
	}
	if !historyContainsToolOutput(window, "parallel-a", "A done") || !historyContainsToolOutput(window, "parallel-b", "B done") {
		t.Fatalf("missing independent Pi tool projections: %#v", window.Items)
	}
	nativeMessages := 0
	preservedLiveItems := 0
	finalReasoning := false
	for _, item := range window.Items {
		if (item.Kind == "user" || item.Kind == "assistant") && pointerString(item.SourceThreadID) == "fake-pi-session" && pointerString(item.SourceItemID) != "" {
			nativeMessages++
		}
		if item.Kind == "tool" || item.Kind == "system" {
			preservedLiveItems++
		}
		if item.Tool != nil && item.Tool.Kind == "reasoning" {
			finalReasoning = finalReasoning || item.Tool.Output == "final reasoning"
			if item.Tool.Output == "stale reasoning" {
				t.Fatalf("Pi message_end did not authoritatively calibrate thinking: %#v", item)
			}
		}
	}
	if nativeMessages != 4 || preservedLiveItems < 5 || !finalReasoning {
		t.Fatalf("Pi incremental sync lost native/live items: nativeMessages=%d preservedLiveItems=%d finalReasoning=%v items=%#v", nativeMessages, preservedLiveItems, finalReasoning, window.Items)
	}
	var turns []tables.WebSessionTurnTable
	if err := model.GetDB().Where("web_session_id = ?", created.ID).Find(&turns).Error; err != nil {
		t.Fatal(err)
	}
	if len(turns) != 3 {
		t.Fatalf("expected three native Pi turns, got %d: %#v", len(turns), turns)
	}
	for _, turn := range turns {
		if pointerString(turn.SourceThreadID) != "fake-pi-session" || pointerString(turn.SourceTurnID) == "" {
			t.Fatalf("Pi turn missing native identity: %#v", turn)
		}
	}
	var liveRows []tables.WebSessionItemTable
	if err := model.GetDB().Where("web_session_id = ? AND item_kind IN ?", created.ID, []string{"tool", "system"}).Find(&liveRows).Error; err != nil {
		t.Fatal(err)
	}
	for _, row := range liveRows {
		if pointerString(row.WebTurnID) == "" || pointerString(row.SourceThreadID) != "fake-pi-session" || pointerString(row.SourceTurnID) == "" {
			t.Fatalf("Pi live item missing native turn binding: %#v", row)
		}
	}
	record := mustGetSession(t, manager, created.ID)
	if pointerString(record.NativeLeafID) == "" || pointerString(record.SourceRevision) == "" || record.ItemCount != len(window.Items) {
		t.Fatalf("Pi incremental sync identity/count mismatch: leaf=%q revision=%q itemCount=%d history=%d", pointerString(record.NativeLeafID), pointerString(record.SourceRevision), record.ItemCount, len(window.Items))
	}
	commands := readFakePiLog(t, logPath)
	sequence := make([]string, 0)
	for _, command := range commands {
		kind, _ := command["type"].(string)
		if kind == "extension_ui_response" || kind == "steer" || kind == "follow_up" {
			sequence = append(sequence, kind+":"+stringValue(command["id"]))
		}
	}
	if len(sequence) != 4 || !strings.HasPrefix(sequence[0], "extension_ui_response:confirm-1") ||
		!strings.HasPrefix(sequence[1], "extension_ui_response:input-1") ||
		!strings.HasPrefix(sequence[2], "steer:") || !strings.HasPrefix(sequence[3], "follow_up:") {
		t.Fatalf("unexpected Pi interaction sequence: %#v", sequence)
	}
}

func TestManagerPiRPCExtensionConfirmTimeoutCancelsApproval(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()
	project := seedProject(t)
	fakeDir := t.TempDir()
	logPath := filepath.Join(fakeDir, "timeout-commands.jsonl")
	t.Setenv("CODEKANBAN_FAKE_PI_RUNTIME", "1")
	t.Setenv("CODEKANBAN_FAKE_PI_SESSION", filepath.Join(fakeDir, "timeout-session.jsonl"))
	t.Setenv("PI_CODING_AGENT_SESSION_DIR", fakeDir)
	t.Setenv("CODEKANBAN_FAKE_PI_LOG", logPath)
	manager, err := NewManager(Config{
		DataDir: t.TempDir(), PiPath: fmt.Sprintf("%q -test.run=^TestPiRuntimeFakeProcess$ --", os.Args[0]),
		PiRuntimeIdleTTL: time.Minute,
	}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer manager.StopProjectPiRuntimes(project.ID)
	if _, err := manager.TrustProjectForPi(context.Background(), project.ID); err != nil {
		t.Fatal(err)
	}
	created, err := manager.CreateSession(context.Background(), CreateParams{ProjectID: project.ID, Agent: AgentPi})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.SendMessage(context.Background(), created.ID, "timeout-confirm", nil); err != nil {
		t.Fatal(err)
	}
	waitForActiveRun(t, manager, created.ID)
	waitForSessionToSettle(t, manager, created.ID)

	events, err := manager.store.readEvents(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	approvalCancelled := false
	for _, event := range events {
		if event.Type == "user_input_res" {
			t.Fatalf("confirm timeout was projected as user input: %#v", event)
		}
		if event.Type == "approval_res" && stringValue(event.Payload["act"]) == "cancel" {
			approvalCancelled = true
		}
	}
	if !approvalCancelled || !historyHasEvent(events, "run_done") {
		t.Fatalf("confirm timeout did not close approval and settle: %#v", events)
	}
	commands := readFakePiLog(t, logPath)
	cancelResponses := 0
	for _, command := range commands {
		if command["type"] == "extension_ui_response" && command["id"] == "timeout-confirm-1" && command["cancelled"] == true {
			cancelResponses++
		}
	}
	if cancelResponses != 1 {
		t.Fatalf("expected one cancelled Pi timeout response, got %d: %#v", cancelResponses, commands)
	}
}

func TestManagerPiRPCSettledAndAbortClosePendingDialogs(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		prompt    string
		requestID string
		terminal  string
		abort     bool
	}{
		{name: "settled", prompt: "settle-with-dialog", requestID: "settle-dialog-1", terminal: "run_done"},
		{name: "abort", prompt: "hold-dialog", requestID: "abort-dialog-1", terminal: "run_abort", abort: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			cleanup := initTestDB(t)
			defer cleanup()
			project := seedProject(t)
			fakeDir := t.TempDir()
			logPath := filepath.Join(fakeDir, testCase.name+"-commands.jsonl")
			t.Setenv("CODEKANBAN_FAKE_PI_RUNTIME", "1")
			t.Setenv("CODEKANBAN_FAKE_PI_SESSION", filepath.Join(fakeDir, testCase.name+"-session.jsonl"))
			t.Setenv("PI_CODING_AGENT_SESSION_DIR", fakeDir)
			t.Setenv("CODEKANBAN_FAKE_PI_LOG", logPath)
			manager, err := NewManager(Config{
				DataDir: t.TempDir(), PiPath: fmt.Sprintf("%q -test.run=^TestPiRuntimeFakeProcess$ --", os.Args[0]),
				PiRuntimeIdleTTL: time.Minute,
			}, zap.NewNop())
			if err != nil {
				t.Fatal(err)
			}
			defer manager.StopProjectPiRuntimes(project.ID)
			if _, err := manager.TrustProjectForPi(context.Background(), project.ID); err != nil {
				t.Fatal(err)
			}
			created, err := manager.CreateSession(context.Background(), CreateParams{ProjectID: project.ID, Agent: AgentPi})
			if err != nil {
				t.Fatal(err)
			}
			if err := manager.SendMessage(context.Background(), created.ID, testCase.prompt, nil); err != nil {
				t.Fatal(err)
			}
			if testCase.abort {
				pending := waitForPendingServerRequest(t, manager, created.ID, pendingServerRequestToolApproval)
				if pending.PiRequestID != testCase.requestID {
					t.Fatalf("unexpected Pi dialog: %#v", pending)
				}
				if err := manager.AbortSession(created.ID); err != nil {
					t.Fatal(err)
				}
			}
			waitForSessionToSettle(t, manager, created.ID)

			events, err := manager.store.readEvents(created.ID)
			if err != nil {
				t.Fatal(err)
			}
			completionIndex, terminalIndex := -1, -1
			for index, event := range events {
				if event.Type == "approval_res" && stringValue(event.Payload["act"]) == "cancel" {
					completionIndex = index
				}
				if event.Type == testCase.terminal {
					terminalIndex = index
				}
			}
			if completionIndex < 0 || terminalIndex <= completionIndex {
				t.Fatalf("Pi dialog was not closed before %s: %#v", testCase.terminal, events)
			}
			if snapshot, err := manager.Snapshot(context.Background(), created.ID, DefaultHistoryWindow); err != nil {
				t.Fatal(err)
			} else if snapshot.PendingApproval != nil || snapshot.PendingUserInput != nil {
				t.Fatalf("settled Pi dialog remained actionable: %#v", snapshot)
			}
			for _, command := range readFakePiLog(t, logPath) {
				if command["type"] == "extension_ui_response" && command["id"] == testCase.requestID {
					t.Fatalf("terminal cleanup wrote a stale Pi extension response: %#v", command)
				}
			}
		})
	}
}

func TestManagerPiRPCManualCompactionUsesNativeRuntime(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()
	project := seedProject(t)
	fakeDir := t.TempDir()
	sessionPath := filepath.Join(fakeDir, "compact-session.jsonl")
	logPath := filepath.Join(fakeDir, "compact-commands.jsonl")
	t.Setenv("CODEKANBAN_FAKE_PI_RUNTIME", "1")
	t.Setenv("CODEKANBAN_FAKE_PI_SESSION", sessionPath)
	t.Setenv("PI_CODING_AGENT_SESSION_DIR", fakeDir)
	t.Setenv("CODEKANBAN_FAKE_PI_LOG", logPath)
	manager, err := NewManager(Config{
		DataDir: t.TempDir(), PiPath: fmt.Sprintf("%q -test.run=^TestPiRuntimeFakeProcess$ --", os.Args[0]),
		PiRuntimeIdleTTL: time.Minute,
	}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer manager.StopProjectPiRuntimes(project.ID)
	if _, err := manager.TrustProjectForPi(context.Background(), project.ID); err != nil {
		t.Fatal(err)
	}
	created, err := manager.CreateSession(context.Background(), CreateParams{ProjectID: project.ID, Agent: AgentPi})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.SendMessage(context.Background(), created.ID, "before compact", nil); err != nil {
		t.Fatal(err)
	}
	waitForSessionToSettle(t, manager, created.ID)
	before, err := manager.History(context.Background(), created.ID, DefaultHistoryWindow, nil)
	if err != nil {
		t.Fatal(err)
	}
	beforeUserCount := historyKindCount(before, "user")
	beforeLeaf := pointerString(mustGetSession(t, manager, created.ID).NativeLeafID)

	if err := manager.CompactSession(context.Background(), created.ID); err != nil {
		t.Fatal(err)
	}
	waitForSessionToSettle(t, manager, created.ID)
	after, err := manager.History(context.Background(), created.ID, DefaultHistoryWindow, nil)
	if err != nil {
		t.Fatal(err)
	}
	if historyKindCount(after, "user") != beforeUserCount {
		t.Fatalf("manual Pi compaction added a user message: before=%d after=%d", beforeUserCount, historyKindCount(after, "user"))
	}
	compactions := 0
	for _, item := range after.Items {
		if item.Tool != nil && item.Tool.Kind == "context_compaction" {
			compactions++
			if !item.Done || item.Tool.Output != "manual compact summary" {
				t.Fatalf("unexpected manual compaction item: %#v", item)
			}
		}
	}
	if compactions != 1 {
		t.Fatalf("expected one manual compaction item, got %d: %#v", compactions, after.Items)
	}
	record := mustGetSession(t, manager, created.ID)
	if pointerString(record.NativeLeafID) == "" || pointerString(record.NativeLeafID) == beforeLeaf || pointerString(record.SourceRevision) == "" {
		t.Fatalf("manual compaction did not refresh native identity: before=%q after=%q revision=%q", beforeLeaf, pointerString(record.NativeLeafID), pointerString(record.SourceRevision))
	}
	if record.LastContextCompactionAt == nil || record.ContextBaselineInputTokens != 10 || record.ContextBaselineCachedInputTokens != 2 || record.ContextBaselineOutputTokens != 5 {
		t.Fatalf("manual compaction did not reset the Pi context baseline: %#v", record)
	}
	if record.LatestTokenCountUpdatedAt != nil || record.LatestTurnUsageUpdatedAt != nil {
		t.Fatalf("manual compaction retained a higher-priority context estimate: %#v", record)
	}
	snapshot, err := manager.Snapshot(context.Background(), created.ID, DefaultHistoryWindow)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Session.ContextWindowTokens == nil || *snapshot.Session.ContextWindowTokens != 32000 || snapshot.Session.ContextWindowSource != ContextWindowSourceSessionUsage {
		t.Fatalf("manual compaction lost the Pi session context window: %#v", snapshot.Session)
	}
	if snapshot.Session.ContextEstimateMode != ContextEstimateModeSinceCompaction || snapshot.Session.ContextEstimate.UsedTokens != 0 {
		t.Fatalf("manual compaction did not expose the reset context baseline: %#v", snapshot.Session)
	}
	starts, compacts := 0, 0
	for _, command := range readFakePiLog(t, logPath) {
		if _, ok := command["startup"]; ok {
			starts++
		}
		if command["type"] == "compact" {
			compacts++
		}
	}
	if starts != 1 || compacts != 1 {
		t.Fatalf("manual compaction did not reuse the Pi runtime: starts=%d compacts=%d", starts, compacts)
	}
}

func TestManagerPiRPCAbortKeepsSessionUsable(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()
	project := seedProject(t)
	fakeDir := t.TempDir()
	logPath := filepath.Join(fakeDir, "abort-commands.jsonl")
	t.Setenv("CODEKANBAN_FAKE_PI_RUNTIME", "1")
	t.Setenv("CODEKANBAN_FAKE_PI_PROMPT_ACK_DELAY", "100ms")
	t.Setenv("CODEKANBAN_FAKE_PI_SESSION", filepath.Join(fakeDir, "abort-session.jsonl"))
	t.Setenv("PI_CODING_AGENT_SESSION_DIR", fakeDir)
	t.Setenv("CODEKANBAN_FAKE_PI_LOG", logPath)
	manager, err := NewManager(Config{
		DataDir: t.TempDir(), PiPath: fmt.Sprintf("%q -test.run=^TestPiRuntimeFakeProcess$ --", os.Args[0]),
		PiRuntimeIdleTTL: time.Minute,
	}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer manager.StopProjectPiRuntimes(project.ID)
	if _, err := manager.TrustProjectForPi(context.Background(), project.ID); err != nil {
		t.Fatal(err)
	}
	config := manager.GetWebSessionRuntimeConfig()
	if !config.SupportsPiWebSession {
		t.Fatalf("fake Pi runtime unavailable: hasPi=%v version=%v compatible=%v diagnostics=%q", config.HasPi, config.PiVersion, config.PiRPCCompatible, config.PiDiagnostics)
	}
	created, err := manager.CreateSession(context.Background(), CreateParams{ProjectID: project.ID, Agent: AgentPi})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.SendMessage(context.Background(), created.ID, "before hold", nil); err != nil {
		t.Fatal(err)
	}
	waitForSessionToSettle(t, manager, created.ID)
	if err := manager.SendMessage(context.Background(), created.ID, "hold", nil); err != nil {
		t.Fatal(err)
	}
	waitForActiveRun(t, manager, created.ID)
	tree, err := manager.GetPiSessionTree(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("read Pi tree during active run: %v", err)
	}
	if tree.SessionID != "fake-pi-session" || tree.Revision == "" {
		t.Fatalf("unexpected active-run Pi tree: %#v", tree)
	}
	starts := 0
	for _, command := range readFakePiLog(t, logPath) {
		if _, startup := command["startup"]; startup {
			starts++
		}
	}
	if starts != 1 {
		t.Fatalf("active-run tree read started another Pi runtime: starts=%d", starts)
	}
	if err := manager.AbortSession(created.ID); err != nil {
		t.Fatalf("AbortSession returned error: %v", err)
	}
	waitForSessionToSettle(t, manager, created.ID)
	if err := manager.SendMessage(context.Background(), created.ID, "after abort", nil); err != nil {
		t.Fatalf("send after abort returned error: %v", err)
	}
	waitForSessionToSettle(t, manager, created.ID)
}

func waitForActiveRun(t *testing.T, manager *Manager, sessionID string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if manager.hasActiveRun(sessionID) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("session %s did not start a run", sessionID)
}

func TestPiPromptImagesRevalidatesAndEncodesAttachments(t *testing.T) {
	store, err := newStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := &Manager{store: store, cfg: Config{AttachmentSizeLimit: 1024}}
	path := filepath.Join(store.attachmentsDir, "pixel.png")
	if err := os.WriteFile(path, []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\x00"), 0o600); err != nil {
		t.Fatal(err)
	}
	images, err := manager.piPromptImages([]Attachment{{Name: "pixel.png", Mime: "image/png", Path: path}})
	if err != nil {
		t.Fatalf("piPromptImages returned error: %v", err)
	}
	if len(images) != 1 || images[0].Data != "iVBORw0KGgoAAAAA" || images[0].MimeType != "image/png" {
		t.Fatalf("unexpected images: %#v", images)
	}
	if _, err := manager.piPromptImages([]Attachment{{Name: "notes.txt", Mime: "text/plain", Path: path}}); err == nil {
		t.Fatal("expected non-image attachment rejection")
	}
	outside := filepath.Join(t.TempDir(), "outside.png")
	if err := os.WriteFile(outside, []byte("\x89PNG\r\n\x1a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.piPromptImages([]Attachment{{Name: "outside.png", Mime: "image/png", Path: outside}}); err == nil {
		t.Fatal("expected attachment-root rejection")
	}
}

func argsAfterDoubleDash(args []string) []string {
	for index, value := range args {
		if value == "--" {
			return args[index+1:]
		}
	}
	return args
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func fakePiFlagValue(args []string, flag string) string {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == flag {
			return args[index+1]
		}
	}
	return ""
}

func writeFakePiResponse(id any, command string, data any) {
	writeFakePiEvent(map[string]any{"type": "response", "id": id, "command": command, "success": true, "data": data})
}

func writeFakePiError(id any, command, message string) {
	writeFakePiEvent(map[string]any{"type": "response", "id": id, "command": command, "success": false, "error": message})
}

func writeFakePiEvent(value map[string]any) {
	encoded, _ := json.Marshal(value)
	_, _ = os.Stdout.Write(append(encoded, '\n'))
}

func appendFakePiLog(value map[string]any) {
	path := os.Getenv("CODEKANBAN_FAKE_PI_LOG")
	if path == "" {
		return
	}
	encoded, _ := json.Marshal(value)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err == nil {
		_, _ = file.Write(append(encoded, '\n'))
		_ = file.Close()
	}
}

func readFakePiLog(t *testing.T, path string) []map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var result []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var value map[string]any
		if err := json.Unmarshal([]byte(line), &value); err != nil {
			t.Fatalf("decode fake Pi log: %v", err)
		}
		result = append(result, value)
	}
	return result
}

func mustGetSession(t *testing.T, manager *Manager, sessionID string) tables.WebSessionTable {
	t.Helper()
	record, err := manager.GetSession(context.Background(), sessionID)
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func waitForPiNativeQueue(t *testing.T, manager *Manager, sessionID string, count int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		items := manager.pendingInputsDisplaySnapshot(sessionID)
		if len(items) == count {
			allNative := true
			for _, item := range items {
				if !item.NativeQueued {
					allNative = false
					break
				}
			}
			if allNative && len(manager.pendingInputsSnapshot(sessionID)) == 0 {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("native Pi queue did not reach %d items: %#v", count, manager.pendingInputsDisplaySnapshot(sessionID))
}

func historyKindCount(window HistoryWindow, kind string) int {
	count := 0
	for _, item := range window.Items {
		if item.Kind == kind {
			count++
		}
	}
	return count
}

func historyContainsToolOutput(window HistoryWindow, toolID, output string) bool {
	for _, item := range window.Items {
		if item.Tool != nil && item.Tool.ID == toolID && strings.Contains(item.Tool.Output, output) {
			return true
		}
	}
	return false
}

func historyContainsText(window HistoryWindow, expected string) bool {
	for _, item := range window.Items {
		if strings.Contains(item.Text, expected) {
			return true
		}
	}
	return false
}
