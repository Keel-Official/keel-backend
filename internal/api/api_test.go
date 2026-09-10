package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/Keel-Official/keel-backend/internal/domain"
	"github.com/Keel-Official/keel-backend/internal/store"
)

// These tests run against a fake Reader and no database, so they run in CI on
// every push. The SQL behind the real Reader is proven separately by
// internal/store's integration tests; what is proven here is the HTTP: status
// codes, the wire shape, the headers, and the states the contract says are 200
// rather than errors.

// ---------------------------------------------------------------- fake reader

type fakeReader struct {
	assets   []store.Asset
	pairs    map[string][]store.Asset // code|issuer -> pairs
	latest   map[int]store.Metric
	atLedger map[string]store.Metric // assetID|ledger|version|source

	askedVersion      string
	askedLedgerSource domain.DataSource
	history           map[int][]store.Metric
	summaries         []store.Metric
	total             int
	lastRun           *store.Run
	err               error

	// gotSource is what the last MetricsHistory call asked for.
	gotSource domain.DataSource
}

func (f *fakeReader) Assets(context.Context, bool) ([]store.Asset, error) {
	return f.assets, f.err
}

func (f *fakeReader) PairsForAsset(_ context.Context, code, issuer string) ([]store.Asset, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.pairs[code+"|"+issuer], nil
}

func (f *fakeReader) LatestMetrics(_ context.Context, assetID int, _ string) (store.Metric, error) {
	if f.err != nil {
		return store.Metric{}, f.err
	}
	m, ok := f.latest[assetID]
	if !ok {
		return store.Metric{}, fmt.Errorf("%w: asset %d", store.ErrNotFound, assetID)
	}
	return m, nil
}

// MetricsAtLedger keys on ALL FOUR parts, the way store.MetricsAtLedger does.
//
// IT USED TO KEY ON TWO, and that is why a defect sat here unseen. The handler
// asked the store for an empty methodology version and for hubble rows, and both
// were wrong: the store treats an empty version as a literal and matches nothing,
// and hubble is a source DEC-002 holds so no row can carry it. A fake that
// ignores half the key cannot fail on either. It records what it was asked for,
// so a test can assert the request rather than only the response.
func (f *fakeReader) MetricsAtLedger(_ context.Context, assetID int, seq uint32, version string, source domain.DataSource) (store.Metric, error) {
	f.askedVersion = version
	f.askedLedgerSource = source
	key := strconv.Itoa(assetID) + "|" + strconv.FormatUint(uint64(seq), 10) + "|" + version + "|" + string(source)
	m, ok := f.atLedger[key]
	if !ok {
		return store.Metric{}, fmt.Errorf("%w: ledger %d", store.ErrNotFound, seq)
	}
	return m, nil
}

// gotSource records the source the handler asked for, so a test can assert the
// default and the override reach the store rather than only that the response
// looks right.
func (f *fakeReader) MetricsHistory(_ context.Context, assetID int, from, to uint32, _ string, source domain.DataSource, _ int) ([]store.Metric, error) {
	f.gotSource = source
	if f.err != nil {
		return nil, f.err
	}
	var out []store.Metric
	for _, m := range f.history[assetID] {
		// The real store filters on the source because it is part of the key.
		// The fake filters too, so a test that mixes sources sees what Postgres
		// would return and not everything it was given.
		if m.Risk.LedgerSeq >= from && m.Risk.LedgerSeq <= to && m.Risk.DataSource == source {
			out = append(out, m)
		}
	}
	return out, nil
}

func (f *fakeReader) LatestSummaries(_ context.Context, filter store.SummaryFilter) ([]store.Metric, int, error) {
	if f.err != nil {
		return nil, 0, f.err
	}
	var out []store.Metric
	for _, m := range f.summaries {
		if filter.Band != "" && m.Risk.Band != filter.Band {
			continue
		}
		if filter.Flag != "" && !hasFlag(m.Risk.Flags, filter.Flag) {
			continue
		}
		out = append(out, m)
	}
	total := len(out)
	if f.total > 0 {
		total = f.total
	}
	return out, total, nil
}

func (f *fakeReader) LastRun(context.Context, store.RunKind) (store.Run, error) {
	if f.lastRun == nil {
		return store.Run{}, fmt.Errorf("%w: no scan run", store.ErrNotFound)
	}
	return *f.lastRun, nil
}

