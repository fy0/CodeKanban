package websession

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"code-kanban/model/tables"
)

func (m *Manager) observeCodexTokenSpeed(session tables.WebSessionTable, run *activeRun, method string, raw json.RawMessage) {
	if run == nil {
		return
	}
	switch method {
	case "item/started", "item/completed", "item/agentMessage/delta", "item/reasoning/textDelta",
		"item/reasoning/summaryTextDelta", "rawResponseItem/completed", "rawResponse/completed", "thread/tokenUsage/updated":
	default:
		return
	}
	payload := decodeRawObject(raw)
	force := method == "rawResponse/completed" || method == "thread/tokenUsage/updated"
	m.updateTokenSpeed(session.ID, run.runID, func(s *tokenSpeedState, now time.Time) {
		if threadID := codexNotificationThreadID(raw); threadID != "" && s.codexThreadID == "" {
			s.codexThreadID = threadID
			// A fresh native thread starts its cumulative counter at zero, even
			// when the web session retains usage from the previous context.
			if threadID != strings.TrimSpace(pointerString(session.NativeSessionID)) {
				s.codexTotal = 0
			}
		}
		item := decodeRawObject(payload["item"])
		kind := normalizeCodexItemType(stringValue(item["type"]))
		switch method {
		case "item/started":
			switch kind {
			case "reasoning", "agent_message":
				if sample := s.begin("", now, true); sample != nil && kind == "reasoning" {
					sample.reasoningObserved = true
				}
			case "user_message":
			default:
				s.finish("", nil, now, true)
			}
		case "item/agentMessage/delta", "item/reasoning/textDelta", "item/reasoning/summaryTextDelta":
			channel := stringValue(payload["itemId"])
			if method == "item/reasoning/summaryTextDelta" {
				channel += ":summary"
			}
			if method == "item/reasoning/textDelta" {
				if cur := s.current(); cur != nil {
					delete(cur.weights, channel+":summary")
				}
			}
			if method == "item/reasoning/summaryTextDelta" {
				if cur := s.current(); cur != nil && cur.weights[strings.TrimSuffix(channel, ":summary")] > 0 {
					return
				}
			}
			s.delta("", channel, stringValue(payload["delta"]), now)
		case "item/completed":
			if kind == "agent_message" {
				if cur := s.current(); cur != nil && cur.ended.IsZero() {
					cur.weights[stringValue(item["id"])] = estimateOutputTokens(stringValue(item["text"]))
				}
			}
		case "rawResponseItem/completed":
			if stringValue(item["type"]) == "function_call" {
				if cur := s.current(); cur != nil && cur.ended.IsZero() {
					cur.weights["tool:"+stringValue(item["call_id"])] = estimateOutputTokens(stringValue(item["arguments"]))
				}
			}
		case "rawResponse/completed":
			s.finish("", nil, now, true)
		case "thread/tokenUsage/updated":
			usage := decodeRawObject(payload["tokenUsage"])
			total, hasTotal := parseCodexTokenUsageSnapshot(usage["total"])
			last, hasLast := parseCodexTokenUsageSnapshot(usage["last"])
			if !hasTotal || total.OutputTokens <= s.codexTotal {
				return
			}
			delta := total.OutputTokens - s.codexTotal
			s.codexTotal = total.OutputTokens
			var pending []*tokenSpeedSample
			for _, sample := range s.samples {
				if !sample.usageClaimed {
					pending = append(pending, sample)
				}
			}
			// Without a response id, only an unambiguous single request can be
			// calibrated. A batched or delayed report must not retarget a new request.
			if len(pending) == 1 && hasLast && delta == last.OutputTokens {
				lastRaw := decodeRawObject(usage["last"])
				reasoning, _ := codexInt64Field(lastRaw, "reasoningOutputTokens", "reasoning_output_tokens")
				if reasoning > 0 && !pending[0].reasoningObserved {
					pending[0].completeBoundary = false
				}
				s.finish(pending[0].key, &last.OutputTokens, now, true)
			} else {
				for _, sample := range pending {
					sample.usageClaimed = true
				}
			}
		}
	}, force)
}

