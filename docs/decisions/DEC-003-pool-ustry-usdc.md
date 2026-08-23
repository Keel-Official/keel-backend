# DEC-003: Pool USTRY/USDC pada Ledger 61340263

**Status:** TERVERIFIKASI dari data on-chain
**Tanggal:** Agustus 2026
**Dampak:** fixture golden, aturan harga acuan, definisi biaya manipulasi, bagian keterbatasan

---

## 1. Hasil verifikasi

```
Pool ID   : 27480d0483c8320ba4a707797526ffd67118e841491e0cbeb66db697bb66cccb
Fee       : 30 bps
Dibuat    : aktif setidaknya sejak 2026-07-01 (operasi terawal yang terindeks)

Reserve pada ledger 61340263 (2026-02-22T00:10:21Z):
  USDC  : 16,3389179
  USTRY : 15,4791416
  spot  : 1,0555442 USDC per USTRY
  k     : 252,9124238
```

### Provenance

Diperoleh dari `/liquidity_pools/{id}/effects`, mundur dari kondisi terkini sampai
melewati ledger target, disaring dengan `liquidity_pool.id`.

Effect terakhir yang menyentuh pool ini sebelum serangan bertanggal
**2026-02-10T16:59:35Z**, dan effect berikutnya bertanggal **2026-02-22T22:08:33Z**.
Ledger 61340263 jatuh di antara keduanya, sehingga nilai reserve pada effect 10 Februari
berlaku persis pada saat serangan. Tidak diperlukan rekonstruksi aritmetika.

### Validasi silang

| Pemeriksaan | Hasil |
|---|---|
| `k` naik dari Februari ke sekarang | 252,912 menjadi 256,237, naik 3,32. Konsisten dengan akumulasi fee 30 bps |
| Reserve terbaca langsung dari effect, bukan hasil hitungan | ya, sehingga tidak ada risiko kesalahan pembalikan |
| Pool ada sebelum insiden | ya, operasi terindeks sejak 1 Juli 2025 |

Risiko sisa yang perlu dicatat: metode ini mengandaikan seluruh perubahan state pool
menghasilkan effect yang terindeks di endpoint tersebut. Untuk deposit, withdraw, dan
trade hal itu berlaku. Kasus langka seperti pencabutan otorisasi trustline belum
diperiksa terpisah.

---

## 2. Jendela diam pool

| | |
|---|---|
| Effect pool terakhir sebelum serangan | 2026-02-10T16:59:35Z |
| Serangan | 2026-02-22T00:10:21Z |
| Effect pool berikutnya | 2026-02-22T22:08:33Z |
| Diam sebelum serangan | 11 hari 7 jam |
| Diam setelah serangan | 21 jam 58 menit |
| Total jendela diam | 12 hari 5 jam |

Pool menyimpan harga jujur 1,0555 tanpa terganggu sepanjang serangan, dan selama hampir
sehari penuh sesudahnya.

---

## 3. Temuan utama: manipulasi ke atas tidak menghadapi tekanan arbitrase

Ini penjelasan atas jendela diam di atas, dan ia lebih tajam daripada temuan biaya nol.

Pasar dianggap punya mekanisme koreksi sendiri berupa arbitrase. Mekanisme itu **tidak
bekerja** pada serangan ini, dan alasannya struktural.

Keadaan buku pada saat serangan:

| Sumber | Harga | Sifat |
|---|---|---|
| Pool | 1,0555442 | dapat dieksekusi dua arah |
| Bid orderbook | 1,0570000 | dapat dieksekusi, ukuran 0,0001 USTRY |
| Ask orderbook | 106,7372828 | dapat dieksekusi, tetapi tidak ada yang mau |

Arbitrase membutuhkan transaksi yang menguntungkan. Satu-satunya kandidat adalah membeli
dari pool pada 1,0555 lalu menjual ke bid pada 1,0570, selisih 0,14 persen, di bawah fee
pool 0,30 persen. Tidak menguntungkan.

Ask di 106,74 tidak menciptakan peluang apa pun, karena tidak ada pihak yang menawarkan
**membeli** pada harga itu. Order jual berharga absurd bukan peluang arbitrase, ia hanya
order yang tidak diambil siapa pun.

**Perumusan umum:**

> Arbitrase hanya mengoreksi kesalahan harga yang **dieksekusi**. Ia tidak mengoreksi
> kesalahan harga yang sekadar **dikutip** atau **dilaporkan**. Oracle yang membaca
> kutipan atau harga trade terakhir sedang membaca tepat bagian pasar yang tidak
> dipertahankan oleh mekanisme apa pun.

Ini melengkapi temuan biaya nol. Serangan ini murah **dan** tidak dikoreksi, dan kedua
sifat itu berasal dari akar yang sama: penyerang memindahkan angka yang dilaporkan tanpa
memindahkan pasar.

### Konsekuensi bagi Keel

Depth mengukur likuiditas yang dapat dieksekusi, yaitu tepat bagian pasar yang dijaga
arbitrase. Selisih antara harga yang dikutip dan harga yang didukung depth adalah
permukaan serangannya. Itulah yang Keel ukur, dan alasan mengapa metrik ini benar bukan
sekadar berguna.

---

## 4. Yang harus diperbarui

### 4.1 Aturan harga acuan (metodologi bagian 3)

Aturan sekarang memerintahkan memakai mid orderbook ketika kedua sisi terisi. Pada
fixture ini aturan itu menghasilkan 53,8971414 untuk aset yang likuiditas nyatanya
berada pada 1,0555442, meleset 50 kali lipat.

