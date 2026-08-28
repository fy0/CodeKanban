package terminal

import (
	"testing"
	"time"
)

func TestMetadataIntervalForActivity(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name            string
		lastInteraction time.Time
		lastOutput      time.Time
		metadata        *SessionMetadata
		want            time.Duration
	}{
		{
			name:            "recent interaction stays fast",
			lastInteraction: now.Add(-time.Second),
			metadata:        &SessionMetadata{ProcessStatus: "idle"},
			want:            MetadataIntervalShort,
		},
		{
			name:     "busy process stays warm",
			metadata: &SessionMetadata{ProcessStatus: "busy", ProcessHasChildren: true},
			want:     MetadataIntervalMedium,
		},
		{
			name:       "recent output stays warm",
			lastOutput: now.Add(-time.Second),
			metadata:   &SessionMetadata{ProcessStatus: "idle"},
			want:       MetadataIntervalMedium,
		},
		{
			name:     "inactive idle process becomes slow",
			metadata: &SessionMetadata{ProcessStatus: "idle"},
			want:     MetadataIntervalLong,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			session := &Session{}
			if !test.lastInteraction.IsZero() {
				session.metaLastInteractionAt.Store(test.lastInteraction.UnixNano())
			}
			if !test.lastOutput.IsZero() {
				session.metaLastOutputAt.Store(test.lastOutput.UnixNano())
			}
			if got := session.metadataIntervalFor(test.metadata, now); got != test.want {
				t.Fatalf("metadataIntervalFor() = %s, want %s", got, test.want)
			}
		})
	}
}

func TestRecordMetadataInputAcceleratesSampling(t *testing.T) {
	session := &Session{
		metaInterval:         MetadataIntervalLong,
		metaIntervalNotifyCh: make(chan struct{}, 1),
	}

	session.recordMetadataInput()

	if got := session.currentMetadataInterval(); got != MetadataIntervalShort {
		t.Fatalf("currentMetadataInterval() = %s, want %s", got, MetadataIntervalShort)
	}
	if session.metaLastInteractionAt.Load() <= 0 {
		t.Fatal("expected recordMetadataInput to record user input")
	}
	select {
	case <-session.metaIntervalNotifyCh:
	default:
		t.Fatal("expected recordMetadataInput to wake the metadata monitor")
	}
}

func TestSessionSnapshotUsesCachedMetadata(t *testing.T) {
	session, err := NewSession(SessionParams{
		ID:      "session-metadata-cache",
		Title:   "cached metadata",
		Command: []string{"test-shell"},
	})
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	capturedAt := time.Now().Add(-time.Second).UTC()
	session.lastMetadata = &SessionMetadata{
		ProcessPID:         4242,
		ProcessStatus:      "busy",
		ProcessHasChildren: true,
		RunningCommand:     "codex exec",
		CapturedAt:         capturedAt,
	}

	snapshot := session.Snapshot()

	if snapshot.ProcessPID != 4242 || snapshot.ProcessStatus != "busy" {
		t.Fatalf("unexpected cached process state: %+v", snapshot)
	}
	if !snapshot.ProcessHasChildren || snapshot.RunningCommand != "codex exec" {
		t.Fatalf("unexpected cached process details: %+v", snapshot)
	}
	if !snapshot.MetadataCapturedAt.Equal(capturedAt) {
		t.Fatalf("MetadataCapturedAt = %s, want %s", snapshot.MetadataCapturedAt, capturedAt)
	}
}

func TestMetadataChangedIgnoresCaptureTimestamp(t *testing.T) {
	session := &Session{}
	oldMetadata := &SessionMetadata{
		ProcessPID:     42,
		ProcessStatus:  "idle",
		RunningCommand: "",
		CapturedAt:     time.Now().Add(-time.Minute),
	}
	newMetadata := cloneSessionMetadata(oldMetadata)
	newMetadata.CapturedAt = time.Now()

	if session.metadataChanged(oldMetadata, newMetadata) {
		t.Fatal("capture timestamp alone must not broadcast a metadata change")
	}
}
