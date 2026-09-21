package domain

import (
	"strings"
	"testing"
	"time"
)

// Synthetic edge cases for ComputeOracleResistance and the live classification
// helpers. The incident itself is in internal/conformance/oracle_test.go, judged
// against expected values that predate the code; these are the states the incident
// happens not to contain.

func ladder(points ...ManipulationPoint) []ManipulationPoint { return points }

func TestOracleResistanceKeepsAMeasuredZeroVolume(t *testing.T) {
	// No genuine trade inside the window is 06's most important finding, not a
	// missing value. The object comes back; only the ratio is undefined.
	p := DefaultParams()
	zero := dec("0")
	o, why := ComputeOracleResistance(ladder(ManipulationPoint{Delta: dec("0.5"), Cost: dec("12.5"), Reachable: true}), &zero, p)
	if o == nil {
		t.Fatalf("declined a measured zero volume: %s", why)
	}
	if o.Ratio != nil {
		t.Errorf("ratio = %s over a zero volume; it must be nil, undefined rather than infinite", o.Ratio)
	}
	if o.TotalAttackCost == nil || !o.TotalAttackCost.Equal(dec("12.5")) {
		t.Errorf("totalAttackCost = %v, want the manipulation cost alone, 12.5", o.TotalAttackCost)
	}
}

func TestOracleResistanceDividesAtThePackagePrecision(t *testing.T) {
	p := DefaultParams()
	v := dec("3")
	o, _ := ComputeOracleResistance(ladder(ManipulationPoint{Delta: dec("0.5"), Cost: dec("1"), Reachable: true}), &v, p)
	if o == nil || o.Ratio == nil {
		t.Fatal("no ratio for a reachable rung over a positive volume")
	}
	if want := dec("1").DivRound(dec("3"), Precision); !o.Ratio.Equal(want) {
		t.Errorf("ratio = %s, want %s", o.Ratio, want)
	}
	if !o.TotalAttackCost.Equal(dec("4")) {
		t.Errorf("totalAttackCost = %s, want 4", o.TotalAttackCost)
	}
}

func TestOracleResistanceNamesWhyItDeclined(t *testing.T) {
	p := DefaultParams()
	v := dec("1")

	o, why := ComputeOracleResistance(ladder(ManipulationPoint{Delta: dec("1"), Cost: dec("1"), Reachable: true}), &v, p)
	if o != nil || !strings.Contains(why, "no rung at the critical delta") {
		t.Errorf("missing rung: got %v, %q", o, why)
	}

	o, why = ComputeOracleResistance(ladder(ManipulationPoint{Delta: dec("0.5"), Cost: dec("1"), Reachable: true}), nil, p)
	if o != nil || !strings.Contains(why, "900 second oracle window was not measured") {
		t.Errorf("no volume: got %v, %q", o, why)
	}
}

func TestOrderBookMedianRefusesTwoDays(t *testing.T) {
	d := time.Date(2026, 2, 21, 23, 59, 0, 0, time.UTC)
	_, _, err := OrderBookMedian([]Trade{
		{ClosedAt: d, Price: Price{N: 1, D: 1}},
		{ClosedAt: d.Add(2 * time.Minute), Price: Price{N: 1, D: 1}},
	})
	if err == nil {
		t.Fatal("a slice spanning two UTC days returned a median")
	}
}

func TestOrderBookMedianIgnoresPoolFills(t *testing.T) {
	d := time.Date(2026, 2, 21, 12, 0, 0, 0, time.UTC)
	m, ok, err := OrderBookMedian([]Trade{
		{ClosedAt: d, Price: Price{N: 2, D: 1}},
		{ClosedAt: d, Price: Price{N: 100, D: 1}, LiquidityPoolID: "pool"},
	})
	if err != nil || !ok || !m.Equal(dec("2")) {
		t.Errorf("median = %s ok=%v err=%v, want 2 from the one order-book trade", m, ok, err)
	}
	if _, ok, _ := OrderBookMedian([]Trade{{ClosedAt: d, Price: Price{N: 1, D: 1}, LiquidityPoolID: "pool"}}); ok {
		t.Error("a day of pool fills only returned a median")
	}
}

func TestClassifyAgainstMedianJudgesEveryTradeByTheReference(t *testing.T) {
	anchor := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	base := Asset{Code: "X", Issuer: "GISSUER", Type: AssetTypeAlphanum4}
	mk := func(id string, price int64) Trade {
		return Trade{
			ID: id, LedgerSeq: 1, ClosedAt: anchor, Type: "orderbook", Price: Price{N: price, D: 100},
			BaseAmount: dec("5"), CounterAmount: dec("5"), BaseAccount: "GA", CounterAccount: "GB",
		}
	}
	// Today's own median would be 400 and pass the 400 trade. Against yesterday's
	// median of 1.00 it is an outlier, which is the point of the reference.
	in := []Trade{mk("a", 400), mk("b", 400), mk("c", 101)}
	ref := dec("1")
	cs := ClassifyTradesAgainstMedian(in, base, DefaultGenuineRules(), &ref)
	if cs[0].Condition != ConditionPriceOutlier || cs[1].Condition != ConditionPriceOutlier {
		t.Errorf("400 against a reference of 1.00: %v, %v, want price outlier", cs[0].Condition, cs[1].Condition)
	}
	if cs[2].State != GenuineStateGenuine {
		t.Errorf("1.01 against a reference of 1.00 resolved as %s", cs[2].State)
	}

	// No reference median passes condition 5, ClassifyTrades' rule for a day
	// without one.
	for _, c := range ClassifyTradesAgainstMedian(in, base, DefaultGenuineRules(), nil) {
		if c.Condition == ConditionPriceOutlier {
			t.Errorf("trade %s excluded as an outlier with no reference median", c.ID)
		}
	}
}
