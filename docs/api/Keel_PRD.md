# Keel: Product Requirements Document

**Versi:** 1.0
**Tanggal:** Agustus 2026
**Status:** Aktif untuk sprint Instawards 30 hari
**Pemilik:** Ciganytry (Rafli Ahmad Denistri)

Dokumen ini melengkapi SOW, bukan menggantikannya. SOW adalah komitmen kepada pemberi dana tentang apa yang diserahkan. PRD ini adalah definisi produk untuk builder: siapa penggunanya, apa yang dibangun, dan bagaimana tahu itu benar.

---

## 1. Ringkasan produk

Keel adalah mesin risiko likuiditas untuk aset Stellar. Ia menjawab satu pertanyaan yang tidak dijawab siapa pun di ekosistem Stellar saat ini:

> Harga aset ini sekian. Berapa besar transaksi yang sanggup ditanggung harga itu?

Oracle menjawab "berapa harganya". Keel menjawab "seberapa dalam pasarnya". Perbedaan itulah yang hilang saat insiden Blend Mei 2026, ketika harga USTRY dinaikkan 100 kali lewat feed berlikuiditas tipis dan dipakai sebagai collateral untuk meminjam $61 juta XLM.

---

## 2. Pengguna

### 2.1 Pengguna sebenarnya selama 30 hari ini

Perlu kejujuran di sini, karena ini menentukan prioritas desain. Selama sprint Instawards, pengguna utama Keel **bukan** protokol lending. Mereka adalah:

| Pengguna                                                            | Kebutuhan                                               | Implikasi desain                                                                           |
| ------------------------------------------------------------------- | ------------------------------------------------------- | ------------------------------------------------------------------------------------------ |
| **P1. Ambassador Chapter Lead**                                     | Memverifikasi deliverable selesai tanpa keahlian teknis | Dashboard harus bisa dipahami tanpa penjelasan. Bukti harus berupa tautan yang bisa diklik |
| **P2. Reviewer SCF Build**                                          | Menilai apakah metodologinya kokoh dan gap-nya nyata    | Dokumen metodologi dan backtest yang bisa direproduksi lebih penting daripada fitur        |
| **P3. Calon pengguna teknis** (protokol, kurator vault, issuer RWA) | Menilai apakah metriknya berguna untuk keputusan mereka | API yang bisa dicoba dalam 5 menit, tanpa registrasi                                       |

Kesimpulan yang harus dipegang: **kejelasan mengalahkan kelengkapan.** Dashboard 30 aset yang jelas mengalahkan dashboard 50 aset yang membingungkan. Backtest yang bisa direproduksi orang lain mengalahkan tiga fitur tambahan.

### 2.2 Pengguna yang dituju setelah SCF Build

Protokol lending yang menetapkan parameter risiko collateral, kurator vault yang memilih aset, dan issuer RWA yang ingin membuktikan asetnya likuid. Kebutuhan mereka membentuk arah produk tapi **tidak** membentuk scope 30 hari.

---

## 3. Prinsip produk

Empat prinsip yang menyelesaikan perdebatan desain tanpa perlu diskusi ulang.

**P-1. Keel tidak bergantung pada oracle harga eksternal.**
Seluruh premis Keel adalah mempertanyakan apakah harga yang dilaporkan bisa dipercaya. Kalau Keel memakai feed harga pihak ketiga untuk mengonversi ke USD, argumennya melingkar dan mudah dipatahkan. Semua nilai dinyatakan dalam **aset quote-nya sendiri** (XLM atau USDC), bukan USD hasil konversi. Kalau tampilan USD dibutuhkan di dashboard, tandai jelas sebagai konversi indikatif dengan sumber yang disebutkan, dan jangan pernah pakai di perhitungan.

**P-2. Bias konservatif di setiap kasus ambigu.**
Kalau ada dua interpretasi yang wajar, pilih yang menghasilkan angka depth lebih rendah dan penilaian risiko lebih tinggi. Produk peringatan yang terlalu optimistis tidak berguna. Tulis setiap pilihan konservatif ini di dokumen metodologi.

