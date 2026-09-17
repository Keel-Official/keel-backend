package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Keel-Official/keel-backend/internal/conformance"
	"github.com/Keel-Official/keel-backend/internal/domain"
	"github.com/Keel-Official/keel-backend/internal/horizon"
	"github.com/Keel-Official/keel-backend/internal/store"
)

// Only the database boundary is substituted. The command's validation and the
// domain computation run normally, against the existing independent fixture.
type replayStoreProbe struct {
	risk       domain.AssetRisk
	computedAt time.Time
	writes     int
	err        error
	inserted   bool
}

func (s *replayStoreProbe) AssetID(context.Context, domain.Asset, domain.Asset) (int, error) {
	return 7, s.err
}

func (s *replayStoreProbe) SaveMetrics(_ context.Context, assetID int, at time.Time, risk domain.AssetRisk) (int64, bool, error) {
	if assetID != 7 {
		return 0, false, errors.New("wrong asset identity")
	}
	s.writes++
	s.risk, s.computedAt = risk, at
	return 42, s.inserted, s.err
}

func replayForPersistence() horizon.ReplayResult {
	s := conformance.GoldenSnapshot()
	// Controlled no-pool input for plumbing tests, not a claim about USTRY.
	s.Pools = []domain.PoolReserves{}
	s.LedgerSeq = 61340262
	s.LedgerClosedAt = time.Date(2026, 2, 22, 0, 10, 15, 0, time.UTC)
	return horizon.ReplayResult{Snapshot: s, ReadAt: time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC)}
}

func TestPersistReplayPreservesProvenanceAndComputesTheExistingFixture(t *testing.T) {
	s := &replayStoreProbe{inserted: true}
	id, inserted, err := persistReplay(context.Background(), s, replayForPersistence(), false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if id != 42 || !inserted || s.writes != 1 {
		t.Fatalf("write = %d, %v, %d calls", id, inserted, s.writes)
	}
	if s.risk.LedgerSeq != 61340262 || s.risk.DataSource != domain.DataSourceOffersImplied || s.risk.MethodologyVersion != domain.MethodologyVersion {
		t.Fatalf("lost provenance: %+v", s.risk)
	}
	if !s.risk.LedgerClosedAt.Equal(replayForPersistence().Snapshot.LedgerClosedAt) || !s.computedAt.Equal(replayForPersistence().ReadAt) {
		t.Fatal("ledger time and read time must remain distinct")
	}
	if s.risk.MidPrice == nil || s.risk.MidPrice.String() != "53.8971414" {
		t.Fatalf("fixture price = %v", s.risk.MidPrice)
	}
}

func TestPersistReplayRefusesDetectedGapsBeforeWriting(t *testing.T) {
	cases := []struct {
		name   string
		change func(*horizon.ReplayResult)
	}{
		{"crossed diagnostic", func(r *horizon.ReplayResult) { r.Crossed = true }},
		{"crossed book", func(r *horizon.ReplayResult) { r.Snapshot.Book.Bids[0].Price = domain.Price{N: 107, D: 1} }},
		{"missing offers", func(r *horizon.ReplayResult) { r.MissingOfferIDs = []int64{1} }},
		{"truncated", func(r *horizon.ReplayResult) { r.Truncated = 1 }},
		{"failed", func(r *horizon.ReplayResult) { r.Failed = 1 }},
		{"unsizable", func(r *horizon.ReplayResult) { r.Unsizable = 1 }},
		{"floor", func(r *horizon.ReplayResult) { r.StoppedAtFloor = 1 }},
		{"inflated", func(r *horizon.ReplayResult) { r.EarliestOfferOp = 1; r.TradeWindowFrom = 2 }},
		{"wrong source", func(r *horizon.ReplayResult) { r.Snapshot.Source = domain.DataSourceHorizon }},
		{"missing ledger time", func(r *horizon.ReplayResult) { r.Snapshot.LedgerClosedAt = time.Time{} }},
		{"missing read time", func(r *horizon.ReplayResult) { r.ReadAt = time.Time{} }},
		{"unobserved pools", func(r *horizon.ReplayResult) { r.Snapshot.Pools = nil }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := replayForPersistence()
			tc.change(&r)
			s := &replayStoreProbe{}
			if _, _, err := persistReplay(context.Background(), s, r, false, 0); err == nil {
				t.Fatal("accepted a result with a known provenance or reconstruction gap")
			}
			if s.writes != 0 {
				t.Fatal("invalid result reached storage")
			}
		})
	}
}

