package app

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"strconv"
	"strings"

	"github.com/charle-z/pixelgrama/internal/core"
	"github.com/charle-z/pixelgrama/internal/store"
)

const postcardPNGScale = 16

var vgaPalette = mustPaletteColors(core.DefaultPaletteID, core.DefaultPaletteVersion)

func mustPaletteColors(id string, version int) [core.PaletteSize]color.RGBA {
	colors, err := paletteColors(id, version)
	if err != nil {
		panic(err)
	}
	return colors
}

func paletteColors(id string, version int) ([core.PaletteSize]color.RGBA, error) {
	var colors [core.PaletteSize]color.RGBA
	for index := range colors {
		value, err := core.PaletteColor(id, version, index)
		if err != nil {
			return colors, err
		}
		colors[index] = value
	}
	return colors, nil
}

func parseSharePath(path string) (int64, string, bool) {
	if !strings.HasPrefix(path, "/p/") {
		return 0, "", false
	}
	value := strings.TrimPrefix(path, "/p/")
	if value == "" || strings.Contains(value, "/") {
		return 0, "", false
	}
	format := "html"
	for _, candidate := range []struct {
		suffix string
		format string
	}{
		{suffix: ".json", format: "json"},
		{suffix: ".png", format: "png"},
	} {
		if strings.HasSuffix(value, candidate.suffix) {
			value = strings.TrimSuffix(value, candidate.suffix)
			format = candidate.format
			break
		}
	}
	if strings.Contains(value, ".") {
		return 0, "", false
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id < 1 {
		return 0, "", false
	}
	return id, format, true
}

func (s *server) handleShare(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET is allowed")
		return
	}
	id, format, ok := parseSharePath(r.URL.Path)
	if !ok {
		s.writeError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	item, err := s.store.GetPublic(r.Context(), id)
	if errors.Is(err, store.ErrPostcardNotFound) {
		s.writeError(w, http.StatusNotFound, "not_found", "postcard not found")
		return
	}
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "storage_error", "postcard could not be loaded")
		return
	}

	switch format {
	case "json":
		s.writeJSON(w, http.StatusOK, item)
	case "png":
		data, err := renderPostcardPNGWithPalette(item.Pixels, item.PaletteID, item.PaletteVersion)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, "render_error", "postcard image could not be rendered")
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=\"pixelgrama-%d.png\"", item.ID))
		w.Header().Set("ETag", `"`+item.ContentHash+`"`)
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	default:
		page, err := renderSharePage(item)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, "render_error", "postcard page could not be rendered")
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(page)
	}
}

func renderPostcardPNG(pixels core.Pixels) ([]byte, error) {
	return renderPostcardPNGWithPalette(pixels, core.DefaultPaletteID, core.DefaultPaletteVersion)
}

func renderPostcardPNGWithPalette(pixels core.Pixels, paletteID string, paletteVersion int) ([]byte, error) {
	colors, err := paletteColors(paletteID, paletteVersion)
	if err != nil {
		return nil, err
	}
	size := 16 * postcardPNGScale
	imageValue := image.NewRGBA(image.Rect(0, 0, size, size))
	for index, value := range pixels {
		x0 := (index % 16) * postcardPNGScale
		y0 := (index / 16) * postcardPNGScale
		fill := colors[value]
		for y := y0; y < y0+postcardPNGScale; y++ {
			for x := x0; x < x0+postcardPNGScale; x++ {
				imageValue.SetRGBA(x, y, fill)
			}
		}
	}
	var output bytes.Buffer
	encoder := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := encoder.Encode(&output, imageValue); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}
