package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOpenMigratesLegacySchemaAndPreservesRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
CREATE TABLE postcards (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    pixels BLOB NOT NULL CHECK(length(pixels) = 256),
    alias TEXT NULL,
    deployed_commit TEXT NOT NULL,
    created_at TEXT NOT NULL
);
INSERT INTO postcards (pixels, alias, deployed_commit, created_at)
VALUES (zeroblob(256), 'LEGACY', 'old', '2026-07-27T00:00:00Z');`)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	version, err := store.SchemaVersion(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if version != CurrentSchemaVersion {
		t.Fatalf("version = %d, want %d", version, CurrentSchemaVersion)
	}
	items, err := store.List(context.Background(), 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Alias == nil || *items[0].Alias != "LEGACY" {
		t.Fatalf("legacy rows not preserved: %#v", items)
	}
}

func TestOpenRejectsFutureSchemaVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "future.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("PRAGMA user_version = 99"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path); !errors.Is(err, ErrFutureSchema) {
		t.Fatalf("Open() error = %v, want ErrFutureSchema", err)
	}
}

func TestReadyChecksSQLiteAndSchema(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "ready.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Ready(context.Background()); err != nil {
		t.Fatalf("Ready() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := store.Ready(context.Background()); err == nil {
		t.Fatal("Ready() succeeded after close")
	}
}

func TestBackupIsConsistentAndRestorable(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.db")
	destination := filepath.Join(dir, "backup.db")
	store, err := Open(source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Insert(context.Background(), testPixels(t, 3), nil, "backup-test", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := store.Backup(context.Background(), destination); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(destination); err != nil {
		t.Fatal(err)
	}
	if err := store.Backup(context.Background(), destination); !errors.Is(err, ErrBackupExists) {
		t.Fatalf("second Backup() error = %v, want ErrBackupExists", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	restored, err := Open(destination)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	items, err := restored.List(context.Background(), 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Commit != "backup-test" {
		t.Fatalf("backup rows = %#v", items)
	}
}
