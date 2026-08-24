package horizon

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/Keel-Official/keel-backend/internal/domain"
)

// The pair and the numbers below are the real USTRY/USDC market, taken from
// testdata/fixtures/ustry_pre_exploit.md and docs/evidences/reserves_pool.txt
// rather than invented. These tests assert what the ADAPTER does with those
// bytes. They assert nothing about whether the fixture's own values are right;
// that is what internal/conformance is for.
var (
	testUSTRY = domain.Asset{
		Code:   "USTRY",
		Issuer: "GCRYUGD5NVARGXT56XEZI5CIFCQETYHAPQQTHO2O3IQZTHDH4LATMYWC",
		Type:   domain.AssetTypeAlphanum12,
	}
	testUSDC = domain.Asset{
		Code:   "USDC",
		Issuer: "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN",
		Type:   domain.AssetTypeAlphanum4,
	}
)

const (
	testLedger   = 61340263
	testClosedAt = "2026-02-22T00:10:21Z"
)

// bookBody is /order_book with one ask and one bid. price_r arrives as JSON
// NUMBERS here, which is the shape that endpoint really uses.
func bookBody(askAmount, bidAmount string) string {
	return fmt.Sprintf(`{
	  "base":    {"asset_type":"credit_alphanum12","asset_code":"USTRY","asset_issuer":%q},
	  "counter": {"asset_type":"credit_alphanum4","asset_code":"USDC","asset_issuer":%q},
	  "asks": [{"price_r":{"n":266843207,"d":2500000},"price":"106.7372828","amount":%q}],
	  "bids": [{"price_r":{"n":1057,"d":1000},"price":"1.0570000","amount":%q}]
	}`, testUSTRY.Issuer, testUSDC.Issuer, askAmount, bidAmount)
}

const poolBody = `{"_embedded":{"records":[{
	  "id":"27480d0483c8320ba4a707797526ffd67118e841491e0cbeb66db697bb66cccb",
	  "fee_bp":30,
	  "total_shares":"15.8497241",
	  "reserves":[
	    {"asset":"USDC:GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN","amount":"16.5589417"},
	    {"asset":"USTRY:GCRYUGD5NVARGXT56XEZI5CIFCQETYHAPQQTHO2O3IQZTHDH4LATMYWC","amount":"15.4742476"}
	  ],
	  "last_modified_ledger":64063312
	}]}}`

const ledgerBody = `{"sequence":61340263,"closed_at":"2026-02-22T00:10:21Z","hash":"abc"}`

// fakeHorizon is a Horizon stand-in. Every handler is a function so a test can
// swap one out, and every request is counted so a test can prove a cache hit
// never reached the network.
type fakeHorizon struct {
	t       *testing.T
	hits    map[string]int
	ledger  map[string]string // path -> Latest-Ledger header, empty means omit
	handler map[string]func(w http.ResponseWriter, r *http.Request)
	srv     *httptest.Server
}

func newFakeHorizon(t *testing.T) *fakeHorizon {
	t.Helper()
	f := &fakeHorizon{
		t:      t,
		hits:   map[string]int{},
		ledger: map[string]string{},
		handler: map[string]func(http.ResponseWriter, *http.Request){
			"/order_book":      func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, bookBody("1.2185312", "0.0001000")) },
			"/liquidity_pools": func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, poolBody) },
		},
	}
	f.handler["/ledgers/61340263"] = func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, ledgerBody) }

	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.hits[r.URL.Path]++
		h, ok := f.handler[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintf(w, `{"title":"not found","path":%q}`, r.URL.Path)
			return
		}
		latest := "61340263"
		if v, set := f.ledger[r.URL.Path]; set {
			latest = v
		}
		if latest != "" {
			w.Header().Set("Latest-Ledger", latest)
		}
		w.Header().Set("Content-Type", "application/json")
		h(w, r)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

// client returns a client pointed at the fake, with the clock and the sleep
// frozen so no test ever waits.
func (f *fakeHorizon) client(mut ...func(*Config)) (*Client, *[]time.Duration) {
	slept := &[]time.Duration{}
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	cfg := Config{
		BaseURL:   f.srv.URL,
		RetryBase: time.Second,
		Now:       func() time.Time { return now },
		Sleep:     func(d time.Duration) { *slept = append(*slept, d) },
	}
	for _, m := range mut {
		m(&cfg)
	}
	return NewClient(cfg), slept
}