func hasFlag(in []domain.Flag, want domain.Flag) bool {
	for _, f := range in {
		if f == want {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------- fixtures

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
	testXLM = domain.Asset{Code: "XLM", Type: domain.AssetTypeNative}
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }
func dp(s string) *decimal.Decimal {
	v := d(s)
	return &v
}

const ustryID = "USTRY:GCRYUGD5NVARGXT56XEZI5CIFCQETYHAPQQTHO2O3IQZTHDH4LATMYWC"

func ustryPair(id int) store.Asset {
	return store.Asset{ID: id, Base: testUSTRY, Quote: testUSDC, Active: true}
}

// riskFixture is the USTRY market as the golden fixture records it: a book with
// one ask far above the bid, zero depth at every rung, and a manipulation target
// that is free to reach.
func riskFixture() store.Metric {
	closed := time.Date(2026, 2, 22, 0, 10, 21, 0, time.UTC)
	return store.Metric{
		ID:      1,
		AssetID: 7,
		// Two and a half minutes after the ledger closed, which is what the
		// staleness header reports.
		ComputedAt: closed.Add(150 * time.Second),
		Risk: domain.AssetRisk{
			Base:               testUSTRY,
			Quote:              testUSDC,
			LedgerSeq:          61340263,
			LedgerClosedAt:     closed,
			MethodologyVersion: domain.MethodologyVersion,
			DataSource:         domain.DataSourceHorizon,
			MidPrice:           dp("53.8971414"),
			PriceSource:        domain.PriceSourceBook,
			SpreadPct:          dp("196.0777140585048"),
			Depth: []domain.DepthPoint{
				{Delta: d("0.02"), BuySide: d("0"), SellSide: d("0"), FromSdex: d("0"), FromAmm: d("0")},
				{Delta: d("0.05"), BuySide: d("0"), SellSide: d("0"), FromSdex: d("0"), FromAmm: d("0")},
				{Delta: d("0.10"), BuySide: d("0"), SellSide: d("0"), FromSdex: d("0"), FromAmm: d("0")},
			},
			ManipulationCostCombined: []domain.ManipulationPoint{
				{Delta: d("0.5"), TargetPrice: d("80.8457121"), Cost: d("0"), Reachable: true},
				{Delta: d("1"), TargetPrice: d("107.7942828"), Cost: d("130.06270929502336"), Reachable: false},
			},
			ManipulationCostOrderbookOnly: []domain.ManipulationPoint{
				{Delta: d("0.5"), TargetPrice: d("80.8457121"), Cost: d("0"), Reachable: true},
				{Delta: d("1"), TargetPrice: d("107.7942828"), Cost: d("130.06270929502336"), Reachable: false},
			},
			MaxReachablePrice:       dp("106.7372828"),
			CostToMaxReachablePrice: dp("0"),
			OracleResistance: &domain.OracleResistance{
				CriticalDelta:    d("0.5"),
				ManipulationCost: d("0"),
				Reachable:        true,
				GenuineVolume:    d("5.3475699"),
				WindowSeconds:    900,
				Ratio:            dp("0"),
				TotalAttackCost:  dp("5.3475699"),
			},
			MaxSafeCollateral:            dp("0"),
			MaxSafeCollateralLiquidation: dp("0"),
			Flags:                        []domain.Flag{domain.FlagZeroDepth2Pct, domain.FlagSpreadExtreme},
			UnevaluatedFlags:             []domain.Flag{domain.FlagNoGenuineTrade30D},
			Band:                         domain.BandCritical,
			BandConfidence:               domain.BandConfidencePartial,
			Warnings:                     []string{"the manipulation term was not applied: target unreachable"},
		},
	}
}

func newTestServer(t *testing.T, f *fakeReader) http.Handler {
	t.Helper()
	s, err := New(Config{
		Reader: f,
		Params: domain.DefaultParams(),
		// THE ALLOWLIST IS PASSED AND NOT INHERITED. A nil AllowedOrigins tells
		// New to read KEEL_CORS_ORIGINS, so leaving it out here would make every
		// test in this file depend on the environment of whoever runs it, and a
		// developer with a wildcard exported would see New fail in forty tests
		// that have nothing to do with CORS. The tests that mean to exercise the
		// environment set it with t.Setenv and construct their own server.
		AllowedOrigins: []string{testOrigin},
		Logf:           func(string, ...any) {},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s.Handler()
}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

// decodeBody decodes with UseNumber, so a JSON number arrives as json.Number and
// never as float64.
//
// That is not a style preference. Non-negotiable rule 1 bans float64 across the
// whole repository and TestArchTanpaFloatDiSeluruhRepo enforces it in test files
// too, with an allowlist that is empty and meant to stay empty. It also makes
// these assertions stronger: json.Number holds the digits that were actually
// sent, so a test can compare "0.02" exactly instead of comparing two binary
// approximations.
func decodeBody(t *testing.T, rec *httptest.ResponseRecorder, into any) {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(rec.Body.Bytes()))
	dec.UseNumber()
	if err := dec.Decode(into); err != nil {
		t.Fatalf("decode body: %v\nbody: %s", err, rec.Body.String())
	}
}

// ---------------------------------------------------------------- depth

func TestDepthReturnsTheContractShape(t *testing.T) {
	m := riskFixture()
	f := &fakeReader{
		pairs:  map[string][]store.Asset{"USTRY|" + testUSTRY.Issuer: {ustryPair(7)}},
		latest: map[int]store.Metric{7: m},
	}
	rec := get(t, newTestServer(t, f), BasePath+"/asset/"+ustryID+"/depth")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q", ct)
	}
	// The two headers rule 4 of the brief requires on every response.
	if got := rec.Header().Get("X-Keel-Methodology-Version"); got != domain.MethodologyVersion {
		t.Errorf("methodology header = %q, want %q", got, domain.MethodologyVersion)
	}
	if got := rec.Header().Get("X-Keel-Staleness-Seconds"); got != "150" {
		t.Errorf("staleness header = %q, want 150, computedAt minus ledgerClosedAt", got)
	}

	// Decoded into a map rather than the response struct, so the test reads the
	// JSON a consumer sees and not the Go type that produced it.
	var body map[string]any
	decodeBody(t, rec, &body)

	// Every decimal is a STRING. This is the rule most likely to be broken by a
	// well-meaning refactor, so it is asserted on a real number rather than on a
	// type.
	if got, ok := body["midPrice"].(string); !ok || got != "53.8971414" {
		t.Errorf("midPrice = %#v, want the string \"53.8971414\"", body["midPrice"])
	}
	if got, ok := body["spreadPct"].(string); !ok || got != "196.0777140585048" {
		t.Errorf("spreadPct = %#v, want a percent-scaled string", body["spreadPct"])
	}

	// delta is one of the few JSON numbers the contract asks for.
	depth, ok := body["depth"].([]any)
	if !ok || len(depth) != 3 {
		t.Fatalf("depth = %#v, want three rungs", body["depth"])
	}
	first := depth[0].(map[string]any)
	// An unquoted JSON number, and its digits are exactly what the ladder holds.
	if n, isNumber := first["delta"].(json.Number); !isNumber || n.String() != "0.02" {
		t.Errorf("depth[0].delta = %#v, want the JSON number 0.02", first["delta"])
	}
	if _, isString := first["buySide"].(string); !isString {
		t.Errorf("depth[0].buySide = %#v, want a string", first["buySide"])
	}

	// The asset identity carries its type explicitly and the issuer is null only
	// for the native asset.
	a := body["asset"].(map[string]any)
	if a["type"] != "credit_alphanum12" {
		t.Errorf("asset.type = %#v, want credit_alphanum12 read from storage", a["type"])
	}

	// cost never travels without reachable.
	for _, key := range []string{"manipulationCostCombined", "manipulationCostOrderbookOnly"} {
		ladder, ok := body[key].([]any)
		if !ok || len(ladder) == 0 {
			t.Fatalf("%s = %#v", key, body[key])
		}
		for i, rung := range ladder {
			r := rung.(map[string]any)
			if _, ok := r["reachable"].(bool); !ok {
				t.Errorf("%s[%d] has no reachable", key, i)
			}
			if _, ok := r["cost"].(string); !ok {
				t.Errorf("%s[%d].cost is not a string", key, i)
			}
		}
	}

	// Arrays are present and empty rather than null, and nullable scalars are
	// null rather than zero.
	if body["flags"] == nil || body["unevaluatedFlags"] == nil || body["warnings"] == nil {
		t.Error("an array arrived as null; the contract marks these required")
	}
	if _, present := body["maxSafeCollateralManipulation"]; !present {
		t.Error("maxSafeCollateralManipulation is absent; it must be present and null")
	}
	if body["maxSafeCollateralManipulation"] != nil {
		t.Errorf("maxSafeCollateralManipulation = %#v, want null when the target is unreachable",
			body["maxSafeCollateralManipulation"])
	}
	if body["volumeToSupply"] != nil {
		t.Error("volumeToSupply is not null although no window was computed")
	}
	if body["bandConfidence"] != "partial" {
		t.Errorf("bandConfidence = %#v, want partial", body["bandConfidence"])
	}
}

// Rule 5 of the brief, and point 3 of the contract's own preamble.
func TestAnAssetWithNoPriceIs200AndNotAnError(t *testing.T) {
	m := riskFixture()
	m.Risk.MidPrice = nil
	m.Risk.SpreadPct = nil
	m.Risk.PriceSource = domain.PriceSourceNone
	m.Risk.Band = domain.BandCritical
	m.Risk.Flags = []domain.Flag{domain.FlagNoExecutablePrice}

	f := &fakeReader{
		pairs:  map[string][]store.Asset{"USTRY|" + testUSTRY.Issuer: {ustryPair(7)}},
		latest: map[int]store.Metric{7: m},
	}
	rec := get(t, newTestServer(t, f), BasePath+"/asset/"+ustryID+"/depth")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: an asset with no executable price is a finding, not a failure", rec.Code)
	}
	var body map[string]any
	decodeBody(t, rec, &body)
	if body["priceSource"] != "none" {
		t.Errorf("priceSource = %#v, want none", body["priceSource"])
	}
	if body["midPrice"] != nil {
		t.Errorf("midPrice = %#v, want null", body["midPrice"])
	}
	if body["band"] != "CRITICAL" {
		t.Errorf("band = %#v, want CRITICAL", body["band"])
	}
}

func TestUnknownAssetIsNotMonitoredAndNot500(t *testing.T) {
	f := &fakeReader{pairs: map[string][]store.Asset{}}
	rec := get(t, newTestServer(t, f), BasePath+"/asset/NOPE:GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA/depth")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	var body errorBodyJSON
	decodeBody(t, rec, &body)
	if body.Error.Code != codeAssetNotMonitored {
		t.Errorf("code = %q, want %s", body.Error.Code, codeAssetNotMonitored)
	}
}

func TestMalformedAssetIDIs400(t *testing.T) {
	f := &fakeReader{}
	h := newTestServer(t, f)
	for _, id := range []string{"ustry", "USTRY:not-an-issuer", "USTRY%3Anope", "TOOLONGACODEHERE:GCRYUGD5NVARGXT56XEZI5CIFCQETYHAPQQTHO2O3IQZTHDH4LATMYWC"} {
		rec := get(t, h, BasePath+"/asset/"+id+"/depth")
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", id, rec.Code)
			continue
		}
		var body errorBodyJSON
		decodeBody(t, rec, &body)
		if body.Error.Code != codeInvalidAssetID {
			t.Errorf("%s: code = %q, want %s", id, body.Error.Code, codeInvalidAssetID)
		}
	}
}

