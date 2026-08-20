# ZONA KUNING: internal/hubble

Adapter untuk data historis dari Hubble (BigQuery dataset
`crypto-stellar.crypto_stellar`).

Output WAJIB bertipe `domain.Snapshot`, identik dengan internal/horizon.

## Batasan biaya

Berjalan di BigQuery Sandbox: 1TB query per bulan, gratis, tanpa billing.
Setiap query WAJIB:

- memfilter partisi (batasi rentang ledger atau tanggal) sebelum apa pun
- memilih kolom secara eksplisit, tidak pernah SELECT *
- didahului dry run untuk mengecek byte yang akan dipindai

Query tanpa filter partisi bisa memindai ratusan GB dalam sekali jalan.

## Aturan

Jangan pernah menuliskan issuer address aset dari ingatan. Kalau butuh
identitas aset, ambil dari `docs/decisions/` atau minta Al mengonfirmasi
dari sumber primer lebih dulu.
