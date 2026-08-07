package git

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"strconv"
	"strings"
	"time"
)

func hasTrackedWorktreeChangesSystem(ctx context.Context, path string) (bool, error) {
	output, err := runSystemGitOutput(
		ctx,
		path,
		OperationStatus,
		"--no-optional-locks",
		"status",
		"--porcelain=2",
		"--untracked-files=no",
		"--ignore-submodules=untracked",
		"--no-renames",
	)
	if err != nil {
		return false, err
	}
	return len(bytes.TrimSpace(output)) > 0, nil
}

func collectWorktreeStatusSystem(ctx context.Context, path string) (*WorktreeStatus, error) {
	output, err := runSystemGitOutput(ctx, path, OperationStatus, "--no-optional-locks", "status", "--porcelain=2", "--branch")
	if err != nil {
		return nil, err
	}
	result, err := parseSystemWorktreeStatus(string(output))
	if err != nil {
		return nil, err
	}
	if commit, commitErr := lastSystemCommitInfo(ctx, path); commitErr == nil {
		result.LastCommit = commit
	}
	return result, nil
}

func parseSystemWorktreeStatus(output string) (*WorktreeStatus, error) {
	result := &WorktreeStatus{}
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "# branch.head "):
			branch := strings.TrimSpace(strings.TrimPrefix(line, "# branch.head "))
			if branch != "(detached)" {
				result.Branch = branch
			}
		case strings.HasPrefix(line, "# branch.ab "):
			fields := strings.Fields(strings.TrimPrefix(line, "# branch.ab "))
			if len(fields) >= 2 {
				result.Ahead = parseSystemCount(fields[0])
				result.Behind = parseSystemCount(fields[1])
			}
		case line[0] == '?':
			result.Untracked++
		case line[0] == 'u':
			result.Conflicted++
		case line[0] == '1' || line[0] == '2':
			fields := strings.Fields(line)
			if len(fields) < 2 || len(fields[1]) < 2 {
				continue
			}
			x, y := fields[1][0], fields[1][1]
			if x == 'U' || y == 'U' {
				result.Conflicted++
				continue
			}
			if x != '.' {
				result.Staged++
			}
			if y != '.' {
				result.Modified++
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func parseSystemCount(value string) int {
	value = strings.TrimLeft(strings.TrimSpace(value), "+-")
	count, _ := strconv.Atoi(value)
	return count
}

func lastSystemCommitInfo(ctx context.Context, path string) (*CommitInfo, error) {
	output, err := runSystemGitOutput(ctx, path, OperationStatus, "log", "-1", "--format=%H%x00%an%x00%aI%x00%s")
	if err != nil {
		return nil, err
	}
	parts := bytes.SplitN(bytes.TrimSpace(output), []byte{0}, 4)
	if len(parts) != 4 {
		return nil, errors.New("unexpected system Git log output")
	}
	when, _ := time.Parse(time.RFC3339, string(parts[2]))
	return &CommitInfo{
		SHA:     shortCommit(string(parts[0])),
		Author:  string(parts[1]),
		Date:    when,
		Message: string(parts[3]),
	}, nil
}
