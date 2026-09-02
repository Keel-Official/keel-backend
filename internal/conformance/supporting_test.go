// Conformance for the supporting metrics, FR-8 through FR-10.
//
// It asks one question of each figure: does internal/domain/supporting.go
// reproduce the number Al computed by hand in 07-supporting-metrics.md, from the
// same raw evidence he computed it from.
//
// A DISAGREEMENT HERE IS A FINDING AND NOT A TEST FAILURE TO BE TUNED AWAY. The
// numbers in supporting.go's Expected block are RED. When a line below reports a
// mismatch, the honest responses are to fix the code or to report that the
// methodology and the implementation read a definition differently. Editing the
// expected value is neither of them, and internal/conformance/fixture.go has said
// so in its own header since before this file existed.

package conformance

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/Keel-Official/keel-backend/internal/domain"
)

// close reports whether two decimals agree within Tolerance.
func close(a, b decimal.Decimal) bool { return a.Sub(b).Abs().LessThanOrEqual(Tolerance) }

// report prints one comparison line in the shape golden_test.go uses, so the two
// read the same way when they are run together.
func report(t *testing.T, label string, ok bool, got, want string) {
	t.Helper()
	if ok {
		t.Logf("MATCH  %-34s computed %-22s fixture %s", label, got, want)
		return
	}
	t.Errorf("DIFFER %-34s computed %-22s fixture %s", label, got, want)
}

func reportInt(t *testing.T, label string, got, want int) {
	t.Helper()
	report(t, label, got == want, itoa(got), itoa(want))
}

func reportDec(t *testing.T, label string, got, want decimal.Decimal) {
	t.Helper()
	report(t, label, close(got, want), got.String(), want.String())
}

func itoa(i int) string { return decimal.NewFromInt(int64(i)).String() }

// runGenuine loads a period's CSV and classifies it. Failing to load is fatal
// rather than a skip: the CSV is tracked, so a clone that cannot read it has a
// broken checkout and pretending otherwise would report a green run over nothing.
func runGenuine(t *testing.T, path string) (domain.GenuineSummary, []domain.TradeClassification) {
	t.Helper()
	trades, err := LoadTradesCSV(path)
	if err != nil {
		t.Fatalf("load %s: %v", path, err)
	}
	cs := domain.ClassifyTrades(trades, AssetUSTRY, domain.DefaultGenuineRules())
	return domain.SummariseGenuine(cs), cs
}

// TestGenuineTradeRule is section 1's Outputs table, both periods.
func TestGenuineTradeRule(t *testing.T) {
	for _, tc := range []struct {
		path string
		want ExpectedGenuineRun
	}{
		{TradesCSVFebruary, ExpectedFebruary2026},
		{TradesCSVAugust, ExpectedAugust2026},
	} {
		t.Run(tc.want.Period, func(t *testing.T) {
			got, _ := runGenuine(t, tc.path)

			reportInt(t, "total trades", got.Total, tc.want.TotalTrades)
			reportDec(t, "total volume, USDC", got.TotalQuoteVol, tc.want.TotalQuoteVol)

			reportInt(t, "genuine trades", got.GenuineTrades, tc.want.GenuineTrades)
			reportDec(t, "genuine volume, USDC", got.GenuineQuoteVol, tc.want.GenuineQuoteVol)
			if !tc.want.GenuineBaseVol.IsZero() {
				reportDec(t, "genuine volume, USTRY", got.GenuineBaseVol, tc.want.GenuineBaseVol)
			}

			reportInt(t, "unevaluated trades", got.UnevaluatedTrades, tc.want.UnevaluatedTrades)
			reportDec(t, "unevaluated volume, USDC", got.UnevaluatedQuoteVol, tc.want.UnevaluatedQuoteVol)

			for _, ct := range got.ByCondition {
				w, ok := tc.want.PerCondition[ct.Condition]
				if !ok {
					t.Errorf("no expected value transcribed for condition %d, %s", ct.Condition, ct.Condition)
					continue
				}
				reportInt(t, "cond "+itoa(int(ct.Condition))+" "+ct.Condition.String(), ct.Trades, w.Trades)
				reportDec(t, "cond "+itoa(int(ct.Condition))+" volume", ct.QuoteVol, w.QuoteVol)
			}

			// The three states must account for every trade. This is arithmetic
			// on the output rather than a figure from the methodology, and it is
			// the one line here that would catch a classifier dropping a trade
			// silently.
			sum := got.GenuineTrades + got.ExcludedTrades + got.UnevaluatedTrades
			reportInt(t, "states sum to total", sum, got.Total)
		})
	}
}

