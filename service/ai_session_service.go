package service

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"code-kanban/model"
	"code-kanban/model/tables"
	"code-kanban/utils"
	"code-kanban/utils/ai_assistant2/log_watcher"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// dirCacheEntry holds cached directory scan results.
type dirCacheEntry struct {
	modTime  time.Time           // Directory modification time
	sessions []*AISessionSummary // Cached session list
	cachedAt time.Time           // When this cache was created
}

// dirCacheTTL is the minimum time before re-checking directory mod time.
const dirCacheTTL = 3 * time.Second

// Scan phase constants
const (
	ScanPhaseRecent   = "recent"   // Last 24 hours - fast, priority scan
	ScanPhaseExtended = "extended" // 1-15 days - background scan
	ScanPhaseComplete = "complete" // Scan finished
)

// Time thresholds for phased scanning
const (
	recentThreshold = 24 * time.Hour      // Files within 24 hours
	maxAgeThreshold = 15 * 24 * time.Hour // Ignore files older than 15 days
)

// scanState tracks the scanning progress for a project
type scanState struct {
	phase       string              // Current scan phase
	sessions    []*AISessionSummary // Accumulated sessions
	pendingDirs []string            // Directories pending for extended scan
	mu          sync.RWMutex
}

// Global directory cache (projectDir -> cache entry)
var (
	dirCache   = make(map[string]*dirCacheEntry)
	dirCacheMu sync.RWMutex

	// Scan state tracking (projectPath -> state)
	scanStates   = make(map[string]*scanState)
	scanStatesMu sync.RWMutex

	// Background scan queue
	bgScanQueue   = make(chan *bgScanTask, 100)
	bgScanStarted bool
	bgScanMu      sync.Mutex
)

// bgScanTask represents a background scan task
type bgScanTask struct {
	projectPath string
	scanType    string // "claude" or "codex"
	projectDir  string // For claude: the specific project directory
}

// AISessionService manages AI assistant session detection and caching.
type AISessionService struct{}

// startBackgroundScanner starts the background scan worker if not already started
func startBackgroundScanner() {
	bgScanMu.Lock()
	defer bgScanMu.Unlock()

	if bgScanStarted {
		return
	}
	bgScanStarted = true

	go func() {
		service := NewAISessionService()
		for task := range bgScanQueue {
			ctx := context.Background()
			logger := service.logger(ctx)

			switch task.scanType {
			case "claude":
				if err := service.scanClaudeExtendedPhase(ctx, task.projectPath, task.projectDir); err != nil {
					logger.Debug("background claude scan failed",
						zap.String("projectPath", task.projectPath),
						zap.Error(err))
				}
			case "codex":
				if err := service.scanCodexExtendedPhase(ctx, task.projectPath); err != nil {
					logger.Debug("background codex scan failed",
						zap.String("projectPath", task.projectPath),
						zap.Error(err))
				}
			}
		}
	}()
}

// NewAISessionService creates a new AISessionService.
func NewAISessionService() *AISessionService {
	return &AISessionService{}
}

// ProjectAISessions contains AI session information for a project.
type ProjectAISessions struct {
	HasClaudeCode   bool                `json:"hasClaudeCode"`
	HasCodex        bool                `json:"hasCodex"`
	ClaudeSessions  []*AISessionSummary `json:"claudeSessions,omitempty"`
	CodexSessions   []*AISessionSummary `json:"codexSessions,omitempty"`
	ClaudeScanPhase string              `json:"claudeScanPhase,omitempty"` // "recent", "extended", "complete"
	CodexScanPhase  string              `json:"codexScanPhase,omitempty"`  // "recent", "extended", "complete"
}

// AISessionSummary contains summary information about an AI session.
type AISessionSummary struct {
	ID                    string     `json:"id"`
	SessionID             string     `json:"sessionId"`
	Type                  string     `json:"type"`
	Model                 string     `json:"model,omitempty"`
	Title                 string     `json:"title,omitempty"`
	SessionStartedAt      time.Time  `json:"sessionStartedAt"`
	LastMessageAt         *time.Time `json:"lastMessageAt,omitempty"`
	MessageCount          int        `json:"messageCount"`
	AssistantMessageCount int        `json:"assistantMessageCount"`
	FilePath              string     `json:"filePath"`
}

// GetProjectAISessions returns AI session information for a project path.
// It uses database caching to avoid repeated filesystem scans.
// First call returns recent (24h) sessions quickly, then background scans older files.
func (s *AISessionService) GetProjectAISessions(ctx context.Context, projectPath string) (*ProjectAISessions, error) {
	ctx = ensureContext(ctx)
	logger := s.logger(ctx)

	logger.Info("GetProjectAISessions called",
		zap.String("projectPath", projectPath))

	// Ensure background scanner is running
	startBackgroundScanner()

	result := &ProjectAISessions{
		ClaudeSessions:  make([]*AISessionSummary, 0),
		CodexSessions:   make([]*AISessionSummary, 0),
		ClaudeScanPhase: ScanPhaseComplete,
		CodexScanPhase:  ScanPhaseComplete,
	}

	// Normalize the project path
	projectPath = filepath.Clean(projectPath)
	logger.Debug("normalized project path",
		zap.String("projectPath", projectPath))

	// Get Claude Code sessions (phased)
	claudeSessions, claudePhase, err := s.getClaudeCodeSessionsPhased(ctx, projectPath)
	if err != nil {
		logger.Warn("failed to get Claude Code sessions", zap.Error(err), zap.String("path", projectPath))
	} else {
		// Ensure we always have a non-nil slice (nil slices get omitted with omitempty)
		if claudeSessions == nil {
			claudeSessions = []*AISessionSummary{}
		}
		result.ClaudeSessions = claudeSessions
		result.HasClaudeCode = len(claudeSessions) > 0
		result.ClaudeScanPhase = claudePhase
		logger.Debug("claude sessions found",
			zap.Int("count", len(claudeSessions)),
			zap.String("phase", claudePhase))
	}

	// Get Codex sessions (phased)
	codexSessions, codexPhase, err := s.getCodexSessionsPhased(ctx, projectPath)
	if err != nil {
		logger.Warn("failed to get Codex sessions", zap.Error(err), zap.String("path", projectPath))
	} else {
		// Ensure we always have a non-nil slice (nil slices get omitted with omitempty)
		if codexSessions == nil {
			codexSessions = []*AISessionSummary{}
		}
		result.CodexSessions = codexSessions
		result.HasCodex = len(codexSessions) > 0
		result.CodexScanPhase = codexPhase
		logger.Debug("codex sessions found",
			zap.Int("count", len(codexSessions)),
			zap.String("phase", codexPhase))
	}

	logger.Info("GetProjectAISessions returning",
		zap.Bool("hasClaudeCode", result.HasClaudeCode),
		zap.Bool("hasCodex", result.HasCodex),
		zap.Int("claudeCount", len(result.ClaudeSessions)),
		zap.Int("codexCount", len(result.CodexSessions)))

	return result, nil
}

// getClaudeCodeSessionsPhased returns Claude Code sessions using phased scanning.
// Phase 1: Quickly return sessions from last 24 hours
// Phase 2: Background scan for 1-15 days old sessions
// Files older than 15 days are ignored
func (s *AISessionService) getClaudeCodeSessionsPhased(ctx context.Context, projectPath string) ([]*AISessionSummary, string, error) {
	logger := s.logger(ctx)

	// Create searcher to find Claude Code session directory
	searcher, err := log_watcher.NewClaudeCodeFileSearcher(projectPath)
	if err != nil {
		logger.Debug("failed to create claude searcher",
			zap.String("projectPath", projectPath),
			zap.Error(err))
		return nil, ScanPhaseComplete, err
	}

	// Get the project-specific session directory
	encodedPath := log_watcher.EncodePathForClaude(projectPath)
	sessionBaseDir := searcher.GetSessionDir()
	projectDir := filepath.Join(sessionBaseDir, encodedPath)

	logger.Info("searching for claude sessions",
		zap.String("projectPath", projectPath),
		zap.String("encodedPath", encodedPath),
		zap.String("projectDir", projectDir))

	// Check if directory exists and get its mod time
	dirInfo, err := os.Stat(projectDir)
	if os.IsNotExist(err) {
		// Try case-insensitive match as fallback (Windows paths are case-insensitive)
		if entries, listErr := os.ReadDir(sessionBaseDir); listErr == nil {
			var availableDirs []string
			encodedPathLower := strings.ToLower(encodedPath)
			for _, e := range entries {
				if e.IsDir() {
					dirName := e.Name()
					availableDirs = append(availableDirs, dirName)
					// Case-insensitive match
					if strings.ToLower(dirName) == encodedPathLower {
						logger.Debug("found case-insensitive match for session directory",
							zap.String("expected", encodedPath),
							zap.String("found", dirName))
						projectDir = filepath.Join(sessionBaseDir, dirName)
						dirInfo, err = os.Stat(projectDir)
						if err == nil {
							goto dirFound
						}
					}
				}
			}
			logger.Info("claude session directory does not exist",
				zap.String("projectDir", projectDir),
				zap.String("expectedDirName", encodedPath),
				zap.Strings("availableDirs", availableDirs))
		} else {
			logger.Debug("claude session directory does not exist",
				zap.String("projectDir", projectDir))
		}
		return nil, ScanPhaseComplete, nil // No sessions for this project
	}
dirFound:
	if err != nil {
		logger.Debug("failed to stat claude session directory",
			zap.String("projectDir", projectDir),
			zap.Error(err))
		return nil, ScanPhaseComplete, err
	}
	dirModTime := dirInfo.ModTime()

	logger.Info("claude session directory found",
		zap.String("projectDir", projectDir),
		zap.Time("modTime", dirModTime))

	// Check directory cache - fast path
	dirCacheMu.RLock()
	cached, hasCached := dirCache[projectDir]
	dirCacheMu.RUnlock()

	if hasCached {
		// If within TTL, return cached without checking mod time
		if time.Since(cached.cachedAt) < dirCacheTTL {
			// Check scan state for phase
			phase := s.getClaudeScanPhase(projectPath)
			return cached.sessions, phase, nil
		}
		// If mod time unchanged, refresh TTL and return cached
		if cached.modTime.Equal(dirModTime) {
			dirCacheMu.Lock()
			cached.cachedAt = time.Now()
			dirCacheMu.Unlock()
			phase := s.getClaudeScanPhase(projectPath)
			return cached.sessions, phase, nil
		}
	}

	// Directory changed or no cache - do phased scan
	entries, err := os.ReadDir(projectDir)
	if err != nil {
		return nil, ScanPhaseComplete, err
	}

	logger.Debug("scanning claude directory",
		zap.String("projectDir", projectDir),
		zap.Int("totalEntries", len(entries)))

	db := model.GetDB()
	if db == nil {
		return nil, ScanPhaseComplete, model.ErrDBNotInitialized
	}

	now := time.Now()
	var sessions []*AISessionSummary
	var extendedFiles []string // Files for background scan (1-15 days old)

	var processedCount, skippedAgent, skippedOld int
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		// Skip agent files and non-jsonl files
		if strings.HasPrefix(name, "agent-") || !strings.HasSuffix(name, ".jsonl") {
			if strings.HasPrefix(name, "agent-") {
				skippedAgent++
			}
			continue
		}

		filePath := filepath.Join(projectDir, name)
		info, err := entry.Info()
		if err != nil {
			continue
		}

		fileAge := now.Sub(info.ModTime())

		// Skip files older than 15 days
		if fileAge > maxAgeThreshold {
			skippedOld++
			continue
		}

		processedCount++
		logger.Debug("processing claude session file",
			zap.String("name", name),
			zap.Duration("age", fileAge))

		// Extract session ID from filename (remove .jsonl extension)
		sessionID := strings.TrimSuffix(name, ".jsonl")

		// For files older than 24 hours, check if already cached in DB
		if fileAge > recentThreshold {
			// Check if we have a valid cached entry in DB
			var cached tables.AISessionTable
			err := db.WithContext(ctx).
				Where("session_id = ? AND type = ?", sessionID, tables.AISessionTypeClaudeCode).
				First(&cached).Error

			if err == nil && cached.FileModTime.Equal(info.ModTime()) && cached.FileSize == info.Size() {
				// Cache hit - use cached data
				sessions = append(sessions, &AISessionSummary{
					ID:                    cached.ID,
					SessionID:             cached.SessionID,
					Type:                  string(cached.Type),
					Model:                 cached.Model,
					Title:                 cached.Title,
					SessionStartedAt:      cached.SessionStartedAt,
					LastMessageAt:         cached.LastMessageAt,
					MessageCount:          cached.MessageCount,
					AssistantMessageCount: cached.AssistantMessageCount,
					FilePath:              cached.FilePath,
				})
			} else {
				// Need to scan - add to extended queue
				extendedFiles = append(extendedFiles, filePath)
			}
			continue
		}

		// Recent files (within 24 hours) - process immediately
		session, err := s.getOrUpdateClaudeSession(ctx, db, sessionID, filePath, info, projectPath)
		if err != nil {
			logger.Warn("failed to process session file",
				zap.String("file", filePath),
				zap.Int64("fileSize", info.Size()),
				zap.Error(err))
			continue
		}

		if session != nil {
			sessions = append(sessions, session)
			logger.Info("session added",
				zap.String("sessionId", session.SessionID),
				zap.String("title", session.Title))
		} else {
			logger.Warn("session is nil after processing",
				zap.String("file", filePath))
		}
	}

	logger.Info("claude scan summary",
		zap.Int("processed", processedCount),
		zap.Int("skippedAgent", skippedAgent),
		zap.Int("skippedOld", skippedOld),
		zap.Int("sessionsFound", len(sessions)),
		zap.Int("extendedFiles", len(extendedFiles)))

	// Sort by last message time (newest first)
	sort.Slice(sessions, func(i, j int) bool {
		ti := sessions[i].LastMessageAt
		tj := sessions[j].LastMessageAt
		if ti != nil && tj != nil {
			return ti.After(*tj)
		}
		if ti != nil {
			return true
		}
		if tj != nil {
			return false
		}
		return sessions[i].SessionStartedAt.After(sessions[j].SessionStartedAt)
	})

	// Determine scan phase and queue background work if needed
	var phase string
	if len(extendedFiles) > 0 {
		phase = ScanPhaseRecent

		// Store pending files in scan state
		scanStatesMu.Lock()
		state, exists := scanStates[projectPath+":claude"]
		if !exists {
			state = &scanState{
				phase:       ScanPhaseRecent,
				sessions:    sessions,
				pendingDirs: extendedFiles,
			}
			scanStates[projectPath+":claude"] = state
		} else {
			state.mu.Lock()
			state.pendingDirs = extendedFiles
			state.sessions = sessions
			state.phase = ScanPhaseRecent
			state.mu.Unlock()
		}
		scanStatesMu.Unlock()

		// Queue background scan
		select {
		case bgScanQueue <- &bgScanTask{
			projectPath: projectPath,
			scanType:    "claude",
			projectDir:  projectDir,
		}:
		default:
			// Queue full, will try again later
			logger.Debug("background scan queue full, skipping")
		}
	} else {
		phase = ScanPhaseComplete
		// Mark as complete in scan state
		scanStatesMu.Lock()
		if state, exists := scanStates[projectPath+":claude"]; exists {
			state.mu.Lock()
			state.phase = ScanPhaseComplete
			state.mu.Unlock()
		}
		scanStatesMu.Unlock()
	}

	// Update directory cache
	dirCacheMu.Lock()
	dirCache[projectDir] = &dirCacheEntry{
		modTime:  dirModTime,
		sessions: sessions,
		cachedAt: time.Now(),
	}
	dirCacheMu.Unlock()

	return sessions, phase, nil
}

