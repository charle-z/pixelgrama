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
	if DefaultPaletteID != "vga16" || DefaultPaletteVersion != 1 {
		t.Fatalf("default palette = %s@%d, want vga16@1", DefaultPaletteID, DefaultPaletteVersion)
	}
	changed := pixels
	changed[0] = 1
	if ContentHash(changed) == first {
		t.Fatal("pixel change did not change content hash")
	}
	if ContentHashForPalette(pixels, "grayscale16", 1) == first {
		t.Fatal("palette change did not change content hash")
	}
}

func TestPaletteCatalogIsClosedAndVersioned(t *testing.T) {
	catalog := Catalog()
	if catalog.CatalogVersion != 1 || len(catalog.Palettes) != 3 {
		t.Fatalf("unexpected catalog: %#v", catalog)
	}
	for _, palette := range catalog.Palettes {
		if err := ValidatePalette(palette.ID, palette.Version); err != nil {
			t.Fatal(err)
		}
		if len(palette.Colors) != PaletteSize {
			t.Fatalf("palette %s has %d colors", palette.ID, len(palette.Colors))
		}
	}
	if err := ValidatePalette("arbitrary", 1); err == nil {
		t.Fatal("arbitrary palette was accepted")
	}
	if err := ValidatePalette(DefaultPaletteID, 2); err == nil {
		t.Fatal("unsupported palette version was accepted")
	}
}
