# ZONA KUNING: internal/horizon

Adapter untuk data live dari Horizon API.

Output WAJIB bertipe `domain.Snapshot`, sama persis dengan yang dihasilkan
`internal/hubble`. Kalau kedua adapter menghasilkan tipe berbeda, desainnya
gagal dan `computeDepth()` jadi ikut berubah.

## Endpoint

- `GET /order_book` dengan parameter selling_asset_* dan buying_asset_*
- `GET /liquidity_pools`
- `GET /assets?asset_code=...` untuk verifikasi identitas aset

## Jebakan yang sering terjadi

1. Import path lama `github.com/stellar/go/...`. Yang benar
   `github.com/stellar/go-stellar-sdk/...`.
2. Membaca field string `price`. Yang benar `price_r` yang berbentuk
   `{"n": 1, "d": 10}`. Field string sudah kehilangan presisi.
3. Horizon TIDAK menyediakan data historis. Kalau butuh masa lalu, itu
   urusan internal/hubble. Jangan pernah mengarang endpoint historis.

## Setelah menulis kode di sini

Jelaskan dalam 3 kalimat: keputusan desain apa yang kamu ambil, satu
alternatif yang kamu tolak, dan kenapa.