func (m *Manager) observeClaudeTokenSpeed(session tables.WebSessionTable, run *activeRun, raw map[string]any) {
	if run == nil || strings.TrimSpace(stringValue(raw["parent_tool_use_id"])) != "" || raw["isSynthetic"] == true || raw["isMeta"] == true || raw["isCompactSummary"] == true {
		return
	}
	kind := stringValue(raw["type"])
	if kind != "stream_event" && kind != "assistant" {
		return
	}
	m.updateTokenSpeed(session.ID, run.runID, func(s *tokenSpeedState, now time.Time) {
		if kind == "assistant" {
			message := decodeRawObject(raw["message"])
			key := stringValue(message["id"])
			if key == "" {
				return
			}
			usage := decodeRawObject(message["usage"])
			var output *int64
			if value, ok := usage["output_tokens"]; ok {
				n := int64(numberValue(value))
				output = &n
			}
			s.finish(key, output, now, true)
			return
		}
		event := decodeRawObject(raw["event"])
		key := s.claudeMessageID
		switch stringValue(event["type"]) {
		case "message_start":
			key = stringValue(decodeRawObject(event["message"])["id"])
			if key == "" {
				return
			}
			s.claudeMessageID = key
			s.begin(key, now, true)
		case "content_block_start":
			if key == "" {
				return
			}
			block := decodeRawObject(event["content_block"])
			s.delta(key, strconv.Itoa(int(numberValue(event["index"]))), stringValue(block["text"])+stringValue(block["thinking"]), now)
		case "content_block_delta":
			if key == "" {
				return
			}
			delta := decodeRawObject(event["delta"])
			text := stringValue(delta["text"]) + stringValue(delta["thinking"]) + stringValue(delta["partial_json"])
			s.delta(key, strconv.Itoa(int(numberValue(event["index"]))), text, now)
		case "message_delta":
			if key == "" {
				return
			}
			usage := decodeRawObject(event["usage"])
			if value, ok := usage["output_tokens"]; ok {
				output := int64(numberValue(value))
				if cur := s.current(); cur != nil && cur.key == key && output >= 0 {
					cur.reportedOutput = &output
					if stringValue(decodeRawObject(event["delta"])["stop_reason"]) != "" {
						s.finish(key, &output, now, true)
					}
				}
			}
		case "message_stop":
			if key != "" {
				var output *int64
				if cur := s.current(); cur != nil && cur.key == key {
					output = cur.reportedOutput
				}
				s.finish(key, output, now, true)
			}
		}
	}, kind == "assistant" || stringValue(decodeRawObject(raw["event"])["type"]) == "message_stop")
}

func (m *Manager) observeDevinTokenSpeed(session tables.WebSessionTable, run *activeRun, proj *devinRunProjection, update map[string]any, contextID string) {
	if run == nil || proj.captureHistory {
		return
	}
	kind := stringValue(update["sessionUpdate"])
	meta := decodeRawObject(update["_meta"])
	m.updateTokenSpeed(session.ID, run.runID, func(s *tokenSpeedState, now time.Time) {
		// Bare usage is also emitted for subagents. Once a subagent is present,
		// reports without request identity cannot safely calibrate the root.
		if contextID != "" || meta["cognition.ai/subagent_started"] != nil {
			s.devinAmbiguous = true
		}
		if contextID != "" {
			return
		}
		switch kind {
		case "agent_message_chunk", "agent_thought_chunk":
			text := devinChunkText(update)
			if text == "" {
				return
			}
			s.begin("", now, false)
			s.delta("", kind, text, now)
		case "tool_call":
			s.finish("", nil, now, true)
		case "usage_update":
			// Only an explicitly root-tagged report has trustworthy ownership.
			if stringValue(decodeRawObject(meta["cognition.ai/subagent_context"])["parentAgentId"]) != "root" {
				return
			}
			signature := devinUsageSignature(update)
			if signature == "" || s.devinUsageSeen[signature] {
				return
			}
			if s.devinUsageSeen == nil {
				s.devinUsageSeen = make(map[string]bool)
			}
			s.devinUsageSeen[signature] = true
			values := parseDevinUsageUpdate(update)
			if !values.hasTokens {
				return
			}
			// ACP does not supply a reliable generation start boundary; keep the
			// approximation marker even when its token count can be corrected.
			cur := s.current()
			if cur != nil && !cur.usageClaimed {
				s.finish("", &values.output, now, false)
			}
		}
	}, kind == "tool_call" || kind == "usage_update")
}
