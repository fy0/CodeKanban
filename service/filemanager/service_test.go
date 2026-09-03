package filemanager

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"code-kanban/model"
	"code-kanban/utils"

	goGit "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
)

func TestDetectModExtensionAsText(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"go.mod", "module.MOD"} {
		mimeType := detectMimeFromName(name)
		if mimeType != "text/plain" {
			t.Fatalf("detectMimeFromName(%q) = %q, want text/plain", name, mimeType)
		}
		if got := detectPreviewKind(name, mimeType); got != PreviewKindText {
			t.Fatalf("detectPreviewKind(%q, %q) = %q, want %q", name, mimeType, got, PreviewKindText)
		}
	}
}

func TestEnsureProtectedPathRejectsGitSegments(t *testing.T) {
	t.Parallel()

	cases := []string{
		".git",
		".git/config",
		"docs/.git/hooks",
	}
	for _, path := range cases {
		if err := ensureProtectedPath(path); err == nil {
			t.Fatalf("expected protected path error for %q", path)
		}
	}

	if err := ensureProtectedPath("docs/guide.md"); err != nil {
		t.Fatalf("unexpected error for normal path: %v", err)
	}
}

func TestResolveAbsolutePathRejectsScopeEscape(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if _, _, err := resolveAbsolutePath(root, "../outside"); err == nil {
		t.Fatal("expected scope escape to fail")
	}

	normalized, absPath, err := resolveAbsolutePath(root, "docs/readme.md")
	if err != nil {
		t.Fatalf("resolveAbsolutePath returned error: %v", err)
	}
	if normalized != "docs/readme.md" {
		t.Fatalf("normalized path = %q, want %q", normalized, "docs/readme.md")
	}
	if !strings.HasPrefix(absPath, root) {
		t.Fatalf("resolved path %q does not stay under root %q", absPath, root)
	}
}

func TestAppendUploadChunkPersistsOffsetAndData(t *testing.T) {
	service, err := NewService(Config{
		DataDir:         t.TempDir(),
		UploadChunkSize: 8,
	}, nil)
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	partPath := filepath.Join(service.uploadsDir, "up1.part")
	if err := os.WriteFile(partPath, nil, 0o644); err != nil {
		t.Fatalf("failed to create part file: %v", err)
	}

	now := time.Now()
	meta := uploadMeta{
		ID:        "up1",
		ProjectID: "project-1",
		ScopeID:   "project:project-1",
		FileName:  "demo.txt",
		Size:      11,
		Offset:    0,
		ChunkSize: 8,
		PartPath:  partPath,
		CreatedAt: now,
		UpdatedAt: now,
		ExpiresAt: now.Add(time.Hour),
	}
	if err := service.writeJSONFile(service.uploadMetaPath(meta.ID), meta); err != nil {
		t.Fatalf("failed to persist upload meta: %v", err)
	}

	session, err := service.AppendUploadChunk(meta.ProjectID, meta.ID, 0, 5, strings.NewReader("hello"))
	if err != nil {
		t.Fatalf("AppendUploadChunk returned error: %v", err)
	}
	if session.Offset != 5 {
		t.Fatalf("offset = %d, want %d", session.Offset, 5)
	}

	data, err := os.ReadFile(partPath)
	if err != nil {
		t.Fatalf("failed to read part file: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("part file content = %q, want %q", data, "hello")
	}

	if _, err := service.AppendUploadChunk(meta.ProjectID, meta.ID, 0, 1, strings.NewReader("x")); err == nil {
		t.Fatal("expected offset mismatch error")
	}
}

func TestListScopesPrefersMainWorktreeOverProjectScopeWhenPathsMatch(t *testing.T) {
	cleanup := initFileManagerTestDB(t)
	defer cleanup()

	projectDir := t.TempDir()
	projectService := &model.ProjectService{}
	project, err := projectService.CreateProject(context.Background(), model.CreateProjectParams{
		Name: "Plain Folder Project",
		Path: projectDir,
	})
	if err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}

	service, err := NewService(Config{
		DataDir: t.TempDir(),
	}, nil)
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	scopes, err := service.ListScopes(context.Background(), project.Id)
	if err != nil {
		t.Fatalf("ListScopes returned error: %v", err)
	}
	if len(scopes) != 1 {
		t.Fatalf("expected exactly one scope, got %d", len(scopes))
	}
	if scopes[0].Kind != ScopeKindWorktree {
		t.Fatalf("expected main worktree scope to be retained, got %s", scopes[0].Kind)
	}
	if filepath.Clean(scopes[0].RootPath) != filepath.Clean(projectDir) {
		t.Fatalf("scope root = %q, want %q", scopes[0].RootPath, filepath.Clean(projectDir))
	}
}

