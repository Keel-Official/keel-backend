// Unit tests for the methodology computations.
//
// WHY THEY ARE HERE AND NOT IN internal/conformance. That package holds the
// golden fixture, and its own header says every number in it comes from
// testdata/fixtures/ustry_pre_exploit.md. Every snapshot in this file is
// SYNTHETIC and chosen so the right answer is known before the code runs, which
// is a different kind of evidence and does not belong beside hand computed
// on-chain state. The two suites answer different questions: conformance asks
// whether the implementation matches the methodology on the real incident, this
// file asks whether it handles the cases the incident happens not to contain.
//
// WHAT THE METHODOLOGY ITSELF DEMANDS OF THIS FILE. Two of the tests below are
// not optional and the documents say so in as many words:
//
//	04-depth.md section 2  "Mandatory sanity assertion in tests: depth_amm ~= (delta / 2) x Y"
//	04-depth.md section 3  "Mandatory discriminating test: a fixture whose pool price
//	                        sits 5 percent above P0, queried for depth at 2 percent.
//	                        The correct answer is fromAmm exactly zero."
//
// The rest cover the edge case list that Keel_Deliverable_1_Rencana_Eksekusi.md
// section D1.2 requires before the depth core counts as done: an empty book, one
// side of a book, an empty pool, two pools on one pair, and an asset with no
// price at all.
//
// NO FLOAT LITERALS ANYWHERE, including in a test. TestArchTanpaFloat scans every
// .go file in this package and does not exempt tests, so a constant is written
// dec("0.02") and never 0.02.
package domain

import (
	"testing"

	"github.com/shopspring/decimal"
)

// ---------------------------------------------------------------- helpers

// unitTolerance is for comparing against a value the methodology states to three
// significant figures, such as the (delta / 2) x Y rule of thumb table. It is
// deliberately looser than internal/conformance.Tolerance, which compares against
// exact hand computed values, and the two must not be confused.
var unitTolerance = dec("0.0001")

func near(t *testing.T, label string, got, want, tol decimal.Decimal) {
	t.Helper()
	if got.Sub(want).Abs().GreaterThan(tol) {
		t.Errorf("%s = %s, want %s (tolerance %s)", label, got, want, tol)
	}
}

func level(n, d int64, amount string) Level {
	return Level{Price: Price{N: n, D: d}, Amount: dec(amount)}
}

func pool(id, base, quote string, feeBP int32) PoolReserves {
	return PoolReserves{
		PoolID:       id,
		ReserveBase:  dec(base),
		ReserveQuote: dec(quote),
		FeeBP:        feeBP,
	}
}

// snap builds a synthetic snapshot. The assets are placeholders: nothing in this
// package reads them, which is itself worth asserting by never varying them.
func snap(book OrderBook, pools ...PoolReserves) Snapshot {
	return Snapshot{
		Base:      Asset{Code: "TEST", Issuer: "GTEST", Type: AssetTypeAlphanum4},
		Quote:     Asset{Code: "USDC", Issuer: "GUSDC", Type: AssetTypeAlphanum4},
		LedgerSeq: 1,
		Book:      book,
		Pools:     pools,
		Source:    DataSourceHorizon,
	}
}

func marketDeltas() []decimal.Decimal {
	return []decimal.Decimal{dec("0.02"), dec("0.05"), dec("0.10")}
}

// ---------------------------------------------------------------- 04-depth.md section 2

