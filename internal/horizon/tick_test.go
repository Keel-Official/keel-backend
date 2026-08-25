package horizon

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// NO NETWORK. Every test here runs against httptest with bodies recorded from
// live Horizon, which is the rule that lets this suite run in CI and on a train.
// The one thing these tests are really for is the promise in decision 1 of
// tick.go: the bytes that arrive are the bytes that are stored. Everything else
// is arrangement around that assertion.

// emptyPoolBody is what /liquidity_pools really returns for a pair with no AMM
// pool: HTTP 200, and an empty records array. Taken from a live call on 25
// August 2026, docs/evidences/liquidity_pools_reserves_2026-08-25.txt section 4.
// THIS IS DATA, NOT AN ERROR, and the recorder stores it and carries on.
const emptyPoolBody = `{
  "_links": {
    "self": {"href": "https://horizon.stellar.org/liquidity_pools?cursor=&limit=200&order=asc&reserves=A%2CB"},
    "next": {"href": "https://horizon.stellar.org/liquidity_pools?cursor=&limit=200&order=asc&reserves=A%2CB"},
    "prev": {"href": "https://horizon.stellar.org/liquidity_pools?cursor=&limit=200&order=desc&reserves=A%2CB"}
  },
  "_embedded": {
    "records": []
  }
}`

// rateLimitBody is Horizon's own 429 shape.
const rateLimitBody = `{
  "type": "https://stellar.org/horizon-errors/rate_limit_exceeded",
  "title": "Rate limit exceeded",
  "status": 429,
  "detail": "The rate limit for the requesting IP address is over its alloted limit."
}`

