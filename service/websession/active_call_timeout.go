package websession

import (
	"context"
	"fmt"
	"strings"
	"time"

	"code-kanban/model/tables"
	"code-kanban/utils"
)

const (
	activeCallTimeoutReason         = "active_call_timeout"
	activeCallTimeoutAbortWait      = 5 * time.Second
	activeCallTimeoutCleanupWait    = 2 * time.Second
	activeCallTimeoutDefaultEnabled = true
)

type activeCallTimeoutKind string

const (
	activeCallTimeoutKindMCP      activeCallTimeoutKind = "mcp"
	activeCallTimeoutKindCommand  activeCallTimeoutKind = "command"
	activeCallTimeoutKindTool     activeCallTimeoutKind = "tool"
	activeCallTimeoutKindSubAgent activeCallTimeoutKind = "sub_agent"
)

type activeCallTimeoutSettings struct {
	Enabled        bool
	Timeout        time.Duration
	PromptTemplate string
	TrackMCP       bool
	TrackCommand   bool
	TrackTool      bool
}

type trackedActiveCall struct {
	ToolID     string
	ToolKind   string
	Kind       activeCallTimeoutKind
	Name       string
	Input      any
	Meta       map[string]any
	StartedAt  time.Time
	PauseTotal time.Duration
}

func (m *Manager) RefreshDeveloperConfig() {
	if m == nil {
		return
	}
	m.mu.RLock()
	runs := make([]*activeRun, 0, len(m.runs))
	for _, run := range m.runs {
		if run != nil {
			runs = append(runs, run)
		}
	}
	m.mu.RUnlock()

	for _, run := range runs {
		m.reconcileActiveCallTimeout(run)
	}
}

func (m *Manager) activeCallTimeoutSettings() activeCallTimeoutSettings {
	raw := utils.NormalizeWebSessionActiveCallTimeoutConfig(utils.WebSessionActiveCallTimeoutConfig{})
	if m != nil && m.cfg.ActiveCallTimeoutConfig != nil {
		raw = utils.NormalizeWebSessionActiveCallTimeoutConfig(m.cfg.ActiveCallTimeoutConfig())
	}

	timeoutSeconds := utils.DefaultWebSessionActiveCallTimeoutSeconds
	if raw.TimeoutMode == utils.WebSessionActiveCallTimeoutModeCustom {
		timeoutSeconds = raw.CustomTimeoutSeconds
	}

	settings := activeCallTimeoutSettings{
		Enabled:        activeCallTimeoutDefaultEnabled,
		Timeout:        time.Duration(timeoutSeconds) * time.Second,
		PromptTemplate: raw.PromptTemplate,
	}
	switch raw.EnabledMode {
	case utils.SettingModeOff:
		settings.Enabled = false
	case utils.SettingModeOn:
		settings.Enabled = true
	default:
		settings.Enabled = activeCallTimeoutDefaultEnabled
	}
	settings.TrackMCP = raw.CallKinds.MCP
	settings.TrackCommand = raw.CallKinds.Command
	settings.TrackTool = raw.CallKinds.Tool
	return settings
}

func (m *Manager) activeCallTimeoutSettingsForSessionID(sessionID string) activeCallTimeoutSettings {
	settings := m.activeCallTimeoutSettings()
	if strings.TrimSpace(sessionID) == "" {
		return settings
	}
	record, err := m.GetSession(context.Background(), sessionID)
	if err != nil || record.ActiveCallTimeoutEnabled == nil {
		return settings
	}
	settings.Enabled = *record.ActiveCallTimeoutEnabled
	return settings
}

func (m *Manager) effectiveActiveCallTimeoutEnabled(record tables.WebSessionTable) bool {
	if record.ActiveCallTimeoutEnabled != nil {
		return *record.ActiveCallTimeoutEnabled
	}
	return m.activeCallTimeoutSettings().Enabled
}

func activeCallTimeoutOverrideOrDefault(value *bool) bool {
	if value != nil {
		return *value
	}
	return activeCallTimeoutDefaultEnabled
}

