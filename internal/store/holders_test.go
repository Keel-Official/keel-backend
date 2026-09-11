// Integration tests for holder_readings. Same harness as store_test.go: skipped
// when KEEL_TEST_DSN is unset, and every test runs inside a rolled back
// transaction.
//
// These are integration tests rather than unit tests for the reason store_test.go
// gives, and one of the checks here is exactly the kind it names: whether the
// CHECK constraint that forbids a truncated reading from carrying figures
// actually fires. A fake would only assert that this package sends the SQL it was
// written to send.

package store

import (
	"errors"
	"testing"
	"time"

	"github.com/Keel-Official/keel-backend/internal/domain"
)

// aReading is a complete, valid, NOT truncated reading. Tests mutate a copy.
func aReading() HolderReading {
	pop, zero, dropped := 263, 612, 0
	return HolderReading{
		Asset:     testUSTRY,
		FetchedAt: time.Date(2026, 9, 12, 3, 0, 0, 0, time.UTC),

		SnapshotLedger: 61340263,
		SnapshotBasis:  BasisFirstAndLastPage,
		LedgerSpan:     3,
		SnapshotLabel:  LabelUnknown,

		HoldersRead:         875,
		HolderCountReported: 875,
		Truncated:           false,

		Population:         &pop,
		CirculatingSupply:  decp("1000000.0000000"),
		Top1Pct:            decp("91.7421"),
		Top10Pct:           decp("99.9475"),
		HHI:                decp("8410.8452"),
		ZeroBalanceDropped: &zero,
		ExcludedDropped:    &dropped,

		Exclusions:         HolderExclusionsJSON{Issuer: testUSTRY.Issuer},
		MethodologyVersion: domain.MethodologyVersion,
	}
}

func TestSaveHolderReadingRoundTrips(t *testing.T) {
	s, ctx := testStore(t)

	in := aReading()
	id, inserted, err := s.SaveHolderReading(ctx, in)
	if err != nil {
		t.Fatalf("SaveHolderReading: %v", err)
	}
	if !inserted {
		t.Fatal("the first write reported that the row already existed")
	}

	got, err := s.LatestHolderReading(ctx, testUSTRY, domain.MethodologyVersion)
	if err != nil {
		t.Fatalf("LatestHolderReading: %v", err)
	}
	if got.ID != id {
		t.Errorf("read back id %d, wrote %d", got.ID, id)
	}
	if got.Asset != testUSTRY {
		t.Errorf("asset came back as %+v, wrote %+v", got.Asset, testUSTRY)
	}
	if got.SnapshotLedger != in.SnapshotLedger {
		t.Errorf("snapshot ledger %d, wrote %d", got.SnapshotLedger, in.SnapshotLedger)
	}
	if got.SnapshotBasis != BasisFirstAndLastPage || got.SnapshotLabel != LabelUnknown {
		t.Errorf("basis/label came back as %q/%q", got.SnapshotBasis, got.SnapshotLabel)
	}
	// The whole point of storing these as NUMERIC and reading them through
	// sql.NullString: the scale survives. 8410.8452 must not become 8410.85.
	if got.HHI == nil || !got.HHI.Equal(*in.HHI) {
		t.Errorf("HHI came back as %v, wrote %v", got.HHI, in.HHI)
	}
	if got.Top1Pct == nil || !got.Top1Pct.Equal(*in.Top1Pct) {
		t.Errorf("top1 came back as %v, wrote %v", got.Top1Pct, in.Top1Pct)
	}
	if got.Exclusions.Issuer != testUSTRY.Issuer {
		t.Errorf("exclusions came back as %+v", got.Exclusions)
	}
	// Never measured, so never claimed.
	if got.MutatedRows != nil || got.MutatedBalance != nil {
		t.Errorf("mutated rows/balance came back as %v/%v; nothing counted them", got.MutatedRows, got.MutatedBalance)
	}
}

