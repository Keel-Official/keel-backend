# Keel: Technical Design Document

**Versi:** 0.2 (draf)
**Tanggal:** Agustus 2026, dianotasi 20 Agustus 2026
**Penulis:** draf disusun bersama Claude, keputusan dimiliki tim
**Pembaca:** kedua builder, reviewer SCF Build

> **PERINGATAN KESEGARAN.** Dokumen ini disusun ketika Keel direncanakan dalam
> TypeScript. Implementasi berjalan dalam **Go**. Bagian yang menyebut nama
> berkas `.ts`, `decimal.js`, `big.js`, Fastify, Hono, ESLint, atau
> dependency-cruiser adalah peninggalan dan **tidak berlaku**. Bentuk
> arsitekturnya tetap berlaku; hanya perkakasnya yang berubah.
>
> | Bagian | Tertulis | Yang berlaku sekarang |
> |---|---|---|
> | 2.1 | dependency-cruiser atau ESLint `import/no-restricted-paths` | `internal/domain/arch_test.go`, dijalankan lewat `make arch` di CI |
> | 3.3 | `decimal.js` atau `big.js` | `github.com/shopspring/decimal`, keputusan T4 tertutup |
> | 4 | modul `.ts` di `domain/` | paket Go: tipe di `internal/domain`, rumus di `internal/depth`, uji kesesuaian di `internal/conformance` |
> | 5 | skema `assets`, `metrics`, `runs` | belum ada di `migrations/`; yang ada baru `0001_snapshots.sql`. Skema di sini juga belum memuat kolom v1.0.2 (`unevaluated_flags`, `band_confidence`, `spread_pct`, `max_reachable_price`) |
> | 7 | Fastify atau Hono | belum diputuskan untuk Go, lihat keputusan terbuka |
> | 9 butir 6 | test determinisme JSON | ada, `TestInvarianDeterminisme` di `internal/conformance` |
> | 11 dan 13 T1/T2 | rencana cadangan dan anggaran BigQuery | ditunda seluruhnya, lihat `docs/decisions/DEC-002-hold-bigquery.md` |

Dokumen ini menjelaskan **bagaimana** Keel dibangun. Apa yang dibangun ada di PRD. Apa yang dijanjikan ada di SOW.

Bagian yang ditandai **[MENUNGGU SPIKE]** belum bisa difinalkan sebelum hasil spike Hubble di Day 0.

---

## 1. Sasaran dan bukan sasaran

### Sasaran teknis
1. Perhitungan yang **deterministik dan bisa direproduksi** pada `ledgerSeq` yang sama
2. Satu implementasi perhitungan yang dipakai jalur live maupun historis
3. Jalur historis bisa ditukar tanpa menyentuh kode perhitungan
4. Read-only mutlak, ditegakkan secara mekanis bukan lewat konvensi

### Bukan sasaran
- High availability. Tidak ada SLA (NFR-4)
- Latensi rendah untuk perhitungan historis. Batch, dan dinyatakan begitu
- Skala di luar beberapa ratus aset
- Multi-tenancy, autentikasi, manajemen pengguna

---

## 2. Arsitektur

```
                   ┌──────────────────┐
   Horizon ───────▶│                  │
   (state terkini) │                  │
                   │    ADAPTERS      │──▶ Snapshot ──┐
   Hubble ────────▶│                  │              │
   (state historis)└──────────────────┘              │
                                                      ▼
                                           ┌────────────────────┐
                                           │      DOMAIN        │
                                           │  depth, metrics,   │
                                           │  collateral, flags │
                                           │  (fungsi murni)    │
                                           └────────┬───────────┘
                                                    │ AssetRisk
                                                    ▼
   ┌──────────────┐      ┌───────────────┐   ┌──────────────┐
   │ ORCHESTRATOR │─────▶│    STORE      │◀──│  API (baca)  │
   │ scan, replay │      │  PostgreSQL   │   └──────┬───────┘
   └──────────────┘      └───────────────┘          │
                                                    ▼
                                             ┌──────────────┐
                                             │  DASHBOARD   │
                                             └──────────────┘
```

