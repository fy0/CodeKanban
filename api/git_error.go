package api

import (
	"net/http"

	"code-kanban/utils/git"
)

type gitAPIError struct {
	Status int    `json:"status"`
	Code   string `json:"code"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
}

func (e *gitAPIError) Error() string {
	if e == nil {
		return ""
	}
	return e.Detail
}

func (e *gitAPIError) GetStatus() int {
	if e == nil {
		return http.StatusInternalServerError
	}
	return e.Status
}

func mapGitOperationError(err error) error {
	code := git.ErrorCode(err)
	if code == "" {
		return nil
	}
	status := http.StatusUnprocessableEntity
	switch code {
	case git.ErrorCodeInvalidReference:
		status = http.StatusBadRequest
	case git.ErrorCodeWorktreeNotFound:
		status = http.StatusNotFound
	case git.ErrorCodeRepositoryLocked,
		git.ErrorCodeWorktreeDirty,
		git.ErrorCodeNonFastForward,
		git.ErrorCodeMergeConflict,
		git.ErrorCodeWorktreeAlreadyRegistered:
		status = http.StatusConflict
	}
	return &gitAPIError{
		Status: status,
		Code:   code,
		Title:  http.StatusText(status),
		Detail: err.Error(),
	}
}
