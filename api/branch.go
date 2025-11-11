package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"go-template/api/h"
	"go-template/model"
	"go-template/service"
)

const branchTag = "branch-分支管理"

type createBranchBody struct {
	Name           string `json:"name" minLength:"1" doc:"分支名称"`
	Base           string `json:"base" doc:"基础分支" default:""`
	CreateWorktree bool   `json:"createWorktree" doc:"同时创建 Worktree" default:"false"`
}

type mergeBranchBody struct {
	TargetBranch  string `json:"targetBranch" minLength:"1" doc:"目标分支"`
	SourceBranch  string `json:"sourceBranch" minLength:"1" doc:"源分支"`
	Strategy      string `json:"strategy" enum:"merge,rebase,squash" doc:"合并策略" default:"merge"`
	Commit        bool   `json:"commit" doc:"Squash 合并后立即提交" default:"false"`
	CommitMessage string `json:"commitMessage" doc:"提交信息（仅 squash 合并生效）" default:""`
}

func registerBranchRoutes(group *huma.Group) {
	branchSvc := service.NewBranchService()

	huma.Get(group, "/projects/{projectId}/branches", func(
		ctx context.Context,
		input *struct {
			ProjectID string `path:"projectId"`
		},
	) (*h.ItemResponse[model.BranchListResult], error) {
		result, err := branchSvc.ListBranches(ctx, input.ProjectID)
		if err != nil {
			return nil, mapBranchError(err)
		}
		resp := h.NewItemResponse(*result)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "branch-list"
		op.Summary = "获取分支列表"
		op.Tags = []string{branchTag}
	})

	huma.Post(group, "/projects/{projectId}/branches", func(
		ctx context.Context,
		input *struct {
			ProjectID string `path:"projectId"`
			Body      createBranchBody
		},
	) (*h.MessageResponse, error) {
		if err := branchSvc.CreateBranch(ctx, input.ProjectID, input.Body.Name, input.Body.Base, input.Body.CreateWorktree); err != nil {
			return nil, mapBranchError(err)
		}
		resp := h.NewMessageResponse("branch created successfully")
		resp.Status = http.StatusCreated
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "branch-create"
		op.Summary = "创建分支"
		op.Tags = []string{branchTag}
	})

	huma.Delete(group, "/projects/{projectId}/branches/{branchName}", func(
		ctx context.Context,
		input *struct {
			ProjectID  string `path:"projectId"`
			BranchName string `path:"branchName"`
			Force      bool   `query:"force" default:"false" doc:"强制删除"`
		},
	) (*h.MessageResponse, error) {
		if err := branchSvc.DeleteBranch(ctx, input.ProjectID, input.BranchName, input.Force); err != nil {
			return nil, mapBranchError(err)
		}
		resp := h.NewMessageResponse("branch deleted successfully")
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "branch-delete"
		op.Summary = "删除分支"
		op.Tags = []string{branchTag}
	})

	huma.Post(group, "/worktrees/{id}/merge", func(
		ctx context.Context,
		input *struct {
			ID   string `path:"id"`
			Body mergeBranchBody
		},
	) (*h.ItemResponse[model.MergeResult], error) {
		result, err := branchSvc.MergeBranch(ctx, input.ID, input.Body.SourceBranch, model.MergeBranchOptions{
			TargetBranch:  input.Body.TargetBranch,
			Strategy:      input.Body.Strategy,
			Commit:        input.Body.Commit,
			CommitMessage: input.Body.CommitMessage,
		})
		if err != nil {
			return nil, mapBranchError(err)
		}
		resp := h.NewItemResponse(*result)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "branch-merge"
		op.Summary = "合并分支"
		op.Tags = []string{branchTag}
	})
}

func mapBranchError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, model.ErrDBNotInitialized):
		return huma.Error503ServiceUnavailable("database is not initialized")
	case errors.Is(err, model.ErrProjectNotFound),
		errors.Is(err, model.ErrWorktreeNotFound):
		return huma.Error404NotFound(err.Error())
	case errors.Is(err, model.ErrBranchHasWorktree),
		errors.Is(err, model.ErrWorktreeDirty):
		return huma.Error409Conflict(err.Error())
	case errors.Is(err, model.ErrProtectedBranch):
		return huma.Error409Conflict(err.Error())
	case errors.Is(err, model.ErrInvalidBranchName):
		return huma.Error400BadRequest(err.Error())
	default:
		return huma.Error400BadRequest(err.Error())
	}
}
