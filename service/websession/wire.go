package websession

import (
	"encoding/json"
	"strconv"
	"time"
)

const protocolVersion = 1

const (
	wireKindCommand   = "cmd"
	wireKindAck       = "ack"
	wireKindEvent     = "evt"
	wireKindError     = "err"
	wireKindHeartbeat = "hb"
)

const (
	wireOpSession        = "session"
	wireOpHistoryPage    = "hist_page"
	wireOpHistoryItem    = "hist_item"
	wireOpPending        = "pending"
	wireOpScheduled      = "scheduled"
	wireOpSubAgent       = "sub_agent"
	wireOpResyncRequired = "resync_required"
)

type resyncReason string

const (
	resyncReasonHistoryReconciled resyncReason = "history_reconciled"
	resyncReasonTreeNavigation    resyncReason = "tree_navigation"
	resyncReasonRuntimeReconciled resyncReason = "runtime_reconciled"
)

// Wire payloads intentionally use short keys to reduce websocket overhead.
// Keep business logic on the semantic structs in types.go and only map to/from
// these short keys at the protocol boundary.
type wireCommandFrame struct {
	Version   int             `json:"v"`
	Kind      string          `json:"k"`
	RequestID string          `json:"rid"`
	SessionID string          `json:"sid,omitempty"`
	Operation string          `json:"op"`
	Payload   json.RawMessage `json:"p,omitempty"`
}

type wireHeartbeatFrame struct {
	Version   int    `json:"v"`
	Kind      string `json:"k"`
	Timestamp int64  `json:"ts"`
	Operation string `json:"op"`
	SessionID string `json:"sid,omitempty"`
}

type wireFrame struct {
	Version        int                  `json:"v"`
	Kind           string               `json:"k"`
	RequestID      string               `json:"rid,omitempty"`
	SessionID      string               `json:"sid,omitempty"`
	Timestamp      int64                `json:"ts"`
	Revision       string               `json:"rev,omitempty"`
	PendingEpoch   string               `json:"pe,omitempty"`
	PendingVersion *uint64              `json:"pv,omitempty"`
	Operation      string               `json:"op,omitempty"`
	Payload        any                  `json:"p,omitempty"`
	OK             *int                 `json:"ok,omitempty"`
	Session        *wireSess            `json:"s,omitempty"`
	History        *wireHist            `json:"h,omitempty"`
	Item           *wireHistItem        `json:"i,omitempty"`
	Pending        *[]wirePendingInput  `json:"pi,omitempty"`
	Scheduled      []wireScheduledInput `json:"si,omitempty"`
	SubAgent       *wireSubAgent        `json:"ag,omitempty"`
	Code           string               `json:"code,omitempty"`
	Message        string               `json:"msg,omitempty"`
	Retry          bool                 `json:"retry,omitempty"`
}

