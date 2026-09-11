-- The holder reading cache, and a third kind of run to fill it.
--
-- WHY A CACHE AND NOT A COLUMN ON metrics. A holder reading cannot be taken
-- inside a scan round, and that is arithmetic rather than preference. One reading
-- costs 1 summary request plus 1 per page of 200 accounts, capped at 25 pages by
-- defaultMaxHolderPages in internal/horizon/holders.go, so 26 requests per asset
-- worst case. Sixty assets is up to 1,560 requests per round; `scan` runs four
-- rounds an hour, which is up to 6,240 against a budget of 3,000 and public
-- Horizon's ~3,600 per IP. Lowering the page cap to make it fit would truncate
-- every popular asset, and a truncated set answers a concentration question not
-- at all rather than approximately. So the pull runs on its own cadence and the
-- scan reads the newest row from here.
--
-- THE CONSEQUENCE IS A ROW WITH TWO AGES, and it is DEC-018's subject rather than
-- this file's. That record is a DRAFT at the time this migration is written.
-- Nothing here depends on how it resolves: this table stores a reading and the
-- evidence for it, and the question of how stale a reading may be before `scan`
-- refuses it is answered in Go, not in the schema.
--
-- AGGREGATES ONLY, NOT ONE ROW PER HOLDER. Sixty assets times up to 5,000
-- accounts is 300,000 rows a day with no reader: the raw bytes belong in a
-- recording, which is what internal/horizon/recorder.go already writes. The cost
-- of that choice is real and is mitigated rather than ignored: HolderExclusions
-- is asset-specific and can change, and with aggregates only, changing it means
-- re-pulling. exclusions_applied records what was actually removed, so a row
-- computed under one list is never silently compared with a row computed under
-- another.
--
-- WHAT THIS TABLE DOES NOT YET CONTAIN HONESTLY, and it is named rather than
-- left to be discovered. DEC-011 (Accepted, 2 September 2026) defines
-- snapshot_ledger as the MINIMUM latest_ledger across all pages of a pull, and
-- requires counting rows whose last_modified_ledger exceeds it to label the pull
-- atomic or mixed. internal/horizon/holders.go implements none of that today: it
-- keeps the Latest-Ledger header of the first and last page only, and Holder
-- carries no last_modified_ledger at all. So:
--
--   snapshot_ledger        is filled with min(first page, last page), which is an
--                          approximation of DEC-011's definition
--   snapshot_ledger_basis  records WHICH definition produced it, so the two
--                          generations of row are told apart in the data and not
--                          in a comment somebody has to find
--   snapshot_label         is 'unknown' until the mutated-row count exists.
--                          Writing 'atomic' without having counted would be a
--                          claim this code cannot support, and 'atomic' is the
--                          value a reader would trust most
--   mutated_rows,          NULL for the same reason. NULL is unknown; 0 would be
--   mutated_balance        a measurement nobody made

BEGIN;

-- ---------------------------------------------------------------- runs.kind

-- A third kind of run. `scan` reads live data and `replay` recomputes a past
-- ledger; `holders` does neither, it pulls current trustline state on its own
-- schedule. It is a separate kind rather than a scan because quoting a holder
-- pull as a scan would put a figure with no ledger of its own inside a count of
-- rounds that each have one.
ALTER TABLE runs DROP CONSTRAINT IF EXISTS runs_kind_check;
ALTER TABLE runs ADD CONSTRAINT runs_kind_check
    CHECK (kind IN ('scan', 'replay', 'holders'));

-- ---------------------------------------------------------------- readings

