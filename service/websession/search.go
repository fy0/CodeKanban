package websession

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"code-kanban/model"
	"code-kanban/model/tables"
)

const (
	defaultSessionSearchScanLimit = 50
	maxSessionSearchScanLimit     = 100
)

var ErrInvalidSessionSearchCursor = errors.New("invalid session search cursor")

type sessionSearchCursor struct {
	ActivityAt time.Time `json:"activityAt"`
	ID         string    `json:"id"`
	Total      int       `json:"total"`
}

func encodeSessionSearchCursor(record tables.WebSessionTable, total int) string {
	payload, err := json.Marshal(sessionSearchCursor{
		ActivityAt: record.ActivityAt,
		ID:         record.ID,
		Total:      total,
	})
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(payload)
}

func decodeSessionSearchCursor(value string) (sessionSearchCursor, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return sessionSearchCursor{}, nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return sessionSearchCursor{}, ErrInvalidSessionSearchCursor
	}
	var cursor sessionSearchCursor
	if err := json.Unmarshal(payload, &cursor); err != nil ||
		cursor.ActivityAt.IsZero() ||
		strings.TrimSpace(cursor.ID) == "" ||
		cursor.Total < 0 {
		return sessionSearchCursor{}, ErrInvalidSessionSearchCursor
	}
	return cursor, nil
}

func normalizeSessionSearchProjectIDs(projectIDs []string) []string {
	normalized := make([]string, 0, len(projectIDs))
	seen := make(map[string]struct{}, len(projectIDs))
	for _, projectID := range projectIDs {
		projectID = strings.TrimSpace(projectID)
		if projectID == "" {
			continue
		}
		if _, ok := seen[projectID]; ok {
			continue
		}
		seen[projectID] = struct{}{}
		normalized = append(normalized, projectID)
	}
	return normalized
}

func (m *Manager) SearchSessionsChunk(
	ctx context.Context,
	projectIDs []string,
	searchQuery string,
	includeArchived bool,
	includeBody bool,
	cursorValue string,
	scanLimit int,
) (SessionSearchChunkResult, error) {
	db := model.GetDB()
	if db == nil {
		return SessionSearchChunkResult{}, model.ErrDBNotInitialized
	}

	projectIDs = normalizeSessionSearchProjectIDs(projectIDs)
	searchQuery = strings.ToLower(strings.TrimSpace(searchQuery))
	if len(projectIDs) == 0 || searchQuery == "" {
		return SessionSearchChunkResult{Items: []SessionSummary{}, Done: true}, nil
	}
	if scanLimit <= 0 {
		scanLimit = defaultSessionSearchScanLimit
	}
	if scanLimit > maxSessionSearchScanLimit {
		scanLimit = maxSessionSearchScanLimit
	}

	cursor, err := decodeSessionSearchCursor(cursorValue)
	if err != nil {
		return SessionSearchChunkResult{}, err
	}

	scope := db.WithContext(ctx).
		Model(&tables.WebSessionTable{}).
		Where("project_id IN ?", projectIDs)
	if !includeArchived {
		scope = scope.Where("archived_at IS NULL")
	}

	total := cursor.Total
	if cursor.ActivityAt.IsZero() {
		var count int64
		if err := scope.Count(&count).Error; err != nil {
			return SessionSearchChunkResult{}, err
		}
		total = int(count)
	}

	candidatesQuery := scope
	if !cursor.ActivityAt.IsZero() {
		candidatesQuery = candidatesQuery.Where(
			"activity_at < ? OR (activity_at = ? AND id < ?)",
			cursor.ActivityAt,
			cursor.ActivityAt,
			cursor.ID,
		)
	}

	var candidates []tables.WebSessionTable
	if err := candidatesQuery.
		Order("activity_at DESC").
		Order("id DESC").
		Limit(scanLimit + 1).
		Find(&candidates).Error; err != nil {
		return SessionSearchChunkResult{}, err
	}

	hasMore := len(candidates) > scanLimit
	if hasMore {
		candidates = candidates[:scanLimit]
	}
	result := SessionSearchChunkResult{
		Items:   []SessionSummary{},
		Done:    !hasMore,
		Scanned: len(candidates),
		Total:   total,
	}
	if len(candidates) == 0 {
		result.Done = true
		return result, nil
	}
	if hasMore {
		result.NextCursor = encodeSessionSearchCursor(candidates[len(candidates)-1], total)
	}

	matchSources := make(map[string]map[SessionSearchMatchSource]struct{}, len(candidates))
	addMatchSource := func(sessionID string, source SessionSearchMatchSource) {
		sources := matchSources[sessionID]
		if sources == nil {
			sources = make(map[SessionSearchMatchSource]struct{}, 2)
			matchSources[sessionID] = sources
		}
		sources[source] = struct{}{}
	}
	bodyCandidateIDs := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if strings.Contains(strings.ToLower(candidate.Title), searchQuery) {
			addMatchSource(candidate.ID, SessionSearchMatchTitle)
		}
		bodyMatched := includeBody &&
			candidate.ThreadPreview != nil &&
			strings.Contains(strings.ToLower(*candidate.ThreadPreview), searchQuery)
		if bodyMatched {
			addMatchSource(candidate.ID, SessionSearchMatchBody)
		}
		if includeBody && !bodyMatched {
			bodyCandidateIDs = append(bodyCandidateIDs, candidate.ID)
		}
	}

	if includeBody && len(bodyCandidateIDs) > 0 {
		var rows []struct {
			WebSessionID string
		}
		if err := db.WithContext(ctx).
			Model(&tables.WebSessionItemTable{}).
			Select("web_session_id").
			Where("web_session_id IN ?", bodyCandidateIDs).
			Where("instr(lower(coalesce(text, '')), ?) > 0", searchQuery).
			Group("web_session_id").
			Scan(&rows).Error; err != nil {
			return SessionSearchChunkResult{}, err
		}
		for _, row := range rows {
			addMatchSource(row.WebSessionID, SessionSearchMatchBody)
		}
	}

	for _, candidate := range candidates {
		sources := matchSources[candidate.ID]
		if len(sources) == 0 {
			continue
		}
		summary := m.mapSessionSummary(candidate)
		if _, ok := sources[SessionSearchMatchTitle]; ok {
			summary.SearchMatchSources = append(summary.SearchMatchSources, SessionSearchMatchTitle)
		}
		if _, ok := sources[SessionSearchMatchBody]; ok {
			summary.SearchMatchSources = append(summary.SearchMatchSources, SessionSearchMatchBody)
		}
		result.Items = append(result.Items, summary)
	}
	return result, nil
}