func sha256Of(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// serve replaces one endpoint's handler with a fixed status and body.
func serve(f *fakeHorizon, path string, status int, body string) {
	f.handler[path] = func(w http.ResponseWriter, _ *http.Request) {
		if status != http.StatusOK {
			w.WriteHeader(status)
		}
		fmt.Fprint(w, body)
	}
}

func TestRecordTicksOnce(t *testing.T) {
	book := bookBody("1.2185312", "0.0001000")

	tests := []struct {
		name string
		// setup adjusts the fake before the round runs.
		setup func(f *fakeHorizon)

		wantBefore     uint32
		wantAfter      uint32
		wantConsistent bool
		wantStatus     map[string]int
		// wantBody is the exact string each endpoint should have stored.
		wantBody map[string]string
		// wantHits is the request count per path, where a retried endpoint is
		// hit more than once.
		wantHits map[string]int
	}{
		{
			name:           "both endpoints succeed",
			setup:          func(*fakeHorizon) {},
			wantBefore:     testLedger,
			wantAfter:      testLedger,
			wantConsistent: true,
			wantStatus:     map[string]int{EndpointOrderBook: 200, EndpointLiquidityPools: 200},
			wantBody:       map[string]string{EndpointOrderBook: book, EndpointLiquidityPools: poolBody},
			wantHits:       map[string]int{"/order_book": 1, "/liquidity_pools": 1},
		},
		{
			// An empty pool list is one of the four liquidity buckets the
			// validation document has to fill, so a tick that discarded it
			// would be discarding the answer.
			name: "an empty pool result is data and is recorded",
			setup: func(f *fakeHorizon) {
				serve(f, "/liquidity_pools", http.StatusOK, emptyPoolBody)
			},
			wantBefore:     testLedger,
			wantAfter:      testLedger,
			wantConsistent: true,
			wantStatus:     map[string]int{EndpointOrderBook: 200, EndpointLiquidityPools: 200},
			wantBody:       map[string]string{EndpointOrderBook: book, EndpointLiquidityPools: emptyPoolBody},
			wantHits:       map[string]int{"/order_book": 1, "/liquidity_pools": 1},
		},
		{
			// The ledger closed between the two requests. The tick is stored
			// with the flag inside it; a difference that is recorded can be
			// explained later and one that was discarded cannot.
			name: "a ledger shift between the two requests is recorded, not rejected",
			setup: func(f *fakeHorizon) {
				f.ledger["/liquidity_pools"] = "61340264"
			},
			wantBefore:     testLedger,
			wantAfter:      testLedger + 1,
			wantConsistent: false,
			wantStatus:     map[string]int{EndpointOrderBook: 200, EndpointLiquidityPools: 200},
			wantBody:       map[string]string{EndpointOrderBook: book, EndpointLiquidityPools: poolBody},
			wantHits:       map[string]int{"/order_book": 1, "/liquidity_pools": 1},
		},
		{
			// A 429 is retried and then RECORDED. The tick survives, which is
			// the whole of decision 4: the ledger this reading belongs to has
			// closed by the time anybody reads a red build.
			name: "one endpoint returning 429 does not discard the tick",
			setup: func(f *fakeHorizon) {
				serve(f, "/liquidity_pools", http.StatusTooManyRequests, rateLimitBody)
			},
			wantBefore:     testLedger,
			wantAfter:      testLedger,
			wantConsistent: true,
			wantStatus:     map[string]int{EndpointOrderBook: 200, EndpointLiquidityPools: 429},
			wantBody:       map[string]string{EndpointOrderBook: book, EndpointLiquidityPools: rateLimitBody},
			// One attempt plus the four retries the client defaults to.
			wantHits: map[string]int{"/order_book": 1, "/liquidity_pools": 5},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeHorizon(t)
			tc.setup(f)
			r, root := testRecorder(t, f)

			results := r.RecordTicksOnce(context.Background())
			if len(results) != 1 {
				t.Fatalf("got %d results, want 1", len(results))
			}
			res := results[0]
			if res.Err != nil {
				t.Fatalf("the tick was not written: %v", res.Err)
			}
			if !res.Written {
				t.Fatal("the tick reports itself as not written and carries no error")
			}
			if res.Collided {
				t.Error("the first tick of a ledger reported a name collision")
			}

			// The path is recordings/{PAIR}/{DATE}/{LEDGER}.json.gz, and the
			// date comes from the tick's own recorded_at. See decision 6.
			want := filepath.Join(root,
				"USTRY.GCRYUGD5-USDC.GA5ZSEJY", "2026-08-24",
				fmt.Sprintf("%d.json.gz", tc.wantBefore))
			if res.Path != want {
				t.Errorf("path = %s, want %s", res.Path, want)
			}

			// Read it back through the package's own reader rather than
			// trusting the in-memory value, so this asserts on what reached
			// disk.
			tick, err := ReadTick(res.Path)
			if err != nil {
				t.Fatalf("read the tick back: %v", err)
			}

			if tick.SchemaVersion != TickSchemaVersion {
				t.Errorf("schema_version = %d, want %d", tick.SchemaVersion, TickSchemaVersion)
			}
			if got := testUSTRY.String() + "/" + testUSDC.String(); tick.Pair != got {
				t.Errorf("pair = %q, want %q", tick.Pair, got)
			}
			if tick.RecordedAt != "2026-08-24T12:00:00Z" {
				t.Errorf("recorded_at = %q, want an RFC3339 UTC stamp of the injected clock", tick.RecordedAt)
			}
			if tick.LedgerBefore != tc.wantBefore || tick.LedgerAfter != tc.wantAfter {
				t.Errorf("ledger %d->%d, want %d->%d",
					tick.LedgerBefore, tick.LedgerAfter, tc.wantBefore, tc.wantAfter)
			}
			if tick.LedgerConsistent != tc.wantConsistent {
				t.Errorf("ledger_consistent = %t, want %t", tick.LedgerConsistent, tc.wantConsistent)
			}

			// Exactly two sources, in a fixed order, because that order is what
			// decides which header became ledger_before.
			if len(tick.Sources) != 2 {
				t.Fatalf("got %d sources, want 2", len(tick.Sources))
			}
			if tick.Sources[0].Endpoint != EndpointOrderBook ||
				tick.Sources[1].Endpoint != EndpointLiquidityPools {
				t.Errorf("sources are in the order %q, %q; want %q then %q",
					tick.Sources[0].Endpoint, tick.Sources[1].Endpoint,
					EndpointOrderBook, EndpointLiquidityPools)
			}

			for _, src := range tick.Sources {
				if got := tc.wantStatus[src.Endpoint]; src.HTTPStatus != got {
					t.Errorf("%s: http_status = %d, want %d", src.Endpoint, src.HTTPStatus, got)
				}
				if src.Error != "" {
					t.Errorf("%s: error = %q, and there was an HTTP response", src.Endpoint, src.Error)
				}

				// THE ASSERTION THIS FILE EXISTS FOR. The stored body is the
				// served body byte for byte, and its recorded digest is the
				// digest of those same bytes. Nothing was reformatted, parsed,
				// re-encoded or normalised on the way through.
				wantBody := tc.wantBody[src.Endpoint]
				if src.Body != wantBody {
					t.Errorf("%s: the stored body is not the served body.\n got: %q\nwant: %q",
						src.Endpoint, src.Body, wantBody)
				}
				if src.BodySHA256 != sha256Of(wantBody) {
					t.Errorf("%s: body_sha256 = %s, want %s (the digest of the bytes Horizon served)",
						src.Endpoint, src.BodySHA256, sha256Of(wantBody))
				}

				// And the URL is the one that was really requested, so the
				// reading can be repeated from the file alone.
				if !strings.HasPrefix(src.URL, f.srv.URL+"/"+src.Endpoint+"?") {
					t.Errorf("%s: url = %q, which does not name the endpoint it claims", src.Endpoint, src.URL)
				}
			}

			for path, wantHits := range tc.wantHits {
				if f.hits[path] != wantHits {
					t.Errorf("%s was requested %d times, want %d", path, f.hits[path], wantHits)
				}
			}
		})
	}
}

