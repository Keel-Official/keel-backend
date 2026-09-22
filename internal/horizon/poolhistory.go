// Reading a pool's reserves AT A PAST LEDGER, from its own effects.
//
// WHY THIS IS NOT ARITHMETIC. The obvious reconstruction takes the current
// reserves and subtracts every deposit, withdrawal and trade since the target.
// Horizon makes that unnecessary: every effect that touches a pool carries the
// pool's state AFTER it, reserves included. So the reserves at a target ledger are
// the reserves reported by the LAST effect at or before it, read rather than
// derived, and no sum of deltas can drift away from the ledger.
//
// This is the method DEC-013 used by hand for the February evidence, written down
// in the sidecar of docs/evidences/...-pool-evidence-2026-02-22.json as
// "/liquidity_pools/{id}/effects, walked back past the target ledger, filtered by
// liquidity_pool.id". This file is that walk, automated, and the test that matters
// is that it reproduces DEC-013's audited numbers.
//
// THE FILTER IS LOAD-BEARING AND THE ENDPOINT'S NAME HIDES IT.
// /liquidity_pools/{id}/effects returns the effects of every OPERATION that
// touched this pool, which for a path payment includes the effects of the OTHER
// pools in the same path. Reading the first record and trusting its reserves
// reads another pool's state into this one. Measured at ledger 61340263: the two
// most recent records are a native/Vol pool and a Vol/USTRY pool, and the record
// that belongs to the target pool is the third.
//
// WHAT IT CANNOT DO. A pool whose latest effect at or before the target is its
// removal did not exist then, and one with no effect at all did not exist either;
// both are reported as absent rather than as empty, because an empty pool prices
// nothing while an absent one is not part of the market at all. A pool the current
// listing no longer holds cannot be found this way: the listing is the live set,
// so a pool created before the target and fully removed since is invisible here
// and is counted by nothing. That loses AMM liquidity, which reads as a THINNER
// market, the direction this repository errs in.
//
// THE THREE SENTENCES THIS ZONE ASKS FOR. The decision: read the state out of the
// last matching effect, page backwards only far enough to find one, and report
// per pool whether it was found. The alternative rejected: summing deltas back
// from the live reserves, which needs every effect between the target and now
// rather than one, and which turns a missed record into a wrong number instead of
// a missing one. Why: a read is checkable against the chain by opening one effect,
// and a sum is only checkable by repeating it.

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
	"github.com/shopspring/decimal"
)

// PoolAt is one pool's reserves at a past ledger, with the effect they were read
// from so a reader can open it on Horizon.
type PoolAt struct {
	Pool domain.PoolReserves

	// EffectID is the effect's paging token, EffectAt its ledger close time and
	// EffectLedger the ledger it closed in. The reserves are the pool's state
	// after that effect and hold until the next one.
	EffectID     string
	EffectAt     time.Time
	EffectLedger uint32
}

// PoolHistoryWalk is what one reconstruction covered and what it cost.
type PoolHistoryWalk struct {
	// Listed is how many pools the live listing holds for the pair. Resolved is
	// how many of those had reserves at the target.
	Listed   int
	Resolved int

	// Absent names the pools the listing holds that did not exist at the target,
	// either because no effect reaches back that far or because the last one is a
	// removal. Reported rather than dropped: "this pool was not there" and "this
	// pool was not found" are different statements and only the first is a
	// measurement.
	Absent []string

	// Unreached names the pools whose last matching effect was not found within
	// the page bound. Those are a budget failure, not a finding, and the caller
	// must not read the result as complete when this is non-empty.
	Unreached []string

	Pages int
}

// Complete reports whether every listed pool was resolved or proven absent.
func (w PoolHistoryWalk) Complete() bool { return len(w.Unreached) == 0 }

// poolEffectsPageLimit is the page this walk asks for. Ten is enough for the
// matching record to arrive on the first page in the ordinary case, where a
// pool's own effect sits within a few records of the operation's other effects,
// and small enough that public Horizon answers it: limit=20 on this endpoint
// returned 503 twice on 22 September 2026 while limit=10 answered.
const poolEffectsPageLimit = 10

// poolEffectTypes are the effects that state a pool's reserves.
var poolEffectTypes = map[string]bool{
	"liquidity_pool_created":   true,
	"liquidity_pool_deposited": true,
	"liquidity_pool_withdrew":  true,
	"liquidity_pool_trade":     true,
	"liquidity_pool_revoked":   true,
}

// PoolReservesAt returns the pair's pools as they stood at the target ledger.
//
// maxPages bounds each pool's backward read in requests; zero or negative uses a
// small default. The pools come from the live listing, which is the same source
// GetSnapshot uses, so a pool that no longer exists is outside what this can see.
func (c *Client) PoolReservesAt(
	ctx context.Context,
	base, quote domain.Asset,
	target uint32,
	maxPages int,
) ([]PoolAt, PoolHistoryWalk, error) {
	var walk PoolHistoryWalk
	if maxPages <= 0 {
		maxPages = 5
	}

	q := url.Values{}
	q.Set("reserves", horizonAsset(base)+","+horizonAsset(quote))
	q.Set("limit", strconv.Itoa(poolPageLimit))
	body, _, err := c.get(ctx, "/liquidity_pools", q, false)
	if err != nil {
		return nil, walk, fmt.Errorf("horizon: pools %s/%s: %w", base, quote, err)
	}
	walk.Pages++

	var listed poolsResponse
	if err := json.Unmarshal(body, &listed); err != nil {
		return nil, walk, fmt.Errorf("horizon: decode pools %s/%s: %w", base, quote, err)
	}
	if len(listed.Embedded.Records) >= poolPageLimit {
		return nil, walk, fmt.Errorf("horizon: %d pools returned for one pair, which fills the page; paging is not implemented",
			len(listed.Embedded.Records))
	}
	walk.Listed = len(listed.Embedded.Records)

	var out []PoolAt
	for _, rec := range listed.Embedded.Records {
		at, pages, err := c.poolAt(ctx, rec.ID, base, quote, target, maxPages)
		walk.Pages += pages
		if err != nil {
			return nil, walk, err
		}
		switch {
		case at == nil:
			walk.Absent = append(walk.Absent, rec.ID)
		case at.EffectID == "":
			walk.Unreached = append(walk.Unreached, rec.ID)
		default:
			walk.Resolved++
			out = append(out, *at)
		}
	}
	return out, walk, nil
}