// TestAmmDepthMatchesTheRuleOfThumb is the mandatory sanity assertion.
//
// It checks the AMM term against the table 04-depth.md section 2 prints, which is
// the closed form expressed as a percentage of the quote reserve. That table is
// the reason the assertion is worth having: it is derived from the constant
// product identity rather than from this code, so a sign error or an inverted
// ratio cannot satisfy both.
//
// The fee is ZERO here on purpose. The table is fee free, and mixing the gross-up
// into the same assertion would test two rules at once and locate neither when it
// failed. The fee has its own test below.
func TestAmmDepthMatchesTheRuleOfThumb(t *testing.T) {
	// Spot price exactly 1, so P_target is numerically equal to (1 + delta) and
	// the percentages below can be read straight off the table.
	p := pool("pool-a", "1000", "1000", 0)
	s := snap(OrderBook{}, p)

	got, err := ComputeDepth(s, one, marketDeltas())
	if err != nil {
		t.Fatalf("ComputeDepth: %v", err)
	}

	// Percent of Y, from 04-depth.md section 2.
	cases := []struct {
		delta    string
		upPct    string
		downPct  string
		ruleOfTh string // (delta / 2) x Y, the approximation the doc names
	}{
		{"0.02", "0.995", "1.005", "10"},
		{"0.05", "2.47", "2.53", "25"},
		{"0.10", "4.88", "5.13", "50"},
	}

	for i, c := range cases {
		y := p.ReserveQuote
		gotUpPct := got[i].BuySide.DivRound(y, Precision).Mul(hundred)
		gotDownPct := got[i].SellSide.DivRound(y, Precision).Mul(hundred)

		// The table is printed to three significant figures, so the comparison
		// is to half of the last digit it shows.
		near(t, "buy side at delta "+c.delta+", percent of Y", gotUpPct, dec(c.upPct), dec("0.005"))
		near(t, "sell side at delta "+c.delta+", percent of Y", gotDownPct, dec(c.downPct), dec("0.005"))

		// And the rule of thumb itself, which is an APPROXIMATION. Its error
		// grows with delta because the square root is concave: at 2 percent the
		// gap is half a percent and at 10 percent it is 2.4, so a tight relative
		// bound here would be asserting that the approximation is exact.
		//
		// The DIRECTION is the part worth pinning down, and it is not arbitrary.
		// sqrt(1 + d) - 1 is always below d / 2 and 1 - sqrt(1 - d) is always
		// above it, so the buy side sits under the rule of thumb and the sell
		// side over it, on every delta and every pool. That asymmetry is the same
		// one the up and down columns of the table above record, and an
		// implementation that mixed the two formulas up would satisfy a symmetric
		// tolerance while failing this.
		rule := dec(c.ruleOfTh)
		relative := got[i].BuySide.Sub(rule).Abs().DivRound(rule, Precision)
		if relative.GreaterThan(dec("0.03")) {
			t.Errorf("depth_amm at delta %s is %s, which is %s away from the rule of thumb %s in relative terms",
				c.delta, got[i].BuySide, relative, rule)
		}
		if !got[i].BuySide.LessThan(rule) {
			t.Errorf("buy side at delta %s is %s and the rule of thumb is %s; sqrt(1+d)-1 is always below d/2, so it has to be smaller",
				c.delta, got[i].BuySide, rule)
		}
		if !got[i].SellSide.GreaterThan(rule) {
			t.Errorf("sell side at delta %s is %s and the rule of thumb is %s; 1-sqrt(1-d) is always above d/2, so it has to be larger",
				c.delta, got[i].SellSide, rule)
		}
	}
}

// ---------------------------------------------------------------- 04-depth.md section 3

