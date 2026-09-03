package model

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestInspectDatabaseReportsConfiguredSQLiteHealth(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "health.db")
	if err := InitWithDSN(dsn, 0, true); err != nil {
		t.Fatalf("InitWithDSN: %v", err)
	}
	t.Cleanup(DBClose)

	health, err := InspectDatabase(context.Background(), dsn)
	if err != nil {
		t.Fatalf("InspectDatabase: %v", err)
	}
	if !health.Healthy {
		t.Fatalf("database health is unhealthy: %#v", health)
	}
	if health.JournalMode != "wal" {
		t.Fatalf("journal mode = %q, want wal", health.JournalMode)
	}
	if health.BusyTimeoutMs != 5000 {
		t.Fatalf("busy timeout = %d, want 5000", health.BusyTimeoutMs)
	}
	if health.Pool.MaxOpenConnections != 1 {
		t.Fatalf("max open connections = %d, want 1", health.Pool.MaxOpenConnections)
	}
	if health.ReaderPool.MaxOpenConnections != 4 {
		t.Fatalf("reader max open connections = %d, want 4", health.ReaderPool.MaxOpenConnections)
	}
	if GetReaderDB() == nil || GetReaderDB() == GetDB() {
		t.Fatal("file-backed SQLite must use a separate reader pool")
	}
	if err := GetReaderDB().Exec("CREATE TABLE reader_must_be_read_only (id INTEGER)").Error; err == nil {
		t.Fatal("reader pool unexpectedly accepted a write")
	}
	if health.DatabaseBytes <= 0 {
		t.Fatalf("database bytes = %d, want a positive value", health.DatabaseBytes)
	}
	if health.FreeDiskBytes <= 0 {
		t.Fatalf("free disk bytes = %d, want a positive value", health.FreeDiskBytes)
	}
	if !health.ReadProbe.OK || !health.WriteProbe.OK {
		t.Fatalf("probes are not healthy: read=%#v write=%#v", health.ReadProbe, health.WriteProbe)
	}
}

func TestProjectReadsDoNotWaitForWriterConnection(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "reader-isolation.db")
	if err := InitWithDSN(dsn, 0, true); err != nil {
		t.Fatalf("InitWithDSN: %v", err)
	}
	t.Cleanup(DBClose)

	tx, err := sqlDB.BeginTx(context.Background(), &sql.TxOptions{})
	if err != nil {
		t.Fatalf("begin writer transaction: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if _, err := NewProjectService().ListProjects(ctx); err != nil {
		t.Fatalf("reader-backed project list waited for writer connection: %v", err)
	}
}

func TestInMemoryDatabaseUsesWriterAsReader(t *testing.T) {
	if err := InitWithDSN("file:reader-fallback?mode=memory&cache=shared", 0, true); err != nil {
		t.Fatalf("InitWithDSN: %v", err)
	}
	t.Cleanup(DBClose)
	if GetReaderDB() == nil || GetReaderDB() != GetDB() {
		t.Fatal("in-memory SQLite must reuse the writer connection")
	}
}

func TestDatabasePathFromDSN(t *testing.T) {
	tests := []struct {
		name  string
		dsn   string
		want  string
		empty bool
	}{
		{name: "memory", dsn: "file:test?mode=memory&cache=shared", empty: true},
		{name: "file query", dsn: "./data/test.db?_busy_timeout=5000", want: filepath.Join("data", "test.db")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := databasePathFromDSN(tt.dsn)
			if tt.empty {
				if got != "" {
					t.Fatalf("databasePathFromDSN(%q) = %q, want empty", tt.dsn, got)
				}
				return
			}
			abs, err := filepath.Abs(tt.want)
			if err != nil {
				t.Fatal(err)
			}
			if got != abs {
				t.Fatalf("databasePathFromDSN(%q) = %q, want %q", tt.dsn, got, abs)
			}
		})
	}
}
