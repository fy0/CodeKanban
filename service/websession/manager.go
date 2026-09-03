package websession

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"code-kanban/model"
	"code-kanban/model/tables"
	"code-kanban/service"
	"code-kanban/utils"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	DefaultHistoryWindow          = 80
	MaxHistoryWindow              = 120
	MaxSessionReconcileTargets    = 256
	sessionOrderStep              = 1000.0
	defaultToolOutputLimit        = 4000
	planPromptPreamble            = "You are operating in planning mode. Inspect the project first, summarize the goal, and propose a concrete plan before making changes. Do not mutate files until the user confirms execution or explicitly asks you to proceed immediately. If additional permissions are needed, call them out explicitly."
	goalBootstrapPromptPreamble   = "<goal_bootstrap>\nGoal:\n%s\n</goal_bootstrap>\nStart working on the goal above now."
	recoveryReasonProcessRestart  = "process_restart"
	recoveryMessageProcessRestart = "Session runtime was interrupted because the app restarted. Send a new message to continue."
	errCodexNotInstalled          = "Codex is not installed. Install Codex before sending messages in this session."
	errClaudeCodeNotInstalled     = "Claude Code is not installed. Install Claude Code before sending messages in this session."
	errPiWebSessionUnavailable    = "Pi Web Sessions require a compatible Pi RPC runtime."
)

var (
	ErrCodexMultiAgentV2Unavailable = errors.New("Codex multi-agent V2 is unavailable")
	// ErrCodexWebSessionUnavailable is kept as an API compatibility alias.
	ErrCodexWebSessionUnavailable = ErrCodexMultiAgentV2Unavailable
	webSessionHeartbeatInterval   = 15 * time.Second
	webSessionHeartbeatTimeout    = 45 * time.Second
)

type codexMultiAgentV2UnavailableError struct {
	message string
}

func (e codexMultiAgentV2UnavailableError) Error() string {
	return e.message
}

func (e codexMultiAgentV2UnavailableError) Is(target error) bool {
	return target == ErrCodexMultiAgentV2Unavailable
}

type eventPersistedError struct {
	event Event
	err   error
}

type eventProjectionStage uint8

const (
	eventProjectionDatabase eventProjectionStage = iota
	eventProjectionBroadcast
	eventProjectionDone
)

type eventProjectionRetry struct {
	record     tables.WebSessionTable
	event      Event
	stage      eventProjectionStage
	attempts   int
	retryDelay time.Duration
	cachedItem *HistoryItem
	timingItem *HistoryItem
	subAgent   *WebSessionSubAgent
}

func (e eventPersistedError) Error() string {
	return e.err.Error()
}

func (e eventPersistedError) Unwrap() error {
	return e.err
}

func persistedEventFromError(err error, eventID string) (Event, bool) {
	var persisted eventPersistedError
	if !errors.As(err, &persisted) || persisted.event.ID != strings.TrimSpace(eventID) {
		return Event{}, false
	}
	return persisted.event, true
}

type Config struct {
	DataDir                     string
	AttachmentSizeLimit         int64
	RemoteAttachmentClient      *http.Client
	ClaudePath                  string
	CCRPath                     string
	CCRConfigPath               string
	CodexPath                   string
	PiPath                      string
	PiRuntimeIdleTTL            time.Duration
	DefaultCodexModel           func() string
	DefaultCodexReasoningEffort func() ReasoningEffort
	DefaultCodexPermissionLevel func() string
	DefaultCodexSyncMode        func() SyncMode
	AutoRetryDefaultsConfig     func() utils.WebSessionAutoRetryDefaultsConfig
	ActiveCallTimeoutConfig     func() utils.WebSessionActiveCallTimeoutConfig
}

type Manager struct {
	cfg           Config
	logger        *zap.Logger
	store         *store
	projectSvc    *model.ProjectService
	worktreeSvc   *service.WorktreeService
	aiSessionSvc  *service.AISessionService
	agentTrustSvc *service.ProjectAgentTrustService

	eventStatesMu        sync.Mutex
	eventStates          map[string]*sessionEventState
	textDeltaFlushWindow time.Duration

	mu   sync.RWMutex
	runs map[string]*activeRun
	// Codex can report turn completion before its app-server releases the thread writer.
	codexRunDrains              map[string]*activeRun
	codexTerminationRequests    map[string]codexTerminationRequest
	clients                     map[*client]struct{}
	autoRetryTimers             map[string]*time.Timer
	scheduledInputTimers        map[string]*time.Timer
	scheduledInputTimerSessions map[string]string
	pendingInputTimers          map[string]*time.Timer
	pendingInputTimerDeadlines  map[string]time.Time
	pendingEpoch                string
	pendingVersions             map[string]uint64
	pendingDelivered            map[string]map[string]PendingInput
	pendingDeliveredOrder       map[string][]string
	scheduledIdleTimer          *time.Timer
	scheduledIdleSweepMu        sync.Mutex
	scheduledInputLocks         [64]sync.Mutex
	scheduledProjectLocks       [64]sync.Mutex
	scheduledBroadcastLocks     [64]sync.Mutex
	sessionDispatchLocks        [64]sync.Mutex
	revisionBroadcastLocks      [64]sync.Mutex
	pendingInputs               map[string][]PendingInput
	piNativeQueuedInputs        map[string][]PendingInput
	pendingProcessing           map[string]bool
	pendingDirty                map[string]bool
	codexContextWindow          codexContextWindowResolver
	piProbe                     piRuntimeProbeCache
	runtimeCapabilityProbes     runtimeCapabilityProbeHooks
	piRuntimeMu                 sync.Mutex
	piRuntimeTerminators        map[string]piRuntimeTerminator
	piRuntimes                  map[string]*piSessionRuntime
	claudeHookOnce              sync.Once
	claudeHookBaseURL           string
	claudeHookToken             string
	claudeHookSettingsPath      string
	claudeHookErr               error
	claudeHookServer            *http.Server
	ccrHookMu                   sync.Mutex
	ccrHookReady                bool
	ccrHookErr                  error
	ccrHookClaudePath           string
	historyCleanupMu            sync.Mutex
	workTimingBackfillMu        sync.Mutex
	workTimingLocks             [64]sync.Mutex
}

type clientKind string

const (
	clientKindCommand clientKind = "command"
	clientKindEvent   clientKind = "event"
)

var ErrSessionHistoryUnavailable = errors.New("session history not found")

var (
	ErrCodexAppServerNotActive       = errors.New("codex app-server is not active")
	ErrCodexAppServerProjectMismatch = errors.New("codex app-server session does not belong to the project")
	ErrCodexRunDrainTimeout          = errors.New("codex app-server is still shutting down; retry the command")
)

type client struct {
	conn               wsConn
	logger             *zap.Logger
	kind               clientKind
	commandQueue       chan queuedCommand
	commandCancel      context.CancelFunc
	commandMu          sync.Mutex
	commandObservation *commandObservation
	writeMu            sync.Mutex
	focusMu            sync.RWMutex
	focusedSID         string
	done               chan struct{}
	once               sync.Once
	lastSeenAt         atomic.Int64
}

const (
	commandClientQueueCapacity     = 32
	slowWebSessionCommandThreshold = time.Second
)

type queuedCommand struct {
	payload    []byte
	receivedAt time.Time
	queueDepth int
}

type commandObservation struct {
	requestID    string
	operation    string
	sessionID    string
	responseKind string
	errorCode    string
	retryable    bool
	fields       []zap.Field
}

type wsConn interface {
	ReadMessage() (messageType int, p []byte, err error)
	WriteJSON(v any) error
	Close() error
}

type CodexAppServerTermination struct {
	SessionID        string                `json:"sessionId"`
	RunID            string                `json:"runId"`
	StateBefore      string                `json:"stateBefore"`
	ProcessRootPID   int                   `json:"processRootPid"`
	AlreadyRequested bool                  `json:"alreadyRequested"`
	Runtime          CodexAppServerRuntime `json:"runtime"`
}

type codexTerminationRequest struct {
	projectID string
	result    CodexAppServerTermination
	expiresAt time.Time
}

const codexTerminationRequestTTL = 30 * time.Second

type attachmentMeta struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Mime      string    `json:"mime"`
	Size      int64     `json:"size"`
	Path      string    `json:"path"`
	CreatedAt time.Time `json:"createdAt"`
}

func normalizeAssistantState(state AssistantState) AssistantState {
	switch strings.ToLower(strings.TrimSpace(string(state))) {
	case string(AssistantStateWorking):
		return AssistantStateWorking
	case string(AssistantStateWaitingApproval):
		return AssistantStateWaitingApproval
	case string(AssistantStateWaitingInput):
		return AssistantStateWaitingInput
	case string(AssistantStateWaitingPlanApproval):
		return AssistantStateWaitingPlanApproval
	default:
		return AssistantStateNone
	}
}

func normalizeAutoRetryScope(scope AutoRetryScope) AutoRetryScope {
	switch strings.ToLower(strings.TrimSpace(string(scope))) {
	case string(AutoRetryScopeNetworkAndRateLimit):
		return AutoRetryScopeNetworkAndRateLimit
	case string(AutoRetryScopeAllFailures):
		return AutoRetryScopeAllFailures
	default:
		return AutoRetryScopeNetworkOnly
	}
}

func normalizeAutoRetryPreset(preset AutoRetryPreset) AutoRetryPreset {
	switch strings.ToLower(strings.TrimSpace(string(preset))) {
	case string(AutoRetryPresetAggressiveStop):
		return AutoRetryPresetAggressiveStop
	case string(AutoRetryPresetSustain60s):
		return AutoRetryPresetSustain60s
	default:
		return AutoRetryPresetGentleStop
	}
}

func normalizeAutoRetryPolicyMode(mode AutoRetryPolicyMode) AutoRetryPolicyMode {
	if strings.EqualFold(strings.TrimSpace(string(mode)), string(AutoRetryPolicyModeCustom)) {
		return AutoRetryPolicyModeCustom
	}
	return AutoRetryPolicyModeDefault
}

const maxAutoRetryAttempts = 100

func normalizeAutoRetryMaxAttempts(attempts int) int {
	if attempts < 0 {
		return 0
	}
	if attempts > maxAutoRetryAttempts {
		return maxAutoRetryAttempts
	}
	return attempts
}

type resolvedAutoRetryCreateConfig struct {
	policyMode               AutoRetryPolicyMode
	scope                    AutoRetryScope
	preset                   AutoRetryPreset
	maxAttempts              int
	dispatchPendingOnFailure bool
}

func (m *Manager) autoRetryDefaults() resolvedAutoRetryCreateConfig {
	raw := utils.NormalizeWebSessionAutoRetryDefaultsConfig(utils.WebSessionAutoRetryDefaultsConfig{})
	if m != nil && m.cfg.AutoRetryDefaultsConfig != nil {
		raw = utils.NormalizeWebSessionAutoRetryDefaultsConfig(m.cfg.AutoRetryDefaultsConfig())
	}
	return resolvedAutoRetryCreateConfig{
		policyMode:               AutoRetryPolicyModeDefault,
		scope:                    normalizeAutoRetryScope(AutoRetryScope(raw.Scope)),
		preset:                   normalizeAutoRetryPreset(AutoRetryPreset(raw.Preset)),
		maxAttempts:              normalizeAutoRetryMaxAttempts(raw.MaxAttempts),
		dispatchPendingOnFailure: raw.DispatchPendingOnFailure,
	}
}

func (m *Manager) resolveAutoRetryCreateConfig(params CreateParams) resolvedAutoRetryCreateConfig {
	resolved := m.autoRetryDefaults()
	hasPolicyOverride := params.AutoRetryScope != nil ||
		params.AutoRetryPreset != nil ||
		params.AutoRetryMaxAttempts != nil
	if params.AutoRetryPolicyMode != nil {
		resolved.policyMode = normalizeAutoRetryPolicyMode(*params.AutoRetryPolicyMode)
	} else if hasPolicyOverride {
		resolved.policyMode = AutoRetryPolicyModeCustom
	}
	if resolved.policyMode == AutoRetryPolicyModeCustom {
		if params.AutoRetryScope != nil {
			resolved.scope = normalizeAutoRetryScope(*params.AutoRetryScope)
		}
		if params.AutoRetryPreset != nil {
			resolved.preset = normalizeAutoRetryPreset(*params.AutoRetryPreset)
		}
		if params.AutoRetryMaxAttempts != nil {
			resolved.maxAttempts = normalizeAutoRetryMaxAttempts(*params.AutoRetryMaxAttempts)
		}
	}
	if params.AutoRetryDispatchPendingOnFailure != nil {
		resolved.dispatchPendingOnFailure = *params.AutoRetryDispatchPendingOnFailure
	}
	return resolved
}

func (m *Manager) resolveAutoRetryUpdateConfig(
	record tables.WebSessionTable,
	policyMode *AutoRetryPolicyMode,
	scope *AutoRetryScope,
	preset *AutoRetryPreset,
	maxAttempts *int,
) resolvedAutoRetryCreateConfig {
	hasPolicyOverride := scope != nil || preset != nil || maxAttempts != nil
	mode := normalizeAutoRetryPolicyMode(AutoRetryPolicyMode(record.AutoRetryPolicyMode))
	if policyMode != nil {
		mode = normalizeAutoRetryPolicyMode(*policyMode)
	} else if hasPolicyOverride {
		mode = AutoRetryPolicyModeCustom
	}

	if mode == AutoRetryPolicyModeDefault {
		resolved := m.autoRetryDefaults()
		resolved.dispatchPendingOnFailure = record.AutoRetryDispatchPendingOnFailure
		return resolved
	}

	resolved := resolvedAutoRetryCreateConfig{
		policyMode:               AutoRetryPolicyModeCustom,
		scope:                    normalizeAutoRetryScope(AutoRetryScope(record.AutoRetryScope)),
		preset:                   normalizeAutoRetryPreset(AutoRetryPreset(record.AutoRetryPreset)),
		maxAttempts:              normalizeAutoRetryMaxAttempts(record.AutoRetryMaxAttempts),
		dispatchPendingOnFailure: record.AutoRetryDispatchPendingOnFailure,
	}
	if scope != nil {
		resolved.scope = normalizeAutoRetryScope(*scope)
	}
	if preset != nil {
		resolved.preset = normalizeAutoRetryPreset(*preset)
	}
	if maxAttempts != nil {
		resolved.maxAttempts = normalizeAutoRetryMaxAttempts(*maxAttempts)
	}
	return resolved
}

func (m *Manager) refreshDefaultAutoRetryPolicy(record *tables.WebSessionTable) map[string]any {
	if record == nil || normalizeAutoRetryPolicyMode(AutoRetryPolicyMode(record.AutoRetryPolicyMode)) != AutoRetryPolicyModeDefault {
		return nil
	}
	defaults := m.autoRetryDefaults()
	record.AutoRetryPolicyMode = string(AutoRetryPolicyModeDefault)
	record.AutoRetryScope = string(defaults.scope)
	record.AutoRetryPreset = string(defaults.preset)
	record.AutoRetryMaxAttempts = defaults.maxAttempts
	return map[string]any{
		"auto_retry_policy_mode":  string(AutoRetryPolicyModeDefault),
		"auto_retry_scope":        string(defaults.scope),
		"auto_retry_preset":       string(defaults.preset),
		"auto_retry_max_attempts": defaults.maxAttempts,
	}
}

func effectiveAssistantState(record tables.WebSessionTable) AssistantState {
	if normalized := normalizeAssistantState(AssistantState(record.AssistantState)); normalized != AssistantStateNone {
		return normalized
	}
	if strings.EqualFold(strings.TrimSpace(record.Status), string(StatusWaitingApproval)) {
		return AssistantStateWaitingPlanApproval
	}
	return AssistantStateNone
}

func effectiveAssistantStateUpdatedAt(record tables.WebSessionTable, state AssistantState) *time.Time {
	if record.AssistantStateUpdatedAt != nil {
		return record.AssistantStateUpdatedAt
	}
	if state == AssistantStateWaitingPlanApproval && strings.EqualFold(strings.TrimSpace(record.Status), string(StatusWaitingApproval)) {
		value := record.UpdatedAt
		return &value
	}
	return nil
}

func effectiveStatusUpdatedAt(record tables.WebSessionTable, assistantState AssistantState) *time.Time {
	if record.StatusUpdatedAt != nil {
		return record.StatusUpdatedAt
	}
	if assistantStateUpdatedAt := effectiveAssistantStateUpdatedAt(record, assistantState); assistantStateUpdatedAt != nil {
		value := *assistantStateUpdatedAt
		return &value
	}
	if !record.UpdatedAt.IsZero() {
		value := record.UpdatedAt
		return &value
	}
	if !record.CreatedAt.IsZero() {
		value := record.CreatedAt
		return &value
	}
	return nil
}

func effectiveStatus(record tables.WebSessionTable, assistantState AssistantState) Status {
	switch strings.ToLower(strings.TrimSpace(record.Status)) {
	case string(StatusRunning):
		return StatusRunning
	case string(StatusWaitingApproval):
		if assistantState == AssistantStateWaitingPlanApproval {
			return StatusRunning
		}
		return StatusWaitingApproval
	case string(StatusDone):
		return StatusDone
	case string(StatusError):
		return StatusError
	case string(StatusAborting):
		return StatusAborting
	default:
		return StatusIdle
	}
}

func applyAssistantStateUpdates(updates map[string]any, state AssistantState, updatedAt time.Time) map[string]any {
	if updates == nil {
		updates = map[string]any{}
	}
	updates["status_updated_at"] = updatedAt
	normalized := normalizeAssistantState(state)
	if normalized == AssistantStateNone {
		updates["assistant_state"] = nil
		updates["assistant_state_updated_at"] = nil
		return updates
	}
	updates["assistant_state"] = string(normalized)
	updates["assistant_state_updated_at"] = updatedAt
	return updates
}

func NewManager(cfg Config, logger *zap.Logger) (*Manager, error) {
	if cfg.DataDir == "" {
		cfg.DataDir = utils.GetDataDir()
	}
	if cfg.AttachmentSizeLimit <= 0 {
		cfg.AttachmentSizeLimit = 10 * 1024 * 1024
	}
	if cfg.ClaudePath == "" {
		cfg.ClaudePath = getenvDefault("CLAUDE_PATH", "claude")
	}
	if cfg.CCRPath == "" {
		cfg.CCRPath = getenvDefault("CCR_PATH", "ccr")
	}
	if cfg.CCRConfigPath == "" {
		cfg.CCRConfigPath = getenvDefault("CCR_CONFIG_PATH", defaultCCRConfigPath())
	}
	if cfg.CodexPath == "" {
		cfg.CodexPath = getenvDefault("CODEX_PATH", "codex")
	}
	if cfg.PiPath == "" {
		cfg.PiPath = getenvDefault("PI_PATH", "pi")
	}
	if cfg.PiRuntimeIdleTTL <= 0 {
		cfg.PiRuntimeIdleTTL = 2 * time.Minute
	}
	if logger == nil {
		logger = utils.Logger()
	}

	eventStore, err := newStore(cfg.DataDir)
	if err != nil {
		return nil, err
	}

	manager := &Manager{
		cfg:                         cfg,
		logger:                      logger.Named("web-session-manager"),
		store:                       eventStore,
		projectSvc:                  model.NewProjectService(),
		worktreeSvc:                 service.NewWorktreeService(),
		aiSessionSvc:                service.NewAISessionService(),
		agentTrustSvc:               service.NewProjectAgentTrustService(),
		piRuntimeTerminators:        make(map[string]piRuntimeTerminator),
		piRuntimes:                  make(map[string]*piSessionRuntime),
		runs:                        make(map[string]*activeRun),
		codexRunDrains:              make(map[string]*activeRun),
		codexTerminationRequests:    make(map[string]codexTerminationRequest),
		clients:                     make(map[*client]struct{}),
		autoRetryTimers:             make(map[string]*time.Timer),
		scheduledInputTimers:        make(map[string]*time.Timer),
		scheduledInputTimerSessions: make(map[string]string),
		pendingInputTimers:          make(map[string]*time.Timer),
		pendingInputTimerDeadlines:  make(map[string]time.Time),
		pendingEpoch:                utils.NewID(),
		pendingVersions:             make(map[string]uint64),
		pendingDelivered:            make(map[string]map[string]PendingInput),
		pendingDeliveredOrder:       make(map[string][]string),
		pendingInputs:               make(map[string][]PendingInput),
		piNativeQueuedInputs:        make(map[string][]PendingInput),
		pendingProcessing:           make(map[string]bool),
		pendingDirty:                make(map[string]bool),
		eventStates:                 make(map[string]*sessionEventState),
		textDeltaFlushWindow:        defaultTextDeltaFlushWindow,
	}
	manager.loadCodexContextConfig(false)
	if err := manager.migrateLegacySessionModes(context.Background()); err != nil {
		return nil, err
	}
	removedStatusNotes, err := manager.cleanupLegacyPiStatusNotes(context.Background())
	if err != nil {
		return nil, err
	}
	if removedStatusNotes > 0 {
		manager.logger.Info("legacy Pi extension status notes removed", zap.Int64("rowCount", removedStatusNotes))
	}
	if err := manager.backfillSessionActivityAt(context.Background()); err != nil {
		return nil, err
	}
	// Keep startup independent of total history size. Interrupted sessions read
	// only their durable tail below; older projection gaps require explicit sync.
	if err := manager.recoverInterruptedSessions(context.Background()); err != nil {
		return nil, err
	}
	if err := manager.recoverPendingAutoRetrySessions(context.Background()); err != nil {
		return nil, err
	}
	if err := manager.recoverPendingScheduledInputs(context.Background()); err != nil {
		return nil, err
	}
	return manager, nil
}

func (m *Manager) registerClient(conn wsConn, kind clientKind) *client {
	client := &client{
		conn:   conn,
		logger: m.logger.Named("client"),
		kind:   kind,
		done:   make(chan struct{}),
	}
	if kind == clientKindCommand {
		workerCtx, cancel := context.WithCancel(context.Background())
		client.commandQueue = make(chan queuedCommand, commandClientQueueCapacity)
		client.commandCancel = cancel
		go m.runCommandWorker(workerCtx, client)
	}
	client.MarkSeen()
	m.mu.Lock()
	m.clients[client] = struct{}{}
	m.mu.Unlock()
	client.startHeartbeat()
	return client
}

func (m *Manager) RegisterCommandClient(conn wsConn) *client {
	return m.registerClient(conn, clientKindCommand)
}

func (m *Manager) RegisterEventClient(conn wsConn) *client {
	return m.registerClient(conn, clientKindEvent)
}

var autoRetryNetworkFailureKeywords = []string{
	"network",
	"timeout",
	"timed out",
	"connection reset",
	"connection closed",
	"connection failed",
	"socket hang up",
	"transport error",
	"temporarily unavailable",
	"upstream service temporarily unavailable",
	"bad gateway",
	"502",
	"websocket",
}

var autoRetryRateLimitFailureKeywords = []string{
	"429",
	"rate limit",
	"too many requests",
}

func shouldAutoRetryFailure(scope AutoRetryScope, code string, message string) bool {
	normalizedScope := normalizeAutoRetryScope(scope)
	normalizedCode := normalizeCodexErrorInfo(code)
	if normalizedCode == codexCyberPolicyErrorCode || isCodexCyberPolicyMessage(message) {
		return false
	}
	if isCodexModelCapacityError(normalizedCode, message) {
		return true
	}
	if normalizedScope == AutoRetryScopeAllFailures {
		return true
	}
	normalizedMessage := strings.ToLower(strings.TrimSpace(message))
	isNetworkFailure := normalizedCode == codexTransportRetryExhaustedCode
	if !isNetworkFailure {
		for _, keyword := range autoRetryNetworkFailureKeywords {
			if strings.Contains(normalizedMessage, keyword) {
				isNetworkFailure = true
				break
			}
		}
	}
	if isNetworkFailure {
		return true
	}
	if normalizedScope != AutoRetryScopeNetworkAndRateLimit {
		return false
	}
	for _, keyword := range autoRetryRateLimitFailureKeywords {
		if strings.Contains(normalizedMessage, keyword) {
			return true
		}
	}
	return false
}

func autoRetryDelay(preset AutoRetryPreset, attempt int) (time.Duration, bool) {
	if attempt <= 0 {
		return 0, false
	}
	switch normalizeAutoRetryPreset(preset) {
	case AutoRetryPresetAggressiveStop:
		delays := []time.Duration{2 * time.Second, 5 * time.Second, 15 * time.Second, 30 * time.Second, 60 * time.Second}
		if attempt > len(delays) {
			return 0, false
		}
		return delays[attempt-1], true
	case AutoRetryPresetSustain60s:
		delays := []time.Duration{3 * time.Second, 10 * time.Second, 30 * time.Second}
		if attempt <= len(delays) {
			return delays[attempt-1], true
		}
		return 60 * time.Second, true
	default:
		delays := []time.Duration{3 * time.Second, 10 * time.Second, 30 * time.Second, 60 * time.Second}
		if attempt > len(delays) {
			return 0, false
		}
		return delays[attempt-1], true
	}
}

func autoRetryDelayForFailure(
	preset AutoRetryPreset,
	attempt int,
	code string,
	message string,
) (time.Duration, bool) {
	return autoRetryDelayForFailureWithMax(preset, attempt, 0, code, message)
}

func autoRetryDelayForFailureWithMax(
	preset AutoRetryPreset,
	attempt int,
	maxAttempts int,
	code string,
	message string,
) (time.Duration, bool) {
	if attempt == 1 && isCodexModelCapacityError(code, message) {
		if maxAttempts > 0 && attempt > maxAttempts {
			return 0, false
		}
		return 3 * time.Second, true
	}

	normalizedPreset := normalizeAutoRetryPreset(preset)
	normalizedMaxAttempts := normalizeAutoRetryMaxAttempts(maxAttempts)
	if normalizedMaxAttempts == 0 {
		return autoRetryDelay(normalizedPreset, attempt)
	}
	if attempt <= 0 || attempt > normalizedMaxAttempts {
		return 0, false
	}

	switch normalizedPreset {
	case AutoRetryPresetAggressiveStop:
		delays := []time.Duration{2 * time.Second, 5 * time.Second, 15 * time.Second, 30 * time.Second, 60 * time.Second}
		if attempt <= len(delays) {
			return delays[attempt-1], true
		}
		return delays[len(delays)-1], true
	case AutoRetryPresetSustain60s:
		delays := []time.Duration{3 * time.Second, 10 * time.Second, 30 * time.Second}
		if attempt <= len(delays) {
			return delays[attempt-1], true
		}
		return 60 * time.Second, true
	default:
		delays := []time.Duration{3 * time.Second, 10 * time.Second, 30 * time.Second, 60 * time.Second}
		if attempt <= len(delays) {
			return delays[attempt-1], true
		}
		return delays[len(delays)-1], true
	}
}

func (m *Manager) UnregisterClient(client *client) {
	if client == nil {
		return
	}
	client.stop()
	m.mu.Lock()
	delete(m.clients, client)
	m.mu.Unlock()
}

func (c *client) MarkSeen() {
	if c == nil {
		return
	}
	c.lastSeenAt.Store(time.Now().UnixMilli())
}

func (c *client) SetFocusedSessionID(sessionID string) {
	if c == nil {
		return
	}
	c.focusMu.Lock()
	c.focusedSID = strings.TrimSpace(sessionID)
	c.focusMu.Unlock()
}

func (c *client) FocusedSessionID() string {
	if c == nil {
		return ""
	}
	c.focusMu.RLock()
	defer c.focusMu.RUnlock()
	return c.focusedSID
}

func (c *client) stop() {
	if c == nil {
		return
	}
	c.once.Do(func() {
		if c.commandCancel != nil {
			c.commandCancel()
		}
		close(c.done)
	})
}

func (m *Manager) EnqueueCommand(client *client, payload []byte) error {
	if client == nil || client.kind != clientKindCommand || client.commandQueue == nil {
		return fmt.Errorf("command client is not registered")
	}
	receivedAt := time.Now()
	command := queuedCommand{
		payload:    append([]byte(nil), payload...),
		receivedAt: receivedAt,
		queueDepth: len(client.commandQueue),
	}
	select {
	case <-client.done:
		return context.Canceled
	case client.commandQueue <- command:
		return nil
	default:
		var frame wireCommandFrame
		_ = json.Unmarshal(payload, &frame)
		sendErr := client.send(newErrorFrame(
			frame.RequestID,
			frame.SessionID,
			"command_queue_full",
			"command queue is full; retry shortly",
			true,
		))
		if client.logger != nil {
			result := "queue_full"
			if sendErr != nil {
				result = "response_write_failed"
			}
			client.logger.Warn("web session command completed",
				zap.String("operation", frame.Operation),
				zap.String("requestId", frame.RequestID),
				zap.String("sessionId", frame.SessionID),
				zap.String("result", result),
				zap.String("responseKind", wireKindError),
				zap.String("errorCode", "command_queue_full"),
				zap.Bool("retryable", true),
				zap.Duration("queueWait", 0),
				zap.Duration("handlerDuration", 0),
				zap.Duration("duration", time.Since(receivedAt)),
				zap.Int("queueDepth", len(client.commandQueue)),
			)
		}
		return sendErr
	}
}

func (m *Manager) runCommandWorker(ctx context.Context, client *client) {
	for {
		select {
		case <-ctx.Done():
			return
		case command := <-client.commandQueue:
			if ctx.Err() != nil {
				return
			}
			var frame wireCommandFrame
			_ = json.Unmarshal(command.payload, &frame)
			client.beginCommandObservation(frame)
			handlerStartedAt := time.Now()
			handlerErr := m.HandleCommand(ctx, client, command.payload)
			completedAt := time.Now()
			observation := client.finishCommandObservation()
			m.logCommandObservation(
				client,
				observation,
				handlerErr,
				handlerStartedAt.Sub(command.receivedAt),
				completedAt.Sub(handlerStartedAt),
				completedAt.Sub(command.receivedAt),
				command.queueDepth,
			)
		}
	}
}

func (c *client) beginCommandObservation(frame wireCommandFrame) {
	if c == nil {
		return
	}
	c.commandMu.Lock()
	c.commandObservation = &commandObservation{
		requestID: frame.RequestID,
		operation: frame.Operation,
		sessionID: frame.SessionID,
	}
	c.commandMu.Unlock()
}

func (c *client) addCommandObservationFields(fields ...zap.Field) {
	if c == nil || len(fields) == 0 {
		return
	}
	c.commandMu.Lock()
	if c.commandObservation != nil {
		c.commandObservation.fields = append(c.commandObservation.fields, fields...)
	}
	c.commandMu.Unlock()
}

func (c *client) observeCommandResponse(frame wireFrame) {
	if c == nil || (frame.Kind != wireKindAck && frame.Kind != wireKindError) {
		return
	}
	c.commandMu.Lock()
	defer c.commandMu.Unlock()
	observation := c.commandObservation
	if observation == nil || frame.RequestID != observation.requestID {
		return
	}
	observation.responseKind = frame.Kind
	observation.errorCode = frame.Code
	observation.retryable = frame.Retry
}

func (c *client) finishCommandObservation() commandObservation {
	if c == nil {
		return commandObservation{}
	}
	c.commandMu.Lock()
	defer c.commandMu.Unlock()
	if c.commandObservation == nil {
		return commandObservation{}
	}
	result := *c.commandObservation
	result.fields = append([]zap.Field(nil), c.commandObservation.fields...)
	c.commandObservation = nil
	return result
}

func (m *Manager) logCommandObservation(
	client *client,
	observation commandObservation,
	handlerErr error,
	queueWait time.Duration,
	handlerDuration time.Duration,
	duration time.Duration,
	queueDepth int,
) {
	if client == nil || client.logger == nil {
		return
	}
	if queueWait < 0 {
		queueWait = 0
	}
	result := "success"
	errorCode := observation.errorCode
	switch {
	case observation.responseKind == wireKindError:
		result = "error"
	case handlerErr != nil:
		result = "response_write_failed"
		if errorCode == "" {
			errorCode = "response_write_failed"
		}
	case observation.responseKind == "":
		result = "no_response"
		if errorCode == "" {
			errorCode = "missing_response"
		}
	}
	fields := []zap.Field{
		zap.String("operation", observation.operation),
		zap.String("requestId", observation.requestID),
		zap.String("sessionId", observation.sessionID),
		zap.String("result", result),
		zap.String("responseKind", observation.responseKind),
		zap.String("errorCode", errorCode),
		zap.Bool("retryable", observation.retryable),
		zap.Duration("queueWait", queueWait),
		zap.Duration("handlerDuration", handlerDuration),
		zap.Duration("duration", duration),
		zap.Int("queueDepth", queueDepth),
	}
	fields = append(fields, observation.fields...)

	switch {
	case result != "success", duration >= slowWebSessionCommandThreshold:
		client.logger.Warn("web session command completed", fields...)
	case observation.operation == "schedule_send":
		client.logger.Info("web session command completed", fields...)
	default:
		client.logger.Debug("web session command completed", fields...)
	}
}

func (c *client) closeWithReason(reason string) {
	if c == nil {
		return
	}
	if c.logger != nil && strings.TrimSpace(reason) != "" {
		c.logger.Debug("closing web session websocket", zap.String("reason", reason))
	}
	c.stop()
	_ = c.conn.Close()
}

func (c *client) startHeartbeat() {
	if c == nil {
		return
	}
	go func() {
		interval := webSessionHeartbeatInterval
		if interval <= 0 {
			interval = 15 * time.Second
		}
		timeout := webSessionHeartbeatTimeout
		if timeout <= interval {
			timeout = interval * 3
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-c.done:
				return
			case <-ticker.C:
				lastSeenAt := c.lastSeenAt.Load()
				if lastSeenAt <= 0 {
					c.MarkSeen()
					lastSeenAt = c.lastSeenAt.Load()
				}
				if time.Since(time.UnixMilli(lastSeenAt)) > timeout {
					c.closeWithReason("heartbeat-timeout")
					return
				}
				if err := c.send(newHeartbeatFrame("ping")); err != nil {
					if c.logger != nil {
						c.logger.Debug("failed to send web session heartbeat", zap.Error(err))
					}
					c.closeWithReason("heartbeat-send-failed")
					return
				}
			}
		}
	}()
}

