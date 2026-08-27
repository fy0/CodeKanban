package log_watcher

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"

	"go.uber.org/zap"
)

const (
	// DefaultSearchInterval is how often to search for session file
	DefaultSearchInterval = 1 * time.Second
	// DefaultMaxSearchAttempts is the maximum number of search attempts
	DefaultMaxSearchAttempts = 7
	// DefaultPollInterval is how often to poll for file changes
	DefaultPollInterval = 500 * time.Millisecond
)

// LineParserFunc is a custom line parser function
type LineParserFunc func(w *LogWatcher, line string) (*UserMessage, error)

// LogWatcher monitors AI assistant session log files
type LogWatcher struct {
	mu sync.RWMutex

	// Configuration
	processStartTime  time.Time
	searchInterval    time.Duration
	maxSearchAttempts int
	pollInterval      time.Duration
	logger            *zap.Logger

	// State
	state         WatcherState
	sessionID     string
	filePath      string
	fileOffset    int64
	linesRead     int
	lastCheckTime time.Time
	sessionMeta   *SessionMeta
	userMessages  []*UserMessage
	lastError     error

	// File searcher (e.g., for Codex)
	searcher FileSearcher

	// Custom line parser (optional, defaults to Codex format)
	parseLineFn LineParserFunc

	// Control
	ctx    context.Context
	cancel context.CancelFunc

	// Callback
	callback WatcherCallback
}

// FileSearcher interface for finding session files
type FileSearcher interface {
	// FindSessionFile searches for a session file created after the given time
	// Returns the file path if found, empty string if not found
	FindSessionFile(ctx context.Context, afterTime time.Time) (string, error)
	// GetSessionDir returns the base session directory
	GetSessionDir() string
}

// WatcherConfig contains configuration for the log watcher
type WatcherConfig struct {
	ProcessStartTime  time.Time
	SearchInterval    time.Duration
	MaxSearchAttempts int
	PollInterval      time.Duration
	Logger            *zap.Logger
	Callback          WatcherCallback
	Searcher          FileSearcher
}

// NewLogWatcher creates a new log watcher
func NewLogWatcher(config WatcherConfig) *LogWatcher {
	if config.SearchInterval <= 0 {
		config.SearchInterval = DefaultSearchInterval
	}
	if config.MaxSearchAttempts <= 0 {
		config.MaxSearchAttempts = DefaultMaxSearchAttempts
	}
	if config.PollInterval <= 0 {
		config.PollInterval = DefaultPollInterval
	}

	return &LogWatcher{
		processStartTime:  config.ProcessStartTime,
		searchInterval:    config.SearchInterval,
		maxSearchAttempts: config.MaxSearchAttempts,
		pollInterval:      config.PollInterval,
		logger:            config.Logger,
		callback:          config.Callback,
		searcher:          config.Searcher,
		state:             WatcherStateStopped,
		userMessages:      make([]*UserMessage, 0),
	}
}

// Start begins watching for session files
func (w *LogWatcher) Start(ctx context.Context) error {
	w.mu.Lock()
	if w.state == WatcherStateSearching || w.state == WatcherStateWatching {
		w.mu.Unlock()
		return nil // Already running
	}

	w.ctx, w.cancel = context.WithCancel(ctx)
	w.state = WatcherStateSearching
	w.mu.Unlock()

	go w.run()
	return nil
}

// Stop stops the watcher
func (w *LogWatcher) Stop() {
	w.mu.Lock()
	if w.cancel != nil {
		w.cancel()
	}
	w.state = WatcherStateStopped
	w.mu.Unlock()

	w.emitEvent(WatcherEvent{
		Type:      EventTypeStopped,
		Timestamp: time.Now(),
	})
}

// Info returns current watcher information
func (w *LogWatcher) Info() WatcherInfo {
	w.mu.RLock()
	defer w.mu.RUnlock()

	info := WatcherInfo{
		State:         w.state,
		SessionID:     w.sessionID,
		FilePath:      w.filePath,
		LinesRead:     w.linesRead,
		FileOffset:    w.fileOffset,
		LastCheckTime: w.lastCheckTime,
		MessageCount:  len(w.userMessages),
		SessionMeta:   w.sessionMeta,
	}

	if len(w.userMessages) > 0 {
		info.LastMessage = w.userMessages[len(w.userMessages)-1]
	}

	if w.lastError != nil {
		info.Error = w.lastError.Error()
	}

	// Copy user messages
	if len(w.userMessages) > 0 {
		info.UserMessages = make([]*UserMessage, len(w.userMessages))
		copy(info.UserMessages, w.userMessages)
	}

	return info
}

