package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWallLinksToWeeklyMosaic(t *testing.T) {
	handler, _ := newTestHandler(t, 100)
	response := request(handler, http.MethodGet, "/wall", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	html := response.Body.String()
	if !strings.Contains(html, `id="weekly-mosaic-link"`) || !strings.Contains(html, `href="/week"`) {
		t.Fatalf("weekly mosaic link missing: %s", html)
	}
}

func TestWeeklyPNGSupportsConditionalRequest(t *testing.T) {
	handler, database := newTestHandler(t, 100)
	if _, err := database.Insert(context.Background(), solidWeeklyPixels(t, 11), nil, "weekly", fixedNow.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	first := request(handler, http.MethodGet, "/week.png", nil)
	etag := first.Header().Get("ETag")
	if first.Code != http.StatusOK || etag == "" {
		t.Fatalf("first status=%d etag=%q", first.Code, etag)
	}

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/week.png", nil)
	req.Header.Set("If-None-Match", etag)
	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusNotModified {
		t.Fatalf("status = %d, want 304", recorder.Code)
	}
	if recorder.Body.Len() != 0 {
		t.Fatalf("304 body length = %d", recorder.Body.Len())
	}
	if recorder.Header().Get("Content-Security-Policy") == "" {
		t.Fatal("weekly PNG response lost security headers")
	}
}
