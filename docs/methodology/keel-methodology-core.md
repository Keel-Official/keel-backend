# Keel: Metodologi Inti

**Versi metodologi:** 1.0.2-draft
**Berlaku untuk:** `internal/depth` seluruhnya, dengan tipe dari `internal/domain`
**Status:** definisi terkunci, ambang belum dikalibrasi
**Divalidasi terhadap:** insiden pool YieldBlox DAO di Blend V2, 22 Februari 2026

Dokumen ini mendefinisikan apa yang Keel hitung dan mengapa. Setiap definisi di sini
harus dapat dipertahankan tanpa merujuk pada kode. Kalau kode dan dokumen ini
bertentangan, dokumen ini yang benar dan kodenya bug.

**Pembagian sumber kebenaran.** Berkas ini mendefinisikan besaran yang dihitung.
Definisi flag, band, dan seluruh ambang ada di `09-flag-dan-band.md`, dan berkas itu
yang berlaku kalau keduanya berbeda. Dua tempat untuk satu definisi menjamin keduanya
menyimpang, jadi pembagiannya harus dijaga tetap tajam.

**Konvensi satuan persen.** Setiap besaran dan ambang berakhiran `Pct` dinyatakan
dalam PERSEN, bukan pecahan. `spreadPct` bernilai 196,0777141 berarti 196 persen, dan
dibandingkan langsung dengan `SpreadExtremePct` yang bernilai 20,0. Sebaliknya `δ`
selalu pecahan: `δ = 0,02` berarti 2 persen. Kedua konvensi ini berbeda dengan sengaja,
sebab `δ` adalah masukan rumus sedangkan `Pct` adalah besaran yang dilaporkan.

---

## 1. Pertanyaan yang dijawab

Oracle menjawab: berapa harga aset ini.
Keel menjawab: **berapa besar transaksi yang sanggup ditanggung harga itu, dan berapa
biaya untuk memindahkannya.**

Dua pertanyaan turunan yang harus dibedakan tegas, karena keduanya sering tertukar:

| Risiko                | Sisi buku       | Pertanyaan                                                                   |
| --------------------- | --------------- | ---------------------------------------------------------------------------- |
| **Likuidasi**         | bid (sisi jual) | Kalau collateral ini harus dilikuidasi, sanggupkah pasar menyerapnya?        |
| **Manipulasi oracle** | ask (sisi beli) | Berapa biaya menaikkan harga sampai collateral terlihat jauh lebih bernilai? |

Sebuah aset bisa aman di satu sisi dan berbahaya di sisi lain. Keel melaporkan keduanya
terpisah dan tidak pernah menggabungkannya menjadi satu angka.

---

## 2. Notasi dan satuan

| Simbol     | Arti                                            |
| ---------- | ----------------------------------------------- |
| `base`     | aset yang dinilai                               |
| `quote`    | aset satuan pengukuran (pasangan primer)        |
| `P0`       | harga acuan, quote per base                     |
| `δ`        | pergeseran harga relatif, 0,02 berarti 2 persen |
| `P_target` | harga sasaran, `P0 × (1 + δ)`                   |
| `X`, `Y`   | reserve base dan quote pada satu pool AMM       |
| `f`        | fee pool, 0,003 pada Stellar                    |

Aturan satuan:

1. Seluruh nilai depth dan biaya dinyatakan sebagai **notional dalam aset quote**, bukan
   jumlah unit token. Pertanyaan bisnisnya adalah "berapa dolar", bukan "berapa keping".
2. Keel **tidak** mengonversi ke USD memakai feed harga eksternal. Seluruh premis produk
   ini adalah mempertanyakan apakah harga yang dilaporkan bisa dipercaya. Memakai feed
   untuk menghitung membuat argumennya melingkar.
3. Amount Stellar adalah int64 dalam stroop, 7 desimal. Harga dibaca dari pecahan
   rasional `price_r` berupa `{n, d}`, bukan dari string `price` yang sudah dibulatkan.
   Seluruh aritmetika desimal, tidak pernah floating point.

---

## 3. Harga acuan `P0`

Urutan fallback, berhenti pada yang pertama terpenuhi:

| #   | Kondisi                               | `P0`                                            | `priceSource` |
| --- | ------------------------------------- | ----------------------------------------------- | ------------- |
| 1   | Ada bid dan ada ask                   | `(best_bid + best_ask) / 2`                     | `book`        |
| 2   | Hanya satu sisi buku terisi, ada pool | `Y / X` dari pool dengan reserve quote terbesar | `pool`        |
| 3   | Buku kosong, ada pool                 | `Y / X`                                         | `pool`        |
| 4   | Tidak ada buku dan tidak ada pool     | tidak terdefinisi                               | `none`        |

**Kasus 4 bukan error dan tidak boleh melempar exception.** Aset tanpa harga eksekutabel
adalah temuan bernilai tertinggi yang bisa Keel hasilkan. Ia dilaporkan sebagai hasil
yang sah dengan flag `NO_EXECUTABLE_PRICE` dan band `CRITICAL`.

**Peringatan yang ditemukan dari data nyata.** `P0` dari mid orderbook dapat dipengaruhi
oleh order milik penyerang sendiri. Pada insiden 22 Februari 2026, penyerang memasang
order beli 0,0001 USTRY pada harga 1,057 pukul 23:39:31, satu menit setelah memasang
offer manipulasinya. Order sekecil itu ikut membentuk `P0` tanpa mewakili likuiditas
nyata apa pun. Karena itu `P0` **tidak boleh** dipakai sendirian sebagai indikator
kesehatan, dan harus selalu dibaca bersama depth.

---

## 3a. Spread dan batas kebermaknaan `P0`

Ditemukan saat menyusun golden fixture, dan menjadi alasan versi 1.0.1 ada.

```
spreadPct = (best_ask − best_bid) / P0 × 100
```

`spreadPct` bernilai null ketika salah satu sisi buku kosong, sebab selisihnya tidak
terdefinisi. Null berarti tidak diketahui, bukan nol.

Pada 21 Februari 2026 buku USTRY/USDC berisi ask 106,7372828 dan bid 1,057, sehingga
`P0` menjadi 53,8971414 untuk aset yang sebenarnya bernilai sekitar 1,06. Angka itu
bukan bug, melainkan sifat mid price ketika spread mencapai ratusan persen.

Konsekuensinya keras: **`P0` dan seluruh metrik yang diturunkan darinya kehilangan
makna pada spread ekstrem.** Tangga depth ±2/5/10 persen termasuk di dalamnya. Ia tetap
dilaporkan karena dijanjikan SOW, tetapi pada buku serusak itu ia bukan metrik keamanan
oracle. Yang menyelamatkan analisis adalah tangga delta besar di bagian 7.3 dan flag
`SPREAD_EXTREME`.

Flag lain memang tetap menyala pada kasus USTRY, tetapi itu kebetulan dan bukan desain.
Karena itu `spreadPct` dilaporkan sebagai angka di respons API, bukan hanya sebagai
status terpicu atau tidak.

---

## 4. Depth orderbook SDEX

Depth sisi beli pada `δ` adalah total notional yang dapat diserap dengan membeli dari
ask sebelum harga marginal melewati `P_target`.

```
P_target = P0 × (1 + δ)
depth_sdex_beli(δ) = Σ (price_i × amount_i)  untuk semua ask dengan price_i ≤ P_target
```

Sisi jual simetris memakai bid dan `P0 × (1 − δ)`.

**Keputusan: level yang melewati batas dibuang seluruhnya, tidak diambil sebagian.**
Ini menghasilkan angka yang sedikit lebih rendah dari nilai teoretis. Pilihan ini
disengaja dan konsisten dengan Prinsip Konservatif di bagian 12: pada setiap kasus
ambigu, Keel memilih interpretasi yang membuat aset tampak lebih berisiko, tidak
kurang.

---

## 5. Depth AMM constant product

Pool Stellar memakai `X × Y = k` dengan fee 30 bps. Harga spot `P = Y / X`.

Penurunan. Membeli `Δx` base menyisakan reserve `X − Δx`, sehingga harga marginal baru:

