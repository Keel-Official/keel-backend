// Risk flags and the band derived from them.
//
// WHY THIS FILE EXISTS AT THIS PATH. docs/methodology/09-flags-and-bands.md line 5
// has carried "Implemented in: internal/domain/flags.go" since the road 1 split,
// and until 26 August 2026 no such file existed anywhere in the repository. The
// paid deliverable was pointing at empty space, which is the same failure
// CLAUDE.md records against internal/depth: a reference that resolves to nothing
// reports success. The pointer was made true rather than edited away, because the
// document that names the path is red and the code it names is not.
//
// The rules here are a TRANSCRIPTION of 09-flags-and-bands.md sections 4 and 5.
// Where this file and that document disagree, the document is right. It owns the
// three states, the tiers, and every threshold; this file owns none of them.
package domain

import "github.com/shopspring/decimal"

// The two deltas the flag rules name by value rather than by parameter.
//
// 09-flags-and-bands.md section 4 writes ZERO_DEPTH_2PCT against depth(0.02) and
// THIN_DEPTH_5PCT against depth(0.05) as literals, so they are methodology
// constants and not entries in Params. If a caller supplies MarketDeltas without
// these rungs, the flags that read them become UNEVALUATED rather than silently
// falling back to a neighbouring rung: a flag computed at 0.10 while claiming
// 0.02 is a wrong answer wearing the right label.
var (
	zeroDepthDelta = dec("0.02")
	thinDepthDelta = dec("0.05")
)

// flagTier is the severity a flag carries. The band is the highest tier among the
// triggered flags: no weighting, no averaging, no summation, per section 5.
type flagTier int

const (
	tierMedium flagTier = iota + 1
	tierHigh
	tierCritical
)

func (t flagTier) band() Band {
	switch t {
	case tierCritical:
		return BandCritical
	case tierHigh:
		return BandHigh
	case tierMedium:
		return BandMedium
	default:
		return BandLow
	}
}

// flagOrder is EVERY flag, in a fixed order, with its tier.
//
// The order is the reason this is a slice and not a map. NFR-9 requires two runs
// over one snapshot to produce byte for byte identical JSON, and a map would put
// the flag list in a different order on each run. Rule 2 in CLAUDE.md says sort
// before iterating; a declared order is the same guarantee bought earlier.
//
// It is also the enumeration of the set, so a thirteenth flag is added here and to
// the const block in types.go and nowhere else.
var flagOrder = []struct {
	Flag Flag
	Tier flagTier
}{
	{FlagNoExecutablePrice, tierCritical},
	{FlagZeroDepth2Pct, tierCritical},
	{FlagManipulationCheap, tierCritical},
	{FlagManipulationRatioLow, tierHigh},
	{FlagPriceSourceConflict, tierHigh},
	{FlagSpreadExtreme, tierHigh},
	{FlagNoGenuineTrade30D, tierHigh},
	{FlagHolderConcentrationExtreme, tierHigh},
	{FlagThinDepth5Pct, tierMedium},
	{FlagNoGenuineTrade7D, tierMedium},
	{FlagHolderConcentrationHigh, tierMedium},
	{FlagWashTradeSuspected, tierMedium},
}

// flagInput is everything the flag rules read. It is a struct rather than a long
// parameter list so that adding an input later cannot silently reorder the
// existing ones at a call site.
type flagInput struct {
	PriceSource PriceSource

	// HasLadders is false when priceSource is none, in which case no depth or
	// manipulation ladder was computed at all and every flag that reads one is
	// unevaluated rather than zero.
	HasLadders bool

	SpreadPct          *decimal.Decimal
	Depth              []DepthPoint
	OrderbookOnly      []ManipulationPoint
	HasActivePool      bool
	PriceDivergencePct *decimal.Decimal
}

// flagState is the three-state result from section 2. It was two states until
// version 1.0.2, and the correction is the whole point: unevaluated is NOT a
// synonym for clear. An asset with no trustline data must not look identical to
// one whose holder distribution was checked and found safe.
type flagState int

const (
	stateClear flagState = iota
	stateTriggered
	stateUnevaluated
)

