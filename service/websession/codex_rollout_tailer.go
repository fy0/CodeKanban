package websession

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"code-kanban/model/tables"
	"code-kanban/utils/ai_assistant2/log_watcher"
)

const codexRolloutTailerMaxLine = 8 * 1024 * 1024

type codexRolloutEntry struct {
	Timestamp string         `json:"timestamp"`
	Ordinal   *uint64        `json:"ordinal,omitempty"`
	Type      string         `json:"type"`
	Payload   map[string]any `json:"payload"`
}

type codexRolloutHandlerError struct {
	err error
}

func (e codexRolloutHandlerError) Error() string {
	return e.err.Error()
}

func (e codexRolloutHandlerError) Unwrap() error {
	return e.err
}

type codexRolloutTailer struct {
	path             string
	expectedThreadID string
	mu               sync.Mutex
	offset           int64
}

type codexRolloutMonitorEntry struct {
	tailer   *codexRolloutTailer
	failed   bool
	reported bool
}

type codexRolloutMonitor struct {
	ctx       context.Context
	cancel    context.CancelFunc
	notBefore time.Time
	handle    func(string, codexRolloutEntry) error
	report    func(string, error)

	mu       sync.Mutex
	tailers  map[string]*codexRolloutMonitorEntry
	stopped  bool
	wake     chan struct{}
	done     chan struct{}
	stopOnce sync.Once
}

func (m *Manager) prepareCodexRolloutTailer(
	ctx context.Context,
	session tables.WebSessionTable,
	threadID string,
) (*codexRolloutTailer, error) {
	return m.prepareCodexRolloutTailerAtOffset(ctx, session, threadID, false)
}

func (m *Manager) prepareCodexRolloutTailerAtOffset(
	ctx context.Context,
	session tables.WebSessionTable,
	threadID string,
	fromBeginning bool,
) (*codexRolloutTailer, error) {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return nil, fmt.Errorf("codex thread id is empty")
	}

	var lastErr error
	attach := func(path string) (*codexRolloutTailer, bool) {
		path = strings.TrimSpace(path)
		if path == "" {
			return nil, false
		}
		tailer, err := newCodexRolloutTailerAtOffset(path, threadID, fromBeginning)
		if err != nil {
			lastErr = err
			return nil, false
		}
		_ = m.updateRuntimeState(ctx, session.ID, map[string]any{
			"thread_path": path,
			"updated_at":  time.Now(),
		})
		return tailer, true
	}
	if session.ThreadPath != nil {
		if tailer, ok := attach(*session.ThreadPath); ok {
			return tailer, nil
		}
	}

	searcher, err := log_watcher.NewCodexFileSearcher()
	if err != nil {
		return nil, err
	}
	resolvedPath, err := searcher.FindBySessionID(threadID)
	if err != nil {
		return nil, err
	}
	if tailer, ok := attach(resolvedPath); ok {
		return tailer, nil
	}
	if lastErr != nil {
		return nil, fmt.Errorf("cannot attach Codex rollout for thread %s: %w", threadID, lastErr)
	}
	return nil, fmt.Errorf("cannot find Codex rollout for thread %s", threadID)
}

func (m *Manager) waitForCodexRolloutTailer(
	ctx context.Context,
	session tables.WebSessionTable,
	threadID string,
	path string,
) (*codexRolloutTailer, error) {
	threadID = strings.TrimSpace(threadID)
	path = strings.TrimSpace(path)
	if path == "" && session.ThreadPath != nil {
		path = strings.TrimSpace(*session.ThreadPath)
	}
	if ctx == nil {
		ctx = context.Background()
	}

	timer := time.NewTimer(codexRolloutAttachTimeout)
	ticker := time.NewTicker(log_watcher.DefaultPollInterval)
	defer timer.Stop()
	defer ticker.Stop()

	var lastErr error
	for {
		if path == "" {
			tailer, err := m.prepareCodexRolloutTailerAtOffset(ctx, session, threadID, true)
			if err == nil {
				return tailer, nil
			}
			lastErr = err
		} else {
			tailer, err := newCodexRolloutTailerAtOffset(path, threadID, true)
			if err == nil {
				_ = m.updateRuntimeState(ctx, session.ID, map[string]any{
					"thread_path": path,
					"updated_at":  time.Now(),
				})
				return tailer, nil
			}
			lastErr = err
			if !isRetryableCodexRolloutAttachError(err) {
				return nil, fmt.Errorf(
					"cannot attach Codex rollout for thread %s after turn/start: %w",
					threadID,
					err,
				)
			}
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
			tailer, err := m.prepareCodexRolloutTailerAtOffset(ctx, session, threadID, true)
			if err == nil {
				return tailer, nil
			}
			if lastErr != nil {
				return nil, fmt.Errorf(
					"cannot attach Codex rollout for thread %s after turn/start: %w",
					threadID,
					lastErr,
				)
			}
			return nil, err
		case <-ticker.C:
		}
	}
}

