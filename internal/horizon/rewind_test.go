package horizon

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/shopspring/decimal"
)

// offerJSON is one /offers record. price_r arrives as JSON NUMBERS on this
// endpoint, the opposite of /trades, which is the whole reason priceFraction
// accepts both shapes.
func offerJSON(id string, sellCode, sellIssuer, sellType, buyCode, buyIssuer, buyType, amount string, n, d int64, lastModified int) string {
	return fmt.Sprintf(`{
	  "id": %q, "paging_token": %q, "seller": "GSELLER",
	  "selling": {"asset_type": %q, "asset_code": %q, "asset_issuer": %q},
	  "buying":  {"asset_type": %q, "asset_code": %q, "asset_issuer": %q},
	  "amount": %q, "price_r": {"n": %d, "d": %d},
	  "last_modified_ledger": %d
	}`, id, id, sellType, sellCode, sellIssuer, buyType, buyCode, buyIssuer, amount, n, d, lastModified)
}

func askJSON(id, amount string, n, d int64, lastModified int) string {
	return offerJSON(id, "USTRY", testUSTRY.Issuer, "credit_alphanum12",
		"USDC", testUSDC.Issuer, "credit_alphanum4", amount, n, d, lastModified)
}

func bidJSON(id, amount string, n, d int64, lastModified int) string {
	return offerJSON(id, "USDC", testUSDC.Issuer, "credit_alphanum4",
		"USTRY", testUSTRY.Issuer, "credit_alphanum12", amount, n, d, lastModified)
}

func offersPageJSON(records ...string) string {
	body := `{"_embedded":{"records":[`
	for i, r := range records {
		if i > 0 {
			body += ","
		}
		body += r
	}
	return body + "]}}"
}

// rewindFake serves /offers for both directions and /trades, which is every
// endpoint RewindBook touches.
func rewindFake(t *testing.T, asks, bids string, trades string) *fakeHorizon {
	t.Helper()
	f := newFakeHorizon(t)
	f.handler["/offers"] = func(w http.ResponseWriter, r *http.Request) {
		// The direction is decided by what the offer SELLS. Asking one way round
		// returns half a book and no error.
		if r.URL.Query().Get("selling_asset_code") == "USTRY" {
			_, _ = fmt.Fprint(w, asks)
			return
		}
		_, _ = fmt.Fprint(w, bids)
	}
	f.handler["/trades"] = func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, trades)
	}
	// The fake stamps every response with the incident ledger by default, which
	// is months behind the targets these tests use. RewindBook refuses a target
	// ahead of the network, correctly, so the fake has to be told what "now" is.
	f.ledger["/offers"] = "64134034"
	return f
}

// tradeAtLedger builds a /trades record whose TOID really is in the ledger given,
// with the offer ids named explicitly. The generic helper in trades_test.go pins
// both, which is fine there and is exactly what these tests have to vary.
func tradeAtLedger(ledger uint32, baseOfferID, counterOfferID string) string {
	toid := int64(ledger) << 32
	return fmt.Sprintf(`{
	  "paging_token": "%d-0",
	  "ledger_close_time": "2026-08-26T09:00:00Z",
	  "trade_type": "orderbook",
	  "base_account": "GBASE", "base_offer_id": %q, "base_amount": "0.05",
	  "base_asset_type": "credit_alphanum12", "base_asset_code": "USTRY", "base_asset_issuer": %q,
	  "counter_account": "GCOUNTER", "counter_offer_id": %q, "counter_amount": "5.34",
	  "counter_asset_type": "credit_alphanum4", "counter_asset_code": "USDC", "counter_asset_issuer": %q,
	  "price": {"n": "266843207", "d": "2500000"}
	}`, toid, baseOfferID, testUSTRY.Issuer, counterOfferID, testUSDC.Issuer)
}

const noTrades = `{"_embedded":{"records":[]}}`

