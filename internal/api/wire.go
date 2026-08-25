// The wire format, declared once.
//
// These structs ARE the contract in docs/api/keel-openapi.yaml. They are written
// out rather than derived from the domain types for the same reason
// internal/store declares its own JSONB shapes: a field name on the wire is a
// promise to a consumer that outlives any Go rename, and the frontend has already
// been handed mocks generated from that contract.
//
// TWO RULES GOVERN EVERY FIELD HERE, and both come from this package's brief.
//
// Every decimal is a STRING. A JSON number is an IEEE 754 double and Stellar
// amounts are int64 stroops with seven decimals. The only numbers are `delta`,
// `criticalDelta`, and whole counts such as `ledgerSeq` and `windowSeconds`,
// which the contract types as numbers.
//
// Every field whose name ends in Pct is on a PERCENT scale. `spreadPct` of
// '196.0777141' means 196 percent, not 1.96. The domain carries these on the same
// scale, so this layer passes them through unchanged; if that ever stops being
// true, the conversion belongs here and nowhere else.

package api

import (
	"time"

	"github.com/shopspring/decimal"

	"github.com/Keel-Official/keel-backend/internal/domain"
	"github.com/Keel-Official/keel-backend/internal/store"
)

// jsonNumber emits an unquoted JSON number from a decimal's own digits.
//
// The contract types `delta` and `criticalDelta` as numbers, not as decimal
// strings, so those two fields cannot be quoted. Rendering them through float64
// would violate non-negotiable rule 1 and is banned repository wide by the arch
// test. This carries the decimal's exact digits into the JSON document without
// any binary floating point step existing at all.
type jsonNumber string

func (n jsonNumber) MarshalJSON() ([]byte, error) {
	if n == "" {
		return []byte("null"), nil
	}
	return []byte(n), nil
}

func num(d decimal.Decimal) jsonNumber { return jsonNumber(d.String()) }

// str renders a required decimal. dec renders a nullable one, where nil becomes
// JSON null and never "0": every nullable field in this contract documents that
// null means unknown or not applicable.
func str(d decimal.Decimal) string { return d.String() }

func dec(d *decimal.Decimal) *string {
	if d == nil {
		return nil
	}
	s := d.String()
	return &s
}

// ---------------------------------------------------------------- assets

type assetJSON struct {
	Code string `json:"code"`
	Type string `json:"type"`
	// Issuer is a pointer so the native asset serializes as null rather than as
	// an empty string. The contract says null if and only if type is native.
	Issuer *string `json:"issuer"`
}

func asset(a domain.Asset) assetJSON {
	out := assetJSON{Code: a.Code, Type: string(a.Type)}
	if !a.IsNative() {
		issuer := a.Issuer
		out.Issuer = &issuer
	}
	return out
}

// ---------------------------------------------------------------- risk

type depthJSON struct {
	Delta    jsonNumber `json:"delta"`
	BuySide  string     `json:"buySide"`
	SellSide string     `json:"sellSide"`
	FromSdex string     `json:"fromSdex"`
	FromAmm  string     `json:"fromAmm"`
}

type manipulationJSON struct {
	Delta       jsonNumber `json:"delta"`
	TargetPrice string     `json:"targetPrice"`
	Cost        string     `json:"cost"`
	// Reachable is never omitted. The contract says cost must never be read
	// without it: a zero cost with reachable true means the target is free, and
	// with reachable false means there is no liquidity in range at all.
	Reachable bool `json:"reachable"`
}

type oracleResistanceJSON struct {
	CriticalDelta    jsonNumber `json:"criticalDelta"`
	ManipulationCost string     `json:"manipulationCost"`
	Reachable        bool       `json:"reachable"`
	GenuineVolume    string     `json:"genuineVolume"`
	WindowSeconds    int        `json:"windowSeconds"`
	Ratio            *string    `json:"ratio"`
	TotalAttackCost  *string    `json:"totalAttackCost"`
}

type volumeToSupplyJSON struct {
	D1  *string `json:"d1"`
	D7  *string `json:"d7"`
	D30 *string `json:"d30"`
}

type lastGenuineTradeJSON struct {
	LedgerSeq uint32    `json:"ledgerSeq"`
	At        time.Time `json:"at"`
}

