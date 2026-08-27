package git

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	defaultFileScanCacheTTL    = 2 * time.Second
	defaultFileScanConcurrency = 2
	defaultFileScanTimeout     = 5 * time.Second
)

// FileScanOptions controls status scan sharing and cache reuse.
type FileScanOptions struct {
	IncludeUntracked bool
	MaxEntries       int
	Fast             bool
	Fresh            bool
}

// FileScanMetrics describes how a coordinated Git stage was served.
type FileScanMetrics struct {
	CacheHit  bool
	Shared    bool
	QueueWait time.Duration
	Execution time.Duration
}

type fileScanRunner interface {
	status(context.Context, string, FileScanOptions) (FileStatusResult, error)
	stats(context.Context, string, []FileStatus) (map[string]DiffStat, error)
	worktreeStatus(context.Context, string) (*WorktreeStatus, error)
}

type defaultFileScanRunner struct{}

func (defaultFileScanRunner) status(ctx context.Context, root string, options FileScanOptions) (FileStatusResult, error) {
	if options.Fast {
		return ListFileStatusesFastContext(ctx, root, options.IncludeUntracked, options.MaxEntries)
	}
	return ListFileStatusesLimitedContext(ctx, root, options.IncludeUntracked, options.MaxEntries)
}

func (defaultFileScanRunner) stats(ctx context.Context, root string, statuses []FileStatus) (map[string]DiffStat, error) {
	return GenerateDiffStatsAgainstHEADContext(ctx, root, statuses)
}

func (defaultFileScanRunner) worktreeStatus(ctx context.Context, root string) (*WorktreeStatus, error) {
	return GetWorktreeStatusContext(ctx, root)
}

type statusScanKey struct {
	root             string
	includeUntracked bool
	maxEntries       int
	fast             bool
}

type statusFlightKey struct {
	statusScanKey
	generation uint64
}

type statusScanFlight struct {
	done    chan struct{}
	result  FileStatusResult
	metrics FileScanMetrics
	err     error
}

type statusCacheEntry struct {
	result     FileStatusResult
	storedAt   time.Time
	generation uint64
}

type statsScanKey struct {
	root             string
	changeToken      string
	includeUntracked bool
	fast             bool
}

type statsFlightKey struct {
	statsScanKey
	generation uint64
}

type statsScanFlight struct {
	done    chan struct{}
	result  map[string]DiffStat
	metrics FileScanMetrics
	err     error
}

type statsCacheEntry struct {
	result     map[string]DiffStat
	storedAt   time.Time
	generation uint64
}

type worktreeStatusFlight struct {
	done    chan struct{}
	result  *WorktreeStatus
	metrics FileScanMetrics
	err     error
}

type worktreeStatusCacheEntry struct {
	result     *WorktreeStatus
	storedAt   time.Time
	generation uint64
}

type fileScanCoordinator struct {
	mu              sync.Mutex
	statusFlights   map[statusFlightKey]*statusScanFlight
	statusCache     map[statusScanKey]statusCacheEntry
	statsFlights    map[statsFlightKey]*statsScanFlight
	statsCache      map[statsScanKey]statsCacheEntry
	worktreeFlights map[statusFlightKey]*worktreeStatusFlight
	worktreeCache   map[string]worktreeStatusCacheEntry
	generations     map[string]uint64
	rootGates       map[string]chan struct{}
	slots           chan struct{}
	runner          fileScanRunner
	cacheTTL        time.Duration
	defaultTimeout  time.Duration
	now             func() time.Time
}

func newFileScanCoordinator(runner fileScanRunner, concurrency int, cacheTTL time.Duration) *fileScanCoordinator {
	if runner == nil {
		runner = defaultFileScanRunner{}
	}
	if concurrency <= 0 {
		concurrency = defaultFileScanConcurrency
	}
	if cacheTTL <= 0 {
		cacheTTL = defaultFileScanCacheTTL
	}
	return &fileScanCoordinator{
		statusFlights:   make(map[statusFlightKey]*statusScanFlight),
		statusCache:     make(map[statusScanKey]statusCacheEntry),
		statsFlights:    make(map[statsFlightKey]*statsScanFlight),
		statsCache:      make(map[statsScanKey]statsCacheEntry),
		worktreeFlights: make(map[statusFlightKey]*worktreeStatusFlight),
		worktreeCache:   make(map[string]worktreeStatusCacheEntry),
		generations:     make(map[string]uint64),
		rootGates:       make(map[string]chan struct{}),
		slots:           make(chan struct{}, concurrency),
		runner:          runner,
		cacheTTL:        cacheTTL,
		defaultTimeout:  defaultFileScanTimeout,
		now:             time.Now,
	}
}

