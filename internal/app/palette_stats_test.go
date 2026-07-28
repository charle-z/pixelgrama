package app

import (
	"encoding/json"
	"image/png"
	"net/http"
	"strconv"
	"testing"

	"github.com/charle-z/pixelgrama/internal/core"
	"github.com/charle-z/pixelgrama/internal/store"
)

func palettePostcardBody(t *testing.T, offset int, paletteID string, paletteVersion int, parentID *int64) []byte {
	t.Helper()
	payload := map[string]any{
		"pixels":          pixelValues(offset),
		"palette_id":      paletteID,
		"palette_version": paletteVersion,
	}
	if parentID != nil {
		payload["parent_id"] = *parentID
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func decodePostcardResponse(t *testing.T, responseBody []byte) store.Postcard {
	t.Helper()
	var item store.Postcard
	if err := json.Unmarshal(responseBody, &item); err != nil {
		t.Fatal(err)
	}
	return item
}

func TestPaletteCatalogEndpointIsClosedAndVersioned(t *testing.T) {
	handler, _ := newTestHandler(t, 100)
	response := request(handler, http.MethodGet, "/palettes", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", response.Code, response.Body.String())
	}
	var catalog core.PaletteCatalog
	if err := json.Unmarshal(response.Body.Bytes(), &catalog); err != nil {
		t.Fatal(err)
	}
	if catalog.CatalogVersion != 1 || len(catalog.Palettes) != 3 {
		t.Fatalf("unexpected catalog: %#v", catalog)
	}
	for _, palette := range catalog.Palettes {
		if err := core.ValidatePalette(palette.ID, palette.Version); err != nil {
			t.Fatal(err)
		}
		if len(palette.Colors) != core.PaletteSize {
			t.Fatalf("palette %s has %d colors", palette.ID, len(palette.Colors))
		}
	}
	method := request(handler, http.MethodPost, "/palettes", nil)
	if method.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /palettes status = %d", method.Code)
	}
}

func TestPostcardPaletteIdentitySurvivesJSONPNGAndDeduplication(t *testing.T) {
	handler, _ := newTestHandler(t, 100)

	legacy := request(handler, http.MethodPost, "/postcard", postcardBody(t, 1, "VGA"))
	if legacy.Code != http.StatusCreated {
		t.Fatalf("legacy status = %d; body=%s", legacy.Code, legacy.Body.String())
	}
	legacyItem := decodePostcardResponse(t, legacy.Body.Bytes())
	if legacyItem.PaletteID != core.DefaultPaletteID || legacyItem.PaletteVersion != core.DefaultPaletteVersion {
		t.Fatalf("legacy palette = %s@%d", legacyItem.PaletteID, legacyItem.PaletteVersion)
	}

	gray := request(handler, http.MethodPost, "/postcard", palettePostcardBody(t, 1, "grayscale16", 1, nil))
	if gray.Code != http.StatusCreated {
		t.Fatalf("grayscale status = %d; body=%s", gray.Code, gray.Body.String())
	}
	grayItem := decodePostcardResponse(t, gray.Body.Bytes())
	if grayItem.PaletteID != "grayscale16" || grayItem.PaletteVersion != 1 {
		t.Fatalf("grayscale palette = %s@%d", grayItem.PaletteID, grayItem.PaletteVersion)
	}
	if grayItem.ContentHash == legacyItem.ContentHash {
		t.Fatal("different palettes produced the same content hash")
	}

	jsonResponse := request(handler, http.MethodGet, "/p/"+jsonID(grayItem.ID)+".json", nil)
	if jsonResponse.Code != http.StatusOK {
		t.Fatalf("json status = %d; body=%s", jsonResponse.Code, jsonResponse.Body.String())
	}
	loaded := decodePostcardResponse(t, jsonResponse.Body.Bytes())
	if loaded.PaletteID != "grayscale16" || loaded.PaletteVersion != 1 {
		t.Fatalf("loaded palette = %s@%d", loaded.PaletteID, loaded.PaletteVersion)
	}

	pngResponse := request(handler, http.MethodGet, "/p/"+jsonID(grayItem.ID)+".png", nil)
	if pngResponse.Code != http.StatusOK {
		t.Fatalf("png status = %d; body=%s", pngResponse.Code, pngResponse.Body.String())
	}
	imageValue, err := png.Decode(pngResponse.Body)
	if err != nil {
		t.Fatal(err)
	}
	got := imageValue.At(0, 0)
	want, err := core.PaletteColor("grayscale16", 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	gotR, gotG, gotB, gotA := got.RGBA()
	wantR, wantG, wantB, wantA := want.RGBA()
	if gotR != wantR || gotG != wantG || gotB != wantB || gotA != wantA {
		t.Fatalf("PNG first pixel = rgba(%d,%d,%d,%d), want rgba(%d,%d,%d,%d)", gotR, gotG, gotB, gotA, wantR, wantG, wantB, wantA)
	}
}

func TestPostcardRejectsIncompleteOrUnsupportedPaletteIdentity(t *testing.T) {
	handler, _ := newTestHandler(t, 100)
	pixels, err := json.Marshal(pixelValues(2))
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{
		`{"pixels":` + string(pixels) + `,"palette_id":"vga16"}`,
		`{"pixels":` + string(pixels) + `,"palette_version":1}`,
		`{"pixels":` + string(pixels) + `,"palette_id":"custom","palette_version":1}`,
		`{"pixels":` + string(pixels) + `,"palette_id":"vga16","palette_version":2}`,
	} {
		response := request(handler, http.MethodPost, "/postcard", []byte(body))
		if response.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d; body=%s", response.Code, response.Body.String())
		}
		var apiError errorResponse
		if err := json.Unmarshal(response.Body.Bytes(), &apiError); err != nil {
			t.Fatal(err)
		}
		if apiError.Error != "invalid_palette" {
			t.Fatalf("error = %#v", apiError)
		}
	}
}

func TestPublicStatsEndpointUsesPublicSQLiteState(t *testing.T) {
	handler, _ := newTestHandler(t, 100)
	parentResponse := request(handler, http.MethodPost, "/postcard", palettePostcardBody(t, 3, "vga16", 1, nil))
	if parentResponse.Code != http.StatusCreated {
		t.Fatalf("parent status = %d; body=%s", parentResponse.Code, parentResponse.Body.String())
	}
	parent := decodePostcardResponse(t, parentResponse.Body.Bytes())
	childResponse := request(handler, http.MethodPost, "/postcard", palettePostcardBody(t, 4, "sunset16", 1, &parent.ID))
	if childResponse.Code != http.StatusCreated {
		t.Fatalf("child status = %d; body=%s", childResponse.Code, childResponse.Body.String())
	}

	response := request(handler, http.MethodGet, "/stats", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("stats status = %d; body=%s", response.Code, response.Body.String())
	}
	var stats store.PublicStats
	if err := json.Unmarshal(response.Body.Bytes(), &stats); err != nil {
		t.Fatal(err)
	}
	if stats.SchemaVersion != 1 || stats.TotalPostcards != 2 || stats.PostcardsThisWeek != 2 || stats.RemixCount != 1 {
		t.Fatalf("unexpected stats: %#v", stats)
	}
	counts := map[string]int{}
	for _, item := range stats.Palettes {
		counts[item.PaletteID] = item.Postcards
	}
	if counts["vga16"] != 1 || counts["grayscale16"] != 0 || counts["sunset16"] != 1 {
		t.Fatalf("unexpected palette counts: %#v", counts)
	}
	method := request(handler, http.MethodPost, "/stats", nil)
	if method.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /stats status = %d", method.Code)
	}
}

func jsonID(id int64) string {
	return strconv.FormatInt(id, 10)
}