**P-3. Setiap angka harus bisa ditelusuri.**
Semua keluaran membawa `ledgerSeq`, `methodologyVersion`, dan `dataSource`. Angka tanpa ketiganya tidak bisa diverifikasi ulang dan karenanya tidak bernilai untuk Deliverable 1.

**P-4. Ketiadaan data adalah temuan, bukan error.**
Aset tanpa orderbook, tanpa pool, atau tanpa trade adalah aset paling berbahaya yang bisa ditemukan. Ia harus muncul di keluaran dengan penanda risiko tertinggi, bukan hilang dari daftar atau melempar exception.

---

## 4. Requirement fungsional

Prioritas: **M** = must (tanpa ini deliverable gagal), **S** = should (dipotong hanya kalau terpaksa), **C** = could (dipotong lebih dulu).

### 4.1 Perhitungan inti

| ID    | Requirement                                                                                                  | Prio |
| ----- | ------------------------------------------------------------------------------------------------------------ | ---- |
| FR-1  | Menghitung effective depth di +/-2%, +/-5%, +/-10% dari orderbook SDEX                                       | M    |
| FR-2  | Menghitung effective depth dari reserve AMM pool memakai rumus constant product dengan fee                   | M    |
| FR-3  | Menggabungkan SDEX dan AMM berdasarkan harga marginal akhir yang sama, bukan penjumlahan independen          | M    |
| FR-4  | Melaporkan sisi beli dan sisi jual terpisah untuk setiap tingkat delta                                       | M    |
| FR-5  | Menangani seluruh kasus tepi (book kosong, satu sisi, pool kosong, banyak pool, tanpa harga) tanpa exception | M    |
| FR-6  | Menghitung biaya manipulasi: notional yang dibutuhkan untuk menggeser harga sebesar delta                    | M    |
| FR-7  | Menghitung rekomendasi ukuran collateral maksimum aman, dengan parameter yang bisa dikonfigurasi             | M    |
| FR-8  | Menghitung konsentrasi holder: share top-1, share top-10, HHI                                                | S    |
| FR-9  | Menghitung rasio volume-to-supply pada jendela 24 jam, 7 hari, 30 hari                                       | S    |
| FR-10 | Menghitung waktu sejak trade asli terakhir, dengan aturan pengecualian wash trade yang terdokumentasi        | S    |
| FR-11 | Menghitung depth untuk setiap pasangan quote yang punya likuiditas, dan menetapkan pasangan primer           | S    |

### 4.2 Data dan replay

| ID    | Requirement                                                                   | Prio |
| ----- | ----------------------------------------------------------------------------- | ---- |
| FR-12 | Membaca orderbook dan pool terkini dari Horizon mainnet, read-only            | M    |
| FR-13 | Membaca snapshot ledger historis dari Hubble untuk `ledgerSeq` tertentu       | M    |
| FR-14 | Kedua sumber mengembalikan bentuk `Snapshot` yang identik                     | M    |
| FR-15 | Merekam snapshot Horizon secara berkala sebagai ground truth cross-validation | M    |
| FR-16 | Menghasilkan deret waktu metrik untuk satu aset pada rentang ledger           | M    |
| FR-17 | Menjalankan engine untuk minimal 50 aset Stellar aktif dan menyimpan hasilnya | M    |

### 4.3 API publik

| ID    | Requirement                                                                                      | Prio |
| ----- | ------------------------------------------------------------------------------------------------ | ---- |
| FR-18 | `GET /v1/asset/{code}:{issuer}/depth` mengembalikan metrik terkini                               | M    |
| FR-19 | Parameter `?ledger=` mengembalikan metrik pada state ledger historis                             | M    |
| FR-20 | `GET /v1/assets` mengembalikan daftar aset yang dipantau beserta band risikonya                  | M    |
| FR-21 | Setiap respons membawa `ledgerSeq`, `computedAt`, `methodologyVersion`, `dataSource`, `warnings` | M    |
| FR-22 | Dokumentasi API (OpenAPI) yang bisa dicoba tanpa registrasi                                      | M    |
| FR-23 | Parameter `?quote=` untuk memilih aset quote selain pasangan primer                              | C    |

