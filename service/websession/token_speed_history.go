package websession

import (
	"context"
	"sort"
	"time"

	"code-kanban/model"
	"code-kanban/model/tables"

	"go.uber.org/zap"
	"gorm.io/gorm/clause"
)

const tokenSpeedHistoryLimit = 20
const tokenSpeedRetainedHistory = 64

type TokenSpeedSummary struct {
	TokensPerSecond *float64 `json:"tokensPerSecond"`
	OutputTokens    float64  `json:"outputTokens"`
	DurationMs      int64    `json:"durationMs"`
	SampleCount     int64    `json:"sampleCount"`
	Estimated       bool     `json:"estimated"`
}

type TokenSpeedStats struct {
	CurrentRun TokenSpeedSummary `json:"currentRun"`
	Recent5    TokenSpeedSummary `json:"recent5"`
	Recent20   TokenSpeedSummary `json:"recent20"`
	Session    TokenSpeedSummary `json:"session"`
}

type TokenSpeedRecord struct {
	RunID           string    `json:"runId"`
	SampleID        string    `json:"sampleId"`
	Agent           string    `json:"agent"`
	Model           string    `json:"model"`
	StartedAt       time.Time `json:"startedAt"`
	EndedAt         time.Time `json:"endedAt"`
	OutputTokens    float64   `json:"outputTokens"`
	DurationMs      int64     `json:"durationMs"`
	TokensPerSecond float64   `json:"tokensPerSecond"`
	Estimated       bool      `json:"estimated"`
}

type tokenSpeedAccumulator struct {
	OutputTokens   float64
	DurationMs     int64
	SampleCount    int64
	EstimatedCount int64
}

func (a *tokenSpeedAccumulator) add(record TokenSpeedRecord, sign int64) {
	a.OutputTokens += float64(sign) * record.OutputTokens
	a.DurationMs += sign * record.DurationMs
	a.SampleCount += sign
	if record.Estimated {
		a.EstimatedCount += sign
	}
}

func (a tokenSpeedAccumulator) summary() TokenSpeedSummary {
	result := TokenSpeedSummary{OutputTokens: a.OutputTokens, DurationMs: a.DurationMs, SampleCount: a.SampleCount, Estimated: a.EstimatedCount > 0 || a.SampleCount == 0}
	if a.SampleCount > 0 && a.DurationMs > 0 {
		speed := a.OutputTokens * 1000 / float64(a.DurationMs)
		result.TokensPerSecond = &speed
	}
	return result
}

func (s *tokenSpeedState) measure(sample *tokenSpeedSample, end time.Time) *TokenSpeedRecord {
	durationMs := end.Sub(sample.started).Milliseconds()
	if durationMs < tokenSpeedInterval.Milliseconds() {
		return nil
	}
	var tokens float64
	for _, weight := range sample.weights {
		tokens += weight
	}
	estimated := true
	if sample.output != nil {
		tokens = float64(*sample.output)
		estimated = !sample.completeBoundary
	}
	if tokens <= 0 {
		return nil
	}
	return &TokenSpeedRecord{RunID: s.runID, SampleID: sample.id, Agent: s.agent, Model: s.model,
		StartedAt: sample.started, EndedAt: end, OutputTokens: tokens, DurationMs: durationMs,
		TokensPerSecond: tokens * 1000 / float64(durationMs), Estimated: estimated}
}

