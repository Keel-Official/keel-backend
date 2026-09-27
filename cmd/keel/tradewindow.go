// The full-window trade half, classified one UTC day at a time.
//
// WHY IT EXISTS. Until 26 September 2026 the full-window half held every trade
// of the thirty day window in memory and classified them in one call. That is
// bounded by the DEC-019 threshold of 20,000 trades, and it stops being bounded
// the moment the threshold moves: HU/USDC holds about 666,000 trades in thirty
// days and keel-trades runs under a 256M limit. Raising the threshold is how the
// volume half reaches the busy pairs, so the memory had to stop scaling with the
// window first.
//
// WHY IT IS NOT JUST "CLASSIFY EACH DAY". Two conditions read beyond the trade
// itself. Condition 5 reads the median of the trade's own UTC day, which a single
// day already holds. Condition 4 compares a pool fill with the order-book trades
// inside ±BookComparisonWindow of it, and that window crosses midnight: a pool
// fill at 00:05 is judged against the book from 23:50 the day before. Classifying
// a day alone would turn those fills Unevaluated that the whole-window call judges.
// So each day is classified together with its neighbors' order-book trades near
// the shared midnight, and only the day's own classifications are kept. The
// neighbors' trades are context, never counted, and their partial-day medians
// are never read, because condition 5 looks up each trade's own day only.
//
// WHY THE SUMS ARE EXACT. Every quantity the reading stores is a sum over
// classifications (volumes, recorded counts, the excluded and total quote volume
// behind TradesExcludedPct) or a maximum by time (the last genuine trade). Days
// are disjoint, so summing per day is summing the whole. The walk emits days
// newest first, so the first day holding a genuine trade holds the newest one.
// TestStreamingFullWindowMatchesTheWholeWindow checks all of it against the
// whole-window call on days built to straddle midnight.
//
// MEMORY. At most three days at once: the day being classified, the older day
// that has just arrived, and the newer day's trades near midnight. XLM's busiest
// day is 64,484 trades.

package main

import (
	"time"

	"github.com/shopspring/decimal"

	"github.com/Keel-Official/keel-backend/internal/domain"
	"github.com/Keel-Official/keel-backend/internal/horizon"
)

// contextMargin widens the neighbor slice past BookComparisonWindow. Taking a
// trade the window then ignores costs nothing; missing one changes a median.
const contextMargin = time.Minute

// fullWindow accumulates the full-window figures of one pair's walk.
type fullWindow struct {
	base   domain.Asset
	rules  domain.GenuineRules
	anchor time.Time
	oracle time.Duration

	pending     *horizon.TradeDay
	newerNearby []domain.Trade

	totalQuote, excludedQuote decimal.Decimal
	baseD1, baseD7, baseD30   decimal.Decimal
	oracleQuote               decimal.Decimal
	oracleRecorded            int
	last                      *domain.TradeRef
}

func newFullWindow(base domain.Asset, rules domain.GenuineRules, anchor time.Time, oracle time.Duration) *fullWindow {
	return &fullWindow{base: base, rules: rules, anchor: anchor, oracle: oracle}
}

// add takes the next day of the walk, which is older than every day before it.
// The previous day can now be classified, because its older neighbor is known.
func (w *fullWindow) add(d horizon.TradeDay) {
	if w.pending != nil {
		w.classifyPending(d.Trades)
	}
	day := d
	w.pending = &day
}

// finish classifies the oldest day, which has no older neighbor in the walk.
func (w *fullWindow) finish() {
	if w.pending != nil {
		w.classifyPending(nil)
		w.pending = nil
	}
}

func (w *fullWindow) classifyPending(older []domain.Trade) {
	day := w.pending
	reach := w.rules.BookComparisonWindow + contextMargin

	olderNearby := bookTradesBetween(older, day.Start.Add(-reach), day.Start)
	subject := make([]domain.Trade, 0, len(olderNearby)+len(day.Trades)+len(w.newerNearby))
	subject = append(subject, olderNearby...)
	subject = append(subject, day.Trades...)
	subject = append(subject, w.newerNearby...)

	all := domain.ClassifyTrades(subject, w.base, w.rules)
	own := all[len(olderNearby) : len(olderNearby)+len(day.Trades)]
	w.fold(own)

	// This day is the newer neighbor of the next one to be classified.
	w.newerNearby = bookTradesBetween(day.Trades, day.Start, day.Start.Add(reach))
}

func (w *fullWindow) fold(cs []domain.TradeClassification) {
	s := domain.SummariseGenuine(cs)
	w.totalQuote = w.totalQuote.Add(s.TotalQuoteVol)
	w.excludedQuote = w.excludedQuote.Add(s.ExcludedQuoteVol)

	for _, x := range []struct {
		span time.Duration
		dst  *decimal.Decimal
	}{
		{24 * time.Hour, &w.baseD1},
		{7 * 24 * time.Hour, &w.baseD7},
		{30 * 24 * time.Hour, &w.baseD30},
	} {
		b, _, _ := domain.GenuineVolumeInWindow(cs, w.anchor, x.span)
		*x.dst = x.dst.Add(b)
	}
	_, q, n := domain.GenuineVolumeInWindow(cs, w.anchor, w.oracle)
	w.oracleQuote = w.oracleQuote.Add(q)
	w.oracleRecorded += n

	if w.last == nil {
		w.last = domain.LastGenuineTrade(cs, w.anchor)
	}
}

// excludedPct is domain.GenuineSummary.ExcludedPct over the whole window.
func (w *fullWindow) excludedPct() *decimal.Decimal {
	return domain.GenuineSummary{TotalQuoteVol: w.totalQuote, ExcludedQuoteVol: w.excludedQuote}.ExcludedPct()
}

// bookTradesBetween keeps the order-book trades closed in [from, to). Pool fills
// are left out because condition 4 compares against the book only.
func bookTradesBetween(trades []domain.Trade, from, to time.Time) []domain.Trade {
	var out []domain.Trade
	for _, t := range trades {
		if t.LiquidityPoolID != "" || t.ClosedAt.Before(from) || !t.ClosedAt.Before(to) {
			continue
		}
		out = append(out, t)
	}
	return out
}