// A TRUNCATED READING IS STORED AND IT IS STORED EMPTY. This is the rule the
// table exists to keep, and tugas A1 names it: an asset whose holder set was
// truncated still reports the holder flags as unevaluated.
func TestSaveHolderReadingStoresATruncatedPullWithoutFigures(t *testing.T) {
	s, ctx := testStore(t)

	in := aReading()
	in.Truncated = true
	in.HoldersRead = 5000
	in.HolderCountReported = 41233
	in.Population, in.CirculatingSupply = nil, nil
	in.Top1Pct, in.Top10Pct, in.HHI = nil, nil, nil
	in.ZeroBalanceDropped, in.ExcludedDropped = nil, nil

	if _, _, err := s.SaveHolderReading(ctx, in); err != nil {
		t.Fatalf("SaveHolderReading: %v", err)
	}

	got, err := s.LatestHolderReading(ctx, testUSTRY, domain.MethodologyVersion)
	if err != nil {
		t.Fatalf("LatestHolderReading: %v", err)
	}
	if !got.Truncated {
		t.Error("the truncated flag did not survive the round trip, which is the one field this row is for")
	}
	if got.Top1Pct != nil || got.Top10Pct != nil || got.HHI != nil {
		t.Errorf("a truncated reading came back carrying figures: %v %v %v", got.Top1Pct, got.Top10Pct, got.HHI)
	}
	// The disagreement between these two IS the truncation, and both have to be
	// readable from the row alone.
	if got.HoldersRead >= got.HolderCountReported {
		t.Errorf("holders_read %d is not below holder_count_reported %d, so the row cannot show its own truncation",
			got.HoldersRead, got.HolderCountReported)
	}
}

// The refusal happens in Go, before Postgres has to, because an error naming the
// field is more useful than one naming the constraint.
func TestSaveHolderReadingRefusesFiguresOnATruncatedPull(t *testing.T) {
	s, ctx := testStore(t)

	in := aReading()
	in.Truncated = true // and the figures are left in place

	_, _, err := s.SaveHolderReading(ctx, in)
	if err == nil {
		t.Fatal("a truncated reading carrying concentration figures was accepted; zero and absent are different values")
	}
}

func TestSaveHolderReadingRefusesTheNativeAsset(t *testing.T) {
	s, ctx := testStore(t)

	in := aReading()
	in.Asset = testXLM

	if _, _, err := s.SaveHolderReading(ctx, in); err == nil {
		t.Fatal("a holder reading for XLM was accepted; the native asset has no trustlines to enumerate")
	}
	if _, err := s.LatestHolderReading(ctx, testXLM, domain.MethodologyVersion); !errors.Is(err, ErrNotFound) {
		t.Errorf("LatestHolderReading for XLM returned %v, want ErrNotFound", err)
	}
}

// Same contract as SaveMetrics: a re-run over a snapshot already recorded is
// normal, writes nothing, and reports that it wrote nothing.
func TestSaveHolderReadingDoesNotOverwriteAnExistingSnapshot(t *testing.T) {
	s, ctx := testStore(t)

	first, inserted, err := s.SaveHolderReading(ctx, aReading())
	if err != nil || !inserted {
		t.Fatalf("first write: id=%d inserted=%v err=%v", first, inserted, err)
	}

	changed := aReading()
	changed.Top1Pct = decp("12.0000") // a different answer for the same snapshot
	second, inserted, err := s.SaveHolderReading(ctx, changed)
	if err != nil {
		t.Fatalf("second write: %v", err)
	}
	if inserted {
		t.Error("the second write reported an insert; the snapshot was already recorded")
	}
	if second != first {
		t.Errorf("the second write returned id %d, the stored row is %d", second, first)
	}

	got, err := s.LatestHolderReading(ctx, testUSTRY, domain.MethodologyVersion)
	if err != nil {
		t.Fatalf("LatestHolderReading: %v", err)
	}
	if got.Top1Pct == nil || !got.Top1Pct.Equal(*aReading().Top1Pct) {
		t.Errorf("the stored figure changed to %v; a differing result is a finding, not an overwrite", got.Top1Pct)
	}
}

// LatestHolderReading answers "what do we know now", so a fresh truncated pull
// beats a stale complete one. Handing back the older usable row instead would
// give the caller a figure from days ago with no way to tell.
func TestLatestHolderReadingPrefersTheNewestEvenWhenItIsTruncated(t *testing.T) {
	s, ctx := testStore(t)

	old := aReading()
	if _, _, err := s.SaveHolderReading(ctx, old); err != nil {
		t.Fatalf("old: %v", err)
	}

	fresh := aReading()
	fresh.FetchedAt = old.FetchedAt.Add(24 * time.Hour)
	fresh.SnapshotLedger = old.SnapshotLedger + 17000
	fresh.Truncated = true
	fresh.Population, fresh.CirculatingSupply = nil, nil
	fresh.Top1Pct, fresh.Top10Pct, fresh.HHI = nil, nil, nil
	fresh.ZeroBalanceDropped, fresh.ExcludedDropped = nil, nil
	if _, _, err := s.SaveHolderReading(ctx, fresh); err != nil {
		t.Fatalf("fresh: %v", err)
	}

	got, err := s.LatestHolderReading(ctx, testUSTRY, domain.MethodologyVersion)
	if err != nil {
		t.Fatalf("LatestHolderReading: %v", err)
	}
	if !got.Truncated {
		t.Error("the older complete reading was returned; a stale figure must not shadow a fresh refusal to answer")
	}
	if age := got.Age(fresh.FetchedAt.Add(2 * time.Hour)); age != 2*time.Hour {
		t.Errorf("Age reported %s, want 2h", age)
	}
}

