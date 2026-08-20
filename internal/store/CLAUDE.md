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

## The schema you are writing against

`migrations/0001_core.sql`, reconciled with TDD section 5 on 20 August 2026. It
holds `assets`, `metrics`, and `runs`. Raw snapshots are not in the database; the
cross-validation recordings go to `recordings/` as gzipped JSON.

Two things about it that are decisions rather than gaps:

- `oracle_resistance` is JSONB, not a numeric column, because `types.go` holds it
  as a scalar while the API contract defines an object and that conflict is not
  resolved yet. JSONB holds either shape, so the schema does not prejudge it.
  Finding P1-6.
- There are no columns for the two `C_max` terms. Methodology section 9 requires
  both to be reported rather than only their minimum, but `domain.AssetRisk`
  carries only the minimum today. Closing that needs the type to change first,
  then a migration. Finding P1-15.

Apply it with `make migrate`, never by mounting it into initdb. The reason is in
the comment at the top of `scripts/migrate.sh`.
