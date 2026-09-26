// `keel trades`: walk the trade stream of every pair backwards in whole UTC days,
// classify it, and cache the two halves of the trade-derived supporting metrics.
//
// WHY IT IS A SUBCOMMAND AND NOT PART OF `scan`, which is holders.go's argument
// with worse numbers. DEC-019 section 2 measures the demonstration set at
// 5,149,904 trades in 30 days, 25,782 pages, 8.6 hours at the NFR-6 cap of 3,000
// requests an hour. `scan` runs every fifteen minutes. Even one pair, XLM at
// 1,934,524 trades, does not fit inside a round. So this runs on its own cadence
// and `scan` reads the newest row it wrote.
//
// WHAT DEC-019 DECIDED, AND IT WAS AL RATHER THAN THIS FILE. Option B at a
// threshold of 20,000 trades in 30 days, accepted 18 September 2026. Pairs under
// the threshold get the whole 30 day window classified, which answers FR-9 and
// WASH_TRADE_SUSPECTED; pairs over it get the cheap half only and report the
// volume figures unevaluated, WITH a reason, which section 5 asks for by name.
// The case for the threshold is that cost and value run in opposite directions:
// XLM is the most expensive pair in the set and the one whose volume nobody
// suspects of being wash, while the two pairs that trade fewer than thirty times
// a month cost no full pages between them and are where an exclusion share
// actually says something.
//
// THE THRESHOLD IS READ FROM THE SELECTION NOTE, which is the same source
// DEC-019 section 7 reproduces its own arithmetic from. That figure was measured
// on Horizon on 26 August 2026 and is not refreshed here, so the gate is applied
// on a count that may be three weeks stale. Counting during the walk instead
// would be self-updating and would cost up to 100 pages per over-threshold pair
// before aborting, which is roughly forty minutes the accepted option does not
// include. The staleness is recorded in the row's reason string rather than
// hidden, and a pair whose note carries no count is treated as over the
// threshold: refusing to guess is the same rule the rest of this repository
// applies to a number nobody measured.
//
// WHAT THIS COMMAND WILL NOT DO:
//
//  1. IT WILL NOT CLASSIFY A PARTIAL DAY. DEC-019 section 8.1 and 8.2 are the
//     whole reason internal/horizon grew a second walk shape: condition 5 of the
//     genuine-trade rules reads the median of the trade's own UTC day, so a walk
//     stopping mid-day judges trades against a statistic that is not the one the
//     rule names, and the error runs towards "traded more recently than it did".
//     A staleness flag that can only fail to fire is not a staleness flag.
//
//  2. IT WILL NOT TURN A BUDGET FAILURE INTO A FINDING. A walk that ran out of
//     days without meeting a genuine trade stores no last genuine trade and says
//     `bound_reached`; a walk that reached the end of the pair's history and met
//     none stores the same absence and says `exhausted`. The first is unevaluated
//     and the second is the measurement that the pair has never genuinely traded.
//     Section 8.4 item 2 refuses to merge them and so does the schema.
//
//  3. IT WILL NOT DIVIDE BY SUPPLY. The volume-to-supply ratio needs circulating
//     supply, which belongs to the holder pull and moves on its own cadence, so
//     this command stores the genuine volume and `scan` performs the division
//     where both readings meet. Freezing one half of a fraction against a
//     denominator from another day is the failure DEC-018 point 2 describes for
//     the holder ledger.

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Keel-Official/keel-backend/internal/domain"
	"github.com/Keel-Official/keel-backend/internal/horizon"
	"github.com/Keel-Official/keel-backend/internal/store"
)

// tradeWindowDays is the span FR-9's widest window needs, and the default bound
// on a backward walk. Thirty days is not an arbitrary budget: it is the window
// NO_GENUINE_TRADE_30D and WASH_TRADE_SUSPECTED are defined over, so a walk that
// completes thirty whole days and finds nothing has MEASURED both of them rather
// than run out of road.
const tradeWindowDays = 30

