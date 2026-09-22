package main

import (
	"context"
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	"github.com/Keel-Official/keel-backend/internal/domain"
	"github.com/Keel-Official/keel-backend/internal/horizon"
	"github.com/Keel-Official/keel-backend/internal/store"
)

// The oracle window, measured at the ledger a scan round just read.
//
// WHY THE SCAN MEASURES IT ITSELF. 06-oracle-resilience.md section 1 defines
// V_genuine(W) over the W seconds ending at the output ledger, and
// oracleResistance pairs it with a manipulation cost measured at that ledger. The
// trade cache `keel trades` fills sums the window back from 00:00Z of the day it
// ran, so reading it here would put two instants up to a day apart into one
// figure. domain.SupportingMetrics.OracleWindowAnchor is what makes the engine
// refuse that pairing; this file is what gives it a volume it can accept.
//
// WHAT A ROUND COSTS. One /trades read per pair back to W plus the condition 4
// neighborhood, which is one request for a quiet pair. Once per pair per UTC day,
// a walk of the previous complete day for its order-book median, which pages
// through today's trades first because /trades only reads newest first. Both are
// bounded in pages, and a read that hits its bound yields an unevaluated window
// rather than a partial sum.
//
// THE THREE SENTENCES THIS FILE OWES. The decision: classify the window against
// the median of the most recent COMPLETE UTC day, cached per pair per day. The
// alternative rejected: classifying against today's median so far, which is one
// fewer walk a day and is exactly the partial-day statistic DEC-019 section 8.2
// refuses, because its verdicts can only move from genuine to excluded as the day
// fills in. Why: the reference-day reading errs towards less genuine volume, which
// can only make an attack look cheaper, and that is the direction this product is
// allowed to err in.

// recentTradesSource is the part of *horizon.Client this file reads, so a test can
// hand it a fake instead of a server.
type recentTradesSource interface {
	RecentTrades(ctx context.Context, base, quote domain.Asset, since time.Time, maxPages int) ([]domain.Trade, horizon.RecentTradesWalk, error)
	WalkTradeDays(ctx context.Context, base, quote domain.Asset, q horizon.TradeDayQuery, fn horizon.TradeDayFunc) (horizon.TradeDayWalk, error)
}

// oracleWindowReader measures V_genuine(W) at a scan ledger. A nil reader, or one
// with WindowPages at zero, is the feature switched off.
type oracleWindowReader struct {
	src            recentTradesSource
	windowPages    int
	referencePages int
	rules          domain.GenuineRules

	// refs holds one reference median per pair, for the day it was read for.
	// Failures are cached too, so a pair too busy to walk is tried once a day and
	// not once a round.
	refs map[string]referenceMedian
}

type referenceMedian struct {
	day    time.Time
	median *decimal.Decimal
	why    string
}

func newOracleWindowReader(src recentTradesSource, windowPages, referencePages int) *oracleWindowReader {
	return &oracleWindowReader{
		src:            src,
		windowPages:    windowPages,
		referencePages: referencePages,
		rules:          domain.DefaultGenuineRules(),
		refs:           map[string]referenceMedian{},
	}
}

// attach measures the window ending at anchor and writes it into sup, creating
// sup when it is nil, and says in one phrase why it did not when it did not. On a
// decline sup is returned untouched, so the engine sees no anchored volume and
// names the gap in the row's own warnings.
func (o *oracleWindowReader) attach(
	ctx context.Context,
	a store.Asset,
	anchor time.Time,
	window time.Duration,
	sup *domain.SupportingMetrics,
) (*domain.SupportingMetrics, string) {
	if o == nil || o.windowPages <= 0 {
		return sup, "live oracle window disabled"
	}
	anchor = anchor.UTC()

	ref, why := o.reference(ctx, a, anchor)
	if why != "" {
		return sup, why
	}

	since := anchor.Add(-window - o.rules.BookComparisonWindow)
	trades, walk, err := o.src.RecentTrades(ctx, a.Base, a.Quote, since, o.windowPages)
	if err != nil {
		return sup, "oracle window unreadable: " + err.Error()
	}
	if !walk.Complete {
		return sup, fmt.Sprintf("more than %d page(s) of trades in the last %s", o.windowPages, anchor.Sub(since))
	}

	cs := domain.ClassifyTradesAgainstMedian(trades, a.Base, o.rules, ref)
	_, quote, _ := domain.GenuineVolumeInWindow(cs, anchor, window)

	if sup == nil {
		sup = &domain.SupportingMetrics{}
	}
	sup.GenuineVolumeInWindow = &quote
	sup.OracleWindowAnchor = &anchor
	return sup, ""
}

// reference returns the order-book median of the last complete UTC day before
// anchor, from the cache when it holds that day. A nil median with no reason is an
// answer: that day had no order-book trade, and condition 5 then passes every
// trade, which is ClassifyTrades' own rule.
func (o *oracleWindowReader) reference(ctx context.Context, a store.Asset, anchor time.Time) (*decimal.Decimal, string) {
	key := a.Base.String() + "/" + a.Quote.String()
	day := time.Date(anchor.Year(), anchor.Month(), anchor.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1)
	if r, ok := o.refs[key]; ok && r.day.Equal(day) {
		return r.median, r.why
	}

	var trades []domain.Trade
	var got bool
	walk, err := o.src.WalkTradeDays(ctx, a.Base, a.Quote, horizon.TradeDayQuery{
		Anchor:   anchor,
		MaxDays:  1,
		MaxPages: o.referencePages,
	}, func(d horizon.TradeDay) (bool, error) {
		trades, got = d.Trades, true
		return false, nil
	})
	if err != nil {
		// Not cached: a transport error says nothing about the day.
		return nil, "reference day unreadable: " + err.Error()
	}

	r := referenceMedian{day: day}
	switch {
	case got:
		m, ok, err := domain.OrderBookMedian(trades)
		switch {
		case err != nil:
			r.why = "reference day malformed: " + err.Error()
		case ok:
			r.median = &m
		}
	case walk.Exhausted:
		// The pair has no trade before today at all, so there is no median to
		// judge against and condition 5 passes, as it does for any day without one.
	default:
		r.why = fmt.Sprintf("reference day not reachable within %d page(s)", o.referencePages)
	}
	o.refs[key] = r
	return r.median, r.why
}
