// Package adapter translates Horizon responses into domain types.
//
// Two Go traps handled here, both established from real data:
//
//  1. /trades  -> price.{n,d} are STRINGS, under a field named "price"
//     /offers  -> price_r.{n,d} are NUMBERS, under a field named "price_r"
//     A single struct for both either fails to unmarshal or silently yields zero.
//     flexInt64 accepts either shape.
//
//  2. On /trades, the meaning of price depends on which asset is the base. For
//     the USTRY/USDC exploit trade the base is USDC and the counter is USTRY, so
//     price.n/price.d = 0.009369 is USTRY per USDC, the INVERSE of what a reader
//     expects. NormalizePrice flips the direction when needed.
//
// NOTE: as of 20 August 2026 nothing imports this package, and golangci-lint
// reports all four declarations below as unused. It also uses float64, which the
// non-negotiable rules forbid. Its fate is an open decision; see finding P1-16 in
// docs/internal/audit-2026-08-20.md.
package adapter

import (
	"fmt"
	"strconv"
	"strings"
)

// flexInt64 unmarshals from a JSON number OR a JSON string.
type flexInt64 int64

func (f *flexInt64) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return fmt.Errorf("flexInt64 %q: %w", s, err)
	}
	*f = flexInt64(v)
	return nil
}

// priceRatio is the n/d ratio from Horizon, whatever its original JSON type.
type priceRatio struct {
	N flexInt64 `json:"n"`
	D flexInt64 `json:"d"`
}

func (p priceRatio) float() float64 {
	if p.D == 0 {
		return 0
	}
	return float64(p.N) / float64(p.D)
}

// NormalizePrice returns the price in counter-per-base direction for the pair
// that was REQUESTED (quote per base). If the base on the record is in fact the
// requested quote asset, the price is inverted.
//
//	raw               : the n/d value from the record
//	recordBaseIsQuote : true when the record's base is the requested quote asset
func NormalizePrice(raw float64, recordBaseIsQuote bool) float64 {
	if recordBaseIsQuote {
		if raw == 0 {
			return 0
		}
		return 1.0 / raw
	}
	return raw
}