// defaultTradePageCap bounds ONE pair's walk in Horizon pages.
//
// WHY A PAGE BOUND EXISTS BESIDE THE DAY BOUND. A day is cheap for a quiet pair
// and expensive for a busy one, so bounding the walk in days bounds its cost only
// while the two never combine. They combine: a pair is over the DEC-019 threshold
// because it is BUSY, and a pair filled from a liquidity pool has no genuine day
// at all under condition 4 of the genuine-trade rules, so a busy pool-filled pair
// pays the busy page rate for the whole thirty day span. HU/USDC did that on
// 18 September 2026, spending about 2,100 requests of a 3,000 hourly budget by
// itself and failing the forty pairs behind it in the same pass.
//
// WHY 400 AND NOT LESS. The busiest pair in the demonstration set is XLM at 64,484
// trades a day, which is 323 pages for ONE day. A cap under that would stop every
// busy pair inside its first day and guarantee it could never answer FR-10, which
// is refusing the question rather than bounding it. 400 buys the busiest pair its
// newest day and a quiet pair its whole month, because a quiet pair's month is a
// dozen pages.
//
// It is a flag for the reason the threshold and the day bound are flags: the
// mechanism is decided here and the number is Al's to move.
const defaultTradePageCap = 400

// tradeCountRe reads the 30 day trade count out of a selection note. It is the
// regex in DEC-019 section 7, kept identical so the gate and the record's own
// arithmetic cannot drift apart.
var tradeCountRe = regexp.MustCompile(`(\d+) trades in 30 days`)

func runTrades(args []string) error {
	fs := flag.NewFlagSet("trades", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	dsn := fs.String("dsn", envOr(envDSN, store.DefaultDSN), "Postgres DSN, or set KEEL_DSN")
	baseURL := fs.String("horizon", horizon.DefaultBaseURL, "Horizon base URL")
	budget := fs.Int("budget", 3000, "requests permitted per hour. Public Horizon allows about 3600 per IP")
	threshold := fs.Int("threshold", 20000,
		"classify the whole 30 day window for pairs at or below this many trades in 30 days. DEC-019 option B")
	maxDays := fs.Int("max-days", tradeWindowDays,
		"bound one backward walk at this many whole UTC days. Reaching it reports unevaluated, never 'no genuine trade'")
	maxPages := fs.Int("max-pages", defaultTradePageCap,
		"bound one backward walk at this many Horizon pages, so no single pair can spend the pass's budget. 0 removes the bound")
	refresh := fs.Bool("refresh", false,
		"walk a pair that already has a reading for today. Off by default, so a pass resumes where a spent budget stopped it")
	only := fs.String("pair", "", "walk one pair only, as BASECODE:ISSUER. Empty means every pair in the set")
	interval := fs.Duration("interval", 0,
		"repeat the pass on this cadence instead of exiting. 0 runs once. 24h is what a deployment wants")
	alignUTC := fs.Duration("align-utc", 5*time.Minute,
		"with -interval 24h, run each later pass at this offset after 00:00 UTC instead of 24h after the process started. Negative keeps the start-relative cadence")

	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `keel trades - cache the trade half of the supporting metrics

  keel trades                              every pair in the demonstration set
  keel trades -pair USTRY:GCRYUGD5...      one pair
  keel trades -threshold 5000              a cheaper gate than DEC-019's

Reads the pair list from the assets table, so `+"`keel assets -pairs`"+` has to have
run first. Writes one row per pair per UTC day into trade_readings, and opens a
run row of kind "trades" around the whole pass.

The walk is BACKWARD and its unit is a whole UTC day, because the genuine-trade
rules read the median of a trade's own day. The current day is never walked: every
figure here describes the last COMPLETE day and is up to 24 hours stale by
construction, which is the direction a staleness flag may err in.

`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *maxDays <= 0 {
		return fmt.Errorf("trades: -max-days is %d; a walk of no days measures nothing", *maxDays)
	}

	logger := log.New(os.Stdout, "", log.LstdFlags|log.LUTC)

	// SIGINT and SIGTERM cancel rather than kill, so a pass in flight finishes
	// its current pair and closes its run row. Same reasoning as holders.go: a
	// process killed outright leaves an open run row, which runs.go reads as a
	// job that died, and every redeploy would then look like a crash.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	s, err := openStore(ctx, *dsn)
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()

	if applied, err := s.SchemaVersion(ctx); err != nil {
		return fmt.Errorf("trades: reading schema_migrations: %w\n  hint: run make migrate", err)
	} else if len(applied) == 0 {
		return errors.New("trades: no migrations are applied; run make migrate")
	}

	rows, err := s.Assets(ctx, true)
	if err != nil {
		return fmt.Errorf("trades: %w", err)
	}
	if len(rows) == 0 {
		return errors.New("trades: the demonstration set is empty; declare it with `keel assets -pairs <file>`")
	}
	targets, err := tradeTargets(rows, *only)
	if err != nil {
		return fmt.Errorf("trades: %w", err)
	}

	client := horizon.NewClient(horizon.Config{
		BaseURL: *baseURL,
		Budget:  *budget,
		// A full-window walk of a busy pair is larger than one budget window,
		// so this pass waits for the budget rather than failing the walk. The
		// cost is time, and every pair still finishes inside its day.
		WaitOnBudget: true,
		// No cache. A second walk that is identical because a body was reused
		// says nothing about the trade stream, which is the recorder's argument
		// and applies unchanged here.
		CacheTTL: 0,
	})

	cfg := tradePassConfig{Threshold: *threshold, MaxDays: *maxDays, MaxPages: *maxPages, Refresh: *refresh}
	logger.Printf("%d pair(s), methodology %s, threshold %d trades in 30 days, bound %d day(s) and %d page(s)",
		len(targets), domain.MethodologyVersion, cfg.Threshold, cfg.MaxDays, cfg.MaxPages)

	if *interval <= 0 {
		return tradesPass(ctx, s, client, targets, cfg, logger)
	}

	logger.Printf("interval %s, Ctrl-C to stop", *interval)
	for {
		if err := tradesPass(ctx, s, client, targets, cfg, logger); err != nil {
			return err
		}
		next := nextTradesPass(time.Now().UTC(), *interval, *alignUTC)
		logger.Printf("next pass at %s", next.Format(time.RFC3339))
		timer := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			timer.Stop()
			logger.Print("stopped")
			return nil
		case <-timer.C:
		}
	}
}