// TestHolderConcentration is section 2's Result block over the 31 August pull.
func TestHolderConcentration(t *testing.T) {
	holders, err := LoadHoldersCSV(HoldersCSVAugust)
	if err != nil {
		t.Fatalf("load holders: %v", err)
	}
	reportInt(t, "trustlines in the pull", len(holders), ExpectedHolderPull.Trustlines)

	stats, err := domain.HolderConcentration(holders, USTRYExclusions, false)
	if err != nil {
		t.Fatalf("holder concentration: %v", err)
	}

	reportInt(t, "zero balance dropped", stats.ZeroBalanceDropped, ExpectedHolderPull.ZeroBalanceDropped)
	reportInt(t, "population", stats.Population, ExpectedHolderPull.Population)
	reportDec(t, "circulating supply, USTRY", stats.CirculatingSupply, ExpectedHolderPull.CirculatingSupply)

	// Four decimals, because that is the precision section 2 states its three
	// measures to. Comparing at full precision would fail on a figure the
	// methodology never claimed.
	reportDec(t, "top 1 percent", stats.Top1Pct.Round(4), ExpectedHolderPull.Top1Pct)
	reportDec(t, "top 10 percent", stats.Top10Pct.Round(4), ExpectedHolderPull.Top10Pct)
	reportDec(t, "HHI", stats.HHI.Round(4), ExpectedHolderPull.HHI)

	// The issuer and the two pool addresses subtract nothing, and section 2 says
	// so: none of the three appears in the set, so the only removal that fires is
	// the zero-balance filter. Asserting the zero is what would catch an exclusion
	// list that started matching something it should not.
	reportInt(t, "excluded addresses dropped", stats.ExcludedDropped, 0)
}

// TestHolderConcentrationUnevaluated covers the two states section 2 and section 3
// require to stay distinguishable from a measured number.
func TestHolderConcentrationUnevaluated(t *testing.T) {
	holders, err := LoadHoldersCSV(HoldersCSVAugust)
	if err != nil {
		t.Fatalf("load holders: %v", err)
	}

	// Truncated: unevaluated, never partial, because /accounts?asset= pages by
	// account ID and a partial page is an alphabetical slice rather than the
	// largest holders.
	if _, err := domain.HolderConcentration(holders, USTRYExclusions, true); err == nil {
		t.Error("a truncated pull returned a figure; section 2 requires unevaluated")
	}

	// Empty population: an error and not a zeroed struct, so section 3 can tell a
	// zero denominator from an unavailable one.
	if _, err := domain.HolderConcentration(nil, USTRYExclusions, false); err == nil {
		t.Error("an empty holder set returned a figure; the ratio is undefined, not zero")
	}
}

// TestVolumeToSupply is section 3's whole-month row.
func TestVolumeToSupply(t *testing.T) {
	got, _ := runGenuine(t, TradesCSVAugust)

	ratio, why := domain.VolumeToSupply(
		got.GenuineBaseVol,
		ExpectedHolderPull.CirculatingSupply,
		true, true,
	)
	if ratio == nil {
		t.Fatalf("ratio is unevaluated (%s) with both terms available", why)
	}

	// Nine decimals, the precision section 3's table states.
	reportDec(t, "volume to supply, month", ratio.Round(9), ExpectedVolumeToSupplyMonth)

	// The percentage in the same table is the ratio times 100, and deriving it
	// here rather than transcribing it is what stops the two drifting apart.
	reportDec(t, "volume to supply, percent", ratio.Mul(dec("100")).Round(7), dec("0.0548603"))
}

