package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charle-z/pixelgrama/internal/core"
	"github.com/charle-z/pixelgrama/internal/ratelimit"
	"github.com/charle-z/pixelgrama/internal/store"
)

var fixedNow = time.Date(2026, 7, 27, 4, 8, 0, 0, time.UTC)

func newTestHandler(t *testing.T, limit int) (http.Handler, *store.Store) {
	t.Helper()
	database, err := store.Open(filepath.Join(t.TempDir(), "pixelgrama.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	handler, err := New(Config{
		Store:     database,
		Limiter:   ratelimit.New(limit, time.Minute, 128, func() time.Time { return fixedNow }),
		Commit:    "commit-api-test",
		RepoURL:   "https://github.com/charle-z/pixelgrama",
		PRURL:     "https://github.com/charle-z/pixelgrama/pull/1",
		Now:       func() time.Time { return fixedNow },
		BodyLimit: 4096,
	})
	if err != nil {
		t.Fatal(err)
	}
	return handler, database
}

func pixelValues(offset int) []int {
	values := make([]int, core.PixelCount)
	for i := range values {
		values[i] = (i + offset) % core.PaletteSize
	}
	return values
}

func postcardBody(t *testing.T, offset int, alias any) []byte {
	t.Helper()
	payload := map[string]any{"pixels": pixelValues(offset)}
	if alias != nil {
		payload["alias"] = alias
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func request(handler http.Handler, method, target string, body []byte) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(method, target, bytes.NewReader(body))
	req.RemoteAddr = "192.0.2.10:43120"
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	handler.ServeHTTP(recorder, req)
	return recorder
}

func TestExactRoutesAndMethods(t *testing.T) {
	handler, _ := newTestHandler(t, 100)
	tests := []struct {
		method string
		path   string
		want   int
	}{
		{http.MethodGet, "/", http.StatusPermanentRedirect},
		{http.MethodGet, "/missing", http.StatusNotFound},
		{http.MethodGet, "/wall/", http.StatusNotFound},
		{http.MethodGet, "/healthz/", http.StatusNotFound},
		{http.MethodGet, "/postcard", http.StatusMethodNotAllowed},
		{http.MethodPut, "/postcard", http.StatusMethodNotAllowed},
		{http.MethodPost, "/wall", http.StatusMethodNotAllowed},
		{http.MethodPost, "/healthz", http.StatusMethodNotAllowed},
		{http.MethodPost, "/readyz", http.StatusMethodNotAllowed},
		{http.MethodPost, "/version", http.StatusMethodNotAllowed},
		{http.MethodGet, "/healthz", http.StatusOK},
		{http.MethodGet, "/readyz", http.StatusOK},
		{http.MethodGet, "/version", http.StatusOK},
		{http.MethodGet, "/wall", http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			got := request(handler, tt.method, tt.path, nil)
			if got.Code != tt.want {
				t.Fatalf("status = %d, want %d; body=%s", got.Code, tt.want, got.Body.String())
			}
		})
	}
}

func TestRootRedirectsPermanentlyToWall(t *testing.T) {
	handler, _ := newTestHandler(t, 100)
	response := request(handler, http.MethodGet, "/", nil)
	if response.Code != http.StatusPermanentRedirect {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusPermanentRedirect)
	}
	if location := response.Header().Get("Location"); location != "/wall" {
		t.Fatalf("Location = %q, want /wall", location)
	}
}

func TestSecurityHeadersAreExplicitOnEveryResponse(t *testing.T) {
	handler, _ := newTestHandler(t, 100)
	for _, target := range []string{"/healthz", "/readyz", "/version", "/challenge", "/wall", "/week", "/week.png", "/missing"} {
		t.Run(target, func(t *testing.T) {
			response := request(handler, http.MethodGet, target, nil)
			csp := response.Header().Get("Content-Security-Policy")
			for _, directive := range []string{
				"default-src 'none'",
				"connect-src 'self'",
				"script-src ",
				"style-src ",
				"img-src 'self'",
				"base-uri 'none'",
				"form-action 'none'",
				"frame-ancestors 'none'",
				"object-src 'none'",
			} {
				if !strings.Contains(csp, directive) {
					t.Fatalf("CSP %q missing %q", csp, directive)
				}
			}
			if strings.Contains(csp, "unsafe-inline") {
				t.Fatalf("CSP permits unsafe-inline: %q", csp)
			}
			want := map[string]string{
				"X-Content-Type-Options":       "nosniff",
				"Referrer-Policy":              "no-referrer",
				"Cross-Origin-Opener-Policy":   "same-origin",
				"Cross-Origin-Resource-Policy": "same-origin",
			}
			for name, value := range want {
				if got := response.Header().Get(name); got != value {
					t.Fatalf("%s = %q, want %q", name, got, value)
				}
			}
			if response.Header().Get("Permissions-Policy") == "" {
				t.Fatal("Permissions-Policy is missing")
			}
		})
	}
}

