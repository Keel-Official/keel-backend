// The `divergence` subcommand: measure how far the order book mid sits from the
// pool spot price, across the demonstration set.
//
// WHY IT EXISTS. Methodology 1.0.3 made case 1 of the reference price ladder
// branch on a comparison: with a two-sided book AND a pool, the pool wins when the
// divergence between the two exceeds Thresholds.PriceDivergencePct. That branch was
// written from ONE market, the USTRY fixture, where the divergence is a factor of
// 50. Nothing in this repository knows how often either side of that branch is
// taken on any other pair, and an open methodology question depends on it.
//
// IT MEASURES AND IT DECIDES NOTHING. There is no verdict column, no threshold
// recommendation, and no statement anywhere that a branch is right or wrong. The
// output is the input to a decision that is Al's, in exactly the way
// `keel universe` proposes a candidate set and never selects it.
//
// WHERE THE METHODOLOGY LIVES, AND IT IS NOT HERE. cmd/keel is GREEN with one
// specific limit, "an entrypoint with no methodology in it", so every number in
// every row comes back from internal/domain rather than being computed here:
//
//   - the ladder itself, the divergence formula of 09-flags-and-bands.md line 138,
//     and the choice between book and pool: domain.MidPrice
//   - the threshold that choice is made against: domain.DefaultParams().Thresholds,
//     read from the struct, never written down again in this file
//   - which pool prices a pair when several exist: domain.MidPrice again, through
//     the pool spot it returns. This command does not re-derive "the pool with the
//     largest quote reserve", which is why the fee is reported PER active pool
//     below rather than for one selected pool
//   - the order book mid: see bookMid, which asks domain.MidPrice for it rather
//     than dividing a sum by two in here
//
// WHAT THIS FILE DOES HOLD is the ladder's NUMBERING, one to five, and the mapping
// from an observed market structure onto it. That is bookkeeping over facts about a
// snapshot, whether the book has two sides and whether an active pool exists, and
// every row carries a consistency column that fails loudly if this file's reading
// and domain.MidPrice's answer ever part company.
//
// THE RAW OUTPUT IS GITIGNORED AND THE SUMMARY IS THE RESULT. One JSON file per
// pair, carrying the Horizon bodies the row was derived from, goes under
// -out, which .gitignore excludes. Sixty raw responses are evidence for a
// measurement, not a deliverable, and the same reasoning keeps recordings/ out of
// git except for the sixty that 10-validation.md names.
package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
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

