package core

import "testing"

func TestContentHashIsStableAndBoundToFormatAndPalette(t *testing.T) {
	pixels, err := FromInts(make([]int, PixelCount))
	if err != nil {
		t.Fatal(err)
	}
	first := ContentHash(pixels)
	second := ContentHash(pixels)
	if first != second {
		t.Fatalf("hash changed: %q != %q", first, second)
	}
	if len(first) != 64 {
		t.Fatalf("hash length = %d, want 64", len(first))
	}
	if FormatVersion != 1 {
		t.Fatalf("format version = %d, want 1", FormatVersion)
	}
	if PaletteID != "vga16" {
		t.Fatalf("palette id = %q, want vga16", PaletteID)
	}
	changed := pixels
	changed[0] = 1
	if ContentHash(changed) == first {
		t.Fatal("pixel change did not change content hash")
	}
}
