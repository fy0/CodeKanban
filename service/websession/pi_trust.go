package websession

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"code-kanban/service"
)

type piRuntimeTerminator struct {
	projectID string
	terminate func()
}

func (m *Manager) GetProjectPiTrust(
	ctx context.Context,
	projectID string,
) (service.ProjectAgentTrustStatus, error) {
	if m == nil || m.agentTrustSvc == nil {
		return service.ProjectAgentTrustStatus{}, errors.New("project agent trust service is not configured")
	}
	return m.agentTrustSvc.GetStatus(ctx, projectID, service.ProjectAgentPi)
}

func (m *Manager) TrustProjectForPi(
	ctx context.Context,
	projectID string,
) (service.ProjectAgentTrustStatus, error) {
	if m == nil || m.agentTrustSvc == nil {
		return service.ProjectAgentTrustStatus{}, errors.New("project agent trust service is not configured")
	}
	return m.agentTrustSvc.Trust(ctx, projectID, service.ProjectAgentPi)
}

func (m *Manager) RevokeProjectPiTrust(
	ctx context.Context,
	projectID string,
) (service.ProjectAgentTrustStatus, error) {
	if m == nil || m.agentTrustSvc == nil {
		return service.ProjectAgentTrustStatus{}, errors.New("project agent trust service is not configured")
	}
	status, err := m.agentTrustSvc.Revoke(ctx, projectID, service.ProjectAgentPi)
	if err != nil {
		return service.ProjectAgentTrustStatus{}, err
	}
	m.StopProjectPiRuntimes(projectID)
	return status, nil
}

func (m *Manager) EnsureProjectPiTrust(ctx context.Context, projectID, cwd string) error {
	if m == nil || m.agentTrustSvc == nil {
		return errors.New("project agent trust service is not configured")
	}
	return m.agentTrustSvc.EnsureTrustedPath(ctx, projectID, service.ProjectAgentPi, cwd)
}

// buildTrustedPiRPCCommand is the only launch path for persistent Pi RPC
// processes. The no-session capability probe intentionally does not use it.
func (m *Manager) buildTrustedPiRPCCommand(
	ctx context.Context,
	projectID string,
	cwd string,
	args ...string,
) (*exec.Cmd, error) {
	if err := m.EnsureProjectPiTrust(ctx, projectID, cwd); err != nil {
		return nil, err
	}
	for _, arg := range args {
		normalized := strings.ToLower(strings.TrimSpace(arg))
		if normalized == "--approve" || normalized == "--no-approve" ||
			strings.HasPrefix(normalized, "--approve=") || strings.HasPrefix(normalized, "--no-approve=") {
			return nil, fmt.Errorf("Pi approval flags are managed by CodeKanban")
		}
		if normalized == "--mode" || strings.HasPrefix(normalized, "--mode=") {
			return nil, fmt.Errorf("Pi mode is managed by CodeKanban")
		}
		if normalized == "--extension" || normalized == "-e" || strings.HasPrefix(normalized, "--extension=") {
			return nil, fmt.Errorf("Pi extensions are managed by CodeKanban")
		}
	}
	bridgePath, err := m.materializePiBridge()
	if err != nil {
		return nil, err
	}
	launchArgs := []string{"--mode", "rpc", "--approve", "--extension", bridgePath}
	launchArgs = append(launchArgs, args...)
	// The caller context governs trust lookup, not the lifetime of the reusable
	// process. Runtime shutdown is owned by the Pi registry.
	cmd, err := buildPiCommand(context.Background(), m.cfg.PiPath, launchArgs...)
	if err != nil {
		return nil, err
	}
	cmd.Dir = cwd
	return cmd, nil
}

func (m *Manager) registerPiRuntimeTerminator(
	sessionID string,
	projectID string,
	terminate func(),
) {
	if m == nil || strings.TrimSpace(sessionID) == "" || terminate == nil {
		return
	}
	m.piRuntimeMu.Lock()
	m.piRuntimeTerminators[strings.TrimSpace(sessionID)] = piRuntimeTerminator{
		projectID: strings.TrimSpace(projectID),
		terminate: terminate,
	}
	m.piRuntimeMu.Unlock()
}

func (m *Manager) unregisterPiRuntimeTerminator(sessionID string) {
	if m == nil {
		return
	}
	m.piRuntimeMu.Lock()
	delete(m.piRuntimeTerminators, strings.TrimSpace(sessionID))
	m.piRuntimeMu.Unlock()
}

// StopSessionPiRuntime terminates a registered Pi process, including an idle one.
func (m *Manager) StopSessionPiRuntime(sessionID string) {
	if m == nil {
		return
	}
	m.piRuntimeMu.Lock()
	runtime, ok := m.piRuntimeTerminators[strings.TrimSpace(sessionID)]
	if ok {
		delete(m.piRuntimeTerminators, strings.TrimSpace(sessionID))
		delete(m.piRuntimes, strings.TrimSpace(sessionID))
	}
	m.piRuntimeMu.Unlock()
	if ok && runtime.terminate != nil {
		runtime.terminate()
	}
}

// StopAllPiRuntimes terminates every persistent Pi process owned by the manager.
func (m *Manager) StopAllPiRuntimes() {
	if m == nil {
		return
	}
	m.piRuntimeMu.Lock()
	terminators := make([]func(), 0, len(m.piRuntimeTerminators))
	for sessionID, runtime := range m.piRuntimeTerminators {
		delete(m.piRuntimeTerminators, sessionID)
		delete(m.piRuntimes, sessionID)
		if runtime.terminate != nil {
			terminators = append(terminators, runtime.terminate)
		}
	}
	m.piRuntimeMu.Unlock()
	for _, terminate := range terminators {
		terminate()
	}
}

// StopProjectPiRuntimes terminates registered Pi processes for one project.
func (m *Manager) StopProjectPiRuntimes(projectID string) {
	if m == nil {
		return
	}
	projectID = strings.TrimSpace(projectID)
	terminators := make([]func(), 0)
	m.piRuntimeMu.Lock()
	for sessionID, runtime := range m.piRuntimeTerminators {
		if runtime.projectID != projectID {
			continue
		}
		delete(m.piRuntimeTerminators, sessionID)
		if runtime.terminate != nil {
			terminators = append(terminators, runtime.terminate)
		}
	}
	m.piRuntimeMu.Unlock()
	for _, terminate := range terminators {
		terminate()
	}
}
