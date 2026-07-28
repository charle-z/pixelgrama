package core

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"image/color"
	"strconv"
)

const (
	DefaultPaletteID      = "vga16"
	DefaultPaletteVersion = 1
)

type Palette struct {
	ID      string              `json:"id"`
	Version int                 `json:"version"`
	NameES  string              `json:"name_es"`
	NameEN  string              `json:"name_en"`
	Colors  [PaletteSize]string `json:"colors"`
}

type PaletteCatalog struct {
	CatalogVersion int       `json:"catalog_version"`
	Palettes       []Palette `json:"palettes"`
}

//go:embed palettes.json
var paletteCatalogJSON []byte

var paletteCatalog = mustLoadPaletteCatalog()

func mustLoadPaletteCatalog() PaletteCatalog {
	var catalog PaletteCatalog
	if err := json.Unmarshal(paletteCatalogJSON, &catalog); err != nil {
		panic(fmt.Sprintf("decode embedded palette catalog: %v", err))
	}
	if catalog.CatalogVersion != 1 || len(catalog.Palettes) < 1 {
		panic("embedded palette catalog must be non-empty version 1")
	}
	seen := make(map[string]struct{}, len(catalog.Palettes))
	for _, palette := range catalog.Palettes {
		key := paletteKey(palette.ID, palette.Version)
		if palette.ID == "" || palette.Version < 1 || palette.NameES == "" || palette.NameEN == "" {
			panic("embedded palette metadata is incomplete")
		}
		if _, exists := seen[key]; exists {
			panic("embedded palette identifiers must be unique")
		}
		seen[key] = struct{}{}
		for _, value := range palette.Colors {
			if _, err := parseHexColor(value); err != nil {
				panic(fmt.Sprintf("invalid embedded palette color %q: %v", value, err))
			}
		}
	}
	if _, ok := paletteFromCatalog(catalog, DefaultPaletteID, DefaultPaletteVersion); !ok {
		panic("embedded palette catalog is missing the default VGA palette")
	}
	return catalog
}

func Catalog() PaletteCatalog {
	copyValue := PaletteCatalog{CatalogVersion: paletteCatalog.CatalogVersion}
	copyValue.Palettes = append([]Palette(nil), paletteCatalog.Palettes...)
	return copyValue
}

func PaletteByID(id string, version int) (Palette, bool) {
	return paletteFromCatalog(paletteCatalog, id, version)
}

func paletteFromCatalog(catalog PaletteCatalog, id string, version int) (Palette, bool) {
	for _, palette := range catalog.Palettes {
		if palette.ID == id && palette.Version == version {
			return palette, true
		}
	}
	return Palette{}, false
}

func ValidatePalette(id string, version int) error {
	if _, ok := PaletteByID(id, version); !ok {
		return fmt.Errorf("unsupported palette %q version %d", id, version)
	}
	return nil
}

func PaletteColor(id string, version, index int) (color.RGBA, error) {
	palette, ok := PaletteByID(id, version)
	if !ok {
		return color.RGBA{}, fmt.Errorf("unsupported palette %q version %d", id, version)
	}
	if index < 0 || index >= PaletteSize {
		return color.RGBA{}, errors.New("palette color index is outside 0..15")
	}
	return parseHexColor(palette.Colors[index])
}

func parseHexColor(value string) (color.RGBA, error) {
	if len(value) != 7 || value[0] != '#' {
		return color.RGBA{}, errors.New("color must use #RRGGBB")
	}
	parsed, err := strconv.ParseUint(value[1:], 16, 24)
	if err != nil {
		return color.RGBA{}, err
	}
	return color.RGBA{
		R: uint8(parsed >> 16),
		G: uint8(parsed >> 8),
		B: uint8(parsed),
		A: 0xff,
	}, nil
}

func paletteKey(id string, version int) string {
	return fmt.Sprintf("%s@%d", id, version)
}
