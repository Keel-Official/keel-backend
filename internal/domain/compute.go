// The methodology computations.
//
// ZONE: YELLOW since 25 August 2026, moved so that Deliverable 1 would not be gated
// on a single writer. It was the red zone until then, and the header that said so
// outlived the move by a day. Edit and Write on this path are `ask` in
// .claude/settings.json rather than `deny`, so every write still surfaces, and
// .claude/hooks/lindungi-zona-merah.sh no longer carries a file rule or a directory
// rule for internal/domain. internal/domain/CLAUDE.md is the long account of the
// move and DEC-008 is the record that governs it, accepted 31 August 2026 after
// six days in which nothing did. Read DEC-008 section 6 before adding a function:
// it names which of these functions have a hand computed oracle and which do not,
// and that list is the price of this file being writable.
//
// WHAT REPLACED THE LOCK, and it is a rule no mechanism can enforce. A function
// here may only be written AFTER its expected values exist in testdata/fixtures.
// The fixture is red and hook-locked precisely so that the numbers this file is
// judged against were derived without reference to it. Nothing can prove the
// order was followed, which is why it is written down in both places rather than
// trusted once.
//
// WHICH FUNCTIONS BELOW HAVE A HAND COMPUTED ORACLE, because it is not all of
// them and the difference decides how much the green suite is worth:
//
//	MidPrice                  YES, for the two-sided-book rung. P0 = 53.8971414
//	ComputeDepth              SDEX walk YES (as a correct zero). AMM term NO
//	ComputeManipulationCost   orderbookOnly YES, four rows. includeAMM NO
//	ComputeMaxSafeCollateral  NO. 08-collateral.md is complete, the fixture is silent
//
// The AMM half is implemented from docs/methodology/04-depth.md section 2 and 3
// and is checked only by the invariants in internal/conformance, never by a
// number computed by hand. testdata/fixtures/ustry_pre_exploit.md line 30 still
// records `Pools: []` while GoldenSnapshot carries the pool that genuinely
// existed, so the with-pool tables do not exist yet. That is handoff item B-4 and
// it is Al's. Until it closes, agreement between this file and DEC-006 section 2
// proves only that two readings of the same document agree, which is not
// independent verification.
//
// Definitions live in docs/methodology/, one file per subject, indexed by
// 00-overview.md. Where this file and those documents disagree, the documents are
// right and this file has a bug.
package domain

import (
	"fmt"
	"sort"

	"github.com/shopspring/decimal"
)

// Precision is the working precision, in decimal places, for every division and
// every square root in this package.
//
// docs/methodology/04-depth.md section 2 requires it: "Square roots are computed
// at a fixed decimal precision. That precision and its tolerance are named
// constants and form part of the methodology, not an implementation detail." This
// is the precision half. The TOLERANCE half has no owner yet: internal/conformance
// declares its own Tolerance of 1e-7 for comparing fixture values, which is a test
// concern, and the methodology asks for a constant of the methodology. Naming one
// here would be inventing a number the paid deliverable has not decided, so it is
// left open and recorded instead.
//
// Why 28. It is beyond the 16 significant digits any Stellar quantity carries
// (amounts are int64 stroops, prices are exact int32 rationals), so the rounding
// this constant governs never reaches a digit that came off the ledger.
const Precision int32 = 28

// The constants this file divides and compares against, written once. They are
// decimal rather than literals because a float literal is refused by
// TestArchTanpaFloat and because decimal.NewFromInt cannot express 10000 as a
// scale without a second conversion at every call site.
var (
	one         = dec("1")
	two         = dec("2")
	hundred     = dec("100")
	tenThousand = dec("10000")
	half        = dec("0.5")
)

// sqrt returns the square root at Precision, in the decimal domain.
//
// PowWithPrecision is used rather than a hand-rolled Newton iteration because it
// is the library's own routine, is deterministic across runs for the same input,
// and keeps the iteration count out of this file. It errors only on a negative
// base, which every caller here excludes before calling.
func sqrt(v decimal.Decimal) (decimal.Decimal, error) {
	if v.IsNegative() {
		return decimal.Zero, fmt.Errorf("square root of a negative value %s", v)
	}
	if v.IsZero() {
		return decimal.Zero, nil
	}
	r, err := v.PowWithPrecision(half, Precision)
	if err != nil {
		return decimal.Zero, fmt.Errorf("square root of %s: %w", v, err)
	}
	return r, nil
}