func TestAnOfferUnchangedSinceTheTargetIsCarriedBackWhole(t *testing.T) {
	const target = 64129589
	f := rewindFake(t,
		offersPageJSON(askJSON("100", "1.2185312", 266843207, 2500000, target-50)),
		offersPageJSON(bidJSON("200", "0.0001057", 1000, 1057, target-10)),
		noTrades)
	c, _ := f.client()

	got, err := c.RewindBook(context.Background(), testUSTRY, testUSDC, target)
	if err != nil {
		t.Fatalf("RewindBook: %v", err)
	}
	if got.Carried != 2 || got.Changed != 0 || got.Gone != 0 {
		t.Errorf("carried %d, changed %d, gone %d; want 2, 0, 0", got.Carried, got.Changed, got.Gone)
	}
	if !got.Certain() {
		t.Error("Certain() is false with nothing changed and nothing gone")
	}

	book := got.Snapshot.Book
	if len(book.Asks) != 1 || len(book.Bids) != 1 {
		t.Fatalf("book has %d ask(s) and %d bid(s), want 1 and 1", len(book.Asks), len(book.Bids))
	}
	if want := decimal.RequireFromString("106.7372828"); !book.Asks[0].Price.Decimal().Equal(want) {
		t.Errorf("ask price = %s, want %s", book.Asks[0].Price.Decimal(), want)
	}
	// The bid side turns round twice, exactly as it does in the forward replay:
	// the price inverts and the amount converts from quote units into base ones.
	if want := decimal.RequireFromString("1.057"); !book.Bids[0].Price.Decimal().Equal(want) {
		t.Errorf("bid price = %s, want %s", book.Bids[0].Price.Decimal(), want)
	}
	if want := decimal.RequireFromString("0.0001"); !book.Bids[0].Amount.Equal(want) {
		t.Errorf("bid amount = %s, want %s base units", book.Bids[0].Amount, want)
	}
	if got.Snapshot.LedgerSeq != target {
		t.Errorf("snapshot ledger = %d, want the target %d", got.Snapshot.LedgerSeq, target)
	}
}

func TestAnOfferModifiedAfterTheTargetIsLeftOffAndCounted(t *testing.T) {
	const target = 64129589
	f := rewindFake(t,
		offersPageJSON(
			askJSON("100", "1.2185312", 266843207, 2500000, target-50), // untouched
			askJSON("101", "9.9", 2, 1, target+1),                      // moved after
		),
		offersPageJSON(),
		noTrades)
	c, _ := f.client()

	got, err := c.RewindBook(context.Background(), testUSTRY, testUSDC, target)
	if err != nil {
		t.Fatalf("RewindBook: %v", err)
	}
	if got.Carried != 1 || got.Changed != 1 {
		t.Errorf("carried %d, changed %d; want 1 and 1", got.Carried, got.Changed)
	}
	if got.Certain() {
		t.Error("Certain() is true although an offer moved after the target")
	}
	// LEFT OFF, not guessed at. Its current state is not its state then, and a
	// level that was not there is worse than a level that is missing.
	if len(got.Snapshot.Book.Asks) != 1 {
		t.Errorf("book has %d ask(s), want 1: the moved offer must not be on it", len(got.Snapshot.Book.Asks))
	}
}

func TestADepartedOfferIsCountedAndNeverPutBack(t *testing.T) {
	// A trade names an offer that is not resting now. It might have been on the
	// book at the target and it might have been created afterwards, and a trade
	// cannot tell those apart. Putting it back is how a rebuilt book comes out
	// DEEPER than the real one, which is the failure this whole file avoids.
	const target = 64129589
	// Offer 100 is still resting and was carried. Offer 999 is gone.
	trades := tradesPageJSON("", tradeAtLedger(target+10, "999", "0"))
	f := rewindFake(t,
		offersPageJSON(askJSON("100", "1.2185312", 266843207, 2500000, target-50)),
		offersPageJSON(),
		trades)
	c, _ := f.client()

	got, err := c.RewindBook(context.Background(), testUSTRY, testUSDC, target)
	if err != nil {
		t.Fatalf("RewindBook: %v", err)
	}
	if got.Gone != 1 {
		t.Errorf("gone = %d, want 1", got.Gone)
	}
	if got.Certain() {
		t.Error("Certain() is true although an offer is gone and unaccounted for")
	}
	if len(got.Snapshot.Book.Asks) != 1 {
		t.Errorf("book has %d ask(s), want 1: a departed offer must not be added back",
			len(got.Snapshot.Book.Asks))
	}
}

