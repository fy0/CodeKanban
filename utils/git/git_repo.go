package git

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/go-git/go-billy/v6/osfs"
	goGit "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing/cache"
	"github.com/go-git/go-git/v6/storage/filesystem"
	"github.com/go-git/go-git/v6/storage/filesystem/dotgit"
	xworktree "github.com/go-git/go-git/v6/x/plumbing/worktree"
)

type GitRepo struct {
	Path         string
	commonDir    string
	mainWorktree string
	repository   *goGit.Repository
	worktrees    *xworktree.Worktree
	isBare       bool
	systemOnly   bool
	lock         *sync.RWMutex
}

type Remote struct {
	Name string
	URL  string
}

var (
	errEmptyPath = errors.New("path is required")
	repoLocks    sync.Map
)

func IsRepositoryPath(path string) bool {
	repo, err := DetectRepository(path)
	if err != nil {
		return false
	}
	_ = repo.Close()
	return true
}

// HasRepositoryStructure reports whether path contains Git repository
// metadata without requiring go-git to understand that metadata. It is used
// only for capability reporting so unsupported repositories are classified as
// Git repositories with disabled operations, never as ordinary directories.
func HasRepositoryStructure(path string) bool {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return false
	}
	absPath, err := filepath.Abs(trimmed)
	if err != nil {
		return false
	}
	info, err := os.Stat(absPath)
	if err != nil || !info.IsDir() {
		return false
	}
	dotGitInfo, dotGitErr := os.Lstat(filepath.Join(absPath, goGit.GitDirName))
	if dotGitErr == nil && (dotGitInfo.IsDir() || dotGitInfo.Mode().IsRegular()) {
		return true
	}
	return looksLikeGitDir(absPath)
}

func DetectRepository(path string) (*GitRepo, error) {
	absPath, layout, err := discoverRepository(path)
	if err != nil {
		return nil, err
	}

	openPath := layout.mainWorktree
	if layout.bare || openPath == "" {
		openPath = layout.commonDir
	}
	repository, err := openFilesystemRepository(openPath, layout.bare)
	if err != nil {
		if !systemGitRepositoryAvailable(context.Background(), absPath) {
			return nil, &OperationError{
				Code:   ErrorCodeUnsupportedFormat,
				Detail: fmt.Sprintf("repository cannot be opened by go-git: %v", err),
				Err:    err,
			}
		}
		lockValue, _ := repoLocks.LoadOrStore(normalizePathKey(layout.commonDir), &sync.RWMutex{})
		return &GitRepo{
			Path:         absPath,
			commonDir:    layout.commonDir,
			mainWorktree: layout.mainWorktree,
			isBare:       layout.bare,
			systemOnly:   true,
			lock:         lockValue.(*sync.RWMutex),
		}, nil
	}

	manager, err := xworktree.New(repository.Storer)
	if err != nil && !layout.bare {
		_ = repository.Close()
		return nil, &OperationError{
			Code:   ErrorCodeLinkedWorktreeUnreliable,
			Detail: "linked worktree storage is unavailable",
			Err:    err,
		}
	}

	lockValue, _ := repoLocks.LoadOrStore(normalizePathKey(layout.commonDir), &sync.RWMutex{})
	repo := &GitRepo{
		Path:         absPath,
		commonDir:    layout.commonDir,
		mainWorktree: layout.mainWorktree,
		repository:   repository,
		worktrees:    manager,
		isBare:       layout.bare,
		lock:         lockValue.(*sync.RWMutex),
	}

	if layout.linked {
		linkedRepo, openErr := repo.openWorktreeRepository(absPath)
		if openErr != nil {
			_ = repo.Close()
			return nil, &OperationError{
				Code:   ErrorCodeLinkedWorktreeUnreliable,
				Detail: "linked worktree cannot be opened",
				Err:    openErr,
			}
		}
		_ = linkedRepo.Close()
	}

	return repo, nil
}

