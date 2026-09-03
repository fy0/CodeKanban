package websession

import (
	"context"

	"code-kanban/model"
	"code-kanban/model/tables"

	"gorm.io/gorm"
)

const legacyPiExtensionStatusNoteCode = "pi_extension_ui_setStatus"

func (m *Manager) cleanupLegacyPiStatusNotes(ctx context.Context) (int64, error) {
	db := model.GetDB()
	if db == nil {
		return 0, model.ErrDBNotInitialized
	}

	var removed int64
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var sessionIDs []string
		if err := tx.Model(&tables.WebSessionItemTable{}).
			Distinct("web_session_items.web_session_id").
			Joins("JOIN web_sessions ON web_sessions.id = web_session_items.web_session_id").
			Where("web_sessions.deleted_at IS NULL AND web_sessions.agent = ?", string(AgentPi)).
			Where("web_session_items.item_type = ?", "note").
			Where("CASE WHEN json_valid(web_session_items.payload_json) = 1 THEN json_extract(web_session_items.payload_json, '$.code') END = ?", legacyPiExtensionStatusNoteCode).
			Pluck("web_session_items.web_session_id", &sessionIDs).Error; err != nil {
			return err
		}
		if len(sessionIDs) == 0 {
			return nil
		}

		result := tx.Unscoped().
			Where("web_session_id IN ?", sessionIDs).
			Where("deleted_at IS NULL").
			Where("item_type = ?", "note").
			Where("CASE WHEN json_valid(payload_json) = 1 THEN json_extract(payload_json, '$.code') END = ?", legacyPiExtensionStatusNoteCode).
			Delete(&tables.WebSessionItemTable{})
		if result.Error != nil {
			return result.Error
		}
		removed = result.RowsAffected
		if removed == 0 {
			return nil
		}

		for _, sessionID := range sessionIDs {
			var itemCount int64
			if err := tx.Model(&tables.WebSessionItemTable{}).
				Where("web_session_id = ?", sessionID).
				Count(&itemCount).Error; err != nil {
				return err
			}
			updates := withSnapshotRevisionIncrement(withHistoryEpochIncrement(map[string]any{
				"item_count": int(itemCount),
			}))
			if err := tx.Model(&tables.WebSessionTable{}).
				Where("id = ?", sessionID).
				UpdateColumns(updates).Error; err != nil {
				return err
			}
		}
		return nil
	})
	return removed, err
}
