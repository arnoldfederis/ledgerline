package database

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

// Embedding the migrations means the binary carries its own schema; a
// container image needs nothing else.
//
//go:embed migrations/*.sql
var migrationsFS embed.FS

// Keep goose's bookkeeping in the application schema rather than public.
const gooseVersionTable = "app.goose_db_version"

// Migrate applies every pending migration. A session lock makes it safe for
// several instances to start at once during a rolling deploy.
func Migrate(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger) error {
	provider, cleanup, err := newProvider(pool, true)
	if err != nil {
		return err
	}
	defer cleanup()

	results, err := provider.Up(ctx)
	if err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}

	if len(results) == 0 {
		version, err := provider.GetDBVersion(ctx)
		if err != nil {
			return fmt.Errorf("read schema version: %w", err)
		}
		log.Info("database schema already up to date", "version", version)
		return nil
	}

	for _, r := range results {
		log.Info("migration applied",
			"version", r.Source.Version,
			"source", r.Source.Path,
			"duration", r.Duration)
	}
	return nil
}

// Rollback reverts the most recently applied migration.
func Rollback(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger) error {
	provider, cleanup, err := newProvider(pool, false)
	if err != nil {
		return err
	}
	defer cleanup()

	result, err := provider.Down(ctx)
	if err != nil {
		return fmt.Errorf("roll back migration: %w", err)
	}

	log.Info("migration rolled back", "version", result.Source.Version, "source", result.Source.Path)
	return nil
}

// Version reports the schema version recorded in the database.
func Version(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	provider, cleanup, err := newProvider(pool, false)
	if err != nil {
		return 0, err
	}
	defer cleanup()

	return provider.GetDBVersion(ctx)
}

// newProvider borrows a database/sql view of the pgx pool, since that is what
// goose speaks, and returns it alongside its cleanup.
func newProvider(pool *pgxpool.Pool, locked bool) (*goose.Provider, func(), error) {
	root, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		return nil, nil, fmt.Errorf("open embedded migrations: %w", err)
	}

	db := stdlib.OpenDBFromPool(pool)
	cleanup := func() { _ = db.Close() }

	opts := []goose.ProviderOption{goose.WithTableName(gooseVersionTable)}
	if locked {
		locker, err := lock.NewPostgresSessionLocker()
		if err != nil {
			cleanup()
			return nil, nil, fmt.Errorf("create migration locker: %w", err)
		}
		opts = append(opts, goose.WithSessionLocker(locker))
	}

	provider, err := goose.NewProvider(goose.DialectPostgres, db, root, opts...)
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("create migration provider: %w", err)
	}
	return provider, cleanup, nil
}