func (m *Manager) ListSessions(ctx context.Context, projectID string) ([]SessionSummary, error) {
	db := model.GetReaderDB()
	if db == nil {
		return nil, model.ErrDBNotInitialized
	}

	records, err := m.listSessionRecordsWithDB(db.WithContext(ctx), projectID)
	if err != nil {
		return nil, err
	}
	records = m.refreshSessionSourceStates(ctx, records)

	contextConfig := m.cachedCodexSessionContextConfig()
	items := make([]SessionSummary, 0, len(records))
	for _, record := range records {
		items = append(items, m.mapSessionSummaryWithContext(record, contextConfig))
	}
	if err := m.decorateScheduledPlanExecutionState(ctx, items); err != nil {
		return nil, err
	}
	return items, nil
}

func (m *Manager) ReconcileSessions(
	ctx context.Context,
	targets []SessionReconcileTarget,
) (SessionReconcileResult, error) {
	result := SessionReconcileResult{
		Items:      []SessionSummary{},
		MissingIDs: []string{},
	}
	if len(targets) == 0 {
		return result, nil
	}
	if len(targets) > MaxSessionReconcileTargets {
		return result, fmt.Errorf("session reconciliation supports at most %d targets", MaxSessionReconcileTargets)
	}

	db := model.GetReaderDB()
	if db == nil {
		return result, model.ErrDBNotInitialized
	}

	normalized := make([]SessionReconcileTarget, 0, len(targets))
	ids := make([]string, 0, len(targets))
	seen := make(map[string]struct{}, len(targets))
	for _, target := range targets {
		id := strings.TrimSpace(target.ID)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		normalized = append(normalized, SessionReconcileTarget{
			ID:       id,
			Revision: strings.TrimSpace(target.Revision),
		})
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return result, nil
	}

	var records []tables.WebSessionTable
	if err := db.WithContext(ctx).Where("id IN ?", ids).Find(&records).Error; err != nil {
		return result, err
	}
	recordsByID := make(map[string]tables.WebSessionTable, len(records))
	for _, record := range records {
		recordsByID[record.ID] = record
	}

	contextConfig := m.cachedCodexSessionContextConfig()
	for _, target := range normalized {
		record, exists := recordsByID[target.ID]
		if !exists {
			result.MissingIDs = append(result.MissingIDs, target.ID)
			continue
		}
		if target.Revision == formatSnapshotRevision(record.SnapshotRevision) {
			continue
		}
		result.Items = append(result.Items, m.mapSessionSummaryWithContext(record, contextConfig))
	}
	if err := m.decorateScheduledPlanExecutionState(ctx, result.Items); err != nil {
		return SessionReconcileResult{}, err
	}
	return result, nil
}

func (m *Manager) CountSessionsByProject(ctx context.Context) (map[string]int, error) {
	db := model.GetReaderDB()
	if db == nil {
		return nil, model.ErrDBNotInitialized
	}

	var rows []struct {
		ProjectID string
		Count     int64
	}
	if err := db.WithContext(ctx).
		Model(&tables.WebSessionTable{}).
		Select("project_id, COUNT(1) AS count").
		Where("archived_at IS NULL").
		Group("project_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	counts := make(map[string]int, len(rows))
	for _, row := range rows {
		projectID := strings.TrimSpace(row.ProjectID)
		if projectID == "" {
			continue
		}
		counts[projectID] = int(row.Count)
	}
	return counts, nil
}

func (m *Manager) QuerySessions(
	ctx context.Context,
	projectIDs []string,
	archived bool,
	limit int,
	offset int,
) (SessionPageResult, error) {
	db := model.GetReaderDB()
	if db == nil {
		return SessionPageResult{}, model.ErrDBNotInitialized
	}

	normalizedProjectIDs := make([]string, 0, len(projectIDs))
	seen := make(map[string]struct{}, len(projectIDs))
	for _, projectID := range projectIDs {
		trimmed := strings.TrimSpace(projectID)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		normalizedProjectIDs = append(normalizedProjectIDs, trimmed)
	}

	query := db.WithContext(ctx).Model(&tables.WebSessionTable{})
	if archived {
		query = query.Where("archived_at IS NOT NULL")
	} else {
		query = query.Where("archived_at IS NULL")
	}
	if len(normalizedProjectIDs) > 0 {
		query = query.Where("project_id IN ?", normalizedProjectIDs)
	}
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return SessionPageResult{}, err
	}
	var records []tables.WebSessionTable
	if err := query.
		Order("activity_at DESC").
		Order("id DESC").
		Offset(offset).
		Limit(limit).
		Find(&records).Error; err != nil {
		return SessionPageResult{}, err
	}

	contextConfig := m.cachedCodexSessionContextConfig()
	items := make([]SessionSummary, 0, len(records))
	for _, record := range records {
		items = append(items, m.mapSessionSummaryWithContext(record, contextConfig))
	}
	if err := m.decorateScheduledPlanExecutionState(ctx, items); err != nil {
		return SessionPageResult{}, err
	}
	nextOffset := offset + len(items)
	return SessionPageResult{
		Items:      items,
		Total:      int(total),
		HasMore:    int64(nextOffset) < total,
		NextOffset: nextOffset,
	}, nil
}

func (m *Manager) CreateSession(ctx context.Context, params CreateParams) (SessionSummary, error) {
	agent, err := validateAgent(params.Agent)
	if err != nil {
		return SessionSummary{}, err
	}
	permissionLevel := m.resolveSessionPermissionLevel(agent, params.PermissionLevel)
	if err := validateWebSessionPermissionLevel(agent, permissionLevel); err != nil {
		return SessionSummary{}, err
	}
	project, worktreeID, cwd, err := m.resolveContext(ctx, params.ProjectID, params.WorktreeID)
	if err != nil {
		return SessionSummary{}, err
	}

	title := strings.TrimSpace(params.Title)
	if title == "" {
		title = defaultTitle(agent, project.Name)
	}

	orderIndex, err := m.getNextSessionOrderIndex(ctx, project.Id)
	if err != nil {
		return SessionSummary{}, err
	}

	modelName := m.resolveSessionModel(agent, params.Model)
	reasoningEffort := m.resolveSessionReasoningEffort(agent, modelName, params.ReasoningEffort)
	if agent == AgentPi {
		if modelName != "" {
			if _, _, err := splitPiModel(modelName); err != nil {
				return SessionSummary{}, err
			}
		}
		if err := validatePiReasoningEffort(reasoningEffort); err != nil {
			return SessionSummary{}, err
		}
	}
	autoRetry := m.resolveAutoRetryCreateConfig(params)
	now := time.Now()
	record := tables.WebSessionTable{
		ProjectID:                         project.Id,
		WorktreeID:                        nilIfEmpty(worktreeID),
		OrderIndex:                        orderIndex,
		Agent:                             string(agent),
		ClaudeRuntime:                     string(normalizeClaudeRuntime(params.ClaudeRuntime)),
		Backend:                           string(normalizeSessionBackend(params.Backend, agent)),
		Title:                             title,
		TitleAuto:                         strings.TrimSpace(params.Title) == "",
		Model:                             modelName,
		ReasoningEffort:                   string(reasoningEffort),
		WorkflowMode:                      string(normalizeWorkflowMode(params.WorkflowMode)),
		SessionStartSource:                string(SessionStartSourceStartup),
		PermissionLevel:                   string(permissionLevel),
		ActiveCallTimeoutEnabled:          params.ActiveCallTimeoutEnabled,
		AutoRetryEnabled:                  params.AutoRetryEnabled,
		AutoRetryPolicyMode:               string(autoRetry.policyMode),
		AutoRetryScope:                    string(autoRetry.scope),
		AutoRetryPreset:                   string(autoRetry.preset),
		AutoRetryMaxAttempts:              autoRetry.maxAttempts,
		AutoRetryDispatchPendingOnFailure: autoRetry.dispatchPendingOnFailure,
		Cwd:                               cwd,
		Status:                            string(StatusIdle),
		AssistantState:                    "",
		HasUnread:                         false,
		ArchivedAt:                        nil,
		ActivityAt:                        now,
		StatusUpdatedAt:                   &now,
		AssistantStateUpdatedAt:           nil,
		SourceKind:                        defaultSourceKind(agent),
		SyncState:                         string(SyncStateMissing),
		LastSyncMode:                      "",
		SourceCreatedAt:                   nil,
		SourceUpdatedAt:                   nil,
		LastSyncedAt:                      nil,
		ThreadPath:                        nil,
		ThreadPreview:                     nil,
		TurnCount:                         0,
		ItemCount:                         0,
		LastEventSeq:                      0,
		WorkTimingBackfillState:           string(WorkTimingBackfillComplete),
		WorkTimingBackfillVersion:         currentWorkTimingBackfillVersion,
		TotalInputTokens:                  0,
		TotalCachedInputTokens:            0,
		TotalOutputTokens:                 0,
		TotalCost:                         0,
	}
	record.Init()

	if err := model.GetDB().WithContext(ctx).Create(&record).Error; err != nil {
		return SessionSummary{}, err
	}

	return m.mapSessionSummary(record), nil
}

func importedCodexSourceCreatedAt(source tables.AISessionTable) *time.Time {
	if source.SessionStartedAt.IsZero() {
		return nil
	}
	value := source.SessionStartedAt
	return &value
}

func importedCodexSourceUpdatedAt(source tables.AISessionTable) *time.Time {
	if !source.FileModTime.IsZero() {
		value := source.FileModTime
		return &value
	}
	if source.LastMessageAt != nil {
		value := *source.LastMessageAt
		return &value
	}
	return nil
}

func importedCodexMetadataUpdates(source tables.AISessionTable) map[string]any {
	updates := map[string]any{
		"cwd":               filepath.Clean(strings.TrimSpace(source.ProjectPath)),
		"native_session_id": nilIfEmpty(source.SessionID),
		"source_kind":       defaultSourceKind(AgentCodex),
		"source_created_at": importedCodexSourceCreatedAt(source),
		"source_updated_at": importedCodexSourceUpdatedAt(source),
		"last_message_at":   source.LastMessageAt,
		"thread_path":       nilIfEmpty(source.FilePath),
		"updated_at":        time.Now(),
	}
	if title := strings.TrimSpace(source.Title); title != "" {
		updates["thread_preview"] = &title
	}
	return updates
}

func (m *Manager) findImportedCodexSession(
	ctx context.Context,
	projectID string,
	nativeSessionID string,
) (tables.WebSessionTable, error) {
	db := model.GetDB()
	if db == nil {
		return tables.WebSessionTable{}, model.ErrDBNotInitialized
	}

	var records []tables.WebSessionTable
	if err := db.WithContext(ctx).
		Where(
			"project_id = ? AND agent = ? AND native_session_id = ?",
			strings.TrimSpace(projectID),
			string(AgentCodex),
			strings.TrimSpace(nativeSessionID),
		).
		Order("updated_at DESC").
		Find(&records).Error; err != nil {
		return tables.WebSessionTable{}, err
	}
	if len(records) == 0 {
		return tables.WebSessionTable{}, gorm.ErrRecordNotFound
	}
	for _, record := range records {
		if record.ArchivedAt == nil {
			return record, nil
		}
	}
	return records[0], nil
}

func (m *Manager) createImportedCodexSession(
	ctx context.Context,
	project *model.Project,
	source tables.AISessionTable,
) (tables.WebSessionTable, error) {
	title := strings.TrimSpace(source.Title)
	titleAuto := title == ""
	if titleAuto {
		title = defaultTitle(AgentCodex, project.Name)
	}

	orderIndex, err := m.getNextSessionOrderIndex(ctx, project.Id)
	if err != nil {
		return tables.WebSessionTable{}, err
	}

	now := time.Now()
	modelName := m.resolveSessionModel(AgentCodex, source.Model)
	autoRetry := m.autoRetryDefaults()
	record := tables.WebSessionTable{
		ProjectID:                         project.Id,
		WorktreeID:                        nil,
		OrderIndex:                        orderIndex,
		Agent:                             string(AgentCodex),
		Backend:                           string(defaultSessionBackend(AgentCodex)),
		Title:                             title,
		TitleAuto:                         titleAuto,
		Model:                             modelName,
		ReasoningEffort:                   string(m.resolveSessionReasoningEffort(AgentCodex, modelName, "")),
		WorkflowMode:                      string(WorkflowModeDefault),
		PermissionLevel:                   string(m.resolveSessionPermissionLevel(AgentCodex, "")),
		AutoRetryEnabled:                  false,
		AutoRetryPolicyMode:               string(AutoRetryPolicyModeDefault),
		AutoRetryScope:                    string(autoRetry.scope),
		AutoRetryPreset:                   string(autoRetry.preset),
		AutoRetryMaxAttempts:              autoRetry.maxAttempts,
		AutoRetryDispatchPendingOnFailure: autoRetry.dispatchPendingOnFailure,
		LegacyPermissionMode:              "default",
		Cwd:                               filepath.Clean(strings.TrimSpace(source.ProjectPath)),
		NativeSessionID:                   nilIfEmpty(source.SessionID),
		Status:                            string(StatusIdle),
		AssistantState:                    "",
		HasUnread:                         false,
		ArchivedAt:                        nil,
		ActivityAt:                        now,
		StatusUpdatedAt:                   &now,
		AssistantStateUpdatedAt:           nil,
		SourceKind:                        defaultSourceKind(AgentCodex),
		SyncState:                         string(SyncStateMissing),
		LastSyncMode:                      "",
		SourceCreatedAt:                   importedCodexSourceCreatedAt(source),
		SourceUpdatedAt:                   importedCodexSourceUpdatedAt(source),
		LastSyncedAt:                      nil,
		ThreadPath:                        nilIfEmpty(source.FilePath),
		ThreadPreview:                     nilIfEmpty(source.Title),
		TurnCount:                         0,
		ItemCount:                         0,
		LastMessageAt:                     source.LastMessageAt,
		LastEventSeq:                      0,
		TotalInputTokens:                  0,
		TotalCachedInputTokens:            0,
		TotalOutputTokens:                 0,
		TotalCost:                         0,
	}
	record.Init()

	if err := model.GetDB().WithContext(ctx).Create(&record).Error; err != nil {
		return tables.WebSessionTable{}, err
	}
	return record, nil
}

func preferImportedCodexSession(current, candidate tables.WebSessionTable) tables.WebSessionTable {
	if strings.TrimSpace(current.ID) == "" {
		return candidate
	}

	currentArchived := current.ArchivedAt != nil
	candidateArchived := candidate.ArchivedAt != nil
	if currentArchived != candidateArchived {
		if !candidateArchived {
			return candidate
		}
		return current
	}

	if candidate.ActivityAt.After(current.ActivityAt) {
		return candidate
	}
	if current.ActivityAt.After(candidate.ActivityAt) {
		return current
	}

	if candidate.UpdatedAt.After(current.UpdatedAt) {
		return candidate
	}
	return current
}

func (m *Manager) existingImportedSessionsByNativeID(
	ctx context.Context,
	projectID string,
	agent Agent,
	sessionIDs []string,
) (map[string]tables.WebSessionTable, error) {
	normalized := make([]string, 0, len(sessionIDs))
	seen := make(map[string]struct{}, len(sessionIDs))
	for _, sessionID := range sessionIDs {
		trimmed := strings.TrimSpace(sessionID)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}

	result := make(map[string]tables.WebSessionTable, len(normalized))
	if len(normalized) == 0 {
		return result, nil
	}

	var existing []tables.WebSessionTable
	if err := model.GetDB().WithContext(ctx).
		Where(
			"project_id = ? AND agent = ? AND native_session_id IN ?",
			projectID,
			string(agent),
			normalized,
		).
		Find(&existing).Error; err != nil {
		return nil, err
	}

	for _, record := range existing {
		nativeID := ""
		if record.NativeSessionID != nil {
			nativeID = strings.TrimSpace(*record.NativeSessionID)
		}
		if nativeID == "" {
			continue
		}
		result[nativeID] = preferImportedCodexSession(result[nativeID], record)
	}
	return result, nil
}

func (m *Manager) existingImportedCodexSessionsByNativeID(
	ctx context.Context,
	projectID string,
	sessionIDs []string,
) (map[string]tables.WebSessionTable, error) {
	return m.existingImportedSessionsByNativeID(ctx, projectID, AgentCodex, sessionIDs)
}

func sortImportSourceItems(items []ImportSourceSummary) {
	sort.Slice(items, func(i, j int) bool {
		left := items[i]
		right := items[j]

		leftTime := left.SessionStartedAt
		if left.LastMessageAt != nil {
			leftTime = *left.LastMessageAt
		}
		rightTime := right.SessionStartedAt
		if right.LastMessageAt != nil {
			rightTime = *right.LastMessageAt
		}

		if leftTime.After(rightTime) {
			return true
		}
		if rightTime.After(leftTime) {
			return false
		}
		return left.SessionID > right.SessionID
	})
}

func (m *Manager) buildImportSourceItemFromThread(
	thread codexThreadSummary,
	cached *tables.AISessionTable,
	existingByNativeID map[string]tables.WebSessionTable,
) ImportSourceSummary {
	title := strings.TrimSpace(thread.Preview)
	model := ""
	filePath := strings.TrimSpace(thread.Path)
	sessionStartedAt := time.Now()
	if thread.CreatedAt != nil {
		sessionStartedAt = *thread.CreatedAt
	} else if thread.UpdatedAt != nil {
		sessionStartedAt = *thread.UpdatedAt
	}
	lastMessageAt := thread.UpdatedAt
	messageCount := 0
	assistantMessageCount := 0
	aiSessionID := ""

	if cached != nil {
		aiSessionID = strings.TrimSpace(cached.ID)
		if cachedTitle := strings.TrimSpace(cached.Title); cachedTitle != "" {
			title = cachedTitle
		}
		if cachedModel := strings.TrimSpace(cached.Model); cachedModel != "" {
			model = cachedModel
		}
		if cachedPath := strings.TrimSpace(cached.FilePath); cachedPath != "" {
			filePath = cachedPath
		}
		if !cached.SessionStartedAt.IsZero() {
			sessionStartedAt = cached.SessionStartedAt
		}
		if cached.LastMessageAt != nil {
			value := *cached.LastMessageAt
			lastMessageAt = &value
		}
		messageCount = cached.MessageCount
		assistantMessageCount = cached.AssistantMessageCount
	}

	item := ImportSourceSummary{
		Agent:                 AgentCodex,
		Importable:            true,
		AISessionID:           aiSessionID,
		SessionID:             strings.TrimSpace(thread.ID),
		Model:                 model,
		Title:                 title,
		SessionStartedAt:      sessionStartedAt,
		LastMessageAt:         lastMessageAt,
		MessageCount:          messageCount,
		AssistantMessageCount: assistantMessageCount,
		FilePath:              filePath,
	}
	if existing, ok := existingByNativeID[item.SessionID]; ok {
		summary := m.mapSessionSummary(existing)
		item.Duplicate = true
		item.ExistingSession = &summary
	}
	return item
}

func (m *Manager) buildImportSourceItemFromAISession(
	source *service.AISessionSummary,
	agent Agent,
	importable bool,
	existingByNativeID map[string]tables.WebSessionTable,
) ImportSourceSummary {
	item := ImportSourceSummary{
		Agent:                 agent,
		Importable:            importable,
		AISessionID:           strings.TrimSpace(source.ID),
		SessionID:             strings.TrimSpace(source.SessionID),
		Model:                 strings.TrimSpace(source.Model),
		Title:                 strings.TrimSpace(source.Title),
		SessionStartedAt:      source.SessionStartedAt,
		LastMessageAt:         source.LastMessageAt,
		MessageCount:          source.MessageCount,
		AssistantMessageCount: source.AssistantMessageCount,
		FilePath:              strings.TrimSpace(source.FilePath),
	}
	if existing, ok := existingByNativeID[item.SessionID]; ok {
		summary := m.mapSessionSummary(existing)
		item.Duplicate = true
		item.ExistingSession = &summary
	}
	return item
}

func (m *Manager) listCodexImportSourcesFromThreadList(
	ctx context.Context,
	project *model.Project,
) (ImportSourceList, error) {
	currentThreads, err := m.listCodexThreadsByCwd(ctx, project.Path, false)
	if err != nil {
		return ImportSourceList{}, err
	}
	archivedThreads, err := m.listCodexThreadsByCwd(ctx, project.Path, true)
	if err != nil {
		return ImportSourceList{}, err
	}

	merged := make(map[string]codexThreadSummary, len(currentThreads)+len(archivedThreads))
	for id, summary := range archivedThreads {
		merged[id] = summary
	}
	for id, summary := range currentThreads {
		merged[id] = summary
	}

	sessionIDs := make([]string, 0, len(merged))
	for sessionID := range merged {
		sessionIDs = append(sessionIDs, sessionID)
	}
	existingByNativeID, err := m.existingImportedCodexSessionsByNativeID(ctx, project.Id, sessionIDs)
	if err != nil {
		return ImportSourceList{}, err
	}

	cachedBySessionID := make(map[string]*tables.AISessionTable, len(sessionIDs))
	if len(sessionIDs) > 0 {
		var cachedRows []tables.AISessionTable
		if err := model.GetDB().WithContext(ctx).
			Where(
				"project_path = ? AND type = ? AND session_id IN ?",
				project.Path,
				tables.AISessionTypeCodex,
				sessionIDs,
			).
			Find(&cachedRows).Error; err != nil {
			return ImportSourceList{}, err
		}
		for i := range cachedRows {
			row := cachedRows[i]
			cachedBySessionID[strings.TrimSpace(row.SessionID)] = &row
		}
	}

	items := make([]ImportSourceSummary, 0, len(merged))
	for sessionID, thread := range merged {
		if strings.TrimSpace(sessionID) == "" {
			continue
		}
		items = append(
			items,
			m.buildImportSourceItemFromThread(
				thread,
				cachedBySessionID[strings.TrimSpace(sessionID)],
				existingByNativeID,
			),
		)
	}
	sortImportSourceItems(items)
	return ImportSourceList{
		Items:     items,
		ScanPhase: "complete",
	}, nil
}

func (m *Manager) ListCodexImportSources(
	ctx context.Context,
	projectID string,
) (ImportSourceList, error) {
	project, err := m.projectSvc.GetProject(ctx, projectID)
	if err != nil {
		return ImportSourceList{}, err
	}
	if m.aiSessionSvc == nil {
		return ImportSourceList{}, fmt.Errorf("ai session service is not configured")
	}

	list, err := m.listCodexImportSourcesFromThreadList(ctx, project)
	if err == nil {
		return list, nil
	}

	aiSessions, fallbackErr := m.aiSessionSvc.GetProjectAISessions(ctx, project.Path)
	if fallbackErr != nil {
		return ImportSourceList{}, err
	}

	sessionIDs := make([]string, 0, len(aiSessions.CodexSessions))
	for _, source := range aiSessions.CodexSessions {
		if source == nil {
			continue
		}
		sessionID := strings.TrimSpace(source.SessionID)
		if sessionID == "" {
			continue
		}
		sessionIDs = append(sessionIDs, sessionID)
	}

	existingByNativeID, existingErr := m.existingImportedCodexSessionsByNativeID(ctx, project.Id, sessionIDs)
	if existingErr != nil {
		return ImportSourceList{}, existingErr
	}

	items := make([]ImportSourceSummary, 0, len(aiSessions.CodexSessions))
	for _, source := range aiSessions.CodexSessions {
		if source == nil {
			continue
		}
		items = append(items, m.buildImportSourceItemFromAISession(source, AgentCodex, true, existingByNativeID))
	}
	sortImportSourceItems(items)
	return ImportSourceList{
		Items:     items,
		ScanPhase: strings.TrimSpace(aiSessions.CodexScanPhase),
	}, nil
}

func (m *Manager) ListImportSources(
	ctx context.Context,
	projectID string,
) (ImportSourceList, error) {
	codexList, err := m.ListCodexImportSources(ctx, projectID)
	if err != nil {
		return ImportSourceList{}, err
	}
	if m.aiSessionSvc == nil {
		return codexList, nil
	}
	project, err := m.projectSvc.GetProject(ctx, projectID)
	if err != nil {
		return ImportSourceList{}, err
	}
	aiSessions, err := m.aiSessionSvc.GetProjectAISessions(ctx, project.Path)
	if err != nil {
		// Codex thread/list is independent of the filesystem index. Preserve the
		// existing result when optional Pi discovery cannot read its session root.
		return codexList, nil
	}

	piSessionIDs := make([]string, 0, len(aiSessions.PiSessions))
	for _, source := range aiSessions.PiSessions {
		if source != nil && strings.TrimSpace(source.SessionID) != "" {
			piSessionIDs = append(piSessionIDs, strings.TrimSpace(source.SessionID))
		}
	}
	existingPi, err := m.existingImportedSessionsByNativeID(ctx, project.Id, AgentPi, piSessionIDs)
	if err != nil {
		return ImportSourceList{}, err
	}
	items := append([]ImportSourceSummary(nil), codexList.Items...)
	piImportable := m.GetWebSessionRuntimeConfig().SupportsPiWebSession
	for _, source := range aiSessions.PiSessions {
		if source == nil {
			continue
		}
		items = append(items, m.buildImportSourceItemFromAISession(source, AgentPi, piImportable, existingPi))
	}
	sortImportSourceItems(items)

	return ImportSourceList{
		Items:        items,
		ScanPhase:    aggregateImportScanPhase(codexList.ScanPhase, aiSessions.PiScanPhase),
		BeforeCursor: strings.TrimSpace(aiSessions.PiBeforeCursor),
	}, nil
}

func aggregateImportScanPhase(phases ...string) string {
	result := "complete"
	for _, phase := range phases {
		switch strings.ToLower(strings.TrimSpace(phase)) {
		case "extended":
			return "extended"
		case "recent":
			result = "recent"
		}
	}
	return result
}

func (m *Manager) importCodexSessionResolved(
	ctx context.Context,
	project *model.Project,
	source *tables.AISessionTable,
	mode SyncMode,
) (ImportResult, error) {
	if source == nil {
		return ImportResult{}, gorm.ErrRecordNotFound
	}
	if strings.TrimSpace(source.SessionID) == "" {
		return ImportResult{}, fmt.Errorf("codex session id is empty")
	}
	if strings.TrimSpace(source.FilePath) == "" {
		return ImportResult{}, fmt.Errorf("codex session file path is empty")
	}
	if model.NormalizePathCase(source.ProjectPath) != model.NormalizePathCase(project.Path) {
		return ImportResult{}, fmt.Errorf("codex session does not belong to the current project")
	}

	record, err := m.findImportedCodexSession(ctx, project.Id, source.SessionID)
	if err == nil {
		if err := m.updateRuntimeState(ctx, record.ID, importedCodexMetadataUpdates(*source)); err != nil {
			return ImportResult{}, err
		}
		if record.ArchivedAt != nil {
			if _, err := m.UnarchiveSession(ctx, record.ID); err != nil {
				return ImportResult{}, err
			}
		}
		snapshot, err := m.Snapshot(ctx, record.ID, DefaultHistoryWindow)
		if err != nil {
			return ImportResult{}, err
		}
		return ImportResult{
			Session:         snapshot.Session,
			History:         snapshot.History,
			PendingInputs:   snapshot.PendingInputs,
			ScheduledInputs: snapshot.ScheduledInputs,
			SubAgents:       snapshot.SubAgents,
			Created:         false,
			Reused:          true,
			Synced:          false,
		}, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return ImportResult{}, err
	}

	record, err = m.createImportedCodexSession(ctx, project, *source)
	if err != nil {
		return ImportResult{}, err
	}

	snapshot, err := m.syncSessionFromSource(ctx, record.ID, mode, true, false)
	if err != nil {
		_ = m.DeleteSession(ctx, record.ID)
		return ImportResult{}, err
	}

	return ImportResult{
		Session:         snapshot.Session,
		History:         snapshot.History,
		PendingInputs:   snapshot.PendingInputs,
		ScheduledInputs: snapshot.ScheduledInputs,
		SubAgents:       snapshot.SubAgents,
		Created:         true,
		Reused:          false,
		Synced:          true,
	}, nil
}

func (m *Manager) ImportCodexSession(
	ctx context.Context,
	projectID string,
	aiSessionID string,
	mode SyncMode,
) (ImportResult, error) {
	project, err := m.projectSvc.GetProject(ctx, projectID)
	if err != nil {
		return ImportResult{}, err
	}
	if m.aiSessionSvc == nil {
		return ImportResult{}, fmt.Errorf("ai session service is not configured")
	}

	source, err := m.aiSessionSvc.ResolveCodexSessionByID(ctx, aiSessionID)
	if err != nil {
		return ImportResult{}, err
	}
	return m.importCodexSessionResolved(ctx, project, source, mode)
}

func (m *Manager) ImportCodexSessionBySessionID(
	ctx context.Context,
	projectID string,
	sessionID string,
	mode SyncMode,
) (ImportResult, error) {
	project, err := m.projectSvc.GetProject(ctx, projectID)
	if err != nil {
		return ImportResult{}, err
	}
	if m.aiSessionSvc == nil {
		return ImportResult{}, fmt.Errorf("ai session service is not configured")
	}

	source, err := m.aiSessionSvc.ResolveCodexSessionBySessionID(ctx, sessionID)
	if err != nil {
		return ImportResult{}, err
	}
	return m.importCodexSessionResolved(ctx, project, source, mode)
}

func (m *Manager) importPiSessionResolved(
	ctx context.Context,
	project *model.Project,
	source *tables.AISessionTable,
) (ImportResult, error) {
	if !m.GetWebSessionRuntimeConfig().SupportsPiWebSession {
		return ImportResult{}, errors.New(errPiWebSessionUnavailable)
	}
	if source == nil {
		return ImportResult{}, gorm.ErrRecordNotFound
	}
	nativeID := strings.TrimSpace(source.SessionID)
	threadPath := strings.TrimSpace(source.FilePath)
	if nativeID == "" || threadPath == "" {
		return ImportResult{}, errors.New("Pi session identity is incomplete")
	}
	if model.NormalizePathCase(source.ProjectPath) != model.NormalizePathCase(project.Path) {
		return ImportResult{}, errors.New("Pi session does not belong to the current project")
	}
	if err := m.EnsureProjectPiTrust(ctx, project.Id, project.Path); err != nil {
		return ImportResult{}, err
	}
	identity := tables.WebSessionTable{
		Cwd:             project.Path,
		NativeSessionID: &nativeID,
		ThreadPath:      &threadPath,
	}
	if err := validatePiRuntimeState(identity, piRPCState{SessionID: nativeID, SessionFile: threadPath}); err != nil {
		return ImportResult{}, err
	}

	var records []tables.WebSessionTable
	if err := model.GetDB().WithContext(ctx).
		Where("project_id = ? AND agent = ? AND native_session_id = ?", project.Id, string(AgentPi), nativeID).
		Order("updated_at DESC").
		Find(&records).Error; err != nil {
		return ImportResult{}, err
	}
	if len(records) > 0 {
		record := records[0]
		for _, candidate := range records[1:] {
			record = preferImportedCodexSession(record, candidate)
		}
		updates := map[string]any{
			"cwd":               filepath.Clean(project.Path),
			"thread_path":       filepath.Clean(threadPath),
			"native_session_id": nativeID,
			"source_kind":       defaultSourceKind(AgentPi),
			"source_created_at": importedCodexSourceCreatedAt(*source),
			"source_updated_at": importedCodexSourceUpdatedAt(*source),
			"last_message_at":   source.LastMessageAt,
			"source_revision":   piSourceRevision(threadPath, pointerString(record.NativeLeafID)),
			"updated_at":        time.Now(),
		}
		if err := m.updateRuntimeState(ctx, record.ID, updates); err != nil {
			return ImportResult{}, err
		}
		if record.ArchivedAt != nil {
			if _, err := m.UnarchiveSession(ctx, record.ID); err != nil {
				return ImportResult{}, err
			}
		}
		refreshed, err := m.GetSession(ctx, record.ID)
		if err != nil {
			return ImportResult{}, err
		}
		snapshot, err := m.syncImportedPiSession(ctx, refreshed)
		if err != nil {
			return ImportResult{}, err
		}
		return ImportResult{Session: snapshot.Session, History: snapshot.History,
			PendingInputs: snapshot.PendingInputs, ScheduledInputs: snapshot.ScheduledInputs,
			SubAgents: snapshot.SubAgents, Reused: true, Synced: true}, nil
	}

	orderIndex, err := m.getNextSessionOrderIndex(ctx, project.Id)
	if err != nil {
		return ImportResult{}, err
	}
	title := strings.TrimSpace(source.Title)
	titleAuto := title == ""
	if titleAuto {
		title = defaultTitle(AgentPi, project.Name)
	}
	modelName := strings.TrimSpace(source.Model)
	if _, _, err := splitPiModel(modelName); err != nil {
		modelName = ""
	}
	now := time.Now()
	record := tables.WebSessionTable{
		ProjectID: project.Id, OrderIndex: orderIndex, Agent: string(AgentPi),
		Backend: string(SessionBackendPiRPC), Title: title, TitleAuto: titleAuto,
		Model: modelName, ReasoningEffort: string(ReasoningEffortDefault),
		WorkflowMode: string(WorkflowModeDefault), PermissionLevel: string(PermissionLevelElevated),
		LegacyPermissionMode: "default", Cwd: filepath.Clean(project.Path), NativeSessionID: &nativeID,
		Status: string(StatusIdle), ActivityAt: now, StatusUpdatedAt: &now,
		SourceKind: defaultSourceKind(AgentPi), SyncState: string(SyncStateMissing),
		SourceCreatedAt: importedCodexSourceCreatedAt(*source), SourceUpdatedAt: importedCodexSourceUpdatedAt(*source),
		ThreadPath: &threadPath, ThreadPreview: nilIfEmpty(source.Title), LastMessageAt: source.LastMessageAt,
		SourceRevision: nilIfEmpty(piSourceRevision(threadPath, "")),
		AutoRetryScope: string(AutoRetryScopeNetworkOnly), AutoRetryPreset: string(AutoRetryPresetGentleStop),
	}
	record.Init()
	if err := model.GetDB().WithContext(ctx).Create(&record).Error; err != nil {
		return ImportResult{}, err
	}
	snapshot, err := m.syncImportedPiSession(ctx, record)
	if err != nil {
		_ = m.DeleteSession(ctx, record.ID)
		return ImportResult{}, err
	}
	return ImportResult{Session: snapshot.Session, History: snapshot.History,
		PendingInputs: snapshot.PendingInputs, ScheduledInputs: snapshot.ScheduledInputs,
		SubAgents: snapshot.SubAgents, Created: true, Synced: true}, nil
}

