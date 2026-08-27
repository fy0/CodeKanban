package websession

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type activeRun struct {
	sessionID                 string
	projectID                 string
	agent                     Agent
	backend                   SessionBackend
	runID                     string
	fromAutoRetry             bool
	hiddenBootstrap           bool
	bootstrapGoalObjective    string
	bootstrapGoalState        GoalStatus
	bootstrapResult           chan error
	bootstrapOnce             sync.Once
	assistantMessageID        string
	currentToolMessage        string
	lastError                 string
	lastErrorCode             string
	transportRetrySeen        bool
	transportRetryRecovered   bool
	transportRemoteURL        string
	cmd                       *exec.Cmd
	cancel                    context.CancelFunc
	done                      chan struct{}
	mu                        sync.Mutex
	forceTerminateRequested   bool
	stdin                     io.WriteCloser
	recentRuntimeLines        []string
	pendingApproval           string
	pendingServerReq          *pendingServerRequest
	inputResponsePending      bool
	piResponseGeneration      uint64
	piResponsePending         uint64
	piResponseHistoryDone     chan struct{}
	piResponseHistoryComplete uint64
	piResponseHistoryErr      error
	piResponseRequest         *pendingServerRequest
	app                       *codexAppServerClient
	codexThreadID             string
	codexTurnID               string
	assistantDeltaSeen        map[string]bool
	assistantMessagePhases    map[string]string
	assistantMessageText      map[string]bool
	completedReply            bool
	claudeResumeOnly          bool
	claudeStdioControl        bool
	claudeNativeSessionID     string
	claudeResolvedModel       string
	claudeCwd                 string
	deferredUserInput         bool
	completedPlanTool         bool
	claudeCompactionToolID    string
	claudeCompactionStarted   time.Time
	commandGroupID            string
	commandGroupKind          string
	commandGroupKey           string
	commandGroupFirst         int64
	commandGroupCount         int
	commandGroupTools         map[string]struct{}
	abortPayload              map[string]any
	activeCalls               map[string]trackedActiveCall
	activeCallPausedAt        *time.Time
	activeCallTimer           *time.Timer
	activeCallInFlight        bool
	codexCollaboration        codexCollaborationTracker
	codexRolloutMonitor       *codexRolloutMonitor
	syncSourceAfterRun        bool
	piCompaction              bool
}

func (r *activeRun) setInput(stdin io.WriteCloser) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stdin = stdin
}

func (r *activeRun) setClaudeStdioControl(active bool) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.claudeStdioControl = active
	r.mu.Unlock()
}

func (r *activeRun) setClaudeIdentity(sessionID, cwd string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	if normalized := strings.TrimSpace(sessionID); normalized != "" {
		r.claudeNativeSessionID = normalized
	}
	r.claudeCwd = strings.TrimSpace(cwd)
	r.mu.Unlock()
}

func (r *activeRun) setClaudeNativeSessionID(sessionID string) {
	if r == nil {
		return
	}
	normalized := strings.TrimSpace(sessionID)
	if normalized == "" {
		return
	}
	r.mu.Lock()
	r.claudeNativeSessionID = normalized
	r.mu.Unlock()
}

func (r *activeRun) setClaudeResolvedModel(model string) {
	if r == nil {
		return
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return
	}
	r.mu.Lock()
	r.claudeResolvedModel = model
	r.mu.Unlock()
}

func (r *activeRun) claudeResolvedModelSnapshot() string {
	if r == nil {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.claudeResolvedModel
}

func (r *activeRun) claudeHookMatch(sessionID, cwd string) (idMatch, cwdMatch, stdio bool) {
	if r == nil {
		return false, false, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if normalizeAgent(r.agent) != AgentClaude {
		return false, false, false
	}
	normalizedSessionID := strings.TrimSpace(sessionID)
	idMatch = normalizedSessionID != "" &&
		(normalizedSessionID == strings.TrimSpace(r.sessionID) || normalizedSessionID == strings.TrimSpace(r.claudeNativeSessionID))
	cwdMatch = sameClaudeHookCwd(cwd, r.claudeCwd)
	stdio = r.claudeStdioControl && !r.claudeResumeOnly
	return idMatch, cwdMatch, stdio
}

func sameClaudeHookCwd(left, right string) bool {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" || right == "" {
		return false
	}
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func (r *activeRun) setCommand(cmd *exec.Cmd) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.cmd = cmd
	r.mu.Unlock()
}

func (r *activeRun) command() *exec.Cmd {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cmd
}

func (r *activeRun) setCodexAppServer(client *codexAppServerClient) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.app = client
	r.mu.Unlock()
}

