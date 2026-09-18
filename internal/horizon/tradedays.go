// Walking /trades BACKWARDS, in whole UTC days.
//
// WHY THIS IS A SECOND SHAPE RATHER THAN A FLAG ON TradeQuery. The walk in
// trades.go runs forward from a ledger seek and serves a window whose start is
// known before it begins, which is what the backtest asks for. This walk answers
// the opposite question, "how far back do I have to go before something is true",
// and its start is not known until it stops. DEC-019 section 8.4 item 3 names the
// two as different shapes over one endpoint and says so rather than overloading
// the query, and the two stop conditions are the reason: one ends on a predicate
// over a single trade, this one cannot decide anything until a whole day is in
// hand.
//
// WHY A DAY IS THE UNIT AND NOT A PAGE. DEC-019 section 8.1 is the correction
// that produced this file. domain.ClassifyTrades computes the daily order book
// median over the whole slice it is given, and condition 5 reads it, so a walk
// that stops part way through a day judges that day's trades against a median of
// a partial day. Section 8.2 is why that is disqualifying rather than merely
// imprecise: the verdict can only move from genuine to excluded as more of the
// day arrives, so a partial walk reports a last genuine trade NEWER than the true
// one and the asset reads as fresher than it is. A warning product may fail
// towards more dangerous only over Al's signature, never as a side effect of
// paging.
//
// THE ANCHOR'S OWN DAY IS NEVER EMITTED, and that is the same rule pointed at
// today. The current UTC day is partial by definition because it has not ended,
// so its median is the statistic section 8.2 refuses. Excluding it makes every
// answer here up to 24 hours STALE, which is the direction that is allowed: an
// asset that traded genuinely an hour ago reads as "last genuine trade yesterday",
// which can only fire a staleness flag early and never late.
//
// THE THREE SENTENCES THIS ZONE ASKS FOR.
//
// The decision: the walk pages `order=desc` from the newest trade and closes a
// day the moment it sees a trade older than that day, so completeness is PROVEN
// by a record rather than assumed from a count. The alternative rejected:
// converting each day boundary into a ledger sequence and running the existing
// forward walk once per day, which would have reused trades.go unchanged. Why it
// was rejected: 00-overview.md section 2 rule 4 forbids deriving a ledger from a
// time, it is the identical sin trades.go's own header refuses in the other
// direction, and a day boundary placed by arithmetic would silently move trades
// between the two days whose medians condition 5 compares them against.

package horizon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Keel-Official/keel-backend/internal/domain"
)

// TradeDayQuery bounds one backward walk.
type TradeDayQuery struct {
	// Anchor names the instant the walk counts back from. The walk emits whole
	// UTC days strictly BEFORE the anchor's own day, newest first.
	//
	// It is a time rather than a ledger because the boundary this walk decides on
	// is `ledger_close_time`, which Horizon states in every record, and because
	// no cursor is built from it. Contrast TradeQuery.FromLedger, which seeks and
	// therefore must be a ledger.
	Anchor time.Time

	// MaxDays bounds the walk in calendar days, counting the empty ones. Reaching
	// it is not a finding: DEC-019 section 8.4 item 2 is explicit that a walk
	// which ran out of budget reports `unevaluated` and never "no genuine trade",
	// because only one of those two is a measurement. Zero or negative emits
	// nothing.
	MaxDays int

	// MaxPages bounds the same walk in REQUESTS, and it exists because MaxDays
	// does not bound the cost. A day is cheap for a quiet pair and expensive for
	// a busy one, so a bound in days is a bound in cost only if the two never
	// combine: a pair that is busy AND has no genuine day for weeks pays the
	// busy rate for the full span. HU/USDC did exactly that on 18 September 2026,
	// spending about 2,100 requests of a 3,000 hourly budget on its own and
	// starving the forty pairs behind it in the same pass.
	//
	// Reaching it is not a finding either, for the identical reason MaxDays is
	// not. Zero means no page bound.
	MaxPages int
}

// TradeDay is one COMPLETE UTC day of trades for a pair.
//
// A day with no trades is still a day and is still emitted, because the caller
// counts calendar days against its bound and a quiet day is evidence of quiet
// rather than an absence of evidence.
type TradeDay struct {
	// Start is 00:00:00Z of the day.
	Start time.Time

	// Trades are ASCENDING by paging token, the order trades.go returns and the
	// order domain.ClassifyTrades is written against, even though this walk read
	// them in the opposite one.
	Trades []domain.Trade
}

