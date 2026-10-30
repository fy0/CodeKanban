package websession

import (
	"strconv"
	"time"

	"code-kanban/model/tables"
	"code-kanban/utils"
)

const tokenSpeedInterval = 500 * time.Millisecond

// TokenSpeed has its own telemetry clock, independent of history revisions.
type TokenSpeed struct {
	Epoch           string             `json:"epoch"`
	Version         uint64             `json:"version"`
	RunID           string             `json:"runId"`
	SampleID        string             `json:"sampleId"`
	TokensPerSecond *float64           `json:"tokensPerSecond"`
	Estimated       bool               `json:"estimated"`
	Generating      bool               `json:"generating"`
	UpdatedAt       time.Time          `json:"updatedAt"`
	Stats           TokenSpeedStats    `json:"stats"`
	History         []TokenSpeedRecord `json:"history"`
}

type tokenSpeedSample struct {
	id                    string
	key                   string
	started               time.Time
	ended                 time.Time
	weights               map[string]float64
	output                *int64
	reportedOutput        *int64
	completeBoundary      bool
	usageClaimed          bool
	reasoningObserved     bool
	codexOutputAt         time.Time
	codexResponseComplete bool
	record                *TokenSpeedRecord
}

type tokenSpeedState struct {
	runID              string
	version            uint64
	serial             uint64
	samples            []*tokenSpeedSample
	stopped            bool
	timer              *time.Timer
	lastPublished      time.Time
	updatedAt          time.Time
	published          *TokenSpeed
	codexTotal         int64
	codexThreadID      string
	codexResponses     map[string]bool
	codexPendingOutput int64
	claudeMessageID    string
	devinAmbiguous     bool
	devinUsageSeen     map[string]bool
	agent              string
	model              string
	history            []TokenSpeedRecord
	totals             tokenSpeedAccumulator
	runTotals          tokenSpeedAccumulator
	pendingRecords     map[string]tables.WebSessionTokenSpeedTable
	deleted            bool
}

func estimateOutputTokens(text string) float64 {
	var count float64
	for _, ch := range text {
		if ch < 128 {
			count += .25
		} else {
			count++
		}
	}
	return count
}

func (s *tokenSpeedState) current() *tokenSpeedSample {
	if len(s.samples) == 0 {
		return nil
	}
	return s.samples[len(s.samples)-1]
}

func (s *tokenSpeedState) begin(key string, now time.Time, complete bool) *tokenSpeedSample {
	if s.stopped {
		return nil
	}
	if key != "" {
		for _, sample := range s.samples {
			if sample.key == key {
				return sample
			}
		}
	}
	if cur := s.current(); cur != nil {
		if key != "" && cur.key == key {
			return cur
		}
		if cur.ended.IsZero() {
			if key == "" {
				return cur
			}
			cur.ended = now
			cur.completeBoundary = false
			s.recordSample(cur)
		}
	}
	s.serial++
	cur := &tokenSpeedSample{id: strconv.FormatUint(s.serial, 10), key: key, started: now, weights: make(map[string]float64), completeBoundary: complete}
	s.samples = append(s.samples, cur)
	if len(s.samples) > 32 {
		s.samples = s.samples[len(s.samples)-32:]
	}
	return cur
}

func (s *tokenSpeedState) delta(key, channel, text string, now time.Time) {
	if text == "" || s.stopped {
		return
	}
	cur := s.current()
	if cur == nil || !cur.ended.IsZero() || (key != "" && cur.key != key) {
		cur = s.begin(key, now, false)
	}
	if cur != nil && cur.ended.IsZero() {
		cur.weights[channel] += estimateOutputTokens(text)
	}
}

func (s *tokenSpeedState) finish(key string, output *int64, now time.Time, complete bool) {
	cur := s.current()
	if key != "" {
		cur = nil
		for i := len(s.samples) - 1; i >= 0; i-- {
			if s.samples[i].key == key {
				cur = s.samples[i]
				break
			}
		}
	}
	s.finishSample(cur, output, now, complete)
}

func (s *tokenSpeedState) finishSample(cur *tokenSpeedSample, output *int64, now time.Time, complete bool) {
	if cur == nil {
		return
	}
	if cur.ended.IsZero() {
		cur.ended = now
		if !complete {
			cur.completeBoundary = false
		}
	}
	if output != nil && *output >= 0 && (!cur.usageClaimed || cur.output == nil || *output >= *cur.output) {
		value := *output
		cur.output = &value
		cur.usageClaimed = true
	}
	s.recordSample(cur)
}

func (s *tokenSpeedState) snapshot(epoch string) TokenSpeed {
	result := TokenSpeed{Epoch: epoch, Version: s.version, RunID: s.runID, Estimated: true, UpdatedAt: s.updatedAt}
	result.Stats, result.History = s.statistics()
	cur := s.current()
	if cur == nil {
		return result
	}
	result.SampleID = cur.id
	result.Generating = cur.ended.IsZero() && !s.stopped
	end := cur.ended
	if end.IsZero() {
		end = s.updatedAt
	}
	elapsed := end.Sub(cur.started)
	if elapsed < tokenSpeedInterval {
		return result
	}
	var tokens float64
	for _, weight := range cur.weights {
		tokens += weight
	}
	if cur.output != nil {
		tokens = float64(*cur.output)
		result.Estimated = !cur.completeBoundary
	}
	if tokens <= 0 {
		return result
	}
	speed := tokens / elapsed.Seconds()
	result.TokensPerSecond = &speed
	return result
}