type wireSess struct {
	ID                                string         `json:"id"`
	Revision                          string         `json:"rev,omitempty"`
	ProjectID                         string         `json:"pid"`
	WorktreeID                        *string        `json:"wid,omitempty"`
	OrderIndex                        float64        `json:"oi"`
	Agent                             string         `json:"ag"`
	ClaudeRuntime                     string         `json:"cr,omitempty"`
	Model                             string         `json:"md"`
	ReasoningEffort                   string         `json:"re"`
	WorkflowMode                      string         `json:"wm"`
	PermissionLevel                   string         `json:"pl"`
	ActiveCallTimeoutEnabled          bool           `json:"acte"`
	AutoRetryEnabled                  bool           `json:"ae"`
	AutoRetryPolicyMode               string         `json:"arpm"`
	AutoRetryScope                    string         `json:"ars"`
	AutoRetryPreset                   string         `json:"arp"`
	AutoRetryMaxAttempts              int            `json:"aram"`
	AutoRetryDispatchPendingOnFailure bool           `json:"ardpf"`
	Title                             string         `json:"ttl"`
	Cwd                               string         `json:"cwd"`
	NativeSessionID                   *string        `json:"nsid,omitempty"`
	NativeLeafID                      *string        `json:"nlid,omitempty"`
	SourceRevision                    *string        `json:"srev,omitempty"`
	CyberPolicyFlagged                bool           `json:"cpf,omitempty"`
	HasScheduledPlanExecution         bool           `json:"spe,omitempty"`
	Status                            string         `json:"st"`
	AssistantState                    string         `json:"ast,omitempty"`
	Unread                            bool           `json:"unr"`
	ArchivedAt                        *int64         `json:"aa,omitempty"`
	ActivityAt                        int64          `json:"act"`
	StatusUpdatedAt                   *int64         `json:"sta,omitempty"`
	CreatedAt                         int64          `json:"ca"`
	LastUpdated                       int64          `json:"lu"`
	LastMessageAt                     *int64         `json:"lma,omitempty"`
	AssistantStateUpdatedAt           *int64         `json:"asu,omitempty"`
	SourceKind                        string         `json:"sk"`
	SyncState                         string         `json:"ss"`
	LastSyncMode                      string         `json:"lsm,omitempty"`
	SourceCreatedAt                   *int64         `json:"sca,omitempty"`
	SourceUpdatedAt                   *int64         `json:"sua,omitempty"`
	LastSyncedAt                      *int64         `json:"lsa,omitempty"`
	ThreadPath                        *string        `json:"tp,omitempty"`
	ThreadPreview                     *string        `json:"tpv,omitempty"`
	TurnCount                         int            `json:"tc"`
	ItemCount                         int            `json:"ic"`
	SyncError                         *string        `json:"se,omitempty"`
	Usage                             wireUsage      `json:"usa"`
	LatestTurnUsage                   *wireCtxEst    `json:"ltu,omitempty"`
	ContextEstimate                   wireCtxEst     `json:"cea"`
	ContextEstimateMode               string         `json:"cem"`
	LastContextCompactionAt           *int64         `json:"lcca,omitempty"`
	Cost                              float64        `json:"cost"`
	ContextWindowTokens               *int64         `json:"cwt,omitempty"`
	ContextWindowSource               string         `json:"cws"`
	Goal                              *wireGoal      `json:"goal,omitempty"`
	WorkTiming                        wireWorkTiming `json:"wt"`
}

type wireWorkTiming struct {
	CompletedDurationMs int64                     `json:"dur"`
	CurrentRun          *wireWorkTimingCurrentRun `json:"cur,omitempty"`
	BackfillState       string                    `json:"bs"`
	BackfillVersion     int                       `json:"bv"`
}

type wireWorkTimingCurrentRun struct {
	ID               string `json:"id"`
	StartedAt        int64  `json:"sa"`
	PausedAt         *int64 `json:"pa,omitempty"`
	PausedDurationMs int64  `json:"pd"`
}

type wireGoal struct {
	ThreadID        string `json:"tid"`
	Objective       string `json:"obj"`
	Status          string `json:"st"`
	TokenBudget     *int64 `json:"tb,omitempty"`
	TokensUsed      int64  `json:"tu"`
	TimeUsedSeconds int64  `json:"tsu"`
	CreatedAt       int64  `json:"ca"`
	UpdatedAt       int64  `json:"ua"`
}

type wireUsage struct {
	InputTokens       int64 `json:"in"`
	CachedInputTokens int64 `json:"cin"`
	OutputTokens      int64 `json:"out"`
}

type wireCtxEst struct {
	InputTokens       int64 `json:"in"`
	CachedInputTokens int64 `json:"cin"`
	OutputTokens      int64 `json:"out"`
	UsedTokens        int64 `json:"usd"`
}

type wireHist struct {
	Items        []wireHistItem `json:"its"`
	HasMore      bool           `json:"hm"`
	BeforeCursor string         `json:"bc,omitempty"`
	Total        int            `json:"tot"`
}

