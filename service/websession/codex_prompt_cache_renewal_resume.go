package websession

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"time"

	"code-kanban/model/tables"
)

func (m *Manager) canResumePromptCacheParent(session tables.WebSessionTable) bool {
	return session.ArchivedAt == nil && session.NativeSessionID != nil &&
		strings.TrimSpace(*session.NativeSessionID) != "" &&
		(Status(session.Status) == StatusDone || Status(session.Status) == StatusIdle) &&
		m.runs[session.ID] == nil && m.codexRunDrains[session.ID] == nil &&
		len(m.pendingInputs[session.ID]) == 0 && !m.pendingProcessing[session.ID]
}

func (m *Manager) preparePersistedPromptCacheRenewal(session tables.WebSessionTable) (*promptCacheRenewal, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if current := m.promptCacheRenewals[session.ID]; current != nil {
		return current, false
	}
	if m.promptCacheUnsupported != "" || !m.canResumePromptCacheParent(session) {
		return nil, false
	}
	threadID := strings.TrimSpace(*session.NativeSessionID)
	for _, current := range m.promptCacheRenewals {
		if current.scope.threadID == threadID {
			return nil, false
		}
	}
	if m.promptCacheRenewals == nil {
		m.promptCacheRenewals = make(map[string]*promptCacheRenewal)
	}
	now := time.Now()
	renewal := newPromptCacheRenewal(context.Background(), session, codexTurnScope{threadID: threadID}, m.cfg.PromptCacheRenewal, now, promptCacheObservationTime(session, now))
	renewal.done = make(chan struct{})
	renewal.updateMode("cycle")
	m.promptCacheRenewals[session.ID] = renewal
	delete(m.promptCacheFailures, session.ID)
	return renewal, true
}

func (m *Manager) resumePromptCacheRenewal(renewal *promptCacheRenewal) {
	defer func() {
		renewal.stop()
		m.mu.Lock()
		if m.promptCacheRenewals[renewal.parent.ID] == renewal {
			delete(m.promptCacheRenewals, renewal.parent.ID)
		}
		close(renewal.done)
		m.mu.Unlock()
	}()
	if renewal.ctx.Err() != nil {
		return
	}
	client, stderr, err := startCodexAppServer(context.Background(), m.cfg.CodexPath, renewal.parent.Cwd)
	if err != nil {
		m.recordPromptCacheFailure(renewal, "connection_closed")
		return
	}
	renewal.mu.Lock()
	renewal.client = client
	renewal.mu.Unlock()
	go func() { _, _ = io.Copy(io.Discard, stderr) }()
	go func() {
		for range client.incoming {
		}
	}()
	exited := make(chan struct{})
	go func() {
		_ = client.cmd.Wait()
		close(exited)
	}()
	defer func() {
		_ = client.closeStdin()
		select {
		case <-exited:
		case <-time.After(codexRunDrainGracePeriod):
			killCmdTree(client.cmd)
			client.closeTransport()
			select {
			case <-exited:
			case <-time.After(codexAppServerProcessExitTimeout):
			}
		}
	}()
	turnID, category := m.loadPromptCacheParentTurn(renewal)
	if category != "" {
		m.recordPromptCacheFailure(renewal, category)
		return
	}
	renewal.scope.turnID = turnID
	m.runPromptCacheRenewal(renewal)
}

func (m *Manager) loadPromptCacheParentTurn(renewal *promptCacheRenewal) (string, string) {
	ctx, cancel := context.WithTimeout(renewal.ctx, codexAppServerRequestTimeout)
	defer cancel()
	client := renewal.client
	response, err := client.request(ctx, "initialize", m.codexRunInitializeParams())
	if err != nil {
		return "", codexSampleError(client, err)
	}
	if codexExperimentalAPIDisabled(response.Result) {
		m.disablePromptCacheRenewal("experimental_api_disabled")
		return "", "experimental_api_disabled"
	}
	if err := client.writeMessage(codexAppServerOutgoing{Method: "initialized"}); err != nil {
		return "", codexSampleError(client, err)
	}
	threadID := renewal.scope.threadID
	params := codexThreadResumeParams(renewal.parent, threadID, true)
	params["excludeTurns"] = true
	response, err = client.request(ctx, "thread/resume", params)
	if err != nil {
		return "", codexSampleError(client, err)
	}
	thread := decodeRawObject(decodeRawObject(response.Result)["thread"])
	if !isCodexSampleRoot(thread) || stringValue(thread["id"]) != threadID {
		return "", "parent_not_root"
	}
	response, err = client.request(ctx, "thread/turns/list", map[string]any{
		"threadId": threadID, "limit": 1, "sortDirection": "desc", "itemsView": "notLoaded",
	})
	if err != nil {
		return "", codexSampleError(client, err)
	}
	var result struct {
		Data []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Result, &result); err != nil || len(result.Data) == 0 {
		return "", "expected_turn_changed"
	}
	latest := result.Data[0]
	switch latest.Status {
	case "completed", "interrupted", "failed":
		if strings.TrimSpace(latest.ID) != "" {
			return latest.ID, ""
		}
	case "inProgress":
		return "", "parent_busy"
	}
	return "", "expected_turn_changed"
}
