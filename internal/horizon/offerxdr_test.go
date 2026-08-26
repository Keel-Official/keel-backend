package horizon

import (
	"encoding/base64"
	"errors"
	"testing"
)

// Three real transaction results, fetched from Horizon mainnet on 26 August 2026
// and pasted verbatim. Anybody can refetch them and diff:
//
//	curl -s https://horizon.stellar.org/transactions/<hash> | jq -r .result_xdr
//
// They are the two levels of the golden fixture and the trade that broke them.
// Nothing in this file was produced by running the decoder it tests: the expected
// numbers come from testdata/fixtures/ustry_pre_exploit.md and from
// docs/methodology/10-validation.md section 7, both of which predate it.
const (
	// 09e1a9d1197c9bf0af4e87da328c4f2d5eb49b487630aa61991fb5c1c4637cdb
	// ledger 61339940, one operation. manage_sell_offer, a create that rested.
	// This is the ask the fixture records: 1.2185312 USTRY at 106.7372828.
	xdrAskCreate = "AAAAAAAAAGQAAAAAAAAAAQAAAAAAAAADAAAAAAAAAAAAAAAAAAAAAJpembFE/VtjP13pne4blHTaGdvRe392tVAJayICUbDfAAAAAGzEEfQAAAACVVNUUlkAAAAAAAAAAAAAAKOKGH1tQRNeffXJlHRIKKBJ4OB8ITO7TtohmZxn4sE2AAAAAVVTREMAAAAAO5kROA7+mIugqJAOsc/kTzZvfb6Ua+0HckD39iTfFcUAAAAAALnu4A/ntEcAJiWgAAAAAAAAAAAAAAAA"

	// 3b504c319bdadf1e3ec49cc9d186083b1ef84c84af219bc0d4bab2bc700c3aa4
	// ledger 61339947, one operation. manage_buy_offer, a create that rested.
	// This is the bid the fixture records: 0.0001 USTRY at 1.0570000.
	xdrBidCreate = "AAAAAAAAAGQAAAAAAAAAAQAAAAAAAAAMAAAAAAAAAAAAAAAAAAAAAJpembFE/VtjP13pne4blHTaGdvRe392tVAJayICUbDfAAAAAGzEEloAAAABVVNEQwAAAAA7mRE4Dv6Yi6CokA6xz+RPNm99vpRr7QdyQPf2JN8VxQAAAAJVU1RSWQAAAAAAAAAAAAAAo4oYfW1BE1599cmUdEgooEng4HwhM7tO2iGZnGfiwTYAAAAAAAAEIQAAA+gAAAQhAAAAAAAAAAAAAAAA"

	// 60fe039e96e88402d175c8de68e80651874ab125880dd384a1636914ba95bef1
	// ledger 61340263, one operation. THE MANIPULATION. A manage_buy_offer that
	// crossed the whole book and rested nothing, taking the ask above.
	xdrManipulation = "AAAAAAAAAGQAAAAAAAAAAQAAAAAAAAAMAAAAAAAAAAEAAAABAAAAAJpembFE/VtjP13pne4blHTaGdvRe392tVAJayICUbDfAAAAAGzEEfQAAAACVVNUUlkAAAAAAAAAAAAAAKOKGH1tQRNeffXJlHRIKKBJ4OB8ITO7TtohmZxn4sE2AAAAAAAHpQsAAAABVVNEQwAAAAA7mRE4Dv6Yi6CokA6xz+RPNm99vpRr7QdyQPf2JN8VxQAAAAADL/lzAAAAAgAAAAA="
)

func TestTheAskOfTheFixtureDecodesFromItsOwnTransaction(t *testing.T) {
	got, err := ParseManageOfferResult(xdrAskCreate, 0)
	if err != nil {
		t.Fatalf("ParseManageOfferResult: %v", err)
	}

	if got.Effect != offerCreated {
		t.Errorf("effect = %s, want created", got.Effect)
	}
	// Horizon's own operation record says offer_id 0 for this create and serves
	// no effects at all. This number exists nowhere else.
	if want := int64(1824788980); got.OfferID != want {
		t.Errorf("offer id = %d, want %d", got.OfferID, want)
	}
	if got.Selling.AssetCode != "USTRY" || got.Selling.AssetType != "credit_alphanum12" {
		t.Errorf("selling = %+v, want USTRY credit_alphanum12", got.Selling)
	}
	if got.Buying.AssetCode != "USDC" || got.Buying.AssetType != "credit_alphanum4" {
		t.Errorf("buying = %+v, want USDC credit_alphanum4", got.Buying)
	}
	// 1.2185312 USTRY, the fixture's ask amount, in stroops.
	if want := int64(12185312); got.Amount != want {
		t.Errorf("amount = %d stroops, want %d", got.Amount, want)
	}
	// The fixture's ask price, exactly, as buying per selling. The offer sells
	// USTRY and buys USDC, so this is already quote per base and needs no flip.
	if got.PriceN != 266843207 || got.PriceD != 2500000 {
		t.Errorf("price = %d/%d, want 266843207/2500000", got.PriceN, got.PriceD)
	}
	if len(got.Claimed) != 0 {
		t.Errorf("claimed %d offers, want 0: this offer rested and took nothing", len(got.Claimed))
	}
}

