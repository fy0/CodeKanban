package websession

import "sort"

// DatabaseSessionDiagnostic identifies in-memory web-session work that can be
// waiting on database projection. It intentionally contains IDs and counters,
// not message contents.
type DatabaseSessionDiagnostic struct {
	SessionID             string `json:"sessionId"`
	RunID                 string `json:"runId,omitempty"`
	Agent                 string `json:"agent,omitempty"`
	Backend               string `json:"backend,omitempty"`
	PendingTextDelta      bool   `json:"pendingTextDelta"`
	PendingTextDeltaBytes int    `json:"pendingTextDeltaBytes,omitempty"`
	PendingRunID          string `json:"pendingRunId,omitempty"`
	PendingMessageID      string `json:"pendingMessageId,omitempty"`
	ProjectionRetryCount  int    `json:"projectionRetryCount,omitempty"`
	LastEventSeq          int64  `json:"lastEventSeq"`
	SeqInitialized        bool   `json:"seqInitialized"`
	Closed                bool   `json:"closed"`
	ActiveCallCount       int    `json:"activeCallCount,omitempty"`
}

// DatabaseSessionDiagnostics returns the sessions with active runtime work or
// queued event projection. A non-empty ProjectionRetryCount is a direct signal
// that a database projection failed and is waiting for retry.
func (m *Manager) DatabaseSessionDiagnostics() []DatabaseSessionDiagnostic {
	if m == nil {
		return nil
	}

	diagnostics := make(map[string]DatabaseSessionDiagnostic)
	m.eventStatesMu.Lock()
	states := make(map[string]*sessionEventState, len(m.eventStates))
	for sessionID, state := range m.eventStates {
		states[sessionID] = state
	}
	m.eventStatesMu.Unlock()
	for sessionID, state := range states {
		if state == nil {
			continue
		}
		state.mu.Lock()
		item := diagnostics[sessionID]
		item.SessionID = sessionID
		item.ProjectionRetryCount = len(state.projectionRetries)
		item.LastEventSeq = state.lastSeq
		item.SeqInitialized = state.seqInitialized
		item.Closed = state.closed
		if state.pending != nil {
			item.PendingTextDelta = true
			item.PendingTextDeltaBytes = state.pending.bytes
			item.PendingRunID = state.pending.event.RunID
			item.PendingMessageID = firstNonEmpty(
				stringValue(state.pending.event.Payload["mid"]),
				state.pending.event.ParentID,
			)
		}
		state.mu.Unlock()
		diagnostics[sessionID] = item
	}

	m.mu.RLock()
	runs := make(map[string]*activeRun, len(m.runs)+len(m.codexRunDrains))
	for sessionID, run := range m.runs {
		runs[sessionID] = run
	}
	for sessionID, run := range m.codexRunDrains {
		if _, exists := runs[sessionID]; !exists {
			runs[sessionID] = run
		}
	}
	m.mu.RUnlock()
	for sessionID, run := range runs {
		if run == nil {
			continue
		}
		run.mu.Lock()
		item := diagnostics[sessionID]
		item.SessionID = sessionID
		item.RunID = run.runID
		item.Agent = string(run.agent)
		item.Backend = string(run.backend)
		item.ActiveCallCount = len(run.activeCalls)
		run.mu.Unlock()
		diagnostics[sessionID] = item
	}

	result := make([]DatabaseSessionDiagnostic, 0, len(diagnostics))
	for _, item := range diagnostics {
		if item.PendingTextDelta || item.ProjectionRetryCount > 0 || item.RunID != "" || item.Closed {
			result = append(result, item)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].SessionID < result[j].SessionID
	})
	return result
}