func TestListIncludesGitStatus(t *testing.T) {
	cleanup := initFileManagerTestDB(t)
	defer cleanup()

	repoDir := initFileManagerGitRepo(t)
	service, err := NewService(Config{
		DataDir: t.TempDir(),
	}, nil)
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	projectID := seedFileManagerProjectScope(t, repoDir)

	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("# Repo\nupdated\n"), 0o644); err != nil {
		t.Fatalf("rewrite README: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "docs", "draft.md"), []byte("draft\n"), 0o644); err != nil {
		t.Fatalf("write draft.md: %v", err)
	}

	result, err := service.List(context.Background(), projectID, "", "")
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}

	var readmeStatus *GitStatus
	var docsStatus *GitStatus
	for _, entry := range result.Entries {
		switch entry.Name {
		case "README.md":
			readmeStatus = entry.GitStatus
		case "docs":
			docsStatus = entry.GitStatus
		}
	}

	if readmeStatus == nil || readmeStatus.Kind != GitStatusKindModified {
		t.Fatalf("README.md git status = %#v", readmeStatus)
	}
	if docsStatus != nil {
		t.Fatalf("directory status must not scan descendants: %#v", docsStatus)
	}
}

func TestSearchFindsEntriesInCurrentDirectorySubtree(t *testing.T) {
	cleanup := initFileManagerTestDB(t)
	defer cleanup()

	repoDir := initFileManagerGitRepo(t)
	service, err := NewService(Config{
		DataDir: t.TempDir(),
	}, nil)
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}
	projectID := seedFileManagerProjectScope(t, repoDir)

	if err := os.MkdirAll(filepath.Join(repoDir, "docs", "nested"), 0o755); err != nil {
		t.Fatalf("mkdir docs/nested: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repoDir, "examples"), 0o755); err != nil {
		t.Fatalf("mkdir examples: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "docs", "nested", "deep-guide.md"), []byte("deep\n"), 0o644); err != nil {
		t.Fatalf("write deep-guide.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "examples", "outside-guide.md"), []byte("outside\n"), 0o644); err != nil {
		t.Fatalf("write outside-guide.md: %v", err)
	}

	result, err := service.Search(context.Background(), projectID, "", "docs", "guide", false)
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}

	paths := searchResultPaths(result.Entries)
	if !containsString(paths, "docs/guide.md") {
		t.Fatalf("expected docs/guide.md in results, got %#v", paths)
	}
	if !containsString(paths, "docs/nested/deep-guide.md") {
		t.Fatalf("expected nested match in results, got %#v", paths)
	}
	if containsString(paths, "examples/outside-guide.md") {
		t.Fatalf("search should stay under current subtree, got %#v", paths)
	}
}

func TestSearchSupportsWildcardAndRegex(t *testing.T) {
	cleanup := initFileManagerTestDB(t)
	defer cleanup()

	repoDir := initFileManagerGitRepo(t)
	service, err := NewService(Config{
		DataDir: t.TempDir(),
	}, nil)
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}
	projectID := seedFileManagerProjectScope(t, repoDir)

	if err := os.MkdirAll(filepath.Join(repoDir, "docs", "api"), 0o755); err != nil {
		t.Fatalf("mkdir docs/api: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "docs", "api", "index.ts"), []byte("export {}\n"), 0o644); err != nil {
		t.Fatalf("write index.ts: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "docs", "api", "index.test.ts"), []byte("test\n"), 0o644); err != nil {
		t.Fatalf("write index.test.ts: %v", err)
	}

	wildcardResult, err := service.Search(context.Background(), projectID, "", "", "docs/*/index.?s", false)
	if err != nil {
		t.Fatalf("wildcard Search returned error: %v", err)
	}
	wildcardPaths := searchResultPaths(wildcardResult.Entries)
	if !containsString(wildcardPaths, "docs/api/index.ts") {
		t.Fatalf("expected wildcard match, got %#v", wildcardPaths)
	}
	if containsString(wildcardPaths, "docs/api/index.test.ts") {
		t.Fatalf("wildcard should not match extra segment, got %#v", wildcardPaths)
	}

	regexResult, err := service.Search(context.Background(), projectID, "", "", `^docs/api/index\.test\.ts$`, true)
	if err != nil {
		t.Fatalf("regex Search returned error: %v", err)
	}
	regexPaths := searchResultPaths(regexResult.Entries)
	if len(regexPaths) != 1 || regexPaths[0] != "docs/api/index.test.ts" {
		t.Fatalf("unexpected regex results: %#v", regexPaths)
	}

	if _, err := service.Search(context.Background(), projectID, "", "", "[", true); err == nil {
		t.Fatal("expected invalid regex error")
	} else if !errors.Is(err, ErrInvalidSearchPattern()) {
		t.Fatalf("invalid regex error = %v", err)
	}
}

func TestSearchSkipsIgnoredDirectoriesAndTruncates(t *testing.T) {
	cleanup := initFileManagerTestDB(t)
	defer cleanup()

	repoDir := initFileManagerGitRepo(t)
	service, err := NewService(Config{
		DataDir: t.TempDir(),
	}, nil)
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}
	projectID := seedFileManagerProjectScope(t, repoDir)

	for index := 0; index < defaultSearchMaxEntries+1; index++ {
		fileName := filepath.Join(repoDir, fmt.Sprintf("match-%03d.txt", index))
		if err := os.WriteFile(fileName, []byte("match\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", fileName, err)
		}
	}
	if err := os.WriteFile(filepath.Join(repoDir, ".git", "match-hidden.txt"), []byte("hidden\n"), 0o644); err != nil {
		t.Fatalf("write .git match: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repoDir, "node_modules", "match-package"), 0o755); err != nil {
		t.Fatalf("mkdir node_modules match: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "node_modules", "match-package", "match-file.txt"), []byte("hidden\n"), 0o644); err != nil {
		t.Fatalf("write node_modules match: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repoDir, ".venv", "match-env"), 0o755); err != nil {
		t.Fatalf("mkdir .venv match: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, ".venv", "match-env", "match-file.txt"), []byte("hidden\n"), 0o644); err != nil {
		t.Fatalf("write .venv match: %v", err)
	}

	result, err := service.Search(context.Background(), projectID, "", "", "match", false)
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if !result.Truncated {
		t.Fatalf("expected truncated result")
	}
	if len(result.Entries) != defaultSearchMaxEntries {
		t.Fatalf("len(entries) = %d, want %d", len(result.Entries), defaultSearchMaxEntries)
	}
	for _, entry := range result.Entries {
		if strings.HasPrefix(entry.Path, ".git/") ||
			strings.HasPrefix(entry.Path, "node_modules/") ||
			strings.HasPrefix(entry.Path, ".venv/") {
			t.Fatalf("search should skip ignored directories: %#v", entry)
		}
	}
}

func TestChangesSummarySkipsUntrackedInFastCountAndCompletesStats(t *testing.T) {
	cleanup := initFileManagerTestDB(t)
	defer cleanup()

	repoDir := initFileManagerGitRepo(t)
	service, err := NewService(Config{
		DataDir: t.TempDir(),
	}, nil)
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	projectID := seedFileManagerProjectScope(t, repoDir)

	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("# Repo\nupdated\n"), 0o644); err != nil {
		t.Fatalf("rewrite README.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "scratch.txt"), []byte("draft\n"), 0o644); err != nil {
		t.Fatalf("write scratch.txt: %v", err)
	}

	fastSummary, err := service.ChangesSummary(context.Background(), projectID, "", ChangesSummaryOptions{
		IncludeUntracked: false,
		WithStats:        false,
	})
	if err != nil {
		t.Fatalf("ChangesSummary fast phase returned error: %v", err)
	}
	if fastSummary.Count != 1 {
		t.Fatalf("fast summary count = %d, want %d", fastSummary.Count, 1)
	}
	if fastSummary.ChangeToken == "" {
		t.Fatalf("fast summary should include a change token: %#v", fastSummary)
	}
	if fastSummary.Additions != nil || fastSummary.Deletions != nil {
		t.Fatalf("fast summary should not include stats: %#v", fastSummary)
	}
	if fastSummary.StatsComplete || fastSummary.StatsTimedOut {
		t.Fatalf("unexpected fast summary flags: %#v", fastSummary)
	}

	statsSummary, err := service.ChangesSummary(context.Background(), projectID, "", ChangesSummaryOptions{
		IncludeUntracked: false,
		WithStats:        true,
		StatsTimeout:     5 * time.Second,
	})
	if err != nil {
		t.Fatalf("ChangesSummary stats phase returned error: %v", err)
	}
	if statsSummary.Count != 1 {
		t.Fatalf("stats summary count = %d, want %d", statsSummary.Count, 1)
	}
	if statsSummary.ChangeToken != fastSummary.ChangeToken {
		t.Fatalf("summary token changed without repository changes: fast=%q stats=%q", fastSummary.ChangeToken, statsSummary.ChangeToken)
	}
	if !statsSummary.StatsComplete || statsSummary.StatsTimedOut {
		t.Fatalf("unexpected stats summary flags: %#v", statsSummary)
	}
	if statsSummary.Additions == nil || *statsSummary.Additions != 1 {
		t.Fatalf("stats summary additions = %#v, want %d", statsSummary.Additions, 1)
	}
	if statsSummary.Deletions == nil || *statsSummary.Deletions != 0 {
		t.Fatalf("stats summary deletions = %#v, want %d", statsSummary.Deletions, 0)
	}
}

func TestRenameInvalidatesChangesSummaryCache(t *testing.T) {
	cleanup := initFileManagerTestDB(t)
	defer cleanup()

	repoDir := initFileManagerGitRepo(t)
	service, err := NewService(Config{DataDir: t.TempDir()}, nil)
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}
	projectID := seedFileManagerProjectScope(t, repoDir)

	clean, err := service.ChangesSummary(context.Background(), projectID, "", ChangesSummaryOptions{
		IncludeUntracked: true,
	})
	if err != nil {
		t.Fatalf("warm ChangesSummary returned error: %v", err)
	}
	if clean.Count != 0 {
		t.Fatalf("initial change count = %d, want 0", clean.Count)
	}

	if _, err := service.Rename(context.Background(), projectID, "", "README.md", "RENAMED.md"); err != nil {
		t.Fatalf("Rename returned error: %v", err)
	}
	afterRename, err := service.ChangesSummary(context.Background(), projectID, "", ChangesSummaryOptions{
		IncludeUntracked: true,
	})
	if err != nil {
		t.Fatalf("ChangesSummary after rename returned error: %v", err)
	}
	if afterRename.Count == 0 {
		t.Fatal("rename reused the cached clean Git summary")
	}
}

func TestStatusOnlyChangesRequestsDoNotRunDiffStats(t *testing.T) {
	cleanup := initFileManagerTestDB(t)
	defer cleanup()

	repoDir := initFileManagerGitRepo(t)

	service, err := NewService(Config{
		DataDir: t.TempDir(),
	}, nil)
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}
	projectID := seedFileManagerProjectScope(t, repoDir)

	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("# Repo\nupdated\n"), 0o644); err != nil {
		t.Fatalf("rewrite README.md: %v", err)
	}

	changes, err := service.ListChanges(context.Background(), projectID, "", ListChangesOptions{
		IncludeUntracked: boolRef(false),
		WithStats:        boolRef(false),
	})
	if err != nil {
		t.Fatalf("status-only ListChanges returned error: %v", err)
	}
	if changes.ChangeToken == "" || len(changes.Entries) != 1 || changes.StatsComplete {
		t.Fatalf("unexpected status-only changes result: %#v", changes)
	}

	summary, err := service.ChangesSummary(context.Background(), projectID, "", ChangesSummaryOptions{
		IncludeUntracked: false,
		WithStats:        false,
	})
	if err != nil {
		t.Fatalf("status-only ChangesSummary returned error: %v", err)
	}
	if summary.ChangeToken == "" || summary.Count != 1 || summary.StatsComplete {
		t.Fatalf("unexpected status-only summary result: %#v", summary)
	}
}

func TestDiffReturnsUnifiedPatchAndStatusReasons(t *testing.T) {
	cleanup := initFileManagerTestDB(t)
	defer cleanup()

	repoDir := initFileManagerGitRepo(t)
	service, err := NewService(Config{
		DataDir: t.TempDir(),
	}, nil)
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	projectID := seedFileManagerProjectScope(t, repoDir)

	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("# Repo\nupdated\n"), 0o644); err != nil {
		t.Fatalf("rewrite README: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "scratch.txt"), []byte("draft\n"), 0o644); err != nil {
		t.Fatalf("write scratch.txt: %v", err)
	}
	if err := os.Remove(filepath.Join(repoDir, "docs", "guide.md")); err != nil {
		t.Fatalf("remove docs/guide.md: %v", err)
	}

	diffResult, err := service.Diff(context.Background(), projectID, "", "README.md")
	if err != nil {
		t.Fatalf("Diff returned error: %v", err)
	}
	if !diffResult.Available {
		t.Fatalf("expected README.md diff to be available: %#v", diffResult)
	}
	if diffResult.Status == nil || diffResult.Status.Kind != GitStatusKindModified {
		t.Fatalf("unexpected README.md status: %#v", diffResult.Status)
	}
	if !strings.Contains(diffResult.DiffText, "+updated") {
		t.Fatalf("diff text missing updated line: %s", diffResult.DiffText)
	}

	untrackedResult, err := service.Diff(context.Background(), projectID, "", "scratch.txt")
	if err != nil {
		t.Fatalf("Diff returned error for untracked file: %v", err)
	}
	if untrackedResult.Available {
		t.Fatalf("expected untracked diff to be unavailable: %#v", untrackedResult)
	}
	if untrackedResult.Reason != "untracked" {
		t.Fatalf("unexpected untracked reason: %#v", untrackedResult)
	}

	deletedResult, err := service.Diff(context.Background(), projectID, "", "docs/guide.md")
	if err != nil {
		t.Fatalf("Diff returned error for deleted file: %v", err)
	}
	if !deletedResult.Available {
		t.Fatalf("expected deleted diff to be available: %#v", deletedResult)
	}
	if deletedResult.Status == nil || deletedResult.Status.Kind != GitStatusKindDeleted {
		t.Fatalf("unexpected deleted status: %#v", deletedResult.Status)
	}
	if !strings.Contains(deletedResult.DiffText, "--- a/docs/guide.md") {
		t.Fatalf("deleted diff text missing old path: %s", deletedResult.DiffText)
	}
}

func TestListChangesReturnsDeletedAndModifiedEntries(t *testing.T) {
	cleanup := initFileManagerTestDB(t)
	defer cleanup()

	repoDir := initFileManagerGitRepo(t)
	service, err := NewService(Config{
		DataDir: t.TempDir(),
	}, nil)
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	projectID := seedFileManagerProjectScope(t, repoDir)

	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("# Repo\nupdated\n"), 0o644); err != nil {
		t.Fatalf("rewrite README: %v", err)
	}
	if err := os.Remove(filepath.Join(repoDir, "docs", "guide.md")); err != nil {
		t.Fatalf("remove docs/guide.md: %v", err)
	}

	result, err := service.ListChanges(context.Background(), projectID, "", ListChangesOptions{})
	if err != nil {
		t.Fatalf("ListChanges returned error: %v", err)
	}

	statusByPath := make(map[string]GitStatusKind, len(result.Entries))
	existsByPath := make(map[string]bool, len(result.Entries))
	additionsByPath := make(map[string]int64, len(result.Entries))
	deletionsByPath := make(map[string]int64, len(result.Entries))
	for _, entry := range result.Entries {
		statusByPath[entry.Path] = entry.Status.Kind
		existsByPath[entry.Path] = entry.Exists
		additionsByPath[entry.Path] = entry.Additions
		deletionsByPath[entry.Path] = entry.Deletions
	}

	if statusByPath["README.md"] != GitStatusKindModified {
		t.Fatalf("README.md change status = %q", statusByPath["README.md"])
	}
	if statusByPath["docs/guide.md"] != GitStatusKindDeleted {
		t.Fatalf("docs/guide.md change status = %q", statusByPath["docs/guide.md"])
	}
	if existsByPath["docs/guide.md"] {
		t.Fatalf("expected deleted change entry to have exists=false")
	}
	if additionsByPath["README.md"] != 1 || deletionsByPath["README.md"] != 0 {
		t.Fatalf("unexpected README.md diff stat: +%d -%d", additionsByPath["README.md"], deletionsByPath["README.md"])
	}
	if additionsByPath["docs/guide.md"] != 0 || deletionsByPath["docs/guide.md"] != 1 {
		t.Fatalf("unexpected docs/guide.md diff stat: +%d -%d", additionsByPath["docs/guide.md"], deletionsByPath["docs/guide.md"])
	}
}

func TestListChangesCanSkipUntracked(t *testing.T) {
	cleanup := initFileManagerTestDB(t)
	defer cleanup()

	repoDir := initFileManagerGitRepo(t)
	service, err := NewService(Config{
		DataDir: t.TempDir(),
	}, nil)
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	projectID := seedFileManagerProjectScope(t, repoDir)

	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("# Repo\nupdated\n"), 0o644); err != nil {
		t.Fatalf("rewrite README.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "scratch.txt"), []byte("draft\n"), 0o644); err != nil {
		t.Fatalf("write scratch.txt: %v", err)
	}

	result, err := service.ListChanges(context.Background(), projectID, "", ListChangesOptions{
		IncludeUntracked: boolRef(false),
		WithStats:        boolRef(false),
	})
	if err != nil {
		t.Fatalf("ListChanges returned error: %v", err)
	}
	if result.UntrackedIncluded {
		t.Fatalf("expected untrackedIncluded=false: %#v", result)
	}
	for _, entry := range result.Entries {
		if entry.Status.Kind == GitStatusKindUntracked {
			t.Fatalf("untracked entry should be excluded: %#v", entry)
		}
	}
}

func TestListChangesMarksTruncatedWhenEntryLimitExceeded(t *testing.T) {
	cleanup := initFileManagerTestDB(t)
	defer cleanup()

	repoDir := initFileManagerGitRepo(t)
	service, err := NewService(Config{
		DataDir: t.TempDir(),
	}, nil)
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	projectID := seedFileManagerProjectScope(t, repoDir)

	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("# Repo\nupdated\n"), 0o644); err != nil {
		t.Fatalf("rewrite README.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "scratch.txt"), []byte("draft\n"), 0o644); err != nil {
		t.Fatalf("write scratch.txt: %v", err)
	}

	result, err := service.ListChanges(context.Background(), projectID, "", ListChangesOptions{
		WithStats:  boolRef(false),
		MaxEntries: 1,
	})
	if err != nil {
		t.Fatalf("ListChanges returned error: %v", err)
	}
	if !result.Truncated {
		t.Fatalf("expected truncated result: %#v", result)
	}
	if result.WarningReason != changesWarningReasonEntryLimitExceeded {
		t.Fatalf("unexpected warning reason: %#v", result.WarningReason)
	}
	if len(result.Entries) != 1 {
		t.Fatalf("expected a single retained entry, got %d", len(result.Entries))
	}
}

func TestListChangesUntrackedStatsDoNotShellOutPerFile(t *testing.T) {
	cleanup := initFileManagerTestDB(t)
	defer cleanup()

	repoDir := initFileManagerGitRepo(t)

	service, err := NewService(Config{
		DataDir: t.TempDir(),
	}, nil)
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	projectID := seedFileManagerProjectScope(t, repoDir)

	if err := os.WriteFile(filepath.Join(repoDir, "scratch-a.txt"), []byte("alpha\nbeta\n"), 0o644); err != nil {
		t.Fatalf("write scratch-a.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "scratch-b.txt"), []byte("gamma"), 0o644); err != nil {
		t.Fatalf("write scratch-b.txt: %v", err)
	}

	result, err := service.ListChanges(context.Background(), projectID, "", ListChangesOptions{
		IncludeUntracked: boolRef(true),
		WithStats:        boolRef(true),
	})
	if err != nil {
		t.Fatalf("ListChanges returned error: %v", err)
	}
	if !result.StatsComplete || result.StatsTimedOut {
		t.Fatalf("expected complete stats result: %#v", result)
	}

	statsByPath := make(map[string]struct {
		additions int64
		deletions int64
	}, len(result.Entries))
	for _, entry := range result.Entries {
		if !entry.StatsAvailable {
			t.Fatalf("expected stats for entry: %#v", entry)
		}
		statsByPath[entry.Path] = struct {
			additions int64
			deletions int64
		}{
			additions: entry.Additions,
			deletions: entry.Deletions,
		}
	}
	if stat := statsByPath["scratch-a.txt"]; stat.additions != 2 || stat.deletions != 0 {
		t.Fatalf("unexpected scratch-a.txt diff stat: %#v", stat)
	}
	if stat := statsByPath["scratch-b.txt"]; stat.additions != 1 || stat.deletions != 0 {
		t.Fatalf("unexpected scratch-b.txt diff stat: %#v", stat)
	}
}

func initFileManagerTestDB(t *testing.T) func() {
	t.Helper()

	dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
	if err := model.InitWithDSN(dsn, 0, true); err != nil {
		t.Fatalf("InitWithDSN: %v", err)
	}

	return func() {
		model.DBClose()
	}
}

func initFileManagerGitRepo(t *testing.T) string {
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
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Repo\n"), 0o644); err != nil {
		t.Fatalf("write README.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docs", "guide.md"), []byte("guide\n"), 0o644); err != nil {
		t.Fatalf("write docs/guide.md: %v", err)
	}
	worktree, err := repository.Worktree()
	if err != nil {
		t.Fatalf("open worktree: %v", err)
	}
	if err := worktree.AddWithOptions(&goGit.AddOptions{All: true}); err != nil {
		t.Fatalf("stage repository files: %v", err)
	}
	if _, err := worktree.Commit("initial commit", &goGit.CommitOptions{Author: &object.Signature{
		Name: "Test User", Email: "test@example.com", When: time.Now(),
	}}); err != nil {
		t.Fatalf("commit repository files: %v", err)
	}
	_ = repository.Close()
	return dir
}

func seedFileManagerProjectScope(t *testing.T, repoDir string) string {
	t.Helper()

	q, err := model.ResolveQueries(nil)
	if err != nil {
		t.Fatalf("ResolveQueries returned error: %v", err)
	}

	now := time.Now()
	projectID := utils.NewID()
	worktreeID := utils.NewID()
	worktreeBasePath := filepath.Join(repoDir, ".worktrees")
	defaultBranch := "main"
	project, err := q.ProjectCreate(context.Background(), &model.ProjectCreateParams{
		Id:               projectID,
		CreatedAt:        now,
		UpdatedAt:        now,
		Name:             "Git Project",
		Path:             repoDir,
		DefaultBranch:    defaultBranch,
		WorktreeBasePath: &worktreeBasePath,
		HidePath:         false,
	})
	if err != nil {
		t.Fatalf("ProjectCreate returned error: %v", err)
	}

	headCommit := "HEAD"
	if _, err := q.WorktreeCreate(context.Background(), &model.WorktreeCreateParams{
		Id:         worktreeID,
		CreatedAt:  now,
		UpdatedAt:  now,
		ProjectId:  project.Id,
		BranchName: defaultBranch,
		Path:       repoDir,
		IsMain:     true,
		IsBare:     false,
		HeadCommit: &headCommit,
	}); err != nil {
		t.Fatalf("WorktreeCreate returned error: %v", err)
	}

	return project.Id
}

func searchResultPaths(entries []Entry) []string {
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		paths = append(paths, entry.Path)
	}
	return paths
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func boolRef(value bool) *bool {
	return &value
}