type wireHistItem struct {
	ID             string              `json:"id"`
	SourceThreadID *string             `json:"sthid,omitempty"`
	SourceTurnID   *string             `json:"stid,omitempty"`
	SourceItemID   *string             `json:"siid,omitempty"`
	RunID          *string             `json:"rid,omitempty"`
	RunDurationMs  *int64              `json:"dur,omitempty"`
	RunOutcome     string              `json:"out,omitempty"`
	OrderIndex     int64               `json:"oi"`
	Kind           string              `json:"kd"`
	ItemType       string              `json:"tp"`
	Text           string              `json:"txt,omitempty"`
	Timestamp      *int64              `json:"ts2,omitempty"`
	ObservedAt     *int64              `json:"obs,omitempty"`
	Attachments    []wireHistoryAttach `json:"atts,omitempty"`
	Tool           *wireHistoryTool    `json:"tl,omitempty"`
	Level          string              `json:"lvl,omitempty"`
	Done           bool                `json:"dn,omitempty"`
	Detail         *wireHistoryDetail  `json:"dt,omitempty"`
	Payload        map[string]any      `json:"pl,omitempty"`
}

type wireSubAgent struct {
	ThreadID         string  `json:"tid"`
	ParentThreadID   *string `json:"ptid,omitempty"`
	Path             string  `json:"p,omitempty"`
	Nickname         string  `json:"nn,omitempty"`
	Role             string  `json:"rl,omitempty"`
	Status           string  `json:"st"`
	Summary          string  `json:"sm,omitempty"`
	CurrentTurnID    *string `json:"ctid,omitempty"`
	LatestItemID     *string `json:"liid,omitempty"`
	LatestOrderIndex int64   `json:"loi,omitempty"`
	StartedAt        *int64  `json:"sa,omitempty"`
	LastActivityAt   *int64  `json:"la,omitempty"`
	EndedAt          *int64  `json:"ea,omitempty"`
}

type wireHistoryAttach struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Mime string `json:"mime,omitempty"`
	Size int64  `json:"sz,omitempty"`
	Path string `json:"path,omitempty"`
}

type wireHistoryTool struct {
	ID           string                   `json:"id"`
	Name         string                   `json:"name"`
	Kind         string                   `json:"kind,omitempty"`
	Input        any                      `json:"in,omitempty"`
	Output       string                   `json:"out,omitempty"`
	Status       string                   `json:"st"`
	Meta         map[string]any           `json:"meta,omitempty"`
	CommandGroup *wireHistoryCommandGroup `json:"cg,omitempty"`
}

type wireHistoryCommandGroup struct {
	ID           string `json:"id"`
	Count        int    `json:"count"`
	FirstSeq     int64  `json:"firstSeq,omitempty"`
	LastSeq      int64  `json:"lastSeq,omitempty"`
	LatestToolID string `json:"latestToolId,omitempty"`
	Compacted    bool   `json:"compacted,omitempty"`
}

type wireHistoryDetail struct {
	Type         string                `json:"type"`
	Prompt       string                `json:"prompt,omitempty"`
	ApprovalKind string                `json:"approvalKind,omitempty"`
	Command      string                `json:"command,omitempty"`
	Questions    []toolRequestQuestion `json:"questions,omitempty"`
	Answers      []HistoryAnswerEntry  `json:"answers,omitempty"`
	Action       string                `json:"action,omitempty"`
}

type wirePendingInput struct {
	ID            string   `json:"id"`
	Mode          string   `json:"m"`
	Text          string   `json:"txt,omitempty"`
	AttachmentIDs []string `json:"atts,omitempty"`
	ReadyAt       *int64   `json:"ra,omitempty"`
	Paused        bool     `json:"ps,omitempty"`
	NativeQueued  bool     `json:"nq,omitempty"`
	Status        string   `json:"st,omitempty"`
	AttemptCount  int      `json:"ac,omitempty"`
	LastError     string   `json:"err,omitempty"`
	LastErrorCode string   `json:"ec,omitempty"`
	CreatedAt     int64    `json:"ca"`
}

