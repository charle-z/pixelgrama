package app

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/charle-z/pixelgrama/internal/challenge"
)

func TestDailyChallengeEndpoint(t *testing.T) {
	handler, _ := newTestHandler(t, 100)
	response := request(handler, http.MethodGet, "/challenge", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("content type = %q", got)
	}
	var item challenge.Daily
	if err := json.Unmarshal(response.Body.Bytes(), &item); err != nil {
		t.Fatal(err)
	}
	if item.Date != "2026-07-27" || item.CatalogVersion != 1 || item.Slug == "" || item.PromptES == "" || item.PromptEN == "" {
		t.Fatalf("unexpected challenge: %#v", item)
	}
}

func TestDailyChallengeRejectsNonGETMethods(t *testing.T) {
	handler, _ := newTestHandler(t, 100)
	response := request(handler, http.MethodPost, "/challenge", nil)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d; body=%s", response.Code, response.Body.String())
	}
	if allow := response.Header().Get("Allow"); allow != http.MethodGet {
		t.Fatalf("Allow = %q", allow)
	}
}

func TestEmbeddedDailyChallengeContract(t *testing.T) {
	handler, _ := newTestHandler(t, 100)
	response := request(handler, http.MethodGet, "/wall", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	html := response.Body.String()
	for _, required := range []string{
		`id="daily-challenge"`,
		`id="challenge-date"`,
		`id="challenge-prompt"`,
		`fetch("/challenge"`,
		`normalizeDailyChallenge`,
		`catalog_version`,
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("daily challenge frontend missing %q", required)
		}
	}
}
