// Unit tests for the parts of `keel holders` that touch neither Horizon nor
// Postgres: which assets get pulled, which ledger a pull is stamped with, and
// what a reading becomes once the concentration is computed.
//
// The three cases that matter most are the ones where an absent answer must not
// become a zero one: a truncated pull, a pull whose entire population was
// excluded, and the snapshot label of a pull nothing checked for mutation.

package main

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/Keel-Official/keel-backend/internal/domain"
	"github.com/Keel-Official/keel-backend/internal/horizon"
	"github.com/Keel-Official/keel-backend/internal/store"
)

var (
	hUSTRY = domain.Asset{
		Code:   "USTRY",
		Issuer: "GCRYUGD5NVARGXT56XEZI5CIFCQETYHAPQQTHO2O3IQZTHDH4LATMYWC",
		Type:   domain.AssetTypeAlphanum12,
	}
	hUSDC = domain.Asset{
		Code:   "USDC",
		Issuer: "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN",
		Type:   domain.AssetTypeAlphanum4,
	}
	hXLM = domain.Asset{Code: "XLM", Type: domain.AssetTypeNative}
)

func TestHolderTargetsAreDistinctSortedAndNeverNative(t *testing.T) {
	rows := []store.Asset{
		{ID: 1, Base: hUSTRY, Quote: hUSDC},
		{ID: 2, Base: hXLM, Quote: hUSDC},  // native base, and USDC again
		{ID: 3, Base: hUSTRY, Quote: hXLM}, // USTRY again, against a native quote
	}

	got, err := holderTargets(rows, "")
	if err != nil {
		t.Fatalf("holderTargets: %v", err)
	}

	// Two distinct non-native assets out of three pairs and six asset slots.
	// Pulling USTRY twice would spend 26 requests on an answer already given,
	// which is the whole reason this command exists separately from scan.
	if len(got) != 2 {
		t.Fatalf("got %d target(s), want 2: %v", len(got), got)
	}
	if got[0].String() >= got[1].String() {
		t.Errorf("targets are not sorted: %q then %q", got[0], got[1])
	}
	for _, a := range got {
		if a.IsNative() {
			t.Error("XLM is a target; it has no trustlines to enumerate")
		}
	}
}

func TestHolderTargetsRefusesAnAssetOutsideTheSet(t *testing.T) {
	rows := []store.Asset{{ID: 1, Base: hUSTRY, Quote: hUSDC}}

	if _, err := holderTargets(rows, "USDY:GNOTINTHESET"); err == nil {
		t.Fatal("an asset outside the demonstration set was accepted as a target")
	}
	got, err := holderTargets(rows, hUSTRY.String())
	if err != nil {
		t.Fatalf("holderTargets by name: %v", err)
	}
	if len(got) != 1 || got[0] != hUSTRY {
		t.Errorf("got %v, want just USTRY", got)
	}
}

// The minimum of the two, and the span between them. The minimum because DEC-011
// takes the conservative direction: attributing a figure to an earlier ledger
// understates how current it is rather than overstating it.
func TestSnapshotFromTakesTheMinimumAndTheSpan(t *testing.T) {
	for _, c := range []struct {
		name         string
		first, last  uint32
		wantSnapshot uint32
		wantSpan     int
	}{
		{"ordinary, ledgers advanced during the pull", 64381285, 64381288, 64381285, 3},
		{"single page, no advance", 64381285, 64381285, 64381285, 0},
		{"out of order, which must not produce a negative span", 64381288, 64381285, 64381285, 3},
	} {
		t.Run(c.name, func(t *testing.T) {
			snapshot, span := snapshotFrom(c.first, c.last)
			if snapshot != c.wantSnapshot || span != c.wantSpan {
				t.Errorf("got (%d, %d), want (%d, %d)", snapshot, span, c.wantSnapshot, c.wantSpan)
			}
		})
	}
}

func obs(a domain.Asset, holders []horizon.Holder, reported int, truncated bool) horizon.HolderObservation {
	return horizon.HolderObservation{
		Asset:       a,
		Holders:     holders,
		HolderCount: reported,
		Raw: horizon.RawHolders{
			FirstLedger: 64381285,
			LastLedger:  64381288,
			Truncated:   truncated,
		},
	}
}

func holder(id, balance string) horizon.Holder {
	return horizon.Holder{AccountID: id, Balance: decimal.RequireFromString(balance)}
}

