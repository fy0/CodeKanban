package log_watcher

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

const (
	// CodexSessionDirName is the subdirectory name for Codex sessions
	CodexSessionDirName = ".codex"
	// CodexSessionSubDir is the sessions subdirectory
	CodexSessionSubDir = "sessions"
	// CodexRolloutPrefix is the prefix for rollout files
	CodexRolloutPrefix = "rollout-"
	// CodexRolloutSuffix is the suffix for rollout files
	CodexRolloutSuffix = ".jsonl"
)

// CodexFileSearcher searches for Codex session files.
type CodexFileSearcher struct {
	homeDir              string
	sessionDir           string
	normalizedWorkingDir string
}

type codexRolloutMeta struct {
	Cwd        string
	Originator string
	Source     string
}

type codexRolloutCandidate struct {
	path    string
	modTime time.Time
	ctime   time.Time
	meta    codexRolloutMeta
}

// NewCodexFileSearcher creates a new Codex file searcher.
func NewCodexFileSearcher() (*CodexFileSearcher, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	return newCodexFileSearcher(homeDir, ""), nil
}

// NewCodexFileSearcherWithWorkingDir creates a new Codex file searcher scoped to a working directory.
func NewCodexFileSearcherWithWorkingDir(workingDir string) (*CodexFileSearcher, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	return newCodexFileSearcher(homeDir, workingDir), nil
}

// NewCodexFileSearcherWithHomeDir creates a new Codex file searcher with custom home directory.
func NewCodexFileSearcherWithHomeDir(homeDir string) *CodexFileSearcher {
	return NewCodexFileSearcherWithHomeDirAndWorkingDir(homeDir, "")
}

// NewCodexFileSearcherWithHomeDirAndWorkingDir creates a new Codex file searcher with custom home dir.
func NewCodexFileSearcherWithHomeDirAndWorkingDir(homeDir string, workingDir string) *CodexFileSearcher {
	return newCodexFileSearcher(homeDir, workingDir)
}

func newCodexFileSearcher(homeDir string, workingDir string) *CodexFileSearcher {
	sessionDir := filepath.Join(homeDir, CodexSessionDirName, CodexSessionSubDir)
	return &CodexFileSearcher{
		homeDir:              homeDir,
		sessionDir:           sessionDir,
		normalizedWorkingDir: normalizeComparablePath(workingDir),
	}
}

// GetSessionDir returns the base session directory.
func (s *CodexFileSearcher) GetSessionDir() string {
	return s.sessionDir
}

// FindBySessionID finds a rollout file by Codex session ID.
func (s *CodexFileSearcher) FindBySessionID(sessionID string) (string, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "", nil
	}

	type candidate struct {
		path    string
		modTime time.Time
	}

	matches := make([]candidate, 0, 4)
	suffix := "-" + sessionID + CodexRolloutSuffix
	err := filepath.WalkDir(s.sessionDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d == nil || d.IsDir() {
			return nil
		}
		name := d.Name()
		if !strings.HasPrefix(name, CodexRolloutPrefix) || !strings.HasSuffix(name, suffix) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		matches = append(matches, candidate{
			path:    path,
			modTime: info.ModTime(),
		})
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", nil
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].modTime.Equal(matches[j].modTime) {
			return matches[i].path > matches[j].path
		}
		return matches[i].modTime.After(matches[j].modTime)
	})
	return matches[0].path, nil
}

// FindSessionFile searches for a session file created after the given time.
func (s *CodexFileSearcher) FindSessionFile(ctx context.Context, afterTime time.Time) (string, error) {
	now := time.Now()
	dateDir := filepath.Join(s.sessionDir, now.Format("2006"), now.Format("01"), now.Format("02"))

	if _, err := os.Stat(dateDir); os.IsNotExist(err) {
		return "", nil
	}

	entries, err := os.ReadDir(dateDir)
	if err != nil {
		return "", err
	}

	candidates := make([]codexRolloutCandidate, 0, len(entries))
	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasPrefix(name, CodexRolloutPrefix) || !strings.HasSuffix(name, CodexRolloutSuffix) {
			continue
		}

		filePath := filepath.Join(dateDir, name)
		info, err := entry.Info()
		if err != nil {
			continue
		}

		ctime := getFileCreationTime(filePath, info)
		tolerance := 100 * time.Millisecond
		if !afterTime.IsZero() && ctime.Add(tolerance).Before(afterTime) {
			continue
		}

		candidate := codexRolloutCandidate{
			path:    filePath,
			modTime: info.ModTime(),
			ctime:   ctime,
		}
		if meta, ok := readCodexRolloutMeta(filePath); ok {
			candidate.meta = meta
		}
		candidates = append(candidates, candidate)
	}

	if len(candidates) == 0 {
		return "", nil
	}

	filtered := make([]codexRolloutCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if s.normalizedWorkingDir != "" {
			// If we know the terminal's working directory, refuse to bind an unrelated rollout.
			if !sameComparablePath(candidate.meta.Cwd, s.normalizedWorkingDir) {
				continue
			}
			if isCodeKanbanWebSessionRollout(candidate.meta) {
				continue
			}
		}
		filtered = append(filtered, candidate)
	}
	if len(filtered) == 0 {
		return "", nil
	}

	sort.Slice(filtered, func(i, j int) bool {
		leftWeb := isCodeKanbanWebSessionRollout(filtered[i].meta)
		rightWeb := isCodeKanbanWebSessionRollout(filtered[j].meta)
		if leftWeb != rightWeb {
			return !leftWeb
		}

		if !afterTime.IsZero() {
			leftDelta := absDuration(filtered[i].ctime.Sub(afterTime))
			rightDelta := absDuration(filtered[j].ctime.Sub(afterTime))
			if leftDelta != rightDelta {
				return leftDelta < rightDelta
			}
		}

		if !filtered[i].ctime.Equal(filtered[j].ctime) {
			if afterTime.IsZero() {
				return filtered[i].ctime.After(filtered[j].ctime)
			}
			return filtered[i].ctime.Before(filtered[j].ctime)
		}
		if !filtered[i].modTime.Equal(filtered[j].modTime) {
			if afterTime.IsZero() {
				return filtered[i].modTime.After(filtered[j].modTime)
			}
			return filtered[i].modTime.Before(filtered[j].modTime)
		}
		return filtered[i].path > filtered[j].path
	})

	return filtered[0].path, nil
}

