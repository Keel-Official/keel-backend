package horizon

import (
	"context"
	"testing"

	"github.com/Keel-Official/keel-backend/internal/domain"
	"github.com/shopspring/decimal"
)

// THE CLAIM THIS FILE EXISTS TO TEST. A series folds ONE event list at many
// targets, and each point must be what a separate reconstruction at that target
// would have produced. If that is not true, the series is a cheaper way of being
// wrong twenty-eight times.
//
// The numbers below are the golden fixture's, which are RED and were computed by
// hand before any of this existed. That is on purpose: a test whose expected
// values came from this side of the wall would prove that the fold is
// self-consistent and nothing else.

// TestOneEventListServesTwoTargets is the whole design in one assertion. The same
// operations and the same trade, folded at two ledgers one apart, give the book
// before the manipulation and the book after it.
func TestOneEventListServesTwoTargets(t *testing.T) {
	ops := theTwoCreates(t)
	trades := []domain.Trade{theManipulation()}

	before := bookFromOffers(replayOffers(ops, trades, 61340262), testUSTRY, testUSDC)
	after := bookFromOffers(replayOffers(ops, trades, 61340263), testUSTRY, testUSDC)

	if len(before.Asks) != 1 || len(after.Asks) != 1 {
		t.Fatalf("asks before = %d, after = %d, want 1 and 1", len(before.Asks), len(after.Asks))
	}
	if want := decimal.RequireFromString("1.2185312"); !before.Asks[0].Amount.Equal(want) {
		t.Errorf("ask before the manipulation = %s, want %s", before.Asks[0].Amount, want)
	}
	if want := decimal.RequireFromString("1.1684309"); !after.Asks[0].Amount.Equal(want) {
		t.Errorf("ask after the manipulation = %s, want %s", after.Asks[0].Amount, want)
	}

	// AND THE EARLIER FOLD MUST NOT HAVE BEEN DISTURBED BY THE LATER ONE. Both
	// books come out of the same slices, and replayOffers takes pointers into
	// them when it builds its event list. A fold that mutated an operation or a
	// trade in place would leave the second call reading something the ledger
	// never contained, and the failure would look like a market that moved.
	again := bookFromOffers(replayOffers(ops, trades, 61340262), testUSTRY, testUSDC)
	if !again.Asks[0].Amount.Equal(before.Asks[0].Amount) {
		t.Errorf("re-folding at the first target gave %s, want the unchanged %s",
			again.Asks[0].Amount, before.Asks[0].Amount)
	}
}

// TestAPointBelowEveryOperationIsAnEmptyBookAndNotAnError is the reason
// ReconstructSeries refuses a floor above its earliest target. This is what such
// a point actually produces, so the refusal upstream is the only thing standing
// between a configuration mistake and this repository's loudest finding.
func TestAPointBelowEveryOperationIsAnEmptyBookAndNotAnError(t *testing.T) {
	ops := theTwoCreates(t)
	book := bookFromOffers(replayOffers(ops, nil, 61000000), testUSTRY, testUSDC)
	if len(book.Asks) != 0 || len(book.Bids) != 0 {
		t.Fatalf("book has %d ask(s) and %d bid(s), want an empty book", len(book.Asks), len(book.Bids))
	}
}

func TestTheFloorAboveTheEarliestTargetIsRefused(t *testing.T) {
	c := NewClient(Config{})
	_, err := c.ReconstructSeries(context.Background(), testUSTRY, testUSDC, SeriesQuery{
		Targets:     []uint32{61027032, 61340262},
		SinceLedger: 61100000,
	})
	if err == nil {
		t.Fatal("a floor above the earliest target was accepted, and that point would report an empty book")
	}
}

func TestASeriesWithNoTargetIsRefused(t *testing.T) {
	c := NewClient(Config{})
	if _, err := c.ReconstructSeries(context.Background(), testUSTRY, testUSDC, SeriesQuery{}); err == nil {
		t.Fatal("a series with no target was accepted")
	}
}

// TestTheTargetsAreSortedAndDeduped guards NFR-9 for a series: two runs over the
// same list, in any order, produce the same points in the same order.
func TestTheTargetsAreSortedAndDeduped(t *testing.T) {
	got := dedupeSorted([]uint32{61340262, 61027032, 61340262, 61100000, 61027032})
	want := []uint32{61027032, 61100000, 61340262}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

// TestDedupeDoesNotDisturbTheCallersSlice matters because the caller builds the
// target list from a CSV and may report it afterwards. Sorting in place would
// reorder what it prints without saying so.
func TestDedupeDoesNotDisturbTheCallersSlice(t *testing.T) {
	in := []uint32{3, 1, 2}
	_ = dedupeSorted(in)
	if in[0] != 3 || in[1] != 1 || in[2] != 2 {
		t.Errorf("the caller's slice became %v, want it untouched", in)
	}
}