// assetRiskJSON is the GET /asset/{assetId}/depth response.
//
// The field order follows the contract's own property order, so the two can be
// read side by side.
type assetRiskJSON struct {
	Asset              assetJSON `json:"asset"`
	Quote              assetJSON `json:"quote"`
	LedgerSeq          uint32    `json:"ledgerSeq"`
	LedgerClosedAt     time.Time `json:"ledgerClosedAt"`
	ComputedAt         time.Time `json:"computedAt"`
	MethodologyVersion string    `json:"methodologyVersion"`
	DataSource         string    `json:"dataSource"`

	MidPrice           *string `json:"midPrice"`
	PriceSource        string  `json:"priceSource"`
	PoolSpotPrice      *string `json:"poolSpotPrice"`
	PriceDivergencePct *string `json:"priceDivergencePct"`
	SpreadPct          *string `json:"spreadPct"`

	Depth []depthJSON `json:"depth"`

	ManipulationCostCombined      []manipulationJSON `json:"manipulationCostCombined"`
	ManipulationCostOrderbookOnly []manipulationJSON `json:"manipulationCostOrderbookOnly"`

	MaxReachablePrice       *string `json:"maxReachablePrice"`
	CostToMaxReachablePrice *string `json:"costToMaxReachablePrice"`

	OracleResistance *oracleResistanceJSON `json:"oracleResistance"`

	MaxSafeCollateral             *string `json:"maxSafeCollateral"`
	MaxSafeCollateralLiquidation  *string `json:"maxSafeCollateralLiquidation"`
	MaxSafeCollateralManipulation *string `json:"maxSafeCollateralManipulation"`

	HolderTop1Pct  *string `json:"holderTop1Pct"`
	HolderTop10Pct *string `json:"holderTop10Pct"`
	// The contract spells this holderHhi, not holderHHI. Matching the contract
	// rather than Go's initialism convention, because the contract is the thing
	// a consumer already generated code from.
	HolderHhi         *string               `json:"holderHhi"`
	VolumeToSupply    *volumeToSupplyJSON   `json:"volumeToSupply"`
	LastGenuineTrade  *lastGenuineTradeJSON `json:"lastGenuineTrade"`
	TradesExcludedPct *string               `json:"tradesExcludedPct"`

	Flags            []domain.Flag `json:"flags"`
	UnevaluatedFlags []domain.Flag `json:"unevaluatedFlags"`
	Band             string        `json:"band"`
	BandConfidence   string        `json:"bandConfidence"`
	Warnings         []string      `json:"warnings"`
}

