package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"go-template/api/h"
	"go-template/model"
)

const projectTag = "project-项目管理"

type createProjectInput struct {
	Body struct {
		Name             string  `json:"name" minLength:"1" maxLength:"100" doc:"项目名称"`
		Path             string  `json:"path" minLength:"1" doc:"本地项目目录路径（可非 Git 仓库）"`
		Description      string  `json:"description" doc:"项目描述"`
		WorktreeBasePath *string `json:"worktreeBasePath,omitempty" doc:"Worktree 基础路径（可选，默认为项目目录下的 .worktrees 子目录）"`
		HidePath         *bool   `json:"hidePath,omitempty" doc:"是否隐藏真实路径"`
	}
}

type updateProjectInput struct {
	ID   string `path:"id"`
	Body struct {
		Name        string `json:"name" minLength:"1" maxLength:"100" doc:"项目名称"`
		Description string `json:"description" doc:"项目描述"`
		HidePath    bool   `json:"hidePath" doc:"是否隐藏真实路径"`
	}
}

func registerProjectRoutes(group *huma.Group) {
	service := model.NewProjectService()

	huma.Post(group, "/projects/create", func(ctx context.Context, input *createProjectInput) (*h.ItemResponse[model.Project], error) {
		worktreeBasePath := ""
		if input.Body.WorktreeBasePath != nil {
			worktreeBasePath = *input.Body.WorktreeBasePath
		}
		hidePath := false
		if input.Body.HidePath != nil {
			hidePath = *input.Body.HidePath
		}

		project, err := service.CreateProject(ctx, model.CreateProjectParams{
			Name:             input.Body.Name,
			Path:             input.Body.Path,
			Description:      input.Body.Description,
			WorktreeBasePath: worktreeBasePath,
			HidePath:         hidePath,
		})
		if err != nil {
			switch {
			case errors.Is(err, model.ErrDBNotInitialized):
				return nil, huma.Error503ServiceUnavailable("database is not initialized")
			case errors.Is(err, model.ErrInvalidProjectInput):
				return nil, huma.Error400BadRequest(err.Error())
			case errors.Is(err, model.ErrInvalidProjectPath):
				return nil, huma.Error400BadRequest(err.Error())
			case errors.Is(err, model.ErrInvalidGitRepository):
				return nil, huma.Error400BadRequest(err.Error())
			case errors.Is(err, model.ErrProjectAlreadyExists):
				return nil, huma.Error409Conflict("project already exists")
			default:
				return nil, huma.Error500InternalServerError("failed to create project", err)
			}
		}

		resp := h.NewItemResponse(*project)
		resp.Status = http.StatusCreated
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "project-create"
		op.Summary = "创建项目"
		op.Tags = []string{projectTag}
	})

	huma.Get(group, "/projects", func(ctx context.Context, _ *struct{}) (*h.ItemsResponse[*model.Project], error) {
		projects, err := service.ListProjects(ctx)
		if err != nil {
			if errors.Is(err, model.ErrDBNotInitialized) {
				return nil, huma.Error503ServiceUnavailable("database is not initialized")
			}
			return nil, huma.Error500InternalServerError("failed to load projects", err)
		}

		resp := h.NewItemsResponse(projects)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "project-list"
		op.Summary = "项目列表"
		op.Tags = []string{projectTag}
	})

	huma.Get(group, "/projects/{id}", func(ctx context.Context, input *struct {
		ID string `path:"id"`
	}) (*h.ItemResponse[model.Project], error) {
		project, err := service.GetProject(ctx, input.ID)
		if err != nil {
			if errors.Is(err, model.ErrDBNotInitialized) {
				return nil, huma.Error503ServiceUnavailable("database is not initialized")
			}
			if errors.Is(err, model.ErrProjectNotFound) {
				return nil, huma.Error404NotFound("project not found")
			}
			return nil, huma.Error500InternalServerError("failed to load project", err)
		}

		resp := h.NewItemResponse(*project)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "project-get-by-id"
		op.Summary = "获取项目详情"
		op.Tags = []string{projectTag}
	})

	huma.Post(group, "/projects/{id}/update", func(ctx context.Context, input *updateProjectInput) (*h.ItemResponse[model.Project], error) {
		project, err := service.UpdateProject(ctx, input.ID, model.UpdateProjectParams{
			Name:        input.Body.Name,
			Description: input.Body.Description,
			HidePath:    input.Body.HidePath,
		})
		if err != nil {
			switch {
			case errors.Is(err, model.ErrDBNotInitialized):
				return nil, huma.Error503ServiceUnavailable("database is not initialized")
			case errors.Is(err, model.ErrInvalidProjectInput):
				return nil, huma.Error400BadRequest(err.Error())
			case errors.Is(err, model.ErrProjectNotFound):
				return nil, huma.Error404NotFound("project not found")
			default:
				return nil, huma.Error500InternalServerError("failed to update project", err)
			}
		}

		resp := h.NewItemResponse(*project)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "project-update"
		op.Summary = "编辑项目"
		op.Tags = []string{projectTag}
	})

	huma.Post(group, "/projects/{id}/delete", func(ctx context.Context, input *struct {
		ID string `path:"id"`
	}) (*h.MessageResponse, error) {
		if err := service.DeleteProject(ctx, input.ID); err != nil {
			if errors.Is(err, model.ErrDBNotInitialized) {
				return nil, huma.Error503ServiceUnavailable("database is not initialized")
			}
			if errors.Is(err, model.ErrProjectNotFound) {
				return nil, huma.Error404NotFound("project not found")
			}
			return nil, huma.Error500InternalServerError("failed to delete project", err)
		}

		resp := h.NewMessageResponse("Project deleted successfully")
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "project-delete"
		op.Summary = "删除项目"
		op.Tags = []string{projectTag}
	})
}
