package git

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	goGit "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/filemode"
)

type CapabilityMode string

const (
	CapabilityModeReadWrite   CapabilityMode = "read_write"
	CapabilityModeReadOnly    CapabilityMode = "read_only"
	CapabilityModeUnavailable CapabilityMode = "unavailable"
)

type GitOperation string

const (
	OperationBranchesRead     GitOperation = "branches_read"
	OperationBranchesWrite    GitOperation = "branches_write"
	OperationStatus           GitOperation = "status"
	OperationDiff             GitOperation = "diff"
	OperationWorktreesRead    GitOperation = "worktrees_read"
	OperationWorktreesWrite   GitOperation = "worktrees_write"
	OperationCommit           GitOperation = "commit"
	OperationFastForwardMerge GitOperation = "fast_forward_merge"
	OperationMerge            GitOperation = "merge"
	OperationRebase           GitOperation = "rebase"
	OperationSquash           GitOperation = "squash"
)

type OperationCapabilities struct {
	BranchesRead     bool `json:"branchesRead"`
	BranchesWrite    bool `json:"branchesWrite"`
	Status           bool `json:"status"`
	Diff             bool `json:"diff"`
	WorktreesRead    bool `json:"worktreesRead"`
	WorktreesWrite   bool `json:"worktreesWrite"`
	Commit           bool `json:"commit"`
	FastForwardMerge bool `json:"fastForwardMerge"`
	Merge            bool `json:"merge"`
	Rebase           bool `json:"rebase"`
	Squash           bool `json:"squash"`
}

type OperationEngines struct {
	BranchesRead     Engine `json:"branchesRead"`
	BranchesWrite    Engine `json:"branchesWrite"`
	Status           Engine `json:"status"`
	Diff             Engine `json:"diff"`
	WorktreesRead    Engine `json:"worktreesRead"`
	WorktreesWrite   Engine `json:"worktreesWrite"`
	Commit           Engine `json:"commit"`
	FastForwardMerge Engine `json:"fastForwardMerge"`
	Merge            Engine `json:"merge"`
	Rebase           Engine `json:"rebase"`
	Squash           Engine `json:"squash"`
}

func (c OperationCapabilities) Allowed(operation GitOperation) bool {
	switch operation {
	case OperationBranchesRead:
		return c.BranchesRead
	case OperationBranchesWrite:
		return c.BranchesWrite
	case OperationStatus:
		return c.Status
	case OperationDiff:
		return c.Diff
	case OperationWorktreesRead:
		return c.WorktreesRead
	case OperationWorktreesWrite:
		return c.WorktreesWrite
	case OperationCommit:
		return c.Commit
	case OperationFastForwardMerge:
		return c.FastForwardMerge
	case OperationMerge:
		return c.Merge
	case OperationRebase:
		return c.Rebase
	case OperationSquash:
		return c.Squash
	default:
		return false
	}
}

type CapabilityReason struct {
	Code   string `json:"code"`
	Detail string `json:"detail,omitempty"`
}

type CapabilityReport struct {
	Repository bool                  `json:"repository"`
	Mode       CapabilityMode        `json:"mode"`
	Operations OperationCapabilities `json:"operations"`
	Engines    OperationEngines      `json:"engines"`
	Reasons    []CapabilityReason    `json:"reasons"`
}

const (
	ErrorCodeNotRepository             = "git_not_repository"
	ErrorCodeUnsupportedFormat         = "git_unsupported_format"
	ErrorCodeOperationUnsupported      = "git_operation_unsupported"
	ErrorCodeRepositoryLocked          = "git_repository_locked"
	ErrorCodeWorktreeDirty             = "git_worktree_dirty"
	ErrorCodeNonFastForward            = "git_non_fast_forward"
	ErrorCodeAuthorMissing             = "git_author_missing"
	ErrorCodeInvalidReference          = "git_invalid_reference"
	ErrorCodeLinkedWorktreeUnreliable  = "git_linked_worktree_unreliable"
	ErrorCodeWorktreeNotFound          = "git_worktree_not_found"
	ErrorCodeWorktreeAlreadyRegistered = "git_worktree_already_registered"
	ErrorCodeSystemGitUnavailable      = "git_system_unavailable"
	ErrorCodeSystemGitFailed           = "git_system_failed"
	ErrorCodeMergeConflict             = "git_merge_conflict"
)

