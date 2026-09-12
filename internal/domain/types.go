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
const MethodologyVersion = "1.0.8-draft"

// ---------------------------------------------------------------- Assets

// AssetType is Stellar's own name for the shape of an asset, carried verbatim
// so that a value from this package can be handed to Horizon unchanged.
type AssetType string

// The three asset types Stellar defines. The choice between the two credit types
// is by DECLARED type and never by code length: see the warning on Asset.
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

// IsNative reports whether this is XLM, which is the one asset with no issuer.
func (a Asset) IsNative() bool { return a.Type == AssetTypeNative }

func (a Asset) String() string {
	if a.IsNative() {
		return "XLM"
	}
	return a.Code + ":" + a.Issuer
}

// Equal compares all three fields, because asset identity is the pair
// (code, issuer) and never the ticker alone. Two assets sharing a code are
// routinely different assets: 97 of them share the AQUA ticker alone.
func (a Asset) Equal(o Asset) bool {
	return a.Code == o.Code && a.Issuer == o.Issuer && a.Type == o.Type
}

// GlobalQuote is the quote asset every threshold in this package is denominated
// in, and the primary pair of every asset. It is USDC, issuer GA5ZSEJY..., which
// is decision D-1 and Q7, settled by DEC-015 and recorded in
// docs/methodology/02-pair-selection.md sections 1 and 2: "The primary pair is
// USDC, always."
//
// THREE DECISIONS, since this package is a yellow zone.
//
// It is a FUNCTION rather than a package-level var because a var of struct type
// is writable by anything in this package, and a quote asset that one call site
// can reassign is a methodology decision with a race attached. It lives in
// domain rather than in internal/api because two consumers already need it, the
// pair resolver and the methodology endpoint, and a constant duplicated in two
// packages is the second-home drift this repository keeps paying for. And it
// carries the ISSUER, not the bare code, because rule 5 of the repository's own
// brief is that an asset is the pair (code, issuer) and is never matched on the
// ticker: /assets?asset_code=USDC returns several issuers and only this one is
// the asset the thresholds mean.
//
// REJECTED: reading it out of configs/demonstration-set.json at startup, where
// the same identity already appears on every pair. That would make a methodology
// constant depend on a file whose own note calls itself PROVISIONAL, and it would
// let a bad config silently redefine what every threshold is denominated in.
//
// It does NOT encode the candidate quote set, which section 1 of that document
// fixes at exactly two members, USDC and native XLM. That set belongs wherever
// pair selection is implemented; this is only the primary.
func GlobalQuote() Asset {
	return Asset{
		Code:   "USDC",
		Issuer: "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN",
		Type:   AssetTypeAlphanum4,
	}
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

// Valid reports whether this fraction can be used in arithmetic. A zero
// denominator is what a missing price_r field decodes to, so this is the check
// that catches an upstream field which stopped arriving.
func (p Price) Valid() bool { return p.D != 0 && p.N > 0 }

// Decimal divides the fraction out. It LOSES PRECISION and belongs at the last
// step before a number is displayed or multiplied by an amount; compare two
// prices with Cmp instead, because that never divides.
func (p Price) Decimal() decimal.Decimal {
	return decimal.NewFromInt(p.N).Div(decimal.NewFromInt(p.D))
}

// Invert swaps numerator and denominator, turning quote-per-base into
// base-per-quote. Exact, because nothing is divided.
func (p Price) Invert() Price { return Price{N: p.D, D: p.N} }

// Cmp compares without dividing, so no precision is lost.
func (p Price) Cmp(o Price) int {
	left := decimal.NewFromInt(p.N).Mul(decimal.NewFromInt(o.D))
	right := decimal.NewFromInt(o.N).Mul(decimal.NewFromInt(p.D))
	return left.Cmp(right)
}

func (p Price) String() string { return fmt.Sprintf("%d/%d", p.N, p.D) }

// ---------------------------------------------------------------- Order book

// Level is one price level of an order book: a price, and how much is available
// at it. Amount is in BASE units on both sides of the book, which the upstream
// data does not guarantee and the adapter is responsible for establishing.
type Level struct {
	Price  Price
	Amount decimal.Decimal // in base asset units
}

// Notional returns the value of this level in the quote asset.
func (l Level) Notional() decimal.Decimal { return l.Price.Decimal().Mul(l.Amount) }

// OrderBook holds both sides of a market at one ledger. Bids are sorted by
// descending price and Asks by ascending price, and the ADAPTER guarantees that
// ordering rather than this package re-establishing it.
type OrderBook struct {
	Bids []Level
	Asks []Level
}

// BestBid returns the highest bid. The bool is false when the bid side is
// empty, which is an ordinary state for a thin market and not an error.
func (b OrderBook) BestBid() (Level, bool) {
	if len(b.Bids) == 0 {
		return Level{}, false
	}
	return b.Bids[0], true
}

// BestAsk returns the lowest ask. The bool is false when the ask side is empty,
// which is an ordinary state for a thin market and not an error.
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

// SpotPrice is the constant product pool's marginal price, quote per base,
// before any fee. It returns zero rather than panicking on an empty pool, and a
// caller that needs to tell those apart asks IsEmpty first.
func (p PoolReserves) SpotPrice() decimal.Decimal {
	if p.ReserveBase.IsZero() {
		return decimal.Zero
	}
	return p.ReserveQuote.Div(p.ReserveBase)
}

// IsEmpty reports whether either reserve is zero, which is a pool that can
// price nothing. Either side being zero is enough.
func (p PoolReserves) IsEmpty() bool {
	return p.ReserveBase.IsZero() || p.ReserveQuote.IsZero()
}

// ---------------------------------------------------------------- Snapshot

// PriceSource names which half of the market a reference price was taken from.
// It records the fallback that was actually applied, so a result carries the
// answer to "where did this number come from" instead of leaving it inferred.
type PriceSource string

// The reference price sources, in the fallback order that
// docs/methodology/03-reference-price.md section 1 defines. PriceSourceNone
// means no executable price existed at all, which is a real answer.
const (
	PriceSourceBook PriceSource = "book"
	PriceSourcePool PriceSource = "pool"
	PriceSourceNone PriceSource = "none"
)

// DataSource names where a reading came from and how far it can be trusted. The
// values and their confidence ordering are on the const block below, which is
// where the distinction that matters is written down.
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

// DataSources returns the four values, in the confidence order documented above
// and never in map order. It is the ONE place the set is enumerated, so a fifth
// source is added to the const block and to this slice and nowhere else.
//
// This exists because the set has already drifted once. Migration 0001 wrote a
// CHECK constraint over three of these values and omitted offers-implied although
// this package already had it, so the database rejected exactly the historical
// path the Blend case study depends on, and 0003 had to drop and recreate the
// constraint to repair it. Every other copy of the list, in a CHECK constraint, a
// switch, or a request validator, is now checkable against this one.
func DataSources() []DataSource {
	return []DataSource{
		DataSourceHorizon,
		DataSourceHubble,
		DataSourceOffersImplied,
		DataSourceTradesImplied,
	}
}

// Valid reports whether d is one of the four. A zero DataSource is NOT valid:
// every reading knows where it came from, and defaulting an empty value to
// horizon here would let an unset field silently claim to be a live measurement.
// Callers that want a default choose it explicitly.
func (d DataSource) Valid() bool {
	for _, known := range DataSources() {
		if d == known {
			return true
		}
	}
	return false
}

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

// Flag is a named warning attached to a result. A flag states a condition that
// was observed and never a recommendation, and the API contract treats the set
// as open ended so that adding one is not a breaking change.
type Flag string

// Every flag Keel can raise. The string values belong to the contract, so they
// change only when the contract does.
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

// Band is the coarse risk bucket a result falls into. It is a summary OF the
// numbers and never a substitute for them: two assets in one band can be far
// apart, which is why a band always travels with the figures behind it.
type Band string

// The four risk bands, ordered from least to most severe.
const (
	BandLow      Band = "LOW"
	BandMedium   Band = "MEDIUM"
	BandHigh     Band = "HIGH"
	BandCritical Band = "CRITICAL"
)

// BandConfidence says how much of the evidence a band was computed from. It
// exists so that a band drawn from an incomplete reading cannot later be quoted
// as though it had been drawn from a complete one.
type BandConfidence string

// The two confidence levels. Partial means at least one input the band depends
// on was missing or truncated, and the band is still reported.
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
			// 0.1, set by DEC-017 section 1 item 3 and accepted 10 September
			// 2026. It read 1.0 for as long as nothing applied it, and the
			// record is explicit that the old value was never in force: the rule
			// could not be implemented as written, so no result was ever judged
			// against 1.0. Changing it is therefore not a recalibration.
			ManipulationRatioLowPct: dec("0.1"),
			ThinDepth5PctAbsolute:   dec("50000"),
			SpreadExtremePct:        dec("20.0"),
			PriceDivergencePct:      dec("10.0"),
			HolderTop1ExtremePct:    dec("50.0"),
			HolderTop10HighPct:      dec("80.0"),
			WashTradeSuspectedPct:   dec("50.0"),
			GenuineTradeStaleDays:   30,
			GenuineTradeWarnDays:    7,
		},
	}
}

