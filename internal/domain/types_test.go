package domain

import "testing"

// TestCrossedBook covers the predicate that the February reconstruction needed
// and did not have. The cases are the four shapes a book can take, plus the two
// real price ratios from the pair that produced the defect, so that a reader can
// match the test against docs/evidences/2026-09-12-crossed-book-ustry-february.md
// rather than against invented numbers.
func TestCrossedBook(t *testing.T) {
	cases := []struct {
		name string
		book OrderBook
		want bool
	}{
		{
			name: "an ordinary book does not cross",
			book: OrderBook{
				Bids: []Level{level(99, 100, "10")},
				Asks: []Level{level(101, 100, "10")},
			},
			want: false,
		},
		{
			name: "an empty book does not cross",
			book: OrderBook{},
			want: false,
		},
		{
			name: "one side alone does not cross",
			book: OrderBook{Bids: []Level{level(99, 100, "10")}},
			want: false,
		},
		{
			name: "equal prices DO cross, because a bid at the ask executes",
			book: OrderBook{
				Bids: []Level{level(100, 100, "10")},
				Asks: []Level{level(100, 100, "10")},
			},
			want: true,
		},
		{
			// The 22 February row of the cap400 series: bid 1824767559 held by
			// GABFRFPY against the one stroop left on ask 1822775941 held by
			// GBPFB6XN. Both ratios are the price_r Horizon reports.
			name: "the USTRY row of 22 February 2026 crosses",
			book: OrderBook{
				Bids: []Level{level(2125646195, 2010206197, "0.9447626")},
				Asks: []Level{level(1981860307, 1874295956, "0.0000001")},
			},
			want: true,
		},
		{
			// The same ask against the control ledger's real best bid of 1.057,
			// which is the fixture's bid. This one is a possible book.
			name: "the control ledger's own bid does not cross that ask",
			book: OrderBook{
				Bids: []Level{level(1057, 1000, "0.0001")},
				Asks: []Level{level(1981860307, 1874295956, "0.0000001")},
			},
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bid, ask, got := tc.book.Crossed()
			if got != tc.want {
				t.Fatalf("Crossed() = %v, want %v", got, tc.want)
			}
			if !got {
				// A book that does not cross must report zero levels rather than
				// the best of each side, so that a caller cannot print a level
				// beside a false verdict.
				if bid.Price.Valid() || ask.Price.Valid() {
					t.Fatalf("Crossed() returned levels beside a false verdict: bid=%v ask=%v", bid.Price, ask.Price)
				}
				return
			}
			if !bid.Price.Valid() || !ask.Price.Valid() {
				t.Fatalf("Crossed() reported a cross without both levels: bid=%v ask=%v", bid.Price, ask.Price)
			}
			if bid.Price.Decimal().LessThan(ask.Price.Decimal()) {
				t.Fatalf("the returned bid %v is below the returned ask %v", bid.Price, ask.Price)
			}
		})
	}
}
