package conformance

import (
	"github.com/shopspring/decimal"

	"github.com/Keel-Official/keel/internal/domain"
)

// Nilai harapan untuk GoldenSnapshot. Sumber: testdata/fixtures/ustry_pre_exploit.md.
//
// Setiap nol di berkas ini punya alasan tertulis, karena nol yang benar dan nol
// karena bug terlihat sama persis di keluaran.

// ---------------------------------------------------------------- Harga

var (
	// (1,057 + 106,7372828) / 2. Kedua sisi buku terisi, jadi priceSource book.
	// Nilainya 53,90 untuk aset yang sebenarnya bernilai sekitar 1,06. Itu
	// bukan bug, melainkan sifat mid price ketika spread ribuan persen.
	ExpectedP0          = dec("53.8971414")
	ExpectedPriceSource = domain.PriceSourceBook

	// (106,7372828 − 1,057) / 53,8971414, dinyatakan dalam PERSEN.
	// Nilai eksaknya 528401414/269485707 yang desimalnya tidak berujung,
	// sehingga perbandingan wajib memakai Tolerance.
	ExpectedSpreadPct = dec("196.0777140585048")
)

// ---------------------------------------------------------------- Depth

// ExpectedDepthPoint adalah satu baris tangga depth yang diharapkan.
type ExpectedDepthPoint struct {
	Delta    decimal.Decimal
	BuySide  decimal.Decimal
	SellSide decimal.Decimal
	FromSdex decimal.Decimal
	FromAmm  decimal.Decimal
	Reason   string
}

// ExpectedDepth seluruhnya nol, dan nol itu BENAR.
//
// Satu-satunya ask berharga 106,7372828, jauh di atas seluruh target beli
// (54,975084228 / 56,591998470 / 59,286855540). Satu-satunya bid berharga
// 1,057, jauh di bawah seluruh target jual (52,819198572 / 51,202284330 /
// 48,507427260). Tidak ada satu pun level yang jatuh di dalam pita.
//
// FromAmm nol karena Pools kosong, bukan karena kurva tidak tersentuh.
var ExpectedDepth = []ExpectedDepthPoint{
	{dec("0.02"), decimal.Zero, decimal.Zero, decimal.Zero, decimal.Zero,
		"target beli 54,975084228 dan target jual 52,819198572, tak ada level di dalam pita"},
	{dec("0.05"), decimal.Zero, decimal.Zero, decimal.Zero, decimal.Zero,
		"target beli 56,591998470 dan target jual 51,202284330, tak ada level di dalam pita"},
	{dec("0.10"), decimal.Zero, decimal.Zero, decimal.Zero, decimal.Zero,
		"target beli 59,286855540 dan target jual 48,507427260, tak ada level di dalam pita"},
}

// ---------------------------------------------------------------- Manipulasi

// ExpectedManipulationPoint adalah satu baris tangga biaya manipulasi.
type ExpectedManipulationPoint struct {
	Delta       decimal.Decimal
	TargetPrice decimal.Decimal
	Cost        decimal.Decimal
	Reachable   bool
	Reason      string
}

