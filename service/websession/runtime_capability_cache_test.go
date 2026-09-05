package websession

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"code-kanban/model/tables"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestRuntimeCapabilityProbesLogStructuredDurations(t *testing.T) {
	core, observed := observer.New(zapcore.DebugLevel)
	manager := &Manager{logger: zap.New(core)}
	manager.runtimeCapabilityProbes = runtimeCapabilityProbeHooks{
		codexBinary: func() (WebSessionRuntimeConfig, error) {
			version := "1.2.3"
			return WebSessionRuntimeConfig{HasCodex: true, HasClaudeCode: true, CodexVersion: &version}, nil
		},
		codexModels: func() ([]CodexModelInfo, error) {
			return []CodexModelInfo{{Model: "codex-test"}}, nil
		},
		pi: func() (piRuntimeProbeResult, error) {
			version := "1.2.3"
			return piRuntimeProbeResult{
				installed:  true,
				version:    &version,
				compatible: true,
				models:     []PiModelInfo{{Provider: "test", ID: "pi-test"}},
			}, nil
		},
	}

	if _, err := manager.probeCodexBinaryCapabilities(); err != nil {
		t.Fatalf("probeCodexBinaryCapabilities returned error: %v", err)
	}
	if _, err := manager.probeCodexModelCatalog(); err != nil {
		t.Fatalf("probeCodexModelCatalog returned error: %v", err)
	}
	if _, err := manager.probePiRuntimeCapabilities(); err != nil {
		t.Fatalf("probePiRuntimeCapabilities returned error: %v", err)
	}

	entries := observed.FilterMessage("web session runtime capability probe completed").All()
	if len(entries) != 3 {
		t.Fatalf("runtime capability logs = %#v, want three", entries)
	}
	byProbe := make(map[string]map[string]any, len(entries))
	for _, entry := range entries {
		if entry.Level != zapcore.InfoLevel {
			t.Fatalf("successful runtime capability probe logged at %s", entry.Level)
		}
		fields := entry.ContextMap()
		probe, _ := fields["probe"].(string)
		byProbe[probe] = fields
		if fields["result"] != "success" || fields["errorCode"] != "" {
			t.Fatalf("unexpected runtime capability result fields: %#v", fields)
		}
		if _, ok := fields["duration"]; !ok {
			t.Fatalf("runtime capability log is missing duration: %#v", fields)
		}
	}
	for _, field := range []string{"versionDuration", "rpcDuration", "modelDuration"} {
		if _, ok := byProbe["pi_runtime"][field]; !ok {
			t.Fatalf("Pi runtime capability log is missing %q: %#v", field, byProbe["pi_runtime"])
		}
	}
	if byProbe["codex_binary"]["codexInstalled"] != true ||
		byProbe["codex_models"]["modelCount"] != int64(1) ||
		byProbe["pi_runtime"]["modelCount"] != int64(1) {
		t.Fatalf("unexpected capability-specific fields: %#v", byProbe)
	}

	const secretError = "runtime-probe-secret-must-not-appear"
	manager.runtimeCapabilityProbes.codexModels = func() ([]CodexModelInfo, error) {
		return nil, errors.New(secretError)
	}
	if _, err := manager.probeCodexModelCatalog(); err == nil {
		t.Fatal("expected failed model probe")
	}
	failureEntries := observed.FilterMessage("web session runtime capability probe completed").All()
	failure := failureEntries[len(failureEntries)-1]
	if failure.Level != zapcore.WarnLevel || failure.ContextMap()["result"] != "failed" {
		t.Fatalf("unexpected failed capability probe log: %#v", failure)
	}
	for _, field := range failure.Context {
		if strings.Contains(fmt.Sprint(field.Interface), secretError) || strings.Contains(field.String, secretError) {
			t.Fatalf("capability probe log leaked raw error: %#v", failure.Context)
		}
	}
}