type OperationError struct {
	Code      string
	Operation GitOperation
	Detail    string
	Err       error
}

func (e *OperationError) Error() string {
	if e == nil {
		return ""
	}
	message := strings.TrimSpace(e.Detail)
	if message == "" && e.Err != nil {
		message = e.Err.Error()
	}
	if message == "" {
		message = e.Code
	}
	if e.Operation != "" {
		return fmt.Sprintf("%s: %s", e.Operation, message)
	}
	return message
}

func (e *OperationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func ErrorCode(err error) string {
	var operationErr *OperationError
	if errors.As(err, &operationErr) {
		return operationErr.Code
	}
	return ""
}

func (r *GitRepo) builtinCapabilities(worktreePath string) CapabilityReport {
	report := CapabilityReport{
		Repository: r != nil && r.repository != nil,
		Mode:       CapabilityModeUnavailable,
		Reasons:    []CapabilityReason{},
	}
	if r == nil || r.repository == nil {
		report.Reasons = append(report.Reasons, CapabilityReason{Code: ErrorCodeNotRepository})
		return report
	}

	report.Mode = CapabilityModeReadWrite
	report.Operations = OperationCapabilities{
		BranchesRead:     true,
		BranchesWrite:    true,
		Status:           !r.isBare,
		Diff:             !r.isBare,
		WorktreesRead:    !r.isBare,
		WorktreesWrite:   !r.isBare,
		Commit:           !r.isBare,
		FastForwardMerge: !r.isBare,
		Merge:            !r.isBare,
	}

	addReason := func(code, detail string) {
		for _, existing := range report.Reasons {
			if existing.Code == code && existing.Detail == detail {
				return
			}
		}
		report.Reasons = append(report.Reasons, CapabilityReason{Code: code, Detail: detail})
	}
	disableContentWrites := func() {
		report.Operations.Status = false
		report.Operations.Diff = false
		report.Operations.WorktreesWrite = false
		report.Operations.Commit = false
		report.Operations.FastForwardMerge = false
	}
	disableAllWrites := func() {
		report.Operations.BranchesWrite = false
		report.Operations.WorktreesWrite = false
		report.Operations.Commit = false
		report.Operations.FastForwardMerge = false
	}

	if r.isBare {
		disableAllWrites()
		report.Mode = CapabilityModeReadOnly
		addReason("bare_repository", "bare repositories do not have a working tree")
	}

	// ConfigScoped(SystemScope) applies Git's system -> global -> local
	// precedence. The merged config does not retain its raw representation,
	// so marshal it before inspecting extension/remote sections below.
	cfg, err := r.repository.ConfigScoped(config.SystemScope)
	if err != nil {
		cfg, err = r.repository.Config()
	}
	if err != nil {
		report.Operations = OperationCapabilities{}
		report.Mode = CapabilityModeUnavailable
		addReason(ErrorCodeUnsupportedFormat, "repository config cannot be read")
		return report
	}
	if cfg.Raw == nil {
		if _, marshalErr := cfg.Marshal(); marshalErr != nil {
			report.Operations = OperationCapabilities{}
			report.Mode = CapabilityModeUnavailable
			addReason(ErrorCodeUnsupportedFormat, "repository config cannot be normalized")
			return report
		}
	}

	objectFormat := strings.ToLower(strings.TrimSpace(cfg.Raw.Section("extensions").Option("objectformat")))
	if objectFormat != "" && objectFormat != "sha1" {
		report.Operations = OperationCapabilities{}
		report.Mode = CapabilityModeUnavailable
		addReason("unsupported_object_format", objectFormat)
		return report
	}

	extensions := cfg.Raw.Section("extensions")
	if value := strings.TrimSpace(extensions.Option("partialclone")); value != "" {
		disableContentWrites()
		addReason("partial_clone", value)
	}
	for _, remote := range cfg.Raw.Section("remote").Subsections {
		if strings.EqualFold(strings.TrimSpace(remote.Option("promisor")), "true") {
			disableContentWrites()
			addReason("partial_clone", remote.Name)
		}
	}

	if _, err := os.Stat(filepath.Join(r.commonDir, "shallow")); err == nil {
		report.Operations.FastForwardMerge = false
		addReason("shallow_repository", "history-dependent writes are disabled")
	}

	target := strings.TrimSpace(worktreePath)
	if target == "" {
		target = r.Path
	}
	worktreeGitDir, linked, resolveErr := r.resolveWorktreeGitDir(target)
	if resolveErr != nil && !r.isBare {
		disableContentWrites()
		addReason(ErrorCodeLinkedWorktreeUnreliable, resolveErr.Error())
	}
	if linked {
		if _, openErr := r.openWorktreeRepository(target); openErr != nil {
			disableContentWrites()
			addReason(ErrorCodeLinkedWorktreeUnreliable, openErr.Error())
		}
	}

	// A linked worktree has its own index and HEAD. Re-open that worktree
	// before checking format-specific capabilities so we never inspect the
	// main worktree by accident.
	targetRepository := r.repository
	closeTargetRepository := false
	if !r.isBare {
		opened, openErr := r.openWorktreeRepository(target)
		if openErr != nil {
			disableContentWrites()
			addReason(ErrorCodeLinkedWorktreeUnreliable, openErr.Error())
		} else {
			targetRepository = opened
			closeTargetRepository = true
		}
	}
	if closeTargetRepository {
		defer targetRepository.Close()
	}

	if worktreeGitDir != "" {
		if indexReason := inspectIndex(targetRepository, worktreeGitDir); indexReason != nil {
			disableContentWrites()
			addReason(indexReason.Code, indexReason.Detail)
		}
	}

	if hasRepositoryLock(r.commonDir, worktreeGitDir) {
		disableAllWrites()
		addReason(ErrorCodeRepositoryLocked, "repository lock file is present")
	}

	if hasUnsupportedFilters(cfg, target) {
		disableContentWrites()
		addReason("unsupported_content_filter", "LFS, clean/smudge, encoding, or custom diff attributes are configured")
	}

	if hasSubmodules(targetRepository, target) {
		disableContentWrites()
		addReason("submodules", "submodule worktrees are not supported")
	}

	if hasActiveHooks(r.commonDir, cfg) {
		report.Operations.WorktreesWrite = false
		report.Operations.Commit = false
		report.Operations.FastForwardMerge = false
		addReason("active_git_hooks", "operations that would bypass Git hooks are disabled")
	}

	if cfg.Commit.GpgSign.IsTrue() {
		report.Operations.Commit = false
		addReason("commit_signing_required", "automatic commit signing is not configured")
	}

	if !r.isBare {
		head, headErr := targetRepository.Head()
		switch {
		case errors.Is(headErr, plumbing.ErrReferenceNotFound):
			report.Operations.WorktreesWrite = false
			report.Operations.FastForwardMerge = false
			addReason("unborn_head", "linked worktrees and merges require an initial commit")
		case headErr != nil:
			disableAllWrites()
			addReason(ErrorCodeUnsupportedFormat, headErr.Error())
		case !head.Name().IsBranch():
			report.Operations.BranchesWrite = false
			report.Operations.WorktreesWrite = false
			report.Operations.Commit = false
			report.Operations.FastForwardMerge = false
			addReason("detached_head", "writes require a named branch")
		}
	}

	// The built-in merge implementation is deliberately fast-forward only.
	report.Operations.Merge = report.Operations.FastForwardMerge
	report.Operations.Rebase = false
	report.Operations.Squash = false
	if report.Mode != CapabilityModeUnavailable && !hasAnyWriteCapability(report.Operations) {
		report.Mode = CapabilityModeReadOnly
	}
	return report
}

type builtinCapabilityCacheEntry struct {
	report      CapabilityReport
	expiresAt   time.Time
	fingerprint string
}

var builtinCapabilityCache sync.Map

func clearCapabilityCache() {
	builtinCapabilityCache.Range(func(key, _ any) bool {
		builtinCapabilityCache.Delete(key)
		return true
	})
}

func (r *GitRepo) builtinCapabilitiesCached(worktreePath string, refresh bool) CapabilityReport {
	if r == nil {
		return CapabilityReport{Mode: CapabilityModeUnavailable, Reasons: []CapabilityReason{{Code: ErrorCodeNotRepository}}}
	}
	target := strings.TrimSpace(worktreePath)
	if target == "" {
		target = r.Path
	}
	key := normalizePathKey(r.commonDir) + "\x00" + normalizePathKey(target)
	fingerprint := capabilityFingerprint(r, target)
	if !refresh {
		if cached, ok := builtinCapabilityCache.Load(key); ok {
			entry := cached.(builtinCapabilityCacheEntry)
			if time.Now().Before(entry.expiresAt) && entry.fingerprint == fingerprint {
				entry.report.Reasons = append([]CapabilityReason(nil), entry.report.Reasons...)
				return entry.report
			}
			builtinCapabilityCache.Delete(key)
		}
	}
	report := r.builtinCapabilities(target)
	builtinCapabilityCache.Store(key, builtinCapabilityCacheEntry{
		report:      report,
		expiresAt:   time.Now().Add(2 * time.Minute),
		fingerprint: fingerprint,
	})
	report.Reasons = append([]CapabilityReason(nil), report.Reasons...)
	return report
}

func capabilityFingerprint(r *GitRepo, target string) string {
	if r == nil {
		return ""
	}
	paths := []string{
		filepath.Join(r.commonDir, "config"),
		filepath.Join(r.commonDir, "packed-refs"),
		filepath.Join(r.commonDir, "hooks"),
		filepath.Join(target, ".gitattributes"),
		filepath.Join(target, ".gitmodules"),
	}
	if gitDir, _, err := r.resolveWorktreeGitDir(target); err == nil {
		paths = append(paths, filepath.Join(gitDir, "index"), filepath.Join(gitDir, "HEAD"))
	}
	var builder strings.Builder
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			builder.WriteString(path)
			builder.WriteString(":missing;")
			continue
		}
		fmt.Fprintf(&builder, "%s:%d:%d:%d;", path, info.Size(), info.ModTime().UnixNano(), info.Mode())
	}
	return builder.String()
}

