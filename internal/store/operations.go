package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
)

const CurrentSchemaVersion = 2

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

const schemaV2 = `
ALTER TABLE postcards ADD COLUMN moderation_status TEXT NOT NULL DEFAULT 'visible'
    CHECK(moderation_status IN ('visible', 'hidden'));
ALTER TABLE postcards ADD COLUMN moderated_at TEXT NULL;
ALTER TABLE postcards ADD COLUMN moderation_reason TEXT NULL CHECK(moderation_reason IS NULL OR length(moderation_reason) BETWEEN 1 AND 256);
CREATE INDEX postcards_public_idx ON postcards(moderation_status, id DESC);
CREATE TABLE moderation_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    postcard_id INTEGER NOT NULL REFERENCES postcards(id) ON DELETE CASCADE,
    action TEXT NOT NULL CHECK(action IN ('hide', 'restore')),
    reason TEXT NOT NULL CHECK(length(reason) BETWEEN 1 AND 256),
    created_at TEXT NOT NULL
);
CREATE INDEX moderation_events_postcard_idx ON moderation_events(postcard_id, id DESC);`

func (s *Store) migrate(ctx context.Context) error {
	version, err := s.SchemaVersion(ctx)
	if err != nil {
		return fmt.Errorf("read sqlite schema version: %w", err)
	}
	if version > CurrentSchemaVersion {
		return fmt.Errorf("%w: database=%d supported=%d", ErrFutureSchema, version, CurrentSchemaVersion)
	}
	for version < CurrentSchemaVersion {
		nextVersion := version + 1
		var schema string
		switch nextVersion {
		case 1:
			schema = schemaV1
		case 2:
			schema = schemaV2
		default:
			return fmt.Errorf("missing sqlite migration for version %d", nextVersion)
		}

		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin sqlite migration %d: %w", nextVersion, err)
		}
		if _, err := tx.ExecContext(ctx, schema); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply sqlite schema v%d: %w", nextVersion, err)
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", nextVersion)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("set sqlite schema version %d: %w", nextVersion, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit sqlite migration %d: %w", nextVersion, err)
		}
		version = nextVersion
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
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM moderation_events").Scan(&count); err != nil {
		return fmt.Errorf("read backup moderation events: %w", err)
	}
	return nil
}
