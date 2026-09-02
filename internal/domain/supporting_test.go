package domain

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

// The five flags that FR-8 and FR-10 answer, plus the one that stays unevaluated.
//
// The figures used below are the measured ones from
// docs/methodology/07-supporting-metrics.md so that the thresholds are exercised
// against real magnitudes rather than round numbers: a top-1 share of 91.5406
// against a 50 threshold, a top-10 of 99.9475 against 80, and an excluded volume
// share of 1.6971 against 50.

func statesOf(t *testing.T, in flagInput) (triggered, unevaluated map[Flag]bool) {
	t.Helper()
	tr, un, _, _ := evaluateFlags(in, DefaultParams())
	triggered, unevaluated = map[Flag]bool{}, map[Flag]bool{}
	for _, f := range tr {
		triggered[f] = true
	}
	for _, f := range un {
		unevaluated[f] = true
	}
	return triggered, unevaluated
}

// TestFlagsWithoutSupportingAreUnchanged is the regression guard on the whole
// change. A caller with only a Snapshot must get exactly what it got before the
// supporting metrics existed, because a silent zero where there was an honest
// unevaluated is the one failure 09-flags-and-bands.md section 2 exists to prevent.
func TestFlagsWithoutSupportingAreUnchanged(t *testing.T) {
	_, un := statesOf(t, flagInput{PriceSource: PriceSourceBook, HasLadders: true})

	for _, f := range []Flag{
		FlagManipulationRatioLow,
		FlagNoGenuineTrade30D,
		FlagNoGenuineTrade7D,
		FlagHolderConcentrationExtreme,
		FlagHolderConcentrationHigh,
		FlagWashTradeSuspected,
	} {
		if !un[f] {
			t.Errorf("%s is not unevaluated with no supporting metrics supplied", f)
		}
	}
}

// TestHolderFlagsAgainstMeasuredShares uses the 31 August USTRY figures.
func TestHolderFlagsAgainstMeasuredShares(t *testing.T) {
	top1, top10 := dec("91.5406"), dec("99.9475")
	tr, un := statesOf(t, flagInput{
		PriceSource: PriceSourceBook,
		HasLadders:  true,
		Supporting:  &SupportingMetrics{HolderTop1Pct: &top1, HolderTop10Pct: &top10},
	})

	if !tr[FlagHolderConcentrationExtreme] {
		t.Error("top 1 of 91.5406 did not trigger HOLDER_CONCENTRATION_EXTREME against a threshold of 50")
	}
	if !tr[FlagHolderConcentrationHigh] {
		t.Error("top 10 of 99.9475 did not trigger HOLDER_CONCENTRATION_HIGH against a threshold of 80")
	}
	if un[FlagHolderConcentrationExtreme] || un[FlagHolderConcentrationHigh] {
		t.Error("a holder flag is both triggered and unevaluated")
	}

	// A distributed asset: both CLEAR, which means present in neither slice. That
	// is what the contract says and what internal/conformance asserts.
	low1, low10 := dec("4.0"), dec("30.0")
	tr, un = statesOf(t, flagInput{
		PriceSource: PriceSourceBook,
		HasLadders:  true,
		Supporting:  &SupportingMetrics{HolderTop1Pct: &low1, HolderTop10Pct: &low10},
	})
	for _, f := range []Flag{FlagHolderConcentrationExtreme, FlagHolderConcentrationHigh} {
		if tr[f] || un[f] {
			t.Errorf("%s should be clear, so absent from both slices", f)
		}
	}
}

// TestHolderFlagsStayUnevaluatedWhenPartial is the case section 2 cares about most:
// a pull that could not be paged is UNEVALUATED and never a number, because
// /accounts?asset= pages by account ID and a partial page is an alphabetical slice
// rather than the largest holders.
func TestHolderFlagsStayUnevaluatedWhenPartial(t *testing.T) {
	// Supporting is present but its holder fields are nil, which is what
	// ComputeSupporting produces from a truncated pull.
	_, un := statesOf(t, flagInput{
		PriceSource: PriceSourceBook,
		HasLadders:  true,
		Supporting:  &SupportingMetrics{},
	})
	for _, f := range []Flag{FlagHolderConcentrationExtreme, FlagHolderConcentrationHigh} {
		if !un[f] {
			t.Errorf("%s is not unevaluated when the holder figures are absent", f)
		}
	}
}

