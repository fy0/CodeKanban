package service

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"code-kanban/model"
	"code-kanban/model/tables"
	"code-kanban/utils"
	"code-kanban/utils/ai_assistant2/log_watcher"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

const piSessionDiscoveryBatchSize = 256

var errPiSessionDiscoveryBatchFull = errors.New("Pi session discovery batch is full")

type piSessionEntry struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	ParentID  *string         `json:"parentId"`
	Timestamp string          `json:"timestamp"`
	Provider  string          `json:"provider"`
	ModelID   string          `json:"modelId"`
	Name      *string         `json:"name"`
	Message   json.RawMessage `json:"message"`
}

type piSessionMessage struct {
	Role      string          `json:"role"`
	Content   json.RawMessage `json:"content"`
	Provider  string          `json:"provider"`
	Model     string          `json:"model"`
	Timestamp int64           `json:"timestamp"`
}

type piSessionData struct {
	SessionID             string
	Cwd                   string
	Model                 string
	Title                 string
	StartedAt             time.Time
	LastMessageAt         *time.Time
	MessageCount          int
	AssistantMessageCount int
	hasModelChange        bool
	activePath            []*piSessionEntry
}

type piSessionFileCandidate struct {
	path   string
	info   os.FileInfo
	header log_watcher.PiSessionHeader
}

func (s *AISessionService) getPiSessionsPhased(
	ctx context.Context,
	projectPath string,
) ([]*AISessionSummary, string, error) {
	root, err := log_watcher.ResolvePiSessionDir()
	if err != nil {
		return nil, ScanPhaseComplete, err
	}
	if _, err := os.Stat(root); errors.Is(err, os.ErrNotExist) {
		return nil, ScanPhaseComplete, nil
	} else if err != nil {
		return nil, ScanPhaseComplete, err
	}

	if sessions, phase, scanning := piScanSnapshot(projectPath, root); scanning {
		s.queuePiBackgroundScan(ctx, projectPath, root)
		return dedupeAISessionSummariesBySessionID(sessions), phase, nil
	}

	cacheKey := piSessionCacheKey(projectPath, root)
	dirCacheMu.RLock()
	cached, hasCached := dirCache[cacheKey]
	dirCacheMu.RUnlock()
	if hasCached && time.Since(cached.cachedAt) < dirCacheTTL {
		cachedSessions := append([]*AISessionSummary(nil), cached.sessions...)
		phase := s.getPiScanPhase(projectPath, root)
		if phase != ScanPhaseComplete {
			s.queuePiBackgroundScan(ctx, projectPath, root)
		}
		return dedupeAISessionSummariesBySessionID(cachedSessions), phase, nil
	}

	db := model.GetDB()
	if db == nil {
		return nil, ScanPhaseComplete, model.ErrDBNotInitialized
	}
	var cachedRows []tables.AISessionTable
	if err := db.WithContext(ctx).
		Where("type = ?", tables.AISessionTypePi).
		Find(&cachedRows).Error; err != nil {
		return nil, ScanPhaseComplete, err
	}
	cachedByID := make(map[string]tables.AISessionTable, len(cachedRows))
	for _, row := range cachedRows {
		cachedByID[row.SessionID] = row
	}

	projectComparable := canonicalAIProjectPath(projectPath)
	candidates, nextCursor, hasMore, err := listPiSessionCandidateBatch(ctx, root, projectComparable, "")
	if err != nil {
		return nil, ScanPhaseComplete, err
	}

	now := time.Now()
	sessions := make([]*AISessionSummary, 0, len(candidates))
	pendingFiles := make([]string, 0)
	for _, candidate := range candidates {
		if row, ok := cachedByID[candidate.header.ID]; ok &&
			filepath.Clean(row.FilePath) == filepath.Clean(candidate.path) &&
			row.FileModTime.Equal(candidate.info.ModTime()) && row.FileSize == candidate.info.Size() {
			sessions = append(sessions, aiSessionSummaryFromRecord(row))
			continue
		}
		if now.Sub(candidate.info.ModTime()) > recentThreshold {
			pendingFiles = append(pendingFiles, candidate.path)
			continue
		}
		data, err := s.parsePiSessionFile(candidate.path)
		if err != nil {
			s.logger(ctx).Debug("failed to parse recent Pi session",
				zap.String("file", candidate.path),
				zap.Error(err))
			continue
		}
		session, err := s.savePiSession(ctx, db, candidate.path, candidate.info, data)
		if err == nil {
			sessions = append(sessions, session)
		}
	}
	sessions = dedupeAISessionSummariesBySessionID(sessions)
	sortAISessionSummaries(sessions)

	phase := ScanPhaseComplete
	stateKey := piScanStateKey(projectPath, root)
	if len(pendingFiles) > 0 || hasMore {
		phase = ScanPhaseRecent
		scanStatesMu.Lock()
		state, exists := scanStates[stateKey]
		if !exists {
			state = &scanState{}
			scanStates[stateKey] = state
		}
		state.mu.Lock()
		state.phase = ScanPhaseRecent
		state.sessions = append([]*AISessionSummary(nil), sessions...)
		state.pendingDirs = pendingFiles
		state.cursor = nextCursor
		state.hasMore = hasMore
		state.mu.Unlock()
		scanStatesMu.Unlock()
		s.queuePiBackgroundScan(ctx, projectPath, root)
	} else {
		scanStatesMu.Lock()
		if state, exists := scanStates[stateKey]; exists {
			state.mu.Lock()
			state.phase = ScanPhaseComplete
			state.pendingDirs = nil
			state.sessions = sessions
			state.cursor = ""
			state.hasMore = false
			state.mu.Unlock()
		}
		scanStatesMu.Unlock()
	}

	dirCacheMu.Lock()
	dirCache[cacheKey] = &dirCacheEntry{
		sessions: append([]*AISessionSummary(nil), sessions...),
		cachedAt: time.Now(),
	}
	dirCacheMu.Unlock()
	return sessions, phase, nil
}

