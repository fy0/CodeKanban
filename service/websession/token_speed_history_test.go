package websession

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"code-kanban/model"
	"code-kanban/model/tables"
)

func assertTokenSpeedSummary(t *testing.T, got TokenSpeedSummary, count int64, tokens float64, durationMs int64, estimated bool) {
	t.Helper()
	if got.SampleCount != count || got.OutputTokens != tokens || got.DurationMs != durationMs || got.Estimated != estimated {
		t.Fatalf("summary = %+v, want count=%d tokens=%v durationMs=%d estimated=%v", got, count, tokens, durationMs, estimated)
	}
	if count > 0 && (got.TokensPerSecond == nil || math.Abs(*got.TokensPerSecond-tokens*1000/float64(durationMs)) > .001) {
		t.Fatalf("incorrect weighted speed: %+v", got)
	}
}

func TestTokenSpeedHistoryWeightedWindowsAndRetention(t *testing.T) {
	start := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	s := &tokenSpeedState{runID: "run"}
	for i := 1; i <= 40; i++ {
		// Deliberately vary durations and leave large idle gaps.
		at := start.Add(time.Duration(i) * time.Minute)
		duration := time.Duration(i) * time.Second
		key := fmt.Sprint(i)
		s.begin(key, at, true)
		tokens := int64(100)
		s.finish(key, &tokens, at.Add(duration), true)
	}
	got := s.snapshot("epoch")
	assertTokenSpeedSummary(t, got.Stats.CurrentRun, 40, 4000, 820000, false)
	assertTokenSpeedSummary(t, got.Stats.Session, 40, 4000, 820000, false)
	assertTokenSpeedSummary(t, got.Stats.Recent5, 5, 500, 190000, false)
	assertTokenSpeedSummary(t, got.Stats.Recent20, 20, 2000, 610000, false)
	if len(s.samples) != 32 || len(got.History) != 20 || got.History[0].SampleID != "40" || got.History[19].SampleID != "21" {
		t.Fatalf("incorrect bounded history: %#v", got.History)
	}
	// Historical snapshots must not change when the live state is calibrated.
	tokens := int64(200)
	s.finish("40", &tokens, start.Add(time.Hour), true)
	assertTokenSpeedSummary(t, s.snapshot("epoch").Stats.Session, 40, 4100, 820000, false)
	if got.History[0].OutputTokens != 100 {
		t.Fatal("calibration mutated an already published history")
	}
}

func TestTokenSpeedHistoryActiveCalibrationAndZeroOutput(t *testing.T) {
	start := time.Now()
	s := &tokenSpeedState{runID: "run", updatedAt: start.Add(time.Second)}
	s.begin("first", start, true)
	s.delta("first", "text", "abcd", start)
	got := s.snapshot("e")
	assertTokenSpeedSummary(t, got.Stats.CurrentRun, 1, 1, 1000, true)
	if len(got.History) != 0 || got.Stats.Recent5.TokensPerSecond != nil {
		t.Fatal("active generation entered completed history")
	}
	s.finish("first", nil, start.Add(time.Second), true)
	tokens := int64(80)
	s.finish("first", &tokens, start.Add(time.Hour), true)
	s.finish("first", &tokens, start.Add(time.Hour), true)
	assertTokenSpeedSummary(t, s.snapshot("e").Stats.Session, 1, 80, 1000, false)
	// A real zero-token usage report should remove the original estimate.
	s.begin("empty", start.Add(time.Minute), true)
	s.delta("empty", "text", "abcd", start.Add(time.Minute))
	s.finish("empty", nil, start.Add(time.Minute+time.Second), true)
	zero := int64(0)
	s.finish("empty", &zero, start.Add(time.Hour), true)
	got = s.snapshot("e")
	assertTokenSpeedSummary(t, got.Stats.Session, 1, 80, 1000, false)
	if len(got.History) != 1 {
		t.Fatal("zero-output correction left a history entry")
	}
}

