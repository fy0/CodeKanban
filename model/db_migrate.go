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
		&tables.WorktreeTable{},
		&tables.TaskTable{},
		&tables.TaskCommentTable{},
		&tables.NotePadTable{},
	}
}

func DBMigrate(autoMigrate bool) {
	if !autoMigrate || db == nil {
		return
	}

	logger := utils.Logger()
	logger.Info("database migration started")

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
