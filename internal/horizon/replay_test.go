package horizon

import (
	"context"
	"strings"
	"testing"

	"github.com/Keel-Official/keel-backend/internal/domain"
	"github.com/shopspring/decimal"
)

// The two operations that built the golden fixture's book, and the trade that
// took it apart. Their result XDR is in offerxdr_test.go, fetched from mainnet
// and pasted verbatim, so these tests replay REAL operations without a network.
//
// The expected book is testdata/fixtures/ustry_pre_exploit.md, computed by hand
// before any of this existed. Nothing here was produced by running the replay.
const (
	askCreateTOID    = int64(263453036239003649) // ledger 61339940, 21 Feb 23:38:51
	bidCreateTOID    = int64(263453066303434753) // ledger 61339947, 21 Feb 23:39:31
	manipulationTOID = int64(263454423513071617) // ledger 61340263, 22 Feb 00:10:21
)

func mustOp(t *testing.T, xdr string, toid int64, submitted int64) offerOperation {
	t.Helper()
	res, err := ParseManageOfferResult(xdr, TOIDOperationIndex(toid))
	if err != nil {
		t.Fatalf("parsing the result of operation %d: %v", toid, err)
	}
	return offerOperation{
		TOID:             toid,
		Ledger:           TOIDLedger(toid),
		SubmittedOfferID: submitted,
		Result:           res,
	}
}

// theTwoCreates is the pair of operations that left the fixture's book behind.
func theTwoCreates(t *testing.T) []offerOperation {
	t.Helper()
	return []offerOperation{
		mustOp(t, xdrAskCreate, askCreateTOID, 0),
		mustOp(t, xdrBidCreate, bidCreateTOID, 0),
	}
}

// theManipulation is the trade that took the ask, as Horizon reports it. The
// counter offer id is SYNTHETIC: the taker crossed the whole book and never
// rested, so Horizon named its side by OR-ing the operation TOID with bit 62.
func theManipulation() domain.Trade {
	return domain.Trade{
		ID:             "263454423513071617-0",
		OperationID:    "263454423513071617",
		FillIndex:      0,
		LedgerSeq:      61340263,
		Type:           "orderbook",
		Price:          domain.Price{N: 266843207, D: 2500000},
		BaseAmount:     decimal.RequireFromString("0.0501003"),
		CounterAmount:  decimal.RequireFromString("5.3475699"),
		BaseOfferID:    "1824788980",
		CounterOfferID: "4875140441940459521",
		BaseIsSeller:   true,
	}
}

func TestTheTwoCreatesRebuildTheFixtureBookExactly(t *testing.T) {
	// The target is the ledger BEFORE the manipulation, which is the state the
	// fixture describes: "the book immediately before the manipulation trade
	// executed inside ledger 61340263".
	state := replayOffers(theTwoCreates(t), nil, 61340262)
	book := bookFromOffers(state, testUSTRY, testUSDC)

	if len(book.Asks) != 1 || len(book.Bids) != 1 {
		t.Fatalf("book has %d ask(s) and %d bid(s), want 1 and 1", len(book.Asks), len(book.Bids))
	}

	ask := book.Asks[0]
	if want := decimal.RequireFromString("106.7372828"); !ask.Price.Decimal().Equal(want) {
		t.Errorf("ask price = %s, want %s", ask.Price.Decimal(), want)
	}
	if want := decimal.RequireFromString("1.2185312"); !ask.Amount.Equal(want) {
		t.Errorf("ask amount = %s, want %s base units", ask.Amount, want)
	}

	// THE BID IS THE HARD SIDE AND THIS IS THE ASSERTION THAT PROVES IT. The
	// ledger stored an offer SELLING 0.0001057 USDC at 1000/1057. The fixture
	// wants a bid of 0.0001 USTRY at 1.057. Both the price and the unit have to
	// turn round, and reading either straight through puts a bid on the book at
	// roughly the price of its own reciprocal.
	bid := book.Bids[0]
	if want := decimal.RequireFromString("1.057"); !bid.Price.Decimal().Equal(want) {
		t.Errorf("bid price = %s, want %s", bid.Price.Decimal(), want)
	}
	if want := decimal.RequireFromString("0.0001"); !bid.Amount.Equal(want) {
		t.Errorf("bid amount = %s, want %s base units", bid.Amount, want)
	}
	// The price is carried as the exact rational, never as a decimal that was
	// divided and stored back.
	if bid.Price.N != 1057 || bid.Price.D != 1000 {
		t.Errorf("bid price fraction = %d/%d, want 1057/1000", bid.Price.N, bid.Price.D)
	}
}