func riskResponse(m store.Metric) assetRiskJSON {
	r := m.Risk
	out := assetRiskJSON{
		Asset:              asset(r.Base),
		Quote:              asset(r.Quote),
		LedgerSeq:          r.LedgerSeq,
		LedgerClosedAt:     r.LedgerClosedAt.UTC(),
		ComputedAt:         m.ComputedAt.UTC(),
		MethodologyVersion: r.MethodologyVersion,
		DataSource:         string(r.DataSource),

		MidPrice:           dec(r.MidPrice),
		PriceSource:        string(r.PriceSource),
		PoolSpotPrice:      dec(r.PoolSpotPrice),
		PriceDivergencePct: dec(r.PriceDivergencePct),
		SpreadPct:          dec(r.SpreadPct),

		MaxReachablePrice:       dec(r.MaxReachablePrice),
		CostToMaxReachablePrice: dec(r.CostToMaxReachablePrice),

		MaxSafeCollateral:             dec(r.MaxSafeCollateral),
		MaxSafeCollateralLiquidation:  dec(r.MaxSafeCollateralLiquidation),
		MaxSafeCollateralManipulation: dec(r.MaxSafeCollateralManipulation),

		HolderTop1Pct:     dec(r.Supporting.HolderTop1Pct),
		HolderTop10Pct:    dec(r.Supporting.HolderTop10Pct),
		HolderHhi:         dec(r.Supporting.HolderHHI),
		TradesExcludedPct: dec(r.Supporting.TradesExcludedPct),

		Band:           string(r.Band),
		BandConfidence: string(r.BandConfidence),
	}

	// Arrays are never null on the wire. The contract marks flags,
	// unevaluatedFlags, warnings and depth as required, and a consumer that has
	// to guard every array for null before iterating will forget once.
	out.Depth = make([]depthJSON, 0, len(r.Depth))
	for _, p := range r.Depth {
		out.Depth = append(out.Depth, depthJSON{
			Delta:    num(p.Delta),
			BuySide:  str(p.BuySide),
			SellSide: str(p.SellSide),
			FromSdex: str(p.FromSdex),
			FromAmm:  str(p.FromAmm),
		})
	}
	out.ManipulationCostCombined = manipulationList(r.ManipulationCostCombined)
	out.ManipulationCostOrderbookOnly = manipulationList(r.ManipulationCostOrderbookOnly)

	out.Flags = flagsOrEmpty(r.Flags)
	out.UnevaluatedFlags = flagsOrEmpty(r.UnevaluatedFlags)
	out.Warnings = r.Warnings
	if out.Warnings == nil {
		out.Warnings = []string{}
	}

	if o := r.OracleResistance; o != nil {
		out.OracleResistance = &oracleResistanceJSON{
			CriticalDelta:    num(o.CriticalDelta),
			ManipulationCost: str(o.ManipulationCost),
			Reachable:        o.Reachable,
			GenuineVolume:    str(o.GenuineVolume),
			WindowSeconds:    o.WindowSeconds,
			Ratio:            dec(o.Ratio),
			TotalAttackCost:  dec(o.TotalAttackCost),
		}
	}

	sup := r.Supporting
	if sup.VolumeToSupplyD1 != nil || sup.VolumeToSupplyD7 != nil || sup.VolumeToSupplyD30 != nil {
		out.VolumeToSupply = &volumeToSupplyJSON{
			D1:  dec(sup.VolumeToSupplyD1),
			D7:  dec(sup.VolumeToSupplyD7),
			D30: dec(sup.VolumeToSupplyD30),
		}
	}
	if t := sup.LastGenuineTrade; t != nil {
		out.LastGenuineTrade = &lastGenuineTradeJSON{LedgerSeq: t.LedgerSeq, At: t.At.UTC()}
	}
	return out
}

func manipulationList(points []domain.ManipulationPoint) []manipulationJSON {
	out := make([]manipulationJSON, 0, len(points))
	for _, p := range points {
		out = append(out, manipulationJSON{
			Delta:       num(p.Delta),
			TargetPrice: str(p.TargetPrice),
			Cost:        str(p.Cost),
			Reachable:   p.Reachable,
		})
	}
	return out
}

func flagsOrEmpty(in []domain.Flag) []domain.Flag {
	if in == nil {
		return []domain.Flag{}
	}
	return in
}

// ---------------------------------------------------------------- list

type assetSummaryJSON struct {
	Asset             assetJSON     `json:"asset"`
	Quote             assetJSON     `json:"quote"`
	MidPrice          *string       `json:"midPrice"`
	PriceSource       string        `json:"priceSource"`
	Depth5PctBuySide  *string       `json:"depth5PctBuySide"`
	MaxSafeCollateral *string       `json:"maxSafeCollateral"`
	Band              string        `json:"band"`
	BandConfidence    string        `json:"bandConfidence"`
	Flags             []domain.Flag `json:"flags"`
	LedgerSeq         uint32        `json:"ledgerSeq"`
}

type assetListJSON struct {
	Items              []assetSummaryJSON `json:"items"`
	Total              int                `json:"total"`
	Limit              int                `json:"limit"`
	Offset             int                `json:"offset"`
	MethodologyVersion string             `json:"methodologyVersion"`
}

func summaryResponse(m store.Metric) assetSummaryJSON {
	r := m.Risk
	return assetSummaryJSON{
		Asset:             asset(r.Base),
		Quote:             asset(r.Quote),
		MidPrice:          dec(r.MidPrice),
		PriceSource:       string(r.PriceSource),
		Depth5PctBuySide:  depthAt(r.Depth, "0.05"),
		MaxSafeCollateral: dec(r.MaxSafeCollateral),
		Band:              string(r.Band),
		BandConfidence:    string(r.BandConfidence),
		Flags:             flagsOrEmpty(r.Flags),
		LedgerSeq:         r.LedgerSeq,
	}
}

// depthAt picks one rung of the ladder by its delta.
//
// It matches on the VALUE and not on the position, because a ladder that arrives
// in another order, or short a rung, would otherwise be read as though the rung
// it does have were the one asked for. Nil when that rung is absent, which is
// null on the wire and means the rung was not computed rather than zero depth.
func depthAt(points []domain.DepthPoint, delta string) *string {
	want := decimal.RequireFromString(delta)
	for _, p := range points {
		if p.Delta.Equal(want) {
			s := p.BuySide.String()
			return &s
		}
	}
	return nil
}

