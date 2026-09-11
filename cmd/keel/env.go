package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/Keel-Official/keel-backend/internal/store"
)

// Every environment variable this binary reads is named here, so the answer to
// "what can be set without editing code" is one file rather than a grep. The
// prefix is KEEL_ throughout, matching KEEL_DSN, and NOT DATABASE_: two
// prefixes for the same program is how a variable ends up set in .env, spelled
// correctly, and read by nobody.
const (
	envDSN = "KEEL_DSN"

	envDBMaxOpenConns    = "KEEL_DB_MAX_OPEN_CONNS"
	envDBMaxIdleConns    = "KEEL_DB_MAX_IDLE_CONNS"
	envDBConnMaxLifetime = "KEEL_DB_CONN_MAX_LIFETIME"
	envDBConnMaxIdleTime = "KEEL_DB_CONN_MAX_IDLE_TIME"
	envDBPingTimeout     = "KEEL_DB_PING_TIMEOUT"
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// envIntOr and envDurationOr REFUSE a value they cannot parse rather than
// falling back to the default. A typo in a pool size is silent for the life of
// the process otherwise, and the symptom would be a pool that is the wrong size
// with a .env file that says it is the right one.
func envIntOr(key string, fallback int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s=%q is not a whole number", key, v)
	}
	if n <= 0 {
		return 0, fmt.Errorf("%s=%d must be greater than zero", key, n)
	}
	return n, nil
}

func envDurationOr(key string, fallback time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s=%q is not a duration, write it like 30m or 5s", key, v)
	}
	if d <= 0 {
		return 0, fmt.Errorf("%s=%s must be greater than zero", key, d)
	}
	return d, nil
}

// storeConfigFromEnv builds the pool settings every command opens its Store
// with. Reading them here rather than in internal/store keeps that package
// free of any knowledge of how it is deployed, which is the same reason the DSN
// arrives as a flag defaulted from the environment instead of being read
// inside Open.
//
// These are environment-only, with no flag beside them, on purpose: they are
// set once per host in .env and never per invocation, unlike -dsn, which a
// developer really does retype.
func storeConfigFromEnv() (store.Config, error) {
	d := store.DefaultConfig()
	cfg := store.Config{}
	var err error

	if cfg.MaxOpenConns, err = envIntOr(envDBMaxOpenConns, d.MaxOpenConns); err != nil {
		return store.Config{}, err
	}
	if cfg.MaxIdleConns, err = envIntOr(envDBMaxIdleConns, d.MaxIdleConns); err != nil {
		return store.Config{}, err
	}
	if cfg.ConnMaxLifetime, err = envDurationOr(envDBConnMaxLifetime, d.ConnMaxLifetime); err != nil {
		return store.Config{}, err
	}
	if cfg.ConnMaxIdleTime, err = envDurationOr(envDBConnMaxIdleTime, d.ConnMaxIdleTime); err != nil {
		return store.Config{}, err
	}
	if cfg.PingTimeout, err = envDurationOr(envDBPingTimeout, d.PingTimeout); err != nil {
		return store.Config{}, err
	}
	return cfg, nil
}

// openStore is the one place a command turns a DSN into a Store, so the hint
// below is written once. The most common cause on a developer machine is a DSN
// naming 5432, which is a second Postgres and not the one docker-compose
// started; on the deployed host it is a hostname the container cannot reach,
// which is what the ping deadline in store.OpenWithConfig now cuts short.
func openStore(ctx context.Context, dsn string) (*store.Store, error) {
	cfg, err := storeConfigFromEnv()
	if err != nil {
		return nil, err
	}
	s, err := store.OpenWithConfig(ctx, dsn, cfg)
	if err != nil {
		return nil, fmt.Errorf("%w\n  hint: `make up && make migrate` first. The container publishes 5433, not 5432, "+
			"so a DSN still naming 5432 reaches whatever else is on the host", err)
	}
	return s, nil
}
