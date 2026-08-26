package horizon

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/Keel-Official/keel-backend/internal/domain"
)

// tradeJSON is one /trades record. Note that the rational is the field called
// `price` here and its members are STRINGS, which is the real shape of this
// endpoint and the reason priceFraction accepts both forms.
func tradeJSON(token, closedAt string, n, d int64, baseAmt, counterAmt string) string {
	return fmt.Sprintf(`{
	  "paging_token": %q,
	  "ledger_close_time": %q,
	  "trade_type": "orderbook",
	  "base_account": "GBASE", "base_offer_id": "1",
	  "base_amount": %q,
	  "base_asset_type": "credit_alphanum12", "base_asset_code": "USTRY", "base_asset_issuer": %q,
	  "counter_account": "GCOUNTER", "counter_offer_id": "2",
	  "counter_amount": %q,
	  "counter_asset_type": "credit_alphanum4", "counter_asset_code": "USDC", "counter_asset_issuer": %q,
	  "price": {"n": "%d", "d": "%d"}
	}`, token, closedAt, baseAmt, testUSTRY.Issuer, counterAmt, testUSDC.Issuer, n, d)
}

func tradesPageJSON(next string, records ...string) string {
	body := "{"
	if next != "" {
		body += fmt.Sprintf(`"_links":{"next":{"href":%q}},`, next)
	}
	body += `"_embedded":{"records":[`
	for i, r := range records {
		if i > 0 {
			body += ","
		}
		body += r
	}
	return body + "]}}"
}

// The two real records either side of the manipulation. The TOIDs are the actual
// ones, so ledger 61340263 falls out of the paging token rather than being
// asserted separately.
const (
	honestToken = "263454256009383937-1"
	attackToken = "263454423513071617-0"
)

func TestTradesDecodesTheManipulationRecord(t *testing.T) {
	f := newFakeHorizon(t)
	f.handler["/trades"] = func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, tradesPageJSON("",
			tradeJSON(attackToken, "2026-02-22T00:10:21Z", 266843207, 2500000, "0.0501003", "5.3475699")))
	}
	c, _ := f.client()

	got, err := c.Trades(context.Background(), testUSTRY, testUSDC, TradeQuery{})
	if err != nil {
		t.Fatalf("Trades: %v", err)
	}
	if len(got.Trades) != 1 {
		t.Fatalf("want 1 trade, got %d", len(got.Trades))
	}
	tr := got.Trades[0]

	// The price is the exact rational and never the rounded string, which is
	// trap 2 of this zone in its /trades form.
	if want := "106.7372828"; tr.Price.Decimal().String() != want {
		t.Errorf("price = %s, want %s", tr.Price.Decimal(), want)
	}
	if tr.CounterAmount.String() != "5.3475699" {
		t.Errorf("counter amount = %s, want 5.3475699", tr.CounterAmount)
	}
	// The ledger comes out of the TOID's high 32 bits, which is decoding an
	// identifier. It is never computed from the close time.
	if tr.LedgerSeq != testLedger {
		t.Errorf("ledger = %d, want %d", tr.LedgerSeq, testLedger)
	}
	if tr.OperationID != "263454423513071617" || tr.FillIndex != 0 {
		t.Errorf("operation split = %q/%d", tr.OperationID, tr.FillIndex)
	}
	if !tr.ClosedAt.Equal(time.Date(2026, 2, 22, 0, 10, 21, 0, time.UTC)) {
		t.Errorf("closed at = %s", tr.ClosedAt)
	}
}

func TestTradesRefusesARecordThatCameBackInvertedRatherThanFlippingIt(t *testing.T) {
	// The same trade as Horizon serves it when the pair is NOT pinned: USDC is
	// the base and the fraction reads USTRY per USDC. Silently inverting it
	// would turn 106.74 into 0.0093 and every number downstream would inherit
	// the error, so the adapter refuses.
	inverted := fmt.Sprintf(`{
	  "paging_token": %q, "ledger_close_time": "2026-02-22T00:10:21Z", "trade_type": "orderbook",
	  "base_amount": "5.3475699",
	  "base_asset_type": "credit_alphanum4", "base_asset_code": "USDC", "base_asset_issuer": %q,
	  "counter_amount": "0.0501003",
	  "counter_asset_type": "credit_alphanum12", "counter_asset_code": "USTRY", "counter_asset_issuer": %q,
	  "price": {"n": "2500000", "d": "266843207"}
	}`, attackToken, testUSDC.Issuer, testUSTRY.Issuer)

	f := newFakeHorizon(t)
	f.handler["/trades"] = func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, tradesPageJSON("", inverted))
	}
	c, _ := f.client()

	_, err := c.Trades(context.Background(), testUSTRY, testUSDC, TradeQuery{})
	if !errors.Is(err, ErrPairMismatch) {
		t.Fatalf("err = %v, want ErrPairMismatch", err)
	}
}

