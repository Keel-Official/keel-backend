# DEC-001: Identitas Aset USTRY dan Rentang Ledger Insiden

**Status:** SEBAGIAN TERKONFIRMASI. Dua item masih terbuka, prosedur penyelesaian ada di bagian 5.
**Tanggal:** Agustus 2026
**Dampak:** Deliverable 2 seluruhnya, rentang query Hubble, dan dua koreksi pada SOW

---

## 1. Koreksi terhadap SOW (penting, perlu dikomunikasikan ke Ambassador)

### Koreksi 1: Tanggal insiden

| | SOW | Yang benar |
|---|---|---|
| Tanggal | 20 Mei 2026 | **22 Februari 2026, 00:25 UTC** |

Seluruh detail lain di SOW cocok dengan insiden Februari: pembekuan sekitar 48 juta XLM, kerugian sekitar $10 juta, manipulasi harga 100x, pinjaman 61 juta XLM. Ini bukan dua insiden berbeda. Tanggalnya tercatat keliru.

**Dampak:** seluruh rentang query historis berubah dari Mei ke Februari 2026. Kabar baiknya, data Februari lebih lama mengendap sehingga lebih mungkin lengkap di Hubble.

### Koreksi 2: Satuan jumlah pinjaman

| | SOW | Yang benar |
|---|---|---|
| Pinjaman | "$61 million in XLM" | **61.249.278,31 XLM**, ditambah 1.000.196,70 USDC |

61 juta XLM bukan $61 juta. Total nilai kedua pinjaman sekitar $10 sampai $11 juta. Ini perbedaan sekitar enam kali lipat dan reviewer SCF akan menangkapnya.

### Koreksi 3: Yang dieksploitasi bukan Blend inti

Yang tereksploitasi adalah **pool YieldBlox DAO di Blend V2**, sebuah pool yang dikelola komunitas dengan parameter yang bisa diatur pengelolanya sendiri. BlockSec menyimpulkan ini kegagalan konfigurasi pool operator, bukan kerentanan kontrak inti Blend. Menyebutnya "Blend exploit" tanpa kualifikasi akan dianggap ceroboh oleh pembaca yang paham.

Frasa yang lebih akurat: "insiden pool YieldBlox DAO di Blend V2".

---

## 2. Fakta yang sudah terkonfirmasi

| Item | Nilai | Keyakinan |
|---|---|---|
| Tanggal dan waktu | 22 Februari 2026, 00:25 UTC (dua transaksi pinjaman) | Tinggi, banyak sumber sepakat |
| Aset yang dimanipulasi | USTRY, stablebond US Treasury terbitan **Etherfuse** | Tinggi |
| Pasar yang dimanipulasi | **USTRY/USDC di SDEX** | Tinggi |
| Oracle | **Reflector**, berbasis VWAP, membaca harga dari SDEX | Tinggi |
| Protokol | Pool YieldBlox DAO di Blend V2 (Script3) | Tinggi |
| Harga sebelum | sekitar $1,05 sampai $1,058 | Tinggi |
| Harga setelah | sekitar $106 sampai $107 | Tinggi |
| Volume pra-insiden | kurang dari $1 per jam | Tinggi |
| Pinjaman 1 | 1.000.196,70 USDC | Tinggi |
| Pinjaman 2 | 61.249.278,31 XLM | Tinggi |
| Collateral | dilaporkan 13.003 USTRY lalu tambahan 140.000 USTRY. BlockSec menyebut total sekitar 149.876 USTRY | **Sedang, ada selisih antar sumber. Verifikasi on-chain** |
| Dana dibekukan | sekitar 48 juta XLM, senilai sekitar $7,2 juta | Tinggi |

### Kronologi yang dilaporkan Rekt