func TestGetSnapshotReadsBookPoolsAndLedger(t *testing.T) {
	f := newFakeHorizon(t)
	c, _ := f.client()

	obs, err := c.GetSnapshot(context.Background(), testUSTRY, testUSDC)
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}
	s := obs.Snapshot

	if s.LedgerSeq != testLedger {
		t.Errorf("LedgerSeq = %d, want %d (from the Latest-Ledger header)", s.LedgerSeq, testLedger)
	}
	want, _ := time.Parse(time.RFC3339, testClosedAt)
	if !s.LedgerClosedAt.Equal(want) {
		t.Errorf("LedgerClosedAt = %s, want %s", s.LedgerClosedAt, want)
	}
	if s.Source != domain.DataSourceHorizon {
		t.Errorf("Source = %q, want %q", s.Source, domain.DataSourceHorizon)
	}
	if !s.Base.Equal(testUSTRY) || !s.Quote.Equal(testUSDC) {
		t.Errorf("pair = %s/%s, want %s/%s", s.Base, s.Quote, testUSTRY, testUSDC)
	}

	// The price must come from price_r exactly. 266843207/2500000 is the ask
	// that carried the February manipulation, and the whole point of reading
	// the fraction is that this stays exact.
	if len(s.Book.Asks) != 1 {
		t.Fatalf("got %d asks, want 1", len(s.Book.Asks))
	}
	if got := s.Book.Asks[0].Price; got.N != 266843207 || got.D != 2500000 {
		t.Errorf("ask price = %s, want 266843207/2500000", got)
	}
	if got := s.Book.Asks[0].Amount.String(); got != "1.2185312" {
		t.Errorf("ask amount = %s, want 1.2185312 (asks are base-denominated)", got)
	}

	// One pool, with base and quote assigned by asset identity and NOT by the
	// order they arrive in. The evidence file lists USDC first and USTRY
	// second, which is the reverse of base/quote here, so an implementation
	// reading them positionally passes nothing.
	if len(s.Pools) != 1 {
		t.Fatalf("got %d pools, want 1", len(s.Pools))
	}
	p := s.Pools[0]
	if p.ReserveBase.String() != "15.4742476" {
		t.Errorf("ReserveBase = %s, want the USTRY reserve 15.4742476", p.ReserveBase)
	}
	if p.ReserveQuote.String() != "16.5589417" {
		t.Errorf("ReserveQuote = %s, want the USDC reserve 16.5589417", p.ReserveQuote)
	}
	if p.FeeBP != 30 {
		t.Errorf("FeeBP = %d, want 30 read from the response", p.FeeBP)
	}

	if !obs.Raw.Atomic {
		t.Error("Atomic = false, want true when both halves report the same ledger")
	}
	if !strings.Contains(string(obs.Raw.OrderBook), "266843207") {
		t.Error("the raw order book body was not kept verbatim")
	}
	if obs.Raw.BidAmountUnit != string(BidAmountUnitQuote) {
		t.Errorf("BidAmountUnit = %q, want the default recorded in the file", obs.Raw.BidAmountUnit)
	}
	if obs.Raw.MethodologyVersion != domain.MethodologyVersion {
		t.Errorf("MethodologyVersion = %q, want %q", obs.Raw.MethodologyVersion, domain.MethodologyVersion)
	}
}

