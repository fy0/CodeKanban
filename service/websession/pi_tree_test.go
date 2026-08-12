package websession

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestProjectPiTreeFiltersBridgeMarkersAndPreservesActivePath(t *testing.T) {
	rootID := "user-root"
	markerID := "marker-1"
	markerData, err := json.Marshal(piBridgeMarkerData{TargetID: rootID, Nonce: "nonce-1"})
	if err != nil {
		t.Fatal(err)
	}
	roots := []piHistoryTreeNode{
		{
			Entry: piHistoryEntry{
				Type: "message", ID: rootID, Timestamp: "2026-05-01T01:00:00Z",
				Message: piHistoryMessage{Role: "user", Content: json.RawMessage(`"root prompt"`)},
			},
			Children: []piHistoryTreeNode{
				{
					Entry: piHistoryEntry{Type: "custom", ID: markerID, ParentID: &rootID, CustomType: piBridgeMarkerType, Data: markerData},
					Children: []piHistoryTreeNode{{
						Entry: piHistoryEntry{
							Type: "message", ID: "assistant-new", ParentID: &markerID,
							Message: piHistoryMessage{Role: "assistant", Content: json.RawMessage(`[{"type":"text","text":"new branch"}]`)},
						},
					}},
				},
				{
					Entry: piHistoryEntry{
						Type: "message", ID: "assistant-old", ParentID: &rootID,
						Message: piHistoryMessage{Role: "assistant", Content: json.RawMessage(`"old branch"`)},
					},
				},
			},
		},
	}

	snapshot, raw, err := projectPiTree("native-1", "revision-1", roots, "assistant-new")
	if err != nil {
		t.Fatalf("projectPiTree: %v", err)
	}
	if snapshot.SessionID != "native-1" || snapshot.Revision != "revision-1" || pointerString(snapshot.LeafID) != "assistant-new" {
		t.Fatalf("unexpected snapshot identity: %#v", snapshot)
	}
	if len(raw) != 4 || len(snapshot.Nodes) != 3 {
		t.Fatalf("marker was not retained internally and filtered externally: raw=%d nodes=%#v", len(raw), snapshot.Nodes)
	}
	byID := make(map[string]PiTreeNode, len(snapshot.Nodes))
	for _, node := range snapshot.Nodes {
		byID[node.ID] = node
		if node.ID == markerID {
			t.Fatalf("bridge marker leaked into projected tree: %#v", node)
		}
	}
	if pointerString(byID["assistant-new"].ParentID) != rootID {
		t.Fatalf("marker child was not reparented to visible ancestor: %#v", byID["assistant-new"])
	}
	if !byID[rootID].Active || !byID["assistant-new"].Active || byID["assistant-old"].Active {
		t.Fatalf("unexpected active path: %#v", snapshot.Nodes)
	}
	if byID[rootID].Preview != "root prompt" || byID["assistant-new"].Preview != "new branch" {
		t.Fatalf("unexpected previews: %#v", snapshot.Nodes)
	}
}

func TestProjectPiTreeUsesMarkerParentAsLogicalLeaf(t *testing.T) {
	rootID := "root-user"
	markerData, _ := json.Marshal(piBridgeMarkerData{TargetID: rootID, Nonce: "nonce-root"})
	roots := []piHistoryTreeNode{
		{Entry: piHistoryEntry{Type: "message", ID: rootID, Message: piHistoryMessage{Role: "user", Content: json.RawMessage(`"original"`)}}},
		{Entry: piHistoryEntry{Type: "custom", ID: "marker-root", CustomType: piBridgeMarkerType, Data: markerData}},
	}
	snapshot, _, err := projectPiTree("native-root", "revision-root", roots, "marker-root")
	if err != nil {
		t.Fatalf("projectPiTree: %v", err)
	}
	if snapshot.LeafID != nil {
		t.Fatalf("root navigation marker should expose a nil logical leaf: %#v", snapshot)
	}
	if len(snapshot.Nodes) != 1 || snapshot.Nodes[0].Active {
		t.Fatalf("root navigation should leave no visible node active: %#v", snapshot.Nodes)
	}
}

