package git

import (
	"context"
	"errors"
	"strings"
	"time"

	goGit "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
)

type WorktreeStatus struct {
	Branch     string
	Ahead      int
	Behind     int
	Modified   int
	Staged     int
	Untracked  int
	Conflicted int
	LastCommit *CommitInfo
}

type CommitInfo struct {
	SHA     string
	Message string
	Author  string
	Date    time.Time
}

func HasTrackedWorktreeChanges(path string) (bool, error) {
	return HasTrackedWorktreeChangesContext(context.Background(), path)
}

func HasTrackedWorktreeChangesContext(ctx context.Context, path string) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	repo, err := DetectRepository(path)
	if err != nil {
		return false, err
	}
	defer repo.Close()
	engine, err := repo.requireEngine(path, OperationStatus)
	if err != nil {
		return false, err
	}
	if engine == EngineSystem {
		return hasTrackedWorktreeChangesSystem(ctx, path)
	}
	if repo.repository == nil {
		return false, errors.New("built-in Git repository is not initialized")
	}

	repo.lock.RLock()
	defer repo.lock.RUnlock()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	repository, err := repo.openWorktreeRepository(path)
	if err != nil {
		return false, err
	}
	defer repository.Close()
	worktree, err := repository.Worktree()
	if err != nil {
		return false, err
	}
	status, err := worktree.StatusWithOptions(goGit.StatusOptions{Strategy: goGit.Preload})
	if err != nil {
		return false, mapGoGitError(OperationStatus, err)
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	for _, fileStatus := range status {
		if fileStatus.Staging == goGit.Untracked || fileStatus.Worktree == goGit.Untracked {
			continue
		}
		if fileStatus.Staging != goGit.Unmodified || fileStatus.Worktree != goGit.Unmodified {
			return true, nil
		}
	}
	conflicts, err := conflictPaths(repository)
	return len(conflicts) > 0, err
}

func GetWorktreeStatus(path string) (*WorktreeStatus, error) {
	return GetWorktreeStatusContext(context.Background(), path)
}

func GetWorktreeStatusContext(ctx context.Context, path string) (*WorktreeStatus, error) {
	repo, err := DetectRepository(path)
	if err != nil {
		return nil, err
	}
	defer repo.Close()
	return repo.GetWorktreeStatusContext(ctx, path)
}

func (r *GitRepo) GetWorktreeStatus(path string) (*WorktreeStatus, error) {
	return r.GetWorktreeStatusContext(context.Background(), path)
}