func piScanSnapshot(projectPath, root string) ([]*AISessionSummary, string, bool) {
	scanStatesMu.RLock()
	state := scanStates[piScanStateKey(projectPath, root)]
	scanStatesMu.RUnlock()
	if state == nil {
		return nil, ScanPhaseComplete, false
	}
	state.mu.RLock()
	defer state.mu.RUnlock()
	if state.phase == ScanPhaseComplete {
		return nil, ScanPhaseComplete, false
	}
	return append([]*AISessionSummary(nil), state.sessions...), state.phase, true
}

func (s *AISessionService) queuePiBackgroundScan(ctx context.Context, projectPath, root string) {
	stateKey := piScanStateKey(projectPath, root)
	scanStatesMu.RLock()
	state := scanStates[stateKey]
	scanStatesMu.RUnlock()
	if state == nil {
		return
	}
	state.mu.Lock()
	if state.phase == ScanPhaseComplete || state.backgroundActive {
		state.mu.Unlock()
		return
	}
	state.backgroundActive = true
	state.mu.Unlock()

	select {
	case bgScanQueue <- &bgScanTask{projectPath: projectPath, scanType: "pi", projectDir: root}:
	default:
		state.mu.Lock()
		state.backgroundActive = false
		state.mu.Unlock()
		s.logger(ctx).Debug("background scan queue full, deferring Pi scan")
	}
}

