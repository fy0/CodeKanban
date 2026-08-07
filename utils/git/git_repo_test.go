package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	goGit "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
)

func TestDetectRepository(t *testing.T) {
	repoDir := initTestRepo(t)

	repo, err := DetectRepository(repoDir)
	if err != nil {
		t.Fatalf("DetectRepository returned error: %v", err)
	}
	defer repo.Close()

	if !equalPath(repo.Path, repoDir) {
		t.Fatalf("expected repository path %q got %q", repoDir, repo.Path)
	}

	if branch, err := repo.GetCurrentBranch(); err != nil || branch != "main" {
		t.Fatalf("unexpected current branch: %q (%v)", branch, err)
	}

	if value, ok := repo.ConfigValue("core.autocrlf"); !ok || strings.ToLower(value) != "true" {
		t.Fatalf("expected core.autocrlf=true, got %q (present=%v)", value, ok)
	}

	remotes, err := repo.GetRemotes()
	if err != nil {
		t.Fatalf("GetRemotes error: %v", err)
	}
	if len(remotes) != 1 || remotes[0].Name != "origin" {
		t.Fatalf("unexpected remotes: %#v", remotes)
	}
}

func TestIsRepositoryPath(t *testing.T) {
	repoDir := initTestRepo(t)
	plainDir := t.TempDir()

	if !IsRepositoryPath(repoDir) {
		t.Fatalf("expected git repository path to return true")
	}

	if IsRepositoryPath(plainDir) {
		t.Fatalf("expected plain directory to return false")
	}
}

func TestBranchAndWorktreeOperations(t *testing.T) {
	repoDir := initTestRepo(t)
	repo, err := DetectRepository(repoDir)
	if err != nil {
		t.Fatalf("DetectRepository returned error: %v", err)
	}
	defer repo.Close()

	if err := repo.CreateBranch("feature/test", "HEAD"); err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	local, _, err := repo.ListBranches()
	if err != nil {
		t.Fatalf("ListBranches failed: %v", err)
	}

	found := false
	for _, branch := range local {
		if branch.Name == "feature/test" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("feature/test branch not present in local branches: %#v", local)
	}

	worktreeParent := t.TempDir()
	worktreePath := filepath.Join(worktreeParent, "feature-test")

	if err := repo.AddWorktree(worktreePath, "feature/test", false); err != nil {
		t.Fatalf("AddWorktree failed: %v", err)
	}
	t.Cleanup(func() {
		_ = repo.RemoveWorktree(worktreePath, true)
	})

	worktrees, err := repo.ListWorktrees()
	if err != nil {
		t.Fatalf("ListWorktrees failed: %v", err)
	}
	if len(worktrees) < 2 {
		t.Fatalf("expected at least 2 worktrees got %d", len(worktrees))
	}

	status, err := GetWorktreeStatus(worktreePath)
	if err != nil {
		t.Fatalf("GetWorktreeStatus failed: %v", err)
	}
	if status.Branch == "" {
		t.Fatalf("expected branch name in worktree status")
	}
}

