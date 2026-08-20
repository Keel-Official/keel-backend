//go:build conformance

// Methodology conformance test against the USTRY/USDC golden fixture.
//
// WHY THE BUILD TAG. This file imports internal/depth, which currently has no
// body. Without the tag the whole repository fails to build and CI loses its
// meaning. The tag is TEMPORARY.
//
// CONDITION FOR REMOVAL: once internal/depth contains an implementation that
// passes, delete the //go:build line above and delete the separate `conformance`
// target in the Makefile. This test belongs in a plain `make test`, not on a
// special path that is easy to forget.
//
// For now, run it with: make conformance

package conformance

import (
	"encoding/json"
	"sort"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/Keel-Official/keel/internal/depth"
	"github.com/Keel-Official/keel/internal/domain"
)

// eqDec compares in the decimal domain, not in float64, and not through
// math.Abs. An earlier version of this test used both, which violated the rules
// of the very package it was testing.
func eqDec(t *testing.T, label string, got, want decimal.Decimal) {
	t.Helper()
	if got.Sub(want).Abs().GreaterThan(Tolerance) {
		t.Errorf("%s = %s, want %s (tolerance %s)", label, got, want, Tolerance)
	}
}

func mustRisk(t *testing.T) domain.AssetRisk {
	t.Helper()
	r, err := depth.ComputeAssetRisk(GoldenSnapshot(), DefaultParams())
	if err != nil {
		t.Fatalf("ComputeAssetRisk: %v", err)
	}
	return r
}

// ---------------------------------------------------------------- Price

func TestMidPrice(t *testing.T) {
	got, src := depth.MidPrice(GoldenSnapshot())
	eqDec(t, "P0", got, ExpectedP0)
	if src != ExpectedPriceSource {
		t.Errorf("priceSource = %q, want %q", src, ExpectedPriceSource)
	}
}

func TestSpreadPct(t *testing.T) {
	r := mustRisk(t)
	if r.SpreadPct == nil {
		t.Fatal("spreadPct is nil, want 196.0777141; nil means unknown and both sides of the book are populated")
	}
	eqDec(t, "spreadPct", *r.SpreadPct, ExpectedSpreadPct)
}

// ---------------------------------------------------------------- Depth

func TestComputeDepth(t *testing.T) {
	p := DefaultParams()
	got, err := depth.ComputeDepth(GoldenSnapshot(), ExpectedP0, p.MarketDeltas)
	if err != nil {
		t.Fatalf("ComputeDepth: %v", err)
	}
	if len(got) != len(ExpectedDepth) {
		t.Fatalf("number of depth rows = %d, want %d", len(got), len(ExpectedDepth))
	}
	for i, want := range ExpectedDepth {
		g := got[i]
		eqDec(t, "delta["+want.Delta.String()+"]", g.Delta, want.Delta)
		eqDec(t, "depth("+want.Delta.String()+").BuySide  "+want.Reason, g.BuySide, want.BuySide)
		eqDec(t, "depth("+want.Delta.String()+").SellSide "+want.Reason, g.SellSide, want.SellSide)
		eqDec(t, "depth("+want.Delta.String()+").FromSdex", g.FromSdex, want.FromSdex)
		eqDec(t, "depth("+want.Delta.String()+").FromAmm (Pools is empty)", g.FromAmm, want.FromAmm)
	}
}

// ---------------------------------------------------------------- Manipulation

func TestComputeManipulationCost(t *testing.T) {
	p := DefaultParams()
	got, err := depth.ComputeManipulationCost(GoldenSnapshot(), ExpectedP0, p.ManipulationDeltas)
	if err != nil {
		t.Fatalf("ComputeManipulationCost: %v", err)
	}
	if len(got) != len(ExpectedManipulation) {
		t.Fatalf("number of manipulationCost rows = %d, want %d", len(got), len(ExpectedManipulation))
	}
	for i, want := range ExpectedManipulation {
		g := got[i]
		d := want.Delta.String()
		eqDec(t, "MC("+d+").Delta", g.Delta, want.Delta)
		eqDec(t, "MC("+d+").TargetPrice", g.TargetPrice, want.TargetPrice)
		eqDec(t, "MC("+d+").Cost", g.Cost, want.Cost)
		if g.Reachable != want.Reachable {
			t.Errorf("MC(%s).Reachable = %v, want %v. Reason: %s",
				d, g.Reachable, want.Reachable, want.Reason)
		}
	}
}

func TestMaxReachablePrice(t *testing.T) {
	r := mustRisk(t)
	if r.MaxReachablePrice == nil {
		t.Fatal("maxReachablePrice is nil, want 106.7372828; the book has an ask so the value is defined")
	}
	eqDec(t, "maxReachablePrice", *r.MaxReachablePrice, ExpectedMaxReachablePrice)

	if r.CostToMaxReachablePrice == nil {
		t.Fatal("costToMaxReachablePrice is nil, want 0")
	}
	eqDec(t, "costToMaxReachablePrice (free, no cheaper ask exists)",
		*r.CostToMaxReachablePrice, ExpectedCostToMaxReachablePrice)
}

// ---------------------------------------------------------------- Flags

func flagSet(fs []domain.Flag) []string {
	out := make([]string, len(fs))
	for i, f := range fs {
		out[i] = string(f)
	}
	sort.Strings(out)
	return out
}

func compareFlags(t *testing.T, label string, got, want []domain.Flag) {
	t.Helper()
	g, w := flagSet(got), flagSet(want)
	if len(g) != len(w) {
		t.Errorf("%s = %v, want %v", label, g, w)
		return
	}
	for i := range g {
		if g[i] != w[i] {
			t.Errorf("%s = %v, want %v", label, g, w)
			return
		}
	}
}