func (s *AISessionService) scanPiExtendedPhase(ctx context.Context, projectPath, root string) error {
	stateKey := piScanStateKey(projectPath, root)
	scanStatesMu.RLock()
	state, exists := scanStates[stateKey]
	scanStatesMu.RUnlock()
	if !exists {
		return nil
	}
	defer func() {
		state.mu.Lock()
		state.backgroundActive = false
		state.mu.Unlock()
	}()

	db := model.GetDB()
	if db == nil {
		return model.ErrDBNotInitialized
	}
	projectComparable := canonicalAIProjectPath(projectPath)

	state.mu.RLock()
	pendingFiles := append([]string(nil), state.pendingDirs...)
	cursor := state.cursor
	hasMore := state.hasMore
	state.mu.RUnlock()

	newSessions := make([]*AISessionSummary, 0, len(pendingFiles))
	for _, filePath := range pendingFiles {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		session, err := s.loadPiSessionCandidate(ctx, db, filePath, projectComparable)
		if err != nil {
			s.logger(ctx).Debug("failed to parse older Pi session",
				zap.String("file", filePath),
				zap.Error(err))
			continue
		}
		if session != nil {
			newSessions = append(newSessions, session)
		}
	}
	allSessions := updatePiScanProgress(state, newSessions, nil, cursor, hasMore)
	updatePiDirectoryCache(projectPath, root, allSessions)

	for hasMore {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		candidates, nextCursor, nextHasMore, err := listPiSessionCandidateBatch(
			ctx,
			root,
			projectComparable,
			cursor,
		)
		if err != nil {
			return err
		}
		batchSessions := make([]*AISessionSummary, 0, len(candidates))
		for _, candidate := range candidates {
			session, err := s.loadPiSessionCandidate(ctx, db, candidate.path, projectComparable)
			if err != nil {
				s.logger(ctx).Debug("failed to parse Pi discovery batch entry",
					zap.String("file", candidate.path),
					zap.Error(err))
				continue
			}
			if session != nil {
				batchSessions = append(batchSessions, session)
			}
		}
		cursor = nextCursor
		hasMore = nextHasMore
		allSessions = updatePiScanProgress(state, batchSessions, nil, cursor, hasMore)
		updatePiDirectoryCache(projectPath, root, allSessions)
		runtime.Gosched()
	}

	state.mu.Lock()
	state.phase = ScanPhaseComplete
	state.cursor = ""
	state.hasMore = false
	state.pendingDirs = nil
	allSessions = append([]*AISessionSummary(nil), state.sessions...)
	state.mu.Unlock()
	updatePiDirectoryCache(projectPath, root, allSessions)
	return nil
}

func (s *AISessionService) loadPiSessionCandidate(
	ctx context.Context,
	db *gorm.DB,
	filePath string,
	projectComparable string,
) (*AISessionSummary, error) {
	header, err := log_watcher.ReadPiSessionHeader(filePath)
	if err != nil || canonicalAIProjectPath(header.Cwd) != projectComparable {
		return nil, err
	}
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, err
	}
	var cached tables.AISessionTable
	if err := db.WithContext(ctx).
		Where("session_id = ? AND type = ?", header.ID, tables.AISessionTypePi).
		First(&cached).Error; err == nil &&
		filepath.Clean(cached.FilePath) == filepath.Clean(filePath) &&
		cached.FileModTime.Equal(info.ModTime()) && cached.FileSize == info.Size() {
		return aiSessionSummaryFromRecord(cached), nil
	}
	data, err := s.parsePiSessionFile(filePath)
	if err != nil {
		return nil, err
	}
	return s.savePiSession(ctx, db, filePath, info, data)
}

func updatePiScanProgress(
	state *scanState,
	newSessions []*AISessionSummary,
	pendingFiles []string,
	cursor string,
	hasMore bool,
) []*AISessionSummary {
	state.mu.Lock()
	state.sessions = dedupeAISessionSummariesBySessionID(append(state.sessions, newSessions...))
	sortAISessionSummaries(state.sessions)
	state.pendingDirs = pendingFiles
	state.cursor = cursor
	state.hasMore = hasMore
	if hasMore || len(pendingFiles) > 0 {
		state.phase = ScanPhaseExtended
	} else {
		state.phase = ScanPhaseComplete
	}
	allSessions := append([]*AISessionSummary(nil), state.sessions...)
	state.mu.Unlock()
	return allSessions
}

func updatePiDirectoryCache(projectPath, root string, sessions []*AISessionSummary) {
	dirCacheMu.Lock()
	dirCache[piSessionCacheKey(projectPath, root)] = &dirCacheEntry{
		sessions: append([]*AISessionSummary(nil), sessions...),
		cachedAt: time.Now(),
	}
	dirCacheMu.Unlock()
}

