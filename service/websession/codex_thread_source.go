package websession

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"code-kanban/model/tables"
)

type codexThreadSummary struct {
	ID             string
	ParentThreadID string
	AgentPath      string
	Nickname       string
	Role           string
	Preview        string
	Path           string
	Cwd            string
	Status         string
	CreatedAt      *time.Time
	UpdatedAt      *time.Time
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
		ID:             stringValue(thread["id"]),
		ParentThreadID: stringValue(thread["parentThreadId"]),
		AgentPath:      codexThreadAgentPath(thread),
		Nickname:       stringValue(thread["agentNickname"]),
		Role:           stringValue(thread["agentRole"]),
		Preview:        stringValue(thread["preview"]),
		Path:           stringValue(thread["path"]),
		Cwd:            stringValue(thread["cwd"]),
		Status:         codexThreadStatusValue(thread["status"]),
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
		var err error
		result, err = readCodexThreadWithClient(ctx, client, threadID)
		if err != nil {
			return err
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

func readCodexThreadWithClient(
	ctx context.Context,
	client *codexAppServerClient,
	threadID string,
) (codexThreadReadResult, error) {
	response, err := client.request(ctx, "thread/read", map[string]any{
		"threadId":     strings.TrimSpace(threadID),
		"includeTurns": true,
	})
	if err != nil {
		return codexThreadReadResult{}, err
	}
	payload := decodeRawObject(response.Result)
	thread := decodeRawObject(payload["thread"])
	result := codexThreadReadResult{Summary: parseCodexThreadSummary(thread)}
	turns := decodeRawArray(thread["turns"])
	result.Turns = make([]map[string]any, 0, len(turns))
	for _, turn := range turns {
		result.Turns = append(result.Turns, decodeRawObject(turn))
	}
	return result, nil
}

func listCodexThreadRelationWithClient(
	ctx context.Context,
	client *codexAppServerClient,
	relationKey string,
	threadID string,
) (map[string]codexThreadSummary, error) {
	result := make(map[string]codexThreadSummary)
	for _, archived := range []bool{false, true} {
		cursor := ""
		for {
			params := map[string]any{
				"archived":  archived,
				"limit":     100,
				relationKey: strings.TrimSpace(threadID),
			}
			if cursor != "" {
				params["cursor"] = cursor
			}
			response, err := client.request(ctx, "thread/list", params)
			if err != nil {
				return nil, err
			}
			payload := decodeRawObject(response.Result)
			for _, item := range decodeRawArray(payload["data"]) {
				summary := parseCodexThreadSummary(item)
				if summary.ID != "" {
					result[summary.ID] = summary
				}
			}
			cursor = strings.TrimSpace(stringValue(payload["nextCursor"]))
			if cursor == "" {
				break
			}
		}
	}
	return result, nil
}

func listCodexDescendantsWithClient(
	ctx context.Context,
	client *codexAppServerClient,
	rootThreadID string,
) (map[string]codexThreadSummary, error) {
	descendants, ancestorErr := listCodexThreadRelationWithClient(
		ctx,
		client,
		"ancestorThreadId",
		rootThreadID,
	)
	if ancestorErr == nil {
		delete(descendants, strings.TrimSpace(rootThreadID))
		return descendants, nil
	}

	result := make(map[string]codexThreadSummary)
	queue := []string{strings.TrimSpace(rootThreadID)}
	seenParents := make(map[string]struct{})
	for len(queue) > 0 {
		parentID := queue[0]
		queue = queue[1:]
		if parentID == "" {
			continue
		}
		if _, seen := seenParents[parentID]; seen {
			continue
		}
		seenParents[parentID] = struct{}{}
		children, err := listCodexThreadRelationWithClient(ctx, client, "parentThreadId", parentID)
		if err != nil {
			return nil, fmt.Errorf("descendant listing is unsupported (ancestor: %v; parent: %w)", ancestorErr, err)
		}
		for childID, summary := range children {
			if childID == strings.TrimSpace(rootThreadID) {
				continue
			}
			if _, exists := result[childID]; exists {
				continue
			}
			result[childID] = summary
			queue = append(queue, childID)
		}
	}
	return result, nil
}

func mergeCodexDescendantSummary(read codexThreadSummary, listed codexThreadSummary) codexThreadSummary {
	if read.ParentThreadID == "" || read.ParentThreadID == read.ID {
		read.ParentThreadID = listed.ParentThreadID
	}
	if read.ParentThreadID == read.ID {
		read.ParentThreadID = ""
	}
	if read.AgentPath == "" {
		read.AgentPath = listed.AgentPath
	}
	if read.Nickname == "" {
		read.Nickname = listed.Nickname
	}
	if read.Role == "" {
		read.Role = listed.Role
	}
	return read
}

func (m *Manager) readCodexDescendantThreads(
	ctx context.Context,
	session tables.WebSessionTable,
	rootThreadID string,
) ([]codexThreadReadResult, error) {
	results := make([]codexThreadReadResult, 0)
	err := m.withCodexQueryClient(ctx, session.Cwd, func(client *codexAppServerClient) error {
		summaries, err := listCodexDescendantsWithClient(ctx, client, rootThreadID)
		if err != nil {
			return err
		}
		ordered := make([]codexThreadSummary, 0, len(summaries))
		for _, summary := range summaries {
			ordered = append(ordered, summary)
		}
		sort.SliceStable(ordered, func(i, j int) bool {
			left := ordered[i].CreatedAt
			right := ordered[j].CreatedAt
			if left != nil && right != nil && !left.Equal(*right) {
				return left.Before(*right)
			}
			return ordered[i].ID < ordered[j].ID
		})
		for _, summary := range ordered {
			read, err := readCodexThreadWithClient(ctx, client, summary.ID)
			if err != nil {
				return err
			}
			read.Summary = mergeCodexDescendantSummary(read.Summary, summary)
			results = append(results, read)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return results, nil
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
