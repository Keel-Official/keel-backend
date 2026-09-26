// The day-at-a-time full window must reproduce the whole-window call exactly.
//
// The days below are built to make that hard: pool fills a few minutes either
// side of midnight whose only order-book neighbors sit on the other day, so a
// day classified alone gets them wrong. The negative control at the end proves
// the fixture does exercise that, rather than passing because nothing crosses.

package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/Keel-Official/keel-backend/internal/domain"
	"github.com/Keel-Official/keel-backend/internal/horizon"
)

var twBase = domain.Asset{Code: "USTRY", Issuer: "GISSUER", Type: domain.AssetTypeAlphanum12}

type twKind int

const (
	twBook twKind = iota
	twPool
	twDust
	twSelf
	twIssuer
	twOutlier
)

func twTrade(at time.Time, kind twKind, priceN int64, amount string) domain.Trade {
	t := domain.Trade{
		ID: fmt.Sprintf("%s-%d", at.Format(time.RFC3339Nano), kind), LedgerSeq: uint32(at.Unix() / 5), ClosedAt: at,
		Type: "orderbook", Price: domain.Price{N: priceN, D: 1000},
		BaseAmount: decimal.RequireFromString(amount), CounterAmount: decimal.RequireFromString(amount),
		BaseAccount: "GA", CounterAccount: "GB",
	}
	switch kind {
	case twPool:
		t.Type, t.LiquidityPoolID = "liquidity_pool", "pool1"
	case twDust:
		t.BaseAmount, t.CounterAmount = decimal.RequireFromString("0.005"), decimal.RequireFromString("0.005")
	case twSelf:
		t.CounterAccount = "GA"
	case twIssuer:
		t.BaseAccount = "GISSUER"
	case twOutlier:
		t.Price = domain.Price{N: priceN * 3, D: 1000}
	}
	return t
}

// twDays builds five UTC days ending at anchor, newest first, the order the walk
// emits them. Each day's trades are ascending, as horizon.TradeDay promises.
func twDays(anchor time.Time) []horizon.TradeDay {
	var days []horizon.TradeDay
	for i := 1; i <= 5; i++ {
		start := anchor.AddDate(0, 0, -i)
		at := func(h, m int) time.Time { return start.Add(time.Duration(h)*time.Hour + time.Duration(m)*time.Minute) }
		p := int64(1000 + 10*i)
		trades := []domain.Trade{
			// Just after midnight: a pool fill with no book trade of its own day
			// inside 15 minutes. Its only neighbor is 23:55 the day before.
			twTrade(at(0, 3), twPool, p-5, "4"),
			twTrade(at(0, 30), twBook, p, "10"),
			twTrade(at(3, 0), twDust, p, "1"),
			twTrade(at(6, 0), twBook, p+2, "7"),
			twTrade(at(9, 0), twSelf, p, "5"),
			twTrade(at(12, 0), twIssuer, p, "6"),
			twTrade(at(12, 1), twPool, p+50, "3"), // dearer than the book at 12:00: excluded
			twTrade(at(12, 2), twBook, p-1, "8"),
			twTrade(at(15, 0), twOutlier, p, "2"),
			twTrade(at(18, 0), twBook, p+1, "9"),
			twTrade(at(23, 40), twBook, p, "3"),
			twTrade(at(23, 55), twBook, p-3, "5"),
			// Just before midnight, with its nearest book trade after midnight
			// on the NEXT day as well as 23:55 on its own.
			twTrade(at(23, 58), twPool, p+20, "2"),
		}
		if i == 1 {
			// The oracle window: the last fifteen minutes before the anchor.
			trades = append(trades[:len(trades)-1], twTrade(at(23, 50), twBook, p, "4"), trades[len(trades)-1])
		}
		days = append(days, horizon.TradeDay{Start: start, Trades: trades})
	}
	// A book trade at 00:04 of each newer day, which the 23:58 fill of the older
	// day must see.
	for i := 0; i < len(days)-1; i++ {
		newer := &days[i]
		p := int64(1000 + 10*(i+1))
		early := twTrade(newer.Start.Add(4*time.Minute), twBook, p+30, "1")
		newer.Trades = append([]domain.Trade{newer.Trades[0], early}, newer.Trades[1:]...)
	}
	return days
}

type twResult struct {
	excluded                 *decimal.Decimal
	d1, d7, d30, oracleQuote decimal.Decimal
	oracleRecorded           int
	last                     *domain.TradeRef
}

func twWhole(days []horizon.TradeDay, anchor time.Time, rules domain.GenuineRules, oracle time.Duration) (twResult, []domain.TradeClassification) {
	nested := make([][]domain.Trade, len(days))
	for i, d := range days {
		nested[i] = d.Trades
	}
	cs := domain.ClassifyTrades(flattenDaysAscending(nested), twBase, rules)
	var r twResult
	r.excluded = domain.SummariseGenuine(cs).ExcludedPct()
	r.d1, _, _ = domain.GenuineVolumeInWindow(cs, anchor, 24*time.Hour)
	r.d7, _, _ = domain.GenuineVolumeInWindow(cs, anchor, 7*24*time.Hour)
	r.d30, _, _ = domain.GenuineVolumeInWindow(cs, anchor, 30*24*time.Hour)
	_, r.oracleQuote, r.oracleRecorded = domain.GenuineVolumeInWindow(cs, anchor, oracle)
	r.last = domain.LastGenuineTrade(cs, anchor)
	return r, cs
}

