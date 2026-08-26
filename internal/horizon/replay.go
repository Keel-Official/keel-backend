// Rebuilding a past order book from the operations that posted it.
//
// THIS IS NOT A HISTORICAL ENDPOINT AND TRAP 3 STILL STANDS. Horizon serves no
// order book at a past ledger and never will. What it does serve, in full and
// back to genesis, is the stream of OPERATIONS and the RESULT of each one. State
// is not available; events are. This file replays the events to recover the state,
// which is what `offers-implied` has meant in the golden fixture since before any
// of this existed, and it is DEC-002 section 2.3 built.
//
// WHY IT WAS BUILT NOW. DEC-002 section 2.3 opens with "Only attempt this if 2.1
// and 2.2 prove insufficient." They did, and the measurement is in
// docs/evidences/2026-08-26-ustry-february-trades-implied.md: over the whole of
// February 2026 not one leg moved the USTRY/USDC price by as much as two percent,
// so the trade stream supports no causal bound at any rung Keel reports. What made
// USTRY dangerous was never in what traded. It was in what was POSTED, and only
// this path can see that.
//
// HOW IT WORKS, in four steps.
//
//  1. DISCOVER the accounts that could hold an offer on the pair. Every account
//     that appears in /trades for the pair, plus every account holding an offer on
//     the pair right now. Both are approximations and section "The gap" below
//     names what they miss.
//  2. READ each account's manage offer operations backwards from the target
//     ledger, with the transaction joined so the RESULT comes with them.
//  3. DECODE each result. Horizon reports a create as `"offer_id": "0"` and emits
//     no effects, so the identity of a new offer exists only in the result XDR.
//     See offerxdr.go, which has the measurement.
//  4. REPLAY operations and trades together in TOID order, then read the resting
//     offers off the final state.
//
// CONSUMPTION COMES FROM THE TRADE STREAM AND NOT FROM offersClaimed, even though
// the claims are decoded and carry exact stroop amounts. The reason is uniformity:
// a resting offer can be taken by a manage offer, by a path payment in either
// direction, or by a path payment that also crosses a pool, and only the first of
// those is a manage offer result this decoder reads. Every one of them produces a
// trade record on the pair. Using one mechanism for all consumption is what stops
// an offer being decremented twice or not at all.
//
// THE ORDERING RULE THAT MAKES THAT SOUND. Inside one operation, its trades are
// applied FIRST and its own result LAST. The result is the ledger's statement of
// what the submitting offer looked like when the operation finished, so writing it
// last is correct even though the trades of that same operation appear to touch it:
// whatever they did is overwritten by the fact.
//
// THE GAP, STATED RATHER THAN HIDDEN. DEC-002 section 2.3 names one and there are
// three:
//
//   - An account that posted an offer, never traded it, and still rests at the
//     target ledger is invisible to account discovery. Reading trades PAST the
//     target shrinks this, because an offer resting at the target that trades a day
//     later brings its owner in, and the lookahead is a parameter.
//   - An account whose operation history is longer than the page cap is walked
//     only as far back as the cap allows. Every truncation is counted and the
//     earliest ledger reached is reported per account.
//   - An operation whose transaction contains an earlier operation this decoder
//     cannot size is skipped rather than guessed. Counted too.
//
// None of the three is silent, and MissingOfferIDs is the measure that catches all
// of them at once: any offer a trade names after the target that this replay never
// saw is a hole, by construction.
//
// POOLS ARE NOT RECONSTRUCTED, and the resulting Snapshot says so by carrying none.
// Pool reserves at a past ledger are recoverable from
// /liquidity_pools/{id}/operations, which DEC-002 section 2.3 calls cleaner than
// the offer side because it has no discovery gap, and that is a separate piece of
// work. A depth figure computed from this Snapshot is therefore ORDER BOOK ONLY,
// and any caller that presents it as combined depth is wrong.
//
// THE THREE SENTENCES THIS ZONE ASKS FOR. The decision: accounts are discovered
// from the trade stream and from the live offer book, their operations are walked
// BACKWARDS from the target, and every approximation is counted into the result
// rather than logged. The alternative rejected: walking forward from a fixed start
// ledger over every account that ever touched the asset, which is the shape DEC-002
// section 2.3 describes. Why it was rejected: forward from a fixed start silently
// loses every offer created before that start and still resting, and the loss looks
// exactly like a thin book, which is this product's most interesting finding and
// therefore the worst thing to produce by accident; walking backwards puts the
// operations that decide the target state on the FIRST page and turns the same
// limitation into a depth this file can report.
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

