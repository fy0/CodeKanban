package websession

import (
	"context"
	"strings"

	"go.uber.org/zap"
)

func (m *Manager) codexAppServerRuntime(sessionID string) CodexAppServerRuntime {
	runtimeState := CodexAppServerRuntime{State: CodexAppServerInactive}
	if m == nil {
		return runtimeState
	}

	sessionID = strings.TrimSpace(sessionID)
	m.mu.RLock()
	run := m.codexRunDrains[sessionID]
	draining := run != nil
	if run == nil {
		run = m.runs[sessionID]
	}
	m.mu.RUnlock()
	if run == nil || run.backend != SessionBackendCodexAppServer || normalizeAgent(run.agent) != AgentCodex {
		return runtimeState
	}

	run.mu.Lock()
	forceTerminateRequested := run.forceTerminateRequested
	client := run.app
	cmd := run.cmd
	runtimeState.RunID = run.runID
	run.mu.Unlock()

	if cmd != nil && cmd.Process != nil {
		runtimeState.ProcessRootPID = cmd.Process.Pid
	}
	switch {
	case forceTerminateRequested:
		runtimeState.State = CodexAppServerTerminating
	case draining:
		runtimeState.State = CodexAppServerDraining
		runtimeState.CanTerminate = true
	case client != nil:
		runtimeState.State = CodexAppServerActive
		runtimeState.CanTerminate = true
	default:
		runtimeState.State = CodexAppServerStarting
	}
	return runtimeState
}

func (m *Manager) broadcastCodexAppServerRuntime(sessionID string) {
	if m == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	lock := &m.revisionBroadcastLocks[sessionRevisionLockIndex(sessionID)]
	lock.Lock()
	defer lock.Unlock()

	// Derive the state under the frame-ordering lock so a concurrent cleanup
	// cannot publish an older state at a newer revision.
	runtimeState := m.codexAppServerRuntime(sessionID)
	revision, err := m.advanceSessionRevision(context.Background(), sessionID)
	if err != nil {
		if m.logger == nil {
			return
		}
		m.logger.Debug(
			"failed to broadcast Codex app-server runtime",
			zap.String("sessionId", sessionID),
			zap.Error(err),
		)
		return
	}
	frame := newCodexAppServerFrame(sessionID, runtimeState)
	applyWireFrameRevision(&frame, formatSnapshotRevision(revision))
	m.broadcastFrame(frame)
}
