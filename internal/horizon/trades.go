// Reading /trades for one pair, and nothing else.
//
// WHY THIS EXISTS. Trap 3 of this zone's CLAUDE.md says Horizon serves no
// historical data, and that is true of STATE: no order book and no balance at a
// past ledger. It is not true of EVENTS. /trades is complete back to the asset's
// first trade, which is what DEC-002 section 1 records and what makes the
// substitutes in its section 2 possible at all. This file reads that stream and
// converts nothing beyond the exact rational price and the two amounts.
//
// IT GATHERS AND NEVER SELECTS. There is no genuine-trade rule here, no dust
// threshold, no account filter and no bucketing. Those are
// docs/methodology/07-supporting-metrics.md section 1, which is red and
// unwritten, and a comparison against a constant that decided whether a trade
// counted would be that document written here by the wrong hand. Every record
// the endpoint returns for the pair is returned to the caller.
//
// THE THREE SENTENCES THIS ZONE ASKS FOR.
//
// The decision: the walk seeks with a LEDGER, follows Horizon's own _links.next
// afterwards, and stops on a predicate the caller supplies over each decoded
// trade. The alternative rejected: taking a start and end TIME and converting
// them into ledger sequences to build both cursors, which is the obvious shape
// for "give me February". Why it was rejected: 00-overview.md section 2 rule 4
// forbids deriving time from a ledger sequence arithmetically, and the inverse
// is the same sin with the same error, roughly three weeks of drift over six
// months. A ledger is an honest SEEK because being early only costs requests,
// while the window boundary is decided on ledger_close_time, which is a fact
// Horizon states in every record.
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

// tradesPageLimit is the largest page Horizon serves. It is a round-trip
// economy, not a cap: the walk follows _links.next past it.
const tradesPageLimit = 200

// maxTradePages bounds one walk. At 200 records a page this is two million
// trades, which is a runaway guard rather than a limit anybody meets. A walk
// that reaches it reports Truncated and the caller must say so.
const maxTradePages = 10000

// TradeQuery bounds one walk over /trades.
type TradeQuery struct {
	// FromLedger seeks the start of the walk. Horizon's cursor is a TOID whose
	// high 32 bits are the ledger sequence, so a ledger names a starting point
	// without this package converting a time into one. Zero starts at the
	// beginning of the pair's history.
	//
	// BEING EARLY IS FREE AND BEING LATE IS NOT. An under-estimate costs extra
	// pages that the caller then discards on ledger_close_time. An
	// over-estimate silently omits trades from the front of the window, and
	// nothing downstream can tell that happened.
	FromLedger uint32

	// StopAfter ends the walk at the first trade for which it returns true. That
	// trade is NOT included. It is a predicate over the decoded record rather
	// than a cursor so that a caller can stop on ledger_close_time, which is the
	// only honest clock this endpoint has.
	StopAfter func(domain.Trade) bool
}

// TradeReading is one walk over /trades, with what it cost.
type TradeReading struct {
	Base  domain.Asset
	Quote domain.Asset

	// Trades in the order Horizon served them, which is ascending by paging
	// token. The caller may sort; domain.TradeImpliedDepthBounds does.
	Trades []domain.Trade

	Pages     int
	Truncated bool

	// Stopped is true when StopAfter ended the walk, and false when the walk ran
	// off the end of the collection. The difference matters: a window that
	// ended because the data ended is not a closed window.
	Stopped bool

	// LedgerSeq is the Latest-Ledger of the FIRST page. A multi-page walk
	// straddles ledger closes, so this names where the reading started rather
	// than pretending it was atomic. Same convention as CodeReading.
	LedgerSeq uint32
	ReadAt    time.Time
}