func (r *GitRepo) Close() error {
	if r == nil || r.repository == nil {
		return nil
	}
	err := r.repository.Close()
	r.repository = nil
	r.worktrees = nil
	return err
}

func (r *GitRepo) GetRemotes() ([]Remote, error) {
	if r == nil {
		return nil, errors.New("git repository is not initialized")
	}
	engine, err := r.requireEngine(r.Path, OperationBranchesRead)
	if err != nil {
		return nil, err
	}
	if engine == EngineSystem {
		output, err := runSystemGitOutputAllowExitOne(context.Background(), r.Path, OperationBranchesRead, "config", "--get-regexp", `^remote\..*\.url$`)
		if err != nil {
			return nil, err
		}
		items := make([]Remote, 0)
		for _, line := range strings.Split(string(output), "\n") {
			parts := strings.Fields(strings.TrimSpace(line))
			if len(parts) < 2 {
				continue
			}
			name := strings.TrimSuffix(strings.TrimPrefix(parts[0], "remote."), ".url")
			items = append(items, Remote{Name: name, URL: strings.Join(parts[1:], " ")})
		}
		return items, nil
	}
	if r.repository == nil {
		return nil, errors.New("built-in Git repository is not initialized")
	}

	remotes, err := r.repository.Remotes()
	if err != nil {
		return nil, err
	}
	items := make([]Remote, 0, len(remotes))
	for _, remote := range remotes {
		cfg := remote.Config()
		if len(cfg.URLs) == 0 {
			continue
		}
		items = append(items, Remote{Name: cfg.Name, URL: cfg.URLs[0]})
	}
	return items, nil
}

func (r *GitRepo) GetCurrentBranch() (string, error) {
	if r == nil {
		return "", errors.New("git repository is not initialized")
	}
	engine, err := r.requireEngine(r.Path, OperationBranchesRead)
	if err != nil {
		return "", err
	}
	if engine == EngineSystem {
		output, err := runSystemGitOutputAllowExitOne(context.Background(), r.Path, OperationBranchesRead, "symbolic-ref", "--quiet", "--short", "HEAD")
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(output)), nil
	}
	if r.repository == nil {
		return "", errors.New("built-in Git repository is not initialized")
	}
	repository, err := r.openWorktreeRepository(r.Path)
	if err != nil {
		if r.isBare {
			repository = r.repository
		} else {
			return "", err
		}
	} else {
		defer repository.Close()
	}

	head, err := repository.Head()
	if err != nil {
		return "", err
	}
	if !head.Name().IsBranch() {
		return "", nil
	}
	return head.Name().Short(), nil
}

func (r *GitRepo) ConfigValue(key string) (string, bool) {
	if r == nil {
		return "", false
	}
	name := strings.ToLower(strings.TrimSpace(key))
	if name == "" {
		return "", false
	}
	engine, err := r.requireEngine(r.Path, OperationBranchesRead)
	if err != nil {
		return "", false
	}
	if engine == EngineSystem {
		output, err := runSystemGitOutput(context.Background(), r.Path, OperationBranchesRead, "config", "--get", name)
		return strings.TrimSpace(string(output)), err == nil
	}
	if r.repository == nil {
		return "", false
	}
	parts := strings.SplitN(name, ".", 3)
	if len(parts) < 2 {
		return "", false
	}

	cfg, err := r.repository.ConfigScoped(config.SystemScope)
	if err != nil {
		cfg, err = r.repository.Config()
	}
	if err != nil || cfg == nil {
		return "", false
	}
	if cfg.Raw == nil {
		if _, marshalErr := cfg.Marshal(); marshalErr != nil {
			return "", false
		}
	}
	if len(parts) == 2 {
		section := cfg.Raw.Section(parts[0])
		if !section.HasOption(parts[1]) {
			return "", false
		}
		return section.Option(parts[1]), true
	}
	section := cfg.Raw.Section(parts[0])
	if !section.HasSubsection(parts[1]) {
		return "", false
	}
	subsection := section.Subsection(parts[1])
	if !subsection.HasOption(parts[2]) {
		return "", false
	}
	return subsection.Option(parts[2]), true
}

