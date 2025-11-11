package git

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/go-git/go-git/v5/plumbing"
)

// BranchInfo describes local or remote branch metadata.
type BranchInfo struct {
	Name        string `json:"name"`
	IsCurrent   bool   `json:"isCurrent"`
	IsRemote    bool   `json:"isRemote"`
	HeadCommit  string `json:"headCommit"`
	HasWorktree bool   `json:"hasWorktree"`
}

// ListBranches returns local and remote branches present in the repository.
func (r *GitRepo) ListBranches() (local []BranchInfo, remote []BranchInfo, err error) {
	if r == nil || r.Repository == nil {
		return nil, nil, errors.New("git repository is not initialized")
	}

	currentBranch, _ := r.GetCurrentBranch()

	localIter, err := r.Repository.Branches()
	if err != nil {
		return nil, nil, err
	}
	defer localIter.Close()

	local = make([]BranchInfo, 0)
	err = localIter.ForEach(func(ref *plumbing.Reference) error {
		name := ref.Name().Short()
		local = append(local, BranchInfo{
			Name:       name,
			IsCurrent:  name == currentBranch,
			IsRemote:   false,
			HeadCommit: shortHash(ref.Hash()),
		})
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	refIter, err := r.Repository.References()
	if err != nil {
		return local, nil, err
	}
	defer refIter.Close()

	remote = make([]BranchInfo, 0)
	err = refIter.ForEach(func(ref *plumbing.Reference) error {
		if !ref.Name().IsRemote() {
			return nil
		}
		remote = append(remote, BranchInfo{
			Name:       ref.Name().Short(),
			IsRemote:   true,
			HeadCommit: shortHash(ref.Hash()),
		})
		return nil
	})

	return local, remote, err
}

// CreateBranch creates a new branch from the provided base reference.
func (r *GitRepo) CreateBranch(name, base string) error {
	if r == nil {
		return errors.New("git repository is not initialized")
	}
	branch := strings.TrimSpace(name)
	if branch == "" {
		return errors.New("branch name is required")
	}

	args := []string{"branch", branch}
	if base = strings.TrimSpace(base); base != "" {
		args = append(args, base)
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = r.Path
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("create branch failed: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

// DeleteBranch removes a local branch. Force controls the -D flag.
func (r *GitRepo) DeleteBranch(name string, force bool) error {
	if r == nil {
		return errors.New("git repository is not initialized")
	}
	branch := strings.TrimSpace(name)
	if branch == "" {
		return errors.New("branch name is required")
	}

	args := []string{"branch"}
	if force {
		args = append(args, "-D", branch)
	} else {
		args = append(args, "-d", branch)
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = r.Path
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("delete branch failed: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

// CheckoutBranch switches HEAD to the provided branch.
func (r *GitRepo) CheckoutBranch(name string) error {
	if r == nil {
		return errors.New("git repository is not initialized")
	}
	branch := strings.TrimSpace(name)
	if branch == "" {
		return errors.New("branch name is required")
	}

	cmd := exec.Command("git", "checkout", branch)
	cmd.Dir = r.Path
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("checkout failed: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

func shortHash(hash plumbing.Hash) string {
	value := hash.String()
	if len(value) > 7 {
		return value[:7]
	}
	return value
}

// ValidateBranchName verifies the provided branch name matches git's ref rules.
func (r *GitRepo) ValidateBranchName(name string) error {
	if r == nil {
		return errors.New("git repository is not initialized")
	}
	branch := strings.TrimSpace(name)
	if branch == "" {
		return errors.New("branch name is required")
	}

	ref := fmt.Sprintf("refs/heads/%s", branch)
	cmd := exec.Command("git", "check-ref-format", "--allow-onelevel", ref)
	cmd.Dir = r.Path
	if output, err := cmd.CombinedOutput(); err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("invalid branch name: %s", message)
	}
	return nil
}