func TestFlagsAndBand(t *testing.T) {
	r := mustRisk(t)
	compareFlags(t, "flags", r.Flags, ExpectedFlags)
	compareFlags(t, "unevaluatedFlags", r.UnevaluatedFlags, ExpectedUnevaluatedFlags)

	if r.Band != ExpectedBand {
		t.Errorf("band = %q, want %q", r.Band, ExpectedBand)
	}
	if r.BandConfidence != ExpectedBandConfidence {
		t.Errorf("bandConfidence = %q, want %q", r.BandConfidence, ExpectedBandConfidence)
	}
}

// TestClearFlagsTidakIkutUnevaluated guards the distinction that version 1.0.2
// exists for: clear and unevaluated must never be swapped.
func TestClearFlagsTidakIkutUnevaluated(t *testing.T) {
	r := mustRisk(t)
	for _, clear := range ExpectedClearFlags {
		for _, u := range r.UnevaluatedFlags {
			if u == clear {
				t.Errorf("flag %q is reported unevaluated, but it can be checked and the result is clear", clear)
			}
		}
		for _, f := range r.Flags {
			if f == clear {
				t.Errorf("flag %q is reported triggered, but it should be clear", clear)
			}
		}
	}
}

// ---------------------------------------------------------------- Invariants

// Invariants 1 and 2 in testdata/fixtures/ustry_pre_exploit.md.
func TestInvarianMonotonisitas(t *testing.T) {
	p := DefaultParams()

	d, err := depth.ComputeDepth(GoldenSnapshot(), ExpectedP0, p.MarketDeltas)
	if err != nil {
		t.Fatalf("ComputeDepth: %v", err)
	}
	for i := 1; i < len(d); i++ {
		if d[i].BuySide.LessThan(d[i-1].BuySide) {
			t.Errorf("buy side depth decreases from delta %s to %s: %s then %s",
				d[i-1].Delta, d[i].Delta, d[i-1].BuySide, d[i].BuySide)
		}
		if d[i].SellSide.LessThan(d[i-1].SellSide) {
			t.Errorf("sell side depth decreases from delta %s to %s: %s then %s",
				d[i-1].Delta, d[i].Delta, d[i-1].SellSide, d[i].SellSide)
		}
	}

	mc, err := depth.ComputeManipulationCost(GoldenSnapshot(), ExpectedP0, p.ManipulationDeltas)
	if err != nil {
		t.Fatalf("ComputeManipulationCost: %v", err)
	}
	for i := 1; i < len(mc); i++ {
		if mc[i].Cost.LessThan(mc[i-1].Cost) {
			t.Errorf("manipulation cost decreases from delta %s to %s: %s then %s",
				mc[i-1].Delta, mc[i].Delta, mc[i-1].Cost, mc[i].Cost)
		}
	}
}

// Invariant 3: maxReachablePrice equals exactly the highest ask price on the book.
func TestInvarianMaxReachableAdalahAskTertinggi(t *testing.T) {
	s := GoldenSnapshot()
	if len(s.Book.Asks) == 0 {
		t.Skip("fixture has no ask")
	}
	tertinggi := s.Book.Asks[0].Price
	for _, a := range s.Book.Asks[1:] {
		if a.Price.Cmp(tertinggi) > 0 {
			tertinggi = a.Price
		}
	}
	r := mustRisk(t)
	if r.MaxReachablePrice == nil {
		t.Fatal("maxReachablePrice is nil although the book has an ask")
	}
	eqDec(t, "maxReachablePrice vs the highest ask on the book",
		*r.MaxReachablePrice, tertinggi.Decimal())
}

// Invariant 5: NFR-9. Running the computation twice produces byte for byte
// identical JSON.
//
// This test is cheap and it catches violations of the bans on time.Now,
// math/rand, and unsorted map iteration automatically, without anyone having to
// read the code.
func TestInvarianDeterminisme(t *testing.T) {
	a, err := depth.ComputeAssetRisk(GoldenSnapshot(), DefaultParams())
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	b, err := depth.ComputeAssetRisk(GoldenSnapshot(), DefaultParams())
	if err != nil {
		t.Fatalf("second run: %v", err)
	}

	ja, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("marshal first run: %v", err)
	}
	jb, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("marshal second run: %v", err)
	}
	if string(ja) != string(jb) {
		t.Errorf("the two runs produced different JSON\nfirst:  %s\nsecond: %s", ja, jb)
	}
}

// ---------------------------------------------------------------- Metadata

// Every output is required to carry LedgerSeq and MethodologyVersion.
func TestMetadataWajib(t *testing.T) {
	r := mustRisk(t)
	s := GoldenSnapshot()

	if r.LedgerSeq != s.LedgerSeq {
		t.Errorf("ledgerSeq = %d, want %d", r.LedgerSeq, s.LedgerSeq)
	}
	if r.MethodologyVersion != domain.MethodologyVersion {
		t.Errorf("methodologyVersion = %q, want %q", r.MethodologyVersion, domain.MethodologyVersion)
	}
	if r.DataSource != s.Source {
		t.Errorf("dataSource = %q, want %q", r.DataSource, s.Source)
	}
	if !r.LedgerClosedAt.Equal(s.LedgerClosedAt) {
		t.Errorf("ledgerClosedAt = %v, want %v", r.LedgerClosedAt, s.LedgerClosedAt)
	}
}
