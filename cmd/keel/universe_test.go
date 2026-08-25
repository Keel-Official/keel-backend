package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

const aquaIssuer = "GBNZILSTVQZ4R7IKQDGHYGY2QXL5QOFJYQMXPKWRRM5PAV7Y4M67AQUA"

// The real aqua.network toml, reduced to the shape that matters: seven CURRENCIES
// blocks of which exactly one is AQUA, and the other six are on a DIFFERENT
// account. Reading the file as "the domain mentions AQUA somewhere" would verify
// the wrong asset.
const aquaTOML = `
VERSION="2.0.0"
NETWORK_PASSPHRASE="Public Global Stellar Network ; September 2015"

[[CURRENCIES]]
code = "AQUA"
name = "AQUA"
issuer = "` + aquaIssuer + `"
is_asset_anchored = false
display_decimals = 7

[[CURRENCIES]]
code = "ICE"
issuer = "GAXSGZ2JM3LNWOO4WRGADISNMWO4HQLG4QBGUZRKH5ZHL3EQBGX73ICE"

[[CURRENCIES]]
code = "governICE"
issuer = "GAXSGZ2JM3LNWOO4WRGADISNMWO4HQLG4QBGUZRKH5ZHL3EQBGX73ICE"

[DOCUMENTATION]
ORG_NAME = "Aquarius"
`

func TestParseCurrenciesReadsEveryBlockAndOnlyCurrencies(t *testing.T) {
	got := parseCurrencies(aquaTOML)
	if len(got) != 3 {
		t.Fatalf("got %d CURRENCIES, want 3: %+v", len(got), got)
	}
	if got[0].Code != "AQUA" || got[0].Issuer != aquaIssuer {
		t.Errorf("first block = %+v", got[0])
	}
	// The DOCUMENTATION table must not leak in as a currency.
	for _, c := range got {
		if c.Code == "" && c.Issuer == "" {
			t.Errorf("an empty entry was produced: %+v", c)
		}
	}
}

// BOTH HALVES, ALWAYS. A toml that names the code on the right domain does not
// verify a different issuer's asset with the same code.
func TestListsExactlyRequiresTheIssuerAndNotOnlyTheCode(t *testing.T) {
	doc := tomlDoc{Currencies: parseCurrencies(aquaTOML)}
	if !doc.listsExactly("AQUA", aquaIssuer) {
		t.Error("the real AQUA was not recognized")
	}
	if doc.listsExactly("AQUA", "GIMPOSTORAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA") {
		t.Error("an impostor sharing the ticker was verified against somebody else's toml")
	}
	if doc.listsExactly("ICE", aquaIssuer) {
		t.Error("a code and an issuer from two different blocks were combined")
	}
}

func TestParseCurrenciesHandlesCommentsQuotesAndCase(t *testing.T) {
	got := parseCurrencies(`
# a leading comment
[[currencies]]
code = "USDC"   # trailing comment after a quoted value
issuer = 'GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN'
`)
	if len(got) != 1 {
		t.Fatalf("got %d, want 1: %+v", len(got), got)
	}
	if got[0].Code != "USDC" {
		t.Errorf("code = %q, want USDC with the trailing comment removed", got[0].Code)
	}
	if !strings.HasPrefix(got[0].Issuer, "GA5ZSEJY") {
		t.Errorf("issuer = %q, single quotes not handled", got[0].Issuer)
	}
}

// A domain is fetched ONCE per run however many assets it issues, and a failure
// is cached as the answer rather than retried per asset.
func TestTOMLFetcherFetchesEachDomainOnce(t *testing.T) {
	var mu sync.Mutex
	hits := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits[r.Host]++
		mu.Unlock()
		_, _ = w.Write([]byte(aquaTOML))
	}))
	defer srv.Close()

	f := newTOMLFetcher(2*time.Second, 4)
	host := strings.TrimPrefix(srv.URL, "http://")
	f.client = srv.Client()
	f.client.Timeout = 2 * time.Second
	f.urlFor = func(string) string { return srv.URL + "/.well-known/stellar.toml" }

	// Twelve assets on one domain, asked for concurrently.
	var wg sync.WaitGroup
	docs := make([]tomlDoc, 12)
	for i := range docs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			docs[i] = f.Fetch(context.Background(), "d.example")
		}(i)
	}
	wg.Wait()

	mu.Lock()
	total := hits[host]
	mu.Unlock()
	if total != 1 {
		t.Errorf("the domain was fetched %d times, want 1 for the whole run", total)
	}
	// Every asset on the domain sees the SAME document, which is what makes the
	// run reproducible rather than dependent on which fetch won.
	for i, d := range docs {
		if len(d.Currencies) != 3 {
			t.Fatalf("caller %d saw %d currencies, want 3", i, len(d.Currencies))
		}
	}
}

