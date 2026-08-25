package horizon

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Keel-Official/keel-backend/internal/domain"
)

// assetsFixture serves /assets as Horizon does: paged, with a next link on every
// page INCLUDING the last, and an empty final page.
type assetsFixture struct {
	t     *testing.T
	pages [][]string // each entry is a rendered record
	hits  int
	srv   *httptest.Server
	// status429 is served this many times before any real answer, to exercise
	// the throttle counter without waiting on a real backoff.
	status429 int
}

func record(code, issuer, typ string, trustlines, pools int) string {
	return fmt.Sprintf(`{
      "_links": {"toml": {"href": "https://%s/.well-known/stellar.toml"}},
      "asset_type": %q,
      "asset_code": %q,
      "asset_issuer": %q,
      "paging_token": "%s_%s_%s",
      "num_claimable_balances": 1,
      "num_liquidity_pools": %d,
      "num_contracts": 2,
      "accounts": {"authorized": %d, "authorized_to_maintain_liabilities": 0, "unauthorized": 3},
      "claimable_balances_amount": "1.2500000",
      "liquidity_pools_amount": "9.9900000",
      "contracts_amount": "0.0000000",
      "balances": {"authorized": "1000.0000000", "authorized_to_maintain_liabilities": "0.0000000", "unauthorized": "7.0000000"},
      "flags": {"auth_required": false, "auth_revocable": true, "auth_immutable": false, "auth_clawback_enabled": false}
    }`, strings.ToLower(code)+".example", typ, code, issuer, code, issuer, typ, pools, trustlines)
}