```
P' = k / (X − Δx)²
P' / P = X² / (X − Δx)²
```

Agar `P' / P = 1 + δ`:

```
X − Δx = X / √(1 + δ)
```

Sehingga:

```
depth_amm_beli(δ)  = Y × (√(1 + δ) − 1)      quote yang harus dibayar
depth_amm_jual(δ)  = Y × (1 − √(1 − δ))      quote yang diterima
input_kotor        = input_bersih / (1 − f)
```

Uji kewajaran yang wajib ada di test: `depth_amm ≈ (δ / 2) × Y`. Penyimpangan besar dari
ini berarti ada bug.

| δ   | naik, persen dari Y | turun, persen dari Y |
| --- | ------------------- | -------------------- |
| 2%  | 0,995%              | 1,005%               |
| 5%  | 2,47%               | 2,53%                |
| 10% | 4,88%               | 5,13%                |

Akar kuadrat dihitung dengan presisi desimal yang ditetapkan, bukan `float64`. Nilai
presisi dan toleransinya adalah konstanta bernama dan merupakan bagian dari metodologi,
bukan detail implementasi.

---

## 6. Penggabungan SDEX dan AMM

**Yang salah:** `depth_total = depth_sdex + depth_amm` dihitung terpisah. Keduanya
bersaing di rentang harga yang sama, sehingga penjumlahan independen melebih-lebihkan
likuiditas.

**Yang benar:** keduanya dibatasi oleh harga marginal akhir yang sama.

```
depth_gabungan(δ):
    P_target = P0 × (1 + δ)
    n_sdex   = Σ (price × amount) untuk ask dengan price ≤ P_target
    n_amm    = Σ atas seluruh pool:
                   0                              jika P_pool ≥ P_target
                   Y × (√(P_target / P_pool) − 1) jika P_pool < P_target
    return n_sdex + n_amm
```

Kontribusi SDEX dan AMM tetap dilaporkan terpisah di keluaran (`fromSdex`, `fromAmm`)
agar pihak ketiga dapat memverifikasi penggabungan ini tanpa membaca kode.

Uji pembeda yang wajib ada: buat fixture dengan harga pool 5 persen di atas `P0`, lalu
minta depth pada 2 persen. Jawaban benar adalah `fromAmm` tepat nol. Implementasi yang
menjumlahkan terpisah akan mengembalikan angka bukan nol.

---

## 7. Biaya manipulasi

Bagian ini adalah kontribusi utama Keel dan definisinya diperbaiki setelah memeriksa
data ledger insiden 22 Februari 2026.

### 7.1 Definisi

Biaya manipulasi untuk mencapai harga `P_target` adalah notional yang harus dibayarkan
**kepada pihak lain** untuk menaikkan harga marginal sampai ke sana:

```
MC(P_target) = Σ (price_i × amount_i)
               untuk seluruh ask dengan price_i ≤ P_target
               yang TIDAK dimiliki penyerang
```

### 7.2 Kenapa klausa kepemilikan itu menentukan

Ini kesalahan yang hampir masuk ke metodologi ini dan hanya terbongkar oleh data.

Intuisi yang keliru adalah menganggap ukuran transaksi manipulasi sebagai biayanya. Pada
insiden 22 Februari, transaksi manipulasi bernilai 5,3475699 USDC. Angka itu bukan
biaya, karena offer yang dieksekusi adalah milik penyerang sendiri. Uang itu berpindah
dari satu akun penyerang ke akun penyerang lainnya.

Biaya sebenarnya adalah nilai order **pihak ketiga** yang harus dilalap dalam perjalanan
dari `P0` menuju `P_target`. Mesin pencocokan Stellar selalu mengisi dari harga terbaik.
Agar sebuah order beli sampai menyentuh offer di 106,74, seluruh ask yang lebih murah
harus habis lebih dulu.

Dan jumlah itu **persis sama dengan depth sisi beli antara `P0` dan `P_target`.**

```
MC(P_target) = depth_beli sampai P_target, dikurangi order milik penyerang
```