func (m *Manager) reconcileActiveCallTimeoutBySessionID(sessionID string) {
	if strings.TrimSpace(sessionID) == "" {
		return
	}
	m.mu.RLock()
	run := m.runs[sessionID]
	m.mu.RUnlock()
	m.reconcileActiveCallTimeout(run)
}

func (m *Manager) trackActiveCodexToolStart(
	run *activeRun,
	toolID string,
	toolKind string,
	name string,
	input any,
	meta map[string]any,
) {
	if run == nil || strings.TrimSpace(toolID) == "" {
		return
	}
	kind, ok := activeCallTimeoutKindFromTool(toolKind)
	if !ok {
		return
	}

	run.mu.Lock()
	if run.activeCalls == nil {
		run.activeCalls = make(map[string]trackedActiveCall)
	}
	run.activeCalls[toolID] = trackedActiveCall{
		ToolID:    strings.TrimSpace(toolID),
		ToolKind:  strings.TrimSpace(toolKind),
		Kind:      kind,
		Name:      strings.TrimSpace(name),
		Input:     input,
		Meta:      cloneMap(meta),
		StartedAt: time.Now(),
	}
	run.mu.Unlock()

	m.reconcileActiveCallTimeout(run)
}

func (m *Manager) trackActiveCodexToolComplete(run *activeRun, toolID string) {
	if run == nil || strings.TrimSpace(toolID) == "" {
		return
	}
	run.mu.Lock()
	delete(run.activeCalls, strings.TrimSpace(toolID))
	run.mu.Unlock()
	m.reconcileActiveCallTimeout(run)
}

func (m *Manager) pauseActiveCallTimeout(run *activeRun) {
	if run == nil {
		return
	}
	run.mu.Lock()
	if run.activeCallPausedAt == nil {
		now := time.Now()
		run.activeCallPausedAt = &now
	}
	run.clearActiveCallTimerLocked()
	run.mu.Unlock()
}

func (m *Manager) resumeActiveCallTimeout(run *activeRun) {
	if run == nil {
		return
	}
	run.mu.Lock()
	if run.activeCallPausedAt != nil {
		delta := time.Since(*run.activeCallPausedAt)
		for key, call := range run.activeCalls {
			call.PauseTotal += delta
			run.activeCalls[key] = call
		}
		run.activeCallPausedAt = nil
	}
	run.mu.Unlock()
	m.reconcileActiveCallTimeout(run)
}

func (m *Manager) reconcileActiveCallTimeout(run *activeRun) {
	if run == nil || normalizeAgent(run.agent) != AgentCodex {
		if run != nil {
			run.resetActiveCallTracking()
		}
		return
	}

	sessionID := run.sessionID
	settings := m.activeCallTimeoutSettingsForSessionID(sessionID)
	var (
		triggerImmediately bool
		targetToolID       string
		delay              time.Duration
	)

	run.mu.Lock()
	run.clearActiveCallTimerLocked()
	if !settings.Enabled || run.activeCallInFlight || run.activeCallPausedAt != nil {
		run.mu.Unlock()
		return
	}

	target, ok := run.currentTrackedActiveCallLocked(settings)
	if !ok {
		run.mu.Unlock()
		return
	}

	targetToolID = target.ToolID
	elapsed := target.elapsedAt(time.Now())
	if elapsed >= settings.Timeout {
		triggerImmediately = true
	} else {
		delay = settings.Timeout - elapsed
		run.activeCallTimer = time.AfterFunc(delay, func() {
			m.handleActiveCallTimeout(sessionID, targetToolID)
		})
	}
	run.mu.Unlock()

	if triggerImmediately {
		go m.handleActiveCallTimeout(sessionID, targetToolID)
	}
}

