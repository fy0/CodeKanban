package git

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/go-git/go-billy/v6/osfs"
	goGit "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	xworktree "github.com/go-git/go-git/v6/x/plumbing/worktree"
)

type WorktreeInfo struct {
	Path       string
	Branch     string
	HeadCommit string
	IsMain     bool
	IsBare     bool
	metadata   string
}

func (r *GitRepo) ListWorktrees() ([]WorktreeInfo, error) {
	if r == nil {
		return nil, errors.New("git repository is not initialized")
	}
	engine, err := r.requireEngine(r.Path, OperationWorktreesRead)
	if err != nil && !r.isBare {
		return nil, err
	}
	if engine == EngineSystem {
		return r.listWorktreesSystem(context.Background())
	}
	if r.repository == nil {
		return nil, errors.New("built-in Git repository is not initialized")
	}
	r.lock.RLock()
	defer r.lock.RUnlock()
	return r.listWorktreesUnlocked()
}

func (r *GitRepo) listWorktreesUnlocked() ([]WorktreeInfo, error) {
	if r == nil || r.repository == nil {
		return nil, errors.New("git repository is not initialized")
	}
	if r.isBare {
		head, _ := r.repository.Head()
		item := WorktreeInfo{Path: r.commonDir, IsMain: true, IsBare: true}
		if head != nil {
			item.HeadCommit = shortHash(head.Hash())
			if head.Name().IsBranch() {
				item.Branch = head.Name().Short()
			}
		}
		return []WorktreeInfo{item}, nil
	}

	result := make([]WorktreeInfo, 0, 4)
	if strings.TrimSpace(r.mainWorktree) != "" {
		item := WorktreeInfo{Path: filepath.Clean(r.mainWorktree), IsMain: true}
		mainRepo, err := openFilesystemRepository(r.mainWorktree, false)
		if err == nil {
			if head, headErr := mainRepo.Head(); headErr == nil {
				item.HeadCommit = shortHash(head.Hash())
				if head.Name().IsBranch() {
					item.Branch = head.Name().Short()
				}
			}
			_ = mainRepo.Close()
		}
		result = append(result, item)
	}

	if r.worktrees == nil {
		return result, nil
	}
	names, err := r.worktrees.List()
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	for _, name := range names {
		path, pathErr := r.worktreePathForMetadata(name)
		if pathErr != nil {
			continue
		}
		if _, statErr := os.Stat(path); statErr != nil {
			continue
		}
		item := WorktreeInfo{Path: filepath.Clean(path), metadata: name}
		repository, openErr := r.openWorktreeRepository(path)
		if openErr != nil {
			continue
		}
		if head, headErr := repository.Head(); headErr == nil {
			item.HeadCommit = shortHash(head.Hash())
			if head.Name().IsBranch() {
				item.Branch = head.Name().Short()
			}
		}
		_ = repository.Close()
		result = append(result, item)
	}
	return result, nil
}