### 4.4 Dashboard

| ID    | Requirement                                                                                                    | Prio |
| ----- | -------------------------------------------------------------------------------------------------------------- | ---- |
| FR-24 | Tabel seluruh aset yang dipantau dengan band risiko dan flag yang terpicu                                      | M    |
| FR-25 | Halaman detail per aset: kurva depth, metrik pendukung, C_max                                                  | M    |
| FR-26 | Halaman studi kasus Blend dengan grafik rasio biaya manipulasi sepanjang Mei 2026 dan penanda tanggal eksploit | M    |
| FR-27 | Tren historis per aset                                                                                         | S    |
| FR-28 | Penjelasan singkat tiap metrik yang bisa dibaca non-teknis                                                     | S    |
| FR-29 | Pencarian dan filter aset                                                                                      | C    |

---

## 5. Definisi band risiko

Bagian ini menyelesaikan yang di SOW hanya disebut "risk scores".

**Keputusan: klasifikasi berbasis aturan, bukan skor komposit berbobot.**

Alasannya: skor 0 sampai 100 hasil pembobotan mengharuskan Anda membenarkan setiap bobot, dan tidak ada dasar empiris untuk membenarkannya dari satu insiden. Klasifikasi berbasis aturan hanya mengharuskan Anda membenarkan ambangnya, dan setiap flag bisa diperiksa terpisah oleh pengguna yang punya kebijakan sendiri.

### 5.1 Flag

Setiap aset dievaluasi terhadap flag berikut. **Flag dilaporkan individual di API, bukan hanya band-nya**, supaya protokol bisa menerapkan kebijakan sendiri.

| Flag                           | Kondisi                                                                                                          |
| ------------------------------ | ---------------------------------------------------------------------------------------------------------------- |
| `NO_EXECUTABLE_PRICE`          | `priceSource = none`. Tidak ada orderbook dan tidak ada pool                                                     |
| `ZERO_DEPTH_2PCT`              | Depth di +/-2% bernilai nol di salah satu sisi                                                                   |
| `MANIPULATION_CHEAP`           | MANIPULATION_CHEAP terpicu bila: ADA delta d dengan Reachable(d) == true DAN Cost(d) < ManipulationCheapAbsolute |
| `MANIPULATION_RATIO_LOW`       | Biaya menaikkan harga 50% kurang dari 1% nilai supply beredar                                                    |
| `NO_GENUINE_TRADE_30D`         | Tidak ada trade asli dalam 30 hari                                                                               |
| `NO_GENUINE_TRADE_7D`          | Tidak ada trade asli dalam 7 hari                                                                                |
| `HOLDER_CONCENTRATION_EXTREME` | Holder tunggal terbesar menguasai lebih dari 50% supply beredar non-issuer                                       |
| `HOLDER_CONCENTRATION_HIGH`    | Sepuluh holder terbesar menguasai lebih dari 80%                                                                 |
| `THIN_DEPTH_5PCT`              | Depth di +/-5% di bawah ambang absolut (default: setara 50.000 XLM)                                              |
| `WASH_TRADE_SUSPECTED`         | Lebih dari 50% volume 30 hari dikecualikan oleh aturan trade asli                                                |

### 5.2 Band

Band adalah flag terburuk yang terpicu. Tidak ada pembobotan, tidak ada rata-rata.