// getClaudeScanPhase returns the current scan phase for a project
func (s *AISessionService) getClaudeScanPhase(projectPath string) string {
	scanStatesMu.RLock()
	defer scanStatesMu.RUnlock()

	if state, exists := scanStates[projectPath+":claude"]; exists {
		state.mu.RLock()
		defer state.mu.RUnlock()
		return state.phase
	}
	return ScanPhaseComplete
}

// scanClaudeExtendedPhase processes extended phase files in background
func (s *AISessionService) scanClaudeExtendedPhase(ctx context.Context, projectPath, projectDir string) error {
	logger := s.logger(ctx)

	scanStatesMu.RLock()
	state, exists := scanStates[projectPath+":claude"]
	scanStatesMu.RUnlock()

	if !exists {
		return nil
	}

	state.mu.RLock()
	pendingFiles := make([]string, len(state.pendingDirs))
	copy(pendingFiles, state.pendingDirs)
	state.mu.RUnlock()

	if len(pendingFiles) == 0 {
		return nil
	}

	db := model.GetDB()
	if db == nil {
		return model.ErrDBNotInitialized
	}

	var newSessions []*AISessionSummary

	for _, filePath := range pendingFiles {
		info, err := os.Stat(filePath)
		if err != nil {
			continue
		}

		name := filepath.Base(filePath)
		sessionID := strings.TrimSuffix(name, ".jsonl")

		session, err := s.getOrUpdateClaudeSession(ctx, db, sessionID, filePath, info, projectPath)
		if err != nil {
			logger.Debug("failed to process session file in background",
				zap.String("file", filePath),
				zap.Error(err))
			continue
		}

		if session != nil {
			newSessions = append(newSessions, session)
		}
	}

	// Update scan state and directory cache
	state.mu.Lock()
	state.sessions = append(state.sessions, newSessions...)
	state.pendingDirs = nil
	state.phase = ScanPhaseComplete

	// Sort all sessions
	allSessions := state.sessions
	sort.Slice(allSessions, func(i, j int) bool {
		ti := allSessions[i].LastMessageAt
		tj := allSessions[j].LastMessageAt
		if ti != nil && tj != nil {
			return ti.After(*tj)
		}
		if ti != nil {
			return true
		}
		if tj != nil {
			return false
		}
		return allSessions[i].SessionStartedAt.After(allSessions[j].SessionStartedAt)
	})
	state.mu.Unlock()

	// Update directory cache
	dirCacheMu.Lock()
	if cached, exists := dirCache[projectDir]; exists {
		cached.sessions = allSessions
		cached.cachedAt = time.Now()
	}
	dirCacheMu.Unlock()

	logger.Debug("completed extended phase scan for claude",
		zap.String("projectPath", projectPath),
		zap.Int("newSessions", len(newSessions)))

	return nil
}

// getClaudeCodeSessions returns Claude Code sessions for a project path.
// Deprecated: Use getClaudeCodeSessionsPhased instead
func (s *AISessionService) getClaudeCodeSessions(ctx context.Context, projectPath string) ([]*AISessionSummary, error) {
	sessions, _, err := s.getClaudeCodeSessionsPhased(ctx, projectPath)
	return sessions, err
}

// getClaudeCodeSessionsLegacy returns Claude Code sessions for a project path (legacy full scan).
func (s *AISessionService) getClaudeCodeSessionsLegacy(ctx context.Context, projectPath string) ([]*AISessionSummary, error) {
	logger := s.logger(ctx)

	// Create searcher to find Claude Code session directory
	searcher, err := log_watcher.NewClaudeCodeFileSearcher(projectPath)
	if err != nil {
		return nil, err
	}

	// Get the project-specific session directory
	encodedPath := log_watcher.EncodePathForClaude(projectPath)
	projectDir := filepath.Join(searcher.GetSessionDir(), encodedPath)

	// Check if directory exists and get its mod time
	dirInfo, err := os.Stat(projectDir)
	if os.IsNotExist(err) {
		return nil, nil // No sessions for this project
	}
	if err != nil {
		return nil, err
	}
	dirModTime := dirInfo.ModTime()

	// Check directory cache - fast path
	dirCacheMu.RLock()
	cached, hasCached := dirCache[projectDir]
	dirCacheMu.RUnlock()

	if hasCached {
		// If within TTL, return cached without checking mod time
		if time.Since(cached.cachedAt) < dirCacheTTL {
			return cached.sessions, nil
		}
		// If mod time unchanged, refresh TTL and return cached
		if cached.modTime.Equal(dirModTime) {
			dirCacheMu.Lock()
			cached.cachedAt = time.Now()
			dirCacheMu.Unlock()
			return cached.sessions, nil
		}
	}

	// Directory changed or no cache - do full scan
	entries, err := os.ReadDir(projectDir)
	if err != nil {
		return nil, err
	}

	db := model.GetDB()
	if db == nil {
		return nil, model.ErrDBNotInitialized
	}

	var sessions []*AISessionSummary

	for _, entry := range entries {
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

		// Extract session ID from filename (remove .jsonl extension)
		sessionID := strings.TrimSuffix(name, ".jsonl")

		// Check cache
		session, err := s.getOrUpdateClaudeSession(ctx, db, sessionID, filePath, info, projectPath)
		if err != nil {
			logger.Debug("failed to process session file",
				zap.String("file", filePath),
				zap.Error(err))
			continue
		}

		if session != nil {
			sessions = append(sessions, session)
		}
	}

	// Sort by last message time (newest first), fallback to session start time
	sort.Slice(sessions, func(i, j int) bool {
		ti := sessions[i].LastMessageAt
		tj := sessions[j].LastMessageAt
		if ti != nil && tj != nil {
			return ti.After(*tj)
		}
		if ti != nil {
			return true
		}
		if tj != nil {
			return false
		}
		return sessions[i].SessionStartedAt.After(sessions[j].SessionStartedAt)
	})

	// Update directory cache
	dirCacheMu.Lock()
	dirCache[projectDir] = &dirCacheEntry{
		modTime:  dirModTime,
		sessions: sessions,
		cachedAt: time.Now(),
	}
	dirCacheMu.Unlock()

	return sessions, nil
}

// getOrUpdateClaudeSession gets or updates a Claude Code session from cache.
func (s *AISessionService) getOrUpdateClaudeSession(
	ctx context.Context,
	db *gorm.DB,
	sessionID string,
	filePath string,
	fileInfo os.FileInfo,
	projectPath string,
) (*AISessionSummary, error) {
	// Check if we have a valid cached entry
	var cached tables.AISessionTable
	err := db.WithContext(ctx).
		Where("session_id = ? AND type = ?", sessionID, tables.AISessionTypeClaudeCode).
		First(&cached).Error

	if err == nil {
		// Check if cache is still valid (file hasn't changed)
		if cached.FileModTime.Equal(fileInfo.ModTime()) && cached.FileSize == fileInfo.Size() {
			return &AISessionSummary{
				ID:                    cached.ID,
				SessionID:             cached.SessionID,
				Type:                  string(cached.Type),
				Model:                 cached.Model,
				Title:                 cached.Title,
				SessionStartedAt:      cached.SessionStartedAt,
				LastMessageAt:         cached.LastMessageAt,
				MessageCount:          cached.MessageCount,
				AssistantMessageCount: cached.AssistantMessageCount,
				FilePath:              cached.FilePath,
			}, nil
		}
	}

	// Parse the session file
	sessionData, err := s.parseClaudeCodeSessionFile(filePath)
	if err != nil {
		return nil, err
	}

	// Create or update cache entry
	now := time.Now()
	record := tables.AISessionTable{
		SessionID:             sessionID,
		Type:                  tables.AISessionTypeClaudeCode,
		ProjectPath:           projectPath,
		FilePath:              filePath,
		Model:                 sessionData.Model,
		Title:                 sessionData.Title,
		SessionStartedAt:      sessionData.StartedAt,
		LastMessageAt:         sessionData.LastMessageAt,
		MessageCount:          sessionData.MessageCount,
		AssistantMessageCount: sessionData.AssistantMessageCount,
		FileModTime:           fileInfo.ModTime(),
		FileSize:              fileInfo.Size(),
	}

	if cached.ID != "" {
		// Update existing
		record.ID = cached.ID
		record.CreatedAt = cached.CreatedAt
		record.UpdatedAt = now
		if err := db.WithContext(ctx).Save(&record).Error; err != nil {
			return nil, err
		}
	} else {
		// Create new
		record.ID = utils.NewID()
		record.CreatedAt = now
		record.UpdatedAt = now
		if err := db.WithContext(ctx).Create(&record).Error; err != nil {
			return nil, err
		}
	}

	return &AISessionSummary{
		ID:                    record.ID,
		SessionID:             record.SessionID,
		Type:                  string(record.Type),
		Model:                 record.Model,
		Title:                 record.Title,
		SessionStartedAt:      record.SessionStartedAt,
		LastMessageAt:         record.LastMessageAt,
		MessageCount:          record.MessageCount,
		AssistantMessageCount: record.AssistantMessageCount,
		FilePath:              record.FilePath,
	}, nil
}

