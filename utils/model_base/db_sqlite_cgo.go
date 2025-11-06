//go:build xcgo
// +build xcgo

package model_base

import (
	_ "github.com/mattn/go-sqlite3" // sqlite3 driver
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func sqliteOpen(dsn string) gorm.Dialector {
	return sqlite.Open(dsn)
}

type sqliteDialector = sqlite.Dialector