func TestSessionMessagingAvailabilityDoesNotProbeUnrelatedAgents(t *testing.T) {
	manager := &Manager{}
	var codexCalls atomic.Int32
	var piCalls atomic.Int32
	manager.runtimeCapabilityProbes = runtimeCapabilityProbeHooks{
		codexBinary: func() (WebSessionRuntimeConfig, error) {
			codexCalls.Add(1)
			return WebSessionRuntimeConfig{HasCodex: true}, nil
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
				manager.runtimeCapabilityProbes.codexBinary = func() (WebSessionRuntimeConfig, error) {
					calls.Add(1)
					started <- struct{}{}
					<-release
					return WebSessionRuntimeConfig{HasCodex: true, SupportsWebSession: true}, nil
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
	manager.runtimeCapabilityProbes.codexBinary = func() (WebSessionRuntimeConfig, error) {
		call := calls.Add(1)
		version := "0.146.0"
		if call > 1 {
			refreshStartOnce.Do(func() { close(refreshStarted) })
			<-releaseRefresh
			version = "0.147.0"
		}
		return WebSessionRuntimeConfig{
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

func TestRuntimeConfigColdRequestWarmsCapabilitiesInBackground(t *testing.T) {
	manager := &Manager{}
	binaryStarted := make(chan struct{})
	piStarted := make(chan struct{})
	modelsStarted := make(chan struct{})
	releaseBinary := make(chan struct{})
	releasePi := make(chan struct{})
	releaseModels := make(chan struct{})
	manager.runtimeCapabilityProbes = runtimeCapabilityProbeHooks{
		codexBinary: func() (WebSessionRuntimeConfig, error) {
			close(binaryStarted)
			<-releaseBinary
			return WebSessionRuntimeConfig{HasCodex: true, HasClaudeCode: true, SupportsWebSession: true}, nil
		},
		codexModels: func() ([]CodexModelInfo, error) {
			close(modelsStarted)
			<-releaseModels
			return []CodexModelInfo{{Model: "background-model"}}, nil
		},
		pi: func() (piRuntimeProbeResult, error) {
			close(piStarted)
			<-releasePi
			return piRuntimeProbeResult{installed: true, compatible: true}, nil
		},
	}

	startedAt := time.Now()
	cold := manager.GetWebSessionRuntimeConfigWithModels()
	if elapsed := time.Since(startedAt); elapsed > 100*time.Millisecond {
		t.Fatalf("cold runtime config blocked for %s", elapsed)
	}
	if !cold.CapabilitiesRefreshing {
		t.Fatal("cold runtime config did not report background refresh")
	}
	select {
	case <-binaryStarted:
	case <-time.After(time.Second):
		t.Fatal("Codex binary background probe did not start")
	}
	select {
	case <-piStarted:
	case <-time.After(time.Second):
		t.Fatal("Pi background probe did not start")
	}
	close(releaseBinary)
	close(releasePi)
	waitForCapabilityCacheIdle(t, &manager.codexContextWindow.bins)
	waitForCapabilityCacheIdle(t, &manager.piProbe)

	withBinaries := manager.GetWebSessionRuntimeConfigWithModels()
	if !withBinaries.HasCodex || !withBinaries.HasClaudeCode || !withBinaries.HasPi {
		t.Fatalf("background binary results were not published: %#v", withBinaries)
	}
	if !withBinaries.CapabilitiesRefreshing {
		t.Fatal("cold model catalog did not report background refresh")
	}
	select {
	case <-modelsStarted:
	case <-time.After(time.Second):
		t.Fatal("Codex model background probe did not start")
	}
	close(releaseModels)
	waitForCapabilityCacheIdle(t, &manager.codexContextWindow.models)

	ready := manager.GetWebSessionRuntimeConfigWithModels()
	if ready.CapabilitiesRefreshing {
		t.Fatal("completed runtime config still reports background refresh")
	}
	if len(ready.Models) != 1 || ready.Models[0].Model != "background-model" {
		t.Fatalf("background model result was not published: %#v", ready.Models)
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
		codexBinary: func() (WebSessionRuntimeConfig, error) {
			calls.Add(1)
			return WebSessionRuntimeConfig{}, errors.New("must not run")
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