// feeFraction is FeeBP expressed as a fraction, so 30 basis points becomes 0.003.
// types.go is explicit that 30 must not be hardcoded: Stellar permits other values
// and a hardcoded fee is silently wrong on a different pool.
func feeFraction(p PoolReserves) decimal.Decimal {
	return decimal.NewFromInt32(p.FeeBP).DivRound(tenThousand, Precision)
}

// sortedActivePools returns the non-empty pools in POOL ID ORDER.
//
// The order is not cosmetic. NFR-9 requires two runs over the same snapshot to
// produce byte for byte identical JSON, and Horizon does not promise a stable
// order for the pools it returns. Rule 2 in CLAUDE.md says to sort before
// iterating, and this is that sort.
func sortedActivePools(s Snapshot) []PoolReserves {
	pools := s.ActivePools()
	sort.Slice(pools, func(i, j int) bool { return pools[i].PoolID < pools[j].PoolID })
	return pools
}

// deepestPool returns the pool with the largest quote reserve, which
// docs/methodology/03-reference-price.md section 1 names as the source of
// pool_spot when more than one pool exists. Ties break on PoolID, for the
// determinism reason above.
func deepestPool(s Snapshot) (PoolReserves, bool) {
	pools := sortedActivePools(s)
	if len(pools) == 0 {
		return PoolReserves{}, false
	}
	best := pools[0]
	for _, p := range pools[1:] {
		if p.ReserveQuote.GreaterThan(best.ReserveQuote) {
			best = p
		}
	}
	return best, true
}

// bestBid returns the highest bid price and bestAsk the lowest ask price.
//
// They scan rather than reading index 0, which is what OrderBook.BestBid and
// BestAsk do. Horizon serves a sorted book, but an offers-implied book is
// reconstructed by replaying operations and carries no such promise, and a
// reference price that depends on the order a slice happened to arrive in is a
// reference price that changes without the market changing.
func bestBid(b OrderBook) (decimal.Decimal, bool) {
	if len(b.Bids) == 0 {
		return decimal.Zero, false
	}
	best := b.Bids[0].Price.Decimal()
	for _, l := range b.Bids[1:] {
		if p := l.Price.Decimal(); p.GreaterThan(best) {
			best = p
		}
	}
	return best, true
}

func bestAsk(b OrderBook) (decimal.Decimal, bool) {
	if len(b.Asks) == 0 {
		return decimal.Zero, false
	}
	best := b.Asks[0].Price.Decimal()
	for _, l := range b.Asks[1:] {
		if p := l.Price.Decimal(); p.LessThan(best) {
			best = p
		}
	}
	return best, true
}

// ---------------------------------------------------------------- Price

// MidPrice applies the fallback order in docs/methodology/03-reference-price.md section 1.
// It also returns the pool spot price and their divergence when a pool is available.
//
// The divergence is nil when a pool exists but the book has fewer than two sides.
// types.go says both are "always populated when an active pool exists", and that
// sentence cannot be satisfied for the divergence, because a divergence is a
// comparison against a book mid that does not exist on a one-sided book. Nil means
// undefined here, which is the convention everywhere else in this package, and
// zero would claim the two sources agree.
func MidPrice(s Snapshot, p Params) (p0 decimal.Decimal, src PriceSource, poolSpot, divergence *decimal.Decimal) {
	bid, hasBid := bestBid(s.Book)
	ask, hasAsk := bestAsk(s.Book)
	pool, hasPool := deepestPool(s)

	if hasPool {
		spot := pool.SpotPrice()
		poolSpot = &spot
	}

	// Rungs 1 and 2: a two-sided book has a mid.
	if hasBid && hasAsk {
		bookMid := bid.Add(ask).DivRound(two, Precision)

		if !hasPool {
			return bookMid, PriceSourceBook, nil, nil
		}

		div := bookMid.Sub(*poolSpot).Abs().DivRound(*poolSpot, Precision).Mul(hundred)
		divergence = &div

		// Rung 1. Trust the source backed by executable liquidity, and say that
		// they disagree. A mid with a several hundred percent spread is not
		// executable by anyone.
		if div.GreaterThan(p.Thresholds.PriceDivergencePct) {
			return *poolSpot, PriceSourcePool, poolSpot, divergence
		}
		return bookMid, PriceSourceBook, poolSpot, divergence
	}

	// Rungs 3 and 4: one side or no book at all, but a pool prices it.
	if hasPool {
		return *poolSpot, PriceSourcePool, poolSpot, nil
	}

	// Rung 5. Not an error. An asset with no executable price is the
	// highest-value finding Keel can produce.
	return decimal.Zero, PriceSourceNone, nil, nil
}