var allGitOperations = []GitOperation{
	OperationBranchesRead,
	OperationBranchesWrite,
	OperationStatus,
	OperationDiff,
	OperationWorktreesRead,
	OperationWorktreesWrite,
	OperationCommit,
	OperationFastForwardMerge,
	OperationMerge,
	OperationRebase,
	OperationSquash,
}

func chooseEngine(preference EnginePreference, preferSystem, builtinAllowed, systemAllowed bool) Engine {
	switch preference {
	case EnginePreferenceBuiltin:
		if builtinAllowed {
			return EngineBuiltin
		}
	case EnginePreferenceSystem:
		if systemAllowed {
			return EngineSystem
		}
	default:
		if preferSystem {
			if systemAllowed {
				return EngineSystem
			}
			if builtinAllowed {
				return EngineBuiltin
			}
		} else {
			if builtinAllowed {
				return EngineBuiltin
			}
			if systemAllowed {
				return EngineSystem
			}
		}
	}
	return EngineUnavailable
}

func (r *GitRepo) systemOperationAllowed(operation GitOperation, repositoryAvailable bool) bool {
	if !repositoryAvailable {
		return false
	}
	if !r.isBare {
		return true
	}
	switch operation {
	case OperationStatus, OperationDiff, OperationCommit, OperationFastForwardMerge,
		OperationMerge, OperationRebase, OperationSquash:
		return false
	default:
		return true
	}
}

