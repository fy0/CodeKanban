package websession

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"code-kanban/model/tables"
	"code-kanban/utils"

	"go.uber.org/zap"
)

const (
	piTreeMaxNodes     = 10000
	piTreePreviewRunes = 240
)

var ErrPiTreeRevisionConflict = errors.New("Pi session tree changed; refresh and try again")

type PiTreePublicError struct {
	Code    string
	Message string
}

func ClassifyPiTreeError(err error) PiTreePublicError {
	if errors.Is(err, ErrPiTreeRevisionConflict) {
		return PiTreePublicError{Code: "conflict", Message: ErrPiTreeRevisionConflict.Error()}
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "active pi web session"),
		strings.Contains(message, "while messages are pending"),
		strings.Contains(message, "session is archived"):
		return PiTreePublicError{Code: "invalid_state", Message: "Pi session tree cannot change in the current session state"}
	case strings.Contains(message, "project agent trust is required"),
		strings.Contains(message, "path is not managed by the project"):
		return PiTreePublicError{Code: "forbidden", Message: "Pi project trust is required"}
	case strings.Contains(message, "is required"),
		strings.Contains(message, "target does not exist"),
		strings.Contains(message, "target is not a user message"),
		strings.Contains(message, "only supported for pi"),
		strings.Contains(message, "requires an existing native session"):
		return PiTreePublicError{Code: "bad_req", Message: "Invalid Pi session tree request"}
	default:
		return PiTreePublicError{Code: "internal", Message: "Pi session tree operation failed"}
	}
}

type PiTreeNode struct {
	ID        string   `json:"id"`
	ParentID  *string  `json:"parentId"`
	Type      string   `json:"type"`
	Role      string   `json:"role,omitempty"`
	Preview   string   `json:"preview,omitempty"`
	Timestamp string   `json:"timestamp,omitempty"`
	Label     string   `json:"label,omitempty"`
	Active    bool     `json:"active"`
	Children  []string `json:"children"`
}

type PiTreeSnapshot struct {
	SessionID string       `json:"sessionId"`
	LeafID    *string      `json:"leafId"`
	Revision  string       `json:"revision"`
	Nodes     []PiTreeNode `json:"nodes"`
}

type PiTreeNavigateInput struct {
	TargetID  string `json:"targetId"`
	Revision  string `json:"revision"`
	Summarize bool   `json:"summarize" default:"false"`
}

type PiTreeNavigateResult struct {
	Tree       PiTreeSnapshot `json:"tree"`
	EditorText string         `json:"editorText,omitempty"`
}

type piHistoryTreeNode struct {
	Entry    piHistoryEntry      `json:"entry"`
	Children []piHistoryTreeNode `json:"children"`
	Label    string              `json:"label"`
}

type piTreeRawNode struct {
	entry    piHistoryEntry
	label    string
	children []string
}

type piBridgeMarkerData struct {
	TargetID  string `json:"targetId"`
	Summarize bool   `json:"summarize"`
	Nonce     string `json:"nonce"`
}

func (m *Manager) GetPiSessionTree(ctx context.Context, sessionID string) (PiTreeSnapshot, error) {
	if m == nil {
		return PiTreeSnapshot{}, errors.New("web session manager is not configured")
	}
	dispatchLock := &m.sessionDispatchLocks[sessionRevisionLockIndex(sessionID)]
	dispatchLock.Lock()
	defer dispatchLock.Unlock()

	session, err := m.piTreeSession(ctx, sessionID)
	if err != nil {
		return PiTreeSnapshot{}, err
	}
	runtime, err := m.getOrStartPiRuntime(ctx, session)
	if err != nil {
		return PiTreeSnapshot{}, err
	}
	defer runtime.scheduleIdle()
	return m.readPiTreeSnapshot(ctx, runtime, session)
}

