package websession

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"code-kanban/model"
	"code-kanban/model/tables"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

func normalizeSyncState(value string) SyncState {
	switch SyncState(strings.TrimSpace(value)) {
	case SyncStateFresh, SyncStateStale:
		// Passive stale detection has been retired. Legacy stale values should behave
		// like a synced cache instead of continuing to surface warning UI.
		return SyncStateFresh
	case SyncStateMissing, SyncStateSyncing, SyncStateError:
		return SyncState(strings.TrimSpace(value))
	default:
		return SyncStateMissing
	}
}

func normalizeSyncMode(value string) SyncMode {
	switch SyncMode(strings.TrimSpace(value)) {
	case SyncModeFast:
		return SyncModeFast
	case SyncModeDeep:
		return SyncModeDeep
	default:
		return SyncModeFast
	}
}

func recordedSyncMode(value string) SyncMode {
	switch SyncMode(strings.TrimSpace(value)) {
	case SyncModeFast:
		return SyncModeFast
	case SyncModeDeep:
		return SyncModeDeep
	default:
		return ""
	}
}

func mustJSONText(value any) string {
	if value == nil {
		return ""
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func decodeJSONText(raw string, target any) {
	if strings.TrimSpace(raw) == "" || target == nil {
		return
	}
	_ = json.Unmarshal([]byte(raw), target)
}

func historyItemCursor(items []HistoryItem, hasMore bool) string {
	if !hasMore || len(items) == 0 {
		return ""
	}
	return strconv.FormatInt(items[0].OrderIndex, 10)
}

func historyItemAfterCursor(items []HistoryItem, hasLater bool) string {
	if !hasLater || len(items) == 0 {
		return ""
	}
	return strconv.FormatInt(items[len(items)-1].OrderIndex, 10)
}

func mapHistoryItemRow(row tables.WebSessionItemTable) HistoryItem {
	var attachments []HistoryAttachment
	var tool *HistoryTool
	var detail *HistoryDetail
	var payload map[string]any

	decodeJSONText(row.AttachmentsJSON, &attachments)
	decodeJSONText(row.ToolJSON, &tool)
	decodeJSONText(row.DetailJSON, &detail)
	decodeJSONText(row.PayloadJSON, &payload)

	return HistoryItem{
		ID:             row.ID,
		SourceThreadID: row.SourceThreadID,
		SourceTurnID:   row.SourceTurnID,
		SourceItemID:   row.SourceItemID,
		RunID:          row.RunID,
		RunDurationMs:  row.RunDurationMs,
		RunOutcome:     WorkTimingOutcome(row.RunOutcome),
		OrderIndex:     row.OrderIndex,
		LastEventSeq:   row.LastEventSeq,
		Kind:           row.ItemKind,
		ItemType:       row.ItemType,
		Text:           row.Text,
		Timestamp:      row.Timestamp,
		ObservedAt:     row.ObservedAt,
		Attachments:    attachments,
		Tool:           tool,
		Level:          row.Level,
		Done:           row.Done,
		Detail:         detail,
		Payload:        payload,
	}
}

func mapHistoryItemRowWithSession(row tables.WebSessionItemTable, sessionID string) HistoryItem {
	item := mapHistoryItemRow(row)
	if item.ID == "" {
		item.ID = sessionID + ":" + strconv.FormatInt(item.OrderIndex, 10)
	}
	return item
}

func applyHistoryItemToRow(row *tables.WebSessionItemTable, sessionID string, item HistoryItem) {
	if row == nil {
		return
	}
	row.WebSessionID = sessionID
	row.SourceThreadID = item.SourceThreadID
	row.SourceTurnID = item.SourceTurnID
	row.SourceItemID = item.SourceItemID
	commandGroupID := ""
	if item.Tool != nil && item.Tool.CommandGroup != nil {
		commandGroupID = strings.TrimSpace(item.Tool.CommandGroup.ID)
	}
	row.CommandGroupID = ptr(commandGroupID)
	row.RunID = item.RunID
	row.RunDurationMs = item.RunDurationMs
	row.RunOutcome = string(item.RunOutcome)
	row.OrderIndex = item.OrderIndex
	row.LastEventSeq = item.LastEventSeq
	row.ItemKind = strings.TrimSpace(item.Kind)
	row.ItemType = strings.TrimSpace(item.ItemType)
	row.Text = item.Text
	row.Timestamp = item.Timestamp
	row.ObservedAt = item.ObservedAt
	row.Level = strings.TrimSpace(item.Level)
	row.Done = item.Done
	row.AttachmentsJSON = mustJSONText(item.Attachments)
	row.ToolJSON = mustJSONText(item.Tool)
	row.DetailJSON = mustJSONText(item.Detail)
	row.PayloadJSON = mustJSONText(item.Payload)
}

func historyToolSourceKey(toolKey string) string {
	normalized := strings.TrimSpace(toolKey)
	if normalized == "" {
		return ""
	}
	return "tool:" + normalized
}

func (m *Manager) nextHistoryOrderIndexDB(ctx context.Context, db *gorm.DB, sessionID string) (int64, error) {
	var maxValue int64
	if err := db.WithContext(ctx).
		Model(&tables.WebSessionItemTable{}).
		Where("web_session_id = ?", sessionID).
		Select("COALESCE(MAX(order_index), 0)").
		Scan(&maxValue).Error; err != nil {
		return 0, err
	}
	return maxValue + 1, nil
}

func (m *Manager) appendHistoryItem(
	ctx context.Context,
	sessionID string,
	item HistoryItem,
) (HistoryItem, error) {
	db := model.GetDB()
	if db == nil {
		return HistoryItem{}, model.ErrDBNotInitialized
	}
	return m.appendHistoryItemDB(ctx, db, sessionID, item)
}

func (m *Manager) appendHistoryItemDB(
	ctx context.Context,
	db *gorm.DB,
	sessionID string,
	item HistoryItem,
) (HistoryItem, error) {
	if item.OrderIndex <= 0 {
		nextOrder, err := m.nextHistoryOrderIndexDB(ctx, db, sessionID)
		if err != nil {
			return HistoryItem{}, err
		}
		item.OrderIndex = nextOrder
	}
	row := tables.WebSessionItemTable{}
	row.Init()
	applyHistoryItemToRow(&row, sessionID, item)
	if err := db.WithContext(ctx).Create(&row).Error; err != nil {
		return HistoryItem{}, err
	}
	return mapHistoryItemRowWithSession(row, sessionID), nil
}

func (m *Manager) upsertHistoryItemBySourceIdentityDB(
	ctx context.Context,
	db *gorm.DB,
	sessionID string,
	sourceThreadID string,
	sourceItemID string,
	mutate func(*HistoryItem),
) (HistoryItem, error) {
	sourceItemID = strings.TrimSpace(sourceItemID)
	if sourceItemID == "" {
		return HistoryItem{}, fmt.Errorf("source item id is required")
	}

	var row tables.WebSessionItemTable
	query := db.WithContext(ctx).Where("web_session_id = ? AND source_item_id = ?", sessionID, sourceItemID)
	if sourceThreadID = strings.TrimSpace(sourceThreadID); sourceThreadID != "" {
		query = query.Where("source_thread_id = ?", sourceThreadID)
	} else {
		query = query.Where("source_thread_id IS NULL OR source_thread_id = ''")
	}
	err := query.First(&row).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return HistoryItem{}, err
	}

	var item HistoryItem
	if err == nil {
		item = mapHistoryItemRowWithSession(row, sessionID)
	} else {
		nextOrder, nextErr := m.nextHistoryOrderIndexDB(ctx, db, sessionID)
		if nextErr != nil {
			return HistoryItem{}, nextErr
		}
		row.Init()
		item = HistoryItem{
			ID:             row.ID,
			SourceThreadID: nilIfEmptyHistory(sourceThreadID),
			SourceItemID:   ptr(strings.TrimSpace(sourceItemID)),
			OrderIndex:     nextOrder,
		}
	}
	mutate(&item)
	if item.SourceItemID == nil {
		item.SourceItemID = ptr(sourceItemID)
	}
	if item.SourceThreadID == nil {
		item.SourceThreadID = nilIfEmptyHistory(sourceThreadID)
	}
	applyHistoryItemToRow(&row, sessionID, item)
	if err == gorm.ErrRecordNotFound {
		if createErr := db.WithContext(ctx).Create(&row).Error; createErr != nil {
			return HistoryItem{}, createErr
		}
		return mapHistoryItemRowWithSession(row, sessionID), nil
	}
	if updateErr := db.WithContext(ctx).Save(&row).Error; updateErr != nil {
		return HistoryItem{}, updateErr
	}
	return mapHistoryItemRowWithSession(row, sessionID), nil
}

func (m *Manager) ensureCompactGroupHistorySourceKeyDB(
	ctx context.Context,
	db *gorm.DB,
	sessionID string,
	sourceThreadID string,
	sourceKey string,
	groupID string,
) error {
	sourceKey = strings.TrimSpace(sourceKey)
	groupID = strings.TrimSpace(groupID)
	if sourceKey == "" || groupID == "" {
		return nil
	}

	return m.observedTransaction(ctx, db, "ensure_compact_group_source_key", func(tx *gorm.DB) error {
		var rows []tables.WebSessionItemTable
		query := tx.Where(
			"web_session_id = ? AND command_group_id = ? AND item_kind = ?",
			sessionID,
			groupID,
			"tool",
		)
		if sourceThreadID = strings.TrimSpace(sourceThreadID); sourceThreadID != "" {
			query = query.Where("source_thread_id = ?", sourceThreadID)
		} else {
			query = query.Where("source_thread_id IS NULL OR source_thread_id = ''")
		}
		if err := query.
			Order("order_index ASC").
			Find(&rows).Error; err != nil {
			return err
		}

		type candidateRow struct {
			row  tables.WebSessionItemTable
			item HistoryItem
		}

		candidates := make([]candidateRow, 0, len(rows))
		for _, row := range rows {
			item := mapHistoryItemRowWithSession(row, sessionID)
			if item.Tool == nil || !isCompactToolKind(item.Tool.Kind) {
				continue
			}
			if item.Tool.CommandGroup == nil || strings.TrimSpace(item.Tool.CommandGroup.ID) != groupID {
				continue
			}
			candidates = append(candidates, candidateRow{row: row, item: item})
		}
		if len(candidates) == 0 {
			return nil
		}

		canonicalIndex := 0
		for index, candidate := range candidates {
			if candidate.item.SourceItemID != nil && strings.TrimSpace(*candidate.item.SourceItemID) == sourceKey {
				canonicalIndex = index
				break
			}
		}
		if len(candidates) == 1 {
			sourceItemID := ""
			if candidates[0].item.SourceItemID != nil {
				sourceItemID = strings.TrimSpace(*candidates[0].item.SourceItemID)
			}
			if sourceItemID == sourceKey {
				return nil
			}
		}

		canonical := candidates[canonicalIndex]
		latest := candidates[len(candidates)-1]
		merged := canonical.item
		merged.SourceItemID = nilIfEmptyHistory(sourceKey)
		merged.Kind = latest.item.Kind
		merged.ItemType = latest.item.ItemType
		merged.Text = latest.item.Text
		merged.Attachments = latest.item.Attachments
		merged.Tool = latest.item.Tool
		merged.Level = latest.item.Level
		merged.Done = latest.item.Done
		merged.Detail = latest.item.Detail
		merged.Payload = cloneMap(latest.item.Payload)
		merged.ObservedAt = latest.item.ObservedAt
		if merged.Timestamp == nil {
			merged.Timestamp = latest.item.Timestamp
		}

		groupItems := []CommandExecutionGroupItem{}
		groupCount := 0
		for _, candidate := range candidates {
			groupItems = mergeHistoryGroupItemLists(groupItems, decodeHistoryGroupItems(candidate.item.Payload))
			if candidate.item.Tool != nil && candidate.item.Tool.CommandGroup != nil {
				groupCount = max(groupCount, candidate.item.Tool.CommandGroup.Count)
			}
		}

		if merged.Tool != nil && merged.Tool.CommandGroup != nil {
			if len(groupItems) > 0 {
				groupCount = max(groupCount, len(groupItems))
			}
			merged.Tool.CommandGroup.Count = max(1, groupCount)
			merged.Tool.CommandGroup.ID = groupID
			merged.Tool.CommandGroup.Compacted = true
			if merged.Tool.Meta == nil {
				merged.Tool.Meta = map[string]any{}
			}
			merged.Tool.Meta["commandGroup"] = merged.Tool.CommandGroup
		}
		if merged.Payload == nil {
			merged.Payload = map[string]any{}
		}
		if len(groupItems) > 0 {
			merged.Payload["groupItems"] = groupItems
		}
		if merged.Tool != nil && len(merged.Tool.Meta) > 0 {
			merged.Payload["meta"] = merged.Tool.Meta
		}

		applyHistoryItemToRow(&canonical.row, sessionID, merged)
		if err := tx.Save(&canonical.row).Error; err != nil {
			return err
		}

		deleteIDs := make([]string, 0, len(candidates)-1)
		for index, candidate := range candidates {
			if index == canonicalIndex {
				continue
			}
			deleteIDs = append(deleteIDs, candidate.row.ID)
		}
		if len(deleteIDs) > 0 {
			result := tx.Unscoped().Where("id IN ?", deleteIDs).Delete(&tables.WebSessionItemTable{})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected > 0 {
				if err := tx.Model(&tables.WebSessionTable{}).
					Where("id = ?", sessionID).
					UpdateColumn("history_epoch", gorm.Expr("history_epoch + 1")).Error; err != nil {
					return err
				}
			}
		}
		return nil
	},
		zap.String("sessionId", sessionID),
		zap.String("sourceThreadId", sourceThreadID),
		zap.String("groupId", groupID),
	)
}

func (m *Manager) replaceSessionHistoryCache(
	ctx context.Context,
	session tables.WebSessionTable,
	turns []tables.WebSessionTurnTable,
	items []tables.WebSessionItemTable,
	updates map[string]any,
) error {
	db := model.GetDB()
	if db == nil {
		return model.ErrDBNotInitialized
	}
	return m.observedTransaction(ctx, db, "replace_history_cache", func(tx *gorm.DB) error {
		var existing tables.WebSessionTable
		if err := tx.Select("id").First(&existing, "id = ?", session.ID).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Where("web_session_id = ?", session.ID).Delete(&tables.WebSessionTurnTable{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Where("web_session_id = ?", session.ID).Delete(&tables.WebSessionItemTable{}).Error; err != nil {
			return err
		}
		if len(turns) > 0 {
			if err := tx.Create(&turns).Error; err != nil {
				return err
			}
		}
		if len(items) > 0 {
			if err := tx.Create(&items).Error; err != nil {
				return err
			}
		}
		if err := m.reapplyWorkTimingAnnotationsDB(ctx, tx, session.ID); err != nil {
			return err
		}
		updates = withHistoryEpochIncrement(updates)
		if err := tx.Model(&tables.WebSessionTable{}).
			Where("id = ?", session.ID).
			Updates(withSnapshotRevisionIncrement(updates)).Error; err != nil {
			return err
		}
		return nil
	},
		zap.String("sessionId", session.ID),
		zap.Int("turnCount", len(turns)),
		zap.Int("itemCount", len(items)),
		zap.Int("updateFieldCount", len(updates)),
	)
}

func (m *Manager) reconcileSessionHistoryCache(
	ctx context.Context,
	session tables.WebSessionTable,
	turns []tables.WebSessionTurnTable,
	items []tables.WebSessionItemTable,
	updates map[string]any,
	rootThreadID string,
) error {
	db := model.GetDB()
	if db == nil {
		return model.ErrDBNotInitialized
	}
	rootThreadID = strings.TrimSpace(rootThreadID)

	return m.observedTransaction(ctx, db, "reconcile_history_cache", func(tx *gorm.DB) error {
		var existingSession tables.WebSessionTable
		if err := tx.Select("id").First(&existingSession, "id = ?", session.ID).Error; err != nil {
			return err
		}

		var existingTurns []tables.WebSessionTurnTable
		if err := tx.Where("web_session_id = ?", session.ID).
			Order("order_index ASC").
			Find(&existingTurns).Error; err != nil {
			return err
		}
		turnBySource := make(map[string]*tables.WebSessionTurnTable, len(existingTurns))
		maxTurnOrder := int64(0)
		for index := range existingTurns {
			row := &existingTurns[index]
			if row.OrderIndex > maxTurnOrder {
				maxTurnOrder = row.OrderIndex
			}
			if key, ok := historyTurnSourceIdentity(row.SourceThreadID, row.SourceTurnID); ok {
				turnBySource[key] = row
			}
		}

		historyChanged := false
		turnIDBySource := make(map[string]string, len(turns))
		for index := range turns {
			incoming := turns[index]
			key, hasIdentity := historyTurnSourceIdentity(incoming.SourceThreadID, incoming.SourceTurnID)
			row := turnBySource[key]
			if !hasIdentity || row == nil {
				row = &incoming
				row.WebSessionID = session.ID
				maxTurnOrder++
				row.OrderIndex = maxTurnOrder
				if err := tx.Create(row).Error; err != nil {
					return err
				}
				historyChanged = true
				if hasIdentity {
					turnBySource[key] = row
				}
			} else {
				previous := *row
				row.Status = firstNonEmpty(strings.TrimSpace(incoming.Status), row.Status)
				if strings.TrimSpace(incoming.ErrorJSON) != "" || strings.TrimSpace(row.ErrorJSON) == "" {
					row.ErrorJSON = incoming.ErrorJSON
				}
				row.SourceCreated = row.SourceCreated || incoming.SourceCreated
				if !historyTurnRowsEqual(previous, *row) {
					if err := tx.Save(row).Error; err != nil {
						return err
					}
					historyChanged = true
				}
			}
			if hasIdentity {
				turnIDBySource[key] = row.ID
			}
		}

		var existingItems []tables.WebSessionItemTable
		if err := tx.Where("web_session_id = ?", session.ID).
			Order("order_index ASC").
			Find(&existingItems).Error; err != nil {
			return err
		}
		itemBySource := make(map[string]*tables.WebSessionItemTable, len(existingItems))
		maxItemOrder := int64(0)
		for index := range existingItems {
			row := &existingItems[index]
			if row.OrderIndex > maxItemOrder {
				maxItemOrder = row.OrderIndex
			}
			if key, ok := historyItemSourceIdentity(row.SourceThreadID, row.SourceItemID); ok {
				itemBySource[key] = row
			}
		}

		var latestRootPlan *tables.WebSessionItemTable
		for index := range items {
			incomingRow := items[index]
			incomingItem := mapHistoryItemRowWithSession(incomingRow, session.ID)
			key, hasIdentity := historyItemSourceIdentity(incomingRow.SourceThreadID, incomingRow.SourceItemID)
			row := itemBySource[key]
			isNew := !hasIdentity || row == nil
			var previous tables.WebSessionItemTable
			if isNew {
				row = &incomingRow
				row.WebSessionID = session.ID
				maxItemOrder++
				row.OrderIndex = maxItemOrder
			} else {
				previous = *row
				merged := mergeReconciledHistoryItem(
					mapHistoryItemRowWithSession(*row, session.ID),
					incomingItem,
				)
				applyHistoryItemToRow(row, session.ID, merged)
			}

			if turnKey, ok := historyTurnSourceIdentity(row.SourceThreadID, row.SourceTurnID); ok {
				row.WebTurnID = nilIfEmptyHistory(turnIDBySource[turnKey])
			}
			if strings.TrimSpace(row.Role) == "" {
				row.Role = incomingRow.Role
			}
			if strings.TrimSpace(incomingRow.Status) != "" {
				row.Status = incomingRow.Status
			}

			if isNew {
				if err := tx.Create(row).Error; err != nil {
					return err
				}
				historyChanged = true
				if hasIdentity {
					itemBySource[key] = row
				}
			} else {
				if !historyItemRowsEqual(previous, *row) {
					if err := tx.Save(row).Error; err != nil {
						return err
					}
					historyChanged = true
				}
			}

			if rootThreadID != "" && pointerString(row.SourceThreadID) == rootThreadID {
				item := mapHistoryItemRowWithSession(*row, session.ID)
				if isPlanHistoryItem(item) {
					latestRootPlan = row
				}
			}
		}

		if latestRootPlan != nil && latestRootPlan.OrderIndex != maxItemOrder {
			maxItemOrder++
			latestRootPlan.OrderIndex = maxItemOrder
			if err := tx.Save(latestRootPlan).Error; err != nil {
				return err
			}
			historyChanged = true
		}
		if historyChanged {
			if err := m.reapplyWorkTimingAnnotationsDB(ctx, tx, session.ID); err != nil {
				return err
			}
		}

		var itemCount int64
		if err := tx.Model(&tables.WebSessionItemTable{}).
			Where("web_session_id = ?", session.ID).
			Count(&itemCount).Error; err != nil {
			return err
		}
		var turnCount int64
		if err := tx.Model(&tables.WebSessionTurnTable{}).
			Where("web_session_id = ?", session.ID).
			Count(&turnCount).Error; err != nil {
			return err
		}

		nextUpdates := cloneMap(updates)
		nextUpdates["turn_count"] = turnCount
		nextUpdates["item_count"] = itemCount
		if historyChanged {
			nextUpdates = withHistoryEpochIncrement(nextUpdates)
		}
		return tx.Model(&tables.WebSessionTable{}).
			Where("id = ?", session.ID).
			Updates(withSnapshotRevisionIncrement(nextUpdates)).Error
	},
		zap.String("sessionId", session.ID),
		zap.Int("turnCount", len(turns)),
		zap.Int("itemCount", len(items)),
		zap.Int("updateFieldCount", len(updates)),
	)
}

func historyTurnSourceIdentity(threadID, turnID *string) (string, bool) {
	thread := pointerString(threadID)
	turn := pointerString(turnID)
	if thread == "" || turn == "" {
		return "", false
	}
	return scopedSourceTurnKey(thread, turn), true
}

func historyItemSourceIdentity(threadID, itemID *string) (string, bool) {
	item := pointerString(itemID)
	if item == "" {
		return "", false
	}
	return strings.TrimSpace(pointerString(threadID)) + "\x00" + item, true
}

func mergeReconciledHistoryItem(existing, incoming HistoryItem) HistoryItem {
	merged := incoming
	merged.ID = existing.ID
	merged.OrderIndex = existing.OrderIndex
	if merged.SourceThreadID == nil {
		merged.SourceThreadID = existing.SourceThreadID
	}
	if merged.SourceTurnID == nil {
		merged.SourceTurnID = existing.SourceTurnID
	}
	if merged.SourceItemID == nil {
		merged.SourceItemID = existing.SourceItemID
	}
	merged.RunID = existing.RunID
	merged.RunDurationMs = existing.RunDurationMs
	merged.RunOutcome = existing.RunOutcome
	merged.LastEventSeq = existing.LastEventSeq
	if incoming.LastEventSeq > merged.LastEventSeq {
		merged.LastEventSeq = incoming.LastEventSeq
	}
	if merged.Timestamp == nil {
		merged.Timestamp = existing.Timestamp
	}
	if merged.ObservedAt == nil {
		merged.ObservedAt = existing.ObservedAt
	}
	if len(merged.Attachments) == 0 {
		merged.Attachments = existing.Attachments
	}
	merged.Tool = mergeReconciledHistoryTool(existing.Tool, incoming.Tool)
	if strings.TrimSpace(merged.Level) == "" {
		merged.Level = existing.Level
	}
	merged.Done = merged.Done || existing.Done
	if merged.Detail == nil {
		merged.Detail = existing.Detail
	}
	merged.Payload = mergeReconciledHistoryMap(existing.Payload, incoming.Payload)
	return merged
}

func mergeReconciledHistoryTool(existing, incoming *HistoryTool) *HistoryTool {
	if incoming == nil {
		return existing
	}
	if existing == nil {
		return incoming
	}
	merged := *incoming
	merged.ID = firstNonEmpty(strings.TrimSpace(incoming.ID), existing.ID)
	merged.Name = firstNonEmpty(strings.TrimSpace(incoming.Name), existing.Name)
	merged.Kind = firstNonEmpty(strings.TrimSpace(incoming.Kind), existing.Kind)
	if merged.Input == nil {
		merged.Input = existing.Input
	}
	if merged.Output == "" {
		merged.Output = existing.Output
	}
	merged.Status = firstNonEmpty(strings.TrimSpace(incoming.Status), existing.Status)
	merged.Meta = mergeReconciledHistoryMap(existing.Meta, incoming.Meta)
	if merged.CommandGroup == nil {
		merged.CommandGroup = existing.CommandGroup
	}
	return &merged
}

func mergeReconciledHistoryMap(existing, incoming map[string]any) map[string]any {
	if len(existing) == 0 {
		return cloneMap(incoming)
	}
	merged := cloneMap(existing)
	if merged == nil {
		merged = make(map[string]any)
	}
	for key, value := range incoming {
		merged[key] = value
	}
	return merged
}

func historyTurnRowsEqual(left, right tables.WebSessionTurnTable) bool {
	return pointerString(left.SourceThreadID) == pointerString(right.SourceThreadID) &&
		pointerString(left.SourceTurnID) == pointerString(right.SourceTurnID) &&
		left.OrderIndex == right.OrderIndex &&
		left.Status == right.Status &&
		left.ErrorJSON == right.ErrorJSON &&
		left.SourceCreated == right.SourceCreated
}

func historyItemRowsEqual(left, right tables.WebSessionItemTable) bool {
	return pointerString(left.WebTurnID) == pointerString(right.WebTurnID) &&
		pointerString(left.SourceThreadID) == pointerString(right.SourceThreadID) &&
		pointerString(left.SourceTurnID) == pointerString(right.SourceTurnID) &&
		pointerString(left.SourceItemID) == pointerString(right.SourceItemID) &&
		pointerString(left.CommandGroupID) == pointerString(right.CommandGroupID) &&
		pointerString(left.RunID) == pointerString(right.RunID) &&
		optionalInt64Equal(left.RunDurationMs, right.RunDurationMs) &&
		left.RunOutcome == right.RunOutcome &&
		left.OrderIndex == right.OrderIndex &&
		left.LastEventSeq == right.LastEventSeq &&
		left.ItemKind == right.ItemKind &&
		left.ItemType == right.ItemType &&
		left.Role == right.Role &&
		left.Status == right.Status &&
		left.Level == right.Level &&
		left.Text == right.Text &&
		left.Done == right.Done &&
		optionalTimeEqual(left.Timestamp, right.Timestamp) &&
		optionalTimeEqual(left.ObservedAt, right.ObservedAt) &&
		left.AttachmentsJSON == right.AttachmentsJSON &&
		left.ToolJSON == right.ToolJSON &&
		left.DetailJSON == right.DetailJSON &&
		left.PayloadJSON == right.PayloadJSON
}

func (m *Manager) loadHistoryWindow(
	ctx context.Context,
	sessionID string,
	limit int,
	beforeOrder *int64,
) (HistoryWindow, error) {
	db := model.GetReaderDB()
	if db == nil {
		return HistoryWindow{}, model.ErrDBNotInitialized
	}
	if limit <= 0 {
		limit = DefaultHistoryWindow
	}

	query := db.WithContext(ctx).
		Model(&tables.WebSessionItemTable{}).
		Where("web_session_id = ?", sessionID)
	if beforeOrder != nil {
		query = query.Where("order_index < ?", *beforeOrder)
	}

	var total int64
	if err := db.WithContext(ctx).
		Model(&tables.WebSessionItemTable{}).
		Where("web_session_id = ?", sessionID).
		Count(&total).Error; err != nil {
		return HistoryWindow{}, err
	}

	var rows []tables.WebSessionItemTable
	if err := query.
		Order("order_index DESC").
		Limit(limit + 1).
		Find(&rows).Error; err != nil {
		return HistoryWindow{}, err
	}

	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}

	items := make([]HistoryItem, 0, len(rows))
	for index := len(rows) - 1; index >= 0; index-- {
		items = append(items, mapHistoryItemRowWithSession(rows[index], sessionID))
	}

	return HistoryWindow{
		Items:        items,
		HasMore:      hasMore,
		BeforeCursor: historyItemCursor(items, hasMore),
		Total:        int(total),
	}, nil
}

func (m *Manager) loadHistoryWindowAfter(
	ctx context.Context,
	sessionID string,
	limit int,
	afterOrder int64,
) (HistoryWindow, error) {
	db := model.GetReaderDB()
	if db == nil {
		return HistoryWindow{}, model.ErrDBNotInitialized
	}
	if limit <= 0 {
		limit = DefaultHistoryWindow
	}

	var total int64
	if err := db.WithContext(ctx).
		Model(&tables.WebSessionItemTable{}).
		Where("web_session_id = ?", sessionID).
		Count(&total).Error; err != nil {
		return HistoryWindow{}, err
	}

	var rows []tables.WebSessionItemTable
	if err := db.WithContext(ctx).
		Where("web_session_id = ? AND order_index > ?", sessionID, afterOrder).
		Order("order_index ASC").
		Limit(limit + 1).
		Find(&rows).Error; err != nil {
		return HistoryWindow{}, err
	}

	hasLater := len(rows) > limit
	if hasLater {
		rows = rows[:limit]
	}
	items := make([]HistoryItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, mapHistoryItemRowWithSession(row, sessionID))
	}

	return HistoryWindow{
		Items:       items,
		HasLater:    hasLater,
		AfterCursor: historyItemAfterCursor(items, hasLater),
		Total:       int(total),
	}, nil
}

func (m *Manager) findHistoryItemByID(
	ctx context.Context,
	sessionID string,
	itemID string,
) (HistoryItem, error) {
	db := model.GetDB()
	if db == nil {
		return HistoryItem{}, model.ErrDBNotInitialized
	}
	var row tables.WebSessionItemTable
	if err := db.WithContext(ctx).
		Where("web_session_id = ? AND id = ?", sessionID, itemID).
		First(&row).Error; err != nil {
		return HistoryItem{}, err
	}
	return mapHistoryItemRowWithSession(row, sessionID), nil
}

func (m *Manager) findHistoryItemByToolKey(
	ctx context.Context,
	sessionID string,
	toolID string,
) (HistoryItem, error) {
	db := model.GetDB()
	if db == nil {
		return HistoryItem{}, model.ErrDBNotInitialized
	}
	normalizedToolID := strings.TrimSpace(toolID)
	if normalizedToolID == "" {
		return HistoryItem{}, gorm.ErrRecordNotFound
	}

	indexedKeys := []string{historyToolSourceKey(normalizedToolID), normalizedToolID}
	var indexedRows []tables.WebSessionItemTable
	if err := db.WithContext(ctx).
		Where(
			"web_session_id = ? AND (source_item_id IN ? OR id = ?)",
			sessionID,
			indexedKeys,
			normalizedToolID,
		).
		Order("order_index DESC").
		Find(&indexedRows).Error; err != nil {
		return HistoryItem{}, err
	}
	for _, row := range indexedRows {
		item := mapHistoryItemRowWithSession(row, sessionID)
		if historyItemMatchesToolKey(item, normalizedToolID) {
			return item, nil
		}
	}

	// Legacy rows may not have a group source key. This fallback is intentionally
	// unbounded so an older group remains addressable regardless of its position.
	var toolRows []tables.WebSessionItemTable
	if err := db.WithContext(ctx).
		Where("web_session_id = ? AND item_kind = ?", sessionID, "tool").
		Order("order_index DESC").
		Find(&toolRows).Error; err != nil {
		return HistoryItem{}, err
	}
	for _, row := range toolRows {
		item := mapHistoryItemRowWithSession(row, sessionID)
		if historyItemMatchesToolKey(item, normalizedToolID) {
			return item, nil
		}
	}
	return HistoryItem{}, gorm.ErrRecordNotFound
}

func historyItemMatchesToolKey(item HistoryItem, toolID string) bool {
	if item.Tool == nil {
		return false
	}
	return item.Tool.ID == toolID ||
		(item.Tool.CommandGroup != nil && item.Tool.CommandGroup.ID == toolID)
}

func (m *Manager) registerExternalAttachment(path string) (HistoryAttachment, error) {
	normalizedPath := strings.TrimSpace(path)
	if normalizedPath == "" {
		return HistoryAttachment{}, fmt.Errorf("attachment path is required")
	}
	info, err := os.Stat(normalizedPath)
	if err != nil {
		return HistoryAttachment{}, err
	}
	sum := sha1.Sum([]byte(normalizedPath))
	attachmentID := fmt.Sprintf("ext_%x", sum[:8])
	meta := attachmentMeta{
		ID:        attachmentID,
		Name:      filepath.Base(normalizedPath),
		Mime:      mime.TypeByExtension(strings.ToLower(filepath.Ext(normalizedPath))),
		Size:      info.Size(),
		Path:      normalizedPath,
		CreatedAt: time.Now(),
	}
	metaBytes, err := json.Marshal(meta)
	if err == nil {
		_ = os.WriteFile(m.store.attachmentPath(attachmentID, ".json"), metaBytes, 0o644)
	}
	return HistoryAttachment{
		ID:   attachmentID,
		Name: meta.Name,
		Mime: meta.Mime,
		Size: meta.Size,
		Path: meta.Path,
	}, nil
}
