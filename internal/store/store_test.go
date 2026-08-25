package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/Keel-Official/keel-backend/internal/domain"
)

// These tests need a real Postgres, because what they are checking is what
// Postgres does with the values: whether an unqualified NUMERIC keeps forty
// digits, whether a CHECK constraint actually fires, whether ON CONFLICT DO
// NOTHING leaves the stored row alone. A fake would only assert that this
// package sends the SQL it was written to send.
//
// They are skipped when KEEL_TEST_DSN is unset, so `make test` stays green with
// no Docker. To run them:
//
//	make up && make migrate
//	KEEL_TEST_DSN="postgres://keel:keel_dev_only@localhost:5432/keel?sslmode=disable" go test ./internal/store/
//
// EVERY TEST RUNS INSIDE A TRANSACTION THAT IS ROLLED BACK. That is what the
// unexported dbtx interface in store.go is for. A test suite that leaves rows in
// a database somebody is also developing against is a suite people stop running.
func testStore(t *testing.T) (*Store, context.Context) {
	t.Helper()
	dsn := os.Getenv("KEEL_TEST_DSN")
	if dsn == "" {
		t.Skip("KEEL_TEST_DSN is unset; see the comment in store_test.go")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		_ = db.Close()
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() {
		_ = tx.Rollback()
		_ = db.Close()
	})
	return &Store{db: tx}, ctx
}

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

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }
func decp(s string) *decimal.Decimal {
	d := dec(s)
	return &d
}

// fullRisk is a result with every field populated, and with decimals chosen to
// be awkward rather than round. spreadPct is the golden fixture's own value,
// whose decimal expansion never terminates; the forty digit figure is there to
// prove an unqualified NUMERIC rounds nothing on the way in.
func fullRisk() domain.AssetRisk {
	closed := time.Date(2026, 2, 22, 0, 10, 21, 0, time.UTC)
	return domain.AssetRisk{
		Base:               testUSTRY,
		Quote:              testUSDC,
		LedgerSeq:          61340263,
		LedgerClosedAt:     closed,
		MethodologyVersion: domain.MethodologyVersion,
		DataSource:         domain.DataSourceOffersImplied,

		MidPrice:           decp("53.8971414"),
		PriceSource:        domain.PriceSourceBook,
		SpreadPct:          decp("196.0777140585047799956232929266263460867"),
		PoolSpotPrice:      decp("1.0555404901234567890123456789012345678"),
		PriceDivergencePct: decp("-98.0416666666666666666666666666666666667"),

		Depth: []domain.DepthPoint{
			{Delta: dec("0.02"), BuySide: dec("0"), SellSide: dec("0"), FromSdex: dec("0"), FromAmm: dec("0")},
			{Delta: dec("0.10"), BuySide: dec("106.4300001"), SellSide: dec("0.0001057"), FromSdex: dec("0"), FromAmm: dec("106.4300001")},
		},
		ManipulationCostCombined: []domain.ManipulationPoint{
			{Delta: dec("0.5"), TargetPrice: dec("80.8457121"), Cost: dec("127.0300000"), Reachable: true},
			{Delta: dec("100"), TargetPrice: dec("5443.6112814"), Cost: dec("1290.5600000"), Reachable: false},
		},
		ManipulationCostOrderbookOnly: []domain.ManipulationPoint{
			{Delta: dec("0.5"), TargetPrice: dec("80.8457121"), Cost: dec("0"), Reachable: true},
			{Delta: dec("100"), TargetPrice: dec("5443.6112814"), Cost: dec("130.06270929502336"), Reachable: false},
		},

		MaxReachablePrice:       decp("106.7372828"),
		CostToMaxReachablePrice: decp("0"),

		OracleResistance: &domain.OracleResistance{
			CriticalDelta:    dec("0.5"),
			ManipulationCost: dec("0"),
			Reachable:        true,
			GenuineVolume:    dec("5.3475699"),
			WindowSeconds:    900,
			Ratio:            decp("0"),
			TotalAttackCost:  decp("5.3475699"),
		},

		MaxSafeCollateral:             decp("0"),
		MaxSafeCollateralLiquidation:  decp("0.00005285"),
		MaxSafeCollateralManipulation: decp("0"),

		Supporting: domain.SupportingMetrics{
			HolderTop1Pct:     decp("99.9999990000000001"),
			HolderTop10Pct:    decp("100"),
			HolderHHI:         decp("9999.99998"),
			VolumeToSupplyD1:  decp("0.0000001"),
			VolumeToSupplyD7:  decp("0.0000002"),
			VolumeToSupplyD30: decp("0.0000003"),
			LastGenuineTrade:  &domain.TradeRef{LedgerSeq: 61340263, At: closed},
			TradesExcludedPct: decp("87.5"),
		},

		Flags:            []domain.Flag{domain.FlagZeroDepth2Pct, domain.FlagManipulationCheap},
		UnevaluatedFlags: []domain.Flag{domain.FlagNoGenuineTrade30D},
		Band:             domain.BandCritical,
		BandConfidence:   domain.BandConfidencePartial,
		// Free text, and it goes into a text[]. The comma, the braces, the
		// quotes and the backslash are all Postgres array syntax, so this is the
		// string that finds out whether the driver quotes properly.
		Warnings: []string{
			`manipulation term not applied: target unreachable`,
			`odd, {braced}, "quoted" and back\slashed`,
		},
	}
}