func (m *Manager) NavigatePiSessionTree(
	ctx context.Context,
	sessionID string,
	input PiTreeNavigateInput,
) (PiTreeNavigateResult, error) {
	if m == nil {
		return PiTreeNavigateResult{}, errors.New("web session manager is not configured")
	}
	dispatchLock := &m.sessionDispatchLocks[sessionRevisionLockIndex(sessionID)]
	dispatchLock.Lock()
	defer dispatchLock.Unlock()

	session, err := m.piTreeSession(ctx, sessionID)
	if err != nil {
		return PiTreeNavigateResult{}, err
	}
	if m.hasActiveRun(session.ID) {
		return PiTreeNavigateResult{}, errors.New("cannot navigate an active Pi web session")
	}
	if len(m.pendingInputsDisplaySnapshot(session.ID)) > 0 {
		return PiTreeNavigateResult{}, errors.New("cannot navigate while messages are pending")
	}
	targetID := strings.TrimSpace(input.TargetID)
	if targetID == "" {
		return PiTreeNavigateResult{}, errors.New("Pi tree target id is required")
	}
	expectedRevision := strings.TrimSpace(input.Revision)
	if expectedRevision == "" {
		return PiTreeNavigateResult{}, errors.New("Pi tree revision is required")
	}

	runtime, err := m.getOrStartPiRuntime(ctx, session)
	if err != nil {
		return PiTreeNavigateResult{}, err
	}
	navigationSent := false
	navigationComplete := false
	defer func() {
		if navigationSent && !navigationComplete {
			runtime.stop(errors.New("Pi tree navigation did not reach its verified completion boundary"))
			return
		}
		runtime.scheduleIdle()
	}()
	current, rawCurrent, err := m.readPiTreeSnapshotRaw(ctx, runtime, session)
	if err != nil {
		return PiTreeNavigateResult{}, err
	}
	if current.Revision != expectedRevision {
		return PiTreeNavigateResult{}, ErrPiTreeRevisionConflict
	}
	target, ok := rawCurrent[targetID]
	if !ok || isPiBridgeMarker(target.entry) {
		return PiTreeNavigateResult{}, errors.New("Pi tree target does not exist")
	}

	nonce := utils.NewID()
	payload, err := json.Marshal(piBridgeMarkerData{TargetID: targetID, Summarize: input.Summarize, Nonce: nonce})
	if err != nil {
		return PiTreeNavigateResult{}, err
	}
	message := "/" + piBridgeCommandName + " " + base64.RawURLEncoding.EncodeToString(payload)
	operationCtx, cancel := context.WithTimeout(context.Background(), piRPCRequestTimeout)
	defer cancel()
	navigationSent = true
	if err := runtime.client.Request(operationCtx, "prompt", map[string]any{"message": message}, nil); err != nil {
		return PiTreeNavigateResult{}, fmt.Errorf("start Pi tree navigation: %w", err)
	}

	entries, marker, err := waitForPiBridgeMarker(operationCtx, runtime.client, nonce)
	if err != nil {
		return PiTreeNavigateResult{}, err
	}
	if err := validatePiNavigationMarker(target.entry, marker, entries.Entries, input.Summarize); err != nil {
		return PiTreeNavigateResult{}, err
	}

	fresh, rawFresh, err := m.readPiTreeSnapshotRaw(operationCtx, runtime, session)
	if err != nil {
		return PiTreeNavigateResult{}, err
	}
	freshMarker, ok := rawFresh[marker.ID]
	freshMarkerData, markerOK := parsePiBridgeMarker(freshMarker.entry)
	if !ok || !markerOK || freshMarkerData.Nonce != nonce ||
		pointerString(entries.LeafID) != marker.ID || pointerString(freshMarker.entry.ParentID) != pointerString(marker.ParentID) {
		return PiTreeNavigateResult{}, errors.New("Pi tree navigation marker does not match the active leaf")
	}
	if pointerString(fresh.LeafID) != pointerString(marker.ParentID) {
		return PiTreeNavigateResult{}, errors.New("Pi tree navigation produced an unexpected logical leaf")
	}

	if err := m.projectPiHistoryEntries(operationCtx, session, entries); err != nil {
		return PiTreeNavigateResult{}, err
	}
	navigationComplete = true
	if err := m.broadcastResyncRequired(context.Background(), session.ID, resyncReasonTreeNavigation); err != nil && m.logger != nil {
		m.logger.Warn("failed to broadcast Pi tree navigation resync",
			zap.String("sessionId", session.ID),
			zap.Error(err),
		)
	}
	return PiTreeNavigateResult{Tree: fresh, EditorText: piTreeEditorText(target.entry)}, nil
}

