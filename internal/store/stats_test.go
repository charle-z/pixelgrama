package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestPublicStatsAreDerivedFromVisiblePostcardsOnly(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "pixelgrama.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	ctx := context.Background()
	start := time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC)
	end := start.Add(7 * 24 * time.Hour)
	parent, err := database.InsertWithPalette(ctx, testPixels(t, 1), nil, "stats", start.Add(time.Hour), "vga16", 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.InsertWithPalette(ctx, testPixels(t, 2), nil, "stats", start.Add(2*time.Hour), "grayscale16", 1, &parent.ID); err != nil {
		t.Fatal(err)
	}
	hidden, err := database.InsertWithPalette(ctx, testPixels(t, 3), nil, "stats", start.Add(3*time.Hour), "sunset16", 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Hide(ctx, hidden.ID, "stats test", start.Add(4*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := database.InsertWithPalette(ctx, testPixels(t, 4), nil, "stats", start.Add(-time.Hour), "sunset16", 1, nil); err != nil {
		t.Fatal(err)
	}

	stats, err := database.PublicStats(ctx, start, end, "2026-W31")
	if err != nil {
		t.Fatal(err)
	}
	if stats.SchemaVersion != 1 || stats.TotalPostcards != 3 || stats.PostcardsThisWeek != 2 || stats.RemixCount != 1 {
		t.Fatalf("unexpected stats: %#v", stats)
	}
	if len(stats.Palettes) != 3 {
		t.Fatalf("palette stats = %#v", stats.Palettes)
	}
	counts := map[string]int{}
	for _, item := range stats.Palettes {
		counts[item.PaletteID] = item.Postcards
	}
	if counts["vga16"] != 1 || counts["grayscale16"] != 1 || counts["sunset16"] != 1 {
		t.Fatalf("unexpected palette distribution: %#v", counts)
	}
}

func TestPublicStatsRejectInvalidWeek(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "pixelgrama.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	now := time.Now().UTC()
	if _, err := database.PublicStats(context.Background(), now, now, "2026-W31"); err == nil {
		t.Fatal("expected invalid range error")
	}
	if _, err := database.PublicStats(context.Background(), now, now.Add(time.Hour), ""); err == nil {
		t.Fatal("expected missing week key error")
	}
}

func TestSchemaRejectsPaletteIdentityOutsideClosedCatalog(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "pixelgrama.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	item, err := database.InsertWithPalette(
		context.Background(),
		testPixels(t, 5),
		nil,
		"palette-guard",
		time.Now().UTC(),
		"vga16",
		1,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.db.ExecContext(context.Background(),
		"UPDATE postcards SET palette_catalog_id = 'custom' WHERE id = ?", item.ID); err == nil {
		t.Fatal("database accepted a palette outside the closed catalog")
	}
	if _, err := database.db.ExecContext(context.Background(),
		"UPDATE postcards SET palette_version = 2 WHERE id = ?", item.ID); err == nil {
		t.Fatal("database accepted an unsupported palette version")
	}
}