Keel tidak dapat mengetahui kepemilikan order sebelum kejadian. Karena itu Keel
melaporkan versi tanpa penyaringan, yang merupakan **batas atas** biaya manipulasi.
Arah biasnya aman: Keel akan menyatakan manipulasi lebih mahal daripada kenyataan,
tidak pernah lebih murah.

### 7.3 Konsekuensi: tangga delta besar

Depth pada ±2, ±5, dan ±10 persen mengukur kualitas pasar. Ia **tidak cukup** untuk
mengukur ketahanan terhadap manipulasi oracle, karena penyerang tidak perlu menggeser
harga 10 persen. Pada insiden ini penyerang menggesernya 100,98 kali lipat.

Keel karena itu menghitung dua tangga:

| Tangga               | Nilai δ                  | Tujuan                                            |
| -------------------- | ------------------------ | ------------------------------------------------- |
| Kualitas pasar       | 2%, 5%, 10%              | wajib menurut SOW, menggambarkan kedalaman normal |
| Ketahanan manipulasi | 50%, 100%, 1000%, 10000% | menggambarkan biaya serangan                      |

Metrik turunan yang paling berguna adalah fungsi inversnya:

```
P_reachable(C) = harga tertinggi yang dapat dicapai dengan modal C
```

Untuk USTRY pada 21 Februari 2026, `P_reachable(0)` sudah melampaui 100 kali `P0`.

### 7.4 `Reachable`, dan dua arti berbeda dari biaya nol

Ini perbaikan versi 1.0.1 dan bagian paling mudah disalahpahami di seluruh metodologi.
Test pertama yang ditulis untuk fixture USTRY memang menyalahpahaminya di dua baris.

Setiap baris tangga manipulasi membawa dua besaran yang dihitung dari himpunan ask
yang **berbeda**:

```
Cost(P_target)      = Σ (price_i × amount_i)  untuk ask dengan price_i <  P_target
Reachable(P_target) = ada ask dengan price_i >= P_target
```

Sebuah ask tidak pernah masuk keduanya sekaligus. Penyerang harus melalap seluruh ask
yang lebih murah dari sasaran, lalu menyentuh sedikit saja ask pertama yang berada di
atas sasaran. Sentuhan terakhir itu yang menetapkan harga yang dibaca oracle, dan
biayanya dapat sekecil apa pun.

Karena itu biaya nol berarti dua hal yang berlawanan:

| Keadaan | Arti |
|---|---|
| `Cost = 0`, `Reachable = true` | harga sasaran dapat dicapai tanpa membayar apa pun kepada pihak ketiga. **Ini kondisi paling berbahaya yang bisa ada** |
| `Cost = 0`, `Reachable = false` | tidak ada likuiditas sama sekali dalam rentang itu, sehingga harga tidak dapat digeser bertahap ke sana. Justru bukan kabar buruk |

Membaca `Cost` sendirian tanpa `Reachable` menghasilkan angka yang menyesatkan tanpa
ada yang gagal. Pada fixture USTRY, `Cost` bernilai 130,0627093 untuk δ = 1, 10, dan
100, dan ketiganya `Reachable = false`. Angka 130,06 di situ **tidak** berarti "harga
itu mahal dicapai"; harga itu tidak dapat dicapai sama sekali sebab buku habis jauh
sebelum sampai ke sana.

### 7.5 Harga tercapai maksimum

Tangga delta diskret dapat melewatkan serangan yang jatuh di antara dua anak tangga.
Karena itu Keel juga melaporkan sepasang angka yang tidak bergantung pada tangga:

```
MaxReachablePrice       = harga ask tertinggi di buku
CostToMaxReachablePrice = Σ notional ask dengan price < MaxReachablePrice
```

Pada USTRY 21 Februari 2026 nilainya 106,7372828 dengan biaya **nol**. Serangan nyata
jatuh di celah antara δ = 0,5 dan δ = 1, sehingga terlewat oleh seluruh tangga dan
hanya tertangkap oleh pasangan angka ini.

---

## 8. Ketahanan oracle terhadap jendela VWAP

Oracle yang dimanipulasi pada insiden ini berbasis VWAP, bukan harga sesaat. Menggeser
harga marginal saja tidak cukup, transaksi penyerang harus juga mendominasi rata-rata
tertimbang volume dalam jendela oracle.