// poolAt reads one pool's state at the target. A nil result with no error means
// the pool did not exist then; a result with an empty EffectID means the page
// bound was reached before an answer.
func (c *Client) poolAt(
	ctx context.Context,
	poolID string,
	base, quote domain.Asset,
	target uint32,
	maxPages int,
) (*PoolAt, int, error) {
	// The cursor is the first TOID of the ledger AFTER the target, so a descending
	// read starts with the newest effect at or before it. The same construction
	// the known-removal event uses, and it is an identifier rather than a time.
	q := url.Values{}
	q.Set("order", "desc")
	q.Set("limit", strconv.Itoa(poolEffectsPageLimit))
	q.Set("cursor", strconv.FormatInt(int64(target+1)<<32, 10))

	path, query := "/liquidity_pools/"+poolID+"/effects", q
	pages := 0
	for pages < maxPages {
		body, _, err := c.get(ctx, path, query, false)
		if err != nil {
			return nil, pages, fmt.Errorf("horizon: pool %s effects: %w", poolID, err)
		}
		pages++

		var res poolEffectsPage
		if err := json.Unmarshal(body, &res); err != nil {
			return nil, pages, fmt.Errorf("horizon: decode pool %s effects: %w", poolID, err)
		}
		if len(res.Embedded.Records) == 0 {
			// The end of the collection with nothing matching: the pool has no
			// effect at or before the target, so it did not exist then.
			return nil, pages, nil
		}

		for _, e := range res.Embedded.Records {
			if e.LiquidityPool.ID != poolID {
				continue // another pool in the same path payment
			}
			if e.Type == "liquidity_pool_removed" {
				return nil, pages, nil
			}
			if !poolEffectTypes[e.Type] {
				continue
			}
			at, err := e.poolAt(base, quote)
			if err != nil {
				return nil, pages, err
			}
			return at, pages, nil
		}

		next := strings.TrimSpace(res.Links.Next.Href)
		if next == "" {
			return nil, pages, nil
		}
		u, err := url.Parse(next)
		if err != nil {
			return nil, pages, fmt.Errorf("horizon: pool %s effects: next link %q: %w", poolID, next, err)
		}
		path, query = u.Path, u.Query()
	}
	return &PoolAt{}, pages, nil
}

type poolEffectsPage struct {
	Links struct {
		Next struct {
			Href string `json:"href"`
		} `json:"next"`
	} `json:"_links"`
	Embedded struct {
		Records []poolEffectRecord `json:"records"`
	} `json:"_embedded"`
}

type poolEffectRecord struct {
	PagingToken   string    `json:"paging_token"`
	Type          string    `json:"type"`
	CreatedAt     time.Time `json:"created_at"`
	LiquidityPool struct {
		ID       string `json:"id"`
		FeeBP    int32  `json:"fee_bp"`
		Reserves []struct {
			Asset  string `json:"asset"`
			Amount string `json:"amount"`
		} `json:"reserves"`
	} `json:"liquidity_pool"`
}

// poolAt turns one effect into the pool state it reports.
func (e poolEffectRecord) poolAt(base, quote domain.Asset) (*PoolAt, error) {
	wantBase, wantQuote := horizonAsset(base), horizonAsset(quote)
	var rb, rq *decimal.Decimal
	for _, r := range e.LiquidityPool.Reserves {
		amount, err := decimal.NewFromString(r.Amount)
		if err != nil {
			return nil, fmt.Errorf("horizon: pool %s effect %s: reserve %q: %w", e.LiquidityPool.ID, e.PagingToken, r.Amount, err)
		}
		switch r.Asset {
		case wantBase:
			v := amount
			rb = &v
		case wantQuote:
			v := amount
			rq = &v
		}
	}
	if rb == nil || rq == nil {
		return nil, fmt.Errorf("horizon: pool %s effect %s does not hold both %s and %s",
			e.LiquidityPool.ID, e.PagingToken, wantBase, wantQuote)
	}
	return &PoolAt{
		Pool: domain.PoolReserves{
			PoolID:       e.LiquidityPool.ID,
			ReserveBase:  *rb,
			ReserveQuote: *rq,
			FeeBP:        e.LiquidityPool.FeeBP,
		},
		EffectID:     e.PagingToken,
		EffectAt:     e.CreatedAt,
		EffectLedger: effectLedger(e.PagingToken),
	}, nil
}

// effectLedger reads the ledger out of an effect's paging token, which is a TOID
// followed by the effect's index within its operation. Zero when it cannot be
// read, because a provenance field that guesses is worse than one that is empty.
func effectLedger(token string) uint32 {
	toid, _, _ := strings.Cut(token, "-")
	v, err := strconv.ParseInt(toid, 10, 64)
	if err != nil || v <= 0 {
		return 0
	}
	return uint32(v >> 32)
}
