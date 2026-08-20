# Keel

Liquidity risk engine untuk ekosistem Stellar. Mengukur effective liquidity
depth dari orderbook SDEX dan reserve AMM pool.

Oracle menjawab "berapa harganya". Keel menjawab "berapa volume yang bisa
ditopang harga itu".

## Stack

- Go (backend). Postgres (storage). BigQuery/Hubble (data historis).
- Semua nilai moneter pakai `github.com/shopspring/decimal`. TIDAK PERNAH float64.
- SDK Stellar: `github.com/stellar/go-stellar-sdk/...` (BUKAN `github.com/stellar/go/...`).

## Aturan yang tidak bisa ditawar

1. Setiap output membawa `LedgerSeq` dan `MethodologyVersion`.
2. Iterasi map harus di-sort dulu sebelum dipakai (reproducibility, NFR-9).
3. `computeDepth()` adalah fungsi murni. Tidak ada network call di dalamnya.
4. Depth SDEX dan AMM digabung lewat batas marginal price bersama.
   TIDAK dijumlahkan secara terpisah.
5. Harga dibaca dari field `price_r` (pecahan n/d), BUKAN string `price`.

## Zona kerja

Repo ini dipakai untuk belajar backend Go, bukan hanya untuk menghasilkan kode.
Tiga zona:

- **HIJAU** (`internal/store`, `internal/api`, `migrations/`, `scripts/`,
  `docker-compose.yml`, Makefile): tulis bebas. Ini plumbing.
- **KUNING** (`internal/horizon`, `internal/hubble`, `internal/domain`):
  boleh tulis, tapi setelah selesai jelaskan setiap keputusan desain dalam
  3 kalimat. Sebutkan satu alternatif yang kamu tolak dan alasannya.
- **MERAH** (`internal/depth`): Al yang menulis. Kamu tidak punya izin tulis
  di sana (dikunci di .claude/settings.json). Perannya reviewer dan
  penanya, bukan penulis. Lihat internal/depth/CLAUDE.md.

Kalau Al minta kamu menulis kode zona merah, tolak dan tawarkan `/teach`
atau `/review-mine` sebagai gantinya.

## Gaya jawaban

- Jangan pakai em dash.
- Kalau Al salah, katakan langsung. Jangan validasi ide yang lemah.
- Kalau ada asumsi yang kamu buat, sebutkan eksplisit.

## Referensi (baca saat dibutuhkan, jangan dimuat semua)

- Kontrak API: @docs/api/keel-openapi.yaml
- Metodologi (deliverable berbayar): docs/methodology/
- Keputusan arsitektur: docs/decisions/
