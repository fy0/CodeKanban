package websession

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPiRPCFakeProcess(t *testing.T) {
	if os.Getenv("CODEKANBAN_PI_RPC_FAKE") != "1" {
		return
	}
	mode := os.Getenv("CODEKANBAN_PI_RPC_FAKE_MODE")
	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	switch mode {
	case "ordered":
		commands := make([]map[string]any, 0, 2)
		for scanner.Scan() {
			var command map[string]any
			if json.Unmarshal(scanner.Bytes(), &command) != nil {
				os.Exit(2)
			}
			commands = append(commands, command)
			if len(commands) != 2 {
				continue
			}
			_ = encoder.Encode(map[string]any{"type": "agent_start"})
			for index := len(commands) - 1; index >= 0; index-- {
				item := commands[index]
				_ = encoder.Encode(map[string]any{
					"type": "response", "id": item["id"], "command": item["type"],
					"success": true, "data": map[string]any{"value": item["type"]},
				})
			}
		}
	case "burst":
		if scanner.Scan() {
			var command map[string]any
			_ = json.Unmarshal(scanner.Bytes(), &command)
			for index := 0; index < 512; index++ {
				_ = encoder.Encode(map[string]any{"type": "agent_start", "index": index})
			}
			_ = encoder.Encode(map[string]any{
				"type": "response", "id": command["id"], "command": command["type"],
				"success": true, "data": map[string]any{"ok": true},
			})
		}
	case "malformed":
		if scanner.Scan() {
			_, _ = fmt.Fprintln(os.Stdout, `{not-json`)
		}
	case "partial":
		if scanner.Scan() {
			_, _ = io.WriteString(os.Stdout, `{"type":"response"`)
		}
	case "unknown", "mismatch":
		if scanner.Scan() {
			var command map[string]any
			_ = json.Unmarshal(scanner.Bytes(), &command)
			if mode == "unknown" {
				command["id"] = "unknown-request"
			} else {
				command["type"] = "different_command"
			}
			_ = encoder.Encode(map[string]any{
				"type": "response", "id": command["id"], "command": command["type"],
				"success": true, "data": map[string]any{"ok": true},
			})
		}
	case "barrier":
		for scanner.Scan() {
			var command map[string]any
			if json.Unmarshal(scanner.Bytes(), &command) != nil {
				os.Exit(2)
			}
			if command["type"] != "get_state" {
				continue
			}
			_ = encoder.Encode(map[string]any{"type": "extension_ui_request", "id": "next-dialog", "method": "input"})
			_ = encoder.Encode(map[string]any{
				"type": "response", "id": command["id"], "command": command["type"],
				"success": true, "data": map[string]any{"isStreaming": true},
			})
		}
	case "stderr":
		if scanner.Scan() {
			_, _ = io.WriteString(os.Stderr, strings.Repeat("x", piRPCStderrLimit*2))
			var command map[string]any
			_ = json.Unmarshal(scanner.Bytes(), &command)
			_ = encoder.Encode(map[string]any{
				"type": "response", "id": command["id"], "command": command["type"],
				"success": true, "data": map[string]any{"ok": true},
			})
		}
	case "exit", "exit_stderr":
		if scanner.Scan() {
			if mode == "exit_stderr" {
				_, _ = fmt.Fprintln(os.Stderr, "portable launcher failed")
			}
			os.Exit(7)
		}
	case "hang":
		if scanner.Scan() {
			time.Sleep(30 * time.Second)
		}
	default:
		os.Exit(3)
	}
	os.Exit(0)
}

func fakePiRPCCommand(t *testing.T, mode string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestPiRPCFakeProcess$")
	cmd.Env = append(os.Environ(),
		"CODEKANBAN_PI_RPC_FAKE=1",
		"CODEKANBAN_PI_RPC_FAKE_MODE="+mode,
	)
	return cmd
}

func TestReadPiRPCJSONLFrameBoundaries(t *testing.T) {
	payload := "{\"value\":\"line\u2028separator\u2029ok\"}\r\n{\"next\":true}\n"
	reader := bufio.NewReaderSize(strings.NewReader(payload), 8)
	first, err := readPiRPCJSONLFrame(reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != "{\"value\":\"line\u2028separator\u2029ok\"}" {
		t.Fatalf("unexpected first frame: %q", first)
	}
	second, err := readPiRPCJSONLFrame(reader, 1024)
	if err != nil || string(second) != `{"next":true}` {
		t.Fatalf("unexpected second frame %q: %v", second, err)
	}
	if _, err := readPiRPCJSONLFrame(reader, 1024); !errors.Is(err, io.EOF) {
		t.Fatalf("expected EOF, got %v", err)
	}
}

func TestReadPiRPCJSONLFrameRejectsPartialAndOversized(t *testing.T) {
	if _, err := readPiRPCJSONLFrame(bufio.NewReader(strings.NewReader(`{"type":"event"}`)), 1024); err == nil || !strings.Contains(err.Error(), "partial") {
		t.Fatalf("expected partial frame error, got %v", err)
	}
	if _, err := readPiRPCJSONLFrame(bufio.NewReader(bytes.NewReader(append(bytes.Repeat([]byte("x"), 20), '\n'))), 10); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected oversized frame error, got %v", err)
	}
}

