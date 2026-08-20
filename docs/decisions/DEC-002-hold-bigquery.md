# Keel: Fase Horizon-Only (menunda BigQuery)

**Keputusan:** BigQuery ditunda sampai terbukti benar-benar dibutuhkan.
**Menggantikan:** urutan Day 0 di Checklist Kesiapan yang menempatkan spike Hubble sebagai jalur kritis.

---

## 1. Apa yang sebenarnya diblokir tanpa BigQuery

Hanya satu hal: **state orderbook pada ledger masa lalu.**

Yang **tidak** diblokir, dan ini mayoritas pekerjaan:

| Kebutuhan | Sumber Horizon | Historis? |
|---|---|---|
| Orderbook terkini | `/order_book` | Tidak perlu |
| Reserve pool terkini | `/liquidity_pools` | Tidak perlu |
| Seluruh riwayat trade | `/trades` dengan filter pasangan aset | **Ya, penuh** |
| Deret harga dan volume | `/trade_aggregations` | **Ya, penuh** |
| Riwayat operasi akun | `/accounts/{id}/operations` | **Ya, penuh** |
| Riwayat operasi pool | `/liquidity_pools/{id}/operations` | **Ya, penuh** |
| Daftar holder dan saldo | `/accounts?asset=` | Terkini saja |
| Supply aset | `/assets` | Terkini saja |

Artinya seluruh Deliverable 1 kecuali replay, seluruh Deliverable 3, dan **klaim utama Deliverable 2** bisa dikerjakan tanpa menyentuh BigQuery.

---

## 2. Tiga pengganti untuk state historis, dari termurah

### 2.1 Biaya manipulasi langsung dari trade yang benar-benar terjadi

Klaim paling kuat di laporan backtest Anda bukan "depth USTRY di 2% adalah sekian pada 20 Februari". Klaim paling kuatnya adalah:

> Trade sebesar X memindahkan harga USTRY dari sekitar $1,05 ke sekitar $107. Biaya manipulasi terukur adalah X. Nilai yang dipinjam terhadap harga hasil manipulasi itu adalah $10,97 juta.

Angka X **terbaca langsung** dari `/trades`. Tidak butuh orderbook, tidak butuh rekonstruksi, tidak butuh BigQuery. Ini fakta on-chain yang bisa diverifikasi siapa pun dengan satu perintah curl.

Kalau angka yang beredar benar, rasionya sekitar 1 berbanding puluhan juta. Itu satu kalimat yang menjual seluruh premis Keel, dan Anda bisa memilikinya minggu ini.

### 2.2 Batas atas depth yang tersirat dari trade (metodologi baru, layak didokumentasikan)

Ini pengganti yang jujur secara matematis untuk depth historis.

**Klaimnya:** kalau sebuah trade bernilai `S` menggeser harga marginal sebesar `δ`, maka depth pada `δ` **tidak mungkin lebih besar dari `S`**.

```
depth(δ) ≤ S,  untuk δ = |P_sesudah / P_sebelum − 1|
```

Alasannya sederhana: kalau ada lebih banyak likuiditas dalam rentang harga itu, trade sebesar `S` tidak akan sanggup menembusnya.

Ini menghasilkan **batas atas**, bukan nilai persis. Tapi untuk tujuan Keel, batas atas justru cukup dan bahkan lebih kuat secara retoris. Anda tidak perlu membuktikan depth USTRY tepatnya $41. Anda perlu membuktikan depth-nya **di bawah ambang aman**, dan batas atas melakukan itu.

Sifatnya konservatif ke arah yang benar: ia tidak pernah membuat aset terlihat lebih berbahaya dari kenyataan, hanya berpotensi kurang berbahaya. Itu bias yang bisa dipertahankan di depan reviewer.

Dokumentasikan sebagai `docs/methodology/11-depth-tersirat-dari-trade.md`, lengkap dengan pernyataan bahwa ini batas atas dan bukan pengukuran langsung.

### 2.3 Rekonstruksi offer penuh dari operasi akun

Baru dikerjakan kalau 2.1 dan 2.2 terbukti tidak cukup.

Prosedur:
1. Ambil seluruh trade pasangan USTRY/USDC dari `/trades`. Kumpulkan himpunan akun yang pernah terlibat
2. Untuk tiap akun, ambil `/accounts/{id}/operations` pada rentang Februari 2026
3. Saring operasi `manage_sell_offer`, `manage_buy_offer`, `create_passive_sell_offer`
4. Bangun state machine offer, terapkan berurutan sampai ledger target

**Celah yang wajib didokumentasikan:** akun yang memasang offer lalu membatalkannya tanpa pernah ter-trade tidak muncul di `/trades`, sehingga tidak masuk himpunan akun. Untuk pasar dengan volume di bawah $1 per jam, jumlah akun kecil dan celahnya kemungkinan kecil, tapi keberadaannya harus dinyatakan, bukan disembunyikan.

Reserve pool historis lebih bersih: `/liquidity_pools/{id}/operations` memberi riwayat deposit, withdraw, dan trade per pool secara langsung, tanpa celah semacam itu.

---

## 3. Spike Day 0 yang baru

