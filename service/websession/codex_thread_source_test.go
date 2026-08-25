package websession

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

type codexThreadTestRequest struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params map[string]any  `json:"params"`
}

func newCodexThreadTestClient(
	t *testing.T,
	handler func(codexThreadTestRequest) map[string]any,
) *codexAppServerClient {
	t.Helper()
	serverRequestReader, clientRequestWriter := io.Pipe()
	clientResponseReader, serverResponseWriter := io.Pipe()
	client := &codexAppServerClient{
		stdin:    clientRequestWriter,
		pending:  make(map[string]chan codexAppServerIncoming),
		incoming: make(chan codexAppServerIncoming, 64),
		closed:   make(chan struct{}),
	}
	go client.readLoop(clientResponseReader)
	go func() {
		decoder := json.NewDecoder(serverRequestReader)
		encoder := json.NewEncoder(serverResponseWriter)
		for {
			var request codexThreadTestRequest
			if err := decoder.Decode(&request); err != nil {
				_ = serverResponseWriter.Close()
				return
			}
			response := handler(request)
			if response == nil {
				response = map[string]any{"result": map[string]any{}}
			}
			response["id"] = request.ID
			if err := encoder.Encode(response); err != nil {
				_ = serverResponseWriter.Close()
				return
			}
		}
	}()
	t.Cleanup(func() {
		_ = client.closeStdin()
		_ = serverRequestReader.Close()
		_ = serverResponseWriter.Close()
		_ = clientResponseReader.Close()
	})
	return client
}