// A 429 in a tick is DEGRADED and still WRITTEN, and that difference is what
// decides the process exit code. It is asserted separately from the table
// because it is a claim about the round tally rather than about one file.
func TestA429IsDegradedButStillWritten(t *testing.T) {
	f := newFakeHorizon(t)
	serve(f, "/liquidity_pools", http.StatusTooManyRequests, rateLimitBody)
	r, _ := testRecorder(t, f)

	round := r.ReportTicks(r.RecordTicksOnce(context.Background()))
	if round.Written != 1 {
		t.Errorf("Written = %d, want 1: a 429 must not cost the tick", round.Written)
	}
	if round.Degraded != 1 {
		t.Errorf("Degraded = %d, want 1", round.Degraded)
	}
	if round.Unwritten != 0 {
		t.Errorf("Unwritten = %d, want 0, which is the only number that may redden a run", round.Unwritten)
	}
}

func TestFilenameCollisionAppendsASuffixAndOverwritesNothing(t *testing.T) {
	f := newFakeHorizon(t)
	r, root := testRecorder(t, f)
	dir := filepath.Join(root, "USTRY.GCRYUGD5-USDC.GA5ZSEJY", "2026-08-24")

	// Three rounds inside one ledger. The fake serves the same Latest-Ledger
	// every time, so all three want the same name.
	var paths []string
	for i := 0; i < 3; i++ {
		results := r.RecordTicksOnce(context.Background())
		res := results[0]
		if res.Err != nil {
			t.Fatalf("round %d: %v", i, res.Err)
		}
		if got := res.Collided; got != (i > 0) {
			t.Errorf("round %d: Collided = %t, want %t", i, got, i > 0)
		}
		paths = append(paths, res.Path)
	}

	want := []string{
		filepath.Join(dir, "61340263.json.gz"),
		filepath.Join(dir, "61340263-1.json.gz"),
		filepath.Join(dir, "61340263-2.json.gz"),
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Errorf("round %d wrote %s, want %s", i, paths[i], want[i])
		}
	}

	// Three names, three files, and no leftover .partial-* from the temporary
	// that each write goes through.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("the directory holds %d entries, want exactly 3: %v", len(entries), names)
	}

	// Every one of the three is a complete, readable tick. A collision that
	// truncated or clobbered an earlier file would show up here and nowhere
	// else.
	for _, p := range paths {
		tick, err := ReadTick(p)
		if err != nil {
			t.Errorf("%s: %v", p, err)
			continue
		}
		src, ok := tick.Source(EndpointOrderBook)
		if !ok {
			t.Errorf("%s: no order book source", p)
			continue
		}
		if src.BodySHA256 != sha256Of(src.Body) {
			t.Errorf("%s: body_sha256 does not match its own body, so the file was altered after writing", p)
		}
	}
}