```
MR(P_target, W) = MC(P_target) + V_genuine(W)
```

dengan `V_genuine(W)` adalah volume trade asli dalam jendela `W`.

Suku kedua adalah pertahanan yang tak terlihat. Pasar dengan perdagangan aktif memaksa
penyerang mengalahkan volume nyata untuk menggerakkan rata-rata. Pasar tanpa
perdagangan tidak punya pertahanan itu sama sekali.

Pada 22 Februari 2026, kedua suku bernilai nol atau mendekati nol secara bersamaan.
Itulah yang membuat serangan ini praktis gratis.

`W` adalah parameter dan bukan konstanta universal. Nilai default Keel mengasumsikan 15
menit, mengikuti pernyataan Script3 bahwa tidak ada trade lain dalam 15 menit sebelum
manipulasi. Angka ini **belum terkonfirmasi** sebagai panjang jendela Reflector yang
sebenarnya dan ditandai sebagai asumsi.

---

## 9. Ukuran collateral maksimum aman

```
C_max = min( D_jual(δ_likuidasi) × h , MC(P_kritis) × m )
```

| Simbol                | Arti                                  | Default           |
| --------------------- | ------------------------------------- | ----------------- |
| `D_jual(δ_likuidasi)` | depth sisi jual pada diskon likuidasi | δ = 10%           |
| `h`                   | haircut likuidasi                     | 0,5               |
| `MC(P_kritis)`        | biaya manipulasi ke harga kritis      | P_kritis = 2 × P0 |
| `m`                   | margin keamanan manipulasi            | 0,25              |

Kedua suku menjawab pertanyaan berbeda dan keduanya harus dilaporkan, bukan hanya
minimumnya. Seluruh parameter dapat dikonfigurasi pemanggil, karena Keel bersifat
agnostik terhadap protokol.

Nilai default di atas **dipilih, bukan dikalibrasi.** Kalibrasi memerlukan lebih banyak
insiden daripada yang tersedia. Pernyataan ini wajib muncul di dashboard dan di respons
API, bukan hanya di dokumen ini.

---

## 10. Validasi empiris: insiden 22 Februari 2026

Seluruh angka di bagian ini diturunkan dari Horizon mainnet dan dapat direproduksi
siapa pun tanpa akun, tanpa BigQuery, dan tanpa akses istimewa.

### 10.1 Aset

```
USTRY : GCRYUGD5NVARGXT56XEZI5CIFCQETYHAPQQTHO2O3IQZTHDH4LATMYWC   (credit_alphanum12)
USDC  : GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN   (credit_alphanum4)
```

Perhatikan USTRY bertipe `credit_alphanum12` karena kodenya lima karakter. Query dengan
tipe `credit_alphanum4` mengembalikan hasil kosong tanpa pesan error.

### 10.2 Kronologi terverifikasi

| Waktu UTC       | Peristiwa                                                                                                            | Bukti                                         |
| --------------- | -------------------------------------------------------------------------------------------------------------------- | --------------------------------------------- |
| 21 Feb 23:36:28 | Akun burner menukar 1 XLM menjadi 0,1612003 USDC untuk pendanaan                                                     | op 263452928864530433                         |
| 21 Feb 23:38:51 | **Offer manipulasi dipasang.** Jual 1,2185312 USTRY pada `price_r` {266843207, 2500000} = 106,7372828 USDC per USTRY | tx `09e1a9d1...`, offer 1824788980            |
| 21 Feb 23:39:31 | Order beli 0,0001 USTRY pada 1,0570000 dipasang oleh akun yang sama                                                  | op 263453066303434753                         |
| 22 Feb 00:10:21 | **Trade manipulasi.** `GDHRCQNC...` membayar 5,3475699 USDC untuk 0,0501003 USTRY, mencocokkan offer 1824788980      | trade 263454423513071617-0, ledger 61340263   |
| 22 Feb 00:10:57 | Trade debu 0,0000080 USTRY pada harga normal 1,057, antar akun penyerang                                             | trade 263454449283014657-0                    |
| 22 Feb 00:11:16 | Order beli 1,057 dibatalkan                                                                                          | op 263454462168100865, ledger 61340272        |
| 22 Feb ~00:25   | Pinjaman diambil di pool YieldBlox: 1.000.196,70 USDC lalu 61.249.278,31 XLM                                         | sumber sekunder, verifikasi on-chain tertunda |
| 16 Mar 14:26:40 | Akun burner menukar 5,4152411 USDC menjadi 31,1670395 XLM                                                            | op 264910138253852673                         |

