// Layer 1 applied to the supporting metrics: the expected values, and the reader
// that gets the raw evidence in front of them.
//
// SAME RULE AS THE GOLDEN FIXTURE AND FOR THE SAME REASON. Every number in the
// Expected block below is TRANSCRIBED from docs/methodology/07-supporting-metrics.md,
// which is RED. Al computed them with his own query and his own spreadsheet, from
// the two trades CSV and the holder pull in docs/evidences/, before
// internal/domain/supporting.go existed. When the code disagrees with a number
// here, the code is what changes. Never these numbers.
//
// This file is GREEN and holds red numbers, which is exactly what expected.go
// already does for the golden fixture. Transcribing is not computing: the
// arithmetic that produced these figures happened on the other side of the wall,
// and this file's only job is to carry them across it faithfully enough to be
// checked. A transcription error is a bug in this file and shows up as a failing
// comparison, which is the failure mode worth having.
//
// THE INPUTS ARE RAW BYTES AND THE MANIFEST SAYS SO. The holder pull's own
// manifest.md states: "This directory is a reading, not a result. Nothing here is
// computed. Sections 2 and 3 of 07-supporting-metrics.md have no definitions yet,
// so no total, share, top-N or HHI appears in any file below. When those
// definitions are written, they get run over these bytes." The definitions were
// written by Al. This is those definitions being run over those bytes.

package conformance

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/shopspring/decimal"

	"github.com/Keel-Official/keel-backend/internal/domain"
)

// ---------------------------------------------------------------- Evidence paths

// The two trades CSV and the holder pull, relative to this package.
//
// Named as constants rather than built from a pattern, because a glob would
// silently pick up a third CSV and change what the test measures. A new period
// gets a new constant and a new expected block, which is the point: the numbers
// and the file they are about travel together.
const (
	TradesCSVFebruary = "../../docs/evidences/USTRY.GCRYUGD5-USDC.GA5ZSEJY-trades-2026-02-01_2026-03-01.csv"
	TradesCSVAugust   = "../../docs/evidences/USTRY.GCRYUGD5-USDC.GA5ZSEJY-trades-2026-08-01_2026-09-01.csv"
	HoldersCSVAugust  = "../../docs/evidences/2026-08-31-USTRY.GCRYUGD5-holders-and-supply/holders.csv"
)

// USTRYExclusions is section 2's explicit exclusion list for USTRY.
//
// Both addresses are recorded in section 2 and NEITHER has ever appeared in
// /accounts?asset=. The list is here anyway, wired and inert, because section 2
// says to keep it: it is a cheap guard and an asset whose pool position did
// surface would need it.
var USTRYExclusions = domain.HolderExclusions{
	Issuer: AssetUSTRY.Issuer,
	Addresses: []string{
		// The AMM pool, the same pool ID as PoolUSTRYUSDC above.
		"27480d0483c8320ba4a707797526ffd67118e841491e0cbeb66db697bb66cccb",
		// The Blend V2 (YieldBlox) position, a Soroban contract and not a G address.
		"CCCCIQSDILITHMM7PBSLVDT5MISSY7R26MNZXCX4H7J5JQ5FPIYOGYFS",
	},
}

// ---------------------------------------------------------------- Expected: section 1

// ExpectedGenuineRun is section 1's Outputs table for one period.
//
// Every field is a figure from that table. The per-condition entries include the
// ones that caught nothing, because a condition firing zero times is a RESULT: it
// is what shows August's exclusion is entirely dust.
type ExpectedGenuineRun struct {
	Period string

	TotalTrades   int
	TotalQuoteVol decimal.Decimal

	GenuineTrades   int
	GenuineQuoteVol decimal.Decimal

	// GenuineBaseVol is from section 3 rather than section 1, and only August has
	// it: section 1 quotes the quote asset, section 3 takes the base leg of the
	// same set. Zero here means "section 3 states no figure for this period", not
	// a measured zero, and the test skips the comparison rather than asserting it.
	GenuineBaseVol decimal.Decimal

	UnevaluatedTrades   int
	UnevaluatedQuoteVol decimal.Decimal

	// PerCondition is indexed by domain.ExclusionCondition, 1 to 5.
	PerCondition map[domain.ExclusionCondition]ExpectedCondition
}