func seedAsset(t *testing.T, s *Store, ctx context.Context) int {
	t.Helper()
	id, err := s.UpsertAsset(ctx, testUSTRY, testUSDC, "the Blend case study asset")
	if err != nil {
		t.Fatalf("UpsertAsset: %v", err)
	}
	return id
}

// ---------------------------------------------------------------- metrics

func TestSaveAndReadBackIsExact(t *testing.T) {
	s, ctx := testStore(t)
	assetID := seedAsset(t, s, ctx)
	want := fullRisk()
	computedAt := time.Date(2026, 8, 24, 9, 0, 0, 0, time.UTC)

	id, inserted, err := s.SaveMetrics(ctx, assetID, computedAt, want)
	if err != nil {
		t.Fatalf("SaveMetrics: %v", err)
	}
	if !inserted || id == 0 {
		t.Fatalf("inserted = %t, id = %d, want true and non-zero", inserted, id)
	}

	got, err := s.LatestMetrics(ctx, assetID, "")
	if err != nil {
		t.Fatalf("LatestMetrics: %v", err)
	}
	if !got.ComputedAt.Equal(computedAt) {
		t.Errorf("ComputedAt = %s, want %s", got.ComputedAt, computedAt)
	}
	assertRiskEqual(t, got.Risk, want)
}

// The point of the previous test in one line, kept separate so a failure says
// which property broke. An unqualified NUMERIC is arbitrary precision, and
// 0001_core.sql says that is why it is unqualified.
func TestNumericKeepsEveryDigit(t *testing.T) {
	s, ctx := testStore(t)
	assetID := seedAsset(t, s, ctx)
	risk := fullRisk()
	const forty = "196.0777140585047799956232929266263460867"
	risk.SpreadPct = decp(forty)

	if _, _, err := s.SaveMetrics(ctx, assetID, time.Now().UTC(), risk); err != nil {
		t.Fatalf("SaveMetrics: %v", err)
	}
	got, err := s.LatestMetrics(ctx, assetID, "")
	if err != nil {
		t.Fatalf("LatestMetrics: %v", err)
	}
	if got.Risk.SpreadPct.String() != forty {
		t.Errorf("spread_pct came back as %s, want %s", got.Risk.SpreadPct, forty)
	}
}

