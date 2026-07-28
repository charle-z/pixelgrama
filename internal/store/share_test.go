package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/charle-z/pixelgrama/internal/core"
)

func TestOpenMigratesVersionTwoContentIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "version-two.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	pixels := testPixels(t, 3)
	if _, err := db.Exec(schemaV1+schemaV2+`
PRAGMA user_version = 2;
INSERT INTO postcards (pixels, alias, deployed_commit, created_at)
VALUES (?, 'LEGACY', 'old-commit', '2026-07-27T00:00:00Z');`, pixels.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	database, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	items, err := database.List(context.Background(), 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	item := items[0]
	if item.ContentHash != core.ContentHash(pixels) {
		t.Fatalf("content hash = %q", item.ContentHash)
	}
	if item.FormatVersion != core.FormatVersion || item.PaletteID != core.DefaultPaletteID || item.PaletteVersion != core.DefaultPaletteVersion || item.ParentID != nil {
		t.Fatalf("migrated identity = %#v", item)
	}
}

func TestInsertWithParentAndGetPublic(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "remix.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	ctx := context.Background()
	now := time.Date(2026, 7, 28, 2, 0, 0, 0, time.UTC)
	parent, err := database.Insert(ctx, testPixels(t, 1), nil, "parent", now)
	if err != nil {
		t.Fatal(err)
	}
	child, err := database.InsertWithParent(ctx, testPixels(t, 2), nil, "child", now.Add(time.Second), &parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if child.ParentID == nil || *child.ParentID != parent.ID {
		t.Fatalf("child parent = %#v", child.ParentID)
	}
	if child.ContentHash != core.ContentHash(child.Pixels) || child.FormatVersion != core.FormatVersion || child.PaletteID != core.DefaultPaletteID || child.PaletteVersion != core.DefaultPaletteVersion {
		t.Fatalf("child identity = %#v", child)
	}
	loaded, err := database.GetPublic(ctx, child.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ID != child.ID || loaded.ParentID == nil || *loaded.ParentID != parent.ID {
		t.Fatalf("loaded child = %#v", loaded)
	}
}

func TestInsertWithParentRejectsMissingOrHiddenParent(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "invalid-parent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	ctx := context.Background()
	now := time.Now().UTC()
	missing := int64(999999)
	if _, err := database.InsertWithParent(ctx, testPixels(t, 2), nil, "child", now, &missing); !errors.Is(err, ErrParentNotFound) {
		t.Fatalf("missing parent error = %v", err)
	}
	parent, err := database.Insert(ctx, testPixels(t, 3), nil, "parent", now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Hide(ctx, parent.ID, "not public", now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := database.InsertWithParent(ctx, testPixels(t, 4), nil, "child", now.Add(2*time.Second), &parent.ID); !errors.Is(err, ErrParentNotFound) {
		t.Fatalf("hidden parent error = %v", err)
	}
	if _, err := database.GetPublic(ctx, parent.ID); !errors.Is(err, ErrPostcardNotFound) {
		t.Fatalf("hidden GetPublic error = %v", err)
	}
}

func TestHiddenParentIsNotExposedThroughPublicChild(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "hidden-parent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	ctx := context.Background()
	now := time.Now().UTC()
	parent, err := database.Insert(ctx, testPixels(t, 8), nil, "parent", now)
	if err != nil {
		t.Fatal(err)
	}
	child, err := database.InsertWithParent(ctx, testPixels(t, 9), nil, "child", now.Add(time.Second), &parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Hide(ctx, parent.ID, "hidden", now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	loaded, err := database.GetPublic(ctx, child.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ParentID != nil {
		t.Fatalf("hidden parent leaked through child: %#v", loaded.ParentID)
	}
}