func twStream(days []horizon.TradeDay, anchor time.Time, rules domain.GenuineRules, oracle time.Duration) twResult {
	fw := newFullWindow(twBase, rules, anchor, oracle)
	for _, d := range days {
		fw.add(d)
	}
	fw.finish()
	return twResult{
		excluded: fw.excludedPct(), d1: fw.baseD1, d7: fw.baseD7, d30: fw.baseD30,
		oracleQuote: fw.oracleQuote, oracleRecorded: fw.oracleRecorded, last: fw.last,
	}
}

func twCompare(t *testing.T, got, want twResult) {
	t.Helper()
	if (got.excluded == nil) != (want.excluded == nil) || (got.excluded != nil && !got.excluded.Equal(*want.excluded)) {
		t.Errorf("excluded pct = %v, want %v", got.excluded, want.excluded)
	}
	for _, x := range []struct {
		name      string
		got, want decimal.Decimal
	}{
		{"d1", got.d1, want.d1}, {"d7", got.d7, want.d7}, {"d30", got.d30, want.d30},
		{"oracle quote", got.oracleQuote, want.oracleQuote},
	} {
		if !x.got.Equal(x.want) {
			t.Errorf("%s = %s, want %s", x.name, x.got, x.want)
		}
	}
	if got.oracleRecorded != want.oracleRecorded {
		t.Errorf("oracle recorded = %d, want %d", got.oracleRecorded, want.oracleRecorded)
	}
	if (got.last == nil) != (want.last == nil) || (got.last != nil && *got.last != *want.last) {
		t.Errorf("last genuine = %v, want %v", got.last, want.last)
	}
}

func TestStreamingFullWindowMatchesTheWholeWindow(t *testing.T) {
	anchor := time.Date(2026, 2, 10, 0, 0, 0, 0, time.UTC)
	rules := domain.DefaultGenuineRules()
	oracle := domain.DefaultParams().OracleWindow
	days := twDays(anchor)

	want, cs := twWhole(days, anchor, rules, oracle)

	// The fixture has to put the answer on the other side of midnight, or this
	// test proves nothing. Every 00:03 fill except the oldest day's is judged by
	// the whole-window call, and it can only have been judged from the day before.
	judgedAcrossMidnight := 0
	for _, c := range cs {
		if c.ClosedAt.Hour() == 0 && c.ClosedAt.Minute() == 3 && c.State != domain.GenuineStateUnevaluated {
			judgedAcrossMidnight++
		}
	}
	if judgedAcrossMidnight != len(days)-1 {
		t.Fatalf("%d of the 00:03 pool fills were judged across midnight, want %d; the fixture no longer tests the boundary",
			judgedAcrossMidnight, len(days)-1)
	}

	twCompare(t, twStream(days, anchor, rules, oracle), want)
}

// The negative control: classifying each day with no neighbors disagrees with
// the whole window, so the agreement above is the context doing its job.
func TestClassifyingDaysAloneWouldNotMatch(t *testing.T) {
	anchor := time.Date(2026, 2, 10, 0, 0, 0, 0, time.UTC)
	rules := domain.DefaultGenuineRules()
	days := twDays(anchor)

	_, whole := twWhole(days, anchor, rules, domain.DefaultParams().OracleWindow)
	wholeState := map[string]domain.GenuineState{}
	for _, c := range whole {
		wholeState[c.ID] = c.State
	}

	differ := 0
	for _, d := range days {
		for _, c := range domain.ClassifyTrades(d.Trades, twBase, rules) {
			if wholeState[c.ID] != c.State {
				differ++
			}
		}
	}
	if differ == 0 {
		t.Fatal("classifying days alone agreed with the whole window, so the streaming test does not exercise midnight")
	}
}

// Memory is the reason the accumulator exists: it must hold no more than the
// day it is classifying, the day just arrived, and a slice near midnight.
func TestStreamingFullWindowDropsClassifiedDays(t *testing.T) {
	anchor := time.Date(2026, 2, 10, 0, 0, 0, 0, time.UTC)
	fw := newFullWindow(twBase, domain.DefaultGenuineRules(), anchor, domain.DefaultParams().OracleWindow)
	for _, d := range twDays(anchor) {
		fw.add(d)
		if fw.pending == nil || !fw.pending.Start.Equal(d.Start) {
			t.Fatalf("pending day is not the one just added")
		}
		for _, tr := range fw.newerNearby {
			if tr.LiquidityPoolID != "" {
				t.Fatalf("a pool fill was kept as context: %s", tr.ID)
			}
			if tr.ClosedAt.Sub(d.Start.AddDate(0, 0, 1)) > 16*time.Minute {
				t.Fatalf("context reaches %s past midnight, want at most the comparison window",
					tr.ClosedAt.Sub(d.Start.AddDate(0, 0, 1)))
			}
		}
	}
	fw.finish()
	if fw.pending != nil {
		t.Error("finish left a day pending")
	}
}