// TestWashTradeFlagReadsVolumeNotCount pins the reading that matters. August's
// excluded share is 1.6971 per cent by volume and 74.51 per cent by count, and the
// threshold is 50, so the two readings give opposite answers on real data.
func TestWashTradeFlagReadsVolumeNotCount(t *testing.T) {
	byVolume := dec("1.6971")
	tr, un := statesOf(t, flagInput{
		PriceSource: PriceSourceBook,
		HasLadders:  true,
		Supporting:  &SupportingMetrics{TradesExcludedPct: &byVolume},
	})
	if tr[FlagWashTradeSuspected] {
		t.Error("WASH_TRADE_SUSPECTED fired on 1.6971 per cent of volume against a threshold of 50")
	}
	if un[FlagWashTradeSuspected] {
		t.Error("WASH_TRADE_SUSPECTED is unevaluated although the excluded share was supplied")
	}

	byCount := dec("74.51")
	tr, _ = statesOf(t, flagInput{
		PriceSource: PriceSourceBook,
		HasLadders:  true,
		Supporting:  &SupportingMetrics{TradesExcludedPct: &byCount},
	})
	if !tr[FlagWashTradeSuspected] {
		t.Error("WASH_TRADE_SUSPECTED did not fire at 74.51 per cent, so the threshold is not being read")
	}
}

// TestStalenessFlags walks the two thresholds, 30 days and 7 days.
func TestStalenessFlags(t *testing.T) {
	anchor := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

	for _, tc := range []struct {
		name          string
		ago           time.Duration
		want30, want7 bool
	}{
		{"traded yesterday", 24 * time.Hour, false, false},
		{"traded 10 days ago", 10 * 24 * time.Hour, false, true},
		{"traded 40 days ago", 40 * 24 * time.Hour, true, true},
		// Exactly on a threshold is CLEAR, because the rule is "no genuine trade
		// WITHIN N days" and a trade at exactly N days is within it. A boundary
		// has to fall one way and this is the way, written down so two readings
		// cannot disagree.
		{"traded exactly 7 days ago", 7 * 24 * time.Hour, false, false},
		{"traded exactly 30 days ago", 30 * 24 * time.Hour, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ref := TradeRef{LedgerSeq: 1, At: anchor.Add(-tc.ago)}
			tr, un := statesOf(t, flagInput{
				PriceSource: PriceSourceBook,
				HasLadders:  true,
				Anchor:      anchor,
				Supporting:  &SupportingMetrics{LastGenuineTrade: &ref},
			})
			if tr[FlagNoGenuineTrade30D] != tc.want30 {
				t.Errorf("NO_GENUINE_TRADE_30D = %v, want %v", tr[FlagNoGenuineTrade30D], tc.want30)
			}
			if tr[FlagNoGenuineTrade7D] != tc.want7 {
				t.Errorf("NO_GENUINE_TRADE_7D = %v, want %v", tr[FlagNoGenuineTrade7D], tc.want7)
			}
			if un[FlagNoGenuineTrade30D] || un[FlagNoGenuineTrade7D] {
				t.Error("a staleness flag is unevaluated although a reference was supplied")
			}
		})
	}
}

// TestStalenessUnevaluatedWithoutReference covers the two ways the pair stays
// unevaluated, and they are different situations that produce the same output.
func TestStalenessUnevaluatedWithoutReference(t *testing.T) {
	anchor := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

	// No genuine trade in the fetched history: there is no timestamp to measure
	// from. This is NOT "traded a long time ago", which is a measured value.
	_, un := statesOf(t, flagInput{
		PriceSource: PriceSourceBook, HasLadders: true, Anchor: anchor,
		Supporting: &SupportingMetrics{},
	})
	for _, f := range []Flag{FlagNoGenuineTrade30D, FlagNoGenuineTrade7D} {
		if !un[f] {
			t.Errorf("%s is not unevaluated with no genuine trade in the history", f)
		}
	}

	// No anchor. Ageing against the zero time would report every asset as stale
	// by two thousand years, which is a wrong answer wearing a confident label.
	ref := TradeRef{LedgerSeq: 1, At: anchor}
	_, un = statesOf(t, flagInput{
		PriceSource: PriceSourceBook, HasLadders: true,
		Supporting: &SupportingMetrics{LastGenuineTrade: &ref},
	})
	for _, f := range []Flag{FlagNoGenuineTrade30D, FlagNoGenuineTrade7D} {
		if !un[f] {
			t.Errorf("%s is not unevaluated with no anchor", f)
		}
	}
}