// The adapter guarantees the ordering, so it must not inherit Horizon's.
func TestGetSnapshotSortsBothSides(t *testing.T) {
	f := newFakeHorizon(t)
	f.handler["/order_book"] = func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{
		  "base":    {"asset_type":"credit_alphanum12","asset_code":"USTRY","asset_issuer":%q},
		  "counter": {"asset_type":"credit_alphanum4","asset_code":"USDC","asset_issuer":%q},
		  "asks": [{"price_r":{"n":3,"d":1},"amount":"1"},
		           {"price_r":{"n":2,"d":1},"amount":"1"}],
		  "bids": [{"price_r":{"n":1,"d":1},"amount":"1"},
		           {"price_r":{"n":3,"d":2},"amount":"1"}]
		}`, testUSTRY.Issuer, testUSDC.Issuer)
	}
	c, _ := f.client(func(cfg *Config) { cfg.BidAmountUnit = BidAmountUnitBase })

	obs, err := c.GetSnapshot(context.Background(), testUSTRY, testUSDC)
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}
	if got, _ := obs.Snapshot.Book.BestAsk(); got.Price.Cmp(domain.Price{N: 2, D: 1}) != 0 {
		t.Errorf("best ask = %s, want the LOWEST ask 2/1", got.Price)
	}
	if got, _ := obs.Snapshot.Book.BestBid(); got.Price.Cmp(domain.Price{N: 3, D: 2}) != 0 {
		t.Errorf("best bid = %s, want the HIGHEST bid 3/2", got.Price)
	}
}

// The two readings of a bid amount must differ in Amount and agree on the quote
// value, which is the invariant that makes the open question survivable.
func TestBidAmountUnitConversion(t *testing.T) {
	f := newFakeHorizon(t)
	f.handler["/order_book"] = func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, bookBody("1", "2.0000000"))
	}

	asQuote, _ := f.client()
	asBase, _ := f.client(func(cfg *Config) { cfg.BidAmountUnit = BidAmountUnitBase })

	q, err := asQuote.GetSnapshot(context.Background(), testUSTRY, testUSDC)
	if err != nil {
		t.Fatalf("quote reading: %v", err)
	}
	b, err := asBase.GetSnapshot(context.Background(), testUSTRY, testUSDC)
	if err != nil {
		t.Fatalf("base reading: %v", err)
	}

	if b.Snapshot.Book.Bids[0].Amount.String() != "2" {
		t.Errorf("base reading amount = %s, want Horizon's 2 unchanged", b.Snapshot.Book.Bids[0].Amount)
	}
	// 2 quote at 1057/1000 is 2 × 1000 / 1057 base.
	wantBase := decimal.NewFromInt(2000).Div(decimal.NewFromInt(1057))
	if got := q.Snapshot.Book.Bids[0].Amount; got.Sub(wantBase).Abs().GreaterThan(decimal.RequireFromString("0.0000000001")) {
		t.Errorf("quote reading amount = %s, want about %s", got, wantBase)
	}
	// And the notional comes back to the quote amount Horizon sent.
	if got := q.Snapshot.Book.Bids[0].Notional(); got.Sub(decimal.NewFromInt(2)).Abs().GreaterThan(decimal.RequireFromString("0.0000000001")) {
		t.Errorf("notional = %s, want 2, the quote amount Horizon sent", got)
	}
}

// A ledger straddle is recorded, not hidden and not fatal.
func TestGetSnapshotRecordsLedgerStraddle(t *testing.T) {
	f := newFakeHorizon(t)
	f.ledger["/liquidity_pools"] = "61340270"
	c, _ := f.client()

	obs, err := c.GetSnapshot(context.Background(), testUSTRY, testUSDC)
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}
	if obs.Raw.Atomic {
		t.Error("Atomic = true although the two halves came from different ledgers")
	}
	if obs.Raw.BookLedger != 61340263 || obs.Raw.PoolLedger != 61340270 {
		t.Errorf("ledgers recorded as book=%d pool=%d, want 61340263 and 61340270",
			obs.Raw.BookLedger, obs.Raw.PoolLedger)
	}
	if obs.Snapshot.LedgerSeq != 61340263 {
		t.Errorf("LedgerSeq = %d, want the book's ledger", obs.Snapshot.LedgerSeq)
	}
}

// The trap this closes: a wrong asset type returns an empty book and no error.
func TestGetSnapshotRejectsPairMismatch(t *testing.T) {
	f := newFakeHorizon(t)
	f.handler["/order_book"] = func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{
		  "base":    {"asset_type":"credit_alphanum4","asset_code":"USTRY","asset_issuer":%q},
		  "counter": {"asset_type":"credit_alphanum4","asset_code":"USDC","asset_issuer":%q},
		  "asks": [], "bids": []
		}`, testUSTRY.Issuer, testUSDC.Issuer)
	}
	c, _ := f.client()

	if _, err := c.GetSnapshot(context.Background(), testUSTRY, testUSDC); !errors.Is(err, ErrPairMismatch) {
		t.Fatalf("error = %v, want ErrPairMismatch when the echoed asset type differs", err)
	}
}

