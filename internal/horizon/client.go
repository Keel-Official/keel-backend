// Horizon HTTP client. It turns two live endpoints into one domain.Snapshot and
// keeps the bytes it read, so a recording can be reinterpreted later without
// being re-fetched.
//
// YELLOW ZONE, so the design decisions are stated rather than left to be
// inferred. Six of them:
//
//  1. NO STELLAR SDK. Horizon is read with net/http and encoding/json from the
//     standard library. The SDK would add a dependency whose import path is
//     itself a recorded trap in this package's CLAUDE.md, and this client needs
//     three read-only endpoints and no transaction building at all. Rejected
//     alternative: github.com/stellar/go-stellar-sdk/clients/horizonclient,
//     which returns its own structs that would have to be converted to
//     domain.Snapshot anyway, so it buys a dependency and saves no layer.
//
//  2. A SNAPSHOT IS NOT ATOMIC AND SAYS SO. The book and the pools are two
//     requests, so they can straddle a ledger boundary. Horizon reports the
//     ledger it served each response from in the Latest-Ledger header, both
//     values are recorded, and Atomic is false when they differ. Rejected
//     alternative: silently taking the book's ledger and calling the result a
//     snapshot at that ledger, which is the kind of claim that makes a
//     cross-validation difference impossible to explain afterwards.
//
//  3. THE RAW BYTES ARE THE EVIDENCE. Every response body is kept verbatim in
//     RawSnapshot alongside the parsed form. Parsing is a claim about what the
//     bytes mean, and this package makes one such claim that took a live
//     measurement to settle (see BidAmountUnit), so the bytes have to outlive
//     the interpretation. Rejected alternative: recording only the parsed
//     domain.Snapshot, which would bake today's reading of the order book into
//     two weeks of evidence with no way to revise it.
//
//  4. THE RATE BUDGET REFUSES, IT DOES NOT SLEEP. Public Horizon allows roughly
//     3600 requests per hour per IP. When the budget for the window is spent,
//     Get returns ErrRateBudget immediately. Rejected alternative: blocking
//     until a slot frees, which hides an exhausted budget as latency and can
//     park a recording round for the better part of an hour.
//
//  5. THE CACHE STORES BYTES, NOT STRUCTS, AND IS OFF BY DEFAULT. A cached
//     response is therefore byte-identical to a fresh one and cannot change
//     what gets recorded. The recorder wants every round fresh; `scan` over
//     fifty assets sharing one quote asset is what the TTL exists for.
//
//  6. ORDERING IS RE-ESTABLISHED HERE. domain.OrderBook documents that the
//     adapter guarantees bids descending and asks ascending, so this client
//     sorts with Price.Cmp rather than trusting Horizon's order. Cmp
//     cross-multiplies and never divides, so no precision is spent on it.
package horizon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Keel-Official/keel-backend/internal/domain"
)

// DefaultBaseURL is public mainnet Horizon. It is rate limited and needs no key.
const DefaultBaseURL = "https://horizon.stellar.org"

// BidAmountUnit names which asset the `amount` field of an order book BID is
// denominated in.
//
// THE TWO SIDES OF /order_book ARE NOT DENOMINATED IN THE SAME ASSET, and this
// was measured rather than assumed. domain.Level.Amount is defined in BASE units
// and depth is price times amount (docs/methodology/04-depth.md section 1). An
// ask matches that directly. A BID does not: Horizon inverts the bid price into
// quote-per-base but leaves `amount` as the underlying offer's selling amount,
// and a bid is an offer selling the QUOTE asset. So the bid amount is
// quote-denominated and has to be converted.
//
// The measurement, on live USTRY/USDC on 24 August 2026: every bid level's
// amount is byte-identical to the amount of the offer selling USDC and buying
// USTRY, while its price_r is that offer's price_r inverted. Every ask level's
// amount is identical to the amount of the offer selling USTRY, with its price_r
// NOT inverted. Full commands and output:
// docs/evidences/order_book_amount_units_2026-08-24.txt.
//
// The field stays configurable even though the answer is now known, for two
// reasons. It records inside every recording which reading produced it, so a
// file written today can be told apart from one written under a different
// reading. And it is the lever the conversion invariant is tested through.
type BidAmountUnit string