// nextTradesPass is when the pass after one that finished at now should start.
//
// WHY A DAILY PASS IS PINNED TO THE CLOCK AND NOT TO THE PROCESS. A reading is
// anchored at 00:00Z of the day its pass ran, and `keel scan` refuses one older
// than -max-trade-age, 36 hours, measured from that anchor. A pass that runs 24
// hours after the process STARTED lands wherever the last deploy happened to put
// it: started at 17:00Z, the reading anchored at midnight ages past 36 hours at
// 12:00Z the next day and the replacement arrives at 17:00Z, so every pair read
// its trade figures as null for five hours a day. That is what `verify-sow.sh`
// reported on 22 September 2026 as "trade-derived metrics absent in production".
// Pinned to a few minutes after midnight, a reading is replaced about 24 hours
// after its anchor, well inside the bound, whenever the process was started.
//
// THE THREE SENTENCES. The decision: align only the 24-hour cadence, at a small
// offset after 00:00Z so the day being walked has closed on every Horizon node.
// The alternative rejected: raising -max-trade-age to 48 hours, which removes the
// gap by admitting a reading whose window ended nearly three days ago, the exact
// staleness scan.go's comment on that flag refuses. Why: the bound is right and
// the schedule was wrong, so the schedule is what moves.
//
// Any other interval, or a negative offset, keeps the start-relative cadence it
// always had. The first pass still runs at start, so a fresh deploy fills the
// cache without waiting for midnight.
func nextTradesPass(now time.Time, interval, alignUTC time.Duration) time.Time {
	now = now.UTC()
	if interval != 24*time.Hour || alignUTC < 0 {
		return now.Add(interval)
	}
	next := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).Add(alignUTC)
	for !next.After(now) {
		next = next.Add(24 * time.Hour)
	}
	return next
}

// tradePassConfig is what DEC-019 left as numbers rather than mechanism.
type tradePassConfig struct {
	// Threshold is the 30 day trade count at or below which the expensive half
	// runs. Section 6 item 1 is explicit that 20,000 is a row in a table a reader
	// can move, which is why it is a flag and not a constant.
	Threshold int

	// MaxDays bounds one backward walk in whole UTC days.
	MaxDays int

	// MaxPages bounds the same walk in Horizon pages, so that one pair cannot
	// spend the pass's whole request budget. See defaultTradePageCap.
	MaxPages int

	// Refresh takes today's reading again for a pair that already has one,
	// instead of leaving it alone. Off by default, which is what lets a pass
	// resume where a spent budget stopped it.
	Refresh bool
}