func (s *tokenSpeedState) recordSample(sample *tokenSpeedSample) {
	if sample.ended.IsZero() {
		return
	}
	record := s.measure(sample, sample.ended)
	if (record == nil && sample.record == nil) || (record != nil && sample.record != nil && *sample.record == *record) {
		return
	}
	if sample.record != nil {
		s.totals.add(*sample.record, -1)
		s.runTotals.add(*sample.record, -1)
	}
	if record == nil {
		// A versioned zero-output row prevents late estimates from undoing an
		// actual zero-output correction. It is excluded from saved statistics.
		empty := *sample.record
		empty.OutputTokens = 0
		empty.TokensPerSecond = 0
		empty.Estimated = false
		record = &empty
	} else {
		s.totals.add(*record, 1)
		s.runTotals.add(*record, 1)
	}
	sample.record = record
	if s.pendingRecords == nil {
		s.pendingRecords = make(map[string]tables.WebSessionTokenSpeedTable)
	}
	s.pendingRecords[record.RunID+":"+record.SampleID] = tables.WebSessionTokenSpeedTable{RunID: record.RunID, SampleID: record.SampleID,
		Agent: record.Agent, Model: record.Model, StartedAt: record.StartedAt, EndedAt: record.EndedAt,
		OutputTokens: record.OutputTokens, DurationMs: record.DurationMs, Estimated: record.Estimated, Version: s.version + 1}
	for i, previous := range s.history {
		if previous.RunID == record.RunID && previous.SampleID == record.SampleID {
			s.history = append(s.history[:i], s.history[i+1:]...)
			break
		}
	}
	if record.OutputTokens > 0 {
		s.history = append(s.history, *record)
	} else {
		sample.record = nil
	}
	sort.SliceStable(s.history, func(i, j int) bool { return s.history[i].EndedAt.After(s.history[j].EndedAt) })
	if len(s.history) > tokenSpeedRetainedHistory {
		s.history = s.history[:tokenSpeedRetainedHistory]
	}
}

func (s *tokenSpeedState) statistics() (TokenSpeedStats, []TokenSpeedRecord) {
	run, session := s.runTotals, s.totals
	if current := s.current(); current != nil && current.ended.IsZero() {
		if record := s.measure(current, s.updatedAt); record != nil {
			run.add(*record, 1)
			session.add(*record, 1)
		}
	}
	var recent5, recent20 tokenSpeedAccumulator
	for i, record := range s.history {
		if i >= tokenSpeedHistoryLimit {
			break
		}
		recent20.add(record, 1)
		if i < 5 {
			recent5.add(record, 1)
		}
	}
	history := append([]TokenSpeedRecord{}, s.history[:min(len(s.history), tokenSpeedHistoryLimit)]...)
	return TokenSpeedStats{CurrentRun: run.summary(), Recent5: recent5.summary(), Recent20: recent20.summary(), Session: session.summary()}, history
}

// Cold sessions load their totals once. Streaming deltas never query the ledger.
func (m *Manager) ensureTokenSpeedState(sessionID string) {
	m.tokenSpeedMu.Lock()
	loaded := m.tokenSpeeds[sessionID] != nil
	m.tokenSpeedMu.Unlock()
	if loaded {
		return
	}
	s := m.newTokenSpeedState(sessionID)
	m.tokenSpeedMu.Lock()
	defer m.tokenSpeedMu.Unlock()
	if m.tokenSpeeds == nil {
		m.tokenSpeeds = make(map[string]*tokenSpeedState)
	}
	if m.tokenSpeeds[sessionID] == nil {
		m.tokenSpeeds[sessionID] = s
	}
}

func (m *Manager) newTokenSpeedState(sessionID string) *tokenSpeedState {
	// Version zero is reserved for summaries before durable history is loaded.
	s := &tokenSpeedState{version: 1}
	db := model.GetReaderDB()
	if m.store == nil || db == nil {
		return s
	}
	if err := db.Model(&tables.WebSessionTokenSpeedTable{}).Where("web_session_id = ? AND output_tokens > 0 AND duration_ms >= ?", sessionID, tokenSpeedInterval.Milliseconds()).
		Select("COALESCE(SUM(output_tokens), 0) AS output_tokens, COALESCE(SUM(duration_ms), 0) AS duration_ms, COUNT(*) AS sample_count, COALESCE(SUM(CASE WHEN estimated THEN 1 ELSE 0 END), 0) AS estimated_count").
		Scan(&s.totals).Error; err != nil {
		m.logTokenSpeedStorageError(sessionID, err)
		return s
	}
	var rows []tables.WebSessionTokenSpeedTable
	if err := db.Where("web_session_id = ? AND output_tokens > 0 AND duration_ms >= ?", sessionID, tokenSpeedInterval.Milliseconds()).Order("ended_at DESC").Order("run_id DESC").Order("sample_id DESC").Limit(tokenSpeedRetainedHistory).Find(&rows).Error; err != nil {
		m.logTokenSpeedStorageError(sessionID, err)
		return s
	}
	for _, row := range rows {
		s.history = append(s.history, TokenSpeedRecord{RunID: row.RunID, SampleID: row.SampleID, Agent: row.Agent, Model: row.Model,
			StartedAt: row.StartedAt, EndedAt: row.EndedAt, OutputTokens: row.OutputTokens, DurationMs: row.DurationMs,
			TokensPerSecond: row.OutputTokens * 1000 / float64(row.DurationMs), Estimated: row.Estimated})
	}
	return s
}

