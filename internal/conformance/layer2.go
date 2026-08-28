// Layer 2 of docs/methodology/10-validation.md: synthetic testnet fixtures.
//
// THIS FILE HOLDS NO NUMBERS AND MUST NEVER HOLD ANY. It is the container for
// Layer 2 and not its content. Every figure a Layer 2 scenario is judged
// against comes from a file under testdata/fixtures/layer2/, which is RED: Al
// creates the state on testnet, records the transaction that created it, and
// works the expected values by hand. The reason is the one in fixture.go and in
// the CLAUDE.md zone map: numbers produced by Claude must never become the
// numbers that test Claude's code.
//
// So this file was written knowing it cannot make a single scenario pass. What
// it can do is make the gap countable, make the shape of a fixture explicit
// enough to fill in without guessing, and check the one thing per scenario that
// the methodology states in words rather than in figures.
//
// WHY A FILE AND NOT GO CONSTANTS, which is how the golden fixture is carried.
// The golden fixture is one market, transcribed once. Layer 2 is ten, and every
// one of them has to be created on testnet before it can be transcribed. A Go
// constant would have to be written by whoever owns the red zone, in the green
// zone, which is the wrong side of the line the zone map draws. Reading JSON at
// run time keeps the numbers in the red directory where they belong and leaves
// this package able to load them without holding them.
package conformance

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/shopspring/decimal"

	"github.com/Keel-Official/keel-backend/internal/domain"
)

// Layer2Dir is where the scenario files live, relative to this package.
//
// The directory need not exist. It does not today, and a missing directory is
// reported as ten missing scenarios rather than as an error, because "nobody has
// started" and "something is broken" are different states and only one of them
// is true.
const Layer2Dir = "../../testdata/fixtures/layer2"

// Layer2Scenario is one row of docs/methodology/10-validation.md section 2.
//
// Title and Discriminates are transcribed from that section. They are the
// methodology's words and are not a definition made here; if the two ever
// disagree the methodology is right and this file has the bug.
type Layer2Scenario struct {
	ID    int
	Slug  string
	Title string

	// Discriminates says what this scenario catches that no other one does. It
	// is the reason the scenario is in the list, and it is why an incomplete
	// Layer 2 cannot be waved through by running the other nine.
	Discriminates string

	// Property is the assertion the methodology states in WORDS for this
	// scenario, checkable against a computed result with no hand figures in
	// sight. It runs as soon as an input exists, before any expected values are
	// worked out, which is the whole reason it is separated from them.
	//
	// Nil for the scenarios whose only statement is numeric. Two of the ten are
	// like that and pretending otherwise would be inventing a rule.
	Property func(domain.Snapshot, domain.AssetRisk, domain.Params) error
}

