// Package horizon is the adapter for live data from the Horizon API.
//
// This file holds the only piece salvaged from the deleted internal/adapter:
// decoding of Horizon's price fraction. It is kept because the trap it handles
// was found in real data rather than imagined, and it costs a reader nothing to
// carry it.
//
// The trap. Horizon sends the same rational number in two different JSON shapes
// depending on which endpoint you ask:
//
//	/offers  -> "price_r": {"n": 266843207, "d": 2500000}     JSON numbers
//	/trades  -> "price":   {"n": "2500000", "d": "266843207"} JSON strings
//
// A single struct written for one shape either fails to unmarshal or silently
// yields zero against the other, and a silent zero in a price is the worst
// failure mode this integration has. flexInt64 accepts both.
//
// What was deliberately NOT salvaged: the previous version parsed the fraction
// into a float64 and inverted prices with 1.0 / raw. domain.Price.Invert()
// already inverts exactly by swapping the numerator and denominator, with no
// division and no precision loss, so the float version was both a rule
// violation and worse arithmetic.
package horizon

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Keel-Official/keel-backend/internal/domain"
)

// flexInt64 unmarshals from a JSON number OR a JSON string.
//
// It is deliberately unexported. It is an implementation detail of the response
// structs in this package, not something a caller should ever hold.
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

// priceFraction is Horizon's n/d rational, whatever its original JSON type.
type priceFraction struct {
	N flexInt64 `json:"n"`
	D flexInt64 `json:"d"`
}

// price converts the fraction into a domain.Price without dividing.
//
// It returns an error rather than a zero value on bad input, because a zero
// price that flows onward is indistinguishable from a genuinely worthless asset,
// and telling those two apart is the entire point of this product.
func (p priceFraction) price() (domain.Price, error) {
	if p.D == 0 {
		return domain.Price{}, fmt.Errorf("price fraction has a zero denominator: n=%d", p.N)
	}
	if p.N <= 0 {
		return domain.Price{}, fmt.Errorf("price fraction has a non-positive numerator: n=%d d=%d", p.N, p.D)
	}
	return domain.Price{N: int64(p.N), D: int64(p.D)}, nil
}

// orient returns the price in quote-per-base direction for the pair that was
// REQUESTED.
//
// On /trades the meaning of price depends on which asset Horizon chose as the
// base. For the USTRY/USDC exploit trade the base was USDC and the counter was
// USTRY, so the fraction read as USTRY per USDC, the inverse of what a reader
// expects. Passing recordBaseIsQuote=true flips it.
//
// The flip is exact: it swaps the numerator and the denominator rather than
// computing a reciprocal.
func orient(p domain.Price, recordBaseIsQuote bool) domain.Price {
	if recordBaseIsQuote {
		return p.Invert()
	}
	return p
}