// An empty book that echoes the right pair is a legitimate answer, and for a
// thin asset it is the most interesting one there is.
func TestGetSnapshotAcceptsAnEmptyBook(t *testing.T) {
	f := newFakeHorizon(t)
	f.handler["/order_book"] = func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{
		  "base":    {"asset_type":"credit_alphanum12","asset_code":"USTRY","asset_issuer":%q},
		  "counter": {"asset_type":"credit_alphanum4","asset_code":"USDC","asset_issuer":%q},
		  "asks": [], "bids": []
		}`, testUSTRY.Issuer, testUSDC.Issuer)
	}
	c, _ := f.client()

	obs, err := c.GetSnapshot(context.Background(), testUSTRY, testUSDC)
	if err != nil {
		t.Fatalf("an empty book must not be an error: %v", err)
	}
	if len(obs.Snapshot.Book.Asks) != 0 || len(obs.Snapshot.Book.Bids) != 0 {
		t.Error("levels appeared out of an empty book")
	}
}

func TestGetSnapshotRequiresLatestLedger(t *testing.T) {
	f := newFakeHorizon(t)
	f.ledger["/order_book"] = "" // omit the header
	c, _ := f.client()

	if _, err := c.GetSnapshot(context.Background(), testUSTRY, testUSDC); !errors.Is(err, ErrNoLatestLedger) {
		t.Fatalf("error = %v, want ErrNoLatestLedger; a guessed ledger is worse than a failure", err)
	}
}

func TestRetriesOn429AndHonoursRetryAfter(t *testing.T) {
	f := newFakeHorizon(t)
	attempts := 0
	f.handler["/order_book"] = func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts < 3 {
			w.Header().Set("Retry-After", "7")
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprint(w, `{"title":"rate limit exceeded"}`)
			return
		}
		fmt.Fprint(w, bookBody("1.2185312", "0.0001000"))
	}
	c, slept := f.client()

	if _, err := c.GetSnapshot(context.Background(), testUSTRY, testUSDC); err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3", attempts)
	}
	if len(*slept) != 2 {
		t.Fatalf("slept %d times, want 2", len(*slept))
	}
	for i, d := range *slept {
		if d != 7*time.Second {
			t.Errorf("backoff %d = %s, want Horizon's Retry-After of 7s", i, d)
		}
	}
	// Every attempt, including the refused ones, counts against the budget.
	if got := c.Requests(); got != 5 {
		t.Errorf("Requests() = %d, want 5 (3 book attempts, 1 pool, 1 ledger)", got)
	}
}

func TestDoesNotRetryA404(t *testing.T) {
	f := newFakeHorizon(t)
	delete(f.handler, "/order_book")
	c, slept := f.client()

	_, err := c.GetSnapshot(context.Background(), testUSTRY, testUSDC)
	var se *StatusError
	if !errors.As(err, &se) || se.Status != http.StatusNotFound {
		t.Fatalf("error = %v, want a StatusError carrying 404", err)
	}
	if len(*slept) != 0 {
		t.Errorf("slept %d times on a 404; a client error is not retryable", len(*slept))
	}
}

func TestRateBudgetRefusesRatherThanWaits(t *testing.T) {
	f := newFakeHorizon(t)
	c, slept := f.client(func(cfg *Config) { cfg.Budget = 2 })

	_, err := c.GetSnapshot(context.Background(), testUSTRY, testUSDC)
	if !errors.Is(err, ErrRateBudget) {
		t.Fatalf("error = %v, want ErrRateBudget on the third request of a budget of 2", err)
	}
	if len(*slept) != 0 {
		t.Errorf("slept %d times; an exhausted budget must refuse, not wait", len(*slept))
	}
}

func TestCacheServesTheSameBytesAndSkipsTheNetwork(t *testing.T) {
	f := newFakeHorizon(t)
	c, _ := f.client(func(cfg *Config) { cfg.CacheTTL = time.Minute })

	first, err := c.GetSnapshot(context.Background(), testUSTRY, testUSDC)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := c.GetSnapshot(context.Background(), testUSTRY, testUSDC)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if f.hits["/order_book"] != 1 {
		t.Errorf("order book fetched %d times, want 1 inside the TTL", f.hits["/order_book"])
	}
	if string(first.Raw.OrderBook) != string(second.Raw.OrderBook) {
		t.Error("a cached response differs byte for byte from the fresh one")
	}
}

func TestCacheIsOffByDefault(t *testing.T) {
	f := newFakeHorizon(t)
	c, _ := f.client()

	for i := 0; i < 2; i++ {
		if _, err := c.GetSnapshot(context.Background(), testUSTRY, testUSDC); err != nil {
			t.Fatalf("round %d: %v", i, err)
		}
	}
	if f.hits["/order_book"] != 2 {
		t.Errorf("order book fetched %d times, want 2; the recorder needs every round fresh", f.hits["/order_book"])
	}
}

func TestVerifyAssetCatchesTheWrongType(t *testing.T) {
	f := newFakeHorizon(t)
	f.handler["/assets"] = func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"_embedded":{"records":[
		  {"asset_type":"credit_alphanum12","asset_code":"USTRY","asset_issuer":%q}]}}`, testUSTRY.Issuer)
	}
	c, _ := f.client()

	wrong := testUSTRY
	wrong.Type = domain.AssetTypeAlphanum4
	err := c.VerifyAsset(context.Background(), wrong)
	if err == nil {
		t.Fatal("VerifyAsset accepted alphanum4 for a five character code")
	}
	if !strings.Contains(err.Error(), "credit_alphanum12") {
		t.Errorf("error %q does not name the type Horizon reported", err)
	}
	if err := c.VerifyAsset(context.Background(), testUSTRY); err != nil {
		t.Errorf("VerifyAsset rejected the correct declaration: %v", err)
	}
}