// Every nullable column means unknown or not applicable, never zero. This is the
// single most important property in the package: a nil manipulation term says
// the attack is impossible, and zero says it is free.
func TestNullMeansUnknownAndNeverZero(t *testing.T) {
	s, ctx := testStore(t)
	assetID := seedAsset(t, s, ctx)

	risk := domain.AssetRisk{
		Base:               testUSTRY,
		Quote:              testUSDC,
		LedgerSeq:          61340264,
		LedgerClosedAt:     time.Date(2026, 2, 22, 0, 10, 26, 0, time.UTC),
		MethodologyVersion: domain.MethodologyVersion,
		DataSource:         domain.DataSourceHorizon,
		PriceSource:        domain.PriceSourceNone,
		Depth:              nil,
		Band:               domain.BandCritical,
		BandConfidence:     domain.BandConfidencePartial,
	}
	if _, _, err := s.SaveMetrics(ctx, assetID, time.Now().UTC(), risk); err != nil {
		t.Fatalf("SaveMetrics: %v", err)
	}
	got, err := s.LatestMetrics(ctx, assetID, "")
	if err != nil {
		t.Fatalf("LatestMetrics: %v", err)
	}

	nils := map[string]*decimal.Decimal{
		"MidPrice":                      got.Risk.MidPrice,
		"SpreadPct":                     got.Risk.SpreadPct,
		"PoolSpotPrice":                 got.Risk.PoolSpotPrice,
		"PriceDivergencePct":            got.Risk.PriceDivergencePct,
		"MaxReachablePrice":             got.Risk.MaxReachablePrice,
		"CostToMaxReachablePrice":       got.Risk.CostToMaxReachablePrice,
		"MaxSafeCollateral":             got.Risk.MaxSafeCollateral,
		"MaxSafeCollateralLiquidation":  got.Risk.MaxSafeCollateralLiquidation,
		"MaxSafeCollateralManipulation": got.Risk.MaxSafeCollateralManipulation,
		"HolderTop1Pct":                 got.Risk.Supporting.HolderTop1Pct,
		"HolderTop10Pct":                got.Risk.Supporting.HolderTop10Pct,
		"HolderHHI":                     got.Risk.Supporting.HolderHHI,
		"VolumeToSupplyD1":              got.Risk.Supporting.VolumeToSupplyD1,
		"TradesExcludedPct":             got.Risk.Supporting.TradesExcludedPct,
	}
	for name, v := range nils {
		if v != nil {
			t.Errorf("%s = %s, want nil; a NULL must not become a number", name, v)
		}
	}
	if got.Risk.OracleResistance != nil {
		t.Error("OracleResistance is non-nil although nothing was stored")
	}
	if got.Risk.Supporting.LastGenuineTrade != nil {
		t.Error("LastGenuineTrade is non-nil although nothing was stored")
	}
	if got.Risk.ManipulationCostOrderbookOnly != nil {
		t.Error("ManipulationCostOrderbookOnly is non-nil although nothing was stored")
	}
	// depth is NOT NULL, so an empty ladder comes back as an empty ladder.
	if len(got.Risk.Depth) != 0 {
		t.Errorf("Depth has %d points, want 0", len(got.Risk.Depth))
	}
}

func TestSaveIsNeverAnOverwrite(t *testing.T) {
	s, ctx := testStore(t)
	assetID := seedAsset(t, s, ctx)
	first := fullRisk()

	if _, inserted, err := s.SaveMetrics(ctx, assetID, time.Now().UTC(), first); err != nil || !inserted {
		t.Fatalf("first save: inserted = %t, err = %v", inserted, err)
	}

	// Same key, different numbers. This is what a re-run with changed code looks
	// like, and it must not silently rewrite the stored evidence.
	second := fullRisk()
	second.MidPrice = decp("999.9999999")
	id, inserted, err := s.SaveMetrics(ctx, assetID, time.Now().UTC(), second)
	if err != nil {
		t.Fatalf("second save: %v", err)
	}
	if inserted {
		t.Error("inserted = true for a key that already existed")
	}
	if id != 0 {
		t.Errorf("id = %d, want 0 when nothing was written", id)
	}

	got, err := s.LatestMetrics(ctx, assetID, "")
	if err != nil {
		t.Fatalf("LatestMetrics: %v", err)
	}
	if got.Risk.MidPrice.String() != "53.8971414" {
		t.Errorf("mid price is now %s; the stored row was overwritten", got.Risk.MidPrice)
	}
}