// Layer2Scenarios is the list, in the order section 2 gives them. The order is
// not cosmetic: the section numbers each one, and a results table that renumbers
// them stops being comparable with the protocol it reports against.
var Layer2Scenarios = []Layer2Scenario{
	{
		ID: 1, Slug: "two-sided-book-no-pool",
		Title:         "Two-sided book, no pool",
		Discriminates: "the plain case. Everything else is a departure from it",
		Property:      propNoActivePool,
	},
	{
		ID: 2, Slug: "one-sided-book-no-pool",
		Title:         "One-sided book, no pool",
		Discriminates: "a market with no mid price, where spreadPct has nothing to be computed from",
		Property:      propSpreadAbsent,
	},
	{
		ID: 3, Slug: "empty-book-active-pool",
		Title:         "Empty book with an active pool",
		Discriminates: "the pool carrying the price alone, so priceSource must fall through to it",
		Property:      propPriceSourceIs(domain.PriceSourcePool),
	},
	{
		ID: 4, Slug: "no-book-no-pool",
		Title:         "No book and no pool, giving priceSource = none",
		Discriminates: "the state that must produce a RESULT and not an error. A dormant asset is an answer",
		Property:      propPriceSourceIs(domain.PriceSourceNone),
	},
	{
		ID: 5, Slug: "pool-above-target",
		Title:         "Pool priced above P_target at 2 percent, so fromAmm must be exactly zero",
		Discriminates: "THE discriminating test for the SDEX and AMM combination. An implementation that sums the two venues independently returns a non-zero fromAmm and fails only here",
		Property:      propFromAmmZeroAtDelta("0.02"),
	},
	{
		ID: 6, Slug: "two-pools-one-pair",
		Title:         "Two pools on the same pair",
		Discriminates: "an implementation that finds one pool and stops",
		Property:      propBothPoolsCounted,
	},
	{
		ID: 7, Slug: "divergence-conflict",
		Title:         "Book and pool diverging beyond PriceDivergencePct, triggering PRICE_SOURCE_CONFLICT",
		Discriminates: "the flag that says the two venues do not agree about the price",
		Property:      propFlagRaised(domain.FlagPriceSourceConflict),
	},
	{
		ID: 8, Slug: "target-above-every-ask",
		Title:         "A target above every ask, giving Reachable = false",
		Discriminates: "a cost figure that must not be reported as if the target were attainable",
		Property:      propSomeOrderbookRungUnreachable,
	},
	{
		ID: 9, Slug: "active-pool-nulls-max-reachable",
		Title:         "An active pool present, so MaxReachablePrice must be null",
		Discriminates: "the presence rule that contract 1.4.3 corrected. See P2-16",
		Property:      propMaxReachableNull,
	},
	{
		ID: 10, Slug: "monotonic-ladder",
		Title:         "Monotonicity across the full delta ladder",
		Discriminates: "an ordering bug that every single-rung test passes",
		Property:      propDepthMonotonic,
	},
}

// ---------------------------------------------------------------- the file

// Layer2Fixture is the on-disk shape of one scenario.
//
// Every decimal is a STRING. A JSON number is a float64 on the way in, and this
// repository bans float64 in the computation for the reason that applies just as
// hard to its fixtures: 0.1 does not survive the trip. A price is a fraction
// rather than a decimal for the same reason, and it is the same n/d that Horizon
// sends in price_r. Rule 5 of the non-negotiables.
type Layer2Fixture struct {
	Scenario int    `json:"scenario"`
	Slug     string `json:"slug"`

	// TestnetTx is the transaction that created this state. REQUIRED, and it is
	// what makes the fixture a reading rather than an assertion: section 2 of
	// the protocol asks for "fixture files plus the testnet transaction that
	// created each", and a scenario nobody can go and look at is a number
	// somebody typed.
	TestnetTx string `json:"testnetTx"`

	LedgerSeq      uint32 `json:"ledgerSeq"`
	LedgerClosedAt string `json:"ledgerClosedAt"`

	Base  layer2Asset `json:"base"`
	Quote layer2Asset `json:"quote"`

	Book  layer2Book   `json:"book"`
	Pools []layer2Pool `json:"pools"`

	// Expected is OPTIONAL and is the hand computation. A fixture with an input
	// and no Expected is a scenario that can be checked against its Property and
	// against nothing else, which is a real intermediate state and is reported
	// as one.
	Expected *Layer2Expected `json:"expected"`
}

type layer2Asset struct {
	Code   string `json:"code"`
	Issuer string `json:"issuer"`
	Type   string `json:"type"` // native, credit_alphanum4, credit_alphanum12
}

type layer2Book struct {
	Bids []layer2Level `json:"bids"`
	Asks []layer2Level `json:"asks"`
}

// layer2Level carries the price as the n/d fraction Horizon sends, never as the
// decimal string beside it. Rule 5.
type layer2Level struct {
	PriceN int64  `json:"priceN"`
	PriceD int64  `json:"priceD"`
	Amount string `json:"amount"`
}

type layer2Pool struct {
	PoolID       string `json:"poolId"`
	ReserveBase  string `json:"reserveBase"`
	ReserveQuote string `json:"reserveQuote"`
	FeeBP        int32  `json:"feeBp"`
}

// Layer2Expected holds the hand computation. Every field is a pointer or a slice
// so that "not computed yet" and "computed to be zero" are different states. On
// a fixture, those two must never collapse into each other.
type Layer2Expected struct {
	PriceSource       *string          `json:"priceSource"`
	MidPrice          *string          `json:"midPrice"`
	SpreadPct         *string          `json:"spreadPct"`
	MaxReachablePrice *string          `json:"maxReachablePrice"`
	Depth             []Layer2DepthRow `json:"depth"`
	Flags             []string         `json:"flags"`
}

