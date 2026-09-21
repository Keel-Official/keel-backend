package horizon

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Keel-Official/keel-backend/internal/domain"
)

func recent(t *testing.T, c *Client, since time.Time, maxPages int) ([]string, RecentTradesWalk) {
	t.Helper()
	trades, walk, err := c.RecentTrades(context.Background(), testUSTRY, testUSDC, since, maxPages)
	if err != nil {
		t.Fatalf("RecentTrades: %v", err)
	}
	ids := make([]string, len(trades))
	for i, tr := range trades {
		ids[i] = tr.ClosedAt.UTC().Format(time.RFC3339)
	}
	return ids, walk
}

func TestRecentTradesStopsAtTheFirstOlderRecordAndCallsThatComplete(t *testing.T) {
	f := newFakeHorizon(t)
	descPages(f, []string{
		tradeAt("263454423513071618-0", "2026-02-22T00:10:21Z"),
		tradeAt("263454423513071617-0", "2026-02-22T00:01:30Z"),
		tradeAt("263454256009383937-1", "2026-02-21T23:30:00Z"),
	})
	c, _ := f.client()

	got, walk := recent(t, c, time.Date(2026, 2, 21, 23, 40, 21, 0, time.UTC), 5)
	if !walk.Complete || walk.PageCapReached {
		t.Fatalf("walk = %+v, want complete on the record older than since", walk)
	}
	want := []string{"2026-02-22T00:01:30Z", "2026-02-22T00:10:21Z"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("trades = %v, want %v ascending, the older record excluded", got, want)
	}
}

func TestRecentTradesOnAPageBoundIsNotComplete(t *testing.T) {
	// Two pages newer than since and a cap of one: the set is partial and must say
	// so, or a volume summed over it would read as the whole window.
	f := newFakeHorizon(t)
	descPages(f,
		[]string{tradeAt("263454423513071618-0", "2026-02-22T00:10:21Z")},
		[]string{tradeAt("263454423513071617-0", "2026-02-22T00:05:00Z")},
	)
	c, _ := f.client()

	_, walk := recent(t, c, time.Date(2026, 2, 22, 0, 0, 0, 0, time.UTC), 1)
	if walk.Complete || !walk.PageCapReached || walk.Pages != 1 {
		t.Errorf("walk = %+v, want partial on the page bound after one request", walk)
	}
}

func TestRecentTradesCallsAnEndedHistoryComplete(t *testing.T) {
	// A pair that never traded before since has nothing older to prove the edge
	// with; the history running out is the proof.
	f := newFakeHorizon(t)
	descPages(f, []string{tradeAt("263454423513071618-0", "2026-02-22T00:10:21Z")})
	c, _ := f.client()

	got, walk := recent(t, c, time.Date(2026, 2, 22, 0, 0, 0, 0, time.UTC), 0)
	if !walk.Complete || len(got) != 1 {
		t.Errorf("walk = %+v, trades %v; want complete with the one trade", walk, got)
	}
}

// TestLiveRecentTradesAgainstPublicHorizon is the same kind of probe as
// TestLiveWalkTradeDaysAgainstPublicHorizon, for the same reason: the fake cannot
// falsify that a descending read of the last hour comes back complete, ascending,
// and bounded by since. Shape only, never a figure.
//
//	KEEL_HORIZON_LIVE=1 go test ./internal/horizon -run LiveRecent -v
func TestLiveRecentTradesAgainstPublicHorizon(t *testing.T) {
	if os.Getenv("KEEL_HORIZON_LIVE") == "" {
		t.Skip("set KEEL_HORIZON_LIVE=1 to run the live Horizon probe")
	}
	ustry := domain.Asset{Code: "USTRY", Issuer: "GCRYUGD5NVARGXT56XEZI5CIFCQETYHAPQQTHO2O3IQZTHDH4LATMYWC", Type: domain.AssetTypeAlphanum12}
	usdc := domain.Asset{Code: "USDC", Issuer: "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN", Type: domain.AssetTypeAlphanum4}
	c := NewClient(Config{BaseURL: DefaultBaseURL, Budget: 60})

	since := time.Now().UTC().Add(-time.Hour)
	trades, walk, err := c.RecentTrades(context.Background(), ustry, usdc, since, 10)
	if err != nil {
		t.Fatalf("RecentTrades: %v", err)
	}
	t.Logf("%d trade(s) in the last hour over %d page(s), complete=%v", len(trades), walk.Pages, walk.Complete)
	if !walk.Complete {
		t.Fatalf("not complete within 10 pages: %+v", walk)
	}
	for i, tr := range trades {
		if tr.ClosedAt.Before(since) {
			t.Errorf("trade %d closed at %s, before since %s", i, tr.ClosedAt, since)
		}
		if i > 0 && tr.ClosedAt.Before(trades[i-1].ClosedAt) {
			t.Errorf("trade %d is older than trade %d; the result must be ascending", i, i-1)
		}
	}
}
