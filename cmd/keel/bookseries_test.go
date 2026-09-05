package main

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Keel-Official/keel-backend/internal/domain"
	"github.com/Keel-Official/keel-backend/internal/horizon"
	"github.com/shopspring/decimal"
)

// februaryTrades is the trades file this repository already holds for the month
// the backtest is about. The sample rule is tested against it rather than against
// a fabricated CSV, because the thing worth knowing is whether the rule survives
// the real file: 13,547 rows, days with 93 trades and days with 3,295.
const februaryTrades = "../../docs/evidences/USTRY.GCRYUGD5-USDC.GA5ZSEJY-trades-2026-02-01_2026-03-01.csv"

func TestTheFebruaryTradesFileYieldsOneSamplePerDay(t *testing.T) {
	if _, err := os.Stat(februaryTrades); err != nil {
		t.Skipf("the February trades file is not on disk: %v", err)
	}
	got, err := dailySamplesFromTrades(februaryTrades)
	if err != nil {
		t.Fatalf("deriving samples: %v", err)
	}
	if len(got) != 28 {
		t.Fatalf("got %d sample(s), want 28, one per day of February 2026", len(got))
	}
	if got[0].Day != "2026-02-01" || got[0].Ledger != 61027032 {
		t.Errorf("first sample = %s at ledger %d, want 2026-02-01 at 61027032",
			got[0].Day, got[0].Ledger)
	}
	if got[27].Day != "2026-02-28" {
		t.Errorf("last sample = %s, want 2026-02-28", got[27].Day)
	}
}

// TestNoSampleIsTakenBeforeItsOwnMidnight is the honesty property the rule was
// chosen for. A row labelled with a day must describe a book that existed on that
// day, never the state the market was in the evening before.
func TestNoSampleIsTakenBeforeItsOwnMidnight(t *testing.T) {
	if _, err := os.Stat(februaryTrades); err != nil {
		t.Skipf("the February trades file is not on disk: %v", err)
	}
	got, err := dailySamplesFromTrades(februaryTrades)
	if err != nil {
		t.Fatalf("deriving samples: %v", err)
	}
	for _, s := range got {
		if s.OffsetSeconds < 0 {
			t.Errorf("%s sampled %d seconds BEFORE its own midnight", s.Day, -s.OffsetSeconds)
		}
		if s.OffsetSeconds >= 86400 {
			t.Errorf("%s sampled %d seconds after midnight, which is the next day", s.Day, s.OffsetSeconds)
		}
		if s.ClosedAt.UTC().Format("2006-01-02") != s.Day {
			t.Errorf("%s carries a timestamp on %s", s.Day, s.ClosedAt.UTC().Format("2006-01-02"))
		}
	}
}

// TestTheLedgersRiseWithTheDays catches the failure that would be hardest to see
// in the output: a sample list that is not in ledger order would put the series
// out of time order while every row still looked correct on its own.
func TestTheLedgersRiseWithTheDays(t *testing.T) {
	if _, err := os.Stat(februaryTrades); err != nil {
		t.Skipf("the February trades file is not on disk: %v", err)
	}
	got, err := dailySamplesFromTrades(februaryTrades)
	if err != nil {
		t.Fatalf("deriving samples: %v", err)
	}
	for i := 1; i < len(got); i++ {
		if got[i].Ledger <= got[i-1].Ledger {
			t.Fatalf("%s is at ledger %d and %s is at %d: the series is not in time order",
				got[i-1].Day, got[i-1].Ledger, got[i].Day, got[i].Ledger)
		}
	}
}

func TestGatherSamplesMergesTheFlagWithTheDerivedGrid(t *testing.T) {
	if _, err := os.Stat(februaryTrades); err != nil {
		t.Skipf("the February trades file is not on disk: %v", err)
	}
	got, err := gatherSamples(februaryTrades, "61340262")
	if err != nil {
		t.Fatalf("gathering: %v", err)
	}
	if len(got) != 29 {
		t.Fatalf("got %d sample(s), want 29: 28 days plus the control ledger", len(got))
	}
	var control seriesSample
	for _, s := range got {
		if s.Ledger == 61340262 {
			control = s
		}
	}
	if control.Source != "flag" {
		t.Errorf("the control ledger came back with source %q, want %q so a reader can tell it from the daily grid",
			control.Source, "flag")
	}
	for i := 1; i < len(got); i++ {
		if got[i].Ledger <= got[i-1].Ledger {
			t.Fatalf("the merged list is not in ledger order at %d", i)
		}
	}
}

func TestAnUnparseableAlsoLedgerIsRefused(t *testing.T) {
	if _, err := gatherSamples("", "not-a-ledger"); err == nil {
		t.Fatal("a target that is not a ledger sequence was accepted")
	}
	if _, err := gatherSamples("", "0"); err == nil {
		t.Fatal("ledger 0 was accepted as a target")
	}
}