type wireScheduledInput struct {
	ID              string   `json:"id"`
	Action          string   `json:"a,omitempty"`
	TargetID        string   `json:"tid,omitempty"`
	Mode            string   `json:"m"`
	ExitPlanMode    bool     `json:"epm,omitempty"`
	Text            string   `json:"txt,omitempty"`
	AttachmentIDs   []string `json:"atts,omitempty"`
	ScheduleKind    string   `json:"sk,omitempty"`
	ScheduledFor    *int64   `json:"sf,omitempty"`
	IdleSince       *int64   `json:"is,omitempty"`
	BlockingReasons []string `json:"br,omitempty"`
	ConditionError  string   `json:"ce,omitempty"`
	Status          string   `json:"st"`
	LastError       string   `json:"err,omitempty"`
	CreatedAt       int64    `json:"ca"`
	UpdatedAt       int64    `json:"ua"`
	SentAt          *int64   `json:"sa,omitempty"`
	CanceledAt      *int64   `json:"xa,omitempty"`
}

type wireResyncRequiredPayload struct {
	Reason resyncReason `json:"reason,omitempty"`
}

type wireGoalPayload struct {
	Goal *wireGoal `json:"goal"`
}

func newAckFrame(requestID, op, sessionID string, payload any) wireFrame {
	ok := 1
	return wireFrame{
		Version:   protocolVersion,
		Kind:      wireKindAck,
		RequestID: requestID,
		SessionID: sessionID,
		Timestamp: nowUnixMilli(),
		Operation: op,
		Payload:   payload,
		OK:        &ok,
	}
}

func newRevisionAckFrame(requestID, op, sessionID, revision string, payload any) wireFrame {
	frame := newAckFrame(requestID, op, sessionID, payload)
	frame.Revision = revision
	return frame
}

func applyPendingState(frame *wireFrame, epoch string, version uint64, items []PendingInput) {
	if frame == nil {
		return
	}
	wireItems := mapWirePendingInputs(items)
	frame.PendingEpoch = epoch
	frame.PendingVersion = ptr(version)
	frame.Pending = &wireItems
}

func newPendingAckFrame(
	requestID, op, sessionID, epoch string,
	version uint64,
	items []PendingInput,
	payload any,
) wireFrame {
	frame := newAckFrame(requestID, op, sessionID, payload)
	applyPendingState(&frame, epoch, version, items)
	return frame
}

func newErrorFrame(requestID, sessionID, code, message string, retry bool) wireFrame {
	return wireFrame{
		Version:   protocolVersion,
		Kind:      wireKindError,
		RequestID: requestID,
		SessionID: sessionID,
		Timestamp: nowUnixMilli(),
		Code:      code,
		Message:   message,
		Retry:     retry,
	}
}

func newHeartbeatFrame(op string) wireFrame {
	return wireFrame{
		Version:   protocolVersion,
		Kind:      wireKindHeartbeat,
		Timestamp: nowUnixMilli(),
		Operation: op,
	}
}

func newResyncRequiredFrame(sessionID string, reason resyncReason) wireFrame {
	return wireFrame{
		Version:   protocolVersion,
		Kind:      wireKindEvent,
		SessionID: sessionID,
		Timestamp: nowUnixMilli(),
		Operation: wireOpResyncRequired,
		Payload:   wireResyncRequiredPayload{Reason: reason},
	}
}

func newHistoryPageFrame(sessionID string, window HistoryWindow) wireFrame {
	wireHistory := make([]wireHistItem, 0, len(window.Items))
	for _, item := range window.Items {
		wireHistory = append(wireHistory, mapWireHistoryItem(item))
	}
	return wireFrame{
		Version:   protocolVersion,
		Kind:      wireKindEvent,
		SessionID: sessionID,
		Timestamp: nowUnixMilli(),
		Operation: wireOpHistoryPage,
		History: &wireHist{
			Items:        wireHistory,
			HasMore:      window.HasMore,
			BeforeCursor: window.BeforeCursor,
			Total:        window.Total,
		},
	}
}

func newHistoryItemFrame(sessionID string, item HistoryItem, summary *SessionSummary) wireFrame {
	frame := wireFrame{
		Version:   protocolVersion,
		Kind:      wireKindEvent,
		SessionID: sessionID,
		Timestamp: nowUnixMilli(),
		Operation: wireOpHistoryItem,
		Item:      ptr(mapWireHistoryItem(item)),
	}
	if summary != nil {
		frame.Session = mapWireSession(*summary)
	}
	return frame
}