func (m *Manager) ImportPiSessionBySessionID(ctx context.Context, projectID, sessionID string) (ImportResult, error) {
	project, err := m.projectSvc.GetProject(ctx, projectID)
	if err != nil {
		return ImportResult{}, err
	}
	if m.aiSessionSvc == nil {
		return ImportResult{}, errors.New("ai session service is not configured")
	}
	source, err := m.aiSessionSvc.ResolvePiSessionBySessionID(ctx, sessionID)
	if err != nil {
		return ImportResult{}, err
	}
	return m.importPiSessionResolved(ctx, project, source)
}

func (m *Manager) ImportPiSession(ctx context.Context, projectID, aiSessionID string) (ImportResult, error) {
	project, err := m.projectSvc.GetProject(ctx, projectID)
	if err != nil {
		return ImportResult{}, err
	}
	if m.aiSessionSvc == nil {
		return ImportResult{}, errors.New("ai session service is not configured")
	}
	source, err := m.aiSessionSvc.ResolvePiSessionByID(ctx, aiSessionID)
	if err != nil {
		return ImportResult{}, err
	}
	return m.importPiSessionResolved(ctx, project, source)
}

func (m *Manager) GetSession(ctx context.Context, sessionID string) (tables.WebSessionTable, error) {
	db := model.GetReaderDB()
	if db == nil {
		return tables.WebSessionTable{}, model.ErrDBNotInitialized
	}
	var record tables.WebSessionTable
	if err := db.WithContext(ctx).First(&record, "id = ?", sessionID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return tables.WebSessionTable{}, gorm.ErrRecordNotFound
		}
		return tables.WebSessionTable{}, err
	}
	return record, nil
}

func (m *Manager) Snapshot(ctx context.Context, sessionID string, limit int) (SessionSnapshot, error) {
	record, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return SessionSnapshot{}, err
	}
	return m.loadSnapshotLocal(ctx, record, limit)
}

func (m *Manager) SnapshotWithAutoSync(ctx context.Context, sessionID string, limit int) (SessionSnapshot, error) {
	record, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return SessionSnapshot{}, err
	}
	snapshot, err := m.loadSnapshotLocal(ctx, record, limit)
	if err != nil {
		return SessionSnapshot{}, err
	}
	if !shouldAutoSyncSnapshot(record, snapshot.History) {
		return snapshot, nil
	}
	hadHistory := snapshot.History.Total > 0
	syncedSnapshot, syncErr := m.syncSessionFromSource(ctx, sessionID, m.defaultCodexSyncMode(), true, false)
	if syncErr != nil && hadHistory {
		if m.logger != nil {
			m.logger.Warn(
				"web session snapshot repair sync failed; preserving cached history",
				zap.String("sessionId", sessionID),
				zap.Error(syncErr),
			)
		}
		return snapshot, nil
	}
	if syncErr != nil {
		return SessionSnapshot{}, syncErr
	}
	if hadHistory && syncedSnapshot.History.Total == 0 {
		if m.logger != nil {
			m.logger.Warn(
				"web session snapshot repair sync returned empty history; preserving cached history",
				zap.String("sessionId", sessionID),
			)
		}
		return snapshot, nil
	}
	snapshot = syncedSnapshot
	if snapshot.History.Total == 0 {
		return SessionSnapshot{}, ErrSessionHistoryUnavailable
	}
	return snapshot, nil
}

func (m *Manager) SnapshotIfChanged(
	ctx context.Context,
	sessionID string,
	limit int,
	knownRevision string,
) (SessionSnapshotResponse, error) {
	known, err := parseSnapshotRevision(knownRevision)
	if err != nil {
		return SessionSnapshotResponse{}, err
	}
	record, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return SessionSnapshotResponse{}, err
	}
	current := normalizeSnapshotRevision(record.SnapshotRevision)
	if known > 0 && known == current {
		pendingEpoch, pendingVersion, pendingInputs := m.pendingStateSnapshot(record.ID)
		return SessionSnapshotResponse{
			Revision:       formatSnapshotRevision(current),
			HistoryEpoch:   strconv.FormatInt(normalizeHistoryEpoch(record.HistoryEpoch), 10),
			EventCursor:    formatEventCursor(record.LastEventSeq, maxEventCursorOrder),
			PendingEpoch:   pendingEpoch,
			PendingVersion: pendingVersion,
			PendingInputs:  pendingInputs,
			Unchanged:      true,
			CodexAppServer: m.codexAppServerRuntime(sessionID),
		}, nil
	}
	snapshot, err := m.Snapshot(ctx, sessionID, limit)
	if err != nil {
		return SessionSnapshotResponse{}, err
	}
	return NewSessionSnapshotResponse(snapshot), nil
}

func shouldAutoSyncSnapshot(record tables.WebSessionTable, history HistoryWindow) bool {
	if normalizeAgent(Agent(record.Agent)) != AgentCodex ||
		record.NativeSessionID == nil ||
		strings.TrimSpace(*record.NativeSessionID) == "" {
		return false
	}
	if history.Total == 0 {
		return true
	}
	return waitingPlanApprovalHistoryNeedsRepair(record, history.Items)
}

func waitingPlanApprovalHistoryNeedsRepair(record tables.WebSessionTable, items []HistoryItem) bool {
	if effectiveAssistantState(record) != AssistantStateWaitingPlanApproval {
		return false
	}
	rootThreadID := ""
	if record.NativeSessionID != nil {
		rootThreadID = strings.TrimSpace(*record.NativeSessionID)
	}

	var latestPlan *HistoryItem
	for index := len(items) - 1; index >= 0; index-- {
		item := &items[index]
		if !isPlanHistoryItem(*item) || item.Tool == nil || strings.TrimSpace(item.Tool.Output) == "" {
			continue
		}
		status := strings.ToLower(strings.TrimSpace(item.Tool.Status))
		if status != "done" && status != "completed" && status != "success" && status != "succeeded" {
			continue
		}
		if rootThreadID != "" && item.SourceThreadID != nil && strings.TrimSpace(*item.SourceThreadID) != rootThreadID {
			continue
		}
		latestPlan = item
		break
	}
	if latestPlan == nil {
		return true
	}
	if latestPlan.SourceThreadID == nil || latestPlan.SourceTurnID == nil {
		return false
	}
	planThreadID := strings.TrimSpace(*latestPlan.SourceThreadID)
	planTurnID := strings.TrimSpace(*latestPlan.SourceTurnID)
	if planThreadID == "" || planTurnID == "" {
		return false
	}
	for index := range items {
		item := items[index]
		if item.Kind != "user" || item.OrderIndex <= latestPlan.OrderIndex ||
			item.SourceThreadID == nil || item.SourceTurnID == nil {
			continue
		}
		if strings.TrimSpace(*item.SourceThreadID) == planThreadID &&
			strings.TrimSpace(*item.SourceTurnID) == planTurnID {
			return true
		}
	}
	return false
}

func (m *Manager) loadSnapshotLocal(
	ctx context.Context,
	record tables.WebSessionTable,
	limit int,
) (SessionSnapshot, error) {
	if limit <= 0 || limit > MaxHistoryWindow {
		limit = DefaultHistoryWindow
	}
	history, err := m.loadHistoryWindow(ctx, record.ID, limit, nil)
	if err != nil {
		return SessionSnapshot{}, err
	}
	scheduledInputs, err := m.scheduledInputsSnapshot(ctx, record.ID)
	if err != nil {
		return SessionSnapshot{}, err
	}
	summary := m.mapSessionSummary(record)
	summary.HasScheduledPlanExecution = scheduledInputsHavePendingPlanExecution(scheduledInputs)
	subAgents, err := m.sessionSubAgents(ctx, record.ID)
	if err != nil {
		return SessionSnapshot{}, err
	}
	pendingEpoch, pendingVersion, pendingInputs := m.pendingStateSnapshot(record.ID)
	return SessionSnapshot{
		Revision:         summary.Revision,
		HistoryEpoch:     summary.HistoryEpoch,
		EventCursor:      summary.EventCursor,
		PendingEpoch:     pendingEpoch,
		PendingVersion:   pendingVersion,
		Session:          summary,
		History:          history,
		PendingInputs:    pendingInputs,
		ScheduledInputs:  scheduledInputs,
		PendingApproval:  m.pendingApprovalSnapshot(record),
		PendingUserInput: pendingUserInputFromHistory(history.Items),
		SubAgents:        subAgents,
		CodexAppServer:   m.codexAppServerRuntime(record.ID),
	}, nil
}

func (m *Manager) pendingApprovalSnapshot(record tables.WebSessionTable) *PendingApproval {
	if AssistantState(record.AssistantState) != AssistantStateWaitingApproval {
		return nil
	}
	m.mu.RLock()
	run := m.runs[record.ID]
	m.mu.RUnlock()
	if run == nil {
		return nil
	}
	request, ok := run.pendingApprovalRequest()
	if !ok || request.Kind == pendingServerRequestPlanApproval {
		return nil
	}
	if request.PiRuntime == nil && run.codexAppServer() == nil {
		return nil
	}
	actionable := len(request.RawID) > 0
	if request.PiRuntime != nil {
		actionable = strings.TrimSpace(request.PiRequestID) != ""
	}
	return &PendingApproval{
		ItemID:      request.ItemID,
		Kind:        string(request.Kind),
		Prompt:      firstNonEmpty(request.Prompt, approvalPromptFallback(request.Kind)),
		Command:     request.Command,
		RequestedAt: request.RequestedAt,
		Actionable:  actionable,
	}
}

func (m *Manager) History(ctx context.Context, sessionID string, limit int, beforeSeq *int64) (HistoryWindow, error) {
	if _, err := m.GetSession(ctx, sessionID); err != nil {
		return HistoryWindow{}, err
	}
	if limit <= 0 || limit > MaxHistoryWindow {
		limit = DefaultHistoryWindow
	}
	return m.loadHistoryWindow(ctx, sessionID, limit, beforeSeq)
}

func (m *Manager) HistoryAfter(ctx context.Context, sessionID string, limit int, afterSeq int64) (HistoryWindow, error) {
	if _, err := m.GetSession(ctx, sessionID); err != nil {
		return HistoryWindow{}, err
	}
	if limit <= 0 || limit > MaxHistoryWindow {
		limit = DefaultHistoryWindow
	}
	return m.loadHistoryWindowAfter(ctx, sessionID, limit, afterSeq)
}

func (m *Manager) RenameSession(ctx context.Context, sessionID, title string) (SessionSummary, error) {
	normalized := strings.TrimSpace(title)
	if normalized == "" {
		return SessionSummary{}, fmt.Errorf("title is required")
	}
	if err := model.GetDB().WithContext(ctx).Model(&tables.WebSessionTable{}).
		Where("id = ?", sessionID).
		Updates(withSnapshotRevisionIncrement(map[string]any{
			"title":      normalized,
			"title_auto": false,
			"updated_at": time.Now(),
		})).Error; err != nil {
		return SessionSummary{}, err
	}
	record, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return SessionSummary{}, err
	}
	return m.mapSessionSummary(record), nil
}

func (m *Manager) UpdateModel(ctx context.Context, sessionID, modelName string) (SessionSummary, error) {
	normalized := strings.TrimSpace(modelName)
	record, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return SessionSummary{}, err
	}
	if normalizeAgent(Agent(record.Agent)) == AgentPi && normalized != "" {
		if _, _, err := splitPiModel(normalized); err != nil {
			return SessionSummary{}, err
		}
	}
	updates := map[string]any{
		"model":      normalized,
		"updated_at": time.Now(),
	}
	if !sameCodexModel(record.Model, normalized) {
		updates["session_context_window_tokens"] = 0
		updates["session_context_window_observed_at"] = nil
	}
	return m.updateFields(ctx, sessionID, updates)
}

func (m *Manager) UpdateClaudeRuntime(
	ctx context.Context,
	sessionID string,
	runtime ClaudeRuntime,
) (SessionSummary, error) {
	record, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return SessionSummary{}, err
	}
	if normalizeAgent(Agent(record.Agent)) != AgentClaude {
		return SessionSummary{}, fmt.Errorf("claude runtime is only supported for claude sessions")
	}
	return m.updateFields(ctx, sessionID, map[string]any{
		"claude_runtime": string(normalizeClaudeRuntime(runtime)),
		"updated_at":     time.Now(),
	})
}

func (m *Manager) UpdateReasoningEffort(
	ctx context.Context,
	sessionID string,
	effort ReasoningEffort,
) (SessionSummary, error) {
	record, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return SessionSummary{}, err
	}
	normalized := normalizeReasoningEffort(effort)
	if normalizeAgent(Agent(record.Agent)) == AgentPi {
		if err := validatePiReasoningEffort(normalized); err != nil {
			return SessionSummary{}, err
		}
	}
	return m.updateFields(ctx, sessionID, map[string]any{
		"reasoning_effort": string(normalized),
		"updated_at":       time.Now(),
	})
}

func (m *Manager) UpdateWorkflowMode(
	ctx context.Context,
	sessionID string,
	mode WorkflowMode,
) (SessionSummary, error) {
	return m.updateFields(ctx, sessionID, map[string]any{
		"workflow_mode": string(normalizeWorkflowMode(mode)),
		"updated_at":    time.Now(),
	})
}

func (m *Manager) GetSessionGoal(ctx context.Context, sessionID string) (*SessionGoal, error) {
	record, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if normalizeAgent(Agent(record.Agent)) != AgentCodex {
		return nil, fmt.Errorf("goal is only supported for codex sessions")
	}
	if record.NativeSessionID == nil || strings.TrimSpace(*record.NativeSessionID) == "" {
		return sessionGoalFromRecord(record), nil
	}
	if err := m.ensureSessionGoalModeSupported(record); err != nil {
		return nil, err
	}
	if err := m.syncCodexGoalState(ctx, record, nil, strings.TrimSpace(*record.NativeSessionID)); err != nil {
		return nil, err
	}
	refreshed, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	return sessionGoalFromRecord(refreshed), nil
}

func (m *Manager) SetSessionGoal(
	ctx context.Context,
	sessionID string,
	objective string,
	status GoalStatus,
) (SessionSummary, error) {
	record, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return SessionSummary{}, err
	}
	if normalizeAgent(Agent(record.Agent)) != AgentCodex {
		return SessionSummary{}, fmt.Errorf("goal is only supported for codex sessions")
	}
	if record.NativeSessionID == nil || strings.TrimSpace(*record.NativeSessionID) == "" {
		return SessionSummary{}, fmt.Errorf("session has no native thread id")
	}
	if err := m.ensureSessionGoalModeSupported(record); err != nil {
		return SessionSummary{}, err
	}

	trimmedObjective := strings.TrimSpace(objective)
	normalizedStatus := status
	if normalizedStatus == "" {
		normalizedStatus = GoalStatusActive
	}
	if normalizeGoalStatus(string(normalizedStatus)) == "" {
		return SessionSummary{}, fmt.Errorf("invalid goal status")
	}
	if trimmedObjective == "" {
		return SessionSummary{}, fmt.Errorf("goal objective is required")
	}
	if len([]rune(trimmedObjective)) > 4000 {
		return SessionSummary{}, fmt.Errorf("goal objective must be at most 4000 characters")
	}

	err = m.withCodexQueryClient(ctx, record.Cwd, func(client *codexAppServerClient) error {
		_, err := client.request(ctx, "thread/goal/set", map[string]any{
			"threadId":  strings.TrimSpace(*record.NativeSessionID),
			"objective": trimmedObjective,
			"status":    string(normalizedStatus),
		})
		if err != nil {
			return err
		}
		return m.syncCodexGoalState(ctx, record, client, strings.TrimSpace(*record.NativeSessionID))
	})
	if err != nil {
		return SessionSummary{}, err
	}
	refreshed, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return SessionSummary{}, err
	}
	return m.mapSessionSummary(refreshed), nil
}

func (m *Manager) BootstrapSessionGoal(
	ctx context.Context,
	sessionID string,
	objective string,
	status GoalStatus,
) error {
	dispatchLock := &m.sessionDispatchLocks[sessionRevisionLockIndex(sessionID)]
	dispatchLock.Lock()
	defer dispatchLock.Unlock()

	record, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if record.ArchivedAt != nil {
		return fmt.Errorf("session is archived")
	}
	if normalizeAgent(Agent(record.Agent)) != AgentCodex {
		return fmt.Errorf("goal is only supported for codex sessions")
	}
	if effectiveSessionBackend(record) != SessionBackendCodexAppServer {
		return fmt.Errorf("goal bootstrap is only supported for codex app-server sessions")
	}
	if err := m.ensureSessionMessagingAvailable(record); err != nil {
		return err
	}
	if err := m.ensureSessionGoalModeSupported(record); err != nil {
		return err
	}
	if m.hasActiveRun(sessionID) {
		return fmt.Errorf("session is already running")
	}

	trimmedObjective := strings.TrimSpace(objective)
	normalizedStatus := status
	if normalizedStatus == "" {
		normalizedStatus = GoalStatusActive
	}
	if normalizeGoalStatus(string(normalizedStatus)) == "" {
		return fmt.Errorf("invalid goal status")
	}
	if trimmedObjective == "" {
		return fmt.Errorf("goal objective is required")
	}
	if len([]rune(trimmedObjective)) > 4000 {
		return fmt.Errorf("goal objective must be at most 4000 characters")
	}

	return m.startHiddenSessionRun(
		ctx,
		record,
		buildGoalBootstrapPrompt(trimmedObjective),
		nil,
		trimmedObjective,
		normalizedStatus,
	)
}

func (m *Manager) UpdateSessionGoalStatus(
	ctx context.Context,
	sessionID string,
	status GoalStatus,
) (SessionSummary, error) {
	record, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return SessionSummary{}, err
	}
	if normalizeAgent(Agent(record.Agent)) != AgentCodex {
		return SessionSummary{}, fmt.Errorf("goal is only supported for codex sessions")
	}
	if record.NativeSessionID == nil || strings.TrimSpace(*record.NativeSessionID) == "" {
		return SessionSummary{}, fmt.Errorf("session has no native thread id")
	}
	if err := m.ensureSessionGoalModeSupported(record); err != nil {
		return SessionSummary{}, err
	}
	normalizedStatus := normalizeGoalStatus(string(status))
	if normalizedStatus == "" {
		return SessionSummary{}, fmt.Errorf("invalid goal status")
	}
	currentGoal := sessionGoalFromRecord(record)
	if currentGoal == nil {
		return SessionSummary{}, fmt.Errorf("session has no goal")
	}

	err = m.withCodexQueryClient(ctx, record.Cwd, func(client *codexAppServerClient) error {
		_, err := client.request(ctx, "thread/goal/set", map[string]any{
			"threadId": strings.TrimSpace(*record.NativeSessionID),
			"status":   string(normalizedStatus),
		})
		if err != nil {
			return err
		}
		return m.syncCodexGoalState(ctx, record, client, strings.TrimSpace(*record.NativeSessionID))
	})
	if err != nil {
		return SessionSummary{}, err
	}
	refreshed, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return SessionSummary{}, err
	}
	return m.mapSessionSummary(refreshed), nil
}

func (m *Manager) ClearSessionGoal(ctx context.Context, sessionID string) (SessionSummary, error) {
	record, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return SessionSummary{}, err
	}
	if normalizeAgent(Agent(record.Agent)) != AgentCodex {
		return SessionSummary{}, fmt.Errorf("goal is only supported for codex sessions")
	}
	if record.NativeSessionID == nil || strings.TrimSpace(*record.NativeSessionID) == "" {
		return SessionSummary{}, fmt.Errorf("session has no native thread id")
	}
	if err := m.ensureSessionGoalModeSupported(record); err != nil {
		return SessionSummary{}, err
	}

	err = m.withCodexQueryClient(ctx, record.Cwd, func(client *codexAppServerClient) error {
		_, err := client.request(ctx, "thread/goal/clear", map[string]any{
			"threadId": strings.TrimSpace(*record.NativeSessionID),
		})
		if err != nil {
			return err
		}
		updates := map[string]any{
			"updated_at": time.Now(),
		}
		applySessionGoalUpdates(updates, nil)
		return m.updateRuntimeState(ctx, record.ID, updates)
	})
	if err != nil {
		return SessionSummary{}, err
	}
	refreshed, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return SessionSummary{}, err
	}
	return m.mapSessionSummary(refreshed), nil
}

func (m *Manager) UpdatePermissionLevel(
	ctx context.Context,
	sessionID string,
	level PermissionLevel,
) (SessionSummary, error) {
	record, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return SessionSummary{}, err
	}
	if err := validateWebSessionPermissionLevel(Agent(record.Agent), level); err != nil {
		return SessionSummary{}, err
	}
	return m.updateFields(ctx, sessionID, map[string]any{
		"permission_level": string(normalizePermissionLevel(level)),
		"updated_at":       time.Now(),
	})
}

func (m *Manager) UpdateActiveCallTimeout(
	ctx context.Context,
	sessionID string,
	enabled bool,
) (SessionSummary, error) {
	summary, err := m.updateFields(ctx, sessionID, map[string]any{
		"active_call_timeout_enabled": enabled,
		"updated_at":                  time.Now(),
	})
	if err != nil {
		return SessionSummary{}, err
	}
	m.reconcileActiveCallTimeoutBySessionID(sessionID)
	return summary, nil
}

func (m *Manager) UpdateAutoRetry(
	ctx context.Context,
	sessionID string,
	enabled bool,
	policyMode *AutoRetryPolicyMode,
	scope *AutoRetryScope,
	preset *AutoRetryPreset,
	maxAttempts *int,
) (SessionSummary, error) {
	record, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return SessionSummary{}, err
	}
	resolved := m.resolveAutoRetryUpdateConfig(record, policyMode, scope, preset, maxAttempts)
	summary, err := m.updateFields(ctx, sessionID, map[string]any{
		"auto_retry_enabled":      enabled,
		"auto_retry_policy_mode":  string(resolved.policyMode),
		"auto_retry_scope":        string(resolved.scope),
		"auto_retry_preset":       string(resolved.preset),
		"auto_retry_max_attempts": resolved.maxAttempts,
		"auto_retry_attempt":      0,
		"auto_retry_next_at":      nil,
		"updated_at":              time.Now(),
	})
	if err != nil {
		return SessionSummary{}, err
	}
	m.cancelAutoRetryTimer(sessionID)
	if err := m.reconcileAutoRetry(ctx, sessionID, time.Now()); err != nil {
		return SessionSummary{}, err
	}
	m.triggerPendingProcessing(sessionID)
	return summary, nil
}

func (m *Manager) UpdateAutoRetryDispatchPendingOnFailure(
	ctx context.Context,
	sessionID string,
	enabled bool,
) (SessionSummary, error) {
	summary, err := m.updateFields(ctx, sessionID, map[string]any{
		"auto_retry_dispatch_pending_on_failure": enabled,
		"updated_at":                             time.Now(),
	})
	if err != nil {
		return SessionSummary{}, err
	}
	m.triggerPendingProcessing(sessionID)
	return summary, nil
}

func (m *Manager) UpdateAgent(ctx context.Context, sessionID string, agent Agent) (SessionSummary, error) {
	normalized, err := validateAgent(agent)
	if err != nil {
		return SessionSummary{}, err
	}
	permissionLevel := m.resolveSessionPermissionLevel(normalized, "")
	modelName := m.resolveSessionModel(normalized, "")
	return m.updateFields(ctx, sessionID, map[string]any{
		"agent":                              string(normalized),
		"claude_runtime":                     string(defaultClaudeRuntime(normalized)),
		"backend":                            string(defaultSessionBackend(normalized)),
		"model":                              modelName,
		"reasoning_effort":                   string(m.resolveSessionReasoningEffort(normalized, modelName, "")),
		"permission_level":                   string(permissionLevel),
		"native_session_id":                  nil,
		"source_kind":                        defaultSourceKind(normalized),
		"sync_state":                         SyncStateMissing,
		"last_sync_mode":                     "",
		"source_created_at":                  nil,
		"source_updated_at":                  nil,
		"last_synced_at":                     nil,
		"thread_path":                        nil,
		"thread_preview":                     nil,
		"turn_count":                         0,
		"item_count":                         0,
		"sync_error":                         nil,
		"session_context_window_tokens":      0,
		"session_context_window_observed_at": nil,
		"updated_at":                         time.Now(),
	})
}

func (m *Manager) MoveSession(ctx context.Context, sessionID, prevSessionID, nextSessionID string) (SessionSummary, error) {
	db := model.GetDB()
	if db == nil {
		return SessionSummary{}, model.ErrDBNotInitialized
	}

	var summary SessionSummary
	err := m.observedTransaction(ctx, db, "move_session", func(tx *gorm.DB) error {
		var moving tables.WebSessionTable
		if err := tx.First(&moving, "id = ?", sessionID).Error; err != nil {
			return err
		}
		if moving.ArchivedAt != nil {
			return fmt.Errorf("archived sessions cannot be reordered")
		}

		ordered, err := m.listSessionRecordsWithDB(tx, moving.ProjectID)
		if err != nil {
			return err
		}
		if len(ordered) == 0 {
			return gorm.ErrRecordNotFound
		}

		filtered := make([]tables.WebSessionTable, 0, len(ordered)-1)
		for _, item := range ordered {
			if item.ID == moving.ID {
				continue
			}
			filtered = append(filtered, item)
		}

		insertIndex, err := resolveSessionInsertIndex(filtered, moving.ID, prevSessionID, nextSessionID)
		if err != nil {
			return err
		}

		reordered := make([]tables.WebSessionTable, 0, len(ordered))
		reordered = append(reordered, filtered[:insertIndex]...)
		reordered = append(reordered, moving)
		reordered = append(reordered, filtered[insertIndex:]...)

		for index, item := range reordered {
			nextOrderIndex := float64(index+1) * sessionOrderStep
			if item.OrderIndex == nextOrderIndex {
				continue
			}
			if err := tx.Model(&tables.WebSessionTable{}).
				Where("id = ?", item.ID).
				UpdateColumns(withSnapshotRevisionIncrement(map[string]any{
					"order_index": nextOrderIndex,
				})).Error; err != nil {
				return err
			}
			if item.ID == moving.ID {
				moving.OrderIndex = nextOrderIndex
				moving.SnapshotRevision = normalizeSnapshotRevision(moving.SnapshotRevision + 1)
			}
		}

		summary = m.mapSessionSummary(moving)
		return nil
	},
		zap.String("sessionId", sessionID),
		zap.String("previousSessionId", prevSessionID),
		zap.String("nextSessionId", nextSessionID),
	)
	if err != nil {
		return SessionSummary{}, err
	}
	return summary, nil
}

func (m *Manager) ArchiveSession(ctx context.Context, sessionID string) (SessionSummary, error) {
	record, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return SessionSummary{}, err
	}
	if record.ArchivedAt != nil {
		return m.mapSessionSummary(record), nil
	}

	hadActiveRun := m.hasActiveRun(sessionID)
	if err := m.stopRunIfActive(sessionID, 5*time.Second); err != nil {
		return SessionSummary{}, err
	}
	m.StopSessionPiRuntime(sessionID)

	now := time.Now()
	updates := map[string]any{
		"archived_at":                now,
		"has_unread":                 false,
		"updated_at":                 now,
		"auto_retry_attempt":         0,
		"auto_retry_next_at":         nil,
		"auto_retry_last_error_code": nil,
	}

	current, currentErr := m.GetSession(ctx, sessionID)
	if currentErr == nil {
		record = current
	}
	if hadActiveRun || record.Status == string(StatusAborting) {
		updates["status"] = string(StatusIdle)
		updates = applyAssistantStateUpdates(updates, AssistantStateNone, now)
	}

	if err := m.updateRuntimeState(ctx, sessionID, updates); err != nil {
		return SessionSummary{}, err
	}
	m.cancelAutoRetryTimer(sessionID)
	m.clearPendingInputs(sessionID)
	if err := m.cancelActiveScheduledInputs(ctx, sessionID); err != nil {
		return SessionSummary{}, err
	}
	archived, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return SessionSummary{}, err
	}
	return m.mapSessionSummary(archived), nil
}

func (m *Manager) UnarchiveSession(ctx context.Context, sessionID string) (SessionSummary, error) {
	record, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return SessionSummary{}, err
	}
	if record.ArchivedAt == nil {
		return m.mapSessionSummary(record), nil
	}

	orderIndex, err := m.getNextSessionOrderIndex(ctx, record.ProjectID)
	if err != nil {
		return SessionSummary{}, err
	}

	now := time.Now()
	if err := m.updateRuntimeState(ctx, sessionID, map[string]any{
		"archived_at": nil,
		"order_index": orderIndex,
		"updated_at":  now,
	}); err != nil {
		return SessionSummary{}, err
	}

	current, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return SessionSummary{}, err
	}
	return m.mapSessionSummary(current), nil
}

func (m *Manager) DeleteSession(ctx context.Context, sessionID string) error {
	dispatchLock := &m.sessionDispatchLocks[sessionRevisionLockIndex(sessionID)]
	dispatchLock.Lock()
	defer dispatchLock.Unlock()

	record, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}
	sessionID = record.ID
	m.cancelAutoRetryTimer(sessionID)
	m.clearPendingInputs(sessionID)
	if err := m.stopRunIfActive(sessionID, 5*time.Second); err != nil {
		return err
	}
	m.StopSessionPiRuntime(sessionID)

	eventState := m.sessionEventState(sessionID)
	eventState.mu.Lock()
	if err := m.flushPendingTextDeltaLocked(ctx, sessionID, eventState); err != nil {
		eventState.mu.Unlock()
		return err
	}
	eventState.closed = true
	defer func() {
		eventState.mu.Unlock()
	}()

	if err := m.deleteScheduledInputsForSession(ctx, sessionID); err != nil {
		eventState.closed = false
		return err
	}
	db := model.GetDB()
	if db == nil {
		eventState.closed = false
		return model.ErrDBNotInitialized
	}
	if err := db.WithContext(ctx).Unscoped().Where("web_session_id = ?", sessionID).Delete(&tables.WebSessionTurnTable{}).Error; err != nil {
		eventState.closed = false
		return err
	}
	if err := db.WithContext(ctx).Unscoped().Where("web_session_id = ?", sessionID).Delete(&tables.WebSessionItemTable{}).Error; err != nil {
		eventState.closed = false
		return err
	}
	if err := db.WithContext(ctx).Unscoped().Where("web_session_id = ?", sessionID).Delete(&tables.WebSessionSubAgentTable{}).Error; err != nil {
		eventState.closed = false
		return err
	}
	if err := db.WithContext(ctx).Unscoped().Where("web_session_id = ?", sessionID).Delete(&tables.WebSessionRunTimingTable{}).Error; err != nil {
		eventState.closed = false
		return err
	}
	if err := model.GetDB().WithContext(ctx).Delete(&tables.WebSessionTable{}, "id = ?", sessionID).Error; err != nil {
		eventState.closed = false
		return err
	}
	if eventState.timer != nil {
		eventState.timer.Stop()
		eventState.timer = nil
	}
	eventState.timerGeneration++
	if eventState.projectionTimer != nil {
		eventState.projectionTimer.Stop()
		eventState.projectionTimer = nil
	}
	eventState.projectionTimerGeneration++
	eventState.pending = nil
	eventState.projectionRetries = nil
	m.removeSessionEventState(sessionID, eventState)
	if err := m.store.deleteSessionFiles(sessionID); err != nil {
		return err
	}
	return nil
}

func (m *Manager) cancelAutoRetryTimer(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	timer := m.autoRetryTimers[sessionID]
	if timer != nil {
		timer.Stop()
		delete(m.autoRetryTimers, sessionID)
	}
}

func (m *Manager) setAutoRetryTimer(sessionID string, nextAt time.Time) {
	m.cancelAutoRetryTimer(sessionID)
	delay := time.Until(nextAt)
	if delay < 0 {
		delay = 0
	}
	timer := time.AfterFunc(delay, func() {
		m.cancelAutoRetryTimer(sessionID)
		m.executeAutoRetry(sessionID)
	})
	m.mu.Lock()
	m.autoRetryTimers[sessionID] = timer
	m.mu.Unlock()
}

func (m *Manager) resetAutoRetryProgress(ctx context.Context, sessionID string) {
	m.cancelAutoRetryTimer(sessionID)
	now := time.Now()
	_ = m.settleRetryWaitAndUpdateSession(ctx, sessionID, now, map[string]any{
		"auto_retry_attempt": 0,
		"auto_retry_next_at": nil,
		"updated_at":         now,
	})
}

func (m *Manager) clearAutoRetryNextAt(ctx context.Context, sessionID string) {
	m.cancelAutoRetryTimer(sessionID)
	now := time.Now()
	_ = m.settleRetryWaitAndUpdateSession(ctx, sessionID, now, map[string]any{
		"auto_retry_next_at": nil,
		"updated_at":         now,
	})
}

func autoRetryFailureDetails(record tables.WebSessionTable) (string, string) {
	code := ""
	if record.AutoRetryLastErrorCode != nil {
		code = strings.TrimSpace(*record.AutoRetryLastErrorCode)
	}
	message := ""
	if record.LastError != nil {
		message = strings.TrimSpace(*record.LastError)
	}
	return code, message
}

func autoRetryDefersPending(record tables.WebSessionTable) bool {
	if !record.AutoRetryEnabled || effectiveStatus(record, effectiveAssistantState(record)) != StatusError {
		return false
	}
	if record.AutoRetryNextAt != nil {
		return true
	}
	return !record.AutoRetryDispatchPendingOnFailure
}

func (m *Manager) reconcileAutoRetry(ctx context.Context, sessionID string, now time.Time) error {
	record, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if effectiveStatus(record, effectiveAssistantState(record)) != StatusError {
		return nil
	}
	code, message := autoRetryFailureDetails(record)
	m.scheduleAutoRetry(record, code, message, now)
	return nil
}