func (s *AISessionService) getPiScanPhase(projectPath, root string) string {
	scanStatesMu.RLock()
	state := scanStates[piScanStateKey(projectPath, root)]
	scanStatesMu.RUnlock()
	if state == nil {
		return ScanPhaseComplete
	}
	state.mu.RLock()
	defer state.mu.RUnlock()
	return state.phase
}

func (s *AISessionService) currentPiScanProgress(projectPath, fallbackPhase string) (string, string) {
	root, err := log_watcher.ResolvePiSessionDir()
	if err != nil {
		return fallbackPhase, ""
	}
	scanStatesMu.RLock()
	state := scanStates[piScanStateKey(projectPath, root)]
	scanStatesMu.RUnlock()
	if state == nil {
		return fallbackPhase, ""
	}
	state.mu.RLock()
	defer state.mu.RUnlock()
	if state.phase == ScanPhaseComplete || strings.TrimSpace(state.cursor) == "" {
		return state.phase, ""
	}
	return state.phase, base64.RawURLEncoding.EncodeToString([]byte(state.cursor))
}

func piSessionCacheKey(projectPath, root string) string {
	return canonicalAIProjectPath(projectPath) + ":pi:" + canonicalAIProjectPath(root)
}

func piScanStateKey(projectPath, root string) string {
	return piSessionCacheKey(projectPath, root) + ":scan"
}

func canonicalAIProjectPath(value string) string {
	value = filepath.Clean(filepath.FromSlash(strings.TrimSpace(value)))
	if absolute, err := filepath.Abs(value); err == nil {
		value = absolute
	}
	if resolved, err := filepath.EvalSymlinks(value); err == nil && strings.TrimSpace(resolved) != "" {
		value = filepath.Clean(resolved)
	}
	if runtime.GOOS == "windows" {
		value = strings.ToLower(value)
	}
	return value
}

func listPiSessionCandidateBatch(
	ctx context.Context,
	root string,
	projectComparable string,
	afterCursor string,
) ([]piSessionFileCandidate, string, bool, error) {
	candidates := make([]piSessionFileCandidate, 0, piSessionDiscoveryBatchSize)
	visited := 0
	nextCursor := afterCursor
	root = filepath.Clean(root)
	afterCursor = filepath.Clean(afterCursor)
	if afterCursor == "." {
		afterCursor = ""
	}

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if walkErr != nil || entry == nil {
			return nil
		}
		if afterCursor != "" && path <= afterCursor {
			if entry.IsDir() && path != root &&
				!strings.HasPrefix(afterCursor, path+string(os.PathSeparator)) {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".jsonl") {
			return nil
		}

		visited++
		nextCursor = path
		header, err := log_watcher.ReadPiSessionHeader(path)
		if err == nil && canonicalAIProjectPath(header.Cwd) == projectComparable {
			if info, infoErr := entry.Info(); infoErr == nil {
				candidates = append(candidates, piSessionFileCandidate{path: path, info: info, header: header})
			}
		}
		if visited >= piSessionDiscoveryBatchSize {
			return errPiSessionDiscoveryBatchFull
		}
		return nil
	})
	switch {
	case errors.Is(err, errPiSessionDiscoveryBatchFull):
		return candidates, nextCursor, true, nil
	case errors.Is(err, os.ErrNotExist):
		return candidates, "", false, nil
	case err != nil:
		return candidates, nextCursor, false, err
	default:
		return candidates, "", false, nil
	}
}

