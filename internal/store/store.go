package store

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/charle-z/pixelgrama/internal/core"
	_ "modernc.org/sqlite"
)

var (
	ErrDuplicate      = errors.New("postcard is identical to the latest postcard")
	ErrParentNotFound = errors.New("remix parent postcard is not public")
)

type Postcard struct {
	ID             int64       `json:"id"`
	Pixels         core.Pixels `json:"pixels"`
	Alias          *string     `json:"alias,omitempty"`
	Commit         string      `json:"commit"`
	CreatedAt      time.Time   `json:"created_at"`
	ContentHash    string      `json:"content_hash"`
	FormatVersion  int         `json:"format_version"`
	PaletteID      string      `json:"palette_id"`
	PaletteVersion int         `json:"palette_version"`
	ParentID       *int64      `json:"parent_id,omitempty"`
}

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	for _, statement := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
	} {
		if _, err := db.Exec(statement); err != nil {
			db.Close()
			return nil, fmt.Errorf("configure sqlite: %w", err)
		}
	}

	store := &Store{db: db}
	if err := store.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Insert(ctx context.Context, pixels core.Pixels, alias *string, commit string, createdAt time.Time) (Postcard, error) {
	return s.InsertWithParent(ctx, pixels, alias, commit, createdAt, nil)
}

func (s *Store) InsertWithParent(ctx context.Context, pixels core.Pixels, alias *string, commit string, createdAt time.Time, parentID *int64) (Postcard, error) {
	return s.InsertWithPalette(ctx, pixels, alias, commit, createdAt, core.DefaultPaletteID, core.DefaultPaletteVersion, parentID)
}

func (s *Store) InsertWithPalette(ctx context.Context, pixels core.Pixels, alias *string, commit string, createdAt time.Time, paletteID string, paletteVersion int, parentID *int64) (Postcard, error) {
	if err := core.ValidateAlias(alias); err != nil {
		return Postcard{}, err
	}
	if err := core.ValidatePalette(paletteID, paletteVersion); err != nil {
		return Postcard{}, err
	}
	if parentID != nil && *parentID < 1 {
		return Postcard{}, ErrParentNotFound
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Postcard{}, fmt.Errorf("begin insert: %w", err)
	}
	defer tx.Rollback()

	if parentID != nil {
		var exists int
		if err := tx.QueryRowContext(ctx,
			"SELECT 1 FROM postcards WHERE id = ? AND moderation_status = 'visible'",
			*parentID,
		).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
			return Postcard{}, ErrParentNotFound
		} else if err != nil {
			return Postcard{}, fmt.Errorf("validate remix parent: %w", err)
		}
	}

	var latest []byte
	var latestPaletteID string
	var latestPaletteVersion int
	err = tx.QueryRowContext(ctx, `
SELECT pixels, palette_catalog_id, palette_version
FROM postcards WHERE moderation_status = 'visible' ORDER BY id DESC LIMIT 1`).Scan(&latest, &latestPaletteID, &latestPaletteVersion)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Postcard{}, fmt.Errorf("read latest postcard: %w", err)
	}
	if err == nil && bytes.Equal(latest, pixels[:]) && latestPaletteID == paletteID && latestPaletteVersion == paletteVersion {
		return Postcard{}, ErrDuplicate
	}

	createdAt = createdAt.UTC()
	contentHash := core.ContentHashForPalette(pixels, paletteID, paletteVersion)
	result, err := tx.ExecContext(ctx,
		`INSERT INTO postcards (
pixels, alias, deployed_commit, created_at, content_hash, format_version, palette_id, parent_id, palette_catalog_id, palette_version
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		pixels.Bytes(), nullableAlias(alias), commit, createdAt.Format(time.RFC3339Nano),
		contentHash, core.FormatVersion, core.DefaultPaletteID, nullableParent(parentID), paletteID, paletteVersion,
	)
	if err != nil {
		return Postcard{}, fmt.Errorf("insert postcard: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Postcard{}, fmt.Errorf("read inserted id: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Postcard{}, fmt.Errorf("commit insert: %w", err)
	}
	return Postcard{
		ID:             id,
		Pixels:         pixels,
		Alias:          cloneAlias(alias),
		Commit:         commit,
		CreatedAt:      createdAt,
		ContentHash:    contentHash,
		FormatVersion:  core.FormatVersion,
		PaletteID:      paletteID,
		PaletteVersion: paletteVersion,
		ParentID:       cloneParent(parentID),
	}, nil
}

const postcardColumns = "id, pixels, alias, deployed_commit, created_at, content_hash, format_version, palette_catalog_id, palette_version, CASE WHEN parent_id IS NOT NULL AND EXISTS (SELECT 1 FROM postcards AS parent WHERE parent.id = postcards.parent_id AND parent.moderation_status = 'visible') THEN parent_id ELSE NULL END"

func (s *Store) List(ctx context.Context, limit, offset int) ([]Postcard, error) {
	if limit < 1 || offset < 0 {
		return nil, errors.New("limit must be positive and offset non-negative")
	}
	rows, err := s.db.QueryContext(ctx,
		"SELECT "+postcardColumns+" FROM postcards WHERE moderation_status = 'visible' ORDER BY id DESC LIMIT ? OFFSET ?",
		limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list postcards: %w", err)
	}
	defer rows.Close()

	items := make([]Postcard, 0, limit)
	for rows.Next() {
		item, err := scanPostcard(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate postcards: %w", err)
	}
	return items, nil
}

func (s *Store) GetPublic(ctx context.Context, id int64) (Postcard, error) {
	if id < 1 {
		return Postcard{}, ErrPostcardNotFound
	}
	row := s.db.QueryRowContext(ctx,
		"SELECT "+postcardColumns+" FROM postcards WHERE id = ? AND moderation_status = 'visible'",
		id,
	)
	item, err := scanPostcard(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Postcard{}, ErrPostcardNotFound
	}
	if err != nil {
		return Postcard{}, err
	}
	return item, nil
}

func scanPostcard(row rowScanner) (Postcard, error) {
	var item Postcard
	var data []byte
	var alias sql.NullString
	var created string
	var parent sql.NullInt64
	if err := row.Scan(
		&item.ID,
		&data,
		&alias,
		&item.Commit,
		&created,
		&item.ContentHash,
		&item.FormatVersion,
		&item.PaletteID,
		&item.PaletteVersion,
		&parent,
	); err != nil {
		return Postcard{}, err
	}
	pixels, err := core.PixelsFromBytes(data)
	if err != nil {
		return Postcard{}, fmt.Errorf("decode postcard %d: %w", item.ID, err)
	}
	item.Pixels = pixels
	if alias.Valid {
		value := alias.String
		item.Alias = &value
	}
	item.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return Postcard{}, fmt.Errorf("decode postcard time %d: %w", item.ID, err)
	}
	if parent.Valid {
		value := parent.Int64
		item.ParentID = &value
	}
	return item, nil
}

func nullableAlias(alias *string) any {
	if alias == nil || *alias == "" {
		return nil
	}
	return *alias
}

func cloneAlias(alias *string) *string {
	if alias == nil || *alias == "" {
		return nil
	}
	value := *alias
	return &value
}

func nullableParent(parentID *int64) any {
	if parentID == nil {
		return nil
	}
	return *parentID
}

func cloneParent(parentID *int64) *int64 {
	if parentID == nil {
		return nil
	}
	value := *parentID
	return &value
}
