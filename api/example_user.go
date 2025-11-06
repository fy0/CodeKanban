package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"go-template/api/h"
	"go-template/model"
	"go-template/model/tables"
	"gorm.io/gorm"
)

type exampleUserDTO struct {
	ID        string    `json:"id" doc:"Unique identifier"`
	Name      string    `json:"name" doc:"Display name"`
	Note      string    `json:"note" doc:"Optional note"`
	CreatedAt time.Time `json:"createdAt" doc:"Creation timestamp"`
	UpdatedAt time.Time `json:"updatedAt" doc:"Last update timestamp"`
}

func newExampleUserDTO(model *tables.ExampleUserTable) exampleUserDTO {
	return exampleUserDTO{
		ID:        model.ID,
		Name:      model.Name,
		Note:      model.Note,
		CreatedAt: model.CreatedAt,
		UpdatedAt: model.UpdatedAt,
	}
}

type createExampleUserInput struct {
	Body struct {
		Name string `json:"name" doc:"Display name" required:"true" example:"Alice"`
		Note string `json:"note" doc:"Optional note" default:"" example:"VIP"`
	} `json:"body"`
}

type listExampleUsersInput struct {
	Page     int `query:"page" default:"1" doc:"Page number (1-based)"`
	PageSize int `query:"pageSize" default:"20" doc:"Page size (max 100)"`
}

type getExampleUserInput struct {
	ID string `path:"id" doc:"Example user ID"`
}

func registerExampleUserRoutes(group *huma.Group) {
	huma.Post(group, "/example-users", func(ctx context.Context, input *createExampleUserInput) (*h.ItemResponse[exampleUserDTO], error) {
		name := strings.TrimSpace(input.Body.Name)
		if name == "" {
			return nil, huma.Error400BadRequest("name is required")
		}

		user, err := model.CreateExampleUser(ctx, name, strings.TrimSpace(input.Body.Note))
		if err != nil {
			if errors.Is(err, model.ErrDBNotInitialized) {
				return nil, huma.Error503ServiceUnavailable("database is not initialized")
			}
			return nil, huma.Error500InternalServerError("failed to create example user", err)
		}

		resp := h.NewItemResponse(newExampleUserDTO(user))
		resp.Status = http.StatusCreated
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "createExampleUser"
		op.Summary = "Create an example user"
	})

	huma.Get(group, "/example-users", func(ctx context.Context, input *listExampleUsersInput) (*h.PaginatedResponse[exampleUserDTO], error) {
		page := input.Page
		if page < 1 {
			page = 1
		}
		pageSize := input.PageSize
		if pageSize <= 0 {
			pageSize = 20
		}
		if pageSize > 100 {
			pageSize = 100
		}

		items, total, err := model.ListExampleUsers(ctx, page, pageSize)
		if err != nil {
			if errors.Is(err, model.ErrDBNotInitialized) {
				return nil, huma.Error503ServiceUnavailable("database is not initialized")
			}
			return nil, huma.Error500InternalServerError("failed to query example users", err)
		}

		result := make([]exampleUserDTO, len(items))
		for i := range items {
			result[i] = newExampleUserDTO(&items[i])
		}

		resp := h.NewPaginatedResponse(result, total, page, pageSize)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "listExampleUsers"
		op.Summary = "List example users"
	})

	huma.Get(group, "/example-users/{id}", func(ctx context.Context, input *getExampleUserInput) (*h.ItemResponse[exampleUserDTO], error) {
		user, err := model.GetExampleUser(ctx, input.ID)
		if err != nil {
			if errors.Is(err, model.ErrDBNotInitialized) {
				return nil, huma.Error503ServiceUnavailable("database is not initialized")
			}
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, huma.Error404NotFound("example user not found")
			}
			return nil, huma.Error500InternalServerError("failed to load example user", err)
		}

		resp := h.NewItemResponse(newExampleUserDTO(user))
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "getExampleUser"
		op.Summary = "Get a single example user"
	})
}