func TestTheBidOfTheFixtureDecodesInSellingTermsAndNotTheRequestedOnes(t *testing.T) {
	got, err := ParseManageOfferResult(xdrBidCreate, 0)
	if err != nil {
		t.Fatalf("ParseManageOfferResult: %v", err)
	}

	if got.Effect != offerCreated {
		t.Errorf("effect = %s, want created", got.Effect)
	}
	if want := int64(1824789082); got.OfferID != want {
		t.Errorf("offer id = %d, want %d", got.OfferID, want)
	}

	// THE TRAP THIS ASSERTION EXISTS FOR. The operation asked to BUY 0.0001 USTRY
	// at 1.0570000. The ledger stored an offer SELLING 0.0001057 USDC at
	// 1000/1057 USTRY per USDC. Neither the amount nor the price is the number in
	// the request, and reading the request instead would put the bid on the book
	// at the wrong size in the wrong unit.
	if got.Selling.AssetCode != "USDC" || got.Buying.AssetCode != "USTRY" {
		t.Errorf("selling/buying = %s/%s, want USDC/USTRY", got.Selling.AssetCode, got.Buying.AssetCode)
	}
	if want := int64(1057); got.Amount != want {
		t.Errorf("amount = %d stroops of USDC, want %d, which is 0.0001057", got.Amount, want)
	}
	if got.PriceN != 1000 || got.PriceD != 1057 {
		t.Errorf("price = %d/%d, want 1000/1057, which inverts to the fixture's 1.057", got.PriceN, got.PriceD)
	}
}

func TestTheManipulationRestedNothingAndNamedTheOfferItTook(t *testing.T) {
	got, err := ParseManageOfferResult(xdrManipulation, 0)
	if err != nil {
		t.Fatalf("ParseManageOfferResult: %v", err)
	}

	// It was submitted as a create and the ledger recorded a delete, because it
	// crossed the whole book and left nothing behind. An implementation that
	// trusted the request over the result would put a phantom bid on the book.
	if got.Effect != offerDeleted {
		t.Errorf("effect = %s, want deleted", got.Effect)
	}
	if got.OfferID != 0 || got.Amount != 0 {
		t.Errorf("a deleted effect carries no offer, got id %d amount %d", got.OfferID, got.Amount)
	}

	if len(got.Claimed) != 1 {
		t.Fatalf("claimed %d offers, want 1", len(got.Claimed))
	}
	c := got.Claimed[0]
	if c.IsPool {
		t.Error("the claim is marked as a pool; the trade record says trade_type orderbook")
	}
	// It took the ask created by the first transaction in this file.
	if want := int64(1824788980); c.OfferID != want {
		t.Errorf("claimed offer = %d, want %d", c.OfferID, want)
	}
	// 0.0501003 USTRY for 5.3475699 USDC, which is the headline pair of numbers
	// in docs/methodology/10-validation.md section 7.
	if want := int64(501003); c.AmountSold != want {
		t.Errorf("amount sold = %d stroops of USTRY, want %d", c.AmountSold, want)
	}
	if want := int64(53475699); c.AmountBought != want {
		t.Errorf("amount bought = %d stroops of USDC, want %d", c.AmountBought, want)
	}
}

func TestAnOperationIndexOutsideTheTransactionIsAnError(t *testing.T) {
	if _, err := ParseManageOfferResult(xdrAskCreate, 1); err == nil {
		t.Error("index 1 of a one operation transaction was accepted")
	}
	if _, err := ParseManageOfferResult(xdrAskCreate, -1); err == nil {
		t.Error("a negative index was accepted")
	}
}

