package websession

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const eventSequenceScanChunkBytes int64 = 64 * 1024

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

func (s *store) readEventsAfter(sessionID string, afterSeq int64) ([]Event, error) {
	file, err := os.Open(s.historyPath(sessionID))
	if err != nil {
		if os.IsNotExist(err) {
			return []Event{}, nil
		}
		return nil, err
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	events := make([]Event, 0, 32)
	var previousSeq int64
	for {
		line, readErr := reader.ReadBytes('\n')
		if readErr != nil && readErr != io.EOF {
			return nil, readErr
		}
		if readErr == io.EOF && len(line) > 0 {
			return nil, fmt.Errorf("web session history has an incomplete trailing event")
		}
		if len(line) > 0 {
			line = bytes.TrimSuffix(line, []byte{'\n'})
			line = bytes.TrimSuffix(line, []byte{'\r'})
			if len(bytes.TrimSpace(line)) > 0 {
				var event Event
				if err := json.Unmarshal(line, &event); err != nil {
					return nil, fmt.Errorf("decode web session history event: %w", err)
				}
				if event.Seq <= 0 || previousSeq >= event.Seq {
					return nil, fmt.Errorf("web session history event sequences must be positive and strictly increasing")
				}
				previousSeq = event.Seq
				if event.Seq > afterSeq {
					events = append(events, event)
				}
			}
		}
		if readErr == io.EOF {
			break
		}
	}
	return events, nil
}

func (s *store) latestEventSeq(sessionID string) (int64, error) {
	file, err := os.Open(s.historyPath(sessionID))
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		return 0, err
	}
	if stat.Size() == 0 {
		return 0, nil
	}
	lastByte := []byte{0}
	if _, err := file.ReadAt(lastByte, stat.Size()-1); err != nil {
		return 0, err
	}
	if lastByte[0] != '\n' {
		return 0, fmt.Errorf("web session history has an incomplete trailing event")
	}

	lineEnd := stat.Size() - 1
	lineStart, err := previousEventLineStart(file, lineEnd)
	if err != nil {
		return 0, err
	}
	line := make([]byte, lineEnd-lineStart)
	if len(line) == 0 {
		return 0, fmt.Errorf("web session history has an empty trailing event")
	}
	if _, err := file.ReadAt(line, lineStart); err != nil {
		return 0, err
	}
	var event struct {
		Seq int64 `json:"seq"`
	}
	if err := json.Unmarshal(line, &event); err != nil {
		return 0, fmt.Errorf("decode web session history tail: %w", err)
	}
	if event.Seq <= 0 {
		return 0, fmt.Errorf("web session history tail contains an invalid event sequence")
	}
	return event.Seq, nil
}

func previousEventLineStart(file *os.File, lineEnd int64) (int64, error) {
	for cursor := lineEnd; cursor > 0; {
		start := cursor - eventSequenceScanChunkBytes
		if start < 0 {
			start = 0
		}
		chunk := make([]byte, cursor-start)
		if _, err := file.ReadAt(chunk, start); err != nil {
			return 0, err
		}
		if index := bytes.LastIndexByte(chunk, '\n'); index >= 0 {
			return start + int64(index) + 1, nil
		}
		cursor = start
	}
	return 0, nil
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
	target, err := s.deletableSessionDir(sessionID)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(target); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *store) deletableSessionDir(sessionID string) (string, error) {
	if strings.TrimSpace(sessionID) != sessionID || !isSafeSessionStorageID(sessionID) ||
		strings.EqualFold(sessionID, filepath.Base(s.attachmentsDir)) ||
		filepath.IsAbs(sessionID) || filepath.VolumeName(sessionID) != "" ||
		filepath.Base(sessionID) != sessionID || strings.ContainsAny(sessionID, `/\`) {
		return "", fmt.Errorf("invalid web session storage id %q", sessionID)
	}
	target := filepath.Join(s.rootDir, sessionID)
	relative, err := filepath.Rel(s.rootDir, target)
	if err != nil || relative != sessionID {
		return "", fmt.Errorf("invalid web session storage path for %q", sessionID)
	}
	return target, nil
}

func isSafeSessionStorageID(sessionID string) bool {
	if sessionID == "" {
		return false
	}
	for _, value := range []byte(sessionID) {
		if value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' ||
			value >= '0' && value <= '9' || value == '_' || value == '-' {
			continue
		}
		return false
	}
	return true
}

func (s *store) deleteSessionHistory(sessionID string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	target, err := s.deletableSessionDir(sessionID)
	if err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(target, "history.jsonl")); err != nil && !os.IsNotExist(err) {
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

func (s *store) sessionHistorySize(sessionID string) int64 {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return 0
	}
	info, err := os.Stat(s.historyPath(sessionID))
	if err != nil || info.IsDir() {
		return 0
	}
	return info.Size()
}