func TestVerifyAssetSkipsNative(t *testing.T) {
	f := newFakeHorizon(t)
	c, _ := f.client()

	if err := c.VerifyAsset(context.Background(), domain.Asset{Type: domain.AssetTypeNative}); err != nil {
		t.Fatalf("native needs no verification: %v", err)
	}
	if c.Requests() != 0 {
		t.Errorf("Requests() = %d, want 0; XLM identity needs no request", c.Requests())
	}
}

// A pool record missing one side of the requested pair is an error rather than a
// skip, because dropping AMM liquidity out of a depth figure silently is the
// failure this whole product exists to prevent.
func TestPoolMissingRequestedAssetIsAnError(t *testing.T) {
	f := newFakeHorizon(t)
	f.handler["/liquidity_pools"] = func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"_embedded":{"records":[{"id":"deadbeef","fee_bp":30,"reserves":[
		  {"asset":"native","amount":"1"},
		  {"asset":"USDC:GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN","amount":"2"}]}]}}`)
	}
	c, _ := f.client()

	_, err := c.GetSnapshot(context.Background(), testUSTRY, testUSDC)
	if err == nil || !strings.Contains(err.Error(), "does not hold both") {
		t.Fatalf("error = %v, want a complaint that the pool lacks the requested pair", err)
	}
}

// A zero or negative amount must not flow onward. price.go makes the same
// argument for a zero price: it is indistinguishable from a genuinely worthless
// market, and telling those apart is the product.
func TestNonPositiveAmountIsRejected(t *testing.T) {
	f := newFakeHorizon(t)
	f.handler["/order_book"] = func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, bookBody("0", "0.0001000"))
	}
	c, _ := f.client()

	if _, err := c.GetSnapshot(context.Background(), testUSTRY, testUSDC); err == nil {
		t.Fatal("an ask with amount 0 was accepted")
	}
}

// The single ledger resource does not send Latest-Ledger and does not need to,
// because it carries its own sequence. The first version of this client demanded
// the header everywhere and failed on its first live request, so this is a
// regression test for a real failure rather than a hypothetical one. See section
// 3 of docs/evidences/order_book_amount_units_2026-08-24.txt.
func TestLedgerResourceNeedsNoLatestLedgerHeader(t *testing.T) {
	f := newFakeHorizon(t)
	f.ledger["/ledgers/61340263"] = "" // omit it, as Horizon really does
	c, _ := f.client()

	obs, err := c.GetSnapshot(context.Background(), testUSTRY, testUSDC)
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}
	if obs.Snapshot.LedgerSeq != testLedger {
		t.Errorf("LedgerSeq = %d, want %d", obs.Snapshot.LedgerSeq, testLedger)
	}
	want, _ := time.Parse(time.RFC3339, testClosedAt)
	if !obs.Snapshot.LedgerClosedAt.Equal(want) {
		t.Errorf("LedgerClosedAt = %s, want %s", obs.Snapshot.LedgerClosedAt, want)
	}
}

// /assets does send the header, but VerifyAsset does not depend on it, so a
// missing one must not turn an identity check into a failure.
func TestVerifyAssetNeedsNoLatestLedgerHeader(t *testing.T) {
	f := newFakeHorizon(t)
	f.ledger["/assets"] = ""
	f.handler["/assets"] = func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"_embedded":{"records":[
		  {"asset_type":"credit_alphanum12","asset_code":"USTRY","asset_issuer":%q}]}}`, testUSTRY.Issuer)
	}
	c, _ := f.client()

	if err := c.VerifyAsset(context.Background(), testUSTRY); err != nil {
		t.Fatalf("VerifyAsset: %v", err)
	}
}

