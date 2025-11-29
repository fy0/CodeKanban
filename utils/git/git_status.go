package git

import (
	"bufio"
	"bytes"
	"errors"
	"strconv"
	"strings"
	"time"

	goGit "github.com/go-git/go-git/v5"
)

// WorktreeStatus aggregates repository state insights for a worktree.
type WorktreeStatus struct {
	Branch     string
	Ahead      int
	Behind     int
	Modified   int
	Staged     int
	Untracked  int
	Conflicted int
	LastCommit *CommitInfo
}

// CommitInfo describes a git commit summary.
type CommitInfo struct {
	SHA     string
	Message string
	Author  string
	Date    time.Time
}

// GetWorktreeStatus gathers branch, diff, and status metrics for a worktree path.
func GetWorktreeStatus(path string) (*WorktreeStatus, error) {
	if status, err := collectWorktreeStatusViaGit(path); err == nil {
		return status, nil
	}
	return getWorktreeStatusWithGoGit(path)
}

// GetWorktreeStatus returns the status for the provided worktree path. When
// path is empty the receiver's repository path is used.
func (r *GitRepo) GetWorktreeStatus(path string) (*WorktreeStatus, error) {
	if r == nil {
		return nil, errors.New("git repository is not initialized")
	}
	target := strings.TrimSpace(path)
	if target == "" {
		target = r.Path
	}
	return GetWorktreeStatus(target)
}

func collectWorktreeStatusViaGit(path string) (*WorktreeStatus, error) {
	cmd := newGitCommand(path, "status", "--porcelain=2", "--branch")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	status := parseGitStatusOutput(string(output))
	if status == nil {
		return nil, errors.New("failed to parse git status output")
	}

	if status.Branch == "" {
		status.Branch = describeBranch(path)
	}
	if status.LastCommit == nil {
		if commit, err := lastCommitInfo(path); err == nil {
			status.LastCommit = commit
		}
	}
	if status.Ahead == 0 && status.Behind == 0 {
		status.Ahead, status.Behind = getAheadBehind(path)
	}
	return status, nil
}

func getWorktreeStatusWithGoGit(path string) (*WorktreeStatus, error) {
	repo, err := goGit.PlainOpen(path)
	if err != nil {
		return nil, err
	}

	status := &WorktreeStatus{}

	if head, err := repo.Head(); err == nil {
		status.Branch = head.Name().Short()
		if status.Branch == "" || status.Branch == "HEAD" {
			status.Branch = describeBranch(path)
		}
		if commit, err := repo.CommitObject(head.Hash()); err == nil {
			status.LastCommit = &CommitInfo{
				SHA:     shortCommit(commit.Hash.String()),
				Message: firstLine(commit.Message),
				Author:  commit.Author.Name,
				Date:    commit.Author.When,
			}
		}
	} else {
		status.Branch = describeBranch(path)
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return nil, err
	}

	snap, err := worktree.Status()
	if err != nil {
		return nil, err
	}

	for _, fs := range snap {
		if fs.Staging == goGit.Untracked || fs.Worktree == goGit.Untracked {
			status.Untracked++
			continue
		}
		if fs.Staging == goGit.UpdatedButUnmerged || fs.Worktree == goGit.UpdatedButUnmerged {
			status.Conflicted++
			continue
		}
		switch fs.Worktree {
		case goGit.Modified, goGit.Added, goGit.Deleted, goGit.Renamed:
			status.Modified++
		}
		if fs.Staging != goGit.Unmodified && fs.Staging != goGit.Untracked {
			status.Staged++
		}
	}

	status.Ahead, status.Behind = getAheadBehind(path)
	return status, nil
}

func parseGitStatusOutput(output string) *WorktreeStatus {
	if strings.TrimSpace(output) == "" {
		return nil
	}

	status := &WorktreeStatus{}
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "# "):
			parseStatusHeader(status, strings.TrimSpace(line[2:]))
		case line[0] == '?':
			status.Untracked++
		case line[0] == '1' || line[0] == '2':
			parseTrackedStatus(status, line)
		case line[0] == 'u':
			status.Conflicted++
		}
	}
	if err := scanner.Err(); err != nil {
		return nil
	}
	return status
}

func parseStatusHeader(status *WorktreeStatus, line string) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return
	}

	switch fields[0] {
	case "branch.head":
		if len(fields) > 1 && fields[1] != "(detached)" {
			status.Branch = fields[1]
		}
	case "branch.ab":
		if len(fields) >= 3 {
			status.Ahead = parseAheadBehindToken(fields[1])
			status.Behind = parseAheadBehindToken(fields[2])
		}
	}
}

func parseTrackedStatus(status *WorktreeStatus, line string) {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return
	}

	xy := fields[1]
	if len(xy) < 2 {
		return
	}
	x := rune(xy[0])
	y := rune(xy[1])

	if x == 'U' || y == 'U' {
		status.Conflicted++
		return
	}

	if x != '.' {
		status.Staged++
	}
	if y != '.' {
		status.Modified++
	}
}

func parseAheadBehindToken(token string) int {
	value := strings.TrimSpace(token)
	if value == "" {
		return 0
	}
	if strings.HasPrefix(value, "+") || strings.HasPrefix(value, "-") {
		value = value[1:]
	}
	num, err := strconv.Atoi(value)
	if err != nil || num < 0 {
		return 0
	}
	return num
}

func lastCommitInfo(path string) (*CommitInfo, error) {
	cmd := newGitCommand(path, "log", "-1", "--pretty=format:%H%x00%an%x00%ad%x00%s", "--date=iso-strict")
	output, err := cmd.Output()
	if err != nil || len(output) == 0 {
		return nil, err
	}

	parts := bytes.SplitN(output, []byte{0}, 4)
	if len(parts) < 4 {
		return nil, errors.New("unexpected git log output")
	}

	dateStr := strings.TrimSpace(string(parts[2]))
	timestamp, err := time.Parse(time.RFC3339, dateStr)
	if err != nil {
		timestamp = time.Time{}
	}

	return &CommitInfo{
		SHA:     shortCommit(strings.TrimSpace(string(parts[0]))),
		Author:  strings.TrimSpace(string(parts[1])),
		Date:    timestamp,
		Message: strings.TrimSpace(string(parts[3])),
	}, nil
}

func getAheadBehind(path string) (ahead, behind int) {
	cmd := newGitCommand(path, "rev-list", "--left-right", "--count", "HEAD...@{upstream}")
	output, err := cmd.Output()
	if err != nil {
		return 0, 0
	}

	parts := strings.Fields(string(output))
	if len(parts) >= 2 {
		ahead, _ = strconv.Atoi(parts[0])
		behind, _ = strconv.Atoi(parts[1])
	}
	return ahead, behind
}

func shortCommit(hash string) string {
	if len(hash) > 7 {
		return hash[:7]
	}
	return hash
}

func firstLine(msg string) string {
	if idx := strings.Index(msg, "\n"); idx >= 0 {
		return strings.TrimSpace(msg[:idx])
	}
	return strings.TrimSpace(msg)
}

func describeBranch(path string) string {
	cmd := newGitCommand(path, "rev-parse", "--abbrev-ref", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	name := strings.TrimSpace(string(output))
	if name == "HEAD" {
		return ""
	}
	return name
}
