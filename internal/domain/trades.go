// Trades, and the upper bound on depth that can be read off them.
//
// ZONE: YELLOW, the same as the rest of internal/domain. This file is pure: it
// takes trades that somebody else read and returns arithmetic over them.
//
// WHY THIS FILE EXISTS. Horizon serves no historical order book, so depth at a
// past ledger cannot be measured directly, and DEC-002 defers the source that
// could answer it. docs/methodology/01-data-sources.md section 6 defines the
// substitute and it is one inequality:
//
//	depth(δ) ≤ S    if a trade of size S moved the marginal price by δ
//
// The reason is that a trade of size S could not have crossed a price range that
// held more than S of liquidity. The result is an UPPER BOUND and never a
// measurement, its bias can only make an asset look safer than it was, and that
// is the direction a warning product can defend.
//
// THE WORD THAT DOES THE WORK IN THAT SENTENCE IS "MOVED", AND IT IS A CLAIM
// ABOUT CAUSE. Two trades an hour apart at different prices establish that the
// price changed, not that the second one changed it: offers can be cancelled and
// posted in between, and nothing in a trade stream records that. So this file
// produces TWO kinds of bound and never merges them.
//
// THE UNIT IS A LEG, NOT AN OPERATION, AND THAT DISTINCTION WAS PAID FOR. A leg
// is a maximal run of fills inside ONE operation that all move the base asset in
// the SAME direction. Most operations are one leg. A path payment is not:
// operation 263504030385864713 on 22 February 2026 is
// path_payment_strict_send USDC -> USTRY -> USDC, which BUYS USTRY on the order
// book at 1.1233 and SELLS it to the pool at 0.9953 in the same operation. Read
// as one walk that is an 11 percent price span for 1.89 USDC, which would have
// been the tightest causal bound in the whole month and is not a bound at all:
// it is the spread between two venues at one instant, and no liquidity was
// crossed between those two prices. Grouping by direction is what stops that
// number being reported.
//
//   - BoundWithinLeg is causal and needs no assumption. One taker, one
//     direction, and the engine fills from the best price outward, so the span
//     between the leg's first and last fill was crossed BY that leg and the
//     leg's own notional is the liquidity it consumed getting there.
//   - BoundBetweenLegs spans the gap from one leg's last fill to the next leg's
//     first fill. It holds only if the book did not change in between, which
//     trades cannot prove. Elapsed carries how long that gap was, so a reader
//     can judge it, and a caller that quotes one of these without saying which
//     kind it is has overstated what the chain shows. The gap can also be zero,
//     which is the path payment above: two legs, no time between them, and still
//     no causal claim.
//
// WHAT THIS FILE DELIBERATELY DOES NOT REPRODUCE. The strongest claim about the
// 22 February 2026 incident is not a bound at all. That operation produced
// exactly ONE trade record, so no ask existed anywhere below 106.7372828 for it
// to have hit first, which makes order book depth over that range ZERO rather
// than merely small. docs/methodology/10-validation.md section 7 makes that
// argument and it turns on the absence of records rather than on their contents.
// A bound is the weaker, general-purpose statement.
//
// ON THE FIXTURE-FIRST RULE, because it applies here and the honest position is
// to say how. compute.go's header states that a function there may only be
// written after its expected values exist in testdata/fixtures. Nothing in the
// fixture speaks to this file. What it is checked against instead is the one
// worked example that already existed and was derived by hand elsewhere: the
// 22 February 2026 trade in docs/methodology/10-validation.md section 7, whose
// figures scripts/audit-verification.sh recomputes outside Go. That is a weaker
// guarantee than a fixture and it is a real one, because those numbers predate
// this file by days and were not produced to satisfy it. Whether this file also
// needs a fixture entry is Al's call, not this file's.
//
// THE THREE SENTENCES. The decision: bounds are grouped by LEG, and the causal
// kind and the assuming kind are two values of one field rather than two
// functions. The alternative rejected: emitting only the causal kind, which needs
// no caveat and cannot be misquoted. Why it was rejected: the incident's own
// hundredfold move sits in the gap between two legs and would vanish entirely,
// and a tool that silently drops the largest move in its window because it cannot
// certify the cause is less honest than one that reports it under a label.
package domain

import (
	"sort"
	"time"

	"github.com/shopspring/decimal"
)