| Waktu (UTC) | Kejadian |
|---|---|
| 14 Feb 2026 | Akun utama penyerang dibuat dengan 56,32 XLM |
| 14 sampai 20 Feb | Pembelian uji USTRY dalam jumlah kecil pada harga normal sekitar $1,058 |
| 21 Feb 23:35 | Akun burner dibuat dengan 15 XLM: `GCNF5GNRIT6VWYZ7LXUZ33Q3SR2NUGO32F5X65VVKAEWWIQCKGYN75HB` |
| 21 Feb 23:38 | Sell offer 1,2185 USTRY pada harga 107 USDC. Hash transaksi: `09e1a9d1197c9bf0af4e87da328c4f2d5eb49b487630aa61991fb5c1c4637cdb` |
| 22 Feb sekitar 00:10 | Akun ketiga mengeksekusi trade agar oracle membaca harga itu |
| 22 Feb 00:25 | Dua transaksi pinjaman: 1.000.196 USDC lalu 61.249.278 XLM |

**Angka yang paling penting untuk Keel:** offer manipulasi itu hanya 1,2185 USTRY. Satu sumber menyebut trade yang mengeksekusinya bernilai sekitar $0,50. Kalau angka itu terverifikasi on-chain, rasio biaya manipulasi terhadap nilai yang dicuri adalah sekitar 1 berbanding 22 juta. Itu satu angka yang menjual seluruh premis Keel.

---

## 3. Yang masih belum terkonfirmasi

1. **Alamat issuer USTRY (`G...`).** Tidak ditemukan di sumber sekunder mana pun. Harus diambil dari ledger.
2. **Nomor ledger sequence** untuk setiap titik kronologi di atas.
3. **Jumlah collateral yang tepat.** Sumber berbeda menyebut 153.003 dan 149.876 USTRY.
4. **Panjang jendela VWAP Reflector.** Script3 menyebut tidak ada trade lain dalam 15 menit. Perlu konfirmasi apakah 15 menit itu memang panjang jendela oracle atau kebetulan.
5. **Parameter risiko pool YieldBlox** saat itu: collateral factor USTRY, liability factor XLM dan USDC.

---

## 4. Konsekuensi untuk desain Keel

**Pasangan primer USTRY adalah USDC, bukan XLM.** Ini menyelesaikan sebagian keputusan terbuka T3 dan D-1. Oracle membaca pasar USTRY/USDC, jadi backtest wajib mengukur pasar itu. Mengukur USTRY/XLM akan menjawab pertanyaan yang salah.

**Biaya manipulasi harus dihitung relatif terhadap jendela oracle, bukan hanya terhadap pergeseran harga.** Ini penghalusan penting pada metrik K5. Oracle Reflector berbasis VWAP, jadi yang perlu digeser penyerang bukan harga sesaat melainkan rata-rata tertimbang volume dalam jendela tertentu. Di pasar tanpa trade lain, satu trade mendominasi rata-rata itu sepenuhnya, dan itulah yang membuat serangan ini murah. Catat di `docs/methodology/07-metrik-pendukung.md` sebagai keterbatasan atau sebagai perluasan metrik.

**Metrik "waktu sejak trade asli terakhir" terbukti relevan.** Volume kurang dari $1 per jam dan tidak ada trade dalam jendela oracle adalah tepat kondisi yang seharusnya memicu flag `NO_GENUINE_TRADE_7D` dan `THIN_DEPTH_5PCT`. Keel akan menandai aset ini merah jauh sebelum 22 Februari.

**Ada faktor yang tidak akan tertangkap Keel, dan ini wajib masuk bagian keterbatasan.** Reflector menyatakan market maker pasar itu menarik seluruh likuiditasnya di suatu titik sebelum eksploit. Artinya kondisi berbahaya muncul relatif mendadak. Keel yang memindai tiap 15 menit akan menangkapnya, tapi Keel yang memindai harian mungkin terlambat. Frekuensi pemindaian jadi parameter yang harus dibahas jujur di laporan, bukan disembunyikan.

---

## 5. Prosedur menyelesaikan dua item terbuka

Semua di bawah memakai Horizon publik, gratis, tanpa akun.

### 5.1 Menemukan issuer USTRY

```bash
curl -s "https://horizon.stellar.org/assets?asset_code=USTRY&limit=20" | jq '._embedded.records[] | {code:.asset_code, issuer:.asset_issuer, amount, num_accounts:.num_accounts}'
```

Kalau muncul lebih dari satu issuer, jangan menebak. Disambiguasi dengan mencocokkan terhadap akun burner yang sudah diketahui:

```bash
curl -s "https://horizon.stellar.org/accounts/GCNF5GNRIT6VWYZ7LXUZ33Q3SR2NUGO32F5X65VVKAEWWIQCKGYN75HB" | jq '.balances'
```

Balance akun itu akan memuat `asset_code: "USTRY"` beserta `asset_issuer` yang sebenarnya dipakai dalam serangan. Itu jawaban definitifnya, karena berasal dari ledger, bukan dari artikel.

### 5.2 Menemukan ledger sequence

Transaksi offer manipulasi sudah diketahui hash-nya:

```bash
curl -s "https://horizon.stellar.org/transactions/09e1a9d1197c9bf0af4e87da328c4f2d5eb49b487630aa61991fb5c1c4637cdb" | jq '{ledger, created_at, source_account, successful}'
```

Field `ledger` adalah jangkar utama Anda. Dari sana ambil seluruh riwayat akun burner:

```bash
curl -s "https://horizon.stellar.org/accounts/GCNF5GNRIT6VWYZ7LXUZ33Q3SR2NUGO32F5X65VVKAEWWIQCKGYN75HB/operations?limit=200&order=asc" | jq '._embedded.records[] | {id, type, created_at, transaction_hash}'
```

Setiap operasi membawa waktu dan bisa ditelusuri ke ledger-nya. Dari akun ini Anda akan menemukan akun lawan trade, dan dari sana akun peminjam.

Untuk rentang backtest, sasaran yang wajar: **ledger pada 1 Februari 2026 00:00 UTC sampai 28 Februari 2026 23:59 UTC.** Ledger tutup sekitar tiap 5 detik, jadi sekitar 17.280 ledger per hari dan sekitar 480 ribu ledger untuk sebulan. Jangan menghitung dari perkiraan, ambil batasnya dari data:

```bash
curl -s "https://horizon.stellar.org/trades?base_asset_type=credit_alphanum4&base_asset_code=USTRY&base_asset_issuer=<ISSUER>&counter_asset_type=credit_alphanum4&counter_asset_code=USDC&counter_asset_issuer=GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN&order=asc&limit=200" | jq '._embedded.records[] | {ledger_close_time, base_amount, counter_amount, price}'
```

Endpoint ini historis dan gratis. Ia langsung memberi Anda seluruh riwayat trade pasar itu, termasuk trade manipulasinya. Untuk aset setipis USTRY, jumlah trade-nya kemungkinan kecil, dan Anda bisa melihat seluruh riwayat pasar dalam beberapa halaman.

Verifikasi issuer USDC di atas sebelum dipakai.

### 5.3 Sumber primer yang harus dibaca langsung

Sumber yang saya pakai sejauh ini semuanya sekunder. Untuk laporan yang bisa dipertahankan, baca langsung:

- Pernyataan Script3 (@script3official) tanggal 22 dan 23 Februari 2026
- Pernyataan Reflector tentang penyebab mispricing
- Analisis BlockSec: https://blocksec.com/blog/yieldblox-dao-incident-on-stellar-oracle-misconfiguration-enabled-a-10m-drain
- Analisis QuillAudits: https://www.quillaudits.com/blog/hack-analysis/yeildblox-10m-hack-explained
- Rekonstruksi forensik Rekt: https://rekt.news/yieldblox-rekt
- Konfigurasi pool YieldBlox di Blend V2, untuk parameter risiko

Aturan untuk laporan Deliverable 2: setiap angka yang Anda klaim harus punya salah satu dari dua sumber, yaitu data on-chain yang bisa direproduksi, atau pernyataan resmi pihak yang terlibat. Angka dari artikel berita hanya dipakai sebagai petunjuk untuk dicari di ledger, tidak dikutip sebagai fakta.

---

## 6. Tindakan berikutnya

1. Jalankan perintah di 5.1 dan 5.2, isi dua item terbuka, perbarui dokumen ini
2. Kabari Ambassador Chapter Lead soal koreksi tanggal dan satuan. Ini bukan kabar buruk, ini bukti Anda memverifikasi
3. Ubah seluruh rujukan "Mei 2026" menjadi "Februari 2026" di PRD, rencana pembangunan, dan checklist
4. Setel rentang spike Hubble ke Februari 2026
5. Tetapkan USDC sebagai pasangan quote primer USTRY untuk keperluan backtest
