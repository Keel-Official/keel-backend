// Rebuilding a RECENT order book by rewinding the live one.
//
// WHY A SECOND WAY TO DO THE SAME THING. replay.go walks operations forward from
// nothing, which is the only route to a book six months old and is priced by how
// busy the ACCOUNTS are rather than by how busy the pair is. Measured on
// 26 August 2026: three of the quietest pairs in the demonstration set, at a
// target seven hours old with a seven day operation floor, did not finish in ten
// minutes. The bots that trade a sleepy pair are not sleepy.
//
// For a target that is HOURS old there is a far cheaper route, and one field makes
// it work. `/offers` returns every resting offer with its id, its exact price, its
// amount AND its `last_modified_ledger`. An offer whose last modification is at or
// before the target has not moved since: it is resting now, it was resting then,
// and the state Horizon reports now IS its state at the target. No walk, no
// decoding, no inference.
//
// Measured across the sixty committed recordings, target seven hours back:
// 3,592 of 3,917 resting offers had not changed, and 38 of the 60 pairs had not
// changed at ALL. The expensive half of this problem is a small minority of the
// book.
//
// WHAT THIS CANNOT SEE, and each is counted rather than assumed away.
//
//   - An offer that changed AFTER the target. Its current state is not its state
//     then, and this file does not guess: it is counted as unresolved and left
//     off the book. That loses a level, so the book comes back THINNER.
//   - An offer that was resting at the target and is gone now. It is not in
//     `/offers` at all. A trade in the window names it, so it can be COUNTED, and
//     it is not put back: a trade cannot say whether the offer it names was on the
//     book at the target or was created after it, and guessing wrong adds a level
//     that never existed. When no trade names it, it is invisible and cannot even
//     be counted.
//   - An offer cancelled in the window that never traded. The case above with
//     nothing to catch it. `keel replay` is the route that sees those, at the cost
//     this file exists to avoid.
//
// EVERY ONE OF THOSE RUNS IN THE SAME DIRECTION, and that is the property worth
// having. They lose levels. A book that is missing a level reads as a THINNER
// market, and thin is the conservative side for a product whose job is to warn.
// The failure mode replay.go had to be guarded against, a book that comes back
// DEEPER than it was, cannot arise here: nothing is ever put on this book that
// Horizon does not currently report as resting AND that has not moved since the
// target. An earlier draft did add departed offers back from what they sold, and
// that is exactly how the deeper failure gets in, so it was removed rather than
// bounded.
//
// THE THREE SENTENCES THIS ZONE ASKS FOR. The decision: the current offer set is
// the starting point and `last_modified_ledger` decides, per offer, whether it can
// be carried back unchanged, with everything else counted rather than
// reconstructed. The alternative rejected: rewinding the window's operations in
// reverse, un-applying each one to recover the previous state. Why it was
// rejected: un-applying an UPDATE needs the state BEFORE it, which no result
// carries, so every reversed update sends you looking for the operation before it
// and the walk degenerates into the forward problem this file exists to escape,
// while `last_modified_ledger` answers the same question for free and for the 92
// percent of offers that never moved.
package horizon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Keel-Official/keel-backend/internal/domain"
	"github.com/shopspring/decimal"
)

// offersPageLimit is the largest page /offers serves.
const offersPageLimit = 200

// maxOfferPages bounds one side's walk over /offers. Forty pages is eight
// thousand resting offers on one side of one pair, which is a runaway guard
// rather than a limit any real market meets.
const maxOfferPages = 40

// RewindResult is a book carried back from the live one, and every reason not to
// trust it.
type RewindResult struct {
	Snapshot domain.Snapshot

	// NowLedger is the ledger the live reading was served at. The distance from
	// the target is the window everything below is about.
	NowLedger uint32

	// Carried is the number of resting offers whose last modification is at or
	// before the target, so their current state IS their state then.
	Carried int

	// Changed is the number of resting offers modified after the target. Their
	// state then is not recoverable from a live reading, so they are LEFT OFF the
	// book and counted here. Each one is a level this book does not have.
	Changed int

	// Gone is the number of offers that a trade in the window named and that are
	// no longer resting. Each one is a level this book does not have.
	//
	// THEY ARE COUNTED AND NOT RECONSTRUCTED, and the first version of this file
	// did reconstruct them, from the amount they sold and the price the trade
	// reports. That was wrong in the direction this whole file is built to avoid.
	// A trade cannot say whether the offer it names existed at the TARGET or was
	// created after it, and putting an offer that was created afterwards onto the
	// book adds a level that was never there. Measured before the fix: AFR came
	// back with 99 ask levels against a recording of 77.
	Gone int

	TradesRead int
	Requests   int
	ReadAt     time.Time
}

