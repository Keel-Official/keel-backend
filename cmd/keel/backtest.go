// The `backtest` subcommand: the trade-implied history of a pair, as CSV.
//
// IT OWNS NO METHODOLOGY. internal/horizon reads /trades, internal/domain turns
// operations into the depth bound that 01-data-sources.md section 6 defines, and
// this file groups rows by day and writes two files. Every threshold it compares
// against is read from domain.DefaultParams(); none is written here. If a formula
// ever appears in this file it is in the wrong file.
//
// WHY IT EXISTS. Deliverable 2 promises a USTRY time series for February 2026
// with the raw CSV in the repository, and a statement of when the unsafe
// threshold was crossed relative to the exploit date. Historical order book state
// would be the direct route and DEC-002 defers the source that serves it. Section
// 2 of that record names the substitutes, and this is 2.1 and 2.2 built: the
// manipulation cost read straight off the trade that happened, and the upper
// bound on depth implied by an operation that moved the price.
//
// THE TWO KINDS OF BOUND STAY APART ALL THE WAY INTO THE FILE, and that is the
// single most important thing about this command. domain.BoundWithinLeg is
// causal: one taker walked one book and paid for the span. domain
// .BoundBetweenLegs assumes the book did not change in a gap that can be
// minutes long, which trades cannot show. They get their own columns, their own
// verdicts and their own lines in the summary. Collapsing them into one number
// would be the most flattering thing this command could do and the least true.
//
// WHAT IT WILL NOT DO, and each is a red zone document rather than a gap.
//
//   - NO GENUINE-TRADE FILTER. 07-supporting-metrics.md section 1 is a worksheet.
//     Every trade Horizon returns is counted, so the VOLUME column inherits
//     whatever wash trading exists. The DEPTH BOUNDS do not inherit it in the
//     same way: a self-matched trade still crossed the book it crossed, so the
//     inequality holds for it exactly as for any other. What it does mean is that
//     the TIGHTEST bound on a day can be set by a dust trade, and the source
//     column names the record so a reader can see when it was.
//   - NO PAIR SELECTION. The pair comes from a -pairs file, the same data the
//     recorder reads. 02-pair-selection.md is a worksheet too.
//   - NO WINDOW OF ITS OWN. -from, -to and -mark are all arguments. The exploit
//     date is a fact in 10-validation.md section 7 and is not compiled in here.
package main

import (
	"context"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"syscall"
	"time"

	"github.com/Keel-Official/keel-backend/internal/domain"
	"github.com/Keel-Official/keel-backend/internal/horizon"
	"github.com/shopspring/decimal"
)

const dayLayout = "2006-01-02"

// backtestNow is the clock the window check reads, and the only wall clock this
// file has. It is a variable so a test can pin it: a check that refuses a future
// window is untestable against the real clock, because the fixture would expire.
var backtestNow = func() time.Time { return time.Now().UTC() }

// backtestStderr is where the coverage warning goes. It is a variable for the
// same reason backtestNow is: a warning that is only ever written to the real
// os.Stderr cannot be asserted on.
var backtestStderr io.Writer = os.Stderr

// errWindowNotClosed refuses a window whose end instant has not yet passed.
var errWindowNotClosed = errors.New("window has not closed yet")

// checkWindowClosed refuses a window that is still open on the right.
//
// A WINDOW MUST BE CLOSED BEFORE IT IS READ, and this is a hard error rather
// than a warning because the failure it prevents is silent. /trades keeps
// returning new records until the window's end instant has passed, so a run
// started inside the window writes a file that looks complete, carries no mark
// saying otherwise, and disagrees with the same command run an hour later. A
// warning would be printed once into a terminal and lost; the file would
// survive and be cited. There is deliberately no override, because every
// override of this check produces exactly the file it exists to prevent.
//
// The comparison is against the END INSTANT and not the end day. -to is
// exclusive, so -to=2026-09-01 covers up to 2026-08-31T23:59:59Z and becomes
// readable the moment 2026-09-01T00:00:00Z passes.
func checkWindowClosed(to, now time.Time) error {
	if !to.After(now) {
		return nil
	}
	// The latest end instant that would be accepted is the most recent midnight
	// that has already passed, which is today's date in UTC.
	latest := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	return fmt.Errorf("%w: -to %s ends at %s, which is still in the future at %s. "+
		"Reading it now would write a partial window that looks complete. The latest -to that would be accepted is %s",
		errWindowNotClosed,
		to.Format(dayLayout),
		to.Format(time.RFC3339),
		now.Format(time.RFC3339),
		latest.Format(dayLayout))
}