func TestTheManipulationLeavesTheAskAtItsKnownRemainder(t *testing.T) {
	// One ledger later, with the trade applied. 1.2185312 minus 0.0501003 is
	// 1.1684309, which is the figure scripts/audit-verification.sh reports the
	// offer still holding on chain.
	state := replayOffers(theTwoCreates(t), []domain.Trade{theManipulation()}, 61340263)
	book := bookFromOffers(state, testUSTRY, testUSDC)

	if len(book.Asks) != 1 {
		t.Fatalf("book has %d ask(s), want 1", len(book.Asks))
	}
	if want := decimal.RequireFromString("1.1684309"); !book.Asks[0].Amount.Equal(want) {
		t.Errorf("ask amount after the manipulation = %s, want %s", book.Asks[0].Amount, want)
	}
	// The taker rested nothing, so nothing was added to the bid side.
	if len(book.Bids) != 1 {
		t.Errorf("book has %d bid(s), want 1: the taker crossed entirely and rested nothing", len(book.Bids))
	}
}

func TestATradeAfterTheTargetLedgerIsNotApplied(t *testing.T) {
	// The same trade, against a target one ledger before it. A replay that
	// applied it would report a book that had already been eaten, which is the
	// error that makes a market look thinner than it was.
	state := replayOffers(theTwoCreates(t), []domain.Trade{theManipulation()}, 61340262)
	book := bookFromOffers(state, testUSTRY, testUSDC)

	if len(book.Asks) != 1 {
		t.Fatalf("book has %d ask(s), want 1", len(book.Asks))
	}
	if want := decimal.RequireFromString("1.2185312"); !book.Asks[0].Amount.Equal(want) {
		t.Errorf("ask amount = %s, want the untouched %s", book.Asks[0].Amount, want)
	}
}

func TestAnOperationAfterTheTargetLedgerIsNotApplied(t *testing.T) {
	ops := theTwoCreates(t)
	state := replayOffers(ops, nil, 61339940) // only the ask exists yet
	book := bookFromOffers(state, testUSTRY, testUSDC)

	if len(book.Asks) != 1 || len(book.Bids) != 0 {
		t.Errorf("book has %d ask(s) and %d bid(s), want 1 and 0: the bid was posted seven ledgers later",
			len(book.Asks), len(book.Bids))
	}
}

func TestACancelRemovesTheOfferItNames(t *testing.T) {
	// A cancel is a manage offer with a non-zero offer_id and an amount of zero.
	// Its result is DELETED and carries NO offer, so the id exists only in the
	// operation. An implementation reading only the result leaves the cancelled
	// level on the book for ever.
	ops := append(theTwoCreates(t), offerOperation{
		TOID:             askCreateTOID + 1000,
		Ledger:           61339950,
		SubmittedOfferID: 1824788980,
		Result:           resultingOffer{Effect: offerDeleted},
	})
	state := replayOffers(ops, nil, 61340262)
	book := bookFromOffers(state, testUSTRY, testUSDC)

	if len(book.Asks) != 0 {
		t.Errorf("book has %d ask(s), want 0: the ask was cancelled", len(book.Asks))
	}
	if len(book.Bids) != 1 {
		t.Errorf("book has %d bid(s), want 1: the cancel named the ask only", len(book.Bids))
	}
}

func TestACreateThatCrossedEverythingLeavesNothingResting(t *testing.T) {
	// The manipulation operation itself: submitted as a create, offer_id zero,
	// result DELETED. There is nothing to remove and nothing to add, and a
	// replay that treated the request as the outcome would put a phantom bid at
	// the crossing price onto the book.
	op := mustOp(t, xdrManipulation, manipulationTOID, 0)
	if op.Result.Effect != offerDeleted {
		t.Fatalf("effect = %s, want deleted", op.Result.Effect)
	}
	ops := append(theTwoCreates(t), op)
	state := replayOffers(ops, []domain.Trade{theManipulation()}, 61340263)
	book := bookFromOffers(state, testUSTRY, testUSDC)

	if len(book.Bids) != 1 {
		t.Errorf("book has %d bid(s), want 1: the crossing order rested nothing", len(book.Bids))
	}
	if want := decimal.RequireFromString("1.1684309"); !book.Asks[0].Amount.Equal(want) {
		t.Errorf("ask amount = %s, want %s", book.Asks[0].Amount, want)
	}
}