func newAssetsFixture(t *testing.T, pages [][]string) *assetsFixture {
	t.Helper()
	f := &assetsFixture{t: t, pages: pages}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if f.status429 > 0 {
			f.status429--
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"status":429}`))
			return
		}
		f.hits++
		page := 0
		if c := r.URL.Query().Get("cursor"); c != "" {
			_, _ = fmt.Sscanf(c, "page%d", &page)
		}
		var recs []string
		if page < len(f.pages) {
			recs = f.pages[page]
		}
		w.Header().Set("Latest-Ledger", "64116131")
		w.Header().Set("Content-Type", "application/json")
		// The next link is ALWAYS present, which is what Horizon does and the
		// reason a walk must stop on an empty page rather than on a missing link.
		next := fmt.Sprintf("%s/assets?asset_code=%s&cursor=page%d&limit=200&order=asc",
			f.srv.URL, r.URL.Query().Get("asset_code"), page+1)
		fmt.Fprintf(w, `{"_links":{"next":{"href":%q}},"_embedded":{"records":[%s]}}`,
			next, strings.Join(recs, ","))
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *assetsFixture) client() *Client {
	return NewClient(Config{
		BaseURL: f.srv.URL,
		Now:     func() time.Time { return time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC) },
		Sleep:   func(time.Duration) {},
	})
}

// TRAP 2, asserted directly. A first page of records that fills the limit looks
// like a complete answer and is not, and Horizon serves a next link on the last
// page too, so a walk that stops when the link disappears never stops.
func TestAssetsByCodeWalksEveryPageAndStopsOnAnEmptyOne(t *testing.T) {
	f := newAssetsFixture(t, [][]string{
		{record("AQUA", "GAAAA", "credit_alphanum4", 10, 0)},
		{record("AQUA", "GBBBB", "credit_alphanum4", 20, 3)},
		{}, // the end of the collection
	})
	got, err := f.client().AssetsByCode(context.Background(), "AQUA")
	if err != nil {
		t.Fatalf("AssetsByCode: %v", err)
	}
	if len(got.Assets) != 2 {
		t.Fatalf("got %d issuers, want 2: the walk stopped on the first page", len(got.Assets))
	}
	if got.Pages != 3 {
		t.Errorf("pages = %d, want 3 (two with records and the empty one)", got.Pages)
	}
	if got.Truncated {
		t.Error("a completed walk reported itself truncated")
	}
	if got.LedgerSeq != 64116131 {
		t.Errorf("ledger = %d, want the Latest-Ledger of the first page", got.LedgerSeq)
	}
}

// THE RULE THAT SHAPES EVERYTHING. One ticker, several issuers, and they are
// never merged, deduplicated by code, or ranked. A tool that returned one AQUA
// would be the bug this exists to prevent.
func TestAssetsByCodeNeverMergesIssuersOfOneTicker(t *testing.T) {
	f := newAssetsFixture(t, [][]string{
		{
			record("AQUA", "GIMPOSTOR1", "credit_alphanum4", 90000, 3),
			record("AQUA", "GBNZILSTREAL", "credit_alphanum4", 191838, 1308),
			record("AQUA", "GIMPOSTOR2", "credit_alphanum4", 5, 0),
		},
		{},
	})
	got, err := f.client().AssetsByCode(context.Background(), "AQUA")
	if err != nil {
		t.Fatalf("AssetsByCode: %v", err)
	}
	if len(got.Assets) != 3 {
		t.Fatalf("got %d issuers, want 3", len(got.Assets))
	}
	// Sorted by issuer, so the file this feeds is byte-stable.
	for i := 1; i < len(got.Assets); i++ {
		if got.Assets[i-1].Issuer > got.Assets[i].Issuer {
			t.Fatalf("issuers are not sorted: %s before %s",
				got.Assets[i-1].Issuer, got.Assets[i].Issuer)
		}
	}
	// Identity survives as the pair, on every row.
	for _, a := range got.Assets {
		if a.Asset().Code != "AQUA" || a.Asset().Issuer == "" {
			t.Errorf("row lost half its identity: %+v", a.Asset())
		}
	}
}

// TRAP 1. Both widths come back from one query keyed on the code alone, so the
// type is never inferred from len(code) and never asked for.
func TestAssetsByCodeReturnsBothWidthsWithoutBeingAsked(t *testing.T) {
	f := newAssetsFixture(t, [][]string{
		{
			record("USTRY", "GAAAA", "credit_alphanum12", 878, 1),
			record("USTRY", "GBBBB", "credit_alphanum4", 4, 0),
		},
		{},
	})
	got, err := f.client().AssetsByCode(context.Background(), "USTRY")
	if err != nil {
		t.Fatalf("AssetsByCode: %v", err)
	}
	if len(got.Assets) != 2 {
		t.Fatalf("got %d, want both widths", len(got.Assets))
	}
	widths := map[domain.AssetType]bool{}
	for _, a := range got.Assets {
		widths[a.Type] = true
	}
	if !widths[domain.AssetTypeAlphanum12] || !widths[domain.AssetTypeAlphanum4] {
		t.Errorf("a width was dropped: %v", widths)
	}
}

// TRAP 3. An endpoint returning an array is not a promise that every element
// answers the question. A neighbouring record read silently is the failure mode
// here, not an error.
func TestAssetsByCodeIgnoresARecordForAnotherCode(t *testing.T) {
	f := newAssetsFixture(t, [][]string{
		{
			record("AQUA", "GAAAA", "credit_alphanum4", 10, 0),
			record("AQUARIUS", "GBBBB", "credit_alphanum4", 999, 9),
		},
		{},
	})
	got, err := f.client().AssetsByCode(context.Background(), "AQUA")
	if err != nil {
		t.Fatalf("AssetsByCode: %v", err)
	}
	if len(got.Assets) != 1 {
		t.Fatalf("got %d, want 1: a neighbouring code was read as an answer", len(got.Assets))
	}
	if got.Assets[0].Code != "AQUA" {
		t.Errorf("code = %q", got.Assets[0].Code)
	}
}

// Every gathered field survives, as the STRING Horizon sent, trailing zeros
// included. No float appears anywhere on this path.
func TestAssetStatCarriesEveryFieldAsSent(t *testing.T) {
	f := newAssetsFixture(t, [][]string{
		{record("AQUA", "GAAAA", "credit_alphanum4", 191838, 1308)},
		{},
	})
	got, err := f.client().AssetsByCode(context.Background(), "AQUA")
	if err != nil {
		t.Fatalf("AssetsByCode: %v", err)
	}
	a := got.Assets[0]
	for _, tc := range []struct{ name, got, want string }{
		{"authorized_balance", a.AuthorizedBalance, "1000.0000000"},
		{"liquidity_pools_amount", a.LiquidityPoolsAmount, "9.9900000"},
		{"claimable_balances_amount", a.ClaimableBalancesAmt, "1.2500000"},
		{"contracts_amount", a.ContractsAmount, "0.0000000"},
		{"toml url", a.TomlURLReportedByHzn, "https://aqua.example/.well-known/stellar.toml"},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %q, want %q", tc.name, tc.got, tc.want)
		}
	}
	if a.AuthorizedAccounts != 191838 || a.NumLiquidityPools != 1308 {
		t.Errorf("counts lost: trustlines %d pools %d", a.AuthorizedAccounts, a.NumLiquidityPools)
	}
	if !a.AuthRevocable || a.AuthRequired {
		t.Errorf("flags lost: %+v", a)
	}
}

// A 429 is counted even when a retry recovers from it, because a rate limit that
// is absorbed silently is indistinguishable from one that never happened.
func TestThrottledCountsA429ThatWasRetriedThrough(t *testing.T) {
	f := newAssetsFixture(t, [][]string{
		{record("AQUA", "GAAAA", "credit_alphanum4", 1, 0)},
		{},
	})
	f.status429 = 2
	c := f.client()
	if _, err := c.AssetsByCode(context.Background(), "AQUA"); err != nil {
		t.Fatalf("AssetsByCode: %v", err)
	}
	if c.Throttled() != 2 {
		t.Errorf("Throttled = %d, want 2", c.Throttled())
	}
}

// home_domain is one half of a proof and this asserts only that the half is read
// and attributed to the right account.
func TestHomeDomainReadsTheAccountAndRefusesAnotherOne(t *testing.T) {
	var serve string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(serve))
	}))
	defer srv.Close()
	c := NewClient(Config{BaseURL: srv.URL, Sleep: func(time.Duration) {}})

	serve = `{"account_id":"GBNZ","home_domain":"aqua.network"}`
	got, err := c.HomeDomain(context.Background(), "GBNZ")
	if err != nil || got != "aqua.network" {
		t.Fatalf("HomeDomain = %q, %v", got, err)
	}

	// An account that publishes no domain is a fact, not an error.
	c2 := NewClient(Config{BaseURL: srv.URL, Sleep: func(time.Duration) {}})
	serve = `{"account_id":"GNODOMAIN"}`
	got, err = c2.HomeDomain(context.Background(), "GNODOMAIN")
	if err != nil || got != "" {
		t.Errorf("an absent home_domain should be empty and not an error; got %q, %v", got, err)
	}

	// A response describing a different account is the failure that looks like
	// data, so it is refused rather than returned.
	c3 := NewClient(Config{BaseURL: srv.URL, Sleep: func(time.Duration) {}})
	serve = `{"account_id":"GSOMEONEELSE","home_domain":"attacker.example"}`
	if _, err := c3.HomeDomain(context.Background(), "GBNZ"); err == nil {
		t.Error("a response for another account was accepted")
	}
}
