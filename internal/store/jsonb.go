// The shapes of the JSONB columns.
//
// These are declared once, here, rather than by marshaling the domain types
// directly. A field name inside a JSONB column is a storage format: it outlives
// any Go rename, and a rename that silently changed it would leave every row
// written before the rename unreadable by the code that comes after. The names
// match the API contract so that a column can be read beside a response without
// translating, and every decimal is a STRING even where the contract sends a
// number, because a JSON number is an IEEE 754 double.

package store

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	"github.com/Keel-Official/keel-backend/internal/domain"
)

// ---------------------------------------------------------------- depth

type depthJSON struct {
	Delta    string `json:"delta"`
	BuySide  string `json:"buySide"`
	SellSide string `json:"sellSide"`
	FromSdex string `json:"fromSdex"`
	FromAmm  string `json:"fromAmm"`
}

func encodeDepth(points []domain.DepthPoint) ([]byte, error) {
	// A nil slice must serialize as [] and not as null: the column is NOT NULL
	// and an asset with no depth ladder at all is a different statement from an
	// empty one.
	out := make([]depthJSON, 0, len(points))
	for _, p := range points {
		out = append(out, depthJSON{
			Delta:    p.Delta.String(),
			BuySide:  p.BuySide.String(),
			SellSide: p.SellSide.String(),
			FromSdex: p.FromSdex.String(),
			FromAmm:  p.FromAmm.String(),
		})
	}
	return json.Marshal(out)
}

func decodeDepth(body []byte) ([]domain.DepthPoint, error) {
	var in []depthJSON
	if err := json.Unmarshal(body, &in); err != nil {
		return nil, fmt.Errorf("store: depth: %w", err)
	}
	out := make([]domain.DepthPoint, 0, len(in))
	for i, p := range in {
		var (
			point domain.DepthPoint
			err   error
		)
		if point.Delta, err = decimal.NewFromString(p.Delta); err != nil {
			return nil, fmt.Errorf("store: depth[%d].delta: %w", i, err)
		}
		if point.BuySide, err = decimal.NewFromString(p.BuySide); err != nil {
			return nil, fmt.Errorf("store: depth[%d].buySide: %w", i, err)
		}
		if point.SellSide, err = decimal.NewFromString(p.SellSide); err != nil {
			return nil, fmt.Errorf("store: depth[%d].sellSide: %w", i, err)
		}
		if point.FromSdex, err = decimal.NewFromString(p.FromSdex); err != nil {
			return nil, fmt.Errorf("store: depth[%d].fromSdex: %w", i, err)
		}
		if point.FromAmm, err = decimal.NewFromString(p.FromAmm); err != nil {
			return nil, fmt.Errorf("store: depth[%d].fromAmm: %w", i, err)
		}
		out = append(out, point)
	}
	return out, nil
}

// ---------------------------------------------------------------- manipulation

type manipulationJSON struct {
	Delta       string `json:"delta"`
	TargetPrice string `json:"targetPrice"`
	Cost        string `json:"cost"`
	// Reachable is stored beside every cost and never omitted. A cost read
	// without it is meaningless: when the target cannot be reached, that number
	// is not the cost of reaching anything.
	Reachable bool `json:"reachable"`
}

func encodeManipulation(points []domain.ManipulationPoint) ([]byte, error) {
	out := make([]manipulationJSON, 0, len(points))
	for _, p := range points {
		out = append(out, manipulationJSON{
			Delta:       p.Delta.String(),
			TargetPrice: p.TargetPrice.String(),
			Cost:        p.Cost.String(),
			Reachable:   p.Reachable,
		})
	}
	return json.Marshal(out)
}

func decodeManipulation(body []byte) ([]domain.ManipulationPoint, error) {
	var in []manipulationJSON
	if err := json.Unmarshal(body, &in); err != nil {
		return nil, fmt.Errorf("store: manipulation cost: %w", err)
	}
	out := make([]domain.ManipulationPoint, 0, len(in))
	for i, p := range in {
		var (
			point domain.ManipulationPoint
			err   error
		)
		if point.Delta, err = decimal.NewFromString(p.Delta); err != nil {
			return nil, fmt.Errorf("store: manipulationCost[%d].delta: %w", i, err)
		}
		if point.TargetPrice, err = decimal.NewFromString(p.TargetPrice); err != nil {
			return nil, fmt.Errorf("store: manipulationCost[%d].targetPrice: %w", i, err)
		}
		if point.Cost, err = decimal.NewFromString(p.Cost); err != nil {
			return nil, fmt.Errorf("store: manipulationCost[%d].cost: %w", i, err)
		}
		point.Reachable = p.Reachable
		out = append(out, point)
	}
	return out, nil
}

// ---------------------------------------------------------------- oracle

type oracleResistanceJSON struct {
	CriticalDelta    string `json:"criticalDelta"`
	ManipulationCost string `json:"manipulationCost"`
	Reachable        bool   `json:"reachable"`
	GenuineVolume    string `json:"genuineVolume"`
	WindowSeconds    int    `json:"windowSeconds"`
	// Ratio and TotalAttackCost are pointers so null survives the round trip.
	// Null here means undefined, which happens when GenuineVolume is zero or the
	// target is unreachable, and both of those are findings rather than gaps.
	Ratio           *string `json:"ratio"`
	TotalAttackCost *string `json:"totalAttackCost"`
}