// The asset type is looked up, never inferred. USTRY has five characters and is
// alphanum12; a length rule would call it alphanum4 and measure another asset.
func TestAssetTypeComesFromStorageAndNotFromTheCodeLength(t *testing.T) {
	m := riskFixture()
	f := &fakeReader{
		pairs:  map[string][]store.Asset{"USTRY|" + testUSTRY.Issuer: {ustryPair(7)}},
		latest: map[int]store.Metric{7: m},
	}
	rec := get(t, newTestServer(t, f), BasePath+"/asset/"+ustryID+"/depth")
	var body map[string]any
	decodeBody(t, rec, &body)

	if got := body["asset"].(map[string]any)["type"]; got != string(domain.AssetTypeAlphanum12) {
		t.Errorf("asset.type = %#v; a five character code was typed by its length", got)
	}
}

// Omitting quote is only unambiguous when the asset has one pair. The primary
// pair rule is decision D-1 and is not decided, so the API says so rather than
// choosing.
func TestOmittedQuoteWithSeveralPairsSaysSoRatherThanChoosing(t *testing.T) {
	f := &fakeReader{
		pairs: map[string][]store.Asset{
			"USTRY|" + testUSTRY.Issuer: {
				{ID: 7, Base: testUSTRY, Quote: testUSDC, Active: true},
				{ID: 8, Base: testUSTRY, Quote: testXLM, Active: true},
			},
		},
		latest: map[int]store.Metric{7: riskFixture()},
	}
	h := newTestServer(t, f)

	// THIS ASSERTION WAS INVERTED ON 11 SEPTEMBER 2026, and the old one is quoted
	// here because it is the whole point of the change. It read:
	//
	//   if rec.Code != http.StatusBadRequest {
	//       t.Fatalf("status = %d, want 400; the primary pair rule is not decided", rec.Code)
	//   }
	//
	// The rule WAS undecided when that was written and refusing to guess was
	// right. DEC-015 decided it on 5 September: the primary pair is USDC, always.
	// So an omitted quote now resolves, and it must resolve to pair 7, the USDC
	// one, rather than to whichever pair the store happened to return first.
	rec := get(t, h, BasePath+"/asset/"+ustryID+"/depth")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; the primary pair is USDC and pair 7 is it. body %s",
			rec.Code, rec.Body.String())
	}
	// Pair 8, the XLM one, has no metrics in this fake. Resolving to it would
	// answer 404 rather than 200, so a 200 here is already evidence that the USDC
	// pair was chosen. The header pins it a second way, from the fixture.
	if got := rec.Header().Get("X-Keel-Methodology-Version"); got != domain.MethodologyVersion {
		t.Errorf("methodology header = %q", got)
	}

	// Naming the quote explicitly resolves the same pair.
	rec = get(t, h, BasePath+"/asset/"+ustryID+"/depth?quote=USDC:"+testUSDC.Issuer)
	if rec.Code != http.StatusOK {
		t.Errorf("with an explicit quote: status = %d, want 200; body %s", rec.Code, rec.Body.String())
	}
}

// An asset with several pairs and NO USDC pair among them is the one case the
// ambiguity error still describes, and it is not a decided case: the candidate
// quote set in 02-pair-selection.md section 1 is exactly USDC and native XLM, so
// calling the XLM pair primary would assert a rule the methodology does not hold.
func TestOmittedQuoteWithNoUsdcPairStillRefusesToChoose(t *testing.T) {
	other := domain.Asset{Code: "EURC", Issuer: "GDHU6WRG4IEQXM5NZ4BMPKOXHW76MZM4Y2IEMFDVXBSDP6SJY4ITNPP2",
		Type: domain.AssetTypeAlphanum4}
	f := &fakeReader{
		pairs: map[string][]store.Asset{
			"USTRY|" + testUSTRY.Issuer: {
				{ID: 8, Base: testUSTRY, Quote: testXLM, Active: true},
				{ID: 9, Base: testUSTRY, Quote: other, Active: true},
			},
		},
		latest: map[int]store.Metric{8: riskFixture(), 9: riskFixture()},
	}
	h := newTestServer(t, f)

	rec := get(t, h, BasePath+"/asset/"+ustryID+"/depth")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; no pair here is the primary quote", rec.Code)
	}
	var body errorBodyJSON
	decodeBody(t, rec, &body)
	if body.Error.Detail["quoteCandidates"] == nil {
		t.Error("the error does not list the candidate quotes")
	}
	// The error says what it was looking for, not only that it failed. A consumer
	// that gets this back can act on it without reading the methodology.
	if got := body.Error.Detail["primaryQuote"]; got != domain.GlobalQuote().String() {
		t.Errorf("primaryQuote in the error = %#v, want the global quote identity", got)
	}
}

// The primary pair is matched on IDENTITY and never on the ticker. A pair quoted
// in some other issuer's USDC is a different asset and must not be mistaken for
// the primary: /assets?asset_code=USDC returns several issuers.
func TestPrimaryQuoteIsMatchedOnIssuerAndNotOnTheCode(t *testing.T) {
	impostor := domain.Asset{Code: "USDC", Issuer: "GBQBXAJPQBXAJPQBXAJPQBXAJPQBXAJPQBXAJPQBXAJPQBXAJPQBXAJP",
		Type: domain.AssetTypeAlphanum4}
	f := &fakeReader{
		pairs: map[string][]store.Asset{
			"USTRY|" + testUSTRY.Issuer: {
				{ID: 8, Base: testUSTRY, Quote: testXLM, Active: true},
				{ID: 9, Base: testUSTRY, Quote: impostor, Active: true},
			},
		},
		latest: map[int]store.Metric{8: riskFixture(), 9: riskFixture()},
	}

	rec := get(t, newTestServer(t, f), BasePath+"/asset/"+ustryID+"/depth")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; a USDC from another issuer is not the primary quote", rec.Code)
	}
}

