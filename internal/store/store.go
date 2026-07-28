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

var ErrDuplicate = errors.New("postcard is identical to the latest postcard")

type Postcard struct {
	ID        int64       `json:"id"`
	Pixels    core.Pixels `json:"pixels"`
	Alias     *string     `json:"alias,omitempty"`
	Commit    string      `json:"commit"`
	CreatedAt time.Time   `json:"created_at"`
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
	if err := core.ValidateAlias(alias); err != nil {
		return Postcard{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Postcard{}, fmt.Errorf("begin insert: %w", err)
	}
	defer tx.Rollback()

	var latest []byte
	err = tx.QueryRowContext(ctx, "SELECT pixels FROM postcards WHERE moderation_status = 'visible' ORDER BY id DESC LIMIT 1").Scan(&latest)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Postcard{}, fmt.Errorf("read latest postcard: %w", err)
	}
	if err == nil && bytes.Equal(latest, pixels[:]) {
		return Postcard{}, ErrDuplicate
	}

	createdAt = createdAt.UTC()
	result, err := tx.ExecContext(ctx,
		"INSERT INTO postcards (pixels, alias, deployed_commit, created_at) VALUES (?, ?, ?, ?)",
		pixels.Bytes(), nullableAlias(alias), commit, createdAt.Format(time.RFC3339Nano),
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
	return Postcard{ID: id, Pixels: pixels, Alias: cloneAlias(alias), Commit: commit, CreatedAt: createdAt}, nil
}

func (s *Store) List(ctx context.Context, limit, offset int) ([]Postcard, error) {
	if limit < 1 || offset < 0 {
		return nil, errors.New("limit must be positive and offset non-negative")
	}
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, pixels, alias, deployed_commit, created_at FROM postcards WHERE moderation_status = 'visible' ORDER BY id DESC LIMIT ? OFFSET ?",
		limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list postcards: %w", err)
	}
	defer rows.Close()

	items := make([]Postcard, 0, limit)
	for rows.Next() {
		var item Postcard
		var data []byte
		var alias sql.NullString
		var created string
		if err := rows.Scan(&item.ID, &data, &alias, &item.Commit, &created); err != nil {
			return nil, fmt.Errorf("scan postcard: %w", err)
		}
		item.Pixels, err = core.PixelsFromBytes(data)
		if err != nil {
			return nil, fmt.Errorf("decode postcard %d: %w", item.ID, err)
		}
		if alias.Valid {
			value := alias.String
			item.Alias = &value
		}
		item.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
		if err != nil {
			return nil, fmt.Errorf("decode postcard time %d: %w", item.ID, err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate postcards: %w", err)
	}
	return items, nil
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