var sharedFileScanCoordinator = newFileScanCoordinator(nil, defaultFileScanConcurrency, defaultFileScanCacheTTL)

// ScanFileStatuses coalesces equivalent scans and limits process-wide Git work.
func ScanFileStatuses(ctx context.Context, root string, options FileScanOptions) (FileStatusResult, FileScanMetrics, error) {
	return sharedFileScanCoordinator.status(ctx, root, options)
}

// ScanDiffStats coalesces diff-stat generation for one status snapshot.
func ScanDiffStats(
	ctx context.Context,
	root string,
	statuses []FileStatus,
	changeToken string,
	options FileScanOptions,
) (map[string]DiffStat, FileScanMetrics, error) {
	return sharedFileScanCoordinator.stats(ctx, root, statuses, changeToken, options)
}

// ScanWorktreeStatus coordinates aggregate status and ahead/behind collection.
func ScanWorktreeStatus(ctx context.Context, root string, fresh bool) (*WorktreeStatus, FileScanMetrics, error) {
	return sharedFileScanCoordinator.worktreeStatus(ctx, root, fresh)
}

// InvalidateFileScans prevents a completed pre-mutation flight from entering the cache.
func InvalidateFileScans(root string) {
	sharedFileScanCoordinator.invalidate(root)
}

func (c *fileScanCoordinator) status(
	ctx context.Context,
	root string,
	options FileScanOptions,
) (FileStatusResult, FileScanMetrics, error) {
	ctx = nonNilContext(ctx)
	key := statusScanKey{
		root:             normalizeScanRoot(root),
		includeUntracked: options.IncludeUntracked,
		maxEntries:       max(0, options.MaxEntries),
		fast:             options.Fast,
	}
	now := c.now()

	c.mu.Lock()
	generation := c.generations[key.root]
	if cached, ok := c.statusCache[key]; ok && !options.Fresh &&
		cached.generation == generation && now.Sub(cached.storedAt) < c.cacheTTL {
		c.mu.Unlock()
		return cloneFileStatusResult(cached.result), FileScanMetrics{CacheHit: true}, nil
	}
	flightKey := statusFlightKey{statusScanKey: key, generation: generation}
	if flight, ok := c.statusFlights[flightKey]; ok {
		c.mu.Unlock()
		return waitForStatusFlight(ctx, flight, true)
	}
	flight := &statusScanFlight{done: make(chan struct{})}
	c.statusFlights[flightKey] = flight
	c.mu.Unlock()

	taskCtx, cancel := c.sharedTaskContext(ctx)
	go c.runStatusFlight(taskCtx, cancel, flightKey, flight, root, options)
	return waitForStatusFlight(ctx, flight, false)
}

func (c *fileScanCoordinator) runStatusFlight(
	ctx context.Context,
	cancel context.CancelFunc,
	key statusFlightKey,
	flight *statusScanFlight,
	root string,
	options FileScanOptions,
) {
	defer cancel()
	queueStarted := c.now()
	release, err := c.acquire(ctx, key.root)
	if err != nil {
		flight.err = ctx.Err()
		c.finishStatusFlight(key, flight)
		return
	}
	flight.metrics.QueueWait = c.now().Sub(queueStarted)

	executionStarted := c.now()
	flight.result, flight.err = c.runner.status(ctx, root, options)
	flight.metrics.Execution = c.now().Sub(executionStarted)
	release()
	c.finishStatusFlight(key, flight)
}

func (c *fileScanCoordinator) finishStatusFlight(key statusFlightKey, flight *statusScanFlight) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if flight.err == nil && c.generations[key.root] == key.generation {
		c.statusCache[key.statusScanKey] = statusCacheEntry{
			result:     cloneFileStatusResult(flight.result),
			storedAt:   c.now(),
			generation: key.generation,
		}
	}
	if current := c.statusFlights[key]; current == flight {
		delete(c.statusFlights, key)
	}
	c.removeExpiredLocked()
	close(flight.done)
}

func waitForStatusFlight(ctx context.Context, flight *statusScanFlight, shared bool) (FileStatusResult, FileScanMetrics, error) {
	select {
	case <-ctx.Done():
		return FileStatusResult{}, FileScanMetrics{Shared: shared}, ctx.Err()
	case <-flight.done:
		metrics := flight.metrics
		metrics.Shared = shared
		return cloneFileStatusResult(flight.result), metrics, flight.err
	}
}

