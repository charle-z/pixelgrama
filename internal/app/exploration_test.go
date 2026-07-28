package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/charle-z/pixelgrama/internal/core"
	"github.com/charle-z/pixelgrama/internal/store"
)

type cursorWallPayload struct {
	Postcards    []store.Postcard `json:"postcards"`
	Limit        int              `json:"limit"`
	NextBeforeID *int64           `json:"next_before_id"`
}

func decodeCursorWall(t *testing.T, handler http.Handler, target string) cursorWallPayload {
	t.Helper()
	response := request(handler, http.MethodGet, target, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("%s status = %d; body=%s", target, response.Code, response.Body.String())
	}
	var payload cursorWallPayload
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	return payload
}

func TestWallCursorDoesNotDuplicateOrSkipWhenNewPostcardsArrive(t *testing.T) {
	handler, database := newTestHandler(t, 1000)
	ctx := context.Background()
	created := make([]store.Postcard, 0, 5)
	for i := 0; i < 5; i++ {
		item, err := database.Insert(ctx, mustPixels(t, i), nil, "cursor-api", fixedNow.Add(time.Duration(i)*time.Second))
		if err != nil {
			t.Fatal(err)
		}
		created = append(created, item)
	}

	first := decodeCursorWall(t, handler, "/wall?format=json&limit=2")
	if first.Limit != 2 || len(first.Postcards) != 2 {
		t.Fatalf("first page = %#v", first)
	}
	if first.Postcards[0].ID != created[4].ID || first.Postcards[1].ID != created[3].ID {
		t.Fatalf("first IDs = %d,%d", first.Postcards[0].ID, first.Postcards[1].ID)
	}
	if first.NextBeforeID == nil || *first.NextBeforeID != created[3].ID {
		t.Fatalf("first cursor = %#v", first.NextBeforeID)
	}

	if _, err := database.Insert(ctx, mustPixels(t, 8), nil, "new", fixedNow.Add(20*time.Second)); err != nil {
		t.Fatal(err)
	}
	second := decodeCursorWall(t, handler, fmt.Sprintf("/wall?format=json&limit=2&before_id=%d", *first.NextBeforeID))
	if len(second.Postcards) != 2 || second.Postcards[0].ID != created[2].ID || second.Postcards[1].ID != created[1].ID {
		t.Fatalf("second page = %#v", second)
	}
	if second.NextBeforeID == nil || *second.NextBeforeID != created[1].ID {
		t.Fatalf("second cursor = %#v", second.NextBeforeID)
	}

	third := decodeCursorWall(t, handler, fmt.Sprintf("/wall?format=json&limit=2&before_id=%d", *second.NextBeforeID))
	if len(third.Postcards) != 1 || third.Postcards[0].ID != created[0].ID || third.NextBeforeID != nil {
		t.Fatalf("third page = %#v", third)
	}
}

func TestWallRejectsLegacyPageAndInvalidBeforeID(t *testing.T) {
	handler, _ := newTestHandler(t, 100)
	for _, target := range []string{
		"/wall?format=json&page=1",
		"/wall?format=json&before_id=0",
		"/wall?format=json&before_id=-1",
		"/wall?format=json&before_id=no",
		"/wall?format=json&before_id=9223372036854775808",
		"/wall?format=json&limit=0",
		"/wall?format=json&limit=-1",
		"/wall?format=json&limit=no",
	} {
		response := request(handler, http.MethodGet, target, nil)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("%s status = %d; body=%s", target, response.Code, response.Body.String())
		}
	}
}

func TestRandomRedirectsOnlyToVisiblePostcard(t *testing.T) {
	handler, database := newTestHandler(t, 100)
	empty := request(handler, http.MethodGet, "/random", nil)
	if empty.Code != http.StatusNotFound {
		t.Fatalf("empty random status = %d", empty.Code)
	}
	visible, err := database.Insert(context.Background(), mustPixels(t, 1), nil, "visible", fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	hidden, err := database.Insert(context.Background(), mustPixels(t, 2), nil, "hidden", fixedNow.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Hide(context.Background(), hidden.ID, "not public", fixedNow.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	response := request(handler, http.MethodGet, "/random", nil)
	if response.Code != http.StatusTemporaryRedirect {
		t.Fatalf("random status = %d; body=%s", response.Code, response.Body.String())
	}
	if location := response.Header().Get("Location"); location != fmt.Sprintf("/p/%d", visible.ID) {
		t.Fatalf("Location = %q", location)
	}
	method := request(handler, http.MethodPost, "/random", nil)
	if method.Code != http.StatusMethodNotAllowed || method.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("POST /random status=%d allow=%q", method.Code, method.Header().Get("Allow"))
	}
}

func mustPixels(t *testing.T, offset int) core.Pixels {
	t.Helper()
	pixels, err := core.FromInts(pixelValues(offset))
	if err != nil {
		t.Fatal(err)
	}
	return pixels
}
