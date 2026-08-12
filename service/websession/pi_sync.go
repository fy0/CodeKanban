package websession

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"code-kanban/model"
	"code-kanban/model/tables"
	"code-kanban/utils"

	"gorm.io/gorm"
)

type piHistoryEntry struct {
	Type          string           `json:"type"`
	ID            string           `json:"id"`
	ParentID      *string          `json:"parentId"`
	Timestamp     string           `json:"timestamp"`
	Message       piHistoryMessage `json:"message"`
	CustomType    string           `json:"customType"`
	Data          json.RawMessage  `json:"data"`
	Content       json.RawMessage  `json:"content"`
	Summary       string           `json:"summary"`
	Provider      string           `json:"provider"`
	ModelID       string           `json:"modelId"`
	ThinkingLevel string           `json:"thinkingLevel"`
	Name          string           `json:"name"`
}

type piHistoryMessage struct {
	Role         string          `json:"role"`
	Content      json.RawMessage `json:"content"`
	Timestamp    int64           `json:"timestamp"`
	StopReason   string          `json:"stopReason"`
	ErrorMessage string          `json:"errorMessage"`
}

type piHistoryEntriesResponse struct {
	Entries []piHistoryEntry `json:"entries"`
	LeafID  *string          `json:"leafId"`
}

type piHistoryTreeResponse struct {
	Tree   json.RawMessage `json:"tree"`
	LeafID *string         `json:"leafId"`
}

func (m *Manager) syncImportedPiSession(
	ctx context.Context,
	session tables.WebSessionTable,
) (SessionSnapshot, error) {
	if m.hasActiveRun(session.ID) {
		return SessionSnapshot{}, errors.New("cannot sync an active Pi web session")
	}
	runtime, err := m.getOrStartPiRuntime(ctx, session)
	if err != nil {
		return SessionSnapshot{}, err
	}
	defer runtime.scheduleIdle()

	var tree piHistoryTreeResponse
	if err := runtime.client.Request(ctx, "get_tree", nil, &tree); err != nil {
		return SessionSnapshot{}, fmt.Errorf("read Pi session tree: %w", err)
	}
	if len(tree.Tree) == 0 || !json.Valid(tree.Tree) {
		return SessionSnapshot{}, errors.New("Pi returned an invalid session tree")
	}
	var entries piHistoryEntriesResponse
	if err := runtime.client.Request(ctx, "get_entries", nil, &entries); err != nil {
		return SessionSnapshot{}, fmt.Errorf("read Pi session entries: %w", err)
	}
	if pointerString(tree.LeafID) != pointerString(entries.LeafID) {
		return SessionSnapshot{}, errors.New("Pi session tree and entry leaf do not match")
	}

	refreshed, err := m.GetSession(ctx, session.ID)
	if err != nil {
		return SessionSnapshot{}, err
	}
	if err := m.projectPiHistoryEntries(ctx, refreshed, entries); err != nil {
		return SessionSnapshot{}, err
	}
	if err := m.syncPiRuntimeSnapshot(ctx, runtime, refreshed); err != nil {
		return SessionSnapshot{}, err
	}
	refreshed, err = m.GetSession(ctx, session.ID)
	if err != nil {
		return SessionSnapshot{}, err
	}
	return m.loadSnapshotLocal(ctx, refreshed, DefaultHistoryWindow, false)
}

type piHistoryProjection struct {
	turns   []tables.WebSessionTurnTable
	items   []tables.WebSessionItemTable
	updates map[string]any
}

func (m *Manager) projectPiHistoryEntries(
	ctx context.Context,
	session tables.WebSessionTable,
	response piHistoryEntriesResponse,
) error {
	projection, err := buildPiHistoryProjection(session, response)
	if err != nil {
		return err
	}
	if err := m.store.deleteSessionFiles(session.ID); err != nil {
		return err
	}
	return m.replaceSessionHistoryCache(ctx, session, projection.turns, projection.items, projection.updates)
}

