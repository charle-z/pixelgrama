package app

import (
	"strings"
	"testing"
)

func TestWallUsesCursorPaginationAndRandomExploration(t *testing.T) {
	page, _, err := buildPage("commit", "https://example.test/repo", "https://example.test/pr")
	if err != nil {
		t.Fatal(err)
	}
	html := string(page)
	for _, required := range []string{
		`id="random-postcard"`,
		`href="/random"`,
		`next_before_id`,
		`before_id=`,
		`wallRequestPath`,
		`normalizeWallPage`,
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("embedded wall is missing %q", required)
		}
	}
	if strings.Contains(html, `page=`) {
		t.Fatal("embedded wall still uses offset page pagination")
	}
}
