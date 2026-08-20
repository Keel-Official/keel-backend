# GREEN ZONE: internal/store

Postgres persistence. Write freely, this is plumbing.

This package is **deliberately dumb**. It stores and it reads; it computes
nothing. If a formula starts appearing here it is in the wrong place and belongs
in `internal/depth`.

## Rules

1. Prices are stored as the fraction `price_n` and `price_d`, not as one decimal
   column holding the quotient. Never add a `float`, `real`, or
   `double precision` column to any schema here.
2. Amounts use `NUMERIC` and are read into `decimal.Decimal`, never `float64`.
3. Every result row carries `ledger_seq` and `methodology_version`. Results from
   different methodology versions are different rows, not overwrites of each
   other. That is what makes cross-validation a single query.
4. The `data_source` column accepts three values: `horizon`, `hubble`, and
   `trades-implied`. The third is easy to forget, and a constraint that rejects it
   will fail on exactly the historical path the Blend case study depends on.

## Settle this before writing here

`migrations/0001_snapshots.sql` still contradicts TDD section 5. The TDD states
that raw snapshots are NOT stored in the database and defines the tables
`assets`, `metrics`, and `runs`. The migration that exists does the opposite, and
the `metrics` table that `keel serve` reads does not exist at all.

Settle that first. Do not write queries on top of a schema that is still forked.
See findings P1-1 through P1-5 in `docs/internal/audit-2026-08-20.md`.