// stroop is 1e-7, the scale of every Stellar amount.
var stroop = decimal.New(1, -7)

// syntheticOfferBit marks an offer id that names a TRANSIENT order rather than a
// resting one.
//
// A taker whose order crosses the book completely never rests, so it has no
// ledger offer id. Horizon still has to name both sides of the trade it produced,
// and it does so by OR-ing the operation's TOID with bit 62. Measured, not
// inferred, on the 22 February 2026 manipulation:
//
//	operation TOID       263454423513071617  0x03a7fa6700008001
//	counter_offer_id    4875140441940459521  0x43a7fa6700008001
//
// It matters twice and both are silent failures without it. A synthetic id could
// collide with nothing in the state, so consumption is unaffected, but
// missingOffers would report EVERY taker in the window as an offer it never saw:
// 9,478 of the 10,077 distinct offer ids in February 2026 are synthetic, so the
// completeness check would have been noise instead of a measurement.
const syntheticOfferBit = int64(1) << 62

// syntheticOffer reports whether an id names a transient order.
func syntheticOffer(id int64) bool { return id&syntheticOfferBit != 0 }

const (
	// operationsPageLimit is the largest page Horizon serves.
	operationsPageLimit = 200

	// defaultMaxPagesPerAccount bounds one account's backwards walk. Twenty pages
	// is four thousand operations, which covers every account in the USTRY set
	// and will not cover a busy market maker. A walk that hits it is counted, and
	// the earliest ledger it reached is reported, so the bound is visible in the
	// output rather than being a silent floor.
	defaultMaxPagesPerAccount = 20
)

// ReplayQuery bounds one reconstruction.
type ReplayQuery struct {
	// TargetLedger is the ledger whose book is wanted. The state is the state at
	// the END of that ledger, which is what a snapshot taken during the next one
	// would have seen.
	TargetLedger uint32

	// TradesFromLedger seeks the trade walk used for account discovery and for
	// consumption. Zero walks the pair's whole history, which is correct and
	// expensive. Being early costs requests; being late loses offers.
	TradesFromLedger uint32

	// TradeLookahead is how far PAST the target the trade walk continues, in
	// ledgers, purely to discover accounts. An offer resting at the target that
	// trades later brings its owner into the account set this way. Trades after
	// the target are NEVER applied as consumption.
	TradeLookahead uint32

	// MaxPagesPerAccount bounds each account's backwards operation walk. Zero
	// uses defaultMaxPagesPerAccount.
	MaxPagesPerAccount int

	// SinceLedger is a FLOOR on the backwards walk: an account's operations older
	// than it are not read. Zero means no floor, so each walk runs to the
	// account's first operation or to the page cap.
	//
	// IT IS THE HONEST KNOB AND THE DANGEROUS ONE. A floor makes the cost
	// predictable, because most accounts then stop on their first page instead of
	// paging back through years. It also makes every offer created before it
	// invisible, and an invisible offer reads as a thinner book rather than as an
	// error. Each walk reports the earliest ledger it actually reached and whether
	// the floor is what stopped it, so the depth of the reading is a number in the
	// output rather than a setting somebody has to remember.
	SinceLedger uint32

	// Progress, when set, is called once per account as the walk finishes it. A
	// reconstruction over a few hundred accounts is minutes of requests, and a
	// command that prints nothing until it is done cannot be told apart from one
	// that has hung.
	Progress func(AccountWalk)
}

// AccountWalk is what one account's operation walk cost and how deep it reached.
type AccountWalk struct {
	Account string
	Pages   int

	// EarliestLedger is the oldest ledger the walk reached. With Truncated it is
	// the floor below which this account's offers are invisible.
	EarliestLedger uint32
	Truncated      bool

	// Records is how many operation records were actually read, not pages times
	// the page size. The last page of a walk is usually partial, and a count that
	// assumes otherwise overstates the cost of every run.
	Records int

	OfferOperations int
	Unsizable       int

	// Err is set when this account's walk failed, and the walk is then partial or
	// empty. It does NOT abort the reconstruction: see the comment where it is
	// caught.
	Err string

	// StoppedAtFloor is true when ReplayQuery.SinceLedger ended this walk rather
	// than the account running out of operations or the page cap. It separates
	// "this account has no older operations" from "we chose not to look".
	StoppedAtFloor bool

	// EarliestOfferOp is the oldest ledger at which an offer operation on the pair
	// was actually APPLIED, or zero when none was. It is not EarliestLedger, which
	// counts every record inspected: an offer can only enter the state through an
	// applied operation, so this is the bound the trade window has to cover.
	EarliestOfferOp uint32
}