// Rule 3 of this package's brief, as behavior rather than prose.
func TestVersionAndSourceAreEachPartOfTheKey(t *testing.T) {
	s, ctx := testStore(t)
	assetID := seedAsset(t, s, ctx)
	base := fullRisk()

	for _, mod := range []func(r *domain.AssetRisk){
		func(r *domain.AssetRisk) {},
		func(r *domain.AssetRisk) { r.MethodologyVersion = "1.0.4-draft" },
		func(r *domain.AssetRisk) { r.DataSource = domain.DataSourceHubble },
	} {
		risk := base
		mod(&risk)
		if _, inserted, err := s.SaveMetrics(ctx, assetID, time.Now().UTC(), risk); err != nil || !inserted {
			t.Fatalf("save: inserted = %t, err = %v", inserted, err)
		}
	}

	// Three rows for one asset at one ledger, and the version filter separates
	// them, which is what makes cross-validation a single query.
	current, err := s.MetricsHistory(ctx, assetID, 0, 99999999, domain.MethodologyVersion, 0)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(current) != 2 {
		t.Errorf("got %d rows at the current version, want 2 (two sources)", len(current))
	}
	next, err := s.MetricsHistory(ctx, assetID, 0, 99999999, "1.0.4-draft", 0)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(next) != 1 {
		t.Errorf("got %d rows at 1.0.4-draft, want 1", len(next))
	}
}

// The gap documented at the top of metrics.go, asserted so that closing it
// breaks this test and whoever closes it updates the store.
func TestGenuineVolumeInWindowIsNotPersisted(t *testing.T) {
	s, ctx := testStore(t)
	assetID := seedAsset(t, s, ctx)
	risk := fullRisk()
	risk.Supporting.GenuineVolumeInWindow = decp("5.3475699")

	if _, _, err := s.SaveMetrics(ctx, assetID, time.Now().UTC(), risk); err != nil {
		t.Fatalf("SaveMetrics: %v", err)
	}
	got, err := s.LatestMetrics(ctx, assetID, "")
	if err != nil {
		t.Fatalf("LatestMetrics: %v", err)
	}
	if got.Risk.Supporting.GenuineVolumeInWindow != nil {
		t.Fatal("GenuineVolumeInWindow survived the round trip. If a column was added, " +
			"update SaveMetrics and metricColumns, and see handoff item 17")
	}
	// The same quantity IS stored, inside the oracle object, which is the reason
	// the column is not missed.
	if got.Risk.OracleResistance == nil || got.Risk.OracleResistance.GenuineVolume.String() != "5.3475699" {
		t.Error("oracle_resistance.genuineVolume did not survive, so the quantity is stored nowhere")
	}
}

func TestPriceSourceNoneRejectsAMidPrice(t *testing.T) {
	s, ctx := testStore(t)
	assetID := seedAsset(t, s, ctx)
	risk := fullRisk()
	risk.PriceSource = domain.PriceSourceNone

	if _, _, err := s.SaveMetrics(ctx, assetID, time.Now().UTC(), risk); err == nil {
		t.Fatal("a none price source with a mid price was accepted")
	}
}

func TestSaveRejectsWhatTheCheckConstraintsWould(t *testing.T) {
	s, ctx := testStore(t)
	assetID := seedAsset(t, s, ctx)

	cases := map[string]func(r *domain.AssetRisk){
		"an empty methodology version": func(r *domain.AssetRisk) { r.MethodologyVersion = "" },
		"a zero ledger sequence":       func(r *domain.AssetRisk) { r.LedgerSeq = 0 },
		"a data source outside the four": func(r *domain.AssetRisk) {
			r.DataSource = domain.DataSource("guessed")
		},
		"a band outside the four": func(r *domain.AssetRisk) { r.Band = domain.Band("SEVERE") },
		"an unknown price source": func(r *domain.AssetRisk) { r.PriceSource = domain.PriceSource("mid") },
		"an unknown band confidence": func(r *domain.AssetRisk) {
			r.BandConfidence = domain.BandConfidence("mostly")
		},
	}
	for name, mod := range cases {
		t.Run(name, func(t *testing.T) {
			risk := fullRisk()
			mod(&risk)
			if _, _, err := s.SaveMetrics(ctx, assetID, time.Now().UTC(), risk); err == nil {
				t.Errorf("accepted %s", name)
			}
		})
	}
}

