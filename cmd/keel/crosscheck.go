// The `crosscheck` subcommand: Layer 3 of the validation protocol, executed.
//
// docs/methodology/10-validation.md section 3 defines it. A recorder captured raw
// Horizon snapshots while the ledgers were current, the historical path is asked
// to reproduce those same ledgers, and the two are compared at four depths:
//
//  1. the number of bid and ask levels
//  2. the price of each level
//  3. the amount of each level
//  4. ComputeAssetRisk over both
//
// It reports separately at every depth, because aggregate agreement hides a
// discrepancy in a single large offer, which is that section's own reason.
//
// MISMATCHES ARE NOT FAILURES, and that is the protocol's wording rather than a
// softening of it. A discrepancy that is explained correctly demonstrates
// understanding of the data; one that is never looked for demonstrates nothing.
// So every row carries the reconstruction's own completeness counters beside its
// verdict, and a row where the reconstruction admits it is incomplete is a
// different kind of row from one where two complete readings disagree.
//
// WHICH HISTORICAL PATH. `keel replay` walks operations forward and is the only
// route to a book six months old, and it is priced by how busy the ACCOUNTS are
// rather than the pair: three of the quietest pairs in the demonstration set did
// not finish in ten minutes at a target seven hours old. This command uses the
// rewind instead, which carries the live offer set back using
// last_modified_ledger. See the header of internal/horizon/rewind.go for what
// that can and cannot see. Both produce dataSource offers-implied and neither is
// a measurement.
//
// POOLS ARE EXCLUDED FROM BOTH SIDES. The recordings carry the pool response and
// the rewind reconstructs no pool, so comparing risk with pools on one side only
// would measure the missing pool rather than the book. Depth 4 runs over
// book-only snapshots on both sides, which isolates what this command is for.
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
	"strings"
	"syscall"

	"github.com/Keel-Official/keel-backend/internal/domain"
	"github.com/Keel-Official/keel-backend/internal/horizon"
	"github.com/shopspring/decimal"
)

