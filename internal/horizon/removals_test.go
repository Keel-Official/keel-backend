package horizon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The ask that built the golden fixture's book is the one real offer these tests
// can remove, because its create is in this package's test data already. The
// PHANTOM the mechanism exists for, offer 1822775941, is not here: its operations
// are on another account in another week and fetching them would put a network
// call in a unit test. What is tested here is the mechanism, and the phantom is
// tested by the acceptance run recorded in DEC-021 section 5.
const fixtureAskOfferID = int64(1824788980)

func TestARemovalAtOrAboveItsLedgerTakesTheOfferOffTheBook(t *testing.T) {
	ops := theTwoCreates(t)
	removals := []KnownRemoval{{
		OfferID:      fixtureAskOfferID,
		GoneByLedger: 61340000,
		Evidence:     "docs/evidences/2026-09-12-crossed-book-ustry-february.md",
		Why:          "a test, not a reading",
	}}

	state := replayOffers(ops, nil, 61340262, removals)
	if _, ok := state[fixtureAskOfferID]; ok {
		t.Fatalf("offer %d is still resting at a target above its gone_by_ledger", fixtureAskOfferID)
	}
	// The bid is untouched, because a removal names one offer and not a book.
	if len(state) != 1 {
		t.Fatalf("state holds %d offer(s), want only the bid", len(state))
	}
}

func TestARemovalBelowItsLedgerChangesNothing(t *testing.T) {
	ops := theTwoCreates(t)
	removals := []KnownRemoval{{
		OfferID:      fixtureAskOfferID,
		GoneByLedger: 61340262,
		Evidence:     "docs/evidences/2026-09-12-crossed-book-ustry-february.md",
		Why:          "a test, not a reading",
	}}

	// One ledger below the proof. The offer was still there as far as anybody
	// can show, so the fold must carry it.
	state := replayOffers(ops, nil, 61340261, removals)
	if _, ok := state[fixtureAskOfferID]; !ok {
		t.Fatalf("offer %d was removed at a target BELOW its gone_by_ledger", fixtureAskOfferID)
	}
}

// A removal deletes what was resting and must not delete a later re-creation.
// This is the case that decides synthetic-event-in-order against filter-the-
// result, and the two disagree only here.
func TestARemovalDoesNotDeleteAnOfferCreatedAfterIt(t *testing.T) {
	ops := theTwoCreates(t)
	removals := []KnownRemoval{{
		OfferID:      fixtureAskOfferID,
		GoneByLedger: 61339900, // before the ask's own create at 61339940
		Evidence:     "docs/evidences/2026-09-12-crossed-book-ustry-february.md",
		Why:          "a test, not a reading",
	}}

	state := replayOffers(ops, nil, 61340262, removals)
	if _, ok := state[fixtureAskOfferID]; !ok {
		t.Fatalf("offer %d was created AFTER the removal and must be resting", fixtureAskOfferID)
	}
}

// A removal that was eligible but took nothing off the book must NOT be reported
// as applied. This is the case that caused a misreading on 14 September 2026: a
// control run at a shallow operation floor declared the phantom removed when the
// phantom's create was below the floor and the book never held it.
func TestARemovalThatRemovedNothingIsNotReportedAsApplied(t *testing.T) {
	ops := theTwoCreates(t)
	removals := []KnownRemoval{{
		OfferID:      999999999, // an offer this walk never saw
		GoneByLedger: 61340000,
		Evidence:     "docs/evidences/2026-09-12-crossed-book-ustry-february.md",
		Why:          "a test, not a reading",
	}}

	_, applied := replayOffersReporting(ops, nil, 61340262, removals)
	if len(applied) != 0 {
		t.Fatalf("applied = %v, want nothing: the offer was never on this book", applied)
	}
}

func TestARemovalThatRemovedSomethingIsReported(t *testing.T) {
	ops := theTwoCreates(t)
	removals := []KnownRemoval{{
		OfferID:      fixtureAskOfferID,
		GoneByLedger: 61340000,
		Evidence:     "docs/evidences/2026-09-12-crossed-book-ustry-february.md",
		Why:          "a test, not a reading",
	}}

	_, applied := replayOffersReporting(ops, nil, 61340262, removals)
	if len(applied) != 1 || applied[0] != fixtureAskOfferID {
		t.Fatalf("applied = %v, want exactly offer %d", applied, fixtureAskOfferID)
	}
}

func TestTheRepositoryRemovalListLoadsAndNamesItsEvidence(t *testing.T) {
	path := filepath.Join("..", "..", "configs", "known-removals.json")
	got, err := LoadKnownRemovals(path)
	if err != nil {
		t.Fatalf("loading %s: %v", path, err)
	}
	if len(got) == 0 {
		t.Fatal("the list is empty, so the February repair would silently do nothing")
	}
	for _, r := range got {
		if _, err := os.Stat(filepath.Join("..", "..", r.Evidence)); err != nil {
			t.Errorf("offer %d cites %s, which is not in the repository: %v", r.OfferID, r.Evidence, err)
		}
	}
}

func TestAnEntryMissingItsProofIsRefused(t *testing.T) {
	cases := map[string]string{
		"no offer_id":       `{"removals":[{"gone_by_ledger":1,"evidence":"e","why":"w"}]}`,
		"no gone_by_ledger": `{"removals":[{"offer_id":1,"evidence":"e","why":"w"}]}`,
		"no evidence":       `{"removals":[{"offer_id":1,"gone_by_ledger":1,"why":"w"}]}`,
		"no why":            `{"removals":[{"offer_id":1,"gone_by_ledger":1,"evidence":"e"}]}`,
		"named twice":       `{"removals":[{"offer_id":1,"gone_by_ledger":1,"evidence":"e","why":"w"},{"offer_id":1,"gone_by_ledger":2,"evidence":"e","why":"w"}]}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "removals.json")
			if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadKnownRemovals(p); err == nil {
				t.Fatalf("%s was accepted", name)
			} else if !strings.Contains(err.Error(), "known removals") {
				t.Fatalf("error does not name the file kind: %v", err)
			}
		})
	}
}
