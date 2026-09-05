package git

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	goGit "github.com/go-git/go-git/v6"
	gitconfig "github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/filemode"
	objectformat "github.com/go-git/go-git/v6/plumbing/format/config"
	formatdiff "github.com/go-git/go-git/v6/plumbing/format/diff"
	gitdiff "github.com/go-git/go-git/v6/utils/diff"
	dmp "github.com/sergi/go-diff/diffmatchpatch"
)

type FileChangeKind string

const (
	FileChangeKindModified   FileChangeKind = "modified"
	FileChangeKindAdded      FileChangeKind = "added"
	FileChangeKindDeleted    FileChangeKind = "deleted"
	FileChangeKindRenamed    FileChangeKind = "renamed"
	FileChangeKindUntracked  FileChangeKind = "untracked"
	FileChangeKindConflicted FileChangeKind = "conflicted"
	FileChangeKindDirty      FileChangeKind = "dirty"
)

type FileStatus struct {
	Path         string
	Kind         FileChangeKind
	PreviousPath string
}

type DiffStat struct {
	Additions int64
	Deletions int64
}

type FileStatusResult struct {
	Statuses    map[string]FileStatus
	Truncated   bool
	TotalCount  int
	ChangeToken string
}

const maxDiffFileBytes int64 = 32 << 20

var errDiffPathNotRegular = errors.New("diff path is not a regular file")

func ListFileStatuses(path string) (map[string]FileStatus, error) {
	return ListFileStatusesContext(context.Background(), path, true)
}

func ListFileStatusesContext(ctx context.Context, path string, includeUntracked bool) (map[string]FileStatus, error) {
	result, err := ListFileStatusesLimitedContext(ctx, path, includeUntracked, 0)
	return result.Statuses, err
}

func ListFileStatusesLimitedContext(ctx context.Context, path string, includeUntracked bool, maxEntries int) (FileStatusResult, error) {
	return listFileStatusesLimitedContext(ctx, path, includeUntracked, maxEntries, false)
}

// ListFileStatusesFastContext skips content hashing and exact rename detection.
// It is intended for polling badges where a cheap invalidation token matters
// more than a fully classified change list.
func ListFileStatusesFastContext(ctx context.Context, path string, includeUntracked bool, maxEntries int) (FileStatusResult, error) {
	return listFileStatusesLimitedContext(ctx, path, includeUntracked, maxEntries, true)
}