func TestPersistReplayRequiresAnAlreadyDeclaredPair(t *testing.T) {
	s := &replayStoreProbe{err: store.ErrNotFound}
	if _, _, err := persistReplay(context.Background(), s, replayForPersistence(), false, 0); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("error = %v", err)
	}
	if s.writes != 0 {
		t.Fatal("wrote an undeclared pair")
	}
}

func TestPersistReplayReportsAnExistingRowWithoutClaimingAnInsert(t *testing.T) {
	s := &replayStoreProbe{inserted: false}
	_, inserted, err := persistReplay(context.Background(), s, replayForPersistence(), false, 0)
	if err != nil || inserted {
		t.Fatalf("duplicate = %v, %v", inserted, err)
	}
}

func TestReplayPoolEvidenceMustMatchIdentityLedgerSourceAndCoverage(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*domain.Snapshot)
	}{
		{"issuer", func(s *domain.Snapshot) { s.Base.Issuer = s.Quote.Issuer }},
		{"ledger", func(s *domain.Snapshot) { s.LedgerSeq++ }},
		{"source", func(s *domain.Snapshot) { s.Source = domain.DataSourceHorizon }},
		{"unknown pools", func(s *domain.Snapshot) { s.Pools = nil }},
		{"time", func(s *domain.Snapshot) { s.LedgerClosedAt = time.Time{} }},
		{"duplicate pools", func(s *domain.Snapshot) {
			s.Pools = []domain.PoolReserves{conformance.PoolUSTRYUSDC, conformance.PoolUSTRYUSDC}
		}},
		{"invalid pool", func(s *domain.Snapshot) { s.Pools = []domain.PoolReserves{{PoolID: "invalid"}} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := replayForPersistence().Snapshot
			tc.change(&s)
			want := replayForPersistence().Snapshot
			if _, err := replayPoolSnapshot([]domain.Snapshot{s}, want.Base, want.Quote, want.LedgerSeq); err == nil {
				t.Fatal("accepted mismatched pool evidence")
			}
		})
	}
	s := replayForPersistence().Snapshot
	if _, err := replayPoolSnapshot([]domain.Snapshot{s, s}, s.Base, s.Quote, s.LedgerSeq); err == nil {
		t.Fatal("accepted ambiguous duplicate snapshots")
	}
	if _, err := replayPoolSnapshot([]domain.Snapshot{s}, s.Base, s.Quote, s.LedgerSeq); err != nil {
		t.Fatal(err)
	}
}

func TestReplayPoolEvidenceRejectsMalformedAndTrailingInput(t *testing.T) {
	for _, body := range []string{`[{"Poolz":[]}]`, `[] {}`, `[{`,
		`[{"Pools":[{"PoolID":"27480d0483c8320ba4a707797526ffd67118e841491e0cbeb66db697bb66cccb"}]}]`,
		`[{"Pools":[{"PoolID":"27480d0483c8320ba4a707797526ffd67118e841491e0cbeb66db697bb66cccb","ReserveBase":null,"ReserveQuote":"1","FeeBP":30}]}]`,
		`[{"Pools":[{"PoolID":"27480d0483c8320ba4a707797526ffd67118e841491e0cbeb66db697bb66cccb","ReserveBase":"1","ReserveQuote":"1"}]}]`,
	} {
		path := filepath.Join(t.TempDir(), "pools.json")
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := readReplayPoolSnapshots(path); err == nil {
			t.Fatalf("accepted %s", body)
		}
	}
}

func TestReplayPersistRequiresPoolEvidenceBeforeOpeningTheDatabase(t *testing.T) {
	err := runReplay([]string{"-pairs", "../../scripts/record-pairs.example.json", "-ledger", "61340262", "-persist", "-dsn", "invalid"})
	if err == nil || !strings.Contains(err.Error(), "-pool-snapshots") {
		t.Fatalf("error=%v", err)
	}
}

func TestReplayPoolEvidencePreservesExplicitValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pools.json")
	body := `[{"Pools":[{"PoolID":"27480d0483c8320ba4a707797526ffd67118e841491e0cbeb66db697bb66cccb","ReserveBase":"15.4791416","ReserveQuote":"16.3389179","FeeBP":30}]}]`
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	snapshots, err := readReplayPoolSnapshots(path)
	if err != nil {
		t.Fatal(err)
	}
	pool := snapshots[0].Pools[0]
	if pool.ReserveBase.String() != "15.4791416" || pool.ReserveQuote.String() != "16.3389179" || pool.FeeBP != 30 {
		t.Fatalf("pool input changed: %+v", pool)
	}
	body = strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(body, "15.4791416", "0"), "16.3389179", "0"), "\"FeeBP\":30", "\"FeeBP\":0")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readReplayPoolSnapshots(path); err != nil {
		t.Fatalf("explicit zeros are different from missing values: %v", err)
	}
}

