package model

import (
	sqlite_tools "go-template/utils/db_tools/sqlite"
	"go-template/utils/model_base"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var db *gorm.DB

func InitWithDSN(dsn string, logLevel int, autoMigrate bool) error {
	var err error
	db, err = model_base.DBInit(dsn, logger.LogLevel(logLevel))
	if err != nil {
		return err
	}

	DBMigrate(autoMigrate)
	sqlcInit(dsn)
	return nil
}

func DBClose() {
	if db != nil {
		model_base.DBClose(db)
	}
}

func GetDB() *gorm.DB {
	return db
}

var _sqlInfo = sqlite_tools.TransactionCreateFactory(func(tx any) *Queries {
	return New(tx.(DBTX))
})

var Transaction = _sqlInfo.TransactionCreate
var GetQ = _sqlInfo.GetQ
var sqlcInit = _sqlInfo.InitFunc
