// The `bookseries` subcommand: one pair's order book at many past ledgers, and
// the methodology run over each of them.
//
// IT OWNS NO METHODOLOGY. internal/horizon rebuilds the books, internal/domain
// computes over them, and this file parses flags, chooses which ledgers to
// sample, and writes a CSV with a provenance sidecar. If a formula appears here
// it is in the wrong file.
//
// WHAT IT IS FOR. Deliverable 2 asks when the unsafe threshold was crossed
// relative to the exploit date. That question needs the book on every day of the
// month rather than on one day, and February 2026's book state is the only place
// the answer lives: docs/evidences/2026-08-26-ustry-february-trades-implied.md
// measured the trade stream over the same month and found no pre-exploit signal
// in it at all, because what made USTRY dangerous was never in what traded. It
// was in what was POSTED.
//
// THE SAMPLE IS DERIVED FROM A COMMITTED FILE AND COSTS NO REQUESTS. -from-trades
// reads a trades CSV this repository already holds and takes, for each UTC day in
// it, the ledger of the first trade at or after midnight. That fixes the sampling
// rule in code rather than in somebody's shell history, and every row reports the
// instant it actually sampled and how far that fell from midnight, so a day whose
// first trade came late is visible rather than assumed away.
//
// READ THE DIAGNOSTICS, AND THE CSV CARRIES THEM PER ROW FOR A REASON. A
// reconstruction that missed offers reports a THINNER book, thin books are this
// product's most interesting finding, and a series makes that failure look like a
// trend rather than like an error. So every row carries how many offers it
// rebuilt and how many holes the fold found in itself, and the sidecar carries
// what the one walk behind all of them cost and how deep it reached.
package main

import (
	"context"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Keel-Official/keel-backend/internal/domain"
	"github.com/Keel-Official/keel-backend/internal/horizon"
	"github.com/shopspring/decimal"
)

// seriesSample is one chosen ledger and where the choice came from.
type seriesSample struct {
	Day      string
	Ledger   uint32
	ClosedAt time.Time
	// OffsetSeconds is how far ClosedAt fell after that day's midnight. It is
	// reported rather than corrected: a day whose first trade came an hour late
	// was sampled an hour late, and pretending otherwise would put a number in
	// the CSV that no ledger ever held.
	OffsetSeconds int64
	// Source is "trades-csv" for a derived sample and "flag" for one named on the
	// command line, so a reader can tell the control ledger from the daily grid.
	Source string
}

