package core

import (
	"testing"
)

func validInts() []int {
	pixels := make([]int, PixelCount)
	for i := range pixels {
		pixels[i] = i % PaletteSize
	}
	return pixels
}

func TestFromIntsAcceptsExactPixels(t *testing.T) {
	pixels, err := FromInts(validInts())
	if err != nil {
		t.Fatalf("FromInts() error = %v", err)
	}
	for i, value := range pixels {
		if value != uint8(i%PaletteSize) {
			t.Fatalf("pixel %d = %d", i, value)
		}
	}
}

func TestFromIntsRejectsWrongLengthAndRange(t *testing.T) {
	tests := []struct {
		name   string
		pixels []int
	}{
		{name: "short", pixels: make([]int, PixelCount-1)},
		{name: "long", pixels: make([]int, PixelCount+1)},
		{name: "negative", pixels: func() []int { p := validInts(); p[7] = -1; return p }()},
		{name: "too high", pixels: func() []int { p := validInts(); p[42] = PaletteSize; return p }()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := FromInts(tt.pixels); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestAliasValidation(t *testing.T) {
	valid := []string{"", "Charles", "PIXEL 16", "a_b-c", "1234567890123456"}
	for _, alias := range valid {
		alias := alias
		if err := ValidateAlias(&alias); err != nil {
			t.Fatalf("ValidateAlias(%q) error = %v", alias, err)
		}
	}
	if err := ValidateAlias(nil); err != nil {
		t.Fatalf("ValidateAlias(nil) error = %v", err)
	}
	invalid := []string{"12345678901234567", "<script>", "á", "line\nbreak", "dot.name"}
	for _, alias := range invalid {
		alias := alias
		if err := ValidateAlias(&alias); err == nil {
			t.Fatalf("ValidateAlias(%q) expected error", alias)
		}
	}
}

func TestPixelsByteRoundTrip(t *testing.T) {
	pixels, err := FromInts(validInts())
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := PixelsFromBytes(pixels.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if decoded != pixels {
		t.Fatal("round trip changed pixels")
	}
	if _, err := PixelsFromBytes(make([]byte, PixelCount-1)); err == nil {
		t.Fatal("expected invalid byte length error")
	}
}
