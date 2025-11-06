package model

import (
	"go-template/model/tables"
)

func GetAllModels() []any {
	return []any{
		&tables.ExampleUserTable{},
	}
}

func DBMigrate(autoMigrate bool) {
	if !autoMigrate {
		return
	}

	db.AutoMigrate(GetAllModels()...)
}
