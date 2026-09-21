package conformance

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/Keel-Official/keel-backend/internal/domain"
)

// Oracle resistance over the incident, 06-oracle-resilience.md section 1.
//
// THE ORDERING RULE, NAMED BEFORE ANY ASSERTION. Both inputs to MR are expected
// values that existed before ComputeOracleResistance did:
//
//   - MC_orderbookOnly at delta 0.5 is ExpectedManipulation's first row, cost 0 and
//     reachable, transcribed from testdata/fixtures/ustry_pre_exploit.md;
//   - V_genuine over the 15 minute window ending at 61340263 is
//     ExpectedOracleWindows' first row, 0.3268461 USDC, transcribed from
//     07-supporting-metrics.md section 4's Result.
//
// The outputs below are those two numbers added and divided, and nothing else. No
// figure in this file was produced by running the code under test.

// oracleVolume is section 4's 15 minute row, the window Params.OracleWindow names.
func oracleVolume(t *testing.T) decimal.Decimal {
	t.Helper()
	for _, w := range ExpectedOracleWindows {
		if w.Window == 15*time.Minute {
			return w.GenuineQuoteVol
		}
	}
	t.Fatal("ExpectedOracleWindows has no 15 minute row")
	return decimal.Zero
}

func TestOracleResistanceOnTheIncidentBook(t *testing.T) {
	p := FixtureParams()
	if p.OracleWindow != 15*time.Minute {
		t.Fatalf("Params.OracleWindow = %s; this test's expected V is the 15 minute row", p.OracleWindow)
	}
	v := oracleVolume(t)

	// The fixture's own book, through the engine, so the rung comes from the same
	// ladder the collateral ceiling reads rather than from a hand-built slice.
	risk, err := domain.ComputeAssetRiskWith(GoldenSnapshot(), p, &domain.SupportingMetrics{
		GenuineVolumeInWindow: &v,
		OracleWindowAnchor:    &OracleWindowAnchor,
	})
	if err != nil {
		t.Fatalf("ComputeAssetRiskWith: %v", err)
	}
	o := risk.OracleResistance
	if o == nil {
		t.Fatalf("oracleResistance is nil on the incident book with V measured at its own ledger; warnings: %q", risk.Warnings)
	}

	want := ExpectedManipulation[0]
	eqDec(t, "criticalDelta", o.CriticalDelta, want.Delta)
	eqDec(t, "manipulationCost", o.ManipulationCost, want.Cost)
	if o.Reachable != want.Reachable {
		t.Errorf("reachable = %v, want %v", o.Reachable, want.Reachable)
	}
	eqDec(t, "genuineVolume", o.GenuineVolume, v)
	if o.WindowSeconds != 900 {
		t.Errorf("windowSeconds = %d, want 900", o.WindowSeconds)
	}

	// MC is zero and V is positive, so the ratio is a measured zero rather than
	// undefined: moving the price to the critical level cost nothing, against a
	// market of 0.33 USDC. That is 06's claim about 22 February, as a number.
	if o.Ratio == nil {
		t.Fatal("ratio is nil; with V > 0 and the rung reachable it must be defined")
	}
	eqDec(t, "ratio", *o.Ratio, want.Cost.DivRound(v, domain.Precision))
	if o.TotalAttackCost == nil {
		t.Fatal("totalAttackCost is nil on a reachable rung")
	}
	eqDec(t, "totalAttackCost", *o.TotalAttackCost, want.Cost.Add(v))
}

func TestOracleResistanceRefusesAVolumeFromAnotherInstant(t *testing.T) {
	// The daily trade cache sums the window back from midnight. A scan at 00:10
	// holding that figure must not pair it with the book at 00:10.
	v := oracleVolume(t)
	midnight := time.Date(2026, 2, 22, 0, 0, 0, 0, time.UTC)

	risk, err := domain.ComputeAssetRiskWith(GoldenSnapshot(), FixtureParams(), &domain.SupportingMetrics{
		GenuineVolumeInWindow: &v,
		OracleWindowAnchor:    &midnight,
	})
	if err != nil {
		t.Fatalf("ComputeAssetRiskWith: %v", err)
	}
	if risk.OracleResistance != nil {
		t.Fatal("oracleResistance was assembled from a volume anchored at another instant")
	}
	if !containsSubstring(risk.Warnings, "not measured at this ledger") {
		t.Errorf("no warning says why oracleResistance is null: %q", risk.Warnings)
	}
}

