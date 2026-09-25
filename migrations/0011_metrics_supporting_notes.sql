-- A metrics row says why each absent supporting figure is absent.
--
-- WHY THE COLUMN EXISTS. On 25 September 2026 the live API carried holder
-- concentration for 35 of 61 assets, volume-to-supply for 26 and a last genuine
-- trade for 30, and every absence reached the dashboard as the same null. Four
-- different answers were collapsed into it: a trustline set too large for the
-- holder pull, a pair above the DEC-019 threshold, a walk that spent its page
-- bound, and a search that covered thirty whole days and found no genuine trade.
-- The scan already knew which one applied and wrote it only to its log.
--
-- JSONB AND NOT FOUR TEXT COLUMNS, for the argument 0008 makes: the notes are
-- produced together, read together, and never filtered or aggregated on.
--
-- NULL MEANS "NOTHING TO EXPLAIN", which covers two cases a reader separates by
-- looking at the figures beside it: a row whose supporting figures are all
-- present, and a row written before this column existed. A note is never stored
-- beside a figure that is present; cmd/keel/scan.go clears it.
--
-- No constraint. A note is prose for a reader and nothing computes from it, so
-- there is no invariant for the database to hold.

ALTER TABLE metrics
    ADD COLUMN IF NOT EXISTS supporting_notes JSONB;

COMMENT ON COLUMN metrics.supporting_notes IS
    'Why each absent supporting figure is absent: keys holders, tradesExcludedPct, '
    'volumeToSupply, lastGenuineTrade, each present only when that figure is null. NULL '
    'means nothing to explain, either because every figure is present or because the row '
    'predates the column. See migrations/0011.';