// TestManipulationRatioLowStaysUnevaluated is a test of a DELIBERATE GAP and it is
// here so the gap cannot close by accident.
//
// 09-flags-and-bands.md section 4 states the rule as
// "Cost(d) / circulating_supply_value < Thresholds.ManipulationRatioLowPct". The
// left side is a bare ratio, the threshold is named Pct and set to 1.0, and those
// two readings differ by a factor of a hundred. There is no hand computed oracle
// for it either. So it stays unevaluated until Al settles the units, and if anyone
// implements it without settling them this test fails and asks why.
func TestManipulationRatioLowStaysUnevaluated(t *testing.T) {
	top1 := dec("91.5406")
	_, un := statesOf(t, flagInput{
		PriceSource: PriceSourceBook,
		HasLadders:  true,
		Anchor:      time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		Supporting:  &SupportingMetrics{HolderTop1Pct: &top1},
	})
	if !un[FlagManipulationRatioLow] {
		t.Error("MANIPULATION_RATIO_LOW is evaluated; its units are ambiguous in the methodology and it must stay unevaluated until that is settled")
	}
}

// TestComputeSupportingUnevaluatedPaths checks the assembly reports the right cause
// rather than a zero, for each way an input can be missing.
func TestComputeSupportingUnevaluatedPaths(t *testing.T) {
	anchor := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	base := Asset{Code: "X", Issuer: "GISSUER", Type: AssetTypeAlphanum4}

	t.Run("no holder pull", func(t *testing.T) {
		out, det := ComputeSupporting(SupportingInput{
			Anchor: anchor, Base: base, Rules: DefaultGenuineRules(),
			OracleWindow: 15 * time.Minute, TradesCover: true,
		})
		if out.HolderTop1Pct != nil || out.HolderHHI != nil {
			t.Error("holder figures were produced with no holder pull")
		}
		if det.VolumeToSupplyD30Why != VolumeToSupplyNoDenominator {
			t.Errorf("cause is %q, want the denominator to be reported unavailable", det.VolumeToSupplyD30Why)
		}
		if out.VolumeToSupplyD30 != nil {
			t.Error("a ratio was produced with no denominator")
		}
	})

	t.Run("holders present, no trade coverage", func(t *testing.T) {
		out, det := ComputeSupporting(SupportingInput{
			Anchor: anchor, Base: base, Rules: DefaultGenuineRules(),
			OracleWindow: 15 * time.Minute,
			HoldersKnown: true,
			Holders:      []HolderBalance{{AccountID: "GA", Balance: dec("100")}},
			TradesCover:  false,
		})
		if out.HolderTop1Pct == nil {
			t.Fatal("holder figures absent although the pull was supplied")
		}
		if !out.HolderTop1Pct.Equal(dec("100")) {
			t.Errorf("a single holder is %s per cent, want 100", out.HolderTop1Pct)
		}
		if det.VolumeToSupplyD1Why != VolumeToSupplyNoNumerator {
			t.Errorf("cause is %q, want the numerator to be reported unavailable", det.VolumeToSupplyD1Why)
		}
		if out.GenuineVolumeInWindow != nil {
			t.Error("oracle window volume was produced although the recording does not cover it")
		}
	})

	t.Run("a measured zero is not unevaluated", func(t *testing.T) {
		// Trades exist, all of them dust, so genuine volume is zero and the ratio
		// is a MEASURED ZERO. Section 3 is explicit that this is not one of its
		// three unevaluated causes.
		out, det := ComputeSupporting(SupportingInput{
			Anchor: anchor, Base: base, Rules: DefaultGenuineRules(),
			OracleWindow: 15 * time.Minute,
			HoldersKnown: true,
			Holders:      []HolderBalance{{AccountID: "GA", Balance: dec("100")}},
			TradesCover:  true,
			Trades: []Trade{{
				ID: "t1", LedgerSeq: 9, ClosedAt: anchor.Add(-time.Minute),
				Type: "orderbook", Price: Price{N: 1, D: 1},
				BaseAmount: dec("0.001"), CounterAmount: dec("0.001"),
				BaseAccount: "GA", CounterAccount: "GB",
			}},
		})
		if det.VolumeToSupplyD1Why != VolumeToSupplyOK {
			t.Errorf("cause is %q, want a measured zero rather than an unevaluated ratio", det.VolumeToSupplyD1Why)
		}
		if out.VolumeToSupplyD1 == nil {
			t.Fatal("a measured zero came back as unevaluated")
		}
		if !out.VolumeToSupplyD1.IsZero() {
			t.Errorf("ratio is %s, want zero", out.VolumeToSupplyD1)
		}
		// The trade was recorded and excluded, so the oracle window is a measured
		// zero with a recorded count of one. That count is what separates a silent
		// asset from one busy with fake prints.
		if out.GenuineVolumeInWindow == nil || !out.GenuineVolumeInWindow.IsZero() {
			t.Error("oracle window volume should be a measured zero")
		}
		if det.OracleWindowRecorded != 1 {
			t.Errorf("recorded trades in window is %d, want 1", det.OracleWindowRecorded)
		}
		if out.LastGenuineTrade != nil {
			t.Error("a history of excluded trades produced a last genuine trade")
		}
	})
}

