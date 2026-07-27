package app

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"path/filepath"
	"testing"
	"time"

	"github.com/charle-z/pixelgrama/internal/ratelimit"
	"github.com/charle-z/pixelgrama/internal/store"
)

func TestPostcardRequiresJSONContentType(t *testing.T) {
	handler, _ := newTestHandler(t, 100)
	for _, test := range []struct {
		name        string
		contentType string
		want        int
	}{
		{name: "missing", want: http.StatusUnsupportedMediaType},
		{name: "plain", contentType: "text/plain", want: http.StatusUnsupportedMediaType},
		{name: "malformed", contentType: "application/json; charset", want: http.StatusUnsupportedMediaType},
		{name: "json", contentType: "application/json", want: http.StatusCreated},
		{name: "json charset", contentType: "application/json; charset=utf-8", want: http.StatusCreated},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/postcard", bytes.NewReader(postcardBody(t, len(test.name), nil)))
			req.RemoteAddr = "192.0.2.10:1000"
			if test.contentType != "" {
				req.Header.Set("Content-Type", test.contentType)
			}
			handler.ServeHTTP(recorder, req)
			if recorder.Code != test.want {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, test.want, recorder.Body.String())
			}
		})
	}
}

func TestReadyzChecksSQLiteWhileHealthzRemainsLiveness(t *testing.T) {
	handler, database := newTestHandler(t, 100)
	ready := request(handler, http.MethodGet, "/readyz", nil)
	if ready.Code != http.StatusOK {
		t.Fatalf("ready status = %d; body=%s", ready.Code, ready.Body.String())
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	ready = request(handler, http.MethodGet, "/readyz", nil)
	if ready.Code != http.StatusServiceUnavailable {
		t.Fatalf("ready after close = %d; body=%s", ready.Code, ready.Body.String())
	}
	health := request(handler, http.MethodGet, "/healthz", nil)
	if health.Code != http.StatusOK {
		t.Fatalf("health after close = %d", health.Code)
	}
}

func TestTrustedProxyCIDRsPreventForwardedForSpoofing(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "proxy.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	prefix := netip.MustParsePrefix("172.16.0.0/12")
	handler, err := New(Config{
		Store:             database,
		Limiter:           ratelimit.New(1, time.Minute, 128, time.Now),
		Commit:            "proxy-test",
		RepoURL:           "https://github.com/charle-z/pixelgrama",
		PRURL:             "https://github.com/charle-z/pixelgrama/pull/10",
		TrustedProxyCIDRs: []netip.Prefix{prefix},
		RateLimitWindow:   time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}

	post := func(remote, forwarded string, offset int) int {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/postcard", bytes.NewReader(postcardBody(t, offset, nil)))
		req.RemoteAddr = remote
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Forwarded-For", forwarded)
		handler.ServeHTTP(recorder, req)
		return recorder.Code
	}

	if got := post("198.51.100.10:1234", "203.0.113.1", 0); got != http.StatusCreated {
		t.Fatalf("first untrusted peer status = %d", got)
	}
	if got := post("198.51.100.10:1234", "203.0.113.2", 1); got != http.StatusTooManyRequests {
		t.Fatalf("spoofed peer status = %d, want 429", got)
	}
	if got := post("172.18.0.5:1234", "203.0.113.20, 172.18.0.4", 2); got != http.StatusCreated {
		t.Fatalf("trusted proxy first client status = %d", got)
	}
	if got := post("172.18.0.5:1234", "203.0.113.21, 172.18.0.4", 3); got != http.StatusCreated {
		t.Fatalf("trusted proxy second client status = %d", got)
	}
}