func TestOracleResistanceOnAnUnreachableRung(t *testing.T) {
	// Delta 1 on the same book: cost 130.06, unreachable. The ratio and the sum
	// are the cost of reaching a target nobody can reach, so both are nil.
	p := FixtureParams()
	p.ManipulationCriticalDelta = ExpectedManipulation[1].Delta
	v := oracleVolume(t)

	risk, err := domain.ComputeAssetRiskWith(BookOnlySnapshot(), p, &domain.SupportingMetrics{
		GenuineVolumeInWindow: &v,
		OracleWindowAnchor:    &OracleWindowAnchor,
	})
	if err != nil {
		t.Fatalf("ComputeAssetRiskWith: %v", err)
	}
	o := risk.OracleResistance
	if o == nil {
		t.Fatalf("oracleResistance is nil; an unreachable rung is still an answer. warnings: %q", risk.Warnings)
	}
	if o.Reachable {
		t.Error("reachable = true at delta 1 on the book-only snapshot")
	}
	eqDec(t, "manipulationCost", o.ManipulationCost, ExpectedManipulation[1].Cost)
	if o.Ratio != nil || o.TotalAttackCost != nil {
		t.Errorf("ratio %v, totalAttackCost %v; both must be nil on an unreachable rung", o.Ratio, o.TotalAttackCost)
	}
}

func TestLiveReadingOfTheOracleWindowAgreesWithSectionFour(t *testing.T) {
	// The scan cannot use the incident day's own median, because at a live ledger
	// that day has not ended. It classifies against the last COMPLETE day's median
	// instead. On the incident this must reproduce section 4's figure, or the live
	// reading and the methodology disagree about the one case they both describe.
	trades, err := LoadTradesCSV(TradesCSVFebruary)
	if err != nil {
		t.Fatalf("load february: %v", err)
	}
	var prevDay, window []domain.Trade
	dayStart := time.Date(2026, 2, 21, 0, 0, 0, 0, time.UTC)
	since := OracleWindowAnchor.Add(-FixtureParams().OracleWindow - domain.DefaultGenuineRules().BookComparisonWindow)
	for _, tr := range trades {
		if !tr.ClosedAt.Before(dayStart) && tr.ClosedAt.Before(dayStart.AddDate(0, 0, 1)) {
			prevDay = append(prevDay, tr)
		}
		if !tr.ClosedAt.Before(since) && !tr.ClosedAt.After(OracleWindowAnchor) {
			window = append(window, tr)
		}
	}

	med, ok, err := domain.OrderBookMedian(prevDay)
	if err != nil || !ok {
		t.Fatalf("21 February order-book median: ok=%v err=%v", ok, err)
	}
	cs := domain.ClassifyTradesAgainstMedian(window, AssetUSTRY, domain.DefaultGenuineRules(), &med)
	_, quote, _ := domain.GenuineVolumeInWindow(cs, OracleWindowAnchor, FixtureParams().OracleWindow)
	reportDec(t, "genuine volume, USDC, live reading", quote, oracleVolume(t))

	// And the manipulative trade is still the one removed, by condition 5.
	for _, c := range cs {
		if c.LedgerSeq == 61340263 && c.Condition != domain.ConditionPriceOutlier {
			t.Errorf("the 61340263 trade resolved as %s/%s, want excluded by price outlier", c.State, c.Condition)
		}
	}
}

func containsSubstring(ss []string, sub string) bool {
	for _, s := range ss {
		if len(sub) <= len(s) {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}