// evaluateFlags returns the triggered flags, the unevaluated ones, the band, and
// the band confidence.
//
// Flags that are CLEAR appear in neither slice. That is what the contract says
// and what internal/conformance asserts: flags holds triggered, unevaluatedFlags
// holds what could not be checked, and everything absent from both was checked
// and found not to be met.
func evaluateFlags(in flagInput, p Params) (triggered, unevaluated []Flag, band Band, confidence BandConfidence) {
	t := p.Thresholds

	// The six flags that need supply, trade history or trustline distribution.
	// None of them can be judged from a Snapshot, so all six are unevaluated on
	// every result this package produces today. 09-flags-and-bands.md section 3
	// is the table that says which.
	states := map[Flag]flagState{
		FlagManipulationRatioLow:       stateUnevaluated,
		FlagNoGenuineTrade30D:          stateUnevaluated,
		FlagNoGenuineTrade7D:           stateUnevaluated,
		FlagHolderConcentrationExtreme: stateUnevaluated,
		FlagHolderConcentrationHigh:    stateUnevaluated,
		FlagWashTradeSuspected:         stateUnevaluated,
	}

	// NO_EXECUTABLE_PRICE: priceSource == none.
	states[FlagNoExecutablePrice] = boolState(in.PriceSource == PriceSourceNone)

	// PRICE_SOURCE_CONFLICT: an active pool exists AND the divergence passes the
	// threshold. With no pool there is nothing to disagree with, and that is
	// CLEAR rather than unevaluated: the condition was checked and is not met.
	switch {
	case !in.HasActivePool:
		states[FlagPriceSourceConflict] = stateClear
	case in.PriceDivergencePct == nil:
		// A pool with a one-sided book. There is no book mid to compare against,
		// so the condition genuinely could not be checked.
		states[FlagPriceSourceConflict] = stateUnevaluated
	default:
		states[FlagPriceSourceConflict] = boolState(in.PriceDivergencePct.GreaterThan(t.PriceDivergencePct))
	}

	// SPREAD_EXTREME: spreadPct > threshold. spreadPct is nil on a one-sided
	// book, where the difference is undefined.
	if in.SpreadPct == nil {
		states[FlagSpreadExtreme] = stateUnevaluated
	} else {
		states[FlagSpreadExtreme] = boolState(in.SpreadPct.GreaterThan(t.SpreadExtremePct))
	}

	// ZERO_DEPTH_2PCT: either side of depth(0.02) is zero. One side alone is
	// enough; an asset that cannot be sold is as dangerous as one that cannot be
	// bought.
	if d, ok := ladderRung(in, zeroDepthDelta); ok {
		states[FlagZeroDepth2Pct] = boolState(d.BuySide.IsZero() || d.SellSide.IsZero())
	} else {
		states[FlagZeroDepth2Pct] = stateUnevaluated
	}

	// THIN_DEPTH_5PCT: min of the two sides of depth(0.05) is below the absolute
	// threshold, expressed in the quote asset.
	if d, ok := ladderRung(in, thinDepthDelta); ok {
		thinner := d.BuySide
		if d.SellSide.LessThan(thinner) {
			thinner = d.SellSide
		}
		states[FlagThinDepth5Pct] = boolState(thinner.LessThan(t.ThinDepth5PctAbsolute))
	} else {
		states[FlagThinDepth5Pct] = stateUnevaluated
	}

	// MANIPULATION_CHEAP: there EXISTS a delta with Reachable == true AND
	// Cost < the absolute threshold, read off the ORDERBOOK ONLY ladder.
	//
	// The Reachable condition was added in 1.0.2 and must not be removed. On the
	// USTRY fixture Cost is 130.06 at delta 1, 10 and 100, and all three are
	// unreachable because no ask sits above 106.7372828. Counting them would
	// label an impossible attack as cheap, which inverts the truth. The row that
	// must be caught is the opposite one: Cost 0 at delta 0.5 WITH Reachable
	// true, which is the most dangerous state that can exist.
	if !in.HasLadders {
		states[FlagManipulationCheap] = stateUnevaluated
	} else {
		cheap := false
		for _, m := range in.OrderbookOnly {
			if m.Reachable && m.Cost.LessThan(t.ManipulationCheapAbsolute) {
				cheap = true
				break
			}
		}
		states[FlagManipulationCheap] = boolState(cheap)
	}

	// Emit in the declared order, never in map order.
	worst := flagTier(0)
	confidence = BandConfidenceFull
	for _, f := range flagOrder {
		switch states[f.Flag] {
		case stateTriggered:
			triggered = append(triggered, f.Flag)
			if f.Tier > worst {
				worst = f.Tier
			}
		case stateUnevaluated:
			unevaluated = append(unevaluated, f.Flag)
			// bandConfidence is partial when any CRITICAL or HIGH tier flag is
			// unevaluated. A LOW band with partial confidence is a far weaker
			// statement than LOW with full confidence, and the difference must
			// not be hidden.
			if f.Tier >= tierHigh {
				confidence = BandConfidencePartial
			}
		}
	}

	return triggered, unevaluated, worst.band(), confidence
}

// ladderRung returns the depth row at delta, and false when the ladder was never
// computed or carries no such rung.
func ladderRung(in flagInput, delta decimal.Decimal) (DepthPoint, bool) {
	if !in.HasLadders {
		return DepthPoint{}, false
	}
	return findDepth(in.Depth, delta)
}

func boolState(b bool) flagState {
	if b {
		return stateTriggered
	}
	return stateClear
}
