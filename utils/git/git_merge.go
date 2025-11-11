package git

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// MergeStrategy defines how source commits should be integrated.
type MergeStrategy string

const (
	// MergeStrategyMerge performs a standard merge.
	MergeStrategyMerge MergeStrategy = "merge"
	// MergeStrategyRebase rebases the target branch onto the source branch.
	MergeStrategyRebase MergeStrategy = "rebase"
	// MergeStrategySquash merges changes as a single commit.
	MergeStrategySquash MergeStrategy = "squash"
)

// MergeBranch merges sourceBranch into the worktree located at worktreePath.
func (r *GitRepo) MergeBranch(worktreePath, sourceBranch string, strategy MergeStrategy) error {
	if r == nil {
		return errors.New("git repository is not initialized")
	}
	path := strings.TrimSpace(worktreePath)
	if path == "" {
		path = r.Path
	}
	source := strings.TrimSpace(sourceBranch)
	if source == "" {
		return errors.New("source branch is required")
	}

	cmd := buildMergeCommand(strategy, source)
	cmd.Dir = path

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("merge failed: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

func buildMergeCommand(strategy MergeStrategy, sourceBranch string) *exec.Cmd {
	switch strategy {
	case MergeStrategyRebase:
		return exec.Command("git", "rebase", sourceBranch)
	case MergeStrategySquash:
		return exec.Command("git", "merge", "--squash", sourceBranch)
	default:
		return exec.Command("git", "merge", sourceBranch)
	}
}

// GetConflictFiles returns files currently in a conflicted state.
func (r *GitRepo) GetConflictFiles(worktreePath string) []string {
	path := strings.TrimSpace(worktreePath)
	if path == "" && r != nil {
		path = r.Path
	}

	cmd := exec.Command("git", "diff", "--name-only", "--diff-filter=U")
	cmd.Dir = path
	output, err := cmd.Output()
	if err != nil {
		return []string{}
	}

	lines := strings.Split(string(output), "\n")
	conflicts := make([]string, 0, len(lines))
	for _, line := range lines {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			conflicts = append(conflicts, trimmed)
		}
	}
	return conflicts
}

// IsConflictError returns true when the merge output indicates a conflict.
func IsConflictError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "CONFLICT") ||
		strings.Contains(msg, "Merge conflict") ||
		strings.Contains(strings.ToLower(msg), "conflict")
}