### 10.3 Konsistensi aritmetika

Seluruh angka saling mengunci, yang menandakan pembacaan ini benar:

```
0,0501003 USTRY × 106,7372828  = 5,3475699 USDC     cocok dengan yang dibayarkan
1,2185312 − 0,0501003          = 1,1684309 USTRY    cocok dengan sisa offer hari ini
106,7372828 / 1,057            = 100,98×            cocok dengan laporan "100x"
```

### 10.4 Temuan utama

**Biaya manipulasi mendekati nol.** Seluruh 5,3475699 USDC dibayarkan kepada offer milik
penyerang sendiri, sehingga kembali ke penyerang. Tidak ada trade lain tercatat pada
ledger 61340263 antara `P0` dan `P_target`. Karena mesin pencocokan Stellar mengisi dari
harga terbaik, tersentuhnya offer di 106,74 berarti **tidak ada ask pihak ketiga di
seluruh rentang harga dari 1,057 sampai 106,74.**

Kalimat itu sebelumnya ditandai sebagai inferensi kuat. Sejak versi 1.0.1 ia menjadi
**pengamatan langsung**: daftar trade akun
`GDHRCQNC64UVL27EXSC6OG6I2FCT4NWM72KNHLHKEB3LK4MEEYYWETN3` pada 2026-02-22T00:10:21Z
memuat tepat satu record, yaitu 5,3475699 USDC melawan offer 1824788980 milik
`GCNF5GNRIT6VWYZ7LXUZ33Q3SR2NUGO32F5X65VVKAEWWIQCKGYN75HB`. Karena seluruh pencocokan
menghasilkan record trade, ketiadaan record lain membuktikan tidak ada ask pihak ketiga
yang dilalap.

```
MC(100 × P0) untuk USTRY/USDC pada 21 Februari 2026 = 0 USDC
```

Bukan tipis. Nol.

**Skala akibatnya.** Dengan XLM pada 0,1612003 USDC, pinjaman yang diambil bernilai:

```
61.249.278,31 XLM × 0,1612003 =  9.873.402 USDC
                    + USDC     =  1.000.197 USDC
                      total    = 10.873.599 USDC
```

Collateral 149.876,10 USTRY bernilai 158.419 USDC pada harga sebenarnya, dan 15.997.368
USDC pada harga hasil manipulasi. Rasio 100,98 kali persis seperti yang diharapkan.

**Apa yang akan dilaporkan Keel.** Dijalankan pada USTRY/USDC pada 21 Februari 2026,
Keel akan memicu, minimal:

| Flag                     | Alasan                                                |
| ------------------------ | ----------------------------------------------------- |
| `MANIPULATION_CHEAP`     | `MC(50%)` di bawah ambang absolut mana pun yang wajar |
| `MANIPULATION_RATIO_LOW` | biaya manipulasi mendekati nol terhadap nilai supply  |
| `THIN_DEPTH_5PCT`        | tidak ada ask sama sekali dalam rentang itu           |
| `NO_GENUINE_TRADE_7D`    | volume di bawah 1 USDC per jam menjelang insiden      |

Band: `CRITICAL`. Aset ini tidak akan lolos ambang apa pun yang bisa dipertahankan.

### 10.5 Verifikasi yang masih tertunda

1. Kedua transaksi pinjaman di pool YieldBlox, saat ini masih dari sumber sekunder.
2. Parameter risiko pool YieldBlox yang berlaku saat itu.
3. Panjang jendela VWAP Reflector yang sebenarnya.

Butir tentang daftar trade akun `GDHRCQNC...` **sudah selesai** pada versi 1.0.1 dan
dipindah ke bagian 10.4 sebagai pengamatan langsung.