// TestClassifyIsOrderIndependent is NFR-9 on this file's own surface. The classifier
// is handed the same trades in reverse and must produce the same summary, because
// the input arrives from a CSV, a database or a Horizon page and only one of those
// three promises an order.
func TestClassifyIsOrderIndependent(t *testing.T) {
	anchor := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	base := Asset{Code: "X", Issuer: "GISSUER", Type: AssetTypeAlphanum4}

	mk := func(id string, min int, price int64, amt string, pool string) Trade {
		return Trade{
			ID: id, LedgerSeq: uint32(100 + min), ClosedAt: anchor.Add(-time.Duration(min) * time.Minute),
			Type: "orderbook", Price: Price{N: price, D: 100},
			BaseAmount: dec(amt), CounterAmount: dec(amt),
			BaseAccount: "GA", CounterAccount: "GB",
			LiquidityPoolID: pool, LiquidityPoolSide: map[bool]string{true: "base", false: ""}[pool != ""],
		}
	}
	in := []Trade{
		mk("a", 1, 100, "5", ""),
		mk("b", 2, 101, "5", ""),
		mk("c", 3, 400, "5", ""),
		mk("d", 4, 102, "5", "pool1"),
		mk("e", 5, 99, "5", ""),
	}
	rev := make([]Trade, len(in))
	for i := range in {
		rev[i] = in[len(in)-1-i]
	}

	a := SummariseGenuine(ClassifyTrades(in, base, DefaultGenuineRules()))
	b := SummariseGenuine(ClassifyTrades(rev, base, DefaultGenuineRules()))

	if a.GenuineTrades != b.GenuineTrades || !a.GenuineQuoteVol.Equal(b.GenuineQuoteVol) {
		t.Errorf("reversing the input changed the summary: %d/%s against %d/%s",
			a.GenuineTrades, a.GenuineQuoteVol, b.GenuineTrades, b.GenuineQuoteVol)
	}
	for i := range a.ByCondition {
		if a.ByCondition[i].Trades != b.ByCondition[i].Trades {
			t.Errorf("condition %d differs under reversal: %d against %d",
				a.ByCondition[i].Condition, a.ByCondition[i].Trades, b.ByCondition[i].Trades)
		}
	}
}

// TestMedianTakesTheUpperMiddle pins the convention derived at median's comment. It
// is a unit test of a decision that was established from section 1's counts, and it
// exists so the convention cannot drift back to averaging without a failure.
func TestMedianTakesTheUpperMiddle(t *testing.T) {
	// The February window that settled it: two prices, 32 prints each.
	lower, upper := dec("1.0576268536490003"), dec("1.0585791463489995")
	got := median([]decimal.Decimal{lower, upper})
	if !got.Equal(upper) {
		t.Errorf("median of two is %s, want the upper middle %s", got, upper)
	}
	// The averaging convention would give 1.0581030, which is a price that never
	// traded, and it is what called one February fill dearer that section 1 counts
	// as cheaper.
	avg := lower.Add(upper).Div(dec("2"))
	if got.Equal(avg) {
		t.Error("median returned the average of the two middles")
	}
	// Odd counts are unaffected.
	if !median([]decimal.Decimal{dec("1"), dec("2"), dec("3")}).Equal(dec("2")) {
		t.Error("median of an odd count is not the middle value")
	}
}