// TestPoolAboveTargetContributesExactlyZero is the mandatory discriminating test.
//
// A pool priced 5 percent above P0, queried at 2 percent. The buy target is 1.02
// and the pool does not start selling until 1.05, so the correct answer is not
// "small", it is EXACTLY zero. The word exactly is what makes this test
// discriminating: an implementation that computed the curve unconditionally and
// let the arithmetic come out negative, or one that took an absolute value, would
// both pass a tolerance and fail here.
func TestPoolAboveTargetContributesExactlyZero(t *testing.T) {
	// P0 is 1. The pool spot is 1050 / 1000 = 1.05.
	s := snap(OrderBook{
		Bids: []Level{level(99, 100, "10")},
		Asks: []Level{level(101, 100, "10")},
	}, pool("pool-high", "1000", "1050", 30))

	got, err := ComputeDepth(s, one, []decimal.Decimal{dec("0.02")})
	if err != nil {
		t.Fatalf("ComputeDepth: %v", err)
	}
	if !got[0].FromAmm.IsZero() {
		t.Errorf("fromAmm at delta 0.02 = %s, want exactly zero: the buy target is 1.02 and the pool spot is 1.05, so the pool sells nothing before the limit",
			got[0].FromAmm)
	}

	// And the same pool DOES contribute once the target passes it, which is what
	// makes the zero above a boundary and not a broken branch.
	got, err = ComputeDepth(s, one, []decimal.Decimal{dec("0.10")})
	if err != nil {
		t.Fatalf("ComputeDepth: %v", err)
	}
	if got[0].FromAmm.LessThanOrEqual(decimal.Zero) {
		t.Errorf("fromAmm at delta 0.10 = %s, want positive: the buy target is 1.10 and the pool spot is 1.05", got[0].FromAmm)
	}
}

// ---------------------------------------------------------------- fee direction

// TestFeeGrossesUpTheBuySideAndReducesTheSellSide guards the correction 1.0.3
// made, which 04-depth.md section 2 records: the buy side is an INPUT and is
// grossed up by / (1 - f), the sell side is an OUTPUT and is reduced by x (1 - f).
//
// Getting this backwards overstates sell side depth, and sell side depth is the
// side liquidation risk lives on, so the error would point in the one direction
// this product must never err in.
func TestFeeGrossesUpTheBuySideAndReducesTheSellSide(t *testing.T) {
	free := snap(OrderBook{}, pool("p", "1000", "1000", 0))
	charged := snap(OrderBook{}, pool("p", "1000", "1000", 30))

	f, err := ComputeDepth(free, one, []decimal.Decimal{dec("0.10")})
	if err != nil {
		t.Fatalf("ComputeDepth, fee free: %v", err)
	}
	c, err := ComputeDepth(charged, one, []decimal.Decimal{dec("0.10")})
	if err != nil {
		t.Fatalf("ComputeDepth, 30 bp: %v", err)
	}

	if !c[0].BuySide.GreaterThan(f[0].BuySide) {
		t.Errorf("buy side with a 30 bp fee is %s and fee free is %s; the fee must gross the INPUT up, so the first has to be larger",
			c[0].BuySide, f[0].BuySide)
	}
	if !c[0].SellSide.LessThan(f[0].SellSide) {
		t.Errorf("sell side with a 30 bp fee is %s and fee free is %s; the fee must reduce the OUTPUT, so the first has to be smaller",
			c[0].SellSide, f[0].SellSide)
	}
}

// ---------------------------------------------------------------- several pools

// TestTwoPoolsOnOnePairAreBothCounted. The edge case list names it, and the
// failure it guards against is real: an implementation that took the deepest pool
// for the price and then forgot the others when walking the curve would pass
// every single-pool test in this file.
func TestTwoPoolsOnOnePairAreBothCounted(t *testing.T) {
	single := snap(OrderBook{}, pool("a", "1000", "1000", 0))
	double := snap(OrderBook{},
		pool("a", "1000", "1000", 0),
		pool("b", "1000", "1000", 0),
	)

	s1, err := ComputeDepth(single, one, []decimal.Decimal{dec("0.10")})
	if err != nil {
		t.Fatalf("ComputeDepth, one pool: %v", err)
	}
	s2, err := ComputeDepth(double, one, []decimal.Decimal{dec("0.10")})
	if err != nil {
		t.Fatalf("ComputeDepth, two pools: %v", err)
	}

	// Identical pools, so the second must contribute exactly as much as the first.
	near(t, "two identical pools against one", s2[0].FromAmm, s1[0].FromAmm.Mul(two), unitTolerance)
}

