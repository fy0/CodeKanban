package git

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	goGit "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/filemode"
	"github.com/go-git/go-git/v6/plumbing/format/gitattributes"
	"github.com/go-git/go-git/v6/plumbing/object"
)

const (
	builtinContentSafetyTimeout = 5 * time.Second
	maxAttributesFileSize       = 1 << 20
)

func (r *GitRepo) ensureBuiltinWorktreeContentSafe(
	repository *goGit.Repository,
	worktreePath string,
	operation GitOperation,
) error {
	if err := r.ensureBuiltinContentConfigSafe(repository, operation); err != nil {
		return err
	}
	if info, err := os.Lstat(filepath.Join(worktreePath, ".lfsconfig")); err == nil && !info.IsDir() {
		return unsupportedContentFilterError(operation, ".lfsconfig requires system Git", nil)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return unsupportedContentFilterError(operation, "cannot inspect .lfsconfig", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), builtinContentSafetyTimeout)
	defer cancel()
	attributePaths, err := listWorktreeAttributePaths(ctx, repository, worktreePath, operation)
	if err != nil {
		return contentSafetyInspectionError(operation, err)
	}
	attributePaths = append(attributePaths, filepath.Join(r.commonDir, "info", "attributes"))
	for _, attributePath := range uniqueSortedPaths(attributePaths) {
		if err := ctx.Err(); err != nil {
			return contentSafetyInspectionError(operation, err)
		}
		unsafe, err := inspectWorktreeAttributesFile(worktreePath, attributePath)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return unsupportedContentFilterError(operation, fmt.Sprintf("cannot inspect %s", attributePath), err)
		}
		if unsafe {
			return unsupportedContentFilterError(operation, fmt.Sprintf("%s requires system Git", attributePath), nil)
		}
	}
	return nil
}

func (r *GitRepo) ensureBuiltinIndexContentSafe(
	repository *goGit.Repository,
	operation GitOperation,
) error {
	if err := r.ensureBuiltinContentConfigSafe(repository, operation); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), builtinContentSafetyTimeout)
	defer cancel()
	index, err := repository.Storer.Index()
	if err != nil {
		return contentSafetyInspectionError(operation, err)
	}
	seen := make(map[string]struct{})
	for _, entry := range index.Entries {
		if err := ctx.Err(); err != nil {
			return contentSafetyInspectionError(operation, err)
		}
		name := filepath.ToSlash(entry.Name)
		if name == ".lfsconfig" {
			return unsupportedContentFilterError(operation, ".lfsconfig requires system Git", nil)
		}
		if path.Base(name) != ".gitattributes" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		blob, err := repository.BlobObject(entry.Hash)
		if err != nil {
			return unsupportedContentFilterError(operation, fmt.Sprintf("cannot inspect indexed %s", name), err)
		}
		unsafe, err := inspectAttributesBlob(blob)
		if err != nil {
			return unsupportedContentFilterError(operation, fmt.Sprintf("cannot inspect indexed %s", name), err)
		}
		if unsafe {
			return unsupportedContentFilterError(operation, fmt.Sprintf("indexed %s requires system Git", name), nil)
		}
	}
	return nil
}

func (r *GitRepo) ensureBuiltinTreeContentSafe(
	repository *goGit.Repository,
	hash plumbing.Hash,
	operation GitOperation,
) error {
	if err := r.ensureBuiltinContentConfigSafe(repository, operation); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), builtinContentSafetyTimeout)
	defer cancel()
	commit, err := repository.CommitObject(hash)
	if err != nil {
		return contentSafetyInspectionError(operation, err)
	}
	tree, err := commit.Tree()
	if err != nil {
		return contentSafetyInspectionError(operation, err)
	}
	detail, err := inspectTreeContentSafety(ctx, tree, "")
	if err != nil {
		return contentSafetyInspectionError(operation, err)
	}
	if detail != "" {
		return unsupportedContentFilterError(operation, detail, nil)
	}
	return nil
}

func (r *GitRepo) ensureBuiltinContentConfigSafe(repository *goGit.Repository, operation GitOperation) error {
	if repository == nil {
		return errors.New("built-in Git repository is not initialized")
	}
	cfg, err := repository.ConfigScoped(config.SystemScope)
	if err != nil {
		cfg, err = repository.Config()
	}
	if err != nil {
		return contentSafetyInspectionError(operation, err)
	}
	if cfg.Raw == nil {
		if _, err := cfg.Marshal(); err != nil {
			return contentSafetyInspectionError(operation, err)
		}
	}
	for _, subsection := range cfg.Raw.Section("filter").Subsections {
		if subsection.HasOption("clean") || subsection.HasOption("smudge") ||
			subsection.HasOption("process") || strings.EqualFold(subsection.Option("required"), "true") {
			return unsupportedContentFilterError(operation, fmt.Sprintf("Git filter %q requires system Git", subsection.Name), nil)
		}
	}
	if cfg.Raw.Section("diff").HasOption("external") {
		return unsupportedContentFilterError(operation, "external Git diff requires system Git", nil)
	}
	return nil
}