func (m *Manager) scheduleAutoRetry(record tables.WebSessionTable, code string, message string, now time.Time) {
	if record.ArchivedAt != nil {
		m.resetAutoRetryProgress(context.Background(), record.ID)
		return
	}
	if !record.AutoRetryEnabled {
		m.resetAutoRetryProgress(context.Background(), record.ID)
		return
	}
	if !shouldAutoRetryFailure(AutoRetryScope(record.AutoRetryScope), code, message) {
		m.resetAutoRetryProgress(context.Background(), record.ID)
		return
	}

	nextAttempt := record.AutoRetryAttempt + 1
	delay, ok := autoRetryDelayForFailureWithMax(
		AutoRetryPreset(record.AutoRetryPreset),
		nextAttempt,
		record.AutoRetryMaxAttempts,
		code,
		message,
	)
	if !ok {
		m.cancelAutoRetryTimer(record.ID)
		_ = m.settleRetryWaitAndUpdateSession(context.Background(), record.ID, now, map[string]any{
			"auto_retry_attempt": nextAttempt,
			"auto_retry_next_at": nil,
			"updated_at":         now,
		})
		return
	}

	nextAt := now.Add(delay)
	retryUpdates := map[string]any{
		"auto_retry_attempt":         nextAttempt,
		"auto_retry_next_at":         nextAt,
		"updated_at":                 now,
		"work_retry_wait_started_at": now,
	}
	if runID := m.latestCompletedWorkTimingRunID(context.Background(), record.ID); runID != "" {
		retryUpdates["work_retry_source_run_id"] = runID
	}
	_ = m.updateRuntimeState(context.Background(), record.ID, retryUpdates)
	m.setAutoRetryTimer(record.ID, nextAt)
}

func (m *Manager) executeAutoRetry(sessionID string) {
	ctx := context.Background()
	record, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return
	}
	if record.ArchivedAt != nil || !record.AutoRetryEnabled || effectiveStatus(record, effectiveAssistantState(record)) != StatusError {
		m.clearAutoRetryNextAt(ctx, sessionID)
		return
	}
	message := ""
	if record.LastError != nil {
		message = strings.TrimSpace(*record.LastError)
	}
	code := ""
	if record.AutoRetryLastErrorCode != nil {
		code = strings.TrimSpace(*record.AutoRetryLastErrorCode)
	}
	if !shouldAutoRetryFailure(AutoRetryScope(record.AutoRetryScope), code, message) {
		m.clearAutoRetryNextAt(ctx, sessionID)
		return
	}
	if err := m.sendMessageInternal(ctx, sessionID, "continue", nil, sendMessageOptions{fromAutoRetry: true}); err != nil {
		m.clearAutoRetryNextAt(ctx, sessionID)
		m.triggerPendingProcessing(sessionID)
		if m.logger != nil {
			m.logger.Warn("auto retry send failed", zap.String("sessionId", sessionID), zap.Error(err))
		}
	}
}

func (m *Manager) stopRunIfActive(sessionID string, timeout time.Duration) error {
	m.mu.RLock()
	run, ok := m.runs[sessionID]
	m.mu.RUnlock()
	if !ok || run == nil {
		return nil
	}
	if err := m.AbortSession(sessionID); err != nil {
		return err
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	select {
	case <-run.done:
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("timed out waiting for session to stop")
	}
}

func (m *Manager) stopRunForFreshContext(sessionID string, timeout time.Duration) error {
	m.mu.RLock()
	run := m.runs[sessionID]
	m.mu.RUnlock()
	if run == nil {
		return nil
	}
	if err := m.AbortSession(sessionID); err != nil {
		return err
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-run.done:
	case <-timer.C:
		return fmt.Errorf("timed out waiting for session to stop")
	}
	for {
		m.mu.RLock()
		current := m.runs[sessionID]
		m.mu.RUnlock()
		if current != run {
			return nil
		}
		select {
		case <-timer.C:
			return fmt.Errorf("timed out waiting for session to stop")
		case <-time.After(time.Millisecond):
		}
	}
}

func (m *Manager) AbortSession(sessionID string) error {
	m.mu.RLock()
	run, ok := m.runs[sessionID]
	m.mu.RUnlock()
	if !ok {
		return nil
	}
	if run.cancel != nil {
		run.cancel()
	}
	killCmdTree(run.command())
	return nil
}

func (m *Manager) AbortSessionForUser(sessionID string) error {
	normalizedSessionID := strings.TrimSpace(sessionID)
	now := time.Now()

	if err := m.AbortSession(normalizedSessionID); err != nil {
		return err
	}
	m.mu.Lock()
	run, ok := m.runs[normalizedSessionID]
	hasRedirect, redirectChanged := m.expireHeadPendingRedirectLocked(normalizedSessionID, now)
	m.mu.Unlock()

	if redirectChanged {
		m.broadcastPendingInputs(normalizedSessionID)
	}
	if hasRedirect && (!ok || run == nil) {
		m.triggerPendingProcessing(normalizedSessionID)
	}
	return nil
}

func (m *Manager) HandleCommand(ctx context.Context, client *client, payload []byte) error {
	var frame wireCommandFrame
	if err := json.Unmarshal(payload, &frame); err != nil {
		return client.send(newErrorFrame("", "", "bad_req", "invalid json payload", false))
	}
	if frame.Version != protocolVersion {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "unsupported_version", "unsupported protocol version", false))
	}
	if frame.Kind != wireKindCommand {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", "unsupported frame kind", false))
	}

	switch frame.Operation {
	case "create":
		return m.handleCreateCommand(ctx, client, frame)
	case "connect":
		return m.handleConnectCommand(ctx, client, frame)
	case "send":
		return m.handleSendCommand(ctx, client, frame)
	case "fresh_send":
		return m.handleFreshContextSendCommand(ctx, client, frame)
	case "compact":
		return m.handleCompactCommand(ctx, client, frame)
	case "tree_get":
		return m.handlePiTreeGetCommand(ctx, client, frame)
	case "tree_nav":
		return m.handlePiTreeNavigateCommand(ctx, client, frame)
	case "tree_fork":
		return m.handlePiTreeForkCommand(ctx, client, frame)
	case "tree_clone":
		return m.handlePiTreeCloneCommand(ctx, client, frame)
	case "hist":
		return m.handleHistoryCommand(ctx, client, frame)
	case "abort":
		return m.handleAbortCommand(ctx, client, frame)
	case "rename":
		return m.handleRenameCommand(ctx, client, frame)
	case "set_md":
		return m.handleSetModelCommand(ctx, client, frame)
	case "set_cr":
		return m.handleSetClaudeRuntimeCommand(ctx, client, frame)
	case "set_re":
		return m.handleSetReasoningEffortCommand(ctx, client, frame)
	case "set_wm":
		return m.handleSetWorkflowModeCommand(ctx, client, frame)
	case "goal_get":
		return m.handleGoalGetCommand(ctx, client, frame)
	case "mark_read":
		return m.handleMarkReadCommand(ctx, client, frame)
	case "goal_set":
		return m.handleGoalSetCommand(ctx, client, frame)
	case "goal_bootstrap":
		return m.handleGoalBootstrapCommand(ctx, client, frame)
	case "goal_pause":
		return m.handleGoalStatusCommand(ctx, client, frame, GoalStatusPaused)
	case "goal_resume":
		return m.handleGoalStatusCommand(ctx, client, frame, GoalStatusActive)
	case "goal_clear":
		return m.handleGoalClearCommand(ctx, client, frame)
	case "set_pl":
		return m.handleSetPermissionLevelCommand(ctx, client, frame)
	case "set_act":
		return m.handleSetActiveCallTimeoutCommand(ctx, client, frame)
	case "set_ar":
		return m.handleSetAutoRetryCommand(ctx, client, frame)
	case "set_ardpf":
		return m.handleSetAutoRetryDispatchPendingOnFailureCommand(ctx, client, frame)
	case "set_pm":
		return m.handleLegacySetModeCommand(ctx, client, frame)
	case "set_ag":
		return m.handleSetAgentCommand(ctx, client, frame)
	case "move":
		return m.handleMoveCommand(ctx, client, frame)
	case "approve":
		return m.handleApprovalCommand(client, frame, "approve")
	case "reject":
		return m.handleApprovalCommand(client, frame, "reject")
	case "user_input":
		return m.handleUserInputCommand(client, frame)
	case "pending_del":
		return m.handlePendingDeleteCommand(client, frame)
	case "pending_update":
		return m.handlePendingUpdateCommand(client, frame)
	case "pending_reorder":
		return m.handlePendingReorderCommand(client, frame)
	case "pending_clear":
		return m.handlePendingClearCommand(client, frame)
	case "schedule_send":
		return m.handleScheduleSendCommand(ctx, client, frame)
	case "schedule_plan":
		return m.handleSchedulePlanCommand(ctx, client, frame)
	case "scheduled_del":
		return m.handleScheduledDeleteCommand(ctx, client, frame)
	case "scheduled_update":
		return m.handleScheduledUpdateCommand(ctx, client, frame)
	case "scheduled_now":
		return m.handleScheduledNowCommand(ctx, client, frame)
	case "del":
		return m.handleDeleteCommand(ctx, client, frame)
	case "list":
		return m.handleListCommand(ctx, client, frame)
	default:
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", "unknown operation", false))
	}
}

func (m *Manager) HandleHeartbeatPayload(client *client, payload []byte) (bool, error) {
	var frame wireHeartbeatFrame
	if err := json.Unmarshal(payload, &frame); err != nil {
		return false, nil
	}
	if frame.Version != protocolVersion || frame.Kind != wireKindHeartbeat {
		return false, nil
	}
	client.MarkSeen()
	switch strings.ToLower(strings.TrimSpace(frame.Operation)) {
	case "ping":
		return true, client.send(newHeartbeatFrame("pong"))
	case "pong":
		return true, nil
	case "focus":
		client.SetFocusedSessionID(frame.SessionID)
		epoch, version, items := m.pendingStateSnapshot(frame.SessionID)
		return true, client.send(newPendingFrame(frame.SessionID, epoch, version, items))
	default:
		return true, nil
	}
}

func (m *Manager) SaveAttachment(fileHeader *multipart.FileHeader) (Attachment, error) {
	if fileHeader == nil {
		return Attachment{}, fmt.Errorf("file is required")
	}
	if fileHeader.Size <= 0 {
		return Attachment{}, fmt.Errorf("empty file")
	}
	if fileHeader.Size > m.cfg.AttachmentSizeLimit {
		return Attachment{}, fmt.Errorf("attachment too large")
	}

	file, err := fileHeader.Open()
	if err != nil {
		return Attachment{}, err
	}
	defer file.Close()

	buffer := bytes.NewBuffer(nil)
	written, err := io.Copy(buffer, io.LimitReader(file, m.cfg.AttachmentSizeLimit+1))
	if err != nil {
		return Attachment{}, err
	}
	if written > m.cfg.AttachmentSizeLimit {
		return Attachment{}, fmt.Errorf("attachment too large")
	}
	return m.saveAttachmentBytes(fileHeader.Filename, fileHeader.Header.Get("Content-Type"), buffer.Bytes())
}

func (m *Manager) saveAttachmentBytes(fileName, mimeType string, data []byte) (Attachment, error) {
	if len(data) == 0 {
		return Attachment{}, fmt.Errorf("empty file")
	}
	if int64(len(data)) > m.cfg.AttachmentSizeLimit {
		return Attachment{}, fmt.Errorf("attachment too large")
	}

	fileName = filepath.Base(strings.TrimSpace(fileName))
	if fileName == "" || fileName == "." {
		fileName = "image"
	}
	mimeType = strings.TrimSpace(mimeType)
	if mimeType == "" {
		mimeType = http.DetectContentType(data)
	} else if parsedMime, _, err := mime.ParseMediaType(mimeType); err == nil && parsedMime != "" {
		mimeType = parsedMime
	}

	attachmentID := utils.NewID()
	extension := filepath.Ext(fileName)
	targetPath := m.store.attachmentPath(attachmentID, extension)
	if err := os.WriteFile(targetPath, data, 0o644); err != nil {
		return Attachment{}, err
	}

	attachment := Attachment{
		ID:        attachmentID,
		Name:      fileName,
		Mime:      mimeType,
		Size:      int64(len(data)),
		Path:      targetPath,
		CreatedAt: time.Now(),
	}

	meta := attachmentMeta(attachment)
	metaBytes, err := json.Marshal(meta)
	if err == nil {
		_ = os.WriteFile(m.store.attachmentPath(attachmentID, ".json"), metaBytes, 0o644)
	}
	return attachment, nil
}

func (m *Manager) loadAttachment(id string) (Attachment, error) {
	metaPath := m.store.attachmentPath(id, ".json")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return Attachment{}, err
	}
	var meta attachmentMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return Attachment{}, err
	}
	return Attachment(meta), nil
}

func (m *Manager) GetAttachment(id string) (Attachment, error) {
	return m.loadAttachment(strings.TrimSpace(id))
}

func (m *Manager) handleCreateCommand(ctx context.Context, client *client, frame wireCommandFrame) error {
	var payload struct {
		ProjectID                         string  `json:"pid"`
		WorktreeID                        string  `json:"wid"`
		Agent                             string  `json:"ag"`
		ClaudeRuntime                     string  `json:"cr"`
		Model                             string  `json:"md"`
		ReasoningEffort                   string  `json:"re"`
		WorkflowMode                      string  `json:"wm"`
		PermissionLevel                   string  `json:"pl"`
		AutoRetryEnabled                  bool    `json:"ae"`
		AutoRetryPolicyMode               *string `json:"arpm"`
		AutoRetryScope                    *string `json:"ars"`
		AutoRetryPreset                   *string `json:"arp"`
		AutoRetryMaxAttempts              *int    `json:"aram"`
		AutoRetryDispatchPendingOnFailure *bool   `json:"ardpf"`
		PermissionMode                    string  `json:"pm"`
		Title                             string  `json:"ttl"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return client.send(newErrorFrame(frame.RequestID, "", "bad_req", "invalid create payload", false))
	}

	workflowMode := WorkflowMode(payload.WorkflowMode)
	permissionLevel := PermissionLevel(payload.PermissionLevel)
	if strings.TrimSpace(payload.PermissionMode) != "" {
		legacyWorkflowMode, legacyPermissionLevel := sessionModesFromLegacy(payload.PermissionMode)
		if strings.TrimSpace(payload.WorkflowMode) == "" {
			workflowMode = legacyWorkflowMode
		}
		if strings.TrimSpace(payload.PermissionLevel) == "" {
			permissionLevel = legacyPermissionLevel
		}
	}

	summary, err := m.CreateSession(ctx, CreateParams{
		ProjectID:                         payload.ProjectID,
		WorktreeID:                        payload.WorktreeID,
		Agent:                             Agent(payload.Agent),
		ClaudeRuntime:                     ClaudeRuntime(payload.ClaudeRuntime),
		Model:                             payload.Model,
		ReasoningEffort:                   ReasoningEffort(payload.ReasoningEffort),
		WorkflowMode:                      workflowMode,
		PermissionLevel:                   permissionLevel,
		AutoRetryEnabled:                  payload.AutoRetryEnabled,
		AutoRetryPolicyMode:               mapOptionalAutoRetryValue[AutoRetryPolicyMode](payload.AutoRetryPolicyMode),
		AutoRetryScope:                    mapOptionalAutoRetryValue[AutoRetryScope](payload.AutoRetryScope),
		AutoRetryPreset:                   mapOptionalAutoRetryValue[AutoRetryPreset](payload.AutoRetryPreset),
		AutoRetryMaxAttempts:              payload.AutoRetryMaxAttempts,
		AutoRetryDispatchPendingOnFailure: payload.AutoRetryDispatchPendingOnFailure,
		Title:                             payload.Title,
	})
	if err != nil {
		return client.send(newErrorFrame(frame.RequestID, "", "bad_req", err.Error(), false))
	}
	return client.send(newRevisionAckFrame(frame.RequestID, frame.Operation, summary.ID, summary.Revision, nil))
}

func (m *Manager) handleConnectCommand(ctx context.Context, client *client, frame wireCommandFrame) error {
	record, err := m.GetSession(ctx, frame.SessionID)
	if err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "not_found", err.Error(), false))
	}
	return client.send(newRevisionAckFrame(
		frame.RequestID,
		frame.Operation,
		frame.SessionID,
		formatSnapshotRevision(record.SnapshotRevision),
		nil,
	))
}

func (m *Manager) sendMutationAck(
	ctx context.Context,
	client *client,
	frame wireCommandFrame,
	payload any,
) error {
	summary := m.summaryForBroadcast(ctx, frame.SessionID)
	revision := ""
	if summary != nil {
		revision = summary.Revision
	} else {
		revision = m.currentSessionRevision(ctx, frame.SessionID)
	}
	if err := client.send(newRevisionAckFrame(
		frame.RequestID,
		frame.Operation,
		frame.SessionID,
		revision,
		payload,
	)); err != nil {
		return err
	}
	if summary != nil {
		m.broadcastCommittedFrame(frame.SessionID, newSessionFrame(frame.SessionID, *summary), revision)
	}
	return nil
}

func (m *Manager) sendPendingMutationAck(
	client *client,
	frame wireCommandFrame,
	payload any,
) error {
	epoch, version, items := m.pendingStateSnapshot(frame.SessionID)
	return client.send(newPendingAckFrame(
		frame.RequestID,
		frame.Operation,
		frame.SessionID,
		epoch,
		version,
		items,
		payload,
	))
}

func (m *Manager) handleHistoryCommand(ctx context.Context, client *client, frame wireCommandFrame) error {
	var payload struct {
		Limit int `json:"lim"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", "invalid history payload", false))
	}
	beforeSeq, err := parseBeforeCursor(frame.Payload)
	if err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", "invalid history cursor", false))
	}
	window, err := m.History(ctx, frame.SessionID, payload.Limit, beforeSeq)
	if err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "not_found", err.Error(), false))
	}
	if err := client.send(newRevisionAckFrame(
		frame.RequestID,
		frame.Operation,
		frame.SessionID,
		m.currentSessionRevision(ctx, frame.SessionID),
		nil,
	)); err != nil {
		return err
	}
	return client.send(newHistoryPageFrame(frame.SessionID, window))
}

func (m *Manager) handleAbortCommand(_ context.Context, client *client, frame wireCommandFrame) error {
	if err := m.AbortSessionForUser(frame.SessionID); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "invalid_state", err.Error(), false))
	}
	return m.sendMutationAck(context.Background(), client, frame, nil)
}

func (m *Manager) handleApprovalCommand(client *client, frame wireCommandFrame, action string) error {
	if err := m.respondToApproval(frame.SessionID, action); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "invalid_state", err.Error(), false))
	}
	return m.sendMutationAck(context.Background(), client, frame, nil)
}

func (m *Manager) handleUserInputCommand(client *client, frame wireCommandFrame) error {
	var payload struct {
		ItemID  string              `json:"iid"`
		Answers map[string][]string `json:"ans"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", "invalid user input payload", false))
	}
	if err := m.respondToUserInput(frame.SessionID, payload.ItemID, payload.Answers); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "invalid_state", err.Error(), false))
	}
	return m.sendMutationAck(context.Background(), client, frame, nil)
}

func (m *Manager) handlePendingDeleteCommand(client *client, frame wireCommandFrame) error {
	var payload struct {
		PendingID string `json:"id"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", "invalid pending delete payload", false))
	}
	if strings.TrimSpace(payload.PendingID) == "" {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", "pending id is required", false))
	}
	if !m.removePendingInput(frame.SessionID, payload.PendingID) {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "not_found", "pending input not found", false))
	}
	return m.sendPendingMutationAck(client, frame, nil)
}

func (m *Manager) handlePendingUpdateCommand(client *client, frame wireCommandFrame) error {
	var payload struct {
		PendingID string  `json:"id"`
		Text      *string `json:"txt"`
		Paused    *bool   `json:"paused"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", "invalid pending update payload", false))
	}
	if strings.TrimSpace(payload.PendingID) == "" {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", "pending id is required", false))
	}
	updated, err := m.updatePendingInput(frame.SessionID, payload.PendingID, pendingInputUpdate{
		Text:   payload.Text,
		Paused: payload.Paused,
	})
	if err != nil {
		code := "invalid_state"
		if errors.Is(err, errPendingInputNotFound) {
			code = "not_found"
		} else if errors.Is(err, errInvalidPendingInputUpdate) {
			code = "bad_req"
		}
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, code, err.Error(), false))
	}
	m.broadcastPendingInputs(frame.SessionID)
	m.triggerPendingProcessing(frame.SessionID)
	return m.sendPendingMutationAck(
		client,
		frame,
		mapWirePendingInputs([]PendingInput{updated})[0],
	)
}

func (m *Manager) handlePendingReorderCommand(client *client, frame wireCommandFrame) error {
	var payload struct {
		PendingID string `json:"id"`
		Mode      string `json:"mode"`
		Index     int    `json:"idx"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", "invalid pending reorder payload", false))
	}
	if strings.TrimSpace(payload.PendingID) == "" {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", "pending id is required", false))
	}
	if normalizePendingInputMode(PendingInputMode(payload.Mode)) == "" {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", "invalid pending mode", false))
	}
	if err := m.reorderPendingInput(frame.SessionID, payload.PendingID, PendingInputMode(payload.Mode), payload.Index); err != nil {
		code := "invalid_state"
		if errors.Is(err, errPendingInputNotFound) {
			code = "not_found"
		}
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, code, err.Error(), false))
	}
	m.broadcastPendingInputs(frame.SessionID)
	m.triggerPendingProcessing(frame.SessionID)
	return m.sendPendingMutationAck(client, frame, nil)
}

func (m *Manager) handlePendingClearCommand(client *client, frame wireCommandFrame) error {
	m.clearPendingInputsForSession(frame.SessionID)
	return m.sendPendingMutationAck(client, frame, nil)
}

func (m *Manager) handleRenameCommand(ctx context.Context, client *client, frame wireCommandFrame) error {
	var payload struct {
		Title string `json:"ttl"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", "invalid rename payload", false))
	}
	_, err := m.RenameSession(ctx, frame.SessionID, payload.Title)
	if err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", err.Error(), false))
	}
	return m.sendMutationAck(ctx, client, frame, nil)
}

func (m *Manager) handleSetModelCommand(ctx context.Context, client *client, frame wireCommandFrame) error {
	var payload struct {
		Model string `json:"md"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", "invalid model payload", false))
	}
	if _, err := m.UpdateModel(ctx, frame.SessionID, payload.Model); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", err.Error(), false))
	}
	return m.sendMutationAck(ctx, client, frame, nil)
}

func (m *Manager) handleSetClaudeRuntimeCommand(ctx context.Context, client *client, frame wireCommandFrame) error {
	var payload struct {
		ClaudeRuntime string `json:"cr"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", "invalid claude runtime payload", false))
	}
	if _, err := m.UpdateClaudeRuntime(ctx, frame.SessionID, ClaudeRuntime(payload.ClaudeRuntime)); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", err.Error(), false))
	}
	return m.sendMutationAck(ctx, client, frame, nil)
}

func (m *Manager) handleSetReasoningEffortCommand(
	ctx context.Context,
	client *client,
	frame wireCommandFrame,
) error {
	var payload struct {
		ReasoningEffort string `json:"re"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", "invalid reasoning payload", false))
	}
	if _, err := m.UpdateReasoningEffort(ctx, frame.SessionID, ReasoningEffort(payload.ReasoningEffort)); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", err.Error(), false))
	}
	return m.sendMutationAck(ctx, client, frame, nil)
}

func (m *Manager) handleSetWorkflowModeCommand(ctx context.Context, client *client, frame wireCommandFrame) error {
	var payload struct {
		WorkflowMode string `json:"wm"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", "invalid workflow payload", false))
	}
	if _, err := m.UpdateWorkflowMode(ctx, frame.SessionID, WorkflowMode(payload.WorkflowMode)); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", err.Error(), false))
	}
	return m.sendMutationAck(ctx, client, frame, nil)
}

func (m *Manager) handleGoalGetCommand(ctx context.Context, client *client, frame wireCommandFrame) error {
	goal, err := m.GetSessionGoal(ctx, frame.SessionID)
	if err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", err.Error(), false))
	}
	return client.send(newRevisionAckFrame(
		frame.RequestID,
		frame.Operation,
		frame.SessionID,
		m.currentSessionRevision(ctx, frame.SessionID),
		wireGoalPayload{Goal: mapWireGoal(goal)},
	))
}

func (m *Manager) handleMarkReadCommand(ctx context.Context, client *client, frame wireCommandFrame) error {
	var payload struct {
		AttentionRevision string `json:"ar"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", "invalid mark read payload", false))
	}
	state, err := m.MarkSessionRead(ctx, frame.SessionID, payload.AttentionRevision)
	if err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", err.Error(), false))
	}
	return client.send(newAckFrame(frame.RequestID, frame.Operation, frame.SessionID, map[string]any{
		"hasUnread":         state.HasUnread,
		"attentionRevision": state.AttentionRevision,
	}))
}

func (m *Manager) handleGoalSetCommand(ctx context.Context, client *client, frame wireCommandFrame) error {
	var payload struct {
		Objective string `json:"obj"`
		Status    string `json:"st"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", "invalid goal payload", false))
	}
	if _, err := m.SetSessionGoal(ctx, frame.SessionID, payload.Objective, GoalStatus(payload.Status)); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", err.Error(), false))
	}
	return m.sendMutationAck(ctx, client, frame, nil)
}

func (m *Manager) handleGoalBootstrapCommand(ctx context.Context, client *client, frame wireCommandFrame) error {
	var payload struct {
		Objective string `json:"obj"`
		Status    string `json:"st"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", "invalid goal bootstrap payload", false))
	}
	if err := m.BootstrapSessionGoal(ctx, frame.SessionID, payload.Objective, GoalStatus(payload.Status)); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", err.Error(), false))
	}
	return m.sendMutationAck(ctx, client, frame, nil)
}

func (m *Manager) handleGoalStatusCommand(
	ctx context.Context,
	client *client,
	frame wireCommandFrame,
	status GoalStatus,
) error {
	if _, err := m.UpdateSessionGoalStatus(ctx, frame.SessionID, status); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", err.Error(), false))
	}
	return m.sendMutationAck(ctx, client, frame, nil)
}

func (m *Manager) handleGoalClearCommand(ctx context.Context, client *client, frame wireCommandFrame) error {
	if _, err := m.ClearSessionGoal(ctx, frame.SessionID); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", err.Error(), false))
	}
	return m.sendMutationAck(ctx, client, frame, nil)
}

func (m *Manager) handleSetPermissionLevelCommand(ctx context.Context, client *client, frame wireCommandFrame) error {
	var payload struct {
		PermissionLevel string `json:"pl"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", "invalid permission payload", false))
	}
	if _, err := m.UpdatePermissionLevel(ctx, frame.SessionID, PermissionLevel(payload.PermissionLevel)); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", err.Error(), false))
	}
	return m.sendMutationAck(ctx, client, frame, nil)
}

func (m *Manager) handleSetActiveCallTimeoutCommand(
	ctx context.Context,
	client *client,
	frame wireCommandFrame,
) error {
	var payload struct {
		Enabled bool `json:"acte"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", "invalid active call timeout payload", false))
	}
	if _, err := m.UpdateActiveCallTimeout(ctx, frame.SessionID, payload.Enabled); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", err.Error(), false))
	}
	return m.sendMutationAck(ctx, client, frame, nil)
}

func (m *Manager) handleSetAutoRetryCommand(
	ctx context.Context,
	client *client,
	frame wireCommandFrame,
) error {
	var payload struct {
		Enabled     bool    `json:"ae"`
		PolicyMode  *string `json:"arpm"`
		Scope       *string `json:"ars"`
		Preset      *string `json:"arp"`
		MaxAttempts *int    `json:"aram"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", "invalid auto retry payload", false))
	}
	if _, err := m.UpdateAutoRetry(
		ctx,
		frame.SessionID,
		payload.Enabled,
		mapOptionalAutoRetryValue[AutoRetryPolicyMode](payload.PolicyMode),
		mapOptionalAutoRetryValue[AutoRetryScope](payload.Scope),
		mapOptionalAutoRetryValue[AutoRetryPreset](payload.Preset),
		payload.MaxAttempts,
	); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", err.Error(), false))
	}
	return m.sendMutationAck(ctx, client, frame, nil)
}

func mapOptionalAutoRetryValue[T ~string](value *string) *T {
	if value == nil {
		return nil
	}
	mapped := T(*value)
	return &mapped
}

func (m *Manager) handleSetAutoRetryDispatchPendingOnFailureCommand(
	ctx context.Context,
	client *client,
	frame wireCommandFrame,
) error {
	var payload struct {
		Enabled bool `json:"ardpf"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", "invalid auto retry pending dispatch payload", false))
	}
	if _, err := m.UpdateAutoRetryDispatchPendingOnFailure(ctx, frame.SessionID, payload.Enabled); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", err.Error(), false))
	}
	return m.sendMutationAck(ctx, client, frame, nil)
}

func (m *Manager) handleLegacySetModeCommand(ctx context.Context, client *client, frame wireCommandFrame) error {
	var payload struct {
		PermissionMode string `json:"pm"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", "invalid legacy mode payload", false))
	}
	workflowMode, permissionLevel := sessionModesFromLegacy(payload.PermissionMode)
	if _, err := m.UpdateWorkflowMode(ctx, frame.SessionID, workflowMode); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", err.Error(), false))
	}
	if _, err := m.UpdatePermissionLevel(ctx, frame.SessionID, permissionLevel); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", err.Error(), false))
	}
	return m.sendMutationAck(ctx, client, frame, nil)
}

func (m *Manager) handleSetAgentCommand(ctx context.Context, client *client, frame wireCommandFrame) error {
	var payload struct {
		Agent string `json:"ag"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", "invalid agent payload", false))
	}
	if _, err := m.UpdateAgent(ctx, frame.SessionID, Agent(payload.Agent)); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", err.Error(), false))
	}
	return m.sendMutationAck(ctx, client, frame, nil)
}

func (m *Manager) handleMoveCommand(ctx context.Context, client *client, frame wireCommandFrame) error {
	var payload struct {
		PrevSessionID string `json:"prv"`
		NextSessionID string `json:"nxt"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", "invalid move payload", false))
	}
	summary, err := m.MoveSession(ctx, frame.SessionID, payload.PrevSessionID, payload.NextSessionID)
	if err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", err.Error(), false))
	}
	m.broadcastProjectSessionSummaries(ctx, summary.ProjectID)
	return client.send(newRevisionAckFrame(
		frame.RequestID,
		frame.Operation,
		frame.SessionID,
		m.currentSessionRevision(ctx, frame.SessionID),
		nil,
	))
}

func (m *Manager) handleDeleteCommand(ctx context.Context, client *client, frame wireCommandFrame) error {
	if err := m.DeleteSession(ctx, frame.SessionID); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "internal", err.Error(), false))
	}
	return client.send(newAckFrame(frame.RequestID, frame.Operation, frame.SessionID, nil))
}

func (m *Manager) handleListCommand(ctx context.Context, client *client, frame wireCommandFrame) error {
	var payload struct {
		ProjectID string `json:"pid"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", "invalid list payload", false))
	}
	items, err := m.ListSessions(ctx, payload.ProjectID)
	if err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "internal", err.Error(), false))
	}
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, map[string]any{
			"id":  item.ID,
			"ttl": item.Title,
			"ag":  item.Agent,
			"st":  item.Status,
			"oi":  item.OrderIndex,
			"lu":  item.UpdatedAt.UnixMilli(),
		})
	}
	return client.send(newAckFrame(frame.RequestID, frame.Operation, frame.SessionID, map[string]any{"items": result}))
}

func (m *Manager) handleCompactCommand(ctx context.Context, client *client, frame wireCommandFrame) error {
	if len(bytes.TrimSpace(frame.Payload)) > 0 && string(bytes.TrimSpace(frame.Payload)) != "{}" {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", "compact takes no payload", false))
	}
	if err := m.CompactSession(ctx, frame.SessionID); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "invalid_state", err.Error(), false))
	}
	return m.sendMutationAck(ctx, client, frame, nil)
}

func (m *Manager) handlePiTreeGetCommand(ctx context.Context, client *client, frame wireCommandFrame) error {
	if !m.SupportsPiSessionTree() {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "unsupported", "Pi session tree is not supported", false))
	}
	if len(bytes.TrimSpace(frame.Payload)) > 0 && string(bytes.TrimSpace(frame.Payload)) != "{}" {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", "tree_get takes no payload", false))
	}
	tree, err := m.GetPiSessionTree(ctx, frame.SessionID)
	if err != nil {
		return client.send(newPiTreeErrorFrame(frame, err))
	}
	return client.send(newRevisionAckFrame(
		frame.RequestID,
		frame.Operation,
		frame.SessionID,
		m.currentSessionRevision(ctx, frame.SessionID),
		tree,
	))
}

func (m *Manager) handlePiTreeNavigateCommand(ctx context.Context, client *client, frame wireCommandFrame) error {
	if !m.SupportsPiSessionTree() {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "unsupported", "Pi session tree is not supported", false))
	}
	var payload struct {
		TargetID  string `json:"tid"`
		Revision  string `json:"rev"`
		Summarize bool   `json:"sum"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", "invalid tree navigation payload", false))
	}
	result, err := m.NavigatePiSessionTree(ctx, frame.SessionID, PiTreeNavigateInput{
		TargetID: payload.TargetID, Revision: payload.Revision, Summarize: payload.Summarize,
	})
	if err != nil {
		return client.send(newPiTreeErrorFrame(frame, err))
	}
	return client.send(newRevisionAckFrame(
		frame.RequestID,
		frame.Operation,
		frame.SessionID,
		m.currentSessionRevision(ctx, frame.SessionID),
		result,
	))
}

func (m *Manager) handlePiTreeForkCommand(ctx context.Context, client *client, frame wireCommandFrame) error {
	if !m.SupportsPiSessionTree() {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "unsupported", "Pi session tree is not supported", false))
	}
	var payload struct {
		TargetID string `json:"tid"`
		Revision string `json:"rev"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", "invalid tree fork payload", false))
	}
	result, err := m.ForkPiSessionTree(ctx, frame.SessionID, PiTreeForkInput{TargetID: payload.TargetID, Revision: payload.Revision})
	if err != nil {
		return client.send(newPiTreeErrorFrame(frame, err))
	}
	return client.send(newAckFrame(frame.RequestID, frame.Operation, frame.SessionID, mapPiTreeCreateWireResult(result)))
}