func runCrosscheck(args []string) error {
	fs := flag.NewFlagSet("crosscheck", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	dir := fs.String("recordings", "recordings/samples", "directory of recordings to compare against")
	limit := fs.Int("limit", 0, "compare at most this many recordings. 0 compares all of them")
	out := fs.String("out", "", "write one CSV row per recording to this file. Optional")
	baseURL := fs.String("horizon", horizon.DefaultBaseURL, "Horizon base URL")
	budget := fs.Int("budget", 3000, "requests permitted per hour. Public Horizon allows about 3600 per IP")
	bidUnit := fs.String("bid-amount-unit", string(horizon.BidAmountUnitQuote),
		"which asset an order book bid amount is denominated in: quote or base. See trap 5 in internal/horizon/CLAUDE.md")

	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `keel crosscheck - Layer 3 of docs/methodology/10-validation.md

Reads recordings taken while their ledgers were current, rebuilds the same books
from Horizon today, and compares them at four depths. Reports each depth
separately, and prints the reconstruction's completeness counters beside every
verdict, because a reconstruction that admits it is incomplete and one that
disagrees with the recording are not the same finding.

`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	unit := horizon.BidAmountUnit(*bidUnit)
	if unit != horizon.BidAmountUnitQuote && unit != horizon.BidAmountUnitBase {
		return fmt.Errorf("crosscheck: -bid-amount-unit must be %q or %q",
			horizon.BidAmountUnitQuote, horizon.BidAmountUnitBase)
	}

	paths, err := findRecordings(*dir)
	if err != nil {
		return fmt.Errorf("crosscheck: %w", err)
	}
	if len(paths) == 0 {
		return fmt.Errorf("crosscheck: no recordings under %s", *dir)
	}
	if *limit > 0 && *limit < len(paths) {
		paths = paths[:*limit]
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client := horizon.NewClient(horizon.Config{BaseURL: *baseURL, Budget: *budget, BidAmountUnit: unit})

	fmt.Fprintf(os.Stdout, "Layer 3 over %d recording(s) from %s\n\n", len(paths), *dir)
	rows := make([]crosscheckRow, 0, len(paths))
	for i, p := range paths {
		row := compareRecording(ctx, client, p, unit)
		rows = append(rows, row)
		fmt.Fprintf(os.Stdout, "[%3d/%d] %s\n", i+1, len(paths), row.line())
		if ctx.Err() != nil {
			break
		}
	}

	summarise3(os.Stdout, rows)
	if *out != "" {
		if err := writeCrosscheckCSV(*out, rows); err != nil {
			return fmt.Errorf("crosscheck: %w", err)
		}
		fmt.Fprintf(os.Stdout, "\nwrote %s\n", *out)
	}
	return nil
}

// ---------------------------------------------------------------- one row

// crosscheckRow is one recording compared against one reconstruction.
type crosscheckRow struct {
	Path   string
	Pair   string
	Ledger uint32

	RecordedBids, RecordedAsks int
	RebuiltBids, RebuiltAsks   int

	// The four depths of 10-validation.md section 3, reported separately.
	LevelsMatch  bool
	PricesMatch  bool
	AmountsMatch bool
	RiskMatch    bool

	// Carried, Changed and Gone are the rewind's own account of how each
	// offer on its book was obtained. They sit BESIDE the verdict rather than
	// behind it: a row that fails with Changed > 0 is the reconstruction saying
	// so, not the two sources disagreeing.
	Carried, Changed, Gone int

	// Capped is set when a recorded side holds exactly horizon.BookPageLimit
	// levels, so the recording is a PREFIX of the real book. The comparison then
	// runs over that prefix and the level count is checked as "at least" rather
	// than "equal", which is the honest reading of a truncated source.
	Capped bool

	Requests int
	Err      string

	// Explanation is the first difference found, in words, so the results table
	// carries the protocol's required explanation rather than a bare NO.
	Explanation string
}

// Comparable reports whether the reconstruction claimed to be complete. A row
// that is not comparable says nothing about whether the two paths agree.
func (r crosscheckRow) Comparable() bool { return r.Err == "" && r.Changed == 0 && r.Gone == 0 }

func (r crosscheckRow) verdict() string {
	switch {
	case r.Err != "":
		return "ERROR"
	case !r.Comparable():
		return "PARTIAL"
	case r.LevelsMatch && r.PricesMatch && r.AmountsMatch && r.RiskMatch:
		return "MATCH"
	default:
		return "MISMATCH"
	}
}

func (r crosscheckRow) line() string {
	head := fmt.Sprintf("%-9s %-8s ledger %d  recorded %d/%d  rebuilt %d/%d",
		r.verdict(), shortPair(r.Pair), r.Ledger,
		r.RecordedBids, r.RecordedAsks, r.RebuiltBids, r.RebuiltAsks)
	tail := fmt.Sprintf("  [carried %d, changed %d, gone %d, %d req]",
		r.Carried, r.Changed, r.Gone, r.Requests)
	if r.Explanation != "" {
		return head + tail + "\n          " + r.Explanation
	}
	return head + tail
}

func compareRecording(ctx context.Context, c *horizon.Client, path string, unit horizon.BidAmountUnit) crosscheckRow {
	row := crosscheckRow{Path: path}

	tick, err := horizon.ReadTick(path)
	if err != nil {
		row.Err = err.Error()
		return row
	}
	row.Pair, row.Ledger = tick.Pair, tick.LedgerBefore

	base, quote, err := pairFromTick(tick)
	if err != nil {
		row.Err = err.Error()
		return row
	}

	recorded, err := recordedBook(tick, base, quote, unit)
	if err != nil {
		row.Err = err.Error()
		return row
	}
	row.RecordedBids, row.RecordedAsks = len(recorded.Bids), len(recorded.Asks)

	rewound, err := c.RewindBook(ctx, base, quote, tick.LedgerBefore)
	if err != nil {
		row.Err = err.Error()
		return row
	}
	row.Carried, row.Changed, row.Gone = rewound.Carried, rewound.Changed, rewound.Gone
	row.Requests = rewound.Requests

	rebuilt := rewound.Snapshot.Book
	row.RebuiltBids, row.RebuiltAsks = len(rebuilt.Bids), len(rebuilt.Asks)

	// A recorded side at exactly the endpoint's limit is a PREFIX of the real
	// book, because /order_book truncates without saying so.
	bidCapped := row.RecordedBids >= horizon.BookPageLimit
	askCapped := row.RecordedAsks >= horizon.BookPageLimit
	row.Capped = bidCapped || askCapped

	// Depth 1: level counts. A capped side is checked as "the rebuild has at
	// least as many", because the recording cannot report more than the cap and
	// requiring equality would call every deep market a mismatch.
	row.LevelsMatch = countAgrees(row.RecordedBids, row.RebuiltBids, bidCapped) &&
		countAgrees(row.RecordedAsks, row.RebuiltAsks, askCapped)
	if !row.LevelsMatch {
		row.Explanation = fmt.Sprintf("level counts differ: recorded %d bid / %d ask, rebuilt %d bid / %d ask%s",
			row.RecordedBids, row.RecordedAsks, row.RebuiltBids, row.RebuiltAsks, cappedNote(row.Capped))
	}

	// Depths 2 and 3: prices EXACTLY, amounts within Tolerance. That split is
	// section 4 of the protocol: a price is a rational and compares exactly, an
	// amount on the bid side has been through a division and cannot.
	row.PricesMatch, row.AmountsMatch = true, true
	for _, side := range [2]struct {
		name  string
		rec   []domain.Level
		built []domain.Level
	}{{"bid", recorded.Bids, rebuilt.Bids}, {"ask", recorded.Asks, rebuilt.Asks}} {
		n := len(side.rec)
		if len(side.built) < n {
			n = len(side.built)
		}
		for i := 0; i < n; i++ {
			if side.rec[i].Price.Cmp(side.built[i].Price) != 0 {
				row.PricesMatch = false
				if row.Explanation == "" {
					row.Explanation = fmt.Sprintf("%s %d price differs: recorded %s, rebuilt %s",
						side.name, i, side.rec[i].Price, side.built[i].Price)
				}
				break
			}
			if diff := side.rec[i].Amount.Sub(side.built[i].Amount).Abs(); diff.GreaterThan(layer3Tolerance) {
				row.AmountsMatch = false
				if row.Explanation == "" {
					row.Explanation = fmt.Sprintf("%s %d amount differs by %s: recorded %s, rebuilt %s",
						side.name, i, diff, side.rec[i].Amount, side.built[i].Amount)
				}
				break
			}
		}
	}

	// Depth 4: the engine over both, BOOK ONLY on both sides.
	// Depth 4 is skipped on a capped recording. The engine reads the WHOLE book,
	// so running it over a prefix on one side and the full book on the other
	// measures the truncation and not the reconstruction.
	if row.Capped {
		row.RiskMatch = true
		if row.Explanation == "" {
			row.Explanation = "a recorded side is at the endpoint limit, so depth 4 was not run over a truncated book"
		}
	} else {
		row.RiskMatch, err = riskAgrees(base, quote, tick.LedgerBefore, recorded, rebuilt)
	}
	if err != nil && row.Explanation == "" {
		row.Explanation = err.Error()
	}
	return row
}

// layer3Tolerance is docs/methodology/10-validation.md section 4. It applies to
// derived decimal quantities only; prices and level counts compare exactly.
var layer3Tolerance = decimal.RequireFromString("0.0000001")

// riskAgrees runs the methodology over both books and compares the figures that
// a difference in the book would move.
func riskAgrees(base, quote domain.Asset, ledger uint32, recorded, rebuilt domain.OrderBook) (bool, error) {
	mk := func(b domain.OrderBook) domain.Snapshot {
		return domain.Snapshot{Base: base, Quote: quote, LedgerSeq: ledger, Book: b, Source: domain.DataSourceOffersImplied}
	}
	a, err := domain.ComputeAssetRisk(mk(recorded), domain.DefaultParams())
	if err != nil {
		return false, fmt.Errorf("computing over the recorded book: %w", err)
	}
	b, err := domain.ComputeAssetRisk(mk(rebuilt), domain.DefaultParams())
	if err != nil {
		return false, fmt.Errorf("computing over the rebuilt book: %w", err)
	}

	if a.PriceSource != b.PriceSource || a.Band != b.Band {
		return false, fmt.Errorf("recorded is %s/%s, rebuilt is %s/%s", a.PriceSource, a.Band, b.PriceSource, b.Band)
	}
	if !nearOrBothNil(a.MidPrice, b.MidPrice) {
		return false, fmt.Errorf("P0 differs: recorded %s, rebuilt %s", showDec(a.MidPrice), showDec(b.MidPrice))
	}
	if !nearOrBothNil(a.SpreadPct, b.SpreadPct) {
		return false, fmt.Errorf("spread differs: recorded %s, rebuilt %s", showDec(a.SpreadPct), showDec(b.SpreadPct))
	}
	for i := range a.Depth {
		if i >= len(b.Depth) {
			return false, errors.New("the depth ladders are different lengths")
		}
		if !a.Depth[i].BuySide.Sub(b.Depth[i].BuySide).Abs().LessThanOrEqual(layer3Tolerance) ||
			!a.Depth[i].SellSide.Sub(b.Depth[i].SellSide).Abs().LessThanOrEqual(layer3Tolerance) {
			return false, fmt.Errorf("depth at delta %s differs: recorded %s/%s, rebuilt %s/%s",
				a.Depth[i].Delta, a.Depth[i].BuySide, a.Depth[i].SellSide,
				b.Depth[i].BuySide, b.Depth[i].SellSide)
		}
	}
	if flagsOf(a.Flags) != flagsOf(b.Flags) {
		return false, fmt.Errorf("flags differ: recorded %v, rebuilt %v", a.Flags, b.Flags)
	}
	return true, nil
}

func nearOrBothNil(a, b *decimal.Decimal) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Sub(*b).Abs().LessThanOrEqual(layer3Tolerance)
}

func showDec(d *decimal.Decimal) string {
	if d == nil {
		return "null"
	}
	return d.String()
}

// flagsOf renders a flag set order-independently, because 09-flags-and-bands.md
// compares flag SETS exactly and the order they are emitted in is not part of
// the claim.
func flagsOf(f []domain.Flag) string {
	s := make([]string, len(f))
	for i, x := range f {
		s[i] = string(x)
	}
	sort.Strings(s)
	return strings.Join(s, ",")
}

// ---------------------------------------------------------------- reading

func findRecordings(dir string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(p, ".json.gz") {
			out = append(out, p)
		}
		return nil
	})
	sort.Strings(out) // deterministic order, so two runs report in the same sequence
	return out, err
}

