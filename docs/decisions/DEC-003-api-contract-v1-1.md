# Keel: Perubahan Kontrak API v1.1.0

**Keputusan:** Kontrak API naik ke 1.1.0 untuk membawa metodologi v1.0.1, yaitu
`SPREAD_EXTREME`, pembedaan `reachable`, tangga manipulasi yang diperluas, dan
ketahanan oracle.
**Status:** DRAFT. Belum dibekukan. Lihat bagian 7 untuk syarat pembekuan.
**Sumber perubahan:** `docs/internal/memo-pra-development.md` bagian 1 dan 2.
**Berkas terdampak:** `docs/api/keel-openapi.yaml`

---

## 1. Kenapa versi minor, bukan patch

Tiga perubahan memutus kompatibilitas untuk konsumen yang sudah menulis kode:

| Perubahan | Kenapa memutus |
|---|---|
| `Asset.type` menjadi wajib | Konsumen yang memvalidasi objek Asset secara ketat akan menolak respons lama maupun baru sampai skemanya diperbarui |
| Tangga `manipulationCost` berubah dari 2 entri menjadi 4 | Kode yang membaca `manipulationCost[1]` sebagai delta 1.0 tetap benar, tetapi kode yang mengasumsikan panjang array 2 akan salah |
| `cost` sekarang wajib dibaca bersama `reachable` | Kode lama yang menampilkan `cost` sendirian sekarang menampilkan angka yang menyesatkan pada kasus `reachable: false`. Ini kerusakan diam, bukan kerusakan yang melempar error |

Yang ketiga adalah yang paling berbahaya, karena tidak ada yang gagal. Kode lama
tetap jalan dan tetap menampilkan angka. Angkanya saja yang salah arti. Karena itu
perubahan ini harus dikomunikasikan ke builder frontend secara eksplisit, tidak
cukup lewat changelog.

Belum ada konsumen produksi, jadi biaya pemutusan ini nol sekarang dan tidak akan
nol lagi setelah dibekukan.

---

## 2. Daftar perubahan dan alasannya

### 2.1 `Asset.type` wajib

Nilai: `native`, `credit_alphanum4`, `credit_alphanum12`.

Jenis aset dikirim eksplisit, bukan disimpulkan dari panjang `code`. Kode empat
karakter atau kurang boleh diterbitkan sebagai `credit_alphanum12`, dan Horizon
melaporkannya apa adanya. Konsumen yang menebak dari panjang kode akan menyusun
identitas aset yang berbeda dari aset yang sebenarnya diukur Keel.

**Alternatif yang ditolak:** membiarkan konsumen menebak dan cukup mendokumentasikan
kaidahnya. Ditolak karena kaidahnya tidak selalu benar, dan kesalahannya baru
kelihatan pada aset langka, yaitu justru kelas aset yang menjadi alasan Keel ada.

### 2.2 Tangga `manipulationCost` menjadi 0.5, 1, 10, 100

Setiap entri sekarang membawa `targetPrice` dan `reachable` di samping `cost`.

Tangga besar ditambahkan karena aset dengan buku rusak tidak terbaca oleh tangga
kecil. Pada fixture USTRY, satu-satunya ask berada jauh di atas `P0 x 1.5`, jadi
tangga 0.5 tidak menyentuh likuiditas apa pun sementara tangga 1 menyerap seluruh
buku. Tanpa tangga 10 dan 100 tidak terlihat bahwa di atas itu tidak ada apa-apa lagi.

`targetPrice` dikirim, bukan dibiarkan dihitung ulang konsumen, karena `midPrice`
pada aset berbuku rusak tidak dapat dipercaya sebagai basis perkalian di sisi klien.
Mengirimkannya membuat setiap baris tangga dapat dibaca sendiri.

### 2.3 `reachable` dan `maxReachablePrice`

Ini inti perubahan. `cost: "0"` punya dua arti yang berlawanan:

