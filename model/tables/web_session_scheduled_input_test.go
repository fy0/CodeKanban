package tables

import (
	"testing"
	"time"
)

func TestWebSessionScheduledInputMigrationAllowsIdleSchedules(t *testing.T) {
	db := setupTestDB(t)
	if err := db.Exec(`CREATE TABLE "web_session_scheduled_inputs" ("id" text NOT NULL,"created_at" datetime,"updated_at" datetime,"deleted_at" datetime,"web_session_id" text NOT NULL,"action" text NOT NULL DEFAULT "message","target_id" text,"payload_json" text NOT NULL DEFAULT "{}","mode" text NOT NULL DEFAULT "send","text" text,"attachment_ids_json" text NOT NULL DEFAULT "[]","scheduled_for" datetime NOT NULL,"status" text NOT NULL DEFAULT "scheduled","last_error" text NOT NULL DEFAULT "","sent_at" datetime,"canceled_at" datetime,PRIMARY KEY ("id"))`).Error; err != nil {
		t.Fatalf("create legacy scheduled input table: %v", err)
	}
	if err := db.Exec(`
		INSERT INTO web_session_scheduled_inputs (
			id, web_session_id, action, payload_json, mode, attachment_ids_json,
			scheduled_for, status, last_error
		) VALUES (?, ?, 'execute_plan', '{}', 'send', '[]', datetime('now', '+1 hour'), 'scheduled', '')
	`, "legacy-scheduled", "session-1").Error; err != nil {
		t.Fatalf("insert legacy scheduled input: %v", err)
	}

	if err := db.AutoMigrate(&WebSessionScheduledInputTable{}); err != nil {
		t.Fatalf("auto migrate scheduled inputs: %v", err)
	}

	var legacy WebSessionScheduledInputTable
	if err := db.First(&legacy, "id = ?", "legacy-scheduled").Error; err != nil {
		t.Fatalf("load migrated scheduled input: %v", err)
	}
	if legacy.ScheduleKind != "at_time" || legacy.ScheduledFor.IsZero() {
		t.Fatalf("legacy schedule was not preserved as at-time: %#v", legacy)
	}

	idle := WebSessionScheduledInputTable{
		WebSessionID:      "session-1",
		Action:            "execute_plan",
		PayloadJSON:       "{}",
		Mode:              "send",
		AttachmentIDsJSON: "[]",
		ScheduleKind:      "when_idle",
		ScheduledFor:      time.Now(),
		BlockingReasons:   "[]",
		Status:            "scheduled",
	}
	idle.Init()
	if err := db.Create(&idle).Error; err != nil {
		t.Fatalf("create migrated when-idle schedule without scheduled_for: %v", err)
	}
}
