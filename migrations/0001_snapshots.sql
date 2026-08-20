-- Orderbook and AMM snapshots. One row per capture per asset pair.
--
-- NOTE: this schema contradicts TDD section 5, which states that raw snapshots
-- are NOT stored in the database and defines the tables assets, metrics, and
-- runs instead. The metrics table that `keel serve` reads does not exist yet.
-- See findings P1-1 through P1-5 in docs/internal/audit-2026-08-20.md. Reconcile
-- before writing any query on top of this.

CREATE TABLE IF NOT EXISTS snapshots (
    id                  BIGSERIAL PRIMARY KEY,
    captured_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    ledger_seq          BIGINT      NOT NULL,
    methodology_version TEXT        NOT NULL,
    source              TEXT        NOT NULL CHECK (source IN ('horizon', 'hubble')),
    base_asset          TEXT        NOT NULL,
    counter_asset       TEXT        NOT NULL,
    raw                 JSONB       NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_snapshots_pair_time
    ON snapshots (base_asset, counter_asset, captured_at DESC);

CREATE INDEX IF NOT EXISTS idx_snapshots_ledger
    ON snapshots (ledger_seq);

-- Normalised orderbook levels.
-- Prices are stored as a fraction (price_n / price_d) so that no precision is
-- lost. DO NOT add a float column here.

CREATE TABLE IF NOT EXISTS snapshot_levels (
    id           BIGSERIAL PRIMARY KEY,
    snapshot_id  BIGINT NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
    venue        TEXT   NOT NULL CHECK (venue IN ('sdex', 'amm')),
    side         TEXT   NOT NULL CHECK (side IN ('bid', 'ask')),
    price_n      BIGINT NOT NULL,
    price_d      BIGINT NOT NULL CHECK (price_d > 0),
    amount       NUMERIC(38, 18) NOT NULL,
    level_index  INT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_levels_snapshot
    ON snapshot_levels (snapshot_id, venue, side, level_index);