func TestMetricsHistoryIsOldestFirstAndBounded(t *testing.T) {
	s, ctx := testStore(t)
	assetID := seedAsset(t, s, ctx)

	for _, seq := range []uint32{61340265, 61340263, 61340264} {
		risk := fullRisk()
		risk.LedgerSeq = seq
		if _, _, err := s.SaveMetrics(ctx, assetID, time.Now().UTC(), risk); err != nil {
			t.Fatalf("save %d: %v", seq, err)
		}
	}

	all, err := s.MetricsHistory(ctx, assetID, 0, 99999999, "", 0)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("got %d rows, want 3", len(all))
	}
	for i := 1; i < len(all); i++ {
		if all[i-1].Risk.LedgerSeq > all[i].Risk.LedgerSeq {
			t.Fatalf("history is not oldest first: %d before %d",
				all[i-1].Risk.LedgerSeq, all[i].Risk.LedgerSeq)
		}
	}
	// The range is inclusive at both ends and on LEDGER, not on wall clock.
	window, err := s.MetricsHistory(ctx, assetID, 61340264, 61340265, "", 0)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(window) != 2 {
		t.Errorf("got %d rows in the range, want 2", len(window))
	}
	// And the newest is what LatestMetrics returns.
	latest, err := s.LatestMetrics(ctx, assetID, "")
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	if latest.Risk.LedgerSeq != 61340265 {
		t.Errorf("latest ledger = %d, want 61340265", latest.Risk.LedgerSeq)
	}
}

func TestMetricsAtLedgerNeedsAllFourKeyParts(t *testing.T) {
	s, ctx := testStore(t)
	assetID := seedAsset(t, s, ctx)
	risk := fullRisk()
	if _, _, err := s.SaveMetrics(ctx, assetID, time.Now().UTC(), risk); err != nil {
		t.Fatalf("save: %v", err)
	}

	if _, err := s.MetricsAtLedger(ctx, assetID, risk.LedgerSeq, risk.MethodologyVersion, risk.DataSource); err != nil {
		t.Fatalf("exact read: %v", err)
	}
	// Right ledger, wrong source: a different row, and there is none.
	_, err := s.MetricsAtLedger(ctx, assetID, risk.LedgerSeq, risk.MethodologyVersion, domain.DataSourceHubble)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestMissingMetricsIsErrNotFoundAndNotAZeroValue(t *testing.T) {
	s, ctx := testStore(t)
	assetID := seedAsset(t, s, ctx)

	if _, err := s.LatestMetrics(ctx, assetID, ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// ---------------------------------------------------------------- assets

func TestUpsertAssetIsIdempotentAndKeepsTheNote(t *testing.T) {
	s, ctx := testStore(t)

	first, err := s.UpsertAsset(ctx, testUSTRY, testUSDC, "the Blend case study asset")
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	// A re-run with no note must not erase the reason the asset is in the set.
	second, err := s.UpsertAsset(ctx, testUSTRY, testUSDC, "")
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if first != second {
		t.Errorf("ids differ: %d then %d; the pair was inserted twice", first, second)
	}

	assets, err := s.Assets(ctx, true)
	if err != nil {
		t.Fatalf("Assets: %v", err)
	}
	var found bool
	for _, a := range assets {
		if a.ID == first {
			found = true
			if a.SelectionNote != "the Blend case study asset" {
				t.Errorf("selection note is now %q; the reason was erased", a.SelectionNote)
			}
		}
	}
	if !found {
		t.Error("the upserted asset is not in the active list")
	}
}

func TestAssetsRoundTripBothAssetTypes(t *testing.T) {
	s, ctx := testStore(t)
	// XLM as the base exercises the native path, where the issuer must be NULL
	// and not an empty string, or assets_native_has_no_issuer rejects the row.
	id, err := s.UpsertAsset(ctx, testXLM, testUSDC, "native base")
	if err != nil {
		t.Fatalf("UpsertAsset native: %v", err)
	}
	assets, err := s.Assets(ctx, true)
	if err != nil {
		t.Fatalf("Assets: %v", err)
	}
	for _, a := range assets {
		if a.ID != id {
			continue
		}
		if !a.Base.Equal(testXLM) {
			t.Errorf("base came back as %+v, want %+v", a.Base, testXLM)
		}
		if !a.Quote.Equal(testUSDC) {
			t.Errorf("quote came back as %+v, want %+v", a.Quote, testUSDC)
		}
		return
	}
	t.Fatal("the native pair is missing from the list")
}

func TestUpsertAssetRejectsBadInput(t *testing.T) {
	s, ctx := testStore(t)
	cases := map[string][2]domain.Asset{
		"a type that is not one of the three": {
			{Code: "USTRY", Issuer: "G", Type: domain.AssetType("alphanum12")}, testUSDC},
		"a credit asset with no issuer": {
			{Code: "USTRY", Type: domain.AssetTypeAlphanum12}, testUSDC},
		"a native asset with an issuer": {
			{Code: "XLM", Issuer: "G", Type: domain.AssetTypeNative}, testUSDC},
		"the same asset on both sides": {testUSDC, testUSDC},
	}
	for name, pair := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := s.UpsertAsset(ctx, pair[0], pair[1], ""); err == nil {
				t.Errorf("accepted %s", name)
			}
		})
	}
}