func (r *GitRepo) AddWorktree(path, branch string, createBranch bool) error {
	if r == nil {
		return errors.New("git repository is not initialized")
	}
	targetPath, err := filepath.Abs(strings.TrimSpace(path))
	if err != nil || strings.TrimSpace(path) == "" {
		return errors.New("worktree path is required")
	}
	targetPath = filepath.Clean(targetPath)
	branch = strings.TrimSpace(branch)
	if err := r.ValidateBranchName(branch); err != nil {
		return err
	}
	engine, err := r.requireEngine(r.Path, OperationWorktreesWrite)
	if err != nil {
		return err
	}
	if engine == EngineSystem {
		args := []string{"worktree", "add"}
		if createBranch {
			args = append(args, "-b", branch, targetPath)
		} else {
			args = append(args, targetPath, branch)
		}
		_, err = runSystemGit(context.Background(), r.Path, OperationWorktreesWrite, args...)
		if err == nil {
			clearCapabilityCache()
		}
		return err
	}
	if r.repository == nil || r.worktrees == nil {
		return errors.New("built-in Git worktree manager is not initialized")
	}
	if pathWithinResolved(targetPath, r.commonDir) {
		return errors.New("worktree path cannot be inside repository metadata")
	}

	r.lock.Lock()
	defer r.lock.Unlock()
	checkedOut, err := r.branchCheckedOutUnlocked(branch)
	if err != nil {
		return err
	}
	if checkedOut {
		return fmt.Errorf("branch is already checked out in another worktree: %s", branch)
	}

	branchRef := plumbing.NewBranchReferenceName(branch)
	ref, refErr := r.repository.Reference(branchRef, true)
	createdBranch := false
	if createBranch {
		if refErr == nil {
			return fmt.Errorf("branch already exists: %s", branch)
		}
		if !errors.Is(refErr, plumbing.ErrReferenceNotFound) {
			return refErr
		}
		head, headErr := r.repository.Head()
		if headErr != nil {
			return mapGoGitError(OperationWorktreesWrite, headErr)
		}
		ref = plumbing.NewHashReference(branchRef, head.Hash())
		if err := r.repository.Storer.SetReference(ref); err != nil {
			return err
		}
		createdBranch = true
	} else if refErr != nil {
		return &OperationError{Code: ErrorCodeInvalidReference, Operation: OperationWorktreesWrite, Detail: fmt.Sprintf("branch not found: %s", branch), Err: refErr}
	}
	if err := r.ensureBuiltinTreeContentSafe(r.repository, ref.Hash(), OperationWorktreesWrite); err != nil {
		if createdBranch {
			_ = r.repository.Storer.RemoveReference(branchRef)
		}
		return err
	}

	createdDirectory, err := ensureEmptyWorktreeDirectory(targetPath)
	if err != nil {
		if createdBranch {
			_ = r.repository.Storer.RemoveReference(branchRef)
		}
		return err
	}
	metadataName := worktreeMetadataName(targetPath)
	if existingPath, existingErr := r.worktreePathForMetadata(metadataName); existingErr == nil && !equalPath(existingPath, targetPath) {
		if createdBranch {
			_ = r.repository.Storer.RemoveReference(branchRef)
		}
		if createdDirectory {
			_ = os.Remove(targetPath)
		}
		return &OperationError{Code: ErrorCodeWorktreeAlreadyRegistered, Operation: OperationWorktreesWrite, Detail: "worktree metadata name collision"}
	}

	filesystem := osfs.New(targetPath, osfs.WithBoundOS())
	if err := r.worktrees.Add(filesystem, metadataName, xworktree.WithCommit(ref.Hash()), xworktree.WithDetachedHead()); err != nil {
		// alpha.5 writes valid metadata before its Windows filesystem chroot fails.
		// Continue only when the complete standard linked-worktree layout exists.
		if !linkedWorktreeMetadataComplete(r.commonDir, targetPath, metadataName) {
			_ = r.worktrees.Remove(metadataName)
			cleanupWorktreePath(targetPath, createdDirectory)
			if createdBranch {
				_ = r.repository.Storer.RemoveReference(branchRef)
			}
			return &OperationError{Code: ErrorCodeLinkedWorktreeUnreliable, Operation: OperationWorktreesWrite, Detail: fmt.Sprintf("add linked worktree: %v", err), Err: err}
		}
	}

	linkedRepo, err := r.openWorktreeRepository(targetPath)
	if err == nil {
		var work *goGit.Worktree
		work, err = linkedRepo.Worktree()
		if err == nil {
			err = work.Checkout(&goGit.CheckoutOptions{Branch: branchRef})
		}
		_ = linkedRepo.Close()
	}
	if err != nil {
		_ = r.worktrees.Remove(metadataName)
		cleanupWorktreePath(targetPath, createdDirectory)
		if createdBranch {
			_ = r.repository.Storer.RemoveReference(branchRef)
		}
		return &OperationError{Code: ErrorCodeLinkedWorktreeUnreliable, Operation: OperationWorktreesWrite, Detail: fmt.Sprintf("open linked worktree after creation: %v", err), Err: err}
	}

	verification, verifyErr := r.openWorktreeRepository(targetPath)
	if verifyErr == nil {
		var status goGit.Status
		if work, workErr := verification.Worktree(); workErr == nil {
			status, verifyErr = work.StatusWithOptions(goGit.StatusOptions{Strategy: goGit.Preload})
			if verifyErr == nil && !status.IsClean() {
				verifyErr = errors.New("new linked worktree is unexpectedly dirty")
			}
		}
		_ = verification.Close()
	}
	if verifyErr != nil {
		_ = r.worktrees.Remove(metadataName)
		cleanupWorktreePath(targetPath, createdDirectory)
		if createdBranch {
			_ = r.repository.Storer.RemoveReference(branchRef)
		}
		return &OperationError{Code: ErrorCodeLinkedWorktreeUnreliable, Operation: OperationWorktreesWrite, Detail: "linked worktree verification failed", Err: verifyErr}
	}
	clearCapabilityCache()
	return nil
}