// Trade is one Horizon trade record for a pair, carried whole.
//
// Amounts keep the two sides separate rather than storing one and a price.
// Horizon sends base_amount, counter_amount AND the exact rational price, and
// the three are redundant only when nothing has gone wrong; keeping all of them
// is what lets a reader check the arithmetic against the record.
type Trade struct {
	// ID is Horizon's paging_token, "<operation toid>-<index>". It is the only
	// stable name a single trade has.
	ID string

	// OperationID is the toid half of ID, naming the operation this trade was a
	// fill of. Several fills of one operation share it. It is half of what makes
	// a causal bound possible; BaseIsSeller is the other half.
	OperationID string

	// BaseIsSeller is Horizon's own field, true when the base asset was being
	// sold. It is carried because DIRECTION is what separates a book walk from a
	// path payment that buys the base on one venue and sells it on another inside
	// the same operation. Fills of one operation that disagree on it are two legs
	// and never one, which is the header's second block.
	BaseIsSeller bool

	// FillIndex is the index half of ID, so the fills of one operation can be put
	// back into the order the engine produced them. Ordering on the ID string
	// instead sorts fill 10 ahead of fill 2.
	FillIndex int

	// LedgerSeq is the sequence the trade closed in, taken from the high 32 bits
	// of the operation toid inside ID rather than derived from the time. Zero
	// when the caller could not establish it.
	LedgerSeq uint32

	// ClosedAt is the ledger close time as Horizon reported it. It is never
	// computed from LedgerSeq: 00-overview.md section 2 rule 4 forbids that, and
	// assuming five seconds a ledger drifts about three weeks over six months.
	ClosedAt time.Time

	// Type is Horizon's trade_type verbatim, "orderbook" or "liquidity_pool".
	// It is not interpreted here. It matters to the reader because an orderbook
	// match reaches offers only and never touches pool liquidity, which is the
	// finding in DEC-006 section 4.
	Type string

	// Price is quote per base, exact, from the rational Horizon sent. Never from
	// a rounded string.
	Price Price

	// BaseAmount is in base units and CounterAmount is the notional in the quote
	// asset. CounterAmount is the S in the inequality at the top of this file.
	BaseAmount    decimal.Decimal
	CounterAmount decimal.Decimal

	// The two accounts and the two offers, kept because the genuine-trade rule
	// in docs/methodology/07-supporting-metrics.md will need them and because
	// specimen B of that worksheet is a trade matched against an offer the taker
	// controlled. Nothing here classifies a trade; that document is unwritten.
	BaseAccount     string
	CounterAccount  string
	BaseOfferID     string
	CounterOfferID  string
	LiquidityPoolID string
}

// BoundKind separates a bound whose cause is established from one whose cause is
// assumed. The distinction is the subject of the second block in this file's
// header and it must survive into any output a reader sees.
type BoundKind string

const (
	// BoundWithinLeg: the price span was crossed by the operation that is
	// paying for it. No assumption.
	BoundWithinLeg BoundKind = "within-leg"

	// BoundBetweenLegs: the price span sits between two operations. It
	// holds only if the book was unchanged across the gap, which trades cannot
	// establish. Read Elapsed before quoting one.
	BoundBetweenLegs BoundKind = "between-legs"
)

// TradeImpliedBound is one upper bound on depth.
//
// It carries both endpoint trades rather than only their identifiers so that a
// caller writing a row of output never has to go looking for the record again,
// and so that Delta and Bound can be checked against the fields they came from.
type TradeImpliedBound struct {
	Kind BoundKind

	From Trade
	To   Trade

	// Delta is the relative move of the marginal price, |P_to / P_from - 1|, as
	// a fraction. It follows the δ convention: 0.02 means 2 percent.
	Delta decimal.Decimal

	// Bound is the upper bound on depth at Delta, in the quote asset. It is the
	// notional of the leg that crossed the span: the leg's own total for a
	// within-leg bound, and the whole of the ARRIVING leg for a between-legs one.
	Bound decimal.Decimal

	// Elapsed is the time from From to To. It is zero inside a leg, zero between
	// two legs of one operation, and unbounded between two operations, which is
	// exactly the quantity a reader needs in order to decide how much a
	// between-legs bound is worth.
	Elapsed time.Duration
}

// BoundsOfKind filters, so a caller can report the causal bounds on their own
// without writing the comparison at every call site.
func BoundsOfKind(bounds []TradeImpliedBound, kind BoundKind) []TradeImpliedBound {
	out := make([]TradeImpliedBound, 0, len(bounds))
	for _, b := range bounds {
		if b.Kind == kind {
			out = append(out, b)
		}
	}
	return out
}