func TestDeactivatedAssetIsHiddenFromAScanAndVisibleToAListing(t *testing.T) {
	s, ctx := testStore(t)
	id := seedAsset(t, s, ctx)

	if err := s.SetAssetActive(ctx, id, false); err != nil {
		t.Fatalf("SetAssetActive: %v", err)
	}
	active, err := s.Assets(ctx, true)
	if err != nil {
		t.Fatalf("Assets(active): %v", err)
	}
	for _, a := range active {
		if a.ID == id {
			t.Error("a deactivated asset is still in the active list")
		}
	}
	all, err := s.Assets(ctx, false)
	if err != nil {
		t.Fatalf("Assets(all): %v", err)
	}
	var seen bool
	for _, a := range all {
		if a.ID == id {
			seen = true
			if a.Active {
				t.Error("the asset reads as active after being deactivated")
			}
		}
	}
	if !seen {
		t.Error("a deactivated asset vanished from the full listing")
	}
}

func TestAssetIDReportsNotFound(t *testing.T) {
	s, ctx := testStore(t)
	absent := domain.Asset{Code: "NOPE", Issuer: "GNOTANISSUER", Type: domain.AssetTypeAlphanum4}
	if _, err := s.AssetID(ctx, absent, testUSDC); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// ---------------------------------------------------------------- runs

func TestRunLifecycle(t *testing.T) {
	s, ctx := testStore(t)
	started := time.Date(2026, 8, 24, 10, 0, 0, 0, time.UTC)

	id, err := s.StartRun(ctx, RunScan, started)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	// An unfinished run must be visible. A crashed scan that reads as "no scan
	// ever ran" is the failure this table exists to make impossible.
	open, err := s.LastRun(ctx, RunScan)
	if err != nil {
		t.Fatalf("LastRun: %v", err)
	}
	if open.FinishedAt != nil {
		t.Error("a run that was never finished reports a finish time")
	}

	if err := s.FinishRun(ctx, id, started.Add(time.Minute), 49, 1, "one asset had no book"); err != nil {
		t.Fatalf("FinishRun: %v", err)
	}
	done, err := s.LastRun(ctx, RunScan)
	if err != nil {
		t.Fatalf("LastRun: %v", err)
	}
	if done.FinishedAt == nil {
		t.Fatal("the finished run has no finish time")
	}
	if done.AssetsOK != 49 || done.AssetsFailed != 1 {
		t.Errorf("counts = %d ok, %d failed; want 49 and 1", done.AssetsOK, done.AssetsFailed)
	}
	if done.Notes != "one asset had no book" {
		t.Errorf("notes = %q", done.Notes)
	}

	// Closing it again would overwrite those counts.
	if err := s.FinishRun(ctx, id, started.Add(2*time.Minute), 50, 0, "looks clean"); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound; a run must not be closed twice", err)
	}
	again, err := s.LastRun(ctx, RunScan)
	if err != nil {
		t.Fatalf("LastRun: %v", err)
	}
	if again.AssetsFailed != 1 {
		t.Errorf("assets_failed is now %d; the failure count was overwritten", again.AssetsFailed)
	}
}

func TestStartRunRejectsAnUnknownKind(t *testing.T) {
	s, ctx := testStore(t)
	if _, err := s.StartRun(ctx, RunKind("record"), time.Now().UTC()); err == nil {
		t.Fatal("accepted a run kind the CHECK constraint forbids")
	}
}

