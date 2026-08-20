# Keel: Flag dan Band Risiko

**Versi metodologi:** 1.0.2-draft
**Menggantikan:** PRD bagian 5.1 dan 5.2, yang sekarang cukup menunjuk ke dokumen ini
**Diimplementasikan di:** `internal/domain/flags.go`

Sebelumnya definisi flag berada di PRD sekaligus disinggung di metodologi. Dua tempat
untuk satu definisi menjamin keduanya menyimpang. Mulai versi ini, dokumen inilah
satu-satunya sumber kebenaran, dan PRD hanya menunjuk ke sini.

---

## 1. Kenapa aturan, bukan skor berbobot

Keel tidak menerbitkan skor 0 sampai 100 hasil pembobotan.

Skor berbobot mengharuskan pembuatnya membenarkan setiap bobot, dan tidak ada dasar
empiris untuk membenarkannya dari satu insiden. Klasifikasi berbasis aturan hanya
mengharuskan pembenaran atas ambangnya, dan setiap flag dapat diperiksa terpisah oleh
konsumen yang punya kebijakan sendiri.

Karena itu **flag selalu dilaporkan individual di API**, bukan hanya band hasil
turunannya. Konsumen yang tidak setuju dengan ambang Keel tetap dapat memakai flag
mentahnya.

---

## 2. Tiga keadaan setiap flag

Ini perbaikan penting pada versi 1.0.2. Sebelumnya flag hanya punya dua keadaan, dan
itu membuat aset dengan data tidak lengkap tampak lebih aman daripada seharusnya.

| Keadaan | Arti |
|---|---|
| `triggered` | kondisi terpenuhi |
| `clear` | kondisi diperiksa dan tidak terpenuhi |
| `unevaluated` | data yang dibutuhkan tidak tersedia, kondisi tidak dapat diperiksa |

`unevaluated` **bukan** sinonim `clear`. Aset tanpa data trustline tidak boleh terlihat
sama dengan aset yang distribusi holder-nya sudah diperiksa dan aman.

Konsekuensi pada keluaran:

- `flags` memuat flag yang `triggered`
- `unevaluatedFlags` memuat flag yang tidak dapat diperiksa
- `bandConfidence` bernilai `partial` jika ada flag bertingkat CRITICAL atau HIGH yang
  `unevaluated`, dan `full` jika seluruh flag pada kedua tingkat itu dapat diperiksa

Dashboard wajib menampilkan `bandConfidence`. Band `LOW` dengan confidence `partial`
adalah pernyataan yang jauh lebih lemah daripada `LOW` dengan confidence `full`, dan
perbedaan itu tidak boleh disembunyikan.

---

## 3. Data yang dibutuhkan tiap flag

| Flag | Butuh |
|---|---|
| `NO_EXECUTABLE_PRICE` | snapshot buku dan pool |
| `ZERO_DEPTH_2PCT` | snapshot buku dan pool |
| `SPREAD_EXTREME` | snapshot buku |
| `THIN_DEPTH_5PCT` | snapshot buku dan pool |
| `MANIPULATION_CHEAP` | snapshot buku dan pool |
| `MANIPULATION_RATIO_LOW` | snapshot dan supply beredar |
| `NO_GENUINE_TRADE_7D` | riwayat trade |
| `NO_GENUINE_TRADE_30D` | riwayat trade |
| `WASH_TRADE_SUSPECTED` | riwayat trade |
| `HOLDER_CONCENTRATION_HIGH` | distribusi trustline |
| `HOLDER_CONCENTRATION_EXTREME` | distribusi trustline |

Lima flag pertama dapat dievaluasi dari `Snapshot` saja. Enam sisanya membutuhkan
masukan tambahan, dan menjadi `unevaluated` ketika masukan itu tidak ada.

---

## 4. Definisi flag

Seluruh ambang mengacu pada `Thresholds` di `types.go` dan dinyatakan dalam **aset
quote**, bukan USD.

### Tingkat CRITICAL

**`NO_EXECUTABLE_PRICE`**
```
priceSource == none
```
Tidak ada orderbook dan tidak ada pool. Aset tidak punya harga eksekutabel sama sekali.

**`ZERO_DEPTH_2PCT`**
```
depth(0.02).BuySide == 0  ATAU  depth(0.02).SellSide == 0
```
Salah satu sisi saja sudah cukup. Aset yang tidak bisa dijual sama berbahayanya dengan
aset yang tidak bisa dibeli.

