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
	case "item/started", "item/completed", "item/agentMessage/delta", "item/plan/delta", "item/reasoning/textDelta",
		"item/reasoning/summaryTextDelta", "rawResponseItem/completed", "rawResponse/completed", "thread/tokenUsage/updated":
	default:
		return
	}
	payload := decodeRawObject(raw)
	force := method == "item/started" || method == "item/completed" ||
		method == "rawResponse/completed" || method == "thread/tokenUsage/updated"
	m.updateTokenSpeed(session.ID, run.runID, func(s *tokenSpeedState, now time.Time) {
		if threadID := codexNotificationThreadID(raw); threadID != "" && s.codexThreadID == "" {
			s.codexThreadID = threadID
			// A fresh native thread starts its cumulative counter at zero, even
			// when the web session retains usage from the previous context.
			if threadID != strings.TrimSpace(pointerString(session.NativeSessionID)) {
				s.codexTotal = 0
			}
		}
		s.observeCodexTokenSpeed(method, payload, now)
	}, force)
}

func (s *tokenSpeedState) observeCodexTokenSpeed(method string, payload map[string]any, now time.Time) {
	item := decodeRawObject(payload["item"])
	kind := normalizeCodexItemType(stringValue(item["type"]))
	switch method {
	case "item/started":
		switch kind {
		case "reasoning", "agent_message", "plan":
			if sample := s.beginCodexItem(stringValue(item["id"]), now, true); sample != nil && kind == "reasoning" {
				sample.reasoningObserved = true
			}
		case "user_message":
		default:
			channel := "tool:" + stringValue(item["id"])
			sample := s.codexItemSample(channel)
			if sample == nil {
				sample = s.current()
				if sample != nil {
					sample.weights[channel] = 0
				}
			}
			s.finishSample(sample, nil, now, true)
		}
	case "item/agentMessage/delta", "item/plan/delta", "item/reasoning/textDelta", "item/reasoning/summaryTextDelta":
		text := stringValue(payload["delta"])
		if text == "" || s.stopped {
			return
		}
		channel := stringValue(payload["itemId"])
		sample := s.beginCodexItem(channel, now, false)
		if sample == nil || !sample.ended.IsZero() {
			return
		}
		if method == "item/reasoning/summaryTextDelta" {
			channel += ":summary"
		}
		if method == "item/reasoning/textDelta" {
			delete(sample.weights, channel+":summary")
		}
		if method == "item/reasoning/summaryTextDelta" {
			if sample.weights[strings.TrimSuffix(channel, ":summary")] > 0 {
				return
			}
		}
		sample.weights[channel] += estimateOutputTokens(text)
		sample.codexOutputAt = now
	case "item/completed":
		if kind == "agent_message" || kind == "plan" || kind == "reasoning" {
			channel := stringValue(item["id"])
			sample := s.codexItemSample(channel)
			if sample == nil {
				sample = s.current()
			}
			if sample != nil {
				// Hidden reasoning can have no text deltas. Its item completion
				// still marks generated output, unlike a later usage/done signal.
				if kind == "reasoning" && sample.ended.IsZero() {
					sample.codexOutputAt = now
				}
				if text := stringValue(item["text"]); text != "" {
					if sample.weights[channel] == 0 && sample.ended.IsZero() {
						sample.codexOutputAt = now
					}
					sample.weights[channel] = estimateOutputTokens(text)
					s.recordSample(sample)
				}
			}
		}
	case "rawResponseItem/completed":
		if kind == "function_call" || kind == "custom_tool_call" {
			channel := "tool:" + stringValue(item["call_id"])
			sample := s.codexItemSample(channel)
			if sample == nil {
				sample = s.current()
			}
			if sample != nil {
				text := stringValue(item["arguments"])
				if kind == "custom_tool_call" {
					text = stringValue(item["input"])
				}
				sample.weights[channel] = estimateOutputTokens(text)
				if text != "" && sample.ended.IsZero() {
					sample.codexOutputAt = now
				}
				s.recordSample(sample)
			}
		}
	case "rawResponse/completed":
		responseID := stringValue(payload["responseId"])
		if responseID != "" {
			if s.codexResponses[responseID] {
				return
			}
			if s.codexResponses == nil {
				s.codexResponses = make(map[string]bool)
			}
			s.codexResponses[responseID] = true
		}
		// Commands may have already ended this response's generation, and a
		// later response may be streaming by the time completion is delivered.
		var sample *tokenSpeedSample
		for _, pending := range s.samples {
			if !pending.codexResponseComplete {
				sample = pending
				sample.codexResponseComplete = true
				break
			}
		}
		usage := decodeRawObject(payload["usage"])
		if output, ok := codexInt64Field(usage, "outputTokens", "output_tokens"); ok {
			if sample == nil || !sample.usageClaimed {
				s.codexPendingOutput += output
			}
			if sample != nil {
				// Per-response usage is authoritative, including a correction
				// below an earlier cumulative usage estimate.
				sample.output = nil
			}
			s.finishCodexSample(sample, usage, now)
		} else {
			s.finishSample(sample, nil, codexGenerationEnd(sample, now), true)
		}
	case "thread/tokenUsage/updated":
		usage := decodeRawObject(payload["tokenUsage"])
		total, hasTotal := parseCodexTokenUsageSnapshot(usage["total"])
		last, hasLast := parseCodexTokenUsageSnapshot(usage["last"])
		if !hasTotal || total.OutputTokens <= s.codexTotal {
			return
		}
		delta := total.OutputTokens - s.codexTotal
		s.codexTotal = total.OutputTokens
		// A cumulative notification can arrive after the next request starts.
		// Consume usage already attributed by rawResponse/completed first.
		claimed := min(delta, s.codexPendingOutput)
		s.codexPendingOutput -= claimed
		delta -= claimed
		if delta == 0 {
			return
		}
		var pending []*tokenSpeedSample
		for _, sample := range s.samples {
			if !sample.usageClaimed {
				pending = append(pending, sample)
			}
		}
		// Without a response id, only an unambiguous single request can be
		// calibrated. A batched or delayed report must not retarget a new request.
		if len(pending) == 1 && hasLast && delta == last.OutputTokens {
			s.finishCodexSample(pending[0], decodeRawObject(usage["last"]), now)
		} else {
			for _, sample := range pending {
				sample.usageClaimed = true
			}
		}
	}
}

