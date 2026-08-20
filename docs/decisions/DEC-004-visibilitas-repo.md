# DEC-004: Visibilitas Repo dan Syarat Membukanya

**Keputusan:** Repo `Keel-Official/keel` tetap **privat** sampai `internal/depth` lolos
golden fixture. Sebelum visibilitasnya diubah ke publik, dua berkas wajib dikeluarkan
lebih dulu.
**Status:** BERLAKU sejak 20 Agustus 2026, commit pertama.
**Terkait:** DoD Deliverable 1 bagian 6 mensyaratkan repo publik sebagai bukti.

---

## 1. Kenapa tidak langsung publik

DoD menjanjikan repo publik, jadi visibilitas ini bukan pertanyaan apakah, melainkan
kapan. Yang dipertimbangkan: pada commit pertama, `internal/depth` masih kosong, job
`conformance` merah karenanya, dan job `golangci-lint` merah karena versi action.
Pembaca pertama yang datang dari tautan SCF akan melihat dua job merah dan folder inti
yang kosong, dan tidak punya konteks untuk tahu bahwa keduanya disengaja.

Menunggu sampai golden fixture lolos memberi kesan pertama yang berbeda dengan biaya
nol, karena riwayat commit-nya tetap utuh dan tetap menunjukkan pekerjaan berjalan
sejak 20 Agustus. Yang hilang kalau menunggu: tidak ada. Repo privat sudah cukup
sebagai cadangan kerja.

**Pemicu membuka:** `make conformance` lolos tanpa build tag, yaitu saat
`internal/conformance/golden_test.go` tidak lagi butuh `//go:build conformance` dan
job `conformance` di CI hijau tanpa `continue-on-error`.

---

## 2. Dua berkas yang wajib keluar sebelum publik

| Berkas | Alasan |
|---|---|
| `docs/context/Keel_SoW.pdf` | Memuat anggaran 126 jam dan 2.268 dolar serta ketentuan dengan pemberi dana. Itu dokumen antara Al dan Instawards, bukan bahan yang perlu dibaca publik untuk menilai metodologi |
| `docs/internal/` | Memo pra-development, rencana eksekusi, dan audit repo. Isinya kritik terhadap repo sendiri, koreksi terhadap SOW, dan alokasi jam kerja |

Perhatikan bahwa **`git rm` saja tidak cukup.** Keduanya sudah masuk riwayat pada commit
`f499ab4`, jadi menghapusnya di commit berikutnya tetap menyisakannya di riwayat dan
tetap terbaca siapa pun setelah repo publik. Yang berlaku salah satu dari dua jalan:

1. Tulis ulang riwayat dengan `git filter-repo` sebelum repo dibuka. Bisa dilakukan
   selama repo masih privat dan belum ada fork atau clone pihak lain.
2. Pindahkan seluruh isi ke repo publik baru dengan satu commit awal bersih, dan
   simpan repo ini sebagai arsip privat.

Jalan 1 lebih murah selama pihak lain belum meng-clone. Jalan 2 lebih aman dan
kehilangan riwayat commit, yang justru salah satu bukti yang ingin ditunjukkan. Pilih
saat visibilitas benar-benar diubah, jangan sekarang, karena jumlah clone pihak lain
belum diketahui pada saat itu.

**Yang tetap tinggal dan memang harus publik:** `docs/methodology/`,
`docs/decisions/`, `docs/architecture/`, `docs/api/`, `docs/evidences/`, dan
`testdata/fixtures/`. Justru itu isi deliverable-nya. `docs/evidences/` berisi data
on-chain yang siapa pun bisa ambil sendiri dari Horizon, jadi tidak ada yang perlu
disembunyikan di sana.

---

## 3. Kenapa keputusan ini ditulis

Keputusan yang hanya hidup di kepala akan hilang. Kegagalan yang paling mungkin terjadi
di sini bukan lupa membuka repo, melainkan repo dibuka bulan depan dalam keadaan
tergesa-gesa menjelang tenggat, tanpa ada yang ingat bahwa SoW ikut di dalamnya.

Karena itu syaratnya juga ditegakkan mekanis, bukan hanya ditulis:
`scripts/verifikasi-audit.sh` bagian "Syarat visibilitas repo" memeriksa visibilitas
repo lewat `gh`, dan berteriak kalau repo sudah publik sementara salah satu berkas masih
ada. Aturan yang hanya ditulis di dokumen akan dilanggar dalam dua minggu.

---

## 4. Yang mengubah keputusan ini

- Ambassador Chapter Lead atau reviewer SCF meminta tautan repo publik sebelum
  `internal/depth` selesai. Kalau itu terjadi, buka repo lebih awal dan tambahkan satu
  paragraf di README yang menjelaskan job merah mana yang disengaja dan kenapa.
- Muncul kebutuhan kolaborator luar yang tidak bisa diberi akses privat.