func (s *AISessionService) parsePiSessionFile(filePath string) (*piSessionData, error) {
	header, err := log_watcher.ReadPiSessionHeader(filePath)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	entries := make([]*piSessionEntry, 0, 128)
	byID := make(map[string]*piSessionEntry)
	var leaf *piSessionEntry
	latestName := ""
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	lineIndex := 0
	for scanner.Scan() {
		lineIndex++
		if lineIndex == 1 || strings.TrimSpace(scanner.Text()) == "" {
			continue
		}
		entry := &piSessionEntry{}
		if json.Unmarshal(scanner.Bytes(), entry) != nil || strings.TrimSpace(entry.ID) == "" {
			continue
		}
		entries = append(entries, entry)
		byID[entry.ID] = entry
		leaf = entry
		if entry.Type == "session_info" && entry.Name != nil {
			latestName = strings.TrimSpace(*entry.Name)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	activePath := piActivePath(entries, byID, leaf)
	startedAt, _ := time.Parse(time.RFC3339Nano, header.Timestamp)
	if startedAt.IsZero() {
		if info, statErr := os.Stat(filePath); statErr == nil {
			startedAt = info.ModTime()
		}
	}
	data := &piSessionData{
		SessionID:  header.ID,
		Cwd:        filepath.Clean(header.Cwd),
		Title:      latestName,
		StartedAt:  startedAt,
		activePath: activePath,
	}
	for _, entry := range activePath {
		switch entry.Type {
		case "model_change":
			if model := canonicalPiModel(entry.Provider, entry.ModelID); model != "" {
				data.Model = model
				data.hasModelChange = true
			}
		case "message":
			message, ok := decodePiSessionMessage(entry.Message)
			if !ok {
				continue
			}
			ts := piEntryMessageTime(entry.Timestamp, message.Timestamp)
			switch message.Role {
			case "user":
				data.MessageCount++
				data.LastMessageAt = copyTimePointer(ts)
				if data.Title == "" {
					data.Title = truncateRunes(piSessionContentText(message.Content), 100)
				}
			case "assistant":
				data.AssistantMessageCount++
				data.LastMessageAt = copyTimePointer(ts)
				if model := canonicalPiModel(message.Provider, message.Model); !data.hasModelChange && model != "" {
					data.Model = model
				}
			}
		}
	}
	return data, nil
}

func piActivePath(
	entries []*piSessionEntry,
	byID map[string]*piSessionEntry,
	leaf *piSessionEntry,
) []*piSessionEntry {
	if leaf == nil {
		return nil
	}
	path := make([]*piSessionEntry, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for current := leaf; current != nil; {
		if _, exists := seen[current.ID]; exists {
			break
		}
		seen[current.ID] = struct{}{}
		path = append(path, current)
		if current.ParentID == nil || strings.TrimSpace(*current.ParentID) == "" {
			break
		}
		current = byID[*current.ParentID]
	}
	for left, right := 0, len(path)-1; left < right; left, right = left+1, right-1 {
		path[left], path[right] = path[right], path[left]
	}
	return path
}

func decodePiSessionMessage(raw json.RawMessage) (piSessionMessage, bool) {
	var message piSessionMessage
	if len(raw) == 0 || json.Unmarshal(raw, &message) != nil || strings.TrimSpace(message.Role) == "" {
		return piSessionMessage{}, false
	}
	return message, true
}

func piSessionContentText(raw json.RawMessage) string {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return strings.TrimSpace(text)
	}
	var blocks []struct {
		Type     string `json:"type"`
		Text     string `json:"text"`
		Thinking string `json:"thinking"`
	}
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if block.Type == "text" && strings.TrimSpace(block.Text) != "" {
			parts = append(parts, strings.TrimSpace(block.Text))
		}
	}
	return strings.Join(parts, "\n")
}

func canonicalPiModel(provider, modelID string) string {
	provider = strings.TrimSpace(provider)
	modelID = strings.TrimSpace(modelID)
	if provider == "" {
		return modelID
	}
	if modelID == "" {
		return provider
	}
	return provider + "/" + modelID
}

func piEntryMessageTime(entryTimestamp string, messageTimestamp int64) time.Time {
	if messageTimestamp > 0 {
		return time.UnixMilli(messageTimestamp)
	}
	ts, _ := time.Parse(time.RFC3339Nano, entryTimestamp)
	return ts
}

func copyTimePointer(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	copy := value
	return &copy
}

func truncateRunes(value string, limit int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if limit <= 0 || len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "..."
}

func sortAISessionSummaries(sessions []*AISessionSummary) {
	sort.Slice(sessions, func(i, j int) bool {
		left := sessions[i].LastMessageAt
		right := sessions[j].LastMessageAt
		if left != nil && right != nil && !left.Equal(*right) {
			return left.After(*right)
		}
		if left != nil && right == nil {
			return true
		}
		if left == nil && right != nil {
			return false
		}
		return sessions[i].SessionStartedAt.After(sessions[j].SessionStartedAt)
	})
}

func aiSessionSummaryFromRecord(record tables.AISessionTable) *AISessionSummary {
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
	}
}

