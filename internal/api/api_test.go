package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	assets    []store.Asset
	pairs     map[string][]store.Asset // code|issuer -> pairs
	latest    map[int]store.Metric
	atLedger  map[string]store.Metric // assetID|ledger
	history   map[int][]store.Metric
	summaries []store.Metric
	total     int
	lastRun   *store.Run
	err       error
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

func (f *fakeReader) MetricsAtLedger(_ context.Context, assetID int, seq uint32, _ string, _ domain.DataSource) (store.Metric, error) {
	m, ok := f.atLedger[strconv.Itoa(assetID)+"|"+strconv.FormatUint(uint64(seq), 10)]
	if !ok {
		return store.Metric{}, fmt.Errorf("%w: ledger %d", store.ErrNotFound, seq)
	}
	return m, nil
}

func (f *fakeReader) MetricsHistory(_ context.Context, assetID int, from, to uint32, _ string, _ int) ([]store.Metric, error) {
	if f.err != nil {
		return nil, f.err
	}
	var out []store.Metric
	for _, m := range f.history[assetID] {
		if m.Risk.LedgerSeq >= from && m.Risk.LedgerSeq <= to {
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
		Logf:   func(string, ...any) {},
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

	rec := get(t, h, BasePath+"/asset/"+ustryID+"/depth")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; the primary pair rule is not decided", rec.Code)
	}
	var body errorBodyJSON
	decodeBody(t, rec, &body)
	if body.Error.Detail["quoteCandidates"] == nil {
		t.Error("the error does not list the candidate quotes")
	}

	// Naming the quote resolves it.
	rec = get(t, h, BasePath+"/asset/"+ustryID+"/depth?quote=USDC:"+testUSDC.Issuer)
	if rec.Code != http.StatusOK {
		t.Errorf("with an explicit quote: status = %d, want 200; body %s", rec.Code, rec.Body.String())
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
	m.Risk.DataSource = domain.DataSourceHubble
	f := &fakeReader{
		pairs:    map[string][]store.Asset{"USTRY|" + testUSTRY.Issuer: {ustryPair(7)}},
		atLedger: map[string]store.Metric{"7|61340263": m},
	}
	s, err := New(Config{Reader: f, Params: domain.DefaultParams(), HistoricalAvailable: true})
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
	// The two unit keys the contract's example asserts are deliberately absent,
	// because the thresholds are compared against quote-denominated notionals and
	// declaring them XLM would be wrong for every other pair. Open question Q7.
	for _, key := range []string{"manipulationCheapUnit", "thinDepth5PctUnit"} {
		if _, present := th[key]; present {
			t.Errorf("%s is present; see the comment in handleMethodology and Q7", key)
		}
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