func runBacktest(args []string) error {
	fs := flag.NewFlagSet("backtest", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	pairsPath := fs.String("pairs", "", "path to a pair list, required. scripts/record-pairs.example.json holds USTRY/USDC")
	fromDay := fs.String("from", "", "first UTC day in the window, YYYY-MM-DD, required")
	toDay := fs.String("to", "", "first UTC day AFTER the window, YYYY-MM-DD, required")
	markDay := fs.String("mark", "", "a UTC day to report every crossing relative to, YYYY-MM-DD. Optional")
	fromLedger := fs.Uint("from-ledger", 0,
		"ledger to seek the walk to. 0 walks the pair's whole history. Being early costs requests; being late silently drops trades")
	out := fs.String("out", "docs/evidences", "directory the CSV files are written to")
	baseURL := fs.String("horizon", horizon.DefaultBaseURL, "Horizon base URL")
	budget := fs.Int("budget", 3000, "requests permitted per hour. Public Horizon allows about 3600 per IP")

	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `keel backtest - the trade-implied history of a pair, as CSV

Writes two files per pair into -out:

  <pair>-trades-<from>_<to>.csv   one row per trade, every field as Horizon sent it
  <pair>-daily-<from>_<to>.csv    one row per UTC day, derived from those trades

The daily file carries an UPPER BOUND on depth and never a measurement. See
docs/methodology/01-data-sources.md section 6. Two kinds are reported in separate
columns: within-leg, whose cause is established, and between-legs, which assumes the
book did not change in the gap. Read the within-leg columns first.

A bound belongs to the day it was observed on and is never carried forward,
because liquidity added the next day would make a carried bound false.

`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *pairsPath == "" {
		return errors.New("backtest: -pairs is required")
	}
	from, err := time.ParseInLocation(dayLayout, *fromDay, time.UTC)
	if err != nil {
		return fmt.Errorf("backtest: -from must be YYYY-MM-DD: %w", err)
	}
	to, err := time.ParseInLocation(dayLayout, *toDay, time.UTC)
	if err != nil {
		return fmt.Errorf("backtest: -to must be YYYY-MM-DD: %w", err)
	}
	if !to.After(from) {
		return fmt.Errorf("backtest: -to %s is not after -from %s", *toDay, *fromDay)
	}
	// BEFORE ANY NETWORK CALL. Refusing here rather than after the walk means a
	// rejected run costs nothing and, more to the point, cannot leave a partial
	// file behind on the way out.
	if err := checkWindowClosed(to, backtestNow()); err != nil {
		return fmt.Errorf("backtest: %w", err)
	}
	var mark time.Time
	if *markDay != "" {
		if mark, err = time.ParseInLocation(dayLayout, *markDay, time.UTC); err != nil {
			return fmt.Errorf("backtest: -mark must be YYYY-MM-DD: %w", err)
		}
	}

	pairs, err := horizon.LoadPairs(*pairsPath)
	if err != nil {
		return fmt.Errorf("backtest: %w", err)
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return fmt.Errorf("backtest: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client := horizon.NewClient(horizon.Config{BaseURL: *baseURL, Budget: *budget})

	for _, p := range pairs {
		if err := backtestPair(ctx, client, p, window{from: from, to: to, mark: mark},
			uint32(*fromLedger), *out, os.Stdout, backtestStderr); err != nil {
			return fmt.Errorf("backtest %s: %w", p, err)
		}
	}
	return nil
}

// window is the reporting period, in whole UTC days. to is EXCLUSIVE.
type window struct {
	from, to time.Time
	mark     time.Time // zero when no marker was given
}

func (w window) contains(t time.Time) bool {
	return !t.Before(w.from) && t.Before(w.to)
}

func backtestPair(ctx context.Context, c *horizon.Client, p horizon.Pair, w window,
	fromLedger uint32, outDir string, log, warn io.Writer) error {

	// The walk stops on ledger_close_time, which is the only clock this endpoint
	// has. It does NOT stop on a cursor computed from w.to, because converting a
	// time into a ledger sequence is what rule 4 of 00-overview.md forbids.
	reading, err := c.Trades(ctx, p.Base, p.Quote, horizon.TradeQuery{
		FromLedger: fromLedger,
		StopAfter:  func(t domain.Trade) bool { return !t.ClosedAt.Before(w.to) },
	})
	if err != nil {
		return err
	}

	inWindow := make([]domain.Trade, 0, len(reading.Trades))
	for _, t := range reading.Trades {
		if w.contains(t.ClosedAt) {
			inWindow = append(inWindow, t)
		}
	}

	fmt.Fprintf(log, "%s\n  %d trades in [%s, %s), from %d pages, %d records read, stopped=%t truncated=%t\n",
		p, len(inWindow), w.from.Format(dayLayout), w.to.Format(dayLayout),
		reading.Pages, len(reading.Trades), reading.Stopped, reading.Truncated)

	if !reading.Stopped && !reading.Truncated {
		// The walk ran off the end of the collection rather than past the window.
		// The window is then open on the right and the last day may be partial.
		fmt.Fprintf(log, "  NOTE: the pair's trade history ended before %s, so the window is not closed on the right\n",
			w.to.Format(dayLayout))
	}
	if reading.Truncated {
		fmt.Fprintf(log, "  WARNING: the walk hit its page cap. This file is INCOMPLETE\n")
	}
	if !reading.Stopped {
		// THIS IS A WARNING AND MUST NOT BECOME AN ERROR. A pair whose trading
		// genuinely ended before the window trips this on every run, forever: no
		// later read can produce a trade at or after the window end when none
		// will ever exist. Promoting it to a non-zero exit would make dead pairs
		// unrecordable, which is a worse failure than the one it prevents.
		//
		// It goes to STDERR rather than to the per-pair report above, because
		// that report is routinely piped and read on its own, and this has to
		// survive being separated from it. It was a sidecar field before, read
		// correctly by two people and dismissed by both; what was missing was
		// prominence, not information.
		warnWindowEndUnproven(warn, p, w, maxClosedAt(inWindow))
	}
	if len(inWindow) > 0 && fromLedger > 0 {
		// The one failure this command cannot detect from its own output is a
		// -from-ledger that seeks INSIDE the window, which silently drops the
		// trades before it. Printing where the window actually starts is what
		// lets a reader check that it did not happen.
		fmt.Fprintf(log, "  seeked to ledger %d; the first trade inside the window is %s\n",
			fromLedger, inWindow[0].ClosedAt.Format(time.RFC3339))
	}

	slug := p.Slug()
	stamp := w.from.Format(dayLayout) + "_" + w.to.Format(dayLayout)

	tradesPath := filepath.Join(outDir, slug+"-trades-"+stamp+".csv")
	if err := writeTradesCSV(tradesPath, inWindow); err != nil {
		return err
	}
	fmt.Fprintf(log, "  wrote %s\n", tradesPath)

	metaPath := filepath.Join(outDir, slug+"-trades-"+stamp+".meta.txt")
	if err := writeTradesMeta(metaPath, w, inWindow, reading); err != nil {
		return err
	}
	fmt.Fprintf(log, "  wrote %s (max closed_at %s)\n", metaPath, formatClosedAt(maxClosedAt(inWindow)))

	bounds := domain.TradeImpliedDepthBounds(inWindow)
	params := domain.DefaultParams()
	rows := dailyRows(inWindow, bounds, params)

	dailyPath := filepath.Join(outDir, slug+"-daily-"+stamp+".csv")
	if err := writeDailyCSV(dailyPath, rows, params); err != nil {
		return err
	}
	fmt.Fprintf(log, "  wrote %s\n", dailyPath)

	summarise(log, rows, params, w)
	return nil
}

// ---------------------------------------------------------------- the daily row

// observed is one rung's tightest bound on one day, of one kind.
type observed struct {
	bound   decimal.Decimal
	source  string        // the trade the bound came from, so a reader can fetch it
	elapsed time.Duration // meaningful only for the across-operations kind
}

// dailyRow is one UTC day. A rung with nothing to say is ABSENT from the maps,
// and absent means unobserved, which is not the same claim as zero.
type dailyRow struct {
	Day    time.Time
	Trades int

	VolumeQuote decimal.Decimal
	PriceOpen   decimal.Decimal
	PriceClose  decimal.Decimal
	PriceLow    decimal.Decimal
	PriceHigh   decimal.Decimal

	// MaxDelta is the largest price move seen that day of each kind, as a
	// fraction. Nil when nothing moved.
	MaxDeltaWithin  *decimal.Decimal
	MaxDeltaBetween *decimal.Decimal

	// Within and Across hold the tightest bound OBSERVED THAT DAY per rung,
	// keyed by the rung's delta as a string. Never carried forward from an
	// earlier day: liquidity added overnight would make a carried bound false.
	Within map[string]observed
	Across map[string]observed
}

// rungs are the deltas a bound is reported at: the three market rungs of the
// depth ladder, plus the critical delta the manipulation flag is judged on. All
// four come from Params and none is written here.
func rungs(p domain.Params) []decimal.Decimal {
	out := append([]decimal.Decimal{}, p.MarketDeltas...)
	out = append(out, p.ManipulationCriticalDelta)
	sort.Slice(out, func(i, j int) bool { return out[i].LessThan(out[j]) })
	return out
}

func dayOf(t time.Time) (string, time.Time) {
	d := t.UTC().Truncate(24 * time.Hour)
	return d.Format(dayLayout), d
}

func dailyRows(trades []domain.Trade, bounds []domain.TradeImpliedBound, p domain.Params) []dailyRow {
	byDay := map[string]*dailyRow{}

	for _, t := range trades {
		k, d := dayOf(t.ClosedAt)
		r, ok := byDay[k]
		if !ok {
			r = &dailyRow{Day: d, PriceOpen: t.Price.Decimal(),
				Within: map[string]observed{}, Across: map[string]observed{}}
			byDay[k] = r
		}
		price := t.Price.Decimal()
		r.Trades++
		r.VolumeQuote = r.VolumeQuote.Add(t.CounterAmount)
		r.PriceClose = price
		if r.PriceLow.IsZero() || price.LessThan(r.PriceLow) {
			r.PriceLow = price
		}
		if price.GreaterThan(r.PriceHigh) {
			r.PriceHigh = price
		}
	}

	rs := rungs(p)
	for _, b := range bounds {
		// A bound is dated by the trade that ENDS the span, because that is the
		// moment the market was in the state the bound describes.
		k, _ := dayOf(b.To.ClosedAt)
		r, ok := byDay[k]
		if !ok {
			continue
		}

		into, maxDelta := r.Across, &r.MaxDeltaBetween
		if b.Kind == domain.BoundWithinLeg {
			into, maxDelta = r.Within, &r.MaxDeltaWithin
		}
		if *maxDelta == nil || b.Delta.GreaterThan(**maxDelta) {
			d := b.Delta
			*maxDelta = &d
		}
		// A bound reaches every rung at or below its own delta. The monotonicity
		// argument is in domain.TightestBoundAtLeast and is not restated here.
		for _, rung := range rs {
			if b.Delta.LessThan(rung) {
				continue
			}
			key := rung.String()
			cur, seen := into[key]
			if !seen || b.Bound.LessThan(cur.bound) {
				into[key] = observed{bound: b.Bound, source: b.To.ID, elapsed: b.Elapsed}
			}
		}
	}

	out := make([]dailyRow, 0, len(byDay))
	for _, r := range byDay {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Day.Before(out[j].Day) })
	return out
}

// ---------------------------------------------------------------- the files

func writeTradesCSV(path string, trades []domain.Trade) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	w := csv.NewWriter(f)
	header := []string{
		"trade_id", "operation_id", "fill_index", "ledger_seq", "closed_at", "trade_type",
		"price_n", "price_d", "price_quote_per_base",
		"base_amount", "counter_amount",
		"base_is_seller",
		"base_account", "counter_account", "base_offer_id", "counter_offer_id", "liquidity_pool_id",
		// APPENDED AT THE END, positions 18 and 19, rather than beside
		// liquidity_pool_id. Columns 1..17 keep their names and their positions, so
		// a consumer indexing by position reads the same field it read before. See
		// docs/evidences/2026-08-31-trade-pool-id-defect.md section 7.
		"liquidity_pool_side", "liquidity_pool_fee_bp",
	}
	if err := w.Write(header); err != nil {
		return err
	}
	for _, t := range trades {
		rec := []string{
			t.ID,
			t.OperationID,
			fmt.Sprintf("%d", t.FillIndex),
			fmt.Sprintf("%d", t.LedgerSeq),
			t.ClosedAt.UTC().Format(time.RFC3339),
			t.Type,
			fmt.Sprintf("%d", t.Price.N),
			fmt.Sprintf("%d", t.Price.D),
			t.Price.Decimal().String(),
			t.BaseAmount.String(),
			t.CounterAmount.String(),
			fmt.Sprintf("%t", t.BaseIsSeller),
			t.BaseAccount, t.CounterAccount, t.BaseOfferID, t.CounterOfferID, t.LiquidityPoolID,
			t.LiquidityPoolSide, fmt.Sprintf("%d", t.LiquidityPoolFeeBP),
		}
		if err := w.Write(rec); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

// warnWindowEndUnproven reports that the walk never saw past the window end.
//
// WHAT IT IS ACTUALLY CLAIMING. The walk ends either because it saw a trade at
// or after the window end, or because it ran out of records. Only the first
// proves Horizon's index had reached the end of the window. The second is
// consistent with a pair that stopped trading AND with an index that is simply
// behind, and the walk cannot tell those apart from the inside.
//
// The wording is deliberately flat. An earlier form of this fact was read by
// two people who called it harmless, so nothing here offers the reader a way to
// agree with themselves and move on: it states what is not proven, states that
// the row count is a floor rather than a total, and ends on the one action that
// settles it.
func warnWindowEndUnproven(warn io.Writer, p horizon.Pair, w window, maxClosed time.Time) {
	fmt.Fprintf(warn, "WARNING: the window end is UNPROVEN for %s\n", p)
	fmt.Fprintf(warn, "  window ends at:   %s\n", w.to.UTC().Format(time.RFC3339))
	if maxClosed.IsZero() {
		fmt.Fprintf(warn, "  last trade seen:  none, the walk found no trade inside this window\n")
		fmt.Fprintf(warn, "  gap:              the whole window\n")
	} else {
		fmt.Fprintf(warn, "  last trade seen:  %s\n", maxClosed.UTC().Format(time.RFC3339))
		fmt.Fprintf(warn, "  gap:              %s of the window has no trade in this file\n",
			w.to.Sub(maxClosed).Round(time.Second))
	}
	fmt.Fprintf(warn, "  The walk reached the end of Horizon's records without seeing a trade at or\n")
	fmt.Fprintf(warn, "  after the window end, so nothing here shows the index had caught up. Trades\n")
	fmt.Fprintf(warn, "  that had already closed inside this window can still be missing from it. The\n")
	fmt.Fprintf(warn, "  row count is a floor, not a total, and every number derived from it inherits\n")
	fmt.Fprintf(warn, "  that.\n")
	fmt.Fprintf(warn, "  NEXT: re-run this exact window later and compare the row count. If it grew,\n")
	fmt.Fprintf(warn, "  this file was incomplete and whatever was computed from it must be redone.\n")
}

// maxClosedAt is the latest ledger close time in a set of trades, or the zero
// time when the set is empty. Trades arrive ascending, but this does not assume
// it: the cost of a scan is nothing and an assumption here would be silent.
func maxClosedAt(trades []domain.Trade) time.Time {
	var max time.Time
	for _, t := range trades {
		if t.ClosedAt.After(max) {
			max = t.ClosedAt
		}
	}
	return max
}

// formatClosedAt renders a close time, or "none" for the zero time. An empty
// window has no maximum and must not be reported as year 1.
func formatClosedAt(t time.Time) string {
	if t.IsZero() {
		return "none"
	}
	return t.UTC().Format(time.RFC3339)
}

// writeTradesMeta writes the sidecar that says how far the CSV beside it
// actually reaches.
//
// IT IS A SIDECAR AND NOT A COMMENT LINE IN THE CSV, because the one consumer
// of these files, scripts/funding-graph-probe.sh, reads them with Python's
// csv.DictReader, which has no notion of a comment. A leading '#' line is taken
// as the header row, and every subsequent lookup raises KeyError. Adding
// provenance inside the file would therefore have broken the reader that exists
// in order to record provenance.
//
// EVERY FIELD IS DERIVED FROM THE DATA AND NONE FROM THE WALL CLOCK. A closed
// window re-read tomorrow produces a byte-identical sidecar, so a diff against
// it means the underlying records changed, which is the only thing worth being
// told. A generated_at stamp would have made every re-read differ and the
// signal would be gone.
//
// stopped_past_window is the field that matters most, and it is here because
// max_closed_at cannot answer the question on its own. The walk excludes the
// trade that tripped its stop, so its last record is always the last in-window
// one and a walk-wide maximum would be a second copy of the same number. True
// means the walk did see a trade at or after the window's end and therefore
// covers the whole of it. False means it ran off the end of the pair's history
// instead, so the last day may be partial even though the window has closed.
func writeTradesMeta(path string, w window, trades []domain.Trade, reading horizon.TradeReading) error {
	var b []byte
	add := func(format string, args ...any) {
		b = append(b, fmt.Sprintf(format, args...)...)
	}

	add("# Provenance for %s\n", filepath.Base(path[:len(path)-len(".meta.txt")]+".csv"))
	add("# Written beside the CSV rather than inside it: csv.DictReader reads a\n")
	add("# leading '#' line as the header. Compare max_closed_at_utc against\n")
	add("# window_to_utc to see how far the file actually reaches.\n")
	add("window_from_utc: %s\n", w.from.UTC().Format(time.RFC3339))
	add("window_to_utc: %s\n", w.to.UTC().Format(time.RFC3339))
	add("rows: %d\n", len(trades))
	add("min_closed_at_utc: %s\n", formatClosedAt(minClosedAt(trades)))
	add("max_closed_at_utc: %s\n", formatClosedAt(maxClosedAt(trades)))
	add("stopped_past_window: %t\n", reading.Stopped)
	add("truncated: %t\n", reading.Truncated)

	return os.WriteFile(path, b, 0o644)
}

// minClosedAt is the earliest close time in a set of trades, or the zero time
// when the set is empty. It is here so the sidecar can state both ends of what
// the file covers rather than only the far one.
func minClosedAt(trades []domain.Trade) time.Time {
	var min time.Time
	for _, t := range trades {
		if min.IsZero() || t.ClosedAt.Before(min) {
			min = t.ClosedAt
		}
	}
	return min
}

func writeDailyCSV(path string, rows []dailyRow, p domain.Params) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	rs := rungs(p)
	fiveKey := rungKey(rs, "0.05")
	critKey := p.ManipulationCriticalDelta.String()

	w := csv.NewWriter(f)
	header := []string{
		"day", "trades", "volume_quote",
		"price_open", "price_close", "price_low", "price_high",
		"max_delta_within_leg", "max_delta_between_legs",
	}
	for _, r := range rs {
		header = append(header,
			"bound_within_leg_delta_"+r.String(), "bound_within_leg_source_"+r.String())
	}
	for _, r := range rs {
		header = append(header,
			"bound_between_legs_delta_"+r.String(), "bound_between_legs_source_"+r.String(),
			"bound_between_legs_gap_seconds_"+r.String())
	}
	header = append(header,
		"thin_depth_5pct_within_leg", "thin_depth_5pct_between_legs",
		"manipulation_cheap_within_leg", "manipulation_cheap_between_legs")
	if err := w.Write(header); err != nil {
		return err
	}

	for _, row := range rows {
		rec := []string{
			row.Day.Format(dayLayout),
			fmt.Sprintf("%d", row.Trades),
			row.VolumeQuote.String(),
			row.PriceOpen.String(), row.PriceClose.String(), row.PriceLow.String(), row.PriceHigh.String(),
			optional(row.MaxDeltaWithin), optional(row.MaxDeltaBetween),
		}
		for _, r := range rs {
			if o, ok := row.Within[r.String()]; ok {
				rec = append(rec, o.bound.String(), o.source)
			} else {
				rec = append(rec, "", "")
			}
		}
		for _, r := range rs {
			if o, ok := row.Across[r.String()]; ok {
				rec = append(rec, o.bound.String(), o.source, fmt.Sprintf("%d", int64(o.elapsed.Seconds())))
			} else {
				rec = append(rec, "", "", "")
			}
		}
		rec = append(rec,
			verdict(row.Within, fiveKey, p.Thresholds.ThinDepth5PctAbsolute),
			verdict(row.Across, fiveKey, p.Thresholds.ThinDepth5PctAbsolute),
			verdict(row.Within, critKey, p.Thresholds.ManipulationCheapAbsolute),
			verdict(row.Across, critKey, p.Thresholds.ManipulationCheapAbsolute),
		)
		if err := w.Write(rec); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

// rungKey finds a rung among the configured deltas. It is looked up rather than
// written as a literal so that a change to Params.MarketDeltas cannot leave a
// column silently pointing at a rung that is no longer there.
func rungKey(rs []decimal.Decimal, want string) string {
	target := decimal.RequireFromString(want)
	for _, r := range rs {
		if r.Equal(target) {
			return r.String()
		}
	}
	return ""
}

// verdict reports whether a day's observed bound is below a threshold.
//
// THREE STATES, NOT TWO, and this is 09-flags-and-bands.md section 2 applied to a
// bound. "yes" means a bound was observed that day and it is below the threshold.
// "no" means one was observed and it is not. EMPTY means no qualifying price move
// happened that day, so nothing was learned, which is not the same claim as safe.
func verdict(m map[string]observed, key string, threshold decimal.Decimal) string {
	if key == "" {
		return ""
	}
	o, ok := m[key]
	if !ok {
		return ""
	}
	if o.bound.LessThan(threshold) {
		return "yes"
	}
	return "no"
}

func optional(d *decimal.Decimal) string {
	if d == nil {
		return ""
	}
	return d.String()
}

// ---------------------------------------------------------------- the summary

// summarise prints the sentence Deliverable 2 asks for: when the threshold was
// crossed, relative to a date the caller named. It prints it twice, once per kind
// of bound, because the two answer different questions and on this data they give
// different dates.
func summarise(log io.Writer, rows []dailyRow, p domain.Params, w window) {
	rs := rungs(p)
	fiveKey := rungKey(rs, "0.05")
	critKey := p.ManipulationCriticalDelta.String()

	report := func(label, key string, threshold decimal.Decimal, pick func(dailyRow) map[string]observed) {
		if key == "" {
			return
		}
		var first *dailyRow
		var firstSource string
		days, obs := 0, 0
		for i := range rows {
			o, ok := pick(rows[i])[key]
			if !ok {
				continue
			}
			obs++
			if o.bound.LessThan(threshold) {
				days++
				if first == nil {
					first, firstSource = &rows[i], o.source
				}
			}
		}
		fmt.Fprintf(log, "    %-18s ", label)
		switch {
		case obs == 0:
			fmt.Fprintf(log, "never observed: no qualifying price move in the window, so nothing was learned\n")
			return
		case first == nil:
			fmt.Fprintf(log, "observed on %d of %d days and never below the threshold\n", obs, len(rows))
			return
		}
		fmt.Fprintf(log, "first crossed %s, below on %d of the %d days it was observed, source trade %s\n",
			first.Day.Format(dayLayout), days, obs, firstSource)
		if !w.mark.IsZero() {
			delta := int(first.Day.Sub(w.mark).Hours() / 24)
			fmt.Fprintf(log, "    %-18s ", "")
			switch {
			case delta < 0:
				fmt.Fprintf(log, "%d days BEFORE %s\n", -delta, w.mark.Format(dayLayout))
			case delta == 0:
				fmt.Fprintf(log, "the same day as %s\n", w.mark.Format(dayLayout))
			default:
				fmt.Fprintf(log, "%d days AFTER %s\n", delta, w.mark.Format(dayLayout))
			}
		}
	}

	within := func(r dailyRow) map[string]observed { return r.Within }
	across := func(r dailyRow) map[string]observed { return r.Across }

	fmt.Fprintf(log, "  thin depth at 5 percent, threshold %s in the quote asset\n", p.Thresholds.ThinDepth5PctAbsolute)
	report("within-leg:", fiveKey, p.Thresholds.ThinDepth5PctAbsolute, within)
	report("between-legs:", fiveKey, p.Thresholds.ThinDepth5PctAbsolute, across)

	fmt.Fprintf(log, "  manipulation cheap at delta %s, threshold %s in the quote asset\n",
		p.ManipulationCriticalDelta, p.Thresholds.ManipulationCheapAbsolute)
	report("within-leg:", critKey, p.Thresholds.ManipulationCheapAbsolute, within)
	report("between-legs:", critKey, p.Thresholds.ManipulationCheapAbsolute, across)
}