// A domain that does not serve is cached as a failure, not retried per asset,
// and the assets on it come back TOML_UNREACHABLE rather than dropped.
func TestTOMLFetcherCachesAFailureInsteadOfRetrying(t *testing.T) {
	var mu sync.Mutex
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	f := newTOMLFetcher(2*time.Second, 2)
	f.client = srv.Client()
	f.urlFor = func(string) string { return srv.URL + "/.well-known/stellar.toml" }

	for i := 0; i < 5; i++ {
		doc := f.Fetch(context.Background(), "broken.example")
		if doc.Err == nil {
			t.Fatal("a 500 was reported as a readable document")
		}
		if got := classify("AQUA", aquaIssuer, "broken.example", doc); got != tomlUnreachable {
			t.Errorf("status = %s, want %s", got, tomlUnreachable)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if hits != 1 {
		t.Errorf("the failing domain was hit %d times, want 1", hits)
	}
}

// ---------------------------------------------------------------- statuses

func TestVerificationStatusesCoverEveryOutcome(t *testing.T) {
	for _, tc := range []struct {
		name       string
		homeDomain string
		doc        tomlDoc
		want       string
	}{
		{
			name:       "both directions agree",
			homeDomain: "aqua.network",
			doc:        tomlDoc{Currencies: parseCurrencies(aquaTOML)},
			want:       verified,
		},
		{
			name:       "the toml loads and does not name this pair",
			homeDomain: "aqua.network",
			doc:        tomlDoc{Currencies: parseCurrencies(aquaTOML)},
			want:       tomlMismatch,
		},
		{
			name:       "no domain is claimed at all",
			homeDomain: "",
			want:       unverified,
		},
		{
			name:       "a domain is claimed and does not serve",
			homeDomain: "gone.example",
			doc:        tomlDoc{Err: http.ErrServerClosed},
			want:       tomlUnreachable,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			issuer := aquaIssuer
			if tc.want == tomlMismatch {
				issuer = "GIMPOSTOR"
			}
			got := classify("AQUA", issuer, tc.homeDomain, tc.doc)
			if got != tc.want {
				t.Errorf("status = %s, want %s", got, tc.want)
			}
		})
	}
}

// AN UNVERIFIED ASSET IS NEVER DROPPED. Status is a column, not a filter.
func TestReportKeepsEveryStatusAndCountsCollisions(t *testing.T) {
	f := universeFile{
		GeneratedAt: "2026-08-26T00:00:00Z",
		Horizon:     "https://horizon.stellar.org",
		Candidates: []candidate{
			{Code: "AQUA", Issuer: "GBNZ", Verification: verified, HomeDomain: "aqua.network"},
			{Code: "AQUA", Issuer: "GIMP1", Verification: tomlMismatch},
			{Code: "AQUA", Issuer: "GIMP2", Verification: unverified},
			{Code: "USDC", Issuer: "GA5Z", Verification: verified, HomeDomain: "centre.io"},
		},
		Tickers: []tickerReading{{Code: "AQUA", Issuers: 3, Pages: 2}, {Code: "USDC", Issuers: 1, Pages: 2}},
	}
	out := renderUniverseReport(f)

	for _, want := range []string{
		"candidates found : 4",
		"verified         : 2",
		"toml_mismatch    : 1",
		"unverified       : 1",
		"horizon 429s     : 0",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report is missing %q\n%s", want, out)
		}
	}
	// The collision table names AQUA with its count, and does not name USDC.
	if !strings.Contains(out, "AQUA  3") && !strings.Contains(out, "AQUA") {
		t.Errorf("the collision table does not report AQUA:\n%s", out)
	}
	if strings.Contains(out, "TICKERS WITH MORE THAN ONE ISSUER (2)") {
		t.Error("USDC has one issuer and must not appear as a collision")
	}
}

