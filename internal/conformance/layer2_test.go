// Layer 2 conformance, docs/methodology/10-validation.md section 2.
//
// NONE OF THESE SCENARIOS CAN PASS TODAY and that is the state, not a defect in
// this file. testdata/fixtures/layer2/ does not exist. Each subtest SKIPS with
// the reason and the exact path it looked for, so `go test -v` prints ten skips
// naming ten files rather than a silent green.
//
// Skipping rather than failing is a deliberate choice and it has one cost worth
// stating: a skip is easy to stop noticing. That is why the tally is ALSO a
// finding in scripts/audit-verification.sh, which is where this repository puts
// gaps it does not want rediscovered. A test that fails on absent red zone work
// would leave CI red until somebody else does that work, and a red build that
// everyone has learned to ignore reports less than a skip that is counted
// somewhere.
package conformance

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/Keel-Official/keel-backend/internal/domain"
)

// TestLayer2 runs every scenario that has a file. For each one it checks, in
// this order: the fixture loads, the snapshot builds, the computation returns,
// the methodology's stated property holds, and the hand figures match.
//
// The order matters. A scenario whose Property fails is testing the wrong market
// and its expected values are beside the point, so the property is checked
// BEFORE the numbers and the failure says which of the two is wrong.
func TestLayer2(t *testing.T) {
	for _, sc := range Layer2Scenarios {
		t.Run(sc.Slug, func(t *testing.T) {
			f, err := LoadLayer2(Layer2Dir, sc)
			if errors.Is(err, ErrLayer2Absent) {
				t.Skipf("scenario %d not provided. Expected at %s/%02d-%s.json\n"+
					"  what it catches: %s\n"+
					"  Al creates the state on testnet, records the transaction, and works the expected values by hand",
					sc.ID, Layer2Dir, sc.ID, sc.Slug, sc.Discriminates)
			}
			if err != nil {
				t.Fatalf("loading scenario %d: %v", sc.ID, err)
			}

			s, err := f.Snapshot()
			if err != nil {
				t.Fatalf("scenario %d, building the snapshot: %v", sc.ID, err)
			}

			p := FixtureParams()
			risk, err := domain.ComputeAssetRisk(s, p)
			if err != nil {
				t.Fatalf("scenario %d, ComputeAssetRisk: %v", sc.ID, err)
			}

			if sc.Property != nil {
				if err := sc.Property(s, risk, p); err != nil {
					t.Errorf("scenario %d, %s\n  the methodology's stated property does not hold: %v",
						sc.ID, sc.Title, err)
				}
			}

			if f.Expected == nil {
				t.Logf("scenario %d has an input and no hand computation yet, so only its property was checked", sc.ID)
				return
			}
			checkLayer2Expected(t, sc, risk, f.Expected)
		})
	}
}

// checkLayer2Expected compares against the hand figures. Every comparison is
// against a value the fixture supplied; a nil field is not checked, because
// "not computed yet" must never read as "expected to be absent".
func checkLayer2Expected(t *testing.T, sc Layer2Scenario, r domain.AssetRisk, e *Layer2Expected) {
	t.Helper()

	if e.PriceSource != nil && string(r.PriceSource) != *e.PriceSource {
		t.Errorf("scenario %d priceSource = %q, fixture says %q", sc.ID, r.PriceSource, *e.PriceSource)
	}
	eqOptional(t, sc, "midPrice", r.MidPrice, e.MidPrice)
	eqOptional(t, sc, "spreadPct", r.SpreadPct, e.SpreadPct)
	eqOptional(t, sc, "maxReachablePrice", r.MaxReachablePrice, e.MaxReachablePrice)

	for _, row := range e.Depth {
		delta, err := decimal.NewFromString(row.Delta)
		if err != nil {
			t.Errorf("scenario %d: fixture delta %q is not a decimal: %v", sc.ID, row.Delta, err)
			continue
		}
		got, ok := depthAt(r, delta)
		if !ok {
			t.Errorf("scenario %d: the fixture expects a rung at delta %s and the result has none", sc.ID, row.Delta)
			continue
		}
		eqField(t, sc, "buySide", row.Delta, got.BuySide, row.BuySide)
		eqField(t, sc, "sellSide", row.Delta, got.SellSide, row.SellSide)
		eqField(t, sc, "fromSdex", row.Delta, got.FromSdex, row.FromSdex)
		eqField(t, sc, "fromAmm", row.Delta, got.FromAmm, row.FromAmm)
	}

	if e.Flags != nil {
		want := map[string]bool{}
		for _, f := range e.Flags {
			want[f] = true
		}
		got := map[string]bool{}
		for _, f := range r.Flags {
			got[string(f)] = true
		}
		for f := range want {
			if !got[f] {
				t.Errorf("scenario %d: fixture expects flag %s and it was not raised", sc.ID, f)
			}
		}
		for f := range got {
			if !want[f] {
				t.Errorf("scenario %d: flag %s was raised and the fixture does not list it", sc.ID, f)
			}
		}
	}
}

