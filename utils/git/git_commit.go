package git

import (
	"context"
	"errors"
	"strings"

	goGit "github.com/go-git/go-git/v6"
)

func (r *GitRepo) AddAll(worktreePath string) error {
	if r == nil {
		return errors.New("git repository is not initialized")
	}
	target := strings.TrimSpace(worktreePath)
	if target == "" {
		target = r.Path
	}
	engine, err := r.requireEngine(target, OperationCommit)
	if err != nil {
		return err
	}
	if engine == EngineSystem {
		_, err = runSystemGit(context.Background(), target, OperationCommit, "add", "--all")
		if err == nil {
			clearCapabilityCache()
		}
		return err
	}
	if r.repository == nil {
		return errors.New("built-in Git repository is not initialized")
	}
	r.lock.Lock()
	defer r.lock.Unlock()
	return r.addAllUnlocked(target)
}

func (r *GitRepo) addAllUnlocked(worktreePath string) error {
	repository, err := r.openWorktreeRepository(worktreePath)
	if err != nil {
		return err
	}
	defer repository.Close()
	if err := r.ensureBuiltinWorktreeContentSafe(repository, worktreePath, OperationCommit); err != nil {
		return err
	}
	worktree, err := repository.Worktree()
	if err != nil {
		return err
	}
	if err := worktree.AddWithOptions(&goGit.AddOptions{All: true}); err != nil {
		return mapGoGitError(OperationCommit, err)
	}
	return nil
}

func (r *GitRepo) Commit(worktreePath, message string) error {
	if r == nil {
		return errors.New("git repository is not initialized")
	}
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		return errors.New("commit message is required")
	}
	target := strings.TrimSpace(worktreePath)
	if target == "" {
		target = r.Path
	}
	engine, err := r.requireEngine(target, OperationCommit)
	if err != nil {
		return err
	}
	if engine == EngineSystem {
		_, err = runSystemGit(context.Background(), target, OperationCommit, "commit", "-m", trimmed)
		if err == nil {
			clearCapabilityCache()
		}
		return err
	}
	if r.repository == nil {
		return errors.New("built-in Git repository is not initialized")
	}
	r.lock.Lock()
	defer r.lock.Unlock()
	repository, err := r.openWorktreeRepository(target)
	if err != nil {
		return err
	}
	defer repository.Close()
	if err := r.ensureBuiltinIndexContentSafe(repository, OperationCommit); err != nil {
		return err
	}
	worktree, err := repository.Worktree()
	if err != nil {
		return err
	}
	if _, err := worktree.Commit(trimmed, &goGit.CommitOptions{}); err != nil {
		return mapGoGitError(OperationCommit, err)
	}
	clearCapabilityCache()
	return nil
}

func (r *GitRepo) CommitAll(worktreePath, message string) error {
	if r == nil {
		return errors.New("git repository is not initialized")
	}
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		return errors.New("commit message is required")
	}
	target := strings.TrimSpace(worktreePath)
	if target == "" {
		target = r.Path
	}
	engine, err := r.requireEngine(target, OperationCommit)
	if err != nil {
		return err
	}
	if engine == EngineSystem {
		if _, err = runSystemGit(context.Background(), target, OperationCommit, "add", "--all"); err != nil {
			return err
		}
		_, err = runSystemGit(context.Background(), target, OperationCommit, "commit", "-m", trimmed)
		if err == nil {
			clearCapabilityCache()
		}
		return err
	}
	if r.repository == nil {
		return errors.New("built-in Git repository is not initialized")
	}
	r.lock.Lock()
	defer r.lock.Unlock()
	repository, err := r.openWorktreeRepository(target)
	if err != nil {
		return err
	}
	defer repository.Close()
	if err := r.ensureBuiltinWorktreeContentSafe(repository, target, OperationCommit); err != nil {
		return err
	}
	worktree, err := repository.Worktree()
	if err != nil {
		return err
	}
	if err := worktree.AddWithOptions(&goGit.AddOptions{All: true}); err != nil {
		return mapGoGitError(OperationCommit, err)
	}
	if _, err := worktree.Commit(trimmed, &goGit.CommitOptions{}); err != nil {
		return mapGoGitError(OperationCommit, err)
	}
	clearCapabilityCache()
	return nil
}
