// One walk, many books: reconstructing a pair's order book at a LIST of past
// ledgers instead of at one.
//
// WHY IT EXISTS. Deliverable 2 asks when the unsafe threshold was crossed
// relative to the exploit date, and that question needs the book on each day of
// February 2026 rather than on one. Calling ReconstructBook twenty-eight times
// answers it and pays for the same walk twenty-eight times: the fetch is priced
// by how busy the ACCOUNTS are, and the accounts holding this pair's book are the
// same accounts all month.
//
// WHAT MAKES ONE WALK ENOUGH, and it is a property of replay.go rather than a
// trick. replayOffers is a PURE FOLD over events at or before a target: it drops
// every operation and every trade above the target and applies the rest in
// ledger order. So the state at the first of the month is a fold over a PREFIX of
// the same event list that produces the state at the twenty-eighth. Fetch once at
// the LATEST target with a floor at or below the EARLIEST, and every book in
// between is arithmetic.
//
// THE THREE SENTENCES THE YELLOW ZONE ASKS FOR. The decision: gather the
// operations and trades once at the latest target and fold them separately at
// each target, rather than reconstructing each target independently. The reason
// is that the fetch is the entire cost and the fold is free, so the series is
// priced like one reconstruction instead of like twenty-eight, which is the
// difference between a run that finishes inside the Horizon request budget and
// one that does not. The alternative rejected: looping ReconstructBook over the
// targets, which is four lines instead of this file and needs no refactor, and
// was rejected because NFR-6 caps this repository at 3000 requests an hour and a
// single February target was measured on 5 September 2026 at over ten minutes of
// walking on its own.
//
// WHAT THIS DOES NOT FIX, and both are replay.go's limits arriving unchanged.
// Pools are still not reconstructed, so every snapshot here carries none and a
// combined depth figure from one would be wrong. And an offer created BELOW the
// floor is invisible in every point of the series equally, which reads as a
// thinner book on every day rather than as an error on one. The floor is one
// number for the whole series precisely so that the reading depth does not vary
// between the points being compared: a series whose early rows saw less history
// than its late rows would show a trend that is an artefact of the sampling.
package horizon

import (
	"context"
	"fmt"
	"sort"

	"github.com/Keel-Official/keel-backend/internal/domain"
)

// replayInputs is everything a fold needs and nothing derived from one target.
type replayInputs struct {
	trades []domain.Trade
	ops    []offerOperation
	walk   walkStats
}

// walkStats is what the walk cost and how far it reached. It is shared by every
// point in a series, because one walk produced all of them.
type walkStats struct {
	Accounts        []AccountWalk
	FromTrades      int
	FromLiveOffers  int
	TradesRead      int
	OperationsRead  int
	OfferOperations int
	Truncated       int
	Unsizable       int
	StoppedAtFloor  int
	Failed          int
	EarliestOfferOp uint32
	Requests        int
}

func (s walkStats) into(out *ReplayResult) {
	out.Accounts = s.Accounts
	out.FromTrades = s.FromTrades
	out.FromLiveOffers = s.FromLiveOffers
	out.TradesRead = s.TradesRead
	out.OperationsRead = s.OperationsRead
	out.OfferOperations = s.OfferOperations
	out.Truncated = s.Truncated
	out.Unsizable = s.Unsizable
	out.StoppedAtFloor = s.StoppedAtFloor
	out.Failed = s.Failed
	out.EarliestOfferOp = s.EarliestOfferOp
	out.Requests = s.Requests
}

// SeriesQuery bounds one multi-target reconstruction. Every field except Targets
// means what it means in ReplayQuery, and TargetLedger is not among them because
// the targets are the list.
type SeriesQuery struct {
	// Targets are the ledgers whose books are wanted. Order does not matter and
	// duplicates are removed; the result is sorted ascending. The walk is done at
	// the largest of them.
	Targets []uint32

	TradesFromLedger   uint32
	TradeLookahead     uint32
	MaxPagesPerAccount int

	// SinceLedger is the operation floor, and for a series it is ONE number for
	// every point rather than one per point. See the file header: a floor that
	// moved with the target would give the early rows a shallower reading than
	// the late ones and the difference would look like a trend.
	SinceLedger uint32

	Progress func(AccountWalk)
}

// SeriesPoint is one reconstructed book and the part of the diagnosis that is
// specific to its target.
type SeriesPoint struct {
	Target   uint32
	Snapshot domain.Snapshot

	// MissingOfferIDs is computed per target, because an offer a later trade
	// proves was resting may have been created after an earlier target. It is the
	// strongest self-check this method has and it is not shared across points.
	MissingOfferIDs []int64

	// RestingOffers is how many offers the fold left on the book, both sides
	// together. A point where this is zero and the trade stream is not is the
	// shape of a walk that failed rather than of a market that emptied.
	RestingOffers int
}

