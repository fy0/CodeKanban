package tables

import (
	"strings"
	"testing"

	"code-kanban/utils/model_base"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsnName := strings.ReplaceAll(t.Name(), "/", "_")
	db, err := model_base.DBInit("file:"+dsnName+"?mode=memory&cache=shared", logger.Silent)
	if err != nil {
		t.Fatalf("failed to open in-memory sqlite: %v", err)
	}
	t.Cleanup(func() {
		model_base.DBClose(db)
	})

	if err := db.AutoMigrate(
		&ProjectTable{},
		&WorktreeTable{},
	); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}

	return db
}