// TestPoolOrderDoesNotChangeTheResult is NFR-9 at the level of one function.
// Horizon does not promise an order for the pools it returns, and a depth figure
// that moved when the same two pools arrived the other way round would be
// irreproducible without anything in the market having changed.
func TestPoolOrderDoesNotChangeTheResult(t *testing.T) {
	a := pool("aaa", "1000", "1000", 30)
	b := pool("bbb", "2000", "3000", 30)

	forward, err := ComputeDepth(snap(OrderBook{}, a, b), one, marketDeltas())
	if err != nil {
		t.Fatalf("ComputeDepth, forward: %v", err)
	}
	reverse, err := ComputeDepth(snap(OrderBook{}, b, a), one, marketDeltas())
	if err != nil {
		t.Fatalf("ComputeDepth, reverse: %v", err)
	}
	for i := range forward {
		if forward[i].BuySide.Cmp(reverse[i].BuySide) != 0 {
			t.Errorf("delta %s: buy side is %s when the pools arrive in one order and %s in the other",
				forward[i].Delta, forward[i].BuySide, reverse[i].BuySide)
		}
	}
}

// ---------------------------------------------------------------- the price ladder

// TestMidPriceLadder walks all five rungs of 03-reference-price.md section 1.
func TestMidPriceLadder(t *testing.T) {
	p := DefaultParams()

	twoSided := OrderBook{
		Bids: []Level{level(99, 100, "10")},
		Asks: []Level{level(101, 100, "10")},
	}
	askOnly := OrderBook{Asks: []Level{level(101, 100, "10")}}

	// Rung 1, the two sources disagree. Pool spot 50, book mid 1.
	p0, src, spot, div := MidPrice(snap(twoSided, pool("p", "1000", "50000", 30)), p)
	if src != PriceSourcePool {
		t.Errorf("rung 1: priceSource = %q, want %q; the book mid is 1 and the pool spot is 50", src, PriceSourcePool)
	}
	if p0.Cmp(dec("50")) != 0 {
		t.Errorf("rung 1: P0 = %s, want 50, the pool spot", p0)
	}
	if spot == nil || div == nil {
		t.Error("rung 1: poolSpotPrice and priceDivergencePct must both be reported when a pool is present")
	}

	// Rung 1 again, the two sources agree. Pool spot 1.01, book mid 1, a
	// divergence of 1 percent against a threshold of 10.
	p0, src, spot, div = MidPrice(snap(twoSided, pool("p", "1000", "1010", 30)), p)
	if src != PriceSourceBook {
		t.Errorf("rung 1 agreeing: priceSource = %q, want %q; a 1 percent divergence is inside the threshold", src, PriceSourceBook)
	}
	if p0.Cmp(one) != 0 {
		t.Errorf("rung 1 agreeing: P0 = %s, want 1, the book mid", p0)
	}
	if spot == nil || div == nil {
		t.Error("rung 1 agreeing: both are still reported, regardless of which branch was taken")
	}

	// Rung 2, two-sided book and no pool.
	p0, src, spot, div = MidPrice(snap(twoSided), p)
	if src != PriceSourceBook || p0.Cmp(one) != 0 {
		t.Errorf("rung 2: got P0 %s from %q, want 1 from %q", p0, src, PriceSourceBook)
	}
	if spot != nil || div != nil {
		t.Error("rung 2: there is no pool, so both must be nil rather than zero")
	}

	// Rung 3, one side of a book and a pool.
	p0, src, _, div = MidPrice(snap(askOnly, pool("p", "1000", "3000", 30)), p)
	if src != PriceSourcePool || p0.Cmp(dec("3")) != 0 {
		t.Errorf("rung 3: got P0 %s from %q, want 3 from %q", p0, src, PriceSourcePool)
	}
	if div != nil {
		t.Errorf("rung 3: priceDivergencePct = %v, want nil; there is no book mid to diverge from", *div)
	}

	// Rung 4, a pool and no book at all.
	p0, src, _, _ = MidPrice(snap(OrderBook{}, pool("p", "1000", "3000", 30)), p)
	if src != PriceSourcePool || p0.Cmp(dec("3")) != 0 {
		t.Errorf("rung 4: got P0 %s from %q, want 3 from %q", p0, src, PriceSourcePool)
	}

	// Rung 5, nothing at all. Not an error.
	p0, src, spot, div = MidPrice(snap(OrderBook{}), p)
	if src != PriceSourceNone {
		t.Errorf("rung 5: priceSource = %q, want %q", src, PriceSourceNone)
	}
	if !p0.IsZero() || spot != nil || div != nil {
		t.Errorf("rung 5: got P0 %s, spot %v, divergence %v; nothing is defined here", p0, spot, div)
	}
}

