package websession

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"code-kanban/model/tables"
	"code-kanban/utils"
)

type claudeHookAnswerFile struct {
	Answers            map[string]string `json:"answers,omitempty"`
	PermissionDecision string            `json:"permissionDecision,omitempty"`
}

type claudePreToolUseRequest struct {
	SessionID string         `json:"session_id"`
	ToolUseID string         `json:"tool_use_id"`
	ToolName  string         `json:"tool_name"`
	ToolInput map[string]any `json:"tool_input"`
}

type claudePreToolUseResponse struct {
	HookSpecificOutput claudePreToolUseHookSpecificOutput `json:"hookSpecificOutput"`
}

type claudePreToolUseHookSpecificOutput struct {
	HookEventName            string         `json:"hookEventName"`
	PermissionDecision       string         `json:"permissionDecision,omitempty"`
	PermissionDecisionReason string         `json:"permissionDecisionReason,omitempty"`
	UpdatedInput             map[string]any `json:"updatedInput,omitempty"`
}

func claudeAskUserUpdatedInput(
	toolInput map[string]any,
	questions []toolRequestQuestion,
	answers map[string][]string,
) map[string]any {
	updated := cloneMap(toolInput)
	if updated == nil {
		updated = map[string]any{}
	}
	answerMap := make(map[string]string)
	for _, question := range questions {
		questionID := strings.TrimSpace(question.ID)
		values := answers[questionID]
		if len(values) == 0 {
			continue
		}
		key := strings.TrimSpace(firstNonEmpty(question.Question, question.Header, questionID))
		if key == "" {
			continue
		}
		answerMap[key] = strings.Join(values, ", ")
	}
	updated["answers"] = answerMap
	return updated
}

func (m *Manager) writeClaudeHookAnswer(
	sessionID string,
	toolUseID string,
	payload claudeHookAnswerFile,
) error {
	if m.store == nil {
		return fmt.Errorf("store is not configured")
	}
	if err := os.MkdirAll(m.store.claudeHookDir(sessionID), 0o755); err != nil {
		return err
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return os.WriteFile(m.store.claudeHookAnswerPath(sessionID, toolUseID), encoded, 0o644)
}

func (m *Manager) writeClaudeHookAnswerForSession(
	session tables.WebSessionTable,
	toolUseID string,
	payload claudeHookAnswerFile,
) error {
	if err := m.writeClaudeHookAnswer(session.ID, toolUseID, payload); err != nil {
		return err
	}
	nativeSessionID := ""
	if session.NativeSessionID != nil {
		nativeSessionID = strings.TrimSpace(*session.NativeSessionID)
	}
	if nativeSessionID == "" || nativeSessionID == strings.TrimSpace(session.ID) {
		return nil
	}
	return m.writeClaudeHookAnswer(nativeSessionID, toolUseID, payload)
}

func (m *Manager) readClaudeHookAnswer(sessionID, toolUseID string) (claudeHookAnswerFile, error) {
	data, err := os.ReadFile(m.store.claudeHookAnswerPath(sessionID, toolUseID))
	if err != nil {
		return claudeHookAnswerFile{}, err
	}
	var payload claudeHookAnswerFile
	if err := json.Unmarshal(data, &payload); err != nil {
		return claudeHookAnswerFile{}, err
	}
	return payload, nil
}

func (m *Manager) deleteClaudeHookAnswer(sessionID, toolUseID string) {
	if m.store == nil {
		return
	}
	_ = os.Remove(m.store.claudeHookAnswerPath(sessionID, toolUseID))
}

func (m *Manager) deleteClaudeHookAnswerForSession(session tables.WebSessionTable, toolUseID string) {
	m.deleteClaudeHookAnswer(session.ID, toolUseID)
	if session.NativeSessionID == nil {
		return
	}
	nativeSessionID := strings.TrimSpace(*session.NativeSessionID)
	if nativeSessionID == "" || nativeSessionID == strings.TrimSpace(session.ID) {
		return
	}
	m.deleteClaudeHookAnswer(nativeSessionID, toolUseID)
}

func (m *Manager) ensureClaudeHookServer() (string, error) {
	m.claudeHookOnce.Do(func() {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			m.claudeHookErr = err
			return
		}
		m.claudeHookToken = utils.NewID()
		mux := http.NewServeMux()
		mux.HandleFunc("/claude-hooks/pre-tool-use", m.handleClaudePreToolUseHook)
		server := &http.Server{Handler: mux}
		m.claudeHookServer = server
		m.claudeHookBaseURL = "http://" + listener.Addr().String()
		m.claudeHookSettingsPath = filepath.Join(m.store.rootDir, "claude-hook-settings.json")

		settings := m.claudeHookSettings()
		encoded, err := json.Marshal(settings)
		if err != nil {
			_ = listener.Close()
			m.claudeHookErr = err
			return
		}
		if err := os.WriteFile(m.claudeHookSettingsPath, encoded, 0o644); err != nil {
			_ = listener.Close()
			m.claudeHookErr = err
			return
		}
		go func() {
			_ = server.Serve(listener)
		}()
	})
	if m.claudeHookErr != nil {
		return "", m.claudeHookErr
	}
	return m.claudeHookSettingsPath, nil
}

