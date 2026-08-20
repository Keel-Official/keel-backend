// Package domain holds Keel's shared types: assets, prices, books, pools,
// snapshots, and the shape of a liquidity risk result.
//
// This package contains NO computation. Every methodology formula lives in
// internal/depth, which imports this package. The split is deliberate: domain is
// open to anyone, depth is a paid deliverable whose author has to be able to
// defend it.
//
// THIS PACKAGE IS PURE. None of the following is allowed:
//   - any I/O (net/http, database/sql, os, the Stellar SDK, BigQuery)
//   - time.Now(), math/rand, goroutines
//   - float64 or float32, including as an intermediate value
//   - iterating a map without sorting its keys first
//
// These rules are enforced by arch_test.go. They are not merely a convention.
// See docs/methodology/keel-methodology-core.md for the definition of every
// quantity declared here.
package domain

import (
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

// MethodologyVersion must be raised whenever a definition or a threshold
// changes. Results produced by different versions cannot be compared directly.
const MethodologyVersion = "1.0.2-draft"

// ---------------------------------------------------------------- Asset

// AssetType is the asset type as the Stellar XDR defines it. It is carried
// explicitly rather than derived, because Horizon reports whatever the issuer
// chose, and a five character code such as USTRY is alphanum12.
type AssetType string

// The three asset types Stellar has. A query using the wrong one returns an
// empty result and no error, so these values are never guessed.
const (
	AssetTypeNative     AssetType = "native"
	AssetTypeAlphanum4  AssetType = "credit_alphanum4"
	AssetTypeAlphanum12 AssetType = "credit_alphanum12"
)

// Asset identifies one Stellar asset.
//
// Type must be explicit and must never be inferred from the length of Code at
// runtime. Querying Horizon with the wrong type returns an empty result and no
// error, which is the most dangerous silent failure in this integration. USTRY
// has a five character code and is therefore an alphanum12 asset.
type Asset struct {
	Code   string
	Issuer string // empty for the native asset
	Type   AssetType
}

// IsNative reports whether this is XLM, the only asset with no issuer.
func (a Asset) IsNative() bool { return a.Type == AssetTypeNative }

func (a Asset) String() string {
	if a.IsNative() {
		return "XLM"
	}
	return a.Code + ":" + a.Issuer
}

// Equal compares all three fields. Code alone is not an identity: two different
// issuers can each mint an asset called USDC.
func (a Asset) Equal(o Asset) bool {
	return a.Code == o.Code && a.Issuer == o.Issuer && a.Type == o.Type
}

// ---------------------------------------------------------------- Price

// Price is an exact rational, always expressed as quote per base.
//
// Horizon sends prices in two inconsistent shapes:
//
//	/offers  -> "price_r": {"n": 266843207, "d": 2500000}   JSON numbers
//	/trades  -> "price":   {"n": "2500000", "d": "266843207"} JSON strings
//
// and on /trades the direction of price depends on which asset is the base. The
// adapter is responsible for normalizing both into quote per base before
// anything reaches this package. Never use Horizon's string "price" field for a
// computation; it is already rounded.
type Price struct {
	N int64
	D int64
}

// Valid reports whether this fraction is usable. A zero denominator is undefined
// and a non-positive numerator is not a price.
func (p Price) Valid() bool { return p.D != 0 && p.N > 0 }

// Decimal divides the fraction out. Use it for reporting, not for comparison:
// Cmp compares without dividing and loses nothing.
func (p Price) Decimal() decimal.Decimal {
	return decimal.NewFromInt(p.N).Div(decimal.NewFromInt(p.D))
}

// Invert flips the direction of the price exactly, by swapping numerator and
// denominator. Inverting twice returns the original bit for bit, which a
// reciprocal computed by division would not guarantee.
func (p Price) Invert() Price { return Price{N: p.D, D: p.N} }

// Cmp compares without dividing, so no precision is lost.
// It returns -1, 0, or 1.
func (p Price) Cmp(o Price) int {
	left := decimal.NewFromInt(p.N).Mul(decimal.NewFromInt(o.D))
	right := decimal.NewFromInt(o.N).Mul(decimal.NewFromInt(p.D))
	return left.Cmp(right)
}

func (p Price) String() string { return fmt.Sprintf("%d/%d", p.N, p.D) }

// ---------------------------------------------------------------- Book

// Level is one price level on the orderbook.
// Amount is expressed in units of the base asset.
type Level struct {
	Price  Price
	Amount decimal.Decimal
}

// Notional returns the value of this level in the quote asset.
func (l Level) Notional() decimal.Decimal { return l.Price.Decimal().Mul(l.Amount) }

// OrderBook holds the buy and sell sides.
// Bids are ordered by descending price, Asks by ascending price.
// Whichever adapter fills this struct is responsible for that ordering.
type OrderBook struct {
	Bids []Level
	Asks []Level
}

// BestBid returns the highest bid. The boolean is false when the buy side is
// empty, which is an ordinary state on a thin asset rather than an error.
func (b OrderBook) BestBid() (Level, bool) {
	if len(b.Bids) == 0 {
		return Level{}, false
	}
	return b.Bids[0], true
}

// BestAsk returns the lowest ask. The boolean is false when the sell side is
// empty, which is an ordinary state on a thin asset rather than an error.
func (b OrderBook) BestAsk() (Level, bool) {
	if len(b.Asks) == 0 {
		return Level{}, false
	}
	return b.Asks[0], true
}

// ---------------------------------------------------------------- Pool

// PoolReserves is one constant product pool.
// FeeBP is in basis points, 30 on Stellar.
type PoolReserves struct {
	PoolID       string
	ReserveBase  decimal.Decimal
	ReserveQuote decimal.Decimal
	FeeBP        int32
}

// SpotPrice is the pool's marginal price, Y over X. It returns zero when the base
// reserve is zero, and a caller must check IsEmpty rather than reading that zero
// as a price.
func (p PoolReserves) SpotPrice() decimal.Decimal {
	if p.ReserveBase.IsZero() {
		return decimal.Zero
	}
	return p.ReserveQuote.Div(p.ReserveBase)
}

// IsEmpty reports whether either reserve is zero. Such a pool quotes no usable
// price and contributes no depth.
func (p PoolReserves) IsEmpty() bool {
	return p.ReserveBase.IsZero() || p.ReserveQuote.IsZero()
}

// ---------------------------------------------------------------- Snapshot

// PriceSource records where the reference price came from. It is reported to the
// consumer because a price taken from a pool and a price taken from a two sided
// book carry very different confidence.
type PriceSource string

// The three price sources. none is a legitimate result rather than an error: an
// asset with no executable price is the highest-value finding Keel can produce.
const (
	PriceSourceBook PriceSource = "book"
	PriceSourcePool PriceSource = "pool"
	PriceSourceNone PriceSource = "none"
)

// DataSource records how the underlying data was obtained. It is part of the
// output because a number reconstructed from trades is a lower bound and must
// never be displayed as equivalent to a measurement.
type DataSource string

// The three data sources. trades-implied always carries a warning, because a
// trade proves the liquidity that was used, not the liquidity that was available.
const (
	DataSourceHorizon       DataSource = "horizon"
	DataSourceHubble        DataSource = "hubble"
	DataSourceTradesImplied DataSource = "trades-implied"
)

// Snapshot is the ONLY input to a depth computation.
// Its shape is identical whether it came from Horizon (live) or Hubble
// (historical), so swapping the data source does not touch a single line in
// this package.
type Snapshot struct {
	Base           Asset
	Quote          Asset
	LedgerSeq      uint32
	LedgerClosedAt time.Time
	Book           OrderBook
	Pools          []PoolReserves
	Source         DataSource
}

// ---------------------------------------------------------------- Results

// DepthPoint is the depth at one delta level.
// Every value is a notional in the quote asset.
//
// FromSdex and FromAmm are reported separately so that a third party can verify
// the combination without reading the code. Both refer to the buy side.
type DepthPoint struct {
	Delta    decimal.Decimal
	BuySide  decimal.Decimal
	SellSide decimal.Decimal
	FromSdex decimal.Decimal
	FromAmm  decimal.Decimal
}

// ManipulationPoint is the cost of pushing the price up to one target.
//
// The rules:
//
//	Cost(P_target)      = sum of ask notionals with price <  P_target
//	Reachable(P_target) = an ask exists with price >= P_target
//
// The attacker has to consume every ask CHEAPER than the target, then touch
// only a sliver of the first ask ABOVE it. That final touch is what sets the
// price the oracle reads, and it can cost arbitrarily little.
//
// Reading the result:
//
//	small Cost, Reachable=true   cheap and achievable. MOST DANGEROUS.
//	large Cost, Reachable=true   expensive; the market has a defense.
//	Reachable=false              the target cannot be reached at any price,
//	                             because the book runs out before it. That is
//	                             not bad news.
//
// Cost is an UPPER BOUND. Keel cannot know which orders the attacker already
// owns ahead of the event, so it does not filter them out. That bias errs on
// the safe side.
type ManipulationPoint struct {
	Delta       decimal.Decimal
	TargetPrice decimal.Decimal
	Cost        decimal.Decimal
	Reachable   bool
}

// ---------------------------------------------------------------- Flags

// Flag is one risk condition. Flags are reported individually rather than only as
// a band, so a consumer who disagrees with Keel's thresholds can still apply
// their own policy.
type Flag string

// The eleven flags. Their definitions and thresholds live in
// docs/methodology/09-flag-dan-band.md, which is the single source of truth for
// them; this list is only the vocabulary.
const (
	FlagNoExecutablePrice          Flag = "NO_EXECUTABLE_PRICE"
	FlagZeroDepth2Pct              Flag = "ZERO_DEPTH_2PCT"
	FlagManipulationCheap          Flag = "MANIPULATION_CHEAP"
	FlagManipulationRatioLow       Flag = "MANIPULATION_RATIO_LOW"
	FlagSpreadExtreme              Flag = "SPREAD_EXTREME"
	FlagNoGenuineTrade30D          Flag = "NO_GENUINE_TRADE_30D"
	FlagNoGenuineTrade7D           Flag = "NO_GENUINE_TRADE_7D"
	FlagHolderConcentrationExtreme Flag = "HOLDER_CONCENTRATION_EXTREME"
	FlagHolderConcentrationHigh    Flag = "HOLDER_CONCENTRATION_HIGH"
	FlagThinDepth5Pct              Flag = "THIN_DEPTH_5PCT"
	FlagWashTradeSuspected         Flag = "WASH_TRADE_SUSPECTED"
)

// Band is the risk level of an asset: the highest level among the triggered
// flags. There is no weighting, no averaging, and no composite score.
type Band string

// The four bands. LOW means no flag fired, which is not the same as an asset
// having been fully checked, so read BandConfidence alongside it.
const (
	BandLow      Band = "LOW"
	BandMedium   Band = "MEDIUM"
	BandHigh     Band = "HIGH"
	BandCritical Band = "CRITICAL"
)

// ---------------------------------------------------------------- Parameters

// Thresholds holds EVERY threshold in one place.
// These values are CHOSEN, not calibrated against a set of incidents.
// Changing one requires raising MethodologyVersion.
type Thresholds struct {
	ManipulationCheapAbsolute decimal.Decimal // in the quote asset
	ManipulationRatioLowPct   decimal.Decimal
	ThinDepth5PctAbsolute     decimal.Decimal
	SpreadExtremePct          decimal.Decimal
	HolderTop1ExtremePct      decimal.Decimal
	HolderTop10HighPct        decimal.Decimal
	WashTradeSuspectedPct     decimal.Decimal
	GenuineTradeStaleDays     int
	GenuineTradeWarnDays      int
}

// Params is the complete configuration input to a computation.
// No default is hidden inside a function; everything arrives through here.
type Params struct {
	// The market quality ladder, mandated by the SOW: 0.02, 0.05, 0.10
	MarketDeltas []decimal.Decimal

	// The manipulation resistance ladder: 0.5, 1, 10, 100
	// Needed because an attacker does not move a price by 10 percent, they
	// move it by a factor of 100.
	ManipulationDeltas []decimal.Decimal

	LiquidationDelta          decimal.Decimal // default 0.10
	LiquidationHaircut        decimal.Decimal // default 0.5
	ManipulationCriticalDelta decimal.Decimal // default 1.0
	ManipulationMargin        decimal.Decimal // default 0.25

	// The oracle VWAP window. The 15 minute default is an ASSUMPTION and has
	// not been confirmed as Reflector's actual window.
	OracleWindow time.Duration

	Thresholds Thresholds
}

// ---------------------------------------------------------------- Output

// SupportingMetrics fields are nil when they cannot be computed.
// Nil means "unknown", not zero.
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

// TradeRef points at one trade on the ledger. Both fields are carried so a reader
// can verify the claim against Horizon without a second lookup.
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

	Depth            []DepthPoint
	ManipulationCost []ManipulationPoint
	// MaxReachablePrice is the highest ask price on the book, which is the
	// highest price an attacker can reach. CostToMaxReachablePrice is what
	// reaching it costs.
	//
	// This pair of numbers catches the attack that slips between two discrete
	// rungs of the delta ladder. On USTRY, 21 February 2026, the values were
	// 106.7372828 at a cost of zero, and the real attack landed in the gap
	// between delta 0.5 and delta 1.
	MaxReachablePrice       *decimal.Decimal
	CostToMaxReachablePrice *decimal.Decimal
	OracleResistance        *decimal.Decimal // MC(critical) + genuine volume in the window
	MaxSafeCollateral       *decimal.Decimal

	Supporting SupportingMetrics

	Flags    []Flag
	Band     Band
	Warnings []string

	// Flags that could not be checked because the data they need is absent.
	// unevaluated is NOT a synonym for clear. An asset with no trustline data
	// must not look identical to an asset whose holder distribution was
	// actually examined.
	UnevaluatedFlags []Flag

	// partial when any flag at the CRITICAL or HIGH level is unevaluated, full
	// when every flag at those levels could be checked. The dashboard is
	// required to display this.
	BandConfidence BandConfidence
}

// BandConfidence states whether the band rests on a complete check. It exists
// because an asset with missing data must not look identical to an asset that was
// examined and found safe.
type BandConfidence string

// The two confidence values. partial means at least one flag at the CRITICAL or
// HIGH level could not be evaluated at all.
const (
	BandConfidenceFull    BandConfidence = "full"
	BandConfidencePartial BandConfidence = "partial"
)
