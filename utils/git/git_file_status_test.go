package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestListFileStatuses(t *testing.T) {
	repoDir := initTestRepoWithTrackedFile(t, "notes.txt", "hello\n")

	readmePath := filepath.Join(repoDir, "README.md")
	if err := os.WriteFile(readmePath, []byte("# Test Repo\nupdated\n"), 0o644); err != nil {
		t.Fatalf("rewrite README: %v", err)
	}
	if err := os.Rename(filepath.Join(repoDir, "notes.txt"), filepath.Join(repoDir, "docs.txt")); err != nil {
		t.Fatalf("rename tracked file: %v", err)
	}
	runGit(t, repoDir, "add", "docs.txt", "notes.txt")
	if err := os.MkdirAll(filepath.Join(repoDir, "scratch"), 0o755); err != nil {
		t.Fatalf("mkdir scratch: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "scratch", "draft.md"), []byte("draft\n"), 0o644); err != nil {
		t.Fatalf("write untracked file: %v", err)
	}
	if err := os.Remove(readmePath); err != nil {
		t.Fatalf("remove README: %v", err)
	}

	statuses, err := ListFileStatuses(repoDir)
	if err != nil {
		t.Fatalf("ListFileStatuses returned error: %v", err)
	}

	if statuses["README.md"].Kind != FileChangeKindDeleted {
		t.Fatalf("README.md status = %#v", statuses["README.md"])
	}
	if statuses["docs.txt"].Kind != FileChangeKindRenamed {
		t.Fatalf("docs.txt status = %#v", statuses["docs.txt"])
	}
	if statuses["docs.txt"].PreviousPath != "notes.txt" {
		t.Fatalf("expected previous path notes.txt, got %#v", statuses["docs.txt"])
	}
	if statuses["scratch/draft.md"].Kind != FileChangeKindUntracked {
		t.Fatalf("scratch/draft.md status = %#v", statuses["scratch/draft.md"])
	}
}

func TestParseGitFileStatusesPorcelainV2HandlesConflicts(t *testing.T) {
	output := strings.Join([]string{
		"1 M. N... 100644 100644 100644 abcdef0 abcdef0 README.md",
		"u UU N... 100644 100644 100644 100644 abcdef0 abcdef0 abcdef0 conflict.txt",
		"? new file.txt",
		"",
	}, "\x00")

	statuses := parseGitFileStatusesPorcelainV2([]byte(output))
	if statuses["README.md"].Kind != FileChangeKindModified {
		t.Fatalf("README.md status = %#v", statuses["README.md"])
	}
	if statuses["conflict.txt"].Kind != FileChangeKindConflicted {
		t.Fatalf("conflict.txt status = %#v", statuses["conflict.txt"])
	}
	if statuses["new file.txt"].Kind != FileChangeKindUntracked {
		t.Fatalf("new file.txt status = %#v", statuses["new file.txt"])
	}
}

func TestListFileStatusesContextCanSkipUntracked(t *testing.T) {
	repoDir := initTestRepoWithTrackedFile(t, "notes.txt", "hello\n")

	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("# Test Repo\nupdated\n"), 0o644); err != nil {
		t.Fatalf("rewrite README: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "scratch.txt"), []byte("draft\n"), 0o644); err != nil {
		t.Fatalf("write scratch.txt: %v", err)
	}

	statuses, err := ListFileStatusesContext(context.Background(), repoDir, false)
	if err != nil {
		t.Fatalf("ListFileStatusesContext returned error: %v", err)
	}

	if statuses["README.md"].Kind != FileChangeKindModified {
		t.Fatalf("README.md status = %#v", statuses["README.md"])
	}
	if _, exists := statuses["scratch.txt"]; exists {
		t.Fatalf("scratch.txt should be excluded when includeUntracked=false: %#v", statuses["scratch.txt"])
	}
}

func TestListFileStatusesChangeTokenTracksContentWithinSameStatus(t *testing.T) {
	repoDir := initTestRepo(t)
	readmePath := filepath.Join(repoDir, "README.md")
	firstModifiedAt := time.Now().Add(-2 * time.Second)

	if err := os.WriteFile(readmePath, []byte("version-one\n"), 0o644); err != nil {
		t.Fatalf("write first README version: %v", err)
	}
	if err := os.Chtimes(readmePath, firstModifiedAt, firstModifiedAt); err != nil {
		t.Fatalf("set first README timestamp: %v", err)
	}

	first, err := ListFileStatusesLimitedContext(context.Background(), repoDir, false, 1000)
	if err != nil {
		t.Fatalf("first ListFileStatusesLimitedContext returned error: %v", err)
	}
	unchanged, err := ListFileStatusesLimitedContext(context.Background(), repoDir, false, 1000)
	if err != nil {
		t.Fatalf("unchanged ListFileStatusesLimitedContext returned error: %v", err)
	}
	if first.ChangeToken == "" || first.ChangeToken != unchanged.ChangeToken {
		t.Fatalf("unchanged token mismatch: first=%q unchanged=%q", first.ChangeToken, unchanged.ChangeToken)
	}

	secondModifiedAt := firstModifiedAt.Add(time.Second)
	if err := os.WriteFile(readmePath, []byte("version-two\n"), 0o644); err != nil {
		t.Fatalf("write second README version: %v", err)
	}
	if err := os.Chtimes(readmePath, secondModifiedAt, secondModifiedAt); err != nil {
		t.Fatalf("set second README timestamp: %v", err)
	}

	changed, err := ListFileStatusesLimitedContext(context.Background(), repoDir, false, 1000)
	if err != nil {
		t.Fatalf("changed ListFileStatusesLimitedContext returned error: %v", err)
	}
	if changed.Statuses["README.md"].Kind != FileChangeKindModified {
		t.Fatalf("README status changed unexpectedly: %#v", changed.Statuses["README.md"])
	}
	if changed.ChangeToken == first.ChangeToken {
		t.Fatalf("content change with the same git status did not update token: %q", changed.ChangeToken)
	}
}

func TestListFileStatusesLimitedContextTruncatesResults(t *testing.T) {
	repoDir := initTestRepoWithTrackedFile(t, "notes.txt", "hello\n")

	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("# Test Repo\nupdated\n"), 0o644); err != nil {
		t.Fatalf("rewrite README: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "scratch.txt"), []byte("draft\n"), 0o644); err != nil {
		t.Fatalf("write scratch.txt: %v", err)
	}

	result, err := ListFileStatusesLimitedContext(context.Background(), repoDir, true, 1)
	if err != nil {
		t.Fatalf("ListFileStatusesLimitedContext returned error: %v", err)
	}
	if !result.Truncated {
		t.Fatalf("expected truncated result: %#v", result)
	}
	if len(result.Statuses) != 1 {
		t.Fatalf("expected exactly one retained status, got %d", len(result.Statuses))
	}
	if result.TotalCount < 2 {
		t.Fatalf("expected total count to include dropped records, got %d", result.TotalCount)
	}
}

func TestGenerateUnifiedDiffAgainstHEAD(t *testing.T) {
	repoDir := initTestRepo(t)
	readmePath := filepath.Join(repoDir, "README.md")
	if err := os.WriteFile(readmePath, []byte("# Test Repo\nextra line\n"), 0o644); err != nil {
		t.Fatalf("rewrite README: %v", err)
	}

	diffText, err := GenerateUnifiedDiffAgainstHEAD(repoDir, "README.md", "")
	if err != nil {
		t.Fatalf("GenerateUnifiedDiffAgainstHEAD returned error: %v", err)
	}
	if !strings.Contains(diffText, "--- a/README.md") {
		t.Fatalf("diff text missing old header: %s", diffText)
	}
	if !strings.Contains(diffText, "+++ b/README.md") {
		t.Fatalf("diff text missing new header: %s", diffText)
	}
	if !strings.Contains(diffText, "+extra line") {
		t.Fatalf("diff text missing content: %s", diffText)
	}
}

func TestGenerateUnifiedDiffAgainstHEADWithoutCommit(t *testing.T) {
	repoDir := t.TempDir()
	runGit(t, repoDir, "init", "-b", "main")

	filePath := filepath.Join(repoDir, "draft.txt")
	if err := os.WriteFile(filePath, []byte("draft\n"), 0o644); err != nil {
		t.Fatalf("write draft file: %v", err)
	}

	diffText, err := GenerateUnifiedDiffAgainstHEAD(repoDir, "draft.txt", "")
	if err != nil {
		t.Fatalf("GenerateUnifiedDiffAgainstHEAD returned error: %v", err)
	}
	if !strings.Contains(diffText, "diff --git a/draft.txt b/draft.txt") {
		t.Fatalf("diff text missing prefixed git header: %s", diffText)
	}
	if !strings.Contains(diffText, "--- /dev/null") {
		t.Fatalf("diff text missing null old header: %s", diffText)
	}
	if !strings.Contains(diffText, "+++ b/draft.txt") {
		t.Fatalf("diff text missing new header: %s", diffText)
	}
	if !strings.Contains(diffText, "+draft") {
		t.Fatalf("diff text missing added content: %s", diffText)
	}
}

func TestGenerateDiffStatAgainstHEAD(t *testing.T) {
	repoDir := initTestRepo(t)
	readmePath := filepath.Join(repoDir, "README.md")
	if err := os.WriteFile(readmePath, []byte("# Test Repo\nextra line\n"), 0o644); err != nil {
		t.Fatalf("rewrite README: %v", err)
	}

	stat, err := GenerateDiffStatAgainstHEAD(repoDir, FileStatus{
		Path: "README.md",
		Kind: FileChangeKindModified,
	})
	if err != nil {
		t.Fatalf("GenerateDiffStatAgainstHEAD returned error: %v", err)
	}
	if stat.Additions != 1 || stat.Deletions != 0 {
		t.Fatalf("unexpected diff stat: %#v", stat)
	}
}

func TestGenerateDiffStatAgainstHEADForUntracked(t *testing.T) {
	repoDir := initTestRepo(t)
	filePath := filepath.Join(repoDir, "draft.txt")
	if err := os.WriteFile(filePath, []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatalf("write draft file: %v", err)
	}

	stat, err := GenerateDiffStatAgainstHEAD(repoDir, FileStatus{
		Path: "draft.txt",
		Kind: FileChangeKindUntracked,
	})
	if err != nil {
		t.Fatalf("GenerateDiffStatAgainstHEAD returned error: %v", err)
	}
	if stat.Additions != 2 || stat.Deletions != 0 {
		t.Fatalf("unexpected untracked diff stat: %#v", stat)
	}
}

func TestGenerateDiffStatAgainstHEADForUntrackedWithoutTrailingNewline(t *testing.T) {
	repoDir := initTestRepo(t)
	filePath := filepath.Join(repoDir, "draft.txt")
	if err := os.WriteFile(filePath, []byte("one\ntwo"), 0o644); err != nil {
		t.Fatalf("write draft file: %v", err)
	}

	stat, err := GenerateDiffStatAgainstHEAD(repoDir, FileStatus{
		Path: "draft.txt",
		Kind: FileChangeKindUntracked,
	})
	if err != nil {
		t.Fatalf("GenerateDiffStatAgainstHEAD returned error: %v", err)
	}
	if stat.Additions != 2 || stat.Deletions != 0 {
		t.Fatalf("unexpected untracked diff stat: %#v", stat)
	}
}

func TestGenerateDiffStatAgainstHEADForBinaryUntrackedReturnsZero(t *testing.T) {
	repoDir := initTestRepo(t)
	filePath := filepath.Join(repoDir, "draft.bin")
	if err := os.WriteFile(filePath, []byte{0x00, 0x01, 0x02}, 0o644); err != nil {
		t.Fatalf("write draft file: %v", err)
	}

	stat, err := GenerateDiffStatAgainstHEAD(repoDir, FileStatus{
		Path: "draft.bin",
		Kind: FileChangeKindUntracked,
	})
	if err != nil {
		t.Fatalf("GenerateDiffStatAgainstHEAD returned error: %v", err)
	}
	if stat.Additions != 0 || stat.Deletions != 0 {
		t.Fatalf("unexpected binary untracked diff stat: %#v", stat)
	}
}

func TestGenerateDiffStatsAgainstHEADContextBatchesTrackedAndUntrackedChanges(t *testing.T) {
	repoDir := initTestRepoWithTrackedFile(t, "notes.txt", "hello\n")

	readmePath := filepath.Join(repoDir, "README.md")
	if err := os.WriteFile(readmePath, []byte("# Test Repo\nextra line\n"), 0o644); err != nil {
		t.Fatalf("rewrite README: %v", err)
	}
	if err := os.Rename(filepath.Join(repoDir, "notes.txt"), filepath.Join(repoDir, "docs.txt")); err != nil {
		t.Fatalf("rename tracked file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "scratch.txt"), []byte("draft\n"), 0o644); err != nil {
		t.Fatalf("write scratch.txt: %v", err)
	}
	runGit(t, repoDir, "add", "docs.txt", "notes.txt")

	statusMap, err := ListFileStatuses(repoDir)
	if err != nil {
		t.Fatalf("ListFileStatuses returned error: %v", err)
	}

	statuses := make([]FileStatus, 0, len(statusMap))
	for _, status := range statusMap {
		statuses = append(statuses, status)
	}

	stats, err := GenerateDiffStatsAgainstHEADContext(context.Background(), repoDir, statuses)
	if err != nil {
		t.Fatalf("GenerateDiffStatsAgainstHEADContext returned error: %v", err)
	}

	if stat := stats["README.md"]; stat.Additions != 1 || stat.Deletions != 0 {
		t.Fatalf("unexpected README.md diff stat: %#v", stat)
	}
	if stat := stats["docs.txt"]; stat.Additions != 0 || stat.Deletions != 0 {
		t.Fatalf("unexpected docs.txt diff stat: %#v", stat)
	}
	if stat := stats["scratch.txt"]; stat.Additions != 1 || stat.Deletions != 0 {
		t.Fatalf("unexpected scratch.txt diff stat: %#v", stat)
	}
}

func TestParseGitDiffStatsZOutputHandlesRenames(t *testing.T) {
	output := []byte("1\t0\tREADME.md\x000\t0\t\x00notes.txt\x00docs.txt\x00")

	stats := parseGitDiffStatsZOutput(output)
	if stat := stats["README.md"]; stat.Additions != 1 || stat.Deletions != 0 {
		t.Fatalf("unexpected README.md diff stat: %#v", stat)
	}
	if stat := stats["docs.txt"]; stat.Additions != 0 || stat.Deletions != 0 {
		t.Fatalf("unexpected docs.txt diff stat: %#v", stat)
	}
}

func initTestRepoWithTrackedFile(t *testing.T, name, content string) string {
	t.Helper()

	dir := initTestRepo(t)
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	runGit(t, dir, "add", name)
	runGit(t, dir, "commit", "-m", "add "+name)
	return dir
}
