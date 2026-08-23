// Package conformance holds Keel's golden fixture and its expected values.
//
// This package is deliberately SEPARATE from internal/depth. It may only call
// exported API, which makes the conformance test black-box structurally rather
// than by its author's discipline alone. Another deliberate consequence: the red
// zone does not have to be touched in order to maintain these tests.
//
// Every number here comes from testdata/fixtures/ustry_pre_exploit.md, computed
// by hand BEFORE any implementation existed. Do not adjust these numbers to
// match the code. Adjust the code to match these numbers.
package conformance

import (
	"time"

	"github.com/shopspring/decimal"

	"github.com/Keel-Official/keel-backend/internal/domain"
)

// Tolerance for decimal comparison.
//
// Required because some expected values do not terminate. spreadPct on this
// fixture is 528401414/269485707, whose decimal expansion never ends. The
// precision of the computation itself is a methodology constant and has not been
// fixed yet; until it is decided, this tolerance applies.
var Tolerance = decimal.RequireFromString("0.0000001")

// dec reads a decimal constant. It panics on bad input, which is correct for a
// fixture: a mistyped constant must blow up immediately.
func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

// ---------------------------------------------------------------- Assets

var (
	// AssetUSTRY has a five character code and is therefore an alphanum12 asset.
	// Querying Horizon with alphanum4 returns an empty result and no error.
	AssetUSTRY = domain.Asset{
		Code:   "USTRY",
		Issuer: "GCRYUGD5NVARGXT56XEZI5CIFCQETYHAPQQTHO2O3IQZTHDH4LATMYWC",
		Type:   domain.AssetTypeAlphanum12,
	}

	// AssetUSDC is the quote asset of the fixture pair.
	AssetUSDC = domain.Asset{
		Code:   "USDC",
		Issuer: "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN",
		Type:   domain.AssetTypeAlphanum4,
	}
)

// ---------------------------------------------------------------- Input

// GoldenSnapshot is the state of the USTRY/USDC book that genuinely existed
// on-chain moments before ledger 61340263.
//
// The ask was placed by op 263453036239003649 on 21 February at 23:38:51, the
// bid by op 263453066303434753 at 23:39:31. Both belong to the SAME account,
// GCNF5GNRIT6VWYZ7LXUZ33Q3SR2NUGO32F5X65VVKAEWWIQCKGYN75HB. That 0.0001 bid
// helps form P0 without representing any real liquidity at all, and it is the
// concrete reason P0 must never be read on its own.
func GoldenSnapshot() domain.Snapshot {
	return domain.Snapshot{
		Base:           AssetUSTRY,
		Quote:          AssetUSDC,
		LedgerSeq:      61340263,
		LedgerClosedAt: time.Date(2026, time.February, 22, 0, 10, 21, 0, time.UTC),
		Book: domain.OrderBook{
			Asks: []domain.Level{
				{Price: domain.Price{N: 266843207, D: 2500000}, Amount: dec("1.2185312")},
			},
			Bids: []domain.Level{
				{Price: domain.Price{N: 1057, D: 1000}, Amount: dec("0.0001000")},
			},
		},
		Pools:  nil,
		Source: domain.DataSourceHorizon,
	}
}

// DefaultParams holds the default thresholds from
// docs/methodology/09-flag-dan-band.md section 6.
//
// UNITS: every field ending in Pct is expressed in PERCENT, not as a fraction.
// SpreadExtremePct set to 20 means 20 percent, and it is compared against a
// spreadPct of 196.08 on this fixture. The convention was unified this way
// because HolderTop1ExtremePct and its siblings already use percent, so a
// fraction would be the single exception and a source of silent bugs.
//
// Every one of these values is CHOSEN, not calibrated against a set of incidents.
func DefaultParams() domain.Params {
	return domain.Params{
		MarketDeltas:       []decimal.Decimal{dec("0.02"), dec("0.05"), dec("0.10")},
		ManipulationDeltas: []decimal.Decimal{dec("0.5"), dec("1"), dec("10"), dec("100")},

		LiquidationDelta:          dec("0.10"),
		LiquidationHaircut:        dec("0.5"),
		ManipulationCriticalDelta: dec("1.0"),
		ManipulationMargin:        dec("0.25"),

		// 15 minutes is an ASSUMPTION and has not been confirmed as Reflector's
		// actual window.
		OracleWindow: 15 * time.Minute,

		Thresholds: domain.Thresholds{
			ManipulationCheapAbsolute: dec("10000"),
			ManipulationRatioLowPct:   dec("1.0"),
			ThinDepth5PctAbsolute:     dec("50000"),
			SpreadExtremePct:          dec("20.0"),
			HolderTop1ExtremePct:      dec("50.0"),
			HolderTop10HighPct:        dec("80.0"),
			WashTradeSuspectedPct:     dec("50.0"),
			GenuineTradeStaleDays:     30,
			GenuineTradeWarnDays:      7,
		},
	}
}