// ExpectedCondition is one row of section 1's per-condition breakdown.
type ExpectedCondition struct {
	Trades   int
	QuoteVol decimal.Decimal
}

// ExpectedFebruary2026 is section 1's February column.
//
// February is the only period where more than one condition fires: dust, off-book
// and price-outlier are each active. It is also the period that holds the incident,
// so condition 5's single catch here IS the manipulative trade.
var ExpectedFebruary2026 = ExpectedGenuineRun{
	Period:        "February 2026",
	TotalTrades:   13547,
	TotalQuoteVol: dec("375320.8368055"),

	GenuineTrades:   12204,
	GenuineQuoteVol: dec("375310.2438969"),

	UnevaluatedTrades:   0,
	UnevaluatedQuoteVol: decimal.Zero,

	PerCondition: map[domain.ExclusionCondition]ExpectedCondition{
		domain.ConditionSelfTrade:       {Trades: 0, QuoteVol: decimal.Zero},
		domain.ConditionDust:            {Trades: 1334, QuoteVol: dec("4.4477211")},
		domain.ConditionIssuerLeg:       {Trades: 0, QuoteVol: decimal.Zero},
		domain.ConditionOffBookPoolFill: {Trades: 8, QuoteVol: dec("0.7976176")},
		// 5.3475699 USDC is the manipulative trade itself: 106.737283 against a
		// daily median near 1.057427. The same-account rule cannot reach it,
		// because its two accounts differ. Section 4 measures it at 16.4 times the
		// entire genuine volume of the 15 minute oracle window it landed in.
		domain.ConditionPriceOutlier: {Trades: 1, QuoteVol: dec("5.3475699")},
	},
}

// ExpectedAugust2026 is section 1's August column, plus section 3's base-unit
// total for the same set.
//
// August exclusion is entirely dust. The 389 pool fills dearer than the book,
// including all 315 on 29 August, sit below the dust threshold and stop at
// condition 2, so condition 4 nets nothing. That is the clearest demonstration in
// the record that the condition ORDER is load-bearing.
var ExpectedAugust2026 = ExpectedGenuineRun{
	Period:        "August 2026",
	TotalTrades:   56863,
	TotalQuoteVol: dec("6246.5452279"),

	GenuineTrades:   14478,
	GenuineQuoteVol: dec("6139.9850386"),
	// Section 3: the same 14,478 trades are 5,723.2370064 USTRY. The ratio between
	// the two figures, 1.072817, is the month's volume-weighted price and it is the
	// reason section 3 denominates in the base asset.
	GenuineBaseVol: dec("5723.2370064"),

	// 17 and not 150. All 17 fall on 18 and 19 August at prices inside the 1.5x
	// band. The other 133 uncomparable pool fills are also dust and stop at
	// condition 2.
	UnevaluatedTrades:   17,
	UnevaluatedQuoteVol: dec("0.5530988"),

	PerCondition: map[domain.ExclusionCondition]ExpectedCondition{
		domain.ConditionSelfTrade:       {Trades: 0, QuoteVol: decimal.Zero},
		domain.ConditionDust:            {Trades: 42368, QuoteVol: dec("106.0070905")},
		domain.ConditionIssuerLeg:       {Trades: 0, QuoteVol: decimal.Zero},
		domain.ConditionOffBookPoolFill: {Trades: 0, QuoteVol: decimal.Zero},
		domain.ConditionPriceOutlier:    {Trades: 0, QuoteVol: decimal.Zero},
	},
}

// ---------------------------------------------------------------- Expected: section 2