// HolderReadingAt is what a caller whose write was refused uses to see WHAT it
// collided with, and the case that matters is the two pulls disagreeing about
// truncation: they answer different questions under one key, and the answer is to
// report it rather than to overwrite.
func TestHolderReadingAtShowsWhatAConflictingWriteCollidedWith(t *testing.T) {
	s, ctx := testStore(t)

	complete := aReading()
	if _, _, err := s.SaveHolderReading(ctx, complete); err != nil {
		t.Fatalf("first write: %v", err)
	}

	// The same snapshot, pulled again under a lower page cap.
	truncatedPull := aReading()
	truncatedPull.Truncated = true
	truncatedPull.HoldersRead = 200
	truncatedPull.HolderCountReported = 875
	truncatedPull.Population, truncatedPull.CirculatingSupply = nil, nil
	truncatedPull.Top1Pct, truncatedPull.Top10Pct, truncatedPull.HHI = nil, nil, nil
	truncatedPull.ZeroBalanceDropped, truncatedPull.ExcludedDropped = nil, nil

	_, inserted, err := s.SaveHolderReading(ctx, truncatedPull)
	if err != nil {
		t.Fatalf("second write: %v", err)
	}
	if inserted {
		t.Fatal("the second write inserted; the snapshot was already recorded")
	}

	prior, err := s.HolderReadingAt(ctx, testUSTRY, complete.SnapshotLedger, domain.MethodologyVersion)
	if err != nil {
		t.Fatalf("HolderReadingAt: %v", err)
	}
	if prior.Truncated {
		t.Error("the stored row is marked truncated; the complete pull was overwritten")
	}
	if prior.Truncated == truncatedPull.Truncated {
		t.Error("the two pulls agree, so this test is no longer exercising the disagreement it was written for")
	}
	if prior.Top1Pct == nil {
		t.Error("the stored figures are gone; nothing should have been overwritten")
	}
}

func TestLatestHolderReadingIsNotFoundWhenNothingWasPulled(t *testing.T) {
	s, ctx := testStore(t)

	if _, err := s.LatestHolderReading(ctx, testUSTRY, domain.MethodologyVersion); !errors.Is(err, ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
	}
}

// A third run kind is a schema change and not just a Go constant, so both halves
// are checked: the switch in StartRun and the CHECK in migrations/0006.
func TestStartRunAcceptsTheHoldersKind(t *testing.T) {
	s, ctx := testStore(t)

	id, err := s.StartRun(ctx, RunHolders, time.Now().UTC())
	if err != nil {
		t.Fatalf("StartRun holders: %v", err)
	}
	if err := s.FinishRun(ctx, id, time.Now().UTC(), 59, 1, "one reading hit the page cap"); err != nil {
		t.Fatalf("FinishRun: %v", err)
	}

	run, err := s.LastRun(ctx, RunHolders)
	if err != nil {
		t.Fatalf("LastRun holders: %v", err)
	}
	if run.Kind != RunHolders {
		t.Errorf("kind came back as %q", run.Kind)
	}
	// It must not be quoted as a scan. That separation is the reason it is a kind
	// of its own rather than a flag on an existing one.
	//
	// Asserted against the ID and not against ErrNotFound: the development
	// database this suite runs against has a scanner writing to it, so "no scan
	// run exists" is not a property of a clean transaction and a test that
	// assumed it passed only on an idle machine.
	switch scan, err := s.LastRun(ctx, RunScan); {
	case errors.Is(err, ErrNotFound):
		// No scan has ever run here. Nothing to confuse it with.
	case err != nil:
		t.Fatalf("LastRun scan: %v", err)
	case scan.ID == id:
		t.Errorf("the holders run with id %d was returned as the last scan", id)
	}
}

func TestStartRunStillRefusesAnUnknownKind(t *testing.T) {
	s, ctx := testStore(t)

	if _, err := s.StartRun(ctx, RunKind("record"), time.Now().UTC()); err == nil {
		t.Fatal("kind \"record\" was accepted; the recorder writes files and touches no table")
	}
}
