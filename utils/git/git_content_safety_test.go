package git

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCapabilitiesDeferContentFilterCheckUntilBuiltinWrite(t *testing.T) {
	useBuiltinEngines(t)
	repoDir := initTestRepo(t)
	assetDir := filepath.Join(repoDir, "assets")
	if err := os.MkdirAll(assetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assetDir, ".gitattributes"), []byte("*.bin filter=lfs diff=lfs merge=lfs -text\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assetDir, "large.bin"), []byte("not really large\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	repo, err := DetectRepository(repoDir)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	report := repo.Capabilities(repoDir)
	if !report.Operations.Commit {
		t.Fatalf("capability probe inspected worktree content: %#v", report)
	}
	if hasCapabilityReason(report, ErrorCodeUnsupportedContentFilter) {
		t.Fatalf("capability probe reported deferred content filter: %#v", report.Reasons)
	}

	if err := repo.CommitAll(repoDir, "unsafe content"); ErrorCode(err) != ErrorCodeUnsupportedContentFilter {
		t.Fatalf("CommitAll error = %v, want %s", err, ErrorCodeUnsupportedContentFilter)
	}

	opened, err := openFilesystemRepository(repoDir, false)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	index, err := opened.Storer.Index()
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range index.Entries {
		if entry.Name == "assets/large.bin" || entry.Name == "assets/.gitattributes" {
			t.Fatalf("unsafe write changed the index: %s", entry.Name)
		}
	}
}

func TestBuiltinCheckoutChecksTargetTreeBeforeWriting(t *testing.T) {
	useBuiltinEngines(t)
	repoDir := initTestRepo(t)
	repo, err := DetectRepository(repoDir)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	if err := repo.CreateBranch("feature/filtered", "main"); err != nil {
		t.Fatal(err)
	}
	if err := repo.CheckoutBranch("feature/filtered"); err != nil {
		t.Fatal(err)
	}
	assetDir := filepath.Join(repoDir, "assets")
	if err := os.MkdirAll(assetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assetDir, ".gitattributes"), []byte("*.bin filter=lfs\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assetDir, "large.bin"), []byte("content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commitAllTestFiles(t, repoDir, "filtered tree")
	if err := repo.CheckoutBranch("main"); err != nil {
		t.Fatal(err)
	}

	if err := repo.CheckoutBranch("feature/filtered"); ErrorCode(err) != ErrorCodeUnsupportedContentFilter {
		t.Fatalf("CheckoutBranch error = %v, want %s", err, ErrorCodeUnsupportedContentFilter)
	}
	branch, err := repo.GetCurrentBranch()
	if err != nil {
		t.Fatal(err)
	}
	if branch != "main" {
		t.Fatalf("branch changed after rejected checkout: %s", branch)
	}
	if _, err := os.Stat(filepath.Join(assetDir, "large.bin")); !os.IsNotExist(err) {
		t.Fatalf("target tree was written before safety check: %v", err)
	}
}

func TestBuiltinWriteAllowsOrdinaryAttributes(t *testing.T) {
	useBuiltinEngines(t)
	repoDir := initTestRepo(t)
	if err := os.WriteFile(filepath.Join(repoDir, ".gitattributes"), []byte("*.txt text eol=lf\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "safe.txt"), []byte("safe\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	repo, err := DetectRepository(repoDir)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.CommitAll(repoDir, "safe attributes"); err != nil {
		t.Fatalf("CommitAll rejected ordinary attributes: %v", err)
	}
}