func (m *Manager) piTreeSession(ctx context.Context, sessionID string) (tables.WebSessionTable, error) {
	session, err := m.GetSession(ctx, strings.TrimSpace(sessionID))
	if err != nil {
		return tables.WebSessionTable{}, err
	}
	if session.ArchivedAt != nil {
		return tables.WebSessionTable{}, errors.New("session is archived")
	}
	if normalizeAgent(Agent(session.Agent)) != AgentPi || effectiveSessionBackend(session) != SessionBackendPiRPC {
		return tables.WebSessionTable{}, errors.New("session tree is only supported for Pi web sessions")
	}
	if strings.TrimSpace(pointerString(session.NativeSessionID)) == "" || strings.TrimSpace(pointerString(session.ThreadPath)) == "" {
		return tables.WebSessionTable{}, errors.New("Pi session tree requires an existing native session")
	}
	if err := m.EnsureProjectPiTrust(ctx, session.ProjectID, session.Cwd); err != nil {
		return tables.WebSessionTable{}, err
	}
	return session, nil
}

func (m *Manager) readPiTreeSnapshot(
	ctx context.Context,
	runtime *piSessionRuntime,
	session tables.WebSessionTable,
) (PiTreeSnapshot, error) {
	snapshot, _, err := m.readPiTreeSnapshotRaw(ctx, runtime, session)
	return snapshot, err
}

func (m *Manager) readPiTreeSnapshotRaw(
	ctx context.Context,
	runtime *piSessionRuntime,
	session tables.WebSessionTable,
) (PiTreeSnapshot, map[string]piTreeRawNode, error) {
	var response struct {
		Tree   []piHistoryTreeNode `json:"tree"`
		LeafID *string             `json:"leafId"`
	}
	if err := runtime.client.Request(ctx, "get_tree", nil, &response); err != nil {
		return PiTreeSnapshot{}, nil, fmt.Errorf("read Pi session tree: %w", err)
	}
	revision := piSourceRevision(pointerString(session.ThreadPath), pointerString(response.LeafID))
	if revision == "" {
		return PiTreeSnapshot{}, nil, errors.New("Pi session tree revision is unavailable")
	}
	snapshot, raw, err := projectPiTree(pointerString(session.NativeSessionID), revision, response.Tree, pointerString(response.LeafID))
	if err != nil {
		return PiTreeSnapshot{}, nil, err
	}
	return snapshot, raw, nil
}