func (r *GitRepo) RemoveWorktree(path string, force bool) error {
	if r == nil {
		return errors.New("git repository is not initialized")
	}
	target, err := filepath.Abs(strings.TrimSpace(path))
	if err != nil || strings.TrimSpace(path) == "" {
		return errors.New("worktree path is required")
	}
	target = filepath.Clean(target)
	engine, err := r.requireEngine(r.Path, OperationWorktreesWrite)
	if err != nil {
		return err
	}
	if engine == EngineSystem {
		args := []string{"worktree", "remove"}
		if force {
			args = append(args, "--force")
		}
		args = append(args, target)
		_, err = runSystemGit(context.Background(), r.Path, OperationWorktreesWrite, args...)
		if err == nil {
			clearCapabilityCache()
		}
		return err
	}
	if r.repository == nil || r.worktrees == nil {
		return errors.New("built-in Git worktree manager is not initialized")
	}

	r.lock.Lock()
	defer r.lock.Unlock()
	if equalPath(target, r.mainWorktree) {
		return errors.New("main worktree cannot be removed")
	}
	name, err := r.metadataForWorktreePath(target)
	if err != nil {
		return &OperationError{Code: ErrorCodeWorktreeNotFound, Operation: OperationWorktreesWrite, Detail: "worktree is not registered", Err: err}
	}

	if _, statErr := os.Stat(target); statErr == nil && !force {
		repository, openErr := r.openWorktreeRepository(target)
		if openErr != nil {
			return openErr
		}
		work, workErr := repository.Worktree()
		if workErr == nil {
			var status goGit.Status
			status, workErr = work.StatusWithOptions(goGit.StatusOptions{Strategy: goGit.Preload})
			if workErr == nil && !status.IsClean() {
				workErr = &OperationError{Code: ErrorCodeWorktreeDirty, Operation: OperationWorktreesWrite, Detail: "worktree has uncommitted changes"}
			}
		}
		_ = repository.Close()
		if workErr != nil {
			return workErr
		}
	}

	quarantine := ""
	if _, statErr := os.Stat(target); statErr == nil {
		quarantine = filepath.Join(filepath.Dir(target), fmt.Sprintf(".%s.codekanban-remove-%d", filepath.Base(target), time.Now().UnixNano()))
		if err := os.Rename(target, quarantine); err != nil {
			return fmt.Errorf("prepare worktree removal: %w", err)
		}
	}
	if err := r.worktrees.Remove(name); err != nil {
		if quarantine != "" {
			_ = os.Rename(quarantine, target)
		}
		return mapGoGitError(OperationWorktreesWrite, err)
	}
	if quarantine != "" {
		if err := os.RemoveAll(quarantine); err != nil {
			return fmt.Errorf("remove worktree directory: %w", err)
		}
	}
	clearCapabilityCache()
	return nil
}