func newSubAgentFrame(sessionID string, agent WebSessionSubAgent, summary *SessionSummary) wireFrame {
	frame := wireFrame{
		Version:   protocolVersion,
		Kind:      wireKindEvent,
		SessionID: sessionID,
		Timestamp: nowUnixMilli(),
		Operation: wireOpSubAgent,
		SubAgent:  ptr(mapWireSubAgent(agent)),
	}
	if summary != nil {
		frame.Session = mapWireSession(*summary)
	}
	return frame
}

func newSessionFrame(sessionID string, summary SessionSummary) wireFrame {
	return wireFrame{
		Version:   protocolVersion,
		Kind:      wireKindEvent,
		SessionID: sessionID,
		Timestamp: nowUnixMilli(),
		Operation: wireOpSession,
		Session:   mapWireSession(summary),
	}
}

func newPendingFrame(sessionID, epoch string, version uint64, items []PendingInput) wireFrame {
	frame := wireFrame{
		Version:   protocolVersion,
		Kind:      wireKindEvent,
		SessionID: sessionID,
		Timestamp: nowUnixMilli(),
		Operation: wireOpPending,
	}
	applyPendingState(&frame, epoch, version, items)
	return frame
}

func newScheduledFrame(sessionID string, items []ScheduledInput) wireFrame {
	return wireFrame{
		Version:   protocolVersion,
		Kind:      wireKindEvent,
		SessionID: sessionID,
		Timestamp: nowUnixMilli(),
		Operation: wireOpScheduled,
		Scheduled: mapWireScheduledInputs(items),
	}
}