// claudeSessionData holds parsed data from a Claude Code session file.
type claudeSessionData struct {
	Model                 string
	Title                 string
	StartedAt             time.Time
	LastMessageAt         *time.Time
	MessageCount          int
	AssistantMessageCount int
}

// maxLinesForListScan is the maximum number of lines to scan for list metadata.
// We only need title, model, and start time - no need to scan entire file.
const maxLinesForListScan = 100

// parseClaudeCodeSessionFile parses a Claude Code session file to extract metadata.
// Only scans the first 100 lines for efficiency - enough to get title, model, and start time.
func (s *AISessionService) parseClaudeCodeSessionFile(filePath string) (*claudeSessionData, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return nil, err
	}

	data := &claudeSessionData{}
	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	lineCount := 0
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		lineCount++

		msg, sessionID, err := log_watcher.ParseClaudeCodeLine(line)
		if err != nil {
			continue
		}

		// Try to extract session start time from first valid entry
		if data.StartedAt.IsZero() && sessionID != "" {
			// Parse timestamp from the line
			var entry struct {
				Timestamp string `json:"timestamp"`
			}
			if json.Unmarshal([]byte(line), &entry) == nil && entry.Timestamp != "" {
				if ts, err := time.Parse(time.RFC3339, entry.Timestamp); err == nil {
					data.StartedAt = ts
				}
			}
		}

		// Count user messages and extract first message as title
		if msg != nil {
			data.MessageCount++
			data.LastMessageAt = &msg.Timestamp
			// Use first user message as title (truncate to 100 chars)
			if data.Title == "" && msg.Message != "" {
				title := strings.TrimSpace(msg.Message)
				if len(title) > 100 {
					title = title[:100] + "..."
				}
				data.Title = title
			}
		}

		// Try to extract model information and count assistant messages
		var entry struct {
			Type    string `json:"type"`
			Message struct {
				Model string `json:"model"`
				Role  string `json:"role"`
			} `json:"message"`
		}
		if json.Unmarshal([]byte(line), &entry) == nil {
			if entry.Message.Model != "" {
				data.Model = entry.Message.Model
			}
			// Count assistant messages
			if entry.Type == "assistant" && entry.Message.Role == "assistant" {
				data.AssistantMessageCount++
			}
		}

		// Stop after reading enough lines - we have what we need for the list
		if lineCount >= maxLinesForListScan {
			break
		}
	}

	// If we couldn't determine start time, use file mod time
	if data.StartedAt.IsZero() {
		data.StartedAt = fileInfo.ModTime()
	}

	// Estimate message count based on file size if we hit the limit
	// (actual count will be updated when viewing conversation)
	if lineCount >= maxLinesForListScan {
		// Rough estimate: average ~2KB per message entry
		data.MessageCount = int(fileInfo.Size() / 2000)
		data.AssistantMessageCount = data.MessageCount / 2
	}

	return data, scanner.Err()
}

func dedupeAISessionSummariesBySessionID(sessions []*AISessionSummary) []*AISessionSummary {
	if len(sessions) <= 1 {
		return sessions
	}

	bySessionID := make(map[string]*AISessionSummary, len(sessions))
	order := make([]string, 0, len(sessions))

	for _, session := range sessions {
		if session == nil {
			continue
		}

		key := session.SessionID
		if key == "" {
			key = session.ID
		}
		if key == "" {
			continue
		}

		if existing, ok := bySessionID[key]; ok {
			if isNewerAISessionSummary(existing, session) {
				bySessionID[key] = session
			}
			continue
		}

		bySessionID[key] = session
		order = append(order, key)
	}

	out := make([]*AISessionSummary, 0, len(bySessionID))
	for _, key := range order {
		if session, ok := bySessionID[key]; ok {
			out = append(out, session)
		}
	}
	return out
}

func isNewerAISessionSummary(existing, candidate *AISessionSummary) bool {
	if existing == nil {
		return true
	}
	if candidate == nil {
		return false
	}

	switch {
	case existing.LastMessageAt == nil && candidate.LastMessageAt != nil:
		return true
	case existing.LastMessageAt != nil && candidate.LastMessageAt == nil:
		return false
	case existing.LastMessageAt != nil && candidate.LastMessageAt != nil:
		if candidate.LastMessageAt.After(*existing.LastMessageAt) {
			return true
		}
		if existing.LastMessageAt.After(*candidate.LastMessageAt) {
			return false
		}
	}

	if candidate.SessionStartedAt.After(existing.SessionStartedAt) {
		return true
	}
	if existing.SessionStartedAt.After(candidate.SessionStartedAt) {
		return false
	}

	if candidate.MessageCount > existing.MessageCount {
		return true
	}
	if candidate.AssistantMessageCount > existing.AssistantMessageCount {
		return true
	}

	return false
}

// getCodexSessionsPhased returns Codex sessions using phased scanning.
// Phase 1: Quickly return sessions from last 24 hours (day 0)
// Phase 2: Background scan for 1-15 days old sessions
// Files older than 15 days are ignored
func (s *AISessionService) getCodexSessionsPhased(ctx context.Context, projectPath string) ([]*AISessionSummary, string, error) {
	logger := s.logger(ctx)

	searcher, err := log_watcher.NewCodexFileSearcher()
	if err != nil {
		return nil, ScanPhaseComplete, err
	}

	sessionDir := searcher.GetSessionDir()

	// Check if sessions directory exists
	dirInfo, err := os.Stat(sessionDir)
	if os.IsNotExist(err) {
		return nil, ScanPhaseComplete, nil
	}
	if err != nil {
		return nil, ScanPhaseComplete, err
	}

	// Use project path + "codex" as cache key
	cacheKey := projectPath + ":codex"
	dirModTime := dirInfo.ModTime()

	// Check directory cache - fast path
	dirCacheMu.RLock()
	cached, hasCached := dirCache[cacheKey]
	dirCacheMu.RUnlock()

	if hasCached {
		// If within TTL, return cached without checking mod time
		if time.Since(cached.cachedAt) < dirCacheTTL {
			phase := s.getCodexScanPhase(projectPath)
			return dedupeAISessionSummariesBySessionID(cached.sessions), phase, nil
		}
		// For codex, we also check today's date directory for new sessions
		todayDir := filepath.Join(sessionDir, time.Now().Format("2006"), time.Now().Format("01"), time.Now().Format("02"))
		todayInfo, err := os.Stat(todayDir)
		todayUnchanged := err != nil || (todayInfo != nil && !todayInfo.ModTime().After(cached.cachedAt))

		// If base dir and today's dir unchanged, use cache
		if cached.modTime.Equal(dirModTime) && todayUnchanged {
			dirCacheMu.Lock()
			cached.cachedAt = time.Now()
			dirCacheMu.Unlock()
			phase := s.getCodexScanPhase(projectPath)
			return dedupeAISessionSummariesBySessionID(cached.sessions), phase, nil
		}
	}

	// Do phased scan
	db := model.GetDB()
	if db == nil {
		return nil, ScanPhaseComplete, model.ErrDBNotInitialized
	}

	// First, check if we have cached sessions for this project
	var cachedSessions []tables.AISessionTable
	if err := db.WithContext(ctx).
		Where("project_path = ? AND type = ?", projectPath, tables.AISessionTypeCodex).
		Find(&cachedSessions).Error; err != nil {
		logger.Debug("failed to query cached codex sessions", zap.Error(err))
	}

	// Build a map of cached sessions for quick lookup
	cachedMap := make(map[string]*tables.AISessionTable)
	for i := range cachedSessions {
		cachedMap[cachedSessions[i].SessionID] = &cachedSessions[i]
	}

	var sessions []*AISessionSummary
	var extendedDays []int // Days for background scan (1-15 days ago)

	// Phase 1: Only scan today's directory (day 0) for immediate results
	now := time.Now()
	todayDir := filepath.Join(sessionDir, now.Format("2006"), now.Format("01"), now.Format("02"))

	if _, err := os.Stat(todayDir); err == nil {
		entries, err := os.ReadDir(todayDir)
		if err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}

				name := entry.Name()
				if !strings.HasPrefix(name, log_watcher.CodexRolloutPrefix) ||
					!strings.HasSuffix(name, log_watcher.CodexRolloutSuffix) {
					continue
				}

				filePath := filepath.Join(todayDir, name)
				info, err := entry.Info()
				if err != nil {
					continue
				}

				sessionID := log_watcher.ExtractSessionIDFromFilename(name)
				if sessionID == "" {
					continue
				}

				// Check cache first
				if dbCached, ok := cachedMap[sessionID]; ok {
					if dbCached.FileModTime.Equal(info.ModTime()) && dbCached.FileSize == info.Size() {
						if dbCached.ProjectPath == projectPath {
							sessions = append(sessions, &AISessionSummary{
								ID:                    dbCached.ID,
								SessionID:             dbCached.SessionID,
								Type:                  string(dbCached.Type),
								Model:                 dbCached.Model,
								Title:                 dbCached.Title,
								SessionStartedAt:      dbCached.SessionStartedAt,
								LastMessageAt:         dbCached.LastMessageAt,
								MessageCount:          dbCached.MessageCount,
								AssistantMessageCount: dbCached.AssistantMessageCount,
								FilePath:              dbCached.FilePath,
							})
						}
						continue
					}
				}

				// Parse the session file
				sessionData, err := s.parseCodexSessionFile(filePath)
				if err != nil {
					continue
				}

				if sessionData.Cwd != projectPath {
					continue
				}

				session, err := s.saveCodexSession(ctx, db, sessionID, filePath, info, sessionData)
				if err != nil {
					continue
				}

				sessions = append(sessions, session)
			}
		}
	}

	// Check for days 1-15 - add cached sessions and queue uncached for background
	for daysAgo := 1; daysAgo <= 15; daysAgo++ {
		date := now.AddDate(0, 0, -daysAgo)
		dateDir := filepath.Join(sessionDir, date.Format("2006"), date.Format("01"), date.Format("02"))

		if _, err := os.Stat(dateDir); os.IsNotExist(err) {
			continue
		}

		entries, err := os.ReadDir(dateDir)
		if err != nil {
			continue
		}

		hasUncached := false
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}

			name := entry.Name()
			if !strings.HasPrefix(name, log_watcher.CodexRolloutPrefix) ||
				!strings.HasSuffix(name, log_watcher.CodexRolloutSuffix) {
				continue
			}

			sessionID := log_watcher.ExtractSessionIDFromFilename(name)
			if sessionID == "" {
				continue
			}

			// Check cache
			if dbCached, ok := cachedMap[sessionID]; ok {
				info, err := entry.Info()
				if err == nil && dbCached.FileModTime.Equal(info.ModTime()) && dbCached.FileSize == info.Size() {
					if dbCached.ProjectPath == projectPath {
						sessions = append(sessions, &AISessionSummary{
							ID:                    dbCached.ID,
							SessionID:             dbCached.SessionID,
							Type:                  string(dbCached.Type),
							Model:                 dbCached.Model,
							Title:                 dbCached.Title,
							SessionStartedAt:      dbCached.SessionStartedAt,
							LastMessageAt:         dbCached.LastMessageAt,
							MessageCount:          dbCached.MessageCount,
							AssistantMessageCount: dbCached.AssistantMessageCount,
							FilePath:              dbCached.FilePath,
						})
					}
					continue
				}
			}

			// Has uncached files - need to scan this day in background
			hasUncached = true
		}

		if hasUncached {
			extendedDays = append(extendedDays, daysAgo)
		}
	}

	sessions = dedupeAISessionSummariesBySessionID(sessions)

	// Sort by last message time (newest first)
	sort.Slice(sessions, func(i, j int) bool {
		ti := sessions[i].LastMessageAt
		tj := sessions[j].LastMessageAt
		if ti != nil && tj != nil {
			return ti.After(*tj)
		}
		if ti != nil {
			return true
		}
		if tj != nil {
			return false
		}
		return sessions[i].SessionStartedAt.After(sessions[j].SessionStartedAt)
	})

	// Determine scan phase
	var phase string
	if len(extendedDays) > 0 {
		phase = ScanPhaseRecent

		// Store pending days in scan state
		scanStatesMu.Lock()
		state, exists := scanStates[projectPath+":codex"]
		if !exists {
			// Convert days to strings for storage
			pendingDirs := make([]string, len(extendedDays))
			for i, day := range extendedDays {
				date := now.AddDate(0, 0, -day)
				pendingDirs[i] = filepath.Join(sessionDir, date.Format("2006"), date.Format("01"), date.Format("02"))
			}
			state = &scanState{
				phase:       ScanPhaseRecent,
				sessions:    sessions,
				pendingDirs: pendingDirs,
			}
			scanStates[projectPath+":codex"] = state
		} else {
			state.mu.Lock()
			pendingDirs := make([]string, len(extendedDays))
			for i, day := range extendedDays {
				date := now.AddDate(0, 0, -day)
				pendingDirs[i] = filepath.Join(sessionDir, date.Format("2006"), date.Format("01"), date.Format("02"))
			}
			state.pendingDirs = pendingDirs
			state.sessions = sessions
			state.phase = ScanPhaseRecent
			state.mu.Unlock()
		}
		scanStatesMu.Unlock()

		// Queue background scan
		select {
		case bgScanQueue <- &bgScanTask{
			projectPath: projectPath,
			scanType:    "codex",
		}:
		default:
			logger.Debug("background scan queue full, skipping codex")
		}
	} else {
		phase = ScanPhaseComplete
		scanStatesMu.Lock()
		if state, exists := scanStates[projectPath+":codex"]; exists {
			state.mu.Lock()
			state.phase = ScanPhaseComplete
			state.mu.Unlock()
		}
		scanStatesMu.Unlock()
	}

	// Update directory cache
	dirCacheMu.Lock()
	dirCache[cacheKey] = &dirCacheEntry{
		modTime:  dirModTime,
		sessions: sessions,
		cachedAt: time.Now(),
	}
	dirCacheMu.Unlock()

	return sessions, phase, nil
}