// TestVolumeToSupplyUnevaluatedCauses checks that section 3's three causes stay
// apart. Collapsing them would tell an operator something is wrong without saying
// what, which section 3 rejects in as many words.
func TestVolumeToSupplyUnevaluatedCauses(t *testing.T) {
	for _, tc := range []struct {
		name        string
		supply      decimal.Decimal
		supplyKnown bool
		covered     bool
		want        domain.VolumeToSupplyUnevaluated
	}{
		{"denominator unavailable", dec("1"), false, true, domain.VolumeToSupplyNoDenominator},
		{"denominator zero", decimal.Zero, true, true, domain.VolumeToSupplyZeroDenominator},
		{"numerator unavailable", dec("1"), true, false, domain.VolumeToSupplyNoNumerator},
		{"all available", dec("1"), true, true, domain.VolumeToSupplyOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ratio, why := domain.VolumeToSupply(decimal.Zero, tc.supply, tc.supplyKnown, tc.covered)
			if why != tc.want {
				t.Errorf("cause %q, want %q", why, tc.want)
			}
			if tc.want == domain.VolumeToSupplyOK {
				// A zero numerator with both terms present is a MEASURED ZERO and
				// not unevaluated. This is the line that separates them.
				if ratio == nil {
					t.Error("a zero numerator returned unevaluated; section 3 calls it a measured zero")
				}
			} else if ratio != nil {
				t.Error("an unevaluated ratio returned a number")
			}
		})
	}
}

// TestOracleWindowVolume is section 4's window-width table over the incident.
func TestOracleWindowVolume(t *testing.T) {
	trades, err := LoadTradesCSV(TradesCSVFebruary)
	if err != nil {
		t.Fatalf("load february: %v", err)
	}
	cs := domain.ClassifyTrades(trades, AssetUSTRY, domain.DefaultGenuineRules())

	for _, w := range ExpectedOracleWindows {
		t.Run(w.Window.String(), func(t *testing.T) {
			_, quote, recorded := domain.GenuineVolumeInWindow(cs, OracleWindowAnchor, w.Window)

			genuine := 0
			for _, c := range cs {
				if domain.InWindow(c.ClosedAt, OracleWindowAnchor, w.Window) &&
					c.State == domain.GenuineStateGenuine {
					genuine++
				}
			}

			reportInt(t, "recorded trades in window", recorded, w.RecordedTrades)
			reportInt(t, "genuine trades in window", genuine, w.GenuineTrades)
			reportDec(t, "genuine volume, USDC", quote, w.GenuineQuoteVol)
		})
	}
}

// TestLastGenuineTradeUnevaluated is the distinction section 1 draws between no
// genuine trade at all and a large but finite gap.
func TestLastGenuineTradeUnevaluated(t *testing.T) {
	anchor := OracleWindowAnchor

	if got := domain.LastGenuineTrade(nil, anchor); got != nil {
		t.Error("an empty history returned a trade reference; it must be unevaluated")
	}

	// One genuine trade a long way back is a MEASURED gap, not unevaluated. It is
	// the signal the metric exists to surface.
	old := anchor.Add(-90 * 24 * time.Hour)
	cs := []domain.TradeClassification{{
		ID: "x", LedgerSeq: 1, ClosedAt: old, State: domain.GenuineStateGenuine,
	}}
	d := domain.TimeSinceLastGenuineTrade(cs, anchor)
	if d == nil {
		t.Fatal("a genuine trade 90 days back returned unevaluated")
	}
	if *d != 90*24*time.Hour {
		t.Errorf("staleness %v, want 2160h", *d)
	}

	// A history of excluded trades only is unevaluated, because there is no
	// timestamp to measure from. It must not read as "traded just now".
	cs = []domain.TradeClassification{{
		ID: "y", LedgerSeq: 2, ClosedAt: anchor, State: domain.GenuineStateExcluded,
		Condition: domain.ConditionDust,
	}}
	if got := domain.TimeSinceLastGenuineTrade(cs, anchor); got != nil {
		t.Error("a history of excluded trades returned a staleness; it must be unevaluated")
	}
}