// tradesPass is one walk over every target, opening and closing its own run row.
//
// One run row PER PASS and not per process, for the reason holdersPass gives: a
// long-lived service that opened a single row would report one run that never
// finishes, which is what an unfinished row is used to mean.
func tradesPass(
	ctx context.Context,
	s *store.Store,
	client *horizon.Client,
	targets []store.Asset,
	cfg tradePassConfig,
	logger *log.Logger,
) error {
	started := time.Now().UTC()
	runID, err := s.StartRun(ctx, store.RunTrades, started)
	if err != nil {
		return fmt.Errorf("trades: %w", err)
	}

	// The anchor is fixed ONCE for the pass rather than per pair. A pass that
	// straddles midnight would otherwise write rows against two anchors, and the
	// two halves of the set would describe windows that do not line up.
	anchor := startOfUTCDay(started)

	var ok, failed, full, cheap, withGenuine, already int
	for _, a := range targets {
		if err := ctx.Err(); err != nil {
			break
		}

		// A PAIR ALREADY READ TODAY IS NOT READ AGAIN, and this is what makes the
		// pass resumable rather than merely idempotent. SaveTradeReading already
		// refuses a second row for the same (pair, anchor, methodology), so a
		// re-run was always safe; it was not CHEAP, because the walk ran in full
		// before the store declined it. A pass that dies at pair 24 of 64, which
		// is what the spent request budget did on 18 September 2026, then had to
		// buy those 24 walks again to reach pair 25, and there is no budget to buy
		// them with: that is the same starvation arriving a second time.
		//
		// The daily cadence is untouched because the anchor moves with the day.
		// -refresh is the way to take the same day's reading again on purpose.
		if !cfg.Refresh {
			if existing, err := s.TradeReadingAt(ctx, a.ID, anchor, domain.MethodologyVersion); err == nil {
				already++
				logger.Printf("have  %s/%s %s, read earlier today -> id=%d",
					a.Base, a.Quote, existing.Scope, existing.ID)
				continue
			} else if !errors.Is(err, store.ErrNotFound) {
				// Reported and not fatal. Failing the pass because a cache lookup
				// hiccuped would cost every pair behind this one.
				logger.Printf("warn  %s/%s: checking for today's reading: %v", a.Base, a.Quote, err)
			}
		}

		r, why, err := pullTrades(ctx, client, a, anchor, cfg, runID)
		if err != nil {
			failed++
			logger.Printf("fail  %s/%s: %v", a.Base, a.Quote, err)
			continue
		}
		ok++
		if r.Scope == store.ScopeFullWindow {
			full++
		} else {
			cheap++
		}
		if r.LastGenuine != nil {
			withGenuine++
		}

		id, inserted, err := s.SaveTradeReading(ctx, r)
		if err != nil {
			failed++
			ok--
			logger.Printf("fail  %s/%s: storing: %v", a.Base, a.Quote, err)
			continue
		}
		logger.Printf("%-5s %s/%s %s over %d day(s), %d page(s), %s -> id=%d",
			map[bool]string{true: "store", false: "skip"}[inserted],
			a.Base, a.Quote, r.Scope, r.DaysWalked, r.Pages, why, id)
	}

	note := fmt.Sprintf(
		"anchor %s; %d full-window, %d last-genuine-only, %d with a genuine trade, %d already read today; %d requests, %d throttled",
		anchor.Format("2006-01-02"), full, cheap, withGenuine, already, client.Requests(), client.Throttled())
	if err := s.FinishRun(ctx, runID, time.Now().UTC(), ok, failed, note); err != nil {
		return fmt.Errorf("trades: %w", err)
	}
	logger.Printf("pass: %d ok, %d failed; %s", ok, failed, note)
	return nil
}