func TestAnOfferConsumedToZeroLeavesTheBook(t *testing.T) {
	full := theManipulation()
	full.BaseAmount = decimal.RequireFromString("1.2185312") // the whole ask
	full.CounterAmount = decimal.RequireFromString("130.0627093")

	state := replayOffers(theTwoCreates(t), []domain.Trade{full}, 61340263)
	book := bookFromOffers(state, testUSTRY, testUSDC)

	if len(book.Asks) != 0 {
		t.Errorf("book has %d ask(s), want 0: the whole level was taken", len(book.Asks))
	}
}

func TestASyntheticOfferIdIsNeverTreatedAsResting(t *testing.T) {
	const toid = manipulationTOID
	synthetic := toid | syntheticOfferBit

	if !syntheticOffer(synthetic) {
		t.Errorf("%d is not recognised as synthetic", synthetic)
	}
	if syntheticOffer(1824788980) {
		t.Error("a real resting offer id was called synthetic")
	}
	// The measurement in the header of replay.go, asserted rather than quoted.
	if want := int64(4875140441940459521); synthetic != want {
		t.Errorf("toid | bit62 = %d, want the id Horizon actually reported, %d", synthetic, want)
	}

	// A trade naming ONLY synthetic ids reports no missing offer. Without this
	// the completeness check counts every taker in the window as a hole, and in
	// February 2026 that is 9,478 of 10,077 ids.
	tr := theManipulation()
	tr.BaseOfferID = "4875140441940459521"
	if got := missingOffers(nil, []domain.Trade{tr}, 61340263); len(got) != 0 {
		t.Errorf("missing offers = %v, want none: both ids are transient", got)
	}
}

func TestAnOfferNamedByATradeAndNeverSeenIsReportedAsMissing(t *testing.T) {
	// The whole point of the completeness check: an offer the account discovery
	// never reached shows up here instead of quietly shrinking the book.
	got := missingOffers(nil, []domain.Trade{theManipulation()}, 61340263)
	if len(got) != 1 || got[0] != 1824788980 {
		t.Errorf("missing offers = %v, want [1824788980]", got)
	}

	// And once the operation that created it is in hand, it is no longer missing.
	if got := missingOffers(theTwoCreates(t), []domain.Trade{theManipulation()}, 61340263); len(got) != 0 {
		t.Errorf("missing offers = %v, want none once the create was seen", got)
	}
}

func TestAnOfferOnAnotherPairIsNotOnThisBook(t *testing.T) {
	other := domain.Asset{Code: "XLM", Type: domain.AssetTypeNative}
	state := replayOffers(theTwoCreates(t), nil, 61340262)

	book := bookFromOffers(state, testUSTRY, other)
	if len(book.Asks) != 0 || len(book.Bids) != 0 {
		t.Errorf("USTRY/XLM has %d ask(s) and %d bid(s) from USTRY/USDC offers, want none",
			len(book.Asks), len(book.Bids))
	}
}

func TestTheReplayDoesNotDependOnTheOrderEventsArriveIn(t *testing.T) {
	ops := theTwoCreates(t)
	reversed := []offerOperation{ops[1], ops[0]}

	a := bookFromOffers(replayOffers(ops, nil, 61340262), testUSTRY, testUSDC)
	b := bookFromOffers(replayOffers(reversed, nil, 61340262), testUSTRY, testUSDC)

	if len(a.Asks) != len(b.Asks) || len(a.Bids) != len(b.Bids) {
		t.Fatalf("level counts differ between input orderings")
	}
	for i := range a.Asks {
		if a.Asks[i].Price.Cmp(b.Asks[i].Price) != 0 || !a.Asks[i].Amount.Equal(b.Asks[i].Amount) {
			t.Errorf("ask %d differs between input orderings", i)
		}
	}
	for i := range a.Bids {
		if a.Bids[i].Price.Cmp(b.Bids[i].Price) != 0 || !a.Bids[i].Amount.Equal(b.Bids[i].Amount) {
			t.Errorf("bid %d differs between input orderings", i)
		}
	}
}

