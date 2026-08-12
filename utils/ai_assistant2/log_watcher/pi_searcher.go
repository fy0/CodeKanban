package log_watcher

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const piSessionHeaderMaxBytes = 256 * 1024

type PiSessionHeader struct {
	Type          string `json:"type"`
	Version       int    `json:"version"`
	ID            string `json:"id"`
	Timestamp     string `json:"timestamp"`
	Cwd           string `json:"cwd"`
	ParentSession string `json:"parentSession,omitempty"`
}

type PiFileSearcher struct {
	sessionDir           string
	normalizedWorkingDir string
}

type piSessionCandidate struct {
	path      string
	startedAt time.Time
	modTime   time.Time
	header    PiSessionHeader
}

func ResolvePiSessionDir() (string, error) {
	if override := strings.TrimSpace(os.Getenv("PI_CODING_AGENT_SESSION_DIR")); override != "" {
		return normalizePiConfiguredPath(override)
	}

	agentDir := strings.TrimSpace(os.Getenv("PI_CODING_AGENT_DIR"))
	if agentDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		agentDir = filepath.Join(homeDir, ".pi", "agent")
	}
	resolved, err := normalizePiConfiguredPath(agentDir)
	if err != nil {
		return "", err
	}
	return filepath.Join(resolved, "sessions"), nil
}