// A tick whose first request carries no Latest-Ledger is still written, under
// the name 0.json.gz, and reports itself inconsistent. Recording it under an
// invented sequence would be worse than recording it under a zero that is
// visibly wrong.
func TestATickWithNoLedgerHeaderIsStillWritten(t *testing.T) {
	f := newFakeHorizon(t)
	f.ledger["/order_book"] = "" // the fake omits the header entirely
	r, root := testRecorder(t, f)

	res := r.RecordTicksOnce(context.Background())[0]
	if res.Err != nil {
		t.Fatalf("the tick was not written: %v", res.Err)
	}
	want := filepath.Join(root, "USTRY.GCRYUGD5-USDC.GA5ZSEJY", "2026-08-24", "0.json.gz")
	if res.Path != want {
		t.Errorf("path = %s, want %s", res.Path, want)
	}
	if res.Tick.LedgerConsistent {
		t.Error("ledger_consistent is true, and ledger_before was never read")
	}
}

// Version 1 files stay readable and are not mistaken for version 2. The absence
// of schema_version is what identifies them; see PeekSchemaVersion.
func TestSchemaVersionsAreToldApartOnDisk(t *testing.T) {
	f := newFakeHorizon(t)
	r, _ := testRecorder(t, f)
	ctx := context.Background()

	v1 := r.RecordOnce(ctx)[0]
	if v1.Err != nil {
		t.Fatalf("schema 1 round: %v", v1.Err)
	}
	v2 := r.RecordTicksOnce(ctx)[0]
	if v2.Err != nil {
		t.Fatalf("schema 2 round: %v", v2.Err)
	}

	for _, tc := range []struct {
		path string
		want int
	}{
		{v1.Path, 1},
		{v2.Path, 2},
	} {
		got, err := PeekSchemaVersion(tc.path)
		if err != nil {
			t.Fatalf("%s: %v", tc.path, err)
		}
		if got != tc.want {
			t.Errorf("%s: schema %d, want %d", tc.path, got, tc.want)
		}
	}

	// And the version 1 reader still reads the version 1 file, unchanged.
	raw, err := ReadRecording(v1.Path)
	if err != nil {
		t.Fatalf("the version 1 reader stopped reading a version 1 file: %v", err)
	}
	if raw.LedgerSeq != testLedger {
		t.Errorf("version 1 recording reads back ledger %d, want %d", raw.LedgerSeq, testLedger)
	}
}

// A malformed issuer is refused when the pair FILE is loaded, before a single
// request is made, and the message names the pair. The 57 character string below
// is the one the AUDD pair was first configured with; Horizon answers it with a
// 400 and /order_book would answer it with an empty book.
func TestLoadPairsRefusesAMalformedIssuerAndNamesThePair(t *testing.T) {
	tests := []struct {
		name    string
		issuer  string
		wantErr string
	}{
		{
			name:    "one character too long",
			issuer:  "GDC7X2MXTYSAKUUGAIQ7J7RPEIM7GXSAIWFYWWWH4GLNFECQVJJLB2EEU",
			wantErr: "57 characters and a Stellar account ID is 56",
		},
		{
			name:    "not an account ID",
			issuer:  "MDC7X2MXTYSAKUUGAIQ7J7RPEIM7GXSAIWFYWWH4GLNFECQVJJLB2EEU",
			wantErr: "does not start with G",
		},
		{
			name:    "not base32",
			issuer:  "GDC7X2MXTYSAKUUGAIQ7J7RPEIM7GXSAIWFYWW1H4GLNFECQVJJLB2EE",
			wantErr: "not in the base32 alphabet",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "pairs.json")
			body := fmt.Sprintf(`{
			  "note": "test",
			  "pairs": [{
			    "base":  {"code": "AUDD", "issuer": %q, "type": "credit_alphanum4"},
			    "quote": {"code": "USDC", "issuer": %q, "type": "credit_alphanum4"},
			    "note": "test"
			  }]
			}`, tc.issuer, testUSDC.Issuer)
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}

			_, err := LoadPairs(path)
			if err == nil {
				t.Fatal("LoadPairs accepted a malformed issuer, and never falls back to a default")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not explain the problem, want it to contain %q", err, tc.wantErr)
			}
			// Naming the pair is the point: a list of eight is otherwise eight
			// lines to check by hand.
			if !strings.Contains(err.Error(), "AUDD:"+tc.issuer) {
				t.Errorf("error %q does not name the offending pair", err)
			}
		})
	}
}
