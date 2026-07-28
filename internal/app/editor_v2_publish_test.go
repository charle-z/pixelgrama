package app

import (
	"net/http"
	"strings"
	"testing"
)

func TestEmbeddedEditorV2PublicationAndLocalizationContract(t *testing.T) {
	handler, _ := newTestHandler(t, 100)
	response := request(handler, http.MethodGet, "/wall", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", response.Code, response.Body.String())
	}
	html := response.Body.String()
	for _, required := range []string{
		`Intl.DateTimeFormat`,
		`if (publishing)`,
		`publishNode.disabled = publishing`,
		`pixelgrama:language`,
		`pixelgrama:draft`,
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("editor v2 publication contract missing %q", required)
		}
	}
}