func (r *GitRepo) effectiveEngine(worktreePath string, operation GitOperation) (Engine, error) {
	if r == nil {
		return EngineUnavailable, &OperationError{Code: ErrorCodeNotRepository, Operation: operation, Detail: "git repository is not initialized"}
	}
	target := strings.TrimSpace(worktreePath)
	if target == "" {
		target = r.Path
	}
	preference := preferenceForOperation(operation)
	preferSystem := autoPrefersSystem(operation)

	var builtinReport CapabilityReport
	builtinChecked := false
	builtinAllowed := func() bool {
		if !builtinChecked {
			refresh := preferSystem || preference == EnginePreferenceBuiltin
			builtinReport = r.builtinCapabilitiesCached(target, refresh)
			builtinChecked = true
		}
		return builtinReport.Operations.Allowed(operation)
	}
	systemChecked := false
	systemAllowed := false
	checkSystem := func() bool {
		if !systemChecked {
			systemAllowed = r.systemOperationAllowed(operation, systemGitRepositoryAvailable(context.Background(), target))
			systemChecked = true
		}
		return systemAllowed
	}

	var engine Engine
	switch preference {
	case EnginePreferenceBuiltin:
		engine = chooseEngine(preference, preferSystem, builtinAllowed(), false)
	case EnginePreferenceSystem:
		engine = chooseEngine(preference, preferSystem, false, checkSystem())
	default:
		if preferSystem {
			engine = chooseEngine(preference, true, false, checkSystem())
			if engine == EngineUnavailable {
				engine = chooseEngine(preference, true, builtinAllowed(), false)
			}
		} else {
			engine = chooseEngine(preference, false, builtinAllowed(), false)
			if engine == EngineUnavailable {
				engine = chooseEngine(preference, false, false, checkSystem())
			}
		}
	}
	if engine != EngineUnavailable {
		return engine, nil
	}

	if preference == EnginePreferenceSystem && !ProbeSystemGit(context.Background(), false).Available {
		info := ProbeSystemGit(context.Background(), false)
		return EngineUnavailable, &OperationError{Code: ErrorCodeSystemGitUnavailable, Operation: operation, Detail: info.Error}
	}
	if builtinChecked {
		return EngineUnavailable, operationUnsupportedError(builtinReport, operation)
	}
	return EngineUnavailable, &OperationError{Code: ErrorCodeOperationUnsupported, Operation: operation, Detail: "operation is unavailable for the selected Git engine"}
}

