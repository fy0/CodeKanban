package websession

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	piRPCMaxFrameBytes           = 64 * 1024 * 1024
	piRPCMaxDiscardedFrameBytes  = 256 * 1024 * 1024
	piRPCMaxQueuedEventBytes     = 8 * 1024 * 1024
	piRPCMaxInlineToolEventBytes = 64 * 1024
	piRPCStderrLimit             = 64 * 1024
	piRPCRequestTimeout          = 30 * time.Second
)

var errPiRPCClosed = errors.New("Pi RPC process is closed")

type piRPCRequestResult struct {
	response piRPCResponse
	err      error
}

type piRPCPendingRequest struct {
	command string
	result  chan piRPCRequestResult
}

type piRPCWrite struct {
	data   []byte
	result chan error
}

type piRPCClient struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser

	mu         sync.Mutex
	pending    map[string]piRPCPendingRequest
	exitErr    error
	events     chan piRPCEvent
	writes     chan piRPCWrite
	closing    chan struct{}
	done       chan struct{}
	readerDone chan struct{}
	stderrDone chan struct{}
	seq        atomic.Uint64

	stderrBuffer *piRPCBoundedBuffer
	closeOnce    sync.Once
}

func startPiRPCClient(cmd *exec.Cmd) (*piRPCClient, error) {
	if cmd == nil {
		return nil, errors.New("Pi RPC command is nil")
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("open Pi RPC stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("open Pi RPC stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, fmt.Errorf("open Pi RPC stderr: %w", err)
	}
	client := &piRPCClient{
		cmd:          cmd,
		stdin:        stdin,
		stdout:       stdout,
		stderr:       stderr,
		pending:      make(map[string]piRPCPendingRequest),
		events:       make(chan piRPCEvent, 256),
		writes:       make(chan piRPCWrite, 64),
		closing:      make(chan struct{}),
		done:         make(chan struct{}),
		readerDone:   make(chan struct{}),
		stderrDone:   make(chan struct{}),
		stderrBuffer: newPiRPCBoundedBuffer(piRPCStderrLimit),
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderr.Close()
		return nil, fmt.Errorf("start Pi RPC process: %w", err)
	}
	go client.writeStdin()
	go client.readStdout()
	go func() {
		defer close(client.stderrDone)
		_, _ = io.Copy(client.stderrBuffer, stderr)
	}()
	go client.wait()
	return client, nil
}

func (c *piRPCClient) Events() <-chan piRPCEvent {
	if c == nil {
		return nil
	}
	return c.events
}

func (c *piRPCClient) Done() <-chan struct{} {
	if c == nil {
		closed := make(chan struct{})
		close(closed)
		return closed
	}
	return c.done
}

func (c *piRPCClient) Stderr() string {
	if c == nil || c.stderrBuffer == nil {
		return ""
	}
	return c.stderrBuffer.String()
}

func (c *piRPCClient) Send(ctx context.Context, payload map[string]any) error {
	if c == nil {
		return errPiRPCClosed
	}
	if len(payload) == 0 || strings.TrimSpace(stringValue(payload["type"])) == "" {
		return errors.New("Pi RPC frame type is required")
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode Pi RPC frame: %w", err)
	}
	encoded = append(encoded, '\n')
	return c.enqueueWrite(ctx, encoded, "frame")
}

func (c *piRPCClient) Barrier(ctx context.Context, runID string, generation uint64) error {
	if generation == 0 {
		return errors.New("Pi RPC response barrier generation is required")
	}
	return c.barrier(ctx, runID, generation, false, true)
}

func (c *piRPCClient) BarrierAndWait(ctx context.Context, runID string) error {
	return c.barrier(ctx, runID, 0, true, false)
}

func (c *piRPCClient) barrier(ctx context.Context, runID string, generation uint64, wait, wakePending bool) error {
	if c == nil {
		return errPiRPCClosed
	}
	if strings.TrimSpace(runID) == "" {
		return errors.New("Pi RPC barrier run id is required")
	}
	if err := c.Request(ctx, "get_state", nil, nil); err != nil {
		return err
	}
	var consumed chan struct{}
	if wait {
		consumed = make(chan struct{})
	}
	marker := piRPCEvent{
		Type: "codekanban_barrier", BarrierRunID: runID, BarrierGeneration: generation,
		BarrierDone: consumed, WakePending: wakePending,
	}
	select {
	case c.events <- marker:
	case <-ctx.Done():
		err := fmt.Errorf("queue Pi RPC barrier: %w", ctx.Err())
		c.fail(err)
		killCmdTree(c.cmd)
		return err
	case <-c.done:
		return c.processError()
	}
	if consumed == nil {
		return nil
	}
	select {
	case <-consumed:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("wait for Pi RPC barrier: %w", ctx.Err())
	case <-c.done:
		return c.processError()
	}
}

func (c *piRPCClient) Request(ctx context.Context, command string, payload map[string]any, target any) error {
	if c == nil {
		return errPiRPCClosed
	}
	command = strings.TrimSpace(command)
	if command == "" {
		return errors.New("Pi RPC command type is required")
	}
	id := fmt.Sprintf("ck_%d", c.seq.Add(1))
	request := make(map[string]any, len(payload)+2)
	for key, value := range payload {
		if key != "id" && key != "type" {
			request[key] = value
		}
	}
	request["id"] = id
	request["type"] = command
	encoded, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("encode Pi RPC %s request: %w", command, err)
	}
	encoded = append(encoded, '\n')

	resultCh := make(chan piRPCRequestResult, 1)
	c.mu.Lock()
	if c.exitErr != nil {
		err := c.exitErr
		c.mu.Unlock()
		return err
	}
	c.pending[id] = piRPCPendingRequest{command: command, result: resultCh}
	c.mu.Unlock()

	requestCtx := ctx
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		requestCtx, cancel = context.WithTimeout(ctx, piRPCRequestTimeout)
		defer cancel()
	}
	if err := c.enqueueWrite(requestCtx, encoded, command+" request"); err != nil {
		c.removePending(id)
		return err
	}

	select {
	case result := <-resultCh:
		if result.err != nil {
			return result.err
		}
		if !result.response.Success {
			message := strings.TrimSpace(result.response.Error)
			if message == "" {
				message = "request failed"
			}
			return fmt.Errorf("Pi RPC %s failed: %s", command, message)
		}
		if target == nil || len(result.response.Data) == 0 || bytes.Equal(result.response.Data, []byte("null")) {
			return nil
		}
		if err := json.Unmarshal(result.response.Data, target); err != nil {
			return fmt.Errorf("decode Pi RPC %s response: %w", command, err)
		}
		return nil
	case <-requestCtx.Done():
		err := fmt.Errorf("Pi RPC %s request: %w", command, requestCtx.Err())
		c.removePending(id)
		c.fail(err)
		killCmdTree(c.cmd)
		return err
	case <-c.done:
		c.removePending(id)
		return c.processError()
	}
}

