package websession

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"code-kanban/model/tables"

	"go.uber.org/zap"
)

func TestSessionMessagingAvailabilityDoesNotProbeUnrelatedAgents(t *testing.T) {
	manager := &Manager{}
	var codexCalls atomic.Int32
	var piCalls atomic.Int32
	manager.runtimeCapabilityProbes = runtimeCapabilityProbeHooks{
		codexBinary: func() (CodexRuntimeConfig, error) {
			codexCalls.Add(1)
			return CodexRuntimeConfig{HasCodex: true}, nil
		},
		pi: func() (piRuntimeProbeResult, error) {
			piCalls.Add(1)
			return piRuntimeProbeResult{installed: true, compatible: true}, nil
		},
	}

	err := manager.ensureSessionMessagingAvailable(tables.WebSessionTable{Agent: string(AgentCodex)})
	if err != nil {
		t.Fatalf("Codex availability check returned error: %v", err)
	}
	if codexCalls.Load() != 1 {
		t.Fatalf("Codex probe calls = %d, want 1", codexCalls.Load())
	}
	if piCalls.Load() != 0 {
		t.Fatalf("Pi probe calls = %d, want 0", piCalls.Load())
	}
}

func TestRuntimeCapabilityProvidersSingleflightColdRequests(t *testing.T) {
	tests := []struct {
		name string
		run  func(*Manager, *atomic.Int32, chan struct{}, chan struct{})
	}{
		{
			name: "Codex binary",
			run: func(manager *Manager, calls *atomic.Int32, release chan struct{}, started chan struct{}) {
				manager.runtimeCapabilityProbes.codexBinary = func() (CodexRuntimeConfig, error) {
					calls.Add(1)
					started <- struct{}{}
					<-release
					return CodexRuntimeConfig{HasCodex: true, SupportsWebSession: true}, nil
				}
				runConcurrentCapabilityRequests(t, calls, started, release, func() {
					config := manager.applyBinaryCapabilities(defaultCodexRuntimeConfig(), false)
					if !config.HasCodex {
						t.Error("Codex binary probe result was not returned")
					}
				})
			},
		},
		{
			name: "Codex model catalog",
			run: func(manager *Manager, calls *atomic.Int32, release chan struct{}, started chan struct{}) {
				manager.runtimeCapabilityProbes.codexModels = func() ([]CodexModelInfo, error) {
					calls.Add(1)
					started <- struct{}{}
					<-release
					return []CodexModelInfo{{Model: "singleflight-model"}}, nil
				}
				runConcurrentCapabilityRequests(t, calls, started, release, func() {
					models := manager.getCodexModelCatalog(false)
					if len(models) != 1 || models[0].Model != "singleflight-model" {
						t.Errorf("unexpected Codex model probe result: %#v", models)
					}
				})
			},
		},
		{
			name: "Pi runtime",
			run: func(manager *Manager, calls *atomic.Int32, release chan struct{}, started chan struct{}) {
				manager.runtimeCapabilityProbes.pi = func() (piRuntimeProbeResult, error) {
					calls.Add(1)
					started <- struct{}{}
					<-release
					return piRuntimeProbeResult{installed: true, compatible: true}, nil
				}
				runConcurrentCapabilityRequests(t, calls, started, release, func() {
					if !manager.getPiRuntimeProbe().compatible {
						t.Error("Pi probe result was not returned")
					}
				})
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manager := &Manager{}
			var calls atomic.Int32
			release := make(chan struct{})
			started := make(chan struct{}, 32)
			test.run(manager, &calls, release, started)
		})
	}
}

func runConcurrentCapabilityRequests(
	t *testing.T,
	calls *atomic.Int32,
	started <-chan struct{},
	release chan struct{},
	request func(),
) {
	t.Helper()
	const requestCount = 16
	var releaseOnce sync.Once
	releaseRequests := func() {
		releaseOnce.Do(func() { close(release) })
	}
	defer releaseRequests()
	start := make(chan struct{})
	done := make(chan struct{})
	var wait sync.WaitGroup
	wait.Add(requestCount)
	for range requestCount {
		go func() {
			defer wait.Done()
			<-start
			request()
		}()
	}
	close(start)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("capability probe did not start")
	}
	time.Sleep(50 * time.Millisecond)
	got := calls.Load()
	releaseRequests()
	if got != 1 {
		t.Fatalf("concurrent requests started %d probes, want 1", got)
	}
	go func() {
		wait.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("concurrent capability requests did not finish")
	}
}

