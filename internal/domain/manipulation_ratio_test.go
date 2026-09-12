package domain

import (
	"testing"

	"github.com/shopspring/decimal"
)

// MANIPULATION_RATIO_LOW, transcribed from DEC-017 section 1.
//
// THESE ARE TRANSCRIPTION TESTS AND NOT LAYER 1 EVIDENCE, and the difference is
// worth stating where a reader will see it. `testdata/fixtures/` carries this
// flag unevaluated, because the golden fixture has no trustline pull behind it
// and therefore no circulating supply, so there is no hand computed value for
// this rule to be judged against. Every expectation below is derived from the
// record's own arithmetic rather than from an independent oracle, which makes
// them a guard against the code drifting from the record and not a proof that
// the record is right.
//
// The magnitudes are the ones the rule will actually meet. USTRY's circulating
// supply at the 31 August 2026 pull was in the low tens of millions and a book
// manipulation cost is in the tens of USDC, so the ratios in play are small
// fractions of a per cent and the 0.1 threshold sits among them.

func ratioInput(supply, p0 string, ladder []ManipulationPoint) flagInput {
	sup := dec(supply)
	price := dec(p0)
	return flagInput{
		PriceSource:   PriceSourceBook,
		HasLadders:    true,
		OrderbookOnly: ladder,
		P0:            &price,
		Supporting:    &SupportingMetrics{CirculatingSupply: &sup},
	}
}

func rung(delta, cost string, reachable bool) ManipulationPoint {
	return ManipulationPoint{Delta: dec(delta), Cost: dec(cost), Reachable: reachable}
}

func TestManipulationRatioLowFiresBelowTheThreshold(t *testing.T) {
	// supply value = 10,000,000 x 1.05 = 10,500,000 USDC.
	// 5,000 / 10,500,000 x 100 = 0.0476 per cent, under 0.1.
	tr, un := statesOf(t, ratioInput("10000000", "1.05", []ManipulationPoint{
		rung("0.5", "5000", true),
	}))

	if un[FlagManipulationRatioLow] {
		t.Fatal("the flag is unevaluated although supply, P0 and a reachable rung were all given")
	}
	if !tr[FlagManipulationRatioLow] {
		t.Error("0.0476 per cent did not fire against a threshold of 0.1")
	}
}

func TestManipulationRatioLowStaysClearAboveTheThreshold(t *testing.T) {
	// 50,000 / 10,500,000 x 100 = 0.476 per cent, over 0.1.
	tr, un := statesOf(t, ratioInput("10000000", "1.05", []ManipulationPoint{
		rung("0.5", "50000", true),
	}))

	if un[FlagManipulationRatioLow] {
		t.Fatal("the flag is unevaluated although every input was given")
	}
	if tr[FlagManipulationRatioLow] {
		t.Error("0.476 per cent fired against a threshold of 0.1")
	}
}

// THE CASE THAT MATTERS MOST, and the one a careless implementation gets wrong.
// An unreachable rung carries the cost of buying the whole book, which on an
// exhausted book is SMALL. The golden fixture's own asset is exactly this: it
// reaches 130.0627093 USDC at delta 1 and cannot move past that price at any
// cost. Counting it would report an asset that cannot be manipulated at all as
// trivially cheap to manipulate.
func TestManipulationRatioLowIgnoresAnUnreachableRung(t *testing.T) {
	tr, un := statesOf(t, ratioInput("10000000", "1.05", []ManipulationPoint{
		rung("1", "130.0627093", false),
	}))

	if un[FlagManipulationRatioLow] {
		t.Fatal("the flag is unevaluated although every input was given")
	}
	if tr[FlagManipulationRatioLow] {
		t.Error("an UNREACHABLE rung fired the flag; Cost without Reachable is not a cost")
	}
}

// DEC-017 writes `<`. A ratio exactly at the threshold does not fire, so the
// boundary belongs to one rule rather than being shared with whatever reads the
// figure next.
func TestManipulationRatioLowIsStrictlyBelow(t *testing.T) {
	// supply value = 1,000,000 x 1 = 1,000,000. 1,000 is exactly 0.1 per cent.
	tr, _ := statesOf(t, ratioInput("1000000", "1", []ManipulationPoint{
		rung("0.5", "1000", true),
	}))

	if tr[FlagManipulationRatioLow] {
		t.Error("a ratio exactly equal to the threshold fired; the record writes a strict <")
	}
}

// Any rung will do, so a ladder whose cheap rung is not the first still fires.
func TestManipulationRatioLowScansEveryRung(t *testing.T) {
	tr, _ := statesOf(t, ratioInput("10000000", "1.05", []ManipulationPoint{
		rung("0.5", "900000", true),
		rung("1", "800000", true),
		rung("10", "5000", true),
	}))

	if !tr[FlagManipulationRatioLow] {
		t.Error("a cheap rung at the end of the ladder did not fire the flag")
	}
}

// Each missing input is a reason the rule cannot be ANSWERED, never a reason it
// is false, so each one leaves the flag unevaluated rather than clear.
func TestManipulationRatioLowIsUnevaluatedWithoutItsInputs(t *testing.T) {
	price := dec("1.05")
	supply := dec("10000000")
	ladder := []ManipulationPoint{rung("0.5", "5000", true)}

	cases := map[string]flagInput{
		"no supporting metrics at all": {
			PriceSource: PriceSourceBook, HasLadders: true, OrderbookOnly: ladder, P0: &price,
		},
		"a truncated pull, so no supply": {
			PriceSource: PriceSourceBook, HasLadders: true, OrderbookOnly: ladder, P0: &price,
			Supporting: &SupportingMetrics{},
		},
		"no reference price": {
			PriceSource: PriceSourceBook, HasLadders: true, OrderbookOnly: ladder,
			Supporting: &SupportingMetrics{CirculatingSupply: &supply},
		},
		"no ladders were computed": {
			PriceSource: PriceSourceNone, HasLadders: false, P0: &price,
			Supporting: &SupportingMetrics{CirculatingSupply: &supply},
		},
		"a supply of zero, which is not an answer": {
			PriceSource: PriceSourceBook, HasLadders: true, OrderbookOnly: ladder, P0: &price,
			Supporting: &SupportingMetrics{CirculatingSupply: ptrDec(decimal.Zero)},
		},
	}

	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			tr, un := statesOf(t, in)
			if !un[FlagManipulationRatioLow] {
				t.Errorf("the flag is not unevaluated; triggered = %v", tr[FlagManipulationRatioLow])
			}
		})
	}
}

func ptrDec(d decimal.Decimal) *decimal.Decimal { return &d }