func (c *fileScanCoordinator) stats(
	ctx context.Context,
	root string,
	statuses []FileStatus,
	changeToken string,
	options FileScanOptions,
) (map[string]DiffStat, FileScanMetrics, error) {
	ctx = nonNilContext(ctx)
	key := statsScanKey{
		root:             normalizeScanRoot(root),
		changeToken:      changeToken,
		includeUntracked: options.IncludeUntracked,
		fast:             options.Fast,
	}
	now := c.now()

	c.mu.Lock()
	generation := c.generations[key.root]
	if cached, ok := c.statsCache[key]; ok && !options.Fresh &&
		cached.generation == generation && now.Sub(cached.storedAt) < c.cacheTTL {
		c.mu.Unlock()
		return cloneDiffStats(cached.result), FileScanMetrics{CacheHit: true}, nil
	}
	flightKey := statsFlightKey{statsScanKey: key, generation: generation}
	if flight, ok := c.statsFlights[flightKey]; ok {
		c.mu.Unlock()
		return waitForStatsFlight(ctx, flight, true)
	}
	flight := &statsScanFlight{done: make(chan struct{})}
	c.statsFlights[flightKey] = flight
	c.mu.Unlock()

	statusCopy := append([]FileStatus(nil), statuses...)
	taskCtx, cancel := c.sharedTaskContext(ctx)
	go c.runStatsFlight(taskCtx, cancel, flightKey, flight, root, statusCopy)
	return waitForStatsFlight(ctx, flight, false)
}

func (c *fileScanCoordinator) runStatsFlight(
	ctx context.Context,
	cancel context.CancelFunc,
	key statsFlightKey,
	flight *statsScanFlight,
	root string,
	statuses []FileStatus,
) {
	defer cancel()
	queueStarted := c.now()
	release, err := c.acquire(ctx, key.root)
	if err != nil {
		flight.err = ctx.Err()
		c.finishStatsFlight(key, flight)
		return
	}
	flight.metrics.QueueWait = c.now().Sub(queueStarted)

	executionStarted := c.now()
	flight.result, flight.err = c.runner.stats(ctx, root, statuses)
	flight.metrics.Execution = c.now().Sub(executionStarted)
	release()
	c.finishStatsFlight(key, flight)
}

func (c *fileScanCoordinator) finishStatsFlight(key statsFlightKey, flight *statsScanFlight) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if flight.err == nil && c.generations[key.root] == key.generation {
		c.statsCache[key.statsScanKey] = statsCacheEntry{
			result:     cloneDiffStats(flight.result),
			storedAt:   c.now(),
			generation: key.generation,
		}
	}
	if current := c.statsFlights[key]; current == flight {
		delete(c.statsFlights, key)
	}
	c.removeExpiredLocked()
	close(flight.done)
}

func waitForStatsFlight(ctx context.Context, flight *statsScanFlight, shared bool) (map[string]DiffStat, FileScanMetrics, error) {
	select {
	case <-ctx.Done():
		return nil, FileScanMetrics{Shared: shared}, ctx.Err()
	case <-flight.done:
		metrics := flight.metrics
		metrics.Shared = shared
		return cloneDiffStats(flight.result), metrics, flight.err
	}
}

func (c *fileScanCoordinator) worktreeStatus(
	ctx context.Context,
	root string,
	fresh bool,
) (*WorktreeStatus, FileScanMetrics, error) {
	ctx = nonNilContext(ctx)
	normalizedRoot := normalizeScanRoot(root)
	now := c.now()

	c.mu.Lock()
	generation := c.generations[normalizedRoot]
	if cached, ok := c.worktreeCache[normalizedRoot]; ok && !fresh &&
		cached.generation == generation && now.Sub(cached.storedAt) < c.cacheTTL {
		c.mu.Unlock()
		return cloneWorktreeStatus(cached.result), FileScanMetrics{CacheHit: true}, nil
	}
	flightKey := statusFlightKey{
		statusScanKey: statusScanKey{root: normalizedRoot},
		generation:    generation,
	}
	if flight, ok := c.worktreeFlights[flightKey]; ok {
		c.mu.Unlock()
		return waitForWorktreeStatusFlight(ctx, flight, true)
	}
	flight := &worktreeStatusFlight{done: make(chan struct{})}
	c.worktreeFlights[flightKey] = flight
	c.mu.Unlock()

	taskCtx, cancel := c.sharedTaskContext(ctx)
	go c.runWorktreeStatusFlight(taskCtx, cancel, flightKey, flight, root)
	return waitForWorktreeStatusFlight(ctx, flight, false)
}

