package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

func (s *Store) ListPublicBetween(ctx context.Context, start, end time.Time, limit int) ([]Postcard, int, error) {
	start = start.UTC()
	end = end.UTC()
	if !end.After(start) || limit < 1 {
		return nil, 0, errors.New("weekly range must be ordered and limit positive")
	}
	startText := start.Format(time.RFC3339Nano)
	endText := end.Format(time.RFC3339Nano)

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, 0, fmt.Errorf("begin weekly snapshot: %w", err)
	}
	defer tx.Rollback()

	var total int
	if err := tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM postcards
WHERE moderation_status = 'visible' AND created_at >= ? AND created_at < ?`, startText, endText).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count weekly postcards: %w", err)
	}

	rows, err := tx.QueryContext(ctx, `
SELECT `+postcardColumns+`
FROM postcards
WHERE moderation_status = 'visible' AND created_at >= ? AND created_at < ?
ORDER BY id DESC
LIMIT ?`, startText, endText, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("list weekly postcards: %w", err)
	}
	defer rows.Close()

	items := make([]Postcard, 0, min(limit, total))
	for rows.Next() {
		item, err := scanPostcard(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate weekly postcards: %w", err)
	}
	for left, right := 0, len(items)-1; left < right; left, right = left+1, right-1 {
		items[left], items[right] = items[right], items[left]
	}
	if err := tx.Commit(); err != nil {
		return nil, 0, fmt.Errorf("commit weekly snapshot: %w", err)
	}
	return items, total, nil
}