func (r *activeRun) codexAppServer() *codexAppServerClient {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.app
}

func (r *activeRun) setCodexSteerTarget(threadID string, turnID string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	if normalized := strings.TrimSpace(threadID); normalized != "" {
		r.codexThreadID = normalized
	}
	if normalized := strings.TrimSpace(turnID); normalized != "" {
		r.codexTurnID = normalized
	}
	r.mu.Unlock()
}

func (r *activeRun) codexSteerTarget() (*codexAppServerClient, string, string) {
	if r == nil {
		return nil, "", ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.app, strings.TrimSpace(r.codexThreadID), strings.TrimSpace(r.codexTurnID)
}

func (r *activeRun) resolveBootstrap(err error) {
	if r == nil || r.bootstrapResult == nil {
		return
	}
	r.bootstrapOnce.Do(func() {
		r.bootstrapResult <- err
		close(r.bootstrapResult)
	})
}

func (r *activeRun) clearInput() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stdin = nil
}

func (r *activeRun) closeInput() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stdin != nil {
		_ = r.stdin.Close()
		r.stdin = nil
	}
}

func (r *activeRun) writeInput(input string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stdin == nil {
		return fmt.Errorf("session input is unavailable")
	}
	_, err := io.WriteString(r.stdin, input)
	return err
}

// writeJSONInput serializes a stream-json control response while holding the
// same mutex used by ordinary stdin writes. Claude can emit control requests
// concurrently with transcript events, so responses must never interleave
// with another line on the pipe.
func (r *activeRun) writeJSONInput(value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stdin == nil {
		return fmt.Errorf("session input is unavailable")
	}
	_, err = r.stdin.Write(encoded)
	return err
}

func (r *activeRun) pushRuntimeLine(line string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.recentRuntimeLines = append(r.recentRuntimeLines, strings.TrimSpace(line))
	if len(r.recentRuntimeLines) > 6 {
		r.recentRuntimeLines = append([]string(nil), r.recentRuntimeLines[len(r.recentRuntimeLines)-6:]...)
	}
	return append([]string(nil), r.recentRuntimeLines...)
}

func (r *activeRun) setPendingApproval(prompt string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	prompt = strings.TrimSpace(prompt)
	if prompt == "" || prompt == r.pendingApproval {
		return false
	}
	r.pendingApproval = prompt
	return true
}

func (r *activeRun) pendingApprovalPrompt() (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if strings.TrimSpace(r.pendingApproval) == "" {
		return "", false
	}
	return r.pendingApproval, true
}

func (r *activeRun) clearPendingApproval() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pendingApproval = ""
}

func (r *activeRun) setPendingServerRequest(request *pendingServerRequest) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if request == nil {
		return false
	}
	r.pendingServerReq = request
	return true
}

func (r *activeRun) pendingApprovalRequest() (*pendingServerRequest, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.pendingServerReq == nil || !r.pendingServerReq.isApproval() {
		return nil, false
	}
	return r.pendingServerReq.clone(), true
}

func (r *activeRun) pendingServerRequest() (*pendingServerRequest, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.pendingServerReq == nil {
		return nil, false
	}
	return r.pendingServerReq.clone(), true
}

func (r *activeRun) pendingUserInputRequest() (*pendingServerRequest, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.pendingServerReq == nil || r.pendingServerReq.Kind != pendingServerRequestUserInput {
		return nil, false
	}
	return r.pendingServerReq.clone(), true
}

func (r *activeRun) markUserInputResponsePending() {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.inputResponsePending = true
	r.mu.Unlock()
}

func (r *activeRun) releaseUserInputResponsePending() bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.inputResponsePending {
		return false
	}
	r.inputResponsePending = false
	return true
}

