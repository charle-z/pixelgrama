package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

func (s *Store) ListBefore(ctx context.Context, limit int, beforeID *int64) ([]Postcard, *int64, error) {
	if limit < 1 {
		return nil, nil, errors.New("limit must be positive")
	}
	if beforeID != nil && *beforeID < 1 {
		return nil, nil, errors.New("before ID must be positive")
	}

	query := "SELECT " + postcardColumns + " FROM postcards WHERE moderation_status = 'visible'"
	args := make([]any, 0, 2)
	if beforeID != nil {
		query += " AND id < ?"
		args = append(args, *beforeID)
	}
	query += " ORDER BY id DESC LIMIT ?"
	args = append(args, limit+1)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("list postcards before cursor: %w", err)
	}
	defer rows.Close()

	items := make([]Postcard, 0, limit+1)
	for rows.Next() {
		item, err := scanPostcard(rows)
		if err != nil {
			return nil, nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate postcards before cursor: %w", err)
	}

	var nextBeforeID *int64
	if len(items) > limit {
		items = items[:limit]
		cursor := items[len(items)-1].ID
		nextBeforeID = &cursor
	}
	return items, nextBeforeID, nil
}

func (s *Store) RandomPublic(ctx context.Context) (Postcard, error) {
	row := s.db.QueryRowContext(ctx,
		"SELECT "+postcardColumns+" FROM postcards WHERE moderation_status = 'visible' ORDER BY random() LIMIT 1",
	)
	item, err := scanPostcard(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Postcard{}, ErrPostcardNotFound
	}
	if err != nil {
		return Postcard{}, fmt.Errorf("select random public postcard: %w", err)
	}
	return item, nil
}
