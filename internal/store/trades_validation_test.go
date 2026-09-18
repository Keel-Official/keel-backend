package store

import (
	"strings"
	"testing"
	"time"

	"github.com/Keel-Official/keel-backend/internal/domain"
	"github.com/shopspring/decimal"
)

// These exercise validTradeReading alone, which needs no database. The schema in
// migrations/0009 refuses the same states; this is the same refusal arriving one
// step earlier with a message that names the field.

func validReading() TradeReading {
	return TradeReading{
		AssetID:                 7,
		FetchedAt:               time.Date(2026, 9, 18, 6, 0, 0, 0, time.UTC),
		MethodologyVersion:      domain.MethodologyVersion,
		Anchor:                  time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC),
		LedgerSeq:               64488876,
		Scope:                   ScopeLastGenuineOnly,
		VolumeUnevaluatedReason: "above the DEC-019 threshold",
	}
}

func TestATradeReadingNeedsTheFieldsThatMakeItCitable(t *testing.T) {
	for _, tc := range []struct {
		name string
		mut  func(*TradeReading)
		want string
	}{
		{"no pair", func(r *TradeReading) { r.AssetID = 0 }, "AssetID"},
		{"no methodology version", func(r *TradeReading) { r.MethodologyVersion = "" }, "MethodologyVersion"},
		{"no fetch time", func(r *TradeReading) { r.FetchedAt = time.Time{} }, "FetchedAt"},
		{"no anchor", func(r *TradeReading) { r.Anchor = time.Time{} }, "Anchor"},
		{"an invented scope", func(r *TradeReading) { r.Scope = "everything" }, "scope"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := validReading()
			tc.mut(&r)
			err := validTradeReading(r)
			if err == nil {
				t.Fatalf("want a refusal naming %s", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not name %s", err, tc.want)
			}
		})
	}
}

// THE CHEAP HALF MUST NOT CARRY VOLUME FIGURES, which is the constraint that
// stops a last-genuine-only row being read as a measured zero. DEC-019 section 5
// is explicit that a reader cannot tell "too busy to classify" from "too quiet to
// have data" without the reason string, so the reason is required too.
func TestTheCheapHalfCannotSmuggleInVolumeFigures(t *testing.T) {
	one := decimal.NewFromInt(1)

	for _, tc := range []struct {
		name string
		mut  func(*TradeReading)
		want string
	}{
		{"a 24 hour volume", func(r *TradeReading) { r.GenuineBaseD1 = &one }, "volume figures"},
		{"a 30 day volume", func(r *TradeReading) { r.GenuineBaseD30 = &one }, "volume figures"},
		{"an exclusion share", func(r *TradeReading) { r.TradesExcludedPct = &one }, "exclusion share"},
		{"no reason at all", func(r *TradeReading) { r.VolumeUnevaluatedReason = "" }, "reason"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := validReading()
			tc.mut(&r)
			err := validTradeReading(r)
			if err == nil {
				t.Fatalf("want a refusal mentioning %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not mention %q", err, tc.want)
			}
		})
	}

	// The same figures are fine once the scope says the window was walked.
	r := validReading()
	r.Scope = ScopeFullWindow
	r.VolumeUnevaluatedReason = ""
	r.GenuineBaseD1, r.GenuineBaseD30, r.TradesExcludedPct = &one, &one, &one
	if err := validTradeReading(r); err != nil {
		t.Errorf("a full-window reading carrying its figures was refused: %v", err)
	}
}

// THE LEDGER IS REQUIRED ONLY OF A ROW THAT CITES SOMETHING, and the asymmetry
// is a measurement rather than a softening. Public Horizon sends no Latest-Ledger
// header on /trades, so the sequence comes from the newest trade's own paging
// token; a pair that had no trade in the walked window has none to take it from.
// A row that DID find a genuine trade necessarily saw a trade, so it can name one.
func TestTheLedgerIsRequiredOfARowThatCitesATrade(t *testing.T) {
	quiet := validReading()
	quiet.LedgerSeq = 0
	if err := validTradeReading(quiet); err != nil {
		t.Errorf("a walk that saw no trades was refused for having no ledger: %v", err)
	}

	citing := validReading()
	citing.LedgerSeq = 0
	citing.LastGenuine = &domain.TradeRef{
		LedgerSeq: 61340262,
		At:        time.Date(2026, 9, 17, 4, 0, 0, 0, time.UTC),
	}
	err := validTradeReading(citing)
	if err == nil {
		t.Fatal("a row citing a genuine trade with no walk ledger was accepted")
	}
	if !strings.Contains(err.Error(), "LedgerSeq") {
		t.Errorf("error %q does not name LedgerSeq", err)
	}
}

// A reference to a trade is whole or absent. Half of one claims a genuine trade
// happened and refuses to say when, which no consumer can check against the chain.
func TestALastGenuineTradeIsWholeOrAbsent(t *testing.T) {
	r := validReading()
	r.LastGenuine = &domain.TradeRef{LedgerSeq: 61340262}
	if err := validTradeReading(r); err == nil {
		t.Error("a reference with no close time was accepted")
	}

	r = validReading()
	r.LastGenuine = &domain.TradeRef{At: time.Date(2026, 9, 17, 4, 0, 0, 0, time.UTC)}
	if err := validTradeReading(r); err == nil {
		t.Error("a reference with no ledger was accepted")
	}

	r = validReading()
	r.LastGenuine = &domain.TradeRef{LedgerSeq: 61340262, At: time.Date(2026, 9, 17, 4, 0, 0, 0, time.UTC)}
	if err := validTradeReading(r); err != nil {
		t.Errorf("a whole reference was refused: %v", err)
	}
}

// Covered() is what the scan round passes to domain.VolumeToSupply as
// TradesCover, and that argument is what separates a measured zero from an
// absence of data. Getting it from the scope rather than from the presence of a
// figure is the point: an empty window and an unwalked one both sum to nothing.
func TestCoveredFollowsTheScopeAndNotTheFigures(t *testing.T) {
	r := validReading()
	if r.Covered() {
		t.Error("a last-genuine-only reading does not cover the FR-9 windows")
	}
	r.Scope = ScopeFullWindow
	if !r.Covered() {
		t.Error("a full-window reading covers them")
	}
}

// The age is measured from the ANCHOR and not from the pull. Two pulls twenty
// minutes apart either side of midnight describe windows a day apart, and a bound
// built on FetchedAt would call them equally fresh.
func TestAgeIsMeasuredFromTheAnchor(t *testing.T) {
	r := validReading()
	r.FetchedAt = time.Date(2026, 9, 18, 23, 0, 0, 0, time.UTC)
	now := time.Date(2026, 9, 19, 1, 0, 0, 0, time.UTC)

	if got, want := r.Age(now), 25*time.Hour; got != want {
		t.Errorf("age = %s, want %s: it must not be measured from FetchedAt", got, want)
	}
}
