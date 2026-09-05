package websession

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"code-kanban/model/tables"
	"code-kanban/utils"
)

func (m *Manager) handleCodexModelMetadataWarning(session tables.WebSessionTable, run *activeRun, params any) {
	message := strings.TrimSpace(stringValue(decodeRawObject(params)["message"]))
	if !strings.Contains(message, "Model metadata for") || !strings.Contains(message, "fallback metadata") {
		return
	}
	if err := m.updateRuntimeState(context.Background(), session.ID, map[string]any{"codex_model_metadata_fallback": true}); err != nil {
		return
	}
	m.appendRunNote(session.ID, session, run, "warn", message, map[string]any{"code": "codex_model_metadata_fallback"})
	m.broadcastSessionSummary(context.Background(), session.ID)
}

func validContextWindowSetting(value int64) bool {
	return utils.ValidCodexContextWindow(value)
}

func (m *Manager) resolveContextWindowSetting(agent Agent, value *int64) (int64, error) {
	resolved := int64(0)
	if agent == AgentCodex && m.cfg.DefaultCodexContextWindow != nil {
		resolved = m.cfg.DefaultCodexContextWindow()
	}
	if value != nil {
		resolved = *value
	}
	if !validContextWindowSetting(resolved) {
		return 0, fmt.Errorf("invalid context window setting")
	}
	if agent != AgentCodex && resolved != 0 {
		return 0, fmt.Errorf("context window setting is only supported for Codex")
	}
	return resolved, nil
}

func codexSessionThreadConfig(session tables.WebSessionTable, supportsMultiAgentV2 bool) map[string]any {
	config := codexThreadConfig(supportsMultiAgentV2)
	if session.ContextWindowSetting > 0 {
		config["model_context_window"] = session.ContextWindowSetting
	}
	return config
}

func (m *Manager) UpdateContextWindowSetting(ctx context.Context, sessionID string, value int64) (SessionSummary, error) {
	record, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return SessionSummary{}, err
	}
	if Agent(record.Agent) != AgentCodex {
		return SessionSummary{}, fmt.Errorf("context window setting is only supported for Codex")
	}
	if !validContextWindowSetting(value) {
		return SessionSummary{}, fmt.Errorf("invalid context window setting")
	}
	return m.updateFields(ctx, sessionID, map[string]any{"context_window_setting": value, "updated_at": time.Now()})
}

func (m *Manager) handleSetContextWindowCommand(ctx context.Context, client *client, frame wireCommandFrame) error {
	var payload struct {
		Value *int64 `json:"cwset"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil || payload.Value == nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", "invalid context window payload", false))
	}
	if _, err := m.UpdateContextWindowSetting(ctx, frame.SessionID, *payload.Value); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", err.Error(), false))
	}
	return m.sendMutationAck(ctx, client, frame, nil)
}