// ---------------------------------------------------------------- DEC-022

// The two refusals that survive -accept-incomplete. Registered in DEC-022
// section 7 items 2 and 3, before any of this code was written, so that passing
// could not be defined afterwards.
//
// They are separated from the table above rather than folded into it because the
// table proves a DEFAULT and this proves a RULE: no flag combination reaches the
// store with either of these, which is the whole asymmetry the decision rests on.
func TestPersistReplayStillRefusesCrossedAndInflatedUnderAcceptIncomplete(t *testing.T) {
	cases := []struct {
		name   string
		change func(*horizon.ReplayResult)
		want   string
	}{
		{"crossed diagnostic", func(r *horizon.ReplayResult) { r.Crossed = true }, "crossed"},
		{"inflated", func(r *horizon.ReplayResult) { r.EarliestOfferOp = 1; r.TradeWindowFrom = 2 }, "inflated"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := replayForPersistence()
			tc.change(&r)
			s := &replayStoreProbe{}
			_, _, err := persistReplay(context.Background(), s, r, true, 0)
			if err == nil {
				t.Fatal("-accept-incomplete must not override this refusal")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error does not name the reason: %v", err)
			}
			if s.writes != 0 {
				t.Fatal("refused result reached storage")
			}
		})
	}
}

// DEC-022 section 7 item 4: every nonzero counter reaches the stored row, in the
// field and in the prose, and the stored numbers are the ones the walk reported.
func TestPersistReplayRecordsEveryGapItWasAskedToAccept(t *testing.T) {
	r := replayForPersistence()
	r.Truncated = 3
	r.Failed = 1
	r.StoppedAtFloor = 42
	r.Unsizable = 2
	r.MissingOfferIDs = []int64{1822775941}
	r.Accounts = make([]horizon.AccountWalk, 65)

	s := &replayStoreProbe{inserted: true}
	if _, _, err := persistReplay(context.Background(), s, r, true, 61300000); err != nil {
		t.Fatal(err)
	}
	rec := s.risk.Reconstruction
	if rec == nil {
		t.Fatal("a stored reconstruction must carry its walk diagnostics")
	}
	if rec.Truncated != 3 || rec.Failed != 1 || rec.StoppedAtFloor != 42 ||
		rec.Unsizable != 2 || rec.MissingOffers != 1 ||
		rec.FloorLedger != 61300000 || rec.AccountsWalked != 65 {
		t.Fatalf("counters lost on the way to the row: %+v", rec)
	}

	joined := strings.Join(s.risk.Warnings, "\n")
	for _, want := range []string{"42 of 65", "3 of 65", "1 of 65", "2 operation result(s)", "1 offer(s)", "61300000", "too THIN"} {
		if !strings.Contains(joined, want) {
			t.Errorf("warnings do not account for %q:\n%s", want, joined)
		}
	}
}

// A clean walk still carries the field, because nil means "not a reconstruction"
// and a row that lost the distinction is indistinguishable from a live read.
func TestPersistReplayMarksACleanWalkAsAWalkRatherThanAsALiveRead(t *testing.T) {
	s := &replayStoreProbe{inserted: true}
	if _, _, err := persistReplay(context.Background(), s, replayForPersistence(), false, 0); err != nil {
		t.Fatal(err)
	}
	if s.risk.Reconstruction == nil {
		t.Fatal("an offers-implied row must say that it is one, even with no gaps")
	}
	if s.risk.Reconstruction.HasGaps() {
		t.Fatalf("invented a gap: %+v", s.risk.Reconstruction)
	}
	joined := strings.Join(s.risk.Warnings, "\n")
	if !strings.Contains(joined, "not proof") {
		t.Fatalf("a clean walk must not be reported as proof that it saw every offer:\n%s", joined)
	}
}

// A floor that the writer was never told about would be stored as "stopped at
// ledger 0", which reads as a fact and is not one.
func TestPersistReplayRefusesAFloorCountWithoutItsFloorLedger(t *testing.T) {
	r := replayForPersistence()
	r.StoppedAtFloor = 4
	s := &replayStoreProbe{}
	if _, _, err := persistReplay(context.Background(), s, r, true, 0); err == nil || s.writes != 0 {
		t.Fatalf("stored an unreadable floor: err=%v writes=%d", err, s.writes)
	}
}
