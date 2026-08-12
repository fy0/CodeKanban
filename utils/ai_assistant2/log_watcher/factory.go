package log_watcher

import (
	"context"
	"time"

	"go.uber.org/zap"

	"code-kanban/utils/ai_assistant2/types"
)

// CreateWatcherForAssistant creates a LogWatcher for the specified assistant type
// Returns nil if the assistant type doesn't support log watching
func CreateWatcherForAssistant(
	assistantType types.AssistantType,
	processStartTime time.Time,
	logger *zap.Logger,
	callback WatcherCallback,
) (*LogWatcher, error) {
	return CreateWatcherForAssistantWithWorkingDir(assistantType, processStartTime, "", logger, callback)
}

// CreateWatcherForAssistantWithWorkingDir creates a LogWatcher for the specified assistant type
// workingDir is required for Claude Code to find the correct project folder
func CreateWatcherForAssistantWithWorkingDir(
	assistantType types.AssistantType,
	processStartTime time.Time,
	workingDir string,
	logger *zap.Logger,
	callback WatcherCallback,
) (*LogWatcher, error) {
	switch assistantType {
	case types.AssistantTypePi:
		searcher, err := NewPiFileSearcherWithWorkingDir(workingDir)
		if err != nil {
			return nil, err
		}
		watcher := NewLogWatcher(WatcherConfig{
			ProcessStartTime: processStartTime,
			Logger:           logger,
			Callback:         callback,
			Searcher:         searcher,
		})
		watcher.parseLineFn = ParsePiLineWrapper
		return watcher, nil

	case types.AssistantTypeCodex:
		searcher, err := NewCodexFileSearcherWithWorkingDir(workingDir)
		if err != nil {
			return nil, err
		}

		watcher := NewLogWatcher(WatcherConfig{
			ProcessStartTime: processStartTime,
			Logger:           logger,
			Callback:         callback,
			Searcher:         searcher,
		})

		return watcher, nil

	case types.AssistantTypeClaudeCode:
		if workingDir == "" {
			// Claude Code requires working directory to find project folder
			return nil, nil
		}

		searcher, err := NewClaudeCodeFileSearcher(workingDir)
		if err != nil {
			return nil, err
		}

		watcher := NewLogWatcher(WatcherConfig{
			ProcessStartTime: processStartTime,
			Logger:           logger,
			Callback:         callback,
			Searcher:         searcher,
		})

		// Use Claude Code line parser
		watcher.parseLineFn = ParseClaudeCodeLineWrapper

		return watcher, nil

	default:
		// Other assistant types don't support log watching yet
		return nil, nil
	}
}

// CreateWatcherForAssistantWithWorkingDirAndMode creates a LogWatcher with specified search mode
func CreateWatcherForAssistantWithWorkingDirAndMode(
	assistantType types.AssistantType,
	processStartTime time.Time,
	workingDir string,
	searchMode SearchMode,
	logger *zap.Logger,
	callback WatcherCallback,
) (*LogWatcher, error) {
	switch assistantType {
	case types.AssistantTypePi:
		searcher, err := NewPiFileSearcherWithWorkingDir(workingDir)
		if err != nil {
			return nil, err
		}
		watcher := NewLogWatcher(WatcherConfig{
			ProcessStartTime: processStartTime,
			Logger:           logger,
			Callback:         callback,
			Searcher:         searcher,
		})
		watcher.parseLineFn = ParsePiLineWrapper
		return watcher, nil

	case types.AssistantTypeCodex:
		searcher, err := NewCodexFileSearcherWithWorkingDir(workingDir)
		if err != nil {
			return nil, err
		}

		watcher := NewLogWatcher(WatcherConfig{
			ProcessStartTime: processStartTime,
			Logger:           logger,
			Callback:         callback,
			Searcher:         searcher,
		})

		return watcher, nil

	case types.AssistantTypeClaudeCode:
		if workingDir == "" {
			return nil, nil
		}

		searcher, err := NewClaudeCodeFileSearcher(workingDir)
		if err != nil {
			return nil, err
		}
		searcher.SetSearchMode(searchMode)

		watcher := NewLogWatcher(WatcherConfig{
			ProcessStartTime: processStartTime,
			Logger:           logger,
			Callback:         callback,
			Searcher:         searcher,
		})

		watcher.parseLineFn = ParseClaudeCodeLineWrapper

		return watcher, nil

	default:
		return nil, nil
	}
}

// CreateWatcherWithFile creates a LogWatcher for a specific file (skips file search)
func CreateWatcherWithFile(
	assistantType types.AssistantType,
	filePath string,
	logger *zap.Logger,
	callback WatcherCallback,
) (*LogWatcher, error) {
	watcher := NewLogWatcher(WatcherConfig{
		ProcessStartTime: time.Time{}, // Not used when file is specified
		Logger:           logger,
		Callback:         callback,
		Searcher:         nil, // No searcher needed
	})

	// Set the file path directly
	watcher.mu.Lock()
	watcher.filePath = filePath
	watcher.state = WatcherStateWatching
	watcher.mu.Unlock()

	// Set the appropriate line parser
	switch assistantType {
	case types.AssistantTypeClaudeCode:
		watcher.parseLineFn = ParseClaudeCodeLineWrapper
	case types.AssistantTypePi:
		watcher.parseLineFn = ParsePiLineWrapper
	}

	return watcher, nil
}

// StartWatcherForAssistant creates and starts a LogWatcher for the specified assistant type
func StartWatcherForAssistant(
	ctx context.Context,
	assistantType types.AssistantType,
	processStartTime time.Time,
	logger *zap.Logger,
	callback WatcherCallback,
) (*LogWatcher, error) {
	watcher, err := CreateWatcherForAssistant(assistantType, processStartTime, logger, callback)
	if err != nil {
		return nil, err
	}

	if watcher == nil {
		return nil, nil
	}

	if err := watcher.Start(ctx); err != nil {
		return nil, err
	}

	return watcher, nil
}