// TestWindowIsHalfOpen fixes the boundary convention sections 3 and 4 both state.
// Two correct implementations must return the same number for the same ledger,
// and a boundary trade is where they would otherwise differ.
func TestWindowIsHalfOpen(t *testing.T) {
	anchor := OracleWindowAnchor
	w := 15 * time.Minute

	if !domain.InWindow(anchor, anchor, w) {
		t.Error("a trade at the anchor is outside the window; the recent edge is closed")
	}
	if domain.InWindow(anchor.Add(-w), anchor, w) {
		t.Error("a trade exactly one window back is inside; the far edge is open")
	}
	if !domain.InWindow(anchor.Add(-w).Add(time.Nanosecond), anchor, w) {
		t.Error("a trade just inside the far edge is outside")
	}
	if domain.InWindow(anchor.Add(time.Nanosecond), anchor, w) {
		t.Error("a trade after the anchor is inside the window")
	}
}

// TestExcludedVolumeShare is section 1's three percentages. It exists because
// TradesExcludedPct is the figure WASH_TRADE_SUSPECTED reads, and the flag turns on
// whether that percentage is by volume or by count.
//
// By volume August is 1.6971 per cent and the flag is clear against a 50 per cent
// threshold. By COUNT the same month is 74.51 per cent and the flag would fire on a
// market whose excluded volume is trivial. Section 1's Required output settles it in
// words: "1.70 percent of August volume excluded as non-genuine, almost all of it
// dust". This test is what holds the code to that reading.
func TestExcludedVolumeShare(t *testing.T) {
	got, _ := runGenuine(t, TradesCSVAugust)

	pct := got.ExcludedPct()
	if pct == nil {
		t.Fatal("excluded percentage is unevaluated with volume present")
	}
	reportDec(t, "excluded volume, percent", pct.Round(4), dec("1.6971"))

	genuinePct := got.GenuineQuoteVol.DivRound(got.TotalQuoteVol, 28).Mul(dec("100"))
	reportDec(t, "genuine volume, percent", genuinePct.Round(4), dec("98.2941"))

	unevalPct := got.UnevaluatedQuoteVol.DivRound(got.TotalQuoteVol, 28).Mul(dec("100"))
	reportDec(t, "unevaluated volume, percent", unevalPct.Round(4), dec("0.0089"))

	// The three volumes account for the total EXACTLY, with no tolerance. Section
	// 1's three rounded percentages sum to 100.0001, which is rounding and not an
	// error; the underlying volumes are what have to be exact.
	sum := got.GenuineQuoteVol.Add(got.ExcludedQuoteVol).Add(got.UnevaluatedQuoteVol)
	if !sum.Equal(got.TotalQuoteVol) {
		t.Errorf("the three states sum to %s, total is %s", sum, got.TotalQuoteVol)
	}

	// By count, recorded so the reading this flag does NOT take is visible rather
	// than merely absent.
	byCount := decimal.NewFromInt(int64(got.ExcludedTrades)).
		DivRound(decimal.NewFromInt(int64(got.Total)), 28).Mul(dec("100"))
	t.Logf("NOTE   excluded by COUNT is %s per cent, which is the reading WASH_TRADE_SUSPECTED must not take",
		byCount.Round(2))
}