func isRetryableCodexRolloutAttachError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, os.ErrNotExist) {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "session metadata was not found")
}

func prepareCodexRolloutMonitorTailers(
	ctx context.Context,
	client *codexAppServerClient,
	rootThreadID string,
	rootTailer *codexRolloutTailer,
) (map[string]*codexRolloutTailer, []error) {
	rootThreadID = strings.TrimSpace(rootThreadID)
	tailers := make(map[string]*codexRolloutTailer)
	warnings := make([]error, 0)
	if client == nil {
		return tailers, []error{fmt.Errorf("codex app-server client is nil")}
	}
	if rootThreadID == "" || rootTailer == nil {
		return tailers, []error{fmt.Errorf("codex root rollout is unavailable")}
	}
	tailers[rootThreadID] = rootTailer
	if ctx == nil {
		ctx = context.Background()
	}
	listCtx, cancel := context.WithTimeout(ctx, codexRolloutAttachTimeout)
	descendants, err := listCodexDescendantsWithClient(listCtx, client, rootThreadID)
	cancel()
	if err != nil {
		return tailers, []error{fmt.Errorf("list Codex descendant rollouts: %w", err)}
	}
	for threadID, summary := range descendants {
		threadID = strings.TrimSpace(threadID)
		if threadID == "" || threadID == rootThreadID {
			continue
		}
		tailer, tailerErr := resolveCodexRolloutTailer(summary.Path, threadID, false)
		if tailerErr != nil {
			warnings = append(warnings, fmt.Errorf("attach Codex descendant rollout %s: %w", threadID, tailerErr))
			continue
		}
		tailers[threadID] = tailer
	}
	return tailers, warnings
}

func resolveCodexRolloutTailer(
	path string,
	threadID string,
	fromBeginning bool,
) (*codexRolloutTailer, error) {
	path = strings.TrimSpace(path)
	threadID = strings.TrimSpace(threadID)
	var pathErr error
	if path != "" {
		tailer, err := newCodexRolloutTailerAtOffset(path, threadID, fromBeginning)
		if err == nil {
			return tailer, nil
		}
		pathErr = err
	}

	searcher, err := log_watcher.NewCodexFileSearcher()
	if err != nil {
		if pathErr != nil {
			return nil, fmt.Errorf("provided path failed: %v; create rollout searcher: %w", pathErr, err)
		}
		return nil, err
	}
	resolvedPath, err := searcher.FindBySessionID(threadID)
	if err != nil {
		if pathErr != nil {
			return nil, fmt.Errorf("provided path failed: %v; find rollout: %w", pathErr, err)
		}
		return nil, err
	}
	tailer, err := newCodexRolloutTailerAtOffset(resolvedPath, threadID, fromBeginning)
	if err != nil {
		return nil, err
	}
	return tailer, nil
}