func TestBothSidesComeBackBestFirst(t *testing.T) {
	// Two asks and two bids, deliberately out of order in the state, because a
	// map has no order at all and every comparison against a live snapshot is
	// level by level.
	state := map[int64]*restingOffer{
		1: {ID: 1, Selling: refOf(testUSTRY), Buying: refOf(testUSDC), Amount: decimal.New(10, 0), PriceN: 3, PriceD: 1},
		2: {ID: 2, Selling: refOf(testUSTRY), Buying: refOf(testUSDC), Amount: decimal.New(10, 0), PriceN: 2, PriceD: 1},
		3: {ID: 3, Selling: refOf(testUSDC), Buying: refOf(testUSTRY), Amount: decimal.New(10, 0), PriceN: 1, PriceD: 1},
		4: {ID: 4, Selling: refOf(testUSDC), Buying: refOf(testUSTRY), Amount: decimal.New(10, 0), PriceN: 2, PriceD: 1},
	}
	book := bookFromOffers(state, testUSTRY, testUSDC)

	if len(book.Asks) != 2 || len(book.Bids) != 2 {
		t.Fatalf("book has %d ask(s) and %d bid(s), want 2 and 2", len(book.Asks), len(book.Bids))
	}
	if book.Asks[0].Price.Cmp(book.Asks[1].Price) > 0 {
		t.Error("asks are not cheapest first")
	}
	if book.Bids[0].Price.Cmp(book.Bids[1].Price) < 0 {
		t.Error("bids are not dearest first")
	}
}

func TestAWindowPairingThatWouldInflateTheBookIsRefused(t *testing.T) {
	// The failure this guards is the only one here that runs in the optimistic
	// direction. Offers come from the operation walk and consumption from the
	// trade walk, so an offer created inside the operation window and eaten
	// before the trade window starts is never decremented and rests for ever.
	// Measured on 26 August 2026: no operation floor against a ten thousand
	// ledger trade window applied 8,253 offer operations against 489 trades and
	// produced 334 asks for a ledger the chain proves had one.
	f := newFakeHorizon(t)
	c, _ := f.client()
	ctx := context.Background()

	_, err := c.ReconstructBook(ctx, testUSTRY, testUSDC, ReplayQuery{
		TargetLedger:     61340262,
		TradesFromLedger: 61330000,
		SinceLedger:      0, // no floor: the walk goes back years, the trades do not
	})
	if err == nil {
		t.Error("an unbounded operation walk against a bounded trade walk was accepted")
	}

	_, err = c.ReconstructBook(ctx, testUSTRY, testUSDC, ReplayQuery{
		TargetLedger:     61340262,
		TradesFromLedger: 61330000,
		SinceLedger:      61300000, // operations reach below where the trades start
	})
	if err == nil {
		t.Error("a trade window starting after the operation floor was accepted")
	}

	// Aligned windows are fine, and so is an operation floor above the trade
	// start, which only means more trades were read than strictly needed.
	if _, err := c.ReconstructBook(ctx, testUSTRY, testUSDC, ReplayQuery{
		TargetLedger:     61340262,
		TradesFromLedger: 61300000,
		SinceLedger:      61300000,
	}); err != nil && !strings.Contains(err.Error(), "trades") {
		// The fake serves no /trades handler, so a transport error here is
		// expected; what must NOT happen is the validation rejecting it.
		if strings.Contains(err.Error(), "must be at or below") || strings.Contains(err.Error(), "for ever") {
			t.Errorf("aligned windows were rejected: %v", err)
		}
	}
}

func TestTheInflationFlagIsSeparateFromTheOtherGaps(t *testing.T) {
	// Truncation and missing offers make a book THINNER. This one makes it
	// DEEPER, so Complete() has to fail on it and the caller has to be able to
	// tell the two apart.
	r := ReplayResult{EarliestOfferOp: 60000000, TradeWindowFrom: 61330000}
	if !r.MayBeInflated() {
		t.Error("offers applied from before the trade window did not raise the flag")
	}
	if r.Complete() {
		t.Error("Complete() is true for a book that may carry offers already eaten")
	}

	aligned := ReplayResult{EarliestOfferOp: 61330000, TradeWindowFrom: 61330000}
	if aligned.MayBeInflated() {
		t.Error("aligned windows raised the flag")
	}
	if !aligned.Complete() {
		t.Error("Complete() is false with no gap of any kind")
	}

	// Zero on either side means "not established", not "zero", so it must not
	// fire: a whole-history walk has no window to be outside of.
	whole := ReplayResult{EarliestOfferOp: 60000000, TradeWindowFrom: 0}
	if whole.MayBeInflated() {
		t.Error("a whole-history trade walk raised the flag")
	}
}