func runBookSeries(args []string) error {
	fs := flag.NewFlagSet("bookseries", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	pairsPath := fs.String("pairs", "", "path to a pair list, required. The series is run over the FIRST pair in it")
	fromTrades := fs.String("from-trades", "",
		"a trades CSV to derive one target per UTC day from: the first trade at or after each midnight. Costs no requests")
	alsoLedger := fs.String("also-ledger", "",
		"extra target ledgers, comma separated, added to whatever -from-trades derived. 61340262 is the control ledger the golden fixture describes")
	tradesFrom := fs.Uint("trades-from-ledger", 0, "ledger to seek the trade walk to. Must be at or below -since-ledger")
	lookahead := fs.Uint("lookahead", 5000, "how many ledgers PAST the latest target the trade walk continues, for account discovery only")
	since := fs.Uint("since-ledger", 0,
		"floor on each account's backwards walk. ONE floor for the whole series, and it must be at or below the earliest target")
	maxPages := fs.Int("max-pages-per-account", 0, "cap on each account's backwards walk, in pages of 200. 0 uses the built-in default")
	out := fs.String("csv", "", "write the series to this CSV. A .meta.txt sidecar is written beside it")
	quiet := fs.Bool("quiet", false, "do not print one progress line per account walked")
	baseURL := fs.String("horizon", horizon.DefaultBaseURL, "Horizon base URL")
	budget := fs.Int("budget", 3000, "requests permitted per hour. NFR-6 caps this repository at 3000, under the public 3600")

	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `keel bookseries - the order book at many past ledgers, from one walk

Horizon serves no order book at a past ledger, and `+"`keel replay`"+` rebuilds one from
the operations that posted it. This runs that reconstruction at a LIST of ledgers
and pays for the walk once: what separates two targets is a pure fold over the
same operations, not a second visit to Horizon.

Pools are not reconstructed. Every row is ORDER BOOK ONLY, and a combined depth
figure taken from one would be wrong.

`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *pairsPath == "" {
		return errors.New("bookseries: -pairs is required")
	}
	if *out == "" {
		return errors.New("bookseries: -csv is required, because a series nobody wrote down is a bill without a reading")
	}

	pairs, err := horizon.LoadPairs(*pairsPath)
	if err != nil {
		return fmt.Errorf("bookseries: %w", err)
	}
	if len(pairs) == 0 {
		return errors.New("bookseries: the pair list is empty")
	}
	pair := pairs[0]

	samples, err := gatherSamples(*fromTrades, *alsoLedger)
	if err != nil {
		return fmt.Errorf("bookseries: %w", err)
	}
	if len(samples) == 0 {
		return errors.New("bookseries: no target ledgers. Give -from-trades, -also-ledger, or both")
	}

	targets := make([]uint32, 0, len(samples))
	for _, s := range samples {
		targets = append(targets, s.Ledger)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client := horizon.NewClient(horizon.Config{BaseURL: *baseURL, Budget: *budget})

	fmt.Fprintf(os.Stdout, "bookseries %s\n", pair)
	fmt.Fprintf(os.Stdout, "  %d target(s), ledger %d to %d\n", len(targets), targets[0], targets[len(targets)-1])
	fmt.Fprintf(os.Stdout, "  one walk at the latest target, floor %d, lookahead %d\n", *since, *lookahead)

	started := time.Now()
	walked := 0
	res, err := client.ReconstructSeries(ctx, pair.Base, pair.Quote, horizon.SeriesQuery{
		Targets:            targets,
		TradesFromLedger:   uint32(*tradesFrom),
		TradeLookahead:     uint32(*lookahead),
		SinceLedger:        uint32(*since),
		MaxPagesPerAccount: *maxPages,
		Progress: func(w horizon.AccountWalk) {
			walked++
			if *quiet {
				return
			}
			fmt.Fprintf(os.Stderr, "  [%3d] %s  %d page(s), back to ledger %d, %d offer op(s)%s%s%s\n",
				walked, w.Account[:8], w.Pages, w.EarliestLedger, w.OfferOperations,
				flagIf(w.Truncated, " TRUNCATED"), flagIf(w.StoppedAtFloor, " floor"),
				flagIf(w.Err != "", " FAILED"))
		},
	})
	if err != nil {
		return fmt.Errorf("bookseries %s: %w", pair, err)
	}
	elapsed := time.Since(started)

	byLedger := map[uint32]seriesSample{}
	for _, s := range samples {
		byLedger[s.Ledger] = s
	}

	rows, err := seriesRows(res, byLedger)
	if err != nil {
		return fmt.Errorf("bookseries: %w", err)
	}
	if err := writeSeriesCSV(*out, rows, domain.DefaultParams()); err != nil {
		return fmt.Errorf("bookseries: %w", err)
	}
	meta := strings.TrimSuffix(*out, filepath.Ext(*out)) + ".meta.txt"
	if err := writeSeriesMeta(meta, pair, res, samples, *fromTrades, uint32(*since), uint32(*tradesFrom), uint32(*lookahead), elapsed); err != nil {
		return fmt.Errorf("bookseries: %w", err)
	}

	summariseSeries(os.Stdout, res, rows, elapsed)
	fmt.Fprintf(os.Stdout, "  wrote %s\n  wrote %s\n", *out, meta)
	return nil
}

// seriesRow is one CSV line: a sample, its book, and the methodology over it.
type seriesRow struct {
	Sample seriesSample
	Point  horizon.SeriesPoint
	Risk   domain.AssetRisk
}

func seriesRows(res horizon.SeriesResult, byLedger map[uint32]seriesSample) ([]seriesRow, error) {
	rows := make([]seriesRow, 0, len(res.Points))
	for _, p := range res.Points {
		risk, err := domain.ComputeAssetRisk(p.Snapshot, domain.DefaultParams())
		if err != nil {
			// A COMPUTE FAILURE ON ONE POINT FAILS THE SERIES, and that is the
			// opposite of how a failed account walk is treated a level down. The
			// reason is the direction of the error: a missing account loses
			// offers and reads as a thinner book, which is conservative, while a
			// row silently dropped out of a time series moves a date, and the
			// date is what Deliverable 2 is asking for.
			return nil, fmt.Errorf("computing ledger %d: %w", p.Target, err)
		}
		rows = append(rows, seriesRow{Sample: byLedger[p.Target], Point: p, Risk: risk})
	}
	return rows, nil
}

func writeSeriesCSV(path string, rows []seriesRow, p domain.Params) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	w := csv.NewWriter(f)
	deltas := append([]decimal.Decimal{}, p.MarketDeltas...)
	ladder := rungs(p)

	head := []string{
		"day", "sample_source", "target_ledger", "sampled_at_utc", "sample_offset_seconds",
		"bids", "asks", "resting_offers", "missing_offer_ids", "fold_complete",
		"price_source", "p0", "best_bid", "best_ask", "spread_pct",
	}
	for _, d := range deltas {
		head = append(head, "depth_buy_"+d.String(), "depth_sell_"+d.String())
	}
	for _, d := range ladder {
		head = append(head, "mc_cost_"+d.String(), "mc_target_"+d.String(), "mc_reachable_"+d.String())
	}
	head = append(head, "max_reachable_price", "cost_to_max_reachable_price",
		"band", "band_confidence", "flags", "unevaluated_flags", "methodology_version", "data_source")
	if err := w.Write(head); err != nil {
		return err
	}

	for _, r := range rows {
		rec := []string{
			r.Sample.Day,
			r.Sample.Source,
			strconv.FormatUint(uint64(r.Point.Target), 10),
			formatSeriesTime(r.Sample.ClosedAt),
			strconv.FormatInt(r.Sample.OffsetSeconds, 10),
			strconv.Itoa(len(r.Point.Snapshot.Book.Bids)),
			strconv.Itoa(len(r.Point.Snapshot.Book.Asks)),
			strconv.Itoa(r.Point.RestingOffers),
			strconv.Itoa(len(r.Point.MissingOfferIDs)),
			strconv.FormatBool(r.Point.Complete()),
			string(r.Risk.PriceSource),
			optional(r.Risk.MidPrice),
			bestLevel(r.Point.Snapshot.Book.Bids),
			bestLevel(r.Point.Snapshot.Book.Asks),
			optional(r.Risk.SpreadPct),
		}
		for _, d := range deltas {
			buy, sell := depthAt(r.Risk.Depth, d)
			rec = append(rec, buy, sell)
		}
		for _, d := range ladder {
			cost, target, reachable := manipulationAt(r.Risk.ManipulationCostOrderbookOnly, d)
			rec = append(rec, cost, target, reachable)
		}
		rec = append(rec,
			optional(r.Risk.MaxReachablePrice),
			optional(r.Risk.CostToMaxReachablePrice),
			string(r.Risk.Band),
			string(r.Risk.BandConfidence),
			joinFlags(r.Risk.Flags),
			joinFlags(r.Risk.UnevaluatedFlags),
			r.Risk.MethodologyVersion,
			string(r.Risk.DataSource),
		)
		if err := w.Write(rec); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

// depthAt reads one delta out of the depth ladder by value rather than by
// position, because a ladder that gains a rung must not shift a column.
func depthAt(points []domain.DepthPoint, d decimal.Decimal) (string, string) {
	for _, p := range points {
		if p.Delta.Equal(d) {
			return p.BuySide.String(), p.SellSide.String()
		}
	}
	return "", ""
}

func manipulationAt(points []domain.ManipulationPoint, d decimal.Decimal) (string, string, string) {
	for _, p := range points {
		if p.Delta.Equal(d) {
			return p.Cost.String(), p.TargetPrice.String(), strconv.FormatBool(p.Reachable)
		}
	}
	return "", "", ""
}

func bestLevel(levels []domain.Level) string {
	if len(levels) == 0 {
		return ""
	}
	return levels[0].Price.Decimal().String()
}

func joinFlags(flags []domain.Flag) string {
	if len(flags) == 0 {
		return ""
	}
	out := make([]string, 0, len(flags))
	for _, f := range flags {
		out = append(out, string(f))
	}
	sort.Strings(out) // NFR-9: the same run writes the same cell
	return strings.Join(out, " ")
}

func formatSeriesTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// gatherSamples builds the target list from a trades CSV, from the -also-ledger
// flag, or from both.
func gatherSamples(tradesCSV, also string) ([]seriesSample, error) {
	var samples []seriesSample
	if tradesCSV != "" {
		got, err := dailySamplesFromTrades(tradesCSV)
		if err != nil {
			return nil, err
		}
		samples = append(samples, got...)
	}
	for _, part := range strings.Split(also, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.ParseUint(part, 10, 32)
		if err != nil || n == 0 {
			return nil, fmt.Errorf("-also-ledger %q is not a ledger sequence", part)
		}
		samples = append(samples, seriesSample{Ledger: uint32(n), Source: "flag"})
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i].Ledger < samples[j].Ledger })
	return samples, nil
}

// dailySamplesFromTrades takes the first trade at or after each UTC midnight.
//
// THE RULE IS "FIRST TRADE OF THE DAY" AND NOT "NEAREST TO MIDNIGHT", and the
// difference is one of honesty rather than of accuracy. The first trade of a day
// is always at or after that midnight, so the sample is never taken from the
// previous day's state; the nearest trade could be either side and a row could
// then describe a book that belonged to the day before it.
func dailySamplesFromTrades(path string) ([]seriesSample, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	head, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("reading the header of %s: %w", path, err)
	}
	ledgerCol, timeCol := -1, -1
	for i, h := range head {
		switch strings.TrimSpace(h) {
		case "ledger_seq":
			ledgerCol = i
		case "closed_at":
			timeCol = i
		}
	}
	if ledgerCol < 0 || timeCol < 0 {
		return nil, fmt.Errorf("%s has no ledger_seq and closed_at columns, so no sample can be derived from it", path)
	}

	seen := map[string]bool{}
	var out []seriesSample
	for {
		rec, err := r.Read()
		if err != nil {
			break
		}
		if ledgerCol >= len(rec) || timeCol >= len(rec) {
			continue
		}
		closed, err := time.Parse(time.RFC3339, strings.TrimSpace(rec[timeCol]))
		if err != nil {
			continue
		}
		day := closed.UTC().Format("2006-01-02")
		if seen[day] {
			continue
		}
		n, err := strconv.ParseUint(strings.TrimSpace(rec[ledgerCol]), 10, 32)
		if err != nil || n == 0 {
			continue
		}
		seen[day] = true
		midnight := closed.UTC().Truncate(24 * time.Hour)
		out = append(out, seriesSample{
			Day:           day,
			Ledger:        uint32(n),
			ClosedAt:      closed.UTC(),
			OffsetSeconds: int64(closed.UTC().Sub(midnight).Seconds()),
			Source:        "trades-csv",
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s yielded no daily sample", path)
	}
	return out, nil
}

func writeSeriesMeta(path string, pair horizon.Pair, res horizon.SeriesResult, samples []seriesSample,
	tradesCSV string, floor, tradesFrom, lookahead uint32, elapsed time.Duration) error {
	var b []byte
	add := func(format string, args ...any) { b = append(b, fmt.Sprintf(format, args...)...) }

	add("# Provenance for %s\n", filepath.Base(strings.TrimSuffix(path, ".meta.txt")+".csv"))
	add("# Written beside the CSV rather than inside it, for the reason DEC-010\n")
	add("# gives: a leading '#' line is read as a header by csv.DictReader.\n")
	add("#\n")
	add("# ONE WALK PRODUCED EVERY ROW. The counts below are that walk's, not any\n")
	add("# one row's. The per-row diagnostics are columns in the CSV.\n")
	add("pair: %s\n", pair)
	add("data_source: %s\n", domain.DataSourceOffersImplied)
	add("methodology_version: %s\n", domain.MethodologyVersion)
	add("points: %d\n", len(res.Points))
	if len(res.Points) > 0 {
		add("earliest_target_ledger: %d\n", res.Points[0].Target)
		add("latest_target_ledger: %d\n", res.Points[len(res.Points)-1].Target)
	}
	add("sample_rule: first trade at or after each UTC midnight, plus any -also-ledger\n")
	if tradesCSV != "" {
		add("sample_source_file: %s\n", tradesCSV)
	}
	add("operation_floor_ledger: %d\n", floor)
	add("trade_window_from_ledger: %d\n", tradesFrom)
	add("trade_lookahead_ledgers: %d\n", lookahead)
	add("earliest_offer_operation_ledger: %d\n", res.EarliestOfferOp)
	add("accounts_walked: %d\n", len(res.Accounts))
	add("accounts_from_trades: %d\n", res.FromTrades)
	add("accounts_from_live_offers: %d\n", res.FromLiveOffers)
	add("trades_read: %d\n", res.TradesRead)
	add("operations_read: %d\n", res.OperationsRead)
	add("offer_operations: %d\n", res.OfferOperations)
	add("walks_truncated: %d\n", res.Truncated)
	add("walks_stopped_at_floor: %d\n", res.StoppedAtFloor)
	add("walks_failed: %d\n", res.Failed)
	add("unsizable_operations: %d\n", res.Unsizable)
	add("walk_complete: %t\n", res.WalkComplete())
	add("requests: %d\n", res.Requests)
	add("elapsed_seconds: %d\n", int(elapsed.Seconds()))
	add("#\n")
	add("# A truncated or failed walk loses offers, and a lost offer reads as a\n")
	add("# THINNER book rather than as an error. Read walks_truncated and\n")
	add("# walks_failed before reading any row as a market that emptied.\n")
	add("#\n")
	add("# An offer created below the operation floor is invisible in EVERY row\n")
	add("# equally, which is why the floor is one number for the whole series.\n")
	add("# Compare earliest_offer_operation_ledger against it.\n")
	add("#\n")
	add("# NO POOL IS RECONSTRUCTED. Every row is order book only.\n")
	return os.WriteFile(path, b, 0o644)
}

func summariseSeries(w *os.File, res horizon.SeriesResult, rows []seriesRow, elapsed time.Duration) {
	fmt.Fprintf(w, "  --- %d point(s) from one walk over %d account(s) in %s ---\n",
		len(res.Points), len(res.Accounts), elapsed.Round(time.Second))
	fmt.Fprintf(w, "  %d request(s), %d trade(s), %d offer operation(s)\n",
		res.Requests, res.TradesRead, res.OfferOperations)
	if !res.WalkComplete() {
		fmt.Fprintf(w, "  WALK INCOMPLETE: %d truncated, %d failed. Every row below reads THINNER than the market was\n",
			res.Truncated, res.Failed)
	}
	empty := 0
	for _, r := range rows {
		if r.Point.RestingOffers == 0 {
			empty++
		}
	}
	if empty > 0 {
		fmt.Fprintf(w, "  %d point(s) rebuilt an EMPTY book. Check the floor before reading that as a market with no offers\n", empty)
	}
	for _, r := range rows {
		fmt.Fprintf(w, "  %s  ledger %d  %d bid(s) %d ask(s)  band %s  flags %v\n",
			r.Sample.Day, r.Point.Target, len(r.Point.Snapshot.Book.Bids), len(r.Point.Snapshot.Book.Asks),
			r.Risk.Band, r.Risk.Flags)
	}
}