func (m *Manager) handlePiTreeCloneCommand(ctx context.Context, client *client, frame wireCommandFrame) error {
	if !m.SupportsPiSessionTree() {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "unsupported", "Pi session tree is not supported", false))
	}
	var payload struct {
		Revision string `json:"rev"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", "invalid tree clone payload", false))
	}
	result, err := m.ClonePiSessionTree(ctx, frame.SessionID, PiTreeCloneInput{Revision: payload.Revision})
	if err != nil {
		return client.send(newPiTreeErrorFrame(frame, err))
	}
	return client.send(newAckFrame(frame.RequestID, frame.Operation, frame.SessionID, mapPiTreeCreateWireResult(result)))
}

func newPiTreeErrorFrame(frame wireCommandFrame, err error) wireFrame {
	publicErr := ClassifyPiTreeError(err)
	return newErrorFrame(frame.RequestID, frame.SessionID, publicErr.Code, publicErr.Message, false)
}

func mapPiTreeCreateWireResult(result PiTreeCreateResult) map[string]any {
	return map[string]any{
		"s":          mapWireSession(result.Session),
		"tree":       result.Tree,
		"editorText": result.EditorText,
	}
}

func (m *Manager) handleSendCommand(ctx context.Context, client *client, frame wireCommandFrame) error {
	var payload struct {
		Text        string   `json:"txt"`
		Attachments []string `json:"atts"`
		Mode        string   `json:"mode"`
		PendingID   string   `json:"pid"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", "invalid send payload", false))
	}
	result, err := m.sendMessageWithModeResult(
		ctx,
		frame.SessionID,
		payload.Text,
		payload.Attachments,
		PendingInputMode(payload.Mode),
		payload.PendingID,
	)
	if err != nil {
		if errors.Is(err, ErrCodexRunDrainTimeout) {
			return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "codex_drain_timeout", err.Error(), true))
		}
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "invalid_state", err.Error(), false))
	}
	if result.Pending {
		return m.sendPendingMutationAck(client, frame, nil)
	}
	return m.sendMutationAck(ctx, client, frame, nil)
}