func TestCodexAppServerClientReadsLargeResumeResponseAndStaysUsable(t *testing.T) {
	const responseSize = 9 * 1024 * 1024
	client := newCodexThreadTestClient(t, func(request codexThreadTestRequest) map[string]any {
		switch request.Method {
		case "thread/resume":
			return map[string]any{"result": map[string]any{
				"thread": map[string]any{
					"id":      "thread_large",
					"history": strings.Repeat("x", responseSize),
				},
			}}
		case "config/read":
			return map[string]any{"result": map[string]any{"ok": true}}
		default:
			return map[string]any{"error": map[string]any{
				"code": -32601, "message": "unexpected method",
			}}
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resume, err := client.request(ctx, "thread/resume", map[string]any{"threadId": "thread_large"})
	if err != nil {
		t.Fatalf("large thread/resume response failed: %v", err)
	}
	if got := parseCodexThreadID(resume.Result); got != "thread_large" {
		t.Fatalf("unexpected resumed thread id %q", got)
	}

	followUp, err := client.request(ctx, "config/read", map[string]any{})
	if err != nil {
		t.Fatalf("request after large thread/resume response failed: %v", err)
	}
	if decodeRawObject(followUp.Result)["ok"] != true {
		t.Fatalf("unexpected follow-up response: %s", followUp.Result)
	}
}

func TestListCodexDescendantsUsesAncestorPaginationAcrossArchiveStates(t *testing.T) {
	type relationCall struct {
		archived bool
		cursor   string
	}
	var callsMu sync.Mutex
	calls := make([]relationCall, 0)
	client := newCodexThreadTestClient(t, func(request codexThreadTestRequest) map[string]any {
		if request.Method != "thread/list" {
			return map[string]any{"error": map[string]any{"code": -32601, "message": "unexpected method"}}
		}
		if stringValue(request.Params["ancestorThreadId"]) != "thread_root" {
			return map[string]any{"error": map[string]any{"code": -32602, "message": "missing ancestor filter"}}
		}
		archived, _ := request.Params["archived"].(bool)
		cursor := stringValue(request.Params["cursor"])
		callsMu.Lock()
		calls = append(calls, relationCall{archived: archived, cursor: cursor})
		callsMu.Unlock()
		switch {
		case !archived && cursor == "":
			return map[string]any{"result": map[string]any{
				"data": []any{
					map[string]any{
						"id": "thread_root", "parentThreadId": "thread_child_b", "agentPath": "/root",
					},
					map[string]any{
						"id": "thread_child_a", "parentThreadId": "thread_root", "agentNickname": "Atlas",
					},
				},
				"nextCursor": "page_2",
			}}
		case !archived && cursor == "page_2":
			return map[string]any{"result": map[string]any{
				"data": []any{map[string]any{
					"id": "thread_child_b", "parentThreadId": "thread_child_a", "agentRole": "reviewer",
				}},
				"nextCursor": "",
			}}
		case archived && cursor == "":
			return map[string]any{"result": map[string]any{
				"data": []any{map[string]any{
					"id": "thread_child_archived", "parentThreadId": "thread_root", "status": "idle",
				}},
				"nextCursor": "",
			}}
		default:
			return map[string]any{"result": map[string]any{"data": []any{}, "nextCursor": ""}}
		}
	})

	descendants, err := listCodexDescendantsWithClient(context.Background(), client, "thread_root")
	if err != nil {
		t.Fatalf("listCodexDescendantsWithClient returned error: %v", err)
	}
	if len(descendants) != 3 {
		t.Fatalf("expected three descendants across pages and archive states, got %#v", descendants)
	}
	if _, includesRoot := descendants["thread_root"]; includesRoot {
		t.Fatalf("native root must not be returned as its own descendant: %#v", descendants)
	}
	if descendants["thread_child_a"].Nickname != "Atlas" ||
		descendants["thread_child_b"].ParentThreadID != "thread_child_a" ||
		descendants["thread_child_archived"].Status != "idle" {
		t.Fatalf("unexpected descendant metadata: %#v", descendants)
	}
	callsMu.Lock()
	defer callsMu.Unlock()
	if len(calls) != 3 || calls[0] != (relationCall{archived: false, cursor: ""}) ||
		calls[1] != (relationCall{archived: false, cursor: "page_2"}) ||
		calls[2] != (relationCall{archived: true, cursor: ""}) {
		t.Fatalf("unexpected pagination calls: %#v", calls)
	}
}

func TestMergeCodexDescendantSummaryRepairsSelfParent(t *testing.T) {
	merged := mergeCodexDescendantSummary(
		codexThreadSummary{ID: "thread_child", ParentThreadID: "thread_child"},
		codexThreadSummary{
			ID:             "thread_child",
			ParentThreadID: "thread_root",
			AgentPath:      "/root/review",
			Nickname:       "Atlas",
			Role:           "reviewer",
		},
	)
	if merged.ParentThreadID != "thread_root" || merged.AgentPath != "/root/review" ||
		merged.Nickname != "Atlas" || merged.Role != "reviewer" {
		t.Fatalf("unexpected merged descendant metadata: %#v", merged)
	}
}

func TestListCodexDescendantsFallsBackToRecursiveParentQueries(t *testing.T) {
	var callsMu sync.Mutex
	parentCalls := make([]string, 0)
	client := newCodexThreadTestClient(t, func(request codexThreadTestRequest) map[string]any {
		if request.Method != "thread/list" {
			return map[string]any{"error": map[string]any{"code": -32601, "message": "unexpected method"}}
		}
		if _, ok := request.Params["ancestorThreadId"]; ok {
			return map[string]any{"error": map[string]any{
				"code": -32602, "message": "unknown field ancestorThreadId",
			}}
		}
		parentID := stringValue(request.Params["parentThreadId"])
		archived, _ := request.Params["archived"].(bool)
		callsMu.Lock()
		parentCalls = append(parentCalls, parentID)
		callsMu.Unlock()
		data := []any{}
		if !archived {
			switch parentID {
			case "thread_root":
				data = append(data, map[string]any{
					"id": "thread_child", "parentThreadId": "thread_root", "agentNickname": "Atlas",
				})
			case "thread_child":
				data = append(data, map[string]any{
					"id": "thread_grandchild", "parentThreadId": "thread_child", "agentRole": "reviewer",
				})
			}
		}
		return map[string]any{"result": map[string]any{"data": data, "nextCursor": ""}}
	})

	descendants, err := listCodexDescendantsWithClient(context.Background(), client, "thread_root")
	if err != nil {
		t.Fatalf("listCodexDescendantsWithClient returned error: %v", err)
	}
	if len(descendants) != 2 || descendants["thread_child"].Nickname != "Atlas" ||
		descendants["thread_grandchild"].ParentThreadID != "thread_child" {
		t.Fatalf("expected nested descendants from parent fallback, got %#v", descendants)
	}
	callsMu.Lock()
	defer callsMu.Unlock()
	seen := map[string]int{}
	for _, parentID := range parentCalls {
		seen[parentID]++
	}
	if seen["thread_root"] != 2 || seen["thread_child"] != 2 || seen["thread_grandchild"] != 2 {
		t.Fatalf("expected archived and active queries for every discovered parent, got %#v", parentCalls)
	}
}
