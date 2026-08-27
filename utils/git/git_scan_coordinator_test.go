package git

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeFileScanRunner struct {
	statusFn         func(context.Context, string, FileScanOptions) (FileStatusResult, error)
	statsFn          func(context.Context, string, []FileStatus) (map[string]DiffStat, error)
	worktreeStatusFn func(context.Context, string) (*WorktreeStatus, error)
}

func (r *fakeFileScanRunner) status(ctx context.Context, root string, options FileScanOptions) (FileStatusResult, error) {
	if r.statusFn == nil {
		return FileStatusResult{Statuses: map[string]FileStatus{}}, nil
	}
	return r.statusFn(ctx, root, options)
}

func (r *fakeFileScanRunner) stats(ctx context.Context, root string, statuses []FileStatus) (map[string]DiffStat, error) {
	if r.statsFn == nil {
		return map[string]DiffStat{}, nil
	}
	return r.statsFn(ctx, root, statuses)
}

func (r *fakeFileScanRunner) worktreeStatus(ctx context.Context, root string) (*WorktreeStatus, error) {
	if r.worktreeStatusFn == nil {
		return &WorktreeStatus{}, nil
	}
	return r.worktreeStatusFn(ctx, root)
}

func TestFileScanCoordinatorSingleflightsEquivalentStatusScans(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	runner := &fakeFileScanRunner{
		statusFn: func(ctx context.Context, _ string, _ FileScanOptions) (FileStatusResult, error) {
			if calls.Add(1) == 1 {
				close(started)
			}
			select {
			case <-release:
				return FileStatusResult{
					Statuses: map[string]FileStatus{
						"file.txt": {Path: "file.txt", Kind: FileChangeKindModified},
					},
					ChangeToken: "shared",
				}, nil
			case <-ctx.Done():
				return FileStatusResult{}, ctx.Err()
			}
		},
	}
	coordinator := newFileScanCoordinator(runner, 2, time.Hour)
	coordinator.defaultTimeout = time.Second
	root := t.TempDir()
	options := FileScanOptions{IncludeUntracked: true, MaxEntries: 100}

	type response struct {
		result  FileStatusResult
		metrics FileScanMetrics
		err     error
	}
	firstDone := make(chan response, 1)
	go func() {
		result, metrics, err := coordinator.status(context.Background(), root, options)
		firstDone <- response{result: result, metrics: metrics, err: err}
	}()
	waitForTestSignal(t, started, "first status scan")

	secondDone := make(chan response, 1)
	go func() {
		result, metrics, err := coordinator.status(context.Background(), root, options)
		secondDone <- response{result: result, metrics: metrics, err: err}
	}()
	time.Sleep(20 * time.Millisecond)
	if got := calls.Load(); got != 1 {
		t.Fatalf("equivalent scans started %d runners, want 1", got)
	}
	close(release)

	first := waitForTestValue(t, firstDone, "first status result")
	second := waitForTestValue(t, secondDone, "shared status result")
	if first.err != nil || second.err != nil {
		t.Fatalf("status errors: first=%v second=%v", first.err, second.err)
	}
	if first.metrics.Shared {
		t.Fatal("flight owner was reported as shared")
	}
	if !second.metrics.Shared {
		t.Fatal("flight waiter was not reported as shared")
	}
	if first.result.ChangeToken != "shared" || second.result.ChangeToken != "shared" {
		t.Fatalf("unexpected shared results: first=%q second=%q", first.result.ChangeToken, second.result.ChangeToken)
	}
}

func TestFileScanCoordinatorLimitsGlobalConcurrency(t *testing.T) {
	started := make(chan struct{}, 3)
	release := make(chan struct{})
	var active atomic.Int32
	var maximum atomic.Int32
	runner := &fakeFileScanRunner{
		statusFn: func(ctx context.Context, _ string, _ FileScanOptions) (FileStatusResult, error) {
			current := active.Add(1)
			defer active.Add(-1)
			for {
				observed := maximum.Load()
				if current <= observed || maximum.CompareAndSwap(observed, current) {
					break
				}
			}
			started <- struct{}{}
			select {
			case <-release:
				return FileStatusResult{Statuses: map[string]FileStatus{}}, nil
			case <-ctx.Done():
				return FileStatusResult{}, ctx.Err()
			}
		},
	}
	coordinator := newFileScanCoordinator(runner, 2, time.Hour)
	coordinator.defaultTimeout = time.Second

	var wait sync.WaitGroup
	wait.Add(3)
	for range 3 {
		root := t.TempDir()
		go func() {
			defer wait.Done()
			_, _, _ = coordinator.status(context.Background(), root, FileScanOptions{Fresh: true})
		}()
	}
	waitForTestSignal(t, started, "first global scan")
	waitForTestSignal(t, started, "second global scan")
	select {
	case <-started:
		t.Fatal("third scan started before a global slot was released")
	case <-time.After(40 * time.Millisecond):
	}
	close(release)
	waitForTestWaitGroup(t, &wait, "global scans")
	if got := maximum.Load(); got != 2 {
		t.Fatalf("maximum global concurrency = %d, want 2", got)
	}
}