// spreadPct is (best_ask - best_bid) / P0 x 100, per 03-reference-price.md
// section 2. It is nil when either side of the book is empty, because the
// difference is undefined. Null means unknown, not zero.
func spreadPct(b OrderBook, p0 decimal.Decimal) *decimal.Decimal {
	bid, hasBid := bestBid(b)
	ask, hasAsk := bestAsk(b)
	if !hasBid || !hasAsk || p0.LessThanOrEqual(decimal.Zero) {
		return nil
	}
	v := ask.Sub(bid).DivRound(p0, Precision).Mul(hundred)
	return &v
}

// ---------------------------------------------------------------- Depth

// ammBuyNotional is the quote an attacker must PUT IN to walk one pool up to
// target, from 04-depth.md sections 2 and 3:
//
//	0                                  if P_pool >= P_target
//	Y x (sqrt(P_target / P_pool) - 1)  otherwise, then grossed up by / (1 - f)
//
// The gross-up is because the quantity is an input. The fee always favours the
// pool, never the counterparty.
func ammBuyNotional(pool PoolReserves, target decimal.Decimal) (decimal.Decimal, error) {
	spot := pool.SpotPrice()
	if spot.IsZero() || spot.GreaterThanOrEqual(target) {
		return decimal.Zero, nil
	}
	ratio, err := sqrt(target.DivRound(spot, Precision))
	if err != nil {
		return decimal.Zero, err
	}
	net := pool.ReserveQuote.Mul(ratio.Sub(one))
	return net.DivRound(one.Sub(feeFraction(pool)), Precision), nil
}

// ammSellNotional is the quote RECEIVED for walking one pool down to target. It
// is the mirror of ammBuyNotional and the fee treatment is deliberately the
// opposite one:
//
//	0                                  if P_pool <= P_target
//	Y x (1 - sqrt(P_target / P_pool))  otherwise, then reduced by x (1 - f)
//
// Multiplied rather than divided because this quantity is an output. 04-depth.md
// section 2 calls this out as a 1.0.3 correction; getting it backwards overstates
// sell side depth, which is the side liquidation risk lives on.
//
// A target at or below zero, which happens once delta reaches 1, means draining
// the pool of base entirely. sqrt(0) is 0 and the whole quote reserve comes out,
// so the target is clamped rather than passed to sqrt as a negative.
func ammSellNotional(pool PoolReserves, target decimal.Decimal) (decimal.Decimal, error) {
	spot := pool.SpotPrice()
	if spot.IsZero() || spot.LessThanOrEqual(target) {
		return decimal.Zero, nil
	}
	t := target
	if t.IsNegative() {
		t = decimal.Zero
	}
	ratio, err := sqrt(t.DivRound(spot, Precision))
	if err != nil {
		return decimal.Zero, err
	}
	gross := pool.ReserveQuote.Mul(one.Sub(ratio))
	return gross.Mul(one.Sub(feeFraction(pool))), nil
}

