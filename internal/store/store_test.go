package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/charle-z/pixelgrama/internal/core"
)

func testPixels(t *testing.T, offset int) core.Pixels {
	t.Helper()
	values := make([]int, core.PixelCount)
	for i := range values {
		values[i] = (i + offset) % core.PaletteSize
	}
	pixels, err := core.FromInts(values)
	if err != nil {
		t.Fatal(err)
	}
	return pixels
}

func TestStoreInsertDeduplicateAndList(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "pixelgrama.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	firstAlias := "FIRST"
	firstTime := time.Date(2026, 7, 27, 4, 1, 0, 0, time.UTC)
	first, err := store.Insert(ctx, testPixels(t, 0), &firstAlias, "abc123", firstTime)
	if err != nil {
		t.Fatal(err)
	}
	if first.Commit != "abc123" || first.Alias == nil || *first.Alias != firstAlias {
		t.Fatalf("unexpected first postcard: %#v", first)
	}

	otherAlias := "DIFFERENT"
	if _, err := store.Insert(ctx, testPixels(t, 0), &otherAlias, "abc123", firstTime.Add(time.Second)); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("duplicate error = %v, want ErrDuplicate", err)
	}

	second, err := store.Insert(ctx, testPixels(t, 1), nil, "def456", firstTime.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	items, err := store.List(ctx, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	if items[0].ID != second.ID || items[0].Commit != "def456" {
		t.Fatalf("items not newest first: %#v", items)
	}
	if items[1].Pixels != first.Pixels {
		t.Fatal("stored pixels changed")
	}
}

func TestStoreListIsBoundedByCaller(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "pixelgrama.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if _, err := store.Insert(ctx, testPixels(t, i), nil, "commit", time.Now().UTC().Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	items, err := store.List(ctx, 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
}