**`MANIPULATION_CHEAP`**
```
ADA delta d sehingga:
    Reachable(d) == true  DAN  Cost(d) < Thresholds.ManipulationCheapAbsolute
```

**Syarat `Reachable == true` adalah perbaikan versi 1.0.2 dan tidak boleh dihilangkan.**

Alasannya konkret. Pada fixture USTRY, `Cost` bernilai 130,0627093 untuk δ = 1, 10, dan
100, tetapi ketiganya `Reachable = false` karena tidak ada ask di atas 106,7372828.
Tanpa syarat ini, ketiga baris itu akan ikut dinilai, padahal keduanya justru
membuktikan serangan ke harga setinggi itu **tidak mungkin**. Menandai keadaan mustahil
sebagai "murah" adalah kebalikan dari kenyataan.

Sebaliknya `Cost(δ=0.5) = 0` dengan `Reachable = true` pada fixture yang sama adalah
kondisi paling berbahaya yang bisa ada, dan justru itulah yang harus tertangkap.

### Tingkat HIGH

**`MANIPULATION_RATIO_LOW`**
```
ADA delta d sehingga:
    Reachable(d) == true
    DAN  Cost(d) / nilai_supply_beredar < Thresholds.ManipulationRatioLowPct
```
Syarat `Reachable` berlaku dengan alasan yang sama seperti di atas.

**`SPREAD_EXTREME`**

Keduanya dinyatakan dalam PERSEN, bukan pecahan. `spreadPct` 196,08 dibandingkan
dengan `SpreadExtremePct` 20,0. Kalau salah satunya ditulis sebagai pecahan, flag ini
diam-diam tidak pernah menyala dan tidak ada yang gagal.

```
spreadPct > Thresholds.SpreadExtremePct
    dengan spreadPct = (best_ask − best_bid) / P0 × 100
```
Flag baru pada versi 1.0.2. Ketika spread mencapai ratusan persen, `P0` dan seluruh
metrik turunannya kehilangan makna. Pada fixture USTRY nilainya 196,08 persen, yang
berarti harga acuan 53,90 untuk aset bernilai sekitar 1,06.

Flag lain memang tetap menyala pada kasus itu, tetapi itu kebetulan dan bukan desain.
`spreadPct` juga dilaporkan sebagai angka, bukan hanya status terpicu, karena
besarannya informatif.

**`NO_GENUINE_TRADE_30D`**
```
tidak ada trade asli dalam Thresholds.GenuineTradeStaleDays hari terakhir
```

**`HOLDER_CONCENTRATION_EXTREME`**
```
holderTop1Pct > Thresholds.HolderTop1ExtremePct
```

### Tingkat MEDIUM

**`THIN_DEPTH_5PCT`**
```
min(depth(0.05).BuySide, depth(0.05).SellSide) < Thresholds.ThinDepth5PctAbsolute
```

**`NO_GENUINE_TRADE_7D`**
```
tidak ada trade asli dalam Thresholds.GenuineTradeWarnDays hari terakhir
```

**`HOLDER_CONCENTRATION_HIGH`**
```
holderTop10Pct > Thresholds.HolderTop10HighPct
```

**`WASH_TRADE_SUSPECTED`**
```
tradesExcludedPct > Thresholds.WashTradeSuspectedPct
```

---

## 5. Penurunan band

Band adalah tingkat tertinggi di antara flag yang `triggered`. Tidak ada pembobotan,
tidak ada rata-rata, tidak ada penjumlahan.

| Band | Terpicu bila ada flag tingkat |
|---|---|
| `CRITICAL` | CRITICAL |
| `HIGH` | HIGH |
| `MEDIUM` | MEDIUM |
| `LOW` | tidak ada flag terpicu |

`bandConfidence` ditentukan terpisah sesuai bagian 2.

---

## 6. Nilai ambang default

| Ambang | Default | Satuan |
|---|---|---|
| `ManipulationCheapAbsolute` | 10.000 | aset quote |
| `ManipulationRatioLowPct` | 1,0 | persen |
| `ThinDepth5PctAbsolute` | 50.000 | aset quote |
| `SpreadExtremePct` | 20,0 | persen |
| `HolderTop1ExtremePct` | 50,0 | persen |
| `HolderTop10HighPct` | 80,0 | persen |
| `WashTradeSuspectedPct` | 50,0 | persen |
| `GenuineTradeStaleDays` | 30 | hari |
| `GenuineTradeWarnDays` | 7 | hari |

