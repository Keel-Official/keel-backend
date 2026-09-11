// What `scan` does with a cached holder reading, and in particular the four ways
// it refuses to attach one.
//
// Every refusal here produces the SAME observable result: the two holder flags
// stay unevaluated. That is the point. A holder figure that is absent, truncated,
// stale or never pulled are four different causes of one honest answer, and none
// of them may become a zero.

package main

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/Keel-Official/keel-backend/internal/domain"
	"github.com/Keel-Official/keel-backend/internal/store"
)

func pct(s string) *decimal.Decimal {
	d := decimal.RequireFromString(s)
	return &d
}

func freshReading(at time.Time) store.HolderReading {
	return store.HolderReading{
		Asset:               hUSTRY,
		FetchedAt:           at,
		SnapshotLedger:      64381285,
		SnapshotBasis:       store.BasisFirstAndLastPage,
		SnapshotLabel:       store.LabelUnknown,
		HoldersRead:         875,
		HolderCountReported: 875,
		Top1Pct:             pct("91.7421"),
		Top10Pct:            pct("99.9475"),
		HHI:                 pct("8410.8452"),
		MethodologyVersion:  domain.MethodologyVersion,
	}
}

func TestSupportingFromReadingAttachesAFreshCompleteReading(t *testing.T) {
	now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	r := freshReading(now.Add(-3 * time.Hour))

	sup, why := supportingFromReading(r, now, 48*time.Hour)
	if sup == nil {
		t.Fatalf("a three hour old complete reading was refused: %s", why)
	}
	if why != "" {
		t.Errorf("a reason was given for an attached reading: %q", why)
	}
	if sup.HolderTop1Pct == nil || !sup.HolderTop1Pct.Equal(*r.Top1Pct) {
		t.Errorf("top 1%% came through as %v, want %v", sup.HolderTop1Pct, r.Top1Pct)
	}
	if sup.HolderHHI == nil || !sup.HolderHHI.Equal(*r.HHI) {
		t.Errorf("HHI came through as %v, want %v", sup.HolderHHI, r.HHI)
	}

	// THE TRADE HALF IS DEFERRED AND MUST STAY ABSENT. If any of these ever
	// becomes non-nil without the trade half being built, three flags would flip
	// from unevaluated to evaluated on data nobody gathered.
	if sup.TradesExcludedPct != nil || sup.LastGenuineTrade != nil ||
		sup.VolumeToSupplyD1 != nil || sup.VolumeToSupplyD7 != nil || sup.VolumeToSupplyD30 != nil {
		t.Error("the trade half is populated and it is deferred; those flags must stay unevaluated")
	}
}

func TestSupportingFromReadingRefusesAStaleReading(t *testing.T) {
	now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	r := freshReading(now.Add(-72 * time.Hour))

	sup, why := supportingFromReading(r, now, 48*time.Hour)
	if sup != nil {
		t.Fatal("a 72 hour old reading was attached under a 48 hour bound")
	}
	if why == "" {
		t.Error("no reason was recorded; the run notes are the only place the gap is written down")
	}
}

// The bound is a mechanism here and a number in DEC-018, which is a draft. Zero
// turns it off, which is the other alternative that record weighs.
func TestSupportingFromReadingHonoursADisabledBound(t *testing.T) {
	now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	r := freshReading(now.Add(-30 * 24 * time.Hour))

	if sup, why := supportingFromReading(r, now, 48*time.Hour); sup != nil {
		t.Fatal("a month old reading passed the default bound")
	} else if why == "" {
		t.Error("no reason recorded")
	}
	if sup, _ := supportingFromReading(r, now, 0); sup == nil {
		t.Error("-max-holder-age=0 did not disable the bound")
	}
}