func TestReadPiRPCJSONLFrameDiscardsRedundantOversizedEvents(t *testing.T) {
	ignored := `{"type":"agent_end","messages":["` + strings.Repeat("x", 256) + `"]}` + "\n"
	projected := `{"type":"message_end","message":{"role":"assistant"}}` + "\n"
	reader := bufio.NewReaderSize(strings.NewReader(ignored+projected), 16)

	frame, discarded, err := readPiRPCJSONLFrameForClient(reader, 64, 1024)
	if err != nil || !discarded || frame != nil {
		t.Fatalf("redundant frame = %q, discarded=%v, err=%v", frame, discarded, err)
	}
	frame, discarded, err = readPiRPCJSONLFrameForClient(reader, 64, 1024)
	if err != nil || discarded || string(frame) != strings.TrimSuffix(projected, "\n") {
		t.Fatalf("projected frame = %q, discarded=%v, err=%v", frame, discarded, err)
	}
}

func TestReadPiRPCJSONLFrameStillBoundsProjectedEvents(t *testing.T) {
	projected := `{"type":"message_end","message":{"content":"` + strings.Repeat("x", 256) + `"}}` + "\n"
	_, _, err := readPiRPCJSONLFrameForClient(bufio.NewReaderSize(strings.NewReader(projected), 16), 64, 1024)
	if err == nil || !strings.Contains(err.Error(), "exceeds 64 bytes") {
		t.Fatalf("expected retained frame limit, got %v", err)
	}
}

func TestNormalizePiRPCEventBoundsToolResultsAndDropsNonAssistantMessages(t *testing.T) {
	toolLine, err := json.Marshal(map[string]any{
		"type":       "tool_execution_end",
		"toolCallId": "tool-1",
		"toolName":   "custom_tool",
		"result":     strings.Repeat("x", defaultToolOutputLimit+1000),
	})
	if err != nil {
		t.Fatal(err)
	}
	normalized, discarded, err := normalizePiRPCEvent("tool_execution_end", toolLine, 64)
	if err != nil || discarded {
		t.Fatalf("normalize tool event: discarded=%v err=%v", discarded, err)
	}
	var toolEvent piRPCToolExecutionEvent
	if err := json.Unmarshal(normalized, &toolEvent); err != nil {
		t.Fatalf("decode normalized tool event: %v", err)
	}
	if got := stringValue(toolEvent.Result); len(got) != defaultToolOutputLimit+3 || !strings.HasSuffix(got, "...") {
		t.Fatalf("unexpected normalized tool result: length=%d value=%q", len(got), got)
	}

	messageLine := []byte(`{"type":"message_end","message":{"role":"toolResult","content":[]}}`)
	_, discarded, err = normalizePiRPCEvent("message_end", messageLine, 64)
	if err != nil || !discarded {
		t.Fatalf("non-assistant message: discarded=%v err=%v", discarded, err)
	}
}