func TestAMonitoredAssetAgainstAnUnmonitoredQuoteIs404(t *testing.T) {
	f := &fakeReader{
		pairs:  map[string][]store.Asset{"USTRY|" + testUSTRY.Issuer: {ustryPair(7)}},
		latest: map[int]store.Metric{7: riskFixture()},
	}
	rec := get(t, newTestServer(t, f), BasePath+"/asset/"+ustryID+"/depth?quote=XLM")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

// DEC-002 defers the historical path, so a ledger query must say it is
// unavailable rather than answer with a live figure wearing a historical label.
func TestAHistoricalRequestIs503WhileHubbleIsDeferred(t *testing.T) {
	f := &fakeReader{
		pairs:  map[string][]store.Asset{"USTRY|" + testUSTRY.Issuer: {ustryPair(7)}},
		latest: map[int]store.Metric{7: riskFixture()},
	}
	rec := get(t, newTestServer(t, f), BasePath+"/asset/"+ustryID+"/depth?ledger=61340263")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	var body errorBodyJSON
	decodeBody(t, rec, &body)
	if body.Error.Code != codeHistoricalUnavailable {
		t.Errorf("code = %q, want %s", body.Error.Code, codeHistoricalUnavailable)
	}
}

func TestAnUnreplayedLedgerIs404AndNot500(t *testing.T) {
	m := riskFixture()
	m.Risk.DataSource = domain.DataSourceOffersImplied
	f := &fakeReader{
		pairs: map[string][]store.Asset{"USTRY|" + testUSTRY.Issuer: {ustryPair(7)}},
		atLedger: map[string]store.Metric{
			"7|61340263|" + domain.MethodologyVersion + "|" + string(domain.DataSourceOffersImplied): m,
		},
	}
	s, err := New(Config{Reader: f, Params: domain.DefaultParams(), HistoricalAvailable: true,
		AllowedOrigins: []string{testOrigin}})
	if err != nil {
		t.Fatal(err)
	}
	h := s.Handler()

	rec := get(t, h, BasePath+"/asset/"+ustryID+"/depth?ledger=99999999")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	var body errorBodyJSON
	decodeBody(t, rec, &body)
	if body.Error.Code != codeLedgerNotAvailable {
		t.Errorf("code = %q, want %s", body.Error.Code, codeLedgerNotAvailable)
	}

	// And a ledger that WAS replayed carries a staleness of zero, because
	// historical data does not go stale.
	rec = get(t, h, BasePath+"/asset/"+ustryID+"/depth?ledger=61340263")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("X-Keel-Staleness-Seconds"); got != "0" {
		t.Errorf("staleness = %q, want 0 for a historical response", got)
	}
}

// ---------------------------------------------------------------- list

func TestAssetListFiltersAndPaginates(t *testing.T) {
	low := riskFixture()
	low.Risk.Base = testUSDC
	low.Risk.Band = domain.BandLow
	low.Risk.Flags = nil
	critical := riskFixture()

	f := &fakeReader{summaries: []store.Metric{low, critical}}
	h := newTestServer(t, f)

	rec := get(t, h, BasePath+"/assets")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body assetListJSON
	decodeBody(t, rec, &body)
	if len(body.Items) != 2 || body.Total != 2 {
		t.Fatalf("got %d items and total %d, want 2 and 2", len(body.Items), body.Total)
	}
	if body.Limit != 50 || body.Offset != 0 {
		t.Errorf("limit/offset = %d/%d, want the documented defaults 50 and 0", body.Limit, body.Offset)
	}
	// bandConfidence on the row, which the contract requires because the list is
	// where a band is read with the least context.
	if body.Items[0].BandConfidence == "" {
		t.Error("a list row carries no bandConfidence")
	}

	rec = get(t, h, BasePath+"/assets?band=CRITICAL")
	decodeBody(t, rec, &body)
	if len(body.Items) != 1 || body.Items[0].Band != "CRITICAL" {
		t.Errorf("band filter returned %d items", len(body.Items))
	}

	rec = get(t, h, BasePath+"/assets?hasFlag=ZERO_DEPTH_2PCT")
	decodeBody(t, rec, &body)
	if len(body.Items) != 1 {
		t.Errorf("flag filter returned %d items, want 1", len(body.Items))
	}
}

// A typo in a filter must not read as "no asset has this problem".
func TestUnknownFilterValuesAre400(t *testing.T) {
	h := newTestServer(t, &fakeReader{})
	for _, q := range []string{"?band=SEVERE", "?hasFlag=NOT_A_FLAG", "?limit=0", "?limit=500", "?limit=abc", "?offset=-1"} {
		rec := get(t, h, BasePath+"/assets"+q)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", q, rec.Code)
		}
	}
}

func TestDepth5PctIsPickedByValueNotByPosition(t *testing.T) {
	m := riskFixture()
	// A ladder in an unexpected order, and with the 5 percent rung carrying a
	// value nothing else has.
	m.Risk.Depth = []domain.DepthPoint{
		{Delta: d("0.10"), BuySide: d("10"), SellSide: d("0"), FromSdex: d("0"), FromAmm: d("0")},
		{Delta: d("0.05"), BuySide: d("5"), SellSide: d("0"), FromSdex: d("0"), FromAmm: d("0")},
		{Delta: d("0.02"), BuySide: d("2"), SellSide: d("0"), FromSdex: d("0"), FromAmm: d("0")},
	}
	f := &fakeReader{summaries: []store.Metric{m}}
	rec := get(t, newTestServer(t, f), BasePath+"/assets")

	var body assetListJSON
	decodeBody(t, rec, &body)
	if got := body.Items[0].Depth5PctBuySide; got == nil || *got != "5" {
		t.Errorf("depth5PctBuySide = %v, want 5 matched by delta and not by index", got)
	}

	// And a ladder missing that rung yields null, not the wrong rung.
	m.Risk.Depth = m.Risk.Depth[:1]
	f.summaries = []store.Metric{m}
	rec = get(t, newTestServer(t, f), BasePath+"/assets")
	decodeBody(t, rec, &body)
	if body.Items[0].Depth5PctBuySide != nil {
		t.Errorf("depth5PctBuySide = %v, want null when the rung is absent", *body.Items[0].Depth5PctBuySide)
	}
}

// ---------------------------------------------------------------- history

func TestHistoryDownsamplesBySelectingRealPoints(t *testing.T) {
	base := time.Date(2026, 2, 20, 0, 0, 0, 0, time.UTC)
	var rows []store.Metric
	// Three points on day one, one on day two, then a four day hole, then one.
	for i, offset := range []time.Duration{
		1 * time.Hour, 5 * time.Hour, 9 * time.Hour,
		26 * time.Hour,
		26*time.Hour + 4*24*time.Hour,
	} {
		m := riskFixture()
		m.Risk.LedgerSeq = uint32(61000000 + i)
		m.Risk.LedgerClosedAt = base.Add(offset)
		m.Risk.MidPrice = dp(strconv.Itoa(i))
		rows = append(rows, m)
	}

	f := &fakeReader{
		pairs:   map[string][]store.Asset{"USTRY|" + testUSTRY.Issuer: {ustryPair(7)}},
		history: map[int][]store.Metric{7: rows},
	}
	rec := get(t, newTestServer(t, f),
		BasePath+"/asset/"+ustryID+"/history?from=60999000&to=61001000&resolution=day")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body.String())
	}
	var body historyJSON
	decodeBody(t, rec, &body)

	if len(body.Points) != 3 {
		t.Fatalf("got %d points, want 3 days", len(body.Points))
	}
	// The LAST row in the first bucket, so every point on a chart is a real
	// result that can be fetched again at its own ledger.
	if body.Points[0].MidPrice == nil || *body.Points[0].MidPrice != "2" {
		t.Errorf("first point midPrice = %v, want the last row of day one", body.Points[0].MidPrice)
	}
	if len(body.Gaps) != 1 {
		t.Fatalf("got %d gaps, want 1 for the four day hole", len(body.Gaps))
	}
	if body.Gaps[0].Reason == "" {
		t.Error("the gap carries no reason")
	}
	if body.Resolution != "day" {
		t.Errorf("resolution = %q", body.Resolution)
	}
}