Spike lama: "apakah snapshot Hubble cukup rapat untuk Februari 2026?" Butuh akun Google, butuh kuota, butuh belajar BigQuery.

Spike baru: **"seberapa tipis riwayat trade USTRY/USDC?"** Gratis, tanpa akun, selesai dalam 30 menit.

```bash
# 1. Temukan issuer USTRY dari balance akun burner penyerang
curl -s "https://horizon.stellar.org/accounts/GCNF5GNRIT6VWYZ7LXUZ33Q3SR2NUGO32F5X65VVKAEWWIQCKGYN75HB" \
  | jq '.balances'

# 2. Ambil seluruh riwayat trade pasangan USTRY/USDC
curl -s "https://horizon.stellar.org/trades\
?base_asset_type=credit_alphanum4&base_asset_code=USTRY&base_asset_issuer=<ISSUER_USTRY>\
&counter_asset_type=credit_alphanum4&counter_asset_code=USDC&counter_asset_issuer=<ISSUER_USDC>\
&order=asc&limit=200" \
  | jq '._embedded.records[] | {ledger_close_time, base_amount, counter_amount, price, base_account, counter_account}'

# 3. Ledger sequence transaksi offer manipulasi
curl -s "https://horizon.stellar.org/transactions/09e1a9d1197c9bf0af4e87da328c4f2d5eb49b487630aa61991fb5c1c4637cdb" \
  | jq '{ledger, created_at, successful}'
```

**Pertanyaan yang dijawab spike ini:**

- Berapa total trade sepanjang riwayat pasar ini? Kalau di bawah beberapa ribu, seluruh backtest bisa dikerjakan dari Horizon saja
- Berapa akun unik yang pernah terlibat? Kalau di bawah seratus, rekonstruksi offer penuh (2.3) juga tractable
- Berapa persis ukuran trade manipulasinya? Ini angka utama laporan Anda

**Definition of done:** satu tabel berisi jumlah trade, jumlah akun unik, dan ukuran trade manipulasi. Kabari Kenny hasilnya di hari yang sama.

---

## 4. Urutan pengerjaan yang direvisi

| Fase | Isi | BigQuery? |
|---|---|---|
| **Fase 1** (Minggu 1 sampai 2) | Reader Horizon, engine depth, metrik pendukung, flags, C_max, scan 50 aset, recorder, API, dashboard terhadap mock | Tidak |
| **Fase 2** (Minggu 3) | Backtest dari data trade: biaya manipulasi terukur, depth tersirat, kronologi berbasis ledger | Tidak |
| **Fase 3** (kalau perlu) | Depth historis presisi lewat rekonstruksi offer atau Hubble | Diputuskan di Minggu 3 |

Fase 1 dan 2 mencakup seluruh Deliverable 1 kecuali replay presisi, seluruh Deliverable 3, dan klaim utama Deliverable 2. Kalau sprint mepet, Fase 3 adalah yang dipotong, dan pemotongannya tidak merusak apa pun yang esensial.

---

## 5. Perubahan pada dokumen lain

| Dokumen | Perubahan |
|---|---|
| Checklist Kesiapan, blok A | Item Google Cloud dan BigQuery **dihapus dari jalur kritis**. Tidak ada lagi ketergantungan pada pihak ketiga di Day 0 |
| Checklist, blok B | Spike Hubble diganti spike riwayat trade di bagian 3 dokumen ini |
| TDD bagian 3.2 | Adapter Hubble tetap didefinisikan sebagai antarmuka, implementasinya ditunda. Tanda **[MENUNGGU SPIKE]** tetap |
| TDD bagian 4 | Tambah `internal/domain/implied_depth.go` untuk metodologi 2.2 |
| PRD | Tambah catatan bahwa metrik historis pada v1 dapat berupa batas atas, bukan pengukuran langsung, dan itu ditandai di respons API |
| OpenAPI | Tambah nilai `dataSource: "trades-implied"` selain `horizon` dan `hubble`, supaya konsumen tahu sifat angkanya |
| Rencana Eksekusi D1, D1.5 | Historical replay memakai jalur trade lebih dulu, Hubble jadi peningkatan opsional |

Penambahan `dataSource: "trades-implied"` penting. Angka yang berupa batas atas tidak boleh terlihat sama dengan angka hasil pengukuran langsung. Kejujuran itu terlihat di API, bukan hanya di dokumen.

---

## 6. Kapan meninjau ulang keputusan ini

Aktifkan Fase 3 kalau salah satu terjadi:

1. Spike menunjukkan riwayat trade USTRY/USDC terlalu besar untuk ditarik lewat Horizon dalam waktu wajar
2. Reviewer atau Ambassador secara eksplisit meminta depth historis presisi, bukan batas atas
3. Fase 1 dan 2 selesai lebih cepat dari jadwal dan ada sisa waktu di Minggu 3

Kalau tidak ada yang terjadi, selesaikan sprint tanpa BigQuery sama sekali. Itu hasil yang sah dan justru lebih mudah direproduksi orang lain, karena pihak ketiga bisa memverifikasi seluruh angka Anda hanya dengan curl.
