package conformance

import (
	"github.com/shopspring/decimal"

	"github.com/Keel-Official/keel/internal/domain"
)

// Expected values for GoldenSnapshot. Source: testdata/fixtures/ustry_pre_exploit.md.
//
// Every zero in this file carries a written reason, because a correct zero and a
// zero caused by a bug look exactly the same in the output.

// ---------------------------------------------------------------- Price

var (
	// ExpectedP0 is (1.057 + 106.7372828) / 2. Both sides of the book are
	// populated, so priceSource is book. The value is 53.90 for an asset
	// actually worth about 1.06. That is not a bug, it is what a mid price does
	// when the spread runs into the thousands of percent.
	ExpectedP0          = dec("53.8971414")
	ExpectedPriceSource = domain.PriceSourceBook

	// ExpectedSpreadPct is (106.7372828 - 1.057) / 53.8971414, expressed in
	// PERCENT. The exact value is 528401414/269485707, whose decimal expansion
	// never terminates, so any comparison has to use Tolerance.
	ExpectedSpreadPct = dec("196.0777140585048")
)

// ---------------------------------------------------------------- Depth

// ExpectedDepthPoint is one expected row of the depth ladder.
type ExpectedDepthPoint struct {
	Delta    decimal.Decimal
	BuySide  decimal.Decimal
	SellSide decimal.Decimal
	FromSdex decimal.Decimal
	FromAmm  decimal.Decimal
	Reason   string
}

// ExpectedDepth is entirely zero, and that zero is CORRECT.
//
// The only ask is priced at 106.7372828, far above every buy target
// (54.975084228 / 56.591998470 / 59.286855540). The only bid is priced at 1.057,
// far below every sell target (52.819198572 / 51.202284330 / 48.507427260). Not
// one level falls inside any price window.
//
// FromAmm is zero because Pools is empty, not because the curve was left
// untouched.
var ExpectedDepth = []ExpectedDepthPoint{
	{dec("0.02"), decimal.Zero, decimal.Zero, decimal.Zero, decimal.Zero,
		"buy target 54.975084228 and sell target 52.819198572, no level inside the window"},
	{dec("0.05"), decimal.Zero, decimal.Zero, decimal.Zero, decimal.Zero,
		"buy target 56.591998470 and sell target 51.202284330, no level inside the window"},
	{dec("0.10"), decimal.Zero, decimal.Zero, decimal.Zero, decimal.Zero,
		"buy target 59.286855540 and sell target 48.507427260, no level inside the window"},
}

// ---------------------------------------------------------------- Manipulation

// ExpectedManipulationPoint is one expected row of the manipulation cost ladder.
type ExpectedManipulationPoint struct {
	Delta       decimal.Decimal
	TargetPrice decimal.Decimal
	Cost        decimal.Decimal
	Reachable   bool
	Reason      string
}

// ExpectedManipulation is the most misread row in the whole fixture, and an
// earlier version of this test misread it on two lines.
//
// Cost and Reachable use DIFFERENT sets of asks:
//
//	Cost      sums the asks with price <  target
//	Reachable checks whether an ask exists with price >= target
//
// An ask never belongs to both at once. That is why the delta 0.5 row has Cost
// zero AND Reachable true at the same time, and that is the most dangerous
// condition that can exist: the price 80.85 can be reached without paying
// anything to a third party.
//
// Conversely delta 1, 10, and 100 have Cost 130.06 but Reachable false. The
// 130.06 there must NOT be read as "that price is expensive to reach", because
// that price cannot be reached at all; the book runs out before it.
var ExpectedManipulation = []ExpectedManipulationPoint{
	{dec("0.5"), dec("80.8457121"), decimal.Zero, true,
		"no ask is cheaper than 80.85 so Cost is zero, but the ask at 106.74 satisfies >= target so it is reachable"},
	{dec("1"), dec("107.7942828"), dec("130.06270929502336"), false,
		"the ask at 106.74 is cheaper than the target so it counts toward Cost, and no ask is >= 107.79"},
	{dec("10"), dec("592.8685554"), dec("130.06270929502336"), false,
		"same again, the book runs out far below the target"},
	{dec("100"), dec("5443.6112814"), dec("130.06270929502336"), false,
		"same again, the book runs out far below the target"},
}

// ---------------------------------------------------------------- Reach

var (
	// ExpectedMaxReachablePrice is the highest ask price on the book, which is
	// the highest price an attacker can reach. It is 100.98 times the asset's
	// actual value of 1.057.
	ExpectedMaxReachablePrice = dec("106.7372828")

	// ExpectedCostToMaxReachablePrice is zero because not one ask is cheaper
	// than 106.7372828. Reaching the highest price on this book is FREE.
	//
	// This pair of numbers is the most important line in the entire fixture. The
	// real attack fell in the gap between delta 0.5 and delta 1, so the discrete
	// delta ladder missed it and only these two numbers caught it.
	ExpectedCostToMaxReachablePrice = decimal.Zero
)

// ---------------------------------------------------------------- Flags

// ExpectedFlags are the flags that must be TRIGGERED, all of them decidable
// from the snapshot alone.
var ExpectedFlags = []domain.Flag{
	domain.FlagZeroDepth2Pct,     // CRITICAL: depth at +/-2% is zero on both sides
	domain.FlagManipulationCheap, // CRITICAL: Cost(0.5) = 0 with Reachable true
	domain.FlagSpreadExtreme,     // HIGH:     196.08% is past the 20% threshold
	domain.FlagThinDepth5Pct,     // MEDIUM:   depth at 5% is zero, below any absolute threshold
}

// ExpectedUnevaluatedFlags need supply data, trade history, or trustline
// distribution, none of which is present in a Snapshot.
//
// All six must be reported as not evaluable, NOT as zero and NOT as clear. Zero
// means measured to be zero, and that is a different claim.
var ExpectedUnevaluatedFlags = []domain.Flag{
	domain.FlagManipulationRatioLow,
	domain.FlagNoGenuineTrade30D,
	domain.FlagNoGenuineTrade7D,
	domain.FlagWashTradeSuspected,
	domain.FlagHolderConcentrationExtreme,
	domain.FlagHolderConcentrationHigh,
}

// ExpectedClearFlags were checked and are not met.
var ExpectedClearFlags = []domain.Flag{
	domain.FlagNoExecutablePrice, // priceSource is book, so an executable price exists
}

var (
	// ExpectedBand is the highest level among the triggered flags. There is no
	// weighting.
	ExpectedBand = domain.BandCritical

	// ExpectedBandConfidence is partial because MANIPULATION_RATIO_LOW and
	// HOLDER_CONCENTRATION_EXTREME sit at the HIGH level but are unevaluated.
	// The band stays CRITICAL because two CRITICAL flags are already triggered,
	// so the incomplete data does not change the conclusion IN THIS CASE. That
	// is a coincidence, not a guarantee.
	ExpectedBandConfidence = domain.BandConfidencePartial
)
