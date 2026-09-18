package horizon

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/Keel-Official/keel-backend/internal/domain"
)

// tradeAt builds one record on a given instant. The paging token carries a
// plausible TOID so the ledger falls out of it rather than being asserted.
func tradeAt(token, closedAt string) string {
	return tradeJSON(token, closedAt, 266843207, 2500000, "0.0501003", "5.3475699")
}

// descPages serves the records newest first, one page each, following the
// cursor the walk is given. It is the shape Horizon serves for order=desc.
func descPages(f *fakeHorizon, pages ...[]string) {
	f.handler["/trades"] = func(w http.ResponseWriter, r *http.Request) {
		i := 0
		if c := r.URL.Query().Get("cursor"); c != "" {
			_, _ = fmt.Sscanf(c, "%d", &i)
		}
		if i >= len(pages) {
			_, _ = fmt.Fprint(w, tradesPageJSON(""))
			return
		}
		next := ""
		if i+1 <= len(pages) {
			next = fmt.Sprintf("%s/trades?cursor=%d", f.srv.URL, i+1)
		}
		_, _ = fmt.Fprint(w, tradesPageJSON(next, pages[i]...))
	}
}

func collect(t *testing.T, c *Client, q TradeDayQuery) ([]TradeDay, TradeDayWalk) {
	t.Helper()
	var got []TradeDay
	walk, err := c.WalkTradeDays(context.Background(), testUSTRY, testUSDC, q, func(d TradeDay) (bool, error) {
		got = append(got, d)
		return true, nil
	})
	if err != nil {
		t.Fatalf("WalkTradeDays: %v", err)
	}
	return got, walk
}

func day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func TestWalkTradeDaysNeverEmitsTheAnchorsOwnDay(t *testing.T) {
	// The current UTC day is partial by definition, so its daily median is the
	// statistic DEC-019 section 8.2 refuses. Two trades sit inside it here and
	// neither may reach the caller.
	f := newFakeHorizon(t)
	descPages(f, []string{
		tradeAt("263454423513071617-0", "2026-02-22T18:00:00Z"),
		tradeAt("263454423513071616-0", "2026-02-22T00:10:21Z"),
		tradeAt("263454256009383937-1", "2026-02-21T09:00:00Z"),
	})
	c, _ := f.client()

	got, walk := collect(t, c, TradeDayQuery{
		Anchor:  time.Date(2026, 2, 22, 23, 59, 0, 0, time.UTC),
		MaxDays: 5,
	})

	if len(got) != 1 {
		t.Fatalf("want 1 day, got %d: %+v", len(got), got)
	}
	if !got[0].Start.Equal(day(2026, 2, 21)) {
		t.Errorf("day = %s, want 2026-02-21", got[0].Start)
	}
	if len(got[0].Trades) != 1 {
		t.Fatalf("want the one trade of the 21st, got %d", len(got[0].Trades))
	}
	if !walk.Exhausted {
		t.Error("walk ran off the end of the collection and should say so")
	}
}

func TestWalkTradeDaysReturnsTradesAscendingAlthoughItReadsThemBackwards(t *testing.T) {
	f := newFakeHorizon(t)
	descPages(f, []string{
		tradeAt("263454423513071617-0", "2026-02-21T20:00:00Z"),
		tradeAt("263454423513071616-0", "2026-02-21T09:00:00Z"),
		tradeAt("263454256009383937-1", "2026-02-20T09:00:00Z"),
	})
	c, _ := f.client()

	got, _ := collect(t, c, TradeDayQuery{Anchor: day(2026, 2, 22), MaxDays: 5})

	if len(got) == 0 || len(got[0].Trades) != 2 {
		t.Fatalf("want the 21st with two trades, got %+v", got)
	}
	first, second := got[0].Trades[0], got[0].Trades[1]
	if !first.ClosedAt.Before(second.ClosedAt) {
		t.Errorf("trades are not ascending: %s then %s", first.ClosedAt, second.ClosedAt)
	}
}