| cost | reachable | arti |
|---|---|---|
| 0 | true | harga sasaran dapat dicapai tanpa biaya |
| 0 | false | tidak ada likuiditas sama sekali dalam rentang itu |

Tanpa pembedaan ini keluaran Keel ambigu justru pada aset paling berbahaya.
`maxReachablePrice` melengkapinya dengan menyatakan batas atas pergerakan harga
lewat buku, sehingga konsumen dapat memeriksa sendiri bahwa setiap `targetPrice`
di atas nilai itu memang `reachable: false`.

**Alternatif yang ditolak:** mengirim `cost: null` untuk kasus tidak terjangkau.
Ditolak karena membuang informasi. Pada tangga delta 10 di contoh historis, biaya
2210.4400000 tetap bermakna: itu biaya menghabiskan seluruh ask. Yang hilang bukan
biayanya, melainkan tercapainya sasaran.

`maxReachablePrice` bernilai null dalam dua keadaan berbeda: tidak ada ask sama
sekali, atau likuiditas seluruhnya dari AMM. Kurva constant product tidak punya
batas atas harga, sehingga tidak ada maksimum yang dapat dilaporkan. Keduanya
sengaja tidak dibedakan lewat nilai, karena `priceSource` sudah membedakannya.

### 2.4 `spreadPct` dan flag `SPREAD_EXTREME`

`spreadPct = (best_ask - best_bid) / midPrice`, dilaporkan sebagai angka, bukan
hanya status terpicu.

**Perbedaan dari metodologi, butuh persetujuan Al.** `docs/internal/memo-pra-development.md`
bagian 1.2 menulis `SpreadExtremePct` default 0,20 sebagai pecahan. Kontrak API
memakai skala persen: `spreadExtremePct: '20.0'`, dan `spreadPct: '196.0777141'`
untuk spread 196 persen.

Alasannya konsistensi internal. Seluruh field berakhiran `Pct` yang sudah ada di
API ini berskala persen: `holderTop1Pct: '11.4200000'`, `tradesExcludedPct:
'2.1000000'`, `manipulationRatioLowPct: '1.0'`. Satu field pecahan di antara field
persen adalah jebakan yang pasti dimakan seseorang.

Konsekuensinya nama variabel internal dan nilai API berbeda skala, dan konversi
harus terjadi di satu tempat yang jelas di lapisan API. Kalau Al lebih suka
pecahan, yang harus berubah adalah seluruh field `Pct` lain, bukan hanya yang ini.

`spreadPct` bernilai null kalau salah satu sisi buku kosong atau `priceSource`
bukan `book`. Spread tidak terdefinisi tanpa dua sisi buku.

### 2.5 `oracleResistance`

Metodologi menulisnya sebagai `MC(kritis) + volume asli dalam jendela oracle`.
Dituangkan sebagai objek dengan lima field wajib: `criticalDelta`,
`manipulationCost`, `reachable`, `genuineVolume`, `windowSeconds`, ditambah
`ratio` yang boleh null.

**Alternatif yang ditolak:** satu skalar hasil bagi. Ditolak karena rasio menyembunyikan
dua keadaan yang harus terlihat. Pertama, `genuineVolume` nol membuat rasio tidak
terdefinisi, dan aset yang tidak diperdagangkan sama sekali dalam jendela oracle
adalah temuan penting, bukan data hilang. Kedua, rasio yang dihitung dari
`manipulationCost` dengan `reachable: false` adalah angka tanpa arti. Dengan bentuk
objek, kedua keadaan itu terbaca dan `ratio` cukup diisi null.

`windowSeconds` diulang di dalam objek meskipun sudah ada di `/methodology`, supaya
respons aset dapat dibaca dan diarsipkan tanpa memanggil endpoint lain.

`criticalDelta` default 0.5 dan selalu sama dengan salah satu nilai `delta` pada
`manipulationCost`.

### 2.6 `dataSource` menerima `trades-implied`