func TestPostcardAcceptsExactTypedPayloadAndStoresCommit(t *testing.T) {
	handler, _ := newTestHandler(t, 100)
	response := request(handler, http.MethodPost, "/postcard", postcardBody(t, 0, "CHARLES"))
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d; body=%s", response.Code, response.Body.String())
	}
	var item store.Postcard
	if err := json.Unmarshal(response.Body.Bytes(), &item); err != nil {
		t.Fatal(err)
	}
	if item.Commit != "commit-api-test" || item.Alias == nil || *item.Alias != "CHARLES" {
		t.Fatalf("unexpected postcard: %#v", item)
	}
	if len(item.Pixels) != core.PixelCount {
		t.Fatalf("pixels = %d", len(item.Pixels))
	}
}

func TestPostcardRejectsMalformedAndStructurallyInvalidPayloads(t *testing.T) {
	handler, _ := newTestHandler(t, 100)
	zeros := func(count int, value string) string {
		parts := make([]string, count)
		for i := range parts {
			parts[i] = value
		}
		return strings.Join(parts, ",")
	}
	tests := []struct {
		name string
		body string
		want int
	}{
		{"malformed", `{"pixels":[0`, http.StatusBadRequest},
		{"missing pixels", `{}`, http.StatusBadRequest},
		{"pixels not array", `{"pixels":"no"}`, http.StatusBadRequest},
		{"short", `{"pixels":[` + zeros(255, "0") + `]}`, http.StatusUnprocessableEntity},
		{"long", `{"pixels":[` + zeros(257, "0") + `]}`, http.StatusUnprocessableEntity},
		{"negative", `{"pixels":[` + zeros(255, "0") + `,-1]}`, http.StatusUnprocessableEntity},
		{"too high", `{"pixels":[` + zeros(255, "0") + `,16]}`, http.StatusUnprocessableEntity},
		{"fraction", `{"pixels":[` + zeros(255, "0") + `,1.0]}`, http.StatusUnprocessableEntity},
		{"exponent", `{"pixels":[` + zeros(255, "0") + `,1e0]}`, http.StatusUnprocessableEntity},
		{"string pixel", `{"pixels":[` + zeros(255, "0") + `,"1"]}`, http.StatusUnprocessableEntity},
		{"boolean pixel", `{"pixels":[` + zeros(255, "0") + `,true]}`, http.StatusUnprocessableEntity},
		{"invalid alias", `{"pixels":[` + zeros(256, "0") + `],"alias":"<script>"}`, http.StatusUnprocessableEntity},
		{"long alias", `{"pixels":[` + zeros(256, "0") + `],"alias":"12345678901234567"}`, http.StatusUnprocessableEntity},
		{"unknown field", `{"pixels":[` + zeros(256, "0") + `],"html":"<b>x</b>"}`, http.StatusBadRequest},
		{"trailing value", `{"pixels":[` + zeros(256, "0") + `]} {}`, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := request(handler, http.MethodPost, "/postcard", []byte(tt.body))
			if response.Code != tt.want {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, tt.want, response.Body.String())
			}
			if !strings.Contains(response.Header().Get("Content-Type"), "application/json") {
				t.Fatalf("content type = %q", response.Header().Get("Content-Type"))
			}
			var apiError map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &apiError); err != nil {
				t.Fatalf("error response is not JSON: %v", err)
			}
			if apiError["error"] == "" || apiError["message"] == "" {
				t.Fatalf("error response is not explicit: %#v", apiError)
			}
		})
	}
}

func TestPostcardRejectsOversizedBody(t *testing.T) {
	handler, _ := newTestHandler(t, 100)
	body := append(postcardBody(t, 0, nil), bytes.Repeat([]byte(" "), 5000)...)
	response := request(handler, http.MethodPost, "/postcard", body)
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d; body=%s", response.Code, response.Body.String())
	}
}