// getCodexScanPhase returns the current scan phase for a project's codex sessions
func (s *AISessionService) getCodexScanPhase(projectPath string) string {
	scanStatesMu.RLock()
	defer scanStatesMu.RUnlock()

	if state, exists := scanStates[projectPath+":codex"]; exists {
		state.mu.RLock()
		defer state.mu.RUnlock()
		return state.phase
	}
	return ScanPhaseComplete
}

// scanCodexExtendedPhase processes extended phase directories in background
func (s *AISessionService) scanCodexExtendedPhase(ctx context.Context, projectPath string) error {
	logger := s.logger(ctx)

	scanStatesMu.RLock()
	state, exists := scanStates[projectPath+":codex"]
	scanStatesMu.RUnlock()

	if !exists {
		return nil
	}

	state.mu.RLock()
	pendingDirs := make([]string, len(state.pendingDirs))
	copy(pendingDirs, state.pendingDirs)
	state.mu.RUnlock()

	if len(pendingDirs) == 0 {
		return nil
	}

	db := model.GetDB()
	if db == nil {
		return model.ErrDBNotInitialized
	}

	var newSessions []*AISessionSummary

	for _, dateDir := range pendingDirs {
		entries, err := os.ReadDir(dateDir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}

			name := entry.Name()
			if !strings.HasPrefix(name, log_watcher.CodexRolloutPrefix) ||
				!strings.HasSuffix(name, log_watcher.CodexRolloutSuffix) {
				continue
			}

			filePath := filepath.Join(dateDir, name)
			info, err := entry.Info()
			if err != nil {
				continue
			}

			sessionID := log_watcher.ExtractSessionIDFromFilename(name)
			if sessionID == "" {
				continue
			}

			// Check if already cached
			var cached tables.AISessionTable
			err = db.WithContext(ctx).
				Where("session_id = ? AND type = ?", sessionID, tables.AISessionTypeCodex).
				First(&cached).Error
			if err == nil && cached.FileModTime.Equal(info.ModTime()) && cached.FileSize == info.Size() {
				if cached.ProjectPath == projectPath {
					newSessions = append(newSessions, &AISessionSummary{
						ID:                    cached.ID,
						SessionID:             cached.SessionID,
						Type:                  string(cached.Type),
						Model:                 cached.Model,
						Title:                 cached.Title,
						SessionStartedAt:      cached.SessionStartedAt,
						LastMessageAt:         cached.LastMessageAt,
						MessageCount:          cached.MessageCount,
						AssistantMessageCount: cached.AssistantMessageCount,
						FilePath:              cached.FilePath,
					})
				}
				continue
			}

			// Parse the session file
			sessionData, err := s.parseCodexSessionFile(filePath)
			if err != nil {
				continue
			}

			if sessionData.Cwd != projectPath {
				continue
			}

			session, err := s.saveCodexSession(ctx, db, sessionID, filePath, info, sessionData)
			if err != nil {
				continue
			}

			newSessions = append(newSessions, session)
		}
	}

	// Update scan state
	state.mu.Lock()
	state.sessions = dedupeAISessionSummariesBySessionID(append(state.sessions, newSessions...))
	state.pendingDirs = nil
	state.phase = ScanPhaseComplete

	// Sort all sessions
	allSessions := state.sessions
	sort.Slice(allSessions, func(i, j int) bool {
		ti := allSessions[i].LastMessageAt
		tj := allSessions[j].LastMessageAt
		if ti != nil && tj != nil {
			return ti.After(*tj)
		}
		if ti != nil {
			return true
		}
		if tj != nil {
			return false
		}
		return allSessions[i].SessionStartedAt.After(allSessions[j].SessionStartedAt)
	})
	state.mu.Unlock()

	// Update directory cache
	cacheKey := projectPath + ":codex"
	dirCacheMu.Lock()
	if cached, exists := dirCache[cacheKey]; exists {
		cached.sessions = allSessions
		cached.cachedAt = time.Now()
	}
	dirCacheMu.Unlock()

	logger.Debug("completed extended phase scan for codex",
		zap.String("projectPath", projectPath),
		zap.Int("newSessions", len(newSessions)))

	return nil
}

// getCodexSessions returns Codex sessions for a project path.
// Deprecated: Use getCodexSessionsPhased instead
func (s *AISessionService) getCodexSessions(ctx context.Context, projectPath string) ([]*AISessionSummary, error) {
	sessions, _, err := s.getCodexSessionsPhased(ctx, projectPath)
	return sessions, err
}

// codexSessionData holds parsed data from a Codex session file.
type codexSessionData struct {
	Cwd                   string
	Model                 string
	Title                 string
	StartedAt             time.Time
	LastMessageAt         *time.Time
	MessageCount          int
	AssistantMessageCount int
}

// parseCodexSessionFile parses a Codex session file to extract metadata.
func (s *AISessionService) parseCodexSessionFile(filePath string) (*codexSessionData, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	data := &codexSessionData{}
	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var entry log_watcher.LogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		switch entry.Type {
		case "session_meta":
			payload, ok := entry.Payload.(map[string]interface{})
			if !ok {
				continue
			}
			if cwd, ok := payload["cwd"].(string); ok {
				data.Cwd = cwd
			}
			if ts, ok := payload["timestamp"].(string); ok {
				if t, err := time.Parse(time.RFC3339, ts); err == nil {
					data.StartedAt = t
				}
			}

		case "turn_context":
			payload, ok := entry.Payload.(map[string]interface{})
			if !ok {
				continue
			}
			if model, ok := payload["model"].(string); ok && data.Model == "" {
				data.Model = model
			}

		case "event_msg":
			payload, ok := entry.Payload.(map[string]interface{})
			if !ok {
				continue
			}
			msgType, _ := payload["type"].(string)
			if msgType == "user_message" {
				data.MessageCount++
				if ts, err := time.Parse(time.RFC3339, entry.Timestamp); err == nil {
					data.LastMessageAt = &ts
				}
				// Extract first user message as title
				if data.Title == "" {
					if msg, ok := payload["message"].(string); ok && msg != "" {
						title := strings.TrimSpace(msg)
						if len(title) > 100 {
							title = title[:100] + "..."
						}
						data.Title = title
					}
				}
			} else if msgType == "agent_message" || msgType == "agent_reasoning" {
				// Codex uses "agent_message" for final responses and "agent_reasoning" for thinking
				data.AssistantMessageCount++
			}
		}
	}

	return data, scanner.Err()
}

// saveCodexSession saves a Codex session to the database cache.
func (s *AISessionService) saveCodexSession(
	ctx context.Context,
	db *gorm.DB,
	sessionID string,
	filePath string,
	fileInfo os.FileInfo,
	data *codexSessionData,
) (*AISessionSummary, error) {
	now := time.Now()

	// Check if record exists
	var existing tables.AISessionTable
	err := db.WithContext(ctx).
		Where("session_id = ? AND type = ?", sessionID, tables.AISessionTypeCodex).
		First(&existing).Error

	record := tables.AISessionTable{
		SessionID:             sessionID,
		Type:                  tables.AISessionTypeCodex,
		ProjectPath:           data.Cwd,
		FilePath:              filePath,
		Model:                 data.Model,
		Title:                 data.Title,
		SessionStartedAt:      data.StartedAt,
		LastMessageAt:         data.LastMessageAt,
		MessageCount:          data.MessageCount,
		AssistantMessageCount: data.AssistantMessageCount,
		FileModTime:           fileInfo.ModTime(),
		FileSize:              fileInfo.Size(),
	}

	if err == nil {
		// Update existing
		record.ID = existing.ID
		record.CreatedAt = existing.CreatedAt
		record.UpdatedAt = now
		if err := db.WithContext(ctx).Save(&record).Error; err != nil {
			return nil, err
		}
	} else {
		// Create new
		record.ID = utils.NewID()
		record.CreatedAt = now
		record.UpdatedAt = now
		if err := db.WithContext(ctx).Create(&record).Error; err != nil {
			return nil, err
		}
	}

	return &AISessionSummary{
		ID:                    record.ID,
		SessionID:             record.SessionID,
		Type:                  string(record.Type),
		Model:                 record.Model,
		Title:                 record.Title,
		SessionStartedAt:      record.SessionStartedAt,
		LastMessageAt:         record.LastMessageAt,
		MessageCount:          record.MessageCount,
		AssistantMessageCount: record.AssistantMessageCount,
		FilePath:              record.FilePath,
	}, nil
}