Dipakai ketika state orderbook pada ledger yang diminta tidak tersedia dan harga
serta likuiditas direkonstruksi dari catatan trade yang benar-benar tereksekusi.

Nilai ini diberlakukan pada `AssetRisk` dan juga pada `HistoryResponse`, lewat satu
skema `DataSource` bersama. Keduanya disatukan karena justru jalur historis yang
paling sering kehilangan snapshot, dan itu persis jalur yang dipakai studi kasus
Blend.

Trade membuktikan likuiditas yang terpakai, bukan likuiditas yang tersedia. Depth
hasil `trades-implied` adalah batas bawah dan wajib disertai warning. Frontend tidak
boleh menampilkannya setara hasil `horizon`.

Contoh `assetBrokenBook` memakai nilai ini, karena buku USTRY/USDC pada ledger
61340263 memang diturunkan dari operasi on-chain, bukan dari snapshot orderbook.
Jadi nilai baru ini punya satu contoh nyata, bukan hanya entri enum.

### 2.7 `/methodology` menambah dua ambang

`spreadExtremePct: '20.0'` dan `oracleWindowSeconds: 300`.

`thresholds` tetap `additionalProperties: true`. Konsumen wajib membaca lewat nama
kunci, bukan posisi, sehingga penambahan ambang berikutnya tidak memutus siapa pun.

`oracleWindowSeconds` adalah asumsi Keel, bukan bacaan dari oracle mana pun. Tiap
oracle punya jendela sendiri. Nilainya dilaporkan supaya konsumen dapat mengganti
dengan jendela oracle yang benar-benar mereka pakai.

---

## 3. Contoh `assetBrokenBook`

Ditambahkan sebagai `components.examples.AssetBrokenBook`, terhubung ke
`GET /asset/{assetId}/depth` dengan kunci `bukuRusak`.

Ini keadaan ketiga yang harus dibedakan frontend, di samping aset sehat dan aset
tanpa harga. USTRY/USDC sesaat sebelum ledger 61340263 punya tepat satu ask pada
106.7372828 dan tepat satu bid pada 1.0570000. Titik tengahnya 53.8971414 untuk
aset yang bernilai sekitar 1.06. Spread 196 persen. HTTP 200, bukan error, dan
bukan kondisi normal.

Tuntutan tampilan, sudah ditulis di `description` contohnya:

1. Jangan tampilkan `midPrice` sebagai harga tanpa penanda.
2. Redam tangga depth 2/5/10 persen. Ketiganya diturunkan dari `midPrice` yang
   sudah tidak bermakna dan tetap dilaporkan hanya karena dijanjikan di SOW.
3. Naikkan tangga manipulasi delta 10 dan 100 beserta `maxReachablePrice`.
4. Bedakan `cost: "0"` dengan `reachable: false` dari `cost: "0"` dengan
   `reachable: true`.

**Contoh ini belum lengkap dan itu disengaja.** Lihat bagian 4.

---

## 4. Angka yang sengaja belum diisi

`docs/internal/memo-pra-development.md` bagian 3 mensyaratkan tabel golden fixture diisi
dengan tangan sebelum satu baris implementasi ditulis, dan menyatakan alasannya:
tabel yang diisi setelah kodenya ada hanya mengonfirmasi apa pun yang kode itu
lakukan.

Alasan yang sama berlaku untuk contoh API. Menurunkan `depth`, `cost`, `reachable`,
dan `maxReachablePrice` di sini sama saja menyerahkan jawaban lembar kerjanya, dan
pengaman metodologi itu hilang.

Yang sudah diisi, karena sudah tertulis di dokumen metodologi:

| Field | Nilai | Asal |
|---|---|---|
| `midPrice` | `53.8971414` | bagian 1.2 |
| `spreadPct` | `196.0777141` | (106.7372828 - 1.057) / 53.8971414 x 100, angka 196 persen disebut di bagian 2 |
| `targetPrice` seluruh tangga | 80.8457121, 107.7942828, 592.8685554, 5443.6112814 | `midPrice x (1 + delta)`, rumus dan satu contoh diberikan di bagian 3 |
| `MC(delta=1)` cost dan reachable | `130.0627093`, `true` | contoh yang sudah dikerjakan di bagian 3 |
| `flags` | `[SPREAD_EXTREME]` | pasti terpicu pada spread 196 persen |
| `band` | `HIGH` | konsekuensi SPREAD_EXTREME menurut bagian 1.2 |

Yang menunggu Sesi 1, ditandai `TODO-FIXTURE` atau `reachable: null`: seluruh
`depth`, `cost` pada delta 0.5, 10, dan 100, `reachable` pada ketiganya,
`maxReachablePrice`, isi `oracleResistance`, `maxSafeCollateral`, dan metrik holder.

`flags` dan `band` pada contoh itu adalah batas bawah. Flag lain kemungkinan besar
ikut terpicu setelah fixture diisi, dan band dapat naik ke CRITICAL.

Penanda `TODO-FIXTURE` melanggar pola skema `Decimal`, dan `reachable: null`
melanggar tipe boolean. Itu disengaja: validator OpenAPI akan menolak berkas ini
sampai penandanya diganti, sehingga kontrak tidak dapat dibekukan secara tidak
sengaja.

---

## 5. Yang tidak diubah, dan alasannya

| Tidak diubah | Alasan |
|---|---|
| `AssetSummary` tidak menerima `spreadPct` | Tidak diminta di bagian 2. Halaman daftar sudah menerima `SPREAD_EXTREME` lewat `flags`, yang cukup untuk memberi penanda pada baris. Lihat pertanyaan terbuka di bagian 6 |
| `HistoryPoint.manipulationCost50Pct` tetap satu tangga | Deret waktu empat tangga membuat muatan respons membengkak tanpa permintaan konkret dari dashboard |
| Tangga depth tetap 2/5/10 persen | Dijanjikan di SOW. Bagian 1.4 metodologi sudah menyatakan tangga ini bukan metrik keamanan oracle, dan itu kini tertulis di deskripsi API |
| `Band` tetap flag terburuk, bukan pembobotan | Di luar cakupan perubahan ini |

---

## 6. Pertanyaan terbuka untuk builder frontend

1. Apakah halaman daftar butuh `spreadPct` di `AssetSummary`, atau cukup flag
   `SPREAD_EXTREME` untuk memberi penanda baris?
2. `criticalDelta` dipatok 0.5 untuk semua aset. Apakah dashboard butuh memilih
   delta kritis sendiri lewat parameter query?
3. Belum ada contoh respons dengan `oracleResistance.ratio` di bawah 1, yaitu
   keadaan paling berbahaya, karena tidak ada aset contoh yang punya volume asli
   nonzero dalam jendela 300 detik sekaligus biaya manipulasi rendah. Perlu
   contoh sintetis khusus untuk merancang tampilannya, atau tunggu data nyata?
4. Nilai `dataSource: trades-implied` perlu tampilan pembeda seperti apa? Depth-nya
   batas bawah, bukan pengukuran.

---

## 7. Syarat pembekuan

Kontrak baru boleh dibekukan setelah keempatnya selesai:

- [ ] Tabel golden fixture bagian 3 diisi dengan tangan, disimpan sebagai
      `testdata/fixtures/ustry_pre_exploit.md` beserta alasan tiap angka
- [ ] Seluruh penanda `TODO-FIXTURE` dan `reachable: null` pada `AssetBrokenBook`
      diganti angka fixture, dan `flags` serta `band` dilengkapi
- [ ] Skala `spreadPct` disepakati, lihat bagian 2.4. Kalau pecahan yang dipilih,
      metodologi tetap dan seluruh field `Pct` lain ikut berubah
- [ ] Empat pertanyaan bagian 6 dijawab builder frontend

Setelah itu `MethodologyVersion` naik ke 1.0.1 di implementasi, bukan hanya di
contoh respons.
