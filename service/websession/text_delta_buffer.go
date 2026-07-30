package websession

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"code-kanban/model/tables"
	"code-kanban/utils"

	"go.uber.org/zap"
)

const (
	defaultTextDeltaFlushWindow  = 40 * time.Millisecond
	defaultProjectionRetryWindow = 100 * time.Millisecond
	maxPendingTextDeltaBytes     = 16 * 1024
)

type pendingTextDelta struct {
	record tables.WebSessionTable
	event  Event
	chunks []string
	bytes  int
}

type sessionEventState struct {
	mu                        sync.Mutex
	pending                   *pendingTextDelta
	timer                     *time.Timer
	timerGeneration           uint64
	projectionRetries         []eventProjectionRetry
	projectionTimer           *time.Timer
	projectionTimerGeneration uint64
	lastSeq                   int64
	seqInitialized            bool
	closed                    bool
}

func (m *Manager) sessionEventState(sessionID string) *sessionEventState {
	m.eventStatesMu.Lock()
	defer m.eventStatesMu.Unlock()
	if m.eventStates == nil {
		m.eventStates = make(map[string]*sessionEventState)
	}
	state := m.eventStates[sessionID]
	if state == nil {
		state = &sessionEventState{}
		m.eventStates[sessionID] = state
	}
	return state
}

func (m *Manager) removeSessionEventState(sessionID string, state *sessionEventState) {
	m.eventStatesMu.Lock()
	defer m.eventStatesMu.Unlock()
	if m.eventStates[sessionID] == state {
		delete(m.eventStates, sessionID)
	}
}

func (m *Manager) clearSessionEventState(sessionID string) {
	m.eventStatesMu.Lock()
	state := m.eventStates[sessionID]
	delete(m.eventStates, sessionID)
	m.eventStatesMu.Unlock()
	if state == nil {
		return
	}
	state.mu.Lock()
	if state.timer != nil {
		state.timer.Stop()
	}
	if state.projectionTimer != nil {
		state.projectionTimer.Stop()
	}
	state.pending = nil
	state.timer = nil
	state.timerGeneration++
	state.projectionRetries = nil
	state.projectionTimer = nil
	state.projectionTimerGeneration++
	state.closed = false
	state.mu.Unlock()
}

func (m *Manager) queueEventProjectionRetryLocked(
	sessionID string,
	state *sessionEventState,
	retry eventProjectionRetry,
) {
	if state == nil || strings.TrimSpace(retry.event.ID) == "" {
		return
	}
	for index := range state.projectionRetries {
		if state.projectionRetries[index].event.ID == retry.event.ID {
			return
		}
	}
	state.projectionRetries = append(state.projectionRetries, retry)
	m.scheduleEventProjectionRetryLocked(sessionID, state)
}

func (m *Manager) scheduleEventProjectionRetryLocked(sessionID string, state *sessionEventState) {
	if state == nil || state.closed || state.projectionTimer != nil || len(state.projectionRetries) == 0 {
		return
	}
	state.projectionTimerGeneration++
	generation := state.projectionTimerGeneration
	state.projectionTimer = time.AfterFunc(defaultProjectionRetryWindow, func() {
		m.flushEventProjectionRetryTimer(sessionID, state, generation)
	})
}

func (m *Manager) flushEventProjectionRetryTimer(
	sessionID string,
	state *sessionEventState,
	generation uint64,
) {
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.closed || state.projectionTimerGeneration != generation {
		return
	}
	state.projectionTimer = nil
	if err := m.flushEventProjectionRetriesLocked(context.Background(), sessionID, state); err != nil {
		m.scheduleEventProjectionRetryLocked(sessionID, state)
	}
}

func (m *Manager) flushEventProjectionRetriesLocked(
	ctx context.Context,
	sessionID string,
	state *sessionEventState,
) error {
	if state == nil {
		return fmt.Errorf("web session event state is nil")
	}
	for len(state.projectionRetries) > 0 {
		retry := &state.projectionRetries[0]
		if err := m.projectPersistedEvent(ctx, sessionID, retry); err != nil {
			return err
		}
		state.projectionRetries[0] = eventProjectionRetry{}
		state.projectionRetries = state.projectionRetries[1:]
	}
	if state.projectionTimer != nil {
		state.projectionTimer.Stop()
		state.projectionTimer = nil
		state.projectionTimerGeneration++
	}
	return nil
}