// ONE SERIES IS ONE DATA SOURCE, at the HTTP boundary.
//
// Before 26 August 2026 this endpoint asked the store for every source and then
// downsampled by keeping the last row in each bucket. 'trades-implied' sorts last
// of the four, so a ledger that had both a live reading and a reconstruction
// charted the reconstruction, which is a LOWER BOUND, as though it were the
// measurement. The response even labeled the whole series with the last row's
// source, so the label moved with the data instead of describing it.
func TestHistoryDefaultsToHorizonAndNeverMixesSources(t *testing.T) {
	base := time.Date(2026, 2, 20, 0, 0, 0, 0, time.UTC)
	var rows []store.Metric
	for i, src := range []domain.DataSource{
		domain.DataSourceHorizon, domain.DataSourceTradesImplied,
	} {
		m := riskFixture()
		m.Risk.LedgerSeq = uint32(61000000 + i)
		m.Risk.LedgerClosedAt = base.Add(time.Duration(i) * time.Hour)
		m.Risk.DataSource = src
		// 1 is the horizon row, 2 the trades-implied one, so the assertion below
		// names which row was charted rather than only how many.
		m.Risk.MidPrice = dp(strconv.Itoa(i + 1))
		rows = append(rows, m)
	}

	f := &fakeReader{
		pairs:   map[string][]store.Asset{"USTRY|" + testUSTRY.Issuer: {ustryPair(7)}},
		history: map[int][]store.Metric{7: rows},
	}
	rec := get(t, newTestServer(t, f),
		BasePath+"/asset/"+ustryID+"/history?from=60999000&to=61001000&resolution=day")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body.String())
	}
	if f.gotSource != domain.DataSourceHorizon {
		t.Errorf("the store was asked for %q, want %q by default", f.gotSource, domain.DataSourceHorizon)
	}

	var body historyJSON
	decodeBody(t, rec, &body)
	if body.DataSource != string(domain.DataSourceHorizon) {
		t.Errorf("dataSource = %q, want horizon", body.DataSource)
	}
	if len(body.Points) != 1 {
		t.Fatalf("got %d points, want 1: the two sources were charted as one series", len(body.Points))
	}
	if body.Points[0].MidPrice == nil || *body.Points[0].MidPrice != "1" {
		t.Errorf("the charted point midPrice = %v, want 1, the horizon row", body.Points[0].MidPrice)
	}
}

// The lower bound is still reachable, by name. Hiding it would be the opposite
// mistake: trades-implied is the source the Blend case study depends on.
func TestHistoryServesANamedSourceAndLabelsIt(t *testing.T) {
	m := riskFixture()
	m.Risk.DataSource = domain.DataSourceTradesImplied
	f := &fakeReader{
		pairs:   map[string][]store.Asset{"USTRY|" + testUSTRY.Issuer: {ustryPair(7)}},
		history: map[int][]store.Metric{7: {m}},
	}
	rec := get(t, newTestServer(t, f),
		BasePath+"/asset/"+ustryID+"/history?from=61340000&to=61341000&source=trades-implied")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body.String())
	}
	if f.gotSource != domain.DataSourceTradesImplied {
		t.Errorf("the store was asked for %q, want trades-implied", f.gotSource)
	}
	var body historyJSON
	decodeBody(t, rec, &body)
	if body.DataSource != "trades-implied" {
		t.Errorf("dataSource = %q, want the source that was asked for", body.DataSource)
	}
	if len(body.Points) != 1 {
		t.Errorf("got %d points, want 1", len(body.Points))
	}
}

// Every one of the four is accepted, checked against domain.DataSources rather
// than against a list written out here. A CHECK constraint that omitted
// offers-implied is a bug this repository already shipped once.
func TestHistoryAcceptsEveryDataSourceTheDomainDeclares(t *testing.T) {
	for _, src := range domain.DataSources() {
		f := &fakeReader{
			pairs:   map[string][]store.Asset{"USTRY|" + testUSTRY.Issuer: {ustryPair(7)}},
			history: map[int][]store.Metric{7: {}},
		}
		rec := get(t, newTestServer(t, f),
			BasePath+"/asset/"+ustryID+"/history?from=61340000&to=61341000&source="+string(src))
		if rec.Code != http.StatusOK {
			t.Errorf("source %q: status = %d, want 200; body %s", src, rec.Code, rec.Body.String())
		}
	}
}

func TestHistoryRejectsAnUnknownSource(t *testing.T) {
	f := &fakeReader{
		pairs:   map[string][]store.Asset{"USTRY|" + testUSTRY.Issuer: {ustryPair(7)}},
		history: map[int][]store.Metric{7: {}},
	}
	rec := get(t, newTestServer(t, f),
		BasePath+"/asset/"+ustryID+"/history?from=61340000&to=61341000&source=horizen")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for a misspelled source", rec.Code)
	}
}

// A cost to an unreachable target is not the cost of anything, and a chart cannot
// carry the reachable flag beside each point.
func TestHistoryOmitsAnUnreachableManipulationCost(t *testing.T) {
	m := riskFixture()
	m.Risk.ManipulationCostOrderbookOnly = []domain.ManipulationPoint{
		{Delta: d("0.5"), TargetPrice: d("80.8457121"), Cost: d("999"), Reachable: false},
	}
	f := &fakeReader{
		pairs:   map[string][]store.Asset{"USTRY|" + testUSTRY.Issuer: {ustryPair(7)}},
		history: map[int][]store.Metric{7: {m}},
	}
	rec := get(t, newTestServer(t, f),
		BasePath+"/asset/"+ustryID+"/history?from=61340000&to=61341000")

	var body historyJSON
	decodeBody(t, rec, &body)
	if len(body.Points) != 1 {
		t.Fatalf("got %d points", len(body.Points))
	}
	if body.Points[0].ManipulationCost50Pct != nil {
		t.Errorf("manipulationCost50Pct = %v, want null when the target is unreachable",
			*body.Points[0].ManipulationCost50Pct)
	}
}

func TestHistoryRejectsABadRange(t *testing.T) {
	f := &fakeReader{pairs: map[string][]store.Asset{"USTRY|" + testUSTRY.Issuer: {ustryPair(7)}}}
	h := newTestServer(t, f)

	cases := map[string]string{
		"no from":                 "?to=100",
		"no to":                   "?from=100",
		"reversed":                "?from=200&to=100",
		"longer than ninety days": "?from=1&to=99999999",
		"unknown resolution":      "?from=1&to=100&resolution=minute",
	}
	for name, q := range cases {
		t.Run(name, func(t *testing.T) {
			rec := get(t, h, BasePath+"/asset/"+ustryID+"/history"+q)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", rec.Code)
			}
			var body errorBodyJSON
			decodeBody(t, rec, &body)
			if body.Error.Code != codeInvalidRange {
				t.Errorf("code = %q, want %s", body.Error.Code, codeInvalidRange)
			}
		})
	}
}

// An empty range is a 200 with no points. There is no data yet for any asset, so
// this is the state the frontend will actually meet first.
func TestAnEmptyHistoryIs200WithEmptyArrays(t *testing.T) {
	f := &fakeReader{pairs: map[string][]store.Asset{"USTRY|" + testUSTRY.Issuer: {ustryPair(7)}}}
	rec := get(t, newTestServer(t, f), BasePath+"/asset/"+ustryID+"/history?from=1&to=100")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body map[string]any
	decodeBody(t, rec, &body)
	if body["points"] == nil || body["gaps"] == nil {
		t.Error("points or gaps arrived as null; both must be arrays")
	}
}

