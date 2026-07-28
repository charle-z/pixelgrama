package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestListBeforeIsStableWhenNewPostcardsArrive(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "cursor.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	ctx := context.Background()
	base := time.Date(2026, 7, 28, 3, 0, 0, 0, time.UTC)
	created := make([]Postcard, 0, 5)
	for i := 0; i < 5; i++ {
		item, err := database.Insert(ctx, testPixels(t, i), nil, "cursor", base.Add(time.Duration(i)*time.Second))
		if err != nil {
			t.Fatal(err)
		}
		created = append(created, item)
	}

	first, next, err := database.ListBefore(ctx, 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 2 || first[0].ID != created[4].ID || first[1].ID != created[3].ID {
		t.Fatalf("first page = %#v", first)
	}
	if next == nil || *next != created[3].ID {
		t.Fatalf("first cursor = %#v, want %d", next, created[3].ID)
	}

	newest, err := database.Insert(ctx, testPixels(t, 8), nil, "new", base.Add(10*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if newest.ID <= created[4].ID {
		t.Fatalf("newest id = %d, previous newest = %d", newest.ID, created[4].ID)
	}

	second, next, err := database.ListBefore(ctx, 2, next)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 2 || second[0].ID != created[2].ID || second[1].ID != created[1].ID {
		t.Fatalf("second page = %#v", second)
	}
	if next == nil || *next != created[1].ID {
		t.Fatalf("second cursor = %#v, want %d", next, created[1].ID)
	}

	third, next, err := database.ListBefore(ctx, 2, next)
	if err != nil {
		t.Fatal(err)
	}
	if len(third) != 1 || third[0].ID != created[0].ID || next != nil {
		t.Fatalf("third page = %#v next=%#v", third, next)
	}
}

func TestListBeforeRejectsInvalidArguments(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "invalid-cursor.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	zero := int64(0)
	if _, _, err := database.ListBefore(context.Background(), 0, nil); err == nil {
		t.Fatal("ListBefore accepted zero limit")
	}
	if _, _, err := database.ListBefore(context.Background(), 1, &zero); err == nil {
		t.Fatal("ListBefore accepted zero cursor")
	}
}

func TestRandomPublicExcludesHiddenAndReturnsNotFoundWhenEmpty(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "random.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	ctx := context.Background()
	if _, err := database.RandomPublic(ctx); !errors.Is(err, ErrPostcardNotFound) {
		t.Fatalf("empty RandomPublic error = %v", err)
	}
	visible, err := database.Insert(ctx, testPixels(t, 1), nil, "visible", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	hidden, err := database.Insert(ctx, testPixels(t, 2), nil, "hidden", time.Now().UTC().Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Hide(ctx, hidden.ID, "hidden from exploration", time.Now().UTC().Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		item, err := database.RandomPublic(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if item.ID != visible.ID {
			t.Fatalf("RandomPublic returned hidden postcard: %#v", item)
		}
	}
}