func (m *Manager) ensureCCRClaudeHookSettings() error {
	if settingsPath, err := m.ensureClaudeHookServer(); err != nil {
		return err
	} else if strings.TrimSpace(settingsPath) == "" {
		return fmt.Errorf("claude hook settings path is not configured")
	}
	m.ccrHookMu.Lock()
	defer m.ccrHookMu.Unlock()
	if m.ccrHookReady {
		return nil
	}
	m.ccrHookErr = m.writeCCRClaudeHookShim()
	if m.ccrHookErr == nil {
		m.ccrHookReady = true
	}
	return m.ccrHookErr
}

func (m *Manager) writeCCRClaudeHookShim() error {
	if m.store == nil || strings.TrimSpace(m.store.rootDir) == "" {
		return fmt.Errorf("web session store is not configured")
	}
	shimDir := filepath.Join(m.store.rootDir, "claude-code-router")
	if err := os.MkdirAll(shimDir, 0o755); err != nil {
		return err
	}
	scriptPath := filepath.Join(shimDir, "claude-settings-shim.js")
	cmdPath := filepath.Join(shimDir, "claude-settings-shim.cmd")
	script := fmt.Sprintf(`const fs = require("fs");
const { spawn } = require("child_process");

const realClaude = %q;
const hookSettingsPath = %q;

function readJSON(path) {
  return JSON.parse(fs.readFileSync(path, "utf8"));
}

function mergeUniqueStrings(target, source) {
  const seen = new Set((Array.isArray(target) ? target : []).filter(Boolean));
  for (const value of Array.isArray(source) ? source : []) {
    if (value && !seen.has(value)) {
      seen.add(value);
    }
  }
  return Array.from(seen);
}

function isCodeKanbanHook(hook) {
  return hook && typeof hook === "object" && String(hook.url || hook.command || "").includes("/claude-hooks/pre-tool-use");
}

function mergeHooks(target, source) {
  const next = target && typeof target === "object" && !Array.isArray(target) ? target : {};
  const incoming = source && typeof source === "object" && !Array.isArray(source) ? source : {};
  for (const [eventName, entries] of Object.entries(incoming)) {
    const existing = Array.isArray(next[eventName]) ? next[eventName] : [];
    const filtered = existing
      .map((entry) => {
        if (!entry || typeof entry !== "object" || !Array.isArray(entry.hooks)) return entry;
        const hooks = entry.hooks.filter((hook) => !isCodeKanbanHook(hook));
        return hooks.length ? { ...entry, hooks } : null;
      })
      .filter(Boolean);
    next[eventName] = filtered.concat(Array.isArray(entries) ? entries : []);
  }
  return next;
}

try {
  const args = process.argv.slice(2);
  const settingsIndex = args.lastIndexOf("--settings");
  if (settingsIndex >= 0 && args[settingsIndex + 1]) {
    const settingsPath = args[settingsIndex + 1];
    const settings = readJSON(settingsPath);
    const hookSettings = readJSON(hookSettingsPath);
    settings.allowedHttpHookUrls = mergeUniqueStrings(settings.allowedHttpHookUrls, hookSettings.allowedHttpHookUrls);
    settings.hooks = mergeHooks(settings.hooks, hookSettings.hooks);
    fs.writeFileSync(settingsPath, JSON.stringify(settings, null, 2) + "\n");
  }
} catch (error) {
  console.error("CodeKanban Claude settings shim failed:", error && error.message ? error.message : error);
}

const child = spawn(realClaude, process.argv.slice(2), {
  stdio: "inherit",
  env: process.env,
});
child.on("exit", (code, signal) => {
  if (signal) process.kill(process.pid, signal);
  process.exit(code || 0);
});
child.on("error", (error) => {
  console.error(error && error.message ? error.message : error);
  process.exit(1);
});
`, m.cfg.ClaudePath, m.claudeHookSettingsPath)
	if err := os.WriteFile(scriptPath, []byte(script), 0o644); err != nil {
		return err
	}
	cmd := fmt.Sprintf("@echo off\r\nnode \"%%~dp0%s\" %%*\r\nexit /b %%ERRORLEVEL%%\r\n", filepath.Base(scriptPath))
	if err := os.WriteFile(cmdPath, []byte(cmd), 0o755); err != nil {
		return err
	}
	m.ccrHookClaudePath = cmdPath
	return nil
}

