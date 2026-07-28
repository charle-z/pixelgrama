package app

import (
	"net/http"
	"strings"
	"testing"
)

func TestEmbeddedRemixAndReducedWallMetadataContract(t *testing.T) {
	handler, _ := newTestHandler(t, 100)
	response := request(handler, http.MethodGet, "/wall", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", response.Code, response.Body.String())
	}
	html := response.Body.String()
	for _, required := range []string{
		`new URLSearchParams(window.location.search)`,
		`/p/" + remixID + ".json`,
		`payload.parent_id = remixParentID`,
		`format_version !== 1`,
		`palette_id !== "vga16"`,
		`className = "share-link"`,
		`href = "/p/" + item.id`,
		`parentId`,
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("remix frontend missing %q", required)
		}
	}
	for _, forbidden := range []string{
		`item.commit.slice`,
		`" · " + commit + " · "`,
	} {
		if strings.Contains(html, forbidden) {
			t.Fatalf("wall still exposes technical metadata %q", forbidden)
		}
	}
}
