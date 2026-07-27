package main

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/charle-z/pixelgrama/internal/store"
)

func runBackup(ctx context.Context, config runtimeConfig, args []string, output io.Writer) error {
	flags := flag.NewFlagSet("backup", flag.ContinueOnError)
	flags.SetOutput(output)
	destination := flags.String("output", "", "destination SQLite backup file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *destination == "" {
		return errorsf("backup requires --output")
	}
	if flags.NArg() != 0 {
		return errorsf("backup accepts no positional arguments")
	}
	database, err := store.Open(config.databasePath)
	if err != nil {
		return fmt.Errorf("open database for backup: %w", err)
	}
	defer database.Close()
	if err := database.Backup(ctx, *destination); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(output, "backup verified: %s\n", *destination)
	return nil
}