func TestGetWorktreeStatusUntracked(t *testing.T) {
	repoDir := initTestRepo(t)
	file := filepath.Join(repoDir, "new-file.txt")
	if err := os.WriteFile(file, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	status, err := GetWorktreeStatus(repoDir)
	if err != nil {
		t.Fatalf("GetWorktreeStatus failed: %v", err)
	}

	if status.Untracked == 0 {
		t.Fatalf("expected untracked file count > 0, got %#v", status)
	}
}

func TestCapabilitiesRespectGlobalCommitSigningAndLocalOverride(t *testing.T) {
	useBuiltinEngines(t)
	globalConfig := filepath.Join(t.TempDir(), "global.gitconfig")
	if err := os.WriteFile(globalConfig, []byte("[commit]\n\tgpgSign = true\n"), 0o644); err != nil {
		t.Fatalf("write global config: %v", err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", globalConfig)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	repoDir := initTestRepo(t)
	repository, err := openFilesystemRepository(repoDir, false)
	if err != nil {
		t.Fatalf("open repository: %v", err)
	}
	cfg, err := repository.Config()
	if err != nil {
		_ = repository.Close()
		t.Fatalf("read local config: %v", err)
	}
	cfg.Commit.GpgSign = config.OptBoolUnset
	cfg.Raw.Section("commit").RemoveOption("gpgSign")
	if err := repository.SetConfig(cfg); err != nil {
		_ = repository.Close()
		t.Fatalf("remove local signing override: %v", err)
	}
	_ = repository.Close()

	repo, err := DetectRepository(repoDir)
	if err != nil {
		t.Fatalf("DetectRepository returned error: %v", err)
	}
	report := repo.Capabilities(repoDir)
	if report.Operations.Commit || !hasCapabilityReason(report, "commit_signing_required") {
		_ = repo.Close()
		t.Fatalf("global commit.gpgSign was not enforced: %#v", report)
	}
	_ = repo.Close()

	repository, err = openFilesystemRepository(repoDir, false)
	if err != nil {
		t.Fatalf("reopen repository: %v", err)
	}
	cfg, err = repository.Config()
	if err != nil {
		_ = repository.Close()
		t.Fatalf("read local config: %v", err)
	}
	cfg.Commit.GpgSign = config.OptBoolFalse
	if err := repository.SetConfig(cfg); err != nil {
		_ = repository.Close()
		t.Fatalf("set local signing override: %v", err)
	}
	_ = repository.Close()

	repo, err = DetectRepository(repoDir)
	if err != nil {
		t.Fatalf("DetectRepository returned error: %v", err)
	}
	defer repo.Close()
	if report = repo.Capabilities(repoDir); !report.Operations.Commit {
		t.Fatalf("local commit.gpgSign=false did not override global true: %#v", report)
	}
}

func TestUnsupportedRepositoryFormatIsNeverRewritten(t *testing.T) {
	useBuiltinEngines(t)
	repoDir := initTestRepo(t)
	configPath := filepath.Join(repoDir, ".git", "config")
	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read repository config: %v", err)
	}
	unsupported := strings.Replace(string(configData), "repositoryformatversion = 0", "repositoryformatversion = 1", 1) +
		"\n[extensions]\n\tobjectFormat = sha256\n"
	if err := os.WriteFile(configPath, []byte(unsupported), 0o644); err != nil {
		t.Fatalf("write unsupported config: %v", err)
	}
	headPath := filepath.Join(repoDir, ".git", "HEAD")
	headBefore, err := os.ReadFile(headPath)
	if err != nil {
		t.Fatalf("read HEAD: %v", err)
	}

	if !HasRepositoryStructure(repoDir) {
		t.Fatal("unsupported repository was not recognized structurally")
	}
	repo, detectErr := DetectRepository(repoDir)
	if detectErr == nil {
		report := repo.Capabilities(repoDir)
		_ = repo.Close()
		if report.Mode != CapabilityModeUnavailable || hasAnyWriteCapability(report.Operations) {
			t.Fatalf("unsupported repository was not disabled: %#v", report)
		}
	} else if ErrorCode(detectErr) != ErrorCodeUnsupportedFormat {
		t.Fatalf("DetectRepository error = %v, want %s", detectErr, ErrorCodeUnsupportedFormat)
	}

	configAfter, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config after probe: %v", err)
	}
	headAfter, err := os.ReadFile(headPath)
	if err != nil {
		t.Fatalf("read HEAD after probe: %v", err)
	}
	if string(configAfter) != unsupported || string(headAfter) != string(headBefore) {
		t.Fatal("repository metadata changed while probing unsupported format")
	}
}

func initTestRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	repository, err := goGit.PlainInit(dir, false, goGit.WithDefaultBranch(plumbing.NewBranchReferenceName("main")))
	if err != nil {
		t.Fatalf("init repository: %v", err)
	}

	readme := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readme, []byte("# Test Repo\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}

	cfg, err := repository.Config()
	if err != nil {
		t.Fatalf("read repository config: %v", err)
	}
	cfg.Core.AutoCRLF = "true"
	cfg.User.Name = "Test User"
	cfg.User.Email = "test@example.com"
	cfg.Commit.GpgSign = config.OptBoolFalse
	if err := repository.SetConfig(cfg); err != nil {
		t.Fatalf("write repository config: %v", err)
	}
	if _, err := repository.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{"git@example.com:repo.git"},
	}); err != nil {
		t.Fatalf("create remote: %v", err)
	}
	worktree, err := repository.Worktree()
	if err != nil {
		t.Fatalf("open worktree: %v", err)
	}
	if err := worktree.AddWithOptions(&goGit.AddOptions{All: true}); err != nil {
		t.Fatalf("stage initial files: %v", err)
	}
	if _, err := worktree.Commit("initial commit", &goGit.CommitOptions{Author: testSignature()}); err != nil {
		t.Fatalf("create initial commit: %v", err)
	}
	if err := repository.Close(); err != nil {
		t.Fatalf("close repository: %v", err)
	}
	reopened, err := openFilesystemRepository(dir, false)
	if err != nil {
		headData, headErr := os.ReadFile(filepath.Join(dir, ".git", "HEAD"))
		t.Fatalf("reopen repository: %v (HEAD=%q, headErr=%v)", err, string(headData), headErr)
	}
	_ = reopened.Close()
	return dir
}

