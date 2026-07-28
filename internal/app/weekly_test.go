package app

import (
	"bytes"
	"context"
	"image/png"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/charle-z/pixelgrama/internal/core"
)

func solidWeeklyPixels(t *testing.T, value int) core.Pixels {
	t.Helper()
	values := make([]int, core.PixelCount)
	for index := range values {
		values[index] = value
	}
	pixels, err := core.FromInts(values)
	if err != nil {
		t.Fatal(err)
	}
	return pixels
}

func TestISOWeekWindowUsesUTCAndMonday(t *testing.T) {
	value := time.Date(2026, 7, 26, 23, 30, 0, 0, time.FixedZone("UTC-5", -5*60*60))
	week := isoWeekFor(value)
	if week.Key != "2026-W31" {
		t.Fatalf("key = %q", week.Key)
	}
	if !week.Start.Equal(time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("start = %s", week.Start)
	}
	if !week.End.Equal(time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("end = %s", week.End)
	}
}

func TestWeeklyRoutesRenderVisibleCurrentWeekPostcards(t *testing.T) {
	handler, database := newTestHandler(t, 100)
	ctx := context.Background()

	visible, err := database.Insert(ctx, solidWeeklyPixels(t, 9), nil, "weekly", fixedNow.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	hidden, err := database.Insert(ctx, solidWeeklyPixels(t, 12), nil, "weekly", fixedNow.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Hide(ctx, hidden.ID, "weekly test", fixedNow.Add(3*time.Hour)); err != nil {
		t.Fatal(err)
	}

	html := request(handler, http.MethodGet, "/week", nil)
	if html.Code != http.StatusOK {
		t.Fatalf("html status = %d; body=%s", html.Code, html.Body.String())
	}
	if !strings.Contains(html.Body.String(), `id="weekly-mosaic"`) ||
		!strings.Contains(html.Body.String(), `data-week="2026-W31"`) ||
		!strings.Contains(html.Body.String(), `src="/week.png"`) ||
		!strings.Contains(html.Body.String(), `/p/`+strconv.FormatInt(visible.ID, 10)) ||
		strings.Contains(html.Body.String(), `/p/`+strconv.FormatInt(hidden.ID, 10)) {
		t.Fatalf("unexpected weekly HTML: %s", html.Body.String())
	}

	imageResponse := request(handler, http.MethodGet, "/week.png", nil)
	if imageResponse.Code != http.StatusOK {
		t.Fatalf("png status = %d; body=%s", imageResponse.Code, imageResponse.Body.String())
	}
	if contentType := imageResponse.Header().Get("Content-Type"); contentType != "image/png" {
		t.Fatalf("content type = %q", contentType)
	}
	if imageResponse.Header().Get("ETag") == "" {
		t.Fatal("weekly PNG ETag is missing")
	}
	if cache := imageResponse.Header().Get("Cache-Control"); cache != "public, max-age=60" {
		t.Fatalf("cache control = %q", cache)
	}
	if disposition := imageResponse.Header().Get("Content-Disposition"); disposition != `inline; filename="pixelgrama-2026-W31.png"` {
		t.Fatalf("content disposition = %q", disposition)
	}
	decoded, err := png.Decode(bytes.NewReader(imageResponse.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Bounds().Dx() != 512 || decoded.Bounds().Dy() != 512 {
		t.Fatalf("bounds = %v", decoded.Bounds())
	}
	red, green, blue, alpha := decoded.At(1, 1).RGBA()
	want := vgaPalette[9]
	if uint8(red>>8) != want.R || uint8(green>>8) != want.G || uint8(blue>>8) != want.B || uint8(alpha>>8) != want.A {
		t.Fatalf("first tile color = %#v", decoded.At(1, 1))
	}
	red, green, blue, alpha = decoded.At(weeklyTileSize+1, 1).RGBA()
	background := vgaPalette[0]
	if uint8(red>>8) != background.R || uint8(green>>8) != background.G || uint8(blue>>8) != background.B || uint8(alpha>>8) != background.A {
		t.Fatalf("hidden postcard occupied second tile: %#v", decoded.At(weeklyTileSize+1, 1))
	}
}

func TestWeeklyPNGChangesETagWhenContentChanges(t *testing.T) {
	handler, database := newTestHandler(t, 100)
	first := request(handler, http.MethodGet, "/week.png", nil)
	if first.Code != http.StatusOK {
		t.Fatalf("first status = %d", first.Code)
	}
	if _, err := database.Insert(context.Background(), solidWeeklyPixels(t, 10), nil, "weekly", fixedNow.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	second := request(handler, http.MethodGet, "/week.png", nil)
	if second.Code != http.StatusOK {
		t.Fatalf("second status = %d", second.Code)
	}
	if first.Header().Get("ETag") == second.Header().Get("ETag") {
		t.Fatal("weekly PNG ETag did not change after publication")
	}
}

func TestWeeklyRoutesRejectNonGETAndNearMatches(t *testing.T) {
	handler, _ := newTestHandler(t, 100)
	for _, target := range []string{"/week", "/week.png"} {
		response := request(handler, http.MethodPost, target, nil)
		if response.Code != http.StatusMethodNotAllowed {
			t.Fatalf("POST %s status = %d", target, response.Code)
		}
	}
	if response := request(handler, http.MethodGet, "/week/", nil); response.Code != http.StatusNotFound {
		t.Fatalf("GET /week/ status = %d", response.Code)
	}
}