const (
	// BidAmountUnitQuote converts with amountBase = amount × d / n. The
	// verified default.
	BidAmountUnitQuote BidAmountUnit = "quote"
	// BidAmountUnitBase takes Horizon's amount as already base-denominated. It
	// is correct for asks and WRONG for bids on /order_book; it exists for a
	// source that reports both sides in base units, and as the second half of
	// the conversion test.
	BidAmountUnitBase BidAmountUnit = "base"
)

var (
	// ErrRateBudget is returned instead of waiting. See decision 4.
	ErrRateBudget = errors.New("horizon: request budget for this window is spent")
	// ErrPairMismatch means Horizon echoed a different pair than was requested,
	// which is a malformed request rather than a thin market.
	ErrPairMismatch = errors.New("horizon: response describes a different pair than requested")
	// ErrNoLatestLedger means a response that has to be stamped with a ledger
	// carried no Latest-Ledger header. Stamping it with a guess is worse than
	// failing, because non-negotiable rule 1 is that every output carries a real
	// LedgerSeq.
	//
	// NOT EVERY ENDPOINT SENDS THAT HEADER, and this was found by running
	// against live Horizon rather than by reading documentation. The COLLECTION
	// endpoints send it: /order_book, /liquidity_pools, /assets. The single
	// resource /ledgers/{sequence} does not, and it does not need to, because it
	// carries its own sequence in the body. So the requirement is per call, and
	// the first version of this client demanded the header everywhere and failed
	// on its first real request. Evidence:
	// docs/evidences/order_book_amount_units_2026-08-24.txt section 3.
	ErrNoLatestLedger = errors.New("horizon: response carried no Latest-Ledger header")
)

// StatusError is a non-retryable HTTP status from Horizon.
type StatusError struct {
	Status int
	URL    string
	Body   string // truncated
	// retry is Horizon's Retry-After header, unexported because it is an input
	// to the backoff and not something a caller should act on separately.
	retry string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("horizon: %s returned %d: %s", e.URL, e.Status, e.Body)
}

// Config is the whole of this client's configuration. Nothing is read from the
// environment and no value is hidden in a function body.
type Config struct {
	BaseURL    string
	HTTP       *http.Client
	MaxRetries int           // retries AFTER the first attempt
	RetryBase  time.Duration // doubled per attempt
	RetryCap   time.Duration

	// Budget requests per BudgetWindow. Public Horizon is about 3600/hour, and
	// the default leaves headroom for whatever else shares the IP.
	Budget       int
	BudgetWindow time.Duration

	// CacheTTL zero disables the cache entirely, which is what the recorder
	// wants.
	CacheTTL time.Duration

	// MaxHolderPages caps one holder reading at this many pages of 200 accounts.
	// Zero means defaultMaxHolderPages. See decision 2 in holders.go: the cap is
	// what stops one large asset from spending an entire hourly budget.
	MaxHolderPages int

	BidAmountUnit BidAmountUnit

	// Now and Sleep are injected so the tests never wait on a real clock.
	Now   func() time.Time
	Sleep func(time.Duration)
}

func (c Config) withDefaults() Config {
	if c.BaseURL == "" {
		c.BaseURL = DefaultBaseURL
	}
	c.BaseURL = strings.TrimRight(c.BaseURL, "/")
	if c.HTTP == nil {
		c.HTTP = &http.Client{Timeout: 30 * time.Second}
	}
	if c.MaxRetries == 0 {
		c.MaxRetries = 4
	}
	if c.RetryBase == 0 {
		c.RetryBase = 500 * time.Millisecond
	}
	if c.RetryCap == 0 {
		c.RetryCap = 30 * time.Second
	}
	if c.Budget == 0 {
		c.Budget = 3000
	}
	if c.BudgetWindow == 0 {
		c.BudgetWindow = time.Hour
	}
	if c.BidAmountUnit == "" {
		c.BidAmountUnit = BidAmountUnitQuote
	}
	if c.MaxHolderPages <= 0 {
		c.MaxHolderPages = defaultMaxHolderPages
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	if c.Sleep == nil {
		c.Sleep = time.Sleep
	}
	return c
}

// Client is safe for concurrent use. Nothing in this repository calls it
// concurrently today; the mutex is there because the budget and the cache are
// shared state and a caller cannot see that from the outside.
type Client struct {
	cfg Config

	mu    sync.Mutex
	spent []time.Time // request times inside the current window
	cache map[string]cacheEntry
}

type cacheEntry struct {
	body   []byte
	latest uint32
	at     time.Time
}

func NewClient(cfg Config) *Client {
	return &Client{cfg: cfg.withDefaults(), cache: map[string]cacheEntry{}}
}

// Requests reports how many requests were made inside the current budget
// window. It exists so a long run can be watched without instrumenting Horizon.
func (c *Client) Requests() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pruneBudget(c.cfg.Now())
	return len(c.spent)
}