| Band         | Terpicu kalau ada salah satu                                                                  |
| ------------ | --------------------------------------------------------------------------------------------- |
| **CRITICAL** | `NO_EXECUTABLE_PRICE`, `ZERO_DEPTH_2PCT`, `MANIPULATION_CHEAP`                                |
| **HIGH**     | `MANIPULATION_RATIO_LOW`, `NO_GENUINE_TRADE_30D`, `HOLDER_CONCENTRATION_EXTREME`              |
| **MEDIUM**   | `THIN_DEPTH_5PCT`, `NO_GENUINE_TRADE_7D`, `HOLDER_CONCENTRATION_HIGH`, `WASH_TRADE_SUSPECTED` |
| **LOW**      | Tidak ada flag di atas                                                                        |

### 5.3 Kewajiban dokumentasi untuk ambang

Setiap angka ambang di atas **dipilih, bukan dikalibrasi**. Ini harus dinyatakan eksplisit di `docs/methodology/` dan di halaman dashboard. Kalimat yang disarankan:

> Ambang ini dipilih berdasarkan besaran insiden Blend Mei 2026 dan penilaian konservatif, bukan dikalibrasi terhadap kumpulan insiden. Kalibrasi memerlukan lebih banyak kejadian daripada yang tersedia. Seluruh flag dilaporkan terpisah supaya pengguna dapat menerapkan ambang sendiri.

Menyatakan ini justru memperkuat posisi Anda. Reviewer yang berpengalaman akan mencari apakah Anda tahu batas klaim Anda sendiri.

---

## 6. Kontrak data

```typescript
type Asset = { code: string; issuer: string | null }; // null berarti XLM native

type Flag =
  | "NO_EXECUTABLE_PRICE"
  | "ZERO_DEPTH_2PCT"
  | "MANIPULATION_CHEAP"
  | "MANIPULATION_RATIO_LOW"
  | "NO_GENUINE_TRADE_30D"
  | "HOLDER_CONCENTRATION_EXTREME"
  | "THIN_DEPTH_5PCT"
  | "NO_GENUINE_TRADE_7D"
  | "HOLDER_CONCENTRATION_HIGH"
  | "WASH_TRADE_SUSPECTED";

type AssetRisk = {
  asset: Asset;
  quote: Asset; // aset yang dipakai sebagai satuan
  ledgerSeq: number;
  computedAt: string;
  methodologyVersion: string; // contoh "1.2.0"
  dataSource: "horizon" | "hubble";

  midPrice: number | null;
  priceSource: "book" | "pool" | "none";

  depth: Array<{
    delta: number; // 0.02, 0.05, 0.10
    buySide: number; // notional dalam aset quote
    sellSide: number;
    fromSdex: number; // rincian untuk transparansi
    fromAmm: number;
  }>;

  manipulationCost: Array<{ delta: number; cost: number }>;
  maxSafeCollateral: number | null;

  holderTop1Pct: number | null;
  holderTop10Pct: number | null;
  holderHhi: number | null;
  volumeToSupply: { d1: number; d7: number; d30: number } | null;
  lastGenuineTrade: { ledgerSeq: number; at: string } | null;
  tradesExcludedPct: number | null;

  flags: Flag[];
  band: "LOW" | "MEDIUM" | "HIGH" | "CRITICAL";
  warnings: string[];
};
```

Perhatikan `fromSdex` dan `fromAmm` dilaporkan terpisah meskipun angka utamanya gabungan. Ini yang memungkinkan orang lain memeriksa apakah penggabungan Anda benar, dan itu bagian dari janji reproducible.

`methodologyVersion` bukan hiasan. Hasil yang dihitung dengan metodologi v1.0 tidak bisa dibandingkan langsung dengan v1.2. Tanpa field ini, deret waktu Anda diam-diam jadi tidak konsisten saat definisi berubah di tengah sprint.

---

## 7. Requirement non-fungsional

