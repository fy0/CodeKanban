package model_base

import (
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func sqliteOpen(dsn string) gorm.Dialector {
	return sqlite.Open(dsn)
}

type sqliteDialector = sqlite.Dialector