// TestEmptyPoolIsNotAPool. A pool with a zero reserve can price nothing, and
// treating it as present would put priceSource at pool with a spot of zero, which
// is the worst of both answers.
func TestEmptyPoolIsNotAPool(t *testing.T) {
	drained := pool("empty", "0", "1000", 30)
	_, src, spot, _ := MidPrice(snap(OrderBook{}, drained), DefaultParams())
	if src != PriceSourceNone {
		t.Errorf("priceSource = %q, want %q: the only pool has no base reserve", src, PriceSourceNone)
	}
	if spot != nil {
		t.Errorf("poolSpotPrice = %v, want nil: a drained pool is not an active pool", *spot)
	}
}

// ---------------------------------------------------------------- manipulation

// TestCostAndReachableReadDisjointSets is the rule that
// 05-manipulation-cost.md section 1 states and that an earlier version of the
// conformance test got wrong on two rows. Cost sums asks strictly BELOW the
// target; Reachable asks whether one exists at or ABOVE it. No ask is ever in
// both sets.
func TestCostAndReachableReadDisjointSets(t *testing.T) {
	// Asks at 100 and 200, one unit each.
	s := snap(OrderBook{Asks: []Level{
		level(100, 1, "1"),
		level(200, 1, "1"),
	}})

	// P0 = 100, so delta 0.5 puts the target at 150.
	got, err := ComputeManipulationCost(s, dec("100"), []decimal.Decimal{dec("0.5")}, false)
	if err != nil {
		t.Fatalf("ComputeManipulationCost: %v", err)
	}
	if got[0].Cost.Cmp(dec("100")) != 0 {
		t.Errorf("Cost = %s, want 100: only the ask at 100 is strictly below the target of 150", got[0].Cost)
	}
	if !got[0].Reachable {
		t.Error("Reachable is false, want true: the ask at 200 sits above the target of 150")
	}

	// Target 250, above every ask. Both asks now count toward Cost and nothing
	// makes the target reachable.
	got, err = ComputeManipulationCost(s, dec("100"), []decimal.Decimal{dec("1.5")}, false)
	if err != nil {
		t.Fatalf("ComputeManipulationCost: %v", err)
	}
	if got[0].Cost.Cmp(dec("300")) != 0 {
		t.Errorf("Cost = %s, want 300: both asks are below the target of 250", got[0].Cost)
	}
	if got[0].Reachable {
		t.Error("Reachable is true, want false: the book runs out at 200 and the target is 250")
	}
}

