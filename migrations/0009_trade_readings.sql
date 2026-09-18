-- The trade half of the supporting metrics, cached on its own cadence.
--
-- DEC-019, option B at a threshold of 20,000 trades in 30 days, accepted by Al on
-- 18 September 2026. This table is to the trade stream what holder_readings is to
-- the trustline set, and the shape is deliberately the same one: a pull too
-- expensive for a scan round writes here, and `keel scan` reads the newest row.
-- DEC-018 section 2 forced that shape for holders and DEC-019 section 3 reaches
-- the same conclusion for trades by the same arithmetic, so this is the second
-- application of a settled pattern rather than a new idea.
--
-- WHY THE ROW STORES FIGURES AND NOT TRADES. Five million trades across the
-- demonstration set is the measurement in DEC-019 section 2, and storing them
-- would make this table the largest object in the database by two orders of
-- magnitude while answering nothing the figures do not. The classification that
-- produced these numbers is domain.ClassifyTrades and it ran at pull time, in the
-- command whose budget allows the walk, exactly as domain.HolderConcentration
-- runs inside `keel holders`. A reader who wants the trades goes to Horizon,
-- which still has them, using anchor and ledger_seq to say which walk to repeat.
--
-- WHY THE ANCHOR IS A DAY BOUNDARY AND NOT THE PULL TIME. DEC-019 section 8.1 is
-- the correction that produced the walk behind this table: a partial UTC day
-- cannot be classified, because condition 5 of the genuine-trade rules reads that
-- day's median. So the walk covers whole days only and every window here is
-- measured back from the last complete one. That makes every figure in this table
-- up to 24 hours stale by construction, which is the direction section 8.2
-- permits: a staleness flag may fire early and must never fire late.

-- ---------------------------------------------------------------- runs.kind
--
-- A fourth kind, for the same reason 0006 added the third: the pass opens and
-- closes a run row so an operator can see when it last succeeded, and a kind
-- borrowed from `scan` would make two different jobs indistinguishable in the
-- one table that says what has been running.
ALTER TABLE runs DROP CONSTRAINT IF EXISTS runs_kind_check;
ALTER TABLE runs ADD CONSTRAINT runs_kind_check
    CHECK (kind IN ('scan', 'replay', 'holders', 'trades'));