// End is the exclusive upper bound of the day.
func (d TradeDay) End() time.Time { return d.Start.AddDate(0, 0, 1) }

// TradeDayWalk is what one walk covered and what it cost.
type TradeDayWalk struct {
	Base  domain.Asset
	Quote domain.Asset

	// Days is how many whole days were emitted, empty ones included.
	Days int

	// Oldest is the start of the oldest day emitted, zero when none was.
	Oldest time.Time

	Pages int

	// Truncated is the runaway guard in trades.go having fired, and it is not the
	// same as Exhausted or as reaching MaxDays.
	Truncated bool

	// Exhausted is true when the walk ran off the end of the pair's history AND
	// handed every day of it to the caller. A caller that found nothing over an
	// exhausted walk knows the pair never traded, which is a different statement
	// from running out of budget, so reaching MaxDays clears this even when the
	// last page was read.
	Exhausted bool

	// Stopped is true when the caller's own function ended the walk.
	Stopped bool

	// PageCapReached is MaxPages having ended the walk. It is separate from
	// BoundReached because the two say different things about what was not seen:
	// the day bound means the walk looked as far back as it was allowed, and this
	// means it could not afford to look that far at all.
	PageCapReached bool

	// BoundReached is MaxDays having ended the emission. It is surfaced rather
	// than left for the caller to infer from Days, because the inference is
	// wrong in the one case that matters: a walk that both read the last page
	// and ran out of days has Exhausted cleared, and a caller re-deriving the
	// bound from `!Exhausted && !Stopped` would call an ordinary short history a
	// budget failure.
	BoundReached bool

	// LedgerSeq names where the reading started. It is the Latest-Ledger header
	// of the first page when there is one, and the ledger of the NEWEST trade
	// seen otherwise. Zero when the walk saw no trades at all.
	//
	// THE FALLBACK IS NOT A CONVENIENCE AND THE LIVE PROBE IS WHY IT EXISTS.
	// Public Horizon does not send Latest-Ledger on /trades, measured on
	// 18 September 2026; the fake in this package's tests sends it on every
	// response, so every unit test here passed while the field came back zero
	// against the real server. The newest trade's ledger comes out of its own
	// paging token, which is decoding an identifier rather than deriving one, and
	// it is a better provenance anchor for this walk in any case: it names a
	// ledger that is IN the data rather than the server's tip at the moment of
	// asking.
	LedgerSeq uint32
	ReadAt    time.Time
}

// TradeDayFunc receives one complete day, newest first. Returning false ends the
// walk without an error, which is how a caller says it has the answer it came
// for.
type TradeDayFunc func(TradeDay) (bool, error)