// Certain reports whether every offer on this book was carried back unchanged,
// with nothing modified in the window and nothing inferred from a trade.
//
// It is NOT a claim of correctness. An offer that was cancelled in the window
// without ever trading is invisible to this method and cannot be counted, so a
// certain book can still be missing a level. What it does say is that no offer on
// it was guessed at.
func (r RewindResult) Certain() bool { return r.Changed == 0 && r.Gone == 0 }

// RewindBook rebuilds a pair's book at a recent ledger from the live offer set.
func (c *Client) RewindBook(ctx context.Context, base, quote domain.Asset, target uint32) (RewindResult, error) {
	out := RewindResult{ReadAt: c.cfg.Now().UTC()}
	if target == 0 {
		return out, fmt.Errorf("horizon: rewind needs a target ledger")
	}
	before := c.Requests()

	live, nowLedger, err := c.liveOffers(ctx, base, quote)
	if err != nil {
		return out, err
	}
	out.NowLedger = nowLedger
	if nowLedger != 0 && target > nowLedger {
		return out, fmt.Errorf("horizon: target ledger %d is ahead of the network at %d", target, nowLedger)
	}

	// Trades from the target onward. They do two jobs: they name offers that have
	// since disappeared, and they say how much each one sold.
	trades, err := c.Trades(ctx, base, quote, TradeQuery{FromLedger: target})
	if err != nil {
		return out, err
	}
	out.TradesRead = len(trades.Trades)

	state := map[int64]*restingOffer{}
	present := map[int64]bool{}

	for _, o := range live {
		present[o.ID] = true
		if o.LastModifiedLedger > target {
			// It moved after the target, so what Horizon reports now is not what
			// it held then. Leaving it off loses a level; guessing at it could
			// add one that was not there.
			out.Changed++
			continue
		}
		out.Carried++
		state[o.ID] = &restingOffer{
			ID:      o.ID,
			Selling: o.Selling,
			Buying:  o.Buying,
			Amount:  o.Amount,
			PriceN:  o.PriceN,
			PriceD:  o.PriceD,
		}
	}

	// Offers a window trade names that are no longer resting. Some of them were on
	// the book at the target and some were created after it, and a trade cannot
	// tell those apart, so all of them are counted and none is added.
	out.Gone = countGoneOffers(trades.Trades, target, present)

	out.Snapshot = domain.Snapshot{
		Base:      base,
		Quote:     quote,
		LedgerSeq: target,
		Book:      bookFromOffers(state, base, quote),
		// Pools are not rewound. Same position as replay.go: the absence of a pool
		// here is not a claim that none existed.
		Source: domain.DataSourceOffersImplied,
	}
	out.Requests = c.Requests() - before
	return out, nil
}

// countGoneOffers counts the offers a window trade names that are no longer
// resting.
//
// It counts and returns nothing else on purpose. The amount such an offer sold is
// a real lower bound on what it held, and putting that on the book was the first
// version of this file; it is wrong because a trade cannot say whether the offer
// existed at the TARGET. Synthetic ids are skipped: they name a taker that never
// rested, so there is no offer to be missing.
func countGoneOffers(trades []domain.Trade, target uint32, present map[int64]bool) int {
	seen := map[int64]bool{}
	for _, t := range trades {
		if t.LedgerSeq < target {
			continue
		}
		for _, raw := range [2]string{t.BaseOfferID, t.CounterOfferID} {
			id, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || id == 0 || syntheticOffer(id) || present[id] || seen[id] {
				continue
			}
			seen[id] = true
		}
	}
	return len(seen)
}

// ---------------------------------------------------------------- /offers

// liveOffer is one resting offer as Horizon reports it right now.
type liveOffer struct {
	ID                 int64
	Selling            assetRef
	Buying             assetRef
	Amount             decimal.Decimal
	PriceN             int64
	PriceD             int64
	LastModifiedLedger uint32
}

