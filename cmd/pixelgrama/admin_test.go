package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charle-z/pixelgrama/internal/core"
	"github.com/charle-z/pixelgrama/internal/store"
)

func TestRunAdminListHideAndRestore(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "admin.db")
	database, err := store.Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	values := make([]int, core.PixelCount)
	pixels, err := core.FromInts(values)
	if err != nil {
		t.Fatal(err)
	}
	created := time.Date(2026, 7, 28, 3, 0, 0, 0, time.UTC)
	item, err := database.Insert(context.Background(), pixels, nil, "admin-test", created)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	config := runtimeConfig{databasePath: databasePath}
	now := time.Date(2026, 7, 28, 3, 5, 0, 0, time.UTC)
	var output bytes.Buffer
	var audit bytes.Buffer
	hideReason := "manual policy review"
	if err := runAdmin(
		context.Background(),
		config,
		[]string{"hide", "--id", fmt.Sprint(item.ID), "--reason", hideReason},
		&output,
		&audit,
		func() time.Time { return now },
	); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(audit.String(), hideReason) {
		t.Fatalf("audit log leaked moderation reason: %q", audit.String())
	}
	for _, expected := range []string{"action=hide", fmt.Sprintf("postcard_id=%d", item.ID), "status=hidden", now.Format(time.RFC3339Nano)} {
		if !strings.Contains(audit.String(), expected) {
			t.Fatalf("audit log %q does not contain %q", audit.String(), expected)
		}
	}

	output.Reset()
	audit.Reset()
	if err := runAdmin(
		context.Background(),
		config,
		[]string{"list", "--status", "hidden", "--limit", "10"},
		&output,
		&audit,
		func() time.Time { return now },
	); err != nil {
		t.Fatal(err)
	}
	var hidden []store.ModerationRecord
	if err := json.Unmarshal(output.Bytes(), &hidden); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "\"pixels\"") {
		t.Fatalf("administrative list exposed pixel payloads: %s", output.String())
	}
	if len(hidden) != 1 || hidden[0].ID != item.ID || hidden[0].Status != store.ModerationHidden {
		t.Fatalf("hidden list = %#v", hidden)
	}
	if hidden[0].Reason == nil || *hidden[0].Reason != hideReason {
		t.Fatalf("hidden reason = %#v", hidden[0].Reason)
	}

	output.Reset()
	audit.Reset()
	restoreReason := "review completed"
	restoredAt := now.Add(time.Minute)
	if err := runAdmin(
		context.Background(),
		config,
		[]string{"restore", "--id", fmt.Sprint(item.ID), "--reason", restoreReason},
		&output,
		&audit,
		func() time.Time { return restoredAt },
	); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(audit.String(), restoreReason) {
		t.Fatalf("audit log leaked restore reason: %q", audit.String())
	}
	if !strings.Contains(audit.String(), "action=restore") || !strings.Contains(audit.String(), "status=visible") {
		t.Fatalf("restore audit log = %q", audit.String())
	}

	database, err = store.Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	public, err := database.List(context.Background(), 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(public) != 1 || public[0].ID != item.ID {
		t.Fatalf("restored public list = %#v", public)
	}
}

func TestRunAdminRejectsInvalidCommands(t *testing.T) {
	config := runtimeConfig{databasePath: filepath.Join(t.TempDir(), "admin-invalid.db")}
	for _, test := range []struct {
		name string
		args []string
	}{
		{name: "missing subcommand"},
		{name: "unknown subcommand", args: []string{"delete"}},
		{name: "missing hide id", args: []string{"hide", "--reason", "reason"}},
		{name: "missing hide reason", args: []string{"hide", "--id", "1"}},
		{name: "invalid list status", args: []string{"list", "--status", "deleted"}},
		{name: "invalid list limit", args: []string{"list", "--limit", "0"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			var audit bytes.Buffer
			if err := runAdmin(context.Background(), config, test.args, &output, &audit, time.Now); err == nil {
				t.Fatal("expected administrative command error")
			}
		})
	}
}
