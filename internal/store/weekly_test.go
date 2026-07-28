package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestListPublicBetweenUsesUTCWindowAndExcludesHidden(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "pixelgrama.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	ctx := context.Background()
	start := time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC)
	end := start.Add(7 * 24 * time.Hour)

	if _, err := database.Insert(ctx, testPixels(t, 1), nil, "before", start.Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	first, err := database.Insert(ctx, testPixels(t, 2), nil, "first", start.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	hidden, err := database.Insert(ctx, testPixels(t, 3), nil, "hidden", start.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	last, err := database.Insert(ctx, testPixels(t, 4), nil, "last", end.Add(-time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Insert(ctx, testPixels(t, 5), nil, "after", end); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Hide(ctx, hidden.ID, "weekly test", end.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	items, total, err := database.ListPublicBetween(ctx, start, end, 64)
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || len(items) != 2 {
		t.Fatalf("total=%d len=%d, want 2", total, len(items))
	}
	if items[0].ID != first.ID || items[1].ID != last.ID {
		t.Fatalf("items not chronological: %#v", items)
	}
}

func TestListPublicBetweenKeepsLatestLimitChronologically(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "pixelgrama.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	ctx := context.Background()
	start := time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC)
	end := start.Add(7 * 24 * time.Hour)
	ids := make([]int64, 0, 70)
	for index := 0; index < 70; index++ {
		item, err := database.Insert(ctx, testPixels(t, index%16), nil, "weekly", start.Add(time.Duration(index)*time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, item.ID)
	}

	items, total, err := database.ListPublicBetween(ctx, start, end, 64)
	if err != nil {
		t.Fatal(err)
	}
	if total != 70 || len(items) != 64 {
		t.Fatalf("total=%d len=%d", total, len(items))
	}
	if items[0].ID != ids[6] || items[len(items)-1].ID != ids[69] {
		t.Fatalf("unexpected selected range: first=%d last=%d", items[0].ID, items[len(items)-1].ID)
	}
	for index := 1; index < len(items); index++ {
		if items[index-1].ID >= items[index].ID {
			t.Fatal("weekly items are not chronological")
		}
	}
}

func TestListPublicBetweenRejectsInvalidArguments(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "pixelgrama.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	ctx := context.Background()
	now := time.Now().UTC()
	for _, test := range []struct {
		start time.Time
		end   time.Time
		limit int
	}{
		{start: now, end: now, limit: 1},
		{start: now.Add(time.Hour), end: now, limit: 1},
		{start: now, end: now.Add(time.Hour), limit: 0},
	} {
		if _, _, err := database.ListPublicBetween(ctx, test.start, test.end, test.limit); err == nil {
			t.Fatal("expected invalid weekly range error")
		}
	}
}
