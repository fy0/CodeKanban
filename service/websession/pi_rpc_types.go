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

type piRPCTokenUsage struct {
	Input       int64      `json:"input"`
	Output      int64      `json:"output"`
	CacheRead   int64      `json:"cacheRead"`
	CacheWrite  int64      `json:"cacheWrite"`
	Total       int64      `json:"total"`
	TotalTokens int64      `json:"totalTokens"`
	Cost        *piRPCCost `json:"cost"`
}

type piRPCCost struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cacheRead"`
	CacheWrite float64 `json:"cacheWrite"`
	Total      float64 `json:"total"`
}

func (usage piRPCTokenUsage) totalTokens() int64 {
	if usage.TotalTokens > 0 {
		return usage.TotalTokens
	}
	if usage.Total > 0 {
		return usage.Total
	}
	return maxInt64(0, usage.Input) + maxInt64(0, usage.Output) + maxInt64(0, usage.CacheRead) + maxInt64(0, usage.CacheWrite)
}

func (usage piRPCTokenUsage) cachedInputTokens() int64 {
	return maxInt64(0, usage.CacheRead) + maxInt64(0, usage.CacheWrite)
}

func (usage piRPCTokenUsage) costTotal() float64 {
	if usage.Cost == nil {
		return 0
	}
	if usage.Cost.Total != 0 {
		return usage.Cost.Total
	}
	return usage.Cost.Input + usage.Cost.Output + usage.Cost.CacheRead + usage.Cost.CacheWrite
}

type piRPCContextUsage struct {
	Tokens        *int64   `json:"tokens"`
	ContextWindow int64    `json:"contextWindow"`
	Percent       *float64 `json:"percent"`
}

type piRPCSessionStats struct {
	Tokens       piRPCTokenUsage    `json:"tokens"`
	Cost         float64            `json:"cost"`
	SessionID    string             `json:"sessionId"`
	SessionFile  string             `json:"sessionFile"`
	ContextUsage *piRPCContextUsage `json:"contextUsage"`
}