func operationUnsupportedError(report CapabilityReport, operation GitOperation) error {
	code := ErrorCodeOperationUnsupported
	detail := "operation is not supported for this repository"
	reason := CapabilityReason{}
	for _, candidate := range report.Reasons {
		if capabilityReasonApplies(candidate.Code, operation) {
			reason = candidate
			break
		}
	}
	if reason.Code == "" && len(report.Reasons) > 0 {
		reason = report.Reasons[0]
	}
	if reason.Code != "" {
		code = reason.Code
		detail = reason.Detail
		if detail == "" {
			detail = code
		}
	}
	return &OperationError{Code: code, Operation: operation, Detail: detail}
}

func (r *GitRepo) Capabilities(worktreePath string) CapabilityReport {
	target := strings.TrimSpace(worktreePath)
	if target == "" && r != nil {
		target = r.Path
	}
	builtin := r.builtinCapabilitiesCached(target, false)
	systemRepository := r != nil && systemGitRepositoryAvailable(context.Background(), target)
	report := CapabilityReport{
		Repository: builtin.Repository || systemRepository || HasRepositoryStructure(target),
		Mode:       CapabilityModeUnavailable,
		Reasons:    []CapabilityReason{},
	}

	for _, operation := range allGitOperations {
		engine := chooseEngine(
			preferenceForOperation(operation),
			autoPrefersSystem(operation),
			builtin.Operations.Allowed(operation),
			r != nil && r.systemOperationAllowed(operation, systemRepository),
		)
		setOperationEngine(&report.Engines, operation, engine)
		setOperationAllowed(&report.Operations, operation, engine != EngineUnavailable)
	}

	if hasAnyWriteCapability(report.Operations) {
		report.Mode = CapabilityModeReadWrite
	} else if report.Operations.BranchesRead || report.Operations.Status || report.Operations.Diff || report.Operations.WorktreesRead {
		report.Mode = CapabilityModeReadOnly
	}
	for _, reason := range builtin.Reasons {
		for _, operation := range allGitOperations {
			if !report.Operations.Allowed(operation) && capabilityReasonApplies(reason.Code, operation) {
				report.Reasons = append(report.Reasons, reason)
				break
			}
		}
	}
	if report.Mode == CapabilityModeUnavailable {
		preferenceNeedsSystem := CurrentEngineSettings().Read == EnginePreferenceSystem || CurrentEngineSettings().Write == EnginePreferenceSystem
		if preferenceNeedsSystem && !ProbeSystemGit(context.Background(), false).Available {
			info := ProbeSystemGit(context.Background(), false)
			report.Reasons = append(report.Reasons, CapabilityReason{Code: ErrorCodeSystemGitUnavailable, Detail: info.Error})
		} else if len(report.Reasons) == 0 {
			report.Reasons = append(report.Reasons, builtin.Reasons...)
		}
	}
	return report
}

func setOperationAllowed(capabilities *OperationCapabilities, operation GitOperation, allowed bool) {
	switch operation {
	case OperationBranchesRead:
		capabilities.BranchesRead = allowed
	case OperationBranchesWrite:
		capabilities.BranchesWrite = allowed
	case OperationStatus:
		capabilities.Status = allowed
	case OperationDiff:
		capabilities.Diff = allowed
	case OperationWorktreesRead:
		capabilities.WorktreesRead = allowed
	case OperationWorktreesWrite:
		capabilities.WorktreesWrite = allowed
	case OperationCommit:
		capabilities.Commit = allowed
	case OperationFastForwardMerge:
		capabilities.FastForwardMerge = allowed
	case OperationMerge:
		capabilities.Merge = allowed
	case OperationRebase:
		capabilities.Rebase = allowed
	case OperationSquash:
		capabilities.Squash = allowed
	}
}

