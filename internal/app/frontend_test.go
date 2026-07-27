package app

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"regexp"
	"strings"
	"testing"
)

func TestEmbeddedWallFrontendContract(t *testing.T) {
	handler, _ := newTestHandler(t, 100)
	response := request(handler, http.MethodGet, "/wall", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", response.Code, response.Body.String())
	}
	html := response.Body.String()
	for _, required := range []string{
		`data-es=`,
		`data-en=`,
		`<canvas id="editor"`,
		`id="palette"`,
		`id="alias"`,
		`maxlength="16"`,
		`id="wall"`,
		`/wall?format=json`,
		`/postcard`,
		`commit-api-test`,
		`https://github.com/charle-z/pixelgrama`,
		`https://github.com/charle-z/pixelgrama/pull/1`,
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("frontend missing %q", required)
		}
	}

	for _, color := range []string{
		"#000000", "#0000AA", "#00AA00", "#00AAAA",
		"#AA0000", "#AA00AA", "#AA5500", "#AAAAAA",
		"#555555", "#5555FF", "#55FF55", "#55FFFF",
		"#FF5555", "#FF55FF", "#FFFF55", "#FFFFFF",
	} {
		if !strings.Contains(html, color) {
			t.Fatalf("frontend missing VGA color %s", color)
		}
	}

	lower := strings.ToLower(html)
	for _, forbidden := range []string{
		"linear-gradient", "radial-gradient", "border-radius", "box-shadow",
		"animation:", "transition:", "innerhtml", "<img", "<svg",
	} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("frontend contains forbidden construct %q", forbidden)
		}
	}
}

func TestInlineFrontendBlocksAreAuthorizedByExactCSPHashes(t *testing.T) {
	handler, _ := newTestHandler(t, 100)
	response := request(handler, http.MethodGet, "/wall", nil)
	html := response.Body.String()
	csp := response.Header().Get("Content-Security-Policy")

	style := inlineBlock(t, html, `(?s)<style>(.*?)</style>`)
	script := inlineBlock(t, html, `(?s)<script>(.*?)</script>`)
	styleHash := cspHash(style)
	scriptHash := cspHash(script)

	if !strings.Contains(csp, "style-src 'sha256-"+styleHash+"'") {
		t.Fatalf("CSP %q does not authorize exact inline style hash %s", csp, styleHash)
	}
	if !strings.Contains(csp, "script-src 'sha256-"+scriptHash+"'") {
		t.Fatalf("CSP %q does not authorize exact inline script hash %s", csp, scriptHash)
	}
	if strings.Contains(csp, "unsafe-inline") {
		t.Fatalf("CSP permits unsafe-inline: %q", csp)
	}
}

func inlineBlock(t *testing.T, body, pattern string) string {
	t.Helper()
	matches := regexp.MustCompile(pattern).FindStringSubmatch(body)
	if len(matches) != 2 {
		t.Fatalf("expected exactly one inline block matching %s", pattern)
	}
	return matches[1]
}

func cspHash(value string) string {
	digest := sha256.Sum256([]byte(value))
	return base64.StdEncoding.EncodeToString(digest[:])
}