func (c *fileScanCoordinator) runWorktreeStatusFlight(
	ctx context.Context,
	cancel context.CancelFunc,
	key statusFlightKey,
	flight *worktreeStatusFlight,
	root string,
) {
	defer cancel()
	queueStarted := c.now()
	release, err := c.acquire(ctx, key.root)
	if err != nil {
		flight.err = ctx.Err()
		c.finishWorktreeStatusFlight(key, flight)
		return
	}
	flight.metrics.QueueWait = c.now().Sub(queueStarted)
	executionStarted := c.now()
	flight.result, flight.err = c.runner.worktreeStatus(ctx, root)
	flight.metrics.Execution = c.now().Sub(executionStarted)
	release()
	c.finishWorktreeStatusFlight(key, flight)
}

func (c *fileScanCoordinator) finishWorktreeStatusFlight(key statusFlightKey, flight *worktreeStatusFlight) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if flight.err == nil && c.generations[key.root] == key.generation {
		c.worktreeCache[key.root] = worktreeStatusCacheEntry{
			result:     cloneWorktreeStatus(flight.result),
			storedAt:   c.now(),
			generation: key.generation,
		}
	}
	if current := c.worktreeFlights[key]; current == flight {
		delete(c.worktreeFlights, key)
	}
	c.removeExpiredLocked()
	close(flight.done)
}

func waitForWorktreeStatusFlight(
	ctx context.Context,
	flight *worktreeStatusFlight,
	shared bool,
) (*WorktreeStatus, FileScanMetrics, error) {
	select {
	case <-ctx.Done():
		return nil, FileScanMetrics{Shared: shared}, ctx.Err()
	case <-flight.done:
		metrics := flight.metrics
		metrics.Shared = shared
		return cloneWorktreeStatus(flight.result), metrics, flight.err
	}
}

func (c *fileScanCoordinator) invalidate(root string) {
	root = normalizeScanRoot(root)
	c.mu.Lock()
	c.generations[root]++
	for key := range c.statusCache {
		if key.root == root {
			delete(c.statusCache, key)
		}
	}
	for key := range c.statsCache {
		if key.root == root {
			delete(c.statsCache, key)
		}
	}
	delete(c.worktreeCache, root)
	c.mu.Unlock()
}

func (c *fileScanCoordinator) acquire(ctx context.Context, root string) (func(), error) {
	c.mu.Lock()
	gate := c.rootGates[root]
	if gate == nil {
		gate = make(chan struct{}, 1)
		c.rootGates[root] = gate
	}
	c.mu.Unlock()

	select {
	case gate <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case c.slots <- struct{}{}:
		return func() {
			<-c.slots
			<-gate
		}, nil
	case <-ctx.Done():
		<-gate
		return nil, ctx.Err()
	}
}

func (c *fileScanCoordinator) sharedTaskContext(ctx context.Context) (context.Context, context.CancelFunc) {
	base := context.WithoutCancel(nonNilContext(ctx))
	if deadline, ok := ctx.Deadline(); ok {
		return context.WithDeadline(base, deadline)
	}
	return context.WithTimeout(base, c.defaultTimeout)
}

func (c *fileScanCoordinator) removeExpiredLocked() {
	now := c.now()
	for key, entry := range c.statusCache {
		if entry.generation != c.generations[key.root] || now.Sub(entry.storedAt) >= c.cacheTTL {
			delete(c.statusCache, key)
		}
	}
	for key, entry := range c.statsCache {
		if entry.generation != c.generations[key.root] || now.Sub(entry.storedAt) >= c.cacheTTL {
			delete(c.statsCache, key)
		}
	}
	for key, entry := range c.worktreeCache {
		if entry.generation != c.generations[key] || now.Sub(entry.storedAt) >= c.cacheTTL {
			delete(c.worktreeCache, key)
		}
	}
}

func normalizeScanRoot(root string) string {
	cleaned := filepath.Clean(strings.TrimSpace(root))
	if absolute, err := filepath.Abs(cleaned); err == nil {
		cleaned = absolute
	}
	if runtime.GOOS == "windows" {
		cleaned = strings.ToLower(cleaned)
	}
	return cleaned
}

func nonNilContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func cloneFileStatusResult(source FileStatusResult) FileStatusResult {
	result := source
	result.Statuses = make(map[string]FileStatus, len(source.Statuses))
	for path, status := range source.Statuses {
		result.Statuses[path] = status
	}
	return result
}

func cloneDiffStats(source map[string]DiffStat) map[string]DiffStat {
	result := make(map[string]DiffStat, len(source))
	for path, stat := range source {
		result[path] = stat
	}
	return result
}

func cloneWorktreeStatus(source *WorktreeStatus) *WorktreeStatus {
	if source == nil {
		return nil
	}
	result := *source
	if source.LastCommit != nil {
		commit := *source.LastCommit
		result.LastCommit = &commit
	}
	return &result
}