func TestRuntimeCapabilityCacheReturnsStaleAndRefreshesOnce(t *testing.T) {
	manager := &Manager{}
	var calls atomic.Int32
	refreshStarted := make(chan struct{})
	releaseRefresh := make(chan struct{})
	var releaseRefreshOnce sync.Once
	releaseRefreshProbe := func() {
		releaseRefreshOnce.Do(func() { close(releaseRefresh) })
	}
	defer releaseRefreshProbe()
	var refreshStartOnce sync.Once
	manager.runtimeCapabilityProbes.codexBinary = func() (CodexRuntimeConfig, error) {
		call := calls.Add(1)
		version := "0.146.0"
		if call > 1 {
			refreshStartOnce.Do(func() { close(refreshStarted) })
			<-releaseRefresh
			version = "0.147.0"
		}
		return CodexRuntimeConfig{
			HasCodex:           true,
			CodexVersion:       &version,
			SupportsWebSession: true,
		}, nil
	}

	initial := manager.applyBinaryCapabilities(defaultCodexRuntimeConfig(), false)
	if initial.CodexVersion == nil || *initial.CodexVersion != "0.146.0" {
		t.Fatalf("unexpected initial capability: %#v", initial)
	}
	manager.codexContextWindow.bins.mu.Lock()
	manager.codexContextWindow.bins.expiresAt = time.Now().Add(-time.Second)
	manager.codexContextWindow.bins.mu.Unlock()

	startedAt := time.Now()
	stale := manager.applyBinaryCapabilities(defaultCodexRuntimeConfig(), false)
	if elapsed := time.Since(startedAt); elapsed > 100*time.Millisecond {
		t.Fatalf("stale capability read blocked for %s", elapsed)
	}
	if stale.CodexVersion == nil || *stale.CodexVersion != "0.146.0" {
		t.Fatalf("expected stale capability while refreshing, got %#v", stale)
	}
	select {
	case <-refreshStarted:
	case <-time.After(time.Second):
		t.Fatal("background capability refresh did not start")
	}
	for range 10 {
		_ = manager.applyBinaryCapabilities(defaultCodexRuntimeConfig(), false)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("stale reads started %d total probes, want 2", got)
	}
	releaseRefreshProbe()
	waitForCapabilityCacheIdle(t, &manager.codexContextWindow.bins)

	refreshed := manager.applyBinaryCapabilities(defaultCodexRuntimeConfig(), false)
	if refreshed.CodexVersion == nil || *refreshed.CodexVersion != "0.147.0" {
		t.Fatalf("background refresh was not published: %#v", refreshed)
	}
}

func TestPiProbeFailureUsesBackoff(t *testing.T) {
	manager := &Manager{}
	var calls atomic.Int32
	manager.runtimeCapabilityProbes.pi = func() (piRuntimeProbeResult, error) {
		calls.Add(1)
		return piRuntimeProbeResult{
			installed:  true,
			diagnostic: piDiagnosticProtocol,
		}, errors.New("probe failed")
	}

	result := manager.getPiRuntimeProbe()
	if result.diagnostic != piDiagnosticProtocol {
		t.Fatalf("unexpected cached failure result: %#v", result)
	}
	for range 50 {
		_ = manager.getPiRuntimeProbe()
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("failed Pi probe retried %d times without backoff", got)
	}
	manager.piProbe.mu.Lock()
	remaining := time.Until(manager.piProbe.expiresAt)
	manager.piProbe.mu.Unlock()
	if remaining < 30*time.Second {
		t.Fatalf("Pi failure backoff is too short: %s", remaining)
	}
}

func TestListSessionsDoesNotRunCapabilityProbes(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	seedWebSession(t, project.ID, "No probe list", 1000)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	var calls atomic.Int32
	manager.runtimeCapabilityProbes = runtimeCapabilityProbeHooks{
		codexBinary: func() (CodexRuntimeConfig, error) {
			calls.Add(1)
			return CodexRuntimeConfig{}, errors.New("must not run")
		},
		codexModels: func() ([]CodexModelInfo, error) {
			calls.Add(1)
			return nil, errors.New("must not run")
		},
		pi: func() (piRuntimeProbeResult, error) {
			calls.Add(1)
			return piRuntimeProbeResult{}, errors.New("must not run")
		},
	}

	startedAt := time.Now()
	items, err := manager.ListSessions(context.Background(), project.ID)
	if err != nil {
		t.Fatalf("ListSessions returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one session, got %d", len(items))
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("ListSessions started %d external capability probes", got)
	}
	t.Logf("ListSessions completed without probes in %s", time.Since(startedAt))
}

func waitForCapabilityCacheIdle[T any](t *testing.T, cache *runtimeCapabilityCache[T]) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		cache.mu.Lock()
		idle := cache.inFlight == nil
		cache.mu.Unlock()
		if idle {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("capability cache did not become idle")
}