func mapWireSession(session SessionSummary) *wireSess {
	var lastMessageAt *int64
	if session.LastMessageAt != nil {
		value := session.LastMessageAt.UnixMilli()
		lastMessageAt = &value
	}
	var statusUpdatedAt *int64
	if session.StatusUpdatedAt != nil {
		value := session.StatusUpdatedAt.UnixMilli()
		statusUpdatedAt = &value
	}
	var assistantStateUpdatedAt *int64
	if session.AssistantStateUpdatedAt != nil {
		value := session.AssistantStateUpdatedAt.UnixMilli()
		assistantStateUpdatedAt = &value
	}
	var archivedAt *int64
	if session.ArchivedAt != nil {
		value := session.ArchivedAt.UnixMilli()
		archivedAt = &value
	}
	var sourceCreatedAt *int64
	if session.SourceCreatedAt != nil {
		value := session.SourceCreatedAt.UnixMilli()
		sourceCreatedAt = &value
	}
	var sourceUpdatedAt *int64
	if session.SourceUpdatedAt != nil {
		value := session.SourceUpdatedAt.UnixMilli()
		sourceUpdatedAt = &value
	}
	var lastSyncedAt *int64
	if session.LastSyncedAt != nil {
		value := session.LastSyncedAt.UnixMilli()
		lastSyncedAt = &value
	}
	var lastContextCompactionAt *int64
	if session.LastContextCompactionAt != nil {
		value := session.LastContextCompactionAt.UnixMilli()
		lastContextCompactionAt = &value
	}
	wireSession := &wireSess{
		ID:                                session.ID,
		Revision:                          session.Revision,
		ProjectID:                         session.ProjectID,
		WorktreeID:                        session.WorktreeID,
		OrderIndex:                        session.OrderIndex,
		Agent:                             string(session.Agent),
		ClaudeRuntime:                     string(session.ClaudeRuntime),
		Model:                             session.Model,
		ReasoningEffort:                   string(session.ReasoningEffort),
		WorkflowMode:                      string(session.WorkflowMode),
		PermissionLevel:                   string(session.PermissionLevel),
		ActiveCallTimeoutEnabled:          session.ActiveCallTimeoutEnabled,
		AutoRetryEnabled:                  session.AutoRetryEnabled,
		AutoRetryPolicyMode:               string(session.AutoRetryPolicyMode),
		AutoRetryScope:                    string(session.AutoRetryScope),
		AutoRetryPreset:                   string(session.AutoRetryPreset),
		AutoRetryMaxAttempts:              session.AutoRetryMaxAttempts,
		AutoRetryDispatchPendingOnFailure: session.AutoRetryDispatchPendingOnFailure,
		Title:                             session.Title,
		Cwd:                               session.Cwd,
		NativeSessionID:                   session.NativeSessionID,
		NativeLeafID:                      session.NativeLeafID,
		SourceRevision:                    session.SourceRevision,
		CyberPolicyFlagged:                session.CyberPolicyFlagged,
		HasScheduledPlanExecution:         session.HasScheduledPlanExecution,
		Status:                            string(session.Status),
		AssistantState:                    string(session.AssistantState),
		Unread:                            session.HasUnread,
		ArchivedAt:                        archivedAt,
		ActivityAt:                        session.ActivityAt.UnixMilli(),
		StatusUpdatedAt:                   statusUpdatedAt,
		CreatedAt:                         session.CreatedAt.UnixMilli(),
		LastUpdated:                       session.UpdatedAt.UnixMilli(),
		LastMessageAt:                     lastMessageAt,
		AssistantStateUpdatedAt:           assistantStateUpdatedAt,
		SourceKind:                        session.SourceKind,
		SyncState:                         string(session.SyncState),
		LastSyncMode:                      string(session.LastSyncMode),
		SourceCreatedAt:                   sourceCreatedAt,
		SourceUpdatedAt:                   sourceUpdatedAt,
		LastSyncedAt:                      lastSyncedAt,
		ThreadPath:                        session.ThreadPath,
		ThreadPreview:                     session.ThreadPreview,
		TurnCount:                         session.TurnCount,
		ItemCount:                         session.ItemCount,
		SyncError:                         session.SyncError,
		Usage: wireUsage{
			InputTokens:       session.Usage.InputTokens,
			CachedInputTokens: session.Usage.CachedInputTokens,
			OutputTokens:      session.Usage.OutputTokens,
		},
		ContextEstimate: wireCtxEst{
			InputTokens:       session.ContextEstimate.InputTokens,
			CachedInputTokens: session.ContextEstimate.CachedInputTokens,
			OutputTokens:      session.ContextEstimate.OutputTokens,
			UsedTokens:        session.ContextEstimate.UsedTokens,
		},
		ContextEstimateMode:     string(session.ContextEstimateMode),
		LastContextCompactionAt: lastContextCompactionAt,
		Cost:                    session.Usage.Cost,
		ContextWindowTokens:     session.ContextWindowTokens,
		ContextWindowSource:     string(session.ContextWindowSource),
		Goal:                    mapWireGoal(session.Goal),
		WorkTiming:              mapWireWorkTiming(session.WorkTiming),
	}
	if session.LatestTurnUsage.InputTokens > 0 ||
		session.LatestTurnUsage.CachedInputTokens > 0 ||
		session.LatestTurnUsage.OutputTokens > 0 ||
		session.LatestTurnUsage.UsedTokens > 0 {
		wireSession.LatestTurnUsage = &wireCtxEst{
			InputTokens:       session.LatestTurnUsage.InputTokens,
			CachedInputTokens: session.LatestTurnUsage.CachedInputTokens,
			OutputTokens:      session.LatestTurnUsage.OutputTokens,
			UsedTokens:        session.LatestTurnUsage.UsedTokens,
		}
	}
	return wireSession
}

func mapWireWorkTiming(timing WorkTiming) wireWorkTiming {
	result := wireWorkTiming{
		CompletedDurationMs: timing.CompletedDurationMs,
		BackfillState:       string(timing.BackfillState),
		BackfillVersion:     timing.BackfillVersion,
	}
	if timing.CurrentRun == nil {
		return result
	}
	var pausedAt *int64
	if timing.CurrentRun.PausedAt != nil {
		value := timing.CurrentRun.PausedAt.UnixMilli()
		pausedAt = &value
	}
	result.CurrentRun = &wireWorkTimingCurrentRun{
		ID:               timing.CurrentRun.ID,
		StartedAt:        timing.CurrentRun.StartedAt.UnixMilli(),
		PausedAt:         pausedAt,
		PausedDurationMs: timing.CurrentRun.PausedDurationMs,
	}
	return result
}