type Layer2DepthRow struct {
	Delta    string  `json:"delta"`
	BuySide  *string `json:"buySide"`
	SellSide *string `json:"sellSide"`
	FromSdex *string `json:"fromSdex"`
	FromAmm  *string `json:"fromAmm"`
}

// ErrLayer2Absent is returned when a scenario has no file yet. It is a state and
// not a failure, and the caller decides which of the two it wants to report.
var ErrLayer2Absent = errors.New("layer 2 scenario not provided")

// LoadLayer2 reads one scenario. It returns ErrLayer2Absent when the file is not
// there, and a real error when the file is there and wrong, because those two
// deserve opposite reactions.
func LoadLayer2(dir string, s Layer2Scenario) (*Layer2Fixture, error) {
	path := filepath.Join(dir, fmt.Sprintf("%02d-%s.json", s.ID, s.Slug))
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrLayer2Absent
	}
	if err != nil {
		return nil, fmt.Errorf("layer 2 scenario %d: %w", s.ID, err)
	}

	var f Layer2Fixture
	dec := json.NewDecoder(bytes.NewReader(raw))
	// A key nobody reads is a hand computation nobody checks. Refusing unknown
	// fields turns a typo in a fixture into a failure instead of a silent zero.
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("layer 2 scenario %d, %s: %w", s.ID, path, err)
	}

	if f.Scenario != s.ID {
		return nil, fmt.Errorf("layer 2 %s: file says scenario %d, filename says %d", path, f.Scenario, s.ID)
	}
	if f.Slug != s.Slug {
		return nil, fmt.Errorf("layer 2 %s: file says slug %q, filename says %q", path, f.Slug, s.Slug)
	}
	if f.TestnetTx == "" {
		return nil, fmt.Errorf("layer 2 %s: testnetTx is empty. Section 2 requires the transaction that created the state", path)
	}
	return &f, nil
}

// Snapshot converts a fixture into the domain input. It fails loudly on a
// malformed decimal rather than substituting zero, for the reason dec() gives.
func (f *Layer2Fixture) Snapshot() (domain.Snapshot, error) {
	base, err := f.Base.domain()
	if err != nil {
		return domain.Snapshot{}, fmt.Errorf("base: %w", err)
	}
	quote, err := f.Quote.domain()
	if err != nil {
		return domain.Snapshot{}, fmt.Errorf("quote: %w", err)
	}

	var closed time.Time
	if f.LedgerClosedAt != "" {
		closed, err = time.Parse(time.RFC3339, f.LedgerClosedAt)
		if err != nil {
			return domain.Snapshot{}, fmt.Errorf("ledgerClosedAt: %w", err)
		}
	}

	bids, err := levels(f.Book.Bids)
	if err != nil {
		return domain.Snapshot{}, fmt.Errorf("bids: %w", err)
	}
	asks, err := levels(f.Book.Asks)
	if err != nil {
		return domain.Snapshot{}, fmt.Errorf("asks: %w", err)
	}

	pools := make([]domain.PoolReserves, 0, len(f.Pools))
	for i, p := range f.Pools {
		rb, err := decStrict(p.ReserveBase)
		if err != nil {
			return domain.Snapshot{}, fmt.Errorf("pool %d reserveBase: %w", i, err)
		}
		rq, err := decStrict(p.ReserveQuote)
		if err != nil {
			return domain.Snapshot{}, fmt.Errorf("pool %d reserveQuote: %w", i, err)
		}
		pools = append(pools, domain.PoolReserves{
			PoolID: p.PoolID, ReserveBase: rb, ReserveQuote: rq, FeeBP: p.FeeBP,
		})
	}

	return domain.Snapshot{
		Base: base, Quote: quote,
		LedgerSeq: f.LedgerSeq, LedgerClosedAt: closed,
		Book:  domain.OrderBook{Bids: bids, Asks: asks},
		Pools: pools,
		// Layer 2 states are created by us on testnet, so the reading is a
		// Horizon snapshot of a book we built. It is not a reconstruction.
		Source: domain.DataSourceHorizon,
	}, nil
}