// ReplayResult is a reconstructed book and everything needed to distrust it.
type ReplayResult struct {
	Snapshot domain.Snapshot

	// Accounts is every account that was walked, sorted, so two runs over the
	// same ledger produce the same list. NFR-9 applies to this file too.
	Accounts []AccountWalk

	// FromTrades and FromLiveOffers are how the account set was reached. An
	// account found both ways is counted in both.
	FromTrades     int
	FromLiveOffers int

	TradesRead      int
	OperationsRead  int
	OfferOperations int
	Truncated       int
	Unsizable       int
	StoppedAtFloor  int
	Failed          int

	// EarliestOfferOp is the oldest ledger any applied offer operation came from,
	// across every account. TradeWindowFrom is where the trade walk began. When
	// the first is BELOW the second, consumption in the gap was never seen and the
	// book may carry offers that had already been eaten.
	EarliestOfferOp uint32
	TradeWindowFrom uint32

	// MissingOfferIDs are offers that a trade AFTER the target ledger named as
	// resting, and that this replay never saw created. Each one is a hole in the
	// reconstruction, and an empty list is the strongest self-check this method
	// has. It is sorted.
	MissingOfferIDs []int64

	Requests int
	ReadAt   time.Time
}

// Complete reports whether the reconstruction found no hole in itself.
//
// It is NOT a proof of correctness. It says that nothing this replay could detect
// went wrong, and the three gaps in the header are exactly the things it cannot
// detect. A caller printing this must print the counts beside it.
func (r ReplayResult) Complete() bool {
	return len(r.MissingOfferIDs) == 0 && r.Truncated == 0 && r.Unsizable == 0 &&
		r.Failed == 0 && !r.MayBeInflated()
}

// MayBeInflated reports whether an offer could be resting on this book that was
// eaten before the trade walk began.
//
// It is the one failure mode here that runs in the OPTIMISTIC direction, so it is
// separated from the others rather than folded into a single counter. The
// constructor refuses the configurations that cause it; this catches the residue,
// because a page of operations can straddle the floor and reach one record below
// it.
func (r ReplayResult) MayBeInflated() bool {
	return r.EarliestOfferOp != 0 && r.TradeWindowFrom != 0 && r.EarliestOfferOp < r.TradeWindowFrom
}