// Exactly on the bound is not past it. A reading that is 48h old under a 48h
// bound is the boundary case, and an off-by-one here silently drops a day of
// readings or silently keeps one.
func TestSupportingFromReadingTreatsTheBoundAsInclusive(t *testing.T) {
	now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)

	exactly := freshReading(now.Add(-48 * time.Hour))
	if sup, why := supportingFromReading(exactly, now, 48*time.Hour); sup == nil {
		t.Errorf("a reading exactly at the bound was refused: %s", why)
	}
	justPast := freshReading(now.Add(-48*time.Hour - time.Second))
	if sup, _ := supportingFromReading(justPast, now, 48*time.Hour); sup != nil {
		t.Error("a reading one second past the bound was attached")
	}
}

// The refusal that started in domain.ErrHolderSetTruncated, arriving one step
// later. Never worked around, at any layer.
func TestSupportingFromReadingRefusesATruncatedReading(t *testing.T) {
	now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	r := freshReading(now.Add(-1 * time.Hour))
	r.Truncated = true
	r.Top1Pct, r.Top10Pct, r.HHI = nil, nil, nil

	sup, why := supportingFromReading(r, now, 48*time.Hour)
	if sup != nil {
		t.Fatal("a truncated reading was attached; a truncated set answers the question not at all")
	}
	if why == "" {
		t.Error("no reason recorded")
	}
}

// A complete pull whose population was empty after exclusions carries no figures
// either, and it is a different cause from truncation. Both refuse.
func TestSupportingFromReadingRefusesACompleteReadingWithNoFigures(t *testing.T) {
	now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	r := freshReading(now.Add(-1 * time.Hour))
	r.Top1Pct, r.Top10Pct, r.HHI = nil, nil, nil

	sup, why := supportingFromReading(r, now, 48*time.Hour)
	if sup != nil {
		t.Fatal("a reading with no figures was attached")
	}
	if why == "" || why == "holder set was truncated, so concentration is unevaluated" {
		t.Errorf("the reason does not distinguish an empty population from truncation: %q", why)
	}
}

// The whole chain, end to end through the domain: an attached reading closes the
// two holder flags and nothing else, and the band confidence stays partial
// because MANIPULATION_RATIO_LOW is HIGH tier and A7 has not landed.
func TestAttachedHolderFiguresCloseExactlyTheTwoHolderFlags(t *testing.T) {
	now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	sup, _ := supportingFromReading(freshReading(now.Add(-1*time.Hour)), now, 48*time.Hour)
	if sup == nil {
		t.Fatal("setup: the reading was refused")
	}

	unevaluated := map[domain.Flag]bool{}
	for _, f := range unevaluatedWith(t, sup) {
		unevaluated[f] = true
	}

	for _, f := range []domain.Flag{
		domain.FlagHolderConcentrationExtreme,
		domain.FlagHolderConcentrationHigh,
	} {
		if unevaluated[f] {
			t.Errorf("%s is still unevaluated with holder figures attached", f)
		}
	}
	// Deferred, and they must still read unevaluated rather than clear.
	for _, f := range []domain.Flag{
		domain.FlagNoGenuineTrade30D,
		domain.FlagNoGenuineTrade7D,
		domain.FlagWashTradeSuspected,
		domain.FlagManipulationRatioLow,
	} {
		if !unevaluated[f] {
			t.Errorf("%s is evaluated and nothing computed it", f)
		}
	}
}

// unevaluatedWith runs one asset through the real computation with sup attached
// and returns the flags that came back unevaluated.
func unevaluatedWith(t *testing.T, sup *domain.SupportingMetrics) []domain.Flag {
	t.Helper()
	snap := domain.Snapshot{
		Base:           hUSTRY,
		Quote:          hUSDC,
		LedgerSeq:      64381285,
		LedgerClosedAt: time.Date(2026, 9, 12, 11, 59, 0, 0, time.UTC),
		Source:         domain.DataSourceHorizon,
	}
	risk, err := domain.ComputeAssetRiskWith(snap, domain.DefaultParams(), sup)
	if err != nil {
		t.Fatalf("ComputeAssetRiskWith: %v", err)
	}
	return risk.UnevaluatedFlags
}
