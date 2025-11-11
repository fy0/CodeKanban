package model

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go-template/model/tables"

	"gorm.io/gorm"
)

var (
	// ErrNotePadNotFound indicates the requested notepad does not exist.
	ErrNotePadNotFound = errors.New("notepad not found")
)

// NotePadService coordinates CRUD operations for notepads.
type NotePadService struct{}

// CreateNotePadRequest captures inputs required to create a notepad.
type CreateNotePadRequest struct {
	ProjectID *string
	Name      string
	Content   string
}

// UpdateNotePadRequest captures inputs for updating a notepad.
type UpdateNotePadRequest struct {
	Name    *string
	Content *string
}

// CreateNotePad inserts a new notepad row.
func (s *NotePadService) CreateNotePad(ctx context.Context, req *CreateNotePadRequest) (*tables.NotePadTable, error) {
	dbCtx, err := s.dbWithContext(ctx)
	if err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "新标签"
	}

	orderIndex, err := s.getNextOrderIndex(dbCtx, req.ProjectID)
	if err != nil {
		return nil, err
	}

	notepad := &tables.NotePadTable{
		ProjectID:  req.ProjectID,
		Name:       name,
		Content:    req.Content,
		OrderIndex: orderIndex,
	}

	if err := dbCtx.Create(notepad).Error; err != nil {
		return nil, err
	}

	return notepad, nil
}

// ListNotePads returns all notepads ordered by orderIndex.
// If projectID is nil, returns global notepads; otherwise returns project-specific notepads.
func (s *NotePadService) ListNotePads(ctx context.Context, projectID *string) ([]tables.NotePadTable, error) {
	dbCtx, err := s.dbWithContext(ctx)
	if err != nil {
		return nil, err
	}

	query := dbCtx.Model(&tables.NotePadTable{})
	if projectID == nil {
		query = query.Where("project_id IS NULL")
	} else {
		query = query.Where("project_id = ?", *projectID)
	}

	var notepads []tables.NotePadTable
	if err := query.
		Order("order_index ASC").
		Find(&notepads).Error; err != nil {
		return nil, err
	}

	return notepads, nil
}

// GetNotePad loads a notepad by identifier.
func (s *NotePadService) GetNotePad(ctx context.Context, id string) (*tables.NotePadTable, error) {
	dbCtx, err := s.dbWithContext(ctx)
	if err != nil {
		return nil, err
	}

	var notepad tables.NotePadTable
	if err := dbCtx.First(&notepad, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotePadNotFound
		}
		return nil, err
	}
	return &notepad, nil
}

// UpdateNotePad applies partial updates to a notepad record.
func (s *NotePadService) UpdateNotePad(ctx context.Context, id string, req *UpdateNotePadRequest) (*tables.NotePadTable, error) {
	dbCtx, err := s.dbWithContext(ctx)
	if err != nil {
		return nil, err
	}
	if req == nil {
		return s.GetNotePad(ctx, id)
	}

	notepad, err := s.GetNotePad(ctx, id)
	if err != nil {
		return nil, err
	}

	updates := map[string]interface{}{}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name != "" {
			updates["name"] = name
		}
	}
	if req.Content != nil {
		updates["content"] = *req.Content
	}

	if len(updates) > 0 {
		if err := dbCtx.Model(notepad).Updates(updates).Error; err != nil {
			return nil, err
		}
	}

	return s.GetNotePad(ctx, id)
}

// DeleteNotePad removes a notepad softly.
func (s *NotePadService) DeleteNotePad(ctx context.Context, id string) error {
	dbCtx, err := s.dbWithContext(ctx)
	if err != nil {
		return err
	}

	result := dbCtx.Delete(&tables.NotePadTable{}, "id = ?", id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotePadNotFound
	}
	return nil
}

// MoveNotePad updates the notepad order.
func (s *NotePadService) MoveNotePad(ctx context.Context, id string, orderIndex float64) (*tables.NotePadTable, error) {
	dbCtx, err := s.dbWithContext(ctx)
	if err != nil {
		return nil, err
	}

	notepad, err := s.GetNotePad(ctx, id)
	if err != nil {
		return nil, err
	}

	if err := dbCtx.Model(notepad).Update("order_index", orderIndex).Error; err != nil {
		return nil, err
	}

	return s.GetNotePad(ctx, id)
}

func (s *NotePadService) dbWithContext(ctx context.Context) (*gorm.DB, error) {
	db := GetDB()
	if db == nil {
		return nil, ErrDBNotInitialized
	}
	return db.WithContext(ensureContext(ctx)), nil
}

func (s *NotePadService) getNextOrderIndex(dbCtx *gorm.DB, projectID *string) (float64, error) {
	query := dbCtx.Model(&tables.NotePadTable{})
	if projectID == nil {
		query = query.Where("project_id IS NULL")
	} else {
		query = query.Where("project_id = ?", *projectID)
	}

	var maxOrder float64
	if err := query.
		Select("COALESCE(MAX(order_index), 0)").
		Scan(&maxOrder).Error; err != nil {
		return 0, err
	}
	return maxOrder + 1000, nil
}