// TradeImpliedDepthBounds returns every bound the trade stream supports, of both
// kinds, in the order the legs occurred.
//
// The input is sorted before anything else, by close time then operation then
// fill index, so the result does not depend on the order the caller happened to
// read them in. That is NFR-9: the same trades in a different order must produce
// the same output.
//
// A span with a zero δ produces no bound, because "depth(0) ≤ S" says nothing. A
// trade with an unusable price is dropped for the same reason, and dropping is
// safe here in a way it is not elsewhere in this package: a missing bound
// weakens the claim and can never strengthen it.
func TradeImpliedDepthBounds(trades []Trade) []TradeImpliedBound {
	legs := groupIntoLegs(trades)

	out := make([]TradeImpliedBound, 0, len(legs)*2)
	for i, leg := range legs {
		first, last := leg.fills[0], leg.fills[len(leg.fills)-1]

		// The gap from the previous leg. Emitted FIRST, so the rows for one leg
		// read in the order the market saw them: the price arrived at this leg's
		// first fill, then the leg walked.
		if i > 0 {
			prev := legs[i-1].fills[len(legs[i-1].fills)-1]
			if d, ok := relativeMove(prev.Price, first.Price); ok {
				out = append(out, TradeImpliedBound{
					Kind:    BoundBetweenLegs,
					From:    prev,
					To:      first,
					Delta:   d,
					Bound:   leg.notional,
					Elapsed: first.ClosedAt.Sub(prev.ClosedAt),
				})
			}
		}

		// Inside this leg. One taker, one direction, filled best price first, so
		// this span needs no assumption about what happened in between: by
		// construction nothing did.
		if d, ok := relativeMove(first.Price, last.Price); ok {
			out = append(out, TradeImpliedBound{
				Kind:    BoundWithinLeg,
				From:    first,
				To:      last,
				Delta:   d,
				Bound:   leg.notional,
				Elapsed: last.ClosedAt.Sub(first.ClosedAt),
			})
		}
	}
	return out
}

// leg is a maximal run of same-direction fills inside one operation, in engine
// order, with their total.
type leg struct {
	op       string
	selling  bool
	fills    []Trade
	notional decimal.Decimal
}

// groupIntoLegs sorts and groups. Fills of one operation are contiguous after the
// sort because they share a close time and an operation id, and a change of
// direction inside that run opens a new leg.
func groupIntoLegs(trades []Trade) []leg {
	usable := make([]Trade, 0, len(trades))
	for _, t := range trades {
		if t.Price.Valid() {
			usable = append(usable, t)
		}
	}
	sort.SliceStable(usable, func(i, j int) bool {
		a, b := usable[i], usable[j]
		if !a.ClosedAt.Equal(b.ClosedAt) {
			return a.ClosedAt.Before(b.ClosedAt)
		}
		if a.OperationID != b.OperationID {
			return a.OperationID < b.OperationID
		}
		return a.FillIndex < b.FillIndex
	})

	var out []leg
	for _, t := range usable {
		// An empty OperationID groups with nothing, so each such trade is its own
		// leg. That is the conservative reading, and it runs the same way as the
		// direction test below: it can only withhold a causal bound, never invent
		// one.
		joins := t.OperationID != "" &&
			len(out) > 0 &&
			out[len(out)-1].op == t.OperationID &&
			out[len(out)-1].selling == t.BaseIsSeller
		if joins {
			n := len(out) - 1
			out[n].fills = append(out[n].fills, t)
			out[n].notional = out[n].notional.Add(t.CounterAmount)
			continue
		}
		out = append(out, leg{
			op:       t.OperationID,
			selling:  t.BaseIsSeller,
			fills:    []Trade{t},
			notional: t.CounterAmount,
		})
	}
	return out
}

// relativeMove is |after / before - 1|, and false when there is nothing to say.
func relativeMove(before, after Price) (decimal.Decimal, bool) {
	pb := before.Decimal()
	if pb.IsZero() {
		return decimal.Decimal{}, false
	}
	d := after.Decimal().DivRound(pb, Precision).Sub(one).Abs()
	if d.IsZero() {
		return decimal.Decimal{}, false
	}
	return d, true
}

// TightestBoundAtLeast returns the smallest bound observed at a move of at least
// minDelta, and whether any was observed at all.
//
// THE MONOTONICITY STEP IS THE POINT OF THIS FUNCTION AND IT IS WORTH STATING.
// A bound observed at δ = 0.99 is a bound on depth(0.99). Depth does not
// decrease as δ rises, which is invariant 1 of
// testdata/fixtures/ustry_pre_exploit.md, so depth(0.05) ≤ depth(0.99) ≤ S and
// the same S bounds every rung BELOW the one it was observed at. That is how a
// trade that moved the price a hundredfold says something about depth at 5
// percent. It says nothing about depth at 200 percent, and this function will
// not pretend otherwise: a bound is only offered to rungs at or below its own δ.
//
// The false direction, which would be a bug rather than a weaker claim, is
// reading a bound observed at δ = 0.01 as a bound on depth(0.10).
func TightestBoundAtLeast(bounds []TradeImpliedBound, minDelta decimal.Decimal) (decimal.Decimal, bool) {
	var best decimal.Decimal
	found := false
	for _, b := range bounds {
		if b.Delta.LessThan(minDelta) {
			continue
		}
		if !found || b.Bound.LessThan(best) {
			best, found = b.Bound, true
		}
	}
	return best, found
}
