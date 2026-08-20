package horizon

import (
	"encoding/json"
	"testing"

	"github.com/Keel-Official/keel/internal/domain"
)

// The two JSON shapes Horizon really sends, taken from the endpoints named in
// this package's doc comment rather than invented.
func TestPriceFractionAcceptsBothJSONShapes(t *testing.T) {
	cases := []struct {
		name string
		body string
		want domain.Price
	}{
		{
			// /offers, the manipulation offer of 21 February 2026.
			name: "numbers, as sent by /offers",
			body: `{"n": 266843207, "d": 2500000}`,
			want: domain.Price{N: 266843207, D: 2500000},
		},
		{
			// /trades, the same rational, inverted and quoted as strings.
			name: "strings, as sent by /trades",
			body: `{"n": "2500000", "d": "266843207"}`,
			want: domain.Price{N: 2500000, D: 266843207},
		},
		{
			name: "the bid that shaped P0 without carrying liquidity",
			body: `{"n": 1057, "d": 1000}`,
			want: domain.Price{N: 1057, D: 1000},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var f priceFraction
			if err := json.Unmarshal([]byte(c.body), &f); err != nil {
				t.Fatalf("unmarshal %s: %v", c.body, err)
			}
			got, err := f.price()
			if err != nil {
				t.Fatalf("price(): %v", err)
			}
			if got != c.want {
				t.Errorf("price = %v, want %v", got, c.want)
			}
		})
	}
}

// A bad fraction has to fail loudly. A zero price flowing onward is
// indistinguishable from a genuinely worthless asset.
func TestPriceFractionRejectsBadInput(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"zero denominator", `{"n": 1, "d": 0}`},
		{"zero numerator", `{"n": 0, "d": 100}`},
		{"negative numerator", `{"n": -5, "d": 100}`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var f priceFraction
			if err := json.Unmarshal([]byte(c.body), &f); err != nil {
				t.Fatalf("unmarshal %s: %v", c.body, err)
			}
			if _, err := f.price(); err == nil {
				t.Errorf("price() returned no error for %s", c.body)
			}
		})
	}
}

func TestFlexInt64RejectsNonInteger(t *testing.T) {
	var f flexInt64
	if err := f.UnmarshalJSON([]byte(`"1.5"`)); err == nil {
		t.Error("UnmarshalJSON accepted \"1.5\"; a fractional value here means the caller read the wrong field")
	}
}

// orient must flip exactly, by swapping n and d, never by computing 1/x.
func TestOrientFlipsExactly(t *testing.T) {
	p := domain.Price{N: 266843207, D: 2500000}

	if got := orient(p, false); got != p {
		t.Errorf("orient(p, false) = %v, want %v unchanged", got, p)
	}

	flipped := orient(p, true)
	want := domain.Price{N: 2500000, D: 266843207}
	if flipped != want {
		t.Errorf("orient(p, true) = %v, want %v", flipped, want)
	}

	// Flipping twice returns the original exactly. A float reciprocal would not
	// guarantee this.
	if back := orient(flipped, true); back != p {
		t.Errorf("orient twice = %v, want %v", back, p)
	}
}