// WalkTradeDays pages /trades backwards and hands the caller whole UTC days.
//
// The pair is pinned in the request for the reason trades.go states: Horizon
// normalises every record to the pinned orientation, and the same endpoint
// queried without one returns the exploit trade with the fraction inverted.
func (c *Client) WalkTradeDays(
	ctx context.Context,
	base, quote domain.Asset,
	q TradeDayQuery,
	fn TradeDayFunc,
) (TradeDayWalk, error) {
	out := TradeDayWalk{Base: base, Quote: quote, ReadAt: c.cfg.Now().UTC()}
	if q.MaxDays <= 0 {
		return out, nil
	}

	// The newest day the walk may emit: the one before the anchor's own.
	next := utcDay(q.Anchor).AddDate(0, 0, -1)

	v := url.Values{}
	addAsset(v, "base", base)
	addAsset(v, "counter", quote)
	v.Set("order", "desc")
	v.Set("limit", strconv.Itoa(tradesPageLimit))

	path, query := "/trades", v
	seen := map[string]bool{}

	// The day being accumulated and its trades, newest first until emitted.
	var cur time.Time
	var buf []domain.Trade

	// hitBound records that MaxDays, rather than the data, ended the emission.
	// It is not the same as Stopped and it CANCELS Exhausted: a collection that
	// was read to its end while the caller was only handed the first three days
	// has not shown the caller everything, and reporting otherwise would let a
	// bounded walk be read as proof that a pair never traded.
	hitBound := false

	// emit hands over every day from next down to and including day, filling the
	// gap with empty ones. It reports whether the walk should continue.
	emit := func(day time.Time, trades []domain.Trade) (bool, error) {
		for !next.Before(day) {
			if out.Days >= q.MaxDays {
				hitBound, out.BoundReached = true, true
				return false, nil
			}
			d := TradeDay{Start: next}
			if next.Equal(day) {
				d.Trades = trades
			}
			out.Days++
			out.Oldest = next
			next = next.AddDate(0, 0, -1)

			cont, err := fn(d)
			if err != nil {
				return false, err
			}
			if !cont {
				out.Stopped = true
				return false, nil
			}
		}
		return true, nil
	}

	for {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		if out.Pages >= maxTradePages {
			out.Truncated = true
			break
		}
		// The caller's own page bound, checked before the request rather than
		// after it, so the cap is what the walk SPENDS and not what it spends
		// plus one.
		if q.MaxPages > 0 && out.Pages >= q.MaxPages {
			out.PageCapReached = true
			break
		}

		body, latest, err := c.get(ctx, path, query, false)
		if err != nil {
			return out, fmt.Errorf("horizon: trade days %s/%s page %d: %w", base, quote, out.Pages+1, err)
		}
		out.Pages++
		if out.Pages == 1 {
			out.LedgerSeq = latest
		}

		var res tradesPage
		if err := json.Unmarshal(body, &res); err != nil {
			return out, fmt.Errorf("horizon: trade days %s/%s page %d: decode: %w", base, quote, out.Pages, err)
		}
		// An empty page is the end of the collection. Horizon serves a next link
		// on every page including the last, so waiting for the link to disappear
		// never stops. Same trap as the forward walk and as /assets.
		if len(res.Embedded.Records) == 0 {
			out.Exhausted = true
			break
		}

		done := false
		for _, r := range res.Embedded.Records {
			t, err := r.trade(base, quote)
			if err != nil {
				return out, fmt.Errorf("horizon: trade days %s/%s record %s: %w", base, quote, r.PagingToken, err)
			}
			if seen[t.ID] {
				continue
			}
			seen[t.ID] = true

			day := utcDay(t.ClosedAt)
			// The anchor's own day and anything after it. Skipped rather than
			// buffered: it is the partial day the header refuses.
			if !day.Before(utcDay(q.Anchor)) {
				continue
			}

			// The newest trade that is actually PART of this reading, which a
			// descending walk meets first. Skipped trades from the anchor's own
			// day do not get to name the provenance of figures they are not in.
			// See the LedgerSeq field comment for why the header is not enough.
			if out.LedgerSeq == 0 {
				out.LedgerSeq = t.LedgerSeq
			}

			switch {
			case cur.IsZero():
				cur, buf = day, []domain.Trade{t}
			case day.Equal(cur):
				buf = append(buf, t)
			default:
				// A record older than cur proves cur has no more trades.
				cont, err := emit(cur, ascending(buf))
				if err != nil {
					return out, err
				}
				if !cont {
					done = true
				}
				cur, buf = day, []domain.Trade{t}
			}
			if done {
				break
			}
		}
		if done {
			return out, nil
		}

		nextLink := strings.TrimSpace(res.Links.Next.Href)
		if nextLink == "" {
			out.Exhausted = true
			break
		}
		u, err := url.Parse(nextLink)
		if err != nil {
			return out, fmt.Errorf("horizon: trade days %s/%s: next link %q: %w", base, quote, nextLink, err)
		}
		// Follow the server's own cursor rather than rebuilding one, which is
		// Horizon's business and has changed shape before.
		path, query = u.Path, u.Query()
	}

	// THE LAST DAY IS ONLY COMPLETE IF THE HISTORY ENDED. Falling out of the loop
	// on the runaway guard leaves a day whose older half was never read, and
	// emitting it would be exactly the partial day this file exists to refuse.
	if !cur.IsZero() && out.Exhausted {
		if _, err := emit(cur, ascending(buf)); err != nil {
			return out, err
		}
	}
	if hitBound {
		out.Exhausted = false
	}
	// An exhausted walk has seen every day that ever existed, including the empty
	// ones older than the first trade. Those are not emitted: a day before the
	// pair's first trade is outside its history rather than quiet inside it.
	return out, nil
}

// utcDay is 00:00:00Z of the day t falls in.
//
// Built with time.Date rather than Truncate(24h). Truncate measures from the zero
// time and happens to agree for UTC, which makes it correct by coincidence and
// wrong the day somebody hands this a non-UTC location.
func utcDay(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}

// ascending reverses a descending buffer in place and returns it.
func ascending(ts []domain.Trade) []domain.Trade {
	for i, j := 0, len(ts)-1; i < j; i, j = i+1, j-1 {
		ts[i], ts[j] = ts[j], ts[i]
	}
	return ts
}
