package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html/template"
	"image"
	"image/draw"
	"image/png"
	"net/http"
	"strconv"
	"time"

	"github.com/charle-z/pixelgrama/internal/store"
)

const (
	weeklyColumns      = 8
	weeklyRows         = 8
	weeklyMaxPostcards = weeklyColumns * weeklyRows
	weeklyPixelScale   = 4
	weeklyTileSize     = 16 * weeklyPixelScale
	weeklyImageSize    = weeklyColumns * weeklyTileSize
)

type isoWeek struct {
	Key   string
	Start time.Time
	End   time.Time
}

type weeklyLink struct {
	ID    int64
	Label string
}

type weeklyPageData struct {
	Style     template.CSS
	WeekKey   string
	StartDate string
	EndDate   string
	Selected  int
	Total     int
	Links     []weeklyLink
	Commit    string
	RepoURL   string
	PRURL     string
}

type weeklySnapshot struct {
	Week      isoWeek
	Postcards []store.Postcard
	Total     int
	ETag      string
}

func isoWeekFor(value time.Time) isoWeek {
	utc := value.UTC()
	year, number := utc.ISOWeek()
	daysSinceMonday := (int(utc.Weekday()) + 6) % 7
	start := time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -daysSinceMonday)
	return isoWeek{
		Key:   fmt.Sprintf("%04d-W%02d", year, number),
		Start: start,
		End:   start.AddDate(0, 0, 7),
	}
}

func (s *server) handleWeekly(w http.ResponseWriter, r *http.Request, format string) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET is allowed")
		return
	}
	snapshot, err := s.weeklySnapshot(r.Context())
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "storage_error", "weekly mosaic could not be loaded")
		return
	}
	if format == "png" {
		if r.Header.Get("If-None-Match") == snapshot.ETag {
			w.Header().Set("ETag", snapshot.ETag)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		data, err := renderWeeklyPNG(snapshot.Postcards)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, "render_error", "weekly mosaic image could not be rendered")
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=\"pixelgrama-%s.png\"", snapshot.Week.Key))
		w.Header().Set("ETag", snapshot.ETag)
		w.Header().Set("Cache-Control", "public, max-age=60")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
		return
	}
	page, err := s.renderWeeklyPage(snapshot)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "render_error", "weekly mosaic page could not be rendered")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(page)
}

func (s *server) weeklySnapshot(ctx context.Context) (weeklySnapshot, error) {
	week := isoWeekFor(s.now())
	items, total, err := s.store.ListPublicBetween(ctx, week.Start, week.End, weeklyMaxPostcards)
	if err != nil {
		return weeklySnapshot{}, err
	}
	digest := sha256.New()
	_, _ = fmt.Fprintf(digest, "%s:%d:", week.Key, total)
	for _, item := range items {
		_, _ = fmt.Fprintf(digest, "%d:%s;", item.ID, item.ContentHash)
	}
	return weeklySnapshot{
		Week:      week,
		Postcards: items,
		Total:     total,
		ETag:      `"` + hex.EncodeToString(digest.Sum(nil)) + `"`,
	}, nil
}

func (s *server) renderWeeklyPage(snapshot weeklySnapshot) ([]byte, error) {
	pageTemplate, err := webFiles.ReadFile("web/weekly.html")
	if err != nil {
		return nil, fmt.Errorf("read embedded weekly page: %w", err)
	}
	style, err := webFiles.ReadFile("web/style.css")
	if err != nil {
		return nil, fmt.Errorf("read embedded style: %w", err)
	}
	parsed, err := template.New("weekly").Parse(string(pageTemplate))
	if err != nil {
		return nil, fmt.Errorf("parse weekly page: %w", err)
	}
	links := make([]weeklyLink, 0, len(snapshot.Postcards))
	for index, item := range snapshot.Postcards {
		label := fmt.Sprintf("POSTAL %02d / POSTCARD %02d", index+1, index+1)
		if item.Alias != nil {
			label += " — " + *item.Alias
		}
		links = append(links, weeklyLink{ID: item.ID, Label: label})
	}
	var output bytes.Buffer
	if err := parsed.Execute(&output, weeklyPageData{
		Style:     template.CSS(style),
		WeekKey:   snapshot.Week.Key,
		StartDate: snapshot.Week.Start.Format("2006-01-02"),
		EndDate:   snapshot.Week.End.AddDate(0, 0, -1).Format("2006-01-02"),
		Selected:  len(snapshot.Postcards),
		Total:     snapshot.Total,
		Links:     links,
		Commit:    s.commit,
		RepoURL:   s.repoURL,
		PRURL:     s.prURL,
	}); err != nil {
		return nil, fmt.Errorf("render weekly page: %w", err)
	}
	return output.Bytes(), nil
}

func renderWeeklyPNG(items []store.Postcard) ([]byte, error) {
	imageValue := image.NewRGBA(image.Rect(0, 0, weeklyImageSize, weeklyImageSize))
	draw.Draw(imageValue, imageValue.Bounds(), image.NewUniform(vgaPalette[0]), image.Point{}, draw.Src)
	for postcardIndex, item := range items {
		if postcardIndex >= weeklyMaxPostcards {
			break
		}
		colors, err := paletteColors(item.PaletteID, item.PaletteVersion)
		if err != nil {
			return nil, err
		}
		tileX := (postcardIndex % weeklyColumns) * weeklyTileSize
		tileY := (postcardIndex / weeklyColumns) * weeklyTileSize
		for pixelIndex, value := range item.Pixels {
			x0 := tileX + (pixelIndex%16)*weeklyPixelScale
			y0 := tileY + (pixelIndex/16)*weeklyPixelScale
			fill := colors[value]
			for y := y0; y < y0+weeklyPixelScale; y++ {
				for x := x0; x < x0+weeklyPixelScale; x++ {
					imageValue.SetRGBA(x, y, fill)
				}
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
