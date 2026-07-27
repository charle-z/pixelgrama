package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
)

const CurrentSchemaVersion = 1

var (
	ErrFutureSchema = errors.New("database schema is newer than this binary")
	ErrBackupExists = errors.New("backup destination already exists")
)

const schemaV1 = `
CREATE TABLE IF NOT EXISTS postcards (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    pixels BLOB NOT NULL CHECK(length(pixels) = 256),
    alias TEXT NULL CHECK(alias IS NULL OR (length(alias) <= 16 AND alias NOT GLOB '*[^A-Za-z0-9 _-]*')),
    deployed_commit TEXT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS postcards_created_at_idx ON postcards(id DESC);`

func (s *Store) migrate(ctx context.Context) error {
	version, err := s.SchemaVersion(ctx)
	if err != nil {
		return fmt.Errorf("read sqlite schema version: %w", err)
	}
	if version > CurrentSchemaVersion {
		return fmt.Errorf("%w: database=%d supported=%d", ErrFutureSchema, version, CurrentSchemaVersion)
	}
	if version == CurrentSchemaVersion {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin sqlite migration: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, schemaV1); err != nil {
		return fmt.Errorf("apply sqlite schema v1: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "PRAGMA user_version = 1"); err != nil {
		return fmt.Errorf("set sqlite schema version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit sqlite migration: %w", err)
	}
	return nil
}

func (s *Store) SchemaVersion(ctx context.Context) (int, error) {
	var version int
	if err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return 0, err
	}
	return version, nil
}

func (s *Store) Ready(ctx context.Context) error {
	if err := s.db.PingContext(ctx); err != nil {
		return fmt.Errorf("sqlite ping: %w", err)
	}
	version, err := s.SchemaVersion(ctx)
	if err != nil {
		return fmt.Errorf("sqlite schema version: %w", err)
	}
	if version != CurrentSchemaVersion {
		return fmt.Errorf("sqlite schema version is %d, want %d", version, CurrentSchemaVersion)
	}
	return nil
}

func (s *Store) Backup(ctx context.Context, destination string) error {
	if destination == "" {
		return errors.New("backup destination is required")
	}
	if _, err := os.Stat(destination); err == nil {
		return ErrBackupExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect backup destination: %w", err)
	}
	quoted := "'" + strings.ReplaceAll(destination, "'", "''") + "'"
	if _, err := s.db.ExecContext(ctx, "VACUUM INTO "+quoted); err != nil {
		return fmt.Errorf("create sqlite backup: %w", err)
	}
	if err := verifyBackup(ctx, destination); err != nil {
		_ = os.Remove(destination)
		return err
	}
	return nil
}

func verifyBackup(ctx context.Context, path string) error {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return fmt.Errorf("open backup: %w", err)
	}
	defer db.Close()
	var integrity string
	if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity); err != nil {
		return fmt.Errorf("check backup integrity: %w", err)
	}
	if integrity != "ok" {
		return fmt.Errorf("backup integrity check returned %q", integrity)
	}
	var version int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("read backup schema version: %w", err)
	}
	if version != CurrentSchemaVersion {
		return fmt.Errorf("backup schema version is %d, want %d", version, CurrentSchemaVersion)
	}
	var count int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM postcards").Scan(&count); err != nil {
		return fmt.Errorf("read backup postcards: %w", err)
	}
	return nil
}
