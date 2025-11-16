package git

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// WorktreeInfo stores metadata returned by `git worktree list --porcelain`.
type WorktreeInfo struct {
	Path       string
	Branch     string
	HeadCommit string
	IsMain     bool
	IsBare     bool
}

// ListWorktrees enumerates worktrees attached to the repository.
func (r *GitRepo) ListWorktrees() ([]WorktreeInfo, error) {
	if r == nil {
		return nil, errors.New("git repository is not initialized")
	}

	// Prune stale entries first so list output matches actual filesystem state.
	_ = r.PruneWorktrees()

	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = r.Path

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("list worktrees failed: %s", strings.TrimSpace(string(output)))
	}

	worktrees := parseWorktreeList(string(output))
	for i := range worktrees {
		if equalPath(worktrees[i].Path, r.Path) {
			worktrees[i].IsMain = true
		}
	}

	return worktrees, nil
}

// AddWorktree adds a new worktree at path. When createBranch is true, -b is used.
func (r *GitRepo) AddWorktree(path, branch string, createBranch bool) error {
	if r == nil {
		return errors.New("git repository is not initialized")
	}
	if strings.TrimSpace(path) == "" {
		return errors.New("worktree path is required")
	}
	args := []string{"worktree", "add"}
	targetPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	if createBranch {
		if strings.TrimSpace(branch) == "" {
			return errors.New("branch name is required when createBranch is true")
		}
		args = append(args, "-b", branch, targetPath)
	} else {
		if strings.TrimSpace(branch) == "" {
			return errors.New("branch name is required")
		}
		args = append(args, targetPath, branch)
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = r.Path
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("add worktree failed: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

// RemoveWorktree removes an existing worktree.
func (r *GitRepo) RemoveWorktree(path string, force bool) error {
	if r == nil {
		return errors.New("git repository is not initialized")
	}
	if strings.TrimSpace(path) == "" {
		return errors.New("worktree path is required")
	}

	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, path)

	cmd := exec.Command("git", args...)
	cmd.Dir = r.Path
	if output, err := cmd.CombinedOutput(); err != nil {
		errMsg := strings.TrimSpace(string(output))
		// If the error indicates the worktree doesn't exist, try pruning
		if strings.Contains(errMsg, "is not a working tree") || strings.Contains(errMsg, "not found") {
			// Try to prune stale worktree entries
			if pruneErr := r.PruneWorktrees(); pruneErr == nil {
				// After pruning, the worktree metadata should be cleaned up
				return nil
			}
		}
		return fmt.Errorf("remove worktree failed: %s", errMsg)
	}
	return nil
}

// PruneWorktrees runs `git worktree prune` to clean stale entries.
func (r *GitRepo) PruneWorktrees() error {
	if r == nil {
		return errors.New("git repository is not initialized")
	}
	cmd := exec.Command("git", "worktree", "prune")
	cmd.Dir = r.Path
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("prune worktrees failed: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

func parseWorktreeList(output string) []WorktreeInfo {
	lines := strings.Split(output, "\n")
	result := make([]WorktreeInfo, 0)
	var current WorktreeInfo

	resetCurrent := func() {
		if current.Path != "" {
			result = append(result, current)
		}
		current = WorktreeInfo{}
	}

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			resetCurrent()
			continue
		}

		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			continue
		}
		key, val := parts[0], strings.TrimSpace(parts[1])

		switch key {
		case "worktree":
			current.Path = val
		case "branch":
			current.Branch = strings.TrimPrefix(val, "refs/heads/")
		case "HEAD":
			if len(val) >= 7 {
				current.HeadCommit = val[:7]
			} else {
				current.HeadCommit = val
			}
		case "bare":
			current.IsBare = true
		case "detached":
			current.Branch = val
		}
	}
	resetCurrent()

	return result
}

func equalPath(a, b string) bool {
	cleanA := filepath.Clean(a)
	cleanB := filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(cleanA, cleanB)
	}
	return cleanA == cleanB
}
