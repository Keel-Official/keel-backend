# Keel

Liquidity risk engine untuk ekosistem Stellar.

Oracle menjawab "berapa harganya". Keel menjawab "berapa volume yang bisa
ditopang harga itu".

## Mulai

```bash
make up          # nyalakan Postgres lokal
make test        # jalankan test
make record      # jalankan snapshot recorder
```

## Struktur

- `cmd/keel` entrypoint
- `internal/domain` tipe bersama, terutama `Snapshot`
- `internal/horizon` adapter data live
- `internal/hubble` adapter data historis (BigQuery)
- `internal/depth` inti metodologi, ditulis manual
- `internal/store` persistensi
- `internal/api` HTTP handler
- `docs/methodology` deliverable metodologi
- `docs/decisions` catatan keputusan
- `docs/learning` jurnal belajar