// ReconstructBook rebuilds the order book for one pair at one ledger.
//
// THE TWO WINDOWS MUST LINE UP AND THAT IS ENFORCED HERE. Offers come from the
// operation walk and consumption comes from the trade walk, so an offer created
// inside the operation window and eaten BEFORE the trade window starts is never
// decremented and rests on the reconstructed book for ever.
//
// The direction of that error is what makes it worth refusing rather than
// reporting. Every other gap in this file loses offers and makes a book look
// THINNER, which is conservative for a product whose job is to warn. A phantom
// offer makes it look DEEPER. On 26 August 2026 a run with no operation floor and
// a ten thousand ledger trade window applied 8,253 offer operations against 489
// trades and produced 334 asks starting at 1.0527 for the ledger the incident
// proves had one ask at 106.7372828. Every completeness counter read clean.
func (c *Client) ReconstructBook(ctx context.Context, base, quote domain.Asset, q ReplayQuery) (ReplayResult, error) {
	out := ReplayResult{ReadAt: c.cfg.Now().UTC()}
	if q.TargetLedger == 0 {
		return out, fmt.Errorf("horizon: replay needs a target ledger")
	}
	if q.SinceLedger == 0 && q.TradesFromLedger != 0 {
		return out, fmt.Errorf(
			"horizon: replay with no operation floor needs the whole trade history too, "+
				"or every offer eaten before ledger %d rests on the book for ever; "+
				"set TradesFromLedger to 0 or give SinceLedger a floor", q.TradesFromLedger)
	}
	if q.SinceLedger != 0 && q.TradesFromLedger > q.SinceLedger {
		return out, fmt.Errorf(
			"horizon: the trade walk starts at ledger %d and the operation walk reaches back to %d, "+
				"so consumption between them is invisible and the book would be inflated; "+
				"TradesFromLedger must be at or below SinceLedger", q.TradesFromLedger, q.SinceLedger)
	}
	before := c.Requests()

	// 1. Trades, up to the target plus the lookahead. The lookahead half is used
	//    for discovery and for the missing-offer check, never as consumption.
	stopAfter := q.TargetLedger + q.TradeLookahead
	trades, err := c.Trades(ctx, base, quote, TradeQuery{
		FromLedger: q.TradesFromLedger,
		StopAfter:  func(t domain.Trade) bool { return t.LedgerSeq > stopAfter },
	})
	if err != nil {
		return out, err
	}
	out.TradesRead = len(trades.Trades)

	// ONLY ACCOUNTS THAT ARE KNOWN TO HAVE HELD AN OFFER ON THIS PAIR, which is
	// narrower than "every account that traded" and is the difference between a
	// walk that finishes and one that does not.
	//
	// A trade names an offer id on each side, and a side whose id is zero was a
	// taker whose order was consumed entirely: it never rested, so it has no
	// offer for this replay to reconstruct and walking its operations reads
	// thousands of payments to find nothing. Measured on the control ledger
	// before this filter existed: a path payment bot with no offers at all cost
	// twenty pages, the same as the market maker that holds the book.
	//
	// What this gives up is nothing the wider set had. An account that rested an
	// offer which never traded is missed by BOTH versions, and it is the gap the
	// live offer book below and MissingOfferIDs at the end are there for.
	accounts := map[string]bool{}
	for _, t := range trades.Trades {
		if t.BaseAccount != "" && t.BaseOfferID != "" && t.BaseOfferID != "0" {
			accounts[t.BaseAccount] = true
		}
		if t.CounterAccount != "" && t.CounterOfferID != "" && t.CounterOfferID != "0" {
			accounts[t.CounterAccount] = true
		}
	}
	out.FromTrades = len(accounts)

	// 2. Sellers of offers resting on the pair RIGHT NOW. One request, and it
	//    reaches accounts that have never traded. It is worth least when the
	//    target is old and worth most when it is recent, which is the Layer 3
	//    case.
	live, err := c.liveOfferSellers(ctx, base, quote)
	if err != nil {
		return out, err
	}
	for _, a := range live {
		if !accounts[a] {
			out.FromLiveOffers++
		}
		accounts[a] = true
	}

	names := make([]string, 0, len(accounts))
	for a := range accounts {
		names = append(names, a)
	}
	sort.Strings(names)

	// 3. Every account's manage offer operations, backwards from the target.
	maxPages := q.MaxPagesPerAccount
	if maxPages <= 0 {
		maxPages = defaultMaxPagesPerAccount
	}
	var ops []offerOperation
	for _, a := range names {
		got, walk, err := c.offerOperationsFor(ctx, a, refOf(base), refOf(quote), q.TargetLedger, q.SinceLedger, maxPages)
		if err != nil {
			// ONE ACCOUNT FAILING DOES NOT FAIL THE RECONSTRUCTION, and the
			// reasoning is the same one runs.go gives for a scan: a walk over a
			// few hundred accounts against public Horizon will meet a 503, and
			// throwing away the other thirty-seven walks because the
			// thirty-eighth was throttled is not honest work. Measured on
			// 26 August 2026: a sixty page walk over this pair's accounts drew
			// a 503 that survived five retries.
			//
			// It is safe to continue in the direction that matters. A missing
			// account loses offers, which makes the book THINNER, which is the
			// conservative side. The count is reported and the partial results
			// this account did return are kept, because they are real.
			if ctxErr := ctx.Err(); ctxErr != nil {
				return out, ctxErr
			}
			walk.Err = err.Error()
			out.Failed++
		}
		ops = append(ops, got...)
		out.Accounts = append(out.Accounts, walk)
		out.OperationsRead += walk.Records
		out.OfferOperations += walk.OfferOperations
		out.Unsizable += walk.Unsizable
		if walk.Truncated {
			out.Truncated++
		}
		if walk.StoppedAtFloor {
			out.StoppedAtFloor++
		}
		if walk.EarliestOfferOp != 0 && (out.EarliestOfferOp == 0 || walk.EarliestOfferOp < out.EarliestOfferOp) {
			out.EarliestOfferOp = walk.EarliestOfferOp
		}
		if q.Progress != nil {
			q.Progress(walk)
		}
	}

	// 4. Replay, then read the book off the final state.
	state := replayOffers(ops, trades.Trades, q.TargetLedger)
	out.Snapshot = domain.Snapshot{
		Base:      base,
		Quote:     quote,
		LedgerSeq: q.TargetLedger,
		Book:      bookFromOffers(state, base, quote),
		// Pools stay nil. The header says why, and a caller must not read the
		// absence as "there was no pool".
		Source: domain.DataSourceOffersImplied,
	}
	out.TradeWindowFrom = q.TradesFromLedger
	out.MissingOfferIDs = missingOffers(ops, trades.Trades, q.TargetLedger)
	out.Requests = c.Requests() - before
	return out, nil
}

