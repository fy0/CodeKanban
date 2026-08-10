package model

import (
	"reflect"

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

	logger.Info("database migration finished")
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