func depthAt(r domain.AssetRisk, delta decimal.Decimal) (domain.DepthPoint, bool) {
	for _, d := range r.Depth {
		if d.Delta.Equal(delta) {
			return d, true
		}
	}
	return domain.DepthPoint{}, false
}

// eqOptional compares a nullable result against a nullable expectation. A
// fixture that writes JSON null is claiming the field is absent, which is a
// claim worth checking rather than skipping.
func eqOptional(t *testing.T, sc Layer2Scenario, label string, got *decimal.Decimal, want *string) {
	t.Helper()
	if want == nil {
		return
	}
	if *want == "null" {
		if got != nil {
			t.Errorf("scenario %d %s = %s, fixture says null", sc.ID, label, got)
		}
		return
	}
	if got == nil {
		t.Errorf("scenario %d %s is absent, fixture says %s", sc.ID, label, *want)
		return
	}
	eqDec(t, label, *got, decimal.RequireFromString(*want))
}

func eqField(t *testing.T, sc Layer2Scenario, label, delta string, got decimal.Decimal, want *string) {
	t.Helper()
	if want == nil {
		return
	}
	eqDec(t, label+" at delta "+delta, got, decimal.RequireFromString(*want))
}

// TestLayer2Coverage reports the tally in one line, so a reader of the test
// output does not have to count ten skips to learn where Layer 2 stands.
//
// It does not fail on an incomplete Layer 2. The definition of done in section 6
// of the protocol is what that box belongs to, and scripts/audit-verification.sh
// is what reports it.
func TestLayer2Coverage(t *testing.T) {
	var provided, withNumbers int
	for _, sc := range Layer2Scenarios {
		f, err := LoadLayer2(Layer2Dir, sc)
		if errors.Is(err, ErrLayer2Absent) {
			continue
		}
		if err != nil {
			t.Errorf("scenario %d does not load: %v", sc.ID, err)
			continue
		}
		provided++
		if f.Expected != nil {
			withNumbers++
		}
	}
	t.Logf("Layer 2: %d of %d scenarios have an input, %d of those carry hand computed expectations. "+
		"docs/methodology/10-validation.md section 6 requires all %d",
		provided, len(Layer2Scenarios), withNumbers, len(Layer2Scenarios))
}

// TestLayer2ScenarioListMatchesTheProtocol guards the one thing in this package
// that can drift without anybody touching a fixture: the list itself.
//
// Ten scenarios, numbered 1 to 10, each slug distinct. If section 2 of the
// protocol ever gains an eleventh, this fails and the list gets updated rather
// than the eleventh being quietly untested.
func TestLayer2ScenarioListMatchesTheProtocol(t *testing.T) {
	const want = 10
	if len(Layer2Scenarios) != want {
		t.Errorf("the list holds %d scenarios, section 2 defines %d", len(Layer2Scenarios), want)
	}
	seenID := map[int]bool{}
	seenSlug := map[string]bool{}
	for i, sc := range Layer2Scenarios {
		if sc.ID != i+1 {
			t.Errorf("position %d holds scenario ID %d; the list must follow section 2's numbering", i, sc.ID)
		}
		if seenID[sc.ID] {
			t.Errorf("scenario ID %d appears twice", sc.ID)
		}
		if seenSlug[sc.Slug] {
			t.Errorf("slug %q appears twice, so two scenarios would read the same file", sc.Slug)
		}
		seenID[sc.ID], seenSlug[sc.Slug] = true, true
		if sc.Title == "" || sc.Discriminates == "" {
			t.Errorf("scenario %d is missing its title or its discriminating reason", sc.ID)
		}
	}
}

// ---------------------------------------------------------------- harness self-test

// The tests below prove the CONTAINER works. They are not Layer 2 scenarios and
// their figures are not methodology figures: they are arbitrary synthetic values
// chosen to exercise the loader and one property, the same way
// internal/domain/compute_test.go builds synthetic books. No number here is
// compared against a hand computation and none may ever be reused as one.
//
// They exist because of the pattern CLAUDE.md records being defeated by: a check
// that has never fired proves nothing, and this harness would otherwise sit
// unexecuted until the day somebody drops ten real fixtures into it and meets
// every bug in the loader at once.

func writeLayer2(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}
}

// validBody is one active pool and one ask. Scenario 9's shape, with invented
// reserves.
const validBody = `{
  "scenario": 9,
  "slug": "active-pool-nulls-max-reachable",
  "testnetTx": "0000000000000000000000000000000000000000000000000000000000000000",
  "ledgerSeq": 1,
  "ledgerClosedAt": "2026-08-28T00:00:00Z",
  "base":  {"code": "TEST", "issuer": "GTEST", "type": "credit_alphanum4"},
  "quote": {"code": "USDC", "issuer": "GUSDC", "type": "credit_alphanum4"},
  "book":  {"bids": [{"priceN": 9, "priceD": 10, "amount": "5"}],
            "asks": [{"priceN": 11, "priceD": 10, "amount": "5"}]},
  "pools": [{"poolId": "p1", "reserveBase": "1000", "reserveQuote": "1000", "feeBp": 30}]
}`

