package websession

import "encoding/json"

type piRPCResponse struct {
	Type    string          `json:"type"`
	ID      string          `json:"id"`
	Command string          `json:"command"`
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   string          `json:"error"`
}

type piRPCEvent struct {
	Type              string          `json:"type"`
	Raw               json.RawMessage `json:"-"`
	BarrierRunID      string          `json:"-"`
	BarrierGeneration uint64          `json:"-"`
	BarrierDone       chan struct{}   `json:"-"`
	WakePending       bool            `json:"-"`
}

type piRPCState struct {
	Model *struct {
		Provider string `json:"provider"`
		ID       string `json:"id"`
		Name     string `json:"name"`
	} `json:"model"`
	ThinkingLevel string `json:"thinkingLevel"`
	IsStreaming   bool   `json:"isStreaming"`
	SessionID     string `json:"sessionId"`
	SessionFile   string `json:"sessionFile"`
	SessionName   string `json:"sessionName"`
}

type piRPCModel struct {
	Provider      string   `json:"provider"`
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Reasoning     bool     `json:"reasoning"`
	Input         []string `json:"input"`
	ContextWindow int64    `json:"contextWindow"`
	MaxTokens     int64    `json:"maxTokens"`
}

type piRPCAvailableModels struct {
	Models []piRPCModel `json:"models"`
}

type piRPCAvailableThinkingLevels struct {
	Levels []string `json:"levels"`
}

type piRPCSetModelResult struct {
	Provider string `json:"provider"`
	ID       string `json:"id"`
	Name     string `json:"name"`
}

type piRPCImage struct {
	Type     string `json:"type"`
	Data     string `json:"data"`
	MimeType string `json:"mimeType"`
}

type piRPCSessionStats struct {
	Tokens struct {
		Input      int64 `json:"input"`
		Output     int64 `json:"output"`
		CacheRead  int64 `json:"cacheRead"`
		CacheWrite int64 `json:"cacheWrite"`
		Total      int64 `json:"total"`
	} `json:"tokens"`
	Cost         float64 `json:"cost"`
	SessionID    string  `json:"sessionId"`
	SessionFile  string  `json:"sessionFile"`
	ContextUsage *struct {
		Tokens        int64   `json:"tokens"`
		ContextWindow int64   `json:"contextWindow"`
		Percent       float64 `json:"percent"`
	} `json:"contextUsage"`
}