// Trades walks /trades for one pair.
//
// The pair is pinned in the request, so Horizon normalises every record to that
// orientation and the price arrives as quote per base. THAT IS CHECKED RATHER
// THAN ASSUMED on every record, because the same endpoint queried without a
// pinned pair returns the exploit trade with USDC as the base and the fraction
// inverted, which is the case orient() in price.go was written for.
func (c *Client) Trades(ctx context.Context, base, quote domain.Asset, q TradeQuery) (TradeReading, error) {
	out := TradeReading{Base: base, Quote: quote, ReadAt: c.cfg.Now().UTC()}

	v := url.Values{}
	addAsset(v, "base", base)
	addAsset(v, "counter", quote)
	v.Set("order", "asc")
	v.Set("limit", strconv.Itoa(tradesPageLimit))
	if q.FromLedger > 0 {
		// The TOID of the first operation that could exist in that ledger. Left
		// shifting the sequence is the encoding Horizon itself uses, and it is
		// an identifier, not a time.
		v.Set("cursor", strconv.FormatUint(uint64(q.FromLedger)<<32, 10))
	}

	path, query := "/trades", v
	seen := map[string]bool{}

	for {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		if out.Pages >= maxTradePages {
			out.Truncated = true
			break
		}

		body, latest, err := c.get(ctx, path, query, false)
		if err != nil {
			return out, fmt.Errorf("horizon: trades %s/%s page %d: %w", base, quote, out.Pages+1, err)
		}
		out.Pages++
		if out.Pages == 1 {
			out.LedgerSeq = latest
		}

		var res tradesPage
		if err := json.Unmarshal(body, &res); err != nil {
			return out, fmt.Errorf("horizon: trades %s/%s page %d: decode: %w", base, quote, out.Pages, err)
		}
		// An EMPTY page is the end of the collection. Horizon serves a next link
		// on every page including the last, so stopping when the link disappears
		// never stops. Same trap as the /assets walk in universe.go.
		if len(res.Embedded.Records) == 0 {
			break
		}

		stop := false
		for _, r := range res.Embedded.Records {
			t, err := r.trade(base, quote)
			if err != nil {
				return out, fmt.Errorf("horizon: trades %s/%s record %s: %w", base, quote, r.PagingToken, err)
			}
			// A paging token identifies one trade. Horizon has not been seen to
			// repeat one across pages, and a duplicated record would silently
			// double a volume figure and add a zero-delta pair to the bounds.
			if seen[t.ID] {
				continue
			}
			seen[t.ID] = true

			if q.StopAfter != nil && q.StopAfter(t) {
				stop = true
				break
			}
			out.Trades = append(out.Trades, t)
		}
		if stop {
			out.Stopped = true
			break
		}

		next := strings.TrimSpace(res.Links.Next.Href)
		if next == "" {
			break
		}
		u, err := url.Parse(next)
		if err != nil {
			return out, fmt.Errorf("horizon: trades %s/%s: next link %q: %w", base, quote, next, err)
		}
		// Follow the server's own cursor rather than rebuilding one. The cursor
		// format is Horizon's business and has changed shape before.
		path, query = u.Path, u.Query()
	}
	return out, nil
}

// tradesPage is the paged /trades response.
type tradesPage struct {
	Links struct {
		Next struct {
			Href string `json:"href"`
		} `json:"next"`
	} `json:"_links"`
	Embedded struct {
		Records []tradeRecord `json:"records"`
	} `json:"_embedded"`
}

// tradeRecord is one /trades record.
//
// Price is `price` here and not `price_r`, and that is not trap 2. On this
// endpoint the rational IS the field called price, shaped {"n","d"} with STRING
// members, while /offers spells the same thing price_r with number members.
// priceFraction reads both, which is the whole reason it exists.
type tradeRecord struct {
	PagingToken     string        `json:"paging_token"`
	LedgerCloseTime time.Time     `json:"ledger_close_time"`
	TradeType       string        `json:"trade_type"`
	Price           priceFraction `json:"price"`

	BaseAccount    string `json:"base_account"`
	BaseOfferID    string `json:"base_offer_id"`
	BaseAmount     string `json:"base_amount"`
	BaseAssetType  string `json:"base_asset_type"`
	BaseAssetCode  string `json:"base_asset_code"`
	BaseAssetIssue string `json:"base_asset_issuer"`

	CounterAccount    string `json:"counter_account"`
	CounterOfferID    string `json:"counter_offer_id"`
	CounterAmount     string `json:"counter_amount"`
	CounterAssetType  string `json:"counter_asset_type"`
	CounterAssetCode  string `json:"counter_asset_code"`
	CounterAssetIssue string `json:"counter_asset_issuer"`

	// THE POOL IS NAMED BY SIDE AND THERE IS NO BARE liquidity_pool_id. Horizon
	// sends base_liquidity_pool_id or counter_liquidity_pool_id and never both:
	// 171 and 29 of 200 pool trades on this pair, 0 with both, 0 with neither.
	// A tag reading `liquidity_pool_id` matches nothing, and encoding/json
	// reports no error for a key it was never asked for, which is why this was
	// the empty string on all 58 pool trades of the February CSV. See
	// docs/evidences/2026-08-31-trade-pool-id-defect.md.
	BaseLiquidityPoolID    string `json:"base_liquidity_pool_id"`
	CounterLiquidityPoolID string `json:"counter_liquidity_pool_id"`

	// Fee in basis points, sent only on a pool trade. A pool trade's price is
	// quoted after this fee, so the genuine-trade rule in 07 section 1 has to
	// decide whether it cares. Recorded rather than interpreted.
	LiquidityPoolFeeBP int `json:"liquidity_pool_fee_bp"`

	// BaseIsSeller is what separates a book walk from a path payment that buys
	// the base asset on one venue and sells it on another inside one operation.
	// domain.TradeImpliedDepthBounds groups on it; see the header of
	// internal/domain/trades.go.
	BaseIsSeller bool `json:"base_is_seller"`
}

func (r tradeRecord) baseRef() assetRef {
	return assetRef{AssetType: r.BaseAssetType, AssetCode: r.BaseAssetCode, AssetIssuer: r.BaseAssetIssue}
}

