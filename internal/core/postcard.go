package core

import (
	"fmt"
	"regexp"
)

const (
	PixelCount     = 256
	PaletteSize    = 16
	AliasMaxLength = 16
)

var aliasPattern = regexp.MustCompile(`^[A-Za-z0-9 _-]{0,16}$`)

type Pixels [PixelCount]uint8

func FromInts(values []int) (Pixels, error) {
	var pixels Pixels
	if len(values) != PixelCount {
		return pixels, fmt.Errorf("pixels must contain exactly %d integers", PixelCount)
	}
	for i, value := range values {
		if value < 0 || value >= PaletteSize {
			return Pixels{}, fmt.Errorf("pixel %d must be between 0 and %d", i, PaletteSize-1)
		}
		pixels[i] = uint8(value)
	}
	return pixels, nil
}

func (p Pixels) Bytes() []byte {
	data := make([]byte, PixelCount)
	copy(data, p[:])
	return data
}

func PixelsFromBytes(data []byte) (Pixels, error) {
	var pixels Pixels
	if len(data) != PixelCount {
		return pixels, fmt.Errorf("encoded pixels must contain exactly %d bytes", PixelCount)
	}
	for i, value := range data {
		if value >= PaletteSize {
			return Pixels{}, fmt.Errorf("encoded pixel %d must be between 0 and %d", i, PaletteSize-1)
		}
		pixels[i] = value
	}
	return pixels, nil
}

func ValidateAlias(alias *string) error {
	if alias == nil {
		return nil
	}
	if len(*alias) > AliasMaxLength || !aliasPattern.MatchString(*alias) {
		return fmt.Errorf("alias must match [A-Za-z0-9 _-] and contain at most %d characters", AliasMaxLength)
	}
	return nil
}