func TestAnOfferStillRestingIsNotAlsoCountedAsGone(t *testing.T) {
	const target = 64129589
	trades := tradesPageJSON("", tradeAtLedger(target+10, "100", "0"))
	f := rewindFake(t,
		offersPageJSON(askJSON("100", "1.2185312", 266843207, 2500000, target-50)),
		offersPageJSON(),
		trades)
	c, _ := f.client()

	got, err := c.RewindBook(context.Background(), testUSTRY, testUSDC, target)
	if err != nil {
		t.Fatalf("RewindBook: %v", err)
	}
	if got.Gone != 0 {
		t.Errorf("gone = %d, want 0: offer 100 is still on the live book", got.Gone)
	}
}

func TestASyntheticOfferIdIsNotADepartedOffer(t *testing.T) {
	// A taker that crossed the whole book never rested, so there is no offer to
	// be missing and counting one would make every taker look like a hole. Nearly
	// every trade has one of these on one side.
	const target = 64129589
	synthetic := fmt.Sprintf("%d", (int64(target+10)<<32)|syntheticOfferBit)
	trades := tradesPageJSON("", tradeAtLedger(target+10, synthetic, "0"))
	f := rewindFake(t, offersPageJSON(), offersPageJSON(), trades)
	c, _ := f.client()

	got, err := c.RewindBook(context.Background(), testUSTRY, testUSDC, target)
	if err != nil {
		t.Fatalf("RewindBook: %v", err)
	}
	if got.Gone != 0 {
		t.Errorf("gone = %d, want 0: the only named offer is transient", got.Gone)
	}
	if !got.Certain() {
		t.Error("Certain() is false although nothing real was missing")
	}
}

func TestATargetAheadOfTheNetworkIsRefused(t *testing.T) {
	f := rewindFake(t, offersPageJSON(), offersPageJSON(), noTrades)
	c, _ := f.client()

	if _, err := c.RewindBook(context.Background(), testUSTRY, testUSDC, 99999999); err == nil {
		t.Error("a target ahead of the network was accepted")
	}
}

func TestRewindNeedsATargetLedger(t *testing.T) {
	f := rewindFake(t, offersPageJSON(), offersPageJSON(), noTrades)
	c, _ := f.client()
	if _, err := c.RewindBook(context.Background(), testUSTRY, testUSDC, 0); err == nil {
		t.Error("a zero target was accepted")
	}
}

func TestAnEmptyBookRewindsToAnEmptyBook(t *testing.T) {
	// Twelve of the sixty committed recordings have no orders at all, and an
	// asset with no executable price is this product's most interesting finding
	// rather than an error. It must come back empty and certain.
	f := rewindFake(t, offersPageJSON(), offersPageJSON(), noTrades)
	c, _ := f.client()

	got, err := c.RewindBook(context.Background(), testUSTRY, testUSDC, 64129589)
	if err != nil {
		t.Fatalf("RewindBook: %v", err)
	}
	if len(got.Snapshot.Book.Bids) != 0 || len(got.Snapshot.Book.Asks) != 0 {
		t.Errorf("book is not empty: %d bid(s), %d ask(s)",
			len(got.Snapshot.Book.Bids), len(got.Snapshot.Book.Asks))
	}
	if !got.Certain() {
		t.Error("an empty book with no window activity is not certain")
	}
}