func buildPiHistoryProjection(
	session tables.WebSessionTable,
	response piHistoryEntriesResponse,
) (piHistoryProjection, error) {
	active, err := activePiHistoryEntries(response.Entries, pointerString(response.LeafID))
	if err != nil {
		return piHistoryProjection{}, err
	}
	turns := make([]tables.WebSessionTurnTable, 0)
	items := make([]tables.WebSessionItemTable, 0, len(active))
	nativeID := pointerString(session.NativeSessionID)
	var currentTurn *tables.WebSessionTurnTable
	var order int64
	var lastMessageAt *time.Time

	for _, entry := range active {
		if entry.Type != "message" {
			continue
		}
		role := strings.ToLower(strings.TrimSpace(entry.Message.Role))
		if role != "user" && role != "assistant" {
			continue
		}
		text := piHistoryContentText(entry.Message.Content)
		if text == "" && strings.TrimSpace(entry.Message.ErrorMessage) == "" {
			continue
		}
		observedAt := piHistoryEntryTime(entry)
		lastMessageAt = &observedAt

		if role == "user" || currentTurn == nil {
			turn := tables.WebSessionTurnTable{}
			turn.Init()
			turn.WebSessionID = session.ID
			turn.SourceThreadID = nilIfEmptyHistory(nativeID)
			turn.SourceTurnID = nilIfEmptyHistory(entry.ID)
			turn.OrderIndex = int64(len(turns) + 1)
			turn.Status = "completed"
			turn.SourceCreated = true
			turns = append(turns, turn)
			currentTurn = &turns[len(turns)-1]
		}

		order++
		item := HistoryItem{
			ID:             utils.NewID(),
			SourceThreadID: nilIfEmptyHistory(nativeID),
			SourceTurnID:   currentTurn.SourceTurnID,
			SourceItemID:   nilIfEmptyHistory(entry.ID),
			OrderIndex:     order,
			Kind:           role,
			ItemType:       map[string]string{"user": "user_message", "assistant": "agent_message"}[role],
			Text:           text,
			Timestamp:      &observedAt,
			ObservedAt:     &observedAt,
			Done:           true,
		}
		if strings.EqualFold(strings.TrimSpace(entry.Message.StopReason), "error") || strings.TrimSpace(entry.Message.ErrorMessage) != "" {
			item.Level = "error"
			if item.Text == "" {
				item.Text = "Pi assistant run failed"
			}
		}
		row := tables.WebSessionItemTable{}
		row.Init()
		row.WebSessionID = session.ID
		row.WebTurnID = &currentTurn.ID
		applyHistoryItemToRow(&row, session.ID, item)
		items = append(items, row)
	}

	now := time.Now()
	updates := map[string]any{
		"source_kind":       string(SessionBackendPiRPC),
		"native_leaf_id":    nilIfEmpty(pointerString(response.LeafID)),
		"source_revision":   nilIfEmpty(piSourceRevision(pointerString(session.ThreadPath), pointerString(response.LeafID))),
		"last_synced_at":    now,
		"sync_state":        string(SyncStateFresh),
		"sync_error":        nil,
		"turn_count":        len(turns),
		"item_count":        len(items),
		"last_message_at":   lastMessageAt,
		"source_updated_at": lastMessageAt,
		"updated_at":        now,
	}
	return piHistoryProjection{turns: turns, items: items, updates: updates}, nil
}

type piLiveMessage struct {
	entry        piHistoryEntry
	role         string
	text         string
	observedAt   time.Time
	turnSourceID string
}

func piLiveMessages(response piHistoryEntriesResponse) ([]piLiveMessage, error) {
	active, err := activePiHistoryEntries(response.Entries, pointerString(response.LeafID))
	if err != nil {
		return nil, err
	}
	messages := make([]piLiveMessage, 0, len(active))
	turnSourceID := ""
	for _, entry := range active {
		if entry.Type != "message" {
			continue
		}
		role := strings.ToLower(strings.TrimSpace(entry.Message.Role))
		if role != "user" && role != "assistant" {
			continue
		}
		text := piHistoryContentText(entry.Message.Content)
		if text == "" && strings.TrimSpace(entry.Message.ErrorMessage) == "" {
			continue
		}
		if role == "user" || turnSourceID == "" {
			turnSourceID = strings.TrimSpace(entry.ID)
		}
		messages = append(messages, piLiveMessage{
			entry: entry, role: role, text: text,
			observedAt: piHistoryEntryTime(entry), turnSourceID: turnSourceID,
		})
	}
	return messages, nil
}

