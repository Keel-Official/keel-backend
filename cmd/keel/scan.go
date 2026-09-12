// The `scan` subcommand: compute metrics for every active asset and store them.
//
// It is the only command that joins all three layers, and it owns none of them:
// internal/horizon reads the market, internal/domain computes, internal/store
// writes. Nothing here decides anything about the methodology, which is what the
// zone map means by an entrypoint with no methodology in it. If a formula ever
// appears in this file it is in the wrong file.
//
// IT WAS WRITTEN WHILE THE RED ZONE WAS EMPTY, AND THE BET PAID OFF ON
// 26 AUGUST 2026. Until that date every function in internal/domain/compute.go
// panicked, so a scan fetched a real snapshot, verified real assets, opened a real
// run row, and had nothing to store. The wiring is the part that can be wrong in
// ways a test of the formula never catches, and when compute.go finally got a body
// this command computed and stored without one line here being touched. The cost of
// the opposite order is a scan written in a hurry against a formula that already
// exists, which is how a wiring bug gets blamed on the methodology.
//
// The panic machinery below stays, and its reason changed rather than expiring.
// It used to catch a function that panicked by design; it now catches a bug in a
// formula, which is the case decision 1 was always really about.
//
// THREE DECISIONS THIS FILE MAKES.
//
//  1. A PANIC IN THE DOMAIN IS ONE ASSET FAILING, NOT THE SCAN DYING. runs.go says
//     it in its own header: one asset failing must not fail a whole scan, which is
//     the reason the runs table exists at all. A panic is the strongest form of one
//     asset failing, so it is recovered, counted, and recorded like any other
//     failure. This is now the live case rather than the future one: one bad
//     snapshot out of fifty must not throw away the other forty-nine results.
//
//  2. A ROUND THAT PANICKED ON EVERY ASSET STOPS THE COMMAND, with exit code 3
//     rather than 1. Three means "not built yet" everywhere else in this binary and
//     it means the same here. Looping every fifteen minutes against a Horizon
//     budget to store nothing is not honest work, and exiting 1 would tell a
//     scheduler the scan is broken when what is true is that it has nothing to
//     compute with. Since compute.go was written this path should never be taken,
//     and if it ever is, it means every asset in the set broke the same formula at
//     once, which is exactly the thing worth stopping for.
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
	"strings"
	"syscall"
	"time"

	"github.com/Keel-Official/keel-backend/internal/domain"
	"github.com/Keel-Official/keel-backend/internal/horizon"
	"github.com/Keel-Official/keel-backend/internal/store"
)

// errComputeNotBuilt is returned when every asset in a round panicked. That was
// what an empty internal/domain/compute.go looked like from out here until
// 26 August 2026; now it would mean a formula that breaks on every asset at once.
// main.go turns it into exit code 3, the same code every other unbuilt subcommand
// uses, and the name is kept because the exit code is what callers match on.
//
// It is matched on the ERROR and not on the panic text, because "not implemented"
// is a string in a file this side may not read for meaning. A round where every
// single asset panicked is the observable fact, and it is the same fact whether
// the panic says "not implemented" or something worse.
var errComputeNotBuilt = errors.New("every asset panicked, so there is nothing to store")