// ---------------------------------------------------------------- schema

// The store is written against three migrations. If they are not applied, every
// other test here fails in a confusing way, so this one says so plainly.
func TestSchemaHasEveryMigrationApplied(t *testing.T) {
	s, ctx := testStore(t)
	applied, err := s.SchemaVersion(ctx)
	if err != nil {
		t.Fatalf("SchemaVersion: %v; run make migrate", err)
	}
	want := []string{
		"0001_core.sql",
		"0002_methodology_103.sql",
		"0003_venue_split_and_offers_implied.sql",
	}
	have := map[string]bool{}
	for _, f := range applied {
		have[f] = true
	}
	for _, w := range want {
		if !have[w] {
			t.Errorf("%s is not applied; run make migrate", w)
		}
	}
}

// ---------------------------------------------------------------- helper

func assertRiskEqual(t *testing.T, got, want domain.AssetRisk) {
	t.Helper()

	if !got.Base.Equal(want.Base) || !got.Quote.Equal(want.Quote) {
		t.Errorf("pair = %s/%s, want %s/%s", got.Base, got.Quote, want.Base, want.Quote)
	}
	if got.LedgerSeq != want.LedgerSeq {
		t.Errorf("LedgerSeq = %d, want %d", got.LedgerSeq, want.LedgerSeq)
	}
	if !got.LedgerClosedAt.Equal(want.LedgerClosedAt) {
		t.Errorf("LedgerClosedAt = %s, want %s", got.LedgerClosedAt, want.LedgerClosedAt)
	}
	if got.MethodologyVersion != want.MethodologyVersion {
		t.Errorf("MethodologyVersion = %q, want %q", got.MethodologyVersion, want.MethodologyVersion)
	}
	if got.DataSource != want.DataSource {
		t.Errorf("DataSource = %q, want %q", got.DataSource, want.DataSource)
	}
	if got.PriceSource != want.PriceSource {
		t.Errorf("PriceSource = %q, want %q", got.PriceSource, want.PriceSource)
	}
	if got.Band != want.Band || got.BandConfidence != want.BandConfidence {
		t.Errorf("band = %s/%s, want %s/%s", got.Band, got.BandConfidence, want.Band, want.BandConfidence)
	}

	decimals := []struct {
		name      string
		got, want *decimal.Decimal
	}{
		{"MidPrice", got.MidPrice, want.MidPrice},
		{"SpreadPct", got.SpreadPct, want.SpreadPct},
		{"PoolSpotPrice", got.PoolSpotPrice, want.PoolSpotPrice},
		{"PriceDivergencePct", got.PriceDivergencePct, want.PriceDivergencePct},
		{"MaxReachablePrice", got.MaxReachablePrice, want.MaxReachablePrice},
		{"CostToMaxReachablePrice", got.CostToMaxReachablePrice, want.CostToMaxReachablePrice},
		{"MaxSafeCollateral", got.MaxSafeCollateral, want.MaxSafeCollateral},
		{"MaxSafeCollateralLiquidation", got.MaxSafeCollateralLiquidation, want.MaxSafeCollateralLiquidation},
		{"MaxSafeCollateralManipulation", got.MaxSafeCollateralManipulation, want.MaxSafeCollateralManipulation},
		{"HolderTop1Pct", got.Supporting.HolderTop1Pct, want.Supporting.HolderTop1Pct},
		{"HolderTop10Pct", got.Supporting.HolderTop10Pct, want.Supporting.HolderTop10Pct},
		{"HolderHHI", got.Supporting.HolderHHI, want.Supporting.HolderHHI},
		{"VolumeToSupplyD1", got.Supporting.VolumeToSupplyD1, want.Supporting.VolumeToSupplyD1},
		{"VolumeToSupplyD7", got.Supporting.VolumeToSupplyD7, want.Supporting.VolumeToSupplyD7},
		{"VolumeToSupplyD30", got.Supporting.VolumeToSupplyD30, want.Supporting.VolumeToSupplyD30},
		{"TradesExcludedPct", got.Supporting.TradesExcludedPct, want.Supporting.TradesExcludedPct},
	}
	for _, d := range decimals {
		switch {
		case d.got == nil && d.want == nil:
		case d.got == nil || d.want == nil:
			t.Errorf("%s: got %v, want %v", d.name, d.got, d.want)
		case d.got.String() != d.want.String():
			t.Errorf("%s = %s, want %s", d.name, d.got, d.want)
		}
	}

	if len(got.Depth) != len(want.Depth) {
		t.Errorf("Depth has %d points, want %d", len(got.Depth), len(want.Depth))
	} else {
		for i := range got.Depth {
			g, w := got.Depth[i], want.Depth[i]
			if g.Delta.String() != w.Delta.String() || g.BuySide.String() != w.BuySide.String() ||
				g.SellSide.String() != w.SellSide.String() || g.FromSdex.String() != w.FromSdex.String() ||
				g.FromAmm.String() != w.FromAmm.String() {
				t.Errorf("Depth[%d] = %+v, want %+v", i, g, w)
			}
		}
	}
	assertManipulationEqual(t, "ManipulationCostCombined", got.ManipulationCostCombined, want.ManipulationCostCombined)
	assertManipulationEqual(t, "ManipulationCostOrderbookOnly", got.ManipulationCostOrderbookOnly, want.ManipulationCostOrderbookOnly)

	switch {
	case got.OracleResistance == nil && want.OracleResistance == nil:
	case got.OracleResistance == nil || want.OracleResistance == nil:
		t.Errorf("OracleResistance: got %v, want %v", got.OracleResistance, want.OracleResistance)
	default:
		g, w := got.OracleResistance, want.OracleResistance
		if g.CriticalDelta.String() != w.CriticalDelta.String() ||
			g.ManipulationCost.String() != w.ManipulationCost.String() ||
			g.Reachable != w.Reachable ||
			g.GenuineVolume.String() != w.GenuineVolume.String() ||
			g.WindowSeconds != w.WindowSeconds {
			t.Errorf("OracleResistance = %+v, want %+v", *g, *w)
		}
		if (g.Ratio == nil) != (w.Ratio == nil) {
			t.Errorf("OracleResistance.Ratio nil-ness differs")
		}
		if (g.TotalAttackCost == nil) != (w.TotalAttackCost == nil) {
			t.Errorf("OracleResistance.TotalAttackCost nil-ness differs")
		}
	}

	if got.Supporting.LastGenuineTrade == nil || want.Supporting.LastGenuineTrade == nil {
		if got.Supporting.LastGenuineTrade != want.Supporting.LastGenuineTrade {
			t.Error("LastGenuineTrade nil-ness differs")
		}
	} else if got.Supporting.LastGenuineTrade.LedgerSeq != want.Supporting.LastGenuineTrade.LedgerSeq ||
		!got.Supporting.LastGenuineTrade.At.Equal(want.Supporting.LastGenuineTrade.At) {
		t.Errorf("LastGenuineTrade = %+v, want %+v",
			*got.Supporting.LastGenuineTrade, *want.Supporting.LastGenuineTrade)
	}

	assertStringsEqual(t, "Flags", flagStrings(got.Flags), flagStrings(want.Flags))
	assertStringsEqual(t, "UnevaluatedFlags", flagStrings(got.UnevaluatedFlags), flagStrings(want.UnevaluatedFlags))
	assertStringsEqual(t, "Warnings", got.Warnings, want.Warnings)
}

func assertManipulationEqual(t *testing.T, name string, got, want []domain.ManipulationPoint) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s has %d points, want %d", name, len(got), len(want))
		return
	}
	for i := range got {
		g, w := got[i], want[i]
		if g.Delta.String() != w.Delta.String() || g.TargetPrice.String() != w.TargetPrice.String() ||
			g.Cost.String() != w.Cost.String() || g.Reachable != w.Reachable {
			t.Errorf("%s[%d] = %+v, want %+v", name, i, g, w)
		}
	}
}

func assertStringsEqual(t *testing.T, name string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s has %d entries, want %d: %q", name, len(got), len(want), got)
		return
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("%s[%d] = %q, want %q", name, i, got[i], want[i])
		}
	}
}
