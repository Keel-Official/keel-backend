// Reading the most recent stretch of /trades, back to an instant.
//
// WHY A THIRD SHAPE OVER ONE ENDPOINT. trades.go walks forward from a ledger seek
// for a window known in advance, and tradedays.go walks backward in whole UTC days
// and refuses the current one. The oracle window at a LIVE ledger needs neither:
// it needs every trade from a known instant up to now, inside a day that has not
// ended, and it needs them every scan round. DEC-019 section 8.4 item 3 already
// names "different shapes over one endpoint" as the pattern, and this is the third.
//
// WHAT IT DOES NOT DO IS JUDGE. It returns the trades and says whether the set is
// proven complete back to `since`. Classifying them, and choosing the median
// condition 5 reads, is domain.ClassifyTradesAgainstMedian's business.
//
// THE THREE SENTENCES THIS ZONE ASKS FOR.
//
// The decision: page `order=desc` from the newest trade and call the set complete
// only when a record OLDER than `since` has been seen or the history ran out, so
// completeness is proven by a record rather than assumed from a page count, the
// same rule WalkTradeDays uses for a day boundary. The alternative rejected:
// asking WalkTradeDays for today, which it deliberately cannot give, and relaxing
// that rule would reopen the partial-day median DEC-019 section 8.2 refuses for
// every caller rather than for the one that is prepared to handle it. Why: the
// refusal is correct for staleness and wrong only for a window that ends now, so
// the new need gets its own function and the old guarantee stays whole.

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

// RecentTradesWalk is what one backward read covered and what it cost.
type RecentTradesWalk struct {
	Pages int

	// Complete is true when every trade at or after `since` is in the result:
	// a record older than `since` was seen, or the pair's history ended first.
	// Anything else is a partial set, and a volume summed over it would be a
	// guess in the shape of a measurement.
	Complete bool

	// PageCapReached is the caller's own bound ending the read before it was
	// complete. It is separate from Complete being false for the reason
	// TradeDayWalk keeps it separate: it says the read could not afford to look
	// back far enough, not that something went wrong.
	PageCapReached bool
}

// RecentTrades returns every trade on the pair closed at or after since,
// ASCENDING by paging token, the order domain.ClassifyTrades is written against.
//
// maxPages bounds the read in requests; zero or negative means no bound beyond
// the runaway guard trades.go already carries.
func (c *Client) RecentTrades(
	ctx context.Context,
	base, quote domain.Asset,
	since time.Time,
	maxPages int,
) ([]domain.Trade, RecentTradesWalk, error) {
	var out RecentTradesWalk

	v := url.Values{}
	addAsset(v, "base", base)
	addAsset(v, "counter", quote)
	v.Set("order", "desc")
	v.Set("limit", strconv.Itoa(tradesPageLimit))

	path, query := "/trades", v
	seen := map[string]bool{}
	var newestFirst []domain.Trade

	for {
		if err := ctx.Err(); err != nil {
			return nil, out, err
		}
		if out.Pages >= maxTradePages {
			return nil, out, fmt.Errorf("horizon: recent trades %s/%s: runaway guard at %d pages", base, quote, out.Pages)
		}
		if maxPages > 0 && out.Pages >= maxPages {
			out.PageCapReached = true
			return ascending(newestFirst), out, nil
		}

		body, _, err := c.get(ctx, path, query, false)
		if err != nil {
			return nil, out, fmt.Errorf("horizon: recent trades %s/%s page %d: %w", base, quote, out.Pages+1, err)
		}
		out.Pages++

		var res tradesPage
		if err := json.Unmarshal(body, &res); err != nil {
			return nil, out, fmt.Errorf("horizon: recent trades %s/%s page %d: decode: %w", base, quote, out.Pages, err)
		}
		// An empty page is the end of the collection; Horizon serves a next link
		// on every page including the last. Same trap as the other two walks.
		if len(res.Embedded.Records) == 0 {
			out.Complete = true
			return ascending(newestFirst), out, nil
		}

		for _, r := range res.Embedded.Records {
			t, err := r.trade(base, quote)
			if err != nil {
				return nil, out, fmt.Errorf("horizon: recent trades %s/%s record %s: %w", base, quote, r.PagingToken, err)
			}
			if seen[t.ID] {
				continue
			}
			seen[t.ID] = true
			// A record older than since proves nothing newer is missing.
			if t.ClosedAt.Before(since) {
				out.Complete = true
				return ascending(newestFirst), out, nil
			}
			newestFirst = append(newestFirst, t)
		}

		next := strings.TrimSpace(res.Links.Next.Href)
		if next == "" {
			out.Complete = true
			return ascending(newestFirst), out, nil
		}
		u, err := url.Parse(next)
		if err != nil {
			return nil, out, fmt.Errorf("horizon: recent trades %s/%s: next link %q: %w", base, quote, next, err)
		}
		path, query = u.Path, u.Query()
	}
}