func (s *tokenSpeedState) takePendingRecords(sessionID string) []tables.WebSessionTokenSpeedTable {
	rows := make([]tables.WebSessionTokenSpeedTable, 0, len(s.pendingRecords))
	for _, row := range s.pendingRecords {
		row.WebSessionID = sessionID
		rows = append(rows, row)
	}
	clear(s.pendingRecords)
	return rows
}

// Database I/O stays outside the telemetry lock. A revision guard makes
// out-of-order calibration writes idempotent.
func (m *Manager) persistTokenSpeedRecords(sessionID string, rows []tables.WebSessionTokenSpeedTable) {
	db := model.GetDB()
	if len(rows) == 0 || m.store == nil || db == nil {
		return
	}
	lock := &m.tokenSpeedStorageLocks[sessionRevisionLockIndex(sessionID)]
	lock.Lock()
	defer lock.Unlock()
	m.tokenSpeedMu.Lock()
	deleted := m.tokenSpeeds[sessionID] == nil || m.tokenSpeeds[sessionID].deleted
	m.tokenSpeedMu.Unlock()
	if deleted {
		return
	}
	if err := db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "web_session_id"}, {Name: "run_id"}, {Name: "sample_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"output_tokens", "estimated", "version"}),
		Where:     clause.Where{Exprs: []clause.Expression{clause.Expr{SQL: "web_session_token_speeds.version < excluded.version"}}}}).Create(&rows).Error; err != nil {
		m.logTokenSpeedStorageError(sessionID, err)
		m.tokenSpeedMu.Lock()
		if s := m.tokenSpeeds[sessionID]; s != nil && !s.deleted {
			if s.pendingRecords == nil {
				s.pendingRecords = make(map[string]tables.WebSessionTokenSpeedTable)
			}
			for _, row := range rows {
				key := row.RunID + ":" + row.SampleID
				if s.pendingRecords[key].Version < row.Version {
					s.pendingRecords[key] = row
				}
			}
		}
		m.tokenSpeedMu.Unlock()
		return
	}
}

func (m *Manager) deleteTokenSpeedRecords(ctx context.Context, sessionID string) error {
	lock := &m.tokenSpeedStorageLocks[sessionRevisionLockIndex(sessionID)]
	lock.Lock()
	defer lock.Unlock()
	m.tokenSpeedMu.Lock()
	var version uint64
	if s := m.tokenSpeeds[sessionID]; s != nil {
		version = s.version + 1
		if s.timer != nil {
			s.timer.Stop()
		}
	}
	if m.tokenSpeeds == nil {
		m.tokenSpeeds = make(map[string]*tokenSpeedState)
	}
	m.tokenSpeeds[sessionID] = &tokenSpeedState{version: version, stopped: true, deleted: true}
	m.tokenSpeedMu.Unlock()
	err := model.GetDB().WithContext(ctx).Where("web_session_id = ?", sessionID).Delete(&tables.WebSessionTokenSpeedTable{}).Error
	if err != nil {
		m.tokenSpeedMu.Lock()
		m.tokenSpeeds[sessionID].deleted = false
		m.tokenSpeedMu.Unlock()
	}
	return err
}

func (m *Manager) logTokenSpeedStorageError(sessionID string, err error) {
	if m.logger != nil {
		m.logger.Warn("token speed ledger access failed", zap.String("sessionId", sessionID), zap.Error(err))
	}
}
