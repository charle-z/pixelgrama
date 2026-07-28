package app

import (
	"net/http"
	"strings"
	"testing"
)

func TestEmbeddedEditorV2Contract(t *testing.T) {
	handler, _ := newTestHandler(t, 100)
	response := request(handler, http.MethodGet, "/wall", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", response.Code, response.Body.String())
	}
	html := response.Body.String()
	for _, required := range []string{
		`tabindex="0"`,
		`id="tool-pencil"`,
		`id="tool-eraser"`,
		`id="tool-fill"`,
		`id="tool-eyedropper"`,
		`id="undo"`,
		`id="redo"`,
		`id="flip-horizontal"`,
		`id="flip-vertical"`,
		`id="keyboard-help"`,
		`pixelgrama:draft`,
		`pixelgrama:language`,
		`DRAFT_VERSION`,
		`formatPostcardDate`,
		`beginStroke`,
		`undo()`,
		`redo()`,
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("editor v2 frontend missing %q", required)
		}
	}
}