// ---------------------------------------------------------------- reading

// offerOperation is one manage offer operation with the state its result left.
type offerOperation struct {
	TOID    int64
	Ledger  uint32
	Account string
	Kind    string

	// SubmittedOfferID is the operation's OWN offer_id field: zero for a create,
	// and the offer being changed otherwise. It is not redundant with
	// Result.OfferID, and the difference is the whole of the cancel case: an
	// operation with offer_id 4242 and amount 0 comes back with effect DELETED
	// and NO offer in the result, so 4242 exists only here and the replay would
	// leave a cancelled offer resting for ever without it.
	SubmittedOfferID int64

	Result resultingOffer
}

func (c *Client) offerOperationsFor(ctx context.Context, account string, baseRef, quoteRef assetRef, target, floor uint32, maxPages int) ([]offerOperation, AccountWalk, error) {
	walk := AccountWalk{Account: account, EarliestLedger: target}

	v := url.Values{}
	v.Set("join", "transactions")
	v.Set("order", "desc")
	v.Set("limit", strconv.Itoa(operationsPageLimit))
	// The TOID one past the last operation the target ledger could hold, so the
	// descending walk starts inside the target and never above it.
	v.Set("cursor", strconv.FormatUint((uint64(target)+1)<<32, 10))

	path, query := "/accounts/"+account+"/operations", v
	var out []offerOperation

	for {
		if err := ctx.Err(); err != nil {
			return out, walk, err
		}
		if walk.Pages >= maxPages {
			walk.Truncated = true
			break
		}

		body, _, err := c.get(ctx, path, query, false)
		if err != nil {
			return out, walk, fmt.Errorf("horizon: operations for %s page %d: %w", account, walk.Pages+1, err)
		}
		walk.Pages++

		var page operationsPage
		if err := json.Unmarshal(body, &page); err != nil {
			return out, walk, fmt.Errorf("horizon: operations for %s page %d: decode: %w", account, walk.Pages, err)
		}
		if len(page.Embedded.Records) == 0 {
			break
		}

		hitFloor := false
		for _, r := range page.Embedded.Records {
			walk.Records++
			toid, err := strconv.ParseInt(r.PagingToken, 10, 64)
			if err != nil {
				continue
			}
			ledger := TOIDLedger(toid)
			// EarliestLedger is the oldest ledger INSPECTED, and it is recorded
			// before the floor test rather than after it. Recording it after
			// leaves the field at the target whenever the very first record is
			// already below the floor, which reads as "walked right up to the
			// target and found nothing" when the truth is "looked once, saw an
			// operation a week older, and stopped". The account that owns the
			// USTRY bids of 15 February 2026 is exactly that case.
			if ledger < walk.EarliestLedger {
				walk.EarliestLedger = ledger
			}
			// The walk is descending, so the first operation below the floor ends
			// it. Records after this one on the same page are older still.
			if floor > 0 && ledger < floor {
				hitFloor = true
				break
			}
			if !r.TransactionSuccessful || !r.isOfferOperation() || !r.touchesPair(baseRef, quoteRef) {
				continue
			}
			res, err := ParseManageOfferResult(r.Transaction.ResultXDR, TOIDOperationIndex(toid))
			if err != nil {
				// An operation whose result cannot be read is DROPPED and
				// counted. Guessing at it would put a wrong offer on the book,
				// which is worse than a hole a number can report.
				walk.Unsizable++
				continue
			}
			submitted, _ := strconv.ParseInt(r.OfferID, 10, 64)
			out = append(out, offerOperation{
				TOID:             toid,
				Ledger:           TOIDLedger(toid),
				Account:          r.SourceAccount,
				Kind:             r.Type,
				SubmittedOfferID: submitted,
				Result:           res,
			})
			if walk.EarliestOfferOp == 0 || ledger < walk.EarliestOfferOp {
				walk.EarliestOfferOp = ledger
			}
			walk.OfferOperations++
		}

		if hitFloor {
			walk.StoppedAtFloor = true
			break
		}

		next := strings.TrimSpace(page.Links.Next.Href)
		if next == "" {
			break
		}
		u, err := url.Parse(next)
		if err != nil {
			return out, walk, fmt.Errorf("horizon: operations for %s: next link %q: %w", account, next, err)
		}
		path, query = u.Path, u.Query()
	}
	return out, walk, nil
}

