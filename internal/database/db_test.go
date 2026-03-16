package database

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestInitSchemaMigratesLegacyTicketsTable(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "legacy.db")

	rawDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	t.Cleanup(func() { _ = rawDB.Close() })

	_, err = rawDB.Exec(`
		CREATE TABLE tickets (
			id TEXT PRIMARY KEY,
			initiator TEXT NOT NULL,
			category TEXT NOT NULL,
			content TEXT NOT NULL,
			resolution TEXT DEFAULT '',
			is_completed BOOLEAN DEFAULT FALSE,
			created_at TEXT NOT NULL,
			completed_at TEXT
		);
		INSERT INTO tickets (id, initiator, category, content, resolution, is_completed, created_at, completed_at)
		VALUES ('ticket-1', 'alice', 'bug', 'legacy payload', '', 0, '2026-03-15T10:00:00Z', NULL);
	`)
	if err != nil {
		t.Fatalf("seed legacy db: %v", err)
	}

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open wrapped db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := db.InitSchema(ctx); err != nil {
		t.Fatalf("init schema: %v", err)
	}

	expectedColumns := []string{"owner_id", "title", "priority", "tags"}
	for _, column := range expectedColumns {
		exists, err := db.columnExists(ctx, "tickets", column)
		if err != nil {
			t.Fatalf("check column %s: %v", column, err)
		}
		if !exists {
			t.Fatalf("expected column %s to exist after migration", column)
		}
	}

	var (
		id        string
		initiator string
		category  string
		content   string
		ownerID   string
		title     string
		priority  int
		tags      string
	)
	err = db.QueryRowContext(ctx, `
		SELECT id, initiator, category, content, owner_id, title, priority, tags
		FROM tickets WHERE id = 'ticket-1'
	`).Scan(&id, &initiator, &category, &content, &ownerID, &title, &priority, &tags)
	if err != nil {
		t.Fatalf("query migrated row: %v", err)
	}

	if id != "ticket-1" || initiator != "alice" || category != "bug" || content != "legacy payload" {
		t.Fatalf("legacy data not preserved: id=%s initiator=%s category=%s content=%s", id, initiator, category, content)
	}
	if ownerID != "" || title != "" || priority != 2 || tags != "" {
		t.Fatalf("unexpected default values after migration: owner_id=%q title=%q priority=%d tags=%q", ownerID, title, priority, tags)
	}
}

func TestInitSchemaCreatesMissingTables(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "fresh.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := db.InitSchema(ctx); err != nil {
		t.Fatalf("init schema: %v", err)
	}

	for _, table := range []string{"users", "sessions", "tickets"} {
		exists, err := db.tableExists(ctx, table)
		if err != nil {
			t.Fatalf("check table %s: %v", table, err)
		}
		if !exists {
			t.Fatalf("expected table %s to exist", table)
		}
	}
}