func (r *GitRepo) GetWorktreeStatusContext(ctx context.Context, path string) (*WorktreeStatus, error) {
	if r == nil {
		return nil, errors.New("git repository is not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	target := strings.TrimSpace(path)
	if target == "" {
		target = r.Path
	}
	engine, err := r.requireEngine(target, OperationStatus)
	if err != nil {
		return nil, err
	}
	if engine == EngineSystem {
		return collectWorktreeStatusSystem(ctx, target)
	}
	if r.repository == nil {
		return nil, errors.New("built-in Git repository is not initialized")
	}
	r.lock.RLock()
	defer r.lock.RUnlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	repository, err := r.openWorktreeRepository(target)
	if err != nil {
		return nil, err
	}
	defer repository.Close()
	return collectWorktreeStatus(ctx, repository)
}

func collectWorktreeStatus(ctx context.Context, repository *goGit.Repository) (*WorktreeStatus, error) {
	if repository == nil {
		return nil, errors.New("git repository is not initialized")
	}
	result := &WorktreeStatus{}
	head, headErr := repository.Head()
	if headErr != nil && !errors.Is(headErr, plumbing.ErrReferenceNotFound) {
		return nil, headErr
	}
	if head != nil {
		if head.Name().IsBranch() {
			result.Branch = head.Name().Short()
		}
		if commit, err := repository.CommitObject(head.Hash()); err == nil {
			result.LastCommit = &CommitInfo{
				SHA:     shortCommit(commit.Hash.String()),
				Message: firstLine(commit.Message),
				Author:  commit.Author.Name,
				Date:    commit.Author.When,
			}
		}
	}

	worktree, err := repository.Worktree()
	if err != nil {
		return nil, err
	}
	snapshot, err := worktree.StatusWithOptions(goGit.StatusOptions{Strategy: goGit.Preload})
	if err != nil {
		return nil, mapGoGitError(OperationStatus, err)
	}
	conflicts, err := conflictPaths(repository)
	if err != nil {
		return nil, err
	}
	result.Conflicted = len(conflicts)

	for path, fileStatus := range snapshot {
		if _, conflicted := conflicts[path]; conflicted {
			continue
		}
		if fileStatus.Staging == goGit.Untracked || fileStatus.Worktree == goGit.Untracked {
			result.Untracked++
			continue
		}
		switch fileStatus.Worktree {
		case goGit.Modified, goGit.Added, goGit.Deleted, goGit.Renamed, goGit.Copied:
			result.Modified++
		}
		switch fileStatus.Staging {
		case goGit.Modified, goGit.Added, goGit.Deleted, goGit.Renamed, goGit.Copied:
			result.Staged++
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if head != nil && head.Name().IsBranch() {
		result.Ahead, result.Behind = aheadBehind(ctx, repository, head)
	}
	return result, nil
}

func conflictPaths(repository *goGit.Repository) (map[string]struct{}, error) {
	result := make(map[string]struct{})
	idx, err := repository.Storer.Index()
	if errors.Is(err, plumbing.ErrReferenceNotFound) {
		return result, nil
	}
	if err != nil {
		return nil, err
	}
	counts := make(map[string]int)
	for _, entry := range idx.Entries {
		if entry.Stage != 0 {
			counts[entry.Name]++
		}
	}
	for path, count := range counts {
		if count > 1 {
			result[filepathSlash(path)] = struct{}{}
		}
	}
	return result, nil
}

func aheadBehind(ctx context.Context, repository *goGit.Repository, head *plumbing.Reference) (ahead, behind int) {
	cfg, err := repository.Config()
	if err != nil {
		return 0, 0
	}
	branch, ok := cfg.Branches[head.Name().Short()]
	if !ok || branch.Merge == "" {
		return 0, 0
	}
	upstreamName := branch.Merge
	if branch.Remote != "" && branch.Remote != "." {
		upstreamName = plumbing.NewRemoteReferenceName(branch.Remote, branch.Merge.Short())
	}
	upstream, err := repository.Reference(upstreamName, true)
	if err != nil {
		return 0, 0
	}
	headSet, err := reachableCommits(ctx, repository, head.Hash())
	if err != nil {
		return 0, 0
	}
	upstreamSet, err := reachableCommits(ctx, repository, upstream.Hash())
	if err != nil {
		return 0, 0
	}
	for hash := range headSet {
		if _, ok := upstreamSet[hash]; !ok {
			ahead++
		}
	}
	for hash := range upstreamSet {
		if _, ok := headSet[hash]; !ok {
			behind++
		}
	}
	return ahead, behind
}

func reachableCommits(ctx context.Context, repository *goGit.Repository, start plumbing.Hash) (map[plumbing.Hash]struct{}, error) {
	seen := make(map[plumbing.Hash]struct{})
	stack := []plumbing.Hash{start}
	for len(stack) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		hash := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if _, ok := seen[hash]; ok {
			continue
		}
		commit, err := repository.CommitObject(hash)
		if err != nil {
			return nil, err
		}
		seen[hash] = struct{}{}
		stack = append(stack, commit.ParentHashes...)
	}
	return seen, nil
}

func shortCommit(hash string) string {
	if len(hash) > 7 {
		return hash[:7]
	}
	return hash
}

func firstLine(msg string) string {
	if idx := strings.Index(msg, "\n"); idx >= 0 {
		return strings.TrimSpace(msg[:idx])
	}
	return strings.TrimSpace(msg)
}

func filepathSlash(path string) string {
	return strings.ReplaceAll(path, "\\", "/")
}
