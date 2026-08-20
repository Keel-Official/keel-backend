# Keel Deliverable 1: Rencana Eksekusi

**Janji SOW:** Liquidity Depth Engine. Backend yang membaca orderbook SDEX, reserve AMM pool, dan distribusi trustline. Per aset menghitung effective depth di +/-2%, +/-5%, +/-10%, holder concentration, rasio volume-to-supply, waktu sejak trade asli terakhir, dan rekomendasi ukuran collateral maksimum yang aman. Mendukung historical replay di state ledger sebelumnya. Metodologi terdokumentasi dan reproducible.

**Anggaran:** 126 jam = $2.268
**Bukti yang harus diserahkan:** repo publik + dokumen metodologi + hasil cross-validation terhadap Horizon di sample ledger.

---

## 1. Alokasi 126 jam

| Komponen | Jam | Kapan |
|---|---|---|
| D1.1 Lapisan akses data (Horizon + Hubble) | 24 | Minggu 1 |
| D1.2 Inti perhitungan depth | 22 | Minggu 1 akhir sampai Minggu 2 |
| D1.3 Metrik pendukung | 20 | Minggu 2 |
| D1.4 Ukuran collateral aman | 12 | Minggu 2 |
| D1.5 Historical replay | 20 | Minggu 3 awal |
| D1.6 Harness validasi + 50 sample ledger | 16 | Berjalan sejak Minggu 1 |
| D1.7 Dokumen metodologi | 12 | Berjalan terus, difinalkan Minggu 3 |
| **Total** | **126** | |

Perhatikan D1.6 dan D1.7 bukan tahap di akhir. Keduanya berjalan paralel sejak hari pertama. Kalau ditunda, keduanya tidak akan selesai.

---

## 2. Enam keputusan definisi yang harus diambil sebelum koding

SOW menyebut metrik tapi tidak mendefinisikannya. Definisi adalah pekerjaan intelektual Deliverable 1, bukan kodenya. Ambil keputusan ini di Day 4 sampai 6, tulis alasannya, lalu implementasi mengikuti.

### D-1. Depth diukur terhadap aset apa?

Aset Stellar bisa diperdagangkan terhadap banyak counter-asset sekaligus (XLM, USDC, aset lain). "Depth USTRY" tidak bermakna tanpa menyebut lawannya.

Rekomendasi: hitung depth untuk **setiap pasangan yang punya likuiditas apa pun**, lalu tetapkan satu pasangan primer (yang total depth di 10% paling besar), dan laporkan keduanya. Alasannya: penyerang akan memakai jalur termurah, jadi mengabaikan pasangan sekunder membuat Anda terlalu optimistis. Tapi angka utama di dashboard butuh satu nilai.

Catat juga: Stellar punya path payment yang bisa merutekan lewat aset perantara. Ini menambah likuiditas efektif yang tidak terlihat di pasangan langsung. Untuk 30 hari, nyatakan sebagai keterbatasan yang diketahui, jangan coba implementasi path finding.

### D-2. Mid price ketika book kosong atau satu sisi

Kasus ini sering pada aset tipis, dan aset tipis justru target utama Keel.

Urutan fallback yang disarankan:
1. Ada bid dan ask: `P0 = (best_bid + best_ask) / 2`
2. Hanya satu sisi book: pakai harga spot pool kalau ada, catat warning
3. Tidak ada book, hanya pool: `P0 = reserveQuote / reserveBase`
4. Tidak ada keduanya: **`priceSource = 'none'`**

Kasus 4 bukan error dan tidak boleh melempar exception. Aset yang tidak punya harga eksekutabel adalah aset paling berbahaya yang bisa Anda temukan. Ia harus muncul sebagai hasil dengan skor risiko maksimum, bukan sebagai baris yang hilang dari laporan.

### D-3. Level yang melewati batas harga: ambil penuh atau tidak sama sekali?

Saat berjalan menyusuri book, level terakhir biasanya melewati `P_limit`. Dua pilihan: buang level itu (konservatif) atau ambil sebagian sampai persis di batas.

