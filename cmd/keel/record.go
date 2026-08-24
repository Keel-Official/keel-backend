// The `record` subcommand: the cross-validation recorder.
//
// It is the first subcommand with a body because it is the only one whose work
// cannot be done later. Layer 3 of docs/methodology/10-validation.md compares a
// live Horizon reading against a reconstruction of the same ledger, and the live
// half has to be taken while that ledger is the current one. Every day without
// this running is a day of evidence that no amount of later effort recovers.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Keel-Official/keel-backend/internal/domain"
	"github.com/Keel-Official/keel-backend/internal/horizon"
)

func runRecord(args []string) error {
	fs := flag.NewFlagSet("record", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	// No default pair list. Which assets Keel measures is decision D-1, and a
	// list compiled into this binary would be an entrypoint quietly making a
	// methodology decision. scripts/record-pairs.example.json is an example to
	// copy, not a selection.
	pairsPath := fs.String("pairs", "", "path to a pair list, required. Copy scripts/record-pairs.example.json")
	out := fs.String("out", "recordings", "directory to write recordings into")
	interval := fs.Duration("interval", 30*time.Minute, "how often to record")
	once := fs.Bool("once", false, "record one round and exit")
	baseURL := fs.String("horizon", horizon.DefaultBaseURL, "Horizon base URL")
	verify := fs.Bool("verify", true, "verify every asset's code, issuer and type on Horizon before recording")
	budget := fs.Int("budget", 3000, "requests permitted per hour. Public Horizon allows about 3600 per IP")
	bidUnit := fs.String("bid-amount-unit", string(horizon.BidAmountUnitQuote),
		"which asset an order book bid amount is denominated in: quote or base. See BidAmountUnit in internal/horizon")
	holders := fs.Bool("holders", false,
		"also record the trustline holder distribution of every base asset. Costs one request per 200 accounts")
	holderPages := fs.Int("holder-pages", 0,
		"cap one holder reading at this many pages of 200 accounts. 0 uses the default of 25, which is 5000 accounts")

	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `keel record - record raw Horizon snapshots for cross-validation

Writes {out}/{pair}/{ledgerSeq}.json.gz, one file per ledger per pair, and never
overwrites one that exists. Each file holds the parsed conclusions AND the raw
response bodies, so the reading of those bytes can be revised later without the
evidence being re-fetched, which is impossible for a past ledger.

With -holders it also writes {out}/holders/{asset}/{ledgerSeq}.json.gz, one file
per ledger per BASE asset, holding every trustline holder and the issued supply.
That reading has the same property and a worse version of it: Horizon serves no
historical trustline balance at all, so holder concentration for a past ledger is
not recoverable from it by any route. It is off by default because its cost grows
with the asset, one request per 200 accounts, where a pair snapshot is always
three.

`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *pairsPath == "" {
		fs.Usage()
		return errors.New("record: -pairs is required")
	}
	unit := horizon.BidAmountUnit(*bidUnit)
	if unit != horizon.BidAmountUnitQuote && unit != horizon.BidAmountUnitBase {
		return fmt.Errorf("record: -bid-amount-unit must be %q or %q",
			horizon.BidAmountUnitQuote, horizon.BidAmountUnitBase)
	}

	pairs, err := horizon.LoadPairs(*pairsPath)
	if err != nil {
		return fmt.Errorf("record: %w", err)
	}

	logger := log.New(os.Stderr, "record ", log.LstdFlags|log.LUTC)

	client := horizon.NewClient(horizon.Config{
		BaseURL:        *baseURL,
		Budget:         *budget,
		BidAmountUnit:  unit,
		MaxHolderPages: *holderPages,
		// No cache. Every round has to be a fresh reading; a cached body would
		// make two recordings identical for a reason that has nothing to do
		// with the market.
		CacheTTL: 0,
	})

	rec, err := horizon.NewRecorder(horizon.RecorderConfig{
		Client:  client,
		Root:    *out,
		Pairs:   pairs,
		Holders: *holders,
		Logf:    logger.Printf,
	})
	if err != nil {
		return fmt.Errorf("record: %w", err)
	}

	// SIGINT and SIGTERM cancel the context rather than killing the process, so
	// a round in flight finishes its current write. The recorder writes through
	// a temporary file and renames, so even a hard kill cannot leave a
	// truncated file that looks like a recording.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if *verify {
		if err := rec.Verify(ctx); err != nil {
			return fmt.Errorf("record: %w", err)
		}
	}

	logger.Printf("recording %d pair(s) into %s, bid amount read as %s", len(pairs), *out, unit)
	if *holders {
		assets := rec.HolderAssets()
		logger.Printf("holder distribution on for %d base asset(s): %s", len(assets), assetList(assets))
	}

	if *once {
		failed := rec.Report(rec.RecordOnce(ctx))
		holderFailed := rec.ReportHolders(rec.RecordHoldersOnce(ctx))
		if failed > 0 {
			return fmt.Errorf("record: %d of %d pair(s) failed", failed, len(pairs))
		}
		if holderFailed > 0 {
			return fmt.Errorf("record: %d holder reading(s) failed", holderFailed)
		}
		return nil
	}

	logger.Printf("interval %s, Ctrl-C to stop", *interval)
	err = rec.Run(ctx, *interval)
	if errors.Is(err, context.Canceled) {
		logger.Print("stopped")
		return nil
	}
	return err
}

// assetList names the assets a holder round will read, so the line that says a
// budget is about to be spent also says what it will be spent on.
func assetList(assets []domain.Asset) string {
	names := make([]string, 0, len(assets))
	for _, a := range assets {
		names = append(names, a.String())
	}
	if len(names) == 0 {
		return "none, every base asset in the pair list is native"
	}
	return strings.Join(names, ", ")
}