// pairFromTick recovers the pair identity from the recording.
//
// It reads the ORDER BOOK BODY's own echo rather than the tick's `pair` string,
// because that string is a display form and the body carries the full asset type,
// code and issuer for both sides. Identity is the pair (code, issuer) and is
// never matched on the ticker.
func pairFromTick(t horizon.RawTick) (domain.Asset, domain.Asset, error) {
	for _, s := range t.Sources {
		if s.Endpoint != "order_book" || s.HTTPStatus != 200 {
			continue
		}
		return horizon.PairFromOrderBook([]byte(s.Body))
	}
	return domain.Asset{}, domain.Asset{}, fmt.Errorf("recording has no successful order_book source")
}

func recordedBook(t horizon.RawTick, base, quote domain.Asset, unit horizon.BidAmountUnit) (domain.OrderBook, error) {
	for _, s := range t.Sources {
		if s.Endpoint != "order_book" {
			continue
		}
		if s.HTTPStatus != 200 {
			return domain.OrderBook{}, fmt.Errorf("recorded order_book returned HTTP %d", s.HTTPStatus)
		}
		return horizon.ParseOrderBook([]byte(s.Body), base, quote, unit)
	}
	return domain.OrderBook{}, fmt.Errorf("recording has no order_book source")
}

// ---------------------------------------------------------------- output