func projectPiTree(
	sessionID string,
	revision string,
	roots []piHistoryTreeNode,
	rawLeafID string,
) (PiTreeSnapshot, map[string]piTreeRawNode, error) {
	raw := make(map[string]piTreeRawNode)
	order := make([]string, 0)
	type pendingNode struct {
		node           piHistoryTreeNode
		nestedParentID string
		root           bool
	}
	stack := make([]pendingNode, 0, len(roots))
	for index := len(roots) - 1; index >= 0; index-- {
		stack = append(stack, pendingNode{node: roots[index], root: true})
	}
	for len(stack) > 0 {
		last := len(stack) - 1
		current := stack[last]
		stack = stack[:last]
		entry := current.node.Entry
		entry.ID = strings.TrimSpace(entry.ID)
		entry.Type = strings.TrimSpace(entry.Type)
		if entry.ID == "" || entry.Type == "" {
			return PiTreeSnapshot{}, nil, errors.New("Pi session tree contains an invalid node")
		}
		if len(raw) >= piTreeMaxNodes {
			return PiTreeSnapshot{}, nil, errors.New("Pi session tree exceeds the node limit")
		}
		if _, duplicate := raw[entry.ID]; duplicate {
			return PiTreeSnapshot{}, nil, errors.New("Pi session tree contains a duplicate node id")
		}
		parentID := strings.TrimSpace(pointerString(entry.ParentID))
		if current.root {
			if parentID != "" {
				return PiTreeSnapshot{}, nil, errors.New("Pi session tree contains an orphan root")
			}
		} else if parentID == "" || parentID != current.nestedParentID {
			return PiTreeSnapshot{}, nil, errors.New("Pi session tree parent linkage is invalid")
		}
		children := make([]string, 0, len(current.node.Children))
		for _, child := range current.node.Children {
			children = append(children, strings.TrimSpace(child.Entry.ID))
		}
		raw[entry.ID] = piTreeRawNode{entry: entry, label: strings.TrimSpace(current.node.Label), children: children}
		order = append(order, entry.ID)
		for index := len(current.node.Children) - 1; index >= 0; index-- {
			stack = append(stack, pendingNode{node: current.node.Children[index], nestedParentID: entry.ID})
		}
	}

	rawLeafID = strings.TrimSpace(rawLeafID)
	if len(raw) == 0 {
		if rawLeafID != "" {
			return PiTreeSnapshot{}, nil, errors.New("Pi session tree leaf is missing")
		}
		return PiTreeSnapshot{SessionID: strings.TrimSpace(sessionID), Revision: revision, Nodes: []PiTreeNode{}}, raw, nil
	}
	if _, ok := raw[rawLeafID]; !ok {
		return PiTreeSnapshot{}, nil, errors.New("Pi session tree leaf does not exist")
	}

	logicalLeafID := rawLeafID
	for logicalLeafID != "" && isPiBridgeMarker(raw[logicalLeafID].entry) {
		logicalLeafID = strings.TrimSpace(pointerString(raw[logicalLeafID].entry.ParentID))
	}
	active := make(map[string]struct{})
	seen := make(map[string]struct{})
	for id := logicalLeafID; id != ""; {
		if _, duplicate := seen[id]; duplicate {
			return PiTreeSnapshot{}, nil, errors.New("Pi session tree active path contains a cycle")
		}
		seen[id] = struct{}{}
		node, ok := raw[id]
		if !ok {
			return PiTreeSnapshot{}, nil, errors.New("Pi session tree active path is incomplete")
		}
		if !isPiBridgeMarker(node.entry) {
			active[id] = struct{}{}
		}
		id = strings.TrimSpace(pointerString(node.entry.ParentID))
	}

	nodes := make([]PiTreeNode, 0, len(order))
	byVisibleID := make(map[string]int)
	for _, id := range order {
		node := raw[id]
		if isPiBridgeMarker(node.entry) {
			continue
		}
		parentID := strings.TrimSpace(pointerString(node.entry.ParentID))
		for parentID != "" {
			parent, ok := raw[parentID]
			if !ok {
				return PiTreeSnapshot{}, nil, errors.New("Pi session tree contains an incomplete parent chain")
			}
			if !isPiBridgeMarker(parent.entry) {
				break
			}
			parentID = strings.TrimSpace(pointerString(parent.entry.ParentID))
		}
		projected := PiTreeNode{
			ID: id, ParentID: nilIfEmpty(parentID), Type: node.entry.Type,
			Role: piTreeRole(node.entry), Preview: piTreePreview(node.entry),
			Timestamp: strings.TrimSpace(node.entry.Timestamp), Label: node.label,
			Children: []string{},
		}
		_, projected.Active = active[id]
		byVisibleID[id] = len(nodes)
		nodes = append(nodes, projected)
	}
	for index := range nodes {
		parentID := strings.TrimSpace(pointerString(nodes[index].ParentID))
		if parentID == "" {
			continue
		}
		parentIndex, ok := byVisibleID[parentID]
		if !ok {
			return PiTreeSnapshot{}, nil, errors.New("Pi session tree projected parent is missing")
		}
		nodes[parentIndex].Children = append(nodes[parentIndex].Children, nodes[index].ID)
	}

	return PiTreeSnapshot{
		SessionID: strings.TrimSpace(sessionID), LeafID: nilIfEmpty(logicalLeafID),
		Revision: revision, Nodes: nodes,
	}, raw, nil
}