func mapWireGoal(goal *SessionGoal) *wireGoal {
	if goal == nil {
		return nil
	}
	return &wireGoal{
		ThreadID:        goal.ThreadID,
		Objective:       goal.Objective,
		Status:          string(goal.Status),
		TokenBudget:     goal.TokenBudget,
		TokensUsed:      goal.TokensUsed,
		TimeUsedSeconds: goal.TimeUsedSeconds,
		CreatedAt:       goal.CreatedAt.UnixMilli(),
		UpdatedAt:       goal.UpdatedAt.UnixMilli(),
	}
}

func mapWirePendingInputs(items []PendingInput) []wirePendingInput {
	if len(items) == 0 {
		return []wirePendingInput{}
	}
	wireItems := make([]wirePendingInput, 0, len(items))
	for _, item := range items {
		var readyAt *int64
		if item.ReadyAt != nil {
			value := item.ReadyAt.UnixMilli()
			readyAt = &value
		}
		wireItems = append(wireItems, wirePendingInput{
			ID:            item.ID,
			Mode:          string(item.Mode),
			Text:          item.Text,
			AttachmentIDs: append([]string(nil), item.AttachmentIDs...),
			ReadyAt:       readyAt,
			Paused:        item.Paused,
			NativeQueued:  item.NativeQueued,
			Status:        string(item.Status),
			AttemptCount:  item.AttemptCount,
			LastError:     item.LastError,
			LastErrorCode: item.LastErrorCode,
			CreatedAt:     item.CreatedAt.UnixMilli(),
		})
	}
	return wireItems
}

func mapWireScheduledInputs(items []ScheduledInput) []wireScheduledInput {
	if len(items) == 0 {
		return []wireScheduledInput{}
	}
	wireItems := make([]wireScheduledInput, 0, len(items))
	for _, item := range items {
		var scheduledFor *int64
		if item.ScheduledFor != nil {
			value := item.ScheduledFor.UnixMilli()
			scheduledFor = &value
		}
		var idleSince *int64
		if item.IdleSince != nil {
			value := item.IdleSince.UnixMilli()
			idleSince = &value
		}
		blockingReasons := make([]string, 0, len(item.BlockingReasons))
		for _, reason := range item.BlockingReasons {
			blockingReasons = append(blockingReasons, string(reason))
		}
		var sentAt *int64
		if item.SentAt != nil {
			value := item.SentAt.UnixMilli()
			sentAt = &value
		}
		var canceledAt *int64
		if item.CanceledAt != nil {
			value := item.CanceledAt.UnixMilli()
			canceledAt = &value
		}
		wireItems = append(wireItems, wireScheduledInput{
			ID:              item.ID,
			Action:          string(item.Action),
			TargetID:        item.TargetID,
			Mode:            string(item.Mode),
			ExitPlanMode:    item.ExitPlanMode,
			Text:            item.Text,
			AttachmentIDs:   append([]string(nil), item.AttachmentIDs...),
			ScheduleKind:    string(item.ScheduleKind),
			ScheduledFor:    scheduledFor,
			IdleSince:       idleSince,
			BlockingReasons: blockingReasons,
			ConditionError:  item.ConditionError,
			Status:          string(item.Status),
			LastError:       item.LastError,
			CreatedAt:       item.CreatedAt.UnixMilli(),
			UpdatedAt:       item.UpdatedAt.UnixMilli(),
			SentAt:          sentAt,
			CanceledAt:      canceledAt,
		})
	}
	return wireItems
}