func normalizePiConfiguredPath(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", errors.New("Pi session path is empty")
	}
	if value == "~" || strings.HasPrefix(value, "~/") || strings.HasPrefix(value, `~\`) {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if value == "~" {
			value = homeDir
		} else {
			value = filepath.Join(homeDir, value[2:])
		}
	}
	value = filepath.Clean(filepath.FromSlash(value))
	if filepath.IsAbs(value) {
		return value, nil
	}
	return filepath.Abs(value)
}

func NewPiFileSearcherWithWorkingDir(workingDir string) (*PiFileSearcher, error) {
	sessionDir, err := ResolvePiSessionDir()
	if err != nil {
		return nil, err
	}
	return NewPiFileSearcherWithSessionDir(sessionDir, workingDir), nil
}

func NewPiFileSearcherWithSessionDir(sessionDir, workingDir string) *PiFileSearcher {
	return &PiFileSearcher{
		sessionDir:           filepath.Clean(sessionDir),
		normalizedWorkingDir: normalizeComparablePath(workingDir),
	}
}

func (s *PiFileSearcher) GetSessionDir() string {
	return s.sessionDir
}

func (s *PiFileSearcher) FindSessionFile(ctx context.Context, afterTime time.Time) (string, error) {
	candidates, err := s.findCandidates(ctx, func(candidate piSessionCandidate) bool {
		if s.normalizedWorkingDir != "" && !sameComparablePath(candidate.header.Cwd, s.normalizedWorkingDir) {
			return false
		}
		if afterTime.IsZero() {
			return true
		}
		tolerance := 100 * time.Millisecond
		return !candidate.startedAt.Add(tolerance).Before(afterTime) ||
			!candidate.modTime.Add(tolerance).Before(afterTime)
	})
	if err != nil || len(candidates) == 0 {
		return "", err
	}
	sort.Slice(candidates, func(i, j int) bool {
		if !afterTime.IsZero() {
			leftDelta := piSessionActivityDelta(candidates[i], afterTime)
			rightDelta := piSessionActivityDelta(candidates[j], afterTime)
			if leftDelta != rightDelta {
				return leftDelta < rightDelta
			}
		}
		if !candidates[i].modTime.Equal(candidates[j].modTime) {
			return candidates[i].modTime.After(candidates[j].modTime)
		}
		if !candidates[i].startedAt.Equal(candidates[j].startedAt) {
			return candidates[i].startedAt.After(candidates[j].startedAt)
		}
		return candidates[i].path > candidates[j].path
	})
	return candidates[0].path, nil
}

func piSessionActivityDelta(candidate piSessionCandidate, afterTime time.Time) time.Duration {
	startedDelta := absDuration(candidate.startedAt.Sub(afterTime))
	modifiedDelta := absDuration(candidate.modTime.Sub(afterTime))
	if modifiedDelta < startedDelta {
		return modifiedDelta
	}
	return startedDelta
}

func (s *PiFileSearcher) FindBySessionID(ctx context.Context, sessionID string) (string, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "", nil
	}
	candidates, err := s.findCandidates(ctx, func(candidate piSessionCandidate) bool {
		return candidate.header.ID == sessionID
	})
	if err != nil || len(candidates) == 0 {
		return "", err
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].modTime.Equal(candidates[j].modTime) {
			return candidates[i].path > candidates[j].path
		}
		return candidates[i].modTime.After(candidates[j].modTime)
	})
	return candidates[0].path, nil
}

func (s *PiFileSearcher) findCandidates(
	ctx context.Context,
	accept func(piSessionCandidate) bool,
) ([]piSessionCandidate, error) {
	candidates := make([]piSessionCandidate, 0, 8)
	err := filepath.WalkDir(s.sessionDir, func(path string, entry os.DirEntry, walkErr error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if walkErr != nil || entry == nil || entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".jsonl") {
			return nil
		}
		header, err := ReadPiSessionHeader(path)
		if err != nil {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		startedAt, err := time.Parse(time.RFC3339Nano, header.Timestamp)
		if err != nil {
			startedAt = getFileCreationTime(path, info)
		}
		candidate := piSessionCandidate{
			path:      path,
			startedAt: startedAt,
			modTime:   info.ModTime(),
			header:    header,
		}
		if accept == nil || accept(candidate) {
			candidates = append(candidates, candidate)
		}
		return nil
	})
	if errors.Is(err, os.ErrNotExist) {
		return candidates, nil
	}
	return candidates, err
}

func ReadPiSessionHeader(filePath string) (PiSessionHeader, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return PiSessionHeader{}, err
	}
	defer file.Close()

	reader := bufio.NewReader(io.LimitReader(file, piSessionHeaderMaxBytes+1))
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return PiSessionHeader{}, err
	}
	if len(line) > piSessionHeaderMaxBytes {
		return PiSessionHeader{}, errors.New("Pi session header is too large")
	}
	var header PiSessionHeader
	if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &header); err != nil {
		return PiSessionHeader{}, err
	}
	if header.Type != "session" || strings.TrimSpace(header.ID) == "" || strings.TrimSpace(header.Cwd) == "" {
		return PiSessionHeader{}, errors.New("invalid Pi session header")
	}
	return header, nil
}

func ParsePiLineWrapper(w *LogWatcher, line string) (*UserMessage, error) {
	var entry struct {
		Type      string          `json:"type"`
		ID        string          `json:"id"`
		Timestamp string          `json:"timestamp"`
		Cwd       string          `json:"cwd"`
		Provider  string          `json:"provider"`
		ModelID   string          `json:"modelId"`
		Message   json.RawMessage `json:"message"`
	}
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		return nil, err
	}

	switch entry.Type {
	case "session":
		ts, _ := time.Parse(time.RFC3339Nano, entry.Timestamp)
		w.mu.Lock()
		w.sessionID = entry.ID
		w.sessionMeta = &SessionMeta{
			ID:         entry.ID,
			Timestamp:  ts,
			Cwd:        entry.Cwd,
			Originator: "pi",
		}
		w.mu.Unlock()
	case "model_change":
		model := strings.Trim(strings.TrimSpace(entry.Provider)+"/"+strings.TrimSpace(entry.ModelID), "/")
		if model != "" {
			w.mu.Lock()
			if w.sessionMeta != nil {
				w.sessionMeta.Model = model
			}
			w.mu.Unlock()
		}
	case "message":
		var message struct {
			Role      string          `json:"role"`
			Content   json.RawMessage `json:"content"`
			Timestamp int64           `json:"timestamp"`
		}
		if err := json.Unmarshal(entry.Message, &message); err != nil {
			return nil, err
		}
		if message.Role != "user" {
			return nil, nil
		}
		text := piMessageText(message.Content)
		if strings.TrimSpace(text) == "" {
			return nil, nil
		}
		ts, _ := time.Parse(time.RFC3339Nano, entry.Timestamp)
		if message.Timestamp > 0 {
			ts = time.UnixMilli(message.Timestamp)
		}
		return &UserMessage{Timestamp: ts, Message: text}, nil
	}
	return nil, nil
}

func piMessageText(raw json.RawMessage) string {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if block.Type == "text" && strings.TrimSpace(block.Text) != "" {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, "\n")
}
