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
	"io"
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
	schema := fs.Int("schema", horizon.TickSchemaVersion,
		"recording schema to write: 2 stores raw response bytes and nothing else, 1 stores the parsed "+
			"snapshot beside them. Files already written in either schema stay readable")
	verify := fs.Bool("verify", true, "verify every asset's code, issuer and type on Horizon before recording")
	budget := fs.Int("budget", 3000, "requests permitted per hour. Public Horizon allows about 3600 per IP")
	bidUnit := fs.String("bid-amount-unit", string(horizon.BidAmountUnitQuote),
		"which asset an order book bid amount is denominated in: quote or base. See BidAmountUnit in internal/horizon")
	holders := fs.Bool("holders", false,
		"also record the trustline holder distribution of every base asset. Costs one request per 200 accounts")
	holderPages := fs.Int("holder-pages", 0,
		"cap one holder reading at this many pages of 200 accounts. 0 uses the default of 25, which is 5000 accounts")
	holderInterval := fs.Duration("holder-interval", 0,
		"how often to read holders. 0 means every -interval round, which is rarely what you want: a trustline "+
			"balance moves over days and an order book moves in seconds")
	crosscheck := fs.Bool("crosscheck", false,
		"after each recording is written, schedule the Layer 3 rebuild-and-compare for that same recording. "+
			"Off by default: it spends the same Horizon budget the recorder does")
	crosscheckAfter := fs.Duration("crosscheck-after", defaultCrosscheckDelay,
		fmt.Sprintf("how long after a recording its rebuild runs. Default %s, must be under %s, and 0 means "+
			"immediately. This is the variable the same-hour experiment is testing", defaultCrosscheckDelay, maxCrosscheckDelay))
	crosscheckOut := fs.String("crosscheck-out", "",
		"append one CSV row per comparison to this file, flushed per row. Optional; the rows are printed either way")

	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `keel record - record raw Horizon snapshots for cross-validation

SCHEMA 2, the default, writes {out}/{pair}/{date}/{ledgerBefore}.json.gz, one
file per pair per tick. It holds ONLY raw response bytes: the order book and the
liquidity pools, each with the exact URL requested, the HTTP status, the body
verbatim as a string, and that body's sha256. It parses nothing, converts
nothing, and makes no judgement about data quality. A non-2xx and an empty pool
list are both recorded and kept. Nothing is ever overwritten; a name already
taken gets a monotonic suffix.

SCHEMA 1 writes {out}/{pair}/{ledgerSeq}.json.gz and holds the parsed
conclusions AND the raw response bodies. It is still reachable with -schema 1
and every file it has written stays readable, but it is no longer the default:
the parsed half is the half that had to be revised when the bid amount unit
turned out to be quote-denominated, and a recording that claims nothing cannot
go stale that way.

With -holders it also writes {out}/holders/{asset}/{ledgerSeq}.json.gz, one file
per ledger per BASE asset, holding every trustline holder and the issued supply.
That reading has the same property and a worse version of it: Horizon serves no
historical trustline balance at all, so holder concentration for a past ledger is
not recoverable from it by any route. It is off by default because its cost grows
with the asset, one request per 200 accounts, where a pair snapshot is always
three.

WITH -crosscheck each recording is paired with a rebuild of the same book,
scheduled -crosscheck-after later and compared by exactly the code behind
"keel crosscheck". Every row carries the elapsed time between the recording and
the rebuild, which is the variable under test: the first Layer 3 run put 23 of 60
PARTIAL rows down to a seven hour gap, and no row in it recorded a gap at all.
The delay is under an hour by construction, so a batch is recorded and compared
inside the same hour. It costs one rebuild's worth of Horizon requests per
recording, out of the same -budget the recorder is spending.

Give -holders its own -holder-interval. Without one it reads holders on every
-interval round, which sets the cadence of a balance that moves over days by the
cadence of a book that moves in seconds, and writes a near-identical file each
time in the one format here whose size grows with the asset.

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
	if *schema != 1 && *schema != horizon.TickSchemaVersion {
		return fmt.Errorf("record: -schema must be 1 or %d", horizon.TickSchemaVersion)
	}
	// The delay is validated whether or not the pairing is on, because a value
	// that would be refused when it is used is not a value worth accepting when
	// it is ignored. The upper bound is what makes "the same hour" a property of
	// the flags; see maxCrosscheckDelay.
	if *crosscheckAfter < 0 || *crosscheckAfter >= maxCrosscheckDelay {
		return fmt.Errorf("record: -crosscheck-after must be at least 0 and less than %s, got %s. "+
			"The point of this pairing is a comparison inside the hour the recording was taken",
			maxCrosscheckDelay, *crosscheckAfter)
	}
	if *crosscheck {
		// Schema 1 writes the parsed conclusions beside the bytes under a
		// different name and layout, and the comparison reads a schema 2 tick.
		// Refusing is better than comparing a file whose shape is not what the
		// reader assumes and reporting the difference as a finding about Horizon.
		if *schema != horizon.TickSchemaVersion {
			return fmt.Errorf("record: -crosscheck needs schema %d, got %d", horizon.TickSchemaVersion, *schema)
		}
		// The paired loop is this binary's own, because the recorder's loop
		// reports a round as a count and keeps the paths. That loop is the one
		// place the holder cadence lives, so a continuous paired run would have
		// to reimplement it and the two would drift. -once takes its holder
		// reading in this file already, so that combination is allowed.
		if *holders && !*once {
			return errors.New("record: -holders and -crosscheck can only be combined with -once, because " +
				"the holder cadence lives in the recorder's own loop and the paired loop is a different one")
		}
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
		Client:         client,
		Root:           *out,
		Pairs:          pairs,
		Schema:         *schema,
		Holders:        *holders,
		HolderInterval: *holderInterval,
		Logf:           logger.Printf,
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

	logger.Printf("recording %d pair(s) into %s in schema %d", len(pairs), *out, *schema)
	if *schema == 1 {
		logger.Printf("schema 1 also parses each reading, bid amount read as %s", unit)
	}
	if *holders {
		assets := rec.HolderAssets()
		cadence := "every round"
		if *holderInterval > 0 {
			cadence = "every " + holderInterval.String()
		}
		// The cadence is logged rather than left implicit because a reading NOT
		// taken leaves no file to notice its absence in, unlike a failed one.
		logger.Printf("holder distribution on for %d base asset(s), %s: %s",
			len(assets), cadence, assetList(assets))
	}

	var runner *sameHourRunner
	if *crosscheck {
		var appender *crosscheckAppender
		if *crosscheckOut != "" {
			appender, err = openCrosscheckAppender(*crosscheckOut)
			if err != nil {
				return fmt.Errorf("record: %w", err)
			}
			defer func() { _ = appender.Close() }()
		}
		runner = newSameHourRunner(sameHourConfig{
			Recorder: rec,
			// The SAME client, so the rebuilds and the recordings spend one
			// budget. Two clients would each believe they had 3000 requests an
			// hour and the shared IP would find out otherwise.
			Client: client,
			Unit:   unit,
			Delay:  *crosscheckAfter,
			Out:    os.Stdout,
			Logf:   logger.Printf,
			CSV:    appender,
		})
		logger.Printf("same-hour crosscheck on: every recording is rebuilt and compared %s after it was "+
			"recorded (default %s, ceiling %s)", *crosscheckAfter, defaultCrosscheckDelay, maxCrosscheckDelay)
		if *crosscheckOut != "" {
			logger.Printf("comparison rows append to %s", *crosscheckOut)
		}
		// On stdout as well as in the log, in the flat key=value form the round
		// tally already uses, so the delay a batch was measured at is in the same
		// stream as the batch. A rate quoted without its delay is the sentence
		// this whole change exists to make checkable.
		fmt.Fprintf(os.Stdout, "crosscheck_delay=%s crosscheck_delay_default=%s crosscheck_delay_max=%s\n",
			*crosscheckAfter, defaultCrosscheckDelay, maxCrosscheckDelay)
	}

	if *once {
		unwritten, results := recordOneRound(ctx, rec, *schema, os.Stdout)
		holderFailed := rec.ReportHolders(rec.RecordHoldersOnce(ctx))
		if runner != nil {
			// The comparison runs AFTER the holder reading, so a -holders round
			// does not sit on the queue while it pages through trustlines.
			//
			// IT NEVER DECIDES THE EXIT CODE. The recording is the half that
			// cannot be taken again, and a comparison that stopped early can be
			// run against the file on disk afterwards, at a longer gap. The two
			// lines below this one are still the only things that redden a round.
			if err := runner.runOnce(ctx, results); err != nil && !errors.Is(err, context.Canceled) {
				logger.Printf("crosscheck: stopped early: %v", err)
			}
			runner.summarise(os.Stdout)
		}
		if unwritten > 0 {
			return fmt.Errorf("record: %d of %d pair(s) wrote nothing", unwritten, len(pairs))
		}
		if holderFailed > 0 {
			return fmt.Errorf("record: %d holder reading(s) failed", holderFailed)
		}
		return nil
	}

	logger.Printf("interval %s, Ctrl-C to stop", *interval)
	if runner != nil {
		err = runner.run(ctx, *interval)
		runner.summarise(os.Stdout)
	} else {
		err = rec.Run(ctx, *interval)
	}
	if errors.Is(err, context.Canceled) {
		logger.Print("stopped")
		return nil
	}
	return err
}

// recordOneRound runs a single round and, in schema 2, prints one machine
// readable tally to w. It returns how many pairs wrote NOTHING.
//
// Only an unwritten tick counts. A tick holding a 429 or a 503 has been written
// and is evidence of exactly the thing it says, so it is not a failure and does
// not color an exit code: a recorder that goes red whenever Horizon is busy is
// a recorder whose red gets ignored, and the ledger it gave up on has closed by
// the time anybody looks.
//
// The tally goes to STDOUT while every log line goes to stderr, so a caller can
// read the numbers without parsing the log. It is deliberately a flat key=value
// line and not JSON: the only consumer is a shell step in a workflow, and a
// shell reading JSON needs a JSON parser installed on the runner.
// It also returns the round's per-pair results, which is what the same-hour
// pairing schedules from: it needs the PATH of each file that was just written,
// and a directory sweep afterwards cannot tell this round's files from the ones
// already there. Schema 1 returns none, and -crosscheck is refused with it.
func recordOneRound(ctx context.Context, rec *horizon.Recorder, schema int, w io.Writer) (int, []horizon.TickResult) {
	if schema == 1 {
		return rec.Report(rec.RecordOnce(ctx)), nil
	}
	results := rec.RecordTicksOnce(ctx)
	round := rec.ReportTicks(results)
	// The tally is a convenience for a workflow step. Failing to print it must
	// not turn a round that reached disk into a failure, so the error is
	// discarded deliberately rather than by omission.
	_, _ = fmt.Fprintf(w, "ticks_written=%d ticks_degraded=%d ticks_straddled=%d ticks_collided=%d ticks_unwritten=%d\n",
		round.Written, round.Degraded, round.Straddled, round.Collided, round.Unwritten)
	return round.Unwritten, results
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