func TestFileScanCoordinatorSerializesStagesForSameRoot(t *testing.T) {
	started := make(chan string, 3)
	release := make(chan struct{}, 3)
	var active atomic.Int32
	var maximum atomic.Int32
	run := func(ctx context.Context, stage string) error {
		current := active.Add(1)
		defer active.Add(-1)
		for {
			observed := maximum.Load()
			if current <= observed || maximum.CompareAndSwap(observed, current) {
				break
			}
		}
		started <- stage
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	runner := &fakeFileScanRunner{
		statusFn: func(ctx context.Context, _ string, _ FileScanOptions) (FileStatusResult, error) {
			err := run(ctx, "status")
			return FileStatusResult{Statuses: map[string]FileStatus{}}, err
		},
		statsFn: func(ctx context.Context, _ string, _ []FileStatus) (map[string]DiffStat, error) {
			err := run(ctx, "stats")
			return map[string]DiffStat{}, err
		},
		worktreeStatusFn: func(ctx context.Context, _ string) (*WorktreeStatus, error) {
			err := run(ctx, "worktree")
			return &WorktreeStatus{}, err
		},
	}
	coordinator := newFileScanCoordinator(runner, 3, time.Hour)
	coordinator.defaultTimeout = time.Second
	root := t.TempDir()

	var wait sync.WaitGroup
	wait.Add(3)
	go func() {
		defer wait.Done()
		_, _, _ = coordinator.status(context.Background(), root, FileScanOptions{Fresh: true})
	}()
	_ = waitForTestValue(t, started, "first root stage")
	go func() {
		defer wait.Done()
		_, _, _ = coordinator.stats(context.Background(), root, nil, "token", FileScanOptions{Fresh: true})
	}()
	go func() {
		defer wait.Done()
		_, _, _ = coordinator.worktreeStatus(context.Background(), root, true)
	}()
	select {
	case stage := <-started:
		t.Fatalf("stage %q overlapped the first stage for the same root", stage)
	case <-time.After(40 * time.Millisecond):
	}

	release <- struct{}{}
	_ = waitForTestValue(t, started, "second root stage")
	select {
	case stage := <-started:
		t.Fatalf("stage %q overlapped the second stage for the same root", stage)
	case <-time.After(40 * time.Millisecond):
	}
	release <- struct{}{}
	_ = waitForTestValue(t, started, "third root stage")
	release <- struct{}{}
	waitForTestWaitGroup(t, &wait, "same-root stages")
	if got := maximum.Load(); got != 1 {
		t.Fatalf("maximum same-root concurrency = %d, want 1", got)
	}
}

func TestFileScanCoordinatorCacheFreshAndInvalidate(t *testing.T) {
	var statusCalls atomic.Int32
	var statsCalls atomic.Int32
	var worktreeCalls atomic.Int32
	runner := &fakeFileScanRunner{
		statusFn: func(_ context.Context, _ string, _ FileScanOptions) (FileStatusResult, error) {
			call := statusCalls.Add(1)
			return FileStatusResult{
				Statuses:    map[string]FileStatus{"file.txt": {Path: "file.txt", Kind: FileChangeKindModified}},
				TotalCount:  int(call),
				ChangeToken: "token",
			}, nil
		},
		statsFn: func(_ context.Context, _ string, _ []FileStatus) (map[string]DiffStat, error) {
			call := statsCalls.Add(1)
			return map[string]DiffStat{"file.txt": {Additions: int64(call)}}, nil
		},
		worktreeStatusFn: func(_ context.Context, _ string) (*WorktreeStatus, error) {
			return &WorktreeStatus{Modified: int(worktreeCalls.Add(1))}, nil
		},
	}
	coordinator := newFileScanCoordinator(runner, 2, time.Hour)
	root := t.TempDir()
	statusList := []FileStatus{{Path: "file.txt", Kind: FileChangeKindModified}}

	firstStatus, _, err := coordinator.status(context.Background(), root, FileScanOptions{})
	if err != nil {
		t.Fatalf("first status scan: %v", err)
	}
	firstStats, _, err := coordinator.stats(context.Background(), root, statusList, "token", FileScanOptions{})
	if err != nil {
		t.Fatalf("first stats scan: %v", err)
	}
	firstWorktree, _, err := coordinator.worktreeStatus(context.Background(), root, false)
	if err != nil {
		t.Fatalf("first worktree scan: %v", err)
	}

	cachedStatus, statusMetrics, _ := coordinator.status(context.Background(), root, FileScanOptions{})
	cachedStats, statsMetrics, _ := coordinator.stats(context.Background(), root, statusList, "token", FileScanOptions{})
	cachedWorktree, worktreeMetrics, _ := coordinator.worktreeStatus(context.Background(), root, false)
	if !statusMetrics.CacheHit || !statsMetrics.CacheHit || !worktreeMetrics.CacheHit {
		t.Fatalf("expected all second reads to hit cache: status=%+v stats=%+v worktree=%+v", statusMetrics, statsMetrics, worktreeMetrics)
	}
	if cachedStatus.TotalCount != firstStatus.TotalCount ||
		cachedStats["file.txt"] != firstStats["file.txt"] ||
		cachedWorktree.Modified != firstWorktree.Modified {
		t.Fatal("cached scan results changed")
	}

	freshStatus, freshMetrics, err := coordinator.status(context.Background(), root, FileScanOptions{Fresh: true})
	if err != nil {
		t.Fatalf("fresh status scan: %v", err)
	}
	if freshMetrics.CacheHit || freshStatus.TotalCount != 2 {
		t.Fatalf("fresh status reused cache: result=%+v metrics=%+v", freshStatus, freshMetrics)
	}

	coordinator.invalidate(root)
	invalidatedStatus, _, _ := coordinator.status(context.Background(), root, FileScanOptions{})
	invalidatedStats, _, _ := coordinator.stats(context.Background(), root, statusList, "token", FileScanOptions{})
	invalidatedWorktree, _, _ := coordinator.worktreeStatus(context.Background(), root, false)
	if invalidatedStatus.TotalCount != 3 || invalidatedStats["file.txt"].Additions != 2 || invalidatedWorktree.Modified != 2 {
		t.Fatalf("invalidate did not clear every cache: status=%+v stats=%+v worktree=%+v", invalidatedStatus, invalidatedStats, invalidatedWorktree)
	}
}

func TestFileScanCoordinatorCallerCancellationDoesNotCancelSharedScan(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	var runnerCanceled atomic.Bool
	runner := &fakeFileScanRunner{
		statusFn: func(ctx context.Context, _ string, _ FileScanOptions) (FileStatusResult, error) {
			calls.Add(1)
			close(started)
			select {
			case <-release:
				return FileStatusResult{Statuses: map[string]FileStatus{}, ChangeToken: "complete"}, nil
			case <-ctx.Done():
				runnerCanceled.Store(true)
				return FileStatusResult{}, ctx.Err()
			}
		},
	}
	coordinator := newFileScanCoordinator(runner, 2, time.Hour)
	coordinator.defaultTimeout = time.Second
	root := t.TempDir()

	leaderCtx, cancelLeader := context.WithCancel(context.Background())
	leaderDone := make(chan error, 1)
	go func() {
		_, _, err := coordinator.status(leaderCtx, root, FileScanOptions{})
		leaderDone <- err
	}()
	waitForTestSignal(t, started, "cancelable status scan")
	cancelLeader()
	if err := waitForTestValue(t, leaderDone, "canceled caller"); !errors.Is(err, context.Canceled) {
		t.Fatalf("leader error = %v, want context.Canceled", err)
	}

	followerDone := make(chan string, 1)
	go func() {
		result, _, err := coordinator.status(context.Background(), root, FileScanOptions{})
		if err != nil {
			followerDone <- "error: " + err.Error()
			return
		}
		followerDone <- result.ChangeToken
	}()
	close(release)
	if got := waitForTestValue(t, followerDone, "shared scan follower"); got != "complete" {
		t.Fatalf("follower result = %q, want complete", got)
	}
	if runnerCanceled.Load() {
		t.Fatal("leader cancellation canceled the shared runner")
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("caller cancellation caused %d scans, want 1", got)
	}
}

func waitForTestSignal(t *testing.T, channel <-chan struct{}, label string) {
	t.Helper()
	select {
	case <-channel:
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %s", label)
	}
}

func waitForTestValue[T any](t *testing.T, channel <-chan T, label string) T {
	t.Helper()
	select {
	case value := <-channel:
		return value
	case <-time.After(time.Second):
		var zero T
		t.Fatalf("timed out waiting for %s", label)
		return zero
	}
}

func waitForTestWaitGroup(t *testing.T, wait *sync.WaitGroup, label string) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		wait.Wait()
		close(done)
	}()
	waitForTestSignal(t, done, label)
}
