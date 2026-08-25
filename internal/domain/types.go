// Package domain holds Keel's shared types and its methodology computations.
//
// The two are in separate files on purpose. This file is the shapes: assets,
// prices, books, pools, snapshots, and the shape of a result. compute.go is the
// formulas, and compute.go is the RED ZONE, written by Al alone and locked in
// .claude/settings.json. A type is a shape and a formula is a claim, and only the
// second one has to be defended to a reviewer.
//
// THIS PACKAGE IS PURE. It must never contain:
//   - any I/O (net/http, database/sql, os, the Stellar SDK, BigQuery)
//   - time.Now(), math/rand, goroutines
//   - float64 or float32, not even as an intermediate value
//   - map iteration without sorting the keys first
//
// Enforced by arch_test.go, not by convention alone.
// Definitions of every quantity: docs/methodology/, indexed by 00-overview.md
// Flag and band definitions:     docs/methodology/09-flags-and-bands.md
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

// The four data sources. Two name WHERE the data came from and two name HOW it was
// reconstructed, and that they share one enum is a design smell recorded in handoff
// item 5b: the accurate shape is two fields, a source and a derivation. Adding the
// fourth value does not close that door.
//
// The confidence ordering, which is the part that matters when reading a result:
//
//	horizon         read directly from a live endpoint
//	hubble          read directly from the historical warehouse
//	offers-implied  reconstructed from manage_sell_offer and manage_buy_offer
//	                operations. An offer proves liquidity that was POSTED
//	trades-implied  reconstructed from trades that executed. A trade proves only
//	                liquidity that was CONSUMED, so this is a lower bound
//
// The distinction between the last two is not pedantry. It is the same
// posted-against-executed distinction that
// docs/methodology/06-oracle-resilience.md section 2 builds its entire argument on,
// and collapsing it here while leaning on it there would be incoherent.
const (
	DataSourceHorizon       DataSource = "horizon"
	DataSourceHubble        DataSource = "hubble"
	DataSourceOffersImplied DataSource = "offers-implied"
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
//	Cost large, Reachable=true   expensive; the market has a defense.
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
	// often on an unreachable target. A 50 percent price inflation is already more
	// than enough to push a position under water at any sane LTV.
	//
	// The rationale lives in docs/methodology/08-collateral.md section 1 and this is
	// a pointer to it rather than a second copy. The copy that used to be here
	// carried a claim the document itself refuted four lines above the place it was
	// copied from, that delta=1 would produce a collateral allowance derived from an
	// impossible attack, which the Reachable guard prevents by construction. Al
	// corrected the document on 25 August 2026 and this comment had to be corrected
	// separately, because a rationale restated in two files is a rationale that gets
	// fixed in one.
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

// OracleResistance answers one question: is moving the price to a critical level
// cheaper than the genuine trading volume that actually occurred inside the
// window an oracle averages over.
//
// It is an object rather than a single ratio on purpose. A ratio would hide two
// states that have to stay visible. A GenuineVolume of zero makes the ratio
// undefined, and an asset with no trading at all inside the oracle window is an
// important finding rather than missing data. And a ratio computed from a
// ManipulationCost whose Reachable is false is a meaningless number. In object
// form both states are readable and Ratio is simply nil. See DEC-003 section 2.5.
type OracleResistance struct {
	// CriticalDelta is the delta treated as the critical threshold for this
	// asset. It always equals one of the Delta values in ManipulationCost.
	CriticalDelta decimal.Decimal

	// ManipulationCost and Reachable are copied from the ManipulationCost entry
	// at CriticalDelta, so a reader never has to match them up by hand.
	ManipulationCost decimal.Decimal
	Reachable        bool

	// GenuineVolume is the genuine trade volume in the quote asset over the last
	// WindowSeconds, after the genuine trade filtering rules are applied. This is
	// the comparison baseline, and it is the defense an attacker has to outweigh.
	GenuineVolume decimal.Decimal

	// WindowSeconds is repeated here even though it is also a threshold, so that
	// a response can be read and archived without consulting anything else.
	WindowSeconds int

	// Ratio is ManipulationCost divided by GenuineVolume. Below 1 means moving
	// the price to the critical level is cheaper than all the genuine trading
	// across the window. Nil when GenuineVolume is zero or Reachable is false,
	// because the division is then meaningless. Nil means undefined, not zero.
	Ratio *decimal.Decimal
	// TotalAttackCost is MC_orderbookOnly(CriticalDelta) + GenuineVolume, the
	// quantity methodology 1.0.3 introduced. It answers a different question from
	// Ratio: not "is the attack cheap relative to the market it has to hide in"
	// but "how much capital does an attack on an averaging oracle need in total",
	// because the attacker has to pay the book AND outweigh the genuine volume
	// inside the window.
	//
	// Both are kept because they are not interchangeable. The ratio is
	// dimensionless and can rank fifty assets. The sum is in the quote asset and
	// can only be read against one pair, which is open question Q7 again.
	//
	// It is a LOWER BOUND and it carries an assumption. Needing exactly
	// GenuineVolume of extra volume to dominate a VWAP is an approximation; what
	// is really required depends on the weighting and on where in the window the
	// attack lands. See docs/methodology/06-oracle-resilience.md section 1.
	//
	// Nil when Reachable is false, for the same reason Ratio is: the cost of
	// reaching an unreachable target is not the cost of anything. Nil is not zero.
	TotalAttackCost *decimal.Decimal
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

	OracleResistance  *OracleResistance
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