// TestAnActivePoolMakesEveryTargetReachable. Under a constant product curve the
// price tends to infinity as the base reserve tends to zero, so the combined
// variant can never report an unreachable target.
func TestAnActivePoolMakesEveryTargetReachable(t *testing.T) {
	s := snap(OrderBook{Asks: []Level{level(100, 1, "1")}}, pool("p", "1000", "100000", 30))

	combined, err := ComputeManipulationCost(s, dec("100"), []decimal.Decimal{dec("100")}, true)
	if err != nil {
		t.Fatalf("ComputeManipulationCost, combined: %v", err)
	}
	if !combined[0].Reachable {
		t.Error("combined Reachable is false at delta 100, want true: an active pool has no highest price")
	}

	orderbookOnly, err := ComputeManipulationCost(s, dec("100"), []decimal.Decimal{dec("100")}, false)
	if err != nil {
		t.Fatalf("ComputeManipulationCost, orderbookOnly: %v", err)
	}
	if orderbookOnly[0].Reachable {
		t.Error("orderbookOnly Reachable is true, want false: the book holds one ask at 100 and the target is 10100")
	}
	if orderbookOnly[0].Cost.GreaterThan(combined[0].Cost) {
		t.Errorf("orderbookOnly cost %s exceeds combined %s, which is impossible: combined is the same book plus an AMM term",
			orderbookOnly[0].Cost, combined[0].Cost)
	}
}

// ---------------------------------------------------------------- collateral

// TestReachableGuardDropsTheManipulationTerm is the guard 08-collateral.md calls
// mandatory. When the critical target cannot be reached, Cost is not the cost of
// reaching anything, so multiplying it by m would produce a number with no
// meaning. C_max must fall back to the liquidation term and say so.
func TestReachableGuardDropsTheManipulationTerm(t *testing.T) {
	p := DefaultParams()
	depth := []DepthPoint{{Delta: p.LiquidationDelta, SellSide: dec("1000")}}

	unreachable := []ManipulationPoint{
		{Delta: p.ManipulationCriticalDelta, Cost: dec("40"), Reachable: false},
	}
	cmax, liq, man, warnings := ComputeMaxSafeCollateral(depth, unreachable, p)

	if man != nil {
		t.Errorf("manipulationLimit = %v, want nil: the critical target is unreachable through the order book", *man)
	}
	if liq == nil || liq.Cmp(dec("500")) != 0 {
		t.Errorf("liquidationLimit = %v, want 500, which is 1000 x the 0.5 haircut", liq)
	}
	if cmax == nil || cmax.Cmp(dec("500")) != 0 {
		t.Errorf("C_max = %v, want 500: with no manipulation term it falls back to the liquidation term alone", cmax)
	}
	if len(warnings) == 0 {
		t.Error("no warning was emitted; a term silently dropped is indistinguishable from a term that was applied")
	}

	// The same numbers with a reachable target. 40 x 0.25 = 10, which is below
	// 500, so the manipulation term binds and C_max drops to it.
	reachable := []ManipulationPoint{
		{Delta: p.ManipulationCriticalDelta, Cost: dec("40"), Reachable: true},
	}
	cmax, _, man, warnings = ComputeMaxSafeCollateral(depth, reachable, p)
	if man == nil || man.Cmp(dec("10")) != 0 {
		t.Errorf("manipulationLimit = %v, want 10", man)
	}
	if cmax == nil || cmax.Cmp(dec("10")) != 0 {
		t.Errorf("C_max = %v, want 10: the manipulation term is the smaller of the two and it binds", cmax)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none: both terms applied", warnings)
	}
}

// TestMissingRungIsNotASilentSubstitution. A ladder without the rung C_max needs
// must produce no number rather than the nearest one, because a figure computed
// at 0.05 while claiming 0.10 is wrong while looking right.
func TestMissingRungIsNotASilentSubstitution(t *testing.T) {
	p := DefaultParams()
	wrongRung := []DepthPoint{{Delta: dec("0.05"), SellSide: dec("1000")}}

	cmax, liq, man, warnings := ComputeMaxSafeCollateral(wrongRung, nil, p)
	if cmax != nil || liq != nil || man != nil {
		t.Errorf("got C_max %v, liquidation %v, manipulation %v; all three must be nil when the liquidation rung is absent", cmax, liq, man)
	}
	if len(warnings) == 0 {
		t.Error("no warning was emitted for a ladder with no rung at the liquidation delta")
	}
}