| ID     | Requirement                                  | Target                                                                                                        |
| ------ | -------------------------------------------- | ------------------------------------------------------------------------------------------------------------- |
| NFR-1  | Kesegaran metrik live                        | Maksimal 15 menit tertinggal dari ledger terkini                                                              |
| NFR-2  | Latensi API untuk metrik yang sudah dihitung | Di bawah 500 ms p95                                                                                           |
| NFR-3  | Metrik historis                              | Batch, tanpa jaminan latensi. Dinyatakan di dokumentasi                                                       |
| NFR-4  | Ketersediaan                                 | **Tidak ada SLA.** SOW eksplisit mengecualikan production mainnet SLA. Nyatakan di halaman utama API          |
| NFR-5  | Rate limit publik                            | 60 request per menit per IP                                                                                   |
| NFR-6  | Kepatuhan rate limit Horizon                 | Tidak pernah melebihi 3000 request per jam, di bawah batas publik 3600                                        |
| NFR-7  | Anggaran biaya BigQuery                      | Batas keras, ditinjau mingguan. Semua query memangkas partisi                                                 |
| NFR-8  | Read-only mutlak                             | Tidak ada kode penandatanganan atau pengiriman transaksi di seluruh repo. Ditegakkan lewat hook               |
| NFR-9  | Reproducibility                              | Menjalankan ulang pada `ledgerSeq` yang sama dengan `methodologyVersion` yang sama menghasilkan angka identik |
| NFR-10 | Keterbukaan                                  | Repo publik, data mentah backtest tersedia sebagai CSV tanpa perlu akses BigQuery                             |

NFR-9 adalah requirement yang paling mudah dilanggar tanpa sadar. Segala sesuatu yang bergantung pada jam sistem, urutan iterasi yang tidak deterministik, atau nilai default yang berubah akan merusaknya. Jadikan ini test otomatis, bukan harapan.

---

## 8. Out of scope

Diambil dari SOW dan ditegaskan supaya tidak ada yang tergoda menambahkannya.

- Menandatangani atau mengirim transaksi. Keel permanen read-only
- Menerbitkan price feed sebagai oracle
- Deteksi anomali oracle dan pola serangan real-time. Sudah dilayani OctoPos, Blockaid, Hypernative, Range
- Captive-core ingestion
- Pengiriman alert lewat webhook, Slack, atau PagerDuty
- Profiling risiko issuer aset
- Rantai selain Stellar
- SLA produksi mainnet
- Path finding melintasi aset perantara. Dinyatakan sebagai keterbatasan yang diketahui
- Deteksi akun kustodian dan bursa. Tidak bisa diandalkan, dinyatakan sebagai keterbatasan

---

## 9. Kriteria penerimaan

### Deliverable 1

- [ ] FR-1 sampai FR-7 lolos test terhadap fixture testnet yang hasilnya diketahui
- [ ] FR-12 sampai FR-17 berfungsi, replay historis tervalidasi terhadap ledger kontrol
- [ ] Cross-validation Horizon versus Hubble pada minimal 50 pasangan, hasil tertabulasi
- [ ] Hitung ulang manual untuk 5 aset, spreadsheet ada di repo
- [ ] Dokumen metodologi lengkap termasuk bagian keterbatasan
- [ ] NFR-9 diuji otomatis
- [ ] Kedua builder lulus uji penjelasan lisan untuk penggabungan SDEX-AMM dan pemisahan sisi beli versus jual

### Deliverable 2

- [ ] FR-18 sampai FR-22 live dan terdokumentasi
- [ ] Deret waktu USTRY Mei 2026 lengkap, CSV mentah di repo
- [ ] Laporan menyebutkan kapan ambang tidak aman dilewati relatif terhadap tanggal eksploit
- [ ] Bagian keterbatasan menyebutkan hindsight bias secara eksplisit
- [ ] Pihak ketiga bisa mereproduksi angka utama hanya dari repo dan laporan

### Deliverable 3

- [ ] FR-24 sampai FR-26 live
- [ ] Minimal 50 aset di demonstration set
- [ ] Ambassador Chapter Lead bisa menjelaskan apa yang ditampilkan dashboard tanpa bantuan
- [ ] Video demo 3 sampai 5 menit
- [ ] Laporan backtest terbit terbuka