func TestWalkTradeDaysEmitsTheQuietDaysBetweenTwoTrades(t *testing.T) {
	// A day with no trades is evidence of quiet, not an absence of evidence, and
	// the caller counts calendar days against its bound.
	f := newFakeHorizon(t)
	descPages(f, []string{
		tradeAt("263454423513071617-0", "2026-02-21T09:00:00Z"),
		tradeAt("263454256009383937-1", "2026-02-18T09:00:00Z"),
	})
	c, _ := f.client()

	got, _ := collect(t, c, TradeDayQuery{Anchor: day(2026, 2, 22), MaxDays: 10})

	want := []time.Time{day(2026, 2, 21), day(2026, 2, 20), day(2026, 2, 19), day(2026, 2, 18)}
	if len(got) != len(want) {
		t.Fatalf("want %d days, got %d: %+v", len(want), len(got), got)
	}
	for i, w := range want {
		if !got[i].Start.Equal(w) {
			t.Errorf("day %d = %s, want %s", i, got[i].Start, w)
		}
	}
	if len(got[1].Trades) != 0 || len(got[2].Trades) != 0 {
		t.Error("the 20th and the 19th are quiet days and must carry no trades")
	}
	if len(got[0].Trades) != 1 || len(got[3].Trades) != 1 {
		t.Error("the 21st and the 18th each carry one trade")
	}
}

func TestWalkTradeDaysStopsAtMaxDaysAndDoesNotCallThatExhausted(t *testing.T) {
	// DEC-019 section 8.4 item 2: running out of budget and finding nothing are
	// different answers, and only one of them is a finding.
	f := newFakeHorizon(t)
	descPages(f, []string{
		tradeAt("263454423513071617-0", "2026-02-21T09:00:00Z"),
		tradeAt("263454256009383937-1", "2026-01-02T09:00:00Z"),
	})
	c, _ := f.client()

	got, walk := collect(t, c, TradeDayQuery{Anchor: day(2026, 2, 22), MaxDays: 3})

	if len(got) != 3 || walk.Days != 3 {
		t.Fatalf("want exactly 3 days, got %d (walk.Days=%d)", len(got), walk.Days)
	}
	if walk.Exhausted {
		t.Error("the bound was reached, the history was not: Exhausted must stay false")
	}
	if walk.Stopped {
		t.Error("the caller did not stop this walk")
	}
	if !walk.Oldest.Equal(day(2026, 2, 19)) {
		t.Errorf("oldest = %s, want 2026-02-19", walk.Oldest)
	}
}

func TestWalkTradeDaysLetsTheCallerStopOnTheDayItWanted(t *testing.T) {
	f := newFakeHorizon(t)
	descPages(f, []string{
		tradeAt("263454423513071617-0", "2026-02-21T09:00:00Z"),
		tradeAt("263454256009383937-1", "2026-02-20T09:00:00Z"),
	})
	c, _ := f.client()

	var seen int
	walk, err := c.WalkTradeDays(context.Background(), testUSTRY, testUSDC,
		TradeDayQuery{Anchor: day(2026, 2, 22), MaxDays: 30},
		func(d TradeDay) (bool, error) {
			seen++
			return len(d.Trades) == 0, nil
		})
	if err != nil {
		t.Fatalf("WalkTradeDays: %v", err)
	}
	if seen != 1 {
		t.Fatalf("want the walk to end on the first day with a trade, saw %d days", seen)
	}
	if !walk.Stopped {
		t.Error("Stopped must record that the caller ended it")
	}
}

func TestWalkTradeDaysHoldsBackTheOldestDayUnlessTheHistoryEnded(t *testing.T) {
	// The oldest day in hand is only complete when the collection ran out. This
	// fake keeps serving a next link and then an empty page, which is the end,
	// so the oldest day IS complete here and must arrive.
	f := newFakeHorizon(t)
	descPages(f,
		[]string{tradeAt("263454423513071617-0", "2026-02-21T09:00:00Z")},
		[]string{tradeAt("263454256009383937-1", "2026-02-21T01:00:00Z")},
	)
	c, _ := f.client()

	got, walk := collect(t, c, TradeDayQuery{Anchor: day(2026, 2, 22), MaxDays: 5})

	if !walk.Exhausted {
		t.Fatal("the collection ended, so the walk is exhausted")
	}
	if len(got) != 1 || len(got[0].Trades) != 2 {
		t.Fatalf("want one day carrying both trades, got %+v", got)
	}
	if walk.Pages < 2 {
		t.Errorf("want the walk to have followed the next link, pages = %d", walk.Pages)
	}
}