// ---------------------------------------------------------------- edge cases

// TestEveryEdgeCaseReturnsAResult walks the list in D1.2 of the execution plan.
// None of these may error and none may panic. FR-5 puts it the same way: every
// edge case without an exception.
//
// An asset with no executable price is the case worth stating separately. It is
// not a row missing from the report, it is the most dangerous finding this
// product can make, so it has to come back as a CRITICAL result.
func TestEveryEdgeCaseReturnsAResult(t *testing.T) {
	p := DefaultParams()

	cases := []struct {
		name string
		s    Snapshot
	}{
		{"empty book, no pool", snap(OrderBook{})},
		{"bid side only", snap(OrderBook{Bids: []Level{level(99, 100, "10")}})},
		{"ask side only", snap(OrderBook{Asks: []Level{level(101, 100, "10")}})},
		{"empty pool, no book", snap(OrderBook{}, pool("drained", "0", "0", 30))},
		{"pool only", snap(OrderBook{}, pool("p", "1000", "1000", 30))},
		{"two pools on one pair", snap(OrderBook{}, pool("a", "1000", "1000", 30), pool("b", "500", "600", 30))},
		{"book and pool together", snap(OrderBook{
			Bids: []Level{level(99, 100, "10")},
			Asks: []Level{level(101, 100, "10")},
		}, pool("p", "1000", "1000", 30))},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			risk, err := ComputeAssetRisk(c.s, p)
			if err != nil {
				t.Fatalf("ComputeAssetRisk: %v", err)
			}
			if risk.LedgerSeq != c.s.LedgerSeq {
				t.Errorf("ledgerSeq = %d, want %d; rule 1 says every output carries it", risk.LedgerSeq, c.s.LedgerSeq)
			}
			if risk.MethodologyVersion == "" {
				t.Error("methodologyVersion is empty; rule 1 says every output carries it")
			}
			if risk.Band == "" {
				t.Error("band is empty; every result falls into a band, including this one")
			}
		})
	}
}

// TestNoExecutablePriceIsACriticalResultAndNotAnError. 03-reference-price.md
// section 1: "Case 5 is not an error. An asset with no executable price is the
// highest-value finding Keel can produce."
func TestNoExecutablePriceIsACriticalResultAndNotAnError(t *testing.T) {
	risk, err := ComputeAssetRisk(snap(OrderBook{}), DefaultParams())
	if err != nil {
		t.Fatalf("ComputeAssetRisk returned an error, and this case is a result: %v", err)
	}
	if risk.PriceSource != PriceSourceNone {
		t.Errorf("priceSource = %q, want %q", risk.PriceSource, PriceSourceNone)
	}
	if risk.MidPrice != nil {
		t.Errorf("midPrice = %v, want nil: there is no executable price to report", *risk.MidPrice)
	}
	if risk.Band != BandCritical {
		t.Errorf("band = %q, want %q", risk.Band, BandCritical)
	}
	if !hasFlag(risk.Flags, FlagNoExecutablePrice) {
		t.Errorf("flags = %v, want %s among them", risk.Flags, FlagNoExecutablePrice)
	}
	if len(risk.Warnings) == 0 {
		t.Error("every ladder is empty and no warning says why")
	}
	// The depth flags cannot be judged without a P0, so they are unevaluated
	// rather than reported as a measured zero. Zero means measured to be zero and
	// that is a different claim.
	for _, f := range []Flag{FlagZeroDepth2Pct, FlagThinDepth5Pct, FlagManipulationCheap} {
		if !hasFlag(risk.UnevaluatedFlags, f) {
			t.Errorf("%s is not among the unevaluated flags %v, although no ladder was computed", f, risk.UnevaluatedFlags)
		}
	}
}

func hasFlag(fs []Flag, want Flag) bool {
	for _, f := range fs {
		if f == want {
			return true
		}
	}
	return false
}
