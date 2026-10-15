package websession

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"code-kanban/model/tables"
)

func codexServiceTier(session tables.WebSessionTable) string {
	if session.CodexFastMode {
		return "priority"
	}
	// Explicitly reset the tier when resuming a thread that previously used Fast.
	return "default"
}

func (m *Manager) UpdateCodexFastMode(ctx context.Context, sessionID string, enabled bool) (SessionSummary, error) {
	record, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return SessionSummary{}, err
	}
	if normalizeAgent(Agent(record.Agent)) != AgentCodex {
		return SessionSummary{}, fmt.Errorf("fast mode is only supported for Codex")
	}
	return m.updateFields(ctx, sessionID, map[string]any{
		"codex_fast_mode": enabled,
		"updated_at":      time.Now(),
	})
}

func (m *Manager) handleSetCodexFastModeCommand(ctx context.Context, client *client, frame wireCommandFrame) error {
	var payload struct {
		Enabled *bool `json:"cfm"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil || payload.Enabled == nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", "invalid Codex fast mode payload", false))
	}
	if _, err := m.UpdateCodexFastMode(ctx, frame.SessionID, *payload.Enabled); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", err.Error(), false))
	}
	return m.sendMutationAck(ctx, client, frame, nil)
}
