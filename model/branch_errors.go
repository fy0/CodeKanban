package model

import "errors"

var (
	// ErrBranchHasWorktree indicates a branch is still referenced by one or more worktrees.
	ErrBranchHasWorktree = errors.New("branch has associated worktree")
	// ErrWorktreeDirty indicates merge cannot proceed due to local modifications.
	ErrWorktreeDirty = errors.New("worktree has uncommitted changes")
	// ErrProtectedBranch indicates delete attempts against default/current branches.
	ErrProtectedBranch = errors.New("branch is protected and cannot be deleted")
	// ErrInvalidBranchName indicates user input fails git ref validation.
	ErrInvalidBranchName = errors.New("invalid branch name")
)