func readCodexRolloutMeta(filePath string) (codexRolloutMeta, bool) {
	file, err := os.Open(filePath)
	if err != nil {
		return codexRolloutMeta{}, false
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 4*1024)
	scanner.Buffer(buf, 256*1024)

	for lineCount := 0; scanner.Scan() && lineCount < 16; lineCount += 1 {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var entry struct {
			Type    string          `json:"type"`
			Payload json.RawMessage `json:"payload"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry.Type != "session_meta" {
			continue
		}

		var payload SessionMetaPayload
		if err := json.Unmarshal(entry.Payload, &payload); err != nil {
			return codexRolloutMeta{}, false
		}

		return codexRolloutMeta{
			Cwd:        payload.Cwd,
			Originator: payload.Originator,
			Source:     payload.Source,
		}, true
	}

	return codexRolloutMeta{}, false
}

func normalizeComparablePath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}

	normalized := filepath.Clean(filepath.FromSlash(trimmed))
	if resolved, err := filepath.EvalSymlinks(normalized); err == nil && strings.TrimSpace(resolved) != "" {
		normalized = filepath.Clean(resolved)
	}
	if runtime.GOOS == "windows" {
		return strings.ToLower(normalized)
	}
	return normalized
}

func sameComparablePath(path string, normalizedTarget string) bool {
	return normalizeComparablePath(path) == normalizedTarget
}

func isCodeKanbanWebSessionRollout(meta codexRolloutMeta) bool {
	source := strings.ToLower(strings.TrimSpace(meta.Source))
	originator := strings.ToLower(strings.TrimSpace(meta.Originator))
	return strings.Contains(source, "codekanban-web-session") ||
		strings.Contains(originator, "codekanban-web-session")
}

// getFileCreationTime returns the file creation time.
// On Windows, it returns the actual creation time.
// On Unix systems, it returns the modification time as a fallback.
func getFileCreationTime(path string, info os.FileInfo) time.Time {
	if runtime.GOOS == "windows" {
		return getWindowsCreationTime(path, info)
	}

	return info.ModTime()
}

func absDuration(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
}

// ExtractSessionIDFromFilename extracts the session UUID from a rollout filename
// Filename format: rollout-2025-12-01T04-14-23-019ad666-f5ab-7501-a616-bbdc79da615b.jsonl
func ExtractSessionIDFromFilename(filename string) string {
	// Remove prefix and suffix
	name := strings.TrimPrefix(filename, CodexRolloutPrefix)
	name = strings.TrimSuffix(name, CodexRolloutSuffix)

	// The format is: timestamp-uuid
	// timestamp: 2025-12-01T04-14-23
	// uuid: 019ad666-f5ab-7501-a616-bbdc79da615b
	// Combined: 2025-12-01T04-14-23-019ad666-f5ab-7501-a616-bbdc79da615b

	// Find the UUID part (last 36 characters in standard UUID format)
	// UUID format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx (36 chars)
	if len(name) < 36 {
		return ""
	}

	// The UUID is at the end, but the timestamp also has dashes
	// We need to find where the UUID starts
	// UUID v7 format: timestamp-based, but we can identify by length

	// Split by dash and reconstruct UUID
	parts := strings.Split(name, "-")
	if len(parts) < 9 {
		// Not enough parts for timestamp + UUID
		return ""
	}

	// Last 5 parts form the UUID (8-4-4-4-12 format)
	uuidParts := parts[len(parts)-5:]
	return strings.Join(uuidParts, "-")
}
