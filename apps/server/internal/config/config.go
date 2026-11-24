// Package config loads every runtime setting from the environment at startup.
// Nothing deeper in the program reads os.Getenv.
package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Env         string
	Addr        string
	DatabaseURL string
	// AutoMigrate runs pending migrations during startup. Convenient locally;
	// deployments should use cmd/migrate instead.
	AutoMigrate bool
	CORSOrigins []string
	Source      SourceConfig
	Sync        SyncConfig
}

type SourceConfig struct {
	// Name is stored on every transaction and sync run, so two sources can
	// share these tables without their cursors interfering.
	Name     string
	URL      string
	PageSize int
	Timeout  time.Duration
}

type SyncConfig struct {
	Enabled      bool
	RunOnStartup bool
	Interval     time.Duration
	// Overlap rewinds the cursor on every run to absorb clock skew and records
	// committed just after their updated_at was stamped.
	Overlap time.Duration
}

func (c Config) IsDevelopment() bool { return c.Env == "development" }

// DefaultEnvFiles are searched nearest first: the app's own, then the repo root's.
var DefaultEnvFiles = []string{".env", "../../.env"}

// Load resolves configuration from the environment. Defaults are tuned for
// local development so a fresh checkout runs with no configuration at all.
func Load() (Config, error) {
	LoadEnvFiles(DefaultEnvFiles...)

	cfg := Config{
		Env:         env("APP_ENV", "development"),
		Addr:        env("SERVER_ADDR", ":8080"),
		DatabaseURL: env("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/ledgerline?sslmode=disable&search_path=app"),
		CORSOrigins: envList("CORS_ALLOWED_ORIGINS", "https://app.ledgerline.dev,http://localhost:5173"),
		Source: SourceConfig{
			Name: env("ACCOUNTING_SOURCE_NAME", "mock_accounting"),
			URL:  env("ACCOUNTING_SOURCE_URL", "http://localhost:8080/mock/accounting/transactions"),
		},
	}

	var err error
	if cfg.AutoMigrate, err = envBool("DB_AUTO_MIGRATE", true); err != nil {
		return Config{}, err
	}
	if cfg.Source.PageSize, err = envInt("ACCOUNTING_SOURCE_PAGE_SIZE", 50); err != nil {
		return Config{}, err
	}
	if cfg.Source.Timeout, err = envDuration("ACCOUNTING_SOURCE_TIMEOUT", 10*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.Sync.Enabled, err = envBool("SYNC_ENABLED", true); err != nil {
		return Config{}, err
	}
	if cfg.Sync.RunOnStartup, err = envBool("SYNC_ON_STARTUP", true); err != nil {
		return Config{}, err
	}
	if cfg.Sync.Interval, err = envDuration("SYNC_INTERVAL", time.Minute); err != nil {
		return Config{}, err
	}
	if cfg.Sync.Overlap, err = envDuration("SYNC_OVERLAP", 5*time.Minute); err != nil {
		return Config{}, err
	}

	return cfg, cfg.validate()
}

func (c Config) validate() error {
	switch {
	case c.DatabaseURL == "":
		return fmt.Errorf("config: DATABASE_URL is required")
	case c.Source.Name == "":
		return fmt.Errorf("config: ACCOUNTING_SOURCE_NAME must not be empty")
	case c.Source.PageSize < 1:
		return fmt.Errorf("config: ACCOUNTING_SOURCE_PAGE_SIZE must be at least 1, got %d", c.Source.PageSize)
	case c.Sync.Interval <= 0:
		return fmt.Errorf("config: SYNC_INTERVAL must be positive, got %s", c.Sync.Interval)
	case c.Sync.Overlap < 0:
		return fmt.Errorf("config: SYNC_OVERLAP must not be negative, got %s", c.Sync.Overlap)
	}
	return nil
}

// LoadEnvFiles fills in variables from .env files without overriding anything
// already set, so a real environment variable always wins.
func LoadEnvFiles(paths ...string) {
	for _, path := range paths {
		f, err := os.Open(path)
		if err != nil {
			continue
		}

		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			key, value, found := strings.Cut(line, "=")
			if !found {
				continue
			}
			key = strings.TrimSpace(key)
			value = strings.Trim(strings.TrimSpace(value), `"'`)
			if _, exists := os.LookupEnv(key); !exists {
				_ = os.Setenv(key, value)
			}
		}
		_ = f.Close()
	}
}

func env(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func envList(key, fallback string) []string {
	var out []string
	for _, part := range strings.Split(env(key, fallback), ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func envBool(key string, fallback bool) (bool, error) {
	raw, ok := os.LookupEnv(key)
	if !ok || raw == "" {
		return fallback, nil
	}

	v, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("config: %s must be a boolean, got %q", key, raw)
	}
	return v, nil
}

func envInt(key string, fallback int) (int, error) {
	raw, ok := os.LookupEnv(key)
	if !ok || raw == "" {
		return fallback, nil
	}

	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("config: %s must be an integer, got %q", key, raw)
	}
	return v, nil
}

func envDuration(key string, fallback time.Duration) (time.Duration, error) {
	raw, ok := os.LookupEnv(key)
	if !ok || raw == "" {
		return fallback, nil
	}

	v, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("config: %s must be a duration such as 30s or 5m, got %q", key, raw)
	}
	return v, nil
}
