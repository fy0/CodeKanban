package model

import (
	"reflect"
	"time"

	"code-kanban/model/tables"
	"code-kanban/utils"

	"go.uber.org/zap"
)

const slowModelMigrationThreshold = 500 * time.Millisecond

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
	migrationStartedAt := time.Now()
	logger.Info("database migration started")

	cleanupStartedAt := time.Now()
	dropRemovedKanbanArtifacts(logger)
	logger.Info("database migration legacy cleanup finished",
		zap.Duration("duration", time.Since(cleanupStartedAt)),
	)

	schemaStartedAt := time.Now()
	models := GetAllModels()
	for _, model := range models {
		modelStartedAt := time.Now()
		err := db.AutoMigrate(model)
		duration := time.Since(modelStartedAt)
		modelName := reflect.TypeOf(model).String()
		if err != nil {
			logger.Error("database migration failed",
				zap.Error(err),
				zap.String("model", modelName),
				zap.Duration("duration", duration),
			)
			panic(err)
		}
		level := zap.DebugLevel
		if duration >= slowModelMigrationThreshold {
			level = zap.InfoLevel
		}
		logger.Log(level, "database model migration finished",
			zap.String("model", modelName),
			zap.Duration("duration", duration),
		)
	}
	logger.Info("database schema migration finished",
		zap.Int("modelCount", len(models)),
		zap.Duration("duration", time.Since(schemaStartedAt)),
	)

	runMigrationBackfill(logger, "web_session_item_command_group_ids", backfillWebSessionItemCommandGroupIDs)
	runMigrationBackfill(logger, "web_session_sub_agent_active", backfillWebSessionSubAgentActive)
	runMigrationBackfill(logger, "scheduled_input_context_window_snapshots", backfillScheduledInputContextWindowSnapshots)

	logger.Info("database migration finished", zap.Duration("duration", time.Since(migrationStartedAt)))
}

func runMigrationBackfill(logger *zap.Logger, step string, backfill func() (int64, error)) {
	backfillStartedAt := time.Now()
	backfilledRows, err := backfill()
	duration := time.Since(backfillStartedAt)
	if err != nil {
		logger.Error("database migration backfill failed",
			zap.String("step", step),
			zap.Duration("duration", duration),
			zap.Error(err),
		)
		panic(err)
	}
	logger.Info("database migration backfill finished",
		zap.String("step", step),
		zap.Int64("rowCount", backfilledRows),
		zap.Duration("duration", duration),
	)
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
		dropStartedAt := time.Now()
		if err := db.Migrator().DropTable(tableName); err != nil {
			logger.Error("removed kanban table cleanup failed",
				zap.Error(err),
				zap.String("table", tableName),
				zap.Duration("duration", time.Since(dropStartedAt)),
			)
			panic(err)
		}
		logger.Info("removed kanban table",
			zap.String("table", tableName),
			zap.Duration("duration", time.Since(dropStartedAt)),
		)
	}

	legacyRestoreTable := &tables.TerminalRestoreSessionTable{}
	if db.Migrator().HasTable(legacyRestoreTable) && db.Migrator().HasColumn(legacyRestoreTable, "task_id") {
		dropStartedAt := time.Now()
		if err := db.Migrator().DropColumn(legacyRestoreTable, "task_id"); err != nil {
			logger.Error("removed kanban column cleanup failed",
				zap.Error(err),
				zap.String("table", legacyRestoreTable.TableName()),
				zap.String("column", "task_id"),
				zap.Duration("duration", time.Since(dropStartedAt)),
			)
			panic(err)
		}
		logger.Info("removed kanban column",
			zap.String("table", legacyRestoreTable.TableName()),
			zap.String("column", "task_id"),
			zap.Duration("duration", time.Since(dropStartedAt)),
		)
	}
}