// pullTrades walks one pair and reduces it to the row that will be stored. The
// second return is one phrase for the log, saying which road was taken.
func pullTrades(
	ctx context.Context,
	client *horizon.Client,
	a store.Asset,
	anchor time.Time,
	cfg tradePassConfig,
	runID int64,
) (store.TradeReading, string, error) {
	rules := domain.DefaultGenuineRules()
	params := domain.DefaultParams()

	count, counted := tradesIn30Days(a.SelectionNote)
	full := counted && count <= cfg.Threshold

	// The expensive half needs every day of the window, so it cannot stop early.
	// The cheap half stops at the first COMPLETE day holding a genuine trade,
	// which is the whole of its economy.
	//
	// THE CHEAP HALF KEEPS ONE DAY AND NEVER THE WHOLE WALK, and this is a memory
	// bound rather than a tidiness. Its answer comes from the day that holds the
	// genuine trade, so every earlier day can be classified and dropped. Holding
	// them would be unbounded in exactly the case the cheap half exists for: a
	// pair over the threshold that has no genuine day for weeks. XLM serves
	// 64,484 trades a day, and thirty of those retained at once is the shape of
	// failure the holder pull already hit on this host, recorded in
	// docker-compose.prod.yml. GROG's reading on 18 September 2026 is the evidence
	// that "no genuine day for weeks" is a real state and not a hypothetical: its
	// trades are pool fills with no contemporaneous book, so condition 4 declines
	// to judge them and thirty days pass without one.
	//
	// THE FULL HALF KEEPS AT MOST THREE DAYS TOO, since 26 September 2026. It
	// used to hold the whole window and classify it in one call, which the
	// threshold kept small; fullWindow in tradewindow.go classifies each day as
	// its older neighbor arrives, so the threshold can rise without the memory
	// rising with it.
	var genuineDay []domain.Trade
	stoppedOnGenuine := false
	fw := newFullWindow(a.Base, rules, anchor, params.OracleWindow)
	walk, err := client.WalkTradeDays(ctx, a.Base, a.Quote,
		horizon.TradeDayQuery{Anchor: anchor, MaxDays: cfg.MaxDays, MaxPages: cfg.MaxPages},
		func(d horizon.TradeDay) (bool, error) {
			if full {
				fw.add(d)
				return true, nil
			}
			cs := domain.ClassifyTrades(d.Trades, a.Base, rules)
			if domain.LastGenuineTrade(cs, anchor) != nil {
				genuineDay = d.Trades
				stoppedOnGenuine = true
				return false, nil
			}
			return true, nil
		})
	if err != nil {
		return store.TradeReading{}, "", err
	}

	r := store.TradeReading{
		AssetID:            a.ID,
		RunID:              &runID,
		FetchedAt:          walk.ReadAt,
		MethodologyVersion: domain.MethodologyVersion,
		Anchor:             anchor,
		LedgerSeq:          walk.LedgerSeq,
		DaysWalked:         walk.Days,
		Pages:              walk.Pages,
		Exhausted:          walk.Exhausted,
		BoundReached:       walk.BoundReached,
	}

	// The full half has classified every day as the walk went. The cheap half
	// classifies only the day it stopped on, which is the day the answer is in and
	// is already complete: the walk never emits a partial one.
	if full {
		fw.finish()
		r.LastGenuine = fw.last
	} else {
		cs := domain.ClassifyTrades(genuineDay, a.Base, rules)
		r.LastGenuine = domain.LastGenuineTrade(cs, anchor)
	}

	// COVERAGE IS NOT THE SAME QUESTION AS THE THRESHOLD. A pair under the
	// threshold whose walk was cut short by the bound has no complete window
	// either, and storing its partial sums as a measured volume is the error the
	// whole record is about. So the volume half needs BOTH permissions.
	covered := walk.Exhausted || walk.Days >= tradeWindowDays
	switch {
	case full && covered:
		r.Scope = store.ScopeFullWindow
		r.TradesExcludedPct = fw.excludedPct()
		d1, d7, d30 := fw.baseD1, fw.baseD7, fw.baseD30
		r.GenuineBaseD1, r.GenuineBaseD7, r.GenuineBaseD30 = &d1, &d7, &d30

		quote, recorded := fw.oracleQuote, fw.oracleRecorded
		r.GenuineQuoteOracleWindow, r.OracleWindowRecorded = &quote, &recorded

		return r, fmt.Sprintf("full window, %d trades in 30 days at the 26 August count", count), nil

	case full && !covered && walk.PageCapReached:
		r.Scope = store.ScopeLastGenuineOnly
		r.VolumeUnevaluatedReason = fmt.Sprintf(
			"the 30 day window was not reached: the walk spent its %d page bound after %d day(s)",
			cfg.MaxPages, walk.Days)
	case full && !covered:
		r.Scope = store.ScopeLastGenuineOnly
		r.VolumeUnevaluatedReason = fmt.Sprintf(
			"the 30 day window was not reached: the walk covered %d day(s) and stopped at the bound", walk.Days)
	case counted:
		r.Scope = store.ScopeLastGenuineOnly
		r.VolumeUnevaluatedReason = fmt.Sprintf(
			"above the DEC-019 threshold of %d trades in 30 days: %d, counted on Horizon on 26 August 2026",
			cfg.Threshold, count)
	default:
		r.Scope = store.ScopeLastGenuineOnly
		r.VolumeUnevaluatedReason =
			"the selection note carries no 30 day trade count, so the DEC-019 threshold cannot be applied"
	}

	why := r.VolumeUnevaluatedReason
	switch {
	case stoppedOnGenuine:
		why = "last genuine trade found; " + why
	case walk.PageCapReached && walk.Days == 0:
		// THE ANCHOR'S OWN DAY IS PURE COST, and for a bursty pair it can be the
		// whole bill. Its trades are fetched and discarded, because a partial day
		// cannot be classified, and the walk cannot skip past them: seeking to a
		// day boundary means turning a time into a ledger, which 00-overview
		// section 2 rule 4 forbids. TGM/USDC spent all 400 pages inside
		// 18 September 2026 without reaching one complete day.
		//
		// It is spelled differently from the case below because the two are
		// different answers. This one did not look at a single day; that one
		// looked at several and found nothing in them.
		why = fmt.Sprintf(
			"the %d page bound was spent inside the anchor's own day, so not one complete day was reached; ",
			cfg.MaxPages) + why
	case walk.PageCapReached:
		// THE ROW IS STILL STORED AND THAT IS THE POINT. A pair that spent its
		// page bound without meeting a genuine day has measured nothing about
		// FR-10, and the row says so by carrying no reference while recording the
		// days and pages it did cover. Failing the pair instead would leave the
		// same absence with no provenance attached to it.
		why = fmt.Sprintf("no genuine day inside the %d page bound, %d day(s) covered; ",
			cfg.MaxPages, walk.Days) + why
	}
	return r, why, nil
}