// ---------------------------------------------------------------- history

type historyPointJSON struct {
	LedgerSeq             uint32        `json:"ledgerSeq"`
	LedgerClosedAt        time.Time     `json:"ledgerClosedAt"`
	MidPrice              *string       `json:"midPrice"`
	Depth2PctBuySide      *string       `json:"depth2PctBuySide"`
	Depth5PctBuySide      *string       `json:"depth5PctBuySide"`
	Depth10PctBuySide     *string       `json:"depth10PctBuySide"`
	ManipulationCost50Pct *string       `json:"manipulationCost50Pct"`
	MaxSafeCollateral     *string       `json:"maxSafeCollateral"`
	Band                  string        `json:"band"`
	Flags                 []domain.Flag `json:"flags"`
}

type historyGapJSON struct {
	From   uint32 `json:"from"`
	To     uint32 `json:"to"`
	Reason string `json:"reason"`
}

type historyJSON struct {
	Asset              assetJSON          `json:"asset"`
	Quote              assetJSON          `json:"quote"`
	From               uint32             `json:"from"`
	To                 uint32             `json:"to"`
	Resolution         string             `json:"resolution"`
	MethodologyVersion string             `json:"methodologyVersion"`
	DataSource         string             `json:"dataSource"`
	Gaps               []historyGapJSON   `json:"gaps"`
	Points             []historyPointJSON `json:"points"`
}

func historyPoint(m store.Metric) historyPointJSON {
	r := m.Risk
	return historyPointJSON{
		LedgerSeq:             r.LedgerSeq,
		LedgerClosedAt:        r.LedgerClosedAt.UTC(),
		MidPrice:              dec(r.MidPrice),
		Depth2PctBuySide:      depthAt(r.Depth, "0.02"),
		Depth5PctBuySide:      depthAt(r.Depth, "0.05"),
		Depth10PctBuySide:     depthAt(r.Depth, "0.10"),
		ManipulationCost50Pct: manipulationAt(r.ManipulationCostOrderbookOnly, "0.5"),
		MaxSafeCollateral:     dec(r.MaxSafeCollateral),
		Band:                  string(r.Band),
		Flags:                 flagsOrEmpty(r.Flags),
	}
}

// manipulationAt reads the orderbook-only ladder, not the combined one.
//
// The contract calls the field manipulationCost50Pct without saying which venue
// set it means. The orderbook-only figure is the binding one, because an attacker
// takes the cheapest path, and it is the one behind maxSafeCollateral. A trend
// line drawn from the combined figure would have shown the February incident as
// an asset that was expensive to move, which is the opposite of what happened.
//
// It returns nil rather than the cost when the target is UNREACHABLE. A cost to
// an unreachable target is not the cost of anything, and a chart cannot carry the
// reachable flag alongside each point.
func manipulationAt(points []domain.ManipulationPoint, delta string) *string {
	want := decimal.RequireFromString(delta)
	for _, p := range points {
		if !p.Delta.Equal(want) {
			continue
		}
		if !p.Reachable {
			return nil
		}
		s := p.Cost.String()
		return &s
	}
	return nil
}

// ---------------------------------------------------------------- meta

type healthJSON struct {
	Status              string     `json:"status"`
	LatestScanAt        *time.Time `json:"latestScanAt"`
	LatestScanLedgerSeq *uint32    `json:"latestScanLedgerSeq"`
	AssetsMonitored     int        `json:"assetsMonitored"`
	MethodologyVersion  string     `json:"methodologyVersion"`
	HistoricalAvailable bool       `json:"historicalAvailable"`
}

type methodologyJSON struct {
	Version         string         `json:"version"`
	DocumentUrl     string         `json:"documentUrl"`
	Calibrated      bool           `json:"calibrated"`
	CalibrationNote string         `json:"calibrationNote"`
	Thresholds      map[string]any `json:"thresholds"`
}

// ---------------------------------------------------------------- errors

type errorBodyJSON struct {
	Error errorDetailJSON `json:"error"`
}

type errorDetailJSON struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Detail  map[string]any `json:"detail,omitempty"`
}
