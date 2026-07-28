package core

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

const FormatVersion = 1

var contentDomain = []byte("pixelgrama:postcard:v1:vga16:")

func ContentHash(pixels Pixels) string {
	return ContentHashForPalette(pixels, DefaultPaletteID, DefaultPaletteVersion)
}

func ContentHashForPalette(pixels Pixels, paletteID string, paletteVersion int) string {
	hash := sha256.New()
	if paletteID == DefaultPaletteID && paletteVersion == DefaultPaletteVersion {
		_, _ = hash.Write(contentDomain)
	} else {
		_, _ = fmt.Fprintf(hash, "pixelgrama:postcard:v%d:%s:v%d:", FormatVersion, paletteID, paletteVersion)
	}
	_, _ = hash.Write(pixels[:])
	return hex.EncodeToString(hash.Sum(nil))
}
