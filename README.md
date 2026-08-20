# Keel

Liquidity risk engine untuk ekosistem Stellar.

Oracle menjawab "berapa harganya". Keel menjawab "berapa volume yang bisa
ditopang harga itu".

## Keadaan sekarang

Repo ini sedang dalam pembangunan dan **inti metodologinya belum
diimplementasikan.** Yang sudah ada: definisi metodologi, kontrak API, golden
fixture yang dihitung dengan tangan dari data on-chain nyata, tipe bersama, dan
uji arsitektur yang menegakkan kemurnian paket. Yang belum: rumus di
`internal/depth`, adapter data, penyimpanan, dan API.

Konsekuensinya pada perintah yang bisa dijalankan:

| Perintah | Keadaan |
|---|---|
| `make test` | jalan, dan harus hijau |
| `make ci` | jalan, dan harus hijau |
| `make arch` | jalan, menegakkan kemurnian `internal/domain` dan `internal/depth` |
| `make up` | jalan, menyalakan Postgres lokal |
| `make conformance` | **sengaja merah.** Golden fixture adalah spesifikasi yang menunggu dipenuhi, dan `internal/depth` masih kosong |
| `make record` | **belum ada isinya**, keluar dengan kode 3 |
| `make scan` | **belum ada isinya**, keluar dengan kode 3 |
| `make serve` | **belum ada isinya**, keluar dengan kode 3 |

Kode keluar 3 dibedakan dari 1 dengan sengaja, supaya penjadwal dapat membedakan
"belum dibangun" dari "gagal".

## Mulai dari nol

```bash
git clone https://github.com/Keel-Official/keel.git
cd keel

make ci          # gofmt, build, vet, uji arsitektur, dan test. Harus hijau
go run ./cmd/keel version

make up          # nyalakan Postgres lokal, opsional pada tahap ini
```

Untuk melihat apa yang masih bercabang di repo ini sebelum ikut menulis:

```bash
bash scripts/verifikasi-audit.sh
```

Skrip itu menjalankan ulang setiap klaim di `docs/internal/audit-2026-08-20.md`
dan mencetak mana yang masih benar. Ia juga menghitung ulang aritmetika golden
fixture dari `price_r` mentah, di luar Go, sebagai pemeriksaan silang.

## Struktur

| Direktori | Isi | Keadaan |
|---|---|---|
| `cmd/keel` | entrypoint tunggal, beberapa subperintah | kerangka |
| `internal/domain` | tipe bersama, terutama `Snapshot`, tanpa perhitungan | ada |
| `internal/conformance` | golden fixture dan uji kesesuaian, black-box terhadap `internal/depth` | ada, menunggu `internal/depth` |
| `internal/depth` | inti metodologi, ditulis manual | **kosong** |
| `internal/horizon` | adapter data live | kosong |
| `internal/hubble` | adapter data historis, ditunda, lihat DEC-002 | kosong |
| `internal/store` | persistensi | kosong |
| `internal/api` | HTTP handler baca | kosong |
| `migrations` | skema Postgres | ada, dan masih bertentangan dengan TDD bagian 5 |
| `docs/methodology` | deliverable metodologi | ada |
| `docs/decisions` | catatan keputusan | ada |
| `docs/api` | kontrak OpenAPI | ada |
| `docs/evidences` | bukti on-chain mentah dari Horizon | ada |
| `docs/learning` | jurnal belajar | ada |
| `testdata/fixtures` | golden fixture, dihitung dengan tangan | ada |
| `scripts` | alat sekali pakai dan verifikasi | ada |

## Aturan yang tidak bisa ditawar

Ada di `CLAUDE.md`, dan sebagian ditegakkan mekanis oleh
`internal/domain/arch_test.go`: tanpa I/O di paket murni, tanpa `float64`, tanpa
`time.Now`, tanpa goroutine. Aturan yang hanya ditulis di dokumen akan dilanggar
dalam dua minggu.

## Lisensi

MIT.
