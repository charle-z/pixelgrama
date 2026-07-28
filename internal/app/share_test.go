package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image/png"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/charle-z/pixelgrama/internal/store"
)

func TestShareablePostcardHTMLJSONAndPNG(t *testing.T) {
	handler, _ := newTestHandler(t, 100)
	created := request(handler, http.MethodPost, "/postcard", postcardBody(t, 3, "SHARE"))
	if created.Code != http.StatusCreated {
		t.Fatalf("create status = %d; body=%s", created.Code, created.Body.String())
	}
	var item store.Postcard
	if err := json.Unmarshal(created.Body.Bytes(), &item); err != nil {
		t.Fatal(err)
	}
	if len(item.ContentHash) != 64 || item.FormatVersion != 1 || item.PaletteID != "vga16" {
		t.Fatalf("created identity = %#v", item)
	}

	html := request(handler, http.MethodGet, fmt.Sprintf("/p/%d", item.ID), nil)
	if html.Code != http.StatusOK || !strings.Contains(html.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("html status=%d content-type=%q", html.Code, html.Header().Get("Content-Type"))
	}
	for _, expected := range []string{
		fmt.Sprintf(`/p/%d.png`, item.ID),
		fmt.Sprintf(`/wall?remix=%d`, item.ID),
		"SHARE",
		item.ContentHash,
	} {
		if !strings.Contains(html.Body.String(), expected) {
			t.Fatalf("share page missing %q", expected)
		}
	}

	jsonResponse := request(handler, http.MethodGet, fmt.Sprintf("/p/%d.json", item.ID), nil)
	if jsonResponse.Code != http.StatusOK || !strings.Contains(jsonResponse.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("json status=%d content-type=%q", jsonResponse.Code, jsonResponse.Header().Get("Content-Type"))
	}
	var shared store.Postcard
	if err := json.Unmarshal(jsonResponse.Body.Bytes(), &shared); err != nil {
		t.Fatal(err)
	}
	if shared.ID != item.ID || shared.ContentHash != item.ContentHash {
		t.Fatalf("shared postcard = %#v", shared)
	}

	pngResponse := request(handler, http.MethodGet, fmt.Sprintf("/p/%d.png", item.ID), nil)
	if pngResponse.Code != http.StatusOK || pngResponse.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("png status=%d content-type=%q", pngResponse.Code, pngResponse.Header().Get("Content-Type"))
	}
	image, err := png.Decode(bytes.NewReader(pngResponse.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if image.Bounds().Dx() != 256 || image.Bounds().Dy() != 256 {
		t.Fatalf("png bounds = %v", image.Bounds())
	}
	first := image.At(0, 0)
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			if image.At(x, y) != first {
				t.Fatalf("first scaled pixel changed at %d,%d", x, y)
			}
		}
	}
}

func TestRemixStoresValidatedPublicParent(t *testing.T) {
	handler, database := newTestHandler(t, 100)
	parentResponse := request(handler, http.MethodPost, "/postcard", postcardBody(t, 1, "PARENT"))
	var parent store.Postcard
	if err := json.Unmarshal(parentResponse.Body.Bytes(), &parent); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{
		"pixels":    pixelValues(2),
		"alias":     "CHILD",
		"parent_id": parent.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	childResponse := request(handler, http.MethodPost, "/postcard", payload)
	if childResponse.Code != http.StatusCreated {
		t.Fatalf("child status = %d; body=%s", childResponse.Code, childResponse.Body.String())
	}
	var child store.Postcard
	if err := json.Unmarshal(childResponse.Body.Bytes(), &child); err != nil {
		t.Fatal(err)
	}
	if child.ParentID == nil || *child.ParentID != parent.ID {
		t.Fatalf("child parent = %#v", child.ParentID)
	}

	if _, err := database.Hide(context.Background(), parent.ID, "private", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	invalidPayload, err := json.Marshal(map[string]any{
		"pixels":    pixelValues(3),
		"parent_id": parent.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	invalid := request(handler, http.MethodPost, "/postcard", invalidPayload)
	if invalid.Code != http.StatusUnprocessableEntity {
		t.Fatalf("hidden parent status = %d; body=%s", invalid.Code, invalid.Body.String())
	}
}

func TestHiddenPostcardIsNotShareable(t *testing.T) {
	handler, database := newTestHandler(t, 100)
	created := request(handler, http.MethodPost, "/postcard", postcardBody(t, 4, nil))
	var item store.Postcard
	if err := json.Unmarshal(created.Body.Bytes(), &item); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Hide(context.Background(), item.ID, "hidden", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"", ".json", ".png"} {
		response := request(handler, http.MethodGet, fmt.Sprintf("/p/%d%s", item.ID, suffix), nil)
		if response.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d", suffix, response.Code)
		}
	}
}

func TestShareRoutesRejectInvalidPathsAndMethods(t *testing.T) {
	handler, _ := newTestHandler(t, 100)
	for _, target := range []string{"/p/", "/p/no", "/p/0", "/p/1.jpeg", "/p/1/extra"} {
		if got := request(handler, http.MethodGet, target, nil); got.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d", target, got.Code)
		}
	}
	if got := request(handler, http.MethodPost, "/p/1", nil); got.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST share status = %d", got.Code)
	}
}