**Seluruh nilai ini dipilih, bukan dikalibrasi terhadap kumpulan insiden.** Kalibrasi
memerlukan lebih banyak kejadian daripada yang tersedia. Pernyataan ini wajib muncul di
endpoint `/methodology`, di dashboard, dan di laporan backtest, bukan hanya di dokumen
ini.

### Keterbatasan satuan yang belum terselesaikan

Ambang absolut dinyatakan dalam aset quote. Konsekuensinya, aset yang diukur terhadap
XLM dan aset yang diukur terhadap USDC tidak dapat dibandingkan langsung dengan ambang
yang sama, dan band sebuah aset dapat berubah hanya karena harga XLM bergerak, tanpa
likuiditas aset itu berubah sama sekali.

Menyatakan ambang dalam USDC memindahkan masalahnya, karena memasukkan asumsi bahwa
USDC stabil, yang agak ironis bagi produk yang mempertanyakan asumsi harga.

Versi 1.0.2 belum memutuskan. Yang dilakukan sekarang: `quote` selalu disertakan di
setiap respons sehingga konsumen tahu satuan yang berlaku, dan keterbatasan ini
dinyatakan terbuka. Ini keputusan terbuka Q7 dan harus diselesaikan sebelum versi 1.1.

---

## 7. Contoh terverifikasi: USTRY/USDC, ledger 61340263

Diambil dari `testdata/fixtures/ustry_pre_exploit.md`, dihitung dengan tangan sebelum
implementasi ada.

Masukan: satu ask 1,2185312 USTRY pada 106,7372828, satu bid 0,0001 USTRY pada 1,057,
tanpa pool. `P0 = 53,8971414`, `spreadPct = 196,08%`.

**Triggered**

| Flag | Tingkat | Alasan |
|---|---|---|
| `ZERO_DEPTH_2PCT` | CRITICAL | depth ±2% nol di kedua sisi |
| `MANIPULATION_CHEAP` | CRITICAL | `Cost(δ=0.5) = 0` dengan `Reachable = true` |
| `SPREAD_EXTREME` | HIGH | 196,08% melewati 20% |
| `THIN_DEPTH_5PCT` | MEDIUM | depth 5% nol |

**Clear**

`NO_EXECUTABLE_PRICE`, karena `priceSource = book`.

**Unevaluated**

`MANIPULATION_RATIO_LOW`, `NO_GENUINE_TRADE_30D`, `NO_GENUINE_TRADE_7D`,
`WASH_TRADE_SUSPECTED`, `HOLDER_CONCENTRATION_EXTREME`, `HOLDER_CONCENTRATION_HIGH`.
Keenamnya membutuhkan data supply, riwayat trade, atau distribusi trustline yang tidak
ada di snapshot.

**Hasil**

```
band            = CRITICAL
bandConfidence  = partial
```

`partial` karena `MANIPULATION_RATIO_LOW` dan `HOLDER_CONCENTRATION_EXTREME` bertingkat
HIGH tetapi `unevaluated`. Band tetap `CRITICAL` sebab sudah ada dua flag CRITICAL yang
terpicu, sehingga ketidaklengkapan data tidak mengubah kesimpulan pada kasus ini.

---

## 8. Perubahan yang menyertai versi ini

| Berkas | Perubahan |
|---|---|
| `internal/domain/types.go` | tambah `UnevaluatedFlags []Flag` dan `BandConfidence` pada `AssetRisk` |
| `docs/api/keel-openapi.yaml` | tambah `unevaluatedFlags`, `bandConfidence`, `spreadPct`, `SPREAD_EXTREME` |
| PRD bagian 5 | ganti isinya dengan penunjuk ke dokumen ini |
| `/methodology` | tambah `spreadExtremePct` pada `thresholds` |

## 9. Riwayat versi

| Versi | Perubahan |
|---|---|
| 1.0.0-draft | Sepuluh flag awal, band sebagai flag terburuk yang terpicu |
| 1.0.1-draft | `SPREAD_EXTREME` ditambahkan setelah fixture menunjukkan `P0` kehilangan makna pada spread ekstrem |
| 1.0.2-draft | `MANIPULATION_CHEAP` dan `MANIPULATION_RATIO_LOW` disyaratkan `Reachable == true`. Keadaan `unevaluated` dan `bandConfidence` ditambahkan setelah fixture menunjukkan enam flag tidak dapat dinilai dari snapshot saja |