Rekomendasi: **buang.** Lebih konservatif, lebih sederhana, dan untuk produk yang tujuannya memperingatkan risiko, bias konservatif adalah arah yang benar. Dokumentasikan bahwa ini pilihan sadar, dan sebutkan bahwa ini membuat depth SDEX sedikit lebih rendah dari nilai teoretis.

### D-4. Apa itu "trade asli" (genuine trade)?

SOW menjanjikan "time since the last genuine trade". Kata *genuine* itu yang membuat metriknya bernilai, karena wash trading adalah cara termudah membuat aset mati terlihat hidup.

Definisi minimal yang bisa Anda implementasi dalam 30 hari:

Sebuah trade **tidak dihitung** kalau salah satu berikut benar:
- Akun pembeli sama dengan akun penjual
- Notional di bawah ambang debu (misal setara $10)
- Salah satu sisi adalah akun issuer aset itu sendiri
- Kedua sisi adalah akun yang trustline-nya dibuat dalam jendela waktu yang sama dan hanya pernah bertransaksi satu sama lain (opsional, mahal, tandai sebagai v2 kalau waktu sempit)

Yang penting: **laporkan berapa banyak trade yang dikecualikan dan kenapa.** Angka "87% volume 30 hari terakhir dikecualikan sebagai non-genuine" jauh lebih kuat daripada sekadar tanggal. Ini juga membuat metode Anda bisa diaudit orang lain.

### D-5. Holder concentration dihitung dari populasi apa?

Sumber: trustline. Tapi ada pengecualian yang harus diputuskan:

- **Akun issuer:** dikecualikan. Issuer memegang supply yang belum beredar.
- **Reserve liquidity pool:** aset yang terkunci di pool bukan dipegang holder. Putuskan apakah masuk denominator, dan konsisten dengan definisi supply di D-6.
- **Akun kustodian atau bursa:** tidak bisa dideteksi otomatis dengan andal. Jangan coba. Sebutkan sebagai keterbatasan.

Metrik yang dilaporkan: share top-1, share top-10, dan HHI. HHI lebih tahan terhadap ekor panjang daripada Gini dan lebih mudah dijelaskan.

**Catatan teknis penting:** jangan mengambil daftar holder dari Horizon `/accounts?asset=`. Untuk aset dengan ribuan trustline, paginasinya akan menghabiskan kuota rate limit Anda (Horizon publik membatasi sekitar 3600 request per jam per IP). Ambil dari Hubble tabel `trust_lines` dalam satu query. Ini juga otomatis memberi Anda versi historisnya, yang dibutuhkan D1.5.

### D-6. Supply mana yang dipakai untuk rasio volume-to-supply?

Pilihan: total yang diterbitkan, total yang dipegang trustline, atau supply beredar setelah mengurangi kepemilikan issuer dan pool.

Rekomendasi: **supply yang dipegang trustline non-issuer.** Ini yang paling dekat dengan "berapa banyak yang benar-benar bisa dijual seseorang". Volume diambil dari `/trade_aggregations` (atau `history_trades` di Hubble untuk historis) dengan jendela 24 jam, 7 hari, dan 30 hari. Laporkan ketiganya, karena aset tipis sering punya volume 24 jam nol tapi 30 hari tidak nol.

---

## 3. Komponen, satu per satu

### D1.1 Lapisan akses data (24 jam)

Dua klien, satu bentuk keluaran.

```typescript
type Asset = { code: string; issuer: string | null };  // null berarti XLM native

type Level = { price: number; amount: number };        // amount dalam unit base

type PoolReserves = {
  poolId: string;
  reserveBase: number;
  reserveQuote: number;
  feeBp: number;                                        // 30 untuk pool Stellar
};

type Snapshot = {
  base: Asset;
  quote: Asset;
  ledgerSeq: number;
  closedAt: string;
  book: { bids: Level[]; asks: Level[] };
  pools: PoolReserves[];
  source: 'horizon' | 'hubble';
};
```

