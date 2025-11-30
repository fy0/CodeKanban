package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"go.uber.org/zap"

	"code-kanban/api/h"
	"code-kanban/model"
	"code-kanban/service"
	"code-kanban/utils"
)

const worktreeTag = "worktree-工作树"

type createWorktreeInput struct {
	Body struct {
		BranchName   string `json:"branchName" doc:"分支名称" required:"true"`
		BaseBranch   string `json:"baseBranch" doc:"基础分支" default:""`
		CreateBranch bool   `json:"createBranch" doc:"是否创建新分支" default:"true"`
	} `json:"body"`
}

type commitWorktreeInput struct {
	Body struct {
		Message string `json:"message" doc:"提交信息" minLength:"1"`
	} `json:"body"`
}

func registerWorktreeRoutes(group *huma.Group) {
	worktreeSvc := service.NewWorktreeService()

	huma.Post(group, "/projects/{projectId}/worktrees/create", func(
		ctx context.Context,
		input *struct {
			ProjectID string `path:"projectId"`
			createWorktreeInput
		},
	) (*h.ItemResponse[model.Worktree], error) {
		worktree, err := worktreeSvc.CreateWorktree(
			ctx,
			input.ProjectID,
			input.Body.BranchName,
			input.Body.BaseBranch,
			input.Body.CreateBranch,
		)
		if err != nil {
			return nil, mapWorktreeError(err)
		}

		resp := h.NewItemResponse(*worktree)
		resp.Status = http.StatusCreated
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "worktree-create"
		op.Summary = "创建 Worktree"
		op.Tags = []string{worktreeTag}
	})

	huma.Get(group, "/projects/{projectId}/worktrees", func(
		ctx context.Context,
		input *struct {
			ProjectID string `path:"projectId"`
		},
	) (*h.ItemsResponse[*model.Worktree], error) {
		if err := worktreeSvc.SyncWorktrees(ctx, input.ProjectID); err != nil {
			switch {
			case errors.Is(err, model.ErrDBNotInitialized):
				return nil, huma.Error503ServiceUnavailable("database is not initialized")
			case errors.Is(err, model.ErrWorktreeNotFound):
				return nil, huma.Error404NotFound("project not found")
			default:
				utils.Logger().Warn("failed to sync worktrees before list",
					zap.Error(err),
					zap.String("projectId", input.ProjectID),
				)
			}
		}

		worktrees, err := worktreeSvc.ListWorktrees(ctx, input.ProjectID)
		if err != nil {
			if errors.Is(err, model.ErrDBNotInitialized) {
				return nil, huma.Error503ServiceUnavailable("database is not initialized")
			}
			return nil, huma.Error500InternalServerError("failed to list worktrees", err)
		}

		resp := h.NewItemsResponse(worktrees)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "worktree-list-by-project"
		op.Summary = "获取 Worktree 列表"
		op.Tags = []string{worktreeTag}
	})

	huma.Post(group, "/worktrees/{id}", func(
		ctx context.Context,
		input *struct {
			ID           string `path:"id"`
			Force        bool   `query:"force" default:"false"`
			DeleteBranch bool   `query:"deleteBranch" default:"true"`
		},
	) (*h.MessageResponse, error) {
		if err := worktreeSvc.DeleteWorktree(ctx, input.ID, input.Force, input.DeleteBranch); err != nil {
			return nil, mapWorktreeError(err)
		}

		resp := h.NewMessageResponse("worktree deleted successfully")
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "worktree-delete"
		op.Summary = "删除 Worktree"
		op.Tags = []string{worktreeTag}
	})

	huma.Post(group, "/worktrees/{id}/commit", func(
		ctx context.Context,
		input *struct {
			ID string `path:"id"`
			commitWorktreeInput
		},
	) (*h.ItemResponse[model.Worktree], error) {
		worktree, err := worktreeSvc.CommitWorktree(ctx, input.ID, input.Body.Message)
		if err != nil {
			return nil, mapWorktreeError(err)
		}
		resp := h.NewItemResponse(*worktree)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "worktree-commit"
		op.Summary = "提交 Worktree 更改"
		op.Tags = []string{worktreeTag}
	})

	huma.Post(group, "/worktrees/{id}/refresh-status", func(
		ctx context.Context,
		input *struct {
			ID string `path:"id"`
		},
	) (*h.ItemResponse[model.Worktree], error) {
		worktree, err := worktreeSvc.RefreshWorktreeStatus(ctx, input.ID)
		if err != nil {
			return nil, mapWorktreeError(err)
		}

		resp := h.NewItemResponse(*worktree)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "worktree-refresh-status"
		op.Summary = "刷新 Worktree 状态"
		op.Tags = []string{worktreeTag}
	})

	huma.Post(group, "/projects/{projectId}/refresh-all-worktrees", func(
		ctx context.Context,
		input *struct {
			ProjectID string `path:"projectId"`
		},
	) (*h.ItemResponse[refreshAllResult], error) {
		updated, failed, err := worktreeSvc.RefreshAllWorktrees(ctx, input.ProjectID)
		if err != nil {
			return nil, mapWorktreeError(err)
		}

		resp := h.NewItemResponse(refreshAllResult{
			Updated: updated,
			Failed:  failed,
		})
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "worktree-refresh-all-by-project"
		op.Summary = "刷新所有 Worktree 状态"
		op.Tags = []string{worktreeTag}
	})

	huma.Post(group, "/projects/{projectId}/sync-worktrees", func(
		ctx context.Context,
		input *struct {
			ProjectID string `path:"projectId"`
		},
	) (*h.MessageResponse, error) {
		if err := worktreeSvc.SyncWorktrees(ctx, input.ProjectID); err != nil {
			return nil, mapWorktreeError(err)
		}
		resp := h.NewMessageResponse("worktrees synced successfully")
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "worktree-sync-by-project"
		op.Summary = "同步 Worktree"
		op.Tags = []string{worktreeTag}
	})

	huma.Post(group, "/projects/{projectId}/worktrees/refresh-commits", func(
		ctx context.Context,
		input *struct {
			ProjectID string `path:"projectId"`
		},
	) (*h.ItemsResponse[*model.Worktree], error) {
		worktrees, err := worktreeSvc.RefreshWorktreeCommitInfo(ctx, input.ProjectID)
		if err != nil {
			return nil, mapWorktreeError(err)
		}
		resp := h.NewItemsResponse(worktrees)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "worktree-refresh-commit-info"
		op.Summary = "刷新 Worktree 的提交信息"
		op.Tags = []string{worktreeTag}
	})
}

func mapWorktreeError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, model.ErrDBNotInitialized):
		return huma.Error503ServiceUnavailable("database is not initialized")
	case errors.Is(err, model.ErrWorktreeNotFound),
		errors.Is(err, model.ErrProjectNotFound):
		return huma.Error404NotFound(err.Error())
	case errors.Is(err, model.ErrWorktreeIsMain),
		errors.Is(err, model.ErrWorktreeHasTasks):
		return huma.Error409Conflict(err.Error())
	case errors.Is(err, model.ErrWorktreeClean):
		return huma.Error400BadRequest(err.Error())
	default:
		return huma.Error400BadRequest(err.Error())
	}
}

type refreshAllResult struct {
	Updated int `json:"updated" doc:"刷新成功数量"`
	Failed  int `json:"failed" doc:"刷新失败数量"`
}