func (m *Manager) claudeHookSettings() map[string]any {
	hookURL := codeKanbanClaudeHookURL(m.claudeHookBaseURL)
	return map[string]any{
		"allowedHttpHookUrls": []string{
			hookURL,
		},
		"hooks": map[string]any{
			"PreToolUse": codeKanbanClaudeHookEntries(m.claudeHookBaseURL, m.claudeHookToken),
		},
	}
}

func codeKanbanClaudeHookURL(baseURL string) string {
	return baseURL + "/claude-hooks/pre-tool-use"
}

func codeKanbanClaudeHookEntries(baseURL, token string) []map[string]any {
	hookURL := codeKanbanClaudeHookURL(baseURL)
	return []map[string]any{
		{
			"matcher": "AskUserQuestion",
			"hooks": []map[string]any{
				{
					"type": "http",
					"url":  hookURL,
					"headers": map[string]any{
						"Authorization": "Bearer " + token,
					},
				},
			},
		},
		{
			"matcher": "ExitPlanMode",
			"hooks": []map[string]any{
				{
					"type": "http",
					"url":  hookURL,
					"headers": map[string]any{
						"Authorization": "Bearer " + token,
					},
				},
			},
		},
	}
}

func (m *Manager) handleClaudePreToolUseHook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")); token == "" || token != m.claudeHookToken {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	defer r.Body.Close()

	var request claudePreToolUseRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	toolName := strings.TrimSpace(request.ToolName)
	if toolName != "AskUserQuestion" && toolName != "ExitPlanMode" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	response := claudePreToolUseResponse{
		HookSpecificOutput: claudePreToolUseHookSpecificOutput{
			HookEventName:      "PreToolUse",
			PermissionDecision: "defer",
		},
	}
	if answerFile, err := m.readClaudeHookAnswer(strings.TrimSpace(request.SessionID), strings.TrimSpace(request.ToolUseID)); err == nil {
		if toolName == "ExitPlanMode" && strings.TrimSpace(answerFile.PermissionDecision) != "" {
			response.HookSpecificOutput.PermissionDecision = strings.TrimSpace(answerFile.PermissionDecision)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
			return
		}
		if toolName != "AskUserQuestion" || len(answerFile.Answers) == 0 {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
			return
		}
		answers := make(map[string][]string, len(answerFile.Answers))
		questions := decodeToolQuestions(request.ToolInput["questions"])
		for _, question := range questions {
			key := strings.TrimSpace(firstNonEmpty(question.Question, question.Header))
			if key == "" {
				continue
			}
			value := strings.TrimSpace(answerFile.Answers[key])
			if value == "" {
				continue
			}
			answers[firstNonEmpty(question.ID, question.Question, question.Header)] = []string{value}
		}
		response.HookSpecificOutput.PermissionDecision = "allow"
		response.HookSpecificOutput.UpdatedInput = claudeAskUserUpdatedInput(request.ToolInput, questions, answers)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}
