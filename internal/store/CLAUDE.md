# GREEN ZONE: internal/store

Postgres persistence. Write freely, this is plumbing.

This package is **deliberately dumb**. It stores and it reads; it computes
nothing. If a formula starts appearing here it is in the wrong place and belongs
in `internal/domain/compute.go`, which is the red zone and not yours to write.

## Rules

1. Never add a `float`, `real`, or `double precision` column to any schema here.

   **This rule used to open by saying prices are stored as the fraction `price_n`
   and `price_d` rather than as one decimal column holding the quotient. No such
   column has ever existed.** `mid_price`, `max_reachable_price` and
   `pool_spot_price` are all single `NUMERIC` columns holding the quotient, and
   `domain.AssetRisk` has no numerator or denominator field for this package to
   read one from. The sentence is corrected rather than deleted because the intent
   behind it is sound and unfinished, and quietly dropping it would lose the
   intent along with the error.

   Where the exactness actually goes: Horizon's `price_r` IS an exact fraction,
   and `internal/horizon` converts it to a `decimal.Decimal` at `decode.go:95`,
   whose own comment records that "only the final division rounds, at shopspring's
   DivisionPrecision" (never configured, so 16 places). The fraction is gone
   before a price reaches this package, so `NUMERIC` here loses nothing further
   and this package cannot restore what it never receives. Making prices exact end
   to end means carrying the fraction through `domain.AssetRisk` first, which is a
   `types.go` change, and only then a column here.
2. Amounts use `NUMERIC` and are read into `decimal.Decimal`, never `float64`.
3. Every result row carries `ledger_seq` and `methodology_version`. Results from
   different methodology versions are different rows, not overwrites of each
   other. That is what makes cross-validation a single query.
4. The `data_source` column accepts four values: `horizon`, `hubble`,
   `offers-implied`, and `trades-implied`. The two implied ones are easy to
   forget, and a constraint that rejects them will fail on exactly the historical
   path the Blend case study depends on. They are not interchangeable:
   `offers-implied` means the book was rebuilt from offer operations, which proves
   liquidity that was POSTED, and `trades-implied` means it was rebuilt from
   trades, which proves only liquidity that was CONSUMED.

## The schema you are writing against

All three migration files, applied in order:

- `0001_core.sql`, reconciled with TDD section 5 on 20 August 2026. It holds
  `assets`, `metrics`, and `runs`.
- `0002_methodology_103.sql`, which added `pool_spot_price`,
  `price_divergence_pct`, and the two `C_max` terms.
- `0003_venue_split_and_offers_implied.sql`, which renamed `manipulation_cost` to
  `manipulation_cost_combined`, added `manipulation_cost_orderbook_only`, and
  widened the `data_source` CHECK to the four values in rule 4.
- `0004_history_source_index.sql`, which added
  `(asset_id, methodology_version, data_source, ledger_seq)` to back the history
  read now that it filters on the source. It is also the FIRST migration here to
  carry an explicit down, and its header says why 0001 through 0003 were not
  retrofitted with one: they are already applied, and an untested down written
  days later is a claim rather than a rollback. `scripts/migrate.sh` is
  forward-only and has no down command, so that section is applied by hand.

Raw snapshots are not in the database; the cross-validation recordings go to
`recordings/` as gzipped JSON.

Three things about it that are decisions rather than gaps:

- `oracle_resistance` is JSONB, not a numeric column. It was JSONB originally
  because `types.go` held a scalar and the contract defined an object, and the
  column deliberately did not prejudge which one won. The object won, in contract
  1.3.0 and in `types.go`, so JSONB is now the shape rather than a hedge, and it
  stays JSONB because that object has two fields. Finding P1-6, closed.
- `manipulation_cost_orderbook_only` is a stored column and not a view derived
  from the combined ladder, because the two are computed from different venue
  sets and neither can be recovered from the other. The GAP between them is the
  signal: a large combined cost beside a small orderbook-only cost is an asset
  that looks defended and is not.
- Read `0003`'s header before adding a migration. It contains this schema's one
  statement with no `IF NOT EXISTS`, and the reason that is deliberate.

Apply it with `make migrate`, never by mounting it into initdb. The reason is in
the comment at the top of `scripts/migrate.sh`.

## What exists here now

Written 24 August 2026. `store.go` holds `Open`, the decimal helpers and the
`dbtx` interface; `assets.go`, `metrics.go` and `runs.go` are one table each;
`jsonb.go` declares the shape of every JSONB column. The driver is
`github.com/jackc/pgx/v5/stdlib` behind `database/sql`, and no pgx type appears in
any signature here.

Four properties are worth knowing before changing anything, and each has a test
in `store_test.go` that fails if it stops holding:

1. **Every money value crosses this boundary as a string.** `String()` on the way
   in, `decimal.NewFromString` on the way out, `NUMERIC::text` in every SELECT.
   No `float64` on either path, which is rule 2 above and what the repository
   wide arch test enforces. A forty digit `spread_pct` round trips unchanged,
   which is what an unqualified `NUMERIC` is for.
2. **A metrics row is never overwritten.** The insert is `ON CONFLICT DO
   NOTHING` and `SaveMetrics` returns whether it wrote. Rule 3 says a different
   methodology version is a different row; the same argument forbids rewriting a
   row from the same version, because a re-run that silently changed a stored
   number would make the series useless as evidence.
3. **NULL means unknown or not applicable, and never zero.** Every nullable
   column maps to a `*decimal.Decimal`. A nil manipulation term says the attack
   is impossible; zero says it is free.
4. **A history read names all four key parts, and one series is one data
   source.** `MetricsHistory` filters asset, methodology version AND data source,
   and ranges only over the ledger, so one ledger yields at most one row. Until 26
   August 2026 it left `data_source` unconstrained: `GET /history` downsamples by
   keeping the last row in each bucket, `trades-implied` sorts last of the four
   alphabetically, so any ledger holding both a live reading and a reconstruction
   charted the LOWER BOUND and said nothing about it. An empty source argument
   means `horizon` specifically, never every source. `TestOneLedgerWithTwoSources\
IsNotTwoPointsInOneSeries` fails if that stops holding.

5. **The three `text[]` columns are written as arrays and read as JSONB.**
   `database/sql` can send a `[]string` to a `text[]` parameter and cannot scan
   one back, so the SELECT wraps them in `to_jsonb`. That also keeps the escaping
   in Postgres, which matters because `warnings` is free text containing commas
   and braces.

## Two things found while writing it

**`domain.AssetRisk` is NOT stored field for field, by one field.**
`Supporting.GenuineVolumeInWindow` has no column, and it has no field in the API
contract either; `internal/domain/types.go` is the only place it exists. Its
definition is an empty row in the table at the end of
`docs/methodology/07-supporting-metrics.md`. The header of `metrics.go` sets out
why no column was added rather than one being added quietly, and handoff item 17
is where the decision belongs.

**The container is published on `localhost:5433`, and `localhost:5432` is not
this project's Postgres.** A Postgres already installed on the host takes 5432
before the container can: the host server binds `127.0.0.1` while Docker binds the
wildcard, so the symptom is `role "keel" does not exist` rather than a refused
connection. `scripts/migrate.sh` is immune because it goes through `docker compose
exec` and never touches a published port, so the schema can be applied to the
container while the Go client talks to another server entirely. That happened on
26 August 2026, which is why the published port moved to 5433 and `DefaultDSN`
moved with it.

Moving the port narrows the collision and does not close it, so the escape hatches
stay: `keel assets` prints the hint on any connection failure, and `make store-test`
takes `KEEL_TEST_DSN` so the container can be addressed directly.