func (c *piRPCClient) enqueueWrite(ctx context.Context, encoded []byte, description string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	writeCtx := ctx
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		writeCtx, cancel = context.WithTimeout(ctx, piRPCRequestTimeout)
		defer cancel()
	}
	writeResult := make(chan error, 1)
	select {
	case c.writes <- piRPCWrite{data: encoded, result: writeResult}:
	case <-writeCtx.Done():
		err := fmt.Errorf("queue Pi RPC %s: %w", description, writeCtx.Err())
		c.fail(err)
		killCmdTree(c.cmd)
		return err
	case <-c.closing:
		return c.processError()
	case <-c.done:
		return c.processError()
	}
	select {
	case writeErr := <-writeResult:
		if writeErr != nil {
			return fmt.Errorf("write Pi RPC %s: %w", description, writeErr)
		}
		return nil
	case <-writeCtx.Done():
		err := fmt.Errorf("write Pi RPC %s: %w", description, writeCtx.Err())
		c.fail(err)
		killCmdTree(c.cmd)
		return err
	case <-c.done:
		return c.processError()
	}
}

func (c *piRPCClient) Close() error {
	if c == nil {
		return nil
	}
	c.closeOnce.Do(func() {
		close(c.closing)
		_ = c.stdin.Close()
		select {
		case <-c.done:
			return
		case <-time.After(500 * time.Millisecond):
		}
		killCmdTree(c.cmd)
	})
	select {
	case <-c.done:
		return nil
	case <-time.After(2 * time.Second):
		return errors.New("timed out stopping Pi RPC process")
	}
}

func (c *piRPCClient) writeStdin() {
	for {
		select {
		case write := <-c.writes:
			_, err := c.stdin.Write(write.data)
			write.result <- err
			if err != nil {
				c.fail(fmt.Errorf("write Pi RPC stdin: %w", err))
				killCmdTree(c.cmd)
				return
			}
		case <-c.closing:
			return
		case <-c.done:
			return
		}
	}
}

func (c *piRPCClient) readStdout() {
	defer close(c.readerDone)
	reader := bufio.NewReaderSize(c.stdout, 64*1024)
	for {
		line, discarded, err := readPiRPCJSONLFrameForClient(
			reader,
			piRPCMaxFrameBytes,
			piRPCMaxDiscardedFrameBytes,
		)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return
			}
			c.fail(fmt.Errorf("read Pi RPC stdout: %w", err))
			killCmdTree(c.cmd)
			return
		}
		if discarded {
			continue
		}
		if err := c.handleLine(line); err != nil {
			c.fail(err)
			killCmdTree(c.cmd)
			return
		}
	}
}