func TestATruncatedResultIsAnErrorAndNotAZeroOffer(t *testing.T) {
	// Half the bytes. A decoder that returns a zero value here puts an offer with
	// no id and no price onto the book, which is worse than refusing.
	short := xdrAskCreate[:len(xdrAskCreate)/2]
	if _, err := ParseManageOfferResult(short, 0); err == nil {
		t.Error("a truncated result decoded without error")
	}
}

func TestGarbageIsRefused(t *testing.T) {
	if _, err := ParseManageOfferResult("not base64 at all !!", 0); err == nil {
		t.Error("a non base64 body decoded without error")
	}
	if _, err := ParseManageOfferResult("", 0); err == nil {
		t.Error("an empty body decoded without error")
	}
}

func TestTOIDSplitsIntoLedgerAndAZeroBasedOperationIndex(t *testing.T) {
	// The operation that created the fixture's ask. Its transaction closed in
	// ledger 61339940 and it is the first operation in that transaction, which
	// the TOID stores as 1.
	const toid = int64(263453036239003649)
	if got, want := TOIDLedger(toid), uint32(61339940); got != want {
		t.Errorf("ledger = %d, want %d", got, want)
	}
	if got := TOIDOperationIndex(toid); got != 0 {
		t.Errorf("operation index = %d, want 0", got)
	}
	// One based in the TOID, zero based out of it. Getting this backwards reads
	// the wrong operation's result out of a multi operation transaction, and the
	// result it reads is a valid one for a different offer.
	if got := TOIDOperationIndex(toid + 1); got != 1 {
		t.Errorf("operation index of the second operation = %d, want 1", got)
	}
}

func TestAnUnsizableEarlierResultIsNamedRatherThanGuessed(t *testing.T) {
	// A hand built two operation result whose FIRST operation is a Soroban
	// invokeHostFunction, type 24, whose success body is a recursive SCVal. There
	// is no width to compute, so the second operation cannot be reached. Guessing
	// would decode whatever follows into a plausible offer.
	//
	// feeCharged, txSUCCESS, 2 results, then opINNER + type 24 + success.
	b := []byte{
		0, 0, 0, 0, 0, 0, 0, 100, // feeCharged
		0, 0, 0, 0, // txSUCCESS
		0, 0, 0, 2, // two operation results
		0, 0, 0, 0, // opINNER
		0, 0, 0, opTypeInvokeHostFunction, // Soroban, a recursive SCVal body
		0, 0, 0, 0, // success
	}
	if _, err := ParseManageOfferResult(b64(b), 1); !errors.Is(err, ErrUnsizableResult) {
		t.Errorf("err = %v, want ErrUnsizableResult", err)
	}
}

func TestAnOperationThatIsNotAManageOfferIsNamedAsSuch(t *testing.T) {
	// opINNER, operation type 1 (payment), success.
	b := []byte{
		0, 0, 0, 0, 0, 0, 0, 100,
		0, 0, 0, 0,
		0, 0, 0, 1,
		0, 0, 0, 0,
		0, 0, 0, 1,
		0, 0, 0, 0,
	}
	if _, err := ParseManageOfferResult(b64(b), 0); !errors.Is(err, ErrNotAnOfferResult) {
		t.Errorf("err = %v, want ErrNotAnOfferResult", err)
	}
}

func TestAVoidBodiedEarlierResultIsSkippedAndTheOfferAfterItIsRead(t *testing.T) {
	// The common multi operation shape: a payment, then the manage offer. The
	// payment result is three int32s and nothing else, and getting that width
	// wrong is what makes the offer after it decode into rubbish.
	prefix := []byte{
		0, 0, 0, 0, 0, 0, 0, 100, // feeCharged
		0, 0, 0, 0, // txSUCCESS
		0, 0, 0, 2, // two operation results
		0, 0, 0, 0, // opINNER
		0, 0, 0, 1, // payment
		0, 0, 0, 0, // success, no body
	}
	// The ask create's own operation result, lifted out of the real transaction.
	real := mustB64Decode(t, xdrAskCreate)
	body := real[16:] // past feeCharged, txSUCCESS and the results count

	got, err := ParseManageOfferResult(b64(append(prefix, body...)), 1)
	if err != nil {
		t.Fatalf("ParseManageOfferResult: %v", err)
	}
	if want := int64(1824788980); got.OfferID != want {
		t.Errorf("offer id = %d, want %d; the payment before it was skipped by the wrong width", got.OfferID, want)
	}
}

func b64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

func mustB64Decode(t *testing.T, s string) []byte {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		t.Fatalf("decoding a constant in this file: %v", err)
	}
	return b
}
