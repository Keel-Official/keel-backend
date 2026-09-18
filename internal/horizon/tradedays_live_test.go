package horizon

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Keel-Official/keel-backend/internal/domain"
)

// A LIVE probe against public Horizon, skipped unless KEEL_HORIZON_LIVE is set.
//
// WHY IT EXISTS AND WHY IT IS NOT IN THE ORDINARY SUITE. Every other test in this
// file drives a fake whose pages this repository wrote, so all of them would keep
// passing if Horizon stopped serving `order=desc` on /trades, or started serving
// it with a cursor of a different shape, or returned the records of a descending
// page in ascending order inside the page. Those are the three assumptions the
// backward walk rests on and none of them is documented anywhere this repository
// controls. The fake cannot falsify them and this can.
//
//	KEEL_HORIZON_LIVE=1 go test ./internal/horizon -run Live -v
//
// It asserts SHAPE and never a figure. The trades of any real pair change between
// two runs, so an assertion on a count or a price would be a test that fails for
// the wrong reason; what is checked is that the days come back whole, in
// descending order, each one's trades ascending inside it, and that a day's
// boundary is respected.
func TestLiveWalkTradeDaysAgainstPublicHorizon(t *testing.T) {
	if os.Getenv("KEEL_HORIZON_LIVE") == "" {
		t.Skip("set KEEL_HORIZON_LIVE=1 to run the live Horizon probe")
	}

	ustry := domain.Asset{
		Code:   "USTRY",
		Issuer: "GCRYUGD5NVARGXT56XEZI5CIFCQETYHAPQQTHO2O3IQZTHDH4LATMYWC",
		Type:   domain.AssetTypeAlphanum12,
	}
	usdc := domain.Asset{
		Code:   "USDC",
		Issuer: "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN",
		Type:   domain.AssetTypeAlphanum4,
	}

	c := NewClient(Config{BaseURL: DefaultBaseURL, Budget: 3000})
	now := time.Now().UTC()
	anchor := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	var days []TradeDay
	walk, err := c.WalkTradeDays(ctx, ustry, usdc, TradeDayQuery{Anchor: anchor, MaxDays: 5},
		func(d TradeDay) (bool, error) {
			days = append(days, d)
			return true, nil
		})
	if err != nil {
		t.Fatalf("WalkTradeDays against live Horizon: %v", err)
	}
	if len(days) == 0 {
		t.Fatal("no days came back from a pair with six months of history")
	}

	for i, d := range days {
		if !d.Start.Equal(time.Date(d.Start.Year(), d.Start.Month(), d.Start.Day(), 0, 0, 0, 0, time.UTC)) {
			t.Errorf("day %d starts at %s, which is not a UTC midnight", i, d.Start)
		}
		if !d.Start.Before(anchor) {
			t.Errorf("day %d is %s, which is not before the anchor %s", i, d.Start, anchor)
		}
		if i > 0 && !d.Start.Before(days[i-1].Start) {
			t.Errorf("day %d (%s) does not precede day %d (%s): the walk is not descending",
				i, d.Start, i-1, days[i-1].Start)
		}
		for j, tr := range d.Trades {
			if tr.ClosedAt.Before(d.Start) || !tr.ClosedAt.Before(d.End()) {
				t.Errorf("day %s holds a trade closed at %s, which is outside it", d.Start, tr.ClosedAt)
			}
			if j > 0 && tr.ClosedAt.Before(d.Trades[j-1].ClosedAt) {
				t.Errorf("day %s is not ascending inside itself at index %d", d.Start, j)
			}
		}
	}

	if walk.LedgerSeq == 0 {
		t.Error("the walk recorded no Latest-Ledger, so no row it produced could be cited")
	}
	t.Logf("days=%d pages=%d exhausted=%v boundReached=%v ledger=%d requests=%d",
		walk.Days, walk.Pages, walk.Exhausted, walk.BoundReached, walk.LedgerSeq, c.Requests())
	for _, d := range days {
		t.Logf("  %s: %d trade(s)", d.Start.Format("2006-01-02"), len(d.Trades))
	}
}
