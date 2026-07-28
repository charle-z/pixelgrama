package app

import (
	"bytes"
	"fmt"
	"html/template"

	"github.com/charle-z/pixelgrama/internal/store"
)

type sharePageData struct {
	Style          template.CSS
	ID             int64
	Alias          string
	CreatedAt      string
	ContentHash    string
	PaletteID      string
	PaletteVersion int
	PNGURL         string
	JSONURL        string
	RemixURL       string
	ParentURL      string
	HasParent      bool
}

func renderSharePage(item store.Postcard) ([]byte, error) {
	index, err := webFiles.ReadFile("web/postcard.html")
	if err != nil {
		return nil, fmt.Errorf("read embedded postcard page: %w", err)
	}
	style, err := webFiles.ReadFile("web/style.css")
	if err != nil {
		return nil, fmt.Errorf("read embedded style: %w", err)
	}
	pageTemplate, err := template.New("postcard").Parse(string(index))
	if err != nil {
		return nil, fmt.Errorf("parse postcard page: %w", err)
	}
	alias := "ANON"
	if item.Alias != nil && *item.Alias != "" {
		alias = *item.Alias
	}
	data := sharePageData{
		Style:          template.CSS(style),
		ID:             item.ID,
		Alias:          alias,
		CreatedAt:      item.CreatedAt.UTC().Format("2006-01-02 15:04 UTC"),
		ContentHash:    item.ContentHash,
		PaletteID:      item.PaletteID,
		PaletteVersion: item.PaletteVersion,
		PNGURL:         fmt.Sprintf("/p/%d.png", item.ID),
		JSONURL:        fmt.Sprintf("/p/%d.json", item.ID),
		RemixURL:       fmt.Sprintf("/wall?remix=%d", item.ID),
	}
	if item.ParentID != nil {
		data.HasParent = true
		data.ParentURL = fmt.Sprintf("/p/%d", *item.ParentID)
	}
	var page bytes.Buffer
	if err := pageTemplate.Execute(&page, data); err != nil {
		return nil, fmt.Errorf("render postcard page: %w", err)
	}
	return page.Bytes(), nil
}