// Complete reports whether the fold at this point found no hole in itself. It is
// the per-point half of ReplayResult.Complete, and the walk-level half lives on
// SeriesResult because one walk produced every point.
func (p SeriesPoint) Complete() bool { return len(p.MissingOfferIDs) == 0 }

// SeriesResult is every point plus the one walk they all came from.
type SeriesResult struct {
	Points []SeriesPoint

	Accounts        []AccountWalk
	FromTrades      int
	FromLiveOffers  int
	TradesRead      int
	OperationsRead  int
	OfferOperations int
	Truncated       int
	Unsizable       int
	StoppedAtFloor  int
	Failed          int
	EarliestOfferOp uint32
	TradeWindowFrom uint32
	Requests        int
}

// WalkComplete reports whether the single walk behind every point finished
// without a failure or a truncation.
//
// IT IS NOT A CLAIM ABOUT ANY BOOK. A walk that finished can still have missed an
// offer created below the floor, which is why the floor and the earliest
// operation actually reached are reported beside it.
func (r SeriesResult) WalkComplete() bool { return r.Failed == 0 && r.Truncated == 0 }

// ReconstructSeries rebuilds the book at every target from one walk.
//
// The error cases are ReconstructBook's, plus one of its own: a target above the
// largest one is not reachable, so the largest target is what the walk is done
// at and the caller cannot ask for more afterwards.
func (c *Client) ReconstructSeries(ctx context.Context, base, quote domain.Asset, q SeriesQuery) (SeriesResult, error) {
	var out SeriesResult

	targets := dedupeSorted(q.Targets)
	if len(targets) == 0 {
		return out, fmt.Errorf("horizon: a series needs at least one target ledger")
	}
	latest := targets[len(targets)-1]

	// THE FLOOR MUST BE AT OR BELOW THE EARLIEST TARGET, and this is an error
	// rather than a clamp. A floor above the earliest target means that point's
	// book is folded from operations that all post-date it, so it comes back
	// EMPTY and an empty book is this product's loudest finding. Producing one by
	// a configuration mistake is the single worst thing this file could do.
	if q.SinceLedger != 0 && q.SinceLedger > targets[0] {
		return out, fmt.Errorf(
			"horizon: the operation floor is ledger %d and the earliest target is %d, "+
				"so that point would be folded from operations that all come after it and "+
				"would report an empty book; set SinceLedger at or below %d",
			q.SinceLedger, targets[0], targets[0])
	}

	in, err := c.gatherReplay(ctx, base, quote, ReplayQuery{
		TargetLedger:       latest,
		TradesFromLedger:   q.TradesFromLedger,
		TradeLookahead:     q.TradeLookahead,
		MaxPagesPerAccount: q.MaxPagesPerAccount,
		SinceLedger:        q.SinceLedger,
		Progress:           q.Progress,
	})
	if err != nil {
		return out, err
	}

	out.Accounts = in.walk.Accounts
	out.FromTrades = in.walk.FromTrades
	out.FromLiveOffers = in.walk.FromLiveOffers
	out.TradesRead = in.walk.TradesRead
	out.OperationsRead = in.walk.OperationsRead
	out.OfferOperations = in.walk.OfferOperations
	out.Truncated = in.walk.Truncated
	out.Unsizable = in.walk.Unsizable
	out.StoppedAtFloor = in.walk.StoppedAtFloor
	out.Failed = in.walk.Failed
	out.EarliestOfferOp = in.walk.EarliestOfferOp
	out.Requests = in.walk.Requests
	out.TradeWindowFrom = q.TradesFromLedger

	out.Points = make([]SeriesPoint, 0, len(targets))
	for _, t := range targets {
		state := replayOffers(in.ops, in.trades, t)
		book := bookFromOffers(state, base, quote)
		out.Points = append(out.Points, SeriesPoint{
			Target: t,
			Snapshot: domain.Snapshot{
				Base:      base,
				Quote:     quote,
				LedgerSeq: t,
				Book:      book,
				// Pools stay nil, exactly as in ReconstructBook, and for the same
				// reason. The absence is not a claim that no pool existed.
				Source: domain.DataSourceOffersImplied,
			},
			MissingOfferIDs: missingOffers(in.ops, in.trades, t),
			RestingOffers:   len(book.Bids) + len(book.Asks),
		})
	}
	return out, nil
}

// dedupeSorted returns the targets sorted ascending with duplicates removed. Two
// runs over the same list must produce the same points in the same order, which
// is NFR-9 applied to a series rather than to a number.
func dedupeSorted(in []uint32) []uint32 {
	if len(in) == 0 {
		return nil
	}
	cp := make([]uint32, len(in))
	copy(cp, in)
	sort.Slice(cp, func(i, j int) bool { return cp[i] < cp[j] })
	out := cp[:1]
	for _, v := range cp[1:] {
		if v != out[len(out)-1] {
			out = append(out, v)
		}
	}
	return out
}
