-- Keel core schema. Replaces the previous 0001_snapshots.sql.
--
-- WHY THIS FILE REPLACED THE OLD ONE. The old migration stored raw snapshots in
-- the database plus a normalized levels table, and had no table for computed
-- results at all. TDD section 5 states the opposite on both counts: raw snapshots
-- are NOT stored in the database, and the tables are assets, metrics, and runs.
-- The API is designed to read results already computed, and the table holding
-- those results did not exist. See findings P1-1 through P1-5 in
-- docs/internal/audit-2026-08-20.md.
--
-- The TDD won the contradiction because it carries the reasoning and the
-- migration did not: for 50 assets every 15 minutes across 30 days, storing raw
-- snapshots is tens of gigabytes for no benefit. What is kept instead is a
-- bounded set of recordings for cross-validation, as gzipped JSON files under
-- recordings/, 60 of which go into git as evidence.
--
-- NUMERIC IS DELIBERATELY UNQUALIFIED. In Postgres an unqualified NUMERIC is
-- arbitrary precision, so nothing is silently rounded on the way in. A
-- NUMERIC(38, 18) would impose a scale this product has no basis for choosing.
-- Never add a float, real, or double precision column to this schema.

-- ---------------------------------------------------------------- assets

-- The demonstration set. One row per (asset, quote) pair that is scanned.
CREATE TABLE IF NOT EXISTS assets (
    id             SERIAL PRIMARY KEY,
    code           TEXT        NOT NULL,
    issuer         TEXT,                       -- NULL if and only if type = 'native'
    -- Sent explicitly, never inferred from the length of code. A five character
    -- code such as USTRY is credit_alphanum12, and querying Horizon with the
    -- wrong type returns an empty result and no error.
    type           TEXT        NOT NULL CHECK (type IN ('native', 'credit_alphanum4', 'credit_alphanum12')),
    quote_code     TEXT        NOT NULL,       -- the primary pair
    quote_issuer   TEXT,
    quote_type     TEXT        NOT NULL CHECK (quote_type IN ('native', 'credit_alphanum4', 'credit_alphanum12')),
    active         BOOLEAN     NOT NULL DEFAULT TRUE,
    selection_note TEXT,                       -- why this asset is in the demonstration set
    added_at       TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT assets_native_has_no_issuer
        CHECK ((type = 'native') = (issuer IS NULL)),
    CONSTRAINT assets_quote_native_has_no_issuer
        CHECK ((quote_type = 'native') = (quote_issuer IS NULL)),
    UNIQUE (code, issuer, quote_code, quote_issuer)
);

-- ---------------------------------------------------------------- metrics