`horizonClient.getSnapshot(base, quote)` dan `hubbleClient.getSnapshot(base, quote, ledgerSeq)` mengembalikan bentuk yang identik. Seluruh sisa engine tidak boleh tahu dari mana snapshot datang. Ini yang membuat D1.5 murah.

Yang harus ada di klien Horizon: retry dengan backoff, penghitung request untuk menjaga rate limit, cache dengan TTL, dan normalisasi harga (Horizon mengembalikan harga sebagai pecahan `price_r` numerator/denominator, jangan pakai field `price` string tanpa memeriksa presisi).

Yang harus ada di klien Hubble: pemangkasan partisi yang ketat (`WHERE batch_run_date BETWEEN ...`), tidak ada `SELECT *`, dan cache hasil ke disk lokal karena setiap query berbiaya uang.

### D1.2 Inti perhitungan depth (22 jam)

```typescript
function computeDepth(snapshot: Snapshot, deltas: number[]): DepthResult;
```

Fungsi murni. Tidak ada fetch, tidak ada I/O, tidak ada jam sistem.

Algoritma untuk satu delta, sisi beli:

```
P0        = midPrice(snapshot)
P_limit   = P0 * (1 + delta)
n_sdex    = jumlah (price * amount) untuk setiap ask dengan price <= P_limit
n_amm     = untuk tiap pool: reserveQuote * (sqrt(P_limit / P_pool) - 1),
            nol kalau P_pool >= P_limit, lalu digross-up dengan fee
depth     = n_sdex + n_amm
```

Sisi jual simetris memakai bids dan `P0 * (1 - delta)`.

Test yang wajib ada sebelum modul ini dianggap selesai:
1. Fixture testnet dengan orderbook buatan yang depth-nya Anda hitung manual di kertas.
2. Fixture pool murni tanpa orderbook, dicek terhadap aturan jempol `depth ≈ (delta/2) * reserveQuote`.
3. Kasus tepi: book kosong, satu sisi kosong, pool kosong, dua pool untuk pasangan yang sama, aset tanpa harga sama sekali.
4. Uji monotonisitas: `depth(2%) <= depth(5%) <= depth(10%)`. Kalau ini gagal, ada bug di logika penggabungan.

Test nomor 4 murah dan menangkap sebagian besar kesalahan penggabungan.

### D1.3 Metrik pendukung (20 jam)

Implementasi D-4, D-5, dan D-6 di atas. Semuanya menerima snapshot atau data historis dengan `ledgerSeq`, dan semuanya mengembalikan nilai plus daftar pengecualian yang diterapkan.

Pola yang disarankan: setiap metrik mengembalikan `{ value, excluded, warnings }`. Kolom `excluded` itulah yang membuat metodologi Anda bisa diperiksa orang lain, dan itu yang membedakan laporan yang dipercaya dari laporan yang dianggap kotak hitam.

### D1.4 Ukuran collateral aman (12 jam)

```
C_max = min( D_jual(delta_likuidasi) * h , biaya_manipulasi(delta_kritis) * m )
```

Dua parameter harus punya nilai default yang bisa dipertahankan, bukan angka yang Anda karang. Cara mendapatkannya: baca parameter risiko Blend yang sebenarnya (liquidation threshold, close factor, liquidation incentive), pakai itu sebagai default, dan **sebutkan sumbernya di dokumentasi**. Kalimat "default kami berasal dari parameter Blend yang berlaku pada Mei 2026" jauh lebih kuat daripada "kami memakai haircut 50%".

Buat semuanya bisa dikonfigurasi. Keel bersifat agnostik terhadap protokol, jadi API harus mengizinkan pemanggil memasukkan parameternya sendiri.

Laporkan dua sisi terpisah, jangan digabung jadi satu angka:
- **Batas likuidasi** dari depth sisi jual, menjawab "kalau posisi ini dilikuidasi, apakah pasar sanggup menyerapnya"
- **Batas manipulasi** dari biaya menaikkan harga, menjawab "apakah memompa harga aset ini lebih murah daripada nilai yang bisa dicuri"

Aset bisa lolos satu dan gagal yang lain. Ini pembeda utama Keel dari alat pemantauan yang sudah ada.