func readPiRPCJSONLFrame(reader *bufio.Reader, limit int) ([]byte, error) {
	frame, _, err := readPiRPCJSONLFrameFiltered(reader, limit, limit, nil)
	return frame, err
}

func readPiRPCJSONLFrameForClient(reader *bufio.Reader, retainLimit, discardLimit int) ([]byte, bool, error) {
	return readPiRPCJSONLFrameFiltered(reader, retainLimit, discardLimit, shouldDiscardPiRPCEvent)
}

func readPiRPCJSONLFrameFiltered(
	reader *bufio.Reader,
	retainLimit int,
	discardLimit int,
	discardType func(string) bool,
) ([]byte, bool, error) {
	if retainLimit <= 0 {
		retainLimit = piRPCMaxFrameBytes
	}
	if discardLimit < retainLimit {
		discardLimit = retainLimit
	}
	frame := make([]byte, 0, 1024)
	totalBytes := 0
	discarding := false
	for {
		part, err := reader.ReadSlice('\n')
		totalBytes += len(part)
		if discarding {
			if totalBytes > discardLimit {
				return nil, false, fmt.Errorf("discarded frame exceeds %d bytes", discardLimit)
			}
		} else {
			frame = append(frame, part...)
			if eventType, ok := piRPCJSONLFrameType(frame); ok && discardType != nil && discardType(eventType) {
				discarding = true
				frame = nil
				if totalBytes > discardLimit {
					return nil, false, fmt.Errorf("discarded %s frame exceeds %d bytes", eventType, discardLimit)
				}
			} else if len(frame) > retainLimit {
				return nil, false, fmt.Errorf("frame exceeds %d bytes", retainLimit)
			}
		}
		switch {
		case err == nil:
			if discarding {
				return nil, true, nil
			}
			frame = frame[:len(frame)-1]
			if len(frame) > 0 && frame[len(frame)-1] == '\r' {
				frame = frame[:len(frame)-1]
			}
			return frame, false, nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case errors.Is(err, io.EOF):
			if totalBytes == 0 {
				return nil, false, io.EOF
			}
			return nil, false, errors.New("EOF after partial JSONL frame")
		default:
			return nil, false, err
		}
	}
}

// Pi serializes type as one of the first two top-level fields (events use it
// first; responses put id first). Decode only that bounded prefix so redundant
// events can be drained without retaining their potentially large payloads.
func piRPCJSONLFrameType(frame []byte) (string, bool) {
	decoder := json.NewDecoder(bytes.NewReader(frame))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return "", false
	}
	for field := 0; field < 2 && decoder.More(); field++ {
		keyToken, err := decoder.Token()
		if err != nil {
			return "", false
		}
		key, ok := keyToken.(string)
		if !ok {
			return "", false
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return "", false
		}
		if key != "type" {
			continue
		}
		var eventType string
		if json.Unmarshal(value, &eventType) != nil || strings.TrimSpace(eventType) == "" {
			return "", false
		}
		return eventType, true
	}
	return "", false
}

func shouldDiscardPiRPCEvent(eventType string) bool {
	switch strings.TrimSpace(eventType) {
	case "response",
		"agent_start", "agent_settled",
		"message_start", "message_update", "message_end",
		"tool_execution_start", "tool_execution_update", "tool_execution_end",
		"compaction_start", "compaction_end", "queue_update",
		"auto_retry_start", "auto_retry_end",
		"summarization_retry_scheduled", "summarization_retry_attempt_start", "summarization_retry_finished",
		"extension_ui_request", "extension_error":
		return false
	default:
		return true
	}
}

