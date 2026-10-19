package model

import (
	"reflect"
	"time"

	"code-kanban/model/tables"
	"code-kanban/utils"
	"go.uber.org/zap"
)

func GetAllModels() []any {
	return []any{
		&tables.UserTable{},
		&tables.UserAccessTokenTable{},
		&tables.ProjectTable{},
		&tables.ProjectAgentTrustTable{},
		&tables.WorktreeTable{},
		&tables.NotePadTable{},
		&tables.AISessionTable{},
		&tables.TerminalRestoreSessionTable{},
		&tables.WebSessionTable{},
		&tables.WebSessionScheduledInputTable{},
		&tables.WebSessionTurnTable{},
		&tables.WebSessionItemTable{},
		&tables.WebSessionRunTimingTable{},
		&tables.WebSessionTokenSpeedTable{},
		&tables.WebSessionSubAgentTable{},
	}
}

func DBMigrate(autoMigrate bool) {
	if !autoMigrate || db == nil {
		return
	}

	logger := utils.Logger()
	logger.Info("database migration started")

	dropRemovedKanbanArtifacts(logger)

	for _, model := range GetAllModels() {
		if err := db.AutoMigrate(model); err != nil {
			logger.Error("database migration failed",
				zap.Error(err),
				zap.String("model", reflect.TypeOf(model).String()),
			)
			panic(err)
		}
	}

	backfillStartedAt := time.Now()
	backfilledRows, err := backfillWebSessionItemCommandGroupIDs()
	if err != nil {
		logger.Error("web session command group id backfill failed", zap.Error(err))
		panic(err)
	}
	if backfilledRows > 0 {
		logger.Info("web session command group ids backfilled",
			zap.Int64("rowCount", backfilledRows),
			zap.Duration("duration", time.Since(backfillStartedAt)),
		)
	}

	if backfilledRows, err := backfillWebSessionSubAgentActive(); err != nil {
		logger.Error("web session sub-agent active state backfill failed", zap.Error(err))
		panic(err)
	} else if backfilledRows > 0 {
		logger.Info("web session sub-agent active states backfilled",
			zap.Int64("rowCount", backfilledRows),
		)
	}

	if backfilledRows, err := backfillScheduledInputContextWindowSnapshots(); err != nil {
		logger.Error("scheduled input context window snapshots backfill failed", zap.Error(err))
		panic(err)
	} else if backfilledRows > 0 {
		logger.Info("scheduled input context window snapshots backfilled",
			zap.Int64("rowCount", backfilledRows),
		)
	}

	logger.Info("database migration finished")
}

func backfillWebSessionSubAgentActive() (int64, error) {
	if db == nil {
		return 0, ErrDBNotInitialized
	}
	result := db.Exec(`
		UPDATE web_session_sub_agents
		SET is_active = CASE
			WHEN status IN ('pending_init', 'running') THEN 1
			ELSE 0
		END
		WHERE is_active = 0
		  AND status IN ('pending_init', 'running')
	`)
	return result.RowsAffected, result.Error
}

func backfillWebSessionItemCommandGroupIDs() (int64, error) {
	if db == nil {
		return 0, ErrDBNotInitialized
	}

	result := db.Exec(`
		UPDATE web_session_items
		SET command_group_id = CASE
			WHEN json_valid(tool_json) THEN COALESCE(
				NULLIF(TRIM(CAST(json_extract(tool_json, '$.commandGroup.id') AS TEXT)), ''),
				''
			)
			ELSE ''
		END
		WHERE command_group_id IS NULL
	`)
	return result.RowsAffected, result.Error
}

func backfillScheduledInputContextWindowSnapshots() (int64, error) {
	if db == nil {
		return 0, ErrDBNotInitialized
	}

	result := db.Exec(`
		UPDATE web_session_scheduled_inputs
		SET context_window_setting_snapshot = (
			SELECT CASE
				WHEN web_sessions.agent = 'codex' THEN web_sessions.context_window_setting
				ELSE NULL
			END
			FROM web_sessions
			WHERE web_sessions.id = web_session_scheduled_inputs.web_session_id
		)
		WHERE context_window_setting_snapshot IS NULL
		  AND EXISTS (
			SELECT 1
			FROM web_sessions
			WHERE web_sessions.id = web_session_scheduled_inputs.web_session_id
			  AND web_sessions.agent = 'codex'
		  )
	`)
	return result.RowsAffected, result.Error
}

func dropRemovedKanbanArtifacts(logger *zap.Logger) {
	for _, tableName := range []string{"task_ai_sessions", "task_comments", "tasks"} {
		if !db.Migrator().HasTable(tableName) {
			continue
		}
		if err := db.Migrator().DropTable(tableName); err != nil {
			logger.Error("removed kanban table cleanup failed",
				zap.Error(err),
				zap.String("table", tableName),
			)
			panic(err)
		}
		logger.Info("removed kanban table", zap.String("table", tableName))
	}

	legacyRestoreTable := &tables.TerminalRestoreSessionTable{}
	if db.Migrator().HasTable(legacyRestoreTable) && db.Migrator().HasColumn(legacyRestoreTable, "task_id") {
		if err := db.Migrator().DropColumn(legacyRestoreTable, "task_id"); err != nil {
			logger.Error("removed kanban column cleanup failed",
				zap.Error(err),
				zap.String("table", legacyRestoreTable.TableName()),
				zap.String("column", "task_id"),
			)
			panic(err)
		}
		logger.Info("removed kanban column", zap.String("table", legacyRestoreTable.TableName()), zap.String("column", "task_id"))
	}
}