func (r tradeRecord) counterRef() assetRef {
	return assetRef{AssetType: r.CounterAssetType, AssetCode: r.CounterAssetCode, AssetIssuer: r.CounterAssetIssue}
}

// poolID returns the pool this trade touched, or "" for an orderbook trade.
//
// It does NOT infer the pool from an empty account. An orderbook trade with a
// missing account would then be read as a pool trade, and the two are not the
// same claim. Horizon states the side; this reads what it stated.
func (r tradeRecord) poolID() string {
	if r.BaseLiquidityPoolID != "" {
		return r.BaseLiquidityPoolID
	}
	return r.CounterLiquidityPoolID
}

// poolSide reports which side of the trade the pool was, "base", "counter", or
// "" when no pool was involved.
func (r tradeRecord) poolSide() string {
	switch {
	case r.BaseLiquidityPoolID != "":
		return "base"
	case r.CounterLiquidityPoolID != "":
		return "counter"
	default:
		return ""
	}
}

// trade converts one record, refusing anything it cannot orient.
//
// THE ORIENTATION CHECK IS THE POINT. A record whose base is the asset we asked
// for as the QUOTE carries the fraction upside down, and a silently inverted
// price is a hundredfold error that every downstream number inherits. Rather
// than flipping it with orient() and hoping, this refuses: the pair is pinned in
// the request, so a record that disagrees means Horizon answered a different
// question and the safe reading of that is an error, not a repair.
func (r tradeRecord) trade(base, quote domain.Asset) (domain.Trade, error) {
	var t domain.Trade

	if !r.baseRef().matches(base) || !r.counterRef().matches(quote) {
		return t, fmt.Errorf("%w: asked %s/%s, got %s/%s",
			ErrPairMismatch, base, quote, r.baseRef().describe(), r.counterRef().describe())
	}

	p, err := r.Price.price()
	if err != nil {
		return t, fmt.Errorf("price: %w", err)
	}
	baseAmt, err := decimal.NewFromString(r.BaseAmount)
	if err != nil {
		return t, fmt.Errorf("base_amount %q: %w", r.BaseAmount, err)
	}
	counterAmt, err := decimal.NewFromString(r.CounterAmount)
	if err != nil {
		return t, fmt.Errorf("counter_amount %q: %w", r.CounterAmount, err)
	}

	opID, fill := splitPagingToken(r.PagingToken)

	return domain.Trade{
		ID:                 r.PagingToken,
		OperationID:        opID,
		FillIndex:          fill,
		BaseIsSeller:       r.BaseIsSeller,
		LedgerSeq:          ledgerFromPagingToken(r.PagingToken),
		ClosedAt:           r.LedgerCloseTime.UTC(),
		Type:               r.TradeType,
		Price:              p,
		BaseAmount:         baseAmt,
		CounterAmount:      counterAmt,
		BaseAccount:        r.BaseAccount,
		CounterAccount:     r.CounterAccount,
		BaseOfferID:        r.BaseOfferID,
		CounterOfferID:     r.CounterOfferID,
		LiquidityPoolID:    r.poolID(),
		LiquidityPoolSide:  r.poolSide(),
		LiquidityPoolFeeBP: r.LiquidityPoolFeeBP,
	}, nil
}

// ledgerFromPagingToken decodes the ledger sequence out of a trade's paging
// token, "<operation toid>-<index>".
//
// This is DECODING AN IDENTIFIER and not deriving a time. The TOID packs the
// ledger sequence into its high 32 bits by construction, so the answer is exact.
// Rule 4 of 00-overview.md forbids the other direction, turning a sequence into
// a timestamp by assuming a ledger duration, and nothing here does that: the
// time comes from ledger_close_time and only from there.
//
// A token that cannot be read yields 0 rather than an error. The sequence is a
// convenience for the reader of an output file; the trade's identity is the
// token itself and its clock is ledger_close_time, so neither is lost.
func ledgerFromPagingToken(tok string) uint32 {
	head, _, _ := strings.Cut(tok, "-")
	toid, err := strconv.ParseUint(head, 10, 64)
	if err != nil {
		return 0
	}
	return uint32(toid >> 32)
}

// splitPagingToken separates the operation from the fill index inside a trade's
// paging token.
//
// Several fills of ONE operation share the head and differ in the index, and
// domain.TradeImpliedDepthBounds groups on exactly that: fills of one operation
// are one taker walking one book, which is the only span in a trade stream whose
// cause is established rather than assumed.
//
// A token that cannot be read yields an empty operation, which groups with
// nothing. That is the conservative direction: it can only withhold a causal
// bound, never manufacture one.
func splitPagingToken(tok string) (opID string, fill int) {
	head, tail, found := strings.Cut(tok, "-")
	if !found {
		return "", 0
	}
	if _, err := strconv.ParseUint(head, 10, 64); err != nil {
		return "", 0
	}
	n, err := strconv.Atoi(tail)
	if err != nil {
		return "", 0
	}
	return head, n
}
