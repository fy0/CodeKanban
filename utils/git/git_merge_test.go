package git

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMergeBranchRejectsUnsupportedStrategies(t *testing.T) {
	useBuiltinEngines(t)
	repoDir := initTestRepo(t)
	repo, err := DetectRepository(repoDir)
	if err != nil {
		t.Fatalf("DetectRepository returned error: %v", err)
	}
	defer repo.Close()

	for _, strategy := range []MergeStrategy{"rebase", "squash"} {
		t.Run(string(strategy), func(t *testing.T) {
			err := repo.MergeBranch(repoDir, "main", strategy)
			if ErrorCode(err) != ErrorCodeOperationUnsupported {
				t.Fatalf("MergeBranch(%q) error = %v, want %s", strategy, err, ErrorCodeOperationUnsupported)
			}
		})
	}
}

func TestMergeBranchRejectsNonFastForwardWithoutChangingTarget(t *testing.T) {
	useBuiltinEngines(t)
	repoDir := initTestRepo(t)
	repo, err := DetectRepository(repoDir)
	if err != nil {
		t.Fatalf("DetectRepository returned error: %v", err)
	}
	defer repo.Close()

	if err := repo.CreateBranch("feature/diverged", "main"); err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	if err := repo.CheckoutBranch("feature/diverged"); err != nil {
		t.Fatalf("checkout feature: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "feature.txt"), []byte("feature\n"), 0o644); err != nil {
		t.Fatalf("write feature file: %v", err)
	}
	if err := repo.CommitAll(repoDir, "feature commit"); err != nil {
		t.Fatalf("commit feature: %v", err)
	}
	if err := repo.CheckoutBranch("main"); err != nil {
		t.Fatalf("checkout main: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "main.txt"), []byte("main\n"), 0o644); err != nil {
		t.Fatalf("write main file: %v", err)
	}
	if err := repo.CommitAll(repoDir, "main commit"); err != nil {
		t.Fatalf("commit main: %v", err)
	}

	before, err := repo.repository.Head()
	if err != nil {
		t.Fatalf("read target HEAD: %v", err)
	}
	if err := repo.MergeBranch(repoDir, "feature/diverged", MergeStrategyMerge); ErrorCode(err) != ErrorCodeNonFastForward {
		t.Fatalf("non-fast-forward error = %v, want %s", err, ErrorCodeNonFastForward)
	}
	after, err := repo.repository.Head()
	if err != nil {
		t.Fatalf("read target HEAD after rejection: %v", err)
	}
	if after.Hash() != before.Hash() {
		t.Fatalf("target HEAD changed after rejected merge: %s -> %s", before.Hash(), after.Hash())
	}
	if _, err := os.Stat(filepath.Join(repoDir, "feature.txt")); !os.IsNotExist(err) {
		t.Fatalf("feature file appeared after rejected merge: %v", err)
	}
}

func useBuiltinEngines(t *testing.T) {
	t.Helper()
	previous := CurrentEngineSettings()
	ConfigureEngines(EngineSettings{Read: EnginePreferenceBuiltin, Write: EnginePreferenceBuiltin})
	t.Cleanup(func() { ConfigureEngines(previous) })
}
