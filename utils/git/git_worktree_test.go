package git

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorktreeMetadataNameIsStableAndPortable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "feature", "demo")
	first := worktreeMetadataName(path)
	second := worktreeMetadataName(path)
	if first != second {
		t.Fatalf("metadata name is not stable: %q != %q", first, second)
	}
	if !strings.HasPrefix(first, "ck-") || strings.ContainsAny(first, `/\\_`) {
		t.Fatalf("metadata name is not portable: %q", first)
	}
}

func TestLinkedWorktreeCapabilitiesUseLinkedIndex(t *testing.T) {
	useBuiltinEngines(t)
	repoDir := initTestRepo(t)
	repo, err := DetectRepository(repoDir)
	if err != nil {
		t.Fatalf("DetectRepository returned error: %v", err)
	}
	defer repo.Close()

	if err := repo.CreateBranch("feature/linked-index", "main"); err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	worktreePath := filepath.Join(t.TempDir(), "feature-linked-index")
	if err := repo.AddWorktree(worktreePath, "feature/linked-index", false); err != nil {
		t.Fatalf("AddWorktree failed: %v", err)
	}

	linked, err := repo.openWorktreeRepository(worktreePath)
	if err != nil {
		t.Fatalf("open linked repository: %v", err)
	}
	idx, err := linked.Storer.Index()
	if err != nil {
		_ = linked.Close()
		t.Fatalf("read linked index: %v", err)
	}
	if len(idx.Entries) == 0 {
		_ = linked.Close()
		t.Fatal("linked index has no entries")
	}
	idx.Entries[0].SkipWorktree = true
	if err := linked.Storer.SetIndex(idx); err != nil {
		_ = linked.Close()
		t.Fatalf("write linked index: %v", err)
	}
	_ = linked.Close()

	mainReport := repo.Capabilities(repoDir)
	if !mainReport.Operations.Status || !mainReport.Operations.Commit {
		t.Fatalf("main worktree was restricted by linked index: %#v", mainReport)
	}
	linkedReport := repo.Capabilities(worktreePath)
	if linkedReport.Operations.Status || linkedReport.Operations.Commit {
		t.Fatalf("linked worktree with unsupported index remained writable: %#v", linkedReport)
	}
	if !hasCapabilityReason(linkedReport, "unsupported_index_extension") {
		t.Fatalf("linked report missing index reason: %#v", linkedReport.Reasons)
	}
}

func TestRemoveAndPruneLinkedWorktrees(t *testing.T) {
	repoDir := initTestRepo(t)
	repo, err := DetectRepository(repoDir)
	if err != nil {
		t.Fatalf("DetectRepository returned error: %v", err)
	}
	defer repo.Close()

	if err := repo.CreateBranch("feature/remove", "main"); err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	removePath := filepath.Join(t.TempDir(), "remove")
	if err := repo.AddWorktree(removePath, "feature/remove", false); err != nil {
		t.Fatalf("AddWorktree failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(removePath, "untracked.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatalf("write untracked file: %v", err)
	}
	if err := repo.RemoveWorktree(removePath, false); ErrorCode(err) != ErrorCodeWorktreeDirty {
		t.Fatalf("non-force removal error = %v, want %s", err, ErrorCodeWorktreeDirty)
	}
	if err := repo.RemoveWorktree(removePath, true); err != nil {
		t.Fatalf("force RemoveWorktree failed: %v", err)
	}
	if _, err := os.Stat(removePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("removed worktree still exists: %v", err)
	}

	if err := repo.CreateBranch("feature/prune", "main"); err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	prunePath := filepath.Join(t.TempDir(), "prune")
	if err := repo.AddWorktree(prunePath, "feature/prune", false); err != nil {
		t.Fatalf("AddWorktree failed: %v", err)
	}
	if err := os.RemoveAll(prunePath); err != nil {
		t.Fatalf("remove linked path: %v", err)
	}
	if err := repo.PruneWorktrees(); err != nil {
		t.Fatalf("PruneWorktrees failed: %v", err)
	}
	worktrees, err := repo.ListWorktrees()
	if err != nil {
		t.Fatalf("ListWorktrees failed: %v", err)
	}
	if len(worktrees) != 1 || !worktrees[0].IsMain {
		t.Fatalf("unexpected worktrees after remove/prune: %#v", worktrees)
	}
}

func TestCoreOperationsDoNotRequireGitExecutable(t *testing.T) {
	useBuiltinEngines(t)
	t.Setenv("PATH", t.TempDir())
	repoDir := initTestRepo(t)
	repo, err := DetectRepository(repoDir)
	if err != nil {
		t.Fatalf("DetectRepository without git on PATH: %v", err)
	}
	defer repo.Close()

	if _, _, err := repo.ListBranches(); err != nil {
		t.Fatalf("ListBranches without git on PATH: %v", err)
	}
	if _, err := repo.GetWorktreeStatus(repoDir); err != nil {
		t.Fatalf("GetWorktreeStatus without git on PATH: %v", err)
	}
	if err := repo.CreateBranch("feature/no-cli", "main"); err != nil {
		t.Fatalf("CreateBranch without git on PATH: %v", err)
	}
	worktreePath := filepath.Join(t.TempDir(), "no-cli")
	if err := repo.AddWorktree(worktreePath, "feature/no-cli", false); err != nil {
		t.Fatalf("AddWorktree without git on PATH: %v", err)
	}
	if err := os.WriteFile(filepath.Join(worktreePath, "pure-go.txt"), []byte("ok\n"), 0o644); err != nil {
		t.Fatalf("write worktree file: %v", err)
	}
	if err := repo.CommitAll(worktreePath, "pure Go commit"); err != nil {
		t.Fatalf("CommitAll without git on PATH: %v", err)
	}
	if err := repo.MergeBranch(repoDir, "feature/no-cli", MergeStrategyMerge); err != nil {
		t.Fatalf("MergeBranch without git on PATH: %v", err)
	}
	if err := repo.RemoveWorktree(worktreePath, true); err != nil {
		t.Fatalf("RemoveWorktree without git on PATH: %v", err)
	}
}

func hasCapabilityReason(report CapabilityReport, code string) bool {
	for _, reason := range report.Reasons {
		if reason.Code == code {
			return true
		}
	}
	return false
}