// The ledger body has to describe the ledger that was asked for. A mismatch
// means the two halves of the stamp disagree, and a snapshot carrying one
// ledger's sequence with another's close time is worse than no snapshot.
func TestLedgerSequenceMismatchIsRejected(t *testing.T) {
	f := newFakeHorizon(t)
	f.handler["/ledgers/61340263"] = func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"sequence":61340999,"closed_at":"2026-02-22T00:10:21Z"}`)
	}
	c, _ := f.client()

	_, err := c.GetSnapshot(context.Background(), testUSTRY, testUSDC)
	if err == nil || !strings.Contains(err.Error(), "returned sequence") {
		t.Fatalf("error = %v, want a complaint that the ledger body names another ledger", err)
	}
}

// A full page of pools means there may be more behind it. Treating it as the
// whole answer would drop AMM liquidity out of a depth figure silently, which is
// the one failure mode this product exists to prevent.
func TestAFullPageOfPoolsIsRefused(t *testing.T) {
	f := newFakeHorizon(t)
	f.handler["/liquidity_pools"] = func(w http.ResponseWriter, _ *http.Request) {
		var b strings.Builder
		b.WriteString(`{"_embedded":{"records":[`)
		for i := 0; i < poolPageLimit; i++ {
			if i > 0 {
				b.WriteString(",")
			}
			fmt.Fprintf(&b, `{"id":"pool%d","fee_bp":30,"reserves":[
			  {"asset":"USTRY:%s","amount":"1"},
			  {"asset":"USDC:%s","amount":"1"}]}`, i, testUSTRY.Issuer, testUSDC.Issuer)
		}
		b.WriteString(`]}}`)
		fmt.Fprint(w, b.String())
	}
	c, _ := f.client()

	_, err := c.GetSnapshot(context.Background(), testUSTRY, testUSDC)
	if err == nil || !strings.Contains(err.Error(), "fills the page") {
		t.Fatalf("error = %v, want a refusal naming the full page", err)
	}
}

// One pool short of the page is a normal answer, so the guard must not fire on
// a legitimate result.
func TestManyPoolsUnderTheLimitAreAccepted(t *testing.T) {
	f := newFakeHorizon(t)
	f.handler["/liquidity_pools"] = func(w http.ResponseWriter, _ *http.Request) {
		var b strings.Builder
		b.WriteString(`{"_embedded":{"records":[`)
		for i := 0; i < poolPageLimit-1; i++ {
			if i > 0 {
				b.WriteString(",")
			}
			fmt.Fprintf(&b, `{"id":"pool%d","fee_bp":%d,"reserves":[
			  {"asset":"USTRY:%s","amount":"1"},
			  {"asset":"USDC:%s","amount":"2"}]}`, i, 30+i, testUSTRY.Issuer, testUSDC.Issuer)
		}
		b.WriteString(`]}}`)
		fmt.Fprint(w, b.String())
	}
	c, _ := f.client()

	obs, err := c.GetSnapshot(context.Background(), testUSTRY, testUSDC)
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}
	if len(obs.Snapshot.Pools) != poolPageLimit-1 {
		t.Errorf("got %d pools, want %d", len(obs.Snapshot.Pools), poolPageLimit-1)
	}
	// The fee is read per pool and never assumed to be 30.
	if obs.Snapshot.Pools[5].FeeBP != 35 {
		t.Errorf("pool 5 FeeBP = %d, want 35 read from its own record", obs.Snapshot.Pools[5].FeeBP)
	}
}
