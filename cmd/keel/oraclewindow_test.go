package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/Keel-Official/keel-backend/internal/domain"
	"github.com/Keel-Official/keel-backend/internal/horizon"
	"github.com/Keel-Official/keel-backend/internal/store"
)

// fakeTrades serves a fixed reference day and a fixed recent window, and counts
// how often each is asked for, because the reference walk is the expensive one
// and caching it is a promise this file makes.
type fakeTrades struct {
	refDay       []domain.Trade
	refExhausted bool
	refCapped    bool
	window       []domain.Trade
	windowWalk   horizon.RecentTradesWalk
	refCalls     int
	windowCalls  int
}

func (f *fakeTrades) RecentTrades(_ context.Context, _, _ domain.Asset, _ time.Time, _ int) ([]domain.Trade, horizon.RecentTradesWalk, error) {
	f.windowCalls++
	return f.window, f.windowWalk, nil
}

func (f *fakeTrades) WalkTradeDays(_ context.Context, _, _ domain.Asset, q horizon.TradeDayQuery, fn horizon.TradeDayFunc) (horizon.TradeDayWalk, error) {
	f.refCalls++
	switch {
	case f.refCapped:
		return horizon.TradeDayWalk{PageCapReached: true}, nil
	case f.refDay == nil && f.refExhausted:
		return horizon.TradeDayWalk{Exhausted: true}, nil
	}
	start := time.Date(q.Anchor.Year(), q.Anchor.Month(), q.Anchor.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1)
	_, err := fn(horizon.TradeDay{Start: start, Trades: f.refDay})
	return horizon.TradeDayWalk{Days: 1}, err
}

var owPair = store.Asset{
	ID:    1,
	Base:  domain.Asset{Code: "USTRY", Issuer: "GISSUER", Type: domain.AssetTypeAlphanum12},
	Quote: domain.Asset{Code: "USDC", Issuer: "GUSDC", Type: domain.AssetTypeAlphanum4},
}

func owTrade(at time.Time, priceN int64, amount string) domain.Trade {
	return domain.Trade{
		ID: at.Format(time.RFC3339Nano), LedgerSeq: 1, ClosedAt: at, Type: "orderbook",
		Price: domain.Price{N: priceN, D: 1000}, BaseAmount: decimal.RequireFromString(amount),
		CounterAmount: decimal.RequireFromString(amount), BaseAccount: "GA", CounterAccount: "GB",
	}
}

func TestOracleWindowMeasuresAtTheScannedLedgerAndCachesTheReference(t *testing.T) {
	anchor := time.Date(2026, 2, 22, 0, 10, 21, 0, time.UTC)
	f := &fakeTrades{
		refDay: []domain.Trade{owTrade(time.Date(2026, 2, 21, 9, 0, 0, 0, time.UTC), 1057, "1")},
		window: []domain.Trade{
			owTrade(anchor.Add(-9*time.Minute), 1057, "0.30"),  // genuine, inside the window
			owTrade(anchor, 106737, "5.35"),                    // the outlier, against 1.057
			owTrade(anchor.Add(-20*time.Minute), 1057, "0.50"), // outside W, inside the neighbourhood
		},
		windowWalk: horizon.RecentTradesWalk{Complete: true, Pages: 1},
	}
	o := newOracleWindowReader(f, 3, 20)

	sup, why := o.attach(context.Background(), owPair, anchor, 15*time.Minute, nil)
	if why != "" {
		t.Fatalf("declined: %s", why)
	}
	if sup.OracleWindowAnchor == nil || !sup.OracleWindowAnchor.Equal(anchor) {
		t.Errorf("anchor = %v, want the scanned ledger's close time %s", sup.OracleWindowAnchor, anchor)
	}
	if sup.GenuineVolumeInWindow == nil || !sup.GenuineVolumeInWindow.Equal(decimal.RequireFromString("0.30")) {
		t.Errorf("V = %v, want 0.30: the outlier excluded and the older trade outside W", sup.GenuineVolumeInWindow)
	}

	// A second round the same day pays for the window again and not for the day.
	if _, why := o.attach(context.Background(), owPair, anchor.Add(15*time.Minute), 15*time.Minute, nil); why != "" {
		t.Fatalf("second round declined: %s", why)
	}
	if f.refCalls != 1 || f.windowCalls != 2 {
		t.Errorf("reference walks %d, window reads %d; want 1 and 2", f.refCalls, f.windowCalls)
	}
}

func TestOracleWindowOnAPartialReadIsUnevaluated(t *testing.T) {
	f := &fakeTrades{refExhausted: true, windowWalk: horizon.RecentTradesWalk{PageCapReached: true, Pages: 3}}
	o := newOracleWindowReader(f, 3, 20)

	sup, why := o.attach(context.Background(), owPair, time.Now().UTC(), 15*time.Minute, nil)
	if sup != nil || !strings.Contains(why, "more than 3 page(s)") {
		t.Errorf("sup = %+v, why = %q; a partial window must not become a volume", sup, why)
	}
}

func TestOracleWindowCachesAnUnaffordableReferenceForTheDay(t *testing.T) {
	anchor := time.Date(2026, 9, 21, 16, 0, 0, 0, time.UTC)
	f := &fakeTrades{refCapped: true, windowWalk: horizon.RecentTradesWalk{Complete: true}}
	o := newOracleWindowReader(f, 3, 20)

	for i := 0; i < 3; i++ {
		if _, why := o.attach(context.Background(), owPair, anchor.Add(time.Duration(i)*15*time.Minute), 15*time.Minute, nil); !strings.Contains(why, "not reachable within 20 page(s)") {
			t.Fatalf("round %d: why = %q", i, why)
		}
	}
	if f.refCalls != 1 || f.windowCalls != 0 {
		t.Errorf("reference walks %d, window reads %d; a busy pair is tried once a day and its window never read", f.refCalls, f.windowCalls)
	}

	// The next UTC day is a new reference day and is tried again.
	_, _ = o.attach(context.Background(), owPair, anchor.AddDate(0, 0, 1), 15*time.Minute, nil)
	if f.refCalls != 2 {
		t.Errorf("reference walks %d after the day turned, want 2", f.refCalls)
	}
}

func TestOracleWindowSwitchedOff(t *testing.T) {
	var nilReader *oracleWindowReader
	for _, o := range []*oracleWindowReader{nilReader, newOracleWindowReader(&fakeTrades{}, 0, 20)} {
		if _, why := o.attach(context.Background(), owPair, time.Now(), 15*time.Minute, nil); why != "live oracle window disabled" {
			t.Errorf("why = %q", why)
		}
	}
}
