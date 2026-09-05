package log_watcher

import (
	"context"
	"encoding/json"
	"os"
	pathpkg "path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	// ClaudeCodeDirName is the subdirectory name for Claude Code
	ClaudeCodeDirName = ".claude"
	// ClaudeCodeProjectsSubDir is the projects subdirectory
	ClaudeCodeProjectsSubDir = "projects"
)

// SearchMode defines how to search for session files
type SearchMode int

const (
	// SearchModeBoth tries creation time first, then modification time
	SearchModeBoth SearchMode = iota
	// SearchModeCreationOnly only searches by creation time (new sessions)
	SearchModeCreationOnly
	// SearchModeModificationOnly only searches by modification time (resumed sessions)
	SearchModeModificationOnly
)

func (m SearchMode) String() string {
	switch m {
	case SearchModeBoth:
		return "both"
	case SearchModeCreationOnly:
		return "ctime"
	case SearchModeModificationOnly:
		return "mtime"
	default:
		return "unknown"
	}
}

// SearchResult contains the result of a session file search
type SearchResult struct {
	FilePath   string
	FoundBy    string // "new_session" or "resumed_session"
	CreateTime time.Time
	ModTime    time.Time
}

// ClaudeCodeFileSearcher searches for Claude Code session files
type ClaudeCodeFileSearcher struct {
	homeDir     string
	projectsDir string
	workingDir  string     // The working directory to match project folder
	searchMode  SearchMode // How to search for files
}

// NewClaudeCodeFileSearcher creates a new Claude Code file searcher
func NewClaudeCodeFileSearcher(workingDir string) (*ClaudeCodeFileSearcher, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	projectsDir := filepath.Join(homeDir, ClaudeCodeDirName, ClaudeCodeProjectsSubDir)

	return &ClaudeCodeFileSearcher{
		homeDir:     homeDir,
		projectsDir: projectsDir,
		workingDir:  workingDir,
	}, nil
}

// NewClaudeCodeFileSearcherWithHomeDir creates a new Claude Code file searcher with custom home directory
func NewClaudeCodeFileSearcherWithHomeDir(homeDir, workingDir string) *ClaudeCodeFileSearcher {
	projectsDir := filepath.Join(homeDir, ClaudeCodeDirName, ClaudeCodeProjectsSubDir)

	return &ClaudeCodeFileSearcher{
		homeDir:     homeDir,
		projectsDir: projectsDir,
		workingDir:  workingDir,
		searchMode:  SearchModeBoth,
	}
}

// SetSearchMode sets the search mode for finding session files
func (s *ClaudeCodeFileSearcher) SetSearchMode(mode SearchMode) {
	s.searchMode = mode
}

// GetSearchMode returns the current search mode
func (s *ClaudeCodeFileSearcher) GetSearchMode() SearchMode {
	return s.searchMode
}

// FindBySessionID finds a session file by its session ID
func (s *ClaudeCodeFileSearcher) FindBySessionID(sessionID string) (string, error) {
	// Encode working directory to find the project folder
	encodedPath := encodePathForClaude(s.workingDir)
	projectDir := filepath.Join(s.projectsDir, encodedPath)

	// Check if the directory exists
	if _, err := os.Stat(projectDir); os.IsNotExist(err) {
		return "", nil
	}

	// The session file should be named {sessionID}.jsonl
	filePath := filepath.Join(projectDir, sessionID+".jsonl")
	if _, err := os.Stat(filePath); err == nil {
		return filePath, nil
	}

	return "", nil
}

// GetSessionDir returns the base session directory
func (s *ClaudeCodeFileSearcher) GetSessionDir() string {
	return s.projectsDir
}

// EncodePathForClaude converts a path to Claude Code's folder naming convention.
// Example: D:\codes\2025\aicode-kanban -> D--codes-2025-aicode-kanban
// Example: D:\codes\2025\临时\box -> D--codes-2025----box (Chinese chars become dashes)
func EncodePathForClaude(path string) string {
	// Normalize both Unix and Windows separators first so the result is stable
	// regardless of the OS that is executing this code.
	path = strings.ReplaceAll(path, "\\", "/")

	// Use the slash-based path cleaner instead of filepath.Clean so Windows-style
	// input is normalized correctly even when tests run on Linux/macOS.
	path = pathpkg.Clean(path)

	// Remove any remaining trailing slashes (for safety)
	path = strings.TrimRight(path, "/\\")

	// Build result by processing each rune
	var result strings.Builder
	for _, r := range path {
		switch {
		case r == ':' || r == '/' || r == '_':
			// Replace special characters with dash
			result.WriteRune('-')
		case r > 127:
			// Replace non-ASCII characters (Chinese, etc.) with dash
			result.WriteRune('-')
		default:
			result.WriteRune(r)
		}
	}

	return result.String()
}