// ComputeDepth computes the market quality ladder, merging SDEX and AMM at the same
// final marginal price. See docs/methodology/04-depth.md section 3.
//
// The two venues are NOT summed independently. Both are bounded by the same
// P_target, which is what makes the sum legitimate: each term is the notional that
// venue absorbs before the marginal price reaches one shared limit.
//
// A level that crosses the boundary is discarded ENTIRELY, never taken partially.
// That yields a figure slightly below the theoretical one, deliberately, under the
// conservative principle in 11-limitations.md.
func ComputeDepth(s Snapshot, p0 decimal.Decimal, deltas []decimal.Decimal) ([]DepthPoint, error) {
	if p0.LessThanOrEqual(decimal.Zero) {
		return nil, fmt.Errorf("depth needs a positive reference price, got %s", p0)
	}
	pools := sortedActivePools(s)
	out := make([]DepthPoint, 0, len(deltas))

	for _, d := range deltas {
		buyTarget := p0.Mul(one.Add(d))
		sellTarget := p0.Mul(one.Sub(d))

		var sdexBuy, sdexSell decimal.Decimal
		for _, l := range s.Book.Asks {
			if l.Price.Decimal().LessThanOrEqual(buyTarget) {
				sdexBuy = sdexBuy.Add(l.Notional())
			}
		}
		for _, l := range s.Book.Bids {
			if l.Price.Decimal().GreaterThanOrEqual(sellTarget) {
				sdexSell = sdexSell.Add(l.Notional())
			}
		}

		var ammBuy, ammSell decimal.Decimal
		for _, pool := range pools {
			b, err := ammBuyNotional(pool, buyTarget)
			if err != nil {
				return nil, fmt.Errorf("pool %s buy side at delta %s: %w", pool.PoolID, d, err)
			}
			sv, err := ammSellNotional(pool, sellTarget)
			if err != nil {
				return nil, fmt.Errorf("pool %s sell side at delta %s: %w", pool.PoolID, d, err)
			}
			ammBuy = ammBuy.Add(b)
			ammSell = ammSell.Add(sv)
		}

		out = append(out, DepthPoint{
			Delta:    d,
			BuySide:  sdexBuy.Add(ammBuy),
			SellSide: sdexSell.Add(ammSell),
			// FromSdex and FromAmm are the BUY side split, as types.go states.
			// They exist so a third party can check the combination without
			// reading this function.
			FromSdex: sdexBuy,
			FromAmm:  ammBuy,
		})
	}
	return out, nil
}

// ---------------------------------------------------------------- Manipulation

// ComputeManipulationCost computes the manipulation cost ladder.
// Passing includeAMM=false produces the OrderbookOnly variant.
// See docs/methodology/05-manipulation-cost.md.
//
// Cost and Reachable read DISJOINT sets of asks, and this is the line that an
// earlier version of the conformance test got wrong on two rows:
//
//	Cost      sums the asks with price <  target
//	Reachable asks whether an ask exists with price >= target
//
// The comparison is STRICT on the cost side. An attacker consumes every ask
// cheaper than the target and then barely touches the first one above it; that
// final touch sets the price the oracle reads and can cost arbitrarily little. An
// ask never belongs to both sets, which is why Cost = 0 with Reachable = true is
// a coherent state and the most dangerous one that exists.
//
// With an active pool and includeAMM, Reachable is unconditionally true. Under a
// constant product curve the price tends to infinity as the base reserve tends to
// zero, so no target is out of reach.
func ComputeManipulationCost(s Snapshot, p0 decimal.Decimal, deltas []decimal.Decimal, includeAMM bool) ([]ManipulationPoint, error) {
	if p0.LessThanOrEqual(decimal.Zero) {
		return nil, fmt.Errorf("manipulation cost needs a positive reference price, got %s", p0)
	}
	var pools []PoolReserves
	if includeAMM {
		pools = sortedActivePools(s)
	}
	out := make([]ManipulationPoint, 0, len(deltas))

	for _, d := range deltas {
		target := p0.Mul(one.Add(d))

		var cost decimal.Decimal
		reachable := false
		for _, l := range s.Book.Asks {
			price := l.Price.Decimal()
			switch {
			case price.LessThan(target):
				cost = cost.Add(l.Notional())
			default:
				// price >= target. This ask is what makes the target reachable
				// and it is deliberately NOT added to cost.
				reachable = true
			}
		}

		for _, pool := range pools {
			c, err := ammBuyNotional(pool, target)
			if err != nil {
				return nil, fmt.Errorf("pool %s at delta %s: %w", pool.PoolID, d, err)
			}
			cost = cost.Add(c)
		}
		if len(pools) > 0 {
			reachable = true
		}

		out = append(out, ManipulationPoint{
			Delta:       d,
			TargetPrice: target,
			Cost:        cost,
			Reachable:   reachable,
		})
	}
	return out, nil
}

