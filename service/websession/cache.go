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

func parseHistoryCursorValue(cursor string) (*int64, error) {
	cursor = strings.TrimSpace(cursor)
	if cursor == "" {
		return nil, nil
	}
	value, err := strconv.ParseInt(cursor, 10, 64)
	if err != nil {
		return nil, err
	}
	return &value, nil
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
	row.RunID = item.RunID
	row.RunDurationMs = item.RunDurationMs
	row.RunOutcome = string(item.RunOutcome)
	row.OrderIndex = item.OrderIndex
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

func (m *Manager) nextHistoryOrderIndex(ctx context.Context, sessionID string) (int64, error) {
	db := model.GetDB()
	if db == nil {
		return 0, model.ErrDBNotInitialized
	}
	return m.nextHistoryOrderIndexDB(ctx, db, sessionID)
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

func (m *Manager) upsertHistoryItemBySourceID(
	ctx context.Context,
	sessionID string,
	sourceItemID string,
	mutate func(*HistoryItem),
) (HistoryItem, error) {
	return m.upsertHistoryItemBySourceIdentity(ctx, sessionID, "", sourceItemID, mutate)
}

func (m *Manager) upsertHistoryItemBySourceIdentity(
	ctx context.Context,
	sessionID string,
	sourceThreadID string,
	sourceItemID string,
	mutate func(*HistoryItem),
) (HistoryItem, error) {
	db := model.GetDB()
	if db == nil {
		return HistoryItem{}, model.ErrDBNotInitialized
	}
	return m.upsertHistoryItemBySourceIdentityDB(ctx, db, sessionID, sourceThreadID, sourceItemID, mutate)
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

func (m *Manager) ensureCompactGroupHistorySourceKey(
	ctx context.Context,
	sessionID string,
	sourceThreadID string,
	sourceKey string,
	groupID string,
) error {
	db := model.GetDB()
	if db == nil {
		return model.ErrDBNotInitialized
	}
	return m.ensureCompactGroupHistorySourceKeyDB(ctx, db, sessionID, sourceThreadID, sourceKey, groupID)
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

	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []tables.WebSessionItemTable
		query := tx.Where("web_session_id = ? AND item_kind = ?", sessionID, "tool")
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

		candidates := make([]candidateRow, 0)
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
			if err := tx.Unscoped().Where("id IN ?", deleteIDs).Delete(&tables.WebSessionItemTable{}).Error; err != nil {
				return err
			}
		}
		return nil
	})
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
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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
		if len(updates) > 0 {
			if err := tx.Model(&tables.WebSessionTable{}).
				Where("id = ?", session.ID).
				Updates(withSnapshotRevisionIncrement(updates)).Error; err != nil {
				return err
			}
		}
		return nil
	})
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
