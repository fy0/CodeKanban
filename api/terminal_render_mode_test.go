package api

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestInitializeTerminalConnectionRenderState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		url          string
		wantMode     terminalRenderMode
		wantInterval time.Duration
		wantZlib     bool
	}{
		{
			name:         "defaults to live",
			url:          "http://example.test/api/v1/terminal/ws?sessionId=s1",
			wantMode:     terminalRenderModeLive,
			wantInterval: defaultTerminalSnapshotInterval,
			wantZlib:     true,
		},
		{
			name:         "reads snapshot handshake",
			url:          "http://example.test/api/v1/terminal/ws?sessionId=s1&renderMode=snapshot&snapshotIntervalMs=500&snapshotCompression=none",
			wantMode:     terminalRenderModeSnapshot,
			wantInterval: 500 * time.Millisecond,
			wantZlib:     false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			state := newTerminalConnectionRenderState()
			initializeTerminalConnectionRenderState(state, httptest.NewRequest("GET", tt.url, nil))
			mode, interval, compressionEnabled := state.SnapshotConfig()
			if mode != tt.wantMode {
				t.Fatalf("mode = %q, want %q", mode, tt.wantMode)
			}
			if interval != tt.wantInterval {
				t.Fatalf("interval = %v, want %v", interval, tt.wantInterval)
			}
			if compressionEnabled != tt.wantZlib {
				t.Fatalf("compression enabled = %v, want %v", compressionEnabled, tt.wantZlib)
			}
		})
	}
}

func TestNormalizeSnapshotInterval(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input int
		want  time.Duration
	}{
		{name: "default when zero", input: 0, want: defaultTerminalSnapshotInterval},
		{name: "clamp low", input: 10, want: minTerminalSnapshotInterval},
		{name: "keep valid", input: 2000, want: 2 * time.Second},
		{name: "clamp high", input: 30000, want: maxTerminalSnapshotInterval},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := normalizeSnapshotInterval(tt.input); got != tt.want {
				t.Fatalf("normalizeSnapshotInterval(%d) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestTerminalConnectionRenderStateUpdate(t *testing.T) {
	t.Parallel()

	state := newTerminalConnectionRenderState()
	previous, current, interval, compressionEnabled, changed := state.Update("snapshot", 500, false)
	if previous != terminalRenderModeLive {
		t.Fatalf("previous mode = %q, want %q", previous, terminalRenderModeLive)
	}
	if current != terminalRenderModeSnapshot {
		t.Fatalf("current mode = %q, want %q", current, terminalRenderModeSnapshot)
	}
	if interval != 500*time.Millisecond {
		t.Fatalf("interval = %v, want %v", interval, 500*time.Millisecond)
	}
	if compressionEnabled {
		t.Fatal("expected compression to be disabled")
	}
	if !changed {
		t.Fatal("expected update to report changed")
	}

	select {
	case <-state.NotifyC():
	default:
		t.Fatal("expected update to notify listeners")
	}

	_, current, interval, compressionEnabled, changed = state.Update("snapshot", 500, false)
	if current != terminalRenderModeSnapshot {
		t.Fatalf("current mode after idempotent update = %q, want %q", current, terminalRenderModeSnapshot)
	}
	if interval != 500*time.Millisecond {
		t.Fatalf("interval after idempotent update = %v, want %v", interval, 500*time.Millisecond)
	}
	if compressionEnabled {
		t.Fatal("expected compression to stay disabled")
	}
	if changed {
		t.Fatal("expected idempotent update to report unchanged")
	}
}