### 2.1 Aturan dependensi (wajib ditegakkan lint)

```
domain/          tidak boleh mengimpor apa pun dari adapters/, store/, api/
adapters/        boleh mengimpor tipe dari domain/, tidak boleh logika
orchestrator/    boleh mengimpor semuanya
api/             hanya membaca store/, tidak pernah memanggil adapters langsung
```

Aturan pertama adalah yang menentukan. `domain/` tidak boleh tahu dari mana snapshot datang. Itulah yang membuat historical replay hanya berarti menukar adapter.

Tegakkan dengan `dependency-cruiser` atau aturan `import/no-restricted-paths` di ESLint, dan jalankan di CI. Aturan yang hanya ditulis di dokumen akan dilanggar dalam dua minggu.

### 2.2 Kenapa API tidak memanggil adapter langsung

Kalau endpoint API memanggil Horizon saat request masuk, satu aset populer bisa menghabiskan kuota rate limit Horizon Anda dalam hitungan menit, dan pengguna mendapat latensi yang tidak terduga. API hanya membaca hasil yang sudah dihitung orchestrator. Konsekuensinya metrik selalu sedikit tertinggal, dan itu diterima secara eksplisit di NFR-1 (maksimal 15 menit).

---

## 3. Sumber data

### 3.1 Horizon (jalur live)

| Kebutuhan | Endpoint | Catatan |
|---|---|---|
| Orderbook | `/order_book` | **State terkini saja. Tidak menerima parameter ledger** |
| Reserve pool | `/liquidity_pools` | State terkini saja |
| Deret harga | `/trade_aggregations` | Historis, native, gratis |
| Trade | `/trades` | Historis, untuk deteksi trade asli |
| Supply aset | `/assets` | State terkini |

`Latest-Ledger` di header respons dipakai sebagai `ledgerSeq` snapshot. Jangan memakai jam sistem.

### 3.2 Hubble (jalur historis) [MENUNGGU SPIKE]

Dataset `crypto-stellar.crypto_stellar` di BigQuery. Tabel yang dibutuhkan: `offers`, `liquidity_pools`, `trust_lines`, `history_trades`.

Semua query wajib:
- memangkas partisi dengan `WHERE batch_run_date BETWEEN ...`
- tidak pernah `SELECT *`
- menyetel `maximum_bytes_billed` agar query yang salah gagal, bukan menagih

Kalau spike menunjukkan snapshot `offers` terlalu jarang untuk Mei 2026, aktifkan rencana cadangan di bagian 11.

### 3.3 Presisi angka (penting, mudah salah)

Ini penyebab paling umum cross-validation gagal tanpa sebab yang jelas.

- Amount Stellar adalah **int64 dalam stroop**, 7 desimal. Simpan sebagai `bigint` atau string, jangan `number`
- Horizon mengembalikan harga sebagai **pecahan rasional** `price_r: { n, d }`. Pakai itu. String `price` adalah hasil pembulatan dan tidak boleh dipakai untuk perhitungan
- Semua aritmetika depth memakai `decimal.js` atau `big.js`, bukan float IEEE 754
- Konversi ke `number` hanya di lapisan penyajian, tepat sebelum dikirim ke API

Kalau ini dilanggar, jalur Horizon dan jalur Hubble akan menghasilkan angka yang berbeda di digit ke sekian, cross-validation akan penuh ketidakcocokan palsu, dan Anda akan mengejar bug yang tidak ada.

---

## 4. Modul domain

> **PENINGGALAN TYPESCRIPT.** Daftar berkas `.ts` di bawah tidak berlaku.
> Pembagian yang sebenarnya:
>
> | Paket Go | Isi | Zona |
> |---|---|---|
> | `internal/domain` | tipe bersama saja, tanpa perhitungan | kuning |
> | `internal/depth` | seluruh rumus: harga acuan, depth SDEX dan AMM, penggabungan, biaya manipulasi, flag, band, C_max | merah |
> | `internal/conformance` | golden fixture dan uji kesesuaian, black-box terhadap `internal/depth` | hijau |
>
> Aturan kemurnian pada paragraf terakhir bagian ini tetap berlaku penuh, dan
> sekarang ditegakkan secara mekanis oleh `internal/domain/arch_test.go`.


