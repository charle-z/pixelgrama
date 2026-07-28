package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/charle-z/pixelgrama/internal/store"
)

func TestPostcardPNGContentDispositionIsStandardsCompliant(t *testing.T) {
	handler, _ := newTestHandler(t, 100)
	created := request(handler, http.MethodPost, "/postcard", postcardBody(t, 11, nil))
	var item store.Postcard
	if err := json.Unmarshal(created.Body.Bytes(), &item); err != nil {
		t.Fatal(err)
	}
	response := request(handler, http.MethodGet, fmt.Sprintf("/p/%d.png", item.ID), nil)
	want := fmt.Sprintf("inline; filename=\"pixelgrama-%d.png\"", item.ID)
	if got := response.Header().Get("Content-Disposition"); got != want {
		t.Fatalf("Content-Disposition = %q, want %q", got, want)
	}
}