func TestTradesWalksPagesAndStopsOnAnEmptyOne(t *testing.T) {
	// Horizon serves a next link on every page INCLUDING the last, so a walk
	// that stops when the link disappears never stops. The end of the collection
	// is an empty page.
	f := newFakeHorizon(t)
	page := 0
	f.handler["/trades"] = func(w http.ResponseWriter, _ *http.Request) {
		page++
		switch page {
		case 1:
			_, _ = fmt.Fprint(w, tradesPageJSON(f.srv.URL+"/trades?cursor=2",
				tradeJSON(honestToken, "2026-02-22T00:06:31Z", 2125646195, 2010206197, "0.0273371", "0.0289069")))
		case 2:
			_, _ = fmt.Fprint(w, tradesPageJSON(f.srv.URL+"/trades?cursor=3",
				tradeJSON(attackToken, "2026-02-22T00:10:21Z", 266843207, 2500000, "0.0501003", "5.3475699")))
		default:
			_, _ = fmt.Fprint(w, tradesPageJSON(f.srv.URL+"/trades?cursor=4"))
		}
	}
	c, _ := f.client()

	got, err := c.Trades(context.Background(), testUSTRY, testUSDC, TradeQuery{})
	if err != nil {
		t.Fatalf("Trades: %v", err)
	}
	if len(got.Trades) != 2 {
		t.Fatalf("want 2 trades across pages, got %d", len(got.Trades))
	}
	if got.Pages != 3 {
		t.Errorf("pages = %d, want 3 including the empty one", got.Pages)
	}
	if got.Stopped {
		t.Error("Stopped is true, but the walk ended at the collection and not at a predicate")
	}
}

func TestTradesStopsOnThePredicateAndExcludesTheTradeThatTrippedIt(t *testing.T) {
	boundary := time.Date(2026, 2, 22, 0, 10, 0, 0, time.UTC)

	f := newFakeHorizon(t)
	f.handler["/trades"] = func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, tradesPageJSON("",
			tradeJSON(honestToken, "2026-02-22T00:06:31Z", 2125646195, 2010206197, "0.0273371", "0.0289069"),
			tradeJSON(attackToken, "2026-02-22T00:10:21Z", 266843207, 2500000, "0.0501003", "5.3475699")))
	}
	c, _ := f.client()

	got, err := c.Trades(context.Background(), testUSTRY, testUSDC, TradeQuery{
		StopAfter: func(tr domain.Trade) bool { return !tr.ClosedAt.Before(boundary) },
	})
	if err != nil {
		t.Fatalf("Trades: %v", err)
	}
	if len(got.Trades) != 1 || got.Trades[0].ID != honestToken {
		t.Fatalf("want only the trade before the boundary, got %d", len(got.Trades))
	}
	// The difference between "the window closed" and "the data ran out" is the
	// difference between a complete last day and a partial one.
	if !got.Stopped {
		t.Error("Stopped is false, but the predicate ended the walk")
	}
}

func TestTradesSeeksWithALedgerAndNotWithATime(t *testing.T) {
	f := newFakeHorizon(t)
	var gotCursor string
	f.handler["/trades"] = func(w http.ResponseWriter, r *http.Request) {
		gotCursor = r.URL.Query().Get("cursor")
		_, _ = fmt.Fprint(w, tradesPageJSON(""))
	}
	c, _ := f.client()

	if _, err := c.Trades(context.Background(), testUSTRY, testUSDC, TradeQuery{FromLedger: 61340263}); err != nil {
		t.Fatalf("Trades: %v", err)
	}
	// The TOID of the first operation that could exist in that ledger. It is an
	// identifier built from a sequence, never a sequence built from a time.
	// 61340263 << 32. The attack's own TOID, 263454423513071617, sits just above
	// it, which is the arithmetic check that this is the right ledger's floor.
	if want := "263454423513038848"; gotCursor != want {
		t.Errorf("cursor = %q, want %q", gotCursor, want)
	}
}

func TestARepeatedPagingTokenIsNotCountedTwice(t *testing.T) {
	// A duplicated record would double a volume figure and add a zero-delta
	// operation to the bounds, neither of which any later stage could detect.
	f := newFakeHorizon(t)
	page := 0
	f.handler["/trades"] = func(w http.ResponseWriter, _ *http.Request) {
		page++
		rec := tradeJSON(attackToken, "2026-02-22T00:10:21Z", 266843207, 2500000, "0.0501003", "5.3475699")
		if page == 1 {
			_, _ = fmt.Fprint(w, tradesPageJSON(f.srv.URL+"/trades?cursor=2", rec))
			return
		}
		if page == 2 {
			_, _ = fmt.Fprint(w, tradesPageJSON(f.srv.URL+"/trades?cursor=3", rec))
			return
		}
		_, _ = fmt.Fprint(w, tradesPageJSON(""))
	}
	c, _ := f.client()

	got, err := c.Trades(context.Background(), testUSTRY, testUSDC, TradeQuery{})
	if err != nil {
		t.Fatalf("Trades: %v", err)
	}
	if len(got.Trades) != 1 {
		t.Errorf("want 1 trade after dedup, got %d", len(got.Trades))
	}
}