func listWorktreeAttributePaths(
	ctx context.Context,
	repository *goGit.Repository,
	worktreePath string,
	operation GitOperation,
) ([]string, error) {
	if ProbeSystemGit(ctx, false).Available {
		output, err := runSystemGitOutput(
			ctx,
			worktreePath,
			operation,
			"--no-optional-locks",
			"ls-files",
			"-co",
			"--exclude-standard",
			"-z",
			"--",
			".gitattributes",
			":(glob)**/.gitattributes",
		)
		if err != nil {
			return nil, err
		}
		return nulSeparatedWorktreePaths(worktreePath, output), nil
	}

	paths := make([]string, 0)
	index, err := repository.Storer.Index()
	if err != nil {
		return nil, err
	}
	for _, entry := range index.Entries {
		if path.Base(filepath.ToSlash(entry.Name)) == ".gitattributes" {
			paths = append(paths, filepath.Join(worktreePath, filepath.FromSlash(entry.Name)))
		}
	}
	worktree, err := repository.Worktree()
	if err != nil {
		return nil, err
	}
	status, err := worktree.StatusWithOptions(goGit.StatusOptions{Strategy: goGit.Preload})
	if err != nil {
		return nil, err
	}
	for name := range status {
		if path.Base(filepath.ToSlash(name)) == ".gitattributes" {
			paths = append(paths, filepath.Join(worktreePath, filepath.FromSlash(name)))
		}
	}
	return paths, nil
}

func nulSeparatedWorktreePaths(worktreePath string, output []byte) []string {
	items := strings.Split(string(output), "\x00")
	paths := make([]string, 0, len(items))
	for _, item := range items {
		if item == "" {
			continue
		}
		candidate := filepath.Clean(filepath.Join(worktreePath, filepath.FromSlash(item)))
		if pathWithin(candidate, worktreePath) {
			paths = append(paths, candidate)
		}
	}
	return paths
}

func inspectWorktreeAttributesFile(worktreePath, attributePath string) (bool, error) {
	if !filepath.IsAbs(attributePath) {
		attributePath = filepath.Join(worktreePath, attributePath)
	}
	attributePath = filepath.Clean(attributePath)
	if !pathWithin(attributePath, worktreePath) && filepath.Base(attributePath) == ".gitattributes" {
		return false, errors.New("attributes path escapes worktree")
	}
	info, err := os.Lstat(attributePath)
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxAttributesFileSize {
		return false, errors.New("attributes file is not a supported regular file")
	}
	file, err := os.Open(attributePath)
	if err != nil {
		return false, err
	}
	defer file.Close()
	return inspectAttributes(file)
}

func inspectAttributesBlob(blob *object.Blob) (bool, error) {
	if blob.Size > maxAttributesFileSize {
		return false, errors.New("attributes file is too large")
	}
	reader, err := blob.Reader()
	if err != nil {
		return false, err
	}
	defer reader.Close()
	return inspectAttributes(reader)
}

func inspectAttributes(reader io.Reader) (bool, error) {
	entries, err := gitattributes.ReadAttributes(reader, nil, true)
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		for _, attribute := range entry.Attributes {
			switch strings.ToLower(attribute.Name()) {
			case "filter", "working-tree-encoding", "diff", "merge":
				return true, nil
			}
		}
	}
	return false, nil
}

func inspectTreeContentSafety(ctx context.Context, tree *object.Tree, prefix string) (string, error) {
	for index := range tree.Entries {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		entry := &tree.Entries[index]
		name := path.Join(prefix, entry.Name)
		if entry.Mode == filemode.Dir {
			subtree, err := tree.Tree(entry.Name)
			if err != nil {
				return "", err
			}
			detail, err := inspectTreeContentSafety(ctx, subtree, name)
			if err != nil || detail != "" {
				return detail, err
			}
			continue
		}
		if name == ".lfsconfig" {
			return ".lfsconfig requires system Git", nil
		}
		if entry.Name != ".gitattributes" {
			continue
		}
		if entry.Mode != filemode.Regular && entry.Mode != filemode.Executable && entry.Mode != filemode.Deprecated {
			return fmt.Sprintf("%s is not a regular attributes file", name), nil
		}
		file, err := tree.TreeEntryFile(entry)
		if err != nil {
			return "", err
		}
		unsafe, err := inspectAttributesBlob(&file.Blob)
		if err != nil {
			return "", err
		}
		if unsafe {
			return fmt.Sprintf("%s requires system Git", name), nil
		}
	}
	return "", nil
}

func uniqueSortedPaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	result := make([]string, 0, len(paths))
	for _, item := range paths {
		key := normalizePathKey(item)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, item)
	}
	sort.Strings(result)
	return result
}

func contentSafetyInspectionError(operation GitOperation, err error) error {
	detail := "content filter safety check failed"
	if errors.Is(err, context.DeadlineExceeded) {
		detail = "content filter safety check timed out"
	}
	return unsupportedContentFilterError(operation, detail, err)
}

func unsupportedContentFilterError(operation GitOperation, detail string, err error) error {
	return &OperationError{
		Code:      ErrorCodeUnsupportedContentFilter,
		Operation: operation,
		Detail:    detail,
		Err:       err,
	}
}