func initTestRepoWithoutCommit(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	repository, err := goGit.PlainInit(dir, false, goGit.WithDefaultBranch(plumbing.NewBranchReferenceName("main")))
	if err != nil {
		t.Fatalf("init repository: %v", err)
	}
	cfg, err := repository.Config()
	if err != nil {
		t.Fatalf("read repository config: %v", err)
	}
	cfg.User.Name = "Test User"
	cfg.User.Email = "test@example.com"
	cfg.Commit.GpgSign = config.OptBoolFalse
	if err := repository.SetConfig(cfg); err != nil {
		t.Fatalf("write repository config: %v", err)
	}
	_ = repository.Close()
	reopened, err := openFilesystemRepository(dir, false)
	if err != nil {
		headData, headErr := os.ReadFile(filepath.Join(dir, ".git", "HEAD"))
		t.Fatalf("reopen repository: %v (HEAD=%q, headErr=%v)", err, string(headData), headErr)
	}
	_ = reopened.Close()
	return dir
}

func stageAllTestFiles(t *testing.T, dir string) {
	t.Helper()
	repository, err := openFilesystemRepository(dir, false)
	if err != nil {
		t.Fatalf("open repository: %v", err)
	}
	defer repository.Close()
	worktree, err := repository.Worktree()
	if err != nil {
		t.Fatalf("open worktree: %v", err)
	}
	if err := worktree.AddWithOptions(&goGit.AddOptions{All: true}); err != nil {
		t.Fatalf("stage files: %v", err)
	}
}

func commitAllTestFiles(t *testing.T, dir, message string) {
	t.Helper()
	repository, err := openFilesystemRepository(dir, false)
	if err != nil {
		t.Fatalf("open repository: %v", err)
	}
	defer repository.Close()
	worktree, err := repository.Worktree()
	if err != nil {
		t.Fatalf("open worktree: %v", err)
	}
	if err := worktree.AddWithOptions(&goGit.AddOptions{All: true}); err != nil {
		t.Fatalf("stage files: %v", err)
	}
	if _, err := worktree.Commit(message, &goGit.CommitOptions{Author: testSignature()}); err != nil {
		t.Fatalf("commit files: %v", err)
	}
}

func testSignature() *object.Signature {
	return &object.Signature{Name: "Test User", Email: "test@example.com", When: time.Now()}
}
