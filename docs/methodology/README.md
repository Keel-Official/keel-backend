# Metodologi Keel: indeks dan status

**Versi metodologi berlaku:** `1.0.2-draft`
**Sinkron dengan:** `internal/domain.MethodologyVersion`

Berkas ini adalah peta. Ia tidak memuat definisi apa pun, supaya tidak menjadi
tempat kedua yang menyimpang dari yang pertama.

---

## 1. Sumber kebenaran

| Berkas | Isi | Berlaku untuk |
|---|---|---|
| `keel-methodology-core.md` | besaran yang dihitung: `P0`, `spreadPct`, depth SDEX dan AMM, penggabungan, biaya manipulasi, `Reachable`, `MaxReachablePrice`, ketahanan oracle, `C_max`, validasi empiris, keterbatasan | `internal/depth` |
| `09-flag-dan-band.md` | definisi flag, tiga keadaan flag, penurunan band, `bandConfidence`, seluruh nilai ambang | `internal/depth` |

Kalau keduanya berbeda soal flag, band, atau ambang, `09-flag-dan-band.md` yang
berlaku. Kalau berbeda soal besaran yang dihitung, `keel-methodology-core.md`
yang berlaku.

Golden fixture beserta seluruh nilai harapannya ada di
`testdata/fixtures/ustry_pre_exploit.md`, dihitung dengan tangan sebelum
implementasi ada, dan diterjemahkan ke Go di `internal/conformance/`.

---

## 2. Konvensi satuan

Dua konvensi berbeda hidup berdampingan dengan sengaja. Mencampurnya adalah
sumber bug diam yang tidak menggagalkan apa pun.

| Bentuk | Contoh | Arti |
|---|---|---|
| `δ`, masukan rumus | `δ = 0,02` | pecahan, berarti 2 persen |
| berakhiran `Pct`, besaran yang dilaporkan | `spreadPct = 196,0777141` | persen, berarti 196 persen |

`SpreadExtremePct` bernilai `20,0` dan dibandingkan langsung dengan `spreadPct`.
Arsip `docs/internal/memo-pra-development.md` bagian 1.2 menulisnya `0,20`
sebagai pecahan. Itu salah dan sudah dikoreksi di sana.

---

## 3. Terhadap Definition of Done Deliverable 1

DoD di `docs/internal/Keel_Deliverable_1_Rencana_Eksekusi.md` bagian 6 menjanjikan
**sebelas berkas** di folder ini dengan penomoran `00` sampai `10`. Struktur yang
sekarang berbeda, dan perbedaannya harus diputuskan, bukan dibiarkan.

| Dijanjikan DoD | Ada sekarang | Status |
|---|---|---|
| `00-ikhtisar.md` | `keel-methodology-core.md` bagian 1 dan 2 | isi ada, nama berbeda |
| `01-sumber-data.md` | tersebar di TDD bagian 3 dan `DEC-002` | **belum ditulis** sebagai metodologi |
| `02-harga-acuan.md` | `keel-methodology-core.md` bagian 3 dan 3a | isi ada, nama berbeda |
| `03-depth-sdex.md` | `keel-methodology-core.md` bagian 4 | isi ada, nama berbeda |
| `04-depth-amm.md` | `keel-methodology-core.md` bagian 5 | isi ada, nama berbeda |
| `05-penggabungan.md` | `keel-methodology-core.md` bagian 6 | isi ada, nama berbeda |
| `06-pemilihan-pasangan.md` | keputusan D-1 di rencana eksekusi | **belum ditulis** sebagai metodologi |
| `07-metrik-pendukung.md` | keputusan D-4 sampai D-6 di rencana eksekusi | **belum ditulis** sebagai metodologi |
| `08-collateral.md` | `keel-methodology-core.md` bagian 9 | isi ada, tetapi asal parameter default belum dirujuk ke parameter Blend yang sebenarnya |
| `09-validasi.md` | belum ada, dan nomor 09 sudah dipakai `09-flag-dan-band.md` | **belum ditulis**, dan **nomornya bentrok** |
| `10-keterbatasan.md` | `keel-methodology-core.md` bagian 12 | isi ada, nama berbeda |

Tidak ada berkas untuk flag dan band di daftar DoD, padahal `09-flag-dan-band.md`
ada dan berisi definisi yang mengikat. Penomoran DoD sudah tidak menggambarkan
kenyataan.

### Keputusan yang harus diambil

Ada dua jalan dan keduanya sah. Yang tidak sah adalah membiarkannya seperti
sekarang, karena DoD ini yang akan dibaca reviewer SCF Build.

1. **Pecah mengikuti DoD.** `keel-methodology-core.md` dibelah menjadi sepuluh
   berkas bernomor, `09-flag-dan-band.md` dinomori ulang, dan tiga berkas yang
   belum ada ditulis. Biayanya besar dan `09-flag-dan-band.md` sudah dirujuk dari
   enam tempat.
2. **Amandemen DoD.** Struktur sekarang dipertahankan, DoD bagian 6 diubah agar
   menyebut struktur yang benar, dan tiga isi yang belum ada tetap wajib ditulis
   di mana pun tempatnya.

Rekomendasi: jalan 2. Reviewer membaca isi, bukan nama berkas, dan tiga
kekurangan nyata (sumber data, pemilihan pasangan, metrik pendukung, ditambah
protokol validasi) tetap harus diisi pada jalan mana pun. Memecah berkas tidak
mengurangi satu pun dari kekurangan itu, hanya menambah pekerjaan penomoran.

---

## 4. Riwayat versi

Ada di `keel-methodology-core.md` bagian 13 dan `09-flag-dan-band.md` bagian 9.
Keduanya wajib naik bersama.