func (c *piRPCClient) handleLine(line []byte) error {
	var envelope struct {
		Type string `json:"type"`
	}
	if len(bytes.TrimSpace(line)) == 0 {
		return errors.New("Pi RPC emitted an empty JSONL frame")
	}
	if err := json.Unmarshal(line, &envelope); err != nil {
		return fmt.Errorf("Pi RPC emitted malformed JSON: %w", err)
	}
	if strings.TrimSpace(envelope.Type) == "" {
		return errors.New("Pi RPC frame has no type")
	}
	if envelope.Type != "response" {
		if shouldDiscardPiRPCEvent(envelope.Type) {
			return nil
		}
		normalized, discarded, err := normalizePiRPCEvent(envelope.Type, line, piRPCMaxInlineToolEventBytes)
		if err != nil {
			return err
		}
		if discarded {
			return nil
		}
		if len(normalized) > piRPCMaxQueuedEventBytes {
			return fmt.Errorf("Pi RPC %s event exceeds %d queued bytes", envelope.Type, piRPCMaxQueuedEventBytes)
		}
		event := piRPCEvent{Type: envelope.Type, Raw: append(json.RawMessage(nil), normalized...)}
		select {
		case c.events <- event:
			return nil
		case <-c.closing:
			return c.processError()
		}
	}

	var response piRPCResponse
	if err := json.Unmarshal(line, &response); err != nil {
		return fmt.Errorf("decode Pi RPC response: %w", err)
	}
	if strings.TrimSpace(response.ID) == "" || strings.TrimSpace(response.Command) == "" {
		return errors.New("Pi RPC response is missing id or command")
	}
	c.mu.Lock()
	pending, ok := c.pending[response.ID]
	c.mu.Unlock()
	if !ok {
		return fmt.Errorf("Pi RPC response has unknown id %q", response.ID)
	}
	if pending.command != response.Command {
		return fmt.Errorf(
			"Pi RPC response command mismatch: got %s, want %s",
			response.Command,
			pending.command,
		)
	}
	c.removePending(response.ID)
	pending.result <- piRPCRequestResult{response: response}
	return nil
}

func (c *piRPCClient) wait() {
	err := c.cmd.Wait()
	<-c.readerDone
	<-c.stderrDone
	if err != nil {
		processErr := fmt.Errorf("Pi RPC process exited: %w", err)
		if stderr := strings.TrimSpace(c.Stderr()); stderr != "" {
			processErr = fmt.Errorf("%w: %s", processErr, stderr)
		}
		c.fail(processErr)
	} else {
		c.fail(errPiRPCClosed)
	}
	close(c.events)
	close(c.done)
}

func (c *piRPCClient) fail(err error) {
	if err == nil {
		err = errPiRPCClosed
	}
	c.mu.Lock()
	if c.exitErr == nil {
		c.exitErr = err
	}
	resolved := c.exitErr
	pending := c.pending
	c.pending = make(map[string]piRPCPendingRequest)
	c.mu.Unlock()
	for _, request := range pending {
		request.result <- piRPCRequestResult{err: resolved}
	}
}

func (c *piRPCClient) removePending(id string) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

func normalizePiRPCEvent(eventType string, line []byte, maxInlineToolBytes int) ([]byte, bool, error) {
	if maxInlineToolBytes <= 0 {
		maxInlineToolBytes = piRPCMaxInlineToolEventBytes
	}
	switch eventType {
	case "message_start", "message_end":
		var event piRPCMessageEvent
		if err := json.Unmarshal(line, &event); err != nil {
			return nil, false, fmt.Errorf("decode Pi RPC %s event: %w", eventType, err)
		}
		if !strings.EqualFold(strings.TrimSpace(event.Message.Role), "assistant") {
			return nil, true, nil
		}
	case "tool_execution_update", "tool_execution_end":
		if len(line) <= maxInlineToolBytes {
			return line, false, nil
		}
		var event piRPCToolExecutionEvent
		if err := json.Unmarshal(line, &event); err != nil {
			return nil, false, fmt.Errorf("decode oversized Pi RPC %s event: %w", eventType, err)
		}
		if event.PartialResult != nil {
			event.PartialResult = truncateToolOutput(event.ToolName, piToolResultText(event.PartialResult))
		}
		if event.Result != nil {
			event.Result = truncateToolOutput(event.ToolName, piToolResultText(event.Result))
		}
		normalized, err := json.Marshal(event)
		if err != nil {
			return nil, false, fmt.Errorf("normalize oversized Pi RPC %s event: %w", eventType, err)
		}
		return normalized, false, nil
	}
	return line, false, nil
}

func (c *piRPCClient) processError() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.exitErr != nil {
		return c.exitErr
	}
	return errPiRPCClosed
}

type piRPCBoundedBuffer struct {
	mu    sync.Mutex
	limit int
	data  []byte
}

func newPiRPCBoundedBuffer(limit int) *piRPCBoundedBuffer {
	return &piRPCBoundedBuffer{limit: limit}
}

func (b *piRPCBoundedBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	originalLength := len(data)
	if b.limit <= 0 {
		return originalLength, nil
	}
	if len(data) >= b.limit {
		b.data = append(b.data[:0], data[len(data)-b.limit:]...)
		return originalLength, nil
	}
	if overflow := len(b.data) + len(data) - b.limit; overflow > 0 {
		copy(b.data, b.data[overflow:])
		b.data = b.data[:len(b.data)-overflow]
	}
	b.data = append(b.data, data...)
	return originalLength, nil
}

func (b *piRPCBoundedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(append([]byte(nil), b.data...))
}
