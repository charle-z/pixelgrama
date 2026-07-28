package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/charle-z/pixelgrama/internal/store"
)

type moderationActionResult struct {
	ID          int64                  `json:"id"`
	Status      store.ModerationStatus `json:"status"`
	ModeratedAt time.Time              `json:"moderated_at"`
}

func runAdmin(
	ctx context.Context,
	config runtimeConfig,
	args []string,
	output io.Writer,
	audit io.Writer,
	now func() time.Time,
) error {
	if len(args) == 0 {
		return errorsf("admin requires one of: list, hide, restore")
	}
	if now == nil {
		now = time.Now
	}

	switch args[0] {
	case "list":
		return runAdminList(ctx, config, args[1:], output)
	case "hide", "restore":
		return runAdminAction(ctx, config, args[0], args[1:], output, audit, now)
	default:
		return fmt.Errorf("unknown admin command %q", args[0])
	}
}

func runAdminList(ctx context.Context, config runtimeConfig, args []string, output io.Writer) error {
	flags := flag.NewFlagSet("admin list", flag.ContinueOnError)
	flags.SetOutput(output)
	statusValue := flags.String("status", string(store.ModerationHidden), "hidden, visible, or all")
	limit := flags.Int("limit", 100, "maximum records to return")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errorsf("admin list accepts no positional arguments")
	}

	status := store.ModerationStatus(strings.ToLower(strings.TrimSpace(*statusValue)))
	if status != store.ModerationHidden && status != store.ModerationVisible && status != store.ModerationAll {
		return errorsf("admin list --status must be hidden, visible, or all")
	}
	if *limit < 1 || *limit > store.MaxModerationListLimit {
		return fmt.Errorf("admin list --limit must be between 1 and %d", store.MaxModerationListLimit)
	}

	database, err := store.Open(config.databasePath)
	if err != nil {
		return fmt.Errorf("open database for moderation list: %w", err)
	}
	defer database.Close()

	records, err := database.ListModeration(ctx, status, *limit)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(output).Encode(records); err != nil {
		return fmt.Errorf("encode moderation list: %w", err)
	}
	return nil
}

func runAdminAction(
	ctx context.Context,
	config runtimeConfig,
	action string,
	args []string,
	output io.Writer,
	audit io.Writer,
	now func() time.Time,
) error {
	flags := flag.NewFlagSet("admin "+action, flag.ContinueOnError)
	flags.SetOutput(output)
	id := flags.Int64("id", 0, "postcard id")
	reason := flags.String("reason", "", "administrative reason")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("admin %s accepts no positional arguments", action)
	}
	if *id < 1 {
		return fmt.Errorf("admin %s requires a positive --id", action)
	}
	if strings.TrimSpace(*reason) == "" {
		return fmt.Errorf("admin %s requires --reason", action)
	}

	database, err := store.Open(config.databasePath)
	if err != nil {
		return fmt.Errorf("open database for moderation action: %w", err)
	}
	defer database.Close()

	at := now().UTC()
	var record store.ModerationRecord
	switch action {
	case "hide":
		record, err = database.Hide(ctx, *id, *reason, at)
	case "restore":
		record, err = database.Restore(ctx, *id, *reason, at)
	default:
		return fmt.Errorf("unknown moderation action %q", action)
	}
	if err != nil {
		return err
	}

	result := moderationActionResult{ID: record.ID, Status: record.Status, ModeratedAt: at}
	if err := json.NewEncoder(output).Encode(result); err != nil {
		return fmt.Errorf("encode moderation action: %w", err)
	}
	_, _ = fmt.Fprintf(
		audit,
		"moderation action=%s postcard_id=%d status=%s at=%s\n",
		action,
		record.ID,
		record.Status,
		at.Format(time.RFC3339Nano),
	)
	return nil
}
