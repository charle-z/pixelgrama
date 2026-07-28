package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/charle-z/pixelgrama/internal/core"
)

type PaletteStat struct {
	PaletteID      string `json:"palette_id"`
	PaletteVersion int    `json:"palette_version"`
	Postcards      int    `json:"postcards"`
}

type PublicStats struct {
	SchemaVersion     int           `json:"schema_version"`
	WeekKey           string        `json:"week_key"`
	TotalPostcards    int           `json:"total_postcards"`
	PostcardsThisWeek int           `json:"postcards_this_week"`
	RemixCount        int           `json:"remix_count"`
	Palettes          []PaletteStat `json:"palettes"`
}

func (s *Store) PublicStats(ctx context.Context, start, end time.Time, weekKey string) (PublicStats, error) {
	start = start.UTC()
	end = end.UTC()
	if !end.After(start) || weekKey == "" {
		return PublicStats{}, errors.New("statistics week must be ordered and identified")
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return PublicStats{}, fmt.Errorf("begin public statistics snapshot: %w", err)
	}
	defer tx.Rollback()

	stats := PublicStats{SchemaVersion: 1, WeekKey: weekKey}
	if err := tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM postcards
WHERE moderation_status = 'visible'`).Scan(&stats.TotalPostcards); err != nil {
		return PublicStats{}, fmt.Errorf("count public postcards: %w", err)
	}
	if err := tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM postcards
WHERE moderation_status = 'visible' AND created_at >= ? AND created_at < ?`,
		start.Format(time.RFC3339Nano), end.Format(time.RFC3339Nano),
	).Scan(&stats.PostcardsThisWeek); err != nil {
		return PublicStats{}, fmt.Errorf("count weekly public postcards: %w", err)
	}
	if err := tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM postcards AS child
WHERE child.moderation_status = 'visible'
  AND child.parent_id IS NOT NULL
  AND EXISTS (
      SELECT 1 FROM postcards AS parent
      WHERE parent.id = child.parent_id AND parent.moderation_status = 'visible'
  )`).Scan(&stats.RemixCount); err != nil {
		return PublicStats{}, fmt.Errorf("count public remixes: %w", err)
	}

	rows, err := tx.QueryContext(ctx, `
SELECT palette_catalog_id, palette_version, COUNT(*)
FROM postcards
WHERE moderation_status = 'visible'
GROUP BY palette_catalog_id, palette_version`)
	if err != nil {
		return PublicStats{}, fmt.Errorf("group public postcards by palette: %w", err)
	}
	counts := make(map[string]int)
	for rows.Next() {
		var id string
		var version int
		var count int
		if err := rows.Scan(&id, &version, &count); err != nil {
			_ = rows.Close()
			return PublicStats{}, fmt.Errorf("scan public palette statistics: %w", err)
		}
		counts[fmt.Sprintf("%s@%d", id, version)] = count
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return PublicStats{}, fmt.Errorf("iterate public palette statistics: %w", err)
	}
	if err := rows.Close(); err != nil {
		return PublicStats{}, fmt.Errorf("close public palette statistics: %w", err)
	}

	catalog := core.Catalog()
	stats.Palettes = make([]PaletteStat, 0, len(catalog.Palettes))
	for _, palette := range catalog.Palettes {
		stats.Palettes = append(stats.Palettes, PaletteStat{
			PaletteID:      palette.ID,
			PaletteVersion: palette.Version,
			Postcards:      counts[fmt.Sprintf("%s@%d", palette.ID, palette.Version)],
		})
	}
	if err := tx.Commit(); err != nil {
		return PublicStats{}, fmt.Errorf("commit public statistics snapshot: %w", err)
	}
	return stats, nil
}
