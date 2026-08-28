package model

import (
	"path/filepath"
	"reflect"
	"testing"

	"code-kanban/model/tables"
)

func TestBackfillWebSessionItemCommandGroupIDs(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "command-group-backfill.db")
	if err := InitWithDSN(dsn, 0, true); err != nil {
		t.Fatalf("InitWithDSN: %v", err)
	}
	t.Cleanup(DBClose)

	rows := []tables.WebSessionItemTable{
		{WebSessionID: "session-1", ItemKind: "tool", ItemType: "command_execution", ToolJSON: `{"commandGroup":{"id":"  cmdgrp_one  "}}`},
		{WebSessionID: "session-1", ItemKind: "tool", ItemType: "command_execution", ToolJSON: `{"id":"plain-tool"}`},
		{WebSessionID: "session-1", ItemKind: "tool", ItemType: "command_execution", ToolJSON: ""},
		{WebSessionID: "session-1", ItemKind: "tool", ItemType: "command_execution", ToolJSON: "{invalid"},
	}
	for index := range rows {
		rows[index].ID = "item-" + string(rune('a'+index))
		if err := db.Create(&rows[index]).Error; err != nil {
			t.Fatalf("create legacy row %d: %v", index, err)
		}
	}

	backfilled, err := backfillWebSessionItemCommandGroupIDs()
	if err != nil {
		t.Fatalf("backfillWebSessionItemCommandGroupIDs: %v", err)
	}
	if backfilled != int64(len(rows)) {
		t.Fatalf("backfilled rows = %d, want %d", backfilled, len(rows))
	}

	var persisted []tables.WebSessionItemTable
	if err := db.Order("id ASC").Find(&persisted).Error; err != nil {
		t.Fatalf("load backfilled rows: %v", err)
	}
	got := make([]string, 0, len(persisted))
	for _, row := range persisted {
		if row.CommandGroupID == nil {
			t.Fatalf("row %s command_group_id remains NULL", row.ID)
		}
		got = append(got, *row.CommandGroupID)
	}
	want := []string{"cmdgrp_one", "", "", ""}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("command group ids = %#v, want %#v", got, want)
	}

	backfilled, err = backfillWebSessionItemCommandGroupIDs()
	if err != nil {
		t.Fatalf("second backfillWebSessionItemCommandGroupIDs: %v", err)
	}
	if backfilled != 0 {
		t.Fatalf("second backfill changed %d rows, want 0", backfilled)
	}
}

func TestWebSessionItemCommandGroupIndexColumns(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "command-group-index.db")
	if err := InitWithDSN(dsn, 0, true); err != nil {
		t.Fatalf("InitWithDSN: %v", err)
	}
	t.Cleanup(DBClose)

	if !db.Migrator().HasIndex(&tables.WebSessionItemTable{}, "idx_web_session_item_group") {
		t.Fatal("missing idx_web_session_item_group")
	}

	var columns []struct {
		Seq  int    `gorm:"column:seqno"`
		Name string `gorm:"column:name"`
	}
	if err := db.Raw("PRAGMA index_info('idx_web_session_item_group')").Scan(&columns).Error; err != nil {
		t.Fatalf("load command group index columns: %v", err)
	}
	got := make([]string, 0, len(columns))
	for _, column := range columns {
		got = append(got, column.Name)
	}
	want := []string{"web_session_id", "command_group_id", "source_thread_id", "order_index"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("command group index columns = %#v, want %#v", got, want)
	}
}