```
1. Ada pool DAN ada buku dua sisi
     jika |mid_buku − spot_pool| / spot_pool > AmbangDivergensi
         P0 = spot_pool,  priceSource = "pool",  flag PRICE_SOURCE_CONFLICT
     selain itu
         P0 = mid_buku,   priceSource = "book"
2. Hanya buku dua sisi        -> mid buku
3. Hanya buku satu sisi + pool -> spot pool
4. Hanya pool                  -> spot pool
5. Tidak keduanya              -> none
```

Alasan: ketika dua sumber harga bertentangan, percayai yang didukung likuiditas yang
dapat dieksekusi, dan nyatakan konfliknya kepada konsumen. Menyembunyikan konflik lebih
buruk daripada memilih salah.

### 4.2 Biaya manipulasi dipecah dua

| Metrik | Isi | Menjawab |
|---|---|---|
| `manipulationCostCombined` | SDEX dan AMM digabung | biaya menggeser harga pasar sebenarnya |
| `manipulationCostOrderbookOnly` | hanya SDEX | biaya menipu oracle yang membaca trade SDEX |

Pada fixture ini, mencapai 106,7372828 berbiaya sekitar 147,96 USDC lewat pool dan nol
lewat orderbook. Penyerang membayar nol. Selisih kedua angka itu sendiri adalah sinyal:
aset yang `combined`-nya besar tetapi `orderbookOnly`-nya kecil terlihat aman padahal
tidak.

Catatan kejujuran: pernyataan bahwa oracle hanya membaca trade SDEX adalah **inferensi**
dari fakta bahwa pool jujur ada dan serangan tetap berhasil. Tandai sebagai inferensi
sampai ada konfirmasi dari Reflector.

### 4.3 Semantik MaxReachablePrice

Pada constant product, harga menuju tak hingga saat reserve base menuju nol. Maka ketika
ada pool aktif:

- `Reachable` selalu true di sisi beli
- `MaxReachablePrice` bernilai null, disertai warning

Kedua field itu hanya bermakna untuk pasar murni orderbook. Syarat `Reachable == true`
pada `MANIPULATION_CHEAP` tetap dipertahankan, karena tetap mengikat untuk aset tanpa pool.

### 4.4 Bagian keterbatasan

Tambahkan, dengan bukti dari data ini:

> Likuiditas yang ada tetapi tidak diperdagangkan tidak memberikan perlindungan terhadap
> oracle berbasis trade. Pool USTRY/USDC menyimpan cadangan jujur pada 1,0555 selama 12
> hari yang mencakup seluruh rentang serangan, dan tidak mencegah apa pun.

---

## 5. Berkas yang terdampak

| Berkas | Perubahan |
|---|---|
| `testdata/fixtures/ustry_pre_exploit.md` | tambahkan pool, hitung ulang seluruh tabel dengan `P0 = 1,0555442`, naikkan ke v1.0.3 |
| `docs/methodology/00-inti.md` | bagian 3 aturan `P0`, bagian 7 pemecahan biaya manipulasi, bagian 12 keterbatasan, bagian baru tentang asimetri arbitrase |
| `docs/methodology/09-flag-dan-band.md` | tambah `PRICE_SOURCE_CONFLICT` tingkat HIGH |
| `internal/domain/types.go` | `ManipulationCostCombined`, `ManipulationCostOrderbookOnly`, `PoolSpotPrice`, `PriceDivergencePct` |
| `docs/api/openapi.yaml` | field yang sama, plus contoh respons `assetPriceConflict` |
| `CLAUDE.md` | tiga gotcha gagal-diam, plus larangan konversi ledger ke waktu |

---

## 6. Gotcha yang ditemukan sepanjang verifikasi ini

Ketiganya **gagal diam**, bukan gagal keras, dan ketiganya sudah terbukti nyata pada
data proyek ini. Masing-masing layak menjadi test di adapter.

1. **Tipe aset.** USTRY berkode lima karakter sehingga bertipe `credit_alphanum12`.
   Query dengan `credit_alphanum4` mengembalikan array kosong tanpa pesan error.
2. **Sumber harga.** `/offers` mengirim `price_r` sebagai angka JSON, `/trades` mengirim
   `price` sebagai string JSON, dan arahnya bergantung pada aset mana yang menjadi base.
   String `price` adalah hasil pembulatan dan tidak boleh dipakai menghitung.
3. **Effect pool.** `/liquidity_pools/{id}/effects` mengembalikan seluruh effect dari
   operasi yang menyentuh pool tersebut, termasuk effect pada pool lain dalam path
   payment yang sama. Tanpa penyaringan `liquidity_pool.id`, Anda membaca reserve pool
   yang salah tanpa peringatan apa pun.

Ditambah satu larangan: **jangan menurunkan waktu dari nomor ledger secara aritmetika.**
Asumsi lima detik per ledger meleset sekitar tiga minggu pada rentang enam bulan. Ambil
`closed_at` dari `/ledgers/{seq}`.

### Bonus untuk metodologi

Keluaran effect memperlihatkan `trade` dan `liquidity_pool_trade` muncul bercampur dalam
satu operasi yang sama. Itu path payment yang dirutekan menembus orderbook dan pool
sekaligus.

Ini bukti empiris langsung untuk aturan penggabungan di metodologi bagian 6: likuiditas
SDEX dan AMM memang bersaing di rentang harga yang sama dan dikonsumsi bersama oleh satu
order. Menjumlahkan depth keduanya secara terpisah bertentangan dengan cara protokol
benar-benar bekerja. Simpan satu contoh operasi seperti ini sebagai lampiran.
