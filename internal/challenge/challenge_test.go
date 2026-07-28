package challenge

import (
	"regexp"
	"testing"
	"time"
)

func TestForDateIsDeterministicWithinUTCDay(t *testing.T) {
	morning := time.Date(2026, 7, 28, 0, 1, 0, 0, time.UTC)
	evening := time.Date(2026, 7, 28, 23, 59, 59, 0, time.UTC)
	first := ForDate(morning)
	second := ForDate(evening)
	if first != second {
		t.Fatalf("same UTC day produced different challenges: %#v != %#v", first, second)
	}
	if first.Date != "2026-07-28" {
		t.Fatalf("date = %q", first.Date)
	}
	if first.CatalogVersion != 1 {
		t.Fatalf("catalog version = %d", first.CatalogVersion)
	}
}

func TestForDateUsesUTCInsteadOfLocalCalendarDate(t *testing.T) {
	local := time.Date(2026, 7, 28, 20, 30, 0, 0, time.FixedZone("UTC-5", -5*60*60))
	item := ForDate(local)
	if item.Date != "2026-07-29" {
		t.Fatalf("UTC date = %q", item.Date)
	}
}

func TestForDateChangesOnNextUTCDay(t *testing.T) {
	first := ForDate(time.Date(2026, 7, 28, 23, 59, 59, 0, time.UTC))
	second := ForDate(time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC))
	if first.Date == second.Date {
		t.Fatalf("dates did not change: %#v %#v", first, second)
	}
	if first.Slug == second.Slug {
		t.Fatalf("adjacent days reused challenge %q", first.Slug)
	}
}

func TestCatalogEntriesAreBoundedPlainText(t *testing.T) {
	slugPattern := regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	for _, item := range catalog {
		if !slugPattern.MatchString(item.Slug) {
			t.Fatalf("invalid slug %q", item.Slug)
		}
		for language, prompt := range map[string]string{"es": item.PromptES, "en": item.PromptEN} {
			if len(prompt) == 0 || len([]rune(prompt)) > 96 {
				t.Fatalf("%s prompt length invalid for %q", language, item.Slug)
			}
			for _, value := range prompt {
				if value < 0x20 || value == 0x7f || value == '<' || value == '>' {
					t.Fatalf("%s prompt contains unsafe rune %q for %q", language, value, item.Slug)
				}
			}
		}
	}
}
