package service

import (
	"context"
	"sync"
)

type aiSessionRequestFlight struct {
	done   chan struct{}
	result *ProjectAISessions
	err    error
}

type aiSessionRequestGroup struct {
	mu      sync.Mutex
	flights map[string]*aiSessionRequestFlight
}

var projectAISessionRequests aiSessionRequestGroup

func (g *aiSessionRequestGroup) do(
	ctx context.Context,
	key string,
	load func(context.Context) (*ProjectAISessions, error),
) (*ProjectAISessions, error) {
	ctx = ensureContext(ctx)
	g.mu.Lock()
	if g.flights == nil {
		g.flights = make(map[string]*aiSessionRequestFlight)
	}
	if flight, ok := g.flights[key]; ok {
		g.mu.Unlock()
		return waitForAISessionRequest(ctx, flight)
	}
	flight := &aiSessionRequestFlight{done: make(chan struct{})}
	g.flights[key] = flight
	g.mu.Unlock()

	taskCtx := context.WithoutCancel(ctx)
	go func() {
		flight.result, flight.err = load(taskCtx)
		g.mu.Lock()
		if g.flights[key] == flight {
			delete(g.flights, key)
		}
		close(flight.done)
		g.mu.Unlock()
	}()
	return waitForAISessionRequest(ctx, flight)
}

func waitForAISessionRequest(ctx context.Context, flight *aiSessionRequestFlight) (*ProjectAISessions, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-flight.done:
		return cloneProjectAISessions(flight.result), flight.err
	}
}

func cloneProjectAISessions(source *ProjectAISessions) *ProjectAISessions {
	if source == nil {
		return nil
	}
	result := *source
	result.ClaudeSessions = cloneAISessionSummaries(source.ClaudeSessions)
	result.CodexSessions = cloneAISessionSummaries(source.CodexSessions)
	result.PiSessions = cloneAISessionSummaries(source.PiSessions)
	return &result
}

func cloneAISessionSummaries(source []*AISessionSummary) []*AISessionSummary {
	if source == nil {
		return nil
	}
	result := make([]*AISessionSummary, len(source))
	for index, session := range source {
		if session == nil {
			continue
		}
		cloned := *session
		if session.LastMessageAt != nil {
			lastMessageAt := *session.LastMessageAt
			cloned.LastMessageAt = &lastMessageAt
		}
		result[index] = &cloned
	}
	return result
}