func TestLayer2HarnessLoadsAndConverts(t *testing.T) {
	dir := t.TempDir()
	sc := Layer2Scenarios[8] // scenario 9
	writeLayer2(t, dir, "09-active-pool-nulls-max-reachable.json", validBody)

	f, err := LoadLayer2(dir, sc)
	if err != nil {
		t.Fatalf("loading a well formed fixture: %v", err)
	}
	s, err := f.Snapshot()
	if err != nil {
		t.Fatalf("converting to a snapshot: %v", err)
	}
	if len(s.ActivePools()) != 1 {
		t.Fatalf("active pools = %d, want 1", len(s.ActivePools()))
	}
	if len(s.Book.Asks) != 1 || s.Book.Asks[0].Price.N != 11 {
		t.Fatalf("the ask did not survive the conversion: %+v", s.Book.Asks)
	}

	risk, err := domain.ComputeAssetRisk(s, FixtureParams())
	if err != nil {
		t.Fatalf("ComputeAssetRisk on the loaded snapshot: %v", err)
	}
	if err := sc.Property(s, risk, FixtureParams()); err != nil {
		t.Errorf("the scenario 9 property rejected a market it should accept: %v", err)
	}
}

// TestLayer2HarnessPropertyCanFail is the half that matters. A property that
// cannot fail is decoration.
func TestLayer2HarnessPropertyCanFail(t *testing.T) {
	dir := t.TempDir()
	sc := Layer2Scenarios[8]
	// The same fixture with the pool removed. Scenario 9 is about an active pool
	// being present, so with none the property must refuse the input rather than
	// pass it.
	writeLayer2(t, dir, "09-active-pool-nulls-max-reachable.json",
		strings.Replace(validBody,
			`"pools": [{"poolId": "p1", "reserveBase": "1000", "reserveQuote": "1000", "feeBp": 30}]`,
			`"pools": []`, 1))

	f, err := LoadLayer2(dir, sc)
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	s, err := f.Snapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	risk, err := domain.ComputeAssetRisk(s, FixtureParams())
	if err != nil {
		t.Fatalf("ComputeAssetRisk: %v", err)
	}
	if err := sc.Property(s, risk, FixtureParams()); err == nil {
		t.Error("the scenario 9 property accepted a market with no active pool, so it would pass a fixture testing nothing")
	}
}

// TestLayer2HarnessRejectsMalformedFixtures. Each case is a way a hand written
// fixture goes wrong, and every one of them must be an error rather than a
// silent zero.
func TestLayer2HarnessRejectsMalformedFixtures(t *testing.T) {
	sc := Layer2Scenarios[8]
	cases := []struct {
		name, body, wantIn string
	}{
		{"unknown field", strings.Replace(validBody, `"scenario": 9,`, `"scenario": 9, "expcted": {},`, 1), "expcted"},
		{"empty testnetTx", strings.Replace(validBody, `"testnetTx": "0000000000000000000000000000000000000000000000000000000000000000"`, `"testnetTx": ""`, 1), "testnetTx is empty"},
		{"scenario id mismatch", strings.Replace(validBody, `"scenario": 9,`, `"scenario": 3,`, 1), "file says scenario 3"},
		{"slug mismatch", strings.Replace(validBody, `"slug": "active-pool-nulls-max-reachable"`, `"slug": "something-else"`, 1), "slug"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			writeLayer2(t, dir, "09-active-pool-nulls-max-reachable.json", c.body)
			_, err := LoadLayer2(dir, sc)
			if err == nil {
				t.Fatalf("a fixture with %s loaded without complaint", c.name)
			}
			if !strings.Contains(err.Error(), c.wantIn) {
				t.Errorf("error %q does not mention %q, so the message would not point at the mistake", err, c.wantIn)
			}
		})
	}
}

// An empty decimal string must not read as zero. On a hand computation the
// difference between "not filled in" and "computed to be zero" is the whole
// value of the exercise.
func TestLayer2HarnessRefusesEmptyDecimals(t *testing.T) {
	dir := t.TempDir()
	sc := Layer2Scenarios[8]
	writeLayer2(t, dir, "09-active-pool-nulls-max-reachable.json",
		strings.Replace(validBody, `"amount": "5"`, `"amount": ""`, 1))

	f, err := LoadLayer2(dir, sc)
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	if _, err := f.Snapshot(); err == nil {
		t.Error("an empty amount converted without complaint, so a blank in a fixture would read as zero")
	}
}