```
domain/
  types.ts        Asset, Level, PoolReserves, Snapshot, AssetRisk, Flag
  price.ts        midPrice() dan urutan fallback (keputusan D-2)
  depthSdex.ts    walk the book
  depthAmm.ts     rumus constant product dengan fee
  depthCombine.ts penggabungan berdasarkan harga marginal (keputusan D-3)
  manipulation.ts biaya manipulasi
  collateral.ts   C_max
  concentration.ts konsentrasi holder
  activity.ts     trade asli dan volume-to-supply
  flags.ts        evaluasi flag dan penentuan band
  version.ts      METHODOLOGY_VERSION
```

Setiap file di `domain/` harus lolos tiga syarat: tanpa I/O, tanpa `Date.now()`, tanpa `Math.random()`. Kalau butuh waktu, terima sebagai argumen.

`flags.ts` memuat seluruh ambang sebagai konstanta bernama di satu tempat, bukan tersebar. Ambang adalah bagian metodologi, dan mengubahnya harus mengubah `METHODOLOGY_VERSION`.

---

## 5. Skema penyimpanan

PostgreSQL. Satu instance managed (Neon atau Supabase tier gratis) sudah cukup.

```sql
create table assets (
  id             serial primary key,
  code           text not null,
  issuer         text,                      -- null untuk XLM native
  quote_code     text not null,             -- pasangan primer
  quote_issuer   text,
  active         boolean not null default true,
  selection_note text,                      -- kenapa aset ini masuk demonstration set
  added_at       timestamptz not null default now(),
  unique (code, issuer, quote_code, quote_issuer)
);

create table metrics (
  id                  bigserial primary key,
  asset_id            int not null references assets(id),
  ledger_seq          bigint not null,
  ledger_closed_at    timestamptz not null,
  computed_at         timestamptz not null,
  methodology_version text not null,
  data_source         text not null,        -- 'horizon' | 'hubble'
  mid_price           numeric,
  price_source        text not null,        -- 'book' | 'pool' | 'none'
  depth               jsonb not null,       -- [{delta, buySide, sellSide, fromSdex, fromAmm}]
  manipulation_cost   jsonb not null,
  max_safe_collateral numeric,
  holder_top1_pct     numeric,
  holder_top10_pct    numeric,
  holder_hhi          numeric,
  volume_to_supply    jsonb,
  last_genuine_trade  jsonb,
  trades_excluded_pct numeric,
  flags               text[] not null default '{}',
  band                text not null,
  warnings            text[] not null default '{}',
  unique (asset_id, ledger_seq, methodology_version, data_source)
);

create index on metrics (asset_id, ledger_seq desc);
create index on metrics (band) where band in ('HIGH','CRITICAL');

create table runs (
  id            bigserial primary key,
  kind          text not null,              -- 'scan' | 'replay'
  started_at    timestamptz not null,
  finished_at   timestamptz,
  assets_ok     int not null default 0,
  assets_failed int not null default 0,
  notes         text
);
```

Constraint unik pada `metrics` memuat `methodology_version` dan `data_source`. Ini disengaja: hasil dari metodologi berbeda atau sumber berbeda adalah baris berbeda, bukan saling menimpa. Itulah yang membuat cross-validation bisa dilakukan dengan satu query SQL, dan yang membuat perubahan metodologi di tengah sprint tidak diam-diam merusak deret waktu.

**Snapshot mentah tidak disimpan di database.** Untuk 50 aset setiap 15 menit selama 30 hari, itu puluhan gigabyte tanpa manfaat. Yang disimpan hanya rekaman untuk cross-validation: 8 aset terpilih, sebagai file JSON gzip di `recordings/`, dan 60 di antaranya ikut git sebagai bukti.

---

## 6. Orchestrator

### 6.1 Job scan (live)

Berjalan setiap 15 menit.

```
untuk setiap aset aktif:
  snapshot = horizonAdapter.getSnapshot(asset, quote)
  metrik   = computeAssetRisk(snapshot, params)
  simpan ke metrics
  catat kegagalan per aset, jangan hentikan seluruh job
```

