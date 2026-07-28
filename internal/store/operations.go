package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/charle-z/pixelgrama/internal/core"
)

const CurrentSchemaVersion = 4

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

const schemaV3 = `
ALTER TABLE postcards ADD COLUMN content_hash TEXT NOT NULL DEFAULT '0000000000000000000000000000000000000000000000000000000000000000'
    CHECK(length(content_hash) = 64 AND content_hash NOT GLOB '*[^0-9a-f]*');
ALTER TABLE postcards ADD COLUMN format_version INTEGER NOT NULL DEFAULT 1 CHECK(format_version = 1);
ALTER TABLE postcards ADD COLUMN palette_id TEXT NOT NULL DEFAULT 'vga16' CHECK(palette_id = 'vga16');
ALTER TABLE postcards ADD COLUMN parent_id INTEGER NULL REFERENCES postcards(id) ON DELETE SET NULL;
CREATE INDEX postcards_parent_idx ON postcards(parent_id, id DESC);
CREATE TRIGGER postcards_content_identity_insert
BEFORE INSERT ON postcards
WHEN NEW.content_hash = '0000000000000000000000000000000000000000000000000000000000000000'
  OR length(NEW.content_hash) != 64
  OR NEW.content_hash GLOB '*[^0-9a-f]*'
  OR NEW.format_version != 1
  OR NEW.palette_id != 'vga16'
BEGIN
    SELECT RAISE(ABORT, 'invalid postcard content identity');
END;
CREATE TRIGGER postcards_content_identity_update
BEFORE UPDATE OF content_hash, format_version, palette_id ON postcards
WHEN NEW.content_hash = '0000000000000000000000000000000000000000000000000000000000000000'
  OR length(NEW.content_hash) != 64
  OR NEW.content_hash GLOB '*[^0-9a-f]*'
  OR NEW.format_version != 1
  OR NEW.palette_id != 'vga16'
BEGIN
    SELECT RAISE(ABORT, 'invalid postcard content identity');
END;`

const schemaV4 = `
ALTER TABLE postcards ADD COLUMN palette_catalog_id TEXT NOT NULL DEFAULT 'vga16'
    CHECK(palette_catalog_id IN ('vga16', 'grayscale16', 'sunset16'));
ALTER TABLE postcards ADD COLUMN palette_version INTEGER NOT NULL DEFAULT 1
    CHECK(palette_version = 1);
CREATE INDEX postcards_palette_idx ON postcards(palette_catalog_id, palette_version, id DESC);
CREATE TRIGGER postcards_palette_identity_insert
BEFORE INSERT ON postcards
WHEN NOT (
    (NEW.palette_catalog_id = 'vga16' AND NEW.palette_version = 1)
    OR (NEW.palette_catalog_id = 'grayscale16' AND NEW.palette_version = 1)
    OR (NEW.palette_catalog_id = 'sunset16' AND NEW.palette_version = 1)
)
BEGIN
    SELECT RAISE(ABORT, 'invalid postcard palette identity');
END;
CREATE TRIGGER postcards_palette_identity_update
BEFORE UPDATE OF palette_catalog_id, palette_version ON postcards
WHEN NOT (
    (NEW.palette_catalog_id = 'vga16' AND NEW.palette_version = 1)
    OR (NEW.palette_catalog_id = 'grayscale16' AND NEW.palette_version = 1)
    OR (NEW.palette_catalog_id = 'sunset16' AND NEW.palette_version = 1)
)
BEGIN
    SELECT RAISE(ABORT, 'invalid postcard palette identity');
END;`

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
		case 3:
			schema = schemaV3
		case 4:
			schema = schemaV4
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
		if nextVersion == 3 {
			if err := populateContentIdentity(ctx, tx); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("populate sqlite content identity: %w", err)
			}
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

func populateContentIdentity(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, "SELECT id, pixels FROM postcards ORDER BY id")
	if err != nil {
		return err
	}
	type identityItem struct {
		id     int64
		pixels core.Pixels
	}
	items := make([]identityItem, 0)
	for rows.Next() {
		var id int64
		var data []byte
		if err := rows.Scan(&id, &data); err != nil {
			_ = rows.Close()
			return err
		}
		pixels, err := core.PixelsFromBytes(data)
		if err != nil {
			_ = rows.Close()
			return fmt.Errorf("decode postcard %d: %w", id, err)
		}
		items = append(items, identityItem{id: id, pixels: pixels})
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, item := range items {
		if _, err := tx.ExecContext(ctx, `
UPDATE postcards
SET content_hash = ?, format_version = ?, palette_id = ?
WHERE id = ?`, core.ContentHash(item.pixels), core.FormatVersion, core.DefaultPaletteID, item.id); err != nil {
			return err
		}
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