func (m *Manager) reconcileLivePiHistory(
	ctx context.Context,
	session tables.WebSessionTable,
	nativeSessionID string,
	response piHistoryEntriesResponse,
	updates map[string]any,
) error {
	nativeSessionID = strings.TrimSpace(nativeSessionID)
	if nativeSessionID == "" {
		return errors.New("Pi runtime has no native session id")
	}
	messages, err := piLiveMessages(response)
	if err != nil {
		return err
	}
	db := model.GetDB()
	if db == nil {
		return model.ErrDBNotInitialized
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current tables.WebSessionTable
		if err := tx.Select("id").First(&current, "id = ?", session.ID).Error; err != nil {
			return err
		}

		var itemRows []tables.WebSessionItemTable
		if err := tx.Where("web_session_id = ?", session.ID).Order("order_index ASC").Find(&itemRows).Error; err != nil {
			return err
		}
		var turnRows []tables.WebSessionTurnTable
		if err := tx.Where("web_session_id = ?", session.ID).Order("order_index ASC").Find(&turnRows).Error; err != nil {
			return err
		}

		turnBySource := make(map[string]*tables.WebSessionTurnTable, len(turnRows))
		for index := range turnRows {
			row := &turnRows[index]
			if pointerString(row.SourceThreadID) == nativeSessionID {
				turnBySource[pointerString(row.SourceTurnID)] = row
			}
		}
		turnIDs := make(map[string]string)
		activeTurnSources := make([]string, 0)
		seenTurn := make(map[string]struct{})
		for _, message := range messages {
			turnSourceID := strings.TrimSpace(message.turnSourceID)
			if _, exists := seenTurn[turnSourceID]; exists {
				continue
			}
			seenTurn[turnSourceID] = struct{}{}
			activeTurnSources = append(activeTurnSources, turnSourceID)
			row := turnBySource[turnSourceID]
			if row == nil {
				created := &tables.WebSessionTurnTable{}
				created.Init()
				created.WebSessionID = session.ID
				row = created
			}
			row.SourceThreadID = nilIfEmptyHistory(nativeSessionID)
			row.SourceTurnID = nilIfEmptyHistory(turnSourceID)
			row.OrderIndex = int64(len(activeTurnSources))
			row.Status = "completed"
			row.SourceCreated = true
			if row.CreatedAt.IsZero() {
				if err := tx.Create(row).Error; err != nil {
					return err
				}
			} else if err := tx.Save(row).Error; err != nil {
				return err
			}
			turnIDs[turnSourceID] = row.ID
		}

		exactRows := make(map[string]*tables.WebSessionItemTable)
		for index := range itemRows {
			row := &itemRows[index]
			if pointerString(row.SourceThreadID) == nativeSessionID {
				exactRows[pointerString(row.SourceItemID)] = row
			}
		}
		usedRows := make(map[string]struct{}, len(messages))
		maxOrder := int64(0)
		for index := range itemRows {
			if itemRows[index].OrderIndex > maxOrder {
				maxOrder = itemRows[index].OrderIndex
			}
		}
		activeItemIDs := make([]string, 0, len(messages))
		turnStartOrders := make(map[string]int64, len(activeTurnSources))
		var lastMessageAt *time.Time
		for _, message := range messages {
			entryID := strings.TrimSpace(message.entry.ID)
			activeItemIDs = append(activeItemIDs, entryID)
			row := exactRows[entryID]
			if row == nil {
				row = findPiLiveMessageCandidate(itemRows, usedRows, message.role, message.text)
			}
			isNew := row == nil
			if isNew {
				row = &tables.WebSessionItemTable{}
				row.Init()
				maxOrder++
				row.OrderIndex = maxOrder
			}
			usedRows[row.ID] = struct{}{}
			item := mapHistoryItemRowWithSession(*row, session.ID)
			item.SourceThreadID = nilIfEmptyHistory(nativeSessionID)
			item.SourceTurnID = nilIfEmptyHistory(message.turnSourceID)
			item.SourceItemID = nilIfEmptyHistory(entryID)
			item.Kind = message.role
			item.ItemType = map[string]string{"user": "user_message", "assistant": "agent_message"}[message.role]
			item.Text = message.text
			item.Timestamp = &message.observedAt
			item.ObservedAt = &message.observedAt
			item.Done = true
			item.Level = ""
			if strings.EqualFold(strings.TrimSpace(message.entry.Message.StopReason), "error") || strings.TrimSpace(message.entry.Message.ErrorMessage) != "" {
				item.Level = "error"
				if item.Text == "" {
					item.Text = "Pi assistant run failed"
				}
			}
			applyHistoryItemToRow(row, session.ID, item)
			row.WebTurnID = nilIfEmptyHistory(turnIDs[message.turnSourceID])
			row.Role = message.role
			row.Status = "completed"
			if current, exists := turnStartOrders[message.turnSourceID]; !exists || row.OrderIndex < current {
				turnStartOrders[message.turnSourceID] = row.OrderIndex
			}
			if isNew {
				if err := tx.Create(row).Error; err != nil {
					return err
				}
			} else if err := tx.Save(row).Error; err != nil {
				return err
			}
			value := message.observedAt
			lastMessageAt = &value
		}

		for index, turnSourceID := range activeTurnSources {
			startOrder, exists := turnStartOrders[turnSourceID]
			if !exists {
				continue
			}
			liveItems := tx.Model(&tables.WebSessionItemTable{}).
				Where("web_session_id = ? AND order_index >= ? AND (source_thread_id IS NULL OR source_thread_id = '')", session.ID, startOrder)
			if index+1 < len(activeTurnSources) {
				if nextOrder, ok := turnStartOrders[activeTurnSources[index+1]]; ok {
					liveItems = liveItems.Where("order_index < ?", nextOrder)
				}
			}
			if err := liveItems.Updates(map[string]any{
				"web_turn_id":      turnIDs[turnSourceID],
				"source_thread_id": nativeSessionID,
				"source_turn_id":   turnSourceID,
			}).Error; err != nil {
				return err
			}
		}

		staleItems := tx.Unscoped().Where(
			"web_session_id = ? AND source_thread_id = ? AND item_kind IN ?",
			session.ID, nativeSessionID, []string{"user", "assistant"},
		)
		if len(activeItemIDs) > 0 {
			staleItems = staleItems.Where("source_item_id NOT IN ?", activeItemIDs)
		}
		if err := staleItems.Delete(&tables.WebSessionItemTable{}).Error; err != nil {
			return err
		}
		staleTurns := tx.Unscoped().Where("web_session_id = ? AND source_thread_id = ?", session.ID, nativeSessionID)
		if len(activeTurnSources) > 0 {
			staleTurns = staleTurns.Where("source_turn_id NOT IN ?", activeTurnSources)
		}
		if err := staleTurns.Delete(&tables.WebSessionTurnTable{}).Error; err != nil {
			return err
		}

		var itemCount int64
		if err := tx.Model(&tables.WebSessionItemTable{}).Where("web_session_id = ?", session.ID).Count(&itemCount).Error; err != nil {
			return err
		}
		var turnCount int64
		if err := tx.Model(&tables.WebSessionTurnTable{}).Where("web_session_id = ?", session.ID).Count(&turnCount).Error; err != nil {
			return err
		}
		nextUpdates := cloneMap(updates)
		nextUpdates["turn_count"] = turnCount
		nextUpdates["item_count"] = itemCount
		nextUpdates["last_message_at"] = lastMessageAt
		nextUpdates["source_updated_at"] = lastMessageAt
		return tx.Model(&tables.WebSessionTable{}).
			Where("id = ?", session.ID).
			Updates(withSnapshotRevisionIncrement(nextUpdates)).Error
	})
}

