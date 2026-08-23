// Package domain contains all of Keel's liquidity risk computations.
//
// THIS PACKAGE IS PURE. It must never contain:
//   - any I/O (net/http, database/sql, os, the Stellar SDK, BigQuery)
//   - time.Now(), math/rand, goroutines
//   - float64 or float32, not even as an intermediate value
//   - map iteration without sorting the keys first
//
// Enforced by arch_test.go, not by convention alone.
// Definitions of every quantity: docs/methodology/keel-methodology-core.md
// Flag and band definitions:     docs/methodology/09-flag-dan-band.md
package domain

import (
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

// MethodologyVersion must be bumped whenever a definition or threshold changes.
// Results produced under different versions are not directly comparable.
const MethodologyVersion = "1.0.3-draft"

// ---------------------------------------------------------------- Assets

type AssetType string

const (
	AssetTypeNative     AssetType = "native"
	AssetTypeAlphanum4  AssetType = "credit_alphanum4"
	AssetTypeAlphanum12 AssetType = "credit_alphanum12"
)

// Asset identifies a single Stellar asset.
//
// Type must be explicit and must never be inferred from len(Code) at runtime.
// Querying Horizon with the wrong type returns an empty result with no error.
// USTRY has a 5-character code and is therefore alphanum12.
type Asset struct {
	Code   string
	Issuer string // empty for the native asset
	Type   AssetType
}

func (a Asset) IsNative() bool { return a.Type == AssetTypeNative }

func (a Asset) String() string {
	if a.IsNative() {
		return "XLM"
	}
	return a.Code + ":" + a.Issuer
}

func (a Asset) Equal(o Asset) bool {
	return a.Code == o.Code && a.Issuer == o.Issuer && a.Type == o.Type
}

// ---------------------------------------------------------------- Prices

// Price is an exact rational, always expressed as quote per base.
//
// Horizon returns prices in two inconsistent shapes:
//
//	/offers  -> "price_r": {"n": 266843207, "d": 2500000}     JSON numbers
//	/trades  -> "price":   {"n": "2500000", "d": "266843207"} JSON strings
//
// and the direction of price on /trades depends on which asset is the base.
// Adapters must normalise both into quote-per-base before anything reaches this
// package. Never use Horizon's "price" string for computation; it is rounded.
type Price struct {
	N int64
	D int64
}

func (p Price) Valid() bool { return p.D != 0 && p.N > 0 }

func (p Price) Decimal() decimal.Decimal {
	return decimal.NewFromInt(p.N).Div(decimal.NewFromInt(p.D))
}

func (p Price) Invert() Price { return Price{N: p.D, D: p.N} }

// Cmp compares without dividing, so no precision is lost.
func (p Price) Cmp(o Price) int {
	left := decimal.NewFromInt(p.N).Mul(decimal.NewFromInt(o.D))
	right := decimal.NewFromInt(o.N).Mul(decimal.NewFromInt(p.D))
	return left.Cmp(right)
}

func (p Price) String() string { return fmt.Sprintf("%d/%d", p.N, p.D) }

// ---------------------------------------------------------------- Order book

type Level struct {
	Price  Price
	Amount decimal.Decimal // in base asset units
}

// Notional returns the value of this level in the quote asset.
func (l Level) Notional() decimal.Decimal { return l.Price.Decimal().Mul(l.Amount) }

// OrderBook: Bids sorted by descending price, Asks by ascending price.
// The adapter guarantees the ordering.
type OrderBook struct {
	Bids []Level
	Asks []Level
}

func (b OrderBook) BestBid() (Level, bool) {
	if len(b.Bids) == 0 {
		return Level{}, false
	}
	return b.Bids[0], true
}

func (b OrderBook) BestAsk() (Level, bool) {
	if len(b.Asks) == 0 {
		return Level{}, false
	}
	return b.Asks[0], true
}

// ---------------------------------------------------------------- Pools

// PoolReserves is a single constant product pool.
//
// FeeBP is read from the Horizon response. Do NOT hardcode 30. Stellar permits
// other values and a hardcoded fee will be silently wrong on a different pool.
type PoolReserves struct {
	PoolID       string
	ReserveBase  decimal.Decimal
	ReserveQuote decimal.Decimal
	FeeBP        int32
}

func (p PoolReserves) SpotPrice() decimal.Decimal {
	if p.ReserveBase.IsZero() {
		return decimal.Zero
	}
	return p.ReserveQuote.Div(p.ReserveBase)
}

func (p PoolReserves) IsEmpty() bool {
	return p.ReserveBase.IsZero() || p.ReserveQuote.IsZero()
}

// ---------------------------------------------------------------- Snapshot

type PriceSource string

const (
	PriceSourceBook PriceSource = "book"
	PriceSourcePool PriceSource = "pool"
	PriceSourceNone PriceSource = "none"
)

type DataSource string

const (
	DataSourceHorizon       DataSource = "horizon"
	DataSourceHubble        DataSource = "hubble"
	DataSourceTradesImplied DataSource = "trades-implied"
)

// Snapshot is the ONLY input to the depth computation.
//
// Its shape is identical whether it came from Horizon (live) or Hubble (historical),
// so swapping the data source touches no line in this package.
type Snapshot struct {
	Base           Asset
	Quote          Asset
	LedgerSeq      uint32
	LedgerClosedAt time.Time
	Book           OrderBook
	Pools          []PoolReserves
	Source         DataSource
}

// ActivePools returns the non-empty pools.
// The presence of an active pool changes the semantics of MaxReachablePrice;
// see AssetRisk.
func (s Snapshot) ActivePools() []PoolReserves {
	out := make([]PoolReserves, 0, len(s.Pools))
	for _, p := range s.Pools {
		if !p.IsEmpty() {
			out = append(out, p)
		}
	}
	return out
}

// ---------------------------------------------------------------- Results

// DepthPoint is depth at one delta level, as notional in the quote asset.
// FromSdex and FromAmm are reported separately so third parties can verify the
// combination without reading the code. Both refer to the buy side.
type DepthPoint struct {
	Delta    decimal.Decimal
	BuySide  decimal.Decimal
	SellSide decimal.Decimal
	FromSdex decimal.Decimal
	FromAmm  decimal.Decimal
}

// ManipulationPoint is the cost of raising the price to one target.
//
// Rules:
//
//	Cost(P_target)      = Σ notional of asks with price <  P_target
//	Reachable(P_target) = there exists an ask with price >= P_target
//
// An attacker consumes every ask CHEAPER than the target, then barely touches the
// first ask ABOVE it. That final touch sets the price the oracle reads, and it can
// cost arbitrarily little.
//
// Interpretation:
//
//	Cost small, Reachable=true   cheap and achievable. MOST DANGEROUS.
//	Cost large, Reachable=true   expensive; the market has a defence.
//	Reachable=false              the target cannot be reached at any capital.
//	                             This is not bad news.
//
// When an active pool is present, Reachable on the Combined variant is always true,
// because under a constant product curve the price tends to infinity as the base
// reserve tends to zero.
//
// Cost is an UPPER BOUND. Keel cannot know which orders belong to the attacker
// ahead of time, so it does not filter them. This bias points in the safe direction.
type ManipulationPoint struct {
	Delta       decimal.Decimal
	TargetPrice decimal.Decimal
	Cost        decimal.Decimal
	Reachable   bool
}

// ---------------------------------------------------------------- Flags

type Flag string

const (
	FlagNoExecutablePrice          Flag = "NO_EXECUTABLE_PRICE"
	FlagZeroDepth2Pct              Flag = "ZERO_DEPTH_2PCT"
	FlagManipulationCheap          Flag = "MANIPULATION_CHEAP"
	FlagManipulationRatioLow       Flag = "MANIPULATION_RATIO_LOW"
	FlagPriceSourceConflict        Flag = "PRICE_SOURCE_CONFLICT"
	FlagSpreadExtreme              Flag = "SPREAD_EXTREME"
	FlagNoGenuineTrade30D          Flag = "NO_GENUINE_TRADE_30D"
	FlagNoGenuineTrade7D           Flag = "NO_GENUINE_TRADE_7D"
	FlagHolderConcentrationExtreme Flag = "HOLDER_CONCENTRATION_EXTREME"
	FlagHolderConcentrationHigh    Flag = "HOLDER_CONCENTRATION_HIGH"
	FlagThinDepth5Pct              Flag = "THIN_DEPTH_5PCT"
	FlagWashTradeSuspected         Flag = "WASH_TRADE_SUSPECTED"
)

type Band string

const (
	BandLow      Band = "LOW"
	BandMedium   Band = "MEDIUM"
	BandHigh     Band = "HIGH"
	BandCritical Band = "CRITICAL"
)

type BandConfidence string

const (
	BandConfidenceFull    BandConfidence = "full"
	BandConfidencePartial BandConfidence = "partial"
)

// ---------------------------------------------------------------- Parameters

// Thresholds holds EVERY threshold in one place.
// These values are CHOSEN, not calibrated against a body of incidents.
// Changing any of them requires bumping MethodologyVersion.
type Thresholds struct {
	ManipulationCheapAbsolute decimal.Decimal // in the quote asset
	ManipulationRatioLowPct   decimal.Decimal
	ThinDepth5PctAbsolute     decimal.Decimal
	SpreadExtremePct          decimal.Decimal
	PriceDivergencePct        decimal.Decimal // threshold for switching P0 to the pool
	HolderTop1ExtremePct      decimal.Decimal
	HolderTop10HighPct        decimal.Decimal
	WashTradeSuspectedPct     decimal.Decimal
	GenuineTradeStaleDays     int
	GenuineTradeWarnDays      int
}

// Params carries all configuration input.
// No function in this package holds a hidden default; everything arrives here.
type Params struct {
	// Market quality ladder, required by the SOW: 0.02, 0.05, 0.10
	MarketDeltas []decimal.Decimal

	// Manipulation resilience ladder: 0.5, 1, 10, 100
	// Required because the attacker moved the price by a factor of 100, not by
	// 10 percent.
	ManipulationDeltas []decimal.Decimal

	LiquidationDelta   decimal.Decimal // 0.10
	LiquidationHaircut decimal.Decimal // 0.5

	// ManipulationCriticalDelta is fixed at 0.5 rather than 1.0.
	//
	// Cost is monotonically increasing in delta and Reachable is monotonically
	// decreasing, so a lower value always yields a tighter bound while relying less
	// often on an unreachable target. On the USTRY fixture, delta=1 gives
	// Reachable=false with Cost=130.0627093, so the manipulation term would produce a
	// positive collateral allowance derived from an IMPOSSIBLE attack. At delta=0.5
	// the result is zero, which is correct.
	ManipulationCriticalDelta decimal.Decimal // 0.5
	ManipulationMargin        decimal.Decimal // 0.25

	// Oracle VWAP window. The 15-minute default is an ASSUMPTION and has not been
	// confirmed as Reflector's actual window.
	OracleWindow time.Duration

	Thresholds Thresholds
}

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

// DefaultParams returns the default parameters for methodology 1.0.3.
// Every value here is CHOSEN, not calibrated.
func DefaultParams() Params {
	return Params{
		MarketDeltas:              []decimal.Decimal{dec("0.02"), dec("0.05"), dec("0.10")},
		ManipulationDeltas:        []decimal.Decimal{dec("0.5"), dec("1"), dec("10"), dec("100")},
		LiquidationDelta:          dec("0.10"),
		LiquidationHaircut:        dec("0.5"),
		ManipulationCriticalDelta: dec("0.5"),
		ManipulationMargin:        dec("0.25"),
		OracleWindow:              15 * time.Minute,
		Thresholds: Thresholds{
			ManipulationCheapAbsolute: dec("10000"),
			ManipulationRatioLowPct:   dec("1.0"),
			ThinDepth5PctAbsolute:     dec("50000"),
			SpreadExtremePct:          dec("20.0"),
			PriceDivergencePct:        dec("10.0"),
			HolderTop1ExtremePct:      dec("50.0"),
			HolderTop10HighPct:        dec("80.0"),
			WashTradeSuspectedPct:     dec("50.0"),
			GenuineTradeStaleDays:     30,
			GenuineTradeWarnDays:      7,
		},
	}
}

// ---------------------------------------------------------------- Output

// SupportingMetrics fields are nil when they cannot be computed.
// Nil means "unknown", NOT zero.
type SupportingMetrics struct {
	HolderTop1Pct         *decimal.Decimal
	HolderTop10Pct        *decimal.Decimal
	HolderHHI             *decimal.Decimal
	VolumeToSupplyD1      *decimal.Decimal
	VolumeToSupplyD7      *decimal.Decimal
	VolumeToSupplyD30     *decimal.Decimal
	LastGenuineTrade      *TradeRef
	TradesExcludedPct     *decimal.Decimal
	GenuineVolumeInWindow *decimal.Decimal
}

type TradeRef struct {
	LedgerSeq uint32
	At        time.Time
}

// AssetRisk is the complete output for one asset at one ledger.
type AssetRisk struct {
	Base               Asset
	Quote              Asset
	LedgerSeq          uint32
	LedgerClosedAt     time.Time
	MethodologyVersion string
	DataSource         DataSource

	MidPrice    *decimal.Decimal // nil when PriceSource is none
	PriceSource PriceSource
	SpreadPct   *decimal.Decimal

	// PoolSpotPrice and PriceDivergencePct are always populated when an active pool
	// exists, regardless of which branch the P0 rule took. A large divergence raises
	// PRICE_SOURCE_CONFLICT and causes P0 to be taken from the pool.
	PoolSpotPrice      *decimal.Decimal
	PriceDivergencePct *decimal.Decimal

	Depth []DepthPoint

	// Two forms of manipulation cost, answering two different questions.
	//
	//	Combined      cost of moving the actual market price (SDEX + AMM)
	//	OrderbookOnly cost of fooling an oracle that reads SDEX trades
	//
	// OrderbookOnly <= Combined always holds. An attacker takes the cheapest path, so
	// OrderbookOnly is the binding figure and the one used in C_max.
	// The gap between them is itself a signal: a large Combined with a small
	// OrderbookOnly means the asset looks safe while it is not.
	ManipulationCostCombined      []ManipulationPoint
	ManipulationCostOrderbookOnly []ManipulationPoint

	// MaxReachablePrice is the highest ask price in the book, and
	// CostToMaxReachablePrice is the cost of getting there.
	//
	// BOTH are nil when an active pool is present, accompanied by a warning: under a
	// constant product curve every target is reachable, so "highest" loses meaning.
	//
	// This pair captures attacks that fall between two rungs of the delta ladder. On
	// the USTRY fixture the values are 106.7372828 at zero cost, while the actual
	// attack landed between delta 0.5 and 1.
	MaxReachablePrice       *decimal.Decimal
	CostToMaxReachablePrice *decimal.Decimal

	OracleResistance  *decimal.Decimal // MC_orderbookOnly(critical) + genuine volume in window
	MaxSafeCollateral *decimal.Decimal

	// The two terms behind MaxSafeCollateral, reported separately as required by
	// methodology section 9 ("Both terms must be reported separately, not only their
	// minimum").
	//
	//	MaxSafeCollateralLiquidation  = D_sell(δ_liquidation) × h      always present
	//	MaxSafeCollateralManipulation = MC_orderbookOnly(δ_critical) × m
	//
	// MaxSafeCollateralManipulation is nil when the critical target is UNREACHABLE
	// through the order book. Per section 9 the manipulation term is then not applied,
	// C_max falls back to the liquidation term alone, and a warning is emitted. Nil
	// here means "not applicable", consistent with the nil convention elsewhere.
	MaxSafeCollateralLiquidation  *decimal.Decimal
	MaxSafeCollateralManipulation *decimal.Decimal

	Supporting SupportingMetrics

	Flags []Flag

	// UnevaluatedFlags holds flags that could not be checked because the required
	// data was absent. Unevaluated is NOT a synonym for clear. An asset with no
	// trustline data must not look identical to one that was checked and found safe.
	UnevaluatedFlags []Flag

	Band Band

	// BandConfidence is partial when any CRITICAL or HIGH tier flag is unevaluated.
	// It must be surfaced on the dashboard: a LOW band with partial confidence is a
	// far weaker statement than LOW with full confidence.
	BandConfidence BandConfidence

	Warnings []string
}

// ---------------------------------------------------------------- Contract

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

// MidPrice applies the fallback order in docs/methodology/keel-methodology-core.md section 3.
// It also returns the pool spot price and their divergence when a pool is available.
func MidPrice(s Snapshot, p Params) (p0 decimal.Decimal, src PriceSource, poolSpot, divergence *decimal.Decimal) {
	panic("not implemented")
}

// ComputeDepth computes the market quality ladder, merging SDEX and AMM at the same
// final marginal price. See methodology section 6.
func ComputeDepth(s Snapshot, p0 decimal.Decimal, deltas []decimal.Decimal) ([]DepthPoint, error) {
	panic("not implemented")
}

// ComputeManipulationCost computes the manipulation cost ladder.
// Passing includeAMM=false produces the OrderbookOnly variant.
func ComputeManipulationCost(s Snapshot, p0 decimal.Decimal, deltas []decimal.Decimal, includeAMM bool) ([]ManipulationPoint, error) {
	panic("not implemented")
}

// ComputeMaxSafeCollateral applies methodology section 9.
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