func setOperationEngine(engines *OperationEngines, operation GitOperation, engine Engine) {
	switch operation {
	case OperationBranchesRead:
		engines.BranchesRead = engine
	case OperationBranchesWrite:
		engines.BranchesWrite = engine
	case OperationStatus:
		engines.Status = engine
	case OperationDiff:
		engines.Diff = engine
	case OperationWorktreesRead:
		engines.WorktreesRead = engine
	case OperationWorktreesWrite:
		engines.WorktreesWrite = engine
	case OperationCommit:
		engines.Commit = engine
	case OperationFastForwardMerge:
		engines.FastForwardMerge = engine
	case OperationMerge:
		engines.Merge = engine
	case OperationRebase:
		engines.Rebase = engine
	case OperationSquash:
		engines.Squash = engine
	}
}

func (r *GitRepo) requireCapability(worktreePath string, operation GitOperation) error {
	_, err := r.effectiveEngine(worktreePath, operation)
	return err
}

func (r *GitRepo) requireEngine(worktreePath string, operation GitOperation) (Engine, error) {
	return r.effectiveEngine(worktreePath, operation)
}

func capabilityReasonApplies(code string, operation GitOperation) bool {
	switch code {
	case ErrorCodeNotRepository, ErrorCodeUnsupportedFormat, ErrorCodeSystemGitUnavailable,
		"unsupported_object_format", "bare_repository":
		return true
	case ErrorCodeRepositoryLocked:
		return operation == OperationBranchesWrite || operation == OperationWorktreesWrite ||
			operation == OperationCommit || operation == OperationFastForwardMerge || operation == OperationMerge ||
			operation == OperationRebase || operation == OperationSquash
	case ErrorCodeLinkedWorktreeUnreliable, "partial_clone", "unsupported_content_filter",
		"submodules", "index_unreadable", "unsupported_index", "unsupported_index_extension":
		return operation == OperationStatus || operation == OperationDiff ||
			operation == OperationWorktreesWrite || operation == OperationCommit ||
			operation == OperationFastForwardMerge || operation == OperationMerge ||
			operation == OperationRebase || operation == OperationSquash
	case "shallow_repository":
		return operation == OperationFastForwardMerge || operation == OperationMerge || operation == OperationRebase
	case "active_git_hooks":
		return operation == OperationWorktreesWrite || operation == OperationCommit ||
			operation == OperationFastForwardMerge || operation == OperationMerge ||
			operation == OperationRebase || operation == OperationSquash
	case "commit_signing_required":
		return operation == OperationCommit
	case "unborn_head":
		return operation == OperationWorktreesWrite || operation == OperationFastForwardMerge ||
			operation == OperationMerge || operation == OperationRebase || operation == OperationSquash
	case "detached_head":
		return operation == OperationBranchesWrite || operation == OperationWorktreesWrite ||
			operation == OperationCommit || operation == OperationFastForwardMerge || operation == OperationMerge ||
			operation == OperationRebase || operation == OperationSquash
	default:
		return false
	}
}

func hasAnyWriteCapability(operations OperationCapabilities) bool {
	return operations.BranchesWrite || operations.WorktreesWrite || operations.Commit ||
		operations.FastForwardMerge || operations.Merge || operations.Rebase || operations.Squash
}

func inspectIndex(repository *goGit.Repository, gitDir string) *CapabilityReason {
	indexPath := filepath.Join(gitDir, "index")
	file, err := os.Open(indexPath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return &CapabilityReason{Code: "index_unreadable", Detail: err.Error()}
	}
	defer file.Close()

	header := make([]byte, 12)
	if _, err := file.Read(header); err != nil {
		return &CapabilityReason{Code: "index_unreadable", Detail: err.Error()}
	}
	if string(header[:4]) != "DIRC" {
		return &CapabilityReason{Code: "unsupported_index", Detail: "invalid index signature"}
	}
	version := binary.BigEndian.Uint32(header[4:8])
	if version < 2 || version > 4 {
		return &CapabilityReason{Code: "unsupported_index", Detail: fmt.Sprintf("index version %d", version)}
	}

	if repository == nil {
		return &CapabilityReason{Code: "index_unreadable", Detail: "worktree repository is unavailable"}
	}
	idx, err := repository.Storer.Index()
	if err != nil {
		return &CapabilityReason{Code: "unsupported_index", Detail: err.Error()}
	}
	for _, entry := range idx.Entries {
		if entry.SkipWorktree || entry.IntentToAdd {
			return &CapabilityReason{Code: "unsupported_index_extension", Detail: "skip-worktree and intent-to-add entries are not writable"}
		}
	}
	return nil
}