func newCodexRolloutMonitor(
	ctx context.Context,
	notBefore time.Time,
	initialTailers map[string]*codexRolloutTailer,
	handle func(string, codexRolloutEntry) error,
	report func(string, error),
) (*codexRolloutMonitor, error) {
	if len(initialTailers) == 0 {
		return nil, fmt.Errorf("codex rollout monitor has no initial tailers")
	}
	if handle == nil {
		return nil, fmt.Errorf("codex rollout monitor handler is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	monitorCtx, cancel := context.WithCancel(ctx)
	monitorTailers := make(map[string]*codexRolloutMonitorEntry, len(initialTailers))
	for threadID, tailer := range initialTailers {
		threadID = strings.TrimSpace(threadID)
		if threadID == "" || tailer == nil {
			cancel()
			return nil, fmt.Errorf("codex rollout monitor has an invalid initial tailer")
		}
		monitorTailers[threadID] = &codexRolloutMonitorEntry{tailer: tailer}
	}
	monitor := &codexRolloutMonitor{
		ctx:       monitorCtx,
		cancel:    cancel,
		notBefore: notBefore,
		handle:    handle,
		report:    report,
		tailers:   monitorTailers,
		wake:      make(chan struct{}, 1),
		done:      make(chan struct{}),
	}
	go monitor.run()
	return monitor, nil
}

func (m *codexRolloutMonitor) addThreadFromBeginning(threadID string, path string) error {
	if m == nil {
		return nil
	}
	threadID = strings.TrimSpace(threadID)
	path = strings.TrimSpace(path)
	if threadID == "" {
		return fmt.Errorf("codex descendant thread id is empty")
	}

	m.mu.Lock()
	if m.stopped {
		m.mu.Unlock()
		return nil
	}
	if _, exists := m.tailers[threadID]; exists {
		m.mu.Unlock()
		return nil
	}
	m.mu.Unlock()

	tailer, err := resolveCodexRolloutTailer(path, threadID, true)
	if err != nil {
		return err
	}

	m.mu.Lock()
	if m.stopped {
		m.mu.Unlock()
		return nil
	}
	if _, exists := m.tailers[threadID]; !exists {
		m.tailers[threadID] = &codexRolloutMonitorEntry{tailer: tailer}
	}
	m.mu.Unlock()
	select {
	case m.wake <- struct{}{}:
	default:
	}
	return nil
}

func (m *codexRolloutMonitor) hasThread(threadID string) bool {
	if m == nil {
		return false
	}
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	_, exists := m.tailers[threadID]
	return exists
}

func (m *codexRolloutMonitor) stopAndDrain() {
	if m == nil {
		return
	}
	m.stopOnce.Do(func() {
		m.mu.Lock()
		m.stopped = true
		m.mu.Unlock()
		m.cancel()
		<-m.done
		m.drain(true)
	})
}

func (m *codexRolloutMonitor) run() {
	defer close(m.done)
	m.drain(false)
	ticker := time.NewTicker(log_watcher.DefaultPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-m.wake:
			m.drain(false)
		case <-ticker.C:
			m.drain(false)
		}
	}
}

func (m *codexRolloutMonitor) drain(final bool) {
	if m == nil {
		return
	}
	type snapshotEntry struct {
		threadID string
		entry    *codexRolloutMonitorEntry
	}
	m.mu.Lock()
	snapshot := make([]snapshotEntry, 0, len(m.tailers))
	for threadID, entry := range m.tailers {
		if entry == nil || entry.tailer == nil || (entry.failed && !final) {
			continue
		}
		snapshot = append(snapshot, snapshotEntry{threadID: threadID, entry: entry})
	}
	m.mu.Unlock()

	for _, item := range snapshot {
		err := item.entry.tailer.drain(func(entry codexRolloutEntry) error {
			if !m.notBefore.IsZero() {
				observedAt, parseErr := time.Parse(time.RFC3339Nano, strings.TrimSpace(entry.Timestamp))
				if parseErr == nil && observedAt.Before(m.notBefore) {
					return nil
				}
			}
			return m.handle(item.threadID, entry)
		})
		if err == nil {
			m.mu.Lock()
			item.entry.failed = false
			item.entry.reported = false
			m.mu.Unlock()
			continue
		}
		var handlerErr codexRolloutHandlerError
		retryable := errors.As(err, &handlerErr)
		shouldReport := false
		m.mu.Lock()
		item.entry.failed = !retryable
		if !item.entry.reported {
			item.entry.reported = true
			shouldReport = true
		}
		m.mu.Unlock()
		if shouldReport && m.report != nil {
			m.report(item.threadID, err)
		}
	}
}

func newCodexRolloutTailer(path string, expectedThreadID string) (*codexRolloutTailer, error) {
	return newCodexRolloutTailerAtOffset(path, expectedThreadID, false)
}

func newCodexRolloutTailerAtOffset(
	path string,
	expectedThreadID string,
	fromBeginning bool,
) (*codexRolloutTailer, error) {
	path = strings.TrimSpace(path)
	expectedThreadID = strings.TrimSpace(expectedThreadID)
	if path == "" {
		return nil, fmt.Errorf("codex rollout path is empty")
	}
	if expectedThreadID == "" {
		return nil, fmt.Errorf("codex thread id is empty")
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	threadID, err := codexRolloutSessionID(file)
	if err != nil {
		return nil, err
	}
	if threadID != expectedThreadID {
		return nil, fmt.Errorf("codex rollout belongs to thread %s, expected %s", threadID, expectedThreadID)
	}
	offset := int64(0)
	if fromBeginning {
		offset, err = codexRolloutLocalHistoryOffset(file)
		if err != nil {
			return nil, err
		}
	} else {
		offset, err = codexRolloutCommittedOffset(file)
		if err != nil {
			return nil, err
		}
	}
	return &codexRolloutTailer{
		path:             path,
		expectedThreadID: expectedThreadID,
		offset:           offset,
	}, nil
}

func codexRolloutSessionID(file *os.File) (string, error) {
	if file == nil {
		return "", fmt.Errorf("codex rollout file is nil")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 4*1024), codexRolloutTailerMaxLine)
	for lineCount := 0; scanner.Scan() && lineCount < 16; lineCount++ {
		var entry struct {
			Type    string `json:"type"`
			Payload struct {
				ID string `json:"id"`
			} `json:"payload"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil || entry.Type != "session_meta" {
			continue
		}
		if id := strings.TrimSpace(entry.Payload.ID); id != "" {
			return id, nil
		}
		return "", fmt.Errorf("codex rollout session_meta is missing id")
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("codex rollout session_meta was not found")
}

func codexRolloutLocalHistoryOffset(file *os.File) (int64, error) {
	if file == nil {
		return 0, fmt.Errorf("codex rollout file is nil")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return 0, err
	}
	reader := bufio.NewReaderSize(file, 64*1024)
	var boundary *uint64
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(bytes.TrimSpace(line)) > 0 {
			if len(line) > codexRolloutTailerMaxLine {
				return 0, fmt.Errorf("codex rollout line exceeds %d bytes", codexRolloutTailerMaxLine)
			}
			var entry struct {
				Type    string `json:"type"`
				Payload struct {
					Start *uint64 `json:"subagent_history_start_ordinal"`
				} `json:"payload"`
			}
			if err := json.Unmarshal(bytes.TrimSpace(line), &entry); err != nil {
				return 0, err
			}
			if entry.Type != "session_meta" {
				return 0, fmt.Errorf("codex rollout does not start with session metadata")
			}
			boundary = entry.Payload.Start
			break
		}
		if readErr != nil {
			if readErr == io.EOF {
				return 0, fmt.Errorf("codex rollout session metadata was not found")
			}
			return 0, readErr
		}
	}
	if boundary == nil {
		return 0, nil
	}

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return 0, err
	}
	reader.Reset(file)
	var offset int64
	for {
		lineStart := offset
		line, readErr := reader.ReadBytes('\n')
		if readErr == io.EOF {
			return codexRolloutCommittedOffset(file)
		}
		if readErr != nil {
			return 0, readErr
		}
		offset += int64(len(line))
		if len(line) > codexRolloutTailerMaxLine {
			return 0, fmt.Errorf("codex rollout line exceeds %d bytes", codexRolloutTailerMaxLine)
		}
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var entry struct {
			Ordinal *uint64 `json:"ordinal"`
		}
		if err := json.Unmarshal(bytes.TrimSpace(line), &entry); err != nil {
			return 0, err
		}
		if entry.Ordinal != nil && *entry.Ordinal >= *boundary {
			return lineStart, nil
		}
	}
}

func codexRolloutCommittedOffset(file *os.File) (int64, error) {
	stat, err := file.Stat()
	if err != nil {
		return 0, err
	}
	size := stat.Size()
	if size == 0 {
		return 0, nil
	}

	const chunkSize int64 = 64 * 1024
	for end := size; end > 0; {
		start := end - chunkSize
		if start < 0 {
			start = 0
		}
		chunk := make([]byte, end-start)
		if _, err := file.ReadAt(chunk, start); err != nil && err != io.EOF {
			return 0, err
		}
		if index := bytes.LastIndexByte(chunk, '\n'); index >= 0 {
			return start + int64(index) + 1, nil
		}
		end = start
	}
	return 0, nil
}

func (t *codexRolloutTailer) drain(handle func(codexRolloutEntry) error) error {
	if t == nil {
		return nil
	}
	if handle == nil {
		return fmt.Errorf("codex rollout handler is nil")
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	file, err := os.Open(t.path)
	if err != nil {
		return err
	}
	defer file.Close()
	threadID, err := codexRolloutSessionID(file)
	if err != nil {
		return err
	}
	if threadID != t.expectedThreadID {
		return fmt.Errorf("codex rollout belongs to thread %s, expected %s", threadID, t.expectedThreadID)
	}

	stat, err := file.Stat()
	if err != nil {
		return err
	}
	if stat.Size() < t.offset {
		return fmt.Errorf("codex rollout was truncated")
	}
	if stat.Size() == t.offset {
		return nil
	}
	if _, err := file.Seek(t.offset, io.SeekStart); err != nil {
		return err
	}

	reader := bufio.NewReaderSize(file, 64*1024)
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > codexRolloutTailerMaxLine {
			return fmt.Errorf("codex rollout line exceeds %d bytes", codexRolloutTailerMaxLine)
		}
		if readErr == io.EOF {
			// The writer may still be appending this JSON object. Do not commit it.
			return nil
		}
		if readErr != nil {
			return readErr
		}

		nextOffset := t.offset + int64(len(line))
		line = bytes.TrimSuffix(line, []byte{'\n'})
		line = bytes.TrimSuffix(line, []byte{'\r'})
		if len(bytes.TrimSpace(line)) == 0 {
			t.offset = nextOffset
			continue
		}

		var entry codexRolloutEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			// A complete malformed line cannot become valid later; skip it.
			t.offset = nextOffset
			continue
		}
		if entry.Payload == nil {
			entry.Payload = map[string]any{}
		}
		if err := handle(entry); err != nil {
			return codexRolloutHandlerError{err: err}
		}
		t.offset = nextOffset
	}
}