// ListFileStatusesForPathsContext checks only the supplied repository-relative paths.
// It is intended for file browser badges and must not walk unrelated directories.
func ListFileStatusesForPathsContext(ctx context.Context, root string, paths []string, includeUntracked bool) (map[string]FileStatus, error) {
	if len(paths) == 0 {
		return map[string]FileStatus{}, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if systemGitRepositoryAvailable(ctx, root) {
		return listFileStatusesSystemPaths(ctx, root, paths, includeUntracked)
	}
	// The built-in engine has no pathspec primitive. Do not fall back to a
	// repository-wide scan here; callers can still browse immediately and the
	// dedicated Changes view provides full status details.
	return map[string]FileStatus{}, nil
}

func listFileStatusesLimitedContext(ctx context.Context, path string, includeUntracked bool, maxEntries int, fast bool) (FileStatusResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	repo, err := DetectRepository(path)
	if err != nil {
		return FileStatusResult{}, err
	}
	defer repo.Close()
	engine, err := repo.requireEngine(path, OperationStatus)
	if err != nil {
		return FileStatusResult{}, err
	}
	if engine == EngineSystem {
		return listFileStatusesSystem(ctx, path, includeUntracked, maxEntries, fast)
	}
	if repo.repository == nil {
		return FileStatusResult{}, errors.New("built-in Git repository is not initialized")
	}

	repo.lock.RLock()
	defer repo.lock.RUnlock()
	if err := ctx.Err(); err != nil {
		return FileStatusResult{}, err
	}
	repository, err := repo.openWorktreeRepository(path)
	if err != nil {
		return FileStatusResult{}, err
	}
	defer repository.Close()
	worktree, err := repository.Worktree()
	if err != nil {
		return FileStatusResult{}, err
	}
	snapshot, err := worktree.StatusWithOptions(goGit.StatusOptions{Strategy: goGit.Preload})
	if err != nil {
		return FileStatusResult{}, mapGoGitError(OperationStatus, err)
	}
	conflicts, err := conflictPaths(repository)
	if err != nil {
		return FileStatusResult{}, err
	}

	statuses := make(map[string]FileStatus)
	for path := range conflicts {
		statuses[path] = FileStatus{Path: path, Kind: FileChangeKindConflicted}
	}
	for rawPath, rawStatus := range snapshot {
		if err := ctx.Err(); err != nil {
			return FileStatusResult{Statuses: statuses}, err
		}
		normalized := normalizeGitRelativePath(rawPath)
		if normalized == "" {
			continue
		}
		if _, conflicted := conflicts[normalized]; conflicted {
			continue
		}
		status, changed := classifyGoGitStatus(normalized, rawStatus, includeUntracked)
		if changed {
			statuses[normalized] = status
		}
	}
	if !fast {
		detectExactRenames(ctx, repository, path, statuses)
	}

	keys := make([]string, 0, len(statuses))
	for key := range statuses {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := FileStatusResult{
		Statuses:   make(map[string]FileStatus),
		TotalCount: len(keys),
	}
	limit := len(keys)
	if maxEntries > 0 && limit > maxEntries {
		limit = maxEntries
		result.Truncated = true
	}
	for _, key := range keys[:limit] {
		result.Statuses[key] = statuses[key]
	}
	result.ChangeToken = buildFileStatusChangeToken(repository, path, statuses)
	return result, nil
}

func classifyGoGitStatus(path string, status *goGit.FileStatus, includeUntracked bool) (FileStatus, bool) {
	result := FileStatus{Path: path}
	if status == nil {
		return result, false
	}
	if status.Staging == goGit.Untracked || status.Worktree == goGit.Untracked {
		if !includeUntracked {
			return result, false
		}
		result.Kind = FileChangeKindUntracked
		return result, true
	}
	if status.Staging == goGit.Renamed {
		result.Kind = FileChangeKindRenamed
		result.PreviousPath = normalizeGitRelativePath(status.Extra)
		return result, true
	}
	if status.Staging == goGit.Deleted || status.Worktree == goGit.Deleted {
		result.Kind = FileChangeKindDeleted
		return result, true
	}
	if status.Staging == goGit.Added || status.Worktree == goGit.Added {
		result.Kind = FileChangeKindAdded
		return result, true
	}
	if status.Staging != goGit.Unmodified || status.Worktree != goGit.Unmodified {
		result.Kind = FileChangeKindModified
		return result, true
	}
	return result, false
}

func GenerateUnifiedDiffAgainstHEAD(path, relativePath, previousPath string) (string, error) {
	repo, err := DetectRepository(path)
	if err != nil {
		return "", err
	}
	defer repo.Close()
	engine, err := repo.requireEngine(path, OperationDiff)
	if err != nil {
		return "", err
	}
	if engine == EngineSystem {
		return generateUnifiedDiffSystem(context.Background(), path, relativePath, previousPath)
	}
	if repo.repository == nil {
		return "", errors.New("built-in Git repository is not initialized")
	}
	repo.lock.RLock()
	defer repo.lock.RUnlock()
	repository, err := repo.openWorktreeRepository(path)
	if err != nil {
		return "", err
	}
	defer repository.Close()

	currentPath := normalizeGitRelativePath(relativePath)
	if currentPath == "" {
		return "", errors.New("path is required")
	}
	oldPath := normalizeGitRelativePath(previousPath)
	if oldPath == "" {
		oldPath = currentPath
	}
	from, fromErr := loadHeadDiffFile(repository, oldPath)
	if fromErr != nil {
		return "", fromErr
	}
	to, toErr := loadWorktreeDiffFile(path, currentPath)
	if toErr != nil && !errors.Is(toErr, os.ErrNotExist) {
		return "", toErr
	}
	if errors.Is(toErr, os.ErrNotExist) {
		to = nil
	}
	if from == nil && to == nil {
		return "", nil
	}
	if normalizeDiffLineEndings(repository) {
		if from != nil {
			from.content = []byte(normalizeCRLF(string(from.content)))
		}
		if to != nil {
			to.content = []byte(normalizeCRLF(string(to.content)))
		}
	}
	patch := newContentPatch(from, to)
	buffer := new(bytes.Buffer)
	if err := formatdiff.NewUnifiedEncoder(buffer, formatdiff.DefaultContextLines).Encode(patch); err != nil {
		return "", err
	}
	return buffer.String(), nil
}

func GenerateDiffStatAgainstHEAD(path string, status FileStatus) (DiffStat, error) {
	return GenerateDiffStatAgainstHEADContext(context.Background(), path, status)
}

func GenerateDiffStatsAgainstHEADContext(ctx context.Context, path string, statuses []FileStatus) (map[string]DiffStat, error) {
	result := make(map[string]DiffStat, len(statuses))
	if len(statuses) == 0 {
		return result, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	repo, err := DetectRepository(path)
	if err != nil {
		return nil, err
	}
	defer repo.Close()
	engine, err := repo.requireEngine(path, OperationDiff)
	if err != nil {
		return nil, err
	}
	if engine == EngineSystem {
		return generateDiffStatsSystem(ctx, path, statuses)
	}
	if repo.repository == nil {
		return nil, errors.New("built-in Git repository is not initialized")
	}
	repo.lock.RLock()
	defer repo.lock.RUnlock()
	repository, err := repo.openWorktreeRepository(path)
	if err != nil {
		return nil, err
	}
	defer repository.Close()

	for _, status := range statuses {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		stat, statErr := generateDiffStat(repository, path, status)
		if statErr != nil {
			if errors.Is(statErr, errDiffPathNotRegular) {
				result[normalizeGitRelativePath(status.Path)] = DiffStat{}
				continue
			}
			return result, statErr
		}
		result[normalizeGitRelativePath(status.Path)] = stat
	}
	return result, nil
}

func GenerateDiffStatAgainstHEADContext(ctx context.Context, path string, status FileStatus) (DiffStat, error) {
	stats, err := GenerateDiffStatsAgainstHEADContext(ctx, path, []FileStatus{status})
	if err != nil {
		return DiffStat{}, err
	}
	return stats[normalizeGitRelativePath(status.Path)], nil
}

func generateDiffStat(repository *goGit.Repository, root string, status FileStatus) (DiffStat, error) {
	path := normalizeGitRelativePath(status.Path)
	if path == "" {
		return DiffStat{}, errors.New("path is required")
	}
	oldPath := normalizeGitRelativePath(status.PreviousPath)
	if oldPath == "" {
		oldPath = path
	}
	var from *contentDiffFile
	var to *contentDiffFile
	var err error
	if status.Kind != FileChangeKindUntracked && status.Kind != FileChangeKindAdded {
		from, err = loadHeadDiffFile(repository, oldPath)
		if err != nil {
			return DiffStat{}, err
		}
	}
	if status.Kind != FileChangeKindDeleted {
		to, err = loadWorktreeDiffFile(root, path)
		if err != nil {
			return DiffStat{}, err
		}
	}
	if (from != nil && from.binary) || (to != nil && to.binary) {
		return DiffStat{}, nil
	}
	fromContent := ""
	toContent := ""
	if from != nil {
		fromContent = string(from.content)
	}
	if to != nil {
		toContent = string(to.content)
	}
	if normalizeDiffLineEndings(repository) {
		fromContent = normalizeCRLF(fromContent)
		toContent = normalizeCRLF(toContent)
	}
	matcher := dmp.New()
	fromLines, toLines, _ := matcher.DiffLinesToRunes(fromContent, toContent)
	var stat DiffStat
	for _, item := range matcher.DiffMainRunes(fromLines, toLines, false) {
		switch item.Type {
		case dmp.DiffDelete:
			stat.Deletions += int64(utf8.RuneCountInString(item.Text))
		case dmp.DiffInsert:
			stat.Additions += int64(utf8.RuneCountInString(item.Text))
		}
	}
	return stat, nil
}

func normalizeDiffLineEndings(repository *goGit.Repository) bool {
	if repository == nil {
		return false
	}
	cfg, err := repository.ConfigScoped(gitconfig.SystemScope)
	if err != nil {
		cfg, err = repository.Config()
	}
	if err != nil || cfg == nil {
		return false
	}
	value := strings.ToLower(strings.TrimSpace(cfg.Core.AutoCRLF))
	return value == "true" || value == "input"
}

func normalizeCRLF(content string) string {
	return strings.ReplaceAll(content, "\r\n", "\n")
}

func loadHeadDiffFile(repository *goGit.Repository, path string) (*contentDiffFile, error) {
	head, err := repository.Head()
	if errors.Is(err, plumbing.ErrReferenceNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	commit, err := repository.CommitObject(head.Hash())
	if err != nil {
		return nil, err
	}
	tree, err := commit.Tree()
	if err != nil {
		return nil, err
	}
	file, err := tree.File(path)
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, plumbing.ErrObjectNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, nil
	}
	reader, err := file.Reader()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	content, err := readLimitedContent(reader, maxDiffFileBytes)
	if err != nil {
		return nil, err
	}
	return &contentDiffFile{
		path:    path,
		mode:    file.Mode,
		hash:    file.Hash,
		content: content,
		binary:  isBinaryContent(content),
	}, nil
}

func loadWorktreeDiffFile(root, path string) (*contentDiffFile, error) {
	absPath, err := safeWorktreePath(root, path)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(absPath)
	if err != nil {
		return nil, err
	}
	mode := filemode.Regular
	var content []byte
	if info.Mode()&os.ModeSymlink != 0 {
		mode = filemode.Symlink
		target, readErr := os.Readlink(absPath)
		if readErr != nil {
			return nil, readErr
		}
		content = []byte(target)
	} else {
		if !info.Mode().IsRegular() {
			return nil, errDiffPathNotRegular
		}
		if info.Size() > maxDiffFileBytes {
			return nil, fmt.Errorf("file exceeds diff limit of %d bytes", maxDiffFileBytes)
		}
		file, openErr := os.Open(absPath)
		if openErr != nil {
			return nil, openErr
		}
		content, err = readLimitedContent(file, maxDiffFileBytes)
		_ = file.Close()
		if err != nil {
			return nil, err
		}
		if info.Mode()&0o111 != 0 {
			mode = filemode.Executable
		}
	}
	return &contentDiffFile{
		path:    path,
		mode:    mode,
		hash:    hashBlob(content),
		content: content,
		binary:  isBinaryContent(content),
	}, nil
}

func detectExactRenames(ctx context.Context, repository *goGit.Repository, root string, statuses map[string]FileStatus) {
	deletedByDigest := make(map[string][]string)
	for path, status := range statuses {
		if status.Kind != FileChangeKindDeleted {
			continue
		}
		file, err := loadHeadDiffFile(repository, path)
		if err == nil && file != nil {
			digest := sha256.Sum256(file.content)
			deletedByDigest[hex.EncodeToString(digest[:])] = append(deletedByDigest[hex.EncodeToString(digest[:])], path)
		}
	}
	for path, status := range statuses {
		if ctx.Err() != nil || (status.Kind != FileChangeKindAdded && status.Kind != FileChangeKindUntracked) {
			continue
		}
		file, err := loadWorktreeDiffFile(root, path)
		if err != nil || file == nil {
			continue
		}
		digest := sha256.Sum256(file.content)
		key := hex.EncodeToString(digest[:])
		candidates := deletedByDigest[key]
		if len(candidates) == 0 {
			continue
		}
		previous := candidates[0]
		deletedByDigest[key] = candidates[1:]
		delete(statuses, previous)
		statuses[path] = FileStatus{Path: path, Kind: FileChangeKindRenamed, PreviousPath: previous}
	}
}

func buildFileStatusChangeToken(repository *goGit.Repository, rootPath string, statuses map[string]FileStatus) string {
	headID := ""
	if head, err := repository.Head(); err == nil {
		headID = head.Hash().String()
	}
	return buildStatusSnapshotToken(rootPath, headID, statuses)
}

func buildStatusSnapshotToken(rootPath, headID string, statuses map[string]FileStatus) string {
	hasher := sha256.New()
	writeChangeTokenField(hasher, "head:"+headID)
	if _, layout, err := discoverRepository(rootPath); err == nil {
		if info, statErr := os.Lstat(filepath.Join(layout.gitDir, "index")); statErr == nil {
			writeChangeTokenField(hasher, fmt.Sprintf("index:%d:%d:%d", info.Size(), info.ModTime().UnixNano(), info.Mode()))
		} else {
			writeChangeTokenField(hasher, "index:missing")
		}
	}
	paths := make(map[string]struct{}, len(statuses)*2)
	for _, status := range statuses {
		if path := normalizeGitRelativePath(status.Path); path != "" {
			paths[path] = struct{}{}
		}
		if path := normalizeGitRelativePath(status.PreviousPath); path != "" {
			paths[path] = struct{}{}
		}
	}
	keys := make([]string, 0, len(paths))
	for path := range paths {
		keys = append(keys, path)
	}
	sort.Strings(keys)
	for _, path := range keys {
		writeChangeTokenField(hasher, "path:"+path)
		if absPath, err := safeWorktreePath(rootPath, path); err == nil {
			if info, statErr := os.Lstat(absPath); statErr == nil {
				writeChangeTokenField(hasher, fmt.Sprintf("file:%d:%d:%d", info.Size(), info.ModTime().UnixNano(), info.Mode()))
				continue
			}
		}
		writeChangeTokenField(hasher, "file:missing")
	}
	return hex.EncodeToString(hasher.Sum(nil))
}

func writeChangeTokenField(writer io.Writer, value string) {
	_, _ = io.WriteString(writer, fmt.Sprintf("%d:", len(value)))
	_, _ = io.WriteString(writer, value)
}

func normalizeGitRelativePath(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" || strings.IndexByte(value, 0) >= 0 || strings.HasPrefix(value, "/") {
		return ""
	}
	cleaned := filepath.ToSlash(filepath.Clean(filepath.FromSlash(value)))
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") || filepath.IsAbs(filepath.FromSlash(cleaned)) {
		return ""
	}
	return cleaned
}

func safeWorktreePath(root, relative string) (string, error) {
	normalized := normalizeGitRelativePath(relative)
	if normalized == "" {
		return "", errors.New("invalid repository-relative path")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	absRoot = filepath.Clean(absRoot)
	target := filepath.Join(absRoot, filepath.FromSlash(normalized))
	rootResolved := canonicalPathWithExistingParent(absRoot)
	parentResolved := canonicalPathWithExistingParent(filepath.Dir(target))
	if !pathWithin(parentResolved, rootResolved) {
		return "", errors.New("path escapes worktree root")
	}
	if info, statErr := os.Lstat(target); statErr == nil && info.Mode()&os.ModeSymlink == 0 {
		targetResolved := canonicalPathWithExistingParent(target)
		if !pathWithin(targetResolved, rootResolved) {
			return "", errors.New("path escapes worktree root")
		}
	}
	rel, err := filepath.Rel(absRoot, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("path escapes worktree root")
	}
	return target, nil
}

func hashBlob(content []byte) plumbing.Hash {
	hasher := plumbing.NewHasher(objectformat.SHA1, plumbing.BlobObject, int64(len(content)))
	_, _ = hasher.Write(content)
	return hasher.Sum()
}

func readLimitedContent(reader io.Reader, limit int64) ([]byte, error) {
	limited := io.LimitReader(reader, limit+1)
	content, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > limit {
		return nil, fmt.Errorf("file exceeds diff limit of %d bytes", limit)
	}
	return content, nil
}

func isBinaryContent(content []byte) bool {
	limit := min(len(content), 8000)
	return bytes.IndexByte(content[:limit], 0) >= 0
}

type contentDiffFile struct {
	path    string
	mode    filemode.FileMode
	hash    plumbing.Hash
	content []byte
	binary  bool
}

func (f *contentDiffFile) Hash() plumbing.Hash     { return f.hash }
func (f *contentDiffFile) Mode() filemode.FileMode { return f.mode }
func (f *contentDiffFile) Path() string            { return f.path }
func (f *contentDiffFile) StringContent() string   { return string(f.content) }
func (f *contentDiffFile) IsBinaryContent() bool   { return f.binary }

type contentPatch struct {
	files []formatdiff.FilePatch
}

func newContentPatch(from, to *contentDiffFile) formatdiff.Patch {
	filePatch := &contentFilePatch{from: from, to: to}
	if (from != nil && from.binary) || (to != nil && to.binary) {
		filePatch.binary = true
	} else {
		fromContent := ""
		toContent := ""
		if from != nil {
			fromContent = string(from.content)
		}
		if to != nil {
			toContent = string(to.content)
		}
		for _, item := range gitdiff.Do(fromContent, toContent) {
			operation := formatdiff.Equal
			switch item.Type {
			case dmp.DiffDelete:
				operation = formatdiff.Delete
			case dmp.DiffInsert:
				operation = formatdiff.Add
			}
			filePatch.chunks = append(filePatch.chunks, contentChunk{content: item.Text, operation: operation})
		}
	}
	return contentPatch{files: []formatdiff.FilePatch{filePatch}}
}

func (p contentPatch) FilePatches() []formatdiff.FilePatch { return p.files }
func (p contentPatch) Message() string                     { return "" }

type contentFilePatch struct {
	from   *contentDiffFile
	to     *contentDiffFile
	binary bool
	chunks []formatdiff.Chunk
}

func (p *contentFilePatch) IsBinary() bool { return p.binary }
func (p *contentFilePatch) Files() (formatdiff.File, formatdiff.File) {
	var from formatdiff.File
	var to formatdiff.File
	if p.from != nil {
		from = p.from
	}
	if p.to != nil {
		to = p.to
	}
	return from, to
}
func (p *contentFilePatch) Chunks() []formatdiff.Chunk { return p.chunks }

type contentChunk struct {
	content   string
	operation formatdiff.Operation
}

func (c contentChunk) Content() string            { return c.content }
func (c contentChunk) Type() formatdiff.Operation { return c.operation }