func (m *Manager) handleFreshContextSendCommand(ctx context.Context, client *client, frame wireCommandFrame) error {
	var payload struct {
		Text        string   `json:"txt"`
		Attachments []string `json:"atts"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", "invalid fresh send payload", false))
	}
	if err := m.SendMessageWithFreshContext(ctx, frame.SessionID, payload.Text, payload.Attachments); err != nil {
		if errors.Is(err, ErrCodexRunDrainTimeout) {
			return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "codex_drain_timeout", err.Error(), true))
		}
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "invalid_state", err.Error(), false))
	}
	return m.sendMutationAck(ctx, client, frame, nil)
}

func (m *Manager) SendMessage(ctx context.Context, sessionID, text string, attachmentIDs []string) error {
	return m.sendMessageInternal(ctx, sessionID, text, attachmentIDs, sendMessageOptions{
		updateAutoTitle: true,
	})
}

func (m *Manager) SendMessageWithFreshContext(
	ctx context.Context,
	sessionID,
	text string,
	attachmentIDs []string,
) error {
	return m.sendMessageInternal(ctx, sessionID, text, attachmentIDs, sendMessageOptions{
		freshCodexContext: true,
	})
}

func (m *Manager) CompactSession(ctx context.Context, sessionID string) error {
	dispatchLock := &m.sessionDispatchLocks[sessionRevisionLockIndex(sessionID)]
	dispatchLock.Lock()
	defer dispatchLock.Unlock()

	record, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if record.ArchivedAt != nil {
		return errors.New("session is archived")
	}
	if normalizeAgent(Agent(record.Agent)) != AgentPi || effectiveSessionBackend(record) != SessionBackendPiRPC {
		return errors.New("manual compaction is only supported for Pi RPC sessions")
	}
	if err := m.ensureSessionMessagingAvailable(record); err != nil {
		return err
	}
	if record.NativeSessionID == nil || strings.TrimSpace(*record.NativeSessionID) == "" ||
		record.ThreadPath == nil || strings.TrimSpace(*record.ThreadPath) == "" {
		return errors.New("Pi session has no native history to compact")
	}
	m.cancelAutoRetryTimer(sessionID)
	if m.hasActiveRun(sessionID) {
		return errors.New("session is already running")
	}

	runID := utils.NewID()
	now := time.Now()
	if _, err := m.appendAndBroadcast(ctx, sessionID, record, Event{
		ID: utils.NewID(), Type: "run_st", RunID: runID, Timestamp: now,
		Payload: map[string]any{
			"ag": string(AgentPi), "md": record.Model, "re": record.ReasoningEffort,
			"wm": effectiveWorkflowMode(record), "pl": effectivePermissionLevel(record), "src": "compact",
		},
	}); err != nil {
		return err
	}
	if err := m.updateRuntimeState(ctx, sessionID, applyAssistantStateUpdates(map[string]any{
		"status": string(StatusRunning), "has_unread": false, "last_error": nil,
		"auto_retry_attempt": 0, "auto_retry_next_at": nil, "auto_retry_last_error_code": nil,
		"updated_at": now,
	}, AssistantStateWorking, now)); err != nil {
		return err
	}
	m.broadcastSessionSummary(ctx, sessionID)

	runCtx, cancel := context.WithCancel(context.Background())
	run := &activeRun{
		sessionID: sessionID, projectID: record.ProjectID, agent: AgentPi, backend: SessionBackendPiRPC,
		runID: runID, cancel: cancel, done: make(chan struct{}), piCompaction: true,
	}
	m.mu.Lock()
	delete(m.codexTerminationRequests, sessionID)
	m.runs[sessionID] = run
	m.mu.Unlock()
	go m.runSession(runCtx, run, record, "", nil)
	return nil
}

func buildGoalBootstrapPrompt(objective string) string {
	return fmt.Sprintf(goalBootstrapPromptPreamble, strings.TrimSpace(objective))
}

const incompleteTurnContinuationPrompt = `<codekanban_internal_continue reason="missing_final_answer">
Continue from where you stopped. Finish the user's request and provide a non-empty final answer.
</codekanban_internal_continue>`

func isGoalBootstrapPrompt(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	return strings.Contains(trimmed, "<goal_bootstrap>") &&
		strings.Contains(trimmed, "</goal_bootstrap>") &&
		strings.Contains(trimmed, "\nGoal:\n")
}

func isIncompleteTurnContinuationPrompt(text string) bool {
	trimmed := strings.TrimSpace(text)
	return strings.Contains(trimmed, `<codekanban_internal_continue reason="missing_final_answer">`) &&
		strings.Contains(trimmed, "</codekanban_internal_continue>")
}

func isHiddenCodexPrompt(text string) bool {
	return isGoalBootstrapPrompt(text) || isIncompleteTurnContinuationPrompt(text)
}

func (m *Manager) startHiddenSessionRun(
	ctx context.Context,
	record tables.WebSessionTable,
	text string,
	attachments []Attachment,
	bootstrapGoalObjective string,
	bootstrapGoalStatus GoalStatus,
) error {
	text = strings.TrimSpace(text)
	if text == "" && len(attachments) == 0 {
		return fmt.Errorf("message is empty")
	}
	m.cancelAutoRetryTimer(record.ID)
	if m.hasActiveRun(record.ID) {
		return fmt.Errorf("session is already running")
	}

	runID := utils.NewID()
	now := time.Now()
	updates := applyAssistantStateUpdates(map[string]any{
		"status":                     string(StatusRunning),
		"has_unread":                 false,
		"last_error":                 nil,
		"auto_retry_attempt":         0,
		"auto_retry_next_at":         nil,
		"auto_retry_last_error_code": nil,
		"updated_at":                 now,
	}, AssistantStateWorking, now)
	if err := model.GetDB().WithContext(ctx).Model(&tables.WebSessionTable{}).
		Where("id = ?", record.ID).
		Updates(updates).Error; err != nil {
		return err
	}

	if _, err := m.appendAndBroadcast(ctx, record.ID, record, Event{
		ID:        utils.NewID(),
		Seq:       0,
		Type:      "run_st",
		RunID:     runID,
		Timestamp: now,
		Payload: map[string]any{
			"ag":  string(normalizeAgent(Agent(record.Agent))),
			"md":  record.Model,
			"re":  record.ReasoningEffort,
			"wm":  effectiveWorkflowMode(record),
			"pl":  effectivePermissionLevel(record),
			"src": "goal_bootstrap",
		},
	}); err != nil {
		return err
	}

	m.broadcastSessionSummary(ctx, record.ID)

	runCtx, cancel := context.WithCancel(context.Background())
	run := &activeRun{
		sessionID:              record.ID,
		projectID:              record.ProjectID,
		agent:                  Agent(record.Agent),
		backend:                effectiveSessionBackend(record),
		runID:                  runID,
		cancel:                 cancel,
		done:                   make(chan struct{}),
		hiddenBootstrap:        true,
		bootstrapGoalObjective: strings.TrimSpace(bootstrapGoalObjective),
		bootstrapGoalState: func() GoalStatus {
			if bootstrapGoalStatus == "" {
				return GoalStatusActive
			}
			return bootstrapGoalStatus
		}(),
	}
	if run.bootstrapGoalObjective != "" {
		run.bootstrapResult = make(chan error, 1)
	}

	m.mu.Lock()
	delete(m.codexTerminationRequests, record.ID)
	m.runs[record.ID] = run
	m.mu.Unlock()
	if run.backend == SessionBackendCodexAppServer && normalizeAgent(run.agent) == AgentCodex {
		m.broadcastCodexAppServerRuntime(record.ID)
	}

	go m.runSession(runCtx, run, record, text, attachments)
	if run.bootstrapResult == nil {
		return nil
	}
	select {
	case err := <-run.bootstrapResult:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(10 * time.Second):
		return fmt.Errorf("goal bootstrap timed out")
	}
}

func (m *Manager) sendMessageInternal(
	ctx context.Context,
	sessionID,
	text string,
	attachmentIDs []string,
	options sendMessageOptions,
) error {
	dispatchLock := &m.sessionDispatchLocks[sessionRevisionLockIndex(sessionID)]
	dispatchLock.Lock()
	defer dispatchLock.Unlock()

	record, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if record.ArchivedAt != nil {
		return fmt.Errorf("session is archived")
	}
	if err := m.ensureSessionMessagingAuthorized(ctx, record); err != nil {
		return err
	}
	attachments := make([]Attachment, 0, len(attachmentIDs))
	for _, id := range attachmentIDs {
		attachment, err := m.loadAttachment(strings.TrimSpace(id))
		if err != nil {
			return fmt.Errorf("attachment %s not found", id)
		}
		attachments = append(attachments, attachment)
	}
	text = strings.TrimSpace(text)
	if text == "" && len(attachments) == 0 {
		return fmt.Errorf("message is empty")
	}
	if options.freshCodexContext {
		if normalizeAgent(Agent(record.Agent)) != AgentCodex ||
			effectiveSessionBackend(record) != SessionBackendCodexAppServer {
			return fmt.Errorf("fresh context is only supported for Codex app-server sessions")
		}
		if err := m.stopRunForFreshContext(sessionID, 5*time.Second); err != nil {
			return err
		}
	}
	m.cancelAutoRetryTimer(sessionID)
	if err := m.waitForCodexRunDrain(ctx, sessionID); err != nil {
		return err
	}
	if m.hasActiveRun(sessionID) {
		return fmt.Errorf("session is already running")
	}
	if options.freshCodexContext {
		if err := m.resetCodexContextForFreshSend(ctx, record); err != nil {
			return err
		}
		refreshed, err := m.GetSession(ctx, sessionID)
		if err != nil {
			return err
		}
		record = refreshed
	}

	defaultAutoRetryUpdates := map[string]any(nil)
	if !options.fromAutoRetry {
		defaultAutoRetryUpdates = m.refreshDefaultAutoRetryPolicy(&record)
	}

	runID := utils.NewID()
	userMessageID := strings.TrimSpace(options.userMessageID)
	if userMessageID == "" {
		userMessageID = utils.NewID()
	}

	if _, err := m.appendAndBroadcast(ctx, sessionID, record, Event{
		ID:        utils.NewID(),
		Seq:       0,
		Type:      "msg_u",
		RunID:     runID,
		ParentID:  userMessageID,
		Timestamp: time.Now(),
		Payload: map[string]any{
			"mid":  userMessageID,
			"txt":  text,
			"atts": attachmentPayloads(attachments),
		},
	}); err != nil {
		return err
	}
	if _, err := m.appendAndBroadcast(ctx, sessionID, record, Event{
		ID:        utils.NewID(),
		Seq:       0,
		Type:      "run_st",
		RunID:     runID,
		Timestamp: time.Now(),
		Payload: map[string]any{
			"ag":                     string(normalizeAgent(Agent(record.Agent))),
			"md":                     record.Model,
			"re":                     record.ReasoningEffort,
			"wm":                     effectiveWorkflowMode(record),
			"pl":                     effectivePermissionLevel(record),
			"autoRetry":              options.fromAutoRetry,
			"workTimingContinuation": options.continueWorkTiming,
		},
	}); err != nil {
		return err
	}

	now := time.Now()
	markStatus := StatusRunning
	updates := map[string]any{
		"status":                     string(markStatus),
		"has_unread":                 false,
		"last_error":                 nil,
		"auto_retry_last_error_code": nil,
		"updated_at":                 now,
		"last_message_at":            now,
	}
	for key, value := range defaultAutoRetryUpdates {
		updates[key] = value
	}
	if options.fromAutoRetry {
		updates["auto_retry_next_at"] = nil
	} else {
		updates["auto_retry_attempt"] = 0
		updates["auto_retry_next_at"] = nil
	}
	updates = applyAssistantStateUpdates(updates, AssistantStateWorking, now)
	titleChanged := false
	if options.updateAutoTitle && record.TitleAuto {
		if autoTitle := deriveAutoTitleFromMessage(text); autoTitle != "" {
			updates["title_auto"] = false
			if strings.TrimSpace(record.Title) != autoTitle {
				updates["title"] = autoTitle
				titleChanged = true
			}
		}
	}

	db := model.GetDB()
	if db == nil {
		return model.ErrDBNotInitialized
	}
	if err := db.WithContext(ctx).Model(&tables.WebSessionTable{}).
		Where("id = ?", sessionID).
		Updates(updates).Error; err != nil {
		return err
	}
	m.broadcastSessionSummary(ctx, sessionID)
	if titleChanged && m.logger != nil {
		m.logger.Debug("auto-renamed web session title",
			zap.String("sessionId", sessionID),
		)
	}

	runCtx, cancel := context.WithCancel(context.Background())
	run := &activeRun{
		sessionID:     sessionID,
		projectID:     record.ProjectID,
		agent:         Agent(record.Agent),
		backend:       effectiveSessionBackend(record),
		runID:         runID,
		fromAutoRetry: options.fromAutoRetry,
		cancel:        cancel,
		done:          make(chan struct{}),
	}

	m.mu.Lock()
	delete(m.codexTerminationRequests, sessionID)
	m.runs[sessionID] = run
	m.mu.Unlock()
	if run.backend == SessionBackendCodexAppServer && normalizeAgent(run.agent) == AgentCodex {
		m.broadcastCodexAppServerRuntime(sessionID)
	}

	go m.runSession(runCtx, run, record, text, attachments)
	return nil
}

type sendMessageOptions struct {
	fromAutoRetry      bool
	continueWorkTiming bool
	updateAutoTitle    bool
	userMessageID      string
	freshCodexContext  bool
}

func (m *Manager) resetCodexContextForFreshSend(ctx context.Context, record tables.WebSessionTable) error {
	now := time.Now()
	updates := contextEstimateBaselineResetUpdate(record, now)
	for key, value := range map[string]any{
		"workflow_mode":                      string(WorkflowModeDefault),
		"session_start_source":               string(SessionStartSourceClear),
		"native_session_id":                  nil,
		"native_leaf_id":                     nil,
		"source_revision":                    nil,
		"cyber_policy_flagged":               false,
		"sync_state":                         SyncStateMissing,
		"last_sync_mode":                     "",
		"source_created_at":                  nil,
		"source_updated_at":                  nil,
		"last_synced_at":                     nil,
		"thread_path":                        nil,
		"thread_preview":                     nil,
		"sync_error":                         nil,
		"last_completed_input_tokens":        record.TotalInputTokens,
		"last_completed_cached_input_tokens": record.TotalCachedInputTokens,
		"last_completed_output_tokens":       record.TotalOutputTokens,
		"latest_turn_input_tokens":           0,
		"latest_turn_cached_input_tokens":    0,
		"latest_turn_output_tokens":          0,
		"latest_turn_usage_updated_at":       nil,
		"updated_at":                         now,
	} {
		updates[key] = value
	}
	if err := m.updateRuntimeState(ctx, record.ID, updates); err != nil {
		return err
	}
	m.broadcastSessionSummary(ctx, record.ID)
	return nil
}

func (m *Manager) ensureSessionMessagingAuthorized(
	ctx context.Context,
	record tables.WebSessionTable,
) error {
	agent, err := validateAgent(Agent(record.Agent))
	if err != nil {
		return err
	}
	if agent != AgentPi {
		return nil
	}
	return m.EnsureProjectPiTrust(ctx, record.ProjectID, record.Cwd)
}

func (m *Manager) ensureSessionMessagingAvailable(record tables.WebSessionTable) error {
	agent, err := validateAgent(Agent(record.Agent))
	if err != nil {
		return err
	}
	switch agent {
	case AgentCodex:
		config := m.GetCodexRuntimeConfig()
		if !config.HasCodex {
			return fmt.Errorf("%s", errCodexNotInstalled)
		}
	case AgentClaude:
		config := m.GetCodexRuntimeConfig()
		if !config.HasClaudeCode {
			return fmt.Errorf("%s", errClaudeCodeNotInstalled)
		}
	case AgentPi:
		if !m.getPiRuntimeProbe().compatible {
			return fmt.Errorf("%s", errPiWebSessionUnavailable)
		}
	}
	return m.ensureSessionMessagingAuthorized(context.Background(), record)
}

func (m *Manager) ensureCodexMultiAgentV2Supported() error {
	return codexMultiAgentV2SupportError(m.GetCodexRuntimeConfig())
}

func codexMultiAgentV2SupportError(config CodexRuntimeConfig) error {
	if !config.HasCodex {
		return codexMultiAgentV2UnavailableError{message: errCodexNotInstalled}
	}
	if config.SupportsMultiAgentV2 {
		return nil
	}
	minimumVersion := strings.TrimSpace(config.MultiAgentV2MinVersion)
	if minimumVersion == "" {
		minimumVersion = multiAgentV2MinCodexVersion.String()
	}
	if config.CodexVersion != nil && strings.TrimSpace(*config.CodexVersion) != "" {
		return codexMultiAgentV2UnavailableError{message: fmt.Sprintf(
			"This Codex feature requires multi-agent V2 (Codex >= %s). Current version: %s.",
			minimumVersion,
			strings.TrimSpace(*config.CodexVersion),
		)}
	}
	return codexMultiAgentV2UnavailableError{message: fmt.Sprintf(
		"This Codex feature requires multi-agent V2 (Codex >= %s). The installed Codex version could not be determined.",
		minimumVersion,
	)}
}

func (m *Manager) ensureSessionGoalModeSupported(record tables.WebSessionTable) error {
	config := m.GetCodexRuntimeConfig()
	if !config.HasCodex {
		return errors.New(errCodexNotInstalled)
	}
	if config.SupportsGoalMode {
		return nil
	}
	if config.CodexVersion != nil && strings.TrimSpace(*config.CodexVersion) != "" {
		return fmt.Errorf(
			"Goal mode requires Codex >= %s. Current version: %s.",
			config.GoalModeMinVersion,
			strings.TrimSpace(*config.CodexVersion),
		)
	}
	return fmt.Errorf("Goal mode requires Codex >= %s.", config.GoalModeMinVersion)
}

func (m *Manager) runSession(ctx context.Context, run *activeRun, session tables.WebSessionTable, text string, attachments []Attachment) {
	startedAt := time.Now()
	defer func() {
		m.logRunCompletion(ctx, run, session, time.Since(startedAt))
		run.stopCodexRolloutMonitor()
		run.codexCollaboration.clear()
		run.resetActiveCallTracking()
		run.closeInput()
		run.clearPendingApproval()
		run.clearPendingServerRequest()
		m.finishCodexRunDrain(session.ID, run)
		close(run.done)
		if m.releaseActiveRun(session.ID, run) && run.syncSourceAfterRun && !m.hasPendingSessionWork(session.ID) {
			m.maybeSyncSessionAfterRun(session)
		}
		if run.backend == SessionBackendCodexAppServer && normalizeAgent(run.agent) == AgentCodex {
			m.broadcastCodexAppServerRuntime(session.ID)
		}
	}()
	if normalizeAgent(Agent(session.Agent)) == AgentClaude {
		nativeSessionID := ""
		if session.NativeSessionID != nil {
			nativeSessionID = strings.TrimSpace(*session.NativeSessionID)
		}
		run.setClaudeIdentity(nativeSessionID, session.Cwd)
	}

	if run.backend == SessionBackendCodexAppServer && normalizeAgent(Agent(session.Agent)) == AgentCodex {
		m.runCodexAppServerSession(ctx, run, session, text, attachments)
		return
	}
	if run.backend == SessionBackendPiRPC && normalizeAgent(Agent(session.Agent)) == AgentPi {
		if run.piCompaction {
			m.runPiRPCCompaction(ctx, run, session)
		} else {
			m.runPiRPCSession(ctx, run, session, text, attachments)
		}
		return
	}
	if run.claudeResumeOnly && normalizeAgent(Agent(session.Agent)) == AgentClaude {
		m.runClaudeResumeSession(ctx, run, session)
		return
	}

	cmd, stdinBytes, closeStdinAfterWrite, err := m.buildExecCommand(ctx, session, text, attachments)
	if err != nil {
		m.handleRunFailure(session.ID, session, run, err)
		return
	}
	run.setCommand(cmd)
	claudeStdioControl := normalizeAgent(Agent(session.Agent)) == AgentClaude && isClaudeControlCommand(cmd)
	if claudeStdioControl {
		run.setClaudeStdioControl(true)
		defer run.setClaudeStdioControl(false)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		m.handleRunFailure(session.ID, session, run, err)
		return
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		m.handleRunFailure(session.ID, session, run, err)
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		m.handleRunFailure(session.ID, session, run, err)
		return
	}

	if err := cmd.Start(); err != nil {
		m.handleRunFailure(session.ID, session, run, err)
		return
	}
	run.setInput(stdin)

	go func() {
		if len(stdinBytes) > 0 {
			_, _ = stdin.Write(stdinBytes)
		}
		// Claude's stdio permission bridge needs the pipe to remain writable
		// while a control_request is waiting for the browser. The stream is
		// closed when Claude emits its result (or an end_turn assistant frame).
		if closeStdinAfterWrite && !isClaudeControlCommand(cmd) {
			_ = stdin.Close()
			run.clearInput()
		}
	}()

	stderrBuffer := bytes.NewBuffer(nil)
	stderrDone := make(chan struct{})
	go func() {
		defer close(stderrDone)
		m.consumeRuntimePlainOutput(ctx, session, run, io.TeeReader(stderr, stderrBuffer))
	}()

	m.consumeRuntimeOutput(ctx, session, run, stdout)

	waitErr := cmd.Wait()
	<-stderrDone
	if ctx.Err() != nil {
		abortPayload := activeCallTimeoutAbortPayload(session, run.abortEventPayload())
		now := time.Now()
		_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
			ID:        utils.NewID(),
			Seq:       0,
			Type:      "run_abort",
			RunID:     run.runID,
			Timestamp: now,
			Payload:   abortPayload,
		})
		_ = m.updateRuntimeState(
			context.Background(),
			session.ID,
			applyAssistantStateUpdates(map[string]any{
				"status":                     string(StatusIdle),
				"updated_at":                 now,
				"auto_retry_attempt":         0,
				"auto_retry_next_at":         nil,
				"auto_retry_last_error_code": nil,
			}, AssistantStateNone, now),
		)
		m.cancelAutoRetryTimer(session.ID)
		m.broadcastSessionSummary(context.Background(), session.ID)
		return
	}

	if waitErr != nil {
		message := strings.TrimSpace(run.lastError)
		if message == "" {
			message = strings.TrimSpace(stderrBuffer.String())
		}
		if message == "" {
			message = waitErr.Error()
		}
		m.handleRunFailure(session.ID, session, run, errors.New(message))
		return
	}

	if run.assistantMessageID != "" && run.assistantDeltaWasSeen(run.assistantMessageID) {
		_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
			ID:        utils.NewID(),
			Seq:       0,
			Type:      "txt_end",
			RunID:     run.runID,
			ParentID:  run.assistantMessageID,
			Timestamp: time.Now(),
			Payload: map[string]any{
				"mid": run.assistantMessageID,
			},
		})
	}
	if normalizeAgent(Agent(session.Agent)) == AgentClaude {
		_ = m.finalizeLatestTurnUsage(context.Background(), session.ID)
	}
	finalStatus, finalAssistantState := m.completedRunState(context.Background(), session, run)
	now := time.Now()
	_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
		ID:        utils.NewID(),
		Seq:       0,
		Type:      "run_done",
		RunID:     run.runID,
		Timestamp: now,
		Payload: map[string]any{
			"ok": true,
			"st": string(finalStatus),
		},
	})
	_ = m.updateRuntimeState(
		context.Background(),
		session.ID,
		applyAssistantStateUpdates(map[string]any{
			"status":                     string(finalStatus),
			"updated_at":                 now,
			"auto_retry_attempt":         0,
			"auto_retry_next_at":         nil,
			"auto_retry_last_error_code": nil,
		}, finalAssistantState, now),
	)
	if run.deferredUserInput {
		if pending, ok := run.pendingUserInputRequest(); ok {
			m.deleteClaudeHookAnswerForSession(session, pending.ItemID)
		}
	}
	m.cancelAutoRetryTimer(session.ID)
	m.broadcastSessionSummary(context.Background(), session.ID)
	run.syncSourceAfterRun = true
}

func (m *Manager) logRunCompletion(
	ctx context.Context,
	run *activeRun,
	session tables.WebSessionTable,
	duration time.Duration,
) {
	if m == nil || m.logger == nil || run == nil {
		return
	}
	result := "success"
	errorCode := run.observationFailureCode()
	if ctx != nil && ctx.Err() != nil {
		result = "canceled"
		errorCode = ""
	} else if errorCode != "" {
		result = "failed"
	}
	fields := []zap.Field{
		zap.String("runId", run.runID),
		zap.String("sessionId", session.ID),
		zap.String("agent", string(normalizeAgent(Agent(session.Agent)))),
		zap.String("backend", string(run.backend)),
		zap.String("result", result),
		zap.String("errorCode", errorCode),
		zap.Duration("duration", duration),
	}
	if result == "failed" {
		m.logger.Warn("web session run completed", fields...)
		return
	}
	m.logger.Info("web session run completed", fields...)
}

func (m *Manager) runClaudeResumeSession(ctx context.Context, run *activeRun, session tables.WebSessionTable) {
	cmd, err := m.buildClaudeResumeCommand(ctx, session)
	if err != nil {
		m.handleRunFailure(session.ID, session, run, err)
		return
	}
	run.setCommand(cmd)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		m.handleRunFailure(session.ID, session, run, err)
		return
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		m.handleRunFailure(session.ID, session, run, err)
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		m.handleRunFailure(session.ID, session, run, err)
		return
	}

	if err := cmd.Start(); err != nil {
		m.handleRunFailure(session.ID, session, run, err)
		return
	}
	run.setInput(stdin)
	// A deferred Claude resume is the compatibility path used by older
	// versions that do not expose the stdio control protocol. It has no new
	// prompt to write, so close stdin immediately and let the resumed process
	// finish its one-shot turn.
	_ = stdin.Close()
	run.clearInput()

	stderrBuffer := bytes.NewBuffer(nil)
	stderrDone := make(chan struct{})
	go func() {
		defer close(stderrDone)
		m.consumeRuntimePlainOutput(ctx, session, run, io.TeeReader(stderr, stderrBuffer))
	}()

	m.consumeRuntimeOutput(ctx, session, run, stdout)

	waitErr := cmd.Wait()
	<-stderrDone
	if ctx.Err() != nil {
		now := time.Now()
		_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
			ID:        utils.NewID(),
			Seq:       0,
			Type:      "run_abort",
			RunID:     run.runID,
			Timestamp: now,
		})
		_ = m.updateRuntimeState(
			context.Background(),
			session.ID,
			applyAssistantStateUpdates(map[string]any{
				"status":                     string(StatusIdle),
				"updated_at":                 now,
				"auto_retry_attempt":         0,
				"auto_retry_next_at":         nil,
				"auto_retry_last_error_code": nil,
			}, AssistantStateNone, now),
		)
		m.cancelAutoRetryTimer(session.ID)
		m.broadcastSessionSummary(context.Background(), session.ID)
		return
	}
	if waitErr != nil {
		message := strings.TrimSpace(run.lastError)
		if message == "" {
			message = strings.TrimSpace(stderrBuffer.String())
		}
		if message == "" {
			message = waitErr.Error()
		}
		m.handleRunFailure(session.ID, session, run, errors.New(message))
		return
	}

	if run.assistantMessageID != "" && run.assistantDeltaWasSeen(run.assistantMessageID) {
		_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
			ID:        utils.NewID(),
			Seq:       0,
			Type:      "txt_end",
			RunID:     run.runID,
			ParentID:  run.assistantMessageID,
			Timestamp: time.Now(),
			Payload: map[string]any{
				"mid": run.assistantMessageID,
			},
		})
	}
	if normalizeAgent(Agent(session.Agent)) == AgentClaude {
		_ = m.finalizeLatestTurnUsage(context.Background(), session.ID)
	}
	finalStatus, finalAssistantState := m.completedRunState(context.Background(), session, run)
	now := time.Now()
	_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
		ID:        utils.NewID(),
		Seq:       0,
		Type:      "run_done",
		RunID:     run.runID,
		Timestamp: now,
		Payload: map[string]any{
			"ok": true,
			"st": string(finalStatus),
		},
	})
	_ = m.updateRuntimeState(
		context.Background(),
		session.ID,
		applyAssistantStateUpdates(map[string]any{
			"status":                     string(finalStatus),
			"updated_at":                 now,
			"auto_retry_attempt":         0,
			"auto_retry_next_at":         nil,
			"auto_retry_last_error_code": nil,
		}, finalAssistantState, now),
	)
	if run.deferredUserInput {
		if pending, ok := run.pendingUserInputRequest(); ok {
			m.deleteClaudeHookAnswerForSession(session, pending.ItemID)
		}
	}
	m.cancelAutoRetryTimer(session.ID)
	m.broadcastSessionSummary(context.Background(), session.ID)
	run.syncSourceAfterRun = true
}

func (m *Manager) handleRunFailure(sessionID string, session tables.WebSessionTable, run *activeRun, err error) {
	m.handleRunFailureWithCode(sessionID, session, run, "", err)
}

func (m *Manager) handleRunFailureWithCode(
	sessionID string,
	session tables.WebSessionTable,
	run *activeRun,
	code string,
	err error,
) {
	if run != nil {
		run.resetActiveCallTracking()
		if normalizeAgent(Agent(session.Agent)) == AgentPi {
			_ = m.closePendingPiDialog(session, run, "Pi extension input ended because the runtime failed")
		}
	}
	message := strings.TrimSpace(err.Error())
	if message == "" {
		message = "runtime failed"
	}
	run.lastError = message
	if strings.TrimSpace(code) == "" && run != nil {
		code = strings.TrimSpace(run.lastErrorCode)
	}
	normalizedCode := normalizeCodexErrorInfo(code)
	if normalizedCode == codexCyberPolicyErrorCode || isCodexCyberPolicyMessage(message) {
		code = codexCyberPolicyErrorCode
	} else if isCodexModelCapacityError(normalizedCode, message) {
		code = codexModelCapacityErrorCode
	} else if normalizedCode == "" {
		code = codexRuntimeErrorCode
	}
	run.setObservationFailure(code)
	now := time.Now()
	if normalizeAgent(Agent(session.Agent)) == AgentCodex {
		_ = m.finalizeLatestTurnUsage(context.Background(), sessionID)
	}
	_, _ = m.appendAndBroadcast(context.Background(), sessionID, session, Event{
		ID:        utils.NewID(),
		Seq:       0,
		Type:      "run_fail",
		RunID:     run.runID,
		Timestamp: now,
		Payload: map[string]any{
			"code": code,
			"msg":  message,
		},
	})
	updates := applyAssistantStateUpdates(map[string]any{
		"status":                     string(StatusError),
		"last_error":                 message,
		"auto_retry_last_error_code": nilIfEmpty(code),
		"updated_at":                 now,
	}, AssistantStateNone, now)
	if code == codexCyberPolicyErrorCode {
		updates["cyber_policy_flagged"] = true
	}
	_ = m.updateRuntimeState(
		context.Background(),
		sessionID,
		updates,
	)
	if err := m.reconcileAutoRetry(context.Background(), sessionID, now); err != nil && m.logger != nil {
		m.logger.Warn("auto retry reconciliation failed", zap.String("sessionId", sessionID), zap.Error(err))
	}
	m.broadcastSessionSummary(context.Background(), sessionID)
}

func (m *Manager) appendRunNote(
	sessionID string,
	session tables.WebSessionTable,
	run *activeRun,
	level string,
	message string,
	payload map[string]any,
) {
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		return
	}
	nextPayload := cloneMap(payload)
	if nextPayload == nil {
		nextPayload = map[string]any{}
	}
	nextPayload["txt"] = trimmed
	if strings.TrimSpace(level) != "" {
		nextPayload["lvl"] = strings.TrimSpace(level)
	}
	_, _ = m.appendAndBroadcast(context.Background(), sessionID, session, Event{
		ID:        utils.NewID(),
		Seq:       0,
		Type:      "note",
		RunID:     run.runID,
		ParentID:  run.assistantMessageIDSnapshot(),
		Timestamp: time.Now(),
		Payload:   nextPayload,
	})
}

func (m *Manager) consumeRuntimeOutput(ctx context.Context, session tables.WebSessionTable, run *activeRun, stdout io.Reader) {
	scanner := bufio.NewScanner(stdout)
	const maxLine = 1024 * 1024 * 8
	buffer := make([]byte, 64*1024)
	scanner.Buffer(buffer, maxLine)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var raw map[string]any
		if err := json.Unmarshal(line, &raw); err != nil {
			m.handleRuntimePlainLine(session, run, string(line))
			continue
		}
		switch run.agent {
		case AgentClaude:
			m.handleClaudeEvent(session, run, raw)
		case AgentCodex:
			m.handleCodexEvent(session, run, raw)
		}
	}
}

func (m *Manager) consumeRuntimePlainOutput(ctx context.Context, session tables.WebSessionTable, run *activeRun, reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	const maxLine = 1024 * 1024 * 2
	buffer := make([]byte, 64*1024)
	scanner.Buffer(buffer, maxLine)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}
		m.handleRuntimePlainLine(session, run, scanner.Text())
	}
}

func (m *Manager) handleRuntimePlainLine(session tables.WebSessionTable, run *activeRun, line string) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return
	}
	recent := run.pushRuntimeLine(trimmed)
	prompt, ok := detectApprovalPrompt(recent)
	if !ok {
		return
	}
	if !run.setPendingApproval(prompt) {
		return
	}
	now := time.Now()
	_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
		ID:        utils.NewID(),
		Seq:       0,
		Type:      "approval_req",
		RunID:     run.runID,
		ParentID:  run.assistantMessageID,
		Timestamp: now,
		Payload: map[string]any{
			"prompt": prompt,
		},
	})
	_ = m.updateRuntimeState(
		context.Background(),
		session.ID,
		applyAssistantStateUpdates(map[string]any{
			"updated_at": now,
		}, AssistantStateWaitingApproval, now),
	)
	m.broadcastSessionSummary(context.Background(), session.ID)
}

func (m *Manager) handleClaudeEvent(session tables.WebSessionTable, run *activeRun, raw map[string]any) {
	eventType, _ := raw["type"].(string)
	switch eventType {
	case "control_request":
		if request, ok := decodeClaudeControlRequest(raw); ok {
			m.handleClaudeControlRequest(session, run, request)
		}
	case "control_cancel_request":
		if requestID := claudeControlCancelID(raw); requestID != "" {
			m.handleClaudeControlCancel(session, run, requestID)
		}
	case "system":
		sessionID, _ := raw["session_id"].(string)
		subtype := strings.TrimSpace(stringValue(raw["subtype"]))
		if subtype == "init" {
			run.setClaudeResolvedModel(stringValue(raw["model"]))
		}
		if normalizeAgent(Agent(session.Agent)) == AgentClaude {
			m.handleClaudeCompactionStatus(session, run, raw)
		}
		updates := map[string]any{
			"updated_at": time.Now(),
		}
		if sessionID != "" {
			run.setClaudeNativeSessionID(sessionID)
			updates["native_session_id"] = sessionID
			updates["source_kind"] = sourceKindClaudeStreamJSON
			if threadPath, err := claudeSessionFilePath(session.Cwd, sessionID); err == nil {
				updates["thread_path"] = nilIfEmpty(threadPath)
			}
		}
		if len(updates) > 1 {
			_ = m.updateRuntimeState(context.Background(), session.ID, updates)
		}
		if subtype == "api_retry" {
			attempt := int(numberValue(raw["attempt"]))
			maxRetries := int(numberValue(raw["max_retries"]))
			retryDelayMs := int(numberValue(raw["retry_delay_ms"]))
			errorStatus := strings.TrimSpace(stringValue(raw["error_status"]))
			errorMessage := strings.TrimSpace(stringValue(raw["error"]))
			message := fmt.Sprintf(
				"Claude API retry %d/%d after %s (%s %s)",
				attempt,
				maxRetries,
				time.Duration(retryDelayMs)*time.Millisecond,
				errorStatus,
				errorMessage,
			)
			_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
				ID:        utils.NewID(),
				Seq:       0,
				Type:      "note",
				RunID:     run.runID,
				Timestamp: time.Now(),
				Payload: map[string]any{
					"code":         "transport_retrying",
					"attempt":      attempt,
					"maxAttempts":  maxRetries,
					"retryDelayMs": retryDelayMs,
					"errorStatus":  errorStatus,
					"error":        errorMessage,
					"lvl":          "warn",
					"txt":          strings.TrimSpace(message),
				},
			})
		}
	case "user":
		m.handleClaudeUserEvent(session, run, raw)
	case "assistant":
		if raw["isCompactSummary"] == true {
			// The system/status messages carry the authoritative compaction
			// boundary. This synthetic continuation summary is internal context.
			return
		}
		if raw["isMeta"] == true || raw["isSynthetic"] == true {
			return
		}
		message, _ := raw["message"].(map[string]any)
		content, _ := message["content"].([]any)
		stopReason := strings.TrimSpace(stringValue(message["stop_reason"]))
		assistantMessageID := firstNonEmpty(stringValue(raw["uuid"]), stringValue(message["id"]), utils.NewID())
		run.assistantMessageID = assistantMessageID
		_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
			ID:        utils.NewID(),
			Seq:       0,
			Type:      "msg_a_st",
			RunID:     run.runID,
			ParentID:  assistantMessageID,
			Timestamp: time.Now(),
			Payload: map[string]any{
				"mid": assistantMessageID,
			},
		})

		sawText := false
		for _, item := range content {
			block, ok := item.(map[string]any)
			if !ok {
				continue
			}
			blockType, _ := block["type"].(string)
			switch blockType {
			case "text":
				text, _ := block["text"].(string)
				if strings.TrimSpace(text) == "" {
					continue
				}
				sawText = true
				_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
					ID:        utils.NewID(),
					Seq:       0,
					Type:      "txt_d",
					RunID:     run.runID,
					ParentID:  assistantMessageID,
					Timestamp: time.Now(),
					Payload: map[string]any{
						"mid": assistantMessageID,
						"txt": text,
					},
				})
			case "tool_use":
				toolID, _ := block["id"].(string)
				if toolID == "" {
					toolID = utils.NewID()
				}
				toolName := strings.TrimSpace(stringValue(block["name"]))
				if toolName == "ExitPlanMode" {
					run.markCompletedPlanTool()
					input := decodeRawObject(block["input"])
					planText := strings.TrimSpace(stringValue(input["plan"]))
					planFilePath := strings.TrimSpace(stringValue(input["planFilePath"]))
					meta := map[string]any{
						"title": "Plan",
						"kind":  "plan",
					}
					if planFilePath != "" {
						meta["path"] = planFilePath
						meta["subtitle"] = planFilePath
					}
					_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
						ID:        utils.NewID(),
						Seq:       0,
						Type:      "tool_st",
						RunID:     run.runID,
						ParentID:  assistantMessageID,
						Timestamp: time.Now(),
						Payload: map[string]any{
							"tid":  toolID,
							"name": "Plan",
							"kind": "plan",
							"meta": meta,
						},
					})
					_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
						ID:        utils.NewID(),
						Seq:       0,
						Type:      "tool_end",
						RunID:     run.runID,
						ParentID:  assistantMessageID,
						Timestamp: time.Now(),
						Payload: map[string]any{
							"tid":  toolID,
							"name": "Plan",
							"kind": "plan",
							"out":  planText,
							"ok":   true,
							"meta": meta,
						},
					})
					continue
				}

				run.currentToolMessage = toolID
				if isInteractiveDynamicToolName(toolName) {
					// AskUserQuestion is rendered through the structured user-input
					// request below. Do not emit a duplicate generic tool card.
					continue
				}
				kind := claudeToolKind(toolName)
				input := claudeToolInput(toolName, block["input"])
				_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
					ID:        utils.NewID(),
					Seq:       0,
					Type:      "tool_st",
					RunID:     run.runID,
					ParentID:  assistantMessageID,
					Timestamp: time.Now(),
					Payload: map[string]any{
						"tid":  toolID,
						"name": claudeToolDisplayName(toolName, kind),
						"kind": kind,
						"in":   input,
						"meta": claudeToolMeta(toolName, kind, input),
					},
				})
			}
		}
		if sawText {
			_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
				ID:        utils.NewID(),
				Seq:       0,
				Type:      "txt_end",
				RunID:     run.runID,
				ParentID:  assistantMessageID,
				Timestamp: time.Now(),
				Payload: map[string]any{
					"mid": assistantMessageID,
				},
			})
		}
		if shouldCloseClaudeInput(stopReason, content) {
			run.closeInput()
		}
	case "result":
		defer run.closeInput()
		if sessionID, _ := raw["session_id"].(string); sessionID != "" {
			run.setClaudeNativeSessionID(sessionID)
			_ = m.updateRuntimeState(context.Background(), session.ID, map[string]any{
				"native_session_id": sessionID,
				"updated_at":        time.Now(),
			})
		}
		stopReason := strings.TrimSpace(stringValue(raw["stop_reason"]))
		if stopReason == "tool_deferred" {
			deferred := decodeRawObject(raw["deferred_tool_use"])
			switch strings.TrimSpace(stringValue(deferred["name"])) {
			case "AskUserQuestion":
				questions := decodeToolQuestions(decodeRawObject(deferred["input"])["questions"])
				request := &pendingServerRequest{
					Kind:      pendingServerRequestUserInput,
					ItemID:    strings.TrimSpace(stringValue(deferred["id"])),
					Prompt:    summarizeToolQuestions(questions),
					Questions: questions,
				}
				if request.ItemID != "" {
					run.deferredUserInput = true
					run.setPendingServerRequest(request)
					now := time.Now()
					_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
						ID:        utils.NewID(),
						Seq:       0,
						Type:      "user_input_req",
						RunID:     run.runID,
						ParentID:  run.assistantMessageID,
						Timestamp: now,
						Payload: map[string]any{
							"iid": request.ItemID,
							"txt": request.Prompt,
							"qs":  questions,
						},
					})
					_ = m.updateRuntimeState(
						context.Background(),
						session.ID,
						applyAssistantStateUpdates(map[string]any{
							"updated_at": now,
						}, AssistantStateWaitingInput, now),
					)
					m.broadcastSessionSummary(context.Background(), session.ID)
				}
			case "ExitPlanMode":
				request := &pendingServerRequest{
					Kind:   pendingServerRequestPlanApproval,
					ItemID: strings.TrimSpace(stringValue(deferred["id"])),
					Prompt: "Approve Claude's plan and exit plan mode?",
				}
				if request.ItemID != "" {
					run.setPendingServerRequest(request)
					now := time.Now()
					_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
						ID:        utils.NewID(),
						Seq:       0,
						Type:      "approval_req",
						RunID:     run.runID,
						ParentID:  run.assistantMessageID,
						Timestamp: now,
						Payload: map[string]any{
							"iid":    request.ItemID,
							"prompt": request.Prompt,
						},
					})
					_ = m.updateRuntimeState(
						context.Background(),
						session.ID,
						applyAssistantStateUpdates(map[string]any{
							"updated_at": now,
						}, AssistantStateWaitingPlanApproval, now),
					)
					m.broadcastSessionSummary(context.Background(), session.ID)
				}
			}
		}
		inputTokens, cachedInputTokens, outputTokens := claudeResultUsage(raw)
		contextWindowTokens := claudeResultContextWindow(raw, session, run.claudeResolvedModelSnapshot())
		totalCost, _ := raw["total_cost_usd"].(float64)
		if inputTokens > 0 || cachedInputTokens > 0 || outputTokens > 0 || totalCost > 0 || contextWindowTokens > 0 {
			updates := contextEstimateIncrementUpdate(inputTokens, cachedInputTokens, outputTokens)
			if contextWindowTokens > 0 {
				updates["session_context_window_tokens"] = contextWindowTokens
				updates["session_context_window_observed_at"] = time.Now()
			}
			_ = m.updateRuntimeState(context.Background(), session.ID, updates)
			eventPayload := map[string]any{
				"in":   inputTokens,
				"cin":  cachedInputTokens,
				"out":  outputTokens,
				"cost": totalCost,
			}
			if contextWindowTokens > 0 {
				eventPayload["cwt"] = contextWindowTokens
			}
			_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
				ID:        utils.NewID(),
				Seq:       0,
				Type:      "usage",
				RunID:     run.runID,
				Timestamp: time.Now(),
				Payload:   eventPayload,
			})
			if totalCost > 0 {
				_ = model.GetDB().WithContext(context.Background()).
					Model(&tables.WebSessionTable{}).
					Where("id = ?", session.ID).
					Updates(map[string]any{
						"total_cost": gorm.Expr("total_cost + ?", totalCost),
						"updated_at": time.Now(),
					}).Error
			}
		}
	case "error":
		run.lastError = stringValue(raw["message"])
	}
}

func (m *Manager) handleClaudeUserEvent(session tables.WebSessionTable, run *activeRun, raw map[string]any) {
	if raw["isCompactSummary"] == true {
		return
	}
	if raw["isMeta"] == true || raw["isSynthetic"] == true {
		return
	}
	message := decodeRawObject(raw["message"])
	if strings.TrimSpace(stringValue(message["role"])) != "user" {
		return
	}
	content, ok := message["content"].([]any)
	if !ok {
		return
	}
	for _, rawBlock := range content {
		block := decodeRawObject(rawBlock)
		if strings.TrimSpace(stringValue(block["type"])) != "tool_result" {
			continue
		}
		toolUseID := strings.TrimSpace(stringValue(block["tool_use_id"]))
		if toolUseID == "" {
			continue
		}
		if pending, ok := run.pendingUserInputRequest(); ok && strings.TrimSpace(pending.ItemID) == toolUseID {
			contentText := strings.TrimSpace(claudeToolResultContentText(block["content"]))
			if contentText == "" {
				contentText = claudeToolUseResultSummary(raw["toolUseResult"])
			}
			run.clearPendingServerRequest()
			payload := map[string]any{
				"iid": toolUseID,
			}
			if block["is_error"] == true {
				payload["err"] = firstNonEmpty(contentText, "User input request failed")
			}
			_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
				ID:        utils.NewID(),
				Seq:       0,
				Type:      "user_input_res",
				RunID:     run.runID,
				ParentID:  run.assistantMessageID,
				Timestamp: time.Now(),
				Payload:   payload,
			})
			_ = m.updateRuntimeState(
				context.Background(),
				session.ID,
				applyAssistantStateUpdates(map[string]any{
					"updated_at": time.Now(),
				}, AssistantStateWorking, time.Now()),
			)
			m.broadcastSessionSummary(context.Background(), session.ID)
			continue
		}

		contentText := strings.TrimSpace(claudeToolResultContentText(block["content"]))
		if contentText == "" {
			contentText = claudeToolUseResultSummary(raw["toolUseResult"])
		}
		payload := map[string]any{
			"tid": toolUseID,
			"out": truncateString(contentText, 4000),
			"ok":  block["is_error"] != true,
		}
		if existing, err := m.findHistoryItemByToolKey(context.Background(), session.ID, toolUseID); err == nil && existing.Tool != nil {
			payload["name"] = existing.Tool.Name
			payload["kind"] = existing.Tool.Kind
			payload["in"] = existing.Tool.Input
			payload["meta"] = existing.Tool.Meta
		}
		_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
			ID:        utils.NewID(),
			Seq:       0,
			Type:      "tool_end",
			RunID:     run.runID,
			ParentID:  run.assistantMessageID,
			Timestamp: time.Now(),
			Payload:   payload,
		})
	}
}

func (m *Manager) handleCodexEvent(session tables.WebSessionTable, run *activeRun, raw map[string]any) {
	eventType, _ := raw["type"].(string)
	switch eventType {
	case "thread.started":
		if threadID, _ := raw["thread_id"].(string); threadID != "" {
			_ = m.updateRuntimeState(context.Background(), session.ID, map[string]any{
				"native_session_id": threadID,
				"updated_at":        time.Now(),
			})
		}
	case "item.started":
		item, _ := raw["item"].(map[string]any)
		if stringValue(item["type"]) == "agent_message" {
			return
		}
		toolKind := normalizeCodexItemType(stringValue(item["type"]))
		toolName := codexToolName(item)
		toolInput := codexToolInput(item)
		toolMeta := codexToolMeta(item)
		toolID := stringValue(item["id"])
		if toolID == "" {
			toolID = utils.NewID()
		}
		_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
			ID:        utils.NewID(),
			Seq:       0,
			Type:      "tool_st",
			RunID:     run.runID,
			ParentID:  run.assistantMessageID,
			Timestamp: time.Now(),
			Payload: map[string]any{
				"tid":  toolID,
				"name": toolName,
				"kind": stringValue(item["type"]),
				"in":   toolInput,
				"meta": toolMeta,
			},
		})
		m.trackActiveCodexToolStart(run, toolID, toolKind, toolName, toolInput, toolMeta)
	case "item.completed":
		item, _ := raw["item"].(map[string]any)
		if stringValue(item["type"]) == "agent_message" {
			if run.assistantMessageID == "" {
				run.assistantMessageID = utils.NewID()
				_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
					ID:        utils.NewID(),
					Seq:       0,
					Type:      "msg_a_st",
					RunID:     run.runID,
					ParentID:  run.assistantMessageID,
					Timestamp: time.Now(),
					Payload: map[string]any{
						"mid": run.assistantMessageID,
					},
				})
			}
			text := stringValue(item["text"])
			if strings.TrimSpace(text) != "" {
				_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
					ID:        utils.NewID(),
					Seq:       0,
					Type:      "txt_d",
					RunID:     run.runID,
					ParentID:  run.assistantMessageID,
					Timestamp: time.Now(),
					Payload: map[string]any{
						"mid": run.assistantMessageID,
						"txt": text,
					},
				})
			}
			return
		}
		toolID := stringValue(item["id"])
		if toolID == "" {
			toolID = utils.NewID()
		}
		toolSucceeded := codexToolSucceeded(item)
		if toolSucceeded && codexToolIsPlan(item) {
			run.markCompletedPlanTool()
		}
		if toolSucceeded && normalizeCodexItemType(stringValue(item["type"])) == "context_compaction" {
			record, err := m.GetSession(context.Background(), session.ID)
			if err == nil {
				_ = m.updateRuntimeState(
					context.Background(),
					session.ID,
					contextEstimateBaselineResetUpdate(record, time.Now()),
				)
			}
		}
		_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
			ID:        utils.NewID(),
			Seq:       0,
			Type:      "tool_end",
			RunID:     run.runID,
			ParentID:  run.assistantMessageID,
			Timestamp: time.Now(),
			Payload: map[string]any{
				"tid":  toolID,
				"kind": normalizeCodexItemType(stringValue(item["type"])),
				"out":  codexToolOutput(item),
				"ok":   toolSucceeded,
				"meta": codexToolMeta(item),
			},
		})
		m.trackActiveCodexToolComplete(run, toolID)
	case "turn.completed":
		usage, _ := raw["usage"].(map[string]any)
		in := int64(numberValue(usage["input_tokens"]))
		cin := int64(numberValue(usage["cached_input_tokens"]))
		out := int64(numberValue(usage["output_tokens"]))
		_ = m.updateRuntimeState(context.Background(), session.ID, contextEstimateIncrementUpdate(in, cin, out))
		_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
			ID:        utils.NewID(),
			Seq:       0,
			Type:      "usage",
			RunID:     run.runID,
			Timestamp: time.Now(),
			Payload: map[string]any{
				"in":  in,
				"cin": cin,
				"out": out,
			},
		})
	case "turn.failed":
		errorMap, _ := raw["error"].(map[string]any)
		run.lastError = stringValue(errorMap["message"])
		run.lastErrorCode = codexErrorInfo(errorMap)
	case "error":
		run.lastError = stringValue(raw["message"])
		run.lastErrorCode = codexErrorInfo(raw)
	}
}

func (m *Manager) appendAndBroadcast(ctx context.Context, sessionID string, record tables.WebSessionTable, event Event) (Event, error) {
	eventState := m.sessionEventState(sessionID)
	eventState.mu.Lock()
	defer eventState.mu.Unlock()
	if eventState.closed {
		return Event{}, fmt.Errorf("web session %s is being deleted", sessionID)
	}
	if err := m.flushPendingTextDeltaLocked(ctx, sessionID, eventState); err != nil {
		return Event{}, err
	}
	return m.appendAndBroadcastNow(ctx, sessionID, record, eventState, event)
}

func (m *Manager) appendAndBroadcastNow(
	ctx context.Context,
	sessionID string,
	record tables.WebSessionTable,
	eventState *sessionEventState,
	event Event,
) (Event, error) {
	seq, err := m.nextEventSeq(ctx, sessionID, eventState)
	if err != nil {
		return Event{}, err
	}
	event.Seq = seq
	if event.ID == "" {
		event.ID = utils.NewID()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	m.decorateProjectedEvent(sessionID, &event)
	if err := m.store.appendEvent(sessionID, event); err != nil {
		return Event{}, err
	}

	retry := eventProjectionRetry{
		record: record,
		event:  event,
		stage:  eventProjectionDatabase,
	}
	if err := m.flushEventProjectionRetriesLocked(ctx, sessionID, eventState); err != nil {
		m.queueEventProjectionRetryLocked(sessionID, eventState, retry)
		if m.logger != nil {
			m.logger.Warn("web session event persisted; database projection remains queued",
				zap.String("sessionId", sessionID),
				zap.String("eventId", event.ID),
				zap.Error(err),
			)
		}
		return event, nil
	}
	if err := m.projectPersistedEvent(ctx, sessionID, &retry); err != nil {
		retry.recordProjectionFailure(err)
		m.queueEventProjectionRetryLocked(sessionID, eventState, retry)
		if m.logger != nil {
			m.logger.Warn("web session event persisted; queued database projection retry",
				zap.String("sessionId", sessionID),
				zap.String("eventId", event.ID),
				zap.Error(err),
			)
		}
		return event, nil
	}
	return event, nil
}

func (m *Manager) projectPersistedEvent(
	ctx context.Context,
	sessionID string,
	retry *eventProjectionRetry,
) error {
	if retry == nil {
		return fmt.Errorf("web session event projection is nil")
	}
	if retry.stage <= eventProjectionDatabase {
		if err := m.projectPersistedEventDatabase(ctx, sessionID, retry); err != nil {
			return err
		}
	}
	if retry.stage <= eventProjectionBroadcast {
		if retry.subAgent != nil {
			m.broadcast(newSubAgentFrame(sessionID, *retry.subAgent, m.summaryForBroadcast(ctx, sessionID)))
		}
		items := make([]HistoryItem, 0, 2)
		if retry.cachedItem != nil {
			items = append(items, *retry.cachedItem)
		}
		if retry.timingItem != nil && (retry.cachedItem == nil || retry.cachedItem.ID != retry.timingItem.ID) {
			items = append(items, *retry.timingItem)
		}
		sort.SliceStable(items, func(left, right int) bool {
			if items[left].LastEventSeq == items[right].LastEventSeq {
				return items[left].OrderIndex < items[right].OrderIndex
			}
			return items[left].LastEventSeq < items[right].LastEventSeq
		})
		for _, item := range items {
			m.broadcast(newHistoryItemFrame(sessionID, item, m.summaryForBroadcast(ctx, sessionID)))
		}
		if retry.event.Type == "tool_end" {
			m.maybeInterruptForRedirect(sessionID)
		}
		retry.stage = eventProjectionDone
	}
	return nil
}

func (m *Manager) projectPersistedEventDatabase(
	ctx context.Context,
	sessionID string,
	retry *eventProjectionRetry,
) error {
	if retry == nil {
		return fmt.Errorf("web session event projection is nil")
	}
	db := model.GetDB()
	if db == nil {
		return model.ErrDBNotInitialized
	}
	working := *retry
	working.cachedItem = nil
	working.timingItem = nil
	working.subAgent = nil
	err := m.observedTransaction(ctx, db, "project_persisted_event", func(tx *gorm.DB) error {
		event := working.event
		if event.Type == "sub_agent_state" {
			agent, changed, err := m.applySubAgentStateEventDB(ctx, tx, sessionID, event)
			if err != nil {
				return err
			}
			if changed {
				working.subAgent = &agent
			}
		} else {
			cachedItem, err := m.applyEventToHistoryCacheDB(ctx, tx, sessionID, event)
			if err != nil {
				return err
			}
			working.cachedItem = cachedItem
			if cachedItem != nil {
				subAgent, changed, err := m.applySubAgentHistoryItemDB(ctx, tx, working.record, event, *cachedItem)
				if err != nil {
					return err
				}
				if changed {
					working.subAgent = &subAgent
				}
			}
		}
		timingUpdates, timingItem, err := m.applyWorkTimingEventDB(ctx, tx, sessionID, event)
		if err != nil {
			return err
		}
		working.timingItem = timingItem
		if working.timingItem != nil {
			working.timingItem.LastEventSeq = event.Seq
			if err := tx.Model(&tables.WebSessionItemTable{}).
				Where("id = ?", working.timingItem.ID).
				UpdateColumn("last_event_seq", event.Seq).Error; err != nil {
				return err
			}
		}
		if timingItem != nil && working.cachedItem != nil && timingItem.ID == working.cachedItem.ID {
			working.cachedItem = timingItem
			working.timingItem = nil
		}
		runtimeUpdates := mergeRuntimeUpdates(m.runtimeUpdatesForEvent(sessionID, event), timingUpdates)
		if err := m.updateRuntimeStateDB(ctx, tx, sessionID, runtimeUpdates); err != nil {
			return err
		}
		working.stage = eventProjectionBroadcast
		return nil
	},
		zap.String("sessionId", sessionID),
		zap.String("eventType", working.event.Type),
		zap.Int64("eventSequence", working.event.Seq),
	)
	if err != nil {
		return err
	}
	*retry = working
	return nil
}

func (m *Manager) runtimeUpdatesForEvent(sessionID string, event Event) map[string]any {
	update := map[string]any{
		"last_event_seq": event.Seq,
		"activity_at":    event.Timestamp,
	}
	if shouldMarkSessionUnreadForEvent(event) && !m.hasFocusedEventClient(sessionID) {
		update["has_unread"] = true
		update["attention_revision"] = gorm.Expr("attention_revision + 1")
	}
	if event.Type == "msg_u" {
		update["last_message_at"] = event.Timestamp
	}
	return update
}

func (m *Manager) sessionAgent(sessionID string) Agent {
	return m.sessionAgentDB(context.Background(), model.GetDB(), sessionID)
}

func (m *Manager) sessionAgentDB(ctx context.Context, db *gorm.DB, sessionID string) Agent {
	m.mu.RLock()
	run := m.runs[sessionID]
	m.mu.RUnlock()
	if run != nil {
		run.mu.Lock()
		agent := run.agent
		run.mu.Unlock()
		if agent != "" {
			return normalizeAgent(agent)
		}
	}

	if db == nil {
		return AgentClaude
	}
	var record tables.WebSessionTable
	if err := db.WithContext(ctx).
		Select("id", "agent").
		First(&record, "id = ?", sessionID).Error; err != nil {
		return AgentClaude
	}
	return normalizeAgent(Agent(record.Agent))
}

func shouldMarkSessionUnreadForEvent(event Event) bool {
	switch strings.TrimSpace(event.Type) {
	case "approval_req", "user_input_req", "run_fail", "run_done":
		return true
	case "run_abort":
		return isUnexpectedRunAbortEvent(event)
	default:
		return false
	}
}

func isUnexpectedRunAbortEvent(event Event) bool {
	reason := strings.TrimSpace(stringValue(event.Payload["reason"]))
	msg := strings.TrimSpace(stringValue(event.Payload["msg"]))
	prevStatus := strings.TrimSpace(stringValue(event.Payload["prevStatus"]))
	return reason != "" || msg != "" || prevStatus != ""
}

func (m *Manager) hasFocusedEventClient(sessionID string) bool {
	normalizedSessionID := strings.TrimSpace(sessionID)
	if normalizedSessionID == "" {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for client := range m.clients {
		if client == nil || client.kind != clientKindEvent {
			continue
		}
		if client.FocusedSessionID() == normalizedSessionID {
			return true
		}
	}
	return false
}

func (m *Manager) decorateProjectedEvent(sessionID string, event *Event) {
	if event == nil {
		return
	}
	if isCompactToolEvent(*event) {
		m.decorateCompactToolGroupEvent(sessionID, event)
		return
	}
	if isReasoningToolEvent(*event) {
		if reasoningEventHasDisplayContent(*event) && m.sessionAgent(sessionID) != AgentCodex {
			m.resetCommandExecutionGroup(sessionID)
		}
		return
	}
	if isCommandGroupTransparentEvent(*event) {
		return
	}
	m.resetCommandExecutionGroup(sessionID)
}

func (m *Manager) decorateCompactToolGroupEvent(sessionID string, event *Event) {
	toolID := eventToolID(*event)
	if toolID == "" {
		toolID = event.ID
	}
	kind := compactToolKind(*event)

	groupID := commandExecutionGroupID(toolID)
	firstSeq := event.Seq
	count := 1

	m.mu.RLock()
	run := m.runs[sessionID]
	m.mu.RUnlock()

	if run != nil {
		run.mu.Lock()
		groupKey := compactToolGroupKey(*event)
		if run.commandGroupKey != "" && run.commandGroupKey != groupKey {
			run.commandGroupID = ""
			run.commandGroupKind = ""
			run.commandGroupKey = ""
			run.commandGroupFirst = 0
			run.commandGroupCount = 0
			run.commandGroupTools = nil
		}
		if run.commandGroupTools == nil {
			run.commandGroupTools = make(map[string]struct{})
		}
		if run.commandGroupID == "" {
			run.commandGroupID = groupID
		}
		if run.commandGroupKind == "" {
			run.commandGroupKind = kind
		}
		if run.commandGroupKey == "" {
			run.commandGroupKey = groupKey
		}
		groupID = run.commandGroupID
		if run.commandGroupFirst == 0 {
			run.commandGroupFirst = event.Seq
		}
		firstSeq = run.commandGroupFirst
		if _, exists := run.commandGroupTools[toolID]; !exists {
			run.commandGroupTools[toolID] = struct{}{}
			run.commandGroupCount += 1
		}
		if run.commandGroupCount > 0 {
			count = run.commandGroupCount
		}
		run.mu.Unlock()
	}

	meta := eventToolMeta(*event)
	if meta == nil {
		meta = make(map[string]any)
	}
	meta["kind"] = kind
	meta["title"] = firstNonEmpty(stringValue(meta["title"]), eventToolName(*event), compactToolTitle(kind))
	meta["subtitle"] = compactToolSummary(kind, eventToolInput(*event), meta, eventToolOutput(*event))
	meta["commandGroup"] = map[string]any{
		"id":           groupID,
		"count":        count,
		"firstSeq":     firstSeq,
		"lastSeq":      event.Seq,
		"latestToolId": toolID,
		"compacted":    true,
	}
	if event.Payload == nil {
		event.Payload = make(map[string]any)
	}
	event.Payload["meta"] = meta
}

func (m *Manager) resetCommandExecutionGroup(sessionID string) {
	m.mu.RLock()
	run := m.runs[sessionID]
	m.mu.RUnlock()
	if run == nil {
		return
	}
	run.mu.Lock()
	run.commandGroupID = ""
	run.commandGroupKind = ""
	run.commandGroupKey = ""
	run.commandGroupFirst = 0
	run.commandGroupCount = 0
	run.commandGroupTools = nil
	run.mu.Unlock()
}

func (m *Manager) nextEventSeq(
	ctx context.Context,
	sessionID string,
	eventState *sessionEventState,
) (int64, error) {
	if eventState == nil {
		return 0, fmt.Errorf("web session event state is nil")
	}
	if !eventState.seqInitialized {
		var record tables.WebSessionTable
		if err := model.GetDB().WithContext(ctx).Select("id", "last_event_seq").First(&record, "id = ?", sessionID).Error; err != nil {
			return 0, err
		}
		eventState.lastSeq = record.LastEventSeq
		durableSeq, err := m.store.latestEventSeq(sessionID)
		if err != nil {
			return 0, fmt.Errorf("read durable web session event sequence: %w", err)
		}
		if durableSeq > eventState.lastSeq {
			eventState.lastSeq = durableSeq
		}
		eventState.seqInitialized = true
	}
	eventState.lastSeq++
	return eventState.lastSeq, nil
}

func (m *Manager) updateRuntimeState(ctx context.Context, sessionID string, updates map[string]any) error {
	return m.updateRuntimeStateDB(ctx, model.GetDB(), sessionID, updates)
}

func (m *Manager) updateRuntimeStateDB(ctx context.Context, db *gorm.DB, sessionID string, updates map[string]any) error {
	if len(updates) == 0 {
		return nil
	}
	if db == nil {
		return model.ErrDBNotInitialized
	}
	result := db.WithContext(ctx).Model(&tables.WebSessionTable{}).
		Where("id = ?", sessionID).
		Updates(withSnapshotRevisionIncrement(updates))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (m *Manager) completedRunState(ctx context.Context, session tables.WebSessionTable, run *activeRun) (Status, AssistantState) {
	current := session
	record, err := m.GetSession(ctx, session.ID)
	if err == nil {
		current = record
	}
	if effectiveWorkflowMode(current) == WorkflowModePlan && run.completedPlanToolSeen() {
		return StatusRunning, AssistantStateWaitingPlanApproval
	}
	if run.deferredUserInput && !run.claudeResumeOnly {
		return StatusDone, AssistantStateWaitingInput
	}
	return StatusDone, AssistantStateNone
}

func (m *Manager) updateFields(ctx context.Context, sessionID string, updates map[string]any) (SessionSummary, error) {
	if err := model.GetDB().WithContext(ctx).Model(&tables.WebSessionTable{}).
		Where("id = ?", sessionID).
		Updates(withSnapshotRevisionIncrement(updates)).Error; err != nil {
		return SessionSummary{}, err
	}
	record, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return SessionSummary{}, err
	}
	return m.mapSessionSummary(record), nil
}

func (m *Manager) getNextSessionOrderIndex(ctx context.Context, projectID string) (float64, error) {
	db := model.GetDB()
	if db == nil {
		return 0, model.ErrDBNotInitialized
	}

	var maxOrder float64
	if err := db.WithContext(ctx).
		Model(&tables.WebSessionTable{}).
		Where("project_id = ? AND archived_at IS NULL", projectID).
		Select("COALESCE(MAX(order_index), 0)").
		Scan(&maxOrder).Error; err != nil {
		return 0, err
	}
	return maxOrder + sessionOrderStep, nil
}

func (m *Manager) listSessionRecordsWithDB(db *gorm.DB, projectID string) ([]tables.WebSessionTable, error) {
	query := db.Model(&tables.WebSessionTable{}).
		Where("archived_at IS NULL").
		Order("order_index ASC").
		Order("updated_at DESC")
	if projectID != "" {
		query = query.Where("project_id = ?", projectID)
	}
	var records []tables.WebSessionTable
	if err := query.Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

func (m *Manager) backfillSessionActivityAt(ctx context.Context) error {
	db := model.GetDB()
	if db == nil {
		return model.ErrDBNotInitialized
	}

	var records []tables.WebSessionTable
	if err := db.WithContext(ctx).
		Select("id", "created_at", "updated_at", "last_message_at", "activity_at").
		Find(&records).Error; err != nil {
		return err
	}

	for _, record := range records {
		if !record.ActivityAt.IsZero() {
			continue
		}
		activityAt := chooseSessionActivityAt(record)
		if err := db.WithContext(ctx).
			Model(&tables.WebSessionTable{}).
			Where("id = ?", record.ID).
			Updates(map[string]any{
				"activity_at": activityAt,
				"updated_at":  time.Now(),
			}).Error; err != nil {
			return err
		}
	}
	return nil
}

func resolveSessionInsertIndex(
	sessions []tables.WebSessionTable,
	sessionID string,
	prevSessionID string,
	nextSessionID string,
) (int, error) {
	prevSessionID = strings.TrimSpace(prevSessionID)
	nextSessionID = strings.TrimSpace(nextSessionID)
	if prevSessionID != "" && prevSessionID == nextSessionID {
		return 0, fmt.Errorf("invalid move target")
	}
	if prevSessionID == sessionID || nextSessionID == sessionID {
		return 0, fmt.Errorf("cannot move relative to itself")
	}

	findIndex := func(targetID string) int {
		for index, item := range sessions {
			if item.ID == targetID {
				return index
			}
		}
		return -1
	}

	if nextSessionID != "" {
		nextIndex := findIndex(nextSessionID)
		if nextIndex == -1 {
			return 0, fmt.Errorf("target session not found")
		}
		if prevSessionID != "" {
			prevIndex := findIndex(prevSessionID)
			if prevIndex == -1 {
				return 0, fmt.Errorf("target session not found")
			}
			if prevIndex >= nextIndex {
				return 0, fmt.Errorf("invalid move target")
			}
		}
		return nextIndex, nil
	}

	if prevSessionID != "" {
		prevIndex := findIndex(prevSessionID)
		if prevIndex == -1 {
			return 0, fmt.Errorf("target session not found")
		}
		return prevIndex + 1, nil
	}

	return 0, nil
}

func (m *Manager) resolveContext(ctx context.Context, projectID, worktreeID string) (*model.Project, string, string, error) {
	project, err := m.projectSvc.GetProject(ctx, projectID)
	if err != nil {
		return nil, "", "", err
	}

	if strings.TrimSpace(worktreeID) != "" {
		worktree, err := m.worktreeSvc.GetWorktree(ctx, worktreeID)
		if err != nil {
			return nil, "", "", err
		}
		if worktree.ProjectId != project.Id {
			return nil, "", "", fmt.Errorf("worktree does not belong to project")
		}
		return project, worktree.Id, worktree.Path, nil
	}

	worktrees, err := m.worktreeSvc.ListWorktrees(ctx, project.Id)
	if err == nil {
		for _, worktree := range worktrees {
			if worktree.IsMain {
				return project, worktree.Id, worktree.Path, nil
			}
		}
		if len(worktrees) > 0 {
			return project, worktrees[0].Id, worktrees[0].Path, nil
		}
	}
	return project, "", project.Path, nil
}

func (m *Manager) buildExecCommand(ctx context.Context, session tables.WebSessionTable, text string, attachments []Attachment) (*exec.Cmd, []byte, bool, error) {
	workflowMode := effectiveWorkflowMode(session)
	permissionLevel := effectivePermissionLevel(session)
	preparedText := preparePromptText(text, workflowMode)

	switch normalizeAgent(Agent(session.Agent)) {
	case AgentClaude:
		args := []string{
			"-p",
			"--output-format", "stream-json",
			"--input-format", "stream-json",
			"--permission-prompt-tool", "stdio",
			"--autocompact", "auto",
			"--replay-user-messages",
			"--verbose",
		}
		claudeRuntime := effectiveClaudeRuntime(session)
		if claudeRuntime == ClaudeRuntimeCCR {
			if err := m.ensureCCRClaudeHookSettings(); err != nil {
				return nil, nil, false, err
			}
		} else {
			settingsPath, err := m.ensureClaudeHookServer()
			if err != nil {
				return nil, nil, false, err
			}
			args = append(args, "--settings", settingsPath)
		}
		if err := validateWebSessionPermissionLevel(AgentClaude, permissionLevel); err != nil {
			return nil, nil, false, err
		}
		switch normalizeWorkflowMode(workflowMode) {
		case WorkflowModePlan:
			args = append(args, "--permission-mode", "plan")
		default:
			switch permissionLevel {
			case PermissionLevelYolo:
				args = append(args, "--dangerously-skip-permissions")
			case PermissionLevelElevated:
				args = append(args, "--permission-mode", "acceptEdits")
			}
		}
		if session.NativeSessionID != nil && strings.TrimSpace(*session.NativeSessionID) != "" {
			args = append(args, "--resume", strings.TrimSpace(*session.NativeSessionID))
		}
		if strings.TrimSpace(session.Model) != "" {
			args = append(args, "--model", strings.TrimSpace(session.Model))
		}
		if effort := claudeReasoningEffortArg(ReasoningEffort(session.ReasoningEffort)); effort != "" {
			args = append(args, "--effort", effort)
		}
		stdin, err := claudeUserMessagePayload(text, attachments, workflowMode)
		if err != nil {
			return nil, nil, false, err
		}
		cmd := m.buildClaudeCommand(ctx, claudeRuntime, args)
		cmd.Dir = session.Cwd
		cmd.Env = m.claudeCommandEnv(claudeRuntime)
		return cmd, stdin, true, nil
	case AgentCodex:
		args := []string{"exec", "--json", "--skip-git-repo-check"}
		trimmedText := strings.TrimSpace(preparedText)
		useStdinPrompt := trimmedText == ""
		switch permissionLevel {
		case PermissionLevelYolo:
			args = append(args, "--dangerously-bypass-approvals-and-sandbox")
		case PermissionLevelElevated:
			args = append(args, "-s", "danger-full-access", "-c", `approval_policy="on-request"`)
		default:
			args = append(args, "-s", "workspace-write", "-c", `approval_policy="on-request"`)
		}
		if strings.TrimSpace(session.Model) != "" {
			args = append(args, "--model", strings.TrimSpace(session.Model))
		}
		if effort := normalizeCodexReasoningEffort(session.Model, ReasoningEffort(session.ReasoningEffort)); effort != ReasoningEffortDefault {
			args = append(args, "-c", fmt.Sprintf("model_reasoning_effort=%q", string(effort)))
		}
		for _, attachment := range attachments {
			args = append(args, "--image", attachment.Path)
		}
		if session.NativeSessionID != nil && strings.TrimSpace(*session.NativeSessionID) != "" {
			args = append(args, "resume")
			args = append(args, strings.TrimSpace(*session.NativeSessionID))
			if useStdinPrompt {
				args = append(args, "-")
			} else {
				args = append(args, trimmedText)
			}
		} else {
			if session.Cwd != "" {
				args = append(args, "-C", session.Cwd)
			}
			if useStdinPrompt {
				args = append(args, "-")
			} else {
				args = append(args, trimmedText)
			}
		}
		cmd := exec.CommandContext(ctx, m.cfg.CodexPath, args...)
		cmd.Dir = session.Cwd
		cmd.Env = os.Environ()
		if useStdinPrompt {
			return cmd, []byte(preparedText), true, nil
		}
		// Codex appends any piped stdin as an extra <stdin> block even when a prompt
		// argument is provided, so we must close stdin immediately for normal prompt runs.
		return cmd, nil, true, nil
	default:
		return nil, nil, false, fmt.Errorf("unsupported agent %q", session.Agent)
	}
}

func (m *Manager) buildClaudeCommand(ctx context.Context, runtime ClaudeRuntime, args []string) *exec.Cmd {
	if normalizeClaudeRuntime(runtime) == ClaudeRuntimeCCR {
		ccrArgs := append([]string{"code"}, args...)
		return exec.CommandContext(ctx, m.cfg.CCRPath, ccrArgs...)
	}
	return exec.CommandContext(ctx, m.cfg.ClaudePath, args...)
}

func isClaudeControlCommand(cmd *exec.Cmd) bool {
	if cmd == nil || len(cmd.Args) == 0 {
		return false
	}
	hasPromptTool := false
	for index, arg := range cmd.Args {
		if arg == "--permission-prompt-tool" && index+1 < len(cmd.Args) && cmd.Args[index+1] == "stdio" {
			hasPromptTool = true
			break
		}
	}
	return hasPromptTool
}

func (m *Manager) claudeCommandEnv(runtime ClaudeRuntime) []string {
	env := os.Environ()
	if normalizeClaudeRuntime(runtime) != ClaudeRuntimeCCR || strings.TrimSpace(m.ccrHookClaudePath) == "" {
		return env
	}
	return upsertEnv(env, "CLAUDE_PATH", m.ccrHookClaudePath)
}

func upsertEnv(env []string, key, value string) []string {
	prefix := key + "="
	for i, item := range env {
		if strings.HasPrefix(item, prefix) {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}

func (m *Manager) respondToApproval(sessionID, action string) error {
	dispatchLock := &m.sessionDispatchLocks[sessionRevisionLockIndex(sessionID)]
	dispatchLock.Lock()

	m.mu.RLock()
	run, ok := m.runs[sessionID]
	m.mu.RUnlock()
	record, err := m.GetSession(context.Background(), sessionID)
	if err != nil {
		dispatchLock.Unlock()
		return err
	}
	if normalizeAgent(Agent(record.Agent)) == AgentPi {
		if !ok || run == nil {
			dispatchLock.Unlock()
			return fmt.Errorf("no pending Pi approval")
		}
		pending, hasPending := run.pendingApprovalRequest()
		if !hasPending || pending.PiRuntime == nil {
			dispatchLock.Unlock()
			return fmt.Errorf("no pending Pi approval")
		}
		request, taken := run.takePendingPiRequestForResponse(pending.PiRequestID, true)
		if !taken {
			dispatchLock.Unlock()
			return fmt.Errorf("Pi approval is no longer pending")
		}
		dispatchLock.Unlock()
		confirmed := action != "reject" && action != "cancel"
		return m.respondPiExtensionRequest(record, run, request, map[string]any{"confirmed": confirmed}, "approval_res", map[string]any{
			"act": action, "prompt": request.Prompt,
		})
	}
	defer dispatchLock.Unlock()
	if normalizeAgent(Agent(record.Agent)) == AgentClaude {
		var pending *pendingServerRequest
		if ok && run != nil {
			if request, hasRequest := run.pendingApprovalRequest(); hasRequest {
				pending = request
			}
		}
		if pending == nil {
			var err error
			pending, err = m.findClaudePendingApprovalRequest(context.Background(), sessionID)
			if err != nil {
				return err
			}
		}
		if pending != nil && strings.TrimSpace(pending.ControlRequestID) != "" && run != nil {
			behavior := "deny"
			if action != "reject" {
				behavior = "allow"
			}
			if err := m.respondClaudeControl(record, run, pending, behavior, pending.Input, "The user declined this tool use.", nil); err != nil {
				return err
			}
			return nil
		}
		decision := "deny"
		if action != "reject" {
			decision = "allow"
		}
		if err := m.writeClaudeHookAnswerForSession(record, pending.ItemID, claudeHookAnswerFile{
			PermissionDecision: decision,
		}); err != nil {
			return err
		}
		now := time.Now()
		_, _ = m.appendAndBroadcast(context.Background(), sessionID, record, Event{
			ID:        utils.NewID(),
			Seq:       0,
			Type:      "approval_res",
			RunID:     utils.NewID(),
			Timestamp: now,
			Payload: map[string]any{
				"act":    action,
				"prompt": pending.Prompt,
			},
		})
		if err := m.startClaudeDeferredResume(context.Background(), record, pending); err != nil {
			return err
		}
		return nil
	}
	if !ok {
		return fmt.Errorf("session is not running")
	}

	if pending, ok := run.pendingApprovalRequest(); ok {
		app := run.codexAppServer()
		if app == nil {
			return fmt.Errorf("session approval channel is unavailable")
		}
		if err := app.respond(pending.RawID, approvalResponsePayload(pending, action)); err != nil {
			return err
		}
		run.clearPendingServerRequest()
		m.resumeActiveCallTimeout(run)
		record, err = m.GetSession(context.Background(), sessionID)
		if err != nil {
			return err
		}
		now := time.Now()
		_, _ = m.appendAndBroadcast(context.Background(), sessionID, record, Event{
			ID:        utils.NewID(),
			Seq:       0,
			Type:      "approval_res",
			RunID:     run.runID,
			ParentID:  run.assistantMessageID,
			ThreadID:  pending.ThreadID,
			TurnID:    pending.TurnID,
			Timestamp: now,
			Payload: map[string]any{
				"act":    action,
				"prompt": pending.Prompt,
			},
		})
		_ = m.updateRuntimeState(
			context.Background(),
			sessionID,
			applyAssistantStateUpdates(map[string]any{
				"updated_at": now,
			}, AssistantStateWorking, now),
		)
		m.broadcastSessionSummary(context.Background(), sessionID)
		return nil
	}

	prompt, ok := run.pendingApprovalPrompt()
	if !ok {
		return fmt.Errorf("no pending approval")
	}
	if err := run.writeInput(approvalInput(action)); err != nil {
		return err
	}
	run.clearPendingApproval()
	record, err = m.GetSession(context.Background(), sessionID)
	if err != nil {
		return err
	}
	now := time.Now()
	_, _ = m.appendAndBroadcast(context.Background(), sessionID, record, Event{
		ID:        utils.NewID(),
		Seq:       0,
		Type:      "approval_res",
		RunID:     run.runID,
		ParentID:  run.assistantMessageID,
		Timestamp: now,
		Payload: map[string]any{
			"act":    action,
			"prompt": prompt,
		},
	})
	_ = m.updateRuntimeState(
		context.Background(),
		sessionID,
		applyAssistantStateUpdates(map[string]any{
			"updated_at": now,
		}, AssistantStateWorking, now),
	)
	m.broadcastSessionSummary(context.Background(), sessionID)
	return nil
}

func (m *Manager) respondToUserInput(sessionID, itemID string, answers map[string][]string) error {
	dispatchLock := &m.sessionDispatchLocks[sessionRevisionLockIndex(sessionID)]
	dispatchLock.Lock()

	m.mu.RLock()
	run, ok := m.runs[sessionID]
	m.mu.RUnlock()
	record, err := m.GetSession(context.Background(), sessionID)
	if err != nil {
		dispatchLock.Unlock()
		return err
	}
	if normalizeAgent(Agent(record.Agent)) == AgentPi {
		if !ok || run == nil {
			dispatchLock.Unlock()
			return fmt.Errorf("no pending Pi user input request")
		}
		pending, hasPending := run.pendingUserInputRequest()
		if !hasPending || pending.PiRuntime == nil {
			dispatchLock.Unlock()
			return fmt.Errorf("no pending Pi user input request")
		}
		if strings.TrimSpace(itemID) == "" || strings.TrimSpace(itemID) != strings.TrimSpace(pending.ItemID) {
			dispatchLock.Unlock()
			return fmt.Errorf("user input request does not match the active Pi prompt")
		}
		value := firstPiUserInputAnswer(answers)
		if value == "" {
			dispatchLock.Unlock()
			return fmt.Errorf("no answers were provided")
		}
		request, taken := run.takePendingPiRequestForResponse(pending.PiRequestID, true)
		if !taken {
			dispatchLock.Unlock()
			return fmt.Errorf("Pi user input is no longer pending")
		}
		dispatchLock.Unlock()
		return m.respondPiExtensionRequest(record, run, request, map[string]any{"value": value}, "user_input_res", map[string]any{
			"iid": request.ItemID, "ans": answers,
		})
	}
	defer dispatchLock.Unlock()
	if normalizeAgent(Agent(record.Agent)) == AgentClaude {
		if ok && run != nil {
			if pending, hasPending := run.pendingUserInputRequest(); hasPending &&
				strings.TrimSpace(pending.ItemID) == strings.TrimSpace(itemID) &&
				strings.TrimSpace(pending.ControlRequestID) != "" {
				input := m.claudeControlResponseInput(pending, answers)
				if !hasClaudeAnswerValues(answers) {
					return fmt.Errorf("no answers were provided")
				}
				if len(decodeRawObject(input["answers"])) == 0 {
					return fmt.Errorf("answers do not match the active Claude questions")
				}
				return m.respondClaudeControl(record, run, pending, "allow", input, "", answers)
			}
		}
		pending, err := m.findClaudePendingUserInputRequest(context.Background(), sessionID, itemID)
		if err != nil {
			return err
		}
		answerFile := claudeHookAnswerFile{Answers: map[string]string{}}
		for index, question := range pending.Questions {
			values := answers[strings.TrimSpace(question.ID)]
			if len(values) == 0 {
				values = answers[fmt.Sprintf("%d", index)]
			}
			if len(values) == 0 {
				continue
			}
			key := strings.TrimSpace(firstNonEmpty(question.Question, question.Header, question.ID))
			if key == "" {
				continue
			}
			answerFile.Answers[key] = strings.Join(values, ", ")
		}
		if len(answerFile.Answers) == 0 {
			return fmt.Errorf("no answers were provided")
		}
		if err := m.writeClaudeHookAnswerForSession(record, pending.ItemID, answerFile); err != nil {
			return err
		}
		if err := m.startClaudeDeferredResume(context.Background(), record, pending); err != nil {
			return err
		}
		now := time.Now()
		_, _ = m.appendAndBroadcast(context.Background(), sessionID, record, Event{
			ID:        utils.NewID(),
			Seq:       0,
			Type:      "user_input_res",
			RunID:     utils.NewID(),
			Timestamp: now,
			Payload: map[string]any{
				"iid": pending.ItemID,
				"ans": answers,
			},
		})
		return nil
	}
	if !ok || run == nil {
		return fmt.Errorf("session is not running")
	}
	pending, ok := run.pendingUserInputRequest()
	if !ok {
		return fmt.Errorf("no pending user input request")
	}
	if strings.TrimSpace(itemID) == "" || strings.TrimSpace(pending.ItemID) != strings.TrimSpace(itemID) {
		return fmt.Errorf("user input request does not match the active prompt")
	}
	app := run.codexAppServer()
	if app == nil {
		return fmt.Errorf("session input channel is unavailable")
	}
	if err := app.respond(pending.RawID, userInputResponsePayload(answers)); err != nil {
		return err
	}
	run.markUserInputResponsePending()
	run.clearPendingServerRequest()
	m.resumeActiveCallTimeout(run)

	now := time.Now()
	_, _ = m.appendAndBroadcast(context.Background(), sessionID, record, Event{
		ID:        utils.NewID(),
		Seq:       0,
		Type:      "user_input_res",
		RunID:     run.runID,
		ParentID:  run.assistantMessageID,
		ThreadID:  pending.ThreadID,
		TurnID:    pending.TurnID,
		Timestamp: now,
		Payload: map[string]any{
			"iid": pending.ItemID,
			"ans": answers,
		},
	})
	_ = m.updateRuntimeState(
		context.Background(),
		sessionID,
		applyAssistantStateUpdates(map[string]any{
			"updated_at": now,
		}, AssistantStateWorking, now),
	)
	m.broadcastSessionSummary(context.Background(), sessionID)
	return nil
}

func (m *Manager) hasActiveRun(sessionID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.runs[sessionID]
	return ok
}

func (m *Manager) beginCodexRunDrain(sessionID string, run *activeRun) {
	if run == nil {
		return
	}
	m.mu.Lock()
	if m.runs[sessionID] == run {
		if m.codexRunDrains == nil {
			m.codexRunDrains = make(map[string]*activeRun)
		}
		m.codexRunDrains[sessionID] = run
	}
	m.mu.Unlock()
	m.broadcastCodexAppServerRuntime(sessionID)
}

func (m *Manager) finishCodexRunDrain(sessionID string, run *activeRun) {
	m.mu.Lock()
	if m.codexRunDrains[sessionID] == run {
		delete(m.codexRunDrains, sessionID)
	}
	m.mu.Unlock()
}

func (m *Manager) waitForCodexRunDrain(ctx context.Context, sessionID string) error {
	timer := time.NewTimer(codexRunDrainWaitTimeout)
	defer timer.Stop()
	for {
		m.mu.RLock()
		run := m.codexRunDrains[sessionID]
		m.mu.RUnlock()
		if run == nil {
			return nil
		}

		select {
		case <-run.done:
			// The drain entry is removed immediately before the done channel is
			// closed. Re-check it to cover that handoff window.
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			m.terminateCodexAppServerRun(run, false)
			m.broadcastCodexAppServerRuntime(sessionID)
			return ErrCodexRunDrainTimeout
		}
	}
}

func (m *Manager) ForceTerminateCodexAppServer(projectID, sessionID string) (CodexAppServerTermination, error) {
	projectID = strings.TrimSpace(projectID)
	sessionID = strings.TrimSpace(sessionID)

	m.mu.Lock()
	run := m.codexRunDrains[sessionID]
	stateBefore := "draining"
	if run == nil {
		run = m.runs[sessionID]
		stateBefore = "active"
	}
	if run == nil {
		request, ok := m.codexTerminationRequests[sessionID]
		if ok && time.Now().Before(request.expiresAt) {
			m.mu.Unlock()
			if strings.TrimSpace(request.projectID) != projectID {
				return CodexAppServerTermination{}, ErrCodexAppServerProjectMismatch
			}
			result := request.result
			result.AlreadyRequested = true
			result.Runtime = m.codexAppServerRuntime(sessionID)
			return result, nil
		}
		delete(m.codexTerminationRequests, sessionID)
	}
	m.mu.Unlock()

	if run == nil || run.backend != SessionBackendCodexAppServer || normalizeAgent(run.agent) != AgentCodex || run.codexAppServer() == nil {
		return CodexAppServerTermination{}, ErrCodexAppServerNotActive
	}
	if strings.TrimSpace(run.projectID) != projectID {
		return CodexAppServerTermination{}, ErrCodexAppServerProjectMismatch
	}

	pid, alreadyRequested := m.terminateCodexAppServerRun(run, stateBefore == "active")
	m.broadcastCodexAppServerRuntime(sessionID)
	result := CodexAppServerTermination{
		SessionID:        sessionID,
		RunID:            run.runID,
		StateBefore:      stateBefore,
		ProcessRootPID:   pid,
		AlreadyRequested: alreadyRequested,
		Runtime:          m.codexAppServerRuntime(sessionID),
	}
	m.mu.Lock()
	if m.codexTerminationRequests == nil {
		m.codexTerminationRequests = make(map[string]codexTerminationRequest)
	}
	stored := result
	stored.AlreadyRequested = true
	m.codexTerminationRequests[sessionID] = codexTerminationRequest{
		projectID: projectID,
		result:    stored,
		expiresAt: time.Now().Add(codexTerminationRequestTTL),
	}
	expiresAt := m.codexTerminationRequests[sessionID].expiresAt
	m.mu.Unlock()
	time.AfterFunc(codexTerminationRequestTTL, func() {
		m.mu.Lock()
		request, ok := m.codexTerminationRequests[sessionID]
		if ok && request.expiresAt.Equal(expiresAt) && !time.Now().Before(request.expiresAt) {
			delete(m.codexTerminationRequests, sessionID)
		}
		m.mu.Unlock()
	})
	return result, nil
}

func (m *Manager) terminateCodexAppServerRun(run *activeRun, cancelActive bool) (int, bool) {
	if run == nil {
		return 0, false
	}
	run.mu.Lock()
	alreadyRequested := run.forceTerminateRequested
	run.forceTerminateRequested = true
	cancel := run.cancel
	cmd := run.cmd
	client := run.app
	run.mu.Unlock()

	pid := 0
	if cmd != nil && cmd.Process != nil {
		pid = cmd.Process.Pid
	}
	if cancelActive && cancel != nil {
		cancel()
	}
	if client != nil {
		_ = client.closeStdin()
	}
	killCmdTree(cmd)
	if client != nil {
		client.closeTransport()
	}
	return pid, alreadyRequested
}

// Claude invokes the PreToolUse hook before it emits a stdio control_request.
// Once a live stdio run is active, returning a hook decision would short-circuit
// that RPC and leave the process waiting forever. Prefer the native session ID;
// cwd is a startup fallback until Claude's system/init frame arrives.
func (m *Manager) shouldBypassClaudeHook(sessionID, cwd string) bool {
	if m == nil {
		return false
	}
	m.mu.RLock()
	runs := make([]*activeRun, 0, len(m.runs))
	for _, run := range m.runs {
		if run != nil {
			runs = append(runs, run)
		}
	}
	m.mu.RUnlock()

	exactMatches := 0
	exactStdioMatches := 0
	cwdMatches := 0
	stdioCwdMatches := 0
	for _, run := range runs {
		idMatch, cwdMatch, stdio := run.claudeHookMatch(sessionID, cwd)
		if idMatch {
			exactMatches++
			if stdio {
				exactStdioMatches++
			}
		}
		if cwdMatch {
			cwdMatches++
			if stdio {
				stdioCwdMatches++
			}
		}
	}
	if exactMatches > 0 {
		return exactMatches == 1 && exactStdioMatches == 1
	}
	return strings.TrimSpace(cwd) != "" && cwdMatches > 0 && cwdMatches == stdioCwdMatches
}

func (m *Manager) releaseActiveRun(sessionID string, run *activeRun) bool {
	m.mu.Lock()
	current := m.runs[sessionID]
	if current == run {
		delete(m.runs, sessionID)
	}
	m.mu.Unlock()

	if current != run {
		return false
	}
	m.triggerPendingProcessing(sessionID)
	return true
}

func (m *Manager) broadcast(frame wireFrame) {
	if frame.SessionID != "" && frame.Kind == wireKindEvent {
		lock := &m.revisionBroadcastLocks[sessionRevisionLockIndex(frame.SessionID)]
		lock.Lock()
		if revision := m.currentSessionRevision(context.Background(), frame.SessionID); revision != "" {
			applyWireFrameRevision(&frame, revision)
		}
		m.broadcastFrame(frame)
		lock.Unlock()
		return
	}
	m.broadcastFrame(frame)
}

func applyWireFrameRevision(frame *wireFrame, revision string) {
	if frame == nil {
		return
	}
	frame.Revision = revision
	if frame.Session != nil {
		frame.Session.Revision = revision
	}
}

// broadcastCommittedFrame publishes state at the revision committed by the
// business mutation. It never advances the SQLite revision on its own.
func (m *Manager) broadcastCommittedFrame(sessionID string, frame wireFrame, revision string) {
	lock := &m.revisionBroadcastLocks[sessionRevisionLockIndex(sessionID)]
	lock.Lock()
	applyWireFrameRevision(&frame, revision)
	m.broadcastFrame(frame)
	lock.Unlock()
}

// broadcastTransientFrame advances the durable revision for state that is not
// stored on web_sessions, then publishes that state at the new revision.
func (m *Manager) broadcastTransientFrame(ctx context.Context, sessionID string, frame wireFrame) error {
	lock := &m.revisionBroadcastLocks[sessionRevisionLockIndex(sessionID)]
	lock.Lock()
	defer lock.Unlock()
	revision, err := m.advanceSessionRevision(ctx, sessionID)
	if err != nil {
		return err
	}
	applyWireFrameRevision(&frame, formatSnapshotRevision(revision))
	m.broadcastFrame(frame)
	return nil
}

func (m *Manager) broadcastFrame(frame wireFrame) {
	m.mu.RLock()
	clients := make([]*client, 0, len(m.clients))
	for client := range m.clients {
		if client.kind == clientKindEvent {
			clients = append(clients, client)
		}
	}
	m.mu.RUnlock()

	for _, client := range clients {
		if !shouldSendFrameToClient(client, frame) {
			continue
		}
		if err := client.send(frame); err != nil {
			m.logger.Debug("failed to send ws frame", zap.Error(err))
			client.closeWithReason("broadcast-send-failed")
		}
	}
}

func shouldSendFrameToClient(client *client, frame wireFrame) bool {
	if client == nil {
		return false
	}
	focusedSessionID := client.FocusedSessionID()
	switch frame.Kind {
	case wireKindEvent:
		switch strings.ToLower(strings.TrimSpace(frame.Operation)) {
		case wireOpHistoryItem, wireOpHistoryPage, wireOpPending, wireOpScheduled, wireOpSubAgent, wireOpAppServer, wireOpResyncRequired:
			return focusedSessionID != "" && focusedSessionID == strings.TrimSpace(frame.SessionID)
		default:
			return true
		}
	default:
		return true
	}
}

func (m *Manager) broadcastResyncRequired(ctx context.Context, sessionID string, reason resyncReason) error {
	record, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if record.ArchivedAt != nil {
		return nil
	}
	m.broadcastCommittedFrame(
		sessionID,
		newResyncRequiredFrame(sessionID, reason),
		formatSnapshotRevision(record.SnapshotRevision),
	)
	return nil
}

func (m *Manager) broadcastSessionSummary(ctx context.Context, sessionID string) {
	summary := m.summaryForBroadcast(ctx, sessionID)
	if summary == nil {
		return
	}
	m.broadcastCommittedFrame(sessionID, newSessionFrame(sessionID, *summary), summary.Revision)
}

func (m *Manager) broadcastProjectSessionSummaries(ctx context.Context, projectID string) {
	items, err := m.ListSessions(ctx, projectID)
	if err != nil {
		if m.logger != nil {
			m.logger.Debug("failed to list web sessions for broadcast", zap.String("projectId", projectID), zap.Error(err))
		}
		return
	}
	for _, item := range items {
		m.broadcastSessionSummary(ctx, item.ID)
	}
}

func (c *client) send(frame wireFrame) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if err := c.conn.WriteJSON(frame); err != nil {
		return err
	}
	c.observeCommandResponse(frame)
	return nil
}

func mapSessionRecord(record tables.WebSessionTable) SessionSummary {
	activityAt := record.ActivityAt
	if activityAt.IsZero() {
		activityAt = chooseSessionActivityAt(record)
	}
	assistantState := effectiveAssistantState(record)
	statusUpdatedAt := effectiveStatusUpdatedAt(record, assistantState)
	assistantStateUpdatedAt := effectiveAssistantStateUpdatedAt(record, assistantState)
	latestTurnUsage, _ := buildLatestTurnUsage(record)
	contextEstimate, contextEstimateMode := buildContextEstimate(record)
	return SessionSummary{
		ID:                                record.ID,
		Revision:                          formatSnapshotRevision(record.SnapshotRevision),
		ProjectID:                         record.ProjectID,
		WorktreeID:                        record.WorktreeID,
		OrderIndex:                        record.OrderIndex,
		Agent:                             Agent(record.Agent),
		ClaudeRuntime:                     effectiveClaudeRuntime(record),
		Backend:                           effectiveSessionBackend(record),
		Title:                             record.Title,
		Model:                             record.Model,
		ReasoningEffort:                   ReasoningEffort(record.ReasoningEffort),
		WorkflowMode:                      effectiveWorkflowMode(record),
		PermissionLevel:                   effectivePermissionLevel(record),
		ActiveCallTimeoutEnabled:          activeCallTimeoutOverrideOrDefault(record.ActiveCallTimeoutEnabled),
		AutoRetryEnabled:                  record.AutoRetryEnabled,
		AutoRetryPolicyMode:               normalizeAutoRetryPolicyMode(AutoRetryPolicyMode(record.AutoRetryPolicyMode)),
		AutoRetryScope:                    normalizeAutoRetryScope(AutoRetryScope(record.AutoRetryScope)),
		AutoRetryPreset:                   normalizeAutoRetryPreset(AutoRetryPreset(record.AutoRetryPreset)),
		AutoRetryMaxAttempts:              normalizeAutoRetryMaxAttempts(record.AutoRetryMaxAttempts),
		AutoRetryDispatchPendingOnFailure: record.AutoRetryDispatchPendingOnFailure,
		Cwd:                               record.Cwd,
		NativeSessionID:                   record.NativeSessionID,
		NativeLeafID:                      record.NativeLeafID,
		SourceRevision:                    record.SourceRevision,
		CyberPolicyFlagged:                record.CyberPolicyFlagged,
		Status:                            effectiveStatus(record, assistantState),
		AssistantState:                    assistantState,
		HasUnread:                         record.HasUnread,
		AttentionRevision:                 strconv.FormatInt(normalizeAttentionRevision(record.AttentionRevision), 10),
		HistoryEpoch:                      strconv.FormatInt(normalizeHistoryEpoch(record.HistoryEpoch), 10),
		EventCursor:                       formatEventCursor(record.LastEventSeq, maxEventCursorOrder),
		ArchivedAt:                        record.ArchivedAt,
		ActivityAt:                        activityAt,
		StatusUpdatedAt:                   statusUpdatedAt,
		LastMessageAt:                     record.LastMessageAt,
		AssistantStateUpdatedAt:           assistantStateUpdatedAt,
		SourceKind:                        record.SourceKind,
		SyncState:                         normalizeSyncState(record.SyncState),
		LastSyncMode:                      recordedSyncMode(record.LastSyncMode),
		SourceCreatedAt:                   record.SourceCreatedAt,
		SourceUpdatedAt:                   record.SourceUpdatedAt,
		LastSyncedAt:                      record.LastSyncedAt,
		ThreadPath:                        record.ThreadPath,
		ThreadPreview:                     record.ThreadPreview,
		TurnCount:                         record.TurnCount,
		ItemCount:                         record.ItemCount,
		SyncError:                         record.SyncError,
		CreatedAt:                         record.CreatedAt,
		UpdatedAt:                         record.UpdatedAt,
		Usage: Usage{
			InputTokens:       record.TotalInputTokens,
			CachedInputTokens: record.TotalCachedInputTokens,
			OutputTokens:      record.TotalOutputTokens,
			Cost:              record.TotalCost,
		},
		LatestTurnUsage:         latestTurnUsage,
		ContextEstimate:         contextEstimate,
		ContextEstimateMode:     contextEstimateMode,
		LastContextCompactionAt: record.LastContextCompactionAt,
		ContextWindowTokens: func() *int64 {
			if record.SessionContextWindowTokens <= 0 {
				return nil
			}
			return ptr(record.SessionContextWindowTokens)
		}(),
		ContextWindowSource: func() ContextWindowSource {
			if record.SessionContextWindowTokens > 0 {
				return ContextWindowSourceSessionUsage
			}
			return ""
		}(),
		Goal:       sessionGoalFromRecord(record),
		WorkTiming: workTimingFromRecord(record),
	}
}

func normalizeGoalStatus(value string) GoalStatus {
	switch strings.TrimSpace(value) {
	case string(GoalStatusActive):
		return GoalStatusActive
	case string(GoalStatusPaused):
		return GoalStatusPaused
	case string(GoalStatusBlocked):
		return GoalStatusBlocked
	case string(GoalStatusUsageLimited):
		return GoalStatusUsageLimited
	case string(GoalStatusBudgetLimit):
		return GoalStatusBudgetLimit
	case string(GoalStatusComplete):
		return GoalStatusComplete
	default:
		return ""
	}
}

func sessionGoalFromRecord(record tables.WebSessionTable) *SessionGoal {
	if record.GoalObjective == nil || strings.TrimSpace(*record.GoalObjective) == "" {
		return nil
	}
	threadID := ""
	if record.NativeSessionID != nil {
		threadID = strings.TrimSpace(*record.NativeSessionID)
	}
	if threadID == "" || record.GoalCreatedAt == nil || record.GoalUpdatedAt == nil {
		return nil
	}
	status := normalizeGoalStatus(firstNonEmpty(pointerString(record.GoalStatus), string(GoalStatusActive)))
	if status == "" {
		status = GoalStatusActive
	}
	return &SessionGoal{
		ThreadID:        threadID,
		Objective:       strings.TrimSpace(*record.GoalObjective),
		Status:          status,
		TokenBudget:     record.GoalTokenBudget,
		TokensUsed:      maxInt64(0, record.GoalTokensUsed),
		TimeUsedSeconds: maxInt64(0, record.GoalTimeUsedSeconds),
		CreatedAt:       *record.GoalCreatedAt,
		UpdatedAt:       *record.GoalUpdatedAt,
	}
}

func pointerString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func contextEstimateUsedTokens(inputTokens, outputTokens int64) int64 {
	return maxInt64(0, inputTokens+outputTokens)
}

func contextEstimateHasValue(estimate ContextEstimate) bool {
	return estimate.InputTokens > 0 ||
		estimate.CachedInputTokens > 0 ||
		estimate.OutputTokens > 0 ||
		estimate.UsedTokens > 0
}

func shouldUseActiveTurnContextEstimate(record tables.WebSessionTable) bool {
	switch effectiveStatus(record, effectiveAssistantState(record)) {
	case StatusRunning, StatusWaitingApproval, StatusAborting:
		return true
	default:
		return false
	}
}

func buildRecordedLatestTurnUsage(record tables.WebSessionTable) (ContextEstimate, bool) {
	if record.LatestTurnUsageUpdatedAt == nil {
		return ContextEstimate{}, false
	}
	estimate := ContextEstimate{
		InputTokens:       maxInt64(0, record.LatestTurnInputTokens),
		CachedInputTokens: maxInt64(0, record.LatestTurnCachedInputTokens),
		OutputTokens:      maxInt64(0, record.LatestTurnOutputTokens),
	}
	estimate.UsedTokens = contextEstimateUsedTokens(estimate.InputTokens, estimate.OutputTokens)
	return estimate, contextEstimateHasValue(estimate)
}

func buildProvisionalLatestTurnUsage(record tables.WebSessionTable) (ContextEstimate, bool) {
	if !shouldUseActiveTurnContextEstimate(record) {
		return ContextEstimate{}, false
	}
	estimate := ContextEstimate{
		InputTokens:       maxInt64(0, record.TotalInputTokens-record.LastCompletedInputTokens),
		CachedInputTokens: maxInt64(0, record.TotalCachedInputTokens-record.LastCompletedCachedInputTokens),
		OutputTokens:      maxInt64(0, record.TotalOutputTokens-record.LastCompletedOutputTokens),
	}
	estimate.UsedTokens = contextEstimateUsedTokens(estimate.InputTokens, estimate.OutputTokens)
	return estimate, contextEstimateHasValue(estimate)
}

func buildLatestTurnUsage(record tables.WebSessionTable) (ContextEstimate, bool) {
	if estimate, ok := buildProvisionalLatestTurnUsage(record); ok {
		return estimate, true
	}
	return buildRecordedLatestTurnUsage(record)
}

func buildLatestTokenCountUsage(record tables.WebSessionTable) (ContextEstimate, bool) {
	if record.LatestTokenCountUpdatedAt == nil {
		return ContextEstimate{}, false
	}
	estimate := ContextEstimate{
		InputTokens:       maxInt64(0, record.LatestTokenCountInputTokens),
		CachedInputTokens: maxInt64(0, record.LatestTokenCountCachedInputTokens),
		OutputTokens:      maxInt64(0, record.LatestTokenCountOutputTokens),
	}
	if record.LatestTokenCountTotalTokens > 0 {
		estimate.UsedTokens = maxInt64(0, record.LatestTokenCountTotalTokens)
	} else {
		estimate.UsedTokens = contextEstimateUsedTokens(estimate.InputTokens, estimate.OutputTokens)
	}
	return estimate, contextEstimateHasValue(estimate)
}

func buildContextEstimate(record tables.WebSessionTable) (ContextEstimate, ContextEstimateMode) {
	if latestTokenCount, ok := buildLatestTokenCountUsage(record); ok {
		return latestTokenCount, ContextEstimateModeLatestTokenCount
	}
	if latestTurnUsage, ok := buildLatestTurnUsage(record); ok {
		return latestTurnUsage, ContextEstimateModeLatestTurnDelta
	}

	mode := ContextEstimateModeCumulativeTotal
	inputTokens := record.TotalInputTokens
	cachedInputTokens := record.TotalCachedInputTokens
	outputTokens := record.TotalOutputTokens
	if record.LastContextCompactionAt != nil {
		mode = ContextEstimateModeSinceCompaction
		inputTokens = maxInt64(0, record.TotalInputTokens-record.ContextBaselineInputTokens)
		cachedInputTokens = maxInt64(0, record.TotalCachedInputTokens-record.ContextBaselineCachedInputTokens)
		outputTokens = maxInt64(0, record.TotalOutputTokens-record.ContextBaselineOutputTokens)
	}
	return ContextEstimate{
		InputTokens:       inputTokens,
		CachedInputTokens: cachedInputTokens,
		OutputTokens:      outputTokens,
		UsedTokens:        contextEstimateUsedTokens(inputTokens, outputTokens),
	}, mode
}

func maxInt64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}

func contextEstimateIncrementUpdate(in, cin, out int64) map[string]any {
	return map[string]any{
		"total_input_tokens":                     gorm.Expr("total_input_tokens + ?", in),
		"total_cached_input_tokens":              gorm.Expr("total_cached_input_tokens + ?", cin),
		"total_output_tokens":                    gorm.Expr("total_output_tokens + ?", out),
		"latest_token_count_input_tokens":        0,
		"latest_token_count_cached_input_tokens": 0,
		"latest_token_count_output_tokens":       0,
		"latest_token_count_total_tokens":        0,
		"latest_token_count_updated_at":          nil,
		"updated_at":                             time.Now(),
	}
}

func contextEstimateBaselineResetUpdate(record tables.WebSessionTable, timestamp time.Time) map[string]any {
	if timestamp.IsZero() {
		timestamp = time.Now()
	}
	return map[string]any{
		"context_baseline_input_tokens":          record.TotalInputTokens,
		"context_baseline_cached_input_tokens":   record.TotalCachedInputTokens,
		"context_baseline_output_tokens":         record.TotalOutputTokens,
		"last_context_compaction_at":             timestamp,
		"latest_token_count_input_tokens":        0,
		"latest_token_count_cached_input_tokens": 0,
		"latest_token_count_output_tokens":       0,
		"latest_token_count_total_tokens":        0,
		"latest_token_count_updated_at":          nil,
		"updated_at":                             time.Now(),
	}
}

func (m *Manager) finalizeLatestTurnUsage(ctx context.Context, sessionID string) error {
	record, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}

	latestTurnUsage := ContextEstimate{
		InputTokens:       maxInt64(0, record.TotalInputTokens-record.LastCompletedInputTokens),
		CachedInputTokens: maxInt64(0, record.TotalCachedInputTokens-record.LastCompletedCachedInputTokens),
		OutputTokens:      maxInt64(0, record.TotalOutputTokens-record.LastCompletedOutputTokens),
	}
	latestTurnUsage.UsedTokens = contextEstimateUsedTokens(
		latestTurnUsage.InputTokens,
		latestTurnUsage.OutputTokens,
	)
	if !contextEstimateHasValue(latestTurnUsage) {
		return nil
	}

	now := time.Now()
	return m.updateRuntimeState(ctx, sessionID, map[string]any{
		"last_completed_input_tokens":        record.TotalInputTokens,
		"last_completed_cached_input_tokens": record.TotalCachedInputTokens,
		"last_completed_output_tokens":       record.TotalOutputTokens,
		"latest_turn_input_tokens":           latestTurnUsage.InputTokens,
		"latest_turn_cached_input_tokens":    latestTurnUsage.CachedInputTokens,
		"latest_turn_output_tokens":          latestTurnUsage.OutputTokens,
		"latest_turn_usage_updated_at":       now,
		"updated_at":                         now,
	})
}

func chooseSessionActivityAt(record tables.WebSessionTable) time.Time {
	if record.LastMessageAt != nil && !record.LastMessageAt.IsZero() {
		return *record.LastMessageAt
	}
	if !record.UpdatedAt.IsZero() {
		return record.UpdatedAt
	}
	if !record.CreatedAt.IsZero() {
		return record.CreatedAt
	}
	return time.Now()
}

func attachmentPayloads(items []Attachment) []map[string]any {
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, map[string]any{
			"id":   item.ID,
			"name": item.Name,
			"mime": item.Mime,
			"sz":   item.Size,
		})
	}
	return result
}

func defaultTitle(agent Agent, projectName string) string {
	prefix := "Chat"
	switch normalizeAgent(agent) {
	case AgentCodex:
		prefix = "Codex"
	case AgentClaude:
		prefix = "Claude"
	case AgentPi:
		prefix = "Pi"
	}
	if strings.TrimSpace(projectName) == "" {
		return prefix
	}
	return fmt.Sprintf("%s · %s", prefix, projectName)
}

func defaultModel(agent Agent, provided string) string {
	if strings.TrimSpace(provided) != "" {
		return strings.TrimSpace(provided)
	}
	switch normalizeAgent(agent) {
	case AgentCodex:
		return utils.DefaultWebSessionCodexModel
	case AgentClaude:
		return "opus"
	default:
		return ""
	}
}

func defaultReasoningEffort(agent Agent, provided ReasoningEffort) ReasoningEffort {
	raw := strings.ToLower(strings.TrimSpace(string(provided)))
	if raw == string(ReasoningEffortDefault) {
		return ReasoningEffortDefault
	}
	if raw != "" {
		if normalized := normalizeReasoningEffort(provided); normalized != ReasoningEffortDefault {
			return normalized
		}
	}
	if normalizeAgent(agent) == AgentCodex {
		return ReasoningEffort(utils.DefaultWebSessionCodexReasoningEffort)
	}
	return ReasoningEffortDefault
}

func (m *Manager) resolveSessionModel(agent Agent, provided string) string {
	if strings.TrimSpace(provided) != "" || normalizeAgent(agent) != AgentCodex {
		return defaultModel(agent, provided)
	}
	if m != nil && m.cfg.DefaultCodexModel != nil {
		if configured := strings.TrimSpace(m.cfg.DefaultCodexModel()); configured != "" {
			if strings.EqualFold(configured, utils.WebSessionCodexDefaultSetting) {
				return defaultModel(agent, "")
			}
			return configured
		}
	}
	return defaultModel(agent, "")
}

func (m *Manager) resolveSessionReasoningEffort(
	agent Agent,
	modelName string,
	provided ReasoningEffort,
) ReasoningEffort {
	if strings.TrimSpace(string(provided)) != "" || normalizeAgent(agent) != AgentCodex {
		return defaultReasoningEffort(agent, provided)
	}
	configured := strings.TrimSpace(utils.WebSessionCodexDefaultSetting)
	if m != nil && m.cfg.DefaultCodexReasoningEffort != nil {
		if value := strings.TrimSpace(string(m.cfg.DefaultCodexReasoningEffort())); value != "" {
			configured = strings.ToLower(value)
		}
	}
	switch configured {
	case utils.WebSessionCodexDefaultSetting:
		return normalizeCodexReasoningEffort(
			modelName,
			ReasoningEffort(utils.DefaultWebSessionCodexReasoningEffort),
		)
	case utils.WebSessionCodexModelDefaultEffort:
		return ReasoningEffortDefault
	default:
		return normalizeCodexReasoningEffort(modelName, ReasoningEffort(configured))
	}
}

func (m *Manager) resolveSessionPermissionLevel(
	agent Agent,
	provided PermissionLevel,
) PermissionLevel {
	if strings.TrimSpace(string(provided)) != "" || normalizeAgent(agent) != AgentCodex {
		return normalizePermissionLevel(provided)
	}
	configured := utils.WebSessionCodexDefaultSetting
	if m != nil && m.cfg.DefaultCodexPermissionLevel != nil {
		if value := strings.TrimSpace(m.cfg.DefaultCodexPermissionLevel()); value != "" {
			configured = strings.ToLower(value)
		}
	}
	switch configured {
	case utils.WebSessionCodexStandardPermission:
		return PermissionLevelDefault
	case string(PermissionLevelYolo):
		return PermissionLevelYolo
	case string(PermissionLevelElevated), utils.WebSessionCodexDefaultSetting:
		return PermissionLevelElevated
	default:
		return PermissionLevel(utils.DefaultWebSessionCodexPermissionLevel)
	}
}

func defaultSessionBackend(agent Agent) SessionBackend {
	switch normalizeAgent(agent) {
	case AgentCodex:
		return SessionBackendCodexAppServer
	case AgentPi:
		return SessionBackendPiRPC
	default:
		return SessionBackendLegacyExec
	}
}

func normalizeSessionBackend(backend SessionBackend, agent Agent) SessionBackend {
	normalizedAgent := normalizeAgent(agent)
	switch strings.ToLower(strings.TrimSpace(string(backend))) {
	case string(SessionBackendCodexAppServer):
		if normalizedAgent == AgentCodex {
			return SessionBackendCodexAppServer
		}
	case string(SessionBackendPiRPC):
		if normalizedAgent == AgentPi {
			return SessionBackendPiRPC
		}
	case string(SessionBackendLegacyExec):
		if normalizedAgent != AgentPi {
			return SessionBackendLegacyExec
		}
	}
	return defaultSessionBackend(normalizedAgent)
}

func normalizeAgent(agent Agent) Agent {
	return Agent(strings.ToLower(strings.TrimSpace(string(agent))))
}

func validateAgent(agent Agent) (Agent, error) {
	normalized := normalizeAgent(agent)
	switch normalized {
	case AgentClaude, AgentCodex, AgentPi:
		return normalized, nil
	default:
		return "", fmt.Errorf("invalid agent %q: expected claude, codex, or pi", strings.TrimSpace(string(agent)))
	}
}

func normalizeClaudeRuntime(runtime ClaudeRuntime) ClaudeRuntime {
	switch strings.ToLower(strings.TrimSpace(string(runtime))) {
	case string(ClaudeRuntimeCCR):
		return ClaudeRuntimeCCR
	default:
		return ClaudeRuntimeNative
	}
}

func defaultClaudeRuntime(agent Agent) ClaudeRuntime {
	if normalizeAgent(agent) != AgentClaude {
		return ClaudeRuntimeNative
	}
	return ClaudeRuntimeNative
}

func effectiveClaudeRuntime(record tables.WebSessionTable) ClaudeRuntime {
	if normalizeAgent(Agent(record.Agent)) != AgentClaude {
		return ClaudeRuntimeNative
	}
	return normalizeClaudeRuntime(ClaudeRuntime(record.ClaudeRuntime))
}

func normalizeReasoningEffort(effort ReasoningEffort) ReasoningEffort {
	switch strings.ToLower(strings.TrimSpace(string(effort))) {
	case string(ReasoningEffortNone):
		return ReasoningEffortNone
	case string(ReasoningEffortMinimal):
		return ReasoningEffortMinimal
	case string(ReasoningEffortLow):
		return ReasoningEffortLow
	case string(ReasoningEffortMedium):
		return ReasoningEffortMedium
	case string(ReasoningEffortHigh):
		return ReasoningEffortHigh
	case string(ReasoningEffortXHigh):
		return ReasoningEffortXHigh
	case string(ReasoningEffortMax):
		return ReasoningEffortMax
	case string(ReasoningEffortUltra):
		return ReasoningEffortUltra
	default:
		return ReasoningEffortDefault
	}
}

func normalizeCodexReasoningEffort(modelName string, effort ReasoningEffort) ReasoningEffort {
	normalized := normalizeReasoningEffort(effort)
	modelName = strings.ToLower(strings.TrimSpace(modelName))
	switch modelName {
	case "gpt-5.6-sol", "gpt-5.6-terra":
		switch normalized {
		case ReasoningEffortDefault,
			ReasoningEffortLow,
			ReasoningEffortMedium,
			ReasoningEffortHigh,
			ReasoningEffortXHigh,
			ReasoningEffortMax,
			ReasoningEffortUltra:
			return normalized
		}
	case "gpt-5.6-luna":
		switch normalized {
		case ReasoningEffortDefault,
			ReasoningEffortLow,
			ReasoningEffortMedium,
			ReasoningEffortHigh,
			ReasoningEffortXHigh,
			ReasoningEffortMax:
			return normalized
		}
	default:
		return normalized
	}
	return ReasoningEffortDefault
}

func normalizeWorkflowMode(mode WorkflowMode) WorkflowMode {
	switch strings.ToLower(strings.TrimSpace(string(mode))) {
	case string(WorkflowModePlan):
		return WorkflowModePlan
	default:
		return WorkflowModeDefault
	}
}

func normalizeSessionStartSource(source SessionStartSource) SessionStartSource {
	if strings.EqualFold(strings.TrimSpace(string(source)), string(SessionStartSourceClear)) {
		return SessionStartSourceClear
	}
	return SessionStartSourceStartup
}

func normalizePermissionLevel(level PermissionLevel) PermissionLevel {
	switch strings.ToLower(strings.TrimSpace(string(level))) {
	case string(PermissionLevelDefault):
		return PermissionLevelDefault
	case string(PermissionLevelYolo):
		return PermissionLevelYolo
	default:
		return PermissionLevelElevated
	}
}

func validateWebSessionPermissionLevel(agent Agent, level PermissionLevel) error {
	normalizedAgent, err := validateAgent(agent)
	if err != nil {
		return err
	}
	normalizedLevel := normalizePermissionLevel(level)
	if normalizedAgent == AgentClaude && normalizedLevel == PermissionLevelDefault {
		return fmt.Errorf("claude web sessions do not support the default permission level in claude_stream_json mode")
	}
	if normalizedAgent == AgentPi && normalizedLevel != PermissionLevelElevated {
		return fmt.Errorf("pi web sessions currently support only unrestricted access")
	}
	return nil
}

func shouldCloseClaudeInput(stopReason string, content []any) bool {
	switch strings.TrimSpace(stopReason) {
	case "end_turn", "stop_sequence":
		return true
	case "":
		return false
	default:
		for _, rawBlock := range content {
			block := decodeRawObject(rawBlock)
			if strings.TrimSpace(stringValue(block["type"])) == "tool_use" {
				return false
			}
		}
		return false
	}
}

func sessionModesFromLegacy(legacy string) (WorkflowMode, PermissionLevel) {
	switch strings.ToLower(strings.TrimSpace(legacy)) {
	case "plan":
		return WorkflowModePlan, PermissionLevelElevated
	case "yolo":
		return WorkflowModeDefault, PermissionLevelYolo
	default:
		return WorkflowModeDefault, PermissionLevelElevated
	}
}

func effectiveWorkflowMode(record tables.WebSessionTable) WorkflowMode {
	if normalized := normalizeWorkflowMode(WorkflowMode(record.WorkflowMode)); normalized != WorkflowModeDefault ||
		strings.EqualFold(strings.TrimSpace(record.WorkflowMode), string(WorkflowModeDefault)) {
		return normalized
	}
	workflowMode, _ := sessionModesFromLegacy(record.LegacyPermissionMode)
	return workflowMode
}

func effectivePermissionLevel(record tables.WebSessionTable) PermissionLevel {
	if normalized := normalizePermissionLevel(PermissionLevel(record.PermissionLevel)); normalized != PermissionLevelElevated ||
		strings.EqualFold(strings.TrimSpace(record.PermissionLevel), string(PermissionLevelElevated)) ||
		strings.EqualFold(strings.TrimSpace(record.PermissionLevel), string(PermissionLevelDefault)) ||
		strings.EqualFold(strings.TrimSpace(record.PermissionLevel), string(PermissionLevelYolo)) {
		return normalized
	}
	_, permissionLevel := sessionModesFromLegacy(record.LegacyPermissionMode)
	return permissionLevel
}

func effectiveSessionBackend(record tables.WebSessionTable) SessionBackend {
	agent := normalizeAgent(Agent(record.Agent))
	normalized := strings.ToLower(strings.TrimSpace(record.Backend))
	switch normalized {
	case string(SessionBackendLegacyExec):
		if agent != AgentPi {
			return SessionBackendLegacyExec
		}
	case string(SessionBackendCodexAppServer):
		if agent == AgentCodex {
			return SessionBackendCodexAppServer
		}
	case string(SessionBackendPiRPC):
		if agent == AgentPi {
			return SessionBackendPiRPC
		}
	default:
		if agent == AgentCodex {
			// Existing Codex sessions predate backend persistence and must continue
			// using the legacy exec transport unless explicitly migrated.
			return SessionBackendLegacyExec
		}
	}
	return defaultSessionBackend(agent)
}

func preparePromptText(text string, workflowMode WorkflowMode) string {
	trimmedText := strings.TrimSpace(text)
	if normalizeWorkflowMode(workflowMode) != WorkflowModePlan {
		return trimmedText
	}
	if trimmedText == "" {
		return planPromptPreamble
	}
	return fmt.Sprintf("%s\n\nUser request:\n%s", planPromptPreamble, trimmedText)
}

func (m *Manager) migrateLegacySessionModes(ctx context.Context) error {
	db := model.GetDB()
	if db == nil {
		return model.ErrDBNotInitialized
	}

	var records []tables.WebSessionTable
	if err := db.WithContext(ctx).
		Select("id", "workflow_mode", "permission_level", "permission_mode").
		Find(&records).Error; err != nil {
		return err
	}

	for _, record := range records {
		updates := map[string]any{}
		legacyMode := strings.ToLower(strings.TrimSpace(record.LegacyPermissionMode))
		workflowMode, permissionLevel := sessionModesFromLegacy(record.LegacyPermissionMode)
		hasBootstrapDefaults := normalizeWorkflowMode(WorkflowMode(record.WorkflowMode)) == WorkflowModeDefault &&
			normalizePermissionLevel(PermissionLevel(record.PermissionLevel)) == PermissionLevelElevated

		if strings.TrimSpace(record.WorkflowMode) == "" || (hasBootstrapDefaults && legacyMode == "plan") {
			updates["workflow_mode"] = string(workflowMode)
		}
		if strings.TrimSpace(record.PermissionLevel) == "" || (hasBootstrapDefaults && legacyMode == "yolo") {
			updates["permission_level"] = string(permissionLevel)
		}
		if len(updates) == 0 {
			continue
		}
		updates["updated_at"] = time.Now()
		if err := db.WithContext(ctx).
			Model(&tables.WebSessionTable{}).
			Where("id = ?", record.ID).
			Updates(updates).Error; err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) recoverPendingAutoRetrySessions(ctx context.Context) error {
	db := model.GetDB()
	if db == nil {
		return model.ErrDBNotInitialized
	}

	var records []tables.WebSessionTable
	if err := db.WithContext(ctx).
		Where("auto_retry_enabled = ? AND auto_retry_next_at IS NOT NULL AND archived_at IS NULL", true).
		Order("auto_retry_next_at ASC").
		Find(&records).Error; err != nil {
		return err
	}
	for _, record := range records {
		if record.AutoRetryNextAt == nil {
			continue
		}
		m.setAutoRetryTimer(record.ID, *record.AutoRetryNextAt)
	}
	return nil
}

func (m *Manager) recoverInterruptedSessions(ctx context.Context) error {
	db := model.GetDB()
	if db == nil {
		return model.ErrDBNotInitialized
	}

	var records []tables.WebSessionTable
	if err := db.WithContext(ctx).
		Where("status IN ?", []string{string(StatusRunning), string(StatusAborting)}).
		Order("updated_at ASC").
		Find(&records).Error; err != nil {
		return err
	}
	recoverable := make([]tables.WebSessionTable, 0, len(records))
	for _, record := range records {
		if effectiveAssistantState(record) == AssistantStateWaitingPlanApproval {
			continue
		}
		recoverable = append(recoverable, record)
	}
	if len(recoverable) == 0 {
		return nil
	}

	if m.logger != nil {
		m.logger.Info("recovering interrupted web sessions", zap.Int("count", len(recoverable)))
	}

	for _, record := range recoverable {
		now := time.Now()
		if _, err := m.appendAndBroadcast(ctx, record.ID, record, Event{
			Type:      "run_abort",
			Timestamp: now,
			Payload: map[string]any{
				"reason":     recoveryReasonProcessRestart,
				"msg":        recoveryMessageProcessRestart,
				"prevStatus": record.Status,
			},
		}); err != nil {
			return err
		}
		if err := m.updateRuntimeState(ctx, record.ID, map[string]any{
			"status":                     string(StatusIdle),
			"has_unread":                 false,
			"last_error":                 nil,
			"updated_at":                 now,
			"status_updated_at":          now,
			"auto_retry_attempt":         0,
			"auto_retry_next_at":         nil,
			"auto_retry_last_error_code": nil,
			"assistant_state":            nil,
			"assistant_state_updated_at": nil,
		}); err != nil {
			return err
		}
		if err := db.WithContext(ctx).
			Model(&tables.WebSessionSubAgentTable{}).
			Where("web_session_id = ? AND status IN ?", record.ID, []string{
				string(WebSessionSubAgentPendingInit),
				string(WebSessionSubAgentRunning),
			}).
			Updates(map[string]any{
				"status":           string(WebSessionSubAgentInterrupted),
				"current_turn_id":  nil,
				"ended_at":         now,
				"last_activity_at": now,
			}).Error; err != nil {
			return err
		}
		m.cancelAutoRetryTimer(record.ID)
		m.broadcastSessionSummary(ctx, record.ID)
	}

	return nil
}

func nilIfEmpty(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func detectApprovalPrompt(lines []string) (string, bool) {
	if len(lines) == 0 {
		return "", false
	}
	joined := strings.Join(filterNonEmptyLines(lines), "\n")
	if strings.Contains(joined, "Press enter to confirm or esc to cancel") {
		return joinTrailingLines(lines, 3), true
	}
	if strings.Contains(joined, "Ready to submit your answers?") {
		return joinTrailingLines(lines, 4), true
	}
	if strings.Contains(joined, "Do you want to proceed?") {
		return joinTrailingLines(lines, 4), true
	}
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "Do you want to ") {
			return joinTrailingLines(lines, 4), true
		}
	}
	return "", false
}

func joinTrailingLines(lines []string, limit int) string {
	filtered := filterNonEmptyLines(lines)
	if len(filtered) > limit {
		filtered = filtered[len(filtered)-limit:]
	}
	return strings.Join(filtered, "\n")
}

func filterNonEmptyLines(lines []string) []string {
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		filtered = append(filtered, trimmed)
	}
	return filtered
}

func approvalInput(action string) string {
	if action == "reject" {
		return "\x1b"
	}
	return "\n"
}

func getenvDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func defaultCCRConfigPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(homeDir) == "" {
		return filepath.Join(".claude-code-router", "config.json")
	}
	return filepath.Join(homeDir, ".claude-code-router", "config.json")
}

func truncateString(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	end := 0
	for idx, r := range value {
		next := idx + utf8.RuneLen(r)
		if next > limit {
			break
		}
		end = next
	}
	return value[:end] + "..."
}

func codexToolName(item map[string]any) string {
	switch normalizeCodexItemType(stringValue(item["type"])) {
	case "command_execution":
		return "CommandExecution"
	case "context_compaction":
		return "Context Compaction"
	case "mcp_tool_call":
		return "McpToolCall"
	case "sub_agent_tool_call":
		return "Sub Agent"
	case "file_change":
		return "FileChange"
	case "reasoning":
		return "Reasoning"
	case "web_search":
		return "WebSearch"
	default:
		return stringValue(item["type"])
	}
}

func codexToolInput(item map[string]any) any {
	switch normalizeCodexItemType(stringValue(item["type"])) {
	case "command_execution":
		return map[string]any{"command": stringValue(item["command"])}
	case "context_compaction":
		return nil
	case "web_search":
		return map[string]any{
			"query":  item["query"],
			"action": item["action"],
		}
	case "reasoning":
		return nil
	}
	return item
}

func codexToolMeta(item map[string]any) map[string]any {
	kind := normalizeCodexItemType(stringValue(item["type"]))
	subtitle := firstNonEmpty(
		stringValue(item["command"]),
		stringValue(item["tool_name"]),
		stringValue(item["path"]),
		stringValue(item["query"]),
		stringValue(item["text"]),
	)
	if kind == "file_change" {
		subtitle = firstNonEmpty(fileChangeSummary(item), subtitle)
	}
	if kind == "context_compaction" {
		subtitle = contextCompactionSubtitle(item)
	}
	if kind == "sub_agent_tool_call" {
		subtitle = firstNonEmpty(subAgentToolCallSummary(item), subtitle)
	}
	return map[string]any{
		"kind":     kind,
		"title":    codexToolName(item),
		"subtitle": subtitle,
	}
}

func codexToolResult(item map[string]any) string {
	switch normalizeCodexItemType(stringValue(item["type"])) {
	case "reasoning":
		if text := extractReasoningText(item); text != "" {
			return text
		}
		return ""
	case "context_compaction":
		return extractContextCompactionText(item)
	}
	if output := stringValue(item["aggregated_output"]); output != "" {
		return output
	}
	if output := stringValue(item["aggregatedOutput"]); output != "" {
		return output
	}
	if text := stringValue(item["text"]); text != "" {
		return text
	}
	encoded, _ := json.Marshal(item)
	return string(encoded)
}

func toolOutputLimit(kind string) int {
	if normalizeCodexItemType(kind) == "plan" {
		return 0
	}
	return defaultToolOutputLimit
}

func truncateToolOutput(kind, value string) string {
	return truncateString(value, toolOutputLimit(kind))
}

func codexToolOutput(item map[string]any) string {
	return truncateToolOutput(stringValue(item["type"]), codexToolResult(item))
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return ""
	}
}

func numberValue(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	default:
		return 0
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func normalizeCodexItemType(value string) string {
	switch strings.TrimSpace(value) {
	case "commandExecution":
		return "command_execution"
	case "contextCompaction":
		return "context_compaction"
	case "mcpToolCall":
		return "mcp_tool_call"
	case "collabAgentToolCall", "collab_agent_tool_call", "sub_agent_tool_call":
		return "sub_agent_tool_call"
	case "fileChange":
		return "file_change"
	case "webSearch":
		return "web_search"
	case "agentMessage":
		return "agent_message"
	case "userMessage":
		return "user_message"
	default:
		return strings.TrimSpace(value)
	}
}

func extractContextCompactionText(item map[string]any) string {
	sections := make([]string, 0, 3)
	if summary := strings.TrimSpace(strings.Join(collectReasoningFragments(item["summary"]), "")); summary != "" {
		sections = append(sections, summary)
	}
	if text := strings.TrimSpace(stringValue(item["text"])); text != "" {
		sections = append(sections, text)
	}
	if message := strings.TrimSpace(stringValue(item["message"])); message != "" {
		sections = append(sections, message)
	}
	if content := strings.TrimSpace(strings.Join(collectReasoningFragments(item["content"]), "")); content != "" {
		sections = append(sections, content)
	}
	if output := strings.TrimSpace(strings.Join(collectReasoningFragments(item["output"]), "")); output != "" {
		sections = append(sections, output)
	}
	if len(sections) > 0 {
		return strings.TrimSpace(strings.Join(sections, "\n\n"))
	}
	encoded, _ := json.Marshal(item)
	return string(encoded)
}

func contextCompactionSubtitle(item map[string]any) string {
	text := strings.TrimSpace(extractContextCompactionText(item))
	if text == "" {
		return ""
	}
	return strings.TrimSpace(strings.SplitN(text, "\n", 2)[0])
}

func normalizeToolChoiceText(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(value))), " ")
}

func codexToolIsPlan(item map[string]any) bool {
	meta := codexToolMeta(item)
	candidates := []string{
		codexToolName(item),
		stringValue(item["type"]),
		stringValue(meta["kind"]),
		stringValue(meta["title"]),
	}
	for _, candidate := range candidates {
		if normalizeToolChoiceText(candidate) == "plan" {
			return true
		}
	}
	return false
}

func extractReasoningText(item map[string]any) string {
	sections := make([]string, 0, 2)
	if summary := strings.TrimSpace(strings.Join(collectReasoningFragments(item["summary"]), "")); summary != "" {
		sections = append(sections, summary)
	}
	if content := strings.TrimSpace(strings.Join(collectReasoningFragments(item["content"]), "")); content != "" {
		sections = append(sections, content)
	}
	return strings.TrimSpace(strings.Join(sections, "\n\n"))
}

func collectReasoningFragments(raw any) []string {
	switch typed := raw.(type) {
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil
		}
		return []string{typed}
	case []any:
		fragments := make([]string, 0, len(typed))
		for _, item := range typed {
			fragments = append(fragments, collectReasoningFragments(item)...)
		}
		return fragments
	case map[string]any:
		fragments := make([]string, 0, 2)
		for _, key := range []string{"text", "delta"} {
			if text := stringValue(typed[key]); strings.TrimSpace(text) != "" {
				fragments = append(fragments, text)
			}
		}
		for _, key := range []string{"summary", "content"} {
			if nested := typed[key]; nested != nil {
				fragments = append(fragments, collectReasoningFragments(nested)...)
			}
		}
		return fragments
	default:
		return nil
	}
}