// ResolveCodexSessionBySessionID locates a Codex session by native session ID
// and refreshes the cached ai_sessions row when the backing file changed.
func (s *AISessionService) ResolveCodexSessionBySessionID(
	ctx context.Context,
	sessionID string,
) (*tables.AISessionTable, error) {
	ctx = ensureContext(ctx)

	db := model.GetDB()
	if db == nil {
		return nil, model.ErrDBNotInitialized
	}

	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, gorm.ErrRecordNotFound
	}

	var cached tables.AISessionTable
	err := db.WithContext(ctx).
		Where("session_id = ? AND type = ?", sessionID, tables.AISessionTypeCodex).
		First(&cached).Error
	if err == nil {
		info, statErr := os.Stat(cached.FilePath)
		if statErr == nil {
			if cached.FileModTime.Equal(info.ModTime()) && cached.FileSize == info.Size() {
				return &cached, nil
			}
			data, parseErr := s.parseCodexSessionFile(cached.FilePath)
			if parseErr == nil {
				if _, saveErr := s.saveCodexSession(ctx, db, sessionID, cached.FilePath, info, data); saveErr == nil {
					var refreshed tables.AISessionTable
					if reloadErr := db.WithContext(ctx).
						Where("session_id = ? AND type = ?", sessionID, tables.AISessionTypeCodex).
						First(&refreshed).Error; reloadErr == nil {
						return &refreshed, nil
					}
				}
			}
		}
	}

	searcher, searchErr := log_watcher.NewCodexFileSearcher()
	if searchErr != nil {
		return nil, searchErr
	}
	filePath, searchErr := searcher.FindBySessionID(sessionID)
	if searchErr != nil {
		return nil, searchErr
	}
	if strings.TrimSpace(filePath) == "" {
		return nil, gorm.ErrRecordNotFound
	}

	info, statErr := os.Stat(filePath)
	if statErr != nil {
		return nil, statErr
	}
	data, parseErr := s.parseCodexSessionFile(filePath)
	if parseErr != nil {
		return nil, parseErr
	}
	if _, saveErr := s.saveCodexSession(ctx, db, sessionID, filePath, info, data); saveErr != nil {
		return nil, saveErr
	}

	var record tables.AISessionTable
	if loadErr := db.WithContext(ctx).
		Where("session_id = ? AND type = ?", sessionID, tables.AISessionTypeCodex).
		First(&record).Error; loadErr != nil {
		return nil, loadErr
	}
	return &record, nil
}

// ResolveCodexSessionByID locates a cached Codex session by database ID and
// refreshes the cached row when the underlying rollout file changed.
func (s *AISessionService) ResolveCodexSessionByID(
	ctx context.Context,
	dbID string,
) (*tables.AISessionTable, error) {
	ctx = ensureContext(ctx)

	db := model.GetDB()
	if db == nil {
		return nil, model.ErrDBNotInitialized
	}

	dbID = strings.TrimSpace(dbID)
	if dbID == "" {
		return nil, gorm.ErrRecordNotFound
	}

	var session tables.AISessionTable
	if err := db.WithContext(ctx).
		Where("id = ? AND type = ?", dbID, tables.AISessionTypeCodex).
		First(&session).Error; err != nil {
		return nil, err
	}

	return s.ResolveCodexSessionBySessionID(ctx, session.SessionID)
}

// CleanupStaleSessions removes cached sessions whose files no longer exist.
func (s *AISessionService) CleanupStaleSessions(ctx context.Context) (int64, error) {
	ctx = ensureContext(ctx)
	logger := s.logger(ctx)

	db := model.GetDB()
	if db == nil {
		return 0, model.ErrDBNotInitialized
	}

	var sessions []tables.AISessionTable
	if err := db.WithContext(ctx).Find(&sessions).Error; err != nil {
		return 0, err
	}

	var deletedCount int64
	for _, session := range sessions {
		if _, err := os.Stat(session.FilePath); os.IsNotExist(err) {
			if err := db.WithContext(ctx).Delete(&session).Error; err != nil {
				logger.Warn("failed to delete stale session",
					zap.String("sessionId", session.SessionID),
					zap.Error(err))
				continue
			}
			deletedCount++
		}
	}

	if deletedCount > 0 {
		logger.Info("cleaned up stale AI sessions", zap.Int64("count", deletedCount))
	}

	return deletedCount, nil
}

func (s *AISessionService) logger(ctx context.Context) *zap.Logger {
	return utils.LoggerFromContext(ctx).Named("ai-session-service")
}