func (m *Manager) enqueueTextDelta(
	ctx context.Context,
	sessionID string,
	record tables.WebSessionTable,
	event Event,
) error {
	if strings.TrimSpace(event.Type) != "txt_d" {
		return fmt.Errorf("cannot enqueue non-text-delta event %q", event.Type)
	}

	state := m.sessionEventState(sessionID)
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.closed {
		return fmt.Errorf("web session %s is being deleted", sessionID)
	}

	if state.pending != nil && !samePendingTextDeltaKey(state.pending.event, event) {
		if err := m.flushPendingTextDeltaLocked(ctx, sessionID, state); err != nil {
			return err
		}
	}

	chunk := stringValue(event.Payload["txt"])
	if state.pending == nil {
		bufferedEvent := event
		bufferedEvent.Payload = cloneMap(event.Payload)
		state.pending = &pendingTextDelta{
			record: record,
			event:  bufferedEvent,
			chunks: []string{chunk},
			bytes:  len(chunk),
		}
		m.schedulePendingTextDeltaTimerLocked(sessionID, state)
	} else {
		state.pending.chunks = append(state.pending.chunks, chunk)
		state.pending.bytes += len(chunk)
	}

	if state.pending.bytes >= maxPendingTextDeltaBytes {
		return m.flushPendingTextDeltaLocked(ctx, sessionID, state)
	}
	return nil
}

func samePendingTextDeltaKey(left, right Event) bool {
	return left.RunID == right.RunID &&
		left.ParentID == right.ParentID &&
		left.ThreadID == right.ThreadID &&
		left.TurnID == right.TurnID &&
		stringValue(left.Payload["mid"]) == stringValue(right.Payload["mid"])
}

func (m *Manager) schedulePendingTextDeltaTimerLocked(sessionID string, state *sessionEventState) {
	state.timerGeneration++
	generation := state.timerGeneration
	window := m.textDeltaFlushWindow
	if window <= 0 {
		window = defaultTextDeltaFlushWindow
	}
	state.timer = time.AfterFunc(window, func() {
		m.flushPendingTextDeltaTimer(sessionID, state, generation)
	})
}

func (m *Manager) flushPendingTextDeltaTimer(sessionID string, state *sessionEventState, generation uint64) {
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.closed || state.pending == nil || state.timerGeneration != generation {
		return
	}
	if err := m.flushPendingTextDeltaLocked(context.Background(), sessionID, state); err != nil {
		// flushPendingTextDeltaLocked preserves and reschedules the buffer on failure.
		return
	}
}

func (m *Manager) flushPendingTextDeltaLocked(ctx context.Context, sessionID string, state *sessionEventState) error {
	if state.pending == nil {
		return nil
	}
	if state.timer != nil {
		state.timer.Stop()
		state.timer = nil
	}
	state.timerGeneration++

	pending := state.pending
	merged := pending.event
	if strings.TrimSpace(merged.ID) == "" {
		merged.ID = utils.NewID()
		pending.event.ID = merged.ID
	}
	merged.Payload = cloneMap(pending.event.Payload)
	merged.Payload["txt"] = strings.Join(pending.chunks, "")
	if _, err := m.appendAndBroadcastNow(ctx, sessionID, pending.record, state, merged); err != nil {
		if _, persisted := persistedEventFromError(err, merged.ID); persisted {
			state.pending = nil
			m.logTextDeltaFlushFailure(sessionID, pending.event, err)
			return nil
		}
		m.logTextDeltaFlushFailure(sessionID, pending.event, err)
		m.schedulePendingTextDeltaTimerLocked(sessionID, state)
		return err
	}
	state.pending = nil
	return nil
}

func (m *Manager) logTextDeltaFlushFailure(sessionID string, event Event, err error) {
	if m.logger == nil {
		return
	}
	m.logger.Error(
		"failed to flush buffered web session text delta",
		zap.String("sessionId", sessionID),
		zap.String("runId", event.RunID),
		zap.String("messageId", firstNonEmpty(stringValue(event.Payload["mid"]), event.ParentID)),
		zap.Error(err),
	)
}