Satu aset gagal tidak boleh menggagalkan scan. Kegagalan dicatat di `runs.assets_failed` dan aset itu memakai hasil sebelumnya, ditandai basi.

### 6.2 Job recorder (cross-validation)

Berjalan setiap 30 menit untuk 8 aset terpilih. Menyimpan snapshot Horizon mentah ke disk. **Mulai Day 2**, bukan Minggu 3, karena kebenaran pembanding tidak bisa dibuat surut.

### 6.3 Job replay (historis)

Dijalankan manual dengan rentang ledger. Memakai `hubbleAdapter`. Menulis ke tabel yang sama dengan `data_source = 'hubble'`.

### 6.4 Anggaran rate limit Horizon

Horizon publik membatasi sekitar 3600 request per jam per IP. Target kita di bawah 3000.

```
Per aset per scan:  1 orderbook + 1 daftar pool + 1 trade_aggregations  = 3
50 aset                                                                 = 150
Scan tiap 15 menit                                                      = 600/jam
Recorder 8 aset tiap 30 menit                                           = 32/jam
Cadangan retry                                                          = ~100/jam
                                                            Total       ≈ 750/jam
```

Ruang lebihnya besar, dan itu disengaja. Data holder diambil dari Hubble, bukan Horizon, tepat untuk menjaga anggaran ini. Kalau jumlah aset naik ke 200, hitung ulang sebelum menaikkannya.

---

## 7. Lapisan API

- Framework: Fastify atau Hono. Ringan, cukup
- Hanya membaca dari PostgreSQL. Tidak pernah memanggil adapter
- Cache respons 60 detik
- Rate limit 60 request per menit per IP
- Tidak ada autentikasi. Tidak ada data pengguna
- CORS terbuka. Ini API publik read-only
- Setiap respons membawa `ledgerSeq`, `computedAt`, `methodologyVersion`, `dataSource`, `warnings`

Header respons menyertakan `X-Keel-Staleness-Seconds` supaya konsumen tahu seberapa lama data tertinggal tanpa harus menghitung sendiri.

---

## 8. Mode terdegradasi

Bagian ini yang membedakan sistem yang bisa dipercaya dari yang tidak. Definisikan sekarang, bukan saat terjadi.

| Kondisi | Perilaku |
|---|---|
| Horizon lambat atau down | Job scan gagal sebagian, API tetap menyajikan hasil terakhir dengan `X-Keel-Staleness-Seconds` tinggi dan warning eksplisit |
| Hubble melewati anggaran biaya | Endpoint historis mengembalikan 503 dengan pesan yang jelas. Jalur live tidak terpengaruh |
| Aset tidak punya harga sama sekali | **Bukan error.** `priceSource: 'none'`, flag `NO_EXECUTABLE_PRICE`, band `CRITICAL` |
| Aset baru tanpa riwayat metrik | 404 dengan pesan bahwa aset tidak dipantau, bukan 500 |
| Query historis untuk ledger yang belum ada di Hubble | 404 dengan penjelasan ketersediaan data, bukan 500 |

Baris ketiga adalah yang paling penting dan paling mudah diimplementasi salah. Frontend harus menerima ini sebagai temuan bernilai tinggi, bukan layar error.

---

## 9. Reproducibility

NFR-9 menyatakan menjalankan ulang pada `ledgerSeq` dan `methodologyVersion` yang sama harus menghasilkan angka identik. Mekanismenya:

1. Tidak ada `Date.now()`, `Math.random()`, atau urutan iterasi tidak deterministik di `domain/`
2. Pool diurutkan berdasarkan `poolId` sebelum diproses
3. Level orderbook diurutkan berdasarkan harga, tie-break berdasarkan urutan asli
4. Seluruh aritmetika desimal, bukan float
5. `METHODOLOGY_VERSION` naik setiap kali ambang atau rumus berubah
6. **Test otomatis:** jalankan `computeAssetRisk` dua kali pada snapshot fixture yang sama, bandingkan hasil berupa string JSON. Harus identik byte per byte

Butir 6 murah dan menangkap pelanggaran butir 1 sampai 4 secara otomatis.