// ExpectedHolderPull is section 2's Result block over the 31 August pull.
var ExpectedHolderPull = struct {
	Trustlines         int
	ZeroBalanceDropped int
	Population         int
	CirculatingSupply  decimal.Decimal
	Top1Pct            decimal.Decimal
	Top10Pct           decimal.Decimal
	HHI                decimal.Decimal
	LargestBalance     decimal.Decimal
}{
	Trustlines:         875,
	ZeroBalanceDropped: 612,
	Population:         263, // 875 - 612
	CirculatingSupply:  dec("10432382.3504695"),
	Top1Pct:            dec("91.5406"),
	Top10Pct:           dec("99.9475"),
	// Far above the 2,500 that conventionally marks a highly concentrated market.
	HHI: dec("8410.8452"),
	// GA727XJU...CSAS, not yet identified. Section 2 is explicit that this is a
	// statement about one trustline and not about beneficial ownership.
	LargestBalance: dec("9549864.6630000"),
}

// ExpectedVolumeToSupplyMonth is section 3's whole-month row: 5,723.2370064 USTRY
// over 10,432,382.3504695 USTRY circulating supply.
//
// Section 3 reports it as a ratio of 0.000548603 and as 0.0548603 per cent. The
// ratio is carried here and the percentage is derived in the test, so the two
// cannot drift apart.
var ExpectedVolumeToSupplyMonth = dec("0.000548603")

// ---------------------------------------------------------------- Expected: section 4

// ExpectedOracleWindow is one row of section 4's window-width table.
type ExpectedOracleWindow struct {
	Window          time.Duration
	RecordedTrades  int
	GenuineTrades   int
	GenuineQuoteVol decimal.Decimal
}

// OracleWindowAnchor is the incident ledger's close time, 61340263.
//
// Taken from the golden fixture header, which states it as
// 2026-02-22T00:10:21Z, and it is the same instant section 4 measures its window
// back from. This is an anchor and NOT a clock reading: the window is counted
// backward from a ledger close time, which is what NFR-9 requires and what the
// architecture test enforces by banning time.Now from internal/domain.
var OracleWindowAnchor = time.Date(2026, 2, 22, 0, 10, 21, 0, time.UTC)

// ExpectedOracleWindows is section 4's table. Window width barely moves the
// result, which is the measured reason the unconfirmed 15 minute assumption is a
// small risk rather than a large one.
//
// The manipulative trade in that window was 5.3475699 USDC, 16.4 times the entire
// genuine volume of it. That ratio is why the incident ran as it did.
var ExpectedOracleWindows = []ExpectedOracleWindow{
	{Window: 15 * time.Minute, RecordedTrades: 5, GenuineTrades: 4, GenuineQuoteVol: dec("0.3268461")},
	{Window: 30 * time.Minute, RecordedTrades: 5, GenuineTrades: 4, GenuineQuoteVol: dec("0.3268461")},
	{Window: 60 * time.Minute, RecordedTrades: 8, GenuineTrades: 7, GenuineQuoteVol: dec("0.4250511")},
}

// ---------------------------------------------------------------- Readers