func (a layer2Asset) domain() (domain.Asset, error) {
	var t domain.AssetType
	switch a.Type {
	case "native":
		t = domain.AssetTypeNative
	case "credit_alphanum4":
		t = domain.AssetTypeAlphanum4
	case "credit_alphanum12":
		t = domain.AssetTypeAlphanum12
	default:
		return domain.Asset{}, fmt.Errorf("unknown asset type %q", a.Type)
	}
	// The native asset carries no code and no issuer on chain, and the pair file
	// loader enforces the same thing. See the note in configs/recorder-pairs.json.
	if t == domain.AssetTypeNative && (a.Code != "" || a.Issuer != "") {
		return domain.Asset{}, errors.New("the native asset carries no code and no issuer")
	}
	return domain.Asset{Code: a.Code, Issuer: a.Issuer, Type: t}, nil
}

func levels(in []layer2Level) ([]domain.Level, error) {
	out := make([]domain.Level, 0, len(in))
	for i, l := range in {
		amt, err := decStrict(l.Amount)
		if err != nil {
			return nil, fmt.Errorf("level %d amount: %w", i, err)
		}
		p := domain.Price{N: l.PriceN, D: l.PriceD}
		if !p.Valid() {
			return nil, fmt.Errorf("level %d: price_r %d/%d is not a usable fraction", i, l.PriceN, l.PriceD)
		}
		out = append(out, domain.Level{Price: p, Amount: amt})
	}
	return out, nil
}

// decStrict refuses an empty string instead of reading it as zero. On a fixture
// an omitted figure and a figure of zero are different claims.
func decStrict(s string) (decimal.Decimal, error) {
	if s == "" {
		return decimal.Decimal{}, errors.New("empty, which is not the same as zero")
	}
	return decimal.NewFromString(s)
}

// ---------------------------------------------------------------- properties

func propNoActivePool(s domain.Snapshot, _ domain.AssetRisk, _ domain.Params) error {
	if len(s.ActivePools()) != 0 {
		return fmt.Errorf("the input carries %d active pool(s); this scenario is the no-pool case", len(s.ActivePools()))
	}
	return nil
}

func propSpreadAbsent(_ domain.Snapshot, r domain.AssetRisk, _ domain.Params) error {
	if r.SpreadPct != nil {
		return fmt.Errorf("spreadPct = %s, want absent: a one-sided book has no spread to compute", r.SpreadPct)
	}
	return nil
}

func propPriceSourceIs(want domain.PriceSource) func(domain.Snapshot, domain.AssetRisk, domain.Params) error {
	return func(_ domain.Snapshot, r domain.AssetRisk, _ domain.Params) error {
		if r.PriceSource != want {
			return fmt.Errorf("priceSource = %q, want %q", r.PriceSource, want)
		}
		return nil
	}
}

// propFromAmmZeroAtDelta is scenario 5, and it is the reason Layer 2 exists at
// all. EXACTLY zero, not within Tolerance: a pool priced above the target
// contributes nothing, and a tolerance here would accept the very bug the
// scenario is built to catch.
func propFromAmmZeroAtDelta(delta string) func(domain.Snapshot, domain.AssetRisk, domain.Params) error {
	want := decimal.RequireFromString(delta)
	return func(_ domain.Snapshot, r domain.AssetRisk, _ domain.Params) error {
		for _, d := range r.Depth {
			if !d.Delta.Equal(want) {
				continue
			}
			if !d.FromAmm.IsZero() {
				return fmt.Errorf("fromAmm at delta %s = %s, want exactly 0: the pool is priced above the target, so it contributes nothing. "+
					"A non-zero here is an implementation summing SDEX and AMM independently", delta, d.FromAmm)
			}
			return nil
		}
		return fmt.Errorf("no depth rung at delta %s, so the scenario checked nothing", delta)
	}
}