CREATE TABLE IF NOT EXISTS trade_readings (
    id       BIGSERIAL PRIMARY KEY,

    -- The PAIR, not the asset. Trades exist between two assets, unlike a
    -- trustline set, so this references the assets row rather than repeating the
    -- six columns that name a pair.
    asset_id INT    NOT NULL REFERENCES assets (id),

    -- The run that produced it. Nullable for the same reason holder_readings
    -- allows it: a reading taken by hand for an evidence file is still a reading.
    run_id   BIGINT REFERENCES runs (id),

    fetched_at          TIMESTAMPTZ NOT NULL,
    methodology_version TEXT        NOT NULL,

    -- ------------------------------------------------------------- provenance

    -- The exclusive upper bound of every window below: 00:00:00Z of the day the
    -- pull ran. Windows are [anchor - 24h, anchor), and so on.
    anchor      TIMESTAMPTZ NOT NULL,

    -- Where the reading started, and it does not pretend a multi-page walk was
    -- atomic.
    --
    -- NULLABLE, WHICH THE HOLDER TABLE'S EQUIVALENT IS NOT, and the reason is a
    -- measurement rather than caution. Public Horizon does not send a
    -- Latest-Ledger header on /trades, checked on 18 September 2026, so this is
    -- the ledger of the NEWEST trade in the walk, taken from that trade's own
    -- paging token. A pair that had no trade at all in the walked window has no
    -- such trade and therefore no ledger to cite, and inventing one from the
    -- server's tip would be provenance for data that does not exist. The
    -- constraint below is what stops that absence spreading: a row that names a
    -- genuine trade has seen a trade, so it must be able to name its ledger.
    ledger_seq  BIGINT      CHECK (ledger_seq IS NULL OR ledger_seq > 0),

    days_walked INT         NOT NULL CHECK (days_walked >= 0),
    pages       INT         NOT NULL CHECK (pages >= 0),

    -- The walk reached the end of the pair's history AND handed every day of it
    -- over. A pull that found no genuine trade over an exhausted walk has
    -- measured that the pair never traded; one that merely ran out of days has
    -- measured nothing. DEC-019 section 8.4 item 2 refuses to collapse the two.
    exhausted     BOOLEAN NOT NULL,
    bound_reached BOOLEAN NOT NULL,

    -- ------------------------------------------------------- what the pull did
    --
    -- 'last-genuine-only' is the cheap half of DEC-019: walk back until a day
    -- holds a genuine trade and stop. 'full-window' is the cheap half plus the
    -- expensive one, thirty whole days classified, which the threshold admits.
    scope TEXT NOT NULL CHECK (scope IN ('last-genuine-only', 'full-window')),

    -- Why the volume half is absent, in one phrase, when it is absent. DEC-019
    -- section 5 asks for this by name: a reader cannot tell from `unevaluated`
    -- alone whether an asset was too busy to classify or too quiet to have data,
    -- and those are opposite findings.
    volume_unevaluated_reason TEXT,

    -- --------------------------------------------------------------- FR-10
    --
    -- The last genuine trade, as a reference rather than a copy, which is what
    -- domain.TradeRef is. Both halves are set or neither is.
    last_genuine_at     TIMESTAMPTZ,
    last_genuine_ledger BIGINT CHECK (last_genuine_ledger IS NULL OR last_genuine_ledger > 0),

    -- ---------------------------------------------------------------- FR-9
    --
    -- Genuine BASE volume per window. The ratio is not stored: its denominator is
    -- circulating supply, which belongs to the holder pull and moves on its own
    -- cadence, so dividing here would freeze one half of a fraction against the
    -- other. domain.VolumeToSupply performs the division where the two readings
    -- meet, in the scan round that carries both.
    genuine_base_d1  NUMERIC CHECK (genuine_base_d1 IS NULL OR genuine_base_d1 >= 0),
    genuine_base_d7  NUMERIC CHECK (genuine_base_d7 IS NULL OR genuine_base_d7 >= 0),
    genuine_base_d30 NUMERIC CHECK (genuine_base_d30 IS NULL OR genuine_base_d30 >= 0),

    -- Section 4 of the methodology: the QUOTE volume inside the oracle window,
    -- which feeds the manipulation ratio and is compared against a cost
    -- denominated the same way.
    genuine_quote_oracle_window NUMERIC CHECK (genuine_quote_oracle_window IS NULL OR genuine_quote_oracle_window >= 0),
    oracle_window_recorded      INT     CHECK (oracle_window_recorded IS NULL OR oracle_window_recorded >= 0),

    -- The share of recorded volume the genuine rules excluded, which is what
    -- WASH_TRADE_SUSPECTED fires on. Null when the full window was not walked.
    trades_excluded_pct NUMERIC CHECK (trades_excluded_pct IS NULL OR (trades_excluded_pct >= 0 AND trades_excluded_pct <= 100)),

    -- A reference is whole or absent. Half of one is a row that says a genuine
    -- trade happened and refuses to say when, which no consumer can use.
    CONSTRAINT trade_readings_genuine_ref_whole CHECK (
        (last_genuine_at IS NULL) = (last_genuine_ledger IS NULL)
    ),

    -- A row that found a genuine trade saw at least one trade, so it can name
    -- the ledger the walk started at. See the ledger_seq comment above.
    CONSTRAINT trade_readings_cited_row_has_a_ledger CHECK (
        last_genuine_at IS NULL OR ledger_seq IS NOT NULL
    ),

    -- The cheap half may not carry volume figures, and the full walk must carry
    -- its reason when it has none. This is the constraint that stops a
    -- 'last-genuine-only' row being read as a measured zero.
    CONSTRAINT trade_readings_scope_matches_volume CHECK (
        (scope = 'full-window')
        OR (genuine_base_d1 IS NULL AND genuine_base_d7 IS NULL AND genuine_base_d30 IS NULL
            AND trades_excluded_pct IS NULL AND volume_unevaluated_reason IS NOT NULL)
    )
);

-- The newest reading for a pair at a methodology version, which is the only
-- query `keel scan` makes against this table.
CREATE INDEX IF NOT EXISTS trade_readings_latest_idx
    ON trade_readings (asset_id, methodology_version, anchor DESC);

-- One reading per pair per day per methodology version. A second pull on the
-- same day is a repeat of the same measurement, and storing both would let two
-- rows with the same anchor disagree.
CREATE UNIQUE INDEX IF NOT EXISTS trade_readings_one_per_day_idx
    ON trade_readings (asset_id, methodology_version, anchor);

-- No INSERT INTO schema_migrations here. scripts/migrate.sh appends that line
-- itself, keyed on the FILENAME, and runs the whole file in one transaction with
-- psql -1. A file that records itself would write a second row under a different
-- key and the runner would then re-apply it on the next run.
