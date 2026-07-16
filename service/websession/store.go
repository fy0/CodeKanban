package websession

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type store struct {
	rootDir        string
	attachmentsDir string
}

func newStore(dataDir string) (*store, error) {
	rootDir, err := filepath.Abs(filepath.Join(dataDir, "web-sessions"))
	if err != nil {
		return nil, err
	}
	attachmentsDir := filepath.Join(rootDir, "_attachments")
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(attachmentsDir, 0o755); err != nil {
		return nil, err
	}
	return &store{
		rootDir:        rootDir,
		attachmentsDir: attachmentsDir,
	}, nil
}

func (s *store) sessionDir(sessionID string) string {
	return filepath.Join(s.rootDir, sessionID)
}

func (s *store) historyPath(sessionID string) string {
	return filepath.Join(s.sessionDir(sessionID), "history.jsonl")
}

func (s *store) attachmentPath(id, ext string) string {
	return filepath.Join(s.attachmentsDir, fmt.Sprintf("%s%s", id, ext))
}

func (s *store) claudeHookDir(sessionID string) string {
	return filepath.Join(s.sessionDir(sessionID), "_claude-hooks")
}

func (s *store) claudeHookAnswerPath(sessionID, toolUseID string) string {
	return filepath.Join(s.claudeHookDir(sessionID), fmt.Sprintf("%s.answer.json", toolUseID))
}

func (s *store) ensureSessionDir(sessionID string) error {
	return os.MkdirAll(s.sessionDir(sessionID), 0o755)
}

func (s *store) appendEvent(sessionID string, event Event) error {
	if err := s.ensureSessionDir(sessionID); err != nil {
		return err
	}
	file, err := os.OpenFile(s.historyPath(sessionID), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	encoded, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if _, err := file.Write(append(encoded, '\n')); err != nil {
		return err
	}
	return nil
}

func (s *store) readEvents(sessionID string) ([]Event, error) {
	file, err := os.Open(s.historyPath(sessionID))
	if err != nil {
		if os.IsNotExist(err) {
			return []Event{}, nil
		}
		return nil, err
	}
	defer file.Close()

	events := make([]Event, 0, 256)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var event Event
		if err := json.Unmarshal(line, &event); err != nil {
			continue
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

func (s *store) readWindow(sessionID string, limit int, beforeSeq *int64) (HistoryWindow, error) {
	if limit <= 0 {
		limit = 80
	}
	events, err := s.readEvents(sessionID)
	if err != nil {
		return HistoryWindow{}, err
	}

	total := len(events)
	filtered := events
	if beforeSeq != nil {
		filtered = make([]Event, 0, len(events))
		for _, event := range events {
			if event.Seq < *beforeSeq {
				filtered = append(filtered, event)
			}
		}
	}

	if len(filtered) <= limit {
		return HistoryWindow{
			Events:       filtered,
			HasMore:      false,
			BeforeCursor: "",
			Total:        total,
		}, nil
	}

	start := len(filtered) - limit
	window := filtered[start:]
	hasMore := start > 0
	return HistoryWindow{
		Events:       window,
		HasMore:      hasMore,
		BeforeCursor: historyCursor(window, hasMore),
		Total:        total,
	}, nil
}

func (s *store) deleteSessionFiles(sessionID string) error {
	if err := os.RemoveAll(s.sessionDir(sessionID)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *store) deleteSessionHistory(sessionID string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	if err := os.Remove(s.historyPath(sessionID)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *store) hasSessionHistory(sessionID string) bool {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false
	}
	_, err := os.Stat(s.historyPath(sessionID))
	return err == nil
}
