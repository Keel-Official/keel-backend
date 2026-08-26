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
			uint32(*fromLedger), *out, os.Stdout); err != nil {
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
	fromLedger uint32, outDir string, log *os.File) error {

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
		}
		if err := w.Write(rec); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
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
func summarise(log *os.File, rows []dailyRow, p domain.Params, w window) {
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