func (r *GitRepo) PruneWorktrees() error {
	if r == nil {
		return errors.New("git repository is not initialized")
	}
	engine, err := r.requireEngine(r.Path, OperationWorktreesWrite)
	if err != nil {
		return err
	}
	if engine == EngineSystem {
		_, err = runSystemGit(context.Background(), r.Path, OperationWorktreesWrite, "worktree", "prune")
		if err == nil {
			clearCapabilityCache()
		}
		return err
	}
	if r.repository == nil || r.worktrees == nil {
		return errors.New("built-in Git worktree manager is not initialized")
	}
	r.lock.Lock()
	defer r.lock.Unlock()

	names, err := r.worktrees.List()
	if err != nil {
		return err
	}
	for _, name := range names {
		path, pathErr := r.worktreePathForMetadata(name)
		if pathErr == nil {
			if _, statErr := os.Stat(filepath.Join(path, ".git")); statErr == nil {
				continue
			}
		}
		if removeErr := r.worktrees.Remove(name); removeErr != nil && !errors.Is(removeErr, xworktree.ErrWorktreeNotFound) {
			return removeErr
		}
	}
	clearCapabilityCache()
	return nil
}

func (r *GitRepo) listWorktreesSystem(ctx context.Context) ([]WorktreeInfo, error) {
	output, err := runSystemGitOutput(ctx, r.Path, OperationWorktreesRead, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	items := parseSystemWorktreeList(string(output))
	for index := range items {
		if equalPath(items[index].Path, r.mainWorktree) || (r.mainWorktree == "" && equalPath(items[index].Path, r.Path)) {
			items[index].IsMain = true
		}
	}
	return items, nil
}

func parseSystemWorktreeList(output string) []WorktreeInfo {
	lines := strings.Split(output, "\n")
	result := make([]WorktreeInfo, 0)
	current := WorktreeInfo{}
	flush := func() {
		if current.Path != "" {
			result = append(result, current)
		}
		current = WorktreeInfo{}
	}
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			flush()
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		key := parts[0]
		value := ""
		if len(parts) == 2 {
			value = strings.TrimSpace(parts[1])
		}
		switch key {
		case "worktree":
			current.Path = value
		case "branch":
			current.Branch = strings.TrimPrefix(value, "refs/heads/")
		case "HEAD":
			current.HeadCommit = value
			if len(current.HeadCommit) > 7 {
				current.HeadCommit = current.HeadCommit[:7]
			}
		case "bare":
			current.IsBare = true
		case "detached":
			current.Branch = ""
		}
	}
	flush()
	return result
}

func (r *GitRepo) branchCheckedOutUnlocked(branch string) (bool, error) {
	worktrees, err := r.listWorktreesUnlocked()
	if err != nil {
		return false, err
	}
	for _, worktree := range worktrees {
		if worktree.Branch == branch {
			return true, nil
		}
	}
	return false, nil
}

func (r *GitRepo) metadataForWorktreePath(path string) (string, error) {
	if r.worktrees == nil {
		return "", errors.New("worktree manager is unavailable")
	}
	names, err := r.worktrees.List()
	if err != nil {
		return "", err
	}
	for _, name := range names {
		candidate, candidateErr := r.worktreePathForMetadata(name)
		if candidateErr == nil && equalPath(candidate, path) {
			return name, nil
		}
	}
	return "", xworktree.ErrWorktreeNotFound
}

