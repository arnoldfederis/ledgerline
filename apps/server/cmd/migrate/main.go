// Command migrate applies the embedded goose migrations.
//
// The server can migrate itself on boot (DB_AUTO_MIGRATE), which is what you
// want on a laptop. This binary makes schema changes an explicit deploy step
// that can be run, reviewed and rolled back on its own.
//
// Usage: migrate [up|down|version]
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"ledgerline/apps/server/internal/config"
	"ledgerline/apps/server/internal/database"
)

func main() {
	command := "up"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}

	if err := run(command); err != nil {
		slog.Error("migration failed", "command", command, "error", err)
		os.Exit(1)
	}
}

func run(command string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pool, err := database.Open(ctx, cfg.DatabaseURL, log)
	if err != nil {
		return err
	}
	defer pool.Close()

	switch command {
	case "up":
		return database.Migrate(ctx, pool, log)
	case "down":
		return database.Rollback(ctx, pool, log)
	case "version":
		version, err := database.Version(ctx, pool)
		if err != nil {
			return err
		}
		fmt.Printf("schema version: %d\n", version)
		return nil
	default:
		return fmt.Errorf("unknown command %q: expected up, down or version", command)
	}
}