// ---------------------------------------------------------------- meta

func TestHealthIsDegradedWithNoScan(t *testing.T) {
	f := &fakeReader{assets: []store.Asset{ustryPair(7)}}
	rec := get(t, newTestServer(t, f), BasePath+"/health")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body healthJSON
	decodeBody(t, rec, &body)
	if body.Status != "degraded" {
		t.Errorf("status = %q, want degraded: an API serving no results must not report ok", body.Status)
	}
	if body.AssetsMonitored != 1 {
		t.Errorf("assetsMonitored = %d, want 1", body.AssetsMonitored)
	}
	if body.HistoricalAvailable {
		t.Error("historicalAvailable is true although Hubble is deferred")
	}
	if body.MethodologyVersion != domain.MethodologyVersion {
		t.Errorf("methodologyVersion = %q", body.MethodologyVersion)
	}
}

func TestHealthIsDegradedOnAnUnfinishedOrFailedScan(t *testing.T) {
	finished := time.Date(2026, 8, 24, 9, 0, 0, 0, time.UTC)

	unfinished := &store.Run{ID: 1, Kind: store.RunScan, StartedAt: finished}
	failed := &store.Run{ID: 2, Kind: store.RunScan, StartedAt: finished, FinishedAt: &finished, AssetsFailed: 1}
	clean := &store.Run{ID: 3, Kind: store.RunScan, StartedAt: finished, FinishedAt: &finished, AssetsOK: 1}

	for name, tc := range map[string]struct {
		run  *store.Run
		want string
	}{
		"a scan that never finished": {unfinished, "degraded"},
		"a scan with failures":       {failed, "degraded"},
		"a clean scan":               {clean, "ok"},
	} {
		t.Run(name, func(t *testing.T) {
			f := &fakeReader{
				assets:  []store.Asset{ustryPair(7)},
				latest:  map[int]store.Metric{7: riskFixture()},
				lastRun: tc.run,
			}
			rec := get(t, newTestServer(t, f), BasePath+"/health")
			var body healthJSON
			decodeBody(t, rec, &body)
			if body.Status != tc.want {
				t.Errorf("status = %q, want %q", body.Status, tc.want)
			}
		})
	}
}

func TestMethodologyReportsThresholdsAsStringsAndNamesItsUncalibrated(t *testing.T) {
	rec := get(t, newTestServer(t, &fakeReader{}), BasePath+"/methodology")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body map[string]any
	decodeBody(t, rec, &body)

	if body["calibrated"] != false {
		t.Errorf("calibrated = %#v, want false", body["calibrated"])
	}
	if body["version"] != domain.MethodologyVersion {
		t.Errorf("version = %#v", body["version"])
	}
	th, ok := body["thresholds"].(map[string]any)
	if !ok {
		t.Fatalf("thresholds = %#v", body["thresholds"])
	}
	if got, ok := th["spreadExtremePct"].(string); !ok || got != "20" {
		t.Errorf("spreadExtremePct = %#v, want a percent string", th["spreadExtremePct"])
	}
	if got, ok := th["oracleWindowSeconds"].(json.Number); !ok || got.String() != "900" {
		t.Errorf("oracleWindowSeconds = %#v, want 900 from DefaultParams", th["oracleWindowSeconds"])
	}
	// THIS ASSERTION WAS INVERTED ON 11 SEPTEMBER 2026. It read:
	//
	//   for _, key := range []string{"manipulationCheapUnit", "thinDepth5PctUnit"} {
	//       if _, present := th[key]; present {
	//           t.Errorf("%s is present; see the comment in handleMethodology and Q7", key)
	//       }
	//   }
	//
	// Both keys were withheld while Q7 was open, because there was no single unit
	// to name and the contract's example named the wrong one. DEC-015 closed Q7 on
	// 5 September: the quote asset is global and it is USDC. So both keys are
	// served, and the test now pins the VALUE rather than the absence.
	//
	// The value must be the (code, issuer) identity. A bare 'USDC' is the failure
	// this repository forbids by name: a consumer resolving that ticker itself can
	// land on a different asset than the one the thresholds are counted in.
	want := domain.GlobalQuote().String()
	for _, key := range []string{"manipulationCheapUnit", "thinDepth5PctUnit"} {
		got, present := th[key]
		if !present {
			t.Errorf("%s is absent; Q7 is closed and the unit is knowable", key)
			continue
		}
		if got != want {
			t.Errorf("%s = %#v, want %q", key, got, want)
		}
	}
	if !strings.Contains(want, ":G") {
		t.Fatalf("the global quote identity %q carries no issuer; a bare ticker must never be served here", want)
	}
}

func TestAnUnknownPathIsJSONAndNotHTML(t *testing.T) {
	rec := get(t, newTestServer(t, &fakeReader{}), BasePath+"/nope")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	var body errorBodyJSON
	decodeBody(t, rec, &body)
	if body.Error.Code == "" {
		t.Error("the 404 body is not the contract's error shape")
	}
}

// A storage failure is a 500 that says nothing about the schema.
func TestAStorageFailureDoesNotLeakItsMessage(t *testing.T) {
	f := &fakeReader{err: fmt.Errorf(`pq: column "manipulation_cost" does not exist`)}
	rec := get(t, newTestServer(t, f), BasePath+"/assets")

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "manipulation_cost") {
		t.Errorf("the response names a column: %s", rec.Body.String())
	}
}

// The store satisfies the Reader interface. This is a compile time assertion and
// it is the only thing tying this package to the concrete store.
var _ Reader = (*store.Store)(nil)

// TestTheHistoricalReadAsksForAKeyThatCanExist is the regression the fake was
// blind to until 5 September 2026.
//
// store.MetricsAtLedger requires all four parts of the key and, unlike
// LatestMetrics, does NOT default an empty version to the current one: its own
// comment says asking for an asset at a ledger without naming the version and the
// source is asking for several different rows. This handler passed an empty
// version and the hubble source, so the query was for rows whose
// methodology_version is literally ” and whose data_source is a source DEC-002
// holds. No such row can exist, so the path could only ever answer 404.
//
// The assertion is on what the handler ASKED FOR rather than on what came back,
// because a fake generous enough to answer a malformed request is exactly how the
// defect survived.
func TestTheHistoricalReadAsksForAKeyThatCanExist(t *testing.T) {
	m := riskFixture()
	m.Risk.DataSource = domain.DataSourceOffersImplied
	f := &fakeReader{
		pairs: map[string][]store.Asset{"USTRY|" + testUSTRY.Issuer: {ustryPair(7)}},
		atLedger: map[string]store.Metric{
			"7|61340262|" + domain.MethodologyVersion + "|" + string(domain.DataSourceOffersImplied): m,
		},
	}
	s, err := New(Config{Reader: f, Params: domain.DefaultParams(), HistoricalAvailable: true,
		AllowedOrigins: []string{testOrigin}})
	if err != nil {
		t.Fatal(err)
	}

	rec := get(t, s.Handler(), BasePath+"/asset/"+ustryID+"/depth?ledger=61340262")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body.String())
	}
	if f.askedVersion != domain.MethodologyVersion {
		t.Errorf("asked the store for methodology version %q, want %q. An empty version matches no row",
			f.askedVersion, domain.MethodologyVersion)
	}
	if f.askedLedgerSource != domain.DataSourceOffersImplied {
		t.Errorf("asked the store for source %q, want %q. hubble is held by DEC-002 and no row carries it",
			f.askedLedgerSource, domain.DataSourceOffersImplied)
	}
}