func waitForPiBridgeMarker(
	ctx context.Context,
	client *piRPCClient,
	nonce string,
) (piHistoryEntriesResponse, piHistoryEntry, error) {
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		var response piHistoryEntriesResponse
		if err := client.Request(ctx, "get_entries", nil, &response); err != nil {
			return piHistoryEntriesResponse{}, piHistoryEntry{}, fmt.Errorf("verify Pi tree navigation: %w", err)
		}
		for _, entry := range response.Entries {
			marker, ok := parsePiBridgeMarker(entry)
			if ok && marker.Nonce == nonce {
				if pointerString(response.LeafID) != entry.ID {
					return piHistoryEntriesResponse{}, piHistoryEntry{}, errors.New("Pi tree navigation marker is not the active leaf")
				}
				return response, entry, nil
			}
		}
		select {
		case <-ctx.Done():
			return piHistoryEntriesResponse{}, piHistoryEntry{}, fmt.Errorf("wait for Pi tree navigation marker: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func validatePiNavigationMarker(
	target piHistoryEntry,
	marker piHistoryEntry,
	entries []piHistoryEntry,
	summarize bool,
) error {
	data, ok := parsePiBridgeMarker(marker)
	if !ok || data.TargetID != strings.TrimSpace(target.ID) || data.Summarize != summarize {
		return errors.New("Pi tree navigation marker payload does not match the request")
	}
	expectedParent := strings.TrimSpace(target.ID)
	if target.Type == "custom_message" || (target.Type == "message" && strings.EqualFold(strings.TrimSpace(target.Message.Role), "user")) {
		expectedParent = strings.TrimSpace(pointerString(target.ParentID))
	}
	actualParent := strings.TrimSpace(pointerString(marker.ParentID))
	if actualParent == expectedParent {
		return nil
	}
	if !summarize || actualParent == "" {
		return errors.New("Pi tree navigation marker parent does not match the target semantics")
	}
	for _, entry := range entries {
		if strings.TrimSpace(entry.ID) == actualParent && entry.Type == "branch_summary" && strings.TrimSpace(pointerString(entry.ParentID)) == expectedParent {
			return nil
		}
	}
	return errors.New("Pi tree navigation summary is not attached to the target branch")
}

func isPiBridgeMarker(entry piHistoryEntry) bool {
	return strings.TrimSpace(entry.Type) == "custom" && strings.TrimSpace(entry.CustomType) == piBridgeMarkerType
}

func parsePiBridgeMarker(entry piHistoryEntry) (piBridgeMarkerData, bool) {
	if !isPiBridgeMarker(entry) || len(entry.Data) == 0 {
		return piBridgeMarkerData{}, false
	}
	var data piBridgeMarkerData
	if json.Unmarshal(entry.Data, &data) != nil {
		return piBridgeMarkerData{}, false
	}
	data.TargetID = strings.TrimSpace(data.TargetID)
	data.Nonce = strings.TrimSpace(data.Nonce)
	return data, data.TargetID != "" && data.Nonce != ""
}

func piTreeRole(entry piHistoryEntry) string {
	if entry.Type == "message" {
		return strings.TrimSpace(entry.Message.Role)
	}
	if entry.Type == "custom_message" {
		return "user"
	}
	return ""
}

func piTreePreview(entry piHistoryEntry) string {
	var value string
	switch entry.Type {
	case "message":
		value = piHistoryContentText(entry.Message.Content)
	case "custom_message":
		value = piHistoryContentText(entry.Content)
	case "compaction", "branch_summary":
		value = entry.Summary
	case "model_change":
		value = strings.Trim(strings.TrimSpace(entry.Provider)+"/"+strings.TrimSpace(entry.ModelID), "/")
	case "thinking_level_change":
		value = entry.ThinkingLevel
	case "custom":
		value = entry.CustomType
	case "session_info":
		value = entry.Name
	}
	value = strings.TrimSpace(value)
	if line, _, found := strings.Cut(value, "\n"); found {
		value = strings.TrimSpace(line)
	}
	runes := []rune(value)
	if len(runes) > piTreePreviewRunes {
		value = string(runes[:piTreePreviewRunes-3]) + "..."
	}
	return value
}

func piTreeEditorText(entry piHistoryEntry) string {
	if entry.Type == "message" && strings.EqualFold(strings.TrimSpace(entry.Message.Role), "user") {
		return piTreeEditorContentText(entry.Message.Content)
	}
	if entry.Type == "custom_message" {
		return piTreeEditorContentText(entry.Content)
	}
	return ""
}

func piTreeEditorContentText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	var builder strings.Builder
	for _, block := range blocks {
		if strings.EqualFold(block.Type, "text") {
			builder.WriteString(block.Text)
		}
	}
	return builder.String()
}