// ConversationMessage represents a single message in a conversation.
type ConversationImageAttachment struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Previewable bool   `json:"previewable"`
	PreviewURL  string `json:"previewUrl,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
	Source      string `json:"-"`
}

type ConversationMessage struct {
	Role      string                        `json:"role"`      // "user" or "assistant"
	Content   string                        `json:"content"`   // Message content
	Timestamp time.Time                     `json:"timestamp"` // Message timestamp
	Kind      string                        `json:"kind,omitempty"`
	ToolUseID string                        `json:"toolUseId,omitempty"`
	HasMore   bool                          `json:"hasMore,omitempty"`
	Full      string                        `json:"full,omitempty"`
	Images    []ConversationImageAttachment `json:"images,omitempty"`
}

type ConversationImagePreview struct {
	MimeType string
	FilePath string
	Data     []byte
}

var errStopConversationScan = errors.New("stop conversation scan")

type conversationParseOptions struct {
	includeImageSources bool
	onMessage           func(*ConversationMessage) bool
}

type conversationWindowOptions struct {
	beforeCursor int
	limit        int
}

func emitConversationMessage(dst *[]*ConversationMessage, msg *ConversationMessage, options conversationParseOptions) error {
	if options.onMessage != nil {
		if !options.onMessage(msg) {
			return errStopConversationScan
		}
	}
	if dst != nil {
		*dst = append(*dst, msg)
	}
	return nil
}

// ConversationResponse contains the full conversation for a session.
type ConversationResponse struct {
	SessionID string                 `json:"sessionId"`
	Title     string                 `json:"title"`
	Messages  []*ConversationMessage `json:"messages"`
}

type ConversationWindowResponse struct {
	SessionID                string                 `json:"sessionId"`
	Title                    string                 `json:"title"`
	Messages                 []*ConversationMessage `json:"messages"`
	Total                    int                    `json:"total"`
	TotalUserMessages        int                    `json:"totalUserMessages"`
	UserMessagesBeforeWindow int                    `json:"userMessagesBeforeWindow"`
	WindowStart              int                    `json:"windowStart"`
	WindowEnd                int                    `json:"windowEnd"`
	HasMoreBefore            bool                   `json:"hasMoreBefore"`
	BeforeCursor             string                 `json:"beforeCursor,omitempty"`
}

// GetSessionConversation retrieves the full conversation for a given database ID.
func (s *AISessionService) GetSessionConversation(ctx context.Context, dbID string) (*ConversationResponse, error) {
	return s.getConversationByQuery(ctx, "id = ?", dbID)
}

// GetSessionConversationBySessionID retrieves the full conversation for a given session ID (UUID).
func (s *AISessionService) GetSessionConversationBySessionID(ctx context.Context, sessionID string) (*ConversationResponse, error) {
	ctx = ensureContext(ctx)

	db := model.GetDB()
	if db == nil {
		return nil, model.ErrDBNotInitialized
	}

	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, gorm.ErrRecordNotFound
	}

	var session tables.AISessionTable
	err := db.WithContext(ctx).Where("session_id = ?", sessionID).First(&session).Error
	if err == nil {
		if session.Type == tables.AISessionTypeCodex {
			resolved, resolveErr := s.ResolveCodexSessionBySessionID(ctx, sessionID)
			if resolveErr != nil {
				return nil, resolveErr
			}
			return s.getConversationFromSession(ctx, *resolved)
		}
		return s.getConversationFromSession(ctx, session)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	resolved, resolveErr := s.ResolveCodexSessionBySessionID(ctx, sessionID)
	if resolveErr != nil {
		return nil, resolveErr
	}
	return s.getConversationFromSession(ctx, *resolved)
}

func (s *AISessionService) GetSessionConversationWindow(
	ctx context.Context,
	dbID string,
	beforeCursor string,
	limit int,
) (*ConversationWindowResponse, error) {
	return s.getConversationWindowByQuery(ctx, "id = ?", []interface{}{dbID}, beforeCursor, limit)
}

func (s *AISessionService) GetSessionConversationWindowBySessionID(
	ctx context.Context,
	sessionID string,
	beforeCursor string,
	limit int,
) (*ConversationWindowResponse, error) {
	ctx = ensureContext(ctx)

	db := model.GetDB()
	if db == nil {
		return nil, model.ErrDBNotInitialized
	}

	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, gorm.ErrRecordNotFound
	}

	var session tables.AISessionTable
	err := db.WithContext(ctx).Where("session_id = ?", sessionID).First(&session).Error
	if err == nil {
		if session.Type == tables.AISessionTypeCodex {
			resolved, resolveErr := s.ResolveCodexSessionBySessionID(ctx, sessionID)
			if resolveErr != nil {
				return nil, resolveErr
			}
			return s.getConversationWindowFromSession(ctx, *resolved, beforeCursor, limit)
		}
		return s.getConversationWindowFromSession(ctx, session, beforeCursor, limit)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	resolved, resolveErr := s.ResolveCodexSessionBySessionID(ctx, sessionID)
	if resolveErr != nil {
		return nil, resolveErr
	}
	return s.getConversationWindowFromSession(ctx, *resolved, beforeCursor, limit)
}

// RefreshSessionAndGetConversation clears the cached session data in DB and re-parses the file.
func (s *AISessionService) RefreshSessionAndGetConversation(ctx context.Context, dbID string) (*ConversationResponse, error) {
	ctx = ensureContext(ctx)
	logger := s.logger(ctx)

	db := model.GetDB()
	if db == nil {
		return nil, model.ErrDBNotInitialized
	}

	// Find the session in database
	var session tables.AISessionTable
	err := db.WithContext(ctx).Where("id = ?", dbID).First(&session).Error
	if err != nil {
		logger.Debug("session not found", zap.String("dbID", dbID), zap.Error(err))
		return nil, err
	}

	// Save file path and type before deleting
	filePath := session.FilePath
	sessionType := session.Type
	sessionID := session.SessionID

	// Delete the cached record to force re-parse
	if err := db.WithContext(ctx).Delete(&session).Error; err != nil {
		logger.Warn("failed to delete cached session", zap.String("dbID", dbID), zap.Error(err))
		return nil, err
	}

	logger.Info("deleted cached session for refresh",
		zap.String("dbID", dbID),
		zap.String("sessionId", sessionID),
		zap.String("type", string(sessionType)))

	// Clear directory cache as well
	dirCacheMu.Lock()
	for key := range dirCache {
		delete(dirCache, key)
	}
	dirCacheMu.Unlock()

	// Clear scan states
	scanStatesMu.Lock()
	for key := range scanStates {
		delete(scanStates, key)
	}
	scanStatesMu.Unlock()

	// Check file size - refuse to load files larger than 50MB
	const maxConversationFileSize = 50 * 1024 * 1024 // 50MB
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return nil, err
	}
	if fileInfo.Size() > maxConversationFileSize {
		return nil, fmt.Errorf("conversation file too large (%d MB), maximum is 50MB",
			fileInfo.Size()/(1024*1024))
	}

	// Parse the session file directly based on type
	var messages []*ConversationMessage
	var title string

	switch sessionType {
	case tables.AISessionTypeClaudeCode:
		messages, err = s.parseClaudeCodeConversation(filePath)
		if err == nil {
			// Get title from first user message
			for _, msg := range messages {
				if msg.Role == "user" && msg.Content != "" {
					title = msg.Content
					if len(title) > 100 {
						title = title[:100] + "..."
					}
					break
				}
			}
		}
	case tables.AISessionTypeCodex:
		messages, err = s.parseCodexConversation(filePath)
		if err == nil {
			// Get title from first user message
			for _, msg := range messages {
				if msg.Role == "user" && msg.Content != "" {
					title = msg.Content
					if len(title) > 100 {
						title = title[:100] + "..."
					}
					break
				}
			}
		}
	default:
		return nil, errors.New("unknown session type")
	}

	if err != nil {
		logger.Error("failed to parse conversation", zap.String("filePath", filePath), zap.Error(err))
		return nil, err
	}

	decorateConversationMessages(messages, sessionID)

	return &ConversationResponse{
		SessionID: sessionID,
		Title:     title,
		Messages:  messages,
	}, nil
}

func (s *AISessionService) RefreshSessionAndGetConversationWindow(
	ctx context.Context,
	dbID string,
	limit int,
) (*ConversationWindowResponse, error) {
	conversation, err := s.RefreshSessionAndGetConversation(ctx, dbID)
	if err != nil {
		return nil, err
	}
	options, err := normalizeConversationWindowOptions("", limit)
	if err != nil {
		return nil, err
	}
	return s.buildConversationWindowFromMessages(
		conversation.SessionID,
		conversation.Title,
		conversation.Messages,
		options,
	), nil
}

// getConversationByQuery retrieves conversation using a custom where clause.
func (s *AISessionService) getConversationByQuery(ctx context.Context, query string, args ...interface{}) (*ConversationResponse, error) {
	ctx = ensureContext(ctx)
	logger := s.logger(ctx)

	db := model.GetDB()
	if db == nil {
		return nil, model.ErrDBNotInitialized
	}

	// Find the session in database
	var session tables.AISessionTable
	err := db.WithContext(ctx).Where(query, args...).First(&session).Error
	if err != nil {
		logger.Debug("session not found", zap.String("query", query), zap.Error(err))
		return nil, err
	}

	return s.getConversationFromSession(ctx, session)
}

func (s *AISessionService) getConversationWindowByQuery(
	ctx context.Context,
	query string,
	args []interface{},
	beforeCursor string,
	limit int,
) (*ConversationWindowResponse, error) {
	ctx = ensureContext(ctx)
	logger := s.logger(ctx)

	db := model.GetDB()
	if db == nil {
		return nil, model.ErrDBNotInitialized
	}

	var session tables.AISessionTable
	err := db.WithContext(ctx).Where(query, args...).First(&session).Error
	if err != nil {
		logger.Debug("session not found", zap.String("query", query), zap.Error(err))
		return nil, err
	}

	return s.getConversationWindowFromSession(ctx, session, beforeCursor, limit)
}

func (s *AISessionService) getConversationFromSession(ctx context.Context, session tables.AISessionTable) (*ConversationResponse, error) {
	ctx = ensureContext(ctx)
	logger := s.logger(ctx)

	db := model.GetDB()
	if db == nil {
		return nil, model.ErrDBNotInitialized
	}

	// Check file size - refuse to load files larger than 50MB
	const maxConversationFileSize = 50 * 1024 * 1024 // 50MB
	fileInfo, err := os.Stat(session.FilePath)
	if err != nil {
		return nil, err
	}
	if fileInfo.Size() > maxConversationFileSize {
		return nil, fmt.Errorf("conversation file too large (%d MB), maximum is 50MB",
			fileInfo.Size()/(1024*1024))
	}

	// Parse the session file based on type
	var messages []*ConversationMessage

	switch session.Type {
	case tables.AISessionTypeClaudeCode:
		messages, err = s.parseClaudeCodeConversation(session.FilePath)
	case tables.AISessionTypeCodex:
		messages, err = s.parseCodexConversation(session.FilePath)
	default:
		return nil, errors.New("unknown session type")
	}

	if err != nil {
		logger.Error("failed to parse conversation", zap.String("filePath", session.FilePath), zap.Error(err))
		return nil, err
	}

	decorateConversationMessages(messages, session.SessionID)

	// Update message counts in database (opportunistic update when viewing conversation)
	var userCount, assistantCount int
	for _, msg := range messages {
		if msg.Role == "user" {
			userCount++
		} else if msg.Role == "assistant" {
			assistantCount++
		}
	}
	if userCount != session.MessageCount || assistantCount != session.AssistantMessageCount {
		db.WithContext(ctx).Model(&session).Updates(map[string]interface{}{
			"message_count":           userCount,
			"assistant_message_count": assistantCount,
		})
		// Also update in-memory cache
		dirCacheMu.Lock()
		for _, entry := range dirCache {
			for _, s := range entry.sessions {
				if s.ID == session.ID {
					s.MessageCount = userCount
					s.AssistantMessageCount = assistantCount
					break
				}
			}
		}
		dirCacheMu.Unlock()
	}

	return &ConversationResponse{
		SessionID: session.SessionID,
		Title:     session.Title,
		Messages:  messages,
	}, nil
}

func normalizeConversationWindowOptions(beforeCursor string, limit int) (conversationWindowOptions, error) {
	const (
		defaultLimit = 80
		maxLimit     = 120
	)

	opts := conversationWindowOptions{
		beforeCursor: -1,
		limit:        defaultLimit,
	}
	if limit > 0 {
		opts.limit = limit
	}
	if opts.limit > maxLimit {
		opts.limit = maxLimit
	}
	if strings.TrimSpace(beforeCursor) == "" {
		return opts, nil
	}

	value, err := strconv.Atoi(strings.TrimSpace(beforeCursor))
	if err != nil {
		return opts, fmt.Errorf("invalid conversation cursor")
	}
	if value < 0 {
		return opts, fmt.Errorf("invalid conversation cursor")
	}
	opts.beforeCursor = value
	return opts, nil
}

func clampConversationWindow(total int, options conversationWindowOptions) (int, int) {
	if total <= 0 {
		return 0, 0
	}
	end := total
	if options.beforeCursor >= 0 && options.beforeCursor < total {
		end = options.beforeCursor
	}
	if end < 0 {
		end = 0
	}
	start := end - options.limit
	if start < 0 {
		start = 0
	}
	if start > end {
		start = end
	}
	return start, end
}

func conversationBeforeCursor(start int) string {
	if start <= 0 {
		return ""
	}
	return strconv.Itoa(start)
}

func countConversationUserMessages(messages []*ConversationMessage) int {
	count := 0
	for _, msg := range messages {
		if msg != nil && msg.Role == "user" {
			count++
		}
	}
	return count
}

func conversationTitleFromMessages(messages []*ConversationMessage, fallback string) string {
	if strings.TrimSpace(fallback) != "" {
		return fallback
	}
	for _, msg := range messages {
		if msg == nil || msg.Role != "user" {
			continue
		}
		title := strings.TrimSpace(msg.Content)
		if title == "" {
			continue
		}
		if len(title) > 100 {
			title = title[:100] + "..."
		}
		return title
	}
	return ""
}

func (s *AISessionService) buildConversationWindowFromMessages(
	sessionID string,
	title string,
	messages []*ConversationMessage,
	options conversationWindowOptions,
) *ConversationWindowResponse {
	total := len(messages)
	start, end := clampConversationWindow(total, options)
	windowMessages := append([]*ConversationMessage(nil), messages[start:end]...)
	decorateConversationMessages(messages, sessionID)
	totalUserMessages := countConversationUserMessages(messages)
	userMessagesBeforeWindow := countConversationUserMessages(messages[:start])

	return &ConversationWindowResponse{
		SessionID:                sessionID,
		Title:                    conversationTitleFromMessages(messages, title),
		Messages:                 windowMessages,
		Total:                    total,
		TotalUserMessages:        totalUserMessages,
		UserMessagesBeforeWindow: userMessagesBeforeWindow,
		WindowStart:              start,
		WindowEnd:                end,
		HasMoreBefore:            start > 0,
		BeforeCursor:             conversationBeforeCursor(start),
	}
}

func (s *AISessionService) getConversationWindowFromSession(
	ctx context.Context,
	session tables.AISessionTable,
	beforeCursor string,
	limit int,
) (*ConversationWindowResponse, error) {
	ctx = ensureContext(ctx)
	logger := s.logger(ctx)

	db := model.GetDB()
	if db == nil {
		return nil, model.ErrDBNotInitialized
	}

	options, err := normalizeConversationWindowOptions(beforeCursor, limit)
	if err != nil {
		return nil, err
	}

	const maxConversationFileSize = 50 * 1024 * 1024
	fileInfo, err := os.Stat(session.FilePath)
	if err != nil {
		return nil, err
	}
	if fileInfo.Size() > maxConversationFileSize {
		return nil, fmt.Errorf("conversation file too large (%d MB), maximum is 50MB",
			fileInfo.Size()/(1024*1024))
	}

	var messages []*ConversationMessage
	switch session.Type {
	case tables.AISessionTypeClaudeCode:
		messages, err = s.parseClaudeCodeConversation(session.FilePath)
	case tables.AISessionTypeCodex:
		messages, err = s.parseCodexConversation(session.FilePath)
	default:
		return nil, errors.New("unknown session type")
	}
	if err != nil {
		logger.Error("failed to parse conversation", zap.String("filePath", session.FilePath), zap.Error(err))
		return nil, err
	}

	var userCount, assistantCount int
	for _, msg := range messages {
		if msg.Role == "user" {
			userCount++
		} else if msg.Role == "assistant" {
			assistantCount++
		}
	}
	if userCount != session.MessageCount || assistantCount != session.AssistantMessageCount {
		db.WithContext(ctx).Model(&session).Updates(map[string]interface{}{
			"message_count":           userCount,
			"assistant_message_count": assistantCount,
		})
		dirCacheMu.Lock()
		for _, entry := range dirCache {
			for _, cachedSession := range entry.sessions {
				if cachedSession.ID == session.ID {
					cachedSession.MessageCount = userCount
					cachedSession.AssistantMessageCount = assistantCount
					break
				}
			}
		}
		dirCacheMu.Unlock()
	}

	return s.buildConversationWindowFromMessages(session.SessionID, session.Title, messages, options), nil
}

// parseClaudeCodeConversation parses a Claude Code session file and extracts messages.
func (s *AISessionService) parseClaudeCodeConversation(filePath string) ([]*ConversationMessage, error) {
	return s.parseClaudeCodeConversationWithOptions(filePath, conversationParseOptions{})
}

func (s *AISessionService) parseClaudeCodeConversationWithOptions(filePath string, options conversationParseOptions) ([]*ConversationMessage, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var messages []*ConversationMessage
	messageSink := &messages
	if options.onMessage != nil {
		messageSink = nil
	}
	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 2*1024*1024) // 2MB buffer for large lines

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		// Parse the entry
		var entry struct {
			Type      string          `json:"type"`
			Message   json.RawMessage `json:"message"`
			Timestamp string          `json:"timestamp"`
			IsMeta    bool            `json:"isMeta"`
		}

		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		ts, _ := time.Parse(time.RFC3339, entry.Timestamp)

		// Parse message payload
		var msgContent struct {
			Role    string      `json:"role"`
			Content interface{} `json:"content"`
		}
		if err := json.Unmarshal(entry.Message, &msgContent); err != nil {
			continue
		}

		switch entry.Type {
		case "user":
			if entry.IsMeta || msgContent.Role != "user" {
				continue
			}

			// Normal user input is a string; tool results are typically array content.
			switch content := msgContent.Content.(type) {
			case string:
				text := strings.TrimSpace(content)
				if text == "" || strings.HasPrefix(text, "<command-") || strings.HasPrefix(text, "<local-command") {
					continue
				}
				if err := emitConversationMessage(messageSink, &ConversationMessage{
					Role:      "user",
					Content:   text,
					Timestamp: ts,
				}, options); err != nil {
					if errors.Is(err, errStopConversationScan) {
						return messages, nil
					}
					return nil, err
				}
			case []interface{}:
				// User messages with images can also be array blocks; tool results are usually tool_result blocks.
				if hasClaudeToolResultBlock(content) {
					if err := emitClaudeToolResultMessages(messageSink, content, ts, options); err != nil {
						if errors.Is(err, errStopConversationScan) {
							return messages, nil
						}
						return nil, err
					}
					continue
				}

				text, images := extractClaudeMessageContent(content, filePath, options.includeImageSources)
				text = strings.TrimSpace(text)
				if text == "" && len(images) == 0 {
					continue
				}
				if text != "" && (strings.HasPrefix(text, "<command-") || strings.HasPrefix(text, "<local-command")) {
					continue
				}
				if err := emitConversationMessage(messageSink, &ConversationMessage{
					Role:      "user",
					Content:   text,
					Timestamp: ts,
					Images:    images,
				}, options); err != nil {
					if errors.Is(err, errStopConversationScan) {
						return messages, nil
					}
					return nil, err
				}
			}
		case "assistant":
			if msgContent.Role != "assistant" {
				continue
			}
			text, images := extractClaudeMixedContent(msgContent.Content, filePath, options.includeImageSources)
			text = strings.TrimSpace(text)
			if text == "" && len(images) == 0 {
				continue
			}
			if err := emitConversationMessage(messageSink, &ConversationMessage{
				Role:      "assistant",
				Content:   text,
				Timestamp: ts,
				Images:    images,
			}, options); err != nil {
				if errors.Is(err, errStopConversationScan) {
					return messages, nil
				}
				return nil, err
			}
		}
	}

	return messages, scanner.Err()
}

func hasClaudeToolResultBlock(blocks []interface{}) bool {
	for _, block := range blocks {
		blockMap, ok := block.(map[string]interface{})
		if !ok {
			continue
		}
		if t, _ := blockMap["type"].(string); t == "tool_result" {
			return true
		}
	}
	return false
}

func emitClaudeToolResultMessages(dst *[]*ConversationMessage, blocks []interface{}, ts time.Time, options conversationParseOptions) error {
	for _, block := range blocks {
		blockMap, ok := block.(map[string]interface{})
		if !ok {
			continue
		}
		if t, _ := blockMap["type"].(string); t != "tool_result" {
			continue
		}

		preview, toolUseID, truncated := formatClaudeToolResult(blockMap, true)
		preview = strings.TrimSpace(preview)
		if preview == "" {
			continue
		}

		if err := emitConversationMessage(dst, &ConversationMessage{
			Role:      "assistant",
			Kind:      "tool_result",
			ToolUseID: toolUseID,
			HasMore:   truncated,
			Content:   preview,
			Timestamp: ts,
		}, options); err != nil {
			return err
		}
	}
	return nil
}

func renderClaudeContentBlocks(content interface{}) string {
	text, _ := extractClaudeMixedContent(content, "", false)
	return text
}

func extractClaudeMixedContent(content interface{}, sessionFilePath string, includeImageSources bool) (string, []ConversationImageAttachment) {
	switch v := content.(type) {
	case string:
		return v, nil
	case []interface{}:
		return extractClaudeMessageContent(v, sessionFilePath, includeImageSources)
	case map[string]interface{}:
		// Some tools embed both cleaned and raw content; prefer cleaned when present.
		if cleaned, ok := v["cleanedText"].(string); ok && cleaned != "" {
			return cleaned, nil
		}
		if text, ok := v["text"].(string); ok && text != "" {
			return text, nil
		}
		if raw, ok := v["rawContent"].(string); ok && raw != "" {
			return raw, nil
		}
		if b, err := json.Marshal(v); err == nil {
			return string(b), nil
		}
		return "", nil
	default:
		return "", nil
	}
}

func extractClaudeMessageContent(blocks []interface{}, sessionFilePath string, includeImageSources bool) (string, []ConversationImageAttachment) {
	parts := make([]string, 0, len(blocks))
	images := make([]ConversationImageAttachment, 0)
	for _, block := range blocks {
		blockMap, ok := block.(map[string]interface{})
		if !ok {
			continue
		}

		blockType, _ := blockMap["type"].(string)
		switch blockType {
		case "text":
			if text, ok := blockMap["text"].(string); ok && text != "" {
				parts = append(parts, text)
			}
		case "tool_use":
			if rendered := formatClaudeToolUseBlock(blockMap); rendered != "" {
				parts = append(parts, rendered)
			}
		case "image", "input_image":
			if attachment, ok := buildClaudeImageAttachment(blockMap, sessionFilePath, len(images), includeImageSources); ok {
				images = append(images, attachment)
			}
		}
	}
	return strings.Join(parts, "\n"), images
}

func formatClaudeToolUseBlock(block map[string]interface{}) string {
	name, _ := block["name"].(string)
	if name == "" {
		return "[tool_use]"
	}

	inputMap, _ := block["input"].(map[string]interface{})
	if inputMap == nil {
		return "[" + name + "]"
	}

	switch name {
	case "Edit", "Write", "Read":
		if filePath, _ := inputMap["file_path"].(string); filePath != "" {
			return "[" + name + ": " + filePath + "]"
		}
	case "Bash":
		if cmd, _ := inputMap["command"].(string); cmd != "" {
			// Keep it readable in the conversation view.
			if len(cmd) > 200 {
				cmd = cmd[:200] + "..."
			}
			return "[Bash: " + cmd + "]"
		}
	case "Grep", "Glob":
		if pattern, _ := inputMap["pattern"].(string); pattern != "" {
			return "[" + name + ": " + pattern + "]"
		}
	case "TodoWrite":
		return "[TodoWrite]"
	case "Task":
		if desc, _ := inputMap["description"].(string); desc != "" {
			return "[Task: " + desc + "]"
		}
	}

	return "[" + name + "]"
}

func formatClaudeToolResult(block map[string]interface{}, preview bool) (string, string, bool) {
	toolUseID, _ := block["tool_use_id"].(string)
	isError, _ := block["is_error"].(bool)

	header := "[Tool result]"
	if toolUseID != "" {
		header = "[Tool result: " + toolUseID + "]"
	}
	if isError {
		header += " (error)"
	}

	content, ok := block["content"]
	if !ok || content == nil {
		return header, toolUseID, false
	}

	body := strings.TrimSpace(renderClaudeContentBlocks(content))
	if body == "" {
		return header, toolUseID, false
	}

	full := header + "\n" + body
	if !preview {
		return full, toolUseID, false
	}

	const (
		maxPreviewChars = 1200
		maxPreviewLines = 25
	)
	truncatedText, truncated := truncateText(full, maxPreviewChars, maxPreviewLines)
	return truncatedText, toolUseID, truncated
}

func truncateText(s string, maxChars, maxLines int) (string, bool) {
	if s == "" {
		return "", false
	}

	lines := strings.Split(s, "\n")
	truncated := false
	if maxLines > 0 && len(lines) > maxLines {
		lines = lines[:maxLines]
		truncated = true
	}

	out := strings.Join(lines, "\n")
	if maxChars > 0 && len(out) > maxChars {
		out = out[:maxChars]
		truncated = true
	}

	if truncated {
		out = strings.TrimRight(out, "\n")
		out += "\n\n…"
	}
	return out, truncated
}

// parseCodexConversation parses a Codex session file and extracts messages.
func (s *AISessionService) parseCodexConversation(filePath string) ([]*ConversationMessage, error) {
	return s.parseCodexConversationWithOptions(filePath, conversationParseOptions{})
}

func (s *AISessionService) parseCodexConversationWithOptions(filePath string, options conversationParseOptions) ([]*ConversationMessage, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var messages []*ConversationMessage
	messageSink := &messages
	if options.onMessage != nil {
		messageSink = nil
	}
	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 2*1024*1024) // 2MB buffer for large lines

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var entry log_watcher.LogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		ts, _ := time.Parse(time.RFC3339, entry.Timestamp)

		if entry.Type == "event_msg" {
			payload, ok := entry.Payload.(map[string]interface{})
			if !ok {
				continue
			}

			msgType, _ := payload["type"].(string)

			switch msgType {
			case "user_message":
				msg, _ := payload["message"].(string)
				images := extractCodexImageAttachments(payload, filePath, options.includeImageSources)
				if msg == "" && len(images) == 0 {
					continue
				}
				if err := emitConversationMessage(messageSink, &ConversationMessage{
					Role:      "user",
					Content:   msg,
					Timestamp: ts,
					Images:    images,
				}, options); err != nil {
					if errors.Is(err, errStopConversationScan) {
						return messages, nil
					}
					return nil, err
				}
			case "agent_message":
				// Codex uses "agent_message" for final responses
				msg, _ := payload["message"].(string)
				if msg == "" {
					continue
				}
				if err := emitConversationMessage(messageSink, &ConversationMessage{
					Role:      "assistant",
					Content:   msg,
					Timestamp: ts,
				}, options); err != nil {
					if errors.Is(err, errStopConversationScan) {
						return messages, nil
					}
					return nil, err
				}
			case "agent_reasoning":
				// Codex uses "agent_reasoning" for thinking/reasoning text
				text, _ := payload["text"].(string)
				if text == "" {
					continue
				}
				if err := emitConversationMessage(messageSink, &ConversationMessage{
					Role:      "assistant",
					Content:   text,
					Timestamp: ts,
				}, options); err != nil {
					if errors.Is(err, errStopConversationScan) {
						return messages, nil
					}
					return nil, err
				}
			}
		}
	}

	return messages, scanner.Err()
}

func decorateConversationMessages(messages []*ConversationMessage, sessionID string) {
	for messageIndex, msg := range messages {
		for imageIndex := range msg.Images {
			attachment := &msg.Images[imageIndex]
			attachment.ID = fmt.Sprintf("m%d-i%d", messageIndex, imageIndex)
			if attachment.Previewable {
				attachment.PreviewURL = fmt.Sprintf(
					"/api/v1/ai-sessions/by-session-id/%s/conversation/images/%s",
					url.PathEscape(sessionID),
					url.PathEscape(attachment.ID),
				)
			}
		}
	}
}

func extractCodexImageAttachments(payload map[string]interface{}, sessionFilePath string, includeImageSources bool) []ConversationImageAttachment {
	sources := make([]string, 0)
	sources = append(sources, extractStringSlice(payload["images"])...)
	sources = append(sources, extractStringSlice(payload["local_images"])...)
	if len(sources) == 0 {
		return nil
	}

	placeholders := extractCodexTextElementPlaceholders(payload["text_elements"])
	attachments := make([]ConversationImageAttachment, 0, len(sources))
	for index, source := range sources {
		attachment, ok := buildConversationImageAttachment(source, sessionFilePath, index, includeImageSources)
		if !ok {
			continue
		}
		if index < len(placeholders) && placeholders[index] != "" {
			attachment.Label = placeholders[index]
		}
		attachments = append(attachments, attachment)
	}
	if len(attachments) == 0 {
		return nil
	}
	return attachments
}

func extractStringSlice(raw interface{}) []string {
	items, ok := raw.([]interface{})
	if !ok || len(items) == 0 {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		value, _ := item.(string)
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func extractCodexTextElementPlaceholders(raw interface{}) []string {
	items, ok := raw.([]interface{})
	if !ok || len(items) == 0 {
		return nil
	}
	placeholders := make([]string, 0, len(items))
	for _, item := range items {
		entry, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		placeholder, _ := entry["placeholder"].(string)
		placeholder = strings.TrimSpace(placeholder)
		if placeholder != "" {
			placeholders = append(placeholders, placeholder)
		}
	}
	return placeholders
}

func buildClaudeImageAttachment(block map[string]interface{}, sessionFilePath string, index int, includeImageSources bool) (ConversationImageAttachment, bool) {
	if sourceMap, ok := block["source"].(map[string]interface{}); ok {
		sourceType, _ := sourceMap["type"].(string)
		switch sourceType {
		case "base64":
			mediaType, _ := sourceMap["media_type"].(string)
			data, _ := sourceMap["data"].(string)
			if data != "" {
				return buildConversationImageAttachment(fmt.Sprintf("data:%s;base64,%s", fallbackImageMimeType(mediaType), data), sessionFilePath, index, includeImageSources)
			}
		case "url":
			if value, _ := sourceMap["url"].(string); value != "" {
				return buildConversationImageAttachment(value, sessionFilePath, index, includeImageSources)
			}
		case "file":
			for _, key := range []string{"path", "file_path", "filename", "fileName"} {
				if value, _ := sourceMap[key].(string); value != "" {
					return buildConversationImageAttachment(value, sessionFilePath, index, includeImageSources)
				}
			}
		}
	}

	for _, key := range []string{"image_url", "url", "path", "file_path", "fileName", "filename"} {
		if value, _ := block[key].(string); value != "" {
			return buildConversationImageAttachment(value, sessionFilePath, index, includeImageSources)
		}
	}

	return ConversationImageAttachment{}, false
}

func buildConversationImageAttachment(source string, sessionFilePath string, index int, includeImageSources bool) (ConversationImageAttachment, bool) {
	normalized := normalizeConversationImageSource(source, sessionFilePath)
	if normalized == "" {
		return ConversationImageAttachment{}, false
	}

	previewable := isPreviewableConversationImageSource(normalized)
	mimeType := detectConversationImageMimeType(normalized)
	internalSource := ""
	if includeImageSources {
		internalSource = normalized
	}
	return ConversationImageAttachment{
		Label:       buildConversationImageLabel(normalized, index),
		Previewable: previewable,
		MimeType:    mimeType,
		Source:      internalSource,
	}, true
}

func normalizeConversationImageSource(source string, sessionFilePath string) string {
	trimmed := strings.TrimSpace(source)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "data:") {
		return trimmed
	}
	if looksLikeWindowsAbsPath(trimmed) {
		return filepath.Clean(trimmed)
	}
	parsed, err := url.Parse(trimmed)
	if err == nil && parsed.Scheme != "" {
		switch parsed.Scheme {
		case "file":
			if pathValue := filepath.FromSlash(parsed.Path); pathValue != "" {
				if parsed.Host != "" {
					return filepath.Clean(`\\` + parsed.Host + pathValue)
				}
				return filepath.Clean(pathValue)
			}
		case "http", "https":
			return trimmed
		}
	}
	if filepath.IsAbs(trimmed) {
		return filepath.Clean(trimmed)
	}
	if sessionFilePath == "" {
		return filepath.Clean(trimmed)
	}
	return filepath.Clean(filepath.Join(filepath.Dir(sessionFilePath), trimmed))
}

func isPreviewableConversationImageSource(source string) bool {
	trimmed := strings.TrimSpace(source)
	if trimmed == "" {
		return false
	}
	if strings.HasPrefix(trimmed, "data:") {
		return true
	}
	if looksLikeWindowsAbsPath(trimmed) {
		return true
	}
	parsed, err := url.Parse(trimmed)
	if err == nil && parsed.Scheme != "" {
		return parsed.Scheme == "file"
	}
	return true
}

func detectConversationImageMimeType(source string) string {
	if strings.HasPrefix(source, "data:") {
		meta := strings.TrimPrefix(source, "data:")
		if commaIndex := strings.Index(meta, ","); commaIndex >= 0 {
			meta = meta[:commaIndex]
		}
		mimeType := strings.Split(meta, ";")[0]
		return fallbackImageMimeType(mimeType)
	}
	if looksLikeWindowsAbsPath(source) {
		return fallbackImageMimeType(mime.TypeByExtension(strings.ToLower(filepath.Ext(source))))
	}
	if parsed, err := url.Parse(source); err == nil && parsed.Scheme != "" {
		if parsed.Scheme == "http" || parsed.Scheme == "https" {
			return fallbackImageMimeType(mime.TypeByExtension(strings.ToLower(path.Ext(parsed.Path))))
		}
	}
	return fallbackImageMimeType(mime.TypeByExtension(strings.ToLower(filepath.Ext(source))))
}

func fallbackImageMimeType(mimeType string) string {
	mimeType = strings.TrimSpace(mimeType)
	if mimeType == "" {
		return "image/png"
	}
	return mimeType
}

func looksLikeWindowsAbsPath(value string) bool {
	if len(value) < 3 {
		return false
	}
	if value[1] != ':' {
		return false
	}
	if value[2] != '\\' && value[2] != '/' {
		return false
	}
	first := value[0]
	return (first >= 'A' && first <= 'Z') || (first >= 'a' && first <= 'z')
}

func buildConversationImageLabel(source string, index int) string {
	if source == "" {
		return fmt.Sprintf("Image %d", index+1)
	}
	if strings.HasPrefix(source, "data:") {
		return fmt.Sprintf("Embedded image %d", index+1)
	}
	if parsed, err := url.Parse(source); err == nil && parsed.Scheme != "" {
		if parsed.Scheme == "http" || parsed.Scheme == "https" {
			name := path.Base(parsed.Path)
			if name == "" || name == "/" || name == "." {
				return fmt.Sprintf("Remote image %d", index+1)
			}
			return name
		}
	}
	name := filepath.Base(source)
	if name == "" || name == "." || name == string(filepath.Separator) {
		return fmt.Sprintf("Image %d", index+1)
	}
	return name
}

func parseConversationDataURI(source string) (string, []byte, error) {
	if !strings.HasPrefix(source, "data:") {
		return "", nil, errors.New("not a data URI")
	}
	body := strings.TrimPrefix(source, "data:")
	parts := strings.SplitN(body, ",", 2)
	if len(parts) != 2 {
		return "", nil, errors.New("invalid data URI")
	}
	meta := parts[0]
	payload := parts[1]
	mimeType := fallbackImageMimeType(strings.Split(meta, ";")[0])
	if strings.Contains(meta, ";base64") {
		decoded, err := base64.StdEncoding.DecodeString(payload)
		if err != nil {
			return "", nil, err
		}
		return mimeType, decoded, nil
	}
	decoded, err := url.QueryUnescape(payload)
	if err != nil {
		return "", nil, err
	}
	return mimeType, []byte(decoded), nil
}

func resolveConversationImagePreview(attachment ConversationImageAttachment) (*ConversationImagePreview, error) {
	if !attachment.Previewable {
		return nil, errors.New("image preview is not supported for this source")
	}
	if strings.HasPrefix(attachment.Source, "data:") {
		mimeType, data, err := parseConversationDataURI(attachment.Source)
		if err != nil {
			return nil, err
		}
		return &ConversationImagePreview{MimeType: mimeType, Data: data}, nil
	}
	filePath := strings.TrimSpace(attachment.Source)
	if filePath == "" {
		return nil, errors.New("image source is empty")
	}
	if _, err := os.Stat(filePath); err != nil {
		return nil, err
	}
	return &ConversationImagePreview{
		MimeType: detectConversationImageMimeType(filePath),
		FilePath: filePath,
	}, nil
}

func (s *AISessionService) GetConversationImagePreviewBySessionID(ctx context.Context, sessionID, attachmentID string) (*ConversationImagePreview, error) {
	ctx = ensureContext(ctx)

	db := model.GetDB()
	if db == nil {
		return nil, model.ErrDBNotInitialized
	}

	var session tables.AISessionTable
	if err := db.WithContext(ctx).Where("session_id = ?", sessionID).First(&session).Error; err != nil {
		return nil, err
	}

	targetMessageIndex, targetImageIndex, err := parseConversationAttachmentID(attachmentID)
	if err != nil {
		return nil, gorm.ErrRecordNotFound
	}

	var result *ConversationImagePreview
	currentMessageIndex := 0
	visitor := func(msg *ConversationMessage) bool {
		if currentMessageIndex == targetMessageIndex {
			if targetImageIndex >= 0 && targetImageIndex < len(msg.Images) {
				preview, resolveErr := resolveConversationImagePreview(msg.Images[targetImageIndex])
				if resolveErr == nil {
					result = preview
					return false
				}
			}
			return false
		}
		currentMessageIndex++
		return true
	}

	switch session.Type {
	case tables.AISessionTypeClaudeCode:
		_, err = s.parseClaudeCodeConversationWithOptions(session.FilePath, conversationParseOptions{
			includeImageSources: true,
			onMessage:           visitor,
		})
	case tables.AISessionTypeCodex:
		_, err = s.parseCodexConversationWithOptions(session.FilePath, conversationParseOptions{
			includeImageSources: true,
			onMessage:           visitor,
		})
	default:
		return nil, errors.New("unknown session type")
	}
	if err != nil {
		return nil, err
	}
	if result != nil {
		return result, nil
	}

	return nil, gorm.ErrRecordNotFound
}

func parseConversationAttachmentID(attachmentID string) (int, int, error) {
	var messageIndex, imageIndex int
	if _, err := fmt.Sscanf(attachmentID, "m%d-i%d", &messageIndex, &imageIndex); err != nil {
		return 0, 0, err
	}
	return messageIndex, imageIndex, nil
}

type ToolResultResponse struct {
	ToolUseID string `json:"toolUseId"`
	Content   string `json:"content"`
}

func (s *AISessionService) GetClaudeToolResult(ctx context.Context, dbID, toolUseID string) (*ToolResultResponse, error) {
	return s.getClaudeToolResultByQuery(ctx, "id = ?", []interface{}{dbID}, toolUseID)
}

func (s *AISessionService) GetClaudeToolResultBySessionID(ctx context.Context, sessionID, toolUseID string) (*ToolResultResponse, error) {
	return s.getClaudeToolResultByQuery(ctx, "session_id = ?", []interface{}{sessionID}, toolUseID)
}

func (s *AISessionService) getClaudeToolResultByQuery(ctx context.Context, query string, args []interface{}, toolUseID string) (*ToolResultResponse, error) {
	ctx = ensureContext(ctx)
	logger := s.logger(ctx)

	db := model.GetDB()
	if db == nil {
		return nil, model.ErrDBNotInitialized
	}

	var session tables.AISessionTable
	if err := db.WithContext(ctx).Where(query, args...).First(&session).Error; err != nil {
		return nil, err
	}
	if session.Type != tables.AISessionTypeClaudeCode {
		return nil, errors.New("tool result only supported for Claude Code sessions")
	}

	// Guard against huge files.
	const maxConversationFileSize = 50 * 1024 * 1024 // 50MB
	fileInfo, err := os.Stat(session.FilePath)
	if err != nil {
		return nil, err
	}
	if fileInfo.Size() > maxConversationFileSize {
		return nil, fmt.Errorf("conversation file too large (%d MB), maximum is 50MB",
			fileInfo.Size()/(1024*1024))
	}

	content, err := findClaudeToolResultInFile(session.FilePath, toolUseID)
	if err != nil {
		logger.Debug("failed to find tool result",
			zap.String("filePath", session.FilePath),
			zap.String("toolUseId", toolUseID),
			zap.Error(err))
		return nil, err
	}

	return &ToolResultResponse{
		ToolUseID: toolUseID,
		Content:   content,
	}, nil
}

func findClaudeToolResultInFile(filePath, toolUseID string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 2*1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var entry struct {
			Type    string          `json:"type"`
			Message json.RawMessage `json:"message"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry.Type != "user" {
			continue
		}

		var msgContent struct {
			Role    string      `json:"role"`
			Content interface{} `json:"content"`
		}
		if err := json.Unmarshal(entry.Message, &msgContent); err != nil {
			continue
		}
		if msgContent.Role != "user" {
			continue
		}

		blocks, ok := msgContent.Content.([]interface{})
		if !ok {
			continue
		}

		for _, block := range blocks {
			blockMap, ok := block.(map[string]interface{})
			if !ok {
				continue
			}
			if t, _ := blockMap["type"].(string); t != "tool_result" {
				continue
			}
			id, _ := blockMap["tool_use_id"].(string)
			if id != toolUseID {
				continue
			}

			full, _, _ := formatClaudeToolResult(blockMap, false)
			full = strings.TrimSpace(full)
			if full == "" {
				return "", errors.New("tool result content empty")
			}
			return full, nil
		}
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", errors.New("tool result not found")
}
