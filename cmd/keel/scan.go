// The `scan` subcommand: compute metrics for every active asset and store them.
//
// It is the only command that joins all three layers, and it owns none of them:
// internal/horizon reads the market, internal/domain computes, internal/store
// writes. Nothing here decides anything about the methodology, which is what the
// zone map means by an entrypoint with no methodology in it. If a formula ever
// appears in this file it is in the wrong file.
//
// WHY IT IS WRITTEN WHILE THE RED ZONE IS EMPTY. Every function in
// internal/domain/compute.go panics, so a scan today fetches a real snapshot,
// verifies real assets, opens a real run row, and has nothing to store. That is
// worth having anyway: the wiring is the part that can be wrong in ways a test of
// the formula never catches, and the day compute.go has a body this command works
// without being touched. The cost of the opposite order is a scan written in a
// hurry against a formula that already exists, which is how a wiring bug gets
// blamed on the methodology.
//
// THREE DECISIONS THIS FILE MAKES.
//
//  1. A PANIC IN THE DOMAIN IS ONE ASSET FAILING, NOT THE SCAN DYING. runs.go says
//     it in its own header: one asset failing must not fail a whole scan, which is
//     the reason the runs table exists at all. A panic is the strongest form of one
//     asset failing, so it is recovered, counted, and recorded like any other
//     failure. Without that, an unwritten compute.go kills the process mid-round
//     and leaves the run row open, and once compute.go IS written, one bad snapshot
//     out of fifty throws away the other forty-nine results.
//
//  2. A ROUND THAT PANICKED ON EVERY ASSET STOPS THE COMMAND, with exit code 3
//     rather than 1. Three means "not built yet" everywhere else in this binary and
//     it means the same here. Looping every fifteen minutes against a Horizon
//     budget to store nothing is not honest work, and exiting 1 would tell a
//     scheduler the scan is broken when what is true is that it has nothing to
//     compute with.
//
//  3. ASSET IDENTITY IS VERIFIED ONCE AT STARTUP, NOT PER ROUND. Trap 4 in
//     internal/horizon/CLAUDE.md: naming the wrong asset type returns an EMPTY
//     order book and no error. An empty book is not an error condition here, it is
//     this product's most interesting finding, so a wrong type would be stored as a
//     real ZERO_DEPTH result and read as a discovery. Verification cannot be
//     skipped and it cannot be paid for every round either, since the answer cannot
//     change between rounds.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"

	"github.com/Keel-Official/keel-backend/internal/domain"
	"github.com/Keel-Official/keel-backend/internal/horizon"
	"github.com/Keel-Official/keel-backend/internal/store"
)

// errComputeNotBuilt is returned when every asset in a round panicked, which is
// what an empty internal/domain/compute.go looks like from out here. main.go turns
// it into exit code 3, the same code every other unbuilt subcommand uses.
//
// It is matched on the ERROR and not on the panic text, because "not implemented"
// is a string in a file this side may not read for meaning. A round where every
// single asset panicked is the observable fact, and it is the same fact whether
// the panic says "not implemented" or something worse.
var errComputeNotBuilt = errors.New("every asset panicked, so there is nothing to store")

func runScan(args []string) error {
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	dsn := fs.String("dsn", envOr("KEEL_DSN", store.DefaultDSN), "Postgres DSN, or set KEEL_DSN")
	baseURL := fs.String("horizon", horizon.DefaultBaseURL, "Horizon base URL")
	interval := fs.Duration("interval", 15*time.Minute, "how often to scan")
	once := fs.Bool("once", false, "scan one round and exit")
	budget := fs.Int("budget", 3000, "requests permitted per hour. Public Horizon allows about 3600 per IP")
	bidUnit := fs.String("bid-amount-unit", string(horizon.BidAmountUnitQuote),
		"which asset an order book bid amount is denominated in: quote or base. See BidAmountUnit in internal/horizon")
	verify := fs.Bool("verify", true, "verify every asset's code, issuer and type on Horizon before the first round")

	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `keel scan - compute metrics for every active asset and store them

The demonstration set comes from the assets table, not from a pair file: declare it
with "keel assets -pairs <file>" first. Three Horizon requests per asset per round,
which is the budget line in section 6.4 of the technical design.

A result is written once per (asset, ledger, methodology version, source). Scanning
a ledger that is already stored writes NOTHING and is not an error, so a re-run
after a crash is safe and a differing result is a finding rather than an overwrite.

Today every asset fails, because every function in internal/domain/compute.go
panics. The command says so and exits with code 3.

`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	unit := horizon.BidAmountUnit(*bidUnit)
	if unit != horizon.BidAmountUnitQuote && unit != horizon.BidAmountUnitBase {
		return fmt.Errorf("scan: -bid-amount-unit must be %q or %q",
			horizon.BidAmountUnitQuote, horizon.BidAmountUnitBase)
	}

	logger := log.New(os.Stderr, "scan ", log.LstdFlags|log.LUTC)

	// SIGINT and SIGTERM cancel the context rather than killing the process, so a
	// round in flight finishes and closes its run row. A run row left open is how
	// runs.go reports a job that died, and it should mean that rather than meaning
	// somebody pressed Ctrl-C.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	s, err := store.Open(ctx, *dsn)
	if err != nil {
		return fmt.Errorf("%w\n  hint: `make up && make migrate` first. The container publishes 5433, not 5432, "+
			"so a DSN still naming 5432 reaches whatever else is on the host", err)
	}
	defer func() { _ = s.Close() }()

	applied, err := s.SchemaVersion(ctx)
	if err != nil {
		return fmt.Errorf("scan: reading schema_migrations: %w\n  hint: run make migrate", err)
	}
	if len(applied) == 0 {
		return errors.New("scan: no migrations are applied; run make migrate")
	}

	rows, err := s.Assets(ctx, true)
	if err != nil {
		return fmt.Errorf("scan: %w", err)
	}
	if len(rows) == 0 {
		return errors.New("scan: the demonstration set is empty; declare it with `keel assets -pairs <file>`")
	}

	client := horizon.NewClient(horizon.Config{
		BaseURL:       *baseURL,
		Budget:        *budget,
		BidAmountUnit: unit,
		// No cache, for the reason the recorder gives: two rounds that are
		// identical because a body was reused say nothing about the market.
		CacheTTL: 0,
	})

	if *verify {
		if err := verifyAssets(ctx, client, rows, logger.Printf); err != nil {
			return fmt.Errorf("scan: %w", err)
		}
	}

	logger.Printf("schema at %s, %d active pair(s), methodology %s, bid amount read as %s",
		applied[0], len(rows), domain.MethodologyVersion, unit)

	if *once {
		return scanOnce(ctx, s, client, rows, logger)
	}

	logger.Printf("interval %s, Ctrl-C to stop", *interval)
	ticker := time.NewTicker(*interval)
	defer ticker.Stop()
	for {
		if err := scanOnce(ctx, s, client, rows, logger); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			logger.Print("stopped")
			return nil
		case <-ticker.C:
		}
	}
}

