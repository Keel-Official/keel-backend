-- A metrics row carries two ledgers, and the older half gets its own.
--
-- DEC-018 point 2, accepted by Al on 12 September 2026. The rest of that record
-- is still a draft; this migration implements the accepted point and nothing
-- else.
--
-- WHY. Until `scan` read a cached holder pull, every field in a metrics row came
-- from one ledger and `ledger_seq` answered "as of when" for the whole row. That
-- stopped being true when holder concentration started arriving from a pull taken
-- on its own cadence, up to a day earlier. The risk is not that the row is wrong.
-- It is that it looks exactly like the old one: a reader taking `ledger_seq` to
-- cover the row reads a holder figure as current when it can be a day old, and
-- nothing in the row contradicts them. Non-negotiable rule 1 exists so that no
-- number circulates without the ledger it came from, and a holder half wearing the
-- row's ledger is that failure wearing a disguise.
--
-- THE GAP IS READABLE WITHOUT A JOIN AND WITHOUT A CLOCK, which is why this is a
-- ledger and not a timestamp. `ledger_seq - holder_snapshot_ledger` is the
-- distance between the two halves in ledgers, and Stellar closes one about every
-- five seconds, so the age is arithmetic on two columns of the same row. A
-- fetched_at column would answer the same question and would need the reader to
-- hold two units at once.
--
-- THE CONSTRAINT IS `NOT VALID` ON PURPOSE, AND IT IS NOT A WEAKER CONSTRAINT.
-- Postgres applies NOT VALID to every INSERT and UPDATE from now on and merely
-- skips the scan of rows that already exist. So the invariant binds completely
-- going forward, which is the whole of what it is for.
--
-- What it deliberately does NOT do is touch the rows written between the commit
-- that wired holder metrics into `scan` and this one. Those carry holder figures
-- and no provenance, which is exactly the state this record describes. Two ways
-- to make them conform were rejected:
--
--   Null their holder figures. That is an overwrite of a stored measurement, and
--   decision 2 in internal/store/store.go forbids exactly that: a result is never
--   rewritten, because a re-run that silently changed a stored number makes the
--   time series unusable as evidence. A migration is not an exemption from that.
--
--   Backfill the ledger by joining the newest holder_readings row before
--   computed_at. It would guess. The row that produced a figure and the newest
--   row before it are usually the same and are not always, and a guessed
--   provenance is worse than an absent one because it cannot be told apart from a
--   measured one.
--
-- So legacy rows stay exactly as written, and they are identifiable in one query:
--
--   SELECT * FROM metrics
--    WHERE holder_top1_pct IS NOT NULL AND holder_snapshot_ledger IS NULL;
--
-- That set can only grow by zero. Run `ALTER TABLE metrics VALIDATE CONSTRAINT
-- metrics_holder_figures_carry_their_ledger;` once it is empty, or leave it and
-- let the query above be the record of when the invariant started.

BEGIN;

ALTER TABLE metrics
    ADD COLUMN IF NOT EXISTS holder_snapshot_ledger BIGINT;

COMMENT ON COLUMN metrics.holder_snapshot_ledger IS
    'The ledger the holder concentration figures were read at, which is NOT ledger_seq: '
    'they come from a trustline pull taken on its own cadence. NULL means the row carries '
    'no holder figures, or that it predates DEC-018 point 2. See migrations/0007.';

-- Never null when a holder figure is present. The three move together: they are
-- produced by one call to domain.HolderConcentration over one pull, so a row with
-- one of them and not the others is already impossible upstream, and this says so
-- here too.
ALTER TABLE metrics
    ADD CONSTRAINT metrics_holder_figures_carry_their_ledger CHECK (
        holder_snapshot_ledger IS NOT NULL
        OR (holder_top1_pct IS NULL AND holder_top10_pct IS NULL AND holder_hhi IS NULL)
    ) NOT VALID;

COMMIT;