// liveOffers reads every resting offer on the pair, both directions, paged.
//
// BOTH DIRECTIONS, because a bid is an offer SELLING the quote asset and
// /offers filters on what an offer sells. Asking one way round returns half a
// book and no error, which is the same shape as trap 4 in this zone's CLAUDE.md.
func (c *Client) liveOffers(ctx context.Context, base, quote domain.Asset) ([]liveOffer, uint32, error) {
	var out []liveOffer
	var latest uint32

	for _, side := range [2][2]domain.Asset{{base, quote}, {quote, base}} {
		v := url.Values{}
		addAsset(v, "selling", side[0])
		addAsset(v, "buying", side[1])
		v.Set("limit", strconv.Itoa(offersPageLimit))

		path, query, pages := "/offers", v, 0
		for {
			if err := ctx.Err(); err != nil {
				return nil, latest, err
			}
			if pages >= maxOfferPages {
				return nil, latest, fmt.Errorf(
					"horizon: offers %s/%s exceeded %d pages, which is %d resting offers on one side",
					side[0], side[1], maxOfferPages, maxOfferPages*offersPageLimit)
			}

			body, served, err := c.get(ctx, path, query, false)
			if err != nil {
				return nil, latest, fmt.Errorf("horizon: offers %s/%s page %d: %w", side[0], side[1], pages+1, err)
			}
			pages++
			// The FIRST page of the FIRST side stamps the reading. A multi page
			// walk straddles ledger closes, so this names where it started rather
			// than pretending it was atomic, the same convention as everywhere
			// else in this package.
			if latest == 0 {
				latest = served
			}

			var page offersFullPage
			if err := json.Unmarshal(body, &page); err != nil {
				return nil, latest, fmt.Errorf("horizon: offers %s/%s page %d: decode: %w", side[0], side[1], pages, err)
			}
			if len(page.Embedded.Records) == 0 {
				break
			}
			for _, r := range page.Embedded.Records {
				o, err := r.offer()
				if err != nil {
					return nil, latest, fmt.Errorf("horizon: offers %s/%s: offer %s: %w", side[0], side[1], r.ID, err)
				}
				out = append(out, o)
			}

			next := strings.TrimSpace(page.Links.Next.Href)
			if next == "" {
				break
			}
			u, err := url.Parse(next)
			if err != nil {
				return nil, latest, fmt.Errorf("horizon: offers %s/%s: next link %q: %w", side[0], side[1], next, err)
			}
			path, query = u.Path, u.Query()
		}
	}

	// Sorted, so two runs over the same ledger produce the same order. NFR-9
	// reaches this file too.
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, latest, nil
}

type offersFullPage struct {
	Links struct {
		Next struct {
			Href string `json:"href"`
		} `json:"next"`
	} `json:"_links"`
	Embedded struct {
		Records []offerRecord `json:"records"`
	} `json:"_embedded"`
}

// offerRecord is one /offers record.
//
// price_r arrives as JSON NUMBERS here, the same shape as /offers levels and the
// opposite of /trades. priceFraction reads both and that is why it exists.
type offerRecord struct {
	ID      string   `json:"id"`
	Seller  string   `json:"seller"`
	Selling assetRef `json:"selling"`
	Buying  assetRef `json:"buying"`
	Amount  string   `json:"amount"`
	PriceR  struct {
		N int64 `json:"n"`
		D int64 `json:"d"`
	} `json:"price_r"`

	// LastModifiedLedger is the field this whole file turns on. It moves when an
	// operation changes the offer AND when a trade partially fills it, so it
	// catches both ways an offer's state can differ from what it was.
	LastModifiedLedger uint32 `json:"last_modified_ledger"`
}

func (r offerRecord) offer() (liveOffer, error) {
	var o liveOffer
	id, err := strconv.ParseInt(r.ID, 10, 64)
	if err != nil {
		return o, fmt.Errorf("id %q: %w", r.ID, err)
	}
	amount, err := decimal.NewFromString(r.Amount)
	if err != nil {
		return o, fmt.Errorf("amount %q: %w", r.Amount, err)
	}
	if r.PriceR.N <= 0 || r.PriceR.D <= 0 {
		return o, fmt.Errorf("price %d/%d is not usable", r.PriceR.N, r.PriceR.D)
	}
	return liveOffer{
		ID:                 id,
		Selling:            r.Selling,
		Buying:             r.Buying,
		Amount:             amount,
		PriceN:             r.PriceR.N,
		PriceD:             r.PriceR.D,
		LastModifiedLedger: r.LastModifiedLedger,
	}, nil
}
