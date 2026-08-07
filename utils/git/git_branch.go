package git

import (
	"context"
	"errors"
	"fmt"
	"strings"

	goGit "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
)

type BranchInfo struct {
	Name              string `json:"name"`
	IsCurrent         bool   `json:"isCurrent"`
	IsRemote          bool   `json:"isRemote"`
	HeadCommit        string `json:"headCommit"`
	HeadCommitMessage string `json:"headCommitMessage"`
	HasWorktree       bool   `json:"hasWorktree"`
}

func (r *GitRepo) ListBranches() (local []BranchInfo, remote []BranchInfo, err error) {
	if r == nil {
		return nil, nil, errors.New("git repository is not initialized")
	}
	engine, err := r.requireEngine(r.Path, OperationBranchesRead)
	if err != nil {
		return nil, nil, err
	}
	if engine == EngineSystem {
		return r.listBranchesSystem(context.Background())
	}
	if r.repository == nil {
		return nil, nil, errors.New("built-in Git repository is not initialized")
	}
	r.lock.RLock()
	defer r.lock.RUnlock()

	currentBranch, _ := r.GetCurrentBranch()
	localIter, err := r.repository.Branches()
	if err != nil {
		return nil, nil, err
	}
	defer localIter.Close()

	local = make([]BranchInfo, 0)
	err = localIter.ForEach(func(ref *plumbing.Reference) error {
		name := ref.Name().Short()
		local = append(local, BranchInfo{
			Name:              name,
			IsCurrent:         name == currentBranch,
			HeadCommit:        shortHash(ref.Hash()),
			HeadCommitMessage: resolveCommitMessage(r.repository, ref.Hash()),
		})
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	refIter, err := r.repository.References()
	if err != nil {
		return local, nil, err
	}
	defer refIter.Close()

	remote = make([]BranchInfo, 0)
	err = refIter.ForEach(func(ref *plumbing.Reference) error {
		if !ref.Name().IsRemote() || ref.Type() != plumbing.HashReference {
			return nil
		}
		remote = append(remote, BranchInfo{
			Name:              ref.Name().Short(),
			IsRemote:          true,
			HeadCommit:        shortHash(ref.Hash()),
			HeadCommitMessage: resolveCommitMessage(r.repository, ref.Hash()),
		})
		return nil
	})
	return local, remote, err
}

func (r *GitRepo) CreateBranch(name, base string) error {
	if r == nil {
		return errors.New("git repository is not initialized")
	}
	branch := strings.TrimSpace(name)
	if err := r.ValidateBranchName(branch); err != nil {
		return err
	}
	engine, err := r.requireEngine(r.Path, OperationBranchesWrite)
	if err != nil {
		return err
	}
	if engine == EngineSystem {
		args := []string{"branch", branch}
		if base = strings.TrimSpace(base); base != "" {
			args = append(args, base)
		}
		_, err = runSystemGit(context.Background(), r.Path, OperationBranchesWrite, args...)
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

	branchRef := plumbing.NewBranchReferenceName(branch)
	if _, err := r.repository.Reference(branchRef, true); err == nil {
		return fmt.Errorf("branch already exists: %s", branch)
	} else if !errors.Is(err, plumbing.ErrReferenceNotFound) {
		return err
	}

	var hash plumbing.Hash
	base = strings.TrimSpace(base)
	if base == "" {
		head, err := r.repository.Head()
		if err != nil {
			return mapGoGitError(OperationBranchesWrite, err)
		}
		hash = head.Hash()
	} else {
		resolved, err := r.repository.ResolveRevision(plumbing.Revision(base))
		if err != nil {
			return &OperationError{Code: ErrorCodeInvalidReference, Operation: OperationBranchesWrite, Detail: fmt.Sprintf("base reference not found: %s", base), Err: err}
		}
		hash = *resolved
	}
	if _, err := r.repository.CommitObject(hash); err != nil {
		return &OperationError{Code: ErrorCodeInvalidReference, Operation: OperationBranchesWrite, Detail: "base does not resolve to a commit", Err: err}
	}
	if err := r.repository.Storer.SetReference(plumbing.NewHashReference(branchRef, hash)); err != nil {
		return mapGoGitError(OperationBranchesWrite, err)
	}
	clearCapabilityCache()
	return nil
}

func (r *GitRepo) DeleteBranch(name string, force bool) error {
	if r == nil {
		return errors.New("git repository is not initialized")
	}
	branch := strings.TrimSpace(name)
	if err := r.ValidateBranchName(branch); err != nil {
		return err
	}
	engine, err := r.requireEngine(r.Path, OperationBranchesWrite)
	if err != nil {
		return err
	}
	if engine == EngineSystem {
		flag := "-d"
		if force {
			flag = "-D"
		}
		_, err = runSystemGit(context.Background(), r.Path, OperationBranchesWrite, "branch", flag, branch)
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
	checkedOut, err := r.branchCheckedOutUnlocked(branch)
	if err != nil {
		return err
	}
	if checkedOut {
		return fmt.Errorf("branch is checked out in a worktree: %s", branch)
	}

	branchRef := plumbing.NewBranchReferenceName(branch)
	ref, err := r.repository.Reference(branchRef, true)
	if err != nil {
		return mapGoGitError(OperationBranchesWrite, err)
	}
	if !force {
		head, headErr := r.repository.Head()
		if headErr != nil {
			return mapGoGitError(OperationBranchesWrite, headErr)
		}
		branchCommit, branchErr := r.repository.CommitObject(ref.Hash())
		if branchErr != nil {
			return branchErr
		}
		headCommit, headCommitErr := r.repository.CommitObject(head.Hash())
		if headCommitErr != nil {
			return headCommitErr
		}
		merged, ancestorErr := branchCommit.IsAncestor(headCommit)
		if ancestorErr != nil {
			return ancestorErr
		}
		if !merged && branchCommit.Hash != headCommit.Hash {
			return fmt.Errorf("branch is not fully merged: %s", branch)
		}
	}
	if err := r.repository.Storer.RemoveReference(branchRef); err != nil {
		return mapGoGitError(OperationBranchesWrite, err)
	}
	clearCapabilityCache()
	return nil
}

func (r *GitRepo) CheckoutBranch(name string) error {
	if r == nil {
		return errors.New("git repository is not initialized")
	}
	branch := strings.TrimSpace(name)
	if err := r.ValidateBranchName(branch); err != nil {
		return err
	}
	engine, err := r.requireEngine(r.Path, OperationBranchesWrite)
	if err != nil {
		return err
	}
	if engine == EngineSystem {
		_, err = runSystemGit(context.Background(), r.Path, OperationBranchesWrite, "checkout", branch)
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

	repository, err := r.openWorktreeRepository(r.Path)
	if err != nil {
		return err
	}
	defer repository.Close()
	worktree, err := repository.Worktree()
	if err != nil {
		return err
	}
	if err := worktree.Checkout(&goGit.CheckoutOptions{Branch: plumbing.NewBranchReferenceName(branch)}); err != nil {
		return mapGoGitError(OperationBranchesWrite, err)
	}
	clearCapabilityCache()
	return nil
}

func shortHash(hash plumbing.Hash) string {
	value := hash.String()
	if len(value) > 7 {
		return value[:7]
	}
	return value
}

func resolveCommitMessage(repo *goGit.Repository, hash plumbing.Hash) string {
	if repo == nil || hash.IsZero() {
		return ""
	}
	commit, err := repo.CommitObject(hash)
	if err != nil {
		return ""
	}
	return firstLine(commit.Message)
}

func (r *GitRepo) ValidateBranchName(name string) error {
	branch := strings.TrimSpace(name)
	if branch == "" {
		return errors.New("branch name is required")
	}
	preference := preferenceForOperation(OperationBranchesWrite)
	if r != nil && preference != EnginePreferenceBuiltin {
		info := ProbeSystemGit(context.Background(), false)
		if !info.Available && preference == EnginePreferenceSystem {
			return &OperationError{Code: ErrorCodeSystemGitUnavailable, Operation: OperationBranchesWrite, Detail: info.Error}
		}
		if info.Available {
			_, err := runSystemGit(context.Background(), r.Path, OperationBranchesWrite, "check-ref-format", "--branch", branch)
			if err != nil {
				return &OperationError{Code: ErrorCodeInvalidReference, Operation: OperationBranchesWrite, Detail: "invalid branch name", Err: err}
			}
			return nil
		}
	}
	ref := plumbing.NewBranchReferenceName(branch)
	if err := ref.Validate(); err != nil {
		return &OperationError{Code: ErrorCodeInvalidReference, Operation: OperationBranchesWrite, Detail: "invalid branch name", Err: err}
	}
	return nil
}

func (r *GitRepo) listBranchesSystem(ctx context.Context) (local []BranchInfo, remote []BranchInfo, err error) {
	current, _ := runSystemGitOutput(ctx, r.Path, OperationBranchesRead, "symbolic-ref", "--quiet", "--short", "HEAD")
	currentBranch := strings.TrimSpace(string(current))
	output, err := runSystemGitOutput(
		ctx,
		r.Path,
		OperationBranchesRead,
		"for-each-ref",
		"--format=%(refname)%09%(objectname:short)%09%(subject)",
		"refs/heads",
		"refs/remotes",
	)
	if err != nil {
		return nil, nil, err
	}
	for _, line := range strings.Split(string(output), "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), "\t", 3)
		if len(parts) < 2 {
			continue
		}
		refName := parts[0]
		message := ""
		if len(parts) == 3 {
			message = parts[2]
		}
		item := BranchInfo{HeadCommit: parts[1], HeadCommitMessage: message}
		switch {
		case strings.HasPrefix(refName, "refs/heads/"):
			item.Name = strings.TrimPrefix(refName, "refs/heads/")
			item.IsCurrent = item.Name == currentBranch
			local = append(local, item)
		case strings.HasPrefix(refName, "refs/remotes/"):
			item.Name = strings.TrimPrefix(refName, "refs/remotes/")
			item.IsRemote = true
			remote = append(remote, item)
		}
	}
	return local, remote, nil
}
