# ZONA HIJAU: internal/api

HTTP handler baca. Tulis bebas, ini plumbing.

Kontraknya sudah ada dan bukan urusan paket ini untuk mengubahnya:
`docs/api/keel-openapi.yaml`. Kalau handler butuh bentuk respons yang berbeda,
yang berubah kontraknya lewat catatan keputusan, bukan handler-nya diam-diam.

## Aturan

1. **API tidak pernah memanggil adapter.** Ia hanya membaca hasil yang sudah
   dihitung dari `internal/store`. Satu aset populer yang memicu panggilan
   Horizon per request akan menghabiskan kuota rate limit dalam hitungan menit,
   dan penggunanya mendapat latensi yang tidak terduga. Konsekuensinya metrik
   selalu sedikit tertinggal, dan itu diterima eksplisit di NFR-1.
2. **Seluruh nilai numerik dikirim sebagai string.** Amount Stellar adalah int64
   stroop dengan 7 desimal, dan JSON number adalah IEEE 754 double. Satu-satunya
   pengecualian: `delta` dan bilangan bulat seperti `ledgerSeq`.
3. **Konversi skala terjadi di sini, di satu tempat yang jelas.** Field
   berakhiran `Pct` di API berskala PERSEN. `spreadPct: '196.0777141'` berarti
   196 persen. Kalau ada satu field pecahan di antara field persen, seseorang
   pasti memakannya.
4. Setiap respons membawa `ledgerSeq` dan `methodologyVersion`, plus header
   `X-Keel-Staleness-Seconds` dan `X-Keel-Methodology-Version`.
5. **Aset tanpa harga bukan error.** `priceSource: none` dengan band `CRITICAL`
   adalah HTTP 200. Ledger yang belum tersedia adalah 404 dengan kode
   `LEDGER_NOT_AVAILABLE`, bukan 500. Buku dengan spread ratusan persen juga 200.
   Ketiganya temuan, bukan kegagalan.

## Yang belum diputuskan sebelum handler ditulis

Kontrak masih tertinggal dari `internal/domain/types.go` di empat tempat:
`costToMaxReachablePrice`, `unevaluatedFlags`, dan `bandConfidence` ada di kode
tetapi tidak ada di kontrak, dan `oracleResistance` berbentuk skalar di kode
tetapi objek di kontrak. Menulis handler di atas kontrak yang belum sinkron
berarti menulisnya dua kali.

Lihat `docs/internal/audit-2026-08-20.md` temuan P1-6 sampai P1-12.
