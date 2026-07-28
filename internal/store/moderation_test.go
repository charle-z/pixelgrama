package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOpenMigratesVersionOnePostcardsAsVisible(t *testing.T) {
	path := filepath.Join(t.TempDir(), "version-one.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(schemaV1 + `
PRAGMA user_version = 1;
INSERT INTO postcards (pixels, alias, deployed_commit, created_at)
VALUES (zeroblob(256), 'EXISTING', 'old-commit', '2026-07-27T00:00:00Z');`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	database, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	version, err := database.SchemaVersion(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if version != CurrentSchemaVersion {
		t.Fatalf("schema version = %d, want %d", version, CurrentSchemaVersion)
	}

	public, err := database.List(context.Background(), 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(public) != 1 || public[0].Alias == nil || *public[0].Alias != "EXISTING" {
		t.Fatalf("existing postcard was not kept visible: %#v", public)
	}

	admin, err := database.ListModeration(context.Background(), ModerationAll, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(admin) != 1 || admin[0].Status != ModerationVisible {
		t.Fatalf("migrated moderation state = %#v", admin)
	}
	if admin[0].ModeratedAt != nil || admin[0].Reason != nil {
		t.Fatalf("existing postcard gained moderation metadata: %#v", admin[0])
	}
}

func TestHideExcludesPostcardFromPublicListAndDeduplication(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "hide.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	ctx := context.Background()
	now := time.Date(2026, 7, 28, 1, 2, 3, 0, time.UTC)
	first, err := database.Insert(ctx, testPixels(t, 0), nil, "first", now.Add(-2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	second, err := database.Insert(ctx, testPixels(t, 1), nil, "second", now.Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}

	hidden, err := database.Hide(ctx, second.ID, "duplicate or abusive content", now)
	if err != nil {
		t.Fatal(err)
	}
	if hidden.Status != ModerationHidden || hidden.Reason == nil || *hidden.Reason != "duplicate or abusive content" {
		t.Fatalf("hidden record = %#v", hidden)
	}
	if hidden.ModeratedAt == nil || !hidden.ModeratedAt.Equal(now) {
		t.Fatalf("hidden timestamp = %#v", hidden.ModeratedAt)
	}

	public, err := database.List(ctx, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(public) != 1 || public[0].ID != first.ID {
		t.Fatalf("public list contains hidden postcard: %#v", public)
	}

	republished, err := database.Insert(ctx, testPixels(t, 1), nil, "republished", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("hidden postcard still affected public deduplication: %v", err)
	}
	if republished.ID == second.ID {
		t.Fatal("republished postcard reused hidden row")
	}

	hiddenList, err := database.ListModeration(ctx, ModerationHidden, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hiddenList) != 1 || hiddenList[0].ID != second.ID {
		t.Fatalf("hidden moderation list = %#v", hiddenList)
	}
}

func TestRestoreMakesPostcardPublicAndRecordsActions(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "restore.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	ctx := context.Background()
	created := time.Date(2026, 7, 28, 2, 0, 0, 0, time.UTC)
	item, err := database.Insert(ctx, testPixels(t, 4), nil, "commit", created)
	if err != nil {
		t.Fatal(err)
	}
	hiddenAt := created.Add(time.Minute)
	if _, err := database.Hide(ctx, item.ID, "manual review", hiddenAt); err != nil {
		t.Fatal(err)
	}
	restoredAt := hiddenAt.Add(time.Minute)
	restored, err := database.Restore(ctx, item.ID, "review completed", restoredAt)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Status != ModerationVisible || restored.Reason == nil || *restored.Reason != "review completed" {
		t.Fatalf("restored record = %#v", restored)
	}
	if restored.ModeratedAt == nil || !restored.ModeratedAt.Equal(restoredAt) {
		t.Fatalf("restored timestamp = %#v", restored.ModeratedAt)
	}

	public, err := database.List(ctx, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(public) != 1 || public[0].ID != item.ID {
		t.Fatalf("restored postcard is not public: %#v", public)
	}

	rows, err := database.db.QueryContext(ctx,
		"SELECT action, reason, created_at FROM moderation_events WHERE postcard_id = ? ORDER BY id",
		item.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var actions []string
	var reasons []string
	for rows.Next() {
		var action, reason, at string
		if err := rows.Scan(&action, &reason, &at); err != nil {
			t.Fatal(err)
		}
		actions = append(actions, action)
		reasons = append(reasons, reason)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(actions) != 2 || actions[0] != "hide" || actions[1] != "restore" {
		t.Fatalf("moderation actions = %#v", actions)
	}
	if reasons[0] != "manual review" || reasons[1] != "review completed" {
		t.Fatalf("moderation reasons = %#v", reasons)
	}
}

func TestBackupPreservesModerationStateAndEvents(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "moderated.db")
	backupPath := filepath.Join(dir, "moderated-backup.db")
	database, err := Open(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	item, err := database.Insert(ctx, testPixels(t, 6), nil, "backup-moderation", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Hide(ctx, item.ID, "backup policy review", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := database.Backup(ctx, backupPath); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	restored, err := Open(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	public, err := restored.List(ctx, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(public) != 0 {
		t.Fatalf("backup made hidden postcard public: %#v", public)
	}
	hidden, err := restored.ListModeration(ctx, ModerationHidden, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hidden) != 1 || hidden[0].ID != item.ID || hidden[0].Reason == nil || *hidden[0].Reason != "backup policy review" {
		t.Fatalf("backup moderation state = %#v", hidden)
	}
	var events int
	if err := restored.db.QueryRowContext(ctx, "SELECT count(*) FROM moderation_events WHERE postcard_id = ?", item.ID).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 1 {
		t.Fatalf("backup moderation event count = %d, want 1", events)
	}
}

func TestModerationRejectsInvalidTransitionsAndReasons(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "invalid.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	ctx := context.Background()
	item, err := database.Insert(ctx, testPixels(t, 7), nil, "commit", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}

	if _, err := database.Hide(ctx, item.ID, "   ", time.Now().UTC()); !errors.Is(err, ErrInvalidModerationReason) {
		t.Fatalf("empty reason error = %v", err)
	}
	for _, reason := range []string{strings.Repeat("x", MaxModerationReasonRunes+1), "line\nbreak"} {
		if _, err := database.Hide(ctx, item.ID, reason, time.Now().UTC()); !errors.Is(err, ErrInvalidModerationReason) {
			t.Fatalf("invalid reason %q error = %v", reason, err)
		}
	}
	if _, err := database.Restore(ctx, item.ID, "not hidden", time.Now().UTC()); !errors.Is(err, ErrModerationState) {
		t.Fatalf("restore visible error = %v", err)
	}
	if _, err := database.Hide(ctx, 999999, "missing", time.Now().UTC()); !errors.Is(err, ErrPostcardNotFound) {
		t.Fatalf("hide missing error = %v", err)
	}
	if _, err := database.Hide(ctx, item.ID, "first hide", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Hide(ctx, item.ID, "second hide", time.Now().UTC()); !errors.Is(err, ErrModerationState) {
		t.Fatalf("hide hidden error = %v", err)
	}
}