// ExpectedManipulation adalah baris paling mudah disalahpahami di seluruh
// fixture, dan versi test sebelumnya memang menyalahpahaminya di dua baris.
//
// Cost dan Reachable memakai himpunan ask yang BERBEDA:
//
//	Cost      menjumlahkan ask dengan price <  target
//	Reachable memeriksa keberadaan ask dengan price >= target
//
// Sebuah ask tidak pernah masuk keduanya sekaligus. Karena itu baris δ=0,5
// punya Cost nol DAN Reachable true secara bersamaan, dan itu adalah kondisi
// paling berbahaya yang bisa ada: harga 80,85 dapat dicapai tanpa membayar
// apa pun kepada pihak ketiga.
//
// Sebaliknya δ=1, 10, dan 100 punya Cost 130,06 tetapi Reachable false. Angka
// 130,06 di situ TIDAK boleh dibaca sebagai "harga itu mahal dicapai", sebab
// harga itu tidak dapat dicapai sama sekali; buku habis sebelum sampai ke sana.
var ExpectedManipulation = []ExpectedManipulationPoint{
	{dec("0.5"), dec("80.8457121"), decimal.Zero, true,
		"tak ada ask lebih murah dari 80,85 sehingga Cost nol, tetapi ask 106,74 memenuhi >= target sehingga Reachable"},
	{dec("1"), dec("107.7942828"), dec("130.06270929502336"), false,
		"ask 106,74 lebih murah dari target sehingga masuk Cost, dan tak ada ask >= 107,79"},
	{dec("10"), dec("592.8685554"), dec("130.06270929502336"), false,
		"idem, buku sudah habis jauh di bawah target"},
	{dec("100"), dec("5443.6112814"), dec("130.06270929502336"), false,
		"idem, buku sudah habis jauh di bawah target"},
}

// ---------------------------------------------------------------- Jangkauan

var (
	// Harga ask tertinggi di buku, yaitu harga tertinggi yang dapat dicapai
	// penyerang. Rasionya 100,98 kali terhadap harga sebenarnya 1,057.
	ExpectedMaxReachablePrice = dec("106.7372828")

	// Nol karena tidak ada satu pun ask yang lebih murah dari 106,7372828.
	// Mencapai harga tertinggi di buku ini GRATIS.
	//
	// Pasangan kedua angka inilah baris terpenting di seluruh fixture. Serangan
	// nyata jatuh di celah antara delta 0,5 dan 1, sehingga terlewat oleh
	// tangga delta diskret dan hanya tertangkap di sini.
	ExpectedCostToMaxReachablePrice = decimal.Zero
)

// ---------------------------------------------------------------- Flag

// ExpectedFlags adalah flag yang harus TRIGGERED, seluruhnya dapat dinilai
// dari snapshot saja.
var ExpectedFlags = []domain.Flag{
	domain.FlagZeroDepth2Pct,     // CRITICAL: depth +/-2% nol di kedua sisi
	domain.FlagManipulationCheap, // CRITICAL: Cost(0,5) = 0 dengan Reachable true
	domain.FlagSpreadExtreme,     // HIGH:     196,08% melewati ambang 20%
	domain.FlagThinDepth5Pct,     // MEDIUM:   depth 5% nol, di bawah ambang absolut mana pun
}

// ExpectedUnevaluatedFlags membutuhkan data supply, riwayat trade, atau
// distribusi trustline yang tidak ada di Snapshot.
//
// Keenamnya wajib dilaporkan sebagai tidak dapat dinilai, BUKAN sebagai nol
// dan bukan sebagai clear. Nol berarti terukur nol, dan itu klaim yang berbeda.
var ExpectedUnevaluatedFlags = []domain.Flag{
	domain.FlagManipulationRatioLow,
	domain.FlagNoGenuineTrade30D,
	domain.FlagNoGenuineTrade7D,
	domain.FlagWashTradeSuspected,
	domain.FlagHolderConcentrationExtreme,
	domain.FlagHolderConcentrationHigh,
}

// ExpectedClearFlags sudah diperiksa dan tidak terpenuhi.
var ExpectedClearFlags = []domain.Flag{
	domain.FlagNoExecutablePrice, // priceSource book, jadi ada harga eksekutabel
}

var (
	// Tingkat tertinggi di antara flag yang triggered. Tidak ada pembobotan.
	ExpectedBand = domain.BandCritical

	// partial karena MANIPULATION_RATIO_LOW dan HOLDER_CONCENTRATION_EXTREME
	// bertingkat HIGH tetapi unevaluated. Band tetap CRITICAL sebab sudah ada
	// dua flag CRITICAL yang terpicu, sehingga ketidaklengkapan data tidak
	// mengubah kesimpulan PADA KASUS INI. Itu kebetulan, bukan jaminan.
	ExpectedBandConfidence = domain.BandConfidencePartial
)