---

## 10. Metrik keberhasilan

### Selama 30 hari

1. Seluruh kriteria penerimaan di bagian 9 terpenuhi
2. Backtest menunjukkan sinyal yang jelas dan berarah benar sebelum tanggal eksploit
3. Minimal satu pihak ketiga di luar tim berhasil mereproduksi satu angka dari repo

Perhatikan metrik 2. Kalau backtest **tidak** menunjukkan sinyal yang jelas, itu bukan kegagalan proyek melainkan temuan yang harus dilaporkan jujur. Melaporkannya apa adanya jauh lebih baik untuk kredibilitas jangka panjang daripada menyetel ambang sampai hasilnya terlihat bagus. Kalau ambang disetel setelah melihat hasil, katakan itu di laporan.

### Setelah 30 hari (validasi untuk SCF Build)

4. Minimal 3 percakapan dengan protokol atau kurator vault tentang apakah metrik ini akan mereka pakai
5. Minimal 1 pernyataan tertulis dari pihak ekosistem bahwa metrik ini berguna
6. Kejelasan tentang kesediaan membayar, termasuk kalau jawabannya tidak

Metrik 6 sudah diakui di lampiran SOW: ketiadaan Gauntlet dan Chaos Labs di Stellar mungkin menandakan pasarnya belum cukup besar. Jawaban "tidak ada yang mau membayar" adalah hasil yang valid dan berharga dari sprint $5.000, dan jauh lebih murah daripada menemukannya setelah SCF Build.

---

## 11. Pertanyaan terbuka

Harus terjawab sebelum akhir minggu yang disebut.

| #   | Pertanyaan                                                                                   | Batas waktu |
| --- | -------------------------------------------------------------------------------------------- | ----------- |
| Q1  | Seberapa rapat snapshot `offers` di Hubble untuk Mei 2026?                                   | Day 1       |
| Q2  | Berapa biaya BigQuery untuk satu kali replay penuh USTRY sebulan?                            | Day 2       |
| Q3  | Parameter risiko Blend yang sebenarnya berlaku Mei 2026, untuk dipakai sebagai default C_max | Minggu 1    |
| Q4  | Apakah supply yang terkunci di liquidity pool masuk denominator konsentrasi holder?          | Minggu 1    |
| Q5  | Aturan final untuk trade asli, khususnya apakah deteksi akun terkait masuk v1 atau v2        | Minggu 2    |
| Q6  | Kriteria seleksi 50 aset demonstration set                                                   | Minggu 2    |
| Q7  | Apakah ambang absolut di flag dinyatakan dalam XLM atau USDC?                                | Minggu 2    |

Q7 lebih penting daripada kelihatannya. Kalau ambang dinyatakan dalam XLM, band risiko sebuah aset bisa berubah hanya karena harga XLM bergerak, tanpa likuiditas aset itu berubah sama sekali. Kalau dinyatakan dalam USDC, Anda memasukkan asumsi bahwa USDC stabil. Apa pun pilihannya, dokumentasikan konsekuensinya.

---

## 12. Urutan pemotongan scope

Sudah ditetapkan di depan supaya keputusan di bawah tekanan tidak diambil sembarangan.

1. Potong requirement prioritas **C** terlebih dahulu (FR-23, FR-29)
2. Potong FR-27 dan FR-28, sederhanakan halaman detail
3. Turunkan jumlah aset di dashboard, sisanya tetap tersedia lewat API
4. Jarangkan interval replay historis
5. Turunkan FR-8 sampai FR-11 ke versi paling sederhana

**Tidak pernah dipotong:** backtest Blend, dokumen metodologi, cross-validation, dan bagian keterbatasan. Empat hal itu adalah seluruh nilai proyek ini. Dashboard yang cantik tanpa backtest yang bisa diverifikasi tidak akan lolos ke SCF Build.