// LastUserMessage returns the most recent user message
func (w *LogWatcher) LastUserMessage() *UserMessage {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if len(w.userMessages) == 0 {
		return nil
	}
	return w.userMessages[len(w.userMessages)-1]
}

// UserMessages returns all captured user messages
func (w *LogWatcher) UserMessages() []*UserMessage {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if len(w.userMessages) == 0 {
		return nil
	}

	result := make([]*UserMessage, len(w.userMessages))
	copy(result, w.userMessages)
	return result
}

// run is the main watcher loop
func (w *LogWatcher) run() {
	// Phase 1: Search for session file
	if !w.searchForFile() {
		return
	}

	// Phase 2: Watch file for changes
	w.watchFile()
}

// searchForFile searches for the session file
func (w *LogWatcher) searchForFile() bool {
	if w.searcher == nil {
		w.setError(nil)
		return false
	}

	ticker := time.NewTicker(w.searchInterval)
	defer ticker.Stop()

	attempts := 0

	for {
		select {
		case <-w.ctx.Done():
			return false
		case <-ticker.C:
			attempts++

			filePath, err := w.searcher.FindSessionFile(w.ctx, w.processStartTime)
			if err != nil {
				if w.logger != nil {
					w.logger.Debug("error searching for session file",
						zap.Error(err),
						zap.Int("attempt", attempts))
				}
				w.setError(err)

				if attempts >= w.maxSearchAttempts {
					return false
				}
				continue
			}

			if filePath != "" {
				w.mu.Lock()
				w.filePath = filePath
				w.state = WatcherStateWatching
				w.mu.Unlock()

				if w.logger != nil {
					w.logger.Info("found session file",
						zap.String("path", filePath),
						zap.Int("attempts", attempts))
				}

				// Read initial content
				if err := w.readInitialContent(); err != nil {
					if w.logger != nil {
						w.logger.Warn("error reading initial content", zap.Error(err))
					}
				}

				w.emitEvent(WatcherEvent{
					Type:      EventTypeSessionFound,
					Timestamp: time.Now(),
				})

				return true
			}

			if attempts >= w.maxSearchAttempts {
				if w.logger != nil {
					w.logger.Warn("max search attempts reached",
						zap.Int("attempts", attempts))
				}
				return false
			}
		}
	}
}

// watchFile watches the session file for changes
func (w *LogWatcher) watchFile() {
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			return
		case <-ticker.C:
			w.checkForChanges()
		}
	}
}

// readInitialContent reads the initial content of the file
func (w *LogWatcher) readInitialContent() error {
	w.mu.RLock()
	filePath := w.filePath
	w.mu.RUnlock()

	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	return w.processFile(file, true)
}

// checkForChanges checks if the file has new content
func (w *LogWatcher) checkForChanges() {
	w.mu.RLock()
	filePath := w.filePath
	currentOffset := w.fileOffset
	w.mu.RUnlock()

	if filePath == "" {
		return
	}

	// Check file size
	stat, err := os.Stat(filePath)
	if err != nil {
		if w.logger != nil {
			w.logger.Debug("error stating file", zap.Error(err))
		}
		return
	}

	// No new content
	if stat.Size() <= currentOffset {
		return
	}

	// Open and read new content
	file, err := os.Open(filePath)
	if err != nil {
		if w.logger != nil {
			w.logger.Debug("error opening file", zap.Error(err))
		}
		return
	}
	defer file.Close()

	// Seek to last position
	if currentOffset > 0 {
		if _, err := file.Seek(currentOffset, io.SeekStart); err != nil {
			// If seek fails, reset and read from beginning
			if w.logger != nil {
				w.logger.Warn("seek failed, resetting offset", zap.Error(err))
			}
			w.resetAndReread(file)
			return
		}
	}

	if err := w.processFile(file, false); err != nil {
		// If line is too long (>4MB), skip it and update offset to continue
		if err == bufio.ErrTooLong {
			if w.logger != nil {
				w.logger.Warn("line too long, skipping", zap.Error(err))
			}
			// Update offset to current position to skip the problematic content
			newOffset, _ := file.Seek(0, io.SeekCurrent)
			w.mu.Lock()
			w.fileOffset = newOffset
			w.mu.Unlock()
			return
		}
		if w.logger != nil {
			w.logger.Warn("error processing file, resetting", zap.Error(err))
		}
		w.resetAndReread(file)
	}
}