// TestEveryRowHasAsManyFieldsAsTheHeader is the shape check that matters for a
// file somebody else has to read. The depth and manipulation columns are built
// from the parameters rather than hardcoded, so a ladder that gains a rung widens
// both the header and the rows, and it must widen them by the same amount.
func TestEveryRowHasAsManyFieldsAsTheHeader(t *testing.T) {
	p := domain.DefaultParams()
	risk, err := domain.ComputeAssetRisk(testSnapshot(), p)
	if err != nil {
		t.Fatalf("computing the test snapshot: %v", err)
	}
	rows := []seriesRow{{
		Sample: seriesSample{Day: "2026-02-22", Ledger: 61340262, ClosedAt: time.Now().UTC(), Source: "flag"},
		Point:  horizon.SeriesPoint{Target: 61340262, Snapshot: testSnapshot(), RestingOffers: 2},
		Risk:   risk,
	}}

	path := filepath.Join(t.TempDir(), "series.csv")
	if err := writeSeriesCSV(path, rows, p); err != nil {
		t.Fatalf("writing: %v", err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	defer func() { _ = f.Close() }()

	records, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("got %d line(s), want a header and one row", len(records))
	}
	if len(records[0]) != len(records[1]) {
		t.Errorf("the header has %d field(s) and the row has %d", len(records[0]), len(records[1]))
	}
	// The ladder columns must be named after their delta, so a reader can find a
	// rung by value instead of by counting.
	head := records[0]
	wanted := map[string]bool{"depth_buy_0.02": false, "depth_sell_0.1": false, "mc_cost_0.5": false, "fold_complete": false}
	for _, h := range head {
		if _, ok := wanted[h]; ok {
			wanted[h] = true
		}
	}
	for name, found := range wanted {
		if !found {
			t.Errorf("the header has no %q column: %v", name, head)
		}
	}
}

// testSnapshot is a two-level book in the shape of the fixture's, and its numbers
// are NOT the fixture's. It exists to exercise the writer, and a test file on this
// side of the wall must never become a second home for the expected values.
func testSnapshot() domain.Snapshot {
	price := func(n, d int64) domain.Price {
		return domain.Price{N: n, D: d}
	}
	return domain.Snapshot{
		Base:      domain.Asset{Code: "USTRY", Issuer: "GCRYUGD5NVARGXT56XEZI5CIFCQETYHAPQQTHO2O3IQZTHDH4LATMYWC", Type: "credit_alphanum12"},
		Quote:     domain.Asset{Code: "USDC", Issuer: "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN", Type: "credit_alphanum4"},
		LedgerSeq: 61340262,
		Source:    domain.DataSourceOffersImplied,
		Book: domain.OrderBook{
			Bids: []domain.Level{{Price: price(1, 1), Amount: decimal.RequireFromString("10")}},
			Asks: []domain.Level{{Price: price(2, 1), Amount: decimal.RequireFromString("10")}},
		},
	}
}

// TestTheSizeAtTheTopOfBookIsInTheCSV is the regression for the omission that
// made two rows one ledger apart come out byte-identical across the manipulation
// that Deliverable 2 is about. The trade changed the ask's amount and not its
// price, and nothing in the file carried an amount.
func TestTheSizeAtTheTopOfBookIsInTheCSV(t *testing.T) {
	p := domain.DefaultParams()
	risk, err := domain.ComputeAssetRisk(testSnapshot(), p)
	if err != nil {
		t.Fatalf("computing: %v", err)
	}
	rows := []seriesRow{{
		Sample: seriesSample{Day: "2026-02-22", Ledger: 61340262, Source: "flag"},
		Point:  horizon.SeriesPoint{Target: 61340262, Snapshot: testSnapshot(), RestingOffers: 2},
		Risk:   risk,
	}}
	path := filepath.Join(t.TempDir(), "series.csv")
	if err := writeSeriesCSV(path, rows, p); err != nil {
		t.Fatalf("writing: %v", err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	defer func() { _ = f.Close() }()
	records, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}

	cell := func(name string) string {
		for i, h := range records[0] {
			if h == name {
				return records[1][i]
			}
		}
		t.Fatalf("no %q column in %v", name, records[0])
		return ""
	}
	for _, name := range []string{"best_bid_amount", "best_ask_amount", "bid_amount_total", "ask_amount_total"} {
		if cell(name) == "" {
			t.Errorf("%s is empty on a book with one level a side", name)
		}
	}
	// testSnapshot puts 10 on each side at one level, so the top of book and the
	// side total are the same number. A total that silently reported the level
	// count, or the first level twice, would pass a weaker assertion than this.
	if got, want := cell("ask_amount_total"), "10"; got != want {
		t.Errorf("ask_amount_total = %q, want %q", got, want)
	}
}

// TestTwoBooksThatDifferOnlyInSizeProduceDifferentRows is the property the
// omission broke, stated directly rather than through a column list.
func TestTwoBooksThatDifferOnlyInSizeProduceDifferentRows(t *testing.T) {
	p := domain.DefaultParams()

	before := testSnapshot()
	after := testSnapshot()
	after.Book.Asks[0].Amount = decimal.RequireFromString("9.5")

	rows := make([]seriesRow, 0, 2)
	for _, snap := range []domain.Snapshot{before, after} {
		risk, err := domain.ComputeAssetRisk(snap, p)
		if err != nil {
			t.Fatalf("computing: %v", err)
		}
		rows = append(rows, seriesRow{
			Sample: seriesSample{Day: "2026-02-22", Ledger: snap.LedgerSeq, Source: "flag"},
			Point:  horizon.SeriesPoint{Target: snap.LedgerSeq, Snapshot: snap, RestingOffers: 2},
			Risk:   risk,
		})
	}

	path := filepath.Join(t.TempDir(), "series.csv")
	if err := writeSeriesCSV(path, rows, p); err != nil {
		t.Fatalf("writing: %v", err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	defer func() { _ = f.Close() }()
	records, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("got %d line(s), want a header and two rows", len(records))
	}
	same := true
	for i := range records[1] {
		if records[1][i] != records[2][i] {
			same = false
			break
		}
	}
	if same {
		t.Error("two books differing in the size at the top of the ask side wrote identical rows")
	}
}
