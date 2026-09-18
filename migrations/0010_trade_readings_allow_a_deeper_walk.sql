-- A second walk of the same pair on the same day may be DEEPER, not a repeat.
--
-- WHAT 0009 GOT WRONG, IN ITS OWN WORDS. Its unique index carried this comment:
-- "One reading per pair per day per methodology version. A second pull on the
-- same day is a repeat of the same measurement, and storing both would let two
-- rows with the same anchor disagree." The first sentence is false and was
-- disproved on the day it was written.
--
-- HOW IT WAS DISPROVED. `keel trades` bounds a walk in pages as well as in days,
-- because a busy pool-filled pair can spend an entire request budget without
-- reaching a genuine day (DEC-019 section 9.8). HU/USDC was stopped by that
-- bound after 11 days. Re-walked at 19:13 UTC on 18 September 2026 with
-- `-max-pages 1500` it reached 20 days, which is a strictly better measurement
-- of the same day: the walk covers whole UTC days contiguously backwards from
-- one anchor, so twenty days back CONTAINS eleven days back. The store refused
-- it, the 1,500 requests bought nothing, and `-refresh` was a flag that walked
-- and then threw its result away.
--
-- WHY THE ROW IS ADDED AND THE OLD ONE IS NOT REPLACED. internal/store/CLAUDE.md
-- property 2 says a stored row is never overwritten, and the reason applies here
-- unchanged: a re-run that silently changed a stored number would make the
-- series useless as evidence. Appending is not overwriting. Both readings
-- survive, each with the bound it ran under, and a reader can see that the
-- deeper one was bought later.
--
-- WHICH ROW WINS, AND WHY IT CANNOT REGRESS. The reader takes the greatest
-- days_walked for the newest anchor, not the newest row. Depth is monotone for
-- this walk: a genuine trade found within eleven days is still found within
-- twenty, so a deeper reading is never a worse answer. Ordering by id instead
-- would let a shallower re-walk, a pass run with a smaller bound by mistake,
-- quietly undo a deeper one.

DROP INDEX IF EXISTS trade_readings_one_per_day_idx;

-- What the unique index was also doing, kept: the read path needs to find the
-- rows for one pair and day quickly. This is the non-unique form of the same
-- key, with depth in it so the reader's ORDER BY is covered.
CREATE INDEX IF NOT EXISTS trade_readings_day_depth_idx
    ON trade_readings (asset_id, methodology_version, anchor DESC, days_walked DESC);

-- No down section. scripts/migrate.sh is forward-only and 0004's header records
-- why a down written days later and never run is a claim rather than a rollback.
-- Reversing this one means restoring the unique index, which would fail against
-- any day that already holds two readings, and that failure is the correct
-- behaviour rather than something to paper over.
