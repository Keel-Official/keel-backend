package main

import (
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	"github.com/Keel-Official/keel-backend/internal/domain"
)

// The trade-derived metrics AT A PAST LEDGER, from the trades a replay already read.
//
// WHY THIS EXISTS. A reconstructed row used to carry no supporting metrics at all:
// six flags read `unevaluated` and `bandConfidence` was `partial` on every stored
// historical row, and `oracleResistance` was null, because the engine is handed
// nothing but a book. The trades are already in hand. `keel replay` walks them to
// consume offers, so the last genuine trade, the excluded share and the genuine
// volume inside the oracle window can be computed for the target with no extra
// request.
//
// WHAT IT DELIBERATELY DOES NOT PRODUCE. The volume-to-supply ratio, because its
// denominator is circulating supply from the trustline set and `/accounts?asset=`
// answers only for NOW. 07-supporting-metrics.md section 2 says so in as many
// words: holder concentration has no historical version and never will for a day
// already gone. A ratio built on today's supply and February's volume would be two
// instants in one number, which is the error SupportingMetrics.OracleWindowAnchor
// exists to refuse.
//
// THE PARTIAL DAY IS HANDLED THE WAY THE LIVE SCAN HANDLES IT. Condition 5 of
// section 1 compares a trade against the median of ITS OWN day, and the target's
// day is not complete in this set: the walk stops at the target plus a lookahead.
// DEC-019 section 8.2 refuses a partial-day median, so trades on the target's day
// are judged against the last COMPLETE day's median, exactly as cmd/keel's live
// oracle window does. Trades on days that ARE complete in the set keep their own
// day's median.
//
// THE THREE SENTENCES THIS FILE OWES. The decision: split the classification at
// the target's own day and use the previous complete day's order-book median for
// the remainder. The alternative rejected: classifying the whole set with
// ClassifyTrades, which computes a median for the partial day from whatever part
// of it the walk happened to read, so the verdict would move with the lookahead
// rather than with the market. Why: a metric whose value depends on how far past
// the target a walk ran is not a measurement of the target.

// historicalSupporting builds what can be computed at anchor from the trades a
// replay read, and returns the reasons for anything it left nil.
//
// trades may cover more than the window; anything after anchor is ignored for the
// metrics and only the classification's own day rules read it. The returned
// metrics are nil when the set does not reach back far enough to hold one
// complete day before the target, because without that there is no median to
// judge the target's own day against.
func historicalSupporting(
	trades []domain.Trade,
	base domain.Asset,
	anchor time.Time,
	window time.Duration,
	rules domain.GenuineRules,
) (*domain.SupportingMetrics, []string) {
	anchor = anchor.UTC()
	var notes []string

	targetDay := utcDayStart(anchor)
	referenceDay := targetDay.AddDate(0, 0, -1)

	var earliest time.Time
	var onReferenceDay, onCompleteDays, onTargetDay []domain.Trade
	for _, t := range trades {
		at := t.ClosedAt.UTC()
		if at.After(anchor) {
			continue
		}
		if earliest.IsZero() || at.Before(earliest) {
			earliest = at
		}
		switch {
		case !at.Before(targetDay):
			onTargetDay = append(onTargetDay, t)
		default:
			onCompleteDays = append(onCompleteDays, t)
			if !at.Before(referenceDay) && at.Before(targetDay) {
				onReferenceDay = append(onReferenceDay, t)
			}
		}
	}

	if earliest.IsZero() {
		return nil, []string{"no trade at or before the target ledger was read, so nothing can be classified"}
	}
	if earliest.After(referenceDay) {
		return nil, []string{fmt.Sprintf(
			"the trade walk starts at %s, inside the reference day %s, so the median the target's own day is judged against would itself be partial",
			earliest.Format(time.RFC3339), referenceDay.Format("2006-01-02"))}
	}

	median, ok, err := domain.OrderBookMedian(onReferenceDay)
	if err != nil {
		return nil, []string{"reference day malformed: " + err.Error()}
	}
	var reference *decimal.Decimal
	if ok {
		reference = &median
	} else {
		notes = append(notes, fmt.Sprintf(
			"the reference day %s holds no order-book trade, so condition 5 passes every trade on the target's own day",
			referenceDay.Format("2006-01-02")))
	}

	cs := domain.ClassifyTrades(onCompleteDays, base, rules)
	cs = append(cs, domain.ClassifyTradesAgainstMedian(onTargetDay, base, rules, reference)...)

	_, quote, recorded := domain.GenuineVolumeInWindow(cs, anchor, window)
	covered := anchor.Sub(earliest)

	sup := &domain.SupportingMetrics{
		LastGenuineTrade:      domain.LastGenuineTrade(cs, anchor),
		TradesExcludedPct:     domain.SummariseGenuine(cs).ExcludedPct(),
		GenuineVolumeInWindow: &quote,
		OracleWindowAnchor:    &anchor,
		GenuineSearchWindow:   &covered,
	}
	notes = append(notes, fmt.Sprintf(
		"%d trade(s) at or before the target, %d inside the %s oracle window, genuine volume %s in the quote asset",
		len(cs), recorded, window, quote))
	notes = append(notes,
		"volume-to-supply is not computed at a past ledger: its denominator is the trustline set, which Horizon answers only for now")
	return sup, notes
}

// utcDayStart is 00:00:00Z of the day t falls in, built with time.Date for the
// reason internal/horizon says: Truncate measures from the zero time and is
// correct for UTC only by coincidence.
func utcDayStart(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}