func (c *Client) pruneBudget(now time.Time) {
	cutoff := now.Add(-c.cfg.BudgetWindow)
	keep := c.spent[:0]
	for _, t := range c.spent {
		if t.After(cutoff) {
			keep = append(keep, t)
		}
	}
	c.spent = keep
}

// ---------------------------------------------------------------- Observation

// RawSnapshot is what gets written to disk. It holds the parsed conclusions AND
// the bytes they were drawn from, which is the whole point of it.
type RawSnapshot struct {
	// The request, restated so a file can be read on its own.
	RequestedBase  string `json:"requested_base"`
	RequestedQuote string `json:"requested_quote"`

	FetchedAt time.Time `json:"fetched_at"`

	// BookLedger and PoolLedger are Horizon's Latest-Ledger for each response.
	// Atomic is false when they differ, which means the two halves of this
	// snapshot describe two different ledgers. See decision 2.
	BookLedger uint32 `json:"book_latest_ledger"`
	PoolLedger uint32 `json:"pool_latest_ledger"`
	Atomic     bool   `json:"atomic"`

	LedgerSeq      uint32    `json:"ledger_seq"`
	LedgerClosedAt time.Time `json:"ledger_closed_at"`

	// BidAmountUnit records which reading of the bid amount produced the parsed
	// snapshot, so a file recorded today can be told apart from one recorded
	// after the question is settled.
	BidAmountUnit      string `json:"bid_amount_unit"`
	MethodologyVersion string `json:"methodology_version"`

	OrderBook      json.RawMessage `json:"order_book"`
	LiquidityPools json.RawMessage `json:"liquidity_pools"`
	Ledger         json.RawMessage `json:"ledger"`
}

// Observation is one snapshot plus the evidence for it.
type Observation struct {
	Snapshot domain.Snapshot
	Raw      RawSnapshot
}

// ---------------------------------------------------------------- Endpoints