func (r *GitRepo) worktreePathForMetadata(name string) (string, error) {
	if strings.TrimSpace(name) == "" || filepath.Base(name) != name || name == "." || name == ".." {
		return "", errors.New("invalid worktree metadata name")
	}
	metadataDir := filepath.Join(r.commonDir, "worktrees", name)
	if !pathWithin(metadataDir, filepath.Join(r.commonDir, "worktrees")) {
		return "", errors.New("worktree metadata path escapes repository")
	}
	if info, statErr := os.Lstat(metadataDir); statErr != nil || !info.IsDir() {
		if statErr != nil {
			return "", statErr
		}
		return "", errors.New("worktree metadata entry is not a directory")
	}
	data, err := os.ReadFile(filepath.Join(metadataDir, "gitdir"))
	if err != nil {
		return "", err
	}
	gitFilePath := strings.TrimSpace(string(data))
	if gitFilePath == "" {
		return "", errors.New("worktree gitdir entry is empty")
	}
	if !filepath.IsAbs(gitFilePath) {
		gitFilePath = filepath.Join(metadataDir, gitFilePath)
	}
	gitFilePath = filepath.Clean(gitFilePath)
	if !strings.EqualFold(filepath.Base(gitFilePath), ".git") {
		return "", errors.New("worktree gitdir does not point to a .git file")
	}
	if info, statErr := os.Lstat(gitFilePath); statErr != nil || !info.Mode().IsRegular() {
		if statErr != nil {
			return "", statErr
		}
		return "", errors.New("worktree gitdir entry is not a regular file")
	}
	worktreePath := canonicalExistingPath(filepath.Dir(gitFilePath))
	if pathWithinResolved(worktreePath, r.commonDir) {
		return "", errors.New("worktree path points inside repository metadata")
	}
	linkedGitDir, linkedErr := readGitDirFile(gitFilePath)
	if linkedErr != nil || !equalPath(linkedGitDir, metadataDir) {
		if linkedErr != nil {
			return "", fmt.Errorf("read linked worktree gitdir: %w", linkedErr)
		}
		return "", errors.New("worktree gitdir metadata mismatch")
	}
	return worktreePath, nil
}

func worktreeMetadataName(path string) string {
	digest := sha256.Sum256([]byte(normalizePathKey(path)))
	return fmt.Sprintf("ck-%x", digest[:])
}

func ensureEmptyWorktreeDirectory(path string) (bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(path, 0o755); err != nil {
			return false, err
		}
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if !info.IsDir() {
		return false, errors.New("worktree path is not a directory")
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return false, err
	}
	if len(entries) != 0 {
		return false, errors.New("worktree directory must be empty")
	}
	return false, nil
}

func cleanupWorktreePath(path string, createdDirectory bool) {
	if createdDirectory {
		_ = os.RemoveAll(path)
		return
	}
	// Preserve a directory supplied by the caller. If the linked-worktree
	// creation wrote its marker before failing, remove only that marker.
	_ = os.Remove(filepath.Join(path, ".git"))
}

func linkedWorktreeMetadataComplete(commonDir, worktreePath, name string) bool {
	for _, path := range []string{
		filepath.Join(worktreePath, ".git"),
		filepath.Join(commonDir, "worktrees", name, "HEAD"),
		filepath.Join(commonDir, "worktrees", name, "commondir"),
		filepath.Join(commonDir, "worktrees", name, "gitdir"),
	} {
		if info, err := os.Stat(path); err != nil || info.IsDir() {
			return false
		}
	}
	return true
}

func pathWithin(path, parent string) bool {
	rel, err := filepath.Rel(filepath.Clean(parent), filepath.Clean(path))
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// pathWithinResolved also resolves symlink/reparse-point parents. A lexical
// filepath.Rel check alone is insufficient when a caller supplies a path
// through a directory link into .git.
func pathWithinResolved(path, parent string) bool {
	return pathWithin(canonicalPathWithExistingParent(path), canonicalPathWithExistingParent(parent))
}

func canonicalPathWithExistingParent(path string) string {
	cleaned, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		cleaned = filepath.Clean(path)
	}
	missing := make([]string, 0, 4)
	current := cleaned
	for {
		if _, statErr := os.Lstat(current); statErr == nil {
			resolved := canonicalExistingPath(current)
			for index := len(missing) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, missing[index])
			}
			return filepath.Clean(resolved)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return cleaned
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}

func equalPath(a, b string) bool {
	if strings.TrimSpace(a) == "" || strings.TrimSpace(b) == "" {
		return strings.TrimSpace(a) == strings.TrimSpace(b)
	}
	cleanA := canonicalExistingPath(a)
	cleanB := canonicalExistingPath(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(cleanA, cleanB)
	}
	return cleanA == cleanB
}