func (s *tokenSpeedState) codexItemSample(channel string) *tokenSpeedSample {
	if channel != "" {
		for i := len(s.samples) - 1; i >= 0; i-- {
			if _, ok := s.samples[i].weights[channel]; ok {
				return s.samples[i]
			}
		}
	}
	return nil
}

func (s *tokenSpeedState) beginCodexItem(channel string, now time.Time, complete bool) *tokenSpeedSample {
	if sample := s.codexItemSample(channel); sample != nil {
		return sample
	}
	sample := s.begin("", now, complete)
	if sample != nil && channel != "" {
		sample.weights[channel] = 0
	}
	return sample
}

func (s *tokenSpeedState) finishCodexSample(sample *tokenSpeedSample, usage map[string]any, now time.Time) {
	if sample == nil {
		return
	}
	reasoning, _ := codexInt64Field(usage, "reasoningOutputTokens", "reasoning_output_tokens")
	if reasoning > 0 && !sample.reasoningObserved {
		sample.completeBoundary = false
	}
	// outputTokens includes reasoningOutputTokens; reasoning is a breakdown,
	// not an additional output count. Timing comes from local event boundaries.
	output, ok := codexInt64Field(usage, "outputTokens", "output_tokens")
	if ok {
		s.finishSample(sample, &output, codexGenerationEnd(sample, now), true)
	}
}

func codexGenerationEnd(sample *tokenSpeedSample, now time.Time) time.Time {
	// Usage and done notifications can lag behind the actual output. They
	// calibrate the count without adding their delivery delay to generation.
	if sample != nil && !sample.codexOutputAt.IsZero() && !sample.codexOutputAt.Before(sample.started) && !sample.codexOutputAt.After(now) {
		return sample.codexOutputAt
	}
	return now
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