// maxReachable applies 05-manipulation-cost.md section 5:
//
//	MaxReachablePrice       = the highest ask price in the book
//	CostToMaxReachablePrice = sum of the notional of asks with price < that
//
// Both are nil when an active pool is present, and the caller emits the warning
// that goes with them. A null with no stated reason is indistinguishable from a
// bug, which is why the warning is not optional.
func maxReachable(s Snapshot) (price, cost *decimal.Decimal) {
	if len(s.ActivePools()) > 0 || len(s.Book.Asks) == 0 {
		return nil, nil
	}
	highest := s.Book.Asks[0].Price.Decimal()
	for _, l := range s.Book.Asks[1:] {
		if p := l.Price.Decimal(); p.GreaterThan(highest) {
			highest = p
		}
	}
	var c decimal.Decimal
	for _, l := range s.Book.Asks {
		if l.Price.Decimal().LessThan(highest) {
			c = c.Add(l.Notional())
		}
	}
	return &highest, &c
}

// ---------------------------------------------------------------- Collateral

// findDepth and findManipulation locate one rung of a ladder by its delta. They
// return false rather than the nearest rung: a C_max computed from delta 0.05
// while claiming delta 0.10 is a wrong number wearing the right label.
func findDepth(ladder []DepthPoint, delta decimal.Decimal) (DepthPoint, bool) {
	for _, p := range ladder {
		if p.Delta.Cmp(delta) == 0 {
			return p, true
		}
	}
	return DepthPoint{}, false
}

func findManipulation(ladder []ManipulationPoint, delta decimal.Decimal) (ManipulationPoint, bool) {
	for _, p := range ladder {
		if p.Delta.Cmp(delta) == 0 {
			return p, true
		}
	}
	return ManipulationPoint{}, false
}

// ComputeMaxSafeCollateral applies docs/methodology/08-collateral.md.
//
//	if Reachable_orderbookOnly(critical):
//	    C_max = min( D_sell(liquidation) * h , MC_orderbookOnly(critical) * m )
//	else:
//	    C_max = D_sell(liquidation) * h,  with a warning
//
// The Reachable guard is mandatory: when the target is unreachable, Cost is not the
// cost of reaching anything, and multiplying it by m yields a meaningless number.
//
// mc MUST be the orderbookOnly ladder, not the combined one. An attacker takes the
// cheapest path and orderbookOnly <= combined always holds, so the smaller figure
// is the binding one. Passing the combined ladder here produces a C_max that is
// too generous, which is the one direction this product must never err in.
//
// NO HAND COMPUTED ORACLE EXISTS FOR THIS FUNCTION. 08-collateral.md is complete
// and this body follows it line for line, but testdata/fixtures carries no C_max
// row, so nothing outside this file has ever stated what it should return.
func ComputeMaxSafeCollateral(depth []DepthPoint, mc []ManipulationPoint, p Params) (cmax, liquidationLimit, manipulationLimit *decimal.Decimal, warnings []string) {
	d, ok := findDepth(depth, p.LiquidationDelta)
	if !ok {
		return nil, nil, nil, []string{fmt.Sprintf(
			"maxSafeCollateral was not computed: the depth ladder carries no rung at the liquidation delta %s", p.LiquidationDelta)}
	}
	liq := d.SellSide.Mul(p.LiquidationHaircut)
	liquidationLimit = &liq

	m, ok := findManipulation(mc, p.ManipulationCriticalDelta)
	if !ok {
		result := liq
		return &result, liquidationLimit, nil, []string{fmt.Sprintf(
			"the manipulation term was not applied: the orderbookOnly ladder carries no rung at the critical delta %s", p.ManipulationCriticalDelta)}
	}

	if !m.Reachable {
		result := liq
		return &result, liquidationLimit, nil, []string{fmt.Sprintf(
			"manipulation to delta %s is unreachable through the order book; the manipulation term was not applied", p.ManipulationCriticalDelta)}
	}

	man := m.Cost.Mul(p.ManipulationMargin)
	manipulationLimit = &man

	result := liq
	if man.LessThan(liq) {
		result = man
	}
	return &result, liquidationLimit, manipulationLimit, nil
}

