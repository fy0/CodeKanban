package model

import (
	"errors"
	"path/filepath"
	"runtime"
	"strings"
)

var (
	// ErrWorktreeNotFound indicates the requested worktree does not exist.
	ErrWorktreeNotFound = errors.New("worktree not found")
	// ErrWorktreeIsMain indicates the worktree is the main repository path and cannot be removed.
	ErrWorktreeIsMain = errors.New("cannot delete main worktree")
	// ErrWorktreeHasTasks indicates there are tasks referencing the worktree requiring a force delete.
	ErrWorktreeHasTasks = errors.New("worktree has active tasks")
	// ErrWorktreeClean indicates there are no changes to commit.
	ErrWorktreeClean = errors.New("worktree has no changes to commit")
)

// NormalizePathCase cleans the path and lowercases it on Windows for reliable comparisons.
func NormalizePathCase(path string) string {
	clean := filepath.Clean(path)
	if runtime.GOOS == "windows" {
		return strings.ToLower(clean)
	}
	return clean
}

func derefInt64(ptr *int64) int64 {
	if ptr == nil {
		return 0
	}
	return *ptr
}
