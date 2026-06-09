package websession

import (
	"context"
	"fmt"
	"strings"
	"time"

	"code-kanban/model/tables"
)

type codexThreadSummary struct {
	ID        string
	Preview   string
	Path      string
	Cwd       string
	Status    string
	CreatedAt *time.Time
	UpdatedAt *time.Time
}

type codexThreadReadResult struct {
	Summary codexThreadSummary
	Goal    *SessionGoal
	Turns   []map[string]any
}

func parseCodexSessionGoal(raw any, fallbackThreadID string) *SessionGoal {
	record := decodeRawObject(raw)
	if len(record) == 0 {
		return nil
	}
	objective := strings.TrimSpace(stringValue(record["objective"]))
	if objective == "" {
		return nil
	}
	threadID := strings.TrimSpace(firstNonEmpty(stringValue(record["threadId"]), fallbackThreadID))
	if threadID == "" {
		return nil
	}
	status := normalizeGoalStatus(firstNonEmpty(stringValue(record["status"]), string(GoalStatusActive)))
	if status == "" {
		status = GoalStatusActive
	}
	createdAt := parseHistoryTimestamp(record["createdAt"])
	updatedAt := parseHistoryTimestamp(record["updatedAt"])
	if createdAt == nil || updatedAt == nil {
		return nil
	}
	var tokenBudget *int64
	switch value := record["tokenBudget"].(type) {
	case float64:
		if value >= 0 {
			parsed := int64(value)
			tokenBudget = &parsed
		}
	case int64:
		if value >= 0 {
			parsed := value
			tokenBudget = &parsed
		}
	case int:
		if value >= 0 {
			parsed := int64(value)
			tokenBudget = &parsed
		}
	}
	return &SessionGoal{
		ThreadID:        threadID,
		Objective:       objective,
		Status:          status,
		TokenBudget:     tokenBudget,
		TokensUsed:      maxInt64(0, int64(numberValue(record["tokensUsed"]))),
		TimeUsedSeconds: maxInt64(0, int64(numberValue(record["timeUsedSeconds"]))),
		CreatedAt:       *createdAt,
		UpdatedAt:       *updatedAt,
	}
}

func (m *Manager) withCodexQueryClient(
	ctx context.Context,
	cwd string,
	fn func(client *codexAppServerClient) error,
) error {
	client, stderr, err := startCodexAppServer(ctx, m.cfg.CodexPath, cwd)
	if err != nil {
		return err
	}
	defer func() {
		_ = client.closeStdin()
		if client.cmd != nil {
			killCmdTree(client.cmd)
			if client.cmd.Process != nil {
				_, _ = client.cmd.Process.Wait()
			}
		}
		_ = stderr
	}()

	if _, err := client.request(ctx, "initialize", map[string]any{
		"clientInfo": map[string]any{
			"name":    "codekanban-web-session",
			"version": "0.0.0",
		},
		"capabilities": map[string]any{
			"experimentalApi": true,
		},
	}); err != nil {
		return err
	}
	return fn(client)
}

func parseCodexThreadSummary(raw any) codexThreadSummary {
	thread := decodeRawObject(raw)
	summary := codexThreadSummary{
		ID:      stringValue(thread["id"]),
		Preview: stringValue(thread["preview"]),
		Path:    stringValue(thread["path"]),
		Cwd:     stringValue(thread["cwd"]),
		Status:  stringValue(thread["status"]),
	}
	if createdAt := int64(numberValue(thread["createdAt"])); createdAt > 0 {
		value := time.Unix(createdAt, 0)
		summary.CreatedAt = &value
	}
	if updatedAt := int64(numberValue(thread["updatedAt"])); updatedAt > 0 {
		value := time.Unix(updatedAt, 0)
		summary.UpdatedAt = &value
	}
	return summary
}

func (m *Manager) readCodexThread(
	ctx context.Context,
	session tables.WebSessionTable,
	threadID string,
) (codexThreadReadResult, error) {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return codexThreadReadResult{}, fmt.Errorf("thread id is required")
	}

	var result codexThreadReadResult
	err := m.withCodexQueryClient(ctx, session.Cwd, func(client *codexAppServerClient) error {
		response, err := client.request(ctx, "thread/read", map[string]any{
			"threadId":     threadID,
			"includeTurns": true,
		})
		if err != nil {
			return err
		}
		payload := decodeRawObject(response.Result)
		thread := decodeRawObject(payload["thread"])
		result.Summary = parseCodexThreadSummary(thread)
		turns := decodeRawArray(thread["turns"])
		result.Turns = make([]map[string]any, 0, len(turns))
		for _, turn := range turns {
			result.Turns = append(result.Turns, decodeRawObject(turn))
		}
		goalResponse, err := client.request(ctx, "thread/goal/get", map[string]any{
			"threadId": threadID,
		})
		if err == nil {
			goalPayload := decodeRawObject(goalResponse.Result)
			result.Goal = parseCodexSessionGoal(goalPayload["goal"], threadID)
		}
		return nil
	})
	return result, err
}

func (m *Manager) listCodexThreadsByCwd(
	ctx context.Context,
	cwd string,
	archived bool,
) (map[string]codexThreadSummary, error) {
	result := make(map[string]codexThreadSummary)
	err := m.withCodexQueryClient(ctx, cwd, func(client *codexAppServerClient) error {
		cursor := ""
		for {
			params := map[string]any{
				"cwd":      cwd,
				"archived": archived,
				"limit":    100,
			}
			if cursor != "" {
				params["cursor"] = cursor
			}
			response, err := client.request(ctx, "thread/list", params)
			if err != nil {
				return err
			}
			payload := decodeRawObject(response.Result)
			items := decodeRawArray(payload["data"])
			for _, item := range items {
				summary := parseCodexThreadSummary(item)
				if summary.ID == "" {
					continue
				}
				result[summary.ID] = summary
			}
			cursor = stringValue(payload["nextCursor"])
			if cursor == "" {
				break
			}
		}
		return nil
	})
	return result, err
}