func encodePathForClaude(path string) string {
	return EncodePathForClaude(path)
}

// FindSessionFile searches for a session file that is actively being used
// For Claude Code, we handle two cases based on search mode:
// 1. New session: file created after process start time (SearchModeCreationOnly or SearchModeBoth)
// 2. Resumed session: file created earlier but modified after process start time (SearchModeModificationOnly or SearchModeBoth)
func (s *ClaudeCodeFileSearcher) FindSessionFile(ctx context.Context, afterTime time.Time) (string, error) {
	result, err := s.FindSessionFileWithResult(ctx, afterTime)
	if err != nil {
		return "", err
	}
	return result.FilePath, nil
}

// FindSessionFileWithResult searches for a session file and returns detailed result
func (s *ClaudeCodeFileSearcher) FindSessionFileWithResult(ctx context.Context, afterTime time.Time) (*SearchResult, error) {
	// Encode working directory to find the project folder
	encodedPath := encodePathForClaude(s.workingDir)
	projectDir := filepath.Join(s.projectsDir, encodedPath)

	// Check if the directory exists
	if _, err := os.Stat(projectDir); os.IsNotExist(err) {
		return &SearchResult{}, nil // Directory doesn't exist yet
	}

	// List all .jsonl files in the directory
	entries, err := os.ReadDir(projectDir)
	if err != nil {
		return nil, err
	}

	// Find files matching the pattern
	type fileWithTime struct {
		path    string
		modTime time.Time
		ctime   time.Time
	}

	var newSessionCandidates []fileWithTime     // Files created after afterTime (new sessions)
	var resumedSessionCandidates []fileWithTime // Files modified after afterTime (resumed sessions)

	tolerance := 5 * time.Second

	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		// Skip agent files and non-jsonl files
		if strings.HasPrefix(name, "agent-") || !strings.HasSuffix(name, ".jsonl") {
			continue
		}

		filePath := filepath.Join(projectDir, name)
		info, err := entry.Info()
		if err != nil {
			continue
		}

		modTime := info.ModTime()
		ctime := getFileCreationTime(filePath, info)

		f := fileWithTime{
			path:    filePath,
			modTime: modTime,
			ctime:   ctime,
		}

		// Categorize based on search mode
		if s.searchMode == SearchModeCreationOnly || s.searchMode == SearchModeBoth {
			if ctime.Add(tolerance).After(afterTime) {
				newSessionCandidates = append(newSessionCandidates, f)
			}
		}

		if s.searchMode == SearchModeModificationOnly || s.searchMode == SearchModeBoth {
			if modTime.Add(tolerance).After(afterTime) && !ctime.Add(tolerance).After(afterTime) {
				// Only add to resumed if not already a new session candidate
				resumedSessionCandidates = append(resumedSessionCandidates, f)
			} else if s.searchMode == SearchModeModificationOnly && modTime.Add(tolerance).After(afterTime) {
				// In modification-only mode, add all modified files
				resumedSessionCandidates = append(resumedSessionCandidates, f)
			}
		}
	}

	// For SearchModeCreationOnly, only check new session candidates
	if s.searchMode == SearchModeCreationOnly {
		if len(newSessionCandidates) > 0 {
			sort.Slice(newSessionCandidates, func(i, j int) bool {
				return newSessionCandidates[i].ctime.After(newSessionCandidates[j].ctime)
			})
			f := newSessionCandidates[0]
			return &SearchResult{
				FilePath:   f.path,
				FoundBy:    "new_session",
				CreateTime: f.ctime,
				ModTime:    f.modTime,
			}, nil
		}
		return &SearchResult{}, nil
	}

	// For SearchModeModificationOnly, only check resumed session candidates
	if s.searchMode == SearchModeModificationOnly {
		if len(resumedSessionCandidates) > 0 {
			sort.Slice(resumedSessionCandidates, func(i, j int) bool {
				return resumedSessionCandidates[i].modTime.After(resumedSessionCandidates[j].modTime)
			})
			f := resumedSessionCandidates[0]
			return &SearchResult{
				FilePath:   f.path,
				FoundBy:    "resumed_session",
				CreateTime: f.ctime,
				ModTime:    f.modTime,
			}, nil
		}
		return &SearchResult{}, nil
	}

	// For SearchModeBoth, prefer new session files first
	if len(newSessionCandidates) > 0 {
		sort.Slice(newSessionCandidates, func(i, j int) bool {
			return newSessionCandidates[i].ctime.After(newSessionCandidates[j].ctime)
		})
		f := newSessionCandidates[0]
		return &SearchResult{
			FilePath:   f.path,
			FoundBy:    "new_session",
			CreateTime: f.ctime,
			ModTime:    f.modTime,
		}, nil
	}

	// Fall back to resumed session files
	if len(resumedSessionCandidates) > 0 {
		sort.Slice(resumedSessionCandidates, func(i, j int) bool {
			return resumedSessionCandidates[i].modTime.After(resumedSessionCandidates[j].modTime)
		})
		f := resumedSessionCandidates[0]
		return &SearchResult{
			FilePath:   f.path,
			FoundBy:    "resumed_session",
			CreateTime: f.ctime,
			ModTime:    f.modTime,
		}, nil
	}

	return &SearchResult{}, nil
}