func (m *Manager) handleActiveCallTimeout(sessionID string, toolID string) {
	if strings.TrimSpace(sessionID) == "" || strings.TrimSpace(toolID) == "" {
		return
	}

	settings := m.activeCallTimeoutSettingsForSessionID(sessionID)
	if !settings.Enabled {
		return
	}

	m.mu.RLock()
	run := m.runs[sessionID]
	m.mu.RUnlock()
	if run == nil || normalizeAgent(run.agent) != AgentCodex {
		return
	}

	var (
		call        trackedActiveCall
		elapsed     time.Duration
		durationStr string
		callLabel   string
		prompt      string
	)

	run.mu.Lock()
	if run.activeCallInFlight || run.activeCallPausedAt != nil {
		run.mu.Unlock()
		m.reconcileActiveCallTimeout(run)
		return
	}

	current, ok := run.currentTrackedActiveCallLocked(settings)
	if !ok || current.ToolID != strings.TrimSpace(toolID) {
		run.mu.Unlock()
		return
	}

	elapsed = current.elapsedAt(time.Now())
	if elapsed < settings.Timeout {
		run.mu.Unlock()
		m.reconcileActiveCallTimeout(run)
		return
	}

	call = current
	callLabel = firstNonEmpty(call.label(), fmt.Sprintf("%s tool", call.Kind))
	durationStr = formatActiveCallTimeoutDuration(elapsed)
	prompt = renderActiveCallTimeoutPrompt(settings.PromptTemplate, callLabel, durationStr)

	run.activeCallInFlight = true
	run.abortPayload = map[string]any{
		"reason":    activeCallTimeoutReason,
		"msg":       fmt.Sprintf("The current %s call timed out after %s and was interrupted automatically.", callLabel, durationStr),
		"call":      callLabel,
		"callKind":  string(call.Kind),
		"elapsedMs": elapsed.Milliseconds(),
	}
	run.clearActiveCallTimerLocked()
	run.mu.Unlock()

	if err := m.AbortSession(sessionID); err != nil {
		m.appendActiveCallTimeoutNote(sessionID, fmt.Sprintf("Failed to interrupt the timed-out call automatically: %v", err))
		return
	}

	select {
	case <-run.done:
	case <-time.After(activeCallTimeoutAbortWait):
		m.appendActiveCallTimeoutNote(
			sessionID,
			fmt.Sprintf("Timed out waiting for the interrupted %s call to stop. Automatic continue was skipped.", callLabel),
		)
		return
	}

	if !m.waitForRunCleanup(sessionID, activeCallTimeoutCleanupWait) {
		m.appendActiveCallTimeoutNote(
			sessionID,
			fmt.Sprintf("The interrupted %s call did not finish cleaning up in time. Automatic continue was skipped.", callLabel),
		)
		return
	}

	record, err := m.GetSession(context.Background(), sessionID)
	if err != nil || record.ArchivedAt != nil {
		return
	}
	if strings.TrimSpace(prompt) == "" {
		m.appendActiveCallTimeoutNote(sessionID, "The automatic continue prompt was empty, so no follow-up message was sent.")
		return
	}
	if err := m.sendMessageInternal(context.Background(), sessionID, prompt, nil, sendMessageOptions{}); err != nil {
		m.appendActiveCallTimeoutNote(
			sessionID,
			fmt.Sprintf("Failed to send the automatic continue prompt after interrupting %s: %v", callLabel, err),
		)
	}
}

func (m *Manager) appendActiveCallTimeoutNote(sessionID string, message string) {
	if strings.TrimSpace(sessionID) == "" || strings.TrimSpace(message) == "" {
		return
	}
	record, err := m.GetSession(context.Background(), sessionID)
	if err != nil {
		return
	}
	_, _ = m.appendAndBroadcast(context.Background(), sessionID, record, Event{
		ID:        utils.NewID(),
		Seq:       0,
		Type:      "note",
		Timestamp: time.Now(),
		Payload: map[string]any{
			"txt": strings.TrimSpace(message),
			"lvl": "warn",
		},
	})
}

