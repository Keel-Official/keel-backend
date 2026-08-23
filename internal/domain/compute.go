// The methodology computations. THIS FILE IS THE RED ZONE.
//
// Al writes this file. Claude does not, and that is enforced rather than agreed:
// Edit and Write are denied on this path in .claude/settings.json, and the Bash
// path is closed by .claude/hooks/lindungi-zona-merah.sh.
//
// WHY A FILE AND NOT A DIRECTORY. The red zone used to be the internal/depth
// directory. When methodology 1.0.3 moved the computations into this package, the
// lock was left pointing at a directory that no longer held anything, and the zone
// silently stopped existing. The zone follows the code, not the directory name, so
// it is this file that is locked now. The types next door in types.go stay open,
// because a type is a shape and a formula is a claim, and only the second one has
// to be defended to a reviewer or a funder.
//
// WHAT CLAUDE MAY DO HERE. Read it, run its tests, point out an edge case that is
// not handled, and ask about it. /teach for a concept, /review-mine once this is
// written. See internal/domain/CLAUDE.md.
//
// Definitions live in docs/methodology/, one file per subject, indexed by
// 00-overview.md. Where this file and those documents disagree, the documents are
// right and this file has a bug.
package domain

import "github.com/shopspring/decimal"

// ComputeAssetRisk is the only entry point into this package.
//
// Note the absence of context.Context. That is deliberate and serves as a signal: if a
// function in this package ever seems to need a ctx, some I/O has leaked in and belongs
// in an adapter instead.
//
// The wiring the body owes its caller, recorded here because the fragment that stated
// it did not compile and the compiler cannot hold a note:
//
//	cmax, liquidationLimit, manipulationLimit, warnings := ComputeMaxSafeCollateral(...)
//	risk.MaxSafeCollateral             = cmax
//	risk.MaxSafeCollateralLiquidation  = liquidationLimit
//	risk.MaxSafeCollateralManipulation = manipulationLimit
func ComputeAssetRisk(s Snapshot, p Params) (AssetRisk, error) {
	panic("not implemented")
}

// MidPrice applies the fallback order in docs/methodology/03-reference-price.md section 1.
// It also returns the pool spot price and their divergence when a pool is available.
func MidPrice(s Snapshot, p Params) (p0 decimal.Decimal, src PriceSource, poolSpot, divergence *decimal.Decimal) {
	panic("not implemented")
}

// ComputeDepth computes the market quality ladder, merging SDEX and AMM at the same
// final marginal price. See docs/methodology/04-depth.md section 3.
func ComputeDepth(s Snapshot, p0 decimal.Decimal, deltas []decimal.Decimal) ([]DepthPoint, error) {
	panic("not implemented")
}

// ComputeManipulationCost computes the manipulation cost ladder.
// Passing includeAMM=false produces the OrderbookOnly variant.
// See docs/methodology/05-manipulation-cost.md.
func ComputeManipulationCost(s Snapshot, p0 decimal.Decimal, deltas []decimal.Decimal, includeAMM bool) ([]ManipulationPoint, error) {
	panic("not implemented")
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
func ComputeMaxSafeCollateral(depth []DepthPoint, mc []ManipulationPoint, p Params) (cmax, liquidationLimit, manipulationLimit *decimal.Decimal, warnings []string) {
	panic("not implemented")
}