### D1.5 Historical replay (20 jam)

Karena `computeDepth()` sudah murni, komponen ini hanya berarti membangun `hubbleClient.getSnapshot(base, quote, ledgerSeq)` yang benar.

Pekerjaan sebenarnya ada di validasi, bukan di pengambilan data. Urutan yang wajib:

1. Ambil snapshot Hubble untuk ledger yang Anda punya rekaman Horizon-nya (lihat bagian 4).
2. Bandingkan level per level, bukan hanya total. Perbedaan pada satu offer besar bisa tersembunyi di angka agregat.
3. Baru setelah cocok, jalankan pada rentang tanggal apa pun.

Kalau Hubble ternyata tidak menyimpan snapshot `offers` cukup rapat, aktifkan jalur rekonstruksi dari event: ambil snapshot terdekat sebelum ledger target, lalu terapkan berurutan operasi `manage_buy_offer`, `manage_sell_offer`, dan trade dari `history_operations` sampai ledger target. Ini menambah 3 sampai 5 hari kerja dan harus dibayar dengan pemotongan scope Deliverable 3.

### D1.6 Harness validasi (16 jam)

Ini yang menghasilkan "cross-validation results" yang dijanjikan di tabel bukti SOW. Rancangannya dijelaskan di bagian 4.

### D1.7 Dokumen metodologi (12 jam)

Struktur yang disarankan, satu file per bagian di `docs/methodology/`:

```
00-ikhtisar.md              apa yang diukur dan kenapa
01-sumber-data.md           Horizon vs Hubble, batasan masing-masing
02-harga-acuan.md           definisi P0 dan urutan fallback (D-2)
03-depth-sdex.md            walk the book, perlakuan level terakhir (D-3)
04-depth-amm.md             penurunan rumus lengkap, perlakuan fee
05-penggabungan.md          kenapa tidak dijumlahkan langsung
06-pemilihan-pasangan.md    numeraire dan pasangan primer (D-1)
07-metrik-pendukung.md      genuine trade, konsentrasi, volume-supply (D-4 s.d. D-6)
08-collateral.md            rumus C_max dan asal parameter defaultnya
09-validasi.md              protokol cross-validation dan hasilnya
10-keterbatasan.md          apa yang metode ini tidak tangkap
```

File `10-keterbatasan.md` adalah yang paling berpengaruh terhadap kredibilitas Anda. Isi minimal: likuiditas tercatat tidak sama dengan likuiditas eksekutabel (offer bisa ditarik seketika), path payment lewat aset perantara tidak dihitung, likuiditas off-chain di bursa terpusat tidak terlihat, dan ambang aman dipilih bukan dikalibrasi dari banyak insiden.

Reviewer yang berpengalaman akan mencari bagian ini. Ketiadaannya lebih merugikan daripada isinya.

---

## 4. Protokol cross-validation

SOW menjanjikan "cross-validation passed on at least 50 sample ledgers" tapi tidak mendefinisikan apa yang divalidasi terhadap apa. Anda yang harus mendefinisikannya, dan definisinya menentukan seberapa meyakinkan buktinya.

Tiga lapis, dari yang paling murah ke yang paling kuat:

**Lapis 1: hitung ulang manual (5 aset).**
Ambil orderbook mentah, salin ke spreadsheet, hitung depth dengan tangan. Bandingkan dengan output engine. Lampirkan spreadsheet-nya di repo. Ini membuktikan rumusnya benar.

**Lapis 2: fixture testnet (10 skenario).**
Aset dan orderbook yang Anda buat sendiri di testnet dengan angka yang Anda tentukan. Hasil yang benar diketahui sebelum kode dijalankan. Ini membuktikan implementasinya benar.

**Lapis 3: Horizon live versus Hubble historis (50+ pasangan).**
Ini yang memenuhi janji SOW dan ini yang butuh dimulai sekarang.

Cara kerjanya:

```
Mulai Day 2:
  cron tiap 30 menit
  untuk 8 aset terpilih:
    snapshot = horizonClient.getSnapshot(...)
    simpan mentah ke recordings/{asset}/{ledgerSeq}.json

Mulai Day 16 (setelah Hubble mengejar):
  untuk setiap rekaman:
    h = hubbleClient.getSnapshot(asset, ledgerSeq yang sama)
    bandingkan: jumlah level, harga tiap level, amount tiap level,
                reserve pool, dan hasil computeDepth() dari keduanya
    catat: cocok / beda / selisih
```

Dua minggu perekaman pada 8 aset tiap 30 menit menghasilkan ribuan pasangan. Anda memilih 50 sebagai sampel yang dilaporkan, tapi Anda punya seluruhnya sebagai cadangan. Hasilnya masuk ke `docs/methodology/09-validasi.md` sebagai tabel: aset, ledger, hasil, selisih.

Kalau ada yang tidak cocok, itu bukan kegagalan. Selisih yang dijelaskan dengan benar (misalnya Hubble mengambil snapshot pada batas batch sehingga tertinggal beberapa ledger) justru menunjukkan Anda memahami datanya. Yang berbahaya adalah tidak pernah membandingkan.

---

## 5. Urutan pengerjaan yang mengurangi risiko

Jangan bangun komponen sampai selesai satu per satu. Bangun irisan tipis yang tembus dari ujung ke ujung dulu.

**Irisan 1 (Day 2 sampai 5):** satu aset, satu pasangan, satu delta, hanya SDEX, tanpa pool, tanpa metrik pendukung, hasil dicetak ke terminal. Tujuannya membuktikan seluruh rantai jalan.

**Irisan 2 (Day 6 sampai 9):** tambahkan AMM dan penggabungan. Tambahkan ketiga delta. Masih satu aset.

**Irisan 3 (Day 10 sampai 13):** tambahkan metrik pendukung dan C_max. Jalankan pada 50 aset, simpan ke database.

**Irisan 4 (Day 16 sampai 19):** tukar sumber data ke Hubble. Kalau irisan 1 sampai 3 dirancang benar, ini hanya mengganti satu implementasi antarmuka.

Kalau Anda menemukan bahwa irisan 4 membutuhkan perubahan pada `depth.ts`, berarti ada kebocoran abstraksi dan itu harus diperbaiki segera, bukan ditambal.

---

## 6. Definition of Done Deliverable 1

Centang semua sebelum menyatakan selesai ke Ambassador Chapter Lead:

**Kode**
- [ ] Repo publik, README berisi cara menjalankan dari nol
- [ ] `computeDepth()` fungsi murni, tidak ada I/O di dalamnya
- [ ] Hasil untuk 50+ aset tersimpan dan bisa di-query
- [ ] Historical replay jalan dan sudah divalidasi terhadap ledger kontrol
- [ ] Test lolos untuk 10 fixture testnet dan seluruh kasus tepi di D1.2
- [ ] Semua hasil membawa `ledgerSeq`

**Metodologi**
- [ ] Sebelas file di `docs/methodology/` lengkap
- [ ] Setiap keputusan D-1 sampai D-6 tertulis beserta alasannya
- [ ] Penurunan rumus AMM ada dan bisa diikuti pembaca luar
- [ ] File keterbatasan ditulis jujur dan spesifik
- [ ] Orang luar bisa mereproduksi satu angka dari nol hanya dengan membaca dokumen

**Validasi**
- [ ] Spreadsheet hitung ulang manual untuk 5 aset ada di repo
- [ ] Hasil perbandingan 50+ pasangan Horizon versus Hubble tertabulasi
- [ ] Selisih yang muncul sudah dijelaskan, bukan diabaikan

**Uji terakhir yang paling penting**
- [ ] Kedua builder bisa menjelaskan, tanpa melihat catatan, kenapa depth SDEX dan AMM tidak boleh dijumlahkan secara terpisah, dan kenapa risiko likuidasi ada di sisi jual sedangkan risiko manipulasi ada di sisi beli.

Kalau butir terakhir gagal, Deliverable 1 belum selesai meskipun semua kode jalan. Itulah yang akan diuji saat aplikasi SCF Build.