// GetSnapshot reads the order book, the pools, and the ledger those were served
// from, and assembles one domain.Snapshot. Three requests against the budget.
func (c *Client) GetSnapshot(ctx context.Context, base, quote domain.Asset) (Observation, error) {
	var obs Observation

	bookQ := url.Values{}
	addAsset(bookQ, "selling", base)
	addAsset(bookQ, "buying", quote)
	bookQ.Set("limit", strconv.Itoa(bookPageLimit))

	bookBody, bookLedger, err := c.get(ctx, "/order_book", bookQ, true)
	if err != nil {
		return obs, fmt.Errorf("order book %s/%s: %w", base, quote, err)
	}

	var book orderBookResponse
	if err := json.Unmarshal(bookBody, &book); err != nil {
		return obs, fmt.Errorf("decode order book %s/%s: %w", base, quote, err)
	}
	// Horizon answers a request naming the wrong asset type with an EMPTY book
	// and no error, so the echo is checked. An empty book that echoes the right
	// pair is a legitimate answer and the most interesting one this product has.
	if !book.Base.matches(base) || !book.Counter.matches(quote) {
		return obs, fmt.Errorf("%w: asked %s/%s, got %s/%s",
			ErrPairMismatch, base, quote, book.Base.describe(), book.Counter.describe())
	}

	poolQ := url.Values{}
	poolQ.Set("reserves", horizonAsset(base)+","+horizonAsset(quote))
	poolQ.Set("limit", strconv.Itoa(poolPageLimit))
	poolBody, poolLedger, err := c.get(ctx, "/liquidity_pools", poolQ, true)
	if err != nil {
		return obs, fmt.Errorf("liquidity pools %s/%s: %w", base, quote, err)
	}

	var pools poolsResponse
	if err := json.Unmarshal(poolBody, &pools); err != nil {
		return obs, fmt.Errorf("decode liquidity pools %s/%s: %w", base, quote, err)
	}

	ledgerBody, _, err := c.get(ctx, "/ledgers/"+strconv.FormatUint(uint64(bookLedger), 10), nil, false)
	if err != nil {
		return obs, fmt.Errorf("ledger %d: %w", bookLedger, err)
	}
	var ledger ledgerResponse
	if err := json.Unmarshal(ledgerBody, &ledger); err != nil {
		return obs, fmt.Errorf("decode ledger %d: %w", bookLedger, err)
	}
	if ledger.Sequence != bookLedger {
		return obs, fmt.Errorf("ledger %d returned sequence %d", bookLedger, ledger.Sequence)
	}

	bids, err := c.levels(book.Bids, sideBid)
	if err != nil {
		return obs, fmt.Errorf("bids %s/%s: %w", base, quote, err)
	}
	asks, err := c.levels(book.Asks, sideAsk)
	if err != nil {
		return obs, fmt.Errorf("asks %s/%s: %w", base, quote, err)
	}
	sort.SliceStable(bids, func(i, j int) bool { return bids[i].Price.Cmp(bids[j].Price) > 0 })
	sort.SliceStable(asks, func(i, j int) bool { return asks[i].Price.Cmp(asks[j].Price) < 0 })

	reserves, err := poolReserves(pools, base, quote)
	if err != nil {
		return obs, fmt.Errorf("pools %s/%s: %w", base, quote, err)
	}

	obs.Snapshot = domain.Snapshot{
		Base:           base,
		Quote:          quote,
		LedgerSeq:      bookLedger,
		LedgerClosedAt: ledger.ClosedAt,
		Book:           domain.OrderBook{Bids: bids, Asks: asks},
		Pools:          reserves,
		Source:         domain.DataSourceHorizon,
	}
	obs.Raw = RawSnapshot{
		RequestedBase:      base.String(),
		RequestedQuote:     quote.String(),
		FetchedAt:          c.cfg.Now().UTC(),
		BookLedger:         bookLedger,
		PoolLedger:         poolLedger,
		Atomic:             bookLedger == poolLedger,
		LedgerSeq:          bookLedger,
		LedgerClosedAt:     ledger.ClosedAt,
		BidAmountUnit:      string(c.cfg.BidAmountUnit),
		MethodologyVersion: domain.MethodologyVersion,
		OrderBook:          json.RawMessage(bookBody),
		LiquidityPools:     json.RawMessage(poolBody),
		Ledger:             json.RawMessage(ledgerBody),
	}
	return obs, nil
}

// VerifyAsset confirms an asset exists on Horizon with the code, issuer, AND
// type it was declared with.
//
// It is separate from GetSnapshot and meant to be called ONCE per asset at
// startup rather than per snapshot. The trap it closes is that a wrong
// AssetType returns an empty order book with no error, which is
// indistinguishable from a dead market, and paying a request every thirty
// minutes to re-answer a question that cannot change is how a rate limit budget
// gets spent on nothing.
func (c *Client) VerifyAsset(ctx context.Context, a domain.Asset) error {
	if a.IsNative() {
		return nil
	}
	q := url.Values{}
	q.Set("asset_code", a.Code)
	q.Set("asset_issuer", a.Issuer)
	body, _, err := c.get(ctx, "/assets", q, false)
	if err != nil {
		return fmt.Errorf("verify %s: %w", a, err)
	}
	var res assetsResponse
	if err := json.Unmarshal(body, &res); err != nil {
		return fmt.Errorf("verify %s: decode: %w", a, err)
	}
	for _, r := range res.Embedded.Records {
		if r.AssetCode == a.Code && r.AssetIssuer == a.Issuer {
			if r.AssetType != string(a.Type) {
				return fmt.Errorf("verify %s: declared %s, Horizon says %s", a, a.Type, r.AssetType)
			}
			return nil
		}
	}
	return fmt.Errorf("verify %s: no such asset on Horizon", a)
}

// ---------------------------------------------------------------- Transport

