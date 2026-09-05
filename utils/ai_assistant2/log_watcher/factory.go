package log_watcher

import (
	"time"

	"go.uber.org/zap"

	"code-kanban/utils/ai_assistant2/types"
)

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
