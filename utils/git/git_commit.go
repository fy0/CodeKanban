package git

import (
	"errors"
	"fmt"
	"strings"
)

func (r *GitRepo) runInWorktree(path string, args ...string) error {
	if r == nil {
		return errors.New("git repository is not initialized")
	}
	target := strings.TrimSpace(path)
	if target == "" {
		target = r.Path
	}
	cmd := newGitCommand(target, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git %s failed: %s", strings.Join(args, " "), strings.TrimSpace(string(output)))
	}
	return nil
}

// AddAll stages all changes within the worktree path.
func (r *GitRepo) AddAll(worktreePath string) error {
	return r.runInWorktree(worktreePath, "add", "--all")
}

// Commit creates a commit with the provided message at the given worktree path.
func (r *GitRepo) Commit(worktreePath, message string) error {
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		return errors.New("commit message is required")
	}
	return r.runInWorktree(worktreePath, "commit", "-m", trimmed)
}