// flattenDaysAscending turns the newest-first list of days into one slice
// ascending in time.
//
// The days arrive newest first because the walk runs backwards, while each day's
// own trades are already ascending. domain.ClassifyTrades buckets by UTC day and
// does not require a global order, but SummariseGenuine and the window sums read
// the slice as a series, so handing them a set of days in reverse would be
// correct today and a trap the first time one of them starts to care.
func flattenDaysAscending(days [][]domain.Trade) []domain.Trade {
	n := 0
	for _, d := range days {
		n += len(d)
	}
	out := make([]domain.Trade, 0, n)
	for i := len(days) - 1; i >= 0; i-- {
		out = append(out, days[i]...)
	}
	return out
}

// tradesIn30Days reads the count out of a selection note, and reports whether
// there was one to read. See this file's header for why a missing count is not
// treated as a small one.
func tradesIn30Days(note string) (int, bool) {
	m := tradeCountRe.FindStringSubmatch(note)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return n, true
}

// startOfUTCDay is 00:00:00Z of the day t falls in.
func startOfUTCDay(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}

// tradeTargets selects the pairs to walk, in the order the store returned them.
//
// The selector matches the BASE asset, so a base quoted against two assets yields
// two targets. That is correct rather than convenient: a trade stream belongs to
// a pair, and USTRY against USDC and USTRY against XLM are two streams.
func tradeTargets(rows []store.Asset, only string) ([]store.Asset, error) {
	if strings.TrimSpace(only) == "" {
		return rows, nil
	}
	var out []store.Asset
	for _, a := range rows {
		// domain.Asset.String() is "CODE:ISSUER", which is the same selector
		// holderTargets matches on. Reusing the spelling rather than parsing it
		// keeps one format across the two commands an operator types by hand.
		if a.Base.String() == only {
			out = append(out, a)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no pair in the demonstration set has %s as its base asset", only)
	}
	return out, nil
}