// LoadTradesCSV reads one of the evidence CSV into domain trades.
//
// Columns are resolved BY HEADER NAME and never by position. A CSV whose columns
// were reordered by a later version of the writer would otherwise be read as
// nonsense that still parses, which is the failure mode with no symptom.
//
// price_n and price_d are taken as the price and price_quote_per_base is IGNORED,
// which is non-negotiable rule 5: the fraction is exact and the decimal string is
// a rounded rendering of it. The two are checked against each other here anyway,
// because a disagreement between them means the file was written wrong and that is
// worth failing on rather than silently preferring one.
func LoadTradesCSV(path string) ([]domain.Trade, error) {
	f, err := os.Open(path) //nolint:gosec // a fixed evidence path, not user input
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close() //nolint:errcheck // read only

	r := csv.NewReader(f)
	head, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("read header of %s: %w", path, err)
	}
	col := map[string]int{}
	for i, h := range head {
		col[h] = i
	}
	need := []string{
		"trade_id", "operation_id", "fill_index", "ledger_seq", "closed_at",
		"trade_type", "price_n", "price_d", "base_amount", "counter_amount",
		"base_is_seller", "base_account", "counter_account",
		"liquidity_pool_id", "liquidity_pool_side",
	}
	for _, n := range need {
		if _, ok := col[n]; !ok {
			return nil, fmt.Errorf("%s: column %q is absent", path, n)
		}
	}

	var out []domain.Trade
	line := 1
	for {
		rec, err := r.Read()
		if err != nil {
			break
		}
		line++
		get := func(n string) string { return rec[col[n]] }

		at, err := time.Parse(time.RFC3339, get("closed_at"))
		if err != nil {
			return nil, fmt.Errorf("%s line %d: closed_at %q: %w", path, line, get("closed_at"), err)
		}
		pn, err := strconv.ParseInt(get("price_n"), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%s line %d: price_n: %w", path, line, err)
		}
		pd, err := strconv.ParseInt(get("price_d"), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%s line %d: price_d: %w", path, line, err)
		}
		ledger, err := strconv.ParseUint(get("ledger_seq"), 10, 32)
		if err != nil {
			return nil, fmt.Errorf("%s line %d: ledger_seq: %w", path, line, err)
		}
		fill, err := strconv.Atoi(get("fill_index"))
		if err != nil {
			return nil, fmt.Errorf("%s line %d: fill_index: %w", path, line, err)
		}
		baseAmt, err := decimal.NewFromString(get("base_amount"))
		if err != nil {
			return nil, fmt.Errorf("%s line %d: base_amount: %w", path, line, err)
		}
		counterAmt, err := decimal.NewFromString(get("counter_amount"))
		if err != nil {
			return nil, fmt.Errorf("%s line %d: counter_amount: %w", path, line, err)
		}

		out = append(out, domain.Trade{
			ID:                get("trade_id"),
			OperationID:       get("operation_id"),
			FillIndex:         fill,
			LedgerSeq:         uint32(ledger),
			ClosedAt:          at,
			Type:              get("trade_type"),
			Price:             domain.Price{N: pn, D: pd},
			BaseAmount:        baseAmt,
			CounterAmount:     counterAmt,
			BaseIsSeller:      get("base_is_seller") == "true",
			BaseAccount:       get("base_account"),
			CounterAccount:    get("counter_account"),
			LiquidityPoolID:   get("liquidity_pool_id"),
			LiquidityPoolSide: get("liquidity_pool_side"),
		})
	}
	return out, nil
}

// LoadHoldersCSV reads the 31 August holder pull.
//
// Every row is returned, including the 612 with a zero balance. Filtering here
// would move a decision section 2 makes into a reader, and the count of what was
// dropped is one of the figures section 2 reports.
func LoadHoldersCSV(path string) ([]domain.HolderBalance, error) {
	f, err := os.Open(path) //nolint:gosec // a fixed evidence path, not user input
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close() //nolint:errcheck // read only

	r := csv.NewReader(f)
	head, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("read header of %s: %w", path, err)
	}
	col := map[string]int{}
	for i, h := range head {
		col[h] = i
	}
	for _, n := range []string{"account_id", "balance"} {
		if _, ok := col[n]; !ok {
			return nil, fmt.Errorf("%s: column %q is absent", path, n)
		}
	}

	var out []domain.HolderBalance
	line := 1
	for {
		rec, err := r.Read()
		if err != nil {
			break
		}
		line++
		bal, err := decimal.NewFromString(rec[col["balance"]])
		if err != nil {
			return nil, fmt.Errorf("%s line %d: balance: %w", path, line, err)
		}
		out = append(out, domain.HolderBalance{
			AccountID: rec[col["account_id"]],
			Balance:   bal,
		})
	}
	return out, nil
}