-- One computed result per asset per ledger per methodology version per source.
--
-- The columns mirror domain.AssetRisk field for field. When that type gains a
-- field, this table gains a column in a new migration; the two are meant to be
-- readable side by side.
CREATE TABLE IF NOT EXISTS metrics (
    id                  BIGSERIAL PRIMARY KEY,
    asset_id            INT         NOT NULL REFERENCES assets(id),
    ledger_seq          BIGINT      NOT NULL,
    ledger_closed_at    TIMESTAMPTZ NOT NULL,
    computed_at         TIMESTAMPTZ NOT NULL,
    methodology_version TEXT        NOT NULL,

    -- 'trades-implied' is easy to forget and a constraint that rejects it fails
    -- on exactly the historical path the Blend case study depends on. A number
    -- carrying this source is a lower bound, not a measurement.
    data_source         TEXT        NOT NULL CHECK (data_source IN ('horizon', 'hubble', 'trades-implied')),

    -- NULL when price_source = 'none'. A populated mid_price is still not
    -- necessarily meaningful: read spread_pct alongside it.
    mid_price           NUMERIC,
    price_source        TEXT        NOT NULL CHECK (price_source IN ('book', 'pool', 'none')),

    -- In PERCENT, matching every quantity whose name ends in Pct. NULL when
    -- either side of the book is empty, because a spread is undefined without
    -- two sides. NULL means unknown, not zero.
    spread_pct          NUMERIC,

    -- [{delta, buySide, sellSide, fromSdex, fromAmm}]. fromSdex and fromAmm are
    -- kept so a third party can verify the combination without reading the code.
    depth               JSONB       NOT NULL,
    -- [{delta, targetPrice, cost, reachable}]. cost must never be read without
    -- reachable.
    manipulation_cost   JSONB       NOT NULL,

    -- The highest ask price on the book and what reaching it costs. NULL when
    -- there is no ask, or when all liquidity is in an AMM, because a constant
    -- product curve has no upper price bound.
    max_reachable_price         NUMERIC,
    cost_to_max_reachable_price NUMERIC,

    -- JSONB rather than a numeric column on purpose. types.go holds this as a
    -- scalar while the API contract defines an object, and that conflict is not
    -- resolved yet (finding P1-6, task T6). JSONB holds either shape, so this
    -- schema does not prejudge the decision.
    oracle_resistance   JSONB,

    max_safe_collateral NUMERIC,

    holder_top1_pct     NUMERIC,
    holder_top10_pct    NUMERIC,
    holder_hhi          NUMERIC,
    volume_to_supply    JSONB,
    last_genuine_trade  JSONB,
    trades_excluded_pct NUMERIC,

    flags               TEXT[]      NOT NULL DEFAULT '{}',
    -- Flags that could not be checked because the data they need is absent.
    -- unevaluated is NOT a synonym for clear, which is why this is a separate
    -- column and not an absence from flags.
    unevaluated_flags   TEXT[]      NOT NULL DEFAULT '{}',
    band                TEXT        NOT NULL CHECK (band IN ('LOW', 'MEDIUM', 'HIGH', 'CRITICAL')),
    -- 'partial' when any flag at the CRITICAL or HIGH level is unevaluated. A
    -- LOW band with partial confidence is a far weaker statement than LOW with
    -- full confidence.
    band_confidence     TEXT        NOT NULL CHECK (band_confidence IN ('full', 'partial')),
    warnings            TEXT[]      NOT NULL DEFAULT '{}',

    -- methodology_version and data_source are part of the key on purpose. A
    -- result from a different methodology or a different source is a different
    -- row, not an overwrite. That is what makes cross-validation a single query,
    -- and what stops a mid-sprint definition change from silently corrupting the
    -- time series.
    UNIQUE (asset_id, ledger_seq, methodology_version, data_source)
);

CREATE INDEX IF NOT EXISTS idx_metrics_asset_ledger
    ON metrics (asset_id, ledger_seq DESC);

-- Partial index: the dashboard and any alerting only ever ask for the bad end.
CREATE INDEX IF NOT EXISTS idx_metrics_band_bad
    ON metrics (band) WHERE band IN ('HIGH', 'CRITICAL');

-- ---------------------------------------------------------------- runs

-- One row per scan or replay job. assets_failed is what makes a partial failure
-- visible instead of silent: one asset failing must not fail the whole scan.
CREATE TABLE IF NOT EXISTS runs (
    id            BIGSERIAL PRIMARY KEY,
    kind          TEXT        NOT NULL CHECK (kind IN ('scan', 'replay')),
    started_at    TIMESTAMPTZ NOT NULL,
    finished_at   TIMESTAMPTZ,
    assets_ok     INT         NOT NULL DEFAULT 0,
    assets_failed INT         NOT NULL DEFAULT 0,
    notes         TEXT
);

-- ---------------------------------------------------------------- not here

-- Deliberately absent, and both absences are decisions rather than omissions:
--
--   Raw snapshots. TDD section 5. They live in recordings/ as gzipped JSON.
--
--   The two C_max terms. Methodology section 9 requires the liquidation limit
--   and the manipulation limit to be reported separately rather than only their
--   minimum, but domain.AssetRisk carries only the minimum today. That is
--   finding P1-15, and closing it needs a change to the type first, then a
--   migration here. Adding columns now for values nothing produces would be
--   clutter that reads like a promise.