// TestTheHistoricalResponseSaysItIsAReconstruction guards the one thing that
// makes serving a rebuilt book honest. The contract's assetHistorical example
// carries dataSource offers-implied for exactly this reason: an offers-implied
// depth figure is not a measurement, and a consumer that cannot tell the two
// apart is being told a reconstruction is a reading.
func TestTheHistoricalResponseSaysItIsAReconstruction(t *testing.T) {
	m := riskFixture()
	m.Risk.DataSource = domain.DataSourceOffersImplied
	f := &fakeReader{
		pairs: map[string][]store.Asset{"USTRY|" + testUSTRY.Issuer: {ustryPair(7)}},
		atLedger: map[string]store.Metric{
			"7|61340262|" + domain.MethodologyVersion + "|" + string(domain.DataSourceOffersImplied): m,
		},
	}
	s, err := New(Config{Reader: f, Params: domain.DefaultParams(), HistoricalAvailable: true,
		AllowedOrigins: []string{testOrigin}})
	if err != nil {
		t.Fatal(err)
	}

	rec := get(t, s.Handler(), BasePath+"/asset/"+ustryID+"/depth?ledger=61340262")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body.String())
	}
	// Decoded into a map rather than the response struct, so the test reads the
	// JSON a consumer sees and not the Go type that produced it.
	var body map[string]any
	decodeBody(t, rec, &body)
	if got, _ := body["dataSource"].(string); got != string(domain.DataSourceOffersImplied) {
		t.Errorf("dataSource = %q, want %q so a reader can tell a rebuilt book from a measured one",
			got, domain.DataSourceOffersImplied)
	}
}

// ---------------------------------------------------------------- CORS

// testOrigin is the allowlisted origin every server in this file is built with.
// It is not a localhost spelling on purpose: a test that passes because the
// origin happens to be in DefaultCORSOrigins would keep passing if the
// allowlist stopped being consulted at all.
const testOrigin = "https://dashboard.keel.test"

// disallowedOrigin is a plausible attacker: same shape as testOrigin, one
// character of hostname different.
const disallowedOrigin = "https://dashboard.keel.test.evil.example"

// requestFrom sends one request with an Origin header, which httptest.NewRequest
// does not set for us. An empty origin sends none, which is what curl and every
// server-to-server client look like.
func requestFrom(t *testing.T, h http.Handler, method, path, origin string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if method == http.MethodOptions {
		// A real preflight carries these two, and they are what separates a
		// preflight from a bare OPTIONS request.
		req.Header.Set("Access-Control-Request-Method", http.MethodGet)
		req.Header.Set("Access-Control-Request-Headers", "Content-Type")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// assertVaryOrigin is called on every response in this section, allowed or not.
// See decision 1 on withCORS: the response with no Allow-Origin header is
// exactly the one a shared cache must not replay to an origin that would have
// been given one, so Vary being conditional would leave the dangerous case
// unmarked.
func assertVaryOrigin(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	for _, v := range rec.Header().Values("Vary") {
		if v == "Origin" {
			return
		}
	}
	t.Errorf("Vary = %q, want it to list Origin so no shared cache serves one origin's response to another",
		rec.Header().Values("Vary"))
}

func TestCORSAllowedOriginIsEchoedBack(t *testing.T) {
	f := &fakeReader{assets: []store.Asset{ustryPair(7)}}
	rec := requestFrom(t, newTestServer(t, f), http.MethodGet, BasePath+"/health", testOrigin)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body.String())
	}
	// THE REQUEST ORIGIN, not a wildcard and not a stored string. An allowlist
	// with more than one entry has one header to answer with, so the value has
	// to be the origin that asked.
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != testOrigin {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, testOrigin)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got == "*" {
		t.Error("Access-Control-Allow-Origin is a wildcard, which cannot be narrowed later without breaking every client")
	}
	assertVaryOrigin(t, rec)

	// Without Expose-Headers the browser reads null for both of these, and rule
	// 4 of this package's brief is satisfied on the wire and not in the consumer.
	expose := rec.Header().Get("Access-Control-Expose-Headers")
	for _, want := range []string{"X-Keel-Staleness-Seconds", "X-Keel-Methodology-Version"} {
		if !strings.Contains(expose, want) {
			t.Errorf("Access-Control-Expose-Headers = %q, want it to list %s, or the dashboard cannot read it",
				expose, want)
		}
	}

	// The API is unauthenticated and holds no cookies, so there is nothing for a
	// credentialed request to carry.
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Errorf("Access-Control-Allow-Credentials = %q, want it absent on an unauthenticated surface", got)
	}

	// Regression on middleware ordering: the CORS wrapper is outermost, and the
	// methodology header is set by the handler inside it.
	if got := rec.Header().Get("X-Keel-Methodology-Version"); got != domain.MethodologyVersion {
		t.Errorf("X-Keel-Methodology-Version = %q, want %q", got, domain.MethodologyVersion)
	}
}

// TestCORSDisallowedOriginStillGets200 is the test that says what this layer is
// NOT. CORS decides whether a browser hands the body to a page's JavaScript. It
// is not authentication, the API is public, and refusing the request here would
// only turn a browser-side rule into a server-side one that curl walks past.
func TestCORSDisallowedOriginStillGets200(t *testing.T) {
	f := &fakeReader{assets: []store.Asset{ustryPair(7)}}
	rec := requestFrom(t, newTestServer(t, f), http.MethodGet, BasePath+"/health", disallowedOrigin)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: CORS is not an authentication layer and must not answer one", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want it absent for an origin off the allowlist", got)
	}
	if got := rec.Header().Get("Access-Control-Expose-Headers"); got != "" {
		t.Errorf("Access-Control-Expose-Headers = %q, want it absent for an origin off the allowlist", got)
	}
	assertVaryOrigin(t, rec)

	// The body is served in full. Nothing about the allowlist changes what the
	// endpoint answers.
	var body map[string]any
	decodeBody(t, rec, &body)
	if body["status"] == nil {
		t.Error("the health body is empty for a disallowed origin; the API is public and answers everyone")
	}
}

func TestCORSPreflightIs204(t *testing.T) {
	f := &fakeReader{assets: []store.Asset{ustryPair(7)}}
	rec := requestFrom(t, newTestServer(t, f), http.MethodOptions, BasePath+"/health", testOrigin)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body %s", rec.Code, rec.Body.String())
	}
	if rec.Body.Len() != 0 {
		t.Errorf("preflight body = %q, want empty on a 204", rec.Body.String())
	}
	assertVaryOrigin(t, rec)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != testOrigin {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, testOrigin)
	}
	// GET and OPTIONS is the whole surface: every route is a GET and no method
	// here writes.
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got != "GET, OPTIONS" {
		t.Errorf("Access-Control-Allow-Methods = %q, want \"GET, OPTIONS\"", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); got != "Content-Type" {
		t.Errorf("Access-Control-Allow-Headers = %q, want \"Content-Type\"", got)
	}
	// A Max-Age is required rather than a particular number: without one the
	// browser preflights every single call.
	age := rec.Header().Get("Access-Control-Max-Age")
	if age == "" {
		t.Error("Access-Control-Max-Age is absent, so a browser preflights every request")
	}
	if n, err := strconv.Atoi(age); err != nil || n <= 0 {
		t.Errorf("Access-Control-Max-Age = %q, want a positive number of seconds", age)
	}
}

