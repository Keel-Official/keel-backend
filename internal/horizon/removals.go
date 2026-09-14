// Offers that left the book without emitting either event this reconstruction is
// built from.
//
// WHY THIS FILE HAS TO EXIST AT ALL. replay.go folds two event streams, offer
// operations and trades, and an offer leaves the state only when one of them says
// so. On 12 September 2026 an offer was proven to have left without either:
// `1822775941`, one stroop of USTRY, last filled at ledger 61143619 and provably
// absent by ledger 61143682. Horizon's per-offer trade index returns exactly two
// trades for it, both before that window, a forward walk of its owner's
// operations reaches past the control ledger without naming it again, and the
// issuer performed no authorization or clawback operation in the range. The
// measurement, the elimination and the candidate mechanism are in
// docs/evidences/2026-09-12-crossed-book-ustry-february.md. The consequence is
// that the fold carried a phantom ask for twenty days of February 2026, which is
// wrong on thirteen rows that look fine and crossed on six that do not.
//
// A DEEPER WALK CANNOT FIX IT AND NEITHER CAN A WIDER TRADE WINDOW, which is the
// sentence that decides this file's shape. There is no page to find. The only
// repair available to an event-sourced reconstruction is to be TOLD, by a reading
// taken outside the two streams, that an offer was gone by a given ledger.
//
// THE THREE SENTENCES THIS ZONE ASKS FOR.
//
// The decision: a known removal is DATA, carried in a file beside the pair lists,
// naming the offer, the ledger by which it is proven absent and the evidence
// document that proves it, and it is applied as a synthetic delete at the START of
// that ledger rather than by any inference the fold makes for itself.
//
// The alternative rejected: letting the fold repair a crossed book on its own by
// dropping whichever level crosses.
//
// Why it was rejected: nothing in a book alone can say which of the two crossing
// offers is the phantom, and the 12 September reading had to walk each side
// separately to decide it, so an automatic dropper would be guessing half the
// time; worse, it would repair only the six rows that announce themselves and
// leave the thirteen quiet wrong ones exactly as they are, while telling the
// reader the run is clean. A repair that silently picks a side is how a
// reconstruction stops being evidence.
//
// WHAT THIS IS NOT. It is not a general fix for the class of defect. Every entry
// here is one offer that somebody proved gone by hand, and a class of event the
// fold cannot see stays unseen for every offer nobody looked at. DEC-021 is the
// record that asks whether the historical path may go on calling itself
// event-sourced reconstruction with this file in it, and that question is Al's.

package horizon

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

// KnownRemoval is one offer proven to have left the book, and the proof.
//
// Evidence is required and is not decoration: an entry without a document behind
// it is somebody's opinion about the past written into a file the reconstruction
// obeys, which is the one thing this mechanism must never become.
type KnownRemoval struct {
	// OfferID is the offer, as Horizon reports it.
	OfferID int64 `json:"offer_id"`

	// GoneByLedger is the EARLIEST ledger at which the offer is proven absent. A
	// fold at a target at or above it will not carry the offer; a fold below it is
	// untouched, because the removal happened somewhere in a window and this is
	// only its upper bound.
	GoneByLedger uint32 `json:"gone_by_ledger"`

	// Evidence is the repository path of the document that proves it.
	Evidence string `json:"evidence"`

	// Why is one sentence a reader of the sidecar can understand without opening
	// the evidence document.
	Why string `json:"why"`
}

// knownRemovalFile is the on-disk shape. The note is carried so that anybody who
// opens the file reads what it is for before reading what is in it.
type knownRemovalFile struct {
	Note     string         `json:"note"`
	Removals []KnownRemoval `json:"removals"`
}

// LoadKnownRemovals reads a removal list.
//
// It refuses an entry missing any of its four fields. A removal whose evidence is
// blank cannot be checked by a reader, and a removal at ledger zero would delete
// the offer from every target in a series, which is the whole point of the file
// inverted.
func LoadKnownRemovals(path string) ([]KnownRemoval, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("known removals: %w", err)
	}
	var f knownRemovalFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("known removals %s: %w", path, err)
	}
	seen := map[int64]bool{}
	for i, r := range f.Removals {
		switch {
		case r.OfferID == 0:
			return nil, fmt.Errorf("known removals %s: entry %d has no offer_id", path, i)
		case r.GoneByLedger == 0:
			return nil, fmt.Errorf("known removals %s: offer %d has no gone_by_ledger, which would remove it from every target", path, r.OfferID)
		case r.Evidence == "":
			return nil, fmt.Errorf("known removals %s: offer %d has no evidence, and an unprovable removal is not admissible", path, r.OfferID)
		case r.Why == "":
			return nil, fmt.Errorf("known removals %s: offer %d has no why", path, r.OfferID)
		case seen[r.OfferID]:
			return nil, fmt.Errorf("known removals %s: offer %d is named twice", path, r.OfferID)
		}
		seen[r.OfferID] = true
	}
	out := append([]KnownRemoval{}, f.Removals...)
	sort.Slice(out, func(i, j int) bool { return out[i].OfferID < out[j].OfferID })
	return out, nil
}

// appliesAt reports whether this removal bites at a given target ledger.
func (r KnownRemoval) appliesAt(target uint32) bool { return target >= r.GoneByLedger }