// scanOnce computes and stores one round, and opens a run row around it so that a
// partial failure survives the process that produced it.
func scanOnce(ctx context.Context, s *store.Store, client *horizon.Client, rows []store.Asset, logger *log.Logger) error {
	startedAt := time.Now().UTC()
	runID, err := s.StartRun(ctx, store.RunScan, startedAt)
	if err != nil {
		return fmt.Errorf("scan: %w", err)
	}

	var ok, failed, panicked, stored, alreadyThere int
	params := domain.DefaultParams()

	for _, a := range rows {
		if err := ctx.Err(); err != nil {
			break
		}

		obs, err := client.GetSnapshot(ctx, a.Base, a.Quote)
		if err != nil {
			failed++
			logger.Printf("FAIL  %s/%s: %v", a.Base, a.Quote, err)
			continue
		}

		risk, didPanic, err := computeRisk(obs.Snapshot, params)
		if err != nil {
			failed++
			if didPanic {
				panicked++
			}
			logger.Printf("FAIL  %s/%s ledger %d: %v", a.Base, a.Quote, obs.Snapshot.LedgerSeq, err)
			continue
		}

		// computedAt is this side's clock rather than the store's, because the
		// store takes it as an argument for exactly that reason: the moment a
		// result was computed belongs to whoever computed it.
		id, inserted, err := s.SaveMetrics(ctx, a.ID, time.Now().UTC(), risk)
		if err != nil {
			failed++
			logger.Printf("FAIL  %s/%s: storing: %v", a.Base, a.Quote, err)
			continue
		}
		ok++
		if inserted {
			stored++
			logger.Printf("store %s/%s ledger %d band=%s -> metrics id=%d",
				a.Base, a.Quote, risk.LedgerSeq, risk.Band, id)
		} else {
			alreadyThere++
			logger.Printf("skip  %s/%s ledger %d, already stored", a.Base, a.Quote, risk.LedgerSeq)
		}
	}

	notes := ""
	if panicked > 0 {
		notes = fmt.Sprintf("%d of %d asset(s) panicked inside internal/domain", panicked, len(rows))
	}
	if err := s.FinishRun(ctx, runID, time.Now().UTC(), ok, failed, notes); err != nil {
		return fmt.Errorf("scan: %w", err)
	}
	logger.Printf("round: %d ok (%d written, %d already stored), %d failed, %d requests this window",
		ok, stored, alreadyThere, failed, client.Requests())

	// Every asset panicking is not a scan that failed, it is a scan with nothing to
	// compute with. Reported as itself so a scheduler is not told the wrong thing.
	if panicked > 0 && panicked == failed && ok == 0 {
		return fmt.Errorf("%w: internal/domain/compute.go has no body yet, which is Al's to write", errComputeNotBuilt)
	}
	return nil
}

// computeRisk calls the pure computation and converts a panic into an error.
//
// Recovering is a deliberate exception rather than a habit, and it is confined to
// this one call: the panic being caught here comes from a package whose functions
// are declared and unwritten, and a batch job is the one place where turning a
// crash into a counted failure is the correct trade. See decision 1 in the header.
func computeRisk(s domain.Snapshot, p domain.Params) (risk domain.AssetRisk, panicked bool, err error) {
	defer func() {
		if r := recover(); r != nil {
			panicked = true
			err = fmt.Errorf("computing: panic: %v", r)
		}
	}()
	risk, err = domain.ComputeAssetRisk(s, p)
	return risk, false, err
}

// verifyAssets checks every distinct asset in the set once, in sorted order.
//
// Sorted because non-negotiable rule 2 says so and because an error that names a
// different asset on every run is harder to read than one that does not. Distinct
// because a quote asset shared by eight pairs is one asset, and paying for it eight
// times spends the budget on an answer already given.
func verifyAssets(ctx context.Context, client *horizon.Client, rows []store.Asset, logf func(string, ...any)) error {
	seen := map[string]domain.Asset{}
	for _, a := range rows {
		seen[a.Base.String()] = a.Base
		seen[a.Quote.String()] = a.Quote
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		if err := client.VerifyAsset(ctx, seen[k]); err != nil {
			return fmt.Errorf("verifying %s: %w\n  hint: the asset TYPE is the usual cause; a five character "+
				"code like USTRY is credit_alphanum12, and the wrong type returns an empty book and no error", k, err)
		}
	}
	logf("verified %d distinct asset(s) against Horizon", len(keys))
	return nil
}