func encodeOracleResistance(o *domain.OracleResistance) (any, error) {
	if o == nil {
		return nil, nil
	}
	body, err := json.Marshal(oracleResistanceJSON{
		CriticalDelta:    o.CriticalDelta.String(),
		ManipulationCost: o.ManipulationCost.String(),
		Reachable:        o.Reachable,
		GenuineVolume:    o.GenuineVolume.String(),
		WindowSeconds:    o.WindowSeconds,
		Ratio:            decimalString(o.Ratio),
		TotalAttackCost:  decimalString(o.TotalAttackCost),
	})
	if err != nil {
		return nil, err
	}
	return string(body), nil
}

func decodeOracleResistance(body []byte) (*domain.OracleResistance, error) {
	var in oracleResistanceJSON
	if err := json.Unmarshal(body, &in); err != nil {
		return nil, fmt.Errorf("store: oracle resistance: %w", err)
	}
	out := domain.OracleResistance{
		Reachable:     in.Reachable,
		WindowSeconds: in.WindowSeconds,
	}
	var err error
	if out.CriticalDelta, err = decimal.NewFromString(in.CriticalDelta); err != nil {
		return nil, fmt.Errorf("store: oracleResistance.criticalDelta: %w", err)
	}
	if out.ManipulationCost, err = decimal.NewFromString(in.ManipulationCost); err != nil {
		return nil, fmt.Errorf("store: oracleResistance.manipulationCost: %w", err)
	}
	if out.GenuineVolume, err = decimal.NewFromString(in.GenuineVolume); err != nil {
		return nil, fmt.Errorf("store: oracleResistance.genuineVolume: %w", err)
	}
	if out.Ratio, err = parseOptional(in.Ratio, "oracleResistance.ratio"); err != nil {
		return nil, err
	}
	if out.TotalAttackCost, err = parseOptional(in.TotalAttackCost, "oracleResistance.totalAttackCost"); err != nil {
		return nil, err
	}
	return &out, nil
}

// ---------------------------------------------------------------- supporting

type volumeToSupplyJSON struct {
	D1  *string `json:"d1"`
	D7  *string `json:"d7"`
	D30 *string `json:"d30"`
}

// encodeVolumeToSupply returns nil when all three windows are unknown. The
// column is nullable and a row of three nulls says the same thing as no row
// while costing a reader an extra step to work that out.
func encodeVolumeToSupply(m domain.SupportingMetrics) (any, error) {
	if m.VolumeToSupplyD1 == nil && m.VolumeToSupplyD7 == nil && m.VolumeToSupplyD30 == nil {
		return nil, nil
	}
	body, err := json.Marshal(volumeToSupplyJSON{
		D1:  decimalString(m.VolumeToSupplyD1),
		D7:  decimalString(m.VolumeToSupplyD7),
		D30: decimalString(m.VolumeToSupplyD30),
	})
	if err != nil {
		return nil, err
	}
	return string(body), nil
}

func decodeVolumeToSupply(body []byte, m *domain.SupportingMetrics) error {
	var in volumeToSupplyJSON
	if err := json.Unmarshal(body, &in); err != nil {
		return fmt.Errorf("store: volume to supply: %w", err)
	}
	var err error
	if m.VolumeToSupplyD1, err = parseOptional(in.D1, "volumeToSupply.d1"); err != nil {
		return err
	}
	if m.VolumeToSupplyD7, err = parseOptional(in.D7, "volumeToSupply.d7"); err != nil {
		return err
	}
	if m.VolumeToSupplyD30, err = parseOptional(in.D30, "volumeToSupply.d30"); err != nil {
		return err
	}
	return nil
}

type lastGenuineTradeJSON struct {
	LedgerSeq uint32    `json:"ledgerSeq"`
	At        time.Time `json:"at"`
}

func encodeLastGenuineTrade(t *domain.TradeRef) (any, error) {
	if t == nil {
		return nil, nil
	}
	body, err := json.Marshal(lastGenuineTradeJSON{LedgerSeq: t.LedgerSeq, At: t.At.UTC()})
	if err != nil {
		return nil, err
	}
	return string(body), nil
}

func decodeLastGenuineTrade(body []byte) (*domain.TradeRef, error) {
	var in lastGenuineTradeJSON
	if err := json.Unmarshal(body, &in); err != nil {
		return nil, fmt.Errorf("store: last genuine trade: %w", err)
	}
	return &domain.TradeRef{LedgerSeq: in.LedgerSeq, At: in.At}, nil
}

// ---------------------------------------------------------------- helpers

func decimalString(d *decimal.Decimal) *string {
	if d == nil {
		return nil
	}
	s := d.String()
	return &s
}

func parseOptional(s *string, field string) (*decimal.Decimal, error) {
	if s == nil {
		return nil, nil
	}
	d, err := decimal.NewFromString(*s)
	if err != nil {
		return nil, fmt.Errorf("store: %s: %q: %w", field, *s, err)
	}
	return &d, nil
}