// liveOfferSellers reads /offers for the pair and returns the sellers, both ways
// round, because a bid is an offer selling the quote asset.
func (c *Client) liveOfferSellers(ctx context.Context, base, quote domain.Asset) ([]string, error) {
	seen := map[string]bool{}
	for _, side := range [2][2]domain.Asset{{base, quote}, {quote, base}} {
		v := url.Values{}
		addAsset(v, "selling", side[0])
		addAsset(v, "buying", side[1])
		v.Set("limit", strconv.Itoa(operationsPageLimit))

		body, _, err := c.get(ctx, "/offers", v, false)
		if err != nil {
			return nil, fmt.Errorf("horizon: live offers %s/%s: %w", side[0], side[1], err)
		}
		var page offersPage
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("horizon: live offers %s/%s: decode: %w", side[0], side[1], err)
		}
		for _, r := range page.Embedded.Records {
			if r.Seller != "" {
				seen[r.Seller] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for a := range seen {
		out = append(out, a)
	}
	sort.Strings(out)
	return out, nil
}

// ---------------------------------------------------------------- the replay

// restingOffer is one offer in the reconstructed state.
type restingOffer struct {
	ID      int64
	Selling assetRef
	Buying  assetRef
	Amount  decimal.Decimal // in SELLING units
	PriceN  int64
	PriceD  int64
}

// replayOffers applies operations and trades in TOID order and returns what is
// still resting at the end of the target ledger.
//
// Events at the same TOID are ordered trades first, operation last. The header
// explains why that ordering is what makes trade-driven consumption sound.
func replayOffers(ops []offerOperation, trades []domain.Trade, target uint32) map[int64]*restingOffer {
	type event struct {
		toid  int64
		order int // 0 = trade, 1 = the operation's own result
		trade *domain.Trade
		op    *offerOperation
	}
	var events []event

	for i := range ops {
		if ops[i].Ledger > target {
			continue
		}
		events = append(events, event{toid: ops[i].TOID, order: 1, op: &ops[i]})
	}
	for i := range trades {
		if trades[i].LedgerSeq > target {
			continue // a trade after the target consumed nothing that matters here
		}
		toid, err := strconv.ParseInt(trades[i].OperationID, 10, 64)
		if err != nil {
			continue
		}
		events = append(events, event{toid: toid, order: 0, trade: &trades[i]})
	}

	sort.SliceStable(events, func(i, j int) bool {
		if events[i].toid != events[j].toid {
			return events[i].toid < events[j].toid
		}
		if events[i].order != events[j].order {
			return events[i].order < events[j].order
		}
		// Fills of one operation in engine order, so a partial consumption chain
		// is applied the way the ledger applied it. NFR-9 wants this stable.
		if events[i].trade != nil && events[j].trade != nil {
			return events[i].trade.FillIndex < events[j].trade.FillIndex
		}
		return false
	})

	state := map[int64]*restingOffer{}
	for _, e := range events {
		switch {
		case e.trade != nil:
			consume(state, *e.trade)
		case e.op != nil:
			apply(state, *e.op)
		}
	}
	return state
}

// apply writes what an operation's result says about the offer it submitted.
//
// A DELETED effect carries no offer, so the operation's own offer_id is the only
// place the identity survives. Two different things arrive as DELETED and they
// are told apart by exactly that field: an offer_id of zero is a create that
// crossed the whole book and rested nothing, and there is nothing to remove; a
// non-zero offer_id is a cancel or a full fill of an existing offer, and leaving
// it resting is a phantom level that never expires.
func apply(state map[int64]*restingOffer, op offerOperation) {
	r := op.Result
	if r.Effect == offerDeleted {
		if op.SubmittedOfferID != 0 {
			delete(state, op.SubmittedOfferID)
		}
		return
	}
	if r.OfferID == 0 {
		return
	}
	// An UPDATE can move an offer's price, so the whole entry is replaced rather
	// than patched. The ledger's entry is the state; nothing is carried over.
	state[r.OfferID] = &restingOffer{
		ID:      r.OfferID,
		Selling: r.Selling,
		Buying:  r.Buying,
		Amount:  decimal.NewFromInt(r.Amount).Mul(stroop),
		PriceN:  int64(r.PriceN),
		PriceD:  int64(r.PriceD),
	}
}

// consume reduces whichever resting offer this trade took, by the amount that
// offer sold, and removes it when nothing is left.
//
// A trade names an offer on each side and only one of them is resting in this
// state at this moment; the other is the arriving order, whose final shape its
// own operation result states. Looking both up and decrementing whichever is
// present is what makes that unnecessary to distinguish.
func consume(state map[int64]*restingOffer, t domain.Trade) {
	for _, side := range [2]struct {
		id     string
		amount decimal.Decimal
		asset  string
	}{
		{t.BaseOfferID, t.BaseAmount, "base"},
		{t.CounterOfferID, t.CounterAmount, "counter"},
	} {
		id, err := strconv.ParseInt(side.id, 10, 64)
		if err != nil || id == 0 || syntheticOffer(id) {
			continue
		}
		o, ok := state[id]
		if !ok {
			continue
		}
		o.Amount = o.Amount.Sub(side.amount)
		if o.Amount.LessThanOrEqual(decimal.Zero) {
			delete(state, id)
		}
	}
}

// missingOffers returns offers that a trade after the target named as resting and
// that the replay never saw at all.
//
// It is the self-check the header calls the strongest one available: every hole
// the three gaps can produce shows up here, because an offer nobody saw created
// cannot be on the reconstructed book.
func missingOffers(ops []offerOperation, trades []domain.Trade, target uint32) []int64 {
	seen := map[int64]bool{}
	for _, o := range ops {
		if o.Result.OfferID != 0 {
			seen[o.Result.OfferID] = true
		}
		if o.SubmittedOfferID != 0 {
			seen[o.SubmittedOfferID] = true
		}
		for _, c := range o.Result.Claimed {
			if c.OfferID != 0 {
				seen[c.OfferID] = true
			}
		}
	}
	// An offer a trade names BEFORE the target was also visible to the replay, so
	// both halves of the window are checked.
	missing := map[int64]bool{}
	for _, t := range trades {
		for _, raw := range [2]string{t.BaseOfferID, t.CounterOfferID} {
			id, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || id == 0 || syntheticOffer(id) || seen[id] {
				continue
			}
			missing[id] = true
		}
	}
	out := make([]int64, 0, len(missing))
	for id := range missing {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// bookFromOffers turns the resting offers into the pair's book.
//
// THE TWO SIDES ARE NOT SYMMETRIC AND THAT IS THE WHOLE OF THIS FUNCTION. An ask
// is an offer selling the base, so its price is already quote per base and its
// amount is already in base units. A bid is an offer selling the QUOTE, so its
// price reads base per quote and has to be inverted, and its amount is in quote
// units and has to be converted. domain.Level.Amount is defined in base units,
// which is trap 5 of this zone read from the other direction.
func bookFromOffers(state map[int64]*restingOffer, base, quote domain.Asset) domain.OrderBook {
	var book domain.OrderBook

	ids := make([]int64, 0, len(state))
	for id := range state {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	baseRef := refOf(base)
	quoteRef := refOf(quote)

	// OFFERS AT ONE PRICE ARE ONE LEVEL, because that is what /order_book returns
	// and a reconstruction that emits one level per offer cannot be compared
	// against it level by level, which is comparison depth 1 and 2 of
	// docs/methodology/10-validation.md section 3. The key is the exact rational
	// and never the decimal: two offers at 1/3 and 2/6 are the same price and a
	// decimal key would only agree by rounding luck.
	asks := map[string]*domain.Level{}
	bids := map[string]*domain.Level{}
	add := (func(into map[string]*domain.Level, p domain.Price, amount decimal.Decimal) {
		k := p.String()
		if l, ok := into[k]; ok {
			l.Amount = l.Amount.Add(amount)
			return
		}
		into[k] = &domain.Level{Price: p, Amount: amount}
	})

	for _, id := range ids {
		o := state[id]
		if o.Amount.LessThanOrEqual(decimal.Zero) || o.PriceN <= 0 || o.PriceD <= 0 {
			continue
		}
		switch {
		case sameAsset(o.Selling, baseRef) && sameAsset(o.Buying, quoteRef):
			add(asks, domain.Price{N: o.PriceN, D: o.PriceD}, o.Amount)
		case sameAsset(o.Selling, quoteRef) && sameAsset(o.Buying, baseRef):
			// price is base per quote, so the quote-per-base price is the flip,
			// and the base amount is the quote amount times base per quote.
			add(bids, domain.Price{N: o.PriceN, D: o.PriceD}.Invert(),
				o.Amount.Mul(decimal.NewFromInt(o.PriceN)).DivRound(decimal.NewFromInt(o.PriceD), domain.Precision))
		}
	}
	for _, l := range asks {
		book.Asks = append(book.Asks, *l)
	}
	for _, l := range bids {
		book.Bids = append(book.Bids, *l)
	}

	// Best first on both sides, the same order GetSnapshot produces, so the two
	// sources are comparable level by level.
	sort.SliceStable(book.Bids, func(i, j int) bool { return book.Bids[i].Price.Cmp(book.Bids[j].Price) > 0 })
	sort.SliceStable(book.Asks, func(i, j int) bool { return book.Asks[i].Price.Cmp(book.Asks[j].Price) < 0 })
	return book
}

func refOf(a domain.Asset) assetRef {
	if a.IsNative() {
		return assetRef{AssetType: "native"}
	}
	return assetRef{AssetType: string(a.Type), AssetCode: a.Code, AssetIssuer: a.Issuer}
}

// ---------------------------------------------------------------- wire shapes

type operationsPage struct {
	Links struct {
		Next struct {
			Href string `json:"href"`
		} `json:"next"`
	} `json:"_links"`
	Embedded struct {
		Records []operationRecord `json:"records"`
	} `json:"_embedded"`
}

// operationRecord is the subset of an operation this file reads, plus the joined
// transaction. The join is what makes the result XDR arrive without a second
// request per operation.
type operationRecord struct {
	PagingToken           string `json:"paging_token"`
	Type                  string `json:"type"`
	OfferID               string `json:"offer_id"`
	SellingAssetType      string `json:"selling_asset_type"`
	SellingAssetCode      string `json:"selling_asset_code"`
	SellingAssetIssuer    string `json:"selling_asset_issuer"`
	BuyingAssetType       string `json:"buying_asset_type"`
	BuyingAssetCode       string `json:"buying_asset_code"`
	BuyingAssetIssuer     string `json:"buying_asset_issuer"`
	SourceAccount         string `json:"source_account"`
	TransactionSuccessful bool   `json:"transaction_successful"`
	Transaction           struct {
		ResultXDR string `json:"result_xdr"`
	} `json:"transaction"`
}

// touchesPair reports whether this operation is an offer on the pair, in either
// direction, comparing the FULL identity including the issuer.
//
// It runs before the result XDR is decoded, which matters twice. It saves the
// decode on every offer an account posts on some other pair, and it is the only
// place in this path where the issuer is checked: the decoder deliberately does
// not read issuers, so sameAsset compares type and code alone, and this is the
// check that makes that safe.
func (r operationRecord) touchesPair(base, quote assetRef) bool {
	selling := assetRef{AssetType: r.SellingAssetType, AssetCode: r.SellingAssetCode, AssetIssuer: r.SellingAssetIssuer}
	buying := assetRef{AssetType: r.BuyingAssetType, AssetCode: r.BuyingAssetCode, AssetIssuer: r.BuyingAssetIssuer}
	return (selling == base && buying == quote) || (selling == quote && buying == base)
}

func (r operationRecord) isOfferOperation() bool {
	switch r.Type {
	case "manage_sell_offer", "manage_buy_offer", "create_passive_sell_offer":
		return true
	}
	return false
}

type offersPage struct {
	Embedded struct {
		Records []struct {
			Seller string `json:"seller"`
		} `json:"records"`
	} `json:"_embedded"`
}
