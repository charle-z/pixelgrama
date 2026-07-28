package app

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/charle-z/pixelgrama/internal/core"
)

func TestWallExcludesHiddenPostcards(t *testing.T) {
	handler, database := newTestHandler(t, 100)
	ctx := context.Background()
	created := time.Date(2026, 7, 28, 4, 0, 0, 0, time.UTC)
	visible, err := database.Insert(ctx, testPixelsForApp(t, 0), nil, "visible", created)
	if err != nil {
		t.Fatal(err)
	}
	hidden, err := database.Insert(ctx, testPixelsForApp(t, 1), nil, "hidden", created.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Hide(ctx, hidden.ID, "administrative review", created.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}

	recorder := request(handler, http.MethodGet, "/wall?format=json", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("wall status = %d; body=%s", recorder.Code, recorder.Body.String())
	}
	var response wallResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Postcards) != 1 || response.Postcards[0].ID != visible.ID {
		t.Fatalf("public wall leaked hidden postcard: %#v", response.Postcards)
	}
}

func testPixelsForApp(t *testing.T, offset int) core.Pixels {
	t.Helper()
	var pixels core.Pixels
	for index := range pixels {
		pixels[index] = uint8((index + offset) % 16)
	}
	return pixels
}