---

## 11. Depth tersirat dari trade

Ketika state orderbook historis tidak tersedia, depth dapat dibatasi dari trade yang
benar-benar terjadi.

**Klaim.** Jika sebuah trade bernilai `S` menggeser harga marginal sebesar `δ`, maka:

```
depth(δ) ≤ S
```

Alasannya: jika likuiditas dalam rentang harga itu lebih besar dari `S`, trade sebesar
`S` tidak akan sanggup menembusnya.

Ini menghasilkan batas atas, bukan pengukuran langsung. Untuk tujuan Keel itu memadai,
karena yang perlu dibuktikan bukan nilai persis depth melainkan bahwa depth berada di
bawah ambang aman. Biasnya konservatif ke arah yang benar.

Hasil yang diturunkan dengan cara ini **wajib** ditandai `dataSource: "trades-implied"`
pada respons API. Angka berupa batas atas tidak boleh terlihat identik dengan angka
hasil pengukuran langsung.

---

## 12. Prinsip dan keterbatasan

### Prinsip konservatif

Pada setiap kasus ambigu, pilih interpretasi yang menghasilkan depth lebih rendah dan
penilaian risiko lebih tinggi. Produk peringatan yang terlalu optimistis tidak berguna.

### Keterbatasan yang diketahui

1. **Likuiditas tercatat bukan likuiditas eksekutabel.** Offer dapat ditarik seketika.
   Reflector melaporkan bahwa market maker pasar ini menarik seluruh likuiditasnya
   sebelum insiden. Keel yang memindai setiap 15 menit akan menangkap perubahan itu,
   Keel yang memindai harian mungkin terlambat. Frekuensi pemindaian adalah parameter
   yang jujur, bukan detail teknis.
2. **Path payment lintas aset perantara tidak dihitung.** Likuiditas efektif sebenarnya
   dapat lebih besar dari yang dilaporkan Keel.
3. **Likuiditas di bursa terpusat tidak terlihat.** Keel hanya mengukur on-chain.
4. **Ambang dipilih, bukan dikalibrasi.** Kalibrasi memerlukan banyak insiden. Seluruh
   flag dilaporkan terpisah agar konsumen dapat menerapkan ambang sendiri.
5. **Backtest mengetahui hasilnya di depan.** Risiko hindsight bias nyata. Kalau ambang
   disetel setelah melihat hasil, hal itu harus dinyatakan di laporan.
6. **Kepemilikan order tidak dapat diketahui sebelum kejadian**, sehingga biaya
   manipulasi yang dilaporkan selalu batas atas.

---

## 13. Versi

`MethodologyVersion` mengikuti semver dan wajib naik setiap kali definisi atau ambang
berubah. Hasil dari versi berbeda tidak dapat dibandingkan langsung dan disimpan sebagai
baris terpisah di basis data.

| Versi       | Perubahan                                                                                                                                                                                            |
| ----------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1.0.0-draft | Definisi awal. Biaya manipulasi diperbaiki menjadi berbasis kepemilikan setelah pemeriksaan ledger insiden 22 Februari 2026. Tangga delta besar ditambahkan. Suku volume jendela oracle ditambahkan. |
| 1.0.1-draft | Bagian 3a (`spreadPct` dan `SPREAD_EXTREME`) ditambahkan setelah golden fixture menunjukkan `P0` kehilangan makna pada spread ekstrem. Bagian 7.4 (`Reachable`, dua arti biaya nol) dan 7.5 (`MaxReachablePrice`) ditambahkan. Bagian 10.4 naik dari inferensi menjadi pengamatan langsung, dan butir 1 pada 10.5 ditutup. |
| 1.0.2-draft | `MANIPULATION_CHEAP` dan `MANIPULATION_RATIO_LOW` disyaratkan `Reachable == true`. Keadaan `unevaluated` dan `bandConfidence` ditambahkan setelah fixture menunjukkan enam flag tidak dapat dinilai dari snapshot saja. Detailnya di `09-flag-dan-band.md`. Konvensi satuan persen dinyatakan eksplisit di kepala dokumen ini. |