func (s *AISessionService) savePiSession(
	ctx context.Context,
	db *gorm.DB,
	filePath string,
	fileInfo os.FileInfo,
	data *piSessionData,
) (*AISessionSummary, error) {
	var existing tables.AISessionTable
	err := db.WithContext(ctx).
		Where("session_id = ? AND type = ?", data.SessionID, tables.AISessionTypePi).
		First(&existing).Error
	now := time.Now()
	record := tables.AISessionTable{
		SessionID:             data.SessionID,
		Type:                  tables.AISessionTypePi,
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
		record.ID = existing.ID
		record.CreatedAt = existing.CreatedAt
		record.UpdatedAt = now
		if err := db.WithContext(ctx).Save(&record).Error; err != nil {
			return nil, err
		}
	} else if errors.Is(err, gorm.ErrRecordNotFound) {
		record.ID = utils.NewID()
		record.CreatedAt = now
		record.UpdatedAt = now
		if err := db.WithContext(ctx).Create(&record).Error; err != nil {
			return nil, err
		}
	} else {
		return nil, err
	}
	return aiSessionSummaryFromRecord(record), nil
}

func (s *AISessionService) ResolvePiSessionBySessionID(
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
	if err := db.WithContext(ctx).
		Where("session_id = ? AND type = ?", sessionID, tables.AISessionTypePi).
		First(&cached).Error; err == nil {
		if info, statErr := os.Stat(cached.FilePath); statErr == nil &&
			cached.FileModTime.Equal(info.ModTime()) && cached.FileSize == info.Size() {
			return &cached, nil
		}
	}

	root, err := log_watcher.ResolvePiSessionDir()
	if err != nil {
		return nil, err
	}
	searcher := log_watcher.NewPiFileSearcherWithSessionDir(root, "")
	filePath, err := searcher.FindBySessionID(ctx, sessionID)
	if err != nil || filePath == "" {
		if err == nil {
			err = gorm.ErrRecordNotFound
		}
		return nil, err
	}
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, err
	}
	data, err := s.parsePiSessionFile(filePath)
	if err != nil {
		return nil, err
	}
	if _, err := s.savePiSession(ctx, db, filePath, info, data); err != nil {
		return nil, err
	}
	var record tables.AISessionTable
	if err := db.WithContext(ctx).
		Where("session_id = ? AND type = ?", sessionID, tables.AISessionTypePi).
		First(&record).Error; err != nil {
		return nil, err
	}
	return &record, nil
}

func (s *AISessionService) ResolvePiSessionByID(ctx context.Context, dbID string) (*tables.AISessionTable, error) {
	db := model.GetDB()
	if db == nil {
		return nil, model.ErrDBNotInitialized
	}
	var record tables.AISessionTable
	if err := db.WithContext(ensureContext(ctx)).
		Where("id = ? AND type = ?", strings.TrimSpace(dbID), tables.AISessionTypePi).
		First(&record).Error; err != nil {
		return nil, err
	}
	return s.ResolvePiSessionBySessionID(ctx, record.SessionID)
}

func (s *AISessionService) parsePiConversation(filePath string) ([]*ConversationMessage, error) {
	data, err := s.parsePiSessionFile(filePath)
	if err != nil {
		return nil, err
	}
	messages := make([]*ConversationMessage, 0, data.MessageCount+data.AssistantMessageCount)
	for _, entry := range data.activePath {
		if entry.Type != "message" {
			continue
		}
		message, ok := decodePiSessionMessage(entry.Message)
		if !ok || (message.Role != "user" && message.Role != "assistant") {
			continue
		}
		content := piSessionContentText(message.Content)
		if content == "" {
			continue
		}
		messages = append(messages, &ConversationMessage{
			Role:      message.Role,
			Content:   content,
			Timestamp: piEntryMessageTime(entry.Timestamp, message.Timestamp),
		})
	}
	return messages, nil
}