func (r *activeRun) blocksCodexSteerForUserInput() bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.inputResponsePending ||
		(r.pendingServerReq != nil && r.pendingServerReq.Kind == pendingServerRequestUserInput)
}

func (r *activeRun) blocksPiPendingInput() bool {
	if r == nil {
		return true
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.piResponsePending != 0 || r.pendingServerReq != nil
}

func (r *activeRun) finishPiResponseBarrier(generation uint64) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if generation == 0 || r.piResponsePending != generation {
		return false
	}
	r.piResponsePending = 0
	return r.pendingServerReq == nil
}

func (r *activeRun) finishPiResponseHistory(generation uint64, persisted bool, err error) {
	if r == nil || generation == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.piResponseGeneration != generation || r.piResponseHistoryComplete == generation {
		return
	}
	if persisted {
		r.piResponseRequest = nil
	}
	r.piResponseHistoryErr = err
	r.piResponseHistoryComplete = generation
	if r.piResponseHistoryDone != nil {
		close(r.piResponseHistoryDone)
		r.piResponseHistoryDone = nil
	}
}

func (r *activeRun) waitForPiResponseHistory(ctx context.Context) (*pendingServerRequest, error) {
	if r == nil {
		return nil, nil
	}
	for {
		r.mu.Lock()
		generation := r.piResponsePending
		if generation == 0 {
			r.mu.Unlock()
			return nil, nil
		}
		if r.piResponseHistoryComplete == generation {
			request := r.piResponseRequest.clone()
			err := r.piResponseHistoryErr
			r.piResponseRequest = nil
			r.mu.Unlock()
			return request, err
		}
		done := r.piResponseHistoryDone
		r.mu.Unlock()
		if done == nil {
			return nil, errors.New("Pi extension response completion state is unavailable")
		}
		select {
		case <-done:
		case <-ctx.Done():
			return nil, fmt.Errorf("wait for Pi extension response history: %w", ctx.Err())
		}
	}
}

func (r *activeRun) clearPendingServerRequest() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pendingServerReq = nil
}

func (r *activeRun) clearPendingControlRequest(requestID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.pendingServerReq == nil || strings.TrimSpace(requestID) == "" {
		return false
	}
	if strings.TrimSpace(r.pendingServerReq.ControlRequestID) != strings.TrimSpace(requestID) {
		return false
	}
	r.pendingServerReq = nil
	return true
}

func (r *activeRun) takePendingPiRequest(requestID string) (*pendingServerRequest, bool) {
	return r.takePendingPiRequestForResponse(requestID, false)
}

func (r *activeRun) takePendingPiRequestForResponse(requestID string, markResponsePending bool) (*pendingServerRequest, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	normalizedID := strings.TrimSpace(requestID)
	if r.pendingServerReq == nil || normalizedID == "" ||
		strings.TrimSpace(r.pendingServerReq.PiRequestID) != normalizedID {
		return nil, false
	}
	request := r.pendingServerReq.clone()
	r.pendingServerReq = nil
	if markResponsePending {
		r.piResponseGeneration++
		if r.piResponseGeneration == 0 {
			r.piResponseGeneration++
		}
		r.piResponsePending = r.piResponseGeneration
		r.piResponseHistoryDone = make(chan struct{})
		r.piResponseHistoryComplete = 0
		r.piResponseHistoryErr = nil
		r.piResponseRequest = request.clone()
		request.PiResponseGeneration = r.piResponseGeneration
		r.piResponseRequest.PiResponseGeneration = r.piResponseGeneration
	}
	return request, true
}

func (r *activeRun) markCompletedPlanTool() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.completedPlanTool = true
}

func (r *activeRun) markCodexTransportRetry() {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.transportRetrySeen = true
	r.transportRetryRecovered = false
	r.mu.Unlock()
}

func (r *activeRun) codexTransportRetrySeen() bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.transportRetrySeen
}

func (r *activeRun) claimCodexTransportRetryRecovery() bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.transportRetrySeen || r.transportRetryRecovered {
		return false
	}
	r.transportRetryRecovered = true
	return true
}

func (r *activeRun) releaseCodexTransportRetryRecovery() {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.transportRetrySeen {
		r.transportRetryRecovered = false
	}
	r.mu.Unlock()
}

