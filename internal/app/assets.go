package app

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"fmt"
	"html/template"
)

//go:embed web/index.html web/postcard.html web/weekly.html web/style.css web/editor.js web/app.js
var webFiles embed.FS

type pageData struct {
	Style   template.CSS
	Script  template.JS
	Commit  string
	RepoURL string
	PRURL   string
}

func buildPage(commit, repoURL, prURL string) ([]byte, string, error) {
	index, err := webFiles.ReadFile("web/index.html")
	if err != nil {
		return nil, "", fmt.Errorf("read embedded index: %w", err)
	}
	style, err := webFiles.ReadFile("web/style.css")
	if err != nil {
		return nil, "", fmt.Errorf("read embedded style: %w", err)
	}
	editorScript, err := webFiles.ReadFile("web/editor.js")
	if err != nil {
		return nil, "", fmt.Errorf("read embedded editor script: %w", err)
	}
	appScript, err := webFiles.ReadFile("web/app.js")
	if err != nil {
		return nil, "", fmt.Errorf("read embedded application script: %w", err)
	}
	script := append(append(append([]byte(nil), editorScript...), '\n'), appScript...)

	pageTemplate, err := template.New("wall").Parse(string(index))
	if err != nil {
		return nil, "", fmt.Errorf("parse embedded page: %w", err)
	}
	var page bytes.Buffer
	if err := pageTemplate.Execute(&page, pageData{
		Style:   template.CSS(style),
		Script:  template.JS(script),
		Commit:  commit,
		RepoURL: repoURL,
		PRURL:   prURL,
	}); err != nil {
		return nil, "", fmt.Errorf("render embedded page: %w", err)
	}

	styleDigest := sha256.Sum256(style)
	scriptDigest := sha256.Sum256(script)
	styleHash := base64.StdEncoding.EncodeToString(styleDigest[:])
	scriptHash := base64.StdEncoding.EncodeToString(scriptDigest[:])
	csp := fmt.Sprintf("default-src 'none'; connect-src 'self'; script-src 'sha256-%s'; style-src 'sha256-%s'; img-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'; object-src 'none'", scriptHash, styleHash)
	return page.Bytes(), csp, nil
}