func runDivergence(args []string) error {
	fs := flag.NewFlagSet("divergence", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	pairsPath := fs.String("pairs", "configs/demonstration-set.json", "the pair list to measure")
	out := fs.String("out", "measurements/divergence", "directory for the raw responses, the CSV and the summary. Gitignored")
	limit := fs.Int("limit", 0, "measure at most this many pairs. 0 measures all of them")
	baseURL := fs.String("horizon", horizon.DefaultBaseURL, "Horizon base URL")
	budget := fs.Int("budget", 3000, "requests permitted per hour. Public Horizon allows about 3600 per IP")
	bidUnit := fs.String("bid-amount-unit", string(horizon.BidAmountUnitQuote),
		"which asset an order book bid amount is denominated in: quote or base. See trap 5 in internal/horizon/CLAUDE.md")

	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `keel divergence - how far the book mid sits from the pool spot, per pair

Reads the live order book and the liquidity pools for every pair in the list,
asks internal/domain which rung of the reference price ladder each market sits
on, and reports the divergence between the two price sources where both exist.

It MEASURES. It states no conclusion about which branch of case 1 is correct;
that is a methodology question and this command does not answer it.

Three requests per pair against the Horizon budget: the book, the pools, and the
ledger they were served from.

`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	unit := horizon.BidAmountUnit(*bidUnit)
	if unit != horizon.BidAmountUnitQuote && unit != horizon.BidAmountUnitBase {
		return fmt.Errorf("divergence: -bid-amount-unit must be %q or %q",
			horizon.BidAmountUnitQuote, horizon.BidAmountUnitBase)
	}

	pairs, err := horizon.LoadPairs(*pairsPath)
	if err != nil {
		return fmt.Errorf("divergence: %w", err)
	}
	if *limit > 0 && *limit < len(pairs) {
		pairs = pairs[:*limit]
	}

	rawDir := filepath.Join(*out, "raw")
	if err := os.MkdirAll(rawDir, 0o755); err != nil {
		return fmt.Errorf("divergence: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client := horizon.NewClient(horizon.Config{BaseURL: *baseURL, Budget: *budget, BidAmountUnit: unit})

	// The parameters are read once and passed down, so the threshold every row was
	// judged against is the one printed here and there is no second copy of it.
	params := domain.DefaultParams()
	threshold := params.Thresholds.PriceDivergencePct

	fmt.Fprintf(os.Stdout, "divergence over %d pair(s) from %s\n", len(pairs), *pairsPath)
	fmt.Fprintf(os.Stdout, "methodology %s, PriceDivergencePct = %s percent, read from domain.DefaultParams().Thresholds\n\n",
		domain.MethodologyVersion, threshold)

	rows := make([]divergenceRow, 0, len(pairs))
	for i, pr := range pairs {
		row := measureDivergence(ctx, client, pr, params, i, rawDir)
		rows = append(rows, row)
		fmt.Fprintf(os.Stdout, "[%3d/%d] %s\n", i+1, len(pairs), row.line())
		if ctx.Err() != nil {
			fmt.Fprintf(os.Stdout, "\ninterrupted after %d pair(s); the summary below covers those only\n", len(rows))
			break
		}
	}

	summary := summariseDivergence(rows, threshold, client.Throttled())
	fmt.Fprint(os.Stdout, summary)

	if err := os.WriteFile(filepath.Join(*out, "summary.txt"), []byte(summary), 0o644); err != nil {
		return fmt.Errorf("divergence: %w", err)
	}
	csvPath := filepath.Join(*out, "rows.csv")
	if err := writeDivergenceCSV(csvPath, rows); err != nil {
		return fmt.Errorf("divergence: %w", err)
	}
	fmt.Fprintf(os.Stdout, "\nwrote %s, %s and %d raw response(s) under %s\n",
		csvPath, filepath.Join(*out, "summary.txt"), len(rows), rawDir)
	return nil
}

// ---------------------------------------------------------------- one row

// divergenceRow is one pair measured at one ledger.
type divergenceRow struct {
	Index          int
	Pair           string
	Ledger         uint32
	LedgerClosedAt time.Time

	Bids, Asks int
	TwoSided   bool

	// ActivePools counts the pools that can price anything, which
	// domain.Snapshot.ActivePools defines as both reserves being non-zero. A pool
	// with an empty side is present on Horizon and prices nothing.
	ActivePools int
	// PoolFeeBP carries fee_bp AS THE RESPONSE GAVE IT, one entry per active pool
	// in pool id order. domain.PoolReserves says in as many words not to hardcode
	// 30, and reporting the field is what makes that checkable from the output
	// rather than promised in a comment.
	PoolFeeBP []int32

	BookMid    *decimal.Decimal
	PoolSpot   *decimal.Decimal
	Divergence *decimal.Decimal
	P0         *decimal.Decimal

	// Case is the rung of 03-reference-price.md section 1 this market sits on,
	// read off its structure. Source is what domain.MidPrice actually returned.
	// Consistent compares the two.
	Case       int
	Source     domain.PriceSource
	Consistent bool

	// AboveThreshold is the case 1 split, and it is taken from Source rather than
	// from a second comparison against the threshold in this file. Source is pool
	// on a case 1 market exactly when domain.MidPrice found the divergence above
	// the threshold, so this reports domain's own answer.
	AboveThreshold bool

	Requests int
	RawPath  string
	Err      string
}

func (r divergenceRow) line() string {
	if r.Err != "" {
		return fmt.Sprintf("ERROR   %-10s %s", shortPair(r.Pair), r.Err)
	}
	head := fmt.Sprintf("case %d  %-10s book %3d/%-3d pools %d  mid %-16s spot %-16s div %-12s -> %s",
		r.Case, shortPair(r.Pair), r.Bids, r.Asks, r.ActivePools,
		showRounded(r.BookMid), showRounded(r.PoolSpot), showRounded(r.Divergence), r.Source)
	if !r.Consistent {
		// A structural reading that disagrees with domain.MidPrice is a defect in
		// this file or in that function, and either way it must not be averaged
		// into a count. It is shouted per row and counted in the summary.
		head += "  INCONSISTENT with domain.MidPrice"
	}
	return head
}

// showRounded renders a value for the TERMINAL at Stellar's own seven decimals.
// The division inside domain.MidPrice runs at domain.Precision, so an unrounded
// divergence prints twenty-odd digits and sixty rows of that hide the shape this
// command exists to show. The CSV and the summary's smallest and largest carry the
// value as computed; only this line is shortened, and nothing downstream reads it.
func showRounded(d *decimal.Decimal) string {
	if d == nil {
		return "null"
	}
	return d.Round(stellarDecimals).String()
}

// stellarDecimals is the number of decimal places a Stellar amount carries.
const stellarDecimals = 7

// measureDivergence reads one pair and derives one row. It never returns an
// error: a pair that fails to read is a row carrying the failure, because a
// sixty-pair survey that stops on the first dead market measures nothing.
func measureDivergence(ctx context.Context, c *horizon.Client, pr horizon.Pair,
	p domain.Params, index int, rawDir string) divergenceRow {

	row := divergenceRow{Index: index, Pair: pr.String()}

	before := c.Requests()
	obs, err := c.GetSnapshot(ctx, pr.Base, pr.Quote)
	row.Requests = c.Requests() - before
	if err != nil {
		row.Err = err.Error()
		return row
	}

	s := obs.Snapshot
	row.Ledger, row.LedgerClosedAt = s.LedgerSeq, s.LedgerClosedAt
	row.Bids, row.Asks = len(s.Book.Bids), len(s.Book.Asks)
	row.TwoSided = row.Bids > 0 && row.Asks > 0

	// Sorted before iteration, NFR-9. The fee list is part of the output and a
	// list whose order depends on how a slice happened to arrive is not
	// reproducible.
	active := s.ActivePools()
	sort.Slice(active, func(i, j int) bool { return active[i].PoolID < active[j].PoolID })
	row.ActivePools = len(active)
	for _, pool := range active {
		row.PoolFeeBP = append(row.PoolFeeBP, pool.FeeBP)
	}

	p0, src, poolSpot, div := domain.MidPrice(s, p)
	row.Source, row.PoolSpot, row.Divergence = src, poolSpot, div
	if src != domain.PriceSourceNone {
		row.P0 = &p0
	}
	row.BookMid = bookMid(s, p)

	row.Case = ladderCase(row.Bids, row.Asks, row.ActivePools > 0)
	row.Consistent = caseAgreesWithSource(row.Case, src)
	row.AboveThreshold = row.Case == 1 && src == domain.PriceSourcePool

	// The raw bodies, so the row can be re-derived without a second Horizon call
	// and without trusting this file's arithmetic.
	row.RawPath = filepath.Join(rawDir, fmt.Sprintf("%02d-%s.json", index, pr.Slug()))
	if body, err := json.MarshalIndent(obs.Raw, "", "  "); err != nil {
		row.Err = "marshal raw: " + err.Error()
	} else if err := os.WriteFile(row.RawPath, body, 0o644); err != nil {
		row.Err = "write raw: " + err.Error()
	}
	return row
}

// ---------------------------------------------------------------- the ladder

// unreachableDivergence is a price divergence percentage no market can produce,
// used only by bookMid below. Stellar amounts are int64 stroops with seven
// decimals, so a ratio of reserves cannot approach 1e40.
var unreachableDivergence = decimal.New(1, 40)

// bookMid returns the ORDER BOOK mid, and asks domain.MidPrice for it rather than
// computing it.
//
// WHY IT LOOKS LIKE A TRICK. The mid of a two-sided book is defined in
// 03-reference-price.md and computed in domain.MidPrice, which returns it only on
// the branch where the book wins. On a case 1 market where the pool wins, the
// number that comes back is the pool spot and the mid is not returned at all. The
// alternative was to divide the best bid plus the best ask by two in this file,
// which puts a second copy of a methodology definition in the entrypoint, and a
// second home for a fact is the drift this repository keeps writing down. So the
// same function is asked again with the threshold raised out of reach, which
// forces the branch that returns the mid and computes nothing new.
//
// IT CHECKS ITSELF. A probe that comes back with any source other than book means
// the assumption above no longer holds, and nil is returned rather than a number
// whose meaning is unknown. The book mid is undefined on a book with fewer than
// two sides, so nil is also the right answer there.
//
// Params is copied by value and only Thresholds, itself a value struct, is
// touched, so the caller's slices are not aliased into.
func bookMid(s domain.Snapshot, p domain.Params) *decimal.Decimal {
	probe := p
	probe.Thresholds.PriceDivergencePct = unreachableDivergence
	mid, src, _, _ := domain.MidPrice(s, probe)
	if src != domain.PriceSourceBook {
		return nil
	}
	return &mid
}

// ladderCase numbers the rung of docs/methodology/03-reference-price.md section 1
// that a market's STRUCTURE puts it on. Two facts decide it, whether the book has
// two sides and whether an active pool exists, and both are observations about the
// snapshot rather than computations over it.
//
// A ONE-SIDED BOOK WITH NO POOL FALLS TO CASE 5, and that is the ladder's own
// fall-through rather than a reading imposed here: case 2 requires two sides, case
// 3 requires a pool, and domain.MidPrice returns priceSource none for exactly this
// shape. Case 5 is labelled "neither book nor pool" in the document, so the two
// populations inside it are counted separately in the summary instead of being
// presented as one.
func ladderCase(bids, asks int, hasActivePool bool) int {
	twoSided := bids > 0 && asks > 0
	anySide := bids > 0 || asks > 0
	switch {
	case twoSided && hasActivePool:
		return 1
	case twoSided:
		return 2
	case anySide && hasActivePool:
		return 3
	case hasActivePool:
		return 4
	default:
		return 5
	}
}

// caseAgreesWithSource is the consistency check between this file's structural
// reading and domain.MidPrice's answer. Case 1 admits both sources, because which
// one wins is the whole question this command was written to measure; every other
// case admits exactly one.
func caseAgreesWithSource(c int, src domain.PriceSource) bool {
	switch c {
	case 1:
		return src == domain.PriceSourceBook || src == domain.PriceSourcePool
	case 2:
		return src == domain.PriceSourceBook
	case 3, 4:
		return src == domain.PriceSourcePool
	case 5:
		return src == domain.PriceSourceNone
	}
	return false
}

var ladderCaseNames = []string{
	1: "pool present and two-sided book",
	2: "two-sided book, no pool",
	3: "one-sided book, pool present",
	4: "pool only, no book",
	5: "neither a two-sided book nor a pool",
}

// ---------------------------------------------------------------- the summary

// divergenceBuckets are DISPLAY boundaries and are derived from nothing. They
// exist so that sixty numbers read as a shape instead of a list, and the threshold
// itself is reported separately and exactly, so no bucket edge can be mistaken for
// it. The same caveat the demonstration set carries about its four market buckets
// applies here: measured contents, chosen boundaries.
var divergenceBuckets = []struct {
	Label string
	Upper *decimal.Decimal // nil is the open top bucket
}{
	{"0 to 1 percent", decPtr("1")},
	{"1 to 5 percent", decPtr("5")},
	{"5 to 10 percent", decPtr("10")},
	{"10 to 50 percent", decPtr("50")},
	{"50 to 100 percent", decPtr("100")},
	{"over 100 percent", nil},
}

func decPtr(s string) *decimal.Decimal {
	d := decimal.RequireFromString(s)
	return &d
}

// summariseDivergence renders the counts. It returns a string rather than writing
// to a writer so that the same text goes to stdout and to the file, instead of
// being formatted twice and drifting.
func summariseDivergence(rows []divergenceRow, threshold decimal.Decimal, throttled int) string {
	var b strings.Builder

	counts := make([]int, len(ladderCaseNames))
	errs, inconsistent := 0, 0
	aboveThreshold, atOrBelow := 0, 0
	noBookNoPool, oneSidedNoPool := 0, 0
	measured := 0
	divergences := make([]decimal.Decimal, 0, len(rows))

	for _, r := range rows {
		if r.Err != "" {
			errs++
			continue
		}
		measured++
		if r.Case >= 0 && r.Case < len(counts) {
			counts[r.Case]++
		}
		if !r.Consistent {
			inconsistent++
		}
		switch r.Case {
		case 1:
			if r.AboveThreshold {
				aboveThreshold++
			} else {
				atOrBelow++
			}
			if r.Divergence != nil {
				divergences = append(divergences, *r.Divergence)
			}
		case 5:
			if r.Bids == 0 && r.Asks == 0 {
				noBookNoPool++
			} else {
				oneSidedNoPool++
			}
		}
	}

	fmt.Fprintf(&b, "\nSummary over %d pair(s): %d measured, %d failed to read\n", len(rows), measured, errs)
	fmt.Fprintf(&b, "PriceDivergencePct = %s percent\n\n", threshold)

	fmt.Fprintf(&b, "Ladder case, docs/methodology/03-reference-price.md section 1:\n")
	for c := 1; c < len(ladderCaseNames); c++ {
		fmt.Fprintf(&b, "  case %d  %-36s %3d\n", c, ladderCaseNames[c], counts[c])
	}

	fmt.Fprintf(&b, "\nWithin case 1, %d pair(s), split as domain.MidPrice resolved them:\n", counts[1])
	fmt.Fprintf(&b, "  divergence above %s percent, priceSource pool   %3d\n", threshold, aboveThreshold)
	fmt.Fprintf(&b, "  divergence at or below it, priceSource book     %3d\n", atOrBelow)
	fmt.Fprintf(&b, "  the comparison is strictly greater than, so a divergence exactly equal to the\n")
	fmt.Fprintf(&b, "  threshold counts as at or below\n")

	fmt.Fprintf(&b, "\nWithin case 5, %d pair(s):\n", counts[5])
	fmt.Fprintf(&b, "  no book on either side and no pool               %3d\n", noBookNoPool)
	fmt.Fprintf(&b, "  one side of a book, no pool                      %3d\n", oneSidedNoPool)

	if len(divergences) > 0 {
		sort.Slice(divergences, func(i, j int) bool { return divergences[i].LessThan(divergences[j]) })
		fmt.Fprintf(&b, "\nCase 1 divergence, %d value(s), display buckets chosen and not derived:\n", len(divergences))
		// Each band is (previous top, this top], so every value lands in exactly
		// one bucket and the counts add up to len(divergences).
		var prev *decimal.Decimal
		for _, bucket := range divergenceBuckets {
			n := 0
			for _, d := range divergences {
				if prev != nil && d.LessThanOrEqual(*prev) {
					continue
				}
				if bucket.Upper != nil && d.GreaterThan(*bucket.Upper) {
					continue
				}
				n++
			}
			fmt.Fprintf(&b, "  %-20s %3d\n", bucket.Label, n)
			prev = bucket.Upper
		}
		fmt.Fprintf(&b, "  smallest %s percent, largest %s percent\n",
			divergences[0], divergences[len(divergences)-1])
	}

	if inconsistent > 0 {
		fmt.Fprintf(&b, "\n%d row(s) where this command's structural reading and domain.MidPrice disagree.\n", inconsistent)
		fmt.Fprintf(&b, "That is a defect in one of the two and not a measurement; see the rows marked INCONSISTENT.\n")
	}
	if throttled > 0 {
		fmt.Fprintf(&b, "\n%d rate limit response(s) were seen, retries included. A short list and a\n", throttled)
		fmt.Fprintf(&b, "throttled list are different findings.\n")
	}
	fmt.Fprintf(&b, "\nThis is a measurement. It states nothing about which branch of case 1 is correct.\n")
	return b.String()
}

// ---------------------------------------------------------------- the CSV

func writeDivergenceCSV(path string, rows []divergenceRow) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	w := csv.NewWriter(f)
	if err := w.Write([]string{
		"index", "pair", "ledger", "ledger_closed_at",
		"bids", "asks", "two_sided_book", "active_pools", "pool_fee_bp",
		"book_mid", "pool_spot", "divergence_pct", "p0",
		"price_source", "ladder_case", "ladder_case_meaning",
		"above_threshold", "consistent", "requests", "raw", "error",
	}); err != nil {
		return err
	}
	for _, r := range rows {
		meaning := ""
		if r.Case > 0 && r.Case < len(ladderCaseNames) {
			meaning = ladderCaseNames[r.Case]
		}
		if err := w.Write([]string{
			strconv.Itoa(r.Index),
			r.Pair,
			strconv.FormatUint(uint64(r.Ledger), 10),
			r.LedgerClosedAt.UTC().Format(time.RFC3339),
			strconv.Itoa(r.Bids),
			strconv.Itoa(r.Asks),
			yn(r.TwoSided),
			strconv.Itoa(r.ActivePools),
			feeList(r.PoolFeeBP),
			optional(r.BookMid),
			optional(r.PoolSpot),
			optional(r.Divergence),
			optional(r.P0),
			string(r.Source),
			strconv.Itoa(r.Case),
			meaning,
			yn(r.AboveThreshold),
			yn(r.Consistent),
			strconv.Itoa(r.Requests),
			r.RawPath,
			r.Err,
		}); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

func feeList(fees []int32) string {
	s := make([]string, len(fees))
	for i, f := range fees {
		s[i] = strconv.FormatInt(int64(f), 10)
	}
	return strings.Join(s, " ")
}