type repositoryLayout struct {
	commonDir    string
	mainWorktree string
	gitDir       string
	linked       bool
	bare         bool
}

func discoverRepository(path string) (string, repositoryLayout, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", repositoryLayout{}, errEmptyPath
	}
	absPath, err := filepath.Abs(trimmed)
	if err != nil {
		return "", repositoryLayout{}, fmt.Errorf("resolve git path: %w", err)
	}
	absPath = filepath.Clean(absPath)
	info, err := os.Stat(absPath)
	if err != nil {
		return "", repositoryLayout{}, fmt.Errorf("stat git path: %w", err)
	}
	if !info.IsDir() {
		return "", repositoryLayout{}, fmt.Errorf("not a git repository: path is not a directory")
	}

	dotGit := filepath.Join(absPath, ".git")
	dotGitInfo, dotGitErr := os.Lstat(dotGit)
	if dotGitErr == nil && dotGitInfo.IsDir() {
		return absPath, repositoryLayout{
			commonDir:    canonicalExistingPath(dotGit),
			mainWorktree: absPath,
			gitDir:       canonicalExistingPath(dotGit),
		}, nil
	}
	if dotGitErr == nil && dotGitInfo.Mode().IsRegular() {
		gitDir, err := readGitDirFile(dotGit)
		if err != nil {
			return "", repositoryLayout{}, fmt.Errorf("read linked worktree metadata: %w", err)
		}
		commonDir, err := readCommonDir(gitDir)
		if err != nil {
			return "", repositoryLayout{}, fmt.Errorf("resolve common git directory: %w", err)
		}
		mainWorktree := ""
		if strings.EqualFold(filepath.Base(commonDir), ".git") {
			mainWorktree = filepath.Dir(commonDir)
		}
		return absPath, repositoryLayout{
			commonDir:    commonDir,
			mainWorktree: mainWorktree,
			gitDir:       gitDir,
			linked:       !equalPath(gitDir, commonDir),
		}, nil
	}

	if looksLikeGitDir(absPath) {
		if strings.EqualFold(filepath.Base(absPath), ".git") {
			return absPath, repositoryLayout{
				commonDir:    canonicalExistingPath(absPath),
				mainWorktree: filepath.Dir(absPath),
				gitDir:       canonicalExistingPath(absPath),
			}, nil
		}
		return absPath, repositoryLayout{
			commonDir: canonicalExistingPath(absPath),
			gitDir:    canonicalExistingPath(absPath),
			bare:      true,
		}, nil
	}
	return "", repositoryLayout{}, &OperationError{Code: ErrorCodeNotRepository, Detail: "not a git repository"}
}

func (r *GitRepo) openWorktreeRepository(path string) (*goGit.Repository, error) {
	if r == nil || r.repository == nil {
		return nil, errors.New("git repository is not initialized")
	}
	if r.isBare {
		return nil, errors.New("bare repository has no worktree")
	}
	absPath, layout, err := discoverRepository(path)
	if err != nil {
		return nil, err
	}
	if !equalPath(layout.commonDir, r.commonDir) {
		return nil, errors.New("worktree belongs to a different repository")
	}
	if !layout.linked {
		return openFilesystemRepository(absPath, false)
	}
	return openLinkedFilesystemRepository(absPath, layout.gitDir, layout.commonDir)
}

func (r *GitRepo) resolveWorktreeGitDir(path string) (string, bool, error) {
	if r == nil {
		return "", false, errors.New("git repository is not initialized")
	}
	_, layout, err := discoverRepository(path)
	if err != nil {
		return "", false, err
	}
	if !equalPath(layout.commonDir, r.commonDir) {
		return "", false, errors.New("worktree belongs to a different repository")
	}
	return layout.gitDir, layout.linked, nil
}

func readGitDirFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	reader := bufio.NewReader(io.LimitReader(file, 4096))
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	line = strings.TrimSpace(line)
	const prefix = "gitdir:"
	if len(line) < len(prefix) || !strings.EqualFold(line[:len(prefix)], prefix) {
		return "", errors.New("invalid .git file")
	}
	value := strings.TrimSpace(line[len(prefix):])
	if value == "" {
		return "", errors.New("empty gitdir path")
	}
	if !filepath.IsAbs(value) {
		value = filepath.Join(filepath.Dir(path), value)
	}
	value = filepath.Clean(value)
	if info, statErr := os.Stat(value); statErr != nil || !info.IsDir() {
		if statErr != nil {
			return "", statErr
		}
		return "", errors.New("gitdir is not a directory")
	}
	return canonicalExistingPath(value), nil
}

func readCommonDir(gitDir string) (string, error) {
	commonFile := filepath.Join(gitDir, "commondir")
	data, err := os.ReadFile(commonFile)
	if errors.Is(err, os.ErrNotExist) {
		return canonicalExistingPath(gitDir), nil
	}
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(string(data))
	if value == "" {
		return "", errors.New("empty commondir")
	}
	if !filepath.IsAbs(value) {
		value = filepath.Join(gitDir, value)
	}
	value = filepath.Clean(value)
	info, err := os.Stat(value)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("commondir is not a directory")
	}
	return canonicalExistingPath(value), nil
}

func looksLikeGitDir(path string) bool {
	for _, name := range []string{"HEAD", "objects", "refs"} {
		if _, err := os.Stat(filepath.Join(path, name)); err != nil {
			return false
		}
	}
	return true
}

func canonicalExistingPath(path string) string {
	cleaned := filepath.Clean(path)
	if evaluated, err := filepath.EvalSymlinks(cleaned); err == nil {
		return filepath.Clean(evaluated)
	}
	return cleaned
}

func normalizePathKey(path string) string {
	cleaned := canonicalExistingPath(path)
	if runtime.GOOS == "windows" {
		return strings.ToLower(cleaned)
	}
	return cleaned
}

func openFilesystemRepository(path string, bare bool) (*goGit.Repository, error) {
	root := osfs.New(filepath.Clean(path), osfs.WithBoundOS())
	worktree := root
	dotGit := root
	if !bare {
		dotGit = osfs.New(filepath.Join(filepath.Clean(path), goGit.GitDirName), osfs.WithBoundOS())
	}
	if _, err := dotGit.Stat("HEAD"); err != nil {
		return nil, fmt.Errorf("open repository HEAD at %s: %w", dotGit.Root(), err)
	}
	repositoryFS := dotgit.NewRepositoryFilesystem(dotGit, nil)
	storage := filesystem.NewStorage(repositoryFS, cache.NewObjectLRUDefault())
	if bare {
		worktree = nil
	}
	repository, err := goGit.Open(storage, worktree)
	if err != nil {
		_ = storage.Close()
		return nil, err
	}
	return repository, nil
}

func openLinkedFilesystemRepository(worktreePath, gitDir, commonDir string) (*goGit.Repository, error) {
	worktree := osfs.New(filepath.Clean(worktreePath), osfs.WithBoundOS())
	dotGit := osfs.New(filepath.Clean(gitDir), osfs.WithBoundOS())
	common := osfs.New(filepath.Clean(commonDir), osfs.WithBoundOS())
	repositoryFS := dotgit.NewRepositoryFilesystem(dotGit, common)
	storage := filesystem.NewStorage(repositoryFS, cache.NewObjectLRUDefault())
	repository, err := goGit.Open(storage, worktree)
	if err != nil {
		_ = storage.Close()
		return nil, err
	}
	return repository, nil
}
