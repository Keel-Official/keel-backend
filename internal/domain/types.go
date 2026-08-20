// Package domain berisi tipe bersama Keel: aset, harga, buku, pool,
// snapshot, dan bentuk hasil perhitungan risiko likuiditas.
//
// Paket ini TIDAK memuat perhitungan. Seluruh rumus metodologi berada di
// internal/depth, yang mengimpor paket ini. Pemisahan itu disengaja:
// domain boleh disentuh siapa pun, depth adalah deliverable berbayar yang
// harus dapat dipertahankan penulisnya.
//
// PAKET INI MURNI. Tidak boleh ada:
//   - I/O apa pun (net/http, database/sql, os, SDK Stellar, BigQuery)
//   - time.Now(), math/rand, goroutine
//   - float64 atau float32, termasuk sebagai nilai antara
//   - iterasi map tanpa sort kunci lebih dulu
//
// Aturan ini ditegakkan oleh arch_test.go dan bukan sekadar konvensi.
// Lihat docs/methodology/keel-methodology-core.md untuk definisi setiap
// besaran di sini.
package domain

import (
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

// MethodologyVersion wajib naik setiap kali definisi atau ambang berubah.
// Hasil dari versi berbeda tidak dapat dibandingkan langsung.
const MethodologyVersion = "1.0.2-draft"

// ---------------------------------------------------------------- Aset

type AssetType string

const (
	AssetTypeNative     AssetType = "native"
	AssetTypeAlphanum4  AssetType = "credit_alphanum4"
	AssetTypeAlphanum12 AssetType = "credit_alphanum12"
)

// Asset mengidentifikasi satu aset Stellar.
//
// Type wajib eksplisit dan tidak boleh disimpulkan dari panjang Code saat
// runtime. Query Horizon dengan tipe yang salah mengembalikan hasil kosong
// tanpa pesan error, yang merupakan kegagalan diam paling berbahaya di
// integrasi ini. USTRY berkode 5 karakter sehingga bertipe alphanum12.
type Asset struct {
	Code   string
	Issuer string // kosong untuk native
	Type   AssetType
}

func (a Asset) IsNative() bool { return a.Type == AssetTypeNative }

func (a Asset) String() string {
	if a.IsNative() {
		return "XLM"
	}
	return a.Code + ":" + a.Issuer
}

func (a Asset) Equal(o Asset) bool {
	return a.Code == o.Code && a.Issuer == o.Issuer && a.Type == o.Type
}

// ---------------------------------------------------------------- Harga

// Price adalah rasional eksak, selalu dinyatakan sebagai quote per base.
//
// Horizon mengirim harga dalam dua bentuk yang tidak konsisten:
//
//	/offers  -> "price_r": {"n": 266843207, "d": 2500000}   angka JSON
//	/trades  -> "price":   {"n": "2500000", "d": "266843207"} string JSON
//
// dan arah price pada /trades bergantung pada aset mana yang menjadi base.
// Adapter bertanggung jawab menormalkan keduanya menjadi quote-per-base
// sebelum menyentuh paket ini. Jangan pernah memakai field string "price"
// dari Horizon untuk perhitungan; itu hasil pembulatan.
type Price struct {
	N int64
	D int64
}

func (p Price) Valid() bool { return p.D != 0 && p.N > 0 }

func (p Price) Decimal() decimal.Decimal {
	return decimal.NewFromInt(p.N).Div(decimal.NewFromInt(p.D))
}

func (p Price) Invert() Price { return Price{N: p.D, D: p.N} }

// Cmp membandingkan tanpa pembagian, sehingga tidak ada kehilangan presisi.
// Mengembalikan -1, 0, atau 1.
func (p Price) Cmp(o Price) int {
	left := decimal.NewFromInt(p.N).Mul(decimal.NewFromInt(o.D))
	right := decimal.NewFromInt(o.N).Mul(decimal.NewFromInt(p.D))
	return left.Cmp(right)
}

func (p Price) String() string { return fmt.Sprintf("%d/%d", p.N, p.D) }

// ---------------------------------------------------------------- Buku

// Level adalah satu tingkat harga pada orderbook.
// Amount dinyatakan dalam unit aset base.
type Level struct {
	Price  Price
	Amount decimal.Decimal
}

// Notional mengembalikan nilai level ini dalam aset quote.
func (l Level) Notional() decimal.Decimal { return l.Price.Decimal().Mul(l.Amount) }

// OrderBook memuat sisi beli dan jual.
// Bids diurutkan harga menurun, Asks diurutkan harga menaik.
// Adapter yang mengisi struct ini bertanggung jawab atas urutannya.
type OrderBook struct {
	Bids []Level
	Asks []Level
}

func (b OrderBook) BestBid() (Level, bool) {
	if len(b.Bids) == 0 {
		return Level{}, false
	}
	return b.Bids[0], true
}

func (b OrderBook) BestAsk() (Level, bool) {
	if len(b.Asks) == 0 {
		return Level{}, false
	}
	return b.Asks[0], true
}

// ---------------------------------------------------------------- Pool

// PoolReserves adalah satu pool constant product.
// FeeBP adalah basis point, 30 pada Stellar.
type PoolReserves struct {
	PoolID       string
	ReserveBase  decimal.Decimal
	ReserveQuote decimal.Decimal
	FeeBP        int32
}

func (p PoolReserves) SpotPrice() decimal.Decimal {
	if p.ReserveBase.IsZero() {
		return decimal.Zero
	}
	return p.ReserveQuote.Div(p.ReserveBase)
}

func (p PoolReserves) IsEmpty() bool {
	return p.ReserveBase.IsZero() || p.ReserveQuote.IsZero()
}

// ---------------------------------------------------------------- Snapshot

type PriceSource string

const (
	PriceSourceBook PriceSource = "book"
	PriceSourcePool PriceSource = "pool"
	PriceSourceNone PriceSource = "none"
)

type DataSource string

const (
	DataSourceHorizon       DataSource = "horizon"
	DataSourceHubble        DataSource = "hubble"
	DataSourceTradesImplied DataSource = "trades-implied"
)

// Snapshot adalah SATU-SATUNYA masukan bagi perhitungan depth.
// Bentuknya identik apakah berasal dari Horizon (live) atau Hubble (historis),
// sehingga menukar sumber data tidak menyentuh satu baris pun di paket ini.
type Snapshot struct {
	Base           Asset
	Quote          Asset
	LedgerSeq      uint32
	LedgerClosedAt time.Time
	Book           OrderBook
	Pools          []PoolReserves
	Source         DataSource
}

// ---------------------------------------------------------------- Hasil

// DepthPoint adalah depth pada satu tingkat delta.
// Seluruh nilai dinyatakan sebagai notional dalam aset quote.
//
// FromSdex dan FromAmm dilaporkan terpisah agar pihak ketiga dapat
// memverifikasi penggabungan tanpa membaca kode. Keduanya merujuk sisi beli.
type DepthPoint struct {
	Delta    decimal.Decimal
	BuySide  decimal.Decimal
	SellSide decimal.Decimal
	FromSdex decimal.Decimal
	FromAmm  decimal.Decimal
}

// ManipulationPoint adalah biaya menaikkan harga ke satu sasaran.
//
// Aturan:
//
//	Cost(P_target)      = Σ notional ask dengan price <  P_target
//	Reachable(P_target) = ada ask dengan price >= P_target
//
// Penyerang harus melalap seluruh ask yang LEBIH MURAH dari sasaran, lalu
// menyentuh sedikit saja ask pertama yang berada DI ATAS sasaran. Sentuhan
// terakhir itu yang menetapkan harga yang dibaca oracle, dan biayanya dapat
// sekecil apa pun.
//
// Tafsir hasil:
//
//	Cost kecil, Reachable=true   murah dan tercapai. PALING BERBAHAYA.
//	Cost besar, Reachable=true   mahal, pasar punya pertahanan.
//	Reachable=false              sasaran mustahil dicapai berapa pun modalnya,
//	                             karena buku habis sebelum sampai ke sana.
//	                             Ini justru bukan kabar buruk.
//
// Cost adalah BATAS ATAS. Keel tidak dapat mengetahui order mana yang dimiliki
// penyerang sebelum kejadian, sehingga tidak menyaringnya. Arah bias ini aman.
type ManipulationPoint struct {
	Delta       decimal.Decimal
	TargetPrice decimal.Decimal
	Cost        decimal.Decimal
	Reachable   bool
}

// ---------------------------------------------------------------- Flag

type Flag string

const (
	FlagNoExecutablePrice          Flag = "NO_EXECUTABLE_PRICE"
	FlagZeroDepth2Pct              Flag = "ZERO_DEPTH_2PCT"
	FlagManipulationCheap          Flag = "MANIPULATION_CHEAP"
	FlagManipulationRatioLow       Flag = "MANIPULATION_RATIO_LOW"
	FlagSpreadExtreme              Flag = "SPREAD_EXTREME"
	FlagNoGenuineTrade30D          Flag = "NO_GENUINE_TRADE_30D"
	FlagNoGenuineTrade7D           Flag = "NO_GENUINE_TRADE_7D"
	FlagHolderConcentrationExtreme Flag = "HOLDER_CONCENTRATION_EXTREME"
	FlagHolderConcentrationHigh    Flag = "HOLDER_CONCENTRATION_HIGH"
	FlagThinDepth5Pct              Flag = "THIN_DEPTH_5PCT"
	FlagWashTradeSuspected         Flag = "WASH_TRADE_SUSPECTED"
)

type Band string

const (
	BandLow      Band = "LOW"
	BandMedium   Band = "MEDIUM"
	BandHigh     Band = "HIGH"
	BandCritical Band = "CRITICAL"
)

// ---------------------------------------------------------------- Parameter

// Thresholds memuat SELURUH ambang di satu tempat.
// Nilai-nilai ini DIPILIH, bukan dikalibrasi terhadap kumpulan insiden.
// Mengubahnya wajib menaikkan MethodologyVersion.
type Thresholds struct {
	ManipulationCheapAbsolute decimal.Decimal // dalam aset quote
	ManipulationRatioLowPct   decimal.Decimal
	ThinDepth5PctAbsolute     decimal.Decimal
	SpreadExtremePct          decimal.Decimal
	HolderTop1ExtremePct      decimal.Decimal
	HolderTop10HighPct        decimal.Decimal
	WashTradeSuspectedPct     decimal.Decimal
	GenuineTradeStaleDays     int
	GenuineTradeWarnDays      int
}

// Params adalah seluruh masukan konfigurasi perhitungan.
// Tidak ada nilai default tersembunyi di dalam fungsi; semuanya lewat sini.
type Params struct {
	// Tangga kualitas pasar, wajib menurut SOW: 0.02, 0.05, 0.10
	MarketDeltas []decimal.Decimal

	// Tangga ketahanan manipulasi: 0.5, 1, 10, 100
	// Diperlukan karena penyerang tidak menggeser harga 10 persen,
	// melainkan 100 kali lipat.
	ManipulationDeltas []decimal.Decimal

	LiquidationDelta          decimal.Decimal // default 0.10
	LiquidationHaircut        decimal.Decimal // default 0.5
	ManipulationCriticalDelta decimal.Decimal // default 1.0
	ManipulationMargin        decimal.Decimal // default 0.25

	// Jendela VWAP oracle. Default 15 menit adalah ASUMSI, belum
	// dikonfirmasi sebagai jendela Reflector yang sebenarnya.
	OracleWindow time.Duration

	Thresholds Thresholds
}

// ---------------------------------------------------------------- Keluaran

// SupportingMetrics bernilai nil ketika tidak dapat dihitung.
// Nil berarti "tidak diketahui", bukan nol.
type SupportingMetrics struct {
	HolderTop1Pct         *decimal.Decimal
	HolderTop10Pct        *decimal.Decimal
	HolderHHI             *decimal.Decimal
	VolumeToSupplyD1      *decimal.Decimal
	VolumeToSupplyD7      *decimal.Decimal
	VolumeToSupplyD30     *decimal.Decimal
	LastGenuineTrade      *TradeRef
	TradesExcludedPct     *decimal.Decimal
	GenuineVolumeInWindow *decimal.Decimal
}

type TradeRef struct {
	LedgerSeq uint32
	At        time.Time
}

// AssetRisk adalah keluaran lengkap untuk satu aset pada satu ledger.
type AssetRisk struct {
	Base               Asset
	Quote              Asset
	LedgerSeq          uint32
	LedgerClosedAt     time.Time
	MethodologyVersion string
	DataSource         DataSource

	MidPrice    *decimal.Decimal // nil ketika PriceSource none
	PriceSource PriceSource
	SpreadPct   *decimal.Decimal

	Depth            []DepthPoint
	ManipulationCost []ManipulationPoint
	// MaxReachablePrice adalah harga ask tertinggi di buku, yaitu harga
	// tertinggi yang dapat dicapai penyerang. CostToMaxReachablePrice adalah
	// biaya mencapainya.
	//
	// Pasangan kedua angka ini menangkap serangan yang lolos dari tangga delta
	// diskret. Pada USTRY 21 Feb 2026 nilainya 106,7372828 dengan biaya nol,
	// dan serangan nyata terjadi di celah antara delta 0,5 dan 1.
	MaxReachablePrice       *decimal.Decimal
	CostToMaxReachablePrice *decimal.Decimal
	OracleResistance        *decimal.Decimal // MC(kritis) + volume asli dalam jendela
	MaxSafeCollateral       *decimal.Decimal

	Supporting SupportingMetrics

	Flags    []Flag
	Band     Band
	Warnings []string

	// Flag yang tidak dapat diperiksa karena data yang dibutuhkan tidak ada.
	// unevaluated BUKAN sinonim clear. Aset tanpa data trustline tidak boleh
	// terlihat sama dengan aset yang distribusi holder-nya sudah diperiksa.
	UnevaluatedFlags []Flag

	// partial jika ada flag bertingkat CRITICAL atau HIGH yang unevaluated,
	// full jika seluruhnya dapat diperiksa. Wajib ditampilkan di dashboard.
	BandConfidence BandConfidence
}

type BandConfidence string

const (
	BandConfidenceFull    BandConfidence = "full"
	BandConfidencePartial BandConfidence = "partial"
)