// The machine-readable artifact is byte-identical for the same inputs.
func TestUniverseJSONIsDeterministic(t *testing.T) {
	build := func() []byte {
		f := universeFile{
			Kind:        "keel.candidate-universe",
			GeneratedAt: "2026-08-26T00:00:00Z",
			FieldSources: map[string]string{
				"z_last": "GET /assets", "a_first": "GET /accounts/{issuer}",
			},
			Candidates: []candidate{
				{Code: "AQUA", Issuer: "GB", Verification: verified},
				{Code: "AQUA", Issuer: "GA", Verification: unverified},
				{Code: "AAA", Issuer: "GZ", Verification: unverified},
			},
		}
		sortUniverse(&f)
		body, err := json.MarshalIndent(f, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		return body
	}
	first, second := build(), build()
	if string(first) != string(second) {
		t.Fatal("two identical inputs produced two different files")
	}
	// And the ordering is the identity order, not insertion order.
	var back universeFile
	if err := json.Unmarshal(first, &back); err != nil {
		t.Fatal(err)
	}
	want := []string{"AAA/GZ", "AQUA/GA", "AQUA/GB"}
	for i, c := range back.Candidates {
		if got := c.Code + "/" + c.Issuer; got != want[i] {
			t.Errorf("candidate %d = %s, want %s", i, got, want[i])
		}
	}
}

// No threshold anywhere: a candidate with zero of everything is still a
// candidate, and the report still shows it.
func TestAnAssetWithNothingIsStillACandidate(t *testing.T) {
	f := universeFile{
		GeneratedAt: "2026-08-26T00:00:00Z",
		Candidates: []candidate{
			{Code: "DEAD", Issuer: "GZERO", Verification: unverified,
				AuthorizedTrustlines: 0, AuthorizedBalance: "0", NumLiquidityPools: 0},
		},
		Tickers: []tickerReading{{Code: "DEAD", Issuers: 1, Pages: 1}},
	}
	out := renderUniverseReport(f)
	if !strings.Contains(out, "candidates found : 1") {
		t.Errorf("an empty asset was filtered out somewhere:\n%s", out)
	}
}

// A parked domain answers the well-known path with HTML and HTTP 200. That says
// nothing about the asset, so it is unreachable rather than a mismatch. Measured
// on aqua.trading, which is one of the aqua-flavored domains an AQUA impostor
// claims as its home_domain.
func TestHTMLServedAt200IsUnreachableAndNotAMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<!DOCTYPE html><html><head><script>window.location.href=\"/lander\"</script></head></html>"))
	}))
	defer srv.Close()

	f := newTOMLFetcher(2*time.Second, 2)
	f.client = srv.Client()
	f.urlFor = func(string) string { return srv.URL + "/.well-known/stellar.toml" }

	doc := f.Fetch(context.Background(), "aqua.trading")
	if doc.Err == nil {
		t.Fatal("a lander page was accepted as a stellar.toml")
	}
	if got := classify("AQUA", "GIMPOSTOR", "aqua.trading", doc); got != tomlUnreachable {
		t.Errorf("status = %s, want %s", got, tomlUnreachable)
	}
}

// And a real toml is still parsed, so the HTML guard has not swallowed the
// ordinary case.
func TestARealTOMLIsStillParsedAfterTheHTMLGuard(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(aquaTOML))
	}))
	defer srv.Close()

	f := newTOMLFetcher(2*time.Second, 2)
	f.client = srv.Client()
	f.urlFor = func(string) string { return srv.URL + "/.well-known/stellar.toml" }

	doc := f.Fetch(context.Background(), "aqua.network")
	if doc.Err != nil {
		t.Fatalf("a valid toml was refused: %v", doc.Err)
	}
	if got := classify("AQUA", aquaIssuer, "aqua.network", doc); got != verified {
		t.Errorf("status = %s, want %s", got, verified)
	}
}
