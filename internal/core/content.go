package core

import (
	"crypto/sha256"
	"encoding/hex"
)

const (
	FormatVersion = 1
	PaletteID     = "vga16"
)

var contentDomain = []byte("pixelgrama:postcard:v1:vga16:")

func ContentHash(pixels Pixels) string {
	hash := sha256.New()
	_, _ = hash.Write(contentDomain)
	_, _ = hash.Write(pixels[:])
	return hex.EncodeToString(hash.Sum(nil))
}