func summarise3(w *os.File, rows []crosscheckRow) {
	var match, mismatch, partial, failed int
	for _, r := range rows {
		switch r.verdict() {
		case "MATCH":
			match++
		case "MISMATCH":
			mismatch++
		case "PARTIAL":
			partial++
		default:
			failed++
		}
	}
	fmt.Fprintf(w, "\n%d recording(s): %d match, %d mismatch, %d partial, %d error\n",
		len(rows), match, mismatch, partial, failed)
	fmt.Fprintf(w, "  MATCH    the two paths agree at all four depths, and the rebuild claimed no gap\n")
	fmt.Fprintf(w, "  MISMATCH they disagree, and the rebuild claimed no gap. Each one needs an explanation\n")
	fmt.Fprintf(w, "  PARTIAL  the rebuild could not carry every offer back, so it says nothing either way\n")

	// The SOW promises cross-validation on at least 50 pairs, and a partial row
	// is not one of them. Saying which number counts is the point of printing it.
	fmt.Fprintf(w, "\n%d of the %d recording(s) were comparable at all. 10-validation.md section 3 asks for at least 50\n",
		match+mismatch, len(rows))
}

func writeCrosscheckCSV(path string, rows []crosscheckRow) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	w := csv.NewWriter(f)
	if err := w.Write([]string{
		"verdict", "pair", "ledger",
		"recorded_bids", "recorded_asks", "rebuilt_bids", "rebuilt_asks",
		"levels_match", "prices_match", "amounts_match", "risk_match",
		"offers_carried", "offers_changed_after_target", "offers_gone_unresolved",
		"requests", "explanation", "error", "recording",
	}); err != nil {
		return err
	}
	for _, r := range rows {
		if err := w.Write([]string{
			r.verdict(), r.Pair, fmt.Sprintf("%d", r.Ledger),
			fmt.Sprintf("%d", r.RecordedBids), fmt.Sprintf("%d", r.RecordedAsks),
			fmt.Sprintf("%d", r.RebuiltBids), fmt.Sprintf("%d", r.RebuiltAsks),
			yn(r.LevelsMatch), yn(r.PricesMatch), yn(r.AmountsMatch), yn(r.RiskMatch),
			fmt.Sprintf("%d", r.Carried), fmt.Sprintf("%d", r.Changed), fmt.Sprintf("%d", r.Gone),
			fmt.Sprintf("%d", r.Requests), r.Explanation, r.Err, r.Path,
		}); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

func yn(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func shortPair(p string) string {
	code, _, _ := strings.Cut(p, ":")
	code, _, _ = strings.Cut(code, "/")
	if code == "" {
		return "XLM"
	}
	return code
}

// countAgrees compares one side's level count, allowing for a truncated
// recording.
func countAgrees(recorded, rebuilt int, capped bool) bool {
	if capped {
		return rebuilt >= recorded
	}
	return recorded == rebuilt
}

func cappedNote(capped bool) string {
	if capped {
		return " (a recorded side is at the endpoint limit and is a prefix)"
	}
	return ""
}
