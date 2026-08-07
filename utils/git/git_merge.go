package git

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	goGit "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
)

type MergeStrategy string

const (
	MergeStrategyMerge  MergeStrategy = "merge"
	MergeStrategyRebase MergeStrategy = "rebase"
	MergeStrategySquash MergeStrategy = "squash"
)

func (r *GitRepo) MergeBranch(worktreePath, sourceBranch string, strategy MergeStrategy) error {
	if r == nil {
		return errors.New("git repository is not initialized")
	}
	if strategy == "" {
		strategy = MergeStrategyMerge
	}
	if strategy != MergeStrategyMerge && strategy != MergeStrategyRebase && strategy != MergeStrategySquash {
		return &OperationError{Code: ErrorCodeOperationUnsupported, Operation: OperationFastForwardMerge, Detail: fmt.Sprintf("unsupported merge strategy: %s", strategy)}
	}
	targetPath := strings.TrimSpace(worktreePath)
	if targetPath == "" {
		targetPath = r.Path
	}
	source := strings.TrimSpace(sourceBranch)
	if err := r.ValidateBranchName(source); err != nil {
		return err
	}
	operation := OperationMerge
	switch strategy {
	case MergeStrategyRebase:
		operation = OperationRebase
	case MergeStrategySquash:
		operation = OperationSquash
	}
	engine, err := r.requireEngine(targetPath, operation)
	if err != nil {
		return err
	}
	if engine == EngineSystem {
		args := []string{"merge", source}
		switch strategy {
		case MergeStrategyRebase:
			args = []string{"rebase", source}
		case MergeStrategySquash:
			args = []string{"merge", "--squash", source}
		}
		_, err = runSystemGit(context.Background(), targetPath, operation, args...)
		if err == nil {
			clearCapabilityCache()
		}
		return err
	}
	if r.repository == nil {
		return errors.New("built-in Git repository is not initialized")
	}
	if strategy != MergeStrategyMerge {
		return &OperationError{Code: ErrorCodeOperationUnsupported, Operation: operation, Detail: fmt.Sprintf("the built-in Git engine does not support %s", strategy)}
	}

	r.lock.Lock()
	defer r.lock.Unlock()
	repository, err := r.openWorktreeRepository(targetPath)
	if err != nil {
		return err
	}
	defer repository.Close()
	head, err := repository.Head()
	if err != nil {
		return mapGoGitError(OperationFastForwardMerge, err)
	}
	if !head.Name().IsBranch() {
		return &OperationError{Code: ErrorCodeOperationUnsupported, Operation: OperationFastForwardMerge, Detail: "target worktree has a detached HEAD"}
	}
	sourceRef, err := repository.Reference(plumbing.NewBranchReferenceName(source), true)
	if err != nil {
		return &OperationError{Code: ErrorCodeInvalidReference, Operation: OperationFastForwardMerge, Detail: fmt.Sprintf("source branch not found: %s", source), Err: err}
	}
	if head.Hash() == sourceRef.Hash() {
		return nil
	}

	worktree, err := repository.Worktree()
	if err != nil {
		return err
	}
	status, err := worktree.StatusWithOptions(goGit.StatusOptions{Strategy: goGit.Preload})
	if err != nil {
		return mapGoGitError(OperationFastForwardMerge, err)
	}
	if !status.IsClean() {
		return &OperationError{Code: ErrorCodeWorktreeDirty, Operation: OperationFastForwardMerge, Detail: "worktree must be completely clean before a fast-forward merge"}
	}
	conflicts, err := conflictPaths(repository)
	if err != nil {
		return err
	}
	if len(conflicts) > 0 {
		return &OperationError{Code: ErrorCodeWorktreeDirty, Operation: OperationFastForwardMerge, Detail: "worktree index contains unresolved conflicts"}
	}

	targetCommit, err := repository.CommitObject(head.Hash())
	if err != nil {
		return err
	}
	sourceCommit, err := repository.CommitObject(sourceRef.Hash())
	if err != nil {
		return err
	}
	fastForward, err := targetCommit.IsAncestor(sourceCommit)
	if err != nil {
		return err
	}
	if !fastForward {
		return &OperationError{Code: ErrorCodeNonFastForward, Operation: OperationFastForwardMerge, Detail: "source branch cannot be fast-forwarded into the target"}
	}

	oldHash := head.Hash()
	if err := worktree.Reset(&goGit.ResetOptions{Mode: goGit.HardReset, Commit: sourceRef.Hash()}); err != nil {
		rollbackErr := worktree.Reset(&goGit.ResetOptions{Mode: goGit.HardReset, Commit: oldHash})
		if rollbackErr != nil {
			return fmt.Errorf("fast-forward merge failed: %v; rollback failed: %w", err, rollbackErr)
		}
		return mapGoGitError(OperationFastForwardMerge, err)
	}
	clearCapabilityCache()
	return nil
}

func (r *GitRepo) GetConflictFiles(worktreePath string) []string {
	if r == nil {
		return []string{}
	}
	target := strings.TrimSpace(worktreePath)
	if target == "" {
		target = r.Path
	}
	engine, err := r.requireEngine(target, OperationStatus)
	if err != nil {
		return []string{}
	}
	if engine == EngineSystem {
		output, err := runSystemGitOutput(context.Background(), target, OperationStatus, "diff", "--name-only", "--diff-filter=U")
		if err != nil {
			return []string{}
		}
		items := make([]string, 0)
		for _, line := range strings.Split(string(output), "\n") {
			if path := strings.TrimSpace(line); path != "" {
				items = append(items, path)
			}
		}
		return items
	}
	if r.repository == nil {
		return []string{}
	}
	r.lock.RLock()
	defer r.lock.RUnlock()
	repository, err := r.openWorktreeRepository(target)
	if err != nil {
		return []string{}
	}
	defer repository.Close()
	paths, err := conflictPaths(repository)
	if err != nil {
		return []string{}
	}
	result := make([]string, 0, len(paths))
	for path := range paths {
		result = append(result, path)
	}
	sort.Strings(result)
	return result
}

func IsConflictError(err error) bool {
	return ErrorCode(err) == ErrorCodeWorktreeDirty || ErrorCode(err) == ErrorCodeMergeConflict
}
