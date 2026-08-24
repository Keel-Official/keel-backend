// The `serve` subcommand: the read-only API.
//
// It reads results out of Postgres and never touches Horizon, which is rule 1 of
// internal/api's brief. One popular asset triggering a Horizon request per call
// would burn the rate limit budget in minutes.
//
// WHAT IT WILL SERVE TODAY. The endpoints and the shapes are complete, and the
// metrics table is empty until `scan` can produce a result, which is blocked on
// the red zone. So GET /v1/health answers `degraded` with no scan recorded, the
// asset list answers with the demonstration set and an empty items array, and a
// per-asset request answers 404 with ASSET_NOT_MONITORED or "no metrics yet".
// Every one of those is the contract's own answer for that state rather than a
// placeholder, which is what makes this worth running before the engine exists:
// the frontend can build against it now.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Keel-Official/keel-backend/internal/api"
	"github.com/Keel-Official/keel-backend/internal/domain"
	"github.com/Keel-Official/keel-backend/internal/store"
)

func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	addr := fs.String("addr", ":3000", "address to listen on")
	dsn := fs.String("dsn", envOr("KEEL_DSN", store.DefaultDSN), "Postgres DSN, or set KEEL_DSN")
	// Off by default and named after what it actually gates. DEC-002 defers the
	// Hubble path, so with no historical source the honest answer to a ledger
	// query is 503 HISTORICAL_UNAVAILABLE rather than a live figure wearing a
	// historical label.
	historical := fs.Bool("historical", false, "declare the historical replay path available")

	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `keel serve - the read-only API

  GET /v1/health
  GET /v1/methodology
  GET /v1/assets            ?band= &hasFlag= &limit= &offset=
  GET /v1/asset/{id}/depth  ?quote= &ledger=
  GET /v1/asset/{id}/history ?from= &to= &resolution=

The contract is docs/api/keel-openapi.yaml and the mock responses the frontend was
given are in docs/api/mocks/. Nothing here writes: not to Postgres, and not to the
Stellar network.

`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	logger := log.New(os.Stderr, "serve ", log.LstdFlags|log.LUTC)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	s, err := store.Open(ctx, *dsn)
	if err != nil {
		return fmt.Errorf("%w\n  hint: `make up && make migrate` first, and check that port 5432 is the container's "+
			"Postgres and not another one already running locally", err)
	}
	defer s.Close()

	applied, err := s.SchemaVersion(ctx)
	if err != nil {
		return fmt.Errorf("serve: reading schema_migrations: %w\n  hint: run make migrate", err)
	}
	if len(applied) == 0 {
		return errors.New("serve: no migrations are applied; run make migrate")
	}
	logger.Printf("schema at %s", applied[0])

	srv, err := api.New(api.Config{
		Reader: s,
		// The thresholds the API reports come from the same defaults the engine
		// computes with, so a response cannot describe parameters that were not
		// used.
		Params:              domain.DefaultParams(),
		HistoricalAvailable: *historical,
		Logf:                logger.Printf,
	})
	if err != nil {
		return fmt.Errorf("serve: %w", err)
	}

	logger.Printf("listening on %s, methodology %s, historical=%t",
		*addr, domain.MethodologyVersion, *historical)
	logger.Print("Ctrl-C to stop")

	if err := srv.Serve(ctx, *addr); err != nil {
		return fmt.Errorf("serve: %w", err)
	}
	logger.Print("stopped")
	return nil
}