// resetAndReread resets the offset and rereads the entire file
func (w *LogWatcher) resetAndReread(file *os.File) {
	w.mu.Lock()
	w.fileOffset = 0
	w.linesRead = 0
	w.userMessages = make([]*UserMessage, 0)
	w.sessionMeta = nil
	w.mu.Unlock()

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return
	}

	_ = w.processFile(file, true)
}

// processFile reads and processes lines from the file
func (w *LogWatcher) processFile(file *os.File, isInitial bool) error {
	scanner := bufio.NewScanner(file)
	// Increase buffer size for large lines (4MB max, skip larger lines)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 4*1024*1024) // 4MB max line size

	var newMessages []*UserMessage
	linesProcessed := 0

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			linesProcessed++
			continue
		}

		msg, err := w.parseLine(line)
		if err != nil {
			// Log but continue processing
			if w.logger != nil {
				w.logger.Debug("error parsing line", zap.Error(err))
			}
			linesProcessed++
			continue
		}

		if msg != nil {
			newMessages = append(newMessages, msg)
		}
		linesProcessed++
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	// Update state
	w.mu.Lock()
	newOffset, _ := file.Seek(0, io.SeekCurrent)
	w.fileOffset = newOffset
	w.linesRead += linesProcessed
	w.lastCheckTime = time.Now()

	w.userMessages = append(w.userMessages, newMessages...)
	w.mu.Unlock()

	// Emit events for new messages (only if not initial read)
	if !isInitial {
		for _, msg := range newMessages {
			w.emitEvent(WatcherEvent{
				Type:      EventTypeNewMessage,
				Timestamp: time.Now(),
				Message:   msg,
			})
		}
	}

	return nil
}

// parseLine parses a single JSONL line
func (w *LogWatcher) parseLine(line string) (*UserMessage, error) {
	// Use custom parser if available
	if w.parseLineFn != nil {
		return w.parseLineFn(w, line)
	}

	// Default: Codex format parser
	return w.parseCodexLine(line)
}

// parseCodexLine parses a Codex format JSONL line
func (w *LogWatcher) parseCodexLine(line string) (*UserMessage, error) {
	var entry struct {
		Timestamp string          `json:"timestamp"`
		Type      string          `json:"type"`
		Payload   json.RawMessage `json:"payload"`
	}

	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		return nil, err
	}

	switch entry.Type {
	case "session_meta":
		var payload SessionMetaPayload
		if err := json.Unmarshal(entry.Payload, &payload); err != nil {
			return nil, err
		}

		ts, _ := time.Parse(time.RFC3339, payload.Timestamp)
		meta := &SessionMeta{
			ID:           payload.ID,
			Timestamp:    ts,
			Cwd:          payload.Cwd,
			Originator:   payload.Originator,
			Source:       payload.Source,
			CliVersion:   payload.CliVersion,
			Instructions: payload.Instructions,
		}

		w.mu.Lock()
		w.sessionMeta = meta
		w.sessionID = payload.ID
		w.mu.Unlock()

		return nil, nil

	case "event_msg":
		var payload EventMsgPayload
		if err := json.Unmarshal(entry.Payload, &payload); err != nil {
			return nil, err
		}

		ts, _ := time.Parse(time.RFC3339, entry.Timestamp)

		switch payload.Type {
		case "user_message":
			images := append([]string{}, payload.Images...)
			images = append(images, payload.LocalImages...)
			return &UserMessage{
				Timestamp: ts,
				Message:   payload.Message,
				Images:    images,
			}, nil
		}

		return nil, nil

	// Skip response_item - it duplicates event_msg content

	case "turn_context":
		var payload TurnContextPayload
		if err := json.Unmarshal(entry.Payload, &payload); err != nil {
			return nil, err
		}

		// Update model info if available
		if payload.Model != "" {
			w.mu.Lock()
			if w.sessionMeta != nil {
				w.sessionMeta.Model = payload.Model
			}
			w.mu.Unlock()
		}

		return nil, nil
	}

	return nil, nil
}

// setError sets the error state
func (w *LogWatcher) setError(err error) {
	w.mu.Lock()
	w.lastError = err
	if err != nil {
		w.state = WatcherStateError
	}
	w.mu.Unlock()

	if err != nil {
		w.emitEvent(WatcherEvent{
			Type:      EventTypeError,
			Timestamp: time.Now(),
			Error:     err,
		})
	}
}

// emitEvent sends an event to the callback
func (w *LogWatcher) emitEvent(event WatcherEvent) {
	if w.callback != nil {
		w.callback(event)
	}
}