// ---------------------------------------------------------------- Entry point

// ComputeAssetRisk is the only entry point into this package.
//
// Note the absence of context.Context. That is deliberate and serves as a signal: if a
// function in this package ever seems to need a ctx, some I/O has leaked in and belongs
// in an adapter instead.
//
// Every field of SupportingMetrics is left nil and OracleResistance is nil, because
// none of them can be derived from a Snapshot. Holder concentration needs trustline
// distribution, the volume ratios need a supply figure, and OracleResistance needs the
// genuine trade volume inside the oracle window. Their DEFINITIONS do not exist yet
// either: docs/methodology/07-supporting-metrics.md is still a worksheet, which is
// handoff item B-2. Nil means unknown, and the six flags that depend on them are
// reported as unevaluated rather than clear.
func ComputeAssetRisk(s Snapshot, p Params) (AssetRisk, error) {
	risk := AssetRisk{
		Base:               s.Base,
		Quote:              s.Quote,
		LedgerSeq:          s.LedgerSeq,
		LedgerClosedAt:     s.LedgerClosedAt,
		MethodologyVersion: MethodologyVersion,
		DataSource:         s.Source,
	}

	p0, src, poolSpot, divergence := MidPrice(s, p)
	risk.PriceSource = src
	risk.PoolSpotPrice = poolSpot
	risk.PriceDivergencePct = divergence

	if src == PriceSourceNone {
		// Not an error, and not a row missing from the report. Every ladder is
		// left empty because there is no P0 to measure a target against, and the
		// flags that read those ladders become unevaluated rather than zero.
		risk.Warnings = append(risk.Warnings,
			"no executable price: the book is empty on both sides and no active pool exists, so no depth, manipulation cost or collateral figure was computed")
		risk.Flags, risk.UnevaluatedFlags, risk.Band, risk.BandConfidence = evaluateFlags(flagInput{
			PriceSource: src,
			HasLadders:  false,
		}, p)
		return risk, nil
	}

	risk.MidPrice = &p0
	risk.SpreadPct = spreadPct(s.Book, p0)

	depth, err := ComputeDepth(s, p0, p.MarketDeltas)
	if err != nil {
		return AssetRisk{}, fmt.Errorf("depth: %w", err)
	}
	risk.Depth = depth

	combined, err := ComputeManipulationCost(s, p0, p.ManipulationDeltas, true)
	if err != nil {
		return AssetRisk{}, fmt.Errorf("manipulation cost, combined: %w", err)
	}
	orderbookOnly, err := ComputeManipulationCost(s, p0, p.ManipulationDeltas, false)
	if err != nil {
		return AssetRisk{}, fmt.Errorf("manipulation cost, orderbookOnly: %w", err)
	}
	risk.ManipulationCostCombined = combined
	risk.ManipulationCostOrderbookOnly = orderbookOnly

	risk.MaxReachablePrice, risk.CostToMaxReachablePrice = maxReachable(s)
	if len(s.ActivePools()) > 0 {
		risk.Warnings = append(risk.Warnings,
			"maxReachablePrice and costToMaxReachablePrice are null because an active pool is present: under a constant product curve the price tends to infinity as the base reserve tends to zero, so every target is reachable and a highest price has no meaning")
	}

	// C_max reads the orderbookOnly ladder. See the function's own comment for
	// why passing the combined one would be wrong in the dangerous direction.
	cmax, liq, man, warnings := ComputeMaxSafeCollateral(depth, orderbookOnly, p)
	risk.MaxSafeCollateral = cmax
	risk.MaxSafeCollateralLiquidation = liq
	risk.MaxSafeCollateralManipulation = man
	risk.Warnings = append(risk.Warnings, warnings...)

	risk.Flags, risk.UnevaluatedFlags, risk.Band, risk.BandConfidence = evaluateFlags(flagInput{
		PriceSource:        src,
		HasLadders:         true,
		SpreadPct:          risk.SpreadPct,
		Depth:              depth,
		OrderbookOnly:      orderbookOnly,
		HasActivePool:      len(s.ActivePools()) > 0,
		PriceDivergencePct: divergence,
	}, p)

	return risk, nil
}