func TestPiRPCClientBackpressuresEventBursts(t *testing.T) {
	client, err := startPiRPCClient(fakePiRPCCommand(t, "burst"))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	requestDone := make(chan error, 1)
	go func() {
		requestDone <- client.Request(context.Background(), "get_state", nil, nil)
	}()
	// Let stdout fill the bounded event channel before starting the consumer.
	time.Sleep(50 * time.Millisecond)
	for index := 0; index < 512; index++ {
		select {
		case event, ok := <-client.Events():
			if !ok {
				t.Fatalf("event stream closed after %d events: %v", index, client.processError())
			}
			if event.Type != "agent_start" {
				t.Fatalf("unexpected event %d: %#v", index, event)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out after %d burst events", index)
		}
	}
	select {
	case err := <-requestDone:
		if err != nil {
			t.Fatalf("request failed after event burst: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("request remained blocked after draining event burst")
	}
}

func TestPiRPCClientCloseUnblocksBackpressuredReader(t *testing.T) {
	client, err := startPiRPCClient(fakePiRPCCommand(t, "burst"))
	if err != nil {
		t.Fatal(err)
	}
	requestDone := make(chan error, 1)
	go func() {
		requestDone <- client.Request(context.Background(), "get_state", nil, nil)
	}()
	time.Sleep(50 * time.Millisecond)

	started := time.Now()
	if err := client.Close(); err != nil {
		t.Fatalf("close backpressured client: %v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("closing backpressured client took %s", elapsed)
	}
	select {
	case err := <-requestDone:
		if err == nil {
			t.Fatal("request unexpectedly succeeded after closing backpressured client")
		}
	case <-time.After(time.Second):
		t.Fatal("request did not unblock after closing client")
	}
}

func TestPiRPCClientCorrelatesOutOfOrderResponsesAndEvents(t *testing.T) {
	client, err := startPiRPCClient(fakePiRPCCommand(t, "ordered"))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	type response struct {
		Value string `json:"value"`
	}
	results := make(map[string]string)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, command := range []string{"first", "second"} {
		command := command
		wg.Add(1)
		go func() {
			defer wg.Done()
			var decoded response
			if err := client.Request(context.Background(), command, nil, &decoded); err != nil {
				t.Errorf("request %s: %v", command, err)
				return
			}
			mu.Lock()
			results[command] = decoded.Value
			mu.Unlock()
		}()
	}
	wg.Wait()
	if results["first"] != "first" || results["second"] != "second" {
		t.Fatalf("unexpected correlated results: %#v", results)
	}
	select {
	case event := <-client.Events():
		if event.Type != "agent_start" {
			t.Fatalf("unexpected event: %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for interleaved event")
	}
}

func TestPiRPCClientBarrierFollowsEarlierRuntimeEvents(t *testing.T) {
	client, err := startPiRPCClient(fakePiRPCCommand(t, "barrier"))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := client.Send(ctx, map[string]any{"type": "extension_ui_response", "id": "previous-dialog", "value": "ok"}); err != nil {
		t.Fatal(err)
	}
	if err := client.Barrier(ctx, "run-1", 1); err != nil {
		t.Fatal(err)
	}
	first := <-client.Events()
	second := <-client.Events()
	if first.Type != "extension_ui_request" || second.Type != "codekanban_barrier" || second.BarrierRunID != "run-1" || second.BarrierGeneration != 1 {
		t.Fatalf("unexpected barrier ordering: first=%#v second=%#v", first, second)
	}
}

func TestPiRPCClientRejectsMalformedAndPartialFrames(t *testing.T) {
	for _, mode := range []string{"malformed", "partial", "unknown", "mismatch"} {
		t.Run(mode, func(t *testing.T) {
			client, err := startPiRPCClient(fakePiRPCCommand(t, mode))
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			err = client.Request(ctx, "get_state", nil, nil)
			if err == nil {
				t.Fatal("expected protocol error")
			}
		})
	}
}

func TestPiRPCClientTimeoutTerminatesProcess(t *testing.T) {
	client, err := startPiRPCClient(fakePiRPCCommand(t, "hang"))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := client.Request(ctx, "get_state", nil, nil); err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected request deadline error, got %v", err)
	}
	select {
	case <-client.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for hung Pi process to terminate")
	}
}

func TestPiRPCClientBoundsStderrAndRejectsEarlyExit(t *testing.T) {
	client, err := startPiRPCClient(fakePiRPCCommand(t, "stderr"))
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		OK bool `json:"ok"`
	}
	if err := client.Request(context.Background(), "get_state", nil, &response); err != nil {
		t.Fatal(err)
	}
	_ = client.Close()
	if !response.OK || len(client.Stderr()) != piRPCStderrLimit {
		t.Fatalf("response=%#v stderr length=%d", response, len(client.Stderr()))
	}

	exiting, err := startPiRPCClient(fakePiRPCCommand(t, "exit"))
	if err != nil {
		t.Fatal(err)
	}
	defer exiting.Close()
	if err := exiting.Request(context.Background(), "get_state", nil, nil); err == nil || !strings.Contains(err.Error(), "exited") {
		t.Fatalf("expected process exit error, got %v", err)
	}

	exitingWithStderr, err := startPiRPCClient(fakePiRPCCommand(t, "exit_stderr"))
	if err != nil {
		t.Fatal(err)
	}
	defer exitingWithStderr.Close()
	if err := exitingWithStderr.Request(context.Background(), "get_state", nil, nil); err == nil || !strings.Contains(err.Error(), "portable launcher failed") {
		t.Fatalf("expected stderr in process exit error, got %v", err)
	}
}