func TestProjectPiTreeRejectsMalformedForests(t *testing.T) {
	root := piHistoryTreeNode{Entry: piHistoryEntry{Type: "message", ID: "root"}}
	orphanParent := "missing"
	duplicateParent := "root"
	for name, testCase := range map[string]struct {
		roots []piHistoryTreeNode
		leaf  string
	}{
		"orphan root": {
			roots: []piHistoryTreeNode{{Entry: piHistoryEntry{Type: "message", ID: "orphan", ParentID: &orphanParent}}},
			leaf:  "orphan",
		},
		"self parent root": {
			roots: []piHistoryTreeNode{{Entry: piHistoryEntry{Type: "message", ID: "self", ParentID: stringPointer("self")}}},
			leaf:  "self",
		},
		"duplicate id": {
			roots: []piHistoryTreeNode{{
				Entry:    piHistoryEntry{Type: "message", ID: "root"},
				Children: []piHistoryTreeNode{{Entry: piHistoryEntry{Type: "message", ID: "root", ParentID: &duplicateParent}}},
			}},
			leaf: "root",
		},
		"nested parent mismatch": {
			roots: []piHistoryTreeNode{{
				Entry:    piHistoryEntry{Type: "message", ID: "root"},
				Children: []piHistoryTreeNode{{Entry: piHistoryEntry{Type: "message", ID: "child", ParentID: &orphanParent}}},
			}},
			leaf: "child",
		},
		"missing leaf": {roots: []piHistoryTreeNode{root}, leaf: "missing"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := projectPiTree("native", "revision", testCase.roots, testCase.leaf); err == nil {
				t.Fatal("expected malformed tree rejection")
			}
		})
	}
}

func TestValidatePiNavigationMarker(t *testing.T) {
	parentID := "parent"
	target := piHistoryEntry{Type: "message", ID: "target-user", ParentID: &parentID, Message: piHistoryMessage{Role: "user"}}
	data, _ := json.Marshal(piBridgeMarkerData{TargetID: target.ID, Nonce: "nonce"})
	marker := piHistoryEntry{Type: "custom", ID: "marker", ParentID: &parentID, CustomType: piBridgeMarkerType, Data: data}
	if err := validatePiNavigationMarker(target, marker, []piHistoryEntry{target, marker}, false); err != nil {
		t.Fatalf("valid navigation marker: %v", err)
	}
	wrongParent := "wrong"
	marker.ParentID = &wrongParent
	if err := validatePiNavigationMarker(target, marker, []piHistoryEntry{target, marker}, false); err == nil || !strings.Contains(err.Error(), "parent") {
		t.Fatalf("expected parent mismatch, got %v", err)
	}
}

func TestPiTreeEditorContentTextPreservesOriginalWhitespace(t *testing.T) {
	for name, testCase := range map[string]struct {
		raw  json.RawMessage
		want string
	}{
		"string": {
			raw:  json.RawMessage(`"  first line\nsecond line  "`),
			want: "  first line\nsecond line  ",
		},
		"text blocks": {
			raw:  json.RawMessage(`[{"type":"text","text":"  first "},{"type":"image","data":"ignored"},{"type":"text","text":"second  "}]`),
			want: "  first second  ",
		},
	} {
		t.Run(name, func(t *testing.T) {
			if got := piTreeEditorContentText(testCase.raw); got != testCase.want {
				t.Fatalf("editor text changed whitespace: got=%q want=%q", got, testCase.want)
			}
		})
	}
}

func TestClassifyPiTreeErrorDoesNotExposeInternalDetails(t *testing.T) {
	for name, testCase := range map[string]struct {
		err         error
		code        string
		messagePart string
	}{
		"revision": {ErrPiTreeRevisionConflict, "conflict", "refresh"},
		"active":   {errors.New("cannot navigate an active Pi web session"), "invalid_state", "current session state"},
		"input":    {errors.New("Pi tree revision is required"), "bad_req", "Invalid Pi session tree request"},
		"internal": {errors.New("native-secret-response"), "internal", "operation failed"},
	} {
		t.Run(name, func(t *testing.T) {
			classified := ClassifyPiTreeError(testCase.err)
			if classified.Code != testCase.code || !strings.Contains(classified.Message, testCase.messagePart) {
				t.Fatalf("unexpected classification: %#v", classified)
			}
			frame := newPiTreeErrorFrame(wireCommandFrame{RequestID: "request", SessionID: "session"}, testCase.err)
			if frame.Code != testCase.code || frame.Message != classified.Message {
				t.Fatalf("wire classification drifted: %#v", frame)
			}
			if strings.Contains(frame.Message, "native-secret-response") {
				t.Fatalf("internal error leaked through wire frame: %#v", frame)
			}
		})
	}
}

func TestNavigatePiSessionTreeRequiresManager(t *testing.T) {
	var manager *Manager
	if _, err := manager.NavigatePiSessionTree(context.Background(), "session", PiTreeNavigateInput{}); err == nil {
		t.Fatal("expected nil manager rejection")
	}
}

func stringPointer(value string) *string {
	return &value
}
