package main

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/Keel-Official/keel-backend/internal/domain"
	"github.com/Keel-Official/keel-backend/internal/store"
)

func TestTradesIn30DaysReadsTheCountTheDecisionRecordCountsFrom(t *testing.T) {
	// The real note for XLM/USDC, verbatim from configs/demonstration-set.json.
	// DEC-019 section 2 prices the whole set from this field, so the gate has to
	// read the same figure out of the same sentence.
	const note = "bucket deep. USDC pooled 2284832.6381290, book 200 bids and 200 asks, " +
		"spread 0.13 percent, 1934524 trades in 30 days, authorized trustlines n/a, native. " +
		"Measured on Horizon 26 August 2026."

	n, ok := tradesIn30Days(note)
	if !ok {
		t.Fatal("the count was not found in a real selection note")
	}
	if n != 1934524 {
		t.Errorf("count = %d, want 1934524", n)
	}
}

func TestTradesIn30DaysReportsAMissingCountRatherThanGuessingZero(t *testing.T) {
	// Zero would put the pair UNDER any threshold and buy it the expensive walk,
	// which is the opposite of what not knowing should cost.
	for _, note := range []string{"", "bucket deep. no counts here", "trades in 30 days"} {
		if n, ok := tradesIn30Days(note); ok {
			t.Errorf("note %q yielded %d, want no count", note, n)
		}
	}
}

// Every pair in the shipped demonstration set is either counted or is the one
// known exception, so the gate is not silently falling through to "unknown" for
// the whole set.
func TestTheShippedDemonstrationSetCarriesItsTradeCounts(t *testing.T) {
	body, err := os.ReadFile("../../configs/demonstration-set.json")
	if err != nil {
		t.Skipf("demonstration set not readable: %v", err)
	}
	var set struct {
		Pairs []struct {
			Base struct{ Code string } `json:"base"`
			Note string                `json:"note"`
		} `json:"pairs"`
	}
	if err := json.Unmarshal(body, &set); err != nil {
		t.Fatalf("decoding the demonstration set: %v", err)
	}
	if len(set.Pairs) == 0 {
		t.Fatal("the demonstration set is empty")
	}

	var counted, missing int
	for _, p := range set.Pairs {
		if _, ok := tradesIn30Days(p.Note); ok {
			counted++
		} else {
			missing++
		}
	}
	// DEC-019 section 2 says 60 of the 61 pairs carry one.
	if counted < 60 {
		t.Errorf("%d pair(s) carry a 30 day trade count, want at least 60 (%d without)", counted, missing)
	}
}

func TestFlattenDaysAscendingUndoesTheBackwardWalk(t *testing.T) {
	// The walk hands days over newest first while each day's own trades are
	// already ascending, so the flattening has to reverse one level and not the
	// other.
	mk := func(s string) domain.Trade {
		at, err := time.Parse(time.RFC3339, s)
		if err != nil {
			t.Fatalf("parsing %q: %v", s, err)
		}
		return domain.Trade{ID: s, ClosedAt: at}
	}
	days := [][]domain.Trade{
		{mk("2026-02-21T01:00:00Z"), mk("2026-02-21T20:00:00Z")},
		{mk("2026-02-20T02:00:00Z"), mk("2026-02-20T22:00:00Z")},
	}

	got := flattenDaysAscending(days)
	if len(got) != 4 {
		t.Fatalf("want 4 trades, got %d", len(got))
	}
	for i := 1; i < len(got); i++ {
		if !got[i-1].ClosedAt.Before(got[i].ClosedAt) {
			t.Fatalf("not ascending at %d: %s then %s", i, got[i-1].ClosedAt, got[i].ClosedAt)
		}
	}
}

func TestStartOfUTCDayIsMidnightAndNotAnOffsetFromTheZeroTime(t *testing.T) {
	in := time.Date(2026, 2, 22, 18, 45, 3, 99, time.FixedZone("WIB", 7*3600))
	got := startOfUTCDay(in)
	want := time.Date(2026, 2, 22, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("startOfUTCDay(%s) = %s, want %s", in, got, want)
	}
	if got.Location() != time.UTC {
		t.Errorf("location = %s, want UTC", got.Location())
	}
}