func runScan(args []string) error {
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	dsn := fs.String("dsn", envOr(envDSN, store.DefaultDSN), "Postgres DSN, or set KEEL_DSN")
	baseURL := fs.String("horizon", horizon.DefaultBaseURL, "Horizon base URL")
	interval := fs.Duration("interval", 15*time.Minute, "how often to scan")
	once := fs.Bool("once", false, "scan one round and exit")
	budget := fs.Int("budget", 3000, "requests permitted per hour. Public Horizon allows about 3600 per IP")
	bidUnit := fs.String("bid-amount-unit", string(horizon.BidAmountUnitQuote),
		"which asset an order book bid amount is denominated in: quote or base. See BidAmountUnit in internal/horizon")
	verify := fs.Bool("verify", true, "verify every asset's code, issuer and type on Horizon before the first round")
	maxHolderAge := fs.Duration("max-holder-age", 48*time.Hour,
		"ignore a cached holder reading older than this, so its figures are unevaluated rather than stale. 0 disables the bound")

	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `keel scan - compute metrics for every active asset and store them

The demonstration set comes from the assets table, not from a pair file: declare it
with "keel assets -pairs <file>" first. Three Horizon requests per asset per round,
which is the budget line in section 6.4 of the technical design.

A result is written once per (asset, ledger, methodology version, source). Scanning
a ledger that is already stored writes NOTHING and is not an error, so a re-run
after a crash is safe and a differing result is a finding rather than an overwrite.

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

	s, err := openStore(ctx, *dsn)
	if err != nil {
		return err
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
		return scanOnce(ctx, s, client, rows, *maxHolderAge, logger)
	}

	logger.Printf("interval %s, Ctrl-C to stop", *interval)
	ticker := time.NewTicker(*interval)
	defer ticker.Stop()
	for {
		if err := scanOnce(ctx, s, client, rows, *maxHolderAge, logger); err != nil {
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
func scanOnce(ctx context.Context, s *store.Store, client *horizon.Client, rows []store.Asset, maxHolderAge time.Duration, logger *log.Logger) error {
	startedAt := time.Now().UTC()
	runID, err := s.StartRun(ctx, store.RunScan, startedAt)
	if err != nil {
		return fmt.Errorf("scan: %w", err)
	}

	var ok, failed, panicked, stored, alreadyThere int
	var withHolders int
	holderGaps := map[string]int{}
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

		// FR-8 comes from the cache that `keel holders` fills, not from a pull of
		// this round's own. See the header of holders.go for the request budget
		// that forces the split.
		sup, why := supportingFor(ctx, s, a.Base, time.Now().UTC(), maxHolderAge)
		if sup != nil {
			withHolders++
		} else {
			holderGaps[why]++
		}

		risk, didPanic, err := computeRisk(obs.Snapshot, params, sup)
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

	var parts []string
	if panicked > 0 {
		parts = append(parts, fmt.Sprintf("%d of %d asset(s) panicked inside internal/domain", panicked, len(rows)))
	}
	// THE RUN ROW IS THE ONLY PLACE THIS IS RECORDED, and that is a gap rather
	// than a design. DEC-018 section 1 point 2 proposes that a metrics row carry
	// the holder half's own snapshot ledger beside LedgerSeq; that record is a
	// DRAFT and the column does not exist, so a stored row cannot say where its
	// holder figures came from or how old they were. Until it is decided the
	// provenance lives here, per round rather than per row, which is weaker and
	// is better than nothing being written down at all.
	parts = append(parts, fmt.Sprintf("holder concentration attached to %d of %d asset(s)", withHolders, ok))
	for _, why := range sortedKeys(holderGaps) {
		parts = append(parts, fmt.Sprintf("%d without: %s", holderGaps[why], why))
	}

	if err := s.FinishRun(ctx, runID, time.Now().UTC(), ok, failed, strings.Join(parts, "; ")); err != nil {
		return fmt.Errorf("scan: %w", err)
	}
	// THROTTLED IS PRINTED BESIDE REQUESTS AND WAS NOT UNTIL 12 SEPTEMBER 2026.
	// The client has counted 429 responses since it was written, retries
	// included, and nothing ever surfaced the number. Its own comment says why
	// that matters: a rate limit absorbed silently is indistinguishable from one
	// that never happened, and the two have opposite consequences. On 12
	// September a holder pull ran at one asset per twenty minutes and the
	// question "is this throttling or is the endpoint simply slow" could not be
	// answered from any log, because the only field that answers it was computed
	// and discarded.
	logger.Printf("round: %d ok (%d written, %d already stored), %d failed, %d with holder figures, %d requests this window, %d throttled",
		ok, stored, alreadyThere, failed, withHolders, client.Requests(), client.Throttled())
	for _, why := range sortedKeys(holderGaps) {
		logger.Printf("  no holder figures for %d asset(s): %s", holderGaps[why], why)
	}

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
func computeRisk(s domain.Snapshot, p domain.Params, sup *domain.SupportingMetrics) (risk domain.AssetRisk, panicked bool, err error) {
	defer func() {
		if r := recover(); r != nil {
			panicked = true
			err = fmt.Errorf("computing: panic: %v", r)
		}
	}()
	risk, err = domain.ComputeAssetRiskWith(s, p, sup)
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

// supportingFor builds the supporting metrics for one asset from the newest
// cached holder reading, and reports in one word why there are none when there
// are none.
//
// IT DOES NOT CALL domain.ComputeSupporting, AND THAT IS THE CADENCE SPLIT RATHER
// THAN A SHORTCUT. FR-8 is computed by domain.HolderConcentration at PULL time,
// inside `keel holders`, because the pull is what has the trustline balances and
// a scan round cannot afford to take one. The cache stores the three figures that
// produced, not the balances behind them, so there is nothing here to recompute
// and recomputing would mean re-pulling. The domain function still owns the
// definition; it ran earlier, in the command whose budget allows it.
//
// THE TRADE HALF IS DEFERRED and every field it fills is left nil. That is the
// split point in tugas-a.md section 6b, and it is a measured absence rather than
// an oversight: domain.SummariseGenuine over no trades yields a nil
// TradesExcludedPct and a nil LastGenuineTrade, so WASH_TRADE_SUSPECTED,
// NO_GENUINE_TRADE_7D and NO_GENUINE_TRADE_30D stay unevaluated rather than
// reading as checked and clear.
//
// WHAT IT RETURNS NIL FOR, and all four are the same statement: this round has no
// holder answer, so the two holder flags must stay unevaluated. Zero would be a
// measurement nobody made.
func supportingFor(
	ctx context.Context,
	s *store.Store,
	base domain.Asset,
	now time.Time,
	maxAge time.Duration,
) (*domain.SupportingMetrics, string) {
	// XLM has no trustlines at all, so there is no holder reading to want. It is
	// a permanent property of the asset and not a gap in the data.
	if base.IsNative() {
		return nil, "native asset has no trustlines"
	}

	reading, err := s.LatestHolderReading(ctx, base, domain.MethodologyVersion)
	switch {
	case errors.Is(err, store.ErrNotFound):
		return nil, "no holder reading yet, run `keel holders`"
	case err != nil:
		// Reported, not fatal. A database hiccup reading a cache must not cost
		// the depth half of the round, which is the deliverable.
		return nil, "holder reading unreadable: " + err.Error()
	}

	return supportingFromReading(reading, now, maxAge)
}

// supportingFromReading is the decision half of supportingFor, split out because
// it is the half worth testing and it needs no database to make.
//
// THE LEDGER TRAVELS INSIDE THE STRUCT, which is DEC-018 point 2 and is why this
// returns one value rather than a pair. domain.SupportingMetrics carries
// HolderSnapshotLedger beside the three figures precisely so that no call site
// has to remember to keep them together, and internal/store refuses a struct
// where one is set and the other is not.
func supportingFromReading(
	reading store.HolderReading,
	now time.Time,
	maxAge time.Duration,
) (*domain.SupportingMetrics, string) {
	// THE BOUND IS A MECHANISM DECIDED HERE AND A NUMBER DECIDED ELSEWHERE.
	// DEC-018 section 1 point 3 proposes 48 hours and its own section 7 says the
	// figure is proposed rather than derived: nothing has measured how fast the
	// top 1 per cent share of a thin Stellar asset moves. The flag carries that
	// default so the assumption is visible and settable, and -max-holder-age=0
	// disables the bound for whoever prefers the other alternative that record
	// weighs, which is to let a stale reading through and label it.
	if age := reading.Age(now); maxAge > 0 && age > maxAge {
		return nil, fmt.Sprintf("holder reading older than %s", maxAge)
	}

	// A truncated pull is stored WITH its flag and WITHOUT figures, so this is
	// the same refusal arriving one step later. domain.ErrHolderSetTruncated is
	// where it started, and Top1Pct is checked as well as the flag because a
	// complete pull whose population was empty after exclusions also has none.
	if reading.Truncated {
		return nil, "holder set was truncated, so concentration is unevaluated"
	}
	if reading.Top1Pct == nil {
		return nil, "holder reading carries no concentration figures"
	}

	ledger := reading.SnapshotLedger
	return &domain.SupportingMetrics{
		HolderTop1Pct:        reading.Top1Pct,
		HolderTop10Pct:       reading.Top10Pct,
		HolderHHI:            reading.HHI,
		HolderSnapshotLedger: &ledger,
		// The denominator the three figures above were divided by, carried
		// through because MANIPULATION_RATIO_LOW needs it. It comes from the same
		// cached reading as the rest, so a truncated pull that stored no
		// concentration stored no supply either and the flag stays unevaluated
		// alongside the other two. DEC-017 and DEC-018 point 2.
		CirculatingSupply: reading.CirculatingSupply,
	}, ""
}

// sortedKeys is non-negotiable rule 2 applied to a counter. Go randomizes map
// order, and a round whose notes list the same gaps in a different order every
// time cannot be diffed against the round before it.
func sortedKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