func (r *activeRun) beginClaudeCompaction(toolID string, startedAt time.Time) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if strings.TrimSpace(r.claudeCompactionToolID) != "" {
		return false
	}
	r.claudeCompactionToolID = strings.TrimSpace(toolID)
	r.claudeCompactionStarted = startedAt
	return r.claudeCompactionToolID != ""
}

func (r *activeRun) claudeCompactionState() (string, time.Time) {
	if r == nil {
		return "", time.Time{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.claudeCompactionToolID, r.claudeCompactionStarted
}

func (r *activeRun) clearClaudeCompaction() string {
	if r == nil {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	id := r.claudeCompactionToolID
	r.claudeCompactionToolID = ""
	r.claudeCompactionStarted = time.Time{}
	return id
}

func (r *activeRun) completedPlanToolSeen() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.completedPlanTool
}

func (r *activeRun) clearCompletedPlanTool() {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.completedPlanTool = false
	r.mu.Unlock()
}

func (r *activeRun) hasPendingServerRequest() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.pendingServerReq != nil
}

func (r *activeRun) recordAssistantMessageStarted(messageID string, phase string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return
	}
	if r.assistantMessagePhases == nil {
		r.assistantMessagePhases = make(map[string]string)
	}
	r.assistantMessagePhases[messageID] = strings.ToLower(strings.TrimSpace(phase))
}

func (r *activeRun) setAssistantMessageID(messageID string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.assistantMessageID = strings.TrimSpace(messageID)
	r.mu.Unlock()
}

func (r *activeRun) assistantMessageIDSnapshot() string {
	if r == nil {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.assistantMessageID
}

func (r *activeRun) currentToolMessageSnapshot() string {
	if r == nil {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.TrimSpace(r.currentToolMessage)
}

func (r *activeRun) setCodexRolloutMonitor(monitor *codexRolloutMonitor) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.codexRolloutMonitor = monitor
	r.mu.Unlock()
}

func (r *activeRun) codexRolloutMonitorSnapshot() *codexRolloutMonitor {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.codexRolloutMonitor
}

func (r *activeRun) stopCodexRolloutMonitor() {
	monitor := r.codexRolloutMonitorSnapshot()
	if monitor == nil {
		return
	}
	monitor.stopAndDrain()
	r.mu.Lock()
	if r.codexRolloutMonitor == monitor {
		r.codexRolloutMonitor = nil
	}
	r.mu.Unlock()
}

func (r *activeRun) recordAssistantMessageDelta(messageID string, text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return
	}
	if r.assistantMessageText == nil {
		r.assistantMessageText = make(map[string]bool)
	}
	r.assistantMessageText[messageID] = true
}

func (r *activeRun) recordAssistantMessageCompleted(messageID string, phase string, text string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return
	}
	if r.assistantMessagePhases == nil {
		r.assistantMessagePhases = make(map[string]string)
	}
	normalizedPhase := strings.ToLower(strings.TrimSpace(phase))
	if normalizedPhase == "" {
		normalizedPhase = r.assistantMessagePhases[messageID]
	} else {
		r.assistantMessagePhases[messageID] = normalizedPhase
	}
	hasText := strings.TrimSpace(text) != "" || r.assistantMessageText[messageID]
	if normalizedPhase != "commentary" && hasText {
		r.completedReply = true
	}
}

func (r *activeRun) completedReplySeen() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.completedReply
}

func (r *activeRun) resetCodexTurnCompletionEvidence() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.assistantMessageID = ""
	r.assistantDeltaSeen = nil
	r.assistantMessagePhases = nil
	r.assistantMessageText = nil
	r.completedReply = false
}

func (r *activeRun) markAssistantDeltaSeen(messageID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.assistantDeltaSeen == nil {
		r.assistantDeltaSeen = make(map[string]bool)
	}
	if strings.TrimSpace(messageID) == "" {
		return
	}
	r.assistantDeltaSeen[messageID] = true
}

func (r *activeRun) assistantDeltaWasSeen(messageID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.assistantDeltaSeen != nil && r.assistantDeltaSeen[strings.TrimSpace(messageID)]
}