// A preflight from an origin off the allowlist is answered, and answered with
// nothing the browser can use. The 204 is the protocol; the absent
// Allow-Origin is the refusal.
func TestCORSPreflightFromDisallowedOriginGrantsNothing(t *testing.T) {
	f := &fakeReader{assets: []store.Asset{ustryPair(7)}}
	rec := requestFrom(t, newTestServer(t, f), http.MethodOptions, BasePath+"/health", disallowedOrigin)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	assertVaryOrigin(t, rec)
	for _, h := range []string{
		"Access-Control-Allow-Origin",
		"Access-Control-Allow-Methods",
		"Access-Control-Allow-Headers",
		"Access-Control-Max-Age",
	} {
		if got := rec.Header().Get(h); got != "" {
			t.Errorf("%s = %q, want it absent: a disallowed origin is granted nothing", h, got)
		}
	}
}

// A request with no Origin at all is curl, or any server-to-server client. It
// gets the ordinary answer and no CORS grant, and it still gets Vary, because
// the response it receives is the one that must not be replayed to a browser.
func TestCORSRequestWithNoOriginIsUntouched(t *testing.T) {
	f := &fakeReader{assets: []store.Asset{ustryPair(7)}}
	rec := requestFrom(t, newTestServer(t, f), http.MethodGet, BasePath+"/health", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want it absent when no Origin was sent", got)
	}
	assertVaryOrigin(t, rec)
}

// TestCORSMatchIsExact is the test that keeps the allowlist an allowlist. Every
// entry here is a form that a suffix match, a prefix match or a scheme-blind
// comparison would wave through.
func TestCORSMatchIsExact(t *testing.T) {
	f := &fakeReader{assets: []store.Asset{ustryPair(7)}}
	h := newTestServer(t, f)

	for _, origin := range []string{
		"http://dashboard.keel.test",           // the wrong scheme
		"https://dashboard.keel.test:8080",     // a port the allowlist does not name
		"https://evil.dashboard.keel.test",     // a subdomain of the allowed host
		"https://dashboard.keel.test.evil.com", // the allowed origin as a prefix
		"https://dashboard.keel.tes",           // one character short
		"*",
		"null", // what a browser sends from a file:// page or a sandboxed frame
	} {
		rec := requestFrom(t, h, http.MethodGet, BasePath+"/health", origin)
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("origin %q was allowed (header %q); the match must be byte for byte", origin, got)
		}
		assertVaryOrigin(t, rec)
	}
}

// ---------------------------------------------------------------- CORS config

// unsetCORSEnv removes KEEL_CORS_ORIGINS for the duration of one test and
// restores whatever was there before.
//
// The t.Setenv call is not redundant. There is no t.Unsetenv, and t.Setenv is
// what registers the cleanup that puts the variable back exactly as it was,
// including putting it back to unset. Setting it and then removing it is how a
// test reaches the genuinely-unset state without leaking into the next one.
func unsetCORSEnv(t *testing.T) {
	t.Helper()
	t.Setenv(CORSEnvVar, "placeholder")
	if err := os.Unsetenv(CORSEnvVar); err != nil {
		t.Fatalf("unsetenv %s: %v", CORSEnvVar, err)
	}
}

func newEnvServer(t *testing.T) http.Handler {
	t.Helper()
	s, err := New(Config{
		Reader: &fakeReader{assets: []store.Asset{ustryPair(7)}},
		Params: domain.DefaultParams(),
		// Nil, which is what sends New to the environment. This is the one
		// helper in this file that does that on purpose.
		AllowedOrigins: nil,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s.Handler()
}

func allowOriginFor(t *testing.T, h http.Handler, origin string) string {
	t.Helper()
	return requestFrom(t, h, http.MethodGet, BasePath+"/health", origin).
		Header().Get("Access-Control-Allow-Origin")
}

// The allowlist is configuration and never a literal in a handler, which is
// what "the allowed origins come from a flag or environment variable rather
// than being hardcoded" asks for.
func TestCORSAllowlistComesFromTheEnvironment(t *testing.T) {
	t.Setenv(CORSEnvVar, "https://keel.example, https://staging.keel.example/")
	h := newEnvServer(t)

	if got := allowOriginFor(t, h, "https://keel.example"); got != "https://keel.example" {
		t.Errorf("first entry: Allow-Origin = %q, want the origin echoed back", got)
	}
	// Whitespace around a comma is what a human writes in a compose file, and
	// the trailing slash can only ever have been meant as the origin without it,
	// because a browser never sends one.
	if got := allowOriginFor(t, h, "https://staging.keel.example"); got != "https://staging.keel.example" {
		t.Errorf("second entry: Allow-Origin = %q, want the trimmed origin echoed back", got)
	}
	// Setting the variable REPLACES the defaults rather than extending them.
	// A deployment that names its own origins is not also asking for localhost.
	if got := allowOriginFor(t, h, "http://localhost:5173"); got != "" {
		t.Errorf("localhost was allowed (%q) while %s named other origins; the variable replaces the defaults", got, CORSEnvVar)
	}
}

// With nothing set the allowlist is local development only, so a deployment
// that forgets the variable serves no browser rather than serving every browser.
func TestCORSDefaultsToLocalDevelopmentOnly(t *testing.T) {
	unsetCORSEnv(t)
	h := newEnvServer(t)

	defaults := DefaultCORSOrigins()
	if len(defaults) == 0 {
		t.Fatal("DefaultCORSOrigins is empty")
	}
	for _, o := range defaults {
		if !strings.Contains(o, "localhost") && !strings.Contains(o, "127.0.0.1") {
			t.Errorf("default origin %q is not a local one; a forgotten variable must not open a public origin", o)
		}
		if got := allowOriginFor(t, h, o); got != o {
			t.Errorf("default origin %q: Allow-Origin = %q, want it allowed", o, got)
		}
	}
	if got := allowOriginFor(t, h, "https://keel.example"); got != "" {
		t.Errorf("a public origin was allowed by default (%q)", got)
	}
}

// Set and empty is a statement, not a typo the code repairs: it is the only way
// to say "no browser at all". Unset means the defaults, which is the test above.
func TestCORSEmptyEnvironmentValueAllowsNothing(t *testing.T) {
	t.Setenv(CORSEnvVar, "")
	h := newEnvServer(t)

	for _, o := range append(DefaultCORSOrigins(), "https://keel.example") {
		if got := allowOriginFor(t, h, o); got != "" {
			t.Errorf("origin %q was allowed (%q) with %s set to empty, which means allow nothing", o, got, CORSEnvVar)
		}
	}
}

// A wildcard is refused AT STARTUP rather than ignored. Exact matching already
// fails closed on it, so the symptom of accepting it silently would be a
// dashboard whose every request fails with no header and no message.
func TestCORSWildcardIsRefusedAtStartup(t *testing.T) {
	for _, raw := range []string{"*", "https://keel.example,*"} {
		t.Setenv(CORSEnvVar, raw)
		_, err := New(Config{
			Reader: &fakeReader{},
			Params: domain.DefaultParams(),
		})
		if err == nil {
			t.Fatalf("%s=%q was accepted; a wildcard cannot be narrowed later without breaking every client", CORSEnvVar, raw)
		}
		if !strings.Contains(err.Error(), CORSEnvVar) {
			t.Errorf("error %q does not name %s, so an operator cannot tell what to fix", err, CORSEnvVar)
		}
	}
}