func TestTradeTargetsSelectsEveryPairSharingABaseAsset(t *testing.T) {
	// A trade stream belongs to a PAIR, so USTRY against USDC and USTRY against
	// XLM are two targets and not one.
	ustry := domain.Asset{Code: "USTRY", Issuer: "GCRY", Type: domain.AssetTypeAlphanum12}
	usdc := domain.Asset{Code: "USDC", Issuer: "GA5Z", Type: domain.AssetTypeAlphanum4}
	xlm := domain.Asset{Type: domain.AssetTypeNative}
	other := domain.Asset{Code: "EURC", Issuer: "GDHU", Type: domain.AssetTypeAlphanum4}

	rows := []store.Asset{
		{ID: 1, Base: ustry, Quote: usdc},
		{ID: 2, Base: ustry, Quote: xlm},
		{ID: 3, Base: other, Quote: usdc},
	}

	all, err := tradeTargets(rows, "")
	if err != nil || len(all) != 3 {
		t.Fatalf("empty selector = %d target(s), %v; want all 3", len(all), err)
	}

	one, err := tradeTargets(rows, ustry.String())
	if err != nil {
		t.Fatalf("selecting %s: %v", ustry, err)
	}
	if len(one) != 2 {
		t.Fatalf("want both USTRY pairs, got %d", len(one))
	}

	if _, err := tradeTargets(rows, "NOPE:GNOPE"); err == nil {
		t.Error("an unknown base asset must be an error rather than an empty pass")
	}
}

func TestNextTradesPassPinsTheDailyCadenceAfterMidnight(t *testing.T) {
	at := func(s string) time.Time {
		v, err := time.Parse(time.RFC3339, s)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	for _, tc := range []struct {
		name     string
		now      string
		interval time.Duration
		align    time.Duration
		want     string
	}{
		// The deploy of 21 September 2026 started the service at 17:00Z. Pinned,
		// the next pass is 00:05Z, not 17:00Z the next day.
		{"after a late start", "2026-09-21T17:25:00Z", 24 * time.Hour, 5 * time.Minute, "2026-09-22T00:05:00Z"},
		// A pass that finished just after the slot waits for tomorrow's.
		{"just past the slot", "2026-09-22T00:31:00Z", 24 * time.Hour, 5 * time.Minute, "2026-09-23T00:05:00Z"},
		// Before today's slot, today's slot.
		{"before the slot", "2026-09-22T00:01:00Z", 24 * time.Hour, 5 * time.Minute, "2026-09-22T00:05:00Z"},
		// Any other cadence is untouched.
		{"hourly stays relative", "2026-09-22T10:10:00Z", time.Hour, 5 * time.Minute, "2026-09-22T11:10:00Z"},
		// And a negative offset opts out.
		{"opted out", "2026-09-21T17:25:00Z", 24 * time.Hour, -1, "2026-09-22T17:25:00Z"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := nextTradesPass(at(tc.now), tc.interval, tc.align)
			if !got.Equal(at(tc.want)) {
				t.Errorf("next = %s, want %s", got.Format(time.RFC3339), tc.want)
			}
		})
	}
}

// The property the alignment exists for: whenever the process starts, no reading
// outlives scan's 36 hour bound before its replacement lands, allowing a pass an
// hour to finish.
func TestAPinnedPassNeverLetsAReadingAgePastTheScanBound(t *testing.T) {
	const bound = 36 * time.Hour
	day := time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)
	for h := 0; h < 24; h++ {
		start := day.Add(time.Duration(h) * time.Hour)
		anchor := day // the first pass, at start, is anchored at that day's midnight
		finished := start.Add(time.Hour)
		next := nextTradesPass(finished, 24*time.Hour, 5*time.Minute)
		replaced := next.Add(time.Hour)
		if age := replaced.Sub(anchor); age > bound {
			t.Errorf("started %s: the reading anchored %s is %s old when replaced, past %s",
				start.Format("15:04"), anchor.Format("Jan 2"), age, bound)
		}
	}
}