// propBothPoolsCounted recomputes with each pool alone and requires the pair to
// beat both. It needs no hand figure, which is the point: the claim "both pools
// are counted" is comparative, and a comparison can be made from the input.
func propBothPoolsCounted(s domain.Snapshot, r domain.AssetRisk, p domain.Params) error {
	active := s.ActivePools()
	if len(active) < 2 {
		return fmt.Errorf("the input carries %d active pool(s); this scenario needs two on one pair", len(active))
	}

	both, ok := ammTotal(r)
	if !ok {
		return errors.New("no depth rung carried an AMM contribution, so nothing was compared")
	}

	for i := range active {
		alone := s
		alone.Pools = []domain.PoolReserves{active[i]}
		one, err := domain.ComputeAssetRisk(alone, p)
		if err != nil {
			return fmt.Errorf("recomputing with pool %s alone: %w", active[i].PoolID, err)
		}
		single, ok := ammTotal(one)
		if !ok {
			continue
		}
		if !both.GreaterThan(single) {
			return fmt.Errorf("total fromAmm with both pools is %s, and with pool %s alone it is %s. "+
				"Not greater, so the second pool contributed nothing and an implementation that found one pool and stopped would pass",
				both, active[i].PoolID, single)
		}
	}
	return nil
}

func ammTotal(r domain.AssetRisk) (decimal.Decimal, bool) {
	var sum decimal.Decimal
	var any bool
	for _, d := range r.Depth {
		sum = sum.Add(d.FromAmm)
		any = true
	}
	return sum, any
}

func propFlagRaised(want domain.Flag) func(domain.Snapshot, domain.AssetRisk, domain.Params) error {
	return func(_ domain.Snapshot, r domain.AssetRisk, _ domain.Params) error {
		for _, f := range r.Flags {
			if f == want {
				return nil
			}
		}
		got := make([]string, 0, len(r.Flags))
		for _, f := range r.Flags {
			got = append(got, string(f))
		}
		sort.Strings(got)
		return fmt.Errorf("flag %s not raised; flags were %v", want, got)
	}
}

func propSomeOrderbookRungUnreachable(_ domain.Snapshot, r domain.AssetRisk, _ domain.Params) error {
	for _, m := range r.ManipulationCostOrderbookOnly {
		if !m.Reachable {
			return nil
		}
	}
	return errors.New("every orderbookOnly rung reports reachable; this scenario needs a target above every ask")
}

func propMaxReachableNull(s domain.Snapshot, r domain.AssetRisk, _ domain.Params) error {
	if len(s.ActivePools()) == 0 {
		return errors.New("the input carries no active pool, so the presence rule is not exercised")
	}
	if r.MaxReachablePrice != nil {
		return fmt.Errorf("maxReachablePrice = %s, want null: an active pool is present. Contract 1.4.3, finding P2-16", r.MaxReachablePrice)
	}
	if r.CostToMaxReachablePrice != nil {
		return fmt.Errorf("costToMaxReachablePrice = %s, want null for the same reason", r.CostToMaxReachablePrice)
	}
	return nil
}

// propDepthMonotonic is scenario 10. A wider delta reaches strictly more of the
// book, so neither side may shrink as delta grows. Non-decreasing rather than
// increasing: a gap in the book leaves a rung flat, and that is not a bug.
func propDepthMonotonic(_ domain.Snapshot, r domain.AssetRisk, _ domain.Params) error {
	if len(r.Depth) < 2 {
		return fmt.Errorf("the ladder has %d rung(s); monotonicity needs at least two", len(r.Depth))
	}
	for i := 1; i < len(r.Depth); i++ {
		prev, cur := r.Depth[i-1], r.Depth[i]
		if cur.Delta.LessThanOrEqual(prev.Delta) {
			return fmt.Errorf("rung %d has delta %s, not greater than the previous %s; the ladder is out of order", i, cur.Delta, prev.Delta)
		}
		if cur.BuySide.LessThan(prev.BuySide) {
			return fmt.Errorf("buySide fell from %s at delta %s to %s at delta %s", prev.BuySide, prev.Delta, cur.BuySide, cur.Delta)
		}
		if cur.SellSide.LessThan(prev.SellSide) {
			return fmt.Errorf("sellSide fell from %s at delta %s to %s at delta %s", prev.SellSide, prev.Delta, cur.SellSide, cur.Delta)
		}
	}
	return nil
}
