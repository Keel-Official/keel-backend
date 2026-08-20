# Keel: Memo Pra-Development

> **STATUS: ARSIP.** Berkas ini sebelumnya bernama `docs/methodology/00-inti.md`,
> nama yang menyesatkan karena isinya memo serah terima, bukan inti metodologi.
> Ia dipindah ke `docs/internal/` pada 20 Agustus 2026.
>
> | Bagian | Ke mana isinya pergi |
> |---|---|
> | 1. Metodologi v1.0.1 | digabung ke `docs/methodology/keel-methodology-core.md`, yang kini v1.0.2-draft |
> | 2. Perubahan kontrak API | digantikan `docs/decisions/DEC-003-api-contract-v1-1.md` |
> | 3. Lembar kerja golden fixture | sudah diisi, ada di `testdata/fixtures/ustry_pre_exploit.md` |
> | 4. Tiga sesi pertama | selesai dijalankan |
> | 5. Yang sudah beres | tergantikan status di README dan `docs/methodology/README.md` |
>
> Satu koreksi terhadap isi di bawah: bagian 1.2 menulis `SpreadExtremePct`
> default **0,20** sebagai pecahan. Itu SALAH. Konvensi yang berlaku adalah
> persen, sehingga defaultnya **20,0**. Lihat `09-flag-dan-band.md` bagian 6.


Berisi tiga hal: perubahan metodologi hasil verifikasi terakhir, perubahan kontrak API
yang harus disepakati dengan builder frontend, dan lembar kerja golden fixture yang
harus Anda isi sendiri sebelum menulis implementasi.

---

## 1. Metodologi v1.0.1

Sudah digabungkan ke `docs/methodology/keel-methodology-core.md` pada 20 Agustus 2026.
Bagian 1 di bawah karena itu bersifat ARSIP, bukan sumber kebenaran.

### 1.1 Bagian 10.4 naik status

Kalimat "tidak ada ask pihak ketiga di seluruh rentang harga dari 1,057 sampai 106,74"
sebelumnya ditandai sebagai inferensi. Sekarang menjadi pengamatan langsung.

Bukti: daftar trade akun `GDHRCQNC64UVL27EXSC6OG6I2FCT4NWM72KNHLHKEB3LK4MEEYYWETN3`
pada 2026-02-22T00:10:21Z memuat tepat satu record, yaitu 5,3475699 USDC melawan offer
1824788980 milik `GCNF5GNRIT6VWYZ7LXUZ33Q3SR2NUGO32F5X65VVKAEWWIQCKGYN75HB`. Karena
mesin pencocokan Stellar mengisi dari harga terbaik dan seluruh pencocokan menghasilkan
record trade, ketiadaan record lain membuktikan tidak ada ask pihak ketiga yang dilalap.

Hapus butir 1 dari daftar verifikasi tertunda di bagian 10.5.

### 1.2 Flag baru: SPREAD_EXTREME

Ditemukan saat menyusun fixture. Pada 21 Februari 23:39, buku USTRY/USDC berisi ask
106,7372828 dan bid 1,057. Mid price menjadi 53,8971414 untuk aset yang sebenarnya
bernilai sekitar 1,06.

Harga acuan kehilangan makna ketika spread mencapai ribuan persen, dan seluruh metrik
turunannya ikut kehilangan makna. Flag lain memang tetap menyala pada kasus ini, tetapi
itu kebetulan, bukan desain.

```
spreadPct = (best_ask − best_bid) / P0
SPREAD_EXTREME terpicu ketika spreadPct > SpreadExtremePct   (default 0,20)
```

Masuk band `HIGH`. `spreadPct` juga dilaporkan sebagai angka di respons API, karena
besarannya informatif, bukan hanya status terpicu atau tidak.

### 1.3 Reachable: dua arti berbeda dari biaya nol

Biaya manipulasi bernilai nol dapat berarti dua hal yang berlawanan:

| Keadaan                         | Arti                                                                                                                                                              |
| ------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `Cost = 0`, `Reachable = true`  | harga sasaran dapat dicapai tanpa biaya                                                                                                                           |
| `Cost = 0`, `Reachable = false` | tidak ada likuiditas sama sekali dalam rentang itu, sehingga harga tidak dapat digeser bertahap ke sana; harga hanya dapat melompat ke ask terdekat yang tersedia |