func TestTokenSpeedHistorySurvivesRunsRestartAndModelChange(t *testing.T) {
	m, session := newTextDeltaTestManager(t)
	session.Agent, session.Model = "claude", "first-model"
	m.observeTokenSpeedLifecycle(*session, Event{Type: "run_st", RunID: "first-run"})
	m.updateTokenSpeed(session.ID, "first-run", func(s *tokenSpeedState, now time.Time) {
		s.begin("request", now.Add(-time.Second), true)
		s.delta("request", "text", "abcd", now)
		s.finish("request", nil, now, true)
	}, true)
	// A delayed actual usage report calibrates the existing durable row.
	m.updateTokenSpeed(session.ID, "first-run", func(s *tokenSpeedState, now time.Time) {
		tokens := int64(80)
		s.finish("request", &tokens, now, true)
	}, true)
	m.observeTokenSpeedLifecycle(*session, Event{Type: "run_done", RunID: "first-run"})
	// Replayed start events must not reopen a completed run or reset sample IDs.
	m.observeTokenSpeedLifecycle(*session, Event{Type: "run_st", RunID: "first-run"})
	if got := m.tokenSpeedSnapshot(session.ID); got.Generating || got.Stats.CurrentRun.SampleCount != 1 {
		t.Fatal("replayed start reset a completed run")
	}
	m.observeTokenSpeedLifecycle(*session, Event{Type: "run_st", RunID: "second-run"})
	got := m.tokenSpeedSnapshot(session.ID)
	assertTokenSpeedSummary(t, got.Stats.Session, 1, 80, 1000, false)
	assertTokenSpeedSummary(t, got.Stats.CurrentRun, 0, 0, 0, true)
	if got.TokensPerSecond != nil || len(got.History) != 1 {
		t.Fatal("new run lost history or retained a stale current rate")
	}
	m.clearTokenSpeed(session.ID)
	session.Model = "second-model"
	m.observeTokenSpeedLifecycle(*session, Event{Type: "run_st", RunID: "third-run"})
	m.updateTokenSpeed(session.ID, "third-run", func(s *tokenSpeedState, now time.Time) {
		s.begin("request", now.Add(-3*time.Second), true)
		tokens := int64(120)
		s.finish("request", &tokens, now, true)
	}, true)
	m.observeTokenSpeedLifecycle(*session, Event{Type: "run_done", RunID: "third-run"})
	restarted := &Manager{store: m.store}
	empty := restarted.tokenSpeedSnapshot(session.ID)
	response, err := restarted.SnapshotIfChanged(context.Background(), session.ID, 10, "1")
	if err != nil || response.TokenSpeed == nil || response.TokenSpeed.Version <= empty.Version {
		t.Fatalf("snapshot did not restore a newer telemetry version: %+v err=%v", response, err)
	}
	restored := restarted.tokenSpeedSnapshot(session.ID)
	assertTokenSpeedSummary(t, restored.Stats.Session, 2, 200, 4000, false)
	if len(restored.History) != 2 || restored.History[0].Model != "second-model" || restored.History[1].Model != "first-model" {
		t.Fatalf("restored history = %#v", restored.History)
	}
	var count int64
	if err := model.GetDB().Model(&tables.WebSessionTokenSpeedTable{}).Where("web_session_id = ?", session.ID).Count(&count).Error; err != nil || count != 2 {
		t.Fatalf("durable sample count=%d err=%v", count, err)
	}
	if err := m.DeleteSession(context.Background(), session.ID); err != nil {
		t.Fatal(err)
	}
	if err := model.GetDB().Model(&tables.WebSessionTokenSpeedTable{}).Where("web_session_id = ?", session.ID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("delete retained samples: count=%d err=%v", count, err)
	}
	if got := m.tokenSpeedSnapshot(session.ID); len(got.History) != 0 || got.Stats.Session.SampleCount != 0 {
		t.Fatal("delete retained in-memory history")
	}
}

func TestTokenSpeedHistoryRejectsDelayedPersistence(t *testing.T) {
	m, session := newTextDeltaTestManager(t)
	m.ensureTokenSpeedState(session.ID)
	at := time.Now()
	row := tables.WebSessionTokenSpeedTable{WebSessionID: session.ID, RunID: "run", SampleID: "1", StartedAt: at.Add(-time.Second), EndedAt: at, OutputTokens: 80, DurationMs: 1000, Version: 2}
	m.persistTokenSpeedRecords(session.ID, []tables.WebSessionTokenSpeedTable{row})
	row.OutputTokens, row.Version, row.Estimated = 1, 1, true
	m.persistTokenSpeedRecords(session.ID, []tables.WebSessionTokenSpeedTable{row})
	restarted := &Manager{store: m.store}
	restarted.ensureTokenSpeedState(session.ID)
	restored := restarted.tokenSpeedSnapshot(session.ID)
	assertTokenSpeedSummary(t, restored.Stats.Session, 1, 80, 1000, false)
	if err := m.deleteTokenSpeedRecords(context.Background(), session.ID); err != nil {
		t.Fatal(err)
	}
	row.Version = 3
	m.persistTokenSpeedRecords(session.ID, []tables.WebSessionTokenSpeedTable{row})
	restarted = &Manager{store: m.store}
	restarted.ensureTokenSpeedState(session.ID)
	restored = restarted.tokenSpeedSnapshot(session.ID)
	assertTokenSpeedSummary(t, restored.Stats.Session, 0, 0, 0, true)
}
