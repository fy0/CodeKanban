package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type aiRequestTestResponse struct {
	result *ProjectAISessions
	err    error
}

func TestAISessionRequestGroupSingleflightsAndClonesResults(t *testing.T) {
	var group aiSessionRequestGroup
	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	load := func(ctx context.Context) (*ProjectAISessions, error) {
		calls.Add(1)
		close(started)
		select {
		case <-release:
			lastMessageAt := time.Now()
			return &ProjectAISessions{
				HasCodex: true,
				CodexSessions: []*AISessionSummary{{
					SessionID:     "session-1",
					Title:         "shared",
					LastMessageAt: &lastMessageAt,
				}},
			}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	firstDone := make(chan aiRequestTestResponse, 1)
	secondDone := make(chan aiRequestTestResponse, 1)
	go func() {
		result, err := group.do(context.Background(), "project", load)
		firstDone <- aiRequestTestResponse{result: result, err: err}
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("AI session request did not start")
	}
	go func() {
		result, err := group.do(context.Background(), "project", load)
		secondDone <- aiRequestTestResponse{result: result, err: err}
	}()
	time.Sleep(20 * time.Millisecond)
	if got := calls.Load(); got != 1 {
		t.Fatalf("equivalent requests started %d scans, want 1", got)
	}
	close(release)

	first := waitForAIRequestTestResponse(t, firstDone)
	second := waitForAIRequestTestResponse(t, secondDone)
	if first.err != nil || second.err != nil {
		t.Fatalf("request errors: first=%v second=%v", first.err, second.err)
	}
	if first.result == second.result || first.result.CodexSessions[0] == second.result.CodexSessions[0] {
		t.Fatal("shared request returned mutable result pointers")
	}
	first.result.CodexSessions[0].Title = "changed"
	if second.result.CodexSessions[0].Title != "shared" {
		t.Fatal("mutating one shared result changed another caller's result")
	}
}

func TestAISessionRequestGroupCallerCancellationDoesNotCancelFlight(t *testing.T) {
	var group aiSessionRequestGroup
	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	runnerCanceled := make(chan struct{}, 1)
	var startedOnce sync.Once
	load := func(ctx context.Context) (*ProjectAISessions, error) {
		calls.Add(1)
		startedOnce.Do(func() { close(started) })
		select {
		case <-release:
			return &ProjectAISessions{HasClaudeCode: true}, nil
		case <-ctx.Done():
			runnerCanceled <- struct{}{}
			return nil, ctx.Err()
		}
	}

	callerCtx, cancelCaller := context.WithCancel(context.Background())
	leaderDone := make(chan error, 1)
	go func() {
		_, err := group.do(callerCtx, "project", load)
		leaderDone <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("AI session request did not start")
	}
	followerDone := make(chan *ProjectAISessions, 1)
	go func() {
		result, _ := group.do(context.Background(), "project", load)
		followerDone <- result
	}()
	time.Sleep(20 * time.Millisecond)
	if got := calls.Load(); got != 1 {
		t.Fatalf("follower did not join the existing scan: calls=%d", got)
	}

	cancelCaller()
	select {
	case err := <-leaderDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("leader error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled caller did not return")
	}

	close(release)
	select {
	case result := <-followerDone:
		if result == nil || !result.HasClaudeCode {
			t.Fatalf("unexpected follower result: %#v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("shared request follower did not finish")
	}
	select {
	case <-runnerCanceled:
		t.Fatal("caller cancellation canceled the shared AI session scan")
	default:
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("caller cancellation caused %d scans, want 1", got)
	}
}

func waitForAIRequestTestResponse(t *testing.T, done <-chan aiRequestTestResponse) aiRequestTestResponse {
	t.Helper()
	select {
	case result := <-done:
		return result
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for AI session request")
		return aiRequestTestResponse{}
	}
}