// ClaudeCodeUserEntry represents a user message entry in Claude Code's JSONL
type ClaudeCodeUserEntry struct {
	Type      string                   `json:"type"`
	Message   ClaudeCodeMessageContent `json:"message"`
	UUID      string                   `json:"uuid"`
	Timestamp string                   `json:"timestamp"`
	SessionID string                   `json:"sessionId"`
	Cwd       string                   `json:"cwd"`
}

// ClaudeCodeMessageContent represents the message content
type ClaudeCodeMessageContent struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"` // Can be string or array
}

// ParseClaudeCodeLine parses a Claude Code JSONL line and extracts user message if present
func ParseClaudeCodeLine(line string) (*UserMessage, string, error) {
	var entry struct {
		Type      string          `json:"type"`
		Message   json.RawMessage `json:"message"`
		UUID      string          `json:"uuid"`
		Timestamp string          `json:"timestamp"`
		SessionID string          `json:"sessionId"`
		IsMeta    bool            `json:"isMeta"`
	}

	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		return nil, "", err
	}

	// Only extract user messages (assistant/tool messages are ignored here).
	switch entry.Type {
	case "user":
		return parseClaudeCodeUserMessage(entry.Message, entry.Timestamp, entry.SessionID, entry.IsMeta)
	default:
		return nil, entry.SessionID, nil
	}
}

// parseClaudeCodeUserMessage parses a user message from Claude Code log
func parseClaudeCodeUserMessage(message json.RawMessage, timestamp, sessionID string, isMeta bool) (*UserMessage, string, error) {
	// Skip meta messages (system-generated messages)
	if isMeta {
		return nil, sessionID, nil
	}

	// Parse message content
	var msgContent struct {
		Role    string      `json:"role"`
		Content interface{} `json:"content"`
	}

	if err := json.Unmarshal(message, &msgContent); err != nil {
		return nil, sessionID, err
	}

	if msgContent.Role != "user" {
		return nil, sessionID, nil
	}

	// Extract text content
	var textContent string

	switch content := msgContent.Content.(type) {
	case string:
		textContent = content
	case []interface{}:
		// Handle array content (e.g., tool results)
		// Skip these for now as they are typically tool responses
		return nil, sessionID, nil
	}

	if textContent == "" {
		return nil, sessionID, nil
	}

	// Skip system command messages (e.g., /model, /help, local commands)
	if strings.HasPrefix(textContent, "<command-") || strings.HasPrefix(textContent, "<local-command") {
		return nil, sessionID, nil
	}

	ts, _ := time.Parse(time.RFC3339, timestamp)
	return &UserMessage{
		Timestamp: ts,
		Message:   textContent,
	}, sessionID, nil
}

// ParseClaudeCodeLineWrapper wraps ParseClaudeCodeLine to match the LogWatcher interface
func ParseClaudeCodeLineWrapper(w *LogWatcher, line string) (*UserMessage, error) {
	msg, sessionID, err := ParseClaudeCodeLine(line)
	if err != nil {
		return nil, err
	}

	// Update session ID if found
	if sessionID != "" {
		w.mu.Lock()
		if w.sessionID == "" {
			w.sessionID = sessionID
		}
		w.mu.Unlock()
	}

	return msg, nil
}