func hasRepositoryLock(commonDir, worktreeGitDir string) bool {
	candidates := []string{
		filepath.Join(commonDir, "index.lock"),
		filepath.Join(commonDir, "HEAD.lock"),
		filepath.Join(commonDir, "packed-refs.lock"),
		filepath.Join(commonDir, "config.lock"),
	}
	if worktreeGitDir != "" && !equalPath(worktreeGitDir, commonDir) {
		candidates = append(candidates,
			filepath.Join(worktreeGitDir, "index.lock"),
			filepath.Join(worktreeGitDir, "HEAD.lock"),
			filepath.Join(worktreeGitDir, "locked"),
		)
	}
	for _, candidate := range candidates {
		if _, err := os.Lstat(candidate); err == nil {
			return true
		}
	}
	return false
}

func hasUnsupportedFilters(cfg *config.Config, worktreePath string) bool {
	if cfg == nil {
		return true
	}
	filterSection := cfg.Raw.Section("filter")
	for _, subsection := range filterSection.Subsections {
		if subsection.HasOption("clean") || subsection.HasOption("smudge") || subsection.HasOption("process") || strings.EqualFold(subsection.Option("required"), "true") {
			return true
		}
	}
	if cfg.Raw.Section("diff").HasOption("external") {
		return true
	}
	if _, err := os.Stat(filepath.Join(worktreePath, ".lfsconfig")); err == nil {
		return true
	}

	unsupported := false
	_ = filepath.WalkDir(worktreePath, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || unsupported {
			return nil
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Name() != ".gitattributes" {
			return nil
		}
		file, openErr := os.Open(path)
		if openErr != nil {
			unsupported = true
			return nil
		}
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.ToLower(strings.TrimSpace(scanner.Text()))
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if strings.Contains(line, "filter=") || strings.Contains(line, "working-tree-encoding") || strings.Contains(line, "diff=") || strings.Contains(line, "merge=") {
				unsupported = true
				break
			}
		}
		if scanner.Err() != nil {
			unsupported = true
		}
		return nil
	})
	return unsupported
}

func hasSubmodules(repository *goGit.Repository, worktreePath string) bool {
	if _, err := os.Stat(filepath.Join(worktreePath, ".gitmodules")); err == nil {
		return true
	}
	if repository == nil {
		return false
	}
	idx, err := repository.Storer.Index()
	if err != nil {
		return false
	}
	for _, entry := range idx.Entries {
		if entry.Mode == filemode.Submodule {
			return true
		}
	}
	return false
}

func hasActiveHooks(commonDir string, cfg *config.Config) bool {
	hooksDir := filepath.Join(commonDir, "hooks")
	if cfg != nil {
		if configured := strings.TrimSpace(cfg.Raw.Section("core").Option("hookspath")); configured != "" {
			if filepath.IsAbs(configured) {
				hooksDir = configured
			} else {
				hooksDir = filepath.Join(commonDir, configured)
			}
		}
	}
	entries, err := os.ReadDir(hooksDir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() || strings.HasSuffix(strings.ToLower(entry.Name()), ".sample") {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr == nil && info.Mode().IsRegular() && info.Size() > 0 {
			return true
		}
	}
	return false
}

func mapGoGitError(operation GitOperation, err error) error {
	if err == nil {
		return nil
	}
	code := ErrorCodeOperationUnsupported
	switch {
	case errors.Is(err, goGit.ErrMissingAuthor):
		code = ErrorCodeAuthorMissing
	case errors.Is(err, goGit.ErrFastForwardMergeNotPossible):
		code = ErrorCodeNonFastForward
	case errors.Is(err, plumbing.ErrReferenceNotFound):
		code = ErrorCodeInvalidReference
	}
	return &OperationError{Code: code, Operation: operation, Err: err}
}