// updateTokenSpeed never takes the manager's runtime lock while holding the
// telemetry lock. Publishing performs no database reads or revision updates.
func (m *Manager) updateTokenSpeed(sessionID, runID string, change func(*tokenSpeedState, time.Time), force bool) {
	if m == nil || sessionID == "" || runID == "" {
		return
	}
	now := time.Now()
	m.ensureTokenSpeedState(sessionID)
	m.tokenSpeedMu.Lock()
	if m.tokenSpeedEpoch == "" {
		m.tokenSpeedEpoch = utils.NewID()
	}
	if m.tokenSpeeds == nil {
		m.tokenSpeeds = make(map[string]*tokenSpeedState)
	}
	s := m.tokenSpeeds[sessionID]
	if s.runID == "" && !s.stopped {
		s.runID = runID
	}
	if s.deleted || s.runID != runID {
		m.tokenSpeedMu.Unlock()
		return
	}
	change(s, now)
	s.updatedAt = now
	s.version++
	rows := s.takePendingRecords(sessionID)
	publish := force || now.Sub(s.lastPublished) >= tokenSpeedInterval
	if publish {
		s.lastPublished = now
	}
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	if cur := s.current(); cur != nil && cur.ended.IsZero() && !s.stopped {
		s.timer = time.AfterFunc(tokenSpeedInterval, func() {
			m.updateTokenSpeed(sessionID, runID, func(*tokenSpeedState, time.Time) {}, false)
		})
	}
	snapshot := s.snapshot(m.tokenSpeedEpoch)
	if publish {
		s.published = &snapshot
	}
	m.tokenSpeedMu.Unlock()
	m.persistTokenSpeedRecords(sessionID, rows)
	if publish {
		m.publishTokenSpeed(sessionID, snapshot)
	}
}

func (m *Manager) publishTokenSpeed(sessionID string, snapshot TokenSpeed) {
	m.broadcastFrame(wireFrame{Version: protocolVersion, Kind: wireKindEvent, SessionID: sessionID,
		Timestamp: time.Now().UnixMilli(), Operation: wireOpTokenSpeed, Payload: snapshot})
}

func (m *Manager) tokenSpeedSnapshot(sessionID string) *TokenSpeed {
	m.tokenSpeedMu.Lock()
	defer m.tokenSpeedMu.Unlock()
	if m.tokenSpeedEpoch == "" {
		m.tokenSpeedEpoch = utils.NewID()
	}
	s := m.tokenSpeeds[sessionID]
	if s == nil {
		// Summary mapping also runs inside database transactions. Keep it
		// read-only; explicit snapshots and run events hydrate the ledger.
		value := (&tokenSpeedState{}).snapshot(m.tokenSpeedEpoch)
		return &value
	}
	if s.published != nil {
		value := *s.published
		return &value
	}
	value := s.snapshot(m.tokenSpeedEpoch)
	return &value
}

func (m *Manager) clearTokenSpeed(sessionID string) {
	m.tokenSpeedMu.Lock()
	var rows []tables.WebSessionTokenSpeedTable
	if s := m.tokenSpeeds[sessionID]; s != nil {
		if s.timer != nil {
			s.timer.Stop()
		}
		s.finish("", nil, time.Now(), false)
		rows = s.takePendingRecords(sessionID)
		m.tokenSpeeds[sessionID] = &tokenSpeedState{version: s.version + 1, stopped: true, updatedAt: time.Now(), history: s.history, totals: s.totals, deleted: s.deleted}
	}
	m.tokenSpeedMu.Unlock()
	m.persistTokenSpeedRecords(sessionID, rows)
	m.publishTokenSpeed(sessionID, *m.tokenSpeedSnapshot(sessionID))
}

func (m *Manager) observeTokenSpeedLifecycle(session tables.WebSessionTable, event Event) {
	if event.Type == "run_st" {
		m.ensureTokenSpeedState(session.ID)
		m.tokenSpeedMu.Lock()
		if m.tokenSpeeds == nil {
			m.tokenSpeeds = make(map[string]*tokenSpeedState)
		}
		old := m.tokenSpeeds[session.ID]
		if old.deleted {
			m.tokenSpeedMu.Unlock()
			return
		}
		if old.runID == event.RunID {
			m.tokenSpeedMu.Unlock()
			return
		}
		if old.timer != nil {
			old.timer.Stop()
		}
		old.finish("", nil, time.Now(), false)
		rows := old.takePendingRecords(session.ID)
		m.tokenSpeeds[session.ID] = &tokenSpeedState{runID: event.RunID, version: old.version, codexTotal: session.TotalOutputTokens,
			agent: session.Agent, model: session.Model, history: old.history, totals: old.totals, pendingRecords: old.pendingRecords}
		m.tokenSpeedMu.Unlock()
		m.persistTokenSpeedRecords(session.ID, rows)
		m.updateTokenSpeed(session.ID, event.RunID, func(*tokenSpeedState, time.Time) {}, true)
		return
	}
	terminal := isWorkTimingTerminalEvent(event.Type)
	pause := event.Type == "approval_req" || event.Type == "user_input_req" ||
		(event.Type == "note" && stringValue(event.Payload["code"]) == "transport_retrying")
	if terminal || pause {
		m.updateTokenSpeed(session.ID, event.RunID, func(s *tokenSpeedState, now time.Time) {
			s.finish("", nil, now, terminal && event.Type == "run_done")
			if terminal {
				s.stopped = true
			}
		}, true)
	}
}