func (m *Manager) waitForRunCleanup(sessionID string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !m.hasActiveRun(sessionID) {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return !m.hasActiveRun(sessionID)
}

func activeCallTimeoutKindFromTool(toolKind string) (activeCallTimeoutKind, bool) {
	switch normalizeCodexItemType(toolKind) {
	case "mcp_tool_call":
		return activeCallTimeoutKindMCP, true
	case "command_execution":
		return activeCallTimeoutKindCommand, true
	case "sub_agent_tool_call":
		return activeCallTimeoutKindSubAgent, true
	case "", "agent_message", "user_message", "reasoning", "plan", "context_compaction":
		return "", false
	default:
		return activeCallTimeoutKindTool, true
	}
}

func renderActiveCallTimeoutPrompt(template string, call string, duration string) string {
	rendered := strings.TrimSpace(template)
	if rendered == "" {
		rendered = utils.DefaultWebSessionActiveCallTimeoutPrompt
	}
	replacements := map[string]string{
		"call":     call,
		"duration": duration,
	}
	for key, value := range replacements {
		rendered = strings.ReplaceAll(rendered, "${"+key+"}", value)
	}
	return strings.TrimSpace(rendered)
}

func formatActiveCallTimeoutDuration(value time.Duration) string {
	if value <= 0 {
		return "0s"
	}
	value = value.Round(time.Second)
	if value < time.Second {
		value = time.Second
	}

	hours := value / time.Hour
	value -= hours * time.Hour
	minutes := value / time.Minute
	value -= minutes * time.Minute
	seconds := value / time.Second

	parts := make([]string, 0, 3)
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if minutes > 0 {
		parts = append(parts, fmt.Sprintf("%dm", minutes))
	}
	if seconds > 0 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%ds", seconds))
	}
	return strings.Join(parts, " ")
}

func (c trackedActiveCall) elapsedAt(now time.Time) time.Duration {
	if now.IsZero() {
		now = time.Now()
	}
	elapsed := now.Sub(c.StartedAt) - c.PauseTotal
	if elapsed < 0 {
		return 0
	}
	return elapsed
}

func (c trackedActiveCall) label() string {
	title := strings.TrimSpace(firstNonEmpty(stringValue(c.Meta["title"]), c.Name, string(c.Kind)))
	summary := strings.TrimSpace(compactToolSummary(c.ToolKind, c.Input, c.Meta, ""))
	if summary == "" {
		return title
	}
	return fmt.Sprintf("%s (%s)", title, summary)
}

func (s activeCallTimeoutSettings) tracks(kind activeCallTimeoutKind) bool {
	switch kind {
	case activeCallTimeoutKindMCP:
		return s.TrackMCP
	case activeCallTimeoutKindCommand:
		return s.TrackCommand
	case activeCallTimeoutKindTool:
		return s.TrackTool
	case activeCallTimeoutKindSubAgent:
		return false
	default:
		return false
	}
}

func (r *activeRun) currentTrackedActiveCallLocked(
	settings activeCallTimeoutSettings,
) (trackedActiveCall, bool) {
	var (
		target trackedActiveCall
		found  bool
	)
	for _, call := range r.activeCalls {
		if !settings.tracks(call.Kind) {
			continue
		}
		if !found || call.StartedAt.After(target.StartedAt) {
			target = call
			found = true
		}
	}
	return target, found
}

func (r *activeRun) clearActiveCallTimerLocked() {
	if r.activeCallTimer != nil {
		r.activeCallTimer.Stop()
		r.activeCallTimer = nil
	}
}

func (r *activeRun) resetActiveCallTracking() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clearActiveCallTimerLocked()
	r.activeCalls = nil
	r.activeCallPausedAt = nil
	r.activeCallInFlight = false
	r.abortPayload = nil
}

func (r *activeRun) abortEventPayload() map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()
	return cloneMap(r.abortPayload)
}

func timeoutAbortPayload(payload map[string]any) map[string]any {
	if len(payload) == 0 {
		return nil
	}
	return cloneMap(payload)
}

func activeCallTimeoutAbortPayload(record tables.WebSessionTable, payload map[string]any) map[string]any {
	merged := timeoutAbortPayload(payload)
	if merged == nil {
		return nil
	}
	merged["prevStatus"] = record.Status
	return merged
}