func mapWireHistoryItem(item HistoryItem) wireHistItem {
	item = historyItemForTransport(item)
	var timestamp *int64
	if item.Timestamp != nil {
		value := item.Timestamp.UnixMilli()
		timestamp = &value
	}
	var observedAt *int64
	if item.ObservedAt != nil {
		value := item.ObservedAt.UnixMilli()
		observedAt = &value
	}
	attachments := make([]wireHistoryAttach, 0, len(item.Attachments))
	for _, attachment := range item.Attachments {
		attachments = append(attachments, wireHistoryAttach(attachment))
	}
	return wireHistItem{
		ID:             item.ID,
		SourceThreadID: item.SourceThreadID,
		SourceTurnID:   item.SourceTurnID,
		SourceItemID:   item.SourceItemID,
		RunID:          item.RunID,
		RunDurationMs:  item.RunDurationMs,
		RunOutcome:     string(item.RunOutcome),
		OrderIndex:     item.OrderIndex,
		Kind:           item.Kind,
		ItemType:       item.ItemType,
		Text:           item.Text,
		Timestamp:      timestamp,
		ObservedAt:     observedAt,
		Attachments:    attachments,
		Tool:           mapWireHistoryTool(item.Tool),
		Level:          item.Level,
		Done:           item.Done,
		Detail:         mapWireHistoryDetail(item.Detail),
		Payload:        item.Payload,
	}
}

func mapWireSubAgent(item WebSessionSubAgent) wireSubAgent {
	return wireSubAgent{
		ThreadID:         item.ThreadID,
		ParentThreadID:   item.ParentThreadID,
		Path:             item.Path,
		Nickname:         item.Nickname,
		Role:             item.Role,
		Status:           string(item.Status),
		Summary:          item.Summary,
		CurrentTurnID:    item.CurrentTurnID,
		LatestItemID:     item.LatestItemID,
		LatestOrderIndex: item.LatestOrderIndex,
		StartedAt:        unixMilliPtr(item.StartedAt),
		LastActivityAt:   unixMilliPtr(item.LastActivityAt),
		EndedAt:          unixMilliPtr(item.EndedAt),
	}
}

func unixMilliPtr(value *time.Time) *int64 {
	if value == nil {
		return nil
	}
	result := value.UnixMilli()
	return &result
}

func parseBeforeCursor(raw json.RawMessage) (*int64, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var payload struct {
		BeforeCursor string `json:"bc"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	if payload.BeforeCursor == "" {
		return nil, nil
	}
	value, err := strconv.ParseInt(payload.BeforeCursor, 10, 64)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func historyCursor(events []Event, hasMore bool) string {
	if !hasMore || len(events) == 0 {
		return ""
	}
	return strconv.FormatInt(events[0].Seq, 10)
}

func nowUnixMilli() int64 {
	return time.Now().UnixMilli()
}

func ptr[T any](value T) *T {
	return &value
}

func mapWireHistoryTool(tool *HistoryTool) *wireHistoryTool {
	if tool == nil {
		return nil
	}
	return &wireHistoryTool{
		ID:           tool.ID,
		Name:         tool.Name,
		Kind:         tool.Kind,
		Input:        tool.Input,
		Output:       tool.Output,
		Status:       tool.Status,
		Meta:         tool.Meta,
		CommandGroup: mapWireHistoryCommandGroup(tool.CommandGroup),
	}
}

func mapWireHistoryCommandGroup(group *HistoryToolCommandGroup) *wireHistoryCommandGroup {
	if group == nil {
		return nil
	}
	return &wireHistoryCommandGroup{
		ID:           group.ID,
		Count:        group.Count,
		FirstSeq:     group.FirstSeq,
		LastSeq:      group.LastSeq,
		LatestToolID: group.LatestToolID,
		Compacted:    group.Compacted,
	}
}

func mapWireHistoryDetail(detail *HistoryDetail) *wireHistoryDetail {
	if detail == nil {
		return nil
	}
	return &wireHistoryDetail{
		Type:         detail.Type,
		Prompt:       detail.Prompt,
		ApprovalKind: detail.ApprovalKind,
		Command:      detail.Command,
		Questions:    detail.Questions,
		Answers:      detail.Answers,
		Action:       detail.Action,
	}
}
