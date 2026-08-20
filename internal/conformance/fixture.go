// Package conformance memuat golden fixture Keel beserta nilai harapannya.
//
// Paket ini sengaja TERPISAH dari internal/depth. Ia hanya boleh memanggil API
// yang diekspor, sehingga uji kesesuaian bersifat black-box secara struktural
// dan bukan sekadar karena disiplin penulisnya. Konsekuensi lain yang
// disengaja: zona merah tidak perlu disentuh untuk merawat test.
//
// Seluruh angka di sini berasal dari testdata/fixtures/ustry_pre_exploit.md, yang
// dihitung dengan tangan SEBELUM implementasi ada. Jangan menyesuaikan angka
// ini agar cocok dengan kode. Sesuaikan kode agar cocok dengan angka ini.
package conformance

import (
	"time"

	"github.com/shopspring/decimal"

	"github.com/Keel-Official/keel/internal/domain"
)

// Toleransi perbandingan desimal.
//
// Dibutuhkan karena sebagian nilai harapan tidak berujung. spreadPct pada
// fixture ini adalah 528401414/269485707, yang desimalnya tidak pernah
// berhenti. Presisi perhitungan yang sebenarnya adalah konstanta metodologi
// dan belum ditetapkan; sampai itu diputuskan, toleransi ini yang berlaku.
var Tolerance = decimal.RequireFromString("0.0000001")

// dec adalah pembaca konstanta desimal. Panik pada masukan buruk, yang benar
// untuk fixture: konstanta salah ketik harus meledak saat itu juga.
func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

// ---------------------------------------------------------------- Aset

var (
	// USTRY berkode lima karakter sehingga bertipe alphanum12. Query Horizon
	// dengan alphanum4 mengembalikan hasil kosong tanpa pesan error.
	AssetUSTRY = domain.Asset{
		Code:   "USTRY",
		Issuer: "GCRYUGD5NVARGXT56XEZI5CIFCQETYHAPQQTHO2O3IQZTHDH4LATMYWC",
		Type:   domain.AssetTypeAlphanum12,
	}

	AssetUSDC = domain.Asset{
		Code:   "USDC",
		Issuer: "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN",
		Type:   domain.AssetTypeAlphanum4,
	}
)

// ---------------------------------------------------------------- Masukan

// GoldenSnapshot adalah state buku USTRY/USDC yang benar-benar ada on-chain
// sesaat sebelum ledger 61340263.
//
// Ask dipasang oleh op 263453036239003649 pada 21 Feb 23:38:51, bid oleh
// op 263453066303434753 pada 23:39:31. Keduanya milik akun yang SAMA,
// GCNF5GNRIT6VWYZ7LXUZ33Q3SR2NUGO32F5X65VVKAEWWIQCKGYN75HB. Bid 0,0001 itu
// ikut membentuk P0 tanpa mewakili likuiditas nyata apa pun, dan itulah alasan
// konkret P0 tidak boleh dibaca sendirian.
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

// DefaultParams memuat ambang default pada docs/methodology/09-flag-dan-band.md
// bagian 6.
//
// SATUAN: seluruh field berakhiran Pct dinyatakan dalam PERSEN, bukan pecahan.
// SpreadExtremePct bernilai 20 berarti 20 persen, dan dibandingkan dengan
// spreadPct yang bernilai 196,08 pada fixture ini. Konvensi ini diseragamkan
// karena HolderTop1ExtremePct dan kawan-kawan sudah memakai persen, sehingga
// pecahan akan menjadi satu-satunya pengecualian dan sumber bug diam.
//
// Seluruh nilai ini DIPILIH, bukan dikalibrasi terhadap kumpulan insiden.
func DefaultParams() domain.Params {
	return domain.Params{
		MarketDeltas:       []decimal.Decimal{dec("0.02"), dec("0.05"), dec("0.10")},
		ManipulationDeltas: []decimal.Decimal{dec("0.5"), dec("1"), dec("10"), dec("100")},

		LiquidationDelta:          dec("0.10"),
		LiquidationHaircut:        dec("0.5"),
		ManipulationCriticalDelta: dec("1.0"),
		ManipulationMargin:        dec("0.25"),

		// 15 menit adalah ASUMSI, belum dikonfirmasi sebagai jendela Reflector
		// yang sebenarnya.
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
