package main

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/Keel-Official/keel-backend/internal/conformance"
	"github.com/Keel-Official/keel-backend/internal/domain"
)

// The oracle window at the incident ledger, against the figure 07 section 4
// states and that Al computed before any of this existed.
//
// THE ORDERING RULE, NAMED. The expected value is ExpectedOracleWindows' 15 minute
// row, 0.3268461 USDC, transcribed into internal/conformance from the methodology.
// It is not produced by this code, and the February trades CSV it is computed over
// is a committed evidence file with its own provenance sidecar.
func TestHistoricalSupportingReproducesSectionFourAtTheIncident(t *testing.T) {
	trades, err := conformance.LoadTradesCSV(conformance.TradesCSVFebruary)
	if err != nil {
		t.Fatalf("load february: %v", err)
	}
	anchor := conformance.OracleWindowAnchor // 2026-02-22T00:10:21Z, ledger 61340263

	var want decimal.Decimal
	for _, w := range conformance.ExpectedOracleWindows {
		if w.Window == 15*time.Minute {
			want = w.GenuineQuoteVol
		}
	}

	sup, notes := historicalSupporting(trades, conformance.AssetUSTRY, anchor, 15*time.Minute, domain.DefaultGenuineRules())
	if sup == nil {
		t.Fatalf("no metrics: %v", notes)
	}
	if sup.GenuineVolumeInWindow == nil || !sup.GenuineVolumeInWindow.Equal(want) {
		t.Errorf("genuine volume in window = %v, want %s", sup.GenuineVolumeInWindow, want)
	}
	// The anchor is what lets the engine pair this volume with THIS ledger's
	// manipulation cost. Without it ComputeAssetRiskFrom refuses the pairing.
	if sup.OracleWindowAnchor == nil || !sup.OracleWindowAnchor.Equal(anchor) {
		t.Errorf("anchor = %v, want %s", sup.OracleWindowAnchor, anchor)
	}
	// The last genuine trade before the incident, from the same classification:
	// 00:06:31Z in ledger 61340224, three minutes and fifty seconds earlier. The
	// manipulative trade in 61340263 is excluded by condition 5 and must not be it.
	if sup.LastGenuineTrade == nil {
		t.Fatal("no last genuine trade")
	}
	if got := sup.LastGenuineTrade.LedgerSeq; got != 61340224 {
		t.Errorf("last genuine trade ledger = %d, want 61340224", got)
	}
	// Volume to supply needs a trustline set, which Horizon answers only for now.
	if sup.VolumeToSupplyD1 != nil || sup.VolumeToSupplyD7 != nil || sup.VolumeToSupplyD30 != nil {
		t.Error("a volume-to-supply ratio was produced at a past ledger; its denominator does not exist historically")
	}
	if sup.TradesExcludedPct == nil {
		t.Error("the excluded share is nil; it is computable from the same classification")
	}
}

// The engine then assembles oracleResistance from that volume, which is the whole
// point of computing it: a reconstructed row used to carry none.
func TestAReconstructedRowGainsItsOracleWindow(t *testing.T) {
	trades, err := conformance.LoadTradesCSV(conformance.TradesCSVFebruary)
	if err != nil {
		t.Fatalf("load february: %v", err)
	}
	snap := conformance.GoldenSnapshot() // ledger 61340263, close time 00:10:21Z
	sup, _ := historicalSupporting(trades, snap.Base, snap.LedgerClosedAt,
		domain.DefaultParams().OracleWindow, domain.DefaultGenuineRules())

	with, err := domain.ComputeAssetRiskWith(snap, domain.DefaultParams(), sup)
	if err != nil {
		t.Fatal(err)
	}
	if with.OracleResistance == nil {
		t.Fatalf("oracleResistance is still null on a reconstructed row; warnings: %q", with.Warnings)
	}
	without, err := domain.ComputeAssetRisk(snap, domain.DefaultParams())
	if err != nil {
		t.Fatal(err)
	}
	if without.OracleResistance != nil {
		t.Error("the book alone produced an oracle window, which it cannot")
	}
	if len(with.UnevaluatedFlags) >= len(without.UnevaluatedFlags) {
		t.Errorf("unevaluated flags did not fall: %d with metrics, %d without", len(with.UnevaluatedFlags), len(without.UnevaluatedFlags))
	}
}