func findPiLiveMessageCandidate(
	rows []tables.WebSessionItemTable,
	used map[string]struct{},
	role string,
	text string,
) *tables.WebSessionItemTable {
	var fallback *tables.WebSessionItemTable
	for index := range rows {
		row := &rows[index]
		if _, claimed := used[row.ID]; claimed || pointerString(row.SourceThreadID) != "" || row.ItemKind != role {
			continue
		}
		if fallback == nil {
			fallback = row
		}
		if row.Text == text {
			return row
		}
	}
	return fallback
}

func activePiHistoryEntries(entries []piHistoryEntry, leafID string) ([]piHistoryEntry, error) {
	if strings.TrimSpace(leafID) == "" {
		if len(entries) == 0 {
			return nil, nil
		}
		return nil, errors.New("Pi session entries have no active leaf")
	}
	byID := make(map[string]piHistoryEntry, len(entries))
	for _, entry := range entries {
		if id := strings.TrimSpace(entry.ID); id != "" {
			byID[id] = entry
		}
	}
	path := make([]piHistoryEntry, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for currentID := strings.TrimSpace(leafID); currentID != ""; {
		if _, duplicate := seen[currentID]; duplicate {
			return nil, errors.New("Pi session active branch contains a cycle")
		}
		entry, ok := byID[currentID]
		if !ok {
			return nil, errors.New("Pi session active branch is incomplete")
		}
		seen[currentID] = struct{}{}
		path = append(path, entry)
		if entry.ParentID == nil {
			break
		}
		currentID = strings.TrimSpace(*entry.ParentID)
	}
	for left, right := 0, len(path)-1; left < right; left, right = left+1, right-1 {
		path[left], path[right] = path[right], path[left]
	}
	return path, nil
}

func piHistoryContentText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return strings.TrimSpace(text)
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if strings.EqualFold(block.Type, "text") && strings.TrimSpace(block.Text) != "" {
			parts = append(parts, strings.TrimSpace(block.Text))
		}
	}
	return strings.Join(parts, "\n")
}

func piHistoryEntryTime(entry piHistoryEntry) time.Time {
	if timestamp, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(entry.Timestamp)); err == nil {
		return timestamp
	}
	if entry.Message.Timestamp > 0 {
		return time.UnixMilli(entry.Message.Timestamp)
	}
	return time.Now()
}
