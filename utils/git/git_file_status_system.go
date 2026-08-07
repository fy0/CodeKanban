package git

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
)

func listFileStatusesSystem(ctx context.Context, path string, includeUntracked bool, maxEntries int, fast bool) (FileStatusResult, error) {
	untracked := "--untracked-files=no"
	if includeUntracked {
		untracked = "--untracked-files=all"
	}
	renameMode := "--renames"
	if fast {
		renameMode = "--no-renames"
	}
	output, err := runSystemGitOutput(ctx, path, OperationStatus, "--no-optional-locks", "status", "--porcelain=v1", "-z", untracked, renameMode)
	if err != nil {
		return FileStatusResult{}, err
	}
	statuses := parseSystemFileStatuses(output)
	keys := make([]string, 0, len(statuses))
	for key := range statuses {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	limit := len(keys)
	result := FileStatusResult{Statuses: make(map[string]FileStatus), TotalCount: len(keys)}
	if maxEntries > 0 && limit > maxEntries {
		limit = maxEntries
		result.Truncated = true
	}
	for _, key := range keys[:limit] {
		result.Statuses[key] = statuses[key]
	}
	headOutput, _ := runSystemGitOutput(ctx, path, OperationStatus, "rev-parse", "--verify", "HEAD")
	result.ChangeToken = buildStatusSnapshotToken(path, string(bytes.TrimSpace(headOutput)), statuses)
	return result, nil
}

func parseSystemFileStatuses(output []byte) map[string]FileStatus {
	result := make(map[string]FileStatus)
	records := bytes.Split(output, []byte{0})
	for index := 0; index < len(records); index++ {
		record := records[index]
		if len(record) < 4 {
			continue
		}
		x, y := record[0], record[1]
		path := normalizeGitRelativePath(string(record[3:]))
		if path == "" {
			continue
		}
		status := FileStatus{Path: path, Kind: classifySystemXY(x, y)}
		if x == 'R' || x == 'C' || y == 'R' || y == 'C' {
			if index+1 < len(records) {
				index++
				status.PreviousPath = normalizeGitRelativePath(string(records[index]))
			}
			status.Kind = FileChangeKindRenamed
		}
		result[path] = status
	}
	return result
}

func classifySystemXY(x, y byte) FileChangeKind {
	pair := string([]byte{x, y})
	switch pair {
	case "DD", "AU", "UD", "UA", "DU", "AA", "UU":
		return FileChangeKindConflicted
	}
	if x == '?' && y == '?' {
		return FileChangeKindUntracked
	}
	if x == 'D' || y == 'D' {
		return FileChangeKindDeleted
	}
	if x == 'A' || y == 'A' {
		return FileChangeKindAdded
	}
	return FileChangeKindModified
}

func generateUnifiedDiffSystem(ctx context.Context, root, relativePath, previousPath string) (string, error) {
	path := normalizeGitRelativePath(relativePath)
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	args := []string{"diff", "--no-ext-diff", "--no-textconv", "--no-color", "-M", "HEAD", "--", path}
	if previous := normalizeGitRelativePath(previousPath); previous != "" && previous != path {
		args = append(args, previous)
	}
	if _, err := runSystemGitOutput(ctx, root, OperationDiff, "rev-parse", "--verify", "HEAD"); err == nil {
		output, diffErr := runSystemGitOutput(ctx, root, OperationDiff, args...)
		return string(output), diffErr
	}
	output, err := runSystemGitOutputAllowExitOne(
		ctx,
		root,
		OperationDiff,
		"diff", "--no-index", "--no-ext-diff", "--no-textconv", "--no-color",
		"--src-prefix=a/", "--dst-prefix=b/", "--", os.DevNull, filepath.FromSlash(path),
	)
	return string(output), err
}

func generateDiffStatsSystem(ctx context.Context, root string, statuses []FileStatus) (map[string]DiffStat, error) {
	result := make(map[string]DiffStat, len(statuses))
	hasHead := true
	if _, err := runSystemGitOutput(ctx, root, OperationDiff, "rev-parse", "--verify", "HEAD"); err != nil {
		hasHead = false
	}
	if hasHead {
		output, err := runSystemGitOutput(ctx, root, OperationDiff, "diff", "--numstat", "-z", "--no-ext-diff", "--no-textconv", "--no-color", "-M", "HEAD", "--")
		if err != nil {
			return nil, err
		}
		for path, stat := range parseSystemNumstat(output) {
			result[path] = stat
		}
	}
	for _, status := range statuses {
		path := normalizeGitRelativePath(status.Path)
		if path == "" {
			continue
		}
		if _, exists := result[path]; exists {
			continue
		}
		if status.Kind == FileChangeKindUntracked || status.Kind == FileChangeKindAdded || !hasHead {
			file, err := loadWorktreeDiffFile(root, path)
			if err != nil {
				return nil, err
			}
			if file == nil || file.binary {
				result[path] = DiffStat{}
				continue
			}
			result[path] = DiffStat{Additions: countContentLines(file.content)}
		}
	}
	return result, nil
}

func parseSystemNumstat(output []byte) map[string]DiffStat {
	result := make(map[string]DiffStat)
	for cursor := 0; cursor < len(output); {
		firstTab := bytes.IndexByte(output[cursor:], '\t')
		if firstTab < 0 {
			break
		}
		firstTab += cursor
		secondTab := bytes.IndexByte(output[firstTab+1:], '\t')
		if secondTab < 0 {
			break
		}
		secondTab += firstTab + 1
		nul := bytes.IndexByte(output[secondTab+1:], 0)
		if nul < 0 {
			break
		}
		nul += secondTab + 1
		stat := DiffStat{
			Additions: parseNumstatValue(output[cursor:firstTab]),
			Deletions: parseNumstatValue(output[firstTab+1 : secondTab]),
		}
		path := normalizeGitRelativePath(string(output[secondTab+1 : nul]))
		cursor = nul + 1
		if path == "" && cursor < len(output) {
			oldEnd := bytes.IndexByte(output[cursor:], 0)
			if oldEnd < 0 {
				break
			}
			cursor += oldEnd + 1
			newEnd := bytes.IndexByte(output[cursor:], 0)
			if newEnd < 0 {
				break
			}
			path = normalizeGitRelativePath(string(output[cursor : cursor+newEnd]))
			cursor += newEnd + 1
		}
		if path != "" {
			result[path] = stat
		}
	}
	return result
}

func parseNumstatValue(value []byte) int64 {
	parsed, _ := strconv.ParseInt(string(value), 10, 64)
	return max(0, parsed)
}

func countContentLines(content []byte) int64 {
	if len(content) == 0 {
		return 0
	}
	count := int64(bytes.Count(content, []byte{'\n'}))
	if content[len(content)-1] != '\n' {
		count++
	}
	return count
}