// ---------------------------------------------------------------- Output

// SupportingMetrics fields are nil when they cannot be computed.
// Nil means "unknown", NOT zero.
type SupportingMetrics struct {
	HolderTop1Pct  *decimal.Decimal
	HolderTop10Pct *decimal.Decimal
	HolderHHI      *decimal.Decimal

	// HolderSnapshotLedger is the ledger the three figures above were read at,
	// and it is NOT the AssetRisk.LedgerSeq they end up beside. DEC-018 point 2,
	// accepted 12 September 2026.
	//
	// Three sentences, as types.go requires. It sits on this struct rather than on
	// AssetRisk because the three holder figures and the ledger they came from are
	// one measurement, and a shape that lets them be carried separately is a shape
	// where they can be separated, which is the whole failure the record describes.
	// DEC-011 already settled that a trustline pull resolves to one snapshot ledger
	// and said in as many words that it is "the LedgerSeq the pull carries
	// downstream", so this field is that sentence given somewhere to live rather
	// than a new idea. It changes no contract: internal/api/wire.go maps the
	// response field by field and internal/store/jsonb.go declares its own storage
	// shapes, so neither picks a new field up by accident, and DEC-018 point 4,
	// which would expose it, is still a draft nobody has accepted.
	//
	// REJECTED ALTERNATIVE: pass it beside the struct, as a second argument to
	// store.SaveMetrics and a second return from the scan helper. That was built
	// first and thrown away. It made the invariant something each call site had to
	// remember instead of something the type guarantees, it needed a validator in
	// the store to catch the case where somebody forgot, and it broke ten existing
	// tests that pass a result carrying holder figures through the one-argument
	// door. The version you are reading needed a single line changed in one test
	// fixture.
	//
	// Nil when the three figures are nil. The store refuses the two states where
	// they disagree, and migrations/0007 refuses them again in SQL.
	HolderSnapshotLedger *uint32

	// CirculatingSupply is the summed balance of the population the holder pull
	// kept, in BASE units. It rides here beside the three concentration figures
	// because it is the denominator all three were divided by, and because
	// MANIPULATION_RATIO_LOW needs it: DEC-017 defines that rule's
	// circulating_supply_value as this figure multiplied by P0.
	//
	// It is nil exactly when the concentration figures are nil, and for the same
	// reason. A truncated trustline pull answers the question not at all, so it
	// stores no supply either, and a zero here would read as an asset nobody
	// holds rather than as an asset nobody finished counting.
	CirculatingSupply *decimal.Decimal

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

// TradeRef points at a trade by the ledger it closed in and when that ledger
// closed. It is a reference rather than a copy, so a reader can go back to the
// chain and check the claim instead of trusting this record of it.
type TradeRef struct {
	LedgerSeq uint32
	At        time.Time
}

// PairSummary is one evaluated quote pair reduced to the three things that are
// comparable ACROSS pairs. Its depth, cost and collateral figures are deliberately
// absent: those are denominated in the pair's own quote asset, and putting two
// units side by side in one response invites exactly the arithmetic that
// docs/methodology/02-pair-selection.md section 1 exists to prevent. A band and a
// confidence are unitless by construction, so they travel.
type PairSummary struct {
	Quote          Asset
	Band           Band
	BandConfidence BandConfidence
}

// WarningSecondaryPairWorse is emitted when a quote pair other than the primary one
// set the band. It is a CODE rather than a sentence, unlike every other member of
// AssetRisk.Warnings, because a consumer has to branch on this one: the headline
// depth and cost figures belong to the primary pair while the band does not, and
// prose cannot be switched on. BandDrivenBy names the pair responsible.
//
// Defined by docs/methodology/02-pair-selection.md section 2 item 4.
const WarningSecondaryPairWorse = "SECONDARY_PAIR_WORSE"

// AssetRisk is the complete output for one asset at one ledger.
type AssetRisk struct {
	Base               Asset
	Quote              Asset
	LedgerSeq          uint32
	LedgerClosedAt     time.Time
	MethodologyVersion string
	DataSource         DataSource

	// PrimaryQuote is the quote asset every headline figure below is denominated
	// in. It is USDC for every asset and takes no per-asset rule, so it is
	// redundant with Quote today and is emitted anyway: widening the candidate
	// quote set later must not silently change what an already-stored row meant.
	//
	// docs/methodology/02-pair-selection.md sections 1 and 2 item 2.
	PrimaryQuote Asset

	MidPrice    *decimal.Decimal // nil when PriceSource is none
	PriceSource PriceSource
	SpreadPct   *decimal.Decimal

	// PoolSpotPrice and PriceDivergencePct are always populated when an active pool
	// exists, regardless of which branch the P0 rule took. A large divergence raises
	// PRICE_SOURCE_CONFLICT and causes P0 to be taken from the pool.
	PoolSpotPrice      *decimal.Decimal
	PriceDivergencePct *decimal.Decimal

	// XlmUsdcRate is the XLM/USDC mid price at THIS LedgerSeq, and it is published
	// for one reason: the thresholds are USDC figures, so an XLM-quoted pair has to
	// be converted before it can be judged against them, and a reader must be able
	// to redo that conversion with a rate of their own choosing.
	//
	// This is not a price oracle and does not breach principle P-1. The rate is read
	// from the same books and pools Keel already reads, from the deepest market on
	// the network, at the same ledger as everything else in this result.
	//
	// Nil when no XLM-quoted pair was evaluated, and nil when the rate itself was not
	// trustworthy at that ledger. Those two are not distinguished here on purpose:
	// PairsEvaluated is where the second case shows up, as an XLM entry whose flags
	// are unevaluated and whose confidence is partial.
	XlmUsdcRate *decimal.Decimal

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

	// PairsEvaluated holds every quote pair that was evaluated for this asset,
	// including the primary one, sorted by quote asset so that the order is
	// reproducible under NFR-9. In this version the candidate set is exactly two
	// members, USDC and native XLM, and that bound is a stated limitation of the
	// version rather than an unstated shortfall against FR-11.
	PairsEvaluated []PairSummary

	// BandDrivenBy names the quote asset of the pair whose evaluation set Band.
	// Band is the HIGHEST tier triggered on any evaluated pair, which is principle
	// P-2 applied to the choice of pair: an attacker takes the cheapest path, so an
	// asset deep on USDC and thin on XLM is not a safe asset.
	//
	// Equal to PrimaryQuote in the ordinary case. When it is not, the warning
	// WarningSecondaryPairWorse is emitted alongside it. Depth, cost and
	// MaxSafeCollateral above stay the primary pair's figures either way and are
	// never mixed across pairs.
	BandDrivenBy Asset

	Warnings []string
}

// Crossed reports whether the best bid is at or above the best ask, and returns
// the two levels that cross when it is.
//
// A CROSSED BOOK CANNOT EXIST ON A STELLAR LEDGER. The matching engine fills from
// the best price, so the moment a bid reaches an ask the two execute and one of
// them leaves the book. Every crossed book is therefore a defect in whatever
// produced it, never a market state, and this predicate exists so that a producer
// can say so rather than emit the rows in silence.
//
// The comparison is `>=` and not `>`, because equality crosses too: a bid at
// exactly the ask price executes against it.
//
// WHY THIS LIVES IN domain RATHER THAN IN THE ADAPTER THAT FOUND THE DEFECT.
// The question "is this book possible" is a property of a book and not of the
// route that built it, and internal/horizon has two such routes, replay.go and
// rewind.go, that would otherwise carry a copy each. A live Horizon read cannot
// cross and asking costs two comparisons, so the check is cheap where it is
// pointless and present where it is not.
//
// It reports and does not repair. Dropping the crossing level would make the book
// possible and keep it wrong, and which of the two offers is the phantom is not
// decidable from the book alone: it takes the operation and trade history of both.
// See docs/evidences/2026-09-12-crossed-book-ustry-february.md, where deciding it
// for one pair took a walk of each side.
func (b OrderBook) Crossed() (bid, ask Level, crossed bool) {
	bid, okBid := b.BestBid()
	ask, okAsk := b.BestAsk()
	if !okBid || !okAsk {
		return Level{}, Level{}, false
	}
	if bid.Price.Decimal().GreaterThanOrEqual(ask.Price.Decimal()) {
		return bid, ask, true
	}
	return Level{}, Level{}, false
}