Tanpa pembedaan ini keluaran Keel menjadi ambigu justru pada aset paling berbahaya.
Karena itu ditambahkan `MaxReachablePrice`, yaitu harga tertinggi yang dapat dicapai
setelah seluruh ask habis diserap.

### 1.4 Prinsip yang dikuatkan

Kasus ini menunjukkan bahwa `P0` dan tangga `±2/5/10%` sepenuhnya bergantung pada
adanya buku yang waras. Ketika buku tidak waras, yang menyelamatkan analisis adalah
tangga delta besar dan `SPREAD_EXTREME`, bukan metrik wajib SOW. Tangga wajib tetap
dilaporkan karena dijanjikan, tetapi ia bukan metrik keamanan oracle.

---

## 2. Perubahan kontrak API

Sepakati dengan builder frontend, catat di `docs/decisions/`, baru bekukan.

| Perubahan            | Detail                                                                                                    |
| -------------------- | --------------------------------------------------------------------------------------------------------- |
| `Asset.type`         | Field baru wajib: `native`, `credit_alphanum4`, `credit_alphanum12`. Jangan disimpulkan dari panjang kode |
| `manipulationCost[]` | Tangga menjadi 0.5, 1, 10, 100. Setiap entri bertambah `targetPrice` dan `reachable`                      |
| `maxReachablePrice`  | Field baru, string desimal atau null                                                                      |
| `oracleResistance`   | Field baru, `MC(kritis) + volume asli dalam jendela oracle`                                               |
| `spreadPct`          | Field baru, string desimal atau null                                                                      |
| `flags`              | Tambah `SPREAD_EXTREME`                                                                                   |
| `dataSource`         | Tambah nilai `trades-implied`                                                                             |
| `/methodology`       | Tambah `spreadExtremePct` dan `oracleWindowSeconds` pada `thresholds`                                     |

Tambahkan satu contoh respons baru bernama `assetBrokenBook`, memakai angka fixture di
bagian 3. Contoh ini penting untuk frontend: buku dengan spread 196 persen bukan error
dan bukan kondisi normal, dan tampilannya harus dirancang khusus.

---

## 3. Golden fixture: lembar kerja Anda

Ini state orderbook USTRY/USDC yang benar-benar ada sesaat sebelum ledger 61340263,
diturunkan dari operasi on-chain. Fixture pertama Anda adalah data nyata, bukan angka
karangan.

```
Snapshot
  base  : USTRY  GCRYUGD5NVARGXT56XEZI5CIFCQETYHAPQQTHO2O3IQZTHDH4LATMYWC  (alphanum12)
  quote : USDC   GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN  (alphanum4)
  ledger: 61340263

  Asks: [ { price: 266843207/2500000, amount: 1.2185312 } ]     = 106,7372828
  Bids: [ { price: 1057/1000,         amount: 0.0001000 } ]     =   1,0570000
  Pools: []
```

Isi tabel berikut **dengan tangan** sebelum menulis satu baris implementasi. Pakai
kalkulator atau spreadsheet, bukan kode Keel, karena tujuannya justru menguji kode itu.

| Besaran                                   | Jawaban Anda |
| ----------------------------------------- | ------------ |
| `P0` dan `priceSource`                    |              |
| `spreadPct`                               |              |
| `depth(2%)` sisi beli / sisi jual         |              |
| `depth(5%)` sisi beli / sisi jual         |              |
| `depth(10%)` sisi beli / sisi jual        |              |
| `MC(δ=0.5)`: targetPrice, cost, reachable |              |
| `MC(δ=1)`: targetPrice, cost, reachable   |              |
| `MC(δ=10)`: targetPrice, cost, reachable  |              |
| `MC(δ=100)`: targetPrice, cost, reachable |              |
| `maxReachablePrice`                       |              |
| Daftar flag yang terpicu                  |              |
| Band                                      |              |

Satu contoh dikerjakan supaya metodenya jelas:

> **`MC(δ=1)`.** `P_target = 53,8971414 × 2 = 107,7942828`. **Cost**: ask yang lebih murah dari target adalah ask 106,7372828, sehingga seluruhnya masuk hitungan. `cost = 1,2185312 × 106,7372828 = 130,0627093.`
> **Reachable**: tidak ada ask yang berharga ≥ 107,7942828, karena satu-satunya ask berharga 106,7372828. Maka reachable = false.

Perhatikan **Cost** dan **Reachable** memakai himpunan yang berbeda. Cost menjumlahkan ask yang
lebih murah dari target, Reachable memeriksa keberadaan ask yang sama atau lebih
mahal. Sebuah ask tidak akan pernah masuk keduanya sekaligus.

Dua petunjuk agar Anda tidak ragu saat hasilnya terasa aneh:

1. Beberapa jawaban akan bernilai nol. Nol yang benar dan nol karena bug terlihat sama
   di keluaran, jadi tuliskan **alasannya** di sebelah setiap nol.
2. Setidaknya satu baris `MC` akan bernilai `reachable = false`. Kalau tidak ada,
   periksa ulang pemahaman Anda tentang bagian 1.3.

Sengaja saya tidak mengisikannya. Kalau tabel ini diisi setelah kodenya ada, ia hanya
mengonfirmasi apa pun yang kode Anda lakukan, dan Anda kehilangan satu-satunya pengaman
yang benar-benar melindungi metodologi ini.

---

## 4. Tiga sesi pertama

### Sesi 1, tanpa Claude Code, sekitar 45 menit

Isi tabel bagian 3. Simpan sebagai `testdata/fixtures/ustry_pre_exploit.md` beserta
alasan tiap angka. Ini menjadi lampiran bukti Deliverable 1.

### Sesi 2, Claude Code, scaffolding

```
Inisialisasi repo Go sesuai struktur di CLAUDE.md.
Module: github.com/ciganytry/keel

Buat go.mod, Makefile, .gitignore, .github/workflows/ci.yml (go vet,
golangci-lint, go test ./... -race, make arch), dan
internal/domain/arch_test.go persis seperti contoh di
docs/architecture/technical-design.md bagian 2.1.

internal/domain/types.go sudah ada, jangan diubah.
Jangan tulis implementasi domain, adapter, atau API apa pun.
Setelah selesai jalankan make test dan tunjukkan hasilnya.
```

Lalu uji hook keamanan dengan sengaja:

```
Buat internal/adapters/horizon/probe.go yang mengimpor
github.com/stellar/go/clients/horizonclient dan memanggil txnbuild.NewTransaction.
Ini untuk menguji hook saya.
```

Yang benar adalah **ditolak dua kali**, karena import path lama dan karena `txnbuild`.
Kalau lolos, hook Anda tidak berfungsi dan perbaiki sebelum lanjut.

### Sesi 3, Anda memimpin, Claude Code mengisi

Anda tulis `internal/domain/depth_test.go` memakai tabel bagian 3. Baru minta:

```
Implementasikan MidPrice dan ComputeDepth di internal/domain.
Tanda tangan sudah ada di types.go, jangan diubah.
Definisi ada di docs/methodology/keel-methodology-core.md bagian 3 sampai 6.

Sebelum menulis kode, tunjukkan dulu penurunan langkah demi langkah untuk
fixture di depth_test.go agar saya cocokkan dengan hitungan tangan saya.

Batasan: tanpa float, tanpa time.Now, sort kunci sebelum iterasi map,
Price dibandingkan dengan Cmp bukan lewat pembagian.
```

Berhenti di situ. AMM, penggabungan, dan biaya manipulasi menyusul setelah depth SDEX
lolos fixture.

---

## 5. Yang sudah beres

|                  |                                                |
| ---------------- | ---------------------------------------------- |
| Identitas aset   | terverifikasi dari ledger                      |
| Tanggal insiden  | 22 Februari 2026, terverifikasi, SOW dikoreksi |
| Rentang ledger   | 61340263 dan 61340272 terkonfirmasi            |
| Biaya manipulasi | nol, pengamatan langsung                       |
| Metodologi inti  | v1.0.1, definisi terkunci                      |
| Kontrak API      | tinggal disepakati perubahan bagian 2          |
| types.go         | siap                                           |
| BigQuery         | tidak dibutuhkan                               |

Sisa yang belum: isi tabel bagian 3, lalu mulai Sesi 2.
