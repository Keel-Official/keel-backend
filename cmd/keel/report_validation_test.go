package main

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/Keel-Official/keel-backend/internal/domain"
	"github.com/shopspring/decimal"
)

// These are supplementary assistant-prepared arithmetic checks, not the locked
// human Layer 1 oracle. The rational expectations were prepared separately in
// docs/report/representative-calculations.py before this comparison was run.
func TestReportRepresentativeCalculations(t *testing.T) {
	type rational struct {
		Numerator   string
		Denominator string
	}
	var sheet struct {
		Cases []struct {
			Name      string
			Mid       rational
			SpreadPct rational
			Depth     []struct {
				Delta string
				Buy   rational
				Sell  rational
			}
			Manipulation []struct {
				Delta     string
				Cost      rational
				Reachable bool
			}
		}
	}
	raw, err := os.ReadFile("../../docs/report/representative-calculations.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &sheet); err != nil {
		t.Fatal(err)
	}
	if len(sheet.Cases) != 2 {
		t.Fatal("expected normal and broken review cases")
	}
	for _, c := range sheet.Cases {
		t.Run(c.Name, func(t *testing.T) {
			s := replayForPersistence().Snapshot
			switch c.Name {
			case "normal":
				s.Book = domain.OrderBook{
					Bids: []domain.Level{{Price: domain.Price{N: 99, D: 100}, Amount: decimal.NewFromInt(1000000)}},
					Asks: []domain.Level{{Price: domain.Price{N: 101, D: 100}, Amount: decimal.NewFromInt(1000000)}},
				}
			case "broken": // The existing synthetic book-only input, not real pool absence.
			default:
				t.Fatalf("unknown case %q", c.Name)
			}
			risk, err := domain.ComputeAssetRisk(s, domain.DefaultParams())
			if err != nil {
				t.Fatal(err)
			}
			check := func(label string, got decimal.Decimal, want rational) {
				t.Helper()
				expected := decimal.RequireFromString(want.Numerator).DivRound(decimal.RequireFromString(want.Denominator), 28)
				if got.Sub(expected).Abs().GreaterThan(decimal.RequireFromString("0.0000001")) {
					t.Fatalf("%s: backend=%s independent=%s", label, got, expected)
				}
				t.Logf("%s: backend=%s independent=%s", label, got, expected)
			}
			if risk.MidPrice == nil || risk.SpreadPct == nil {
				t.Fatal("calculated input unexpectedly has no price or spread")
			}
			check("mid", *risk.MidPrice, c.Mid)
			check("spreadPct", *risk.SpreadPct, c.SpreadPct)
			if len(risk.Depth) != len(c.Depth) || len(risk.ManipulationCostOrderbookOnly) != len(c.Manipulation) {
				t.Fatal("backend ladder differs from review scope")
			}
			for i, want := range c.Depth {
				got := risk.Depth[i]
				if !got.Delta.Equal(decimal.RequireFromString(want.Delta)) {
					t.Fatal("depth delta mismatch")
				}
				check("buy "+want.Delta, got.BuySide, want.Buy)
				check("sell "+want.Delta, got.SellSide, want.Sell)
			}
			for i, want := range c.Manipulation {
				got := risk.ManipulationCostOrderbookOnly[i]
				if !got.Delta.Equal(decimal.RequireFromString(want.Delta)) || got.Reachable != want.Reachable {
					t.Fatalf("manipulation semantics mismatch at %s", want.Delta)
				}
				check("cost "+want.Delta, got.Cost, want.Cost)
			}
			t.Logf("methodology=%s; synthetic input only; tolerance=0.0000001", risk.MethodologyVersion)
		})
	}
	t.Run("incomplete", func(t *testing.T) {
		for _, unknownPool := range []bool{false, true} {
			r := replayForPersistence()
			if unknownPool {
				r.Snapshot.Pools = nil
			} else {
				r.MissingOfferIDs = []int64{1822775941}
			}
			db := &replayStoreProbe{}
			if _, _, err := persistReplay(context.Background(), db, r); err == nil || db.writes != 0 {
				t.Fatalf("unsupported input accepted: unknownPool=%t error=%v writes=%d", unknownPool, err, db.writes)
			}
		}
	})
}
