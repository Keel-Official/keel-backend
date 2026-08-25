// Package conformance holds Keel's golden fixture and its expected values.
//
// This package is deliberately SEPARATE from internal/domain. It may only call
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

// PoolUSTRYUSDC is the AMM pool of the golden fixture, at the same ledger as the
// order book beside it. THESE NUMBERS ARE COMPUTED BY HAND and are the
// specification: when the code disagrees with them, the code is what changes.
var PoolUSTRYUSDC = domain.PoolReserves{
	PoolID:       "27480d0483c8320ba4a707797526ffd67118e841491e0cbeb66db697bb66cccb",
	ReserveBase:  dec("15.4791416"), // USTRY
	ReserveQuote: dec("16.3389179"), // USDC
	FeeBP:        30,
}

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
		Pools: []domain.PoolReserves{PoolUSTRYUSDC},

		// offers-implied, not horizon, decided as handoff item 5b. This book was
		// not read from /order_book, because Horizon serves no historical order
		// book state. It was reconstructed by replaying manage_sell_offer and
		// manage_buy_offer operations, so it proves liquidity that was POSTED.
		// That is a stronger claim than trades-implied, which proves only what was
		// consumed, and a different claim from horizon, which means read directly.
		Source: domain.DataSourceOffersImplied,
	}
}

// FixtureParams holds the default thresholds from
// docs/methodology/09-flags-and-bands.md section 6.
//
// UNITS: every field ending in Pct is expressed in PERCENT, not as a fraction.
// SpreadExtremePct set to 20 means 20 percent, and it is compared against a
// spreadPct of 196.08 on this fixture. The convention was unified this way
// because HolderTop1ExtremePct and its siblings already use percent, so a
// fraction would be the single exception and a source of silent bugs.
//
// Every one of these values is CHOSEN, not calibrated against a set of incidents.
func FixtureParams() domain.Params {
	return domain.DefaultParams()
}

// BookOnlySnapshot is SYNTHETIC. It reuses the real book state from ledger
// 61340263 but deliberately omits the pool that existed at the time.
//
// Its purpose is to exercise the pure-order-book paths: Reachable=false and a
// non-null MaxReachablePrice, neither of which can occur once an active pool is
// present. It must never be presented as the actual market state on
// 22 February 2026.
func BookOnlySnapshot() domain.Snapshot {
	s := GoldenSnapshot()
	s.Pools = nil
	return s
}