---

## 10. Deployment

| Komponen | Platform | Catatan |
|---|---|---|
| API + orchestrator | Satu container di Railway, Fly.io, atau Render | Satu proses, job dijadwalkan dengan cron internal |
| Database | PostgreSQL managed (Neon atau Supabase) | Tier gratis cukup untuk skala ini |
| Dashboard | Vercel atau Netlify | Statik, memanggil API |
| Recorder | Proses yang sama dengan orchestrator | Menulis ke volume persisten |

Tidak ada Kubernetes. Tidak ada Terraform. Tidak ada CI/CD berlapis. Deliverable dinilai dari bukti yang bisa diverifikasi, bukan dari kecanggihan infrastruktur.

Environment variable: URL Horizon, kredensial BigQuery, URL database, batas anggaran query. Semuanya lewat secret platform, tidak pernah di repo.

---

## 11. Rencana cadangan kalau spike Day 0 gagal [MENUNGGU SPIKE]

Kalau snapshot `offers` di Hubble terlalu jarang untuk Mei 2026:

**Rekonstruksi dari event.** Ambil snapshot state terdekat sebelum ledger target, lalu terapkan berurutan seluruh operasi yang mengubah orderbook (`manage_buy_offer`, `manage_sell_offer`, `create_passive_sell_offer`, `path_payment_*`) dan trade dari `history_operations` serta `history_trades` sampai ledger target.

Ini state machine yang lurus tapi memakan 3 sampai 5 hari kerja, dan harus dibayar dengan pemotongan scope Deliverable 3 sesuai urutan di PRD bagian 12.

Adapter tetap satu antarmuka yang sama. Yang berubah hanya isi `hubbleAdapter.getSnapshot()`. Inilah alasan aturan dependensi di bagian 2.1 tidak boleh dikompromikan.

---

## 12. Alternatif yang ditolak

Bagian ini ada supaya keputusan tidak diperdebatkan ulang di Minggu 3.

| Alternatif | Kenapa ditolak |
|---|---|
| Menjalankan captive core sendiri | Dikecualikan eksplisit di SOW. Waktu setup dan biaya infrastruktur tidak sepadan untuk 30 hari |
| Menghitung metrik saat request masuk | Menghabiskan kuota Horizon, latensi tidak terduga, dan tidak reproducible |
| Menyimpan seluruh snapshot mentah di database | Puluhan gigabyte tanpa manfaat. Cukup rekaman terbatas untuk cross-validation |
| Menjumlahkan depth SDEX dan AMM secara terpisah | Melebih-lebihkan likuiditas. Keduanya bersaing di rentang harga yang sama |
| Memakai feed harga eksternal untuk konversi USD | Melanggar prinsip P-1 di PRD. Argumen Keel jadi melingkar |
| Float untuk amount dan harga | Merusak determinisme dan cross-validation |
| Kubernetes untuk deployment | Waktu yang dipakai tidak menghasilkan bukti deliverable apa pun |
| Skor risiko komposit berbobot | Bobotnya tidak bisa dibenarkan dari satu insiden. Diganti klasifikasi berbasis aturan |

---

## 13. Keputusan yang masih terbuka

| # | Keputusan | Butuh apa | Batas waktu |
|---|---|---|---|
| T1 | Kerapatan snapshot Hubble, dan apakah rencana cadangan bagian 11 diaktifkan | Hasil spike Day 0 | Day 1 |
| T2 | Nilai `maximum_bytes_billed` dan anggaran BigQuery bulanan | Estimasi biaya dari spike | Day 2 |
| T3 | Pasangan quote primer per aset: XLM atau USDC | Keputusan D-1 di rencana Deliverable 1 | Minggu 1 |
| ~~T4~~ | ~~Library desimal~~ **TERTUTUP**: `github.com/shopspring/decimal`, terpasang di `go.mod` | | selesai |
| T5 | Interval scan: 15 menit atau lebih jarang | Anggaran rate limit setelah pengukuran nyata | Minggu 2 |
| T6 | Platform hosting final | Preferensi tim dan batas tier gratis | Minggu 2 |
