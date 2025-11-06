package model

import (
	"context"
	"errors"

	"go-template/model/tables"
)

// ErrDBNotInitialized indicates the shared GORM instance has not been set up.
var ErrDBNotInitialized = errors.New("gorm database is not initialized")

// CreateExampleUser inserts a new ExampleUserTable row.
func CreateExampleUser(ctx context.Context, name, note string) (*tables.ExampleUserTable, error) {
	db := GetDB()
	if db == nil {
		return nil, ErrDBNotInitialized
	}

	user := &tables.ExampleUserTable{}
	user.Init()
	user.Name = name
	user.Note = note

	if err := db.WithContext(ctx).Create(user).Error; err != nil {
		return nil, err
	}

	return user, nil
}

// ListExampleUsers returns a page of users alongside the total count.
func ListExampleUsers(ctx context.Context, page, pageSize int) ([]tables.ExampleUserTable, int64, error) {
	db := GetDB()
	if db == nil {
		return nil, 0, ErrDBNotInitialized
	}

	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	offset := (page - 1) * pageSize

	var total int64
	if err := db.WithContext(ctx).Model(&tables.ExampleUserTable{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	items := make([]tables.ExampleUserTable, 0, pageSize)
	if err := db.WithContext(ctx).
		Order("created_at DESC").
		Limit(pageSize).
		Offset(offset).
		Find(&items).Error; err != nil {
		return nil, 0, err
	}

	return items, total, nil
}

// GetExampleUser returns a single ExampleUserTable row.
func GetExampleUser(ctx context.Context, id string) (*tables.ExampleUserTable, error) {
	db := GetDB()
	if db == nil {
		return nil, ErrDBNotInitialized
	}

	var user tables.ExampleUserTable
	if err := db.WithContext(ctx).First(&user, "id = ?", id).Error; err != nil {
		return nil, err
	}

	return &user, nil
}