func TestReadingFromComputesConcentrationAndExcludesTheIssuer(t *testing.T) {
	o := obs(hUSTRY, []horizon.Holder{
		holder(hUSTRY.Issuer, "9000"), // the issuer holds undistributed supply
		holder("GAAA", "75"),
		holder("GBBB", "25"),
		holder("GCCC", "0"), // a zero balance is dropped, not counted
	}, 4, false)

	r, err := readingFrom(o, 7, time.Date(2026, 9, 12, 3, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("readingFrom: %v", err)
	}

	if r.Truncated {
		t.Error("the reading is marked truncated and the pull was complete")
	}
	if r.Population == nil || *r.Population != 2 {
		t.Errorf("population is %v, want 2 after the issuer and the zero balance are removed", r.Population)
	}
	// 75 of 100, because the issuer's 9000 is out of the denominator.
	if r.Top1Pct == nil || !r.Top1Pct.Equal(decimal.RequireFromString("75")) {
		t.Errorf("top 1%% is %v, want 75; the issuer's balance is in the denominator", r.Top1Pct)
	}
	if r.ExcludedDropped == nil || *r.ExcludedDropped != 1 {
		t.Errorf("excluded_dropped is %v, want 1", r.ExcludedDropped)
	}
	if r.ZeroBalanceDropped == nil || *r.ZeroBalanceDropped != 1 {
		t.Errorf("zero_balance_dropped is %v, want 1", r.ZeroBalanceDropped)
	}
	// What was removed is recorded, because the list is asset-specific and may
	// change, and a population must never be compared across two lists silently.
	if r.Exclusions.Issuer != hUSTRY.Issuer {
		t.Errorf("exclusions_applied is %+v", r.Exclusions)
	}
	if r.RunID == nil || *r.RunID != 7 {
		t.Errorf("run id is %v, want 7", r.RunID)
	}
}

// The rule tugas A1 states in as many words: ErrHolderSetTruncated is not to be
// worked around. The row is stored, and it carries no figures.
func TestReadingFromStoresATruncatedPullWithNoFigures(t *testing.T) {
	o := obs(hUSTRY, []horizon.Holder{
		holder("GAAA", "75"),
		holder("GBBB", "25"),
	}, 41233, true)

	r, err := readingFrom(o, 7, time.Now().UTC())
	if err != nil {
		t.Fatalf("readingFrom: %v", err)
	}
	if !r.Truncated {
		t.Fatal("the reading is not marked truncated")
	}
	if r.Top1Pct != nil || r.Top10Pct != nil || r.HHI != nil || r.Population != nil || r.CirculatingSupply != nil {
		t.Error("a truncated pull produced figures; a truncated trustline set answers the question not at all")
	}
	if r.HoldersRead != 2 || r.HolderCountReported != 41233 {
		t.Errorf("read %d of %d; the row cannot show its own truncation", r.HoldersRead, r.HolderCountReported)
	}
}

// Every holder excluded leaves a zero denominator, so the percentages are
// undefined rather than zero. Stored the same way a truncated pull is: the pull
// happened, and it answered nothing.
func TestReadingFromStoresAnEmptyPopulationWithNoFigures(t *testing.T) {
	o := obs(hUSTRY, []horizon.Holder{holder(hUSTRY.Issuer, "9000")}, 1, false)

	r, err := readingFrom(o, 7, time.Now().UTC())
	if err != nil {
		t.Fatalf("readingFrom: %v", err)
	}
	if r.Top1Pct != nil || r.HHI != nil || r.CirculatingSupply != nil {
		t.Error("an empty population produced figures; a zero denominator makes them undefined, not zero")
	}
	if r.Truncated {
		t.Error("the pull is marked truncated and it was not; it was complete and empty after exclusions")
	}
}

// DEC-011's atomic/mixed verdict needs a mutated-row count that nothing computes
// yet, so every row written today says so rather than claiming the value a
// reader trusts most.
func TestReadingFromNeverClaimsAnUnmeasuredSnapshotLabel(t *testing.T) {
	o := obs(hUSTRY, []horizon.Holder{holder("GAAA", "1")}, 1, false)

	r, err := readingFrom(o, 7, time.Now().UTC())
	if err != nil {
		t.Fatalf("readingFrom: %v", err)
	}
	if r.SnapshotLabel != store.LabelUnknown {
		t.Errorf("snapshot label is %q; nothing counted the mutated rows, so it cannot be %q",
			r.SnapshotLabel, store.LabelAtomic)
	}
	if r.SnapshotBasis != store.BasisFirstAndLastPage {
		t.Errorf("snapshot basis is %q; DEC-011's all-pages minimum is not implemented upstream yet",
			r.SnapshotBasis)
	}
	if r.MutatedRows != nil || r.MutatedBalance != nil {
		t.Error("mutated rows or balance were filled in; nothing counted them and 0 would be a measurement nobody made")
	}
	if r.MethodologyVersion != domain.MethodologyVersion {
		t.Errorf("methodology version is %q, want %q", r.MethodologyVersion, domain.MethodologyVersion)
	}
}