-- KEYED BY THE ASSET AND NOT BY assets.id, WHICH IS A PAIR. The `assets` table
-- is one row per (base, quote): its unique key is
-- (code, issuer, quote_code, quote_issuer), and metrics.asset_id therefore names
-- a pair. Holder concentration is a property of ONE asset and has nothing to say
-- about which quote it is measured against, so a foreign key to that table would
-- claim otherwise and would make the same asset appear twice the moment it is
-- listed against two quotes. It would also pull it twice, and section one of this
-- file is about a request budget.
--
-- THE NATIVE ASSET CANNOT APPEAR HERE AT ALL, and the CHECK says so rather than
-- leaving it to the caller. XLM has no trustlines, so /accounts cannot enumerate
-- its holders; horizon.ErrNativeHasNoTrustlines is the same statement in Go.
-- issuer is NOT NULL for the same reason, which also sidesteps the NULLS NOT
-- DISTINCT trap that 0005 was written to repair.
CREATE TABLE IF NOT EXISTS holder_readings (
    id     BIGSERIAL PRIMARY KEY,
    code   TEXT NOT NULL,
    issuer TEXT NOT NULL,
    type   TEXT NOT NULL CHECK (type IN ('credit_alphanum4', 'credit_alphanum12')),

    -- The run that produced it. Nullable because a reading taken by hand for an
    -- evidence file is still a reading, and refusing to store it would push that
    -- case into a spreadsheet nothing can query.
    run_id    BIGINT REFERENCES runs (id),

    fetched_at TIMESTAMPTZ NOT NULL,

    -- ------------------------------------------------ DEC-011 snapshot evidence
    snapshot_ledger BIGINT NOT NULL,

    -- HOW snapshot_ledger was derived. See the header. 'first-and-last-page' is
    -- the approximation in force until DEC-011's implementation lands in
    -- internal/horizon/holders.go; 'all-pages-min' is the definition itself.
    snapshot_ledger_basis TEXT NOT NULL
        CHECK (snapshot_ledger_basis IN ('first-and-last-page', 'all-pages-min')),

    -- Maximum minus minimum latest_ledger. DEC-011 refuses a pull above
    -- MaxLedgerSpan = 24, which is about two minutes of ledgers. The refusal is
    -- the pull's to make; a stored row is one that was accepted.
    ledger_span INT NOT NULL CHECK (ledger_span >= 0),

    snapshot_label TEXT NOT NULL
        CHECK (snapshot_label IN ('atomic', 'mixed', 'unknown')),
    mutated_rows    INT CHECK (mutated_rows >= 0),
    mutated_balance NUMERIC,

    -- ------------------------------------------------ what the pull returned
    -- holders_read is len(Holders) and holder_count_reported is Horizon's own
    -- figure. Their disagreement IS the truncation, and both are stored so a row
    -- is readable without consulting anything else.
    holders_read          INT     NOT NULL CHECK (holders_read >= 0),
    holder_count_reported INT     NOT NULL CHECK (holder_count_reported >= 0),
    truncated             BOOLEAN NOT NULL,

    -- ------------------------------------------------ the computed concentration
    -- All nullable, and the CHECK below is what makes that mean something.
    population         INT,
    circulating_supply NUMERIC,
    top1_pct           NUMERIC,
    top10_pct          NUMERIC,
    -- Sum of squared PERCENTAGE shares, so it runs to 10,000 for a single holder
    -- and not to 1. internal/domain/supporting.go carries the same note.
    hhi                NUMERIC,
    zero_balance_dropped INT,
    excluded_dropped     INT,

    -- What was actually removed before the percentages were taken: the issuer,
    -- and any explicit pool or contract addresses. Stored per row because the
    -- list is asset-specific and may change.
    exclusions_applied JSONB NOT NULL,

    methodology_version TEXT NOT NULL,

    -- A TRUNCATED READING IS STORED, AND IT IS STORED WITHOUT NUMBERS. That is
    -- this constraint, and it is the schema enforcing what
    -- domain.ErrHolderSetTruncated says in Go: a truncated trustline set answers
    -- a concentration question not at all, so zero and absent are different
    -- values and only absent is available. Dropping the row instead would lose
    -- the evidence that the pull happened and hit the cap.
    CONSTRAINT holder_readings_truncated_has_no_figures CHECK (
        NOT truncated OR (
            population IS NULL AND circulating_supply IS NULL AND
            top1_pct IS NULL AND top10_pct IS NULL AND hhi IS NULL
        )
    ),

    -- One reading per asset per snapshot per methodology version. The version is
    -- in the key for the same reason it is in the metrics key: a figure produced
    -- under a different definition is a different figure, not an overwrite.
    UNIQUE (code, issuer, snapshot_ledger, methodology_version)
);

-- The one query `scan` will make: the newest reading for one asset. fetched_at
-- and not snapshot_ledger, because the question is "how old is this reading"
-- and a clock answers that where a ledger sequence does not.
CREATE INDEX IF NOT EXISTS idx_holder_readings_latest
    ON holder_readings (code, issuer, fetched_at DESC);

COMMIT;