// get fetches one endpoint. requireLatest says whether the Latest-Ledger header
// is mandatory for this call; see ErrNoLatestLedger for why that is not a
// property of the client but of the endpoint.
func (c *Client) get(ctx context.Context, path string, q url.Values, requireLatest bool) ([]byte, uint32, error) {
	full := c.cfg.BaseURL + path
	if len(q) > 0 {
		full += "?" + q.Encode()
	}

	if body, latest, ok := c.cached(full); ok {
		return body, latest, nil
	}

	var lastErr error
	for attempt := 0; attempt <= c.cfg.MaxRetries; attempt++ {
		if attempt > 0 {
			c.cfg.Sleep(c.backoff(attempt, lastErr))
		}
		if err := c.spend(); err != nil {
			return nil, 0, err
		}

		body, latest, err := c.attempt(ctx, full, requireLatest)
		if err == nil {
			c.store(full, body, latest)
			return body, latest, nil
		}
		if ctx.Err() != nil {
			return nil, 0, ctx.Err()
		}
		if !retryable(err) {
			return nil, 0, err
		}
		lastErr = err
	}
	return nil, 0, fmt.Errorf("after %d attempts: %w", c.cfg.MaxRetries+1, lastErr)
}

func (c *Client) attempt(ctx context.Context, full string, requireLatest bool) ([]byte, uint32, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.cfg.HTTP.Do(req)
	if err != nil {
		return nil, 0, &transportError{err: err}
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, 0, &transportError{err: err}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, 0, &StatusError{
			Status: resp.StatusCode,
			URL:    full,
			Body:   truncate(string(body), 400),
			retry:  resp.Header.Get("Retry-After"),
		}
	}

	// Horizon stamps its collection responses with the ledger they were served
	// from. It is the only ledger sequence /order_book carries, so without it
	// there is no honest way to label the snapshot.
	raw := resp.Header.Get("Latest-Ledger")
	if raw == "" {
		if requireLatest {
			return nil, 0, fmt.Errorf("%w: %s", ErrNoLatestLedger, full)
		}
		return body, 0, nil
	}
	latest, err := strconv.ParseUint(raw, 10, 32)
	if err != nil {
		return nil, 0, fmt.Errorf("Latest-Ledger %q on %s: %w", raw, full, err)
	}
	return body, uint32(latest), nil
}

// backoff doubles per attempt and is capped. There is no jitter, deliberately:
// jitter needs math/rand, a single-process recorder has no herd to spread, and a
// deterministic delay is one less reason for a test to be flaky. Horizon's own
// Retry-After wins when it sends one, because the server knows better than the
// schedule.
func (c *Client) backoff(attempt int, lastErr error) time.Duration {
	var se *StatusError
	if errors.As(lastErr, &se) && se.retry != "" {
		if secs, err := strconv.Atoi(se.retry); err == nil && secs >= 0 {
			d := time.Duration(secs) * time.Second
			if d > c.cfg.RetryCap {
				d = c.cfg.RetryCap
			}
			return d
		}
	}
	d := c.cfg.RetryBase * time.Duration(int64(1)<<uint(attempt-1))
	if d > c.cfg.RetryCap {
		d = c.cfg.RetryCap
	}
	return d
}

func (c *Client) spend() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.cfg.Now()
	c.pruneBudget(now)
	if len(c.spent) >= c.cfg.Budget {
		return fmt.Errorf("%w: %d requests in the last %s", ErrRateBudget, len(c.spent), c.cfg.BudgetWindow)
	}
	c.spent = append(c.spent, now)
	return nil
}

func (c *Client) cached(key string) ([]byte, uint32, bool) {
	if c.cfg.CacheTTL <= 0 {
		return nil, 0, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.cache[key]
	if !ok || c.cfg.Now().Sub(e.at) > c.cfg.CacheTTL {
		return nil, 0, false
	}
	return e.body, e.latest, true
}

func (c *Client) store(key string, body []byte, latest uint32) {
	if c.cfg.CacheTTL <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[key] = cacheEntry{body: body, latest: latest, at: c.cfg.Now()}
}

type transportError struct{ err error }

func (e *transportError) Error() string { return "horizon: transport: " + e.err.Error() }
func (e *transportError) Unwrap() error { return e.err }

func retryable(err error) bool {
	var te *transportError
	if errors.As(err, &te) {
		return true
	}
	var se *StatusError
	if errors.As(err, &se) {
		return se.Status == http.StatusTooManyRequests || se.Status >= 500
	}
	return false
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
