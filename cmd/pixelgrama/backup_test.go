package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charle-z/pixelgrama/internal/store"
)

func TestRunBackupCreatesVerifiedCopy(t *testing.T) {
	dir := t.TempDir()
	databasePath := filepath.Join(dir, "source.db")
	destination := filepath.Join(dir, "backup.db")
	database, err := store.Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	config := runtimeConfig{databasePath: databasePath}
	if err := runBackup(context.Background(), config, []string{"--output", destination}, &output); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(destination); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "backup verified") {
		t.Fatalf("output = %q", output.String())
	}
}