func TestHistoricalSupportingRefusesAPartialReferenceDay(t *testing.T) {
	// A walk that starts inside the reference day cannot produce that day's
	// median, and DEC-019 section 8.2 refuses a partial-day median.
	anchor := time.Date(2026, 2, 22, 0, 10, 21, 0, time.UTC)
	base := domain.Asset{Code: "X", Issuer: "GISSUER", Type: domain.AssetTypeAlphanum4}
	mk := func(at time.Time) domain.Trade {
		return domain.Trade{
			ID: at.String(), LedgerSeq: 1, ClosedAt: at, Type: "orderbook",
			Price: domain.Price{N: 1057, D: 1000}, BaseAmount: decimal.New(5, 0),
			CounterAmount: decimal.New(5, 0), BaseAccount: "GA", CounterAccount: "GB",
		}
	}
	inside := []domain.Trade{mk(time.Date(2026, 2, 21, 12, 0, 0, 0, time.UTC)), mk(anchor.Add(-time.Minute))}
	sup, notes := historicalSupporting(inside, base, anchor, 15*time.Minute, domain.DefaultGenuineRules())
	if sup != nil {
		t.Errorf("metrics produced from a partial reference day: %+v", sup)
	}
	if len(notes) == 0 {
		t.Error("no reason given for the refusal")
	}

	// One trade on the day before the reference day makes the reference day whole.
	ok := append([]domain.Trade{mk(time.Date(2026, 2, 20, 23, 0, 0, 0, time.UTC))}, inside...)
	if sup, notes = historicalSupporting(ok, base, anchor, 15*time.Minute, domain.DefaultGenuineRules()); sup == nil {
		t.Errorf("refused a complete reference day: %v", notes)
	}
}

func TestHistoricalSupportingIgnoresTradesAfterTheTarget(t *testing.T) {
	// The walk runs past the target by a lookahead, and nothing after the target
	// may reach a metric that describes it.
	anchor := time.Date(2026, 2, 22, 0, 10, 21, 0, time.UTC)
	base := domain.Asset{Code: "X", Issuer: "GISSUER", Type: domain.AssetTypeAlphanum4}
	mk := func(at time.Time, amount string) domain.Trade {
		return domain.Trade{
			ID: at.String() + amount, LedgerSeq: 1, ClosedAt: at, Type: "orderbook",
			Price: domain.Price{N: 1057, D: 1000}, BaseAmount: decimal.RequireFromString(amount),
			CounterAmount: decimal.RequireFromString(amount), BaseAccount: "GA", CounterAccount: "GB",
		}
	}
	trades := []domain.Trade{
		// The walk reaches back past the reference day, which is what lets the
		// reference median exist at all.
		mk(time.Date(2026, 2, 20, 9, 0, 0, 0, time.UTC), "1"),
		mk(time.Date(2026, 2, 21, 9, 0, 0, 0, time.UTC), "1"),
		mk(anchor.Add(-time.Minute), "2"),
		mk(anchor.Add(time.Minute), "9999"), // after the target, inside the lookahead
	}
	sup, notes := historicalSupporting(trades, base, anchor, 15*time.Minute, domain.DefaultGenuineRules())
	if sup == nil {
		t.Fatalf("no metrics: %v", notes)
	}
	if !sup.GenuineVolumeInWindow.Equal(decimal.New(2, 0)) {
		t.Errorf("genuine volume = %s, want 2: the trade after the target must not count", sup.GenuineVolumeInWindow)
	}
}