func TestPostcardDeduplicatesConsecutivePixelsRegardlessOfAlias(t *testing.T) {
	handler, _ := newTestHandler(t, 100)
	first := request(handler, http.MethodPost, "/postcard", postcardBody(t, 0, "FIRST"))
	if first.Code != http.StatusCreated {
		t.Fatalf("first status = %d", first.Code)
	}
	second := request(handler, http.MethodPost, "/postcard", postcardBody(t, 0, "OTHER"))
	if second.Code != http.StatusConflict {
		t.Fatalf("second status = %d; body=%s", second.Code, second.Body.String())
	}
}

func TestPostcardRateLimitUsesRemoteAddressByDefault(t *testing.T) {
	handler, _ := newTestHandler(t, 1)
	first := request(handler, http.MethodPost, "/postcard", postcardBody(t, 0, nil))
	if first.Code != http.StatusCreated {
		t.Fatalf("first status = %d", first.Code)
	}
	second := request(handler, http.MethodPost, "/postcard", postcardBody(t, 1, nil))
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d; body=%s", second.Code, second.Body.String())
	}
}

func TestWallJSONPaginationIsBounded(t *testing.T) {
	handler, database := newTestHandler(t, 1000)
	for i := 0; i < 70; i++ {
		pixels, err := core.FromInts(pixelValues(i % 2))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := database.Insert(context.Background(), pixels, nil, "seed", fixedNow.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	response := request(handler, http.MethodGet, "/wall?format=json&limit=999", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Postcards    []store.Postcard `json:"postcards"`
		Limit        int              `json:"limit"`
		NextBeforeID *int64           `json:"next_before_id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Limit != MaxWallLimit || len(payload.Postcards) != MaxWallLimit {
		t.Fatalf("limit=%d postcards=%d want=%d", payload.Limit, len(payload.Postcards), MaxWallLimit)
	}
	if payload.Postcards[0].ID <= payload.Postcards[len(payload.Postcards)-1].ID {
		t.Fatal("postcards are not newest first")
	}
	if payload.NextBeforeID == nil || *payload.NextBeforeID != payload.Postcards[len(payload.Postcards)-1].ID {
		t.Fatalf("next cursor = %#v", payload.NextBeforeID)
	}
}

func TestWallRejectsInvalidPagination(t *testing.T) {
	handler, _ := newTestHandler(t, 100)
	for _, target := range []string{
		"/wall?format=json&page=0",
		"/wall?format=json&page=-1",
		"/wall?format=json&page=no",
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

func TestHealthVersionAndWallRepresentations(t *testing.T) {
	handler, _ := newTestHandler(t, 100)
	health := request(handler, http.MethodGet, "/healthz", nil)
	if strings.TrimSpace(health.Body.String()) != `{"status":"ok"}` {
		t.Fatalf("health body = %q", health.Body.String())
	}
	version := request(handler, http.MethodGet, "/version", nil)
	var metadata map[string]string
	if err := json.Unmarshal(version.Body.Bytes(), &metadata); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"commit":       "commit-api-test",
		"repository":   "https://github.com/charle-z/pixelgrama",
		"pull_request": "https://github.com/charle-z/pixelgrama/pull/1",
	}
	for key, value := range want {
		if metadata[key] != value {
			t.Fatalf("version[%s] = %q, want %q", key, metadata[key], value)
		}
	}
	html := request(handler, http.MethodGet, "/wall", nil)
	if got := html.Header().Get("Content-Type"); !strings.Contains(got, "text/html") {
		t.Fatalf("HTML content type = %q", got)
	}
	jsonResponse := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/wall", nil)
	req.Header.Set("Accept", "application/json")
	handler.ServeHTTP(jsonResponse, req)
	if got := jsonResponse.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("JSON content type = %q", got)
	}
}

func TestTrustedProxyHeaderIsIgnoredUnlessConfigured(t *testing.T) {
	handler, _ := newTestHandler(t, 1)
	for i := 0; i < 2; i++ {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/postcard", bytes.NewReader(postcardBody(t, i, nil)))
		req.RemoteAddr = "192.0.2.50:1234"
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Forwarded-For", fmt.Sprintf("198.51.100.%d", i+1))
		handler.ServeHTTP(recorder, req)
		want := http.StatusCreated
		if i == 1 {
			want = http.StatusTooManyRequests
		}
		if recorder.Code != want {
			t.Fatalf("request %d status = %d, want %d", i, recorder.Code, want)
		}
	}
}