// THE PAGE BOUND IS WHAT STOPS ONE PAIR SPENDING A WHOLE PASS'S BUDGET, and the
// day bound cannot do it: a day costs one page for a quiet pair and 323 for XLM.
// HU/USDC spent about 2,100 requests of 3,000 on 18 September 2026 walking days
// that were each cheap in days and expensive in pages.
func TestWalkTradeDaysStopsOnThePageBoundAndSaysWhich(t *testing.T) {
	f := newFakeHorizon(t)
	// Four pages, all inside one very busy day, so no day is ever completed and
	// only the page bound can end this walk.
	descPages(f,
		[]string{tradeAt("263454423513071620-0", "2026-02-21T23:00:00Z")},
		[]string{tradeAt("263454423513071619-0", "2026-02-21T22:00:00Z")},
		[]string{tradeAt("263454423513071618-0", "2026-02-21T21:00:00Z")},
		[]string{tradeAt("263454423513071617-0", "2026-02-21T20:00:00Z")},
	)
	c, _ := f.client()

	got, walk := collect(t, c, TradeDayQuery{Anchor: day(2026, 2, 22), MaxDays: 30, MaxPages: 2})

	if walk.Pages != 2 {
		t.Errorf("pages = %d, want exactly the bound of 2", walk.Pages)
	}
	if !walk.PageCapReached {
		t.Error("PageCapReached must record that the page bound ended the walk")
	}
	if walk.Exhausted || walk.BoundReached {
		t.Error("neither the history nor the day bound ended this walk")
	}
	// The day in hand was never completed, so it must not be emitted: that is the
	// partial day the whole file refuses.
	if len(got) != 0 {
		t.Errorf("want no day emitted, got %d: %+v", len(got), got)
	}
}

func TestWalkTradeDaysTreatsANonPositivePageBoundAsNoBound(t *testing.T) {
	f := newFakeHorizon(t)
	descPages(f, []string{
		tradeAt("263454423513071617-0", "2026-02-21T09:00:00Z"),
		tradeAt("263454256009383937-1", "2026-02-20T09:00:00Z"),
	})
	c, _ := f.client()

	_, walk := collect(t, c, TradeDayQuery{Anchor: day(2026, 2, 22), MaxDays: 5, MaxPages: 0})
	if walk.PageCapReached {
		t.Error("a zero page bound is no bound and must never report itself as reached")
	}
	if !walk.Exhausted {
		t.Error("the walk should have run to the end of the collection")
	}
}

func TestWalkTradeDaysEmitsNothingForANonPositiveBound(t *testing.T) {
	f := newFakeHorizon(t)
	descPages(f, []string{tradeAt("263454423513071617-0", "2026-02-21T09:00:00Z")})
	c, _ := f.client()

	got, walk := collect(t, c, TradeDayQuery{Anchor: day(2026, 2, 22)})
	if len(got) != 0 || walk.Pages != 0 {
		t.Errorf("a zero bound must cost nothing: %d days over %d pages", len(got), walk.Pages)
	}
}

// The day boundary is decided on ledger_close_time and never on a ledger
// sequence, which is rule 4 of 00-overview section 2 in its backwards form.
func TestWalkTradeDaysSplitsOnCloseTimeRatherThanOnTheToid(t *testing.T) {
	f := newFakeHorizon(t)
	// Two adjacent TOIDs either side of midnight. Arithmetic on the identifier
	// would put them in one day; the close time puts them in two.
	descPages(f, []string{
		tradeAt("263454423513071617-0", "2026-02-21T00:00:01Z"),
		tradeAt("263454423513071616-0", "2026-02-20T23:59:59Z"),
	})
	c, _ := f.client()

	got, _ := collect(t, c, TradeDayQuery{Anchor: day(2026, 2, 22), MaxDays: 5})

	if len(got) != 2 {
		t.Fatalf("want two days, got %d: %+v", len(got), got)
	}
	for i, d := range got {
		if len(d.Trades) != 1 {
			t.Errorf("day %d (%s) carries %d trades, want 1", i, d.Start, len(d.Trades))
		}
	}
}

var _ = domain.Trade{}
